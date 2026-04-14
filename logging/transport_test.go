// Copyright 2026 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
	stdErrors "errors"
	"io"
	"net/http"
	"strings"
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

// roundTrip is a shorthand that constructs a MetricsTransport from a stub and calls RoundTrip.
// It uses a unique host derived from the test name so Prometheus counters never collide across tests,
// while preserving the real target value for code-path correctness.
func roundTrip(
	t *testing.T, target ContextRequestTarget, base *stubRoundTripper, method string,
) (*http.Response, error, string, string, string) {
	t.Helper()
	host := strings.ReplaceAll(t.Name(), "/", "_")
	mt := &MetricsTransport{base: base, target: target}
	req := makeRequest(context.Background(), method, "https://"+host+"/path")
	resp, err := mt.RoundTrip(req)
	return resp, err, string(target), host, method
}

func TestMetricsTransport_ONTAP_EOF_WrapsAsServerBackPressure(t *testing.T) {
	base := &stubRoundTripper{resp: nil, err: io.EOF}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetONTAP))
	req := makeRequest(context.Background(), http.MethodGet, "https://ontap.local/cluster")

	_, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.Error(t, gotErr)
	assert.True(t, utilsErrors.IsServerBackPressureError(gotErr),
		"EOF from ONTAP should be wrapped as ServerBackPressureError")
}

func TestMetricsTransport_ONTAP_EOF_PreservesEOFInChain(t *testing.T) {
	base := &stubRoundTripper{resp: nil, err: io.EOF}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetONTAP))
	req := makeRequest(context.Background(), http.MethodGet, "https://ontap.local/cluster")

	_, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.True(t, stdErrors.Is(gotErr, io.EOF),
		"io.EOF must remain reachable in the error chain so LimitedRetryTransport can retry")
}

func TestMetricsTransport_NonONTAP_EOF_PassesThrough(t *testing.T) {
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

func TestMetricsTransport_TransportError_PassesThrough(t *testing.T) {
	someErr := stdErrors.New("connection refused")
	base := &stubRoundTripper{resp: nil, err: someErr}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetUnknown))
	req := makeRequest(context.Background(), http.MethodDelete, "https://service.local/item")

	_, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	assert.True(t, stdErrors.Is(gotErr, someErr),
		"non-EOF transport errors should be returned unchanged")
	assert.False(t, utilsErrors.IsServerBackPressureError(gotErr))
}

func TestMetricsTransport_ONTAP_HTTP429_RecordsBackpressureMetric(t *testing.T) {
	_, _, target, host, method := roundTrip(
		t, ContextRequestTargetONTAP,
		&stubRoundTripper{resp: makeResponse(http.StatusTooManyRequests)},
		http.MethodGet,
	)

	assert.Equal(t, float64(1),
		readCounter(outgoingAPIRequestBackpressureTotal, target, host, method),
		"backpressure counter must increment for ONTAP 429",
	)
}

func TestMetricsTransport_Kubernetes_HTTP429_RecordsBackpressureMetric(t *testing.T) {
	resp, err, target, host, method := roundTrip(
		t, ContextRequestTargetKubernetes,
		&stubRoundTripper{resp: makeResponse(http.StatusTooManyRequests)},
		http.MethodGet,
	)

	assert.NoError(t, err, "caller must get nil error per RoundTripper semantics")
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, float64(1),
		readCounter(outgoingAPIRequestBackpressureTotal, target, host, method),
		"backpressure counter must increment for Kubernetes 429 even though caller gets nil error",
	)
}

func TestMetricsTransport_Kubernetes_HTTP503_RecordsBackpressureMetric(t *testing.T) {
	resp, err, target, host, method := roundTrip(
		t, ContextRequestTargetKubernetes,
		&stubRoundTripper{resp: makeResponse(http.StatusServiceUnavailable)},
		http.MethodGet,
	)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, float64(1),
		readCounter(outgoingAPIRequestBackpressureTotal, target, host, method),
		"backpressure counter must increment for Kubernetes 503",
	)
}

func TestMetricsTransport_Kubernetes_HTTP504_RecordsBackpressureMetric(t *testing.T) {
	resp, err, target, host, method := roundTrip(
		t, ContextRequestTargetKubernetes,
		&stubRoundTripper{resp: makeResponse(http.StatusGatewayTimeout)},
		http.MethodGet,
	)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)
	assert.Equal(t, float64(1),
		readCounter(outgoingAPIRequestBackpressureTotal, target, host, method),
		"backpressure counter must increment for Kubernetes 504",
	)
}

