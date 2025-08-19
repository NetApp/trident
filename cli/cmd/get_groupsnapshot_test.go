// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"os/exec"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

const (
	getTestGroupSnapshotID1 = "groupsnapshot-12345678-1234-1234-1234-123456789abc"
	getTestGroupSnapshotID2 = "groupsnapshot-87654321-4321-4321-4321-cba987654321"
	getTestGroupSnapshotID3 = "groupsnapshot-test1"
	getTestGroupSnapshotID4 = "groupsnapshot-test2"
	getInvalidGroupID       = "invalid-format"
)

const (
	validGroupSnapshotJSON = `{
  "groupSnapshot": {
    "version": "v1",
    "name": "test-group",
    "internalName": "int-test-group",
    "volumeNames": ["vol1"],
    "created": "2024-01-01T00:00:00Z",
    "snapshotIDs": ["snap1"]
  }
}`

	groupSnapshot1JSON = `{
  "groupSnapshot": {
    "id": "groupsnapshot-test1",
    "version": "v1",
    "name": "snapshot-group-1",
    "internalName": "int-snap-group",
    "volumeNames": ["vol1", "vol2"],
    "created": "2024-01-01T00:00:00Z",
    "snapshotIDs": ["snap1", "snap2"]
  }
}`

	groupSnapshot2JSON = `{
  "groupSnapshot": {
    "id": "groupsnapshot-test2",
    "version": "v1",
    "name": "snapshot-group-2",
    "internalName": "int-snap-group",
    "volumeNames": ["vol1", "vol2"],
    "created": "2024-01-02T00:00:00Z",
    "snapshotIDs": ["snap3", "snap4"]
  }
}`

	groupSnapshotsListJSON = `{"groupSnapshots": ["groupsnapshot-test1", "groupsnapshot-test2"]}`
	notFoundErrorJSON      = `{"error": "group snapshot not found"}`
	serverErrorJSON        = `{"error": "internal server error"}`
	nullGroupSnapshotJSON  = `{"groupSnapshot": null}`
	nullGroupSnapshotsJSON = `{"groupSnapshots": null}`
)

func setupGroupSnapshotHTTPMock(t *testing.T) func() {
	t.Helper()
	httpmock.Activate()
	return func() {
		httpmock.DeactivateAndReset()
	}
}

func setupGetGroupSnapshotMocks() {
	httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot/"+getTestGroupSnapshotID3,
		httpmock.NewStringResponder(200, groupSnapshot1JSON))
	httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot/"+getTestGroupSnapshotID4,
		httpmock.NewStringResponder(200, groupSnapshot2JSON))
}

