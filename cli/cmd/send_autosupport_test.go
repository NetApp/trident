// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/utils/errors"
)

var autosupportTestMutex sync.RWMutex

func withAutosupportTestMode(t *testing.T, fn func()) {
	autosupportTestMutex.Lock()
	defer autosupportTestMutex.Unlock()
	fn()
}

func setupAutosupportGlobals(acceptAgreementVal bool, sinceVal time.Duration, operatingMode, namespace string) func() {
	prevAcceptAgreement := acceptAgreement
	prevSince := since
	prevOperatingMode := OperatingMode
	prevPodNamespace := TridentPodNamespace

	acceptAgreement = acceptAgreementVal
	since = sinceVal
	OperatingMode = operatingMode
	TridentPodNamespace = namespace

	return func() {
		acceptAgreement = prevAcceptAgreement
		since = prevSince
		OperatingMode = prevOperatingMode
		TridentPodNamespace = prevPodNamespace
	}
}

func buildMockPodResponse(includeAutosupport bool) string {
	containers := `[{"name": "trident"}`
	if includeAutosupport {
		containers += `,{"name": "` + config.DefaultAutosupportName + `"}`
	}
	containers += `]`
	return `{"items": [{"spec": {"containers": ` + containers + `}}]}`
}

func TestSendAutosupportCmd_Init(t *testing.T) {
	withAutosupportTestMode(t, func() {
		// Find the send autosupport command
		var sendCmd *cobra.Command
		for _, cmd := range RootCmd.Commands() {
			if cmd.Use == "send" {
				sendCmd = cmd
				break
			}
		}

		if sendCmd == nil {
			t.Skip("send command not found")
			return
		}

		var autosupportCmd *cobra.Command
		for _, cmd := range sendCmd.Commands() {
			if cmd.Use == "autosupport" {
				autosupportCmd = cmd
				break
			}
		}

		if autosupportCmd == nil {
			t.Skip("send autosupport command not found")
			return
		}

		// Test command properties
		assert.Equal(t, "autosupport", autosupportCmd.Use)
		assert.Equal(t, "Send an Autosupport archive to NetApp", autosupportCmd.Short)
		assert.Contains(t, autosupportCmd.Aliases, "a")
		assert.Contains(t, autosupportCmd.Aliases, "asup")

		// Test flags
		acceptFlag := autosupportCmd.PersistentFlags().Lookup("accept-agreement")
		assert.NotNil(t, acceptFlag)
		assert.Equal(t, "false", acceptFlag.DefValue)

		sinceFlag := autosupportCmd.PersistentFlags().Lookup("since")
		assert.NotNil(t, sinceFlag)
		assert.Equal(t, "24h0m0s", sinceFlag.DefValue)
	})
}

