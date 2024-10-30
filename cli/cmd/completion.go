package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

const (
	cobraOutputSuffix = `Completion ended with directive: ShellCompDirectiveNoFileComp`
)

// CLI flags
var skipPluginDetection bool

// The cobra library comes by default with a completion command that generates completion scripts for bash, zsh, fish, and powershell.
// But these "standard" completion scripts are not aware of the plugins that are available in the system.
// This command here overwrites the default cobra completion command and generates bash and zsh completion scripts that are aware of the plugins
//  and redirect the completion requests to the appropriate plugin binaries.

var completionCmd = &cobra.Command{
	Use:   "completion [command] ",
	Short: "Generate completion script ",
	Long: `Generate the autocompletion script for tridentctl for the specified shell.
See each sub-command's help for details on how to use the generated script.`,
}

var completionBashCmd = &cobra.Command{
	Use:   "bash",
	Short: "Generate the autocompletion script for bash",
	RunE: func(cmd *cobra.Command, args []string) error {
		cobraPoweredPlugins := GetCobraPoweredPlugins()

		if skipPluginDetection || len(cobraPoweredPlugins) == 0 {
			return cmd.Root().GenBashCompletionV2(os.Stdout, true)
		}

		var buf bytes.Buffer
		err := cmd.Root().GenBashCompletionV2(&buf, true)
		if err != nil {
			return err
		}

		patchedCompletionScript := GenPatchedBashCompletionScript(cmd, cobraPoweredPlugins, buf.String())
		fmt.Print(patchedCompletionScript)
		return nil
	},
}

var completionZshCmd = &cobra.Command{
	Use:   "zsh",
	Short: "Generate the autocompletion script for zsh ",
	RunE: func(cmd *cobra.Command, args []string) error {
		cobraPoweredPlugins := GetCobraPoweredPlugins()

		if skipPluginDetection || len(cobraPoweredPlugins) == 0 {
			return cmd.Root().GenZshCompletion(os.Stdout)
		}

		var buf bytes.Buffer
		err := cmd.Root().GenZshCompletion(&buf)
		if err != nil {
			return err
		}

		patchedCompletionScript := GenPatchedZshCompletionScript(cmd, cobraPoweredPlugins, buf.String())
		fmt.Print(patchedCompletionScript)
		return nil
	},
}

var completionFishCmd = &cobra.Command{
	Use:   "fish",
	Short: "Generate the autocompletion script for fish",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Root().GenFishCompletion(os.Stdout, true)
	},
}

var completionPowershellCmd = &cobra.Command{
	Use:   "powershell",
	Short: "Generate the autocompletion script for powershell",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
	},
}

func GenPatchedBashCompletionScript(cmd *cobra.Command, cobraPoweredPlugins []string, originalScript string) string {
	searchString := `requestComp="${words[0]} __complete ${args[*]}"`
	ifReplaceString := `if [[ ${args[0]} == "%s" ]] ; then
        requestComp="tridentctl-%s __complete ${args[*]:1}"`
	elseReplaceString := `else
        requestComp="${words[0]} __complete ${args[*]}"
    fi`

	return patchFunction(originalScript, searchString, ifReplaceString, elseReplaceString, cobraPoweredPlugins)
}

func GenPatchedZshCompletionScript(cmd *cobra.Command, cobraPoweredPlugins []string, originalScript string) string {
	searchString := `requestComp="${words[1]} __complete ${words[2,-1]}"`
	ifReplaceString := `if [[ ${#words[@]} -ge 3 && "${words[2]}" == "%s" ]] ; then
        requestComp="tridentctl-%s __complete ${words[3,-1]}"`

	elseReplaceString := `else
        requestComp="${words[1]} __complete ${words[2,-1]}"
    fi`

	return patchFunction(originalScript, searchString, ifReplaceString, elseReplaceString, cobraPoweredPlugins)
}

// patchFunction creates if-elif-...-else block in the completion script
// seems to be generic enough to work for both zsh and bash
func patchFunction(originalScript, searchString, ifReplaceString, elseReplaceString string, cobraPoweredPlugins []string) string {
	fullReplaceString := "# Patch for cobra-powered plugins"

	i := 0
	for _, plugin := range cobraPoweredPlugins {
		prefix := "    "
		if i > 0 {
			// make it an elif
			prefix += "el"
		}
		fullReplaceString += "\n" + prefix + fmt.Sprintf(ifReplaceString, plugin, plugin)
		i++
	}
	fullReplaceString += "\n    " + elseReplaceString

	return strings.Replace(originalScript, searchString, fullReplaceString, 1)
}

// GetCobraPoweredPlugins collect cobra-powered plugins
// to check if the plugin is a cobra plugin we run the plugin with argument "__complete -- @@@@"
// and compare the output with the cobraOutputSuffix string
func GetCobraPoweredPlugins() []string {
	if skipPluginDetection {
		return nil
	}

	cobraPoweredPlugins := make([]string, 0)
	for plugin := range pluginsFound {
		// run command and compare output
		pluginCommand := tridentctlPrefix + plugin

		cmd := exec.Command(pluginCommand, "__complete", "--", "@@@@") // @@@@ is random string unlikely to be a valid prefix

		// Run command and capture output
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return nil
		}

		// Compare output with string
		if strings.HasSuffix(strings.TrimSpace(string(output)), cobraOutputSuffix) {
			cobraPoweredPlugins = append(cobraPoweredPlugins, plugin)
		} else {
			fmt.Fprintf(os.Stderr, "Plugin %s does not seem to be a cobra plugin\n", plugin)
		}
	}
	return cobraPoweredPlugins
}

func init() {
	completionCmd.AddCommand(completionBashCmd)
	completionCmd.AddCommand(completionZshCmd)
	completionCmd.AddCommand(completionFishCmd)
	completionCmd.AddCommand(completionPowershellCmd)

	RootCmd.AddCommand(completionCmd)
	completionCmd.PersistentFlags().BoolVar(&skipPluginDetection, "skip-plugin-detection", false,
		"Create completion script without checking for cobra-powered plugins")
}
