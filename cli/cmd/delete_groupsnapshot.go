package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

var allGroupSnapshots bool

func init() {
	deleteCmd.AddCommand(deleteGroupSnapshotCmd)
	deleteGroupSnapshotCmd.Flags().BoolVarP(&allGroupSnapshots, "all", "a", false, "Delete all group snapshots")
}

var deleteGroupSnapshotCmd = &cobra.Command{
	Use:     "groupsnapshot <group snapshot name>...",
	Short:   "Delete one or more group snapshots from Trident",
	Aliases: []string{"gs", "gsnap", "groupsnapshots"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"delete", "groupsnapshot"}

			if allGroupSnapshots {
				command = append(command, "--all")
			}

			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		}
		return groupSnapshotDelete(args...)
	},
}

func groupSnapshotDelete(groupSnapshotIDs ...string) error {
	// The --all switch cannot be used with specific snapshot IDs.
	if allGroupSnapshots && len(groupSnapshotIDs) > 0 {
		return errors.InvalidInputError("cannot use --all switch with individual group snapshots")
	} else if len(groupSnapshotIDs) == 0 && !allGroupSnapshots {
		return errors.InvalidInputError("must specify at least one group snapshot name or use --all switch")
	}

	if allGroupSnapshots {
		var err error
		groupSnapshotIDs, err = GetGroupSnapshots()
		if err != nil {
			return err
		}
	}

	for _, groupSnapshotID := range groupSnapshotIDs {
		if err := DeleteGroupSnapshot(groupSnapshotID); err != nil {
			return err
		}
	}
	return nil
}

func DeleteGroupSnapshot(groupID string) error {
	if _, err := storage.ConvertGroupSnapshotID(groupID); err != nil {
		return errors.InvalidInputError(fmt.Sprintf("invalid group snapshot ID %s: %v", groupID, err))
	}

	url := BaseURL() + "/groupsnapshot/" + groupID
	response, responseBody, err := api.InvokeRESTAPI("DELETE", url, nil)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		errorMessage := fmt.Sprintf("could not delete group snapshot %s: %v", groupID,
			GetErrorFromHTTPResponse(response, responseBody))
		switch response.StatusCode {
		case http.StatusNotFound:
			return errors.NotFoundError(errorMessage)
		default:
			return errors.New(errorMessage)
		}
	}

	return nil
}
