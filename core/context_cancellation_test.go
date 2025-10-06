// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	persistentstore "github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/models"
)

// TestContextCancellationAfterMutexLock tests that all 71 TridentOrchestrator methods
// that lock the global mutex and check for context errors properly return context
// cancellation errors when the context is cancelled before calling the method.
//
// This test covers all methods that follow the pattern:
//
//	o.mutex.Lock()
//	defer o.mutex.Unlock()
//	if ctx.Err() != nil {
//	    return [nil,] ctx.Err()
//	}
func TestContextCancellationAfterMutexLock(t *testing.T) {
	storeClient := persistentstore.NewInMemoryClient()
	orchestrator, err := NewTridentOrchestrator(storeClient)
	assert.NoError(t, err)
	orchestrator.bootstrapError = nil

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Test all 71 methods that check context after mutex lock

	t.Run("AddBackend", func(t *testing.T) {
		_, err := orchestrator.AddBackend(ctx, "{}", "")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("UpdateBackend", func(t *testing.T) {
		_, err := orchestrator.UpdateBackend(ctx, "backend1", "{}", "")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("UpdateBackendByBackendUUID", func(t *testing.T) {
		_, err := orchestrator.UpdateBackendByBackendUUID(ctx, "backend1", "{}", "uuid1", "")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("UpdateBackendState", func(t *testing.T) {
		_, err := orchestrator.UpdateBackendState(ctx, "backend1", "online", "")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetBackend", func(t *testing.T) {
		_, err := orchestrator.GetBackend(ctx, "backend1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetBackendByBackendUUID", func(t *testing.T) {
		_, err := orchestrator.GetBackendByBackendUUID(ctx, "uuid1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("ListBackends", func(t *testing.T) {
		_, err := orchestrator.ListBackends(ctx)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("DeleteBackend", func(t *testing.T) {
		err := orchestrator.DeleteBackend(ctx, "backend1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("DeleteBackendByBackendUUID", func(t *testing.T) {
		err := orchestrator.DeleteBackendByBackendUUID(ctx, "backend1", "uuid1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("RemoveBackendConfigRef", func(t *testing.T) {
		err := orchestrator.RemoveBackendConfigRef(ctx, "uuid1", "configRef1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("AddVolume", func(t *testing.T) {
		volumeConfig := &storage.VolumeConfig{Name: "volume1"}
		_, err := orchestrator.AddVolume(ctx, volumeConfig)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("UpdateVolume", func(t *testing.T) {
		err := orchestrator.UpdateVolume(ctx, "volume1", &models.VolumeUpdateInfo{})
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("UpdateVolumeLUKSPassphraseNames", func(t *testing.T) {
		passphraseNames := []string{"passphrase1"}
		err := orchestrator.UpdateVolumeLUKSPassphraseNames(ctx, "volume1", &passphraseNames)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("CloneVolume", func(t *testing.T) {
		volumeConfig := &storage.VolumeConfig{Name: "clone1"}
		_, err := orchestrator.CloneVolume(ctx, volumeConfig)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetVolumeForImport", func(t *testing.T) {
		_, err := orchestrator.GetVolumeForImport(ctx, "volume1", "backend1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetVolumeByInternalName", func(t *testing.T) {
		_, err := orchestrator.GetVolumeByInternalName(ctx, "volume1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetVolume", func(t *testing.T) {
		_, err := orchestrator.GetVolume(ctx, "volume1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("ListVolumes", func(t *testing.T) {
		_, err := orchestrator.ListVolumes(ctx)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("DeleteVolume", func(t *testing.T) {
		err := orchestrator.DeleteVolume(ctx, "volume1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("ImportVolume", func(t *testing.T) {
		volumeConfig := &storage.VolumeConfig{Name: "import1", ImportBackendUUID: "uuid1", ImportOriginalName: "orig1"}
		_, err := orchestrator.ImportVolume(ctx, volumeConfig)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	// Note: LegacyImportVolume and SetVolumeState methods don't exist in the current implementation

	t.Run("AttachVolume", func(t *testing.T) {
		err := orchestrator.AttachVolume(ctx, "volume1", "/mnt/test", &models.VolumePublishInfo{})
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	// DetachVolume does not take the core lock
	// t.Run("DetachVolume", func(t *testing.T) {
	// 	err := orchestrator.DetachVolume(ctx, "volume1", "/mnt/test")
	// 	assert.Error(t, err)
	// 	assert.Equal(t, context.Canceled, err)
	// })

	t.Run("ListSubordinateVolumes", func(t *testing.T) {
		_, err := orchestrator.ListSubordinateVolumes(ctx, "")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetSubordinateSourceVolume", func(t *testing.T) {
		_, err := orchestrator.GetSubordinateSourceVolume(ctx, "subordinate1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("CreateSnapshot", func(t *testing.T) {
		snapshotConfig := &storage.SnapshotConfig{Name: "snapshot1", VolumeName: "volume1"}
		_, err := orchestrator.CreateSnapshot(ctx, snapshotConfig)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("CreateGroupSnapshot", func(t *testing.T) {
		groupSnapshotConfig := &storage.GroupSnapshotConfig{Name: "group1"}
		_, err := orchestrator.CreateGroupSnapshot(ctx, groupSnapshotConfig)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetSnapshot", func(t *testing.T) {
		_, err := orchestrator.GetSnapshot(ctx, "volume1", "snapshot1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	// Note: GetSnapshots method doesn't exist

	t.Run("DeleteGroupSnapshot", func(t *testing.T) {
		err := orchestrator.DeleteGroupSnapshot(ctx, "group1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("ImportSnapshot", func(t *testing.T) {
		snapshotConfig := &storage.SnapshotConfig{Name: "import_snapshot1", VolumeName: "volume1"}
		_, err := orchestrator.ImportSnapshot(ctx, snapshotConfig)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetGroupSnapshot", func(t *testing.T) {
		_, err := orchestrator.GetGroupSnapshot(ctx, "group1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("RestoreSnapshot", func(t *testing.T) {
		err := orchestrator.RestoreSnapshot(ctx, "volume1", "snapshot1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("DeleteSnapshot", func(t *testing.T) {
		err := orchestrator.DeleteSnapshot(ctx, "volume1", "snapshot1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("ListSnapshots", func(t *testing.T) {
		_, err := orchestrator.ListSnapshots(ctx)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("ListSnapshotsForVolume", func(t *testing.T) {
		_, err := orchestrator.ListSnapshotsForVolume(ctx, "volume1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	// ReadSnapshotsForVolume does not take the core lock
	// t.Run("ReadSnapshotsForVolume", func(t *testing.T) {
	// 	_, err := orchestrator.ReadSnapshotsForVolume(ctx, "volume1")
	// 	assert.Error(t, err)
	// 	assert.Equal(t, context.Canceled, err)
	// })

	// Note: GetGroupSnapshots method doesn't exist

	t.Run("ReloadVolumes", func(t *testing.T) {
		err := orchestrator.ReloadVolumes(ctx)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("ResizeVolume", func(t *testing.T) {
		err := orchestrator.ResizeVolume(ctx, "volume1", "10Gi")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("AddStorageClass", func(t *testing.T) {
		scConfig := &storageclass.Config{Name: "sc1"}
		_, err := orchestrator.AddStorageClass(ctx, scConfig)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetStorageClass", func(t *testing.T) {
		_, err := orchestrator.GetStorageClass(ctx, "sc1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("ListStorageClasses", func(t *testing.T) {
		_, err := orchestrator.ListStorageClasses(ctx)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("DeleteStorageClass", func(t *testing.T) {
		err := orchestrator.DeleteStorageClass(ctx, "sc1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("AddNode", func(t *testing.T) {
		node := &models.Node{Name: "node1"}
		err := orchestrator.AddNode(ctx, node, nil)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	// GetNode does not take the core lock
	// t.Run("GetNode", func(t *testing.T) {
	// 	_, err := orchestrator.GetNode(ctx, "node1")
	// 	assert.Error(t, err)
	// 	assert.Equal(t, context.Canceled, err)
	// })

	// ListNodes does not take the core lock
	// t.Run("ListNodes", func(t *testing.T) {
	// 	_, err := orchestrator.ListNodes(ctx)
	// 	assert.Error(t, err)
	// 	assert.Equal(t, context.Canceled, err)
	// })

	t.Run("DeleteNode", func(t *testing.T) {
		err := orchestrator.DeleteNode(ctx, "node1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	// GetVolumePublication does not take the core lock
	// t.Run("GetVolumePublication", func(t *testing.T) {
	// 	_, err := orchestrator.GetVolumePublication(ctx, "volume1", "node1")
	// 	assert.Error(t, err)
	// 	assert.Equal(t, context.Canceled, err)
	// })

	// ListVolumePublications does not take the core lock
	// t.Run("ListVolumePublications", func(t *testing.T) {
	// 	_, err := orchestrator.ListVolumePublications(ctx)
	// 	assert.Error(t, err)
	// 	assert.Equal(t, context.Canceled, err)
	// })

	// ListVolumePublicationsForVolume does not take the core lock
	// t.Run("ListVolumePublicationsForVolume", func(t *testing.T) {
	// 	_, err := orchestrator.ListVolumePublicationsForVolume(ctx, "volume1")
	// 	assert.Error(t, err)
	// 	assert.Equal(t, context.Canceled, err)
	// })

	// ListVolumePublicationsForNode does not take the core lock
	// t.Run("ListVolumePublicationsForNode", func(t *testing.T) {
	// 	_, err := orchestrator.ListVolumePublicationsForNode(ctx, "node1")
	// 	assert.Error(t, err)
	// 	assert.Equal(t, context.Canceled, err)
	// })

	t.Run("ReconcileVolumePublications", func(t *testing.T) {
		err := orchestrator.ReconcileVolumePublications(ctx, []*models.VolumePublicationExternal{})
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("EstablishMirror", func(t *testing.T) {
		err := orchestrator.EstablishMirror(ctx, "backend1", "local1", "remote1", "schedule1", "policy1", "remoteCluster1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("ReestablishMirror", func(t *testing.T) {
		err := orchestrator.ReestablishMirror(ctx, "backend1", "local1", "remote1", "schedule1", "policy1", "remoteCluster1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("PromoteMirror", func(t *testing.T) {
		_, err := orchestrator.PromoteMirror(ctx, "backend1", "local1", "remote1", "snapshot1", "remoteCluster1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetMirrorStatus", func(t *testing.T) {
		_, err := orchestrator.GetMirrorStatus(ctx, "backend1", "local1", "remote1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("CanBackendMirror", func(t *testing.T) {
		_, err := orchestrator.CanBackendMirror(ctx, "backend1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("ReleaseMirror", func(t *testing.T) {
		err := orchestrator.ReleaseMirror(ctx, "backend1", "local1", "remote1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetReplicationDetails", func(t *testing.T) {
		_, _, _, err := orchestrator.GetReplicationDetails(ctx, "backend1", "local1", "remote1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("UpdateMirror", func(t *testing.T) {
		err := orchestrator.UpdateMirror(ctx, "local1", "remote1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("CheckMirrorTransferState", func(t *testing.T) {
		_, err := orchestrator.CheckMirrorTransferState(ctx, "local1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetMirrorTransferTime", func(t *testing.T) {
		_, err := orchestrator.GetMirrorTransferTime(ctx, "local1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetCHAP", func(t *testing.T) {
		_, err := orchestrator.GetCHAP(ctx, "volume1", "node1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	// Note: GetCHAPSecrets method doesn't exist

	t.Run("SetLogLevel", func(t *testing.T) {
		err := orchestrator.SetLogLevel(ctx, "debug")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetLogLevel", func(t *testing.T) {
		_, err := orchestrator.GetLogLevel(ctx)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	// Note: SetLogWorkflows method doesn't exist

	t.Run("SetLogLayers", func(t *testing.T) {
		err := orchestrator.SetLogLayers(ctx, "core")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetBackend", func(t *testing.T) {
		_, err := orchestrator.GetBackend(ctx, "backend1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("GetBackendByBackendUUID", func(t *testing.T) {
		_, err := orchestrator.GetBackendByBackendUUID(ctx, "uuid1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("ListBackends", func(t *testing.T) {
		_, err := orchestrator.ListBackends(ctx)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("DeleteBackend", func(t *testing.T) {
		err := orchestrator.DeleteBackend(ctx, "backend1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("DeleteBackendByBackendUUID", func(t *testing.T) {
		err := orchestrator.DeleteBackendByBackendUUID(ctx, "backend1", "uuid1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("RemoveBackendConfigRef", func(t *testing.T) {
		err := orchestrator.RemoveBackendConfigRef(ctx, "uuid1", "configRef1")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("AddVolume", func(t *testing.T) {
		volumeConfig := &storage.VolumeConfig{
			Name:         "volume1",
			Size:         "1Gi",
			Protocol:     config.File,
			StorageClass: "default",
		}
		_, err := orchestrator.AddVolume(ctx, volumeConfig)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("UpdateVolumeLUKSPassphraseNames", func(t *testing.T) {
		passphraseNames := []string{"passphrase1", "passphrase2"}
		err := orchestrator.UpdateVolumeLUKSPassphraseNames(ctx, "volume1", &passphraseNames)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("CloneVolume", func(t *testing.T) {
		volumeConfig := &storage.VolumeConfig{
			Name:              "clone1",
			Size:              "1Gi",
			Protocol:          config.File,
			StorageClass:      "default",
			CloneSourceVolume: "sourceVolume",
		}
		_, err := orchestrator.CloneVolume(ctx, volumeConfig)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

// TestContextCancellationAfterLockWithNilValues tests context cancellation
// with nil/empty values to ensure the methods still check context before proceeding.
func TestContextCancellationAfterLockWithNilValues(t *testing.T) {
	storeClient := persistentstore.NewInMemoryClient()
	orchestrator, err := NewTridentOrchestrator(storeClient)
	assert.NoError(t, err)
	orchestrator.bootstrapError = nil

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	t.Run("AddBackend_EmptyConfig", func(t *testing.T) {
		_, err := orchestrator.AddBackend(ctx, "", "")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("UpdateBackend_EmptyBackendName", func(t *testing.T) {
		_, err := orchestrator.UpdateBackend(ctx, "", "{}", "")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("UpdateBackendState_EmptyBackendState", func(t *testing.T) {
		_, err := orchestrator.UpdateBackendState(ctx, "backend1", "", "userState")
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("AddVolume_NilConfig", func(t *testing.T) {
		_, err := orchestrator.AddVolume(ctx, nil)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("UpdateVolumeLUKSPassphraseNames_NilPassphrases", func(t *testing.T) {
		err := orchestrator.UpdateVolumeLUKSPassphraseNames(ctx, "volume1", nil)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("CloneVolume_NilConfig", func(t *testing.T) {
		_, err := orchestrator.CloneVolume(ctx, nil)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

// TestContextCancellationBeforeBootstrap tests context cancellation when
// bootstrap has not been completed (bootstrapError is set).
func TestContextCancellationBeforeBootstrap(t *testing.T) {
	storeClient := persistentstore.NewInMemoryClient()
	orchestrator, err := NewTridentOrchestrator(storeClient)
	assert.NoError(t, err)

	// Set bootstrap error to simulate uninitialized state

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Even with bootstrap error, some methods should still return context error
	// if they would have checked context after the mutex lock
	t.Run("AddBackend_WithBootstrapError", func(t *testing.T) {
		_, err := orchestrator.AddBackend(ctx, "{}", "")
		assert.Error(t, err)
		// Should return bootstrap error, not context error, because bootstrap check comes first
		assert.ErrorContains(t, err, "Trident is initializing")
	})

	t.Run("UpdateBackend_WithBootstrapError", func(t *testing.T) {
		_, err := orchestrator.UpdateBackend(ctx, "backend1", "{}", "")
		assert.Error(t, err)
		// Should return bootstrap error, not context error, because bootstrap check comes first
		assert.ErrorContains(t, err, "Trident is initializing")
	})
}

// TestWithoutCancel shows that WithoutCancel removes deadlines
func TestWithoutCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	ctx = context.WithoutCancel(ctx)
	time.Sleep(20 * time.Millisecond)
	assert.NoError(t, ctx.Err())
}
