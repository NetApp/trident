package core

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/netapp/trident/config"
)

var (
	// deprecated metrics start here.
	// Will be removed in the next release
	tridentBuildInfoDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "build_info",
			Help:      "Trident build and release information (deprecated)",
		},
		[]string{"trident_revision", "trident_version", "build_type"},
	)
	tridentBackendInfoDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "backend_info",
			Help:      "Trident backend information (deprecated)",
		},
		[]string{"backend_type", "backend_name", "backend_uuid"},
	)
	backendsGaugeDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "backend_count",
			Help:      "The total number of backends (deprecated)",
		},
		[]string{"backend_type", "backend_state"},
	)
	backendsByTypeGaugeDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "backend_count_by_type",
			Help:      "The total number of backends by type (deprecated)",
		},
		[]string{"type"},
	)
	backendsByStateGaugeDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "backend_count_by_state",
			Help:      "The total number of backends by state (deprecated)",
		},
		[]string{"state"},
	)
	volumeAllocatedBytesGaugeDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "volume_allocated_bytes",
			Help:      "The total allocated number of bytes grouped by backends and volumes (deprecated)",
		},
		[]string{"backend_type", "backend_uuid", "volume_state", "volume_type"},
	)
	volumesGaugeDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "volume_count",
			Help:      "The total number of volumes (deprecated)",
		},
		[]string{"backend_type", "backend_uuid", "volume_state", "volume_type"},
	)
	volumesByBackendGaugeDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "volume_count_by_backend",
			Help:      "The total number of volumes by backend type (deprecated)",
		},
		[]string{"backend"},
	)
	volumesByStateGaugeDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "volume_count_by_state",
			Help:      "The total number of volumes by state (deprecated)",
		},
		[]string{"state"},
	)
	volumesTotalBytesGaugeDeprecated = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "volume_total_bytes",
			Help:      "The total number of bytes in all volumes (deprecated)",
		},
	)
	volumesTotalBytesByBackendGaugeDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "volume_total_bytes_by_backend",
			Help:      "The total number of bytes by backend type (deprecated)",
		},
		[]string{"backend"},
	)
	scGaugeDeprecated = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "storage_class_count",
			Help:      "The total number of storage classes (deprecated)",
		},
	)
	nodeGaugeDeprecated = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "node_count",
			Help:      "The total number of nodes (deprecated)",
		},
	)
	snapshotGaugeDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "snapshot_count",
			Help:      "The total number of snapshots (deprecated)",
		},
		[]string{"backend_type", "backend_uuid"},
	)
	snapshotAllocatedBytesGaugeDeprecated = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "core",
			Name:      "snapshot_allocated_bytes",
			Help:      "The allocated number of snapshot bytes in all backends (deprecated)",
		},
		[]string{"backend_type", "backend_uuid"},
	)
	operationDurationInMsSummaryDeprecated = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  config.OrchestratorName,
			Subsystem:  "core",
			Name:       "operation_duration_in_milliseconds",
			Help:       "The duration of operations by backend (deprecated)",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"op", "success"},
	)
	// deprecated metrics end here.
	// New metrics start here
	tridentBuildInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "build_info",
			Help:      "Trident build and release information",
		},
		[]string{"trident_revision", "trident_version", "build_type"},
	)
	tridentBackendInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "backend_info",
			Help:      "Trident backend information",
		},
		[]string{"backend_type", "backend_name", "backend_uuid"},
	)
	backendsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "backend_count",
			Help:      "The total number of backends",
		},
		[]string{"backend_type", "backend_state"},
	)
	volumeAllocatedBytesGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "volume_allocated_bytes",
			Help:      "The total allocated number of bytes grouped by backends and volumes",
		},
		[]string{"backend_type", "backend_uuid", "volume_state", "volume_type"},
	)
	volumesGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "volume_count",
			Help:      "The total number of volumes",
		},
		[]string{"backend_type", "backend_uuid", "volume_state", "volume_type"},
	)
	volumesTotalBytesGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "volume_total_bytes",
			Help:      "The total number of bytes in all volumes",
		},
	)
	scGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "storageclass_count",
			Help:      "The total number of storage classes",
		},
	)
	nodeGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "node_count",
			Help:      "The total number of nodes",
		},
	)
	snapshotGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "snapshot_count",
			Help:      "The total number of snapshots",
		},
		[]string{"backend_type", "backend_uuid"},
	)
	snapshotAllocatedBytesGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "snapshot_allocated_bytes",
			Help:      "The allocated number of snapshot bytes in all backends",
		},
		[]string{"backend_type", "backend_uuid"},
	)
	operationDurationInMsSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  config.OrchestratorName,
			Name:       "operation_duration_milliseconds",
			Help:       "The duration of operations by backend",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"operation", "success"},
	)
)
