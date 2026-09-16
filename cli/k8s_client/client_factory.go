// Copyright 2025 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	k8ssnapshots "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	torc "github.com/netapp/trident/operator/crd/client/clientset/versioned"
	tridentv1clientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils/errors"
	tridentexec "github.com/netapp/trident/utils/exec"
)

type Clients struct {
	RestConfig     *rest.Config
	KubeClient     *kubernetes.Clientset
	SnapshotClient *k8ssnapshots.Clientset
	K8SClient      KubernetesClient
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

// command executes subprocesses for CLI discovery (tests replace with a mock Command).
var command = tridentexec.NewCommand()

// CreateK8SClients returns process-cached Kubernetes clients when already built, otherwise
// constructs and caches them. Subsequent calls return the first successful result regardless
// of arguments (intended for long-lived tridentctl / helper processes).
func CreateK8SClients(masterURL, kubeConfigPath, overrideNamespace string) (*Clients, error) {
	ctx := GenerateRequestContext(nil, "", "", WorkflowK8sClientFactory, LogLayerNone)
	Logc(ctx).WithFields(LogFields{
		"MasterURL":         masterURL,
		"KubeConfigPath":    kubeConfigPath,
		"OverrideNamespace": overrideNamespace,
	}).Trace(">>>> CreateK8SClients")
	defer Logc(ctx).Trace("<<<< CreateK8SClients")

	if cachedClients != nil {
		return cachedClients, nil
	}
	clients, err := buildK8SClients(ctx, masterURL, kubeConfigPath, overrideNamespace)
	if err != nil {
		return nil, err
	}
	cachedClients = clients
	return clients, nil
}

// BuildK8SClients constructs Kubernetes clients without using or updating the process cache.
func BuildK8SClients(masterURL, kubeConfigPath, overrideNamespace string) (*Clients, error) {
	ctx := GenerateRequestContext(nil, "", "", WorkflowK8sClientFactory, LogLayerNone)
	return buildK8SClients(ctx, masterURL, kubeConfigPath, overrideNamespace)
}

func buildK8SClients(
	ctx context.Context, masterURL, kubeConfigPath, overrideNamespace string,
) (*Clients, error) {
	Logc(ctx).WithFields(LogFields{
		"MasterURL":         masterURL,
		"KubeConfigPath":    kubeConfigPath,
		"OverrideNamespace": overrideNamespace,
	}).Trace(">>>> BuildK8SClients")
	defer Logc(ctx).Trace("<<<< BuildK8SClients")

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

	// Register client-go metrics adapters to feed Trident telemeters
	registerK8sClientGoMetricsAdapter()

	// Wrap all interaction with K8s in a metrics transport http.RoundTripper.
	clients.RestConfig.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return NewMetricsTransport(
			rt,
			WithMetricsTransportTarget(ContextRequestTargetKubernetes),
		)
	})

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

	return clients, nil
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
		out, err := command.ExecuteWithoutLog(ctx, kubernetesCLI, "config", "view", "--raw")
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
	restConfig.QPS = config.K8sAPIQPS
	restConfig.Burst = config.K8sAPIBurst
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
	restConfig.QPS = config.K8sAPIQPS
	restConfig.Burst = config.K8sAPIBurst

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
	_, err := command.ExecuteWithoutLog(ctx, CLIOpenShift, "version")
	if getExitCodeFromError(err) == ExitCodeSuccess {
		return CLIOpenShift, nil
	}

	// Fall back to the K8S CLI
	_, err = command.ExecuteWithoutLog(ctx, CLIKubernetes, "version")
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
