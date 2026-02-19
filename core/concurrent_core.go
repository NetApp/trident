// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"go.uber.org/multierr"
	"golang.org/x/time/rate"

	"github.com/netapp/trident/config"
	db "github.com/netapp/trident/core/concurrent_cache"
	"github.com/netapp/trident/frontend"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	. "github.com/netapp/trident/logging"
	persistentstore "github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/factory"
	sa "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/utils/autogrow"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
)

type ConcurrentTridentOrchestrator struct {
	frontends      map[string]frontend.Plugin
	storeClient    persistentstore.Client
	uuid           string
	bootstrapped   bool
	bootstrapError error
	bootstrapCond  *sync.Cond // Signals when bootstrap is complete

	// mtx locks brief access to orchestrator state
	mtx *sync.Mutex

	scPoolMap      *storageclass.PoolMap
	scPoolMapMutex *sync.RWMutex

	txnMutex *locks.GCNamedMutex

	lastNodeRegistrationMutex *sync.RWMutex
	lastNodeRegistrationTime  time.Time
	nodeAccessReconcilePeriod time.Duration

	stopNodeAccessLoop       chan bool
	stopReconcileBackendLoop chan bool
}

var (
	_ Orchestrator = &ConcurrentTridentOrchestrator{}
)

func NewConcurrentTridentOrchestrator(client persistentstore.Client) (Orchestrator, error) {
	// Initialize global VP sync rate limiter if not already initialized
	if vpSyncRateLimiter == nil {
		vpSyncRateLimiter = rate.NewLimiter(vpUpdateRateLimit, vpUpdateBurst)
	}
	return &ConcurrentTridentOrchestrator{
		frontends:                 make(map[string]frontend.Plugin),
		storeClient:               client,
		bootstrapped:              false,
		bootstrapError:            errors.NotReadyError(),
		bootstrapCond:             sync.NewCond(&sync.Mutex{}),
		mtx:                       &sync.Mutex{},
		scPoolMap:                 storageclass.NewPoolMap(),
		scPoolMapMutex:            &sync.RWMutex{},
		txnMutex:                  locks.NewGCNamedMutex(),
		lastNodeRegistrationMutex: &sync.RWMutex{},
		lastNodeRegistrationTime:  time.Now(),
		nodeAccessReconcilePeriod: NodeAccessReconcilePeriod,
	}, nil
}

func (o *ConcurrentTridentOrchestrator) ListSnapshotsForGroup(ctx context.Context, groupName string) ([]*storage.SnapshotExternal, error) {
	return nil, fmt.Errorf("ListSnapshotsForGroup is not implemented for concurrent core")
}

func (o *ConcurrentTridentOrchestrator) CreateGroupSnapshot(ctx context.Context, config *storage.GroupSnapshotConfig) (*storage.GroupSnapshotExternal, error) {
	return nil, fmt.Errorf("CreateGroupSnapshot is not implemented for concurrent core")
}

func (o *ConcurrentTridentOrchestrator) DeleteGroupSnapshot(ctx context.Context, groupName string) error {
	return fmt.Errorf("DeleteGroupSnapshot is not implemented for concurrent core")
}

func (o *ConcurrentTridentOrchestrator) GetGroupSnapshot(ctx context.Context, groupName string) (*storage.GroupSnapshotExternal, error) {
	return nil, fmt.Errorf("GetGroupSnapshot is not implemented for concurrent core")
}

func (o *ConcurrentTridentOrchestrator) ListGroupSnapshots(ctx context.Context) ([]*storage.GroupSnapshotExternal, error) {
	return nil, fmt.Errorf("ListGroupSnapshots is not implemented for concurrent core")
}

// RebuildStorageClassPoolMap rebuilds the storage class to pool mapping for each backend.
// TODO (cknight): automate this when the pool map is moved into the cache layer.
func (o *ConcurrentTridentOrchestrator) RebuildStorageClassPoolMap(ctx context.Context) {
	o.scPoolMapMutex.Lock()
	defer o.scPoolMapMutex.Unlock()

	results, unlocker, err := db.Lock(ctx, db.Query(db.ListBackends(), db.ListStorageClasses()))
	defer unlocker()
	if err != nil {
		return
	}

	o.scPoolMap.Rebuild(ctx, results[0].StorageClasses, results[0].Backends)
}

// GetStorageClassPoolMap returns a deep copy of the storage class to pool mapping for each backend.
// TODO (cknight): we're using deep copy semantics to mimic the lockless reads of the cache layer.
func (o *ConcurrentTridentOrchestrator) GetStorageClassPoolMap() *storageclass.PoolMap {
	o.scPoolMapMutex.RLock()
	defer o.scPoolMapMutex.RUnlock()

	return o.scPoolMap.DeepCopy()
}

func (o *ConcurrentTridentOrchestrator) transformPersistentState(ctx context.Context) error {
	version, err := o.storeClient.GetVersion(ctx)
	if err != nil && persistentstore.MatchKeyNotFoundErr(err) {
		// Persistent store and Trident API versions should be crdv1 and v1 respectively.
		version = &config.PersistentStateVersion{
			PersistentStoreVersion: string(persistentstore.CRDV1Store),
			OrchestratorAPIVersion: config.OrchestratorAPIVersion,
			// Assume publications are not synced if the version CR wasn't found.
			PublicationsSynced: false,
		}
		Logc(ctx).WithFields(LogFields{
			"PersistentStoreVersion": version.PersistentStoreVersion,
			"OrchestratorAPIVersion": version.OrchestratorAPIVersion,
		}).Warning("Persistent state version not found, creating.")
	} else if err != nil {
		return fmt.Errorf("couldn't determine the orchestrator persistent state version: %v", err)
	}

	if config.OrchestratorAPIVersion != version.OrchestratorAPIVersion {
		Logc(ctx).WithFields(LogFields{
			"current_api_version": version.OrchestratorAPIVersion,
			"desired_api_version": config.OrchestratorAPIVersion,
		}).Info("Transforming Trident API objects on the persistent store.")
		// Transform Trident API objects if the API version has changed.
	}

	// Store the persistent store and API versions.
	version.PersistentStoreVersion = string(o.storeClient.GetType())
	version.OrchestratorAPIVersion = config.OrchestratorAPIVersion
	if err = o.storeClient.SetVersion(ctx, version); err != nil {
		return fmt.Errorf("failed to set the persistent state version after migration: %v", err)
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) Bootstrap(_ bool) error {
	config.IsConcurrent = true
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowCoreBootstrap, LogLayerCore)
	var err error

	if len(o.frontends) == 0 {
		Logc(ctx).Warning("Trident is bootstrapping with no frontend.")
	}

	// Transform persistent state, if necessary
	if err = o.transformPersistentState(ctx); err != nil {
		o.bootstrapError = errors.BootstrapError(err)
		return o.bootstrapError
	}

	o.uuid, err = o.storeClient.GetTridentUUID(ctx)
	if err != nil {
		o.bootstrapError = errors.BootstrapError(err)
		return o.bootstrapError
	}

	// Bootstrap state from persistent store
	if err = o.bootstrap(ctx); err != nil {
		o.bootstrapCond.L.Lock()
		o.bootstrapError = errors.BootstrapError(err)
		o.bootstrapCond.Broadcast()
		o.bootstrapCond.L.Unlock()
		return o.bootstrapError
	}

	// Signal any goroutines waiting for bootstrap to complete
	o.bootstrapCond.L.Lock()
	o.bootstrapped = true
	o.bootstrapError = nil
	o.bootstrapCond.Broadcast()
	o.bootstrapCond.L.Unlock()

	Logc(ctx).Infof("%s bootstrapped successfully.", convert.ToTitle(config.OrchestratorName))
	return nil
}

// bootstrap initializes the orchestrator core by reading the persistent store and populating the cache.
func (o *ConcurrentTridentOrchestrator) bootstrap(ctx context.Context) error {
	// Call the various bootstrap functions in order, handling errors as they occur.

	type bootstrapFunc func(context.Context) error
	for _, f := range []bootstrapFunc{
		o.bootstrapBackends,
		// Autogrow policies loaded before volumes so volumes can be associated with policies
		o.bootstrapAutogrowPolicies,
		// Volumes, storage classes, and snapshots require backends to be bootstrapped.
		o.bootstrapStorageClasses, o.bootstrapVolumes, o.bootstrapSnapshots,
		// Volume transactions require volumes and snapshots to be bootstrapped.
		o.bootstrapVolTxns,
		// Node access reconciliation is part of node bootstrap and requires volume publications to be bootstrapped.
		o.bootstrapVolumePublications, o.bootstrapNodes,
		// Subordinate volumes require volumes to be bootstrapped.
		o.bootstrapSubordinateVolumes,
	} {
		err := f(ctx)
		if err != nil {
			if persistentstore.MatchKeyNotFoundErr(err) {
				keyError, ok := err.(*persistentstore.Error)
				if !ok {
					return errors.TypeAssertionError("err.(*persistentstore.Error)")
				}
				Logc(ctx).Warnf("Unable to find key %s.  Continuing bootstrap, but "+
					"consider checking integrity if Trident installation is not new.", keyError.Key)
			} else {
				return err
			}
		}
	}

	// Clean up any deleting backends that lack volumes.  This can happen if a connection to the store
	// fails when attempting to delete a backend.
	o.cleanupDeletingBackends(ctx)

	o.RebuildStorageClassPoolMap(ctx)

	return nil
}

// bootstrapBackends reads backends from the persistent store and loads them into the concurrent cache.
func (o *ConcurrentTridentOrchestrator) bootstrapBackends(ctx context.Context) error {
	persistentBackendList, storeErr := o.storeClient.GetBackends(ctx)
	if storeErr != nil {
		Logc(ctx).WithError(storeErr).Error("Failed to get backends from the persistent store.")
		return storeErr
	}

	backendCount := 0

	for _, storageBackend := range persistentBackendList {
		serializedConfig, marshalErr := storageBackend.MarshalConfig()
		if marshalErr != nil {
			Logc(ctx).WithFields(LogFields{
				"backend":     storageBackend.Name,
				"backendUUID": storageBackend.BackendUUID,
			}).WithError(marshalErr).Debug("Failed to marshal backend config.")
			return marshalErr
		}

		backend, backendErr := o.validateAndCreateBackendFromConfig(ctx, serializedConfig, storageBackend.ConfigRef,
			storageBackend.BackendUUID)
		if backendErr != nil {
			Logc(ctx).WithError(backendErr).WithFields(LogFields{
				"handler":            "Bootstrap",
				"newBackendExternal": backend,
			}).Error("Failed to create backend from config.")

			// Trident for Docker supports one backend at a time, and the Docker volume plugin
			// should not start if the backend fails to initialize, so return any error here.
			if config.CurrentDriverContext == config.ContextDocker {
				return backendErr
			}
		}

		if backend == nil {
			// If we couldn't create a backend, even a failed one, skip this backend.
			continue
		}

		results, unlocker, lockErr := db.Lock(
			ctx, db.Query(db.UpsertBackend(backend.BackendUUID(), "", backend.Name())))
		if lockErr != nil {
			Logc(ctx).WithFields(LogFields{
				"backend":     storageBackend.Name,
				"backendUUID": storageBackend.BackendUUID,
			}).WithError(lockErr).Error("Failed to lock backend for upsert.")
			unlocker()
			return lockErr
		}

		// Set some backend values from the persistent version that aren't in the config
		if storageBackend.State == storage.Deleting {
			backend.SetState(storage.Deleting)
		}
		backend.SetUserState(storageBackend.UserState)

		results[0].Backend.Upsert(backend)
		unlocker()

		Logc(ctx).WithFields(LogFields{
			"backend": backend.Name(),
			"handler": "Bootstrap",
		}).Info("Added an existing backend.")

		backendCount++
	}

	Logc(ctx).Infof("Added %d existing backend(s).", backendCount)
	return nil
}

// bootstrapStorageClasses reads storage classes from the persistent store and loads them into the concurrent cache.
func (o *ConcurrentTridentOrchestrator) bootstrapStorageClasses(ctx context.Context) error {
	persistentStorageClasses, storeErr := o.storeClient.GetStorageClasses(ctx)
	if storeErr != nil {
		Logc(ctx).WithError(storeErr).Error("Failed to get storage classes from the persistent store.")
		return storeErr
	}

	scCount := 0

	for _, psc := range persistentStorageClasses {
		sc := storageclass.NewFromPersistent(psc)

		results, unlocker, upsertErr := db.Lock(ctx, db.Query(db.UpsertStorageClass(psc.GetName())))
		if upsertErr != nil {
			Logc(ctx).WithField("name", sc.GetName()).WithError(upsertErr).Error(
				"Failed to lock storage class for upsert.")
			unlocker()
			return upsertErr
		}

		results[0].StorageClass.Upsert(sc)
		unlocker()

		Logc(ctx).WithFields(LogFields{
			"storageClass": sc.GetName(),
			"handler":      "Bootstrap",
		}).Info("Added an existing storage class.")

		scCount++
	}

	Logc(ctx).Infof("Added %d existing storage class(es).", scCount)
	return nil
}

// bootstrapVolumes reads volumes from the persistent store and loads them into the concurrent cache.
func (o *ConcurrentTridentOrchestrator) bootstrapVolumes(ctx context.Context) error {
	volumes, storeErr := o.storeClient.GetVolumes(ctx)
	if storeErr != nil {
		Logc(ctx).WithError(storeErr).Error("Failed to get volumes from the persistent store.")
		return storeErr
	}

	volCount := 0

	for _, v := range volumes {
		vol := storage.NewVolume(v.Config, v.BackendUUID, v.Pool, v.Orphaned, v.State)
		vol.AutogrowStatus = v.AutogrowStatus

		if vol.IsSubordinate() {

			results, unlocker, upsertErr := db.Lock(ctx,
				db.Query(db.UpsertSubordinateVolume(vol.Config.Name, vol.Config.ShareSourceVolume),
					db.ReadBackend("")))
			if upsertErr != nil {
				Logc(ctx).WithFields(LogFields{
					"subordinateVolume": vol.Config.Name,
					"sourceVolume":      vol.Config.ShareSourceVolume,
				}).WithError(upsertErr).Error("Failed to lock subordinateVolume for upsert.")
				unlocker()
				return upsertErr
			}

			backend := results[0].Backend.Read

			// Set the publish enforcement flag on the subordinate volume if supported by the backend and drvier.
			// This is needed for nas and nas eco volumes. Legacy volumes may not have this flag set. Needed for
			// automatic force-detach.
			o.healTridentVolumePublishEnforcement(ctx, vol, backend)

			// Rebuild Autogrow policy associations (CSI mode only)
			if config.CurrentDriverContext == config.ContextCSI {
				_, _ = o.resolveAndSetEffectiveAutogrowPolicy(ctx, vol, "bootstrap", "debug")
			}

			results[0].SubordinateVolume.Upsert(vol)
			unlocker()

		} else {

			results, unlocker, upsertErr := db.Lock(ctx, db.Query(
				db.UpsertVolume(vol.Config.Name, vol.BackendUUID),
				db.ReadBackend(""),
			))
			if upsertErr != nil {
				Logc(ctx).WithField("volume", vol.Config.Name).WithError(upsertErr).Error(
					"Failed to lock volume for upsert.")
				unlocker()
				return upsertErr
			}

			backend := results[0].Backend.Read
			upserter := results[0].Volume.Upsert

			o.healTridentVolumePublishEnforcement(ctx, vol, backend)

			if backend == nil {
				Logc(ctx).WithFields(LogFields{
					"volume":      v.Config.Name,
					"backendUUID": v.BackendUUID,
				}).Warning("Couldn't find backend. Setting state to MissingBackend.")
				vol.State = storage.VolumeStateMissingBackend
			} else {
				backend.Volumes().Store(vol.Config.Name, vol)
				if fakeDriver, ok := backend.Driver().(*fake.StorageDriver); ok {
					fakeDriver.BootstrapVolume(ctx, vol)
				}
			}

			// Rebuild Autogrow policy associations (CSI mode only)
			if config.CurrentDriverContext == config.ContextCSI {
				_, _ = o.resolveAndSetEffectiveAutogrowPolicy(ctx, vol, "bootstrap", "debug")
			}

			upserter(vol)
			unlocker()
		}

		volCount++
	}

	Logc(ctx).Infof("Added %d existing volume(s).", volCount)
	return nil
}

func (o *ConcurrentTridentOrchestrator) healTridentVolumePublishEnforcement(
	ctx context.Context, vol *storage.Volume, backend storage.Backend,
) {
	if vol.Config.AccessInfo.PublishEnforcement {
		// If publish enforcement is already enabled on the volume, nothing to do.
		return
	}

	// If this backend cannot enable publish enforcement, then, no volume on this backend
	// can have publish enforcement enabled.
	if backend == nil {
		Logc(ctx).WithField("volume", vol.Config.Name).
			Info("Volume cannot have publish enforcement enabled, backend missing.")
		return
	}

	// Enable publish enforcement on the volume.
	_ = backend.HealVolumePublishEnforcement(ctx, vol)

	return
}

// bootstrapSnapshots reads snapshots from the persistent store and loads them into the concurrent cache.
func (o *ConcurrentTridentOrchestrator) bootstrapSnapshots(ctx context.Context) error {
	snapshots, storeErr := o.storeClient.GetSnapshots(ctx)
	if storeErr != nil {
		Logc(ctx).WithError(storeErr).Error("Failed to get snapshots from the persistent store.")
		return storeErr
	}

	snapshotCount := 0

	for _, s := range snapshots {
		snapshot := storage.NewSnapshot(s.Config, s.Created, s.SizeBytes, s.State)

		results, unlocker, upsertErr := db.Lock(ctx, db.Query(
			db.UpsertSnapshot(snapshot.Config.VolumeName, snapshot.Config.ID()),
			db.ReadBackend(""),
			db.ReadVolume(""),
		))
		if upsertErr != nil {
			Logc(ctx).WithField("volume", snapshot.Config.Name).WithError(upsertErr).Error(
				"Failed to lock snapshot for upsert.")
			unlocker()
			return upsertErr
		}

		volume := results[0].Volume.Read
		backend := results[0].Backend.Read
		upserter := results[0].Snapshot.Upsert

		if volume == nil {
			Logc(ctx).Warnf("Couldn't find volume %s for snapshot %s. Setting snapshot state to MissingVolume.",
				s.Config.VolumeName, s.Config.Name)
			snapshot.State = storage.SnapshotStateMissingVolume
		} else if backend == nil {
			snapshot.State = storage.SnapshotStateMissingBackend
		} else {
			if fakeDriver, ok := backend.Driver().(*fake.StorageDriver); ok {
				fakeDriver.BootstrapSnapshot(ctx, snapshot, volume.Config)
			}
		}

		upserter(snapshot)
		unlocker()

		snapshotCount++
	}

	Logc(ctx).Infof("Added %d existing snapshots(s).", snapshotCount)
	return nil
}

// bootstrapAutogrowPolicies reads Autogrow policies from the persistent store and loads them into the concurrent cache.
func (o *ConcurrentTridentOrchestrator) bootstrapAutogrowPolicies(ctx context.Context) error {
	persistentAGPolicies, storeErr := o.storeClient.GetAutogrowPolicies(ctx)
	if storeErr != nil {
		Logc(ctx).WithError(storeErr).Error("Failed to get autogrow policies from the persistent store.")
		return storeErr
	}

	agpCount := 0

	for _, persistentAGPolicy := range persistentAGPolicies {
		// Create autogrow policy object with empty volume list (associations rebuilt during bootstrapVolumes)
		agPolicy := storage.NewAutogrowPolicyFromPersistent(persistentAGPolicy)

		results, unlocker, upsertErr := db.Lock(ctx, db.Query(db.UpsertAutogrowPolicy(agPolicy.Name())))
		if upsertErr != nil {
			Logc(ctx).WithField("autogrowPolicy", agPolicy.Name()).WithError(upsertErr).Error(
				"Failed to lock autogrow policy for upsert.")
			unlocker()
			return upsertErr
		}

		results[0].AutogrowPolicy.Upsert(agPolicy)
		unlocker()

		Logc(ctx).WithFields(LogFields{
			"autogrowPolicyName":  agPolicy.Name(),
			"autogrowPolicyState": agPolicy.State(),
			"handler":             "Bootstrap",
		}).Info("Added an existing autogrow policy.")

		agpCount++
	}

	Logc(ctx).Infof("Added %d existing autogrow policy(ies).", agpCount)
	return nil
}

// bootstrapVolTxns reads volume transactions from the persistent store and processes them.
func (o *ConcurrentTridentOrchestrator) bootstrapVolTxns(ctx context.Context) error {
	volTxns, storeErr := o.storeClient.GetVolumeTransactions(ctx)
	if storeErr != nil && !persistentstore.MatchKeyNotFoundErr(storeErr) {
		Logc(ctx).WithError(storeErr).Error("Failed to get transactions from the persistent store.")
		return storeErr
	}

	for _, txn := range volTxns {

		// Acquire lock on the transaction name
		o.txnMutex.Lock(txn.Name())

		// Clean up transaction and anything that may have been left behind
		if txnErr := o.handleFailedTransaction(ctx, txn); txnErr != nil {
			o.txnMutex.Unlock(txn.Name())
			return txnErr
		}

		o.txnMutex.Unlock(txn.Name())
	}
	return nil
}

// bootstrapVolumePublications reads volume publications from the persistent store and loads them into the concurrent cache.
func (o *ConcurrentTridentOrchestrator) bootstrapVolumePublications(ctx context.Context) error {
	// Don't bootstrap volume publications if we're not CSI
	if config.CurrentDriverContext != config.ContextCSI {
		return nil
	}

	volumePublications, storeErr := o.storeClient.GetVolumePublications(ctx)
	if storeErr != nil {
		Logc(ctx).WithError(storeErr).Error("Failed to get volume publications from the persistent store.")
		return storeErr
	}

	vpCount := 0
	vpsToBeSynced := make([]*models.VolumePublication, 0, len(volumePublications))

	for _, vp := range volumePublications {
		// Update VP fields from corresponding TridentVolume (use inconsistent read since we're only examining)
		// Check both regular volumes and subordinate volumes in a single query
		results, unlocker, err := db.Lock(
			ctx,
			db.Query(db.InconsistentReadVolume(vp.VolumeName)),
			db.Query(db.InconsistentReadSubordinateVolume(vp.VolumeName)))
		unlocker()

		var vol *storage.Volume

		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"volumeName": vp.VolumeName,
				"nodeName":   vp.NodeName,
			}).WithError(err).Warn("Failed to read volume for VP sync during bootstrap")
		} else if results[0].Volume.Read != nil {
			// Regular volume found
			vol = results[0].Volume.Read
		} else if results[1].SubordinateVolume.Read != nil {
			// Subordinate volume found
			vol = results[1].SubordinateVolume.Read
		} else {
			// Neither found
			Logc(ctx).WithFields(LogFields{
				"volumeName": vp.VolumeName,
				"nodeName":   vp.NodeName,
			}).Debug("Volume not found for volume publication during bootstrap, skipping sync")
		}

		// Common sync logic - executes if vol was found (either regular or subordinate)
		if vol != nil {
			// syncVolumePublicationFields modifies vp in place and returns true if sync is needed
			syncNeeded := syncVolumePublicationFields(vol, vp)

			// Add it to the list of vpsToBeSynced if syncNeeded is true
			if syncNeeded {
				vpsToBeSynced = append(vpsToBeSynced, vp)
			}
		}

		// Add VP to cache with updated information
		results, unlocker, upsertErr := db.Lock(ctx, db.Query(db.UpsertVolumePublication(vp.VolumeName, vp.NodeName)))
		if upsertErr != nil {
			Logc(ctx).WithField("publication", vp.Name).WithError(upsertErr).Error(
				"Failed to lock volume publication for upsert.")
			unlocker()
			return upsertErr
		}

		results[0].VolumePublication.Upsert(vp)
		unlocker()

		Logc(ctx).WithFields(LogFields{
			"volumeName": vp.VolumeName,
			"nodeName":   vp.NodeName,
		}).Trace("Loaded volume publication into cache during bootstrap")

		vpCount++
	}

	Logc(ctx).Infof("Added %d existing volume publications(s).", vpCount)

	// Asynchronously persist VPs that need syncing (e.g., after upgrade)
	// The goroutine will wait for bootstrap to complete before proceeding
	if len(vpsToBeSynced) > 0 {
		go func() {
			// Wait for bootstrap to complete
			o.bootstrapCond.L.Lock()
			for !o.bootstrapped && (o.bootstrapError == nil || errors.IsNotReadyError(o.bootstrapError)) {
				o.bootstrapCond.Wait()
			}
			err := o.bootstrapError
			o.bootstrapCond.L.Unlock()

			if err != nil {
				Logc(ctx).WithError(err).Error("Bootstrap failed, skipping volume publication sync")
				return
			}

			// Now safe to proceed with sync
			o.SyncVolumePublications(ctx, vpsToBeSynced)
		}()
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) bootstrapNodes(ctx context.Context) error {
	// Don't bootstrap nodes if we're not CSI
	if config.CurrentDriverContext != config.ContextCSI {
		return nil
	}

	nodes, storeErr := o.storeClient.GetNodes(ctx)
	if storeErr != nil {
		Logc(ctx).WithError(storeErr).Error("Failed to get volume publications from the persistent store.")
		return storeErr
	}

	nodeCount := 0

	for _, node := range nodes {
		results, unlocker, upsertErr := db.Lock(ctx, db.Query(db.UpsertNode(node.Name)))
		if upsertErr != nil {
			Logc(ctx).WithField("node", node.Name).WithError(upsertErr).Error("Failed to lock node for upsert.")
			unlocker()
			return upsertErr
		}

		results[0].Node.Upsert(node)
		unlocker()

		nodeCount++
	}

	err := o.reconcileNodeAccessOnAllBackends(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warning("Node access reconciliation failed during bootstrapping.")
	}

	Logc(ctx).Infof("Added %d existing node(s).", nodeCount)
	return nil
}

// bootstrapSubordinateVolumes updates the source volumes to point to their subordinates.  Updating the
// super->sub references here allows us to persist only the scalar sub->super references.
func (o *ConcurrentTridentOrchestrator) bootstrapSubordinateVolumes(ctx context.Context) error {
	results, unlocker, listErr := db.Lock(ctx, db.Query(db.ListSubordinateVolumes()))
	unlocker()
	if listErr != nil {
		Logc(ctx).WithError(listErr).Error("Failed to list subordinate volumes.")
		return listErr
	}

	subVols := results[0].SubordinateVolumes

	for _, subVol := range subVols {

		results, unlocker, upsertErr := db.Lock(ctx, db.Query(db.UpsertVolume(subVol.Config.ShareSourceVolume, "")))
		if upsertErr != nil {
			Logc(ctx).WithFields(LogFields{
				"subordinateVolume": subVol.Config.Name,
				"sourceVolume":      subVol.Config.ShareSourceVolume,
			}).WithError(upsertErr).Error("Failed to lock share source volume for upsert.")
			unlocker()
			return upsertErr
		}

		sourceVolume := results[0].Volume.Read
		upserter := results[0].Volume.Upsert

		if sourceVolume == nil {
			Logc(ctx).WithFields(LogFields{
				"subordinateVolume": subVol.Config.Name,
				"sourceVolume":      subVol.Config.ShareSourceVolume,
				"handler":           "Bootstrap",
			}).Warning("Source volume for subordinate volume not found.")

			unlocker()
			continue
		}

		// Make the source volume point to the subordinate
		if sourceVolume.Config.SubordinateVolumes == nil {
			sourceVolume.Config.SubordinateVolumes = make(map[string]interface{})
		}
		sourceVolume.Config.SubordinateVolumes[subVol.Config.Name] = nil

		upserter(sourceVolume)
		unlocker()
	}

	return nil
}

// cleanupDeletingBackends cleans up any deleting backends that lack volumes.  This can happen if
// a connection to the store fails when attempting to delete a backend.  This function is designed to
// run only during bootstrapping.
func (o *ConcurrentTridentOrchestrator) cleanupDeletingBackends(ctx context.Context) {
	results, unlocker, err := db.Lock(ctx, db.Query(db.ListBackends()))
	unlocker()
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to list backends.")
		return
	}

	var deletingBackends []string

	// Build list of deleting backends that lack volumes.
	for _, backend := range results[0].Backends {
		if !backend.State().IsFailed() && !backend.HasVolumes() &&
			(backend.State().IsDeleting() || o.storeClient.IsBackendDeleting(ctx, backend)) {
			deletingBackends = append(deletingBackends, backend.BackendUUID())
		}
	}

	for _, backendUUID := range deletingBackends {
		results, unlocker, err = db.Lock(ctx, db.Query(db.DeleteBackend(backendUUID)))
		if err != nil {
			Logc(ctx).WithField("backendUUID", backendUUID).WithError(err).Error("Failed to lock deleting backend.")
			unlocker()
			continue
		}

		backend := results[0].Backend.Read
		deleter := results[0].Backend.Delete

		if backend == nil {
			Logc(ctx).WithField("backendUUID", backendUUID).Debug("Backend not found.")
			unlocker()
			continue
		}

		if err = o.storeClient.DeleteBackend(ctx, backend); err != nil {
			Logc(ctx).WithField("backendUUID", backendUUID).WithError(err).Error("Failed to delete deleting backend.")
			unlocker()
			continue
		}

		backend.Terminate(ctx)
		deleter()
		unlocker()
	}
}

// Stop stops the orchestrator core; this is expected to be called during shutdown,
// and new cache locks will block forever.
func (o *ConcurrentTridentOrchestrator) Stop() {
	// Stop the node access and backends' state reconciliation background tasks
	if o.stopNodeAccessLoop != nil {
		o.stopNodeAccessLoop <- true
	}
	if o.stopReconcileBackendLoop != nil {
		o.stopReconcileBackendLoop <- true
	}

	_, _, err := db.Lock(context.Background(), db.Query(db.LockCache()))
	if err != nil {
		Log().WithError(err).Error("Failed to lock cache during orchestrator stop, stopping immediately.")
		return
	}
	Log().Info("Stopped concurrent Trident orchestrator.")
}

// validateAndCreateBackendFromConfig validates config and creates backend based on Config
func (o *ConcurrentTridentOrchestrator) validateAndCreateBackendFromConfig(
	ctx context.Context, configJSON, configRef, backendUUID string,
) (storage.Backend, error) {
	var backendSecret map[string]string

	commonConfig, configInJSON, err := factory.ValidateCommonSettings(ctx, configJSON)
	if err != nil {
		return nil, err
	}

	commonConfig.Flags[FlagConcurrent] = "true"

	if !collection.StringInSlice(commonConfig.StorageDriverName, config.ConcurrentBackends) {
		return nil, fmt.Errorf("backend type %s is not yet supported by concurrent Trident",
			commonConfig.StorageDriverName)
	}

	// For backends created using CRD Controller ensure there are no forbidden fields
	if isCRDContext(ctx) {
		if err = factory.SpecOnlyValidation(ctx, commonConfig, configInJSON); err != nil {
			return nil, errors.WrapUnsupportedConfigError(err)
		}
	}

	// If Credentials are set, fetch them and set them in the configJSON matching field names
	if len(commonConfig.Credentials) != 0 {
		secretName, secretType, secretErr := commonConfig.GetCredentials()
		if secretErr != nil {
			return nil, secretErr
		} else if secretName == "" {
			return nil, fmt.Errorf("credentials `name` field cannot be empty")
		}

		// Handle known secret store types here, but driver-specific ones may be handled in the drivers.
		if secretType == string(drivers.CredentialStoreK8sSecret) {
			if backendSecret, err = o.storeClient.GetBackendSecret(ctx, secretName); err != nil {
				return nil, err
			} else if backendSecret == nil {
				return nil, fmt.Errorf("backend credentials not found")
			}
		}
	}

	sb, err := factory.NewStorageBackendForConfig(ctx, configInJSON, configRef, backendUUID, commonConfig, backendSecret)

	if commonConfig.UserState != "" {
		// If the userState field is present in tbc/backend.json, then update the userBackendState.
		if err = o.updateUserBackendState(ctx, &sb, commonConfig.UserState, false); err != nil {
			return nil, err
		}
	}

	return sb, err
}

// updateUserBackendState sets the user state on an existing backend.  The caller is expected
// to obtain a cache lock, pass the locked backend reference in sb, and handle upserting,
// unlocking and any cache errors.
func (o *ConcurrentTridentOrchestrator) updateUserBackendState(
	ctx context.Context, sb *storage.Backend, userBackendState string, isCLI bool,
) (err error) {
	backend := *sb
	Logc(ctx).WithFields(LogFields{
		"backendName":      backend.Name(),
		"userBackendState": userBackendState,
	}).Debug("updateUserBackendState")

	// There are primarily two methods for creating a backend:
	// 1. Backend is either created via tbc or linked to tbc, then there can be two scenarios:
	//     A. If the userState field is present in the config section of the tbc,
	//        then updating via tridentctl is not allowed.
	//	   B. If the userState field is not present in the config section of the tbc,
	//	      then updating via tridentctl is allowed.
	// 2. Backend is created via tridentctl, using backend.json:
	//     A. It doesn't matter if the userState field is present in the backend.json or not,
	//        updating via tridentctl is allowed.
	if isCLI {
		commonConfig := backend.Driver().GetCommonConfig(ctx)
		if commonConfig.UserState != "" {
			if backend.ConfigRef() != "" {
				return fmt.Errorf("updating via tridentctl is not allowed when `userState` " +
					"field is set in the tbc of the backend")
			} else {
				// If the userState has been updated via tridentctl,
				//    then in the config section of tbe, userState will be shown empty.
				// We will only come to this section of the code when a backend is created via tridentctl using backend.json,
				//    and hasn't been linked to any of the tbc yet.
				commonConfig.UserState = ""
			}
		}
	}

	newUserBackendState := storage.UserBackendState(strings.ToLower(userBackendState))

	// An extra check to ensure that the user-backend state is valid.
	if err = newUserBackendState.Validate(); err != nil {
		return fmt.Errorf("invalid user backend state provided: %s, allowed are: `%s`, `%s`",
			string(newUserBackendState), storage.UserNormal, storage.UserSuspended)
	}

	// Idempotent check.
	if backend.UserState() == newUserBackendState {
		return nil
	}

	// If the user requested for the backend to be suspended.
	if newUserBackendState.IsSuspended() {
		// Backend is only suspended when its current state is either online, offline or failed.
		if !backend.State().IsOnline() && !backend.State().IsOffline() && !backend.State().IsFailed() {
			return fmt.Errorf("the backend '%s' is currently not in any of the expected states: offline, "+
				"online, or failed. Its current state is '%s'", backend.Name(),
				backend.State())
		}
	}

	// Update the user-backend state.
	backend.SetUserState(newUserBackendState)

	return nil
}

