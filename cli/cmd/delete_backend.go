package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/netapp/trident/cli/api"
	"github.com/spf13/cobra"
)

var AllBackends bool

func init() {
	deleteCmd.AddCommand(deleteBackendCmd)
	deleteBackendCmd.Flags().BoolVarP(&AllBackends, "all", "", false, "Delete all backends")
}

var deleteBackendCmd = &cobra.Command{
	Use:     "backend",
	Short:   "Delete one or more storage backends from Trident",
	Aliases: []string{"b", "backends"},
	Run: func(cmd *cobra.Command, args []string) {
		if OperatingMode == MODE_TUNNEL {
			command := []string{"delete", "backend"}
			if AllBackends {
				command = append(command, "--all")
			}
			TunnelCommand(append(command, args...))
		} else {
			errs := backendDelete(args)
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
		}
	},
}

func backendDelete(backendNames []string) []error {

	errs := make([]error, 0, 1)

	baseURL, err := GetBaseURL()
	if err != nil {
		errs = append(errs, err)
		return errs
	}

	if AllBackends {
		// Make sure --all isn't being used along with specific backends
		if len(backendNames) > 0 {
			errs = append(errs, errors.New("cannot use --all switch and specify individual backends."))
			return errs
		}

		// Get list of backend names so we can delete them all
		backendNames, err = GetBackends(baseURL)
		if err != nil {
			errs = append(errs, err)
			return errs
		}
	} else {
		// Not using --all, so make sure one or more backends were specified
		if len(backendNames) == 0 {
			errs = append(errs, errors.New("backend name not specified."))
			return errs
		}
	}

	for _, backendName := range backendNames {
		url := baseURL + "/backend/" + backendName

		response, _, err := api.InvokeRestApi("DELETE", url, nil, Debug)
		if err != nil {
			errs = append(errs, err)
		} else if response.StatusCode != http.StatusOK {
			errs = append(errs, fmt.Errorf("could not delete backend %s. %v", backendName, response.Status))
		}
	}

	return errs
}
