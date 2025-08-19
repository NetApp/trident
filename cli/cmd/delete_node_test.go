// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"os/exec"
	"sync"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/frontend/rest"
)

var nodeTestMutex sync.RWMutex

func withNodeTestMode(t *testing.T, mode string, allFlag bool, fn func()) {
	t.Helper()

	nodeTestMutex.Lock()
	defer nodeTestMutex.Unlock()

	origMode := OperatingMode
	origAllNodes := allNodes

	defer func() {
		OperatingMode = origMode
		allNodes = origAllNodes
	}()

	OperatingMode = mode
	allNodes = allFlag

	fn()
}

func TestDeleteNodeCmd_RunE(t *testing.T) {
	cleanup := setupHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name          string
		operatingMode string
		args          []string
		allNodes      bool
		wantErr       bool
		setupMocks    func()
	}{
		{
			name:          "tunnel mode success",
			operatingMode: ModeTunnel,
			args:          []string{"node1"},
			allNodes:      false,
			wantErr:       false,
		},
		{
			name:          "tunnel mode failure",
			operatingMode: ModeTunnel,
			args:          []string{"node1"},
			allNodes:      false,
			wantErr:       true,
		},
		{
			name:          "direct mode success",
			operatingMode: ModeDirect,
			args:          []string{"node1"},
			allNodes:      false,
			wantErr:       false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/node/node1",
					httpmock.NewStringResponder(200, ""))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withNodeTestMode(t, tc.operatingMode, tc.allNodes, func() {
				httpmock.Reset()

				if tc.operatingMode == ModeTunnel {
					prevExec := execKubernetesCLIRaw
					defer func() { execKubernetesCLIRaw = prevExec }()

					execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
						if tc.wantErr {
							return exec.Command("false")
						}
						return exec.Command("echo", "Node deleted successfully")
					}
				}

				if tc.setupMocks != nil {
					tc.setupMocks()
				}

				cmd := &cobra.Command{}
				err := deleteNodeCmd.RunE(cmd, tc.args)

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestNodeDelete(t *testing.T) {
	cleanup := setupHTTPMock(t)
	defer cleanup()

	testCases := []struct {
		name       string
		nodeNames  []string
		allNodes   bool
		wantErr    bool
		setupMocks func()
	}{
		{
			name:      "delete specific node",
			nodeNames: []string{"node1"},
			allNodes:  false,
			wantErr:   false,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/node/node1",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:      "delete all nodes",
			nodeNames: []string{},
			allNodes:  true,
			wantErr:   false,
			setupMocks: func() {
				responder, _ := httpmock.NewJsonResponder(200, rest.ListNodesResponse{
					Nodes: []string{"node1"},
				})
				httpmock.RegisterResponder("GET", BaseURL()+"/node", responder)
				httpmock.RegisterResponder("DELETE", BaseURL()+"/node/node1",
					httpmock.NewStringResponder(200, ""))
			},
		},
		{
			name:       "validation error with all flag and specific nodes",
			nodeNames:  []string{"node1"},
			allNodes:   true,
			wantErr:    true,
			setupMocks: func() {},
		},
		{
			name:       "no nodes specified without all flag",
			nodeNames:  []string{},
			allNodes:   false,
			wantErr:    true,
			setupMocks: func() {},
		},
		{
			name:      "get nodes list error",
			nodeNames: []string{},
			allNodes:  true,
			wantErr:   true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node",
					httpmock.NewStringResponder(500, `{"error": "server error"}`))
			},
		},
		{
			name:      "HTTP error response",
			nodeNames: []string{"node1"},
			allNodes:  false,
			wantErr:   true,
			setupMocks: func() {
				httpmock.RegisterResponder("DELETE", BaseURL()+"/node/node1",
					httpmock.NewStringResponder(404, `{"error": "node not found"}`))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			withNodeTestMode(t, ModeDirect, tc.allNodes, func() {
				err := nodeDelete(tc.nodeNames)

				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}
