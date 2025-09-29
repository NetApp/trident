// Copyright 2025 NetApp, Inc. All Rights Reserved.

package metrics

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

// generateTestCertificates creates a CA, server certificate, and private key for testing
func generateTestCertificates(t *testing.T) (caCertPEM, serverCertPEM, serverKeyPEM []byte) {
	// Generate CA private key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)

	// Create CA certificate template
	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test CA"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test City"},
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
	assert.NoError(t, err)

	// Encode CA certificate to PEM
	caCertPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCertDER,
	})

	// Generate server private key
	serverPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)

	// Create server certificate template
	serverTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization:  []string{"Test Server"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test City"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    "localhost",
		},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// Parse CA certificate for signing
	caCert, err := x509.ParseCertificate(caCertDER)
	assert.NoError(t, err)

	// Create server certificate signed by CA
	serverCertDER, err := x509.CreateCertificate(rand.Reader, &serverTemplate, caCert, &serverPrivKey.PublicKey, caPrivKey)
	assert.NoError(t, err)

	// Encode server certificate to PEM
	serverCertPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverCertDER,
	})

	// Encode server private key to PEM
	serverKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverPrivKey),
	})

	return caCertPEM, serverCertPEM, serverKeyPEM
}

// writeTempCertFiles writes certificate data to temporary files and returns their paths
func writeTempCertFiles(t *testing.T, caCertPEM, serverCertPEM, serverKeyPEM []byte) (string, string, string) {
	tempDir := t.TempDir()

	caCertFile := filepath.Join(tempDir, "ca.crt")
	serverCertFile := filepath.Join(tempDir, "server.crt")
	serverKeyFile := filepath.Join(tempDir, "server.key")

	err := os.WriteFile(caCertFile, caCertPEM, 0o644)
	assert.NoError(t, err)

	err = os.WriteFile(serverCertFile, serverCertPEM, 0o644)
	assert.NoError(t, err)

	err = os.WriteFile(serverKeyFile, serverKeyPEM, 0o644)
	assert.NoError(t, err)

	return caCertFile, serverCertFile, serverKeyFile
}

func TestNewMetricsServer(t *testing.T) {
	server := NewMetricsServer("127.0.0.1", "8001")
	assert.NotNil(t, server)
	assert.Equal(t, "127.0.0.1:8001", server.server.Addr)
	assert.Equal(t, "metrics", server.GetName())
	assert.Equal(t, config.OrchestratorAPIVersion, server.Version())
}

func TestNewHTTPSMetricsServer(t *testing.T) {
	// Generate test certificates dynamically
	caCertPEM, serverCertPEM, serverKeyPEM := generateTestCertificates(t)

	// Write certificates to temporary files
	caCertFile, serverCertFile, serverKeyFile := writeTempCertFiles(t, caCertPEM, serverCertPEM, serverKeyPEM)

	// Test HTTPS metrics server creation
	server, err := NewHTTPSMetricsServer("127.0.0.1", "8443", caCertFile, serverCertFile, serverKeyFile, false, time.Minute)
	assert.NoError(t, err)
	assert.NotNil(t, server)
	assert.Equal(t, "127.0.0.1:8443", server.server.Addr)
	assert.Equal(t, "HTTPS metrics", server.GetName())
	assert.Equal(t, config.OrchestratorAPIVersion, server.Version())
	assert.Equal(t, tls.NoClientCert, server.server.TLSConfig.ClientAuth)
}

func TestNewHTTPSMetricsServerWithMutualTLS(t *testing.T) {
	// Generate test certificates dynamically
	caCertPEM, serverCertPEM, serverKeyPEM := generateTestCertificates(t)

	// Write certificates to temporary files
	caCertFile, serverCertFile, serverKeyFile := writeTempCertFiles(t, caCertPEM, serverCertPEM, serverKeyPEM)

	// Test HTTPS metrics server creation with mutual TLS
	server, err := NewHTTPSMetricsServer("127.0.0.1", "8443", caCertFile, serverCertFile, serverKeyFile, true, time.Minute)
	assert.NoError(t, err)
	assert.NotNil(t, server)
	assert.Equal(t, tls.RequireAndVerifyClientCert, server.server.TLSConfig.ClientAuth)
	assert.NotNil(t, server.server.TLSConfig.ClientCAs)
}

func TestNewHTTPSMetricsServerInvalidCACert(t *testing.T) {
	// Generate valid server certificates but use invalid CA cert
	_, serverCertPEM, serverKeyPEM := generateTestCertificates(t)

	tempDir := t.TempDir()
	caCertFile := filepath.Join(tempDir, "invalid_ca.crt")
	serverCertFile := filepath.Join(tempDir, "server.crt")
	serverKeyFile := filepath.Join(tempDir, "server.key")

	// Write invalid CA certificate file
	err := os.WriteFile(caCertFile, []byte("invalid certificate content"), 0o644)
	assert.NoError(t, err)

	err = os.WriteFile(serverCertFile, serverCertPEM, 0o644)
	assert.NoError(t, err)

	err = os.WriteFile(serverKeyFile, serverKeyPEM, 0o644)
	assert.NoError(t, err)

	// Test HTTPS metrics server creation with invalid CA cert
	server, err := NewHTTPSMetricsServer("127.0.0.1", "8443", caCertFile, serverCertFile, serverKeyFile, true, time.Minute)
	assert.NoError(t, err) // Should not error on invalid cert content, just won't add to pool
	assert.NotNil(t, server)
}

func TestNewHTTPSMetricsServerMissingCACert(t *testing.T) {
	// Generate valid server certificates
	_, serverCertPEM, serverKeyPEM := generateTestCertificates(t)

	tempDir := t.TempDir()
	caCertFile := filepath.Join(tempDir, "missing_ca.crt")
	serverCertFile := filepath.Join(tempDir, "server.crt")
	serverKeyFile := filepath.Join(tempDir, "server.key")

	// Don't create the CA cert file, but create server cert and key
	err := os.WriteFile(serverCertFile, serverCertPEM, 0o644)
	assert.NoError(t, err)

	err = os.WriteFile(serverKeyFile, serverKeyPEM, 0o644)
	assert.NoError(t, err)

	// Test HTTPS metrics server creation with missing CA cert file
	server, err := NewHTTPSMetricsServer("127.0.0.1", "8443", caCertFile, serverCertFile, serverKeyFile, true, time.Minute)
	assert.Error(t, err)
	assert.Nil(t, server)
	assert.Contains(t, err.Error(), "could not read CA certificate file")
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
