// Copyright 2019 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/dustin/go-humanize"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var backendsByUUID map[string]*storage.BackendExternal

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
	var err error

	// If no volumes were specified, we'll get all of them
	getAll := false
	if len(volumeNames) == 0 {
		getAll = true
		volumeNames, err = GetVolumes()
		if err != nil {
			return err
		}
	}

	volumes := make([]storage.VolumeExternal, 0, 10)

	// Get the actual volume objects
	for _, volumeName := range volumeNames {

		volume, err := GetVolume(volumeName)
		if err != nil {
			if getAll && utils.IsNotFoundError(err) {
				continue
			}
			return err
		}

		if OutputFormat == FormatWide {
			// look up and cache the backends by UUID
			if backendsByUUID[volume.BackendUUID] == nil {
				backend, err := GetBackendByBackendUUID(volume.BackendUUID)
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

func GetVolumes() ([]string, error) {
	url := BaseURL() + "/volume"

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

func GetVolume(volumeName string) (storage.VolumeExternal, error) {
	url := BaseURL() + "/volume/" + volumeName

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil, Debug)
	if err != nil {
		return storage.VolumeExternal{}, err
	} else if response.StatusCode != http.StatusOK {
		errorMessage := fmt.Sprintf("could not get volume %s: %v", volumeName,
			GetErrorFromHTTPResponse(response, responseBody))
		switch response.StatusCode {
		case http.StatusNotFound:
			return storage.VolumeExternal{}, utils.NotFoundError(errorMessage)
		default:
			return storage.VolumeExternal{}, errors.New(errorMessage)
		}
	}

	var getVolumeResponse rest.GetVolumeResponse
	err = json.Unmarshal(responseBody, &getVolumeResponse)
	if err != nil {
		return storage.VolumeExternal{}, err
	}
	if getVolumeResponse.Volume == nil {
		return storage.VolumeExternal{}, fmt.Errorf("could not get volume %s: no volume returned", volumeName)
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
	table.SetHeader([]string{"Name", "Size", "Storage Class", "Protocol", "Backend UUID", "State", "Managed"})

	for _, volume := range volumes {

		volumeSize, _ := strconv.ParseUint(volume.Config.Size, 10, 64)

		table.Append([]string{
			volume.Config.Name,
			humanize.IBytes(volumeSize),
			volume.Config.StorageClass,
			string(volume.Config.Protocol),
			volume.BackendUUID,
			string(volume.State),
			strconv.FormatBool(!volume.Config.ImportNotManaged),
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
		"State",
		"Managed",
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
			string(volume.State),
			strconv.FormatBool(!volume.Config.ImportNotManaged),
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
