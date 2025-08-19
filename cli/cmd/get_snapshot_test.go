// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

const (
	testSnapshotVolume1 = "vol1"
	testSnapshotVolume2 = "vol2"
	testSnapshot1       = "snapshot1"
	testSnapshot2       = "snapshot2"
	testSnap1           = "snap1"
	testSnap2           = "snap2"
	testSnapshotName    = "test-snap"
	testVolumeName      = "test-vol"
	testVolumeFlag      = "test-volume"
	testGroupFlag       = "test-group"
	testGroupSnapshot   = "test-group"
	testSnapshot1ID     = testSnapshotVolume1 + "/" + testSnap1
	testSnapshot2ID     = testSnapshotVolume2 + "/" + testSnap2
	testNonexistentID   = testSnapshotVolume1 + "/nonexistent"
	testInvalidID       = "invalid-id"
	testCreated1        = "2024-01-01T10:00:00Z"
	testCreated2        = "2024-01-01T12:00:00Z"
	testSizeBytes1      = int64(1024)
	testSizeBytes2      = int64(1073741824) // 1GB
	testSizeBytes3      = int64(536870912)  // 512MB
)

var (
	basicTestSnapshot1 = storage.SnapshotExternal{
		Snapshot: storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Name:       testSnapshot1,
				VolumeName: testSnapshotVolume1,
			},
			Created:   testCreated1,
			SizeBytes: testSizeBytes1,
			State:     storage.SnapshotStateOnline,
		},
	}

	basicTestSnapshot2 = storage.SnapshotExternal{
		Snapshot: storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Name:       testSnapshot2,
				VolumeName: testSnapshotVolume2,
			},
			Created:   testCreated2,
			SizeBytes: testSizeBytes1,
			State:     storage.SnapshotStateUploading,
		},
	}

	validSnapshot = storage.SnapshotExternal{
		Snapshot: storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Name:              testSnapshotName,
				VolumeName:        testVolumeName,
				ImportNotManaged:  false,
				GroupSnapshotName: testGroupSnapshot,
			},
			Created:   testCreated1,
			SizeBytes: testSizeBytes2,
			State:     storage.SnapshotStateOnline,
		},
	}

	unManagedSnapshot = storage.SnapshotExternal{
		Snapshot: storage.Snapshot{
			Config: &storage.SnapshotConfig{
				Name:              "unmanaged-snap",
				VolumeName:        "unmanaged-vol",
				ImportNotManaged:  true,
				GroupSnapshotName: "",
			},
			Created:   testCreated2,
			SizeBytes: testSizeBytes3,
			State:     storage.SnapshotStateCreating,
		},
	}

	basicTestSnapshots = []storage.SnapshotExternal{basicTestSnapshot1, basicTestSnapshot2}
	mixedTestSnapshots = []storage.SnapshotExternal{validSnapshot, unManagedSnapshot}
)

func getFakeSnapshots() []storage.SnapshotExternal {
	return basicTestSnapshots
}

func setupSuccessfulSnapshotMock(snapshotID string, snapshot *storage.SnapshotExternal) {
	httpmock.RegisterResponder("GET", BaseURL()+"/snapshot/"+snapshotID,
		func(req *http.Request) (*http.Response, error) {
			response := rest.GetSnapshotResponse{Snapshot: snapshot}
			return httpmock.NewJsonResponse(200, response)
		})
}

func setupSnapshotListMock(snapshots []string) {
	httpmock.RegisterResponder("GET", BaseURL()+"/snapshot",
		func(req *http.Request) (*http.Response, error) {
			response := rest.ListSnapshotsResponse{Snapshots: snapshots}
			return httpmock.NewJsonResponse(200, response)
		})
}

func redirectSnapshotStdout(t *testing.T) func() {
	origOut := os.Stdout
	return func() {
		os.Stdout = origOut
	}
}

