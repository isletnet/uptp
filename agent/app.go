package main

import (
	"encoding/json"
	"os"

	"github.com/isletnet/uptp/types"
)

type app struct {
	PeerID     string `json:"peer_id"`
	ResID      uint64 `json:"res_id"`
	Network    string `json:"network"`
	LocalIP    string `json:"local_ip"`
	LocalPort  int    `json:"local_port"`
	TargetAddr string `json:"target_addr"`
	TargetPort int    `json:"target_port"`
}

type appMgr struct {
	apps []app
}

func (m *appMgr) loadAppFromFile(fp string) error {
	buf, err := os.ReadFile(fp)
	if err != nil {
		return err
	}
	err = json.Unmarshal(buf, &m.apps)
	if err != nil {
		return err
	}
	return nil
}

func (m *appMgr) findAppWithPort(network string, port int) app {
	for _, r := range m.apps {
		if r.LocalPort == port && r.Network == network {
			return r
		}
	}
	return app{}
}

type PortmapAppHandshake struct {
	ResID      types.ID `json:"res_id"`
	Network    string   `json:"network"`
	TargetAddr string   `json:"target_addr"`
	TargetPort int      `json:"target_port"`
}
