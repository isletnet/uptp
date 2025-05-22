package socks5

import (
	"io"
	"net"
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

func socks5RequestConnect(rw io.ReadWriter) error {
	r, err := socks5.NewRequestFrom(rw)
	if err != nil {
		return err
	}

	if r.Cmd != socks5.CmdConnect {
		if e := replyErr(r, rw, socks5.RepCommandNotSupported); err != nil {
			return e
		}
		return socks5.ErrUnsupportCmd
	}
	conn, err := net.DialTimeout("tcp", r.Address(), time.Second)
	if err != nil {
		if e := replyErr(r, rw, socks5.RepHostUnreachable); e != nil {
			return e
		}
		return err
	}

	defer conn.Close()
	a, addr, port, err := socks5.ParseAddress(conn.LocalAddr().String())
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
	return tunneling(conn, rw)
}

func socks5Negotiate(rw io.ReadWriter) error {
	rq, err := socks5.NewNegotiationRequestFrom(rw)
	if err != nil {
		return err
	}

	for _, m := range rq.Methods {
		if m == socks5.MethodNone {
			rp := socks5.NewNegotiationReply(socks5.MethodNone)
			_, err = rp.WriteTo(rw)
			return err
		}
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
