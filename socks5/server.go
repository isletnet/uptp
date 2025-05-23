package socks5

import (
	"io"
	"net"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/isletnet/uptp/logging"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/txthinking/socks5"
)

const (
	socks5ID = "/socks5/1.0.0"
)

const (
	CmdConnectUDP byte = 0x04
)

func StartServe(h host.Host) {
	h.SetStreamHandler(protocol.ID(socks5ID), handler)
}
func StopServe(h host.Host) {
	h.RemoveStreamHandler(protocol.ID(socks5ID))
}
func handler(s network.Stream) {
	if err := socks5Negotiate(s); shouldLogError(err) {
		logging.Error("socks5Negotiate err: %v", err)
		return
	}
	if err := socks5RequestConnect(s); shouldLogError(err) {
		logging.Error("socks5RequestConnect err: %v", err)
		return
	}
}

func socks5RequestConnect(rw io.ReadWriteCloser) error {
	defer rw.Close()
	r, err := socks5.NewRequestFrom(rw)
	if err != nil {
		return err
	}

	var targetConn io.ReadWriteCloser
	var sourceConn io.ReadWriter
	var localAddr string

	switch r.Cmd {
	case socks5.CmdConnect:
		conn, err := net.DialTimeout("tcp", r.Address(), time.Second)
		if err != nil {
			if e := replyErr(r, rw, socks5.RepHostUnreachable); e != nil {
				return e
			}
			return err
		}
		localAddr = conn.LocalAddr().String()
		targetConn = conn
		sourceConn = rw
	case CmdConnectUDP:
		ra, err := net.ResolveUDPAddr("udp", r.Address())
		if err != nil {
			if e := replyErr(r, rw, socks5.RepAddressNotSupported); e != nil {
				return e
			}
			return err
		}
		conn, err := net.DialUDP("udp", nil, ra)
		if err != nil {
			if e := replyErr(r, rw, socks5.RepHostUnreachable); e != nil {
				return e
			}
			return err
		}
		localAddr = conn.LocalAddr().String()
		targetConn = conn
		sourceConn = &packetStream{rw: rw}
	default:
		if e := replyErr(r, rw, socks5.RepCommandNotSupported); e != nil {
			return e
		}
		return socks5.ErrUnsupportCmd
	}

	defer targetConn.Close()
	a, addr, port, err := socks5.ParseAddress(localAddr)
	if err != nil {
		if e := replyErr(r, rw, socks5.RepHostUnreachable); e != nil {
			return e
		}
		return err
	}

	reply := socks5.NewReply(socks5.RepSuccess, a, addr, port)
	if _, err := reply.WriteTo(rw); err != nil {
		return err
	}
	return tunneling(targetConn, sourceConn)
}

func socks5Negotiate(rw io.ReadWriter) error {
	rq, err := socks5.NewNegotiationRequestFrom(rw)
	if err != nil {
		return err
	}

	if slices.Contains(rq.Methods, socks5.MethodNone) {
		rp := socks5.NewNegotiationReply(socks5.MethodNone)
		_, err = rp.WriteTo(rw)
		return err
	}

	rp := socks5.NewNegotiationReply(socks5.MethodUnsupportAll)
	_, err = rp.WriteTo(rw)
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
