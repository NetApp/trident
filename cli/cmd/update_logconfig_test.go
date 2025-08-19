// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"os"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
	execCmd "github.com/netapp/trident/utils/exec"
)

func TestUpdateLoggingConfig_RunE(t *testing.T) {
	testCases := []struct {
		name             string
		operatingMode    string
		args             []string
		logLevel         string
		loggingWorkflows string
		loggingLayers    string
		configFilename   string
		wantErr          bool
		errorContains    string
		useMock          bool
	}{
		{
			name:             "tunnel mode with flags",
			operatingMode:    ModeTunnel,
			args:             []string{},
			logLevel:         "debug",
			loggingWorkflows: "crud",
			loggingLayers:    "core",
			wantErr:          true,
			useMock:          true,
		},
		{
			name:           "tunnel mode with config file",
			operatingMode:  ModeTunnel,
			args:           []string{},
			configFilename: "config.yaml",
			wantErr:        true,
			useMock:        true,
		},
		{
			name:             "direct mode",
			operatingMode:    ModeDirect,
			args:             []string{},
			logLevel:         "info",
			loggingWorkflows: "csi",
			loggingLayers:    "api",
			wantErr:          true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prevOperatingMode := OperatingMode
			prevLogLevel := logLevel
			prevLoggingWorkflows := loggingWorkflows
			prevLoggingLayers := loggingLayers
			prevConfigFilename := configFilename
			defer func() {
				OperatingMode = prevOperatingMode
				logLevel = prevLogLevel
				loggingWorkflows = prevLoggingWorkflows
				loggingLayers = prevLoggingLayers
				configFilename = prevConfigFilename
			}()

			if tc.useMock {
				mockCtrl := gomock.NewController(t)
				mockCommand := mockexec.NewMockCommand(mockCtrl)

				defer func(previousCommand execCmd.Command) {
					command = previousCommand
				}(command)

				command = mockCommand

				mockCommand.EXPECT().
					ExecuteWithTimeout(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]byte(""), errors.New("mock error for coverage")).
					AnyTimes()
			}

			OperatingMode = tc.operatingMode
			logLevel = tc.logLevel
			loggingWorkflows = tc.loggingWorkflows
			loggingLayers = tc.loggingLayers
			configFilename = tc.configFilename

			cmd := &cobra.Command{}
			err := updateLoggingConfig.RunE(cmd, tc.args)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLogConfigUpdate(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name          string
		level         string
		workflows     string
		layers        string
		wantErr       bool
		errorContains string
		setupMocks    func()
	}{
		{
			name:      "successful update all parameters",
			level:     "debug",
			workflows: "crud",
			layers:    "core",
			wantErr:   false,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/logging/level/debug",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("POST", BaseURL()+"/logging/workflows",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("POST", BaseURL()+"/logging/layers",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:          "empty log level error",
			level:         "",
			workflows:     "crud",
			layers:        "core",
			wantErr:       true,
			errorContains: "log level was empty, it is required",
		},
		{
			name:          "log level update error",
			level:         "info",
			workflows:     "crud",
			layers:        "core",
			wantErr:       true,
			errorContains: "could not update log level",
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/logging/level/info",
					httpmock.NewStringResponder(500, `{"error": "server error"}`))
			},
		},
		{
			name:          "workflows update error",
			level:         "info",
			workflows:     "crud",
			layers:        "core",
			wantErr:       true,
			errorContains: "could not update logging workflows",
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/logging/level/info",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("POST", BaseURL()+"/logging/workflows",
					httpmock.NewStringResponder(500, `{"error": "server error"}`))
			},
		},
		{
			name:          "layers update error",
			level:         "info",
			workflows:     "crud",
			layers:        "core",
			wantErr:       true,
			errorContains: "could not update logging layers",
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/logging/level/info",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("POST", BaseURL()+"/logging/workflows",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("POST", BaseURL()+"/logging/layers",
					httpmock.NewStringResponder(500, `{"error": "server error"}`))
			},
		},
		{
			name:      "network error",
			level:     "info",
			workflows: "crud",
			layers:    "core",
			wantErr:   true,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/logging/level/info",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			err := logConfigUpdate(tc.level, tc.workflows, tc.layers)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test tryAsYAML function
func TestTryAsYAML(t *testing.T) {
	testCases := []struct {
		name          string
		configFile    string
		wantErr       bool
		errorContains string
		description   string
	}{
		{
			name:        "nonexistent file",
			configFile:  "nonexistent.yaml",
			wantErr:     true,
			description: "Should error when file doesn't exist",
		},
		{
			name:        "empty filename",
			configFile:  "",
			wantErr:     true,
			description: "Should error with empty filename",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tryAsYAML(tc.configFile)

			if tc.wantErr {
				assert.Error(t, err, tc.description)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err, tc.description)
			}
		})
	}
}

