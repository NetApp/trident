// Copyright 2020 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/utils/errors"
)

var allNodes bool

func init() {
	deleteCmd.AddCommand(deleteNodeCmd)
	deleteNodeCmd.Flags().BoolVarP(&allNodes, "all", "", false, "Delete all nodes")
}

var deleteNodeCmd = &cobra.Command{
	Use:     "node <name> [<name>...]",
	Short:   "Delete one or more csi nodes from Trident",
	Aliases: []string{"n", "nodes"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"delete", "node"}
			if allVolumes {
				command = append(command, "--all")
			}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return nodeDelete(args)
		}
	},
}

func nodeDelete(nodeNames []string) error {
	var err error

	if allNodes {
		// Make sure --all isn't being used along with specific nodes
		if len(nodeNames) > 0 {
			return errors.New("cannot use --all switch and specify individual nodes")
		}

		// Get list of node names so we can delete them all
		nodeNames, err = GetNodes()
		if err != nil {
			return err
		}
	} else {
		// Not using --all, so make sure one or more nodes were specified
		if len(nodeNames) == 0 {
			return errors.New("node name not specified")
		}
	}

	for _, nodeName := range nodeNames {
		url := BaseURL() + "/node/" + nodeName

		response, responseBody, err := api.InvokeRESTAPI("DELETE", url, nil)
		if err != nil {
			return err
		} else if response.StatusCode != http.StatusOK {
			return fmt.Errorf("could not delete volume %s: %v", nodeName,
				GetErrorFromHTTPResponse(response, responseBody))
		}
	}

	return nil
}
