// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"os/exec"
	"sync"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/utils/errors"
)

func setupHTTPMockStorageClass(t *testing.T) func() {
	httpmock.Activate()
	return func() {
		httpmock.DeactivateAndReset()
	}
}

var storageClassTestStateMutex sync.RWMutex

func withStorageClassTestMode(operatingMode string, allFlag bool, callback func()) {
	storageClassTestStateMutex.Lock()
	defer storageClassTestStateMutex.Unlock()

	prevOperatingMode := OperatingMode
	prevAllStorageClasses := allStorageClasses
	prevExecKubernetesCLIRaw := execKubernetesCLIRaw

	OperatingMode = operatingMode
	allStorageClasses = allFlag
	if operatingMode == ModeTunnel {
		execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
			return exec.Command("echo", "Storage class deleted successfully")
		}
	}
	defer func() {
		OperatingMode = prevOperatingMode
		allStorageClasses = prevAllStorageClasses
		execKubernetesCLIRaw = prevExecKubernetesCLIRaw
	}()

	callback()
}

func TestDeleteStorageClassCmd_RunE(t *testing.T) {
	cleanup := setupHTTPMockStorageClass(t)
	defer cleanup()

	testCases := []struct {
		name          string
		operatingMode string
		args          []string
		setupMocks    func()
	}{
		{
			name:          "tunnel mode success",
			operatingMode: ModeTunnel,
			args:          []string{"sc1"},
			setupMocks:    func() {},
		},
		{
			name:          "direct mode success",
			operatingMode: ModeDirect,
			args:          []string{"sc1"},
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/storageclass/sc1",
					httpmock.NewStringResponder(200, ""))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			tc.setupMocks()

			withStorageClassTestMode(tc.operatingMode, false, func() {
				cmd := &cobra.Command{}
				err := deleteStorageClassCmd.RunE(cmd, tc.args)
				assert.NoError(t, err)
			})
		})
	}
}

func TestStorageClassDelete(t *testing.T) {
	cleanup := setupHTTPMockStorageClass(t)
	defer cleanup()

	testCases := []struct {
		name          string
		args          []string
		allFlag       bool
		wantErr       bool
		errorContains string
		setupMocks    func()
	}{
		{
			name:    "delete specific storage class",
			args:    []string{"sc1"},
			allFlag: false,
			wantErr: false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/storageclass/sc1",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:    "delete all storage classes",
			args:    []string{},
			allFlag: true,
			wantErr: false,
			setupMocks: func() {
				response := rest.ListStorageClassesResponse{
					StorageClasses: []string{"sc1", "sc2"},
				}
				responder, _ := httpmock.NewJsonResponder(200, response)
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass", responder)

				httpmock.RegisterResponder("DELETE", BaseURL()+"/storageclass/sc1",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/storageclass/sc2",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:          "all flag with specific storage classes error",
			args:          []string{"sc1"},
			allFlag:       true,
			wantErr:       true,
			errorContains: "cannot use --all switch and specify individual storage classes",
			setupMocks:    func() {},
		},
		{
			name:          "no storage classes specified without all flag",
			args:          []string{},
			allFlag:       false,
			wantErr:       true,
			errorContains: "storage class name not specified",
			setupMocks:    func() {},
		},
		{
			name:          "get storage classes list failed",
			args:          []string{},
			allFlag:       true,
			wantErr:       true,
			errorContains: "could not get storage classes",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass",
					httpmock.NewStringResponder(500, `{"error": "server error"}`))
			},
		},
		{
			name:          "delete storage class HTTP error",
			args:          []string{"sc1"},
			allFlag:       false,
			wantErr:       true,
			errorContains: "network error",
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/storageclass/sc1",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:          "delete storage class not found",
			args:          []string{"sc1"},
			allFlag:       false,
			wantErr:       true,
			errorContains: "could not delete storage class",
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/storageclass/sc1",
					httpmock.NewStringResponder(404, `{"error": "storage class not found"}`))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			tc.setupMocks()

			withStorageClassTestMode(ModeDirect, tc.allFlag, func() {
				err := storageClassDelete(tc.args)

				if tc.wantErr {
					assert.Error(t, err)
					if tc.errorContains != "" {
						assert.Contains(t, err.Error(), tc.errorContains)
					}
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestDeleteStorageClassCmd_TunnelModeAllFlag(t *testing.T) {
	cleanup := setupHTTPMockStorageClass(t)
	defer cleanup()

	withStorageClassTestMode(ModeTunnel, true, func() {
		cmd := &cobra.Command{}
		err := deleteStorageClassCmd.RunE(cmd, []string{"sc1"})
		assert.NoError(t, err, "Tunnel mode should pass through arguments without validation")
	})
}
