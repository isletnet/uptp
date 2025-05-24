package agent

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/isletnet/uptp/socks5"
	tunstack "github.com/isletnet/uptp/tun_stack"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
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
func (ag *agent) startTunProxy(tunDevice, peerID string) error {
	err := startTun2socks(tunDevice)
	if err != nil {
		return err
	}
	pid, err := peer.Decode(peerID)
	if err != nil {
		return err
	}
	tunstack.SetProxyDialer(&proxyDialer{
		h:   ag.p2p.Libp2pHost(),
		pid: pid,
	})
	return nil
}

func (ag *agent) stopTunProxy() error {
	tunstack.SetProxyDialer(nil)
	return stopTun2socks()
}

type proxyDialer struct {
	h   host.Host
	pid peer.ID
}

func (pd *proxyDialer) proxyRoute(metadata *M.Metadata) peer.ID {
	return pd.pid
}

func (pd *proxyDialer) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	if metadata == nil {
		return nil, errors.New("metadata is nil")
	}
	targetAddr := net.JoinHostPort(metadata.DstIP.String(), strconv.Itoa(int(metadata.DstPort)))
	pid := pd.proxyRoute(metadata)
	if pid == "" {
		return net.Dial("tcp", targetAddr)
	}
	return socks5.DialContext(ctx, pd.h, pid, targetAddr)
}

func (pd *proxyDialer) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	if metadata == nil {
		return nil, errors.New("metadata is nil")
	}
	targetAddr := fmt.Sprintf("%s:%d", metadata.DstIP, metadata.DstPort)
	pid := pd.proxyRoute(metadata)
	if pid == "" {
		ra, err := net.ResolveUDPAddr("udp", targetAddr)
		if err != nil {
			return nil, err
		}
		return net.DialUDP("udp", nil, ra)
	}
	return socks5.DialUDP(context.Background(), pd.h, pid, targetAddr)
}
