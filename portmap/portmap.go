package portmap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/isletnet/uptp/logging"
	// "github.com/isletnet/uptp/stream"
	"github.com/lesismal/nbio"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	portmapID = "/portmap/1.0.0"
)

type relayListener interface {
	close() error
	getPort() int
	// getConf() *PortmapConf
}

type handshakeRsp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type relayTCPListener struct {
	// *PortmapConf
	listener net.Listener
}

func (ptl *relayTCPListener) close() error {
	return ptl.listener.Close()
}

func (ptl *relayTCPListener) getPort() int {
	parsePort := strings.Split(ptl.listener.Addr().String(), ":")
	if len(parsePort) != 2 {
		return 0
	}
	p, _ := strconv.Atoi(parsePort[1])
	return p
}

type relayUDPListener struct {
	// *PortmapConf
	ul *net.UDPConn
	uc *nbio.Conn
}

func (pul *relayUDPListener) close() error {
	return pul.uc.Close()
}
func (pul *relayUDPListener) getPort() int {
	parsePort := strings.Split(pul.ul.LocalAddr().String(), ":")
	if len(parsePort) != 2 {
		return 0
	}
	p, _ := strconv.Atoi(parsePort[1])
	return p
}

type GetHandshake func(network string, ip string, port int) (peerID string, handshake []byte)
type HandleHandshake func(handshake []byte) (network string, addr string, port int, err error)

type Portmap struct {
	listeners map[string]relayListener
	connMtx   sync.RWMutex

	connEngine *nbio.Gopher

	p2pEngine host.Host

	funcGetHandshake    GetHandshake
	funcHandleHandshake HandleHandshake
}

func NewPortMap(h host.Host) *Portmap {
	var ret Portmap
	ret.p2pEngine = h
	ret.listeners = make(map[string]relayListener)
	g := nbio.NewGopher(nbio.Config{
		Network:        "tcp",
		UDPReadTimeout: time.Minute,
	})
	g.OnOpen(ret.onConn)
	g.OnData(ret.onData)
	g.OnClose(ret.onClose)
	ret.connEngine = g

	return &ret
}

func (pm *Portmap) SetGetHandshakeFunc(f GetHandshake) {
	pm.funcGetHandshake = f
}

func (pm *Portmap) SetHandleHandshakeFunc(f HandleHandshake) {
	pm.funcHandleHandshake = f
}

func (pm *Portmap) Start(server bool) {
	// nblog.SetLogger(nil)
	pm.connEngine.Start()
	if server {
		pm.p2pEngine.SetStreamHandler(portmapID, pm.handleUptpStream)
	}
}

func (pm *Portmap) AddListener(network string, ip string, port int) (int, error) {
	pm.connMtx.RLock()
	defer pm.connMtx.RUnlock()
	l, ok := pm.listeners[convertIndex(network, ip, port)]
	if !ok {
		var err error
		if strings.Contains(network, "tcp") {
			l, err = pm.addTCPListener(ip, port)
		} else {
			l, err = pm.addUDPListener(ip, port)
		}
		if err != nil {
			return 0, err
		}
	}
	retPort := l.getPort()
	pm.listeners[convertIndex(network, ip, retPort)] = l

	return retPort, nil
}

func (pm *Portmap) addTCPListener(ip string, port int) (relayListener, error) {
	l, err := net.Listen("tcp4", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return nil, err
	}
	ret := &relayTCPListener{l}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				//todo log
				break
			}
			pm.connEngine.AddConn(conn)
		}
	}()
	return ret, nil
}

func (pm *Portmap) addUDPListener(ip string, port int) (relayListener, error) {
	la, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", ip, port))
	if err != nil {
		return nil, err
	}
	l, err := net.ListenUDP("udp4", la)
	if err != nil {
		return nil, err
	}
	nc, err := pm.connEngine.AddConn(l)
	if err != nil {
		l.Close()
		return nil, err
	}
	return &relayUDPListener{l, nc}, nil
}

func (pm *Portmap) DeleteListener(network string, ip string, port int) {
	pm.connMtx.Lock()
	defer pm.connMtx.Unlock()
	index := convertIndex(network, ip, port)
	l, ok := pm.listeners[index]
	if ok {
		l.close()
		delete(pm.listeners, index)
	}
}

func (pm *Portmap) relayHandshake(peerID string, hs []byte) (s network.Stream, err error) {
	// var s network.Stream
	defer func() {
		if s != nil && err != nil {
			s.Reset()
		}
	}()
	pid, err := peer.Decode(peerID)
	if err != nil {
		logging.Error("[Portmap:relayHandshake] decode peer id error: %s", err)
		return nil, err
	}
	s, err = pm.p2pEngine.NewStream(context.Background(), pid, portmapID)
	if err != nil {
		logging.Error("[Portmap:relayHandshake] create stream error: %s", err)
		return nil, err
	}
	// ps = stream.NewVarLenPacketStream(s, 32*1024)
	_, err = s.Write(hs)
	if err != nil {
		logging.Error("[Portmap:relayHandshake] write connection handshake error: %s", err)
		return nil, err
	}
	hsBuf := make([]byte, 100)
	rspLen, err := s.Read(hsBuf)
	if err != nil {
		logging.Error("[Portmap:relayHandshake] read connection handshake rsp error: %s", err)
		return nil, err
	}
	hsRsp := handshakeRsp{}
	err = json.Unmarshal(hsBuf[:rspLen], &hsRsp)
	if err != nil {
		logging.Error("[Portmap:relayHandshake] unmarshal connection handshake rsp error: %s", err)
		return
	}
	if hsRsp.Code != 0 {
		logging.Error("[Portmap:relayHandshake]  handshake rsp error: %s", hsRsp.Msg)
		err = fmt.Errorf("handshak response: %s", hsRsp.Msg)
	}
	return
}

