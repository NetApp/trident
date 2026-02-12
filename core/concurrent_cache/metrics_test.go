package concurrent_cache

import (
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core/metrics"
	mockstorage "github.com/netapp/trident/mocks/mock_storage"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/models"
)

func getMockBackendWithMap(mockCtrl *gomock.Controller, attributes map[string]string) *mockstorage.MockBackend {
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	if name, ok := attributes["name"]; ok {
		mockBackend.EXPECT().Name().Return(name).AnyTimes()
	}
	if uuid, ok := attributes["uuid"]; ok {
		mockBackend.EXPECT().BackendUUID().Return(uuid).AnyTimes()
	}
	if protocol, ok := attributes["protocol"]; ok {
		mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.Protocol(protocol)).AnyTimes()
	}
	if driverName, ok := attributes["driverName"]; ok {
		mockBackend.EXPECT().GetDriverName().Return(driverName).AnyTimes()
	}
	if state, ok := attributes["state"]; ok {
		mockBackend.EXPECT().State().Return(storage.BackendState(state)).AnyTimes()
		mockBackend.EXPECT().Online().Return(true).AnyTimes()
	}
	if online, ok := attributes["online"]; ok {
		mockBackend.EXPECT().Online().Return(online == "true").AnyTimes()
	}

	if uniqueKey, ok := attributes["uniqueKey"]; ok {
		mockBackend.EXPECT().GetUniqueKey().Return(uniqueKey).AnyTimes()
	}

	mockBackend.EXPECT().SmartCopy().Return(mockBackend).AnyTimes()

	mockBackend.EXPECT().ConstructPersistent(gomock.Any()).Return(&storage.BackendPersistent{
		Name:        attributes["name"],
		BackendUUID: attributes["uuid"],
	}).AnyTimes()

	return mockBackend
}

func TestAddAndDeleteBackendMetrics(t *testing.T) {
	tests := []struct {
		name        string
		driverName  string
		backendName string
		backendUUID string
		state       storage.BackendState
	}{
		{
			name:        "add and delete backend with online state",
			driverName:  "ontap-nas",
			backendName: "backend1",
			backendUUID: "uuid-123",
			state:       storage.Online,
		},
		{
			name:        "add and delete backend with offline state",
			driverName:  "ontap-san",
			backendName: "backend2",
			backendUUID: "uuid-456",
			state:       storage.Offline,
		},
		{
			name:        "add and delete backend with failed state",
			driverName:  "solidfire-san",
			backendName: "backend3",
			backendUUID: "uuid-789",
			state:       storage.Failed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.BackendsGauge.Reset()
			metrics.TridentBackendInfo.Reset()

			mockCtrl := gomock.NewController(t)

			// Create mock backend
			mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
				"driverName": tt.driverName,
				"name":       tt.backendName,
				"uuid":       tt.backendUUID,
				"state":      string(tt.state),
			})

			// Get initial metric values
			initialGaugeValue := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues(tt.driverName, tt.state.String()))
			initialInfoValue := testutil.ToFloat64(metrics.TridentBackendInfo.WithLabelValues(tt.driverName, tt.backendName, tt.backendUUID))

			// Test adding backend to metrics
			addBackendToMetrics(mockBackend)

			// Verify metrics were updated correctly after add
			afterAddGaugeValue := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues(tt.driverName, tt.state.String()))
			afterAddInfoValue := testutil.ToFloat64(metrics.TridentBackendInfo.WithLabelValues(tt.driverName, tt.backendName, tt.backendUUID))

			assert.Equal(t, initialGaugeValue+1, afterAddGaugeValue, "BackendsGauge should be incremented by 1 after add")
			assert.Equal(t, initialInfoValue+1, afterAddInfoValue, "TridentBackendInfo should be set to 1 after add")

			// Test deleting backend from metrics
			deleteBackendFromMetrics(mockBackend)

			// Verify metrics were updated correctly after delete
			finalGaugeValue := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues(tt.driverName, tt.state.String()))
			finalInfoValue := testutil.ToFloat64(metrics.TridentBackendInfo.WithLabelValues(tt.driverName, tt.backendName, tt.backendUUID))

			assert.Equal(t, initialGaugeValue, finalGaugeValue, "BackendsGauge should return to initial value after delete")
			assert.Equal(t, initialInfoValue, finalInfoValue, "TridentBackendInfo should return to initial value after delete")
		})
	}
}

