// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/utils/errors"
)

const (
	metricStatusSuccess          = "success"
	metricStatusFailure          = "failure"
	metricStatusCanceled         = "canceled"
	metricStatusDeadlineExceeded = "deadline_exceeded"
)

type (
	// Recorder records metrics by relying on a captured context and errors that have been staged by the Telemeter.
	Recorder func(err *error)
	// Telemeter stages metrics recording by capturing the current state of a context and returns a Recorder.
	Telemeter func(context.Context) Recorder
)

var (
	incomingAPIRequestSharedLabels = []string{"caller", "method"}
	// incomingAPIRequestDurationSeconds tracks the duration of incoming API requests.
	incomingAPIRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "incoming_api",
			Name:      "request_duration_seconds",
			Help:      "Duration of calls to incoming APIs from start to finish.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
		},
		append([]string{"status"}, incomingAPIRequestSharedLabels...),
	)
	// incomingAPIRequestsInFlight tracks the number of in-flight incoming API requests.
	incomingAPIRequestsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "incoming_api",
			Name:      "requests_in_flight",
			Help:      "Number of in-flight incoming API requests.",
		},
		incomingAPIRequestSharedLabels,
	)

	outgoingAPIRequestSharedLabels = []string{"target", "address", "method"}
	// outgoingAPIRequestDurationSeconds tracks the duration of outgoing API requests.
	// i.e. "How long are outgoing API calls taking?
	outgoingAPIRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "outgoing_api",
			Name:      "request_duration_seconds",
			Help:      "Duration of calls to external APIs from start to finish.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
		},
		append([]string{"status"}, outgoingAPIRequestSharedLabels...),
	)
	// outgoingAPIRequestsInFlight tracks the number of in-flight outgoing API requests.
	// i.e. "How many outgoing API calls are in-flight at any given time? And to which targets?"
	outgoingAPIRequestsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "outgoing_api",
			Name:      "requests_in_flight",
			Help:      "Number of in-flight outgoing API requests.",
		},
		outgoingAPIRequestSharedLabels,
	)
	outgoingAPIRequestLimitedDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "outgoing_api",
			Name:      "requests_limited_duration_seconds",
			Help:      "Duration of outgoing API calls that were throttled due to rate limiting.",
			// Default K8s API QPS is 100 (10ms token refill), burst=50.
			// Buckets - 10ms (1 token), 50-100ms (5-10 tokens), 500ms-1s (moderate),
			// 2.5s-5s (heavy), 10s+ (severe sustained throttling).
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2.5, 5, 10},
		},
		outgoingAPIRequestSharedLabels,
	)
	outgoingAPIRequestRetryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "outgoing_api",
			Name:      "requests_retried_total",
			Help:      "Total number of outgoing API requests that were retried.",
		},
		outgoingAPIRequestSharedLabels,
	)

	// Compile time safety; every telemeter must conform to the Telemeter type.
	_ Telemeter = IncomingAPIRequestDurationTelemeter
	_ Telemeter = IncomingAPIRequestInFlightTelemeter
	_ Telemeter = OutgoingAPIRequestDurationTelemeter
	_ Telemeter = OutgoingAPIRequestInFlightTelemeter
	_ Telemeter = OutgoingAPIRequestRetryTotalTelemeter
	_ Telemeter = OutgoingAPIRequestLimitedDurationTelemeter
)

// IncomingAPIRequestDurationTelemeter creates a Telemeter for measuring how long API requests take.
// The returned Recorder captures a context to update metrics.
func IncomingAPIRequestDurationTelemeter(ctx context.Context) Recorder {
	status := metricStatusSuccess
	client := getContextClient(ctx)
	method := getContextMethod(ctx)
	values := []string{
		client, // client is treated as the "caller" for incoming API metrics
		method, // API method
	}

	// Create a UUID for this request.
	duration := getContextDuration(ctx)
	startTime := time.Now()
	var once sync.Once
	return func(errPtr *error) {
		once.Do(func() {
			elapsed := time.Since(startTime).Seconds()
			if duration > 0 {
				elapsed = duration.Seconds()
			}
			if err := convert.ToVal(errPtr); err != nil {
				status = metricStatusFailure
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				switch ctxErr {
				case context.Canceled:
					status = metricStatusCanceled
				case context.DeadlineExceeded:
					status = metricStatusDeadlineExceeded
				}
			}

			values = append([]string{status}, values...)
			incomingAPIRequestDurationSeconds.WithLabelValues(values...).Observe(elapsed)
		})
	}
}

