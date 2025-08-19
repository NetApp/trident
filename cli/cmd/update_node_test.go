// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

var updateNodeTestMutex sync.RWMutex

const (
	testUpdateNodeName1 = "node1"
	testUpdateNodeName2 = "node2"
	testUpdateNodeIQN   = "iqn.1993-08.org.debian:01:node1"
	testUpdateNodeIP    = "192.168.1.1"
)

type updateNodeTestFlags struct {
	orchestratorReady  bool
	administratorReady bool
	provisionerReady   bool
	forceUpdate        bool
}

func withUpdateNodeTestMode(operatingMode string, flags updateNodeTestFlags, fn func()) {
	updateNodeTestMutex.Lock()
	defer updateNodeTestMutex.Unlock()

	oldOperatingMode := OperatingMode
	oldForceUpdate := forceUpdate
	oldOrchReady := orchestratorReady
	oldAdminReady := administratorReady
	oldProvReady := provisionerReady

	OperatingMode = operatingMode
	forceUpdate = flags.forceUpdate
	orchestratorReady = flags.orchestratorReady
	administratorReady = flags.administratorReady
	provisionerReady = flags.provisionerReady

	defer func() {
		OperatingMode = oldOperatingMode
		forceUpdate = oldForceUpdate
		orchestratorReady = oldOrchReady
		administratorReady = oldAdminReady
		provisionerReady = oldProvReady
	}()

	fn()
}

func setupUpdateNodeHTTPMocks(nodeName string, success bool,
	validateRequest bool,
) func() (*models.NodePublicationStateFlags, error) {
	httpmock.Reset()

	var capturedRequest *models.NodePublicationStateFlags
	var captureError error

	putURL := fmt.Sprintf("%s/node/%s/publicationState", BaseURL(), nodeName)

	if success {
		httpmock.RegisterResponder("PUT", putURL, func(req *http.Request) (*http.Response, error) {
			if validateRequest {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					captureError = err
					return httpmock.NewStringResponse(500, `{"error":"read error"}`), nil
				}

				var nodeFlags models.NodePublicationStateFlags
				if err := json.Unmarshal(body, &nodeFlags); err != nil {
					captureError = err
					return httpmock.NewStringResponse(400, `{"error":"invalid json"}`), nil
				}
				capturedRequest = &nodeFlags
			}

			return httpmock.NewStringResponse(200, fmt.Sprintf(`{"name":"%s"}`, nodeName)), nil
		})

		getURL := fmt.Sprintf("%s/node/%s", BaseURL(), nodeName)
		httpmock.RegisterResponder("GET", getURL, httpmock.NewStringResponder(200,
			fmt.Sprintf(`{"node": {"name": "%s", "iqn": "%s", "ips": ["%s"], "publicationState": "online"}}`,
				nodeName, testUpdateNodeIQN, testUpdateNodeIP)))
	} else {
		httpmock.RegisterResponder("PUT", putURL, httpmock.NewErrorResponder(errors.New("node update failed")))
	}

	return func() (*models.NodePublicationStateFlags, error) {
		return capturedRequest, captureError
	}
}

func createUpdateNodeCommand(setOrchFlag, setAdminFlag, setProvFlag bool,
	orchReady, adminReady, provReady bool,
) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("orchestratorReady", false, "")
	cmd.Flags().Bool("administratorReady", false, "")
	cmd.Flags().Bool("provisionerReady", false, "")

	if setOrchFlag {
		cmd.Flags().Set("orchestratorReady", fmt.Sprintf("%t", orchReady))
	}
	if setAdminFlag {
		cmd.Flags().Set("administratorReady", fmt.Sprintf("%t", adminReady))
	}
	if setProvFlag {
		cmd.Flags().Set("provisionerReady", fmt.Sprintf("%t", provReady))
	}

	return cmd
}

