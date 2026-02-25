// Copyright 2025 NetApp, Inc. All Rights Reserved.
package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	versionutils "github.com/netapp/trident/utils/version"
)

var versionTestMutex sync.RWMutex

const (
	testVersionValue = "1.0.0"
	testGoVersion    = "go1.19"
	testAPIVersion   = "v1"
	testLongVersion  = "26.06.0-custom+20250101"
)

func withVersionTestMode(clientOnlyVal bool, operatingMode, outputFormat string, fn func()) {
	versionTestMutex.Lock()
	defer versionTestMutex.Unlock()

	oldClientOnly := clientOnly
	oldOperatingMode := OperatingMode
	oldOutputFormat := OutputFormat

	clientOnly = clientOnlyVal
	OperatingMode = operatingMode
	OutputFormat = outputFormat

	defer func() {
		clientOnly = oldClientOnly
		OperatingMode = oldOperatingMode
		OutputFormat = oldOutputFormat
	}()

	fn()
}

func captureVersionOutput(fn func()) string {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = oldStdout

	output := make([]byte, 1024)
	n, _ := r.Read(output)
	return string(output[:n])
}

func TestVersionCmd(t *testing.T) {
	testCases := []struct {
		name          string
		clientOnly    bool
		operatingMode string
		wantErr       bool
		setupMocks    func() func() // Return cleanup function
	}{
		{
			name:       "client only success",
			clientOnly: true,
			wantErr:    false,
		},
		{
			name:          "server version rest mode success",
			clientOnly:    false,
			operatingMode: "",
			wantErr:       false,
			setupMocks: func() func() {
				httpmock.Activate()
				response := rest.GetVersionResponse{
					Version:    testLongVersion,
					GoVersion:  testGoVersion,
					ACPVersion: testLongVersion,
				}
				responder, _ := httpmock.NewJsonResponder(200, response)
				httpmock.RegisterResponder("GET", BaseURL()+"/version", responder)
				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:          "server version rest mode success without ACP",
			clientOnly:    false,
			operatingMode: "",
			wantErr:       false,
			setupMocks: func() func() {
				httpmock.Activate()
				response := rest.GetVersionResponse{
					Version:   testLongVersion,
					GoVersion: testGoVersion,
				}
				responder, _ := httpmock.NewJsonResponder(200, response)
				httpmock.RegisterResponder("GET", BaseURL()+"/version", responder)
				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:          "server version rest mode error",
			clientOnly:    false,
			operatingMode: "",
			wantErr:       true,
			setupMocks: func() func() {
				httpmock.Activate()
				httpmock.RegisterResponder("GET", BaseURL()+"/version",
					httpmock.NewErrorResponder(errors.New("network error")))
				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:          "server version tunnel mode success",
			clientOnly:    false,
			operatingMode: ModeTunnel,
			wantErr:       false,
			setupMocks: func() func() {
				originalTunnelCommand := TunnelCommandRaw
				TunnelCommandRaw = func(command []string) ([]byte, []byte, error) {
					response := api.VersionResponse{
						Server: &api.Version{
							Version:   testLongVersion,
							GoVersion: testGoVersion,
						},
						ACPServer: &api.Version{
							Version: testLongVersion,
						},
					}
					jsonBytes, _ := json.Marshal(response)
					return jsonBytes, []byte{}, nil
				}
				return func() { TunnelCommandRaw = originalTunnelCommand }
			},
		},
		{
			name:          "server version parse error",
			clientOnly:    false,
			operatingMode: "",
			wantErr:       true,
			setupMocks: func() func() {
				httpmock.Activate()
				response := rest.GetVersionResponse{
					Version: "invalid-version",
				}
				responder, _ := httpmock.NewJsonResponder(200, response)
				httpmock.RegisterResponder("GET", BaseURL()+"/version", responder)
				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:          "ACP version parse error",
			clientOnly:    false,
			operatingMode: "",
			wantErr:       true,
			setupMocks: func() func() {
				httpmock.Activate()
				response := rest.GetVersionResponse{
					Version:    testVersionValue,
					ACPVersion: "invalid-acp-version",
				}
				responder, _ := httpmock.NewJsonResponder(200, response)
				httpmock.RegisterResponder("GET", BaseURL()+"/version", responder)
				return func() { httpmock.DeactivateAndReset() }
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var cleanup func()
			if tc.setupMocks != nil {
				cleanup = tc.setupMocks()
				defer cleanup()
			}

			withVersionTestMode(tc.clientOnly, tc.operatingMode, "", func() {
				err := versionCmd.RunE(versionCmd, []string{})

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestGetVersionFromRest(t *testing.T) {
	testCases := []struct {
		name       string
		wantErr    bool
		setupMocks func() func() // Return cleanup function
	}{
		{
			name:    "network error",
			wantErr: true,
			setupMocks: func() func() {
				httpmock.Activate()
				httpmock.RegisterResponder("GET", BaseURL()+"/version",
					httpmock.NewErrorResponder(errors.New("network error")))
				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:    "HTTP 500 error",
			wantErr: true,
			setupMocks: func() func() {
				httpmock.Activate()
				httpmock.RegisterResponder("GET", BaseURL()+"/version",
					httpmock.NewStringResponder(500, "Internal Server Error"))
				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:    "invalid JSON response",
			wantErr: true,
			setupMocks: func() func() {
				httpmock.Activate()
				httpmock.RegisterResponder("GET", BaseURL()+"/version",
					httpmock.NewStringResponder(200, "invalid json"))
				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:    "successful response",
			wantErr: false,
			setupMocks: func() func() {
				httpmock.Activate()
				response := rest.GetVersionResponse{
					Version:   testLongVersion,
					GoVersion: testGoVersion,
				}
				responder, _ := httpmock.NewJsonResponder(200, response)
				httpmock.RegisterResponder("GET", BaseURL()+"/version", responder)
				return func() { httpmock.DeactivateAndReset() }
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var cleanup func()
			if tc.setupMocks != nil {
				cleanup = tc.setupMocks()
				defer cleanup()
			}

			_, err := getVersionFromRest()

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetVersionFromTunnel(t *testing.T) {
	testCases := []struct {
		name       string
		wantErr    bool
		setupMocks func() func() // Return cleanup function
	}{
		{
			name:    "tunnel command error",
			wantErr: true,
			setupMocks: func() func() {
				originalTunnelCommand := TunnelCommandRaw
				TunnelCommandRaw = func(command []string) ([]byte, []byte, error) {
					return nil, nil, errors.New("tunnel error")
				}
				return func() { TunnelCommandRaw = originalTunnelCommand }
			},
		},
		{
			name:    "tunnel command error with output",
			wantErr: true,
			setupMocks: func() func() {
				originalTunnelCommand := TunnelCommandRaw
				TunnelCommandRaw = func(command []string) ([]byte, []byte, error) {
					return []byte("error output"), nil, errors.New("tunnel error")
				}
				return func() { TunnelCommandRaw = originalTunnelCommand }
			},
		},
		{
			name:    "invalid JSON response",
			wantErr: true,
			setupMocks: func() func() {
				originalTunnelCommand := TunnelCommandRaw
				TunnelCommandRaw = func(command []string) ([]byte, []byte, error) {
					return []byte("invalid json"), []byte{}, nil
				}
				return func() { TunnelCommandRaw = originalTunnelCommand }
			},
		},
		{
			name:    "successful response without ACP",
			wantErr: false,
			setupMocks: func() func() {
				originalTunnelCommand := TunnelCommandRaw
				TunnelCommandRaw = func(command []string) ([]byte, []byte, error) {
					response := api.VersionResponse{
						Server: &api.Version{
							Version:   testLongVersion,
							GoVersion: testGoVersion,
						},
					}
					jsonBytes, _ := json.Marshal(response)
					return jsonBytes, []byte{}, nil
				}
				return func() { TunnelCommandRaw = originalTunnelCommand }
			},
		},
		{
			name:    "successful response with ACP",
			wantErr: false,
			setupMocks: func() func() {
				originalTunnelCommand := TunnelCommandRaw
				originalDebug := Debug
				Debug = true // Test the Debug path
				TunnelCommandRaw = func(command []string) ([]byte, []byte, error) {
					response := api.VersionResponse{
						Server: &api.Version{
							Version:   testLongVersion,
							GoVersion: testGoVersion,
						},
						ACPServer: &api.Version{
							Version: testLongVersion,
						},
					}
					jsonBytes, _ := json.Marshal(response)
					return jsonBytes, []byte("stderr output"), nil
				}
				return func() {
					TunnelCommandRaw = originalTunnelCommand
					Debug = originalDebug
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var cleanup func()
			if tc.setupMocks != nil {
				cleanup = tc.setupMocks()
				defer cleanup()
			}

			_, err := getVersionFromTunnel()

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetClientVersion(t *testing.T) {
	result := getClientVersion()
	assert.NotNil(t, result)
	assert.NotNil(t, result.Client)
	assert.NotEmpty(t, result.Client.Version)
}

func TestAddClientVersion(t *testing.T) {
	testCases := []struct {
		name          string
		hasACPVersion bool
	}{
		{
			name:          "without ACP server version",
			hasACPVersion: false,
		},
		{
			name:          "with ACP server version",
			hasACPVersion: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			serverVersion, _ := versionutils.ParseSemantic(testVersionValue)
			var acpServerVersion *versionutils.Version
			if tc.hasACPVersion {
				acpServerVersion, _ = versionutils.ParseSemantic("2.0.0")
			}

			result := addClientVersion(serverVersion, acpServerVersion)
			assert.NotNil(t, result)
			assert.NotNil(t, result.Server)
			assert.NotNil(t, result.Client)
			if tc.hasACPVersion {
				assert.NotNil(t, result.ACPServer)
			} else {
				assert.Nil(t, result.ACPServer)
			}
		})
	}
}

func TestWriteVersionFormats(t *testing.T) {
	testCases := []struct {
		name         string
		outputFormat string
		expectJSON   bool
		expectYAML   bool
		expectTable  bool
	}{
		{
			name:         "JSON format",
			outputFormat: FormatJSON,
			expectJSON:   true,
		},
		{
			name:         "YAML format",
			outputFormat: FormatYAML,
			expectYAML:   true,
		},
		{
			name:         "Wide format",
			outputFormat: FormatWide,
			expectTable:  true,
		},
		{
			name:         "Default format",
			outputFormat: "default",
			expectTable:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			version := &api.ClientVersionResponse{
				Client: api.Version{
					Version:    testVersionValue,
					APIVersion: testAPIVersion,
					GoVersion:  testGoVersion,
				},
			}

			withVersionTestMode(false, "", tc.outputFormat, func() {
				output := captureVersionOutput(func() {
					writeVersion(version)
				})

				assert.NotEmpty(t, output)
				if tc.expectJSON {
					assert.Contains(t, output, `"version"`)
				}
				if tc.expectYAML {
					assert.Contains(t, output, "version:")
				}
				if tc.expectTable {
					assert.Contains(t, output, testVersionValue)
				}
			})
		})
	}
}

func TestWriteVersionsFormats(t *testing.T) {
	testCases := []struct {
		name         string
		outputFormat string
		expectJSON   bool
		expectYAML   bool
		expectTable  bool
	}{
		{
			name:         "JSON format",
			outputFormat: FormatJSON,
			expectJSON:   true,
		},
		{
			name:         "YAML format",
			outputFormat: FormatYAML,
			expectYAML:   true,
		},
		{
			name:         "Wide format",
			outputFormat: FormatWide,
			expectTable:  true,
		},
		{
			name:         "Default format",
			outputFormat: "default",
			expectTable:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			versions := &api.VersionResponse{
				Server: &api.Version{
					Version:    testVersionValue,
					APIVersion: testAPIVersion,
					GoVersion:  testGoVersion,
				},
				Client: &api.Version{
					Version:    testVersionValue,
					APIVersion: testAPIVersion,
					GoVersion:  testGoVersion,
				},
			}

			withVersionTestMode(false, "", tc.outputFormat, func() {
				output := captureVersionOutput(func() {
					writeVersions(versions)
				})

				assert.NotEmpty(t, output)
				if tc.expectJSON {
					assert.Contains(t, output, `"version"`)
				}
				if tc.expectYAML {
					assert.Contains(t, output, "version:")
				}
				if tc.expectTable {
					assert.Contains(t, output, testVersionValue)
				}
			})
		})
	}
}

func TestTableOutputFunctions(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func()
		expected []string
	}{
		{
			name: "writeVersionTable",
			testFunc: func() {
				version := &api.ClientVersionResponse{
					Client: api.Version{Version: testVersionValue},
				}
				writeVersionTable(version)
			},
			expected: []string{testVersionValue, "CLIENT VERSION"},
		},
		{
			name: "writeVersionsTable",
			testFunc: func() {
				versions := &api.VersionResponse{
					Server: &api.Version{Version: testVersionValue},
					Client: &api.Version{Version: testVersionValue},
				}
				writeVersionsTable(versions)
			},
			expected: []string{testVersionValue, "SERVER VERSION", "CLIENT VERSION"},
		},
		{
			name: "writeWideVersionTable",
			testFunc: func() {
				version := &api.ClientVersionResponse{
					Client: api.Version{
						Version:    testVersionValue,
						APIVersion: testAPIVersion,
						GoVersion:  testGoVersion,
					},
				}
				writeWideVersionTable(version)
			},
			expected: []string{testVersionValue, testAPIVersion, testGoVersion},
		},
		{
			name: "writeWideVersionsTable",
			testFunc: func() {
				versions := &api.VersionResponse{
					Server: &api.Version{
						Version:    testVersionValue,
						APIVersion: testAPIVersion,
						GoVersion:  testGoVersion,
					},
					Client: &api.Version{
						Version:    testVersionValue,
						APIVersion: testAPIVersion,
						GoVersion:  testGoVersion,
					},
				}
				writeWideVersionsTable(versions)
			},
			expected: []string{testVersionValue, testAPIVersion, testGoVersion},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := captureVersionOutput(tc.testFunc)
			assert.NotEmpty(t, output)
			for _, expected := range tc.expected {
				assert.Contains(t, strings.ToUpper(output), strings.ToUpper(expected))
			}
		})
	}
}
