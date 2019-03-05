package k8sclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/utils"
	"k8s.io/apimachinery/pkg/version"
)

type KubectlClient struct {
	cli       string
	flavor    OrchestratorFlavor
	version   *utils.Version
	namespace string
}

func NewKubectlClient(namespace string) (Interface, error) {

	// Discover which CLI to use (kubectl or oc)
	cli, err := discoverKubernetesCLI()
	if err != nil {
		return nil, err
	}

	var flavor OrchestratorFlavor
	var k8sVersion *utils.Version

	// Discover Kubernetes server version
	switch cli {
	default:
		fallthrough
	case CLIKubernetes:
		flavor = FlavorKubernetes
		k8sVersion, err = discoverKubernetesServerVersion(cli)
	case CLIOpenShift:
		flavor = FlavorOpenShift
		k8sVersion, err = discoverOpenShiftServerVersion(cli)
	}
	if err != nil {
		return nil, err
	}

	// Ensure the version is a supported one
	minSupportedVersion := utils.MustParseSemantic(tridentconfig.KubernetesVersionMin)
	maxSupportedVersion := utils.MustParseSemantic(tridentconfig.KubernetesVersionMax)
	if !k8sVersion.AtLeast(minSupportedVersion) {
		return nil, fmt.Errorf("trident requires Kubernetes %s or later", minSupportedVersion.ShortString())
	}
	mmVersion := k8sVersion.ToMajorMinorVersion()
	maxSupportedMMVersion := maxSupportedVersion.ToMajorMinorVersion()
	if maxSupportedMMVersion.LessThan(mmVersion) {
		log.WithFields(log.Fields{
			"kubernetesVersion":   k8sVersion.ShortString(),
			"maxSupportedVersion": maxSupportedMMVersion.String(),
		}).Warning("Trident has not been qualified with this version of Kubernetes.")
	}

	client := &KubectlClient{
		cli:       cli,
		flavor:    flavor,
		version:   k8sVersion,
		namespace: namespace,
	}

	// Get current namespace if one wasn't specified
	if namespace == "" {
		client.namespace, err = client.getCurrentNamespace()
		if err != nil {
			return nil, fmt.Errorf("could not determine current namespace; %v", err)
		}
	}

	log.WithFields(log.Fields{
		"cli":       cli,
		"flavor":    flavor,
		"version":   k8sVersion.String(),
		"namespace": client.namespace,
	}).Debug("Initialized Kubernetes CLI client.")

	return client, nil
}

func discoverKubernetesCLI() (string, error) {

	// Try the OpenShift CLI first
	_, err := exec.Command(CLIOpenShift, "version").CombinedOutput()
	if err == nil {
		return CLIOpenShift, nil
	}

	// Fall back to the K8S CLI
	out, err := exec.Command(CLIKubernetes, "version").CombinedOutput()
	if err == nil {
		return CLIKubernetes, nil
	}

	return "", fmt.Errorf("could not find the Kubernetes CLI; %s", string(out))
}

func discoverKubernetesServerVersion(kubernetesCLI string) (*utils.Version, error) {

	const k8SServerVersionPrefix = "Server Version: "

	cmd := exec.Command(kubernetesCLI, "version", "--short")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, k8SServerVersionPrefix) {
			serverVersion := strings.TrimPrefix(line, k8SServerVersionPrefix)
			return utils.ParseSemantic(serverVersion)
		}
	}

	return nil, errors.New("could not get Kubernetes server version")
}

func discoverOpenShiftServerVersion(kubernetesCLI string) (*utils.Version, error) {

	cmd := exec.Command(kubernetesCLI, "version")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	inServerSection := false
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Server") {
			inServerSection = true
		} else if inServerSection {
			if strings.HasPrefix(line, "kubernetes ") {
				serverVersion := strings.TrimPrefix(line, "kubernetes ")
				return utils.ParseSemantic(serverVersion)
			}
		}
	}

	return nil, errors.New("could not get OpenShift server version")
}