func TestGetValuesFromConfig(t *testing.T) {
	testCases := []struct {
		name         string
		configFile   string
		fileContent  string
		stdinContent string
		wantErr      bool
		expectValues map[string]string
	}{
		{
			name:       "valid YAML config file",
			configFile: "test-config.yaml",
			fileContent: `logLevel: debug
workflows:
  - storage=create,delete
  - core=list,get
logLayers:
  - core
  - rest
  - storage`,
			wantErr: false,
			expectValues: map[string]string{
				"level":     "debug",
				"workflows": "core=list,get:storage=create,delete", // sorted
				"layers":    "core,rest,storage",
			},
		},
		{
			name:       "valid JSON config file",
			configFile: "test-config.json",
			fileContent: `{
  "logLevel": "info",
  "workflows": ["backend=update", "api=create"],
  "logLayers": ["api", "backend"]
}`,
			wantErr: false,
			expectValues: map[string]string{
				"level":     "info",
				"workflows": "api=create:backend=update", // sorted
				"layers":    "api,backend",
			},
		},
		{
			name:       "empty workflows and layers",
			configFile: "empty-config.yaml",
			fileContent: `logLevel: warn
workflows: []
logLayers: []`,
			wantErr: false,
			expectValues: map[string]string{
				"level":     "warn",
				"workflows": "",
				"layers":    "",
			},
		},
		{
			name:       "single workflow and layer",
			configFile: "single-config.yaml",
			fileContent: `logLevel: error
workflows:
  - core=create
logLayers:
  - core`,
			wantErr: false,
			expectValues: map[string]string{
				"level":     "error",
				"workflows": "core=create",
				"layers":    "core",
			},
		},
		{
			name:       "stdin input with dash",
			configFile: "-",
			stdinContent: `logLevel: trace
workflows:
  - volume=delete
  - node=list
logLayers:
  - volume
  - node`,
			wantErr: false,
			expectValues: map[string]string{
				"level":     "trace",
				"workflows": "node=list:volume=delete", // sorted
				"layers":    "volume,node",
			},
		},
		{
			name:       "stdin with empty config",
			configFile: "-",
			stdinContent: `logLevel: fatal
workflows: []
logLayers: []`,
			wantErr: false,
			expectValues: map[string]string{
				"level":     "fatal",
				"workflows": "",
				"layers":    "",
			},
		},
		{
			name:       "file not found",
			configFile: "nonexistent.yaml",
			wantErr:    true,
		},
		{
			name:       "invalid YAML file",
			configFile: "invalid.yaml",
			fileContent: `logLevel: debug
workflows:
  - invalid: yaml: structure
    - nested: incorrectly`,
			wantErr: true,
		},
		{
			name:         "invalid YAML from stdin",
			configFile:   "-",
			stdinContent: `invalid: yaml: content`,
			wantErr:      true,
		},
		{
			name:       "complex workflows with special characters",
			configFile: "complex-config.yaml",
			fileContent: `logLevel: debug
workflows:
  - "storage=create,delete,update"
  - "core=list,get,watch"
  - "api=post,put,patch"
logLayers:
  - storage
  - core
  - api`,
			wantErr: false,
			expectValues: map[string]string{
				"level":     "debug",
				"workflows": "api=post,put,patch:core=list,get,watch:storage=create,delete,update", // sorted
				"layers":    "storage,core,api",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.configFile != "-" && tc.fileContent != "" {
				err := os.WriteFile(tc.configFile, []byte(tc.fileContent), 0o644)
				assert.NoError(t, err)
				defer os.Remove(tc.configFile)
			}

			if tc.configFile == "-" {
				r, w, err := os.Pipe()
				assert.NoError(t, err)

				oldStdin := os.Stdin
				os.Stdin = r
				defer func() { os.Stdin = oldStdin }()

				go func() {
					defer w.Close()
					if tc.stdinContent != "" {
						w.Write([]byte(tc.stdinContent))
					}
				}()
			}

			values, err := getValuesFromConfig(tc.configFile)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, values)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectValues, values)
			}
		})
	}
}