func (pm *Portmap) onData(c *nbio.Conn, data []byte) {
	s := c.Session()
	if s == nil {
		c.Close()
		return
	}
	stream, ok := s.(network.Stream)
	if !ok {
		c.SetSession(nil)
		c.Close()
		return
	}
	_, err := stream.Write(data)
	if err != nil {
		c.SetSession(nil)
		c.Close()
		return
	}
}
func (pm *Portmap) onClose(c *nbio.Conn, err error) {
	s := c.Session()
	if s == nil {
		return
	}
	stream, ok := s.(network.Stream)
	if !ok {
		return
	}
	stream.Close()
}

func (pm *Portmap) onConn(c *nbio.Conn) {
	// logging.Info("onOpen: [%p, %v]", c, c.RemoteAddr().String())
	s := c.Session()
	if s == nil {
		prot := c.LocalAddr().Network()
		if len(prot) < 3 {
			c.Close()
		}
		prot = prot[:3]
		var port int
		var ip string
		switch prot {
		case "tcp":
			ta := c.LocalAddr().(*net.TCPAddr)
			port = ta.Port
			ip = ta.IP.String()
		case "udp":
			ua := c.LocalAddr().(*net.UDPAddr)
			port = ua.Port
			ip = ua.IP.String()
		default:
			c.Close()
			return
		}
		pid, hs := pm.funcGetHandshake(prot, ip, port)
		if hs == nil || pid == "" {
			c.Close()
			return
		}
		s, err := pm.relayHandshake(pid, hs)
		if err != nil {
			c.Close()
			return
		}
		c.SetSession(s)
		go func() {
			_, err = io.Copy(c, s)
			logging.Error("[Portmap:onConn] forward stram to connection error: %s", err)
			c.SetSession(nil)
			s.Close()
			c.Close()
		}()
		return
	}
	_, ok := s.(network.Stream)
	if !ok {
		c.Close()
		return
	}
}

func (pm *Portmap) handleUptpStream(s network.Stream) {
	go func(s network.Stream) {
		// ps := stream.NewVarLenPacketStream(s, 32*1024)
		errMsg := ""
		defer func() {
			if errMsg != "" {
				if rspBuf, err := json.Marshal(handshakeRsp{
					Code: 1,
					Msg:  errMsg,
				}); err == nil {
					_, _ = s.Write(rspBuf)
				}
				s.Close()
			}
		}()

		hsbuf := make([]byte, 1024)
		n, err := s.Read(hsbuf)
		if err != nil {
			s.Close()
			logging.Error("[Portmap:handleUptpStream] read connection handshake error: %s", err)
			return
		}
		network, addr, port, err := pm.funcHandleHandshake(hsbuf[:n])
		if err != nil {
			errMsg = err.Error()
			logging.Error("[Portmap:handleUptpStream] handle handshake error: %s", err)
			return
		}
		var conn net.Conn
		if network == "tcp" {
			conn, err = net.DialTimeout("tcp", fmt.Sprintf("%s:%d", addr, port), time.Second*5)
			if err != nil {
				errMsg = "connect target addr failed"
				logging.Error("[Portmap:handleUptpStream] dial tcp connection error: %s", err)
				return
			}
		} else {
			ua, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", addr, port))
			if err != nil {
				errMsg = "resolve target addr failed"
				logging.Error("[Portmap:handleUptpStream] resolve target udp addr error: %s", err)
				return
			}
			conn, err = net.DialUDP("udp", nil, ua)
			if err != nil {
				errMsg = "connect target addr failed"
				logging.Error("[Portmap:handleUptpStream] dial udp connection error: %s", err)
				return
			}
		}

		nc, err := nbio.NBConn(conn)
		if err != nil {
			conn.Close()
			errMsg = "unexpected connection failed"
			return
		}
		nc.SetSession(s)
		rsp := handshakeRsp{
			Msg: "ok",
		}
		rspBuf, err := json.Marshal(rsp)
		if err != nil {
			conn.Close()
			errMsg = "marshal rsp failed"
			return
		}
		_, err = s.Write(rspBuf)
		if err != nil {
			s.Close()
			nc.Close()
			logging.Error("[Portmap:handleUptpStream] write connection handshake error: %s", err)
			return
		}
		_, _ = pm.connEngine.AddConn(nc)

		_, err = io.Copy(nc, s)
		logging.Error("[Portmap:handleUptpStream] forward stram to connection error: %s", err)
		nc.SetSession(nil)
		s.Close()
		nc.Close()
	}(s)
}

func convertIndex(network string, ip string, port int) string {
	return fmt.Sprintf("%s://%s:%d", network, ip, port)
}
