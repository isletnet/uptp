package agent

import "encoding/json"

func Start(workDir string, withPortmap bool) error {
	return agentIns().start(workDir, withPortmap)
}

func Close() {
	agentIns().close()
}

func AddApp(a *PortmapApp) error {
	return agentIns().addApp(a)
}

func UpdateApp(a *PortmapApp) error {
	return agentIns().updateAPP(a)
}

func DelApp(a *PortmapApp) error {
	return agentIns().delApp(a)
}

func GetApps() []PortmapApp {
	return agentIns().getApps()
}

func AddProxyGateway(peerID string, token string) error {
	return agentIns().addProxyGateway(peerID, token)
}

func GetProxyGateways() []string {
	return agentIns().getProxyGatewayList()
}

func GetProxyGatewaysJson() string {
	l := GetProxyGateways()
	if l == nil {
		return ""
	}
	buf, _ := json.Marshal(l)
	return string(buf)
}

func StartTunProxy(tunDevice string, gatewayIdx int) error {
	return agentIns().startTunProxy(tunDevice, gatewayIdx)
}

func StopTunProxy() error {
	return agentIns().stopTunProxy()
}

// func SetLog(d string) {
// 	agentIns().setLog(d)
// }

func PingProxyGateway(idx int) error {
	return agentIns().pingProxyGateway(idx)
}
