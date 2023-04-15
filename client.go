package uptp

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/lesismal/nbio"
	"github.com/lesismal/nbio/logging"
)

type NptpcConfig struct {
	ServerAddr string `yaml:"server_address"`
	ListenPort int    `yaml:"listen_port,omitempty"`
}

type peerSock struct {
	peerID uint64
	// ct      int64
	mux     sync.Mutex
	cond    *sync.Cond
	conn    uptpConn
	stopSig chan struct{}
	ready   bool
}

type peerSockMgr struct {
	mux     sync.Mutex
	cache   map[uint64]*peerSock
	reqCB   func(uint64, string) error
	toCB    func(uint64)
	addrCB  func(uint64, string) (uptpConn, uint64, error)
	network string
}

func newPeerSockMgr(netwrok string, reqCB func(uint64, string) error, toCB func(uint64), addrCB func(uint64, string) (uptpConn, uint64, error)) *peerSockMgr {
	ps := peerSockMgr{
		network: netwrok,
		cache:   make(map[uint64]*peerSock),
		reqCB:   reqCB,
		toCB:    toCB,
		addrCB:  addrCB,
	}
	return &ps
}

func (pm *peerSockMgr) getConn(peerID uint64, notBlock bool) (uptpConn, error) {
	pm.mux.Lock()
	ps, ok := pm.cache[peerID]
	if !ok {
		//new sock and send start
		ps = newPeerSock(peerID)
		pm.cache[peerID] = ps
		err := pm.reqCB(peerID, pm.network)
		if err != nil {
			pm.mux.Unlock()
			return nil, fmt.Errorf("request connect fail: %s", err)
		}
	}
	pm.mux.Unlock()

	return ps.getConn(pm.toCB, notBlock)
}

func (pm *peerSockMgr) addPeerConn(peerID uint64, conn uptpConn) {
	pm.mux.Lock()
	defer pm.mux.Unlock()
	ps, ok := pm.cache[peerID]
	if !ok {
		ps = newPeerSock(peerID)
	}
	if ps.isReady() {
		return
	}
	ps.setConnect(conn)
	pm.cache[peerID] = ps
	ps.stopWait()
}

func (pm *peerSockMgr) deletePeerSock(peerID uint64) {
	pm.mux.Lock()
	_, ok := pm.cache[peerID]
	if ok {
		delete(pm.cache, peerID)
	}
	pm.mux.Unlock()
}

func (pm *peerSockMgr) clearPeerSock() {
	pm.mux.Lock()
	for peerID, ps := range pm.cache {
		delete(pm.cache, peerID)
		ps.stopWait()
	}
	pm.mux.Unlock()
}

func (pm *peerSockMgr) handleAddr(peerID uint64, peerAddr string) {
	if peerID == 0 {
		return
	}
	pm.mux.Lock()
	ps, ok := pm.cache[peerID]
	if !ok {
		pm.mux.Unlock()
		return
	}
	pm.mux.Unlock()
	if ps.isReady() {
		return
	}
	c, cid, err := pm.addrCB(ps.peerID, peerAddr)
	if err != nil {
		logging.Debug("[peerSockMgr:handleAddr] handle addr fail: %s", err)
		//ps.stopWait()
		return
	}
	pm.mux.Lock()
	ps.setConnect(c)
	pm.cache[ps.peerID] = ps
	ps.stopWait()
	pm.mux.Unlock()
	c.SendMessage(cid, ps.peerID, 3, nil)
}

func newPeerSock(id uint64) *peerSock {
	return &peerSock{
		peerID: id,
	}
}

func (ps *peerSock) isReady() bool {
	ps.mux.Lock()
	defer ps.mux.Unlock()
	return ps.ready
}

func (ps *peerSock) setConnect(conn uptpConn) {
	ps.mux.Lock()
	defer ps.mux.Unlock()
	ps.conn = conn
	ps.ready = conn != nil
}

