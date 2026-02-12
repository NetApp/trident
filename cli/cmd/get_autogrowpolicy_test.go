// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
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

var getAutogrowPolicyTestMutex sync.RWMutex

func withAutogrowPolicyTestMode(t *testing.T, mode string, fn func()) {
	t.Helper()

	getAutogrowPolicyTestMutex.Lock()
	defer getAutogrowPolicyTestMutex.Unlock()

	origMode := OperatingMode
	defer func() {
		OperatingMode = origMode
	}()

	OperatingMode = mode
	fn()
}

func getFakeAutogrowPolicies() []storage.AutogrowPolicyExternal {
	return []storage.AutogrowPolicyExternal{
		{
			Name:          "policy1",
			UsedThreshold: "80",
			GrowthAmount:  "20",
			MaxSize:       "1000",
			State:         storage.AutogrowPolicyStateSuccess,
			Volumes:       []string{"vol1", "vol2"},
			VolumeCount:   2,
		},
		{
			Name:          "policy2",
			UsedThreshold: "90",
			GrowthAmount:  "10",
			MaxSize:       "",
			State:         storage.AutogrowPolicyStateSuccess,
			Volumes:       []string{},
			VolumeCount:   0,
		},
		{
			Name:          "policy3",
			UsedThreshold: "85",
			GrowthAmount:  "",
			MaxSize:       "2000",
			State:         storage.AutogrowPolicyStateFailed,
			Volumes:       []string{"vol3"},
			VolumeCount:   1,
		},
	}
}

// Command Tests

func TestGetAutogrowPolicyCmd_RunE(t *testing.T) {
	// Test tunnel mode success
	t.Run("tunnel mode success", func(t *testing.T) {
		withAutogrowPolicyTestMode(t, ModeTunnel, func() {
			prevExecKubernetesCLIRaw := execKubernetesCLIRaw
			defer func() {
				execKubernetesCLIRaw = prevExecKubernetesCLIRaw
			}()

			// Mock execKubernetesCLIRaw for tunnel mode
			execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
				return exec.Command("echo", "success")
			}

			cmd := &cobra.Command{}
			err := getAutogrowPolicyCmd.RunE(cmd, []string{})
			assert.NoError(t, err)
		})
	})

	// Test tunnel mode with specific policy name
	t.Run("tunnel mode with args", func(t *testing.T) {
		withAutogrowPolicyTestMode(t, ModeTunnel, func() {
			prevExecKubernetesCLIRaw := execKubernetesCLIRaw
			defer func() {
				execKubernetesCLIRaw = prevExecKubernetesCLIRaw
			}()

			// Mock execKubernetesCLIRaw for tunnel mode
			execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
				return exec.Command("echo", "success")
			}

			cmd := &cobra.Command{}
			err := getAutogrowPolicyCmd.RunE(cmd, []string{"policy1"})
			assert.NoError(t, err)
		})
	})

	// Test direct mode success
	t.Run("direct mode success", func(t *testing.T) {
		withAutogrowPolicyTestMode(t, ModeDirect, func() {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			policies := getFakeAutogrowPolicies()

			// Mock GET /autogrowpolicy to return list of policies
			httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy",
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewJsonResponse(200, rest.ListAutogrowPoliciesResponse{
						AutogrowPolicies: []string{"policy1", "policy2"},
					})
				})

			// Mock GET /autogrowpolicy/{name} for individual policy details
			httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/policy1",
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewJsonResponse(200, rest.GetAutogrowPolicyResponse{
						AutogrowPolicy: policies[0],
					})
				})

			httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/policy2",
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewJsonResponse(200, rest.GetAutogrowPolicyResponse{
						AutogrowPolicy: policies[1],
					})
				})

			cmd := &cobra.Command{}
			err := getAutogrowPolicyCmd.RunE(cmd, []string{})
			assert.NoError(t, err)
		})
	})
}

// autogrowPolicyList Tests

