// Copyright 2018 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	v13 "k8s.io/api/rbac/v1"
	v1beta12 "k8s.io/api/storage/v1beta1"
	apiextensionv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	commontypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils"
)

type OrchestratorFlavor string

const (
	CLIKubernetes = "kubectl"
	CLIOpenShift  = "oc"

	FlavorKubernetes OrchestratorFlavor = "k8s"
	FlavorOpenShift  OrchestratorFlavor = "openshift"

	YAMLSeparator = `\n---\s*\n`

	// CRD Finalizer name
	TridentFinalizer = "trident.netapp.io"
)

type LogLineCallback func(string)

var (
	listOpts   = metav1.ListOptions{}
	getOpts    = metav1.GetOptions{}
	createOpts = metav1.CreateOptions{}
	updateOpts = metav1.UpdateOptions{}
	patchOpts  = metav1.PatchOptions{}

	ctx = context.TODO
)

type Interface interface {
	Version() *version.Info
	ServerVersion() *utils.Version
	Namespace() string
	SetNamespace(namespace string)
	Flavor() OrchestratorFlavor
	CLI() string
	Exec(podName, containerName string, commandArgs []string) ([]byte, error)
	GetDeploymentByLabel(label string, allNamespaces bool) (*appsv1.Deployment, error)
	GetDeploymentsByLabel(label string, allNamespaces bool) ([]appsv1.Deployment, error)
	CheckDeploymentExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteDeploymentByLabel(label string) error
	DeleteDeployment(name, namespace string) error
	PatchDeploymentByLabel(label string, patchBytes []byte) error
	GetServiceByLabel(label string, allNamespaces bool) (*v1.Service, error)
	GetServicesByLabel(label string, allNamespaces bool) ([]v1.Service, error)
	CheckServiceExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteServiceByLabel(label string) error
	DeleteService(name, namespace string) error
	PatchServiceByLabel(label string, patchBytes []byte) error
	GetStatefulSetByLabel(label string, allNamespaces bool) (*appsv1.StatefulSet, error)
	GetStatefulSetsByLabel(label string, allNamespaces bool) ([]appsv1.StatefulSet, error)
	CheckStatefulSetExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteStatefulSetByLabel(label string) error
	DeleteStatefulSet(name, namespace string) error
	GetDaemonSetByLabel(label string, allNamespaces bool) (*appsv1.DaemonSet, error)
	GetDaemonSetsByLabel(label string, allNamespaces bool) ([]appsv1.DaemonSet, error)
	CheckDaemonSetExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteDaemonSetByLabel(label string) error
	DeleteDaemonSet(name, namespace string) error
	PatchDaemonSetByLabel(label string, patchBytes []byte) error
	GetConfigMapByLabel(label string, allNamespaces bool) (*v1.ConfigMap, error)
	GetConfigMapsByLabel(label string, allNamespaces bool) ([]v1.ConfigMap, error)
	CheckConfigMapExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteConfigMapByLabel(label string) error
	CreateConfigMapFromDirectory(path, name, label string) error
	GetPodByLabel(label string, allNamespaces bool) (*v1.Pod, error)
	GetPodsByLabel(label string, allNamespaces bool) ([]v1.Pod, error)
	CheckPodExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeletePodByLabel(label string) error
	DeletePod(name, namespace string) error
	GetPVC(pvcName string) (*v1.PersistentVolumeClaim, error)
	GetPVCByLabel(label string, allNamespaces bool) (*v1.PersistentVolumeClaim, error)
	CheckPVCExists(pvcName string) (bool, error)
	CheckPVCBound(pvcName string) (bool, error)
	DeletePVCByLabel(label string) error
	GetPV(pvName string) (*v1.PersistentVolume, error)
	GetPVByLabel(label string) (*v1.PersistentVolume, error)
	CheckPVExists(pvName string) (bool, error)
	DeletePVByLabel(label string) error
	GetCRD(crdName string) (*apiextensionv1beta1.CustomResourceDefinition, error)
	CheckCRDExists(crdName string) (bool, error)
	DeleteCRD(crdName string) error
	GetPodSecurityPolicyByLabel(label string) (*v1beta1.PodSecurityPolicy, error)
	GetPodSecurityPoliciesByLabel(label string) ([]v1beta1.PodSecurityPolicy, error)
	CheckPodSecurityPolicyExistsByLabel(label string) (bool, string, error)
	DeletePodSecurityPolicyByLabel(label string) error
	DeletePodSecurityPolicy(pspName string) error
	PatchPodSecurityPolicyByLabel(label string, patchBytes []byte) error
	GetServiceAccountByLabel(label string, allNamespaces bool) (*v1.ServiceAccount, error)
	GetServiceAccountsByLabel(label string, allNamespaces bool) ([]v1.ServiceAccount, error)
	CheckServiceAccountExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteServiceAccountByLabel(label string) error
	DeleteServiceAccount(name, namespace string) error
	PatchServiceAccountByLabel(label string, patchBytes []byte) error
	GetClusterRoleByLabel(label string) (*v13.ClusterRole, error)
	GetClusterRolesByLabel(label string) ([]v13.ClusterRole, error)
	CheckClusterRoleExistsByLabel(label string) (bool, string, error)
	DeleteClusterRoleByLabel(label string) error
	DeleteClusterRole(name string) error
	PatchClusterRoleByLabel(label string, patchBytes []byte) error
	GetClusterRoleBindingByLabel(label string) (*v13.ClusterRoleBinding, error)
	GetClusterRoleBindingsByLabel(label string) ([]v13.ClusterRoleBinding, error)
	CheckClusterRoleBindingExistsByLabel(label string) (bool, string, error)
	DeleteClusterRoleBindingByLabel(label string) error
	DeleteClusterRoleBinding(name string) error
	PatchClusterRoleBindingByLabel(label string, patchBytes []byte) error
	GetCSIDriverByLabel(label string) (*v1beta12.CSIDriver, error)
	GetCSIDriversByLabel(label string) ([]v1beta12.CSIDriver, error)
	CheckCSIDriverExistsByLabel(label string) (bool, string, error)
	DeleteCSIDriverByLabel(label string) error
	DeleteCSIDriver(name string) error
	PatchCSIDriverByLabel(label string, patchBytes []byte) error
	CheckNamespaceExists(namespace string) (bool, error)
	CreateSecret(secret *v1.Secret) (*v1.Secret, error)
	UpdateSecret(secret *v1.Secret) (*v1.Secret, error)
	CreateCHAPSecret(secretName, accountName, initiatorSecret, targetSecret string) (*v1.Secret, error)
	GetSecret(secretName string) (*v1.Secret, error)
	GetSecretByLabel(label string, allNamespaces bool) (*v1.Secret, error)
	GetSecretsByLabel(label string, allNamespaces bool) ([]v1.Secret, error)
	CheckSecretExists(secretName string) (bool, error)
	DeleteSecretDefault(secretName string) error
	DeleteSecretByLabel(label string) error
	DeleteSecret(name, namespace string) error
	PatchSecretByLabel(label string, patchBytes []byte) error
	CreateObjectByFile(filePath string) error
	CreateObjectByYAML(yaml string) error
	DeleteObjectByFile(filePath string, ignoreNotFound bool) error
	DeleteObjectByYAML(yaml string, ignoreNotFound bool) error
	RemoveTridentUserFromOpenShiftSCC(user, scc string) error
	GetOpenShiftSCCByName(user, scc string) (bool, bool, []byte, error)
	PatchOpenShiftSCC(newJSONData []byte) error
	FollowPodLogs(pod, container, namespace string, logLineCallback LogLineCallback)
	AddFinalizerToCRD(crdName string) error
	AddFinalizerToCRDs(CRDnames []string) error
	RemoveFinalizerFromCRD(crdName string) error
	GetCRDClient() (*crdclient.Clientset, error)
}