func (ps *peerSock) getConn(tocb func(uint64), notBlock bool) (uptpConn, error) {
	ps.mux.Lock()
	defer ps.mux.Unlock()
	if ps.ready {
		return ps.conn, nil
	}
	if notBlock {
		return nil, fmt.Errorf("connect not ready")
	}
	var stat = 0
	if ps.cond == nil {
		ps.stopSig = make(chan struct{}, 1)
		ps.cond = sync.NewCond(&ps.mux)
		go func() {
			tm := time.NewTimer(time.Second * 10)
			select {
			case <-tm.C:
				ps.mux.Lock()
				if stat == 0 {
					stat = 1
				}
				ps.mux.Unlock()
				if stat == 1 {
					tocb(ps.peerID)
					ps.stopWait()
				}
			case <-ps.stopSig:
				tm.Stop()
			}
		}()
	}
	ps.cond.Wait()
	if stat == 1 {
		return nil, fmt.Errorf("wait connect time out")
	}
	stat = 2
	select {
	case ps.stopSig <- struct{}{}:
	default:
	}
	return ps.conn, nil
}

func (ps *peerSock) stopWait() {
	if ps.cond == nil {
		return
	}
	ps.cond.Broadcast()
}

// type connCheckItem struct {
// 	check  uint32
// 	peerID int64
// }

type Uptpc struct {
	g             *nbio.Gopher
	tcpEngine     *nbio.Gopher
	appHandleFunc map[uint32]func(uptpConn, *uptpHead, []byte)
	// messageHandleMap []func(*rawUDPconn, *uptpHead, []byte)
	serverConn       uptpConn
	cid              uint64
	mux              sync.RWMutex
	heartbeatTK      *time.Ticker
	heartbeatStopSig chan struct{}
	psm              *peerSockMgr
	psmTCP           *peerSockMgr
	info             *NptpcConfig
	udpPort          int
	tcpPort          int
	isRunning        bool

	// idCh chan uint64
}

// func (uc *Uptpc) GetNptpCID() uint64 {
// 	var ret uint64
// 	select {
// 	case ret = <-uc.idCh:
// 	case <-time.After(time.Second * 5):
// 		return 0
// 	}
// 	if ret == 0 {
// 		return 0
// 	}
// 	select {
// 	case uc.idCh <- ret:
// 	default:
// 	}
// 	return ret
// }

func (uc *Uptpc) setConnect(conn uptpConn) {
	uc.mux.Lock()
	uc.serverConn = conn
	uc.mux.Unlock()
}

func (uc *Uptpc) sendMessageToServer(appID uint32, data []byte) error {
	uc.mux.RLock()
	conn := uc.serverConn
	cid := uc.cid
	uc.mux.RUnlock()

	if conn == nil {
		return fmt.Errorf("no server connection")
	}
	err := conn.SendMessage(cid, 0, appID, data)
	if err != nil {
		return fmt.Errorf("send data to server fail:%s", err)
	}
	return nil
}

func NewUPTPClient(name string, nc NptpcConfig) *Uptpc {
	h := make(map[uint32]func(uptpConn, *uptpHead, []byte))
	ret := &Uptpc{
		appHandleFunc:    h,
		info:             &nc,
		heartbeatStopSig: make(chan struct{}, 1),
		// idCh:             make(chan uint64, 1),
	}
	g := nbio.NewGopher(nbio.Config{
		Network:            "udp",
		Addrs:              []string{"[::]:" + strconv.Itoa(nc.ListenPort)},
		ReadBufferSize:     1600,
		MaxWriteBufferSize: 1600,
		UDPReadTimeout:     time.Second * 30,
		ListenUDP:          ret.funListenUDP,
	})
	g.OnData(wrapOnDataRawUDPConn(ret.handleRawUDPData, nil))
	g.OnOpen(wrapOnOpenRawUDPConn(nil))
	g.OnClose(wrapOnCloseRawUDPConn(ret.onRawUDPConnClose))
	ret.g = g

	tcpG := nbio.NewGopher(nbio.Config{
		Network:            "tcp",
		Addrs:              []string{":" + strconv.Itoa(nc.ListenPort)},
		ReadBufferSize:     5300,
		MaxWriteBufferSize: 5300,
		Listen:             ret.funListenTCP,
	})
	tcpG.OnClose(wrapOnCloseRawTCPConn(ret.onRawTCPConnClose))
	tcpG.OnData(wrapOnDataRawTCPConn(ret.handleRawTCPData, nil))
	ret.tcpEngine = tcpG
	// ret.messageHandleMap = []func(*rawUDPconn, *uptpHead, []byte){ret.handleV1Data}
	ret.appHandleFunc[1] = ret.appid1handler
	ret.appHandleFunc[2] = ret.appid2handler
	ret.appHandleFunc[3] = ret.appid3handler
	ret.psm = newPeerSockMgr("udp", ret.queryAddrByID, ret.waitPeerConnectTimeout, ret.dialPeerUDP)
	ret.psmTCP = newPeerSockMgr("tcp", ret.queryAddrByID, ret.waitPeerConnectTimeoutTCP, ret.dialPeerTCP)
	ret.cid = GetIDByName(name)
	return ret
}

