// Copyright 2023 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/ghodss/yaml"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	k8ssnapshots "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	v13 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	. "github.com/netapp/trident/logging"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentv1clientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils/errors"
	versionutils "github.com/netapp/trident/utils/version"
)

type OrchestratorFlavor string

const (
	CLIKubernetes = "kubectl"
	CLIOpenShift  = "oc"

	FlavorKubernetes OrchestratorFlavor = "k8s"
	FlavorOpenShift  OrchestratorFlavor = "openshift"

	ExitCodeSuccess = 0
	ExitCodeFailure = 1

	YAMLSeparator = `\n---\s*\n`

	// CRD Finalizer name
	TridentFinalizer = "trident.netapp.io"

	CloudProviderAzure         = "Azure"
	CloudProviderAWS           = "AWS"
	CloudProviderGCP           = "GCP"
	AzureCloudIdentityKey      = "azure.workload.identity/client-id:"
	AzureWorkloadIdentityLabel = "azure.workload.identity/use: 'true'"
	AWSCloudIdentityKey        = "eks.amazonaws.com/role-arn:"
	GCPCloudIdentityKey        = "iam.gke.io/gcp-service-account:"
)

type LogLineCallback func(string)

var (
	listOpts   = metav1.ListOptions{}
	getOpts    = metav1.GetOptions{}
	createOpts = metav1.CreateOptions{}
	updateOpts = metav1.UpdateOptions{}
	patchOpts  = metav1.PatchOptions{}

	ctx    = GenerateRequestContext(nil, "", "", WorkflowK8sClientAPI, LogLayerNone)
	reqCtx = context.Background

	yamlToJSON     = yaml.YAMLToJSON
	jsonMarshal    = json.Marshal
	jsonUnmarshal  = json.Unmarshal
	jsonMergePatch = jsonpatch.MergePatch
)

type KubeClient struct {
	clientset    kubernetes.Interface
	extClientset apiextension.Interface
	crdClientset tridentv1clientset.Interface
	apiResources map[string]*metav1.APIResourceList
	restConfig   *rest.Config
	namespace    string
	versionInfo  *version.Info
	cli          string
	flavor       OrchestratorFlavor
	timeout      time.Duration
}

func NewKubeClient(config *rest.Config, namespace string, k8sTimeout time.Duration) (KubernetesClient, error) {
	var versionInfo *version.Info
	if namespace == "" {
		return nil, errors.New("an empty namespace is not acceptable")
	}

	// Create core client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	// Create extension client
	extClientset, err := apiextension.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	crdClientset, err := tridentv1clientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	versionInfo, err = clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("couldn't retrieve API server's version: %v", err)
	}

	kubeClient := &KubeClient{
		clientset:    clientset,
		extClientset: extClientset,
		crdClientset: crdClientset,
		apiResources: make(map[string]*metav1.APIResourceList),
		restConfig:   config,
		namespace:    namespace,
		versionInfo:  versionInfo,
		timeout:      k8sTimeout,
	}

	// Initialize the API resource cache
	kubeClient.getAPIResources()

	kubeClient.flavor = kubeClient.discoverKubernetesFlavor()

	switch kubeClient.flavor {
	case FlavorKubernetes:
		kubeClient.cli = CLIKubernetes
	case FlavorOpenShift:
		kubeClient.cli = CLIOpenShift
	}

	Logc(ctx).WithFields(LogFields{
		"cli":       kubeClient.cli,
		"flavor":    kubeClient.flavor,
		"version":   kubeClient.Version().String(),
		"timeout":   kubeClient.timeout,
		"namespace": kubeClient.namespace,
	}).Trace("Initialized Kubernetes API client.")

	return kubeClient, nil
}

func NewFakeKubeClient() (KubernetesClient, error) {
	// Create core client
	clientset := fake.NewSimpleClientset()
	kubeClient := &KubeClient{
		clientset:    clientset,
		apiResources: make(map[string]*metav1.APIResourceList),
	}

	return kubeClient, nil
}

// getAPIResources attempts to get all API resources from the K8S API server, and it populates this
// clients resource cache with the returned data.  This function is called only once, as calling it
// repeatedly when one or more API resources are unavailable can trigger API throttling.  If this
// cache population fails, then the cache will be populated one GVR at a time on demand, which works
// even if other resources aren't available.  This function's discovery client invocation performs the
// equivalent of "kubectl api-resources".
func (k *KubeClient) getAPIResources() {
	// Read the API groups/resources available from the server
	if _, apiResources, err := k.clientset.Discovery().ServerGroupsAndResources(); err != nil {
		Logc(ctx).WithField("error", err).Warn("Could not get all server resources.")
	} else {
		// Update the cache
		for _, apiResourceList := range apiResources {
			k.apiResources[apiResourceList.GroupVersion] = apiResourceList
		}
	}
}

func (k *KubeClient) discoverKubernetesFlavor() OrchestratorFlavor {
	// Look through the API resource cache for any with openshift group
	for groupVersion := range k.apiResources {

		gv, err := schema.ParseGroupVersion(groupVersion)
		if err != nil {
			Logc(ctx).WithField("groupVersion", groupVersion).Warnf("Could not parse group/version; %v", err)
			continue
		}

		Logc(ctx).WithFields(LogFields{
			"group":   gv.Group,
			"version": gv.Version,
		}).Trace("Considering dynamic resource, looking for openshift group.")

		// OCP will have an openshift api server we can use to determine if the environment is OCP or Kubernetes
		if strings.Contains(gv.Group, "config.openshift.io") {
			return FlavorOpenShift
		}
	}

	return FlavorKubernetes
}

func (k *KubeClient) Version() *version.Info {
	return k.versionInfo
}

func (k *KubeClient) ServerVersion() *versionutils.Version {
	return versionutils.MustParseSemantic(k.versionInfo.GitVersion)
}

func (k *KubeClient) Flavor() OrchestratorFlavor {
	return k.flavor
}

func (k *KubeClient) CLI() string {
	return k.cli
}

func (k *KubeClient) Namespace() string {
	return k.namespace
}

func (k *KubeClient) SetNamespace(namespace string) {
	k.namespace = namespace
}

func (k *KubeClient) SetTimeout(timeout time.Duration) {
	k.timeout = timeout
}

func (k *KubeClient) Exec(podName, containerName string, commandArgs []string) ([]byte, error) {
	// Get the pod and ensure it is in a good state
	pod, err := k.clientset.CoreV1().Pods(k.namespace).Get(reqCtx(), podName, getOpts)
	if err != nil {
		return nil, err
	}

	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		return nil, fmt.Errorf("cannot exec into a completed pod; current phase is %s", pod.Status.Phase)
	}

	// Infer container name if necessary
	if len(containerName) == 0 {
		if len(pod.Spec.Containers) > 1 {
			return nil, fmt.Errorf("pod %s has multiple containers, but no container was specified", pod.Name)
		}
		containerName = pod.Spec.Containers[0].Name
	}

	// Construct the request
	req := k.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		Param("container", containerName)

	req.VersionedParams(&v1.PodExecOptions{
		Container: containerName,
		Command:   commandArgs,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(k.restConfig, "POST", req.URL())
	if err != nil {
		return nil, err
	}

	// Set up the data streams
	var stdinBuffer, stdoutBuffer, stderrBuffer bytes.Buffer
	stdin := bufio.NewReader(&stdinBuffer)
	stdout := bufio.NewWriter(&stdoutBuffer)
	stderr := bufio.NewWriter(&stderrBuffer)

	var sizeQueue remotecommand.TerminalSizeQueue

	Logc(ctx).Debugf("Invoking tunneled command: '%v'", strings.Join(commandArgs, " "))

	// Execute the request
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             stdin,
		Stdout:            stdout,
		Stderr:            stderr,
		Tty:               false,
		TerminalSizeQueue: sizeQueue,
	})

	// Combine all output into a single buffer
	var combinedBuffer bytes.Buffer
	combinedBuffer.Write(stdoutBuffer.Bytes())
	combinedBuffer.Write(stderrBuffer.Bytes())
	if err != nil {
		combinedBuffer.WriteString(err.Error())
	}

	return combinedBuffer.Bytes(), err
}

// GetDeploymentByLabel returns a deployment object matching the specified label if it is unique
func (k *KubeClient) GetDeploymentByLabel(label string, allNamespaces bool) (*appsv1.Deployment, error) {
	deployments, err := k.GetDeploymentsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(deployments) == 1 {
		return &deployments[0], nil
	} else if len(deployments) > 1 {
		return nil, fmt.Errorf("multiple deployments have the label %s", label)
	} else {
		return nil, errors.NotFoundError("no deployments have the label %s", label)
	}
}

// GetDeploymentByLabel returns all deployment objects matching the specified label
func (k *KubeClient) GetDeploymentsByLabel(label string, allNamespaces bool) ([]appsv1.Deployment, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	namespace := k.namespace
	if allNamespaces {
		namespace = ""
	}

	deploymentList, err := k.clientset.AppsV1().Deployments(namespace).List(reqCtx(), listOptions)
	if err != nil {
		return nil, err
	}

	return deploymentList.Items, nil
}

// CheckDeploymentExists returns true if the specified deployment exists.
func (k *KubeClient) CheckDeploymentExists(name, namespace string) (bool, error) {
	if _, err := k.clientset.AppsV1().Deployments(namespace).Get(reqCtx(), name, getOpts); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CheckDeploymentExistsByLabel returns true if one or more deployment objects
// matching the specified label exist.
func (k *KubeClient) CheckDeploymentExistsByLabel(label string, allNamespaces bool) (bool, string, error) {
	deployments, err := k.GetDeploymentsByLabel(label, allNamespaces)
	if err != nil {
		return false, "", err
	}

	switch len(deployments) {
	case 0:
		return false, "", nil
	case 1:
		return true, deployments[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeleteDeploymentByLabel deletes a deployment object matching the specified label
// in the namespace of the client.
func (k *KubeClient) DeleteDeploymentByLabel(label string) error {
	deployment, err := k.GetDeploymentByLabel(label, false)
	if err != nil {
		return err
	}

	err = k.clientset.AppsV1().Deployments(k.namespace).Delete(reqCtx(), deployment.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":      label,
		"deployment": deployment.Name,
		"namespace":  k.namespace,
	}).Trace("Deleted Kubernetes deployment.")

	return nil
}

// DeleteDeployment deletes a deployment object matching the specified name and namespace.
func (k *KubeClient) DeleteDeployment(name, namespace string, foreground bool) error {
	if !foreground {
		return k.deleteDeploymentBackground(name, namespace)
	}
	return k.deleteDeploymentForeground(name, namespace)
}

// deleteDeploymentBackground deletes a deployment object matching the specified name and namespace.  It does so
// with a background propagation policy, so it returns immediately and the Kubernetes garbage collector reaps any
// associated pod(s) asynchronously.
func (k *KubeClient) deleteDeploymentBackground(name, namespace string) error {
	if err := k.clientset.AppsV1().Deployments(namespace).Delete(reqCtx(), name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"deployment": name,
		"namespace":  namespace,
	}).Trace("Deleted Kubernetes deployment.")

	return nil
}

// deleteDeploymentForeground deletes a deployment object matching the specified name and namespace.  It does so
// with a foreground propagation policy, so that it can then wait for the deployment to disappear which will happen
// only after all owned objects (i.e. pods) are cleaned up.
func (k *KubeClient) deleteDeploymentForeground(name, namespace string) error {
	logFields := LogFields{"name": name, "namespace": namespace}
	Logc(ctx).WithFields(logFields).Debug("Starting foreground deletion of Deployment.")

	propagationPolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}

	if err := k.clientset.AppsV1().Deployments(namespace).Delete(reqCtx(), name, deleteOptions); err != nil {
		return err
	}

	checkDeploymentExists := func() error {
		if exists, err := k.CheckDeploymentExists(name, namespace); err != nil {
			return err
		} else if exists {
			return fmt.Errorf("deployment %s/%s exists", namespace, name)
		}
		return nil
	}

	checkDeploymentNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"name":      name,
			"namespace": namespace,
			"increment": duration,
			"err":       err,
		}).Tracef("Deployment still exists, waiting.")
	}
	checkDeploymentBackoff := backoff.NewExponentialBackOff()
	checkDeploymentBackoff.MaxElapsedTime = k.timeout

	Logc(ctx).WithFields(logFields).Trace("Waiting for Deployment to be deleted.")

	if err := backoff.RetryNotify(checkDeploymentExists, checkDeploymentBackoff, checkDeploymentNotify); err != nil {
		return fmt.Errorf("deployment %s/%s was not deleted after %3.2f seconds",
			namespace, name, k.timeout.Seconds())
	}

	Logc(ctx).WithFields(logFields).Debug("Completed foreground deletion of Deployment.")

	return nil
}

