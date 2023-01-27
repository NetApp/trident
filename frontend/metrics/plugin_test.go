// Copyright 2022 NetApp, Inc. All Rights Reserved.

package metrics

import (
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestNewMetricsServer(t *testing.T) {
	result := NewMetricsServer("1.1.1.1", "90")
	assert.NotNil(t, result, "metrics server is not initialized")
}

func TestServer_Activate(t *testing.T) {
	mux := http.NewServeMux()
	rh := http.RedirectHandler("trident-metrics.com", 307)
	mux.Handle("/endpoint", rh)
	serv := http.Server{
		Addr:    "10.10.10.10:90",
		Handler: mux,
	}
	server := Server{&serv}
	result := server.Activate()

	// Stop the server after the test run
	_ = server.Deactivate()

	assert.Nil(t, result, "server not active")
}

func TestServer_Deactivate(t *testing.T) {
	var serv http.Server
	server := Server{&serv}
	result := server.Deactivate()
	assert.Nil(t, result, "server is active")
}

func TestServer_GetName(t *testing.T) {
	var serv http.Server
	server := Server{&serv}
	result := server.GetName()
	assert.Equal(t, result, "metrics", "server name mismatch")
}

func TestServer_Version(t *testing.T) {
	var serv http.Server
	server := Server{&serv}
	result := server.Version()
	assert.Equal(t, result, "1", "orchestration API version mismatch")
}