// reconcileNodeAccessOnAllBackends updates node access on backends based on current
// volume publications.  Only backends that need reconciliation are affected.
func (o *ConcurrentTridentOrchestrator) reconcileNodeAccessOnAllBackends(ctx context.Context) error {
	if config.CurrentDriverContext != config.ContextCSI {
		return nil
	}

	Logc(ctx).Debug("Reconciling node access on current backends.")

	allBackends, err := func() ([]storage.Backend, error) {
		results, unlocker, err := db.Lock(ctx, db.Query(db.ListBackends()))
		defer unlocker()
		if err != nil {
			return nil, err
		}
		return results[0].Backends, nil
	}()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(len(allBackends))
	var reconcileErrors error

	for _, b := range allBackends {
		backend := b

		go func(backend storage.Backend) {
			defer wg.Done()

			// Check if reconcile is needed to avoid taking backend write lock unnecessarily
			nodeAccessUpToDate := func() bool {
				results, unlocker, dbErr := db.Lock(ctx, db.Query(db.ReadBackend(backend.BackendUUID())))
				defer unlocker()
				if dbErr != nil {
					Logc(ctx).WithError(dbErr).WithField("backend", backend.BackendUUID()).Warning(
						"Could not lock backend for reconcile check.")
					return false
				}

				readBackend := results[0].Backend.Read
				if readBackend == nil {
					Logc(ctx).WithField("backend", backend.BackendUUID()).Warning(
						"Could not find backend for reconcile check.")
					return false
				}

				return readBackend.IsNodeAccessUpToDate()
			}()

			// Return early if no reconcile is needed
			if nodeAccessUpToDate {
				Logc(ctx).WithField("backend", backend.BackendUUID()).Debug(
					"Backend node access up to date, skipping reconcile.")
				return
			}

			// Now we know a reconcile is needed, so take the write lock and do it.  If an error occurs,
			// just log it and continue.
			err = func() error {
				results, unlocker, dbErr := db.Lock(ctx, db.Query(
					db.ListVolumePublications(), db.ListNodes(), db.UpsertBackend(backend.BackendUUID(), "", "")))
				defer unlocker()
				if dbErr != nil {
					return dbErr
				}

				upsertBackend := results[0].Backend.Read
				if upsertBackend == nil {
					return errors.NotFoundError("backend %s not found for reconcile", backend.BackendUUID())
				}

				if reconcileErr := o.reconcileNodeAccessOnBackend(
					ctx, upsertBackend, results[0].VolumePublications, results[0].Nodes); reconcileErr != nil {
					return reconcileErr
				}
				results[0].Backend.Upsert(upsertBackend)
				return nil
			}()
			if err != nil {
				reconcileErrors = multierr.Append(reconcileErrors, err)
				Logc(ctx).WithError(err).WithField("backend", backend.Name()).Warn(
					"Error during node access reconciliation.")
			}
		}(backend)
	}

	wg.Wait()

	return reconcileErrors
}

func (o *ConcurrentTridentOrchestrator) reconcileNodeAccessOnBackend(ctx context.Context, b storage.Backend,
	allVolumePublications []*models.VolumePublication, allNodes []*models.Node,
) error {
	if config.CurrentDriverContext != config.ContextCSI {
		return nil
	}

	var nodes []*models.Node

	if b.CanEnablePublishEnforcement() {
		nodes = publishedNodesForBackend(b, allVolumePublications, allNodes)
	} else {
		nodes = allNodes
	}

	if err := b.ReconcileNodeAccess(ctx, nodes, o.uuid); err != nil {
		return err
	}

	b.SetNodeAccessUpToDate()
	return nil
}

// invalidateAllBackendNodeAccess sets nodeAccessUpToDate to false on all backends.  That value is
// protected from concurrent modification, so under the principle of interior mutability, a backend
// read lock is sufficient here.
func (o *ConcurrentTridentOrchestrator) invalidateAllBackendNodeAccess(ctx context.Context) error {
	results, unlocker, dbErr := db.Lock(ctx, db.Query(db.ListBackends()))
	unlocker()
	if dbErr != nil {
		return dbErr
	}
	backends := results[0].Backends

	var backendErrors error

	for _, b := range backends {
		backendErr := func() error {
			results, unlocker, dbErr = db.Lock(ctx, db.Query(db.ReadBackend(b.BackendUUID())))
			defer unlocker()
			if dbErr != nil {
				return dbErr
			}

			backend := results[0].Backend.Read
			if backend == nil {
				return errors.NotFoundError("backend %s not found, skipping node access invalidation", b.BackendUUID())
			}

			backend.InvalidateNodeAccess()
			return nil
		}()

		backendErrors = multierr.Append(backendErrors, backendErr)
	}

	return backendErrors
}

// getLastNodeRegistrationTime returns the latest time a node was added.
func (o *ConcurrentTridentOrchestrator) getLastNodeRegistrationTime() time.Time {
	o.lastNodeRegistrationMutex.RLock()
	defer o.lastNodeRegistrationMutex.RUnlock()
	return o.lastNodeRegistrationTime
}

// updateLastNodeRegistrationTime updates the latest node registration time to the current time.
func (o *ConcurrentTridentOrchestrator) updateLastNodeRegistrationTime() {
	o.lastNodeRegistrationMutex.Lock()
	defer o.lastNodeRegistrationMutex.Unlock()
	o.lastNodeRegistrationTime = time.Now()
}

// publishedNodesForBackend returns the nodes that a backend has published volumes to
func publishedNodesForBackend(b storage.Backend, allVolumePublications []*models.VolumePublication,
	allNodes []*models.Node,
) []*models.Node {
	nodesByName := make(map[string]*models.Node, len(allNodes))
	for _, n := range allNodes {
		nodesByName[n.Name] = n
	}

	volumes := b.Volumes()
	m := make(map[string]*models.Node)
	for _, pub := range allVolumePublications {
		if _, ok := volumes.Load(pub.VolumeName); ok {
			m[pub.NodeName] = nodesByName[pub.NodeName]
		}
	}

	nodes := make([]*models.Node, 0, len(m))
	for _, n := range m {
		nodes = append(nodes, n)
	}
	return nodes
}

func (o *ConcurrentTridentOrchestrator) AddFrontend(ctx context.Context, f frontend.Plugin) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	name := f.GetName()
	if _, ok := o.frontends[name]; ok {
		Logc(ctx).WithField("name", name).Warn("Adding frontend already present.")
		return
	}
	Logc(ctx).WithField("name", name).Info("Added frontend.")
	o.frontends[name] = f
}

func (o *ConcurrentTridentOrchestrator) GetFrontend(ctx context.Context, name string) (frontend.Plugin, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	fe, ok := o.frontends[name]
	if !ok {
		err := fmt.Errorf("requested frontend %s does not exist", name)
		return nil, err
	}

	Logc(ctx).WithField("name", name).Debug("Found requested frontend.")
	return fe, nil
}

func (o *ConcurrentTridentOrchestrator) GetVersion(ctx context.Context) (string, error) {
	return config.OrchestratorVersion.String(), o.bootstrapError
}

func (o *ConcurrentTridentOrchestrator) AddBackend(
	ctx context.Context, configJSON, configRef string,
) (backendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_add", &err)()

	newBackendName, err := factory.ParseBackendName(configJSON)
	if err != nil {
		return nil, err
	}

	if newBackendName == "" {
		return nil, fmt.Errorf("backend name cannot be empty")
	}

	results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertBackend("", newBackendName, "")))
	defer unlocker()
	if err != nil {
		return nil, err
	}

	backend, err := o.upsertBackend(ctx, configJSON, results[0], configRef)
	if err != nil {
		Logc(ctx).WithError(err).WithFields(LogFields{
			"backendName": newBackendName,
			"configRef":   configRef,
		}).Debug("AddBackend failed.")
		return nil, err
	}

	return backend.ConstructExternalWithPoolMap(ctx,
		o.GetStorageClassPoolMap().StorageClassNamesForBackendName(ctx, backend.Name())), nil
}

func (o *ConcurrentTridentOrchestrator) DeleteBackend(ctx context.Context, backendName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		Logc(ctx).WithFields(LogFields{
			"bootstrapError": o.bootstrapError,
		}).Warn("DeleteBackend error")
		return o.bootstrapError
	}

	defer recordTiming("backend_delete", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.ReadBackendByName(backendName)))
	if err != nil {
		unlocker()
		if errors.IsNotFoundError(err) {
			err = errors.NotFoundError("backend %v was not found", backendName)
		}
		return err
	}
	backend := results[0].Backend.Read
	unlocker()

	if backend == nil {
		return errors.NotFoundError("backend %v was not found", backendName)
	}

	return o.deleteBackendByBackendUUID(ctx, backendName, backend.BackendUUID())
}

func (o *ConcurrentTridentOrchestrator) DeleteBackendByBackendUUID(
	ctx context.Context, backendName, backendUUID string,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		Logc(ctx).WithFields(LogFields{
			"bootstrapError": o.bootstrapError,
		}).Warn("DeleteBackend error")
		return o.bootstrapError
	}

	defer recordTiming("backend_delete", &err)()

	return o.deleteBackendByBackendUUID(ctx, backendName, backendUUID)
}

func (o *ConcurrentTridentOrchestrator) deleteBackendByBackendUUID(
	ctx context.Context, backendName, backendUUID string,
) error {
	Logc(ctx).WithFields(LogFields{
		"backendName": backendName,
		"backendUUID": backendUUID,
	}).Debug("deleteBackendByBackendUUID")

	results, unlocker, err := db.Lock(
		ctx,
		db.Query(db.DeleteBackend(backendUUID)),
		db.Query(db.UpsertBackend(backendUUID, "", "")),
	)
	defer unlocker()
	if err != nil {
		return err
	}

	backend := results[0].Backend.Read
	if backend == nil {
		return errors.NotFoundError("backend %v was not found", backendName)
	}

	// Do not allow deletion of TridentBackendConfig-based backends using tridentctl
	if backend.ConfigRef() != "" {
		if !isCRDContext(ctx) {
			Logc(ctx).WithFields(LogFields{
				"backendName": backendName,
				"backendUUID": backendUUID,
				"configRef":   backend.ConfigRef(),
			}).Error("Cannot delete backend created using TridentBackendConfig CR; delete the TridentBackendConfig" +
				" CR first.")

			return fmt.Errorf("cannot delete backend '%v' created using TridentBackendConfig CR; delete the"+
				" TridentBackendConfig CR first", backendName)
		}
	}

	if !backend.HasVolumes() {

		// Terminate the backend & driver
		backend.Terminate(ctx)

		// Delete backend from the cache
		results[0].Backend.Delete()

		// Update storage class to pool map
		o.RebuildStorageClassPoolMap(ctx)

		// Delete backend from the persistent store
		return o.storeClient.DeleteBackend(ctx, backend)
	}

	Logc(ctx).WithFields(LogFields{
		"backend":        backend,
		"backend.Name":   backend.Name(),
		"backend.State":  backend.State().String(),
		"backend.Online": backend.Online(),
	}).Debug("OfflineBackend information.")

	// Prepare cache for backend update
	backend.SetOnline(false)
	backend.SetState(storage.Deleting)

	// Update the backend in the cache
	results[1].Backend.Upsert(backend)

	// Update storage class to pool map
	o.RebuildStorageClassPoolMap(ctx)

	// Update backend on the persistent store
	return o.storeClient.UpdateBackend(ctx, backend)
}

func (o *ConcurrentTridentOrchestrator) GetBackend(
	ctx context.Context, backendName string,
) (backendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_get", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.ReadBackendByName(backendName)))
	defer unlocker()
	if err != nil {
		if strings.Contains(err.Error(), "no Backend found with key") {
			err = errors.NotFoundError("backend %v was not found", backendName)
		}
		return nil, err
	}

	backend := results[0].Backend.Read
	if backend == nil {
		return nil, errors.NotFoundError("backend %v was not found", backendName)
	}

	backendExternal = backend.ConstructExternalWithPoolMap(ctx,
		o.GetStorageClassPoolMap().StorageClassNamesForBackendName(ctx, backend.Name()))

	Logc(ctx).WithFields(LogFields{
		"backend":               backend,
		"backendUUID":           backend.BackendUUID(),
		"backendExternal":       backendExternal,
		"backendExternal.Name":  backendExternal.Name,
		"backendExternal.State": backendExternal.State.String(),
	}).Trace("GetBackend information.")
	return backendExternal, nil
}

func (o *ConcurrentTridentOrchestrator) GetBackendByBackendUUID(
	ctx context.Context, backendUUID string,
) (backendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_get", &err)()

	backend, err := getInconsistentBackendByUUID(backendUUID)
	if err != nil {
		return nil, err
	}

	backendExternal = backend.ConstructExternalWithPoolMap(ctx,
		o.GetStorageClassPoolMap().StorageClassNamesForBackendName(ctx, backend.Name()))

	Logc(ctx).WithFields(LogFields{
		"backend":               backend,
		"backendUUID":           backendUUID,
		"backendExternal":       backendExternal,
		"backendExternal.Name":  backendExternal.Name,
		"backendExternal.State": backendExternal.State.String(),
	}).Trace("GetBackend information.")
	return backendExternal, nil
}

// GetResizeDeltaForBackend returns the resize delta (bytes) for the backend.
// Returns (0, err) when the orchestrator is not ready (e.g. bootstrap error).
func (o *ConcurrentTridentOrchestrator) GetResizeDeltaForBackend(ctx context.Context, backendUUID string) (int64, error) {
	if o.bootstrapError != nil {
		return 0, o.bootstrapError
	}
	backend, err := getInconsistentBackendByUUID(backendUUID)
	if err != nil {
		return 0, nil
	}
	return backend.GetResizeDeltaBytes(), nil
}

// NOTE: DO NOT USE THIS FUNCTION INSIDE ANOTHER LOCK CONTEXT
func getInconsistentBackendByUUID(
	backendUUID string,
) (storage.Backend, error) {
	results, unlocker, err := db.Lock(context.Background(), db.Query(db.InconsistentReadBackend(backendUUID)))
	defer unlocker()
	if err != nil {
		return nil, err
	}

	backend := results[0].Backend.Read
	if backend == nil {
		return nil, errors.NotFoundError("backend with UUID %v was not found", backendUUID)
	}

	return backend, nil
}

func (o *ConcurrentTridentOrchestrator) ListBackends(
	ctx context.Context,
) (backendExternals []*storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		Logc(ctx).WithFields(LogFields{
			"bootstrapError": o.bootstrapError,
		}).Warn("ListBackends error")
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_list", &err)()

	poolMap := o.GetStorageClassPoolMap()
	results, unlocker, err := db.Lock(ctx, db.Query(db.ListBackends()))
	defer unlocker()
	if err != nil {
		return nil, err
	}

	backends := results[0].Backends
	backendExternals = make([]*storage.BackendExternal, 0, len(backends))
	for _, b := range backends {
		backendExternal := b.ConstructExternalWithPoolMap(
			ctx, poolMap.StorageClassNamesForBackendName(ctx, b.Name()))
		backendExternals = append(backendExternals, backendExternal)
	}

	return backendExternals, nil
}

// UpdateBackend updates an existing backend.
func (o *ConcurrentTridentOrchestrator) UpdateBackend(
	ctx context.Context, backendName, configJSON, configRef string,
) (backendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_update", &err)()

	newBackendName, err := o.checkForBackendNameChange(configJSON, backendName)
	if err != nil {
		return nil, err
	}

	results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertBackend("", backendName, newBackendName)))
	if err != nil {
		unlocker()
		if strings.Contains(err.Error(), "no Backend found with key") {
			err = errors.NotFoundError("backend %v was not found", backendName)
		}
		return nil, err
	}

	if results[0].Backend.Read == nil {
		unlocker()
		return nil, errors.NotFoundError("backend %v was not found", backendName)
	}

	backend, err := o.upsertBackend(ctx, configJSON, results[0], configRef)
	unlocker()
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"err":         err.Error(),
			"backendName": backendName,
			"configRef":   configRef,
		}).Error("UpdateBackend failed.")
		return nil, err
	}

	// TODO: Ideally we would like to keep holding the backend write lock and acquire the volume locks. But currently,
	// the locking mechanism does not support sub-locks. We have to revisit this code when we implement sub-locks.
	err = o.updateBackendVolumes(ctx, backend)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"err":         err.Error(),
			"backendName": backend.Name(),
			"configRef":   configRef,
		}).Warn("Update of backend volumes failed.")
	}

	return backend.ConstructExternalWithPoolMap(ctx,
		o.GetStorageClassPoolMap().StorageClassNamesForBackendName(ctx, backend.Name())), nil
}

// UpdateBackendByBackendUUID updates an existing backend.
func (o *ConcurrentTridentOrchestrator) UpdateBackendByBackendUUID(
	ctx context.Context, backendName, configJSON, backendUUID, configRef string,
) (backendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_update", &err)()

	newBackendName, err := o.checkForBackendNameChange(configJSON, backendName)
	if err != nil {
		return nil, err
	}

	results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertBackend(backendUUID, "", newBackendName)))
	if err != nil {
		unlocker()
		if strings.Contains(err.Error(), "no Backend found with key") {
			err = errors.NotFoundError("backend %v was not found", backendName)
		}
		return nil, err
	}

	if results[0].Backend.Read == nil {
		unlocker()
		return nil, errors.NotFoundError("backend %v was not found", backendName)
	}

	backend, err := o.upsertBackend(ctx, configJSON, results[0], configRef)
	unlocker()
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"backendName": backendName,
			"configRef":   configRef,
		}).WithError(err).Error("UpdateBackend failed.")
		return nil, err
	}

	// TODO: Ideally we would like to keep holding the backend write lock we acquired in upsertBackend() and call
	//  updateBackendVolumes(). But currently, the locking mechanism does not support sub-locks. We have to revisit
	//  this code if and when we implement sub-locks.
	err = o.updateBackendVolumes(ctx, backend)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"err":         err.Error(),
			"backendName": backend.Name(),
			"configRef":   configRef,
		}).Warn("Update of backend volumes failed.")
	}

	return backend.ConstructExternalWithPoolMap(ctx,
		o.GetStorageClassPoolMap().StorageClassNamesForBackendName(ctx, backend.Name())), nil
}

// upsertBackend updates an existing backend.  The caller is expected to obtain a cache lock,
// pass that cache result in upsertResult, and handle unlocking and any cache errors.  Any call
// to the cache result upsert function is done here.  Since a lock is held by the caller, only
// lockless (inconsistent) cache reads may be performed in this function.
func (o *ConcurrentTridentOrchestrator) upsertBackend(
	ctx context.Context, configJSON string, upsertResult db.Result, callingConfigRef string,
) (storage.Backend, error) {
	var backend storage.Backend

	Logc(ctx).Debug(">>>>>> upsertBackend")
	defer Logc(ctx).Debug("<<<<<< upsertBackend")

	results, unlocker, err := db.Lock(ctx, db.Query(db.ListVolumePublications(), db.ListNodes()))
	defer unlocker()
	if err != nil {
		return nil, err
	}

	originalBackend := upsertResult.Backend.Read

	if originalBackend == nil {
		backend, err = o.addBackend(ctx, configJSON, uuid.New().String(), callingConfigRef)
		if err != nil {
			if backend != nil && backend.State().IsFailed() {
				return backend, err
			}
			return nil, err
		}
	} else {
		backend, err = o.updateBackend(ctx, configJSON, originalBackend, callingConfigRef)
		if err != nil {
			return nil, err
		}
	}

	// Node access rules may have changed in the backend config
	backend.InvalidateNodeAccess()
	err = o.reconcileNodeAccessOnBackend(ctx, backend, results[0].VolumePublications, results[0].Nodes)
	if err != nil {
		return nil, err
	}

	// For update backend request, terminate the old backend and update the volumes in backend
	if originalBackend != nil {
		originalBackend.Terminate(ctx)
	}

	// Update the backend in the cache
	upsertResult.Backend.Upsert(backend)

	// Update storage class to pool map
	o.RebuildStorageClassPoolMap(ctx)

	Logc(ctx).WithField("backend", backend).Debug("Backend upserted.")

	return backend, nil
}

func (o *ConcurrentTridentOrchestrator) addBackend(
	ctx context.Context, configJSON, backendUUID, configRef string,
) (storage.Backend, error) {
	var backend storage.Backend
	var err error

	defer func() {
		if backend != nil && err != nil {
			backend.Terminate(ctx)
		}
	}()

	backend, err = o.validateAndCreateBackendFromConfig(ctx, configJSON, configRef, backendUUID)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"err":         err.Error(),
			"backend":     backend,
			"backendUUID": backendUUID,
			"configRef":   configRef,
		}).Debug("NewStorageBackendForConfig failed.")

		if backend != nil && backend.State().IsFailed() {
			return backend, err
		}
		return nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"backend":             backend.Name(),
		"backend.BackendUUID": backend.BackendUUID(),
		"backend.ConfigRef":   backend.ConfigRef(),
	}).Debug("Adding a new backend.")

	// Update persistent store
	if err = o.updateBackendOnPersistentStore(ctx, backend, true); err != nil {
		return nil, err
	}

	return backend, nil
}

func (o *ConcurrentTridentOrchestrator) updateBackend(
	ctx context.Context, configJSON string, originalBackend storage.Backend, callingConfigRef string,
) (storage.Backend, error) {
	var backend storage.Backend
	backendName := originalBackend.Name()
	backendUUID := originalBackend.BackendUUID()

	logFields := LogFields{"existingBackendName": backendName, "backendUUID": backendUUID, "configJSON": "<suppressed>"}

	Logc(ctx).WithFields(LogFields{
		"originalBackend.Name":        originalBackend.Name(),
		"originalBackend.BackendUUID": originalBackend.BackendUUID(),
		"originalBackend.ConfigRef":   originalBackend.ConfigRef(),
		"GetExternalConfig":           originalBackend.Driver().GetExternalConfig(ctx),
	}).Debug("found original backend")

	originalConfigRef := originalBackend.ConfigRef()

	// Do not allow update of TridentBackendConfig-based backends using tridentctl
	if originalConfigRef != "" {
		if !isCRDContext(ctx) && !isPeriodicContext(ctx) {
			Logc(ctx).WithFields(LogFields{
				"existingBackendName": backendName,
				"backendUUID":         backendUUID,
				"configRef":           originalConfigRef,
			}).Error("Cannot update backend created using TridentBackendConfig CR; please update the" +
				" TridentBackendConfig CR instead.")

			return nil, fmt.Errorf("cannot update backend '%v' created using TridentBackendConfig CR; "+
				"please update the TridentBackendConfig CR", backendName)
		}
	}

	// If originalBackend.ConfigRef happens to be empty, we should assign callingConfigRef to
	// originalBackend.ConfigRef as part of linking legacy tridentctl backend with TridentBackendConfig-based backend
	if originalConfigRef != callingConfigRef {
		ctxSource := ctx.Value(ContextKeyRequestSource)
		if originalConfigRef == "" && ctxSource != nil && ctxSource == ContextSourceCRD {
			Logc(ctx).WithFields(LogFields{
				"existingBackendName":     originalBackend.Name(),
				"backendUUID":             originalBackend.BackendUUID(),
				"originalConfigRef":       originalConfigRef,
				"TridentBackendConfigUID": callingConfigRef,
			}).Tracef("Backend is not bound to any Trident Backend Config, attempting to bind it.")
		} else {
			Logc(ctx).WithFields(LogFields{
				"existingBackendName": originalBackend.Name(),
				"backendUUID":         originalBackend.BackendUUID(),
				"originalConfigRef":   originalConfigRef,
				"invalidConfigRef":    callingConfigRef,
			}).Errorf("Backend update initiated using an invalid ConfigRef.")
			return nil, errors.UnsupportedConfigError(
				"backend '%v' update initiated using an invalid configRef, it is associated with configRef "+
					"'%v' and not '%v'", originalBackend.Name(), originalConfigRef, callingConfigRef)
		}
	}

	var err error

	defer func() {
		Logc(ctx).WithFields(logFields).Debug("<<<<<< updateBackend")
		if backend != nil && err != nil {
			backend.Terminate(ctx)
		}
	}()

	Logc(ctx).WithFields(logFields).Debug(">>>>>> updateBackend")

	// Second, validate the update.
	backend, err = o.validateAndCreateBackendFromConfig(ctx, configJSON, callingConfigRef, backendUUID)
	if err != nil {
		return nil, err
	}

	// We're updating a backend, there can be two scenarios (related to userState):
	// 1. userState field is not present in the tbc/backend.json,
	//       so we should set it to whatever it was before.
	// 2. userState field is present in the tbc/backend.json, then we've already set it
	//       when we called validateAndCreateBackendFromConfig() above.
	if backend.Driver().GetCommonConfig(ctx).UserState == "" {
		backend.SetUserState(originalBackend.UserState())
	}

	if err = o.validateBackendUpdate(originalBackend, backend); err != nil {
		return nil, err
	}
	Logc(ctx).WithFields(LogFields{
		"originalBackend.Name":        originalBackend.Name(),
		"originalBackend.BackendUUID": originalBackend.BackendUUID(),
		"backend":                     backend.Name(),
		"backend.BackendUUID":         backend.BackendUUID(),
		"backendUUID":                 backendUUID,
	}).Trace("Updating an existing backend.")

	// Third, determine what type of backend update we're dealing with.
	// Here are the major categories and their implications:
	// 1) Backend rename
	//    a) Affects in-memory backend, storage class, and volume objects
	//    b) Affects backend and volume objects in the persistent store
	// 2) Change in the data plane IP address
	//    a) Affects in-memory backend and volume objects
	//    b) Affects backend and volume objects in the persistent store
	// 3) Updates to fields other than the name and IP address
	//    This scenario is the same as the AddBackend
	// 4) Some combination of above scenarios
	updateCode := backend.GetUpdateType(ctx, originalBackend)
	switch {
	case updateCode.Contains(storage.InvalidUpdate):
		err = errors.New("invalid backend update")
		Logc(ctx).WithField("error", err).Error("Backend update failed.")
		return nil, err
	case updateCode.Contains(storage.InvalidVolumeAccessInfoChange):
		err = errors.New("updating the data plane IP address isn't currently supported")
		Logc(ctx).WithField("error", err).Error("Backend update failed.")
		return nil, err
	case updateCode.Contains(storage.PrefixChange):
		err = errors.UnsupportedConfigError("updating the storage prefix isn't currently supported")
		Logc(ctx).WithField("error", err).Error("Backend update failed.")
		return nil, err
	case updateCode.Contains(storage.BackendRename):
		if err = o.storeClient.ReplaceBackendAndUpdateVolumes(ctx, originalBackend, backend); err != nil {
			Logc(ctx).WithField("error", err).Errorf(
				"Could not rename backend from %v to %v", originalBackend.Name(), backend.Name())
			return nil, err
		}
	default:
		// Update backend information
		if err = o.updateBackendOnPersistentStore(ctx, backend, false); err != nil {
			Logc(ctx).WithField("error", err).Errorf("Could not persist renamed backend from %v to %v",
				originalBackend.Name(), backend.Name())
			return nil, err
		}
	}

	// The fake driver needs volumes copied forward
	if originalFakeDriver, ok := originalBackend.Driver().(*fake.StorageDriver); ok {
		Logc(ctx).Debug("Using fake driver, going to copy volumes forward...")
		if fakeDriver, ok := backend.Driver().(*fake.StorageDriver); ok {
			fakeDriver.CopyVolumes(originalFakeDriver.Volumes)
			Logc(ctx).Debug("Copied volumes forward.")
		}
	}

	return backend, nil
}

func (o *ConcurrentTridentOrchestrator) checkForBackendNameChange(configJSON, backendName string) (string, error) {
	parsedBackendName, err := factory.ParseBackendName(configJSON)
	if err != nil {
		return "", err
	}

	newBackendName := ""
	if parsedBackendName != "" && parsedBackendName != backendName {
		// This is a backend name change
		newBackendName = parsedBackendName
	}

	return newBackendName, nil
}

func (o *ConcurrentTridentOrchestrator) updateBackendVolumes(ctx context.Context, backend storage.Backend) error {
	// Update the volume state in memory
	// Identify orphaned volumes (i.e., volumes that are not present on the
	// new backend). Such a scenario can happen if a subset of volumes are
	// replicated for DR or volumes get deleted out of band. Operations on
	// such volumes are likely to fail, so here we just warn the users about
	// such volumes and mark them as orphaned. This is a best effort activity,
	// so it doesn't have to be part of the persistent store transaction.

	// Get a list of volumes in the backend
	results, unlocker, err := db.Lock(ctx, db.Query(db.ListVolumesForBackend(backend.BackendUUID())))
	if err != nil {
		unlocker()
		return err
	}
	volumes := results[0].Volumes
	unlocker()

	lockQuery := make([][]db.Subquery, 0, len(volumes))

	backendUUID := backend.BackendUUID()
	// Acquire the write lock for the backend and write lock for all the volumes in the backend in a single lock call.
	lockQuery = append(lockQuery, db.Query(db.UpsertBackend(backendUUID, "", "")))
	for _, vol := range volumes {
		lockQuery = append(lockQuery, db.Query(db.UpsertVolume(vol.Config.Name, vol.BackendUUID)))
	}
	results, unlocker, err = db.Lock(ctx, lockQuery...)
	defer unlocker()
	if err != nil {
		return err
	}

	backendResult := results[0]
	backend = backendResult.Backend.Read
	if backend == nil {
		Logc(ctx).WithFields(LogFields{
			"backend": backendUUID,
		}).Warn("Backend not found.")

		// this is best case effort. so return nil
		return nil
	}

	// loop from 1 to len(results) - 1 to skip the first result which is the backend
	for i := 1; i < len(results); i++ {
		volResult := results[i]
		vol := volResult.Volume.Read
		if vol == nil {
			// This can happen if the volume was deleted after we queried the list of volumes
			continue
		}

		vol.BackendUUID = backend.BackendUUID()
		updatePersistentStore := false
		volumeExists := backend.Driver().Get(ctx, vol.Config) == nil
		if !volumeExists {
			if !vol.Orphaned {
				vol.Orphaned = true
				updatePersistentStore = true
				Logc(ctx).WithFields(LogFields{
					"volume":                  vol.Config.Name,
					"vol.Config.InternalName": vol.Config.InternalName,
					"backend":                 backend.Name(),
				}).Warn("Backend update resulted in an orphaned volume.")
			}
		} else {
			if vol.Orphaned {
				vol.Orphaned = false
				updatePersistentStore = true
				Logc(ctx).WithFields(LogFields{
					"volume":                  vol.Config.Name,
					"vol.Config.InternalName": vol.Config.InternalName,
					"backend":                 backend.Name(),
				}).Debug("The volume is no longer orphaned as a result of the backend update.")
			}
		}
		if updatePersistentStore {
			if err := o.storeClient.UpdateVolume(ctx, vol); err != nil {
				Logc(ctx).WithField("volume", vol.Config.Name).Error(
					"Persistent store update failed for orphan volume.")
				continue
			}
		}

		// Update volume in cache
		volResult.Volume.Upsert(vol)

		// Update volume cache on backend.
		backend.Volumes().Store(vol.Config.Name, vol)
	}

	// update the backend cache
	backendResult.Backend.Upsert(backend)

	return nil
}

func (o *ConcurrentTridentOrchestrator) validateBackendUpdate(oldBackend, newBackend storage.Backend) error {
	// Validate that backend type isn't being changed as backend type has
	// implications for the internal volume names.
	if oldBackend.GetDriverName() != newBackend.GetDriverName() {
		return fmt.Errorf(
			"cannot update the backend as the old backend is of type %s and the new backend is of type %s",
			oldBackend.GetDriverName(), newBackend.GetDriverName())
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) UpdateBackendState(
	ctx context.Context, backendName, backendState, userBackendState string,
) (storageBackendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_update_state", &err)()

	// Extra check to ensure exactly one is set.
	if (backendState == "" && userBackendState == "") || (backendState != "" && userBackendState != "") {
		return nil, fmt.Errorf("exactly one of backendState or userBackendState must be set")
	}

	results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertBackend("", backendName, "")))
	defer unlocker()
	if err != nil {
		return nil, err
	}

	backend := results[0].Backend.Read
	upserter := results[0].Backend.Upsert

	if backend == nil {
		return nil, errors.NotFoundError("backend %v was not found", backendName)
	}

	if userBackendState != "" {
		if err = o.updateUserBackendState(ctx, &backend, userBackendState, true); err != nil {
			return nil, err
		}
	}
	if backendState != "" {
		if err = o.updateBackendState(ctx, &backend, backendState); err != nil {
			return nil, err
		}
	}

	if err = o.storeClient.UpdateBackend(ctx, backend); err != nil {
		return nil, err
	}
	upserter(backend)

	return backend.ConstructExternal(ctx), nil
}

// updateBackendState sets the state on an existing backend.  The caller is expected
// to obtain a cache lock, pass the locked backend reference in sb, and handle upserting,
// unlocking and any cache errors.
func (o *ConcurrentTridentOrchestrator) updateBackendState(
	ctx context.Context, sb *storage.Backend, backendState string,
) (err error) {
	backend := *sb
	Logc(ctx).WithFields(LogFields{
		"backendName":  backend.Name(),
		"backendState": backendState,
	}).Debug("updateBackendState")

	newBackendState := storage.BackendState(strings.ToLower(backendState))

	// Limit the command to Failed
	if !newBackendState.IsFailed() {
		return fmt.Errorf("unsupported backend state: %s", newBackendState)
	}

	if !newBackendState.IsOnline() {
		backend.Terminate(ctx)
	}
	backend.SetState(newBackendState)

	return nil
}

func (o *ConcurrentTridentOrchestrator) RemoveBackendConfigRef(
	ctx context.Context, backendUUID, configRef string,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	defer recordTiming("backend_update", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertBackend(backendUUID, "", "")))
	defer unlocker()
	if err != nil {
		return fmt.Errorf("error locking backend with UUID '%s'; %w", backendUUID, err)
	}

	backend := results[0].Backend.Read
	updateBackendInCache := results[0].Backend.Upsert

	if backend == nil {
		return errors.NotFoundError("backend with UUID '%s' not found", backendUUID)
	}

	backendConfigRef := backend.ConfigRef()

	if backendConfigRef != "" {
		if backendConfigRef != configRef {
			return fmt.Errorf("TridentBackendConfig with UID '%s' cannot request removal of configRef '%s' for backend"+
				" with UUID '%s'", configRef, backendConfigRef, backendUUID)
		}

		backend.SetConfigRef("")

		// update in persistence and cache
		err = o.storeClient.UpdateBackend(ctx, backend)
		if err != nil {
			return fmt.Errorf("failed to remove configRef '%s' from backend with UUID '%s'; %w",
				configRef, backendUUID, err)
		}
		updateBackendInCache(backend)
	}

	return
}

