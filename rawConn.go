package uptp

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"
	"unsafe"

	"github.com/lesismal/nbio"
)

type uptpconn struct {
	conn      net.Conn
	checkSend uint32
	checkRecv uint32
	peerID    int64
	isClient  bool
}

func newUPTPConn(c net.Conn) *uptpconn {
	return &uptpconn{
		conn: c,
	}
}

func (uconn *uptpconn) checkMessage(data []byte) (*uptpHead, []byte, error) {
	head, content, err := UnmarshalUPTPMessage(data)
	if err != nil {
		return nil, nil, err
	}
	if int(head.Len) != len(content) {
		return nil, nil, fmt.Errorf("data len check fail")
	}
	return head, content, nil
}

func (uconn *uptpconn) sendMessage(from, to int64, appID uint32, content []byte) error {
	// if !uconn.ready && appID == 0 {
	// 	return fmt.Errorf("uptp connect is not ready to send message")
	// }
	sendData, err := marshalUPTPMessage(from, to, appID, uconn.checkSend, content)
	if err != nil {
		return fmt.Errorf("marshal message fail: %s", err)
	}
	_, err = uconn.conn.Write(sendData)
	if err != nil {
		return fmt.Errorf("send uptp message fail: %s", err)
	}
	return nil
}

func (uconn *uptpconn) close() error {
	return uconn.conn.Close()
}

func dialRawConn(addr string, eg *nbio.Engine) (*uptpconn, error) {
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	c, err := net.DialUDP("udp", nil, ua)
	if err != nil {
		return nil, err
	}
	checkRecv := uint32(uintptr(unsafe.Pointer(c)))
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, checkRecv)
	sendData, err := marshalUPTPMessage(0, 0, 0, 0, buf)
	if err != nil {
		c.Close()
		return nil, err
	}
	_, err = c.Write(sendData)
	if err != nil {
		c.Close()
		return nil, err
	}
	c.SetReadDeadline(time.Now().Add(time.Second * 10))
	var rsp = make([]byte, 1500)
	n, err := c.Read(rsp)
	if err != nil {
		c.Close()
		return nil, err
	}
	if n < sizeUPTPHead {
		c.Close()
		return nil, fmt.Errorf("wrong handshake response")
	}
	head, content, err := UnmarshalUPTPMessage(rsp[:n])
	if err != nil {
		c.Close()
		return nil, err
	}
	if int(head.Len) != len(content) {
		c.Close()
		return nil, fmt.Errorf("wrong handshake message")
	}
	if head.Len < 4 {
		c.Close()
		return nil, fmt.Errorf("wrong handshake len")
	}
	c.SetReadDeadline(time.Time{})
	u := binary.LittleEndian.Uint32(content)
	nc, err := eg.AddConn(c)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("known error add connect")
	}
	uptpConn := newUPTPConn(nc)
	uptpConn.checkSend = u
	uptpConn.checkRecv = checkRecv
	uptpConn.isClient = true
	nc.SetSession(uptpConn)
	return uptpConn, nil
}

func wrapOnOpen(h func(*uptpconn)) func(*nbio.Conn) {
	return func(c *nbio.Conn) {
		uptpConn := newUPTPConn(c)
		uptpConn.isClient = false
		uptpConn.checkRecv = uint32(uintptr(unsafe.Pointer(c)))
		c.SetSession(uptpConn)
		if h != nil {
			h(uptpConn)
		}
	}
}

func wrapOnClose(h func(*uptpconn, error)) func(*nbio.Conn, error) {
	return func(c *nbio.Conn, err error) {
		if h != nil && c != nil {
			if session := c.Session(); session != nil {
				if uptpConn, ok := session.(*uptpconn); ok {
					if h != nil {
						h(uptpConn, err)
					}
					uptpConn.conn = nil
				}
			}
		}
	}
}

func wrapOnData(h func(*uptpconn, *uptpHead, []byte), handshakeCheck func(*uptpHead, []byte) bool) func(*nbio.Conn, []byte) {
	return func(c *nbio.Conn, data []byte) {
		if session := c.Session(); session != nil {
			if uptpConn, ok := session.(*uptpconn); ok {
				head, content, err := uptpConn.checkMessage(data)
				if err != nil {
					c.CloseWithError(fmt.Errorf("uptp message check fail:%s", err))
					return
				}
				if int(head.Len) != len(content) {
					c.CloseWithError(fmt.Errorf("len check fail"))
					return
				}
				if head.AppID == 0 {
					//handshake
					if uptpConn.isClient {
						c.CloseWithError(fmt.Errorf("unexpected handshake message for client"))
						return
					}

					if head.Len < 4 {
						c.CloseWithError(fmt.Errorf("handshake message len check fail"))
						return
					}
					if handshakeCheck != nil && !handshakeCheck(head, content[4:]) {
						c.CloseWithError(fmt.Errorf("handshake check fail"))
						return
					}
					uptpConn.checkSend = binary.LittleEndian.Uint32(content[:4])
					buf := make([]byte, 4)
					binary.LittleEndian.PutUint32(buf, uptpConn.checkRecv)
					err = uptpConn.sendMessage(head.To, head.From, 0, buf)
					if err != nil {
						c.CloseWithError(fmt.Errorf("send handshake response fail:%s", err))
						return
					}
					return
				}

				if head.Check != uptpConn.checkRecv {
					c.CloseWithError(fmt.Errorf("message check fail"))
					return
				}
				if uptpConn.isClient {
					c.SetReadDeadline(time.Now().Add(time.Second * 30))
				}
				h(uptpConn, head, content)
			}
		}
	}
}
