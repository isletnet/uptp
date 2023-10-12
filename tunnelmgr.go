package uptp

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/antlabs/timer"
	"github.com/isletnet/uptp/logging"
	"github.com/lesismal/nbio"
	reuseport "github.com/libp2p/go-reuseport"
)

type tunnelMgr struct {
	uc        *Uptpc
	mtx       sync.RWMutex
	tunnelMap map[uint64]uptpTunnel
	myID      uint64

	tcpEngine *nbio.Gopher
	udpEngine *nbio.Gopher

	heartbeatTm  timer.Timer
	timerNodeMap map[uint64]timer.TimeNoder

	addJudger *tunnelAddJudger
	// reqCB func(uint64, string) error
	// toCB  func(uint64)
	// addrCB  func(uint64, string) (uptpConn, uint64, error)
	// network string
}

func newTunnelMgr(uc *Uptpc) *tunnelMgr {
	ret := &tunnelMgr{
		uc:           uc,
		tunnelMap:    make(map[uint64]uptpTunnel),
		myID:         hashStringToInt(uc.opt.Name),
		heartbeatTm:  timer.NewTimer(),
		timerNodeMap: make(map[uint64]timer.TimeNoder),
		addJudger:    newTunnelAddJudger(),
	}
	g := nbio.NewGopher(nbio.Config{
		Network: "tcp",
		Addrs:   []string{":" + strconv.Itoa(uc.opt.PublicPort)},
		// ReadBufferSize:     5300,
		// MaxWriteBufferSize: 5300,
	})
	g.OnClose(wrapOnCloseRawTCPConn(ret.onUptpConnClose))
	g.OnData(wrapOnDataRawTCPConn(ret.handleUptpData, nil))
	ret.tcpEngine = g

	ug := nbio.NewGopher(nbio.Config{
		UDPReadTimeout: time.Second * 30,
	})
	ug.OnClose(wrapOnCloseRawUDPConn(ret.onUptpConnClose))
	ug.OnData(wrapOnDataRawUDPConn(ret.handleUptpData, nil))
	ret.udpEngine = ug
	ret.uc.dispatch.registerHandler(msgIDTypeAddTunnelReq, ret.handleAddTunnelReq)
	return ret
}

func (tm *tunnelMgr) start() error {
	err := tm.tcpEngine.Start()
	if err != nil {
		return err
	}
	tm.udpEngine.Start()
	go tm.heartbeatTm.Run()
	return nil
}

func (tm *tunnelMgr) onUptpConnClose(c uptpTunnel, err error) {
	logging.Info("[uptp:tunnelMgr] tcp connect close: %s", err)
	tm.mtx.Lock()
	delete(tm.tunnelMap, c.GetPeerID())
	noder, ok := tm.timerNodeMap[c.GetPeerID()]
	delete(tm.timerNodeMap, c.GetPeerID())
	tm.mtx.Unlock()
	if ok {
		noder.Stop()
	}
}

func (tm *tunnelMgr) handleUptpData(c uptpTunnel, head *uptpPacketHead, data []byte) {
	logging.Debug("[uptp:tunnelMgr] recv data, head %+v from %+v", head, c)
	if head.To == 0 {
		//tunnel ctrl msg
		tm.handleTunnelCtrl(c, data)
		return
	}
	if head.To != tm.myID {
		//forward
		tm.mtx.RLock()
		targetConn, ok := tm.tunnelMap[head.To]
		tm.mtx.RUnlock()
		if ok {
			targetConn.SendPacket(head.From, head.To, data)
		}
		//todo send rsp when not found
		return
	}
	tm.uc.handleUptpPacket(head, data)
}

func (tm *tunnelMgr) handleTunnelCtrl(c uptpTunnel, data []byte) {
	if len(data) < 4 {
		return
	}
	ctrlType := binary.LittleEndian.Uint32(data[:4])
	if ctrlType == tunnelCtrlHeartbeat {
		//return empty
		logging.Debug("[uptp:tunnelMgr] recv tunnel heartbeat")
		c.SendPacket(0, 0, nil)
	}
}

