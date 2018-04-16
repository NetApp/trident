// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/netapp/trident/cli/api"
	"github.com/spf13/cobra"
)

var AllBackends bool

func init() {
	deleteCmd.AddCommand(deleteBackendCmd)
	deleteBackendCmd.Flags().BoolVarP(&AllBackends, "all", "", false, "Delete all backends")
}

var deleteBackendCmd = &cobra.Command{
	Use:     "backend <name> [<name>...]",
	Short:   "Delete one or more storage backends from Trident",
	Aliases: []string{"b", "backends"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"delete", "backend"}
			if AllBackends {
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

	baseURL, err := GetBaseURL()
	if err != nil {
		return err
	}

	if AllBackends {
		// Make sure --all isn't being used along with specific backends
		if len(backendNames) > 0 {
			return errors.New("cannot use --all switch and specify individual backends")
		}

		// Get list of backend names so we can delete them all
		backendNames, err = GetBackends(baseURL)
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
		url := baseURL + "/backend/" + backendName

		response, _, err := api.InvokeRESTAPI("DELETE", url, nil, Debug)
		if err != nil {
			return err
		} else if response.StatusCode != http.StatusOK {
			return fmt.Errorf("could not delete backend %s. %v", backendName, response.Status)
		}
	}

	return nil
}
