package uptp

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/isletnet/uptp/logging"
)

type Uptpc struct {
	opt        UptpcOption
	dispatch   *uptpcMsgDispatcher
	msgHandler *uptpcMqttHandler
	wg         sync.WaitGroup
	globalInfo uptpcInfo
	tm         *tunnelMgr

	appHandleFunc map[uint32]*UptpConn
	appHandleMtx  sync.RWMutex

	reconnectMsgCh chan bool
}

func NewUptpc(opt UptpcOption) *Uptpc {
	d := newUptpcMsgDispatcher()
	c := &Uptpc{}
	mh := newUptpcMqttHandler(uptpcMqttOption{
		host:            opt.ServerHost,
		port:            opt.ServerPort,
		name:            opt.Name,
		epid:            hashStringToInt(opt.Name),
		handler:         d,
		disConnCallback: c.onMsgDisconnect,
		mqttPrefix:      "uptpc",
	})
	c.opt = opt
	c.dispatch = d
	c.msgHandler = mh
	c.tm = newTunnelMgr(c)
	c.reconnectMsgCh = make(chan bool, 1)
	c.appHandleFunc = make(map[uint32]*UptpConn)
	return c
}

// func Testest() {
// 	// publicIPTest("192.168.6.3", 34567)
// 	log.Println(publicIPTest("192.168.34.121", 34567))
// }

func (c *Uptpc) Start() {
	c.init()
	c.dispatch.registerHandler(msgIDTypeHeartbeat, c.handleHeartbeat)
	c.dispatch.registerHandler(msgIDTypeUptpcInfoReq, c.handleUptpcInfoReq)
	c.tm.start()
	go c.run()
}

func (c *Uptpc) init() {
	c.wg.Add(1)
	defer c.wg.Done()
	err := c.msgHandler.connectMqtt()
	if err != nil {
		logging.Error("[Uptpc:init] connect msg center error: %s", err)
		c.onMsgDisconnect(err)
		return
	}
	c.collectData()
	c.reportUptpcInfo()
}

func (c *Uptpc) collectData() {
	localIP, err := getLocalIPv4()
	if err != nil {
		logging.Error("[Uptpc:init] get local ip error: %s", err)
	}
	c.globalInfo.setLocalIP(localIP)
	logging.Info("[Uptpc:init] local ip %s", c.globalInfo.getLocalIP())
	c.globalInfo.setOS(getOsName())
	logging.Info("[Uptpc:init] OS %s", c.globalInfo.getOS())
	ipv6, pp, lp, err := natTest(c.opt.NatTestHost6, c.opt.NatTestPort1, 0)
	if err == nil && pp == lp {
		c.globalInfo.setIPv6(ipv6)
	}
	logging.Info("[Uptpc:init] ipv6 %s", c.globalInfo.getIPv6())
	natInfo, err := natTypeTest(c.opt.NatTestHost, c.opt.NatTestPort1, c.opt.NatTestPort2, c.opt.PublicPort)
	if err != nil {
		logging.Error("[Uptpc:init] nat type test error: %s", err)
	} else {
		c.globalInfo.setNatTypeInfo(natInfo)
	}
	logging.Info("[Uptpc:init] nat type info %+v", c.globalInfo.getNatTypeInfo())
	pipInfo, err := getPublicIPv4Info()
	if err != nil {
		logging.Error("[Uptpc:init] get public ipv4 info error: %s", err)
	} else {
		c.globalInfo.setPublicIPInfo(pipInfo)
	}
	logging.Info("[Uptpc:init] public ipv4 info: %+v", c.globalInfo.getPublicIPInfo())
}

