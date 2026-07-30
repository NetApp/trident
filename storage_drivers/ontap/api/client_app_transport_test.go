// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
)

// roundTripFunc adapts a function into an http.RoundTripper for tests.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestClientAppHeaderValue verifies the computed header value matches the
// documented format "Trident/<OrchestratorVersion.ShortString()>".
func TestClientAppHeaderValue(t *testing.T) {
	want := "Trident/" + tridentconfig.OrchestratorVersion.ShortString()
	assert.Equal(t, want, ClientAppHeaderValue())
	assert.True(t, strings.HasPrefix(ClientAppHeaderValue(), "Trident/"),
		"header value must start with 'Trident/'")
}

// TestClientAppHeaderName locks in the header name required by ONTAP.
func TestClientAppHeaderName(t *testing.T) {
	assert.Equal(t, "X-Dot-Client-App", ClientAppHeader)
}

// TestClientAppTransport_RoundTrip verifies the transport stamps the header
// on every outgoing request.
func TestClientAppTransport_RoundTrip(t *testing.T) {
	const want = "Trident/26.06.1"

	var seen atomic.Int32
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen.Add(1)
		assert.Equal(t, want, r.Header.Get("X-Dot-Client-App"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	tr := NewClientAppTransport(base, want)

	for i := 0; i < 3; i++ {
		req, err := http.NewRequest(http.MethodGet, "http://example.invalid/api", nil)
		assert.NoError(t, err)
		resp, err := tr.RoundTrip(req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		_ = resp.Body.Close()

		// The caller's original request must not be mutated (RoundTripper contract).
		assert.Empty(t, req.Header.Get("X-Dot-Client-App"),
			"caller's request header must not be mutated")
	}

	assert.Equal(t, int32(3), seen.Load(), "base transport should be called once per RoundTrip")
}

// TestClientAppTransport_OverwritesExistingHeader verifies a pre-set header
// on the incoming request is overwritten with our value (without mutating the
// caller's request object).
func TestClientAppTransport_OverwritesExistingHeader(t *testing.T) {
	const want = "Trident/26.06.1"

	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, want, r.Header.Get("X-Dot-Client-App"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	})

	tr := NewClientAppTransport(base, want)
	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/api", nil)
	assert.NoError(t, err)
	req.Header.Set("X-Dot-Client-App", "SomeOther/1.0")

	resp, err := tr.RoundTrip(req)
	assert.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, "SomeOther/1.0", req.Header.Get("X-Dot-Client-App"),
		"caller's request header must be left alone")
}
