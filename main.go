// Copyright 2018 NetApp, Inc. All Rights Reserved.

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/docker"
	"github.com/netapp/trident/frontend/kubernetes"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/logging"
	"github.com/netapp/trident/persistent_store"
)

var (
	// Logging
	debug    = flag.Bool("debug", false, "Enable debugging output")
	logLevel = flag.String("log_level", "info", "Logging level (debug, info, warn, error, fatal)")

	// Kubernetes
	k8sAPIServer = flag.String("k8s_api_server", "", "Kubernetes API server "+
		"address to enable dynamic storage provisioning for Kubernetes.")
	k8sConfigPath = flag.String("k8s_config_path", "", "Path to KubeConfig file.")
	k8sPod        = flag.Bool("k8s_pod", false, "Enables dynamic storage provisioning "+
		"for Kubernetes if running in a pod.")

	// Docker
	driverName = flag.String("volume_driver", "netapp", "Register as a Docker "+
		"volume plugin with this driver name")
	driverPort = flag.String("driver_port", "", "Listen on this port instead of using a "+
		"Unix domain socket")
	configPath = flag.String("config", "", "Path to configuration file(s)")

	// Persistence
	etcdV2 = flag.String("etcd_v2", "", "etcd server (v2 API) for "+
		"persisting orchestrator state (e.g., -etcd_v2=http://127.0.0.1:8001)")
	etcdV3 = flag.String("etcd_v3", "", "etcd server (v3 API) for "+
		"persisting orchestrator state (e.g., -etcd_v3=http://127.0.0.1:8001)")
	etcdV3Cert = flag.String("etcd_v3_cert", "/root/certs/etcd-client.crt",
		"etcdV3 client certificate")
	etcdV3CACert = flag.String("etcd_v3_cacert", "/root/certs/etcd-client-ca.crt",
		"etcdV3 client CA certificate")
	etcdV3Key = flag.String("etcd_v3_key", "/root/certs/etcd-client.key",
		"etcdV3 client private key")
	useInMemory = flag.Bool("no_persistence", false, "Does not persist "+
		"any metadata.  WILL LOSE TRACK OF VOLUMES ON REBOOT/CRASH.")
	usePassthrough = flag.Bool("passthrough", false, "Uses the storage backends "+
		"as the source of truth.  No data is stored anywhere else.")

	// REST interface
	address    = flag.String("address", "127.0.0.1", "Storage orchestrator API address")
	port       = flag.String("port", "8000", "Storage orchestrator API port")
	enableREST = flag.Bool("rest", true, "Enable REST interface")

	storeClient      persistentstore.Client
	enableKubernetes bool
	enableDocker     bool
)

func shouldEnableTLS() bool {
	// Check for client certificate, client CA certificate, and client private key
	if _, err := os.Stat(*etcdV3Cert); err != nil {
		return false
	}
	if _, err := os.Stat(*etcdV3CACert); err != nil {
		return false
	}
	if _, err := os.Stat(*etcdV3Key); err != nil {
		return false
	}
	return true
}

func printFlag(f *flag.Flag) {
	log.WithFields(log.Fields{
		"name":  f.Name,
		"value": f.Value,
	}).Debug("Flag")
}