type KubeClient struct {
	clientset    kubernetes.Interface
	extClientset *apiextension.Clientset
	apiResources []*metav1.APIResourceList
	restConfig   *rest.Config
	namespace    string
	versionInfo  *version.Info
	cli          string
	flavor       OrchestratorFlavor
	timeout      time.Duration
}

func NewKubeClient(config *rest.Config, namespace string, k8sTimeout time.Duration) (Interface, error) {
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

	versionInfo, err = clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("couldn't retrieve API server's version: %v", err)
	}
	kubeClient := &KubeClient{
		clientset:    clientset,
		extClientset: extClientset,
		apiResources: make([]*metav1.APIResourceList, 0),
		restConfig:   config,
		namespace:    namespace,
		versionInfo:  versionInfo,
		timeout:      k8sTimeout,
	}

	kubeClient.flavor = kubeClient.discoverKubernetesFlavor()

	switch kubeClient.flavor {
	case FlavorKubernetes:
		kubeClient.cli = CLIKubernetes
	case FlavorOpenShift:
		kubeClient.cli = CLIOpenShift
	}

	log.WithFields(log.Fields{
		"cli":       kubeClient.cli,
		"flavor":    kubeClient.flavor,
		"version":   versionInfo.String(),
		"timeout":   kubeClient.timeout,
		"namespace": namespace,
	}).Debug("Initialized Kubernetes API client.")

	return kubeClient, nil
}

func NewFakeKubeClient() (Interface, error) {

	// Create core client
	clientset := fake.NewSimpleClientset()
	kubeClient := &KubeClient{
		clientset:    clientset,
		apiResources: make([]*metav1.APIResourceList, 0),
	}

	return kubeClient, nil
}

func (k *KubeClient) discoverKubernetesFlavor() OrchestratorFlavor {

	// Read the API groups/resources available from the server
	discoveryClient := k.clientset.Discovery()

	_, apiResources, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		log.WithField("error", err).Warning("Could not get server resources, defaulting to Kubernetes flavor.")
		return FlavorKubernetes
	}

	// Populate the cache so the dynamic client is fast
	k.apiResources = apiResources

	resources, err := discovery.GroupVersionResources(k.apiResources)
	if err != nil {
		log.WithField("error", err).Warning("Could not parse server resources, defaulting to Kubernetes flavor.")
		return FlavorKubernetes
	}

	for gvr := range resources {

		//log.WithFields(log.Fields{
		//	"group":    gvr.Group,
		//	"version":  gvr.Version,
		//	"resource": gvr.Resource,
		//}).Debug("Considering dynamic resource, looking for openshift group.")

		if strings.Contains(gvr.Group, "openshift") {
			return FlavorOpenShift
		}
	}

	return FlavorKubernetes
}

func (k *KubeClient) Version() *version.Info {
	return k.versionInfo
}

func (k *KubeClient) ServerVersion() *utils.Version {
	return utils.MustParseSemantic(k.versionInfo.GitVersion)
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

func (k *KubeClient) Exec(podName, containerName string, commandArgs []string) ([]byte, error) {

	// Get the pod and ensure it is in a good state
	pod, err := k.clientset.CoreV1().Pods(k.namespace).Get(ctx(), podName, getOpts)
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

	log.Debugf("Invoking tunneled command: '%v'", strings.Join(commandArgs, " "))

	// Execute the request
	err = exec.Stream(remotecommand.StreamOptions{
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
		return nil, utils.NotFoundError(fmt.Sprintf("no deployments have the label %s", label))
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

	deploymentList, err := k.clientset.AppsV1().Deployments(namespace).List(ctx(), listOptions)
	if err != nil {
		return nil, err
	}

	return deploymentList.Items, nil
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

	err = k.clientset.AppsV1().Deployments(k.namespace).Delete(ctx(), deployment.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":      label,
		"deployment": deployment.Name,
		"namespace":  k.namespace,
	}).Debug("Deleted Kubernetes deployment.")

	return nil
}

// DeleteDeployment deletes a deployment object matching the specified name and namespace
func (k *KubeClient) DeleteDeployment(name, namespace string) error {

	if err := k.clientset.AppsV1().Deployments(namespace).Delete(ctx(), name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"deployment": name,
		"namespace":  namespace,
	}).Debug("Deleted Kubernetes deployment.")

	return nil
}

// PatchDeploymentByLabel patches a deployment object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchDeploymentByLabel(label string, patchBytes []byte) error {

	deployment, err := k.GetDeploymentByLabel(label, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.AppsV1().Deployments(k.namespace).Patch(ctx(), deployment.Name,
		commontypes.StrategicMergePatchType, patchBytes, patchOpts); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":      label,
		"deployment": deployment.Name,
		"namespace":  k.namespace,
	}).Debug("Patched Kubernetes deployment.")

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
		return nil, utils.NotFoundError(fmt.Sprintf("no services have the label %s", label))
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

	serviceList, err := k.clientset.CoreV1().Services(namespace).List(ctx(), listOptions)
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

	if err = k.clientset.CoreV1().Services(k.namespace).Delete(ctx(), service.Name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":     label,
		"service":   service.Name,
		"namespace": k.namespace,
	}).Debug("Deleted Kubernetes service.")

	return nil
}

