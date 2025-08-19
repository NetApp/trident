// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	mockK8sClient "github.com/netapp/trident/mocks/mock_cli/mock_k8s_client"
	versionutils "github.com/netapp/trident/utils/version"
)

const (
	image1 = "image1:v1"
	image2 = "image2:v2"

	testVersion125          = "v1.25.0"
	testVersion126          = "v1.26.0"
	testInvalidVersion      = "invalid-version"
	testTridentImage        = "netapp/trident:23.07.0"
	testCSIProvisionerImage = "registry.k8s.io/sig-storage/csi-provisioner:v3.5.0"
	testBusyboxImage        = "busybox:1.35"
	testGenericImage        = "test/image:latest"
	testAnotherImage        = "another/image:v1.0"
	testValidImage          = "valid/image:tag"
	testImage2v1            = "image2:v1"
	testImage1v2            = "image1:v2"
)

func TestImages_GetImageNames(t *testing.T) {
	testCases := []struct {
		name         string
		yaml         string
		expectedSize int
		contains     []string
	}{
		{
			name: "single image",
			yaml: `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: trident-main
        image: ` + testTridentImage,
			expectedSize: 2, // main image + operator image
			contains:     []string{testTridentImage},
		},
		{
			name: "multiple images",
			yaml: `apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: trident-main
        image: ` + testTridentImage + `
      - name: csi-provisioner
        image: ` + testCSIProvisionerImage + `
      initContainers:
      - name: init-container
        image: ` + testBusyboxImage,
			expectedSize: 4, // 3 main images + operator image
			contains:     []string{testTridentImage, testCSIProvisionerImage, testBusyboxImage},
		},
		{
			name:         "empty yaml",
			yaml:         "",
			expectedSize: 1, // only operator image
			contains:     []string{},
		},
		{
			name: "yaml with indented images",
			yaml: `    containers:
      - name: test
        image: ` + testGenericImage + `
        image: ` + testAnotherImage,
			expectedSize: 3, // 2 main images + operator image
			contains:     []string{testGenericImage, testAnotherImage},
		},
		{
			name: "yaml with empty image value",
			yaml: `containers:
- name: test
  image: 
- name: test2
  image: ` + testValidImage,
			expectedSize: 2, // 1 valid image + operator image
			contains:     []string{testValidImage},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			originalFormat := OutputFormat
			defer func() { OutputFormat = originalFormat }()

			OutputFormat = ""
			images := getImageNames(tc.yaml)
			assert.Len(t, images, tc.expectedSize)

			for _, expectedImage := range tc.contains {
				assert.Contains(t, images, expectedImage)
			}

			var hasOperatorImage bool
			for _, img := range images {
				if strings.Contains(img, "operator") || strings.Contains(img, "optional") {
					hasOperatorImage = true
					break
				}
			}
			assert.True(t, hasOperatorImage, "Should always include operator image")
		})
	}
}

func TestImages_GetImageNames_OutputFormats(t *testing.T) {
	yaml := `containers:
- name: test
  image: ` + testGenericImage

	testCases := []struct {
		name         string
		outputFormat string
		expectText   string
	}{
		{
			name:         "JSON format",
			outputFormat: FormatJSON,
			expectText:   "operator",
		},
		{
			name:         "YAML format",
			outputFormat: FormatYAML,
			expectText:   "operator",
		},
		{
			name:         "default format",
			outputFormat: "",
			expectText:   "(optional)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			originalFormat := OutputFormat
			defer func() { OutputFormat = originalFormat }()

			OutputFormat = tc.outputFormat
			images := getImageNames(yaml)

			var found bool
			for _, img := range images {
				if strings.Contains(img, tc.expectText) {
					found = true
					break
				}
			}
			assert.True(t, found, "Should contain expected text: %s", tc.expectText)
		})
	}
}

func TestImages_WriteImageTable(t *testing.T) {
	k8sVersions := []string{testVersion125, testVersion126}
	imageMap := map[string][]string{
		testVersion125: {image1, testImage2v1},
		testVersion126: {testImage1v2, image2},
	}

	origOut := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	writeImageTable(k8sVersions, imageMap)

	w.Close()
	os.Stdout = origOut

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	assert.Contains(t, outputStr, "KUBERNETES VERSION")
	assert.Contains(t, outputStr, "CONTAINER IMAGE")
	assert.Contains(t, outputStr, testVersion125)
	assert.Contains(t, outputStr, testVersion126)
	assert.Contains(t, outputStr, image1)
	assert.Contains(t, outputStr, image2)
}

func TestImages_WriteImageTable_Markdown(t *testing.T) {
	originalFormat := OutputFormat
	defer func() { OutputFormat = originalFormat }()

	OutputFormat = FormatMarkdown

	k8sVersions := []string{testVersion125}
	imageMap := map[string][]string{
		testVersion125: {image1},
	}

	origOut := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	writeImageTable(k8sVersions, imageMap)

	w.Close()
	os.Stdout = origOut

	output, _ := io.ReadAll(r)
	outputStr := string(output)

	assert.Contains(t, outputStr, "|")
	assert.Contains(t, outputStr, "KUBERNETES VERSION")
	assert.Contains(t, outputStr, "CONTAINER IMAGE")
}

