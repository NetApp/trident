// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/ghodss/yaml"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Get one or more resources from Trident",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		initCmdLogging()
		err := discoverOperatingMode(cmd)
		return err
	},
}

func WriteJSON(out interface{}) {
	jsonBytes, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(jsonBytes))
}

func WriteYAML(out interface{}) {
	jsonBytes, _ := json.Marshal(out)
	yamlBytes, _ := yaml.JSONToYAML(jsonBytes)
	fmt.Println(string(yamlBytes))
}
