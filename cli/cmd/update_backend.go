// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
)

var (
	updateFilename   string
	updateBase64Data string
)

func init() {
	updateCmd.AddCommand(updateBackendCmd)
	updateBackendCmd.Flags().StringVarP(&updateFilename, "filename", "f", "", "Path to YAML or JSON file")
	updateBackendCmd.Flags().StringVarP(&updateBase64Data, "base64", "", "", "Base64 encoding")
	if err := updateBackendCmd.Flags().MarkHidden("base64"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

var updateBackendCmd = &cobra.Command{
	Use:     "backend <name>",
	Short:   "Update a backend in Trident",
	Aliases: []string{"b"},
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonData, err := getBackendData(updateFilename, updateBase64Data)
		if err != nil {
			return err
		}

		if OperatingMode == ModeTunnel {
			command := []string{
				"update", "backend",
				"--base64", base64.StdEncoding.EncodeToString(jsonData),
			}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return backendUpdate(args, jsonData)
		}
	},
}

func backendUpdate(backendNames []string, postData []byte) error {
	switch len(backendNames) {
	case 0:
		return errors.New("backend name not specified")
	case 1:
		break
	default:
		return errors.New("multiple backend names specified")
	}

	// Send the file to Trident
	url := BaseURL() + "/backend/" + backendNames[0]

	response, responseBody, err := api.InvokeRESTAPI("POST", url, postData)
	if err != nil {
		return err
	} else if response.StatusCode != http.StatusOK {
		return fmt.Errorf("could not update backend %s: %v", backendNames[0],
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
