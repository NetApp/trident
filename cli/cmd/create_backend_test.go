// Copyright 2024 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
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

const (
	testBackendJSON = `{"version": 1, "storageDriverName": "ontap-nas", "backendName": "test-backend"}`
	testBackendYAML = `version: 1
storageDriverName: ontap-nas
backendName: test-backend`
)

func captureOutput(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	outputChan := make(chan string, 1)
	go func() {
		defer close(outputChan)
		output, _ := io.ReadAll(r)
		outputChan <- string(output)
	}()

	fn()

	w.Close()
	os.Stdout = origStdout

	output := <-outputChan
	r.Close()

	return output
}

func setupHTTPMock(t *testing.T) func() {
	t.Helper()
	httpmock.Activate()
	return func() {
		httpmock.DeactivateAndReset()
	}
}

func createTempFile(t *testing.T, content, pattern string) (string, func()) {
	t.Helper()

	tempFile, err := os.CreateTemp("", pattern)
	require.NoError(t, err)

	_, err = tempFile.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, tempFile.Close())

	return tempFile.Name(), func() {
		os.Remove(tempFile.Name())
	}
}

func getFakeBackend() storage.BackendExternal {
	return storage.BackendExternal{
		Name:        "test-backend",
		BackendUUID: "test-uuid-1234",
		State:       storage.Online,
		UserState:   "",
		Online:      true,
		Config: map[string]interface{}{
			"storageDriverName": "ontap-nas",
		},
		Volumes: []string{"vol1", "vol2"},
	}
}

func TestGetBackendData(t *testing.T) {
	testCases := []struct {
		name       string
		filename   string
		base64Data string
		setupFile  func() (string, func())
		wantErr    bool
		validate   func(t *testing.T, data []byte)
	}{
		{
			name:       "no input provided",
			filename:   "",
			base64Data: "",
			wantErr:    true,
		},
		{
			name: "valid YAML file",
			setupFile: func() (string, func()) {
				return createTempFile(t, testBackendYAML, "backend-*.yaml")
			},
			wantErr: false,
			validate: func(t *testing.T, data []byte) {
				var result map[string]interface{}
				err := json.Unmarshal(data, &result)
				assert.NoError(t, err)
				assert.Equal(t, "ontap-nas", result["storageDriverName"])
				assert.Equal(t, "test-backend", result["backendName"])
			},
		},
		{
			name: "valid JSON file",
			setupFile: func() (string, func()) {
				return createTempFile(t, testBackendJSON, "backend-*.json")
			},
			wantErr: false,
			validate: func(t *testing.T, data []byte) {
				var result map[string]interface{}
				err := json.Unmarshal(data, &result)
				assert.NoError(t, err)
				assert.Equal(t, "ontap-nas", result["storageDriverName"])
			},
		},
		{
			name:       "valid base64 data",
			base64Data: base64.StdEncoding.EncodeToString([]byte(testBackendJSON)),
			wantErr:    false,
			validate: func(t *testing.T, data []byte) {
				var result map[string]interface{}
				err := json.Unmarshal(data, &result)
				assert.NoError(t, err)
				assert.Equal(t, "ontap-nas", result["storageDriverName"])
			},
		},
		{
			name:     "nonexistent file",
			filename: "nonexistent-file.yaml",
			wantErr:  true,
		},
		{
			name:       "invalid base64",
			base64Data: "invalid-base64!!!",
			wantErr:    true,
		},
		{
			name: "invalid YAML",
			setupFile: func() (string, func()) {
				tempFile, err := os.Create("backend-*.yaml")
				require.NoError(t, err)
				invalidYAML := `{this is not valid yaml or json`
				_, err = tempFile.WriteString(invalidYAML)
				require.NoError(t, err)
				tempFile.Close()
				return tempFile.Name(), func() { os.Remove(tempFile.Name()) }
			},
			wantErr: true,
		},
		{
			name:       "base64 priority over file",
			base64Data: base64.StdEncoding.EncodeToString([]byte(`{"version": 1, "storageDriverName": "base64-driver"}`)),
			setupFile: func() (string, func()) {
				tempFile, err := os.Create("backend-*.yaml")
				require.NoError(t, err)
				fileData := `version: 2
storageDriverName: file-driver`
				_, err = tempFile.WriteString(fileData)
				require.NoError(t, err)
				tempFile.Close()
				return tempFile.Name(), func() { os.Remove(tempFile.Name()) }
			},
			wantErr: false,
			validate: func(t *testing.T, data []byte) {
				var result map[string]interface{}
				err := json.Unmarshal(data, &result)
				assert.NoError(t, err)
				assert.Equal(t, "base64-driver", result["storageDriverName"])
				assert.Equal(t, float64(1), result["version"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var filename string
			var cleanup func()

			if tc.setupFile != nil {
				filename, cleanup = tc.setupFile()
				defer cleanup()
			} else {
				filename = tc.filename
			}

			jsonData, err := getBackendData(filename, tc.base64Data)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, jsonData)
				if tc.validate != nil {
					tc.validate(t, jsonData)
				}
			}
		})
	}
}

