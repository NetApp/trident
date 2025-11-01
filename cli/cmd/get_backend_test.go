// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

var getBackendTestMutex sync.RWMutex

func withBackendTestMode(t *testing.T, mode string, fn func()) {
	t.Helper()

	getBackendTestMutex.Lock()
	defer getBackendTestMutex.Unlock()

	origMode := OperatingMode
	defer func() {
		OperatingMode = origMode
	}()

	OperatingMode = mode
	fn()
}

func getFakeBackends1() []storage.BackendExternal {
	return []storage.BackendExternal{
		{
			Name:        "backend1",
			BackendUUID: "uuid-1234",
			State:       storage.Online,
			Online:      true,
			Config: map[string]interface{}{
				"storageDriverName": "ontap-nas",
			},
			Volumes: []string{"vol1", "vol2"},
		},
		{
			Name:        "backend2",
			BackendUUID: "uuid-5678",
			State:       storage.Online,
			Online:      true,
			Config: map[string]interface{}{
				"storageDriverName": "gcp-gcnv",
			},
			Volumes: []string{},
		},
	}
}

func TestGetBackendCmd_RunE(t *testing.T) {
	// Test tunnel mode success
	t.Run("tunnel mode success", func(t *testing.T) {
		withBackendTestMode(t, ModeTunnel, func() {
			prevExecKubernetesCLIRaw := execKubernetesCLIRaw
			defer func() {
				execKubernetesCLIRaw = prevExecKubernetesCLIRaw
			}()

			// Mock execKubernetesCLIRaw for tunnel mode
			execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
				return exec.Command("echo", "success")
			}

			cmd := &cobra.Command{}
			err := getBackendCmd.RunE(cmd, []string{})
			assert.NoError(t, err)
		})
	})

	// Test direct mode success
	t.Run("direct mode success", func(t *testing.T) {
		withBackendTestMode(t, ModeDirect, func() {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			// Mock GET /backend to return list of backends
			httpmock.RegisterResponder("GET", BaseURL()+"/backend",
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewJsonResponse(200, rest.ListBackendsResponse{
						Backends: []string{"backend1", "backend2"},
					})
				})

			// Mock GET /backend/{name} for individual backend details
			httpmock.RegisterResponder("GET", BaseURL()+"/backend/backend1",
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewJsonResponse(200, api.GetBackendResponse{
						Backend: storage.BackendExternal{
							Name:        "backend1",
							BackendUUID: "uuid-1234",
							State:       storage.Online,
							Online:      true,
							Config: map[string]interface{}{
								"storageDriverName": "ontap-nas",
							},
						},
					})
				})

			httpmock.RegisterResponder("GET", BaseURL()+"/backend/backend2",
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewJsonResponse(200, api.GetBackendResponse{
						Backend: storage.BackendExternal{
							Name:        "backend2",
							BackendUUID: "uuid-5678",
							State:       storage.Online,
							Online:      true,
							Config: map[string]interface{}{
								"storageDriverName": "gcp-gcnv",
							},
						},
					})
				})

			cmd := &cobra.Command{}
			err := getBackendCmd.RunE(cmd, []string{})
			assert.NoError(t, err)
		})
	})
}