// DeleteService deletes a Service object matching the specified name and namespace
func (k *KubeClient) DeleteService(name, namespace string) error {

	if err := k.clientset.CoreV1().Services(namespace).Delete(ctx(), name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"Service":   name,
		"namespace": namespace,
	}).Debug("Deleted Kubernetes Service.")

	return nil
}

// PatchServiceByLabel patches a deployment object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchServiceByLabel(label string, patchBytes []byte) error {

	service, err := k.GetServiceByLabel(label, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.CoreV1().Services(k.namespace).Patch(ctx(), service.Name,
		commontypes.StrategicMergePatchType, patchBytes, patchOpts); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":      label,
		"deployment": service.Name,
		"namespace":  k.namespace,
	}).Debug("Patched Kubernetes service.")

	return nil
}

// GetStatefulSetByLabel returns a statefulset object matching the specified label if it is unique
func (k *KubeClient) GetStatefulSetByLabel(label string, allNamespaces bool) (*appsv1.StatefulSet, error) {

	statefulsets, err := k.GetStatefulSetsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(statefulsets) == 1 {
		return &statefulsets[0], nil
	} else if len(statefulsets) > 1 {
		return nil, fmt.Errorf("multiple statefulsets have the label %s", label)
	} else {
		return nil, utils.NotFoundError(fmt.Sprintf("no statefulsets have the label %s", label))
	}
}

// GetStatefulSetsByLabel returns all stateful objects matching the specified label
func (k *KubeClient) GetStatefulSetsByLabel(label string, allNamespaces bool) ([]appsv1.StatefulSet, error) {

	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	namespace := k.namespace
	if allNamespaces {
		namespace = ""
	}

	statefulSetList, err := k.clientset.AppsV1().StatefulSets(namespace).List(ctx(), listOptions)
	if err != nil {
		return nil, err
	}

	return statefulSetList.Items, nil
}

// CheckStatefulSetExistsByLabel returns true if one or more statefulset objects
// matching the specified label exist.
func (k *KubeClient) CheckStatefulSetExistsByLabel(label string, allNamespaces bool) (bool, string, error) {

	statefulsets, err := k.GetStatefulSetsByLabel(label, allNamespaces)
	if err != nil {
		return false, "", err
	}

	switch len(statefulsets) {
	case 0:
		return false, "", nil
	case 1:
		return true, statefulsets[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeleteStatefulSetByLabel deletes a statefulset object matching the specified label
// in the namespace of the client.
func (k *KubeClient) DeleteStatefulSetByLabel(label string) error {

	statefulset, err := k.GetStatefulSetByLabel(label, false)
	if err != nil {
		return err
	}

	err = k.clientset.AppsV1().StatefulSets(k.namespace).Delete(ctx(), statefulset.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":       label,
		"statefulset": statefulset.Name,
		"namespace":   k.namespace,
	}).Debug("Deleted Kubernetes statefulset.")

	return nil
}

// DeleteStatefulSet deletes a statefulset object matching the specified name and namespace.
func (k *KubeClient) DeleteStatefulSet(name, namespace string) error {

	if err := k.clientset.AppsV1().StatefulSets(namespace).Delete(ctx(), name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"statefulset": name,
		"namespace":   k.namespace,
	}).Debug("Deleted Kubernetes statefulset.")

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
		return nil, utils.NotFoundError(fmt.Sprintf("no daemonsets have the label %s", label))
	}
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

	daemonSetList, err := k.clientset.AppsV1().DaemonSets(namespace).List(ctx(), listOptions)
	if err != nil {
		return nil, err
	}

	return daemonSetList.Items, nil
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

	err = k.clientset.AppsV1().DaemonSets(k.namespace).Delete(ctx(), daemonset.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":     label,
		"daemonset": daemonset.Name,
		"namespace": k.namespace,
	}).Debug("Deleted Kubernetes daemonset.")

	return nil
}

// DeleteDaemonSet deletes a DaemonSet object matching the specified name and namespace
func (k *KubeClient) DeleteDaemonSet(name, namespace string) error {

	if err := k.clientset.AppsV1().DaemonSets(namespace).Delete(ctx(), name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"DaemonSet": name,
		"namespace": namespace,
	}).Debug("Deleted Kubernetes DaemonSet.")

	return nil
}

// PatchDaemonSetByLabel patches a DaemonSet object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchDaemonSetByLabel(label string, patchBytes []byte) error {

	daemonSet, err := k.GetDaemonSetByLabel(label, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.AppsV1().DaemonSets(k.namespace).Patch(ctx(), daemonSet.Name,
		commontypes.StrategicMergePatchType, patchBytes, patchOpts); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":     label,
		"DaemonSet": daemonSet.Name,
		"namespace": k.namespace,
	}).Debug("Patched Kubernetes DaemonSet.")

	return nil
}

// GetConfigMapByLabel returns a configmap object matching the specified label if it is unique
func (k *KubeClient) GetConfigMapByLabel(label string, allNamespaces bool) (*v1.ConfigMap, error) {

	configMaps, err := k.GetConfigMapsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(configMaps) == 1 {
		return &configMaps[0], nil
	} else if len(configMaps) > 1 {
		return nil, fmt.Errorf("multiple configmaps have the label %s", label)
	} else {
		return nil, utils.NotFoundError(fmt.Sprintf("no configmaps have the label %s", label))
	}
}

// GetConfigMapsByLabel returns all configmap objects matching the specified label
func (k *KubeClient) GetConfigMapsByLabel(label string, allNamespaces bool) ([]v1.ConfigMap, error) {

	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	namespace := k.namespace
	if allNamespaces {
		namespace = ""
	}

	configMapList, err := k.clientset.CoreV1().ConfigMaps(namespace).List(ctx(), listOptions)
	if err != nil {
		return nil, err
	}

	return configMapList.Items, nil
}

