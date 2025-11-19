// Copyright 2025 NetApp, Inc. All Rights Reserved.

package rest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	mockcore "github.com/netapp/trident/mocks/mock_core"
)

// Helper function to create test certificates for TLS testing
func createTestCertificates(t *testing.T, tempDir string) (caCertFile, serverCertFile, serverKeyFile, clientCertFile, clientKeyFile string) {
	// Generate CA private key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create CA certificate template
	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test CA"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Create CA certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	// Save CA certificate to file
	caCertFile = filepath.Join(tempDir, "ca.crt")
	caCertOut, err := os.Create(caCertFile)
	require.NoError(t, err)
	defer caCertOut.Close()
	err = pem.Encode(caCertOut, &pem.Block{Type: "CERTIFICATE", Bytes: caCertDER})
	require.NoError(t, err)

	// Generate server private key
	serverPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create server certificate template
	serverTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization:  []string{"Test Server"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// Create server certificate
	serverCertDER, err := x509.CreateCertificate(rand.Reader, &serverTemplate, &caTemplate, &serverPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	// Save server certificate to file
	serverCertFile = filepath.Join(tempDir, "server.crt")
	serverCertOut, err := os.Create(serverCertFile)
	require.NoError(t, err)
	defer serverCertOut.Close()
	err = pem.Encode(serverCertOut, &pem.Block{Type: "CERTIFICATE", Bytes: serverCertDER})
	require.NoError(t, err)

	// Save server private key to file
	serverKeyFile = filepath.Join(tempDir, "server.key")
	serverKeyOut, err := os.Create(serverKeyFile)
	require.NoError(t, err)
	defer serverKeyOut.Close()
	serverKeyPEM := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverPrivKey)}
	err = pem.Encode(serverKeyOut, serverKeyPEM)
	require.NoError(t, err)

	// Generate client private key
	clientPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create client certificate template
	clientTemplate := x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			Organization: []string{"Test Client"},
			Country:      []string{"US"},
			Province:     []string{""},
			Locality:     []string{"Test"},
			CommonName:   config.ClientCertName, // This is important for authentication
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		SubjectKeyId: []byte{1, 2, 3, 4, 5},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// Create client certificate
	clientCertDER, err := x509.CreateCertificate(rand.Reader, &clientTemplate, &caTemplate, &clientPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	// Save client certificate to file
	clientCertFile = filepath.Join(tempDir, "client.crt")
	clientCertOut, err := os.Create(clientCertFile)
	require.NoError(t, err)
	defer clientCertOut.Close()
	err = pem.Encode(clientCertOut, &pem.Block{Type: "CERTIFICATE", Bytes: clientCertDER})
	require.NoError(t, err)

	// Save client private key to file
	clientKeyFile = filepath.Join(tempDir, "client.key")
	clientKeyOut, err := os.Create(clientKeyFile)
	require.NoError(t, err)
	defer clientKeyOut.Close()
	clientKeyPEM := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientPrivKey)}
	err = pem.Encode(clientKeyOut, clientKeyPEM)
	require.NoError(t, err)

	return caCertFile, serverCertFile, serverKeyFile, clientCertFile, clientKeyFile
}

