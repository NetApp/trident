// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

var (
	backendState string
	userState    string
)

func init() {
	updateBackendCmd.AddCommand(updateBackendStateCmd)
	updateBackendStateCmd.Flags().StringVarP(&backendState, "state", "", "", "New backend state")
	updateBackendStateCmd.Flags().StringVarP(&userState, "user-state", "", "", "User-defined backend state (suspended, normal)")
	// making the flag hidden, as it is for internal testing purposes only.
	err := updateBackendStateCmd.Flags().MarkHidden("state")
	if err != nil {
		return
	}
}

var updateBackendStateCmd = &cobra.Command{
	Use:     "state <name> <user-state>",
	Short:   "Update a backend's state in Trident",
	Aliases: []string{"s"},
	PreRunE: validateUpdateBackendStateArguments,
	RunE:    updateBackendStateRunE,
}

func validateUpdateBackendStateArguments(cmd *cobra.Command, args []string) error {
	// Only one of the following flags may be set.
	flags := []string{"state", "user-state"}
	flagSet := 0

	for _, flag := range flags {
		if cmd.Flags().Changed(flag) {
			flagSet++
		}
	}

	if flagSet != 1 {
		return errors.New("exactly one of --state or --user-state must be specified")
	}

	return nil
}

func updateBackendStateRunE(cmd *cobra.Command, args []string) error {
	if OperatingMode == ModeTunnel {
		// setting the command based on the selected flag.
		var command []string
		if cmd.Flags().Changed("user-state") {
			command = []string{
				"update", "backend", "state", "--user-state", userState,
			}
		}
		if cmd.Flags().Changed("state") {
			command = []string{
				"update", "backend", "state", "--state", backendState,
			}
		}
		out, err := TunnelCommand(append(command, args...))
		printOutput(cmd, out, err)
		return err
	} else {
		return backendUpdateState(args)
	}
}

func backendUpdateState(backendNames []string) error {
	switch len(backendNames) {
	case 0:
		return errors.New("backend name not specified")
	case 1:
		break
	default:
		return errors.New("multiple backend names specified")
	}

	// Send the new backend state to Trident
	url := BaseURL() + "/backend/" + backendNames[0] + "/state"

	request := storage.UpdateBackendStateRequest{
		BackendState:     backendState,
		UserBackendState: userState,
	}
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	response, responseBody, err := api.InvokeRESTAPI("POST", url, requestBytes)
	if err != nil {
		return err
	} else if response.StatusCode != http.StatusOK {
		return fmt.Errorf("could not update state for backend %s: %v", backendNames[0],
			GetErrorFromHTTPResponse(response, responseBody))
	}

	var updateBackendResponse rest.UpdateBackendResponse
	err = json.Unmarshal(responseBody, &updateBackendResponse)
	if err != nil {
		return err
	}

	backends := make([]storage.BackendExternal, 0, 1)
	backendName := updateBackendResponse.BackendID

	// Retrieve the updated backend and write to stdout
	backend, err := GetBackend(backendName)
	if err != nil {
		return err
	}
	backends = append(backends, backend)

	WriteBackends(backends)

	return nil
}