func (c *Uptpc) reportUptpcInfo() {
	info := uptpcInfoReport{
		uptpcBaseInfo: c.getUptpcBaseInfo(),
		PIPInfo:       c.globalInfo.getPublicIPInfo(),
	}
	err := c.send("", msgIDTypeUptpcInfoReport, 0, info)
	if err != nil {
		logging.Error("[Uptpc:reportUptpcInfo] send uptp info report error: %s", err)
	}
}
func (c *Uptpc) run() {
	tk := time.NewTicker(time.Second * 5)
	for {
		select {
		case <-c.reconnectMsgCh:
			logging.Info("detect uptpc msg disconnect, reconnect later")
			c.wg.Wait()
			time.Sleep(time.Second * 10)
			c.init()
		case <-tk.C:
			// c.msgHandler.sendUptpMsg(&uptpMsg{
			// 	MsgType: msgIDTypeHeartbeat,
			// 	Content: []byte(""),
			// })
			// rsp, err := c.sendAndWaitUptpMsg("", &uptpMsg{
			// 	MsgType: msgIDTypeHeartbeat,
			// 	Content: []byte(""),
			// }, time.Second)
			// if err != nil {
			// 	logging.Error("send and wait error: ", err)
			// 	continue
			// }
			// logging.Info("%v", rsp.Content)
		}
	}
}

func (c *Uptpc) onMsgDisconnect(err error) {
	logging.Error("uptpc msg disconnect: %s", err)
	select {
	case c.reconnectMsgCh <- true:
	default:
	}
}

func (c *Uptpc) sendAndWait(to string, msgType uint16, data interface{}, d time.Duration) (*uptpMsg, error) {
	buf, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal send data error: %s", err)
	}
	return c.sendAndWaitUptpMsg(to, &uptpMsg{
		MsgType: msgType,
		Content: buf,
	}, d)
}

func (c *Uptpc) sendAndWaitUptpMsg(to string, msg *uptpMsg, d time.Duration) (*uptpMsg, error) {
	ch := make(chan *uptpMsg, 1)
	msg.CorrelationID = rand.Uint64()
	c.dispatch.addWaitCh(msg.CorrelationID, ch)
	defer c.dispatch.delWaitCh(msg.CorrelationID)
	err := c.sendUptpMsg(to, msg)
	if err != nil {
		return nil, fmt.Errorf("send uptp msg error: %s", err)
	}
	select {
	case <-time.After(d):
	case rsp := <-ch:
		return rsp, nil
	}
	return nil, errors.New("timeout")
}

func (c *Uptpc) getUptpcBaseInfo() uptpcBaseInfo {
	nt := c.globalInfo.getNatTypeInfo()
	return uptpcBaseInfo{
		Name:     c.opt.Name,
		ID:       hashStringToInt(c.opt.Name),
		LocalIP:  c.globalInfo.getLocalIP(),
		OS:       c.globalInfo.getOS(),
		IPv6:     c.globalInfo.getIPv6(),
		PublicIP: nt.publicIP,
		NatType:  nt.natType,

		IsExclusivePublicIPV4: nt.isExclusivePublicIPV4,
	}
}

func (c *Uptpc) send(to string, msgType uint16, corrID uint64, data interface{}) error {
	buf, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal send data error: %s", err)
	}
	return c.sendUptpMsg(to, &uptpMsg{
		MsgType:       msgType,
		CorrelationID: corrID,
		Content:       buf,
	})
}

func (c *Uptpc) sendUptpMsg(to string, msg *uptpMsg) error {
	if to == "" {
		return c.msgHandler.sendUptpMsg(msg)
	}

	return c.msgHandler.sendUptpMsgTo(to, msg)
}

func (c *Uptpc) handleHeartbeat(msg *uptpMsg) {
	logging.Debug("%v", msg.Content)
}

func (c *Uptpc) handleUptpcInfoReq(msg *uptpMsg) {
	logging.Info("[Uptpc:handleUptpcInfoReq] recv %s", string(msg.Content))
	req := uptpcInfoRequest{}
	err := json.Unmarshal(msg.Content, &req)
	if err != nil {
		logging.Error("[Uptpc:handleUptpcInfoReq] format request error: %s", err)
		return
	}
	rsp := uptpcInfoResponse{
		uptpcBaseInfo: c.getUptpcBaseInfo(),
	}
	err = c.send(req.ReplyTo, msgIDTypeUptpcInfoRsp, msg.CorrelationID, rsp)
	if err != nil {
		logging.Error("[Uptpc:handleUptpcInfoReq] send response error: %s", err)
		return
	}
}