// CheckConfigMapExistsByLabel returns true if one or more configmap objects
// matching the specified label exist.
func (k *KubeClient) CheckConfigMapExistsByLabel(label string, allNamespaces bool) (bool, string, error) {

	configMaps, err := k.GetConfigMapsByLabel(label, allNamespaces)
	if err != nil {
		return false, "", err
	}

	switch len(configMaps) {
	case 0:
		return false, "", nil
	case 1:
		return true, configMaps[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeleteConfigMapByLabel deletes a configmap object matching the specified label
func (k *KubeClient) DeleteConfigMapByLabel(label string) error {

	configmap, err := k.GetConfigMapByLabel(label, false)
	if err != nil {
		return err
	}

	err = k.clientset.CoreV1().ConfigMaps(k.namespace).Delete(ctx(), configmap.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":     label,
		"configmap": configmap.Name,
		"namespace": k.namespace,
	}).Debug("Deleted Kubernetes configmap.")

	return nil
}

func (k *KubeClient) CreateConfigMapFromDirectory(_, _, _ string) error {
	return errors.New("not implemented")
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
		return nil, utils.NotFoundError(fmt.Sprintf("no pods have the label %s", label))
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

	podList, err := k.clientset.CoreV1().Pods(namespace).List(ctx(), listOptions)
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

	if err = k.clientset.CoreV1().Pods(k.namespace).Delete(ctx(), pod.Name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":     label,
		"pod":       pod.Name,
		"namespace": k.namespace,
	}).Debug("Deleted Kubernetes pod.")

	return nil
}

// DeletePod deletes a pod object matching the specified name and namespace
func (k *KubeClient) DeletePod(name, namespace string) error {

	var gracePeriod int64
	deleteOptions := &metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	}

	if err := k.clientset.CoreV1().Pods(namespace).Delete(ctx(), name, *deleteOptions); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"pod":       name,
		"namespace": namespace,
	}).Debug("Deleted Kubernetes pod.")

	return nil
}

func (k *KubeClient) GetPVC(pvcName string) (*v1.PersistentVolumeClaim, error) {
	var options metav1.GetOptions
	return k.clientset.CoreV1().PersistentVolumeClaims(k.namespace).Get(ctx(), pvcName, options)
}

func (k *KubeClient) GetPVCByLabel(label string, allNamespaces bool) (*v1.PersistentVolumeClaim, error) {

	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	namespace := k.namespace
	if allNamespaces {
		namespace = ""
	}

	pvcList, err := k.clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx(), listOptions)
	if err != nil {
		return nil, err
	}

	if len(pvcList.Items) == 1 {
		return &pvcList.Items[0], nil
	} else if len(pvcList.Items) > 1 {
		return nil, fmt.Errorf("multiple PVCs have the label %s", label)
	} else {
		return nil, utils.NotFoundError(fmt.Sprintf("no PVCs have the label %s", label))
	}
}

func (k *KubeClient) CheckPVCExists(pvc string) (bool, error) {
	if _, err := k.GetPVC(pvc); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k *KubeClient) CheckPVCBound(pvcName string) (bool, error) {

	pvc, err := k.GetPVC(pvcName)
	if err != nil {
		return false, err
	}

	return pvc.Status.Phase == v1.ClaimBound, nil
}

func (k *KubeClient) DeletePVCByLabel(label string) error {

	pvc, err := k.GetPVCByLabel(label, false)
	if err != nil {
		return err
	}

	err = k.clientset.CoreV1().PersistentVolumeClaims(k.namespace).Delete(ctx(), pvc.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":     label,
		"namespace": k.namespace,
	}).Debug("Deleted PVC by label.")

	return nil
}

func (k *KubeClient) GetPV(pvName string) (*v1.PersistentVolume, error) {
	var options metav1.GetOptions
	return k.clientset.CoreV1().PersistentVolumes().Get(ctx(), pvName, options)
}

func (k *KubeClient) GetPVByLabel(label string) (*v1.PersistentVolume, error) {

	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	pvList, err := k.clientset.CoreV1().PersistentVolumes().List(ctx(), listOptions)
	if err != nil {
		return nil, err
	}

	if len(pvList.Items) == 1 {
		return &pvList.Items[0], nil
	} else if len(pvList.Items) > 1 {
		return nil, fmt.Errorf("multiple PVs have the label %s", label)
	} else {
		return nil, utils.NotFoundError(fmt.Sprintf("no PVs have the label %s", label))
	}
}