func (uc *Uptpc) StopHeartbeat() {
	uc.heartbeatTK.Stop()
	select {
	case uc.heartbeatStopSig <- struct{}{}:
	default:
	}
}

func (uc *Uptpc) funListenUDP(network string, laddr *net.UDPAddr) (*net.UDPConn, error) {
	c, e := net.ListenUDP("udp", laddr)
	if e != nil {
		return c, e
	}
	uc.udpPort = c.LocalAddr().(*net.UDPAddr).Port
	return c, e
}
func (uc *Uptpc) funListenTCP(network, addr string) (net.Listener, error) {
	l, e := net.Listen("tcp", addr)
	if e != nil {
		return nil, e
	}
	uc.tcpPort = l.Addr().(*net.TCPAddr).Port
	return l, nil
}

func (uc *Uptpc) Start() error {
	uc.heartbeatTK = time.NewTicker(time.Second * 25)
	uc.heartbeatTK.Stop()
	err := uc.g.Start()
	if err != nil {
		return err
	}
	err = uc.tcpEngine.Start()
	if err != nil {
		return err
	}
	uc.isRunning = true
	uc.startConnectServer()
	return nil
}

func (uc *Uptpc) startConnectServer() {
	go func() {
		for {
			err := uc.connectServer()
			if err == nil {
				break
			}
			logging.Debug("[Uptpc:startConnectServer] connect server: %s", err)
			time.Sleep(time.Second)
		}
	}()
}

func (uc *Uptpc) connectServer() error {
	logging.Debug("[Uptpc:connectServer] start to connect server: %s", uc.info.ServerAddr)
	uptpConn, err := dialRawUDPConn(uc.info.ServerAddr, uc.g)
	if err != nil {
		return err
	}
	ui := UPTPInfo{
		PeerID:  uc.cid,
		TCPPort: uc.tcpPort,
		UDPPort: uc.udpPort,
	}
	sendBuf, err := json.Marshal(ui)
	if err != nil {
		return err
	}
	err = uptpConn.SendMessage(0, 0, 1, sendBuf)
	if err != nil {
		return err
	}
	uptpConn.conn.SetReadDeadline(time.Now().Add(time.Second * 30))
	return nil
}

func (uc *Uptpc) Stop() {
	uc.isRunning = false
	uc.psm.clearPeerSock()
	uc.psmTCP.clearPeerSock()
	uc.g.Stop()
	uc.tcpEngine.Stop()
	uc.g.Wait()
}

type appHandler func(from uint64, data []byte)

func (uc *Uptpc) RegisterAppID(appID uint32, h appHandler) {
	logging.Debug("[Uptpc:RegisterAppID] register app: %d", appID)
	uc.appHandleFunc[appID] = func(u uptpConn, uh *uptpHead, b []byte) {
		h(uh.From, b)
	}
}

func (uc *Uptpc) RegisterPeerDisconnect() {}