type UptpcOption struct {
	Name       string
	PublicPort int
	UptpServerOption
}

type UptpServerOption struct {
	ServerHost   string `yaml:"server_host"`
	ServerPort   int    `yaml:"server_port"`
	NatTestHost  string `yaml:"nat_test_host"`
	NatTestPort1 int    `yaml:"nat_test_port1"`
	NatTestPort2 int    `yaml:"nat_test_port2"`
	NatTestHost6 string `yaml:"nat_test_host6"`
}

// func (c *Uptpc) Dial(peerName string) (*UptpConn, error) {
// 	peerInfo, err := c.requestPeerInfo(peerName)
// 	if err != nil {
// 		return nil, fmt.Errorf("request peer info error: %s", err)
// 	}
// 	conn, err := c.tm.getTunnel(*peerInfo)
// 	if conn == nil {
// 		logging.Error("add tunnel failed")
// 		return nil, nil
// 	}
// 	for {
// 		err := conn.SendPacket(1234, 5678, []byte("dfjkdfkdfjkdfjkdfjkdjkf"))
// 		if err != nil {
// 			logging.Error("write tunnel error: %s", err)
// 			break
// 		}
// 		time.Sleep(time.Second)
// 	}
// 	return &UptpConn{}, nil
// }

func (c *Uptpc) handleUptpPacket(head *uptpPacketHead, data []byte) {
	if len(data) < 8 {
		logging.Error("wrong uptp app packet")
		return
	}
	fromApp := binary.LittleEndian.Uint32(data[:4])
	toApp := binary.LittleEndian.Uint32(data[4:8])
	c.appHandleMtx.RLock()
	uconn, ok := c.appHandleFunc[toApp]
	c.appHandleMtx.RUnlock()
	if !ok {
		logging.Error("no conn for %d found", toApp)
		return
	}
	uconn.handle(head.From, fromApp, data[8:])
}

func (c *Uptpc) requestPeerInfo(peerID uint64) (*uptpcBaseInfo, error) {
	to := fmt.Sprintf("uptpc/%d", peerID)
	reply := fmt.Sprintf("uptpc/%d", hashStringToInt(c.opt.Name))
	req := uptpcInfoRequest{
		ReplyTo: reply,
	}
	rsp, err := c.sendAndWait(to, msgIDTypeUptpcInfoReq, req, time.Second*10)
	if err != nil {
		logging.Error("[Uptpc:requestPeerInfo] send or wait peer info error: %s", err)
		return nil, err
	}
	// logging.Info("%+v", rsp)
	peerInfoRsp := uptpcInfoResponse{}
	err = json.Unmarshal(rsp.Content, &peerInfoRsp)
	if err != nil {
		return nil, err
	}
	return &peerInfoRsp.uptpcBaseInfo, nil
}

func (c *Uptpc) ListenApp(app uint32) *UptpConn {
	uconn := newUptpConn()
	uconn.la = &UptpAddr{
		cid: hashStringToInt(c.opt.Name),
		app: app,
	}
	uconn.onClose = c.onUptpConnClose
	uconn.writeFunc = c.sendAppMsg
	c.appHandleMtx.Lock()
	c.appHandleFunc[app] = uconn
	c.appHandleMtx.Unlock()
	return uconn
}

func (c *Uptpc) sendAppMsg(la, ra *UptpAddr, data []byte) error {
	conn, err := c.tm.getTunnel(ra.cid)
	if err != nil {
		return fmt.Errorf("get tunnel error: %w", err)
	}
	appBuf := [8]byte{}
	binary.LittleEndian.PutUint32(appBuf[:4], la.app)
	binary.LittleEndian.PutUint32(appBuf[4:8], ra.app)
	packetData := append(appBuf[:], data...)
	return conn.SendPacket(la.cid, ra.cid, packetData)
}

func (c *Uptpc) onUptpConnClose(app uint32) {
	c.appHandleMtx.Lock()
	delete(c.appHandleFunc, app)
	c.appHandleMtx.Unlock()
}
