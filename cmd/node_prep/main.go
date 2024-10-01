// Copyright 2024 NetApp, Inc. All Rights Reserved.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/nodeprep"
	. "github.com/netapp/trident/logging"
)

func main() {
	flags := initFlags()

	initLogging(flags)

	Log().WithFields(LogFields{
		"version":    config.OrchestratorVersion.String(),
		"build_time": config.BuildTime,
		"binary":     os.Args[0],
	}).Info("Running Trident node preparation.")

	ctx := context.Background()

	// This is a work-around for osutils because getting system information checks for this environment variable
	// to decide if this is running inside a container.  osutils should be refactored to not rely on this.
	if err := os.Setenv("CSI_ENDPOINT", "unix://run/csi/socket"); err != nil {
		Log().Fatal("Failed to set environment variable.")
	}

	os.Exit(nodeprep.New().Prepare(ctx, strings.Split(strings.ToLower(*flags.nodePrep), ",")))
}

func initLogging(flags appFlags) {
	if err := setLoggingFlags(flags); err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		println("Failed to initialize logging, attempting default settings")
		*flags.logLevel = "info"
		*flags.logFormat = "text"
		if err = setLoggingFlags(flags); err != nil {
			_, _ = fmt.Fprint(os.Stderr, err)
			println("Failed to initialize logging")
		}
	}
}

func setLoggingFlags(flags appFlags) error {
	// if debug override log level
	if *flags.debug {
		*flags.logLevel = "debug"
	}

	if err := InitLogLevel(*flags.logLevel); err != nil {
		return err
	}

	if err := InitLogFormat(*flags.logFormat); err != nil {
		return err
	}

	return nil
}

type appFlags struct {
	logFormat *string
	logLevel  *string
	debug     *bool
	nodePrep  *string
}

func initFlags() appFlags {
	flags := appFlags{
		nodePrep:  flag.String("node-prep", "", "List of protocols to prepare node with"),
		logFormat: flag.String("log-format", "text", "Logging format (text, json)"),
		logLevel:  flag.String("log-level", "info", "Logging level (trace, debug, info, warn, error, fatal)"),
		debug:     flag.Bool("debug", false, "Enable debugging output"),
	}
	flag.Parse()
	return flags
}
