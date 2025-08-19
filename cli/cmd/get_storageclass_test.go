// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/utils/errors"
)

const (
	testStorageClass1    = "sc1"
	testStorageClass2    = "sc2"
	testStorageClassTest = "test-sc"
	testNonexistent      = "nonexistent"
	testVersion          = "1.0"
	testBackend1         = "backend1"
	testBackend2         = "backend2"
	testBackend3         = "backend3"
	testPool1            = "pool1"
	testPool2            = "pool2"
	testPool3            = "pool3"
	testPool4            = "pool4"
	testPool5            = "pool5"
	testOntapNas         = "ontap-nas"
	testSolidFireSan     = "solidfire-san"
	testNfsProtocol      = "nfs"
	testIscsiProtocol    = "iscsi"
	testHighPerformance  = "high"
	testStdPerformance   = "standard"
	testSsdMedia         = "ssd"
	testHddMedia         = "hdd"
)

var (
	basicStorageClass1  api.StorageClass
	basicStorageClass2  api.StorageClass
	basicStorageClasses []api.StorageClass
)

func init() {
	basicStorageClass1 = api.StorageClass{}
	basicStorageClass1.Config.Version = testVersion
	basicStorageClass1.Config.Name = testStorageClass1
	basicStorageClass1.Config.Attributes = map[string]interface{}{
		"performance": testHighPerformance,
		"media":       testSsdMedia,
	}
	basicStorageClass1.Config.Pools = map[string][]string{
		testBackend1: {testPool1, testPool2},
	}
	basicStorageClass1.Config.AdditionalPools = map[string][]string{
		testBackend2: {testPool3},
	}
	basicStorageClass1.Storage = map[string]interface{}{
		"type":     testOntapNas,
		"protocol": testNfsProtocol,
	}

	basicStorageClass2 = api.StorageClass{}
	basicStorageClass2.Config.Version = testVersion
	basicStorageClass2.Config.Name = testStorageClass2
	basicStorageClass2.Config.Attributes = map[string]interface{}{
		"performance": testStdPerformance,
		"media":       testHddMedia,
	}
	basicStorageClass2.Config.Pools = map[string][]string{
		testBackend3: {testPool4, testPool5},
	}
	basicStorageClass2.Config.AdditionalPools = map[string][]string{}
	basicStorageClass2.Storage = map[string]interface{}{
		"type":     testSolidFireSan,
		"protocol": testIscsiProtocol,
	}

	basicStorageClasses = []api.StorageClass{basicStorageClass1, basicStorageClass2}
}

func getFakeStorageClassesData() []api.StorageClass {
	return basicStorageClasses
}

func setupSuccessfulStorageClassMock(className string, storageClass *api.StorageClass) {
	httpmock.RegisterResponder("GET", BaseURL()+"/storageclass/"+className,
		func(req *http.Request) (*http.Response, error) {
			response := api.GetStorageClassResponse{StorageClass: *storageClass}
			return httpmock.NewJsonResponse(200, response)
		})
}

func setupStorageClassListMock(classNames []string) {
	httpmock.RegisterResponder("GET", BaseURL()+"/storageclass",
		func(req *http.Request) (*http.Response, error) {
			response := rest.ListStorageClassesResponse{StorageClasses: classNames}
			return httpmock.NewJsonResponse(200, response)
		})
}

func redirectStorageClassStdout(t *testing.T) func() {
	origOut := os.Stdout
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	require.NoError(t, err)
	os.Stdout = devNull
	return func() {
		os.Stdout = origOut
		devNull.Close()
	}
}

