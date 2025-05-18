package gateway

import (
	"crypto/ed25519"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/isletnet/uptp/logger"
	"github.com/isletnet/uptp/logging"
	"github.com/isletnet/uptp/p2pengine"
	"github.com/isletnet/uptp/portmap"
	"github.com/isletnet/uptp/types"

	"github.com/go-chi/chi"
	"github.com/google/uuid"
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
)

type Gateway struct {
	pe  *p2pengine.P2PEngine
	pm  *portmap.Portmap
	db  *leveldb.DB
	pam *PortmapResMgr

	trial bool
	token uint64
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
	wd := conf.Workdir
	if wd == "" {
		e, err := os.Executable()
		if err != nil {
			return err
		}
		wd = filepath.Dir(e)
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
	pe, err := p2pengine.NewP2PEngine(us, filepath.Join(ld, "libp2p.log"), "dht.db", false, g.getBootstraps)
	if err != nil {
		return err
	}
	logging.Info("libp2p id: %s", pe.Libp2pHost().ID())
	g.pe = pe

	pam, err := NewPortmapResMgr(db)
	if err != nil {
		return err
	}
	g.pam = pam

	g.pm = portmap.NewPortMap(pe.Libp2pHost())
	g.pm.SetHandleHandshakeFunc(g.handlePortmapHandshake)
	g.pm.Start(true)

	apiSer := &apiServer{}
	g.router(apiSer)
	g.authorize()

	return apiSer.serve()
}

func (g *Gateway) router(ser *apiServer) {
	ser.addRoute("/resource", func(r chi.Router) {
		r.Get("/list", g.listResources)
		r.Get("/get/{id}", g.getResource)
		r.Post("/add", g.addResource)
		r.Post("/update", g.updateResource)
		r.Post("/delete", g.deleteResource)
	})

	ser.addRoute("/gateway", func(r chi.Router) {
		r.Get("/info", g.getGatewayInfo)
		r.Post("/name", g.updateGatewayName)
	})

	// 使用嵌入的静态文件
	fileServer := http.FileServer(getWebFS())
	ser.addRoute("/", func(r chi.Router) {
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			fileServer.ServeHTTP(w, r)
		})
	})
}

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
	pa := g.pam.GetAppByID(pmhs.ResID)
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
	rsp := apiResponse{}

	rsp.Data = g.pam.GetResources()
	rsp.Message = "ok"
	sendAPIRespWithOk(w, rsp)
}

func (g *Gateway) getResource(w http.ResponseWriter, r *http.Request) {
	rsp := apiResponse{}
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	resource := g.pam.GetAppByID(types.ID(id))
	if resource.ID == 0 {
		rsp.Code = 404
		rsp.Message = "resource not found"
		sendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Data = resource
	sendAPIRespWithOk(w, rsp)
}

func (g *Gateway) addResource(w http.ResponseWriter, r *http.Request) {
	rsp := apiResponse{}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}
	var pa PortmapResource
	if err = json.Unmarshal(body, &pa); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}
	pa.ID = types.ID(rand.Uint64())
	if err = g.pam.AddPortmapRes(&pa); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}
	rsp.Data = pa.ID
	rsp.Message = "ok"
	sendAPIRespWithOk(w, rsp)
}

func (g *Gateway) updateResource(w http.ResponseWriter, r *http.Request) {
	rsp := apiResponse{}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}
	var pa PortmapResource
	if err = json.Unmarshal(body, &pa); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}

	if pa.ID == 0 {
		rsp.Code = 400
		rsp.Message = "resource id is required"
		sendAPIRespWithOk(w, rsp)
		return
	}

	// 验证资源是否存在
	existing := g.pam.GetAppByID(pa.ID)
	if existing.ID == 0 {
		rsp.Code = 404
		rsp.Message = "resource not found"
		sendAPIRespWithOk(w, rsp)
		return
	}

	// 验证资源参数
	if err := validatePortmapResource(&pa); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}

	if err = g.pam.UpdatePortmapRes(&pa); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Message = "ok"
	sendAPIRespWithOk(w, rsp)
}

func (g *Gateway) deleteResource(w http.ResponseWriter, r *http.Request) {
	rsp := apiResponse{}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}
	var req struct {
		ID types.ID `json:"id"`
	}
	if err = json.Unmarshal(body, &req); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}

	if req.ID == 0 {
		rsp.Code = 400
		rsp.Message = "resource id is required"
		sendAPIRespWithOk(w, rsp)
		return
	}

	// 验证资源是否存在
	existing := g.pam.GetAppByID(req.ID)
	if existing.ID == 0 {
		rsp.Code = 404
		rsp.Message = "resource not found"
		sendAPIRespWithOk(w, rsp)
		return
	}

	if err = g.pam.DelPortmapApp(req.ID); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Message = "ok"
	sendAPIRespWithOk(w, rsp)
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
	P2PID string `json:"p2p_id"`
	Name  string `json:"name"`
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

func (g *Gateway) getGatewayInfo(w http.ResponseWriter, r *http.Request) {
	rsp := apiResponse{}

	name, err := g.getGatewayName()
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}

	info := GatewayInfo{
		P2PID: g.pe.Libp2pHost().ID().String(),
		Name:  name,
	}

	rsp.Data = info
	sendAPIRespWithOk(w, rsp)
}

func (g *Gateway) updateGatewayName(w http.ResponseWriter, r *http.Request) {
	rsp := apiResponse{}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		rsp.Code = 400
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}

	if req.Name == "" {
		rsp.Code = 400
		rsp.Message = "name is required"
		sendAPIRespWithOk(w, rsp)
		return
	}

	if err := g.setGatewayName(req.Name); err != nil {
		rsp.Code = 500
		rsp.Message = err.Error()
		sendAPIRespWithOk(w, rsp)
		return
	}

	rsp.Message = "ok"
	sendAPIRespWithOk(w, rsp)
}
