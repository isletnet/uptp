package uptp

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type uptpHead struct {
	Len     uint16
	Version uint16
	Check   uint32
	From    int64
	To      int64
	AppID   uint32
}

var sizeUPTPHead = binary.Size(uptpHead{})

func marshalUPTPMessage(from, to int64, appID uint32, check uint32, content []byte) ([]byte, error) {
	head := uptpHead{
		Len:     uint16(len(content)),
		Version: 1,
		Check:   check,
		From:    from,
		To:      to,
		AppID:   appID,
	}
	headBuf := new(bytes.Buffer)
	err := binary.Write(headBuf, binary.LittleEndian, head)
	if err != nil {
		return nil, fmt.Errorf("marshal uptp message head fail: %s", err)
	}
	writeBytes := append(headBuf.Bytes(), content...)
	return writeBytes, nil
}

func UnmarshalUPTPMessage(message []byte) (*uptpHead, []byte, error) {
	head := uptpHead{}
	rd := bytes.NewReader(message)
	err := binary.Read(rd, binary.LittleEndian, &head)
	if err != nil {
		return nil, nil, fmt.Errorf("read message header fail: %s", err)
	}
	if int(head.Version) != 1 {
		return nil, nil, fmt.Errorf("unknown message version: %d", head.Version)
	}
	if int(head.Len)+sizeUPTPHead != len(message) {
		return nil, nil, fmt.Errorf("message len check fail")
	}
	return &head, message[sizeUPTPHead:], nil
}
