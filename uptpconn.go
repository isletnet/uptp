package uptp

type UptpAddr struct {
	cid uint64
	app uint32
}

func (ua *UptpAddr) GetClientID() uint64 {
	return ua.cid
}

func ResolveUptpAddr(name string, app uint32) *UptpAddr {
	return &UptpAddr{
		cid: hashStringToInt(name),
		app: app,
	}
}

type uptpAppMsg struct {
	ra   UptpAddr
	data []byte
}

type UptpConn struct {
	la *UptpAddr

	readCh chan *uptpAppMsg

	onClose   func(uint32)
	writeFunc func(la, ra *UptpAddr, data []byte) error
	closed    bool
}

func newUptpConn() *UptpConn {
	return &UptpConn{
		readCh: make(chan *uptpAppMsg, 100),
	}
}

func (uconn *UptpConn) handle(from uint64, app uint32, data []byte) {
	if uconn.closed {
		return
	}
	uconn.readCh <- &uptpAppMsg{
		ra: UptpAddr{
			cid: from,
			app: app,
		},
		data: data,
	}
}

func (uconn *UptpConn) Write(to *UptpAddr, data []byte) error {
	return uconn.writeFunc(uconn.la, to, data)
}
func (uconn *UptpConn) Read() (*UptpAddr, []byte, error) {
	if uconn.closed {
		return nil, nil, ErrUptpConnClosed
	}
	msg, ok := <-uconn.readCh
	if !ok {
		return nil, nil, ErrUptpConnClosed
	}
	return &msg.ra, msg.data, nil
}
func (uconn *UptpConn) Close() {
	if uconn.closed {
		return
	}
	uconn.closed = true
	uconn.onClose(uconn.la.app)
	close(uconn.readCh)
}
