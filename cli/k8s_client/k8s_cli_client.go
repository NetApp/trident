package k8s_client

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/utils"
)

type OrchestratorFlavor string

const (
	CLIKubernetes = "kubectl"
	CLIOpenShift  = "oc"

	FlavorKubernetes OrchestratorFlavor = "k8s"
	FlavorOpenShift  OrchestratorFlavor = "openshift"
)

type Interface interface {
	Version() *utils.Version
	Flavor() OrchestratorFlavor
	CLI() string
	Namespace() string
	SetNamespace(namespace string)
	GetCurrentNamespace() (string, error)
	Exec(pod, container string, commandArgs []string) ([]byte, error)
	GetDeploymentByLabel(label string, allNamespaces bool) (*v1beta1.Deployment, error)
	GetDeploymentsByLabel(label string, allNamespaces bool) ([]v1beta1.Deployment, error)
	CheckDeploymentExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteDeploymentByLabel(label string) error
	GetServiceByLabel(label string, allNamespaces bool) (*v1.Service, error)
	GetServicesByLabel(label string, allNamespaces bool) ([]v1.Service, error)
	CheckServiceExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteServiceByLabel(label string) error
	GetStatefulSetByLabel(label string, allNamespaces bool) (*appsv1.StatefulSet, error)
	GetStatefulSetsByLabel(label string, allNamespaces bool) ([]appsv1.StatefulSet, error)
	CheckStatefulSetExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteStatefulSetByLabel(label string) error
	GetDaemonSetByLabel(label string, allNamespaces bool) (*v1beta1.DaemonSet, error)
	GetDaemonSetsByLabel(label string, allNamespaces bool) ([]v1beta1.DaemonSet, error)
	CheckDaemonSetExistsByLabel(label string, allNamespaces bool) (bool, string, error)
	DeleteDaemonSetByLabel(label string) error
	GetPodByLabel(label string, allNamespaces bool) (*v1.Pod, error)
	GetPVC(pvcName string) (*v1.PersistentVolumeClaim, error)
	GetPVCByLabel(label string, allNamespaces bool) (*v1.PersistentVolumeClaim, error)
	CheckPVCExists(pvcName string) (bool, error)
	CheckPVCBound(pvcName string) (bool, error)
	DeletePVCByLabel(label string) error
	GetPV(pvName string) (*v1.PersistentVolume, error)
	GetPVByLabel(label string) (*v1.PersistentVolume, error)
	CheckPVExists(pvName string) (bool, error)
	DeletePVByLabel(label string) error
	CheckSecretExists(secretName string) (bool, error)
	CheckNamespaceExists(namespace string) (bool, error)
	CreateObjectByFile(filePath string) error
	CreateObjectByName(typeName, objectName string, additionalArgs []string) error
	CreateObjectByYAML(yaml string) error
	DeleteObjectByFile(filePath string, ignoreNotFound bool) error
	DeleteObjectByName(typeName, objectName string, ignoreNotFound bool) error
	DeleteObjectByYAML(yaml string, ignoreNotFound bool) error
	AddTridentUserToOpenShiftSCC() error
	RemoveTridentUserFromOpenShiftSCC() error
	ReadDeploymentFromFile(filePath string) (*v1beta1.Deployment, error)
	ReadServiceFromFile(filePath string) (*v1.Service, error)
	ReadStatefulSetFromFile(filePath string) (*appsv1.StatefulSet, error)
	ReadDaemonSetFromFile(filePath string) (*v1beta1.DaemonSet, error)
	ReadPVCFromFile(filePath string) (*v1.PersistentVolumeClaim, error)
}

type KubectlClient struct {
	cli       string
	flavor    OrchestratorFlavor
	version   *utils.Version
	namespace string
}

