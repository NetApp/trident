package cmd

import "github.com/spf13/cobra"

func init() {
	RootCmd.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove one or more resources from Trident",
}
