package agent

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
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
	am  *appMgr
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

func (ag *agent) start(workDir string) error {
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

	ag.am = newAppMgr(db)
	apps, err := ag.am.loadApps()
	if err != nil {
		return err
	}

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

	ag.p2p, err = p2pengine.NewP2PEngine(us, filepath.Join(workDir, "libp2p.log"), filepath.Join(workDir, "dht.db"), true, func() []string {
		return []string{"/dns6/bootstrap.isletnet.cn/tcp/2025/p2p/12D3KooWPqvupWVWbcjwKkvfBwPi19KerGwEfmWxdyrqRd7AtCaa"}
	})
	if err != nil {
		return err
	}
	logging.Info("libp2p id: %s", ag.p2p.Libp2pHost().ID())

	ag.pm = portmap.NewPortMap(ag.p2p.Libp2pHost())
	ag.pm.SetGetHandshakeFunc(func(network, ip string, port int) (peerID string, handshake []byte) {
		app := ag.am.findAppWithPort(network, port)
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
	ag.pm.Start(false)
	for _, a := range apps {
		if !a.Running {
			continue
		}
		_, err = ag.pm.AddListener(a.Network, a.LocalIP, a.LocalPort)
		if err != nil {
			a.Err = err
			a.Running = false
			ag.am.updateApp(&a)
			logging.Error("add portmap listener error: %s", err)
		}
	}
	return nil
}

func (ag *agent) addApps(a *App) error {
	err := ag.am.updateApp(a)
	if err != nil {
		return err
	}
	if a.Running {
		_, err := ag.pm.AddListener(a.Network, a.LocalIP, a.LocalPort)
		if err != nil {
			a.Err = err
			a.Running = false
			ag.am.updateApp(a)
			return err
		}
	}
	return nil
}

func (ag *agent) delApps(a *App) error {
	ag.pm.DeleteListener(a.Network, a.LocalIP, a.LocalPort)
	return ag.am.delApp(a.Name)
}
