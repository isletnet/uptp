package main

import (
	"encoding/json"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
)

type PortmapResource struct {
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
	keyPortmapRes = []byte("portmap_resources")
)

type PortmapResMgr struct {
	db *leveldb.DB

	resMtx    sync.Mutex
	resources map[uint64]PortmapResource
}

func NewPortmapResMgr(db *leveldb.DB) (*PortmapResMgr, error) {
	ret := &PortmapResMgr{
		db: db,
	}
	err := ret.loadRes()
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (pam *PortmapResMgr) AddPortmapRes(res *PortmapResource) error {
	pam.resMtx.Lock()
	defer pam.resMtx.Unlock()
	if pam.resources == nil {
		pam.resources = make(map[uint64]PortmapResource)
	}
	pam.resources[res.ID] = *res
	err := pam.savePortmap()
	if err != nil {
		return err
	}
	return nil
}

func (pam *PortmapResMgr) DelPortmapApp(resID uint64) error {
	pam.resMtx.Lock()
	defer pam.resMtx.Unlock()
	if pam.resources == nil {
		return nil
	}
	delete(pam.resources, resID)
	err := pam.savePortmap()
	if err != nil {
		return err
	}
	return nil
}

func (pam *PortmapResMgr) GetAppByID(resID uint64) PortmapResource {
	pam.resMtx.Lock()
	defer pam.resMtx.Unlock()
	if pam.resources == nil {
		pam.resources = make(map[uint64]PortmapResource)
	}
	return pam.resources[resID]
}

func (pam *PortmapResMgr) GetResources() (ress []PortmapResource) {
	pam.resMtx.Lock()
	defer pam.resMtx.Unlock()
	for _, r := range pam.resources {
		ress = append(ress, r)
	}
	return
}

func (pam *PortmapResMgr) loadRes() error {
	pam.resMtx.Lock()
	defer pam.resMtx.Unlock()
	v, err := pam.db.Get(keyPortmapRes, nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			return err
		}
		return nil
	}
	err = json.Unmarshal(v, &pam.resources)
	if err != nil {
		return err
	}
	return nil
}

func (pam *PortmapResMgr) savePortmap() error {
	// pam.appMtx.Lock()
	// defer pam.appMtx.Unlock()

	if pam.resources == nil {
		return nil
	}
	buf, err := json.Marshal(pam.resources)
	if err != nil {
		return err
	}
	err = pam.db.Put(keyPortmapRes, buf, nil)
	if err != nil {
		return err
	}
	return nil
}

type PortmapAppHandshake struct {
	ResID      uint64 `json:"res_id"`
	Network    string `json:"network"`
	TargetAddr string `json:"target_addr"`
	TargetPort int    `json:"target_port"`
}