// PatchDeploymentByLabel patches a deployment object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchDeploymentByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	deployment, err := k.GetDeploymentByLabel(label, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.AppsV1().Deployments(k.namespace).Patch(reqCtx(), deployment.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":      label,
		"deployment": deployment.Name,
		"namespace":  k.namespace,
	}).Trace("Patched Kubernetes deployment.")

	return nil
}

// GetServiceByLabel returns a service object matching the specified label if it is unique
func (k *KubeClient) GetServiceByLabel(label string, allNamespaces bool) (*v1.Service, error) {
	services, err := k.GetServicesByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(services) == 1 {
		return &services[0], nil
	} else if len(services) > 1 {
		return nil, fmt.Errorf("multiple services have the label %s", label)
	} else {
		return nil, errors.NotFoundError("no services have the label %s", label)
	}
}

// GetServicesByLabel returns all service objects matching the specified label
func (k *KubeClient) GetServicesByLabel(label string, allNamespaces bool) ([]v1.Service, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	namespace := k.namespace
	if allNamespaces {
		namespace = ""
	}

	serviceList, err := k.clientset.CoreV1().Services(namespace).List(reqCtx(), listOptions)
	if err != nil {
		return nil, err
	}

	return serviceList.Items, nil
}

// CheckServiceExistsByLabel returns true if one or more service objects
// matching the specified label exist.
func (k *KubeClient) CheckServiceExistsByLabel(label string, allNamespaces bool) (bool, string, error) {
	services, err := k.GetServicesByLabel(label, allNamespaces)
	if err != nil {
		return false, "", err
	}

	switch len(services) {
	case 0:
		return false, "", nil
	case 1:
		return true, services[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeleteServiceByLabel deletes a service object matching the specified label
// in the namespace of the client.
func (k *KubeClient) DeleteServiceByLabel(label string) error {
	service, err := k.GetServiceByLabel(label, false)
	if err != nil {
		return err
	}

	if err = k.clientset.CoreV1().Services(k.namespace).Delete(reqCtx(), service.Name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":     label,
		"service":   service.Name,
		"namespace": k.namespace,
	}).Trace("Deleted Kubernetes service.")

	return nil
}

// DeleteService deletes a Service object matching the specified name and namespace
func (k *KubeClient) DeleteService(name, namespace string) error {
	if err := k.clientset.CoreV1().Services(namespace).Delete(reqCtx(), name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"Service":   name,
		"namespace": namespace,
	}).Trace("Deleted Kubernetes Service.")

	return nil
}

// PatchServiceByLabel patches a deployment object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchServiceByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	service, err := k.GetServiceByLabel(label, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.CoreV1().Services(k.namespace).Patch(reqCtx(), service.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":      label,
		"deployment": service.Name,
		"namespace":  k.namespace,
	}).Trace("Patched Kubernetes service.")

	return nil
}

// GetDaemonSetByLabel returns a daemonset object matching the specified label if it is unique
func (k *KubeClient) GetDaemonSetByLabel(label string, allNamespaces bool) (*appsv1.DaemonSet, error) {
	daemonsets, err := k.GetDaemonSetsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(daemonsets) == 1 {
		return &daemonsets[0], nil
	} else if len(daemonsets) > 1 {
		return nil, fmt.Errorf("multiple daemonsets have the label %s", label)
	} else {
		return nil, errors.NotFoundError("no daemonsets have the label %s", label)
	}
}

// GetDaemonSetByLabelAndName returns a daemonset object matching the specified label and name
func (k *KubeClient) GetDaemonSetByLabelAndName(label, name string, allNamespaces bool) (*appsv1.DaemonSet,
	error,
) {
	daemonsets, err := k.GetDaemonSetsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	var daemonset *appsv1.DaemonSet

	for i := range daemonsets {
		if daemonsets[i].Name == name {
			daemonset = &daemonsets[i]
		}
	}
	return daemonset, nil
}

// GetDaemonSetsByLabel returns all daemonset objects matching the specified label
func (k *KubeClient) GetDaemonSetsByLabel(label string, allNamespaces bool) ([]appsv1.DaemonSet, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	namespace := k.namespace
	if allNamespaces {
		namespace = ""
	}

	daemonSetList, err := k.clientset.AppsV1().DaemonSets(namespace).List(reqCtx(), listOptions)
	if err != nil {
		return nil, err
	}

	return daemonSetList.Items, nil
}

// CheckDaemonSetExists returns true if the specified daemonset exists.
func (k *KubeClient) CheckDaemonSetExists(name, namespace string) (bool, error) {
	if _, err := k.clientset.AppsV1().DaemonSets(namespace).Get(reqCtx(), name, getOpts); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CheckDaemonSetExistsByLabel returns true if one or more daemonset objects
// matching the specified label exist.
func (k *KubeClient) CheckDaemonSetExistsByLabel(label string, allNamespaces bool) (bool, string, error) {
	daemonsets, err := k.GetDaemonSetsByLabel(label, allNamespaces)
	if err != nil {
		return false, "", err
	}

	switch len(daemonsets) {
	case 0:
		return false, "", nil
	case 1:
		return true, daemonsets[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeleteDaemonSetByLabel deletes a daemonset object matching the specified label
// in the namespace of the client.
func (k *KubeClient) DeleteDaemonSetByLabel(label string) error {
	daemonset, err := k.GetDaemonSetByLabel(label, false)
	if err != nil {
		return err
	}

	err = k.clientset.AppsV1().DaemonSets(k.namespace).Delete(reqCtx(), daemonset.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":     label,
		"daemonset": daemonset.Name,
		"namespace": k.namespace,
	}).Trace("Deleted Kubernetes daemonset.")

	return nil
}

// DeleteDaemonSetByLabelAndName deletes a daemonset object matching the specified label and name
// in the namespace of the client.
func (k *KubeClient) DeleteDaemonSetByLabelAndName(label, name string) error {
	daemonset, err := k.GetDaemonSetByLabelAndName(label, name, false)
	if err != nil {
		return err
	}

	err = k.clientset.AppsV1().DaemonSets(k.namespace).Delete(reqCtx(), daemonset.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":     label,
		"daemonset": daemonset.Name,
		"namespace": k.namespace,
	}).Trace("Deleted Kubernetes daemonset.")

	return nil
}

// DeleteDeployment deletes a deployment object matching the specified name and namespace.
func (k *KubeClient) DeleteDaemonSet(name, namespace string, foreground bool) error {
	if !foreground {
		return k.deleteDaemonSetBackground(name, namespace)
	}
	return k.deleteDaemonSetForeground(name, namespace)
}

// deleteDaemonSetBackground deletes a daemonset object matching the specified name and namespace.  It does so
// with a background propagation policy, so it returns immediately and the Kubernetes garbage collector reaps any
// associated pod(s) asynchronously.
func (k *KubeClient) deleteDaemonSetBackground(name, namespace string) error {
	Logc(ctx).WithFields(LogFields{
		"name":      name,
		"namespace": namespace,
	}).Debug("Starting background deletion of DaemonSet.")

	if err := k.clientset.AppsV1().DaemonSets(namespace).Delete(reqCtx(), name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"DaemonSet": name,
		"namespace": namespace,
	}).Trace("Deleted Kubernetes DaemonSet.")

	return nil
}

// deleteDaemonSetForeground deletes a daemonset object matching the specified name and namespace.  It does so
// with a foreground propagation policy, so that it can then wait for the daemonset to disappear which will happen
// only after all owned objects (i.e. pods) are cleaned up.
func (k *KubeClient) deleteDaemonSetForeground(name, namespace string) error {
	logFields := LogFields{"name": name, "namespace": namespace}
	Logc(ctx).WithFields(logFields).Debug("Starting foreground deletion of DaemonSet.")

	propagationPolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}

	if err := k.clientset.AppsV1().DaemonSets(namespace).Delete(reqCtx(), name, deleteOptions); err != nil {
		return err
	}

	checkDaemonSetExists := func() error {
		if exists, err := k.CheckDaemonSetExists(name, namespace); err != nil {
			return err
		} else if exists {
			return fmt.Errorf("daemonset %s/%s exists", namespace, name)
		}
		return nil
	}

	checkDaemonSetNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"name":      name,
			"namespace": namespace,
			"increment": duration,
			"err":       err,
		}).Debugf("DaemonSet still exists, waiting.")
	}
	checkDaemonSetBackoff := backoff.NewExponentialBackOff()
	checkDaemonSetBackoff.MaxElapsedTime = k.timeout

	Logc(ctx).WithFields(logFields).Trace("Waiting for DaemonSet to be deleted.")

	if err := backoff.RetryNotify(checkDaemonSetExists, checkDaemonSetBackoff, checkDaemonSetNotify); err != nil {
		return fmt.Errorf("daemonset %s/%s was not deleted after %3.2f seconds",
			namespace, name, k.timeout.Seconds())
	}

	Logc(ctx).WithFields(logFields).Debug("Completed foreground deletion of DaemonSet.")

	return nil
}

// PatchDaemonSetByLabelAndName patches a DaemonSet object matching the specified label and name
// in the namespace of the client.
func (k *KubeClient) PatchDaemonSetByLabelAndName(
	label, daemonSetName string, patchBytes []byte, patchType types.PatchType,
) error {
	daemonSet, err := k.GetDaemonSetByLabelAndName(label, daemonSetName, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.AppsV1().DaemonSets(k.namespace).Patch(reqCtx(), daemonSet.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":     label,
		"DaemonSet": daemonSet.Name,
		"namespace": k.namespace,
	}).Trace("Patched Kubernetes DaemonSet.")

	return nil
}

