package logging

import "regexp"

var basicAuthorization = redactedPattern{
	re:  regexp.MustCompile(`Authorization: Basic [A-Za-z0-9+/=]+`),
	rep: []byte("Authorization: Basic <REDACTED>"),
}

// csiSecrets is a pattern intended to match the logging of the secrets field in CSI gRPC requests from the CSI sidecars
// Example match: `secrets: <key: \"foo\" value: \"bar\" >`
// Example match: `secrets:<key:\\\"foo\"value:'\\\"bar\" >`
// Not a match with \" in key or value: `secrets: <key:\"f\"oo\" value:'\"bar\" >`
var csiSecrets = redactedPattern{
	re:  regexp.MustCompile(`secrets:\s*<key:\s*\\*\"([^\\\"])*\\*\"\s*value:\\*\"([^\\\"])*\\*\"\s*>`),
	rep: []byte("secrets:<REDACTED>"),
}

// chapAuthorization pattern intended to redact chap credentials in REST communication
// Example match: `"password":"<string of characters other than \""`
var chapAuthorization = redactedPattern{
	re:  regexp.MustCompile(`\\*"password\\*":\\*"([^\\"])*\\*"`),
	rep: []byte("\\\"password\\\":<REDACTED>"),
}

// chapAuthorizationUser pattern intended to redact chap usernames in REST communication
// Example match: `"user":"<string of characters other than \""`
var chapAuthorizationUser = redactedPattern{
	re:  regexp.MustCompile(`\\*"user\\*":\\*"([^\\"])*\\*"`),
	rep: []byte("\\\"user\\\":<REDACTED>"),
}

// backendCreateCHAPSecrets pattern intended to redact chap secrets during "tridentctl create backend" in debug mode
// Example match: `"chap(Target)InitiatorSecret":"<string of characters other than \ and \""`
var backendCreateCHAPSecrets = redactedPattern{
	re:  regexp.MustCompile(`\\*"chap(Target)*InitiatorSecret\\*":\\*"([^\\"])*\\*"`),
	rep: []byte("\\\"chap[Target]InitiatorSecrets\\\":<REDACTED>"),
}

// backendCreateCHAPUsername  pattern intended to redact chap usernames during "tridentctl create backend" in debug mode
// Example match: `"chap(Target)Username":"<string of characters other than \ and \""`
var backendCreateCHAPUsername = redactedPattern{
	re:  regexp.MustCompile(`\\*"chap(Target)*Username\\*":\\*"([^\\"])*\\*"`),
	rep: []byte("\\\"chap[Target]Username\\\":<REDACTED>"),
}

// backendAuthorization pattern intended to redact username during "tridentctl create backend" in debug mode
// Example match: `"password":"<string of characters other than \""`
var backendAuthorization = redactedPattern{
	re:  regexp.MustCompile(`\\*"username\\*":\\*"([^\\"])*\\*"`),
	rep: []byte("\\\"username\\\":<REDACTED>"),
}

// backendMapOneLevelNested matches map[...] in %+#v logs with at most one nested map[...]
// (e.g. credentialSource:map[file:...]). [^\]]* alone stops at the first ']' and leaks trailing fields.
const backendMapOneLevelNested = `(?:[^\[\]]|map\[[^\]]*\])*`

// backendSensitiveMap redacts backend credential maps in map-formatted logs (%+#v).
// Examples:
//
//	credentials:map[name:my-secret type:secret]
//	apiKey:map[private_key:... private_key_id:...]
//	wipCredentialConfig:map[audience:... credentialSource:map[file:...] ...]
//	wipCredential:map[audience:... credentialSource:map[file:...] ...]
var backendSensitiveMap = redactedPattern{
	re: regexp.MustCompile(
		`(credentials|apiKey|wipCredentialConfig|wipCredential):map\[` + backendMapOneLevelNested + `\]`,
	),
	rep: []byte("$1:<REDACTED>"),
}