func TestAddAndDeleteBackendMetrics_MultipleBackends(t *testing.T) {
	// Reset metrics
	metrics.BackendsGauge.Reset()
	metrics.TridentBackendInfo.Reset()

	// Create multiple mock backends
	backends := []struct {
		driverName  string
		backendName string
		backendUUID string
		state       storage.BackendState
	}{
		{"ontap-nas", "backend1", "uuid-1", storage.Online},
		{"ontap-nas", "backend2", "uuid-2", storage.Online},
		{"ontap-san", "backend3", "uuid-3", storage.Offline},
	}

	var mockBackends []*mockstorage.MockBackend

	// Add all backends to metrics
	for i, b := range backends {
		mockCtrl := gomock.NewController(t)

		mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
			"driverName": b.driverName,
			"name":       b.backendName,
			"uuid":       b.backendUUID,
			"state":      string(b.state),
		})

		mockBackends = append(mockBackends, mockBackend)
		addBackendToMetrics(mockBackend)

		// Verify individual backend info metrics after add
		infoValue := testutil.ToFloat64(metrics.TridentBackendInfo.WithLabelValues(b.driverName, b.backendName, b.backendUUID))
		assert.Equal(t, float64(1), infoValue, "TridentBackendInfo should be set to 1 for backend %d", i)
	}

	// Verify aggregated gauge metrics after all adds
	onlineOntapNas := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues("ontap-nas", storage.Online.String()))
	offlineOntapSan := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues("ontap-san", storage.Offline.String()))

	assert.Equal(t, float64(2), onlineOntapNas, "Should have 2 online ontap-nas backends")
	assert.Equal(t, float64(1), offlineOntapSan, "Should have 1 offline ontap-san backend")

	// Delete all backends from metrics
	for i, mockBackend := range mockBackends {
		deleteBackendFromMetrics(mockBackend)

		// Verify individual backend info metrics are deleted
		b := backends[i]
		infoValue := testutil.ToFloat64(metrics.TridentBackendInfo.WithLabelValues(b.driverName, b.backendName, b.backendUUID))
		assert.Equal(t, float64(0), infoValue, "TridentBackendInfo should be 0 after delete for backend %d", i)
	}

	// Verify aggregated gauge metrics after all deletes
	finalOnlineOntapNas := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues("ontap-nas", storage.Online.String()))
	finalOfflineOntapSan := testutil.ToFloat64(metrics.BackendsGauge.WithLabelValues("ontap-san", storage.Offline.String()))

	assert.Equal(t, float64(0), finalOnlineOntapNas, "Should have 0 online ontap-nas backends after delete")
	assert.Equal(t, float64(0), finalOfflineOntapSan, "Should have 0 offline ontap-san backends after delete")
}