func (k *KubeClient) CheckPVExists(pvName string) (bool, error) {
	if _, err := k.GetPV(pvName); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (k *KubeClient) DeletePVByLabel(label string) error {

	pv, err := k.GetPVByLabel(label)
	if err != nil {
		return err
	}

	if err = k.clientset.CoreV1().PersistentVolumes().Delete(ctx(), pv.Name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithField("label", label).Debug("Deleted PV by label.")

	return nil
}

func (k *KubeClient) GetCRD(crdName string) (*apiextensionv1beta1.CustomResourceDefinition, error) {
	var options metav1.GetOptions
	return k.extClientset.ApiextensionsV1beta1().CustomResourceDefinitions().Get(ctx(), crdName, options)
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
	return k.extClientset.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(ctx(), crdName, k.deleteOptions())
}

// GetPodSecurityPolicyByLabel returns a pod security policy object matching the specified label if it is unique
func (k *KubeClient) GetPodSecurityPolicyByLabel(label string) (*v1beta1.PodSecurityPolicy, error) {

	pspList, err := k.GetPodSecurityPoliciesByLabel(label)
	if err != nil {
		return nil, err
	}

	if len(pspList) == 1 {
		return &pspList[0], nil
	} else if len(pspList) > 1 {
		return nil, fmt.Errorf("multiple pod security policies have the label %s", label)
	} else {
		return nil, utils.NotFoundError(fmt.Sprintf("no pod security policy have the label %s", label))
	}
}

// GetPodSecurityPoliciesByLabel returns all pod security policy objects matching the specified label
func (k *KubeClient) GetPodSecurityPoliciesByLabel(label string) ([]v1beta1.PodSecurityPolicy,
	error) {

	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	pspList, err := k.clientset.PolicyV1beta1().PodSecurityPolicies().List(ctx(), listOptions)
	if err != nil {
		return nil, err
	}

	return pspList.Items, nil
}

// CheckPodSecurityPolicyExistsByLabel returns true if one or more pod security policy objects
// matching the specified label exist.
func (k *KubeClient) CheckPodSecurityPolicyExistsByLabel(label string) (bool, string, error) {

	pspList, err := k.GetPodSecurityPoliciesByLabel(label)
	if err != nil {
		return false, "", err
	}

	switch len(pspList) {
	case 0:
		return false, "", nil
	case 1:
		return true, pspList[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeletePodSecurityPolicyByLabel deletes a pod security policy object matching the specified label
// in the namespace of the client.
func (k *KubeClient) DeletePodSecurityPolicyByLabel(label string) error {

	psp, err := k.GetPodSecurityPolicyByLabel(label)
	if err != nil {
		return err
	}

	if err = k.clientset.PolicyV1beta1().PodSecurityPolicies().Delete(ctx(), psp.Name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":             label,
		"podSecurityPolicy": psp.Name,
	}).Debug("Deleted Kubernetes pod security policy.")

	return nil
}

// DeletePodSecurityPolicy deletes a pod security policy object matching the specified PSP name.
func (k *KubeClient) DeletePodSecurityPolicy(pspName string) error {

	if err := k.clientset.PolicyV1beta1().PodSecurityPolicies().Delete(ctx(), pspName, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"service": pspName,
	}).Debug("Deleted Kubernetes pod security policy.")

	return nil
}

// PatchPodSecurityPolicyByLabel patches a pod security policy object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchPodSecurityPolicyByLabel(label string, patchBytes []byte) error {

	psp, err := k.GetPodSecurityPolicyByLabel(label)
	if err != nil {
		return err
	}

	if _, err = k.clientset.PolicyV1beta1().PodSecurityPolicies().Patch(ctx(), psp.Name,
		commontypes.StrategicMergePatchType, patchBytes, patchOpts); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":      label,
		"deployment": psp.Name,
	}).Debug("Patched Kubernetes pod security policy.")

	return nil
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
		return nil, utils.NotFoundError(fmt.Sprintf("no service accounts have the label %s", label))
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

	serviceAccountList, err := k.clientset.CoreV1().ServiceAccounts(namespace).List(ctx(), listOptions)
	if err != nil {
		return nil, err
	}

	return serviceAccountList.Items, nil
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

	err = k.clientset.CoreV1().ServiceAccounts(k.namespace).Delete(ctx(), serviceAccount.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":          label,
		"serviceAccount": serviceAccount.Name,
		"namespace":      k.namespace,
	}).Debug("Deleted Kubernetes service account.")

	return nil
}

// DeleteServiceAccount deletes a service account object matching the specified
// name and namespace.
func (k *KubeClient) DeleteServiceAccount(name, namespace string) error {

	if err := k.clientset.CoreV1().ServiceAccounts(namespace).Delete(ctx(), name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"serviceAccount": name,
		"namespace":      namespace,
	}).Debug("Deleted Kubernetes service account.")

	return nil
}

// PatchServiceAccountByLabel patches a Service Account object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchServiceAccountByLabel(label string, patchBytes []byte) error {

	serviceAccount, err := k.GetServiceAccountByLabel(label, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.CoreV1().ServiceAccounts(k.namespace).Patch(ctx(), serviceAccount.Name,
		commontypes.StrategicMergePatchType, patchBytes, patchOpts); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":          label,
		"serviceAccount": serviceAccount.Name,
		"namespace":      k.namespace,
	}).Debug("Patched Kubernetes Service Account.")

	return nil
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
		return nil, utils.NotFoundError(fmt.Sprintf("no cluster roles have the label %s", label))
	}
}

// GetClusterRoleByLabel returns all cluster role objects matching the specified label
func (k *KubeClient) GetClusterRolesByLabel(label string) ([]v13.ClusterRole, error) {

	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	clusterRoleList, err := k.clientset.RbacV1().ClusterRoles().List(ctx(), listOptions)
	if err != nil {
		return nil, err
	}

	return clusterRoleList.Items, nil
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

	if err = k.clientset.RbacV1().ClusterRoles().Delete(ctx(), clusterRole.Name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":       label,
		"clusterRole": clusterRole.Name,
	}).Debug("Deleted Kubernetes cluster role.")

	return nil
}

// DeleteClusterRole deletes a cluster role object matching the specified name
func (k *KubeClient) DeleteClusterRole(name string) error {

	if err := k.clientset.RbacV1().ClusterRoles().Delete(ctx(), name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"clusterRole": name,
	}).Debug("Deleted Kubernetes cluster role.")

	return nil
}

// PatchClusterRoleByLabel patches a Cluster Role object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchClusterRoleByLabel(label string, patchBytes []byte) error {

	clusterRole, err := k.GetClusterRoleByLabel(label)
	if err != nil {
		return err
	}

	if _, err = k.clientset.RbacV1().ClusterRoles().Patch(ctx(), clusterRole.Name,
		commontypes.StrategicMergePatchType, patchBytes, patchOpts); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":       label,
		"clusterRole": clusterRole.Name,
	}).Debug("Patched Kubernetes cluster role.")

	return nil
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
		return nil, utils.NotFoundError(fmt.Sprintf("no cluster role bindings have the label %s", label))
	}
}

// GetClusterRoleBindingsByLabel returns all cluster role binding objects matching the specified label
func (k *KubeClient) GetClusterRoleBindingsByLabel(label string) ([]v13.ClusterRoleBinding, error) {

	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	clusterRoleBindingList, err := k.clientset.RbacV1().ClusterRoleBindings().List(ctx(), listOptions)
	if err != nil {
		return nil, err
	}

	return clusterRoleBindingList.Items, nil
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

	err = k.clientset.RbacV1().ClusterRoleBindings().Delete(ctx(), clusterRoleBinding.Name, k.deleteOptions())
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":              label,
		"clusterRoleBinding": clusterRoleBinding.Name,
	}).Debug("Deleted Kubernetes cluster role binding.")

	return nil
}

