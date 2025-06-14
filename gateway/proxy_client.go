package gateway

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/isletnet/uptp/logging"
	"github.com/isletnet/uptp/socks5"
	tunstack "github.com/isletnet/uptp/tun_stack"
	"github.com/isletnet/uptp/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	M "github.com/xjasonlyu/tun2socks/v2/metadata"
)

type socksOutbound struct {
	ID       types.ID `json:"id"`
	Remark   string   `json:"remark"`
	Open     bool     `json:"open"`
	Peer     string   `json:"peer"`
	PeerName string   `json:"peer_name"`
	Token    types.ID `json:"token"`
	Route    string   `json:"route"`

	routeNet   *net.IPNet          `json:"-"`
	socks5Peer socks5.PeerWithAuth `json:"-"`
}

const (
	tunName   = "uptptun0"
	tunLocal  = "10.8.0.3/32"
	tunRemote = "10.8.0.254/32"
)

func socks5OutboundFillRunningInfo(ob *socksOutbound) error {
	pid, err := peer.Decode(ob.Peer)
	if err != nil {
		return err
	}

	_, n, err := net.ParseCIDR(ob.Route)
	if err != nil {
		return err
	}
	tokenBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(tokenBytes, ob.Token.Uint64())
	ob.routeNet = n
	ob.socks5Peer = socks5.PeerWithAuth{
		ID:       pid,
		UserName: tokenBytes,
		Password: tokenBytes,
	}
	return nil
}

var (
	keySocksOutbound = []byte("socks_outbound")
)

type socks5ProxyManager struct {
	db *leveldb.DB

	mtx sync.Mutex

	outbounds map[types.ID]*socksOutbound

	tunReady bool
}

func NewSocksOutboundsManager(db *leveldb.DB) (*socks5ProxyManager, error) {
	mgr := &socks5ProxyManager{
		db: db,
	}
	err := mgr.loadOutbounds()
	if err != nil {
		return nil, err
	}
	return mgr, nil
}

func (mgr *socks5ProxyManager) AddOutbound(outbound *socksOutbound) error {
	err := socks5OutboundFillRunningInfo(outbound)
	if err != nil {
		return err
	}
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	if mgr.outbounds == nil {
		mgr.outbounds = make(map[types.ID]*socksOutbound)
	}
	mgr.outbounds[outbound.ID] = outbound
	return mgr.saveOutbounds()
}

func (mgr *socks5ProxyManager) UpdateOutbound(outbound *socksOutbound) error {
	err := socks5OutboundFillRunningInfo(outbound)
	if err != nil {
		return err
	}
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	if mgr.outbounds == nil {
		mgr.outbounds = make(map[types.ID]*socksOutbound)
	}
	mgr.outbounds[outbound.ID] = outbound
	return mgr.saveOutbounds()
}

func (mgr *socks5ProxyManager) DeleteOutbound(id types.ID) (*socksOutbound, error) {
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	if mgr.outbounds == nil {
		return nil, nil
	}
	ob, ok := mgr.outbounds[id]
	if !ok {
		return nil, nil
	}
	delete(mgr.outbounds, id)
	return ob, mgr.saveOutbounds()
}

func (mgr *socks5ProxyManager) GetOutbound(id types.ID) *socksOutbound {
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	if mgr.outbounds == nil {
		mgr.outbounds = make(map[types.ID]*socksOutbound)
	}
	return mgr.outbounds[id]
}

func (mgr *socks5ProxyManager) ListOutbounds() []socksOutbound {
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	var outbounds []socksOutbound
	for _, ob := range mgr.outbounds {
		outbounds = append(outbounds, *ob)
	}
	return outbounds
}

func (mgr *socks5ProxyManager) loadOutbounds() error {
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	v, err := mgr.db.Get(keySocksOutbound, nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			return err
		}
		return nil
	}
	err = json.Unmarshal(v, &mgr.outbounds)
	if err != nil {
		return err
	}
	for _, ob := range mgr.outbounds {
		socks5OutboundFillRunningInfo(ob)
	}
	return nil
}

func (mgr *socks5ProxyManager) saveOutbounds() error {
	if mgr.outbounds == nil {
		return nil
	}
	buf, err := json.Marshal(mgr.outbounds)
	if err != nil {
		return err
	}
	return mgr.db.Put(keySocksOutbound, buf, nil)
}

type proxyClient struct {
	*socks5ProxyManager

	h host.Host

	router *proxyRouter
}

