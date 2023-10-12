package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/isletnet/uptp/logging"
)

type portPacketHead struct {
	PacketType uint8
	ConnSN     uint64
	BodyLen    uint16
}

var (
	portPacketHeadSize = binary.Size(portPacketHead{})
)

const (
	packetTypeConnect      = 0
	packetTypeDisconnect   = 1
	packetTypeBinary       = 2
	packetTypeHeartbeat    = 3
	packetTypeHeartbeatACK = 4

	readBuffLen = 4096
)

type multiKeyMap struct {
	mtx        sync.Mutex
	mapSnAddr  map[uint64]string
	mapAddrSn  map[string]uint64
	mapSnValue sync.Map
}

func (mkm *multiKeyMap) init() {
	mkm.mapSnAddr = make(map[uint64]string)
	mkm.mapAddrSn = make(map[string]uint64)
}

func (mkm *multiKeyMap) LoadWithAddr(addr string) (interface{}, bool) {
	mkm.mtx.Lock()
	defer mkm.mtx.Unlock()
	sn, ok := mkm.mapAddrSn[addr]
	if !ok {
		return nil, false
	}
	return mkm.mapSnValue.Load(sn)
}
func (mkm *multiKeyMap) LoadWithSn(sn uint64) (interface{}, bool) {
	mkm.mtx.Lock()
	defer mkm.mtx.Unlock()
	return mkm.mapSnValue.Load(sn)
}
func (mkm *multiKeyMap) Store(sn uint64, addr string, value interface{}) {
	mkm.mtx.Lock()
	defer mkm.mtx.Unlock()
	mkm.mapAddrSn[addr] = sn
	mkm.mapSnAddr[sn] = addr
	mkm.mapSnValue.Store(sn, value)
}
func (mkm *multiKeyMap) DeleteWithSn(sn uint64) {
	mkm.mtx.Lock()
	defer mkm.mtx.Unlock()
	addr, ok := mkm.mapSnAddr[sn]
	if !ok {
		return
	}
	delete(mkm.mapSnAddr, sn)
	delete(mkm.mapAddrSn, addr)
	mkm.mapSnValue.Delete(sn)
}
func (mkm *multiKeyMap) Range(rf func(key, value interface{}) bool) {
	mkm.mapSnValue.Range(rf)
}

type proxyListener interface {
	close() error
	getBase() proxyListenerBase
}
type socketWriter interface {
	write([]byte, uint64) (int, error)
	close(uint64)
}

type proxyListenerBase struct {
	RemoteAddr string `json:"remoteAddr"`
	LocalPort  int    `json:"localPort"`
	Protocol   string `json:"protocol"`
}

func (plb *proxyListenerBase) getBase() proxyListenerBase {
	return *plb
}

type proxyTCPListener struct {
	*proxyListenerBase
	listener net.Listener
}

func (ptl *proxyTCPListener) close() error {
	return ptl.listener.Close()
}

type proxyUDPListener struct {
	*proxyListenerBase
	uc *net.UDPConn
}

func (pul *proxyUDPListener) close() error {
	return pul.uc.Close()
}

type tcpWriter struct {
	conn net.Conn
}

func (tw *tcpWriter) write(packet []byte, sn uint64) (int, error) {
	tw.conn.SetWriteDeadline(time.Now().Add(time.Second))
	return tw.conn.Write(packet)
}
func (tw *tcpWriter) close(sn uint64) {
	tw.conn.Close()
}

type udpWriter struct {
	conn *net.UDPConn
	mkm  *multiKeyMap
}

type udpSession struct {
	addr net.Addr
	sn   uint64
}
type udpConnWriter struct {
	conn *net.UDPConn
}

func (ucw *udpConnWriter) write(packet []byte, sn uint64) (int, error) {
	ucw.conn.SetWriteDeadline(time.Now().Add(time.Minute))
	return ucw.conn.Write(packet)
}
func (ucw *udpConnWriter) close(sn uint64) {
	ucw.conn.Close()
}

func (uw *udpWriter) write(packet []byte, sn uint64) (int, error) {
	v, ok := uw.mkm.LoadWithSn(sn)
	if !ok {
		return 0, fmt.Errorf("udp session not found")
	}
	us, ok := v.(*udpSession)
	if !ok {
		return 0, fmt.Errorf("wrong value in udp session list")
	}
	return uw.conn.WriteTo(packet, us.addr)
}
func (uw *udpWriter) close(sn uint64) {
	uw.mkm.DeleteWithSn(sn)
}

