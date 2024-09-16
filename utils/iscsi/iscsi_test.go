// Copyright 2024 NetApp, Inc. All Rights Reserved.

package iscsi

import (
	"context"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

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

func TestFilterTargets(t *testing.T) {
	type FilterCase struct {
		CommandOutput string
		InputPortal   string
		OutputIQNs    []string
	}
	tests := []FilterCase{
		{
			// Simple positive test, expect first
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.3:3260,-1 iqn.2010-01.com.solidfire:baz\n",
			InputPortal: "203.0.113.1:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:foo"},
		},
		{
			// Simple positive test, expect second
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar"},
		},
		{
			// Expect empty list
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n",
			InputPortal: "203.0.113.3:3260",
			OutputIQNs:  []string{},
		},
		{
			// Expect multiple
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar", "iqn.1992-08.com.netapp:baz"},
		},
		{
			// Bad input
			CommandOutput: "" +
				"Foobar\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{},
		},
		{
			// Good and bad input
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"Foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"Bar\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar", "iqn.1992-08.com.netapp:baz"},
		},
		{
			// Try nonstandard port number
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.2:3261,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3261",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:baz"},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputPortal, func(t *testing.T) {
			targets := filterTargets(testCase.CommandOutput, testCase.InputPortal)
			assert.Equal(t, testCase.OutputIQNs, targets, "Wrong targets returned")
		})
	}
}

func TestFormatPortal(t *testing.T) {
	type IPAddresses struct {
		InputPortal  string
		OutputPortal string
	}
	tests := []IPAddresses{
		{
			InputPortal:  "203.0.113.1",
			OutputPortal: "203.0.113.1:3260",
		},
		{
			InputPortal:  "203.0.113.1:3260",
			OutputPortal: "203.0.113.1:3260",
		},
		{
			InputPortal:  "203.0.113.1:3261",
			OutputPortal: "203.0.113.1:3261",
		},
		{
			InputPortal:  "[2001:db8::1]",
			OutputPortal: "[2001:db8::1]:3260",
		},
		{
			InputPortal:  "[2001:db8::1]:3260",
			OutputPortal: "[2001:db8::1]:3260",
		},
		{
			InputPortal:  "[2001:db8::1]:3261",
			OutputPortal: "[2001:db8::1]:3261",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputPortal, func(t *testing.T) {
			assert.Equal(t, testCase.OutputPortal, formatPortal(testCase.InputPortal), "Portal not correctly formatted")
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

	findMultipathsValue := GetFindMultipathValue(multipathConf)
	assert.Equal(t, "no", findMultipathsValue)

	inputStringCopy := strings.ReplaceAll(multipathConf, "find_multipaths", "#find_multipaths")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "find_multipaths no", "")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "yes")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'yes'")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'on'")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'off'")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "no", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "on")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "off")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "no", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "random")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "random", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "smart")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "smart", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "greedy")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "greedy", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'no'")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "no", findMultipathsValue)
}

func TestEnsureHostportFormatted(t *testing.T) {
	type IPAddresses struct {
		InputIP  string
		OutputIP string
	}
	tests := []IPAddresses{
		{
			InputIP:  "1.2.3.4:5678",
			OutputIP: "1.2.3.4:5678",
		},
		{
			InputIP:  "1.2.3.4",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "[1:2:3:4]:5678",
			OutputIP: "[1:2:3:4]:5678",
		},
		{
			InputIP:  "[1:2:3:4]",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "1:2:3:4",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "2607:f8b0:4006:818:0:0:0:2004",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]:5678",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]:5678",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputIP, func(t *testing.T) {
			assert.Equal(t, testCase.OutputIP, ensureHostportFormatted(testCase.InputIP),
				"Hostport not correctly formatted")
		})
	}
}
