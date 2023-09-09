// Copyright 2022 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/rest"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils"
)

var (
	getVolume string
	getNode   string
)

func init() {
	getCmd.AddCommand(getPublicationCmd)
	getPublicationCmd.Flags().StringVar(&getVolume, "volume", "", "Limit query to volume")
	getPublicationCmd.Flags().StringVar(&getNode, "node", "", "Limit query to node")
}

var getPublicationCmd = &cobra.Command{
	Use:     "publication",
	Short:   "Get one or more volume publications from Trident",
	Aliases: []string{"p", "pub", "vp", "vpub"},
	Args:    cobra.ExactArgs(0),
	Hidden:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if getVolume != "" && getNode != "" && cmd.Flags().Changed("dirty") {
			Log().Fatalf("--dirty flag not supported if both node and volume are specified.")
		}

		if OperatingMode == ModeTunnel {

			command := []string{"get", "publication"}
			if getVolume != "" {
				command = append(command, "--volume", getVolume)
			}
			if getNode != "" {
				command = append(command, "--node", getNode)
			}

			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		} else {
			if getVolume != "" && getNode != "" {
				return volumePublicationGet()
			} else {
				return volumePublicationsList(cmd)
			}
		}
	},
}

func GetVolumePublication(volumeName, nodeName string) (*utils.VolumePublicationExternal, error) {
	var err error

	url := BaseURL() + "/publication/" + volumeName + "/" + nodeName

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get volume publications: %v",
			GetErrorFromHTTPResponse(response, responseBody))
	}

	var getPublicationResponse rest.VolumePublicationResponse
	err = json.Unmarshal(responseBody, &getPublicationResponse)
	if err != nil {
		return nil, err
	}

	return getPublicationResponse.VolumePublication, nil
}

func volumePublicationGet() error {
	pub, err := GetVolumePublication(getVolume, getNode)
	if err != nil {
		return err
	}

	pubs := make([]utils.VolumePublicationExternal, 0, 1)
	pubs = append(pubs, *pub)

	WriteVolumePublications(pubs)

	return nil
}

func volumePublicationsList(cmd *cobra.Command) error {
	url := BaseURL() + "/publication"
	if getVolume != "" {
		url = BaseURL() + "/volume/" + getVolume + "/publication"
	}
	if getNode != "" {
		url = BaseURL() + "/node/" + getNode + "/publication"
	}

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("could not get volume publications: %v",
			GetErrorFromHTTPResponse(response, responseBody))
	}

	var listPublicationsResponse rest.VolumePublicationsResponse
	err = json.Unmarshal(responseBody, &listPublicationsResponse)
	if err != nil {
		return err
	}

	pubs := make([]utils.VolumePublicationExternal, 0, len(listPublicationsResponse.VolumePublications))
	for _, pub := range listPublicationsResponse.VolumePublications {
		pubs = append(pubs, *pub)
	}

	WriteVolumePublications(pubs)

	return nil
}

func WriteVolumePublications(pubs []utils.VolumePublicationExternal) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(api.MultipleVolumePublicationResponse{Items: pubs})
	case FormatYAML:
		WriteYAML(api.MultipleVolumePublicationResponse{Items: pubs})
	case FormatName:
		writeVolumePublicationNames(pubs)
	case FormatWide:
		writeWideVolumePublicationTable(pubs)
	default:
		writeVolumePublicationTable(pubs)
	}
}

func writeVolumePublicationTable(pubs []utils.VolumePublicationExternal) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Node", "Volume"})

	for _, pub := range pubs {
		table.Append([]string{
			pub.NodeName,
			pub.VolumeName,
		})
	}

	table.Render()
}

func writeWideVolumePublicationTable(pubs []utils.VolumePublicationExternal) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Node", "Volume", "ReadOnly", "Dirty", "AccessMode"})

	for _, pub := range pubs {
		table.Append([]string{
			pub.Name,
			pub.NodeName,
			pub.VolumeName,
			strconv.FormatBool(pub.ReadOnly),
			config.CSIAccessModes[pub.AccessMode],
		})
	}

	table.Render()
}

func writeVolumePublicationNames(pubs []utils.VolumePublicationExternal) {
	for _, pub := range pubs {
		fmt.Println(pub.Name)
	}
}