func (uc *Uptpc) getPeerConn(cid uint64, notBlock bool) (uptpConn, error) {
	if !uc.isRunning {
		return nil, fmt.Errorf("uptp client is not running")
	}
	return uc.psm.getConn(cid, notBlock)
}
func (uc *Uptpc) getPeerConnTCP(cid uint64, notBlock bool) (uptpConn, error) {
	if !uc.isRunning {
		return nil, fmt.Errorf("uptp client is not running")
	}
	return uc.psmTCP.getConn(cid, notBlock)
}

func (uc *Uptpc) onRawUDPConnClose(c *rawUDPconn, err error) {
	if c.peerID == 0 {
		if !c.isClient {
			//ignore connect accept from other peer
			return
		}
		if err != nil {
			logging.Debug("[Uptpc:onRawUDPConnClose] server connect close: %s", err)
		}
		uc.serverConn = nil
		uc.StopHeartbeat()

		if uc.isRunning {
			//run without go will already in use
			go uc.startConnectServer()
		}
	} else {
		if err != nil {
			logging.Debug("[Uptpc:onRawUDPConnClose] peer %d connect close: %s", c.peerID, err)
		}
		uc.psm.deletePeerSock(c.peerID)
		uc.onUPTPConnClose(c.peerID)
	}
}

func (uc *Uptpc) onRawTCPConnClose(c *rawTCPConn, err error) {
	if err != nil {
		logging.Debug("[Uptpc:onRawTCPConnClose] peer %d tcp connect close: %s", c.peerID, err)
	}
	if c.GetPeerID() == 0 {
		return
	}
	uc.psmTCP.deletePeerSock(c.peerID)
	uc.onUPTPConnClose(c.peerID)
}

func (uc *Uptpc) onUPTPConnClose(peerID uint64) {

}

func (uc *Uptpc) handleRawUDPData(c *rawUDPconn, head *uptpHead, data []byte) {
	uc.handleV1Data(c, head, data)
}

func (uc *Uptpc) handleRawTCPData(c *rawTCPConn, head *uptpHead, data []byte) {
	uc.handleV1Data(c, head, data)
}

func (uc *Uptpc) handleV1Data(c uptpConn, head *uptpHead, data []byte) {
	if head.To != uc.cid {
		//forward
		return
	}
	f, ok := uc.appHandleFunc[head.AppID]
	if !ok {
		return
	}
	f(c, head, data)
}

func (uc *Uptpc) appid1handler(c uptpConn, head *uptpHead, data []byte) {
	if head.Len == 0 {
		return
	}
	id := binary.LittleEndian.Uint64(data)
	logging.Debug("[Uptpc:appid1handler] login success %d", id)
	uc.setConnect(c)
	// select {
	// case uc.idCh <- id:
	// default:
	// }
	uc.startHeartbeatLoop()
}

func (uc *Uptpc) appid2handler(c uptpConn, head *uptpHead, data []byte) {
	if head.Len == 0 {
		return
	}
	var ui UPTPInfo
	err := json.Unmarshal(data, &ui)
	if err != nil {
		logging.Debug("[Uptpc:appid2handler] unmarshal uptp info fail: %s", err)
		return
	}

	network := ui.Extra.(string)
	var peerAddr string
	if network == "tcp" {
		peerAddr = fmt.Sprintf("[%s]:%d", ui.PublicIP, ui.TCPPort)
		go uc.psmTCP.handleAddr(ui.PeerID, peerAddr)
	} else {
		peerAddr = fmt.Sprintf("[%s]:%d", ui.PublicIP, ui.UDPPort)
		go uc.psm.handleAddr(ui.PeerID, peerAddr)
	}
}

func (uc *Uptpc) appid3handler(c uptpConn, head *uptpHead, data []byte) {
	//noti conn peer id
	c.SetPeerID(head.From)
	if _, ok := c.(*rawUDPconn); ok {
		uc.psm.addPeerConn(head.From, c)
	} else {
		uc.psmTCP.addPeerConn(head.From, c)
	}
}

