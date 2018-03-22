// Copyright 2018 NetApp, Inc. All Rights Reserved.

package cmd

import "github.com/spf13/cobra"

func init() {
	RootCmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Add a resource to Trident",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		err := discoverOperatingMode(cmd)
		return err
	},
}
