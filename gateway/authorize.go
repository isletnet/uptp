package gateway

import (
	"context"
	"encoding/json"
	"time"

	"github.com/isletnet/uptp/logging"
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
	var authRes bool

	if req.ResourceID == types.ID(666666) && g.trial {
		authRes = true
	} else if res := g.pam.GetAppByID(req.ResourceID); res.ID == req.ResourceID {
		authRes = true
	} else {
		token, err := g.getToken()
		if err != nil {
			logging.Error("authorize handler get token error: %s", err)
		}
		if token == uint64(req.ResourceID) {
			authRes = true
		}
	}
	resp := AuthorizeResp{}
	if authRes {
		gwName, err := g.getGatewayName()
		if err != nil {
			resp.Err = err.Error()
		}
		resp.NodeName = gwName
		resp.IsTrial = req.ResourceID == types.ID(666666)
	} else {
		resp.Err = "authorize failed"
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
