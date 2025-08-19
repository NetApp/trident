// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"net/http"
	"os/exec"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestValidateUpdateBackendStateArguments(t *testing.T) {
	testCases := []struct {
		name          string
		stateFlag     bool
		userStateFlag bool
		wantErr       bool
		errorContains string
	}{
		{
			name:          "no flags set",
			stateFlag:     false,
			userStateFlag: false,
			wantErr:       true,
			errorContains: "exactly one of --state or --user-state must be specified",
		},
		{
			name:          "state flag set",
			stateFlag:     true,
			userStateFlag: false,
			wantErr:       false,
		},
		{
			name:          "user-state flag set",
			stateFlag:     false,
			userStateFlag: true,
			wantErr:       false,
		},
		{
			name:          "both flags set",
			stateFlag:     true,
			userStateFlag: true,
			wantErr:       true,
			errorContains: "exactly one of --state or --user-state must be specified",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().StringVarP(&backendState, "state", "", "", "New backend state")
			cmd.Flags().StringVarP(&userState, "user-state", "", "", "User-defined backend state")

			if tc.stateFlag {
				cmd.Flags().Set("state", "online")
			}
			if tc.userStateFlag {
				cmd.Flags().Set("user-state", "normal")
			}

			err := validateUpdateBackendStateArguments(cmd, []string{})

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

// Test updateBackendStateRunE function
func TestUpdateBackendStateRunE(t *testing.T) {
	testCases := []struct {
		name          string
		operatingMode string
		args          []string
		stateFlag     string
		userStateFlag string
		wantErr       bool
		setupMocks    func()
	}{
		{
			name:          "tunnel mode success",
			operatingMode: ModeTunnel,
			args:          []string{"test-backend"},
			stateFlag:     "online",
			wantErr:       false,
			setupMocks:    func() {}, // No HTTP mocks needed for tunnel mode
		},
		{
			name:          "tunnel mode success with user-state flag",
			operatingMode: ModeTunnel,
			args:          []string{"test-backend"},
			userStateFlag: "normal",
			wantErr:       false,
			setupMocks:    func() {},
		},
		{
			name:          "direct mode success",
			operatingMode: ModeDirect,
			args:          []string{"test-backend"},
			userStateFlag: "normal",
			wantErr:       false,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/backend/test-backend/state",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(200, map[string]interface{}{
							"backend": "test-backend",
						})
					})

				// Mock the GET request that retrieves the backend after update
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			prevOperatingMode := OperatingMode
			prevBackendState := backendState
			prevUserState := userState
			prevServer := Server
			prevExecKubernetesCLIRaw := execKubernetesCLIRaw
			defer func() {
				OperatingMode = prevOperatingMode
				backendState = prevBackendState
				userState = prevUserState
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

			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().StringVarP(&backendState, "state", "", "", "New backend state")
			cmd.Flags().StringVarP(&userState, "user-state", "", "", "User-defined backend state")

			if tc.stateFlag != "" {
				cmd.Flags().Set("state", tc.stateFlag)
				backendState = tc.stateFlag
			}
			if tc.userStateFlag != "" {
				cmd.Flags().Set("user-state", tc.userStateFlag)
				userState = tc.userStateFlag
			}

			err := validateUpdateBackendStateArguments(cmd, tc.args)
			assert.NoError(t, err)

			err = updateBackendStateRunE(cmd, tc.args)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBackendUpdateState(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name         string
		backendNames []string
		wantErr      bool
		setupMocks   func()
	}{
		{
			name:         "no backend name specified",
			backendNames: []string{},
			wantErr:      true,
		},
		{
			name:         "multiple backend names specified",
			backendNames: []string{"backend1", "backend2"},
			wantErr:      true,
		},
		{
			name:         "successful update",
			backendNames: []string{"test-backend"},
			wantErr:      false,
			setupMocks: func() {
				// Mock the POST request for state update - handle multiple possible URLs
				httpmock.RegisterResponder("POST", BaseURL()+"/backend/test-backend/state",
					httpmock.NewStringResponder(200, `{"backendID": "test-backend"}`))

				httpmock.RegisterResponder("GET", BaseURL()+"/backend/test-backend",
					httpmock.NewStringResponder(200, `{"backend": {"name": "test-backend"}}`))

				httpmock.RegisterNoResponder(func(req *http.Request) (*http.Response, error) {
					return httpmock.NewStringResponse(200, `{"backend": {"name": "test-backend"}}`), nil
				})
			},
		},
		{
			name:         "update failed",
			backendNames: []string{"test-backend"},
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/backend/test-backend/state",
					httpmock.NewStringResponder(400, `{"error": "bad request"}`))
			},
		},
		{
			name:         "network error",
			backendNames: []string{"test-backend"},
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/backend/test-backend/state",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:         "invalid JSON response",
			backendNames: []string{"test-backend"},
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterResponder("POST", BaseURL()+"/backend/test-backend/state",
					httpmock.NewStringResponder(200, "invalid json"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			err := backendUpdateState(tc.backendNames)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
