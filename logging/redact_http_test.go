// Copyright 2026 NetApp, Inc. All Rights Reserved.

package logging

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactedHeadersDropsCredentialHeaders(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string][]string
		wantKept    []string
		wantDropped []string
	}{
		{
			name: "canonical basic auth header",
			headers: http.Header{
				"Authorization": []string{"Basic aGVsbG86d29ybGQ="},
				"Content-Type":  []string{"application/json"},
			},
			wantKept:    []string{"Content-Type"},
			wantDropped: []string{"Authorization"},
		},
		{
			name: "api and secret key headers",
			headers: http.Header{
				"Api-Key":      []string{"key-value"},
				"Secret-Key":   []string{"secret-value"},
				"X-Trident-Id": []string{"abc"},
			},
			wantKept:    []string{"X-Trident-Id"},
			wantDropped: []string{"Api-Key", "Secret-Key"},
		},
		{
			name: "token, proxy and session headers",
			headers: http.Header{
				"Proxy-Authorization": []string{"Basic aGVsbG86d29ybGQ="},
				"X-Auth-Token":        []string{"ncm-token-value"},
				"Cookie":              []string{"session=abc123"},
				"Set-Cookie":          []string{"session=abc123; HttpOnly"},
				"Accept":              []string{"application/json"},
			},
			wantKept:    []string{"Accept"},
			wantDropped: []string{"Proxy-Authorization", "X-Auth-Token", "Cookie", "Set-Cookie"},
		},
		{
			name: "non-canonical spelling",
			// A header set built by hand rather than through net/http keeps whatever spelling it was
			// given, and the rendered log line would carry the credential verbatim.
			headers: map[string][]string{
				"authorization": []string{"Basic aGVsbG86d29ybGQ="},
				"AUTHORIZATION": []string{"Basic aGVsbG86d29ybGQ="},
				"content-type":  []string{"application/json"},
			},
			wantKept:    []string{"content-type"},
			wantDropped: []string{"authorization", "AUTHORIZATION"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := RedactedHeaders(test.headers)

			for _, kept := range test.wantKept {
				assert.Contains(t, got, kept, "benign headers must survive, so the fix is not to log nothing")
			}
			for _, dropped := range test.wantDropped {
				assert.NotContains(t, got, dropped)
			}
		})
	}
}

func TestRedactedHeadersDoesNotMutateInput(t *testing.T) {
	headers := http.Header{"Authorization": []string{"Basic aGVsbG86d29ybGQ="}}

	RedactedHeaders(headers)

	assert.Contains(t, headers, "Authorization", "the request must keep the header it needs to send")
}

func TestRedactedURLDropsUserinfo(t *testing.T) {
	parsed, err := url.Parse("http://admin:S3cr3tP%40ss@trident.trident.svc:17201/trident/v1/backend?verbose=true")
	require.NoError(t, err)

	safe := RedactedURL(parsed)

	assert.Nil(t, safe.User)
	assert.Equal(t, "trident.trident.svc:17201", safe.Host)
	assert.Equal(t, "/trident/v1/backend", safe.Path)
	assert.Equal(t, "verbose=true", safe.RawQuery)

	// The original is what the client actually dials, so it must still carry the credentials.
	assert.NotNil(t, parsed.User)

	assert.Nil(t, RedactedURL(nil), "a URL that failed to parse must not panic the caller that logs it")
}
