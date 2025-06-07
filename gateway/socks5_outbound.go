package gateway

import (
	"encoding/json"
	"sync"

	"github.com/isletnet/uptp/types"
	"github.com/syndtr/goleveldb/leveldb"
)

type socksOutbound struct {
	ID    types.ID `json:"id"`
	Peer  string   `json:"peer"`
	Token types.ID `json:"token"`
	Route string   `json:"route"`
}

var (
	keySocksOutbound = []byte("socks_outbound")
)

type socksOutboundsManager struct {
	db *leveldb.DB

	mtx       sync.Mutex
	outbounds map[types.ID]socksOutbound
}

func NewSocksOutboundsManager(db *leveldb.DB) (*socksOutboundsManager, error) {
	mgr := &socksOutboundsManager{
		db: db,
	}
	err := mgr.loadOutbounds()
	if err != nil {
		return nil, err
	}
	return mgr, nil
}

func (mgr *socksOutboundsManager) AddOutbound(outbound *socksOutbound) error {
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	if mgr.outbounds == nil {
		mgr.outbounds = make(map[types.ID]socksOutbound)
	}
	mgr.outbounds[outbound.ID] = *outbound
	return mgr.saveOutbounds()
}

func (mgr *socksOutboundsManager) UpdateOutbound(outbound *socksOutbound) error {
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	if mgr.outbounds == nil {
		mgr.outbounds = make(map[types.ID]socksOutbound)
	}
	mgr.outbounds[outbound.ID] = *outbound
	return mgr.saveOutbounds()
}

func (mgr *socksOutboundsManager) DeleteOutbound(id types.ID) error {
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	if mgr.outbounds == nil {
		return nil
	}
	delete(mgr.outbounds, id)
	return mgr.saveOutbounds()
}

func (mgr *socksOutboundsManager) GetOutbound(id types.ID) socksOutbound {
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	if mgr.outbounds == nil {
		mgr.outbounds = make(map[types.ID]socksOutbound)
	}
	return mgr.outbounds[id]
}

func (mgr *socksOutboundsManager) ListOutbounds() []socksOutbound {
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	var outbounds []socksOutbound
	for _, ob := range mgr.outbounds {
		outbounds = append(outbounds, ob)
	}
	return outbounds
}

func (mgr *socksOutboundsManager) loadOutbounds() error {
	mgr.mtx.Lock()
	defer mgr.mtx.Unlock()
	v, err := mgr.db.Get(keySocksOutbound, nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			return err
		}
		return nil
	}
	err = json.Unmarshal(v, &mgr.outbounds)
	if err != nil {
		return err
	}
	return nil
}

func (mgr *socksOutboundsManager) saveOutbounds() error {
	if mgr.outbounds == nil {
		return nil
	}
	buf, err := json.Marshal(mgr.outbounds)
	if err != nil {
		return err
	}
	return mgr.db.Put(keySocksOutbound, buf, nil)
}
