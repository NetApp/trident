// Copyright 2025 NetApp, Inc. All Rights Reserved.

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

func TestMetricsTransport_ONTAP_EOF_TooManyRequests(t *testing.T) {
	// Arrange
	resp := &http.Response{StatusCode: http.StatusBadGateway}
	base := &stubRoundTripper{resp: resp, err: io.EOF}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetONTAP))
	req := makeRequest(context.Background(), http.MethodGet, "https://ontap.local/cluster")

	// Act
	gotResp, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	// Assert
	assert.Equal(t, resp, gotResp)
	assert.Error(t, gotErr)
	assert.True(t, utilsErrors.IsTooManyRequestsError(gotErr))
}

func TestMetricsTransport_DefaultTargetPassthrough(t *testing.T) {
	// Arrange
	resp := &http.Response{StatusCode: http.StatusServiceUnavailable}
	someErr := stdErrors.New("boom")
	base := &stubRoundTripper{resp: resp, err: someErr}
	mt := NewMetricsTransport(base, WithMetricsTransportTarget(ContextRequestTargetUnknown))
	req := makeRequest(context.Background(), http.MethodDelete, "https://service.local/item")

	// Act
	gotResp, gotErr := mt.(*MetricsTransport).RoundTrip(req)

	// Assert returns original error
	assert.Equal(t, resp, gotResp)
	assert.Error(t, gotErr)
	assert.True(t, stdErrors.Is(gotErr, someErr))
}
