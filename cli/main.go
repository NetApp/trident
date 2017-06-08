package main

import (
	"os"

	"github.com/netapp/trident/cli/cmd"
)

func main() {
	cmd.ExitCode = cmd.EXIT_CODE_SUCCESS

	if err := cmd.RootCmd.Execute(); err != nil {
		cmd.SetExitCodeFromError(err)
	}

	os.Exit(cmd.ExitCode)
}