func (uc *Uptpc) startHeartbeatLoop() {
	go func() {
		uc.heartbeatTK.Reset(time.Second * 25)
		running := true
		select {
		case <-uc.heartbeatStopSig:
		default:
		}
		for running {
			select {
			case <-uc.heartbeatTK.C:
				uc.sendHeartbeatToServer()
			case <-uc.heartbeatStopSig:
				running = false
			}
		}
	}()
}

func (uc *Uptpc) sendHeartbeatToServer() {
	err := uc.sendMessageToServer(1, nil)
	if err != nil {
		logging.Debug("[Uptpc:sendHeartbeatToServer] send heartbeat to server fail: %s", err)
	}
}

func (uc *Uptpc) queryAddrByID(id uint64, network string) error {
	logging.Debug("[Uptpc:queryAddrByID] start query addr of: %d", id)
	var ui UPTPInfo
	ui.Extra = network
	ui.PeerID = id
	buf, err := json.Marshal(ui)
	if err != nil {
		return err
	}
	err = uc.sendMessageToServer(2, buf)
	if err != nil {
		return fmt.Errorf("send query request fail: %s", err)
	}
	return nil
}

func (uc *Uptpc) waitPeerConnectTimeout(peerID uint64) {
	uc.psm.deletePeerSock(peerID)
}

func (uc *Uptpc) waitPeerConnectTimeoutTCP(peerID uint64) {
	uc.psmTCP.deletePeerSock(peerID)
}

func (uc *Uptpc) dialPeerUDP(peerID uint64, peerAddr string) (uptpConn, uint64, error) {
	logging.Debug("[Uptpc:dialPeerUDP] start to dial peer udp: %d, %s", peerID, peerAddr)
	uc.mux.RLock()
	cid := uc.cid
	uc.mux.RUnlock()
	uptpConn, err := dialRawUDPConn(peerAddr, uc.g)
	if err != nil {
		return nil, 0, err
	}
	uptpConn.peerID = peerID
	uptpConn.conn.SetReadDeadline(time.Now().Add(time.Second * 30))
	return uptpConn, cid, nil
}
func (uc *Uptpc) dialPeerTCP(peerID uint64, peerAddr string) (uptpConn, uint64, error) {
	logging.Debug("[uptpc:dialPeerTCP] start to dial peer tcp: %d, %s", peerID, peerAddr)
	uc.mux.RLock()
	cid := uc.cid
	uc.mux.RUnlock()
	uptpConn, err := dialRawTCPConn(peerAddr, uc.tcpEngine)
	if err != nil {
		return nil, 0, err
	}
	uptpConn.peerID = peerID
	uptpConn.conn.SetReadDeadline(time.Now().Add(time.Second * 30))
	return uptpConn, cid, nil
}

func (uc *Uptpc) SendTo(peerID uint64, appID uint32, content []byte, notBlock bool) error {
	uc.mux.RLock()
	cid := uc.cid
	uc.mux.RUnlock()
	if peerID == 0 {
		return fmt.Errorf("wrong peer id")
	}
	conn, err := uc.getPeerConn(peerID, notBlock)
	if err != nil {
		return fmt.Errorf("try connect peer fail: %s", err)
	}
	err = conn.SendMessage(cid, peerID, appID, content)
	if err != nil {
		return fmt.Errorf("send message fail: %s", err)
	}
	return nil
}

func (uc *Uptpc) SendToTCP(peerID uint64, appID uint32, content []byte, notBlock bool) error {
	uc.mux.RLock()
	cid := uc.cid
	uc.mux.RUnlock()
	if peerID == 0 {
		return fmt.Errorf("wrong peer id")
	}
	conn, err := uc.getPeerConnTCP(peerID, notBlock)
	if err != nil {
		return fmt.Errorf("try connect peer fail: %s", err)
	}
	err = conn.SendMessage(cid, peerID, appID, content)
	if err != nil {
		return fmt.Errorf("send message fail: %s", err)
	}
	return nil
}
