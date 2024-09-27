// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/rest"
	versionutils "github.com/netapp/trident/utils/version"
)

var clientOnly bool

func init() {
	RootCmd.AddCommand(versionCmd)
	versionCmd.Flags().BoolVar(&clientOnly, "client", false, "Client version only (no server required).")
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of Trident",
	Long:  "Print the version of the Trident storage orchestrator for Kubernetes",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		initCmdLogging()
		if !clientOnly {
			err = discoverOperatingMode(cmd)
		}
		return err
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if clientOnly {
			writeVersion(getClientVersion())
		} else {

			var serverVersion rest.GetVersionResponse
			var err error

			// Get the server version
			if OperatingMode == ModeTunnel {
				serverVersion, err = getVersionFromTunnel()
			} else {
				serverVersion, err = getVersionFromRest()
			}

			if err != nil {
				return err
			}

			parsedServerVersion, err := versionutils.ParseDate(serverVersion.Version)
			if err != nil {
				return err
			}

			// Add the ACP version
			var parsedACPServerVersion *versionutils.Version
			if serverVersion.ACPVersion != "" {
				parsedACPServerVersion, err = versionutils.ParseDate(serverVersion.ACPVersion)
				if err != nil {
					return err
				}
			}

			// Add the client version, which is always hardcoded at compile time
			versions := addClientVersion(parsedServerVersion, parsedACPServerVersion)

			// Add the server's Go version
			versions.Server.GoVersion = serverVersion.GoVersion

			// TODO: Add the ACP server's Go version
			// versions.ACPServer.GoVersion = serverVersion.GoVersion

			writeVersions(versions)
		}

		return nil
	},
}

// getVersion retrieves the Trident server version directly using the REST API
func getVersionFromRest() (rest.GetVersionResponse, error) {
	url := BaseURL() + "/version"

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return rest.GetVersionResponse{}, err
	} else if response.StatusCode != http.StatusOK {
		return rest.GetVersionResponse{}, fmt.Errorf("could not get version: %v",
			GetErrorFromHTTPResponse(response, responseBody))
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
	versionJSON, stderrOut, err := TunnelCommandRaw(command)
	if err != nil {
		if len(versionJSON) > 0 {
			err = fmt.Errorf("%v; %s", err, string(versionJSON))
		}
		return rest.GetVersionResponse{}, err
	}

	os.Stderr.Write(stderrOut)

	if Debug {
		fmt.Printf("Version JSON: %s\n", versionJSON)
	}

	var tunnelVersionResponse api.VersionResponse
	err = json.Unmarshal(versionJSON, &tunnelVersionResponse)
	if err != nil {
		return rest.GetVersionResponse{}, err
	}

	version := rest.GetVersionResponse{
		Version:   tunnelVersionResponse.Server.Version,
		GoVersion: tunnelVersionResponse.Server.GoVersion,
	}

	if tunnelVersionResponse.ACPServer != nil {
		version.ACPVersion = tunnelVersionResponse.ACPServer.Version
	}

	return version, nil
}

func getClientVersion() *api.ClientVersionResponse {
	return &api.ClientVersionResponse{
		Client: api.Version{
			Version:       config.OrchestratorVersion.String(),
			MajorVersion:  config.OrchestratorVersion.MajorVersion(),
			MinorVersion:  config.OrchestratorVersion.MinorVersion(),
			PatchVersion:  config.OrchestratorVersion.PatchVersion(),
			PreRelease:    config.OrchestratorVersion.PreRelease(),
			BuildMetadata: config.OrchestratorVersion.BuildMetadata(),
			APIVersion:    config.OrchestratorAPIVersion,
			GoVersion:     runtime.Version(),
		},
	}
}

// addClientVersion accepts the server version and fills in the client version
func addClientVersion(serverVersion, acpServerVersion *versionutils.Version) *api.VersionResponse {
	versions := api.VersionResponse{
		Server: &api.Version{
			Version:       serverVersion.String(),
			MajorVersion:  serverVersion.MajorVersion(),
			MinorVersion:  serverVersion.MinorVersion(),
			PatchVersion:  serverVersion.PatchVersion(),
			PreRelease:    serverVersion.PreRelease(),
			BuildMetadata: serverVersion.BuildMetadata(),
			APIVersion:    config.OrchestratorAPIVersion,
		},
		Client: &getClientVersion().Client,
	}

	ACPServer := &api.Version{}
	if acpServerVersion != nil {
		ACPServer.Version = acpServerVersion.String()
		ACPServer.MajorVersion = acpServerVersion.MajorVersion()
		ACPServer.MinorVersion = acpServerVersion.MinorVersion()
		ACPServer.PatchVersion = acpServerVersion.PatchVersion()
		ACPServer.PreRelease = acpServerVersion.PreRelease()
		ACPServer.BuildMetadata = acpServerVersion.BuildMetadata()
		ACPServer.APIVersion = config.OrchestratorAPIVersion

		versions.ACPServer = ACPServer
	}

	return &versions
}

func writeVersion(version *api.ClientVersionResponse) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(version)
	case FormatYAML:
		WriteYAML(version)
	case FormatWide:
		writeWideVersionTable(version)
	default:
		writeVersionTable(version)
	}
}

func writeVersions(versions *api.VersionResponse) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(versions)
	case FormatYAML:
		WriteYAML(versions)
	case FormatWide:
		writeWideVersionsTable(versions)
	default:
		writeVersionsTable(versions)
	}
}

func writeVersionTable(version *api.ClientVersionResponse) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Client Version"})

	table.Append([]string{
		version.Client.Version,
	})

	table.Render()
}

func writeVersionsTable(versions *api.VersionResponse) {
	table := tablewriter.NewWriter(os.Stdout)

	table.SetHeader([]string{"Server Version", "Client Version"})

	table.Append([]string{
		versions.Server.Version,
		versions.Client.Version,
	})

	table.Render()
}

func writeWideVersionTable(version *api.ClientVersionResponse) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Client Version", "Client API Version", "Client Go Version"})

	table.Append([]string{
		version.Client.Version,
		version.Client.APIVersion,
		version.Client.GoVersion,
	})

	table.Render()
}

func writeWideVersionsTable(versions *api.VersionResponse) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{
		"Server Version", "Server API Version", "Server Go Version", "Client Version",
		"Client API Version", "Client Go Version",
	})

	table.Append([]string{
		versions.Server.Version,
		versions.Server.APIVersion,
		versions.Server.GoVersion,
		versions.Client.Version,
		versions.Client.APIVersion,
		versions.Client.GoVersion,
	})

	table.Render()
}
