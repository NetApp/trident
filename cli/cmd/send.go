// Copyright 2020 NetApp, Inc. All Rights Reserved.

package cmd

import "github.com/spf13/cobra"

func init() {
	RootCmd.AddCommand(sendCmd)
}

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a resource from Trident",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		initCmdLogging()
		err := discoverOperatingMode(cmd)
		return err
	},
}
