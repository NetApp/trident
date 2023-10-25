// Copyright 2023 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

var (
	snapshotDirectory string
	poolLevel         string
)

func init() {
	snapshotDirFlag := "snapshot-dir"
	poolLevelFlag := "pool-level"

	updateCmd.AddCommand(updateVolumeCmd)
	updateVolumeCmd.Flags().StringVarP(&snapshotDirectory, snapshotDirFlag, "", "",
		"Value of snapshot directory. Allowed values: true|false")
	updateVolumeCmd.Flags().StringVarP(&poolLevel, poolLevelFlag, "", "false",
		"Whether update is to be done at pool level. Allowed values: true|false")
}

var updateVolumeCmd = &cobra.Command{
	Use:     "volume <name>",
	Short:   "Update a volume in Trident",
	Aliases: []string{"v"},
	RunE: func(cmd *cobra.Command, args []string) error {
		snapDir, poolLevelVal := snapshotDirectory, poolLevel

		// Validate command
		err := validateCmd(args, snapDir, poolLevelVal)
		if err != nil {
			return err
		}

		if OperatingMode == ModeTunnel {
			snapDirBool, _ := strconv.ParseBool(snapDir)

			command := []string{
				"update", "volume",
				"--snapshot-dir", strconv.FormatBool(snapDirBool),
				"--pool-level", poolLevelVal,
			}
			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		} else {
			return updateVolume(args[0], snapDir, poolLevelVal)
		}
	},
}

func validateCmd(args []string, snapshotDir, poolLevel string) error {
	// Ensure one and only one volume name is passed
	switch len(args) {
	case 0:
		return errors.New("volume name not specified")
	case 1:
		break
	default:
		return errors.New("multiple volume names specified")
	}

	// Ensure expected flags are present
	if snapshotDir == "" {
		return errors.New("no value for snapshot directory provided")
	}

	// Ensure flags are of the correct type
	_, err := strconv.ParseBool(snapshotDir)
	if err != nil {
		return err
	}

	_, err = strconv.ParseBool(poolLevel)
	if err != nil {
		return err
	}

	return nil
}

func updateVolume(volumeName, snapDirValue, poolLevelVal string) error {
	url := BaseURL() + "/volume/" + volumeName

	snapDirBool, _ := strconv.ParseBool(snapDirValue)
	poolLevelBool, _ := strconv.ParseBool(poolLevelVal)

	request := utils.VolumeUpdateInfo{
		SnapshotDirectory: strconv.FormatBool(snapDirBool),
		PoolLevel:         poolLevelBool,
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	response, responseBody, err := api.InvokeRESTAPI("PUT", url, requestBytes)

	if err != nil {
		return err
	} else if response.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update volume, error: %v", GetErrorFromHTTPResponse(response, responseBody))
	}

	var updateVolResponse rest.UpdateVolumeResponse
	err = json.Unmarshal(responseBody, &updateVolResponse)
	if err != nil {
		return err
	}

	// Write the response
	volumes := make([]storage.VolumeExternal, 0, 1)
	volumes = append(volumes, *updateVolResponse.Volume)
	WriteVolumes(volumes)

	return nil
}
