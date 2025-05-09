package main

import (
	"os"
	"path/filepath"

	"github.com/isletnet/uptp/logger"
	"github.com/isletnet/uptp/logging"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		default:
		}
	}

	rc := parseRunParams("", os.Args[1:])
	baseDir := filepath.Dir(os.Args[0])
	lm := logger.LogFile
	if rc.verbose {
		lm = logger.LogFileAndConsole
	}

	logging.SetLogger(logger.NewLogger(baseDir, "uptp-agent", rc.logLevel, 1024*1024, lm))

	logging.Info("uptp agent start")

	err := agentRun(baseDir)
	if err != nil {
		logging.Error("agent run error: %s", err)
	}
	logging.Info("uptp agent stopped")
}
