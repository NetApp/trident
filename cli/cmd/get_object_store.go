// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/utils/errors"
)

func init() {
	getCmd.AddCommand(getObjectStoreCmd)
}

var getObjectStoreCmd = &cobra.Command{
	Use:     "objectstore [<name>...]",
	Short:   "Get one or more object stores from Trident",
	Aliases: []string{"os", "objectstores"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"get", "objectstore"}
			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		}
		return objectStoreList(args)
	},
}

func objectStoreList(objectStoreNames []string) error {
	var err error

	// If no object stores were specified, we'll get all of them
	getAll := false
	if len(objectStoreNames) == 0 {
		getAll = true
		objectStoreNames, err = GetObjectStores()
		if err != nil {
			return err
		}
	}

	objectStores := make([]map[string]interface{}, 0, len(objectStoreNames))

	for _, objectStoreName := range objectStoreNames {
		objectStore, err := GetObjectStore(objectStoreName)
		if err != nil {
			if getAll && errors.IsNotFoundError(err) {
				continue
			}
			return err
		}
		objectStores = append(objectStores, objectStore)
	}

	WriteObjectStores(objectStores)

	return nil
}

func GetObjectStores() ([]string, error) {
	url := BaseURL() + "/objectstore"

	response, responseBody, err := api.InvokeRESTAPI(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	} else if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get object stores: %v",
			GetErrorFromHTTPResponse(response, responseBody))
	}

	var listObjectStoresResponse rest.ListObjectStoresResponse
	if err = json.Unmarshal(responseBody, &listObjectStoresResponse); err != nil {
		return nil, err
	}

	return listObjectStoresResponse.ObjectStores, nil
}

func GetObjectStore(objectStoreName string) (map[string]interface{}, error) {
	url := BaseURL() + "/objectstore/" + objectStoreName

	response, responseBody, err := api.InvokeRESTAPI(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	} else if response.StatusCode != http.StatusOK {
		errorMessage := fmt.Sprintf("could not get object store %s: %v", objectStoreName,
			GetErrorFromHTTPResponse(response, responseBody))
		switch response.StatusCode {
		case http.StatusNotFound:
			return nil, errors.NotFoundError(errorMessage)
		default:
			return nil, errors.New(errorMessage)
		}
	}

	var getObjectStoreResponse api.GetObjectStoreResponse
	if err = json.Unmarshal(responseBody, &getObjectStoreResponse); err != nil {
		return nil, err
	}

	return getObjectStoreResponse.ObjectStore, nil
}

func WriteObjectStores(objectStores []map[string]interface{}) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(api.MultipleObjectStoreResponse{Items: objectStores})
	case FormatYAML:
		WriteYAML(api.MultipleObjectStoreResponse{Items: objectStores})
	case FormatName:
		writeObjectStoreNames(objectStores)
	default:
		writeObjectStoreTable(objectStores)
	}
}

func writeObjectStoreTable(objectStores []map[string]interface{}) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Name", "Storage Driver", "UUID", "State"})

	for _, s := range objectStores {
		_ = table.Append([]string{
			fmt.Sprint(s["objectStoreName"]),
			fmt.Sprint(s["objectStoreDriver"]),
			fmt.Sprint(s["uuid"]),
			fmt.Sprint(s["state"]),
		})
	}

	_ = table.Render()
}

func writeObjectStoreNames(objectStores []map[string]interface{}) {
	for _, s := range objectStores {
		fmt.Println(s["objectStoreName"])
	}
}
