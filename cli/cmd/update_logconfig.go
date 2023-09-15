// Copyright 2022 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/logging"
)

var (
	logLevel         string
	loggingWorkflows string
	loggingLayers    string
	configFilename   string
)

func init() {
	levelFlag, workflowFlag, layersFlag, configFlag := "log-level", "log-workflows", "log-layers", "config-file"

	updateCmd.AddCommand(updateLoggingConfig)
	updateLoggingConfig.Flags().StringVarP(&logLevel, levelFlag, "", "",
		"Log level to select: trace|debug|info|warn|error|fatal")
	updateLoggingConfig.Flags().StringVarP(&loggingWorkflows, workflowFlag, "", "",
		"Workflows to select: one or more <category>=<comma-delimited list of operations> separated by a colon")
	updateLoggingConfig.Flags().StringVarP(&loggingLayers, layersFlag, "", "",
		"Log layers to select: one or more log layers separated by commas")
	updateLoggingConfig.Flags().StringVarP(&configFilename, "config-file", "f", "",
		"A logging configuration JSON or YAML file. Call tridentctl get logconfig -o json (or yaml) for an example.")

	updateLoggingConfig.MarkFlagsMutuallyExclusive(levelFlag, configFlag)
	updateLoggingConfig.MarkFlagsMutuallyExclusive(workflowFlag, configFlag)
	updateLoggingConfig.MarkFlagsMutuallyExclusive(layersFlag, configFlag)
}

var updateLoggingConfig = &cobra.Command{
	Use:     "logconfig",
	Short:   "Update the logging configuration in Trident",
	Aliases: []string{"lc"},
	RunE: func(cmd *cobra.Command, args []string) error {
		level, workflows, layers := logLevel, loggingWorkflows, loggingLayers

		if configFilename != "" {
			values, err := getValuesFromConfig(configFilename)
			if err != nil {
				return err
			}
			level, workflows, layers = values["level"], values["workflows"], values["layers"]
		}

		if OperatingMode == ModeTunnel {
			command := []string{
				"update", "logconfig",
				"--log-level", level,
				"--log-workflows", workflows,
				"--log-layers", layers,
			}
			out, err := TunnelCommand(append(command, args...))
			printOutput(cmd, out, err)
			return err
		} else {
			return logConfigUpdate(level, workflows, layers)
		}
	},
}

func logConfigUpdate(level, workflows, layers string) error {
	url := BaseURL() + "/logging/level/" + level

	if level == "" {
		return fmt.Errorf("log level was empty, it is required")
	}

	response, responseBody, err := api.InvokeRESTAPI("POST", url, []byte{})
	if err != nil {
		return err
	} else if response.StatusCode != http.StatusOK {
		return fmt.Errorf("could not update log level to %s: %v", level,
			GetErrorFromHTTPResponse(response, responseBody))
	}

	url = BaseURL() + "/logging/workflows"

	req := rest.SetLoggingWorkflowsRequest{LoggingWorkflows: workflows}
	jsonReq, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("could not marshal logging workflows value to create a request payload: %v", err)
	}

	response, responseBody, err = api.InvokeRESTAPI("POST", url, jsonReq)
	if err != nil {
		return err
	} else if response.StatusCode != http.StatusOK {
		return fmt.Errorf("could not update logging workflows to %s: %v", workflows,
			GetErrorFromHTTPResponse(response, responseBody))
	}

	url = BaseURL() + "/logging/layers"

	layerReq := rest.SetLoggingLayersRequest{LogLayers: layers}
	jsonReq, err = json.Marshal(layerReq)
	if err != nil {
		return fmt.Errorf("could not marshal logging layers value to create a request payload: %v", err)
	}

	response, responseBody, err = api.InvokeRESTAPI("POST", url, jsonReq)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("could not update logging layers to %s: %v", workflows,
			GetErrorFromHTTPResponse(response, responseBody))
	}

	return nil
}

func getValuesFromConfig(configFile string) (map[string]string, error) {
	config := &logConfigResp{}
	var err error

	if configFile == "-" {
		config, err = unmarshalStdinLogConfig()
		if err != nil {
			return nil, err
		}
	} else {
		config, err = tryAsYAML(configFile)
		if err != nil {
			return nil, err
		}
	}

	level := config.LogLevel
	// Sorting is necessary here because they're sorted in the logging package. Removing these sorts will break the
	// ability to use multiple workflows.
	sort.Strings(config.Workflows)
	workflows := strings.Join(config.Workflows, logging.WorkflowFlagSeparator)
	layers := strings.Join(config.LogLayers, logging.LogLayerSeparator)

	return map[string]string{
		"level":     level,
		"workflows": workflows,
		"layers":    layers,
	}, nil
}

func unmarshalStdinLogConfig() (*logConfigResp, error) {
	config := &logConfigResp{}
	rawData, err := io.ReadAll(os.Stdin)
	if err != nil {
		return config, err
	}

	err = yaml.Unmarshal(rawData, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func tryAsYAML(configFile string) (*logConfigResp, error) {
	config := &logConfigResp{}
	contents, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(contents, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}