func (o *ConcurrentTridentOrchestrator) AddVolume(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (volExternal *storage.VolumeExternal, err error) {
	ctx = NewContextBuilder(ctx).WithLayer(LogLayerCore).BuildContext()

	if o.bootstrapError != nil {
		Logc(ctx).WithField("bootstrapError", o.bootstrapError).Warn("AddVolume error.")
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_add", &err)()

	volumeConfig.Version = config.OrchestratorAPIVersion

	if volumeConfig.ShareSourceVolume != "" {
		return o.addSubordinateVolume(ctx, volumeConfig)
	}
	return o.addVolume(ctx, volumeConfig)
}

func (o *ConcurrentTridentOrchestrator) addVolume(
	ctx context.Context, volConfig *storage.VolumeConfig,
) (externalVol *storage.VolumeExternal, err error) {
	var (
		poolMap  *storageclass.PoolMap
		sc       *storageclass.StorageClass
		backend  storage.Backend
		vol      *storage.Volume
		volTxn   *storage.VolumeTransaction
		results  []db.Result
		upserter func(volume *storage.Volume)
		unlocker func()
	)

	// Ensure volume doesn't exist
	if err = func(volumeName string) error {
		results, unlocker, err = db.Lock(
			ctx,
			db.Query(db.InconsistentReadVolume(volConfig.Name)),
			db.Query(db.InconsistentReadSubordinateVolume(volConfig.Name)))
		defer unlocker()
		if err != nil {
			return fmt.Errorf("error checking for existing volume; %w", err)
		} else if results[0].Volume.Read != nil {
			return fmt.Errorf("volume %s already exists", volConfig.Name)
		} else if results[1].SubordinateVolume.Read != nil {
			return fmt.Errorf("volume %s exists but is a subordinate volume", volConfig.Name)
		}
		return nil
	}(volConfig.Name); err != nil {
		return nil, err
	}

	// Get the protocol based on the specified access mode & protocol
	protocol, err := getProtocol(ctx, volConfig.VolumeMode, volConfig.AccessMode, volConfig.Protocol)
	if err != nil {
		return nil, err
	}

	// Get an independent scratch copy of the storage class
	results, unlocker, err = db.Lock(ctx, db.Query(db.InconsistentReadStorageClass(volConfig.StorageClass)))
	unlocker()
	if err != nil {
		return nil, err
	}

	sc = results[0].StorageClass.Read
	if sc == nil {
		return nil, fmt.Errorf("unknown storage class: %s", volConfig.StorageClass)
	}

	// Add a transaction to clean out any existing transactions
	volTxn = &storage.VolumeTransaction{
		Config: volConfig,
		Op:     storage.AddVolume,
	}

	// Acquire lock on the transaction name to avoid race condition of multiple concurrent requests handling transactions
	o.txnMutex.Lock(volTxn.Name())
	defer o.txnMutex.Unlock(volTxn.Name())

	if err = o.AddVolumeTransaction(ctx, volTxn); err != nil {
		return nil, err
	}

	// Get matching backend/pool names from storage class pool map
	poolMap = o.GetStorageClassPoolMap()
	matchingBackendPools := poolMap.BackendPoolMapForStorageClass(ctx, volConfig.StorageClass)
	if len(matchingBackendPools) == 0 {
		return nil, fmt.Errorf("no available backends for storage class %s", volConfig.StorageClass)
	}

	// Copy the volume config into a working copy should any backend mutate the config but fail to create the volume.
	mutableConfig := volConfig.ConstructClone()

	// Recovery functions in case of error
	defer func() {
		err = o.addVolumeCleanup(ctx, err, backend, vol, volTxn, unlocker)
	}()

	var volumeCreateErrors error

	// Loop through backends, exhausting pools on one before moving to the next.  This also shuffles the backends.
	for backendName, poolNames := range matchingBackendPools {

		// Get a lock on the backend as well as the volume name
		results, unlocker, err = db.Lock(ctx, db.Query(
			db.UpsertVolumeByBackendName(volConfig.Name, backendName), db.ReadBackendByName(backendName)))
		if err != nil {
			return nil, fmt.Errorf("error locking backend for volume create; %w", err)
		}
		backend = results[0].Backend.Read
		upserter = results[0].Volume.Upsert

		if backend == nil {
			Logc(ctx).WithField("backend", backendName).Debug("Backend not found.")
			unlocker()
			continue
		}

		// Get actual pools from backend
		pools := make(map[string]*storage.Pool)
		backend.StoragePools().Range(func(k, v interface{}) bool {
			pool := v.(storage.Pool)
			pools[k.(string)] = &pool
			return true
		})

		// Filter pools to those matching pool map.  This also shuffles the pools.
		filteredPools := []storage.Pool{}
		for poolName, pool := range pools {
			if collection.ContainsString(poolNames, poolName) {
				filteredPools = append(filteredPools, *pool)
			}
		}

		// Update scratch storage class with pools
		sc.SetPools(filteredPools)

		// Further filter pools by protocol, topology, etc.  This also sorts the pools by preferred topologies.
		filteredPools = sc.GetStoragePoolsForProtocolByBackend(ctx, protocol, volConfig.RequisiteTopologies,
			volConfig.PreferredTopologies, volConfig.AccessMode)
		if len(filteredPools) == 0 {
			Logc(ctx).WithFields(LogFields{
				"backend": backendName,
				"sc":      sc.GetName(),
				"volume":  volConfig.Name,
			}).Debug("No matching pools on backend for volume.")
			unlocker()
			continue
		}

		for _, pool := range filteredPools {

			// Mirror destinations can only be placed on mirroring enabled backends
			if mutableConfig.IsMirrorDestination && !backend.CanMirror() {
				Logc(ctx).Debugf("MirrorDestinations can only be placed on mirroring enabled backends")
				break // Exit pool loop and move on to next backend
			}

			// CreatePrepare has a side effect that updates the mutableConfig with the backend-specific internal name
			backend.CreatePrepare(ctx, mutableConfig, pool)

			// Update transaction with updated mutableConfig
			volTxn = &storage.VolumeTransaction{
				Config: mutableConfig,
				Op:     storage.AddVolume,
			}
			if err = o.storeClient.UpdateVolumeTransaction(ctx, volTxn); err != nil {
				Logc(ctx).Errorf("Error updating volume transaction; %w", err)
				volumeCreateErrors = multierr.Append(volumeCreateErrors, err)
				break // Exit pool loop and move on to next backend
			}

			// Create the volume, waiting as long as necessary for a successful completion
			vol, err = o.addVolumeRetry(ctx, mutableConfig, pool, sc.GetAttributes())
			if err != nil {
				logFields := LogFields{
					"backend":     backend.Name(),
					"backendUUID": backend.BackendUUID(),
					"pool":        pool.Name(),
					"volume":      mutableConfig.Name,
					"error":       err,
				}

				// Log failure and continue for loop to find a pool that can create the volume.
				Logc(ctx).WithFields(logFields).Warn("Failed to create the volume on this pool.")
				err = fmt.Errorf("failed to create volume %s on storage pool %s from backend %s; %w",
					mutableConfig.Name, pool.Name(), backend.Name(), err)
				volumeCreateErrors = multierr.Append(volumeCreateErrors, err)

				// If this backend cannot handle the new volume on any pool, remove it from further consideration.
				var backendIneligibleError *drivers.BackendIneligibleError
				if errors.As(err, &backendIneligibleError) {
					break // Exit pool loop and move on to next backend
				}

				// AddVolume has failed and the backend may have mutated the volume config used so reset the mutable
				// config to a fresh copy of the original config. This prepares the mutable config for the next backend.
				mutableConfig = volConfig.ConstructClone()
			} else {
				// Volume creation succeeded, so register it and return the result.
				// If registration fails, then don't add to cache so that the cleanup
				// code doesn't have to deal with it.  In this path we rely on cleanup
				// to release the locks since we need to be holding it if the volume
				// must be deleted.
				volumeExternal, err := o.addVolumeFinish(ctx, volTxn, vol, backend, pool)
				if err == nil {
					upserter(vol)
				}
				return volumeExternal, err
			}
		} // end pool loop

		// Moving on to the next backend, so unlock the current one
		unlocker()

	} // end backend loop

	// We should never get here with a lock being held, so clear the unlocker to prevent
	// it from being called again during cleanup.
	unlocker = nil

	if volumeCreateErrors == nil {
		err = fmt.Errorf("no suitable %s backend with %s storage class and %s of free space was found",
			protocol, mutableConfig.StorageClass, mutableConfig.Size)
	} else {
		err = fmt.Errorf("encountered error(s) in creating the volume: %w", volumeCreateErrors)
	}

	return nil, err
}

// addVolumeRetry is used to retry volume creations in the case a driver returns VolumeCreatingError.
func (o *ConcurrentTridentOrchestrator) addVolumeRetry(
	ctx context.Context, volConfig *storage.VolumeConfig, pool storage.Pool, volAttributes map[string]sa.Request,
) (volume *storage.Volume, err error) {
	if pool == nil {
		return nil, fmt.Errorf("pool cannot be nil")
	}

	logFields := LogFields{
		"backend": pool.Backend().Name(),
		"pool":    pool.Name(),
		"sc":      volConfig.StorageClass,
		"volume":  volConfig.Name,
	}

	Logc(ctx).WithFields(logFields).Debug("Creating volume.")

	// Create the volume
	createVolume := func() error {
		if volume, err = pool.Backend().AddVolume(ctx, volConfig, pool, volAttributes, false); err != nil {
			if errors.IsVolumeCreatingError(err) {
				return err
			}
			return backoff.Permanent(err)
		}
		return nil
	}

	createNotify := func(err error, duration time.Duration) {
		logFields["increment"] = duration
		Logc(ctx).WithFields(logFields).Debug("Volume is still creating, waiting.")
	}

	createBackoff := backoff.NewExponentialBackOff()
	createBackoff.InitialInterval = 2 * time.Second
	createBackoff.Multiplier = 1.414
	createBackoff.RandomizationFactor = 0.1
	createBackoff.MaxInterval = 15 * time.Second
	createBackoff.MaxElapsedTime = 10 * time.Minute

	// Create volume using an exponential backoff
	if err = backoff.RetryNotify(createVolume, createBackoff, createNotify); err != nil {
		Logc(ctx).WithFields(logFields).Warnf("Volume not created after %3.2f seconds.",
			createBackoff.MaxElapsedTime.Seconds())
	}

	return
}

// addVolumeFinish is called after successful completion of a volume create/clone operation
// to save the volume in the persistent store as well as Trident's in-memory cache.
func (o *ConcurrentTridentOrchestrator) addVolumeFinish(
	ctx context.Context, txn *storage.VolumeTransaction, vol *storage.Volume, backend storage.Backend,
	pool storage.Pool,
) (externalVol *storage.VolumeExternal, err error) {
	recordTransactionTiming(txn, &err)

	if vol.Config.Protocol == config.ProtocolAny {
		vol.Config.Protocol = backend.GetProtocol(ctx)
	}

	// If allowed topologies was not set by the driver, update it if the pool limits supported topologies
	if len(vol.Config.AllowedTopologies) == 0 {
		if len(pool.SupportedTopologies()) > 0 {
			vol.Config.AllowedTopologies = pool.SupportedTopologies()
		}
	}

	// Add new volume to persistent store
	if err = o.storeClient.AddVolume(ctx, vol); err != nil {
		return nil, err
	}

	_, _ = o.resolveAndSetEffectiveAutogrowPolicy(ctx, vol, "volume creation", "warn")

	// Return external form of the volume
	return vol.ConstructExternal(), nil
}

// addVolumeCleanup is used as a deferred method from the volume create/clone methods
// to clean up in case anything goes wrong during the operation.
func (o *ConcurrentTridentOrchestrator) addVolumeCleanup(
	ctx context.Context, err error, backend storage.Backend, vol *storage.Volume,
	volTxn *storage.VolumeTransaction, unlocker func(),
) error {
	var cleanupErr, txErr error

	if err != nil {
		// We failed somewhere.  There are two possible cases:
		// 1.  We failed to allocate on a backend and fell through to the
		//     end of the function.  In this case, we don't need to roll
		//     anything back.
		// 2.  We failed to add the volume to the store.  In this case, we need
		//     to remove the volume from the backend.
		if backend != nil && vol != nil {
			// We succeeded in adding the volume to the backend; now delete it.
			cleanupErr = backend.RemoveVolume(ctx, vol.Config)
			if cleanupErr != nil {
				cleanupErr = fmt.Errorf("unable to delete volume from backend during cleanup:  %v", cleanupErr)
			}
		}
	}
	if cleanupErr == nil {
		// Only clean up the volume transaction if we've succeeded at cleaning up on the backend or if we
		// didn't need to do so in the first place.  If we failed to clean up the volume, we want to leave
		// the transaction in place so the volume is cleaned up on the next Trident controller restart.
		txErr = o.DeleteVolumeTransaction(ctx, volTxn)
		if txErr != nil {
			txErr = fmt.Errorf("unable to clean up add volume transaction; %v", txErr)
		}
	}
	if cleanupErr != nil || txErr != nil {
		err = multierr.Combine(err, cleanupErr, txErr)
		Logc(ctx).Warnf(
			"Unable to clean up artifacts of volume creation; w. Repeat creating the volume or restart %v.",
			err, config.OrchestratorName)
	}
	if unlocker != nil {
		unlocker()
	}
	return err
}

// UpdateVolume updates the allowed fields of a volume in the backend, persistent store and cache.
func (o *ConcurrentTridentOrchestrator) UpdateVolume(
	ctx context.Context, volume string, volumeUpdateInfo *models.VolumeUpdateInfo,
) error {
	return fmt.Errorf("UpdateVolume is not implemented in concurrent core")
}

// UpdateVolumeLUKSPassphraseNames updates the LUKS passphrase names stored on a volume in the cache and persistent store.
func (o *ConcurrentTridentOrchestrator) UpdateVolumeLUKSPassphraseNames(
	ctx context.Context, volumeName string, passphraseNames *[]string,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	// Get write lock for volume
	results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertVolume(volumeName, "")))
	defer unlocker()
	if err != nil {
		return fmt.Errorf("error checking for existing volume; %w", err)
	}
	volume := results[0].Volume.Read
	upserter := results[0].Volume.Upsert
	if volume == nil {
		return errors.NotFoundError("volume %s not found", volumeName)
	}

	// Ensure we have something to do
	if passphraseNames == nil {
		return nil
	}
	volume.Config.LUKSPassphraseNames = *passphraseNames

	// Update persistent store
	if err = o.storeClient.UpdateVolume(ctx, volume); err != nil {
		return err
	}

	upserter(volume)
	return nil
}

// UpdateVolumeAutogrowStatus updates the volume's AutogrowStatus in the in-memory cache and in the
// persistent store (TridentVolume CR).
//
// We do inconsistent reads first (volume and subordinate volume) so we know exactly which lock to take,
// then take the appropriate write lock.
func (o *ConcurrentTridentOrchestrator) UpdateVolumeAutogrowStatus(
	ctx context.Context, volumeName string, status *models.VolumeAutogrowStatus,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)
	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	// Inconsistent read first to know whether this is a main volume or subordinate
	results, unlocker, err := db.Lock(
		ctx,
		db.Query(db.InconsistentReadVolume(volumeName)),
		db.Query(db.InconsistentReadSubordinateVolume(volumeName)))
	unlocker()
	if err != nil {
		return fmt.Errorf("error updating volume autogrow status: %w", err)
	}
	vol := results[0].Volume.Read
	subVol := results[1].SubordinateVolume.Read

	if vol != nil {
		// Main volume: take write lock and update
		results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertVolume(volumeName, "")))
		defer unlocker()
		if err != nil {
			return fmt.Errorf("error updating volume autogrow status: %w", err)
		}
		volume := results[0].Volume.Read
		upserter := results[0].Volume.Upsert
		if volume == nil {
			return errors.NotFoundError("volume %v was not found", volumeName)
		}
		volume.AutogrowStatus = status
		upserter(volume)
		return o.storeClient.UpdateVolume(ctx, volume)
	} else if subVol != nil {
		// Subordinate volume: take write lock and update
		sourceName := subVol.Config.ShareSourceVolume
		results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertSubordinateVolume(volumeName, sourceName)))
		defer unlocker()
		if err != nil {
			return fmt.Errorf("error updating volume autogrow status: %w", err)
		}
		vol := results[0].SubordinateVolume.Read
		if vol == nil {
			return errors.NotFoundError("volume %v was not found", volumeName)
		}
		vol.AutogrowStatus = status
		if results[0].SubordinateVolume.Upsert != nil {
			results[0].SubordinateVolume.Upsert(vol)
		}
		return o.storeClient.UpdateVolume(ctx, vol)
	} else {
		return errors.NotFoundError("volume %v was not found", volumeName)
	}
}

// AttachVolume mounts a volume to the local host.  This method is currently only used by Docker,
// and it should be able to accomplish its task using only the data passed in; it should not need to
// use the storage controller API.  It may be assumed that this method always runs on the host to
// which the volume will be attached.
func (o *ConcurrentTridentOrchestrator) AttachVolume(
	ctx context.Context, volumeName, mountpoint string, publishInfo *models.VolumePublishInfo,
) error {
	return fmt.Errorf("concurrent core is not supported for Docker")
}

func (o *ConcurrentTridentOrchestrator) CloneVolume(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (volExternal *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_clone", &err)()

	volumeConfig.Version = config.OrchestratorAPIVersion

	return o.cloneVolume(ctx, volumeConfig)
}

func (o *ConcurrentTridentOrchestrator) cloneVolume(
	ctx context.Context, volConfig *storage.VolumeConfig,
) (externalVol *storage.VolumeExternal, err error) {
	var (
		backend   storage.Backend
		newVolume *storage.Volume
		pool      storage.Pool
		volTxn    *storage.VolumeTransaction
	)

	// Get backend UUID from the source volume, plus other objects for initial checks
	backendUUID, err := func(volConfig *storage.VolumeConfig) (backendUUID string, returnErr error) {
		results, unlocker, returnErr := db.Lock(
			ctx,
			db.Query(
				db.InconsistentReadVolume(volConfig.Name),
				db.InconsistentReadSubordinateVolume(volConfig.Name),
			),
			db.Query(
				db.InconsistentReadVolume(volConfig.CloneSourceVolume),
				db.InconsistentReadSubordinateVolume(volConfig.CloneSourceVolume),
			),
		)
		defer unlocker()
		if returnErr != nil {
			return
		}

		cloneVolume := results[0].Volume.Read
		cloneVolumeAsSubordinate := results[0].SubordinateVolume.Read
		sourceVolume := results[1].Volume.Read
		sourceVolumeAsSubordinate := results[1].SubordinateVolume.Read

		if cloneVolume != nil {
			returnErr = fmt.Errorf("volume %s already exists", volConfig.Name)
		} else if cloneVolumeAsSubordinate != nil {
			returnErr = fmt.Errorf("volume %s exists but is a subordinate volume", volConfig.Name)
		} else if sourceVolumeAsSubordinate != nil {
			returnErr = errors.NotFoundError("cloning subordinate volume %s is not allowed", volConfig.CloneSourceVolume)
		} else if sourceVolume == nil {
			returnErr = errors.NotFoundError("source volume not found: %s", volConfig.CloneSourceVolume)
		} else {
			backendUUID = sourceVolume.BackendUUID
		}
		return
	}(volConfig)
	if err != nil {
		return nil, err
	}

	// Add transaction in case the operation must be rolled back later
	volTxn = &storage.VolumeTransaction{
		Config: volConfig,
		Op:     storage.AddVolume,
	}

	// Acquire lock on the transaction name to avoid race condition of multiple concurrent requests handling transactions
	o.txnMutex.Lock(volTxn.Name())
	defer o.txnMutex.Unlock(volTxn.Name())

	if err = o.AddVolumeTransaction(ctx, volTxn); err != nil {
		return nil, err
	}

	// Recovery functions in case of error
	defer func() {
		err = o.addVolumeCleanup(ctx, err, backend, newVolume, volTxn, nil)
	}()

	// Get the needed locks for the clone operation
	queries := [][]db.Subquery{
		db.Query(db.UpsertVolume(volConfig.Name, backendUUID)),
		db.Query(db.ReadVolume(volConfig.CloneSourceVolume), db.ReadBackend("")),
	}

	// Also lock the clone source snapshot if we're cloning from a snapshot in a CSI context
	if volConfig.CloneSourceSnapshot != "" && config.CurrentDriverContext == config.ContextCSI {
		snapshotID := storage.MakeSnapshotID(volConfig.CloneSourceVolume, volConfig.CloneSourceSnapshot)
		queries = append(queries, db.Query(db.ReadSnapshot(snapshotID)))
	}

	results, unlocker, err := db.Lock(ctx, queries...)
	defer unlocker()
	if err != nil {
		return nil, err
	}

	cloneVolume := results[0].Volume.Read
	cloneUpserter := results[0].Volume.Upsert
	sourceVolume := results[1].Volume.Read
	backend = results[1].Backend.Read

	var sourceSnapshot *storage.Snapshot
	if len(results) == 3 {
		sourceSnapshot = results[2].Snapshot.Read
	}

	if cloneVolume != nil {
		return nil, fmt.Errorf("volume %s already exists", volConfig.Name)
	} else if sourceVolume == nil {
		return nil, errors.NotFoundError("source volume not found: %s", volConfig.CloneSourceVolume)
	} else if backend == nil {
		return nil, errors.NotFoundError("backend for source volume %s not found", volConfig.CloneSourceVolume)
	}

	// Check if the source volume's backend is honored by the target storage class, only if the orchestrator
	// is not in Docker plugin mode. In Docker plugin mode, the storage class of source and clone volume
	// will be different at times.
	if !isDockerPluginMode() && volConfig.StorageClass != sourceVolume.Config.StorageClass {
		poolMap := o.GetStorageClassPoolMap()
		if !poolMap.BackendMatchesStorageClass(ctx, backend.Name(), volConfig.StorageClass) {
			return nil, errors.MismatchedStorageClassError("clone volume %s from source volume %s with "+
				"different storage classes that have no common backends is not allowed",
				volConfig.Name, volConfig.CloneSourceVolume)
		}
	}

	if volConfig.Size != "" {
		cloneSourceVolumeSize, err := strconv.ParseInt(sourceVolume.Config.Size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not get size of the clone source volume")
		}

		cloneVolumeSize, err := strconv.ParseInt(volConfig.Size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not get size of the clone volume")
		}

		fields := LogFields{
			"source_volume": sourceVolume.Config.Name,
			"volume":        volConfig.Name,
			"backendUUID":   sourceVolume.BackendUUID,
		}
		if cloneSourceVolumeSize < cloneVolumeSize {
			Logc(ctx).WithFields(fields).Error("requested PVC size is too large for the clone source")
			return nil, fmt.Errorf("requested PVC size '%d' is too large for the clone source '%d'",
				cloneVolumeSize, cloneSourceVolumeSize)
		}
	}

	if sourceVolume.Orphaned {
		Logc(ctx).WithFields(LogFields{
			"sourceVolume": sourceVolume.Config.Name,
			"volume":       volConfig.Name,
			"backendUUID":  sourceVolume.BackendUUID,
		}).Warnf("Clone operation is likely to fail with an orphaned source volume.")
	}

	sourceVolumeMode := sourceVolume.Config.VolumeMode
	if sourceVolumeMode != "" && sourceVolumeMode != volConfig.VolumeMode {
		return nil, errors.NotFoundError(
			"source volume's volume-mode (%s) is incompatible with requested clone's volume-mode (%s)",
			sourceVolume.Config.VolumeMode, volConfig.VolumeMode)
	}

	pool = storage.NewStoragePool(backend, "")

	// Clone the source config, as most of its attributes will apply to the clone
	cloneConfig := sourceVolume.Config.ConstructClone()
	internalName := ""
	internalID := ""

	Logc(ctx).WithFields(LogFields{
		"CloneName":              volConfig.Name,
		"SourceVolume":           volConfig.CloneSourceVolume,
		"SourceVolumeInternal":   sourceVolume.Config.InternalName,
		"SourceSnapshot":         volConfig.CloneSourceSnapshot,
		"SourceSnapshotInternal": volConfig.CloneSourceSnapshotInternal,
		"Qos":                    volConfig.Qos,
		"QosType":                volConfig.QosType,
	}).Trace("Adding attributes from the request which will affect clone creation.")

	cloneConfig.Name = volConfig.Name
	cloneConfig.InternalName = internalName
	cloneConfig.InternalID = internalID
	cloneConfig.CloneSourceVolume = volConfig.CloneSourceVolume
	cloneConfig.CloneSourceVolumeInternal = sourceVolume.Config.InternalName
	cloneConfig.CloneSourceSnapshot = volConfig.CloneSourceSnapshot
	cloneConfig.CloneSourceSnapshotInternal = volConfig.CloneSourceSnapshotInternal
	cloneConfig.Qos = volConfig.Qos
	cloneConfig.QosType = volConfig.QosType
	// Clear these values as they were copied from the source volume Config
	cloneConfig.SubordinateVolumes = make(map[string]interface{})
	cloneConfig.ShareSourceVolume = ""
	cloneConfig.LUKSPassphraseNames = sourceVolume.Config.LUKSPassphraseNames
	// Override this value only if SplitOnClone has been defined in clone volume's config
	if volConfig.SplitOnClone != "" {
		cloneConfig.SplitOnClone = volConfig.SplitOnClone
	}
	cloneConfig.ReadOnlyClone = volConfig.ReadOnlyClone
	cloneConfig.Namespace = volConfig.Namespace
	cloneConfig.RequestName = volConfig.RequestName

	// If skipRecoveryQueue is set for the clone, use it. If not, default to the source volume's setting.
	if volConfig.SkipRecoveryQueue != "" {
		cloneConfig.SkipRecoveryQueue = volConfig.SkipRecoveryQueue
	}

	// Override tiering settings from clone volume config if provided
	if volConfig.TieringPolicy != "" {
		cloneConfig.TieringPolicy = volConfig.TieringPolicy
	}
	if volConfig.TieringMinimumCoolingDays != "" {
		cloneConfig.TieringMinimumCoolingDays = volConfig.TieringMinimumCoolingDays
	}

	// If it's from snapshot, we need the LUKS passphrases value from the snapshot
	isLUKS, err := strconv.ParseBool(cloneConfig.LUKSEncryption)
	// If the LUKSEncryption is not a bool (or is empty string) assume we are not making a LUKS volume
	if err != nil {
		isLUKS = false
	}

	if cloneConfig.CloneSourceSnapshot != "" {
		switch config.CurrentDriverContext {
		case config.ContextCSI:
			// Only look for the snapshot in our cache if we are in a CSI context.
			if sourceSnapshot == nil {
				return nil, errors.NotFoundError("could not find source snapshot for volume")
			}

			// Get the passphrase names from the source snapshot if LUKS the clone requires LUKSEncryption.
			if isLUKS {
				cloneConfig.LUKSPassphraseNames = sourceSnapshot.Config.LUKSPassphraseNames
			}

			if cloneConfig.CloneSourceSnapshotInternal == "" {
				cloneConfig.CloneSourceSnapshotInternal = sourceSnapshot.Config.InternalName
			}
		case config.ContextDocker:
			// Docker only supplies the source snapshot name, but backends must rely on the internal snapshot
			// names for cloning from snapshots.
			cloneConfig.CloneSourceSnapshotInternal = cloneConfig.CloneSourceSnapshot
		}

		// If no internal snapshot name is set, fail immediately. Attempting a clone
		// will fail because backends rely on the internal snapshot name.
		if cloneConfig.CloneSourceSnapshotInternal == "" {
			return nil, fmt.Errorf(
				"cannot clone from snapshot %v; internal snapshot name not found", cloneConfig.CloneSourceSnapshot)
		}
	}

	if sourceVolume.Config.ImportNotManaged && cloneConfig.CloneSourceSnapshot == "" {
		return nil, fmt.Errorf("cannot clone an unmanaged volume without a snapshot")
	}
	// Make the cloned volume managed
	cloneConfig.ImportNotManaged = false

	// With the introduction of Virtual Pools we will try our best to place the cloned volume in the same
	// Virtual Pool. For cases where attributes are not defined in the PVC (source/clone) but instead in the
	// backend storage pool, e.g. splitOnClone, we would like the cloned PV to have the same attribute value
	// as the source PV.
	// NOTE: The clone volume config can be different than the source volume config when a backend modification
	// changes the Virtual Pool's arrangement or the Virtual Pool's attributes are modified. This
	// is because pool names are autogenerated and are based on their relative arrangement.
	sourceVolumePoolName := sourceVolume.Pool
	if sourceVolumePoolName != drivers.UnsetPool {
		if sourceVolumeStoragePool, ok := backend.StoragePools().Load(sourceVolumePoolName); ok {
			pool = sourceVolumeStoragePool.(storage.Pool)
		}
	}

	// Create the backend-specific internal names so they are saved in the transaction
	backend.CreatePrepare(ctx, cloneConfig, pool)

	// Update transaction with updated cloneConfig
	volTxn = &storage.VolumeTransaction{
		Config: cloneConfig,
		Op:     storage.AddVolume,
	}
	if err = o.storeClient.UpdateVolumeTransaction(ctx, volTxn); err != nil {
		Logc(ctx).Errorf("Error updating volume transaction; %w", err)
		return nil, err
	}

	logFields := LogFields{
		"backend":      backend.Name(),
		"backendUUID":  backend.BackendUUID(),
		"volume":       cloneConfig.Name,
		"sourceVolume": cloneConfig.CloneSourceVolume,
	}

	// Create the clone
	newVolume, err = o.cloneVolumeRetry(ctx, backend, sourceVolume.Config, cloneConfig, pool)
	if err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Failed to create cloned volume on this backend.")
		return nil, fmt.Errorf("failed to create cloned volume %s on backend %s: %w",
			cloneConfig.Name, backend.Name(), err)
	}

	volumeExternal, err := o.addVolumeFinish(ctx, volTxn, newVolume, backend, pool)
	if err == nil {
		Logc(ctx).WithFields(logFields).Trace("Volume clone succeeded, registering it and returning the result.")
		cloneUpserter(newVolume)
	}

	return volumeExternal, err
}

// cloneVolumeRetry is used to retry volume clones in the case a driver returns VolumeCreatingError.
func (o *ConcurrentTridentOrchestrator) cloneVolumeRetry(
	ctx context.Context, backend storage.Backend, sourceVolConfig, cloneConfig *storage.VolumeConfig, pool storage.Pool,
) (volume *storage.Volume, err error) {
	logFields := LogFields{
		"backend":      backend.Name(),
		"backendUUID":  backend.BackendUUID(),
		"volume":       cloneConfig.Name,
		"sourceVolume": cloneConfig.CloneSourceVolume,
	}

	Logc(ctx).WithFields(logFields).Debug("Cloning volume.")

	// Create the volume
	createVolume := func() error {
		if volume, err = backend.CloneVolume(ctx, sourceVolConfig, cloneConfig, pool, false); err != nil {
			if errors.IsVolumeCreatingError(err) {
				return err
			}
			return backoff.Permanent(err)
		}
		return nil
	}

	createNotify := func(err error, duration time.Duration) {
		logFields["increment"] = duration
		Logc(ctx).WithFields(logFields).Debug("Volume is still creating, waiting.")
	}

	createBackoff := backoff.NewExponentialBackOff()
	createBackoff.InitialInterval = 2 * time.Second
	createBackoff.Multiplier = 1.414
	createBackoff.RandomizationFactor = 0.1
	createBackoff.MaxInterval = 15 * time.Second
	createBackoff.MaxElapsedTime = 10 * time.Minute

	// Create volume using an exponential backoff
	if err = backoff.RetryNotify(createVolume, createBackoff, createNotify); err != nil {
		Logc(ctx).WithFields(logFields).Warnf("Volume not created after %3.2f seconds.",
			createBackoff.MaxElapsedTime.Seconds())
	}

	return
}

// DetachVolume unmounts a volume from the local host.  This method is currently only used by Docker,
// and it should be able to accomplish its task using only the data passed in; it should not need to
// use the storage controller API.  It may be assumed that this method always runs on the host to
// which the volume will be attached.  It ensures the volume is already mounted, and it attempts to
// delete the mount point.
func (o *ConcurrentTridentOrchestrator) DetachVolume(ctx context.Context, volumeName, mountpoint string) error {
	return fmt.Errorf("concurrent core is not supported for Docker")
}

// DeleteVolume removes a volume from storage, persistence, and cache.
func (o *ConcurrentTridentOrchestrator) DeleteVolume(ctx context.Context, volumeName string) (err error) {
	ctx = NewContextBuilder(ctx).WithLayer(LogLayerCore).BuildContext()

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_delete", &err)()

	// Check for subordinate volume
	results, unlocker, err := db.Lock(ctx, db.Query(db.InconsistentReadSubordinateVolume(volumeName)))
	unlocker()
	if err != nil {
		return fmt.Errorf("error checking for existing subordinate volume; %w", err)
	}
	if results[0].SubordinateVolume.Read != nil {
		return o.deleteSubordinateVolume(ctx, volumeName)
	}

	results, unlocker, err = db.Lock(ctx, db.Query(db.InconsistentReadVolume(volumeName)))
	unlocker()
	if err != nil {
		return fmt.Errorf("error checking for existing volume; %w", err)
	}
	volume := results[0].Volume.Read
	if volume == nil {
		return errors.NotFoundError("volume %s not found", volumeName)
	}

	if volume.Orphaned {
		Logc(ctx).WithFields(LogFields{
			"volume":      volumeName,
			"backendUUID": volume.BackendUUID,
		}).Warnf("Delete operation is likely to fail with an orphaned volume.")
	}

	volTxn := &storage.VolumeTransaction{
		Config: volume.Config,
		Op:     storage.DeleteVolume,
	}

	// Acquire lock on the transaction name to avoid race condition of multiple concurrent requests handling transactions
	o.txnMutex.Lock(volTxn.Name())
	defer o.txnMutex.Unlock(volTxn.Name())

	if err = o.AddVolumeTransaction(ctx, volTxn); err != nil {
		return err
	}

	defer func() {
		errTxn := o.DeleteVolumeTransaction(ctx, volTxn)
		if errTxn != nil {
			Logc(ctx).WithFields(LogFields{
				"volume":    volume,
				"error":     errTxn,
				"operation": volTxn.Op,
			}).Warnf("Unable to delete volume transaction. Repeat deletion using %s or restart %v.",
				config.OrchestratorClientName, config.OrchestratorName)
		}
		if err != nil || errTxn != nil {
			err = multierr.Combine(err, errTxn)
		}
	}()

	return o.deleteVolume(ctx, volumeName)
}

