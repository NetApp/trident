package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	tridentctlPrefix = "tridentctl-"
	execMask         = 0o111
	pluginPathEnvVar = "TRIDENTCTL_PLUGIN_PATH"
)

var (
	pluginsFound       = make(map[string]struct{})
	invalidPluginNames = map[string]struct{}{
		"completion": {},
		"create":     {},
		"delete":     {},
		"get":        {},
		"help":       {},
		"images":     {},
		"import":     {},
		"install":    {},
		"logs":       {},
		"send":       {},
		"uninstall":  {},
		"update":     {},
		"version":    {},
	}
)

func init() {
	// search PATH for all binaries starting with "tridentctl-"
	binaries := findBinaries(tridentctlPrefix)

	plugins := make([]*cobra.Command, 0, len(binaries))
	for _, binary := range binaries {
		pluginName := strings.TrimPrefix(filepath.Base(binary), tridentctlPrefix)
		if _, ok := invalidPluginNames[pluginName]; ok {
			continue
		}
		if _, ok := pluginsFound[pluginName]; ok {
			// Skip duplicates
			continue
		}

		pluginCmd := &cobra.Command{
			Use:                pluginName,
			Short:              "Run the " + pluginName + " plugin",
			GroupID:            "plugins",
			DisableFlagParsing: true,
			Run: func(cmd *cobra.Command, args []string) {
				pluginCommand := binary

				os.Setenv("TRIDENTCTL_PLUGIN_INVOCATION", "true")

				execCmd := exec.Command(pluginCommand, args...)
				execCmd.Stdin = os.Stdin
				execCmd.Stdout = os.Stdout
				execCmd.Stderr = os.Stderr

				err := execCmd.Run()
				if err != nil {
					fmt.Printf("Error executing plugin command: %v\n", err)
				}
			},
		}
		plugins = append(plugins, pluginCmd)
		pluginsFound[pluginName] = struct{}{}
	}

	if len(plugins) == 0 {
		return
	}

	pluginGroup := cobra.Group{
		ID:    "plugins",
		Title: "Plugins",
	}

	RootCmd.AddGroup(&pluginGroup)
	for _, pluginCmd := range plugins {
		RootCmd.AddCommand(pluginCmd)
	}
}

func findBinaries(prefix string) []string {
	var binaries []string
	path := os.Getenv(pluginPathEnvVar)
	if path == "" {
		path = os.Getenv("PATH")
	}

	for _, dir := range strings.Split(path, ":") {
		files, err := filepath.Glob(filepath.Join(dir, prefix+"*"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		for _, file := range files {
			if isExecutable(file) {
				binaries = append(binaries, file)
			}
		}
	}
	return binaries
}

func isExecutable(file string) bool {
	info, err := os.Stat(file)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Mode()&execMask != 0
}
