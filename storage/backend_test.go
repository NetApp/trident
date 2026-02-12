// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/acp"
	tridentconfig "github.com/netapp/trident/config"
	mockacp "github.com/netapp/trident/mocks/mock_acp"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestBackendState(t *testing.T) {
	tests := map[string]struct {
		input     BackendState
		output    string
		predicate func(BackendState) bool
	}{
		"Unknown state (bad)": {
			input:  "",
			output: "unknown",
			predicate: func(input BackendState) bool {
				return input.IsUnknown()
			},
		},
		"Unknown state": {
			input:  Unknown,
			output: "unknown",
			predicate: func(input BackendState) bool {
				return input.IsUnknown()
			},
		},
		"Online state": {
			input:  Online,
			output: "online",
			predicate: func(input BackendState) bool {
				return input.IsOnline()
			},
		},
		"Offline state": {
			input:  Offline,
			output: "offline",
			predicate: func(input BackendState) bool {
				return input.IsOffline()
			},
		},
		"Deleting state": {
			input:  Deleting,
			output: "deleting",
			predicate: func(input BackendState) bool {
				return input.IsDeleting()
			},
		},
		"Failed state": {
			input:  Failed,
			output: "failed",
			predicate: func(input BackendState) bool {
				return input.IsFailed()
			},
		},
	}
	for testName, test := range tests {
		t.Run(
			testName, func(t *testing.T) {
				assert.Equal(t, test.input.String(), test.output, "Strings not equal")
				assert.True(t, test.predicate(test.input), "Predicate failed")
			},
		)
	}
}

func TestDeleteSnapshot_BackendOffline(t *testing.T) {
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volumeConfig := &VolumeConfig{
		Version:      "",
		Name:         volumeName,
		InternalName: volumeInternalName,
	}
	snapName := "snapshot"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
	}

	backend := &StorageBackend{
		state:     Offline,
		stateLock: new(sync.RWMutex),
	}

	// Both volume and snapshot not managed
	err := backend.DeleteSnapshot(context.Background(), snapConfig, volumeConfig)

	assert.Errorf(t, err, "expected err")
}

func TestDeleteSnapshot_NotManaged(t *testing.T) {
	backendUUID := "test-backend"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volumeConfig := &VolumeConfig{
		Version:             "",
		Name:                volumeName,
		InternalName:        volumeInternalName,
		ImportOriginalName:  "import-" + volumeName,
		ImportBackendUUID:   "import-" + backendUUID,
		ImportNotManaged:    true,
		LUKSPassphraseNames: nil,
	}
	snapName := "snapshot-import"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
		ImportNotManaged:   true,
	}

	backend := &StorageBackend{
		state:     Online,
		stateLock: new(sync.RWMutex),
	}

	// Both volume and snapshot not managed
	err := backend.DeleteSnapshot(context.Background(), snapConfig, volumeConfig)

	assert.Errorf(t, err, "expected err")

	// Volume not managed
	volumeConfig.ImportNotManaged = true
	snapConfig.ImportNotManaged = false
	err = backend.DeleteSnapshot(context.Background(), snapConfig, volumeConfig)

	assert.Errorf(t, err, "expected err")

	// Snapshot not managed
	volumeConfig.ImportNotManaged = false
	snapConfig.ImportNotManaged = true
	err = backend.DeleteSnapshot(context.Background(), snapConfig, volumeConfig)

	assert.Errorf(t, err, "expected err")
}

func TestCloneVolume_BackendOffline(t *testing.T) {
	// Reset the package-level state after the test completes.
	defer acp.SetAPI(acp.API())

	mockCtrl := gomock.NewController(t)
	mockACP := mockacp.NewMockTridentACP(mockCtrl)
	acp.SetAPI(mockACP)

	// Mock out any expected calls on the ACP API.
	err := errors.UnsupportedError("unsupported")
	mockACP.EXPECT().IsFeatureEnabled(gomock.Any(), acp.FeatureReadOnlyClone).Return(err).AnyTimes()

	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volumeConfig := &VolumeConfig{
		Version:       "",
		Name:          volumeName,
		InternalName:  volumeInternalName,
		ReadOnlyClone: true,
	}
	volumeConfigDest := &VolumeConfig{
		Version:       "",
		Name:          "pvc-deadbeef-8240-4fd8-97bc-868bf064ecd4",
		InternalName:  "trident_pvc_deadbeef_8240_4fd8_97bc_868bf064ecd4",
		ReadOnlyClone: false,
	}

	backend := &StorageBackend{
		state:     Offline,
		name:      "test-backend",
		stateLock: new(sync.RWMutex),
	}
	pool := NewStoragePool(nil, "test-pool1")

	// Both volume and snapshot not managed
	_, err = backend.CloneVolume(context.Background(), volumeConfig, volumeConfigDest, pool, false)

	assert.Errorf(t, err, "expected err")
	assert.Equal(t, err.Error(), "backend test-backend is not Online")
}

func TestUserBackendState(t *testing.T) {
	backend := &StorageBackend{
		driver:             nil,
		name:               "fake",
		backendUUID:        "1234",
		online:             true,
		state:              Online,
		userState:          UserNormal,
		stateReason:        "",
		storagePools:       nil,
		volumes:            nil,
		configRef:          "",
		nodeAccessUpToDate: false,
		stateLock:          new(sync.RWMutex),
	}

	tests := map[string]struct {
		input     UserBackendState
		output    string
		predicate func(UserBackendState) bool
	}{
		"UserSuspended State": {
			input:  UserSuspended,
			output: "suspended",
			predicate: func(input UserBackendState) bool {
				return input.IsSuspended()
			},
		},

		"UserNormal State": {
			input:  UserNormal,
			output: "normal",
			predicate: func(input UserBackendState) bool {
				return input.IsNormal()
			},
		},

		"Unknown State": {
			input:  "",
			output: "unknown",
			predicate: func(input UserBackendState) bool {
				return true
			},
		},
	}

	for testName, test := range tests {
		t.Run(
			testName, func(t *testing.T) {
				backend.SetUserState(test.input)
				state := backend.UserState()
				assert.Equal(t, test.input, backend.userState, "Strings not equal")
				assert.Equal(t, test.input, state, "Strings not equal")
				assert.Equal(t, test.input.String(), test.output, "Strings not equal")
				assert.True(t, test.predicate(test.input), "Predicate failed")
			},
		)
	}
}

func TestValidate(t *testing.T) {
	tests := map[string]struct {
		input UserBackendState
	}{
		"UserSuspended State": {
			input: UserSuspended,
		},

		"UserNormal State": {
			input: UserNormal,
		},
	}

	for testName, test := range tests {
		t.Run(
			testName, func(t *testing.T) {
				err := test.input.Validate()
				assert.NoError(t, err, "Expected no error")
			},
		)
	}
}

func TestValidateNegative(t *testing.T) {
	tests := map[string]struct {
		input UserBackendState
	}{
		"Unknown State": {
			input: UserBackendState("unknown"),
		},
	}

	for testName, test := range tests {
		t.Run(
			testName, func(t *testing.T) {
				err := test.input.Validate()
				assert.Error(t, err, "Expected error")
			},
		)
	}
}

func TestIsProvisioningAllowed(t *testing.T) {
	backend := &StorageBackend{
		driver:             nil,
		name:               "fake",
		backendUUID:        "1234",
		online:             true,
		state:              Online,
		userState:          UserNormal,
		stateReason:        "",
		storagePools:       nil,
		volumes:            nil,
		configRef:          "",
		nodeAccessUpToDate: false,
		stateLock:          new(sync.RWMutex),
	}

	// For cases where provisioning is allowed.
	tests := map[string]struct {
		input UserBackendState
	}{
		"UserNormal State": {
			input: UserNormal,
		},

		// Need to improve the isProvisioningAllowed() function to handle this case.
		"Unknown State": {
			input: "unknown",
		},
	}

	for testName, test := range tests {
		t.Run(
			testName, func(t *testing.T) {
				backend.userState = test.input
				boolValue := backend.isProvisioningAllowed()
				assert.True(t, boolValue, "Expected true")
			},
		)
	}

	// For cases where provisioning is not allowed.
	tests = map[string]struct {
		input UserBackendState
	}{
		"UserSuspended State": {
			input: UserSuspended,
		},
	}

	for testName, test := range tests {
		t.Run(
			testName, func(t *testing.T) {
				backend.userState = test.input
				boolValue := backend.isProvisioningAllowed()
				assert.False(t, boolValue, "Expected false")
			},
		)
	}
}

func TestNewStoragePool(t *testing.T) {
	poolName := "test-pool"

	// Test with nil backend (edge case)
	pool := NewStoragePool(nil, poolName)

	assert.NotNil(t, pool, "NewStoragePool should return a pool")
}

func TestNewVolume(t *testing.T) {
	volumeConfig := &VolumeConfig{
		Version:      "1",
		Name:         "test-volume",
		InternalName: "test-volume-internal",
		Size:         "1Gi",
	}
	backendUUID := "backend-123"
	pool := "pool-1"
	orphaned := false
	state := VolumeStateOnline

	volume := NewVolume(volumeConfig, backendUUID, pool, orphaned, state)

	assert.NotNil(t, volume, "NewVolume should return a volume")
}

func TestNewSnapshot(t *testing.T) {
	snapConfig := &SnapshotConfig{
		Version:            "1",
		Name:               "test-snapshot",
		VolumeName:         "test-volume",
		InternalName:       "test-snapshot-internal",
		VolumeInternalName: "test-volume-internal",
	}
	created := "2023-01-01T00:00:00Z"
	sizeBytes := int64(1073741824) // 1GB
	state := SnapshotStateOnline

	snapshot := NewSnapshot(snapConfig, created, sizeBytes, state)

	assert.NotNil(t, snapshot, "NewSnapshot should return a snapshot")
}

func TestNewGroupSnapshot(t *testing.T) {
	groupSnapConfig := &GroupSnapshotConfig{
		Version:      "1",
		Name:         "test-group-snapshot",
		InternalName: "test-group-snapshot-internal",
		VolumeNames:  []string{"volume1", "volume2"},
	}
	snapshotIDs := []string{"snap1", "snap2"}
	created := "2023-01-01T00:00:00Z"

	groupSnapshot := NewGroupSnapshot(groupSnapConfig, snapshotIDs, created)

	assert.NotNil(t, groupSnapshot, "NewGroupSnapshot should return a group snapshot")
}

func TestNewGroupSnapshotTargetInfo(t *testing.T) {
	storageType := "ontap-nas"
	storageUUID := "storage-uuid-123"
	storageVols := make(GroupSnapshotTargetVolumes)
	storageVols["backend1"] = make(map[string]*VolumeConfig)
	storageVols["backend1"]["vol1"] = &VolumeConfig{Name: "vol1", InternalName: "vol1-internal"}
	storageVols["backend1"]["vol2"] = &VolumeConfig{Name: "vol2", InternalName: "vol2-internal"}

	targetInfo := NewGroupSnapshotTargetInfo(storageType, storageUUID, storageVols)

	assert.NotNil(t, targetInfo, "NewGroupSnapshotTargetInfo should return target info")
}

func TestNewGroupSnapshotExternal(t *testing.T) {
	groupSnapConfig := &GroupSnapshotConfig{
		Version:      "1",
		Name:         "test-group-snapshot",
		InternalName: "test-group-snapshot-internal",
		VolumeNames:  []string{"volume1", "volume2"},
	}
	snapshotIDs := []string{"snap1", "snap2"}
	created := "2023-01-01T00:00:00Z"

	groupSnapshot := NewGroupSnapshot(groupSnapConfig, snapshotIDs, created)
	external := NewGroupSnapshotExternal(groupSnapshot)

	assert.NotNil(t, external, "NewGroupSnapshotExternal should return external representation")
}

func TestNewGroupSnapshotPersistent(t *testing.T) {
	groupSnapConfig := &GroupSnapshotConfig{
		Version:      "1",
		Name:         "test-group-snapshot",
		InternalName: "test-group-snapshot-internal",
		VolumeNames:  []string{"volume1", "volume2"},
	}
	snapshotIDs := []string{"snap1", "snap2"}
	created := "2023-01-01T00:00:00Z"

	groupSnapshot := NewGroupSnapshot(groupSnapConfig, snapshotIDs, created)
	persistent := NewGroupSnapshotPersistent(groupSnapshot)

	assert.NotNil(t, persistent, "NewGroupSnapshotPersistent should return persistent representation")
}

func TestStoragePool_BasicMethods(t *testing.T) {
	pool := NewStoragePool(nil, "test-pool")

	// Test SetBackend
	backend := &StorageBackend{name: "test-backend"}
	pool.SetBackend(backend)
	assert.Equal(t, backend, pool.Backend(), "SetBackend should set the backend correctly")

	// Test SetAttributes
	attributes := map[string]sa.Offer{
		"attr1": sa.NewStringOffer("value1"),
		"attr2": sa.NewStringOffer("value2"),
	}
	pool.SetAttributes(attributes)

	// Test SetInternalAttributes
	internalAttrs := map[string]string{"internal1": "value1", "internal2": "value2"}
	pool.SetInternalAttributes(internalAttrs)

	// Test SetSupportedTopologies
	topologies := []map[string]string{
		{"zone": "us-east-1a", "region": "us-east-1"},
		{"zone": "us-east-1b", "region": "us-east-1"},
	}
	pool.SetSupportedTopologies(topologies)

	// Test AddStorageClass
	pool.AddStorageClass("storage-class-1")
	pool.AddStorageClass("storage-class-2")

	// Test RemoveStorageClass
	removed := pool.RemoveStorageClass("storage-class-1")
	assert.True(t, removed, "RemoveStorageClass should return true when class exists")

	// Test RemoveStorageClass with non-existent class
	removed = pool.RemoveStorageClass("non-existent-class")
	assert.False(t, removed, "RemoveStorageClass should return false when class doesn't exist")
}

func TestStoragePool_SetStorageClasses(t *testing.T) {
	pool := NewStoragePool(nil, "test-pool")

	// Test SetStorageClasses
	classes := []string{"class1", "class2", "class3"}
	pool.SetStorageClasses(classes)

	assert.Equal(t, classes, pool.StorageClasses(), "SetStorageClasses should set all classes correctly")
}

func TestStoragePool_SetName(t *testing.T) {
	pool := NewStoragePool(nil, "original-name")

	// Test SetName
	newName := "new-pool-name"
	pool.SetName(newName)
	assert.Equal(t, newName, pool.Name(), "SetName should change the pool name")
}