func (tm *tunnelMgr) getTunnel(peerID uint64) (uptpTunnel, error) {
	tm.mtx.RLock()
	conn, ok := tm.tunnelMap[peerID]
	tm.mtx.RUnlock()
	if ok {
		return conn, nil
	}

	addTime := time.Now().UnixNano()
	if !(tm.addJudger.checkAndAdd(tm.myID+peerID, addTime, false)) {
		return nil, ErrAddTunnelBusy
	}
	defer tm.addJudger.delete(tm.myID + peerID)
	logging.Info("[uptp:tunnelMgr] try to add tunnel to %d", peerID)
	peerInfo, err := tm.uc.requestPeerInfo(peerID)
	//todo use peer info
	logging.Debug("[uptp:tunnelMgr] peer info: %+v", peerInfo)
	if err != nil {
		return nil, fmt.Errorf("request peer info error %s", err)
	}
	conn, err = tm.tryAddTunnelPunchTCP(peerID, addTime)
	if err != nil {
		return nil, fmt.Errorf("try tcp punch error %s", err)
	}

	noder := tm.heartbeatTm.ScheduleFunc(time.Second*30, func() {
		logging.Debug("[uptp:tunnelMgr] send tunnel heartbeat, peerID %d", peerID)
		bytes := binary.LittleEndian.AppendUint32(nil, tunnelCtrlHeartbeat)
		conn.SendPacket(0, 0, bytes)
	})
	tm.mtx.Lock()
	tm.tunnelMap[peerID] = conn
	tm.timerNodeMap[peerID] = noder
	tm.mtx.Unlock()
	return conn, nil
}

func (tm *tunnelMgr) tryAddTunnelPublicIP() {}

func (tm *tunnelMgr) tryAddTunnelPublicIPPassive() {}

func (tm *tunnelMgr) tryAddTunnelPunchTCP(peerID uint64, t int64) (uptpTunnel, error) {
	pip, pp, lp, err := natTestTcp(tm.uc.opt.NatTestHost, tm.uc.opt.NatTestPort1, 0)
	if err != nil {
		return nil, fmt.Errorf("tcp nat test error: %s", err)
	}
	config := addTunnelConfig{
		PublicIP: pip,
		TCPPort:  pp,
	}
	to := fmt.Sprintf("uptpc/%d", peerID)
	reply := fmt.Sprintf("uptpc/%d", tm.myID)
	t0 := time.Now().UnixNano()
	rspMsg, err := tm.uc.sendAndWait(to, msgIDTypeAddTunnelReq, addTunnelRequest{
		uptpcBaseInfo: tm.uc.getUptpcBaseInfo(),
		Time:          t,
		Config:        config,
		ReplyTo:       reply,
	}, addTunnelRspWaitTimeout)
	if err != nil {
		return nil, fmt.Errorf("send add tunnel req error: %s", err)
	}
	t4 := time.Now().UnixNano()

	rsp := addTunnelResponse{}
	err = json.Unmarshal(rspMsg.Content, &rsp)
	if err != nil {
		return nil, fmt.Errorf("unmarshal add tunnel request error: %s", err)
	}
	if rsp.Errno != 0 {
		return nil, fmt.Errorf("add tunnel response error: %d", rsp.Errno)
	}
	ttl := t4 - t0 - rsp.T2 + rsp.T1
	syncTime := rsp.SyncTime - time.Duration(ttl/2)
	conn, err := tm.punchTCP(peerID, tm.uc.globalInfo.getLocalIP(), lp, rsp.Config.PublicIP, rsp.Config.TCPPort, syncTime)
	if err != nil {
		return nil, fmt.Errorf("punch tcp error: %s", err)
	}
	return conn, nil
}

func (tm *tunnelMgr) tryAddTunnelPunchUDP(peerID uint64) (uptpTunnel, error) {
	pip, pp, lp, err := natTest(tm.uc.opt.NatTestHost, tm.uc.opt.NatTestPort1, 0)
	if err != nil {
		return nil, fmt.Errorf("tcp nat test error: %s", err)
	}
	config := addTunnelConfig{
		PublicIP: pip,
		TCPPort:  pp,
	}
	to := fmt.Sprintf("uptpc/%d", peerID)
	reply := fmt.Sprintf("uptpc/%d", tm.myID)
	t0 := time.Now().UnixNano()
	rspMsg, err := tm.uc.sendAndWait(to, msgIDTypeAddTunnelReq, addTunnelRequest{
		uptpcBaseInfo: tm.uc.getUptpcBaseInfo(),
		Config:        config,
		ReplyTo:       reply,
	}, addTunnelRspWaitTimeout)
	if err != nil {
		return nil, fmt.Errorf("send add tunnel req error: %s", err)
	}
	t4 := time.Now().UnixNano()

	rsp := addTunnelResponse{}
	err = json.Unmarshal(rspMsg.Content, &rsp)
	if err != nil {
		return nil, fmt.Errorf("unmarshal add tunnel request error: %s", err)
	}
	ttl := t4 - t0 - rsp.T2 + rsp.T1
	syncTime := rsp.SyncTime - time.Duration(ttl/2)
	conn, err := tm.punchUDP(peerID, tm.uc.globalInfo.getLocalIP(), lp, rsp.Config.PublicIP, rsp.Config.TCPPort, syncTime)
	if err != nil {
		return nil, fmt.Errorf("punch udp error: %s", err)
	}
	return conn, nil
}

