// Copyright 2021 NetApp, Inc. All Rights Reserved.

package main

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/crd"
	"github.com/netapp/trident/frontend/csi"
	"github.com/netapp/trident/frontend/csi/helpers"
	k8shelper "github.com/netapp/trident/frontend/csi/helpers/kubernetes"
	plainhelper "github.com/netapp/trident/frontend/csi/helpers/plain"
	"github.com/netapp/trident/frontend/docker"
	"github.com/netapp/trident/frontend/metrics"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/logging"
	persistentstore "github.com/netapp/trident/persistent_store"
)

var (
	// Logging
	debug     = flag.Bool("debug", false, "Enable debugging output")
	logLevel  = flag.String("log_level", "info", "Logging level (debug, info, warn, error, fatal)")
	logFormat = flag.String("log_format", "text", "Logging format (text, json)")

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

	// CSI
	csiEndpoint = flag.String("csi_endpoint", "", "Register as a CSI storage "+
		"provider with this endpoint")
	csiNodeName = flag.String("csi_node_name", "", "CSI node name")
	csiRole     = flag.String("csi_role", "", fmt.Sprintf("CSI role to play: '%s' or '%s'", csi.CSIController, csi.CSINode))

	csiUnsafeNodeDetach = flag.Bool("csi_unsafe_detach", false, "Prefer to detach successfully rather than safely")

	nodePrep = flag.Bool("node_prep", true, "Attempt to install required packages on nodes.")

	// Persistence
	useInMemory = flag.Bool("no_persistence", false, "Does not persist "+
		"any metadata.  WILL LOSE TRACK OF VOLUMES ON REBOOT/CRASH.")
	usePassthrough = flag.Bool("passthrough", false, "Uses the storage backends "+
		"as the source of truth.  No data is stored anywhere else.")
	useCRD = flag.Bool("crd_persistence", false, "Uses CRDs for persisting orchestrator state.")

	// HTTP REST interface
	address            = flag.String("address", "127.0.0.1", "Storage orchestrator HTTP API address")
	port               = flag.String("port", "8000", "Storage orchestrator HTTP API port")
	enableREST         = flag.Bool("rest", true, "Enable HTTP REST interface")
	httpRequestTimeout = flag.String("http_request_timeout", config.HTTPTimeoutString, "Set the request timeout for HTTP")

	// HTTPS REST interface
	httpsAddress    = flag.String("https_address", "", "Storage orchestrator HTTPS API address")
	httpsPort       = flag.String("https_port", "8443", "Storage orchestrator HTTPS API port")
	enableHTTPSREST = flag.Bool("https_rest", false, "Enable HTTPS REST interface")
	httpsCACert     = flag.String("https_ca_cert", config.CACertPath, "HTTPS CA certificate")
	httpsServerKey  = flag.String("https_server_key", config.ServerKeyPath, "HTTPS server private key")
	httpsServerCert = flag.String("https_server_cert", config.ServerCertPath, "HTTPS server certificate")
	httpsClientKey  = flag.String("https_client_key", config.ClientKeyPath, "HTTPS client private key")
	httpsClientCert = flag.String("https_client_cert", config.ClientCertPath, "HTTPS client certificate")

	aesKey = flag.String("aes_key", config.AESKeyPath, "AES encryption key")

	// HTTP metrics interface
	metricsAddress = flag.String("metrics_address", "", "Storage orchestrator metrics address")
	metricsPort    = flag.String("metrics_port", "8001", "Storage orchestrator metrics port")
	enableMetrics  = flag.Bool("metrics", false, "Enable metrics interface")

	storeClient  persistentstore.Client
	enableDocker bool
	enableCSI    bool
)

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
	enableCSI = *csiEndpoint != ""
	enableDocker = *configPath != "" && !enableCSI

	frontendCount := 0
	if enableDocker {
		frontendCount++
	}
	if enableCSI {
		frontendCount++
	}

	if frontendCount > 1 {
		log.Fatal("Trident can only run one frontend type (Kubernetes, Docker, CSI).")
	} else if !enableDocker && !enableCSI && !*useInMemory {
		log.Fatal("Insufficient arguments provided for Trident to start.  Specify " +
			"--config (for Docker) or --csi_endpoint (for CSI).")
	}

	// Determine persistent store type from arguments
	storeCount := 0
	if *useInMemory {
		storeCount++
	}
	if *usePassthrough {
		storeCount++
	}
	if *useCRD {
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

	switch {
	case *useInMemory:
		log.Debug("Trident is configured with an in-memory store client.")
		storeClient = persistentstore.NewInMemoryClient()

	case *usePassthrough:
		log.Debug("Trident is configured with passthrough store client.")
		storeClient, err = persistentstore.NewPassthroughClient(*configPath)
		if err != nil {
			log.Fatalf("Unable to create the passthrough store client. %v", err)
		}

	case *useCRD:
		log.Debug("Trident is configured with a CRD client.")
		storeClient, err = persistentstore.NewCRDClientV1(*k8sAPIServer, *k8sConfigPath)
		if err != nil {
			log.Fatalf("Unable to create the Kubernetes store client. %v", err)
		}
	}

	config.UsingPassthroughStore = storeClient.GetType() == persistentstore.PassthroughStore
}

