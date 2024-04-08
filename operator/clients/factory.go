// Copyright 2022 NetApp, Inc. All Rights Reserved.

package clients

import (
	"fmt"
	"os"
	"time"

	snapshotClient "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	commonconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/operator/crd/client/clientset/versioned"
	tridentClient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
)

type Clients struct {
	KubeConfig       *rest.Config
	KubeClient       *kubernetes.Clientset
	K8SClient        k8sclient.KubernetesClient
	CRDClient        *versioned.Clientset
	TridentCRDClient *tridentClient.Clientset
	SnapshotClient   *snapshotClient.Clientset
	K8SVersion       *k8sversion.Info
	Namespace        string
}

const k8sTimeout = 30 * time.Second

func CreateK8SClients(apiServerIP, kubeConfigPath string) (*Clients, error) {
	var clients *Clients
	var err error

	// Get the API config based on whether we are running in or out of cluster
	if kubeConfigPath != "" {
		Log().Debug("Creating ex-cluster Kubernetes clients.")
		clients, err = createK8SClientsExCluster(apiServerIP, kubeConfigPath)
	} else {
		Log().Debug("Creating in-cluster Kubernetes clients.")
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

	// Create the Operator CRD client
	clients.CRDClient, err = versioned.NewForConfig(clients.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("could not initialize CRD client; %v", err)
	}

	// Create the Trident CRD client
	clients.TridentCRDClient, err = tridentClient.NewForConfig(clients.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Trident CRD client; %v", err)
	}

	// Create the Snapshot CRD client
	clients.SnapshotClient, err = snapshotClient.NewForConfig(clients.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Snapshot CRD client; %v", err)
	}

	// Get the Kubernetes server version
	clients.K8SVersion, err = clients.KubeClient.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("could not get Kubernetes version: %v", err)
	}

	Log().WithFields(LogFields{
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
	namespaceBytes, err := os.ReadFile(commonconfig.NamespaceFile)
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
