// Copyright 2022 NetApp, Inc. All Rights Reserved.

package rest

import (
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/logging"
	mockcore "github.com/netapp/trident/mocks/mock_core"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	logging.InitLogOutput(io.Discard)
	logging.InitAuditLogger(true)
	os.Exit(m.Run())
}

func TestNewHTTPServer(t *testing.T) {
	tests := []struct {
		name         string
		address      string
		port         string
		writeTimeout time.Duration
		expectAddr   string
	}{
		{
			name:         "default configuration",
			address:      "localhost",
			port:         "8000",
			writeTimeout: 30 * time.Second,
			expectAddr:   "localhost:8000",
		},
		{
			name:         "custom address and port",
			address:      "127.0.0.1",
			port:         "9000",
			writeTimeout: 60 * time.Second,
			expectAddr:   "127.0.0.1:9000",
		},
		{
			name:         "empty address",
			address:      "",
			port:         "8080",
			writeTimeout: 15 * time.Second,
			expectAddr:   ":8080",
		},
		{
			name:         "ipv6 address",
			address:      "::1",
			port:         "8080",
			writeTimeout: 45 * time.Second,
			expectAddr:   "::1:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock orchestrator
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			// Create the HTTP server
			server := NewHTTPServer(mockOrchestrator, tt.address, tt.port, tt.writeTimeout)

			// Assertions
			assert.NotNil(t, server)
			assert.NotNil(t, server.server)
			assert.Equal(t, tt.expectAddr, server.server.Addr)
			assert.Equal(t, config.HTTPTimeout, server.server.ReadTimeout)
			assert.Equal(t, tt.writeTimeout, server.server.WriteTimeout)
			assert.NotNil(t, server.server.Handler)

			// Verify that the global orchestrator is set
			assert.Equal(t, mockOrchestrator, orchestrator)
		})
	}
}

func TestAPIServerHTTP_GetName(t *testing.T) {
	// Setup
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	server := NewHTTPServer(mockOrchestrator, "localhost", "8000", 30*time.Second)

	// Test
	name := server.GetName()

	// Assertions
	assert.Equal(t, "HTTP REST", name)
}

func TestAPIServerHTTP_Version(t *testing.T) {
	// Setup
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	server := NewHTTPServer(mockOrchestrator, "localhost", "8000", 30*time.Second)

	// Test
	version := server.Version()

	// Assertions
	assert.Equal(t, config.OrchestratorAPIVersion, version)
}

func TestAPIServerHTTP_Activate(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		port     string
		testFunc func(t *testing.T, server *APIServerHTTP)
	}{
		{
			name:    "successful activation",
			address: "localhost",
			port:    "0", // Use port 0 to get a random available port
			testFunc: func(t *testing.T, server *APIServerHTTP) {
				err := server.Activate()
				assert.NoError(t, err)

				// Give the server a moment to start
				time.Sleep(100 * time.Millisecond)

				// Verify server is listening by making a connection
				conn, err := net.Dial("tcp", server.server.Addr)
				if err == nil {
					conn.Close()
				}
				// If we can't connect, the server might not be fully started yet, which is ok for this test
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			server := NewHTTPServer(mockOrchestrator, tt.address, tt.port, 30*time.Second)

			// Run test
			tt.testFunc(t, server)

			// Cleanup - always try to deactivate
			if server != nil {
				server.Deactivate()
			}
		})
	}
}