func processCmdLineArgs() {
	var err error

	flag.Visit(printFlag)

	// Infer frontend from arguments
	enableKubernetes = *k8sPod || *k8sAPIServer != ""
	enableDocker = *configPath != ""

	if enableKubernetes && enableDocker {
		log.Fatal("Trident cannot serve both Docker and Kubernetes at the same time.")
	} else if !enableKubernetes && !enableDocker && !*useInMemory {
		log.Fatal("Insufficient arguments provided for Trident to start.  Specify either " +
			"k8sAPIServer (for Kubernetes) or configPath (for Docker).")
	}

	// Determine persistent store type from arguments
	storeCount := 0
	if *etcdV2 != "" {
		storeCount++
	}
	if *etcdV3 != "" {
		storeCount++
	}
	if *useInMemory {
		storeCount++
	}
	if *usePassthrough {
		storeCount++
	}
	// Infer persistent store type if not explicitly specified
	if storeCount == 0 && enableDocker {
		log.Debug("Inferred passthrough persistent store.")
		*usePassthrough = true
		storeCount++
	}
	if storeCount != 1 {
		log.Fatal("Trident must be configured with exactly one persistence type.")
	}

	// Don't bother validating the Kubernetes API server address; we'll know if
	// it's invalid during start-up.  Given that users can specify DNS names,
	// validation would be more trouble than it's worth.
	if *etcdV3 != "" {
		if shouldEnableTLS() {
			log.Debug("Trident is configured with an etcdv3 client with TLS.")
			storeClient, err = persistentstore.NewEtcdClientV3WithTLS(*etcdV3,
				*etcdV3Cert, *etcdV3CACert, *etcdV3Key)
		} else {
			log.Debug("Trident is configured with an etcdv3 client without TLS.")
			if !strings.Contains(*etcdV3, "127.0.0.1") {
				log.Warn("Trident's etcdv3 client should be configured with TLS!")
			}
			storeClient, err = persistentstore.NewEtcdClientV3(*etcdV3)
		}
		if err != nil {
			log.Fatalf("Unable to create the etcd V3 client. %v", err)
		}
	} else if *etcdV2 != "" {
		log.Debug("Trident is configured with an etcdv2 client.")
		storeClient, err = persistentstore.NewEtcdClientV2(*etcdV2)
		if err != nil {
			log.Fatalf("Unable to create the etcd V2 client. %v", err)
		}
	} else if *useInMemory {
		log.Debug("Trident is configured with an in-memory store client.")
		storeClient = persistentstore.NewInMemoryClient()
	} else if *usePassthrough {
		log.Debug("Trident is configured with passthrough store client.")
		storeClient, err = persistentstore.NewPassthroughClient(*configPath)
		if err != nil {
			log.Fatalf("Unable to create the passthrough store client. %v", err)
		}
	}

	config.UsingPassthroughStore = storeClient.GetType() == persistentstore.PassthroughStore
}

func main() {

	var err error

	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()
	frontends := make([]frontend.Plugin, 0)

	// Set log level
	err = logging.InitLogLevel(*debug, *logLevel)
	if err != nil {
		log.Fatal(err)
	}

	log.WithFields(log.Fields{
		"version":    config.OrchestratorVersion.String(),
		"build_time": config.BuildTime,
		"binary":     os.Args[0],
	}).Info("Running Trident storage orchestrator.")

	processCmdLineArgs()

	orchestrator := core.NewTridentOrchestrator(storeClient)

	// Create Kubernetes *or* Docker frontend
	if enableKubernetes {

		var kubernetesFrontend frontend.Plugin
		config.CurrentDriverContext = config.ContextKubernetes

		if *k8sAPIServer != "" {
			kubernetesFrontend, err = kubernetes.NewPlugin(orchestrator, *k8sAPIServer, *k8sConfigPath)
		} else {
			kubernetesFrontend, err = kubernetes.NewPluginInCluster(orchestrator)
		}
		if err != nil {
			log.Fatalf("Unable to start the Kubernetes frontend. %v", err)
		}
		orchestrator.AddFrontend(kubernetesFrontend)
		frontends = append(frontends, kubernetesFrontend)

	} else if enableDocker {

		config.CurrentDriverContext = config.ContextDocker

		// Set up multi-output logging
		err = logging.InitLogging(*driverName)
		if err != nil {
			fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}

		dockerFrontend, err := docker.NewPlugin(*driverName, *driverPort, orchestrator)
		if err != nil {
			log.Fatalf("Unable to start the Docker frontend. %v", err)
		}
		orchestrator.AddFrontend(dockerFrontend)
		frontends = append(frontends, dockerFrontend)
	}

	// Create REST frontend
	if *enableREST {
		if *port == "" {
			log.Warning("REST interface will not be available (port not specified).")
		} else {
			restServer := rest.NewAPIServer(orchestrator, *address, *port)
			frontends = append(frontends, restServer)
			log.WithFields(log.Fields{"name": "REST"}).Info("Added frontend.")
		}
	}

	// Bootstrap the orchestrator and start its frontends
	for _, f := range frontends {
		f.Activate()
	}
	if err = orchestrator.Bootstrap(); err != nil {
		log.Error(err.Error())
	}

	// Register and wait for a shutdown signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Info("Shutting down.")
	for _, f := range frontends {
		f.Deactivate()
	}
	storeClient.Stop()
}
