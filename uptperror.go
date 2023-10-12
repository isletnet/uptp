package uptp

import "errors"

var (
	ErrAddTunnelBusy  = errors.New("add tunnel bsy")
	ErrUptpConnClosed = errors.New("uptpconn is closed")
)
