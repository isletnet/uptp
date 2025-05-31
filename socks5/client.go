package socks5

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/txthinking/socks5"
)

type PeerWithAuth struct {
	ID       peer.ID
	UserName []byte
	Password []byte
}

// NewDialer 创建并返回一个新的Dialer实例
func NewDialer(h host.Host, peerID peer.ID, username, password string) *Dialer {
	return &Dialer{
		h: h,
		peer: PeerWithAuth{
			ID:       peerID,
			UserName: []byte(username),
			Password: []byte(password),
		},
	}
}

type Dialer struct {
	h    host.Host
	peer PeerWithAuth
}

type ProxyAddr struct {
	network string
	address string
}

func (a *ProxyAddr) Network() string {
	return a.network
}

func (a *ProxyAddr) String() string {
	return a.address
}

func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.dialContext(ctx, network, address)
}

func (d *Dialer) DialUDPConn(network, address string) (net.PacketConn, error) {
	return d.dialContext(context.Background(), network, address)
}

func (d *Dialer) dialContext(ctx context.Context, network, address string) (*streamConn, error) {
	// 创建libp2p stream
	if len(network) < 3 {
		return nil, errors.New("unsupport network type")
	}
	s, err := d.h.NewStream(ctx, d.peer.ID, protocol.ID(socks5ID))
	if err != nil {
		return nil, err
	}

	// 方法协商
	nr := socks5.NewNegotiationRequest([]byte{socks5.MethodUsernamePassword})
	if _, err := nr.WriteTo(s); err != nil {
		s.Reset()
		return nil, err
	}

	// 读取服务器响应
	nreply, err := socks5.NewNegotiationReplyFrom(s)
	if err != nil {
		s.Reset()
		return nil, err
	}
	switch nreply.Method {
	case socks5.MethodUsernamePassword:
		urq := socks5.NewUserPassNegotiationRequest([]byte(d.peer.UserName), []byte(d.peer.Password))
		if _, err := urq.WriteTo(s); err != nil {
			return nil, err
		}
		urp, err := socks5.NewUserPassNegotiationReplyFrom(s)
		if err != nil {
			return nil, err
		}
		if urp.Status != socks5.UserPassStatusSuccess {
			return nil, socks5.ErrUserPassAuth
		}
	default:
		s.Reset()
		return nil, errors.New("no acceptable authentication methods")
	}

	// 解析目标地址
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		s.Reset()
		return nil, err
	}

	ip := net.ParseIP(host)
	var atyp byte
	var addr []byte
	if ip != nil {
		if ip.To4() != nil {
			atyp = socks5.ATYPIPv4
			addr = ip.To4()
		} else {
			atyp = socks5.ATYPIPv6
			addr = ip.To16()
		}
	} else {
		atyp = socks5.ATYPDomain
		addr = []byte(host)
	}

	portNum, err := net.LookupPort(network, port)
	if err != nil {
		s.Reset()
		return nil, err
	}
	p := make([]byte, 2)
	p[0] = byte(portNum >> 8)
	p[1] = byte(portNum)

	// 根据network参数选择命令类型
	cmd := socks5.CmdConnect
	if network[:3] == "udp" {
		cmd = CmdConnectUDP
	}

	// 构造连接请求
	req := socks5.NewRequest(cmd, atyp, addr, p)
	if _, err := req.WriteTo(s); err != nil {
		s.Reset()
		return nil, err
	}

	// 读取服务器响应
	reply, err := socks5.NewReplyFrom(s)
	if err != nil {
		s.Reset()
		return nil, err
	}

	if reply.Rep != socks5.RepSuccess {
		s.Reset()
		return nil, errors.New("SOCKS connect failed")
	}
	ra := ProxyAddr{
		network: network,
		address: address,
	}
	la := ProxyAddr{
		network: network,
		address: socks5.ToAddress(reply.Atyp, reply.BndAddr, reply.BndPort),
	}
	ret := &streamConn{
		Stream: s,
		la:     &la,
		ra:     &ra,
		rw:     s,
	}
	if network == "udp" {
		ret.rw = &packetReadWriter{rw: s}
	}
	return ret, nil
}

type streamConn struct {
	network.Stream
	rw io.ReadWriter
	la *ProxyAddr
	ra *ProxyAddr
}

func (c *streamConn) Read(b []byte) (n int, err error) {
	return c.rw.Read(b)
}

func (c *streamConn) Write(b []byte) (n int, err error) {
	return c.rw.Write(b)
}

func (c *streamConn) LocalAddr() net.Addr {
	return c.la
}

func (c *streamConn) RemoteAddr() net.Addr {
	return c.ra
}

