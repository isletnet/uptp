package uptp

import (
	"encoding/binary"
	"fmt"
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

	messageHandleMap []func(*uptpconn, *uptpHead, []byte)
	appHandleFunc    map[uint32]func(*uptpconn, *uptpHead, []byte)

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
	ret.nbioEngine.OnData(wrapOnData(ret.handleRecvData, nil))
	ret.nbioEngine.OnOpen(wrapOnOpen(func(u *uptpconn) {
		// log.Printf("onOpen: %v", u.conn.RemoteAddr().String())
	}))
	ret.nbioEngine.OnClose(wrapOnClose(func(u *uptpconn, err error) {
		log.Printf("onClose: [%+v, %v, %v]", u, u.conn.RemoteAddr().String(), err)
	}))
	ret.messageHandleMap = []func(*uptpconn, *uptpHead, []byte){ret.handleV1Data}
	ret.appHandleFunc = make(map[uint32]func(*uptpconn, *uptpHead, []byte))
	ret.appHandleFunc[1] = ret.appid1handler
	ret.appHandleFunc[2] = ret.appid2handler
	// ret.appHandleFunc[3] = ret.appid3handler

	return ret
}

func (us *Uptps) handleRecvData(uptpConn *uptpconn, head *uptpHead, data []byte) {
	us.messageHandleMap[head.Version-1](uptpConn, head, data)
}

func (us *Uptps) handleV1Data(c *uptpconn, head *uptpHead, data []byte) {
	f, ok := us.appHandleFunc[head.AppID]
	if !ok {
		return
	}
	f(c, head, data)
}

func (us *Uptps) appid1handler(c *uptpconn, head *uptpHead, data []byte) {
	if head.From != 0 {
		//hearbeat
		c.sendMessage(0, head.From, 1, nil)
	}
	if head.Len != 12 {
		return
	}

	reqID := int64(binary.LittleEndian.Uint64(data[:8]))
	port := int(binary.LittleEndian.Uint32(data[8:]))
	log.Printf("get client register: [%v], %v, %v", c.conn.RemoteAddr().String(), reqID, port)
	if reqID == 0 {
		reqID = us.snowNode.Generate().Int64()
	}
	c.peerID = reqID
	ip := c.conn.RemoteAddr().(*net.UDPAddr).IP.String()
	addr := fmt.Sprintf("[%s]:%d", ip, port)
	us.peerMap.Store(reqID, addr)
	var idBytes [8]byte
	binary.LittleEndian.PutUint64(idBytes[:], uint64(reqID))
	err := c.sendMessage(0, 0, 1, idBytes[:])
	if err != nil {
		log.Println("send register response to client fail:", err)
	}
}

func (us *Uptps) appid2handler(c *uptpconn, head *uptpHead, data []byte) {
	id := int64(binary.LittleEndian.Uint64(data))
	log.Printf("get query request from: [%v,%v], %v", c.conn.RemoteAddr().String(), head.From, id)
	v, ok := us.peerMap.Load(id)
	if !ok {
		//write no found
		return
	}
	retAddr := v.(string)
	ret := append(data, []byte(retAddr)...)
	err := c.sendMessage(0, head.From, 2, ret)
	if err != nil {
		log.Println("send query response to client fail:", err)
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
