package agent

import (
	"crypto/ed25519"
	"encoding/json"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/isletnet/uptp/gateway"
	"github.com/isletnet/uptp/logger"
	"github.com/isletnet/uptp/logging"
	"github.com/isletnet/uptp/p2pengine"
	"github.com/isletnet/uptp/portmap"
	"github.com/isletnet/uptp/types"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

type agent struct {
	p2p *p2pengine.P2PEngine
	pm  *portmap.Portmap
	db  *leveldb.DB
	am  *gateway.PortmapAppMgr

	*proxyMgr

	running bool
}

var (
	gAgent *agent
	gOnce  sync.Once
)

func agentIns() *agent {
	gOnce.Do(func() {
		gAgent = &agent{}
	})
	return gAgent
}

func (ag *agent) setLog(workDir string) {
	gLog := logger.NewLogger(workDir, "uptp-agent", 0, 1024*1024, logger.LogFileAndConsole)
	logging.SetLogger(gLog)
}

func (ag *agent) start(workDir string, withPortmap bool) error {
	if ag.running {
		return nil
	}
	ag.setLog(workDir)
	logging.Info("start agent...")

	var nopts opt.Options

	p := filepath.Join(workDir, "data.db")
	db, err := leveldb.OpenFile(p, &nopts)
	if errors.IsCorrupted(err) && !nopts.GetReadOnly() {
		db, err = leveldb.RecoverFile(p, &nopts)
	}
	if err != nil {
		return err
	}
	ag.db = db

	us, err := os.ReadFile(filepath.Join(workDir, "uuid"))
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
		err = os.WriteFile(filepath.Join(workDir, "uuid"), []byte(str), 0644)
		if err != nil {
			return err
		}
		us = []byte(str)
	}

	ag.p2p, err = p2pengine.NewP2PEngine(0, us, filepath.Join(workDir, "log", "libp2p.log"), filepath.Join(workDir, "dht.db"), true, func() []string {
		return []string{"/ip6/2402:4e00:101a:d400:0:9a33:9051:1549/tcp/2025/p2p/12D3KooWPqvupWVWbcjwKkvfBwPi19KerGwEfmWxdyrqRd7AtCaa"}
	})
	if err != nil {
		return err
	}
	logging.Info("libp2p id: %s", ag.p2p.Libp2pHost().ID())
	if withPortmap {
		ag.startPortmap(workDir)
	}
	ag.proxyMgr = &proxyMgr{db: db}
	err = ag.proxyMgr.loadProxys()
	if err != nil {
		return err
	}
	ag.running = true
	return nil
}
func (ag *agent) startPortmap(workDir string) error {
	apps, err := ag.initAppMgr(workDir)
	if err != nil {
		return err
	}
	ag.pm = portmap.NewPortMap(ag.p2p.Libp2pHost())
	ag.pm.SetGetHandshakeFunc(func(network, ip string, port int) (peerID string, handshake []byte) {
		app := ag.am.FindAppWithPort(network, port)
		if app.ResID == 0 {
			return
		}
		peerID = app.PeerID
		hs := gateway.PortmapAppHandshake{
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
	ag.pm.Start(false)
	for _, a := range apps {
		if !a.Running {
			continue
		}
		_, err = ag.pm.AddListener(a.Network, a.LocalIP, a.LocalPort)
		if err != nil {
			a.Err = ""
			a.Running = false
			ag.am.UpdatePortmapApp(&a)
			logging.Error("add portmap listener error: %s", err)
		}
	}
	return nil
}
func (ag *agent) close() {
	if ag.pm != nil {
		ag.pm.Close()
		ag.pm = nil
	}
	if ag.p2p != nil {
		ag.p2p.Close()
		ag.p2p = nil
	}
	if ag.db != nil {
		ag.db.Close()
		ag.db = nil
	}
}
func (ag *agent) initAppMgr(workDir string) ([]gateway.PortmapApp, error) {
	ag.am = gateway.NewPortmapAppMgr(ag.db)
	apps, err := ag.am.LoadPortmapApps()
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func (ag *agent) addApp(a *gateway.PortmapApp) error {
	a.ID = types.ID(rand.Uint64())
	rsp, err := gateway.ResourceAuthorize(ag.p2p.Libp2pHost(), a.PeerID, gateway.AuthorizeReq{
		Portmap: &gateway.AuthorizePortmapInfo{
			ResourceID: types.ID(a.ResID),
		},
	})
	if err != nil {
		return err
	}
	if rsp.Err != "" {
		return errors.New(rsp.Err)
	}
	if rsp.Portmap == nil {
		return errors.New("portmap resource auth failed")
	}
	if !rsp.Portmap.IsTrial {
		a.TargetAddr = ""
		a.TargetPort = 0
	}
	a.PeerName = rsp.NodeName
	if a.Running {
		_, err := ag.pm.AddListener(a.Network, a.LocalIP, a.LocalPort)
		if err != nil {
			a.Running = false
			a.Err = err.Error()
		}
	}
	return ag.am.UpdatePortmapApp(a)
}

func (ag *agent) updateAPP(a *gateway.PortmapApp) error {
	if a.ID == 0 {
		return errors.New("app id is empty")
	}
	exist := ag.am.GetPortmapApp(a.ID.Uint64())
	if exist == nil {
		return errors.New("app not exists")
	}
	exist.Running = a.Running
	exist.Name = a.Name
	exist.Network = a.Network
	exist.LocalIP = a.LocalIP
	exist.LocalPort = a.LocalPort
	if exist.TargetAddr != "" {
		exist.TargetAddr = a.TargetAddr
		exist.TargetPort = a.TargetPort
	}
	if a.Running {
		if exist.Running &&
			exist.Network != a.Network || exist.LocalIP != a.LocalIP || exist.LocalPort != a.LocalPort {
			ag.pm.DeleteListener(exist.Network, exist.LocalIP, exist.LocalPort)
		}
		_, err := ag.pm.AddListener(a.Network, a.LocalIP, a.LocalPort)
		if err != nil {
			exist.Running = false
			exist.Err = err.Error()
		}
	} else {
		ag.pm.DeleteListener(a.Network, a.LocalIP, a.LocalPort)
	}
	return ag.am.UpdatePortmapApp(exist)
}

func (ag *agent) delApp(a *gateway.PortmapApp) error {
	if !ag.running {
		return errors.New("agent not running")
	}
	ag.pm.DeleteListener(a.Network, a.LocalIP, a.LocalPort)
	return ag.am.DelPortmapApp(a.ID.Uint64())
}

func (ag *agent) getApps() []gateway.PortmapApp {
	if !ag.running {
		return nil
	}
	return ag.am.GetPortmapApps()
}
