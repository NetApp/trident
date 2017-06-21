package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/netapp/trident/config"
	"github.com/spf13/cobra"
)

var container string

func init() {
	RootCmd.AddCommand(logsCmd)
	logsCmd.Flags().StringVarP(&container, "container", "c", config.ContainerTrident, "Container to query for logs. One of trident-main|etcd")
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Print the logs from Trident",
	Long:  "Print the logs from the Trident storage orchestrator for Kubernetes",
	RunE: func(cmd *cobra.Command, args []string) error {

		var logs []byte
		var err error

		if container != config.ContainerTrident && container != config.ContainerEtcd {
			return fmt.Errorf("Container must be either %s or %s.",
				config.ContainerTrident, config.ContainerEtcd)
		}

		// Get the server version
		if OperatingMode == MODE_TUNNEL {
			logs, err = getLogs(container)
		} else {
			err = errors.New("'tridentctl logs' only supports Trident running in a Kubernetes pod.")
		}

		SetExitCodeFromError(err)
		if err != nil {
			return err
		}

		fmt.Printf("%s\n", string(logs))
		return nil
	},
}

func getLogs(container string) ([]byte, error) {

	// Build command for 'kubectl logs'
	logsCommand := []string{"logs", TridentPodName, "-n", TridentPodNamespace, "-c", container}

	if Debug {
		fmt.Printf("Invoking command: kubectl %v\n", strings.Join(logsCommand, " "))
	}

	// Get logs
	return exec.Command("kubectl", logsCommand...).CombinedOutput()
}
