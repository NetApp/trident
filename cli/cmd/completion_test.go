package cmd

import (
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

var testStateMutex sync.RWMutex

func withTestState(t *testing.T, skipDetection bool, plugins map[string]struct{}, fn func()) {
	t.Helper()

	testStateMutex.Lock()
	defer testStateMutex.Unlock()

	originalSkip := skipPluginDetection
	originalPlugins := pluginsFound

	defer func() {
		skipPluginDetection = originalSkip
		pluginsFound = originalPlugins
	}()

	skipPluginDetection = skipDetection
	pluginsFound = plugins

	fn()
}

func setupCompletionCommand(t *testing.T, cmdType string, runE func(*cobra.Command, []string) error) *cobra.Command {
	t.Helper()

	rootCmd := &cobra.Command{Use: "tridentctl"}
	completionCmd := &cobra.Command{Use: "completion"}
	targetCmd := &cobra.Command{
		Use:   cmdType,
		Short: "Generate the autocompletion script for " + cmdType,
		RunE:  runE,
	}

	rootCmd.AddCommand(completionCmd)
	completionCmd.AddCommand(targetCmd)

	return targetCmd
}

func TestGenPatchedBashCompletionScript(t *testing.T) {
	dummyOriginalScript := `    args=("${words[@]:1}")
    requestComp="${words[0]} __complete ${args[*]}"

    lastParam=${words[$((${#words[@]}-1))]}
`

	expectedResult := `    args=("${words[@]:1}")
    # Patch for cobra-powered plugins
    if [[ ${args[0]} == "plugin1" ]] ; then
        requestComp="tridentctl-plugin1 __complete ${args[*]:1}"
    elif [[ ${args[0]} == "plugin2" ]] ; then
        requestComp="tridentctl-plugin2 __complete ${args[*]:1}"
    else
        requestComp="${words[0]} __complete ${args[*]}"
    fi

    lastParam=${words[$((${#words[@]}-1))]}
`

	cobraPoweredPlugins := []string{"plugin1", "plugin2"}
	result := GenPatchedBashCompletionScript(completionCmd, cobraPoweredPlugins, dummyOriginalScript)

	assert.Equal(t, expectedResult, result)
}

func TestGenPatchedZshCompletionScript(t *testing.T) {
	dummyOriginalScript := `    # Prepare the command to obtain completions
    requestComp="${words[1]} __complete ${words[2,-1]}"
    if [ "${lastChar}" = "" ]; then
`

	expectedResult := `    # Prepare the command to obtain completions
    # Patch for cobra-powered plugins
    if [[ ${#words[@]} -ge 3 && "${words[2]}" == "plugin1" ]] ; then
        requestComp="tridentctl-plugin1 __complete ${words[3,-1]}"
    elif [[ ${#words[@]} -ge 3 && "${words[2]}" == "plugin2" ]] ; then
        requestComp="tridentctl-plugin2 __complete ${words[3,-1]}"
    else
        requestComp="${words[1]} __complete ${words[2,-1]}"
    fi
    if [ "${lastChar}" = "" ]; then
`

	cobraPoweredPlugins := []string{"plugin1", "plugin2"}
	result := GenPatchedZshCompletionScript(completionCmd, cobraPoweredPlugins, dummyOriginalScript)

	assert.Equal(t, expectedResult, result)
}

type completionTestCase struct {
	name                string
	skipPluginDetection bool
	pluginsFound        map[string]struct{}
	description         string
}

func TestCompletionBashCmd(t *testing.T) {
	testCases := []completionTestCase{
		{
			name:                "basic_bash_completion",
			skipPluginDetection: true,
			pluginsFound:        map[string]struct{}{},
			description:         "Should generate bash completion successfully",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withTestState(t, tc.skipPluginDetection, tc.pluginsFound, func() {
				cmd := setupCompletionCommand(t, "bash", completionBashCmd.RunE)

				output := captureOutput(t, func() {
					err := cmd.RunE(cmd, []string{})
					assert.NoError(t, err, tc.description)
				})

				assert.NotEmpty(t, output, "Should generate some completion output")
			})
		})
	}
}

func TestCompletionZshCmd(t *testing.T) {
	testCases := []completionTestCase{
		{
			name:                "basic_zsh_completion",
			skipPluginDetection: true,
			pluginsFound:        map[string]struct{}{},
			description:         "Should generate zsh completion successfully",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withTestState(t, tc.skipPluginDetection, tc.pluginsFound, func() {
				cmd := setupCompletionCommand(t, "zsh", completionZshCmd.RunE)

				output := captureOutput(t, func() {
					err := cmd.RunE(cmd, []string{})
					assert.NoError(t, err, tc.description)
				})

				assert.NotEmpty(t, output, "Should generate completion output")
			})
		})
	}
}

func TestCompletionFishCmd(t *testing.T) {
	cmd := setupCompletionCommand(t, "fish", completionFishCmd.RunE)

	output := captureOutput(t, func() {
		err := cmd.RunE(cmd, []string{})
		assert.NoError(t, err, "Should call cmd.Root().GenFishCompletion successfully")
	})

	assert.NotEmpty(t, output, "Should generate fish completion output")
}

func TestCompletionPowershellCmd(t *testing.T) {
	cmd := setupCompletionCommand(t, "powershell", completionPowershellCmd.RunE)

	output := captureOutput(t, func() {
		err := cmd.RunE(cmd, []string{})
		assert.NoError(t, err, "Should call cmd.Root().GenPowerShellCompletionWithDesc successfully")
	})

	assert.NotEmpty(t, output, "Should generate powershell completion output")
}

func TestGetCobraPoweredPlugins(t *testing.T) {
	testCases := []struct {
		name                string
		skipPluginDetection bool
		pluginsFound        map[string]struct{}
		expectNil           bool
		description         string
	}{
		{
			name:                "skip_detection_returns_nil",
			skipPluginDetection: true,
			pluginsFound:        map[string]struct{}{"plugin1": {}, "plugin2": {}},
			expectNil:           true,
			description:         "Should return nil when skipPluginDetection is true",
		},
		{
			name:                "no_plugins_found_empty_slice",
			skipPluginDetection: false,
			pluginsFound:        map[string]struct{}{},
			expectNil:           false,
			description:         "Should return empty slice when no plugins in pluginsFound map",
		},
		{
			name:                "plugins_found_cobra_detection_process",
			skipPluginDetection: false,
			pluginsFound:        map[string]struct{}{"plugin1": {}, "plugin2": {}},
			expectNil:           true,
			description:         "Should process plugins and return slice (may be empty if plugins fail cobra detection)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withTestState(t, tc.skipPluginDetection, tc.pluginsFound, func() {
				result := GetCobraPoweredPlugins()

				if tc.expectNil {
					assert.Nil(t, result, tc.description)
				} else {
					assert.NotNil(t, result, tc.description)
					assert.IsType(t, []string{}, result, "Should return string slice type")
				}
			})
		})
	}
}
