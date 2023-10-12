package main

import "flag"

type runConfig struct {
	daemonMode bool
	verbose    bool
}

func parseRunParams(cmd string, args []string) runConfig {
	var ret runConfig
	if len(args) == 0 {
		return ret
	}
	flagSet := flag.NewFlagSet(cmd, flag.ExitOnError)
	daemonMode := flagSet.Bool("d", false, "daemonMode")
	verbose := flagSet.Bool("v", false, "log console")
	flagSet.Parse(args)
	ret.daemonMode = *daemonMode
	ret.verbose = *verbose
	return ret
}
