// Copyright 2026 NetApp, Inc. All Rights Reserved.

package gcpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	drivers "github.com/netapp/trident/storage_drivers"
)

func TestResolveCredentials_NilConfig(t *testing.T) {
	ctx := context.Background()
	creds, err := ResolveCredentials(ctx, nil)
	assert.Error(t, err)
	assert.Nil(t, creds)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestResolveCredentials_WIPCredential(t *testing.T) {
	ctx := context.Background()
	config := &ClientConfig{
		WIPCredential: &drivers.GCPWIPCredential{
			Type:             "external_account",
			Audience:         "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/prov",
			SubjectTokenType: "urn:ietf:params:oauth:token-type:jwt",
			TokenURL:         "https://sts.googleapis.com/v1/token",
			CredentialSource: &drivers.GCPWIPCredentialSource{
				File: "/var/run/secrets/token",
			},
		},
	}

	// CredentialsFromJSONWithType parses the JSON and returns a Credentials
	// object with a lazily-evaluated TokenSource; it succeeds even though the
	// credential source file doesn't exist.
	creds, err := ResolveCredentials(ctx, config)
	assert.NoError(t, err)
	assert.NotNil(t, creds)
}

func TestResolveCredentials_WIPCredentialWrongType(t *testing.T) {
	ctx := context.Background()
	config := &ClientConfig{
		WIPCredential: &drivers.GCPWIPCredential{
			Type: "service_account", // wrong type — ExternalAccount expected
		},
	}

	_, err := ResolveCredentials(ctx, config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "external_account")
}

func TestResolveCredentials_APIKey(t *testing.T) {
	ctx := context.Background()
	config := &ClientConfig{
		APIKey: &drivers.GCPPrivateKey{
			Type:         "service_account",
			ProjectID:    "test-project",
			PrivateKeyID: "key-id",
			// Use a short-lived, runtime-generated key so the repo has no PEM-shaped literals.
			PrivateKey:  testRSAPrivateKeyPEM(t),
			ClientEmail: "test@test-project.iam.gserviceaccount.com",
			ClientID:    "123456789",
			AuthURI:     "https://accounts.google.com/o/oauth2/auth",
			TokenURI:    "https://oauth2.googleapis.com/token",
		},
	}

	creds, err := ResolveCredentials(ctx, config)
	assert.NoError(t, err)
	assert.NotNil(t, creds)
}

func TestResolveCredentials_APIKeyWrongType(t *testing.T) {
	ctx := context.Background()
	config := &ClientConfig{
		APIKey: &drivers.GCPPrivateKey{
			Type:      "external_account", // wrong type — ServiceAccount expected
			ProjectID: "test-project",
		},
	}

	_, err := ResolveCredentials(ctx, config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "service_account")
}

func TestResolveCredentials_ZeroAPIKeyFallsThrough(t *testing.T) {
	ctx := context.Background()
	// Non-nil but zero-valued APIKey should be treated as absent, falling to ADC.
	// Use a real JSON file and t.Setenv so we never hit the metadata server or
	// depend on ambient developer credentials; behavior is hermetic and deterministic.
	setupTestADCFile(t)

	config := &ClientConfig{
		APIKey: &drivers.GCPPrivateKey{},
	}

	creds, err := ResolveCredentials(ctx, config)
	assert.NoError(t, err)
	assert.NotNil(t, creds)
}

func TestResolveCredentials_NilFallsToADC(t *testing.T) {
	ctx := context.Background()
	setupTestADCFile(t)

	config := &ClientConfig{}

	creds, err := ResolveCredentials(ctx, config)
	assert.NoError(t, err)
	assert.NotNil(t, creds)
}

func TestResolveCredentials_WIPTakesPriorityOverAPIKey(t *testing.T) {
	ctx := context.Background()
	config := &ClientConfig{
		WIPCredential: &drivers.GCPWIPCredential{
			Type:             "external_account",
			Audience:         "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/prov",
			SubjectTokenType: "urn:ietf:params:oauth:token-type:jwt",
			TokenURL:         "https://sts.googleapis.com/v1/token",
			CredentialSource: &drivers.GCPWIPCredentialSource{
				File: "/var/run/secrets/token",
			},
		},
		APIKey: &drivers.GCPPrivateKey{
			Type:      "service_account",
			ProjectID: "should-not-be-used",
		},
	}

	// WIP should be chosen; its type is ExternalAccount so parsing succeeds.
	creds, err := ResolveCredentials(ctx, config)
	assert.NoError(t, err)
	assert.NotNil(t, creds)
}

// testRSAPrivateKeyPEM returns a valid PKCS#1 RSA private key PEM generated at
// test runtime. No key material is committed; avoids secret-scanner false positives
// on static -----BEGIN * PRIVATE KEY----- blocks.
func testRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	blk := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(blk))
}

// setupTestADCFile writes a minimal service account JSON using testRSAPrivateKeyPEM
// and sets GOOGLE_APPLICATION_CREDENTIALS. Restores the previous env on test end.
func setupTestADCFile(t *testing.T) {
	t.Helper()
	sa := &drivers.GCPPrivateKey{
		Type:         "service_account",
		ProjectID:    "test-project-adc",
		PrivateKeyID: "key-id",
		PrivateKey:   testRSAPrivateKeyPEM(t),
		ClientEmail:  "adc@test-project-adc.iam.gserviceaccount.com",
		ClientID:     "999",
		AuthURI:      "https://accounts.google.com/o/oauth2/auth",
		TokenURI:     "https://oauth2.googleapis.com/token",
	}
	b, err := json.Marshal(sa)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	p := filepath.Join(t.TempDir(), "adc_creds.json")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", p)
}
