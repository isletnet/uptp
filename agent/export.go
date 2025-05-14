package agent

func Start(workDir string) error {
	return agentIns().start(workDir)
}

func AddApps(a *App) error {
	return agentIns().addApps(a)
}

func DelApps(a *App) error {
	return agentIns().delApps(a)
}