func TestMetricsTransport_Unknown_HTTP429_ReturnsNilError(t *testing.T) {
	resp, err, target, host, method := roundTrip(
		t, ContextRequestTargetUnknown,
		&stubRoundTripper{resp: makeResponse(http.StatusTooManyRequests)},
		http.MethodGet,
	)

	assert.NoError(t, err,
		"unknown targets must return nil error to preserve RoundTripper semantics")
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, float64(1),
		readCounter(outgoingAPIRequestBackpressureTotal, target, host, method),
		"backpressure counter must still increment for unknown target 429",
	)
}

func TestMetricsTransport_ONTAP_EOF_RecordsBackpressureMetric(t *testing.T) {
	_, _, target, host, method := roundTrip(
		t, ContextRequestTargetONTAP,
		&stubRoundTripper{err: io.EOF},
		http.MethodGet,
	)

	assert.Equal(t, float64(1),
		readCounter(outgoingAPIRequestBackpressureTotal, target, host, method),
		"backpressure counter must increment for ONTAP EOF",
	)
}

func TestMetricsTransport_HTTP200_NoBackpressureMetric(t *testing.T) {
	_, _, target, host, method := roundTrip(
		t, ContextRequestTargetONTAP,
		&stubRoundTripper{resp: makeResponse(http.StatusOK)},
		http.MethodGet,
	)

	assert.Equal(t, float64(0),
		readCounter(outgoingAPIRequestBackpressureTotal, target, host, method),
		"backpressure counter must not increment for 200 OK",
	)
}

func TestMetricsTransport_HTTP200_RecordsDurationAsSuccess(t *testing.T) {
	_, _, target, host, method := roundTrip(
		t, ContextRequestTargetONTAP,
		&stubRoundTripper{resp: makeResponse(http.StatusOK)},
		http.MethodGet,
	)

	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusSuccess, target, host, method),
	)
}

func TestMetricsTransport_ONTAP_HTTP429_RecordsDurationAsFailure(t *testing.T) {
	_, _, target, host, method := roundTrip(
		t, ContextRequestTargetONTAP,
		&stubRoundTripper{resp: makeResponse(http.StatusTooManyRequests)},
		http.MethodGet,
	)

	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusFailure, target, host, method),
		"ONTAP 429 must be recorded as failure in the duration histogram",
	)
}

func TestMetricsTransport_Kubernetes_HTTP429_RecordsDurationAsFailure(t *testing.T) {
	_, err, target, host, method := roundTrip(
		t, ContextRequestTargetKubernetes,
		&stubRoundTripper{resp: makeResponse(http.StatusTooManyRequests)},
		http.MethodGet,
	)

	assert.NoError(t, err, "caller gets nil")
	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusFailure, target, host, method),
		"Kubernetes 429 must be recorded as failure in the duration histogram even though caller gets nil",
	)
}

func TestMetricsTransport_TransportError_RecordsDurationAsFailure(t *testing.T) {
	_, _, target, host, method := roundTrip(
		t, ContextRequestTargetUnknown,
		&stubRoundTripper{err: stdErrors.New("connection refused")},
		http.MethodGet,
	)

	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusFailure, target, host, method),
		"transport errors must be recorded as failure",
	)
}

func TestMetricsTransport_InFlight_ReturnsToZeroAfterSuccess(t *testing.T) {
	_, _, target, host, method := roundTrip(
		t, ContextRequestTargetONTAP,
		&stubRoundTripper{resp: makeResponse(http.StatusOK)},
		http.MethodGet,
	)

	assert.Equal(t, float64(0),
		readGauge(outgoingAPIRequestsInFlight, target, host, method),
		"in-flight gauge must return to zero after a completed request",
	)
}

func TestMetricsTransport_InFlight_ReturnsToZeroAfterError(t *testing.T) {
	_, _, target, host, method := roundTrip(
		t, ContextRequestTargetUnknown,
		&stubRoundTripper{err: stdErrors.New("fail")},
		http.MethodGet,
	)

	assert.Equal(t, float64(0),
		readGauge(outgoingAPIRequestsInFlight, target, host, method),
		"in-flight gauge must return to zero after an error",
	)
}

func TestMetricsTransport_InFlight_ReturnsToZeroAfterKubernetesBackpressure(t *testing.T) {
	_, _, target, host, method := roundTrip(
		t, ContextRequestTargetKubernetes,
		&stubRoundTripper{resp: makeResponse(http.StatusTooManyRequests)},
		http.MethodGet,
	)

	assert.Equal(t, float64(0),
		readGauge(outgoingAPIRequestsInFlight, target, host, method),
		"in-flight gauge must return to zero after Kubernetes backpressure",
	)
}
