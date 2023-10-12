package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/isletnet/machineid"
	"github.com/isletnet/uptp"
	"github.com/isletnet/uptp/logging"
)

const Version = "0.1.0"
const ProductName = "isletPortmap"

var gPortMapClient *portMapClient

func main() {
	// tmpLog := NewLogger("", "", logging.LevelDebug, 1024*1024, LogConsole)
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
			isletid, err := machineid.ProtectedID("isletnet")
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Println(isletid)
			return
		}
	}
	rc := parseRunParams("", os.Args[1:])
	baseDir := filepath.Dir(os.Args[0])
	os.Chdir(baseDir)
	if rc.daemonMode {
		// tmpLog.Info("run in daemon mode")
		gLog := NewLogger(baseDir, "daemon", logging.LevelDebug, 1024*1024, LogFile)
		logging.SetLogger(gLog)
		daemonStart()
		return
	}
	lm := LogFile
	if rc.verbose {
		lm = LogFileAndConsole
	}
	gLog := NewLogger(baseDir, "portmap", logging.LevelInfo, 1024*1024, lm)
	logging.SetLogger(gLog)

	nc, err := loadNptpcConfig("config.yml")
	if err != nil {
		logging.Error("load nptp client config fail: %s", err)
		return
	}
	logging.SetLevel(nc.LogLevel)

	uptpcOpt := uptp.UptpcOption{
		Name:             nc.PortMapServiceConfig.NodeName,
		UptpServerOption: *nc.UptpServerConfig,
	}
	uptpc := uptp.NewUptpc(uptpcOpt)
	uptpc.Start()
	logging.Info("uptpc start")
	gPortMapClient = NewPortMapClient(nc.PortMapServiceConfig)
	gPortMapClient.Init(uptpc)

	gPortMapClient.Start()
	logging.Info("port map client start")
	for {
		logging.Info("run all port map")
		gPortMapClient.runAllPortMap(nc.PortMapConfig)
		time.Sleep(time.Minute * 10)
	}
}