// PatchDaemonSetByLabel patches a DaemonSet object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchDaemonSetByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	daemonSet, err := k.GetDaemonSetByLabel(label, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.AppsV1().DaemonSets(k.namespace).Patch(reqCtx(), daemonSet.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":     label,
		"DaemonSet": daemonSet.Name,
		"namespace": k.namespace,
	}).Trace("Patched Kubernetes DaemonSet.")

	return nil
}

// GetPodByLabel returns a pod object matching the specified label
func (k *KubeClient) GetPodByLabel(label string, allNamespaces bool) (*v1.Pod, error) {
	pods, err := k.GetPodsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(pods) == 1 {
		return &pods[0], nil
	} else if len(pods) > 1 {
		return nil, fmt.Errorf("multiple pods have the label %s", label)
	} else {
		return nil, errors.NotFoundError("no pods have the label %s", label)
	}
}

// GetPodsByLabel returns all pod objects matching the specified label
func (k *KubeClient) GetPodsByLabel(label string, allNamespaces bool) ([]v1.Pod, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	namespace := k.namespace
	if allNamespaces {
		namespace = ""
	}

	podList, err := k.clientset.CoreV1().Pods(namespace).List(reqCtx(), listOptions)
	if err != nil {
		return nil, err
	}

	return podList.Items, nil
}

// CheckPodExistsByLabel returns true if one or more pod objects
// matching the specified label exist.
func (k *KubeClient) CheckPodExistsByLabel(label string, allNamespaces bool) (bool, string, error) {
	pods, err := k.GetPodsByLabel(label, allNamespaces)
	if err != nil {
		return false, "", err
	}

	switch len(pods) {
	case 0:
		return false, "", nil
	case 1:
		return true, pods[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeletePodByLabel deletes a pod object matching the specified label
func (k *KubeClient) DeletePodByLabel(label string) error {
	pod, err := k.GetPodByLabel(label, false)
	if err != nil {
		return err
	}

	if err = k.clientset.CoreV1().Pods(k.namespace).Delete(reqCtx(), pod.Name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":     label,
		"pod":       pod.Name,
		"namespace": k.namespace,
	}).Trace("Deleted Kubernetes pod.")

	return nil
}

// DeletePod deletes a pod object matching the specified name and namespace
func (k *KubeClient) DeletePod(name, namespace string) error {
	var gracePeriod int64
	deleteOptions := &metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	}

	if err := k.clientset.CoreV1().Pods(namespace).Delete(reqCtx(), name, *deleteOptions); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"pod":       name,
		"namespace": namespace,
	}).Trace("Deleted Kubernetes pod.")

	return nil
}

func (k *KubeClient) GetCRD(crdName string) (*apiextensionv1.CustomResourceDefinition, error) {
	var options metav1.GetOptions
	return k.extClientset.ApiextensionsV1().CustomResourceDefinitions().Get(reqCtx(), crdName, options)
}

func (k *KubeClient) CheckCRDExists(crdName string) (bool, error) {
	if _, err := k.GetCRD(crdName); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k *KubeClient) DeleteCRD(crdName string) error {
	return k.extClientset.ApiextensionsV1().CustomResourceDefinitions().Delete(reqCtx(), crdName, k.deleteOptions())
}

// GetServiceAccountByLabelAndName returns a service account object matching the specified label and name
func (k *KubeClient) GetServiceAccountByLabelAndName(label, serviceAccountName string, allNamespaces bool) (
	*v1.ServiceAccount, error,
) {
	serviceAccounts, err := k.GetServiceAccountsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	var serviceAccount *v1.ServiceAccount

	for i := range serviceAccounts {
		if serviceAccounts[i].Name == serviceAccountName {
			serviceAccount = &serviceAccounts[i]
		}
	}
	return serviceAccount, nil
}

// GetServiceAccountByLabel returns a service account object matching the specified label if it is unique
func (k *KubeClient) GetServiceAccountByLabel(label string, allNamespaces bool) (*v1.ServiceAccount, error) {
	serviceAccounts, err := k.GetServiceAccountsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(serviceAccounts) == 1 {
		return &serviceAccounts[0], nil
	} else if len(serviceAccounts) > 1 {
		return nil, fmt.Errorf("multiple service accounts have the label %s", label)
	} else {
		return nil, errors.NotFoundError("no service accounts have the label %s", label)
	}
}

// GetServiceAccountsByLabel returns all service account objects matching the specified label
func (k *KubeClient) GetServiceAccountsByLabel(label string, allNamespaces bool) ([]v1.ServiceAccount, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	namespace := k.namespace
	if allNamespaces {
		namespace = ""
	}

	serviceAccountList, err := k.clientset.CoreV1().ServiceAccounts(namespace).List(reqCtx(), listOptions)
	if err != nil {
		return nil, err
	}

	return serviceAccountList.Items, nil
}

// CheckServiceAccountExists returns true if a matching service account exists.
func (k *KubeClient) CheckServiceAccountExists(name, namespace string) (bool, error) {
	if _, err := k.clientset.CoreV1().ServiceAccounts(namespace).Get(reqCtx(), name, getOpts); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CheckServiceAccountExistsByLabel returns true if one or more service account objects
// matching the specified label exist.
func (k *KubeClient) CheckServiceAccountExistsByLabel(label string, allNamespaces bool) (bool, string, error) {
	serviceAccounts, err := k.GetServiceAccountsByLabel(label, allNamespaces)
	if err != nil {
		return false, "", err
	}

	switch len(serviceAccounts) {
	case 0:
		return false, "", nil
	case 1:
		return true, serviceAccounts[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeleteServiceAccountByLabel deletes a service account object matching the specified label
// in the namespace of the client.
func (k *KubeClient) DeleteServiceAccountByLabel(label string) error {
	serviceAccount, err := k.GetServiceAccountByLabel(label, false)
	if err != nil {
		return err
	}

	err = k.clientset.CoreV1().ServiceAccounts(k.namespace).Delete(reqCtx(), serviceAccount.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":          label,
		"serviceAccount": serviceAccount.Name,
		"namespace":      k.namespace,
	}).Trace("Deleted Kubernetes service account.")

	return nil
}

// DeleteServiceAccount deletes a service account object matching the specified
// name and namespace.
func (k *KubeClient) DeleteServiceAccount(name, namespace string, foreground bool) error {
	if !foreground {
		return k.deleteServiceAccountBackground(name, namespace)
	}
	return k.deleteServiceAccountForeground(name, namespace)
}

func (k *KubeClient) deleteServiceAccountBackground(name, namespace string) error {
	if err := k.clientset.CoreV1().ServiceAccounts(namespace).Delete(reqCtx(), name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"serviceAccount": name,
		"namespace":      namespace,
	}).Trace("Deleted Kubernetes service account.")

	return nil
}

// DeleteServiceAccountForeground deletes a service account object matching the specified
// name and namespace in the foreground
func (k *KubeClient) deleteServiceAccountForeground(name, namespace string) error {
	logFields := LogFields{"name": name, "namespace": namespace}
	Logc(ctx).WithFields(logFields).Debug("Starting foreground deletion of Service Account.")

	propagationPolicy := metav1.DeletePropagationForeground
	gracePeriodSeconds := int64(10)
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy:  &propagationPolicy,
		GracePeriodSeconds: &gracePeriodSeconds,
	}

	if err := k.clientset.CoreV1().ServiceAccounts(namespace).Delete(reqCtx(), name, deleteOptions); err != nil {
		return err
	}

	checkServiceAccountExists := func() error {
		if exists, err := k.CheckServiceAccountExists(name, namespace); err != nil {
			return err
		} else if exists {
			return fmt.Errorf("service account %s/%s exists", namespace, name)
		}
		return nil
	}

	checkServiceAccountNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"name":      name,
			"namespace": namespace,
			"increment": duration,
			"err":       err,
		}).Debugf("Service Account still exists, waiting.")
	}
	checkServiceAccountBackoff := backoff.NewExponentialBackOff()
	checkServiceAccountBackoff.MaxElapsedTime = k.timeout

	Logc(ctx).WithFields(logFields).Trace("Waiting for Service Account to be deleted.")

	if err := backoff.RetryNotify(checkServiceAccountExists, checkServiceAccountBackoff,
		checkServiceAccountNotify); err != nil {
		return fmt.Errorf("service account %s/%s was not deleted after %3.2f seconds",
			namespace, name, k.timeout.Seconds())
	}

	Logc(ctx).WithFields(logFields).Debug("Completed foreground deletion of Service Account.")

	return nil
}

// PatchServiceAccountByLabelAndName patches a Service Account object matching the specified label
// and name in the namespace of the client.
func (k *KubeClient) PatchServiceAccountByLabelAndName(
	label, serviceAccountName string, patchBytes []byte,
	patchType types.PatchType,
) error {
	serviceAccount, err := k.GetServiceAccountByLabelAndName(label, serviceAccountName, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.CoreV1().ServiceAccounts(k.namespace).Patch(reqCtx(), serviceAccount.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":          label,
		"serviceAccount": serviceAccount.Name,
		"namespace":      k.namespace,
	}).Trace("Patched Kubernetes Service Account.")

	return nil
}

// PatchServiceAccountByLabel patches a Service Account object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchServiceAccountByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	serviceAccount, err := k.GetServiceAccountByLabel(label, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.CoreV1().ServiceAccounts(k.namespace).Patch(reqCtx(), serviceAccount.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":          label,
		"serviceAccount": serviceAccount.Name,
		"namespace":      k.namespace,
	}).Trace("Patched Kubernetes Service Account.")

	return nil
}

// GetClusterRoleByLabelAndName returns a cluster role object matching the specified label and name
func (k *KubeClient) GetClusterRoleByLabelAndName(label, clusterRoleName string) (*v13.ClusterRole, error) {
	clusterRoles, err := k.GetClusterRolesByLabel(label)
	if err != nil {
		return nil, err
	}

	var clusterRole *v13.ClusterRole

	for i := range clusterRoles {
		if clusterRoles[i].Name == clusterRoleName {
			clusterRole = &clusterRoles[i]
		}
	}
	return clusterRole, nil
}

// GetRoleByLabelAndName returns a cluster role object matching the specified label and name
func (k *KubeClient) GetRoleByLabelAndName(label, roleName string) (*v13.Role, error) {
	roles, err := k.GetRolesByLabel(label)
	if err != nil {
		return nil, err
	}

	var role *v13.Role

	for i := range roles {
		if roles[i].Name == roleName {
			role = &roles[i]
		}
	}
	return role, nil
}

// GetClusterRoleByLabel returns a cluster role object matching the specified label if it is unique
func (k *KubeClient) GetClusterRoleByLabel(label string) (*v13.ClusterRole, error) {
	clusterRoles, err := k.GetClusterRolesByLabel(label)
	if err != nil {
		return nil, err
	}

	if len(clusterRoles) == 1 {
		return &clusterRoles[0], nil
	} else if len(clusterRoles) > 1 {
		return nil, fmt.Errorf("multiple cluster roles have the label %s", label)
	} else {
		return nil, errors.NotFoundError("no cluster roles have the label %s", label)
	}
}

// GetClusterRoleByLabel returns all cluster role objects matching the specified label
func (k *KubeClient) GetClusterRolesByLabel(label string) ([]v13.ClusterRole, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	clusterRoleList, err := k.clientset.RbacV1().ClusterRoles().List(reqCtx(), listOptions)
	if err != nil {
		return nil, err
	}

	return clusterRoleList.Items, nil
}

