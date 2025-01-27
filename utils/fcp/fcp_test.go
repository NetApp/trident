// Copyright 2024 NetApp, Inc. All Rights Reserved.

package fcp

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	. "github.com/netapp/trident/logging"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
)

const multipathConf = `
defaults {
    user_friendly_names yes
    find_multipaths no
}

`

func TestMultipathdIsRunning(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockExec := mockexec.NewMockCommand(mockCtrl)
	tests := []struct {
		name          string
		execOut       string
		execErr       error
		returnCode    int
		expectedValue bool
	}{
		{name: "True", execOut: "1234", execErr: nil, expectedValue: true},
		{name: "False", execOut: "", execErr: nil, expectedValue: false},
		{name: "Error", execOut: "1234", execErr: errors.New("cmd error"), expectedValue: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup mock calls.
			mockExec.EXPECT().Execute(
				ctx, gomock.Any(), gomock.Any(),
			).Return([]byte(tt.execOut), tt.execErr)

			// Only mock out the second call if the expected value isn't true.
			if !tt.expectedValue {
				mockExec.EXPECT().Execute(
					ctx, gomock.Any(), gomock.Any(), gomock.Any(),
				).Return([]byte(tt.execOut), tt.execErr)
			}
			iscsiClient := NewDetailed("", mockExec, nil, nil, nil, nil, nil, nil)

			actualValue := iscsiClient.multipathdIsRunning(context.Background())
			assert.Equal(t, tt.expectedValue, actualValue)
		})
	}
}

func TestPidRunningOrIdleRegex(t *testing.T) {
	Log().Debug("Running TestPidRegexes...")

	tests := map[string]struct {
		input          string
		expectedOutput bool
	}{
		// Negative tests
		"Negative input #1": {
			input:          "",
			expectedOutput: false,
		},
		"Negative input #2": {
			input:          "pid -5 running",
			expectedOutput: false,
		},
		"Negative input #3": {
			input:          "pid running",
			expectedOutput: false,
		},
		// Positive tests
		"Positive input #1": {
			input:          "pid 5 running",
			expectedOutput: true,
		},
		// Positive tests
		"Positive input #2": {
			input:          "pid 2509 idle",
			expectedOutput: true,
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			result := pidRunningOrIdleRegex.MatchString(test.input)
			assert.True(t, test.expectedOutput == result)
		})
	}
}

func TestGetFindMultipathValue(t *testing.T) {
	Log().Debug("Running TestGetFindMultipathValue...")

	findMultipathsValue := getFindMultipathValue(multipathConf)
	assert.Equal(t, "no", findMultipathsValue)

	inputStringCopy := strings.ReplaceAll(multipathConf, "find_multipaths", "#find_multipaths")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "find_multipaths no", "")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "yes")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'yes'")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'on'")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'off'")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "no", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "on")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "off")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "no", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "random")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "random", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "smart")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "smart", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "greedy")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "greedy", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'no'")

	findMultipathsValue = getFindMultipathValue(inputStringCopy)
	assert.Equal(t, "no", findMultipathsValue)
}
