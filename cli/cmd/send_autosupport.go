// Copyright 2020 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
)

const (
	agreementPrompt = "Please see NetApp's privacy policy at https://www.netapp.com/company/legal/privacy-policy/\n\nDo you authorize NetApp to collect personal information for the exclusive purpose of providing support services?"
)

var acceptAgreement bool

func init() {
	sendCmd.AddCommand(sendAutosupportCmd)
	sendAutosupportCmd.PersistentFlags().BoolVar(&acceptAgreement, "accept-agreement", false, "By supplying this flag, you authorize NetApp to collect personal information for the exclusive purpose of providing support services. View NetApp's privacy policy here: https://www.netapp.com/company/legal/privacy-policy/")
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
			command := []string{"send", "autosupport"}
			args = append(args, "--accept-agreement")
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return triggerAutosupport()
		}
	},
}

func triggerAutosupport() error {

	url := BaseAutosupportURL() + "/collector/trident/trigger"

	response, responseBody, err := api.InvokeRESTAPI("POST", url, nil, Debug)
	if err != nil {
		return err
	} else if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("could not send autosupport: %v", GetErrorFromHTTPResponse(response, responseBody))
	} else {
		fmt.Println("Autosupport sent.")
	}

	return nil
}
