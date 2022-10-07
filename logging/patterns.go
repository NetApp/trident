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
