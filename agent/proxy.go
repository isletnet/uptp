package agent

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/isletnet/uptp/gateway"
	"github.com/isletnet/uptp/logging"
	"github.com/isletnet/uptp/socks5"
	"github.com/isletnet/uptp/types"

	tunstack "github.com/isletnet/uptp/tun_stack"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	M "github.com/xjasonlyu/tun2socks/v2/metadata"
)

func startTun2socks(tunDevice string) error {
	k := &tunstack.Key{
		Device:     tunDevice,
		LogLevel:   "debug",
		UDPTimeout: time.Minute,
	}
	tunstack.Insert(k)
	return tunstack.Start()
}
func stopTun2socks() error {
	return tunstack.Stop()
}

func (ag *agent) addProxyGateway(peerID string, token string) error {
	pid, err := peer.Decode(peerID)
	if err != nil {
		return err
	}
	uToken, err := strconv.ParseUint(token, 10, 64)
	if err != nil {
		return err
	}
	rsp, err := gateway.ResourceAuthorize(ag.p2p.Libp2pHost(), peerID, gateway.AuthorizeReq{
		Proxy: &gateway.AuthorizeProxyInfo{
			Token: types.ID(uToken),
		},
	})
	if err != nil {
		return err
	}
	if rsp.Err != "" {
		return errors.New(rsp.Err)
	}
	if rsp.Proxy == nil {
		return errors.New("proxy auth failed")
	}
	tokenByte := make([]byte, 8)
	binary.LittleEndian.PutUint64(tokenByte, uToken)
	return ag.proxyMgr.addProxy(&proxyGateway{
		peer: socks5.PeerWithAuth{
			ID:       pid,
			UserName: tokenByte,
			Password: tokenByte,
		},
		Name:   rsp.NodeName,
		PeerID: peerID,
		Token:  uToken,
		Route:  rsp.Proxy.Route,
		Dns:    rsp.Proxy.Dns,
	})
}

func (ag *agent) getProxyGatewayList() []proxyGateway {
	return ag.proxyMgr.getProxys()
}

func (ag *agent) startTunProxy(tunDevice string, gatewayIdx int) error {
	pg := ag.proxyMgr.getProxyByIndex(gatewayIdx)
	if pg == nil {
		return fmt.Errorf("gateway not found")
	}
	err := startTun2socks(tunDevice)
	if err != nil {
		return err
	}
	logging.Info("start proxy to gateway %s with %x", pg.peer.ID.ShortString(), pg.peer.Password)
	ag.p2p.DHT().ForceRefresh()
	tunstack.SetProxyDialer(&proxyDialer{
		dialer: socks5.NewDialer(ag.p2p.Libp2pHost(), pg.peer.ID, pg.peer.UserName, pg.peer.Password),
	})
	return nil
}

func (ag *agent) stopTunProxy() error {
	tunstack.SetProxyDialer(nil)
	return stopTun2socks()
}

func (ag *agent) pingProxyGateway(idx int) error {
	pg := ag.proxyMgr.getProxyByIndex(idx)
	if pg == nil {
		return fmt.Errorf("gateway not found")
	}
	err := ag.p2p.DHT().Ping(context.Background(), pg.peer.ID)
	if err != nil {
		return err
	}
	return nil
}

type proxyMgr struct {
	db     *leveldb.DB
	proxys []*proxyGateway
	mtx    sync.Mutex
}

var (
	keyProxys = []byte("proxys")
)

func (pm *proxyMgr) loadProxys() error {
	pm.mtx.Lock()
	defer pm.mtx.Unlock()
	v, err := pm.db.Get(keyProxys, nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			return err
		}
		return nil
	}
	err = json.Unmarshal(v, &pm.proxys)
	if err != nil {
		return err
	}
	for _, p := range pm.proxys {
		p.peer.ID, err = peer.Decode(p.PeerID)
		if err != nil {
			logging.Error("wrong proxy gateway id")
			continue
		}
		tokenBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(tokenBytes, p.Token)
		p.peer.Password = tokenBytes
		p.peer.UserName = tokenBytes
	}
	return nil
}

func (pm *proxyMgr) addProxy(p *proxyGateway) error {
	pm.mtx.Lock()
	defer pm.mtx.Unlock()
	pm.proxys = append(pm.proxys, p)
	return pm.saveProxys()
}
func (pm *proxyMgr) getProxyByIndex(idx int) *proxyGateway {
	pm.mtx.Lock()
	defer pm.mtx.Unlock()
	if len(pm.proxys) <= idx {
		return nil
	}
	return pm.proxys[idx]
}
func (pm *proxyMgr) getProxys() []proxyGateway {
	pm.mtx.Lock()
	defer pm.mtx.Unlock()
	var proxys []proxyGateway
	for _, p := range pm.proxys {
		proxys = append(proxys, *p)
	}
	return proxys
}

// func (pm *proxyMgr) delProxy(p *proxyGateway) error {
// 	pm.mtx.Lock()
// 	defer pm.mtx.Unlock()
// }

func (pm *proxyMgr) saveProxys() error {
	if pm.proxys == nil {
		return pm.db.Delete(keyProxys, nil)
	}
	buf, err := json.Marshal(pm.proxys)
	if err != nil {
		return err
	}
	err = pm.db.Put(keyProxys, buf, nil)
	if err != nil {
		return err
	}
	return nil
}

type proxyGateway struct {
	peer   socks5.PeerWithAuth `json:"-"`
	Name   string              `json:"name"`
	PeerID string              `json:"peer_id"`
	Token  uint64              `json:"token"`
	Route  string              `json:"route"`
	Dns    string              `json:"dns"`
}

type proxyDialer struct {
	dialer *socks5.Dialer
}

// func (pd *proxyDialer) proxyRoute(metadata *M.Metadata) peer.ID {
// 	return pd.peer.ID
// }

func (pd *proxyDialer) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	// logging.Debug("proxy dialer dial context: %+v", metadata)
	if metadata == nil {
		return nil, errors.New("metadata is nil")
	}
	targetAddr := net.JoinHostPort(metadata.DstIP.String(), strconv.Itoa(int(metadata.DstPort)))
	ret, err := pd.dialer.DialContext(ctx, metadata.Addr().Network(), targetAddr)
	if err != nil {
		logging.Error("proxy dialer dial context error: %v", err)
		return nil, err
	}
	return ret, nil
}

func (pd *proxyDialer) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	if metadata == nil {
		return nil, errors.New("metadata is nil")
	}
	targetAddr := fmt.Sprintf("%s:%d", metadata.DstIP, metadata.DstPort)
	return pd.dialer.DialUDPConn(metadata.Network.String(), targetAddr)
}