type socketProxy struct {
	writeTimeTimestamp int64

	listeners map[int]proxyListener
	connects  map[uint64]socketWriter
	connMtx   sync.RWMutex
	packetCh  chan []byte
	//listenerCh chan proxyListener
	stopped       bool
	stopMtx       sync.Mutex
	activeConnNum int

	lastHeartbeatTime time.Time
}

func newSocketProxy() *socketProxy {
	return &socketProxy{
		listeners: make(map[int]proxyListener),
		connects:  make(map[uint64]socketWriter),
		packetCh:  make(chan []byte, 1),
	}
}
func (sp *socketProxy) updateWriteTimestamp() {
	atomic.StoreInt64(&sp.writeTimeTimestamp, time.Now().Unix())
}
func (sp *socketProxy) writeTimestampDiff() int64 {
	ts := atomic.LoadInt64(&sp.writeTimeTimestamp)
	return time.Now().Unix() - ts
}

//	func (sp *SocketProxy) Run() {
//		for {
//			select {
//			case <-sp.listenerCh:
//			case <-sp.stopCh:
//			case <-:
//
//			}
//		}
//	}
func (sp *socketProxy) handleUDPListener(plb *proxyListenerBase) (int, error) {
	//todo udp
	l, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: plb.LocalPort})
	if err != nil {
		return 0, fmt.Errorf("listen udp failed:%s", err)
	}
	var portRet int
	if plb.LocalPort == 0 {
		parsePort := strings.Split(l.LocalAddr().String(), ":")
		if len(parsePort) != 2 {
			l.Close()
			return 0, fmt.Errorf("wrong listener:%s", l.LocalAddr().String())
		}
		p, err := strconv.Atoi(parsePort[1])
		if err != nil {
			l.Close()
			return 0, fmt.Errorf("parse port failed:%s", err)
		}
		portRet = p
		plb.LocalPort = portRet
	} else {
		portRet = plb.LocalPort
	}
	mkm := &multiKeyMap{}
	mkm.init()
	uw := udpWriter{
		conn: l,
		mkm:  mkm,
	}
	sp.connMtx.Lock()
	sp.listeners[convertIndex(plb.Protocol, plb.LocalPort)] = &proxyUDPListener{plb, l}
	sp.connMtx.Unlock()

	buffer := make([]byte, 64*1024)
	go func() {
		for {
			l.SetReadDeadline(time.Now().Add(time.Second * 10))
			len, connAddr, err := l.ReadFrom(buffer)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					if sp.writeTimestampDiff() < 60 {
						continue
					}
					sp.writeHeartbeat()
					continue
				} else {
					logging.Error("spcketProxy read udp on %d error %s", plb.LocalPort, err)
					break
				}
			} else {
				b := bytes.Buffer{}
				b.Write(buffer[:len])
				sp.handleUDPPacket(plb.RemoteAddr, connAddr, &b, mkm, &uw)
			}
		}
	}()
	return portRet, nil
}
func (sp *socketProxy) handleUDPPacket(remoteAddr string, addr net.Addr, buffer *bytes.Buffer, mkm *multiKeyMap, uw *udpWriter) {
	var us *udpSession
	v, ok := mkm.LoadWithAddr(addr.String())
	if ok {
		us, ok = v.(*udpSession)
	}
	if !ok {
		sn := rand.New(rand.NewSource(time.Now().UnixNano())).Uint64()
		sp.connMtx.Lock()
		sp.connects[sn] = uw
		sp.connMtx.Unlock()
		us = &udpSession{
			addr,
			sn,
		}
		mkm.Store(sn, addr.String(), us)
		logging.Debug("send udp connect request, sn %d, target %s", sn, remoteAddr)
		remoteAddrWithPro := []byte(fmt.Sprintf("%s,udp", remoteAddr))
		dataLen := len(remoteAddrWithPro)
		connectReq := portPacketHead{packetTypeConnect, sn, uint16(dataLen)}
		reqBuf := new(bytes.Buffer)
		binary.Write(reqBuf, binary.LittleEndian, connectReq)
		sendBuf := append(reqBuf.Bytes(), remoteAddrWithPro[:dataLen]...)
		sp.fillChan(sendBuf)
	}
	logging.Debug("send udp data packet from listener, sn %d, target %s", us.sn, remoteAddr)
	packetLen := buffer.Len()
	forwardReq := portPacketHead{packetTypeBinary, us.sn, uint16(packetLen)}
	reqBuf := new(bytes.Buffer)
	binary.Write(reqBuf, binary.LittleEndian, forwardReq)
	sendBuf := append(reqBuf.Bytes(), buffer.Bytes()...)
	sp.fillChan(sendBuf)
}
func (sp *socketProxy) handleTCPListener(plb *proxyListenerBase) (int, error) {
	la, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("0.0.0.0:%d", plb.LocalPort))
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp4", la)
	if err != nil {
		return 0, err
	}
	var portRet int
	if plb.LocalPort == 0 {
		parsePort := strings.Split(l.Addr().String(), ":")
		if len(parsePort) != 2 {
			l.Close()
			return 0, fmt.Errorf("wrong listener:%s", l.Addr().String())
		}
		p, err := strconv.Atoi(parsePort[1])
		if err != nil {
			l.Close()
			return 0, fmt.Errorf("parse port failed:%s", err)
		}
		portRet = p
		plb.LocalPort = portRet
	} else {
		portRet = plb.LocalPort
	}
	sp.connMtx.Lock()
	sp.listeners[convertIndex(plb.Protocol, plb.LocalPort)] = &proxyTCPListener{plb, l}
	sp.connMtx.Unlock()
	go func() {
		for {
			l.SetDeadline(time.Now().Add(time.Second * 10))
			conn, err := l.Accept()
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					if sp.writeTimestampDiff() < 60 {
						continue
					}
					sp.writeHeartbeat()
					continue
				}
				logging.Error("listenr %s:%d error %s", plb.Protocol, plb.LocalPort, err)
				break
			}
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			sp.tcpConnectionRun(conn, r.Uint64(), plb.RemoteAddr)
		}
	}()
	return portRet, nil
}

