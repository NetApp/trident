// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/utils/errors"
)

func init() {
	deleteCmd.AddCommand(deleteObjectStoreCmd)
}

var deleteObjectStoreCmd = &cobra.Command{
	Use:     "objectstore <name> [<name>...]",
	Short:   "Delete one or more object stores from Trident",
	Aliases: []string{"os", "objectstores"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"delete", "objectstore"}
			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		}
		return objectStoreDelete(args)
	},
}

func objectStoreDelete(objectStoreNames []string) error {
	if len(objectStoreNames) == 0 {
		return errors.New("object store name not specified")
	}

	for _, objectStoreName := range objectStoreNames {
		if objectStoreName == "" {
			continue
		}

		url := BaseURL() + "/objectstore/" + objectStoreName

		response, responseBody, err := api.InvokeRESTAPI(http.MethodDelete, url, nil)
		if err != nil {
			return err
		} else if response.StatusCode != http.StatusOK {
			return fmt.Errorf("could not delete object store %s: %v", objectStoreName,
				GetErrorFromHTTPResponse(response, responseBody))
		}
	}

	return nil
}
