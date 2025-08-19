package cmd

// Copyright 2025 NetApp, Inc. All Rights Reserved.

import (
	"os/exec"
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// Test getNodeCmd RunE function
func TestGetNodeCmd_RunE(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name          string
		operatingMode string
		setupMocks    func()
	}{
		{
			name:          "tunnel mode success",
			operatingMode: ModeTunnel,
			setupMocks: func() {
			},
		},
		{
			name:          "direct mode success",
			operatingMode: ModeDirect,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node",
					httpmock.NewStringResponder(200, `{"nodes": ["node1", "node2"]}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node1",
					httpmock.NewStringResponder(200, `{"node": {"name": "node1", "iqn": "iqn.1993-08.org.debian:01:node1", "ips": ["192.168.1.1"], "publicationState": "online"}}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node2",
					httpmock.NewStringResponder(200, `{"node": {"name": "node2", "iqn": "iqn.1993-08.org.debian:01:node2", "ips": ["192.168.1.2"], "publicationState": "online"}}`))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			// Store original values
			prevOperatingMode := OperatingMode
			prevExecKubernetesCLIRaw := execKubernetesCLIRaw

			defer func() {
				OperatingMode = prevOperatingMode
				execKubernetesCLIRaw = prevExecKubernetesCLIRaw
			}()

			OperatingMode = tc.operatingMode

			// Mock execKubernetesCLIRaw for tunnel mode
			if tc.operatingMode == ModeTunnel {
				execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
					return exec.Command("echo", "Nodes retrieved successfully")
				}
			}

			// Set up HTTP mocks for direct mode
			tc.setupMocks()

			cmd := &cobra.Command{}
			err := getNodeCmd.RunE(cmd, []string{})
			assert.NoError(t, err, "getNodeCmd.RunE should not return an error")
		})
	}
}

