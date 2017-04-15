package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/netapp/trident/cli/api"
	"github.com/spf13/cobra"
)

var AllStorageClasses bool

func init() {
	deleteCmd.AddCommand(deleteStorageClassCmd)
	deleteStorageClassCmd.Flags().BoolVarP(&AllStorageClasses, "all", "", false, "Delete all storage classes")
}

var deleteStorageClassCmd = &cobra.Command{
	Use:     "storageclass",
	Short:   "Delete one or more storage classes from Trident",
	Aliases: []string{"sc", "storageclasses"},
	Run: func(cmd *cobra.Command, args []string) {
		if OperatingMode == MODE_TUNNEL {
			command := []string{"delete", "storageclass"}
			if AllStorageClasses {
				command = append(command, "--all")
			}
			TunnelCommand(append(command, args...))
		} else {
			errs := storageClassDelete(args)
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
		}
	},
}

func storageClassDelete(storageClassNames []string) []error {

	errs := make([]error, 0, 1)

	baseURL, err := GetBaseURL()
	if err != nil {
		errs = append(errs, err)
		return errs
	}

	if AllStorageClasses {
		// Make sure --all isn't being used along with specific storage classes
		if len(storageClassNames) > 0 {
			errs = append(errs, errors.New("cannot use --all switch and specify individual storage classes."))
			return errs
		}

		// Get list of storage class names so we can delete them all
		storageClassNames, err = GetStorageClasses(baseURL)
		if err != nil {
			errs = append(errs, err)
			return errs
		}
	} else {
		// Not using --all, so make sure one or more storage classes were specified
		if len(storageClassNames) == 0 {
			errs = append(errs, errors.New("storage class name not specified."))
			return errs
		}
	}

	for _, storageClassName := range storageClassNames {
		url := baseURL + "/storageclass/" + storageClassName

		response, _, err := api.InvokeRestApi("DELETE", url, nil, Debug)
		if err != nil {
			errs = append(errs, err)
		} else if response.StatusCode != http.StatusOK {
			errs = append(errs, fmt.Errorf("could not delete storage class %s. %v", storageClassName, response.Status))
		}
	}

	return errs
}