// DeleteClusterRoleBinding deletes a cluster role binding object matching the specified name
func (k *KubeClient) DeleteClusterRoleBinding(name string) error {

	if err := k.clientset.RbacV1().ClusterRoleBindings().Delete(ctx(), name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"clusterRoleBinding": name,
	}).Debug("Deleted Kubernetes cluster role binding.")

	return nil
}

// PatchClusterRoleBindingByLabel patches a Cluster Role binding object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchClusterRoleBindingByLabel(label string, patchBytes []byte) error {

	clusterRoleBinding, err := k.GetClusterRoleBindingByLabel(label)
	if err != nil {
		return err
	}

	if _, err = k.clientset.RbacV1().ClusterRoleBindings().Patch(ctx(), clusterRoleBinding.Name,
		commontypes.StrategicMergePatchType, patchBytes, patchOpts); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":              label,
		"clusterRoleBinding": clusterRoleBinding.Name,
	}).Debug("Patched Kubernetes cluster role binding.")

	return nil
}

// GetCSIDriverByLabel returns a CSI driver object matching the specified label if it is unique
func (k *KubeClient) GetCSIDriverByLabel(label string) (*v1beta12.CSIDriver, error) {

	CSIDrivers, err := k.GetCSIDriversByLabel(label)
	if err != nil {
		return nil, err
	}

	if len(CSIDrivers) == 1 {
		return &CSIDrivers[0], nil
	} else if len(CSIDrivers) > 1 {
		return nil, fmt.Errorf("multiple CSI drivers have the label %s", label)
	} else {
		return nil, utils.NotFoundError(fmt.Sprintf("no CSI drivers have the label %s", label))
	}
}

// GetCSIDriversByLabel returns all CSI driver objects matching the specified label
func (k *KubeClient) GetCSIDriversByLabel(label string) ([]v1beta12.CSIDriver, error) {

	listOptions, err := k.listOptionsFromLabel(label)
	if err != nil {
		return nil, err
	}

	CSIDriverList, err := k.clientset.StorageV1beta1().CSIDrivers().List(ctx(), listOptions)
	if err != nil {
		return nil, err
	}

	return CSIDriverList.Items, nil
}

// GetCSIDriversByLabel returns true if one or more CSI Driver objects
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

	if err = k.clientset.StorageV1beta1().CSIDrivers().Delete(ctx(), CSIDriver.Name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":       label,
		"CSIDriverCR": CSIDriver.Name,
	}).Debug("Deleted CSI driver.")

	return nil
}

// DeleteCSIDriverByLabel deletes a CSI Driver object matching the specified name
func (k *KubeClient) DeleteCSIDriver(name string) error {

	if err := k.clientset.StorageV1beta1().CSIDrivers().Delete(ctx(), name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"CSIDriverCR": name,
	}).Debug("Deleted CSI driver.")

	return nil
}

// PatchCSIDriverByLabel patches a deployment object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchCSIDriverByLabel(label string, patchBytes []byte) error {

	csiDriver, err := k.GetCSIDriverByLabel(label)
	if err != nil {
		return err
	}

	if _, err = k.clientset.StorageV1beta1().CSIDrivers().Patch(ctx(), csiDriver.Name,
		commontypes.StrategicMergePatchType, patchBytes, patchOpts); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":      label,
		"deployment": csiDriver.Name,
	}).Debug("Patched Kubernetes CSI driver.")

	return nil
}

func (k *KubeClient) CheckNamespaceExists(namespace string) (bool, error) {

	if _, err := k.clientset.CoreV1().Namespaces().Get(ctx(), namespace, getOpts); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CreateSecret creates a new Secret
func (k *KubeClient) CreateSecret(secret *v1.Secret) (*v1.Secret, error) {
	return k.clientset.CoreV1().Secrets(k.namespace).Create(ctx(), secret, createOpts)
}

// UpdateSecret updates an existing Secret
func (k *KubeClient) UpdateSecret(secret *v1.Secret) (*v1.Secret, error) {
	return k.clientset.CoreV1().Secrets(k.namespace).Update(ctx(), secret, updateOpts)
}

// CreateCHAPSecret creates a new Secret for iSCSI CHAP mutual authentication
func (k *KubeClient) CreateCHAPSecret(secretName, accountName, initiatorSecret, targetSecret string,
) (*v1.Secret, error) {

	return k.CreateSecret(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: k.namespace,
			Name:      secretName,
		},
		Type: "kubernetes.io/iscsi-chap",
		Data: map[string][]byte{
			"discovery.sendtargets.auth.username":    []byte(accountName),
			"discovery.sendtargets.auth.password":    []byte(initiatorSecret),
			"discovery.sendtargets.auth.username_in": []byte(accountName),
			"discovery.sendtargets.auth.password_in": []byte(targetSecret),
			"node.session.auth.username":             []byte(accountName),
			"node.session.auth.password":             []byte(initiatorSecret),
			"node.session.auth.username_in":          []byte(accountName),
			"node.session.auth.password_in":          []byte(targetSecret),
		},
	})
}

// GetSecret looks up a Secret by name
func (k *KubeClient) GetSecret(secretName string) (*v1.Secret, error) {
	var options metav1.GetOptions
	return k.clientset.CoreV1().Secrets(k.namespace).Get(ctx(), secretName, options)
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

	secretList, err := k.clientset.CoreV1().Secrets(namespace).List(ctx(), listOptions)
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

	if err = k.clientset.CoreV1().Secrets(k.namespace).Delete(ctx(), secret.Name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":     label,
		"namespace": k.namespace,
	}).Debug("Deleted secret by label.")

	return nil
}

// DeleteSecret deletes the specified Secret by name and namespace
func (k *KubeClient) DeleteSecret(name, namespace string) error {
	if err := k.clientset.CoreV1().Secrets(namespace).Delete(ctx(), name, k.deleteOptions()); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Debug("Deleted secret by label.")

	return nil
}

// PatchPodSecurityPolicyByLabel patches a pod security policy object matching the specified label
// in the namespace of the client.
func (k *KubeClient) PatchSecretByLabel(label string, patchBytes []byte) error {

	secret, err := k.GetSecretByLabel(label, false)
	if err != nil {
		return err
	}

	if _, err = k.clientset.CoreV1().Secrets(k.namespace).Patch(ctx(), secret.Name,
		commontypes.StrategicMergePatchType, patchBytes, patchOpts); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":      label,
		"deployment": secret.Name,
		"namespace":  k.namespace,
	}).Debug("Patched Kubernetes secret.")

	return nil
}