func TestGetGroupSnapshotCmd_RunE(t *testing.T) {
	cleanup := setupGroupSnapshotHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name             string
		operatingMode    string
		args             []string
		maxSnapshotRows  int
		wantErr          bool
		setupMocks       func()
		mockExecFunction func(args ...string) *exec.Cmd
	}{
		{
			name:            "tunnel mode success",
			operatingMode:   ModeTunnel,
			args:            []string{getTestGroupSnapshotID1},
			maxSnapshotRows: defaultMaxSnapshotRows,
			wantErr:         false,
			mockExecFunction: func(args ...string) *exec.Cmd {
				return exec.Command("echo", "Group snapshot retrieved successfully")
			},
		},
		{
			name:            "tunnel mode failure",
			operatingMode:   ModeTunnel,
			args:            []string{getTestGroupSnapshotID1},
			maxSnapshotRows: defaultMaxSnapshotRows,
			wantErr:         true,
			mockExecFunction: func(args ...string) *exec.Cmd {
				return exec.Command("false")
			},
		},
		{
			name:            "direct mode success",
			operatingMode:   ModeDirect,
			args:            []string{getTestGroupSnapshotID1},
			maxSnapshotRows: defaultMaxSnapshotRows,
			wantErr:         false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot/"+getTestGroupSnapshotID1,
					httpmock.NewStringResponder(200, validGroupSnapshotJSON))
			},
		},
		{
			name:            "direct mode error",
			operatingMode:   ModeDirect,
			args:            []string{getTestGroupSnapshotID1},
			maxSnapshotRows: defaultMaxSnapshotRows,
			wantErr:         true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot/"+getTestGroupSnapshotID1,
					httpmock.NewStringResponder(500, serverErrorJSON))
			},
		},
		{
			name:            "negative max snapshots validation",
			operatingMode:   ModeDirect,
			args:            []string{getTestGroupSnapshotID1},
			maxSnapshotRows: -1,
			wantErr:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			prevOperatingMode := OperatingMode
			prevMaxSnapshotRows := maxSnapshotRows
			prevExecKubernetesCLIRaw := execKubernetesCLIRaw

			defer func() {
				OperatingMode = prevOperatingMode
				maxSnapshotRows = prevMaxSnapshotRows
				execKubernetesCLIRaw = prevExecKubernetesCLIRaw
			}()

			OperatingMode = tc.operatingMode
			maxSnapshotRows = tc.maxSnapshotRows

			if tc.mockExecFunction != nil {
				execKubernetesCLIRaw = tc.mockExecFunction
			}

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			cmd := &cobra.Command{}
			err := getGroupSnapshotCmd.RunE(cmd, tc.args)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGroupSnapshotList(t *testing.T) {
	cleanup := setupGroupSnapshotHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name             string
		groupSnapshotIDs []string
		wantErr          bool
		errorContains    string
		setupMocks       func()
	}{
		{
			name:             "list specific group snapshots",
			groupSnapshotIDs: []string{getTestGroupSnapshotID3, getTestGroupSnapshotID4},
			wantErr:          false,
			setupMocks: func() {
				setupGetGroupSnapshotMocks()
			},
		},
		{
			name:             "list all group snapshots",
			groupSnapshotIDs: []string{},
			wantErr:          false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot",
					httpmock.NewStringResponder(200, groupSnapshotsListJSON))
				setupGetGroupSnapshotMocks()
			},
		},
		{
			name:             "error getting group snapshots list",
			groupSnapshotIDs: []string{},
			wantErr:          true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:             "invalid group snapshot ID format",
			groupSnapshotIDs: []string{getInvalidGroupID},
			wantErr:          true,
			errorContains:    "invalid group snapshot ID",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			err := groupSnapshotList(tc.groupSnapshotIDs...)

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

func TestGetGroupSnapshot(t *testing.T) {
	cleanup := setupGroupSnapshotHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name          string
		groupID       string
		wantErr       bool
		errorContains string
		setupMocks    func()
		isNotFound    bool
	}{
		{
			name:    "successful group snapshot retrieval",
			groupID: getTestGroupSnapshotID3,
			wantErr: false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot/"+getTestGroupSnapshotID3,
					httpmock.NewStringResponder(200, validGroupSnapshotJSON))
			},
		},
		{
			name:          "invalid group snapshot ID",
			groupID:       getInvalidGroupID,
			wantErr:       true,
			errorContains: "invalid group snapshot ID",
		},
		{
			name:       "group snapshot not found",
			groupID:    "groupsnapshot-notfound",
			wantErr:    true,
			isNotFound: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot/groupsnapshot-notfound",
					httpmock.NewStringResponder(404, notFoundErrorJSON))
			},
		},
		{
			name:    "server error during retrieval",
			groupID: "groupsnapshot-server-error",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot/groupsnapshot-server-error",
					httpmock.NewStringResponder(500, serverErrorJSON))
			},
		},
		{
			name:    "network error during HTTP call",
			groupID: "groupsnapshot-network-error",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot/groupsnapshot-network-error",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:    "invalid JSON response",
			groupID: "groupsnapshot-invalid-json",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot/groupsnapshot-invalid-json",
					httpmock.NewStringResponder(200, `invalid json`))
			},
		},
		{
			name:          "null group snapshot in response",
			groupID:       "groupsnapshot-null",
			wantErr:       true,
			errorContains: "no snapshot returned",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot/groupsnapshot-null",
					httpmock.NewStringResponder(200, nullGroupSnapshotJSON))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			result, err := GetGroupSnapshot(tc.groupID)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				if tc.isNotFound {
					assert.True(t, errors.IsNotFoundError(err))
				}
				assert.Equal(t, storage.GroupSnapshotExternal{}, result)
			} else {
				assert.NoError(t, err)
				assert.NotEqual(t, storage.GroupSnapshotExternal{}, result)
			}
		})
	}
}

