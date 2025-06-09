// Copyright 2019 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

const maskDisplayOfSnapshotStateOnline = storage.SnapshotState("") // Used for display in 'tridentctl' query

var (
	getSnapshotVolume string
	getSnapshotGroup  string

	// getSnapshotFlagValues is used to store the REST API URL parameters for the 'delete snapshot' command.
	getSnapshotFlagValues map[string]string
)

func init() {
	getCmd.AddCommand(getSnapshotCmd)
	getSnapshotCmd.Flags().StringVar(&getSnapshotVolume, "volume", "", "Limit query to volume "+
		"(unless additional arguments are provided)")
	getSnapshotCmd.Flags().StringVar(&getSnapshotGroup, "group", "", "Limit query to group")

	// Only one of these flags may be specified at a time.
	getSnapshotCmd.MarkFlagsMutuallyExclusive("volume", "group")
}

var getSnapshotCmd = &cobra.Command{
	Use:     "snapshot [<volume name>/<snapshot name>...]",
	Short:   "Get one or more snapshots from Trident",
	Aliases: []string{"s", "snap", "snapshots"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Build a map of URL parameters for the 'get snapshot' command.
		getSnapshotFlagValues = make(map[string]string)
		cmd.Flags().Visit(func(flag *pflag.Flag) {
			// Store the flag name and value in getSnapshotFlagParams for use in REST API calls.
			getSnapshotFlagValues[flag.Name] = flag.Value.String()
		})

		if OperatingMode == ModeTunnel {
			command := []string{"get", "snapshot"}

			volume, ok := getSnapshotFlagValues["volume"]
			if ok && volume != "" {
				command = append(command, "--volume", volume)
			}
			group, ok := getSnapshotFlagValues["group"]
			if ok && group != "" {
				command = append(command, "--group", group)
			}

			// Fail if both volume and group are specified. This "should" be caught by the CLI framework, but
			// in the case it's not, check here.
			if volume != "" && group != "" {
				return errors.InvalidInputError("cannot specify both --volume and --group flags")
			}

			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		}
		return snapshotList(args)
	},
}

// buildSnapshotURL accepts a map of flag names to values and builds a URL with said values embedded for
// 'get|delete snapshot' commands. If a 'volume' or 'group' is specified, it will build the URL accordingly.
// These flags are mutually exclusive. It will select the first non-empty value for volume or group and
// return the appropriate URL.
func buildSnapshotURL(params map[string]string) string {
	if volume, ok := params["volume"]; ok && volume != "" {
		return BaseURL() + "/snapshot" + "?volume=" + volume
	}

	if group, ok := params["group"]; ok && group != "" {
		return BaseURL() + "/snapshot" + "?group=" + group
	}

	return BaseURL() + "/snapshot"
}

func snapshotList(snapshotIDs []string) error {
	var err error

	// If no snapshots were specified, we'll get all of them.
	getAll := len(snapshotIDs) == 0
	if getAll {
		// Build the URL here and supply it to get snapshots.
		snapshotIDs, err = GetSnapshots(buildSnapshotURL(getSnapshotFlagValues))
		if err != nil {
			return err
		}
	}

	// Get the actual snapshot objects
	snapshots := make([]storage.SnapshotExternal, 0, 10)
	for _, snapshotID := range snapshotIDs {
		snapshot, err := GetSnapshot(snapshotID)
		if err != nil {
			if getAll && errors.IsNotFoundError(err) {
				continue
			}
			return err
		}
		snapshots = append(snapshots, snapshot)
	}

	WriteSnapshots(snapshots)
	return nil
}