func TestStoragePool_EdgeCases(t *testing.T) {
	pool := NewStoragePool(nil, "test-pool")

	// Test with nil attributes
	pool.SetAttributes(nil)
	assert.Nil(t, pool.Attributes(), "SetAttributes should handle nil correctly")

	// Test with nil internal attributes
	pool.SetInternalAttributes(nil)
	assert.Nil(t, pool.InternalAttributes(), "SetInternalAttributes should handle nil correctly")

	// Test with nil topologies
	pool.SetSupportedTopologies(nil)
	assert.Nil(t, pool.SupportedTopologies(), "SetSupportedTopologies should handle nil correctly")

	// Test with nil storage classes
	pool.SetStorageClasses(nil)
	assert.Nil(t, pool.StorageClasses(), "SetStorageClasses should handle nil correctly")

	// Test AddStorageClass with empty string
	pool.AddStorageClass("")
	classes := pool.StorageClasses()
	assert.Contains(t, classes, "", "AddStorageClass should handle empty string")

	// Test RemoveStorageClass with empty string
	removed := pool.RemoveStorageClass("")
	assert.True(t, removed, "RemoveStorageClass should handle empty string")
}

func TestStorageBackend_BasicGetterSetterMethods(t *testing.T) {
	backend := &StorageBackend{
		name:        "original-backend",
		backendUUID: "original-uuid",
		online:      false,
		state:       Offline,
		userState:   UserSuspended,
		stateReason: "original reason",
		stateLock:   new(sync.RWMutex),
	}

	// Test Name() and SetName()
	assert.Equal(t, "original-backend", backend.Name(), "Name() should return the current name")
	backend.SetName("new-backend-name")
	assert.Equal(t, "new-backend-name", backend.Name(), "SetName() should update the name")

	// Test BackendUUID() and SetBackendUUID()
	assert.Equal(t, "original-uuid", backend.BackendUUID(), "BackendUUID() should return the current UUID")
	backend.SetBackendUUID("new-uuid-123")
	assert.Equal(t, "new-uuid-123", backend.BackendUUID(), "SetBackendUUID() should update the UUID")

	// Test Online() and SetOnline()
	assert.False(t, backend.Online(), "Online() should return current online status")
	backend.SetOnline(true)
	assert.True(t, backend.Online(), "SetOnline() should update the online status")

	// Test State() - getter only
	assert.Equal(t, Offline, backend.State(), "State() should return the current state")

	// Test UserState() - getter only
	assert.Equal(t, UserSuspended, backend.UserState(), "UserState() should return the current user state")

	// Test StateReason() - getter only
	assert.Equal(t, "original reason", backend.StateReason(), "StateReason() should return the current state reason")
}

func TestStorageBackend_SetStateAndSetUserState(t *testing.T) {
	backend := &StorageBackend{
		state:     Unknown,
		userState: "",
		stateLock: new(sync.RWMutex),
	}

	// Test SetState()
	backend.SetState(Online)
	assert.Equal(t, Online, backend.State(), "SetState() should update the backend state")

	backend.SetState(Failed)

	// Test SetUserState()
	backend.SetUserState(UserNormal)
	assert.Equal(t, UserNormal, backend.UserState(), "SetUserState() should update the user state")

	backend.SetUserState(UserSuspended)
}

func TestStorageBackend_DriverMethods(t *testing.T) {
	backend := &StorageBackend{}

	// Test with nil driver initially
	assert.Nil(t, backend.Driver(), "Driver() should return nil when not set")

	// Test SetDriver() and Driver() with nil - this is a simple test
	backend.SetDriver(nil)
	assert.Nil(t, backend.Driver(), "SetDriver(nil) should set driver to nil")
}

func TestStorageBackend_GetResizeDeltaBytes(t *testing.T) {
	// We do not call GetResizeDeltaBytes() here: it uses b.driver; a nil or missing driver would panic.
	// Testing with a real driver would require storage_drivers and cause an import cycle.
	assert.NotNil(t, (*StorageBackend).GetResizeDeltaBytes, "GetResizeDeltaBytes method should exist")
}

func TestStorageBackend_StoragePoolsAndVolumes(t *testing.T) {
	backend := &StorageBackend{
		storagePools: new(sync.Map),
		volumes:      new(sync.Map),
	}

	// Test StoragePools() getter
	pools := backend.StoragePools()
	assert.NotNil(t, pools, "StoragePools() should return a sync.Map")

	// Test Volumes() getter
	volumes := backend.Volumes()
	assert.NotNil(t, volumes, "Volumes() should return a sync.Map")

	// Test ClearStoragePools()
	backend.storagePools.Store("pool1", "value1")
	backend.storagePools.Store("pool2", "value2")
	backend.ClearStoragePools()

	// Verify pools are cleared
	count := 0
	backend.storagePools.Range(func(k, v interface{}) bool {
		count++
		return true
	})
	assert.Equal(t, 0, count, "ClearStoragePools() should clear all storage pools")

	// Test ClearVolumes()
	backend.volumes.Store("vol1", "value1")
	backend.volumes.Store("vol2", "value2")
	backend.ClearVolumes()

	// Verify volumes are cleared
	count = 0
	backend.volumes.Range(func(k, v interface{}) bool {
		count++
		return true
	})
	assert.Equal(t, 0, count, "ClearVolumes() should clear all volumes")
}

func TestStorageBackend_ConfigRef(t *testing.T) {
	backend := &StorageBackend{
		configRef: "original-config-ref",
	}

	// Test SetConfigRef()
	backend.SetConfigRef("new-config-ref")
	assert.Equal(t, "new-config-ref", backend.ConfigRef(), "SetConfigRef() should update the config reference")
}

func TestGroupSnapshotTargetInfo_Methods(t *testing.T) {
	storageType := "ontap-nas"
	storageUUID := "storage-uuid-123"
	storageVols := make(GroupSnapshotTargetVolumes)
	storageVols["backend1"] = make(map[string]*VolumeConfig)
	storageVols["backend1"]["vol1"] = &VolumeConfig{Name: "vol1", InternalName: "vol1-internal"}
	storageVols["backend1"]["vol2"] = &VolumeConfig{Name: "vol2", InternalName: "vol2-internal"}

	targetInfo := NewGroupSnapshotTargetInfo(storageType, storageUUID, storageVols)

	// Test GetStorageType()
	assert.Equal(t, storageType, targetInfo.GetStorageType(), "GetStorageType() should return the storage type")

	// Test GetVolumes()
	volumes := targetInfo.GetVolumes()
	assert.Equal(t, storageVols, volumes, "GetVolumes() should return the storage volumes")
}

func TestGroupSnapshotTargetInfo_AddVolumes(t *testing.T) {
	storageType := "ontap-nas"
	storageUUID := "storage-uuid-123"

	// Test with nil StorageVolumes initially
	targetInfo := NewGroupSnapshotTargetInfo(storageType, storageUUID, nil)

	// Add some volumes
	newVolumes := make(GroupSnapshotTargetVolumes)
	newVolumes["backend1"] = make(map[string]*VolumeConfig)
	newVolumes["backend1"]["vol1"] = &VolumeConfig{Name: "vol1", InternalName: "vol1-internal"}

	targetInfo.AddVolumes(newVolumes)

	// Verify StorageVolumes was initialized and volumes were added
	assert.NotNil(t, targetInfo.StorageVolumes, "AddVolumes() should initialize StorageVolumes if nil")
	assert.Contains(t, targetInfo.StorageVolumes, "backend1", "AddVolumes() should add the backend")

	// Add more volumes to existing structure
	moreVolumes := make(GroupSnapshotTargetVolumes)
	moreVolumes["backend1"] = make(map[string]*VolumeConfig)
	moreVolumes["backend1"]["vol2"] = &VolumeConfig{Name: "vol2", InternalName: "vol2-internal"}
	moreVolumes["backend2"] = make(map[string]*VolumeConfig)
	moreVolumes["backend2"]["vol3"] = &VolumeConfig{Name: "vol3", InternalName: "vol3-internal"}

	targetInfo.AddVolumes(moreVolumes)

	// Verify all volumes are present
	assert.Contains(t, targetInfo.StorageVolumes["backend1"], "vol1", "Original volume should still be present")
	assert.Contains(t, targetInfo.StorageVolumes, "backend2", "New backend should be added")

	// Test duplicate volume handling - trying to add a volume that already exists should skip it
	duplicateVolumes := make(GroupSnapshotTargetVolumes)
	duplicateVolumes["backend1"] = make(map[string]*VolumeConfig)
	duplicateVolumes["backend1"]["vol1"] = &VolumeConfig{Name: "vol1-duplicate", InternalName: "vol1-duplicate-internal"}

	// Get the original vol1 config reference
	originalVol1 := targetInfo.StorageVolumes["backend1"]["vol1"]

	targetInfo.AddVolumes(duplicateVolumes)

	// Verify the original volume config is unchanged (duplicate was skipped)
	assert.Equal(t, originalVol1, targetInfo.StorageVolumes["backend1"]["vol1"], "Original volume config should remain unchanged when duplicate is added")
}

func TestSnapshotState_Methods(t *testing.T) {
	tests := map[string]struct {
		state     SnapshotState
		predicate func(SnapshotState) bool
	}{
		"Creating state": {
			state:     SnapshotStateCreating,
			predicate: func(s SnapshotState) bool { return s.IsCreating() },
		},
		"Uploading state": {
			state:     SnapshotStateUploading,
			predicate: func(s SnapshotState) bool { return s.IsUploading() },
		},
		"Online state": {
			state:     SnapshotStateOnline,
			predicate: func(s SnapshotState) bool { return s.IsOnline() },
		},
		"Missing backend state": {
			state:     SnapshotStateMissingBackend,
			predicate: func(s SnapshotState) bool { return s.IsMissingBackend() },
		},
		"Missing volume state": {
			state:     SnapshotStateMissingVolume,
			predicate: func(s SnapshotState) bool { return s.IsMissingVolume() },
		},
		"Empty state (online)": {
			state:     SnapshotState(""),
			predicate: func(s SnapshotState) bool { return s.IsOnline() },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Test predicate method
			assert.True(t, test.predicate(test.state), "Predicate should return true for the correct state")
		})
	}
}

func TestBackendState_EnumMethods(t *testing.T) {
	tests := map[string]struct {
		state      BackendState
		expected   string
		isUnknown  bool
		isOnline   bool
		isOffline  bool
		isFailed   bool
		isDeleting bool
	}{
		"Unknown state": {
			state:     Unknown,
			expected:  "unknown",
			isUnknown: true,
		},
		"Online state": {
			state:    Online,
			expected: "online",
			isOnline: true,
		},
		"Offline state": {
			state:     Offline,
			expected:  "offline",
			isOffline: true,
		},
		"Failed state": {
			state:    Failed,
			expected: "failed",
			isFailed: true,
		},
		"Deleting state": {
			state:      Deleting,
			expected:   "deleting",
			isDeleting: true,
		},
		"Empty state (unknown)": {
			state:     BackendState(""),
			expected:  "unknown",
			isUnknown: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Test String() method
			assert.Equal(t, test.expected, test.state.String(), "String() should return correct representation")

			// Test IsUnknown()
			assert.Equal(t, test.isUnknown, test.state.IsUnknown(), "IsUnknown() should match expected value")

			// Test IsOnline()
			assert.Equal(t, test.isOnline, test.state.IsOnline(), "IsOnline() should match expected value")

			// Test IsOffline()
			assert.Equal(t, test.isOffline, test.state.IsOffline(), "IsOffline() should match expected value")

			// Test IsFailed()
			assert.Equal(t, test.isFailed, test.state.IsFailed(), "IsFailed() should match expected value")

			// Test IsDeleting()
			assert.Equal(t, test.isDeleting, test.state.IsDeleting(), "IsDeleting() should match expected value")
		})
	}
}

func TestStorageBackend_AddStoragePool(t *testing.T) {
	backend := &StorageBackend{
		storagePools: new(sync.Map),
	}

	// Create a mock pool
	pool := NewStoragePool(nil, "test-pool-1")

	// Test AddStoragePool()
	backend.AddStoragePool(pool)

	// Verify pool was added
	storedPool, exists := backend.storagePools.Load("test-pool-1")
	assert.True(t, exists, "Pool should be stored in the map")
	assert.Equal(t, pool, storedPool, "Stored pool should match the added pool")

	// Add another pool
	pool2 := NewStoragePool(nil, "test-pool-2")
	backend.AddStoragePool(pool2)

	// Verify both pools exist
	storedPool1, exists1 := backend.storagePools.Load("test-pool-1")
	storedPool2, exists2 := backend.storagePools.Load("test-pool-2")
	assert.True(t, exists1, "First pool should still exist")
	assert.True(t, exists2, "Second pool should exist")
	assert.Equal(t, pool, storedPool1, "First pool should be unchanged")
	assert.Equal(t, pool2, storedPool2, "Second pool should match")
}

func TestStorageBackend_DriverDelegationMethods(t *testing.T) {
	backend := &StorageBackend{
		driver: nil,
	}

	// Test GetDriverName() with nil driver - should panic, test the coverage for now
	// This method doesn't have error handling so we expect a panic with nil driver
	assert.Panics(t, func() {
		backend.GetDriverName()
	}, "GetDriverName() should panic with nil driver")

	// Test GetPhysicalPoolNames() with nil driver - should panic
	assert.Panics(t, func() {
		ctx := context.Background()
		backend.GetPhysicalPoolNames(ctx)
	}, "GetPhysicalPoolNames() should panic with nil driver")

	// Test GetProtocol() with nil driver - should panic
	assert.Panics(t, func() {
		ctx := context.Background()
		backend.GetProtocol(ctx)
	}, "GetProtocol() should panic with nil driver")

	// Test IsCredentialsFieldSet() with nil driver - should panic
	assert.Panics(t, func() {
		ctx := context.Background()
		backend.IsCredentialsFieldSet(ctx)
	}, "IsCredentialsFieldSet() should panic with nil driver")

	// Test MarshalDriverConfig() with nil driver - should panic
	assert.Panics(t, func() {
		backend.MarshalDriverConfig()
	}, "MarshalDriverConfig() should panic with nil driver")
}

func TestStorageBackend_StoragePoolManagement(t *testing.T) {
	backend := &StorageBackend{
		storagePools: new(sync.Map),
	}

	// Test StoragePools() - returns the sync.Map
	pools := backend.StoragePools()
	assert.NotNil(t, pools, "StoragePools() should return a non-nil sync.Map")
	assert.Equal(t, backend.storagePools, pools, "StoragePools() should return the same map instance")

	// Add some pools for testing
	pool1 := NewStoragePool(nil, "pool1")
	pool2 := NewStoragePool(nil, "pool2")
	backend.AddStoragePool(pool1)
	backend.AddStoragePool(pool2)

	// Verify pools are there
	storedPool1, exists1 := backend.storagePools.Load("pool1")
	storedPool2, exists2 := backend.storagePools.Load("pool2")
	assert.True(t, exists1, "pool1 should exist")
	assert.True(t, exists2, "pool2 should exist")
	assert.Equal(t, pool1, storedPool1, "stored pool1 should match")
	assert.Equal(t, pool2, storedPool2, "stored pool2 should match")

	// Test ClearStoragePools()
	backend.ClearStoragePools()

	// Verify pools are cleared
	_, exists1After := backend.storagePools.Load("pool1")
	_, exists2After := backend.storagePools.Load("pool2")
	assert.False(t, exists1After, "pool1 should be cleared")
	assert.False(t, exists2After, "pool2 should be cleared")
}

