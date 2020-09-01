/*
 *  Copyright (c) 2020 NetApp
 *  All rights reserved
 */

package cmd

import (
	"time"

	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(pauseCmd)
}

var pauseCmd = &cobra.Command{
	Use:    "pause",
	Short:  "Sleep forever",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		for {
			time.Sleep(time.Second)
		}
	},
}