func TestUpdateNodeCmd_RunE(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name              string
		operatingMode     string
		args              []string
		flags             updateNodeTestFlags
		setOrchFlag       bool
		setAdminFlag      bool
		setProvFlag       bool
		nodeUpdateSuccess bool
		wantErr           bool
		errorContains     string
		validateRequest   bool
		expectedFlags     *models.NodePublicationStateFlags
	}{
		{
			name:              "direct_mode_success_all_flags",
			operatingMode:     ModeDirect,
			args:              []string{testUpdateNodeName1},
			flags:             updateNodeTestFlags{orchestratorReady: true, administratorReady: false, provisionerReady: true},
			setOrchFlag:       true,
			setAdminFlag:      true,
			setProvFlag:       true,
			nodeUpdateSuccess: true,
			validateRequest:   true,
			expectedFlags: &models.NodePublicationStateFlags{
				OrchestratorReady:  boolPtr(true),
				AdministratorReady: boolPtr(false),
				ProvisionerReady:   boolPtr(true),
			},
		},
		{
			name:              "direct_mode_success_partial_flags",
			operatingMode:     ModeDirect,
			args:              []string{testUpdateNodeName2},
			flags:             updateNodeTestFlags{orchestratorReady: true},
			setOrchFlag:       true,
			setAdminFlag:      false,
			setProvFlag:       false,
			nodeUpdateSuccess: true,
			validateRequest:   true,
			expectedFlags: &models.NodePublicationStateFlags{
				OrchestratorReady:  boolPtr(true),
				AdministratorReady: nil,
				ProvisionerReady:   nil,
			},
		},
		{
			name:              "direct_mode_no_flags_set",
			operatingMode:     ModeDirect,
			args:              []string{"node3"},
			flags:             updateNodeTestFlags{},
			setOrchFlag:       false,
			setAdminFlag:      false,
			setProvFlag:       false,
			nodeUpdateSuccess: true,
			validateRequest:   true,
			expectedFlags: &models.NodePublicationStateFlags{
				OrchestratorReady:  nil,
				AdministratorReady: nil,
				ProvisionerReady:   nil,
			},
		},
		{
			name:          "tunnel_mode_force_true",
			operatingMode: ModeTunnel,
			args:          []string{"node4"},
			flags:         updateNodeTestFlags{forceUpdate: true, orchestratorReady: true},
			setOrchFlag:   true,
			setAdminFlag:  false,
			setProvFlag:   false,
			wantErr:       true,
			errorContains: "exec: no command",
		},
		{
			name:          "tunnel_mode_force_false",
			operatingMode: ModeTunnel,
			args:          []string{"node5"},
			flags:         updateNodeTestFlags{forceUpdate: false},
			setOrchFlag:   false,
			setAdminFlag:  false,
			setProvFlag:   false,
			wantErr:       true, // Will fail due to user confirmation prompt
		},
		{
			name:              "direct_mode_update_failure",
			operatingMode:     ModeDirect,
			args:              []string{"node6"},
			flags:             updateNodeTestFlags{orchestratorReady: true},
			setOrchFlag:       true,
			setAdminFlag:      false,
			setProvFlag:       false,
			nodeUpdateSuccess: false,
			wantErr:           true,
			errorContains:     "node update failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withUpdateNodeTestMode(tc.operatingMode, tc.flags, func() {
				var getCapture func() (*models.NodePublicationStateFlags, error)

				if tc.operatingMode == ModeDirect {
					getCapture = setupUpdateNodeHTTPMocks(tc.args[0], tc.nodeUpdateSuccess, tc.validateRequest)
				}

				cmd := createUpdateNodeCommand(tc.setOrchFlag, tc.setAdminFlag, tc.setProvFlag,
					tc.flags.orchestratorReady, tc.flags.administratorReady, tc.flags.provisionerReady)

				err := updateNodeCmd.RunE(cmd, tc.args)

				// Verify results
				if tc.wantErr {
					assert.Error(t, err)
					if tc.errorContains != "" {
						assert.Contains(t, err.Error(), tc.errorContains)
					}
				} else {
					assert.NoError(t, err)

					// Validate captured request if expected
					if tc.validateRequest && tc.expectedFlags != nil && getCapture != nil {
						capturedRequest, captureError := getCapture()
						assert.NoError(t, captureError, "Should not have error capturing request")
						assert.NotNil(t, capturedRequest, "Should have captured request")
						assert.Equal(t, tc.expectedFlags, capturedRequest, "Request flags should match expected")
					}
				}
			})
		})
	}
}

