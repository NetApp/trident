package concurrent_cache

import (
	"strconv"

	"github.com/netapp/trident/core/metrics"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/models"
)

// addBackendToMetrics adds a backend to the Prometheus metrics.
func addBackendToMetrics(backend storage.Backend) {
	metrics.BackendsGauge.WithLabelValues(
		backend.GetDriverName(),
		backend.State().String(),
	).Inc()

	metrics.TridentBackendInfo.WithLabelValues(
		backend.GetDriverName(),
		backend.Name(),
		backend.BackendUUID(),
	).Set(float64(1))
}

// deleteBackendFromMetrics deletes a backend from the Prometheus metrics.
func deleteBackendFromMetrics(backend storage.Backend) {
	metrics.BackendsGauge.WithLabelValues(
		backend.GetDriverName(),
		backend.State().String(),
	).Dec()

	metrics.TridentBackendInfo.DeleteLabelValues(
		backend.GetDriverName(),
		backend.Name(),
		backend.BackendUUID(),
	)
}

// addVolumeToMetrics adds a volume to the Prometheus metrics.
func addVolumeToMetrics(volume *storage.Volume, backend storage.Backend) {
	bytes, err := strconv.ParseFloat(volume.Config.Size, 64)
	if err != nil {
		return
	}

	metrics.VolumesTotalBytesGauge.Add(bytes)

	metrics.VolumesGauge.WithLabelValues(
		backend.GetDriverName(),
		volume.BackendUUID,
		string(volume.State),
		string(volume.Config.VolumeMode),
	).Inc()

	metrics.VolumeAllocatedBytesGauge.WithLabelValues(
		backend.GetDriverName(),
		volume.BackendUUID,
		string(volume.State),
		string(volume.Config.VolumeMode),
	).Add(bytes)
}

// deleteVolumeFromMetrics deletes a volume from the Prometheus metrics.
func deleteVolumeFromMetrics(volume *storage.Volume, backend storage.Backend) {
	bytes, err := strconv.ParseFloat(volume.Config.Size, 64)
	if err != nil {
		return
	}

	metrics.VolumesTotalBytesGauge.Sub(bytes)

	metrics.VolumesGauge.WithLabelValues(
		backend.GetDriverName(),
		volume.BackendUUID,
		string(volume.State),
		string(volume.Config.VolumeMode),
	).Dec()

	metrics.VolumeAllocatedBytesGauge.WithLabelValues(
		backend.GetDriverName(),
		volume.BackendUUID,
		string(volume.State),
		string(volume.Config.VolumeMode),
	).Sub(bytes)
}

// addSnapshotToMetrics adds a snapshot to the Prometheus metrics.
func addSnapshotToMetrics(snapshot *storage.Snapshot, volume *storage.Volume, backend storage.Backend) {
	metrics.SnapshotGauge.WithLabelValues(
		backend.GetDriverName(),
		volume.BackendUUID,
	).Inc()

	metrics.SnapshotAllocatedBytesGauge.WithLabelValues(
		backend.GetDriverName(),
		volume.BackendUUID,
	).Add(float64(snapshot.SizeBytes))
}

// deleteSnapshotFromMetrics deletes a snapshot from the Prometheus metrics.
func deleteSnapshotFromMetrics(snapshot *storage.Snapshot, volume *storage.Volume, backend storage.Backend) {
	metrics.SnapshotGauge.WithLabelValues(
		backend.GetDriverName(),
		volume.BackendUUID,
	).Dec()

	metrics.SnapshotAllocatedBytesGauge.WithLabelValues(
		backend.GetDriverName(),
		volume.BackendUUID,
	).Sub(float64(snapshot.SizeBytes))
}

// addNodeToMetrics adds a node to the Prometheus metrics.
func addNodeToMetrics(_ *models.Node) {
	metrics.NodeGauge.Inc()
}

// deleteNodeFromMetrics deletes a node from the Prometheus metrics.
func deleteNodeFromMetrics(_ *models.Node) {
	metrics.NodeGauge.Dec()
}

// addStorageClassToMetrics adds a storage class to the Prometheus metrics.
func addStorageClassToMetrics(_ *storageclass.StorageClass) {
	metrics.SCGauge.Inc()
}

// deleteStorageClassFromMetrics deletes a storage class from the Prometheus metrics.
func deleteStorageClassFromMetrics(_ *storageclass.StorageClass) {
	metrics.SCGauge.Dec()
}
