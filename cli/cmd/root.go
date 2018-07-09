// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/config"
	"github.com/spf13/cobra"
	k8s "k8s.io/api/core/v1"
)

const (
	FormatJSON = "json"
	FormatName = "name"
	FormatWide = "wide"
	FormatYAML = "yaml"

	ModeDirect  = "direct"
	ModeTunnel  = "tunnel"
	ModeInstall = "install"

	CLIKubernetes = "kubectl"
	CLIOpenshift  = "oc"

	PodServer = "127.0.0.1:8000"

	ExitCodeSuccess = 0
	ExitCodeFailure = 1

	TridentLabelKey   = "app"
	TridentLabelValue = "trident.netapp.io"
	TridentLabel      = TridentLabelKey + "=" + TridentLabelValue

	TridentCSILabelKey   = "app"
	TridentCSILabelValue = "controller.csi.trident.netapp.io"
	TridentCSILabel      = TridentCSILabelKey + "=" + TridentCSILabelValue

	TridentNodeLabelKey   = "app"
	TridentNodeLabelValue = "node.csi.trident.netapp.io"
	TridentNodeLabel      = TridentNodeLabelKey + "=" + TridentNodeLabelValue

	TridentInstallerLabelKey   = "app"
	TridentInstallerLabelValue = "trident-installer.netapp.io"
	TridentInstallerLabel      = TridentInstallerLabelKey + "=" + TridentInstallerLabelValue
)

var (
	OperatingMode       string
	KubernetesCLI       string
	TridentPodName      string
	TridentPodNamespace string
	ExitCode            int

	Debug        bool
	Server       string
	OutputFormat string
	CSI          bool
)

var RootCmd = &cobra.Command{
	SilenceUsage: true,
	Use:          "tridentctl",
	Short:        "A CLI tool for NetApp Trident",
	Long:         `A CLI tool for managing the NetApp Trident external storage provisioner for Kubernetes`,
}

func init() {
	RootCmd.PersistentFlags().BoolVarP(&Debug, "debug", "d", false, "Debug output")
	RootCmd.PersistentFlags().StringVarP(&Server, "server", "s", "", "Address/port of Trident REST interface")
	RootCmd.PersistentFlags().StringVarP(&OutputFormat, "output", "o", "", "Output format. One of json|yaml|name|wide|ps (default)")
	RootCmd.PersistentFlags().StringVarP(&TridentPodNamespace, "namespace", "n", "", "Namespace of Trident deployment")

	RootCmd.PersistentFlags().BoolVar(&CSI, "csi", false, "Manage Trident as a CSI plugin (experimental)")
	RootCmd.PersistentFlags().MarkHidden("csi")
}

func discoverOperatingMode(cmd *cobra.Command) error {

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

	if CSI {
		// Find the CSI Trident pod
		if TridentPodName, err = getTridentPod(TridentPodNamespace, TridentCSILabel); err != nil {
			return err
		}
	} else {
		// Find the Trident pod
		if TridentPodName, err = getTridentPod(TridentPodNamespace, TridentLabel); err != nil {

			// Try falling back to CSI pod
			if TridentPodName, err = getTridentPod(TridentPodNamespace, TridentCSILabel); err != nil {
				return err
			}
		}
	}

	OperatingMode = ModeTunnel
	Server = PodServer
	return nil
}

func discoverKubernetesCLI() error {

	// Try the OpenShift CLI first
	_, err := exec.Command(CLIOpenshift, "version").CombinedOutput()
	if GetExitCodeFromError(err) == ExitCodeSuccess {
		KubernetesCLI = CLIOpenshift
		return nil
	}

	// Fall back to the K8S CLI
	_, err = exec.Command(CLIKubernetes, "version").CombinedOutput()
	if GetExitCodeFromError(err) == ExitCodeSuccess {
		KubernetesCLI = CLIKubernetes
		return nil
	}

	return errors.New("could not find the Kubernetes CLI")
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

	//fmt.Printf("%+v\n", serviceAccount)

	// Get Trident pod name & namespace
	namespace := serviceAccount.ObjectMeta.Namespace

	return namespace, nil
}

// getTridentPod returns the name of the Trident pod in the specified namespace
func getTridentPod(namespace, appLabel string) (string, error) {

	// Get 'trident' pod info
	cmd := exec.Command(KubernetesCLI, "get", "pod", "-n", namespace, "-l", appLabel, "-o=json")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var tridentPod k8s.PodList
	if err := json.NewDecoder(stdout).Decode(&tridentPod); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
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

func GetBaseURL() (string, error) {

	url := fmt.Sprintf("http://%s%s", Server, config.BaseURL)

	if Debug {
		fmt.Printf("Trident URL: %s\n", url)
	}

	return url, nil
}

func TunnelCommand(commandArgs []string) {

	// Build tunnel command to exec command in container
	execCommand := []string{"exec", TridentPodName, "-n", TridentPodNamespace, "-c", config.ContainerTrident, "--"}

	// Build CLI command
	cliCommand := []string{"tridentctl", "-s", Server}
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

func TunnelCommandRaw(commandArgs []string) ([]byte, error) {

	// Build tunnel command to exec command in container
	execCommand := []string{"exec", TridentPodName, "-n", TridentPodNamespace, "-c", config.ContainerTrident, "--"}

	// Build CLI command
	cliCommand := []string{"tridentctl", "-s", Server}
	cliCommand = append(cliCommand, commandArgs...)

	// Combine tunnel and CLI commands
	execCommand = append(execCommand, cliCommand...)

	if Debug {
		fmt.Printf("Invoking tunneled command: %s %v\n", KubernetesCLI, strings.Join(execCommand, " "))
	}

	// Invoke tridentctl inside the Trident pod
	output, err := exec.Command(KubernetesCLI, execCommand...).CombinedOutput()

	SetExitCodeFromError(err)
	return output, err
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