// IncomingAPIRequestInFlightTelemeter creates a Telemeter for gauging incoming API requests.
// The returned Recorder captures a context to update metrics.
func IncomingAPIRequestInFlightTelemeter(ctx context.Context) Recorder {
	client := getContextClient(ctx)
	method := getContextMethod(ctx)
	values := []string{
		client, // client is treated as the "caller" for incoming API metrics
		method, // API method
	}

	// Increment the count with appropriate values.
	incomingAPIRequestsInFlight.WithLabelValues(values...).Inc()
	var once sync.Once
	return func(_ *error) {
		once.Do(func() {
			incomingAPIRequestsInFlight.WithLabelValues(values...).Dec()
		})
	}
}

// OutgoingAPIRequestDurationTelemeter creates a Telemeter for measuring how long outgoing API requests take.
// The returned Recorder captures a context to update metrics.
func OutgoingAPIRequestDurationTelemeter(ctx context.Context) Recorder {
	status := metricStatusSuccess
	target := getContextTarget(ctx)
	address := getContextAddress(ctx)
	method := getContextMethod(ctx)
	values := []string{
		target,
		address,
		method,
	}

	// Create a UUID for this request.
	duration := getContextDuration(ctx)
	startTime := time.Now()
	var once sync.Once
	return func(errPtr *error) {
		once.Do(func() {
			elapsed := time.Since(startTime).Seconds()
			if duration > 0 {
				elapsed = duration.Seconds()
			}

			if err := convert.ToVal(errPtr); err != nil {
				status = metricStatusFailure
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				switch ctxErr {
				case context.Canceled:
					status = metricStatusCanceled
				case context.DeadlineExceeded:
					status = metricStatusDeadlineExceeded
				}
			}

			values = append([]string{status}, values...)
			outgoingAPIRequestDurationSeconds.WithLabelValues(values...).Observe(elapsed)
		})
	}
}

// OutgoingAPIRequestInFlightTelemeter creates a Telemeter for gauging outgoing API requests.
// The returned Recorder captures a context to update metrics.
func OutgoingAPIRequestInFlightTelemeter(ctx context.Context) Recorder {
	target := getContextTarget(ctx)
	address := getContextAddress(ctx)
	method := getContextMethod(ctx)
	values := []string{
		target,
		address,
		method,
	}

	outgoingAPIRequestsInFlight.WithLabelValues(values...).Inc()
	var once sync.Once
	return func(_ *error) {
		once.Do(func() {
			outgoingAPIRequestsInFlight.WithLabelValues(values...).Dec()
		})
	}
}

// OutgoingAPIRequestRetryTotalTelemeter creates a Telemeter for counting outgoing API requests that have been retried.
// Retries could be due to rate limiting or other transient errors.
// The returned Recorder captures a context to update metrics.
func OutgoingAPIRequestRetryTotalTelemeter(ctx context.Context) Recorder {
	target := getContextTarget(ctx)
	address := getContextAddress(ctx)
	method := getContextMethod(ctx)
	values := []string{
		target,
		address,
		method,
	}

	return func(errPtr *error) {
		err := convert.ToVal(errPtr)
		if err == nil {
			return
		}
		// This telemeter only cares about rate limiting and throttling errors.
		if errors.IsMustRetryError(err) {
			outgoingAPIRequestRetryTotal.WithLabelValues(values...).Inc()
		}
	}
}

// OutgoingAPIRequestLimitedDurationTelemeter creates a Telemeter that measures the duration of
// outgoing API requests that were limited due to rate limiting. This telemeter requires a duration
// in the supplied context, otherwise it will not record any metrics.
// The returned Recorder captures a context to update metrics.
func OutgoingAPIRequestLimitedDurationTelemeter(ctx context.Context) Recorder {
	target := getContextTarget(ctx)
	address := getContextAddress(ctx)
	method := getContextMethod(ctx)
	values := []string{
		target,
		address,
		method,
	}

	duration := getContextDuration(ctx)
	var once sync.Once
	return func(errPtr *error) {
		once.Do(func() {
			// If no duration was set in the context, do not record any metrics.
			if duration <= 0 {
				return
			}

			// Check if the error indicates rate limiting.
			// If an error wasn't  it for this telemeter.
			var err error
			if err = convert.ToVal(errPtr); err == nil {
				return
			}

			// This telemeter only cares about rate limiting and throttling errors.
			if errors.IsTooManyRequestsError(err) {
				elapsed := duration.Seconds()
				outgoingAPIRequestLimitedDurationSeconds.WithLabelValues(values...).Observe(elapsed)
			}
		})
	}
}
