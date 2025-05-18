package agent

import (
	"encoding/json"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
)

var (
	keyPortmapApps = []byte("portmap_apps")
)

type App struct {
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

type appMgr struct {
	db *leveldb.DB

	mtx  sync.Mutex
	apps map[uint64]App
}

func newAppMgr(db *leveldb.DB) *appMgr {
	return &appMgr{
		db:   db,
		apps: make(map[uint64]App),
	}
}

func (m *appMgr) loadApps() ([]App, error) {
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
	return m.getApps(), nil
}

func (m *appMgr) getApps() []App {
	ret := make([]App, 0, len(m.apps))
	for _, v := range m.apps {
		ret = append(ret, v)
	}
	return ret
}

func (m *appMgr) getApp(id uint64) *App {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	a, ok := m.apps[id]
	if !ok {
		return nil
	}
	return &a
}

func (m *appMgr) updateApp(a *App) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	m.apps[a.ID] = *a
	return m.savePortmap()
}

func (m *appMgr) delApp(id uint64) error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	delete(m.apps, id)
	return m.savePortmap()
}

func (m *appMgr) savePortmap() error {
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

func (m *appMgr) findAppWithPort(network string, port int) App {
	for _, r := range m.apps {
		if r.LocalPort == port && r.Network == network {
			return r
		}
	}
	return App{}
}
