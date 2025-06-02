package p2pengine

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"strconv"

	dsync "github.com/ipfs/go-datastore/sync"
	levelds "github.com/ipfs/go-ds-leveldb"
	lplog "github.com/ipfs/go-log/v2"
	"github.com/isletnet/uptp/logging"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	rhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/multiformats/go-multiaddr"
	// "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
)

type P2PEngine struct {
	rhost *rhost.RoutedHost
	dht   *dht.IpfsDHT

	realPort int

	// rs    *relay.Relay
	// relayPeer peer.ID
	// resv      *circuit.Reservation
}

// func GetRelayResources() relay.Resources {
// 	res := relay.DefaultResources()
// 	res.ReservationTTL = 600 * time.Second
// 	res.MaxReservationsPerIP = 5
// 	res.MaxReservations = 100
// 	res.MaxCircuits = 500
// 	res.BufferSize = 4 * 1024
// 	res.Limit = nil
// 	return res
// }

var lplogger = lplog.Logger("uptp")

func NewP2PEngine(listenPort int, seed []byte, logFile, dhtDBPath string, clentMode bool, bf func() []string) (*P2PEngine, error) {
	os.Remove(logFile)
	if len(seed) < ed25519.SeedSize {
		return nil, errors.New("wrong seed")
	}
	var err error
	var h host.Host
	defer func() {
		if err != nil && h != nil {
			h.Close()
		}
	}()

	var ds *levelds.Datastore
	if dhtDBPath != "" {
		ds, err = levelds.NewDatastore(dhtDBPath, nil)
		if err != nil {
			return nil, err
		}
	}

	lplConfig := lplog.GetConfig()
	lplConfig.Stderr = false
	lplConfig.Stdout = false
	lplConfig.File = logFile
	lplConfig.Level = lplog.LevelWarn
	lplog.SetupLogging(lplConfig)
	lplog.SetLogLevel("dht", "error")
	lplog.SetLogLevel("uptp", "info")

	priv, _, err := crypto.GenerateEd25519Key(bytes.NewBuffer(seed[:ed25519.SeedSize]))
	if err != nil {
		return nil, err
	}
	ret := P2PEngine{}
	ipv6BlackHoleSC := &swarm.BlackHoleSuccessCounter{N: 100, MinSuccesses: 5, Name: "IPv6"}
	opts := []libp2p.Option{
		libp2p.Security(noise.ID, NewSessionTransport),
		libp2p.Identity(priv),
		libp2p.IPv6BlackHoleSuccessCounter(ipv6BlackHoleSC),
		libp2p.EnableRelay(),
		// libp2p.AddrsFactory(ret.addrsFactory),
		libp2p.DefaultTransports,
	}
	if clentMode {
		opts = append(opts, func(cfg *libp2p.Config) error {
			cfg.DisableIdentifyAddressDiscovery = true
			return nil
		}, libp2p.NoListenAddrs)
	} else {
		rm, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(rcmgr.InfiniteLimits))
		if err != nil {
			lplogger.Error("create infinite limiter resource manager error: %s", err)
		} else {
			opts = append(opts, libp2p.ResourceManager(rm))
		}
		opts = append(opts, libp2p.ListenAddrStrings(
			fmt.Sprintf("/ip6/::/tcp/%d", listenPort),
			// "/ip6/::/udp/0/quic-v1",
		))
	}

	h, err = libp2p.New(opts...)
	if err != nil {
		return nil, err
	}
	var realPort int
	for _, addr := range h.Addrs() {
		// 检查是否为 IPv6 地址
		_, err := addr.ValueForProtocol(multiaddr.P_IP6)
		if err != nil {
			continue // 不是 IPv6 地址
		}

		// 提取 TCP 端口
		if tcp, err := addr.ValueForProtocol(multiaddr.P_TCP); err == nil {
			realPort, err = strconv.Atoi(tcp)
			if err != nil {
				logging.Error("parse port from multi addr %s error: %s", addr.String(), err)
				continue
			}
			break
		}
	}

	// rs, err := relay.New(h, relay.WithInfiniteLimits(), relay.WithResources(GetRelayResources()))
	// if err != nil {
	// 	return nil, err
	// }

	// 启用 DHT
	dhtMode := dht.ModeServer
	if clentMode {
		dhtMode = dht.ModeClient
	}

	bootstrapFunc := func() []peer.AddrInfo {
		lplogger.Info("bootstrap peers func is called")
		var bootstrapAddr []peer.AddrInfo
		//重置ipv6 black hole计数
		ipv6BlackHoleSC.RecordResult(true)
		for _, b := range bf() {
			addrInfo, err := peer.AddrInfoFromString(b)
			if err != nil {
				lplogger.Error("parse bootstrap addr %s error: %s", b, err)
				continue
			}
			bootstrapAddr = append(bootstrapAddr, *addrInfo)
		}
		return bootstrapAddr
	}

	dhtOpt := []dht.Option{
		dht.ProtocolPrefix("/uptp"),
		dht.Mode(dhtMode),
		dht.BootstrapPeersFunc(bootstrapFunc)}

	if ds != nil {
		dstore := dsync.MutexWrap(ds)
		dhtOpt = append(dhtOpt, dht.Datastore(dstore))
	}

	kademliaDHT, err := dht.New(context.Background(), h, dhtOpt...)
	if err != nil {
		return nil, err
	}

	// Make the routed host
	routedHost := rhost.Wrap(h, kademliaDHT)

	ret.rhost = routedHost
	ret.dht = kademliaDHT
	ret.realPort = realPort
	go ret.listenAndHandleConnectEvent()
	// ret.rs = rs
	// go ret.background()
	return &ret, nil
}