func TestNewHTTPSServer(t *testing.T) {
	// Create temporary directory for test certificates
	tempDir, err := os.MkdirTemp("", "trident_https_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	caCertFile, serverCertFile, serverKeyFile, _, _ := createTestCertificates(t, tempDir)

	tests := []struct {
		name            string
		address         string
		port            string
		caCertFile      string
		serverCertFile  string
		serverKeyFile   string
		enableMutualTLS bool
		handler         http.Handler
		writeTimeout    time.Duration
		expectError     bool
		expectAddr      string
	}{
		{
			name:            "default configuration with mutual TLS",
			address:         "localhost",
			port:            "8443",
			caCertFile:      caCertFile,
			serverCertFile:  serverCertFile,
			serverKeyFile:   serverKeyFile,
			enableMutualTLS: true,
			handler:         http.DefaultServeMux,
			writeTimeout:    30 * time.Second,
			expectError:     false,
			expectAddr:      "localhost:8443",
		},
		{
			name:            "configuration without mutual TLS",
			address:         "127.0.0.1",
			port:            "9443",
			caCertFile:      "",
			serverCertFile:  serverCertFile,
			serverKeyFile:   serverKeyFile,
			enableMutualTLS: false,
			handler:         http.DefaultServeMux,
			writeTimeout:    45 * time.Second,
			expectError:     false,
			expectAddr:      "127.0.0.1:9443",
		},
		{
			name:            "configuration with custom handler",
			address:         "",
			port:            "8080",
			caCertFile:      caCertFile,
			serverCertFile:  serverCertFile,
			serverKeyFile:   serverKeyFile,
			enableMutualTLS: true,
			handler:         &testHandler{},
			writeTimeout:    60 * time.Second,
			expectError:     false,
			expectAddr:      ":8080",
		},
		{
			name:            "invalid CA certificate file",
			address:         "localhost",
			port:            "8443",
			caCertFile:      "/nonexistent/ca.crt",
			serverCertFile:  serverCertFile,
			serverKeyFile:   serverKeyFile,
			enableMutualTLS: true,
			handler:         http.DefaultServeMux,
			writeTimeout:    30 * time.Second,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock orchestrator
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			// Create the HTTPS server
			server, err := NewHTTPSServer(
				mockOrchestrator,
				tt.address,
				tt.port,
				tt.caCertFile,
				tt.serverCertFile,
				tt.serverKeyFile,
				tt.enableMutualTLS,
				tt.handler,
				tt.writeTimeout,
			)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, server)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, server)
				assert.NotNil(t, server.server)
				assert.Equal(t, tt.expectAddr, server.server.Addr)
				assert.Equal(t, config.HTTPTimeout, server.server.ReadTimeout)
				assert.Equal(t, tt.writeTimeout, server.server.WriteTimeout)
				assert.NotNil(t, server.server.Handler)
				assert.NotNil(t, server.server.TLSConfig)

				// Verify TLS configuration
				if tt.enableMutualTLS {
					assert.Equal(t, tls.RequireAndVerifyClientCert, server.server.TLSConfig.ClientAuth)
					assert.IsType(t, &tlsAuthHandler{}, server.server.Handler)
				} else {
					assert.Equal(t, tls.NoClientCert, server.server.TLSConfig.ClientAuth)
					assert.Equal(t, tt.handler, server.server.Handler)
				}

				assert.Equal(t, uint16(config.MinServerTLSVersion), server.server.TLSConfig.MinVersion)

				// Verify CA certificate configuration
				if tt.caCertFile != "" {
					assert.NotNil(t, server.server.TLSConfig.ClientCAs)
				}

				// Verify that the global orchestrator is set
				assert.Equal(t, mockOrchestrator, orchestrator)

				// Verify file paths are stored
				assert.Equal(t, tt.caCertFile, server.caCertFile)
				assert.Equal(t, tt.serverCertFile, server.serverCertFile)
				assert.Equal(t, tt.serverKeyFile, server.serverKeyFile)
			}
		})
	}
}

func TestAPIServerHTTPS_GetName(t *testing.T) {
	// Setup
	tempDir, err := os.MkdirTemp("", "trident_https_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	_, serverCertFile, serverKeyFile, _, _ := createTestCertificates(t, tempDir)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	server, err := NewHTTPSServer(
		mockOrchestrator,
		"localhost",
		"8443",
		"",
		serverCertFile,
		serverKeyFile,
		false,
		http.DefaultServeMux,
		30*time.Second,
	)
	require.NoError(t, err)

	// Test
	name := server.GetName()

	// Assertions
	assert.Equal(t, "HTTPS REST", name)
}

func TestAPIServerHTTPS_Version(t *testing.T) {
	// Setup
	tempDir, err := os.MkdirTemp("", "trident_https_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	_, serverCertFile, serverKeyFile, _, _ := createTestCertificates(t, tempDir)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	server, err := NewHTTPSServer(
		mockOrchestrator,
		"localhost",
		"8443",
		"",
		serverCertFile,
		serverKeyFile,
		false,
		http.DefaultServeMux,
		30*time.Second,
	)
	require.NoError(t, err)

	// Test
	version := server.Version()

	// Assertions
	assert.Equal(t, config.OrchestratorAPIVersion, version)
}

