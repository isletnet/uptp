package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
)

// ResourceID 自定义资源ID类型
type ResourceID uint64

// MarshalJSON 实现 json.Marshaler 接口
func (id ResourceID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
func (id *ResourceID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	var v uint64
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return err
	}
	*id = ResourceID(v)
	return nil
}

// String 实现 fmt.Stringer 接口
func (id ResourceID) String() string {
	return fmt.Sprintf("%d", uint64(id))
}

// Uint64 转换为 uint64
func (id ResourceID) Uint64() uint64 {
	return uint64(id)
}

type PortmapResource struct {
	ID         ResourceID `json:"id"`
	Name       string     `json:"name"`
	Type       int        `json:"type"`
	Network    string     `json:"network"`
	TargetAddr string     `json:"target_addr"`
	TargetPort int        `json:"target_port"`
	LocalIP    string     `json:"local_ip"`
	LocalPort  int        `json:"local_port"`
}

var (
	keyPortmapRes = []byte("portmap_resources")
)

type PortmapResMgr struct {
	db *leveldb.DB

	resMtx    sync.Mutex
	resources map[ResourceID]PortmapResource
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
		pam.resources = make(map[ResourceID]PortmapResource)
	}
	pam.resources[res.ID] = *res
	err := pam.savePortmap()
	if err != nil {
		return err
	}
	return nil
}

func (pam *PortmapResMgr) DelPortmapApp(resID ResourceID) error {
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
		pam.resources = make(map[ResourceID]PortmapResource)
	}
	pam.resources[res.ID] = *res
	err := pam.savePortmap()
	if err != nil {
		return err
	}
	return nil
}

func (pam *PortmapResMgr) GetAppByID(resID ResourceID) PortmapResource {
	pam.resMtx.Lock()
	defer pam.resMtx.Unlock()
	if pam.resources == nil {
		pam.resources = make(map[ResourceID]PortmapResource)
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

type PortmapAppHandshake struct {
	ResID      ResourceID `json:"res_id"`
	Network    string     `json:"network"`
	TargetAddr string     `json:"target_addr"`
	TargetPort int        `json:"target_port"`
}
