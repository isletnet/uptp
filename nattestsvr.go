package uptp

import (
	"time"

	"github.com/isletnet/uptp/logging"
	"github.com/lesismal/nbio"
	reuseport "github.com/libp2p/go-reuseport"
)

type NetTestSvr struct {
	nbg *nbio.Gopher
}

func NewNetTestSvr(prot string, addrs []string) *NetTestSvr {
	s := &NetTestSvr{}
	s.nbg = nbio.NewGopher(nbio.Config{
		Network:            prot,
		Addrs:              addrs,
		MaxWriteBufferSize: 1024,
		ReadBufferSize:     128,
		UDPReadTimeout:     time.Second,
		Listen:             reuseport.Listen,
	})

	// s.nbg.OnOpen(func(c *nbio.Conn) {
	// 	log.Printf("onOpen: [%p, %v]", c, c.RemoteAddr().String())
	// })
	s.nbg.OnData(func(c *nbio.Conn, data []byte) {
		logging.Info("onData: [%p, %v], %v", c, c.RemoteAddr().String(), string(data))
		c.Write([]byte(c.RemoteAddr().String()))
		c.Close()
	})
	return s
}

func (nts *NetTestSvr) Start() error {
	err := nts.nbg.Start()
	if err != nil {
		return err
	}
	go nts.run()
	return nil
}

func (nts *NetTestSvr) run() {
	defer nts.nbg.Stop()
	nts.nbg.Wait()
}
