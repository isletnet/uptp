package socks5

import (
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
