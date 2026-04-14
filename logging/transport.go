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
func (m *MetricsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// target: "kubernetes", "ontap", etc.,
	// host:   "10.0.0.1", "10.0.0.2", etc.,
	// method: "GET", "POST", etc.
	// recErr is a separate local variable used by the deferred recorder so that the
	// Kubernetes backpressure path can return nil to the caller (RoundTripper semantics)
	// while still recording the backpressure error in metrics.
	var recErr error
	ctx, rec := NewContextBuilder(req.Context()).
		WithOutgoingAPIMetrics(m.target, req.URL.Host, req.Method).
		BuildContextAndTelemetry()
	defer rec(&recErr)

	res, err := m.base.RoundTrip(req.WithContext(ctx))
	if err != nil {
		recErr = err
		// Handle the special ONTAP case where the API may return EOF instead of
		// an HTTP backpressure status when the server gives up.
		if errors.Is(err, io.EOF) && m.target == ContextRequestTargetONTAP {
			recErr = errors.WrapWithServerBackPressureError(err, "received EOF from server")
		}
		return res, recErr
	}

	// Check for HTTP-level backpressure.
	switch res.StatusCode {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		// Set the recorder error to a backpressure error.
		// This should be set for all outgoing API requests that experience backpressure.
		// Some outgoing API requests handle backpressure differently, so switch on the target
		// to decide what to return.
		recErr = errors.ServerBackPressureError("received status: %d from the server", res.StatusCode)

		switch m.target {
		case ContextRequestTargetONTAP:
			// This breaks the standard RoundTripper semantics but is necessary to handle
			// the special case where ONTAP returns an EOF error when the API is too busy.
			// ONTAP callers gate on err != nil and may not inspect the HTTP status code,
			// so return a non-nil backpressure error.
			return res, recErr
		case ContextRequestTargetKubernetes:
			// Kubernetes client-go depends on the response status, not the error, but
			// the metrics telemetry still needs to record the backpressure error.
			// Therefore, handle this case explicitly by returning nil to the caller
			// and recording the backpressure error in metrics.
			return res, nil
		}
	}

	// Preserve standard RoundTripper semantics: return the response with nil error
	// and let callers inspect the status code. recErr is captured independently for metrics.
	return res, nil
}