// getenvAsPointerToBool returns the key's value and defaults to false if not set
func getenvAsPointerToBool(key string) *bool {
	result := false
	if v := os.Getenv(key); v != "" {
		if strings.EqualFold("true", v) {
			result = true
		}
	}
	return &result
}

// ensureDockerPluginExecPath ensures the docker plugin has utility path in PATH
func ensureDockerPluginExecPath() {
	path := os.Getenv("PATH")
	if !strings.Contains(path, "/netapp") {
		path = "/netapp:" + path
		_ = os.Setenv("PATH", path)
	}
}

// processDockerPluginArgs replaces our container-launch.sh script
// see also:  https://github.com/NetApp/trident/blob/stable/v20.07/contrib/docker/plugin/container-launch.sh
func processDockerPluginArgs() error {
	if os.Getenv(config.DockerPluginModeEnvVariable) == "" {
		// not running in docker plugin mode, can stop further processing here
		return nil
	}

	ensureDockerPluginExecPath()

	debug = getenvAsPointerToBool("debug")
	enableREST = getenvAsPointerToBool("rest")
	if configEnv := os.Getenv("config"); configEnv != "" {
		configFile := configEnv
		if !strings.HasPrefix(configFile, config.DockerPluginConfigLocation) {
			configFile = filepath.Join(config.DockerPluginConfigLocation, configEnv)
		}
		if _, err := os.Stat(configFile); err != nil {
			if os.IsNotExist(err) {
				return errors.New("config file '" + configFile + "' does not exist")
			} else {
				return err
			}
		}
		configPath = &configFile
	}
	return nil
}