func TestGetGroupSnapshots(t *testing.T) {
	cleanup := setupGroupSnapshotHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name          string
		wantErr       bool
		errorContains string
		setupMocks    func()
		expectedCount int
	}{
		{
			name:          "successful group snapshots retrieval",
			wantErr:       false,
			expectedCount: 2,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot",
					httpmock.NewStringResponder(200, groupSnapshotsListJSON))
			},
		},
		{
			name:    "server error during retrieval",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot",
					httpmock.NewStringResponder(500, serverErrorJSON))
			},
		},
		{
			name:    "network error during HTTP call",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:    "invalid JSON response",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot",
					httpmock.NewStringResponder(200, `invalid json`))
			},
		},
		{
			name:          "null group snapshots in response",
			wantErr:       true,
			errorContains: "no group snapshots found",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/groupsnapshot",
					httpmock.NewStringResponder(200, nullGroupSnapshotsJSON))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			result, err := GetGroupSnapshots()

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tc.expectedCount)
			}
		})
	}
}

func TestFormatIDs(t *testing.T) {
	testCases := []struct {
		name     string
		ids      []string
		max      int
		expected string
	}{
		{
			name:     "empty ids",
			ids:      []string{},
			max:      3,
			expected: "",
		},
		{
			name:     "max is zero shows all",
			ids:      []string{"id1", "id2", "id3", "id4"},
			max:      0,
			expected: "id1\nid2\nid3\nid4",
		},
		{
			name:     "ids count less than max",
			ids:      []string{"id1", "id2"},
			max:      3,
			expected: "id1\nid2",
		},
		{
			name:     "ids count equals max",
			ids:      []string{"id1", "id2", "id3"},
			max:      3,
			expected: "id1\nid2\nid3",
		},
		{
			name:     "ids count greater than max",
			ids:      []string{"id1", "id2", "id3", "id4", "id5"},
			max:      3,
			expected: "id1\nid2\nid3\n... (+2 more)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := formatIDs(tc.ids, tc.max)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestWriteGroupSnapshots(t *testing.T) {
	testCases := []struct {
		name         string
		outputFormat string
		snapshots    []storage.GroupSnapshotExternal
	}{
		{
			name:         "JSON format",
			outputFormat: FormatJSON,
			snapshots: []storage.GroupSnapshotExternal{{
				GroupSnapshot: storage.GroupSnapshot{
					SnapshotIDs: []string{"id-1"},

					GroupSnapshotConfig: &storage.GroupSnapshotConfig{
						Name: "test-group",
					},
				},
			}},
		},
		{
			name:         "YAML format",
			outputFormat: FormatYAML,
			snapshots: []storage.GroupSnapshotExternal{{
				GroupSnapshot: storage.GroupSnapshot{
					SnapshotIDs: []string{"id-1"},

					GroupSnapshotConfig: &storage.GroupSnapshotConfig{
						Name: "test-group",
					},
				},
			}},
		},
		{
			name:         "Name format",
			outputFormat: FormatName,
			snapshots: []storage.GroupSnapshotExternal{{
				GroupSnapshot: storage.GroupSnapshot{
					SnapshotIDs: []string{"id-1"},

					GroupSnapshotConfig: &storage.GroupSnapshotConfig{
						Name: "test-group",
					},
				},
			}},
		},
		{
			name:         "Wide format",
			outputFormat: FormatWide,
			snapshots: []storage.GroupSnapshotExternal{{
				GroupSnapshot: storage.GroupSnapshot{
					SnapshotIDs: []string{"id-1"},

					GroupSnapshotConfig: &storage.GroupSnapshotConfig{
						Name: "test-group",
					},
				},
			}},
		},
		{
			name:         "Default format",
			outputFormat: "",
			snapshots: []storage.GroupSnapshotExternal{{
				GroupSnapshot: storage.GroupSnapshot{
					SnapshotIDs: []string{"id-1"},

					GroupSnapshotConfig: &storage.GroupSnapshotConfig{
						Name: "test-group",
					},
				},
			}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prevOutputFormat := OutputFormat
			defer func() {
				OutputFormat = prevOutputFormat
			}()

			OutputFormat = tc.outputFormat

			assert.NotPanics(t, func() {
				WriteGroupSnapshots(tc.snapshots)
			})
		})
	}
}
