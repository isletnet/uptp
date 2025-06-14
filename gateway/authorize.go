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

const (
	AuthorizeTypePortmap = 1
	AuthorizeTypeProxy   = 2
)

type AuthorizeReq struct {
	Type    int                   `json:"type"`
	Portmap *AuthorizePortmapInfo `json:"portmap,omitempty"`
	Proxy   *AuthorizeProxyInfo   `json:"proxy,omitempty"`
}

type AuthorizePortmapInfo struct {
	ResourceID types.ID `json:"resource_id"`
}

type AuthorizeProxyInfo struct {
	Token types.ID `json:"token"`
	Route string   `json:"route"`
	Dns   string   `json:"dns"`
}

type AuthorizeResp struct {
	NodeName string                `json:"node_name"`
	Err      string                `json:"err"`
	Portmap  *AuthorizePortmapResp `json:"portmap,omitempty"`
	Proxy    *AuthorizeProxyResp   `json:"proxy,omitempty"`
}
type AuthorizePortmapResp struct {
	IsTrial bool `json:"is_trial"`
}

type AuthorizeProxyResp struct {
	Route string `json:"route"`
	Dns   string `json:"dns"`
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
	switch req.Type {
	case AuthorizeTypePortmap:
		g.handlePortmapAuth(s, req.Portmap)
	case AuthorizeTypeProxy:
		g.handleProxyAuth(s, req.Proxy)
	default:
		return
	}
}

func (g *Gateway) handlePortmapAuth(s network.Stream, info *AuthorizePortmapInfo) {
	var authRes bool

	if info.ResourceID == types.ID(666666) && g.trial {
		authRes = true
	} else if res := g.prm.GetAppByID(info.ResourceID); res.ID == info.ResourceID {
		authRes = true
	}
	resp := AuthorizeResp{
		Portmap: &AuthorizePortmapResp{},
	}
	if authRes {
		gwName, err := g.getGatewayName()
		if err != nil {
			resp.Err = err.Error()
		}
		resp.NodeName = gwName
		resp.Portmap.IsTrial = info.ResourceID == types.ID(666666)
	} else {
		resp.Err = "authorize failed"
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	s.Write(data)
}

func (g *Gateway) handleProxyAuth(s network.Stream, info *AuthorizeProxyInfo) {
	token, err := g.getToken()
	if err != nil {
		return
	}
	if token != uint64(info.Token) {
		return
	}

	resp := AuthorizeResp{
		Proxy: &AuthorizeProxyResp{},
	}
	gwName, err := g.getGatewayName()
	if err != nil {
		resp.Err = err.Error()
	}
	resp.NodeName = gwName
	pc := g.proxySvc.getConfig()
	resp.Proxy.Route = pc.Route
	resp.Proxy.Dns = pc.DNS
	if resp.Proxy.Route == "" {
		resp.Proxy.Route = "0.0.0.0/0"
	}
	if resp.Proxy.Dns == "" {
		resp.Proxy.Dns = "8.8.8.8"
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
