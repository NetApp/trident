// Copyright 2018 NetApp, Inc. All Rights Reserved.

package main

import (
	"os"

	"github.com/netapp/trident/v21/cli/cmd"
)

func main() {
	cmd.ExitCode = cmd.ExitCodeSuccess

	if err := cmd.RootCmd.Execute(); err != nil {
		cmd.SetExitCodeFromError(err)
	}

	os.Exit(cmd.ExitCode)
}