// GetRolesByLabel returns all cluster role objects matching the specified label
func (k *KubeClient) GetRolesByLabel(label string) ([]v13.Role, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	roleList, err := k.clientset.RbacV1().Roles(k.Namespace()).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	return roleList.Items, nil
}

// CheckClusterRoleExistsByLabel returns true if one or more cluster role objects
// matching the specified label exist.
func (k *KubeClient) CheckClusterRoleExistsByLabel(label string) (bool, string, error) {
	clusterRoles, err := k.GetClusterRolesByLabel(label)
	if err != nil {
		return false, "", err
	}

	switch len(clusterRoles) {
	case 0:
		return false, "", nil
	case 1:
		return true, clusterRoles[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeleteClusterRoleByLabel deletes a cluster role object matching the specified label
// in the namespace of the client.
func (k *KubeClient) DeleteClusterRoleByLabel(label string) error {
	clusterRole, err := k.GetClusterRoleByLabel(label)
	if err != nil {
		return err
	}

	if err = k.clientset.RbacV1().ClusterRoles().Delete(reqCtx(), clusterRole.Name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":       label,
		"clusterRole": clusterRole.Name,
	}).Trace("Deleted Kubernetes cluster role.")

	return nil
}

// DeleteClusterRole deletes a cluster role object matching the specified name
func (k *KubeClient) DeleteClusterRole(name string) error {
	if err := k.clientset.RbacV1().ClusterRoles().Delete(reqCtx(), name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"clusterRole": name,
	}).Trace("Deleted Kubernetes cluster role.")

	return nil
}

// DeleteRole deletes a cluster role object matching the specified name
func (k *KubeClient) DeleteRole(name string) error {
	if err := k.clientset.RbacV1().Roles(k.Namespace()).Delete(ctx, name, k.deleteOptions()); err != nil {
		return err
	}

	Log().WithFields(LogFields{
		"role": name,
	}).Debug("Deleted Kubernetes role.")

	return nil
}

// PatchClusterRoleByLabelAndName patches a Cluster Role object matching the specified label
// and name in the namespace of the client.
func (k *KubeClient) PatchClusterRoleByLabelAndName(
	label, clusterRoleName string, patchBytes []byte,
	patchType types.PatchType,
) error {
	clusterRole, err := k.GetClusterRoleByLabelAndName(label, clusterRoleName)
	if err != nil {
		return err
	}

	if _, err = k.clientset.RbacV1().ClusterRoles().Patch(reqCtx(), clusterRole.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":       label,
		"clusterRole": clusterRole.Name,
	}).Trace("Patched Kubernetes cluster role.")

	return nil
}

// patchClusterRole patches an existing CRD to meet Trident's expected specification in the installer.
func (k *KubeClient) PatchClusterRole(
	newClusterRole, currentClusterRole *v13.ClusterRole,
) error {
	currentClusterRoleJson, err := json.Marshal(currentClusterRole)
	if err != nil {
		return fmt.Errorf("error marshalling cluster role %s: %v", currentClusterRole.Name, err)
	}
	newClusterRoleJson, err := json.Marshal(newClusterRole)
	if err != nil {
		return fmt.Errorf("error marshalling cluster role %s: %v", newClusterRole.Name, err)
	}

	// Generate the deltas between the currentCRD and the new CRD YAML using a json merge patch strategy.
	patchBytes, err := jsonMergePatch(currentClusterRoleJson, newClusterRoleJson)
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for cluster role %s; %v", newClusterRole.Name, err)
	}

	// Update the object
	if _, err := k.clientset.RbacV1().ClusterRoles().Patch(reqCtx(), newClusterRole.Name, types.MergePatchType,
		patchBytes, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("could not patch cluster role %v; %v", newClusterRole.Name, err)
	}

	Log().Debugf("Patched cluster role %v.", currentClusterRole.Name)
	return nil
}

// CreateOrPatchClusterRole Creates the clusterRole if it doesn't exist, and patches if it does exist.
func (k *KubeClient) CreateOrPatchClusterRole(
	clusterRole *v13.ClusterRole,
) error {
	// Get the CR if it exists
	existingCr, err := k.clientset.RbacV1().ClusterRoles().Get(reqCtx(), clusterRole.Name, getOpts)
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, err := k.clientset.RbacV1().ClusterRoles().Create(reqCtx(), clusterRole, createOpts)
			if err != nil {
				return fmt.Errorf("error creating cluster role %s: %v", clusterRole.Name, err)
			}
			Log().Debugf("Created cluster role %v.", clusterRole.Name)
			return nil
		} else {
			return fmt.Errorf("error getting cluster role %s: %v", clusterRole.Name, err)
		}
	} else {
		// CR exists, patch it
		err := k.PatchClusterRole(clusterRole, existingCr)
		if err != nil {
			return fmt.Errorf("error patching cluster role %s: %v", clusterRole.Name, err)
		}
		return nil
	}
}

// CreateOrPatchNodeRemediationTemplate Creates the TNRT if it doesn't exist, and patches if it does exist.
func (k *KubeClient) CreateOrPatchNodeRemediationTemplate(
	tnrt *tridentv1.TridentNodeRemediationTemplate, namespace string,
) error {
	// Get the CR if it exists
	existingCr, err := k.crdClientset.TridentV1().TridentNodeRemediationTemplates(namespace).Get(reqCtx(),
		tnrt.Name, getOpts)
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, err := k.crdClientset.TridentV1().TridentNodeRemediationTemplates(namespace).Create(reqCtx(), tnrt, createOpts)
			if err != nil {
				return fmt.Errorf("error TridentNodeRemediationTemplate %s: %v", tnrt.Name, err)
			}
			Log().Debugf("Created TridentNodeRemediationTemplate %v.", tnrt.Name)
			return nil
		} else {
			return fmt.Errorf("error getting TridentNodeRemediationTemplate %s: %v", tnrt.Name, err)
		}
	} else {
		// CR exists, patch it
		err := k.PatchNodeRemediationTemplate(tnrt, existingCr)
		if err != nil {
			return fmt.Errorf("error patching TridentNodeRemediationTemplate %s: %v", tnrt.Name, err)
		}
		return nil
	}
}

// patchClusterRole patches an existing CRD to meet Trident's expected specification in the installer.
func (k *KubeClient) PatchNodeRemediationTemplate(newTnrt *tridentv1.TridentNodeRemediationTemplate,
	currentTnrt *tridentv1.TridentNodeRemediationTemplate,
) error {
	currentTnrtJson, err := json.Marshal(currentTnrt)
	if err != nil {
		return fmt.Errorf("error marshalling TridentNodeRemediationTemplate %s: %v", currentTnrt.Name, err)
	}
	newTnrtJson, err := json.Marshal(newTnrt)
	if err != nil {
		return fmt.Errorf("error marshalling TridentNodeRemediationTemplate %s: %v", newTnrt.Name, err)
	}

	// Generate the deltas between the currentCR and the new CR YAML using a json merge patch strategy.
	patchBytes, err := jsonMergePatch(currentTnrtJson, newTnrtJson)
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for TridentNodeRemediationTemplate %s; %v",
			newTnrt.Name, err)
	}

	// Update the object
	if _, err := k.crdClientset.TridentV1().TridentNodeRemediationTemplates(newTnrt.Namespace).Patch(reqCtx(),
		newTnrt.Name, types.MergePatchType,
		patchBytes, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("could not patch TridentNodeRemediationTemplate %v; %v", newTnrt.Name, err)
	}

	Log().Debugf("Patched TridentNodeRemediationTemplate %v.", newTnrt.Name)
	return nil
}

// PatchRoleByLabelAndName patches a Role object matching the specified label
// and name in the namespace of the client.
func (k *KubeClient) PatchRoleByLabelAndName(
	label, roleName string, patchBytes []byte,
	patchType types.PatchType,
) error {
	role, err := k.GetRoleByLabelAndName(label, roleName)
	if err != nil {
		return err
	}

	if role == nil {
		return fmt.Errorf("role for name %s and label %s does not exist", roleName, label)
	}

	if _, err = k.clientset.RbacV1().Roles(k.Namespace()).Patch(ctx, role.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Log().WithFields(LogFields{
		"label": label,
		"role":  role.Name,
	}).Debug("Patched Kubernetes role.")

	return nil
}

// PatchClusterRoleByLabel patches a Cluster Role object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchClusterRoleByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	clusterRole, err := k.GetClusterRoleByLabel(label)
	if err != nil {
		return err
	}

	if _, err = k.clientset.RbacV1().ClusterRoles().Patch(reqCtx(), clusterRole.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":       label,
		"clusterRole": clusterRole.Name,
	}).Trace("Patched Kubernetes cluster role.")

	return nil
}

// GetClusterRoleBindingByLabelAndName returns a cluster role binding object matching the specified label and name
func (k *KubeClient) GetClusterRoleBindingByLabelAndName(label, clusterRoleBindingName string) (
	*v13.ClusterRoleBinding, error,
) {
	clusterRoleBindings, err := k.GetClusterRoleBindingsByLabel(label)
	if err != nil {
		return nil, err
	}

	var clusterRoleBinding *v13.ClusterRoleBinding

	for i := range clusterRoleBindings {
		if clusterRoleBindings[i].Name == clusterRoleBindingName {
			clusterRoleBinding = &clusterRoleBindings[i]
		}
	}
	return clusterRoleBinding, nil
}

// GetRoleBindingByLabelAndName returns a role binding object matching the specified label and name
func (k *KubeClient) GetRoleBindingByLabelAndName(label, roleBindingName string) (
	*v13.RoleBinding, error,
) {
	roleBindings, err := k.GetRoleBindingsByLabel(label)
	if err != nil {
		return nil, err
	}

	var roleBinding *v13.RoleBinding

	for i := range roleBindings {
		if roleBindings[i].Name == roleBindingName {
			roleBinding = &roleBindings[i]
		}
	}
	return roleBinding, nil
}

// GetClusterRoleBindingByLabel returns a cluster role binding object matching the specified label if it is unique
func (k *KubeClient) GetClusterRoleBindingByLabel(label string) (*v13.ClusterRoleBinding, error) {
	clusterRoleBindings, err := k.GetClusterRoleBindingsByLabel(label)
	if err != nil {
		return nil, err
	}

	if len(clusterRoleBindings) == 1 {
		return &clusterRoleBindings[0], nil
	} else if len(clusterRoleBindings) > 1 {
		return nil, fmt.Errorf("multiple cluster role bindings have the label %s", label)
	} else {
		return nil, errors.NotFoundError("no cluster role bindings have the label %s", label)
	}
}

// GetClusterRoleBindingsByLabel returns all cluster role binding objects matching the specified label
func (k *KubeClient) GetClusterRoleBindingsByLabel(label string) ([]v13.ClusterRoleBinding, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	clusterRoleBindingList, err := k.clientset.RbacV1().ClusterRoleBindings().List(reqCtx(), listOptions)
	if err != nil {
		return nil, err
	}

	return clusterRoleBindingList.Items, nil
}