func GetSnapshots(url string) ([]string, error) {
	if url == "" {
		return nil, errors.InvalidInputError("snapshot URL cannot be empty")
	}

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// TODO: Remove this check after v26.10.0.
	if vol, ok := getSnapshotFlagValues["volume"]; ok && vol != "" && response.StatusCode == http.StatusNotFound {
		// If we failed with not found while attempting to list snapshots for a specific volume, try the legacy route.
		legacyURL := BaseURL() + "/volume/" + vol + "/snapshot"
		if Debug {
			fmt.Printf("Failed to get snapshots for volume '%s' using: '%s', "+
				"falling back to legacy route: '%s'\n", vol, url, legacyURL)
		}

		response, responseBody, err = api.InvokeRESTAPI("GET", legacyURL, nil)
		if err != nil {
			return nil, err
		}
	}

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get snapshots: %v", GetErrorFromHTTPResponse(response, responseBody))
	}

	var listSnapshotsResponse rest.ListSnapshotsResponse
	err = json.Unmarshal(responseBody, &listSnapshotsResponse)
	if err != nil {
		return nil, err
	}

	return listSnapshotsResponse.Snapshots, nil
}

func GetSnapshot(snapshotID string) (storage.SnapshotExternal, error) {
	if !strings.ContainsRune(snapshotID, '/') {
		return storage.SnapshotExternal{}, errors.InvalidInputError(fmt.Sprintf("invalid snapshot ID: %s; "+
			"Please use the format <volume name>/<snapshot name>", snapshotID))
	}

	url := BaseURL() + "/snapshot/" + snapshotID
	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return storage.SnapshotExternal{}, err
	} else if response.StatusCode != http.StatusOK {
		errorMessage := fmt.Sprintf("could not get snapshot %s: %v", snapshotID,
			GetErrorFromHTTPResponse(response, responseBody))
		switch response.StatusCode {
		case http.StatusNotFound:
			return storage.SnapshotExternal{}, errors.NotFoundError(errorMessage)
		default:
			return storage.SnapshotExternal{}, errors.New(errorMessage)
		}
	}

	var getSnapshotResponse rest.GetSnapshotResponse
	err = json.Unmarshal(responseBody, &getSnapshotResponse)
	if err != nil {
		return storage.SnapshotExternal{}, err
	}
	if getSnapshotResponse.Snapshot == nil {
		return storage.SnapshotExternal{}, fmt.Errorf("could not get snapshot %s: no snapshot returned",
			snapshotID)
	}

	if getSnapshotResponse.Snapshot.State == storage.SnapshotStateOnline {
		// Currently, this is used only for display, mask 'online' state as "".
		// If in future any callers use this attribute, need to take care of it.
		getSnapshotResponse.Snapshot.State = maskDisplayOfSnapshotStateOnline
	}

	return *getSnapshotResponse.Snapshot, nil
}

func WriteSnapshots(snapshots []storage.SnapshotExternal) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(api.MultipleSnapshotResponse{Items: snapshots})
	case FormatYAML:
		WriteYAML(api.MultipleSnapshotResponse{Items: snapshots})
	case FormatName:
		writeSnapshotIDs(snapshots)
	case FormatWide:
		writeWideSnapshotTable(snapshots)
	default:
		writeSnapshotTable(snapshots)
	}
}

func writeSnapshotTable(snapshots []storage.SnapshotExternal) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Volume", "Managed"})

	for _, snapshot := range snapshots {
		table.Append([]string{
			snapshot.Config.Name,
			snapshot.Config.VolumeName,
			strconv.FormatBool(!snapshot.Config.ImportNotManaged),
		})
	}

	table.Render()
}

func writeWideSnapshotTable(snapshots []storage.SnapshotExternal) {
	table := tablewriter.NewWriter(os.Stdout)
	header := []string{
		"Name",
		"Volume",
		"Created",
		"Size",
		"State",
		"Managed",
		"GroupSnapshot",
	}
	table.SetHeader(header)

	for _, snapshot := range snapshots {
		table.Append([]string{
			snapshot.Config.Name,
			snapshot.Config.VolumeName,
			snapshot.Created,
			humanize.IBytes(uint64(snapshot.SizeBytes)),
			string(snapshot.State),
			strconv.FormatBool(!snapshot.Config.ImportNotManaged),
			snapshot.Config.GroupSnapshotName,
		})
	}

	table.Render()
}

func writeSnapshotIDs(snapshots []storage.SnapshotExternal) {
	for _, s := range snapshots {
		fmt.Println(storage.MakeSnapshotID(s.Config.VolumeName, s.Config.Name))
	}
}