func TestAPIServerHTTPS_Activate(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		port     string
		testFunc func(t *testing.T, server *APIServerHTTPS)
	}{
		{
			name:    "successful activation",
			address: "localhost",
			port:    "0", // Use port 0 to get a random available port
			testFunc: func(t *testing.T, server *APIServerHTTPS) {
				err := server.Activate()
				assert.NoError(t, err)

				// Give the server a moment to start
				time.Sleep(100 * time.Millisecond)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tempDir, err := os.MkdirTemp("", "trident_https_test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			_, serverCertFile, serverKeyFile, _, _ := createTestCertificates(t, tempDir)

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			server, err := NewHTTPSServer(
				mockOrchestrator,
				tt.address,
				tt.port,
				"",
				serverCertFile,
				serverKeyFile,
				false,
				http.DefaultServeMux,
				30*time.Second,
			)
			require.NoError(t, err)

			// Run test
			tt.testFunc(t, server)

			// Cleanup - always try to deactivate
			if server != nil {
				server.Deactivate()
			}
		})
	}
}

func TestAPIServerHTTPS_Deactivate(t *testing.T) {
	tests := []struct {
		name           string
		setupServer    func() *APIServerHTTPS
		expectError    bool
		errorAssertion func(t *testing.T, err error)
	}{
		{
			name: "deactivate non-activated server",
			setupServer: func() *APIServerHTTPS {
				tempDir, err := os.MkdirTemp("", "trident_https_test")
				require.NoError(t, err)
				defer os.RemoveAll(tempDir)

				_, serverCertFile, serverKeyFile, _, _ := createTestCertificates(t, tempDir)

				mockCtrl := gomock.NewController(t)
				mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
				server, err := NewHTTPSServer(
					mockOrchestrator,
					"localhost",
					"0",
					"",
					serverCertFile,
					serverKeyFile,
					false,
					http.DefaultServeMux,
					30*time.Second,
				)
				require.NoError(t, err)
				return server
			},
			expectError: false,
		},
		{
			name: "deactivate activated server",
			setupServer: func() *APIServerHTTPS {
				tempDir, err := os.MkdirTemp("", "trident_https_test")
				require.NoError(t, err)
				defer os.RemoveAll(tempDir)

				_, serverCertFile, serverKeyFile, _, _ := createTestCertificates(t, tempDir)

				mockCtrl := gomock.NewController(t)
				mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
				server, err := NewHTTPSServer(
					mockOrchestrator,
					"localhost",
					"0",
					"",
					serverCertFile,
					serverKeyFile,
					false,
					http.DefaultServeMux,
					30*time.Second,
				)
				require.NoError(t, err)

				// Activate the server
				err = server.Activate()
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

// Test handler for testing purposes
type testHandler struct{}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("test response"))
}

func TestTLSAuthHandler_ServeHTTP(t *testing.T) {
	tests := []struct {
		name                string
		setupRequest        func() *http.Request
		expectStatus        int
		expectAuthHeader    bool
		expectHandlerCalled bool
	}{
		{
			name: "valid client certificate",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.TLS = &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{
						{
							Subject: pkix.Name{
								CommonName: config.ClientCertName,
							},
						},
					},
				}
				return req
			},
			expectStatus:        http.StatusOK,
			expectAuthHeader:    false,
			expectHandlerCalled: true,
		},
		{
			name: "no client certificate",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.TLS = &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{},
				}
				return req
			},
			expectStatus:        http.StatusUnauthorized,
			expectAuthHeader:    true,
			expectHandlerCalled: false,
		},
		{
			name: "invalid client certificate common name",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.TLS = &tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{
						{
							Subject: pkix.Name{
								CommonName: "invalid-cert-name",
							},
						},
					},
				}
				return req
			},
			expectStatus:        http.StatusUnauthorized,
			expectAuthHeader:    true,
			expectHandlerCalled: false,
		},
		{
			name: "empty TLS connection state",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.TLS = &tls.ConnectionState{
					PeerCertificates: nil,
				}
				return req
			},
			expectStatus:        http.StatusUnauthorized,
			expectAuthHeader:    true,
			expectHandlerCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test handler that tracks if it was called
			handlerCalled := false
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			// Create TLS auth handler
			tlsHandler := &tlsAuthHandler{handler: testHandler}

			// Setup request and response recorder
			req := tt.setupRequest()
			rr := httptest.NewRecorder()

			// Execute the handler
			tlsHandler.ServeHTTP(rr, req)

			// Assertions
			assert.Equal(t, tt.expectStatus, rr.Code)
			assert.Equal(t, tt.expectHandlerCalled, handlerCalled)

			if tt.expectAuthHeader {
				authHeader := rr.Header().Get("WWW-Authenticate")
				expectedAuth := fmt.Sprintf("Basic realm=\"%s\"", config.OrchestratorName)
				assert.Equal(t, expectedAuth, authHeader)
			}
		})
	}
}