func TestNodeUpdate(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name               string
		nodeName           string
		orchestratorReady  *bool
		administratorReady *bool
		provisionerReady   *bool
		putStatus          int
		putResponse        string
		putError           error
		putHeaders         map[string]string
		getStatus          int
		getResponse        string
		getError           error
		wantErr            bool
		errorContains      string
		validateRequest    bool
	}{
		{
			name:               "success_with_mixed_flags",
			nodeName:           testUpdateNodeName1,
			orchestratorReady:  boolPtr(true),
			administratorReady: boolPtr(false),
			provisionerReady:   nil,
			putStatus:          200,
			putResponse:        fmt.Sprintf(`{"name":"%s"}`, testUpdateNodeName1),
			getStatus:          200,
			getResponse:        fmt.Sprintf(`{"node": {"name": "%s", "iqn": "%s", "ips": ["%s"], "publicationState": "online"}}`, testUpdateNodeName1, testUpdateNodeIQN, testUpdateNodeIP),
			validateRequest:    true,
		},
		{
			name:               "success_http_202_accepted",
			nodeName:           "node2",
			orchestratorReady:  boolPtr(false),
			administratorReady: boolPtr(true),
			provisionerReady:   boolPtr(true),
			putStatus:          202,
			putResponse:        `{"name":"node2"}`,
			getStatus:          200,
			getResponse:        `{"node": {"name": "node2", "iqn": "iqn.test", "ips": ["192.168.1.2"], "publicationState": "online"}}`,
		},
		{
			name:               "put_network_error",
			nodeName:           "node3",
			orchestratorReady:  boolPtr(true),
			administratorReady: boolPtr(true),
			provisionerReady:   boolPtr(true),
			putError:           errors.New("network error"),
			wantErr:            true,
			errorContains:      "network error",
		},
		{
			name:               "put_404_not_found",
			nodeName:           "node4",
			orchestratorReady:  boolPtr(true),
			administratorReady: boolPtr(true),
			provisionerReady:   boolPtr(true),
			putStatus:          404,
			putResponse:        `{"error":"not found"}`,
			wantErr:            true,
			errorContains:      "could not update node node4",
		},
		{
			name:               "put_429_rate_limit_with_retry_header",
			nodeName:           "node5",
			orchestratorReady:  boolPtr(true),
			administratorReady: boolPtr(true),
			provisionerReady:   boolPtr(true),
			putStatus:          429,
			putResponse:        `{"error":"rate limited"}`,
			putHeaders:         map[string]string{"Retry-After": "60"},
			wantErr:            true,
			errorContains:      "could not update node node5",
		},
		{
			name:               "put_500_server_error",
			nodeName:           "node6",
			orchestratorReady:  boolPtr(true),
			administratorReady: boolPtr(true),
			provisionerReady:   boolPtr(true),
			putStatus:          500,
			putResponse:        `{"error":"internal server error"}`,
			wantErr:            true,
			errorContains:      "could not update node node6",
		},
		{
			name:               "invalid_json_response_from_put",
			nodeName:           "node7",
			orchestratorReady:  boolPtr(true),
			administratorReady: boolPtr(true),
			provisionerReady:   boolPtr(true),
			putStatus:          200,
			putResponse:        `invalid json response`,
			wantErr:            true,
			errorContains:      "invalid character",
		},
		{
			name:               "get_node_error_after_successful_put",
			nodeName:           "node8",
			orchestratorReady:  boolPtr(true),
			administratorReady: boolPtr(true),
			provisionerReady:   boolPtr(true),
			putStatus:          200,
			putResponse:        `{"name":"node8"}`,
			getError:           errors.New("get node failed"),
			wantErr:            true,
			errorContains:      "get node failed",
		},
		{
			name:               "get_node_404_after_successful_put",
			nodeName:           "node9",
			orchestratorReady:  boolPtr(true),
			administratorReady: boolPtr(true),
			provisionerReady:   boolPtr(true),
			putStatus:          200,
			putResponse:        `{"name":"node9"}`,
			getStatus:          404,
			getResponse:        `{"error":"not found"}`,
			wantErr:            true,
			errorContains:      "could not get node node9",
		},
		{
			name:               "all_flags_nil",
			nodeName:           "node10",
			orchestratorReady:  nil,
			administratorReady: nil,
			provisionerReady:   nil,
			putStatus:          200,
			putResponse:        `{"name":"node10"}`,
			getStatus:          200,
			getResponse:        `{"node": {"name": "node10", "iqn": "iqn.test", "ips": ["192.168.1.10"], "publicationState": "online"}}`,
			validateRequest:    true,
		},
		{
			name:               "get_node_returns_nil_node",
			nodeName:           "node11",
			orchestratorReady:  boolPtr(true),
			administratorReady: boolPtr(true),
			provisionerReady:   boolPtr(true),
			putStatus:          200,
			putResponse:        `{"name":"node11"}`,
			getStatus:          200,
			getResponse:        `null`,
			wantErr:            true,
			errorContains:      "node was empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			var capturedRequestBody []byte
			var captureError error

			// Setup PUT responder with request capture
			putURL := fmt.Sprintf("%s/node/%s/publicationState", BaseURL(), tc.nodeName)
			if tc.putError != nil {
				httpmock.RegisterResponder("PUT", putURL, httpmock.NewErrorResponder(tc.putError))
			} else {
				httpmock.RegisterResponder("PUT", putURL, func(req *http.Request) (*http.Response, error) {
					if tc.validateRequest {
						body, err := io.ReadAll(req.Body)
						if err != nil {
							captureError = err
						} else {
							capturedRequestBody = body
						}
					}

					resp := &http.Response{
						StatusCode: tc.putStatus,
						Body:       httpmock.NewRespBodyFromString(tc.putResponse),
						Header:     make(http.Header),
					}

					for k, v := range tc.putHeaders {
						resp.Header.Set(k, v)
					}

					return resp, nil
				})
			}

			if tc.putError == nil && (tc.putStatus == 200 || tc.putStatus == 202) {
				getURL := fmt.Sprintf("%s/node/%s", BaseURL(), tc.nodeName)
				if tc.getError != nil {
					httpmock.RegisterResponder("GET", getURL, httpmock.NewErrorResponder(tc.getError))
				} else {
					httpmock.RegisterResponder("GET", getURL, httpmock.NewStringResponder(tc.getStatus, tc.getResponse))
				}
			}

			err := nodeUpdate(tc.nodeName, tc.orchestratorReady, tc.administratorReady, tc.provisionerReady)

			// Verify results
			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			if tc.validateRequest && !tc.wantErr && capturedRequestBody != nil {
				assert.NoError(t, captureError, "Should not have error capturing request")

				var capturedFlags models.NodePublicationStateFlags
				err := json.Unmarshal(capturedRequestBody, &capturedFlags)
				assert.NoError(t, err, "Should be able to unmarshal captured request")

				assert.Equal(t, tc.orchestratorReady, capturedFlags.OrchestratorReady)
				assert.Equal(t, tc.administratorReady, capturedFlags.AdministratorReady)
				assert.Equal(t, tc.provisionerReady, capturedFlags.ProvisionerReady)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
