// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/base64"
	"net/http"
	"os/exec"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

// Test updateBackendCmd RunE function
func TestUpdateBackendCmd_RunE(t *testing.T) {
	testCases := []struct {
		name          string
		operatingMode string
		args          []string
		setupFile     func() (string, func())
		base64Data    string
		wantErr       bool
		setupMocks    func()
	}{
		{
			name:          "tunnel mode success",
			operatingMode: ModeTunnel,
			args:          []string{"test-backend"},
			base64Data:    base64.StdEncoding.EncodeToString([]byte(`{"version": 1, "storageDriverName": "ontap-nas"}`)),
			wantErr:       false,
			setupMocks:    func() {},
		},
		{
			name:          "direct mode success",
			operatingMode: ModeDirect,
			args:          []string{"test-backend"},
			base64Data:    base64.StdEncoding.EncodeToString([]byte(`{"version": 1, "storageDriverName": "ontap-nas"}`)),
			wantErr:       false,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/backend/test-backend",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewStringResponse(200, `{"backend": "test-backend"}`), nil
					})

				httpmock.RegisterResponder("GET", BaseURL()+"/backend/test-backend",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(200, map[string]interface{}{
							"backend": map[string]interface{}{
								"name":        "test-backend",
								"backendUUID": "uuid-1234",
								"state":       "online",
								"online":      true,
								"config": map[string]interface{}{
									"storageDriverName": "ontap-nas",
								},
							},
						})
					})
			},
		},
		{
			name:          "no input error",
			operatingMode: ModeDirect,
			args:          []string{"test-backend"},
			wantErr:       true,
			setupMocks:    func() {},
		},
		{
			name:          "invalid base64 error",
			operatingMode: ModeDirect,
			args:          []string{"test-backend"},
			base64Data:    "invalid-base64!!!",
			wantErr:       true,
			setupMocks:    func() {},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			prevOperatingMode := OperatingMode
			prevUpdateFilename := updateFilename
			prevUpdateBase64Data := updateBase64Data
			prevServer := Server
			prevExecKubernetesCLIRaw := execKubernetesCLIRaw
			defer func() {
				OperatingMode = prevOperatingMode
				updateFilename = prevUpdateFilename
				updateBase64Data = prevUpdateBase64Data
				Server = prevServer
				execKubernetesCLIRaw = prevExecKubernetesCLIRaw
			}()

			OperatingMode = tc.operatingMode
			Server = "localhost:8000"

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			if tc.operatingMode == ModeTunnel {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					return exec.Command("echo", "success")
				}
			}

			updateFilename = ""
			updateBase64Data = tc.base64Data

			cmd := &cobra.Command{}
			err := updateBackendCmd.RunE(cmd, tc.args)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBackendUpdate(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name         string
		backendNames []string
		wantErr      bool
		setupMocks   func()
	}{
		{
			name:         "no backend name",
			backendNames: []string{},
			wantErr:      true,
		},
		{
			name:         "multiple backend names",
			backendNames: []string{"backend1", "backend2"},
			wantErr:      true,
		},
		{
			name:         "successful update",
			backendNames: []string{"test-backend"},
			wantErr:      false,
			setupMocks: func() {
				httpmock.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
					if req.Method == "POST" {
						return httpmock.NewStringResponse(200, `{"backendID": "test-backend"}`), nil
					}
					return httpmock.NewStringResponse(200, `{"backend": {"name": "test-backend"}}`), nil
				})
			},
		},
		{
			name:         "update failed",
			backendNames: []string{"test-backend"},
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
					return httpmock.NewStringResponse(400, `{"error": "bad request"}`), nil
				})
			},
		},
		{
			name:         "network error",
			backendNames: []string{"test-backend"},
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
					return nil, errors.New("network error")
				})
			},
		},
		{
			name:         "invalid JSON",
			backendNames: []string{"test-backend"},
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
					return httpmock.NewStringResponse(200, "invalid json"), nil
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

			postData := []byte(`{"version": 1}`)
			err := backendUpdate(tc.backendNames, postData)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUpdateBackendCmd_Base64(t *testing.T) {
	prevOperatingMode := OperatingMode
	prevUpdateFilename := updateFilename
	prevUpdateBase64Data := updateBase64Data
	defer func() {
		OperatingMode = prevOperatingMode
		updateFilename = prevUpdateFilename
		updateBase64Data = prevUpdateBase64Data
	}()

	OperatingMode = ModeDirect
	updateFilename = ""
	updateBase64Data = base64.StdEncoding.EncodeToString([]byte(`{"version": 1}`))

	cmd := &cobra.Command{}
	err := updateBackendCmd.RunE(cmd, []string{"test-backend"})
	assert.Error(t, err)
}