func newProxyClient(h host.Host, db *leveldb.DB) (*proxyClient, error) {
	obMgr, err := NewSocksOutboundsManager(db)
	if err != nil {
		return nil, err
	}
	return &proxyClient{
		socks5ProxyManager: obMgr,
		h:                  h,
		router:             newProxyRouter(),
	}, nil
}

func (pc *proxyClient) Start() {
	err := pc.startTunStack()
	if err != nil {
		logging.Error("start tun failed: %s", err)
		return
	}
	obs := pc.ListOutbounds()
	for _, ob := range obs {
		if !ob.Open {
			continue
		}
		d := socks5.NewDialer(pc.h, ob.socks5Peer.ID, ob.socks5Peer.UserName, ob.socks5Peer.Password)
		err := pc.addOutboundRoute(&ob, d)
		if err != nil {
			logging.Error("add route %s errors", ob.Route)
		}
	}
}

func (pc *proxyClient) AddOutbound(outbound *socksOutbound) error {
	peerName, err := pc.CheckPeer(outbound.Peer, outbound.Token)
	if err != nil {
		return err
	}
	outbound.PeerName = peerName
	err = pc.socks5ProxyManager.AddOutbound(outbound)
	if err != nil {
		return err
	}
	if !outbound.Open {
		return nil
	}

	if outbound.Open {
		d := socks5.NewDialer(pc.h, outbound.socks5Peer.ID, outbound.socks5Peer.UserName, outbound.socks5Peer.Password)
		err = pc.addOutboundRoute(outbound, d)
		if err != nil {
			logging.Error("add route %s errors", outbound.Route)
		}
	}
	return nil
}

func (pc *proxyClient) UpdateOutbound(outbound *socksOutbound) error {
	peerName, err := pc.CheckPeer(outbound.Peer, outbound.Token)
	if err != nil {
		return err
	}
	outbound.PeerName = peerName
	err = pc.socks5ProxyManager.UpdateOutbound(outbound)
	if err != nil {
		return err
	}
	if outbound.Open {
		d := socks5.NewDialer(pc.h, outbound.socks5Peer.ID, outbound.socks5Peer.UserName, outbound.socks5Peer.Password)
		err = pc.addOutboundRoute(outbound, d)
		if err != nil {
			logging.Error("add route %s errors", outbound.Route)
		}
	}
	return nil
}

func (pc *proxyClient) DeleteOutbound(id types.ID) error {
	ob, err := pc.socks5ProxyManager.DeleteOutbound(id)
	if err != nil {
		return err
	}
	if ob == nil {
		return nil
	}
	pc.router.delRoute(ob.routeNet)
	return nil
}

func (pc *proxyClient) CheckPeer(peer string, token types.ID) (string, error) {
	rsp, err := ResourceAuthorize(pc.h, peer, AuthorizeReq{
		ResourceID: types.ID(token),
	})
	if err != nil {
		return "", err
	}
	if rsp.Err != "" {
		return "", errors.New(rsp.Err)
	}
	return rsp.NodeName, nil
}

func (pc *proxyClient) addOutboundRoute(outbound *socksOutbound, d *socks5.Dialer) error {
	pc.router.addRoute(outbound.routeNet, d)
	return addRoute(outbound.Route, tunRemote)
}

func (pc *proxyClient) DeleteOutboundRoute(outbound *socksOutbound) error {
	pc.router.delRoute(outbound.routeNet)
	return delRoute(outbound.Route, tunRemote)
}

func (pc *proxyClient) startTunStack() error {
	k := &tunstack.Key{
		Device:     tunName,
		LogLevel:   "silent",
		UDPTimeout: time.Minute,
	}
	tunstack.Insert(k)
	tunstack.SetProxyDialer(pc)
	err := tunstack.Start()
	if err != nil {
		return err
	}
	return setTunAddr(tunName, tunLocal, tunRemote)
}

func (pc *proxyClient) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	if metadata.DstIP.Is6() {
		return nil, errors.New("not support v6")
	}
	dstIP := metadata.DstIP.As4()
	uip := uint32(dstIP[3]) | uint32(dstIP[2])<<8 | uint32(dstIP[1])<<16 | uint32(dstIP[0])<<24
	d := pc.router.get(uip)
	if d == nil {
		return nil, errors.New("no route found")
	}
	return d.DialContext(context.Background(), metadata.Addr().Network(), net.JoinHostPort(metadata.DstIP.String(), strconv.Itoa(int(metadata.DstPort))))
}

func (pc *proxyClient) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	return nil, errors.New("unsuport")
}
