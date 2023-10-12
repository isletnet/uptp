// package uptp

// import (
// 	"bytes"
// 	"encoding/binary"
// 	"encoding/json"
// 	"errors"
// 	"fmt"
// )

// var (
// 	ErrHeaderError = errors.New("header error")
// )

// type UptpMsgHeader struct {
// 	DataLen  uint32
// 	MainType uint16
// 	SubType  uint16
// }

// var UptpMsgHeaderSize = binary.Size(UptpMsgHeader{})

// type UptpMsg struct {
// 	Head    UptpMsgHeader
// 	Content []byte
// }

// func ParseUPTPMsg(msg []byte) (*UptpMsg, error) {
// 	um := UptpMsg{}
// 	if len(msg) < UptpMsgHeaderSize {
// 		return nil, fmt.Errorf("%w: len too short", ErrHeaderError)
// 	}
// 	err := binary.Read(bytes.NewReader(msg[:UptpMsgHeaderSize]), binary.LittleEndian, &um.Head)
// 	if err != nil {
// 		return nil, fmt.Errorf("%w: wrong head format", ErrHeaderError)
// 	}
// 	um.Content = msg[UptpMsgHeaderSize:]
// 	if len(um.Content) != int(um.Head.DataLen) {
// 		return nil, fmt.Errorf("%w: len check failed", ErrHeaderError)
// 	}
// 	return &um, nil
// }

// func NewUptpMsgStruct(mt, st uint16, content interface{}) ([]byte, error) {
// 	data, err := json.Marshal(content)
// 	if err != nil {
// 		return nil, err
// 	}
// 	um := UptpMsg{
// 		Head: UptpMsgHeader{
// 			DataLen:  uint32(len(data)),
// 			MainType: mt,
// 			SubType:  st,
// 		},
// 		Content: data,
// 	}
// 	return FormatUPTPMsg(um)
// }

// func NewUptpMsgBytes(mt, st uint16, content []byte) ([]byte, error) {
// 	um := UptpMsg{
// 		Head: UptpMsgHeader{
// 			DataLen:  uint32(len(content)),
// 			MainType: mt,
// 			SubType:  st,
// 		},
// 		Content: content,
// 	}
// 	return FormatUPTPMsg(um)
// }

// func FormatUPTPMsg(um UptpMsg) ([]byte, error) {
// 	headBuf, err := EncodeHeader(um.Head)
// 	if err != nil {
// 		return nil, err
// 	}
// 	writeBytes := append(headBuf, um.Content...)
// 	return writeBytes, nil
// }

// func EncodeHeader(head UptpMsgHeader) ([]byte, error) {
// 	headBuf := new(bytes.Buffer)
// 	err := binary.Write(headBuf, binary.LittleEndian, head)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return headBuf.Bytes(), nil
// }

// const (
// 	UptpMsgHeartbeat = 2025

package uptp

import "time"

const (
	msgIDTypeOffline         = 1
	msgIDTypeHeartbeat       = 2
	msgIDTypeUptpcInfoReport = 3
	msgIDTypeUptpcInfoReq    = 4
	msgIDTypeUptpcInfoRsp    = 5
	msgIDTypeAddTunnelReq    = 6
	msgIDTypeAddTunnelRsp    = 7
)

const (
	natTestTimeout          = time.Second * 5
	publicIPTestTimeout     = time.Second * 1
	tcpPunchHoleTimeout     = time.Second * 5
	addTunnelRspWaitTimeout = time.Second * 10
	punchHoleSyncTime       = time.Millisecond * 1000
)

type uptpcBaseInfo struct {
	Name     string `json:"name"`
	ID       uint64 `json:"id"`
	LocalIP  string `json:"localIP"`
	OS       string `json:"os"`
	IPv6     string `json:"ipv6"`
	PublicIP string `json:"publicIP"`
	NatType  int    `json:"natType"`

	IsExclusivePublicIPV4 bool `json:"isExclusivePublicIPV4"`
}

type uptpcInfoReport struct {
	uptpcBaseInfo
	PIPInfo publicIPInfo `json:"pipInfo"`
}

type uptpcInfoRequest struct {
	ReplyTo string `json:"replyTo"`
}

type uptpcInfoResponse struct {
	uptpcBaseInfo
}

const (
	addTunnelTypePublicIP        = 1
	addTunnelTypePublicIPPassive = 2
	addTunnelTypeTCPPunch        = 3
	addTunnelTypeUDPPunch        = 4
)

type addTunnelConfig struct {
	AddTnnelType int    `json:"addTunnelType"`
	PublicIP     string `json:"publicIP"`
	TCPPort      int    `json:"tcpPort"`
}

type addTunnelRequest struct {
	uptpcBaseInfo
	Time    int64           `json:"time"`
	Config  addTunnelConfig `json:"config"`
	ReplyTo string          `json:"replyTo"`
}

type addTunnelResponse struct {
	Errno    int             `json:"errno"`
	T1       int64           `json:"t1"`
	T2       int64           `json:"t2"`
	SyncTime time.Duration   `json:"syncTime"`
	Config   addTunnelConfig `json:"config"`
}
