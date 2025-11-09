package cmd

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
)

func TestGetPVCData(t *testing.T) {
	testCases := []struct {
		name      string
		filename  string
		b64Data   string
		wantErr   bool
		setupFile func() (string, func())
	}{
		{
			name:     "no input provided",
			filename: "",
			b64Data:  "",
			wantErr:  true,
		},
		{
			name:    "valid base64 JSON",
			b64Data: base64.StdEncoding.EncodeToString([]byte(`{"test": "data"}`)),
			wantErr: false,
		},
		{
			name:    "valid base64 YAML",
			b64Data: base64.StdEncoding.EncodeToString([]byte(`test: data`)),
			wantErr: false,
		},
		{
			name:    "invalid base64",
			b64Data: "invalid-base64!@#",
			wantErr: true,
		},
		{
			name:    "invalid YAML/JSON in base64",
			b64Data: base64.StdEncoding.EncodeToString([]byte(`invalid: yaml: content:`)),
			wantErr: true,
		},
		{
			name:     "valid file JSON",
			filename: "test.json",
			wantErr:  false,
			setupFile: func() (string, func()) {
				file, err := os.CreateTemp("", "test*.json")
				if err != nil {
					panic(err)
				}
				file.WriteString(`{"test": "file"}`)
				file.Close()
				return file.Name(), func() { os.Remove(file.Name()) }
			},
		},
		{
			name:     "valid file YAML",
			filename: "test.yaml",
			wantErr:  false,
			setupFile: func() (string, func()) {
				file, err := os.CreateTemp("", "test*.yaml")
				if err != nil {
					panic(err)
				}
				file.WriteString(`test: file`)
				file.Close()
				return file.Name(), func() { os.Remove(file.Name()) }
			},
		},
		{
			name:     "file not found",
			filename: "nonexistent.json",
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filename := tc.filename
			var cleanup func()

			if tc.setupFile != nil {
				filename, cleanup = tc.setupFile()
				defer cleanup()
			}

			data, err := getPVCData(filename, tc.b64Data)

			if tc.wantErr {
				assert.Error(t, err)
				assert.Nil(t, data)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, data)
				var jsonObj interface{}
				err = json.Unmarshal(data, &jsonObj)
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetPVCDataStdin(t *testing.T) {
	originalStdin := os.Stdin

	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	defer w.Close()

	os.Stdin = r

	go func() {
		defer w.Close()
		w.WriteString(`{"stdin": "test"}`)
	}()

	data, err := getPVCData("-", "")

	os.Stdin = originalStdin

	assert.NoError(t, err)
	assert.NotNil(t, data)

	var jsonObj map[string]interface{}
	err = json.Unmarshal(data, &jsonObj)
	assert.NoError(t, err)
	assert.Equal(t, "test", jsonObj["stdin"])
}

func TestVolumeImport(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name               string
		backendName        string
		internalVolumeName string
		noManage           bool
		noRename           bool
		pvcDataJSON        []byte
		wantErr            bool
		setupMocks         func()
	}{
		{
			name:               "successful import",
			backendName:        "test-backend",
			internalVolumeName: "test-volume",
			noManage:           false,
			noRename:           false,
			pvcDataJSON:        []byte(`{"test": "data"}`),
			wantErr:            false,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/volume/import",
					func(req *http.Request) (*http.Response, error) {
						body, _ := io.ReadAll(req.Body)
						var importReq storage.ImportVolumeRequest
						err := json.Unmarshal(body, &importReq)
						if err != nil {
							return httpmock.NewStringResponse(400, ""), nil
						}

						volume := storage.VolumeExternal{
							Config: &storage.VolumeConfig{
								Name: "test-volume",
							},
						}
						response := rest.ImportVolumeResponse{
							Volume: &volume,
						}
						responseJSON, _ := json.Marshal(response)
						return httpmock.NewBytesResponse(http.StatusCreated, responseJSON), nil
					})
			},
		},
		{
			name:               "marshal request error",
			backendName:        "test-backend",
			internalVolumeName: "test-volume",
			noManage:           false,
			noRename:           false,
			pvcDataJSON:        nil,
			wantErr:            false,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/volume/import",
					httpmock.NewStringResponder(http.StatusCreated,
						`{"volume": {"config": {"name": "test"}}}`))
			},
		},
		{
			name:               "API error",
			backendName:        "test-backend",
			internalVolumeName: "test-volume",
			noManage:           false,
			noRename:           false,
			pvcDataJSON:        []byte(`{"test": "data"}`),
			wantErr:            true,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/volume/import",
					func(req *http.Request) (*http.Response, error) {
						return nil, errors.New("network error")
					})
			},
		},
		{
			name:               "HTTP error status",
			backendName:        "test-backend",
			internalVolumeName: "test-volume",
			noManage:           true,
			noRename:           false,
			pvcDataJSON:        []byte(`{"test": "data"}`),
			wantErr:            true,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/volume/import",
					httpmock.NewStringResponder(http.StatusBadRequest, `{"error": "bad request"}`))
			},
		},
		{
			name:               "invalid response JSON",
			backendName:        "test-backend",
			internalVolumeName: "test-volume",
			noManage:           false,
			noRename:           false,
			pvcDataJSON:        []byte(`{"test": "data"}`),
			wantErr:            true,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/volume/import",
					httpmock.NewStringResponder(http.StatusCreated, "invalid json"))
			},
		},
		{
			name:               "nil volume in response",
			backendName:        "test-backend",
			internalVolumeName: "test-volume",
			noManage:           false,
			noRename:           false,
			pvcDataJSON:        []byte(`{"test": "data"}`),
			wantErr:            true,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/volume/import",
					httpmock.NewStringResponder(http.StatusCreated, `{"volume": null}`))
			},
		},
		{
			name:               "missing volume in response",
			backendName:        "test-backend",
			internalVolumeName: "test-volume",
			noManage:           false,
			noRename:           false,
			pvcDataJSON:        []byte(`{"test": "data"}`),
			wantErr:            true,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/volume/import",
					httpmock.NewStringResponder(http.StatusCreated, `{}`))
			},
		},
		{
			name:               "successful import with no-rename",
			backendName:        "test-backend",
			internalVolumeName: "test-volume",
			noManage:           false,
			noRename:           true,
			pvcDataJSON:        []byte(`{"test": "data"}`),
			wantErr:            false,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/volume/import",
					func(req *http.Request) (*http.Response, error) {
						body, _ := io.ReadAll(req.Body)
						var importReq storage.ImportVolumeRequest
						err := json.Unmarshal(body, &importReq)
						if err != nil {
							return httpmock.NewStringResponse(400, ""), nil
						}

						// Verify that NoRename flag is set correctly
						if !importReq.NoRename {
							return httpmock.NewStringResponse(400, "NoRename flag not set"), nil
						}

						volume := storage.VolumeExternal{
							Config: &storage.VolumeConfig{
								Name: "test-volume",
							},
						}
						response := rest.ImportVolumeResponse{
							Volume: &volume,
						}
						responseJSON, _ := json.Marshal(response)
						return httpmock.NewBytesResponse(http.StatusCreated, responseJSON), nil
					})
			},
		},
		{
			name:               "no-manage and no-rename both true - should fail validation",
			backendName:        "test-backend",
			internalVolumeName: "test-volume",
			noManage:           true,
			noRename:           true,
			pvcDataJSON:        []byte(`{"test": "data"}`),
			wantErr:            true,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/volume/import",
					func(req *http.Request) (*http.Response, error) {
						body, _ := io.ReadAll(req.Body)
						var importReq storage.ImportVolumeRequest
						err := json.Unmarshal(body, &importReq)
						if err != nil {
							return httpmock.NewStringResponse(400, ""), nil
						}

						// Validate the request - should fail when both flags are true
						if err := importReq.Validate(); err != nil {
							errorResponse := map[string]string{
								"error": err.Error(),
							}
							errorJSON, _ := json.Marshal(errorResponse)
							return httpmock.NewBytesResponse(http.StatusBadRequest, errorJSON), nil
						}

						// Should never reach here
						return httpmock.NewStringResponse(500, "validation should have failed"), nil
					})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			err := volumeImport(tc.backendName, tc.internalVolumeName, tc.noManage, tc.noRename, tc.pvcDataJSON)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestImportVolumeCmd_RunE(t *testing.T) {
	testCases := []struct {
		name          string
		operatingMode string
		args          []string
		setupFile     func() (string, func())
		base64Data    string
		noManage      bool
		noRename      bool
		wantErr       bool
	}{
		{
			name:          "no input error",
			operatingMode: ModeDirect,
			args:          []string{"test-backend", "test-volume"},
			noManage:      false,
			noRename:      false,
			wantErr:       true,
		},
		{
			name:          "invalid base64 error",
			operatingMode: ModeDirect,
			args:          []string{"test-backend", "test-volume"},
			base64Data:    "invalid-base64!!!",
			noManage:      false,
			noRename:      false,
			wantErr:       true,
		},
		{
			name:          "valid base64 data - tunnel mode",
			operatingMode: ModeTunnel,
			args:          []string{"test-backend", "test-volume"},
			base64Data:    base64.StdEncoding.EncodeToString([]byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim"}`)),
			noManage:      false,
			noRename:      false,
			wantErr:       true,
		},
		{
			name:          "valid base64 data - direct mode",
			operatingMode: ModeDirect,
			args:          []string{"test-backend", "test-volume"},
			base64Data:    base64.StdEncoding.EncodeToString([]byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim"}`)),
			noManage:      false,
			noRename:      false,
			wantErr:       true,
		},
		{
			name:          "file not found error",
			operatingMode: ModeDirect,
			args:          []string{"test-backend", "test-volume"},
			setupFile: func() (string, func()) {
				return "nonexistent-file.yaml", func() {}
			},
			noManage: false,
			noRename: false,
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prevOperatingMode := OperatingMode
			prevImportFilename := importFilename
			prevImportBase64Data := importBase64Data
			prevImportNoManage := importNoManage
			prevImportNoRename := importNoRename
			defer func() {
				OperatingMode = prevOperatingMode
				importFilename = prevImportFilename
				importBase64Data = prevImportBase64Data
				importNoManage = prevImportNoManage
				importNoRename = prevImportNoRename
			}()

			// Set test values
			OperatingMode = tc.operatingMode
			importNoManage = tc.noManage
			importNoRename = tc.noRename

			var filename string
			var cleanup func()

			if tc.setupFile != nil {
				filename, cleanup = tc.setupFile()
				defer cleanup()
			}

			importFilename = filename
			importBase64Data = tc.base64Data

			// Execute command
			cmd := &cobra.Command{}
			err := importVolumeCmd.RunE(cmd, tc.args)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
