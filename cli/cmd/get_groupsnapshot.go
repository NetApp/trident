package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

// defaultMaxSnapshotRows is the maximum amount of IDs to display before truncating the output.
const defaultMaxSnapshotRows = 3

// maxSnapshotRows is the maximum number of constituent snapshots to display in the wide output format.
// It defaults to defaultMaxSnapshotRows.
var maxSnapshotRows int

func init() {
	getCmd.AddCommand(getGroupSnapshotCmd)
	getGroupSnapshotCmd.Flags().IntVar(
		&maxSnapshotRows,
		"max-snapshots",
		defaultMaxSnapshotRows,
		"Set the maximum number of snapshots (inclusive) to display when displaying grouped snapshots.\n"+
			"Setting this to 0 will show all snapshots in the group.",
	)
}

// getGroupSnapshotCmd is the command for retrieving group snapshots from Trident.
var getGroupSnapshotCmd = &cobra.Command{
	Use:     "groupsnapshot [<group snapshot name>...]",
	Short:   "Get one or more group snapshots from Trident",
	Aliases: []string{"gs", "gsnap", "groupsnapshots"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Fail if maxSnapshotRows is negative.
		if maxSnapshotRows < 0 {
			return errors.InvalidInputError("invalid value for --max-snapshots switch")
		}

		if OperatingMode == ModeTunnel {
			command := []string{"get", "groupsnapshot"}
			command = append(command, "--max-snapshots", strconv.Itoa(maxSnapshotRows))
			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		}
		return groupSnapshotList(args...)
	},
}

func groupSnapshotList(groupSnapshotIDs ...string) error {
	// Get all group snapshots if no group snapshot IDs are provided.
	if len(groupSnapshotIDs) == 0 {
		var err error
		groupSnapshotIDs, err = GetGroupSnapshots()
		if err != nil {
			return err
		}
	}

	groupSnapshots := make([]storage.GroupSnapshotExternal, 0, 10)
	for _, groupSnapshotID := range groupSnapshotIDs {
		groupSnapshot, err := GetGroupSnapshot(groupSnapshotID)
		if err != nil {
			return err
		}
		groupSnapshots = append(groupSnapshots, groupSnapshot)
	}

	WriteGroupSnapshots(groupSnapshots)
	return nil
}

// GetGroupSnapshot retrieves a concrete group snapshot by ID from Trident.
func GetGroupSnapshot(groupID string) (storage.GroupSnapshotExternal, error) {
	if _, err := storage.ConvertGroupSnapshotID(groupID); err != nil {
		return storage.GroupSnapshotExternal{}, errors.InvalidInputError(
			fmt.Sprintf("invalid group snapshot ID %s: %v", groupID, err))
	}

	url := BaseURL() + "/groupsnapshot/" + groupID
	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return storage.GroupSnapshotExternal{}, err
	}

	if response.StatusCode != http.StatusOK {
		errorMessage := fmt.Sprintf("could not get group snapshot %s: %v", groupID,
			GetErrorFromHTTPResponse(response, responseBody))
		switch response.StatusCode {
		case http.StatusNotFound:
			return storage.GroupSnapshotExternal{}, errors.NotFoundError(errorMessage)
		default:
			return storage.GroupSnapshotExternal{}, errors.New(errorMessage)
		}
	}

	// Parse the response body into a GroupSnapshotExternal object.
	var getGroupSnapshotResp rest.GetGroupSnapshotResponse
	err = json.Unmarshal(responseBody, &getGroupSnapshotResp)
	if err != nil {
		return storage.GroupSnapshotExternal{}, err
	}

	if getGroupSnapshotResp.GroupSnapshot == nil {
		return storage.GroupSnapshotExternal{}, fmt.Errorf("could not get group snapshot %s: no snapshot returned",
			groupID)
	}

	return *getGroupSnapshotResp.GroupSnapshot, nil
}

// GetGroupSnapshots retrieves all group snapshots from Trident.
func GetGroupSnapshots() ([]string, error) {
	url := BaseURL() + "/groupsnapshot"
	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get group snapshots: %v", GetErrorFromHTTPResponse(response, responseBody))
	}

	var listGroupSnapshotsResponse rest.ListGroupSnapshotsResponse
	err = json.Unmarshal(responseBody, &listGroupSnapshotsResponse)
	if err != nil {
		return nil, err
	}
	if listGroupSnapshotsResponse.GroupSnapshots == nil {
		return nil, fmt.Errorf("could not get group snapshots; no group snapshots found")
	}

	return listGroupSnapshotsResponse.GroupSnapshots, nil
}

func WriteGroupSnapshots(groupSnapshots []storage.GroupSnapshotExternal) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(api.MultipleGroupSnapshotResponse{Items: groupSnapshots})
	case FormatYAML:
		WriteYAML(api.MultipleGroupSnapshotResponse{Items: groupSnapshots})
	case FormatName:
		writeGroupSnapshotIDs(groupSnapshots)
	case FormatWide:
		writeWideGroupSnapshotTable(groupSnapshots)
	default:
		writeGroupSnapshotTable(groupSnapshots)
	}
}

func writeGroupSnapshotIDs(groups []storage.GroupSnapshotExternal) {
	for _, group := range groups {
		fmt.Println(group.ID())
	}
}

func writeGroupSnapshotTable(groupSnapshots []storage.GroupSnapshotExternal) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name"})

	for _, group := range groupSnapshots {
		table.Append([]string{
			group.ID(),
		})
	}

	table.Render()
}

func formatIDs(ids []string, max int) string {
	if len(ids) == 0 {
		return ""
	}

	// If max is zero or greater than the number of IDs, show the IDs.
	if max == 0 || len(ids) <= max {
		return strings.Join(ids, "\n")
	}

	// If len(ids) > max, truncate the output to max IDs and append the overflow count.
	displayIDs := strings.Join(ids[:max], "\n")
	overflowIDs := len(ids) - max
	return fmt.Sprintf("%s\n... (+%d more)", displayIDs, overflowIDs)
}

func writeWideGroupSnapshotTable(groupSnapshots []storage.GroupSnapshotExternal) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{
		"Name",
		"Created",
		"Snapshots",
		"Snapshot Count",
	})

	for _, group := range groupSnapshots {
		table.Append([]string{
			group.ID(),
			group.GetCreated(),
			formatIDs(group.GetSnapshotIDs(), maxSnapshotRows),
			fmt.Sprintf("%d", len(group.GetSnapshotIDs())),
		})
	}

	table.Render()
}
