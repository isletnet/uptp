package gateway

import (
	"context"
	"encoding/json"
	"time"

	"github.com/isletnet/uptp/types"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	ResourceAuthorizeID = "/resource/authorize/1.0.0"
)

type AuthorizeReq struct {
	ResourceID types.ID `json:"resource_id"`
	// Token      string
}

type AuthorizeResp struct {
	NodeName string `json:"node_name"`
	IsTrial  bool   `json:"is_trial"`
	Err      string `json:"err"`
	// Token      string
}

func (g *Gateway) authorize() {
	g.pe.Libp2pHost().SetStreamHandler(ResourceAuthorizeID, g.authorizeHandler)
}

func (g *Gateway) authorizeHandler(s network.Stream) {
	defer s.Close()
	// TODO
	buf := make([]byte, 1024)
	n, err := s.Read(buf)
	if err != nil {
		return
	}
	var req AuthorizeReq
	err = json.Unmarshal(buf[:n], &req)
	if err != nil {
		return
	}
	gwName, err := g.getGatewayName()
	if err != nil {
		return
	}
	resp := AuthorizeResp{
		NodeName: gwName,
		IsTrial:  req.ResourceID == types.ID(666666),
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	s.Write(data)
}

func ResourceAuthorize(h host.Host, peerID string, req AuthorizeReq) (resp AuthorizeResp, err error) {
	data, err := json.Marshal(req)
	if err != nil {
		return
	}
	pid, err := peer.Decode(peerID)
	if err != nil {
		return
	}
	s, err := h.NewStream(context.Background(), pid, ResourceAuthorizeID)
	if err != nil {
		return
	}
	defer s.Close()
	s.SetDeadline(time.Now().Add(10 * time.Second))
	s.Write(data)
	buf := make([]byte, 1024)
	n, err := s.Read(buf)
	if err != nil {
		return
	}
	err = json.Unmarshal(buf[:n], &resp)
	return
}