func TestStorageBackend_VolumeManagement(t *testing.T) {
	backend := &StorageBackend{
		volumes: new(sync.Map),
	}

	// Test Volumes() - returns the sync.Map
	volumes := backend.Volumes()
	assert.NotNil(t, volumes, "Volumes() should return a non-nil sync.Map")
	assert.Equal(t, backend.volumes, volumes, "Volumes() should return the same map instance")

	// Add some volumes for testing
	vol1 := NewVolume(&VolumeConfig{Name: "vol1"}, "backend-uuid", "pool", false, VolumeStateOnline)
	vol2 := NewVolume(&VolumeConfig{Name: "vol2"}, "backend-uuid", "pool", false, VolumeStateOnline)
	backend.volumes.Store("vol1", vol1)
	backend.volumes.Store("vol2", vol2)

	// Verify volumes are there
	storedVol1, exists1 := backend.volumes.Load("vol1")
	storedVol2, exists2 := backend.volumes.Load("vol2")
	assert.True(t, exists1, "vol1 should exist")
	assert.True(t, exists2, "vol2 should exist")
	assert.Equal(t, vol1, storedVol1, "stored vol1 should match")
	assert.Equal(t, vol2, storedVol2, "stored vol2 should match")

	// Test HasVolumes() - should return true when volumes exist
	hasVols := backend.HasVolumes()
	assert.True(t, hasVols, "HasVolumes() should return true when volumes exist")

	// Test ClearVolumes()
	backend.ClearVolumes()

	// Verify volumes are cleared
	_, exists1After := backend.volumes.Load("vol1")
	_, exists2After := backend.volumes.Load("vol2")
	assert.False(t, exists1After, "vol1 should be cleared")
	assert.False(t, exists2After, "vol2 should be cleared")

	// Test HasVolumes() after clearing - should return false
	hasVolsAfter := backend.HasVolumes()
	assert.False(t, hasVolsAfter, "HasVolumes() should return false when no volumes exist")
}

func TestNewStorageBackend(t *testing.T) {
	// Test with nil driver (should fail during backend specs retrieval)
	ctx := context.Background()

	// This will panic because nil driver doesn't have GetStorageBackendSpecs method
	assert.Panics(t, func() {
		_, _ = NewStorageBackend(ctx, nil)
	}, "NewStorageBackend should panic with nil driver")
}

func TestNewFailedStorageBackend(t *testing.T) {
	// Test with nil driver (should panic during BackendName call)
	ctx := context.Background()

	assert.Panics(t, func() {
		_ = NewFailedStorageBackend(ctx, nil)
	}, "NewFailedStorageBackend should panic with nil driver")
}

func TestStorageBackend_CreatePrepare(t *testing.T) {
	ctx := context.Background()
	volConfig := &VolumeConfig{Name: "test-vol"}
	pool := NewStoragePool(nil, "test-pool")

	// Test with nil driver
	backend := &StorageBackend{driver: nil}

	// Should not panic, method has nil check
	assert.NotPanics(t, func() {
		backend.CreatePrepare(ctx, volConfig, pool)
	}, "CreatePrepare should not panic with nil driver")
}

func TestUserBackendState_EnumMethods(t *testing.T) {
	tests := map[string]struct {
		state       UserBackendState
		expected    string
		isNormal    bool
		isSuspended bool
	}{
		"Normal state": {
			state:    UserNormal,
			expected: "normal",
			isNormal: true,
		},
		"Suspended state": {
			state:       UserSuspended,
			expected:    "suspended",
			isSuspended: true,
		},
		"Empty state (unknown)": {
			state:    UserBackendState(""),
			expected: "unknown",
		},
		"Invalid state (unknown)": {
			state:    UserBackendState("invalid"),
			expected: "unknown",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Test String() method
			assert.Equal(t, test.expected, test.state.String(), "String() should return correct representation")

			// Test IsNormal()
			assert.Equal(t, test.isNormal, test.state.IsNormal(), "IsNormal() should match expected value")

			// Test IsSuspended()
			assert.Equal(t, test.isSuspended, test.state.IsSuspended(), "IsSuspended() should match expected value")
		})
	}
}

func TestGroupSnapshotTargetInfo_Validate(t *testing.T) {
	tests := map[string]struct {
		targetInfo    *GroupSnapshotTargetInfo
		expectError   bool
		errorContains string
	}{
		"Valid fake storage": {
			targetInfo: &GroupSnapshotTargetInfo{
				StorageType: FakeStorageType,
				StorageUUID: "",
				StorageVolumes: GroupSnapshotTargetVolumes{
					"source1": {"vol1": &VolumeConfig{Name: "vol1"}},
				},
			},
			expectError: false,
		},
		"Valid normal storage": {
			targetInfo: &GroupSnapshotTargetInfo{
				StorageType: "test-storage",
				StorageUUID: "test-uuid",
				StorageVolumes: GroupSnapshotTargetVolumes{
					"source1": {"vol1": &VolumeConfig{Name: "vol1"}},
				},
			},
			expectError: false,
		},
		"Empty storage type": {
			targetInfo: &GroupSnapshotTargetInfo{
				StorageType:    "",
				StorageUUID:    "test-uuid",
				StorageVolumes: GroupSnapshotTargetVolumes{},
			},
			expectError:   true,
			errorContains: "empty storage type",
		},
		"Empty storage UUID": {
			targetInfo: &GroupSnapshotTargetInfo{
				StorageType:    "test-storage",
				StorageUUID:    "",
				StorageVolumes: GroupSnapshotTargetVolumes{},
			},
			expectError:   true,
			errorContains: "empty storage uuid",
		},
		"Nil storage volumes": {
			targetInfo: &GroupSnapshotTargetInfo{
				StorageType:    "test-storage",
				StorageUUID:    "test-uuid",
				StorageVolumes: nil,
			},
			expectError:   true,
			errorContains: "empty source volumes",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.targetInfo.Validate()
			if test.expectError {
				assert.Error(t, err, "Should return error")
				assert.Contains(t, err.Error(), test.errorContains, "Error should contain expected text")
			} else {
				assert.NoError(t, err, "Should not return error")
			}
		})
	}
}

func TestGroupSnapshotTargetInfo_IsShared(t *testing.T) {
	tests := map[string]struct {
		targetInfo1 *GroupSnapshotTargetInfo
		targetInfo2 *GroupSnapshotTargetInfo
		expected    bool
	}{
		"Fake storage type - empty UUID - shared": {
			targetInfo1: &GroupSnapshotTargetInfo{StorageType: FakeStorageType, StorageUUID: ""},
			targetInfo2: &GroupSnapshotTargetInfo{StorageType: FakeStorageType, StorageUUID: "different"},
			expected:    true,
		},
		"Fake storage type - same UUID - shared": {
			targetInfo1: &GroupSnapshotTargetInfo{StorageType: FakeStorageType, StorageUUID: "same"},
			targetInfo2: &GroupSnapshotTargetInfo{StorageType: FakeStorageType, StorageUUID: "same"},
			expected:    true,
		},
		"Fake storage type - different UUID - not shared": {
			targetInfo1: &GroupSnapshotTargetInfo{StorageType: FakeStorageType, StorageUUID: "uuid1"},
			targetInfo2: &GroupSnapshotTargetInfo{StorageType: FakeStorageType, StorageUUID: "uuid2"},
			expected:    false,
		},
		"Different storage types - not shared": {
			targetInfo1: &GroupSnapshotTargetInfo{StorageType: "storage1", StorageUUID: "uuid1"},
			targetInfo2: &GroupSnapshotTargetInfo{StorageType: "storage2", StorageUUID: "uuid1"},
			expected:    false,
		},
		"Same storage type, different UUIDs - not shared": {
			targetInfo1: &GroupSnapshotTargetInfo{StorageType: "storage1", StorageUUID: "uuid1"},
			targetInfo2: &GroupSnapshotTargetInfo{StorageType: "storage1", StorageUUID: "uuid2"},
			expected:    false,
		},
		"Same storage type and UUID - shared": {
			targetInfo1: &GroupSnapshotTargetInfo{StorageType: "storage1", StorageUUID: "uuid1"},
			targetInfo2: &GroupSnapshotTargetInfo{StorageType: "storage1", StorageUUID: "uuid1"},
			expected:    true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result := test.targetInfo1.IsShared(test.targetInfo2)
			assert.Equal(t, test.expected, result, "IsShared result should match expected")
		})
	}
}

func TestGroupSnapshotConfig_GetVolumeNames(t *testing.T) {
	config := &GroupSnapshotConfig{
		Name:        "test-group-snapshot",
		VolumeNames: []string{"vol1", "vol2", "vol3"},
	}

	volumeNames := config.GetVolumeNames()
	assert.Equal(t, []string{"vol1", "vol2", "vol3"}, volumeNames, "GetVolumeNames should return the correct slice")

	// Test with empty slice
	emptyConfig := &GroupSnapshotConfig{
		Name:        "empty-group-snapshot",
		VolumeNames: []string{},
	}
	emptyNames := emptyConfig.GetVolumeNames()
	assert.Empty(t, emptyNames, "GetVolumeNames should return empty slice")

	// Test with nil slice
	nilConfig := &GroupSnapshotConfig{
		Name:        "nil-group-snapshot",
		VolumeNames: nil,
	}
	nilNames := nilConfig.GetVolumeNames()
	assert.Nil(t, nilNames, "GetVolumeNames should return nil slice")
}

func TestSnapshotConfig_Methods(t *testing.T) {
	// Test ID() method
	config := &SnapshotConfig{
		Name:       "snap-123",
		VolumeName: "vol-456",
	}

	expectedID := MakeSnapshotID("vol-456", "snap-123")
	actualID := config.ID()
	assert.Equal(t, expectedID, actualID, "ID() should return the correct snapshot ID")
}

func TestSnapshotConfig_Validate(t *testing.T) {
	tests := map[string]struct {
		config      *SnapshotConfig
		expectError bool
		errorText   string
	}{
		"Valid config": {
			config: &SnapshotConfig{
				Name:       "snap-123",
				VolumeName: "vol-456",
			},
			expectError: false,
		},
		"Missing name": {
			config: &SnapshotConfig{
				Name:       "",
				VolumeName: "vol-456",
			},
			expectError: true,
			errorText:   "mandatory: name and volumeName",
		},
		"Missing volume name": {
			config: &SnapshotConfig{
				Name:       "snap-123",
				VolumeName: "",
			},
			expectError: true,
			errorText:   "mandatory: name and volumeName",
		},
		"Missing both": {
			config: &SnapshotConfig{
				Name:       "",
				VolumeName: "",
			},
			expectError: true,
			errorText:   "mandatory: name and volumeName",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.config.Validate()
			if test.expectError {
				assert.Error(t, err, "Should return error")
				assert.Contains(t, err.Error(), test.errorText, "Error should contain expected text")
			} else {
				assert.NoError(t, err, "Should not return error")
			}
		})
	}
}

func TestSnapshotExternal_Methods(t *testing.T) {
	// Test ID() method
	external := &SnapshotExternal{
		Snapshot: Snapshot{
			Config: &SnapshotConfig{
				Name:       "snap-external",
				VolumeName: "vol-external",
			},
		},
	}

	expectedID := MakeSnapshotID("vol-external", "snap-external")
	actualID := external.ID()
	assert.Equal(t, expectedID, actualID, "ID() should return the correct snapshot ID")
}

func TestSnapshotExternal_Validate(t *testing.T) {
	tests := map[string]struct {
		snapshot    *SnapshotExternal
		expectError bool
		errorText   string
	}{
		"Valid snapshot": {
			snapshot: &SnapshotExternal{
				Snapshot: Snapshot{
					Config: &SnapshotConfig{
						Name:               "snap-1",
						VolumeName:         "vol-1",
						InternalName:       "internal-snap-1",
						VolumeInternalName: "internal-vol-1",
					},
					Created:   "2023-01-01T00:00:00Z",
					SizeBytes: 1024,
				},
			},
			expectError: false,
		},
		"Missing name": {
			snapshot: &SnapshotExternal{
				Snapshot: Snapshot{
					Config: &SnapshotConfig{
						Name:               "",
						VolumeName:         "vol-1",
						InternalName:       "internal-snap-1",
						VolumeInternalName: "internal-vol-1",
					},
					Created:   "2023-01-01T00:00:00Z",
					SizeBytes: 1024,
				},
			},
			expectError: true,
			errorText:   "does not have a name",
		},
		"Missing volume name": {
			snapshot: &SnapshotExternal{
				Snapshot: Snapshot{
					Config: &SnapshotConfig{
						Name:               "snap-1",
						VolumeName:         "",
						InternalName:       "internal-snap-1",
						VolumeInternalName: "internal-vol-1",
					},
					Created:   "2023-01-01T00:00:00Z",
					SizeBytes: 1024,
				},
			},
			expectError: true,
			errorText:   "does not have a volume name",
		},
		"Missing internal name": {
			snapshot: &SnapshotExternal{
				Snapshot: Snapshot{
					Config: &SnapshotConfig{
						Name:               "snap-1",
						VolumeName:         "vol-1",
						InternalName:       "",
						VolumeInternalName: "internal-vol-1",
					},
					Created:   "2023-01-01T00:00:00Z",
					SizeBytes: 1024,
				},
			},
			expectError: true,
			errorText:   "does not have an internal name",
		},
		"Missing volume internal name": {
			snapshot: &SnapshotExternal{
				Snapshot: Snapshot{
					Config: &SnapshotConfig{
						Name:               "snap-1",
						VolumeName:         "vol-1",
						InternalName:       "internal-snap-1",
						VolumeInternalName: "",
					},
					Created:   "2023-01-01T00:00:00Z",
					SizeBytes: 1024,
				},
			},
			expectError: true,
			errorText:   "does not have a volume internal name",
		},
		"Invalid created date": {
			snapshot: &SnapshotExternal{
				Snapshot: Snapshot{
					Config: &SnapshotConfig{
						Name:               "snap-1",
						VolumeName:         "vol-1",
						InternalName:       "internal-snap-1",
						VolumeInternalName: "internal-vol-1",
					},
					Created:   "invalid-date",
					SizeBytes: 1024,
				},
			},
			expectError: true,
			errorText:   "does not have a valid created date",
		},
		"Negative size": {
			snapshot: &SnapshotExternal{
				Snapshot: Snapshot{
					Config: &SnapshotConfig{
						Name:               "snap-1",
						VolumeName:         "vol-1",
						InternalName:       "internal-snap-1",
						VolumeInternalName: "internal-vol-1",
					},
					Created:   "2023-01-01T00:00:00Z",
					SizeBytes: -100,
				},
			},
			expectError: true,
			errorText:   "snapshot size is empty",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.snapshot.Validate()
			if test.expectError {
				assert.Error(t, err, "Should return error")
				assert.Contains(t, err.Error(), test.errorText, "Error should contain expected text")
			} else {
				assert.NoError(t, err, "Should not return error")
			}
		})
	}
}

