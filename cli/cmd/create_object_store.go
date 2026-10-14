// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
)

var (
	createObjectStoreFilename   string
	createObjectStoreBase64Data string
)

func init() {
	createCmd.AddCommand(createObjectStoreCmd)
	createObjectStoreCmd.Flags().StringVarP(&createObjectStoreFilename, "filename", "f", "", "Path to YAML or JSON file")
	createObjectStoreCmd.Flags().StringVarP(&createObjectStoreBase64Data, "base64", "", "", "Base64 encoding")
	if err := createObjectStoreCmd.Flags().MarkHidden("base64"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

var createObjectStoreCmd = &cobra.Command{
	Use:     "objectstore",
	Short:   "Add an object store to Trident",
	Aliases: []string{"os"},
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonData, err := getBackendData(createObjectStoreFilename, createObjectStoreBase64Data)
		if err != nil {
			return err
		}

		if OperatingMode == ModeTunnel {
			command := []string{"create", "objectstore", "--base64", base64.StdEncoding.EncodeToString(jsonData)}
			out, err := TunnelCommand(command)
			printOutput(cmd, out, err)
			return err
		}

		return objectStoreCreate(jsonData)
	},
}

func objectStoreCreate(postData []byte) error {
	url := BaseURL() + "/objectstore"

	response, responseBody, err := api.InvokeRESTAPI(http.MethodPost, url, postData)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("could not create object store: %v", GetErrorFromHTTPResponse(response, responseBody))
	}

	var addObjectStoreResponse rest.AddObjectStoreResponse
	if err = json.Unmarshal(responseBody, &addObjectStoreResponse); err != nil {
		return err
	}

	if addObjectStoreResponse.ObjectStoreID != "" {
		fmt.Println(addObjectStoreResponse.ObjectStoreID)
	}

	return nil
}
