// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import "github.com/spf13/cobra"

func init() {
	RootCmd.AddCommand(importCmd)
}

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import an existing resource to Trident",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		err := discoverOperatingMode(cmd)
		return err
	},
}