func TestAddAndDeleteVolumeMetrics(t *testing.T) {
	tests := []struct {
		name        string
		driverName  string
		backendUUID string
		volumeName  string
		volumeSize  string
		volumeState storage.VolumeState
		volumeMode  config.VolumeMode
		accessMode  config.AccessMode
	}{
		{
			name:        "add and delete volume with provisioned state",
			driverName:  "ontap-nas",
			backendUUID: "backend-uuid-1",
			volumeName:  "volume1",
			volumeSize:  "1073741824", // 1GB in bytes
			volumeState: storage.VolumeStateOnline,
			volumeMode:  config.Filesystem,
			accessMode:  config.ReadWriteOnce,
		},
		{
			name:        "add and delete volume with different state",
			driverName:  "ontap-san",
			backendUUID: "backend-uuid-2",
			volumeName:  "volume2",
			volumeSize:  "2147483648", // 2GB in bytes
			volumeState: storage.VolumeStateMissingBackend,
			volumeMode:  config.Filesystem,
			accessMode:  config.ReadWriteMany,
		},
		{
			name:        "add and delete volume with block mode",
			driverName:  "solidfire-san",
			backendUUID: "backend-uuid-3",
			volumeName:  "volume3",
			volumeSize:  "5368709120", // 5GB in bytes
			volumeState: storage.VolumeStateOnline,
			volumeMode:  config.RawBlock,
			accessMode:  config.ReadWriteOnce,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.VolumesTotalBytesGauge.Set(0)
			metrics.VolumesGauge.Reset()
			metrics.VolumeAllocatedBytesGauge.Reset()

			mockCtrl := gomock.NewController(t)

			// Create mock backend
			mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
				"driverName": tt.driverName,
				"uuid":       tt.backendUUID,
			})

			// Create volume
			volume := &storage.Volume{
				Config: &storage.VolumeConfig{
					Name:       tt.volumeName,
					Size:       tt.volumeSize,
					VolumeMode: tt.volumeMode,
					AccessMode: tt.accessMode,
				},
				BackendUUID: tt.backendUUID,
				State:       tt.volumeState,
			}

			expectedBytes, _ := strconv.ParseFloat(tt.volumeSize, 64)

			// Get initial metric values
			initialTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			initialVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))
			initialAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))

			// Test adding volume to metrics
			addVolumeToMetrics(volume, mockBackend)

			// Verify metrics were updated correctly after add
			afterAddTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			afterAddVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))
			afterAddAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))

			assert.Equal(t, initialTotalBytes+expectedBytes, afterAddTotalBytes, "VolumesTotalBytesGauge should be increased by volume size")
			assert.Equal(t, initialVolumeGauge+1, afterAddVolumeGauge, "VolumesGauge should be incremented by 1")
			assert.Equal(t, initialAllocatedBytes+expectedBytes, afterAddAllocatedBytes, "VolumeAllocatedBytesGauge should be increased by volume size")

			// Test deleting volume from metrics
			deleteVolumeFromMetrics(volume, mockBackend)

			// Verify metrics were updated correctly after delete
			finalTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			finalVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))
			finalAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))

			assert.Equal(t, initialTotalBytes, finalTotalBytes, "VolumesTotalBytesGauge should return to initial value after delete")
			assert.Equal(t, initialVolumeGauge, finalVolumeGauge, "VolumesGauge should return to initial value after delete")
			assert.Equal(t, initialAllocatedBytes, finalAllocatedBytes, "VolumeAllocatedBytesGauge should return to initial value after delete")
		})
	}
}

func TestAddAndDeleteVolumeMetrics_InvalidSize(t *testing.T) {
	tests := []struct {
		name        string
		driverName  string
		backendUUID string
		volumeName  string
		volumeSize  string // Invalid size that will cause strconv.ParseFloat to fail
		volumeState storage.VolumeState
		volumeMode  config.VolumeMode
		accessMode  config.AccessMode
	}{
		{
			name:        "invalid volume size - non-numeric string",
			driverName:  "ontap-nas",
			backendUUID: "backend-uuid-1",
			volumeName:  "volume1",
			volumeSize:  "not-a-number",
			volumeState: storage.VolumeStateOnline,
			volumeMode:  config.Filesystem,
			accessMode:  config.ReadWriteOnce,
		},
		{
			name:        "invalid volume size - empty string",
			driverName:  "ontap-san",
			backendUUID: "backend-uuid-2",
			volumeName:  "volume2",
			volumeSize:  "",
			volumeState: storage.VolumeStateOnline,
			volumeMode:  config.RawBlock,
			accessMode:  config.ReadWriteMany,
		},
		{
			name:        "invalid volume size - special characters",
			driverName:  "solidfire-san",
			backendUUID: "backend-uuid-3",
			volumeName:  "volume3",
			volumeSize:  "1GB",
			volumeState: storage.VolumeStateMissingBackend,
			volumeMode:  config.Filesystem,
			accessMode:  config.ReadWriteOnce,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.VolumesTotalBytesGauge.Set(0)
			metrics.VolumesGauge.Reset()
			metrics.VolumeAllocatedBytesGauge.Reset()

			mockCtrl := gomock.NewController(t)

			// Create mock backend
			mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
				"driverName": tt.driverName,
				"uuid":       tt.backendUUID,
			})

			// Add a good volume first to establish non-zero baseline metrics
			goodVolume := &storage.Volume{
				Config: &storage.VolumeConfig{
					Name:       "good-volume",
					Size:       "1073741824", // 1GB
					VolumeMode: tt.volumeMode,
					AccessMode: tt.accessMode,
				},
				BackendUUID: tt.backendUUID,
				State:       tt.volumeState,
			}
			addVolumeToMetrics(goodVolume, mockBackend)

			// Create volume with invalid size
			volume := &storage.Volume{
				Config: &storage.VolumeConfig{
					Name:       tt.volumeName,
					Size:       tt.volumeSize,
					VolumeMode: tt.volumeMode,
					AccessMode: tt.accessMode,
				},
				BackendUUID: tt.backendUUID,
				State:       tt.volumeState,
			}

			// Get initial metric values (after adding good volume)
			initialTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			initialVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))
			initialAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))

			// Test adding volume to metrics with invalid size - should fail gracefully
			addVolumeToMetrics(volume, mockBackend)

			// Verify that metrics were NOT updated due to parse error
			afterAddTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			afterAddVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))
			afterAddAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))

			// Total bytes and allocated bytes should remain unchanged due to parse error
			assert.Equal(t, initialTotalBytes, afterAddTotalBytes, "VolumesTotalBytesGauge should remain unchanged due to parse error")
			assert.Equal(t, initialAllocatedBytes, afterAddAllocatedBytes, "VolumeAllocatedBytesGauge should remain unchanged due to parse error")
			// Volume gauge should also remain unchanged since the function returns early on parse error
			assert.Equal(t, initialVolumeGauge, afterAddVolumeGauge, "VolumesGauge should remain unchanged due to parse error")

			// Test deleting volume from metrics with invalid size - should also fail gracefully
			deleteVolumeFromMetrics(volume, mockBackend)

			// Verify that metrics remain unchanged after attempted delete
			finalTotalBytes := testutil.ToFloat64(metrics.VolumesTotalBytesGauge)
			finalVolumeGauge := testutil.ToFloat64(metrics.VolumesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))
			finalAllocatedBytes := testutil.ToFloat64(metrics.VolumeAllocatedBytesGauge.WithLabelValues(tt.driverName, tt.backendUUID, string(tt.volumeState), string(tt.volumeMode)))

			assert.Equal(t, initialTotalBytes, finalTotalBytes, "VolumesTotalBytesGauge should remain at initial value after failed delete")
			assert.Equal(t, initialVolumeGauge, finalVolumeGauge, "VolumesGauge should remain at initial value after failed delete")
			assert.Equal(t, initialAllocatedBytes, finalAllocatedBytes, "VolumeAllocatedBytesGauge should remain at initial value after failed delete")
		})
	}
}

