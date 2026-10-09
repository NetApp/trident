// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
)

// RedactedValue stands in for a credential in log output.
const RedactedValue = "<REDACTED>"

// sensitiveKeys holds the backend configuration keys whose values are credentials, spelled as they
// appear in a spec and lowercased for matching, since encoding/json binds keys case-insensitively
// and a spec may write "Password" and still reach a driver. The set mirrors the fields each storage
// driver treats as a secret in its ExtractSecrets implementation, plus the secret references,
// workload-identity credentials, and cloud keys that have no ExtractSecrets to mirror:
// storage_drivers has a test pinning every credential-named config key to this set.
var sensitiveKeys = map[string]struct{}{
	"username":                  {},
	"password":                  {},
	"clientprivatekey":          {},
	"chapusername":              {},
	"chapinitiatorsecret":       {},
	"chaptargetusername":        {},
	"chaptargetinitiatorsecret": {},
	"clientid":                  {},
	"clientsecret":              {},
	"apikey":                    {},
	"secretkey":                 {},
	"adadminuser":               {},
	"private_key":               {},
	"private_key_id":            {},
	"credentials":               {},
	"wipcredential":             {},
	"wipcredentialconfig":       {},
	"access_key":                {},
	"access_key_id":             {},
	"secret_key":                {},
}

// embeddedCredentialKeys holds the keys that name a location rather than a secret, but which some
// driver fills in with credentials embedded in it: solidfire-san writes
// https://admin:password@10.0.0.1. Only the embedded part is removed, because the host is the
// useful half of the value and is not a credential.
var embeddedCredentialKeys = map[string]struct{}{
	"endpoint": {},
}

// IsSensitiveKey reports whether a configuration key is redacted, wholly or in part. Matching is
// case-insensitive for the same reason the sets are lowercased.
func IsSensitiveKey(key string) bool {
	lowered := strings.ToLower(key)
	if _, found := sensitiveKeys[lowered]; found {
		return true
	}

	_, found := embeddedCredentialKeys[lowered]

	return found
}

// hasEmbeddedCredential reports whether a key's value carries credentials inside a larger value that
// is worth keeping.
func hasEmbeddedCredential(key string) bool {
	_, found := embeddedCredentialKeys[strings.ToLower(key)]
	return found
}

// redactEmbeddedCredentials removes the userinfo from an endpoint URL and leaves the rest of the
// value alone, because the host is the half that makes the log line useful. The result is cut out of
// the raw value rather than rebuilt with url.URL.String, which would percent-encode the marker into
// %3CREDACTED%3E.
func redactEmbeddedCredentials(value interface{}) string {
	location, ok := value.(string)
	if !ok {
		return RedactedValue
	}

	parsed, err := url.Parse(location)
	if err != nil {
		return RedactedValue
	}

	if parsed.User == nil {
		if strings.Contains(location, "@") {
			// An '@' the parser did not read as userinfo leaves no way to tell which part is the
			// credential, so the whole value goes.
			return RedactedValue
		}

		return location
	}

	// The authority runs from "//" to the first separator, and its last '@' ends the userinfo; that is
	// the only place a credential that contains '@' itself can sit.
	const scheme = "://"

	authorityStart := strings.Index(location, scheme)
	if authorityStart < 0 {
		return RedactedValue
	}

	authorityStart += len(scheme)

	authorityEnd := len(location)
	if i := strings.IndexAny(location[authorityStart:], "/?#"); i >= 0 {
		authorityEnd = authorityStart + i
	}

	at := strings.LastIndex(location[authorityStart:authorityEnd], "@")
	if at < 0 {
		return RedactedValue
	}

	return location[:authorityStart] + RedactedValue + location[authorityStart+at:]
}

// RedactSecrets returns a copy of config with every credential-bearing value replaced at any depth,
// or with the credential lifted out of it where the surrounding value is worth keeping. The input is
// left untouched so that a caller that still holds the decoded configuration keeps the configuration
// it asked for.
func RedactSecrets(config map[string]interface{}) map[string]interface{} {
	safe := make(map[string]interface{}, len(config))

	for key, value := range config {
		if hasEmbeddedCredential(key) {
			safe[key] = redactEmbeddedCredentials(value)
			continue
		}

		if IsSensitiveKey(key) {
			safe[key] = RedactedValue
			continue
		}

		switch nested := value.(type) {
		case map[string]interface{}:
			safe[key] = RedactSecrets(nested)
		case []interface{}:
			safe[key] = redactSecretsInList(nested)
		default:
			safe[key] = value
		}
	}

	return safe
}

// redactSecretsInList walks the nested configurations inside list-valued settings such as virtual
// pools, which can carry their own credentials. Like RedactSecrets, it returns a copy.
func redactSecretsInList(values []interface{}) []interface{} {
	safe := make([]interface{}, 0, len(values))

	for _, value := range values {
		switch nested := value.(type) {
		case map[string]interface{}:
			safe = append(safe, RedactSecrets(nested))
		case []interface{}:
			safe = append(safe, redactSecretsInList(nested))
		default:
			safe = append(safe, value)
		}
	}

	return safe
}

// RedactJSONBody returns a logged representation of a JSON request or response body with every
// credential-bearing key's value replaced. Keys are reordered and numbers normalised by the
// decode/encode round trip, which is acceptable in a debug trace; the credential is not. A body
// that is not a JSON object is returned unchanged, since there is no key to redact by and
// rewriting it would hide whatever it does hold.
func RedactJSONBody(body []byte) []byte {
	if len(body) == 0 {
		return body
	}

	var config map[string]interface{}
	if err := json.Unmarshal(body, &config); err != nil {
		return body
	}

	safe, err := marshalNoEscapeHTML(RedactSecrets(config))
	if err != nil {
		return body
	}

	return safe
}

// marshalNoEscapeHTML keeps <REDACTED> readable; json.Marshal would write it as \u003cREDACTED\u003e.
func marshalNoEscapeHTML(value interface{}) ([]byte, error) {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(value); err != nil {
		return nil, err
	}

	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