// GetRoleBindingsByLabel returns all role binding objects matching the specified label
func (k *KubeClient) GetRoleBindingsByLabel(label string) ([]v13.RoleBinding, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	roleBindingList, err := k.clientset.RbacV1().RoleBindings(k.Namespace()).List(ctx, listOptions)
	if err != nil {
		return nil, err
	}

	return roleBindingList.Items, nil
}

// CheckClusterRoleBindingExistsByLabel returns true if one or more cluster role binding objects
// matching the specified label exist.
func (k *KubeClient) CheckClusterRoleBindingExistsByLabel(label string) (bool, string, error) {
	clusterRoleBindings, err := k.GetClusterRoleBindingsByLabel(label)
	if err != nil {
		return false, "", err
	}

	switch len(clusterRoleBindings) {
	case 0:
		return false, "", nil
	case 1:
		return true, clusterRoleBindings[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeleteClusterRoleBindingByLabel deletes a cluster role binding object matching the specified label
// in the namespace of the client.
func (k *KubeClient) DeleteClusterRoleBindingByLabel(label string) error {
	clusterRoleBinding, err := k.GetClusterRoleBindingByLabel(label)
	if err != nil {
		return err
	}

	err = k.clientset.RbacV1().ClusterRoleBindings().Delete(reqCtx(), clusterRoleBinding.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":              label,
		"clusterRoleBinding": clusterRoleBinding.Name,
	}).Trace("Deleted Kubernetes cluster role binding.")

	return nil
}

// DeleteClusterRoleBinding deletes a cluster role binding object matching the specified name
func (k *KubeClient) DeleteClusterRoleBinding(name string) error {
	if err := k.clientset.RbacV1().ClusterRoleBindings().Delete(reqCtx(), name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"clusterRoleBinding": name,
	}).Trace("Deleted Kubernetes cluster role binding.")

	return nil
}

// DeleteRoleBinding deletes a role binding object matching the specified name
func (k *KubeClient) DeleteRoleBinding(name string) error {
	if err := k.clientset.RbacV1().RoleBindings(k.Namespace()).Delete(ctx, name, k.deleteOptions()); err != nil {
		return err
	}

	Log().WithFields(LogFields{
		"roleBinding": name,
	}).Debug("Deleted Kubernetes role binding.")

	return nil
}

// PatchClusterRoleBindingByLabelAndName patches a Cluster Role binding object matching the specified label
// and name in the namespace of the client.
func (k *KubeClient) PatchClusterRoleBindingByLabelAndName(
	label, clusterRoleBindingName string, patchBytes []byte,
	patchType types.PatchType,
) error {
	clusterRoleBinding, err := k.GetClusterRoleBindingByLabelAndName(label, clusterRoleBindingName)
	if err != nil {
		return err
	}

	if _, err = k.clientset.RbacV1().ClusterRoleBindings().Patch(reqCtx(), clusterRoleBinding.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":              label,
		"clusterRoleBinding": clusterRoleBinding.Name,
	}).Trace("Patched Kubernetes cluster role binding.")

	return nil
}

// PatchRoleBindingByLabelAndName patches a Role binding object matching the specified label
// and name in the namespace of the client.
func (k *KubeClient) PatchRoleBindingByLabelAndName(
	label, roleBindingName string, patchBytes []byte,
	patchType types.PatchType,
) error {
	roleBinding, err := k.GetRoleBindingByLabelAndName(label, roleBindingName)
	if err != nil {
		return err
	}

	if roleBinding == nil {
		return fmt.Errorf("roleBinding for name %s and label %s does not exist", roleBindingName, label)
	}

	if _, err = k.clientset.RbacV1().RoleBindings(k.Namespace()).Patch(ctx, roleBinding.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Log().WithFields(LogFields{
		"label":       label,
		"roleBinding": roleBinding.Name,
	}).Debug("Patched Kubernetes role binding.")

	return nil
}

// PatchClusterRoleBindingByLabel patches a Cluster Role binding object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchClusterRoleBindingByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	clusterRoleBinding, err := k.GetClusterRoleBindingByLabel(label)
	if err != nil {
		return err
	}

	if _, err = k.clientset.RbacV1().ClusterRoleBindings().Patch(reqCtx(), clusterRoleBinding.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":              label,
		"clusterRoleBinding": clusterRoleBinding.Name,
	}).Trace("Patched Kubernetes cluster role binding.")

	return nil
}

// GetCSIDriverByLabel returns a CSI driver object matching the specified label if it is unique
func (k *KubeClient) GetCSIDriverByLabel(label string) (*storagev1.CSIDriver, error) {
	CSIDrivers, err := k.GetCSIDriversByLabel(label)
	if err != nil {
		return nil, err
	}

	if len(CSIDrivers) == 1 {
		return &CSIDrivers[0], nil
	} else if len(CSIDrivers) > 1 {
		return nil, fmt.Errorf("multiple CSI drivers have the label %s", label)
	} else {
		return nil, errors.NotFoundError("no CSI drivers have the label %s", label)
	}
}

// GetCSIDriversByLabel returns all CSI driver objects matching the specified label
func (k *KubeClient) GetCSIDriversByLabel(label string) ([]storagev1.CSIDriver, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	CSIDriverList, err := k.clientset.StorageV1().CSIDrivers().List(reqCtx(), listOptions)
	if err != nil {
		return nil, err
	}

	return CSIDriverList.Items, nil
}

// CheckCSIDriverExistsByLabel returns true if one or more CSI Driver objects
// matching the specified label exist.
func (k *KubeClient) CheckCSIDriverExistsByLabel(label string) (bool, string, error) {
	CSIDrivers, err := k.GetCSIDriversByLabel(label)
	if err != nil {
		return false, "", err
	}

	switch len(CSIDrivers) {
	case 0:
		return false, "", nil
	case 1:
		return true, CSIDrivers[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeleteCSIDriverByLabel deletes a CSI Driver object matching the specified label
// in the namespace of the client.
func (k *KubeClient) DeleteCSIDriverByLabel(label string) error {
	CSIDriver, err := k.GetCSIDriverByLabel(label)
	if err != nil {
		return err
	}

	if err = k.clientset.StorageV1().CSIDrivers().Delete(reqCtx(), CSIDriver.Name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":       label,
		"CSIDriverCR": CSIDriver.Name,
	}).Trace("Deleted CSI driver.")

	return nil
}

// DeleteCSIDriver deletes a CSI Driver object matching the specified name
func (k *KubeClient) DeleteCSIDriver(name string) error {
	if err := k.clientset.StorageV1().CSIDrivers().Delete(reqCtx(), name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"CSIDriverCR": name,
	}).Trace("Deleted CSI driver.")

	return nil
}

// PatchCSIDriverByLabel patches a deployment object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchCSIDriverByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	csiDriver, err := k.GetCSIDriverByLabel(label)
	if err != nil {
		return err
	}

	if _, err = k.clientset.StorageV1().CSIDrivers().Patch(reqCtx(), csiDriver.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":      label,
		"deployment": csiDriver.Name,
	}).Trace("Patched Kubernetes CSI driver.")

	return nil
}

func (k *KubeClient) CheckNamespaceExists(namespace string) (bool, error) {
	if _, err := k.GetNamespace(namespace); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// PatchNamespaceLabels patches the namespace with provided labels
func (k *KubeClient) PatchNamespaceLabels(namespace string, labels map[string]string) error {
	patch, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": labels,
		},
	})
	if err != nil {
		return err
	}
	return k.PatchNamespace(namespace, patch, types.MergePatchType)
}

// PatchNamespace patches the namespace with matching name
func (k *KubeClient) PatchNamespace(namespace string, patchBytes []byte, patchType types.PatchType) error {
	ns, err := k.GetNamespace(namespace)
	if err != nil {
		return err
	}

	if _, err := k.clientset.CoreV1().Namespaces().Patch(reqCtx(), ns.Name, patchType, patchBytes,
		patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"namespace": namespace,
	}).Trace("Patched Kubernetes namespace")

	return nil
}

func (k *KubeClient) GetNamespace(namespace string) (*v1.Namespace, error) {
	return k.clientset.CoreV1().Namespaces().Get(reqCtx(), namespace, getOpts)
}

// GetResourceQuota returns a ResourceQuota by name.
func (k *KubeClient) GetResourceQuota(name string) (*v1.ResourceQuota, error) {
	var options metav1.GetOptions
	return k.clientset.CoreV1().ResourceQuotas(k.namespace).Get(reqCtx(), name, options)
}

// GetResourceQuotaByLabel returns a ResourceQuota by label.
func (k *KubeClient) GetResourceQuotaByLabel(label string) (*v1.ResourceQuota, error) {
	resourceQuotas, err := k.GetResourceQuotasByLabel(label)
	if err != nil {
		return nil, err
	}

	if len(resourceQuotas) == 0 {
		return nil, errors.NotFoundError("no resource quotas have the label %s", label)
	} else if len(resourceQuotas) > 1 {
		return nil, fmt.Errorf("multiple resource quotas have the label %s", label)
	}

	Logc(ctx).WithFields(LogFields{
		"label":     label,
		"namespace": k.namespace,
	}).Trace("Found resource quota by label.")

	return &resourceQuotas[0], nil
}

// GetResourceQuotasByLabel returns all ResourceQuotas matching a given label.
func (k *KubeClient) GetResourceQuotasByLabel(label string) ([]v1.ResourceQuota, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	resourceQuotas, err := k.clientset.CoreV1().ResourceQuotas(k.namespace).List(reqCtx(), listOptions)
	if err != nil {
		return nil, err
	}

	return resourceQuotas.Items, nil
}

// DeleteResourceQuota deletes a ResourceQuota by name.
func (k *KubeClient) DeleteResourceQuota(name string) error {
	if err := k.clientset.CoreV1().ResourceQuotas(k.namespace).Delete(reqCtx(), name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"resourcequota": name,
		"namespace":     k.namespace,
	}).Trace("Deleted resource quota by name.")

	return nil
}

// DeleteResourceQuotaByLabel deletes a ResourceQuota by label.
func (k *KubeClient) DeleteResourceQuotaByLabel(label string) error {
	resourceQuota, err := k.GetResourceQuotaByLabel(label)
	if err != nil {
		return err
	}

	if err = k.clientset.CoreV1().ResourceQuotas(k.namespace).Delete(reqCtx(), resourceQuota.Name,
		k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":     label,
		"namespace": k.namespace,
	}).Trace("Deleted resource quota by label.")

	return nil
}

// PatchResourceQuotaByLabel patches a ResourceQuota object matching a given label in the client namespace.
func (k *KubeClient) PatchResourceQuotaByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	resourceQuota, err := k.GetResourceQuotaByLabel(label)
	if err != nil {
		return err
	}

	if _, err = k.clientset.CoreV1().ResourceQuotas(k.namespace).Patch(reqCtx(), resourceQuota.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":         label,
		"resourcequota": resourceQuota.Name,
		"namespace":     k.namespace,
	}).Trace("Patched Trident resource quota.")

	return nil
}

// CreateSecret creates a new Secret
func (k *KubeClient) CreateSecret(secret *v1.Secret) (*v1.Secret, error) {
	return k.clientset.CoreV1().Secrets(k.namespace).Create(reqCtx(), secret, createOpts)
}

// UpdateSecret updates an existing Secret
func (k *KubeClient) UpdateSecret(secret *v1.Secret) (*v1.Secret, error) {
	return k.clientset.CoreV1().Secrets(k.namespace).Update(reqCtx(), secret, updateOpts)
}