func TestBackendList(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	backends := getFakeBackends1()

	testCases := []struct {
		name                string
		backendNames        []string
		wantErr             bool
		getBackendsResponse httpmock.Responder
		getBackendResponse  httpmock.Responder
	}{
		{
			name:         "get all backends successfully",
			backendNames: []string{},
			wantErr:      false,
			getBackendsResponse: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewJsonResponse(200, rest.ListBackendsResponse{
					Backends: []string{"backend1", "backend2"},
				})
			},
			getBackendResponse: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/backend/backend1" {
					return httpmock.NewJsonResponse(200, api.GetBackendResponse{Backend: backends[0]})
				}
				if req.URL.Path == "/backend/backend2" {
					return httpmock.NewJsonResponse(200, api.GetBackendResponse{Backend: backends[1]})
				}
				return httpmock.NewStringResponse(404, "not found"), nil
			},
		},
		{
			name:         "get specific backend successfully",
			backendNames: []string{"backend1"},
			wantErr:      false,
			getBackendResponse: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewJsonResponse(200, api.GetBackendResponse{Backend: backends[0]})
			},
		},
		{
			name:         "get backends list failed",
			backendNames: []string{},
			wantErr:      true,
			getBackendsResponse: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(500, `{"error": "server error"}`), nil
			},
		},
		{
			name:         "get specific backend failed",
			backendNames: []string{"nonexistent"},
			wantErr:      true,
			getBackendResponse: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(404, `{"error": "backend not found"}`), nil
			},
		},
		{
			name:         "get all backends with one not found",
			backendNames: []string{},
			wantErr:      false,
			getBackendsResponse: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewJsonResponse(200, rest.ListBackendsResponse{
					Backends: []string{"backend1", "nonexistent"},
				})
			},
			getBackendResponse: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/backend/backend1" {
					return httpmock.NewJsonResponse(200, api.GetBackendResponse{Backend: backends[0]})
				}
				return httpmock.NewStringResponse(404, "not found"), nil
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			if tc.getBackendsResponse != nil {
				httpmock.RegisterResponder("GET", BaseURL()+"/backend", tc.getBackendsResponse)
			}
			if tc.getBackendResponse != nil {
				if len(tc.backendNames) == 0 && tc.name == "get all backends successfully" {
					// Register multiple endpoints for list all scenario
					httpmock.RegisterResponder("GET", BaseURL()+"/backend/backend1", tc.getBackendResponse)
					httpmock.RegisterResponder("GET", BaseURL()+"/backend/backend2", tc.getBackendResponse)
				} else if len(tc.backendNames) == 0 && tc.name == "get all backends with one not found" {
					// Register multiple endpoints for list all with error scenario
					httpmock.RegisterResponder("GET", BaseURL()+"/backend/backend1", tc.getBackendResponse)
					httpmock.RegisterResponder("GET", BaseURL()+"/backend/nonexistent", tc.getBackendResponse)
				} else if len(tc.backendNames) > 0 {
					// Register specific backend endpoint
					httpmock.RegisterResponder("GET", BaseURL()+"/backend/"+tc.backendNames[0], tc.getBackendResponse)
				} else {
					// Register specific backend endpoint for error cases
					httpmock.RegisterResponder("GET", BaseURL()+"/backend/nonexistent", tc.getBackendResponse)
				}
			}

			origOut := os.Stdout
			os.Stdout, _ = os.Open(os.DevNull)
			defer func() { os.Stdout = origOut }()

			err := backendList(tc.backendNames)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetBackends(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	backend := getFakeBackends1()[0]

	testCases := []struct {
		name           string
		functionName   string
		url            string
		input          string
		response       httpmock.Responder
		wantErr        bool
		expectedResult interface{}
		checkNotFound  bool
	}{
		// GetBackends test cases
		{
			name:         "GetBackends - successful",
			functionName: "GetBackends",
			url:          BaseURL() + "/backend",
			response: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewJsonResponse(200, rest.ListBackendsResponse{
					Backends: []string{"backend1", "backend2"},
				})
			},
			wantErr:        false,
			expectedResult: []string{"backend1", "backend2"},
		},
		{
			name:         "GetBackends - HTTP error",
			functionName: "GetBackends",
			url:          BaseURL() + "/backend",
			response: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("network error")
			},
			wantErr:        true,
			expectedResult: ([]string)(nil),
		},
		{
			name:         "GetBackends - bad status code",
			functionName: "GetBackends",
			url:          BaseURL() + "/backend",
			response: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(500, `{"error": "server error"}`), nil
			},
			wantErr:        true,
			expectedResult: ([]string)(nil),
		},
		{
			name:         "GetBackends - invalid JSON",
			functionName: "GetBackends",
			url:          BaseURL() + "/backend",
			response: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(200, "invalid json"), nil
			},
			wantErr:        true,
			expectedResult: ([]string)(nil),
		},
		// GetBackend test cases
		{
			name:         "GetBackend - successful",
			functionName: "GetBackend",
			input:        "backend1",
			url:          BaseURL() + "/backend/backend1",
			response: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewJsonResponse(200, api.GetBackendResponse{Backend: backend})
			},
			wantErr:        false,
			expectedResult: backend.Name,
		},
		{
			name:         "GetBackend - HTTP error",
			functionName: "GetBackend",
			input:        "backend1",
			url:          BaseURL() + "/backend/backend1",
			response: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("network error")
			},
			wantErr:        true,
			expectedResult: "",
		},
		{
			name:         "GetBackend - not found",
			functionName: "GetBackend",
			input:        "nonexistent",
			url:          BaseURL() + "/backend/nonexistent",
			response: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(404, `{"error": "backend not found"}`), nil
			},
			wantErr:        true,
			checkNotFound:  true,
			expectedResult: "",
		},
		{
			name:         "GetBackend - other error status",
			functionName: "GetBackend",
			input:        "backend1",
			url:          BaseURL() + "/backend/backend1",
			response: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(500, `{"error": "server error"}`), nil
			},
			wantErr:        true,
			expectedResult: "",
		},
		{
			name:         "GetBackend - invalid JSON",
			functionName: "GetBackend",
			input:        "backend1",
			url:          BaseURL() + "/backend/backend1",
			response: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(200, "invalid json"), nil
			},
			wantErr:        true,
			expectedResult: "",
		},
		// GetBackendByBackendUUID test cases
		{
			name:         "GetBackendByBackendUUID - successful",
			functionName: "GetBackendByBackendUUID",
			input:        "uuid-1234",
			url:          BaseURL() + "/backend/uuid-1234",
			response: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewJsonResponse(200, api.GetBackendResponse{Backend: backend})
			},
			wantErr:        false,
			expectedResult: backend.Name,
		},
		{
			name:         "GetBackendByBackendUUID - HTTP error",
			functionName: "GetBackendByBackendUUID",
			input:        "uuid-1234",
			url:          BaseURL() + "/backend/uuid-1234",
			response: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("network error")
			},
			wantErr:        true,
			expectedResult: "",
		},
		{
			name:         "GetBackendByBackendUUID - not found",
			functionName: "GetBackendByBackendUUID",
			input:        "nonexistent-uuid",
			url:          BaseURL() + "/backend/nonexistent-uuid",
			response: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(404, `{"error": "backend not found"}`), nil
			},
			wantErr:        true,
			expectedResult: "",
		},
		{
			name:         "GetBackendByBackendUUID - invalid JSON",
			functionName: "GetBackendByBackendUUID",
			input:        "uuid-1234",
			url:          BaseURL() + "/backend/uuid-1234",
			response: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(200, "invalid json"), nil
			},
			wantErr:        true,
			expectedResult: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			httpmock.RegisterResponder("GET", tc.url, tc.response)

			switch tc.functionName {
			case "GetBackends":
				backends, err := GetBackends()
				if tc.wantErr {
					assert.Error(t, err)
					assert.Equal(t, tc.expectedResult, backends)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tc.expectedResult, backends)
				}

			case "GetBackend":
				result, err := GetBackend(tc.input)
				if tc.wantErr {
					assert.Error(t, err)
					if tc.checkNotFound {
						assert.True(t, errors.IsNotFoundError(err))
					}
					assert.Equal(t, tc.expectedResult, result.Name)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tc.expectedResult, result.Name)
				}

			case "GetBackendByBackendUUID":
				result, err := GetBackendByBackendUUID(tc.input)
				if tc.wantErr {
					assert.Error(t, err)
					assert.Equal(t, tc.expectedResult, result.Name)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tc.expectedResult, result.Name)
				}
			}
		})
	}
}