func TestAutogrowPolicyList(t *testing.T) {
	testCases := []struct {
		name           string
		policyNames    []string
		wantErr        bool
		setupMocks     func()
		cleanupMocks   func()
		expectedErrMsg string
	}{
		{
			name:        "get all policies success",
			policyNames: []string{},
			wantErr:     false,
			setupMocks: func() {
				httpmock.Activate()

				policies := getFakeAutogrowPolicies()

				// Mock list endpoint
				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(200, rest.ListAutogrowPoliciesResponse{
							AutogrowPolicies: []string{"policy1", "policy2"},
						})
					})

				// Mock individual get endpoints
				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/policy1",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(200, rest.GetAutogrowPolicyResponse{
							AutogrowPolicy: policies[0],
						})
					})

				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/policy2",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(200, rest.GetAutogrowPolicyResponse{
							AutogrowPolicy: policies[1],
						})
					})
			},
			cleanupMocks: func() {
				httpmock.DeactivateAndReset()
			},
		},
		{
			name:        "get specific policies success",
			policyNames: []string{"policy1", "policy2"},
			wantErr:     false,
			setupMocks: func() {
				httpmock.Activate()

				policies := getFakeAutogrowPolicies()

				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/policy1",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(200, rest.GetAutogrowPolicyResponse{
							AutogrowPolicy: policies[0],
						})
					})

				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/policy2",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(200, rest.GetAutogrowPolicyResponse{
							AutogrowPolicy: policies[1],
						})
					})
			},
			cleanupMocks: func() {
				httpmock.DeactivateAndReset()
			},
		},
		{
			name:        "get all policies with list error",
			policyNames: []string{},
			wantErr:     true,
			setupMocks: func() {
				httpmock.Activate()

				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy",
					httpmock.NewStringResponder(500, "Internal Server Error"))
			},
			cleanupMocks: func() {
				httpmock.DeactivateAndReset()
			},
		},
		{
			name:        "get specific policy with error",
			policyNames: []string{"policy1"},
			wantErr:     true,
			setupMocks: func() {
				httpmock.Activate()

				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/policy1",
					httpmock.NewStringResponder(500, "Internal Server Error"))
			},
			cleanupMocks: func() {
				httpmock.DeactivateAndReset()
			},
		},
		{
			name:        "get all policies with one not found - should skip",
			policyNames: []string{},
			wantErr:     false,
			setupMocks: func() {
				httpmock.Activate()

				policies := getFakeAutogrowPolicies()

				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(200, rest.ListAutogrowPoliciesResponse{
							AutogrowPolicies: []string{"policy1", "policy2"},
						})
					})

				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/policy1",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(200, rest.GetAutogrowPolicyResponse{
							AutogrowPolicy: policies[0],
						})
					})

				// policy2 not found
				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/policy2",
					httpmock.NewStringResponder(404, "Not Found"))
			},
			cleanupMocks: func() {
				httpmock.DeactivateAndReset()
			},
		},
		{
			name:        "get specific policy not found - should error",
			policyNames: []string{"nonexistent"},
			wantErr:     true,
			setupMocks: func() {
				httpmock.Activate()

				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/nonexistent",
					httpmock.NewStringResponder(404, "Not Found"))
			},
			cleanupMocks: func() {
				httpmock.DeactivateAndReset()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setupMocks()
			defer tc.cleanupMocks()

			err := autogrowPolicyList(tc.policyNames)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// GetAutogrowPolicies Tests

func TestGetAutogrowPolicies(t *testing.T) {
	testCases := []struct {
		name             string
		mockResponse     interface{}
		mockStatusCode   int
		expectedPolicies []string
		wantErr          bool
	}{
		{
			name: "success",
			mockResponse: rest.ListAutogrowPoliciesResponse{
				AutogrowPolicies: []string{"policy1", "policy2", "policy3"},
			},
			mockStatusCode:   200,
			expectedPolicies: []string{"policy1", "policy2", "policy3"},
			wantErr:          false,
		},
		{
			name: "success empty list",
			mockResponse: rest.ListAutogrowPoliciesResponse{
				AutogrowPolicies: []string{},
			},
			mockStatusCode:   200,
			expectedPolicies: []string{},
			wantErr:          false,
		},
		{
			name:           "http error 500",
			mockResponse:   "Internal Server Error",
			mockStatusCode: 500,
			wantErr:        true,
		},
		{
			name:           "http error 404",
			mockResponse:   "Not Found",
			mockStatusCode: 404,
			wantErr:        true,
		},
		{
			name: "success single policy",
			mockResponse: rest.ListAutogrowPoliciesResponse{
				AutogrowPolicies: []string{"policy1"},
			},
			mockStatusCode:   200,
			expectedPolicies: []string{"policy1"},
			wantErr:          false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			if tc.mockStatusCode == 200 {
				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy",
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(tc.mockStatusCode, tc.mockResponse)
					})
			} else {
				httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy",
					httpmock.NewStringResponder(tc.mockStatusCode, tc.mockResponse.(string)))
			}

			policies, err := GetAutogrowPolicies()

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedPolicies, policies)
			}
		})
	}

	// Test network error
	t.Run("network error", func(t *testing.T) {
		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy",
			httpmock.NewErrorResponder(assert.AnError))

		_, err := GetAutogrowPolicies()
		assert.Error(t, err)
	})

	// Test invalid JSON unmarshal
	t.Run("invalid json unmarshal", func(t *testing.T) {
		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy",
			httpmock.NewStringResponder(200, "{invalid json}"))

		_, err := GetAutogrowPolicies()
		assert.Error(t, err)
	})
}

