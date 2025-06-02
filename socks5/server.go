package socks5

import (
	"encoding/binary"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/lesismal/nbio/logging"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/txthinking/socks5"
)

const (
	socks5ID = "/socks5/1.0.0"

	socks5ConnectSessionKey = "ss5csk"
)

const (
	CmdConnectUDP byte = 0x04
	CmdPacketConn byte = 0x05
)

var authFunc func(pid peer.ID, urq *socks5.UserPassNegotiationRequest) bool
var preCheckFunc func(pid peer.ID) bool

var AuthFunc func(authID uint64) bool

type socks5SessionInfo struct {
}

// var proxyDialer proxy.Dialer
var obDialer outboundDialer = &directDialer{}

type outboundDialer interface {
	Dial(network, address string) (net.Conn, error)
	DialUDP(network, address string) (net.Conn, error)
}

type socks5Dialer struct {
	c *socks5.Client
}

func (sd *socks5Dialer) Dial(network, address string) (net.Conn, error) {
	return sd.c.Dial(network, address)
}

func (sd *socks5Dialer) DialUDP(network, address string) (net.Conn, error) {
	return sd.c.Dial(network, address)
}

type directDialer struct{}

func (dd *directDialer) Dial(network, address string) (net.Conn, error) {
	return net.DialTimeout(network, address, time.Second*30)
}

func (dd *directDialer) DialUDP(network, address string) (net.Conn, error) {
	ra, err := net.ResolveUDPAddr(network, address)
	if err != nil {
		return nil, err
	}
	return net.DialUDP(network, nil, ra)
}

func SetOutboundProxy(addr, user, pass string) {
	if addr == "" {
		obDialer = &directDialer{}
		return
	}

	cli, _ := socks5.NewClient(addr, user, pass, 30, 10)
	obDialer = &socks5Dialer{
		c: cli,
	}
}
func StartServe(h host.Host, af func(authID uint64) bool) {
	h.SetStreamHandler(protocol.ID(socks5ID), handler)

	AuthFunc = af
	authFunc = socks5Auth(h)
	preCheckFunc = socks5Precheck(h)
}
func StopServe(h host.Host) {
	h.RemoveStreamHandler(protocol.ID(socks5ID))
}
func handler(s network.Stream) {
	if err := socks5Negotiate(s, preCheckFunc(s.Conn().RemotePeer())); shouldLogError(err) {
		logging.Error("socks5Negotiate err: %v", err)
		return
	}
	if err := socks5RequestConnect(s); shouldLogError(err) {
		logging.Error("socks5RequestConnect err: %v", err)
		return
	}
}

func socks5RequestConnect(rwc io.ReadWriteCloser) error {
	defer rwc.Close()
	req, err := socks5.NewRequestFrom(rwc)
	if err != nil {
		return err
	}

	var targetConn io.ReadWriteCloser
	var sourceConn io.ReadWriter
	var localAddr string

	switch req.Cmd {
	case socks5.CmdConnect:
		var conn net.Conn
		conn, err := obDialer.Dial("tcp", req.Address())
		if err != nil {
			if e := replyErr(req, rwc, socks5.RepHostUnreachable); e != nil {
				return e
			}
			return err
		}
		localAddr = conn.LocalAddr().String()
		targetConn = conn
		sourceConn = rwc
	case CmdConnectUDP:
		conn, err := obDialer.DialUDP("udp", req.Address())
		if err != nil {
			if e := replyErr(req, rwc, socks5.RepHostUnreachable); e != nil {
				return e
			}
			return err
		}
		logging.Debug("udp connected to %v", req.Address())
		localAddr = conn.LocalAddr().String()
		targetConn = conn
		sourceConn = &packetReadWriter{rw: rwc}
	case CmdPacketConn:
		return handleUDPPackConnRequest(req, rwc)
	default:
		if e := replyErr(req, rwc, socks5.RepCommandNotSupported); e != nil {
			return e
		}
		return socks5.ErrUnsupportCmd
	}

	defer targetConn.Close()
	a, addr, port, err := socks5.ParseAddress(localAddr)
	if err != nil {
		if e := replyErr(req, rwc, socks5.RepHostUnreachable); e != nil {
			return e
		}
		return err
	}

	reply := socks5.NewReply(socks5.RepSuccess, a, addr, port)
	if _, err := reply.WriteTo(rwc); err != nil {
		return err
	}
	return tunneling(targetConn, sourceConn)
}

func socks5Precheck(h host.Host) func(peer.ID) bool {
	return func(pid peer.ID) bool {
		v, _ := h.Peerstore().Get(pid, socks5ConnectSessionKey)
		if v == nil {
			return true
		}
		_, ok := v.(*socks5SessionInfo)
		if !ok {
			return true
		}
		//todo check session
		return false
	}
}

