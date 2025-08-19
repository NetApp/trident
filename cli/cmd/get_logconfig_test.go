// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"os/exec"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestGetLogConfigCmd_RunE(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name          string
		operatingMode string
		setupMocks    func()
	}{
		{
			name:          "tunnel mode success",
			operatingMode: ModeTunnel,
			setupMocks: func() {
			},
		},
		{
			name:          "direct mode success",
			operatingMode: ModeDirect,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
					httpmock.NewStringResponder(200, `{"logLevel": "info"}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows/selected",
					httpmock.NewStringResponder(200, `{"logWorkflows": "crud"}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers/selected",
					httpmock.NewStringResponder(200, `{"logLayers": "core"}`))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			// Store original values
			prevOperatingMode := OperatingMode
			prevGetAllFlows := getAllFlows
			prevGetAllLayers := getAllLayers
			prevExecKubernetesCLIRaw := execKubernetesCLIRaw

			defer func() {
				OperatingMode = prevOperatingMode
				getAllFlows = prevGetAllFlows
				getAllLayers = prevGetAllLayers
				execKubernetesCLIRaw = prevExecKubernetesCLIRaw
			}()

			// Set up test conditions
			OperatingMode = tc.operatingMode
			getAllFlows = false
			getAllLayers = false

			// Mock execKubernetesCLIRaw for tunnel mode
			if tc.operatingMode == ModeTunnel {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					return exec.Command("echo", "Log config retrieved successfully")
				}
			}

			// Set up HTTP mocks for direct mode
			tc.setupMocks()

			cmd := &cobra.Command{}
			err := getLogConfigCmd.RunE(cmd, []string{})
			assert.NoError(t, err, "getLogConfigCmd.RunE should not return an error")
		})
	}
}