func (c *KubectlClient) ServerVersion() *utils.Version {
	return c.version
}

func (c *KubectlClient) Version() *version.Info {
	serverVersion := c.ServerVersion()
	versionInfo := &version.Info{
		Major: serverVersion.MajorVersionString(),
		Minor: serverVersion.MinorVersionString(),
	}
	return versionInfo
}

func (c *KubectlClient) Flavor() OrchestratorFlavor {
	return c.flavor
}

func (c *KubectlClient) CLI() string {
	return c.cli
}

func (c *KubectlClient) Namespace() string {
	return c.namespace
}

func (c *KubectlClient) SetNamespace(namespace string) {
	c.namespace = namespace
}

func (c *KubectlClient) getCurrentNamespace() (string, error) {

	// Get current namespace from service account info
	cmd := exec.Command(c.cli, "get", "serviceaccount", "default", "-o=json")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var serviceAccount v1.ServiceAccount
	if err := json.NewDecoder(stdout).Decode(&serviceAccount); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}

	// Get Trident pod name & namespace
	namespace := serviceAccount.ObjectMeta.Namespace

	return namespace, nil
}

func (c *KubectlClient) Exec(podName, containerName string, commandArgs []string) ([]byte, error) {

	// Build tunnel command to exec command in container
	execCommand := []string{
		"exec",
		podName,
		"-n", c.namespace,
		"-c", containerName,
		"--",
	}

	// Combine tunnel and CLI commands
	execCommand = append(execCommand, commandArgs...)

	log.Debugf("Invoking tunneled command: %s %v", c.cli, strings.Join(execCommand, " "))

	// Invoke command inside the Trident pod
	return exec.Command(c.cli, execCommand...).CombinedOutput()
}

// GetDeploymentByLabel returns a deployment object matching the specified label if it is unique
func (c *KubectlClient) GetDeploymentByLabel(label string, allNamespaces bool) (*v1beta1.Deployment, error) {

	deployments, err := c.GetDeploymentsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(deployments) == 1 {
		return &deployments[0], nil
	} else if len(deployments) > 1 {
		return nil, fmt.Errorf("multiple deployments have the label %s", label)
	} else {
		return nil, fmt.Errorf("no deployments have the label %s", label)
	}
}

