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

// TestTridentBackendConfigSpec_ToString_redactsScalarCredentials covers the shape the pattern-based
// redaction could not reach: %+v renders a decoded map as key:value with no quotes, so the
// quote-anchored username and password patterns in the logging package never match it.
func TestTridentBackendConfigSpec_ToString_redactsScalarCredentials(t *testing.T) {
	specJSON := `{
		"version": 1,
		"storageDriverName": "ontap-nas",
		"managementLIF": "10.0.0.1",
		"username": "admin",
		"password": "Scalar-Sentinel-Pass",
		"clientPrivateKey": "Scalar-Sentinel-Key",
		"useCHAP": true,
		"chapUsername": "chap-user",
		"chapInitiatorSecret": "Scalar-Sentinel-Chap",
		"nfsMountOptions": "nfsvers=4"
	}`

	out := toStringOfSpec(t, specJSON)

	for _, secret := range []string{"Scalar-Sentinel-Pass", "Scalar-Sentinel-Key", "Scalar-Sentinel-Chap", "chap-user", "admin"} {
		require.NotContains(t, out, secret)
	}

	require.Contains(t, out, "password:<REDACTED>")
	require.Contains(t, out, "username:<REDACTED>")
	require.Contains(t, out, "chapInitiatorSecret:<REDACTED>")
	// Debug value of the line has to survive the redaction.
	require.Contains(t, out, "managementLIF:10.0.0.1")
	require.Contains(t, out, "nfsMountOptions:nfsvers=4")
}

// TestTridentBackendConfigSpec_ToString_redactsCaseVariants covers the bypass that follows from
// encoding/json binding keys case-insensitively: a spec spelled "Password" is still accepted by a
// storage driver.
func TestTridentBackendConfigSpec_ToString_redactsCaseVariants(t *testing.T) {
	specJSON := `{
		"version": 1,
		"storageDriverName": "ontap-nas",
		"USERNAME": "Case-Sentinel-User",
		"Password": "Case-Sentinel-Pass"
	}`

	out := toStringOfSpec(t, specJSON)

	require.NotContains(t, out, "Case-Sentinel-User")
	require.NotContains(t, out, "Case-Sentinel-Pass")
}

// TestTridentBackendConfigSpec_ToString_redactsVirtualPools covers credentials inside list-valued
// settings, which is how a virtual-pool backend carries per-pool credentials.
func TestTridentBackendConfigSpec_ToString_redactsVirtualPools(t *testing.T) {
	specJSON := `{
		"version": 1,
		"storageDriverName": "ontap-nas",
		"managementLIF": "10.0.0.1",
		"username": "Pool-Sentinel-User",
		"password": "Pool-Sentinel-Pass",
		"virtualPool": [
			{
				"poolName": "pool1",
				"server": "10.0.0.2",
				"username": "Pool-Sentinel-Virtual-User",
				"password": "Pool-Sentinel-Virtual-Pass"
			}
		]
	}`

	out := toStringOfSpec(t, specJSON)

	require.NotContains(t, out, "Pool-Sentinel-User")
	require.NotContains(t, out, "Pool-Sentinel-Pass")
	require.NotContains(t, out, "Pool-Sentinel-Virtual-User")
	require.NotContains(t, out, "Pool-Sentinel-Virtual-Pass")
	require.Contains(t, out, "poolName:pool1")
	require.Contains(t, out, "server:10.0.0.2")
}

// TestTridentBackendConfigSpec_ToString_redactsSolidfireEndpoint covers the one credential that is
// not a dedicated field: the solidfire-san endpoint embeds the administrator credentials. Only the
// userinfo goes, because the host is the half of the value that makes the log line useful.
func TestTridentBackendConfigSpec_ToString_redactsSolidfireEndpoint(t *testing.T) {
	specJSON := `{
		"version": 1,
		"storageDriverName": "solidfire-san",
		"endpoint": "https://admin:Endpoint-Sentinel-Pass@10.0.0.1",
		"storageVirtualMachineID": 1
	}`

	out := toStringOfSpec(t, specJSON)

	require.NotContains(t, out, "Endpoint-Sentinel-Pass")
	require.NotContains(t, out, "admin")
	require.Contains(t, out, "endpoint:https://<REDACTED>@10.0.0.1", "the host must stay visible")
	require.Contains(t, out, "storageVirtualMachineID:1")
}

// TestTridentBackendConfigSpec_ToString_redactsAWSSecretKey pins the FSx for ONTAP credentials.
// AWSConfig has no ExtractSecrets to mirror, and the ONTAP REST API models spell the same secret
// secret_key, so the underscore spelling reached the sensitive set and the config spelling did not.
func TestTridentBackendConfigSpec_ToString_redactsAWSSecretKey(t *testing.T) {
	specJSON := `{
		"version": 1,
		"storageDriverName": "ontap-nas",
		"managementLIF": "10.0.0.1",
		"aws": {
			"apiRegion": "us-east-1",
			"fsxFilesystemID": "fs-0123456789abcdef0",
			"apiKey": "AWS-Sentinel-APIKey",
			"secretKey": "AWS-Sentinel-SecretKey"
		}
	}`

	out := toStringOfSpec(t, specJSON)

	require.NotContains(t, out, "AWS-Sentinel-APIKey")
	require.NotContains(t, out, "AWS-Sentinel-SecretKey")
	require.Contains(t, out, "fs-0123456789abcdef0", "the FSx filesystem ID is not a secret and must stay readable")
	require.Contains(t, out, "us-east-1")
}

// TestTridentBackendConfigSpec_ToString_redactsSMBAdminUser covers the directory account name a
// secure-SMB backend is configured with. It is a name rather than a secret, and is masked for the
// same reason username is: it names the account whose credential is carried elsewhere.
func TestTridentBackendConfigSpec_ToString_redactsSMBAdminUser(t *testing.T) {
	specJSON := `{
		"version": 1,
		"storageDriverName": "ontap-nas",
		"managementLIF": "10.0.0.1",
		"smbConfig": {
			"useSecureCommands": true,
			"adAdminUser": "CORP-Sentinel-ADAdmin"
		}
	}`

	out := toStringOfSpec(t, specJSON)

	require.NotContains(t, out, "CORP-Sentinel-ADAdmin")
	require.Contains(t, out, "useSecureCommands:true", "the rest of the SMB config must stay readable")
}

func toStringOfSpec(t *testing.T, specJSON string) string {
	t.Helper()

	spec := &TridentBackendConfigSpec{
		RawExtension: runtime.RawExtension{Raw: json.RawMessage(specJSON)},
	}

	return spec.ToString()
}
