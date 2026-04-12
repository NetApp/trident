// Copyright 2026 NetApp, Inc. All Rights Reserved.

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
	// This metric only tracks the time spent from when an HTTP request is made to when the response is received.
	// This metric does not include the time spent waiting for a rate limit token or a semaphore lock.
	// i.e. "How long are outgoing API calls taking once they hit the wire, from start to finish?"
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
	// outgoingAPIRequestPendingDurationSeconds tracks the client-side queuing delay before
	// an outgoing API request is dispatched. This covers time spent waiting for a rate-limit
	// token (e.g. the Kubernetes client-go token bucket) or a concurrency slot (e.g. the
	// ONTAP semaphore). It does not include the HTTP round-trip, which is captured separately in outgoingAPIRequestDurationSeconds.
	// The goal of this metric is to track the latency introduced by the client-side rate limiting or lock contention around an API.
	// i.e. "How long are outgoing API requests waiting before they even hit the wire?"
	outgoingAPIRequestTokenDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "outgoing_api",
			Name:      "requests_limited_duration_seconds",
			Help:      "Duration of outgoing API calls waiting for a rate limit token or semaphore lock.",
			// Default K8s API QPS is 100 (10ms token refill), burst=50.
			// Buckets - 10ms (1 token), 50-100ms (5-10 tokens), 500ms-1s (moderate),
			// 2.5s-5s (heavy), 10s+ (severe sustained throttling).
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2.5, 5, 10},
		},
		outgoingAPIRequestSharedLabels,
	)
	// outgoingAPIRequestBackpressureTotal tracks how often the server signals overload
	// on outgoing API requests (e.g. 429, 503, EOF). Unlike outgoingAPIRequestRetryTotal,
	// this only increments on explicit server-side backpressure, not on all retryable failures.
	// i.e. "How often is the server telling us to back off?"
	outgoingAPIRequestBackpressureTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "outgoing_api",
			Name:      "requests_backpressure_total",
			Help:      "Total number of outgoing API requests that were rejected by the server due to overload (e.g. 429, 503, EOF).",
		},
		outgoingAPIRequestSharedLabels,
	)
	// outgoingAPIRequestRetryTotal tracks the total number of outgoing API requests that were retried.
	// i.e. "How many times did we have to retry an outgoing API request?"
	outgoingAPIRequestRetryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "outgoing_api",
			Name:      "requests_retried_total",
			Help:      "Total number of outgoing API requests that were retried.",
		},
		outgoingAPIRequestSharedLabels,
	)
	// outgoingAPIRequestsInFlight tracks the number of in-flight outgoing API requests.
	// i.e. "How many outgoing API calls are on the wire, to which targets, at a given point in time?"
	outgoingAPIRequestsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "outgoing_api",
			Name:      "requests_in_flight",
			Help:      "Number of in-flight outgoing API requests.",
		},
		outgoingAPIRequestSharedLabels,
	)
)

// IncomingAPITelemeter starts tracking an incoming request. It captures all incoming API metrics:
// in-flight count and request duration.
// client and method are required — they are Prometheus label fields.
// ctx is used only for lifecycle signals (cancellation, deadline).
func IncomingAPITelemeter(client, method string) Telemeter {
	return func(ctx context.Context) Recorder {
		fields := LogFields{
			"client": client,
			"method": method,
		}
		Logc(ctx).WithFields(fields).Trace("Recording incoming API request metrics.")

		// Record the start time.
		startTime := time.Now()

		// Increment the in-flight count.
		incomingAPIRequestsInFlight.WithLabelValues(client, method).Inc()

		var once sync.Once
		return func(errPtr *error) {
			once.Do(func() {
				// Decrement the in-flight count.
				incomingAPIRequestsInFlight.WithLabelValues(client, method).Dec()

				// Calculate the elapsed time.
				elapsed := time.Since(startTime).Seconds()

				// Resolve the status.
				status := resolveStatus(ctx, errPtr)
				incomingAPIRequestDurationSeconds.WithLabelValues(status, client, method).Observe(elapsed)

				Logc(ctx).WithFields(fields).WithFields(LogFields{
					"status":  status,
					"elapsed": elapsed,
				}).Trace("Recorded incoming API request metrics.")
			})
		}
	}
}

// OutgoingAPITelemeter starts tracking an outgoing request. It captures outgoing API metrics:
// in-flight count, request duration, and backpressure.
func OutgoingAPITelemeter(target, address, method string) Telemeter {
	return func(ctx context.Context) Recorder {
		fields := LogFields{
			"target":  target,
			"address": address,
			"method":  method,
		}
		Logc(ctx).WithFields(fields).Trace("Recording outgoing API request metrics.")

		// Record the start time.
		startTime := time.Now()

		// Increment the in-flight count.
		outgoingAPIRequestsInFlight.WithLabelValues(target, address, method).Inc()

		var once sync.Once
		return func(errPtr *error) {
			once.Do(func() {
				// Decrement the in-flight count.
				outgoingAPIRequestsInFlight.WithLabelValues(target, address, method).Dec()

				// Calculate the elapsed time.
				elapsed := time.Since(startTime).Seconds()

				// Resolve the status.
				status := resolveStatus(ctx, errPtr)

				// Record when we hit server-side backpressure.
				err := convert.ToVal(errPtr)
				if errors.IsServerBackPressureError(err) {
					outgoingAPIRequestBackpressureTotal.WithLabelValues(target, address, method).Inc()
				}

				// Record the duration.
				outgoingAPIRequestDurationSeconds.WithLabelValues(status, target, address, method).Observe(elapsed)

				Logc(ctx).WithFields(fields).WithFields(LogFields{
					"status":  status,
					"elapsed": elapsed,
				}).Trace("Recorded outgoing API request metrics.")
			})
		}
	}
}

func resolveStatus(ctx context.Context, errPtr *error) string {
	if err := convert.ToVal(errPtr); err == nil {
		return metricStatusSuccess
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		switch {
		case errors.Is(ctxErr, context.Canceled):
			return metricStatusCanceled
		case errors.Is(ctxErr, context.DeadlineExceeded):
			return metricStatusDeadlineExceeded
		}
	}
	return metricStatusFailure
}

// CaptureOutgoingAPIRequestTokenDuration records how long outgoing API requests waited for a rate limit token or lock.
// This does not include time spent on the wire.
// Sub-millisecond waits are ignored — they represent uncontended overhead, not real queuing pressure.
func CaptureOutgoingAPIRequestTokenDuration(ctx context.Context, target, address, method string, latency time.Duration) {
	if latency < time.Millisecond {
		// Ignore sub-millisecond waits — these are uncontended overhead, not real client-side queuing pressure.
		return
	}

	Logc(ctx).WithFields(LogFields{
		"target": target, "address": address, "method": method, "latency_seconds": latency.Seconds(),
	}).Trace("Recording outgoing API request token duration.")
	outgoingAPIRequestTokenDurationSeconds.WithLabelValues(target, address, method).Observe(latency.Seconds())
}

// CaptureOutgoingAPIRequestRetryTotal records the total number of outgoing API requests that were retried.
// This should only be used in cases when it is already known that a retry has occurred.
func CaptureOutgoingAPIRequestRetryTotal(ctx context.Context, target, address, method string) {
	Logc(ctx).WithFields(LogFields{
		"target": target, "address": address, "method": method,
	}).Trace("Incrementing outgoing retry API total.")
	outgoingAPIRequestRetryTotal.WithLabelValues(target, address, method).Inc()
}