func main() {

	var err error

	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()
	preBootstrapFrontends := make([]frontend.Plugin, 0)
	postBootstrapFrontends := make([]frontend.Plugin, 0)

	// Seed RNG one time
	rand.Seed(time.Now().UnixNano())

	// Process any docker plugin environment variables (after we flag.Parse() the CLI above)
	err = processDockerPluginArgs()
	if err != nil {
		log.Fatal(err)
	}

	// Set log level
	err = logging.InitLogLevel(*debug, *logLevel)
	if err != nil {
		log.Fatal(err)
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
		"version":    config.OrchestratorVersion.String(),
		"build_time": config.BuildTime,
		"binary":     os.Args[0],
	}).Info("Running Trident storage orchestrator.")

	processCmdLineArgs()

	orchestrator := core.NewTridentOrchestrator(storeClient)

	// Create HTTP metrics frontend
	if *enableMetrics {
		if *metricsPort == "" {
			log.Warning("HTTP metrics interface will not be available (port not specified).")
		} else {
			metricsServer := metrics.NewMetricsServer(*metricsAddress, *metricsPort)
			preBootstrapFrontends = append(preBootstrapFrontends, metricsServer)
			log.WithFields(log.Fields{"name": metricsServer.GetName()}).Info("Added frontend.")
		}
	}

	enableMutualTLS := true
	handler := rest.NewRouter()

	// Create Docker *or* CSI/K8S frontend
	if enableDocker {

		config.CurrentDriverContext = config.ContextDocker

		// Set up multi-output logging
		err = logging.InitLoggingForDocker(*driverName, *logFormat)
		if err != nil {
			_, _ = fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}

		dockerFrontend, err := docker.NewPlugin(*driverName, *driverPort, orchestrator)
		if err != nil {
			log.Fatalf("Unable to start the Docker frontend. %v", err)
		}
		orchestrator.AddFrontend(dockerFrontend)
		preBootstrapFrontends = append(preBootstrapFrontends, dockerFrontend)

	} else if enableCSI {

		config.CurrentDriverContext = config.ContextCSI

		if *csiRole != csi.CSIController && *csiRole != csi.CSINode && *csiRole != csi.CSIAllInOne {
			log.Fatalf("CSI is enabled but an unknown role has been assigned: '%s'", *csiRole)
		}

		if *csiEndpoint == "" {
			log.Fatal("CSI is enabled but csi_endpoint was not specified.")
		}

		if *csiNodeName == "" {
			log.Fatal("CSI is enabled but csi_node_name was not specified.")
		}

		var hybridFrontend frontend.Plugin
		if *k8sAPIServer != "" || *k8sPod {
			hybridFrontend, err = k8shelper.NewPlugin(orchestrator, *k8sAPIServer, *k8sConfigPath)
		} else {
			hybridFrontend = plainhelper.NewPlugin(orchestrator)
		}
		if err != nil {
			log.Fatalf("Unable to start the K8S hybrid frontend. %v", err)
		}
		orchestrator.AddFrontend(hybridFrontend)
		postBootstrapFrontends = append(postBootstrapFrontends, hybridFrontend)
		hybridPlugin := hybridFrontend.(helpers.HybridPlugin)

		log.WithFields(log.Fields{
			"name":    *csiNodeName,
			"version": config.OrchestratorVersion,
		}).Info("Initializing CSI frontend.")

		var csiFrontend *csi.Plugin
		switch *csiRole {
		case csi.CSIController:
			csiFrontend, err = csi.NewControllerPlugin(*csiNodeName, *csiEndpoint, *aesKey, orchestrator, &hybridPlugin)
		case csi.CSINode:
			csiFrontend, err = csi.NewNodePlugin(*csiNodeName, *csiEndpoint, *httpsCACert, *httpsClientCert,
				*httpsClientKey, *aesKey, orchestrator, *csiUnsafeNodeDetach, *nodePrep)
			enableMutualTLS = false
			handler = rest.NewNodeRouter(csiFrontend)
		case csi.CSIAllInOne:
			csiFrontend, err = csi.NewAllInOnePlugin(*csiNodeName, *csiEndpoint, *httpsCACert, *httpsClientCert,
				*httpsClientKey, *aesKey, orchestrator, &hybridPlugin, *csiUnsafeNodeDetach, *nodePrep)
		}
		if err != nil {
			log.Fatalf("Unable to start the CSI frontend. %v", err)
		}
		orchestrator.AddFrontend(csiFrontend)
		postBootstrapFrontends = append(postBootstrapFrontends, csiFrontend)

		if *useCRD {
			crdController, err := crd.NewTridentCrdController(orchestrator, *k8sAPIServer, *k8sConfigPath)
			if err != nil {
				log.Fatalf("Unable to start the Trident CRD controller frontend. %v", err)
			}
			orchestrator.AddFrontend(crdController)
			postBootstrapFrontends = append(postBootstrapFrontends, crdController)
		}
	}

	// Create HTTP REST frontend
	if *enableREST {
		httpTimeout, err := time.ParseDuration(*httpRequestTimeout)
		if err != nil {
			log.Fatalf("HTTP request timeout could not be converted to a duration, cannot continue")
		}

		if httpTimeout < 0 {
			log.Fatalf("HTTP request timeout cannot be a negative duration, cannot continue")
		}

		if *port == "" {
			log.Warning("HTTP REST interface will not be available (port not specified).")
		} else {
			httpServer := rest.NewHTTPServer(orchestrator, *address, *port, httpTimeout)
			preBootstrapFrontends = append(preBootstrapFrontends, httpServer)
			log.WithFields(log.Fields{"name": httpServer.GetName()}).Info("Added frontend.")
		}
	}

	// Create HTTPS REST frontend
	if *enableHTTPSREST {
		if *httpsPort == "" {
			log.Warning("HTTPS REST interface will not be available (httpsPort not specified).")
		} else {
			httpTimeout, err := time.ParseDuration(*httpRequestTimeout)
			if err != nil {
				log.Fatalf("HTTP request timeout could not be converted to a duration, cannot continue")
			}

			if httpTimeout < 0 {
				log.Fatalf("HTTP request timeout cannot be a negative duration, cannot continue")
			}

			httpsServer, err := rest.NewHTTPSServer(
				orchestrator, *httpsAddress, *httpsPort, *httpsCACert, *httpsServerCert, *httpsServerKey,
				enableMutualTLS, handler, httpTimeout)
			if err != nil {
				log.Fatalf("Unable to start the HTTPS REST frontend. %v", err)
			}
			preBootstrapFrontends = append(preBootstrapFrontends, httpsServer)
			log.WithFields(log.Fields{"name": httpsServer.GetName()}).Info("Added frontend.")
		}
	}

	// Bootstrap the orchestrator and start its frontends.  Some frontends, notably REST and Docker, must
	// start before the core so that the external interfaces are minimally responding while the core is
	// still initializing.  Other frontends such as legacy Kubernetes and CSI benefit from starting after
	// the core is ready.
	for _, f := range preBootstrapFrontends {
		if err := f.Activate(); err != nil {
			log.Error(err)
		}
	}
	if err = orchestrator.Bootstrap(); err != nil {
		log.Error(err.Error())
	}
	for _, f := range postBootstrapFrontends {
		if err := f.Activate(); err != nil {
			log.Error(err)
		}
	}

	if config.CurrentDriverContext == config.ContextCSI {
		go orchestrator.PeriodicallyReconcileNodeAccessOnBackends()
	}

	// Register and wait for a shutdown signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Info("Shutting down.")
	for _, f := range postBootstrapFrontends {
		if err := f.Deactivate(); err != nil {
			log.Error(err)
		}
	}
	orchestrator.Stop()
	for _, f := range preBootstrapFrontends {
		if err := f.Deactivate(); err != nil {
			log.Error(err)
		}
	}
	if err = storeClient.Stop(); err != nil {
		log.Error(err)
	}
}
