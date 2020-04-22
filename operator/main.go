// Copyright 2020 NetApp, Inc. All Rights Reserved.

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

	operatorclient "github.com/netapp/trident/operator/clients"
	"github.com/netapp/trident/operator/controllers/provisioner"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/logging"
	"github.com/netapp/trident/operator/config"
	"github.com/netapp/trident/operator/controllers"
)

var (
	// Logging
	debug     = flag.Bool("debug", false, "Enable debugging output")
	logLevel  = flag.String("log-level", "info", "Logging level (debug, info, warn, error, fatal)")
	logFormat = flag.String("log-format", "text", "Logging format (text, json)")

	// Kubernetes
	k8sAPIServer  = flag.String("k8s-api-server", "", "Kubernetes API server address")
	k8sConfigPath = flag.String("k8s-config-path", "", "Path to KubeConfig file")
)

func printFlag(f *flag.Flag) {
	log.WithFields(log.Fields{"name": f.Name, "value": f.Value}).Debug("Flag")
}

func main() {

	var err error

	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()
	crdControllers := make([]controllers.Controller, 0)

	// Seed RNG one time
	rand.Seed(time.Now().UnixNano())

	// Set log level
	err = logging.InitLogLevel(*debug, *logLevel)
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	// Set log format
	err = logging.InitLogFormat(*logFormat)
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	// Print all env variables
	for _, element := range os.Environ() {
		v := strings.Split(element, "=")
		log.WithField(v[0], v[1]).Debug("Environment")
	}

	log.WithFields(log.Fields{
		"version":    config.OperatorVersion.String(),
		"build_time": config.BuildTime,
		"binary":     os.Args[0],
	}).Info("Running Trident operator.")

	// Print all command-line flags
	flag.Visit(printFlag)

	// Create the Kubernetes clients
	k8sClients, err := operatorclient.CreateK8SClients(*k8sAPIServer, *k8sConfigPath)
	if err != nil {
		log.WithField("error", err).Fatalf("Could not create Kubernetes k8sclient.")
	}

	// Create Trident Provisioner controller
	tridentProvisioner, err := provisioner.NewController(k8sClients)
	if err != nil {
		log.WithField("error", err).Fatalf("Could not create Trident Provisioner controller.")
	}
	crdControllers = append(crdControllers, tridentProvisioner)

	// Activate the controllers
	for _, c := range crdControllers {
		_ = c.Activate()
	}

	// Register and wait for a shutdown signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Info("Shutting down.")

	// Deactivate the controllers
	for _, c := range crdControllers {
		_ = c.Deactivate()
	}
}