func (sp *socketProxy) tcpConnectionRun(conn net.Conn, sn uint64, remoteAddr string) {
	connectionRead := func(conn net.Conn, sn uint64, remoteAddr string) {
		if remoteAddr != "" {
			logging.Debug("send tcp connect request, sn %d, target %s", sn, remoteAddr)
			remoteAddrWithPro := []byte(fmt.Sprintf("%s,tcp", remoteAddr))
			dataLen := len(remoteAddrWithPro)
			connectReq := portPacketHead{packetTypeConnect, sn, uint16(dataLen)}
			reqBuf := new(bytes.Buffer)
			binary.Write(reqBuf, binary.LittleEndian, connectReq)
			sendBuf := append(reqBuf.Bytes(), remoteAddrWithPro[:dataLen]...)
			sp.fillChan(sendBuf)
		}
		buffer := make([]byte, readBuffLen)
		for !sp.stopped {
			// conn.SetReadDeadline(time.Now().Add(time.Second * 5))
			dataLen, err := conn.Read(buffer)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				// gLogger.Printf(LevelDEBUG, "connection %d read error:%s,close it", sn, err)
				if nil == conn.Close() {
					logging.Debug("send tcp disconnect request, sn %d", sn)
					disconnectReq := portPacketHead{packetTypeDisconnect, sn, 0}
					reqBuf := new(bytes.Buffer)
					binary.Write(reqBuf, binary.LittleEndian, disconnectReq)
					sp.fillChan(reqBuf.Bytes())
				}
				break
			} else {
				//tcm.ts.bandwidthLimit(len)
				logging.Debug("send tcp data packet, sn %d, len %d", sn, dataLen)
				forwardReq := portPacketHead{packetTypeBinary, sn, uint16(dataLen)}
				reqBuf := new(bytes.Buffer)
				binary.Write(reqBuf, binary.LittleEndian, forwardReq)
				sendBuf := append(reqBuf.Bytes(), buffer[:dataLen]...)
				sp.fillChan(sendBuf)
			}
		}
		if sp.stopped {
			conn.Close()
		}
	}

	sp.connMtx.Lock()
	sp.connects[sn] = &tcpWriter{conn}
	sp.connMtx.Unlock()

	go func() {
		logging.Debug("tcp connection begin sn %d, from %s", sn, conn.RemoteAddr().String())
		defer logging.Debug("tcp connection end, sn %d", sn)
		connectionRead(conn, sn, remoteAddr)

		// gLogger.Printf(LevelDEBUG, "connection %d close", sn)
		sp.connMtx.Lock()
		delete(sp.connects, sn)
		sp.connMtx.Unlock()

	}()
}
func (sp *socketProxy) udpConnectionRun(conn *net.UDPConn, sn uint64) {
	connectionRead := func(conn *net.UDPConn, sn uint64) {
		buffer := make([]byte, 64*1024)
		for !sp.stopped {
			conn.SetReadDeadline(time.Now().Add(time.Minute))
			dataLen, err := conn.Read(buffer)
			if err != nil {
				// gLogger.Printf(LevelDEBUG, "connection %d read error:%s,close it", sn, err)
				if nil == conn.Close() {
					logging.Debug("send udp disconnect request, sn %d", sn)
					disconnectReq := portPacketHead{packetTypeDisconnect, sn, 0}
					reqBuf := new(bytes.Buffer)
					binary.Write(reqBuf, binary.LittleEndian, disconnectReq)
					sp.fillChan(reqBuf.Bytes())
				}
				break
			}

			logging.Debug("send udp data packet, sn %d, len %d", sn, dataLen)
			forwardReq := portPacketHead{packetTypeBinary, sn, uint16(dataLen)}
			reqBuf := new(bytes.Buffer)
			binary.Write(reqBuf, binary.LittleEndian, forwardReq)
			sendBuf := append(reqBuf.Bytes(), buffer[:dataLen]...)
			sp.fillChan(sendBuf)
		}
		if sp.stopped {
			conn.Close()
		}
	}

	sp.connMtx.Lock()
	sp.connects[sn] = &udpConnWriter{conn}
	sp.connMtx.Unlock()

	go func() {
		logging.Debug("udp connection begin sn %d, from %s", sn, conn.RemoteAddr().String())
		defer logging.Debug("udp connection end, sn %d", sn)
		connectionRead(conn, sn)

		// gLogger.Printf(LevelDEBUG, "connection %d close", sn)
		sp.connMtx.Lock()
		delete(sp.connects, sn)
		sp.connMtx.Unlock()

	}()
}
func (sp *socketProxy) AddListener(remoteAddr string, protocol string, localPort int) (int, error) {
	sp.connMtx.RLock()
	l, ok := sp.listeners[convertIndex(protocol, localPort)]
	sp.connMtx.RUnlock()
	if ok {
		p := l.getBase()
		if p.RemoteAddr == remoteAddr {
			return p.LocalPort, nil
		}
		logging.Info("delete expired listenr on %s:%d", protocol, localPort)
		sp.DeleteListener(protocol, localPort)
	}
	plb := &proxyListenerBase{
		RemoteAddr: remoteAddr,
		LocalPort:  localPort,
		Protocol:   protocol,
	}
	logging.Info("add listenr on %s:%d", protocol, localPort)
	defer sp.writeHeartbeat()
	if protocol == "tcp" {
		return sp.handleTCPListener(plb)
	}
	return sp.handleUDPListener(plb)
}
func convertIndex(protocol string, localPort int) int {
	index := localPort
	if protocol == "udp" {
		index += 100000
	}
	return index
}
func (sp *socketProxy) ListenerExist(protocol string, localPort int) bool {
	sp.connMtx.RLock()
	l, ok := sp.listeners[convertIndex(protocol, localPort)]
	sp.connMtx.RUnlock()
	if ok {
		if p := l.getBase(); p.LocalPort != localPort {
			ok = false
		}
	}
	return ok
}
func (sp *socketProxy) GetAllListener() []proxyListenerBase {
	sp.connMtx.RLock()
	defer sp.connMtx.RUnlock()
	rsp := make([]proxyListenerBase, 0)
	for _, l := range sp.listeners {
		rsp = append(rsp, l.getBase())
	}
	return rsp
}
func (sp *socketProxy) WritePacket(data []byte) error {
	packetLen := len(data)
	head := portPacketHead{}
	bufHead := bytes.NewReader(data[:portPacketHeadSize])
	err := binary.Read(bufHead, binary.LittleEndian, &head)
	if err != nil {
		return fmt.Errorf("read header of content from ws failed:%s", err)
	}
	if packetLen != portPacketHeadSize+int(head.BodyLen) {
		return fmt.Errorf("content error:body len check failed")
	}
	if head.PacketType == packetTypeConnect {
		//dial
		targetAddrParam := strings.Split(string(data[portPacketHeadSize:]), ",")
		if len(targetAddrParam) != 2 {
			return fmt.Errorf("wrong data in connect packet")
		}
		logging.Debug("recv connect request packet, protocol %s, sn %d, target %s", targetAddrParam[1], head.ConnSN, targetAddrParam[0])
		if targetAddrParam[1] == "tcp" {
			conn, err := net.DialTimeout("tcp", targetAddrParam[0], time.Second*5)
			if err != nil {
				return fmt.Errorf("tcp dial target adderss failed:%s", err)
			}
			sp.tcpConnectionRun(conn, head.ConnSN, "")
		} else {
			//todo udp
			ua, err := net.ResolveUDPAddr("udp", targetAddrParam[0])
			if err != nil {
				return fmt.Errorf("resolve udp address failed:%s", err)
			}
			conn, err := net.DialUDP("udp", nil, ua)
			if err != nil {
				return fmt.Errorf("udp dial target adderss failed:%s", err)
			}
			sp.udpConnectionRun(conn, head.ConnSN)
		}
		return nil
	} else if head.PacketType == packetTypeDisconnect {
		logging.Debug("socketProxy recv disconnect %d", head.ConnSN)
		sp.connMtx.Lock()
		if w, ok := sp.connects[head.ConnSN]; ok {
			w.close(head.ConnSN)
			if _, ok := w.(*udpWriter); ok {
				delete(sp.connects, head.ConnSN)
			}
		}
		sp.connMtx.Unlock()
		return nil
	} else if head.PacketType == packetTypeHeartbeat {
		sp.connMtx.Lock()
		sp.lastHeartbeatTime = time.Now()
		sp.connMtx.Unlock()
		logging.Debug("socketProxy recv heartbeat")
		ack := portPacketHead{packetTypeHeartbeatACK, 0, 0}
		reqBuf := new(bytes.Buffer)
		binary.Write(reqBuf, binary.LittleEndian, ack)
		sp.fillChan(reqBuf.Bytes())
		return nil
	} else if head.PacketType == packetTypeHeartbeatACK {
		logging.Debug("socketProxy recv heartbeat ack")
		return nil
	}
	logging.Debug("socketProxy recv connect data, sn ", head.ConnSN)
	err = sp.send(head.PacketType, head.ConnSN, data[portPacketHeadSize:])
	if err != nil {
		logging.Debug("socketProxy send target fail %s, send disconnect %d", err, head.ConnSN)
		disconnectReq := portPacketHead{packetTypeDisconnect, head.ConnSN, 0}
		reqBuf := new(bytes.Buffer)
		binary.Write(reqBuf, binary.LittleEndian, disconnectReq)
		sp.fillChan(reqBuf.Bytes())
	}
	return err
}
func (sp *socketProxy) GetPacketChan() chan []byte {
	return sp.packetCh
}
func (sp *socketProxy) send(packetType uint8, sn uint64, data []byte) error {
	sp.connMtx.RLock()
	defer sp.connMtx.RUnlock()
	if w, ok := sp.connects[sn]; ok {
		writeLen := 0
		for !sp.stopped {
			l, err := w.write(data[writeLen:], sn)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					writeLen += l
					continue
				}
			}
			return err
		}
	}
	return fmt.Errorf("connection %d not found", sn)
}
func (sp *socketProxy) DeleteListener(protocol string, localPort int) string {
	sp.stopMtx.Lock()
	defer sp.stopMtx.Unlock()
	for k, l := range sp.listeners {
		if l == nil {
			continue
		}
		base := l.getBase()
		if localPort == base.LocalPort && protocol == base.Protocol {
			l.close()
			delete(sp.listeners, k)
			return base.RemoteAddr
		}
	}
	return ""
}
func (sp *socketProxy) Close() {
	sp.stopMtx.Lock()
	defer sp.stopMtx.Unlock()
	sp.activeConnNum = sp.getConnCount()
	sp.stopped = true
	select {
	case <-sp.packetCh:
	case <-time.After(time.Millisecond * 10):
	}
	close(sp.packetCh)
	for _, l := range sp.listeners {
		if l != nil {
			l.close()
		}
	}
}
func (sp *socketProxy) fillChan(p []byte) error {
	sp.stopMtx.Lock()
	defer sp.stopMtx.Unlock()
	if sp.stopped {
		return fmt.Errorf("closed socket proxy")
	}
	sp.updateWriteTimestamp()
	sp.packetCh <- p
	return nil
}

func (sp *socketProxy) getConnCount() int {
	sp.connMtx.RLock()
	defer sp.connMtx.RUnlock()
	return len(sp.connects)
}

func (sp *socketProxy) isActive() bool {
	sp.connMtx.RLock()
	defer sp.connMtx.RUnlock()
	connNum := len(sp.connects)
	return connNum > 0 || time.Since(sp.lastHeartbeatTime) < time.Minute*2
}

func (sp *socketProxy) writeHeartbeat() {
	logging.Debug("socketProxy write heartbeat")
	heartbeat := portPacketHead{packetTypeHeartbeat, 0, 0}
	reqBuf := new(bytes.Buffer)
	binary.Write(reqBuf, binary.LittleEndian, heartbeat)
	sp.fillChan(reqBuf.Bytes())
}
