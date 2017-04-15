package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/netapp/trident/cli/api"
	"github.com/spf13/cobra"
)

var AllVolumes bool

func init() {
	deleteCmd.AddCommand(deleteVolumeCmd)
	deleteVolumeCmd.Flags().BoolVarP(&AllVolumes, "all", "", false, "Delete all volumes")
}

var deleteVolumeCmd = &cobra.Command{
	Use:     "volume",
	Short:   "Delete one or more storage volumes from Trident",
	Aliases: []string{"b", "volumes"},
	Run: func(cmd *cobra.Command, args []string) {
		if OperatingMode == MODE_TUNNEL {
			command := []string{"delete", "volume"}
			if AllVolumes {
				command = append(command, "--all")
			}
			TunnelCommand(append(command, args...))
		} else {
			errs := volumeDelete(args)
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
		}
	},
}

func volumeDelete(volumeNames []string) []error {

	errs := make([]error, 0, 1)

	baseURL, err := GetBaseURL()
	if err != nil {
		errs = append(errs, err)
		return errs
	}

	if AllVolumes {
		// Make sure --all isn't being used along with specific volumes
		if len(volumeNames) > 0 {
			errs = append(errs, errors.New("cannot use --all switch and specify individual volumes."))
			return errs
		}

		// Get list of volume names so we can delete them all
		volumeNames, err = GetVolumes(baseURL)
		if err != nil {
			errs = append(errs, err)
			return errs
		}
	} else {
		// Not using --all, so make sure one or more volumes were specified
		if len(volumeNames) == 0 {
			errs = append(errs, errors.New("volume name not specified."))
			return errs
		}
	}

	for _, volumeName := range volumeNames {
		url := baseURL + "/volume/" + volumeName

		response, _, err := api.InvokeRestApi("DELETE", url, nil, Debug)
		if err != nil {
			errs = append(errs, err)
		} else if response.StatusCode != http.StatusOK {
			errs = append(errs, fmt.Errorf("could not delete volume %s. %v", volumeName, response.Status))
		}
	}

	return errs
}
