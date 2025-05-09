package main

import (
	"encoding/json"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
)

type PortmapApp struct {
	ID         uint64 `json:"id"`
	Name       string `json:"name"`
	Type       int    `json:"type"`
	Network    string `json:"network"`
	TargetAddr string `json:"target_addr"`
	TargetPort int    `json:"target_port"`
	LocalIP    string `json:"local_ip"`
	LocalPort  int    `json:"local_port"`
}

var (
	keyPortmapApps = []byte("portmap_apps")
)

type PortmapAppMgr struct {
	db *leveldb.DB

	appMtx sync.Mutex
	apps   map[uint64]PortmapApp
}

func NewPortmapAppMgr(db *leveldb.DB) (*PortmapAppMgr, error) {
	ret := &PortmapAppMgr{
		db: db,
	}
	err := ret.loadApp()
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (pam *PortmapAppMgr) AddPortmapApp(app *PortmapApp) error {
	pam.appMtx.Lock()
	defer pam.appMtx.Unlock()
	if pam.apps == nil {
		pam.apps = make(map[uint64]PortmapApp)
	}
	pam.apps[app.ID] = *app
	err := pam.savePortmap()
	if err != nil {
		return err
	}
	return nil
}

func (pam *PortmapAppMgr) DelPortmapApp(appID uint64) error {
	pam.appMtx.Lock()
	defer pam.appMtx.Unlock()
	if pam.apps == nil {
		return nil
	}
	delete(pam.apps, appID)
	err := pam.savePortmap()
	if err != nil {
		return err
	}
	return nil
}

func (pam *PortmapAppMgr) GetAppByID(appID uint64) PortmapApp {
	pam.appMtx.Lock()
	defer pam.appMtx.Unlock()
	if pam.apps == nil {
		pam.apps = make(map[uint64]PortmapApp)
	}
	return pam.apps[appID]
}

func (pam *PortmapAppMgr) GetApps() (apps []PortmapApp) {
	pam.appMtx.Lock()
	defer pam.appMtx.Unlock()
	for _, app := range pam.apps {
		apps = append(apps, app)
	}
	return
}

func (pam *PortmapAppMgr) loadApp() error {
	pam.appMtx.Lock()
	defer pam.appMtx.Unlock()
	v, err := pam.db.Get(keyPortmapApps, nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			return err
		}
		return nil
	}
	err = json.Unmarshal(v, &pam.apps)
	if err != nil {
		return err
	}
	return nil
}

func (pam *PortmapAppMgr) savePortmap() error {
	// pam.appMtx.Lock()
	// defer pam.appMtx.Unlock()

	if pam.apps == nil {
		return nil
	}
	buf, err := json.Marshal(pam.apps)
	if err != nil {
		return err
	}
	err = pam.db.Put(keyPortmapApps, buf, nil)
	if err != nil {
		return err
	}
	return nil
}

type PortmapAppHandshake struct {
	AppID uint64 `json:"app_id"`
}
