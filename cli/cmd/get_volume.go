// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/dustin/go-humanize"
	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var (
	backendsByUUID map[string]*storage.BackendExternal
)

func init() {
	getCmd.AddCommand(getVolumeCmd)
	backendsByUUID = make(map[string]*storage.BackendExternal)
}

var getVolumeCmd = &cobra.Command{
	Use:     "volume [<name>...]",
	Short:   "Get one or more volumes from Trident",
	Aliases: []string{"v", "volumes"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"get", "volume"}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return volumeList(args)
		}
	},
}

func volumeList(volumeNames []string) error {

	baseURL, err := GetBaseURL()
	if err != nil {
		return err
	}

	// If no volumes were specified, we'll get all of them
	if len(volumeNames) == 0 {
		volumeNames, err = GetVolumes(baseURL)
		if err != nil {
			return err
		}
	}

	volumes := make([]storage.VolumeExternal, 0, 10)

	// Get the actual volume objects
	for _, volumeName := range volumeNames {

		volume, err := GetVolume(baseURL, volumeName)
		if err != nil {
			return err
		}

		if OutputFormat == FormatWide {
			// look up and cache the backends by UUID
			if backendsByUUID[volume.BackendUUID] == nil {
				backend, err := GetBackendByBackendUUID(baseURL, volume.BackendUUID)
				if err != nil {
					return err
				}
				backendsByUUID[volume.BackendUUID] = &backend
			}
		}

		volumes = append(volumes, volume)
	}

	WriteVolumes(volumes)

	return nil
}

func GetVolumes(baseURL string) ([]string, error) {

	url := baseURL + "/volume"

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil, Debug)
	if err != nil {
		return nil, err
	} else if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get volumes: %v",
			GetErrorFromHTTPResponse(response, responseBody))
	}

	var listVolumesResponse rest.ListVolumesResponse
	err = json.Unmarshal(responseBody, &listVolumesResponse)
	if err != nil {
		return nil, err
	}

	return listVolumesResponse.Volumes, nil
}

func GetVolume(baseURL, volumeName string) (storage.VolumeExternal, error) {

	url := baseURL + "/volume/" + volumeName

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil, Debug)
	if err != nil {
		return storage.VolumeExternal{}, err
	} else if response.StatusCode != http.StatusOK {
		return storage.VolumeExternal{}, fmt.Errorf("could not get volume %s: %v", volumeName,
			GetErrorFromHTTPResponse(response, responseBody))
	}

	var getVolumeResponse rest.GetVolumeResponse
	err = json.Unmarshal(responseBody, &getVolumeResponse)
	if err != nil {
		return storage.VolumeExternal{}, err
	}

	return *getVolumeResponse.Volume, nil
}

func WriteVolumes(volumes []storage.VolumeExternal) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(api.MultipleVolumeResponse{Items: volumes})
	case FormatYAML:
		WriteYAML(api.MultipleVolumeResponse{Items: volumes})
	case FormatName:
		writeVolumeNames(volumes)
	case FormatWide:
		writeWideVolumeTable(volumes)
	default:
		writeVolumeTable(volumes)
	}
}

func writeVolumeTable(volumes []storage.VolumeExternal) {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Size", "Storage Class", "Protocol", "Backend UUID", "Pool", "State"})

	for _, volume := range volumes {

		volumeSize, _ := strconv.ParseUint(volume.Config.Size, 10, 64)

		table.Append([]string{
			volume.Config.Name,
			humanize.IBytes(volumeSize),
			volume.Config.StorageClass,
			string(volume.Config.Protocol),
			volume.BackendUUID,
			volume.Pool,
			string(volume.State),
		})
	}

	table.Render()
}

func writeWideVolumeTable(volumes []storage.VolumeExternal) {

	table := tablewriter.NewWriter(os.Stdout)
	header := []string{
		"Name",
		"Internal Name",
		"Size",
		"Storage Class",
		"Protocol",
		"Backend UUID",
		"Backend",
		"Pool",
		"State",
		"Access Mode",
	}
	table.SetHeader(header)

	for _, volume := range volumes {

		volumeSize, _ := strconv.ParseUint(volume.Config.Size, 10, 64)

		backendName := "unknown"
		if backend := backendsByUUID[volume.BackendUUID]; backend != nil {
			backendName = backend.Name
		}

		table.Append([]string{
			volume.Config.Name,
			volume.Config.InternalName,
			humanize.IBytes(volumeSize),
			volume.Config.StorageClass,
			string(volume.Config.Protocol),
			volume.BackendUUID,
			backendName,
			volume.Pool,
			string(volume.State),
			string(volume.Config.AccessMode),
		})
	}

	table.Render()
}

func writeVolumeNames(volumes []storage.VolumeExternal) {

	for _, sc := range volumes {
		fmt.Println(sc.Config.Name)
	}
}
