// Copyright 2020 NetApp, Inc. All Rights Reserved.

package clients

import (
	"fmt"
	"io/ioutil"
	"time"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	commonconfig "github.com/netapp/trident/config"
	log "github.com/sirupsen/logrus"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/netapp/trident/operator/controllers/orchestrator/client/clientset/versioned"
	versionedTprov "github.com/netapp/trident/operator/controllers/provisioner/client/clientset/versioned"
)

type Clients struct {
	KubeConfig     *rest.Config
	KubeClient     *kubernetes.Clientset
	K8SClient      k8sclient.KubernetesClient
	CRDClient      *versioned.Clientset
	CRDTprovClient *versionedTprov.Clientset
	K8SVersion     *k8sversion.Info
	Namespace      string
}

const k8sTimeout = 30 * time.Second

func CreateK8SClients(apiServerIP, kubeConfigPath string) (*Clients, error) {

	var clients *Clients
	var err error

	// Get the API config based on whether we are running in or out of cluster
	if kubeConfigPath != "" {
		log.Debug("Creating ex-cluster Kubernetes clients.")
		clients, err = createK8SClientsExCluster(apiServerIP, kubeConfigPath)
	} else {
		log.Debug("Creating in-cluster Kubernetes clients.")
		clients, err = createK8SClientsInCluster()
	}
	if err != nil {
		return nil, err
	}

	// Create the Kubernetes client
	clients.KubeClient, err = kubernetes.NewForConfig(clients.KubeConfig)
	if err != nil {
		return nil, err
	}

	// Create the CRD client
	clients.CRDClient, err = versioned.NewForConfig(clients.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("could not initialize CRD client; %v", err)
	}

	// Create the tprov CRD client
	clients.CRDTprovClient, err = versionedTprov.NewForConfig(clients.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Tprov CRD client; %v", err)
	}

	// Get the Kubernetes server version
	clients.K8SVersion, err = clients.KubeClient.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("could not get Kubernetes version: %v", err)
	}

	log.WithFields(log.Fields{
		"namespace": clients.Namespace,
		"version":   clients.K8SVersion.String(),
	}).Info("Created Kubernetes clients.")

	return clients, nil
}

func createK8SClientsExCluster(apiServerIP, kubeConfigPath string) (*Clients, error) {

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{
			ExplicitPath: kubeConfigPath,
		},
		&clientcmd.ConfigOverrides{
			ClusterInfo: clientcmdapi.Cluster{
				Server: apiServerIP,
			},
		},
	)

	kubeConfig, err := clientConfig.ClientConfig()

	if err != nil {
		return nil, err
	}

	namespace, _, err := clientConfig.Namespace()
	if err != nil {
		return nil, err
	}

	// Create the CLI-based Kubernetes client
	k8sClient, err := k8sclient.NewKubeClient(kubeConfig, namespace, k8sTimeout)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	return &Clients{
		KubeConfig: kubeConfig,
		K8SClient:  k8sClient,
		Namespace:  namespace,
	}, nil
}

func createK8SClientsInCluster() (*Clients, error) {

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	// when running in a pod, we use the Trident pod's namespace
	namespaceBytes, err := ioutil.ReadFile(commonconfig.NamespaceFile)
	if err != nil {
		return nil, fmt.Errorf("could not read namespace file %s; %v", commonconfig.NamespaceFile, err)
	}
	namespace := string(namespaceBytes)

	// Create the Kubernetes client
	k8sClient, err := k8sclient.NewKubeClient(kubeConfig, namespace, k8sTimeout)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	return &Clients{
		KubeConfig: kubeConfig,
		K8SClient:  k8sClient,
		Namespace:  namespace,
	}, nil
}
