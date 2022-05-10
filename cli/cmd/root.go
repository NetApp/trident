// Copyright 2019 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	k8s "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/config"

	// Load all auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	FormatJSON     = "json"
	FormatName     = "name"
	FormatWide     = "wide"
	FormatYAML     = "yaml"
	FormatMarkdown = "markdown"

	ModeDirect  = "direct"
	ModeTunnel  = "tunnel"
	ModeInstall = "install"

	CLIKubernetes = "kubectl"
	CLIOpenshift  = "oc"

	PodServer               = "127.0.0.1:8000"
	PodAutosupportCollector = "127.0.0.1:8003"

	ExitCodeSuccess = 0
	ExitCodeFailure = 1

	TridentLegacyLabelKey   = "app"
	TridentLegacyLabelValue = "trident.netapp.io"
	TridentLegacyLabel      = TridentLegacyLabelKey + "=" + TridentLegacyLabelValue

	TridentCSILabelKey   = "app"
	TridentCSILabelValue = "controller.csi.trident.netapp.io"
	TridentCSILabel      = TridentCSILabelKey + "=" + TridentCSILabelValue

	TridentNodeLabelKey   = "app"
	TridentNodeLabelValue = "node.csi.trident.netapp.io"
	TridentNodeLabel      = TridentNodeLabelKey + "=" + TridentNodeLabelValue

	TridentInstallerLabelKey   = "app"
	TridentInstallerLabelValue = "trident-installer.netapp.io"
	TridentInstallerLabel      = TridentInstallerLabelKey + "=" + TridentInstallerLabelValue

	TridentMigratorLabelKey   = "app"
	TridentMigratorLabelValue = "trident-migrator.netapp.io"
	TridentMigratorLabel      = TridentMigratorLabelKey + "=" + TridentMigratorLabelValue

	TridentPersistentObjectLabelKey   = "object"
	TridentPersistentObjectLabelValue = "persistent.trident.netapp.io"
	TridentPersistentObjectLabel      = TridentPersistentObjectLabelKey + "=" + TridentPersistentObjectLabelValue

	TridentOperatorLabelKey   = "app"
	TridentOperatorLabelValue = "operator.trident.netapp.io"
	TridentOperatorLabel      = TridentOperatorLabelKey + "=" + TridentOperatorLabelValue

	AutosupportCollectorURL = "/autosupport/v1"
)

var (
	OperatingMode       string
	KubernetesCLI       string
	TridentPodName      string
	TridentPodNamespace string
	ExitCode            int

	Debug                bool
	Server               string
	AutosupportCollector string
	OutputFormat         string

	listOpts   = metav1.ListOptions{}
	updateOpts = metav1.UpdateOptions{}
	deleteOpts = metav1.DeleteOptions{}

	ctx = context.TODO
)

var RootCmd = &cobra.Command{
	SilenceUsage: true,
	Use:          "tridentctl",
	Short:        "A CLI tool for NetApp Trident",
	Long:         `A CLI tool for managing the NetApp Trident external storage provisioner for Kubernetes`,
}

func init() {
	RootCmd.PersistentFlags().BoolVarP(&Debug, "debug", "d", false, "Debug output")
	RootCmd.PersistentFlags().StringVarP(&Server, "server", "s", "", "Address/port of Trident REST interface (127.0.0.1 or [::1] only)")
	RootCmd.PersistentFlags().StringVarP(&OutputFormat, "output", "o", "", "Output format. One of json|yaml|name|wide|ps (default)")
	RootCmd.PersistentFlags().StringVarP(&TridentPodNamespace, "namespace", "n", "", "Namespace of Trident deployment")
}

