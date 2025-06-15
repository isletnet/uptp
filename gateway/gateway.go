package gateway

import (
	"crypto/ed25519"
	"encoding/json"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/isletnet/uptp/apiutil.go"
	"github.com/isletnet/uptp/logger"
	"github.com/isletnet/uptp/logging"
	"github.com/isletnet/uptp/p2pengine"
	"github.com/isletnet/uptp/portmap"
	"github.com/isletnet/uptp/socks5"
	"github.com/isletnet/uptp/types"

	"github.com/go-chi/chi"
	"github.com/google/uuid"
	"github.com/isletnet/uptp/common"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

const (
	dbData = "data.db"
)

const (
	dbKeyToken       = "token"
	dbKeyBootstraps  = "bootstraps"
	dbKeyGatewayName = "gateway_name"
	dbKeyListenPort  = "listen_port"
)

type Gateway struct {
	pe       *p2pengine.P2PEngine
	pm       *portmap.Portmap
	db       *leveldb.DB
	prm      *PortmapResMgr
	pam      *PortmapAppMgr
	proxySvc *proxyService

	trial bool
}

type Config struct {
	Workdir  string
	LogDir   string
	LogMod   int
	LogLevel int
}

type PortmapAppHandshake struct {
	ResID      types.ID `json:"res_id"`
	Network    string   `json:"network"`
	TargetAddr string   `json:"target_addr"`
	TargetPort int      `json:"target_port"`
}

func (g *Gateway) SetTrialMod() {
	g.trial = true
}

func (g *Gateway) Run(conf Config) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	bak := exePath + ".bak"
	if _, err := os.Stat(bak); os.IsExist(err) {
		os.Remove(bak)
	}
	wd := conf.Workdir
	if wd == "" {
		wd = filepath.Dir(exePath)
	}
	os.Chdir(wd)
	ld := conf.LogDir
	if ld == "" {
		ld = "."
	}

	lm := conf.LogMod

	gLog := logger.NewLogger(ld, "uptp-gateway", conf.LogLevel, 1024*1024, lm)
	logging.SetLogger(gLog)

	logging.Info("uptp gateway start")

	var nopts opt.Options

	db, err := leveldb.OpenFile(dbData, &nopts)
	if errors.IsCorrupted(err) && !nopts.GetReadOnly() {
		db, err = leveldb.RecoverFile(dbData, &nopts)
	}
	if err != nil {
		return err
	}
	g.db = db

	us, err := os.ReadFile("uuid")
	if err != nil && os.IsExist(err) {
		return err
	}
	if len(us) < ed25519.SeedSize {
		u, err := uuid.NewRandom()
		if err != nil {
			return err
		}
		str := strings.Replace(u.String(), "-", "", -1)
		if len(str) < ed25519.SeedSize {
			return err
		}
		err = os.WriteFile("uuid", []byte(str), 0644)
		if err != nil {
			return err
		}
		us = []byte(str)
	}
	listenPort, err := g.getListenPort()
	if err != nil {
		return err
	}
	pe, err := p2pengine.NewP2PEngine(listenPort, us, filepath.Join(ld, "libp2p.log"), "dht.db", false, g.getBootstraps)
	if err != nil {
		return err
	}
	logging.Info("libp2p start, id: %s, port: %d", pe.Libp2pHost().ID(), pe.GetListenPort())
	if listenPort != pe.GetListenPort() {
		g.setListenPort(pe.GetListenPort())
	}
	g.pe = pe

	prm, err := NewPortmapResMgr(db)
	if err != nil {
		return err
	}
	g.prm = prm

	pam := NewPortmapAppMgr(db)
	pmApps, err := pam.LoadPortmapApps()
	if err != nil {
		return err
	}
	g.pam = pam

	// 初始化代理服务
	g.proxySvc = &proxyService{
		db:     db,
		config: proxyServiceConfig{},
	}
	if err := g.proxySvc.loadConfig(); err != nil {
		return err
	}
	proxyConfig := g.proxySvc.getConfig()
	socks5.SetOutboundProxy(proxyConfig.ProxyAddr, proxyConfig.ProxyUser, proxyConfig.ProxyPass)

	g.pm = portmap.NewPortMap(pe.Libp2pHost())
	g.pm.SetHandleHandshakeFunc(g.handlePortmapHandshake)
	g.pm.SetGetHandshakeFunc(func(network, ip string, port int) (peerID string, handshake []byte) {
		app := g.pam.FindAppWithPort(network, port)
		if app.ResID == 0 {
			return
		}
		peerID = app.PeerID
		hs := PortmapAppHandshake{
			ResID:      types.ID(app.ResID),
			Network:    app.Network,
			TargetAddr: app.TargetAddr,
			TargetPort: app.TargetPort,
		}
		handshake, err = json.Marshal(hs)
		if err != nil {
			return
		}
		return
	})
	g.pm.Start(true)

	if g.trial {
		socks5.StartServe(g.pe.Libp2pHost(), func(authID uint64) bool {
			// 试用模式下允许所有连接
			return true
		})
	} else {
		socks5.StartServe(g.pe.Libp2pHost(), g.proxyAuth)
	}

	for _, a := range pmApps {
		if !a.Running {
			continue
		}
		_, err = g.pm.AddListener(a.Network, a.LocalIP, a.LocalPort)
		if err != nil {
			a.Err = ""
			a.Running = false
			g.pam.UpdatePortmapApp(&a)
			logging.Error("add portmap listener error: %s", err)
		}
	}
	apiSer := &apiutil.ApiServer{}
	g.router(apiSer)
	g.authorize()

	ln, err := net.Listen("tcp", "0.0.0.0:3000")
	if err != nil {
		return err
	}

	return apiSer.Serve(ln)
}

