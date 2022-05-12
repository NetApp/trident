// Copyright 2022 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
)

var allStorageClasses bool

func init() {
	deleteCmd.AddCommand(deleteStorageClassCmd)
	deleteStorageClassCmd.Flags().BoolVarP(&allStorageClasses, "all", "", false, "Delete all storage classes")
}

var deleteStorageClassCmd = &cobra.Command{
	Use:     "storageclass <name> [<name>...]",
	Short:   "Delete one or more storage classes from Trident",
	Aliases: []string{"sc", "storageclasses"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"delete", "storageclass"}
			if allStorageClasses {
				command = append(command, "--all")
			}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return storageClassDelete(args)
		}
	},
}

func storageClassDelete(storageClassNames []string) error {
	var err error

	if allStorageClasses {
		// Make sure --all isn't being used along with specific storage classes
		if len(storageClassNames) > 0 {
			return errors.New("cannot use --all switch and specify individual storage classes")
		}

		// Get list of storage class names so we can delete them all
		storageClassNames, err = GetStorageClasses()
		if err != nil {
			return err
		}
	} else {
		// Not using --all, so make sure one or more storage classes were specified
		if len(storageClassNames) == 0 {
			return errors.New("storage class name not specified")
		}
	}

	for _, storageClassName := range storageClassNames {
		url := BaseURL() + "/storageclass/" + storageClassName

		response, responseBody, err := api.InvokeRESTAPI("DELETE", url, nil, Debug)
		if err != nil {
			return err
		} else if response.StatusCode != http.StatusOK {
			return fmt.Errorf("could not delete storage class %s: %v", storageClassName,
				GetErrorFromHTTPResponse(response, responseBody))
		}
	}

	return nil
}