func (o *ConcurrentTridentOrchestrator) deleteVolume(ctx context.Context, volumeName string) error {
	// Grab locks suitable for deleting a volume or updating a volume after soft-deleting it
	results, unlocker, err := db.Lock(ctx,
		db.Query(db.DeleteVolume(volumeName), db.ReadBackend("")), db.Query(db.UpsertVolume(volumeName, "")),
		db.Query(db.ListSnapshotsForVolume(volumeName), db.ListSubordinateVolumesForVolume(volumeName)),
	)
	if err != nil {
		unlocker()
		return err
	}

	volume := results[0].Volume.Read
	backend := results[0].Backend.Read
	deleter := results[0].Volume.Delete
	upserter := results[1].Volume.Upsert
	snapshotsForVolume := results[2].Snapshots
	subordinatesForVolume := results[2].SubordinateVolumes

	if volume == nil {
		unlocker()
		return errors.NotFoundError("volume %s not found", volumeName)
	}

	logFields := LogFields{"volume": volumeName, "backendUUID": volume.BackendUUID}

	// If there are any snapshots or subordinate volumes for this volume, we need to "soft" delete.
	// Only hard delete this volume when its last snapshot is deleted and no subordinates remain.
	if len(snapshotsForVolume) > 0 || len(subordinatesForVolume) > 0 {
		Logc(ctx).WithFields(logFields).Debug("Soft deleting.")

		volume.State = storage.VolumeStateDeleting

		if updateErr := o.storeClient.UpdateVolume(ctx, volume); updateErr != nil {
			Logc(ctx).WithFields(logFields).WithError(updateErr).Error(
				"Unable to update the volume's state to deleting in the persistent store.")
			unlocker()
			return updateErr
		}

		upserter(volume)
		unlocker()
		return nil
	}

	// Note that this block will only be entered in the case that the volume is missing its backend
	// and the backend is nil. If the backend does not exist, delete the volume and clean up, then return.
	if backend == nil {
		if err = o.storeClient.DeleteVolume(ctx, volume); err != nil {
			unlocker()
			return err
		}

		deleter()
		unlocker()
		return nil
	}

	// Here the backend cannot be nil, so check if it is in Deleting state.
	backendDeleting := backend.State().IsDeleting()

	// Note that this call will only return an error if the backend actually fails to delete the volume.
	// If the volume does not exist on the backend, the driver will not return an error.  Thus, we're fine.
	if err := backend.RemoveVolume(ctx, volume.Config); err != nil {
		if !errors.IsNotManagedError(err) {
			Logc(ctx).WithFields(logFields).WithError(err).Error("Unable to delete volume from backend.")
			unlocker()
			return err
		} else {
			Logc(ctx).WithFields(logFields).WithError(err).Debug("Skipping backend deletion of unmanaged volume.")
		}
	}

	// Delete volume from persistent store
	if err = o.storeClient.DeleteVolume(ctx, volume); err != nil {
		unlocker()
		return err
	}

	// Delete volume from the cache and unlock everything in case we need to delete a backend.
	deleter()
	unlocker()

	// Check if we need to remove a soft-deleted backend
	if backendDeleting {
		if backendDeleteErr := o.cleanupDeletingBackend(ctx, backend.BackendUUID()); backendDeleteErr != nil {
			Logc(ctx).WithError(backendDeleteErr).WithField("name", backend.Name()).Warning(
				"Could not delete backend in deleting state.")
		}
	}

	return nil
}

// cleanupDeletingBackend deletes a soft-deleted backend that lacks volumes.
func (o *ConcurrentTridentOrchestrator) cleanupDeletingBackend(ctx context.Context, backendUUID string) error {
	// Before grabbing a write lock, confirm backend exists, has no volumes, and is in Deleting state
	results, unlocker, err := db.Lock(ctx, db.Query(db.InconsistentReadBackend(backendUUID)))
	unlocker()
	if err != nil {
		return err
	}

	backend := results[0].Backend.Read
	if backend == nil || backend.HasVolumes() || !backend.State().IsDeleting() {
		return nil
	}

	// Grab locks suitable for deleting a soft-deleted backend
	results, unlocker, err = db.Lock(ctx, db.Query(db.DeleteBackend(backendUUID)))
	defer unlocker()
	if err != nil {
		return err
	}

	backend = results[0].Backend.Read
	deleter := results[0].Backend.Delete

	if !backend.HasVolumes() && (backend.State().IsDeleting() || o.storeClient.IsBackendDeleting(ctx, backend)) {
		// Delete backend from the persistent store
		if err := o.storeClient.DeleteBackend(ctx, backend); err != nil {
			return fmt.Errorf("failed to delete empty deleting backend %s: %w", backend.BackendUUID(), err)
		}

		backend.Terminate(ctx)

		// Delete backend from the cache
		deleter()
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) GetVolume(
	ctx context.Context, volumeName string,
) (volExternal *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_get", &err)()

	return o.getVolume(ctx, volumeName)
}

func (o *ConcurrentTridentOrchestrator) getVolume(
	ctx context.Context, volumeName string,
) (volExternal *storage.VolumeExternal, err error) {
	// Check for both types of volumes
	results, unlocker, err := db.Lock(
		ctx,
		db.Query(db.InconsistentReadVolume(volumeName)),
		db.Query(db.InconsistentReadSubordinateVolume(volumeName)))
	defer unlocker()

	if err != nil {
		return nil, fmt.Errorf("error checking for existing volume; %w", err)
	} else if results[0].Volume.Read != nil {
		return results[0].Volume.Read.ConstructExternal(), nil
	} else if results[1].SubordinateVolume.Read != nil {
		return results[1].SubordinateVolume.Read.ConstructExternal(), nil
	}

	return nil, errors.NotFoundError("volume %v was not found", volumeName)
}

func (o *ConcurrentTridentOrchestrator) GetVolumeByInternalName(
	ctx context.Context, volumeInternal string,
) (volume string, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	defer recordTiming("volume_internal_get", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.ListVolumesByInternalName(volumeInternal)))
	unlocker()
	if err != nil {
		return "", fmt.Errorf("error listing existing volumes; %w", err)
	}

	volumes := results[0].Volumes

	if len(volumes) > 0 {
		return volumes[0].Config.Name, nil
	}

	return "", errors.NotFoundError("volume %s not found", volumeInternal)
}

func (o *ConcurrentTridentOrchestrator) GetVolumeForImport(
	ctx context.Context, volumeID, backendName string,
) (volExternal *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_get_for_import", &err)()

	Logc(ctx).WithFields(LogFields{
		"volumeID":    volumeID,
		"backendName": backendName,
	}).Debug("Orchestrator#GetVolumeForImport")

	results, unlocker, err := db.Lock(ctx, db.Query(db.ReadBackendByName(backendName)))
	defer unlocker()
	if err != nil {
		return nil, err
	}
	backend := results[0].Backend.Read
	if backend == nil {
		return nil, errors.NotFoundError("backend %v was not found", backendName)
	}

	return backend.GetVolumeForImport(ctx, volumeID)
}

func (o *ConcurrentTridentOrchestrator) ImportVolume(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (v *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	if volumeConfig.ImportBackendUUID == "" {
		return nil, fmt.Errorf("no backend specified for import")
	}
	if volumeConfig.ImportOriginalName == "" {
		return nil, fmt.Errorf("original name not specified")
	}

	defer recordTiming("volume_import", &err)()

	Logc(ctx).WithFields(LogFields{
		"volumeConfig": volumeConfig,
		"backendUUID":  volumeConfig.ImportBackendUUID,
	}).Debug("ConcurrentOrchestrator#ImportVolume")

	volume, err := o.importVolume(ctx, volumeConfig)
	if err != nil {
		return nil, err
	}

	if !volumeConfig.ImportNotManaged {
		// we successfully renamed the backend volume as part of the import operation. We need to update the volume
		// cache's uniqueKey with new internal volume name.

		results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertVolumeByInternalName(
			volumeConfig.Name,
			volumeConfig.ImportOriginalName,
			volumeConfig.InternalName,
			volumeConfig.ImportBackendUUID)),
		)
		if err != nil {
			unlocker()
			return nil, err
		}

		updateVolumeInCache := results[0].Volume.Upsert
		updateVolumeInCache(volume)
		unlocker()
	}

	volExternal := volume.ConstructExternal()

	Logc(ctx).WithFields(LogFields{
		"backend":        volExternal.Backend,
		"name":           volExternal.Config.Name,
		"internalName":   volExternal.Config.InternalName,
		"size":           volExternal.Config.Size,
		"protocol":       volExternal.Config.Protocol,
		"fileSystem":     volExternal.Config.FileSystem,
		"accessInfo":     volExternal.Config.AccessInfo,
		"accessMode":     volExternal.Config.AccessMode,
		"encryption":     volExternal.Config.Encryption,
		"LUKSEncryption": volExternal.Config.LUKSEncryption,
		"exportPolicy":   volExternal.Config.ExportPolicy,
		"snapshotPolicy": volExternal.Config.SnapshotPolicy,
	}).Trace("Driver processed volume import.")

	return volExternal, nil
}

func (o *ConcurrentTridentOrchestrator) importVolume(ctx context.Context,
	volumeConfig *storage.VolumeConfig,
) (*storage.Volume, error) {
	var err, importErr error

	// Add transaction in case operation must be rolled back
	volTxn := &storage.VolumeTransaction{
		Config: volumeConfig,
		Op:     storage.ImportVolume,
	}

	// Transaction lock must be acquired before any cache locks to avoid potential deadlock.
	o.txnMutex.Lock(volTxn.Name())
	defer o.txnMutex.Unlock(volTxn.Name())

	if err := o.AddVolumeTransaction(ctx, volTxn); err != nil {
		return nil, fmt.Errorf("failed to add volume transaction: %v", err)
	}

	// Acquire a read lock on the backend
	// Acquire a write lock on the PV name and write lock on the backend volume name that being imported. This is to avoid the race condition
	// Acquire read lock on the storage class.
	results, unlocker, err := db.Lock(
		ctx,
		db.Query(
			db.UpsertVolumeByInternalName(volumeConfig.Name, "", volumeConfig.ImportOriginalName, volumeConfig.ImportBackendUUID),
			db.ReadBackend(""),
			db.ReadStorageClass(volumeConfig.StorageClass),
		),
		db.Query( // This lock is for cleaning up the volume from the cache in case of an error
			db.DeleteVolume(volumeConfig.Name),
		),
	)
	defer unlocker()
	if err != nil {
		// we still need to delete the transaction if we err out here
		if cleanupErr := o.DeleteVolumeTransaction(ctx, volTxn); cleanupErr != nil {
			return nil, fmt.Errorf("failed to clean up volume addition transaction: %v", err)
		}
		return nil, err
	}

	backend := results[0].Backend.Read
	volume := results[0].Volume.Read
	storageClass := results[0].StorageClass.Read
	insertVolumeInCache := results[0].Volume.Upsert
	deleteVolumeInCache := results[1].Volume.Delete

	// Recover function in case of error
	defer func() {
		err = o.importVolumeCleanup(ctx, importErr, err, volumeConfig, backend, volTxn, deleteVolumeInCache)
	}()

	if volume != nil {
		return nil, fmt.Errorf("volume %s already exists", volumeConfig.Name)
	}

	err = o.validateImportVolume(ctx, volumeConfig, backend, storageClass)
	if err != nil {
		return nil, err
	}

	volume, importErr = backend.ImportVolume(ctx, volumeConfig)
	if importErr != nil {
		importErr = fmt.Errorf("failed to import volume %s on backend %s: %v", volumeConfig.ImportOriginalName,
			volumeConfig.ImportBackendUUID, importErr)
		return nil, importErr
	}

	importErr = o.storeClient.AddVolume(ctx, volume)
	if importErr != nil {
		importErr = fmt.Errorf("failed to persist imported volume data: %v", importErr)
		return nil, importErr
	}

	_, _ = o.resolveAndSetEffectiveAutogrowPolicy(ctx, volume, "volume import", "warn")

	insertVolumeInCache(volume)

	return volume, nil
}

func (o *ConcurrentTridentOrchestrator) validateImportVolume(ctx context.Context,
	volumeConfig *storage.VolumeConfig, backend storage.Backend, storageClass *storageclass.StorageClass,
) error {
	if backend == nil {
		return errors.NotFoundError("backend %s not found", volumeConfig.ImportBackendUUID)
	}

	if storageClass == nil {
		return fmt.Errorf("unknown storage class: %s", volumeConfig.StorageClass)
	}

	originalName := volumeConfig.ImportOriginalName
	backendUUID := volumeConfig.ImportBackendUUID

	extantVol, err := backend.GetVolumeForImport(ctx, originalName)
	if err != nil {
		return errors.NotFoundError("volume %s was not found: %v", originalName, err)
	}

	requestedSize, err := strconv.ParseInt(volumeConfig.Size, 10, 64)
	if err != nil {
		return fmt.Errorf("could not determine requested size to import: %v", err)
	}

	actualSize, err := strconv.ParseInt(extantVol.Config.Size, 10, 64)
	if err != nil {
		return fmt.Errorf("could not determine actual size of the volume being imported: %v", err)
	}

	if actualSize < requestedSize {
		Logc(ctx).WithFields(LogFields{
			"requestedSize": requestedSize,
			"actualSize":    actualSize,
		}).Error("Import request size is more than actual size.")
		return errors.UnsupportedCapacityRangeError(errors.New("requested size is more than actual size"))
	}

	results, unlocker, err := db.Lock(ctx, db.Query(db.ListVolumes()))
	unlocker()
	if err != nil {
		return err
	}
	allVolumes := results[0].Volumes

	for _, vol := range allVolumes {
		if vol.Config.InternalName == extantVol.Config.InternalName && vol.BackendUUID == backendUUID {
			return errors.FoundError("PV %s already exists for volume %s", originalName, vol.Config.Name)
		}
	}

	poolMap := o.GetStorageClassPoolMap()
	if !poolMap.BackendMatchesStorageClass(ctx, backend.Name(), volumeConfig.StorageClass) {
		return fmt.Errorf("storageClass %s does not match any storage pools for backend %s",
			volumeConfig.StorageClass, backend.Name())
	}

	// Identify the resultant protocol based on the VolumeMode, AccessMode and the Protocol. For a valid case
	// getProtocol() returns a protocol that is either same as volumeConfig.Protocol or `file/block` protocol
	// in place of `Any` Protocol.
	protocol, err := getProtocol(ctx, volumeConfig.VolumeMode, volumeConfig.AccessMode, volumeConfig.Protocol)
	if err != nil {
		return err
	}

	backendProtocol := backend.GetProtocol(ctx)
	// Make sure the resultant protocol matches the backend protocol
	if protocol != config.ProtocolAny && protocol != backendProtocol {
		return fmt.Errorf(
			"requested volume mode (%s), access mode (%s), protocol (%s) are incompatible with the backend %s",
			volumeConfig.VolumeMode, volumeConfig.AccessMode, volumeConfig.Protocol, backend.Name())
	}

	// For `Any` protocol make it same as the requested backend's protocol
	if volumeConfig.Protocol == "" {
		volumeConfig.Protocol = backend.GetProtocol(ctx)
	}

	// Make sure that for the Raw-block volume import we do not have ext3, ext4 or xfs filesystem specified
	if volumeConfig.VolumeMode == config.RawBlock {
		if volumeConfig.FileSystem != "" && volumeConfig.FileSystem != filesystem.Raw {
			return fmt.Errorf("cannot create raw-block volume %s with the filesystem %s",
				originalName, volumeConfig.FileSystem)
		}
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) importVolumeCleanup(
	ctx context.Context, importErr, err error, volumeConfig *storage.VolumeConfig, backend storage.Backend,
	volTxn *storage.VolumeTransaction, deleteVolumeFromCache func(),
) error {
	var cleanupErr, txErr error

	if importErr != nil && backend != nil {
		// We failed somewhere. Most likely we failed to rename the volume or retrieve its size.
		// Rename the volume
		if !volumeConfig.ImportNotManaged {
			cleanupErr = backend.RenameVolume(ctx, volumeConfig, volumeConfig.ImportOriginalName)
			Logc(ctx).WithFields(LogFields{
				"InternalName":       volumeConfig.InternalName,
				"importOriginalName": volumeConfig.ImportOriginalName,
				"cleanupErr":         cleanupErr,
			}).Warn("importVolumeCleanup: failed to cleanup volume import.")
		}

		// Remove volume from backend cache
		backend.RemoveCachedVolume(volumeConfig.Name)

		// Remove volume from orchestrator cache
		results, unlocker, err := db.Lock(ctx, db.Query(db.InconsistentReadVolume(volumeConfig.Name)))
		unlocker()
		if err != nil {
			return fmt.Errorf("error checking for existing volume; %w", err)
		}

		volume := results[0].Volume.Read
		if volume != nil {
			deleteVolumeFromCache()

			// Remove the volume from the persistent store
			if err = o.storeClient.DeleteVolume(ctx, volume); err != nil {
				return fmt.Errorf("error occurred removing volume from persistent store; %v", err)
			}
		}
	}

	txErr = o.DeleteVolumeTransaction(ctx, volTxn)
	if txErr != nil {
		txErr = fmt.Errorf("unable to clean up import volume transaction:  %v", txErr)
	}

	combinedErr := multierr.Combine(err, importErr, cleanupErr, txErr)
	if cleanupErr != nil || txErr != nil {
		Logc(ctx).Warnf("Unable to clean up artifacts of volume import: %v. Repeat importing the volume %v.",
			combinedErr, volumeConfig.ImportOriginalName)
	}

	return combinedErr
}

func (o *ConcurrentTridentOrchestrator) ListVolumes(
	ctx context.Context,
) (volumeExternals []*storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_list", &err)()

	// Check for both types of volumes
	results, unlocker, err := db.Lock(
		ctx,
		db.Query(db.ListVolumes()),
		db.Query(db.ListSubordinateVolumes()))
	defer unlocker()

	if err != nil {
		return nil, fmt.Errorf("error listing existing volume; %w", err)
	}

	volumes := results[0].Volumes
	subordinateVolumes := results[1].SubordinateVolumes

	volumeExternals = make([]*storage.VolumeExternal, 0, len(volumes)+len(subordinateVolumes))
	for _, v := range volumes {
		volumeExternals = append(volumeExternals, v.ConstructExternal())
	}
	for _, v := range subordinateVolumes {
		volumeExternals = append(volumeExternals, v.ConstructExternal())
	}

	sort.Sort(storage.ByVolumeExternalName(volumeExternals))

	return volumeExternals, nil
}

func (o *ConcurrentTridentOrchestrator) PublishVolume(ctx context.Context, volumeName string,
	publishInfo *models.VolumePublishInfo,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_publish", &err)()

	fields := LogFields{
		"volume": volumeName,
		"node":   publishInfo.HostName,
	}
	Logc(ctx).WithFields(fields).Info("Publishing volume to node.")

	err = o.publishVolume(ctx, volumeName, publishInfo)
	return
}

func (o *ConcurrentTridentOrchestrator) publishVolume(ctx context.Context, volumeName string,
	publishInfo *models.VolumePublishInfo,
) error {
	results, unlocker, err := db.Lock(ctx, db.Query(
		db.ListVolumePublications(),
		db.ListNodes(),
		db.UpsertVolumePublication(volumeName, publishInfo.HostName),
		db.ReadBackend(""),
		db.ReadNode(""),
		db.UpsertVolume("", ""),
	))
	defer unlocker()
	if err != nil {
		return err
	}

	volume := results[0].Volume.Read
	if volume == nil {
		return errors.NotFoundError("volume %v was not found", volumeName)
	}
	node := results[0].Node.Read
	if node == nil {
		return errors.NotFoundError("node %v was not found", publishInfo.HostName)
	}
	backend := results[0].Backend.Read
	if backend == nil {
		return errors.NotFoundError("backend was not found")
	}

	fields := LogFields{
		"volumeName": volumeName,
		"nodeName":   publishInfo.HostName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>>>> Orchestrator#publishVolume")
	defer Logc(ctx).Debug("<<<<<< Orchestrator#publishVolume")

	if volume.State.IsDeleting() {
		return errors.VolumeStateError(fmt.Sprintf("volume %s is deleting", volumeName))
	}

	// Publish enforcement is considered only when the backend supports publish enforcement in CSI deployments.
	publishEnforceable := config.CurrentDriverContext == config.ContextCSI && backend.CanEnablePublishEnforcement()
	if publishEnforceable {
		if node.PublicationState != models.NodeClean {
			return errors.NodeNotSafeToPublishForBackendError(publishInfo.HostName, backend.GetDriverName())
		}
	}

	publishInfo.TridentUUID = o.uuid
	publishInfo.BackendUUID = backend.BackendUUID()

	// Enable publish enforcement if the backend supports it and the volume isn't already enforced
	if publishEnforceable && !volume.Config.AccessInfo.PublishEnforcement {
		publications := make([]*models.VolumePublication, 0)
		for _, vp := range results[0].VolumePublications {
			if vp.VolumeName == volumeName {
				publications = append(publications, vp)
			}
		}
		safeToEnable := len(publications) == 0 || len(publications) == 1 && publications[0].VolumeName == volumeName
		if safeToEnable {
			if err := backend.EnablePublishEnforcement(ctx, volume); err != nil {
				if !errors.IsUnsupportedError(err) {
					Logc(ctx).WithFields(fields).WithError(err).Errorf(
						"Failed to enable volume publish enforcement for volume.")
					return err
				}
				Logc(ctx).WithFields(fields).WithError(err).Debug(
					"Volume publish enforcement is not fully supported on backend.")
			}
		}
	}

	// Fill in what we already know
	publishInfo.VolumeAccessInfo = volume.Config.AccessInfo
	publishInfo.Nodes = results[0].Nodes

	if err := backend.PublishVolume(ctx, volume.Config, publishInfo); err != nil {
		return err
	}
	vp := &models.VolumePublication{
		Name:               models.GenerateVolumePublishName(volumeName, publishInfo.HostName),
		VolumeName:         volumeName,
		NodeName:           publishInfo.HostName,
		ReadOnly:           publishInfo.ReadOnly,
		AccessMode:         publishInfo.AccessMode,
		AutogrowPolicy:     volume.Config.RequestedAutogrowPolicy,
		AutogrowIneligible: isVolumeAutogrowIneligible(volume.Config),
		StorageClass:       volume.Config.StorageClass,
		BackendUUID:        backend.BackendUUID(),
		Pool:               volume.Pool,
	}
	if err := o.storeClient.AddVolumePublication(ctx, vp); err != nil {
		// Handle idempotent publish - if publication already exists, treat as success
		// This can happen when Kubernetes retries ControllerPublishVolume
		if !persistentstore.IsAlreadyExistsError(err) {
			return err
		}
		Logc(ctx).WithFields(fields).Debug("Volume publication already exists, continuing.")
	}

	if err := o.updateVolumeInStoreAndCache(ctx, &results[0]); err != nil {
		return err
	}

	results[0].VolumePublication.Upsert(vp)
	return nil
}

func (o *ConcurrentTridentOrchestrator) updateVolumeInStoreAndCache(ctx context.Context, result *db.Result) error {
	volume := result.Volume.Read
	Logc(ctx).WithFields(LogFields{
		"volume":          volume.Config.Name,
		"volume_orphaned": volume.Orphaned,
		"volume_size":     volume.Config.Size,
		"volumeState":     string(volume.State),
	}).Debug("Updating an existing volume.")
	if err := o.storeClient.UpdateVolume(ctx, volume); err != nil {
		return err
	}
	result.Volume.Upsert(volume)
	return nil
}

func (o *ConcurrentTridentOrchestrator) UnpublishVolume(ctx context.Context, volumeName, nodeName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_unpublish", &err)()

	fields := LogFields{
		"volume": volumeName,
		"node":   nodeName,
	}
	Logc(ctx).WithFields(fields).Info("Unpublishing volume from node.")

	nodeDeleted, err := o.unpublishVolume(ctx, volumeName, nodeName)
	if err != nil {
		return err
	}

	// If the node was soft-deleted, we need to check if the CR can be deleted
	if nodeDeleted {
		return o.tryDeleteSoftDeletedNode(ctx, nodeName)
	}

	return
}

func (o *ConcurrentTridentOrchestrator) unpublishVolume(
	ctx context.Context, volumeName, nodeName string,
) (bool, error) {
	fields := LogFields{
		"volumeName": volumeName,
		"nodeName":   nodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>>>> Orchestrator#unpublishVolume")
	defer Logc(ctx).Debug("<<<<<< Orchestrator#unpublishVolume")

	results, unlocker, err := db.Lock(ctx,
		db.Query(
			db.ListVolumePublicationsForVolume(volumeName),
			db.ReadNode(""),
			db.ListNodes(),
			db.ReadBackend(""),
			db.UpsertVolume("", ""),
			db.DeleteVolumePublication(volumeName, nodeName),
		),
		db.Query(
			db.ListVolumesForNode(nodeName),
		),
	)
	defer unlocker()
	if err != nil {
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithError(err).WithFields(fields).Warn("Volume is not published to node; doing nothing.")
			return false, nil
		}
		return false, err
	}

	node := results[0].Node.Read
	if node == nil {
		return false, errors.NotFoundError("node %v was not found", nodeName)
	}
	backend := results[0].Backend.Read
	if backend == nil {
		return false, errors.NotFoundError("backend was not found")
	}
	volume := results[0].Volume.Read
	if volume == nil {
		return false, errors.NotFoundError("volume %v was not found", volumeName)
	}

	volumePublication := results[0].VolumePublication.Read
	if volumePublication == nil {
		// If the publication couldn't be found and Trident isn't in docker mode, bail out.
		// Otherwise, continue un-publishing. It is ok for the publication to not exist at this point for docker.
		if config.CurrentDriverContext != config.ContextDocker {
			Logc(ctx).WithError(errors.NotFoundError("unable to get volume publication record")).
				WithFields(fields).Warn("Volume is not published to node; doing nothing.")
			return false, nil
		}
	}

	publishInfo := &models.VolumePublishInfo{
		HostName:    nodeName,
		TridentUUID: o.uuid,
		HostNQN:     node.NQN,
		HostIP:      node.IPs,
	}

	nodesByName := make(map[string]*models.Node)
	for _, n := range results[0].Nodes {
		nodesByName[n.Name] = n
	}
	nodeMap := make(map[string]*models.Node)
	for _, pub := range results[0].VolumePublications {
		if pub.VolumeName == volumeName && pub.NodeName == nodeName {
			continue
		}
		nodeMap[pub.NodeName] = nodesByName[pub.NodeName]
	}
	nodes := make([]*models.Node, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	publishInfo.Nodes = nodes

	// Build list of NVMe namespace UUIDs that remain published to this host.
	// This is used to determine if the host should be removed from a SuperSubsystem.
	// Only collect namespace UUIDs if this volume is NVMe (has a namespace UUID).
	if volume.Config.AccessInfo.NVMeNamespaceUUID != "" {
		namespaceUUIDs := make([]string, 0)
		for _, vol := range results[1].Volumes {
			if vol.Config.Name == volumeName {
				continue
			}
			if vol.Config.AccessInfo.NVMeNamespaceUUID != "" {
				namespaceUUIDs = append(namespaceUUIDs, vol.Config.AccessInfo.NVMeNamespaceUUID)
			}
		}
		publishInfo.HostNVMeNamespaceUUIDs = namespaceUUIDs
	}

	if err := backend.UnpublishVolume(ctx, volume.Config, publishInfo); err != nil {
		if !errors.IsNotFoundError(err) {
			return false, err
		}
		Logc(ctx).Debug("Volume not found in backend during unpublish; continuing with unpublish.")
	}

	if volumePublication != nil {
		if err := o.storeClient.DeleteVolumePublication(ctx, volumePublication); err != nil {
			return false, err
		}
	}

	if err := o.updateVolumeInStoreAndCache(ctx, &results[0]); err != nil {
		return false, err
	}

	results[0].VolumePublication.Delete()
	return node.Deleted, nil
}

func (o *ConcurrentTridentOrchestrator) tryDeleteSoftDeletedNode(ctx context.Context, nodeName string) error {
	// Check that the node is still soft-deleted.
	results, unlocker, err := db.Lock(ctx, db.Query(db.DeleteNode(nodeName)))
	defer unlocker()
	if err != nil {
		return err
	}

	node := results[0].Node.Read
	if node == nil || !node.Deleted {
		// node is gone from cache or it's no longer deleted, nothing to do
		return nil
	}

	// Check if the node has any publications left
	listResults, listUnlocker, err := db.Lock(ctx, db.Query(db.ListVolumePublicationsForNode(nodeName)))
	defer listUnlocker()
	if err != nil {
		return err
	}

	if len(listResults[0].VolumePublications) == 0 {
		// No publications left, we can delete the node
		if err := o.storeClient.DeleteNode(ctx, node); err != nil {
			return fmt.Errorf("failed to delete node %s: %w", nodeName, err)
		}
		results[0].Node.Delete()
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) resizeVolume(ctx context.Context, volume *storage.Volume, newSize string) error {
	volName := volume.Config.Name
	backendUUID := volume.BackendUUID

	// Acquire write lock on the volume to resize it.
	results, unlocker, err := db.Lock(ctx, db.Query(
		db.UpsertVolume(volume.Config.Name, volume.BackendUUID),
		db.ReadBackend(""), // this is for populating the parent backend in the results
	))
	defer unlocker()
	if err != nil {
		return err
	}

	backend := results[0].Backend.Read
	volume = results[0].Volume.Read
	upsertVolumeInCache := results[0].Volume.Upsert

	if volume == nil {
		Logc(ctx).WithFields(LogFields{
			"volume":      volName,
			"backendUUID": backendUUID,
		}).Error("Unable to find volume during resize.")
		return fmt.Errorf("unable to find volume %v during resize", volName)
	}

	if backend == nil {
		Logc(ctx).WithFields(LogFields{
			"volume":      volName,
			"backendUUID": backendUUID,
		}).Error("Unable to find backend during volume resize.")
		return fmt.Errorf("unable to find backend %v during volume resize", volume.BackendUUID)
	}

	if volume.Config.Size != newSize {
		// If the resize is successful the driver updates the volume.Config.Size, as a side effect, with the actual
		// byte size of the expanded volume.
		if err := backend.ResizeVolume(ctx, volume.Config, newSize); err != nil {
			Logc(ctx).WithFields(LogFields{
				"volume":       volume.Config.Name,
				"internalName": volume.Config.InternalName,
				"backendUUID":  volume.BackendUUID,
				"currentSize":  volume.Config.Size,
				"newSize":      newSize,
				"error":        err,
			}).Error("Unable to resize the volume.")
			return fmt.Errorf("unable to resize the volume: %v", err)
		}
	}

	if err := o.storeClient.UpdateVolume(ctx, volume); err != nil {
		// It's ok not to revert volume size as we don't clean up the
		// transaction object in this situation.
		Logc(ctx).WithFields(LogFields{
			"volume": volume.Config.Name,
		}).Error("Unable to update the volume's size in persistent store.")
		return err
	}

	upsertVolumeInCache(volume)

	return nil
}

func (o *ConcurrentTridentOrchestrator) resizeSubordinateVolume(ctx context.Context, volumeName, newSize string) error {
	results, unlocker, err := db.Lock(ctx, db.Query(
		db.InconsistentReadSubordinateVolume(volumeName),
	))
	unlocker()
	if err != nil {
		return fmt.Errorf("error checking for existing subordinate volume; %w", err)
	}

	subordinateVolume := results[0].SubordinateVolume.Read
	if subordinateVolume == nil {
		return errors.NotFoundError("subordinate volume %s not found", volumeName)
	}

	sourceVolumeName := subordinateVolume.Config.ShareSourceVolume

	results, unlocker, err = db.Lock(ctx, db.Query(
		db.UpsertSubordinateVolume(volumeName, sourceVolumeName),
		db.ReadVolume(""),
	))
	defer unlocker()
	if err != nil {
		return err
	}

	sourceVolume := results[0].Volume.Read
	subordinateVolume = results[0].SubordinateVolume.Read
	updateSubVolumeInCache := results[0].SubordinateVolume.Upsert

	if subordinateVolume == nil {
		return errors.NotFoundError("subordinate volume %s not found", volumeName)
	}

	if sourceVolume == nil {
		return errors.NotFoundError("source volume %s for subordinate volume %s not found",
			sourceVolumeName, volumeName)
	}

	// Determine volume size in bytes
	newSizeStr, err := capacity.ToBytes(newSize)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s: %v", newSize, err)
	}
	newSizeBytes, err := strconv.ParseUint(newSizeStr, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", newSize, err)
	}

	// Determine source volume size in bytes
	sourceSizeStr, err := capacity.ToBytes(sourceVolume.Config.Size)
	if err != nil {
		return fmt.Errorf("could not convert source volume size %s: %v", sourceVolume.Config.Size, err)
	}
	sourceSizeBytes, err := strconv.ParseUint(sourceSizeStr, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid source volume size: %v", sourceVolume.Config.Size, err)
	}

	// Allow resizing subordinate volume up to the size of its source volume
	if newSizeBytes > sourceSizeBytes {
		return fmt.Errorf("subordinate volume %s may not be larger than source volume %s", volumeName,
			sourceVolumeName)
	}

	subordinateVolume.Config.Size = newSizeStr

	Logc(ctx).WithFields(LogFields{
		"volume":   subordinateVolume.Config.Name,
		"orphaned": subordinateVolume.Orphaned,
		"size":     subordinateVolume.Config.Size,
		"state":    string(subordinateVolume.State),
	}).Debug("Updating an existing volume.")

	err = o.storeClient.UpdateVolume(ctx, subordinateVolume)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume": subordinateVolume.Config.Name,
		}).Error("Unable to update the subordinate volume's size in persistent store.")
		return err
	}

	updateSubVolumeInCache(subordinateVolume)

	return nil
}

// resizeVolumeCleanup is used to clean up artifacts of volume resize in case
// anything goes wrong during the operation.
func (o *ConcurrentTridentOrchestrator) resizeVolumeCleanup(
	ctx context.Context, err error, vol *storage.Volume, volTxn *storage.VolumeTransaction,
) error {
	if vol == nil || err == nil || vol.Config.Size != volTxn.Config.Size {
		// We either succeeded or failed to resize the volume on the
		// backend. There are two possible failure cases:
		// 1.  We failed to resize the volume. In this case, we just need to
		//     remove the volume transaction.
		// 2.  We failed to update the volume on persistent store. In this
		//     case, we leave the volume transaction around so that we can
		//     update the persistent store later.
		txErr := o.DeleteVolumeTransaction(ctx, volTxn)
		if txErr != nil {
			txErr = fmt.Errorf("unable to clean up resize transaction:  %v", txErr)
			err = multierr.Combine(err, txErr)

			Logc(ctx).Warnf(
				"Unable to clean up artifacts of volume resize: %v. Repeat resizing the volume or restart %v.",
				err, config.OrchestratorName)
		}
	} else {
		// We get here only when we fail to update the volume size in
		// persistent store after successfully updating the volume on the
		// backend. We leave the transaction object around so that the
		// persistent store can be updated in the future through retries.
	}
	return err
}

