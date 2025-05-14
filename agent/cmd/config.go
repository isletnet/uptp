package main

import (
	"flag"

	"github.com/isletnet/uptp/logging"
)

type runConfig struct {
	verbose  bool
	logLevel int
}

func parseRunParams(cmd string, args []string) runConfig {
	var ret runConfig
	if len(args) == 0 {
		return ret
	}
	flagSet := flag.NewFlagSet(cmd, flag.ExitOnError)
	verbose := flagSet.Bool("v", false, "log console")
	logLevel := flagSet.Int("log-level", logging.LevelWarn, "log level")
	flagSet.Parse(args)
	ret.verbose = *verbose
	ret.logLevel = *logLevel
	return ret
}
