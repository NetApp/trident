// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import "github.com/spf13/cobra"

func init() {
	RootCmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Modify a resource in Trident",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		initCmdLogging()
		err := discoverOperatingMode(cmd)
		return err
	},
}
