// Copyright 2023 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

var (
	orchestratorReady  bool
	administratorReady bool
	provisionerReady   bool
	forceUpdate        bool
)

const updateNodeConfirmation = "Are you sure you want to update Trident node state???"

func init() {
	updateCmd.AddCommand(updateNodeCmd)
	updateNodeCmd.Flags().BoolVar(&orchestratorReady, "orchestratorReady", true, "Set ready state from the CO perspective")
	updateNodeCmd.Flags().BoolVar(&administratorReady, "administratorReady", true, "Set ready state from the administrative perspective")
	updateNodeCmd.Flags().BoolVar(&provisionerReady, "provisionerReady", true, "Set ready state from the SP perspective")
	updateNodeCmd.PersistentFlags().BoolVar(&forceUpdate, forceConfirmation, false, "Update node without confirmation.")
}

var updateNodeCmd = &cobra.Command{
	Use:     "node <name>",
	Short:   "Update a CSI provider node in Trident",
	Aliases: []string{"n"},
	Args:    cobra.ExactArgs(1),
	Hidden:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		var ready, adminReady, cleaned *bool
		var err error
		if cmd.Flags().Changed("orchestratorReady") {
			ready = utils.Ptr(orchestratorReady)
		}
		if cmd.Flags().Changed("administratorReady") {
			adminReady = utils.Ptr(administratorReady)
		}
		if cmd.Flags().Changed("provisionerReady") {
			cleaned = utils.Ptr(provisionerReady)
		}

		if OperatingMode == ModeTunnel {

			if !forceUpdate {
				if forceUpdate, err = getUserConfirmation(updateNodeConfirmation, cmd); err != nil {
					return err
				} else if !forceUpdate {
					return errors.New("update node canceled")
				}
			}
			command := []string{"update", "node", fmt.Sprintf("--%s", forceConfirmation)}

			if cmd.Flags().Changed("orchestratorReady") {
				command = append(command, "--orchestratorReady="+strconv.FormatBool(orchestratorReady))
			}
			if cmd.Flags().Changed("administratorReady") {
				command = append(command, "--administratorReady="+strconv.FormatBool(administratorReady))
			}
			if cmd.Flags().Changed("provisionerReady") {
				command = append(command, "--provisionerReady="+strconv.FormatBool(provisionerReady))
			}

			TunnelCommand(append(command, args...))
			return nil
		} else {
			return nodeUpdate(args[0], ready, adminReady, cleaned)
		}
	},
}

func nodeUpdate(nodeName string, orchestratorReady, administratorReady, provisionerReady *bool) error {
	nodeFlags := utils.NodePublicationStateFlags{
		OrchestratorReady:  orchestratorReady,
		AdministratorReady: administratorReady,
		ProvisionerReady:   provisionerReady,
	}
	requestBody, err := json.Marshal(nodeFlags)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/%s/%s/publicationState", BaseURL(), "node", nodeName)
	response, responseBody, err := api.InvokeRESTAPI("PUT", url, requestBody)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusAccepted && response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			log.Warning("Usage: node <orchestratorReady> <administratorReady> <provisionerReady>")
		}
		if response.StatusCode == http.StatusTooManyRequests {
			retryAfterSeconds := response.Header.Get("Retry-After")
			log.Warningf("Rejected due to rate limiting, try again after: %v", retryAfterSeconds)
		}
		return fmt.Errorf("could not update node %s: %v", nodeName, GetErrorFromHTTPResponse(response, responseBody))
	}

	var updateNodeResponse rest.UpdateNodeResponse
	err = json.Unmarshal(responseBody, &updateNodeResponse)
	if err != nil {
		return err
	}

	node, err := GetNode(updateNodeResponse.Name)
	if err != nil {
		return err
	} else if node == nil {
		return fmt.Errorf("node was empty")
	}

	nodes := make([]utils.NodeExternal, 0, 1)
	nodes = append(nodes, *node)
	WriteNodes(nodes)
	return nil
}
