package agent

import (
	"encoding/json"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
)

var (
	keyPortmapApps = []byte("portmap_apps")
)

type PortmapApp struct {
	ID         uint64 `json:"id"`
	Name       string `json:"name"`
	PeerID     string `json:"peer_id"`
	ResID      uint64 `json:"res_id"`
	Network    string `json:"network"`
	LocalIP    string `json:"local_ip"`
	LocalPort  int    `json:"local_port"`
	TargetAddr string `json:"target_addr"`
	TargetPort int    `json:"target_port"`
	Running    bool   `json:"running"`

	PeerName string `json:"peer_name"`
	Err      string `json:"-"`
}

type PortmapAppMgr struct {
	db *leveldb.DB

	mtx  sync.Mutex
	apps map[uint64]PortmapApp
}

func NewPortmapAppMgr(db *leveldb.DB) *PortmapAppMgr {
	return &PortmapAppMgr{
		db:   db,
		apps: make(map[uint64]PortmapApp),
	}
}

func (m *PortmapAppMgr) LoadPortmapApps() ([]PortmapApp, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	v, err := m.db.Get(keyPortmapApps, nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			return nil, err
		}
		return nil, nil
	}
	err = json.Unmarshal(v, &m.apps)
	if err != nil {
		return nil, err
	}
	return m.GetPortmapApps(), nil
}

func (m *PortmapAppMgr) GetPortmapApps() []PortmapApp {
	ret := make([]PortmapApp, 0, len(m.apps))
	for _, v := range m.apps {
		ret = append(ret, v)
	}
	return ret
}

func (m *PortmapAppMgr) GetPortmapApp(id uint64) *PortmapApp {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	a, ok := m.apps[id]
	if !ok {
		return nil
	}
	return &a
}

func (m *PortmapAppMgr) UpdatePortmapApp(a *PortmapApp) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.apps[a.ID] = *a
	return m.savePortmap()
}

func (m *PortmapAppMgr) DelPortmapApp(id uint64) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	delete(m.apps, id)
	return m.savePortmap()
}

func (m *PortmapAppMgr) savePortmap() error {
	if m.apps == nil {
		return m.db.Delete(keyPortmapApps, nil)
	}
	buf, err := json.Marshal(m.apps)
	if err != nil {
		return err
	}
	err = m.db.Put(keyPortmapApps, buf, nil)
	if err != nil {
		return err
	}
	return nil
}

func (m *PortmapAppMgr) findAppWithPort(network string, port int) PortmapApp {
	for _, r := range m.apps {
		if r.LocalPort == port && r.Network == network {
			return r
		}
	}
	return PortmapApp{}
}