func TestGetSnapshotCmd_Execution(t *testing.T) {
	originalMode := OperatingMode
	originalExecFunc := execKubernetesCLIRaw
	t.Cleanup(func() {
		OperatingMode = originalMode
		execKubernetesCLIRaw = originalExecFunc
	})
	snapshots := getFakeSnapshots()

	testCases := []struct {
		name          string
		mode          string
		args          []string
		volumeFlag    string
		groupFlag     string
		expectError   bool
		errorContains string
		setupMocks    func()
	}{
		{
			name:        "tunnel mode success",
			mode:        "tunnel",
			args:        []string{testSnapshot1ID},
			expectError: false,
			setupMocks: func() {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					return exec.Command("echo", "tunnel success output")
				}
			},
		},
		{
			name:        "direct mode success",
			mode:        "direct",
			args:        []string{testSnapshot1ID},
			expectError: false,
			setupMocks: func() {
				httpmock.Activate()
				setupSuccessfulSnapshotMock(testSnapshot1ID, &snapshots[0])
			},
		},
		{
			name:          "tunnel mode with both flags error",
			mode:          "tunnel",
			args:          []string{},
			volumeFlag:    testVolumeFlag,
			groupFlag:     testGroupFlag,
			expectError:   true,
			errorContains: "cannot specify both --volume and --group flags",
			setupMocks: func() {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					return exec.Command("false") // command that fails
				}
			},
		},
		{
			name:        "tunnel mode with volume flag success",
			mode:        "tunnel",
			args:        []string{},
			volumeFlag:  testVolumeFlag,
			expectError: false,
			setupMocks: func() {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					return exec.Command("echo", "tunnel success with volume")
				}
			},
		},
		{
			name:        "direct mode with specific snapshot success",
			mode:        "direct",
			args:        []string{testSnapshot2ID},
			expectError: false,
			setupMocks: func() {
				httpmock.Activate()
				setupSuccessfulSnapshotMock(testSnapshot2ID, &snapshots[1])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.DeactivateAndReset()

			if tc.mode == "tunnel" {
				OperatingMode = ModeTunnel
			} else {
				OperatingMode = ModeDirect
			}

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			cmd := &cobra.Command{
				Use: "snapshot",
			}

			cmd.Flags().String("volume", "", "Volume name")
			cmd.Flags().String("group", "", "Group name")

			if tc.volumeFlag != "" {
				err := cmd.Flags().Set("volume", tc.volumeFlag)
				require.NoError(t, err)
			}
			if tc.groupFlag != "" {
				err := cmd.Flags().Set("group", tc.groupFlag)
				require.NoError(t, err)
			}

			cleanup := redirectSnapshotStdout(t)
			defer cleanup()

			err := getSnapshotCmd.RunE(cmd, tc.args)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			// Clean up httpmock after each test
			if tc.mode == "direct" {
				httpmock.DeactivateAndReset()
			}
		})
	}
}

func TestBuildSnapshotURL(t *testing.T) {
	testCases := []struct {
		name     string
		params   map[string]string
		expected string
	}{
		{
			name:     "with volume",
			params:   map[string]string{"volume": testVolumeFlag},
			expected: BaseURL() + "/snapshot?volume=" + testVolumeFlag,
		},
		{
			name:     "with group",
			params:   map[string]string{"group": testGroupFlag},
			expected: BaseURL() + "/snapshot?group=" + testGroupFlag,
		},
		{
			name:     "with both volume and group - volume takes precedence",
			params:   map[string]string{"volume": testVolumeFlag, "group": testGroupFlag},
			expected: BaseURL() + "/snapshot?volume=" + testVolumeFlag,
		},
		{
			name:     "with empty values",
			params:   map[string]string{"volume": "", "group": ""},
			expected: BaseURL() + "/snapshot",
		},
		{
			name:     "with no params",
			params:   map[string]string{},
			expected: BaseURL() + "/snapshot",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := buildSnapshotURL(tc.params)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSnapshotList(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	snapshots := getFakeSnapshots()

	testCases := []struct {
		name        string
		snapshotIDs []string
		wantErr     bool
		setupMocks  func()
		errorCheck  func(error) bool
	}{
		{
			name:        "get specific snapshots",
			snapshotIDs: []string{testSnapshot1ID},
			wantErr:     false,
			setupMocks: func() {
				setupSuccessfulSnapshotMock(testSnapshot1ID, &snapshots[0])
			},
		},
		{
			name:        "get all snapshots",
			snapshotIDs: []string{},
			wantErr:     false,
			setupMocks: func() {
				setupSnapshotListMock([]string{testSnapshot1ID, testSnapshot2ID})
				setupSuccessfulSnapshotMock(testSnapshot1ID, &snapshots[0])
				setupSuccessfulSnapshotMock(testSnapshot2ID, &snapshots[1])
			},
		},
		{
			name:        "error getting snapshot list",
			snapshotIDs: []string{},
			wantErr:     true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot",
					httpmock.NewErrorResponder(errors.New("api error")))
			},
		},
		{
			name:        "error getting specific snapshot",
			snapshotIDs: []string{testNonexistentID},
			wantErr:     true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot/"+testNonexistentID,
					httpmock.NewStringResponder(404, `{"error":"not found"}`))
			},
		},
		{
			name:        "skip not found when getting all",
			snapshotIDs: []string{},
			wantErr:     false,
			setupMocks: func() {
				setupSnapshotListMock([]string{testSnapshot1ID, testNonexistentID})
				setupSuccessfulSnapshotMock(testSnapshot1ID, &snapshots[0])
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot/"+testNonexistentID,
					httpmock.NewStringResponder(404, `{"error":"not found"}`))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			cleanup := redirectSnapshotStdout(t)
			defer cleanup()

			devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
			require.NoError(t, err)
			os.Stdout = devNull
			defer devNull.Close()

			err = snapshotList(tc.snapshotIDs)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorCheck != nil {
					assert.True(t, tc.errorCheck(err))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetSnapshots(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name         string
		url          string
		wantErr      bool
		setupMocks   func()
		expectedList []string
	}{
		{
			name: "successful get snapshots",
			url:  BaseURL() + "/snapshot",
			setupMocks: func() {
				setupSnapshotListMock([]string{testSnapshot1ID, testSnapshot2ID})
			},
			expectedList: []string{testSnapshot1ID, testSnapshot2ID},
		},
		{
			name:    "empty URL error",
			url:     "",
			wantErr: true,
		},
		{
			name: "network error",
			url:  BaseURL() + "/snapshot",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
			wantErr: true,
		},
		{
			name: "HTTP error response",
			url:  BaseURL() + "/snapshot",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot",
					httpmock.NewStringResponder(500, `{"error":"server error"}`))
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			result, err := GetSnapshots(tc.url)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedList, result)
			}
		})
	}
}

