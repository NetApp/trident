// Copyright 2020 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
)

func init() {
	sendCmd.AddCommand(sendAutosupportCmd)
}

var sendAutosupportCmd = &cobra.Command{
	Use:     "autosupport",
	Short:   "Send an Autosupport archive to NetApp",
	Aliases: []string{"a", "asup"},
	RunE: func(cmd *cobra.Command, args []string) error {

		if OperatingMode == ModeTunnel {
			command := []string{"send", "autosupport"}
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