func TestUnmarshalStdinLogConfig(t *testing.T) {
	testCases := []struct {
		name         string
		input        string
		wantErr      bool
		expectConfig *logConfigResp
	}{
		{
			name: "valid YAML from stdin",
			input: `logLevel: debug
workflows:
  - core=create,delete
  - storage=list
logLayers:
  - core
  - rest`,
			wantErr: false,
			expectConfig: &logConfigResp{
				LogLevel:  "debug",
				Workflows: []string{"core=create,delete", "storage=list"},
				LogLayers: []string{"core", "rest"},
			},
		},
		{
			name: "valid JSON from stdin",
			input: `{
  "logLevel": "info",
  "workflows": ["api=post", "backend=get"],
  "logLayers": ["api", "backend"]
}`,
			wantErr: false,
			expectConfig: &logConfigResp{
				LogLevel:  "info",
				Workflows: []string{"api=post", "backend=get"},
				LogLayers: []string{"api", "backend"},
			},
		},
		{
			name: "empty workflows and layers",
			input: `logLevel: warn
workflows: []
logLayers: []`,
			wantErr: false,
			expectConfig: &logConfigResp{
				LogLevel:  "warn",
				Workflows: []string{},
				LogLayers: []string{},
			},
		},
		{
			name:    "minimal config",
			input:   `logLevel: error`,
			wantErr: false,
			expectConfig: &logConfigResp{
				LogLevel:  "error",
				Workflows: nil,
				LogLayers: nil,
			},
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: false,
			expectConfig: &logConfigResp{
				LogLevel:  "",
				Workflows: nil,
				LogLayers: nil,
			},
		},
		{
			name: "invalid YAML structure",
			input: `logLevel: debug
workflows:
  - invalid: yaml: structure
    - nested: incorrectly
logLayers: [core]`,
			wantErr: true,
		},
		{
			name:    "malformed YAML",
			input:   `logLevel debug workflows core`,
			wantErr: true,
		},
		{
			name: "YAML with quoted strings and special characters",
			input: `logLevel: "trace"
workflows:
  - "volume=create,delete"
  - "snapshot=list,get"
logLayers:
  - "volume"
  - "snapshot"`,
			wantErr: false,
			expectConfig: &logConfigResp{
				LogLevel:  "trace",
				Workflows: []string{"volume=create,delete", "snapshot=list,get"},
				LogLayers: []string{"volume", "snapshot"},
			},
		},
		{
			name: "YAML with tabs should fail",
			input: `logLevel: "trace"
workflows:
	- "volume=create,delete"
	- "snapshot=list,get"`,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			assert.NoError(t, err)

			oldStdin := os.Stdin
			os.Stdin = r
			defer func() { os.Stdin = oldStdin }()

			go func() {
				defer w.Close()
				if tc.input != "" {
					w.Write([]byte(tc.input))
				}
			}()

			config, err := unmarshalStdinLogConfig()

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectConfig, config)
			}
		})
	}
}

func TestUnmarshalStdinLogConfig_ReadError(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	assert.NoError(t, err)
	w.Close()
	r.Close()

	os.Stdin = r

	config, err := unmarshalStdinLogConfig()

	assert.Error(t, err)
	assert.NotNil(t, config)
}
