// Copyright 2022 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/utils"
)

var (
	setDirty bool
	setClean bool
)

func init() {
	updateCmd.AddCommand(updatePublicationCmd)
	updatePublicationCmd.Flags().BoolVar(&setDirty, "dirty", false, "Set publication dirty")
	updatePublicationCmd.Flags().BoolVar(&setClean, "clean", false, "Set publication clean")
	updatePublicationCmd.MarkFlagsMutuallyExclusive("dirty", "clean")
}

var updatePublicationCmd = &cobra.Command{
	Use:     "publication <volume> <node>",
	Short:   "Update a volume publication",
	Aliases: []string{"p", "pub", "vp", "vpub"},
	Args:    cobra.ExactArgs(2),
	Hidden:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var notSafeToAttach *bool
		if cmd.Flags().Changed("dirty") {
			notSafeToAttach = utils.Ptr(setDirty)
		} else if cmd.Flags().Changed("clean") {
			notSafeToAttach = utils.Ptr(!setClean)
		}

		if OperatingMode == ModeTunnel {
			command := []string{
				"update", "publication",
			}
			command = append(command, args...)

			if cmd.Flags().Changed("dirty") {
				command = append(command, "--dirty="+strconv.FormatBool(setDirty))
			} else if cmd.Flags().Changed("clean") {
				command = append(command, "--clean="+strconv.FormatBool(setClean))
			}

			TunnelCommand(append(command))
			return nil
		} else {
			return publicationUpdate(args[0], args[1], notSafeToAttach)
		}
	},
}

func publicationUpdate(volumeName, nodeName string, notSafeToAttach *bool) error {
	url := BaseURL() + "/publication/" + volumeName + "/" + nodeName

	request := utils.VolumePublicationExternal{
		Name:            utils.GenerateVolumePublishName(volumeName, nodeName),
		VolumeName:      volumeName,
		NodeName:        nodeName,
		NotSafeToAttach: notSafeToAttach,
	}
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	response, responseBody, err := api.InvokeRESTAPI("PUT", url, requestBytes, Debug)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusAccepted && response.StatusCode != http.StatusOK {

		if response.StatusCode == http.StatusNotFound {
			log.Warning("Usage: publication <volume> <node>")
		}

		return fmt.Errorf("could not update volume publication %s: %v",
			utils.GenerateVolumePublishName(volumeName, nodeName),
			GetErrorFromHTTPResponse(response, responseBody))
	}

	return nil
}