func (o *ConcurrentTridentOrchestrator) ResizeVolume(ctx context.Context, volumeName, newSize string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_resize", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(
		db.InconsistentReadVolume(volumeName),
		db.InconsistentReadSubordinateVolume(volumeName),
	))
	unlocker()
	if err != nil {
		return err
	}

	volume := results[0].Volume.Read
	subordinateVolume := results[0].SubordinateVolume.Read

	if subordinateVolume != nil {
		return o.resizeSubordinateVolume(ctx, volumeName, newSize)
	}

	if volume == nil {
		return errors.NotFoundError("volume %s not found", volumeName)
	}

	if volume.Orphaned {
		Logc(ctx).WithFields(LogFields{
			"volume":      volumeName,
			"backendUUID": volume.BackendUUID,
		}).Warnf("Resize operation is likely to fail with an orphaned volume.")
	}

	if volume.State.IsDeleting() {
		return errors.VolumeStateError(fmt.Sprintf("volume %s is deleting", volumeName))
	}

	// Create a new config for the volume transaction
	cloneConfig := volume.Config.ConstructClone()
	cloneConfig.Size = newSize

	// Add a transaction in case the operation must be retried during bootstrapping.
	volTxn := &storage.VolumeTransaction{
		Config: cloneConfig,
		Op:     storage.ResizeVolume,
	}

	// Acquire lock on the transaction name to avoid race condition of multiple concurrent resize requests handling transactions
	o.txnMutex.Lock(volTxn.Name())
	defer o.txnMutex.Unlock(volTxn.Name())

	if err = o.AddVolumeTransaction(ctx, volTxn); err != nil {
		return err
	}

	defer func() {
		if err == nil {
			Logc(ctx).WithFields(LogFields{
				"volume":      volumeName,
				"volume_size": newSize,
			}).Info("Orchestrator resized the volume on the storage backend.")
		}
		err = o.resizeVolumeCleanup(ctx, err, volume, volTxn)
	}()

	// Resize the volume.
	return o.resizeVolume(ctx, volume, newSize)
}

func (o *ConcurrentTridentOrchestrator) ReloadVolumes(ctx context.Context) error {
	return fmt.Errorf("concurrent core is not supported for Docker")
}

// addSubordinateVolume defines a new subordinate volume and updates all references to it.  It does not create any
// backing storage resources.
func (o *ConcurrentTridentOrchestrator) addSubordinateVolume(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (externalVol *storage.VolumeExternal, err error) {
	// Check for existing volumes
	if err = func(volumeName string) error {
		results, unlocker, err := db.Lock(ctx, db.Query(
			db.InconsistentReadVolume(volumeName),
			db.InconsistentReadSubordinateVolume(volumeName)))
		defer unlocker()
		if err != nil {
			return fmt.Errorf("error checking for existing volume %s; %w", volumeName, err)
		}

		extantVolume := results[0].Volume.Read
		extantSubordinateVolume := results[0].SubordinateVolume.Read

		// Check if the volume already exists
		if extantVolume != nil {
			return fmt.Errorf("volume %s exists but is not a subordinate volume", volumeName)
		}
		if extantSubordinateVolume != nil {
			return fmt.Errorf("subordinate volume %s already exists", volumeName)
		}
		return nil
	}(volumeConfig.Name); err != nil {
		return nil, err
	}

	// Ensure requested size is valid
	volumeSizeStr, err := capacity.ToBytes(volumeConfig.Size)
	if err != nil {
		return nil, fmt.Errorf("invalid size for subordinate volume %s", volumeConfig.Name)
	}
	volumeSize, err := strconv.ParseUint(volumeSizeStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid size for subordinate volume %s", volumeConfig.Name)
	}

	// Disallow illegal conditions for subordinate volumes
	if volumeConfig.CloneSourceVolume != "" {
		return nil, fmt.Errorf("subordinate volume may not be a clone")
	}
	if volumeConfig.IsMirrorDestination {
		return nil, fmt.Errorf("subordinate volume may not be part of a mirror")
	}
	if volumeConfig.ImportOriginalName != "" {
		return nil, fmt.Errorf("subordinate volume may not be imported")
	}

	// Get locks for the subordinate volume creation
	results, unlocker, err := db.Lock(
		ctx,
		db.Query(
			db.UpsertSubordinateVolume(volumeConfig.Name, volumeConfig.ShareSourceVolume),
			db.UpsertVolume("", ""),
			db.ReadBackend("")),
		db.Query(db.InconsistentReadSubordinateVolume(volumeConfig.ShareSourceVolume)),
	)
	defer unlocker()
	if err != nil {
		return nil, fmt.Errorf("error getting locks for volume %s; %w", volumeConfig.Name, err)
	}

	subordinateVolume := results[0].SubordinateVolume.Read
	subordinateVolumeUpserter := results[0].SubordinateVolume.Upsert
	sourceVolume := results[0].Volume.Read
	backend := results[0].Backend.Read
	sourceVolumeUpserter := results[0].Volume.Upsert
	sourceVolumeAsSubordinate := results[1].SubordinateVolume.Read

	// Ensure the source exists, is not a subordinate volume, is in a good state, and matches the new volume as needed
	if sourceVolume == nil {
		if sourceVolumeAsSubordinate != nil {
			return nil, errors.NotFoundError("creating subordinate to a subordinate volume %s is not allowed",
				volumeConfig.ShareSourceVolume)
		}
		return nil, errors.NotFoundError("source volume %s for subordinate volume %s not found",
			volumeConfig.ShareSourceVolume, volumeConfig.Name)
	} else if subordinateVolume != nil {
		return nil, fmt.Errorf("subordinate volume %s already exists", volumeConfig.Name)
	} else if sourceVolume.Config.ImportNotManaged {
		return nil, fmt.Errorf("source volume %s for subordinate volume %s is an unmanaged import",
			volumeConfig.ShareSourceVolume, volumeConfig.Name)
	} else if sourceVolume.Config.StorageClass != volumeConfig.StorageClass {
		return nil, fmt.Errorf("source volume %s for subordinate volume %s has a different storage class",
			volumeConfig.ShareSourceVolume, volumeConfig.Name)
	} else if sourceVolume.Orphaned {
		return nil, fmt.Errorf("source volume %s for subordinate volume %s is orphaned",
			volumeConfig.ShareSourceVolume, volumeConfig.Name)
	} else if sourceVolume.State != storage.VolumeStateOnline {
		return nil, fmt.Errorf("source volume %s for subordinate volume %s is not online",
			volumeConfig.ShareSourceVolume, volumeConfig.Name)
	}

	// Get the backend and ensure it and the source volume are NFS
	if backend == nil {
		return nil, errors.NotFoundError("backend %s for source volume %s not found",
			sourceVolume.BackendUUID, sourceVolume.Config.Name)
	} else if backend.GetProtocol(ctx) != config.File || sourceVolume.Config.AccessInfo.NfsPath == "" {
		return nil, fmt.Errorf("source volume %s for subordinate volume %s must be NFS",
			volumeConfig.ShareSourceVolume, volumeConfig.Name)
	}

	// Ensure requested volume size is not larger than the source
	sourceVolumeSizeStr, err := capacity.ToBytes(sourceVolume.Config.Size)
	if err != nil {
		return nil, fmt.Errorf("invalid size for source volume %s", sourceVolume.Config.Name)
	}
	sourceVolumeSize, err := strconv.ParseUint(sourceVolumeSizeStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid size for source volume %s", sourceVolume.Config.Name)
	}
	if sourceVolumeSize < volumeSize {
		return nil, fmt.Errorf("source volume %s may not be smaller than subordinate volume %s",
			volumeConfig.ShareSourceVolume, volumeConfig.Name)
	}

	// Copy a few needed fields from the source volume
	volumeConfig.AccessInfo = sourceVolume.Config.AccessInfo
	volumeConfig.FileSystem = sourceVolume.Config.FileSystem
	volumeConfig.MountOptions = sourceVolume.Config.MountOptions
	volumeConfig.Protocol = sourceVolume.Config.Protocol
	volumeConfig.AccessInfo.PublishEnforcement = true

	newSubordinateVolume := storage.NewVolume(volumeConfig, sourceVolume.BackendUUID, sourceVolume.Pool, false,
		storage.VolumeStateSubordinate)

	// Add new volume to persistent store
	if err = o.storeClient.AddVolume(ctx, newSubordinateVolume); err != nil {
		return nil, err
	}

	_, _ = o.resolveAndSetEffectiveAutogrowPolicy(ctx, newSubordinateVolume, "subordinate volume creation", "warn")

	// Register subordinate volume with source volume
	if sourceVolume.Config.SubordinateVolumes == nil {
		sourceVolume.Config.SubordinateVolumes = make(map[string]interface{})
	}
	sourceVolume.Config.SubordinateVolumes[newSubordinateVolume.Config.Name] = nil

	// Update cache
	subordinateVolumeUpserter(newSubordinateVolume)
	sourceVolumeUpserter(sourceVolume)

	return newSubordinateVolume.ConstructExternal(), nil
}

// deleteSubordinateVolume removes all references to a subordinate volume.  It does not delete any
// backing storage resources.
func (o *ConcurrentTridentOrchestrator) deleteSubordinateVolume(ctx context.Context, volumeName string) (err error) {
	// Get locks for the subordinate volume deletion
	results, unlocker, err := db.Lock(ctx, db.Query(db.DeleteSubordinateVolume(volumeName), db.UpsertVolume("", "")))
	if err != nil {
		unlocker()
		return fmt.Errorf("error getting locks for volume %s; %w", volumeName, err)
	}

	subordinateVolume := results[0].SubordinateVolume.Read
	subordinateVolumeDeleter := results[0].SubordinateVolume.Delete
	sourceVolume := results[0].Volume.Read
	sourceVolumeUpserter := results[0].Volume.Upsert

	if subordinateVolume == nil {
		unlocker()
		return errors.NotFoundError("subordinate volume %s not found", volumeName)
	}

	// Remove volume from persistent store
	if err = o.storeClient.DeleteVolume(ctx, subordinateVolume); err != nil {
		unlocker()
		return err
	}

	// Delete subordinate volume from cache
	subordinateVolumeDeleter()

	shouldDeleteSourceVolume := false
	if sourceVolume != nil {
		// Unregister subordinate volume with source volume
		if sourceVolume.Config.SubordinateVolumes != nil {
			delete(sourceVolume.Config.SubordinateVolumes, volumeName)
		}
		if len(sourceVolume.Config.SubordinateVolumes) == 0 {
			sourceVolume.Config.SubordinateVolumes = nil
		}

		// Update source volume in cache
		sourceVolumeUpserter(sourceVolume)

		// If this subordinate volume pinned its source volume in Deleting state, we want to
		// clean up the source volume if it isn't still pinned by something else (snapshots).
		if sourceVolume.State.IsDeleting() && len(sourceVolume.Config.SubordinateVolumes) == 0 {
			shouldDeleteSourceVolume = true
		}
	}

	unlocker()

	if shouldDeleteSourceVolume {
		if o.deleteVolume(ctx, sourceVolume.Config.Name) != nil {
			Logc(ctx).WithField("name", sourceVolume.Config.Name).Warning(
				"Could not delete volume in deleting state.")
		}
	}

	return nil
}

// ListSubordinateVolumes returns all subordinate volumes for all source volumes, or all subordinate
// volumes for a single source volume.
func (o *ConcurrentTridentOrchestrator) ListSubordinateVolumes(
	ctx context.Context, sourceVolumeName string,
) (volumesExt []*storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("subordinate_volume_list", &err)()

	var (
		results  []db.Result
		unlocker func()
	)

	if sourceVolumeName == "" {
		// List all subordinate volumes
		results, unlocker, err = db.Lock(ctx, db.Query(db.ListSubordinateVolumes()))
		defer unlocker()
		if err != nil {
			return nil, err
		}
	} else {
		// List subordinate volumes for a single source volume
		results, unlocker, err = db.Lock(ctx, db.Query(
			db.ListSubordinateVolumesForVolume(sourceVolumeName),
			db.InconsistentReadVolume(sourceVolumeName)))
		defer unlocker()
		if err != nil {
			return nil, err
		}
		if results[0].Volume.Read == nil {
			return nil, errors.NotFoundError("source volume %s not found", sourceVolumeName)
		}
	}

	volumes := results[0].SubordinateVolumes
	volumesExt = make([]*storage.VolumeExternal, 0, len(volumes))

	for _, volume := range volumes {
		volumesExt = append(volumesExt, volume.ConstructExternal())
	}

	sort.Sort(storage.ByVolumeExternalName(volumesExt))

	return volumesExt, nil
}

// GetSubordinateSourceVolume returns the parent volume for a given subordinate volume.
func (o *ConcurrentTridentOrchestrator) GetSubordinateSourceVolume(
	ctx context.Context, subordinateVolumeName string,
) (volume *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("subordinate_source_volume_get", &err)()

	// Get subordinate volume
	results, subordinateUnlocker, err := db.Lock(ctx, db.Query(db.InconsistentReadSubordinateVolume(subordinateVolumeName)))
	defer subordinateUnlocker()
	if err != nil {
		return nil, err
	}
	subordinateVolume := results[0].SubordinateVolume.Read
	if subordinateVolume == nil {
		return nil, errors.NotFoundError("subordinate volume %s not found", subordinateVolumeName)
	}

	parentVolumeName := subordinateVolume.Config.ShareSourceVolume

	results, parentUnlocker, err := db.Lock(ctx, db.Query(db.InconsistentReadVolume(parentVolumeName)))
	defer parentUnlocker()
	if err != nil {
		return nil, err
	}
	parentVolume := results[0].Volume.Read
	if parentVolume == nil {
		return nil, errors.NotFoundError("subordinate parent volume %s not found", parentVolumeName)
	}

	return parentVolume.ConstructExternal(), nil
}

// addSnapshotCleanup is used as a deferred method from the snapshot create method
// to clean up in case anything goes wrong during the operation.
func (o *ConcurrentTridentOrchestrator) addSnapshotCleanup(
	ctx context.Context, snapshotErr, err error, backend storage.Backend, snapshot *storage.Snapshot,
	volTxn *storage.VolumeTransaction, snapConfig *storage.SnapshotConfig,
) error {
	var cleanupErr, txErr error
	if snapshotErr != nil {
		// We failed somewhere.  There are two possible cases:
		// 1.  We failed to create a snapshot and fell through to the
		//     end of the function.  In this case, we don't need to roll
		//     anything back.
		// 2.  We failed to save the snapshot to the persistent store.
		//     In this case, we need to remove the snapshot from the backend.
		//     No need to delete the snapshot from the cache, as we only add to cache after successfully
		//     adding the snapshot to persistence.
		if backend != nil && snapshot != nil {
			// We succeeded in adding the snapshot to the backend; now delete it.
			cleanupErr = backend.DeleteSnapshot(ctx, snapshot.Config, volTxn.Config)
			if cleanupErr != nil {
				cleanupErr = fmt.Errorf("unable to delete snapshot from backend during cleanup:  %v", cleanupErr)
			}
		}
	}
	if cleanupErr == nil {
		// Only clean up the snapshot transaction if we've succeeded at
		// cleaning up on the backend or if we didn't need to do so in the
		// first place.
		if txErr = o.DeleteVolumeTransaction(ctx, volTxn); txErr != nil {
			txErr = fmt.Errorf("unable to clean up snapshot transaction: %v", txErr)
		}
	}

	combinedErr := multierr.Combine(err, snapshotErr, cleanupErr, txErr)
	if cleanupErr != nil || txErr != nil {
		Logc(ctx).Warnf(
			"Unable to clean up artifacts of snapshot creation: %v. Repeat creating the snapshot or restart %v.",
			combinedErr, config.OrchestratorName)
	}

	return combinedErr
}

func (o *ConcurrentTridentOrchestrator) CreateSnapshot(
	ctx context.Context, snapshotConfig *storage.SnapshotConfig,
) (*storage.SnapshotExternal, error) {
	var (
		backend          storage.Backend
		volume           *storage.Volume
		snapshot         *storage.Snapshot
		snapshotErr, err error
	)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("snapshot_create", &err)()

	// inconsistent read volume
	results, unlocker, err := db.Lock(ctx, db.Query(
		db.InconsistentReadVolume(snapshotConfig.VolumeName),
		db.InconsistentReadSubordinateVolume(snapshotConfig.VolumeName),
	))
	unlocker()
	if err != nil {
		return nil, err
	}

	volume = results[0].Volume.Read
	subordinateVolume := results[0].SubordinateVolume.Read

	if volume == nil {
		if subordinateVolume != nil {
			return nil, errors.NotFoundError("creating snapshot is not allowed on subordinate volume %s",
				snapshotConfig.VolumeName)
		}
		return nil, errors.NotFoundError("source volume %s not found", snapshotConfig.VolumeName)
	}

	// Complete the snapshot config
	snapshotConfig.InternalName = snapshotConfig.Name
	snapshotConfig.VolumeInternalName = volume.Config.InternalName
	snapshotConfig.LUKSPassphraseNames = volume.Config.LUKSPassphraseNames

	// Add transaction in case the operation must be rolled back later
	txn := &storage.VolumeTransaction{
		Config:         volume.Config,
		SnapshotConfig: snapshotConfig,
		Op:             storage.AddSnapshot,
	}

	// Acquire the txn lock before acquiring any cache locks to avoid deadlocks
	o.txnMutex.Lock(txn.Name())
	defer o.txnMutex.Unlock(txn.Name())

	if err = o.AddVolumeTransaction(ctx, txn); err != nil {
		return nil, err
	}

	// Acquire the read lock on backend and parent volume.
	// Acquire the write lock on the snapshot.
	// Snapshots are independent resources. It is ok to allow multiple snapshot requests for the same volume.
	results, unlocker, err = db.Lock(ctx, db.Query(
		db.UpsertSnapshot(snapshotConfig.VolumeName, snapshotConfig.ID()),
		db.ReadBackend(""), // this is for populating the parent backend in the results
		db.ReadVolume(""),  // this is for populating the parent volume in the results
	))
	defer unlocker()
	if err != nil {
		// We still need to clean up the txn
		if txErr := o.DeleteVolumeTransaction(ctx, txn); txErr != nil {
			return nil, fmt.Errorf("unable to clean up snapshot transaction: %v", err)
		}
		return nil, err
	}

	backend = results[0].Backend.Read
	volume = results[0].Volume.Read
	snapshot = results[0].Snapshot.Read
	upsertSnapshotInCache := results[0].Snapshot.Upsert

	// Recovery function in case of error
	defer func() {
		err = o.addSnapshotCleanup(ctx, snapshotErr, err, backend, snapshot, txn, snapshotConfig)
	}()

	// Check if the snapshot already exists
	if snapshot != nil {
		return nil, fmt.Errorf("snapshot %s already exists", snapshotConfig.ID())
	}

	if volume.State.IsDeleting() {
		return nil, errors.VolumeStateError(fmt.Sprintf("source volume %s is deleting", snapshotConfig.VolumeName))
	}

	// Get the backend
	if backend == nil {
		// Should never get here but just to be safe
		return nil, errors.NotFoundError("backend %s for the source volume not found: %s",
			volume.BackendUUID, snapshotConfig.VolumeName)
	}

	// Ensure a snapshot is even possible before creating the transaction
	if err = backend.CanSnapshot(ctx, snapshotConfig, volume.Config); err != nil {
		return nil, err
	}

	// Create the snapshot
	snapshot, snapshotErr = backend.CreateSnapshot(ctx, snapshotConfig, volume.Config)
	if snapshotErr != nil {
		if errors.IsMaxLimitReachedError(snapshotErr) {
			snapshotErr = errors.MaxLimitReachedError(fmt.Sprintf("failed to create snapshot %s for volume %s on backend %s: %v",
				snapshotConfig.Name, snapshotConfig.VolumeName, backend.Name(), snapshotErr))
			return nil, snapshotErr
		}
		snapshotErr = fmt.Errorf("failed to create snapshot %s for volume %s on backend %s: %v",
			snapshotConfig.Name, snapshotConfig.VolumeName, backend.Name(), snapshotErr)
		return nil, snapshotErr
	}

	// Save new snapshot in persistence
	if snapshotErr = o.storeClient.AddSnapshot(ctx, snapshot); snapshotErr != nil {
		return nil, snapshotErr
	}

	// Save new snapshot in cache
	upsertSnapshotInCache(snapshot)

	return snapshot.ConstructExternal(), nil
}

func (o *ConcurrentTridentOrchestrator) ImportSnapshot(
	ctx context.Context, snapshotConfig *storage.SnapshotConfig,
) (externalSnapshot *storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)
	fields := LogFields{
		"snapshotID":   snapshotConfig.ID(),
		"snapshotName": snapshotConfig.Name,
		"volumeName":   snapshotConfig.VolumeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>>>> Orchestrator#ImportSnapshot")
	defer Logc(ctx).Debug("<<<<<< Orchestrator#ImportSnapshot")

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("snapshot_import", &err)()

	// Fail immediately if the internal name isn't set.
	if snapshotConfig.InternalName == "" {
		return nil, errors.InvalidInputError("snapshot internal name not found")
	}

	fields["internalName"] = snapshotConfig.InternalName

	var (
		backend  storage.Backend
		volume   *storage.Volume
		snapshot *storage.Snapshot
	)

	// Acquire the read lock on backend and parent volume.
	// Acquire the write lock on the snapshot.
	results, unlocker, err := db.Lock(ctx, db.Query(
		db.UpsertSnapshot(snapshotConfig.VolumeName, snapshotConfig.ID()),
		db.ReadBackend(""), // this is for populating the parent backend in the results
		db.ReadVolume(""),  // this is for populating the parent volume in the results
		db.InconsistentReadSubordinateVolume(snapshotConfig.VolumeName),
	))
	defer unlocker()
	if err != nil {
		return nil, err
	}

	backend = results[0].Backend.Read
	volume = results[0].Volume.Read
	subordinateVolume := results[0].SubordinateVolume.Read
	snapshot = results[0].Snapshot.Read
	upsertSnapshot := results[0].Snapshot.Upsert

	if volume == nil {
		if subordinateVolume != nil {
			return nil, errors.NotFoundError("importing snapshot is not allowed on subordinate volume %s",
				snapshotConfig.VolumeName)
		}
		return nil, errors.NotFoundError("volume %v was not found", snapshotConfig.VolumeName)
	}

	// Check if the snapshot already exists
	if snapshot != nil {
		Logc(ctx).WithFields(fields).Warn("Snapshot already exists.")
		return nil, errors.FoundError("snapshot %s already exists", snapshotConfig.ID())
	}

	if backend == nil {
		return nil, errors.NotFoundError("backend %s for volume %s not found",
			volume.BackendUUID, snapshotConfig.VolumeName)
	}
	fields["backendName"] = backend.Name()

	// Complete the snapshot config.
	snapshotConfig.VolumeInternalName = volume.Config.InternalName
	snapshotConfig.LUKSPassphraseNames = volume.Config.LUKSPassphraseNames
	snapshotConfig.ImportNotManaged = volume.Config.ImportNotManaged // Snapshots inherit the managed state of their volume

	// Query the storage backend for the snapshot.
	snapshot, err = backend.GetSnapshot(ctx, snapshotConfig, volume.Config)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Failed to get snapshot from backend.")
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithFields(fields).WithError(err).Warn("Snapshot not found on backend.")
			return nil, err
		}
		return nil, fmt.Errorf(
			"failed to get snapshot %s from backend %s; %w", snapshotConfig.InternalName, backend.Name(), err,
		)
	}

	// Save references to the snapshot.
	if err = o.storeClient.AddSnapshot(ctx, snapshot); err != nil {
		return nil, fmt.Errorf("failed to store snapshot %s info; %v", snapshot.Config.Name, err)
	}

	// update the cache
	upsertSnapshot(snapshot)

	return snapshot.ConstructExternal(), nil
}

func (o *ConcurrentTridentOrchestrator) GetSnapshot(ctx context.Context, volumeName, snapshotName string,
) (snapshotExternal *storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	defer recordTiming("snapshot_get", &err)()

	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)

	results, unlocker, err := db.Lock(ctx, db.Query(db.InconsistentReadSnapshot(snapshotID)))
	unlocker()
	if err != nil {
		return nil, err
	}

	snapshot := results[0].Snapshot.Read

	if snapshot == nil {
		return nil, errors.NotFoundError("snapshot %v was not found", snapshotName)
	} else if snapshot.State != storage.SnapshotStateCreating && snapshot.State != storage.SnapshotStateUploading {
		return snapshot.ConstructExternal(), nil
	}

	// The snapshot state is either 'creating' or 'uploading'.
	// Fetch the latest snapshot from the backend and update the cache and persistence.
	if updatedSnapshot, err := o.fetchAndUpdateSnapshot(ctx, snapshot); err != nil {
		return nil, err
	} else {
		return updatedSnapshot.ConstructExternal(), nil
	}
}

func (o *ConcurrentTridentOrchestrator) fetchAndUpdateSnapshot(
	ctx context.Context, snapshot *storage.Snapshot,
) (*storage.Snapshot, error) {
	snapshotID := snapshot.Config.ID()

	// Acquire the read lock on backend and parent volume.
	// Acquire the write lock on the snapshot.
	results, unlocker, err := db.Lock(ctx, db.Query(
		db.UpsertSnapshot(snapshot.Config.VolumeName, snapshotID),
		db.ReadBackend(""), // this is for populating the parent backend in the results
		db.ReadVolume(""),  // this is for populating the parent volume in the results
	))
	defer unlocker()
	if err != nil {
		return nil, err
	}

	backend := results[0].Backend.Read
	volume := results[0].Volume.Read
	cachedSnapshot := results[0].Snapshot.Read
	upsertSnapshotInCache := results[0].Snapshot.Upsert

	if volume == nil {
		return snapshot, errors.NotFoundError("volume %s not found", snapshot.Config.VolumeName)
	}

	if backend == nil {
		return snapshot, errors.NotFoundError("backend %s not found", volume.BackendUUID)
	}

	// the snapshot could have been deleted.
	if cachedSnapshot == nil {
		return nil, errors.NotFoundError("snapshot %s not found", snapshotID)
	}

	// We already have a read lock on the backend, which should prevent another thread from acquiring a write lock on
	// the backend and modifying it.
	// It should be safe to call backend.GetSnapshot() here.
	latestSnapshot, err := backend.GetSnapshot(ctx, cachedSnapshot.Config, volume.Config)
	if err != nil {
		return nil, err
	}

	// If the snapshot state has changed, persist it here and in the store
	if latestSnapshot != nil && latestSnapshot.State != snapshot.State {

		if err := o.storeClient.UpdateSnapshot(ctx, latestSnapshot); err != nil {
			Logc(ctx).Errorf("could not update snapshot %s in persistent store; %v", snapshotID, err)
			return nil, err
		}

		upsertSnapshotInCache(latestSnapshot)
	}

	return latestSnapshot, nil
}

func (o *ConcurrentTridentOrchestrator) ListSnapshots(
	ctx context.Context,
) (externalSnapshots []*storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("snapshot_list", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.ListSnapshots()))
	defer unlocker()
	if err != nil {
		return nil, err
	}
	cachedSnapshots := results[0].Snapshots

	externalSnapshots = make([]*storage.SnapshotExternal, 0, len(cachedSnapshots))
	for _, s := range cachedSnapshots {
		externalSnapshots = append(externalSnapshots, s.ConstructExternal())
	}

	sort.Sort(storage.BySnapshotExternalID(externalSnapshots))
	return externalSnapshots, nil
}

func (o *ConcurrentTridentOrchestrator) ListSnapshotsByName(
	ctx context.Context, snapshotName string,
) (externalSnapshots []*storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	defer recordTiming("snapshot_list_by_snapshot_name", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.ListSnapshotsByName(snapshotName)))
	defer unlocker()
	if err != nil {
		return nil, err
	}
	cachedSnapshots := results[0].Snapshots

	externalSnapshots = make([]*storage.SnapshotExternal, 0, len(cachedSnapshots))
	for _, s := range cachedSnapshots {
		externalSnapshots = append(externalSnapshots, s.ConstructExternal())
	}

	sort.Sort(storage.BySnapshotExternalID(externalSnapshots))
	return externalSnapshots, nil
}

func (o *ConcurrentTridentOrchestrator) ListSnapshotsForVolume(
	ctx context.Context, volumeName string,
) (externalSnapshots []*storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	defer recordTiming("snapshot_list_by_volume_name", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.ReadVolume(volumeName), db.ListSnapshotsForVolume(volumeName)))
	defer unlocker()
	if err != nil {
		return nil, err
	}

	volume := results[0].Volume.Read
	cachedSnapshots := results[0].Snapshots

	if volume == nil {
		return nil, errors.NotFoundError("volume %s not found", volumeName)
	}

	externalSnapshots = make([]*storage.SnapshotExternal, 0, len(cachedSnapshots))
	for _, s := range cachedSnapshots {
		externalSnapshots = append(externalSnapshots, s.ConstructExternal())
	}

	sort.Sort(storage.BySnapshotExternalID(externalSnapshots))
	return externalSnapshots, nil
}

func (o *ConcurrentTridentOrchestrator) ReadSnapshotsForVolume(
	ctx context.Context, volumeName string,
) (externalSnapshots []*storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	defer recordTiming("snapshot_read_by_volume", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.InconsistentReadVolume(volumeName)))
	unlocker()
	if err != nil {
		return nil, err
	}

	volume := results[0].Volume.Read

	if volume == nil {
		return nil, errors.NotFoundError("volume %s not found", volumeName)
	}

	results, unlocker, err = db.Lock(ctx, db.Query(db.ReadBackend(volume.BackendUUID)))
	defer unlocker()
	if err != nil {
		return nil, err
	}

	backend := results[0].Backend.Read
	if backend == nil {
		return nil, errors.NotFoundError("backend %s not found", volume.BackendUUID)
	}

	// We already have a read lock on the backend, which should prevent another thread from acquiring a write lock on
	// the backend and modifying it.
	// It should be safe to call backend.GetSnapshot() here.
	snapshots, err := backend.GetSnapshots(ctx, volume.Config)
	if err != nil {
		return nil, err
	}

	externalSnapshots = make([]*storage.SnapshotExternal, 0)
	for _, snapshot := range snapshots {
		externalSnapshots = append(externalSnapshots, snapshot.ConstructExternal())
	}

	sort.Sort(storage.BySnapshotExternalID(externalSnapshots))
	return externalSnapshots, nil
}

// RestoreSnapshot restores a volume to the specified snapshot.  The caller is responsible for ensuring this is
// the newest snapshot known to the container orchestrator.  Any other snapshots that are newer than the specified
// one that are not known to the container orchestrator may be lost.
func (o *ConcurrentTridentOrchestrator) RestoreSnapshot(
	ctx context.Context, volumeName, snapshotName string,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("snapshot_restore", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.InconsistentReadVolume(volumeName)))
	unlocker()
	if err != nil {
		return err
	}

	volume := results[0].Volume.Read

	if volume == nil {
		return errors.NotFoundError("volume %s not found", volumeName)
	}

	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)

	// Acquire read lock on backend
	// Acquire read lock on snapshot
	// Acquire write lock on parent volume because we are restoring the volume to a snapshot
	results, unlocker, err = db.Lock(
		ctx,
		db.Query(
			db.ListVolumePublicationsForVolume(volumeName),
			db.ReadSnapshot(snapshotID),
			db.ReadBackend(""), // this is for populating the parent backend in the results
			db.ReadVolume(""),  // this is for populating the parent volume in the results
		),
		db.Query(
			// This is to acquire the write lock on the volume while we restore the volume to the specified snapshot. We
			// are not going to update the volume.
			db.UpsertVolume(volumeName, volume.BackendUUID),
		),
	)
	defer unlocker()
	if err != nil {
		return err
	}

	volumePublicationsForVolume := results[0].VolumePublications
	volume = results[0].Volume.Read
	snapshot := results[0].Snapshot.Read
	backend := results[0].Backend.Read

	if snapshot == nil {
		return errors.NotFoundError("snapshot %s not found on volume %s", snapshotName, volumeName)
	}

	if backend == nil {
		return errors.NotFoundError("backend %s not found", volume.BackendUUID)
	}

	logFields := LogFields{
		"volume":   snapshot.Config.VolumeName,
		"snapshot": snapshot.Config.Name,
		"backend":  backend.Name(),
	}

	// Ensure volume is not attached to any containers
	if len(volumePublicationsForVolume) > 0 {
		err = errors.New("cannot restore attached volume to snapshot")
		Logc(ctx).WithFields(logFields).WithError(err).Error("Unable to restore volume to snapshot.")
		return err
	}

	// Restore the snapshot
	if err = backend.RestoreSnapshot(ctx, snapshot.Config, volume.Config); err != nil {
		Logc(ctx).WithFields(logFields).WithError(err).Error("Unable to restore volume to snapshot.")
		return err
	}

	Logc(ctx).WithFields(logFields).Info("Restored volume to snapshot.")
	return nil
}

func (o *ConcurrentTridentOrchestrator) deleteSnapshot(ctx context.Context, volumeName, snapshotName string) (err error) {
	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)

	// Acquire read lock on backend
	// Acquire read lock on parent volume
	// Acquire write lock on snapshot
	// We also need the entire list of volumes and the list of snapshots for the parent volume.
	results, unlocker, err := db.Lock(ctx, db.Query(
		db.ListReadOnlyCloneVolumes(),
		db.ReadBackend(""), // this is for populating the parent backend in the results
		db.ReadVolume(""),  // this is for populating the parent volume in the results
		db.DeleteSnapshot(snapshotID)))
	defer unlocker()
	if err != nil {
		return err
	}

	allReadOnlyCloneVolumes := results[0].Volumes
	backend := results[0].Backend.Read
	volume := results[0].Volume.Read
	snapshot := results[0].Snapshot.Read
	deleteSnapshotInCache := results[0].Snapshot.Delete

	if snapshot == nil {
		return errors.NotFoundError("snapshot %s not found on volume %s", snapshotName, volumeName)
	}

	if volume == nil {
		if !snapshot.State.IsMissingVolume() {
			return errors.NotFoundError("volume %s not found", volumeName)
		}
	}

	// Note that this block will only be entered in the case that the snapshot
	// is missing it's volume and the volume is nil. If the volume does not
	// exist, delete the snapshot and clean up, then return.
	if volume == nil {
		if err = o.storeClient.DeleteSnapshot(ctx, snapshot); err != nil {
			return err
		}
		deleteSnapshotInCache()
		return nil
	}

	// Check if the snapshot is a source for a read-only volume. If so, return error.
	for _, vol := range allReadOnlyCloneVolumes {
		if vol.Config.CloneSourceSnapshot == snapshotName {
			return fmt.Errorf("unable to delete snapshot %s as it is a source for read-only clone %s", snapshotName,
				vol.Config.Name)
		}
	}

	if backend == nil {
		if !snapshot.State.IsMissingBackend() {
			return errors.NotFoundError("backend %s not found", volume.BackendUUID)
		}
	}

	// Note that this block will only be entered in the case that the snapshot
	// is missing it's backend and the backend is nil. If the backend does not
	// exist, delete the snapshot and clean up, then return.
	if backend == nil {
		if err = o.storeClient.DeleteSnapshot(ctx, snapshot); err != nil {
			return err
		}
		deleteSnapshotInCache()
		return nil
	}

	// TODO: Is this needed?
	if volume.Orphaned {
		Logc(ctx).WithFields(LogFields{
			"volume":   volumeName,
			"snapshot": snapshotName,
			"backend":  volume.BackendUUID,
		}).Warnf("Delete operation is likely to fail with an orphaned volume.")
	}

	// Note that this call will only return an error if the backend actually
	// fails to delete the snapshot. If the snapshot does not exist on the backend,
	// the driver will not return an error. Thus, we're fine.
	if err = backend.DeleteSnapshot(ctx, snapshot.Config, volume.Config); err != nil {
		fields := LogFields{
			"snapshotID":  snapshotID,
			"volume":      volume.Config.Name,
			"backendUUID": volume.BackendUUID,
			"backendName": backend.Name(),
			"error":       err,
		}
		if !errors.IsNotManagedError(err) {
			Logc(ctx).WithFields(fields).Error("Unable to delete snapshot from backend.")
			return err
		}
		Logc(ctx).WithFields(fields).Debug("Skipping backend deletion of snapshot.")
	}

	if err = o.storeClient.DeleteSnapshot(ctx, snapshot); err != nil {
		return err
	}

	deleteSnapshotInCache()

	return nil
}