func TestGetSnapshot(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	snapshots := getFakeSnapshots()

	testCases := []struct {
		name       string
		snapshotID string
		wantErr    bool
		setupMocks func()
		isNotFound bool
		checkState func(*testing.T, storage.SnapshotExternal)
	}{
		{
			name:       "invalid snapshot ID",
			snapshotID: testInvalidID,
			wantErr:    true,
		},
		{
			name:       "successful get snapshot",
			snapshotID: testSnapshot1ID,
			setupMocks: func() {
				setupSuccessfulSnapshotMock(testSnapshot1ID, &snapshots[0])
			},
			checkState: func(t *testing.T, snapshot storage.SnapshotExternal) {
				assert.Equal(t, maskDisplayOfSnapshotStateOnline, snapshot.State)
			},
		},
		{
			name:       "snapshot not found",
			snapshotID: testNonexistentID,
			wantErr:    true,
			isNotFound: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot/"+testNonexistentID,
					httpmock.NewStringResponder(404, `{"error":"not found"}`))
			},
		},
		{
			name:       "server error",
			snapshotID: testSnapshot1ID,
			wantErr:    true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot/"+testSnapshot1ID,
					httpmock.NewStringResponder(500, `{"error":"server error"}`))
			},
		},
		{
			name:       "network error",
			snapshotID: testSnapshot1ID,
			wantErr:    true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot/"+testSnapshot1ID,
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:       "invalid JSON response",
			snapshotID: testSnapshot1ID,
			wantErr:    true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot/"+testSnapshot1ID,
					httpmock.NewStringResponder(200, "invalid json"))
			},
		},
		{
			name:       "nil snapshot in response",
			snapshotID: testSnapshot1ID,
			wantErr:    true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/snapshot/"+testSnapshot1ID,
					func(req *http.Request) (*http.Response, error) {
						response := rest.GetSnapshotResponse{Snapshot: nil}
						return httpmock.NewJsonResponse(200, response)
					})
			},
		},
		{
			name:       "non-online state not masked",
			snapshotID: testSnapshot2ID,
			setupMocks: func() {
				setupSuccessfulSnapshotMock(testSnapshot2ID, &snapshots[1])
			},
			checkState: func(t *testing.T, snapshot storage.SnapshotExternal) {
				assert.Equal(t, storage.SnapshotStateUploading, snapshot.State)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			result, err := GetSnapshot(tc.snapshotID)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.isNotFound {
					assert.True(t, errors.IsNotFoundError(err))
				}
			} else {
				assert.NoError(t, err)
				if tc.checkState != nil {
					tc.checkState(t, result)
				}
			}
		})
	}
}