// GetSecret looks up a Secret by name
func (k *KubeClient) GetSecret(secretName string) (*v1.Secret, error) {
	var options metav1.GetOptions
	return k.clientset.CoreV1().Secrets(k.namespace).Get(reqCtx(), secretName, options)
}

// GetSecretByLabel looks up a Secret by label
func (k *KubeClient) GetSecretByLabel(label string, allNamespaces bool) (*v1.Secret, error) {
	secrets, err := k.GetSecretsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(secrets) == 1 {
		return &secrets[0], nil
	} else if len(secrets) > 1 {
		return nil, fmt.Errorf("multiple secrets have the label %s", label)
	} else {
		return nil, fmt.Errorf("no secrets have the label %s", label)
	}
}

// GetSecretsByLabel returns all secret object matching specified label
func (k *KubeClient) GetSecretsByLabel(label string, allNamespaces bool) ([]v1.Secret, error) {
	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	namespace := k.namespace
	if allNamespaces {
		namespace = ""
	}

	secretList, err := k.clientset.CoreV1().Secrets(namespace).List(reqCtx(), listOptions)
	if err != nil {
		return nil, err
	}

	return secretList.Items, nil
}

// CheckSecretExists returns true if the Secret exists
func (k *KubeClient) CheckSecretExists(secretName string) (bool, error) {
	if _, err := k.GetSecret(secretName); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DeleteSecret deletes the specified Secret
func (k *KubeClient) DeleteSecretDefault(secretName string) error {
	return k.DeleteSecret(secretName, k.namespace)
}

// DeleteSecretByLabel deletes a secret object matching the specified label
func (k *KubeClient) DeleteSecretByLabel(label string) error {
	secret, err := k.GetSecretByLabel(label, false)
	if err != nil {
		return err
	}

	if err = k.clientset.CoreV1().Secrets(k.namespace).Delete(reqCtx(), secret.Name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":     label,
		"namespace": k.namespace,
	}).Trace("Deleted secret by label.")

	return nil
}

// DeleteSecret deletes the specified Secret by name and namespace
func (k *KubeClient) DeleteSecret(name, namespace string) error {
	if err := k.clientset.CoreV1().Secrets(namespace).Delete(reqCtx(), name, k.deleteOptions()); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"name":      name,
		"namespace": namespace,
	}).Trace("Deleted secret by label.")

	return nil
}

// PatchSecretByLabel patches a pod security policy object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchSecretByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	secret, err := k.GetSecretByLabel(label, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.CoreV1().Secrets(k.namespace).Patch(reqCtx(), secret.Name,
		patchType, patchBytes, patchOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"label":      label,
		"deployment": secret.Name,
		"namespace":  k.namespace,
	}).Trace("Patched Kubernetes secret.")

	return nil
}

// CreateObjectByFile creates one or more objects on the server from a YAML/JSON file at the specified path.
func (k *KubeClient) CreateObjectByFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	return k.CreateObjectByYAML(string(content))
}

// CreateObjectByYAML creates one or more objects on the server from a YAML/JSON document.
func (k *KubeClient) CreateObjectByYAML(yamlData string) error {
	for _, yamlDocument := range regexp.MustCompile(YAMLSeparator).Split(yamlData, -1) {

		checkCreateObjectByYAML := func() error {
			if returnError := k.createObjectByYAML(yamlDocument); returnError != nil {
				Logc(ctx).WithFields(LogFields{
					"yamlDocument": yamlDocument,
					"err":          returnError,
				}).Errorf("Object creation failed.")
				return returnError
			}
			return nil
		}

		createObjectNotify := func(err error, duration time.Duration) {
			Logc(ctx).WithFields(LogFields{
				"yamlDocument": yamlDocument,
				"increment":    duration,
				"err":          err,
			}).Debugf("Object not created, waiting.")
		}
		createObjectBackoff := backoff.NewExponentialBackOff()
		createObjectBackoff.MaxElapsedTime = k.timeout

		Logc(ctx).WithField("yamlDocument", yamlDocument).Trace("Waiting for object to be created.")

		if err := backoff.RetryNotify(checkCreateObjectByYAML, createObjectBackoff, createObjectNotify); err != nil {
			returnError := fmt.Errorf("yamlDocument %s was not created after %3.2f seconds",
				yamlDocument, k.timeout.Seconds())
			return returnError
		}
	}
	return nil
}

// createObjectByYAML creates an object on the server from a YAML/JSON document.
func (k *KubeClient) createObjectByYAML(yamlData string) error {
	// Parse the data
	unstruct, gvk, err := k.convertYAMLToUnstructuredObject(yamlData)
	if err != nil {
		return err
	}

	// Get a matching API resource
	gvr, namespaced, err := k.getDynamicResource(gvk)
	if err != nil {
		return err
	}

	// Get a dynamic REST client
	client, err := dynamic.NewForConfig(k.restConfig)
	if err != nil {
		return err
	}

	// Read the namespace if available in the object, otherwise use the client's namespace
	objNamespace := k.namespace
	objName := ""
	if md, ok := unstruct.Object["metadata"]; ok {
		if metadata, ok := md.(map[string]interface{}); ok {
			if ns, ok := metadata["namespace"]; ok {
				if objNamespace, ok = ns.(string); !ok {
					return errors.TypeAssertionError("ns.(string)")
				}
			}
			if name, ok := metadata["name"]; ok {
				if objName, ok = name.(string); !ok {
					return errors.TypeAssertionError("name.(string)")
				}
			}
		}
	}

	Logc(ctx).WithFields(LogFields{
		"name":      objName,
		"namespace": objNamespace,
		"kind":      gvk.Kind,
	}).Debugf("Creating object.")

	// Create the object
	if namespaced {
		if _, err = client.Resource(*gvr).Namespace(objNamespace).Create(reqCtx(), unstruct, createOpts); err != nil {
			return err
		}
	} else {
		if _, err = client.Resource(*gvr).Create(reqCtx(), unstruct, createOpts); err != nil {
			return err
		}
	}

	Logc(ctx).WithField("name", objName).Debug("Created object by YAML.")
	return nil
}

// PatchCRD patches a CRD with a supplied YAML/JSON document in []byte format.
func (k *KubeClient) PatchCRD(crdName string, patchBytes []byte, patchType types.PatchType) error {
	Logc(ctx).WithField("name", crdName).Debug("Patching CRD.")

	// Update the object
	patchOptions := metav1.PatchOptions{}
	if _, err := k.extClientset.ApiextensionsV1().CustomResourceDefinitions().Patch(reqCtx(), crdName, patchType,
		patchBytes, patchOptions); err != nil {
		return err
	}

	Logc(ctx).WithField("name", crdName).Debug("Patched CRD.")

	return nil
}

// DeleteObjectByFile deletes one or more objects on the server from a YAML/JSON file at the specified path.
func (k *KubeClient) DeleteObjectByFile(filePath string, ignoreNotFound bool) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	return k.DeleteObjectByYAML(string(content), ignoreNotFound)
}

// DeleteObjectByYAML deletes one or more objects on the server from a YAML/JSON document.
func (k *KubeClient) DeleteObjectByYAML(yamlData string, ignoreNotFound bool) error {
	for _, yamlDocument := range regexp.MustCompile(YAMLSeparator).Split(yamlData, -1) {
		if err := k.deleteObjectByYAML(yamlDocument, ignoreNotFound); err != nil {
			return err
		}
	}
	return nil
}

// DeleteObjectByYAML deletes an object on the server from a YAML/JSON document.
func (k *KubeClient) deleteObjectByYAML(yamlData string, ignoreNotFound bool) error {
	// Parse the data
	unstruct, gvk, err := k.convertYAMLToUnstructuredObject(yamlData)
	if err != nil {
		return err
	}

	// Get a matching API resource
	gvr, namespaced, err := k.getDynamicResource(gvk)
	if err != nil {
		return err
	}

	// Get a dynamic REST client
	client, err := dynamic.NewForConfig(k.restConfig)
	if err != nil {
		return err
	}

	// Read the namespace and name from the object
	objNamespace := k.namespace
	objName := ""
	if md, ok := unstruct.Object["metadata"]; ok {
		if metadata, ok := md.(map[string]interface{}); ok {
			if ns, ok := metadata["namespace"]; ok {
				if objNamespace, ok = ns.(string); !ok {
					return errors.TypeAssertionError("ns.(string)")
				}
			}
			if name, ok := metadata["name"]; ok {
				if objName, ok = name.(string); !ok {
					return errors.TypeAssertionError("name.(string)")
				}
			}
		}
	}

	// We must have the object name to delete it
	if objName == "" {
		return errors.New("cannot determine object name from YAML")
	}

	Logc(ctx).WithFields(LogFields{
		"name":      objName,
		"namespace": objNamespace,
		"kind":      gvk.Kind,
	}).Trace("Deleting object.")

	// Delete the object
	if namespaced {
		err = client.Resource(*gvr).Namespace(objNamespace).Delete(reqCtx(), objName, k.deleteOptions())
	} else {
		err = client.Resource(*gvr).Delete(reqCtx(), objName, k.deleteOptions())
	}

	if err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok {
			if ignoreNotFound && statusErr.Status().Reason == metav1.StatusReasonNotFound {
				Logc(ctx).WithField("name", objName).Debugf("Object not found for delete, ignoring.")
				return nil
			}
		}
		return err
	}

	Logc(ctx).WithField("name", objName).Debug("Deleted object by YAML.")
	return nil
}

// getUnstructuredObjectByYAML uses the dynamic client-go API to get an unstructured object from a YAML/JSON document.
func (k *KubeClient) getUnstructuredObjectByYAML(yamlData string) (*unstructured.Unstructured, error) {
	// Parse the data
	unstruct, gvk, err := k.convertYAMLToUnstructuredObject(yamlData)
	if err != nil {
		return nil, err
	}

	// Get a matching API resource
	gvr, namespaced, err := k.getDynamicResource(gvk)
	if err != nil {
		return nil, err
	}

	// Get a dynamic REST client
	client, err := dynamic.NewForConfig(k.restConfig)
	if err != nil {
		return nil, err
	}

	// Read the namespace if available in the object, otherwise use the client's namespace
	objNamespace := k.namespace
	objName := ""
	if md, ok := unstruct.Object["metadata"]; ok {
		if metadata, ok := md.(map[string]interface{}); ok {
			if ns, ok := metadata["namespace"]; ok {
				if objNamespace, ok = ns.(string); !ok {
					return nil, errors.TypeAssertionError("ns.(string)")
				}
			}
			if name, ok := metadata["name"]; ok {
				if objName, ok = name.(string); !ok {
					return nil, errors.TypeAssertionError("name.(string)")
				}
			}
		}
	}

	Logc(ctx).WithFields(LogFields{
		"name":      objName,
		"namespace": objNamespace,
		"kind":      gvk.Kind,
	}).Debug("Getting object.")

	// Get the object
	if namespaced {
		return client.Resource(*gvr).Namespace(objNamespace).Get(reqCtx(), objName, getOpts)
	} else {
		return client.Resource(*gvr).Get(reqCtx(), objName, getOpts)
	}
}

