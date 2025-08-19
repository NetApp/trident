package cmd

import (
	"bytes"
	"encoding/json"
	"os"
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
	testVolume3        = "vol3"
	testParentVolume   = "parent-vol"
	testSubVolume1     = "sub-vol1"
	testSubVolume2     = "sub-vol2"
	testSubordinateVol = "sub-vol"
	testSize1GB        = "1073741824"
	testSize2GB        = "2147483648"
	testBackendName1   = "backend1"
	testBackendName2   = "backend2"
	testBackendUUID1   = "backend-uuid-1"
	testBackendUUID2   = "backend-uuid-2"
	testFormatWide     = "wide"
	testFormatYAML     = "yaml"
	testFormatJSON     = "json"
	urlVolume          = "/volume"
	urlBackend         = "/backend"
	msgNotFound        = "Not Found"
	msgInternalError   = "Internal Server Error"
	msgInvalidJSON     = "invalid json"
	stateOffline       = "offline"
)

func createTestVolume(name, size string) storage.VolumeExternal {
	return storage.VolumeExternal{
		Config:      &storage.VolumeConfig{Name: name, Size: size},
		BackendUUID: testBackendUUID1,
		State:       storage.VolumeStateOnline,
	}
}

func createTestVolumeWithBackend(name, size, backendUUID string) storage.VolumeExternal {
	return storage.VolumeExternal{
		Config:      &storage.VolumeConfig{Name: name, Size: size},
		BackendUUID: backendUUID,
		State:       storage.VolumeStateOnline,
	}
}

func createTestBackend(name, uuid string) storage.BackendExternal {
	return storage.BackendExternal{
		Name:        name,
		BackendUUID: uuid,
	}
}

func setupVolumeListMockForMultiple(volumes []string) {
	listResponse := rest.ListVolumesResponse{Volumes: volumes}
	responder, _ := httpmock.NewJsonResponder(200, listResponse)
	httpmock.RegisterResponder("GET", BaseURL()+urlVolume, responder)
}

func setupVolumeGetMock(volumeName string, volume *storage.VolumeExternal) {
	getResponse := rest.GetVolumeResponse{Volume: volume}
	responder, _ := httpmock.NewJsonResponder(200, getResponse)
	httpmock.RegisterResponder("GET", BaseURL()+urlVolume+"/"+volumeName, responder)
}

func setupBackendGetMock(backendUUID string, backend *storage.BackendExternal) {
	backendResponse := rest.GetBackendResponse{Backend: backend}
	responder, _ := httpmock.NewJsonResponder(200, backendResponse)
	httpmock.RegisterResponder("GET", BaseURL()+urlBackend+"/"+backendUUID, responder)
}

func setupErrorMock(url string, status int, message string) {
	httpmock.RegisterResponder("GET", url, httpmock.NewStringResponder(status, message))
}

