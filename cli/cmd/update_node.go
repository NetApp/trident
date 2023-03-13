// Copyright 2023 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/utils"
)

var (
	setNodeReady      bool
	setNodeAdminReady bool
	setNodeCleaned    bool
	forceUpdate       bool
)

const updateNodeConfirmation = "Are you sure you want to update Trident node state???"

func init() {
	updateCmd.AddCommand(updateNodeCmd)
	updateNodeCmd.Flags().BoolVar(&setNodeReady, "ready", true, "Set ready state from the cluster perspective")
	updateNodeCmd.Flags().BoolVar(&setNodeAdminReady, "adminReady", true, "Set service state from the administrative perspective")
	updateNodeCmd.Flags().BoolVar(&setNodeCleaned, "cleaned", true, "Set node as cleaned")
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
		if cmd.Flags().Changed("ready") {
			ready = utils.Ptr(setNodeReady)
		}
		if cmd.Flags().Changed("adminReady") {
			adminReady = utils.Ptr(setNodeAdminReady)
		}
		if cmd.Flags().Changed("cleaned") {
			cleaned = utils.Ptr(setNodeCleaned)
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

			if cmd.Flags().Changed("ready") {
				command = append(command, "--ready="+strconv.FormatBool(setNodeReady))
			}
			if cmd.Flags().Changed("adminReady") {
				command = append(command, "--adminReady="+strconv.FormatBool(setNodeAdminReady))
			}
			if cmd.Flags().Changed("cleaned") {
				command = append(command, "--cleaned="+strconv.FormatBool(setNodeCleaned))
			}

			TunnelCommand(append(command, args...))
			return nil
		} else {
			return nodeUpdate(args[0], ready, adminReady, cleaned)
		}
	},
}

func nodeUpdate(nodeName string, ready, adminReady, cleaned *bool) error {
	nodeFlags := utils.NodePublicationStateFlags{
		Ready:      ready,
		AdminReady: adminReady,
		Cleaned:    cleaned,
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
			log.Warning("Usage: node <read> <adminReady> <clean>")
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
