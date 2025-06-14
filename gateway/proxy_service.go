package gateway

import (
	"encoding/json"
	"sync"

	leveldb "github.com/syndtr/goleveldb/leveldb"
)

var (
	keyProxyServiceConfig = []byte("proxy_service_config")
)

type proxyServiceConfig struct {
	Route     string `json:"route"`
	DNS       string `json:"dns"`
	ProxyAddr string `json:"proxy_addr"`
	ProxyUser string `json:"proxy_user"`
	ProxyPass string `json:"proxy_pass"`
	// AccessTokens map[uint64]string `json:"access_tokens"`
}
type proxyService struct {
	db *leveldb.DB

	mtx    sync.Mutex
	config proxyServiceConfig
}

// func (ps *proxyService) proxyAuth(token uint64) bool {
// 	config := ps.getConfig()
// 	if config.AccessTokens == nil {
// 		return false
// 	}
// 	_, ok := config.AccessTokens[token]
// 	return ok
// }

func (ps *proxyService) loadConfig() error {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	v, err := ps.db.Get(keyProxyServiceConfig, nil)
	if err != nil {
		if err != leveldb.ErrNotFound {
			return err
		}
		return nil
	}
	err = json.Unmarshal(v, &ps.config)
	if err != nil {
		return err
	}
	return nil
}

func (ps *proxyService) getConfig() proxyServiceConfig {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	return ps.config
}

func (ps *proxyService) set(config proxyServiceConfig) error {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	ps.config = config
	return ps.saveConfig()
}

func (ps *proxyService) setDNS(dns string) error {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	ps.config.DNS = dns
	return ps.saveConfig()
}

func (ps *proxyService) setProxy(addr, user, pass string) error {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	ps.config.ProxyAddr = addr
	ps.config.ProxyUser = user
	ps.config.ProxyPass = pass
	return ps.saveConfig()
}

// func (ps *proxyService) addToken(token uint64, remark string) error {
// 	ps.mtx.Lock()
// 	defer ps.mtx.Unlock()
// 	if ps.config.AccessTokens == nil {
// 		ps.config.AccessTokens = make(map[uint64]string)
// 	}
// 	ps.config.AccessTokens[token] = remark
// 	return ps.saveConfig()
// }

// func (ps *proxyService) removeToken(token uint64) error {
// 	ps.mtx.Lock()
// 	defer ps.mtx.Unlock()
// 	delete(ps.config.AccessTokens, token)
// 	return ps.saveConfig()
// }

func (ps *proxyService) saveConfig() error {
	v, err := json.Marshal(ps.config)
	if err != nil {
		return err
	}
	err = ps.db.Put(keyProxyServiceConfig, v, nil)
	if err != nil {
		return err
	}
	return nil
}
