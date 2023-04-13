package uptp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc64"
)

type uptpHead struct {
	From  uint64
	To    uint64
	Len   uint32
	AppID uint32
}

var sizeUPTPHead = binary.Size(uptpHead{})

func marshalUPTPMessage(from, to uint64, appID uint32, content []byte) ([]byte, error) {
	head := uptpHead{
		Len:   uint32(len(content)),
		From:  from,
		To:    to,
		AppID: appID,
	}
	headBuf := new(bytes.Buffer)
	err := binary.Write(headBuf, binary.LittleEndian, head)
	if err != nil {
		return nil, fmt.Errorf("marshal uptp message head fail: %s", err)
	}
	writeBytes := append(headBuf.Bytes(), content...)
	return writeBytes, nil
}

func marshalUPTPMessageToBuffer(from, to uint64, appID uint32, content []byte, buf *bytes.Buffer) error {
	head := uptpHead{
		Len:   uint32(len(content)),
		From:  from,
		To:    to,
		AppID: appID,
	}
	err := binary.Write(buf, binary.LittleEndian, head)
	if err != nil {
		return fmt.Errorf("marshal uptp message head fail: %s", err)
	}
	_, err = buf.Write(content)
	if err != nil {
		return fmt.Errorf("write uptp message content to buf fail: %s", err)
	}
	return nil
}

func UnmarshalUPTPHead(message []byte) (*uptpHead, error) {
	head := uptpHead{}
	rd := bytes.NewReader(message)
	err := binary.Read(rd, binary.LittleEndian, &head)
	if err != nil {
		return nil, fmt.Errorf("read message header fail: %s", err)
	}
	return &head, nil
}

func UnmarshalUPTPMessage(message []byte) (*uptpHead, []byte, error) {
	head, err := UnmarshalUPTPHead(message)
	if err != nil {
		return nil, nil, err
	}
	if int(head.Len)+sizeUPTPHead != len(message) {
		return nil, nil, fmt.Errorf("message len check fail")
	}
	return head, message[sizeUPTPHead:], nil
}

type uptpConn interface {
	SendMessage(from, to uint64, appID uint32, content []byte) error
	SetPeerID(uint64)
	GetPeerID() uint64
	GetTimeStamp() int64
	SetTimeStamp(ct int64)
}

type connTime int64

func (t *connTime) GetTimeStamp() int64 {
	return int64(*t)
}

func (t *connTime) SetTimeStamp(ct int64) {
	*t = connTime(ct)
}

type UPTPInfo struct {
	PeerID   uint64      `json:"peerID"`
	PublicIP string      `json:"publicIP"`
	TCPPort  int         `json:"tcpPort"`
	UDPPort  int         `json:"udpPort"`
	Extra    interface{} `json:"extra"`
}

func GetIDByName(name string) uint64 {
	return crc64.Checksum([]byte(name), crc64.MakeTable(crc64.ISO))
}
