// Copyright 2026 NetApp, Inc. All Rights Reserved.

package storagedrivers

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/netapp/trident/logging"
)

// credentialNameMarkers are the name fragments that make a backend configuration key worth checking
// against the redaction set. They are deliberately wider than the set: the point is to notice a new
// credential-shaped key, not to decide in advance which names turn out to hold secrets.
var credentialNameMarkers = []string{
	"password", "secret", "key", "token", "credential", "chap", "user", "cert",
}

// benignCredentialNamedKeys holds every configuration key whose name trips a marker above but whose
// value is not a credential, with the reason it is safe to log. Anything new that trips a marker has
// to end up either in logging's sensitive set or in this map, which is the reason for the test: the
// sensitive set is kept by hand, and AWSConfig's secretKey stayed out of it because nothing
// cross-checked the two. AWSConfig, SMBConfig and AzureNAS's encryption keys have no ExtractSecrets
// to compare against either, so a comparison with ExtractSecrets could not have found that gap.
var benignCredentialNamedKeys = map[string]string{
	// GCP service-account and workload-identity documents. The credential in each is apiKey or
	// wipCredential, both redacted; what is left names the token exchange rather than a token.
	"token_uri":                   "GCP STS endpoint URL",
	"token_url":                   "GCP workload-identity STS endpoint URL",
	"subject_token_type":          "OAuth token type identifier, e.g. urn:ietf:params:oauth:token-type:jwt",
	"credential_source":           "where a credential is read from, a file path or metadata server",
	"auth_provider_x509_cert_url": "GCP certificate endpoint URL",
	"client_x509_cert_url":        "GCP certificate endpoint URL",

	// Certificates are the half meant to be handed to the peer. The private half of the ONTAP
	// certificate pair is clientPrivateKey, which is redacted.
	"clientcertificate":    "ONTAP client certificate; its key is clientPrivateKey, which is redacted",
	"trustedcacertificate": "a CA certificate is distributed to peers rather than kept secret",

	// Names that only look like credentials.
	"usechap":   "boolean selecting CHAP; the CHAP values are the chap*Secret and chap*Username keys",
	"userest":   "boolean selecting the ONTAP REST backend",
	"userstate": "requested backend state, online or offline",
	"customerencryptionkeys": "azure-netapp-files maps a NetApp account to a Key Vault resource name; " +
		"azure_anf.go passes the value to api.CreateKeyVaultEndpoint rather than using it as key material",
}

func TestBackendConfigCredentialKeysAreRedacted(t *testing.T) {
	visited := map[reflect.Type]bool{}
	found := map[string]struct{}{}

	var walk func(target reflect.Type)
	walk = func(target reflect.Type) {
		if target == nil || visited[target] {
			return
		}
		visited[target] = true

		switch target.Kind() {
		case reflect.Pointer:
			walk(target.Elem())
		case reflect.Slice, reflect.Array, reflect.Map:
			walk(target.Elem())
		case reflect.Struct:
			for i := 0; i < target.NumField(); i++ {
				field := target.Field(i)
				if field.PkgPath != "" {
					continue
				}

				name := field.Name
				if tag, tagged := field.Tag.Lookup("json"); tagged {
					tagName := strings.Split(tag, ",")[0]
					if tagName == "-" {
						continue
					}
					if tagName != "" {
						name = tagName
					}
				}

				if namesCredential(name) {
					found[strings.ToLower(name)] = struct{}{}
				}

				walk(field.Type)
			}
		}
	}

	// The nested types a driver reaches for - the AWS block, the SMB block, workload-identity
	// credentials, pools and defaults - are discovered by the walk rather than listed here.
	for _, seed := range []interface{}{
		CommonStorageDriverConfig{},
		CommonStorageDriverConfigDefaults{},
		OntapStorageDriverConfig{},
		OntapStorageDriverConfigDefaults{},
		SolidfireStorageDriverConfig{},
		SolidfireStorageDriverConfigDefaults{},
		AzureNASStorageDriverConfig{},
		AzureNASStorageDriverConfigDefaults{},
		GCNVStorageDriverConfig{},
		GCNVStorageDriverConfigDefaults{},
		GCNVNASDriverConfig{},
		GCNVSANDriverConfig{},
	} {
		walk(reflect.TypeOf(seed))
	}

	// Guards the walk: a discovery step that silently found nothing would make the checks below pass.
	if len(found) < 20 {
		t.Fatalf("only %d credential-named config keys discovered, the walk is not reaching the configs", len(found))
	}

	var uncovered []string

	for key := range found {
		if logging.IsSensitiveKey(key) {
			continue
		}
		if _, benign := benignCredentialNamedKeys[key]; benign {
			continue
		}
		uncovered = append(uncovered, key)
	}
	sort.Strings(uncovered)

	if len(uncovered) > 0 {
		t.Errorf(
			"%d credential-named backend config key(s) are not redacted when a TridentBackendConfig is logged: %v\n"+
				"either add each to sensitiveKeys in logging/redact_values.go, or to benignCredentialNamedKeys "+
				"here with the reason its value is safe to log",
			len(uncovered), uncovered,
		)
	}
}

func namesCredential(key string) bool {
	lowered := strings.ToLower(key)

	for _, marker := range credentialNameMarkers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}

	return false
}
