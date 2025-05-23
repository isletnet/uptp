package socks5

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
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
	errWritePacketLen = errors.New("write packet len failed")
	errReadPacketLen  = errors.New("read packet len failed")
	errPacketTooLarge = errors.New("packet too large")
)

type packetStream struct {
	rw io.ReadWriter
}

func (ps *packetStream) Write(p []byte) (n int, err error) {
	length := len(p)
	if length > 0xFFFF {
		return 0, errPacketTooLarge
	}
	if err := binary.Write(ps.rw, binary.BigEndian, uint16(length)); err == nil {
		return ps.rw.Write(p)
	}
	return 0, errWritePacketLen
}

func (ps *packetStream) Read(p []byte) (n int, err error) {
	var l uint16
	if err := binary.Read(ps.rw, binary.BigEndian, &l); err == nil {
		return io.ReadFull(ps.rw, p[:l])
	}
	return 0, errReadPacketLen
}
