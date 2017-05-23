package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/utils"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of Trident",
	Long:  "Print the version of the Trident storage orchestrator for Kubernetes",
	RunE: func(cmd *cobra.Command, args []string) error {

		var serverVersion rest.GetVersionResponse
		var err error

		// Get the server version
		if OperatingMode == MODE_TUNNEL {
			serverVersion, err = getVersionFromTunnel()

		} else {
			serverVersion, err = getVersionFromRest()
		}

		if err != nil {
			return fmt.Errorf("Error: %v\n", err)
		}

		parsedServerVersion, err := utils.ParseDate(serverVersion.Version)
		if err != nil {
			return fmt.Errorf("Error: %v\n", err)
		}

		// Add the client version, which is always hardcoded at compile time
		versions := addClientVersion(parsedServerVersion)

		writeVersions(versions)

		return nil
	},
}

// getVersion retrieves the Trident server version directly using the REST API
func getVersionFromRest() (rest.GetVersionResponse, error) {

	baseURL, err := GetBaseURL()
	if err != nil {
		return rest.GetVersionResponse{}, err
	}

	url := baseURL + "/version"

	response, responseBody, err := api.InvokeRestApi("GET", url, nil, Debug)
	if err != nil {
		return rest.GetVersionResponse{}, err
	} else if response.StatusCode != http.StatusOK {
		return rest.GetVersionResponse{}, fmt.Errorf("could not get version. %v", response.Status)
	}

	var getVersionResponse rest.GetVersionResponse
	err = json.Unmarshal(responseBody, &getVersionResponse)
	if err != nil {
		return rest.GetVersionResponse{}, err
	}

	return getVersionResponse, nil
}

// getVersionFromTunnel retrieves the Trident server version using the exec tunnel
func getVersionFromTunnel() (rest.GetVersionResponse, error) {

	command := []string{"version", "-o", "json"}
	versionJson, err := TunnelCommandRaw(command)
	if err != nil {
		return rest.GetVersionResponse{}, err
	}

	if Debug {
		fmt.Printf("Version JSON: %s\n", versionJson)
	}

	var tunnelVersionResponse api.VersionResponse
	err = json.Unmarshal(versionJson, &tunnelVersionResponse)
	if err != nil {
		return rest.GetVersionResponse{}, err
	}

	version := rest.GetVersionResponse{
		Version: tunnelVersionResponse.Server.Version,
	}
	return version, nil
}

// addClientVersion accepts the server version and fills in the client version
func addClientVersion(serverVersion *utils.Version) api.VersionResponse {

	versions := api.VersionResponse{}

	versions.Server.Version = serverVersion.String()
	versions.Server.MajorVersion = serverVersion.MajorVersion()
	versions.Server.MinorVersion = serverVersion.MinorVersion()
	versions.Server.PatchVersion = serverVersion.PatchVersion()
	versions.Server.PreRelease = serverVersion.PreRelease()
	versions.Server.BuildMetadata = serverVersion.BuildMetadata()
	versions.Server.APIVersion = config.OrchestratorAPIVersion

	versions.Client.Version = config.OrchestratorVersion.String()
	versions.Client.MajorVersion = config.OrchestratorVersion.MajorVersion()
	versions.Client.MinorVersion = config.OrchestratorVersion.MinorVersion()
	versions.Client.PatchVersion = config.OrchestratorVersion.PatchVersion()
	versions.Client.PreRelease = config.OrchestratorVersion.PreRelease()
	versions.Client.BuildMetadata = config.OrchestratorVersion.BuildMetadata()
	versions.Client.APIVersion = config.OrchestratorAPIVersion

	return versions
}

func writeVersions(versions api.VersionResponse) {
	switch OutputFormat {
	case FORMAT_JSON:
		WriteJSON(versions)
	case FORMAT_YAML:
		WriteYAML(versions)
	case FORMAT_WIDE:
		writeWideVersionTable(versions)
	default:
		writeVersionTable(versions)
	}
}

func writeVersionTable(versions api.VersionResponse) {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Server Version", "Client Version"})

	table.Append([]string{
		versions.Server.Version,
		versions.Client.Version,
	})

	table.Render()
}

func writeWideVersionTable(versions api.VersionResponse) {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Server Version", "Server API Version", "Client Version", "Client API Version"})

	table.Append([]string{
		versions.Server.Version,
		versions.Server.APIVersion,
		versions.Client.Version,
		versions.Client.APIVersion,
	})

	table.Render()
}