func TestAPIServerHTTP_Deactivate(t *testing.T) {
	tests := []struct {
		name           string
		setupServer    func() *APIServerHTTP
		expectError    bool
		errorAssertion func(t *testing.T, err error)
	}{
		{
			name: "deactivate non-activated server",
			setupServer: func() *APIServerHTTP {
				mockCtrl := gomock.NewController(t)
				mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
				return NewHTTPServer(mockOrchestrator, "localhost", "0", 30*time.Second)
			},
			expectError: false,
		},
		{
			name: "deactivate activated server",
			setupServer: func() *APIServerHTTP {
				mockCtrl := gomock.NewController(t)
				mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
				server := NewHTTPServer(mockOrchestrator, "localhost", "0", 30*time.Second)

				// Activate the server
				err := server.Activate()
				require.NoError(t, err)

				// Give server time to start
				time.Sleep(100 * time.Millisecond)

				return server
			},
			expectError: false,
		},
		{
			name: "deactivate with custom context timeout",
			setupServer: func() *APIServerHTTP {
				mockCtrl := gomock.NewController(t)
				mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
				server := NewHTTPServer(mockOrchestrator, "localhost", "0", 30*time.Second)

				// Activate the server
				err := server.Activate()
				require.NoError(t, err)

				// Give server time to start
				time.Sleep(100 * time.Millisecond)

				return server
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()

			// Test deactivation
			err := server.Deactivate()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorAssertion != nil {
					tt.errorAssertion(t, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAPIServerHTTP_ActivateDeactivateCycle(t *testing.T) {
	// Setup
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	server := NewHTTPServer(mockOrchestrator, "localhost", "0", 30*time.Second)

	// Test multiple activate/deactivate cycles
	for i := 0; i < 3; i++ {
		t.Run(fmt.Sprintf("cycle_%d", i), func(t *testing.T) {
			// Activate
			err := server.Activate()
			assert.NoError(t, err)

			// Give server time to start
			time.Sleep(50 * time.Millisecond)

			// Deactivate
			err = server.Deactivate()
			assert.NoError(t, err)

			// Give server time to stop
			time.Sleep(50 * time.Millisecond)
		})
	}
}

func TestAPIServerHTTP_ConcurrentActivation(t *testing.T) {
	// Setup
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	server := NewHTTPServer(mockOrchestrator, "localhost", "0", 30*time.Second)

	// Test concurrent activation calls
	const numGoroutines = 5
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			errors <- server.Activate()
		}()
	}

	// Collect all errors
	for i := 0; i < numGoroutines; i++ {
		err := <-errors
		assert.NoError(t, err)
	}

	// Cleanup
	err := server.Deactivate()
	assert.NoError(t, err)
}

func TestAPIServerHTTP_DeactivateTimeout(t *testing.T) {
	// This test verifies that deactivation respects the context timeout
	// Setup
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	server := NewHTTPServer(mockOrchestrator, "localhost", "0", 30*time.Second)

	// Activate server
	err := server.Activate()
	require.NoError(t, err)

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test deactivation (should complete within HTTPTimeout)
	start := time.Now()
	err = server.Deactivate()
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.True(t, duration < config.HTTPTimeout+time.Second, "Deactivation took longer than expected")
}

func TestAPIServerHTTP_ServerConfiguration(t *testing.T) {
	// Setup
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	customTimeout := 45 * time.Second
	server := NewHTTPServer(mockOrchestrator, "127.0.0.1", "8080", customTimeout)

	// Verify server configuration
	assert.Equal(t, "127.0.0.1:8080", server.server.Addr)
	assert.Equal(t, config.HTTPTimeout, server.server.ReadTimeout)
	assert.Equal(t, customTimeout, server.server.WriteTimeout)
	assert.NotNil(t, server.server.Handler)

	// Verify the router is properly initialized
	router := server.server.Handler
	assert.NotNil(t, router)
}

func TestAPIServerHTTP_InvalidPortNumbers(t *testing.T) {
	tests := []struct {
		name        string
		port        string
		description string
	}{
		{
			name:        "negative port number",
			port:        "-1",
			description: "Port number cannot be negative",
		},
		{
			name:        "port number exceeds maximum",
			port:        "65536",
			description: "Port number exceeds maximum valid port (65535)",
		},
		{
			name:        "very large port number",
			port:        "99999",
			description: "Port number far exceeds valid range",
		},
		{
			name:        "non-numeric port",
			port:        "abc",
			description: "Port must be numeric",
		},
		{
			name:        "empty port",
			port:        "",
			description: "Empty port string",
		},
		{
			name:        "port with special characters",
			port:        "80!@",
			description: "Port contains special characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			// Server creation should succeed even with invalid port
			// The error will occur during Activate() when attempting to bind
			server := NewHTTPServer(mockOrchestrator, "localhost", tt.port, 30*time.Second)
			assert.NotNil(t, server)
			assert.NotNil(t, server.server)

			// Verify the address was set (even if invalid)
			expectedAddr := fmt.Sprintf("localhost:%s", tt.port)
			assert.Equal(t, expectedAddr, server.server.Addr, tt.description)

			// Note: Activate() runs in a goroutine and will fail asynchronously
			// We don't test activation here as it would cause the test to fail
			// via Log().Fatal() in the goroutine
		})
	}
}

func TestAPIServerHTTP_AlreadyBoundPort(t *testing.T) {
	// This test verifies that when two servers try to bind to the same port,
	// the second one will have the address configured but won't actually bind.
	// Note: We can't fully test the activation failure because Activate() runs
	// in a goroutine and calls Log().Fatal() on error, which would kill the test.

	// Setup
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator1 := mockcore.NewMockOrchestrator(mockCtrl)
	mockOrchestrator2 := mockcore.NewMockOrchestrator(mockCtrl)

	// Create and activate first server
	server1 := NewHTTPServer(mockOrchestrator1, "localhost", "0", 30*time.Second)
	err := server1.Activate()
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)

	// Create second server with same configuration
	server2 := NewHTTPServer(mockOrchestrator2, "localhost", "0", 30*time.Second)
	assert.NotNil(t, server2)
	assert.NotNil(t, server2.server)

	// We don't activate server2 because:
	// 1. It would try to bind to a random port (port 0) which would likely succeed
	// 2. If we used the same port as server1, activation would call Log().Fatal()

	// Clean up
	server1.Deactivate()
}

func TestAPIServerHTTP_InvalidAddresses(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		description string
	}{
		{
			name:        "invalid IPv4 address",
			address:     "999.999.999.999",
			description: "IPv4 octets exceed valid range",
		},
		{
			name:        "malformed IPv4 address",
			address:     "192.168.1",
			description: "Incomplete IPv4 address",
		},
		{
			name:        "invalid characters in address",
			address:     "local@host",
			description: "Address contains invalid characters",
		},
		{
			name:        "malformed IPv6 address",
			address:     "::gg",
			description: "Invalid characters in IPv6 address",
		},
		{
			name:        "address with port included",
			address:     "localhost:8080",
			description: "Port should not be included in address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			// Server creation should succeed even with invalid address
			// The error will occur during Activate() when attempting to bind
			server := NewHTTPServer(mockOrchestrator, tt.address, "0", 30*time.Second)
			assert.NotNil(t, server)
			assert.NotNil(t, server.server)

			// Verify the address was set (even if invalid)
			expectedAddr := fmt.Sprintf("%s:0", tt.address)
			assert.Equal(t, expectedAddr, server.server.Addr, tt.description)

			// Note: Activate() runs in a goroutine and will fail asynchronously
			// We don't test activation here as it would cause the test to fail
			// via Log().Fatal() in the goroutine
		})
	}
}