func (o *ConcurrentTridentOrchestrator) deleteSnapshotCleanup(
	ctx context.Context, err error, volTxn *storage.VolumeTransaction,
) error {
	errTxn := o.DeleteVolumeTransaction(ctx, volTxn)
	if errTxn != nil {
		Logc(ctx).WithFields(LogFields{
			"error":     errTxn,
			"operation": volTxn.Op,
		}).Warnf("Unable to delete snapshot transaction. Repeat deletion using %s or restart %v.",
			config.OrchestratorClientName, config.OrchestratorName)
	}

	combinedErr := multierr.Combine(err, errTxn)

	return combinedErr
}

func (o *ConcurrentTridentOrchestrator) DeleteSnapshot(
	ctx context.Context, volumeName, snapshotName string,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("snapshot_delete", &err)()

	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)

	results, unlocker, err := db.Lock(ctx, db.Query(
		db.InconsistentReadSnapshot(snapshotID),
	))
	unlocker()
	if err != nil {
		return err
	}

	snapshot := results[0].Snapshot.Read

	if snapshot == nil {
		return errors.NotFoundError("snapshot %s not found on volume %s", snapshotName, volumeName)
	}

	txn := &storage.VolumeTransaction{
		// delete txn cleanup logic only uses snapshot config. It is ok to not set the volume config here.
		SnapshotConfig: snapshot.Config,
		Op:             storage.DeleteSnapshot,
	}

	// Acquire the txn lock before acquiring any cache locks to avoid deadlocks
	o.txnMutex.Lock(txn.Name())
	defer o.txnMutex.Unlock(txn.Name())

	if err = o.AddVolumeTransaction(ctx, txn); err != nil {
		return err
	}

	defer func() {
		err = o.deleteSnapshotCleanup(ctx, err, txn)
	}()

	// Delete the snapshot
	err = o.deleteSnapshot(ctx, volumeName, snapshotName)
	if err != nil {
		return err
	}

	// If the volume state is "deleting" and if the volume does not have any snapshots, delete the volume.
	results, unlocker, err = db.Lock(ctx, db.Query(
		db.InconsistentReadVolume(volumeName),
		db.ListSnapshotsForVolume(volumeName),
	))
	unlocker()
	if err != nil {
		return err
	}

	volume := results[0].Volume.Read
	allSnapshotsForVolume := results[0].Snapshots

	// If this snapshot volume pinned its source volume in Deleting state, clean up the source volume
	// if it isn't still pinned by something else (subordinate volumes).
	if volume != nil && volume.State.IsDeleting() {
		if len(allSnapshotsForVolume) == 0 {
			// TODO: (vivaker) Verify this works and add unit test once Clinton's Volume CRUD PR is merged.
			return o.deleteVolume(ctx, volume.Config.Name)
		}
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) AddStorageClass(
	ctx context.Context, scConfig *storageclass.Config,
) (scExternal *storageclass.External, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("storageclass_add", &err)()

	sc := storageclass.New(scConfig)
	scName := scConfig.Name

	// Lock what we need
	results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertStorageClass(scName)))
	if err != nil {
		unlocker()
		return nil, err
	}
	if results[0].StorageClass.Read != nil {
		unlocker()
		return nil, fmt.Errorf("storage class %s already exists", sc.GetName())
	}

	// Persist storage class
	err = o.storeClient.AddStorageClass(ctx, sc)
	if err != nil {
		unlocker()
		return nil, err
	}

	// Add storage class to cache
	results[0].StorageClass.Upsert(sc.SmartCopy().(*storageclass.StorageClass))
	unlocker()

	// Check if Autogrow policy exists
	autogrowPolicy := sc.GetAutogrowPolicy()
	var addErr error
	if autogrowPolicy != "" {
		Logc(ctx).WithFields(LogFields{
			"storageClass":   scName,
			"AutogrowPolicy": autogrowPolicy,
		}).Info("StorageClass has Autogrow policy; updating affected volumes.")

		addErr = o.upsertStorageClassAutogrowPolicyInternal(ctx, scName)
	}

	// Update storage class to pool map
	o.RebuildStorageClassPoolMap(ctx)

	return sc.ConstructExternalWithPoolMap(ctx,
		o.GetStorageClassPoolMap().BackendPoolMapForStorageClass(ctx, scName)), addErr
}

func (o *ConcurrentTridentOrchestrator) UpdateStorageClass(
	ctx context.Context, scConfig *storageclass.Config,
) (scExternal *storageclass.External, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("storageclass_update", &err)()

	newSC := storageclass.New(scConfig)
	scName := newSC.GetName()

	// Lock what we need
	results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertStorageClass(scName)))
	if err != nil {
		unlocker()
		return nil, err
	}
	oldSC := results[0].StorageClass.Read

	if oldSC == nil {
		unlocker()
		return nil, errors.NotFoundError("storage class %s not found", scName)
	}

	if err = o.storeClient.UpdateStorageClass(ctx, newSC); err != nil {
		unlocker()
		return nil, err
	}

	// Add storage class to cache
	results[0].StorageClass.Upsert(newSC.SmartCopy().(*storageclass.StorageClass))

	unlocker()

	// Check if Autogrow policy changed
	oldAGPolicy := oldSC.GetAutogrowPolicy()
	newAGPolicy := newSC.GetAutogrowPolicy()

	var updateErr error
	if oldAGPolicy != newAGPolicy {
		Logc(ctx).WithFields(LogFields{
			"storageClass":      scName,
			"oldAutogrowPolicy": oldAGPolicy,
			"newAutogrowPolicy": newAGPolicy,
		}).Info("StorageClass Autogrow policy changed; updating affected volumes.")

		updateErr = o.upsertStorageClassAutogrowPolicyInternal(ctx, scName)

	}

	// Update storage class to pool map
	o.RebuildStorageClassPoolMap(ctx)

	return newSC.ConstructExternalWithPoolMap(ctx,
		o.GetStorageClassPoolMap().BackendPoolMapForStorageClass(ctx, scName)), updateErr
}

func (o *ConcurrentTridentOrchestrator) DeleteStorageClass(ctx context.Context, scName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("storageclass_delete", &err)()

	// Prepare cache for storage class delete
	results, unlocker, err := db.Lock(ctx, db.Query(db.DeleteStorageClass(scName)))
	if err != nil {
		unlocker()
		return err
	}
	sc := results[0].StorageClass.Read
	if sc == nil {
		unlocker()
		return errors.NotFoundError("storage class %v was not found", scName)
	}

	// Get SC Autogrow policy before deletion
	// This is needed to know which volumes might be affected
	scAutogrowPolicy := sc.GetAutogrowPolicy()

	// Note that we don't need a transaction here.  If this crashes prior
	// to successful deletion, the storage class will be reloaded upon reboot
	// automatically, which is consistent with the method never having returned
	// successfully.
	err = o.storeClient.DeleteStorageClass(ctx, sc)
	if err != nil {
		unlocker()
		return err
	}

	// Delete storage class from cache
	results[0].StorageClass.Delete()

	unlocker()

	// Re-resolve effective Autogrow policy for affected volumes
	// This handles the case where SC had an Autogrow policy and volumes were inheriting from it
	// After SC deletion, volumes should fall back to next priority (TBC in future, or empty)
	if scAutogrowPolicy != "" {
		Logc(ctx).WithFields(LogFields{
			"storageClass":   scName,
			"autogrowPolicy": scAutogrowPolicy,
		}).Debug("Re-resolving Autogrow policy for volumes after StorageClass deletion.")

		// List volumes using this deleted SC without PVC override (optimized query)
		volResults, volUnlocker, volLockErr := db.Lock(ctx,
			db.Query(db.ListVolumesForStorageClassWithoutAutogrowOverride(scName)),
			db.Query(db.ListSubordinateVolumesForStorageClassWithoutAutogrowOverride(scName)))
		if volLockErr == nil {
			volumes := volResults[0].Volumes
			subordinateVolumes := volResults[1].SubordinateVolumes
			// Combine both normal and subordinate volumes
			allVolumes := append(volumes, subordinateVolumes...)
			volUnlocker()

			// Find all volumes using this deleted SC
			for _, volume := range allVolumes {
				isSubordinate := volume.IsSubordinate()

				// Only update volumes that:
				//  1. Were using this SC
				//  2. Have NO volume-level autogrow policy (were inheriting from SC)

				// Lock this specific volume for update
				var volUpdateResults []db.Result
				var volUpdateUnlocker func()
				var volUpdateLockErr error

				if isSubordinate {
					volUpdateResults, volUpdateUnlocker, volUpdateLockErr = db.Lock(ctx,
						db.Query(db.UpsertSubordinateVolume(volume.Config.Name, volume.Config.ShareSourceVolume)))
				} else {
					volUpdateResults, volUpdateUnlocker, volUpdateLockErr = db.Lock(ctx,
						db.Query(db.UpsertVolume(volume.Config.Name, "")))
				}
				if volUpdateLockErr != nil {
					Logc(ctx).WithFields(LogFields{
						"volume":       volume.Config.Name,
						"storageClass": scName,
					}).WithError(volUpdateLockErr).Warn(
						"Failed to lock volume for re-evaluation after SC deletion.")
					volUpdateUnlocker()
					continue
				}

				// Extract volume based on type
				var vol *storage.Volume
				var upserter func(*storage.Volume)

				if isSubordinate {
					vol = volUpdateResults[0].SubordinateVolume.Read
					upserter = volUpdateResults[0].SubordinateVolume.Upsert
				} else {
					vol = volUpdateResults[0].Volume.Read
					upserter = volUpdateResults[0].Volume.Upsert
				}

				if vol == nil {
					volUpdateUnlocker()
					continue
				}

				oldEffectiveAGPolicy := vol.EffectiveAGPolicy

				// Re-resolve effective Autogrow policy (SC now gone, will fall to TBC or empty)
				// Current: Returns "" (no TBC support yet)
				// Future: Will check TBC and return TBC policy if configured
				newEffectiveAGPolicy, _ := o.resolveEffectiveAutogrowPolicy(ctx, vol.Config)

				// Update effective policy
				vol.EffectiveAGPolicy = newEffectiveAGPolicy

				// Log if Autogrow policy changed
				if oldEffectiveAGPolicy.PolicyName != newEffectiveAGPolicy.PolicyName {
					Logc(ctx).WithFields(LogFields{
						"volume":                vol.Config.Name,
						"oldAutogrowPolicyName": oldEffectiveAGPolicy.PolicyName,
						"newAutogrowPolicyName": newEffectiveAGPolicy.PolicyName,
					}).Debug("Re-resolved effective Autogrow policy after StorageClass deletion.")
				}

				upserter(vol)
				volUpdateUnlocker()
			}
		} else {
			volUnlocker()
			Logc(ctx).WithField("storageClass", scName).WithError(volLockErr).
				Warn("Failed to list volumes for re-evaluation after SC deletion.")
		}

		Logc(ctx).WithFields(LogFields{
			"storageClass": scName,
		}).Info("Re-resolved Autogrow policy for volumes after StorageClass deletion.")
	}

	// Update storage class to pool map
	o.RebuildStorageClassPoolMap(ctx)

	return nil
}

func (o *ConcurrentTridentOrchestrator) GetStorageClass(
	ctx context.Context, scName string,
) (scExternal *storageclass.External, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("storageclass_get", &err)()

	// Get storage class from cache
	results, unlocker, err := db.Lock(ctx, db.Query(db.InconsistentReadStorageClass(scName)))
	defer unlocker()
	if err != nil {
		return nil, err
	}
	sc := results[0].StorageClass.Read

	if sc == nil {
		return nil, errors.NotFoundError("storage class %v was not found", scName)
	}

	return sc.ConstructExternalWithPoolMap(ctx,
		o.GetStorageClassPoolMap().BackendPoolMapForStorageClass(ctx, scName)), nil
}

func (o *ConcurrentTridentOrchestrator) ListStorageClasses(ctx context.Context) (
	scExternals []*storageclass.External, err error,
) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("storageclass_list", &err)()

	// Get storage classes from cache
	results, unlocker, err := db.Lock(ctx, db.Query(db.ListStorageClasses()))
	defer unlocker()
	if err != nil {
		return nil, err
	}
	storageClasses := results[0].StorageClasses

	poolMap := o.GetStorageClassPoolMap()

	scExternals = make([]*storageclass.External, 0, len(storageClasses))
	for _, sc := range storageClasses {
		scExternals = append(scExternals,
			sc.ConstructExternalWithPoolMap(ctx, poolMap.BackendPoolMapForStorageClass(ctx, sc.GetName())))
	}
	return scExternals, nil
}

func (o *ConcurrentTridentOrchestrator) AddNode(
	ctx context.Context, node *models.Node, nodeEventCallback NodeEventCallback,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("node_add", &err)()

	// Check if node services have changed
	if node.HostInfo != nil && len(node.HostInfo.Services) > 0 {
		nodeEventCallback(controllerhelpers.EventTypeNormal, "TridentServiceDiscovery",
			fmt.Sprintf("%s detected on host.", node.HostInfo.Services))
	}

	results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertNode(node.Name)))
	if err != nil {
		unlocker()
		return err
	}

	// Copy some values from existing node
	if results[0].Node.Read != nil {
		node.PublicationState = results[0].Node.Read.PublicationState
	}

	// Setting log level, log workflows and log layers on the node same as to what is set on the controller.
	logLevel := GetDefaultLogLevel()
	node.LogLevel = logLevel

	logWorkflows := GetSelectedWorkFlows()
	node.LogWorkflows = logWorkflows

	logLayers := GetSelectedLogLayers()
	node.LogLayers = logLayers

	Logc(ctx).WithFields(LogFields{
		"node":  node.Name,
		"state": node.PublicationState,
	}).Debug("Adding node to persistence layer.")
	if err = o.storeClient.AddOrUpdateNode(ctx, node); err != nil {
		unlocker()
		return
	}

	results[0].Node.Upsert(node)
	unlocker()

	o.updateLastNodeRegistrationTime()
	if invalidateErr := o.invalidateAllBackendNodeAccess(ctx); invalidateErr != nil {
		Logc(ctx).WithError(invalidateErr).Error("Could not invalidate backend node access.")
	}
	return
}

// UpdateNode updates the publication state of a node. It does not create a new node if it does not exist.
func (o *ConcurrentTridentOrchestrator) UpdateNode(
	ctx context.Context, nodeName string, flags *models.NodePublicationStateFlags,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)
	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("node_update", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertNode(nodeName)))
	defer unlocker()
	if err != nil {
		return err
	}
	node := results[0].Node.Read
	if node == nil {
		return errors.NotFoundError("node %v was not found", nodeName)
	}

	// Update node publication state based on state flags
	if flags != nil {
		logFields := LogFields{
			"node":  nodeName,
			"flags": flags,
		}
		Logc(ctx).WithFields(logFields).WithField("state", node.PublicationState).Trace("Pre-state transition.")

		switch node.PublicationState {
		case models.NodeClean:
			if flags.IsNodeDirty() {
				node.PublicationState = models.NodeDirty
				Logc(ctx).WithFields(logFields).Info("Transitioning Node state from Clean to Dirty.")
			}
		case models.NodeCleanable:
			if flags.IsNodeDirty() {
				node.PublicationState = models.NodeDirty
				Logc(ctx).WithFields(logFields).Info("Transitioning Node state from Cleanable to Dirty.")
			} else if flags.IsNodeCleaned() {
				node.PublicationState = models.NodeClean
				Logc(ctx).WithFields(logFields).Info("Transitioning Node state from Cleanable to Clean.")
			}
		case models.NodeDirty:
			if flags.IsNodeCleanable() {
				node.PublicationState = models.NodeCleanable
				Logc(ctx).WithFields(logFields).Info("Transitioning Node state from Dirty to Cleanable.")
			}
		}
	}

	// Update the node in the persistent store if anything changed
	Logc(ctx).WithFields(LogFields{
		"node":  nodeName,
		"state": node.PublicationState,
	}).Debug("Updating node in persistence layer.")
	if err = o.storeClient.AddOrUpdateNode(ctx, node); err != nil {
		return
	}

	results[0].Node.Upsert(node)
	return
}

func (o *ConcurrentTridentOrchestrator) GetNode(
	ctx context.Context, nodeName string,
) (node *models.NodeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	defer recordTiming("node_get", &err)()

	results, unlocker, e := db.Lock(ctx, db.Query(db.InconsistentReadNode(nodeName)))
	defer unlocker()
	if e != nil {
		err = e
		return
	}
	n := results[0].Node.Read
	if n == nil {
		Logc(ctx).WithField("node", nodeName).Info(
			"There may exist a networking or DNS issue preventing this node from registering with the" +
				" Trident controller")
		err = errors.NotFoundError("node %v was not found", nodeName)
		return
	}

	node = n.ConstructExternal()
	return
}

func (o *ConcurrentTridentOrchestrator) ListNodes(
	ctx context.Context,
) (nodes []*models.NodeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)
	defer recordTiming("node_list", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.ListNodes()))
	defer unlocker()
	if err != nil {
		return nil, err
	}
	nodes = make([]*models.NodeExternal, 0, len(results[0].Nodes))
	for _, node := range results[0].Nodes {
		nodes = append(nodes, node.ConstructExternal())
	}

	return
}

func (o *ConcurrentTridentOrchestrator) DeleteNode(ctx context.Context, nodeName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("node_delete", &err)()

	results, unlocker, err := db.Lock(
		ctx,
		db.Query(db.ListVolumePublicationsForNode(nodeName), db.DeleteNode(nodeName)),
		db.Query(db.UpsertNode(nodeName)),
	)
	if err != nil {
		unlocker()
		return err
	}
	volumePublications := results[0].VolumePublications
	node := results[0].Node.Read
	deleteNode := results[0].Node.Delete
	upsertNode := results[1].Node.Upsert

	if node == nil {
		return errors.NotFoundError("node %v was not found", nodeName)
	}

	// If there are still volumes published to this node, this is a sudden node removal. Preserve the node CR and mark
	// it as deleted so we can handle the eventual unpublish calls for the affected volumes.
	if len(volumePublications) > 0 {
		Logc(ctx).WithField("node", nodeName).Debug(
			"There are still volumes published to this node, marking node CR as deleted.")
		node.Deleted = true
		if err = o.storeClient.AddOrUpdateNode(ctx, node); err != nil {
			unlocker()
			return
		}
		upsertNode(node)
		unlocker()
		return
	}

	// No publications for this node, so we can delete it.
	if err := o.storeClient.DeleteNode(ctx, node); err != nil {
		unlocker()
		return fmt.Errorf("failed to delete node %s in store: %w", nodeName, err)
	}
	deleteNode()
	unlocker()

	if invalidateErr := o.invalidateAllBackendNodeAccess(ctx); invalidateErr != nil {
		Logc(ctx).WithError(invalidateErr).Error("Could not invalidate backend node access.")
	}
	return o.reconcileNodeAccessOnAllBackends(ctx)
}

func (o *ConcurrentTridentOrchestrator) PeriodicallyReconcileNodeAccessOnBackends() {
	o.stopNodeAccessLoop = make(chan bool)
	ctx := GenerateRequestContext(nil, "", ContextSourcePeriodic, WorkflowCoreNodeReconcile, LogLayerCore)

	Logc(ctx).Info("Starting periodic node access reconciliation service.")
	defer Logc(ctx).Info("Stopping periodic node access reconciliation service.")

	// Every period seconds after the last run
	ticker := time.NewTicker(o.nodeAccessReconcilePeriod)
	defer ticker.Stop()

	for {
		select {
		case <-o.stopNodeAccessLoop:
			// Exit on shutdown signal
			return

		case <-ticker.C:
			Logc(ctx).Trace("Periodic node access reconciliation loop beginning.")
			// Do nothing if it has not been at least cooldown seconds after the last node registration to prevent thundering herd
			if time.Now().After(o.getLastNodeRegistrationTime().Add(NodeRegistrationCooldownPeriod)) {
				if err := o.reconcileNodeAccessOnAllBackends(ctx); err != nil {
					// If there's a problem log an error and keep going
					Logc(ctx).WithError(err).Error("Problem encountered updating node access rules for backends.")
				}
			} else {
				Logc(ctx).Trace(
					"Time is too soon since last node registration, delaying node access rule reconciliation.")
			}
		}
	}
}

func (o *ConcurrentTridentOrchestrator) PeriodicallyReconcileBackendState(pollInterval time.Duration) {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourcePeriodic, WorkflowCoreBackendReconcile,
		LogLayerCore)

	// Provision to disable reconciling backend state, just in case
	if pollInterval <= 0 {
		Logc(ctx).Debug("Periodic reconciliation of backends is disabled.")
		return
	}

	Logc(ctx).WithField("pollInterval", pollInterval).Info("Starting periodic backend state reconciliation service.")
	defer Logc(ctx).WithField("pollInterval", pollInterval).Info("Stopping periodic backend state reconciliation service.")

	o.stopReconcileBackendLoop = make(chan bool)
	reconcileBackendTimer := time.NewTimer(pollInterval)
	defer func(t *time.Timer) {
		if !t.Stop() {
			<-t.C
		}
	}(reconcileBackendTimer)

	for {
		select {
		case <-o.stopReconcileBackendLoop:
			// Exit on shutdown signal.
			return

		case <-reconcileBackendTimer.C:
			Logc(ctx).Debug("Periodic backend state reconciliation loop beginning.")

			if o.bootstrapError != nil {
				Logc(ctx).WithError(o.bootstrapError).Debug("Core not ready yet, skipping backend reconcile.")
				reconcileBackendTimer.Reset(pollInterval)
				continue
			}

			results, unlocker, dbErr := db.Lock(ctx, db.Query(db.ListBackends()))
			unlocker()
			if dbErr != nil {
				// If we can't list backends from cache, log an error and try again at the next interval.
				Logc(ctx).WithError(dbErr).Error("Unable to list backends from cache.")
				reconcileBackendTimer.Reset(pollInterval)
				continue
			}
			backends := results[0].Backends

			var wg sync.WaitGroup
			wg.Add(len(backends))

			for _, b := range backends {
				backend := b

				go func(backend storage.Backend) {
					defer wg.Done()
					if err := o.reconcileBackendState(ctx, backend.BackendUUID()); err != nil {
						// If there is a problem, log an error and keep going.
						Logc(ctx).WithField("backend", backend.Name()).WithError(err).Error(
							"Unable to reconcile state for backend.")
					}
				}(backend)
			}

			wg.Wait()

			// Reset the timer so that next poll starts after pollInterval.
			reconcileBackendTimer.Reset(pollInterval)
		}
	}
}

func (o *ConcurrentTridentOrchestrator) reconcileBackendState(ctx context.Context, backendUUID string) error {
	Logc(ctx).WithField("backend", backendUUID).Debug(">>>> reconcileBackendState")
	defer Logc(ctx).WithField("backend", backendUUID).Debug("<<<< reconcileBackendState")

	results, unlocker, dbErr := db.Lock(ctx, db.Query(db.ReadBackend(backendUUID)))
	if dbErr != nil {
		unlocker()
		return dbErr
	}
	backend := results[0].Backend.Read

	if backend == nil {
		unlocker()
		return errors.NotFoundError("backend %s not found", backendUUID)
	}

	if !backend.CanGetState() {
		// This backend does not support polling backend for state.
		unlocker()
		return nil
	}

	reason, changeMap := backend.GetBackendState(ctx)
	unlocker()

	if changeMap != nil && !changeMap.IsEmpty() {

		// Acquire write lock on backend
		results, unlocker, dbErr = db.Lock(ctx, db.Query(db.UpsertBackend(backendUUID, "", "")))
		defer unlocker()
		if dbErr != nil {
			return dbErr
		}
		backend = results[0].Backend.Read
		upserter := results[0].Backend.Upsert

		if backend == nil {
			return errors.NotFoundError("backend '%s' not found", backendUUID)
		}

		// Ensure there is work to do now that we hold the write lock
		reason, changeMap = backend.GetBackendState(ctx)
		if changeMap == nil || changeMap.IsEmpty() {
			// In the unlikely event there is now no issue, just return
			return nil
		}

		backend.UpdateBackendState(ctx, reason)
		upserter(backend)

		logFields := LogFields{
			"backend": backend.Name(),
			"reason":  reason,
		}

		if changeMap.Contains(storage.BackendStateReasonChange) {
			// Update CR.
			Logc(ctx).WithFields(logFields).Debugf("Backend state reason change detected.")
			if err := o.storeClient.UpdateBackend(ctx, backend); err != nil {
				return err
			}
		}

		// If reason is non-empty, the backend is offline, so it doesn't make sense to call updateBackend()
		// and we should return early. Additionally, calling updateBackend() might fail for various reasons, such as
		// - A dataLIF being down
		// - The aggregate specified in the backend config's aggregate field being different
		// - And other similar issues
		if reason != "" {
			return nil
		}

		var logMessage string

		// Determine the log message based on the changes detected
		switch {
		case changeMap.Contains(storage.BackendStatePoolsChange) && changeMap.Contains(storage.BackendStateAPIVersionChange):
			logMessage = "Change in physical pools and API version detected for the backend."
		case changeMap.Contains(storage.BackendStatePoolsChange):
			logMessage = "Change in physical pools detected for the backend."
		case changeMap.Contains(storage.BackendStateAPIVersionChange):
			logMessage = "Change in API version detected for the backend."
		}

		if logMessage != "" {
			Logc(ctx).WithFields(logFields).Debug(logMessage)

			// Getting the marshaled driver's config.
			configBytes, configErr := backend.MarshalDriverConfig()
			if configErr != nil {
				return configErr
			}

			_, err := o.upsertBackend(ctx, string(configBytes), results[0], backend.ConfigRef())
			if err != nil {
				Logc(ctx).WithFields(LogFields{
					"err":         err.Error(),
					"backendName": backend.Name(),
					"configRef":   backend.ConfigRef(),
				}).WithError(err).Error("UpdateBackend failed.")
				return err
			}
		}
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) ReconcileVolumePublications(
	ctx context.Context, attachedLegacyVolumes []*models.VolumePublicationExternal,
) (reconcileErr error) {
	Logc(ctx).Debug(">>>>>> ReconcileVolumePublications")
	defer Logc(ctx).Debug("<<<<<< ReconcileVolumePublications")
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("reconcile_legacy_vol_pubs", &reconcileErr)()

	Logc(ctx).Error("Volume publication reconciliation is not supported in the concurrent core. " +
		"Disable concurrency to upgrade from Trident 21.10 or earlier.")
	return nil
}

// GetVolumePublication returns the volume publication for a given volume/node pair
func (o *ConcurrentTridentOrchestrator) GetVolumePublication(
	ctx context.Context, volumeName, nodeName string,
) (publication *models.VolumePublication, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_get", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.InconsistentReadVolumePublication(volumeName, nodeName)))
	defer unlocker()
	if err != nil {
		return nil, err
	}

	publication = results[0].VolumePublication.Read
	if publication == nil {
		err = errors.NotFoundError("volume publication %v was not found",
			models.GenerateVolumePublishName(volumeName, nodeName))
	}
	return
}

// ListVolumePublications returns a list of all volume publications.
func (o *ConcurrentTridentOrchestrator) ListVolumePublications(
	ctx context.Context,
) (publications []*models.VolumePublicationExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	Logc(ctx).Debug(">>>>>> ListVolumePublications")
	defer Logc(ctx).Debug("<<<<<< ListVolumePublications")

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_list", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.ListVolumePublications()))
	defer unlocker()
	if err != nil {
		return nil, err
	}
	publications = make([]*models.VolumePublicationExternal, 0, len(results[0].VolumePublications))
	for _, pub := range results[0].VolumePublications {
		publications = append(publications, pub.ConstructExternal())
	}
	return
}

// SyncVolumePublications asynchronously persists VP fields to the store with rate limiting.
// The VPs are already updated in cache; this method only persists them to the store.
//
// Used by:
//   - Bootstrap: Syncing/migrating all VPs (labels, AGP, legacy fields)
//   - UpdateVolumeAutogrowPolicy: Propagating AGP changes to RWX/RO volume's VPs
func (o *ConcurrentTridentOrchestrator) SyncVolumePublications(
	ctx context.Context, vpsToBeSynced []*models.VolumePublication,
) {
	Logc(ctx).WithField("vpCount", len(vpsToBeSynced)).Debug(">>>>>> SyncVolumePublications")
	defer Logc(ctx).Debug("<<<<<< SyncVolumePublications")

	Logc(ctx).WithField("vpCount", len(vpsToBeSynced)).Info("Starting volume publication sync")

	if len(vpsToBeSynced) == 0 {
		Logc(ctx).Debug("No volume publications to sync")
		return
	}

	// Configure exponential backoff for rate limiter waits
	rateLimiterBackoff := backoff.NewExponentialBackOff()
	rateLimiterBackoff.InitialInterval = 500 * time.Millisecond
	rateLimiterBackoff.Multiplier = 1.5
	rateLimiterBackoff.MaxInterval = 2 * time.Second // Cap at 2s for rate limiting
	rateLimiterBackoff.RandomizationFactor = 0.2
	rateLimiterBackoff.MaxElapsedTime = 0 // Retry indefinitely

	// Configure exponential backoff for store update retries
	vpStoreBackoff := backoff.NewExponentialBackOff()
	vpStoreBackoff.InitialInterval = 1 * time.Second
	vpStoreBackoff.Multiplier = 2.0
	vpStoreBackoff.MaxInterval = 30 * time.Second
	vpStoreBackoff.RandomizationFactor = 0.2
	vpStoreBackoff.MaxElapsedTime = 0 // Retry indefinitely

	for i := range vpsToBeSynced {
		// Reset rate limiter backoff for this VP
		rateLimiterBackoff.Reset()

		// Wait for rate limiter token WITHOUT holding lock to avoid blocking other operations
		for !vpSyncRateLimiter.Allow() {
			waitDuration := rateLimiterBackoff.NextBackOff()
			Logc(ctx).WithFields(LogFields{
				"volumeName":  vpsToBeSynced[i].VolumeName,
				"nodeName":    vpsToBeSynced[i].NodeName,
				"backoffWait": waitDuration.String(),
			}).Debug("Rate limiter throttling VP sync, backing off before retry")

			// Sleep with exponential backoff to give rate limiter time to refill
			time.Sleep(waitDuration)
		}

		// Reset store update backoff for this VP
		vpStoreBackoff.Reset()

		// Log progress
		vpsRemaining := len(vpsToBeSynced) - i
		Logc(ctx).WithFields(LogFields{
			"vpsRemaining": vpsRemaining,
			"vpsTotal":     len(vpsToBeSynced),
		}).Debug("Syncing volume publications")

		// Retry logic for persisting VP to store with exponential backoff
		// The lock is acquired and released for each attempt, allowing other operations to proceed
		vpStoreUpdateAttempt := func() error {
			// Try to persist this VP
			err := o.syncVolumePublication(ctx, vpsToBeSynced[i])
			if err != nil {
				Logc(ctx).WithFields(LogFields{
					"volumeName": vpsToBeSynced[i].VolumeName,
					"nodeName":   vpsToBeSynced[i].NodeName,
				}).WithError(err).Warn("Failed to sync VP fields to store, will retry")
				return err
			}
			return nil
		}

		// Persist the VP to the store with exponential backoff retry
		// The backoff sleep happens WITHOUT holding the lock
		// Will retry indefinitely until success or VP is deleted from cache
		if err := backoff.RetryNotify(vpStoreUpdateAttempt, vpStoreBackoff, nil); err != nil {
			Logc(ctx).WithFields(LogFields{
				"volumeName": vpsToBeSynced[i].VolumeName,
				"nodeName":   vpsToBeSynced[i].NodeName,
			}).WithError(err).Error("Failed to sync VP fields to store (unexpected error)")
		}

		// Yield CPU to give other goroutines priority (this is a low-priority background operation)
		runtime.Gosched()
	}

	Logc(ctx).WithField("vpCount", len(vpsToBeSynced)).Info("Completed propagating fields to all volume publications")
}

// syncVolumePublication persists a single VP from cache to store.
//
// This function reads the latest VP state from cache (source of truth) and persists it to the store.
// If the VP was deleted from cache between queueing and persistence, the update is safely skipped.
//
// IMPORTANT: This does NOT acquire the global mutex like orchestrator_core, instead it uses
// concurrent cache locking pattern which is the standard for concurrent_core.
func (o *ConcurrentTridentOrchestrator) syncVolumePublication(
	ctx context.Context, vp *models.VolumePublication,
) error {
	Logc(ctx).WithFields(LogFields{
		"volumeName": vp.VolumeName,
		"nodeName":   vp.NodeName,
	}).Debug(">>>>>> syncVolumePublication")
	defer Logc(ctx).Debug("<<<<<< syncVolumePublication")

	// Fetch the latest VP from cache using concurrent core's locking pattern
	results, unlocker, err := db.Lock(ctx, db.Query(db.UpsertVolumePublication(vp.VolumeName, vp.NodeName)))
	defer unlocker()

	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeName": vp.VolumeName,
			"nodeName":   vp.NodeName,
		}).WithError(err).Debug("Failed to lock VP for sync, skipping update")
		return err
	}

	latestVP := results[0].VolumePublication.Read
	if latestVP == nil {
		// VP was deleted while we were waiting in rate limiter - skip update
		Logc(ctx).WithFields(LogFields{
			"volumeName": vp.VolumeName,
			"nodeName":   vp.NodeName,
		}).Debug("VP not found in cache, skipping update (may have been deleted)")
		return err
	}

	// Persist the vp to the store
	if err := o.storeClient.UpdateVolumePublication(ctx, latestVP); err != nil {
		Logc(ctx).WithFields(LogFields{
			"publication": latestVP.Name,
			"volumeName":  latestVP.VolumeName,
			"nodeName":    latestVP.NodeName,
		}).WithError(err).Error("Failed to sync VP fields to store")
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"volumeName": latestVP.VolumeName,
		"nodeName":   latestVP.NodeName,
		"labels":     latestVP.Labels,
		"agPolicy":   latestVP.AutogrowPolicy,
	}).Debug("Successfully synced VP fields to store")

	return nil
}

