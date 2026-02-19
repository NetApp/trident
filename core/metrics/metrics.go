package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/netapp/trident/config"
)

var (
	TridentBuildInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "build_info",
			Help:      "Trident build and release information",
		},
		[]string{"trident_revision", "trident_version", "build_type"},
	)
	TridentBackendInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "backend_info",
			Help:      "Trident backend information",
		},
		[]string{"backend_type", "backend_name", "backend_uuid"},
	)
	BackendsGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "backend_count",
			Help:      "The total number of backends",
		},
		[]string{"backend_type", "backend_state"},
	)
	VolumeAllocatedBytesGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "volume_allocated_bytes",
			Help:      "The total allocated number of bytes grouped by backends and volumes",
		},
		[]string{"backend_type", "backend_uuid", "volume_state", "volume_type"},
	)
	VolumesGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "volume_count",
			Help:      "The total number of volumes",
		},
		[]string{"backend_type", "backend_uuid", "volume_state", "volume_type"},
	)
	VolumesTotalBytesGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "volume_total_bytes",
			Help:      "The total number of bytes in all volumes",
		},
	)
	SCGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "storageclass_count",
			Help:      "The total number of storage classes",
		},
	)
	NodeGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "node_count",
			Help:      "The total number of nodes",
		},
	)
	SnapshotGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "snapshot_count",
			Help:      "The total number of snapshots",
		},
		[]string{"backend_type", "backend_uuid"},
	)
	SnapshotAllocatedBytesGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "snapshot_allocated_bytes",
			Help:      "The allocated number of snapshot bytes in all backends",
		},
		[]string{"backend_type", "backend_uuid"},
	)
	OperationDurationInMsSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  config.OrchestratorName,
			Name:       "operation_duration_milliseconds",
			Help:       "The duration of operations by backend",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"operation", "success"},
	)
	AutogrowPolicyGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "autogrow_policy_count",
			Help:      "The total number of Autogrow policies",
		},
	)
	AutogrowVolumes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: config.OrchestratorName,
			Name:      "autogrow_volume_count",
			Help:      "The total number of volumes on which Autogrow is enabled",
		},
		[]string{"autogrow_policy", "storage_class"},
	)
)
