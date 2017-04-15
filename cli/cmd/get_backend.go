package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func init() {
	getCmd.AddCommand(getBackendCmd)
}

var getBackendCmd = &cobra.Command{
	Use:     "backend",
	Short:   "Get one or more storage backends from Trident",
	Aliases: []string{"b", "backends"},
	Run: func(cmd *cobra.Command, args []string) {
		if OperatingMode == MODE_TUNNEL {
			command := []string{"get", "backend"}
			TunnelCommand(append(command, args...))
		} else {
			err := backendList(args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
		}
	},
}

func backendList(backendNames []string) error {

	baseURL, err := GetBaseURL()
	if err != nil {
		return err
	}

	// If no backends were specified, we'll get all of them
	if len(backendNames) == 0 {
		backendNames, err = GetBackends(baseURL)
		if err != nil {
			return err
		}
	}

	backends := make([]api.Backend, 0, 10)

	// Get the actual backend objects
	for _, backendName := range backendNames {

		backend, err := GetBackend(baseURL, backendName)
		if err != nil {
			return err
		}
		backends = append(backends, backend)
	}

	WriteBackends(backends)

	return nil
}

func GetBackends(baseURL string) ([]string, error) {

	url := baseURL + "/backend"

	response, responseBody, err := api.InvokeRestApi("GET", url, nil, Debug)
	if err != nil {
		return nil, err
	} else if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get backends. %v", response.Status)
	}

	var listBackendsResponse rest.ListBackendsResponse
	err = json.Unmarshal(responseBody, &listBackendsResponse)
	if err != nil {
		return nil, err
	}

	return listBackendsResponse.Backends, nil
}

func GetBackend(baseURL, backendName string) (api.Backend, error) {

	url := baseURL + "/backend/" + backendName

	response, responseBody, err := api.InvokeRestApi("GET", url, nil, Debug)
	if err != nil {
		return api.Backend{}, err
	} else if response.StatusCode != http.StatusOK {
		return api.Backend{}, fmt.Errorf("could not get backend %s. %v", backendName, response.Status)
	}

	var getBackendResponse api.GetBackendResponse
	err = json.Unmarshal(responseBody, &getBackendResponse)
	if err != nil {
		return api.Backend{}, err
	}

	return getBackendResponse.Backend, nil
}

func WriteBackends(backends []api.Backend) {
	switch OutputFormat {
	case FORMAT_JSON:
		WriteJSON(api.MultipleBackendResponse{backends})
	case FORMAT_YAML:
		WriteYAML(api.MultipleBackendResponse{backends})
	case FORMAT_NAME:
		writeBackendNames(backends)
	default:
		writeBackendTable(backends)
	}
}

func writeBackendTable(backends []api.Backend) {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Storage Driver", "Online", "Volumes"})

	for _, b := range backends {
		table.Append([]string{
			b.Name,
			b.Config.StorageDriverName,
			strconv.FormatBool(b.Online),
			strconv.Itoa(len(b.Volumes)),
		})
	}

	table.Render()
}

func writeBackendNames(backends []api.Backend) {

	for _, b := range backends {
		fmt.Println(b.Name)
	}
}
