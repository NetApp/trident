// Copyright 2021 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	k8ssnapshots "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	torc "github.com/netapp/trident/operator/controllers/orchestrator/client/clientset/versioned"
	tprov "github.com/netapp/trident/operator/controllers/provisioner/client/clientset/versioned"
	tridentv1clientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils/errors"
)

type Clients struct {
	RestConfig     *rest.Config
	KubeClient     *kubernetes.Clientset
	SnapshotClient *k8ssnapshots.Clientset
	K8SClient      KubernetesClient
	TprovClient    *tprov.Clientset
	TorcClient     *torc.Clientset
	TridentClient  *tridentv1clientset.Clientset
	K8SVersion     *k8sversion.Info
	Namespace      string
	InK8SPod       bool
}

const (
	k8sTimeout       = 30 * time.Second
	defaultNamespace = "default"
)

var cachedClients *Clients

// CreateK8SClients is the top-level factory method for creating Kubernetes clients.  Whether this code is running
// inside or outside a pod is detected automatically.  If inside, we can get the kubeconfig and namespace from the
// context we are running in.  If outside, either a kubeconfig is specified or we use kubectl/oc to read the kubeconfig
// using `kubectl config view --raw` and we attempt to discern the namespace from the kubeconfig context.  The
// namespace may be overridden, and if the namespace may not be determined by any other means, it is set to 'default'.
func CreateK8SClients(masterURL, kubeConfigPath, overrideNamespace string) (*Clients, error) {
	ctx := GenerateRequestContext(nil, "", "", WorkflowK8sClientFactory, LogLayerNone)
	Logc(ctx).WithFields(LogFields{
		"MasterURL":         masterURL,
		"KubeConfigPath":    kubeConfigPath,
		"OverrideNamespace": overrideNamespace,
	}).Trace(">>>> CreateK8SClients")
	defer Logc(ctx).Trace("<<<< CreateK8SClients")

	// Return a cached copy if available
	if cachedClients != nil {
		return cachedClients, nil
	}

	var clients *Clients
	var err error

	inK8SPod := true
	if _, err := os.Stat(config.NamespaceFile); os.IsNotExist(err) {
		inK8SPod = false
	}

	// Get the API config based on whether we are running in or out of cluster
	if !inK8SPod {
		Logc(ctx).Debug("Creating ex-cluster Kubernetes clients.")
		clients, err = createK8SClientsExCluster(ctx, masterURL, kubeConfigPath, overrideNamespace)
	} else {
		Logc(ctx).Debug("Creating in-cluster Kubernetes clients.")
		clients, err = createK8SClientsInCluster(ctx, overrideNamespace)
	}
	if err != nil {
		return nil, err
	}

	// Create the Kubernetes client
	clients.KubeClient, err = kubernetes.NewForConfig(clients.RestConfig)
	if err != nil {
		return nil, err
	}
	// Create the k8s snapshot client
	clients.SnapshotClient, err = k8ssnapshots.NewForConfig(clients.RestConfig)
	if err != nil {
		return nil, err
	}

	// Create the CRD clients
	clients.TprovClient, err = tprov.NewForConfig(clients.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Tprov CRD client; %v", err)
	}
	clients.TorcClient, err = torc.NewForConfig(clients.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Torc CRD client; %v", err)
	}
	clients.TridentClient, err = tridentv1clientset.NewForConfig(clients.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Trident CRD client; %v", err)
	}

	// Get the Kubernetes server version
	clients.K8SVersion, err = clients.KubeClient.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("could not get Kubernetes version: %v", err)
	}

	clients.InK8SPod = inK8SPod

	Logc(ctx).WithFields(LogFields{
		"namespace": clients.Namespace,
		"version":   clients.K8SVersion.String(),
	}).Debug("Created Kubernetes clients.")

	cachedClients = clients

	return cachedClients, nil
}