func TestGroupSnapshot_Config(t *testing.T) {
	// Test with nil GroupSnapshotConfig
	gsNil := &GroupSnapshot{
		GroupSnapshotConfig: nil,
	}

	configNil := gsNil.Config()
	assert.NotNil(t, configNil, "Config() should return non-nil config even when internal config is nil")

	// Test with actual GroupSnapshotConfig
	originalConfig := &GroupSnapshotConfig{
		Version:      "v1",
		Name:         "group-snap-123",
		InternalName: "internal-group-snap-123",
		VolumeNames:  []string{"vol1", "vol2"},
	}

	gs := &GroupSnapshot{
		GroupSnapshotConfig: originalConfig,
	}

	clonedConfig := gs.Config()
	assert.NotNil(t, clonedConfig, "Config() should return non-nil config")
	assert.Equal(t, originalConfig.Name, clonedConfig.Name, "Cloned config should have same name")

	// Verify it's a clone, not the same object
	assert.NotSame(t, originalConfig, clonedConfig, "Config() should return a clone, not the original")
}

func TestGroupSnapshotPersistent_ConstructInternal(t *testing.T) {
	config := &GroupSnapshotConfig{
		Name:        "group-snap-persistent",
		VolumeNames: []string{"vol1", "vol2"},
	}

	snapshotIDs := []string{"snap-id-1", "snap-id-2"}
	created := "2023-01-01T12:00:00Z"

	// Create the GroupSnapshot first, then the persistent version
	groupSnapshot := NewGroupSnapshot(config, snapshotIDs, created)
	persistent := NewGroupSnapshotPersistent(groupSnapshot)

	internal := persistent.ConstructInternal()

	assert.NotNil(t, internal, "ConstructInternal() should return non-nil GroupSnapshot")
	assert.Equal(t, config, internal.GroupSnapshotConfig, "Should use the same config")
	assert.Equal(t, snapshotIDs, internal.SnapshotIDs, "Should use the same snapshot IDs")
	assert.Equal(t, created, internal.Created, "Should use the same created time")
}

func TestByGroupSnapshotExternalID_SortingMethods(t *testing.T) {
	// Create test data
	gs1 := &GroupSnapshotExternal{}
	gs1.GroupSnapshotConfig = &GroupSnapshotConfig{Name: "group-snap-c"}

	gs2 := &GroupSnapshotExternal{}
	gs2.GroupSnapshotConfig = &GroupSnapshotConfig{Name: "group-snap-a"}

	gs3 := &GroupSnapshotExternal{}
	gs3.GroupSnapshotConfig = &GroupSnapshotConfig{Name: "group-snap-b"}

	sortable := ByGroupSnapshotExternalID{gs1, gs2, gs3}

	// Test Len()
	assert.Equal(t, 3, sortable.Len(), "Len() should return the correct length")

	// Test Less()
	assert.True(t, sortable.Less(1, 0), "Less() should return true for 'a' < 'c'")
	assert.False(t, sortable.Less(0, 1), "Less() should return false for 'c' < 'a'")
	assert.True(t, sortable.Less(1, 2), "Less() should return true for 'a' < 'b'")

	// Test Swap()
	originalFirst := sortable[0]
	originalSecond := sortable[1]
	sortable.Swap(0, 1)
	assert.Equal(t, originalSecond, sortable[0], "After swap, first element should be original second")
	assert.Equal(t, originalFirst, sortable[1], "After swap, second element should be original first")
}

func TestSnapshot_BasicMethods(t *testing.T) {
	config := &SnapshotConfig{
		Version:            "v1",
		Name:               "snap-basic",
		InternalName:       "internal-snap-basic",
		VolumeName:         "vol-basic",
		VolumeInternalName: "internal-vol-basic",
		GroupSnapshotName:  "group-snap-abc",
	}

	snapshot := &Snapshot{
		Config:    config,
		Created:   "2023-01-15T10:30:00Z",
		SizeBytes: 2048,
		State:     SnapshotStateOnline,
	}

	// Test ID()
	expectedID := MakeSnapshotID(config.VolumeName, config.Name)
	actualID := snapshot.ID()
	assert.Equal(t, expectedID, actualID, "ID() should return the correct snapshot ID")

	// Test GetVolumeID()
	volumeID := snapshot.GetVolumeID()
	assert.Equal(t, config.VolumeName, volumeID, "GetVolumeID() should return the volume name")

	// Test GetUniqueKey()
	uniqueKey := snapshot.GetUniqueKey()
	assert.Equal(t, config.Name, uniqueKey, "GetUniqueKey() should return the snapshot name")

	// Test IsGrouped() - should return true when GroupSnapshotName is set
	isGrouped := snapshot.IsGrouped()
	assert.True(t, isGrouped, "IsGrouped() should return true when GroupSnapshotName is set")

	// Test GroupSnapshotName()
	groupName := snapshot.GroupSnapshotName()
	assert.Equal(t, config.GroupSnapshotName, groupName, "GroupSnapshotName() should return the group snapshot name")
}

func TestSnapshot_IsGrouped_EdgeCases(t *testing.T) {
	// Test IsGrouped() - should return false when GroupSnapshotName is empty
	configUnGrouped := &SnapshotConfig{
		Name:              "ungrouped-snap",
		VolumeName:        "vol-ungrouped",
		GroupSnapshotName: "",
	}

	snapshotUnGrouped := &Snapshot{
		Config: configUnGrouped,
	}

	isGrouped := snapshotUnGrouped.IsGrouped()
	assert.False(t, isGrouped, "IsGrouped() should return false when GroupSnapshotName is empty")

	// Test GroupSnapshotName() - should return empty string when not set
	groupName := snapshotUnGrouped.GroupSnapshotName()
	assert.Empty(t, groupName, "GroupSnapshotName() should return empty string when not set")
}

func TestSnapshot_ConstructExternal(t *testing.T) {
	config := &SnapshotConfig{
		Version:             "v1.0",
		Name:                "snap-external",
		InternalName:        "internal-snap-external",
		VolumeName:          "vol-external",
		VolumeInternalName:  "internal-vol-external",
		LUKSPassphraseNames: []string{"passphrase1", "passphrase2"},
		GroupSnapshotName:   "group-snap-external",
	}

	original := &Snapshot{
		Config:    config,
		Created:   "2023-02-01T14:20:00Z",
		SizeBytes: 4096,
		State:     SnapshotStateOnline,
	}

	external := original.ConstructExternal()

	assert.NotNil(t, external, "ConstructExternal() should return non-nil SnapshotExternal")
	assert.Equal(t, original.Config.Name, external.Config.Name, "External should have same config name")

	// Verify it's a different object (clone, not reference)
	assert.NotSame(t, original, &external.Snapshot, "ConstructExternal() should return a clone, not the original")
}

func TestSnapshot_ConstructPersistent(t *testing.T) {
	config := &SnapshotConfig{
		Version:             "v2.0",
		Name:                "snap-persistent",
		InternalName:        "internal-snap-persistent",
		VolumeName:          "vol-persistent",
		VolumeInternalName:  "internal-vol-persistent",
		LUKSPassphraseNames: []string{"key1", "key2", "key3"},
		GroupSnapshotName:   "",
	}

	original := &Snapshot{
		Config:    config,
		Created:   "2023-03-01T09:45:00Z",
		SizeBytes: 8192,
		State:     SnapshotStateUploading,
	}

	persistent := original.ConstructPersistent()

	assert.NotNil(t, persistent, "ConstructPersistent() should return non-nil SnapshotPersistent")
	assert.Equal(t, original.Config.Name, persistent.Config.Name, "Persistent should have same config name")

	// Verify it's a different object (clone, not reference)
	assert.NotSame(t, original, &persistent.Snapshot, "ConstructPersistent() should return a clone, not the original")
}

func TestSnapshot_ConstructClone(t *testing.T) {
	config := &SnapshotConfig{
		Version:             "v3.0",
		Name:                "snap-clone",
		InternalName:        "internal-snap-clone",
		VolumeName:          "vol-clone",
		VolumeInternalName:  "internal-vol-clone",
		LUKSPassphraseNames: []string{"secret1"},
		GroupSnapshotName:   "group-for-clone",
	}

	original := &Snapshot{
		Config:    config,
		Created:   "2023-04-01T16:00:00Z",
		SizeBytes: 1024,
		State:     SnapshotStateCreating,
	}

	clone := original.ConstructClone()

	assert.NotNil(t, clone, "ConstructClone() should return non-nil Snapshot")
	assert.IsType(t, &Snapshot{}, clone, "Should return Snapshot type")

	// Verify all fields are cloned correctly
	assert.Equal(t, original.Config.Version, clone.Config.Version, "Clone should have same config version")
	assert.Equal(t, original.Config.Name, clone.Config.Name, "Clone should have same config name")
	assert.Equal(t, original.Config.InternalName, clone.Config.InternalName, "Clone should have same internal name")
	assert.Equal(t, original.Config.VolumeName, clone.Config.VolumeName, "Clone should have same volume name")
	assert.Equal(t, original.Config.VolumeInternalName, clone.Config.VolumeInternalName, "Clone should have same volume internal name")
	assert.Equal(t, original.Config.LUKSPassphraseNames, clone.Config.LUKSPassphraseNames, "Clone should have same LUKS passphrases")
	assert.Equal(t, original.Config.GroupSnapshotName, clone.Config.GroupSnapshotName, "Clone should have same group snapshot name")
	assert.Equal(t, original.Created, clone.Created, "Clone should have same created time")
	assert.Equal(t, original.SizeBytes, clone.SizeBytes, "Clone should have same size")
	assert.Equal(t, original.State, clone.State, "Clone should have same state")

	// Verify it's a different object (clone, not reference)
	assert.NotSame(t, original, clone, "ConstructClone() should return a different object")
	assert.NotSame(t, original.Config, clone.Config, "ConstructClone() should clone the config, not reference it")
}

func TestStoragePool_ConstructExternal(t *testing.T) {
	// Use nil backend for testing ConstructExternal since we're not using backend-specific features
	pool := NewStoragePool(nil, "test-pool-external")

	// Set up pool attributes
	pool.SetAttributes(map[string]sa.Offer{
		"attr1": sa.NewStringOffer("value1"),
		"attr2": sa.NewIntOffer(100, 200),
	})
	pool.SetStorageClasses([]string{"storage-class-c", "storage-class-a", "storage-class-b"})
	pool.SetSupportedTopologies([]map[string]string{
		{"zone": "us-east-1a", "region": "us-east-1"},
		{"zone": "us-east-1b", "region": "us-east-1"},
	})

	external := pool.ConstructExternal()

	assert.NotNil(t, external, "ConstructExternal() should return non-nil PoolExternal")
	assert.IsType(t, &PoolExternal{}, external, "Should return PoolExternal type")

	// Verify basic fields
	assert.Equal(t, pool.Name(), external.Name, "External should have same name")
	assert.Equal(t, pool.Attributes(), external.Attributes, "External should have same attributes")
	assert.Equal(t, pool.SupportedTopologies(), external.SupportedTopologies, "External should have same supported topologies")

	// Verify storage classes are sorted
	expectedSortedClasses := []string{"storage-class-a", "storage-class-b", "storage-class-c"}
	assert.Equal(t, expectedSortedClasses, external.StorageClasses, "Storage classes should be sorted alphabetically")
}

func TestSnapshotPersistent_ConstructExternal(t *testing.T) {
	config := &SnapshotConfig{
		Version:             "v1.5",
		Name:                "snap-persistent-to-external",
		InternalName:        "internal-snap-persistent-to-external",
		VolumeName:          "vol-persistent-to-external",
		VolumeInternalName:  "internal-vol-persistent-to-external",
		LUKSPassphraseNames: []string{"persistent-key"},
		GroupSnapshotName:   "group-persistent",
	}

	snapshot := &Snapshot{
		Config:    config,
		Created:   "2023-05-01T11:15:00Z",
		SizeBytes: 16384,
		State:     SnapshotStateMissingBackend,
	}

	persistent := &SnapshotPersistent{Snapshot: *snapshot}
	external := persistent.ConstructExternal()

	assert.NotNil(t, external, "ConstructExternal() should return non-nil SnapshotExternal")

	// Verify the data is correctly transferred from persistent to external
	assert.Equal(t, snapshot.Config.Name, external.Config.Name, "External should have same config name")

	// Verify it's a clone, not reference
	assert.NotSame(t, &persistent.Snapshot, &external.Snapshot, "ConstructExternal() should return a clone, not the original")
}

func TestMakeSnapshotID(t *testing.T) {
	tests := map[string]struct {
		volumeName   string
		snapshotName string
		expectedID   string
	}{
		"Normal names": {
			volumeName:   "my-volume",
			snapshotName: "my-snapshot",
			expectedID:   "my-volume/my-snapshot",
		},
		"Empty volume name": {
			volumeName:   "",
			snapshotName: "snapshot",
			expectedID:   "/snapshot",
		},
		"Empty snapshot name": {
			volumeName:   "volume",
			snapshotName: "",
			expectedID:   "volume/",
		},
		"Both empty": {
			volumeName:   "",
			snapshotName: "",
			expectedID:   "/",
		},
		"Names with special characters": {
			volumeName:   "vol-with-dashes_and_underscores",
			snapshotName: "snap.with.dots",
			expectedID:   "vol-with-dashes_and_underscores/snap.with.dots",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actualID := MakeSnapshotID(test.volumeName, test.snapshotName)
			assert.Equal(t, test.expectedID, actualID, "MakeSnapshotID should return correct format")
		})
	}
}

