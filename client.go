package uptp

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/lesismal/nbio"
)

type NptpcConfig struct {
	ServerAddr string `yaml:"server_address"`
	ListenPort int    `yaml:"listen_port,omitempty"`
	Token      int64  `yaml:"token"`
}

type peerSock struct {
	peerID  int64
	mux     sync.Mutex
	cond    *sync.Cond
	conn    uptpConn
	stopSig chan struct{}
	ready   bool
}

type peerSockMgr struct {
	mux    sync.Mutex
	cache  map[int64]*peerSock
	reqCB  func(int64) error
	toCB   func(int64)
	addrCB func(int64, string) (uptpConn, int64, error)
}

func newPeerSockMgr(reqCB func(int64) error, toCB func(int64), addrCB func(int64, string) (uptpConn, int64, error)) *peerSockMgr {
	ps := peerSockMgr{
		cache:  make(map[int64]*peerSock),
		reqCB:  reqCB,
		toCB:   toCB,
		addrCB: addrCB,
	}
	return &ps
}

func (pm *peerSockMgr) getConn(peerID int64) (uptpConn, error) {
	pm.mux.Lock()
	ps, ok := pm.cache[peerID]
	if !ok {
		//new sock and send start
		ps = newPeerSock(peerID)
		pm.cache[peerID] = ps
		err := pm.reqCB(peerID)
		if err != nil {
			pm.mux.Unlock()
			return nil, fmt.Errorf("request connect fail: %s", err)
		}
	}
	pm.mux.Unlock()

	return ps.getConn(pm.toCB)
}

