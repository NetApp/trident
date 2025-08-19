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

	tridentErrors "github.com/netapp/trident/utils/errors"
)

var groupSnapshotTestMutex sync.RWMutex

func withGroupSnapshotTestMode(t *testing.T, mode string, allFlag bool, fn func()) {
	t.Helper()

	groupSnapshotTestMutex.Lock()
	defer groupSnapshotTestMutex.Unlock()

	origMode := OperatingMode
	origAllGroupSnapshots := allGroupSnapshots

	defer func() {
		OperatingMode = origMode
		allGroupSnapshots = origAllGroupSnapshots
	}()

	OperatingMode = mode
	allGroupSnapshots = allFlag

	fn()
}

func TestDeleteGroupSnapshotCmd_RunE(t *testing.T) {
	cleanup := setupHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name              string
		operatingMode     string
		args              []string
		allGroupSnapshots bool
		wantErr           bool
		setupMocks        func()
	}{
		{
			name:              "tunnel mode success",
			operatingMode:     ModeTunnel,
			args:              []string{"groupsnapshot-12345678-1234-1234-1234-123456789abc"},
			allGroupSnapshots: false,
			wantErr:           false,
		},
		{
			name:              "tunnel mode failure",
			operatingMode:     ModeTunnel,
			args:              []string{"groupsnapshot-12345678-1234-1234-1234-123456789abc"},
			allGroupSnapshots: false,
			wantErr:           true,
		},
		{
			name:              "direct mode with specific group snapshots",
			operatingMode:     ModeDirect,
			args:              []string{"groupsnapshot-12345678-1234-1234-1234-123456789abc"},
			allGroupSnapshots: false,
			wantErr:           false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-12345678-1234-1234-1234-123456789abc",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:              "direct mode with multiple group snapshots",
			operatingMode:     ModeDirect,
			args:              []string{"groupsnapshot-12345678-1234-1234-1234-123456789abc", "groupsnapshot-87654321-4321-4321-4321-cba987654321"},
			allGroupSnapshots: false,
			wantErr:           false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-12345678-1234-1234-1234-123456789abc",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-87654321-4321-4321-4321-cba987654321",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:              "direct mode with all flag",
			operatingMode:     ModeDirect,
			args:              []string{},
			allGroupSnapshots: true,
			wantErr:           false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot",
					httpmock.NewStringResponder(200, `{"groupSnapshots": ["groupsnapshot-12345678-1234-1234-1234-123456789abc", "groupsnapshot-87654321-4321-4321-4321-cba987654321"]}`))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-12345678-1234-1234-1234-123456789abc",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-87654321-4321-4321-4321-cba987654321",
					httpmock.NewStringResponder(200, ""))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withGroupSnapshotTestMode(t, tc.operatingMode, tc.allGroupSnapshots, func() {
				httpmock.Reset()

				if tc.operatingMode == ModeTunnel {
					prevExec := execKubernetesCLIRaw
					defer func() { execKubernetesCLIRaw = prevExec }()

					execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
						if tc.wantErr {
							return exec.Command("false")
						}
						return exec.Command("echo", "Group snapshot deleted successfully")
					}
				}

				if tc.setupMocks != nil {
					tc.setupMocks()
				}

				cmd := &cobra.Command{}
				err := deleteGroupSnapshotCmd.RunE(cmd, tc.args)

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestGroupSnapshotDelete(t *testing.T) {
	cleanup := setupHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name              string
		groupSnapshotIDs  []string
		allGroupSnapshots bool
		wantErr           bool
		setupMocks        func()
	}{
		{
			name:              "delete specific group snapshots",
			groupSnapshotIDs:  []string{"groupsnapshot-12345678-1234-1234-1234-123456789abc"},
			allGroupSnapshots: false,
			wantErr:           false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-12345678-1234-1234-1234-123456789abc",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:              "delete multiple group snapshots",
			groupSnapshotIDs:  []string{"groupsnapshot-12345678-1234-1234-1234-123456789abc", "groupsnapshot-87654321-4321-4321-4321-cba987654321"},
			allGroupSnapshots: false,
			wantErr:           false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-12345678-1234-1234-1234-123456789abc",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-87654321-4321-4321-4321-cba987654321",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:              "HTTP error response",
			groupSnapshotIDs:  []string{"groupsnapshot-12345678-1234-1234-1234-123456789abc"},
			allGroupSnapshots: false,
			wantErr:           true,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-12345678-1234-1234-1234-123456789abc",
					httpmock.NewStringResponder(500, `{"error": "internal server error"}`))
			},
		},
		{
			name:              "validation error with all flag and specific IDs",
			groupSnapshotIDs:  []string{"groupsnapshot-12345678-1234-1234-1234-123456789abc"},
			allGroupSnapshots: true,
			wantErr:           true,
			setupMocks:        func() {},
		},
		{
			name:              "no IDs and no all flag error",
			groupSnapshotIDs:  []string{},
			allGroupSnapshots: false,
			wantErr:           true,
			setupMocks:        func() {},
		},
		{
			name:              "all flag without specific IDs",
			groupSnapshotIDs:  []string{},
			allGroupSnapshots: true,
			wantErr:           false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot",
					httpmock.NewStringResponder(200, `{"groupSnapshots": ["groupsnapshot-12345678-1234-1234-1234-123456789abc", "groupsnapshot-87654321-4321-4321-4321-cba987654321"]}`))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-12345678-1234-1234-1234-123456789abc",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-87654321-4321-4321-4321-cba987654321",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:              "all flag error getting group snapshots list",
			groupSnapshotIDs:  []string{},
			allGroupSnapshots: true,
			wantErr:           true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot",
					httpmock.NewStringResponder(500, `{"error": "internal server error"}`))
			},
		},
		{
			name:              "invalid group snapshot ID format",
			groupSnapshotIDs:  []string{"invalid-format"},
			allGroupSnapshots: false,
			wantErr:           true,
			setupMocks:        func() {},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			withGroupSnapshotTestMode(t, ModeDirect, tc.allGroupSnapshots, func() {
				err := groupSnapshotDelete(tc.groupSnapshotIDs...)

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestDeleteGroupSnapshot(t *testing.T) {
	cleanup := setupHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name       string
		groupID    string
		wantErr    bool
		setupMock  func()
		isNotFound bool
	}{
		{
			name:    "success",
			groupID: "groupsnapshot-12345678-1234-1234-1234-123456789abc",
			setupMock: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-12345678-1234-1234-1234-123456789abc",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:    "HTTP error response",
			groupID: "groupsnapshot-12345678-1234-1234-1234-123456789abc",
			wantErr: true,
			setupMock: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-12345678-1234-1234-1234-123456789abc",
					httpmock.NewStringResponder(500, `{"error": "server error"}`))
			},
		},
		{
			name:    "invalid ID format",
			groupID: "invalid-format",
			wantErr: true,
		},
		{
			name:    "HTTP request error",
			groupID: "groupsnapshot-12345678-1234-1234-1234-123456789abc",
			wantErr: true,
			setupMock: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-12345678-1234-1234-1234-123456789abc",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:       "404 not found",
			groupID:    "groupsnapshot-87654321-4321-4321-4321-cba987654321",
			wantErr:    true,
			isNotFound: true,
			setupMock: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/groupsnapshot/groupsnapshot-87654321-4321-4321-4321-cba987654321",
					httpmock.NewStringResponder(404, `{"error": "not found"}`))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			if tc.setupMock != nil {
				tc.setupMock()
			}

			err := DeleteGroupSnapshot(tc.groupID)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.isNotFound {
					assert.True(t, tridentErrors.IsNotFoundError(err))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