// Test getLogConfig function
func TestGetLogConfig(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name          string
		getAllFlows   bool
		getAllLayers  bool
		wantErr       bool
		errorContains string
		setupMocks    func()
	}{
		{
			name:         "successful log config retrieval",
			getAllFlows:  false,
			getAllLayers: false,
			wantErr:      false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
					httpmock.NewStringResponder(200, `{"logLevel": "info"}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows/selected",
					httpmock.NewStringResponder(200, `{"logWorkflows": "crud"}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers/selected",
					httpmock.NewStringResponder(200, `{"logLayers": "core"}`))
			},
		},
		{
			name:         "successful log config with all workflows",
			getAllFlows:  true,
			getAllLayers: false,
			wantErr:      false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
					httpmock.NewStringResponder(200, `{"logLevel": "debug"}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows",
					httpmock.NewStringResponder(200, `{"availableLoggingWorkflows": ["crud", "csi", "core"]}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers/selected",
					httpmock.NewStringResponder(200, `{"logLayers": "core"}`))
			},
		},
		{
			name:         "successful log config with all layers",
			getAllFlows:  false,
			getAllLayers: true,
			wantErr:      false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
					httpmock.NewStringResponder(200, `{"logLevel": "warn"}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows/selected",
					httpmock.NewStringResponder(200, `{"logWorkflows": "crud"}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers",
					httpmock.NewStringResponder(200, `{"availableLogLayers": ["core", "api", "csi"]}`))
			},
		},
		{
			name:          "error getting log level",
			getAllFlows:   false,
			getAllLayers:  false,
			wantErr:       true,
			errorContains: "could not get configured log level",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
					httpmock.NewStringResponder(500, `{"error": "server error"}`))
			},
		},
		{
			name:          "error getting workflows",
			getAllFlows:   false,
			getAllLayers:  false,
			wantErr:       true,
			errorContains: "could not get configured logging workflows",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
					httpmock.NewStringResponder(200, `{"logLevel": "info"}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows/selected",
					httpmock.NewStringResponder(500, `{"error": "server error"}`))
			},
		},
		{
			name:          "error getting layers",
			getAllFlows:   false,
			getAllLayers:  false,
			wantErr:       true,
			errorContains: "could not get configured logging layers",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
					httpmock.NewStringResponder(200, `{"logLevel": "info"}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows/selected",
					httpmock.NewStringResponder(200, `{"logWorkflows": "crud"}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers/selected",
					httpmock.NewStringResponder(500, `{"error": "server error"}`))
			},
		},
		{
			name:         "network error",
			getAllFlows:  false,
			getAllLayers: false,
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			prevGetAllFlows := getAllFlows
			prevGetAllLayers := getAllLayers
			defer func() {
				getAllFlows = prevGetAllFlows
				getAllLayers = prevGetAllLayers
			}()

			getAllFlows = tc.getAllFlows
			getAllLayers = tc.getAllLayers

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			err := getLogConfig()

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

// Test getLogLevel function
func TestGetLogLevel(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name          string
		wantErr       bool
		errorContains string
		setupMocks    func()
		expectedLevel string
	}{
		{
			name:          "successful log level retrieval",
			wantErr:       false,
			expectedLevel: "debug",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
					httpmock.NewStringResponder(200, `{"logLevel": "debug"}`))
			},
		},
		{
			name:          "server error during retrieval",
			wantErr:       true,
			errorContains: "could not get configured log level",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
					httpmock.NewStringResponder(500, `{"error": "internal server error"}`))
			},
		},
		{
			name:    "invalid JSON response",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
					httpmock.NewStringResponder(200, `invalid json`))
			},
		},
		{
			name:    "network error during HTTP call",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/level",
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

			result, err := getLogLevel()

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedLevel, result)
			}
		})
	}
}

// Test getWorkflows function
func TestGetWorkflows(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name           string
		getAllFlows    bool
		wantErr        bool
		errorContains  string
		setupMocks     func()
		expectedResult []string
	}{
		{
			name:           "successful selected workflows retrieval",
			getAllFlows:    false,
			wantErr:        false,
			expectedResult: []string{"crud"},
			setupMocks: func() {
				// getAllFlows=false hits /logging/workflows/selected endpoint (currently configured workflows)
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows/selected",
					httpmock.NewStringResponder(200, `{"logWorkflows": "crud"}`))
			},
		},
		{
			name:           "successful all workflows retrieval",
			getAllFlows:    true,
			wantErr:        false,
			expectedResult: []string{"crud", "csi", "core"},
			setupMocks: func() {
				// getAllFlows=true hits /logging/workflows endpoint (all available workflows)
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows",
					httpmock.NewStringResponder(200, `{"availableLoggingWorkflows": ["crud", "csi", "core"]}`))
			},
		},
		{
			name:          "server error during selected workflows retrieval",
			getAllFlows:   false,
			wantErr:       true,
			errorContains: "could not get configured logging workflows",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows/selected",
					httpmock.NewStringResponder(500, `{"error": "internal server error"}`))
			},
		},
		{
			name:          "server error during all workflows retrieval",
			getAllFlows:   true,
			wantErr:       true,
			errorContains: "could not get configured logging workflows",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows",
					httpmock.NewStringResponder(500, `{"error": "internal server error"}`))
			},
		},
		{
			name:        "invalid JSON response for selected",
			getAllFlows: false,
			wantErr:     true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows/selected",
					httpmock.NewStringResponder(200, `invalid json`))
			},
		},
		{
			name:        "invalid JSON response for all available workflows", // "all" = getAllFlows=true, gets all available workflows
			getAllFlows: true,
			wantErr:     true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows",
					httpmock.NewStringResponder(200, `invalid json`))
			},
		},
		{
			name:        "network error during HTTP call",
			getAllFlows: false,
			wantErr:     true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/workflows/selected",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			prevGetAllFlows := getAllFlows
			defer func() {
				getAllFlows = prevGetAllFlows
			}()

			getAllFlows = tc.getAllFlows

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			result, err := getWorkflows()

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

// Test getLogLayers function
func TestGetLogLayers(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name           string
		getAllLayers   bool
		wantErr        bool
		errorContains  string
		setupMocks     func()
		expectedResult []string
	}{
		{
			name:           "successful selected layers retrieval",
			getAllLayers:   false,
			wantErr:        false,
			expectedResult: []string{"core"},
			setupMocks: func() {
				// getAllLayers=false hits /logging/layers/selected endpoint (currently configured layers)
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers/selected",
					httpmock.NewStringResponder(200, `{"logLayers": "core"}`))
			},
		},
		{
			name:           "successful all layers retrieval",
			getAllLayers:   true,
			wantErr:        false,
			expectedResult: []string{"core", "api", "csi"},
			setupMocks: func() {
				// getAllLayers=true hits /logging/layers endpoint (all available layers)
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers",
					httpmock.NewStringResponder(200, `{"availableLogLayers": ["core", "api", "csi"]}`))
			},
		},
		{
			name:          "server error during selected layers retrieval",
			getAllLayers:  false,
			wantErr:       true,
			errorContains: "could not get configured logging layers",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers/selected",
					httpmock.NewStringResponder(500, `{"error": "internal server error"}`))
			},
		},
		{
			name:          "server error during all layers retrieval",
			getAllLayers:  true,
			wantErr:       true,
			errorContains: "could not get configured logging layers",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers",
					httpmock.NewStringResponder(500, `{"error": "internal server error"}`))
			},
		},
		{
			name:         "invalid JSON response for selected",
			getAllLayers: false,
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers/selected",
					httpmock.NewStringResponder(200, `invalid json`))
			},
		},
		{
			name:         "invalid JSON response for all available layers", // "all" = getAllLayers=true, gets all available layers
			getAllLayers: true,
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers",
					httpmock.NewStringResponder(200, `invalid json`))
			},
		},
		{
			name:         "network error during HTTP call",
			getAllLayers: false,
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/logging/layers/selected",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			prevGetAllLayers := getAllLayers
			defer func() {
				getAllLayers = prevGetAllLayers
			}()

			getAllLayers = tc.getAllLayers

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			result, err := getLogLayers()

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedResult, result)
			}
		})
	}
}

// Test writeLogConfig function with different output formats
func TestWriteLogConfig(t *testing.T) {
	testCases := []struct {
		name         string
		outputFormat string
		config       *logConfigResp
	}{
		{
			name:         "JSON format",
			outputFormat: FormatJSON,
			config: &logConfigResp{
				LogLevel:  "info",
				Workflows: []string{"crud"},
				LogLayers: []string{"core"},
			},
		},
		{
			name:         "YAML format",
			outputFormat: FormatYAML,
			config: &logConfigResp{
				LogLevel:  "debug",
				Workflows: []string{"crud", "csi"},
				LogLayers: []string{"core", "api"},
			},
		},
		{
			name:         "Default format",
			outputFormat: "",
			config: &logConfigResp{
				LogLevel:  "warn",
				Workflows: []string{"core"},
				LogLayers: []string{"api"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prevOutputFormat := OutputFormat
			defer func() {
				OutputFormat = prevOutputFormat
			}()

			OutputFormat = tc.outputFormat

			assert.NotPanics(t, func() {
				writeLogConfig(tc.config)
			})
		})
	}
}

// Test writeLogConfigHelper function with different flag combinations
func TestWriteLogConfigHelper(t *testing.T) {
	testCases := []struct {
		name         string
		getAllFlows  bool
		getAllLayers bool
		config       *logConfigResp
	}{
		{
			name:         "configured workflows and layers",
			getAllFlows:  false,
			getAllLayers: false,
			config: &logConfigResp{
				LogLevel:  "info",
				Workflows: []string{"crud"},
				LogLayers: []string{"core"},
			},
		},
		{
			name:         "available workflows, configured layers",
			getAllFlows:  true,
			getAllLayers: false,
			config: &logConfigResp{
				LogLevel:  "debug",
				Workflows: []string{"crud", "csi", "core"},
				LogLayers: []string{"core"},
			},
		},
		{
			name:         "configured workflows, available layers",
			getAllFlows:  false,
			getAllLayers: true,
			config: &logConfigResp{
				LogLevel:  "warn",
				Workflows: []string{"crud"},
				LogLayers: []string{"core", "api", "csi"},
			},
		},
		{
			name:         "available workflows and layers",
			getAllFlows:  true,
			getAllLayers: true,
			config: &logConfigResp{
				LogLevel:  "error",
				Workflows: []string{"crud", "csi", "core"},
				LogLayers: []string{"core", "api", "csi"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prevGetAllFlows := getAllFlows
			prevGetAllLayers := getAllLayers
			defer func() {
				getAllFlows = prevGetAllFlows
				getAllLayers = prevGetAllLayers
			}()

			getAllFlows = tc.getAllFlows
			getAllLayers = tc.getAllLayers

			assert.NotPanics(t, func() {
				writeLogConfigHelper(tc.config)
			})
		})
	}
}
