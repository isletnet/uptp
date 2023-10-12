package main

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/isletnet/uptp"
	"github.com/isletnet/uptp/logging"
)

type portMapMessage struct {
	ra      *uptp.UptpAddr
	message []byte
}

type wrapSocketProxy struct {
	// ct int64
	ra *uptp.UptpAddr
	// messageID uint32
	// serviceID uint32
	sp *socketProxy
}

// func (wsp *wrapSocketProxy) setMessageID(id uint32) {
// 	atomic.StoreUint32(&wsp.messageID, id)
// }

// func (wsp *wrapSocketProxy) getMessageID() uint32 {
// 	return atomic.LoadUint32(&wsp.messageID)
// }

// func (wsp *wrapSocketProxy) setCheckTime(t int64) {
// 	atomic.StoreInt64(&wsp.ct, t)
// }

// func (wsp *wrapSocketProxy) getCheckTime() int64 {
// 	return atomic.LoadInt64(&wsp.ct)
// }

type portMapClient struct {
	updateTime int64
	serviceID  uint32
	// forwardID  uint32

	inCh  chan portMapMessage
	outCh chan portMapMessage

	mtxSPS    sync.RWMutex
	sps       map[uint64]*wrapSocketProxy
	listenMap sync.Map
	wgSPS     sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc

	serviceConn *uptp.UptpConn
	// forwardConn *uptp.UptpConn

	mtxUpdate sync.Mutex
	stopped   bool
}

func NewPortMapClient(conf *portMapServiceConf) *portMapClient {
	var sid uint32
	if conf != nil {
		sid = conf.ServiceID
	}
	return &portMapClient{
		serviceID: sid,
		// forwardID: fid,
		inCh:  make(chan portMapMessage, 1),
		outCh: make(chan portMapMessage, 1),
		sps:   make(map[uint64]*wrapSocketProxy),
	}
}
func (pmc *portMapClient) Init(uc *uptp.Uptpc) {
	if pmc.serviceID == 0 {
		pmc.serviceID = rand.Uint32()
	}
	// if pmc.forwardID == 0 {
	// 	pmc.forwardID = pmc.serviceID + 1
	// }
	pmc.serviceConn = uc.ListenApp(pmc.serviceID)
	// pmc.forwardConn = uc.ListenApp(pmc.forwardID)
	// logging.Debug("[portMapClient:init] %d %d", pmc.serviceID, pmc.forwardID)
	// uc.RegisterAppID(pmc.serviceID, pmc.wrapPortMapCtrlMsg())
	// uc.RegisterAppID(pmc.forwardID, pmc.handleForward)
	pmc.ctx, pmc.cancel = context.WithCancel(context.Background())
}
func (pmc *portMapClient) Start() {
	//todo select channel
	// go func() {
	// 	for {
	// 		ra, data, err := pmc.serviceConn.Read()
	// 		if err != nil {
	// 			logging.Error("service conn read error: %s", err)
	// 			break
	// 		}
	// 		pmc.wrapPortMapCtrlMsg(ra, data)
	// 	}
	// }()
	go func() {
		for {
			msg, ok := <-pmc.outCh
			if !ok {
				break
			}
			err := pmc.serviceConn.Write(msg.ra, msg.message)
			if err != nil {
				logging.Error("uptp connect send error: %s", err)
			}
		}
	}()
	go func() {
		for {
			msg, ok := <-pmc.inCh
			if !ok {
				break
			}
			pmc.forwardToSocketProxy(msg.ra, msg.message)
		}
	}()

	go func() {
		for {
			ra, data, err := pmc.serviceConn.Read()
			if err != nil {
				logging.Error("uptp conn read error: %s", err)
				break
			}

			pmc.inCh <- portMapMessage{
				ra:      ra,
				message: data,
			}
		}
	}()
}

func (pmc *portMapClient) Stop() {
	if pmc.stopped {
		return
	}
	pmc.stopped = true
	//todo unregister
	pmc.cancel()
	pmc.wgSPS.Wait()
	close(pmc.inCh)
	close(pmc.outCh)
	pmc.listenMap.Range(func(key, value any) bool {
		pmc.listenMap.Delete(key)
		return true
	})
}

func (pmc *portMapClient) forwardToSocketProxy(ra *uptp.UptpAddr, data []byte) {
	// pmc.mtxSPS.RLock()
	// s, ok := pmc.sps[id]
	// pmc.mtxSPS.RUnlock()
	// if !ok {
	// 	return
	// }
	s := pmc.getSocketProxy(ra)
	s.sp.WritePacket(data)
}

