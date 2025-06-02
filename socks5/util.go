package socks5

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"

	"github.com/txthinking/socks5"
)

func tunneling(dst, src io.ReadWriter) (err error) {
	waitCh := make(chan struct{})
	var once sync.Once
	endFunc := func(e error) {
		once.Do(func() {
			err = e
			close(waitCh)
		})
	}
	go forward(dst, src, endFunc)
	go forward(src, dst, endFunc)
	// Wait
	<-waitCh
	return
}

func forward(dst, src io.ReadWriter, end func(error)) {
	_, e := io.Copy(dst, src)
	end(e)
}

var (
	errPacketTooLarge = errors.New("packet too large")
)

type packetReadWriter struct {
	rw io.ReadWriter
}

func (ps *packetReadWriter) Write(p []byte) (n int, err error) {
	length := len(p)
	if length > 0xFFFF {
		return 0, errPacketTooLarge
	}
	if err = binary.Write(ps.rw, binary.BigEndian, uint16(length)); err == nil {
		return ps.rw.Write(p)
	}
	return 0, err
}

func (ps *packetReadWriter) Read(p []byte) (n int, err error) {
	var l uint16
	if err = binary.Read(ps.rw, binary.BigEndian, &l); err == nil {
		return io.ReadFull(ps.rw, p[:l])
	}
	return 0, err
}

func socks5ReadFrom(p []byte, rd io.Reader) (payload []byte, from string, err error) {
	n, err := rd.Read(p)
	if err != nil {
		return
	}
	a, addr, port, err := socks5.ParseBytesAddress(p)
	if err != nil {
		return
	}
	from = socks5.ToAddress(a, addr, port)
	payload = p[1+len(addr)+2 : n]
	return
}

func socks5WriteTo(p []byte, to string, w io.Writer) (err error) {
	a, addr, port, err := socks5.ParseAddress(to)
	if err != nil {
		return
	}
	_, err = w.Write(append(append(append([]byte{a}, addr...), port...), p...))
	return
}