func TestAddAndDeleteSnapshotMetrics(t *testing.T) {
	tests := []struct {
		name         string
		driverName   string
		backendUUID  string
		volumeName   string
		snapshotName string
		sizeBytes    int64
	}{
		{
			name:         "add and delete snapshot with small size",
			driverName:   "ontap-nas",
			backendUUID:  "backend-uuid-1",
			volumeName:   "volume1",
			snapshotName: "snapshot1",
			sizeBytes:    1073741824, // 1GB
		},
		{
			name:         "add and delete snapshot with large size",
			driverName:   "ontap-san",
			backendUUID:  "backend-uuid-2",
			volumeName:   "volume2",
			snapshotName: "snapshot2",
			sizeBytes:    5368709120, // 5GB
		},
		{
			name:         "add and delete snapshot with zero size",
			driverName:   "solidfire-san",
			backendUUID:  "backend-uuid-3",
			volumeName:   "volume3",
			snapshotName: "snapshot3",
			sizeBytes:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.SnapshotGauge.Reset()
			metrics.SnapshotAllocatedBytesGauge.Reset()

			mockCtrl := gomock.NewController(t)

			// Create mock backend
			mockBackend := getMockBackendWithMap(mockCtrl, map[string]string{
				"driverName": tt.driverName,
				"uuid":       tt.backendUUID,
			})

			// Create volume
			volume := &storage.Volume{
				Config: &storage.VolumeConfig{
					Name: tt.volumeName,
				},
				BackendUUID: tt.backendUUID,
			}

			// Create snapshot
			snapshot := &storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name: tt.snapshotName,
				},
				SizeBytes: tt.sizeBytes,
			}

			// Get initial metric values
			initialSnapshotGauge := testutil.ToFloat64(metrics.SnapshotGauge.WithLabelValues(tt.driverName, tt.backendUUID))
			initialAllocatedBytes := testutil.ToFloat64(metrics.SnapshotAllocatedBytesGauge.WithLabelValues(tt.driverName, tt.backendUUID))

			// Test adding snapshot to metrics
			addSnapshotToMetrics(snapshot, volume, mockBackend)

			// Verify metrics were updated correctly after add
			afterAddSnapshotGauge := testutil.ToFloat64(metrics.SnapshotGauge.WithLabelValues(tt.driverName, tt.backendUUID))
			afterAddAllocatedBytes := testutil.ToFloat64(metrics.SnapshotAllocatedBytesGauge.WithLabelValues(tt.driverName, tt.backendUUID))

			assert.Equal(t, initialSnapshotGauge+1, afterAddSnapshotGauge, "SnapshotGauge should be incremented by 1")
			assert.Equal(t, initialAllocatedBytes+float64(tt.sizeBytes), afterAddAllocatedBytes, "SnapshotAllocatedBytesGauge should be increased by snapshot size")

			// Test deleting snapshot from metrics
			deleteSnapshotFromMetrics(snapshot, volume, mockBackend)

			// Verify metrics were updated correctly after delete
			finalSnapshotGauge := testutil.ToFloat64(metrics.SnapshotGauge.WithLabelValues(tt.driverName, tt.backendUUID))
			finalAllocatedBytes := testutil.ToFloat64(metrics.SnapshotAllocatedBytesGauge.WithLabelValues(tt.driverName, tt.backendUUID))

			assert.Equal(t, initialSnapshotGauge, finalSnapshotGauge, "SnapshotGauge should return to initial value after delete")
			assert.Equal(t, initialAllocatedBytes, finalAllocatedBytes, "SnapshotAllocatedBytesGauge should return to initial value after delete")
		})
	}
}

