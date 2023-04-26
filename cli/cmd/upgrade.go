// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import "github.com/spf13/cobra"

func init() {
	RootCmd.AddCommand(upgradeCmd)
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade a resource in Trident",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		initCmdLogging()
		err := discoverOperatingMode(cmd)
		return err
	},
}