// ListVolumePublicationsForVolume returns a list of all volume publications for a given volume.
func (o *ConcurrentTridentOrchestrator) ListVolumePublicationsForVolume(
	ctx context.Context, volumeName string,
) (publications []*models.VolumePublicationExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	fields := LogFields{"volumeName": volumeName}
	Logc(ctx).WithFields(fields).Trace(">>>> ListVolumePublicationsForVolume")
	defer Logc(ctx).Trace("<<<< ListVolumePublicationsForVolume")

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_list_for_vol", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.ListVolumePublicationsForVolume(volumeName)))
	defer unlocker()
	if err != nil {
		return nil, err
	}
	publications = make([]*models.VolumePublicationExternal, 0, len(results[0].VolumePublications))
	for _, pub := range results[0].VolumePublications {
		publications = append(publications, pub.ConstructExternal())
	}
	return
}

// ListVolumePublicationsForNode returns a list of all volume publications for a given node.
func (o *ConcurrentTridentOrchestrator) ListVolumePublicationsForNode(
	ctx context.Context, nodeName string,
) (publications []*models.VolumePublicationExternal, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	fields := LogFields{"nodeName": nodeName}
	Logc(ctx).WithFields(fields).Debug(">>>>>> ListVolumePublicationsForNode")
	defer Logc(ctx).Debug("<<<<<< ListVolumePublicationsForNode")

	defer recordTiming("vol_pub_list_for_node", &err)()

	results, unlocker, err := db.Lock(ctx, db.Query(db.ListVolumePublicationsForNode(nodeName)))
	defer unlocker()
	if err != nil {
		return nil, err
	}
	publications = make([]*models.VolumePublicationExternal, 0, len(results[0].VolumePublications))
	for _, pub := range results[0].VolumePublications {
		publications = append(publications, pub.ConstructExternal())
	}
	return
}

func (o *ConcurrentTridentOrchestrator) handleFailedTransaction(ctx context.Context, v *storage.VolumeTransaction) error {
	// Remove any deadlines or timeouts from the context so we can clean up
	ctx = context.WithoutCancel(GenerateRequestContextForLayer(ctx, LogLayerCore))

	switch v.Op {
	case storage.AddVolume, storage.DeleteVolume,
		storage.ImportVolume, storage.ResizeVolume:
		Logc(ctx).WithFields(LogFields{
			"volume":       v.Config.Name,
			"size":         v.Config.Size,
			"storageClass": v.Config.StorageClass,
			"op":           v.Op,
		}).Info("Processed volume transaction log.")
	case storage.AddSnapshot, storage.DeleteSnapshot:
		Logc(ctx).WithFields(LogFields{
			"volume":   v.SnapshotConfig.VolumeName,
			"snapshot": v.SnapshotConfig.Name,
			"op":       v.Op,
		}).Info("Processed snapshot transaction log.")
	case storage.VolumeCreating:
		Logc(ctx).WithFields(LogFields{
			"volume":      v.VolumeCreatingConfig.Name,
			"backendUUID": v.VolumeCreatingConfig.BackendUUID,
			"op":          v.Op,
		}).Info("Processed volume creating transaction log.")
	}

	var (
		results  []db.Result
		unlocker func()
		dbErr    error
	)

	switch v.Op {
	case storage.AddVolume:

		results, unlocker, dbErr = db.Lock(ctx, db.Query(db.InconsistentReadVolume(v.Config.Name)))
		unlocker()
		if dbErr != nil {
			return fmt.Errorf("unable to get volume %s from cache: %w", v.Config.Name, dbErr)
		}
		volume := results[0].Volume.Read

		// Regardless of whether the transaction succeeded or not, we need
		// to roll it back.  There are three possible states:
		// 1) Volume transaction created only
		// 2) Volume created on backend
		// 3) Volume created in the store.
		if volume != nil {
			// If the volume was added to the store, we will have loaded the
			// volume into memory, and we can just delete it normally.
			// Handles case 3)

			// Don't call locks for top-level methods.
			if err := o.deleteVolume(ctx, v.Config.Name); err != nil {
				return fmt.Errorf("unable to clean up volume %s: %w", v.Config.Name, err)
			}
		} else {
			// If the volume wasn't added into the store, we attempt to delete
			// it at each backend, since we don't know where it might have
			// landed.  We're guaranteed that the volume name will be
			// unique across backends, thanks to the StoragePrefix field,
			// so this should be idempotent.
			// Handles case 2)

			results, unlocker, dbErr = db.Lock(ctx, db.Query(db.ListBackends()))
			unlocker()
			if dbErr != nil {
				return fmt.Errorf("unable to list backends from cache: %w", dbErr)
			}
			backends := results[0].Backends

			for _, b := range backends {
				// Backend offlining is serialized with volume creation, so we can safely skip offline backends.
				if !b.State().IsOnline() && !b.State().IsDeleting() {
					continue
				}

				// Volume deletion is an idempotent operation, so it's safe to delete an already
				// deleted volume, but we must try all backends since we can't distinguish between
				// a successful deletion and a non-existent volume.  We lock the volume's internal
				// name to ensure that no other workflow is trying to access the volume on the
				// backend storage system.

				results, unlocker, dbErr = db.Lock(ctx,
					db.Query(db.UpsertBackend(b.BackendUUID(), "", "")),
					db.Query(db.UpsertVolumeByInternalName(v.Config.Name, "", v.Config.InternalName, "")),
				)
				if dbErr != nil {
					unlocker()
					Logc(ctx).WithField("backend", b.Name()).WithError(dbErr).Warn("Unable to lock backend for upsert.")
					continue
				}

				backend := results[0].Backend.Read
				backendUpserter := results[0].Backend.Upsert

				if backend == nil {
					unlocker()
					Logc(ctx).WithField("backend", b.Name()).WithError(dbErr).Warn("Unable to find backend for upsert.")
					continue
				}

				if err := backend.RemoveVolume(ctx, v.Config); err != nil {
					unlocker()
					Logc(ctx).WithFields(LogFields{
						"volume":  v.Config.Name,
						"backend": b.Name(),
					}).WithError(err).Error("Error attempting to clean up volume from backend.")
					return fmt.Errorf("error attempting to clean up volume %s from backend %s: %w",
						v.Config.Name, b.Name(), err)
				}

				backendUpserter(backend)
				unlocker()
			}
		}

		// Finally, we need to clean up the volume transaction.  Necessary for all cases.
		if err := o.DeleteVolumeTransaction(ctx, v); err != nil {
			return fmt.Errorf("failed to clean up volume addition transaction: %w", err)
		}

	case storage.DeleteVolume:

		results, unlocker, dbErr = db.Lock(ctx, db.Query(db.InconsistentReadVolume(v.Config.Name)))
		unlocker()
		if dbErr != nil {
			return fmt.Errorf("unable to get volume %s from cache: %w", v.Config.Name, dbErr)
		}
		volume := results[0].Volume.Read

		// Because we remove the volume from persistent store after we remove
		// it from the backend, we need to take any special measures only when
		// the volume is still in the persistent store. In this case, the
		// volume should have been loaded into memory when we bootstrapped.
		if volume != nil {
			if err := o.deleteVolume(ctx, v.Config.Name); err != nil {
				Logc(ctx).WithField("volume", v.Config.Name).WithError(err).Errorf(
					"Unable to finalize deletion of the volume. Repeat deleting the volume using %s.",
					config.OrchestratorClientName)
			}
		} else {
			Logc(ctx).WithField("volume", v.Config.Name).Info("Volume for the delete transaction wasn't found.")
		}

		if err := o.DeleteVolumeTransaction(ctx, v); err != nil {
			return fmt.Errorf("failed to clean up volume deletion transaction: %v", err)
		}

	case storage.AddSnapshot:
		// Regardless of whether the transaction succeeded or not, we need
		// to roll it back.  There are three possible states:
		// 1) Snapshot transaction created only
		// 2) Snapshot created on backend
		// 3) Snapshot created in persistent store

		// do inconsistent read of snapshot
		results, unlocker, dbErr = db.Lock(ctx, db.Query(
			db.InconsistentReadSnapshot(v.SnapshotConfig.ID()),
		))
		unlocker()
		if dbErr != nil {
			return dbErr
		}

		snapshot := results[0].Snapshot.Read
		if snapshot != nil {
			// If the snapshot was added to the store, we will have loaded the
			// snapshot into memory, and we can just delete it normally.
			// Handles case 3)
			if err := o.deleteSnapshot(ctx, v.SnapshotConfig.VolumeName, v.SnapshotConfig.Name); err != nil {
				return fmt.Errorf("unable to clean up snapshot %s: %v", v.SnapshotConfig.Name, err)
			}
		} else {
			// If the snapshot wasn't added into the store, we attempt to delete
			// it at each backend, since we don't know where it might have landed.
			// We're guaranteed that the volume name will be unique across backends,
			// thanks to the StoragePrefix field, so this should be idempotent.
			// Handles case 2)

			results, unlocker, dbErr = db.Lock(ctx, db.Query(db.ListBackends()))
			unlocker()
			if dbErr != nil {
				return dbErr
			}
			backends := results[0].Backends

			for _, backend := range backends {
				// Skip backends that aren't ready to accept a snapshot delete operation
				if !backend.State().IsOnline() && !backend.State().IsDeleting() {
					continue
				}
				// Snapshot deletion is an idempotent operation, so it's safe to
				// delete an already deleted snapshot.
				// If the volume gets deleted before the snapshot, the snapshot deletion returns "NotFoundError".

				// Acquire read lock on the backend
				// Acquire read lock on the volume
				// Acquire write lock on the snapshot

				backendUUID := backend.BackendUUID()

				results, unlocker, err := db.Lock(
					ctx,
					db.Query(db.DeleteSnapshot(v.SnapshotConfig.ID())),
					db.Query(db.ReadBackend(backendUUID)),
				)
				if err != nil {
					unlocker()
					return err
				}
				deleteSnapshotInCache := results[0].Snapshot.Delete
				backend = results[1].Backend.Read

				if backend == nil {
					unlocker()
					return fmt.Errorf("backend %s not found", backendUUID)
				}

				err = backend.DeleteSnapshot(ctx, v.SnapshotConfig, v.Config)
				if err != nil && !errors.IsUnsupportedError(err) && !errors.IsNotFoundError(err) {
					unlocker()
					return fmt.Errorf("error attempting to clean up snapshot %s from backend %s: %v",
						v.SnapshotConfig.Name, backend.Name(), err)
				}

				deleteSnapshotInCache()
				unlocker()
			}
		}
		// Finally, we need to clean up the snapshot transaction.  Necessary for all cases.
		if err := o.DeleteVolumeTransaction(ctx, v); err != nil {
			return fmt.Errorf("failed to clean up snapshot addition transaction: %v", err)
		}

	case storage.DeleteSnapshot:
		// Because we remove the snapshot from persistent store after we remove
		// it from the backend, we need to take any special measures only when
		// the snapshot is still in the persistent store. In this case, the
		// snapshot, volume, and backend should all have been loaded into memory
		// when we bootstrapped and we can retry the snapshot deletion.
		logFields := LogFields{"volume": v.SnapshotConfig.VolumeName, "snapshot": v.SnapshotConfig.Name}

		if err := o.deleteSnapshot(ctx, v.SnapshotConfig.VolumeName, v.SnapshotConfig.Name); err != nil {
			if errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(logFields).Info("Snapshot for the delete transaction wasn't found.")
			} else {
				Logc(ctx).WithFields(logFields).Errorf("Unable to finalize deletion of the snapshot! "+
					"Repeat deleting the snapshot using %s.", config.OrchestratorClientName)
			}
		}

		if err := o.DeleteVolumeTransaction(ctx, v); err != nil {
			return fmt.Errorf("failed to clean up snapshot deletion transaction: %v", err)
		}

	case storage.ResizeVolume:
		// There are a few possible states:
		// 1) We failed to resize the volume on the backend.
		// 2) We successfully resized the volume on the backend.
		//    2a) We couldn't update the volume size in persistent store.
		//    2b) Persistent store was updated, but we couldn't delete the
		//        transaction object.
		var err error

		results, unlocker, dbErr = db.Lock(ctx, db.Query(db.InconsistentReadVolume(v.Config.Name)))
		unlocker()
		if dbErr != nil {
			Logc(ctx).WithFields(LogFields{
				"volume": v.Config.Name,
				"error":  dbErr,
			}).Error("Error getting volume from the cache")
			return dbErr
		}

		volume := results[0].Volume.Read

		if volume != nil {
			err = o.resizeVolume(ctx, volume, v.Config.Size)
			if err != nil {
				Logc(ctx).WithFields(LogFields{
					"volume": v.Config.Name,
					"error":  err,
				}).Error("Unable to resize the volume! Repeat resizing the volume.")
			} else {
				Logc(ctx).WithFields(LogFields{
					"volume":      volume.Config.Name,
					"volume_size": v.Config.Size,
				}).Info("Orchestrator resized the volume on the storage backend.")
			}
		} else {
			Logc(ctx).WithField("volume", v.Config.Name).Info("Volume for the resize transaction wasn't found.")
		}

		if err = o.DeleteVolumeTransaction(ctx, v); err != nil {
			return fmt.Errorf("failed to clean up volume resize transaction: %v", err)
		}

	case storage.ImportVolume:
		/*
			There are a few possible states:
				1) We created a PVC.
				2) We created a PVC and renamed the volume on the backend.
				3) We created a PVC, renamed the volume, and persisted the volume.
				4) We created a PVC, renamed the vol, persisted the vol, and created a PV

			To handle these states:
				1) Do nothing. If the PVC is still needed,
					k8s will trigger us to try and perform the import again; if not,
					it is up to the user to remove the unwanted PVC.
				2) We will rename the volume on the backend back to its original name,
					to allow for future retries to find it; or if the user aborts the import by removing the PVC,
					then the volume is back in its original state.
				3) Same is (2) but now we also remove the volume from Trident's persistent store.
				4) Same as (3).

			If the import failed the PVC and PV should be cleaned up by the K8S frontend code.
			There is a situation where the PVC/PV bind operation may fail after the import operation is complete.
			In this case the end user needs to delete the PVC and PV via kubectl.
		*/

		logFields := LogFields{
			"volume":  v.Config.Name,
			"backend": v.Config.ImportBackendUUID,
		}

		results, unlocker, dbErr = db.Lock(ctx, db.Query(db.DeleteVolume(v.Config.Name)))
		if dbErr != nil {
			unlocker()
			Logc(ctx).WithFields(logFields).WithError(dbErr).Error("Error getting volume from the cache.")
			return dbErr
		}

		volume := results[0].Volume.Read
		deleteVolumeInCache := results[0].Volume.Delete
		if volume != nil {

			if err := o.storeClient.DeleteVolume(ctx, volume); err != nil {
				unlocker()
				return err
			}

			deleteVolumeInCache()
		}
		unlocker()

		if !v.Config.ImportNotManaged {
			// The volume could be renamed (notManaged = false) without being persisted.

			renameErr := func(backendUUID string) error {
				// Acquire the read lock on the backend.
				// Acquire the write lock on the volume.Config.Name before proceeding to renaming volume.
				// This is required to prevent any other requests from importing the same backend volume
				// while we are trying to rename the backend volume back to its original name.

				results, unlocker, dbErr = db.Lock(ctx,
					db.Query(db.ReadBackend(backendUUID)),
					db.Query(db.DeleteVolume(v.Config.Name)),
				)
				defer unlocker()

				if dbErr != nil {
					Logc(ctx).WithFields(logFields).WithError(dbErr).Error("Error getting backend from the cache.")
					return dbErr
				}

				backend := results[0].Backend.Read
				if backend == nil {
					return fmt.Errorf("backend %s not found", backendUUID)
				}

				if renameErr := backend.RenameVolume(ctx, v.Config, v.Config.ImportOriginalName); renameErr != nil {
					return fmt.Errorf("failed to rename volume on backend; %w", renameErr)
				}

				return nil
			}(v.Config.ImportBackendUUID)

			if renameErr != nil {
				Logc(ctx).Warningf("Could not rename volume %s on backend %s: %v",
					v.Config.Name, v.Config.ImportBackendUUID, renameErr)
			}
		}

		// Finally, we need to clean up the volume transaction
		if err := o.DeleteVolumeTransaction(ctx, v); err != nil {
			return fmt.Errorf("failed to clean up volume addition transaction: %v", err)
		}

	case storage.VolumeCreating:
		// The concurrent core doesn't use long-running transactions, so just delete one if found
		if err := o.DeleteVolumeTransaction(ctx, v); err != nil {
			return fmt.Errorf("failed to clean up volume creating transaction: %v", err)
		}
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) AddVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error {
	// Check if a transaction already exists for this volume. This condition
	// can occur if we failed to clean up the transaction object during the
	// last operation on the volume. The check for a preexisting transaction
	// allows recovery from a failure by repeating the operation and without
	// having to restart Trident. It's important to note that repeating a
	// failed transaction may have side-effects on the new transaction (e.g.,
	// repeating a failed volume delete before a volume resize or clone).
	// Therefore, it's important that the implementation can take care of such
	// scenarios. The current implementation allows only one outstanding
	// transaction per volume.
	// If a transaction is found, we failed in cleaning up the transaction
	// earlier and we need to call the bootstrap cleanup code. If this fails,
	// return an error. If no transaction existed or the operation succeeds,
	// log a new transaction in the persistent store and proceed.
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	oldTxn, err := o.storeClient.GetVolumeTransaction(ctx, volTxn)
	if err != nil {
		Logc(ctx).Errorf("Unable to check for a preexisting volume transaction: %v", err)
		return err
	}
	if oldTxn != nil {
		if oldTxn.Op != storage.VolumeCreating {
			err = o.handleFailedTransaction(ctx, oldTxn)
			if err != nil {
				return fmt.Errorf("unable to process the preexisting transaction for volume %s:  %v",
					volTxn.Config.Name, err)
			}

			switch oldTxn.Op {
			case storage.DeleteVolume, storage.DeleteSnapshot:
				return fmt.Errorf(
					"rejecting the %v transaction after successful completion of a preexisting %v transaction",
					volTxn.Op, oldTxn.Op)
			}
		} else {
			return errors.FoundError("volume transaction already exists")
		}
	}

	return o.storeClient.AddVolumeTransaction(ctx, volTxn)
}

func (o *ConcurrentTridentOrchestrator) GetVolumeTransaction(
	ctx context.Context, volTxn *storage.VolumeTransaction,
) (*storage.VolumeTransaction, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)
	return o.storeClient.GetVolumeTransaction(ctx, volTxn)
}

func (o *ConcurrentTridentOrchestrator) DeleteVolumeTransaction(
	ctx context.Context, volTxn *storage.VolumeTransaction,
) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)
	return o.storeClient.DeleteVolumeTransaction(ctx, volTxn)
}

func (o *ConcurrentTridentOrchestrator) EstablishMirror(
	ctx context.Context, backendUUID, volumeName, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
	replicationSchedule string,
) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	var err error

	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("mirror_establish", &err)()

	backend, err := getInconsistentBackendByUUID(backendUUID)
	if err != nil {
		return err
	}

	if !backend.CanMirror() {
		return fmt.Errorf("backend does not support mirroring")
	}

	_, unlocker, err := db.Lock(ctx, db.Query(
		db.UpsertVolume(volumeName, backendUUID),
	))
	defer unlocker()
	if err != nil {
		Logc(ctx).WithField("volume", volumeName).WithError(err).Error(
			"Failed to lock volume for upsert.")
		return err
	}

	return backend.EstablishMirror(ctx, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
		replicationSchedule)
}

func (o *ConcurrentTridentOrchestrator) ReestablishMirror(
	ctx context.Context, backendUUID, volumeName, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
	replicationSchedule string,
) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	var err error
	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("mirror_reestablish", &err)()

	backend, err := getInconsistentBackendByUUID(backendUUID)
	if err != nil {
		return err
	}

	if !backend.CanMirror() {
		return fmt.Errorf("backend does not support mirroring")
	}

	_, unlocker, err := db.Lock(ctx, db.Query(
		db.UpsertVolume(volumeName, backendUUID),
	))
	defer unlocker()
	if err != nil {
		Logc(ctx).WithField("volume", volumeName).WithError(err).Error(
			"Failed to lock volume for upsert.")
		return err
	}

	return backend.ReestablishMirror(ctx, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
		replicationSchedule)
}

func (o *ConcurrentTridentOrchestrator) PromoteMirror(
	ctx context.Context, backendUUID, volumeName, localInternalVolumeName, remoteVolumeHandle, snapshotHandle string,
) (bool, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	var err error
	if o.bootstrapError != nil {
		return false, o.bootstrapError
	}
	defer recordTiming("mirror_promote", &err)()

	backend, err := getInconsistentBackendByUUID(backendUUID)
	if err != nil {
		return false, err
	}

	if !backend.CanMirror() {
		return false, fmt.Errorf("backend does not support mirroring")
	}

	_, unlocker, err := db.Lock(ctx, db.Query(
		db.UpsertVolume(volumeName, backendUUID),
	))
	defer unlocker()
	if err != nil {
		Logc(ctx).WithField("volume", volumeName).WithError(err).Error(
			"Failed to lock volume for upsert.")
		return false, err
	}

	return backend.PromoteMirror(ctx, localInternalVolumeName, remoteVolumeHandle, snapshotHandle)
}

func (o *ConcurrentTridentOrchestrator) GetMirrorStatus(
	ctx context.Context, backendUUID, localInternalVolumeName, remoteVolumeHandle string,
) (string, error) {
	var err error

	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return "", o.bootstrapError
	}
	defer recordTiming("mirror_status", &err)()

	backend, err := getInconsistentBackendByUUID(backendUUID)
	if err != nil {
		return "", err
	}

	if !backend.CanMirror() {
		return "", fmt.Errorf("backend does not support mirroring")
	}
	return backend.GetMirrorStatus(ctx, localInternalVolumeName, remoteVolumeHandle)
}

func (o *ConcurrentTridentOrchestrator) CanBackendMirror(ctx context.Context, backendUUID string) (bool, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	var err error

	if o.bootstrapError != nil {
		return false, o.bootstrapError
	}
	defer recordTiming("mirror_capable", &err)()

	backend, err := getInconsistentBackendByUUID(backendUUID)
	if err != nil {
		return false, err
	}
	return backend.CanMirror(), nil
}

func (o *ConcurrentTridentOrchestrator) ReleaseMirror(
	ctx context.Context, backendUUID, volumeName, localInternalVolumeName string,
) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	var err error
	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("mirror_release", &err)()

	backend, err := getInconsistentBackendByUUID(backendUUID)
	if err != nil {
		return err
	}

	if !backend.CanMirror() {
		return fmt.Errorf("backend does not support mirroring")
	}

	_, unlocker, err := db.Lock(ctx, db.Query(
		db.UpsertVolume(volumeName, backendUUID),
	))
	defer unlocker()
	if err != nil {
		Logc(ctx).WithField("volume", volumeName).WithError(err).Error(
			"Failed to lock volume for upsert.")
		return err
	}

	return backend.ReleaseMirror(ctx, localInternalVolumeName)
}

func (o *ConcurrentTridentOrchestrator) GetReplicationDetails(
	ctx context.Context, backendUUID, localInternalVolumeName, remoteVolumeHandle string,
) (string, string, string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	var err error
	if o.bootstrapError != nil {
		return "", "", "", o.bootstrapError
	}
	defer recordTiming("replication_details", &err)()

	backend, err := getInconsistentBackendByUUID(backendUUID)
	if err != nil {
		return "", "", "", err
	}

	if !backend.CanMirror() {
		return "", "", "", fmt.Errorf("backend does not support mirroring")
	}

	return backend.GetReplicationDetails(ctx, localInternalVolumeName, remoteVolumeHandle)
}

func (o *ConcurrentTridentOrchestrator) UpdateMirror(ctx context.Context, volumeName, snapshotName string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	var err error
	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("update_mirror", &err)()

	// Get volume
	tridentVolume, err := o.getVolume(ctx, volumeName)
	if err != nil {
		return fmt.Errorf("could not find volume '%v' in Trident; %v", volumeName, err)
	}

	backendUUID := tridentVolume.BackendUUID

	backend, err := getInconsistentBackendByUUID(backendUUID)
	if err != nil {
		return fmt.Errorf("backend %s not found", backendUUID)
	}

	if !backend.CanMirror() {
		return fmt.Errorf("backend does not support mirroring")
	}

	_, unlocker, err := db.Lock(ctx, db.Query(
		db.UpsertVolume(volumeName, backendUUID),
	))
	defer unlocker()
	if err != nil {
		Logc(ctx).WithField("volume", volumeName).WithError(err).Error(
			"Failed to lock volume for upsert.")
		return err
	}

	logFields := LogFields{
		"volume":   volumeName,
		"snapshot": snapshotName,
	}

	// Mirror update
	Logc(ctx).WithFields(logFields).Info("Mirror update in progress.")
	return backend.UpdateMirror(ctx, tridentVolume.Config.InternalName, snapshotName)
}

func (o *ConcurrentTridentOrchestrator) CheckMirrorTransferState(
	ctx context.Context, volumeName string,
) (*time.Time, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	var err error
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	defer recordTiming("check_mirror_transfer_state", &err)()

	// Get volume
	tridentVolume, err := o.getVolume(ctx, volumeName)
	if err != nil {
		return nil, fmt.Errorf("could not find volume '%v' in Trident; %v", volumeName, err)
	}

	// Get backend to ensure it can mirror
	backend, err := getInconsistentBackendByUUID(tridentVolume.BackendUUID)
	if err != nil {
		return nil, fmt.Errorf("backend %s not found", tridentVolume.BackendUUID)
	}

	if !backend.CanMirror() {
		return nil, fmt.Errorf("backend does not support mirroring")
	}

	// Check transfer state of mirror relationship
	return backend.CheckMirrorTransferState(ctx, tridentVolume.Config.InternalName)
}

func (o *ConcurrentTridentOrchestrator) GetMirrorTransferTime(
	ctx context.Context, volumeName string,
) (*time.Time, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	var err error
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	defer recordTiming("get_mirror_transfer_time", &err)()

	// Get volume
	tridentVolume, err := o.getVolume(ctx, volumeName)
	if err != nil {
		return nil, fmt.Errorf("could not find volume '%v' in Trident; %v", volumeName, err)
	}

	// Get backend to ensure it can mirror
	backend, err := getInconsistentBackendByUUID(tridentVolume.BackendUUID)
	if err != nil {
		return nil, fmt.Errorf("backend %s not found", tridentVolume.BackendUUID)
	}

	if !backend.CanMirror() {
		return nil, fmt.Errorf("backend does not support mirroring")
	}

	// Get last transfer time of mirror relationship
	return backend.GetMirrorTransferTime(ctx, tridentVolume.Config.InternalName)
}

func (o *ConcurrentTridentOrchestrator) GetCHAP(
	ctx context.Context, volumeName, nodeName string,
) (chapInfo *models.IscsiChapInfo, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("get_chap", &err)()

	volume, err := o.getVolume(ctx, volumeName)
	if err != nil {
		return nil, err
	}

	results, unlocker, err := db.Lock(ctx, db.Query(db.InconsistentReadBackend(volume.BackendUUID)))
	defer unlocker()
	if err != nil {
		return nil, err
	}

	backend := results[0].Backend.Read
	if backend == nil {
		return nil, fmt.Errorf("backend %s not found for volume %s", volume.BackendUUID, volumeName)
	}

	return backend.GetChapInfo(ctx, volumeName, nodeName)
}

/******************************************************************************
REST API Handlers for retrieving and setting the current logging configuration.
******************************************************************************/

func (o *ConcurrentTridentOrchestrator) GetLogLevel(ctx context.Context) (string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return "", o.bootstrapError
	}

	o.mtx.Lock()
	defer o.mtx.Unlock()

	return GetDefaultLogLevel(), nil
}

func (o *ConcurrentTridentOrchestrator) SetLogLevel(ctx context.Context, level string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mtx.Lock()
	defer o.mtx.Unlock()

	if err := SetDefaultLogLevel(level); err != nil {
		return err
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) GetSelectedLoggingWorkflows(ctx context.Context) (string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return "", o.bootstrapError
	}

	o.mtx.Lock()
	defer o.mtx.Unlock()

	return GetSelectedWorkFlows(), nil
}

func (o *ConcurrentTridentOrchestrator) ListLoggingWorkflows(ctx context.Context) ([]string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return []string{}, o.bootstrapError
	}

	o.mtx.Lock()
	defer o.mtx.Unlock()

	return ListWorkflowTypes(), nil
}

func (o *ConcurrentTridentOrchestrator) SetLoggingWorkflows(ctx context.Context, flows string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mtx.Lock()
	defer o.mtx.Unlock()

	err := SetWorkflows(flows)
	if err != nil {
		return err
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) GetSelectedLogLayers(ctx context.Context) (string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return "", o.bootstrapError
	}

	o.mtx.Lock()
	defer o.mtx.Unlock()

	return GetSelectedLogLayers(), nil
}

func (o *ConcurrentTridentOrchestrator) ListLogLayers(ctx context.Context) ([]string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return []string{}, o.bootstrapError
	}

	o.mtx.Lock()
	defer o.mtx.Unlock()

	return ListLogLayers(), nil
}

func (o *ConcurrentTridentOrchestrator) SetLogLayers(ctx context.Context, layers string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mtx.Lock()
	defer o.mtx.Unlock()

	err := SetLogLayers(layers)
	if err != nil {
		return err
	}

	return nil
}