func TestGetBackendData_FromStdin(t *testing.T) {
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdin = r

	defer func() {
		os.Stdin = origStdin
		r.Close()
	}()

	yamlData := `version: 1
storageDriverName: ontap-nas`
	go func() {
		defer w.Close()
		w.WriteString(yamlData)
	}()

	jsonData, err := getBackendData("-", "")
	assert.NoError(t, err)
	assert.NotNil(t, jsonData)

	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	assert.NoError(t, err)
	assert.Equal(t, "ontap-nas", result["storageDriverName"])
}

func TestCreateBackendCmd_RunE(t *testing.T) {
	originalMode := OperatingMode
	defer func() {
		OperatingMode = originalMode
	}()

	// Mock execKubernetesCLIRaw for tunnel mode tests
	prevExecKubernetesCLIRaw := execKubernetesCLIRaw
	defer func() {
		execKubernetesCLIRaw = prevExecKubernetesCLIRaw
	}()

	backend := getFakeBackend()

	testCases := []struct {
		name          string
		operatingMode string
		base64Data    string
		expectError   bool
		setupMocks    func()
		setupCommand  func()
	}{
		{
			name:          "tunnel mode success",
			operatingMode: ModeTunnel,
			base64Data:    base64.StdEncoding.EncodeToString([]byte(`{"version": 1, "storageDriverName": "ontap-nas", "backendName": "test-backend"}`)),
			expectError:   false,
			setupMocks: func() {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					return exec.Command("echo", "success")
				}
			},
		},
		{
			name:          "tunnel mode error",
			operatingMode: ModeTunnel,
			base64Data:    base64.StdEncoding.EncodeToString([]byte(`{"version": 1, "storageDriverName": "ontap-nas", "backendName": "test-backend"}`)),
			expectError:   true,
			setupMocks: func() {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					return exec.Command("false") // Command that returns exit code 1
				}
			},
		},
		{
			name:          "direct mode success",
			operatingMode: ModeDirect,
			base64Data:    base64.StdEncoding.EncodeToString([]byte(`{"version": 1, "storageDriverName": "ontap-nas", "backendName": "test-backend"}`)),
			expectError:   false,
			setupCommand: func() {
				httpmock.Activate()
			},
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/backend",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(201, rest.AddBackendResponse{
							BackendID: backend.Name,
							Error:     "",
						})
					})
				httpmock.RegisterResponder("GET", BaseURL()+"/backend/"+backend.Name,
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(200, api.GetBackendResponse{
							Backend: backend,
							Error:   "",
						})
					})
			},
		},
		{
			name:          "direct mode no input error",
			operatingMode: ModeDirect,
			base64Data:    "",
			expectError:   true,
			setupCommand: func() {
				httpmock.Activate()
			},
		},
		{
			name:          "direct mode creation failed",
			operatingMode: ModeDirect,
			base64Data:    base64.StdEncoding.EncodeToString([]byte(`{"version": 1, "storageDriverName": "ontap-nas", "backendName": "test-backend"}`)),
			expectError:   true,
			setupCommand: func() {
				httpmock.Activate()
			},
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/backend",
					httpmock.NewStringResponder(400, `{"error": "backend already exists"}`))
			},
		},
		{
			name:          "direct mode invalid base64",
			operatingMode: ModeDirect,
			base64Data:    "invalid-base64!!!",
			expectError:   true,
			setupCommand: func() {
				httpmock.Activate()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.operatingMode == ModeDirect {
				httpmock.DeactivateAndReset()
			}

			OperatingMode = tc.operatingMode

			if tc.setupCommand != nil {
				tc.setupCommand()
			}

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			// Set up test data
			prevCreateFilename := createFilename
			prevCreateBase64Data := createBase64Data
			defer func() {
				createFilename = prevCreateFilename
				createBase64Data = prevCreateBase64Data
			}()

			createFilename = ""
			createBase64Data = tc.base64Data

			origOut := os.Stdout
			os.Stdout, _ = os.Open(os.DevNull)
			defer func() { os.Stdout = origOut }()

			cmd := &cobra.Command{}
			err := createBackendCmd.RunE(cmd, []string{})

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tc.operatingMode == ModeDirect {
				httpmock.DeactivateAndReset()
			}
		})
	}
}