// GetAutogrowPolicy Tests

func TestGetAutogrowPolicy(t *testing.T) {
	policies := getFakeAutogrowPolicies()

	testCases := []struct {
		name           string
		policyName     string
		mockResponse   interface{}
		mockStatusCode int
		expectedPolicy storage.AutogrowPolicyExternal
		wantErr        bool
		expectNotFound bool
	}{
		{
			name:       "success",
			policyName: "policy1",
			mockResponse: rest.GetAutogrowPolicyResponse{
				AutogrowPolicy: policies[0],
			},
			mockStatusCode: 200,
			expectedPolicy: policies[0],
			wantErr:        false,
		},
		{
			name:           "not found",
			policyName:     "nonexistent",
			mockResponse:   "Not Found",
			mockStatusCode: 404,
			wantErr:        true,
			expectNotFound: true,
		},
		{
			name:           "internal server error",
			policyName:     "policy1",
			mockResponse:   "Internal Server Error",
			mockStatusCode: 500,
			wantErr:        true,
		},
		{
			name:           "bad request",
			policyName:     "policy1",
			mockResponse:   "Bad Request",
			mockStatusCode: 400,
			wantErr:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Activate()
			defer httpmock.DeactivateAndReset()

			url := BaseURL() + "/autogrowpolicy/" + tc.policyName

			if tc.mockStatusCode == 200 {
				httpmock.RegisterResponder("GET", url,
					func(req *http.Request) (*http.Response, error) {
						return httpmock.NewJsonResponse(tc.mockStatusCode, tc.mockResponse)
					})
			} else {
				httpmock.RegisterResponder("GET", url,
					httpmock.NewStringResponder(tc.mockStatusCode, tc.mockResponse.(string)))
			}

			policy, err := GetAutogrowPolicy(tc.policyName)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.expectNotFound {
					assert.True(t, errors.IsAutogrowPolicyNotFoundError(err), "Expected AutogrowPolicyNotFoundError")
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedPolicy, policy)
			}
		})
	}

	// Test network error
	t.Run("network error", func(t *testing.T) {
		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/policy1",
			httpmock.NewErrorResponder(assert.AnError))

		_, err := GetAutogrowPolicy("policy1")
		assert.Error(t, err)
	})

	// Test invalid JSON unmarshal
	t.Run("invalid json unmarshal", func(t *testing.T) {
		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/policy1",
			httpmock.NewStringResponder(200, "{invalid json}"))

		_, err := GetAutogrowPolicy("policy1")
		assert.Error(t, err)
	})
}

