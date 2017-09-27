// Copyright 2016 NetApp, Inc. All Rights Reserved.

package main

import (
	"flag"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	log "github.com/Sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/kubernetes"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/persistent_store"
)

var (
	debug        = flag.Bool("debug", false, "Enable debugging output")
	k8sAPIServer = flag.String("k8s_api_server", "", "Kubernetes API server "+
		"address to enable dynamic storage provisioning for Kubernetes.")
	k8sConfigPath = flag.String("k8s_config_path", "", "Path to KubeConfig file.")
	k8sPod        = flag.Bool("k8s_pod", false, "Enables dynamic storage provisioning "+
		"for Kubernetes if running in a pod.")
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
	address     = flag.String("address", "localhost", "Storage orchestrator API address")
	port        = flag.String("port", "8000", "Storage orchestrator API port")
	useInMemory = flag.Bool("no_persistence", false, "Does not persist "+
		"any metadata.  WILL LOSE TRACK OF VOLUMES ON REBOOT/CRASH.")
	kubernetesVersion = "unknown"
	storeClient       persistent_store.Client
	enableKubernetes  bool
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

func processCmdLineArgs() {
	var err error
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
	// Don't bother validating the Kubernetes API server address; we'll know if
	// it's invalid during start-up.  Given that users can specify DNS names,
	// validation would be more trouble than it's worth.
	if *etcdV3 != "" {
		if shouldEnableTLS() {
			log.Debug("Trident is configured with an etcdv3 client with TLS.")
			storeClient, err = persistent_store.NewEtcdClientV3WithTLS(*etcdV3,
				*etcdV3Cert, *etcdV3CACert, *etcdV3Key)
		} else {
			log.Debug("Trident is configured with an etcdv3 client without TLS.")
			if !strings.Contains(*etcdV3, "127.0.0.1") {
				log.Warn("Trident's etcdv3 client should be configured with TLS!")
			}
			storeClient, err = persistent_store.NewEtcdClientV3(*etcdV3)
		}
		if err != nil {
			panic(err)
		}
	} else if *etcdV2 != "" {
		log.Debug("Trident is configured with an etcdv2 client.")
		storeClient, err = persistent_store.NewEtcdClientV2(*etcdV2)
		if err != nil {
			panic(err)
		}
	} else if (*etcdV3 != "" || *etcdV2 != "") && *useInMemory {
		log.Fatal("Cannot skip persistence and use etcd.")
	} else if *useInMemory {
		storeClient = persistent_store.NewInMemoryClient()
	} else {
		log.Fatal("Must specify a valid persistent store (currently " +
			"supporting etcdv2 and etcdv3) or no persistence.")
	}
	enableKubernetes = *k8sPod || *k8sAPIServer != ""
}

func main() {
	log.WithFields(log.Fields{
		"version":    config.OrchestratorVersion.String(),
		"build_time": config.BuildTime,
	}).Info("Running Trident storage orchestrator.")

	frontends := make([]frontend.FrontendPlugin, 0)
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()

	processCmdLineArgs()

	orchestrator := core.NewTridentOrchestrator(storeClient)

	if enableKubernetes {
		var (
			kubernetesFrontend frontend.FrontendPlugin
			err                error
		)
		if *k8sAPIServer != "" {
			kubernetesFrontend, err = kubernetes.NewPlugin(orchestrator, *k8sAPIServer, *k8sConfigPath)
		} else {
			kubernetesFrontend, err = kubernetes.NewPluginInCluster(orchestrator)
		}
		if err != nil {
			log.Fatal("Unable to start the Kubernetes frontend:  ", err)
		}
		orchestrator.AddFrontend(kubernetesFrontend)
		frontends = append(frontends, kubernetesFrontend)
	}
	restServer := rest.NewAPIServer(orchestrator, *address, *port)
	frontends = append(frontends, restServer)
	// Bootstrapping the orchestrator
	if err := orchestrator.Bootstrap(); err != nil {
		log.Fatal(err.Error())
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	for _, frontend := range frontends {
		frontend.Activate()
	}
	<-c
	log.Info("Shutting down.")
	for _, frontend := range frontends {
		frontend.Deactivate()
	}
	storeClient.Stop()
}
