package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"strings"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/config"
	"github.com/spf13/cobra"
)

const (
	FORMAT_JSON = "json"
	FORMAT_NAME = "name"
	FORMAT_WIDE = "wide"
	FORMAT_YAML = "yaml"

	MODE_DIRECT = "direct"
	MODE_TUNNEL = "tunnel"

	POD_SERVER = "127.0.0.1:8000"
)

var (
	OperatingMode string
	TridentPod    string

	Debug        bool
	Server       string
	OutputFormat string
)

var RootCmd = &cobra.Command{
	Use:   "tridentctl",
	Short: "A CLI tool for NetApp Trident",
	Long:  `A CLI tool for managing the NetApp Trident external storage provisioner for Kubernetes`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		err := discoverOperatingMode()
		if err != nil {
			return fmt.Errorf("%v", err)
		}
		return nil
	},
}

func init() {
	RootCmd.PersistentFlags().BoolVarP(&Debug, "debug", "d", false, "Debug output")
	RootCmd.PersistentFlags().StringVarP(&Server, "server", "s", "", "Address/port of Trident REST interface")
	RootCmd.PersistentFlags().StringVarP(&OutputFormat, "output", "o", "", "Output format.  One of json|yaml|name|wide|ps (default)")
}

func discoverOperatingMode() error {

	envServer := os.Getenv("TRIDENT_SERVER")

	if Server != "" {

		// Server specified on command line takes precedence
		OperatingMode = MODE_DIRECT
		if Debug {
			fmt.Printf("Operating mode = %s, Server = %s\n", OperatingMode, Server)
		}
		return nil
	} else if envServer != "" {

		// Consider environment variable next
		Server = envServer
		OperatingMode = MODE_DIRECT
		if Debug {
			fmt.Printf("Operating mode = %s, Server = %s\n", OperatingMode, Server)
		}
		return nil
	} else {

		// Server not specified, so try tunneling to a pod
		pod, err := getTridentPod()
		if err != nil {
			return err
		} else {
			OperatingMode = MODE_TUNNEL
			TridentPod = pod
			Server = POD_SERVER
			if Debug {
				fmt.Printf("Operating mode = %s, Trident pod = %s\n", OperatingMode, TridentPod)
			}
			return nil
		}
	}
}

func getTridentPod() (string, error) {

	// Get 'trident' pod info
	cmd := exec.Command("kubectl", "get", "pod", "-l", "app=trident.netapp.io", "-o=json")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var tridentPod api.KubectlPodInfo
	if err := json.NewDecoder(stdout).Decode(&tridentPod); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return "", err
	}

	//fmt.Printf("%+v\n", tridentPod)

	if len(tridentPod.Items) != 1 {
		return "", errors.New("Could not find a Trident pod.")
	}

	// Get Trident pod name
	name := tridentPod.Items[0].Metadata.Name

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

	// Build tunnel command for 'kubectl exec'
	execCommand := []string{"exec", TridentPod, "-c", "trident-main", "--"}

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
		fmt.Printf("Invoking tunneled command: kubectl %v\n", strings.Join(execCommand, " "))
	}

	// Invoke tridentctl inside the Trident pod
	out, err := exec.Command("kubectl", execCommand...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	} else {
		fmt.Print(string(out))
	}
}

func TunnelCommandRaw(commandArgs []string) ([]byte, error) {

	// Build tunnel command for 'kubectl exec'
	execCommand := []string{"exec", TridentPod, "-c", "trident-main", "--"}

	// Build CLI command
	cliCommand := []string{"tridentctl", "-s", Server}
	cliCommand = append(cliCommand, commandArgs...)

	// Combine tunnel and CLI commands
	execCommand = append(execCommand, cliCommand...)

	if Debug {
		fmt.Printf("Invoking tunneled command: kubectl %v\n", strings.Join(execCommand, " "))
	}

	// Invoke tridentctl inside the Trident pod
	return exec.Command("kubectl", execCommand...).CombinedOutput()
}
