// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
)

func init() {
	createCmd.AddCommand(createBackendCmd)
	createBackendCmd.Flags().StringVarP(&filename, "filename", "f", "", "Path to YAML or JSON file")
	createBackendCmd.Flags().StringVarP(&b64Data, "base64", "", "", "Base64 encoding")
	createBackendCmd.Flags().MarkHidden("base64")
}

var createBackendCmd = &cobra.Command{
	Use:     "backend",
	Short:   "Add a backend to Trident",
	Aliases: []string{"b"},
	RunE: func(cmd *cobra.Command, args []string) error {

		jsonData, err := getBackendData()
		if err != nil {
			return err
		}

		if OperatingMode == ModeTunnel {
			command := []string{"create", "backend", "--base64", base64.StdEncoding.EncodeToString(jsonData)}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return backendCreate(jsonData)
		}
	},
}

func getBackendData() ([]byte, error) {

	var err error
	var rawData []byte

	if b64Data == "" && filename == "" {
		return nil, errors.New("no input file was specified")
	}

	// Read from file or stdin or b64 data
	if b64Data != "" {
		rawData, err = base64.StdEncoding.DecodeString(b64Data)
	} else if filename == "-" {
		rawData, err = ioutil.ReadAll(os.Stdin)
	} else {
		rawData, err = ioutil.ReadFile(filename)
	}
	if err != nil {
		return nil, err
	}

	// Ensure the file is valid JSON/YAML, and return JSON
	jsonData, err := yaml.YAMLToJSON(rawData)
	if err != nil {
		return nil, err
	}

	return jsonData, nil
}

func backendCreate(postData []byte) error {

	baseURL, err := GetBaseURL()
	if err != nil {
		return err
	}

	// Send the file to Trident
	url := baseURL + "/backend"

	response, responseBody, err := api.InvokeRESTAPI("POST", url, postData, Debug)
	if err != nil {
		return err
	} else if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("could not create backend: %v", GetErrorFromHTTPResponse(response, responseBody))
	}

	var addBackendResponse rest.AddBackendResponse
	err = json.Unmarshal(responseBody, &addBackendResponse)
	if err != nil {
		return err
	}

	backends := make([]api.Backend, 0, 1)
	backendName := addBackendResponse.BackendID

	// Retrieve the newly created backend and write to stdout
	backend, err := GetBackend(baseURL, backendName)
	if err != nil {
		return err
	}
	backends = append(backends, backend)

	WriteBackends(backends)

	return nil
}
