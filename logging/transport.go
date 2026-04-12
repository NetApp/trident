// Copyright 2026 NetApp, Inc. All Rights Reserved.

package logging

import (
	"io"
	"net/http"

	"github.com/netapp/trident/utils/errors"
)

// MetricsTransport is an HTTP transport that records metrics for outgoing HTTP requests via RoundTripper.
type MetricsTransport struct {
	base   http.RoundTripper
	target ContextRequestTarget
}

type MetricsTransportOption func(*MetricsTransport)

func WithMetricsTransportTarget(target ContextRequestTarget) MetricsTransportOption {
	return func(m *MetricsTransport) {
		if target == "" {
			return
		}
		m.target = target
	}
}

// NewMetricsTransport creates a new MetricsTransport with the given options.
// If no options are provided, it defaults the target to ContextRequestTargetUnknown.
func NewMetricsTransport(
	base http.RoundTripper, options ...MetricsTransportOption,
) http.RoundTripper {
	transport := &MetricsTransport{
		base:   base,                        // Base transport is required.
		target: ContextRequestTargetUnknown, // Default to unknown target.
	}
	for _, option := range options {
		option(transport)
	}
	return transport
}

// RoundTrip captures the time on the wire of a request. It does not capture the time
// an HTTP client may be waiting for a lock or token.
//
// err serves double duty: the deferred recorder observes it for metrics,
// while the Kubernetes backpressure path returns nil to the caller without
// clearing err, so backpressure is still recorded without violating
// RoundTripper semantics.
func (m *MetricsTransport) RoundTrip(req *http.Request) (res *http.Response, err error) {
	// target: "kubernetes", "ontap", etc.,
	// host:   "10.0.0.1", "10.0.0.2", etc.,
	// method: "GET", "POST", etc.
	ctx, rec := NewContextBuilder(req.Context()).
		WithOutgoingAPIMetrics(m.target, req.URL.Host, req.Method).
		BuildContextAndTelemetry()
	defer rec(&err)

	res, err = m.base.RoundTrip(req.WithContext(ctx))
	if err != nil {
		// Handle the special case where ONTAP or any other API doesn't honor HTTP status codes.
		// This is a workaround for ONTAP's EOF handling when the API gives up.
		if errors.Is(err, io.EOF) && m.target == ContextRequestTargetONTAP {
			err = errors.WrapWithServerBackPressureError(err, "received EOF from server")
		}
		return res, err
	}

	// Check for HTTP-level backpressure.
	switch res.StatusCode {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		err = errors.ServerBackPressureError("received status: %d from the server", res.StatusCode)

		// ONTAP callers (LimitedRetryTransport, go-openapi, azgo) gate on err != nil
		// and do not inspect the response status code on the error path.
		if m.target == ContextRequestTargetONTAP {
			return res, err
		}

		// Kubernetes client-go depends on the response status, not the error.
		// Return the response with a nil error so client-go can inspect the status code.
		return res, nil
	}

	return res, nil
}
