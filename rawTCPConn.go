package uptp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/lesismal/nbio"
)

type rawTCPConn struct {
	connTime
	conn      net.Conn
	checkSend uint32
	checkRecv uint32
	isClient  bool
	peerID    int64
	rspTime   int64

	handshakeStatus bool
	curHead         *uptpHead
	inputBuf        *bytes.Buffer

	wMtx     sync.Mutex
	writeBuf *bytes.Buffer
}

func newRawTCPConn(conn net.Conn) *rawTCPConn {
	return &rawTCPConn{
		conn:     conn,
		inputBuf: bytes.NewBuffer(make([]byte, 0, 5200)),
		writeBuf: bytes.NewBuffer(make([]byte, 0, 5300)),
	}
}

func (rtc *rawTCPConn) handshakeComplete() bool {
	return rtc.handshakeStatus
}

func (rtc *rawTCPConn) readUPTPHead(data []byte) (*uptpHead, uint32, error) {
	check := binary.LittleEndian.Uint32(data[:4])
	head, err := UnmarshalUPTPHead(data[4:])
	if err != nil {
		return nil, 0, err
	}
	return head, check, nil
}

func (rtc *rawTCPConn) SendMessage(from, to int64, appID uint32, content []byte) error {
	// if !uconn.ready && appID == 0 {
	// 	return fmt.Errorf("uptp connect is not ready to send message")
	// }
	rtc.wMtx.Lock()
	defer rtc.wMtx.Unlock()
	err := binary.Write(rtc.writeBuf, binary.LittleEndian, rtc.checkSend)
	if err != nil {
		return fmt.Errorf("write check to buffer fail:%s", err)
	}
	err = marshalUPTPMessageToBuffer(from, to, appID, content, rtc.writeBuf)
	if err != nil {
		return fmt.Errorf("write uptp message to buffer fail: %s", err)
	}
	_, err = rtc.conn.Write(rtc.writeBuf.Bytes())
	if err != nil {
		return fmt.Errorf("send uptp message fail: %s", err)
	}
	rtc.writeBuf.Reset()
	return nil
}

func (rtc *rawTCPConn) GetPeerID() int64 {
	return rtc.peerID
}
func (rtc *rawTCPConn) SetPeerID(id int64) {
	rtc.peerID = id
}

func dialRawTCPConn(addr string, eg *nbio.Engine) (*rawTCPConn, error) {
	ta, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	c, err := net.DialTCP("tcp", nil, ta)
	if err != nil {
		return nil, err
	}

	checkRecv := uint32(uintptr(unsafe.Pointer(c)))
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, checkRecv)
	_, err = c.Write(buf)
	if err != nil {
		return nil, fmt.Errorf("write handshake fail:%s", err)
	}
	c.SetReadDeadline(time.Now().Add(time.Second * 5))
	_, err = io.ReadFull(c, buf)
	if err != nil {
		return nil, fmt.Errorf("read handshake fail:%s", err)
	}
	c.SetReadDeadline(time.Time{})
	u := binary.LittleEndian.Uint32(buf)

	nc, err := eg.AddConn(c)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("known error add connect")
	}
	uptpConn := newRawTCPConn(nc)
	uptpConn.checkSend = u
	uptpConn.checkRecv = checkRecv
	uptpConn.isClient = true
	uptpConn.handshakeStatus = true
	nc.SetSession(uptpConn)
	return uptpConn, nil
}

// func wrapOnOpenRawTCPConn(h func(*rawTCPConn)) func(*nbio.Conn) {
// 	return func(c *nbio.Conn) {
// 		uptpConn := newRawTCPConn(c)
// 		c.SetSession(uptpConn)
// 		if h != nil {
// 			h(uptpConn)
// 		}
// 	}
// }

func wrapOnCloseRawTCPConn(h func(*rawTCPConn, error)) func(*nbio.Conn, error) {
	return func(c *nbio.Conn, err error) {
		if h != nil && c != nil {
			if session := c.Session(); session != nil {
				if uptpConn, ok := session.(*rawTCPConn); ok {
					if h != nil {
						h(uptpConn, err)
					}
					uptpConn.conn = nil
				}
			}
		}
	}
}

func wrapOnDataRawTCPConn(h func(*rawTCPConn, *uptpHead, []byte), handshakeCheck func(*uptpHead, []byte) bool) func(*nbio.Conn, []byte) {
	return func(c *nbio.Conn, data []byte) {
		var rtc *rawTCPConn
		session := c.Session()
		if session == nil {
			//connect from accept, start handshake
			rtc = newRawTCPConn(c)
			rtc.checkRecv = uint32(uintptr(unsafe.Pointer(c)))
			c.SetSession(rtc)
		} else {
			ec, ok := session.(*rawTCPConn)
			if !ok {
				c.CloseWithError(fmt.Errorf("wrong session"))
				return
			}
			rtc = ec
		}
		_, err := rtc.inputBuf.Write(data)
		if err != nil {
			c.CloseWithError(fmt.Errorf("write input buffer fail:%s", err))
			return
		}
		c.SetReadDeadline(time.Now().Add(time.Second * 30))
		if !rtc.handshakeComplete() {
			bufLen := rtc.inputBuf.Len()
			//handshake server
			if bufLen < 4 {
				//handshake len 4
				return
			}
			rtc.checkSend = binary.LittleEndian.Uint32(rtc.inputBuf.Next(4))
			buf := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, rtc.checkRecv)
			_, err = c.Write(buf)
			if err != nil {
				c.CloseWithError(fmt.Errorf("write handshake fail:%s", err))
				return
			}
			rtc.handshakeStatus = true
			return
		}
		var setDeadline bool
		for {
			bufLen := rtc.inputBuf.Len()
			if rtc.curHead == nil {
				//read head
				if bufLen < sizeUPTPHead+4 {
					return
				}
				head, check, err := rtc.readUPTPHead(rtc.inputBuf.Next(sizeUPTPHead + 4))
				if err != nil {
					c.CloseWithError(fmt.Errorf("read uptp head fail:%s", err))
					return
				}
				if check != rtc.checkRecv {
					c.CloseWithError(fmt.Errorf("message check fail"))
					return
				}
				if head.Len > 64*1024*1024 {
					c.CloseWithError(fmt.Errorf("check head fail: message len to large"))
					return
				}
				rtc.curHead = head
				bufLen = rtc.inputBuf.Len()
			}
			if !setDeadline && !rtc.isClient {
				setDeadline = true
				tn := time.Now().Unix()
				if tn-rtc.rspTime > 10 {
					rtc.SendMessage(0, rtc.curHead.From, 1, nil)
					rtc.rspTime = tn
				}
			}
			if bufLen < int(rtc.curHead.Len) {
				return
			}
			uptpData := make([]byte, rtc.curHead.Len)
			copy(uptpData, rtc.inputBuf.Next(int(rtc.curHead.Len)))
			h(rtc, rtc.curHead, uptpData)
			rtc.curHead = nil
		}
	}
}