func TestWriteSnapshots(t *testing.T) {
	snapshots := getFakeSnapshots()

	testCases := []struct {
		name         string
		outputFormat string
		snapshots    []storage.SnapshotExternal
		contains     []string
	}{
		{
			name:         "JSON format",
			outputFormat: FormatJSON,
			snapshots:    snapshots,
			contains:     []string{testSnapshot1, testSnapshotVolume1},
		},
		{
			name:         "YAML format",
			outputFormat: FormatYAML,
			snapshots:    snapshots,
			contains:     []string{testSnapshot1, testSnapshotVolume1},
		},
		{
			name:         "Name format",
			outputFormat: FormatName,
			snapshots:    snapshots,
			contains:     []string{testSnapshotVolume1 + "/" + testSnapshot1, testSnapshotVolume2 + "/" + testSnapshot2},
		},
		{
			name:         "Wide format",
			outputFormat: FormatWide,
			snapshots:    snapshots,
			contains:     []string{"NAME", "VOLUME", "CREATED", "SIZE"},
		},
		{
			name:         "Default format (table)",
			outputFormat: "",
			snapshots:    snapshots,
			contains:     []string{"NAME", "VOLUME", "MANAGED"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origFormat := OutputFormat
			defer func() {
				OutputFormat = origFormat
			}()

			cleanup := redirectSnapshotStdout(t)
			defer cleanup()

			OutputFormat = tc.outputFormat

			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			WriteSnapshots(tc.snapshots)

			w.Close()
			output, _ := io.ReadAll(r)
			outputStr := string(output)

			for _, expected := range tc.contains {
				assert.Contains(t, outputStr, expected)
			}
		})
	}
}

func TestWriteSnapshotTable(t *testing.T) {
	snapshots := getFakeSnapshots()

	cleanup := redirectSnapshotStdout(t)
	defer cleanup()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	writeSnapshotTable(snapshots)

	w.Close()

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	assert.Contains(t, outputStr, "NAME")
	assert.Contains(t, outputStr, "VOLUME")
	assert.Contains(t, outputStr, "MANAGED")
	assert.Contains(t, outputStr, testSnapshot1)
	assert.Contains(t, outputStr, testSnapshotVolume1)
}

func TestWriteWideSnapshotTable(t *testing.T) {
	snapshots := getFakeSnapshots()

	cleanup := redirectSnapshotStdout(t)
	defer cleanup()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	writeWideSnapshotTable(snapshots)

	w.Close()

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	assert.Contains(t, outputStr, "NAME")
	assert.Contains(t, outputStr, "VOLUME")
	assert.Contains(t, outputStr, "CREATED")
	assert.Contains(t, outputStr, "SIZE")
	assert.Contains(t, outputStr, "STATE")
	assert.Contains(t, outputStr, "MANAGED")
	assert.Contains(t, outputStr, "GROUPSNAPSHOT")
}

func TestWriteSnapshotIDs(t *testing.T) {
	snapshots := []storage.SnapshotExternal{
		{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       testSnapshot1,
					VolumeName: testSnapshotVolume1,
				},
			},
		},
		{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       testSnapshot2,
					VolumeName: testSnapshotVolume2,
				},
			},
		},
	}

	cleanup := redirectSnapshotStdout(t)
	defer cleanup()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	writeSnapshotIDs(snapshots)

	w.Close()

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	assert.Contains(t, outputStr, testSnapshotVolume1+"/"+testSnapshot1)
	assert.Contains(t, outputStr, testSnapshotVolume2+"/"+testSnapshot2)
}

func TestMaskDisplayOfSnapshotStateOnline(t *testing.T) {
	assert.Equal(t, storage.SnapshotState(""), maskDisplayOfSnapshotStateOnline)
}