// WriteAutogrowPolicies Tests

func TestWriteAutogrowPolicies(t *testing.T) {
	policies := getFakeAutogrowPolicies()

	testCases := []struct {
		name         string
		outputFormat string
		policies     []storage.AutogrowPolicyExternal
	}{
		{
			name:         "json format",
			outputFormat: FormatJSON,
			policies:     policies,
		},
		{
			name:         "yaml format",
			outputFormat: FormatYAML,
			policies:     policies,
		},
		{
			name:         "name format",
			outputFormat: FormatName,
			policies:     policies,
		},
		{
			name:         "table format (default)",
			outputFormat: "",
			policies:     policies,
		},
		{
			name:         "wide format falls back to table",
			outputFormat: "wide",
			policies:     policies,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Set output format
			origFormat := OutputFormat
			OutputFormat = tc.outputFormat
			defer func() {
				OutputFormat = origFormat
			}()

			// Execute
			WriteAutogrowPolicies(tc.policies)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read output
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Basic validation that output was produced
			if tc.outputFormat == FormatJSON {
				var result api.MultipleAutogrowPolicyResponse
				err := json.Unmarshal([]byte(output), &result)
				assert.NoError(t, err, "JSON output should be valid")
				assert.Len(t, result.Items, len(tc.policies), "Should have correct number of items")
			} else if tc.outputFormat == FormatName {
				// Name format should contain policy names
				for _, policy := range tc.policies {
					assert.Contains(t, output, policy.Name)
				}
			} else {
				// Table format should have some output
				assert.NotEmpty(t, output)
			}
		})
	}
}

// writeAutogrowPolicyTable Tests

func TestWriteAutogrowPolicyTable(t *testing.T) {
	testCases := []struct {
		name     string
		policies []storage.AutogrowPolicyExternal
	}{
		{
			name:     "empty policies",
			policies: []storage.AutogrowPolicyExternal{},
		},
		{
			name:     "single policy",
			policies: getFakeAutogrowPolicies()[:1],
		},
		{
			name:     "multiple policies",
			policies: getFakeAutogrowPolicies(),
		},
		{
			name: "policy with empty maxSize",
			policies: []storage.AutogrowPolicyExternal{
				{
					Name:          "test-policy",
					UsedThreshold: "80",
					GrowthAmount:  "20",
					MaxSize:       "",
					VolumeCount:   5,
				},
			},
		},
		{
			name: "policy with empty growthAmount",
			policies: []storage.AutogrowPolicyExternal{
				{
					Name:          "test-policy",
					UsedThreshold: "90",
					GrowthAmount:  "",
					MaxSize:       "1000",
					VolumeCount:   3,
				},
			},
		},
		{
			name: "unsorted policies",
			policies: []storage.AutogrowPolicyExternal{
				{Name: "zebra", UsedThreshold: "80", GrowthAmount: "20", MaxSize: "1000", VolumeCount: 1},
				{Name: "alpha", UsedThreshold: "90", GrowthAmount: "10", MaxSize: "2000", VolumeCount: 2},
				{Name: "beta", UsedThreshold: "85", GrowthAmount: "15", MaxSize: "1500", VolumeCount: 3},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Execute
			writeAutogrowPolicyTable(tc.policies)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read output
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Verify output contains expected elements
			if len(tc.policies) > 0 {
				// Should contain headers
				assert.Contains(t, output, "NAME")
				assert.Contains(t, output, "USED THRESHOLD")
				assert.Contains(t, output, "GROWTH AMOUNT")

				// Should contain policy data
				for _, policy := range tc.policies {
					assert.Contains(t, output, policy.Name)
					assert.Contains(t, output, policy.UsedThreshold)
				}

				// Verify N/A replacement for empty MaxSize
				for _, policy := range tc.policies {
					if policy.MaxSize == "" {
						assert.Contains(t, output, "N/A")
					}
				}
			}
		})
	}
}

// writeAutogrowPolicyNames Tests

