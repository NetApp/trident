package k8sclient

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"k8s.io/api/policy/v1beta1"
	v13 "k8s.io/api/rbac/v1"
	v1beta12 "k8s.io/api/storage/v1beta1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/cenkalti/backoff/v4"
	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/clientcmd"

	crdclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/utils"
)

type KubectlClient struct {
	cli       string
	flavor    OrchestratorFlavor
	version   *utils.Version
	namespace string
	timeout   time.Duration
}

type Version struct {
	ClientVersion struct {
		Major        string    `json:"major"`
		Minor        string    `json:"minor"`
		GitVersion   string    `json:"gitVersion"`
		GitCommit    string    `json:"gitCommit"`
		GitTreeState string    `json:"gitTreeState"`
		BuildDate    time.Time `json:"buildDate"`
		GoVersion    string    `json:"goVersion"`
		Compiler     string    `json:"compiler"`
		Platform     string    `json:"platform"`
	} `json:"clientVersion"`
	ServerVersion struct {
		Major        string    `json:"major"`
		Minor        string    `json:"minor"`
		GitVersion   string    `json:"gitVersion"`
		GitCommit    string    `json:"gitCommit"`
		GitTreeState string    `json:"gitTreeState"`
		BuildDate    time.Time `json:"buildDate"`
		GoVersion    string    `json:"goVersion"`
		Compiler     string    `json:"compiler"`
		Platform     string    `json:"platform"`
	} `json:"serverVersion"`
	OpenshiftVersion string `json:"openshiftVersion"`
}

func NewKubectlClient(namespace string, k8sTimeout time.Duration) (Interface, error) {

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
		k8sVersion, err = discoverKubernetesServerVersion(cli)
		if err != nil {
			k8sVersion, err = discoverOpenShift3ServerVersion(cli)
		}
	}
	if err != nil {
		return nil, err
	}

	client := &KubectlClient{
		cli:       cli,
		flavor:    flavor,
		version:   k8sVersion,
		namespace: namespace,
		timeout:   k8sTimeout,
	}

	// Get current namespace if one wasn't specified
	if namespace == "" {
		client.namespace, err = client.getCurrentNamespace()
		if err != nil {
			return nil, fmt.Errorf("could not determine current namespace; %v", err)
		}
	}

	log.WithFields(log.Fields{
		"cli":       client.cli,
		"flavor":    client.flavor,
		"version":   client.ServerVersion().String(),
		"timeout":   client.timeout,
		"namespace": client.namespace,
	}).Debug("Initialized Kubernetes CLI client.")

	return client, nil
}

func discoverKubernetesCLI() (string, error) {

	// Try the OpenShift CLI first
	_, err := exec.Command(CLIOpenShift, "version").CombinedOutput()
	if err == nil {
		if verifyOpenShiftAPIResources() {
			return CLIOpenShift, nil
		}
	}

	// Fall back to the K8S CLI
	out, err := exec.Command(CLIKubernetes, "version").CombinedOutput()
	if err == nil {
		return CLIKubernetes, nil
	}

	return "", fmt.Errorf("could not find the Kubernetes CLI; %s", string(out))
}

func verifyOpenShiftAPIResources() bool {

	out, err := exec.Command("oc", "api-resources").CombinedOutput()
	if err != nil {
		return false
	}

	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		if strings.Contains(l, "config.openshift.io") {
			return true
		}
	}

	log.Debug("Couldn't find OpenShift api-resources, hence not using oc tools for CLI")
	return false
}

func discoverKubernetesServerVersion(kubernetesCLI string) (*utils.Version, error) {

	cmd := exec.Command(kubernetesCLI, "version", "-o", "json")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	versionBytes, err := ioutil.ReadAll(stdout)
	if err != nil {
		return nil, fmt.Errorf("could not read version data from stdout; %v", err)
	}

	var cliVersion Version
	err = json.Unmarshal(versionBytes, &cliVersion)
	if err != nil {
		return nil, fmt.Errorf("could not parse version data: %s; %v", string(versionBytes), err)
	}

	return utils.ParseSemantic(cliVersion.ServerVersion.GitVersion)
}

