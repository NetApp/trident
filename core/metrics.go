package core

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/netapp/trident/config"
)

var (
	tridentBuildInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "build_info",
			Help:      "Trident build and release information",
		},
		[]string{"trident_revision", "trident_version", "build_type"},
	)
	tridentBackendInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "backend_info",
			Help:      "Trident backend information",
		},
		[]string{"backend_type", "backend_name", "backend_uuid"},
	)
	backendsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "backend_count",
			Help:      "The total number of backends",
		},
		[]string{"backend_type", "backend_state"},
	)
	backendsByTypeGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "backend_count_by_type",
			Help:      "The total number of backends by type",
		},
		[]string{"type"},
	)
	backendsByStateGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "backend_count_by_state",
			Help:      "The total number of backends by state",
		},
		[]string{"state"},
	)
	backendAllocatedBytesGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "backend_allocated_bytes",
			Help:      "The allocated number of bytes in all backends",
		},
		[]string{"backend_type", "backend_uuid", "volume_state", "volume_type"},
	)
	volumesGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "volume_count",
			Help:      "The total number of volumes",
		},
		[]string{"backend_type", "backend_uuid", "volume_state", "volume_type"},
	)
	volumesByBackendGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "volume_count_by_backend",
			Help:      "The total number of volumes by backend type",
		},
		[]string{"backend"},
	)
	volumesByStateGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "volume_count_by_state",
			Help:      "The total number of volumes by state",
		},
		[]string{"state"},
	)
	volumesTotalBytesGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "volume_total_bytes",
			Help:      "The total number of bytes in all volumes",
		},
	)
	volumesTotalBytesByBackendGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "volume_total_bytes_by_backend",
			Help:      "The total number of bytes by backend type",
		},
		[]string{"backend"},
	)
	scGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "storage_class_count",
			Help:      "The total number of storage classes",
		},
	)
	nodeGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "node_count",
			Help:      "The total number of nodes",
		},
	)
	snapshotGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "snapshot_count",
			Help:      "The total number of snapshots",
		},
		[]string{"backend_type", "backend_uuid"},
	)
	snapshotAllocatedBytesGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "snapshot_allocated_bytes",
			Help:      "The allocated number of snapshot bytes in all backends",
		},
		[]string{"backend_type", "backend_uuid"},
	)
	operationDurationInMsSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  config.OrchestratorName,
			Subsystem:  "core",
			Name:       "operation_duration_in_milliseconds",
			Help:       "The duration of operations by backend",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"op", "success"},
	)
)
