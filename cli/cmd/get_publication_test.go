// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/models"
)

const (
	testVolume1          = "vol1"
	testVolume2          = "vol2"
	testNode1            = "node1"
	testNode2            = "node2"
	testPublication1     = "vol1-node1"
	testPublication2     = "vol2-node2"
	testPublicationName  = "test"
	errorServerError     = "internal server error"
	errorNetworkError    = "network error"
	errorGetPublications = "could not get volume publications"
	errorServerErrorMsg  = "server error"
	invalidJSONResponse  = "invalid json"
	mockOutput           = "mock output"
	httpStatusOK         = 200
	httpStatusError      = 500
)

const (
	successJSONTemplate       = `{"volumePublication": {"name": "%s", "volume": "%s", "node": "%s", "readOnly": false, "accessMode": 1}}`
	successListJSONTemplate   = `{"volumePublications": [{"name": "%s", "volume": "%s", "node": "%s", "readOnly": %t, "accessMode": %d}]}`
	emptyListJSONResponse     = `{"volumePublications": []}`
	serverErrorJSONResponse   = `{"error": "` + errorServerError + `"}`
	customServerErrorResponse = `{"error": "` + errorServerErrorMsg + `"}`
)

var (
	testVolumePublication1 = models.VolumePublicationExternal{
		Name:       testPublication1,
		VolumeName: testVolume1,
		NodeName:   testNode1,
		ReadOnly:   false,
		AccessMode: 1,
	}

	testVolumePublication2 = models.VolumePublicationExternal{
		Name:       testPublication2,
		VolumeName: testVolume2,
		NodeName:   testNode2,
		ReadOnly:   true,
		AccessMode: 2,
	}

	basicTestPublications = []models.VolumePublicationExternal{testVolumePublication1}
	multiTestPublications = []models.VolumePublicationExternal{
		{Name: testPublication1, VolumeName: testVolume1, NodeName: testNode1},
		{Name: testPublication2, VolumeName: testVolume2, NodeName: testNode2},
	}
)

func restoreGlobalState(
	prevOperatingMode, prevGetVolume, prevGetNode string,
) func() {
	return func() {
		OperatingMode = prevOperatingMode
		getVolume = prevGetVolume
		getNode = prevGetNode
	}
}

func restoreOutputFormat(prevOutputFormat string) func() {
	return func() {
		OutputFormat = prevOutputFormat
	}
}

func setupSuccessfulPublicationMock(volumeName, nodeName, publicationName string) {
	url := BaseURL() + "/publication/" + volumeName + "/" + nodeName
	response := fmt.Sprintf(successJSONTemplate, publicationName, volumeName, nodeName)
	httpmock.RegisterResponder("GET", url, httpmock.NewStringResponder(httpStatusOK, response))
}

func setupVolumeListMock(volumeName string) {
	url := BaseURL() + "/volume/" + volumeName + "/publication"
	httpmock.RegisterResponder("GET", url, httpmock.NewStringResponder(httpStatusOK, emptyListJSONResponse))
}

func setupServerErrorMock(url string) {
	httpmock.RegisterResponder("GET", url, httpmock.NewStringResponder(httpStatusError, serverErrorJSONResponse))
}

func setupNetworkErrorMock(url string) {
	httpmock.RegisterResponder("GET", url, httpmock.NewErrorResponder(errors.New(errorNetworkError)))
}

func setupInvalidJSONMock(url string) {
	httpmock.RegisterResponder("GET", url, httpmock.NewStringResponder(httpStatusOK, invalidJSONResponse))
}

func TestGetPublicationCmd_RunE(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name          string
		operatingMode string
		args          []string
		getVolume     string
		getNode       string
		wantErr       bool
		errorContains string
		setupMocks    func()
	}{
		{
			name:          "tunnel mode with volume and node - failure",
			operatingMode: ModeTunnel,
			args:          []string{},
			getVolume:     testVolume1,
			getNode:       testNode1,
			wantErr:       true,
		},
		{
			name:          "tunnel mode with volume only - success",
			operatingMode: ModeTunnel,
			args:          []string{},
			getVolume:     testVolume1,
			getNode:       "",
			wantErr:       false,
		},
		{
			name:          "direct mode with volume only - success",
			operatingMode: ModeDirect,
			args:          []string{},
			getVolume:     testVolume1,
			getNode:       "",
			wantErr:       false,
			setupMocks: func() {
				setupVolumeListMock(testVolume1)
			},
		},
		{
			name:          "direct mode with volume and node - success",
			operatingMode: ModeDirect,
			args:          []string{},
			getVolume:     testVolume1,
			getNode:       testNode1,
			wantErr:       false,
			setupMocks: func() {
				setupSuccessfulPublicationMock(testVolume1, testNode1, testPublicationName)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			prevOperatingMode := OperatingMode
			prevGetVolume := getVolume
			prevGetNode := getNode
			defer restoreGlobalState(prevOperatingMode, prevGetVolume, prevGetNode)()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			prevExecKubernetesCLIRaw := execKubernetesCLIRaw
			defer func() {
				execKubernetesCLIRaw = prevExecKubernetesCLIRaw
			}()

			if tc.operatingMode == ModeTunnel {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					cmd := exec.Command("echo", mockOutput)
					if tc.wantErr {
						cmd = exec.Command("false")
					}
					return cmd
				}
			}

			OperatingMode = tc.operatingMode
			getVolume = tc.getVolume
			getNode = tc.getNode

			cmd := &cobra.Command{}
			err := getPublicationCmd.RunE(cmd, tc.args)

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

func TestGetVolumePublication(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name          string
		volumeName    string
		nodeName      string
		wantErr       bool
		errorContains string
		setupMocks    func()
	}{
		{
			name:       "successful publication retrieval",
			volumeName: testVolume1,
			nodeName:   testNode1,
			wantErr:    false,
			setupMocks: func() {
				setupSuccessfulPublicationMock(testVolume1, testNode1, testPublication1)
			},
		},
		{
			name:          "server error during retrieval",
			volumeName:    testVolume1,
			nodeName:      testNode1,
			wantErr:       true,
			errorContains: errorGetPublications,
			setupMocks: func() {
				url := BaseURL() + "/publication/" + testVolume1 + "/" + testNode1
				setupServerErrorMock(url)
			},
		},
		{
			name:       "network error during HTTP call",
			volumeName: testVolume1,
			nodeName:   testNode1,
			wantErr:    true,
			setupMocks: func() {
				url := BaseURL() + "/publication/" + testVolume1 + "/" + testNode1
				setupNetworkErrorMock(url)
			},
		},
		{
			name:       "invalid JSON response",
			volumeName: testVolume1,
			nodeName:   testNode1,
			wantErr:    true,
			setupMocks: func() {
				url := BaseURL() + "/publication/" + testVolume1 + "/" + testNode1
				setupInvalidJSONMock(url)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			result, err := GetVolumePublication(tc.volumeName, tc.nodeName)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tc.volumeName, result.VolumeName)
				assert.Equal(t, tc.nodeName, result.NodeName)
			}
		})
	}
}

