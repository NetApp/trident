package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