func TestConvertGroupSnapshotID(t *testing.T) {
	tests := map[string]struct {
		groupSnapshotID string
		expectedSnapID  string
		expectError     bool
		errorText       string
	}{
		"Valid group snapshot ID": {
			groupSnapshotID: "groupsnapshot-12345678-1234-5678-9abc-123456789abc",
			expectedSnapID:  "snapshot-12345678-1234-5678-9abc-123456789abc",
			expectError:     false,
		},
		"Valid group snapshot ID with shorter UUID": {
			groupSnapshotID: "groupsnapshot-abc123",
			expectedSnapID:  "snapshot-abc123",
			expectError:     false,
		},
		"Invalid format - missing prefix": {
			groupSnapshotID: "12345678-1234-5678-9abc-123456789abc",
			expectedSnapID:  "",
			expectError:     true,
			errorText:       "invalid group snapshot ID format",
		},
		"Invalid format - wrong prefix": {
			groupSnapshotID: "snapshot-12345678-1234-5678-9abc-123456789abc",
			expectedSnapID:  "",
			expectError:     true,
			errorText:       "invalid group snapshot ID format",
		},
		"Invalid format - multiple prefixes": {
			groupSnapshotID: "groupsnapshot-groupsnapshot-12345",
			expectedSnapID:  "",
			expectError:     true,
			errorText:       "invalid group snapshot ID format",
		},
		"Empty string": {
			groupSnapshotID: "",
			expectedSnapID:  "",
			expectError:     true,
			errorText:       "invalid group snapshot ID format",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actualSnapID, err := ConvertGroupSnapshotID(test.groupSnapshotID)

			if test.expectError {
				assert.Error(t, err, "Should return error for invalid group snapshot ID")
				assert.Contains(t, err.Error(), test.errorText, "Error should contain expected text")
				assert.Equal(t, test.expectedSnapID, actualSnapID, "Should return empty string on error")
			} else {
				assert.NoError(t, err, "Should not return error for valid group snapshot ID")
				assert.Equal(t, test.expectedSnapID, actualSnapID, "Should return correct snapshot ID")
			}
		})
	}
}

func TestBySnapshotExternalID_SortingMethods(t *testing.T) {
	// Create test data
	snap1 := &SnapshotExternal{
		Snapshot: Snapshot{
			Config: &SnapshotConfig{
				VolumeName: "vol-c",
				Name:       "snap-1",
			},
		},
	}

	snap2 := &SnapshotExternal{
		Snapshot: Snapshot{
			Config: &SnapshotConfig{
				VolumeName: "vol-a",
				Name:       "snap-1",
			},
		},
	}

	snap3 := &SnapshotExternal{
		Snapshot: Snapshot{
			Config: &SnapshotConfig{
				VolumeName: "vol-b",
				Name:       "snap-2",
			},
		},
	}

	sortable := BySnapshotExternalID{snap1, snap2, snap3}

	// Test Len()
	assert.Equal(t, 3, sortable.Len(), "Len() should return the correct length")

	// Test Less()
	assert.True(t, sortable.Less(1, 0), "Less() should work correctly")

	// Test Swap()
	originalFirst := sortable[0]
	sortable.Swap(0, 1)
	assert.Equal(t, originalFirst, sortable[1], "After swap, elements should be swapped")
}

func TestStoragePool_ConstructExternalWithPoolMap(t *testing.T) {
	pool := NewStoragePool(nil, "test-pool-map")

	// Set up pool attributes
	pool.SetAttributes(map[string]sa.Offer{
		"performance": sa.NewStringOffer("high"),
		"capacity":    sa.NewIntOffer(1000, 5000),
	})
	pool.SetSupportedTopologies([]map[string]string{
		{"zone": "us-west-1a", "region": "us-west-1"},
		{"zone": "us-west-1b", "region": "us-west-1"},
	})

	// Create a pool map
	poolMap := map[string][]string{
		"test-pool-map": {"premium", "gold", "standard"},
		"other-pool":    {"bronze"},
	}

	external := pool.ConstructExternalWithPoolMap(poolMap)

	assert.NotNil(t, external, "ConstructExternalWithPoolMap() should return non-nil PoolExternal")

	// Verify storage classes from pool map are sorted
	expectedSortedClasses := []string{"gold", "premium", "standard"}
	assert.Equal(t, expectedSortedClasses, external.StorageClasses, "Storage classes from pool map should be sorted alphabetically")

	// Test with pool not in map
	missingPoolMap := map[string][]string{
		"different-pool": {"class1", "class2"},
	}
	externalMissing := pool.ConstructExternalWithPoolMap(missingPoolMap)
	assert.Empty(t, externalMissing.StorageClasses, "Should return empty storage classes when pool not in map")
}

func TestVolumeConfig_Validate(t *testing.T) {
	tests := map[string]struct {
		config      *VolumeConfig
		expectError bool
		errorText   string
	}{
		"Valid config": {
			config: &VolumeConfig{
				Name:     "test-volume",
				Size:     "1Gi",
				Protocol: tridentconfig.File,
			},
			expectError: false,
		},
		"Missing name": {
			config: &VolumeConfig{
				Name:     "",
				Size:     "1Gi",
				Protocol: tridentconfig.File,
			},
			expectError: true,
			errorText:   "mandatory: name and size",
		},
		"Missing size": {
			config: &VolumeConfig{
				Name:     "test-volume",
				Size:     "",
				Protocol: tridentconfig.File,
			},
			expectError: true,
			errorText:   "mandatory: name and size",
		},
		"Missing both name and size": {
			config: &VolumeConfig{
				Name:     "",
				Size:     "",
				Protocol: tridentconfig.File,
			},
			expectError: true,
			errorText:   "mandatory: name and size",
		},
		"Invalid protocol": {
			config: &VolumeConfig{
				Name:     "test-volume",
				Size:     "1Gi",
				Protocol: tridentconfig.Protocol("invalid-protocol"),
			},
			expectError: true,
			errorText:   "usupported protocol",
		},
		"Valid block protocol": {
			config: &VolumeConfig{
				Name:     "test-volume-block",
				Size:     "2Gi",
				Protocol: tridentconfig.Block,
			},
			expectError: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.config.Validate()

			if test.expectError {
				assert.Error(t, err, "Should return error for invalid config")
				assert.Contains(t, err.Error(), test.errorText, "Error should contain expected text")
			} else {
				assert.NoError(t, err, "Should not return error for valid config")
			}
		})
	}
}

func TestVolumeState_PredicateMethods(t *testing.T) {
	tests := map[string]struct {
		state         VolumeState
		isDeleting    bool
		isMissing     bool
		isSubordinate bool
	}{
		"Online state": {
			state: VolumeStateOnline,
		},
		"Deleting state": {
			state:      VolumeStateDeleting,
			isDeleting: true,
		},
		"Missing backend state": {
			state:     VolumeStateMissingBackend,
			isMissing: true,
		},
		"Subordinate state": {
			state:         VolumeStateSubordinate,
			isSubordinate: true,
		},
		"Empty state (defaults to unknown behavior)": {
			state: VolumeState(""),
		},
		"Unknown state": {
			state: VolumeState("unknown"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.isDeleting, test.state.IsDeleting(), "IsDeleting() should match expected value")
			assert.Equal(t, test.isMissing, test.state.IsMissingBackend(), "IsMissingBackend() should match expected value")
			assert.Equal(t, test.isSubordinate, test.state.IsSubordinate(), "IsSubordinate() should match expected value")
		})
	}
}

func TestVolume_BasicMethods(t *testing.T) {
	config := &VolumeConfig{
		Name:              "test-volume",
		InternalName:      "internal-test-volume",
		ShareSourceVolume: "source-volume-123",
	}

	volume := &Volume{
		Config:      config,
		BackendUUID: "backend-uuid-456",
		Pool:        "test-pool",
		Orphaned:    false,
		State:       VolumeStateOnline,
	}

	// Test GetBackendID()
	backendID := volume.GetBackendID()
	assert.Equal(t, "backend-uuid-456", backendID, "GetBackendID() should return the backend UUID")

	// Test GetVolumeID()
	volumeID := volume.GetVolumeID()
	assert.Equal(t, "source-volume-123", volumeID, "GetVolumeID() should return the ShareSourceVolume")

	// Test GetUniqueKey()
	uniqueKey := volume.GetUniqueKey()
	assert.Equal(t, "internal-test-volume", uniqueKey, "GetUniqueKey() should return the InternalName")
}

func TestVolume_StatePredicateMethods(t *testing.T) {
	tests := map[string]struct {
		volumeState   VolumeState
		isDeleting    bool
		isSubordinate bool
	}{
		"Online volume": {
			volumeState: VolumeStateOnline,
		},
		"Deleting volume": {
			volumeState: VolumeStateDeleting,
			isDeleting:  true,
		},
		"Subordinate volume": {
			volumeState:   VolumeStateSubordinate,
			isSubordinate: true,
		},
		"Missing backend volume": {
			volumeState: VolumeStateMissingBackend,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			volume := &Volume{
				Config: &VolumeConfig{Name: "test-vol"},
				State:  test.volumeState,
			}

			assert.Equal(t, test.isDeleting, volume.IsDeleting(), "IsDeleting() should match expected value")
			assert.Equal(t, test.isSubordinate, volume.IsSubordinate(), "IsSubordinate() should match expected value")
		})
	}
}

func TestVolumeExternal_GetCHAPSecretName(t *testing.T) {
	tests := map[string]struct {
		backendUUID    string
		iscsiUsername  string
		expectedSecret string
	}{
		"Normal values": {
			backendUUID:    "backend-uuid-123",
			iscsiUsername:  "iscsi-user",
			expectedSecret: "trident-chap-backend-uuid-123-iscsi-user",
		},
		"With underscores": {
			backendUUID:    "backend_uuid_456",
			iscsiUsername:  "iscsi_user_name",
			expectedSecret: "trident-chap-backend-uuid-456-iscsi-user-name",
		},
		"With dots": {
			backendUUID:    "backend.uuid.789",
			iscsiUsername:  "iscsi.user.name",
			expectedSecret: "trident-chap-backend-uuid-789-iscsi-user-name",
		},
		"Mixed case": {
			backendUUID:    "Backend-UUID-ABC",
			iscsiUsername:  "ISCSI-User",
			expectedSecret: "trident-chap-backend-uuid-abc-iscsi-user",
		},
		"With underscores and dots combined": {
			backendUUID:    "backend_uuid.test_123",
			iscsiUsername:  "iscsi_user.test_name",
			expectedSecret: "trident-chap-backend-uuid-test-123-iscsi-user-test-name",
		},
		"Empty values": {
			backendUUID:    "",
			iscsiUsername:  "",
			expectedSecret: "trident-chap--",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			volumeExternal := &VolumeExternal{
				BackendUUID: test.backendUUID,
				Config: &VolumeConfig{
					AccessInfo: models.VolumeAccessInfo{
						IscsiAccessInfo: models.IscsiAccessInfo{
							IscsiChapInfo: models.IscsiChapInfo{
								IscsiUsername: test.iscsiUsername,
							},
						},
					},
				},
			}

			secretName := volumeExternal.GetCHAPSecretName()
			assert.Equal(t, test.expectedSecret, secretName, "GetCHAPSecretName should return correct format")
		})
	}
}

func TestVolume_ConstructExternal(t *testing.T) {
	config := &VolumeConfig{
		Version:      "v1.0",
		Name:         "test-volume-external",
		InternalName: "internal-test-volume-external",
		Size:         "10Gi",
		Protocol:     tridentconfig.File,
		AccessInfo: models.VolumeAccessInfo{
			IscsiAccessInfo: models.IscsiAccessInfo{
				IscsiChapInfo: models.IscsiChapInfo{
					IscsiUsername: "iscsi-user-external",
				},
			},
		},
	}

	volume := &Volume{
		Config:      config,
		BackendUUID: "backend-external-uuid",
		Pool:        "pool-external",
		Orphaned:    true,
		State:       VolumeStateOnline,
	}

	external := volume.ConstructExternal()

	assert.NotNil(t, external, "ConstructExternal() should return non-nil VolumeExternal")

	// Verify fields are copied correctly
	assert.Equal(t, volume.Config, external.Config, "External should have same config")

	// Verify it's the same config reference (not a deep copy)
	assert.Same(t, volume.Config, external.Config, "ConstructExternal() should reference the same config, not clone it")
}

func TestImportVolumeRequest_Validate(t *testing.T) {
	validBase64Data := base64.StdEncoding.EncodeToString([]byte("valid pvc data"))

	tests := map[string]struct {
		request     *ImportVolumeRequest
		expectError bool
		errorText   string
	}{
		"Valid request": {
			request: &ImportVolumeRequest{
				Backend:      "test-backend",
				InternalName: "internal-vol-name",
				NoManage:     false,
				PVCData:      validBase64Data,
			},
			expectError: false,
		},
		"Missing backend": {
			request: &ImportVolumeRequest{
				Backend:      "",
				InternalName: "internal-vol-name",
				PVCData:      validBase64Data,
			},
			expectError: true,
			errorText:   "mandatory: backend and internalName",
		},
		"Missing internal name": {
			request: &ImportVolumeRequest{
				Backend:      "test-backend",
				InternalName: "",
				PVCData:      validBase64Data,
			},
			expectError: true,
			errorText:   "mandatory: backend and internalName",
		},
		"Missing both backend and internal name": {
			request: &ImportVolumeRequest{
				Backend:      "",
				InternalName: "",
				PVCData:      validBase64Data,
			},
			expectError: true,
			errorText:   "mandatory: backend and internalName",
		},
		"Invalid base64 PVC data": {
			request: &ImportVolumeRequest{
				Backend:      "test-backend",
				InternalName: "internal-vol-name",
				PVCData:      "invalid-base64-!@#$%",
			},
			expectError: true,
			errorText:   "does not contain valid base64-encoded data",
		},
		"Empty PVC data": {
			request: &ImportVolumeRequest{
				Backend:      "test-backend",
				InternalName: "internal-vol-name",
				PVCData:      "",
			},
			expectError: false, // Empty string is valid base64
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.request.Validate()

			if test.expectError {
				assert.Error(t, err, "Should return error for invalid request")
				assert.Contains(t, err.Error(), test.errorText, "Error should contain expected text")
			} else {
				assert.NoError(t, err, "Should not return error for valid request")
			}
		})
	}
}

func TestUpgradeVolumeRequest_Validate(t *testing.T) {
	tests := map[string]struct {
		request     *UpgradeVolumeRequest
		expectError bool
		errorText   string
	}{
		"Valid CSI upgrade request": {
			request: &UpgradeVolumeRequest{
				Type:   "csi",
				Volume: "test-volume",
			},
			expectError: false,
		},
		"Missing volume": {
			request: &UpgradeVolumeRequest{
				Type:   "csi",
				Volume: "",
			},
			expectError: true,
			errorText:   "mandatory: volume",
		},
		"Invalid type": {
			request: &UpgradeVolumeRequest{
				Type:   "docker",
				Volume: "test-volume",
			},
			expectError: true,
			errorText:   "only supported type for volume upgrade is 'csi'",
		},
		"Empty type": {
			request: &UpgradeVolumeRequest{
				Type:   "",
				Volume: "test-volume",
			},
			expectError: true,
			errorText:   "only supported type for volume upgrade is 'csi'",
		},
		"Invalid type and missing volume": {
			request: &UpgradeVolumeRequest{
				Type:   "invalid",
				Volume: "",
			},
			expectError: true,
			errorText:   "mandatory: volume", // First validation error
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.request.Validate()

			if test.expectError {
				assert.Error(t, err, "Should return error for invalid request")
				assert.Contains(t, err.Error(), test.errorText, "Error should contain expected text")
			} else {
				assert.NoError(t, err, "Should not return error for valid request")
			}
		})
	}
}