func TestAddAndDeleteNodeMetrics(t *testing.T) {
	tests := []struct {
		name     string
		nodeName string
		nodeID   string
	}{
		{
			name:     "add and delete node with standard name",
			nodeName: "worker-node-1",
			nodeID:   "node-uuid-1",
		},
		{
			name:     "add and delete node with different name",
			nodeName: "master-node-1",
			nodeID:   "node-uuid-2",
		},
		{
			name:     "add and delete node with special characters",
			nodeName: "node-with-dashes_and_underscores",
			nodeID:   "node-uuid-3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.NodeGauge.Set(0)

			// Create node
			node := &models.Node{
				Name: tt.nodeName,
				IQN:  tt.nodeID,
			}

			// Get initial metric value
			initialNodeGauge := testutil.ToFloat64(metrics.NodeGauge)

			// Test adding node to metrics
			addNodeToMetrics(node)

			// Verify metrics were updated correctly after add
			afterAddNodeGauge := testutil.ToFloat64(metrics.NodeGauge)

			assert.Equal(t, initialNodeGauge+1, afterAddNodeGauge, "NodeGauge should be incremented by 1")

			// Test deleting node from metrics
			deleteNodeFromMetrics(node)

			// Verify metrics were updated correctly after delete
			finalNodeGauge := testutil.ToFloat64(metrics.NodeGauge)

			assert.Equal(t, initialNodeGauge, finalNodeGauge, "NodeGauge should return to initial value after delete")
		})
	}
}

func TestAddAndDeleteStorageClassMetrics(t *testing.T) {
	tests := []struct {
		name             string
		storageClassName string
	}{
		{
			name:             "add and delete storage class with standard name",
			storageClassName: "fast-ssd",
		},
		{
			name:             "add and delete storage class with different name",
			storageClassName: "slow-hdd",
		},
		{
			name:             "add and delete storage class with special characters",
			storageClassName: "premium-storage_class-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.SCGauge.Set(0)

			// Create storage class
			storageClass := &storageclass.StorageClass{
				// Note: The actual StorageClass struct would have more fields,
				// but since the functions don't use the parameter, we only need a minimal instance
			}

			// Get initial metric value
			initialSCGauge := testutil.ToFloat64(metrics.SCGauge)

			// Test adding storage class to metrics
			addStorageClassToMetrics(storageClass)

			// Verify metrics were updated correctly after add
			afterAddSCGauge := testutil.ToFloat64(metrics.SCGauge)

			assert.Equal(t, initialSCGauge+1, afterAddSCGauge, "SCGauge should be incremented by 1")

			// Test deleting storage class from metrics
			deleteStorageClassFromMetrics(storageClass)

			// Verify metrics were updated correctly after delete
			finalSCGauge := testutil.ToFloat64(metrics.SCGauge)

			assert.Equal(t, initialSCGauge, finalSCGauge, "SCGauge should return to initial value after delete")
		})
	}
}
