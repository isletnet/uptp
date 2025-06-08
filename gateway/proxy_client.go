package gateway

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"sync"

	"github.com/isletnet/uptp/socks5"
	"github.com/isletnet/uptp/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
)

type socksOutbound struct {
	ID       types.ID `json:"id"`
	Open     bool     `json:"open"`
	Peer     string   `json:"peer"`
	PeerName string   `json:"peer_name"`
	Token    types.ID `json:"token"`
	Route    string   `json:"route"`

	routeNet   *net.IPNet          `json:"-"`
	socks5Peer socks5.PeerWithAuth `json:"-"`
}

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

func (mgr *socks5ProxyManager) GetOutbound(id types.ID) socksOutbound {
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	if mgr.outbounds == nil {
		mgr.outbounds = make(map[types.ID]*socksOutbound)
	}
	return *mgr.outbounds[id]
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
	obs := pc.ListOutbounds()
	for _, ob := range obs {
		if !ob.Open {
			continue
		}
		d := socks5.NewDialer(pc.h, ob.socks5Peer.ID, ob.socks5Peer.UserName, ob.socks5Peer.Password)
		pc.router.addRoute(ob.routeNet, d)
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
	d := socks5.NewDialer(pc.h, outbound.socks5Peer.ID, outbound.socks5Peer.UserName, outbound.socks5Peer.Password)
	pc.router.addRoute(outbound.routeNet, d)
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
