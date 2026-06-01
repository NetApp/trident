// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ratelimit

import (
	"math"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/netapp/trident/config"
)

// Metrics holds the Prometheus collectors for an AdaptiveRateLimiter instance.
// Callers provide a subsystem (e.g. "gcnv") and a label name (e.g. "project")
// so the emitted metric names are scoped to the backend that owns the limiter.
//
// The current-limit gauge is exposed in BOTH units: per-second (the limiter's
// native unit, conventional for Prometheus) and per-minute (the unit that
// customers see in GCP's quota docs). The two are kept in lockstep via
// SetCurrentPerSecond.
type Metrics struct {
	DecreasesTotal   *prometheus.CounterVec
	IncreasesTotal   *prometheus.CounterVec
	CurrentPerSecond *prometheus.GaugeVec
	CurrentPerMinute *prometheus.GaugeVec
	SuccessesTotal   *prometheus.CounterVec
}

// NewMetrics registers and returns a Metrics set for the given subsystem and
// label. subsystem is embedded in the Prometheus metric name (e.g. "gcnv"
// produces trident_gcnv_rate_limit_decreases_total). labelName is the
// cardinality key identifying the scope of each limiter (e.g. "project").
func NewMetrics(subsystem, labelName string) *Metrics {
	return &Metrics{
		DecreasesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.OrchestratorName,
				Subsystem: subsystem,
				Name:      "rate_limit_decreases_total",
				Help:      "Total count of adaptive rate limit decreases due to upstream throttling.",
			},
			[]string{labelName},
		),
		IncreasesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.OrchestratorName,
				Subsystem: subsystem,
				Name:      "rate_limit_increases_total",
				Help:      "Total count of adaptive rate limit additive increases after sustained success.",
			},
			[]string{labelName},
		),
		CurrentPerSecond: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: config.OrchestratorName,
				Subsystem: subsystem,
				Name:      "rate_limit_current_per_second",
				Help:      "Current adaptive rate limit, in requests per second (the limiter's native unit).",
			},
			[]string{labelName},
		),
		CurrentPerMinute: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: config.OrchestratorName,
				Subsystem: subsystem,
				Name:      "rate_limit_current_per_minute",
				Help:      "Current adaptive rate limit, in requests per minute (matches upstream quota wording, e.g. GCP's 1200/min).",
			},
			[]string{labelName},
		),
		SuccessesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.OrchestratorName,
				Subsystem: subsystem,
				Name:      "rate_limit_successes_total",
				Help:      "Total count of successful RPCs observed by the adaptive rate limiter.",
			},
			[]string{labelName},
		),
	}
}

// SetCurrentPerSecond updates both current-limit gauges (per-second and the
// derived per-minute companion) atomically from a single per-second value.
// Safe to call with a nil receiver — it becomes a no-op, so callers that may
// or may not have metrics wired up don't have to nil-check first.
func (m *Metrics) SetCurrentPerSecond(label string, perSecond float64) {
	if m == nil {
		return
	}
	m.CurrentPerSecond.WithLabelValues(label).Set(perSecond)
	m.CurrentPerMinute.WithLabelValues(label).Set(math.Round(perSecond * 60))
}
