// Copyright 2022 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func init() {
	getCmd.AddCommand(getNodeCmd)
}

var getNodeCmd = &cobra.Command{
	Use:     "node [<name>...]",
	Short:   "Get one or more CSI provider nodes from Trident",
	Aliases: []string{"n", "nodes"},
	Hidden:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"get", "node"}
			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		} else {
			return nodeList(args)
		}
	},
}

func nodeList(nodeNames []string) error {
	var err error

	// If no nodes were specified, we'll get all of them
	getAll := false
	if len(nodeNames) == 0 {
		getAll = true
		nodeNames, err = GetNodes()
		if err != nil {
			return err
		}
	}

	nodes := make([]models.NodeExternal, 0, 10)

	// Get the actual node objects
	for _, nodeName := range nodeNames {

		node, err := GetNode(nodeName)
		if err != nil {
			if getAll && errors.IsNotFoundError(err) {
				continue
			}
			return err
		}
		nodes = append(nodes, *node)
	}

	WriteNodes(nodes)

	return nil
}

func GetNodes() ([]string, error) {
	url := BaseURL() + "/node"

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return nil, err
	} else if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not get nodes: %v",
			GetErrorFromHTTPResponse(response, responseBody))
	}

	var listNodesResponse rest.ListNodesResponse
	err = json.Unmarshal(responseBody, &listNodesResponse)
	if err != nil {
		return nil, err
	}

	return listNodesResponse.Nodes, nil
}

func GetNode(nodeName string) (*models.NodeExternal, error) {
	url := BaseURL() + "/node/" + nodeName

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return nil, err
	} else if response.StatusCode != http.StatusOK {
		errorMessage := fmt.Sprintf("could not get node %s: %v", nodeName,
			GetErrorFromHTTPResponse(response, responseBody))
		switch response.StatusCode {
		case http.StatusNotFound:
			return nil, errors.NotFoundError(errorMessage)
		default:
			return nil, errors.New(errorMessage)
		}
	}

	var getNodeResponse rest.GetNodeResponse
	err = json.Unmarshal(responseBody, &getNodeResponse)
	if err != nil {
		return nil, err
	}

	return getNodeResponse.Node, nil
}

func WriteNodes(nodes []models.NodeExternal) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(api.MultipleNodeResponse{Items: nodes})
	case FormatYAML:
		WriteYAML(api.MultipleNodeResponse{Items: nodes})
	case FormatName:
		writeNodeNames(nodes)
	case FormatWide:
		writeWideNodeTable(nodes)
	default:
		writeNodeTable(nodes)
	}
}

func writeNodeTable(nodes []models.NodeExternal) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name"})

	for _, n := range nodes {
		table.Append([]string{
			n.Name,
		})
	}

	table.Render()
}

func writeWideNodeTable(nodes []models.NodeExternal) {
	table := tablewriter.NewWriter(os.Stdout)

	header := []string{
		"Name",
		"IQN",
		"IPs",
		"Services",
		"State",
	}
	table.SetHeader(header)

	for _, node := range nodes {
		var services []string
		if node.HostInfo != nil {
			services = node.HostInfo.Services
		}
		table.Append([]string{
			node.Name,
			node.IQN,
			strings.Join(node.IPs, "\n"),
			strings.Join(services, "\n"),
			string(node.PublicationState),
		})
	}

	table.Render()
}

func writeNodeNames(nodes []models.NodeExternal) {
	for _, n := range nodes {
		fmt.Println(n.Name)
	}
}