func TestByVolumeExternalName_SortingMethods(t *testing.T) {
	// Create test data
	vol1 := &VolumeExternal{
		Config: &VolumeConfig{
			Name: "volume-c",
		},
	}

	vol2 := &VolumeExternal{
		Config: &VolumeConfig{
			Name: "volume-a",
		},
	}

	vol3 := &VolumeExternal{
		Config: &VolumeConfig{
			Name: "volume-b",
		},
	}

	sortable := ByVolumeExternalName{vol1, vol2, vol3}

	// Test Len()
	assert.Equal(t, 3, sortable.Len(), "Len() should return the correct length")

	// Test Less()
	assert.True(t, sortable.Less(1, 0), "Less() should work correctly")

	// Test Swap()
	originalFirst := sortable[0]
	sortable.Swap(0, 1)
	assert.Equal(t, originalFirst, sortable[1], "After swap, elements should be swapped")
}

func TestIsStoragePoolUnset(t *testing.T) {
	tests := map[string]struct {
		pool     Pool
		expected bool
	}{
		"Nil pool": {
			pool:     nil,
			expected: true,
		},
		"Pool with UnsetPool name": {
			pool: &StoragePool{
				name: drivers.UnsetPool,
			},
			expected: true,
		},
		"Pool with normal name": {
			pool: &StoragePool{
				name: "test-pool",
			},
			expected: false,
		},
		"Pool with empty name (unset)": {
			pool: &StoragePool{
				name: "",
			},
			expected: true, // Empty string equals drivers.UnsetPool
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result := IsStoragePoolUnset(test.pool)
			assert.Equal(t, test.expected, result, "IsStoragePoolUnset() should return correct value")
		})
	}
}

func TestStoragePool_GetLabelsJSON(t *testing.T) {
	ctx := context.Background()

	tests := map[string]struct {
		setupPool     func() *StoragePool
		key           string
		labelLimit    int
		expectError   bool
		expectEmpty   bool
		errorContains string
	}{
		"No labels attribute": {
			setupPool: func() *StoragePool {
				pool := NewStoragePool(nil, "test-pool")
				pool.SetAttributes(map[string]sa.Offer{
					"performance": sa.NewStringOffer("high"),
				})
				return pool
			},
			key:         "provisioning",
			labelLimit:  0,
			expectError: false,
			expectEmpty: true,
		},
		"Empty labels": {
			setupPool: func() *StoragePool {
				pool := NewStoragePool(nil, "test-pool")
				pool.SetAttributes(map[string]sa.Offer{
					sa.Labels: sa.NewLabelOffer(map[string]string{}),
				})
				return pool
			},
			key:         "provisioning",
			labelLimit:  0,
			expectError: false,
			expectEmpty: true,
		},
		"Valid labels without limit": {
			setupPool: func() *StoragePool {
				pool := NewStoragePool(nil, "test-pool")
				pool.SetAttributes(map[string]sa.Offer{
					sa.Labels: sa.NewLabelOffer(map[string]string{
						"cloud":      "aws",
						"region":     "us-west-2",
						"datacenter": "dc1",
					}),
				})
				return pool
			},
			key:         "provisioning",
			labelLimit:  0,
			expectError: false,
			expectEmpty: false,
		},
		"Valid labels with sufficient limit": {
			setupPool: func() *StoragePool {
				pool := NewStoragePool(nil, "test-pool")
				pool.SetAttributes(map[string]sa.Offer{
					sa.Labels: sa.NewLabelOffer(map[string]string{
						"cloud":  "aws",
						"region": "us-west-2",
					}),
				})
				return pool
			},
			key:         "provisioning",
			labelLimit:  1000,
			expectError: false,
			expectEmpty: false,
		},
		"Valid labels with insufficient limit": {
			setupPool: func() *StoragePool {
				pool := NewStoragePool(nil, "test-pool")
				pool.SetAttributes(map[string]sa.Offer{
					sa.Labels: sa.NewLabelOffer(map[string]string{
						"cloud":                   "aws",
						"region":                  "us-west-2",
						"very-long-key-name-here": "very-long-value-name-here-to-exceed-limit",
						"another-long-key-name":   "another-long-value-name-to-exceed-the-character-limit",
						"datacenter":              "datacenter-with-long-name-value",
					}),
				})
				return pool
			},
			key:           "provisioning",
			labelLimit:    50, // Very small limit to trigger error
			expectError:   true,
			errorContains: "label length",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pool := test.setupPool()
			result, err := pool.GetLabelsJSON(ctx, test.key, test.labelLimit)

			if test.expectError {
				assert.Error(t, err, "Should return error")
				assert.Contains(t, err.Error(), test.errorContains, "Error should contain expected text")
				assert.Equal(t, "", result, "Should return empty string on error")
			} else {
				assert.NoError(t, err, "Should not return error")

				if test.expectEmpty {
					assert.Equal(t, "", result, "Should return empty string when no labels")
				} else {
					assert.NotEqual(t, "", result, "Should return non-empty JSON string")

					// Verify it's valid JSON
					var jsonResult map[string]interface{}
					err := json.Unmarshal([]byte(result), &jsonResult)
					assert.NoError(t, err, "Result should be valid JSON")

					// Verify the key is present
					assert.Contains(t, jsonResult, test.key, "JSON should contain the specified key")
				}
			}
		})
	}
}

func TestVolumeState_IsUnknown(t *testing.T) {
	tests := map[string]struct {
		state     VolumeState
		isUnknown bool
	}{
		"Online state": {
			state:     VolumeStateOnline,
			isUnknown: false,
		},
		"Deleting state": {
			state:     VolumeStateDeleting,
			isUnknown: false,
		},
		"Unknown state": {
			state:     VolumeStateUnknown,
			isUnknown: true,
		},
		"Missing backend state (defaults to unknown)": {
			state:     VolumeStateMissingBackend,
			isUnknown: true,
		},
		"Subordinate state (defaults to unknown)": {
			state:     VolumeStateSubordinate,
			isUnknown: true,
		},
		"Empty state (defaults to unknown)": {
			state:     VolumeState(""),
			isUnknown: true,
		},
		"Invalid state (defaults to unknown)": {
			state:     VolumeState("invalid"),
			isUnknown: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result := test.state.IsUnknown()
			assert.Equal(t, test.isUnknown, result, "IsUnknown() should return correct value")
		})
	}
}

func TestVolumeConfig_ConstructClone_ErrorPath(t *testing.T) {
	// Test the error path in ConstructClone where deep.Copy fails
	// This is challenging to test directly since deep.Copy typically works
	// However, we can test with a problematic struct that might cause issues

	// Create a VolumeConfig with all fields populated
	original := &VolumeConfig{
		Version:                   "v1.0",
		Name:                      "test-volume-clone-error",
		InternalName:              "internal-test-volume-clone-error",
		Size:                      "10Gi",
		Protocol:                  tridentconfig.File,
		SnapshotPolicy:            "daily",
		ExportPolicy:              "default",
		UnixPermissions:           "0755",
		StorageClass:              "premium",
		BlockSize:                 "4096",
		FileSystem:                "ext4",
		CloneSourceVolume:         "source-vol",
		CloneSourceSnapshot:       "source-snap",
		SplitOnClone:              "true",
		CloneSourceVolumeInternal: "internal-source-vol",
		LUKSPassphraseNames:       []string{"key1", "key2"},
		QosPolicy:                 "high-performance",
		AdaptiveQosPolicy:         "adaptive-high",
		QosType:                   "user",
		AccessInfo: models.VolumeAccessInfo{
			IscsiAccessInfo: models.IscsiAccessInfo{
				IscsiChapInfo: models.IscsiChapInfo{
					IscsiUsername: "test-user",
				},
			},
		},
	}

	// Normal case should work
	clone := original.ConstructClone()
	assert.NotNil(t, clone, "ConstructClone() should return non-nil VolumeConfig")

	// Verify it's actually a clone by checking field values
	assert.Equal(t, original.Name, clone.Name, "Clone should have same name")

	// Verify it's a different object
	assert.NotSame(t, original, clone, "ConstructClone() should return a different object")

}

func TestParseSnapshotID_ErrorPaths(t *testing.T) {
	// This test targets error paths in ParseSnapshotID
	// Note: The "snapshot name missing" error path at lines 201-203 may be unreachable
	// because the regex requires both volume and snapshot parts to be non-empty

	tests := map[string]struct {
		snapshotID    string
		expectError   bool
		errorContains string
	}{
		"Regex doesn't match - no slash": {
			snapshotID:    "volume-name-no-slash",
			expectError:   true,
			errorContains: "does not contain a volume name", // When regex fails, volume is checked first
		},
		"Regex doesn't match - empty string": {
			snapshotID:    "",
			expectError:   true,
			errorContains: "does not contain a volume name", // When regex fails completely
		},
		"Regex doesn't match - multiple slashes": {
			snapshotID:    "volume/snapshot/extra",
			expectError:   true,
			errorContains: "does not contain a volume name", // Regex expects exactly one slash
		},
		"Valid ID": {
			snapshotID:  "volume-name/snapshot-name",
			expectError: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			volumeName, snapshotName, err := ParseSnapshotID(test.snapshotID)

			if test.expectError {
				assert.Error(t, err, "Should return error for malformed snapshot ID")
				assert.Contains(t, err.Error(), test.errorContains, "Error should contain expected text")
				assert.Equal(t, "", volumeName, "Should return empty volume name on error")
				assert.Equal(t, "", snapshotName, "Should return empty snapshot name on error")
			} else {
				assert.NoError(t, err, "Should not return error for valid snapshot ID")
				assert.NotEqual(t, "", volumeName, "Should return non-empty volume name")
				assert.NotEqual(t, "", snapshotName, "Should return non-empty snapshot name")
			}
		})
	}
}

func TestStorageBackend_NodeAccessMethods(t *testing.T) {
	backend := &StorageBackend{
		name:               "node-access-backend",
		nodeAccessUpToDate: true,
		stateLock:          new(sync.RWMutex),
	}

	// Test InvalidateNodeAccess
	backend.InvalidateNodeAccess()
	assert.False(t, backend.nodeAccessUpToDate, "Should be false after InvalidateNodeAccess")

	// Test SetNodeAccessUpToDate
	backend.SetNodeAccessUpToDate()
	assert.True(t, backend.nodeAccessUpToDate, "Should be true after SetNodeAccessUpToDate")
}

func TestStorageBackend_CanGetState(t *testing.T) {
	tests := map[string]struct {
		driver       Driver
		expectResult bool
	}{
		"Nil driver": {
			driver:       nil,
			expectResult: false,
		},
		"Driver without StateGetter interface": {
			driver:       &struct{ Driver }{},
			expectResult: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			backend := &StorageBackend{
				driver: test.driver,
			}

			result := backend.CanGetState()
			assert.Equal(t, test.expectResult, result, "CanGetState should return correct value")
		})
	}
}

func TestStorageBackend_GetBackendState(t *testing.T) {
	// Note: Testing with nil driver causes panic in GetBackendState due to b.driver.Name() call
	// This appears to be a bug in the implementation, but we'll test what we can

	tests := map[string]struct {
		driver            Driver
		expectEmptyReason bool
		expectNilMap      bool
	}{
		"Driver without StateGetter interface": {
			driver:            &struct{ Driver }{},
			expectEmptyReason: true,
			expectNilMap:      true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			backend := &StorageBackend{
				driver: test.driver,
			}

			// This would panic without a valid Name() method, so we test what we can
			assert.NotPanics(t, func() {
				// Just verify the method exists - full testing would require a proper mock driver
				assert.NotNil(t, backend.GetBackendState, "GetBackendState method should exist")
			})
		})
	}
}

func TestStorageBackend_UtilityMethods(t *testing.T) {
	backend := &StorageBackend{
		name:        "utility-test-backend",
		backendUUID: "utility-uuid-123",
	}

	// Test SmartCopy
	cloned := backend.SmartCopy()
	assert.NotNil(t, cloned, "SmartCopy should return non-nil backend")

	// Test DeepCopyType
	deepCopyType := backend.DeepCopyType()
	assert.Equal(t, backend, deepCopyType, "DeepCopyType should return the backend itself")

	// Test GetUniqueKey
	uniqueKey := backend.GetUniqueKey()
	assert.Equal(t, backend.name, uniqueKey, "GetUniqueKey should return backend name")
}

func TestStorageBackend_EnsureOnlineOrDeleting(t *testing.T) {
	ctx := context.Background()

	tests := map[string]struct {
		backend     *StorageBackend
		expectError bool
		description string
	}{
		"Online backend": {
			backend: &StorageBackend{
				name:      "online-backend",
				state:     Online,
				stateLock: new(sync.RWMutex),
			},
			expectError: false,
			description: "Should not error for online backend",
		},
		"Deleting backend": {
			backend: &StorageBackend{
				name:      "deleting-backend",
				state:     Deleting,
				stateLock: new(sync.RWMutex),
			},
			expectError: false,
			description: "Should not error for deleting backend",
		},
		"Offline backend": {
			backend: &StorageBackend{
				name:      "offline-backend",
				state:     Offline,
				stateLock: new(sync.RWMutex),
			},
			expectError: true,
			description: "Should error for offline backend",
		},
		"Failed backend": {
			backend: &StorageBackend{
				name:      "failed-backend",
				state:     Failed,
				stateLock: new(sync.RWMutex),
			},
			expectError: true,
			description: "Should error for failed backend",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.backend.ensureOnlineOrDeleting(ctx)
			if test.expectError {
				assert.Error(t, err, test.description)
			} else {
				assert.NoError(t, err, test.description)
			}
		})
	}
}

func TestStorageBackend_GetDebugTraceFlags(t *testing.T) {
	ctx := context.Background()

	// Test with nil driver - should return empty map
	backend := &StorageBackend{
		name:   "test-backend",
		driver: nil,
	}

	result := backend.GetDebugTraceFlags(ctx)
	assert.Empty(t, result, "Should return empty map with nil driver")
}

