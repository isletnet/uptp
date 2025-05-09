package main

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/isletnet/uptp/logging"
	"github.com/isletnet/uptp/p2pengine"
	"github.com/isletnet/uptp/portmap"
)

func agentRun(workDir string) error {
	resMgr := appMgr{}
	err := resMgr.loadAppFromFile(filepath.Join(workDir, "res.json"))
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

	pe, err := p2pengine.NewP2PEngine(us, filepath.Join(workDir, "libp2p.log"), filepath.Join(workDir, "dht.db"), false, func() []string {
		return []string{"/ip6/2402:4e00:101a:d400:0:9a33:9051:1549/tcp/2025/p2p/12D3KooWPqvupWVWbcjwKkvfBwPi19KerGwEfmWxdyrqRd7AtCaa"}
	})
	if err != nil {
		return err
	}
	logging.Info("libp2p id: %s", pe.Libp2pHost().ID())

	pmEngine := portmap.NewPortMap(pe.Libp2pHost())
	pmEngine.SetGetHandshakeFunc(func(network, ip string, port int) (peerID string, handshake []byte) {
		rs := resMgr.findAppWithPort(network, port)
		if rs.ResID == 0 {
			return
		}
		peerID = rs.PeerID
		hs := PortmapAppHandshake{
			ResID: rs.ResID,
		}
		handshake, err = json.Marshal(hs)
		if err != nil {
			return
		}
		return
	})
	pmEngine.Start(false)
	for _, r := range resMgr.apps {
		_, err = pmEngine.AddListener(r.Network, r.LocalIP, r.LocalPort)
		if err != nil {
			logging.Error("add portmap listener error: %s", err)
		}
	}
	select {}
}