func (pe *P2PEngine) listenAndHandleConnectEvent() {
	sub, err := pe.Libp2pHost().EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
	if err != nil {
		lplogger.Error("subscribe connectedness change event error: %s", err)
	}
	for {
		evt := <-sub.Out()
		connEvt := evt.(event.EvtPeerConnectednessChanged)
		if connEvt.Connectedness == network.Connected {
			lplogger.Debug("peer %s connected", connEvt.Peer.ShortString())
			// pe.Libp2pHost().ConnManager().Protect(connEvt.Peer, "connected")
		} else {
			lplogger.Debug("peer %s disconnected", connEvt.Peer.ShortString())
			pe.Libp2pHost().Peerstore().RemovePeer(connEvt.Peer)
		}
	}

}

func (pe *P2PEngine) Libp2pHost() host.Host {
	return pe.rhost
}

func (pe *P2PEngine) GetListenPort() int {
	return pe.realPort
}

func (pe *P2PEngine) DHT() *dht.IpfsDHT {
	return pe.dht
}

func (pe *P2PEngine) Close() error {
	pe.dht.Host().Close()
	return pe.dht.Close()
}

// func (pe *P2PEngine) addrsFactory(mas []ma.Multiaddr) []ma.Multiaddr {
// 	lplogger.Debug("addrs factory func is called")
// 	var ret []ma.Multiaddr
// 	for _, ma := range mas {
// 		if manet.IsIPLoopback(ma) {
// 			continue
// 		}
// 		ret = append(ret, ma)
// 	}
// 	if pe.resv == nil {
// 		return ret
// 	}
// 	relayAddr := ma.StringCast(fmt.Sprintf("/p2p/%s/p2p-circuit", pe.relayPeer))
// 	ret = append(ret, relayAddr)
// 	return ret
// }

// func (pe *P2PEngine) background() {
// 	tk := time.NewTicker(time.Minute)
// 	for {
// 		<-tk.C
// 		pe.checkRelay()
// 		err := pe.dht.Provide(context.Background(), peer.ToCid(pe.rhost.ID()), true)
// 		if err != nil {
// 			lplogger.Warnf("dht provide error: %s", err)
// 		}
// 	}
// }

// func (pe *P2PEngine) checkRelay() {
// 	if pe.relayPeer == "" {
// 		return
// 	}
// 	if pe.resv == nil || time.Now().Add(time.Minute*2).Before(pe.resv.Expiration) {
// 		res, err := circuit.Reserve(context.Background(), pe.rhost, peer.AddrInfo{ID: pe.relayPeer})
// 		if err != nil {
// 			lplogger.Warnf("circuit reserve on %s error: %s", pe.relayPeer, err)
// 			pe.resv = nil
// 			return
// 		}
// 		pe.resv = res
// 	}
// }

// func (pe *P2PEngine) SetRelayAddr(pid string) error {
// 	if pid == "" {
// 		pe.relayPeer = ""
// 		pe.resv = nil
// 		return nil
// 	}
// 	p, err := peer.Decode(pid)
// 	if err != nil {
// 		return err
// 	}
// 	if p == pe.relayPeer {
// 		return nil
// 	}
// 	pe.relayPeer = p
// 	pe.resv = nil
// 	return nil
// 	// for _, addr := range addrs {
// 	// 	pub := addr.Encapsulate(circuit)
// 	// 	raddrs = append(raddrs, pub)
// 	// }
// }
