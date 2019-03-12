// Copyright 2019 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
)

var (
	allSnapshots         bool
	allSnapshotsInVolume string
)

func init() {
	deleteCmd.AddCommand(deleteSnapshotCmd)
	deleteSnapshotCmd.Flags().BoolVar(&allSnapshots, "all", false, "Delete all snapshots")
	deleteSnapshotCmd.Flags().StringVar(&allSnapshotsInVolume, "volume", "", "Delete all snapshots in volume")
}

var deleteSnapshotCmd = &cobra.Command{
	Use:     "snapshot <volume/snapshot> [<volume/snapshot>...]",
	Short:   "Delete one or more volume snapshots from Trident",
	Aliases: []string{"s", "snap", "snapshots"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"delete", "snapshot"}
			if allSnapshotsInVolume != "" {
				command = append(command, "--volume", allSnapshotsInVolume)
			}
			if allSnapshots {
				command = append(command, "--all")
			}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return snapshotDelete(args)
		}
	},
}

func snapshotDelete(snapshotIDs []string) error {

	baseURL, err := GetBaseURL()
	if err != nil {
		return err
	}

	if allSnapshotsInVolume != "" {
		// Make sure --volume isn't being used along with specific snapshots
		if len(snapshotIDs) > 0 {
			return errors.New("cannot use --volume switch and specify individual snapshots")
		}

		// Get list of snapshot IDs in the specified volume so we can delete them all
		snapshotIDs, err = GetSnapshots(baseURL, allSnapshotsInVolume)
		if err != nil {
			return err
		}

	} else if allSnapshots {
		// Make sure --all isn't being used along with specific snapshots
		if len(snapshotIDs) > 0 {
			return errors.New("cannot use --all switch and specify individual snapshots")
		}

		// Get list of snapshot IDs so we can delete them all
		snapshotIDs, err = GetSnapshots(baseURL, "")
		if err != nil {
			return err
		}
	} else {
		// Not using --all or --volume, so make sure one or more volumes were specified
		if len(snapshotIDs) == 0 {
			return errors.New("volume/snapshot not specified")
		}
	}

	for _, snapshotID := range snapshotIDs {
		url := baseURL + "/snapshot/" + snapshotID

		response, responseBody, err := api.InvokeRESTAPI("DELETE", url, nil, Debug)
		if err != nil {
			return err
		} else if response.StatusCode != http.StatusOK {
			return fmt.Errorf("could not delete snapshot %s: %v", snapshotID,
				GetErrorFromHTTPResponse(response, responseBody))
		}
	}

	return nil
}
