// Copyright 2019 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/utils/errors"
)

var (
	allSnapshots         bool
	allSnapshotsInVolume string
	allSnapshotsInGroup  string

	// deleteSnapshotFlagValues is used to store the REST API URL parameters for the 'delete snapshot' command.
	deleteSnapshotFlagValues map[string]string
)

func init() {
	deleteCmd.AddCommand(deleteSnapshotCmd)
	deleteSnapshotCmd.Flags().BoolVarP(&allSnapshots, "all", "a", false, "Delete all snapshots")
	deleteSnapshotCmd.Flags().StringVar(&allSnapshotsInVolume, "volume", "", "Delete all snapshots in volume")
	deleteSnapshotCmd.Flags().StringVar(&allSnapshotsInGroup, "group", "", "Delete all snapshots in a group")

	// Only one of these flags may be specified at a time.
	deleteSnapshotCmd.MarkFlagsMutuallyExclusive("volume", "group", "all")
}

var deleteSnapshotCmd = &cobra.Command{
	Use:     "snapshot <volume/snapshot> [<volume/snapshot>...]",
	Short:   "Delete one or more volume snapshots from Trident",
	Aliases: []string{"s", "snap", "snapshots"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Build a map of URL parameters for the 'delete snapshot' command.
		deleteSnapshotFlagValues = make(map[string]string)
		cmd.Flags().Visit(func(flag *pflag.Flag) {
			// Store the flag name and value in deleteSnapshotURLVar for use in REST API calls.
			deleteSnapshotFlagValues[flag.Name] = flag.Value.String()
		})

		if OperatingMode == ModeTunnel {
			command := []string{"delete", "snapshot"}

			volume, ok := deleteSnapshotFlagValues["volume"]
			if ok && volume != "" {
				command = append(command, "--volume", volume)
			}

			group, ok := deleteSnapshotFlagValues["group"]
			if ok && group != "" {
				// If a group is specified, we delete all snapshots in that group.
				// This typically isn't recommended, but we'll allow it for exceptional cases where
				// the group snapshot has been lost and the admin needs to clean up.
				command = append(command, "--group", group)
			}

			all, ok := deleteSnapshotFlagValues["all"]
			if ok && convert.ToBool(all) {
				command = append(command, "--all")
			}

			// Fail if a mix of flags is used that doesn't make sense. This should be caught by the CLI framework,
			// but in the case it's not, check here.
			if volume != "" && group != "" {
				return errors.InvalidInputError("cannot specify both --volume and --group flags")
			} else if volume != "" && convert.ToBool(all) {
				return errors.InvalidInputError("cannot specify --volume flag and --all flag")
			} else if group != "" && convert.ToBool(all) {
				return errors.InvalidInputError("cannot specify --group flag and --all flag")
			}

			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		}

		return snapshotDelete(args)
	},
}

func snapshotDelete(snapshotIDs []string) error {
	// The --all switch cannot be used with specific snapshot IDs.
	if all, _ := deleteSnapshotFlagValues["all"]; convert.ToBool(all) && len(snapshotIDs) > 0 {
		return errors.InvalidInputError("cannot use --all switch with individual snapshots")
	}

	// The --volume switch cannot be used with specific snapshot IDs.
	if volume, _ := deleteSnapshotFlagValues["volume"]; volume != "" && len(snapshotIDs) > 0 {
		return errors.InvalidInputError("cannot use --volume switch with individual snapshots")
	}

	// The --group switch cannot be used with specific snapshot IDs.
	if group, _ := deleteSnapshotFlagValues["group"]; group != "" && len(snapshotIDs) > 0 {
		return errors.InvalidInputError("cannot use --group switch with individual snapshots")
	}

	// If some snapshots were specified, we'll delete those.
	var err error
	if len(snapshotIDs) == 0 {
		snapshotIDs, err = GetSnapshots(buildSnapshotURL(deleteSnapshotFlagValues))
		if err != nil {
			return err
		}
	}

	var errs error
	for _, snapshotID := range snapshotIDs {
		// Each snapshot must have volume specified.
		if !strings.ContainsRune(snapshotID, '/') {
			return errors.InvalidInputError(fmt.Sprintf("invalid snapshot ID: %s; Please use the format "+
				"<volume name>/<snapshot name>", snapshotID))
		}

		url := BaseURL() + "/snapshot/" + snapshotID

		response, responseBody, err := api.InvokeRESTAPI("DELETE", url, nil)
		if err != nil {
			return err
		} else if response.StatusCode != http.StatusOK {
			errs = errors.Join(errs, GetErrorFromHTTPResponse(response, responseBody))
		}
	}

	return errs
}
