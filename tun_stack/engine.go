package tunstack

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/core/device"
	"github.com/xjasonlyu/tun2socks/v2/dialer"
	"github.com/xjasonlyu/tun2socks/v2/log"
	"github.com/xjasonlyu/tun2socks/v2/proxy"
	"github.com/xjasonlyu/tun2socks/v2/tunnel"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

var (
	_engineMu sync.Mutex

	// _defaultKey holds the default key for the engine.
	_defaultKey *Key

	// _defaultDevice holds the default device for the engine.
	_defaultDevice device.Device

	// _defaultStack holds the default stack for the engine.
	_defaultStack *stack.Stack
)

// Start starts the default engine up.
func Start() {
	if err := start(); err != nil {
		log.Fatalf("[ENGINE] failed to start: %v", err)
	}
}

// Stop shuts the default engine down.
func Stop() {
	if err := stop(); err != nil {
		log.Fatalf("[ENGINE] failed to stop: %v", err)
	}
}

func start() error {
	_engineMu.Lock()
	defer _engineMu.Unlock()

	for _, f := range []func(*Key) error{
		general,
		netstack,
	} {
		if err := f(_defaultKey); err != nil {
			return err
		}
	}
	return nil
}

func stop() (err error) {
	_engineMu.Lock()
	if _defaultDevice != nil {
		_defaultDevice.Close()
	}
	if _defaultStack != nil {
		_defaultStack.Close()
		_defaultStack.Wait()
	}
	_engineMu.Unlock()
	return nil
}

// Insert loads *Key to the default engine.
func Insert(k *Key) {
	_engineMu.Lock()
	_defaultKey = k
	_engineMu.Unlock()
}

func general(k *Key) error {
	level, err := log.ParseLevel(k.LogLevel)
	if err != nil {
		return err
	}
	log.SetLogger(log.Must(log.NewLeveled(level)))

	if k.Interface != "" {
		iface, err := net.InterfaceByName(k.Interface)
		if err != nil {
			return err
		}
		dialer.DefaultDialer.InterfaceName.Store(iface.Name)
		dialer.DefaultDialer.InterfaceIndex.Store(int32(iface.Index))
		log.Infof("[DIALER] bind to interface: %s", k.Interface)
	}

	if k.Mark != 0 {
		dialer.DefaultDialer.RoutingMark.Store(int32(k.Mark))
		log.Infof("[DIALER] set fwmark: %#x", k.Mark)
	}

	if k.UDPTimeout > 0 {
		if k.UDPTimeout < time.Second {
			return errors.New("invalid udp timeout value")
		}
		tunnel.T().SetUDPTimeout(k.UDPTimeout)
	}
	return nil
}

func SetProxyDialer(dialer proxy.Dialer) {
	if dialer == nil {
		dialer = &proxy.Base{}
	}
	tunnel.T().SetDialer(dialer)
}

// type TunnelDialer struct{}

// func (td *TunnelDialer) DialContext(ctx context.Context, metadata *M.Metadata) (c net.Conn, err error) {
// 	if metadata == nil {
// 		return nil, errors.New("metadata is nil")
// 	}
// 	targetAddr := fmt.Sprintf("%s:%d", metadata.DstIP, metadata.DstPort)
// 	return socks5.DialContext(ctx, d.p2p.Libp2pHost(), d.pid, targetAddr)
// }
// func (td *TunnelDialer) DialUDP(metadata *M.Metadata) (_ net.PacketConn, err error) {
// 	if metadata == nil {
// 		return nil, errors.New("metadata is nil")
// 	}
// 	targetAddr := fmt.Sprintf("%s:%d", metadata.DstIP, metadata.DstPort)
// 	return socks5.DialUDP(context.Background(), d.p2p.Libp2pHost(), d.pid, targetAddr)
// }
