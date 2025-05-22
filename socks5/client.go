package socks5

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/txthinking/socks5"
)

func DialContext(ctx context.Context, h host.Host, peerID peer.ID, targetAddr string) (net.Conn, error) {
	// 创建libp2p stream
	s, err := h.NewStream(ctx, peerID, protocol.ID(socks5ID))
	if err != nil {
		return nil, err
	}

	// 方法协商
	nr := socks5.NewNegotiationRequest([]byte{socks5.MethodNone})
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

	if nreply.Method != socks5.MethodNone {
		s.Reset()
		return nil, errors.New("no acceptable authentication methods")
	}

	// 解析目标地址
	host, port, err := net.SplitHostPort(targetAddr)
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

	portNum, err := net.LookupPort("tcp", port)
	if err != nil {
		s.Reset()
		return nil, err
	}
	p := make([]byte, 2)
	p[0] = byte(portNum >> 8)
	p[1] = byte(portNum)

	// 构造连接请求
	req := socks5.NewRequest(socks5.CmdConnect, atyp, addr, p)
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

	return &streamConn{Stream: s}, nil
}

type streamConn struct {
	network.Stream
}

func (c *streamConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func (c *streamConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
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
