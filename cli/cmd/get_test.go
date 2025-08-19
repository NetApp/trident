// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureStdout(t *testing.T) (*os.File, func() string) {
	origOut := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	return origOut, func() string {
		w.Close()
		os.Stdout = origOut
		output, _ := io.ReadAll(r)
		return string(output)
	}
}

const (
	jsonIndent  = "  "
	emptyPrefix = ""
)

func TestGetCmd_PersistentPreRunE(t *testing.T) {
	originalOperatingMode := OperatingMode
	originalServer := Server
	defer func() {
		OperatingMode = originalOperatingMode
		Server = originalServer
	}()

	testCases := []struct {
		name        string
		cmd         *cobra.Command
		args        []string
		setupTest   func()
		expectError bool
	}{
		{
			name: "valid command execution with direct mode",
			cmd:  &cobra.Command{},
			args: []string{},
			setupTest: func() {
				Server = "http://localhost:8000"
			},
			expectError: false,
		},
		{
			name: "with arguments in direct mode",
			cmd:  &cobra.Command{},
			args: []string{"arg1", "arg2"},
			setupTest: func() {
				Server = "http://localhost:8000"
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setupTest != nil {
				tc.setupTest()
			}

			err := getCmd.PersistentPreRunE(tc.cmd, tc.args)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		contains []string
	}{
		{
			name: "simple object",
			input: map[string]interface{}{
				"name":    "test",
				"version": 1,
				"active":  true,
			},
			contains: []string{
				`"name": "test"`,
				`"version": 1`,
				`"active": true`,
			},
		},
		{
			name: "array of objects",
			input: []map[string]interface{}{
				{"id": 1, "name": "first"},
				{"id": 2, "name": "second"},
			},
			contains: []string{
				`"id": 1`,
				`"name": "first"`,
				`"id": 2`,
				`"name": "second"`,
			},
		},
		{
			name:     "nil input",
			input:    nil,
			contains: []string{"null"},
		},
		{
			name:     "empty object",
			input:    map[string]interface{}{},
			contains: []string{"{}"},
		},
		{
			name: "nested object",
			input: map[string]interface{}{
				"config": map[string]interface{}{
					"name":    "nested",
					"enabled": true,
				},
				"metadata": map[string]interface{}{
					"version": "1.0",
				},
			},
			contains: []string{
				`"config"`,
				`"name": "nested"`,
				`"enabled": true`,
				`"metadata"`,
				`"version": "1.0"`,
			},
		},
		{
			name: "formatting with indentation",
			input: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": "value",
					},
				},
			},
			contains: []string{
				`"level1"`,
				`"level2"`,
				`"level3": "value"`,
			},
		},
		{
			name: "special characters",
			input: map[string]interface{}{
				"unicode":       "测试 🎉 émojis",
				"special_chars": "!@#$%^&*()_+-={}[]|\\:;\"'<>?,./",
				"newlines":      "line1\nline2\r\nline3",
				"tabs":          "col1\tcol2\tcol3",
				"quotes":        `"quoted" and 'single quoted'`,
				"backslashes":   "C:\\Windows\\System32",
			},
			contains: []string{
				`"unicode"`,
				`"special_chars"`,
				`"newlines"`,
				`"tabs"`,
				`"quotes"`,
				`"backslashes"`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origOut, getOutput := captureStdout(t)
			defer func() { os.Stdout = origOut }()

			WriteJSON(tc.input)

			outputStr := getOutput()

			for _, expected := range tc.contains {
				assert.Contains(t, outputStr, expected)
			}

			if tc.input != nil {
				var parsed interface{}
				err := json.Unmarshal([]byte(outputStr), &parsed)
				assert.NoError(t, err, "Output should be valid JSON")
			}

			// Check indentation for formatting test case
			if tc.name == "formatting with indentation" {
				lines := strings.Split(outputStr, "\n")
				assert.GreaterOrEqual(t, len(lines), 3)
				foundIndentation := false
				for _, line := range lines {
					if strings.HasPrefix(line, jsonIndent) && !strings.HasPrefix(line, jsonIndent+jsonIndent) {
						foundIndentation = true
						break
					}
				}
				assert.True(t, foundIndentation, "Should find 2-space indentation")
			}

			// Check special character preservation
			if tc.name == "special characters" {
				var parsed map[string]interface{}
				err := json.Unmarshal([]byte(outputStr), &parsed)
				assert.NoError(t, err, "Output should be valid JSON")

				inputMap := tc.input.(map[string]interface{})
				assert.Equal(t, inputMap["unicode"], parsed["unicode"])
				assert.Equal(t, inputMap["special_chars"], parsed["special_chars"])
			}
		})
	}
}

func TestWriteYAML(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		contains []string
	}{
		{
			name: "simple object",
			input: map[string]interface{}{
				"name":    "test",
				"version": 1,
				"active":  true,
			},
			contains: []string{
				"name: test",
				"version: 1",
				"active: true",
			},
		},
		{
			name: "array of objects",
			input: []map[string]interface{}{
				{"id": 1, "name": "first"},
				{"id": 2, "name": "second"},
			},
			contains: []string{
				"- id: 1",
				"  name: first",
				"- id: 2",
				"  name: second",
			},
		},
		{
			name:     "nil input",
			input:    nil,
			contains: []string{"null"},
		},
		{
			name:     "empty object",
			input:    map[string]interface{}{},
			contains: []string{"{}"},
		},
		{
			name: "nested object",
			input: map[string]interface{}{
				"config": map[string]interface{}{
					"name":    "nested",
					"enabled": true,
				},
				"metadata": map[string]interface{}{
					"version": "1.0",
				},
			},
			contains: []string{
				"config:",
				"  name: nested",
				"  enabled: true",
				"metadata:",
				"  version: \"1.0\"",
			},
		},
		{
			name: "string values with special characters",
			input: map[string]interface{}{
				"description": "This is a test with special chars: @#$%",
				"path":        "/var/lib/trident",
				"url":         "https://example.com:8443/api/v1",
			},
			contains: []string{
				"description:",
				"path: /var/lib/trident",
				"url: https://example.com:8443/api/v1",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origOut, getOutput := captureStdout(t)
			defer func() { os.Stdout = origOut }()

			WriteYAML(tc.input)

			outputStr := getOutput()

			for _, expected := range tc.contains {
				assert.Contains(t, outputStr, expected)
			}
			if tc.input != nil && len(outputStr) > 0 {
				if len(tc.contains) > 1 || (len(tc.contains) == 1 && tc.contains[0] != "{}") {
					assert.False(t, strings.HasPrefix(outputStr, "{"), "Output should not start with JSON bracket")
					assert.False(t, strings.HasSuffix(strings.TrimSpace(outputStr), "}"), "Output should not end with JSON bracket")
				}
			}
		})
	}
}