func (g *Gateway) router(ser *apiutil.ApiServer) {
	// 初始化文件服务器
	fileServer := http.FileServer(getWebFS())

	ser.AddRoute("/resource", func(r chi.Router) {
		r.Get("/list", g.listResources)
		r.Get("/get/{id}", g.getResource)
		r.Post("/add", g.addResource)
		r.Post("/update", g.updateResource)
		r.Post("/delete", g.deleteResource)
	})

	ser.AddRoute("/gateway", func(r chi.Router) {
		r.Get("/info", g.getGatewayInfo)
		r.Post("/name", g.updateGatewayName)
		r.Get("/restart", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("ok"))
			os.Exit(0)
		})
	})

	ser.AddRoute("/proxy", func(r chi.Router) {
		r.Get("/config", g.getProxyConfig)
		// r.Get("/token/list", g.getProxyTokens)
		// r.Post("/token/add", g.addProxyToken)
		// r.Post("/token/delete", g.deleteProxyToken)
		r.Post("/config", g.updateProxyConfig)
		// r.Post("/dns/set", g.setProxyDNS)
		// r.Get("/dns/get", g.getProxyDNS)
		// r.Post("/outbound_proxy/set", g.setProxyOBProxy)
		// r.Get("/outbound_proxy/get", g.getProxyOBProxy)
	})
	ser.AddRoute("/upgrade", func(r chi.Router) {
		r.Get("/myself", g.upgradeMyself)
		r.Get("/agent/android", g.downloadAPK)
	})

	ser.AddRoute("/app", func(r chi.Router) {
		r.Post("/add", g.addApp)
		r.Post("/update", g.updateApp)
		r.Post("/delete", g.deleteApp)
		r.Get("/list", g.listApps)
		r.Get("/get/{id}", g.getApp)
	})

	// 静态文件路由
	ser.AddRoute("/", func(r chi.Router) {
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			fileServer.ServeHTTP(w, r)
		})
	})
}

func transferHTTP(w http.ResponseWriter, resp *http.Response) {
	// Copy any headers
	for k, v := range resp.Header {
		for _, s := range v {
			w.Header().Add(k, s)
		}
	}

	// Write response status and headers
	w.WriteHeader(resp.StatusCode)

	// Finally copy the body
	io.Copy(w, resp.Body)
}

func (g *Gateway) proxyAuth(authToken uint64) bool {
	token, err := g.getToken()
	if err != nil {
		logging.Error("proxy auth get token error: %s", err)
		return false
	}
	return token == authToken
}

func (g *Gateway) getProxyConfig(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}
	if g.proxySvc == nil {
		rsp.Code = 500
		rsp.Message = "proxy service not initialized"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	config := g.proxySvc.getConfig()
	rsp.Data = config
	apiutil.SendAPIRespWithOk(w, rsp)
}