func TestWriteAutogrowPolicyNames(t *testing.T) {
	testCases := []struct {
		name     string
		policies []storage.AutogrowPolicyExternal
	}{
		{
			name:     "empty policies",
			policies: []storage.AutogrowPolicyExternal{},
		},
		{
			name:     "single policy",
			policies: getFakeAutogrowPolicies()[:1],
		},
		{
			name:     "multiple policies",
			policies: getFakeAutogrowPolicies(),
		},
		{
			name: "unsorted policies - should be sorted",
			policies: []storage.AutogrowPolicyExternal{
				{Name: "zebra"},
				{Name: "alpha"},
				{Name: "beta"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Execute
			writeAutogrowPolicyNames(tc.policies)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read output
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Verify output contains all policy names
			for _, policy := range tc.policies {
				assert.Contains(t, output, policy.Name+"\n")
			}
		})
	}
}

// Integration Tests

func TestGetAutogrowPolicyIntegration(t *testing.T) {
	t.Run("end to end workflow", func(t *testing.T) {
		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		policies := getFakeAutogrowPolicies()

		// Mock list endpoint
		httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy",
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewJsonResponse(200, rest.ListAutogrowPoliciesResponse{
					AutogrowPolicies: []string{"policy1", "policy2", "policy3"},
				})
			})

		// Mock individual get endpoints
		for i, policyName := range []string{"policy1", "policy2", "policy3"} {
			localIndex := i
			httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/"+policyName,
				func(req *http.Request) (*http.Response, error) {
					return httpmock.NewJsonResponse(200, rest.GetAutogrowPolicyResponse{
						AutogrowPolicy: policies[localIndex],
					})
				})
		}

		// Get all policies
		policyNames, err := GetAutogrowPolicies()
		require.NoError(t, err)
		assert.Len(t, policyNames, 3)

		// Get individual policies
		for _, name := range policyNames {
			policy, err := GetAutogrowPolicy(name)
			require.NoError(t, err)
			assert.NotEmpty(t, policy.Name)
		}

		// Write policies
		allPolicies := make([]storage.AutogrowPolicyExternal, 0)
		for _, name := range policyNames {
			policy, _ := GetAutogrowPolicy(name)
			allPolicies = append(allPolicies, policy)
		}

		// Capture output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		OutputFormat = FormatJSON
		WriteAutogrowPolicies(allPolicies)

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		io.Copy(&buf, r)
		output := buf.String()

		var result api.MultipleAutogrowPolicyResponse
		err = json.Unmarshal([]byte(output), &result)
		assert.NoError(t, err)
		assert.Len(t, result.Items, 3)
	})
}

// Edge Cases and Error Handling Tests

func TestGetAutogrowPolicyEdgeCases(t *testing.T) {
	t.Run("empty policy name", func(t *testing.T) {
		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/",
			httpmock.NewStringResponder(400, "Bad Request"))

		_, err := GetAutogrowPolicy("")
		assert.Error(t, err)
	})

	t.Run("policy name with special characters", func(t *testing.T) {
		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		specialName := "policy-with-special_chars.123"
		httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy/"+specialName,
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewJsonResponse(200, rest.GetAutogrowPolicyResponse{
					AutogrowPolicy: storage.AutogrowPolicyExternal{
						Name:          specialName,
						UsedThreshold: "80",
						GrowthAmount:  "20",
						MaxSize:       "1000",
					},
				})
			})

		policy, err := GetAutogrowPolicy(specialName)
		assert.NoError(t, err)
		assert.Equal(t, specialName, policy.Name)
	})

	t.Run("multiple policies with same prefix", func(t *testing.T) {
		httpmock.Activate()
		defer httpmock.DeactivateAndReset()

		httpmock.RegisterResponder("GET", BaseURL()+"/autogrowpolicy",
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewJsonResponse(200, rest.ListAutogrowPoliciesResponse{
					AutogrowPolicies: []string{"policy", "policy-1", "policy-2"},
				})
			})

		policies, err := GetAutogrowPolicies()
		assert.NoError(t, err)
		assert.Len(t, policies, 3)
	})
}