// Test nodeList function
func TestNodeList(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name          string
		nodeNames     []string
		wantErr       bool
		errorContains string
		setupMocks    func()
	}{
		{
			name:      "list specific nodes",
			nodeNames: []string{"node1", "node2"},
			wantErr:   false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node1",
					httpmock.NewStringResponder(200, `{"node": {"name": "node1", "iqn": "iqn.1993-08.org.debian:01:node1", "ips": ["192.168.1.1"], "publicationState": "online"}}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node2",
					httpmock.NewStringResponder(200, `{"node": {"name": "node2", "iqn": "iqn.1993-08.org.debian:01:node2", "ips": ["192.168.1.2"], "publicationState": "online"}}`))
			},
		},

		{
			name:      "list all nodes",
			nodeNames: []string{},
			wantErr:   false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node",
					httpmock.NewStringResponder(200, `{"nodes": ["node1", "node2"]}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node1",
					httpmock.NewStringResponder(200, `{"node": {"name": "node1", "iqn": "iqn.1993-08.org.debian:01:node1", "ips": ["192.168.1.1"], "publicationState": "online"}}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node2",
					httpmock.NewStringResponder(200, `{"node": {"name": "node2", "iqn": "iqn.1993-08.org.debian:01:node2", "ips": ["192.168.1.2"], "publicationState": "online"}}`))
			},
		},
		{
			name:      "error getting nodes list",
			nodeNames: []string{},
			wantErr:   true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:          "error getting specific node",
			nodeNames:     []string{"nonexistent-node"},
			wantErr:       true,
			errorContains: "could not get node",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node/nonexistent-node",
					httpmock.NewStringResponder(404, `{"error": "node not found"}`))
			},
		},
		{
			name:      "skip not found node when getting all",
			nodeNames: []string{},
			wantErr:   false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node",
					httpmock.NewStringResponder(200, `{"nodes": ["node1", "nonexistent-node", "node2"]}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node1",
					httpmock.NewStringResponder(200, `{"node": {"name": "node1", "iqn": "iqn.1993-08.org.debian:01:node1", "ips": ["192.168.1.1"], "publicationState": "online"}}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/node/nonexistent-node",
					httpmock.NewStringResponder(404, `{"error": "node not found"}`))
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node2",
					httpmock.NewStringResponder(200, `{"node": {"name": "node2", "iqn": "iqn.1993-08.org.debian:01:node2", "ips": ["192.168.1.2"], "publicationState": "online"}}`))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			err := nodeList(tc.nodeNames)

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

// Test GetNodes function
func TestGetNodes(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name          string
		wantErr       bool
		errorContains string
		setupMocks    func()
		expectedCount int
	}{
		{
			name:          "successful nodes retrieval",
			wantErr:       false,
			expectedCount: 2,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node",
					httpmock.NewStringResponder(200, `{"nodes": ["node1", "node2"]}`))
			},
		},
		{
			name:          "server error during retrieval",
			wantErr:       true,
			errorContains: "could not get nodes",
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node",
					httpmock.NewStringResponder(500, `{"error": "internal server error"}`))
			},
		},
		{
			name:    "network error during HTTP call",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:    "invalid JSON response",
			wantErr: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node",
					httpmock.NewStringResponder(200, `invalid json`))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			result, err := GetNodes()

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tc.expectedCount)
			}
		})
	}
}

// Test GetNode function
func TestGetNode(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	testCases := []struct {
		name          string
		nodeName      string
		wantErr       bool
		errorContains string
		setupMocks    func()
		isNotFound    bool
	}{
		{
			name:     "successful node retrieval",
			nodeName: "node1",
			wantErr:  false,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node1",
					httpmock.NewStringResponder(200, `{"node": {"name": "node1", "iqn": "iqn.1993-08.org.debian:01:node1", "ips": ["192.168.1.1"], "publicationState": "online"}}`))
			},
		},
		{
			name:       "node not found",
			nodeName:   "nonexistent-node",
			wantErr:    true,
			isNotFound: true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node/nonexistent-node",
					httpmock.NewStringResponder(404, `{"error": "node not found"}`))
			},
		},
		{
			name:     "server error during retrieval",
			nodeName: "node-server-error",
			wantErr:  true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node-server-error",
					httpmock.NewStringResponder(500, `{"error": "internal server error"}`))
			},
		},
		{
			name:     "network error during HTTP call",
			nodeName: "node-network-error",
			wantErr:  true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node-network-error",
					httpmock.NewErrorResponder(errors.New("network error")))
			},
		},
		{
			name:     "invalid JSON response",
			nodeName: "node-invalid-json",
			wantErr:  true,
			setupMocks: func() {
				httpmock.RegisterResponder("GET", BaseURL()+"/node/node-invalid-json",
					httpmock.NewStringResponder(200, `invalid json`))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			httpmock.Reset()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			result, err := GetNode(tc.nodeName)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				if tc.isNotFound {
					assert.True(t, errors.IsNotFoundError(err))
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tc.nodeName, result.Name)
			}
		})
	}
}

// Test WriteNodes function with different output formats
func TestWriteNodes(t *testing.T) {
	testCases := []struct {
		name         string
		outputFormat string
		nodes        []models.NodeExternal
	}{
		{
			name:         "JSON format",
			outputFormat: FormatJSON,
			nodes: []models.NodeExternal{
				{
					Name: "node1",
					IQN:  "iqn.1993-08.org.debian:01:node1",
					IPs:  []string{"192.168.1.1"},
				},
			},
		},
		{
			name:         "YAML format",
			outputFormat: FormatYAML,
			nodes: []models.NodeExternal{
				{
					Name: "node1",
					IQN:  "iqn.1993-08.org.debian:01:node1",
					IPs:  []string{"192.168.1.1"},
				},
			},
		},
		{
			name:         "Name format",
			outputFormat: FormatName,
			nodes: []models.NodeExternal{
				{
					Name: "node1",
					IQN:  "iqn.1993-08.org.debian:01:node1",
					IPs:  []string{"192.168.1.1"},
				},
			},
		},
		{
			name:         "Wide format",
			outputFormat: FormatWide,
			nodes: []models.NodeExternal{
				{
					Name: "node1",
					IQN:  "iqn.1993-08.org.debian:01:node1",
					IPs:  []string{"192.168.1.1", "192.168.1.2"},
					HostInfo: &models.HostSystem{
						Services: []string{"iscsid", "multipathd"},
					},
					PublicationState: "",
				},
			},
		},
		{
			name:         "Default format",
			outputFormat: "",
			nodes: []models.NodeExternal{
				{
					Name: "node1",
					IQN:  "iqn.1993-08.org.debian:01:node1",
					IPs:  []string{"192.168.1.1"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prevOutputFormat := OutputFormat
			defer func() {
				OutputFormat = prevOutputFormat
			}()

			OutputFormat = tc.outputFormat
			assert.NotPanics(t, func() {
				WriteNodes(tc.nodes)
			})
		})
	}
}

func TestWriteWideNodeTable(t *testing.T) {
	testCases := []struct {
		name  string
		nodes []models.NodeExternal
	}{
		{
			name: "nodes with host info",
			nodes: []models.NodeExternal{
				{
					Name: "node1",
					IQN:  "iqn.1993-08.org.debian:01:node1",
					IPs:  []string{"192.168.1.1", "192.168.1.2"},
					HostInfo: &models.HostSystem{
						Services: []string{"iscsid", "multipathd"},
					},
					PublicationState: "",
				},
			},
		},
		{
			name: "nodes without host info",
			nodes: []models.NodeExternal{
				{
					Name:             "node2",
					IQN:              "iqn.1993-08.org.debian:01:node2",
					IPs:              []string{"192.168.1.3"},
					HostInfo:         nil,
					PublicationState: "",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				writeWideNodeTable(tc.nodes)
			})
		})
	}
}
