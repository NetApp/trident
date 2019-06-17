// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/netapp/trident/cli/api"
	"github.com/spf13/cobra"
)

var allVolumes bool

func init() {
	deleteCmd.AddCommand(deleteVolumeCmd)
	deleteVolumeCmd.Flags().BoolVarP(&allVolumes, "all", "", false, "Delete all volumes")
}

var deleteVolumeCmd = &cobra.Command{
	Use:     "volume <name> [<name>...]",
	Short:   "Delete one or more storage volumes from Trident",
	Aliases: []string{"v", "volumes"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"delete", "volume"}
			if allVolumes {
				command = append(command, "--all")
			}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return volumeDelete(args)
		}
	},
}

func volumeDelete(volumeNames []string) error {

	baseURL, err := GetBaseURL()
	if err != nil {
		return err
	}

	if allVolumes {
		// Make sure --all isn't being used along with specific volumes
		if len(volumeNames) > 0 {
			return errors.New("cannot use --all switch and specify individual volumes")
		}

		// Get list of volume names so we can delete them all
		volumeNames, err = GetVolumes(baseURL)
		if err != nil {
			return err
		}
	} else {
		// Not using --all, so make sure one or more volumes were specified
		if len(volumeNames) == 0 {
			return errors.New("volume name not specified")
		}
	}

	for _, volumeName := range volumeNames {
		url := baseURL + "/volume/" + volumeName

		response, responseBody, err := api.InvokeRESTAPI("DELETE", url, nil, Debug)
		if err != nil {
			return err
		} else if response.StatusCode != http.StatusOK {
			return fmt.Errorf("could not delete volume %s: %v", volumeName,
				GetErrorFromHTTPResponse(response, responseBody))
		}
	}

	return nil
}
