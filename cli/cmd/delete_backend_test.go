package cmd

import (
	"errors"
	"os/exec"
	"sync"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/frontend/rest"
)

var testMutex sync.RWMutex

func withTestMode(t *testing.T, mode string, allBackendsFlag bool, fn func()) {
	t.Helper()

	testMutex.Lock()
	defer testMutex.Unlock()

	origMode := OperatingMode
	origAllBackends := allBackends

	defer func() {
		OperatingMode = origMode
		allBackends = origAllBackends
	}()

	OperatingMode = mode
	allBackends = allBackendsFlag

	fn()
}

func TestDeleteBackendCmd_RunE(t *testing.T) {
	cleanup := setupHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name          string
		operatingMode string
		args          []string
		allBackends   bool
		wantErr       bool
		setupMocks    func()
	}{
		{
			name:          "tunnel mode with specific backends - success",
			operatingMode: ModeTunnel,
			args:          []string{"backend1"},
			allBackends:   false,
			wantErr:       false,
			setupMocks:    func() {},
		},
		{
			name:          "direct mode with specific backends - success",
			operatingMode: ModeDirect,
			args:          []string{"backend1"},
			allBackends:   false,
			wantErr:       false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/backend/backend1",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:          "direct mode with all flag - success",
			operatingMode: ModeDirect,
			args:          []string{},
			allBackends:   true,
			wantErr:       false,
			setupMocks: func() {
				responder, err := httpmock.NewJsonResponder(200, rest.ListBackendsResponse{
					Backends: []string{"backend1"},
				})
				if err != nil {
					panic(err)
				}
				httpmock.RegisterResponder("GET", BaseURL()+"/backend", responder)
				httpmock.RegisterResponder("DELETE", BaseURL()+"/backend/backend1",
					httpmock.NewStringResponder(200, ""))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withTestMode(t, tc.operatingMode, tc.allBackends, func() {
				httpmock.Reset()

				if tc.setupMocks != nil {
					tc.setupMocks()
				}

				if tc.operatingMode == ModeTunnel {
					prevExecKubernetesCLIRaw := execKubernetesCLIRaw
					defer func() {
						execKubernetesCLIRaw = prevExecKubernetesCLIRaw
					}()

					execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
						return exec.Command("echo", "mock output")
					}
				}

				cmd := &cobra.Command{}
				err := deleteBackendCmd.RunE(cmd, tc.args)

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestBackendDelete(t *testing.T) {
	cleanup := setupHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name          string
		backendNames  []string
		allBackends   bool
		wantErr       bool
		errorContains string
		setupMocks    func()
	}{
		{
			name:         "delete specific backends successfully",
			backendNames: []string{"backend1", "backend2"},
			allBackends:  false,
			wantErr:      false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/backend/backend1",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/backend/backend2",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:         "delete all backends successfully",
			backendNames: []string{},
			allBackends:  true,
			wantErr:      false,
			setupMocks: func() {
				responder, err := httpmock.NewJsonResponder(200, rest.ListBackendsResponse{
					Backends: []string{"backend1", "backend2"},
				})
				if err != nil {
					panic(err)
				}
				httpmock.RegisterResponder("GET", BaseURL()+"/backend", responder)
				httpmock.RegisterResponder("DELETE", BaseURL()+"/backend/backend1",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/backend/backend2",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:          "all flag with specific backends error",
			backendNames:  []string{"backend1"},
			allBackends:   true,
			wantErr:       true,
			errorContains: "cannot use --all switch and specify individual backends",
			setupMocks:    func() {},
		},
		{
			name:          "no backends specified without all flag",
			backendNames:  []string{},
			allBackends:   false,
			wantErr:       true,
			errorContains: "backend name not specified",
			setupMocks:    func() {},
		},
		{
			name:         "get backends list failed when using all flag",
			backendNames: []string{},
			allBackends:  true,
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/backend",
					httpmock.NewStringResponder(500, `{"error": "server error"}`))
			},
		},
		{
			name:         "delete backend HTTP error",
			backendNames: []string{"backend1"},
			allBackends:  false,
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/backend/backend1",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:         "delete backend bad status code",
			backendNames: []string{"backend1"},
			allBackends:  false,
			wantErr:      true,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/backend/backend1",
					httpmock.NewStringResponder(404, `{"error": "backend not found"}`))
			},
		},
		{
			name:         "skip empty backend names",
			backendNames: []string{"backend1", "", "backend2"},
			allBackends:  false,
			wantErr:      false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/backend/backend1",
					httpmock.NewStringResponder(200, ""))
				httpmock.RegisterResponder("DELETE", BaseURL()+"/backend/backend2",
					httpmock.NewStringResponder(200, ""))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withTestMode(t, ModeDirect, tc.allBackends, func() {
				httpmock.Reset()

				if tc.setupMocks != nil {
					tc.setupMocks()
				}

				err := backendDelete(tc.backendNames)

				if tc.wantErr {
					assert.Error(t, err)
					if tc.errorContains != "" {
						assert.Contains(t, err.Error(), tc.errorContains)
					}
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}
