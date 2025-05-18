package agent

func Start(workDir string) error {
	return agentIns().start(workDir)
}

func AddApp(a *App) error {
	return agentIns().addApp(a)
}

func UpdateApp(a *App) error {
	return agentIns().updateAPP(a)
}

func DelApp(a *App) error {
	return agentIns().delApp(a)
}

func GetApps() []App {
	return agentIns().getApps()
}
