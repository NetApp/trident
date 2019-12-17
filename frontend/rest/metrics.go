package rest

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/netapp/trident/config"
)

var (
	restOpsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "rest",
			Name:      "ops_total",
			Help:      "The total number of handled REST operations",
		},
		[]string{"op", "route"},
	)
	restOpsSecondsTotal = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  config.OrchestratorName,
			Subsystem:  "rest",
			Name:       "ops_seconds_total",
			Help:       "The total number of seconds spent handling REST operations",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"op", "route"},
	)
)