// updateObjectByYAML uses the dynamic client-go API to update an object on the server using a YAML/JSON document.
func (k *KubeClient) updateObjectByYAML(yamlData string) error {
	// Parse the data
	unstruct, gvk, err := k.convertYAMLToUnstructuredObject(yamlData)
	if err != nil {
		return err
	}

	// Get a matching API resource
	gvr, namespaced, err := k.getDynamicResource(gvk)
	if err != nil {
		return err
	}

	// Get a dynamic REST client
	client, err := dynamic.NewForConfig(k.restConfig)
	if err != nil {
		return err
	}

	// Read the namespace if available in the object, otherwise use the client's namespace
	objNamespace := k.namespace
	objName := ""
	if md, ok := unstruct.Object["metadata"]; ok {
		if metadata, ok := md.(map[string]interface{}); ok {
			if ns, ok := metadata["namespace"]; ok {
				if objNamespace, ok = ns.(string); !ok {
					return errors.TypeAssertionError("ns.(string)")
				}
			}
			if name, ok := metadata["name"]; ok {
				if objName, ok = name.(string); !ok {
					return errors.TypeAssertionError("name.(string)")
				}
			}
		}
	}

	Logc(ctx).WithFields(LogFields{
		"name":      objName,
		"namespace": objNamespace,
		"kind":      gvk.Kind,
	}).Debug("Updating object.")

	// Update the object
	updateOptions := updateOpts

	if namespaced {
		if _, err = client.Resource(*gvr).Namespace(objNamespace).Update(reqCtx(), unstruct,
			updateOptions); err != nil {
			return err
		}
	} else {
		if _, err = client.Resource(*gvr).Update(reqCtx(), unstruct, updateOptions); err != nil {
			return err
		}
	}

	Logc(ctx).WithField("name", objName).Debug("Updated object by YAML.")
	return nil
}

// convertYAMLToUnstructuredObject parses a YAML/JSON document into an unstructured object as used by the
// dynamic client-go API.
func (k *KubeClient) convertYAMLToUnstructuredObject(yamlData string) (
	*unstructured.Unstructured, *schema.GroupVersionKind, error,
) {
	// Ensure the data is valid JSON/YAML, and convert to JSON
	jsonData, err := yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		return nil, nil, err
	}

	// Decode the JSON so we can get its group/version/kind info
	_, gvk, err := unstructured.UnstructuredJSONScheme.Decode(jsonData, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	// Read the JSON data into an unstructured map
	var unstruct unstructured.Unstructured
	unstruct.Object = make(map[string]interface{})
	if err := unstruct.UnmarshalJSON(jsonData); err != nil {
		return nil, nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"group":   gvk.Group,
		"version": gvk.Version,
		"kind":    gvk.Kind,
	}).Debugf("Parsed YAML into unstructured object.")

	return &unstruct, gvk, nil
}

// getDynamicResource accepts a GVK value and returns a matching Resource from the dynamic client-go API.  This
// method will first consult this client's internal cache and will update the cache only if the resource being
// sought is not present.
func (k *KubeClient) getDynamicResource(gvk *schema.GroupVersionKind) (*schema.GroupVersionResource, bool, error) {
	if gvr, namespaced, err := k.getDynamicResourceNoRefresh(gvk); errors.IsNotFoundError(err) {

		discoveryClient := k.clientset.Discovery()

		// The resource wasn't in the cache, so try getting just the one we need.
		apiResourceList, err := discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"group":   gvk.Group,
				"version": gvk.Version,
				"kind":    gvk.Kind,
				"error":   err,
			}).Error("Could not get dynamic resource, failed to list server resources.")
			return nil, false, err
		}

		// Update the cache
		k.apiResources[apiResourceList.GroupVersion] = apiResourceList

		return k.getDynamicResourceFromResourceList(gvk, apiResourceList)

	} else {
		// Success the first time, or some other error
		return gvr, namespaced, err
	}
}

// getDynamicResourceNoRefresh accepts a GVK value and returns a matching Resource from the list of API resources
// cached here in the client.  Most callers should use getDynamicResource() instead, which will update the cached
// API list if it doesn't already contain the resource being sought.
func (k *KubeClient) getDynamicResourceNoRefresh(
	gvk *schema.GroupVersionKind,
) (*schema.GroupVersionResource, bool, error) {
	if apiResourceList, ok := k.apiResources[gvk.GroupVersion().String()]; ok {
		if gvr, namespaced, err := k.getDynamicResourceFromResourceList(gvk, apiResourceList); err == nil {
			return gvr, namespaced, nil
		}
	}

	Logc(ctx).WithFields(LogFields{
		"group":   gvk.Group,
		"version": gvk.Version,
		"kind":    gvk.Kind,
	}).Trace("API resource not found.")

	return nil, false, errors.NotFoundError("API resource not found")
}

// getDynamicResourceFromResourceList accepts an APIResourceList array as returned from the K8S API server
// and returns a GroupVersionResource matching the supplied GroupVersionKind if and only if the GVK is
// represented in the resource array.  For example:
//
// GroupVersion            Resource     Kind
// -----------------------------------------------
// apps/v1                 deployments  Deployment
// apps/v1                 daemonsets   DaemonSet
// events.k8s.io/v1beta1   events       Event
func (k *KubeClient) getDynamicResourceFromResourceList(
	gvk *schema.GroupVersionKind, resources *metav1.APIResourceList,
) (*schema.GroupVersionResource, bool, error) {
	groupVersion, err := schema.ParseGroupVersion(resources.GroupVersion)
	if err != nil {
		Logc(ctx).WithField("groupVersion", groupVersion).Errorf("Could not parse group/version; %v", err)
		return nil, false, errors.NotFoundError("Could not parse group/version")
	}

	if groupVersion.Group != gvk.Group || groupVersion.Version != gvk.Version {
		return nil, false, errors.NotFoundError("API resource not found, group/version mismatch")
	}

	for _, resource := range resources.APIResources {
		Logc(ctx).WithFields(LogFields{
			"group":    groupVersion.Group,
			"version":  groupVersion.Version,
			"kind":     resource.Kind,
			"resource": resource.Name,
		}).Trace("Considering dynamic API resource.")

		if resource.Kind == gvk.Kind {

			Logc(ctx).WithFields(LogFields{
				"group":    groupVersion.Group,
				"version":  groupVersion.Version,
				"kind":     resource.Kind,
				"resource": resource.Name,
			}).Trace("Found API resource.")

			return &schema.GroupVersionResource{
				Group:    groupVersion.Group,
				Version:  groupVersion.Version,
				Resource: resource.Name,
			}, resource.Namespaced, nil
		}
	}

	return nil, false, errors.NotFoundError("API resource not found")
}

// RemoveTridentUserFromOpenShiftSCC removes the specified user (typically a service account) from the 'anyuid'
// security context constraint. This only works for OpenShift, and it must be idempotent.
func (k *KubeClient) RemoveTridentUserFromOpenShiftSCC(user, scc string) error {
	sccUser := fmt.Sprintf("system:serviceaccount:%s:%s", k.namespace, user)

	// Get the YAML into a form we can modify
	openShiftSCCQueryYAML := GetOpenShiftSCCQueryYAML(scc)
	unstruct, err := k.getUnstructuredObjectByYAML(openShiftSCCQueryYAML)
	if err != nil {
		return err
	}

	// Remove the user
	found := false
	if users, ok := unstruct.Object["users"]; ok {
		if usersSlice, ok := users.([]interface{}); ok {
			for index, userIntf := range usersSlice {
				if user, ok := userIntf.(string); ok && user == sccUser {
					unstruct.Object["users"] = append(usersSlice[:index], usersSlice[index+1:]...)
					found = true
					break
				}
			}
		} else {
			return fmt.Errorf("users type is %T", users)
		}
	}

	// Maintain idempotency by returning success if user not found
	if !found {
		Logc(ctx).WithField("user", sccUser).Debug("SCC user not found, ignoring.")
		return nil
	}

	// Convert to JSON
	jsonData, err := unstruct.MarshalJSON()
	if err != nil {
		return err
	}

	// Update the object on the server
	return k.updateObjectByYAML(string(jsonData))
}

// GetOpenShiftSCCByName gets the specified user (typically a service account) from the specified
// security context constraint. This only works for OpenShift, and it must be idempotent.
func (k *KubeClient) GetOpenShiftSCCByName(user, scc string) (bool, bool, []byte, error) {
	var SCCExist, SCCUserExist bool
	sccUser := fmt.Sprintf("system:serviceaccount:%s:%s", k.namespace, user)

	// Get the YAML into a form we can read/modify
	openShiftSCCQueryYAML := GetOpenShiftSCCQueryYAML(scc)
	unstruct, err := k.getUnstructuredObjectByYAML(openShiftSCCQueryYAML)
	if err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return SCCExist, SCCUserExist, nil, nil
		}
		return SCCExist, SCCUserExist, nil, err
	}

	Logc(ctx).WithField("scc", scc).Debug("SCC found.")
	SCCExist = true

	// Convert to JSON
	jsonData, err := unstruct.MarshalJSON()
	if err != nil {
		Logc(ctx).WithField("scc", scc).Errorf("failed to marshal unstructured data in JSON: %v", unstruct)
		return SCCExist, SCCUserExist, jsonData, err
	}

	// Find the user
	if users, ok := unstruct.Object["users"]; ok {
		if usersSlice, ok := users.([]interface{}); ok {
			for _, userIntf := range usersSlice {
				if user, ok := userIntf.(string); ok && user == sccUser {
					Logc(ctx).WithField("sccUser", sccUser).Debug("SCC User found.")
					SCCUserExist = true
					break
				}
			}
		} else {
			return SCCExist, SCCUserExist, jsonData, fmt.Errorf("users type is %T", users)
		}
	}

	return SCCExist, SCCUserExist, jsonData, nil
}

// PatchOpenShiftSCC patches the specified user (typically a service account) from the specified
// security context constraint. This only works for OpenShift, and it must be idempotent.
func (k *KubeClient) PatchOpenShiftSCC(newJSONData []byte) error {
	// Update the object on the server
	return k.updateObjectByYAML(string(newJSONData))
}

// listOptionsFromLabel accepts a label in the form "key=value" and returns a ListOptions value
// suitable for passing to the K8S API.
func (k *KubeClient) listOptionsFromLabel(label string) (metav1.ListOptions, error) {
	selector, err := k.getSelectorFromLabel(label)
	if err != nil {
		return listOpts, err
	}

	return metav1.ListOptions{LabelSelector: selector}, nil
}

// getSelectorFromLabel accepts a label in the form "key=value" and returns a string in the
// correct form to pass to the K8S API as a LabelSelector.
func (k *KubeClient) getSelectorFromLabel(label string) (string, error) {
	selectorSet := make(labels.Set)

	if label != "" {
		labelParts := strings.Split(label, "=")
		if len(labelParts) != 2 {
			return "", fmt.Errorf("invalid label: %s", label)
		}
		selectorSet[labelParts[0]] = labelParts[1]
	}

	return selectorSet.String(), nil
}

// deleteOptions returns a DeleteOptions struct suitable for most DELETE calls to the K8S REST API.
func (k *KubeClient) deleteOptions() metav1.DeleteOptions {
	propagationPolicy := metav1.DeletePropagationBackground
	return metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}
}

