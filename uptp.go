package uptp

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type uptpHead struct {
	From  int64
	To    int64
	Len   uint32
	AppID uint32
}

var sizeUPTPHead = binary.Size(uptpHead{})

func marshalUPTPMessage(from, to int64, appID uint32, content []byte) ([]byte, error) {
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

func marshalUPTPMessageToBuffer(from, to int64, appID uint32, content []byte, buf *bytes.Buffer) error {
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

func UnmarshalUPTPMessage(message []byte) (*uptpHead, []byte, error) {
	head := uptpHead{}
	rd := bytes.NewReader(message)
	err := binary.Read(rd, binary.LittleEndian, &head)
	if err != nil {
		return nil, nil, fmt.Errorf("read message header fail: %s", err)
	}
	if int(head.Len)+sizeUPTPHead != len(message) {
		return nil, nil, fmt.Errorf("message len check fail")
	}
	return &head, message[sizeUPTPHead:], nil
}

type uptpConn interface {
	SendMessage(from, to int64, appID uint32, content []byte) error
}
