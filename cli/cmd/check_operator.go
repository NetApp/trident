// Copyright 2024 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	cliapi "github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/utils/errors"
)

var checkTimeout int32

const (
	RequestTimeout      = 5 * time.Second
	backoffMaxInterval  = 5 * time.Second
	operatorService     = "trident-operator"
	operatorServicePort = "8000"
	operatorServicePath = "/operator/status"
	podNamespace        = "POD_NAMESPACE"
)

func init() {
	checkCmd.AddCommand(checkOperatorCmd)
	checkOperatorCmd.Flags().Int32VarP(&checkTimeout, "timeout", "t", 1, "timeout in seconds for the operator check")
	if err := checkOperatorCmd.Flags().MarkHidden("timeout"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

var checkOperatorCmd = &cobra.Command{
	Use:     "operator",
	Short:   "check operator pod status",
	Aliases: []string{"o"},
	Hidden:  true,
	RunE:    checkOperatorStatusRunE,
}

func checkOperatorStatusRunE(cmd *cobra.Command, args []string) error {
	if OperatingMode == ModeTunnel {
		command := []string{
			"check",
			"operator",
			"--timeout",
			fmt.Sprintf("%d", checkTimeout),
		}
		out, err := TunnelCommand(append(command, args...))
		printOutput(cmd, out, err)
		return err
	} else {
		return checkOperatorStatus(checkTimeout)
	}
}

func getOperatorStatusURL(namespace string) string {
	return "http://" + operatorService + "." + namespace + "." + "svc.cluster.local" + ":" +
		operatorServicePort + operatorServicePath
}

func checkOperatorStatus(checkTimeout int32) error {
	var err error
	status := cliapi.OperatorStatus{}
	var response *http.Response
	var url string

	getOperatorStatus := func() error {
		if namespace := os.Getenv(podNamespace); namespace == "" {
			return errors.New("error in getting trident operator pod")
		} else {
			url = getOperatorStatusURL(namespace)
		}
		request, err := http.NewRequest("GET", url, nil)
		request.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: RequestTimeout}
		if response, err = client.Do(request); err != nil {
			return err
		}

		defer response.Body.Close()

		err = json.NewDecoder(response.Body).Decode(&status)
		if status.Status != string(cliapi.OperatorPhaseDone) {
			return errors.New("operator pod status is not set to done")
		}

		return nil
	}

	checkOperatorNotify := func(err error, duration time.Duration) {}

	checkOperatorBackoff := backoff.NewExponentialBackOff()
	checkOperatorBackoff.MaxInterval = backoffMaxInterval
	checkOperatorBackoff.MaxElapsedTime = time.Duration(checkTimeout) * time.Second

	if err = backoff.RetryNotify(getOperatorStatus, checkOperatorBackoff, checkOperatorNotify); err != nil {
		if status.Status != "" {
			writeStatus(status)
		}
		return err
	}

	writeStatus(status)
	return nil
}

func writeStatus(status cliapi.OperatorStatus) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(status)
	case FormatYAML:
		WriteYAML(status)
	case FormatWide:
		writeStatusTableWide(status)
	default:
		writeStatusTable(status)
	}
}

func writeStatusTable(status cliapi.OperatorStatus) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Type", "Status"})
	if status.ErrorMessage != "" {
		table.SetFooter([]string{"Overall Status", status.Status, status.ErrorMessage})
	} else {
		table.SetFooter([]string{"Overall Status", status.Status, ""})
	}

	for torcName, torcStatus := range status.TorcStatus {
		table.Append([]string{torcName, "TridentOrchestratorCR", torcStatus.Status})
	}

	for tconfName, tconfStatus := range status.TconfStatus {
		table.Append([]string{tconfName, "TridentConfiguratorCR", tconfStatus.Status})
	}

	table.Render()
}

func writeStatusTableWide(status cliapi.OperatorStatus) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Type", "Status", "Message"})
	if status.ErrorMessage != "" {
		table.SetFooter([]string{"Overall Status", status.Status, status.ErrorMessage, ""})
	} else {
		table.SetFooter([]string{"Overall Status", status.Status, "", ""})
	}

	for torcName, torcStatus := range status.TorcStatus {
		table.Append([]string{torcName, "TridentOrchestratorCR", torcStatus.Status, torcStatus.Message})
	}

	for tconfName, tconfStatus := range status.TconfStatus {
		table.Append([]string{tconfName, "TridentConfiguratorCR", tconfStatus.Status, tconfStatus.Message})
	}

	table.Render()
}