func TestVolumePublicationGet(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name          string
		getVolume     string
		getNode       string
		wantErr       bool
		errorContains string
		setupMocks    func()
	}{
		{
			name:      "successful publication get",
			getVolume: testVolume1,
			getNode:   testNode1,
			wantErr:   false,
			setupMocks: func() {
				setupSuccessfulPublicationMock(testVolume1, testNode1, testPublication1)
			},
		},
		{
			name:          "error getting publication",
			getVolume:     testVolume1,
			getNode:       testNode1,
			wantErr:       true,
			errorContains: errorGetPublications,
			setupMocks: func() {
				url := BaseURL() + "/publication/" + testVolume1 + "/" + testNode1
				httpmock.RegisterResponder("GET", url, httpmock.NewStringResponder(httpStatusError, customServerErrorResponse))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			prevGetVolume := getVolume
			prevGetNode := getNode
			defer func() {
				getVolume = prevGetVolume
				getNode = prevGetNode
			}()

			getVolume = tc.getVolume
			getNode = tc.getNode

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			err := volumePublicationGet()

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

func TestVolumePublicationsList(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	baseURL := BaseURL() + "/publication"
	volumeURL := BaseURL() + "/volume/" + testVolume1 + "/publication"
	nodeURL := BaseURL() + "/node/" + testNode1 + "/publication"

	successResponse := fmt.Sprintf(successListJSONTemplate, testPublication1, testVolume1, testNode1, false, 1)

	testCases := []struct {
		name          string
		getVolume     string
		getNode       string
		wantErr       bool
		errorContains string
		setupMocks    func()
		expectedURL   string
	}{
		{
			name:        "list all publications",
			getVolume:   "",
			getNode:     "",
			wantErr:     false,
			expectedURL: baseURL,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", baseURL, httpmock.NewStringResponder(httpStatusOK, successResponse))
			},
		},
		{
			name:        "list publications by volume",
			getVolume:   testVolume1,
			getNode:     "",
			wantErr:     false,
			expectedURL: volumeURL,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", volumeURL, httpmock.NewStringResponder(httpStatusOK, successResponse))
			},
		},
		{
			name:        "list publications by node",
			getVolume:   "",
			getNode:     testNode1,
			wantErr:     false,
			expectedURL: nodeURL,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", nodeURL, httpmock.NewStringResponder(httpStatusOK, successResponse))
			},
		},
		{
			name:          "server error during retrieval",
			getVolume:     "",
			getNode:       "",
			wantErr:       true,
			errorContains: errorGetPublications,
			setupMocks: func() {
				setupServerErrorMock(baseURL)
			},
		},
		{
			name:      "network error during HTTP call",
			getVolume: "",
			getNode:   "",
			wantErr:   true,
			setupMocks: func() {
				setupNetworkErrorMock(baseURL)
			},
		},
		{
			name:      "invalid JSON response",
			getVolume: "",
			getNode:   "",
			wantErr:   true,
			setupMocks: func() {
				setupInvalidJSONMock(baseURL)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			prevGetVolume := getVolume
			prevGetNode := getNode
			defer func() {
				getVolume = prevGetVolume
				getNode = prevGetNode
			}()

			getVolume = tc.getVolume
			getNode = tc.getNode

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			cmd := &cobra.Command{}
			err := volumePublicationsList(cmd)

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

func TestWriteVolumePublications(t *testing.T) {
	testCases := []struct {
		name         string
		outputFormat string
	}{
		{
			name:         "JSON format",
			outputFormat: FormatJSON,
		},
		{
			name:         "YAML format",
			outputFormat: FormatYAML,
		},
		{
			name:         "Name format",
			outputFormat: FormatName,
		},
		{
			name:         "Wide format",
			outputFormat: FormatWide,
		},
		{
			name:         "Default format",
			outputFormat: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prevOutputFormat := OutputFormat
			defer restoreOutputFormat(prevOutputFormat)()

			OutputFormat = tc.outputFormat

			assert.NotPanics(t, func() {
				WriteVolumePublications(basicTestPublications)
			})
		})
	}
}
