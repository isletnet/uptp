package uptp

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type uptpPacketHead struct {
	From uint64
	To   uint64
	Len  uint32
}

var uptpPacketHeadSize = binary.Size(uptpPacketHead{})

func marshalUPTPPacket(from, to uint64, content []byte) ([]byte, error) {
	head := uptpPacketHead{
		Len:  uint32(len(content)),
		From: from,
		To:   to,
	}
	headBuf := new(bytes.Buffer)
	err := binary.Write(headBuf, binary.LittleEndian, head)
	if err != nil {
		return nil, fmt.Errorf("marshal uptp packet head fail: %s", err)
	}
	writeBytes := append(headBuf.Bytes(), content...)
	return writeBytes, nil
}

func marshalUPTPPacketToBuffer(from, to uint64, content []byte, buf *bytes.Buffer) error {
	head := uptpPacketHead{
		Len:  uint32(len(content)),
		From: from,
		To:   to,
	}
	err := binary.Write(buf, binary.LittleEndian, head)
	if err != nil {
		return fmt.Errorf("marshal uptp packet head fail: %s", err)
	}
	_, err = buf.Write(content)
	if err != nil {
		return fmt.Errorf("write uptp packet content to buf fail: %s", err)
	}
	return nil
}

func unmarshalUPTPPacketHead(packet []byte) (*uptpPacketHead, error) {
	head := uptpPacketHead{}
	rd := bytes.NewReader(packet)
	err := binary.Read(rd, binary.LittleEndian, &head)
	if err != nil {
		return nil, fmt.Errorf("read packet header fail: %s", err)
	}
	return &head, nil
}

func unmarshalUPTPPacket(packet []byte) (*uptpPacketHead, []byte, error) {
	head, err := unmarshalUPTPPacketHead(packet)
	if err != nil {
		return nil, nil, err
	}
	if int(head.Len)+uptpPacketHeadSize != len(packet) {
		return nil, nil, fmt.Errorf("packet len check fail")
	}
	return head, packet[uptpPacketHeadSize:], nil
}

type uptpTunnel interface {
	SendPacket(from, to uint64, content []byte) error
	// SetPeerID(uint64)
	GetPeerID() uint64
	// GetTimeStamp() int64
	// SetTimeStamp(ct int64)
}

const (
	tunnelCtrlHeartbeat    = 1
	tunnelCtrlHeartbeatAck = 2
)
