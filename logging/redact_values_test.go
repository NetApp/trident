// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRedactSecretsDoesNotModifyInput pins the property that lets a caller keep using the
// configuration it decoded after asking for a loggable rendering of it.
func TestRedactSecretsDoesNotModifyInput(t *testing.T) {
	decoded := map[string]interface{}{
		"username":      "Live-Sentinel-User",
		"managementLIF": "10.0.0.1",
		"virtualPool": []interface{}{
			map[string]interface{}{"poolName": "pool1", "password": "Live-Sentinel-Pass"},
		},
	}

	redacted := RedactSecrets(decoded)

	// Without these the input checks below would pass on a helper that simply returned its input.
	require.Equal(t, RedactedValue, redacted["username"])
	require.Equal(
		t, RedactedValue,
		redacted["virtualPool"].([]interface{})[0].(map[string]interface{})["password"],
		"nested configurations inside a list must be redacted in the copy",
	)
	require.Equal(t, "10.0.0.1", redacted["managementLIF"], "benign values must be carried into the copy")

	require.Equal(t, "Live-Sentinel-User", decoded["username"])
	require.Equal(
		t, "Live-Sentinel-Pass",
		decoded["virtualPool"].([]interface{})[0].(map[string]interface{})["password"],
		"nested configurations inside a list must keep their values",
	)
	require.Equal(t, "pool1", decoded["virtualPool"].([]interface{})[0].(map[string]interface{})["poolName"])
}

// TestRedactSecretsReturnsDeepCopy proves the copy shares no mutable node with its input. Only maps
// and slices are mutable in a decoded JSON tree, so writing through every container the result hands
// back is what shows the copy is deep rather than one level wide.
func TestRedactSecretsReturnsDeepCopy(t *testing.T) {
	decoded := map[string]interface{}{
		"managementLIF": "10.0.0.1",
		"dataLIFs":      []interface{}{"10.0.0.2", "10.0.0.3"},
		"nfs":           map[string]interface{}{"rsize": 65536},
		"virtualPool": []interface{}{
			map[string]interface{}{"poolName": "pool1"},
		},
		"tiers": []interface{}{
			[]interface{}{"performance", "capacity"},
		},
		"username": "Live-Sentinel-User",
	}

	redacted := RedactSecrets(decoded)
	require.Equal(t, RedactedValue, redacted["username"])

	// Write through every container in the result, including ones the redaction leaves alone.
	redacted["managementLIF"] = "written-through"
	redacted["dataLIFs"].([]interface{})[0] = "written-through"
	redacted["nfs"].(map[string]interface{})["rsize"] = 1
	redacted["virtualPool"].([]interface{})[0].(map[string]interface{})["poolName"] = "written-through"
	redacted["tiers"].([]interface{})[0].([]interface{})[0] = "written-through"

	require.Equal(t, "10.0.0.1", decoded["managementLIF"])
	require.Equal(t, []interface{}{"10.0.0.2", "10.0.0.3"}, decoded["dataLIFs"], "a top-level list must not be shared")
	require.Equal(t, map[string]interface{}{"rsize": 65536}, decoded["nfs"], "a nested map must not be shared")
	require.Equal(
		t, []interface{}{map[string]interface{}{"poolName": "pool1"}}, decoded["virtualPool"],
		"a map inside a list must not be shared",
	)
	require.Equal(
		t, []interface{}{[]interface{}{"performance", "capacity"}}, decoded["tiers"],
		"a list inside a list must not be shared",
	)
}

func TestIsSensitiveKeyMatchesEveryCase(t *testing.T) {
	for _, key := range []string{"secretKey", "secretkey", "SECRETKEY", "SecretKey", "secret_key"} {
		require.True(t, IsSensitiveKey(key), "%s names a credential", key)
	}

	for _, key := range []string{"managementLIF", "fsxFilesystemID", "apiRegion", ""} {
		require.False(t, IsSensitiveKey(key), "%s is not a credential", key)
	}
}