func NewKubectlClient() (Interface, error) {

	// Discover which CLI to use (kubectl or oc)
	cli, err := discoverKubernetesCLI()
	if err != nil {
		return nil, err
	}

	var flavor OrchestratorFlavor
	var version *utils.Version

	// Discover Kubernetes server version
	switch cli {
	default:
		fallthrough
	case CLIKubernetes:
		flavor = FlavorKubernetes
		version, err = discoverKubernetesServerVersion(cli)
	case CLIOpenShift:
		flavor = FlavorOpenShift
		version, err = discoverOpenShiftServerVersion(cli)
	}
	if err != nil {
		return nil, err
	}

	// Ensure the version is a supported one
	minSupportedVersion := utils.MustParseSemantic(tridentconfig.KubernetesVersionMin)
	maxSupportedVersion := utils.MustParseSemantic(tridentconfig.KubernetesVersionMax)
	if !version.AtLeast(minSupportedVersion) {
		return nil, fmt.Errorf("Trident requires Kubernetes %s or later", minSupportedVersion.ShortString())
	}
	mmVersion := version.ToMajorMinorVersion()
	maxSupportedMMVersion := maxSupportedVersion.ToMajorMinorVersion()
	if maxSupportedMMVersion.LessThan(mmVersion) {
		log.WithFields(log.Fields{
			"kubernetesVersion":   version.ShortString(),
			"maxSupportedVersion": maxSupportedMMVersion.String(),
		}).Warning("Trident has not been qualified with this version of Kubernetes.")
	}

	client := &KubectlClient{
		cli:     cli,
		flavor:  flavor,
		version: version,
	}

	// Get current namespace
	currentNamespace, err := client.GetCurrentNamespace()
	if err != nil {
		return nil, fmt.Errorf("could not determine current namespace; %v", err)
	}
	client.namespace = currentNamespace

	log.WithFields(log.Fields{
		"cli":       cli,
		"flavor":    flavor,
		"version":   version.String(),
		"namespace": currentNamespace,
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
	_, err = exec.Command(CLIKubernetes, "version").CombinedOutput()
	if err == nil {
		return CLIKubernetes, nil
	}

	return "", errors.New("could not find the Kubernetes CLI.")
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

	return nil, errors.New("could not get Kubernetes server version.")
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

	return nil, errors.New("could not get OpenShift server version.")
}

func (c *KubectlClient) Version() *utils.Version {
	return c.version
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

func (c *KubectlClient) GetCurrentNamespace() (string, error) {

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

func (c *KubectlClient) Exec(pod, container string, commandArgs []string) ([]byte, error) {

	// Build tunnel command to exec command in container
	execCommand := []string{
		"exec",
		pod,
		"-n", c.namespace,
		"-c", container,
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
	_, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return err
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
	_, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return err
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
	_, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return err
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

// GetDaemonSetsByLabel returns all deployment objects matching the specified label
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
	_, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"label":     label,
		"namespace": c.namespace,
	}).Debug("Deleted Kubernetes daemonset.")

	return nil
}

// GetPodByLabel returns a pod object matching the specified label
func (c *KubectlClient) GetPodByLabel(label string, allNamespaces bool) (*v1.Pod, error) {

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

	if len(podList.Items) == 1 {
		return &podList.Items[0], nil
	} else if len(podList.Items) > 1 {
		return nil, fmt.Errorf("multiple pods have the label %s", label)
	} else {
		return nil, fmt.Errorf("no pods have the label %s", label)
	}
}

func (c *KubectlClient) GetPVC(pvcName string) (*v1.PersistentVolumeClaim, error) {

	var pvc v1.PersistentVolumeClaim

	args := []string{"get", "pvc", pvcName, "--namespace", c.namespace, "-o=json"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return nil, err
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
		return false, err
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
	_, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return err
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
		return nil, err
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
		return false, err
	}
	return len(out) > 0, nil
}

func (c *KubectlClient) DeletePVByLabel(label string) error {

	cmdArgs := []string{"delete", "pv", "-l", label}
	_, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return err
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
		return false, err
	}
	return len(out) > 0, nil
}

// CheckNamespaceExists returns true if the specified namespace exists, false otherwise.
// It only returns an error if the check failed, not if the namespace doesn't exist.
func (c *KubectlClient) CheckNamespaceExists(namespace string) (bool, error) {
	args := []string{"get", "namespace", namespace, "--ignore-not-found"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}

// CreateObjectByFile creates an object from a YAML/JSON file at the specified path.
func (c *KubectlClient) CreateObjectByFile(filePath string) error {

	args := []string{
		fmt.Sprintf("--namespace=%s", c.namespace),
		"create",
		"-f",
		filePath,
	}
	_, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return err
	}

	log.WithField("path", filePath).Debug("Created Kubernetes object by file.")

	return nil
}

func (c *KubectlClient) CreateObjectByName(typeName, objectName string, additionalArgs []string) error {

	args := []string{
		fmt.Sprintf("--namespace=%s", c.namespace),
		"create",
		typeName,
		objectName,
	}
	if len(additionalArgs) > 0 {
		args = append(args, additionalArgs...)
	}

	_, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return err
	}

	log.WithField(typeName, objectName).Debug("Created Kubernetes object by name.")

	return nil
}

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

	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}

	log.Debug("Created Kubernetes object by YAML.")

	return nil
}

