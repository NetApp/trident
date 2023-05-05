// Copyright 2022 NetApp, Inc. All Rights Reserved.

package main

import (
	"context"
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

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/crd"
	"github.com/netapp/trident/frontend/csi"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	k8sctrlhelper "github.com/netapp/trident/frontend/csi/controller_helpers/kubernetes"
	plainctrlhelper "github.com/netapp/trident/frontend/csi/controller_helpers/plain"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	k8snodehelper "github.com/netapp/trident/frontend/csi/node_helpers/kubernetes"
	plainnodehelper "github.com/netapp/trident/frontend/csi/node_helpers/plain"
	"github.com/netapp/trident/frontend/docker"
	"github.com/netapp/trident/frontend/metrics"
	"github.com/netapp/trident/frontend/rest"
	. "github.com/netapp/trident/logging"
	persistentstore "github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

var (
	// Logging
	auditLog  = flag.Bool("disable_audit_log", true, "Disable the audit logger")
	debug     = flag.Bool("debug", false, "Enable debugging output")
	logFormat = flag.String("log_format", "text", "Logging format (text, json)")
	logLevel  = flag.String("log_level", "info", "Logging level (trace, debug, info, warn, "+
		"error, fatal)")
	logWorkflows = flag.String("log_workflows", "", "A comma-delimited list of Trident workflows "+
		"to enable trace logging for")
	logLayers = flag.String("log_layers", "", "A comma-delimited list of Trident layers to enable "+
		"trace logging for")

	// Kubernetes
	k8sAPIServer = flag.String("k8s_api_server", "", "Kubernetes API server "+
		"address to enable dynamic storage provisioning for Kubernetes.")
	k8sConfigPath = flag.String("k8s_config_path", "", "Path to KubeConfig file.")
	k8sPod        = flag.Bool("k8s_pod", false, "Enables dynamic storage provisioning "+
		"for Kubernetes if running in a pod.")

	// Docker
	dockerPluginMode = flag.Bool("docker_plugin_mode", false, "Enable docker plugin mode")
	driverName       = flag.String("volume_driver", "netapp", "Register as a Docker "+
		"volume plugin with this driver name")
	driverPort = flag.String("driver_port", "", "Listen on this port instead of using a "+
		"Unix domain socket")
	configPath = flag.String("config", "", "Path to configuration file(s)")

	// CSI
	csiEndpoint = flag.String("csi_endpoint", "", "Register as a CSI storage "+
		"provider with this endpoint")
	csiNodeName = flag.String("csi_node_name", "", "CSI node name")
	csiRole     = flag.String("csi_role", "",
		fmt.Sprintf("CSI role to play: '%s' or '%s'", csi.CSIController, csi.CSINode))

	csiUnsafeNodeDetach = flag.Bool("csi_unsafe_detach", false, "Prefer to detach successfully rather than safely")
	enableForceDetach   = new(bool)
	nodePrep            = flag.Bool("node_prep", true, "Attempt to install required packages on nodes.")

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
	httpRequestTimeout = flag.Duration("http_request_timeout", config.HTTPTimeout,
		"Override the HTTP request timeout for Trident controller’s REST API")

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

	// iSCSI
	iSCSISelfHealingInterval = flag.Duration("iscsi_self_healing_interval", config.IscsiSelfHealingInterval,
		"Interval at which the iSCSI self-healing thread is invoked")
	iSCSISelfHealingWaitTime = flag.Duration("iscsi_self_healing_wait_time",
		config.ISCSISelfHealingWaitTime,
		"Wait time after which iSCSI self-healing attempts to fix stale sessions")

	// core
	backendStoragePollInterval = flag.Duration("backend_storage_poll_interval", config.BackendStoragePollInterval,
		"Interval at which core polls backend storage for its state")

	storeClient  persistentstore.Client
	enableDocker bool
	enableCSI    bool
)

func printFlag(f *flag.Flag) {
	Logc(context.Background()).WithFields(LogFields{
		"name":  f.Name,
		"value": f.Value,
	}).Debug("Flag")
}

func processCmdLineArgs(ctx context.Context) {
	Logc(ctx).Trace(">>>> processCmdLineArgs")
	defer Logc(ctx).Trace("<<<< processCmdLineArgs")

	var err error

	flag.Visit(printFlag)

	// Infer frontend from arguments
	enableCSI = *csiEndpoint != ""
	enableDocker = (*dockerPluginMode || *configPath != "") && !enableCSI

	frontendCount := 0
	if enableDocker {
		frontendCount++
	}
	if enableCSI {
		frontendCount++
	}

	if frontendCount > 1 {
		Logc(ctx).Fatal("Trident can only run one frontend type (Kubernetes, Docker, CSI).")
	} else if !enableDocker && !enableCSI && !*useInMemory {
		Logc(ctx).Fatal("Insufficient arguments provided for Trident to start.  Specify --config (for Docker) " +
			"or --csi_endpoint (for CSI).")
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
		Logc(ctx).Debug("Inferred passthrough persistent store.")
		*usePassthrough = true
		storeCount++
	}
	if storeCount != 1 {
		Logc(ctx).Fatal("Trident must be configured with exactly one persistence type.")
	}

	switch {
	case *useInMemory:
		Logc(ctx).Debug("Trident is configured with an in-memory store client.")
		storeClient = persistentstore.NewInMemoryClient()

	case *usePassthrough:
		Logc(ctx).Debug("Trident is configured with passthrough store client.")
		storeClient, err = persistentstore.NewPassthroughClient(*configPath)
		if err != nil {
			Logc(ctx).Fatalf("Unable to create the passthrough store client. %v", err)
		}

	case *useCRD:
		Logc(ctx).Debug("Trident is configured with a CRD client.")
		storeClient, err = persistentstore.NewCRDClientV1(*k8sAPIServer, *k8sConfigPath)
		if err != nil {
			Logc(ctx).Fatalf("Unable to create the Kubernetes store client. %v", err)
		}
	}

	config.UsingPassthroughStore = storeClient.GetType() == persistentstore.PassthroughStore
}

// getenvAsPointerToBool returns the key's value and defaults to false if not set
func getenvAsPointerToBool(ctx context.Context, key string) *bool {
	Logc(ctx).Trace(">>>> getenvAsPointerToBool")
	defer Logc(ctx).Trace("<<<< getenvAsPointerToBool")

	result := false
	if v := os.Getenv(key); v != "" {
		if strings.EqualFold("true", v) {
			result = true
		}
	}
	return &result
}

// ensureDockerPluginExecPath ensures the docker plugin has utility path in PATH
func ensureDockerPluginExecPath(ctx context.Context) {
	Logc(ctx).Trace(">>>> ensureDockerPluginExecPath")
	defer Logc(ctx).Trace("<<<< ensureDockerPluginExecPath")

	path := os.Getenv("PATH")
	if !strings.Contains(path, "/netapp") {
		Logc(ctx).Trace("PATH did not contain /netapp, putting it there.")
		path = "/netapp:" + path
		_ = os.Setenv("PATH", path)
	}
}

// processDockerPluginArgs replaces our container-launch.sh script
// see also:  https://github.com/NetApp/trident/blob/stable/v20.07/contrib/docker/plugin/container-launch.sh
func processDockerPluginArgs(ctx context.Context) error {
	Logc(ctx).Trace(">>>> processDockerPluginArgs")
	defer Logc(ctx).Trace("<<<< processDockerPluginArgs")

	if os.Getenv(config.DockerPluginModeEnvVariable) == "" {
		// not running in docker plugin mode, can stop further processing here
		return nil
	}

	ensureDockerPluginExecPath(ctx)

	enableREST = getenvAsPointerToBool(ctx, "rest")
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
	var txnMonitor bool

	runtime.GOMAXPROCS(runtime.NumCPU())

	ctx := GenerateRequestContext(nil, "", "", WorkflowCoreInit, LogLayerCore)

	// These features are only supported on Linux.
	if runtime.GOOS == "linux" {
		Logc(ctx).Trace("On a Linux OS, creating force detach feature flag.")
		enableForceDetach = flag.Bool("enable_force_detach", false, "Enable force detach feature.")
	}

	flag.Parse()
	preBootstrapFrontends := make([]frontend.Plugin, 0)
	postBootstrapFrontends := make([]frontend.Plugin, 0)

	// Seed RNG one time
	rand.Seed(time.Now().UnixNano())

	// Process any docker plugin environment variables (after we flag.Parse() the CLI above)
	if *dockerPluginMode {
		os.Setenv("DOCKER_PLUGIN_MODE", "1")
		utils.SetChrootPathPrefix("/host")
	}

	err = processDockerPluginArgs(ctx)
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	// Set log level
	if *debug {
		*logLevel = "debug"
	}
	err = InitLogLevel(*logLevel)
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	// Set log format
	err = InitLogFormat(*logFormat)
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	// Set logging workflows.
	err = SetWorkflows(*logWorkflows)
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	// Set logging layers.
	err = SetLogLayers(*logLayers)
	if err != nil {
		_, _ = fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	// Initialize the audit logger.
	InitAuditLogger(*auditLog)

	// Print all env variables
	for _, element := range os.Environ() {
		v := strings.Split(element, "=")
		Log().WithField(v[0], v[1]).Debug("Environment")
	}

	Log().WithFields(LogFields{
		"version":    config.OrchestratorVersion.String(),
		"build_time": config.BuildTime,
		"binary":     os.Args[0],
	}).Info("Running Trident storage orchestrator.")

	processCmdLineArgs(ctx)

	orchestrator := core.NewTridentOrchestrator(storeClient)

	// Create HTTP metrics frontend
	if *enableMetrics {
		if *metricsPort == "" {
			Log().Warning("HTTP metrics interface will not be available (port not specified).")
		} else {
			metricsServer := metrics.NewMetricsServer(*metricsAddress, *metricsPort)
			preBootstrapFrontends = append(preBootstrapFrontends, metricsServer)
			Log().WithFields(LogFields{"name": metricsServer.GetName()}).Info("Added frontend.")
		}
	}

	enableMutualTLS := true
	handler := rest.NewRouter(true)

	// Create Docker *or* CSI/K8S frontend
	if enableDocker {

		config.CurrentDriverContext = config.ContextDocker

		// Set up multi-output logging
		err = InitLoggingForDocker(*driverName, *logFormat)
		if err != nil {
			_, _ = fmt.Fprint(os.Stderr, err)
			os.Exit(1)
		}

		dockerFrontend, err := docker.NewPlugin(*driverName, *driverPort, orchestrator)
		if err != nil {
			Log().Fatalf("Unable to start the Docker frontend. %v", err)
		}
		orchestrator.AddFrontend(ctx, dockerFrontend)
		preBootstrapFrontends = append(preBootstrapFrontends, dockerFrontend)

	} else if enableCSI {

		config.CurrentDriverContext = config.ContextCSI

		if *csiRole != csi.CSIController && *csiRole != csi.CSINode && *csiRole != csi.CSIAllInOne {
			Log().Fatalf("CSI is enabled but an unknown role has been assigned: '%s'", *csiRole)
		}

		if *csiEndpoint == "" {
			Log().Fatal("CSI is enabled but csi_endpoint was not specified.")
		}

		if *csiNodeName == "" {
			Log().Fatal("CSI is enabled but csi_node_name was not specified.")
		}

		var hybridControllerFrontend frontend.Plugin
		var hybridNodeFrontend frontend.Plugin
		if *k8sAPIServer != "" || *k8sPod {
			hybridControllerFrontend, err = k8sctrlhelper.NewHelper(orchestrator, *k8sAPIServer, *k8sConfigPath,
				*enableForceDetach)
			if err != nil {
				Log().WithError(err).Fatal("Unable to start the K8S hybrid controller frontend.")
			}
			hybridNodeFrontend, err = k8snodehelper.NewHelper(orchestrator, *k8sConfigPath, *enableForceDetach)
			if err != nil {
				Log().WithError(err).Fatal("Unable to start the K8S hybrid node frontend.")
			}
		} else {
			hybridControllerFrontend = plainctrlhelper.NewHelper(orchestrator)
			hybridNodeFrontend = plainnodehelper.NewHelper(orchestrator)
		}

		if *csiRole == csi.CSIController || *csiRole == csi.CSIAllInOne {
			orchestrator.AddFrontend(ctx, hybridControllerFrontend)
			postBootstrapFrontends = append(postBootstrapFrontends, hybridControllerFrontend)
		}
		controllerHelper, ok := hybridControllerFrontend.(controllerhelpers.ControllerHelper)
		if !ok {
			Log().Fatalf("%v",
				errors.TypeAssertionError("hybridControllerFrontend.(controllerhelpers.ControllerHelper)"))
		}

		if *csiRole == csi.CSINode || *csiRole == csi.CSIAllInOne {
			orchestrator.AddFrontend(ctx, hybridNodeFrontend)
			postBootstrapFrontends = append(postBootstrapFrontends, hybridNodeFrontend)
		}
		nodeHelper, ok := hybridNodeFrontend.(nodehelpers.NodeHelper)
		if !ok {
			Log().Fatalf("%v", errors.TypeAssertionError("hybridNodeFrontend.(nodehelpers.NodeHelper)"))
		}

		Log().WithFields(LogFields{
			"name":    *csiNodeName,
			"version": config.OrchestratorVersion,
		}).Info("Initializing CSI frontend.")

		var csiFrontend *csi.Plugin
		switch *csiRole {
		case csi.CSIController:
			txnMonitor = true
			csiFrontend, err = csi.NewControllerPlugin(*csiNodeName, *csiEndpoint, *aesKey, orchestrator,
				&controllerHelper, *enableForceDetach)
		case csi.CSINode:
			csiFrontend, err = csi.NewNodePlugin(*csiNodeName, *csiEndpoint, *httpsCACert, *httpsClientCert,
				*httpsClientKey, *aesKey, orchestrator, *csiUnsafeNodeDetach, &nodeHelper, *enableForceDetach,
				*iSCSISelfHealingInterval, *iSCSISelfHealingWaitTime)
			enableMutualTLS = false
			handler = rest.NewNodeRouter(csiFrontend)
		case csi.CSIAllInOne:
			txnMonitor = true
			csiFrontend, err = csi.NewAllInOnePlugin(*csiNodeName, *csiEndpoint, *httpsCACert, *httpsClientCert,
				*httpsClientKey, *aesKey, orchestrator, &controllerHelper, &nodeHelper, *csiUnsafeNodeDetach,
				*iSCSISelfHealingInterval, *iSCSISelfHealingWaitTime)
		}
		if err != nil {
			Log().Fatalf("Unable to start the CSI frontend. %v", err)
		}
		orchestrator.AddFrontend(ctx, csiFrontend)
		postBootstrapFrontends = append(postBootstrapFrontends, csiFrontend)

		if *useCRD {
			crdController, err := crd.NewTridentCrdController(orchestrator, *k8sAPIServer, *k8sConfigPath)
			if err != nil {
				Log().Fatalf("Unable to start the Trident CRD controller frontend. %v", err)
			}
			orchestrator.AddFrontend(ctx, crdController)
			postBootstrapFrontends = append(postBootstrapFrontends, crdController)
		}
	}

	// Create HTTP REST frontend
	if *enableREST {

		if *httpRequestTimeout < 0 {
			Log().Fatalf("HTTP request timeout cannot be a negative duration, cannot continue")
		}

		if *port == "" {
			Log().Warning("HTTP REST interface will not be available (port not specified).")
		} else {

			if *address != "127.0.0.1" && *address != "[::1]" {
				*address = "127.0.0.1"
			}
			httpServer := rest.NewHTTPServer(orchestrator, *address, *port, *httpRequestTimeout)
			preBootstrapFrontends = append(preBootstrapFrontends, httpServer)
			Log().WithFields(LogFields{"name": httpServer.GetName()}).Info("Added frontend.")
		}
	}

	// Create HTTPS REST frontend
	if *enableHTTPSREST {
		if *httpsPort == "" {
			Log().Warning("HTTPS REST interface will not be available (httpsPort not specified).")
		} else {
			if *httpRequestTimeout < 0 {
				Log().Fatalf("HTTP request timeout cannot be a negative duration, cannot continue")
			}

			httpsServer, err := rest.NewHTTPSServer(
				orchestrator, *httpsAddress, *httpsPort, *httpsCACert, *httpsServerCert, *httpsServerKey,
				enableMutualTLS, handler, *httpRequestTimeout)
			if err != nil {
				Log().Fatalf("Unable to start the HTTPS REST frontend. %v", err)
			}
			preBootstrapFrontends = append(preBootstrapFrontends, httpsServer)
			Log().WithFields(LogFields{"name": httpsServer.GetName()}).Info("Added frontend.")
		}
	}

	// Bootstrap the orchestrator and start its frontends.  Some frontends, notably REST and Docker, must
	// start before the core so that the external interfaces are minimally responding while the core is
	// still initializing.  Other frontends such as legacy Kubernetes and CSI benefit from starting after
	// the core is ready.
	for _, f := range preBootstrapFrontends {
		if err := f.Activate(); err != nil {
			Log().Error(err)
		}
	}

	if err = orchestrator.Bootstrap(txnMonitor); err != nil {
		Log().Error(err.Error())
	}
	for _, f := range postBootstrapFrontends {
		if err := f.Activate(); err != nil {
			// Terminal errors only come from application states that are unrecoverable or may lead to
			// significant but unintentional changes in behavior.
			if csi.IsTerminalReconciliationError(err) {
				Log().WithError(err).Fatal("Activation failed for one or more helper frontends")
			}
		}
	}

	if config.CurrentDriverContext == config.ContextCSI {
		go orchestrator.PeriodicallyReconcileNodeAccessOnBackends()
	}
	go orchestrator.PeriodicallyReconcileBackendState(*backendStoragePollInterval)

	// Register and wait for a shutdown signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	Log().Info("Shutting down.")
	for _, f := range preBootstrapFrontends {
		if err := f.Deactivate(); err != nil {
			Log().Error(err)
		}
	}
	for _, f := range postBootstrapFrontends {
		if err := f.Deactivate(); err != nil {
			Log().Error(err)
		}
	}
	orchestrator.Stop()
	if err = storeClient.Stop(); err != nil {
		Log().Error(err)
	}
}