func TestSendAutosupportCmd_RunE(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	const (
		testNamespace = "trident"
	)

	tests := []struct {
		name               string
		acceptAgreement    bool
		since              time.Duration
		operatingMode      string
		autosupportPresent bool
		mockError          error
		httpStatus         int
		httpResponse       string
		wantErr            bool
		errorContains      string
	}{
		{
			name: "direct_mode_accepted_with_since", acceptAgreement: true, since: 48 * time.Hour,
			operatingMode: ModeDirect, httpStatus: 201, httpResponse: "created",
		},
		{
			name: "direct_mode_accepted_no_since", acceptAgreement: true, since: 0,
			operatingMode: ModeDirect, httpStatus: 201, httpResponse: "created",
		},
		{
			name: "direct_mode_http_error", acceptAgreement: true, since: 24 * time.Hour,
			operatingMode: ModeDirect, httpStatus: 500, httpResponse: "server error",
			wantErr: true, errorContains: "could not send autosupport",
		},
		{
			name: "tunnel_mode_autosupport_present", acceptAgreement: true, since: 24 * time.Hour,
			operatingMode: ModeTunnel, autosupportPresent: true, wantErr: false,
		},
		{
			name: "tunnel_mode_autosupport_not_present", acceptAgreement: true, since: 24 * time.Hour,
			operatingMode: ModeTunnel, autosupportPresent: false,
		},
		{
			name: "tunnel_mode_check_error", acceptAgreement: true, since: 24 * time.Hour,
			operatingMode: ModeTunnel, mockError: errors.New("kubectl error"),
			wantErr: true, errorContains: "exit status 1",
		},
		{
			name: "tunnel_mode_with_zero_since", acceptAgreement: true, since: 0,
			operatingMode: ModeTunnel, autosupportPresent: true,
		},
		{
			name: "direct_mode_large_since", acceptAgreement: true, since: 168 * time.Hour,
			operatingMode: ModeDirect, httpStatus: 201, httpResponse: "created",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withAutosupportTestMode(t, func() {
				restore := setupAutosupportGlobals(tt.acceptAgreement, tt.since, tt.operatingMode, testNamespace)
				defer restore()

				// Mock HTTP for direct mode
				if tt.operatingMode == ModeDirect {
					httpmock.Reset()
					url := BaseAutosupportURL() + "/collector/trident/trigger"
					if tt.since != 0 {
						url += "?since=" + tt.since.String()
					}
					httpmock.RegisterResponder("POST", url,
						httpmock.NewStringResponder(tt.httpStatus, tt.httpResponse))
				}

				// Mock for tunnel mode
				if tt.operatingMode == ModeTunnel {
					originalExec := execKubernetesCLIRaw
					defer func() { execKubernetesCLIRaw = originalExec }()

					execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
						if tt.mockError != nil {
							return exec.Command("false")
						}
						return exec.Command("echo", buildMockPodResponse(tt.autosupportPresent))
					}
				}

				// Execute
				cmd := &cobra.Command{}
				err := sendAutosupportCmd.RunE(cmd, []string{})

				// Verify
				if tt.wantErr {
					assert.Error(t, err)
					if tt.errorContains != "" {
						assert.Contains(t, err.Error(), tt.errorContains)
					}
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestTriggerAutosupport(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	tests := []struct {
		name          string
		since         string
		httpStatus    int
		httpResponse  string
		wantErr       bool
		errorContains string
		isNetworkErr  bool
	}{
		{
			name: "success_with_since", since: "48h", httpStatus: 201, httpResponse: "created",
		},
		{
			name: "success_without_since", since: "", httpStatus: 201, httpResponse: "created",
		},
		{
			name: "http_error", since: "24h", httpStatus: 500, httpResponse: "server error",
			wantErr: true, errorContains: "could not send autosupport",
		},
		{
			name: "network_error", since: "24h", wantErr: true, isNetworkErr: true,
		},
		{
			name: "empty_since_param", since: "", httpStatus: 201, httpResponse: "created",
		},
		{
			name: "malformed_response", since: "1h", httpStatus: 400, httpResponse: "bad request",
			wantErr: true, errorContains: "could not send autosupport",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpmock.Reset()

			url := BaseAutosupportURL() + "/collector/trident/trigger"
			if tt.since != "" {
				url += "?since=" + tt.since
			}

			if tt.isNetworkErr {
				httpmock.RegisterResponder("POST", url,
					httpmock.NewErrorResponder(errors.New("network error")))
			} else {
				httpmock.RegisterResponder("POST", url,
					httpmock.NewStringResponder(tt.httpStatus, tt.httpResponse))
			}

			err := triggerAutosupport(tt.since)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsAutosupportContainerPresent(t *testing.T) {
	tests := []struct {
		name           string
		namespace      string
		appLabel       string
		mockOutput     string
		mockError      error
		expectedResult bool
		wantErr        bool
		errorContains  string
	}{
		{
			name: "autosupport_present", namespace: "trident", appLabel: "app=trident",
			mockOutput: buildMockPodResponse(true), expectedResult: true,
		},
		{
			name: "autosupport_not_present", namespace: "trident", appLabel: "app=trident",
			mockOutput: buildMockPodResponse(false), expectedResult: false,
		},
		{
			name: "no_pods_found", namespace: "trident", appLabel: "app=trident",
			mockOutput: `{"items": []}`, wantErr: true,
			errorContains: "could not find a Trident pod",
		},
		{
			name: "multiple_pods_found", namespace: "trident", appLabel: "app=trident",
			mockOutput: `{
				"items": [
					{"spec": {"containers": [{"name": "trident"}]}},
					{"spec": {"containers": [{"name": "trident"}]}}
				]
			}`,
			wantErr: true, errorContains: "could not find a Trident pod",
		},
		{
			name: "command_error", namespace: "trident", appLabel: "app=trident",
			mockError: errors.New("kubectl error"), wantErr: true, errorContains: "exit status 1",
		},
		{
			name: "invalid_json", namespace: "trident", appLabel: "app=trident",
			mockOutput: "invalid json", wantErr: true, errorContains: "could not unmarshal",
		},
		{
			name: "empty_namespace", namespace: "", appLabel: "app=trident",
			mockError: errors.New("namespace error"), wantErr: true, errorContains: "exit status 1",
		},
		{
			name: "empty_containers", namespace: "trident", appLabel: "app=trident",
			mockOutput: `{"items": [{"spec": {"containers": []}}]}`, expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withAutosupportTestMode(t, func() {
				originalExec := execKubernetesCLIRaw
				defer func() { execKubernetesCLIRaw = originalExec }()

				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					expectedArgs := []string{
						"get", "pod", "-n", tt.namespace, "-l", tt.appLabel,
						"-o=json", "--field-selector=status.phase=Running",
					}
					assert.Equal(t, expectedArgs, args)

					if tt.mockError != nil {
						return exec.Command("false")
					}
					return exec.Command("echo", tt.mockOutput)
				}

				result, err := isAutosupportContainerPresent(tt.namespace, tt.appLabel)

				if tt.wantErr {
					assert.Error(t, err)
					if tt.errorContains != "" {
						assert.Contains(t, err.Error(), tt.errorContains)
					}
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.expectedResult, result)
				}
			})
		})
	}
}
