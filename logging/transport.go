// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	"io"
	"net/http"

	"github.com/netapp/trident/utils/errors"
)

// MetricsTransport is an HTTP transport that records metrics for outgoing requests.
// The layer, source and client fields must be set based on the HTTP client when initializing this transport.
type MetricsTransport struct {
	base       http.RoundTripper
	target     ContextRequestTarget
	telemeters []Telemeter
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

func WithMetricsTransportTelemeters(telemeters ...Telemeter) MetricsTransportOption {
	return func(m *MetricsTransport) {
		if telemeters == nil {
			return
		}
		m.telemeters = telemeters
	}
}

// NewMetricsTransport creates a new MetricsTransport with the given options.
// If no options are provided, it defaults the target to ContextRequestTargetUnknown
// and telemeters to all outgoing request telemeters.
func NewMetricsTransport(
	base http.RoundTripper, options ...MetricsTransportOption,
) http.RoundTripper {
	transport := &MetricsTransport{
		base:   base,                        // Base transport is required.
		target: ContextRequestTargetUnknown, // Default to unknown target.
		telemeters: []Telemeter{
			OutgoingAPIRequestDurationTelemeter,
			OutgoingAPIRequestInFlightTelemeter,
			OutgoingAPIRequestRetryTotalTelemeter,
		}, // Default to all outgoing request telemeters. This can be overridden.
	}
	for _, option := range options {
		option(transport)
	}
	return transport
}

func (m *MetricsTransport) RoundTrip(req *http.Request) (res *http.Response, err error) {
	ctx, rec := NewContextBuilder(req.Context()).
		WithTarget(m.target).      // "kubernetes", "ontap", etc.
		WithAddress(req.URL.Host). // IP Address
		WithMethod(req.Method).    // GET, POST, etc.
		WithTelemetry(m.telemeters...).
		BuildContextAndTelemetry()
	defer rec(&err)

	req = req.WithContext(ctx)
	res, err = m.base.RoundTrip(req)
	if err != nil {
		if m.target == ContextRequestTargetONTAP {
			// Treat EOF from ONTAP as a rate limiting error for metrics purposes.
			if err == io.EOF || (res != nil && res.StatusCode == http.StatusServiceUnavailable) {
				return res, errors.WrapWithTooManyRequestsError(err, "received EOF or 429 from ONTAP")
			}
		}
		return res, err
	}
	return res, nil
}
