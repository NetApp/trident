// Copyright 2022 NetApp, Inc. All Rights Reserved.

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

func setupHTTPMockVolume(t *testing.T) func() {
	httpmock.Activate()
	return func() {
		httpmock.DeactivateAndReset()
	}
}

// Thread-safe helper for managing global state in tests
var volumeTestStateMutex sync.RWMutex

func withVolumeTestMode(operatingMode string, allFlag bool, mockCommand func() *exec.Cmd, callback func()) {
	volumeTestStateMutex.Lock()
	defer volumeTestStateMutex.Unlock()

	// Store original values
	prevOperatingMode := OperatingMode
	prevAllVolumes := allVolumes
	prevTridentPodName := TridentPodName
	prevTridentPodNamespace := TridentPodNamespace
	prevExecKubernetesCLIRaw := execKubernetesCLIRaw

	defer func() {
		OperatingMode = prevOperatingMode
		allVolumes = prevAllVolumes
		TridentPodName = prevTridentPodName
		TridentPodNamespace = prevTridentPodNamespace
		execKubernetesCLIRaw = prevExecKubernetesCLIRaw
	}()

	OperatingMode = operatingMode
	allVolumes = allFlag
	if operatingMode == ModeTunnel {
		TridentPodName = "trident-controller-test"
		TridentPodNamespace = "trident"
		if mockCommand != nil {
			execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
				return mockCommand()
			}
		} else {
			execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
				return exec.Command("echo", "Volume deleted successfully")
			}
		}
	}

	callback()
}

func TestDeleteVolumeCmd_RunE(t *testing.T) {
	cleanup := setupHTTPMockVolume(t)
	defer cleanup()

	testCases := []struct {
		name          string
		operatingMode string
		args          []string
		allFlag       bool
		wantErr       bool
		setupMocks    func()
		mockCommand   func() *exec.Cmd
	}{
		{
			name:          "tunnel mode success",
			operatingMode: ModeTunnel,
			args:          []string{"vol1"},
			allFlag:       false,
			wantErr:       false,
			setupMocks:    func() {},
		},
		{
			name:          "tunnel mode with all flag",
			operatingMode: ModeTunnel,
			args:          []string{},
			allFlag:       true,
			wantErr:       false,
			setupMocks:    func() {},
		},
		{
			name:          "tunnel mode error",
			operatingMode: ModeTunnel,
			args:          []string{"vol1"},
			allFlag:       false,
			wantErr:       true,
			setupMocks:    func() {},
			mockCommand: func() *exec.Cmd {
				return exec.Command("false") // Command that fails
			},
		},
		{
			name:          "direct mode success",
			operatingMode: ModeDirect,
			args:          []string{"vol1"},
			allFlag:       false,
			wantErr:       false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/volume/vol1",
					httpmock.NewStringResponder(200, ""))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			tc.setupMocks()

			withVolumeTestMode(tc.operatingMode, tc.allFlag, tc.mockCommand, func() {
				cmd := &cobra.Command{}
				err := deleteVolumeCmd.RunE(cmd, tc.args)

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestVolumeDelete(t *testing.T) {
	cleanup := setupHTTPMockVolume(t)
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
			name:    "delete specific volume",
			args:    []string{"vol1"},
			allFlag: false,
			wantErr: false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/volume/vol1",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:    "delete all volumes",
			args:    []string{},
			allFlag: true,
			wantErr: false,
			setupMocks: func() {
				response := rest.ListVolumesResponse{
					Volumes: []string{"vol1", "vol2"},
				}
				responder, _ := httpmock.NewJsonResponder(200, response)
				httpmock.RegisterResponder("GET", BaseURL()+"/volume", responder)

				httpmock.RegisterResponder("DELETE", BaseURL()+"/volume/vol1",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/volume/vol2",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:          "all flag with specific volumes error",
			args:          []string{"vol1"},
			allFlag:       true,
			wantErr:       true,
			errorContains: "cannot use --all switch and specify individual volumes",
			setupMocks:    func() {},
		},
		{
			name:          "no volumes specified without all flag",
			args:          []string{},
			allFlag:       false,
			wantErr:       true,
			errorContains: "volume name not specified",
			setupMocks:    func() {},
		},
		{
			name:          "get volumes list failed",
			args:          []string{},
			allFlag:       true,
			wantErr:       true,
			errorContains: "could not get volumes",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/volume",
					httpmock.NewStringResponder(500, `{"error": "server error"}`))
			},
		},
		{
			name:          "delete volume HTTP error",
			args:          []string{"vol1"},
			allFlag:       false,
			wantErr:       true,
			errorContains: "network error",
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/volume/vol1",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:          "delete volume not found",
			args:          []string{"vol1"},
			allFlag:       false,
			wantErr:       true,
			errorContains: "could not delete volume",
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/volume/vol1",
					httpmock.NewStringResponder(404, `{"error": "volume not found"}`))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			tc.setupMocks()

			withVolumeTestMode(ModeDirect, tc.allFlag, nil, func() {
				err := volumeDelete(tc.args)

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