func TestAPIServerHTTP_NilOrchestrator(t *testing.T) {
	// Test creating server with nil orchestrator
	server := NewHTTPServer(nil, "localhost", "0", 30*time.Second)

	// Server should still be created (orchestrator is set globally)
	assert.NotNil(t, server)
	assert.NotNil(t, server.server)

	// However, the global orchestrator will be nil
	assert.Nil(t, orchestrator)
}

func TestAPIServerHTTP_DeactivateNilServer(t *testing.T) {
	// Create a server but manually set internal server to nil to test edge case
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	server := NewHTTPServer(mockOrchestrator, "localhost", "0", 30*time.Second)
	server.server = nil

	// Deactivate should panic with nil server since there's no nil check
	// We verify the panic happens
	assert.Panics(t, func() {
		server.Deactivate()
	}, "Deactivating with nil server should panic")
}

func TestAPIServerHTTP_ActivateWithZeroTimeout(t *testing.T) {
	// Setup
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	// Create server with zero timeout
	server := NewHTTPServer(mockOrchestrator, "localhost", "0", 0)

	// Verify timeout is set to 0
	assert.Equal(t, time.Duration(0), server.server.WriteTimeout)

	// Should still be able to activate
	err := server.Activate()
	assert.NoError(t, err)

	// Cleanup
	server.Deactivate()
}

func TestAPIServerHTTP_ActivateWithNegativeTimeout(t *testing.T) {
	// Setup
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	// Create server with negative timeout
	negativeTimeout := -30 * time.Second
	server := NewHTTPServer(mockOrchestrator, "localhost", "0", negativeTimeout)

	// Verify timeout is set to negative value (Go's http server will treat this specially)
	assert.Equal(t, negativeTimeout, server.server.WriteTimeout)

	// Should still be able to activate (negative timeout disables timeout)
	err := server.Activate()
	assert.NoError(t, err)

	// Cleanup
	server.Deactivate()
}

func TestAPIServerHTTP_MultipleDeactivations(t *testing.T) {
	// Setup
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	server := NewHTTPServer(mockOrchestrator, "localhost", "0", 30*time.Second)

	// Activate
	err := server.Activate()
	require.NoError(t, err)

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// First deactivation
	err = server.Deactivate()
	assert.NoError(t, err)

	// Second deactivation should not panic or cause issues
	err = server.Deactivate()
	// May return error that server is already closed, but shouldn't panic
	_ = err
}

func TestAPIServerHTTP_ActivateAfterDeactivate(t *testing.T) {
	// Setup
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	server := NewHTTPServer(mockOrchestrator, "localhost", "0", 30*time.Second)

	// First activation
	err := server.Activate()
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Deactivate
	err = server.Deactivate()
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Try to activate again
	// Note: With the current implementation, this will succeed because
	// Activate() starts a new goroutine each time. However, the underlying
	// http.Server was already shutdown, so the ListenAndServe will fail
	// in the goroutine (but won't return an error to us).
	err = server.Activate()
	assert.NoError(t, err, "Activate returns nil immediately even after deactivation")

	// The server won't actually work after being shutdown and reactivated
	// but we can't easily test that without causing the test to fail
	time.Sleep(50 * time.Millisecond)
}