func TestVolumeList(t *testing.T) {
	testCases := []struct {
		name         string
		volumeNames  []string
		outputFormat string
		wantErr      bool
		setupMocks   func() func()
	}{
		{
			name:        "get all volumes success",
			volumeNames: []string{},
			wantErr:     false,
			setupMocks: func() func() {
				httpmock.Activate()

				setupVolumeListMockForMultiple([]string{testVolume1, testVolume2})

				volume1 := createTestVolume(testVolume1, testSize1GB)
				volume2 := createTestVolumeWithBackend(testVolume2, testSize2GB, testBackendUUID2)
				setupVolumeGetMock(testVolume1, &volume1)
				setupVolumeGetMock(testVolume2, &volume2)

				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:        "get specific volumes success",
			volumeNames: []string{testVolume1, testVolume2},
			wantErr:     false,
			setupMocks: func() func() {
				httpmock.Activate()

				volume1 := createTestVolume(testVolume1, testSize1GB)
				volume2 := createTestVolumeWithBackend(testVolume2, testSize2GB, testBackendUUID2)
				setupVolumeGetMock(testVolume1, &volume1)
				setupVolumeGetMock(testVolume2, &volume2)

				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:        "get all volumes with GetVolumes error",
			volumeNames: []string{},
			wantErr:     true,
			setupMocks: func() func() {
				httpmock.Activate()

				setupErrorMock(BaseURL()+urlVolume, 500, msgInternalError)

				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:        "get specific volume with GetVolume error",
			volumeNames: []string{testVolume1},
			wantErr:     true,
			setupMocks: func() func() {
				httpmock.Activate()

				setupErrorMock(BaseURL()+urlVolume+"/"+testVolume1, 500, msgInternalError)

				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:        "get all volumes with some volumes not found (should continue)",
			volumeNames: []string{},
			wantErr:     false,
			setupMocks: func() func() {
				httpmock.Activate()

				setupVolumeListMockForMultiple([]string{testVolume1, testVolume2})

				volume1 := createTestVolume(testVolume1, testSize1GB)
				setupVolumeGetMock(testVolume1, &volume1)
				setupErrorMock(BaseURL()+urlVolume+"/"+testVolume2, 404, msgNotFound)

				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:        "get specific volume not found (should error)",
			volumeNames: []string{testVolume1},
			wantErr:     true,
			setupMocks: func() func() {
				httpmock.Activate()

				setupErrorMock(BaseURL()+urlVolume+"/"+testVolume1, 404, msgNotFound)

				return func() { httpmock.DeactivateAndReset() }
			},
		},
		{
			name:         "wide format with backend lookup",
			volumeNames:  []string{testVolume1},
			outputFormat: testFormatWide,
			wantErr:      false,
			setupMocks: func() func() {
				httpmock.Activate()

				volume1 := createTestVolume(testVolume1, testSize1GB)
				setupVolumeGetMock(testVolume1, &volume1)

				backend := createTestBackend(testBackendName1, testBackendUUID1)
				setupBackendGetMock(testBackendUUID1, &backend)

				return func() { httpmock.DeactivateAndReset() }
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cleanup := tc.setupMocks()
			defer cleanup()

			if tc.outputFormat != "" {
				originalFormat := OutputFormat
				OutputFormat = tc.outputFormat
				defer func() { OutputFormat = originalFormat }()
			}

			backendsByUUID = make(map[string]*storage.BackendExternal)

			err := volumeList(tc.volumeNames)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetVolumes(t *testing.T) {
	testCases := []struct {
		name              string
		sourceVolume      string
		subordinateVolume string
		expectedURL       string
		responseStatus    int
		responseBody      interface{}
		expectedVolumes   []string
		wantErr           bool
	}{
		{
			name:           "success without filters",
			expectedURL:    BaseURL() + urlVolume,
			responseStatus: 200,
			responseBody: rest.ListVolumesResponse{
				Volumes: []string{testVolume1, testVolume2, testVolume3},
			},
			expectedVolumes: []string{testVolume1, testVolume2, testVolume3},
			wantErr:         false,
		},
		{
			name:           "success with subordinateOf filter",
			sourceVolume:   testParentVolume,
			expectedURL:    BaseURL() + urlVolume + "?subordinateOf=" + testParentVolume,
			responseStatus: 200,
			responseBody: rest.ListVolumesResponse{
				Volumes: []string{testSubVolume1, testSubVolume2},
			},
			expectedVolumes: []string{testSubVolume1, testSubVolume2},
			wantErr:         false,
		},
		{
			name:              "success with parentOfSubordinate filter",
			subordinateVolume: testSubordinateVol,
			expectedURL:       BaseURL() + urlVolume + "?parentOfSubordinate=" + testSubordinateVol,
			responseStatus:    200,
			responseBody: rest.ListVolumesResponse{
				Volumes: []string{testParentVolume},
			},
			expectedVolumes: []string{testParentVolume},
			wantErr:         false,
		},
		{
			name:           "http error response",
			expectedURL:    BaseURL() + urlVolume,
			responseStatus: 500,
			responseBody:   msgInternalError,
			wantErr:        true,
		},
		{
			name:           "invalid json response",
			expectedURL:    BaseURL() + urlVolume,
			responseStatus: 200,
			responseBody:   msgInvalidJSON,
			wantErr:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			getSourceVolume = tc.sourceVolume
			getSubordinateVolume = tc.subordinateVolume
			defer func() {
				getSourceVolume = ""
				getSubordinateVolume = ""
			}()

			if tc.responseStatus == 200 {
				responder, _ := httpmock.NewJsonResponder(tc.responseStatus, tc.responseBody)
				httpmock.RegisterResponder("GET", tc.expectedURL, responder)
			} else {
				httpmock.RegisterResponder("GET", tc.expectedURL,
					httpmock.NewStringResponder(tc.responseStatus, tc.responseBody.(string)))
			}

			volumes, err := GetVolumes()

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedVolumes, volumes)
			}
		})
	}
}

func TestGetVolume(t *testing.T) {
	testCases := []struct {
		name           string
		volumeName     string
		responseStatus int
		responseBody   interface{}
		expectedVolume storage.VolumeExternal
		wantErr        bool
		isNotFound     bool
	}{
		{
			name:           "success with online state",
			volumeName:     testVolume1,
			responseStatus: 200,
			responseBody: rest.GetVolumeResponse{
				Volume: &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						Name: testVolume1,
						Size: testSize1GB,
					},
					BackendUUID: testBackendUUID1,
					State:       storage.VolumeStateOnline,
				},
			},
			expectedVolume: storage.VolumeExternal{
				Config: &storage.VolumeConfig{
					Name: testVolume1,
					Size: testSize1GB,
				},
				BackendUUID: testBackendUUID1,
				State:       maskDisplayOfVolumeStateOnline,
			},
			wantErr: false,
		},
		{
			name:           "success with non-online state",
			volumeName:     testVolume1,
			responseStatus: 200,
			responseBody: rest.GetVolumeResponse{
				Volume: &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						Name: testVolume1,
						Size: testSize1GB,
					},
					BackendUUID: testBackendUUID1,
					State:       storage.VolumeState(stateOffline),
				},
			},
			expectedVolume: storage.VolumeExternal{
				Config: &storage.VolumeConfig{
					Name: testVolume1,
					Size: testSize1GB,
				},
				BackendUUID: testBackendUUID1,
				State:       storage.VolumeState(stateOffline),
			},
			wantErr: false,
		},
		{
			name:           "not found error",
			volumeName:     testVolume1,
			responseStatus: 404,
			responseBody:   msgNotFound,
			wantErr:        true,
			isNotFound:     true,
		},
		{
			name:           "server error",
			volumeName:     testVolume1,
			responseStatus: 500,
			responseBody:   msgInternalError,
			wantErr:        true,
			isNotFound:     false,
		},
		{
			name:           "invalid json response",
			volumeName:     testVolume1,
			responseStatus: 200,
			responseBody:   msgInvalidJSON,
			wantErr:        true,
		},
		{
			name:           "nil volume in response",
			volumeName:     testVolume1,
			responseStatus: 200,
			responseBody: rest.GetVolumeResponse{
				Volume: nil,
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			expectedURL := BaseURL() + "/volume/" + tc.volumeName

			if tc.responseStatus == 200 {
				responder, _ := httpmock.NewJsonResponder(tc.responseStatus, tc.responseBody)
				httpmock.RegisterResponder("GET", expectedURL, responder)
			} else {
				httpmock.RegisterResponder("GET", expectedURL,
					httpmock.NewStringResponder(tc.responseStatus, tc.responseBody.(string)))
			}

			volume, err := GetVolume(tc.volumeName)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.isNotFound {
					assert.True(t, errors.IsNotFoundError(err))
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedVolume, volume)
			}
		})
	}
}

func TestWriteVolumes(t *testing.T) {
	testCases := []struct {
		name         string
		outputFormat string
		volumes      []storage.VolumeExternal
	}{
		{
			name:         "json format",
			outputFormat: FormatJSON,
			volumes: []storage.VolumeExternal{
				{
					Config: &storage.VolumeConfig{
						Name: "vol1",
						Size: "1073741824",
					},
					BackendUUID: "backend-uuid-1",
				},
			},
		},
		{
			name:         "yaml format",
			outputFormat: FormatYAML,
			volumes: []storage.VolumeExternal{
				{
					Config: &storage.VolumeConfig{
						Name: "vol1",
						Size: "1073741824",
					},
					BackendUUID: "backend-uuid-1",
				},
			},
		},
		{
			name:         "name format",
			outputFormat: FormatName,
			volumes: []storage.VolumeExternal{
				{
					Config: &storage.VolumeConfig{
						Name: "vol1",
						Size: "1073741824",
					},
					BackendUUID: "backend-uuid-1",
				},
			},
		},
		{
			name:         "wide format",
			outputFormat: FormatWide,
			volumes: []storage.VolumeExternal{
				{
					Config: &storage.VolumeConfig{
						Name:         "vol1",
						InternalName: "internal-vol1",
						Size:         "1073741824",
						StorageClass: "standard",
						Protocol:     "nfs",
						AccessMode:   "ReadWriteOnce",
					},
					BackendUUID: "backend-uuid-1",
					State:       storage.VolumeState("online"),
				},
			},
		},
		{
			name:         "default table format",
			outputFormat: "table",
			volumes: []storage.VolumeExternal{
				{
					Config: &storage.VolumeConfig{
						Name:         "vol1",
						Size:         "1073741824",
						StorageClass: "standard",
						Protocol:     "nfs",
					},
					BackendUUID: "backend-uuid-1",
					State:       storage.VolumeState("online"),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			originalFormat := OutputFormat
			OutputFormat = tc.outputFormat
			defer func() { OutputFormat = originalFormat }()

			if tc.outputFormat == FormatWide {
				backendsByUUID = map[string]*storage.BackendExternal{
					"backend-uuid-1": {
						Name:        "backend1",
						BackendUUID: "backend-uuid-1",
					},
				}
			}

			WriteVolumes(tc.volumes)

			w.Close()
			os.Stdout = oldStdout
			var buf bytes.Buffer
			_, err := buf.ReadFrom(r)
			require.NoError(t, err)

			output := buf.String()
			assert.NotEmpty(t, output)

			switch tc.outputFormat {
			case FormatJSON:
				var jsonOutput api.MultipleVolumeResponse
				assert.NoError(t, json.Unmarshal([]byte(output), &jsonOutput))
			case FormatName:
				assert.Contains(t, output, "vol1")
			case FormatWide:
				assert.Contains(t, output, "vol1")
				assert.Contains(t, output, "internal-vol1")
				assert.Contains(t, output, "backend1")
			default:
				assert.Contains(t, output, "vol1")
			}
		})
	}
}

func TestGetVolumeCmd(t *testing.T) {
	testCases := []struct {
		name          string
		args          []string
		flags         map[string]string
		operatingMode string
		setupMocks    func() func()
		wantErr       bool
	}{
		{
			name: "tunnel mode",
			args: []string{"vol1"},
			flags: map[string]string{
				"subordinateOf": "parent-vol",
			},
			operatingMode: ModeTunnel,
			setupMocks: func() func() {
				return func() {}
			},
			wantErr: true,
		},
		{
			name:          "normal mode success",
			args:          []string{"vol1"},
			operatingMode: "ModeNormal",
			setupMocks: func() func() {
				httpmock.Activate()

				volume1 := storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						Name: "vol1",
						Size: "1073741824",
					},
					BackendUUID: "backend-uuid-1",
					State:       storage.VolumeStateOnline,
				}
				getResponse1 := rest.GetVolumeResponse{Volume: &volume1}
				responder1, _ := httpmock.NewJsonResponder(200, getResponse1)
				httpmock.RegisterResponder("GET", BaseURL()+"/volume/vol1", responder1)

				return func() { httpmock.DeactivateAndReset() }
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cleanup := tc.setupMocks()
			defer cleanup()

			originalMode := OperatingMode
			OperatingMode = tc.operatingMode
			defer func() { OperatingMode = originalMode }()

			cmd := &cobra.Command{}
			for flag, value := range tc.flags {
				cmd.Flags().String(flag, "", "")
				cmd.Flags().Set(flag, value)
			}

			backendsByUUID = make(map[string]*storage.BackendExternal)

			err := getVolumeCmd.RunE(cmd, tc.args)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func setupTestVolume(name, size string) storage.VolumeExternal {
	return storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name:         name,
			Size:         size,
			StorageClass: "standard",
			Protocol:     "nfs",
			AccessMode:   "ReadWriteOnce",
		},
		BackendUUID: "backend-uuid-1",
		State:       storage.VolumeStateOnline,
	}
}

func TestWriteVolumeNames(t *testing.T) {
	volumes := []storage.VolumeExternal{
		setupTestVolume("vol1", "1073741824"),
		setupTestVolume("vol2", "2147483648"),
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	writeVolumeNames(volumes)

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "vol1")
	assert.Contains(t, output, "vol2")
}

func TestWriteVolumeTable(t *testing.T) {
	volumes := []storage.VolumeExternal{
		createTestVolume("vol1", "10"),
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	writeVolumeTable(volumes)

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "vol1")
}

func TestWriteWideVolumeTable(t *testing.T) {
	volumes := []storage.VolumeExternal{
		setupTestVolume("vol1", "1073741824"),
	}

	backendsByUUID = map[string]*storage.BackendExternal{
		"backend-uuid-1": {
			Name:        "backend1",
			BackendUUID: "backend-uuid-1",
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	writeWideVolumeTable(volumes)

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "vol1")
	assert.Contains(t, output, "backend1")
}
