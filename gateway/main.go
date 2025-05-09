package main

import (
	"os"
	"path/filepath"

	"github.com/isletnet/uptp/logger"
	"github.com/isletnet/uptp/logging"
	// "github.com/isletnet/machineid"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			install()
			return
		case "uninstall":
			uninstall()
			return
		case "start":
			serviceControl("start", "", nil)
			return
		case "restart":
			serviceControl("restart", "", nil)
			return
		case "stop":
			serviceControl("stop", "", nil)
			return
		case "isletid":
			// gLog := NewLogger("", "", logging.LevelDebug, 0, LogConsole)
			// isletid, err := machineid.ProtectedID("isletnet")
			// if err != nil {
			// 	fmt.Println(err)
			// 	return
			// }
			// fmt.Println(isletid)
			return
		}
	}

	rc := parseRunParams("", os.Args[1:])
	baseDir := filepath.Dir(os.Args[0])
	if rc.daemonMode {
		// tmpLog.Info("run in daemon mode")
		gLog := logger.NewLogger(baseDir, "daemon", rc.logLevel, 1024*1024, logger.LogFile)
		logging.SetLogger(gLog)
		daemonStart()
		return
	}
	lm := logger.LogFile
	if rc.verbose {
		lm = logger.LogFileAndConsole
	}
	if rc.trial {
		gwIns().setTrialMod()
	}
	if err := gwIns().run(gatewayConf{
		logMod:   lm,
		logLevel: rc.logLevel,
	}); err != nil {
		logging.Error("gateway run error: %s", err)
	}
}