func (tm *tunnelMgr) handleAddTunnelReq(msg *uptpMsg) {
	req := addTunnelRequest{}
	err := json.Unmarshal(msg.Content, &req)
	if err != nil {
		logging.Error("[uptp:tunnelMgr] marshal add tunnel request error: %s", err)
		return
	}
	logging.Info("[uptp:tunnelMgr] recv add tunnel request %+v", req)
	if !tm.addJudger.checkAndAdd(tm.myID+req.ID, req.Time, true) {
		logging.Info("[uptp:tunnelMgr] add tunnel busy")
		rsp := addTunnelResponse{
			Errno: 10,
		}
		err := tm.uc.send(req.ReplyTo, msgIDTypeAddTunnelRsp, msg.CorrelationID, rsp)
		if err != nil {
			logging.Error("[uptp:tunnelMgr] send add tunnel response error: %s", err)
		}
		return
	}
	defer tm.addJudger.delete(tm.myID + req.ID)
	tm.handleAddTunnelPunchTCP(&req, msg.CorrelationID)
}

func (tm *tunnelMgr) handleAddTunnelPublicIP()        {}
func (tm *tunnelMgr) handleAddTunnelPublicIPPassive() {}
func (tm *tunnelMgr) handleAddTunnelPunchTCP(req *addTunnelRequest, corrID uint64) {
	logging.Info("[uptp:tunnelMgr] handle punch tcp corrid %d, req %+v", corrID, req)
	var errno int
	var publicIP string
	var localPort, publicPort int
	t1 := time.Now().UnixNano()
	for i := 0; i < 1; i++ {
		pip, pp, lp, err := natTestTcp(tm.uc.opt.NatTestHost, tm.uc.opt.NatTestPort1, 0)
		if err != nil {
			logging.Error("[uptp:tunnelMgr] tcp nat test error: %s", err)
			errno = 2
			break
		}
		publicIP = pip
		publicPort = pp
		localPort = lp
		break
	}
	rsp := addTunnelResponse{
		Errno:    errno,
		T1:       t1,
		T2:       time.Now().UnixNano(),
		SyncTime: punchHoleSyncTime,
		Config: addTunnelConfig{
			PublicIP: publicIP,
			TCPPort:  publicPort,
		},
	}
	err := tm.uc.send(req.ReplyTo, msgIDTypeAddTunnelRsp, corrID, rsp)
	if err != nil {
		logging.Error("[uptp:tunnelMgr] send add tunnel response error: %s", err)
		return
	}
	go func() {
		conn, err := tm.punchTCP(req.ID, tm.uc.globalInfo.getLocalIP(), localPort, req.Config.PublicIP, req.Config.TCPPort, punchHoleSyncTime)
		if err != nil {
			logging.Error("[uptp:tunnelMgr] punch tcp error: %s", err)
			return
		}
		tm.mtx.Lock()
		tm.tunnelMap[req.ID] = conn
		tm.mtx.Unlock()
	}()
}
func (tm *tunnelMgr) handleAddTunnelPunchUDP(req *addTunnelRequest, corrID uint64) {
	var errno int
	var publicIP string
	var localPort, publicPort int
	t1 := time.Now().UnixNano()
	for i := 0; i < 1; i++ {
		pip, pp, lp, err := natTest(tm.uc.opt.NatTestHost, tm.uc.opt.NatTestPort1, 0)
		if err != nil {
			logging.Error("[tunnelMgr:handleAddTunnelReq] tcp nat test error: %s", err)
			errno = 2
			break
		}
		publicIP = pip
		publicPort = pp
		localPort = lp
		break
	}
	rsp := addTunnelResponse{
		Errno:    errno,
		T1:       t1,
		T2:       time.Now().UnixNano(),
		SyncTime: punchHoleSyncTime,
		Config: addTunnelConfig{
			PublicIP: publicIP,
			TCPPort:  publicPort,
		},
	}
	err := tm.uc.send(req.ReplyTo, msgIDTypeAddTunnelRsp, corrID, rsp)
	if err != nil {
		logging.Error("[tunnelMgr:handleTunnelReq] send response error: %s", err)
		return
	}
	go func() {
		conn, err := tm.punchUDP(req.ID, tm.uc.globalInfo.getLocalIP(), localPort, req.Config.PublicIP, req.Config.TCPPort, punchHoleSyncTime)
		if err != nil {
			logging.Error("[uptp:tunnelMgr] punch udp error %s", err)
		}
		tm.mtx.Lock()
		tm.tunnelMap[req.ID] = conn
		tm.mtx.Unlock()
	}()
}

