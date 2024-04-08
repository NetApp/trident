// Copyright 2022 NetApp, Inc. All Rights Reserved.

package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	. "github.com/netapp/trident/logging"
	operatorclient "github.com/netapp/trident/operator/clients"
	"github.com/netapp/trident/operator/config"
	"github.com/netapp/trident/operator/controllers"
	"github.com/netapp/trident/operator/controllers/configurator"
	"github.com/netapp/trident/operator/controllers/orchestrator"
	"github.com/netapp/trident/operator/frontend/rest"
)

var (
	// Logging
	debug     = flag.Bool("debug", false, "Set log level to debug, takes precedence over log-level")
	logLevel  = flag.String("log-level", "info", "Logging level (trace, debug, info, warn, error, fatal)")
	logFormat = flag.String("log-format", "text", "Logging format (text, json)")

	// Kubernetes
	k8sAPIServer        = flag.String("k8s-api-server", "", "Kubernetes API server address")
	k8sConfigPath       = flag.String("k8s-config-path", "", "Path to KubeConfig file")
	skipK8sVersionCheck = flag.Bool("skip-k8s-version-check", false, "(Deprecated) Skip Kubernetes version check for Trident compatibility")

	configuratorReconcileInterval = flag.Duration("configurator-reconcile-interval",
		config.ConfiguratorReconcileInterval, "Set resource refresh rate for the auto generated backends")
)

const (
	operatorCheckAddress = ""
	operatorCheckPort    = "8002"
)

func printFlag(f *flag.Flag) {
	Log().WithFields(LogFields{"name": f.Name, "value": f.Value}).Debug("Flag")
}

func main() {
	var err error

	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()
	frontends := make([]controllers.Controller, 0)

	// Seed RNG one time
	rand.Seed(time.Now().UnixNano())

	if *debug {
		*logLevel = "debug"
	} else if *logLevel == "" {
		*logLevel = "info"
	}

	// Set log level
	err = InitLogLevel(*logLevel)
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, fmt.Sprintf("Error during InitLogLevel: %s, %v", *logLevel, err))
		os.Exit(1)
	}

	// Set log format
	err = InitLogFormat(*logFormat)
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	// Print all env variables
	for _, element := range os.Environ() {
		v := strings.Split(element, "=")
		Log().WithField(v[0], v[1]).Debug("Environment")
	}

	Log().WithFields(LogFields{
		"version":    config.OperatorVersion.String(),
		"build_time": config.BuildTime,
		"binary":     os.Args[0],
	}).Info("Running Trident operator.")

	// Print all command-line flags
	flag.Visit(printFlag)

	// Create the Kubernetes clients
	k8sClients, err := operatorclient.CreateK8SClients(*k8sAPIServer, *k8sConfigPath)
	if err != nil {
		Log().WithField("error", err).Fatalf("Could not create Kubernetes k8sclient.")
	}

	// Create Trident Orchestrator controller
	torcFrontend, err := orchestrator.NewController(k8sClients)
	if err != nil {
		Log().WithField("error", err).Fatalf("Could not create Trident Orchestrator controller.")
	}

	// Create Trident Configurator controller
	tconfFrontend, err := configurator.NewController(k8sClients, *configuratorReconcileInterval)
	if err != nil {
		Log().WithField("error", err).Fatalf("Could not create Trident Configurator controller.")
	}

	// Create the Operator frontend
	operatorFrontend := rest.NewHTTPServer(operatorCheckAddress, operatorCheckPort, k8sClients)

	frontends = append(frontends, torcFrontend, tconfFrontend, operatorFrontend)

	// Activate the frontends
	for _, c := range frontends {
		_ = c.Activate()
	}

	// Register and wait for a shutdown signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	Log().Info("Shutting down.")

	// Deactivate the frontends
	for _, c := range frontends {
		_ = c.Deactivate()
	}
}
