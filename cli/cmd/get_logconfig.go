// Copyright 2022 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/common"
	"github.com/netapp/trident/frontend/rest"
)

var (
	getAllFlows  bool
	getAllLayers bool
)

func init() {
	getCmd.AddCommand(getLogConfigCmd)
	getLogConfigCmd.Flags().BoolVarP(&getAllFlows, "get-available-workflows", "", false,
		"If set, will retrieve all possible logging workflow types")
	getLogConfigCmd.Flags().BoolVarP(&getAllLayers, "get-available-log-layers", "", false,
		"If set, will retrieve all possible log layers")
}

var getLogConfigCmd = &cobra.Command{
	Use:     "logconfig",
	Short:   "Get the current logging configuration for Trident",
	Aliases: []string{"lc"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if OperatingMode == ModeTunnel {
			command := []string{"get", "logconfig"}
			if getAllFlows {
				command = append(command, "--get-available-workflows")
			}
			if getAllLayers {
				command = append(command, "--get-available-log-layers")
			}
			TunnelCommand(append(command, args...))
			return nil
		} else {
			return getLogConfig()
		}
	},
}

func getLogConfig() error {
	resp := &logConfigResp{}

	level, err := getLogLevel()
	if err != nil {
		return err
	}
	resp.LogLevel = level

	flows, err := getWorkflows()
	if err != nil {
		return err
	}
	resp.Workflows = flows

	layers, err := getLogLayers()
	if err != nil {
		return err
	}
	resp.LogLayers = layers

	writeLogConfig(resp)

	return nil
}

type logConfigResp struct {
	LogLevel  string   `json:"logLevel"`
	Workflows []string `json:"workflows"`
	LogLayers []string `json:"logLayers"`
}

func getLogLevel() (string, error) {
	url := BaseURL() + "/logging/level"

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return "", err
	} else if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("could not get configured log level: %v",
			GetErrorFromHTTPResponse(response, responseBody))
	}

	result := &common.GetLogLevelResponse{}
	if err = json.Unmarshal(responseBody, result); err != nil {
		return "", err
	}

	return result.LogLevel, nil
}

func getWorkflows() ([]string, error) {
	url := BaseURL() + "/logging/workflows"
	if !getAllFlows {
		url += "/selected"
	}

	result := []string{}

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return result, err
	} else if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("could not get configured logging workflows: %v",
			GetErrorFromHTTPResponse(response, responseBody))
	}

	if getAllFlows {
		resp := &rest.ListLoggingWorkflowsResponse{}
		if err = json.Unmarshal(responseBody, resp); err != nil {
			return result, err
		}
		result = resp.AvailableLoggingWorkflows
	} else {
		resp := &common.GetLoggingWorkflowsResponse{}
		if err = json.Unmarshal(responseBody, resp); err != nil {
			return result, err
		}
		result = []string{resp.LogWorkflows}
	}

	return result, nil
}

func getLogLayers() ([]string, error) {
	url := BaseURL() + "/logging/layers"
	if !getAllLayers {
		url += "/selected"
	}

	result := []string{}

	response, responseBody, err := api.InvokeRESTAPI("GET", url, nil)
	if err != nil {
		return result, err
	} else if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("could not get configured logging layers: %v",
			GetErrorFromHTTPResponse(response, responseBody))
	}

	if getAllLayers {
		resp := &rest.ListLoggingLayersResponse{}
		if err = json.Unmarshal(responseBody, resp); err != nil {
			return result, err
		}
		result = resp.AvailableLogLayers
	} else {
		resp := &common.GetLoggingLayersResponse{}
		if err = json.Unmarshal(responseBody, resp); err != nil {
			return result, err
		}
		result = []string{resp.LogLayers}
	}

	return result, nil
}

func writeLogConfig(cfg *logConfigResp) {
	switch OutputFormat {
	case FormatJSON:
		WriteJSON(cfg)
	case FormatYAML:
		WriteYAML(cfg)
	default:
		writeLogConfigHelper(cfg)
	}
}

func writeLogConfigHelper(cfg *logConfigResp) {
	configured := "Configured"
	available := "Available"
	hint := " (value as entered as a command line argument)"
	layersHint := hint

	start := configured
	layersStart := configured
	if getAllFlows {
		start = available
		hint = ""
	}
	if getAllLayers {
		layersStart = available
		layersHint = ""
	}
	fmt.Printf("%s log level: %s\n", configured, cfg.LogLevel)
	fmt.Printf("%s logging workflows%s: %s\n", start, hint, cfg.Workflows)
	fmt.Printf("%s log layers%s: %s\n", layersStart, layersHint, cfg.LogLayers)
}