func (pm *peerSockMgr) addPeerConn(peerID int64, conn uptpConn) {
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

func (pm *peerSockMgr) deletePeerSock(peerID int64) {
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

func (pm *peerSockMgr) handleAddr(peerID int64, peerAddr string) {
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
	c, cid, err := pm.addrCB(peerID, peerAddr)
	if err != nil {
		log.Println("handle addr fail: ", err)
		//ps.stopWait()
		return
	}
	pm.mux.Lock()
	ps.setConnect(c)
	pm.cache[peerID] = ps
	ps.stopWait()
	pm.mux.Unlock()
	c.SendMessage(cid, peerID, 3, nil)
}

func newPeerSock(id int64) *peerSock {
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

func (ps *peerSock) getConn(tocb func(int64)) (uptpConn, error) {
	ps.mux.Lock()
	defer ps.mux.Unlock()
	if ps.ready {
		return ps.conn, nil
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

type connCheckItem struct {
	check  uint32
	peerID int64
}

type Uptpc struct {
	g             *nbio.Gopher
	cache         sync.Map
	appHandleFunc map[uint32]func(uptpConn, *uptpHead, []byte)
	// messageHandleMap []func(*rawUDPconn, *uptpHead, []byte)
	serverConn       uptpConn
	cid              int64
	mux              sync.RWMutex
	heartbeatTK      *time.Ticker
	heartbeatStopSig chan struct{}
	psm              *peerSockMgr
	info             *NptpcConfig
	localPort        int
	isRunning        bool

	idCh chan int64
}

func (uc *Uptpc) GetNptpCID() int64 {
	var ret int64
	select {
	case ret = <-uc.idCh:
	case <-time.After(time.Second * 5):
		return 0
	}
	if ret == 0 {
		return 0
	}
	select {
	case uc.idCh <- ret:
	default:
	}
	return ret
}

func (uc *Uptpc) setConnect(conn uptpConn, cid int64) {
	if cid != uc.info.Token {
		uc.info.Token = cid
	}
	uc.mux.Lock()
	uc.serverConn = conn
	uc.cid = cid
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

func NewUPTPClient(nc NptpcConfig) *Uptpc {
	h := make(map[uint32]func(uptpConn, *uptpHead, []byte))
	ret := &Uptpc{
		appHandleFunc:    h,
		info:             &nc,
		heartbeatStopSig: make(chan struct{}, 1),
		idCh:             make(chan int64, 1),
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
	// ret.messageHandleMap = []func(*rawUDPconn, *uptpHead, []byte){ret.handleV1Data}
	ret.appHandleFunc[1] = ret.appid1handler
	ret.appHandleFunc[2] = ret.appid2handler
	ret.appHandleFunc[3] = ret.appid3handler
	ret.psm = newPeerSockMgr(ret.queryAddrByID, ret.waitPeerConnectTimeout, ret.dialPeer)
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
	uc.localPort = c.LocalAddr().(*net.UDPAddr).Port
	return c, e
}

func (uc *Uptpc) Start() error {
	uc.heartbeatTK = time.NewTicker(time.Second * 25)
	uc.heartbeatTK.Stop()
	err := uc.g.Start()
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
			log.Println("connect server:", err)
			time.Sleep(time.Second)
		}
	}()
}

func (uc *Uptpc) connectServer() error {
	log.Println("start to connect server:", uc.info.ServerAddr)
	uptpConn, err := dialRawUDPConn(uc.info.ServerAddr, uc.g)
	if err != nil {
		return err
	}

	var idBytes [12]byte
	binary.LittleEndian.PutUint64(idBytes[:8], uint64(uc.info.Token))
	binary.LittleEndian.PutUint32(idBytes[8:], uint32(uc.localPort))
	err = uptpConn.SendMessage(0, 0, 1, idBytes[:])
	if err != nil {
		return err
	}
	uptpConn.conn.SetReadDeadline(time.Now().Add(time.Second * 30))
	return nil
}

func (uc *Uptpc) Stop() {
	uc.isRunning = false
	uc.psm.clearPeerSock()
	uc.g.Stop()
	uc.g.Wait()
}

type appHandler func(from int64, data []byte)

func (uc *Uptpc) RegisterAppID(appID uint32, h appHandler) {
	uc.appHandleFunc[appID] = func(u uptpConn, uh *uptpHead, b []byte) {
		h(uh.From, b)
	}
}

func (uc *Uptpc) getPeerConn(cid int64) (uptpConn, error) {
	if !uc.isRunning {
		return nil, fmt.Errorf("uptp client is not running")
	}
	return uc.psm.getConn(cid)
}

func (uc *Uptpc) onRawUDPConnClose(c *rawUDPconn, err error) {
	if c.peerID == 0 {
		if !c.isClient {
			//ignore connect accept from other peer
			return
		}
		if err != nil {
			log.Println("server connect close: ", err)
		}
		uc.serverConn = nil
		uc.cid = 0
		uc.StopHeartbeat()

		if uc.isRunning {
			//run without go will already in use
			go uc.startConnectServer()
		}
	} else {
		if err != nil {
			log.Printf("peer %d connect close: %s", c.peerID, err)
		}
		uc.psm.deletePeerSock(c.peerID)
	}
}

func (uc *Uptpc) handleRawUDPData(c *rawUDPconn, head *uptpHead, data []byte) {
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
	id := int64(binary.LittleEndian.Uint64(data))
	// log.Printf("onappid1Message: [%p, %v], %v", c, c.RemoteAddr().String(), id)
	uc.setConnect(c, id)
	select {
	case uc.idCh <- id:
	default:
	}
	uc.startHeartbeatLoop()
}

func (uc *Uptpc) appid2handler(c uptpConn, head *uptpHead, data []byte) {
	if len(data) < 8 {
		return
	}
	id := int64(binary.LittleEndian.Uint64(data[:8]))
	// log.Printf("onappid3Message: [%p, %v], %v, %v", c, c.RemoteAddr().String(), id, string(data[8:]))
	go uc.psm.handleAddr(id, string(data[8:]))
}

func (uc *Uptpc) appid3handler(c uptpConn, head *uptpHead, data []byte) {
	//noti conn peer id
	uc.psm.addPeerConn(head.From, c)
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
		log.Println("send heartbeat to server fail:", err)
	}
}

func (uc *Uptpc) queryAddrByID(id int64) error {
	log.Println("start query addr of: ", id)
	var idBytes [8]byte
	binary.LittleEndian.PutUint64(idBytes[:], uint64(id))
	err := uc.sendMessageToServer(2, idBytes[:])
	if err != nil {
		return fmt.Errorf("send query request fail:%s", err)
	}
	return nil
}

func (uc *Uptpc) waitPeerConnectTimeout(peerID int64) {
	uc.psm.deletePeerSock(peerID)
}

func (uc *Uptpc) dialPeer(peerID int64, peerAddr string) (uptpConn, int64, error) {
	log.Printf("start to dial peer: %d, %s", peerID, peerAddr)
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

func (uc *Uptpc) SendTo(peerID int64, appID uint32, content []byte) error {
	uc.mux.RLock()
	cid := uc.cid
	uc.mux.RUnlock()
	if peerID == 0 {
		return fmt.Errorf("wrong peer id")
	}
	conn, err := uc.getPeerConn(peerID)
	if err != nil {
		return fmt.Errorf("try connect peer fail: %s", err)
	}
	err = conn.SendMessage(cid, peerID, appID, content)
	if err != nil {
		return fmt.Errorf("send message fail: %s", err)
	}
	return nil
}
