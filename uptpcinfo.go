package uptp

import (
	"sync"
)

type uptpcInfo struct {
	localIP   string
	mac       string
	os        string
	ipv6      string
	natInfo   natTypeInfo
	pubIPInfo publicIPInfo

	mtx sync.RWMutex
}

func (ui *uptpcInfo) getLocalIP() string {
	ui.mtx.RLock()
	defer ui.mtx.RUnlock()
	return ui.localIP
}
func (ui *uptpcInfo) setLocalIP(ip string) {
	ui.mtx.Lock()
	ui.localIP = ip
	ui.mtx.Unlock()
}
func (ui *uptpcInfo) getOS() string {
	ui.mtx.RLock()
	defer ui.mtx.RUnlock()
	return ui.os
}
func (ui *uptpcInfo) setOS(os string) {
	ui.mtx.Lock()
	ui.os = os
	ui.mtx.Unlock()
}
func (ui *uptpcInfo) getMac() string {
	ui.mtx.RLock()
	defer ui.mtx.RUnlock()
	return ui.mac
}
func (ui *uptpcInfo) setMac(mac string) {
	ui.mtx.Lock()
	ui.mac = mac
	ui.mtx.Unlock()
}
func (ui *uptpcInfo) getIPv6() string {
	ui.mtx.RLock()
	defer ui.mtx.RUnlock()
	return ui.ipv6
}
func (ui *uptpcInfo) setIPv6(ipv6 string) {
	ui.mtx.Lock()
	ui.ipv6 = ipv6
	ui.mtx.Unlock()
}
func (ui *uptpcInfo) getNatTypeInfo() natTypeInfo {
	ui.mtx.RLock()
	defer ui.mtx.RUnlock()
	return ui.natInfo
}
func (ui *uptpcInfo) setNatTypeInfo(info *natTypeInfo) {
	ui.mtx.Lock()
	ui.natInfo = *info
	ui.mtx.Unlock()
}
func (ui *uptpcInfo) getPublicIPInfo() publicIPInfo {
	ui.mtx.RLock()
	defer ui.mtx.RUnlock()
	return ui.pubIPInfo
}
func (ui *uptpcInfo) setPublicIPInfo(info *publicIPInfo) {
	ui.mtx.Lock()
	ui.pubIPInfo = *info
	ui.mtx.Unlock()
}