// func (g *Gateway) getProxyTokens(w http.ResponseWriter, r *http.Request) {
// 	rsp := apiResponse{}
// 	if g.proxySvc == nil {
// 		rsp.Code = 500
// 		rsp.Message = "proxy service not initialized"
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}
// 	config := g.proxySvc.getConfig()
// 	rsp.Data = config.AccessTokens
// 	sendAPIRespWithOk(w, rsp)
// }

// func (g *Gateway) addProxyToken(w http.ResponseWriter, r *http.Request) {
// 	rsp := apiResponse{}
// 	if g.proxySvc == nil {
// 		rsp.Code = 500
// 		rsp.Message = "proxy service not initialized"
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	body, err := io.ReadAll(r.Body)
// 	if err != nil {
// 		rsp.Code = 500
// 		rsp.Message = err.Error()
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	var req struct {
// 		Token  uint64 `json:"token"`
// 		Remark string `json:"remark"`
// 	}
// 	if err := json.Unmarshal(body, &req); err != nil {
// 		rsp.Code = 400
// 		rsp.Message = err.Error()
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	if req.Token == 0 {
// 		rsp.Code = 400
// 		rsp.Message = "uid and token are required"
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	if err := g.proxySvc.addToken(req.Token, req.Remark); err != nil {
// 		rsp.Code = 500
// 		rsp.Message = err.Error()
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	rsp.Message = "ok"
// 	sendAPIRespWithOk(w, rsp)
// }

// func (g *Gateway) deleteProxyToken(w http.ResponseWriter, r *http.Request) {
// 	rsp := apiResponse{}
// 	if g.proxySvc == nil {
// 		rsp.Code = 500
// 		rsp.Message = "proxy service not initialized"
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	body, err := io.ReadAll(r.Body)
// 	if err != nil {
// 		rsp.Code = 500
// 		rsp.Message = err.Error()
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	var req struct {
// 		Token uint64 `json:"token"`
// 	}
// 	if err := json.Unmarshal(body, &req); err != nil {
// 		rsp.Code = 400
// 		rsp.Message = err.Error()
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	if req.Token == 0 {
// 		rsp.Code = 400
// 		rsp.Message = "token is required"
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	if err := g.proxySvc.removeToken(req.Token); err != nil {
// 		rsp.Code = 500
// 		rsp.Message = err.Error()
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	rsp.Message = "ok"
// 	sendAPIRespWithOk(w, rsp)
// }

func (g *Gateway) updateProxyConfig(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}
	if g.proxySvc == nil {
		rsp.Code = 500
		rsp.Message = "proxy service not initialized"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	req := proxyServiceConfig{}
	if err := json.Unmarshal(body, &req); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	if err := g.proxySvc.set(req); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	socks5.SetOutboundProxy(req.ProxyAddr, req.ProxyUser, req.ProxyPass)

	rsp.Message = "ok"
	apiutil.SendAPIRespWithOk(w, rsp)
}

// func (g *Gateway) setProxyDNS(w http.ResponseWriter, r *http.Request) {
// 	rsp := apiutil.ApiResponse{}
// 	if g.proxySvc == nil {
// 		rsp.Code = 500
// 		rsp.Message = "proxy service not initialized"
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	body, err := io.ReadAll(r.Body)
// 	if err != nil {
// 		rsp.Code = 500
// 		rsp.Message = err.Error()
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	var req struct {
// 		DNS string `json:"dns"`
// 	}
// 	if err := json.Unmarshal(body, &req); err != nil {
// 		rsp.Code = 400
// 		rsp.Message = err.Error()
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	if req.DNS == "" {
// 		rsp.Code = 400
// 		rsp.Message = "dns is required"
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	if err := g.proxySvc.setDNS(req.DNS); err != nil {
// 		rsp.Code = 500
// 		rsp.Message = err.Error()
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	rsp.Message = "ok"
// 	apiutil.SendAPIRespWithOk(w, rsp)
// }

// func (g *Gateway) getProxyDNS(w http.ResponseWriter, r *http.Request) {
// 	rsp := apiResponse{}
// 	if g.proxySvc == nil {
// 		rsp.Code = 500
// 		rsp.Message = "proxy service not initialized"
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}
// 	config := g.proxySvc.getConfig()
// 	rsp.Data = config.DNS
// 	sendAPIRespWithOk(w, rsp)
// }

// func (g *Gateway) setProxyOBProxy(w http.ResponseWriter, r *http.Request) {
// 	rsp := apiutil.ApiResponse{}
// 	if g.proxySvc == nil {
// 		rsp.Code = 500
// 		rsp.Message = "proxy service not initialized"
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	body, err := io.ReadAll(r.Body)
// 	if err != nil {
// 		rsp.Code = 500
// 		rsp.Message = err.Error()
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	var req struct {
// 		Addr string `json:"addr"`
// 		User string `json:"user"`
// 		Pass string `json:"pass"`
// 	}
// 	if err := json.Unmarshal(body, &req); err != nil {
// 		rsp.Code = 400
// 		rsp.Message = err.Error()
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	if req.Addr == "" {
// 		rsp.Code = 400
// 		rsp.Message = "proxy address is required"
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}

// 	if err := g.proxySvc.setProxy(req.Addr, req.User, req.Pass); err != nil {
// 		rsp.Code = 500
// 		rsp.Message = err.Error()
// 		apiutil.SendAPIRespWithOk(w, rsp)
// 		return
// 	}
// 	socks5.SetOutboundProxy(req.Addr, req.User, req.Pass)

// 	rsp.Message = "ok"
// 	apiutil.SendAPIRespWithOk(w, rsp)
// }

func (g *Gateway) addApp(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	var app PortmapApp
	if err := json.Unmarshal(body, &app); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	authRsp, err := ResourceAuthorize(g.pe.Libp2pHost(), app.PeerID, AuthorizeReq{
		Type: AuthorizeTypePortmap,
		Portmap: &AuthorizePortmapInfo{
			ResourceID: types.ID(app.ResID),
		},
	})
	if err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	if authRsp.Err != "" {
		rsp.Code = 400
		rsp.Message = authRsp.Err
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	app.PeerName = authRsp.NodeName

	// 生成随机ID
	app.ID = types.ID(rand.Uint64())

	// 如果应用设置为运行状态，则添加listener
	if app.Running {
		_, err := g.pm.AddListener(app.Network, app.LocalIP, app.LocalPort)
		if err != nil {
			app.Running = false
			app.Err = err.Error()
		}
	}

	if err := g.pam.UpdatePortmapApp(&app); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Message = "ok"
	apiutil.SendAPIRespWithOk(w, rsp)
}

func (g *Gateway) updateApp(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	var app PortmapApp
	if err := json.Unmarshal(body, &app); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	if app.ID == 0 {
		rsp.Code = 400
		rsp.Message = "app id is empty"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	// 获取当前应用状态
	oldApp := g.pam.GetPortmapApp(app.ID.Uint64())
	if oldApp == nil {
		rsp.Code = 404
		rsp.Message = "app not found"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	if app.PeerID != oldApp.PeerID || app.ResID != oldApp.ResID {
		authRsp, err := ResourceAuthorize(g.pe.Libp2pHost(), app.PeerID, AuthorizeReq{
			Type: AuthorizeTypePortmap,
			Portmap: &AuthorizePortmapInfo{
				ResourceID: types.ID(app.ResID),
			},
		})
		if err != nil {
			rsp.Code = 400
			rsp.Message = err.Error()
			apiutil.SendAPIRespWithOk(w, rsp)
			return
		}
		if authRsp.Err != "" {
			rsp.Code = 400
			rsp.Message = authRsp.Err
			apiutil.SendAPIRespWithOk(w, rsp)
			return
		}
		app.PeerName = authRsp.NodeName
	} else {
		app.PeerName = oldApp.PeerName
	}

	// 如果运行状态有变化，则更新listener
	if oldApp.Running {
		g.pm.DeleteListener(oldApp.Network, oldApp.LocalIP, oldApp.LocalPort)

	}
	if app.Running {
		_, err := g.pm.AddListener(app.Network, app.LocalIP, app.LocalPort)
		if err != nil {
			app.Running = false
			app.Err = err.Error()
		}
	}

	if err := g.pam.UpdatePortmapApp(&app); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Message = "ok"
	apiutil.SendAPIRespWithOk(w, rsp)
}

func (g *Gateway) deleteApp(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	var req struct {
		ID types.ID `json:"id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	// 获取应用信息
	app := g.pam.GetPortmapApp(req.ID.Uint64())
	if app != nil && app.Running {
		// 如果应用正在运行，则移除listener
		g.pm.DeleteListener(app.Network, app.LocalIP, app.LocalPort)
	}

	if err := g.pam.DelPortmapApp(req.ID.Uint64()); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Message = "ok"
	apiutil.SendAPIRespWithOk(w, rsp)
}

func (g *Gateway) listApps(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}
	apps := g.pam.GetPortmapApps()
	rsp.Data = apps
	apiutil.SendAPIRespWithOk(w, rsp)
}

func (g *Gateway) getApp(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	app := g.pam.GetPortmapApp(id)
	if app == nil {
		rsp.Code = 404
		rsp.Message = "app not found"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Data = app
	apiutil.SendAPIRespWithOk(w, rsp)
}

// func (g *Gateway) getProxyOBProxy(w http.ResponseWriter, r *http.Request) {
// 	rsp := apiResponse{}
// 	if g.proxySvc == nil {
// 		rsp.Code = 500
// 		rsp.Message = "proxy service not initialized"
// 		sendAPIRespWithOk(w, rsp)
// 		return
// 	}
// 	config := g.proxySvc.getConfig()
// 	rsp.Data = struct {
// 		Addr string `json:"addr"`
// 		User string `json:"user"`
// 		Pass string `json:"pass"`
// 	}{
// 		Addr: config.ProxyAddr,
// 		User: config.ProxyUser,
// 		Pass: config.ProxyPass,
// 	}
// 	sendAPIRespWithOk(w, rsp)
// }

func (g *Gateway) handlePortmapHandshake(handshake []byte) (network string, addr string, port int, err error) {
	pmhs := PortmapAppHandshake{}
	err = json.Unmarshal(handshake, &pmhs)
	if err != nil {
		return
	}
	if g.trial && pmhs.ResID == types.ID(666666) {
		network = pmhs.Network
		addr = pmhs.TargetAddr
		port = pmhs.TargetPort
		return
	}
	pa := g.prm.GetAppByID(pmhs.ResID)
	if pa.ID == 0 {
		err = errors.New("portmap app not found")
		return
	}
	network = pa.Network
	addr = pa.TargetAddr
	port = pa.TargetPort
	return
}

func (g *Gateway) listResources(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}

	rsp.Data = g.prm.GetResources()
	rsp.Message = "ok"
	apiutil.SendAPIRespWithOk(w, rsp)
}

func (g *Gateway) getResource(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	resource := g.prm.GetAppByID(types.ID(id))
	if resource.ID == 0 {
		rsp.Code = 404
		rsp.Message = "resource not found"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Data = resource
	apiutil.SendAPIRespWithOk(w, rsp)
}

func (g *Gateway) addResource(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	var pa PortmapResource
	if err = json.Unmarshal(body, &pa); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	pa.ID = types.ID(rand.Uint64())
	if err = g.prm.AddPortmapRes(&pa); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	rsp.Data = pa.ID
	rsp.Message = "ok"
	apiutil.SendAPIRespWithOk(w, rsp)
}

func (g *Gateway) updateResource(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	var pa PortmapResource
	if err = json.Unmarshal(body, &pa); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	if pa.ID == 0 {
		rsp.Code = 400
		rsp.Message = "resource id is required"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	// 验证资源是否存在
	existing := g.prm.GetAppByID(pa.ID)
	if existing.ID == 0 {
		rsp.Code = 404
		rsp.Message = "resource not found"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	// 验证资源参数
	if err := validatePortmapResource(&pa); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	if err = g.prm.UpdatePortmapRes(&pa); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Message = "ok"
	apiutil.SendAPIRespWithOk(w, rsp)
}

func (g *Gateway) deleteResource(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	var req struct {
		ID types.ID `json:"id"`
	}
	if err = json.Unmarshal(body, &req); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	if req.ID == 0 {
		rsp.Code = 400
		rsp.Message = "resource id is required"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	// 验证资源是否存在
	existing := g.prm.GetAppByID(req.ID)
	if existing.ID == 0 {
		rsp.Code = 404
		rsp.Message = "resource not found"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	if err = g.prm.DelPortmapApp(req.ID); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Message = "ok"
	apiutil.SendAPIRespWithOk(w, rsp)
}

func validatePortmapResource(res *PortmapResource) error {
	if res.Name == "" {
		return errors.New("resource name is required")
	}
	if res.Network != "tcp" && res.Network != "udp" {
		return errors.New("invalid network type, must be tcp or udp")
	}
	if res.TargetAddr == "" {
		return errors.New("target address is required")
	}
	if res.TargetPort <= 0 || res.TargetPort > 65535 {
		return errors.New("invalid target port")
	}
	if res.LocalPort > 0 && res.LocalPort > 65535 {
		return errors.New("invalid local port")
	}
	return nil
}

func (g *Gateway) loadBootstraps() (ret []string) {
	data, err := g.db.Get([]byte(dbKeyBootstraps), nil)
	if err != nil {
		return nil
	}
	err = json.Unmarshal(data, &ret)
	if err != nil {
		return nil
	}
	return
}

func (g *Gateway) getBootstraps() []string {
	bts := g.loadBootstraps()
	if bts == nil {
		bts = append(bts, "/dns6/bootstrap.isletnet.cn/tcp/2025/p2p/12D3KooWPqvupWVWbcjwKkvfBwPi19KerGwEfmWxdyrqRd7AtCaa")
	}
	return bts
}

func (g *Gateway) getToken() (uint64, error) {
	v, err := g.db.Get([]byte(dbKeyToken), nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			return 0, err
		}
		t := rand.Uint64()
		err = g.setToken(t)
		if err != nil {
			return 0, err
		}
		return t, nil
	}
	return strconv.ParseUint(string(v), 10, 64)
}

func (g *Gateway) setToken(t uint64) error {
	ts := strconv.FormatUint(t, 10)
	return g.db.Put([]byte(dbKeyToken), []byte(ts), nil)
}

var (
	gGW    *Gateway
	gwOnce sync.Once
)

type GatewayInfo struct {
	P2PID   string   `json:"p2p_id"`
	Token   types.ID `json:"token"`
	Name    string   `json:"name"`
	Port    int      `json:"running_port"`
	Version string   `json:"version"`
}

func Instance() *Gateway {
	gwOnce.Do(func() {
		gGW = &Gateway{}
	})
	return gGW
}

func (g *Gateway) getGatewayName() (string, error) {
	v, err := g.db.Get([]byte(dbKeyGatewayName), nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return os.Hostname()
		}
		return "", err
	}
	return string(v), nil
}

func (g *Gateway) setGatewayName(name string) error {
	return g.db.Put([]byte(dbKeyGatewayName), []byte(name), nil)
}

func (g *Gateway) getListenPort() (int, error) {
	v, err := g.db.Get([]byte(dbKeyListenPort), nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return 0, nil
		}
		return 0, err
	}
	return strconv.Atoi(string(v))
}

func (g *Gateway) setListenPort(port int) error {
	return g.db.Put([]byte(dbKeyListenPort), []byte(strconv.Itoa(port)), nil)
}

func (g *Gateway) getGatewayInfo(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}

	name, err := g.getGatewayName()
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	token, err := g.getToken()
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}
	if token == 0 {
		token = rand.Uint64()
		err = g.setToken(token)
		if err != nil {
			rsp.Code = 500
			rsp.Message = err.Error()
			apiutil.SendAPIRespWithOk(w, rsp)
			return
		}
	}

	info := GatewayInfo{
		P2PID:   g.pe.Libp2pHost().ID().String(),
		Token:   types.ID(token),
		Name:    name,
		Port:    g.pe.GetListenPort(),
		Version: common.GatewayVersion,
	}

	rsp.Data = info
	apiutil.SendAPIRespWithOk(w, rsp)
}

func (g *Gateway) updateGatewayName(w http.ResponseWriter, r *http.Request) {
	rsp := apiutil.ApiResponse{}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	if req.Name == "" {
		rsp.Code = 400
		rsp.Message = "name is required"
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	if err := g.setGatewayName(req.Name); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		apiutil.SendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Message = "ok"
	apiutil.SendAPIRespWithOk(w, rsp)
}
