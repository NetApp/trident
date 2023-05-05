// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

var backendState string

func init() {
	updateBackendCmd.AddCommand(updateBackendStateCmd)
	updateBackendStateCmd.Flags().StringVarP(&backendState, "state", "", "", "New backend state")
}

var updateBackendStateCmd = &cobra.Command{
	Use:     "state <name> <state>",
	Short:   "Update a backend's state in Trident",
	Aliases: []string{"s"},
	Hidden:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		newBackendState, err := getBackendState()
		if err != nil {
			return err
		}

		if OperatingMode == ModeTunnel {
			command := []string{
				"update", "backend", "state", "--state", backendState,
			}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return backendUpdateState(args, newBackendState)
		}
	},
}

func getBackendState() (string, error) {
	if backendState == "" {
		return "", errors.New("no state was specified")
	}

	backendState = strings.ToLower(backendState)
	return backendState, nil
}

func backendUpdateState(backendNames []string, backendState string) error {
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
		State: backendState,
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