func TestWriteBackends(t *testing.T) {
	backends := getFakeBackends1()

	testCases := []struct {
		name         string
		outputFormat string
		contains     []string
	}{
		{
			name:         "JSON format",
			outputFormat: FormatJSON,
			contains:     []string{"backend1", "ontap-nas"},
		},
		{
			name:         "YAML format",
			outputFormat: FormatYAML,
			contains:     []string{"backend1", "ontap-nas"},
		},
		{
			name:         "Name format",
			outputFormat: FormatName,
			contains:     []string{"backend1", "backend2"},
		},
		{
			name:         "Default format (table)",
			outputFormat: "",
			contains:     []string{"backend1", "ontap-nas"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origFormat := OutputFormat
			defer func() {
				OutputFormat = origFormat
			}()

			OutputFormat = tc.outputFormat

			origOut := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			WriteBackends(backends)

			w.Close()
			os.Stdout = origOut

			output, _ := io.ReadAll(r)
			outputStr := string(output)

			for _, expected := range tc.contains {
				assert.Contains(t, outputStr, expected)
			}
		})
	}
}

func TestStorageDriverConfigs(t *testing.T) {
	testCases := []struct {
		name          string
		functionName  string
		configMap     map[string]interface{}
		wantErr       bool
		errorContains string
	}{
		// FakeStorageDriverConfig test cases
		{
			name:         "FakeStorageDriverConfig - valid config",
			functionName: "FakeStorageDriverConfig",
			configMap: map[string]interface{}{
				"version":           1,
				"storageDriverName": "fake",
				"backendName":       "fake-backend",
			},
			wantErr: false,
		},
		{
			name:         "FakeStorageDriverConfig - empty config",
			functionName: "FakeStorageDriverConfig",
			configMap:    map[string]interface{}{},
			wantErr:      false,
		},
		{
			name:         "FakeStorageDriverConfig - marshal error",
			functionName: "FakeStorageDriverConfig",
			configMap: map[string]interface{}{
				"invalidField": make(chan int),
			},
			wantErr:       true,
			errorContains: "json: unsupported type",
		},
		// OntapStorageDriverConfig test cases
		{
			name:         "OntapStorageDriverConfig - valid config",
			functionName: "OntapStorageDriverConfig",
			configMap: map[string]interface{}{
				"version":           1,
				"storageDriverName": "ontap-nas",
				"managementLIF":     "192.168.1.100",
				"svm":               "svm1",
			},
			wantErr: false,
		},
		{
			name:         "OntapStorageDriverConfig - minimal config",
			functionName: "OntapStorageDriverConfig",
			configMap: map[string]interface{}{
				"storageDriverName": "ontap-nas",
			},
			wantErr: false,
		},
		{
			name:         "OntapStorageDriverConfig - empty config",
			functionName: "OntapStorageDriverConfig",
			configMap:    map[string]interface{}{},
			wantErr:      false,
		},
		{
			name:         "OntapStorageDriverConfig - marshal error",
			functionName: "OntapStorageDriverConfig",
			configMap: map[string]interface{}{
				"channel": make(chan string),
			},
			wantErr:       true,
			errorContains: "json: unsupported type",
		},
		// SolidfireStorageDriverConfig test cases
		{
			name:         "SolidfireStorageDriverConfig - valid config",
			functionName: "SolidfireStorageDriverConfig",
			configMap: map[string]interface{}{
				"version":           1,
				"storageDriverName": "solidfire-san",
				"endpoint":          "https://admin:pass@192.168.1.100/json-rpc/8.0",
			},
			wantErr: false,
		},
		{
			name:         "SolidfireStorageDriverConfig - minimal config",
			functionName: "SolidfireStorageDriverConfig",
			configMap: map[string]interface{}{
				"endpoint": "https://admin:pass@10.0.0.1/json-rpc/8.0",
			},
			wantErr: false,
		},
		{
			name:         "SolidfireStorageDriverConfig - marshal error",
			functionName: "SolidfireStorageDriverConfig",
			configMap: map[string]interface{}{
				"callback": func() string { return "test" },
			},
			wantErr:       true,
			errorContains: "json: unsupported type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var result interface{}
			var err error

			switch tc.functionName {
			case "FakeStorageDriverConfig":
				result, err = getFakeStorageDriverConfig(tc.configMap)
			case "OntapStorageDriverConfig":
				result, err = getOntapStorageDriverConfig(tc.configMap)
			case "SolidfireStorageDriverConfig":
				result, err = getSolidfireStorageDriverConfig(tc.configMap)
			}

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