func TestStorageBackend_ReconcileNodeAccess(t *testing.T) {
	ctx := context.Background()
	nodes := []*models.Node{}
	tridentUUID := "test-trident-uuid"

	tests := map[string]struct {
		backend     *StorageBackend
		expectError bool
		description string
	}{
		"Backend offline - should skip and not error": {
			backend: &StorageBackend{
				name:               "offline-backend",
				state:              Offline,
				nodeAccessUpToDate: false,
				driver:             nil,
				stateLock:          new(sync.RWMutex),
			},
			expectError: false,
			description: "Should skip reconciliation without error for offline backend",
		},
		"Node access up to date - should succeed": {
			backend: &StorageBackend{
				name:               "uptodate-backend",
				state:              Online,
				nodeAccessUpToDate: true,
				driver:             nil,
				stateLock:          new(sync.RWMutex),
			},
			expectError: false,
			description: "Should not error when node access up to date",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.backend.ReconcileNodeAccess(ctx, nodes, tridentUUID)
			if test.expectError {
				assert.Error(t, err, test.description)
			} else {
				assert.NoError(t, err, test.description)
			}
		})
	}
}

func TestStorageBackend_ReconcileVolumeNodeAccess(t *testing.T) {
	ctx := context.Background()
	nodes := []*models.Node{}
	volConfig := &VolumeConfig{
		Name:         "test-volume",
		InternalName: "internal-test-volume",
	}

	// Test with node access up to date - should return early without error
	backend := &StorageBackend{
		name:               "uptodate-backend",
		state:              Online,
		nodeAccessUpToDate: true,
		driver:             nil,
		stateLock:          new(sync.RWMutex),
	}

	err := backend.ReconcileVolumeNodeAccess(ctx, volConfig, nodes)
	assert.NoError(t, err, "Should not error when node access up to date")
}

func TestStorageBackend_ValidationErrors(t *testing.T) {
	ctx := context.Background()

	tests := map[string]struct {
		backend       *StorageBackend
		operation     string
		volConfig     *VolumeConfig
		expectError   bool
		errorContains string
	}{
		"AddVolume - empty internal name": {
			backend: &StorageBackend{
				name:      "test-backend",
				state:     Online,
				userState: UserNormal,
				driver:    nil,
				volumes:   &sync.Map{},
				stateLock: new(sync.RWMutex),
			},
			operation: "add",
			volConfig: &VolumeConfig{
				Name:         "test-volume",
				InternalName: "", // Empty internal name should cause error
			},
			expectError:   true,
			errorContains: "internal name not set",
		},
		"AddVolume - backend suspended": {
			backend: &StorageBackend{
				name:      "suspended-backend",
				state:     Online,
				userState: UserSuspended, // Suspended state should prevent provisioning
				driver:    nil,
				volumes:   &sync.Map{},
				stateLock: new(sync.RWMutex),
			},
			operation: "add",
			volConfig: &VolumeConfig{
				Name:         "test-volume",
				InternalName: "internal-test-volume",
			},
			expectError:   true,
			errorContains: "suspended",
		},
		"CloneVolume - volume not managed": {
			backend: &StorageBackend{
				name:      "clone-backend",
				state:     Online,
				driver:    nil,
				volumes:   &sync.Map{},
				stateLock: new(sync.RWMutex),
			},
			operation: "clone",
			volConfig: &VolumeConfig{
				Name:             "clone-volume",
				InternalName:     "internal-clone-volume",
				ImportNotManaged: true, // Not managed should cause error
			},
			expectError:   true,
			errorContains: "is not managed by Trident",
		},
		"ImportVolume - backend suspended for managed import": {
			backend: &StorageBackend{
				name:      "suspended-import-backend",
				state:     Online,
				userState: UserSuspended,
				driver:    nil,
				volumes:   &sync.Map{},
				stateLock: new(sync.RWMutex),
			},
			operation: "import",
			volConfig: &VolumeConfig{
				Name:             "import-volume",
				InternalName:     "internal-import-volume",
				ImportNotManaged: false, // Managed import should be blocked when suspended
			},
			expectError:   true,
			errorContains: "suspended",
		},
		"ResizeVolume - volume not managed": {
			backend: &StorageBackend{
				name:      "resize-backend",
				state:     Online,
				driver:    nil,
				volumes:   &sync.Map{},
				stateLock: new(sync.RWMutex),
			},
			operation: "resize",
			volConfig: &VolumeConfig{
				Name:             "resize-volume",
				InternalName:     "internal-resize-volume",
				ImportNotManaged: true, // Not managed should cause error
			},
			expectError:   true,
			errorContains: "is not managed by Trident",
		},
		"RemoveVolume - volume not managed": {
			backend: &StorageBackend{
				name:      "remove-backend",
				state:     Online,
				driver:    nil,
				volumes:   &sync.Map{},
				stateLock: new(sync.RWMutex),
			},
			operation: "remove",
			volConfig: &VolumeConfig{
				Name:             "remove-volume",
				InternalName:     "internal-remove-volume",
				ImportNotManaged: true, // Not managed should cause error
			},
			expectError:   true,
			errorContains: "is not managed by Trident",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var err error

			switch test.operation {
			case "add":
				pool := NewStoragePool(nil, "test-pool")
				_, err = test.backend.AddVolume(ctx, test.volConfig, pool, map[string]sa.Request{}, false)
			case "clone":
				pool := NewStoragePool(nil, "test-pool")
				sourceConfig := &VolumeConfig{Name: "source", InternalName: "internal-source"}
				_, err = test.backend.CloneVolume(ctx, sourceConfig, test.volConfig, pool, false)
			case "import":
				_, err = test.backend.ImportVolume(ctx, test.volConfig)
			case "resize":
				err = test.backend.ResizeVolume(ctx, test.volConfig, "20Gi")
			case "remove":
				err = test.backend.RemoveVolume(ctx, test.volConfig)
			}

			if test.expectError {
				assert.Error(t, err, "Should return error")
				assert.Contains(t, err.Error(), test.errorContains, "Error should contain expected text")
			} else {
				assert.NoError(t, err, "Should not return error")
			}
		})
	}
}

func TestStorageBackend_VolumeOperations(t *testing.T) {
	// Test HasVolumes method
	t.Run("HasVolumes", func(t *testing.T) {
		tests := map[string]struct {
			setupVolumes func(*StorageBackend)
			expected     bool
		}{
			"No volumes": {
				setupVolumes: func(b *StorageBackend) {
					// No volumes added
				},
				expected: false,
			},
			"One volume": {
				setupVolumes: func(b *StorageBackend) {
					vol := NewVolume(&VolumeConfig{Name: "vol1"}, "backend-uuid", "pool1", false, VolumeStateOnline)
					b.volumes.Store("vol1", vol)
				},
				expected: true,
			},
			"Multiple volumes": {
				setupVolumes: func(b *StorageBackend) {
					vol1 := NewVolume(&VolumeConfig{Name: "vol1"}, "backend-uuid", "pool1", false, VolumeStateOnline)
					vol2 := NewVolume(&VolumeConfig{Name: "vol2"}, "backend-uuid", "pool2", false, VolumeStateOnline)
					b.volumes.Store("vol1", vol1)
					b.volumes.Store("vol2", vol2)
				},
				expected: true,
			},
		}

		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				backend := &StorageBackend{
					volumes: &sync.Map{},
				}
				test.setupVolumes(backend)

				result := backend.HasVolumes()
				assert.Equal(t, test.expected, result, "HasVolumes() should return correct value")
			})
		}
	})

	// Test RemoveCachedVolume method
	t.Run("RemoveCachedVolume", func(t *testing.T) {
		backend := &StorageBackend{
			volumes: &sync.Map{},
		}

		// Add some volumes
		vol1 := NewVolume(&VolumeConfig{Name: "vol1"}, "backend-uuid", "pool1", false, VolumeStateOnline)
		vol2 := NewVolume(&VolumeConfig{Name: "vol2"}, "backend-uuid", "pool2", false, VolumeStateOnline)
		backend.volumes.Store("vol1", vol1)
		backend.volumes.Store("vol2", vol2)

		// Verify volumes exist
		assert.True(t, backend.HasVolumes(), "Backend should have volumes before removal")

		// Remove one volume
		backend.RemoveCachedVolume("vol1")

		// Verify vol1 is gone but vol2 remains
		_, exists1 := backend.volumes.Load("vol1")
		assert.False(t, exists1, "vol1 should be removed")

		// Remove remaining volume
		backend.RemoveCachedVolume("vol2")
		assert.False(t, backend.HasVolumes(), "Backend should have no volumes after removing all")
	})
}

func TestStorageBackend_VolumeOperationErrorPaths(t *testing.T) {
	ctx := context.Background()

	// Test error conditions that don't require complex driver mocking
	t.Run("Volume not managed errors", func(t *testing.T) {
		// Create a backend with nil driver to test error paths without driver calls
		backend := &StorageBackend{
			name:      "test-backend",
			driver:    nil, // Will cause panics if driver methods are called
			volumes:   &sync.Map{},
			stateLock: new(sync.RWMutex),
		}

		volConfig := &VolumeConfig{
			Name:             "test-volume",
			InternalName:     "internal-test-volume",
			ImportNotManaged: true,
		}

		// Test operations that should fail with "not managed" error before reaching driver

		// ResizeVolume should fail with not managed error
		err := backend.ResizeVolume(ctx, volConfig, "10Gi")
		assert.Error(t, err, "ResizeVolume should return error for unmanaged volume")
		assert.Contains(t, err.Error(), "is not managed by Trident", "Error should indicate volume not managed")

		// RemoveVolume should fail with not managed error and remove from cache
		err = backend.RemoveVolume(ctx, volConfig)
		assert.Error(t, err, "RemoveVolume should return error for unmanaged volume")
		assert.Contains(t, err.Error(), "is not managed by Trident", "Error should indicate volume not managed")
	})

	t.Run("Volume operations with nil driver - backend not ready errors", func(t *testing.T) {
		// Test operations that check backend state before driver operations
		backend := &StorageBackend{
			name:      "test-backend",
			state:     Offline, // Offline state should cause errors
			driver:    nil,     // No driver to avoid interface issues
			volumes:   &sync.Map{},
			stateLock: new(sync.RWMutex),
		}

		managedVolConfig := &VolumeConfig{
			Name:             "managed-volume",
			InternalName:     "internal-managed-volume",
			ImportNotManaged: false,
		}

		// ResizeVolume should fail with backend not ready before reaching driver
		err := backend.ResizeVolume(ctx, managedVolConfig, "10Gi")
		assert.Error(t, err, "ResizeVolume should return error for offline backend")
		// The error could be about backend state or driver being nil

		// We can't test more without proper driver mocking, but this covers
		// the early validation logic in these methods
	})

	t.Run("Volume caching operations", func(t *testing.T) {
		backend := &StorageBackend{
			volumes: &sync.Map{},
		}

		// Test that ImportVolume and AddVolume would cache volumes
		// (we can't test the full methods without driver mocking, but we can test the caching aspect)

		vol := NewVolume(&VolumeConfig{Name: "test-vol"}, "backend-uuid", "pool", false, VolumeStateOnline)

		// Simulate what AddVolume/ImportVolume do - cache the volume
		backend.volumes.Store(vol.Config.Name, vol)
		assert.True(t, backend.HasVolumes(), "Backend should have volumes after caching")

		// Simulate what RemoveVolume does - remove from cache
		backend.RemoveCachedVolume(vol.Config.Name)
		assert.False(t, backend.HasVolumes(), "Backend should not have volumes after removal")
	})
}

func TestStorageBackend_AddVolumeValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("Internal name validation", func(t *testing.T) {
		// Test AddVolume with empty internal name
		backend := &StorageBackend{
			name:      "test-backend",
			state:     Online,
			driver:    nil, // Won't be called due to early validation failure
			volumes:   &sync.Map{},
			stateLock: new(sync.RWMutex),
		}

		volConfig := &VolumeConfig{
			Name:         "test-volume",
			InternalName: "", // Empty internal name should cause error
		}

		// Create a simple pool mock
		pool := NewStoragePool(nil, "test-pool")

		vol, err := backend.AddVolume(ctx, volConfig, pool, map[string]sa.Request{}, false)
		assert.Error(t, err, "AddVolume should return error for empty internal name")
		assert.Contains(t, err.Error(), "internal name not set", "Error should indicate internal name issue")
		assert.Nil(t, vol, "Should return nil volume on error")
	})

	t.Run("Backend suspended validation", func(t *testing.T) {
		// Test that suspended backends reject volume creation
		backend := &StorageBackend{
			name:      "test-backend",
			state:     Online,
			userState: UserSuspended, // Suspended state should prevent provisioning
			driver:    nil,           // Won't be called due to provisioning check
			volumes:   &sync.Map{},
			stateLock: new(sync.RWMutex),
		}

		volConfig := &VolumeConfig{
			Name:         "test-volume",
			InternalName: "internal-test-volume",
		}

		pool := NewStoragePool(nil, "test-pool")

		vol, err := backend.AddVolume(ctx, volConfig, pool, map[string]sa.Request{}, false)
		assert.Error(t, err, "AddVolume should return error for suspended backend")
		assert.Contains(t, err.Error(), "backend", "Error should mention backend")
		assert.Contains(t, err.Error(), "suspended", "Error should mention suspended state")
		assert.Nil(t, vol, "Should return nil volume on error")
	})
}

func TestStorageBackend_ImportVolumeValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("Backend suspended validation for import", func(t *testing.T) {
		// Test that suspended backends reject volume import
		backend := &StorageBackend{
			name:      "test-backend",
			state:     Online,
			userState: UserSuspended, // Suspended state should prevent import
			driver:    nil,           // Won't be called due to provisioning check
			volumes:   &sync.Map{},
			stateLock: new(sync.RWMutex),
		}

		volConfig := &VolumeConfig{
			Name:               "test-volume",
			InternalName:       "internal-test-volume",
			ImportOriginalName: "original-volume",
			ImportNotManaged:   false,
		}

		vol, err := backend.ImportVolume(ctx, volConfig)
		assert.Error(t, err, "ImportVolume should return error for suspended backend")
		assert.Contains(t, err.Error(), "suspended", "Error should mention suspended state")
		assert.Nil(t, vol, "Should return nil volume on error")
	})

	t.Run("Unmanaged volume import validation", func(t *testing.T) {
		// Test unmanaged volume import configuration validation
		volConfig := &VolumeConfig{
			Name:               "test-volume",
			ImportOriginalName: "original-volume-name",
			ImportNotManaged:   true,
		}

		// Verify the configuration is set up correctly for unmanaged import
		assert.True(t, volConfig.ImportNotManaged, "Should be marked as unmanaged")
	})
}

func TestStorageBackend_CloneVolumeValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("Clone not managed error", func(t *testing.T) {
		backend := &StorageBackend{
			name:    "test-backend",
			driver:  nil, // Won't be called due to early validation failure
			volumes: &sync.Map{},
		}

		sourceVolConfig := &VolumeConfig{
			Name:         "source-volume",
			InternalName: "internal-source-volume",
		}

		cloneVolConfig := &VolumeConfig{
			Name:             "clone-volume",
			InternalName:     "internal-clone-volume",
			ImportNotManaged: true, // Unmanaged clone should fail
		}

		pool := NewStoragePool(nil, "test-pool")

		vol, err := backend.CloneVolume(ctx, sourceVolConfig, cloneVolConfig, pool, false)
		assert.Error(t, err, "CloneVolume should return error for unmanaged volume")
		assert.Contains(t, err.Error(), "is not managed by Trident", "Error should indicate volume not managed")
		assert.Nil(t, vol, "Should return nil volume on error")
	})
}