func (pmc *portMapClient) getSocketProxy(ra *uptp.UptpAddr) *wrapSocketProxy {
	pmc.mtxSPS.Lock()
	defer pmc.mtxSPS.Unlock()
	s, ok := pmc.sps[ra.GetClientID()]
	if !ok {
		s = &wrapSocketProxy{
			ra: ra,
			// messageID: 6666,
			sp: newSocketProxy(),
		}
		pmc.sps[ra.GetClientID()] = s
		pmc.wgSPS.Add(1)
		go pmc.readSocketProxy(s)
	}
	return s
}

func (pmc *portMapClient) deleteSocketProxy(portMapID uint64) {
	pmc.mtxSPS.Lock()
	defer pmc.mtxSPS.Unlock()
	s, ok := pmc.sps[portMapID]
	if !ok {
		return
	}
	listens := s.sp.GetAllListener()
	s.sp.Close()
	delete(pmc.sps, portMapID)
	for _, listen := range listens {
		pmc.listenMap.Delete(convertIndex(listen.Protocol, listen.LocalPort))
	}
}

type portMapCtrlMsg struct {
	MsgType    int    `json:"msgType"`
	ResponseID uint32 `json:"responseID"`
	AppID      uint32 `json:"appID"`
}

// type portMapResponse struct {
// 	AppID uint32 `json:"appID"`
// }

// func (pmc *portMapClient) wrapPortMapCtrlMsg(ra *uptp.UptpAddr, data []byte) {
// 	logging.Debug("[portMapClient:wrapPortMapCtrlMsg] recv port map ctrl msg: %s", string(data))
// 	req := portMapCtrlMsg{}
// 	err := json.Unmarshal(data, &req)
// 	if err != nil {
// 		//log
// 		logging.Error("[portMapClient:wrapPortMapCtrlMsg] unmarshal port map ctrl msg fail: %s", err)
// 		return
// 	}
// 	wsp := pmc.getSocketProxy(ra)
// 	wsp.setMessageID(req.AppID)

// 	if req.MsgType == 1 {
// 		return
// 	}

// 	rsp := portMapCtrlMsg{
// 		MsgType: 1,
// 		AppID:   pmc.forwardID,
// 	}
// 	buf, err := json.Marshal(rsp)
// 	if err != nil {
// 		//log
// 		logging.Error("[portMapClient:wrapPortMapCtrlMsg] marshal port map ctrl respons fail: %s", err)
// 		return
// 	}
// 	err = pmc.serviceConn.Write(ra, buf)
// 	if err != nil {
// 		//log
// 		logging.Error("[portMapClient:wrapPortMapCtrlMsg] marshal port map ctrl respons fail: %s", err)
// 		return
// 	}
// 	wsp.setCheckTime(time.Now().Unix())
// }

func (pmc *portMapClient) addPortMap(pmConf portMapConf) {
	// peerID := uptp.GetIDByName(pmConf.Peer)
	ra := uptp.ResolveUptpAddr(pmConf.Peer, pmConf.ServiceID)
	wsp := pmc.getSocketProxy(ra)
	// wsp.serviceID = pmConf.ServiceID
	oldListen := wsp.sp.GetAllListener()
	existPort := make(map[int]bool)
	for _, pmi := range pmConf.MapList {
		index := convertIndex(pmi.Protocol, pmi.LocalPort)
		_, err := wsp.sp.AddListener(fmt.Sprintf("%s:%d", pmi.TargetAddr, pmi.TargetPort), pmi.Protocol, pmi.LocalPort)
		if err != nil {
			logging.Error("[portMapClient:addPortMap] add listener fail: %s", err)
			continue
		}
		logging.Debug("[portMapClient:addPortMap] add or check listener on %s %s:%d--%s:%d", pmConf.Peer, pmi.Protocol, pmi.LocalPort, pmi.TargetAddr, pmi.TargetPort)
		pmc.listenMap.Store(index, wsp)
		existPort[index] = true
	}
	for _, ol := range oldListen {
		if existPort[convertIndex(ol.Protocol, ol.LocalPort)] {
			continue
		}
		logging.Debug("[portMapClient:addPortMap] delete expire listener on %s %s:%d", pmConf.Peer, ol.Protocol, ol.LocalPort)
		wsp.sp.DeleteListener(ol.Protocol, ol.LocalPort)
	}
	// wsp.setCheckTime(time.Now().Unix())
}