func TestGetStorageClassCmd_Execution(t *testing.T) {
	originalMode := OperatingMode
	defer func() {
		OperatingMode = originalMode
	}()

	prevExecKubernetesCLIRaw := execKubernetesCLIRaw
	defer func() {
		execKubernetesCLIRaw = prevExecKubernetesCLIRaw
	}()

	storageClasses := getFakeStorageClassesData()

	testCases := []struct {
		name          string
		operatingMode string
		args          []string
		expectError   bool
		setupMocks    func()
		setupCommand  func()
	}{
		{
			name:          "tunnel mode success with no args",
			operatingMode: ModeTunnel,
			args:          []string{},
			expectError:   false,
			setupMocks: func() {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					return exec.Command("echo", "success")
				}
			},
		},
		{
			name:          "tunnel mode success with args",
			operatingMode: ModeTunnel,
			args:          []string{testStorageClass1, testStorageClass2},
			expectError:   false,
			setupMocks: func() {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					return exec.Command("echo", "success")
				}
			},
		},
		{
			name:          "tunnel mode error",
			operatingMode: ModeTunnel,
			args:          []string{testStorageClass1},
			expectError:   true,
			setupMocks: func() {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					return exec.Command("false")
				}
			},
		},
		{
			name:          "direct mode success with specific storage class",
			operatingMode: ModeDirect,
			args:          []string{testStorageClassTest},
			expectError:   false,
			setupCommand: func() {
				httpmock.Activate()
			},
			setupMocks: func() {
				setupSuccessfulStorageClassMock(testStorageClassTest, &storageClasses[0])
			},
		},
		{
			name:          "direct mode success with no args",
			operatingMode: ModeDirect,
			args:          []string{},
			expectError:   false,
			setupCommand: func() {
				httpmock.Activate()
			},
			setupMocks: func() {
				setupStorageClassListMock([]string{testStorageClass1, testStorageClass2})
				setupSuccessfulStorageClassMock(testStorageClass1, &storageClasses[0])
				setupSuccessfulStorageClassMock(testStorageClass2, &storageClasses[1])
			},
		},
		{
			name:          "direct mode error - storage class not found",
			operatingMode: ModeDirect,
			args:          []string{testNonexistent},
			expectError:   true,
			setupCommand: func() {
				httpmock.Activate()
			},
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass/"+testNonexistent,
					httpmock.NewStringResponder(404, `{"error":"not found"}`))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.operatingMode == ModeDirect {
				httpmock.DeactivateAndReset()
			}

			OperatingMode = tc.operatingMode

			if tc.setupCommand != nil {
				tc.setupCommand()
			}

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			cleanup := redirectStorageClassStdout(t)
			defer cleanup()

			cmd := &cobra.Command{}
			err := getStorageClassCmd.RunE(cmd, tc.args)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Cleanup for direct mode tests
			if tc.operatingMode == ModeDirect {
				httpmock.DeactivateAndReset()
			}
		})
	}
}

func TestStorageClassListFunction(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	storageClasses := getFakeStorageClassesData()

	testCases := []struct {
		name              string
		storageClassNames []string
		wantErr           bool
		setupMocks        func()
		errorCheck        func(error) bool
	}{
		{
			name:              "get specific storage classes",
			storageClassNames: []string{testStorageClass1},
			wantErr:           false,
			setupMocks: func() {
				setupSuccessfulStorageClassMock(testStorageClass1, &storageClasses[0])
			},
		},
		{
			name:              "get all storage classes",
			storageClassNames: []string{},
			wantErr:           false,
			setupMocks: func() {
				setupStorageClassListMock([]string{testStorageClass1, testStorageClass2})
				setupSuccessfulStorageClassMock(testStorageClass1, &storageClasses[0])
				setupSuccessfulStorageClassMock(testStorageClass2, &storageClasses[1])
			},
		},
		{
			name:              "error getting storage class list",
			storageClassNames: []string{},
			wantErr:           true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass",
					httpmock.NewErrorResponder(errors.New("api error")))
			},
		},
		{
			name:              "error getting specific storage class",
			storageClassNames: []string{testNonexistent},
			wantErr:           true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass/"+testNonexistent,
					httpmock.NewStringResponder(404, `{"error":"not found"}`))
			},
		},
		{
			name:              "skip not found when getting all",
			storageClassNames: []string{},
			wantErr:           false,
			setupMocks: func() {
				setupStorageClassListMock([]string{testStorageClass1, testNonexistent})
				setupSuccessfulStorageClassMock(testStorageClass1, &storageClasses[0])
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass/"+testNonexistent,
					httpmock.NewStringResponder(404, `{"error":"not found"}`))
			},
		},
		{
			name:              "other error getting specific storage class",
			storageClassNames: []string{testStorageClass1},
			wantErr:           true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass/"+testStorageClass1,
					httpmock.NewErrorResponder(errors.New("other error")))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()
			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			cleanup := redirectStorageClassStdout(t)
			defer cleanup()

			err := storageClassList(tc.storageClassNames)

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

func TestGetStorageClassesFunction(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name         string
		wantErr      bool
		setupMocks   func()
		expectedList []string
	}{
		{
			name: "successful get storage classes",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass",
					func(req *http.Request) (*http.Response, error) {
						response := rest.ListStorageClassesResponse{
							StorageClasses: []string{"sc1", "sc2"},
						}
						return httpmock.NewJsonResponse(200, response)
					})
			},
			expectedList: []string{"sc1", "sc2"},
		},
		{
			name:    "network error",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:    "HTTP error response",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass",
					httpmock.NewStringResponder(500, `{"error":"server error"}`))
			},
		},
		{
			name:    "invalid JSON response",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass",
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

			result, err := GetStorageClasses()

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedList, result)
			}
		})
	}
}

