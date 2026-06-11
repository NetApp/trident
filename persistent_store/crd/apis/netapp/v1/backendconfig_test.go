// Copyright 2026 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestTridentBackendConfigSpec_ToString_redactsGCNVAPIKey(t *testing.T) {
	specJSON := `{
		"version": 1,
		"storageDriverName": "ontap-nas",
		"credentials": {"name": "gcnv-sa-secret", "type": "secret"},
		"gcnv": {
			"proxyURL": "https://netapp.googleapis.com",
			"apiKey": {
				"type": "service_account",
				"private_key": "FAKE-PRIVATE-KEY-VALUE",
				"private_key_id": "key-id-123"
			},
			"wipCredential": {"audience": "test", "serviceAccountEmail": "sa@test"}
		}
	}`

	spec := &TridentBackendConfigSpec{
		RawExtension: runtime.RawExtension{Raw: json.RawMessage(specJSON)},
	}
	out := spec.ToString()

	require.NotContains(t, out, "SECRET")
	require.NotContains(t, out, "key-id-123")
	require.NotContains(t, out, "gcnv-sa-secret")
	require.Contains(t, out, "credentials:<REDACTED>")
	require.Contains(t, out, "apiKey:<REDACTED>")
	require.Contains(t, out, "wipCredential:<REDACTED>")
	require.Contains(t, out, "proxyURL:https://netapp.googleapis.com")
}

func TestTridentBackendConfigSpec_ToString_redactsNativeGCNVAPIKey(t *testing.T) {
	specJSON := `{
		"version": 1,
		"storageDriverName": "gcnv-nas",
		"apiKey": {"private_key": "native-secret", "private_key_id": "native-id"},
		"wipCredentialConfig": {"audience": "a"}
	}`

	spec := &TridentBackendConfigSpec{
		RawExtension: runtime.RawExtension{Raw: json.RawMessage(specJSON)},
	}
	out := spec.ToString()

	require.NotContains(t, out, "native-secret")
	require.NotContains(t, out, "native-id")
	require.Contains(t, out, "apiKey:<REDACTED>")
	require.Contains(t, out, "wipCredentialConfig:<REDACTED>")
}

func TestTridentBackendConfigSpec_ToString_redactsNestedWIPCredential(t *testing.T) {
	specJSON := `{
		"version": 1,
		"storageDriverName": "ontap-nas",
		"gcnv": {
			"proxyURL": "https://netapp.googleapis.com",
			"wipCredential": {
				"audience": "//iam.googleapis.com/projects/123",
				"credentialSource": {"file": "/var/run/secrets/token"},
				"subjectTokenType": "urn:ietf:params:oauth:token-type:jwt",
				"tokenURL": "https://sts.googleapis.com/v1/token",
				"type": "external_account"
			}
		}
	}`

	spec := &TridentBackendConfigSpec{
		RawExtension: runtime.RawExtension{Raw: json.RawMessage(specJSON)},
	}
	out := spec.ToString()

	require.NotContains(t, out, "/var/run/secrets/token")
	require.NotContains(t, out, "sts.googleapis.com")
	require.Contains(t, out, "wipCredential:<REDACTED>")
}
