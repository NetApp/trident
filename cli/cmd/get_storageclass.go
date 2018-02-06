// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func init() {
	getCmd.AddCommand(getStorageClassCmd)
}

var getStorageClassCmd = &cobra.Command{
	Use:     "storageclass",
	Short:   "Get one or more storage classes from Trident",
	Aliases: []string{"sc", "storageclasses"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"get", "storageclass"}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return storageClassList(args)
		}
	},
}

func storageClassList(storageClassNames []string) error {

	baseURL, err := GetBaseURL()
	if err != nil {
		return err
	}

	// If no storage classes were specified, we'll get all of them
	if len(storageClassNames) == 0 {
		storageClassNames, err = GetStorageClasses(baseURL)
		if err != nil {
			return err
		}
	}

	storageClasses := make([]api.StorageClass, 0, 10)

	// Get the actual storage class objects
	for _, storageClassName := range storageClassNames {

		storageClass, err := GetStorageClass(baseURL, storageClassName)
		if err != nil {
			return err
		}
		storageClasses = append(storageClasses, storageClass)
	}

	WriteStorageClasses(storageClasses)

	return nil
}

func GetStorageClasses(baseURL string) ([]string, error) {

	url := baseURL + "/storageclass"

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil, Debug)
	if err != nil {
		return nil, err
	} else if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get storage classes. %v", response.Status)
	}

	var listStorageClassesResponse rest.ListStorageClassesResponse
	err = json.Unmarshal(responseBody, &listStorageClassesResponse)
	if err != nil {
		return nil, err
	}

	return listStorageClassesResponse.StorageClasses, nil
}

func GetStorageClass(baseURL, storageClassName string) (api.StorageClass, error) {

	url := baseURL + "/storageclass/" + storageClassName

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil, Debug)
	if err != nil {
		return api.StorageClass{}, err
	} else if response.StatusCode != http.StatusOK {
		return api.StorageClass{}, fmt.Errorf("could not get storage class %s. %v", storageClassName, response.Status)
	}

	var getStorageClassResponse api.GetStorageClassResponse
	err = json.Unmarshal(responseBody, &getStorageClassResponse)
	if err != nil {
		return api.StorageClass{}, err
	}

	return getStorageClassResponse.StorageClass, nil
}

func WriteStorageClasses(storageClasses []api.StorageClass) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(api.MultipleStorageClassResponse{storageClasses})
	case FormatYAML:
		WriteYAML(api.MultipleStorageClassResponse{storageClasses})
	case FormatName:
		writeStorageClassNames(storageClasses)
	default:
		writeStorageClassTable(storageClasses)
	}
}

func writeStorageClassTable(storageClasses []api.StorageClass) {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name"})

	for _, sc := range storageClasses {
		table.Append([]string{
			sc.Config.Name,
		})
	}

	table.Render()
}

func writeStorageClassNames(storageClasses []api.StorageClass) {

	for _, sc := range storageClasses {
		fmt.Println(sc.Config.Name)
	}
}