func (o *ConcurrentTridentOrchestrator) updateBackendOnPersistentStore(
	ctx context.Context, backend storage.Backend, newBackend bool,
) error {
	// Update the persistent store with the backend information
	if o.bootstrapped || config.UsingPassthroughStore {
		var err error
		if newBackend {
			err = o.storeClient.AddBackend(ctx, backend)
		} else {
			Logc(ctx).WithFields(LogFields{
				"backend":             backend.Name(),
				"backend.BackendUUID": backend.BackendUUID(),
				"backend.ConfigRef":   backend.ConfigRef(),
			}).Debug("Updating an existing backend.")
			err = o.storeClient.UpdateBackend(ctx, backend)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ============================================================================
// Autogrow Policy CRUD Operations
// ============================================================================

// AddAutogrowPolicy creates a new Autogrow policy in the orchestrator. The policy can be referenced
// by volumes directly or as a default in StorageClass. This method does
// not persist policies to the backing store as they are managed by the frontend.
func (o *ConcurrentTridentOrchestrator) AddAutogrowPolicy(
	ctx context.Context, config *storage.AutogrowPolicyConfig,
) (*storage.AutogrowPolicyExternal, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	var err error
	defer recordTiming("autogrowpolicy_add", &err)()

	Logc(ctx).WithFields(LogFields{
		"autogrowPolicyName": config.Name,
		"usedThreshold":      config.UsedThreshold,
		"growthAmount":       config.GrowthAmount,
		"maxSize":            config.MaxSize,
	}).Debug("Adding Autogrow policy.")

	// Lock the Autogrow policy
	results, unlocker, lockErr := db.Lock(ctx, db.Query(db.UpsertAutogrowPolicy(config.Name)))
	if lockErr != nil {
		unlocker()
		return nil, fmt.Errorf("error locking Autogrow policy %s; %w", config.Name, lockErr)
	}

	// Check if policy already exists
	if results[0].AutogrowPolicy.Read != nil {
		unlocker()
		return nil, errors.AlreadyExistsError("Autogrow policy %s already exists", config.Name)
	}

	autogrowPolicy := storage.NewAutogrowPolicyFromConfig(config)
	results[0].AutogrowPolicy.Upsert(autogrowPolicy)

	// UNLOCK BEFORE calling helper function that acquires more locks
	unlocker()

	// Retroactively re-evaluate existing volumes for this newly created policy
	// This works for both Success and Failed states:
	// - Success: volumes get associated with the policy
	// - Failed: volumes get updated with Reason="Unusable"
	Logc(ctx).WithFields(LogFields{
		"autogrowPolicyName": config.Name,
		"state":              autogrowPolicy.State(),
	}).Debug("Re-evaluating volumes for newly added policy.")
	o.reevaluateVolumesForPolicy(ctx, config.Name)

	Logc(ctx).WithField("autogrowPolicyName", config.Name).Info("Autogrow policy added.")

	// Get volumes using concurrent_cache query (should be empty for new policy)
	volResults, volUnlocker, volLockErr := db.Lock(ctx, db.Query(db.ListVolumesForAutogrowPolicy(config.Name)))
	defer volUnlocker()

	var volumes []string
	if volLockErr == nil {
		volumes = make([]string, 0, len(volResults[0].Volumes))
		for _, vol := range volResults[0].Volumes {
			volumes = append(volumes, vol.Config.Name)
		}
	} else {
		volumes = []string{}
	}

	return autogrowPolicy.ConstructExternalWithVolumes(ctx, volumes), nil
}

// UpdateAutogrowPolicy updates an existing Autogrow policy with new configuration values. Volumes
// already associated with the policy will use the updated settings. This method updates the policy
// configuration but does not re-associate existing volumes.
func (o *ConcurrentTridentOrchestrator) UpdateAutogrowPolicy(
	ctx context.Context, config *storage.AutogrowPolicyConfig,
) (*storage.AutogrowPolicyExternal, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	var err error
	defer recordTiming("autogrowpolicy_update", &err)()

	Logc(ctx).WithFields(LogFields{
		"autogrowPolicyName": config.Name,
		"usedThreshold":      config.UsedThreshold,
		"growthAmount":       config.GrowthAmount,
		"maxSize":            config.MaxSize,
	}).Debug("Updating Autogrow policy.")

	// Lock the Autogrow policy
	results, unlocker, lockErr := db.Lock(ctx, db.Query(db.UpsertAutogrowPolicy(config.Name)))
	if lockErr != nil {
		unlocker()
		return nil, fmt.Errorf("error locking Autogrow policy %s; %w", config.Name, lockErr)
	}

	policy := results[0].AutogrowPolicy.Read
	if policy == nil {
		unlocker()
		return nil, errors.AutogrowPolicyNotFoundError(config.Name)
	}

	// Store old state to detect state transitions
	oldAGPState := policy.State()

	// Update the policy spec + status in cache to match with the external world (CR)
	policy.UpdateFromConfig(config)
	results[0].AutogrowPolicy.Upsert(policy)

	// UNLOCK BEFORE calling helper functions that acquire more locks
	unlocker()

	// Handle state transitions by re-evaluating volume associations
	if oldAGPState != config.State {
		// State transitions require re-evaluation because:
		// - Success → Failed/Deleting: Volumes can't use this policy anymore
		// - Failed/Deleting → Success: Volumes can now use this policy
		// - Without re-evaluation, volumes would have stale associations
		Logc(ctx).WithFields(LogFields{
			"autogrowPolicyName": config.Name,
			"oldAGPState":        oldAGPState,
			"newAGPState":        config.State,
		}).Info("Autogrow policy state changed, re-evaluating volume associations.")

		if config.State.IsSuccess() {
			// Policy is now Success (was Failed/Deleting) - re-evaluate ALL volumes
			o.reevaluateVolumesForPolicy(ctx, config.Name)
		} else {
			// Policy is Failed/Deleting
			// NO re-resolution needed - policy settings haven't changed, so NO fallbacks exist
			// Just invalidate all volumes directly
			o.invalidateVolumesForPolicy(ctx, config.Name)
		}
	}

	Logc(ctx).WithField("autogrowPolicyName", config.Name).Info("Autogrow policy updated.")

	// Get volumes using concurrent_cache query
	volResults, volUnlocker, volLockErr := db.Lock(ctx, db.Query(db.ListVolumesForAutogrowPolicy(config.Name)))
	defer volUnlocker()

	var volumes []string
	if volLockErr == nil {
		volumes = make([]string, 0, len(volResults[0].Volumes))
		for _, vol := range volResults[0].Volumes {
			volumes = append(volumes, vol.Config.Name)
		}
	} else {
		volumes = []string{}
	}

	return policy.ConstructExternalWithVolumes(ctx, volumes), nil
}

// DeleteAutogrowPolicy removes an Autogrow policy from the orchestrator.
//
// Behavior:
//   - If policy has no volumes: Immediately deletes from cache (hard delete)
//   - If policy has volumes: Sets state to Deleting (soft delete), returns nil
//   - Soft-deleted policies are automatically cleaned up when the last volume is disassociated
//
// This matches the backend deletion pattern for consistency.
func (o *ConcurrentTridentOrchestrator) DeleteAutogrowPolicy(
	ctx context.Context, agPolicyName string,
) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	var err error
	defer recordTiming("autogrowpolicy_delete", &err)()

	Logc(ctx).WithField("autogrowPolicyName", agPolicyName).Info("Deleting Autogrow policy.")

	// Phase 1: Read policy to verify it exists
	readResults, readUnlocker, readLockErr := db.Lock(ctx, db.Query(db.ReadAutogrowPolicy(agPolicyName)))
	if readLockErr != nil {
		readUnlocker()
		return fmt.Errorf("error locking Autogrow policy %s; %w", agPolicyName, readLockErr)
	}

	policy := readResults[0].AutogrowPolicy.Read
	if policy == nil {
		readUnlocker()
		return errors.AutogrowPolicyNotFoundError(agPolicyName)
	}

	// UNLOCK policy before checking volumes
	readUnlocker()

	// Check if any volumes use this policy (no locks held)
	volResults, volUnlocker, volLockErr := db.Lock(ctx, db.Query(db.ListVolumesForAutogrowPolicy(agPolicyName)))
	if volLockErr != nil {
		volUnlocker()
		return fmt.Errorf("error checking volumes for policy %s; %w", agPolicyName, volLockErr)
	}
	volumes := make([]string, 0, len(volResults[0].Volumes))
	for _, vol := range volResults[0].Volumes {
		volumes = append(volumes, vol.Config.Name)
	}
	volUnlocker()

	if len(volumes) > 0 {
		// Soft delete: Re-acquire policy lock for update
		updateResults, updateUnlocker, updateLockErr := db.Lock(ctx, db.Query(db.UpsertAutogrowPolicy(agPolicyName)))
		defer updateUnlocker()
		if updateLockErr != nil {
			return fmt.Errorf("error locking Autogrow policy %s; %w", agPolicyName, updateLockErr)
		}

		updatePolicy := updateResults[0].AutogrowPolicy.Read
		if updatePolicy == nil {
			return errors.AutogrowPolicyNotFoundError(agPolicyName)
		}

		// Set Deleting state (soft delete)
		updatePolicy.UpdateFromConfig(&storage.AutogrowPolicyConfig{
			Name:  updatePolicy.Name(),
			State: storage.AutogrowPolicyStateDeleting,
		})
		updateResults[0].AutogrowPolicy.Upsert(updatePolicy)

		Logc(ctx).WithFields(LogFields{
			"autogrowPolicyName": agPolicyName,
			"volumeCount":        len(volumes),
			"volumes":            volumes,
		}).Info("Autogrow policy has associated volumes, setting to Deleting state.")

		return nil
	}

	// Hard delete: Re-acquire lock for deletion
	deleteResults, deleteUnlocker, deleteLockErr := db.Lock(ctx, db.Query(db.DeleteAutogrowPolicy(agPolicyName)))
	defer deleteUnlocker()
	if deleteLockErr != nil {
		return fmt.Errorf("error locking Autogrow policy %s; %w", agPolicyName, deleteLockErr)
	}

	deletePolicy := deleteResults[0].AutogrowPolicy.Read
	if deletePolicy == nil {
		// Policy was already deleted (race condition - another thread deleted it)
		Logc(ctx).WithField("autogrowPolicyName", agPolicyName).
			Debug("Policy already deleted by another operation.")
		return nil
	}

	// Proceed with hard delete
	// If a volume is created referencing this policy during deletion,
	// the volume will be created successfully but without the autogrow policy association (warning logged)
	deleteResults[0].AutogrowPolicy.Delete()

	Logc(ctx).WithField("autogrowPolicyName", agPolicyName).Info("Autogrow policy deleted.")
	return nil
}

// GetAutogrowPolicy retrieves a single Autogrow policy by name. The returned external representation
// includes the policy configuration and a list of all volumes currently using the policy.
func (o *ConcurrentTridentOrchestrator) GetAutogrowPolicy(
	ctx context.Context, agPolicyName string,
) (*storage.AutogrowPolicyExternal, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	var err error
	defer recordTiming("autogrowpolicy_get", &err)()

	// Lock the Autogrow policy for read
	results, unlocker, lockErr := db.Lock(ctx, db.Query(db.InconsistentReadAutogrowPolicy(agPolicyName)))
	if lockErr != nil {
		unlocker()
		return nil, fmt.Errorf("error locking Autogrow policy %s; %w", agPolicyName, lockErr)
	}

	policy := results[0].AutogrowPolicy.Read
	if policy == nil {
		unlocker()
		return nil, errors.AutogrowPolicyNotFoundError(agPolicyName)
	}

	// UNLOCK policy before acquiring volume locks
	unlocker()

	// Get volumes using concurrent_cache query
	volResults, volUnlocker, volLockErr := db.Lock(ctx, db.Query(db.ListVolumesForAutogrowPolicy(agPolicyName)))
	defer volUnlocker()

	var volumes []string
	if volLockErr == nil {
		volumes = make([]string, 0, len(volResults[0].Volumes))
		for _, vol := range volResults[0].Volumes {
			volumes = append(volumes, vol.Config.Name)
		}
	} else {
		Logc(ctx).WithError(volLockErr).Warn("Failed to get volumes for policy.")
		volumes = []string{}
	}

	return policy.ConstructExternalWithVolumes(ctx, volumes), nil
}

// ListAutogrowPolicies returns all Autogrow policies currently registered in the orchestrator. Each
// policy includes its configuration and the list of volumes using it.
func (o *ConcurrentTridentOrchestrator) ListAutogrowPolicies(
	ctx context.Context,
) ([]*storage.AutogrowPolicyExternal, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	var err error
	defer recordTiming("autogrowpolicy_list", &err)()

	// List all Autogrow policies
	results, unlocker, lockErr := db.Lock(ctx, db.Query(db.ListAutogrowPolicies()))
	if lockErr != nil {
		unlocker()
		return nil, fmt.Errorf("error listing Autogrow policies; %w", lockErr)
	}

	policiesList := results[0].AutogrowPolicies

	// UNLOCK policy list before acquiring volume locks
	unlocker()

	policies := make([]*storage.AutogrowPolicyExternal, 0, len(policiesList))
	for _, policy := range policiesList {
		// Get volumes for each policy using concurrent_cache query
		volResults, volUnlocker, volLockErr := db.Lock(ctx, db.Query(db.ListVolumesForAutogrowPolicy(policy.Name())))

		var volumes []string
		if volLockErr == nil {
			volumes = make([]string, 0, len(volResults[0].Volumes))
			for _, vol := range volResults[0].Volumes {
				volumes = append(volumes, vol.Config.Name)
			}
		} else {
			Logc(ctx).WithError(volLockErr).Warn("Failed to get volumes for policy.")
			volumes = []string{}
		}
		volUnlocker()

		policies = append(policies, policy.ConstructExternalWithVolumes(ctx, volumes))
	}

	return policies, nil
}

// UpdateVolumeAutogrowPolicy updates a volume when its autogrow policy changes.
// This resolves the new effective policy, validates it exists, and updates associations.
// Returns AutogrowPolicyNotFoundError or AutogrowPolicyNotUsableError for event recording.
func (o *ConcurrentTridentOrchestrator) UpdateVolumeAutogrowPolicy(
	ctx context.Context, volumeName, requestedAutogrowPolicy string,
) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	var err error
	defer recordTiming("volume_autogrow_policy_update", &err)()

	Logc(ctx).WithFields(LogFields{
		"volumeName":              volumeName,
		"requestedAutogrowPolicy": requestedAutogrowPolicy,
	}).Debug("Updating volume Autogrow policy.")

	// Inconsistent read first to know whether this is a source volume or subordinate volume
	checkResults, checkUnlocker, checkErr := db.Lock(ctx, db.Query(db.InconsistentReadVolume(volumeName)),
		db.Query(db.InconsistentReadSubordinateVolume(volumeName)))
	checkUnlocker()
	if checkErr != nil {
		return fmt.Errorf("error checking volume type: %w", checkErr)
	}

	isSubordinate := checkResults[1].SubordinateVolume.Read != nil
	var sourceName string
	if isSubordinate {
		sourceName = checkResults[1].SubordinateVolume.Read.Config.ShareSourceVolume
	}

	// Lock the volume for update based on type
	var results []db.Result
	var unlocker func()
	var lockErr error

	if isSubordinate {
		results, unlocker, lockErr = db.Lock(ctx, db.Query(db.UpsertSubordinateVolume(volumeName, sourceName)))
	} else {
		results, unlocker, lockErr = db.Lock(ctx, db.Query(db.UpsertVolume(volumeName, "")))
	}

	if lockErr != nil {
		unlocker()
		return fmt.Errorf("error locking volume %s; %w", volumeName, lockErr)
	}

	// Extract volume and upserter based on type
	var volume *storage.Volume
	var upserter func(*storage.Volume)

	if isSubordinate {
		volume = results[0].SubordinateVolume.Read
		upserter = results[0].SubordinateVolume.Upsert
	} else {
		volume = results[0].Volume.Read
		upserter = results[0].Volume.Upsert
	}

	if volume == nil {
		unlocker()
		return errors.NotFoundError("volume %s not found", volumeName)
	}

	// Store old effective Autogrow policy for comparison
	oldEffectiveAGPolicy := volume.EffectiveAGPolicy

	// Update the requested autogrow policy in volume config
	volume.Config.RequestedAutogrowPolicy = requestedAutogrowPolicy

	// Persist the updated volume to the store
	if err := o.storeClient.UpdateVolume(ctx, volume); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeName": volumeName,
		}).Error("Failed to persist volume Autogrow policy update to store.")
		unlocker()
		return err
	}

	newEffectiveAGPolicy, policyErr := o.resolveAndSetEffectiveAutogrowPolicy(ctx, volume, "volume autogrow policy update", "warn")

	// Log if policy changed
	if oldEffectiveAGPolicy.PolicyName != newEffectiveAGPolicy.PolicyName {
		Logc(ctx).WithFields(LogFields{
			"volumeName":            volumeName,
			"oldAutogrowPolicyName": oldEffectiveAGPolicy.PolicyName,
			"newAutogrowPolicyName": newEffectiveAGPolicy.PolicyName,
		}).Info("Updated volume's effective Autogrow policy.")
	}

	// Upsert the volume back to cache
	upserter(volume)
	unlocker()

	Logc(ctx).WithField("volumeName", volumeName).Debug(
		"Syncing volume publications after Autogrow policy update")

	// Propagate AGP changes to all the VPs associated with this volume
	vpResults, vpUnlocker, vpErr := db.Lock(ctx, db.Query(db.ListVolumePublicationsForVolume(volumeName)))
	if vpErr != nil {
		Logc(ctx).WithError(vpErr).Warn("Failed to lock VPs for sync after Autogrow policy update")
		vpUnlocker()
		return vpErr
	}
	vps := vpResults[0].VolumePublications
	syncNeededVPS := make([]*models.VolumePublication, 0)
	vpUnlocker()

	Logc(ctx).WithFields(LogFields{
		"volumeName": volumeName,
		"vpCount":    len(vps),
	}).Debug("Locked volume publications for Autogrow policy sync")

	// Detect and modify VPs that need syncing
	for _, vp := range vps {
		// Update cache with modified VP (using upsert query)
		vpUpsertResults, vpUpsertUnlocker, vpUpsertErr := db.Lock(ctx,
			db.Query(db.UpsertVolumePublication(vp.VolumeName, vp.NodeName)))
		if vpUpsertErr != nil {
			Logc(ctx).WithFields(LogFields{
				"volumeName": vp.VolumeName,
				"nodeName":   vp.NodeName,
			}).WithError(vpUpsertErr).Error("Failed to update VP cache")
			vpUpsertUnlocker()
			continue
		}
		vp = vpUpsertResults[0].VolumePublication.Read
		syncNeeded := syncVolumePublicationFields(volume, vp)

		if syncNeeded {
			Logc(ctx).WithFields(LogFields{
				"volumeName": vp.VolumeName,
				"nodeName":   vp.NodeName,
			}).Debug("Volume publication requires sync after policy update")
			syncNeededVPS = append(syncNeededVPS, vp)

			vpUpsertResults[0].VolumePublication.Upsert(vp)
		}
		vpUpsertUnlocker()
	}

	Logc(ctx).WithFields(LogFields{
		"volumeName":      volumeName,
		"totalVPs":        len(vps),
		"syncNeededCount": len(syncNeededVPS),
	}).Debug("Completed VP sync check after Autogrow policy update")

	// Propagate changes with optimization for single VP
	if len(syncNeededVPS) > 1 {
		Logc(ctx).WithFields(LogFields{
			"volumeName": volumeName,
			"vpCount":    len(syncNeededVPS),
		}).Debug("Asynchronously propagating Autogrow policy changes to volume publications")
		// For multiple VPs, use async goroutine to avoid blocking
		go o.SyncVolumePublications(ctx, syncNeededVPS)
	} else if len(syncNeededVPS) == 1 {
		Logc(ctx).WithFields(LogFields{
			"volumeName": volumeName,
			"nodeName":   syncNeededVPS[0].NodeName,
		}).Debug("Synchronously propagating Autogrow policy change to single volume publication")
		// For single VP, update synchronously in this goroutine
		err := o.syncVolumePublication(ctx, syncNeededVPS[0])
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"volumeName": volumeName,
				"nodeName":   syncNeededVPS[0].NodeName,
			}).WithError(err).Errorf("failed to update Autogrow policy change to volume publication store")
		}
	}

	return policyErr
}

// ============================================================================
// Autogrow Policy Helper Methods
// ============================================================================

// resolveEffectiveAutogrowPolicy determines the effective Autogrow policy for a volume.
// It uses the stateless utils function and concurrent cache's StorageClass data.
// Returns policy info (name + reason) and error if the policy doesn't exist or is not usable.
func (o *ConcurrentTridentOrchestrator) resolveEffectiveAutogrowPolicy(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (models.EffectiveAutogrowPolicyInfo, error) {

	// Check if volume is fundamentally ineligible for autogrow
	// This must be checked FIRST before any policy resolution
	if isVolumeAutogrowIneligible(volumeConfig) {
		Logc(ctx).WithFields(LogFields{
			"volume":             volumeConfig.Name,
			"importNotManaged":   volumeConfig.ImportNotManaged,
			"importOriginalName": volumeConfig.ImportOriginalName,
			"spaceReserve":       volumeConfig.SpaceReserve,
			"readOnlyClone":      volumeConfig.ReadOnlyClone,
			"shareSourceVolume":  volumeConfig.ShareSourceVolume,
		}).Debug("Volume is ineligible for autogrow based on configuration.")
		return models.EffectiveAutogrowPolicyInfo{
			PolicyName: "",
			Reason:     models.AutogrowPolicyReasonIneligible,
		}, nil
	}

	// Get StorageClass Autogrow policy from concurrent cache
	// Use inconsistent read for performance since this is a non-critical lookup
	scAutogrowPolicy := ""
	if volumeConfig.StorageClass != "" {
		results, unlocker, lockErr := db.Lock(ctx, db.Query(db.InconsistentReadStorageClass(volumeConfig.StorageClass)))
		if lockErr == nil {
			sc := results[0].StorageClass.Read
			if sc != nil {
				scAutogrowPolicy = sc.GetAutogrowPolicy()
			} else {
				Logc(ctx).WithField("storageClass", volumeConfig.StorageClass).Warn(
					"StorageClass not found in orchestrator while resolving Autogrow policy.")
			}
			unlocker()
		}
	}

	// Get volume autogrow policy
	volumeAutogrowPolicy := volumeConfig.RequestedAutogrowPolicy

	effectiveAGPolicyName := autogrow.ResolveEffectiveAutogrowPolicy(
		ctx,
		volumeConfig.Name,
		volumeConfig.RequestedAutogrowPolicy,
		scAutogrowPolicy,
	)

	// If empty, determine WHY it's empty
	if effectiveAGPolicyName == "" {
		// Check if explicitly disabled with "none" in either volume or StorageClass (case-insensitive)
		if strings.EqualFold(volumeAutogrowPolicy, autogrow.AutogrowPolicyNone) ||
			strings.EqualFold(scAutogrowPolicy, autogrow.AutogrowPolicyNone) {
			return models.EffectiveAutogrowPolicyInfo{
				PolicyName: "",
				Reason:     models.AutogrowPolicyReasonDisabled,
			}, nil
		}
		// Not configured at all
		return models.EffectiveAutogrowPolicyInfo{
			PolicyName: "",
			Reason:     models.AutogrowPolicyReasonNotConfigured,
		}, nil
	}

	// Validate policy exists in concurrent cache
	// Use inconsistent read to check policy existence and state
	results, unlocker, lockErr := db.Lock(ctx, db.Query(db.InconsistentReadAutogrowPolicy(effectiveAGPolicyName)))
	defer unlocker()
	if lockErr != nil {
		return models.EffectiveAutogrowPolicyInfo{
			PolicyName: "",
			Reason:     models.AutogrowPolicyReasonNotFound,
		}, errors.AutogrowPolicyNotFoundError(effectiveAGPolicyName)
	}

	autogrowPolicy := results[0].AutogrowPolicy.Read
	if autogrowPolicy == nil {
		// Policy doesn't exist - return empty string and AutogrowPolicyNotFoundError
		return models.EffectiveAutogrowPolicyInfo{
			PolicyName: "",
			Reason:     models.AutogrowPolicyReasonNotFound,
		}, errors.AutogrowPolicyNotFoundError(effectiveAGPolicyName)
	}

	if !autogrowPolicy.State().IsSuccess() {
		// Policy exists but is Failed or Deleting:
		// - Failed: Validation failed, policy shouldn't be used
		// - Deleting: Policy deletion in progress, volumes being migrated
		// Return policy info (empty string + reason) to prevent new associations
		Logc(ctx).WithFields(LogFields{
			"volume": volumeConfig.Name,
			"policy": effectiveAGPolicyName,
			"state":  autogrowPolicy.State(),
		}).Debug("Autogrow policy exists but not in Success state.")
		// Return NEW AutogrowPolicyNotUsableError
		return models.EffectiveAutogrowPolicyInfo{
			PolicyName: "",
			Reason:     models.AutogrowPolicyReasonUnusable,
		}, errors.AutogrowPolicyNotUsableError(effectiveAGPolicyName, string(autogrowPolicy.State()))
	}

	// Policy is active
	return models.EffectiveAutogrowPolicyInfo{
		PolicyName: effectiveAGPolicyName,
		Reason:     models.AutogrowPolicyReasonActive,
	}, nil
}

// invalidateVolumesForPolicy invalidates volumes using a policy when it transitions to Failed or Deleting state.
// Uses concurrent_cache query to find volumes associated with the policy.
func (o *ConcurrentTridentOrchestrator) invalidateVolumesForPolicy(
	ctx context.Context, agPolicyName string,
) {
	// Use concurrent_cache query to find volumes with this policy
	results, unlocker, lockErr := db.Lock(ctx, db.Query(db.ListVolumesForAutogrowPolicy(agPolicyName)),
		db.Query(db.ListSubordinateVolumesForAutogrowPolicy(agPolicyName)))
	if lockErr != nil {
		unlocker()
		Logc(ctx).WithError(lockErr).Warn("Failed to list volumes for invalidation.")
		return
	}

	associatedVolumes := results[0].Volumes
	associatedSubordinateVolumes := results[1].SubordinateVolumes
	allAssociatedVolumes := append(associatedVolumes, associatedSubordinateVolumes...)

	// UNLOCK BEFORE processing individual volumes (avoids holding list lock)
	unlocker()

	for _, volume := range allAssociatedVolumes {
		isSubordinate := volume.IsSubordinate()

		// Lock the volume for update
		var volResults []db.Result
		var volUnlocker func()
		var volLockErr error

		if isSubordinate {
			volResults, volUnlocker, volLockErr = db.Lock(ctx,
				db.Query(db.UpsertSubordinateVolume(volume.Config.Name, volume.Config.ShareSourceVolume)))
		} else {
			volResults, volUnlocker, volLockErr = db.Lock(ctx,
				db.Query(db.UpsertVolume(volume.Config.Name, "")))
		}

		if volLockErr != nil {
			Logc(ctx).WithFields(LogFields{
				"volume":     volume.Config.Name,
				"policyName": agPolicyName,
			}).WithError(volLockErr).Warn("Failed to lock volume for invalidation.")
			volUnlocker()
			continue
		}

		// Extract volume and upserter based on type
		var vol *storage.Volume
		var upserter func(*storage.Volume)

		if isSubordinate {
			vol = volResults[0].SubordinateVolume.Read
			upserter = volResults[0].SubordinateVolume.Upsert
		} else {
			vol = volResults[0].Volume.Read
			upserter = volResults[0].Volume.Upsert
		}

		if vol == nil {
			volUnlocker()
			continue
		}

		// Re-validate: Only invalidate if still using this policy (race condition protection)
		if vol.EffectiveAGPolicy.PolicyName == agPolicyName {
			vol.EffectiveAGPolicy = models.EffectiveAutogrowPolicyInfo{
				PolicyName: "",
				Reason:     models.AutogrowPolicyReasonUnusable,
			}

			upserter(vol)
		}

		volUnlocker()
	}

	Logc(ctx).WithFields(LogFields{
		"policyName":   agPolicyName,
		"volumesTotal": len(associatedVolumes),
	}).Info("Completed invalidating volumes for policy.")
}

// reevaluateVolumesForPolicy checks all volumes to see if they should now use the given policy.
// Used when a policy transitions to Success state from Failed or Deleting.
func (o *ConcurrentTridentOrchestrator) reevaluateVolumesForPolicy(ctx context.Context, agPolicyName string) {
	// List all volumes
	results, unlocker, lockErr := db.Lock(ctx, db.Query(db.ListVolumesForAutogrowPolicyReevaluation(agPolicyName)),
		db.Query(db.ListSubordinateVolumesForAutogrowPolicyReevaluation(agPolicyName)))
	if lockErr != nil {
		Logc(ctx).WithField("policyName", agPolicyName).WithError(lockErr).
			Warn("Failed to list volumes for re-evaluation.")
		unlocker()
		return
	}
	volumes := results[0].Volumes
	subordinateVolumes := results[1].SubordinateVolumes

	// Combine both type of volumes
	allVolumes := append(volumes, subordinateVolumes...)
	unlocker()

	// No more filtering needed in the loop - query already filtered
	for _, volume := range allVolumes {
		isSubordinate := volume.IsSubordinate()

		// Phase 1: Read lock to check if update is needed (cheap, allows concurrency)
		var volResults []db.Result
		var volUnlocker func()
		var volLockErr error

		if isSubordinate {
			volResults, volUnlocker, volLockErr = db.Lock(ctx, db.Query(db.ReadSubordinateVolume(volume.Config.Name)))
		} else {
			volResults, volUnlocker, volLockErr = db.Lock(ctx, db.Query(db.ReadVolume(volume.Config.Name)))
		}

		if volLockErr != nil {
			Logc(ctx).WithFields(LogFields{
				"volume":     volume.Config.Name,
				"policyName": agPolicyName,
			}).WithError(volLockErr).Warn("Failed to lock volume for re-evaluation.")
			volUnlocker()
			continue
		}

		// Extract volume based on type
		var vol *storage.Volume
		if isSubordinate {
			vol = volResults[0].SubordinateVolume.Read
		} else {
			vol = volResults[0].Volume.Read
		}

		if vol == nil {
			volUnlocker()
			continue
		}

		// Re-resolve effective policy
		effectiveAGPolicy, _ := o.resolveEffectiveAutogrowPolicy(ctx, vol.Config)

		volUnlocker() // Release read lock

		// Only acquire write lock if the effective policy actually changed
		// This works for Success, Failed, NotFound - any state transition
		if vol.EffectiveAGPolicy.PolicyName != effectiveAGPolicy.PolicyName ||
			vol.EffectiveAGPolicy.Reason != effectiveAGPolicy.Reason {
			// Phase 2: Write lock only if update is needed (expensive, exclusive)
			var updateResults []db.Result
			var updateUnlocker func()
			var updateLockErr error

			if isSubordinate {
				updateResults, updateUnlocker, updateLockErr = db.Lock(ctx,
					db.Query(db.UpsertSubordinateVolume(volume.Config.Name, volume.Config.ShareSourceVolume)))
			} else {
				updateResults, updateUnlocker, updateLockErr = db.Lock(ctx,
					db.Query(db.UpsertVolume(volume.Config.Name, "")))
			}

			if updateLockErr != nil {
				Logc(ctx).WithFields(LogFields{
					"volume":     volume.Config.Name,
					"policyName": agPolicyName,
				}).WithError(updateLockErr).Warn("Failed to lock volume for update.")
				updateUnlocker()
				continue
			}

			// Re-read after write lock
			var updateVol *storage.Volume
			var upserter func(*storage.Volume)

			if isSubordinate {
				updateVol = updateResults[0].SubordinateVolume.Read
				upserter = updateResults[0].SubordinateVolume.Upsert
			} else {
				updateVol = updateResults[0].Volume.Read
				upserter = updateResults[0].Volume.Upsert
			}

			if updateVol != nil {
				// Re-resolve with fresh data
				effectiveAGPolicy, _ := o.resolveEffectiveAutogrowPolicy(ctx, updateVol.Config)

				// Always update effective policy
				// This ensures the reason is updated even for Failed/Unusable policies
				updateVol.EffectiveAGPolicy = effectiveAGPolicy
				upserter(updateVol)
			}

			updateUnlocker()
		}

	}

	Logc(ctx).WithFields(LogFields{
		"policyName": agPolicyName,
	}).Info("Completed re-evaluating volumes for policy.")
}

// upsertStorageClassAutogrowPolicyInternal handles Autogrow policy changes in StorageClass.
// This is an internal helper that updates effective policies for all volumes that use this StorageClass
// and have no volume-level autogrow policy.
func (o *ConcurrentTridentOrchestrator) upsertStorageClassAutogrowPolicyInternal(
	ctx context.Context, scName string,
) error {
	// List volumes using this storage class without PVC override (optimized query)
	results, unlocker, lockErr := db.Lock(ctx, db.Query(db.ListVolumesForStorageClassWithoutAutogrowOverride(
		scName)), db.Query(db.ListSubordinateVolumesForStorageClassWithoutAutogrowOverride(scName)))
	if lockErr != nil {
		Logc(ctx).WithField("storageClass", scName).WithError(lockErr).
			Warn("Failed to list volumes for storage class autogrow policy update.")
		unlocker()
		return lockErr
	}
	volumes := results[0].Volumes
	subordinateVolumes := results[1].SubordinateVolumes
	allVolumes := append(volumes, subordinateVolumes...)
	unlocker()

	var policyErr error

	// Process all matched volumes (no filtering needed - query already filtered)
	for _, volume := range allVolumes {
		isSubordinate := volume.IsSubordinate()

		// Phase 1: Read lock to check if update is needed (cheap, allows concurrency)
		var volResults []db.Result
		var volUnlocker func()
		var volLockErr error

		if isSubordinate {
			volResults, volUnlocker, volLockErr = db.Lock(ctx, db.Query(db.ReadSubordinateVolume(volume.Config.Name)))
		} else {
			volResults, volUnlocker, volLockErr = db.Lock(ctx, db.Query(db.ReadVolume(volume.Config.Name)))
		}

		if volLockErr != nil {
			Logc(ctx).WithFields(LogFields{
				"volume":       volume.Config.Name,
				"storageClass": scName,
			}).WithError(volLockErr).Warn("Failed to lock volume for autogrow policy check.")
			volUnlocker()
			continue
		}

		// Extract volume based on type
		var vol *storage.Volume
		if isSubordinate {
			vol = volResults[0].SubordinateVolume.Read
		} else {
			vol = volResults[0].Volume.Read
		}

		if vol == nil {
			volUnlocker()
			continue
		}

		// Resolve the effective policy with current state (read lock phase)
		effectiveAGPolicy, resolveErr := o.resolveEffectiveAutogrowPolicy(ctx, vol.Config)

		volUnlocker() // Release read lock

		if resolveErr != nil {
			policyErr = resolveErr
		}

		// Check if this volume should be updated
		if vol.Config.StorageClass == scName &&
			vol.Config.RequestedAutogrowPolicy == "" &&
			vol.EffectiveAGPolicy.PolicyName != effectiveAGPolicy.PolicyName {

			// Phase 2: Write lock only if update is needed (expensive, exclusive)
			var updateResults []db.Result
			var updateUnlocker func()
			var updateLockErr error

			if isSubordinate {
				updateResults, updateUnlocker, updateLockErr = db.Lock(ctx,
					db.Query(db.UpsertSubordinateVolume(volume.Config.Name, volume.Config.ShareSourceVolume)))
			} else {
				updateResults, updateUnlocker, updateLockErr = db.Lock(ctx,
					db.Query(db.UpsertVolume(volume.Config.Name, "")))
			}

			if updateLockErr != nil {
				Logc(ctx).WithFields(LogFields{
					"volume":       volume.Config.Name,
					"storageClass": scName,
				}).WithError(updateLockErr).Warn("Failed to lock volume for autogrow policy update.")
				updateUnlocker()
				continue
			}

			// Re-read after write lock
			var updateVol *storage.Volume
			var upserter func(*storage.Volume)

			if isSubordinate {
				updateVol = updateResults[0].SubordinateVolume.Read
				upserter = updateResults[0].SubordinateVolume.Upsert
			} else {
				updateVol = updateResults[0].Volume.Read
				upserter = updateResults[0].Volume.Upsert
			}
			if updateVol == nil {
				updateUnlocker()
				continue
			}

			// Re-resolve effective policy with fresh data (protects against changes between locks)
			newEffectiveAGPolicy, newPolicyErr := o.resolveEffectiveAutogrowPolicy(ctx, updateVol.Config)

			// Skip if policy resolution failed
			if newPolicyErr != nil {
				policyErr = newPolicyErr
			}

			// Skip if volume state changed between locks
			if updateVol.Config.StorageClass != scName {
				updateUnlocker()
				continue
			}
			if updateVol.Config.RequestedAutogrowPolicy != "" {
				updateUnlocker()
				continue
			}
			if updateVol.EffectiveAGPolicy.PolicyName == newEffectiveAGPolicy.PolicyName {
				updateUnlocker()
				continue
			}

			// Update the volume
			oldPolicyName := updateVol.EffectiveAGPolicy.PolicyName
			updateVol.EffectiveAGPolicy = newEffectiveAGPolicy

			Logc(ctx).WithFields(LogFields{
				"volumeName":            updateVol.Config.Name,
				"oldAutogrowPolicyName": oldPolicyName,
				"newAutogrowPolicyName": newEffectiveAGPolicy.PolicyName,
			}).Debug("Updated volume effective Autogrow policy due to StorageClass change.")

			upserter(updateVol)
			updateUnlocker()
		}
	}

	Logc(ctx).WithFields(LogFields{
		"storageClass": scName,
	}).Info("Completed updating volumes for StorageClass Autogrow policy change.")

	return policyErr
}

// resolveAndSetEffectiveAutogrowPolicy resolves the effective Autogrow policy for a volume,
// logs any errors appropriately, and sets it on the volume.
// Returns the policy info and any error for the caller to handle if needed.
func (o *ConcurrentTridentOrchestrator) resolveAndSetEffectiveAutogrowPolicy(
	ctx context.Context, vol *storage.Volume, operationContext string, logLevel string,
) (models.EffectiveAutogrowPolicyInfo, error) {
	// Resolve effective Autogrow policy
	effectiveAGPolicy, policyErr := o.resolveEffectiveAutogrowPolicy(ctx, vol.Config)

	// Log any resolution errors
	if policyErr != nil {
		logFields := LogFields{}
		// Use appropriate field name based on whether the volume is subordinate
		if vol.IsSubordinate() {
			logFields["subordinateVolume"] = vol.Config.Name
		} else {
			logFields["volume"] = vol.Config.Name
		}

		var msg string
		if errors.IsAutogrowPolicyNotFoundError(policyErr) {
			msg = fmt.Sprintf("Referenced Autogrow policy not found during %s.", operationContext)
		} else if errors.IsAutogrowPolicyNotUsableError(policyErr) {
			msg = fmt.Sprintf("Referenced Autogrow policy not usable during %s.", operationContext)
		}

		if msg != "" {
			switch logLevel {
			case "debug":
				Logc(ctx).WithFields(logFields).Debug(msg)
			case "warn":
				Logc(ctx).WithFields(logFields).Warn(msg)
			}
		}
	}

	// Always set effective Autogrow policy even if error occurred
	vol.EffectiveAGPolicy = effectiveAGPolicy

	return effectiveAGPolicy, policyErr
}
