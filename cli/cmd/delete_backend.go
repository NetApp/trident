// Copyright 2022 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/utils/errors"
)

var allBackends bool

func init() {
	deleteCmd.AddCommand(deleteBackendCmd)
	deleteBackendCmd.Flags().BoolVarP(&allBackends, "all", "", false, "Delete all backends")
}

var deleteBackendCmd = &cobra.Command{
	Use:     "backend <name> [<name>...]",
	Short:   "Delete one or more storage backends from Trident",
	Aliases: []string{"b", "backends"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"delete", "backend"}
			if allBackends {
				command = append(command, "--all")
			}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return backendDelete(args)
		}
	},
}

func backendDelete(backendNames []string) error {
	var err error

	if allBackends {
		// Make sure --all isn't being used along with specific backends
		if len(backendNames) > 0 {
			return errors.New("cannot use --all switch and specify individual backends")
		}

		// Get list of backend names so we can delete them all
		backendNames, err = GetBackends()
		if err != nil {
			return err
		}
	} else {
		// Not using --all, so make sure one or more backends were specified
		if len(backendNames) == 0 {
			return errors.New("backend name not specified")
		}
	}

	for _, backendName := range backendNames {

		if backendName == "" {
			continue
		}

		url := BaseURL() + "/backend/" + backendName

		response, responseBody, err := api.InvokeRESTAPI("DELETE", url, nil)
		if err != nil {
			return err
		} else if response.StatusCode != http.StatusOK {
			return fmt.Errorf("could not delete backend %s: %v", backendName,
				GetErrorFromHTTPResponse(response, responseBody))
		}
	}

	return nil
}