func TestGetStorageClassFunction(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	storageClasses := getFakeStorageClassesData()

	testCases := []struct {
		name             string
		storageClassName string
		wantErr          bool
		setupMocks       func()
		isNotFound       bool
	}{
		{
			name:             "successful get storage class",
			storageClassName: "sc1",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass/sc1",
					func(req *http.Request) (*http.Response, error) {
						response := api.GetStorageClassResponse{StorageClass: storageClasses[0]}
						return httpmock.NewJsonResponse(200, response)
					})
			},
		},
		{
			name:             "storage class not found",
			storageClassName: "nonexistent",
			wantErr:          true,
			isNotFound:       true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass/nonexistent",
					httpmock.NewStringResponder(404, `{"error":"not found"}`))
			},
		},
		{
			name:             "server error",
			storageClassName: "sc1",
			wantErr:          true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass/sc1",
					httpmock.NewStringResponder(500, `{"error":"server error"}`))
			},
		},
		{
			name:             "network error",
			storageClassName: "sc1",
			wantErr:          true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass/sc1",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:             "invalid JSON response",
			storageClassName: "sc1",
			wantErr:          true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/storageclass/sc1",
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

			result, err := GetStorageClass(tc.storageClassName)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.isNotFound {
					assert.True(t, errors.IsNotFoundError(err))
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.storageClassName, result.Config.Name)
			}
		})
	}
}

func TestWriteStorageClassesFunction(t *testing.T) {
	storageClasses := getFakeStorageClassesData()

	testCases := []struct {
		name           string
		outputFormat   string
		storageClasses []api.StorageClass
		contains       []string
	}{
		{
			name:           "JSON format",
			outputFormat:   FormatJSON,
			storageClasses: storageClasses,
			contains:       []string{"sc1", "sc2"},
		},
		{
			name:           "YAML format",
			outputFormat:   FormatYAML,
			storageClasses: storageClasses,
			contains:       []string{"sc1", "sc2"},
		},
		{
			name:           "Name format",
			outputFormat:   FormatName,
			storageClasses: storageClasses,
			contains:       []string{"sc1", "sc2"},
		},
		{
			name:           "Default format (table)",
			outputFormat:   "",
			storageClasses: storageClasses,
			contains:       []string{"NAME", "sc1", "sc2"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origFormat := OutputFormat
			defer func() {
				OutputFormat = origFormat
			}()

			OutputFormat = tc.outputFormat

			origOut := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			WriteStorageClasses(tc.storageClasses)

			w.Close()
			os.Stdout = origOut

			output, _ := io.ReadAll(r)
			outputStr := string(output)

			for _, expected := range tc.contains {
				assert.Contains(t, outputStr, expected)
			}
		})
	}
}

func TestWriteStorageClassTableFunction(t *testing.T) {
	storageClasses := getFakeStorageClassesData()

	origOut := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	writeStorageClassTable(storageClasses)

	w.Close()
	os.Stdout = origOut

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	assert.Contains(t, outputStr, "NAME")
	assert.Contains(t, outputStr, "sc1")
	assert.Contains(t, outputStr, "sc2")
}

func TestWriteStorageClassNamesFunction(t *testing.T) {
	storageClasses := getFakeStorageClassesData()

	origOut := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	writeStorageClassNames(storageClasses)

	w.Close()
	os.Stdout = origOut

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	// Verify output
	assert.Contains(t, outputStr, "sc1")
	assert.Contains(t, outputStr, "sc2")
}

func TestWriteStorageClassesFunction_AllFormats(t *testing.T) {
	storageClasses := getFakeStorageClassesData()

	testCases := []struct {
		name         string
		outputFormat string
		checkContent func(string) bool
	}{
		{
			name:         "JSON format validation",
			outputFormat: FormatJSON,
			checkContent: func(output string) bool {
				var parsed interface{}
				return json.Unmarshal([]byte(output), &parsed) == nil
			},
		},
		{
			name:         "YAML format validation",
			outputFormat: FormatYAML,
			checkContent: func(output string) bool {
				return strings.Contains(output, "name:") || strings.Contains(output, "sc1")
			},
		},
		{
			name:         "Name format validation",
			outputFormat: FormatName,
			checkContent: func(output string) bool {
				return strings.Contains(output, "sc1") && strings.Contains(output, "sc2")
			},
		},
		{
			name:         "Table format validation",
			outputFormat: "",
			checkContent: func(output string) bool {
				return strings.Contains(output, "NAME")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origFormat := OutputFormat
			defer func() {
				OutputFormat = origFormat
			}()

			OutputFormat = tc.outputFormat

			origOut := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			WriteStorageClasses(storageClasses)

			w.Close()
			os.Stdout = origOut

			output, _ := io.ReadAll(r)
			outputStr := string(output)

			if tc.checkContent != nil {
				assert.True(t, tc.checkContent(outputStr), "Output format validation failed for %s", tc.outputFormat)
			}
		})
	}
}
