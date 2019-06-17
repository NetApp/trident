// Copyright 2019 NetApp, Inc. All Rights Reserved.

package cmd

import "github.com/spf13/cobra"

var forceObliviate bool

const (
	forceConfirmation   = "yesireallymeanit"
	crdConfirmation     = "Are you sure you want to wipe out all of Trident's custom resources and CRDs???"
	storageConfirmation = "Are you sure you want to wipe out all of Trident's storage resources???"
)

func init() {
	RootCmd.AddCommand(obliviateCmd)
	obliviateCmd.PersistentFlags().BoolVar(&forceObliviate, forceConfirmation, false, "Obliviate without confirmation.")
}

var obliviateCmd = &cobra.Command{
	Use:    "obliviate",
	Short:  "Reset Trident state",
	Hidden: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		err := discoverOperatingMode(cmd)
		return err
	},
}