// CreateObjectByFile creates one or more objects on the server from a YAML/JSON file at the specified path.
func (k *KubeClient) CreateObjectByFile(filePath string) error {

	content, err := ioutil.ReadFile(filePath)
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
				log.WithFields(log.Fields{
					"yamlDocument": yamlDocument,
					"err":          returnError,
				}).Errorf("Object creation failed.")
				return returnError
			}
			return nil
		}

		createObjectNotify := func(err error, duration time.Duration) {
			log.WithFields(log.Fields{
				"yamlDocument": yamlDocument,
				"increment":    duration,
				"err":          err,
			}).Debugf("Object not created, waiting.")
		}
		createObjectBackoff := backoff.NewExponentialBackOff()
		createObjectBackoff.MaxElapsedTime = k.timeout

		log.WithField("yamlDocument", yamlDocument).Trace("Waiting for object to be created.")

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
				objNamespace = ns.(string)
			}
			if name, ok := metadata["name"]; ok {
				objName = name.(string)
			}
		}
	}

	log.WithFields(log.Fields{
		"name":      objName,
		"namespace": objNamespace,
		"kind":      gvk.Kind,
	}).Debugf("Creating object.")

	// Create the object
	if namespaced {
		if _, err = client.Resource(*gvr).Namespace(objNamespace).Create(ctx(), unstruct, createOpts); err != nil {
			return err
		}
	} else {
		if _, err = client.Resource(*gvr).Create(ctx(), unstruct, createOpts); err != nil {
			return err
		}
	}

	log.WithField("name", objName).Debug("Created object by YAML.")
	return nil
}