func discoverOperatingMode(_ *cobra.Command) error {
	defer func() {
		if !Debug {
			return
		}

		switch OperatingMode {
		case ModeDirect:
			fmt.Printf("Operating mode = %s, Server = %s, Autosuport server = %s\n",
				OperatingMode, Server, AutosupportCollector)
		case ModeTunnel:
			fmt.Printf("Operating mode = %s, Trident pod = %s, Namespace = %s, CLI = %s\n",
				OperatingMode, TridentPodName, TridentPodNamespace, KubernetesCLI)
		}
	}()

	// Use the operating mode to inform the Autosupport collector
	defer discoverAutosupportCollector()

	var err error

	envServer := os.Getenv("TRIDENT_SERVER")

	if Server != "" {

		// Server specified on command line takes precedence
		OperatingMode = ModeDirect
		return nil
	} else if envServer != "" {

		// Consider environment variable next
		Server = envServer
		OperatingMode = ModeDirect
		return nil
	}

	// To work with pods, we need to discover which CLI to invoke
	err = discoverKubernetesCLI()
	if err != nil {
		return err
	}

	// Server not specified, so try tunneling to a pod
	if TridentPodNamespace == "" {
		if TridentPodNamespace, err = getCurrentNamespace(); err != nil {
			return err
		}
	}

	// Find the CSI Trident pod
	if TridentPodName, err = getTridentPod(TridentPodNamespace, TridentCSILabel); err != nil {
		// Fall back to non-CSI Trident pod
		if TridentPodName, err = getTridentPod(TridentPodNamespace, TridentLegacyLabel); err != nil {
			return err
		}
	}

	OperatingMode = ModeTunnel
	Server = PodServer
	return nil
}

func discoverJustOperatingMode(_ *cobra.Command) error {
	defer func() {
		if !Debug {
			return
		}

		switch OperatingMode {
		case ModeDirect:
			fmt.Printf("Operating mode = %s, Server = %s\n", OperatingMode, Server)
		case ModeTunnel:
			fmt.Printf("Operating mode = %s, Trident pod = %s, Namespace = %s, CLI = %s\n",
				OperatingMode, TridentPodName, TridentPodNamespace, KubernetesCLI)
		}
	}()

	// Use the operating mode to inform the Autosupport server
	defer discoverAutosupportCollector()

	var err error

	envServer := os.Getenv("TRIDENT_SERVER")

	if Server != "" {

		// Server specified on command line takes precedence
		OperatingMode = ModeDirect
		return nil
	} else if envServer != "" {

		// Consider environment variable next
		Server = envServer
		OperatingMode = ModeDirect
		return nil
	}

	// To work with pods, we need to discover which CLI to invoke
	err = discoverKubernetesCLI()
	if err != nil {
		return err
	}

	OperatingMode = ModeTunnel
	Server = PodServer
	return nil
}

func discoverKubernetesCLI() error {
	// Try the OpenShift CLI first
	_, err := exec.Command(CLIOpenshift, "version").Output()
	if GetExitCodeFromError(err) == ExitCodeSuccess {
		KubernetesCLI = CLIOpenshift
		return nil
	}

	// Fall back to the K8S CLI
	_, err = exec.Command(CLIKubernetes, "version").Output()
	if GetExitCodeFromError(err) == ExitCodeSuccess {
		KubernetesCLI = CLIKubernetes
		return nil
	}

	if ee, ok := err.(*exec.ExitError); ok {
		return fmt.Errorf("found the Kubernetes CLI, but it exited with error: %s",
			strings.TrimRight(string(ee.Stderr), "\n"))
	}

	return fmt.Errorf("could not find the Kubernetes CLI: %v", err)
}

// getCurrentNamespace returns the default namespace from service account info
func getCurrentNamespace() (string, error) {
	// Get current namespace from service account info
	cmd := exec.Command(KubernetesCLI, "get", "serviceaccount", "default", "-o=json")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var serviceAccount k8s.ServiceAccount
	if err := json.NewDecoder(stdout).Decode(&serviceAccount); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}

	// fmt.Printf("%+v\n", serviceAccount)

	// Get Trident pod name & namespace
	namespace := serviceAccount.ObjectMeta.Namespace

	return namespace, nil
}

func discoverAutosupportCollector() {
	switch OperatingMode {
	case ModeDirect:
		envCollector := os.Getenv("TRIDENT_AUTOSUPPORT_COLLECTOR")
		if envCollector != "" {
			AutosupportCollector = envCollector
		} else {
			AutosupportCollector = PodAutosupportCollector
		}
	case ModeTunnel:
		// Nothing to do on the outside
	}
}

