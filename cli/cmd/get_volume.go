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

func init() {
	getCmd.AddCommand(getVolumeCmd)
}

var getVolumeCmd = &cobra.Command{
	Use:     "volume",
	Short:   "Get one or more volumes from Trident",
	Aliases: []string{"v", "volumes"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == MODE_TUNNEL {
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
		volumes = append(volumes, volume)
	}

	WriteVolumes(volumes)

	return nil
}

func GetVolumes(baseURL string) ([]string, error) {

	url := baseURL + "/volume"

	response, responseBody, err := api.InvokeRestApi("GET", url, nil, Debug)
	if err != nil {
		return nil, err
	} else if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get volumes. %v", response.Status)
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

	response, responseBody, err := api.InvokeRestApi("GET", url, nil, Debug)
	if err != nil {
		return storage.VolumeExternal{}, err
	} else if response.StatusCode != http.StatusOK {
		return storage.VolumeExternal{}, fmt.Errorf("could not get volume %s. %v", volumeName, response.Status)
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
	case FORMAT_JSON:
		WriteJSON(api.MultipleVolumeResponse{volumes})
	case FORMAT_YAML:
		WriteYAML(api.MultipleVolumeResponse{volumes})
	case FORMAT_NAME:
		writeVolumeNames(volumes)
	case FORMAT_WIDE:
		writeWideVolumeTable(volumes)
	default:
		writeVolumeTable(volumes)
	}
}

func writeVolumeTable(volumes []storage.VolumeExternal) {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Size", "Storage Class", "Protocol", "Backend", "Pool"})

	for _, volume := range volumes {

		volumeSize, _ := strconv.ParseUint(volume.Config.Size, 10, 64)

		table.Append([]string{
			volume.Config.Name,
			humanize.IBytes(volumeSize),
			volume.Config.StorageClass,
			string(volume.Config.Protocol),
			volume.Backend,
			volume.Pool,
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
		"Backend",
		"Pool",
		"Access Mode",
	}
	table.SetHeader(header)

	for _, volume := range volumes {

		volumeSize, _ := strconv.ParseUint(volume.Config.Size, 10, 64)

		table.Append([]string{
			volume.Config.Name,
			volume.Config.InternalName,
			humanize.IBytes(volumeSize),
			volume.Config.StorageClass,
			string(volume.Config.Protocol),
			volume.Backend,
			volume.Pool,
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