func socks5Auth(h host.Host) func(pid peer.ID, urq *socks5.UserPassNegotiationRequest) bool {
	return func(pid peer.ID, urq *socks5.UserPassNegotiationRequest) bool {
		//todo call authorize
		if urq.Plen < 8 {
			return false
		}
		authID := binary.LittleEndian.Uint64(urq.Passwd[:8])
		if !AuthFunc(authID) {
			return false
		}
		h.Peerstore().Put(pid, socks5ConnectSessionKey, &socks5SessionInfo{})
		return true
	}
}

func handleUDPPackConnRequest(req *socks5.Request, rw io.ReadWriter) error {
	ua, err := net.ResolveUDPAddr("udp", req.Address())
	if err != nil {
		if e := replyErr(req, rw, socks5.RepAddressNotSupported); e != nil {
			return e
		}
		return err
	}
	conn, err := net.ListenUDP("udp", ua)
	if err != nil {
		if e := replyErr(req, rw, socks5.RepHostUnreachable); e != nil {
			return e
		}
		return err
	}
	defer conn.Close()
	a, addr, port, err := socks5.ParseAddress(conn.LocalAddr().String())
	if err != nil {
		if e := replyErr(req, rw, socks5.RepHostUnreachable); e != nil {
			return e
		}
		reply := socks5.NewReply(socks5.RepSuccess, a, addr, port)
		if _, err := reply.WriteTo(rw); err != nil {
			return err
		}
		return err
	}
	sourceConn := &packetReadWriter{rw: rw}
	stopped := false
	go func() {
		tunnelBuf := make([]byte, 32*1024)
		for {
			payload, to, err := socks5ReadFrom(tunnelBuf, sourceConn)
			if err != nil {
				logging.Error("udp pack conn read socks5 pack err: %s", err)
				break
			}
			ra, err := net.ResolveUDPAddr("udp", to)
			if err != nil {
				logging.Error("udp pack conn parse to addr err: %s", err)
				continue
			}
			_, err = conn.WriteTo(payload, ra)
			if err != nil {
				logging.Error("udp pack conn forward to udp err: %s", err)
				continue
			}
		}
		stopped = true
	}()
	connBuf := make([]byte, 32*1024)
	for !stopped {
		conn.SetReadDeadline(time.Now().Add(time.Minute * 2))
		n, ra, err := conn.ReadFrom(connBuf)
		if err != nil {
			logging.Error("udp pack conn read err: %s", err)
			break
		}
		err = socks5WriteTo(connBuf[:n], ra.String(), sourceConn)
		if err != nil {
			logging.Error("udp pack conn forward to socks err: %s", err)
			break
		}
	}
	return nil
}

func socks5Negotiate(s network.Stream, needAuth bool) error {
	_, err := socks5.NewNegotiationRequestFrom(s)
	if err != nil {
		return err
	}

	if needAuth {
		rp := socks5.NewNegotiationReply(socks5.MethodUsernamePassword)
		_, err = rp.WriteTo(s)
		if err != nil {
			return err
		}

		urq, err := socks5.NewUserPassNegotiationRequestFrom(s)
		if err != nil {
			return err
		}
		if !authFunc(s.Conn().RemotePeer(), urq) {
			urp := socks5.NewUserPassNegotiationReply(socks5.UserPassStatusFailure)
			if _, err := urp.WriteTo(s); err != nil {
				return err
			}
			return socks5.ErrUserPassAuth
		}
		urp := socks5.NewUserPassNegotiationReply(socks5.UserPassStatusSuccess)
		if _, err := urp.WriteTo(s); err != nil {
			return err
		}
		return nil
	}

	rp := socks5.NewNegotiationReply(socks5.MethodNone)
	_, err = rp.WriteTo(s)
	return err
}

func replyErr(req *socks5.Request, rw io.ReadWriter, rep byte) error {
	var reply *socks5.Reply
	if req.Atyp == socks5.ATYPIPv4 || req.Atyp == socks5.ATYPDomain {
		reply = socks5.NewReply(rep, socks5.ATYPIPv4, []byte{0x00, 0x00, 0x00, 0x00}, []byte{0x00, 0x00})
	} else {
		reply = socks5.NewReply(rep, socks5.ATYPIPv6, []byte(net.IPv6zero), []byte{0x00, 0x00})
	}
	_, err := reply.WriteTo(rw)
	return err
}

func shouldLogError(err error) bool {
	return err != nil && err != io.EOF &&
		err != io.ErrUnexpectedEOF && err != syscall.ECONNRESET &&
		!strings.Contains(err.Error(), "timeout") &&
		!strings.Contains(err.Error(), "reset") &&
		!strings.Contains(err.Error(), "closed")
}