func (c *KubectlClient) DeleteObjectByFile(filePath string, ignoreNotFound bool) error {

	args := []string{
		fmt.Sprintf("--namespace=%s", c.namespace),
		fmt.Sprintf("--ignore-not-found=%t", ignoreNotFound),
		"delete",
		"-f",
		filePath,
	}
	_, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return err
	}

	log.WithField("path", filePath).Debug("Deleted Kubernetes object by file.")

	return nil
}

func (c *KubectlClient) DeleteObjectByName(typeName, objectName string, ignoreNotFound bool) error {

	args := []string{
		fmt.Sprintf("--namespace=%s", c.namespace),
		fmt.Sprintf("--ignore-not-found=%t", ignoreNotFound),
		"delete",
		typeName,
		objectName,
	}
	_, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return err
	}

	log.WithField(typeName, objectName).Debug("Deleted Kubernetes object by name.")

	return nil
}

func (c *KubectlClient) DeleteObjectByYAML(yaml string, ignoreNotFound bool) error {

	args := []string{
		fmt.Sprintf("--namespace=%s", c.namespace),
		fmt.Sprintf("--ignore-not-found=%t", ignoreNotFound),
		"delete",
		"-f",
		"-",
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

	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}

	log.Debug("Deleted Kubernetes object by YAML.")

	return nil
}

func (c *KubectlClient) AddTridentUserToOpenShiftSCC() error {

	if c.flavor != FlavorOpenShift {
		return errors.New("The current client context is not OpenShift.")
	}

	// This command appears to be idempotent, so no need to call isTridentUserInOpenShiftSCC() first.
	args := []string{
		fmt.Sprintf("--namespace=%s", c.namespace),
		"adm",
		"policy",
		"add-scc-to-user",
		"anyuid",
		"-z",
		"trident",
	}
	_, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

func (c *KubectlClient) RemoveTridentUserFromOpenShiftSCC() error {

	if c.flavor != FlavorOpenShift {
		return errors.New("The current client context is not OpenShift.")
	}

	// This command appears to be idempotent, so no need to call isTridentUserInOpenShiftSCC() first.
	args := []string{
		fmt.Sprintf("--namespace=%s", c.namespace),
		"adm",
		"policy",
		"remove-scc-from-user",
		"anyuid",
		"-z",
		"trident",
	}
	_, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

// ReadDeploymentFromFile parses and returns a deployment object from a file.
func (c *KubectlClient) ReadDeploymentFromFile(filePath string) (*v1beta1.Deployment, error) {

	var deployment v1beta1.Deployment

	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &deployment)
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

// ReadServiceFromFile parses and returns a service object from a file.
func (c *KubectlClient) ReadServiceFromFile(filePath string) (*v1.Service, error) {

	var service v1.Service

	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &service)
	if err != nil {
		return nil, err
	}
	return &service, nil
}

// ReadStatefulSetFromFile parses and returns a statefulset object from a file.
func (c *KubectlClient) ReadStatefulSetFromFile(filePath string) (*appsv1.StatefulSet, error) {

	var statefulset appsv1.StatefulSet

	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &statefulset)
	if err != nil {
		return nil, err
	}
	return &statefulset, nil
}

// ReadDaemonSetFromFile parses and returns a daemonset object from a file.
func (c *KubectlClient) ReadDaemonSetFromFile(filePath string) (*v1beta1.DaemonSet, error) {

	var daemonset v1beta1.DaemonSet

	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &daemonset)
	if err != nil {
		return nil, err
	}
	return &daemonset, nil
}

// ReadPVCFromFile parses and returns a PVC object from a file.
func (c *KubectlClient) ReadPVCFromFile(filePath string) (*v1.PersistentVolumeClaim, error) {

	var pvc v1.PersistentVolumeClaim

	yamlBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlBytes, &pvc)
	if err != nil {
		return nil, err
	}
	return &pvc, nil
}
