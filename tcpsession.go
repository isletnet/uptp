package uptp

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/lesismal/nbio"
)

type rawTCPSession struct {
	conn   net.Conn
	peerID uint64

	curHead  *uptpPacketHead
	inputBuf *bytes.Buffer

	wMtx     sync.Mutex
	writeBuf *bytes.Buffer
}

func newRawTCPSession(conn net.Conn, id uint64) *rawTCPSession {
	return &rawTCPSession{
		conn:     conn,
		peerID:   id,
		inputBuf: bytes.NewBuffer(make([]byte, 0, 5200)),
		writeBuf: bytes.NewBuffer(make([]byte, 0, 5300)),
	}
}

// func (rtc *rawTCPConn) handshakeComplete() bool {
// 	return rtc.handshakeStatus
// }

func (rtc *rawTCPSession) readUPTPHead(data []byte) (*uptpPacketHead, error) {
	head, err := unmarshalUPTPPacketHead(data)
	if err != nil {
		return nil, err
	}
	return head, nil
}

func (rtc *rawTCPSession) SendPacket(from, to uint64, content []byte) error {
	// if !uconn.ready && appID == 0 {
	// 	return fmt.Errorf("uptp connect is not ready to send packet")
	// }
	rtc.wMtx.Lock()
	defer rtc.wMtx.Unlock()
	err := marshalUPTPPacketToBuffer(from, to, content, rtc.writeBuf)
	if err != nil {
		return fmt.Errorf("write uptp packet to buffer fail: %s", err)
	}
	_, err = rtc.conn.Write(rtc.writeBuf.Bytes())
	if err != nil {
		return fmt.Errorf("send uptp packet fail: %s", err)
	}
	rtc.writeBuf.Reset()
	return nil
}

func (uconn *rawTCPSession) GetPeerID() uint64 {
	return uconn.peerID
}
func wrapOnCloseRawTCPConn(h func(uptpTunnel, error)) func(*nbio.Conn, error) {
	return func(c *nbio.Conn, err error) {
		if h != nil && c != nil {
			if session := c.Session(); session != nil {
				if uptpConn, ok := session.(*rawTCPSession); ok {
					if h != nil {
						h(uptpConn, err)
					}
					uptpConn.conn = nil
				}
			}
		}
	}
}

func wrapOnDataRawTCPConn(h func(uptpTunnel, *uptpPacketHead, []byte), handshakeCheck func(*uptpPacketHead, []byte) bool) func(*nbio.Conn, []byte) {
	return func(c *nbio.Conn, data []byte) {
		var rtc *rawTCPSession
		session := c.Session()
		if session == nil {
			// connect from accept, start handshake
			rtc = newRawTCPSession(c, 0)
			c.SetSession(rtc)
			return
		} else {
			ec, ok := session.(*rawTCPSession)
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
		//todo handshake
		c.SetReadDeadline(time.Now().Add(time.Second * 35))
		if rtc.peerID == 0 {
			return
		}
		for {
			bufLen := rtc.inputBuf.Len()
			if rtc.curHead == nil {
				//read head
				if bufLen < uptpPacketHeadSize {
					return
				}
				head, err := rtc.readUPTPHead(rtc.inputBuf.Next(uptpPacketHeadSize))
				if err != nil {
					c.CloseWithError(fmt.Errorf("read uptp head fail:%s", err))
					return
				}
				if head.Len > 64*1024*1024 {
					c.CloseWithError(fmt.Errorf("check head fail: packet len to large"))
					return
				}
				rtc.curHead = head
				bufLen = rtc.inputBuf.Len()
			}
			// if !setDeadline && !rtc.isClient {
			// 	setDeadline = true
			// 	tn := time.Now().Unix()
			// 	if tn-rtc.rspTime > 10 {
			// 		rtc.SendMessage(0, rtc.curHead.From, 1, nil)
			// 		rtc.rspTime = tn
			// 	}
			// }
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