func TestAPIServerHTTPS_ActivateDeactivateCycle(t *testing.T) {
	// Setup
	tempDir, err := os.MkdirTemp("", "trident_https_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	_, serverCertFile, serverKeyFile, _, _ := createTestCertificates(t, tempDir)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	server, err := NewHTTPSServer(
		mockOrchestrator,
		"localhost",
		"0",
		"",
		serverCertFile,
		serverKeyFile,
		false,
		http.DefaultServeMux,
		30*time.Second,
	)
	require.NoError(t, err)

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

func TestAPIServerHTTPS_ServerConfiguration(t *testing.T) {
	// Setup
	tempDir, err := os.MkdirTemp("", "trident_https_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	caCertFile, serverCertFile, serverKeyFile, _, _ := createTestCertificates(t, tempDir)

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	customTimeout := 45 * time.Second
	server, err := NewHTTPSServer(
		mockOrchestrator,
		"127.0.0.1",
		"8443",
		caCertFile,
		serverCertFile,
		serverKeyFile,
		true,
		http.DefaultServeMux,
		customTimeout,
	)
	require.NoError(t, err)

	// Verify server configuration
	assert.Equal(t, "127.0.0.1:8443", server.server.Addr)
	assert.Equal(t, config.HTTPTimeout, server.server.ReadTimeout)
	assert.Equal(t, customTimeout, server.server.WriteTimeout)
	assert.NotNil(t, server.server.Handler)
	assert.NotNil(t, server.server.TLSConfig)

	// Verify TLS configuration
	assert.Equal(t, tls.RequireAndVerifyClientCert, server.server.TLSConfig.ClientAuth)
	assert.Equal(t, uint16(config.MinServerTLSVersion), server.server.TLSConfig.MinVersion)
	assert.NotNil(t, server.server.TLSConfig.ClientCAs)

	// Verify the TLS auth handler is properly initialized
	tlsAuthHandler, ok := server.server.Handler.(*tlsAuthHandler)
	assert.True(t, ok)
	assert.Equal(t, http.DefaultServeMux, tlsAuthHandler.handler)

	// Verify certificate files are stored
	assert.Equal(t, caCertFile, server.caCertFile)
	assert.Equal(t, serverCertFile, server.serverCertFile)
	assert.Equal(t, serverKeyFile, server.serverKeyFile)
}

func TestAPIServerHTTPS_InvalidCertificateHandling(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tests := []struct {
		name           string
		caCertFile     string
		serverCertFile string
		serverKeyFile  string
		expectError    bool
	}{
		{
			name:           "missing CA certificate file",
			caCertFile:     "/nonexistent/ca.crt",
			serverCertFile: "server.crt",
			serverKeyFile:  "server.key",
			expectError:    true,
		},
		{
			name:           "empty CA certificate file path",
			caCertFile:     "",
			serverCertFile: "server.crt",
			serverKeyFile:  "server.key",
			expectError:    false, // Should not error when caCertFile is empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewHTTPSServer(
				mockOrchestrator,
				"localhost",
				"8443",
				tt.caCertFile,
				tt.serverCertFile,
				tt.serverKeyFile,
				true,
				http.DefaultServeMux,
				30*time.Second,
			)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, server)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, server)
			}
		})
	}
}