// DeleteObjectByFile deletes one or more objects on the server from a YAML/JSON file at the specified path.
func (k *KubeClient) DeleteObjectByFile(filePath string, ignoreNotFound bool) error {

	content, err := ioutil.ReadFile(filePath)
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
				objNamespace = ns.(string)
			}
			if name, ok := metadata["name"]; ok {
				objName = name.(string)
			}
		}
	}

	// We must have the object name to delete it
	if objName == "" {
		return errors.New("cannot determine object name from YAML")
	}

	log.WithFields(log.Fields{
		"name":      objName,
		"namespace": objNamespace,
		"kind":      gvk.Kind,
	}).Debug("Deleting object.")

	// Delete the object
	if namespaced {
		err = client.Resource(*gvr).Namespace(objNamespace).Delete(ctx(), objName, k.deleteOptions())
	} else {
		err = client.Resource(*gvr).Delete(ctx(), objName, k.deleteOptions())
	}

	if err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok {
			if ignoreNotFound && statusErr.Status().Reason == metav1.StatusReasonNotFound {
				log.WithField("name", objName).Debugf("Object not found for delete, ignoring.")
				return nil
			}
		}
		return err
	}

	log.WithField("name", objName).Debug("Deleted object by YAML.")
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
				objNamespace = ns.(string)
			}
			if name, ok := metadata["name"]; ok {
				objName = name.(string)
			}
		}
	}

	log.WithFields(log.Fields{
		"name":      objName,
		"namespace": objNamespace,
		"kind":      gvk.Kind,
	}).Debug("Getting object.")

	// Get the object
	if namespaced {
		return client.Resource(*gvr).Namespace(objNamespace).Get(ctx(), objName, getOpts)
	} else {
		return client.Resource(*gvr).Get(ctx(), objName, getOpts)
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
				objNamespace = ns.(string)
			}
			if name, ok := metadata["name"]; ok {
				objName = name.(string)
			}
		}
	}

	log.WithFields(log.Fields{
		"name":      objName,
		"namespace": objNamespace,
		"kind":      gvk.Kind,
	}).Debug("Updating object.")

	// Update the object
	updateOptions := updateOpts

	if namespaced {
		if _, err = client.Resource(*gvr).Namespace(objNamespace).Update(ctx(), unstruct, updateOptions); err != nil {
			return err
		}
	} else {
		if _, err = client.Resource(*gvr).Update(ctx(), unstruct, updateOptions); err != nil {
			return err
		}
	}

	log.WithField("name", objName).Debug("Updated object by YAML.")
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

	log.WithFields(log.Fields{
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

	if gvr, namespaced, err := k.getDynamicResourceNoRefresh(gvk); utils.IsNotFoundError(err) {

		discoveryClient := k.clientset.Discovery()

		// Read the API groups/resources available from the server
		_, apiResources, err := discoveryClient.ServerGroupsAndResources()
		if err != nil {
			log.WithFields(log.Fields{
				"group":   gvk.Group,
				"version": gvk.Version,
				"kind":    gvk.Kind,
				"error":   err,
			}).Error("Could not get dynamic resource, failed to list server resources.")
			return nil, false, err
		}

		k.apiResources = apiResources

		// Try again once after updating the API resources list
		return k.getDynamicResourceNoRefresh(gvk)

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

	for _, resources := range k.apiResources {

		groupVersion, err := schema.ParseGroupVersion(resources.GroupVersion)
		if err != nil {
			log.WithField("groupVersion", groupVersion).Error("Could not parse group/version.")
			continue
		}

		for _, resource := range resources.APIResources {

			//log.WithFields(log.Fields{
			//	"group":    groupVersion.Group,
			//	"version":  groupVersion.Version,
			//	"kind":     resource.Kind,
			//	"resource": resource.Name,
			//}).Debug("Considering dynamic API resource.")

			if groupVersion.Group == gvk.Group &&
				groupVersion.Version == gvk.Version &&
				resource.Kind == gvk.Kind {

				log.WithFields(log.Fields{
					"group":    groupVersion.Group,
					"version":  groupVersion.Version,
					"kind":     resource.Kind,
					"resource": resource.Name,
				}).Debug("Found API resource.")

				return &schema.GroupVersionResource{
					Group:    groupVersion.Group,
					Version:  groupVersion.Version,
					Resource: resource.Name,
				}, resource.Namespaced, nil
			}
		}
	}

	log.WithFields(log.Fields{
		"group":   gvk.Group,
		"version": gvk.Version,
		"kind":    gvk.Kind,
	}).Error("API resource not found.")

	return nil, false, utils.NotFoundError("API resource not found")
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
		log.WithField("user", sccUser).Debug("SCC user not found, ignoring.")
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

	log.WithField("scc", scc).Debug("SCC found.")
	SCCExist = true

	// Convert to JSON
	jsonData, err := unstruct.MarshalJSON()
	if err != nil {
		log.WithField("scc", scc).Errorf("failed to marshal unstructured data in JSON: %v", unstruct)
		return SCCExist, SCCUserExist, jsonData, err
	}

	// Find the user
	if users, ok := unstruct.Object["users"]; ok {
		if usersSlice, ok := users.([]interface{}); ok {
			for _, userIntf := range usersSlice {
				if user, ok := userIntf.(string); ok && user == sccUser {
					log.WithField("sccUser", sccUser).Debug("SCC User found.")

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

func (k *KubeClient) FollowPodLogs(pod, container, namespace string, logLineCallback LogLineCallback) {

	logOptions := &v1.PodLogOptions{
		Container:  container,
		Follow:     true,
		Previous:   false,
		Timestamps: false,
	}

	req := k.clientset.CoreV1().RESTClient().
		Get().
		Namespace(namespace).
		Name(pod).
		Resource("pods").
		SubResource("log").
		Param("follow", strconv.FormatBool(logOptions.Follow)).
		Param("container", logOptions.Container).
		Param("previous", strconv.FormatBool(logOptions.Previous)).
		Param("timestamps", strconv.FormatBool(logOptions.Timestamps))

	readCloser, err := req.Stream(ctx())
	if err != nil {
		log.Errorf("Could not follow pod logs; %v", err)
		return
	}
	defer func() {
		_ = readCloser.Close()
	}()

	// Create a new scanner
	buff := bufio.NewScanner(readCloser)

	// Iterate over buff and handle one line at a time
	for buff.Scan() {
		line := buff.Text()
		logLineCallback(line)
	}

	log.WithFields(log.Fields{
		"pod":       pod,
		"container": container,
	}).Debug("Received EOF from pod logs.")
}

// addFinalizerToCRDObject is a helper function that updates the CRD object to include our Trident finalizer (
//definitions are not namespaced)
func (k *KubeClient) addFinalizerToCRDObject(crdName string, gvk *schema.GroupVersionKind,
	gvr *schema.GroupVersionResource, client dynamic.Interface) error {

	var err error

	log.WithFields(log.Fields{
		"name": crdName,
		"kind": gvk.Kind,
	}).Debugf("Adding finalizers to CRD object %v.", crdName)

	// Get the CRD object
	var unstruct *unstructured.Unstructured
	if unstruct, err = client.Resource(*gvr).Get(ctx(), crdName, getOpts); err != nil {
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
							log.Debugf("Trident finalizer already present on Kubernetes CRD object %v, nothing to do", crdName)
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

	if _, err = client.Resource(*gvr).Update(ctx(), unstruct, updateOptions); err != nil {
		return err
	}

	log.Debugf("Added finalizers to Kubernetes CRD object %v", crdName)

	return nil
}

// AddFinalizerToCRDs updates the CRD objects to include our Trident finalizer (definitions are not namespaced)
func (k *KubeClient) AddFinalizerToCRDs(CRDnames []string) error {
	gvk := &schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1beta1",
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
			log.Errorf("error in adding finalizers to Kubernetes CRD object %v", crdName)
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
	   apiextensions.k8s.io/v1beta1
	*/
	gvk := &schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1beta1",
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

	log.WithFields(log.Fields{
		"name": crdName,
		"kind": gvk.Kind,
	}).Debugf("Adding finalizers to CRD object.")

	// Get the CRD object
	var unstruct *unstructured.Unstructured
	if unstruct, err = client.Resource(*gvr).Get(ctx(), crdName, getOpts); err != nil {
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
							log.Debugf("Trident finalizer already present on Kubernetes CRD object %v, nothing to do", crdName)
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
	if _, err = client.Resource(*gvr).Update(ctx(), unstruct, updateOpts); err != nil {
		return err
	}

	log.Debugf("Added finalizers to Kubernetes CRD object %v", crdName)

	return nil
}

// RemoveFinalizerFromCRD updates the CRD object to remove all finalizers (definitions are not namespaced)
func (k *KubeClient) RemoveFinalizerFromCRD(crdName string) error {
	/*
	   $ kubectl api-resources -o wide | grep -i crd
	    NAME                              SHORTNAMES          APIGROUP                       NAMESPACED   KIND                             VERBS
	   customresourcedefinitions         crd,crds            apiextensions.k8s.io           false        CustomResourceDefinition         [create delete deletecollection get list patch update watch]

	   $ kubectl  api-versions | grep apiextensions
	   apiextensions.k8s.io/v1beta1
	*/
	gvk := &schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1beta1",
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

	log.WithFields(log.Fields{
		"name": crdName,
		"kind": gvk.Kind,
	}).Debugf("Removing finalizers from CRD object.")

	// Get the CRD object
	var unstruct *unstructured.Unstructured
	if unstruct, err = client.Resource(*gvr).Get(ctx(), crdName, getOpts); err != nil {
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
						log.Debugf("Finalizer not present on Kubernetes CRD object %v, nothing to do", crdName)
						return nil
					}
					// Found one or more finalizers, remove them all
					metadata["finalizers"] = make([]string, 0)
				} else {
					return fmt.Errorf("unexpected finalizer type %T", finalizers)
				}
			} else {
				// No finalizers present, nothing to do
				log.Debugf("No finalizer found on Kubernetes CRD object %v, nothing to do", crdName)
				return nil
			}
		}
	}
	if !removedFinalizer {
		return fmt.Errorf("could not determine finalizer metadata for CRD %v", crdName)
	}

	// Update the object with the updated finalizers
	if _, err = client.Resource(*gvr).Update(ctx(), unstruct, updateOpts); err != nil {
		return err
	}

	log.Debugf("Removed finalizers from Kubernetes CRD object %v", crdName)

	return nil
}

func (k *KubeClient) GetCRDClient() (*crdclient.Clientset, error) {
	return crdclient.NewForConfig(k.restConfig)
}
