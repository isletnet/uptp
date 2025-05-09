package main

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
	dbKeyToken      = "token"
	dbKeyBootstraps = "bootstraps"
)

type gateway struct {
	pe  *p2pengine.P2PEngine
	pm  *portmap.Portmap
	db  *leveldb.DB
	pam *PortmapResMgr

	token uint64
}

type gatewayConf struct {
	workdir  string
	logDir   string
	logMod   int
	logLevel int
}

func (g *gateway) run(conf gatewayConf) error {
	wd := conf.workdir
	if wd == "" {
		e, err := os.Executable()
		if err != nil {
			return err
		}
		wd = filepath.Dir(e)
	}
	os.Chdir(wd)
	ld := conf.logDir
	if ld == "" {
		ld = "."
	}

	lm := conf.logMod

	gLog := logger.NewLogger(ld, "uptp-gateway", conf.logLevel, 1024*1024, lm)
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

	return apiSer.serve()
}

func (g *gateway) router(ser *apiServer) {
	ser.addRoute("/portmap", func(r chi.Router) {
		r.Get("/list_resources", g.listPortmapResources)
		r.Post("/add_resource", g.addPortmapResource)
		r.Post("/delete_resource", g.deletePortmapResource)
	})
}

func (g *gateway) handlePortmapHandshake(handshake []byte) (network string, addr string, port int, err error) {
	pmhs := PortmapAppHandshake{}
	err = json.Unmarshal(handshake, &pmhs)
	if err != nil {
		return
	}
	pa := g.pam.GetAppByID(pmhs.AppID)
	if pa.ID == 0 {
		err = errors.New("portmap app not found")
		return
	}
	network = pa.Network
	addr = pa.TargetAddr
	port = pa.TargetPort
	return
}

func (g *gateway) listPortmapResources(w http.ResponseWriter, r *http.Request) {
	rsp := apiResponse{}

	rsp.Data = g.pam.GetResources()
	rsp.Message = "ok"
	sendAPIRespWithOk(w, rsp)
}
func (g *gateway) addPortmapResource(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	var pa PortmapResource
	if err = json.Unmarshal(body, &pa); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	pa.ID = rand.Uint64()
	if err = g.pam.AddPortmapRes(&pa); err != nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(err.Error()))
		return
	}
	sendAPIRespWithOk(w, apiResponse{
		Code:    0,
		Message: "ok",
		Data:    pa.ID,
	})
}
func (g *gateway) deletePortmapResource(w http.ResponseWriter, r *http.Request) {

}

func (g *gateway) loadBootstraps() (ret []string) {
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

func (g *gateway) getBootstraps() []string {
	bts := g.loadBootstraps()
	if bts == nil {
		bts = append(bts, "/ip6/2402:4e00:101a:d400:0:9a33:9051:1549/tcp/2025/p2p/12D3KooWPqvupWVWbcjwKkvfBwPi19KerGwEfmWxdyrqRd7AtCaa")
	}
	return bts
}

func (g *gateway) getToken() (uint64, error) {
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

func (g *gateway) setToken(t uint64) error {
	ts := strconv.FormatUint(t, 10)
	return g.db.Put([]byte(dbKeyToken), []byte(ts), nil)
}

var (
	gGW    *gateway
	gwOnce sync.Once
)

func gwIns() *gateway {
	gwOnce.Do(func() {
		gGW = &gateway{}
	})
	return gGW
}