// addFinalizerToCRDObject is a helper function that updates the CRD object to include our Trident finalizer (
// definitions are not namespaced)
func (k *KubeClient) addFinalizerToCRDObject(
	crdName string, gvk *schema.GroupVersionKind,
	gvr *schema.GroupVersionResource, client dynamic.Interface,
) error {
	var err error

	Logc(ctx).WithFields(LogFields{
		"CRD":  crdName,
		"kind": gvk.Kind,
	}).Debugf("Adding finalizers to CRD.")

	// Get the CRD object
	var unstruct *unstructured.Unstructured
	if unstruct, err = client.Resource(*gvr).Get(reqCtx(), crdName, getOpts); err != nil {
		return err
	}

	// Check the CRD object
	addedFinalizer := false
	if md, ok := unstruct.Object["metadata"]; ok {
		if metadata, ok := md.(map[string]interface{}); ok {
			if finalizers, ok := metadata["finalizers"]; ok {
				if finalizersSlice, ok := finalizers.([]interface{}); ok {
					// check if the Trident finalizer is already present
					for _, element := range finalizersSlice {
						if finalizer, ok := element.(string); ok && finalizer == TridentFinalizer {
							Logc(ctx).WithField("CRD", crdName).Debugf("Trident finalizer already present " +
								"on Kubernetes CRD object, nothing to do.")
							return nil
						}
					}
					// checked the list and didn't find it, let's add it to the list
					metadata["finalizers"] = append(finalizersSlice, TridentFinalizer)
					addedFinalizer = true
				} else {
					return fmt.Errorf("unexpected finalizer type %T", finalizers)
				}
			} else {
				// no finalizers present, this will be the first one for this CRD
				metadata["finalizers"] = []string{TridentFinalizer}
				addedFinalizer = true
			}
		}
	}
	if !addedFinalizer {
		return fmt.Errorf("could not determine finalizer metadata for CRD %v", crdName)
	}

	// Update the object with the newly added finalizer
	updateOptions := updateOpts

	if _, err = client.Resource(*gvr).Update(reqCtx(), unstruct, updateOptions); err != nil {
		return err
	}

	Logc(ctx).Debugf("Added finalizers to Kubernetes CRD object %v", crdName)

	return nil
}

// AddFinalizerToCRDs updates the CRD objects to include our Trident finalizer (definitions are not namespaced)
func (k *KubeClient) AddFinalizerToCRDs(CRDnames []string) error {
	gvk := &schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	}

	// Get a matching API resource
	gvr, _, err := k.getDynamicResource(gvk)
	if err != nil {
		return err
	}

	// Get a dynamic REST client
	client, err := dynamic.NewForConfig(k.restConfig)
	if err != nil {
		return err
	}

	for _, crdName := range CRDnames {
		err := k.addFinalizerToCRDObject(crdName, gvk, gvr, client)
		if err != nil {
			Logc(ctx).Errorf("error in adding finalizers to Kubernetes CRD object %v", crdName)
			return err
		}
	}

	return nil
}

// AddFinalizerToCRD updates the CRD object to include our Trident finalizer (definitions are not namespaced)
func (k *KubeClient) AddFinalizerToCRD(crdName string) error {
	/*
	   $ kubectl api-resources -o wide | grep -i crd
	    NAME                              SHORTNAMES          APIGROUP                       NAMESPACED   KIND                             VERBS
	   customresourcedefinitions         crd,crds            apiextensions.k8s.io           false        CustomResourceDefinition         [create delete deletecollection get list patch update watch]

	   $ kubectl  api-versions | grep apiextensions
	   apiextensions.k8s.io/v1
	*/
	gvk := &schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	}
	// Get a matching API resource
	gvr, _, err := k.getDynamicResource(gvk)
	if err != nil {
		return err
	}

	// Get a dynamic REST client
	client, err := dynamic.NewForConfig(k.restConfig)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"CRD":  crdName,
		"kind": gvk.Kind,
	}).Debugf("Adding finalizers to CRD.")

	// Get the CRD object
	var unstruct *unstructured.Unstructured
	if unstruct, err = client.Resource(*gvr).Get(reqCtx(), crdName, getOpts); err != nil {
		return err
	}

	// Check the CRD object
	addedFinalizer := false
	if md, ok := unstruct.Object["metadata"]; ok {
		if metadata, ok := md.(map[string]interface{}); ok {
			if finalizers, ok := metadata["finalizers"]; ok {
				if finalizersSlice, ok := finalizers.([]interface{}); ok {
					// check if the Trident finalizer is already present
					for _, element := range finalizersSlice {
						if finalizer, ok := element.(string); ok && finalizer == TridentFinalizer {
							Logc(ctx).WithField("CRD", crdName).Debugf("Trident finalizer already present " +
								"on Kubernetes CRD object, nothing to do.")
							return nil
						}
					}
					// checked the list and didn't find it, let's add it to the list
					metadata["finalizers"] = append(finalizersSlice, TridentFinalizer)
					addedFinalizer = true
				} else {
					return fmt.Errorf("unexpected finalizer type %T", finalizers)
				}
			} else {
				// no finalizers present, this will be the first one for this CRD
				metadata["finalizers"] = []string{TridentFinalizer}
				addedFinalizer = true
			}
		}
	}
	if !addedFinalizer {
		return fmt.Errorf("could not determine finalizer metadata for CRD %v", crdName)
	}

	// Update the object with the newly added finalizer
	if _, err = client.Resource(*gvr).Update(reqCtx(), unstruct, updateOpts); err != nil {
		return err
	}

	Logc(ctx).Debugf("Added finalizers to Kubernetes CRD object %v", crdName)

	return nil
}

// RemoveFinalizerFromCRD updates the CRD object to remove all finalizers (definitions are not namespaced)
func (k *KubeClient) RemoveFinalizerFromCRD(crdName string) error {
	/*
	   $ kubectl api-resources -o wide | grep -i crd
	    NAME                              SHORTNAMES          APIGROUP                       NAMESPACED   KIND                             VERBS
	   customresourcedefinitions         crd,crds            apiextensions.k8s.io           false        CustomResourceDefinition         [create delete deletecollection get list patch update watch]

	   $ kubectl  api-versions | grep apiextensions
	   apiextensions.k8s.io/v1
	*/
	gvk := &schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	}
	// Get a matching API resource
	gvr, _, err := k.getDynamicResource(gvk)
	if err != nil {
		return err
	}

	// Get a dynamic REST client
	client, err := dynamic.NewForConfig(k.restConfig)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"name": crdName,
		"kind": gvk.Kind,
	}).Debugf("Removing finalizers from CRD object.")

	// Get the CRD object
	var unstruct *unstructured.Unstructured
	if unstruct, err = client.Resource(*gvr).Get(reqCtx(), crdName, getOpts); err != nil {
		return err
	}

	// Check the CRD object
	removedFinalizer := false
	if md, ok := unstruct.Object["metadata"]; ok {
		if metadata, ok := md.(map[string]interface{}); ok {
			if finalizers, ok := metadata["finalizers"]; ok {
				if finalizersSlice, ok := finalizers.([]interface{}); ok {
					// Check if the Trident finalizer is already present
					for _, element := range finalizersSlice {
						if _, ok := element.(string); ok {
							removedFinalizer = true
						}
					}
					if !removedFinalizer {
						Logc(ctx).Debugf("Finalizer not present on Kubernetes CRD object %v, nothing to do", crdName)
						return nil
					}
					// Found one or more finalizers, remove them all
					metadata["finalizers"] = make([]string, 0)
				} else {
					return fmt.Errorf("unexpected finalizer type %T", finalizers)
				}
			} else {
				// No finalizers present, nothing to do
				Logc(ctx).Debugf("No finalizer found on Kubernetes CRD object %v, nothing to do", crdName)
				return nil
			}
		}
	}
	if !removedFinalizer {
		return fmt.Errorf("could not determine finalizer metadata for CRD %v", crdName)
	}

	// Update the object with the updated finalizers
	if _, err = client.Resource(*gvr).Update(reqCtx(), unstruct, updateOpts); err != nil {
		return err
	}

	Logc(ctx).Debugf("Removed finalizers from Kubernetes CRD object %v", crdName)

	return nil
}

// GetPersistentVolumes returns all PersistentVolumes.
func (k *KubeClient) GetPersistentVolumes() ([]v1.PersistentVolume, error) {
	persistentVolumeList, err := k.clientset.CoreV1().PersistentVolumes().List(reqCtx(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return persistentVolumeList.Items, nil
}

// GetPersistentVolumeClaims returns all PersistentVolumeClaims.
func (k *KubeClient) GetPersistentVolumeClaims(allNamespaces bool) ([]v1.PersistentVolumeClaim, error) {
	namespace := k.namespace
	if allNamespaces {
		namespace = ""
	}

	persistentVolumeClaimsList, err := k.clientset.CoreV1().PersistentVolumeClaims(namespace).List(reqCtx(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return persistentVolumeClaimsList.Items, nil
}

// GetVolumeSnapshotClasses returns all VolumeSnapshotClasses.
func (k *KubeClient) GetVolumeSnapshotClasses() ([]snapshotv1.VolumeSnapshotClass, error) {
	// Get a snapshot client
	snapshotClient, err := k8ssnapshots.NewForConfig(k.restConfig)
	if err != nil {
		return nil, err
	}

	volumeSnapshotClassesList, err := snapshotClient.SnapshotV1().VolumeSnapshotClasses().List(reqCtx(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return volumeSnapshotClassesList.Items, nil
}

// GetVolumeSnapshotContents returns all VolumeSnapshotContents.
func (k *KubeClient) GetVolumeSnapshotContents() ([]snapshotv1.VolumeSnapshotContent, error) {
	// Get a snapshot client
	snapshotClient, err := k8ssnapshots.NewForConfig(k.restConfig)
	if err != nil {
		return nil, err
	}

	volumeSnapshotContentsList, err := snapshotClient.SnapshotV1().VolumeSnapshotContents().List(reqCtx(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return volumeSnapshotContentsList.Items, nil
}

// GetVolumeSnapshots returns all VolumeSnapshots.
func (k *KubeClient) GetVolumeSnapshots(allNamespaces bool) ([]snapshotv1.VolumeSnapshot, error) {
	// Get a snapshot client
	snapshotClient, err := k8ssnapshots.NewForConfig(k.restConfig)
	if err != nil {
		return nil, err
	}

	namespace := k.namespace
	if allNamespaces {
		namespace = ""
	}

	volumeSnapshotsList, err := snapshotClient.SnapshotV1().VolumeSnapshots(namespace).List(reqCtx(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return volumeSnapshotsList.Items, nil
}

// GenericPatch merges an object with a new YAML definition.
func GenericPatch(original interface{}, modifiedYAML []byte) ([]byte, error) {
	// Get existing object in JSON format
	originalJSON, err := jsonMarshal(original)
	if err != nil {
		return nil, fmt.Errorf("error in marshaling current object; %v", err)
	}

	// Convert new object from YAML to JSON format
	modifiedJSON, err := yamlToJSON(modifiedYAML)
	if err != nil {
		return nil, fmt.Errorf("could not convert new object from YAML to JSON; %v", err)
	}

	// JSON Merge patch
	return jsonMergePatch(originalJSON, modifiedJSON)
}