func (pmc *portMapClient) runAllPortMap(conf []portMapConf) {
	if conf == nil {
		return
	}
	tn := time.Now().Unix()
	atomic.StoreInt64(&pmc.updateTime, tn)
	logging.Debug("[portMapClient:runAllPortMap] start run all port map current time %d", tn)
	defer logging.Debug("[portMapClient:runAllPortMap] finish run all port map current time %d", tn)
	pmc.mtxUpdate.Lock()
	defer pmc.mtxUpdate.Unlock()
	if pmc.stopped {
		return
	}

	// clear expired listener
	peerMap := make(map[uint64]bool)
	portMap := make(map[int]bool)
	for _, pmcconf := range conf {
		if tn < atomic.LoadInt64(&pmc.updateTime) {
			logging.Info("[portMapClient:runAllPortMap] new task,exit current at %d ", tn)
			return
		}
		ra := uptp.ResolveUptpAddr(pmcconf.Peer, pmcconf.ServiceID)
		peerMap[ra.GetClientID()] = true
		for _, ml := range pmcconf.MapList {
			index := convertIndex(ml.Protocol, ml.LocalPort)
			tmpSP, ok := pmc.listenMap.Load(index)
			if !ok {
				continue
			}
			portMap[index] = true
			sp := tmpSP.(*wrapSocketProxy)
			if sp.ra.GetClientID() != ra.GetClientID() {
				logging.Info("clear expired listener %s:%d", ml.Protocol, ml.LocalPort)
				sp.sp.DeleteListener(ml.Protocol, ml.LocalPort)
				pmc.listenMap.Delete(index)
			}
		}
	}

	pmc.mtxSPS.Lock()
	for id, s := range pmc.sps {
		if peerMap[id] {
			continue
		}
		if s.sp.isActive() {
			continue
		}
		logging.Info("[portMapClient:runAllPortMap] delete expire socket proxy: %d", id)
		s.sp.Close()
		delete(pmc.sps, id)
	}
	pmc.mtxSPS.Unlock()

	pmc.listenMap.Range(func(key, value any) bool {
		index, ok := key.(int)
		if ok && portMap[index] {
			return true
		}
		logging.Info("[portMapClient:runAllPortMap] delete listen record: %d", index)
		pmc.listenMap.Delete(key)
		return true
	})
	for _, pmConf := range conf {
		if tn < atomic.LoadInt64(&pmc.updateTime) {
			logging.Info("[portMapClient:runAllPortMap] new task,exit current at %d", tn)
			return
		}
		pmc.addPortMap(pmConf)
	}
}

// todo clear socket proxy
func (pmc *portMapClient) readSocketProxy(wsp *wrapSocketProxy) {
	// pmc.sendCtrl(wsp.portMapID, wsp.serviceID)
	defer pmc.wgSPS.Done()
	readCh := wsp.sp.GetPacketChan()
	// tk := time.NewTicker(time.Second * 30)
	// defer tk.Stop()
	for {
		var stopped bool
		select {
		case <-pmc.ctx.Done():
			stopped = true
		// case <-tk.C:
		// 	ct := wsp.getCheckTime()
		// 	logging.Debug("[portMapClient:readSocketProxy] checktime %d", ct)
		// 	tn := time.Now().Unix()
		// 	if tn-ct > 600 {
		// 		stopped = true
		// 	}
		// 	pmc.sendCtrl(wsp.portMapID, wsp.serviceID)
		case msg, ok := <-readCh:
			if !ok {
				return
			}
			pmc.outCh <- portMapMessage{
				ra: wsp.ra,
				// messageID: wsp.getMessageID(),
				message: msg,
			}
		}
		if stopped {
			pmc.deleteSocketProxy(wsp.ra.GetClientID())
			break
		}
	}
}

// func (pmc *portMapClient) sendCtrl(peerID uint64, peerServiceID uint32) {
// 	req := portMapCtrlMsg{
// 		MsgType:    0,
// 		ResponseID: pmc.serviceID,
// 		AppID:      pmc.forwardID,
// 	}
// 	buf, err := json.Marshal(req)
// 	if err != nil {
// 		logging.Error("[portMapClient:addPortMap] marshal port map ctrl request fail: %s", err)
// 		return
// 	}
// 	err = pmc.uc.SendToTCP(peerID, peerServiceID, buf, false)
// 	if err != nil {
// 		logging.Error("[portMapClient:addPortMap] send port map ctrl request fail: %s", err)
// 		return
// 	}
// }