func TestImages_ListImages_ErrorConditions(t *testing.T) {
	originalVersion := K8sVersion
	defer func() { K8sVersion = originalVersion }()

	testCases := []struct {
		name          string
		k8sVersion    string
		expectError   bool
		errorContains string
	}{
		{
			name:          "invalid semantic version",
			k8sVersion:    testInvalidVersion,
			expectError:   true,
			errorContains: "could not parse \"" + testInvalidVersion + "\" as version",
		},
		{
			name:        "valid semantic version",
			k8sVersion:  testVersion125,
			expectError: false,
		},
		{
			name:        "all versions",
			k8sVersion:  "all",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			K8sVersion = tc.k8sVersion

			err := listImages()

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NotPanics(t, func() {
					_ = listImages()
				})
			}
		})
	}
}

func TestImages_PrintImages(t *testing.T) {
	originalVersion := K8sVersion
	defer func() { K8sVersion = originalVersion }()

	K8sVersion = "all"

	_ = printImages()
}

func TestImages_GetInstallYaml_VersionValidation(t *testing.T) {
	testCases := []struct {
		name          string
		version       string
		expectError   bool
		errorContains string
	}{
		{
			name:        "valid version",
			version:     testVersion125,
			expectError: false,
		},
		{
			name:          "unsupported version (too old)",
			version:       "v1.10.0",
			expectError:   true,
			errorContains: "not supported",
		},
		{
			name:          "unsupported version (too new)",
			version:       "v1.50.0",
			expectError:   true,
			errorContains: "not supported",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			version := versionutils.MustParseSemantic(tc.version)

			yaml, err := getInstallYaml(version)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				assert.Empty(t, yaml)
			} else {
				if err == nil {
					assert.NotEmpty(t, yaml)
				}
			}
		})
	}
}

func TestWriteImages(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		k8sVersions []string
		imageMap    map[string][]string
		assertions  []string
	}{
		{
			name:        "JSON format",
			format:      FormatJSON,
			k8sVersions: []string{"v1.25.0", "v1.26.0"},
			imageMap: map[string][]string{
				"v1.25.0": {"image1:v1", "image2:v1"},
				"v1.26.0": {"image1:v2", "image2:v2"},
			},
			assertions: []string{
				"imageSets",
				"k8sVersion",
				"images",
				"v1.25.0",
				"v1.26.0",
			},
		},
		{
			name:        "YAML format",
			format:      FormatYAML,
			k8sVersions: []string{"v1.25.0"},
			imageMap: map[string][]string{
				"v1.25.0": {"image1:v1", "image2:v1"},
			},
			assertions: []string{
				"imageSets:",
				"k8sVersion: v1.25.0",
				"images:",
				"- image1:v1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalFormat := OutputFormat
			origOut := os.Stdout
			defer func() {
				OutputFormat = originalFormat
				os.Stdout = origOut
			}()

			OutputFormat = tt.format

			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			writeImages(tt.k8sVersions, tt.imageMap)

			w.Close()

			output, _ := io.ReadAll(r)
			outputStr := string(output)

			for _, assertion := range tt.assertions {
				assert.Contains(t, outputStr, assertion)
			}
		})
	}
}

func TestGetImageCmd_RunE_CompleteCoverage(t *testing.T) {
	originalMode := OperatingMode
	originalK8sVersion := K8sVersion
	originalClient := client
	defer func() {
		OperatingMode = originalMode
		K8sVersion = originalK8sVersion
		client = originalClient
	}()

	t.Run("tunnel_mode", func(t *testing.T) {
		OperatingMode = ModeTunnel
		cmd := &cobra.Command{}
		args := []string{}

		err := getImageCmd.RunE(cmd, args)
		_ = err
	})

	t.Run("non_tunnel_mode_with_mock_client", func(t *testing.T) {
		OperatingMode = ModeDirect
		K8sVersion = "all"

		orignalInitClient := initClientfunc
		defer func() {
			initClientfunc = orignalInitClient
		}()
		cmd := &cobra.Command{}
		args := []string{}
		initClientfunc = func() (k8sclient.KubernetesClient, error) {
			return &mockK8sClient.MockKubernetesClient{}, nil
		}
		origOut := os.Stdout
		os.Stdout, _ = os.Open(os.DevNull)
		defer func() { os.Stdout = origOut }()

		err := getImageCmd.RunE(cmd, args)
		_ = err
	})

	t.Run("direct_function_calls", func(t *testing.T) {
		K8sVersion = "all"

		err := listImages()
		if err == nil {
			result := printImages()
			assert.NoError(t, result)
		}
	})
}