func (tm *tunnelMgr) punchTCP(id uint64, localIP string, localPort int, peerHost string, peerPort int, syncTime time.Duration) (uptpTunnel, error) {
	logging.Debug("[uptp:tunnelMgr] punch tcp sync time:%s", syncTime)
	time.Sleep(syncTime)
	c, err := reuseport.DialTimeout("tcp", fmt.Sprintf("%s:%d", localIP, localPort), fmt.Sprintf("%s:%d", peerHost, peerPort), tcpPunchHoleTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial peer error: %s", err)
	}
	nc, err := tm.tcpEngine.AddConn(c)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("unknown add connect error %s", err)
	}
	nc.SetKeepAlivePeriod(time.Second * 5)
	nc.SetKeepAlive(true)
	logging.Info("[uptp:tunnelMgr] punch tcp ok to %d", id)
	uptpConn := newRawTCPSession(nc, id)
	nc.SetSession(uptpConn)
	return uptpConn, nil
}

type udpPunchHead struct {
	MsgType uint16
	Len     uint16
}

const (
	udpPunchTypeHandshake    = 1
	udpPunchTypeHandshakeACK = 2
)

var udpPunchHeadSize = binary.Size(udpPunchHead{})

func udpPunchWrite(conn *net.UDPConn, dst net.Addr, msgType uint16, content []byte) error {
	head := udpPunchHead{
		MsgType: msgType,
		Len:     uint16(len(content)),
	}
	headBuf := new(bytes.Buffer)
	err := binary.Write(headBuf, binary.LittleEndian, head)
	if err != nil {
		return err
	}
	writeBytes := append(headBuf.Bytes(), content...)
	if dst == nil {
		_, err = conn.Write(writeBytes)
	} else {
		_, err = conn.WriteTo(writeBytes, dst)
	}
	return err
}

func udpPunchRead(conn *net.UDPConn, timeout time.Duration) (*udpPunchHead, []byte, error) {
	if timeout > 0 {
		err := conn.SetReadDeadline(time.Now().Add(timeout))
		if err != nil {
			return nil, nil, err
		}
	}

	result := make([]byte, 1024)
	len, err := conn.Read(result)
	if err != nil {
		return nil, nil, err
	}
	if len < udpPunchHeadSize {
		return nil, nil, errors.New("wrong punch packet")
	}
	head := &udpPunchHead{}
	err = binary.Read(bytes.NewReader(result[:udpPunchHeadSize]), binary.LittleEndian, head)
	if err != nil {
		return nil, nil, err
	}
	return head, result[udpPunchHeadSize:len], nil
}

func (tm *tunnelMgr) punchUDP(id uint64, localIP string, localPort int, peerHost string, peerPort int, syncTime time.Duration) (uptpTunnel, error) {
	logging.Debug("[uptp:tunnelMgr] punch udp sync time:%s", syncTime)
	time.Sleep(syncTime)
	la, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", localIP, localPort))
	if err != nil {
		return nil, fmt.Errorf("resolve local address error: %s", err)
	}
	ra, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", peerHost, peerPort))
	if err != nil {
		return nil, fmt.Errorf("resolve remote address error: %s", err)
	}
	// conn, err := net.ListenUDP("udp", la)
	// if err != nil {
	// 	logging.Error("listen error: %s", err)
	// 	return nil
	// }
	conn, err := net.DialUDP("udp", la, ra)
	if err != nil {
		return nil, fmt.Errorf("dial error: %s", err)
	}
	var rid = rand.Uint32()
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, rid)
	err = udpPunchWrite(conn, nil, udpPunchTypeHandshake, buf)
	if err != nil {
		return nil, fmt.Errorf("write punch handshake error: %s", err)
	}
	rspHead, rsp, err := udpPunchRead(conn, time.Second*10)
	if err != nil {
		return nil, fmt.Errorf("read punch handshake rsp error: %s", err)
	}
	logging.Debug("[uptp:tunnelMgr] read udp punch packet: %+v", rspHead)
	if rspHead.MsgType == udpPunchTypeHandshake {
		err = udpPunchWrite(conn, nil, udpPunchTypeHandshakeACK, buf)
		if err != nil {
			return nil, fmt.Errorf("write punch handshake ack error: %s", err)
		}
		rspHead, rsp, err = udpPunchRead(conn, time.Second*10)
	} else {
		err = udpPunchWrite(conn, nil, udpPunchTypeHandshakeACK, buf)
		if err != nil {
			return nil, fmt.Errorf("write punch handshake ack error: %s", err)
		}
	}
	if len(rsp) != 4 {
		return nil, fmt.Errorf("rand wrong handshake content")
	}
	rspID := binary.LittleEndian.Uint32(rsp)
	logging.Debug("[uptp:tunnelMgr] punch udp ok to %d", id)
	nc, err := tm.udpEngine.AddConn(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("unknown add connect error %s", err)
	}
	uptpConn := newRawUDPSession(nc, id)
	uptpConn.checkRecv = rid
	uptpConn.checkSend = rspID
	nc.SetSession(uptpConn)
	return uptpConn, nil
}
