package agent

func Start(workDir string) error {
	return agentIns().start(workDir)
}

func AddApps(a *App, editOnly bool) error {
	return agentIns().addApps(a, editOnly)
}

func DelApps(a *App, editOnly bool) error {
	return agentIns().delApps(a, editOnly)
}
