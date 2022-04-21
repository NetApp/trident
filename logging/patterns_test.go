package logging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicAuthorizationRedactor(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{
			name:     "whitespace follows token",
			line:     "Authorization: Basic aGkgbW9tIEknbSBob21lCg== ",
			expected: "Authorization: Basic <REDACTED> ",
		},
		{
			name:     "bearer token",
			line:     "Authorization: Bearer aXQncyByYWluaW5nIGJlYXJzIG91dCB0aGVyZSB0b2RheQo=",
			expected: "Authorization: Bearer aXQncyByYWluaW5nIGJlYXJzIG91dCB0aGVyZSB0b2RheQo=",
		},
		{
			name:     "trident log line",
			line:     "time=\"2022-04-19T16:10:21Z\" level=debug msg=\"GET /api/network/ip/interfaces?fields=%2A%2A&services=data_nfs&svm.name=svm0 HTTP/1.1\\r\\nHost: 127.0.0.1\\r\\nUser-Agent: Go-http-client/1.1\\r\\nAccept: application/hal+json\\r\\nAccept: application/json\\r\\nAuthorization: Basic V2hlbiB5b3UgY29tZSB0byBhIGZvcmsgaW4gdGhlIHJvYWQsIHRha2UgaXQu\\r\\nAccept-Encoding: gzip\\r\\n\\r\\n\\n\"",
			expected: "time=\"2022-04-19T16:10:21Z\" level=debug msg=\"GET /api/network/ip/interfaces?fields=%2A%2A&services=data_nfs&svm.name=svm0 HTTP/1.1\\r\\nHost: 127.0.0.1\\r\\nUser-Agent: Go-http-client/1.1\\r\\nAccept: application/hal+json\\r\\nAccept: application/json\\r\\nAuthorization: Basic <REDACTED>\\r\\nAccept-Encoding: gzip\\r\\n\\r\\n\\n\"",
		},
		{
			name:     "quotes around header",
			line:     "\"Authorization: Basic WWVzLCBJJ20gYmVpbmcgZm9sbG93ZWQgYnkgYSBtb29uc2hhZG93\"",
			expected: "\"Authorization: Basic <REDACTED>\"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := basicAuthorization.re.ReplaceAll([]byte(test.line), basicAuthorization.rep)
			assert.Equal(t, test.expected, string(actual))
		})
	}
}
