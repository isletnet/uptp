package gateway

import (
	"encoding/json"
	"sync"

	"github.com/isletnet/uptp/types"
	"github.com/syndtr/goleveldb/leveldb"
)

type PortmapResource struct {
	ID         types.ID `json:"id"`
	Name       string   `json:"name"`
	Type       int      `json:"type"`
	Network    string   `json:"network"`
	TargetAddr string   `json:"target_addr"`
	TargetPort int      `json:"target_port"`
	LocalIP    string   `json:"local_ip"`
	LocalPort  int      `json:"local_port"`
}

var (
	keyPortmapRes = []byte("portmap_resources")
)

type PortmapResMgr struct {
	db *leveldb.DB

	resMtx    sync.Mutex
	resources map[types.ID]PortmapResource
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
		pam.resources = make(map[types.ID]PortmapResource)
	}
	pam.resources[res.ID] = *res
	err := pam.savePortmap()
	if err != nil {
		return err
	}
	return nil
}

func (pam *PortmapResMgr) DelPortmapApp(resID types.ID) error {
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

func (pam *PortmapResMgr) UpdatePortmapRes(res *PortmapResource) error {
	pam.resMtx.Lock()
	defer pam.resMtx.Unlock()
	if pam.resources == nil {
		pam.resources = make(map[types.ID]PortmapResource)
	}
	pam.resources[res.ID] = *res
	err := pam.savePortmap()
	if err != nil {
		return err
	}
	return nil
}

func (pam *PortmapResMgr) GetAppByID(resID types.ID) PortmapResource {
	pam.resMtx.Lock()
	defer pam.resMtx.Unlock()
	if pam.resources == nil {
		pam.resources = make(map[types.ID]PortmapResource)
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
