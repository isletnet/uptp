package uptp

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"github.com/bwmarrin/snowflake"
	"github.com/lesismal/nbio"
)

type Uptps struct {
	peerMap  sync.Map
	snowNode *snowflake.Node

	// messageHandleMap []func(*rawUDPconn, *uptpHead, []byte)
	appHandleFunc map[uint32]func(*rawUDPconn, *uptpHead, []byte)

	nbioEngine *nbio.Engine
}

type NptpsConfig struct {
	Udp6Addr string
	SnowNode int64
}

func NewUPTPServer(nc NptpsConfig) *Uptps {
	ret := &Uptps{}
	g := nbio.NewGopher(nbio.Config{
		Network:            "udp",
		Addrs:              []string{nc.Udp6Addr},
		ReadBufferSize:     1600,
		MaxWriteBufferSize: 1600,
		UDPReadTimeout:     time.Second * 30,
		ListenUDP:          ret.funListenUDP,
	})
	sn, _ := snowflake.NewNode(nc.SnowNode)
	ret.snowNode = sn
	ret.nbioEngine = g
	ret.nbioEngine.OnData(wrapOnDataRawUDPConn(ret.handleRecvData, nil))
	ret.nbioEngine.OnOpen(wrapOnOpenRawUDPConn(func(u *rawUDPconn) {
		// log.Printf("onOpen: %v", u.conn.RemoteAddr().String())
	}))
	ret.nbioEngine.OnClose(wrapOnCloseRawUDPConn(func(u *rawUDPconn, err error) {
		log.Printf("onClose: [%+v, %v, %v]", u, u.conn.RemoteAddr().String(), err)
	}))
	// ret.messageHandleMap = []func(*rawUDPconn, *uptpHead, []byte){ret.handleV1Data}
	ret.appHandleFunc = make(map[uint32]func(*rawUDPconn, *uptpHead, []byte))
	ret.appHandleFunc[1] = ret.appid1handler
	ret.appHandleFunc[2] = ret.appid2handler
	// ret.appHandleFunc[3] = ret.appid3handler

	return ret
}

func (us *Uptps) handleRecvData(uptpConn *rawUDPconn, head *uptpHead, data []byte) {
	// us.messageHandleMap[head.Version-1](uptpConn, head, data)
	us.handleV1Data(uptpConn, head, data)
}

func (us *Uptps) handleV1Data(c *rawUDPconn, head *uptpHead, data []byte) {
	f, ok := us.appHandleFunc[head.AppID]
	if !ok {
		return
	}
	f(c, head, data)
}

func (us *Uptps) appid1handler(c *rawUDPconn, head *uptpHead, data []byte) {
	if head.From != 0 {
		//hearbeat
		c.SendMessage(0, head.From, 1, nil)
	}
	if head.Len == 0 {
		return
	}

	var ui UPTPInfo
	err := json.Unmarshal(data, &ui)
	if err != nil {
		log.Println("unmarshal uptp info fail: ", err)
		return
	}
	log.Printf("get client register: [%v], %+v", c.conn.RemoteAddr().String(), ui)
	if ui.PeerID == 0 {
		ui.PeerID = us.snowNode.Generate().Int64()
	}
	c.peerID = ui.PeerID
	ui.PublicIP = c.conn.RemoteAddr().(*net.UDPAddr).IP.String()
	us.peerMap.Store(ui.PeerID, ui)
	var idBytes [8]byte
	binary.LittleEndian.PutUint64(idBytes[:], uint64(ui.PeerID))
	err = c.SendMessage(0, 0, 1, idBytes[:])
	if err != nil {
		log.Println("send register response to client fail:", err)
	}
}

func (us *Uptps) appid2handler(c *rawUDPconn, head *uptpHead, data []byte) {
	var ui UPTPInfo
	err := json.Unmarshal(data, &ui)
	if err != nil {
		log.Println("[uptps:appid2handler] unmarshal request fail:", err)
	}
	log.Printf("get query request from: [%v,%v], %+v", c.conn.RemoteAddr().String(), head.From, ui)
	v, ok := us.peerMap.Load(ui.PeerID)
	if !ok {
		//write no found
		return
	}
	retInfo := v.(UPTPInfo)
	retInfo.Extra = ui.Extra
	peerInfo, err := json.Marshal(retInfo)
	if err != nil {
		log.Println("[uptps:appid2handler] marshal peer info fail:", err)
		return
	}
	err = c.SendMessage(0, head.From, 2, peerInfo)
	if err != nil {
		log.Println("[uptps:appid2handler] send query response to client fail:", err)
	}
	return
}

func (us *Uptps) funListenUDP(network string, laddr *net.UDPAddr) (*net.UDPConn, error) {
	log.Println("start listen ", laddr.String())
	if laddr.IP.Equal(net.IPv6zero) {
		return net.ListenUDP("udp6", laddr)
	}
	return net.ListenUDP(network, laddr)
}

func (us *Uptps) Start() {
	us.nbioEngine.Start()
}

func (us *Uptps) Stop() {
	us.nbioEngine.Stop()
}

func (us *Uptps) Wait() {
	us.nbioEngine.Wait()
}
