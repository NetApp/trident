// Copyright 2021 NetApp, Inc. All Rights Reserved.

package utils

import (
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestParseIPv6Valid(t *testing.T) {
	log.Debug("Running TestParseIPv6Valid...")

	tests := map[string]struct {
		input     string
		output    bool
		predicate func(string) bool
	}{
		"IPv6 Address": {
			input:  "fd20:8b1e:b258:2000:f816:3eff:feec:0",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Address with Brackets": {
			input:  "[fd20:8b1e:b258:2000:f816:3eff:feec:0]",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Address with Port": {
			input:  "[fd20:8b1e:b258:2000:f816:3eff:feec:0]:8000",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 localhost Address": {
			input:  "::1",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 localhost Address with Brackets": {
			input:  "[::1]",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Zero Address": {
			input:  "::",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Zero Address with Brackets": {
			input:  "[::]",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
	}
	for testName, test := range tests {
		t.Logf("Running test case '%s'", testName)

		assert.True(t, test.predicate(test.input), "Predicate failed")
	}
}

func TestParseIPv4Valid(t *testing.T) {
	log.Debug("Running TestParseIPv4Valid...")

	tests := map[string]struct {
		input     string
		output    bool
		predicate func(string) bool
	}{
		"IPv4 Address": {
			input:  "127.0.0.1",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv4 Address with Brackets": {
			input:  "[127.0.0.1]",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv4 Address with Port": {
			input:  "127.0.0.1:8000",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv4 Zero Address": {
			input:  "0.0.0.0",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
	}
	for testName, test := range tests {
		t.Logf("Running test case '%s'", testName)

		assert.False(t, test.predicate(test.input), "Predicate failed")
	}
}

func TestSanitizeString(t *testing.T) {
	log.Debug("Running TestSanitizeString...")

	tests := map[string]struct {
		input  string
		output string
	}{
		"Replace xtermControlRegex#1": {
			input:  "\x1B[A" + "HelloWorld",
			output: "HelloWorld",
		},
		"Strip trailing newline": {
			input:  "\n\n",
			output: "\n",
		},
	}
	for testName, test := range tests {
		t.Logf("Running test case '%s'", testName)
		result := sanitizeString(test.input)
		t.Logf("      test.input: '%s'", test.input)
		t.Logf("     test.output: '%s'", test.output)
		t.Logf("          result: '%s'", result)
		assert.True(t, test.output == result, fmt.Sprintf("Expected %v not %v", test.output, result))
	}
}

func TestPidRunningOrIdleRegex(t *testing.T) {
	log.Debug("Running TestPidRegexes...")

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
		t.Logf("Running test case '%s'", testName)
		result := pidRunningOrIdleRegex.MatchString(test.input)
		t.Logf("              test.input: '%s'", test.input)
		t.Logf("     test.expectedOutput: '%v'", test.expectedOutput)
		t.Logf("           actual result: '%v'", result)
		assert.True(t, test.expectedOutput == result)
	}
}

func TestGetHostportIP(t *testing.T) {
	log.Debug("Running TestGetHostportIP...")

	type IPAddresses struct {
		InputIP  string
		OutputIP string
	}

	tests := []IPAddresses{
		{
			InputIP:  "1.2.3.4:5678",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "1.2.3.4",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "[1:2:3:4]:5678",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "[1:2:3:4]",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]:5678",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "2607:f8b0:4006:818:0:0:0:2004",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
	}
	for _, testCase := range tests {
		assert.Equal(t, testCase.OutputIP, getHostportIP(testCase.InputIP), "IP mismatch")
	}
}

func TestEnsureHostportFormatted(t *testing.T) {
	log.Debug("Running TestEnsureHostportFormatted...")

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
		assert.Equal(t, testCase.OutputIP, ensureHostportFormatted(testCase.InputIP),
			"Hostport not correctly formatted")
	}
}

func TestFormatPortal(t *testing.T) {
	log.Debug("Running TestFormatPortal...")

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
		assert.Equal(t, testCase.OutputPortal, formatPortal(testCase.InputPortal),
			"Portal not correctly formatted")
	}
}

func TestFilterTargets(t *testing.T) {
	log.Debug("Running TestFilterTargets...")

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
		targets := filterTargets(testCase.CommandOutput, testCase.InputPortal)
		assert.Equal(t, testCase.OutputIQNs, targets, "Wrong targets returned")
	}
}
