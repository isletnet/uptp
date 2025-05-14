package main

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/isletnet/uptp/agent"
	"github.com/isletnet/uptp/logger"
	"github.com/isletnet/uptp/logging"
)

func parseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

func main() {
	baseDir := filepath.Dir(os.Args[0])
	rc := parseRunParams("", os.Args[1:])

	// Initialize logging first
	lm := logger.LogFile
	if rc.verbose {
		lm = logger.LogFileAndConsole
	}
	logging.SetLogger(logger.NewLogger(baseDir, "uptp-agent", rc.logLevel, 1024*1024, lm))

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "add":
			if len(os.Args) < 8 {
				logging.Error("Usage: uptp-agent add <name> <peer_id> <network> <local_ip> <local_port> <target_addr> <target_port>")
				return
			}
			app := &agent.App{
				Name:       os.Args[2],
				PeerID:     os.Args[3],
				Network:    os.Args[4],
				LocalIP:    os.Args[5],
				LocalPort:  parseInt(os.Args[6]),
				TargetAddr: os.Args[7],
				TargetPort: parseInt(os.Args[8]),
				Running:    true,
			}
			if err := agent.AddApps(app, true); err != nil {
				logging.Error("add app error: %v", err)
			}
			return

		case "del":
			if len(os.Args) < 3 {
				logging.Error("Usage: uptp-agent del <name>")
				return
			}
			if err := agent.DelApps(&agent.App{Name: os.Args[2]}, true); err != nil {
				logging.Error("del app error: %v", err)
			}
			return
		}
	}

	// Normal agent start mode
	if err := agent.Start(baseDir); err != nil {
		logging.Error("agent run error: %s", err)
		return
	}
	logging.Info("uptp agent started")
	select {}
}
