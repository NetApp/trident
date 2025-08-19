// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"errors"
	"os/exec"
	"sync"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/frontend/rest"
)

var snapshotTestMutex sync.RWMutex

func withSnapshotTestMode(t *testing.T, mode string, fn func()) {
	t.Helper()

	snapshotTestMutex.Lock()
	defer snapshotTestMutex.Unlock()

	origMode := OperatingMode
	origExecKubernetesCLIRaw := execKubernetesCLIRaw

	defer func() {
		OperatingMode = origMode
		execKubernetesCLIRaw = origExecKubernetesCLIRaw
	}()

	OperatingMode = mode

	fn()
}

func TestDeleteSnapshotCmd_RunE(t *testing.T) {
	cleanup := setupHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name          string
		operatingMode string
		args          []string
		flagValues    map[string]string
		wantErr       bool
		setupMocks    func()
	}{
		{
			name:          "tunnel mode success",
			operatingMode: ModeTunnel,
			args:          []string{"vol1/snap1"},
			flagValues:    map[string]string{},
			wantErr:       false,
		},
		{
			name:          "direct mode success",
			operatingMode: ModeDirect,
			args:          []string{"vol1/snap1"},
			flagValues:    map[string]string{},
			wantErr:       false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/snapshot/vol1/snap1",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:          "tunnel mode flags conflict",
			operatingMode: ModeTunnel,
			args:          []string{},
			flagValues:    map[string]string{"volume": "vol1", "group": "group1"},
			wantErr:       true,
		},
		{
			name:          "tunnel mode volume and all conflict",
			operatingMode: ModeTunnel,
			args:          []string{},
			flagValues:    map[string]string{"volume": "vol1", "all": "true"},
			wantErr:       true,
		},
		{
			name:          "tunnel mode group and all conflict",
			operatingMode: ModeTunnel,
			args:          []string{},
			flagValues:    map[string]string{"group": "group1", "all": "true"},
			wantErr:       true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withSnapshotTestMode(t, tc.operatingMode, func() {
				httpmock.Reset()

				// Mock execKubernetesCLIRaw for tunnel mode
				if tc.operatingMode == ModeTunnel {
					execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
						return exec.Command("echo", "Snapshot deleted successfully")
					}
				}

				if tc.setupMocks != nil {
					tc.setupMocks()
				}

				// Create a fresh command for testing
				cmd := &cobra.Command{}

				// Add flags
				cmd.Flags().String("volume", "", "Volume name")
				cmd.Flags().String("group", "", "Group name")
				cmd.Flags().Bool("all", false, "Delete all snapshots")

				// Set flag values
				for flagName, flagValue := range tc.flagValues {
					err := cmd.Flags().Set(flagName, flagValue)
					assert.NoError(t, err)
				}

				// Execute the command
				err := deleteSnapshotCmd.RunE(cmd, tc.args)

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestSnapshotDelete(t *testing.T) {
	cleanup := setupHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name        string
		snapshotIDs []string
		flagValues  map[string]string
		wantErr     bool
		setupMocks  func()
	}{
		{
			name:        "delete specific snapshots",
			snapshotIDs: []string{"vol1/snap1", "vol2/snap2"},
			flagValues:  map[string]string{},
			wantErr:     false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/snapshot/vol1/snap1",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/snapshot/vol2/snap2",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:        "all flag with specific snapshots error",
			snapshotIDs: []string{"vol1/snap1"},
			flagValues:  map[string]string{"all": "true"},
			wantErr:     true,
		},
		{
			name:        "volume flag with specific snapshots error",
			snapshotIDs: []string{"vol1/snap1"},
			flagValues:  map[string]string{"volume": "vol1"},
			wantErr:     true,
		},
		{
			name:        "group flag with specific snapshots error",
			snapshotIDs: []string{"vol1/snap1"},
			flagValues:  map[string]string{"group": "group1"},
			wantErr:     true,
		},
		{
			name:        "invalid snapshot ID format",
			snapshotIDs: []string{"invalid_snapshot_without_slash"},
			flagValues:  map[string]string{},
			wantErr:     true,
		},
		{
			name:       "delete all snapshots success",
			flagValues: map[string]string{"all": "true"},
			wantErr:    false,
			setupMocks: func() {
				response := rest.ListSnapshotsResponse{
					Snapshots: []string{"vol1/snap1", "vol2/snap2"},
				}
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot",
					httpmock.NewJsonResponderOrPanic(200, response))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/snapshot/vol1/snap1",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/snapshot/vol2/snap2",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:       "delete by volume success",
			flagValues: map[string]string{"volume": "vol1"},
			wantErr:    false,
			setupMocks: func() {
				response := rest.ListSnapshotsResponse{
					Snapshots: []string{"vol1/snap1"},
				}
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot?volume=vol1",
					httpmock.NewJsonResponderOrPanic(200, response))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/snapshot/vol1/snap1",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:       "delete by group success",
			flagValues: map[string]string{"group": "group1"},
			wantErr:    false,
			setupMocks: func() {
				response := rest.ListSnapshotsResponse{
					Snapshots: []string{"vol1/snap1"},
				}
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot?group=group1",
					httpmock.NewJsonResponderOrPanic(200, response))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/snapshot/vol1/snap1",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:       "HTTP error during GetSnapshots",
			flagValues: map[string]string{"all": "true"},
			wantErr:    true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot",
					httpmock.NewStringResponder(500, "Internal Server Error"))
			},
		},
		{
			name:        "network error during delete",
			snapshotIDs: []string{"vol1/snap1"},
			flagValues:  map[string]string{},
			wantErr:     true,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/snapshot/vol1/snap1",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:        "multiple delete errors - some succeed some fail",
			snapshotIDs: []string{"vol1/snap1", "vol2/snap2"},
			flagValues:  map[string]string{},
			wantErr:     true,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/snapshot/vol1/snap1",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/snapshot/vol2/snap2",
					httpmock.NewStringResponder(404, "Snapshot not found"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			deleteSnapshotFlagValues = tc.flagValues

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			err := snapshotDelete(tc.snapshotIDs)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