// GetDeploymentByLabel returns all deployment objects matching the specified label
func (c *KubectlClient) GetDeploymentsByLabel(label string, allNamespaces bool) ([]v1beta1.Deployment, error) {

	// Get deployment info
	cmdArgs := []string{"get", "deployment", "-l", label, "-o=json"}
	if allNamespaces {
		cmdArgs = append(cmdArgs, "--all-namespaces")
	} else {
		cmdArgs = append(cmdArgs, "--namespace", c.namespace)
	}
	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var deploymentList v1beta1.DeploymentList
	if err := json.NewDecoder(stdout).Decode(&deploymentList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return deploymentList.Items, nil
}

// CheckDeploymentExistsByLabel returns true if one or more deployment objects
// matching the specified label exist.
func (c *KubectlClient) CheckDeploymentExistsByLabel(label string, allNamespaces bool) (bool, string, error) {

	deployments, err := c.GetDeploymentsByLabel(label, allNamespaces)
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
func (c *KubectlClient) DeleteDeploymentByLabel(label string) error {

	cmdArgs := []string{"delete", "deployment", "-l", label, "--namespace", c.namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"label":     label,
		"namespace": c.namespace,
	}).Debug("Deleted Kubernetes deployment.")

	return nil
}

// GetServiceByLabel returns a service object matching the specified label if it is unique
func (c *KubectlClient) GetServiceByLabel(label string, allNamespaces bool) (*v1.Service, error) {

	services, err := c.GetServicesByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(services) == 1 {
		return &services[0], nil
	} else if len(services) > 1 {
		return nil, fmt.Errorf("multiple services have the label %s", label)
	} else {
		return nil, fmt.Errorf("no services have the label %s", label)
	}
}

// GetServicesByLabel returns all service objects matching the specified label
func (c *KubectlClient) GetServicesByLabel(label string, allNamespaces bool) ([]v1.Service, error) {

	// Get service info
	cmdArgs := []string{"get", "service", "-l", label, "-o=json"}
	if allNamespaces {
		cmdArgs = append(cmdArgs, "--all-namespaces")
	} else {
		cmdArgs = append(cmdArgs, "--namespace", c.namespace)
	}
	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var serviceList v1.ServiceList
	if err := json.NewDecoder(stdout).Decode(&serviceList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return serviceList.Items, nil
}

// CheckServiceExistsByLabel returns true if one or more service objects
// matching the specified label exist.
func (c *KubectlClient) CheckServiceExistsByLabel(label string, allNamespaces bool) (bool, string, error) {

	services, err := c.GetServicesByLabel(label, allNamespaces)
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
func (c *KubectlClient) DeleteServiceByLabel(label string) error {

	cmdArgs := []string{"delete", "service", "-l", label, "--namespace", c.namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"label":     label,
		"namespace": c.namespace,
	}).Debug("Deleted Kubernetes service.")

	return nil
}

// GetStatefulSetByLabel returns a statefulset object matching the specified label if it is unique
func (c *KubectlClient) GetStatefulSetByLabel(label string, allNamespaces bool) (*appsv1.StatefulSet, error) {

	statefulsets, err := c.GetStatefulSetsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(statefulsets) == 1 {
		return &statefulsets[0], nil
	} else if len(statefulsets) > 1 {
		return nil, fmt.Errorf("multiple statefulsets have the label %s", label)
	} else {
		return nil, fmt.Errorf("no statefulsets have the label %s", label)
	}
}

// GetStatefulSetsByLabel returns all stateful objects matching the specified label
func (c *KubectlClient) GetStatefulSetsByLabel(label string, allNamespaces bool) ([]appsv1.StatefulSet, error) {

	// Get statefulset info
	cmdArgs := []string{"get", "statefulset", "-l", label, "-o=json"}
	if allNamespaces {
		cmdArgs = append(cmdArgs, "--all-namespaces")
	} else {
		cmdArgs = append(cmdArgs, "--namespace", c.namespace)
	}
	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var statefulsetList appsv1.StatefulSetList
	if err := json.NewDecoder(stdout).Decode(&statefulsetList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return statefulsetList.Items, nil
}

// CheckStatefulSetExistsByLabel returns true if one or more statefulset objects
// matching the specified label exist.
func (c *KubectlClient) CheckStatefulSetExistsByLabel(label string, allNamespaces bool) (bool, string, error) {

	statefulsets, err := c.GetStatefulSetsByLabel(label, allNamespaces)
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
func (c *KubectlClient) DeleteStatefulSetByLabel(label string) error {

	cmdArgs := []string{"delete", "statefulset", "-l", label, "--namespace", c.namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"label":     label,
		"namespace": c.namespace,
	}).Debug("Deleted Kubernetes statefulset.")

	return nil
}

// GetDaemonSetByLabel returns a daemonset object matching the specified label if it is unique
func (c *KubectlClient) GetDaemonSetByLabel(label string, allNamespaces bool) (*v1beta1.DaemonSet, error) {

	daemonsets, err := c.GetDaemonSetsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(daemonsets) == 1 {
		return &daemonsets[0], nil
	} else if len(daemonsets) > 1 {
		return nil, fmt.Errorf("multiple daemonsets have the label %s", label)
	} else {
		return nil, fmt.Errorf("no daemonsets have the label %s", label)
	}
}

// GetDaemonSetsByLabel returns all daemonset objects matching the specified label
func (c *KubectlClient) GetDaemonSetsByLabel(label string, allNamespaces bool) ([]v1beta1.DaemonSet, error) {

	// Get daemonset info
	cmdArgs := []string{"get", "daemonset", "-l", label, "-o=json"}
	if allNamespaces {
		cmdArgs = append(cmdArgs, "--all-namespaces")
	} else {
		cmdArgs = append(cmdArgs, "--namespace", c.namespace)
	}
	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var daemonsetList v1beta1.DaemonSetList
	if err := json.NewDecoder(stdout).Decode(&daemonsetList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return daemonsetList.Items, nil
}

// CheckDaemonSetExistsByLabel returns true if one or more daemonset objects
// matching the specified label exist.
func (c *KubectlClient) CheckDaemonSetExistsByLabel(label string, allNamespaces bool) (bool, string, error) {

	daemonsets, err := c.GetDaemonSetsByLabel(label, allNamespaces)
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
func (c *KubectlClient) DeleteDaemonSetByLabel(label string) error {

	cmdArgs := []string{"delete", "daemonset", "-l", label, "--namespace", c.namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"label":     label,
		"namespace": c.namespace,
	}).Debug("Deleted Kubernetes daemonset.")

	return nil
}

// GetConfigMapByLabel returns a configmap object matching the specified label if it is unique
func (c *KubectlClient) GetConfigMapByLabel(label string, allNamespaces bool) (*v1.ConfigMap, error) {

	configmaps, err := c.GetConfigMapsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(configmaps) == 1 {
		return &configmaps[0], nil
	} else if len(configmaps) > 1 {
		return nil, fmt.Errorf("multiple configmaps have the label %s", label)
	} else {
		return nil, fmt.Errorf("no configmaps have the label %s", label)
	}
}

// GetConfigMapsByLabel returns all configmap objects matching the specified label
func (c *KubectlClient) GetConfigMapsByLabel(label string, allNamespaces bool) ([]v1.ConfigMap, error) {

	// Get configmap info
	cmdArgs := []string{"get", "configmap", "-l", label, "-o=json"}
	if allNamespaces {
		cmdArgs = append(cmdArgs, "--all-namespaces")
	} else {
		cmdArgs = append(cmdArgs, "--namespace", c.namespace)
	}
	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var configMapList v1.ConfigMapList
	if err := json.NewDecoder(stdout).Decode(&configMapList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return configMapList.Items, nil
}

// CheckConfigMapExistsByLabel returns true if one or more configmap objects
// matching the specified label exist.
func (c *KubectlClient) CheckConfigMapExistsByLabel(label string, allNamespaces bool) (bool, string, error) {

	configmaps, err := c.GetConfigMapsByLabel(label, allNamespaces)
	if err != nil {
		return false, "", err
	}

	switch len(configmaps) {
	case 0:
		return false, "", nil
	case 1:
		return true, configmaps[0].Namespace, nil
	default:
		return true, "<multiple>", nil
	}
}

// DeleteConfigMapByLabel deletes a configmap object matching the specified label
func (c *KubectlClient) DeleteConfigMapByLabel(label string) error {

	cmdArgs := []string{"delete", "configmap", "-l", label, "--namespace", c.namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"label":     label,
		"namespace": c.namespace,
	}).Debug("Deleted Kubernetes configmap.")

	return nil
}

func (c *KubectlClient) CreateConfigMapFromDirectory(path, name, label string) error {

	cmdArgs := []string{"create", "configmap", name, "--from-file", path, "--namespace", c.namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	if label != "" {
		cmdArgs = []string{"label", "configmap", name, "--namespace", c.namespace, label}
		out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s; %v", string(out), err)
		}
	}

	log.WithFields(log.Fields{
		"label":     label,
		"name":      name,
		"path":      path,
		"namespace": c.namespace,
	}).Debug("Created Kubernetes configmap from directory.")

	return nil
}

// GetPodByLabel returns a pod object matching the specified label
func (c *KubectlClient) GetPodByLabel(label string, allNamespaces bool) (*v1.Pod, error) {

	// Get pod info
	pods, err := c.GetPodsByLabel(label, allNamespaces)
	if err != nil {
		return nil, err
	}

	if len(pods) == 1 {
		return &pods[0], nil
	} else if len(pods) > 1 {
		return nil, fmt.Errorf("multiple pods have the label %s", label)
	} else {
		return nil, fmt.Errorf("no pods have the label %s", label)
	}
}

// GetPodsByLabel returns all pod objects matching the specified label
func (c *KubectlClient) GetPodsByLabel(label string, allNamespaces bool) ([]v1.Pod, error) {

	// Get pod info
	cmdArgs := []string{"get", "pod", "-l", label, "-o=json"}
	if allNamespaces {
		cmdArgs = append(cmdArgs, "--all-namespaces")
	} else {
		cmdArgs = append(cmdArgs, "--namespace", c.namespace)
	}
	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var podList v1.PodList
	if err := json.NewDecoder(stdout).Decode(&podList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return podList.Items, nil
}

// CheckPodExistsByLabel returns true if one or more pod objects
// matching the specified label exist.
func (c *KubectlClient) CheckPodExistsByLabel(label string, allNamespaces bool) (bool, string, error) {

	pods, err := c.GetPodsByLabel(label, allNamespaces)
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
func (c *KubectlClient) DeletePodByLabel(label string) error {

	cmdArgs := []string{"delete", "pod", "-l", label, "--namespace", c.namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"label":     label,
		"namespace": c.namespace,
	}).Debug("Deleted Kubernetes pod.")

	return nil
}

func (c *KubectlClient) GetPVC(pvcName string) (*v1.PersistentVolumeClaim, error) {

	var pvc v1.PersistentVolumeClaim

	args := []string{"get", "pvc", pvcName, "--namespace", c.namespace, "-o=json"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s; %v", string(out), err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("PVC %s does not exist in namespace %s", pvcName, c.namespace)
	}

	err = yaml.Unmarshal(out, &pvc)
	if err != nil {
		return nil, err
	}
	return &pvc, nil
}

func (c *KubectlClient) GetPVCByLabel(label string, allNamespaces bool) (*v1.PersistentVolumeClaim, error) {

	// Get PVC info
	cmdArgs := []string{"get", "pvc", "-l", label, "-o=json"}
	if allNamespaces {
		cmdArgs = append(cmdArgs, "--all-namespaces")
	} else {
		cmdArgs = append(cmdArgs, "--namespace", c.namespace)
	}
	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var pvcList v1.PersistentVolumeClaimList
	if err := json.NewDecoder(stdout).Decode(&pvcList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	if len(pvcList.Items) == 1 {
		return &pvcList.Items[0], nil
	} else if len(pvcList.Items) > 1 {
		return nil, fmt.Errorf("multiple PVCs have the label %s", label)
	} else {
		return nil, fmt.Errorf("no PVCs have the label %s", label)
	}
}

// CheckPVCExists returns true if the specified PVC exists, false otherwise.
// It only returns an error if the check failed, not if the PVC doesn't exist.
func (c *KubectlClient) CheckPVCExists(pvcName string) (bool, error) {
	args := []string{"get", "pvc", pvcName, "--namespace", c.namespace, "--ignore-not-found"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%s; %v", string(out), err)
	}
	return len(out) > 0, nil
}

// CheckPVCBound returns true if the specified PVC is bound, false otherwise.
// It only returns an error if the check failed, not if the PVC doesn't exist.
func (c *KubectlClient) CheckPVCBound(pvcName string) (bool, error) {

	pvc, err := c.GetPVC(pvcName)
	if err != nil {
		return false, err
	}

	return pvc.Status.Phase == v1.ClaimBound, nil
}

// DeletePVCByLabel deletes a PVC object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) DeletePVCByLabel(label string) error {

	cmdArgs := []string{"delete", "pvc", "-l", label, "--namespace", c.namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"label":     label,
		"namespace": c.namespace,
	}).Debug("Deleted PVC by label.")

	return nil
}

func (c *KubectlClient) GetPV(pvName string) (*v1.PersistentVolume, error) {

	var pv v1.PersistentVolume

	args := []string{"get", "pv", pvName, "-o=json"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s; %v", string(out), err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("PV %s does not exist", pvName)
	}

	err = yaml.Unmarshal(out, &pv)
	if err != nil {
		return nil, err
	}
	return &pv, nil
}

func (c *KubectlClient) GetPVByLabel(label string) (*v1.PersistentVolume, error) {

	// Get PV info
	cmdArgs := []string{"get", "pv", "-l", label, "-o=json"}
	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var pvList v1.PersistentVolumeList
	if err := json.NewDecoder(stdout).Decode(&pvList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	if len(pvList.Items) == 1 {
		return &pvList.Items[0], nil
	} else if len(pvList.Items) > 1 {
		return nil, fmt.Errorf("multiple PVs have the label %s", label)
	} else {
		return nil, fmt.Errorf("no PVs have the label %s", label)
	}
}

// CheckPVExists returns true if the specified PV exists, false otherwise.
// It only returns an error if the check failed, not if the PV doesn't exist.
func (c *KubectlClient) CheckPVExists(pvName string) (bool, error) {
	args := []string{"get", "pv", pvName, "--ignore-not-found"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%s; %v", string(out), err)
	}
	return len(out) > 0, nil
}

func (c *KubectlClient) DeletePVByLabel(label string) error {

	cmdArgs := []string{"delete", "pv", "-l", label}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithField("label", label).Debug("Deleted PV by label.")

	return nil
}

// CheckSecretExists returns true if the specified secret exists, false otherwise.
// It only returns an error if the check failed, not if the secret doesn't exist.
func (c *KubectlClient) CheckSecretExists(secretName string) (bool, error) {
	args := []string{"get", "secret", secretName, "--namespace", c.namespace, "--ignore-not-found"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%s; %v", string(out), err)
	}
	return len(out) > 0, nil
}

// CreateSecret creates a new Secret
func (c *KubectlClient) CreateSecret(secret *v1.Secret) (*v1.Secret, error) {

	// Convert to YAML
	jsonBytes, err := json.Marshal(secret)
	if err != nil {
		return nil, err
	}
	yamlBytes, _ := yaml.JSONToYAML(jsonBytes)

	// Create object
	err = c.CreateObjectByYAML(string(yamlBytes))
	if err != nil {
		return nil, err
	}

	// Get secret
	return c.GetSecret(secret.Name)
}

// CreateCHAPSecret creates a new Secret for iSCSI CHAP mutual authentication
func (c *KubectlClient) CreateCHAPSecret(secretName, accountName, initiatorSecret, targetSecret string,
) (*v1.Secret, error) {
	return c.CreateSecret(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.namespace,
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
func (c *KubectlClient) GetSecret(secretName string) (*v1.Secret, error) {

	cmdArgs := []string{"get", "secret", secretName, "--namespace", c.namespace, "-o=json"}
	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var createdSecret v1.Secret
	if err := json.NewDecoder(stdout).Decode(&createdSecret); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return &createdSecret, nil
}

func (c *KubectlClient) GetSecretByLabel(label string, allNamespaces bool) (*v1.Secret, error) {

	// Get secret info
	cmdArgs := []string{"get", "secret", "-l", label, "-o=json"}
	if allNamespaces {
		cmdArgs = append(cmdArgs, "--all-namespaces")
	} else {
		cmdArgs = append(cmdArgs, "--namespace", c.namespace)
	}
	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var secretList v1.SecretList
	if err := json.NewDecoder(stdout).Decode(&secretList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	if len(secretList.Items) == 1 {
		return &secretList.Items[0], nil
	} else if len(secretList.Items) > 1 {
		return nil, fmt.Errorf("multiple secrets have the label %s", label)
	} else {
		return nil, fmt.Errorf("no secrets have the label %s", label)
	}
}

// DeleteSecret deletes the specified Secret
func (c *KubectlClient) DeleteSecret(secretName string) error {

	cmdArgs := []string{"delete", "secret", secretName, "--namespace", c.namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}
	return nil
}

// DeleteSecretByLabel deletes a secret object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) DeleteSecretByLabel(label string) error {

	cmdArgs := []string{"delete", "secret", "-l", label, "--namespace", c.namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"label":     label,
		"namespace": c.namespace,
	}).Debug("Deleted secret by label.")

	return nil
}

// CheckNamespaceExists returns true if the specified namespace exists, false otherwise.
// It only returns an error if the check failed, not if the namespace doesn't exist.
func (c *KubectlClient) CheckNamespaceExists(namespace string) (bool, error) {
	args := []string{"get", "namespace", namespace, "--ignore-not-found"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%s; %v", string(out), err)
	}
	return len(out) > 0, nil
}

// CreateObjectByFile creates an object on the server from a YAML/JSON file at the specified path.
func (c *KubectlClient) CreateObjectByFile(filePath string) error {

	args := []string{
		fmt.Sprintf("--namespace=%s", c.namespace),
		"create",
		"-f",
		filePath,
	}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithField("path", filePath).Debug("Created Kubernetes object by file.")

	return nil
}

// CreateObjectByYAML creates an object on the server from a YAML/JSON document.
func (c *KubectlClient) CreateObjectByYAML(yaml string) error {

	args := []string{fmt.Sprintf("--namespace=%s", c.namespace), "create", "-f", "-"}
	cmd := exec.Command(c.cli, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		stdin.Write([]byte(yaml))
	}()

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.Debug("Created Kubernetes object by YAML.")

	return nil
}

// DeleteObjectByFile deletes an object on the server from a YAML/JSON file at the specified path.
func (c *KubectlClient) DeleteObjectByFile(filePath string, ignoreNotFound bool) error {

	args := []string{
		"delete",
		"-f",
		filePath,
		fmt.Sprintf("--namespace=%s", c.namespace),
		fmt.Sprintf("--ignore-not-found=%t", ignoreNotFound),
	}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithField("path", filePath).Debug("Deleted Kubernetes object by file.")

	return nil
}

// DeleteObjectByYAML deletes an object on the server from a YAML/JSON document.
func (c *KubectlClient) DeleteObjectByYAML(yaml string, ignoreNotFound bool) error {

	args := []string{
		"delete",
		"-f",
		"-",
		fmt.Sprintf("--namespace=%s", c.namespace),
		fmt.Sprintf("--ignore-not-found=%t", ignoreNotFound),
	}
	cmd := exec.Command(c.cli, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		stdin.Write([]byte(yaml))
	}()

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.Debug("Deleted Kubernetes object by YAML.")

	return nil
}

// AddTridentUserToOpenShiftSCC adds the specified user (typically a service account) to the 'anyuid'
// security context constraint. This only works for OpenShift.
func (c *KubectlClient) AddTridentUserToOpenShiftSCC(user, scc string) error {

	if c.flavor != FlavorOpenShift {
		return errors.New("the current client context is not OpenShift")
	}

	// This command appears to be idempotent, so no need to call isTridentUserInOpenShiftSCC() first.
	args := []string{
		fmt.Sprintf("--namespace=%s", c.namespace),
		"adm",
		"policy",
		"add-scc-to-user",
		scc,
		"-z",
		user,
	}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}
	return nil
}

// RemoveTridentUserFromOpenShiftSCC removes the specified user (typically a service account) from the 'anyuid'
// security context constraint. This only works for OpenShift.
func (c *KubectlClient) RemoveTridentUserFromOpenShiftSCC(user, scc string) error {

	if c.flavor != FlavorOpenShift {
		return errors.New("the current client context is not OpenShift")
	}

	// This command appears to be idempotent, so no need to call isTridentUserInOpenShiftSCC() first.
	args := []string{
		fmt.Sprintf("--namespace=%s", c.namespace),
		"adm",
		"policy",
		"remove-scc-from-user",
		scc,
		"-z",
		user,
	}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}
	return nil
}