func discoverOpenShift3ServerVersion(kubernetesCLI string) (*utils.Version, error) {

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
func (c *KubectlClient) GetDeploymentByLabel(label string, allNamespaces bool) (*appsv1.Deployment, error) {

	deployments, err := c.GetDeploymentsByLabel(label, allNamespaces)
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
func (c *KubectlClient) GetDeploymentsByLabel(label string, allNamespaces bool) ([]appsv1.Deployment, error) {

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

	var deploymentList appsv1.DeploymentList
	if err := json.NewDecoder(stdout).Decode(&deploymentList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return deploymentList.Items, nil
}

// CheckDeploymentExists returns true if the specified deployment exists.
func (c *KubectlClient) CheckDeploymentExists(name, namespace string) (bool, error) {

	var deployment appsv1.Deployment

	args := []string{"get", "deployment", name, "--namespace", namespace, "-o=json"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%s; %v", string(out), err)
	}
	if len(out) == 0 {
		return false, nil
	}

	err = yaml.Unmarshal(out, &deployment)
	if err != nil {
		return false, err
	}
	return true, nil
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

// DeleteDeployment deletes a deployment object matching the specified name and namespace.
func (c *KubectlClient) DeleteDeployment(name, namespace string, foreground bool) error {
	if !foreground {
		return c.deleteDeploymentBackground(name, namespace)
	}
	return c.deleteDeploymentForeground(name, namespace)
}

// deleteDeploymentBackground deletes a deployment object matching the specified name and namespace.
func (c *KubectlClient) deleteDeploymentBackground(name, namespace string) error {

	cmdArgs := []string{"delete", "deployment", name, "--namespace", namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Debug("Deleted Kubernetes deployment.")

	return nil
}

// deleteDeploymentForeground deletes a deployment object matching the specified name and namespace.  Note that
// kubectl does not support specifying a foreground propagation policy, but this method uses --wait and --grace-period
// to at least ensure the deployment is deleted synchronously and finalizers are honored.
func (c *KubectlClient) deleteDeploymentForeground(name, namespace string) error {

	cmdArgs := []string{
		"delete", "deployment", name, "--namespace", namespace,
		"--wait", "--grace-period", strconv.Itoa(int(c.timeout.Seconds())),
	}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Debug("Deleted Kubernetes deployment.")

	return nil
}

// PatchDeploymentByLabel patches a deployment object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) PatchDeploymentByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	return c.PatchObjectByLabel(label, c.namespace, "deployment", "Deployment", patchBytes, patchType)
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
		return nil, utils.NotFoundError(fmt.Sprintf("no services have the label %s", label))
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

// DeleteService deletes a Service object matching the specified name and namespace
func (c *KubectlClient) DeleteService(name, namespace string) error {

	cmdArgs := []string{"delete", "service", name, "--namespace", namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Debug("Deleted Kubernetes service.")

	return nil
}

// PatchServiceByLabel patches a deployment object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) PatchServiceByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	return c.PatchObjectByLabel(label, c.namespace, "service", "Service", patchBytes, patchType)
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
		return nil, utils.NotFoundError(fmt.Sprintf("no statefulsets have the label %s", label))
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

// DeleteStatefulSet deletes a statefulset object matching the specified name and namespace.
func (c *KubectlClient) DeleteStatefulSet(name, namespace string) error {

	cmdArgs := []string{"delete", "statefulset", name, "--namespace", namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Debug("Deleted Kubernetes statefulset.")

	return nil
}

// GetDaemonSetByLabel returns a daemonset object matching the specified label if it is unique
func (c *KubectlClient) GetDaemonSetByLabel(label string, allNamespaces bool) (*appsv1.DaemonSet, error) {

	daemonsets, err := c.GetDaemonSetsByLabel(label, allNamespaces)
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
func (c *KubectlClient) GetDaemonSetsByLabel(label string, allNamespaces bool) ([]appsv1.DaemonSet, error) {

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

	var daemonsetList appsv1.DaemonSetList
	if err := json.NewDecoder(stdout).Decode(&daemonsetList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return daemonsetList.Items, nil
}

// CheckDaemonSetExists returns true if the specified daemonset exists.
func (c *KubectlClient) CheckDaemonSetExists(name, namespace string) (bool, error) {

	var daemonset appsv1.DaemonSet

	args := []string{"get", "daemonset", name, "--namespace", namespace, "-o=json"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%s; %v", string(out), err)
	}
	if len(out) == 0 {
		return false, nil
	}

	err = yaml.Unmarshal(out, &daemonset)
	if err != nil {
		return false, err
	}
	return true, nil
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

// DeleteDaemonSet deletes a daemonset object matching the specified name and namespace.
func (c *KubectlClient) DeleteDaemonSet(name, namespace string, foreground bool) error {
	if !foreground {
		return c.deleteDaemonSetBackground(name, namespace)
	}
	return c.deleteDaemonSetForeground(name, namespace)
}

// deleteDaemonSetBackground deletes a DaemonSet object matching the specified name and namespace.
func (c *KubectlClient) deleteDaemonSetBackground(name, namespace string) error {

	cmdArgs := []string{"delete", "daemonset", name, "--namespace", namespace}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Debug("Deleted Kubernetes daemonset.")

	return nil
}

// deleteDaemonSetForeground deletes a daemonset object matching the specified name and namespace.  Note that
// kubectl does not support specifying a foreground propagation policy, but this method uses --wait and --grace-period
// to at least ensure the daemonset is deleted synchronously and finalizers are honored.
func (c *KubectlClient) deleteDaemonSetForeground(name, namespace string) error {

	cmdArgs := []string{
		"delete", "daemonset", name, "--namespace", namespace,
		"--wait", "--grace-period", strconv.Itoa(int(c.timeout.Seconds())),
	}
	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Debug("Deleted Kubernetes daemonset.")

	return nil
}

// PatchDaemonSetByLabel patches a DaemonSet object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) PatchDaemonSetByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	return c.PatchObjectByLabel(label, c.namespace, "daemonset", "Daemonset", patchBytes, patchType)
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
		return nil, utils.NotFoundError(fmt.Sprintf("no configmaps have the label %s", label))
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
		return nil, utils.NotFoundError(fmt.Sprintf("no pods have the label %s", label))
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

// DeletePod deletes a pod object matching the specified name and namespace
func (c *KubectlClient) DeletePod(name, namespace string) error {
	return c.DeleteObject(name, namespace, "pod", "Pod")
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

func (c *KubectlClient) GetCRD(crdName string) (*apiextensionv1beta1.CustomResourceDefinition, error) {

	var crd apiextensionv1beta1.CustomResourceDefinition

	args := []string{"get", "crd", crdName, "-o=json"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s; %v", string(out), err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("CRD %s does not exist", crdName)
	}

	err = yaml.Unmarshal(out, &crd)
	if err != nil {
		return nil, err
	}
	return &crd, nil
}

func (c *KubectlClient) CheckCRDExists(crdName string) (bool, error) {
	args := []string{"get", "crd", crdName, "--ignore-not-found"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%s; %v", string(out), err)
	}
	return len(out) > 0, nil
}

func (c *KubectlClient) DeleteCRD(crdName string) error {
	// kubectl delete crd tridentversions.trident.netapp.io --wait=false
	args := []string{"delete", "crd", crdName, "--wait=false"}
	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithField("CRD", crdName).Debug("Deleted CRD.")

	return nil
}

// GetPodSecurityPolicyByLabel returns a pod security policy object matching the specified label if it is unique
func (c *KubectlClient) GetPodSecurityPolicyByLabel(label string) (*v1beta1.PodSecurityPolicy, error) {

	pspList, err := c.GetPodSecurityPoliciesByLabel(label)
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
func (c *KubectlClient) GetPodSecurityPoliciesByLabel(label string) ([]v1beta1.PodSecurityPolicy,
	error) {

	// Get pod security policy info
	cmdArgs := []string{"get", "psp", "-l", label, "-o=json"}

	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var pspList v1beta1.PodSecurityPolicyList
	if err := json.NewDecoder(stdout).Decode(&pspList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return pspList.Items, nil
}

// CheckPodSecurityPolicyExistsByLabel returns true if one or more pod security policy objects
// matching the specified label exist.
func (c *KubectlClient) CheckPodSecurityPolicyExistsByLabel(label string) (bool, string, error) {

	pspList, err := c.GetPodSecurityPoliciesByLabel(label)
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
func (c *KubectlClient) DeletePodSecurityPolicyByLabel(label string) error {
	return c.DeleteObjectByLabel(label, "", "psp", "Pod Security Policy")
}

// DeletePodSecurityPolicy deletes a pod security policy object matching the specified PSP name.
func (c *KubectlClient) DeletePodSecurityPolicy(pspName string) error {
	return c.DeleteObject(pspName, "", "psp", "Pod Security Policy")
}

// PatchPodSecurityPolicyByLabel patches a pod security policy object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) PatchPodSecurityPolicyByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	return c.PatchObjectByLabel(label, "", "psp", "Pod Security Policy", patchBytes, patchType)
}

// GetServiceAccountByLabel returns a service account object matching the specified label if it is unique
func (c *KubectlClient) GetServiceAccountByLabel(label string, allNamespaces bool) (*v1.ServiceAccount, error) {

	serviceAccounts, err := c.GetServiceAccountsByLabel(label, allNamespaces)
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
func (c *KubectlClient) GetServiceAccountsByLabel(label string, allNamespaces bool) ([]v1.ServiceAccount, error) {

	// Get service account info
	cmdArgs := []string{"get", "serviceaccount", "-l", label, "-o=json"}
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

	var ServiceAccountList v1.ServiceAccountList
	if err := json.NewDecoder(stdout).Decode(&ServiceAccountList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return ServiceAccountList.Items, nil
}

// CheckServiceAccountExistsByLabel returns true if one or more service account objects
// matching the specified label exist.
func (c *KubectlClient) CheckServiceAccountExistsByLabel(label string, allNamespaces bool) (bool, string, error) {

	serviceAccounts, err := c.GetServiceAccountsByLabel(label, allNamespaces)
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
func (c *KubectlClient) DeleteServiceAccountByLabel(label string) error {
	return c.DeleteObjectByLabel(label, c.namespace, "serviceaccount", "Service Account")
}

// DeleteServiceAccount deletes a service account object matching the specified
// name and namespace.
func (c *KubectlClient) DeleteServiceAccount(name, namespace string) error {
	return c.DeleteObject(name, namespace, "serviceaccount", "Service Account")
}

// PatchServiceAccountByLabel patches a Service Account object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) PatchServiceAccountByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	return c.PatchObjectByLabel(label, c.namespace, "serviceaccount", "Service Account", patchBytes, patchType)
}

// GetClusterRoleByLabel returns a cluster role object matching the specified label if it is unique
func (c *KubectlClient) GetClusterRoleByLabel(label string) (*v13.ClusterRole, error) {

	clusterRoles, err := c.GetClusterRolesByLabel(label)
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
func (c *KubectlClient) GetClusterRolesByLabel(label string) ([]v13.ClusterRole, error) {

	// Get clusterrole info
	cmdArgs := []string{"get", "clusterrole", "-l", label, "-o=json"}

	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var clusterRoleList v13.ClusterRoleList
	if err := json.NewDecoder(stdout).Decode(&clusterRoleList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return clusterRoleList.Items, nil
}

// CheckClusterRoleExistsByLabel returns true if one or more cluster role objects
// matching the specified label exist.
func (c *KubectlClient) CheckClusterRoleExistsByLabel(label string) (bool, string, error) {

	clusterRoles, err := c.GetClusterRolesByLabel(label)
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
func (c *KubectlClient) DeleteClusterRoleByLabel(label string) error {
	return c.DeleteObjectByLabel(label, "", "clusterrole", "Cluster Role")
}

// DeleteClusterRole deletes a cluster role object matching the specified name
func (c *KubectlClient) DeleteClusterRole(name string) error {
	return c.DeleteObject(name, "", "clusterrole", "Cluster Role")
}

// PatchClusterRoleByLabel patches a Cluster Role object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) PatchClusterRoleByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	return c.PatchObjectByLabel(label, "", "clusterrole", "Cluster Role", patchBytes, patchType)
}

// GetClusterRoleBindingByLabel returns a cluster role binding object matching the specified label if it is unique
func (c *KubectlClient) GetClusterRoleBindingByLabel(label string) (*v13.ClusterRoleBinding, error) {

	clusterRoleBindings, err := c.GetClusterRoleBindingsByLabel(label)
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
func (c *KubectlClient) GetClusterRoleBindingsByLabel(label string) ([]v13.ClusterRoleBinding, error) {

	// Get clusterrolebinding info
	cmdArgs := []string{"get", "clusterrolebinding", "-l", label, "-o=json"}

	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var clusterRoleBindingList v13.ClusterRoleBindingList
	if err := json.NewDecoder(stdout).Decode(&clusterRoleBindingList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return clusterRoleBindingList.Items, nil
}

// CheckClusterRoleBindingExistsByLabel returns true if one or more cluster role binding objects
// matching the specified label exist.
func (c *KubectlClient) CheckClusterRoleBindingExistsByLabel(label string) (bool, string, error) {

	clusterRoleBindings, err := c.GetClusterRoleBindingsByLabel(label)
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
func (c *KubectlClient) DeleteClusterRoleBindingByLabel(label string) error {
	return c.DeleteObjectByLabel(label, "", "clusterrolebinding", "Cluster Role Binding")
}

// DeleteClusterRoleBinding deletes a cluster role binding object matching the specified name
func (c *KubectlClient) DeleteClusterRoleBinding(name string) error {
	return c.DeleteObject(name, "", "clusterrolebinding", "Cluster Role Binding")
}

// PatchClusterRoleBindingByLabel patches a Cluster Role binding object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) PatchClusterRoleBindingByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	return c.PatchObjectByLabel(label, "", "clusterrolebinding", "Cluster Role Binding", patchBytes, patchType)
}

// GetCSIDriverByLabel returns a CSI driver object matching the specified label if it is unique
func (c *KubectlClient) GetCSIDriverByLabel(label string) (*v1beta12.CSIDriver, error) {

	CSIDrivers, err := c.GetCSIDriversByLabel(label)
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
func (c *KubectlClient) GetCSIDriversByLabel(label string) ([]v1beta12.CSIDriver, error) {

	// Get CSI Driver info
	cmdArgs := []string{"get", "CSIDriver", "-l", label, "-o=json"}

	cmd := exec.Command(c.cli, cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var CSIDriverList v1beta12.CSIDriverList
	if err := json.NewDecoder(stdout).Decode(&CSIDriverList); err != nil {
		return nil, err
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	return CSIDriverList.Items, nil
}

// GetCSIDriversByLabel returns true if one or more CSI Driver objects
// matching the specified label exist.
func (c *KubectlClient) CheckCSIDriverExistsByLabel(label string) (bool, string, error) {

	CSIDrivers, err := c.GetCSIDriversByLabel(label)
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
func (c *KubectlClient) DeleteCSIDriverByLabel(label string) error {
	return c.DeleteObjectByLabel(label, "", "CSIDriver", "CSI Driver")
}

// DeleteCSIDriver deletes a CSI Driver object matching the specified name
func (c *KubectlClient) DeleteCSIDriver(name string) error {
	return c.DeleteObject(name, "", "CSIDriver", "CSI Driver")
}

// PatchCSIDriverByLabel patches a deployment object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) PatchCSIDriverByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	return c.PatchObjectByLabel(label, "", "CSIDriver", "CSI Driver", patchBytes, patchType)
}

// GetSecretsByLabel returns all secret object matching specified label
func (c *KubectlClient) GetSecretsByLabel(label string, allNamespaces bool) ([]v1.Secret, error) {

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

	return secretList.Items, nil
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

// UpdateSecret updates an existing Secret
func (c *KubectlClient) UpdateSecret(secret *v1.Secret) (*v1.Secret, error) {

	// Convert to YAML
	jsonBytes, err := json.Marshal(secret)
	if err != nil {
		return nil, err
	}
	yamlBytes, _ := yaml.JSONToYAML(jsonBytes)

	// Create object
	err = c.updateObjectByYAML(string(yamlBytes))
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
func (c *KubectlClient) DeleteSecretDefault(secretName string) error {
	return c.DeleteSecret(secretName, c.namespace)
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

// DeleteSecret deletes the specified Secret by name and namespace
func (c *KubectlClient) DeleteSecret(name, namespace string) error {
	return c.DeleteObject(name, namespace, "secret", "Secret")
}

// PatchPodSecurityPolicyByLabel patches a pod security policy object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) PatchSecretByLabel(label string, patchBytes []byte, patchType types.PatchType) error {
	return c.PatchObjectByLabel(label, c.namespace, "secret", "Secret", patchBytes, patchType)
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

// CreateObjectByFile creates one or more objects on the server from a YAML/JSON file at the specified path.
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

// CreateObjectByYAML creates one or more objects on the server from a YAML/JSON document.
func (c *KubectlClient) CreateObjectByYAML(yamlData string) error {
	for _, yamlDocument := range regexp.MustCompile(YAMLSeparator).Split(yamlData, -1) {
		checkCreateObjectByYAML := func() error {
			if returnError := c.createObjectByYAML(yamlDocument); returnError != nil {
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
		createObjectBackoff.MaxElapsedTime = c.timeout

		log.WithField("yamlDocument", yamlDocument).Trace("Waiting for object to be created.")

		if err := backoff.RetryNotify(checkCreateObjectByYAML, createObjectBackoff, createObjectNotify); err != nil {
			returnError := fmt.Errorf("yamlDocument %s was not created after %3.2f seconds",
				yamlDocument, c.timeout.Seconds())
			return returnError
		}
	}
	return nil
}

// CreateObjectByYAML creates one or more objects on the server from a YAML/JSON document.
func (c *KubectlClient) createObjectByYAML(yaml string) error {

	args := []string{fmt.Sprintf("--namespace=%s", c.namespace), "create", "-f", "-"}
	cmd := exec.Command(c.cli, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		stdin.Write([]byte(yaml)) //nolint
	}()

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.Debug("Created Kubernetes object by YAML.")

	return nil
}

// DeleteObjectByFile deletes one or more objects on the server from a YAML/JSON file at the specified path.
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

// DeleteObjectByYAML deletes one or more objects on the server from a YAML/JSON document.
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
		stdin.Write([]byte(yaml)) //nolint
	}()

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.Debug("Deleted Kubernetes object by YAML.")

	return nil
}

// updateObjectByYAML updates one or more objects on the server from a YAML/JSON document..
func (c *KubectlClient) updateObjectByYAML(yaml string) error {

	args := []string{fmt.Sprintf("--namespace=%s", c.namespace), "apply", "-f", "-"}
	cmd := exec.Command(c.cli, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		stdin.Write([]byte(yaml)) //nolint
	}()

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.Debug("Applied changes to Kubernetes object by YAML.")

	return nil
}

// RemoveTridentUserFromOpenShiftSCC removes the specified user (typically a service account) from the
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

// GetOpenShiftSCCByName gets the specified user (typically a service account) from the specified
// security context constraint. This only works for OpenShift, and it must be idempotent.
func (c *KubectlClient) GetOpenShiftSCCByName(user, scc string) (bool, bool, []byte, error) {
	var SCCExist, SCCUserExist bool
	sccUser := fmt.Sprintf("system:serviceaccount:%s:%s", c.namespace, user)
	sccName := `"name": "trident"`

	if c.flavor != FlavorOpenShift {
		return SCCExist, SCCUserExist, nil, errors.New("the current client context is not OpenShift")
	}

	args := []string{
		"get",
		"scc",
		sccName,
		"-o=json",
	}

	out, err := exec.Command(c.cli, args...).Output()
	if err != nil {
		return SCCExist, SCCUserExist, nil, fmt.Errorf("%s; %v", string(out), err)
	}

	SCCExist = true

	if strings.Contains(string(out), sccUser) {
		SCCUserExist = true
	}

	return SCCExist, SCCUserExist, out, nil
}

// PatchOpenShiftSCC patches the specified user (typically a service account) from the specified
// security context constraint. This only works for OpenShift, and it must be idempotent.
func (c *KubectlClient) PatchOpenShiftSCC(newJSONData []byte) error {

	// Update the object on the server
	return c.updateObjectByYAML(string(newJSONData))
}

func (c *KubectlClient) FollowPodLogs(pod, container, namespace string, logLineCallback LogLineCallback) {

	var (
		err error
		cmd *exec.Cmd
	)

RetryLoop:
	for {
		time.Sleep(1 * time.Second)

		args := []string{
			fmt.Sprintf("--namespace=%s", namespace),
			"logs",
			pod,
			"-f",
		}
		if container != "" {
			args = append(args, []string{"-c", container}...)
		}

		log.WithField("cmd", c.cli+" "+strings.Join(args, " ")).Debug("Getting logs.")

		cmd = exec.Command(c.cli, args...)

		// Create a pipe that holds stdout
		stdout, _ := cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		// Start the child process
		err = cmd.Start()
		if err != nil {
			log.WithFields(log.Fields{
				"pod":       pod,
				"container": container,
				"namespace": namespace,
				"error":     err,
			}).Error("Could not get pod logs.")
			return
		}

		// Create a new scanner
		buff := bufio.NewScanner(io.MultiReader(stdout, stderr))

		// Iterate over buff and handle one line at a time
		for buff.Scan() {
			line := buff.Text()

			// If we get an error from Kubernetes, just try again
			if strings.Contains(line, "Error from server") {

				log.WithFields(log.Fields{
					"pod":       pod,
					"container": container,
					"error":     line,
				}).Debug("Got server error, retrying.")

				_ = cmd.Wait()
				continue RetryLoop
			}

			logLineCallback(line)
		}

		log.WithFields(log.Fields{
			"pod":       pod,
			"container": container,
		}).Debug("Received EOF from pod logs.")
		break
	}

	// Clean up
	_ = cmd.Wait()
}

// AddFinalizerToCRDs updates the CRD objects to include our Trident finalizer (definitions are not namespaced)
func (c *KubectlClient) AddFinalizerToCRDs(CRDnames []string) error {
	for _, crdName := range CRDnames {
		err := c.AddFinalizerToCRD(crdName)
		if err != nil {
			log.Errorf("error in adding finalizers to Kubernetes CRD object %v", crdName)
			return err
		}
	}

	return nil
}

// AddFinalizerToCRD patches the CRD object to include our Trident finalizer (definitions are not namespaced)
func (c *KubectlClient) AddFinalizerToCRD(crdName string) error {

	log.Debugf("Adding finalizers to Kubernetes CRD object %v", crdName)

	// k.cli patch crd ${crd} -p '{"metadata":{"finalizers": ["trident.netapp.io"]}}' --type=merge
	args := []string{
		"patch",
		"crd",
		crdName,
		"-p",
		"{\"metadata\":{\"finalizers\": [\"" + TridentFinalizer + "\"]}}",
		"--type=merge",
	}

	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.Debugf("Added finalizers to Kubernetes CRD object %v", crdName)

	return nil
}

// RemoveFinalizerFromCRD patches the CRD object to remove all finalizers (definitions are not namespaced)
func (c *KubectlClient) RemoveFinalizerFromCRD(crdName string) error {

	log.Debugf("Removing finalizers from Kubernetes CRD object %v", crdName)

	// k.cli patch crd ${crd} -p '{"metadata":{"finalizers": []}}' --type=merge
	args := []string{
		"patch",
		"crd",
		crdName,
		"-p",
		"{\"metadata\":{\"finalizers\": []}}",
		"--type=merge",
	}

	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.Debugf("Removed finalizers from Kubernetes CRD object %v", crdName)

	return nil
}

func (c *KubectlClient) GetCRDClient() (*crdclient.Clientset, error) {

	// c.cli config view --raw
	args := []string{"config", "view", "--raw"}

	out, err := exec.Command(c.cli, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s; %v", string(out), err)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(out)
	if err != nil {
		return nil, err
	}

	return crdclient.NewForConfig(restConfig)
}

// DeleteObjectByLabelNamespaced deletes an object matching the specified label in the namespace of the client.
func (c *KubectlClient) DeleteObjectByLabel(label, namespace, kind, kindName string) error {

	cmdArgs := []string{"delete", kind, "-l", label}
	if namespace != "" {
		cmdArgs = append(cmdArgs, "--namespace", namespace)
	}

	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"label": label,
	}).Debugf("Deleted Kubernetes %s.", kindName)

	return nil
}

// PatchObjectByLabel deletes an object matching the specified name and namespace
func (c *KubectlClient) DeleteObject(name, namespace, kind, kindName string) error {

	cmdArgs := []string{"delete", kind, name}
	if namespace != "" {
		cmdArgs = append(cmdArgs, "--namespace", namespace)
	}

	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"name": name,
	}).Debugf("Deleted Kubernetes %s.", kindName)

	return nil
}

// PatchObjectByLabelAndNamespace patches an object matching the specified label
// in the namespace of the client.
func (c *KubectlClient) PatchObjectByLabel(label, namespace, kind, kindName string, patchBytes []byte, patchType types.PatchType) error {

	cmdArgs := []string{"patch", kind, "-l", label, "-p", string(patchBytes), "--type", string(patchType)}
	if namespace != "" {
		cmdArgs = append(cmdArgs, "--namespace", namespace)
	}

	out, err := exec.Command(c.cli, cmdArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s; %v", string(out), err)
	}

	log.WithFields(log.Fields{
		"label": label,
	}).Debugf("Patched Kubernetes %s.", kindName)

	return nil
}

func (c *KubectlClient) IsTopologyInUse() (bool, error) {
	cmd := exec.Command(c.cli, "get", "node", "-o=json")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, err
	}
	if err := cmd.Start(); err != nil {
		return false, err
	}

	var nodeList *v1.NodeList
	if err := json.NewDecoder(stdout).Decode(&nodeList); err != nil {
		return false, err
	}
	if err := cmd.Wait(); err != nil {
		return false, err
	}

	for _, node := range nodeList.Items {
		for key := range node.Labels {
			if strings.Contains(key, "topology.kubernetes.io") {
				return true, nil
			}
		}
	}

	return false, nil
}