func TestBackendCreate(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	backend := getFakeBackend()

	testCases := []struct {
		name                  string
		wantErr               bool
		urlCreateBackend      string
		urlGetBackend         string
		createBackendResponse httpmock.Responder
		getBackendResponse    httpmock.Responder
		matchOutput           bool
	}{
		{
			name:             "backend created successfully",
			wantErr:          false,
			urlCreateBackend: BaseURL() + "/backend",
			urlGetBackend:    BaseURL() + "/backend/" + backend.Name,
			createBackendResponse: func(req *http.Request) (*http.Response, error) {
				resp, err := httpmock.NewJsonResponse(201, rest.AddBackendResponse{
					BackendID: backend.Name,
					Error:     "",
				})
				if err != nil {
					return httpmock.NewStringResponse(500, ""), nil
				}
				return resp, nil
			},
			getBackendResponse: func(req *http.Request) (*http.Response, error) {
				resp, err := httpmock.NewJsonResponse(200, api.GetBackendResponse{
					Backend: backend,
					Error:   "",
				})
				if err != nil {
					return httpmock.NewStringResponse(500, ""), nil
				}
				return resp, nil
			},
			matchOutput: true,
		},
		{
			name:             "backend creation failed",
			wantErr:          true,
			urlCreateBackend: BaseURL() + "/backend",
			createBackendResponse: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(400, `{"error": "backend already exists"}`), nil
			},
			matchOutput: false,
		},
		{
			name:             "network error during creation",
			wantErr:          true,
			urlCreateBackend: BaseURL() + "/backend",
			createBackendResponse: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("network error")
			},
			matchOutput: false,
		},
		{
			name:             "invalid JSON response",
			wantErr:          true,
			urlCreateBackend: BaseURL() + "/backend",
			createBackendResponse: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(201, "invalid json"), nil
			},
			matchOutput: false,
		},
		{
			name:             "get backend failed after creation",
			wantErr:          true,
			urlCreateBackend: BaseURL() + "/backend",
			urlGetBackend:    BaseURL() + "/backend/" + backend.Name,
			createBackendResponse: func(req *http.Request) (*http.Response, error) {
				resp, err := httpmock.NewJsonResponse(201, rest.AddBackendResponse{
					BackendID: backend.Name,
					Error:     "",
				})
				if err != nil {
					return httpmock.NewStringResponse(500, ""), nil
				}
				return resp, nil
			},
			getBackendResponse: func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(404, `{"error": "backend not found"}`), nil
			},
			matchOutput: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cleanup := setupHTTPMock(t)
			defer cleanup()

			expectedBackends := []storage.BackendExternal{backend}

			expectedOutput := captureOutput(t, func() {
				WriteBackends(expectedBackends)
			})

			httpmock.RegisterResponder("POST", tc.urlCreateBackend, tc.createBackendResponse)
			if tc.getBackendResponse != nil {
				httpmock.RegisterResponder("GET", tc.urlGetBackend, tc.getBackendResponse)
			}

			var err error
			actualOutput := captureOutput(t, func() {
				postData := []byte(`{"version": 1, "storageDriverName": "ontap-nas", "backendName": "test-backend"}`)
				err = backendCreate(postData)
			})

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tc.matchOutput {
				assert.Equal(t, expectedOutput, actualOutput, "Expected output to match")
			}

			httpmock.Reset()
		})
	}
}