func (c *streamConn) SetDeadline(t time.Time) error {
	return c.Stream.SetDeadline(t)
}

func (c *streamConn) SetReadDeadline(t time.Time) error {
	return c.Stream.SetReadDeadline(t)
}

func (c *streamConn) SetWriteDeadline(t time.Time) error {
	return c.Stream.SetWriteDeadline(t)
}

func (c *streamConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = c.rw.Read(p)
	addr = c.ra
	return
}

func (c *streamConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	n, err = c.rw.Write(p)
	return
}

func (d *Dialer) DialUDP(ctx context.Context, network, localAddr, remoteAddr string) (net.PacketConn, error) {
	// 创建libp2p stream
	if len(network) < 3 {
		return nil, errors.New("unsupport network type")
	}
	s, err := d.h.NewStream(ctx, d.peer.ID, protocol.ID(socks5ID))
	if err != nil {
		return nil, err
	}

	// 方法协商
	nr := socks5.NewNegotiationRequest([]byte{socks5.MethodUsernamePassword})
	if _, err := nr.WriteTo(s); err != nil {
		s.Reset()
		return nil, err
	}

	// 读取服务器响应
	nreply, err := socks5.NewNegotiationReplyFrom(s)
	if err != nil {
		s.Reset()
		return nil, err
	}
	switch nreply.Method {
	case socks5.MethodUsernamePassword:
		urq := socks5.NewUserPassNegotiationRequest([]byte(d.peer.UserName), []byte(d.peer.Password))
		if _, err := urq.WriteTo(s); err != nil {
			return nil, err
		}
		urp, err := socks5.NewUserPassNegotiationReplyFrom(s)
		if err != nil {
			return nil, err
		}
		if urp.Status != socks5.UserPassStatusSuccess {
			return nil, socks5.ErrUserPassAuth
		}
	default:
		s.Reset()
		return nil, errors.New("no acceptable authentication methods")
	}

	// 解析目标地址
	host, port, err := net.SplitHostPort(localAddr)
	if err != nil {
		s.Reset()
		return nil, err
	}

	ip := net.ParseIP(host)
	var atyp byte
	var addr []byte
	if ip != nil {
		if ip.To4() != nil {
			atyp = socks5.ATYPIPv4
			addr = net.IPv4zero
		} else {
			atyp = socks5.ATYPIPv6
			addr = net.IPv6zero
		}
	} else {
		atyp = socks5.ATYPDomain
		addr = []byte(host)
	}

	portNum, err := net.LookupPort(network, port)
	if err != nil {
		s.Reset()
		return nil, err
	}
	p := make([]byte, 2)
	p[0] = byte(portNum >> 8)
	p[1] = byte(portNum)

	// 发送PacketConn请求
	req := socks5.NewRequest(CmdPacketConn, atyp, addr, p)
	if _, err := req.WriteTo(s); err != nil {
		s.Reset()
		return nil, err
	}

	// 读取服务器响应
	reply, err := socks5.NewReplyFrom(s)
	if err != nil {
		s.Reset()
		return nil, err
	}

	if reply.Rep != socks5.RepSuccess {
		s.Reset()
		return nil, errors.New("SOCKS connect failed")
	}

	// 创建并返回PacketConn
	// ra := ProxyAddr{
	// 	network: network,
	// 	address: remoteAddr,
	// }
	la := ProxyAddr{
		network: network,
		address: socks5.ToAddress(reply.Atyp, reply.BndAddr, reply.BndPort),
	}
	return &packetConn{
		Stream: s,
		la:     &la,
		// ra:     &ra,
		rw: &packetReadWriter{rw: s},
	}, nil
}

type packetConn struct {
	network.Stream
	rw *packetReadWriter
	la *ProxyAddr
	// ra *ProxyAddr
}

func (c *packetConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	payload, from, err := socks5ReadFrom(p, c.rw)
	if err != nil {
		return 0, nil, err
	}
	n = copy(p, payload)
	addr = &ProxyAddr{
		network: c.la.Network(),
		address: from,
	}
	return n, addr, nil
}

func (c *packetConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	err = socks5WriteTo(p, addr.String(), c.rw)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *packetConn) LocalAddr() net.Addr {
	return c.la
}

func (c *packetConn) SetDeadline(t time.Time) error {
	return c.Stream.SetDeadline(t)
}

func (c *packetConn) SetReadDeadline(t time.Time) error {
	return c.Stream.SetReadDeadline(t)
}

func (c *packetConn) SetWriteDeadline(t time.Time) error {
	return c.Stream.SetWriteDeadline(t)
}
