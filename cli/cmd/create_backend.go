package cmd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/spf13/cobra"
)

var filename string
var b64Data string

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

		postData, err := getBackendCreateData()
		if err != nil {
			return err
		}

		if OperatingMode == MODE_TUNNEL {
			command := []string{"create", "backend", "--base64", base64.StdEncoding.EncodeToString(postData)}
			TunnelCommand(append(command, args...))
		} else {
			err := backendCreate(postData)
			if err != nil {
				return err
			}
		}
		return nil
	},
}

func getBackendCreateData() ([]byte, error) {

	var err error
	var postData []byte

	if b64Data == "" && filename == "" {
		return nil, errors.New("No input file was specified.")
	}

	// Read from file or stdin or b64 data
	if b64Data != "" {
		postData, err = base64.StdEncoding.DecodeString(b64Data)
	} else if filename == "-" {
		postData, err = ioutil.ReadAll(os.Stdin)
	} else {
		postData, err = ioutil.ReadFile(filename)
	}
	if err != nil {
		return nil, err
	}

	return postData, nil
}

func backendCreate(postData []byte) error {

	baseURL, err := GetBaseURL()
	if err != nil {
		return err
	}

	// Send the file to Trident
	url := baseURL + "/backend"

	response, responseBody, err := api.InvokeRestApi("POST", url, postData, Debug)
	if err != nil {
		return err
	} else if response.StatusCode != http.StatusCreated {
		return errors.New(response.Status)
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