// TestRedactJSONBodyDropsCredentialValues covers the tridentctl debug trace, whose request body is
// the backend JSON from `tridentctl create backend -d`. The pattern-based redaction in this package
// only matches the keys it was written for, so clientSecret and friends need the key-based walk.
func TestRedactJSONBodyDropsCredentialValues(t *testing.T) {
	body := []byte(`{"backend_name":"b1","config":{"storageDriverName":"azure-netapp-files",` +
		`"username":"Body-Sentinel-User","password":"Body-Sentinel-Pass",` +
		`"clientSecret":"Body-Sentinel-ClientSecret","clientPrivateKey":"Body-Sentinel-PrivateKey",` +
		`"secretKey":"Body-Sentinel-SecretKey","apiKey":"Body-Sentinel-APIKey",` +
		`"chapInitiatorSecret":"Body-Sentinel-CHAP","location":"eastus"}}`)

	out := string(RedactJSONBody(body))

	// Guards the fixture: without a redaction marker in the line, the absence checks below would
	// pass on a body that was returned unchanged.
	require.Contains(t, out, RedactedValue)

	for _, sentinel := range []string{
		"Body-Sentinel-User", "Body-Sentinel-Pass", "Body-Sentinel-ClientSecret",
		"Body-Sentinel-PrivateKey", "Body-Sentinel-SecretKey", "Body-Sentinel-APIKey", "Body-Sentinel-CHAP",
	} {
		require.NotContains(t, out, sentinel)
	}

	require.Contains(t, out, "b1", "the body must stay useful for debugging")
	require.Contains(t, out, "eastus")
	require.Contains(t, out, "azure-netapp-files")

	// json.Marshal escapes < and >, which would make the marker unreadable in a log line.
	require.Contains(t, out, `"<REDACTED>"`)
	require.NotContains(t, out, `\u003cREDACTED\u003e`)
}

// TestRedactJSONBodyDoesNotModifyInput pins that redacting for the log leaves the body that is
// actually sent to the server alone.
func TestRedactJSONBodyDoesNotModifyInput(t *testing.T) {
	body := []byte(`{"backend_name":"b1","config":{"password":"Live-Sentinel-Pass"}}`)
	original := string(body)

	redacted := RedactJSONBody(body)

	require.NotContains(t, string(redacted), "Live-Sentinel-Pass")
	require.Equal(t, original, string(body), "the outbound body must keep its credential")
}

func TestRedactJSONBodyLeavesBodiesWithoutKeysAlone(t *testing.T) {
	for _, body := range [][]byte{
		nil,
		[]byte(``),
		[]byte(`{"backend_name":"b1"}`),
		[]byte(`not JSON`),
		[]byte(`["array","body"]`),
		[]byte(`"scalar body"`),
	} {
		require.Equal(t, string(body), string(RedactJSONBody(body)))
	}
}

// TestRedactSecretsLiftsCredentialsOutOfEndpoints pins the narrower treatment for keys that name a
// location: solidfire-san puts the credentials in the endpoint, but the host is the half of the
// value that makes a log line useful, so only the userinfo is removed.
func TestRedactSecretsLiftsCredentialsOutOfEndpoints(t *testing.T) {
	for _, test := range []struct{ name, in, want string }{
		{"userinfo removed and host kept", "https://admin:Sentinel-Pass@sf.example.com", "https://<REDACTED>@sf.example.com"},
		{"user-only userinfo removed", "https://admin@sf.example.com", "https://<REDACTED>@sf.example.com"},
		{"port kept", "https://admin:Sentinel-Pass@10.0.0.1:443", "https://<REDACTED>@10.0.0.1:443"},
		{"endpoint without credentials kept whole", "https://sf.example.com", "https://sf.example.com"},
		// A credential may contain '@' itself; the authority's last '@' is the split.
		{"password containing @ removed", "https://admin:Sen@tinclPass@10.0.0.1", "https://<REDACTED>@10.0.0.1"},
		{"path and query kept", "https://admin:Sentinel-Pass@sf.example.com/some/thing?x=1", "https://<REDACTED>@sf.example.com/some/thing?x=1"},
		// An '@' outside any userinfo is not a split we can trust, so the value is replaced.
		{"@ in the path fully replaced", "https://sf.example.com/a@Sentinel-Pass", RedactedValue},
		// An '@' the parser will not read as userinfo leaves no safe split between host and
		// credential, so the whole value is replaced.
		{"shape with no scheme fully replaced", "admin:Sentinel-Pass@10.0.0.1", RedactedValue},
	} {
		got, ok := RedactSecrets(map[string]interface{}{"endpoint": test.in})["endpoint"].(string)

		require.True(t, ok, test.name)
		require.Equal(t, test.want, got, test.name)
		require.NotContains(t, got, "Sentinel-Pass", test.name)
	}
}

func TestRedactSecretsReplacesAnEndpointThatIsNotAString(t *testing.T) {
	got, ok := RedactSecrets(map[string]interface{}{"endpoint": 443})["endpoint"].(string)

	require.True(t, ok)
	require.Equal(t, RedactedValue, got)
}
