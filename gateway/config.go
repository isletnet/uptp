package main

import (
	"flag"

	"github.com/isletnet/uptp/logging"
)

type runConfig struct {
	daemonMode bool
	verbose    bool
	logLevel   int
	trial      bool
}

func parseRunParams(cmd string, args []string) runConfig {
	var ret runConfig
	if len(args) == 0 {
		return ret
	}
	flagSet := flag.NewFlagSet(cmd, flag.ExitOnError)
	daemonMode := flagSet.Bool("d", false, "daemonMode")
	verbose := flagSet.Bool("v", false, "log console")
	trial := flagSet.Bool("trial", false, "trial mod")
	logLevel := flagSet.Int("log-level", logging.LevelWarn, "log level")
	flagSet.Parse(args)
	ret.daemonMode = *daemonMode
	ret.verbose = *verbose
	ret.logLevel = *logLevel
	ret.trial = *trial
	return ret
}
