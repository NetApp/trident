// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package k8sclienttest provides helpers for k8sclient unit tests. Import only from _test.go files.
package k8sclienttest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// StartKubernetesAPIServer starts a mock API server for version discovery.
func StartKubernetesAPIServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/version"):
			version := map[string]interface{}{
				"major":      "1",
				"minor":      "28",
				"gitVersion": "v1.28.0",
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(version)
		case strings.Contains(r.URL.Path, "/api"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"versions":["v1"]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// WriteKubeconfig creates a temporary kubeconfig pointing at serverURL.
func WriteKubeconfig(t *testing.T, serverURL, namespace string) string {
	t.Helper()

	tempDir := t.TempDir()
	kubeconfigPath := filepath.Join(tempDir, "kubeconfig")

	kubeconfig := &api.Config{
		Clusters: map[string]*api.Cluster{
			"test-cluster": {
				Server:                serverURL,
				InsecureSkipTLSVerify: true,
			},
		},
		Contexts: map[string]*api.Context{
			"test-context": {
				Cluster:   "test-cluster",
				Namespace: namespace,
			},
		},
		CurrentContext: "test-context",
	}

	kubeconfigData, err := clientcmd.Write(*kubeconfig)
	require.NoError(t, err)
	err = os.WriteFile(kubeconfigPath, kubeconfigData, 0o600)
	require.NoError(t, err)

	return kubeconfigPath
}