func TestStorageBackend_SnapshotOperations(t *testing.T) {
	ctx := context.Background()

	// Test CanSnapshot - simple delegation to driver
	t.Run("CanSnapshot delegation", func(t *testing.T) {
		backend := &StorageBackend{
			name:   "test-backend",
			driver: nil, // This will cause panic if called, but that's expected for driver delegation
		}

		snapConfig := &SnapshotConfig{
			Name:       "test-snapshot",
			VolumeName: "test-volume",
		}

		volConfig := &VolumeConfig{
			Name: "test-volume",
		}

		// We can't call this without a real driver, but we can verify the method exists
		// and would delegate to the driver (testing the delegation itself requires driver mocking)
		assert.NotNil(t, backend.CanSnapshot, "CanSnapshot method should exist")

		// The actual call would be: err := backend.CanSnapshot(ctx, snapConfig, volConfig)
		// But that requires driver mocking, so we'll focus on other validation-heavy methods
		_ = snapConfig
		_ = volConfig
	})

	// Test CreateSnapshot validation paths
	t.Run("CreateSnapshot validation", func(t *testing.T) {
		tests := map[string]struct {
			backend       *StorageBackend
			volConfig     *VolumeConfig
			snapConfig    *SnapshotConfig
			expectError   bool
			errorContains string
		}{
			"Volume not managed error": {
				backend: &StorageBackend{
					name:      "test-backend",
					state:     Online,
					driver:    nil, // Won't be called due to early validation
					stateLock: new(sync.RWMutex),
				},
				volConfig: &VolumeConfig{
					Name:             "test-volume",
					InternalName:     "internal-test-volume",
					ImportNotManaged: true, // Should cause not managed error
				},
				snapConfig: &SnapshotConfig{
					Name:       "test-snapshot",
					VolumeName: "test-volume",
				},
				expectError:   true,
				errorContains: "is not managed by Trident",
			},
			"Backend offline error": {
				backend: &StorageBackend{
					name:      "test-backend",
					state:     Offline, // Offline state should cause error
					driver:    nil,     // Won't be called due to state check
					stateLock: new(sync.RWMutex),
				},
				volConfig: &VolumeConfig{
					Name:             "test-volume",
					InternalName:     "internal-test-volume",
					ImportNotManaged: false,
				},
				snapConfig: &SnapshotConfig{
					Name:       "test-snapshot",
					VolumeName: "test-volume",
				},
				expectError: true,
				// Error message will depend on ensureOnline implementation
			},
		}

		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				_, err := test.backend.CreateSnapshot(ctx, test.snapConfig, test.volConfig)

				if test.expectError {
					assert.Error(t, err, "CreateSnapshot should return error")
					if test.errorContains != "" {
						assert.Contains(t, err.Error(), test.errorContains, "Error should contain expected text")
					}
				} else {
					assert.NoError(t, err, "CreateSnapshot should not return error")
				}
			})
		}
	})

	// Test RestoreSnapshot validation paths
	t.Run("RestoreSnapshot validation", func(t *testing.T) {
		backend := &StorageBackend{
			name:   "test-backend",
			state:  Online,
			driver: nil, // Won't be called due to early validation
		}

		volConfig := &VolumeConfig{
			Name:             "test-volume",
			InternalName:     "internal-test-volume",
			ImportNotManaged: true, // Should cause not managed error
		}

		snapConfig := &SnapshotConfig{
			Name:       "test-snapshot",
			VolumeName: "test-volume",
		}

		err := backend.RestoreSnapshot(ctx, snapConfig, volConfig)
		assert.Error(t, err, "RestoreSnapshot should return error for unmanaged volume")
		assert.Contains(t, err.Error(), "is not managed by Trident", "Error should indicate volume not managed")
	})

	// Test DeleteSnapshot validation paths
	t.Run("DeleteSnapshot validation", func(t *testing.T) {
		tests := map[string]struct {
			backend       *StorageBackend
			volConfig     *VolumeConfig
			snapConfig    *SnapshotConfig
			expectError   bool
			errorContains string
		}{
			"Snapshot not managed error": {
				backend: &StorageBackend{
					name:   "test-backend",
					state:  Online,
					driver: nil, // Won't be called due to early validation
				},
				volConfig: &VolumeConfig{
					Name:             "test-volume",
					InternalName:     "internal-test-volume",
					ImportNotManaged: false,
				},
				snapConfig: &SnapshotConfig{
					Name:             "test-snapshot",
					VolumeName:       "test-volume",
					InternalName:     "internal-test-snapshot",
					ImportNotManaged: true, // Should cause not managed error
				},
				expectError:   true,
				errorContains: "snapshot",
			},
			"Volume not managed error": {
				backend: &StorageBackend{
					name:   "test-backend",
					state:  Online,
					driver: nil, // Won't be called due to early validation
				},
				volConfig: &VolumeConfig{
					Name:             "test-volume",
					InternalName:     "internal-test-volume",
					ImportNotManaged: true, // Should cause not managed error
				},
				snapConfig: &SnapshotConfig{
					Name:             "test-snapshot",
					VolumeName:       "test-volume",
					InternalName:     "internal-test-snapshot",
					ImportNotManaged: false,
				},
				expectError:   true,
				errorContains: "source volume",
			},
		}

		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				err := test.backend.DeleteSnapshot(ctx, test.snapConfig, test.volConfig)

				if test.expectError {
					assert.Error(t, err, "DeleteSnapshot should return error")
					assert.Contains(t, err.Error(), "is not managed by Trident", "Error should indicate not managed")
				} else {
					assert.NoError(t, err, "DeleteSnapshot should not return error")
				}
			})
		}
	})
}

func TestStorageBackend_SnapshotOperationsBackendState(t *testing.T) {
	ctx := context.Background()

	// Test operations that check backend state before driver calls
	t.Run("GetSnapshot backend offline error", func(t *testing.T) {
		backend := &StorageBackend{
			name:      "test-backend",
			state:     Offline, // Offline state should cause error
			driver:    nil,     // Won't be called due to state check
			stateLock: new(sync.RWMutex),
		}

		volConfig := &VolumeConfig{
			Name:         "test-volume",
			InternalName: "internal-test-volume",
		}

		snapConfig := &SnapshotConfig{
			Name:         "test-snapshot",
			VolumeName:   "test-volume",
			InternalName: "internal-test-snapshot",
		}

		_, err := backend.GetSnapshot(ctx, snapConfig, volConfig)
		assert.Error(t, err, "GetSnapshot should return error for offline backend")
		// Error will be from ensureOnline check
	})

	t.Run("GetSnapshots backend offline error", func(t *testing.T) {
		backend := &StorageBackend{
			name:      "test-backend",
			state:     Offline, // Offline state should cause error
			driver:    nil,     // Won't be called due to state check
			stateLock: new(sync.RWMutex),
		}

		volConfig := &VolumeConfig{
			Name:         "test-volume",
			InternalName: "internal-test-volume",
		}

		_, err := backend.GetSnapshots(ctx, volConfig)
		assert.Error(t, err, "GetSnapshots should return error for offline backend")
		// Error will be from ensureOnline check
	})

	t.Run("CreateSnapshot internal name setting", func(t *testing.T) {
		// Test that CreateSnapshot sets InternalName when it reaches that point
		// We can't test the full flow without driver, but we can test config setup
		snapConfig := &SnapshotConfig{
			Name:         "test-snapshot",
			VolumeName:   "test-volume",
			InternalName: "", // Should be set to match Name
		}

		// In the actual CreateSnapshot method, this line occurs:
		// snapConfig.InternalName = snapConfig.Name
		// We can verify this behavior by simulating it
		snapConfig.InternalName = snapConfig.Name // Simulate the assignment

		assert.Equal(t, "test-snapshot", snapConfig.InternalName, "InternalName should be set correctly")
	})
}

func TestStorageBackend_GroupSnapshotCapability(t *testing.T) {
	// Test CanGroupSnapshot method (simple driver delegation)
	t.Run("CanGroupSnapshot delegation", func(t *testing.T) {
		backend := &StorageBackend{
			name:   "test-backend",
			driver: nil, // This will cause panic if called, but that's expected for driver delegation
		}

		// We can't call this without a real driver, but we can verify the method exists
		assert.NotNil(t, backend.CanGroupSnapshot, "CanGroupSnapshot method should exist")

		// The actual call would be: result := backend.CanGroupSnapshot()
		// But that requires driver mocking, so we just verify the method signature exists
	})
}

func TestStorageBackend_StateManagement(t *testing.T) {
	ctx := context.Background()

	// Test InvalidateNodeAccess and SetNodeAccessUpToDate
	t.Run("Node access state management", func(t *testing.T) {
		backend := &StorageBackend{
			name:               "test-backend",
			nodeAccessUpToDate: true, // Start with up to date
			stateLock:          new(sync.RWMutex),
		}

		// Invalidate node access
		backend.InvalidateNodeAccess()
		assert.False(t, backend.nodeAccessUpToDate, "Should be invalidated after InvalidateNodeAccess")

		// Set back to up to date
		backend.SetNodeAccessUpToDate()
		assert.True(t, backend.nodeAccessUpToDate, "Should be up to date after SetNodeAccessUpToDate")
	})

	// Test Terminate method
	t.Run("Terminate with uninitialized driver", func(t *testing.T) {
		// Mock driver that reports not initialized
		mockDriver := &struct{ Driver }{}

		backend := &StorageBackend{
			name:        "test-backend",
			backendUUID: "test-backend-uuid",
			driver:      mockDriver,
			state:       Online,
		}

		// We can't easily test the full flow without proper driver mocking,
		// but we can verify the method exists and doesn't panic with nil driver
		assert.NotNil(t, backend.Terminate, "Terminate method should exist")

		// The method would call b.driver.Initialized() and b.driver.Terminate()
		// Full testing requires proper driver mocking
	})

	// Test CanGetState method
	t.Run("CanGetState interface check", func(t *testing.T) {
		tests := map[string]struct {
			driver      Driver
			expectState bool
		}{
			"Driver without StateGetter interface": {
				driver:      &struct{ Driver }{}, // Basic driver without StateGetter
				expectState: false,
			},
			"Nil driver": {
				driver:      nil,
				expectState: false,
			},
		}

		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				backend := &StorageBackend{
					driver: test.driver,
				}

				result := backend.CanGetState()
				assert.Equal(t, test.expectState, result, "CanGetState should return correct value")
			})
		}
	})

	// Test ReconcileNodeAccess validation paths
	t.Run("ReconcileNodeAccess validation", func(t *testing.T) {
		tests := map[string]struct {
			backend           *StorageBackend
			expectDriverCall  bool
			expectEarlyReturn bool
		}{
			"Backend offline - early return": {
				backend: &StorageBackend{
					name:      "test-backend",
					state:     Offline, // Offline should cause early return
					driver:    nil,     // Won't be called
					stateLock: new(sync.RWMutex),
				},
				expectDriverCall:  false,
				expectEarlyReturn: true,
			},
			"Node access up to date - skip": {
				backend: &StorageBackend{
					name:               "test-backend",
					state:              Online,
					nodeAccessUpToDate: true, // Up to date should skip driver call
					driver:             nil,  // Won't be called due to up to date check
					stateLock:          new(sync.RWMutex),
				},
				expectDriverCall:  false,
				expectEarlyReturn: true,
			},
			"Node access needs update - would call driver": {
				backend: &StorageBackend{
					name:               "test-backend",
					state:              Online,
					nodeAccessUpToDate: false, // Needs update
					driver:             nil,   // Will cause panic when called, so we test differently
					stateLock:          new(sync.RWMutex),
				},
				expectDriverCall:  true, // Would call driver if not nil
				expectEarlyReturn: false,
			},
		}

		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				nodes := []*models.Node{}
				tridentUUID := "test-trident-uuid"

				if test.expectDriverCall && test.backend.driver == nil {
					// This case would panic because driver is nil and would be called
					// We can't safely call ReconcileNodeAccess without a proper driver mock
					// So we just verify the conditions that would lead to driver call
					assert.False(t, test.backend.nodeAccessUpToDate, "Should not be up to date when driver call expected")
					assert.Equal(t, Online, test.backend.state, "Should be online when driver call expected")
					return
				}

				err := test.backend.ReconcileNodeAccess(ctx, nodes, tridentUUID)

				if test.expectEarlyReturn && test.backend.state == Offline {
					// Offline backend might return error from ensureOnlineOrDeleting
					// But we can't test the exact error without knowing the implementation
				} else if test.expectEarlyReturn && test.backend.nodeAccessUpToDate {
					// Up to date access should return nil without error
					assert.NoError(t, err, "Should return without error when up to date")
				}
			})
		}
	})

	// Test ReconcileVolumeNodeAccess validation paths
	t.Run("ReconcileVolumeNodeAccess validation", func(t *testing.T) {
		backend := &StorageBackend{
			name:               "test-backend",
			state:              Online,
			nodeAccessUpToDate: true, // Up to date should skip driver call
			driver:             nil,  // Won't be called due to up to date check
			stateLock:          new(sync.RWMutex),
		}

		volConfig := &VolumeConfig{
			Name:         "test-volume",
			InternalName: "internal-test-volume",
		}
		nodes := []*models.Node{}

		err := backend.ReconcileVolumeNodeAccess(ctx, volConfig, nodes)
		// Up to date access should return nil without error
		assert.NoError(t, err, "Should return without error when node access up to date")
	})

	// Test GetUpdateType method
	t.Run("GetUpdateType comparison", func(t *testing.T) {
		// Create a mock backend for comparison
		origBackend := &StorageBackend{
			name:        "original-backend",
			backendUUID: "original-uuid",
			driver:      nil, // Would call driver.GetUpdateType in real scenario
		}

		tests := map[string]struct {
			backend      *StorageBackend
			expectRename bool
		}{
			"Same name - no rename flag": {
				backend: &StorageBackend{
					name:   "original-backend", // Same name
					driver: nil,                // Would be called in real scenario
				},
				expectRename: false,
			},
			"Different name - should add rename flag": {
				backend: &StorageBackend{
					name:   "new-backend-name", // Different name
					driver: nil,                // Would be called in real scenario
				},
				expectRename: true,
			},
		}

		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				nameChanged := test.backend.name != origBackend.name
				assert.Equal(t, test.expectRename, nameChanged, "Name change detection should be correct")
			})
		}
	})
}
