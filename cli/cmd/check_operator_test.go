// Copyright 2024 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"os"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/cli/api"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
	execCmd "github.com/netapp/trident/utils/exec"
)

const testNamespace = "test-namespace"

func TestCheckOperatorStatusRunE(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	prevOperatingMode := OperatingMode

	defer func(previousCommand execCmd.Command) {
		command = previousCommand
		OperatingMode = prevOperatingMode
	}(command)

	command = mockCommand

	OperatingMode = ModeDirect
	_ = os.Unsetenv(podNamespace)
	err := checkOperatorStatusRunE(&cobra.Command{}, nil)
	assert.Error(t, err, "expected error when pod namespace is not set")
}

func TestCheckOperatorStatus_Success(t *testing.T) {
	origOut := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = origOut }()

	_ = os.Setenv(podNamespace, testNamespace)
	url := getOperatorStatusURL(testNamespace)

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", url,
		httpmock.NewStringResponder(200, `{"operatorStatus":"Done"}`))

	err := checkOperatorStatus(1)

	assert.NoError(t, err, "failed to check operator status")

	_ = os.Unsetenv(podNamespace)
}

func TestCheckOperatorStatus_Failure(t *testing.T) {
	origOut := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	defer func() { os.Stdout = origOut }()

	_ = os.Setenv(podNamespace, testNamespace)
	url := getOperatorStatusURL(testNamespace)

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", url,
		httpmock.NewStringResponder(200, `{"operatorStatus":"NotDone"}`))

	err := checkOperatorStatus(1)

	assert.Error(t, err, "expected error when operator status is not done")

	_ = os.Unsetenv(podNamespace)
}

func TestCheckOperatorStatus_RequestError(t *testing.T) {
	_ = os.Setenv(podNamespace, testNamespace)
	url := getOperatorStatusURL(testNamespace)

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", url,
		httpmock.NewErrorResponder(errors.New("random error")))

	err := checkOperatorStatus(1)

	assert.Error(t, err, "expected error when HTTP REST request fails")

	_ = os.Unsetenv(podNamespace)
}

func TestWriteStatus(t *testing.T) {
	origFormat := OutputFormat
	defer func() {
		OutputFormat = origFormat
	}()

	tests := []struct {
		name         string
		outputFormat string
		status       api.OperatorStatus
		contains     []string
	}{
		{
			name:         "JSONFormat",
			outputFormat: FormatJSON,
			status:       api.OperatorStatus{Status: "Done"},
			contains:     []string{"\"operatorStatus\": \"Done\""},
		},
		{
			name:         "YAMLFormat",
			outputFormat: FormatYAML,
			status:       api.OperatorStatus{Status: "Done"},
			contains:     []string{"operatorStatus: Done"},
		},
		{
			name:         "WideFormat",
			outputFormat: FormatWide,
			status: api.OperatorStatus{
				Status: "Done",
				TorcStatus: map[string]api.CRStatus{
					"torc1": {Status: "Done", Message: "Done"},
				},
				TconfStatus: map[string]api.CRStatus{
					"tconf1": {Status: "Done", Message: "Done"},
				},
			},
			contains: []string{"DONE", "Done", "torc1", "tconf1"},
		},
		{
			name:         "WideFormatError",
			outputFormat: FormatWide,
			status: api.OperatorStatus{
				Status:       "Failed",
				ErrorMessage: "Error",
			},
			contains: []string{"FAILED", "ERROR"},
		},
		{
			name:         "DefaultFormat",
			outputFormat: "",
			status: api.OperatorStatus{
				Status: "Done",
				TorcStatus: map[string]api.CRStatus{
					"torc1": {Status: "Done"},
				},
				TconfStatus: map[string]api.CRStatus{
					"tconf1": {Status: "Done"},
				},
			},
			contains: []string{"DONE", "Done", "torc1", "tconf1"},
		},
		{
			name:         "DefaultFormatError",
			outputFormat: "",
			status: api.OperatorStatus{
				Status:       "Failed",
				ErrorMessage: "Error",
			},
			contains: []string{"FAILED", "ERROR"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			OutputFormat = test.outputFormat

			output := captureOutput(t, func() {
				writeStatus(test.status)
			})

			for _, c := range test.contains {
				assert.Contains(t, output, c, "unexpected output")
			}
		})
	}
}
