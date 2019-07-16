// Copyright 2019 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
)

var upgradeAllVolumes bool

func init() {
	upgradeCmd.AddCommand(upgradeVolumeCmd)
	upgradeVolumeCmd.Flags().BoolVarP(&upgradeAllVolumes, "all", "", false, "Upgrade all volumes")
}

var upgradeVolumeCmd = &cobra.Command{
	Use:     "volume <name> [<name>...]",
	Short:   "Upgrade one or more persistent volumes from NFS/iSCSI to CSI",
	Aliases: []string{"v", "volumes"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"upgrade", "volume"}
			if upgradeAllVolumes {
				command = append(command, "--all")
			}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return volumeUpgrade(args)
		}
	},
}

func volumeUpgrade(volumeNames []string) error {

	var err error

	if upgradeAllVolumes {
		// Make sure --all isn't being used along with specific volumes
		if len(volumeNames) > 0 {
			return errors.New("cannot use --all switch and specify individual volumes")
		}

		// Get list of volume names so we can upgrade them all
		volumeNames, err = GetVolumes()
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
		url := BaseURL() + "/volume/" + volumeName + "/upgrade"

		request := storage.UpgradeVolumeRequest{
			Type:   "csi",
			Volume: volumeName,
		}
		requestBytes, err := json.Marshal(request)
		if err != nil {
			return err
		}

		response, responseBody, err := api.InvokeRESTAPI("POST", url, requestBytes, Debug)
		if err != nil {
			return err
		} else if response.StatusCode != http.StatusOK {
			return fmt.Errorf("could not upgrade volume %s: %v", volumeName,
				GetErrorFromHTTPResponse(response, responseBody))
		}

		var upgradeVolumeResponse rest.UpgradeVolumeResponse
		if err = json.Unmarshal(responseBody, &upgradeVolumeResponse); err != nil {
			return err
		}

		volumes := []storage.VolumeExternal{*upgradeVolumeResponse.Volume}
		WriteVolumes(volumes)
	}

	return nil
}