func createK8SClientsExCluster(
	ctx context.Context, masterURL, kubeConfigPath, overrideNamespace string,
) (*Clients, error) {
	var namespace string
	var restConfig *rest.Config
	var err error

	Logc(ctx).WithFields(LogFields{
		"MasterURL":         masterURL,
		"KubeConfigPath":    kubeConfigPath,
		"OverrideNamespace": overrideNamespace,
	}).Trace(">>>> createK8SClientsExCluster")
	defer Logc(ctx).Trace("<<<< createK8SClientsExCluster")

	if kubeConfigPath != "" {
		Logc(ctx).Trace("Provided kubeConfigPath not empty.")

		if restConfig, err = clientcmd.BuildConfigFromFlags(masterURL, kubeConfigPath); err != nil {
			return nil, err
		}

		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		rules.ExplicitPath = kubeConfigPath

		apiConfig, err := rules.Load()
		if err != nil {
			return nil, err
		} else if apiConfig.CurrentContext == "" {
			return nil, errors.New("current context is empty")
		}

		currentContext := apiConfig.Contexts[apiConfig.CurrentContext]
		if currentContext == nil {
			return nil, errors.New("current context is nil")
		}
		namespace = currentContext.Namespace

	} else {
		Logc(ctx).Trace("Provided kubeConfigPath empty.")
		// Get K8S CLI
		kubernetesCLI, err := discoverKubernetesCLI(ctx)
		if err != nil {
			return nil, err
		}

		// c.cli config view --raw
		args := []string{"config", "view", "--raw"}

		out, err := exec.Command(kubernetesCLI, args...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("%s; %v", string(out), err)
		}

		clientConfig, err := clientcmd.NewClientConfigFromBytes(out)
		if err != nil {
			return nil, err
		}

		restConfig, err = clientConfig.ClientConfig()
		if err != nil {
			return nil, err
		}

		namespace, _, err = clientConfig.Namespace()
		if err != nil {
			return nil, err
		}
	}

	if namespace == "" {
		namespace = defaultNamespace
	}
	if overrideNamespace != "" {
		namespace = overrideNamespace
	}

	// Create the CLI-based Kubernetes client
	k8sClient, err := NewKubeClient(restConfig, namespace, k8sTimeout)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	return &Clients{
		RestConfig: restConfig,
		K8SClient:  k8sClient,
		Namespace:  namespace,
	}, nil
}

func createK8SClientsInCluster(ctx context.Context, overrideNamespace string) (*Clients, error) {
	Logc(ctx).WithFields(LogFields{
		"OverrideNamespace": overrideNamespace,
	}).Trace(">>>> createK8SClientsInCluster")
	defer Logc(ctx).Trace("<<<< createK8SClientsInCluster")

	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	// when running in a pod, we use the Trident pod's namespace
	namespaceBytes, err := os.ReadFile(config.NamespaceFile)
	if err != nil {
		return nil, fmt.Errorf("could not read namespace file %s; %v", config.NamespaceFile, err)
	}
	namespace := string(namespaceBytes)

	if overrideNamespace != "" {
		namespace = overrideNamespace
	}

	// Create the Kubernetes client
	k8sClient, err := NewKubeClient(restConfig, namespace, k8sTimeout)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	return &Clients{
		RestConfig: restConfig,
		K8SClient:  k8sClient,
		Namespace:  namespace,
	}, nil
}

func discoverKubernetesCLI(ctx context.Context) (string, error) {
	Logc(ctx).Trace(">>>> discoverKubernetesCLI")
	defer Logc(ctx).Trace("<<<< discoverKubernetesCLI")
	// Try the OpenShift CLI first
	_, err := exec.Command(CLIOpenShift, "version").Output()
	if getExitCodeFromError(err) == ExitCodeSuccess {
		return CLIOpenShift, nil
	}

	// Fall back to the K8S CLI
	_, err = exec.Command(CLIKubernetes, "version").Output()
	if getExitCodeFromError(err) == ExitCodeSuccess {
		return CLIKubernetes, nil
	}

	if ee, ok := err.(*exec.ExitError); ok {
		return "", fmt.Errorf("found the Kubernetes CLI, but it exited with error: %s",
			strings.TrimRight(string(ee.Stderr), "\n"))
	}

	return "", fmt.Errorf("could not find the Kubernetes CLI: %v", err)
}

func getExitCodeFromError(err error) int {
	if err == nil {
		return ExitCodeSuccess
	} else {

		// Default to 1 in case we can't determine a process exit code
		code := ExitCodeFailure

		if exitError, ok := err.(*exec.ExitError); ok {
			ws := exitError.Sys().(syscall.WaitStatus)
			code = ws.ExitStatus()
		}

		return code
	}
}
