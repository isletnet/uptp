package main

import (
	"encoding/json"
	"os"
)

type resource struct {
	PeerID    string `json:"peer_id"`
	AppID     uint64 `json:"app_id"`
	Network   string `json:"network"`
	LocalIP   string `json:"local_ip"`
	LocalPort int    `json:"local_port"`
}

type resourcesMgr struct {
	resList []resource
}

func (m *resourcesMgr) loadResourcesFromFile(fp string) error {
	buf, err := os.ReadFile(fp)
	if err != nil {
		return err
	}
	err = json.Unmarshal(buf, &m.resList)
	if err != nil {
		return err
	}
	return nil
}

func (m *resourcesMgr) findResourceWithPort(network string, port int) resource {
	for _, r := range m.resList {
		if r.LocalPort == port && r.Network == network {
			return r
		}
	}
	return resource{}
}

type PortmapAppHandshake struct {
	AppID uint64 `json:"app_id"`
}
