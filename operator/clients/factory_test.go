// Copyright 2022 NetApp, Inc. All Rights Reserved.

package clients

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	. "github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

// createValidTempKubeConfig creates a valid temporary kubeconfig file for testing
func createValidTempKubeConfig(t *testing.T) string {
	tempDir := t.TempDir()
	kubeconfigPath := filepath.Join(tempDir, "kubeconfig")

	config := &clientcmdapi.Config{
		APIVersion: "v1",
		Kind:       "Config",
		Clusters: map[string]*clientcmdapi.Cluster{
			"test-cluster": {
				Server:                "https://127.0.0.1:6443",
				InsecureSkipTLSVerify: true,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"test-context": {
				Cluster:   "test-cluster",
				AuthInfo:  "test-user",
				Namespace: "test-namespace",
			},
		},
		CurrentContext: "test-context",
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"test-user": {
				Token: "test-token",
			},
		},
	}

	err := clientcmd.WriteToFile(*config, kubeconfigPath)
	assert.NoError(t, err, "Failed to write kubeconfig file")

	return kubeconfigPath
}

// createInvalidKubeConfig creates an invalid kubeconfig file for testing
func createInvalidKubeConfig(t *testing.T) string {
	tempDir := t.TempDir()
	kubeconfigPath := filepath.Join(tempDir, "bad-kubeconfig")
	err := os.WriteFile(kubeconfigPath, []byte("invalid yaml content"), 0644)
	assert.NoError(t, err, "Failed to write bad kubeconfig file")
	return kubeconfigPath
}

// createEmptyKubeConfig creates an empty kubeconfig file for testing
func createEmptyKubeConfig(t *testing.T) string {
	tempDir := t.TempDir()
	kubeconfigPath := filepath.Join(tempDir, "empty-kubeconfig")
	err := os.WriteFile(kubeconfigPath, []byte(""), 0644)
	assert.NoError(t, err, "Failed to write empty kubeconfig file")
	return kubeconfigPath
}

func TestCreateK8SClients(t *testing.T) {
	validKubeConfig := createValidTempKubeConfig(t)

	tests := []struct {
		name           string
		apiServerIP    string
		kubeConfigPath string
		expectError    bool
		errorContains  string
		description    string
	}{
		{
			name:           "ValidKubeConfigWithInvalidServer",
			apiServerIP:    "https://invalid:6443",
			kubeConfigPath: validKubeConfig,
			expectError:    true,
			description:    "Valid kubeconfig but invalid server should fail",
		},
		{
			name:           "NonExistentKubeConfig",
			apiServerIP:    "",
			kubeConfigPath: "/non/existent/path",
			expectError:    true,
			description:    "Non-existent kubeconfig file should fail",
		},
		{
			name:           "InClusterConfigOutsideCluster",
			apiServerIP:    "",
			kubeConfigPath: "",
			expectError:    true,
			description:    "In-cluster config outside cluster should fail",
		},
		{
			name:           "EmptyAPIWithKubeConfig",
			apiServerIP:    "",
			kubeConfigPath: validKubeConfig,
			expectError:    true,
			description:    "Empty API with kubeconfig should trigger ex-cluster path",
		},
		{
			name:           "APIWithEmptyKubeConfig",
			apiServerIP:    "https://test:6443",
			kubeConfigPath: "",
			expectError:    true,
			description:    "API server with empty kubeconfig should trigger in-cluster path",
		},
		{
			name:           "BothNonEmpty",
			apiServerIP:    "https://test:6443",
			kubeConfigPath: "/invalid/path",
			expectError:    true,
			description:    "Both non-empty should trigger ex-cluster path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clients, err := CreateK8SClients(tt.apiServerIP, tt.kubeConfigPath)

			if tt.expectError {
				assert.Error(t, err, tt.description)
				assert.Nil(t, clients, "Clients should be nil on error")
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains, "Error message should contain expected text")
				}
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, clients, "Clients should not be nil on success")
			}
		})
	}
}

func TestCreateK8SClientsExCluster(t *testing.T) {
	validKubeConfig := createValidTempKubeConfig(t)
	invalidKubeConfig := createInvalidKubeConfig(t)
	emptyKubeConfig := createEmptyKubeConfig(t)

	tests := []struct {
		name           string
		apiServerIP    string
		kubeConfigPath string
		expectError    bool
		errorContains  string
		description    string
	}{
		{
			name:           "InvalidKubeConfigPath",
			apiServerIP:    "",
			kubeConfigPath: "/invalid/path",
			expectError:    true,
			description:    "Invalid kubeconfig path should fail",
		},
		{
			name:           "ValidConfigWithValidServer",
			apiServerIP:    "https://127.0.0.1:6443",
			kubeConfigPath: validKubeConfig,
			expectError:    true,
			errorContains:  "could not initialize Kubernetes client",
			description:    "Valid config should parse but fail at K8S client creation in test environment",
		},
		{
			name:           "InvalidKubeConfigContent",
			apiServerIP:    "",
			kubeConfigPath: invalidKubeConfig,
			expectError:    true,
			description:    "Invalid kubeconfig content should fail",
		},
		{
			name:           "EmptyKubeConfig",
			apiServerIP:    "",
			kubeConfigPath: emptyKubeConfig,
			expectError:    true,
			description:    "Empty kubeconfig should fail",
		},
		{
			name:           "EmptyAPIServerWithInvalidPath",
			apiServerIP:    "",
			kubeConfigPath: "/invalid/path",
			expectError:    true,
			description:    "Empty API server with invalid path should fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clients, err := createK8SClientsExCluster(tt.apiServerIP, tt.kubeConfigPath)

			if tt.expectError {
				assert.Error(t, err, tt.description)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains, "Error message should contain expected text")
				}
				// For the valid config case, clients might not be nil if config parsing succeeded
				if tt.errorContains != "could not initialize Kubernetes client" {
					assert.Nil(t, clients, "Clients should be nil on error")
				}
			} else {
				assert.NoError(t, err, tt.description)
				assert.NotNil(t, clients, "Clients should not be nil on success")
				assert.NotNil(t, clients.KubeConfig, "KubeConfig should not be nil")
			}
		})
	}
}

func TestCreateK8SClientsInCluster(t *testing.T) {
	// Test in-cluster creation (will fail due to missing namespace file in test environment)
	clients, err := createK8SClientsInCluster()

	assert.Error(t, err, "Expected error for in-cluster config outside cluster")
	assert.Nil(t, clients, "Clients should be nil on error")
}

func TestClientsStruct(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
	}{
		{
			name:      "TestNamespace",
			namespace: "test-namespace",
		},
		{
			name:      "EmptyNamespace",
			namespace: "",
		},
		{
			name:      "DefaultNamespace",
			namespace: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clients := &Clients{
				Namespace: tt.namespace,
			}

			assert.Equal(t, tt.namespace, clients.Namespace, "Namespace should be set correctly")
			assert.Nil(t, clients.KubeClient, "KubeClient should be nil initially")
			assert.Nil(t, clients.CRDClient, "CRDClient should be nil initially")
			assert.Nil(t, clients.TridentCRDClient, "TridentCRDClient should be nil initially")
			assert.Nil(t, clients.SnapshotClient, "SnapshotClient should be nil initially")
			assert.Nil(t, clients.K8SVersion, "K8SVersion should be nil initially")
		})
	}
}
