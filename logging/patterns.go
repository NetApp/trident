package logging

import "regexp"

// basicAuthorization masks the ONTAP management credentials in the ONTAP REST client's debug trace
// (backend config debug: [api]): the go-openapi runtime renders the request in wire form with
// httputil.DumpRequestOut, and the Redactor-wrapped logger installed in
// storage_drivers/ontap/api/ontap_rest.go is what keeps the credential out of the log. It cannot
// match a header set rendered as a Go map, so it is not a substitute for RedactedHeaders.
var basicAuthorization = redactedPattern{
	re:  regexp.MustCompile(`Authorization: Basic [A-Za-z0-9+/=]+`),
	rep: []byte("Authorization: Basic <REDACTED>"),
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
