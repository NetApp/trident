// Copyright 2026 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
	stdErrors "errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	utilsErrors "github.com/netapp/trident/utils/errors"
)

// stubRoundTripper allows controlling the response and error returned by RoundTrip.
type stubRoundTripper struct {
	resp *http.Response
	err  error
}

func (s *stubRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return s.resp, s.err
}

func makeRequest(ctx context.Context, method, url string) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, method, url, nil)
	return req
}

func makeResponse(statusCode int) *http.Response {
	return &http.Response{StatusCode: statusCode}
}

// ---- ONTAP EOF handling ----

func TestMetricsTransport_ONTAP_EOF_WrapsAsServerBackPressure(t *testing.T) {
	// ONTAP signals overload with a bare EOF instead of an HTTP status code.
	// MetricsTransport must wrap it as a ServerBackPressureError so the telemeter
	// increments the backpressure counter correctly.
	base := &stubRoundTripper{resp: nil, err: io.EOF}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetONTAP))
	req := makeRequest(context.Background(), http.MethodGet, "https://ontap.local/cluster")

	_, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.Error(t, gotErr)
	assert.True(t, utilsErrors.IsServerBackPressureError(gotErr),
		"EOF from ONTAP should be wrapped as ServerBackPressureError")
}

func TestMetricsTransport_ONTAP_EOF_PreservesEOFInChain(t *testing.T) {
	// LimitedRetryTransport uses errors.Is(err, io.EOF) to decide whether to retry.
	// The wrapping must preserve io.EOF in the chain so retries still trigger.
	base := &stubRoundTripper{resp: nil, err: io.EOF}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetONTAP))
	req := makeRequest(context.Background(), http.MethodGet, "https://ontap.local/cluster")

	_, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.True(t, stdErrors.Is(gotErr, io.EOF),
		"io.EOF must remain reachable in the error chain so LimitedRetryTransport can retry")
}

func TestMetricsTransport_NonONTAP_EOF_PassesThrough(t *testing.T) {
	// Only ONTAP gets EOF-to-backpressure wrapping; other targets pass EOF through unchanged.
	base := &stubRoundTripper{resp: nil, err: io.EOF}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetKubernetes))
	req := makeRequest(context.Background(), http.MethodGet, "https://k8s.local/api/v1/pods")

	_, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.Error(t, gotErr)
	assert.False(t, utilsErrors.IsServerBackPressureError(gotErr),
		"non-ONTAP EOF should not be wrapped as ServerBackPressureError")
	assert.True(t, stdErrors.Is(gotErr, io.EOF),
		"original io.EOF should be returned unchanged for non-ONTAP targets")
}

// ---- HTTP status code backpressure detection ----

func TestMetricsTransport_HTTP429_WrapsAsServerBackPressure(t *testing.T) {
	base := &stubRoundTripper{resp: makeResponse(http.StatusTooManyRequests), err: nil}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetONTAP))
	req := makeRequest(context.Background(), http.MethodGet, "https://ontap.local/cluster")

	gotResp, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.NotNil(t, gotResp, "response should be returned alongside the error for caller inspection")
	assert.True(t, utilsErrors.IsServerBackPressureError(gotErr),
		"HTTP 429 should be wrapped as ServerBackPressureError")
}

func TestMetricsTransport_HTTP503_WrapsAsServerBackPressure(t *testing.T) {
	base := &stubRoundTripper{resp: makeResponse(http.StatusServiceUnavailable), err: nil}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetONTAP))
	req := makeRequest(context.Background(), http.MethodGet, "https://ontap.local/cluster")

	gotResp, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.NotNil(t, gotResp)
	assert.True(t, utilsErrors.IsServerBackPressureError(gotErr),
		"HTTP 503 should be wrapped as ServerBackPressureError")
}

func TestMetricsTransport_HTTP504_WrapsAsServerBackPressure(t *testing.T) {
	base := &stubRoundTripper{resp: makeResponse(http.StatusGatewayTimeout), err: nil}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetONTAP))
	req := makeRequest(context.Background(), http.MethodGet, "https://ontap.local/cluster")

	gotResp, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.NotNil(t, gotResp)
	assert.True(t, utilsErrors.IsServerBackPressureError(gotErr),
		"HTTP 504 should be wrapped as ServerBackPressureError")
}

// ---- Non-ONTAP targets preserve RoundTripper semantics for backpressure status codes ----

func TestMetricsTransport_Kubernetes_HTTP429_ReturnsNilError(t *testing.T) {
	base := &stubRoundTripper{resp: makeResponse(http.StatusTooManyRequests), err: nil}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetKubernetes))
	req := makeRequest(context.Background(), http.MethodGet, "https://k8s.local/api/v1/pods")

	gotResp, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.NotNil(t, gotResp, "response must be returned so client-go can inspect StatusCode")
	assert.Equal(t, http.StatusTooManyRequests, gotResp.StatusCode)
	assert.NoError(t, gotErr,
		"non-ONTAP targets must return nil error for HTTP responses to preserve RoundTripper semantics")
}

func TestMetricsTransport_Kubernetes_HTTP503_ReturnsNilError(t *testing.T) {
	base := &stubRoundTripper{resp: makeResponse(http.StatusServiceUnavailable), err: nil}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetKubernetes))
	req := makeRequest(context.Background(), http.MethodGet, "https://k8s.local/api/v1/pods")

	gotResp, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.NotNil(t, gotResp)
	assert.Equal(t, http.StatusServiceUnavailable, gotResp.StatusCode)
	assert.NoError(t, gotErr)
}

func TestMetricsTransport_Kubernetes_HTTP504_ReturnsNilError(t *testing.T) {
	base := &stubRoundTripper{resp: makeResponse(http.StatusGatewayTimeout), err: nil}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetKubernetes))
	req := makeRequest(context.Background(), http.MethodGet, "https://k8s.local/api/v1/pods")

	gotResp, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.NotNil(t, gotResp)
	assert.Equal(t, http.StatusGatewayTimeout, gotResp.StatusCode)
	assert.NoError(t, gotErr)
}

func TestMetricsTransport_HTTP200_Success(t *testing.T) {
	base := &stubRoundTripper{resp: makeResponse(http.StatusOK), err: nil}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetONTAP))
	req := makeRequest(context.Background(), http.MethodGet, "https://ontap.local/cluster")

	gotResp, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.NotNil(t, gotResp)
	assert.NoError(t, gotErr, "HTTP 200 should return no error")
}

// ---- Non-backpressure transport errors pass through unchanged ----

func TestMetricsTransport_TransportError_PassesThrough(t *testing.T) {
	// Connection-level errors that are not EOF pass through as-is for all targets.
	someErr := stdErrors.New("connection refused")
	base := &stubRoundTripper{resp: nil, err: someErr}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetUnknown))
	req := makeRequest(context.Background(), http.MethodDelete, "https://service.local/item")

	_, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.True(t, stdErrors.Is(gotErr, someErr),
		"non-EOF transport errors should be returned unchanged")
	assert.False(t, utilsErrors.IsServerBackPressureError(gotErr))
}