// getTridentPod returns the name of the Trident pod in the specified namespace
func getTridentPod(namespace, appLabel string) (string, error) {
	// Get 'trident' pod info
	cmd := exec.Command(
		KubernetesCLI,
		"get", "pod",
		"-n", namespace,
		"-l", appLabel,
		"-o=json",
		"--field-selector=status.phase=Running",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = cmd.Start(); err != nil {
		return "", err
	}

	var tridentPod k8s.PodList
	if err = json.NewDecoder(stdout).Decode(&tridentPod); err != nil {
		return "", err
	}
	if err = cmd.Wait(); err != nil {
		return "", err
	}

	if len(tridentPod.Items) != 1 {
		return "", fmt.Errorf("could not find a Trident pod in the %s namespace. "+
			"You may need to use the -n option to specify the correct namespace", namespace)
	}

	// Get Trident pod name & namespace
	name := tridentPod.Items[0].ObjectMeta.Name

	return name, nil
}

// getTridentOperatorPod returns the name and namespace of the Trident pod
func getTridentOperatorPod(appLabel string) (string, string, error) {
	// Get 'trident-operator' pod info
	cmd := exec.Command(
		KubernetesCLI,
		"get", "pod",
		"--all-namespaces",
		"-l", appLabel,
		"-o=json",
		"--field-selector=status.phase=Running",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	if err = cmd.Start(); err != nil {
		return "", "", err
	}

	var tridentOperatorPod k8s.PodList
	if err = json.NewDecoder(stdout).Decode(&tridentOperatorPod); err != nil {
		return "", "", err
	}
	if err = cmd.Wait(); err != nil {
		return "", "", err
	}

	if len(tridentOperatorPod.Items) != 1 {
		return "", "", fmt.Errorf("could not find a Trident operator pod in the all the namespaces")
	}

	// Get Trident pod name & namespace
	name := tridentOperatorPod.Items[0].ObjectMeta.Name
	namespace := tridentOperatorPod.Items[0].ObjectMeta.Namespace

	return name, namespace, nil
}

// listTridentSidecars returns a list of sidecar container names inside the trident controller pod
func listTridentSidecars(podName, podNameSpace string) ([]string, error) {
	// Get 'trident' pod info
	var sidecarNames []string
	cmd := exec.Command(
		KubernetesCLI,
		"get", "pod",
		podName,
		"-n", podNameSpace,
		"-o=json",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return sidecarNames, err
	}
	if err = cmd.Start(); err != nil {
		return sidecarNames, err
	}

	var tridentPod k8s.Pod
	if err = json.NewDecoder(stdout).Decode(&tridentPod); err != nil {
		return sidecarNames, err
	}
	if err = cmd.Wait(); err != nil {
		return sidecarNames, err
	}

	for _, sidecar := range tridentPod.Spec.Containers {
		// Ignore Trident's main container
		if sidecar.Name != config.ContainerTrident {
			sidecarNames = append(sidecarNames, sidecar.Name)
		}
	}

	return sidecarNames, nil
}

func getTridentNode(nodeName, namespace string) (string, error) {
	selector := fmt.Sprintf("--field-selector=spec.nodeName=%s", nodeName)
	cmd := exec.Command(
		KubernetesCLI,
		"get", "pod",
		"-n", namespace,
		"-l", TridentNodeLabel,
		"-o=json",
		selector,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = cmd.Start(); err != nil {
		return "", err
	}

	var tridentPods k8s.PodList
	if err = json.NewDecoder(stdout).Decode(&tridentPods); err != nil {
		return "", err
	}
	if err = cmd.Wait(); err != nil {
		return "", err
	}

	if len(tridentPods.Items) != 1 {
		return "", fmt.Errorf("could not find a Trident node pod in the %s namespace on node %s. "+
			"You may need to use the -n option to specify the correct namespace", namespace, nodeName)
	}

	// Get Trident node pod name
	name := tridentPods.Items[0].ObjectMeta.Name

	return name, nil
}

// listTridentNodes returns a list of names of the Trident node pods in the specified namespace
func listTridentNodes(namespace string) (map[string]string, error) {
	// Get trident node pods info
	tridentNodes := make(map[string]string)
	cmd := exec.Command(
		KubernetesCLI,
		"get", "pod",
		"-n", namespace,
		"-l", TridentNodeLabel,
		"-o=json",
		"--field-selector=status.phase=Running",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return tridentNodes, err
	}
	if err = cmd.Start(); err != nil {
		return tridentNodes, err
	}

	var tridentPods k8s.PodList
	if err = json.NewDecoder(stdout).Decode(&tridentPods); err != nil {
		return tridentNodes, err
	}
	if err = cmd.Wait(); err != nil {
		return tridentNodes, err
	}

	if len(tridentPods.Items) < 1 {
		return tridentNodes, fmt.Errorf("could not find any Trident node pods in the %s namespace. "+
			"You may need to use the -n option to specify the correct namespace", namespace)
	}

	// Get Trident node and pod name
	for _, pod := range tridentPods.Items {
		tridentNodes[pod.Spec.NodeName] = pod.Name
	}

	return tridentNodes, nil
}

func BaseURL() string {
	url := fmt.Sprintf("http://%s%s", Server, config.BaseURL)

	if Debug {
		fmt.Printf("Trident URL: %s\n", url)
	}

	return url
}

func BaseAutosupportURL() string {
	url := fmt.Sprintf("http://%s%s", AutosupportCollector, AutosupportCollectorURL)

	if Debug {
		fmt.Printf("Trident autosupport URL: %s\n", url)
	}

	return url
}

func TunnelCommand(commandArgs []string) {
	// Build tunnel command to exec command in container
	execCommand := []string{"exec", TridentPodName, "-n", TridentPodNamespace, "-c", config.ContainerTrident, "--"}

	// Build CLI command
	cliCommand := []string{"tridentctl"}
	if Debug {
		cliCommand = append(cliCommand, "--debug")
	}
	if OutputFormat != "" {
		cliCommand = append(cliCommand, []string{"--output", OutputFormat}...)
	}
	cliCommand = append(cliCommand, commandArgs...)

	// Combine tunnel and CLI commands
	execCommand = append(execCommand, cliCommand...)

	if Debug {
		fmt.Printf("Invoking tunneled command: %s %v\n", KubernetesCLI, strings.Join(execCommand, " "))
	}

	// Invoke tridentctl inside the Trident pod
	out, err := exec.Command(KubernetesCLI, execCommand...).CombinedOutput()

	SetExitCodeFromError(err)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", string(out))
	} else {
		fmt.Print(string(out))
	}
}

func TunnelCommandRaw(commandArgs []string) ([]byte, []byte, error) {
	// Build tunnel command to exec command in container
	execCommand := []string{"exec", TridentPodName, "-n", TridentPodNamespace, "-c", config.ContainerTrident, "--"}

	// Build CLI command
	cliCommand := []string{"tridentctl"}
	cliCommand = append(cliCommand, commandArgs...)

	// Combine tunnel and CLI commands
	execCommand = append(execCommand, cliCommand...)

	if Debug {
		fmt.Printf("Invoking tunneled command: %s %v\n", KubernetesCLI, strings.Join(execCommand, " "))
	}

	// Invoke tridentctl inside the Trident pod and get Stdout and Stderr separately in two buffers
	// Capture the Stdout for the command in outbuff which will later be unmarshalled and
	// capture the Stderr for the command in os.Stderr
	cmd := exec.Command(KubernetesCLI, execCommand...)
	var outbuff, stderrBuff bytes.Buffer
	cmd.Stdout = &outbuff
	cmd.Stderr = &stderrBuff
	err := cmd.Run()

	SetExitCodeFromError(err)
	return outbuff.Bytes(), stderrBuff.Bytes(), err
}

func GetErrorFromHTTPResponse(response *http.Response, responseBody []byte) error {
	var errorResponse api.ErrorResponse
	if err := json.Unmarshal(responseBody, &errorResponse); err == nil {
		return fmt.Errorf("%s (%s)", errorResponse.Error, response.Status)
	}
	return errors.New(response.Status)
}

func SetExitCodeFromError(err error) {
	ExitCode = GetExitCodeFromError(err)
}

func GetExitCodeFromError(err error) int {
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

func getUserConfirmation(s string, cmd *cobra.Command) (bool, error) {
	reader := bufio.NewReader(cmd.InOrStdin())

	for {
		cmd.Printf("%s [y/n]: ", s)

		input, err := reader.ReadString('\n')
		if err != nil {
			return false, err
		}

		input = strings.ToLower(strings.TrimSpace(input))

		if input == "y" || input == "yes" {
			return true, nil
		} else if input == "n" || input == "no" {
			return false, nil
		}
	}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}

func kubeConfigPath() string {
	// If KUBECONFIG contains multiple paths, return the first one.
	if paths := os.Getenv("KUBECONFIG"); paths != "" {
		for _, path := range strings.Split(paths, ":") {
			if len(path) > 0 {
				return path
			}
		}
	}

	if home := homeDir(); home != "" {
		return filepath.Join(home, ".kube", "config")
	}

	return ""
}
