// Copyright 2024 NetApp, Inc. All Rights Reserved.

package cmd

import "github.com/spf13/cobra"

func init() {
	RootCmd.AddCommand(checkCmd)
}

var checkCmd = &cobra.Command{
	Use:    "check",
	Short:  "check status of a Trident pod",
	Hidden: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		initCmdLogging()
		err := discoverOperatingMode(cmd)
		return err
	},
}
