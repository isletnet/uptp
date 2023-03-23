package uptp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/lesismal/nbio"
)

type rawUDPconn struct {
	conn      net.Conn
	checkSend uint32
	checkRecv uint32
	peerID    int64
	isClient  bool
	rspTime   int64

	wMtx     sync.Mutex
	writeBuf *bytes.Buffer
}

func newRawUDPConn(c net.Conn) *rawUDPconn {
	buf := bytes.NewBuffer(nil)
	buf.Grow(1600)
	return &rawUDPconn{
		conn:     c,
		writeBuf: buf,
	}
}

func (uconn *rawUDPconn) checkMessage(data []byte) (*uptpHead, uint32, []byte, error) {
	if len(data) < 4 {
		return nil, 0, nil, fmt.Errorf("wrong packet: to small")
	}
	check := binary.LittleEndian.Uint32(data[:4])
	head, content, err := UnmarshalUPTPMessage(data[4:])
	if err != nil {
		return nil, 0, nil, err
	}
	if int(head.Len) != len(content) {
		return nil, 0, nil, fmt.Errorf("data len check fail")
	}
	return head, check, content, nil
}

func (uconn *rawUDPconn) sendMessage(from, to int64, appID uint32, content []byte) error {
	// if !uconn.ready && appID == 0 {
	// 	return fmt.Errorf("uptp connect is not ready to send message")
	// }
	uconn.wMtx.Lock()
	defer uconn.wMtx.Unlock()
	err := binary.Write(uconn.writeBuf, binary.LittleEndian, uconn.checkSend)
	if err != nil {
		return fmt.Errorf("write check to buffer fail:%s", err)
	}
	err = marshalUPTPMessageToBuffer(from, to, appID, content, uconn.writeBuf)
	if err != nil {
		return fmt.Errorf("write uptp message to buffer fail: %s", err)
	}
	_, err = uconn.conn.Write(uconn.writeBuf.Bytes())
	if err != nil {
		return fmt.Errorf("send uptp message fail: %s", err)
	}
	uconn.writeBuf.Reset()
	return nil
}

func (uconn *rawUDPconn) close() error {
	return uconn.conn.Close()
}

func dialRawUDPConn(addr string, eg *nbio.Engine) (*rawUDPconn, error) {
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
	sendBuf := bytes.NewBuffer(nil)
	err = binary.Write(sendBuf, binary.LittleEndian, uint32(0))
	if err != nil {
		c.Close()
		return nil, err
	}
	err = marshalUPTPMessageToBuffer(0, 0, 0, buf, sendBuf)
	if err != nil {
		c.Close()
		return nil, err
	}
	_, err = c.Write(sendBuf.Bytes())
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
	if n < sizeUPTPHead+4 {
		c.Close()
		return nil, fmt.Errorf("wrong handshake response")
	}
	head, content, err := UnmarshalUPTPMessage(rsp[4:n])
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
	uptpConn := newRawUDPConn(nc)
	uptpConn.checkSend = u
	uptpConn.checkRecv = checkRecv
	uptpConn.isClient = true
	nc.SetSession(uptpConn)
	return uptpConn, nil
}

func wrapOnOpenRawUDPConn(h func(*rawUDPconn)) func(*nbio.Conn) {
	return func(c *nbio.Conn) {
		uptpConn := newRawUDPConn(c)
		uptpConn.isClient = false
		uptpConn.checkRecv = uint32(uintptr(unsafe.Pointer(c)))
		c.SetSession(uptpConn)
		if h != nil {
			h(uptpConn)
		}
	}
}

func wrapOnCloseRawUDPConn(h func(*rawUDPconn, error)) func(*nbio.Conn, error) {
	return func(c *nbio.Conn, err error) {
		if h != nil && c != nil {
			if session := c.Session(); session != nil {
				if uptpConn, ok := session.(*rawUDPconn); ok {
					if h != nil {
						h(uptpConn, err)
					}
					uptpConn.conn = nil
				}
			}
		}
	}
}

func wrapOnDataRawUDPConn(h func(*rawUDPconn, *uptpHead, []byte), handshakeCheck func(*uptpHead, []byte) bool) func(*nbio.Conn, []byte) {
	return func(c *nbio.Conn, data []byte) {
		if session := c.Session(); session != nil {
			if uptpConn, ok := session.(*rawUDPconn); ok {
				head, check, content, err := uptpConn.checkMessage(data)
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

				if check != uptpConn.checkRecv {
					c.CloseWithError(fmt.Errorf("message check fail"))
					return
				}

				if uptpConn.isClient {
					c.SetReadDeadline(time.Now().Add(time.Second * 30))
				} else {
					tn := time.Now().Unix()
					if tn-uptpConn.rspTime > 10 {
						uptpConn.sendMessage(0, head.From, 1, nil)
						uptpConn.rspTime = tn
					}
				}
				h(uptpConn, head, content)
			}
		}
	}
}