func TestWriteSnapshots_ErrorHandling(t *testing.T) {
	testSnapshots := mixedTestSnapshots

	testCases := []struct {
		name         string
		outputFormat string
		snapshots    []storage.SnapshotExternal
		expectPanic  bool
	}{
		{
			name:         "table format with nil config",
			outputFormat: "",
			snapshots:    testSnapshots,
			expectPanic:  false,
		},
		{
			name:         "wide format with nil config",
			outputFormat: FormatWide,
			snapshots:    testSnapshots,
			expectPanic:  false,
		},
		{
			name:         "name format with nil config",
			outputFormat: FormatName,
			snapshots:    testSnapshots,
			expectPanic:  false,
		},
		{
			name:         "JSON format with nil config",
			outputFormat: FormatJSON,
			snapshots:    testSnapshots,
			expectPanic:  false,
		},
		{
			name:         "YAML format with nil config",
			outputFormat: FormatYAML,
			snapshots:    testSnapshots,
			expectPanic:  false,
		},
		{
			name:         "empty snapshots list - table format",
			outputFormat: "",
			snapshots:    []storage.SnapshotExternal{},
			expectPanic:  false,
		},
		{
			name:         "empty snapshots list - wide format",
			outputFormat: FormatWide,
			snapshots:    []storage.SnapshotExternal{},
			expectPanic:  false,
		},
		{
			name:         "empty snapshots list - name format",
			outputFormat: FormatName,
			snapshots:    []storage.SnapshotExternal{},
			expectPanic:  false,
		},
		{
			name:         "only valid snapshots - table format",
			outputFormat: "",
			snapshots:    []storage.SnapshotExternal{validSnapshot},
			expectPanic:  false,
		},
		{
			name:         "only valid snapshots - wide format",
			outputFormat: FormatWide,
			snapshots:    []storage.SnapshotExternal{validSnapshot},
			expectPanic:  false,
		},
		{
			name:         "only valid snapshots - name format",
			outputFormat: FormatName,
			snapshots:    []storage.SnapshotExternal{validSnapshot},
			expectPanic:  false,
		},
		{
			name:         "managed and unmanaged snapshots - table format",
			outputFormat: "",
			snapshots:    []storage.SnapshotExternal{validSnapshot, unManagedSnapshot},
			expectPanic:  false,
		},
		{
			name:         "managed and unmanaged snapshots - wide format",
			outputFormat: FormatWide,
			snapshots:    []storage.SnapshotExternal{validSnapshot, unManagedSnapshot},
			expectPanic:  false,
		},
		{
			name:         "managed and unmanaged snapshots - name format",
			outputFormat: FormatName,
			snapshots:    []storage.SnapshotExternal{validSnapshot, unManagedSnapshot},
			expectPanic:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origFormat := OutputFormat
			defer func() {
				OutputFormat = origFormat
			}()

			cleanup := redirectSnapshotStdout(t)
			defer cleanup()

			OutputFormat = tc.outputFormat

			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			testFunc := func() {
				WriteSnapshots(tc.snapshots)
			}

			if tc.expectPanic {
				assert.Panics(t, testFunc)
			} else {
				assert.NotPanics(t, testFunc)
			}

			w.Close()

			output, err := io.ReadAll(r)
			require.NoError(t, err)

			if len(tc.snapshots) > 0 {
				switch tc.outputFormat {
				case FormatJSON:
					assert.Contains(t, string(output), "items")
				case FormatYAML:
					assert.Contains(t, string(output), "items")
				case FormatWide, "":
					assert.NotEmpty(t, string(output))
				}
			}
		})
	}
}

func TestWriteSnapshotTableFunctions(t *testing.T) {
	testSnapshots := []storage.SnapshotExternal{validSnapshot}

	t.Run("writeSnapshotTable", func(t *testing.T) {
		cleanup := redirectSnapshotStdout(t)
		defer cleanup()

		r, w, _ := os.Pipe()
		os.Stdout = w

		assert.NotPanics(t, func() {
			writeSnapshotTable(testSnapshots)
		})

		w.Close()
		output, _ := io.ReadAll(r)

		outputStr := string(output)
		assert.Contains(t, outputStr, testSnapshotName)
		assert.Contains(t, outputStr, testVolumeName)
	})

	t.Run("writeWideSnapshotTable", func(t *testing.T) {
		cleanup := redirectSnapshotStdout(t)
		defer cleanup()

		r, w, _ := os.Pipe()
		os.Stdout = w

		assert.NotPanics(t, func() {
			writeWideSnapshotTable(testSnapshots)
		})

		w.Close()
		output, _ := io.ReadAll(r)

		outputStr := string(output)
		assert.Contains(t, outputStr, testSnapshotName)
		assert.Contains(t, outputStr, testVolumeName)
		assert.Contains(t, outputStr, testGroupSnapshot)
		assert.Contains(t, outputStr, testCreated1)
	})

	t.Run("writeSnapshotIDs", func(t *testing.T) {
		cleanup := redirectSnapshotStdout(t)
		defer cleanup()

		r, w, _ := os.Pipe()
		os.Stdout = w

		assert.NotPanics(t, func() {
			writeSnapshotIDs(testSnapshots)
		})

		w.Close()
		output, _ := io.ReadAll(r)

		outputStr := string(output)
		assert.Contains(t, outputStr, testVolumeName)
		assert.Contains(t, outputStr, testSnapshotName)
	})
}
