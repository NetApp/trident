// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	k8s "k8s.io/api/core/v1"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/config"
)

const (
	agreementPrompt = "Please see NetApp's privacy policy at https://www.netapp.com/company/legal/privacy-policy/\n\nDo you authorize NetApp to collect personal information for the exclusive purpose of providing support services?"
)

var (
	acceptAgreement bool
	since           time.Duration
)

func init() {
	sendCmd.AddCommand(sendAutosupportCmd)
	sendAutosupportCmd.PersistentFlags().BoolVar(&acceptAgreement, "accept-agreement", false, "By supplying this flag, you authorize NetApp to collect personal information for the exclusive purpose of providing support services. View NetApp's privacy policy here: https://www.netapp.com/company/legal/privacy-policy/")
	sendAutosupportCmd.PersistentFlags().DurationVar(&since, "since", 24*time.Hour,
		"Duration before the current time to start logs from")
}

var sendAutosupportCmd = &cobra.Command{
	Use:     "autosupport",
	Short:   "Send an Autosupport archive to NetApp",
	Aliases: []string{"a", "asup"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if !acceptAgreement {
			confirmed, err := getUserConfirmation(agreementPrompt, cmd)
			if err != nil {
				return err
			}
			if !confirmed {
				cmd.Println("You must accept the agreement.")
				return nil
			}
		}

		if OperatingMode == ModeTunnel {
			// Do not attempt to send a support bundle if the ASUP container does not exist.
			isAutosupportPresent, err := isAutosupportContainerPresent(TridentPodNamespace, TridentCSILabel)
			if err != nil {
				return err
			} else if !isAutosupportPresent {
				cmd.Println("Unable to send autosupport bundle; no autosupport container found.")
				return nil
			}

			command := []string{"send", "autosupport"}
			args = append(args, "--accept-agreement")

			if since != time.Duration(0) {
				args = append(args, "--since="+since.String())
			}

			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		} else {
			if since != time.Duration(0) {
				return triggerAutosupport(since.String())
			} else {
				return triggerAutosupport("")
			}
		}
	},
}

func triggerAutosupport(since string) error {
	url := BaseAutosupportURL() + "/collector/trident/trigger"

	if since != "" {
		url += "?since=" + since
	}

	response, responseBody, err := api.InvokeRESTAPI("POST", url, nil)
	if err != nil {
		return err
	} else if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("could not send autosupport: %v", GetErrorFromHTTPResponse(response, responseBody))
	} else {
		fmt.Println("Autosupport sent.")
	}

	return nil
}

func isAutosupportContainerPresent(namespace, appLabel string) (bool, error) {
	// Get 'trident' pod info
	cmd := execKubernetesCLIRaw(
		"get", "pod",
		"-n", namespace,
		"-l", appLabel,
		"-o=json",
		"--field-selector=status.phase=Running")
	var outbuff bytes.Buffer
	cmd.Stdout = &outbuff
	err := cmd.Run()
	if err != nil {
		return false, err
	}

	var tridentPodList k8s.PodList
	if err = json.Unmarshal(outbuff.Bytes(), &tridentPodList); err != nil {
		return false, fmt.Errorf("could not unmarshal Trident pod list; %v", err)
	}

	if len(tridentPodList.Items) != 1 {
		return false, fmt.Errorf("could not find a Trident pod in the %s namespace. "+
			"You may need to use the -n option to specify the correct namespace", namespace)
	}
	tridentPod := tridentPodList.Items[0]

	// Check for the autosupport container.
	for _, c := range tridentPod.Spec.Containers {
		if c.Name == config.DefaultAutosupportName {
			return true, nil
		}
	}

	return false, nil
}
