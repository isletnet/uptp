package main

import (
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/isletnet/uptp/gateway"
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
		os.Chdir(baseDir)
		daemonStart()
		return
	}
	lm := logger.LogFile
	if rc.verbose {
		lm = logger.LogFileAndConsole
	}
	if rc.trial {
		gateway.Instance().SetTrialMod()
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := gateway.Instance().Run(gateway.Config{
			LogMod:   lm,
			LogLevel: rc.logLevel,
		}); err != nil {
			logging.Error("gateway run error: %s", err)
		}
		logging.Info("uptp gateway run end")
	}()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigChan:
	case <-gateway.Instance().ExitSignalChan():
	}
	gateway.Instance().Stop()
	wg.Wait()
}
