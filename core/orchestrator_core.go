// Copyright 2022 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/multierr"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core/cache"
	"github.com/netapp/trident/frontend"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	. "github.com/netapp/trident/logging"
	persistentstore "github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/factory"
	sa "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/utils"
)

const (
	NodeAccessReconcilePeriod      = time.Second * 30
	NodeRegistrationCooldownPeriod = time.Second * 30
	AttachISCSIVolumeTimeoutLong   = time.Second * 90
)

// recordTiming is used to record in Prometheus the total time taken for an operation as follows:
//
//	defer recordTiming("backend_add")()
//
// see also: https://play.golang.org/p/6xRXlhFdqBd
func recordTiming(operation string, err *error) func() {
	startTime := time.Now()
	return func() {
		endTime := time.Since(startTime)
		endTimeMS := float64(endTime.Milliseconds())
		success := "true"
		if *err != nil {
			success = "false"
		}
		operationDurationInMsSummary.WithLabelValues(operation, success).Observe(endTimeMS)
	}
}

func recordTransactionTiming(txn *storage.VolumeTransaction, err *error) {
	if txn == nil || txn.VolumeCreatingConfig == nil {
		// for unit tests, there will be no txn to record
		return
	}

	operation := "transaction_volume_finish"

	startTime := txn.VolumeCreatingConfig.StartTime
	endTime := time.Since(startTime)
	endTimeMS := float64(endTime.Milliseconds())
	success := "true"
	if *err != nil {
		success = "false"
	}
	operationDurationInMsSummary.WithLabelValues(operation, success).Observe(endTimeMS)
}

type TridentOrchestrator struct {
	backends                 map[string]storage.Backend // key is UUID, not name
	volumes                  map[string]*storage.Volume
	subordinateVolumes       map[string]*storage.Volume
	frontends                map[string]frontend.Plugin
	mutex                    *sync.Mutex
	storageClasses           map[string]*storageclass.StorageClass
	nodes                    cache.NodeCache
	volumePublications       *cache.VolumePublicationCache
	snapshots                map[string]*storage.Snapshot
	storeClient              persistentstore.Client
	bootstrapped             bool
	bootstrapError           error
	txnMonitorTicker         *time.Ticker
	txnMonitorChannel        chan struct{}
	txnMonitorStopped        bool
	lastNodeRegistration     time.Time
	volumePublicationsSynced bool
	stopNodeAccessLoop       chan bool
	stopReconcileBackendLoop chan bool
	uuid                     string
}

// NewTridentOrchestrator returns a storage orchestrator instance
func NewTridentOrchestrator(client persistentstore.Client) *TridentOrchestrator {
	return &TridentOrchestrator{
		backends:           make(map[string]storage.Backend), // key is UUID, not name
		volumes:            make(map[string]*storage.Volume),
		subordinateVolumes: make(map[string]*storage.Volume),
		frontends:          make(map[string]frontend.Plugin),
		storageClasses:     make(map[string]*storageclass.StorageClass),
		nodes:              *cache.NewNodeCache(),
		volumePublications: cache.NewVolumePublicationCache(),
		snapshots:          make(map[string]*storage.Snapshot), // key is ID, not name
		mutex:              &sync.Mutex{},
		storeClient:        client,
		bootstrapped:       false,
		bootstrapError:     utils.NotReadyError(),
	}
}

func (o *TridentOrchestrator) transformPersistentState(ctx context.Context) error {
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
		// TODO: transform Trident API objects
	}

	// Store the persistent store and API versions.
	version.PersistentStoreVersion = string(o.storeClient.GetType())
	version.OrchestratorAPIVersion = config.OrchestratorAPIVersion
	if err = o.storeClient.SetVersion(ctx, version); err != nil {
		return fmt.Errorf("failed to set the persistent state version after migration: %v", err)
	}

	// TODO (websterj): Revisit or remove this assignment completely once Trident v21.10.1 has reached EOL.
	Logc(ctx).WithField(
		"PublicationsSynced", version.PublicationsSynced,
	).Debug("Adding current Trident Version CR state to Trident core.")
	o.volumePublicationsSynced = version.PublicationsSynced
	return nil
}

func (o *TridentOrchestrator) Bootstrap(monitorTransactions bool) error {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowCoreBootstrap, LogLayerCore)
	var err error

	if len(o.frontends) == 0 {
		Logc(ctx).Warning("Trident is bootstrapping with no frontend.")
	}

	// Transform persistent state, if necessary
	if err = o.transformPersistentState(ctx); err != nil {
		o.bootstrapError = utils.BootstrapError(err)
		return o.bootstrapError
	}

	// Set UUID
	o.uuid, err = o.storeClient.GetTridentUUID(ctx)
	if err != nil {
		o.bootstrapError = utils.BootstrapError(err)
		return o.bootstrapError
	}

	// Bootstrap state from persistent store
	if err = o.bootstrap(ctx); err != nil {
		o.bootstrapError = utils.BootstrapError(err)
		return o.bootstrapError
	}

	if monitorTransactions {
		// Start transaction monitor
		o.StartTransactionMonitor(ctx, txnMonitorPeriod, txnMonitorMaxAge)
	}

	o.bootstrapped = true
	o.bootstrapError = nil
	Logc(ctx).Infof("%s bootstrapped successfully.", utils.Title(config.OrchestratorName))
	return nil
}

func (o *TridentOrchestrator) bootstrapBackends(ctx context.Context) error {
	persistentBackends, err := o.storeClient.GetBackends(ctx)
	if err != nil {
		return err
	}

	for _, b := range persistentBackends {

		Logc(ctx).WithFields(LogFields{
			"persistentBackend.Name":        b.Name,
			"persistentBackend.BackendUUID": b.BackendUUID,
			"persistentBackend.online":      b.Online,
			"persistentBackend.state":       b.State,
			"persistentBackend.configRef":   b.ConfigRef,
			"handler":                       "Bootstrap",
		}).Debug("Processing backend.")

		// TODO:  If the API evolves, check the Version field here.
		serializedConfig, err := b.MarshalConfig()
		if err != nil {
			return err
		}

		newBackendExternal, backendErr := o.addBackend(ctx, serializedConfig, b.BackendUUID, b.ConfigRef)
		if backendErr == nil {
			newBackendExternal.BackendUUID = b.BackendUUID
		} else {

			errorLogFields := LogFields{
				"handler":            "Bootstrap",
				"newBackendExternal": newBackendExternal,
				"backendErr":         backendErr.Error(),
			}

			// Trident for Docker supports one backend at a time, and the Docker volume plugin
			// should not start if the backend fails to initialize, so return any error here.
			if config.CurrentDriverContext == config.ContextDocker {
				Logc(ctx).WithFields(errorLogFields).Error("Problem adding backend.")
				return backendErr
			}

			Logc(ctx).WithFields(errorLogFields).Warn("Problem adding backend.")

			if newBackendExternal != nil {
				newBackend, _ := o.validateAndCreateBackendFromConfig(ctx, serializedConfig, b.ConfigRef, b.BackendUUID)
				newBackend.SetName(b.Name)
				newBackendExternal.Name = b.Name // have to set it explicitly, so it's not ""
				o.backends[newBackendExternal.BackendUUID] = newBackend

				Logc(ctx).WithFields(LogFields{
					"newBackendExternal":             newBackendExternal,
					"newBackendExternal.Name":        newBackendExternal.Name,
					"newBackendExternal.State":       newBackendExternal.State.String(),
					"newBackendExternal.BackendUUID": newBackendExternal.BackendUUID,
					"persistentBackend.Name":         b.Name,
					"persistentBackend.State":        b.State.String(),
					"persistentBackend.BackendUUID":  b.BackendUUID,
				}).Debug("Backend information.")
			}
		}

		// Note that addBackend returns an external copy of the newly
		// added backend, so we have to go fetch it manually.
		newBackend, found := o.backends[b.BackendUUID]
		if found {
			newBackend.SetOnline(b.Online)
			if backendErr != nil {
				newBackend.SetState(storage.Failed)
			} else {
				if b.State == storage.Deleting {
					newBackend.SetState(storage.Deleting)
				}
			}
			Logc(ctx).WithFields(LogFields{
				"backend":                        newBackend.Name(),
				"backendUUID":                    newBackend.BackendUUID(),
				"configRef":                      newBackend.ConfigRef(),
				"persistentBackends.BackendUUID": b.BackendUUID,
				"online":                         newBackend.Online(),
				"state":                          newBackend.State(),
				"handler":                        "Bootstrap",
			}).Trace("Added an existing backend.")
		} else {
			Logc(ctx).WithFields(LogFields{
				"b.BackendUUID": b.BackendUUID,
			}).Warn("Could not find backend.")
			continue
		}
	}
	return nil
}

func (o *TridentOrchestrator) bootstrapStorageClasses(ctx context.Context) error {
	persistentStorageClasses, err := o.storeClient.GetStorageClasses(ctx)
	if err != nil {
		return err
	}
	for _, psc := range persistentStorageClasses {
		// TODO:  If the API evolves, check the Version field here.
		sc := storageclass.NewFromPersistent(psc)
		Logc(ctx).WithFields(LogFields{
			"storageClass": sc.GetName(),
			"handler":      "Bootstrap",
		}).Info("Added an existing storage class.")
		o.storageClasses[sc.GetName()] = sc
		for _, b := range o.backends {
			sc.CheckAndAddBackend(ctx, b)
		}
	}
	return nil
}

// Updates the o.volumes cache with the latest backend data. This function should only edit o.volumes in place to avoid
// briefly losing track of volumes that do exist.
func (o *TridentOrchestrator) bootstrapVolumes(ctx context.Context) error {
	volumes, err := o.storeClient.GetVolumes(ctx)
	if err != nil {
		return err
	}

	// Remove extra volumes in list
	volNames := make([]string, 0)
	for _, v := range volumes {
		volNames = append(volNames, v.Config.Name)
	}
	for k := range o.volumes {
		if !utils.SliceContainsString(volNames, k) {
			delete(o.volumes, k)
		}
	}
	volCount := 0
	for _, v := range volumes {
		// TODO:  If the API evolves, check the Version field here.
		var backend storage.Backend
		var ok bool
		vol := storage.NewVolume(v.Config, v.BackendUUID, v.Pool, v.Orphaned, v.State)
		if vol.IsSubordinate() {
			o.subordinateVolumes[vol.Config.Name] = vol
		} else {
			o.volumes[vol.Config.Name] = vol

			if backend, ok = o.backends[v.BackendUUID]; !ok {
				Logc(ctx).WithFields(LogFields{
					"volume":      v.Config.Name,
					"backendUUID": v.BackendUUID,
				}).Warning("Couldn't find backend. Setting state to MissingBackend.")
				vol.State = storage.VolumeStateMissingBackend
			} else {
				backend.Volumes()[vol.Config.Name] = vol
				if fakeDriver, ok := backend.Driver().(*fake.StorageDriver); ok {
					fakeDriver.BootstrapVolume(ctx, vol)
				}
			}
		}

		Logc(ctx).WithFields(LogFields{
			"volume":       vol.Config.Name,
			"internalName": vol.Config.InternalName,
			"size":         vol.Config.Size,
			"backendUUID":  vol.BackendUUID,
			"pool":         vol.Pool,
			"orphaned":     vol.Orphaned,
			"state":        vol.State,
			"handler":      "Bootstrap",
		}).Trace("Added an existing volume.")
		volCount++
	}
	Logc(ctx).Infof("Added %v existing volume(s)", volCount)
	return nil
}

func (o *TridentOrchestrator) bootstrapSnapshots(ctx context.Context) error {
	snapshots, err := o.storeClient.GetSnapshots(ctx)
	if err != nil {
		return err
	}
	for _, s := range snapshots {
		// TODO:  If the API evolves, check the Version field here.
		snapshot := storage.NewSnapshot(s.Config, s.Created, s.SizeBytes, s.State)
		o.snapshots[snapshot.ID()] = snapshot
		volume, ok := o.volumes[s.Config.VolumeName]
		if !ok {
			Logc(ctx).Warnf("Couldn't find volume %s for snapshot %s. Setting snapshot state to MissingVolume.",
				s.Config.VolumeName, s.Config.Name)
			snapshot.State = storage.SnapshotStateMissingVolume
		} else {
			backend, ok := o.backends[volume.BackendUUID]
			if !ok {
				snapshot.State = storage.SnapshotStateMissingBackend
			} else {
				if fakeDriver, ok := backend.Driver().(*fake.StorageDriver); ok {
					fakeDriver.BootstrapSnapshot(ctx, snapshot, volume.Config)
				}
			}
		}

		Logc(ctx).WithFields(LogFields{
			"snapshot": snapshot.Config.Name,
			"volume":   snapshot.Config.VolumeName,
			"handler":  "Bootstrap",
		}).Info("Added an existing snapshot.")
	}
	return nil
}

func (o *TridentOrchestrator) bootstrapVolTxns(ctx context.Context) error {
	volTxns, err := o.storeClient.GetVolumeTransactions(ctx)
	if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		Logc(ctx).Warnf("Couldn't retrieve volume transaction logs: %s", err.Error())
	}
	for _, v := range volTxns {
		o.mutex.Lock()
		err = o.handleFailedTransaction(ctx, v)
		o.mutex.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *TridentOrchestrator) bootstrapNodes(ctx context.Context) error {
	// Don't bootstrap nodes if we're not CSI
	if config.CurrentDriverContext != config.ContextCSI {
		return nil
	}

	nodes, err := o.storeClient.GetNodes(ctx)
	if err != nil {
		return err
	}
	for _, n := range nodes {
		Logc(ctx).WithFields(LogFields{
			"node":    n.Name,
			"handler": "Bootstrap",
		}).Info("Added an existing node.")
		o.nodes.Set(n.Name, n)
	}
	err = o.reconcileNodeAccessOnAllBackends(ctx)
	if err != nil {
		Logc(ctx).Warningf("%v", err)
	}
	return nil
}

func (o *TridentOrchestrator) bootstrapVolumePublications(ctx context.Context) error {
	// Don't bootstrap volume publications if we're not CSI
	if config.CurrentDriverContext != config.ContextCSI {
		return nil
	}

	volumePublications, err := o.storeClient.GetVolumePublications(ctx)
	if err != nil {
		return err
	}
	for _, vp := range volumePublications {
		fields := LogFields{
			"volume":  vp.VolumeName,
			"node":    vp.NodeName,
			"handler": "Bootstrap",
		}

		Logc(ctx).WithFields(fields).Debug("Added an existing volume publication.")
		err = o.volumePublications.Set(vp.VolumeName, vp.NodeName, vp)
		if err != nil {
			bootstrapErr := fmt.Errorf("unable to bootstrap volume publication %s; %v", vp.Name, err)
			err = multierr.Append(err, bootstrapErr)
			Logc(ctx).WithFields(fields).WithError(bootstrapErr).Error("Unable to add an existing volume publication.")
		}
	}

	return err
}

// bootstrapSubordinateVolumes updates the source volumes to point to their subordinates.  Updating the
// super->sub references here allows us to persist only the scalar sub->super references.
func (o *TridentOrchestrator) bootstrapSubordinateVolumes(ctx context.Context) error {
	for volumeName, volume := range o.subordinateVolumes {

		// Get the source volume
		sourceVolume, ok := o.volumes[volume.Config.ShareSourceVolume]
		if !ok {
			Logc(ctx).WithFields(LogFields{
				"subordinateVolume": volumeName,
				"sourceVolume":      volume.Config.ShareSourceVolume,
				"handler":           "Bootstrap",
			}).Warning("Source volume for subordinate volume not found.")

			continue
		}

		// Make the source volume point to the subordinate
		if sourceVolume.Config.SubordinateVolumes == nil {
			sourceVolume.Config.SubordinateVolumes = make(map[string]interface{})
		}
		sourceVolume.Config.SubordinateVolumes[volumeName] = nil
	}

	return nil
}

func (o *TridentOrchestrator) bootstrap(ctx context.Context) error {
	// Fetching backend information

	type bootstrapFunc func(context.Context) error
	for _, f := range []bootstrapFunc{
		o.bootstrapBackends,
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
					return utils.TypeAssertionError("err.(*persistentstore.Error)")
				}
				Logc(ctx).Warnf("Unable to find key %s.  Continuing bootstrap, but "+
					"consider checking integrity if Trident installation is not new.", keyError.Key)
			} else {
				return err
			}
		}
	}

	// Clean up any offline backends that lack volumes.  This can happen if
	// a connection to the store fails when attempting to delete a backend.
	for backendUUID, backend := range o.backends {
		if backend.State().IsFailed() {
			continue
		}

		if !backend.HasVolumes() && (backend.State().IsDeleting() || o.storeClient.IsBackendDeleting(ctx, backend)) {
			backend.Terminate(ctx)
			delete(o.backends, backendUUID)
			err := o.storeClient.DeleteBackend(ctx, backend)
			if err != nil {
				return fmt.Errorf("failed to delete empty offline/deleting backend %s: %v", backendUUID, err)
			}
		}
	}

	// Periodically get new logging config from the controller, and update the node server's from it.

	// If nothing failed during bootstrapping, initialize the core metrics
	o.updateMetrics()

	return nil
}

// Stop stops the orchestrator core.
func (o *TridentOrchestrator) Stop() {
	// Stop the node access and backends' state reconciliation background tasks
	if o.stopNodeAccessLoop != nil {
		o.stopNodeAccessLoop <- true
	}
	if o.stopReconcileBackendLoop != nil {
		o.stopReconcileBackendLoop <- true
	}

	// Stop transaction monitor
	o.StopTransactionMonitor()
}

// updateMetrics updates the metrics that track the core objects.
// The caller should hold the orchestrator lock.
func (o *TridentOrchestrator) updateMetrics() {
	tridentBuildInfo.WithLabelValues(config.BuildHash,
		config.OrchestratorVersion.ShortString(),
		config.BuildType).Set(float64(1))

	backendsGauge.Reset()
	tridentBackendInfo.Reset()
	for _, backend := range o.backends {
		if backend == nil {
			continue
		}
		backendsGauge.WithLabelValues(backend.GetDriverName(), backend.State().String()).Inc()
		tridentBackendInfo.WithLabelValues(backend.GetDriverName(), backend.Name(),
			backend.BackendUUID()).Set(float64(1))
	}

	volumesGauge.Reset()
	volumesTotalBytes := float64(0)
	volumeAllocatedBytesGauge.Reset()
	for _, volume := range o.volumes {
		if volume == nil {
			continue
		}
		bytes, _ := strconv.ParseFloat(volume.Config.Size, 64)
		volumesTotalBytes += bytes
		if backend, err := o.getBackendByBackendUUID(volume.BackendUUID); err == nil {
			driverName := backend.GetDriverName()
			volumesGauge.WithLabelValues(driverName,
				volume.BackendUUID,
				string(volume.State),
				string(volume.Config.VolumeMode)).Inc()

			volumeAllocatedBytesGauge.WithLabelValues(driverName, backend.BackendUUID(), string(volume.State),
				string(volume.Config.VolumeMode)).Add(bytes)
		}
	}
	volumesTotalBytesGauge.Set(volumesTotalBytes)

	scGauge.Set(float64(len(o.storageClasses)))
	nodeGauge.Set(float64(o.nodes.Len()))
	snapshotGauge.Reset()
	snapshotAllocatedBytesGauge.Reset()
	for _, snapshot := range o.snapshots {
		vol := o.volumes[snapshot.Config.VolumeName]
		if vol != nil {
			if backend, err := o.getBackendByBackendUUID(vol.BackendUUID); err == nil {
				driverName := backend.GetDriverName()
				snapshotGauge.WithLabelValues(
					driverName,
					vol.BackendUUID).Inc()
				snapshotAllocatedBytesGauge.WithLabelValues(driverName, backend.BackendUUID()).
					Add(float64(snapshot.SizeBytes))
			}
		}
	}
}

func (o *TridentOrchestrator) handleFailedTransaction(ctx context.Context, v *storage.VolumeTransaction) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

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
	case storage.UpgradeVolume:
		Logc(ctx).WithFields(LogFields{
			"volume": v.Config.Name,
			"PVC":    v.PVUpgradeConfig.PVCConfig.Name,
			"PV":     v.PVUpgradeConfig.PVConfig.Name,
			"op":     v.Op,
		}).Info("Processed volume upgrade transaction log.")
	case storage.VolumeCreating:
		Logc(ctx).WithFields(LogFields{
			"volume":      v.VolumeCreatingConfig.Name,
			"backendUUID": v.VolumeCreatingConfig.BackendUUID,
			"op":          v.Op,
		}).Info("Processed volume creating transaction log.")
	}

	switch v.Op {
	case storage.AddVolume:
		// Regardless of whether the transaction succeeded or not, we need
		// to roll it back.  There are three possible states:
		// 1) Volume transaction created only
		// 2) Volume created on backend
		// 3) Volume created in the store.
		if _, ok := o.volumes[v.Config.Name]; ok {
			// If the volume was added to the store, we will have loaded the
			// volume into memory, and we can just delete it normally.
			// Handles case 3)
			err := o.deleteVolume(ctx, v.Config.Name)
			if err != nil {
				return fmt.Errorf("unable to clean up volume %s: %v", v.Config.Name, err)
			}
		} else {
			// If the volume wasn't added into the store, we attempt to delete
			// it at each backend, since we don't know where it might have
			// landed.  We're guaranteed that the volume name will be
			// unique across backends, thanks to the StoragePrefix field,
			// so this should be idempotent.
			// Handles case 2)
			for _, backend := range o.backends {
				// Backend offlining is serialized with volume creation,
				// so we can safely skip offline backends.
				if !backend.State().IsOnline() && !backend.State().IsDeleting() {
					continue
				}
				// Volume deletion is an idempotent operation, so it's safe to
				// delete an already deleted volume.

				if err := backend.RemoveVolume(ctx, v.Config); err != nil {
					return fmt.Errorf("error attempting to clean up volume %s from backend %s: %v", v.Config.Name,
						backend.Name(), err)
				}
			}
		}
		// Finally, we need to clean up the volume transaction.
		// Necessary for all cases.
		if err := o.DeleteVolumeTransaction(ctx, v); err != nil {
			return fmt.Errorf("failed to clean up volume addition transaction: %v", err)
		}

	case storage.DeleteVolume:
		// Because we remove the volume from persistent store after we remove
		// it from the backend, we need to take any special measures only when
		// the volume is still in the persistent store. In this case, the
		// volume should have been loaded into memory when we bootstrapped.
		if _, ok := o.volumes[v.Config.Name]; ok {

			err := o.deleteVolume(ctx, v.Config.Name)
			if err != nil {
				Logc(ctx).WithFields(LogFields{
					"volume": v.Config.Name,
					"error":  err,
				}).Errorf("Unable to finalize deletion of the volume! Repeat deleting the volume using %s.",
					config.OrchestratorClientName)
			}
		} else {
			Logc(ctx).WithFields(LogFields{
				"volume": v.Config.Name,
			}).Info("Volume for the delete transaction wasn't found.")
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
		if _, ok := o.snapshots[v.SnapshotConfig.ID()]; ok {
			// If the snapshot was added to the store, we will have loaded the
			// snapshot into memory, and we can just delete it normally.
			// Handles case 3)
			if err := o.deleteSnapshot(ctx, v.SnapshotConfig); err != nil {
				return fmt.Errorf("unable to clean up snapshot %s: %v", v.SnapshotConfig.Name, err)
			}
		} else {
			// If the snapshot wasn't added into the store, we attempt to delete
			// it at each backend, since we don't know where it might have landed.
			// We're guaranteed that the volume name will be unique across backends,
			// thanks to the StoragePrefix field, so this should be idempotent.
			// Handles case 2)
			for _, backend := range o.backends {
				// Skip backends that aren't ready to accept a snapshot delete operation
				if !backend.State().IsOnline() && !backend.State().IsDeleting() {
					continue
				}
				// Snapshot deletion is an idempotent operation, so it's safe to
				// delete an already deleted snapshot.
				err := backend.DeleteSnapshot(ctx, v.SnapshotConfig, v.Config)
				if err != nil && !utils.IsUnsupportedError(err) {
					return fmt.Errorf("error attempting to clean up snapshot %s from backend %s: %v",
						v.SnapshotConfig.Name, backend.Name(), err)
				}
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

		if err := o.deleteSnapshot(ctx, v.SnapshotConfig); err != nil {
			if utils.IsNotFoundError(err) {
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
		vol, ok := o.volumes[v.Config.Name]
		if ok {
			err = o.resizeVolume(ctx, vol, v.Config.Size)
			if err != nil {
				Logc(ctx).WithFields(LogFields{
					"volume": v.Config.Name,
					"error":  err,
				}).Error("Unable to resize the volume! Repeat resizing the volume.")
			} else {
				Logc(ctx).WithFields(LogFields{
					"volume":      vol.Config.Name,
					"volume_size": v.Config.Size,
				}).Info("Orchestrator resized the volume on the storage backend.")
			}
		} else {
			Logc(ctx).WithFields(LogFields{
				"volume": v.Config.Name,
			}).Error("Volume for the resize transaction wasn't found.")
		}
		return o.resizeVolumeCleanup(ctx, err, vol, v)

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
			The volume import process sets the reclaim policy to "retain" by default,
			for legacy imports. In the case where notManaged is true then the volume is not renamed,
			and in the legacy import case it is also not persisted.
		*/

		if volume, ok := o.volumes[v.Config.Name]; ok {
			if err := o.storeClient.DeleteVolume(ctx, volume); err != nil {
				return err
			}
			delete(o.volumes, v.Config.Name)
		}
		if !v.Config.ImportNotManaged {
			if err := o.resetImportedVolumeName(ctx, v.Config); err != nil {
				return err
			}
		}

		// Finally, we need to clean up the volume transactions
		if err := o.DeleteVolumeTransaction(ctx, v); err != nil {
			return fmt.Errorf("failed to clean up volume addition transaction: %v", err)
		}

	case storage.UpgradeVolume, storage.VolumeCreating:
		// Do nothing
	}

	return nil
}

func (o *TridentOrchestrator) resetImportedVolumeName(ctx context.Context, volume *storage.VolumeConfig) error {
	// The volume could be renamed (notManaged = false) without being persisted.
	// If the volume wasn't added to the persistent store, we attempt to rename
	// it at each backend, since we don't know where it might have
	// landed.  We're guaranteed that the volume name will be
	// unique across backends, thanks to the StoragePrefix field,
	// so this should be idempotent.
	for _, backend := range o.backends {
		if err := backend.RenameVolume(ctx, volume, volume.ImportOriginalName); err == nil {
			return nil
		}
	}
	Logc(ctx).Debugf("could not find volume %s to reset the volume name", volume.InternalName)
	return nil
}

func (o *TridentOrchestrator) AddFrontend(ctx context.Context, f frontend.Plugin) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	name := f.GetName()
	if _, ok := o.frontends[name]; ok {
		Logc(ctx).WithField("name", name).Warn("Adding frontend already present.")
		return
	}
	Logc(ctx).WithField("name", name).Info("Added frontend.")
	o.frontends[name] = f
}

func (o *TridentOrchestrator) GetFrontend(ctx context.Context, name string) (frontend.Plugin, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if fe, ok := o.frontends[name]; !ok {
		err := fmt.Errorf("requested frontend %s does not exist", name)
		return nil, err
	} else {
		Logc(ctx).WithField("name", name).Debug("Found requested frontend.")
		return fe, nil
	}
}

func (o *TridentOrchestrator) validateBackendUpdate(oldBackend, newBackend storage.Backend) error {
	// Validate that backend type isn't being changed as backend type has
	// implications for the internal volume names.
	if oldBackend.GetDriverName() != newBackend.GetDriverName() {
		return fmt.Errorf(
			"cannot update the backend as the old backend is of type %s and the new backend is of type %s",
			oldBackend.GetDriverName(), newBackend.GetDriverName())
	}

	return nil
}

func (o *TridentOrchestrator) GetVersion(_ context.Context) (string, error) {
	return config.OrchestratorVersion.String(), o.bootstrapError
}

// AddBackend handles creation of a new storage backend
func (o *TridentOrchestrator) AddBackend(
	ctx context.Context, configJSON, configRef string,
) (backendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_add", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	backend, err := o.addBackend(ctx, configJSON, uuid.New().String(), configRef)
	if err != nil {
		return backend, err
	}

	b, err := o.getBackendByBackendUUID(backend.BackendUUID)
	if err != nil {
		return backend, err
	}
	err = o.reconcileNodeAccessOnBackend(ctx, b)
	if err != nil {
		return backend, err
	}

	return backend, nil
}

// addBackend creates a new storage backend. It assumes the mutex lock is
// already held or not required (e.g., during bootstrapping).
func (o *TridentOrchestrator) addBackend(
	ctx context.Context, configJSON, backendUUID, configRef string,
) (backendExternal *storage.BackendExternal, err error) {
	var (
		newBackend = true
		backend    storage.Backend
	)

	defer func() {
		if backend != nil {
			if err != nil || !newBackend {
				backend.Terminate(ctx)
			}
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
			return backend.ConstructExternal(ctx), err
		}
		return nil, err
	}

	// can we find this backend by UUID? (if so, it's an update)
	foundBackend := o.backends[backend.BackendUUID()]
	if foundBackend != nil {
		// Let the updateBackend method handle an existing backend
		newBackend = false
		return o.updateBackend(ctx, backend.Name(), configJSON, configRef)
	}

	// can we find this backend by name instead of UUID? (if so, it's also an update)
	foundBackend, _ = o.getBackendByBackendName(backend.Name())
	if foundBackend != nil {
		// Let the updateBackend method handle an existing backend
		newBackend = false
		return o.updateBackend(ctx, backend.Name(), configJSON, configRef)
	}

	if configRef != "" {
		// can we find this backend by configRef (if so, then something is wrong)
		foundBackend, _ := o.getBackendByConfigRef(configRef)
		if foundBackend != nil {
			// IDEALLY WE SHOULD NOT BE HERE:
			// If we are here it means that there already exists a backend with the
			// given configRef but the backendName and backendUUID of that backend
			// do not match backend.Name() or the backend.BackendUUID(), this can only
			// happen with a newly created tbc which failed to updated status
			// on success and had a name change in the next reconcile loop.
			Logc(ctx).WithFields(LogFields{
				"backendName":    foundBackend.Name(),
				"backendUUID":    foundBackend.BackendUUID(),
				"newBackendName": backend.Name(),
				"newbackendUUID": backend.BackendUUID(),
			}).Debug("Backend found by configRef.")

			// Let the updateBackend method handle an existing backend
			newBackend = false
			return o.updateBackend(ctx, foundBackend.Name(), configJSON, configRef)
		}
	}

	// not found by name OR by UUID, we're adding a new backend
	Logc(ctx).WithFields(LogFields{
		"backend":             backend.Name(),
		"backend.BackendUUID": backend.BackendUUID(),
		"backend.ConfigRef":   backend.ConfigRef(),
	}).Debug("Adding a new backend.")
	if err = o.updateBackendOnPersistentStore(ctx, backend, true); err != nil {
		return nil, err
	}
	o.backends[backend.BackendUUID()] = backend

	// Update storage class information
	classes := make([]string, 0, len(o.storageClasses))
	for _, sc := range o.storageClasses {
		if added := sc.CheckAndAddBackend(ctx, backend); added > 0 {
			classes = append(classes, sc.GetName())
		}
	}
	if len(classes) == 0 {
		Logc(ctx).WithFields(LogFields{
			"backend": backend.Name(),
		}).Info("Newly added backend satisfies no storage classes.")
	} else {
		Logc(ctx).WithFields(LogFields{
			"backend": backend.Name(),
		}).Infof("Newly added backend satisfies storage classes %s.", strings.Join(classes, ", "))
	}

	return backend.ConstructExternal(ctx), nil
}

// validateAndCreateBackendFromConfig validates config and creates backend based on Config
func (o *TridentOrchestrator) validateAndCreateBackendFromConfig(
	ctx context.Context, configJSON, configRef, backendUUID string,
) (backendExternal storage.Backend, err error) {
	var backendSecret map[string]string

	commonConfig, configInJSON, err := factory.ValidateCommonSettings(ctx, configJSON)
	if err != nil {
		return nil, err
	}

	// For backends created using CRD Controller ensure there are no forbidden fields
	if o.isCRDContext(ctx) {
		if err = factory.SpecOnlyValidation(ctx, commonConfig, configInJSON); err != nil {
			return nil, utils.UnsupportedConfigError(err)
		}
	}

	// If Credentials are set, fetch them and set them in the configJSON matching field names
	if len(commonConfig.Credentials) != 0 {
		secretName, _, err := commonConfig.GetCredentials()
		if err != nil {
			return nil, err
		} else if secretName == "" {
			return nil, fmt.Errorf("credentials `name` field cannot be empty")
		}

		if backendSecret, err = o.storeClient.GetBackendSecret(ctx, secretName); err != nil {
			return nil, err
		} else if backendSecret == nil {
			return nil, fmt.Errorf("backend credentials not found")
		}
	}

	return factory.NewStorageBackendForConfig(ctx, configInJSON, configRef, backendUUID, commonConfig, backendSecret)
}

// UpdateBackend updates an existing backend.
func (o *TridentOrchestrator) UpdateBackend(
	ctx context.Context, backendName, configJSON, configRef string,
) (backendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_update", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	backend, err := o.updateBackend(ctx, backendName, configJSON, configRef)
	if err != nil {
		return backend, err
	}

	b, err := o.getBackendByBackendUUID(backend.BackendUUID)
	if err != nil {
		return backend, err
	}
	// Node access rules may have changed in the backend config
	b.InvalidateNodeAccess()
	err = o.reconcileNodeAccessOnBackend(ctx, b)
	if err != nil {
		return backend, err
	}

	return backend, nil
}

// updateBackend updates an existing backend. It assumes the mutex lock is already held.
func (o *TridentOrchestrator) updateBackend(
	ctx context.Context, backendName, configJSON, configRef string,
) (backendExternal *storage.BackendExternal, err error) {
	backendToUpdate, err := o.getBackendByBackendName(backendName)
	if err != nil {
		return nil, err
	}
	backendUUID := backendToUpdate.BackendUUID()

	return o.updateBackendByBackendUUID(ctx, backendName, configJSON, backendUUID, configRef)
}

// UpdateBackendByBackendUUID updates an existing backend.
func (o *TridentOrchestrator) UpdateBackendByBackendUUID(
	ctx context.Context, backendName, configJSON, backendUUID, configRef string,
) (backend *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_update", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	backend, err = o.updateBackendByBackendUUID(ctx, backendName, configJSON, backendUUID, configRef)
	if err != nil {
		return backend, err
	}

	b, err := o.getBackendByBackendUUID(backendUUID)
	if err != nil {
		return backend, err
	}
	// Node access rules may have changed in the backend config
	b.InvalidateNodeAccess()
	err = o.reconcileNodeAccessOnBackend(ctx, b)
	if err != nil {
		return backend, err
	}

	return backend, nil
}

// TODO combine this one and the one above
// updateBackendByBackendUUID updates an existing backend. It assumes the mutex lock is already held.
func (o *TridentOrchestrator) updateBackendByBackendUUID(
	ctx context.Context, backendName, configJSON, backendUUID, callingConfigRef string,
) (backendExternal *storage.BackendExternal, err error) {
	var backend storage.Backend

	// Check whether the backend exists.
	originalBackend, found := o.backends[backendUUID]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("backend %v was not found", backendUUID))
	}

	logFields := LogFields{"backendName": backendName, "backendUUID": backendUUID, "configJSON": "<suppressed>"}

	Logc(ctx).WithFields(LogFields{
		"originalBackend.Name":        originalBackend.Name(),
		"originalBackend.BackendUUID": originalBackend.BackendUUID(),
		"originalBackend.ConfigRef":   originalBackend.ConfigRef(),
		"GetExternalConfig":           originalBackend.Driver().GetExternalConfig(ctx),
	}).Debug("found original backend")

	originalConfigRef := originalBackend.ConfigRef()

	// Do not allow update of TridentBackendConfig-based backends using tridentctl
	if originalConfigRef != "" {
		if !o.isCRDContext(ctx) {
			Logc(ctx).WithFields(LogFields{
				"backendName": backendName,
				"backendUUID": backendUUID,
				"configRef":   originalConfigRef,
			}).Error("Cannot update backend created using TridentBackendConfig CR; please update the" +
				" TridentBackendConfig CR instead.")

			return nil, fmt.Errorf("cannot update backend '%v' created using TridentBackendConfig CR; please update"+
				" the TridentBackendConfig CR", backendName)
		}
	}

	// If originalBackend.ConfigRef happens to be empty, we should assign callingConfigRef to
	// originalBackend.ConfigRef as part of linking legacy tridentctl backend with TridentBackendConfig-based backend
	if originalConfigRef != callingConfigRef {
		ctxSource := ctx.Value(ContextKeyRequestSource)
		if originalConfigRef == "" && ctxSource != nil && ctxSource == ContextSourceCRD {
			Logc(ctx).WithFields(LogFields{
				"backendName":             originalBackend.Name(),
				"backendUUID":             originalBackend.BackendUUID(),
				"originalConfigRef":       originalConfigRef,
				"TridentBackendConfigUID": callingConfigRef,
			}).Tracef("Backend is not bound to any Trident Backend Config, attempting to bind it.")
		} else {
			Logc(ctx).WithFields(LogFields{
				"backendName":       originalBackend.Name(),
				"backendUUID":       originalBackend.BackendUUID(),
				"originalConfigRef": originalConfigRef,
				"invalidConfigRef":  callingConfigRef,
			}).Errorf("Backend update initiated using an invalid ConfigRef.")
			return nil, utils.UnsupportedConfigError(fmt.Errorf(
				"backend '%v' update initiated using an invalid configRef, it is associated with configRef "+
					"'%v' and not '%v'", originalBackend.Name(), originalConfigRef, callingConfigRef))
		}
	}

	defer func() {
		Logc(ctx).WithFields(logFields).Debug("<<<<<< updateBackendByBackendUUID")
		if backend != nil && err != nil {
			backend.Terminate(ctx)
		}
	}()

	Logc(ctx).WithFields(logFields).Debug(">>>>>> updateBackendByBackendUUID")

	// Second, validate the update.
	backend, err = o.validateAndCreateBackendFromConfig(ctx, configJSON, callingConfigRef, backendUUID)
	if err != nil {
		return nil, err
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
		err := errors.New("invalid backend update")
		Logc(ctx).WithField("error", err).Error("Backend update failed.")
		return nil, err
	case updateCode.Contains(storage.InvalidVolumeAccessInfoChange):
		err := errors.New("updating the data plane IP address isn't currently supported")
		Logc(ctx).WithField("error", err).Error("Backend update failed.")
		return nil, err
	case updateCode.Contains(storage.PrefixChange):
		err := utils.UnsupportedConfigError(errors.New("updating the storage prefix isn't currently supported"))
		Logc(ctx).WithField("error", err).Error("Backend update failed.")
		return nil, err
	case updateCode.Contains(storage.BackendRename):
		checkingBackend, lookupErr := o.getBackendByBackendName(backend.Name())
		if lookupErr == nil {
			// Don't rename if the name is already in use
			err := fmt.Errorf("backend name %v is already in use by %v", backend.Name(), checkingBackend.BackendUUID())
			Logc(ctx).WithField("error", err).Error("Backend update failed.")
			return nil, err
		}
		// getBackendByBackendName always returns NotFoundError if lookupErr is not nil
		// We couldn't find it, so it's not in use, let's rename
		if err := o.replaceBackendAndUpdateVolumesOnPersistentStore(ctx, originalBackend, backend); err != nil {
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

	// Update the backend state in memory
	delete(o.backends, originalBackend.BackendUUID())
	// the fake driver needs these copied forward
	if originalFakeDriver, ok := originalBackend.Driver().(*fake.StorageDriver); ok {
		Logc(ctx).Debug("Using fake driver, going to copy volumes forward...")
		if fakeDriver, ok := backend.Driver().(*fake.StorageDriver); ok {
			fakeDriver.CopyVolumes(originalFakeDriver.Volumes)
			Logc(ctx).Debug("Copied volumes forward.")
		}
	}
	originalBackend.Terminate(ctx)
	o.backends[backend.BackendUUID()] = backend

	// Update the volume state in memory
	// Identify orphaned volumes (i.e., volumes that are not present on the
	// new backend). Such a scenario can happen if a subset of volumes are
	// replicated for DR or volumes get deleted out of band. Operations on
	// such volumes are likely to fail, so here we just warn the users about
	// such volumes and mark them as orphaned. This is a best effort activity,
	// so it doesn't have to be part of the persistent store transaction.
	for volName, vol := range o.volumes {
		if vol.BackendUUID == originalBackend.BackendUUID() {
			vol.BackendUUID = backend.BackendUUID()
			updatePersistentStore := false
			volumeExists := backend.Driver().Get(ctx, vol.Config.InternalName) == nil
			if !volumeExists {
				if !vol.Orphaned {
					vol.Orphaned = true
					updatePersistentStore = true
					Logc(ctx).WithFields(LogFields{
						"volume":                  volName,
						"vol.Config.InternalName": vol.Config.InternalName,
						"backend":                 backend.Name(),
					}).Warn("Backend update resulted in an orphaned volume.")
				}
			} else {
				if vol.Orphaned {
					vol.Orphaned = false
					updatePersistentStore = true
					Logc(ctx).WithFields(LogFields{
						"volume":                  volName,
						"vol.Config.InternalName": vol.Config.InternalName,
						"backend":                 backend.Name(),
					}).Debug("The volume is no longer orphaned as a result of the backend update.")
				}
			}
			if updatePersistentStore {
				if err := o.updateVolumeOnPersistentStore(ctx, vol); err != nil {
					return nil, err
				}
			}
			o.backends[backend.BackendUUID()].Volumes()[volName] = vol
		}
	}

	// Update storage class information
	classes := make([]string, 0, len(o.storageClasses))
	for _, sc := range o.storageClasses {
		sc.RemovePoolsForBackend(originalBackend)
		if added := sc.CheckAndAddBackend(ctx, backend); added > 0 {
			classes = append(classes, sc.GetName())
		}
	}
	if len(classes) == 0 {
		Logc(ctx).WithFields(LogFields{
			"backend": backend.Name(),
		}).Info("Updated backend satisfies no storage classes.")
	} else {
		Logc(ctx).WithFields(LogFields{
			"backend": backend.Name(),
		}).Infof("Updated backend satisfies storage classes %s.",
			strings.Join(classes, ", "))
	}

	Logc(ctx).WithFields(LogFields{
		"backend": backend,
	}).Debug("Returning external version")
	return backend.ConstructExternal(ctx), nil
}

// UpdateBackendState updates an existing backend's state.
func (o *TridentOrchestrator) UpdateBackendState(
	ctx context.Context, backendName, backendState string,
) (backendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_update_state", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	return o.updateBackendState(ctx, backendName, backendState)
}

// updateBackendState updates an existing backend's state. It assumes the mutex lock is already held.
func (o *TridentOrchestrator) updateBackendState(
	ctx context.Context, backendName, backendState string,
) (backendExternal *storage.BackendExternal, err error) {
	var backend storage.Backend

	Logc(ctx).WithFields(LogFields{
		"backendName":  backendName,
		"backendState": backendState,
	}).Debug("UpdateBackendState")

	// First, check whether the backend exists.
	backendUUID, err := o.getBackendUUIDByBackendName(backendName)
	if err != nil {
		return nil, err
	}
	backend, found := o.backends[backendUUID]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("backend %v was not found", backendName))
	}

	newBackendState := storage.BackendState(backendState)

	// Limit the command to Failed
	if !newBackendState.IsFailed() {
		return nil, fmt.Errorf("unsupported backend state: %s", newBackendState)
	}

	if !newBackendState.IsOnline() {
		backend.Terminate(ctx)
	}
	backend.SetState(newBackendState)

	return backend.ConstructExternal(ctx), o.storeClient.UpdateBackend(ctx, backend)
}

func (o *TridentOrchestrator) getBackendUUIDByBackendName(backendName string) (string, error) {
	backendUUID := ""
	for _, b := range o.backends {
		if b.Name() == backendName {
			backendUUID = b.BackendUUID()
			return backendUUID, nil
		}
	}
	return "", utils.NotFoundError(fmt.Sprintf("backend %v was not found", backendName))
}

func (o *TridentOrchestrator) getBackendByBackendName(backendName string) (storage.Backend, error) {
	for _, b := range o.backends {
		if b.Name() == backendName {
			return b, nil
		}
	}
	return nil, utils.NotFoundError(fmt.Sprintf("backend %v was not found", backendName))
}

func (o *TridentOrchestrator) getBackendByConfigRef(configRef string) (storage.Backend, error) {
	for _, b := range o.backends {
		if b.ConfigRef() == configRef {
			return b, nil
		}
	}
	return nil, utils.NotFoundError(fmt.Sprintf("backend based on configRef '%v' was not found", configRef))
}

func (o *TridentOrchestrator) getBackendByBackendUUID(backendUUID string) (storage.Backend, error) {
	backend := o.backends[backendUUID]
	if backend != nil {
		return backend, nil
	}
	return nil, utils.NotFoundError(fmt.Sprintf("backend uuid %v was not found", backendUUID))
}

func (o *TridentOrchestrator) GetBackend(
	ctx context.Context, backendName string,
) (backendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	backendUUID, err := o.getBackendUUIDByBackendName(backendName)
	if err != nil {
		return nil, err
	}
	backend, found := o.backends[backendUUID]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("backend %v was not found", backendName))
	}

	backendExternal = backend.ConstructExternal(ctx)
	Logc(ctx).WithFields(LogFields{
		"backend":               backend,
		"backendExternal":       backendExternal,
		"backendExternal.Name":  backendExternal.Name,
		"backendExternal.State": backendExternal.State.String(),
	}).Trace("GetBackend information.")
	return backendExternal, nil
}

func (o *TridentOrchestrator) GetBackendByBackendUUID(
	ctx context.Context, backendUUID string,
) (backendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	backend, err := o.getBackendByBackendUUID(backendUUID)
	if err != nil {
		return nil, err
	}

	backendExternal = backend.ConstructExternal(ctx)
	Logc(ctx).WithFields(LogFields{
		"backend":               backend,
		"backendUUID":           backendUUID,
		"backendExternal":       backendExternal,
		"backendExternal.Name":  backendExternal.Name,
		"backendExternal.State": backendExternal.State.String(),
	}).Trace("GetBackend information.")
	return backendExternal, nil
}

func (o *TridentOrchestrator) ListBackends(
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

	o.mutex.Lock()
	defer o.mutex.Unlock()

	Logc(ctx).Debugf("About to list backends: %v", o.backends)
	backends := make([]*storage.BackendExternal, 0)
	for _, b := range o.backends {
		backends = append(backends, b.ConstructExternal(ctx))
	}
	return backends, nil
}

func (o *TridentOrchestrator) DeleteBackend(ctx context.Context, backendName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		Logc(ctx).WithFields(LogFields{
			"bootstrapError": o.bootstrapError,
		}).Warn("DeleteBackend error")
		return o.bootstrapError
	}

	defer recordTiming("backend_delete", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	backendUUID, err := o.getBackendUUIDByBackendName(backendName)
	if err != nil {
		return err
	}
	return o.deleteBackendByBackendUUID(ctx, backendName, backendUUID)
}

func (o *TridentOrchestrator) DeleteBackendByBackendUUID(
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

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	return o.deleteBackendByBackendUUID(ctx, backendName, backendUUID)
}

func (o *TridentOrchestrator) deleteBackendByBackendUUID(ctx context.Context, backendName, backendUUID string) error {
	Logc(ctx).WithFields(LogFields{
		"backendName": backendName,
		"backendUUID": backendUUID,
	}).Debug("deleteBackendByBackendUUID")

	backend, found := o.backends[backendUUID]
	if !found {
		return utils.NotFoundError(fmt.Sprintf("backend %s not found", backendName))
	}

	// Do not allow deletion of TridentBackendConfig-based backends using tridentctl
	if backend.ConfigRef() != "" {
		if !o.isCRDContext(ctx) {
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

	backend.SetOnline(false) // TODO eventually remove
	backend.SetState(storage.Deleting)
	storageClasses := make(map[string]*storageclass.StorageClass)
	for _, storagePool := range backend.Storage() {
		for _, scName := range storagePool.StorageClasses() {
			storageClasses[scName] = o.storageClasses[scName]
		}
		storagePool.SetStorageClasses([]string{})
	}
	for _, sc := range storageClasses {
		sc.RemovePoolsForBackend(backend)
	}
	if !backend.HasVolumes() {
		backend.Terminate(ctx)
		delete(o.backends, backendUUID)
		return o.storeClient.DeleteBackend(ctx, backend)
	}
	Logc(ctx).WithFields(LogFields{
		"backend":        backend,
		"backend.Name":   backend.Name(),
		"backend.State":  backend.State().String(),
		"backend.Online": backend.Online(),
	}).Debug("OfflineBackend information.")

	return o.storeClient.UpdateBackend(ctx, backend)
}

// RemoveBackendConfigRef sets backend configRef to empty and updates it.
func (o *TridentOrchestrator) RemoveBackendConfigRef(ctx context.Context, backendUUID, configRef string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	defer recordTiming("backend_update", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	var b storage.Backend
	if b, err = o.getBackendByBackendUUID(backendUUID); err != nil {
		return utils.NotFoundError(fmt.Sprintf("backend with UUID '%s' not found", backendUUID))
	}

	if b.ConfigRef() != "" {
		if b.ConfigRef() != configRef {
			return fmt.Errorf("TridentBackendConfig with UID '%s' cannot request removal of configRef '%s' for backend"+
				" with UUID '%s'", configRef, b.ConfigRef(), backendUUID)
		}

		b.SetConfigRef("")
	}

	return o.storeClient.UpdateBackend(ctx, b)
}

func (o *TridentOrchestrator) AddVolume(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (externalVol *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_add", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	volumeConfig.Version = config.OrchestratorAPIVersion

	if volumeConfig.ShareSourceVolume != "" {
		return o.addSubordinateVolume(ctx, volumeConfig)
	} else {
		return o.addVolume(ctx, volumeConfig)
	}
}

func (o *TridentOrchestrator) addVolume(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (externalVol *storage.VolumeExternal, err error) {
	if _, ok := o.volumes[volumeConfig.Name]; ok {
		return nil, fmt.Errorf("volume %s already exists", volumeConfig.Name)
	}
	if _, ok := o.subordinateVolumes[volumeConfig.Name]; ok {
		return nil, fmt.Errorf("volume %s exists but is a subordinate volume", volumeConfig.Name)
	}

	// If a volume is already being created, retry the operation with the same backend
	// instead of continuing here and potentially starting over on a different backend.
	// Otherwise, treat this as a new volume creation workflow.
	if retryTxn, err := o.GetVolumeCreatingTransaction(ctx, volumeConfig); err != nil {
		return nil, err
	} else if retryTxn != nil {
		return o.addVolumeRetry(ctx, retryTxn)
	}

	return o.addVolumeInitial(ctx, volumeConfig)
}

// addVolumeInitial continues the volume creation operation.
// This method should only be called from AddVolume, as it does not take locks or otherwise do much validation
// of the volume config.
func (o *TridentOrchestrator) addVolumeInitial(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (externalVol *storage.VolumeExternal, err error) {
	var (
		backend storage.Backend
		vol     *storage.Volume
		pool    storage.Pool
		txn     *storage.VolumeTransaction
	)

	Logc(ctx).WithFields(LogFields{
		"VolumeMode": volumeConfig.VolumeMode,
		"AccessMode": volumeConfig.AccessMode,
		"Protocol":   volumeConfig.Protocol,
	}).Trace("Calling getProtocol with args.")
	// Get the protocol based on the specified access mode & protocol
	protocol, err := o.getProtocol(ctx, volumeConfig.VolumeMode, volumeConfig.AccessMode, volumeConfig.Protocol)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"StorageClass": volumeConfig.StorageClass,
	}).Trace("Looking for storage class in cache.")
	sc, ok := o.storageClasses[volumeConfig.StorageClass]
	if !ok {
		return nil, fmt.Errorf("unknown storage class: %s", volumeConfig.StorageClass)
	}

	Logc(ctx).WithFields(LogFields{
		"RequisiteTopologies": volumeConfig.RequisiteTopologies,
		"PreferredTopologies": volumeConfig.PreferredTopologies,
		"AccessMode":          volumeConfig.AccessMode,
	}).Trace("Calling GetStoragePoolsForProtocolByBackend with args.")
	pools := sc.GetStoragePoolsForProtocolByBackend(ctx, protocol, volumeConfig.RequisiteTopologies,
		volumeConfig.PreferredTopologies, volumeConfig.AccessMode)
	if len(pools) == 0 {
		return nil, fmt.Errorf("no available backends for storage class %s", volumeConfig.StorageClass)
	}

	// Add a transaction to clean out any existing transactions
	txn = &storage.VolumeTransaction{
		Config: volumeConfig,
		Op:     storage.AddVolume,
	}
	if err = o.AddVolumeTransaction(ctx, txn); err != nil {
		return nil, err
	}

	// Copy the volume config into a working copy should any backend mutate the config but fail to create the volume.
	mutableConfig := volumeConfig.ConstructClone()

	// Recovery functions in case of error
	defer func() {
		err = o.addVolumeCleanup(ctx, err, backend, vol, txn, mutableConfig)
	}()
	defer func() {
		err = o.addVolumeRetryCleanup(ctx, err, backend, pool, txn, mutableConfig)
	}()

	Logc(ctx).WithField("volume", mutableConfig.Name).Debugf("Looking through %d storage pools.", len(pools))

	var volumeCreateErrors error
	ineligibleBackends := make(map[string]struct{})

	// The pool lists are already shuffled, so just try them in order.
	// The loop terminates when creation on all matching pools has failed.
	for _, pool = range pools {
		backend = pool.Backend()

		// Mirror destinations can only be placed on mirroring enabled backends
		if mutableConfig.IsMirrorDestination && !backend.CanMirror() {
			Logc(ctx).Debugf("MirrorDestinations can only be placed on mirroring enabled backends")
			ineligibleBackends[backend.BackendUUID()] = struct{}{}
		}

		// If the pool's backend cannot possibly work, skip trying
		if _, ok := ineligibleBackends[backend.BackendUUID()]; ok {
			continue
		}

		// CreatePrepare has a side effect that updates the mutableConfig with the backend-specific internal name
		backend.Driver().CreatePrepare(ctx, mutableConfig)

		// Update transaction with updated mutableConfig
		txn = &storage.VolumeTransaction{
			Config: mutableConfig,
			Op:     storage.AddVolume,
		}
		if err = o.storeClient.UpdateVolumeTransaction(ctx, txn); err != nil {
			return nil, err
		}

		vol, err = backend.AddVolume(ctx, mutableConfig, pool, sc.GetAttributes(), false)
		if err != nil {

			logFields := LogFields{
				"backend":     backend.Name(),
				"backendUUID": backend.BackendUUID(),
				"pool":        pool.Name(),
				"volume":      mutableConfig.Name,
				"error":       err,
			}

			// If the volume is still creating, don't try another pool but let the cleanup logic
			// save the state of this operation in a transaction.
			if utils.IsVolumeCreatingError(err) {
				Logc(ctx).WithFields(logFields).Warn("Volume still creating on this backend.")
				return nil, err
			}

			// Log failure and continue for loop to find a pool that can create the volume.
			Logc(ctx).WithFields(logFields).Warn("Failed to create the volume on this pool.")
			volumeCreateErrors = multierr.Append(volumeCreateErrors,
				fmt.Errorf("[Failed to create volume %s on storage pool %s from backend %s: %w]",
					mutableConfig.Name, pool.Name(), backend.Name(), err))

			// If this backend cannot handle the new volume on any pool, remove it from further consideration.
			if drivers.IsBackendIneligibleError(err) {
				ineligibleBackends[backend.BackendUUID()] = struct{}{}
			}

			// AddVolume has failed and the backend may have mutated the volume config used so reset the mutable
			// config to a fresh copy of the original config. This prepares the mutable config for the next backend.
			mutableConfig = volumeConfig.ConstructClone()
		} else {
			// Volume creation succeeded, so register it and return the result
			return o.addVolumeFinish(ctx, txn, vol, backend, pool)
		}
	}

	externalVol = nil
	if volumeCreateErrors == nil {
		err = fmt.Errorf("no suitable %s backend with \"%s\" storage class and %s of free space was found",
			protocol, mutableConfig.StorageClass, mutableConfig.Size)
	} else {
		err = fmt.Errorf("encountered error(s) in creating the volume: %w", volumeCreateErrors)
	}

	return nil, err
}

// addVolumeRetry continues a volume creation operation that previously failed with a VolumeCreatingError.
// This method should only be called from AddVolume, as it does not take locks or otherwise do much validation
// of the volume config.
func (o *TridentOrchestrator) addVolumeRetry(
	ctx context.Context, txn *storage.VolumeTransaction,
) (externalVol *storage.VolumeExternal, err error) {
	var (
		backend storage.Backend
		pool    storage.Pool
		vol     *storage.Volume
	)

	if txn.Op != storage.VolumeCreating {
		return nil, errors.New("wrong transaction type for addVolumeRetry")
	}

	volumeConfig := &txn.VolumeCreatingConfig.VolumeConfig

	backend, found := o.backends[txn.VolumeCreatingConfig.BackendUUID]
	if !found {
		// Should never get here but just to be safe
		return nil, utils.NotFoundError(fmt.Sprintf("backend %s for volume %s not found",
			txn.VolumeCreatingConfig.BackendUUID, volumeConfig.Name))
	}

	pool, found = backend.Storage()[txn.VolumeCreatingConfig.Pool]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("pool %s for backend %s not found",
			txn.VolumeCreatingConfig.Pool, txn.VolumeCreatingConfig.BackendUUID))
	}

	// Recovery functions in case of error
	defer func() {
		err = o.addVolumeCleanup(ctx, err, backend, vol, txn, volumeConfig)
	}()
	defer func() {
		err = o.addVolumeRetryCleanup(ctx, err, backend, pool, txn, volumeConfig)
	}()

	vol, err = backend.AddVolume(ctx, volumeConfig, pool, make(map[string]sa.Request), true)
	if err != nil {

		logFields := LogFields{
			"backend":     backend.Name(),
			"backendUUID": backend.BackendUUID(),
			"pool":        pool.Name(),
			"volume":      volumeConfig.Name,
			"error":       err,
		}

		// If the volume is still creating, don't try another pool but let the cleanup logic
		// save the state of this operation in a transaction.
		if utils.IsVolumeCreatingError(err) {
			Logc(ctx).WithFields(logFields).Warn("Volume still creating on this backend.")
		} else {
			Logc(ctx).WithFields(logFields).Error("addVolumeRetry failed on this backend.")
		}

		return nil, err
	}

	// Volume creation succeeded, so register it and return the result
	return o.addVolumeFinish(ctx, txn, vol, backend, pool)
}

// addVolumeFinish is called after successful completion of a volume create/clone operation
// to save the volume in the persistent store as well as Trident's in-memory cache.
func (o *TridentOrchestrator) addVolumeFinish(
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

	// Update internal cache and return external form of the new volume
	o.volumes[vol.Config.Name] = vol
	externalVol = vol.ConstructExternal()
	return externalVol, nil
}

// UpdateVolume updates the LUKS passphrase names stored on a volume in the cache and persistent store.
func (o *TridentOrchestrator) UpdateVolume(ctx context.Context, volume string, passphraseNames *[]string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()
	vol, err := o.storeClient.GetVolume(ctx, volume)
	if err != nil {
		return err
	}

	// We want to make sure the persistence layer is updated before we update the core copy
	// So we have to deep copy the volume from the cache to construct the volume to pass into the store client
	newVolume := storage.NewVolume(vol.Config.ConstructClone(), vol.BackendUUID, vol.Pool, vol.Orphaned, vol.State)
	if passphraseNames != nil {
		newVolume.Config.LUKSPassphraseNames = *passphraseNames
	}
	err = o.storeClient.UpdateVolume(ctx, newVolume)
	if err != nil {
		return err
	}
	o.volumes[volume] = newVolume
	return nil
}

func (o *TridentOrchestrator) CloneVolume(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (externalVol *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_clone", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	if _, ok := o.volumes[volumeConfig.Name]; ok {
		return nil, fmt.Errorf("volume %s already exists", volumeConfig.Name)
	}

	volumeConfig.Version = config.OrchestratorAPIVersion

	// If a volume is already being cloned, retry the operation with the same backend
	// instead of continuing here and potentially starting over on a different backend.
	// Otherwise, treat this as a new volume creation workflow.
	if retryTxn, err := o.GetVolumeCreatingTransaction(ctx, volumeConfig); err != nil {
		return nil, err
	} else if retryTxn != nil {
		return o.cloneVolumeRetry(ctx, retryTxn)
	}

	return o.cloneVolumeInitial(ctx, volumeConfig)
}

func (o *TridentOrchestrator) cloneVolumeInitial(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (externalVol *storage.VolumeExternal, err error) {
	var (
		backend storage.Backend
		vol     *storage.Volume
		pool    storage.Pool
		txn     *storage.VolumeTransaction
	)

	Logc(ctx).WithFields(LogFields{
		"CloneSourceVolume": volumeConfig.CloneSourceVolume,
	}).Trace("Looking for source volume in volumes cache.")
	// Get the source volume
	sourceVolume, found := o.volumes[volumeConfig.CloneSourceVolume]
	if !found {
		Logc(ctx).WithFields(LogFields{
			"CloneSourceVolume": volumeConfig.CloneSourceVolume,
		}).Trace("Looking for source volume in subordinate volumes cache.")
		if _, ok := o.subordinateVolumes[volumeConfig.CloneSourceVolume]; ok {
			return nil, utils.NotFoundError(fmt.Sprintf("cloning subordinate volume %s is not allowed",
				volumeConfig.CloneSourceVolume))
		}
		return nil, utils.NotFoundError(fmt.Sprintf("source volume not found: %s", volumeConfig.CloneSourceVolume))
	}

	Logc(ctx).WithFields(LogFields{
		"Config.Size": sourceVolume.Config.Size,
	}).Trace("Checking volume size.")
	if volumeConfig.Size != "" {
		cloneSourceVolumeSize, err := strconv.ParseInt(sourceVolume.Config.Size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not get size of the clone source volume")
		}

		Logc(ctx).WithFields(LogFields{
			"Size": volumeConfig.Size,
		}).Trace("Checking clone volume size.")
		cloneVolumeSize, err := strconv.ParseInt(volumeConfig.Size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not get size of the clone volume")
		}

		fields := LogFields{
			"source_volume": sourceVolume.Config.Name,
			"volume":        volumeConfig.Name,
			"backendUUID":   sourceVolume.BackendUUID,
		}
		if cloneSourceVolumeSize < cloneVolumeSize {
			Logc(ctx).WithFields(fields).Error("requested PVC size is too large for the clone source")
			return nil, fmt.Errorf("requested PVC size '%d' is too large for the clone source '%d'",
				cloneVolumeSize, cloneSourceVolumeSize)
		}
		Logc(ctx).WithFields(fields).Trace("Requested PVC size is large enough for the clone source.")
	}

	if sourceVolume.Orphaned {
		Logc(ctx).WithFields(LogFields{
			"source_volume": sourceVolume.Config.Name,
			"volume":        volumeConfig.Name,
			"backendUUID":   sourceVolume.BackendUUID,
		}).Warnf("Clone operation is likely to fail with an orphaned source volume.")
	}

	sourceVolumeMode := sourceVolume.Config.VolumeMode
	Logc(ctx).WithFields(LogFields{
		"sourceVolumeMode": sourceVolumeMode,
		"VolumeMode":       volumeConfig.VolumeMode,
	}).Trace("Checking if the source volume's mode is compatible with the requested clone's mode.")
	if sourceVolumeMode != "" && sourceVolumeMode != volumeConfig.VolumeMode {
		return nil, utils.NotFoundError(fmt.Sprintf(
			"source volume's volume-mode (%s) is incompatible with requested clone's volume-mode (%s)",
			sourceVolume.Config.VolumeMode, volumeConfig.VolumeMode))
	}

	// Get the source backend
	Logc(ctx).WithFields(LogFields{
		"backendUUID": sourceVolume.BackendUUID,
	}).Trace("Checking if the source volume's backend is in the backends cache.")
	if backend, found = o.backends[sourceVolume.BackendUUID]; !found {
		// Should never get here but just to be safe
		return nil, utils.NotFoundError(fmt.Sprintf("backend %s for the source volume not found: %s",
			sourceVolume.BackendUUID, volumeConfig.CloneSourceVolume))
	}

	pool = storage.NewStoragePool(backend, "")

	// Clone the source config, as most of its attributes will apply to the clone
	cloneConfig := sourceVolume.Config.ConstructClone()
	internalName := ""
	internalID := ""

	Logc(ctx).WithFields(LogFields{
		"CloneName":            volumeConfig.Name,
		"SourceVolume":         volumeConfig.CloneSourceVolume,
		"SourceVolumeInternal": sourceVolume.Config.InternalName,
		"SourceSnapshot":       volumeConfig.CloneSourceSnapshot,
		"Qos":                  volumeConfig.Qos,
		"QosType":              volumeConfig.QosType,
	}).Trace("Adding attributes from the request which will affect clone creation.")

	cloneConfig.Name = volumeConfig.Name
	cloneConfig.InternalName = internalName
	cloneConfig.InternalID = internalID
	cloneConfig.CloneSourceVolume = volumeConfig.CloneSourceVolume
	cloneConfig.CloneSourceVolumeInternal = sourceVolume.Config.InternalName
	cloneConfig.CloneSourceSnapshot = volumeConfig.CloneSourceSnapshot
	cloneConfig.Qos = volumeConfig.Qos
	cloneConfig.QosType = volumeConfig.QosType
	// Clear these values as they were copied from the source volume Config
	cloneConfig.SubordinateVolumes = make(map[string]interface{})
	cloneConfig.ShareSourceVolume = ""
	cloneConfig.LUKSPassphraseNames = sourceVolume.Config.LUKSPassphraseNames
	// Override this value only if SplitOnClone has been defined in clone volume's config
	if volumeConfig.SplitOnClone != "" {
		cloneConfig.SplitOnClone = volumeConfig.SplitOnClone
	}

	// If it's from snapshot, we need the LUKS passphrases value from the snapshot
	isLUKS, err := strconv.ParseBool(cloneConfig.LUKSEncryption)
	// If the LUKSEncryption is not a bool (or is empty string) assume we are not making a LUKS volume
	if err != nil {
		isLUKS = false
	}
	if cloneConfig.CloneSourceSnapshot != "" && isLUKS {
		snapshotID := storage.MakeSnapshotID(cloneConfig.CloneSourceVolume, cloneConfig.CloneSourceSnapshot)
		sourceSnapshot, found := o.snapshots[snapshotID]
		if !found {
			return nil, utils.NotFoundError("Could not find source snapshot for volume")
		}
		cloneConfig.LUKSPassphraseNames = sourceSnapshot.Config.LUKSPassphraseNames
	}

	// With the introduction of Virtual Pools we will try our best to place the cloned volume in the same
	// Virtual Pool. For cases where attributes are not defined in the PVC (source/clone) but instead in the
	// backend storage pool, e.g. splitOnClone, we would like the cloned PV to have the same attribute value
	// as the source PV.
	// NOTE: The clone volume config can be different than the source volume config when a backend modification
	// changes the Virtual Pool's arrangement or the Virtual Pool's attributes are modified. This
	// is because pool names are autogenerated based and are based on their relative arrangement.
	sourceVolumePoolName := sourceVolume.Pool
	if sourceVolumePoolName != drivers.UnsetPool {
		if sourceVolumeStoragePool, ok := backend.Storage()[sourceVolumePoolName]; ok {
			pool = sourceVolumeStoragePool
		}
	}

	// Create the backend-specific internal names so they are saved in the transaction
	backend.Driver().CreatePrepare(ctx, cloneConfig)

	// Add transaction in case the operation must be rolled back later
	txn = &storage.VolumeTransaction{
		Config: cloneConfig,
		Op:     storage.AddVolume,
	}
	if err = o.AddVolumeTransaction(ctx, txn); err != nil {
		return nil, err
	}

	// Recovery functions in case of error
	defer func() {
		err = o.addVolumeCleanup(ctx, err, backend, vol, txn, cloneConfig)
	}()
	defer func() {
		err = o.addVolumeRetryCleanup(ctx, err, backend, pool, txn, cloneConfig)
	}()

	logFields := LogFields{
		"backend":      backend.Name(),
		"backendUUID":  backend.BackendUUID(),
		"volume":       cloneConfig.Name,
		"sourceVolume": cloneConfig.CloneSourceVolume,
		"error":        nil,
	}

	// Create the clone
	if vol, err = backend.CloneVolume(ctx, sourceVolume.Config, cloneConfig, pool, false); err != nil {

		logFields["error"] = err

		// If the volume is still creating, return the error to let the cleanup logic
		// save the state of this operation in a transaction.
		if utils.IsVolumeCreatingError(err) {
			Logc(ctx).WithFields(logFields).Warn("Volume still creating on this backend.")
			return nil, err
		}

		Logc(ctx).WithFields(logFields).Warn("Failed to create cloned volume on this backend.")
		return nil, fmt.Errorf("failed to create cloned volume %s on backend %s: %v",
			cloneConfig.Name, backend.Name(), err)
	}

	Logc(ctx).WithFields(logFields).Trace("Volume clone succeeded, registering it and returning the result.")
	return o.addVolumeFinish(ctx, txn, vol, backend, pool)
}

func (o *TridentOrchestrator) cloneVolumeRetry(
	ctx context.Context, txn *storage.VolumeTransaction,
) (externalVol *storage.VolumeExternal, err error) {
	var (
		backend storage.Backend
		vol     *storage.Volume
		pool    storage.Pool
	)

	if txn.Op != storage.VolumeCreating {
		return nil, errors.New("wrong transaction type for cloneVolumeRetry")
	}

	cloneConfig := &txn.VolumeCreatingConfig.VolumeConfig

	// Get the source volume
	sourceVolume, found := o.volumes[cloneConfig.CloneSourceVolume]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("source volume not found: %s", cloneConfig.CloneSourceVolume))
	}

	backend, found = o.backends[txn.VolumeCreatingConfig.BackendUUID]
	if !found {
		// Should never get here but just to be safe
		return nil, utils.NotFoundError(fmt.Sprintf("backend %s for volume %s not found",
			txn.VolumeCreatingConfig.BackendUUID, cloneConfig.Name))
	}

	// Try to place the cloned volume in the same pool as the source.  This doesn't always work,
	// as may be the case with imported volumes or virtual pools, so drivers must tolerate a nil
	// or non-existent pool.
	sourceVolumePoolName := txn.VolumeCreatingConfig.Pool
	if sourceVolumePoolName != drivers.UnsetPool {
		if sourceVolumeStoragePool, ok := backend.Storage()[sourceVolumePoolName]; ok {
			pool = sourceVolumeStoragePool
		}
	}
	if pool == nil {
		pool = storage.NewStoragePool(backend, "")
	}

	// Recovery functions in case of error
	defer func() {
		err = o.addVolumeCleanup(ctx, err, backend, vol, txn, cloneConfig)
	}()
	defer func() {
		err = o.addVolumeRetryCleanup(ctx, err, backend, pool, txn, cloneConfig)
	}()

	// Create the clone
	if vol, err = backend.CloneVolume(ctx, sourceVolume.Config, cloneConfig, pool, true); err != nil {

		logFields := LogFields{
			"backend":      backend.Name(),
			"backendUUID":  backend.BackendUUID(),
			"volume":       cloneConfig.Name,
			"sourceVolume": cloneConfig.CloneSourceVolume,
			"error":        err,
		}

		// If the volume is still creating, let the cleanup logic save the state
		// of this operation in a transaction.
		if utils.IsVolumeCreatingError(err) {
			Logc(ctx).WithFields(logFields).Warn("Volume still creating on this backend.")
		} else {
			Logc(ctx).WithFields(logFields).Error("addVolumeRetry failed on this backend.")
		}

		return nil, err
	}

	// Volume creation succeeded, so register it and return the result
	return o.addVolumeFinish(ctx, txn, vol, backend, pool)
}

// GetVolumeExternal is used by volume import so it doesn't check core's o.volumes to see if the
// volume exists or not. Instead it asks the driver if the volume exists before requesting
// the volume size. Returns the VolumeExternal representation of the volume.
func (o *TridentOrchestrator) GetVolumeExternal(
	ctx context.Context, volumeName, backendName string,
) (volExternal *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_get_external", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	Logc(ctx).WithFields(LogFields{
		"originalName": volumeName,
		"backendName":  backendName,
	}).Debug("Orchestrator#GetVolumeExternal")

	backendUUID, err := o.getBackendUUIDByBackendName(backendName)
	if err != nil {
		return nil, err
	}
	backend, ok := o.backends[backendUUID]
	if !ok {
		return nil, utils.NotFoundError(fmt.Sprintf("backend %s not found", backendName))
	}

	volExternal, err = backend.GetVolumeExternal(ctx, volumeName)
	if err != nil {
		return nil, err
	}

	return volExternal, nil
}

// GetVolumeByInternalName returns a volume by the given internal name
func (o *TridentOrchestrator) GetVolumeByInternalName(
	ctx context.Context, volumeInternal string,
) (volume string, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	defer recordTiming("volume_internal_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	for _, vol := range o.volumes {
		if vol.Config.InternalName == volumeInternal {
			return vol.Config.Name, nil
		}
	}
	return "", utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeInternal))
}

func (o *TridentOrchestrator) validateImportVolume(ctx context.Context, volumeConfig *storage.VolumeConfig) error {
	backend, err := o.getBackendByBackendUUID(volumeConfig.ImportBackendUUID)
	if err != nil {
		return fmt.Errorf("could not find backend; %v", err)
	}

	originalName := volumeConfig.ImportOriginalName
	backendUUID := volumeConfig.ImportBackendUUID

	for volumeName, volume := range o.volumes {
		if volume.Config.InternalName == originalName && volume.BackendUUID == backendUUID {
			return utils.FoundError(fmt.Sprintf("PV %s already exists for volume %s", originalName, volumeName))
		}
	}

	sc, ok := o.storageClasses[volumeConfig.StorageClass]
	if !ok {
		return fmt.Errorf("unknown storage class: %s", volumeConfig.StorageClass)
	}

	if !sc.IsAddedToBackend(backend, volumeConfig.StorageClass) {
		return fmt.Errorf("storageClass %s does not match any storage pools for backend %s",
			volumeConfig.StorageClass, backend.Name())
	}

	if backend.Driver().Get(ctx, originalName) != nil {
		return utils.NotFoundError(fmt.Sprintf("volume %s was not found", originalName))
	}

	// Identify the resultant protocol based on the VolumeMode, AccessMode and the Protocol. Fro a valid case
	// o.getProtocol returns a protocol that is either same as volumeConfig.Protocol or `file/block` protocol
	// in place of `Any` Protocol.
	protocol, err := o.getProtocol(ctx, volumeConfig.VolumeMode, volumeConfig.AccessMode, volumeConfig.Protocol)
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
	if len(volumeConfig.Protocol) == 0 {
		volumeConfig.Protocol = backend.GetProtocol(ctx)
	}

	// Make sure that for the Raw-block volume import we do not have ext3, ext4 or xfs filesystem specified
	if volumeConfig.VolumeMode == config.RawBlock {
		if volumeConfig.FileSystem != "" && volumeConfig.FileSystem != config.FsRaw {
			return fmt.Errorf("cannot create raw-block volume %s with the filesystem %s",
				originalName, volumeConfig.FileSystem)
		}
	}

	return nil
}

func (o *TridentOrchestrator) LegacyImportVolume(
	ctx context.Context, volumeConfig *storage.VolumeConfig, backendName string, notManaged bool,
	createPVandPVC VolumeCallback,
) (externalVol *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_import_legacy", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	Logc(ctx).WithFields(LogFields{
		"volumeConfig": volumeConfig,
		"backendName":  backendName,
	}).Debug("Orchestrator#ImportVolume")

	backend, err := o.getBackendByBackendName(backendName)
	if err != nil {
		return nil, utils.NotFoundError(fmt.Sprintf("backend %s not found: %v", backendName, err))
	}

	err = o.validateImportVolume(ctx, volumeConfig)
	if err != nil {
		return nil, err
	}

	// Add transaction in case operation must be rolled back
	volTxn := &storage.VolumeTransaction{
		Config: volumeConfig,
		Op:     storage.ImportVolume,
	}
	if err = o.AddVolumeTransaction(ctx, volTxn); err != nil {
		return nil, fmt.Errorf("failed to add volume transaction: %v", err)
	}

	// Recover function in case or error
	defer func() {
		err = o.importVolumeCleanup(ctx, err, volumeConfig, volTxn)
	}()

	volume, err := backend.ImportVolume(ctx, volumeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to import volume %s on backend %s: %v",
			volumeConfig.ImportOriginalName, backendName, err)
	}

	if !notManaged {
		// The volume is managed and is persisted.
		err = o.storeClient.AddVolume(ctx, volume)
		if err != nil {
			return nil, fmt.Errorf("failed to persist imported volume data: %v", err)
		}
		o.volumes[volumeConfig.Name] = volume
	}

	volExternal := volume.ConstructExternal()

	driverType, err := o.driverTypeForBackend(volExternal.BackendUUID)
	if err != nil {
		return nil, fmt.Errorf("unable to determine driver type from volume %v; %v", volExternal, err)
	}

	Logc(ctx).WithFields(LogFields{
		"backendUUID":    volExternal.BackendUUID,
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
		"driverType":     driverType,
	}).Debug("Driver processed volume import.")

	// This function includes cleanup logic. Thus it must be called at the end of the ImportVolume func.
	err = createPVandPVC(volExternal, driverType)
	if err != nil {
		Logc(ctx).Errorf("error occurred while creating PV and PVC %s: %v", volExternal.Config.Name, err)
		return nil, err
	}

	return volExternal, nil
}

func (o *TridentOrchestrator) ImportVolume(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (externalVol *storage.VolumeExternal, err error) {
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

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	Logc(ctx).WithFields(LogFields{
		"volumeConfig": volumeConfig,
		"backendUUID":  volumeConfig.ImportBackendUUID,
	}).Debug("Orchestrator#ImportVolume")

	backend, ok := o.backends[volumeConfig.ImportBackendUUID]
	if !ok {
		return nil, utils.NotFoundError(fmt.Sprintf("backend %s not found", volumeConfig.ImportBackendUUID))
	}

	err = o.validateImportVolume(ctx, volumeConfig)
	if err != nil {
		return nil, err
	}

	// Add transaction in case operation must be rolled back
	volTxn := &storage.VolumeTransaction{
		Config: volumeConfig,
		Op:     storage.ImportVolume,
	}
	if err = o.AddVolumeTransaction(ctx, volTxn); err != nil {
		return nil, fmt.Errorf("failed to add volume transaction: %v", err)
	}

	// Recover function in case or error
	defer func() {
		err = o.importVolumeCleanup(ctx, err, volumeConfig, volTxn)
	}()

	volume, err := backend.ImportVolume(ctx, volumeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to import volume %s on backend %s: %v", volumeConfig.ImportOriginalName,
			volumeConfig.ImportBackendUUID, err)
	}

	err = o.storeClient.AddVolume(ctx, volume)
	if err != nil {
		return nil, fmt.Errorf("failed to persist imported volume data: %v", err)
	}
	o.volumes[volumeConfig.Name] = volume

	volExternal := volume.ConstructExternal()

	driverType, err := o.driverTypeForBackend(volExternal.BackendUUID)
	if err != nil {
		return nil, fmt.Errorf("unable to determine driver type from volume %v; %v", volExternal, err)
	}

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
		"driverType":     driverType,
	}).Trace("Driver processed volume import.")

	return volExternal, nil
}

// AddVolumeTransaction is called from the volume create, clone, and resize
// methods to save a record of the operation in case it fails and must be
// cleaned up later.
func (o *TridentOrchestrator) AddVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error {
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
		if oldTxn.Op != storage.UpgradeVolume && oldTxn.Op != storage.VolumeCreating {
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
			return utils.FoundError("volume transaction already exists")
		}
	}

	return o.storeClient.AddVolumeTransaction(ctx, volTxn)
}

func (o *TridentOrchestrator) GetVolumeCreatingTransaction(
	ctx context.Context, config *storage.VolumeConfig,
) (*storage.VolumeTransaction, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	volTxn := &storage.VolumeTransaction{
		VolumeCreatingConfig: &storage.VolumeCreatingConfig{
			VolumeConfig: *config,
		},
		Op: storage.VolumeCreating,
	}

	if txn, err := o.storeClient.GetVolumeTransaction(ctx, volTxn); err != nil {
		return nil, err
	} else if txn != nil && txn.Op == storage.VolumeCreating {
		return txn, nil
	} else {
		return nil, nil
	}
}

func (o *TridentOrchestrator) GetVolumeTransaction(
	ctx context.Context, volTxn *storage.VolumeTransaction,
) (*storage.VolumeTransaction, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	return o.storeClient.GetVolumeTransaction(ctx, volTxn)
}

// DeleteVolumeTransaction deletes a volume transaction created by
// addVolumeTransaction.
func (o *TridentOrchestrator) DeleteVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	return o.storeClient.DeleteVolumeTransaction(ctx, volTxn)
}

// addVolumeCleanup is used as a deferred method from the volume create/clone methods
// to clean up in case anything goes wrong during the operation.
func (o *TridentOrchestrator) addVolumeCleanup(
	ctx context.Context, err error, backend storage.Backend, vol *storage.Volume,
	volTxn *storage.VolumeTransaction, volumeConfig *storage.VolumeConfig,
) error {
	var cleanupErr, txErr error

	// If in a retry situation, we already handled the error in addVolumeRetryCleanup.
	if utils.IsVolumeCreatingError(err) {
		return err
	}

	if err != nil {
		// We failed somewhere.  There are two possible cases:
		// 1.  We failed to allocate on a backend and fell through to the
		//     end of the function.  In this case, we don't need to roll
		//     anything back.
		// 2.  We failed to add the volume to the store.  In this case, we need
		//     to remove the volume from the backend.
		if backend != nil && vol != nil {
			// We succeeded in adding the volume to the backend; now
			// delete it.
			cleanupErr = backend.RemoveVolume(ctx, vol.Config)
			if cleanupErr != nil {
				cleanupErr = fmt.Errorf("unable to delete volume from backend during cleanup:  %v", cleanupErr)
			}
		}
	}
	if cleanupErr == nil {
		// Only clean up the volume transaction if we've succeeded at
		// cleaning up on the backend or if we didn't need to do so in the
		// first place.
		txErr = o.DeleteVolumeTransaction(ctx, volTxn)
		if txErr != nil {
			txErr = fmt.Errorf("unable to clean up add volume transaction; %v", txErr)
		}
	}
	if cleanupErr != nil || txErr != nil {
		// Remove the volume from memory, if it's there, so that the user
		// can try to re-add.  This will trigger recovery code.
		delete(o.volumes, volumeConfig.Name)

		// Report on all errors we encountered.
		errList := make([]string, 0, 3)
		for _, e := range []error{err, cleanupErr, txErr} {
			if e != nil {
				errList = append(errList, e.Error())
			}
		}
		err = fmt.Errorf(strings.Join(errList, ", "))
		Logc(ctx).Warnf(
			"Unable to clean up artifacts of volume creation; %v. Repeat creating the volume or restart %v.",
			err, config.OrchestratorName)
	}
	return err
}

// addVolumeRetryCleanup is used as a deferred method from the volume create/clone methods
// to handles the case where a volume creation took too long, is still ongoing, and must be
// preserved in a transaction.
func (o *TridentOrchestrator) addVolumeRetryCleanup(
	ctx context.Context, err error, backend storage.Backend, pool storage.Pool,
	volTxn *storage.VolumeTransaction, volumeConfig *storage.VolumeConfig,
) error {
	// If not in a retry situation, return the original error so addVolumeCleanup can do its job.
	if err == nil || !utils.IsVolumeCreatingError(err) {
		return err
	}

	// The volume is still creating.  There are two possible cases:
	// 1.  The initial create failed, in which case we need to replace the
	//     AddVolume transaction with a VolumeCreating transaction.
	// 2.  The create failed after one or more retries, in which case we
	//     leave the existing VolumeCreating transaction in place.

	existingTxn, getTxnErr := o.storeClient.GetVolumeTransaction(ctx, volTxn)

	if getTxnErr != nil || existingTxn == nil {
		Logc(ctx).WithFields(LogFields{
			"error":       getTxnErr,
			"existingTxn": existingTxn,
		}).Debug("addVolumeRetryCleanup, nothing to do.")

		// Return the original error
		return err
	}

	if existingTxn.Op == storage.AddVolume {

		creatingTxn := &storage.VolumeTransaction{
			VolumeCreatingConfig: &storage.VolumeCreatingConfig{
				StartTime:    time.Now(),
				BackendUUID:  backend.BackendUUID(),
				Pool:         pool.Name(),
				VolumeConfig: *volumeConfig,
			},
			Op: storage.VolumeCreating,
		}

		if creatingErr := o.storeClient.UpdateVolumeTransaction(ctx, creatingTxn); creatingErr != nil {
			return err
		}

		Logc(ctx).Debug("addVolumeRetryCleanup, volume still creating, replaced AddVolume transaction.")

	} else if existingTxn.Op == storage.VolumeCreating {
		Logc(ctx).Debug("addVolumeRetryCleanup, volume still creating, leaving VolumeCreating transaction.")
	}
	return err
}

func (o *TridentOrchestrator) importVolumeCleanup(
	ctx context.Context, err error, volumeConfig *storage.VolumeConfig, volTxn *storage.VolumeTransaction,
) error {
	var cleanupErr, txErr error

	backend, ok := o.backends[volumeConfig.ImportBackendUUID]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("backend %s not found", volumeConfig.ImportBackendUUID))
	}

	if err != nil {
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
		if volume, ok := o.volumes[volumeConfig.Name]; ok {
			delete(o.volumes, volumeConfig.Name)
			if err = o.storeClient.DeleteVolume(ctx, volume); err != nil {
				return fmt.Errorf("error occurred removing volume from persistent store; %v", err)
			}
		}
	}
	txErr = o.DeleteVolumeTransaction(ctx, volTxn)
	if txErr != nil {
		txErr = fmt.Errorf("unable to clean up import volume transaction:  %v", txErr)
	}

	if cleanupErr != nil || txErr != nil {
		// Report on all errors we encountered.
		errList := make([]string, 0, 3)
		for _, e := range []error{err, cleanupErr, txErr} {
			if e != nil {
				errList = append(errList, e.Error())
			}
		}
		err = fmt.Errorf(strings.Join(errList, ", "))
		Logc(ctx).Warnf("Unable to clean up artifacts of volume import: %v. Repeat importing the volume %v.",
			err, volumeConfig.ImportOriginalName)
	}

	return err
}

func (o *TridentOrchestrator) GetVolume(
	ctx context.Context, volumeName string,
) (volExternal *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return o.getVolume(ctx, volumeName)
}

func (o *TridentOrchestrator) getVolume(
	_ context.Context, volumeName string,
) (volExternal *storage.VolumeExternal, err error) {
	if volume, found := o.volumes[volumeName]; found {
		return volume.ConstructExternal(), nil
	}
	if volume, found := o.subordinateVolumes[volumeName]; found {
		return volume.ConstructExternal(), nil
	}
	return nil, utils.NotFoundError(fmt.Sprintf("volume %v was not found", volumeName))
}

// driverTypeForBackend does the necessary work to get the driver type.  It does
// not construct a transaction, nor does it take locks; it assumes that the
// caller will take care of both of these.  It also assumes that the backend
// exists in memory.
func (o *TridentOrchestrator) driverTypeForBackend(backendUUID string) (string, error) {
	if b, ok := o.backends[backendUUID]; ok {
		return b.GetDriverName(), nil
	}
	return config.UnknownDriver, fmt.Errorf("unknown backend with UUID %s", backendUUID)
}

func (o *TridentOrchestrator) ListVolumes(ctx context.Context) (volumes []*storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_list", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	volumes = make([]*storage.VolumeExternal, 0, len(o.volumes)+len(o.subordinateVolumes))
	for _, v := range o.volumes {
		volumes = append(volumes, v.ConstructExternal())
	}
	for _, v := range o.subordinateVolumes {
		volumes = append(volumes, v.ConstructExternal())
	}

	sort.Sort(storage.ByVolumeExternalName(volumes))

	return volumes, nil
}

// volumeSnapshots returns any Snapshots for the specified volume
func (o *TridentOrchestrator) volumeSnapshots(volumeName string) ([]*storage.Snapshot, error) {
	volume, volumeFound := o.volumes[volumeName]
	if !volumeFound {
		return nil, fmt.Errorf("could not find volume %v", volumeName)
	}

	result := make([]*storage.Snapshot, 0, len(o.snapshots))
	for _, snapshot := range o.snapshots {
		if snapshot.Config.VolumeName == volume.Config.Name {
			result = append(result, snapshot)
		}
	}
	return result, nil
}

// deleteVolume does the necessary work to delete a volume entirely.  It does
// not construct a transaction, nor does it take locks; it assumes that the
// caller will take care of both of these.  It also assumes that the volume
// exists in memory.
func (o *TridentOrchestrator) deleteVolume(ctx context.Context, volumeName string) error {
	volume := o.volumes[volumeName]
	volumeBackend := o.backends[volume.BackendUUID]

	// If there are any snapshots or subordinate volumes for this volume, we need to "soft" delete.
	// Only hard delete this volume when its last snapshot is deleted and no subordinates remain.
	snapshotsForVolume, err := o.volumeSnapshots(volumeName)
	if err != nil {
		return err
	}

	if len(snapshotsForVolume) > 0 || len(volume.Config.SubordinateVolumes) > 0 {
		Logc(ctx).WithFields(LogFields{
			"volume":                  volumeName,
			"backendUUID":             volume.BackendUUID,
			"len(snapshotsForVolume)": len(snapshotsForVolume),
		}).Debug("Soft deleting.")
		volume.State = storage.VolumeStateDeleting
		if updateErr := o.updateVolumeOnPersistentStore(ctx, volume); updateErr != nil {
			Logc(ctx).WithFields(LogFields{
				"volume":    volume.Config.Name,
				"updateErr": updateErr.Error(),
			}).Error("Unable to update the volume's state to deleting in the persistent store.")
			return updateErr
		}
		return nil
	}

	// Note that this block will only be entered in the case that the volume
	// is missing its backend and the backend is nil. If the backend does not
	// exist, delete the volume and clean up, then return.
	if volumeBackend == nil {
		if err = o.storeClient.DeleteVolume(ctx, volume); err != nil {
			return err
		}
		delete(o.volumes, volumeName)
		return nil
	}

	// Note that this call will only return an error if the backend actually
	// fails to delete the volume.  If the volume does not exist on the backend,
	// the driver will not return an error.  Thus, we're fine.
	if err := volumeBackend.RemoveVolume(ctx, volume.Config); err != nil {
		if _, ok := err.(*storage.NotManagedError); !ok {
			Logc(ctx).WithFields(LogFields{
				"volume":      volumeName,
				"backendUUID": volume.BackendUUID,
				"error":       err,
			}).Error("Unable to delete volume from backend.")
			return err
		} else {
			Logc(ctx).WithFields(LogFields{
				"volume":      volumeName,
				"backendUUID": volume.BackendUUID,
				"error":       err,
			}).Debug("Skipping backend deletion of volume.")
		}
	}
	if err = o.storeClient.DeleteVolume(ctx, volume); err != nil {
		return err
	}

	// Check if we need to remove a soft-deleted source volume
	if volume.Config.CloneSourceVolume != "" {
		if cloneSourceVolume, ok := o.volumes[volume.Config.CloneSourceVolume]; ok && cloneSourceVolume.IsDeleting() {
			if o.deleteVolume(ctx, cloneSourceVolume.Config.Name) != nil {
				Logc(ctx).WithField("name", cloneSourceVolume.Config.Name).Warning(
					"Could not delete volume in deleting state.")
			}
		}
	}

	// Check if we need to remove a soft-deleted backend
	if volumeBackend.State().IsDeleting() && !volumeBackend.HasVolumes() {
		if err := o.storeClient.DeleteBackend(ctx, volumeBackend); err != nil {
			Logc(ctx).WithFields(LogFields{
				"backendUUID": volume.BackendUUID,
				"volume":      volumeName,
			}).Error("Unable to delete offline backend from the backing store after its last volume was deleted. " +
				"Delete the volume again to remove the backend.")
			return err
		}
		volumeBackend.Terminate(ctx)
		delete(o.backends, volume.BackendUUID)
	}
	delete(o.volumes, volumeName)
	return nil
}

// DeleteVolume does the necessary set up to delete a volume during the course
// of normal operation, verifying that the volume is present in Trident and
// creating a transaction to ensure that the delete eventually completes.
func (o *TridentOrchestrator) DeleteVolume(ctx context.Context, volumeName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_delete", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	if _, ok := o.subordinateVolumes[volumeName]; ok {
		return o.deleteSubordinateVolume(ctx, volumeName)
	}

	volume, ok := o.volumes[volumeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
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
	if err := o.AddVolumeTransaction(ctx, volTxn); err != nil {
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
			errList := make([]string, 0, 2)
			for _, e := range []error{err, errTxn} {
				if e != nil {
					errList = append(errList, e.Error())
				}
			}
			err = fmt.Errorf(strings.Join(errList, ", "))
		}
	}()

	return o.deleteVolume(ctx, volumeName)
}

func (o *TridentOrchestrator) PublishVolume(
	ctx context.Context, volumeName string, publishInfo *utils.VolumePublishInfo,
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

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return o.publishVolume(ctx, volumeName, publishInfo)
}

func (o *TridentOrchestrator) publishVolume(
	ctx context.Context, volumeName string, publishInfo *utils.VolumePublishInfo,
) error {
	fields := LogFields{
		"volumeName": volumeName,
		"nodeName":   publishInfo.HostName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>>>> Orchestrator#publishVolume")
	defer Logc(ctx).Debug("<<<<<< Orchestrator#publishVolume")

	volume, ok := o.subordinateVolumes[volumeName]
	if ok {
		// If volume is a subordinate, replace it with its source volume since that is what we will manipulate.
		// The inclusion of all volume publications for the source and it subordinates will produce the correct
		// set of exported nodes.
		if volume, ok = o.volumes[volume.Config.ShareSourceVolume]; !ok {
			return utils.NotFoundError(fmt.Sprintf("source volume for subordinate volumes %s not found",
				volumeName))
		}
	} else if volume, ok = o.volumes[volumeName]; !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}

	if volume.State.IsDeleting() {
		return utils.VolumeStateError(fmt.Sprintf("volume %s is deleting", volumeName))
	}

	publishInfo.BackendUUID = volume.BackendUUID
	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		// Not a not found error because this is not user input
		return fmt.Errorf("backend %s not found", volume.BackendUUID)
	}

	// Publish enforcement is considered only when the backend supports publish enforcement in CSI deployments.
	publishEnforceable := config.CurrentDriverContext == config.ContextCSI && backend.CanEnablePublishEnforcement()
	if publishEnforceable {
		nodeInfo := o.nodes.Get(publishInfo.HostName)
		if nodeInfo == nil {
			return fmt.Errorf("node %s not found", publishInfo.HostName)
		}
		if nodeInfo.PublicationState != utils.NodeClean {
			return utils.NodeNotSafeToPublishForBackendError(publishInfo.HostName, backend.GetDriverName())
		}
	}

	// Check if the publication already exists.
	publication, found := o.volumePublications.TryGet(volumeName, publishInfo.HostName)
	if !found {
		Logc(ctx).WithFields(fields).Debug("Volume publication record not found; generating a new one.")
		publication = generateVolumePublication(volumeName, publishInfo)

		// Bail out if the publication is nil at this point, or we fail to add the publication to the store and cache.
		if err := o.addVolumePublication(ctx, publication); err != nil || publication == nil {
			msg := "error saving volume publication record"
			Logc(ctx).WithError(err).Error(msg)
			return fmt.Errorf(msg)
		}
	}

	// Check if new request matches old publication
	if publishInfo.ReadOnly != publication.ReadOnly || publishInfo.AccessMode != publication.AccessMode {
		msg := "this volume is already published to this node with different options"
		Logc(ctx).WithFields(LogFields{
			"OldReadOnly":   publication.ReadOnly,
			"OldAccessMode": publication.AccessMode,
			"NewReadOnly":   publishInfo.ReadOnly,
			"NewAccessMode": publishInfo.AccessMode,
		}).Errorf(msg)
		return utils.FoundError(msg)
	}

	publishInfo.TridentUUID = o.uuid

	// Enable publish enforcement if the backend supports publish enforcement and the volume isn't already enforced.
	if publishEnforceable && !volume.Config.AccessInfo.PublishEnforcement {
		// Only enforce volume publication access if we're confident we're synced with the container orchestrator.
		if o.volumePublicationsSynced {
			publications := o.listVolumePublicationsForVolumeAndSubordinates(ctx, volumeName)
			// If there are no volume publications or the only publication is the current one, we can enable enforcement.
			var safeToEnable bool
			switch len(publications) {
			case 0:
				safeToEnable = true
			case 1:
				if publications[0].VolumeName == volumeName {
					safeToEnable = true
				}
			default:
				safeToEnable = false
			}

			if safeToEnable {
				if err := backend.EnablePublishEnforcement(ctx, volume); err != nil {
					if !utils.IsUnsupportedError(err) {
						Logc(ctx).WithFields(fields).WithError(err).Errorf(
							"Failed to enable volume publish enforcement for volume.")
						return err
					}
					Logc(ctx).WithFields(fields).WithError(err).Debug(
						"Volume publish enforcement is not fully supported on backend.")
				}
			}
		}
	}

	// Fill in what we already know
	publishInfo.VolumeAccessInfo = volume.Config.AccessInfo

	publishInfo.Nodes = o.nodes.List()
	if err := o.reconcileNodeAccessOnBackend(ctx, backend); err != nil {
		err = fmt.Errorf("unable to update node access rules on backend %s; %v", backend.Name(), err)
		Logc(ctx).Error(err)
		return err
	}

	err := backend.PublishVolume(ctx, volume.Config, publishInfo)
	if err != nil {
		return err
	}
	if err := o.updateVolumeOnPersistentStore(ctx, volume); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume": volume.Config.Name,
		}).Error("Unable to update the volume in persistent store.")
		return err
	}

	return nil
}

func generateVolumePublication(volName string, publishInfo *utils.VolumePublishInfo) *utils.VolumePublication {
	vp := &utils.VolumePublication{
		Name:       utils.GenerateVolumePublishName(volName, publishInfo.HostName),
		VolumeName: volName,
		NodeName:   publishInfo.HostName,
		ReadOnly:   publishInfo.ReadOnly,
		AccessMode: publishInfo.AccessMode,
	}

	return vp
}

// setPublicationsSynced marks if volume publications have been synced or not and updates it in the store.
func (o *TridentOrchestrator) setPublicationsSynced(ctx context.Context, volumePublicationsSynced bool) error {
	version, err := o.storeClient.GetVersion(ctx)
	if err != nil {
		msg := "error getting Trident version object"
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}
	version.PublicationsSynced = volumePublicationsSynced
	err = o.storeClient.SetVersion(ctx, version)
	if err != nil {
		msg := "error updating Trident version object"
		Logc(ctx).WithError(err).Error(msg)
		return fmt.Errorf(msg)
	}
	// Update in-memory flag.
	o.volumePublicationsSynced = volumePublicationsSynced
	return nil
}

func (o *TridentOrchestrator) UnpublishVolume(ctx context.Context, volumeName, nodeName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_unpublish", &err)()

	fields := LogFields{
		"volume": volumeName,
		"node":   nodeName,
	}
	Logc(ctx).WithFields(fields).Info("Unpublishing volume from node.") // audit trail

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return o.unpublishVolume(ctx, volumeName, nodeName)
}

func (o *TridentOrchestrator) unpublishVolume(ctx context.Context, volumeName, nodeName string) error {
	fields := LogFields{
		"volumeName": volumeName,
		"nodeName":   nodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>>>> Orchestrator#unpublishVolume")
	defer Logc(ctx).Debug("<<<<<< Orchestrator#unpublishVolume")

	publication, found := o.volumePublications.TryGet(volumeName, nodeName)
	if !found {
		// If the publication couldn't be found and Trident isn't in docker mode, bail out.
		// Otherwise, continue un-publishing. It is ok for the publication to not exist at this point for docker.
		if config.CurrentDriverContext != config.ContextDocker {
			Logc(ctx).WithError(utils.NotFoundError("unable to get volume publication record")).WithFields(fields).Warn("Volume is not published to node; doing nothing.")
			return nil
		}
	} else if publication == nil {
		msg := "volumePublication is nil"
		Logc(ctx).WithFields(fields).Errorf(msg)
		return fmt.Errorf(msg)
	}

	publishInfo := &utils.VolumePublishInfo{
		HostName:    nodeName,
		TridentUUID: o.uuid,
	}

	volume, ok := o.subordinateVolumes[volumeName]
	if ok {
		// If volume is a subordinate, replace it with its source volume since that is what we will manipulate.
		// The inclusion of all volume publications for the source and it subordinates will produce the correct
		// set of exported nodes.
		sourceVol := volume.Config.ShareSourceVolume
		if volume, ok = o.volumes[volume.Config.ShareSourceVolume]; !ok {
			return utils.NotFoundError(fmt.Sprintf("source volume %s for subordinate volumes %s not found",
				sourceVol, volumeName))
		}
	} else if volume, ok = o.volumes[volumeName]; !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}

	// Build list of nodes to which the volume remains published.
	nodeMap := make(map[string]*utils.Node)
	for _, pub := range o.listVolumePublicationsForVolumeAndSubordinates(ctx, volume.Config.Name) {

		// Exclude the publication we are unpublishing.
		if pub.VolumeName == volumeName && pub.NodeName == nodeName {
			continue
		}

		if n := o.nodes.Get(pub.NodeName); n == nil {
			Logc(ctx).WithField("node", pub.NodeName).Warning("Node not found during volume unpublish.")
			continue
		} else {
			nodeMap[pub.NodeName] = n
		}
	}

	for _, n := range nodeMap {
		publishInfo.Nodes = append(publishInfo.Nodes, n)
	}

	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		// Not a not found error because this is not user input.
		return fmt.Errorf("backend %s not found", volume.BackendUUID)
	}

	// Unpublish the volume.
	if err := backend.UnpublishVolume(ctx, volume.Config, publishInfo); err != nil {
		if !utils.IsNotFoundError(err) {
			return err
		}
		Logc(ctx).Debug("Volume not found in backend during unpublish; continuing with unpublish.")
	}

	if err := o.updateVolumeOnPersistentStore(ctx, volume); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume": volume.Config.Name,
		}).Error("Unable to update the volume in persistent store.")
		return err
	}

	// Delete the publication from memory and the persistence layer.
	if err := o.deleteVolumePublication(ctx, volumeName, nodeName); err != nil {
		msg := "unable to delete volume publication record"
		if !utils.IsNotFoundError(err) {
			Logc(ctx).WithError(err).Error(msg)
			return fmt.Errorf(msg)
		}
		Logc(ctx).WithError(err).Debugf("%s; publication not found, ignoring.", msg)
	}

	return nil
}

// isDockerPluginMode returns true if the ENV variable config.DockerPluginModeEnvVariable is set
func isDockerPluginMode() bool {
	return os.Getenv(config.DockerPluginModeEnvVariable) != ""
}

// AttachVolume mounts a volume to the local host.  This method is currently only used by Docker,
// and it should be able to accomplish its task using only the data passed in; it should not need to
// use the storage controller API.  It may be assumed that this method always runs on the host to
// which the volume will be attached.
func (o *TridentOrchestrator) AttachVolume(
	ctx context.Context, volumeName, mountpoint string, publishInfo *utils.VolumePublishInfo,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_attach", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}
	if volume.State.IsDeleting() {
		return utils.VolumeStateError(fmt.Sprintf("volume %s is deleting", volumeName))
	}

	hostMountpoint := mountpoint
	isDockerPluginModeSet := isDockerPluginMode()
	if isDockerPluginModeSet {
		hostMountpoint = filepath.Join("/host", mountpoint)
	}

	Logc(ctx).WithFields(LogFields{
		"volume":                volumeName,
		"mountpoint":            mountpoint,
		"hostMountpoint":        hostMountpoint,
		"isDockerPluginModeSet": isDockerPluginModeSet,
	}).Debug("Mounting volume.")

	// Ensure mount point exists and is a directory
	fileInfo, err := os.Lstat(hostMountpoint)
	if os.IsNotExist(err) {
		// Create the mount point if it doesn't exist
		if err := os.MkdirAll(hostMountpoint, 0o755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !fileInfo.IsDir() {
		return fmt.Errorf("%v already exists and it's not a directory", hostMountpoint)
	}

	// Check if volume is already mounted
	dfOutput, dfOuputErr := utils.GetDFOutput(ctx)
	if dfOuputErr != nil {
		err = fmt.Errorf("error checking if %v is already mounted: %v", mountpoint, dfOuputErr)
		return err
	}
	for _, e := range dfOutput {
		if e.Target == mountpoint {
			Logc(ctx).Debugf("%v is already mounted", mountpoint)
			return nil
		}
	}

	if publishInfo.FilesystemType == "nfs" {
		return utils.AttachNFSVolume(ctx, volumeName, mountpoint, publishInfo)
	} else if strings.Contains(publishInfo.FilesystemType, "nfs/") {
		// Determine where to mount the NFS share containing publishInfo.SubvolumeName
		mountpointDir := path.Dir(mountpoint)
		publishInfo.NFSMountpoint = path.Join(mountpointDir, publishInfo.NfsPath)

		// TODO: Check if Docker can do raw block volumes, if not, then remove this part
		isRawBlock := publishInfo.FilesystemType == "nfs/"+config.FsRaw
		if isRawBlock {
			publishInfo.SubvolumeMountOptions = utils.AppendToStringList(publishInfo.SubvolumeMountOptions, "bind", ",")
		}

		if loopDeviceName, _, err := utils.AttachBlockOnFileVolume(ctx, "", publishInfo); err != nil {
			return err
		} else {
			return utils.MountDevice(ctx, loopDeviceName, mountpoint, publishInfo.SubvolumeMountOptions, isRawBlock)
		}
	} else {
		return utils.AttachISCSIVolumeRetry(ctx, volumeName, mountpoint, publishInfo, map[string]string{}, AttachISCSIVolumeTimeoutLong)
	}
}

// DetachVolume unmounts a volume from the local host.  This method is currently only used by Docker,
// and it should be able to accomplish its task using only the data passed in; it should not need to
// use the storage controller API.  It may be assumed that this method always runs on the host to
// which the volume will be attached.  It ensures the volume is already mounted, and it attempts to
// delete the mount point.
func (o *TridentOrchestrator) DetachVolume(ctx context.Context, volumeName, mountpoint string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_detach", &err)()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}
	if volume.State.IsDeleting() {
		return utils.VolumeStateError(fmt.Sprintf("volume %s is deleting", volumeName))
	}

	hostMountpoint := mountpoint
	isDockerPluginModeSet := isDockerPluginMode()
	if isDockerPluginModeSet {
		hostMountpoint = filepath.Join("/host", mountpoint)
	}

	Logc(ctx).WithFields(LogFields{
		"volume":                volumeName,
		"mountpoint":            mountpoint,
		"hostMountpoint":        hostMountpoint,
		"isDockerPluginModeSet": isDockerPluginModeSet,
	}).Debug("Unmounting volume.")

	// Check if the mount point exists, so we know that it's attached and must be cleaned up
	_, err = os.Stat(hostMountpoint) // trident is not chrooted, therefore we must use the /host if in plugin mode
	if err != nil {
		// Not attached, so nothing to do
		return nil
	}

	// Unmount the volume
	if err := utils.Umount(ctx, mountpoint); err != nil {
		// utils.Unmount is chrooted, therefore it does NOT need the /host
		return err
	}

	// Best effort removal of the mount point
	_ = os.Remove(hostMountpoint) // trident is not chrooted, therefore we must the /host if in plugin mode
	return nil
}

// SetVolumeState sets the state of a volume to a given value
func (o *TridentOrchestrator) SetVolumeState(
	ctx context.Context, volumeName string, state storage.VolumeState,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_set_state", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}
	// Nothing to do if the volume is already in desired state
	if volume.State == state {
		return nil
	}
	if err := o.updateVolumeOnPersistentStore(ctx, volume); err != nil {
		msg := fmt.Sprintf("error updating volume in persistent store; %v", err)
		Logc(ctx).WithField("volume", volumeName).Errorf(msg)
		return fmt.Errorf(msg)
	}
	volume.State = state
	Logc(ctx).WithField("volume", volumeName).Debugf("Volume state set to %s.", string(state))
	return nil
}

// addSubordinateVolume defines a new subordinate volume and updates all references to it.  It does not create any
// backing storage resources.
func (o *TridentOrchestrator) addSubordinateVolume(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (externalVol *storage.VolumeExternal, err error) {
	var (
		ok           bool
		volume       *storage.Volume
		sourceVolume *storage.Volume
	)

	// Check if the volume already exists
	if _, ok = o.volumes[volumeConfig.Name]; ok {
		return nil, fmt.Errorf("volume %s exists but is not a subordinate volume", volumeConfig.Name)
	}
	if _, ok = o.subordinateVolumes[volumeConfig.Name]; ok {
		return nil, fmt.Errorf("subordinate volume %s already exists", volumeConfig.Name)
	}

	// Ensure requested size is valid
	volumeSize, err := strconv.ParseUint(volumeConfig.Size, 10, 64)
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

	// Ensure the source exists, is not a subordinate volume, is in a good state, and matches the new volume as needed
	sourceVolume, ok = o.volumes[volumeConfig.ShareSourceVolume]
	if !ok {
		if _, ok = o.subordinateVolumes[volumeConfig.ShareSourceVolume]; ok {
			return nil, utils.NotFoundError(fmt.Sprintf("creating subordinate to a subordinate volume %s is not allowed",
				volumeConfig.ShareSourceVolume))
		}
		return nil, utils.NotFoundError(fmt.Sprintf("source volume %s for subordinate volume %s not found",
			volumeConfig.ShareSourceVolume, volumeConfig.Name))
	} else if sourceVolume.Config.ImportNotManaged {
		return nil, fmt.Errorf(fmt.Sprintf("source volume %s for subordinate volume %s is an unmanaged import",
			volumeConfig.ShareSourceVolume, volumeConfig.Name))
	} else if sourceVolume.Config.StorageClass != volumeConfig.StorageClass {
		return nil, fmt.Errorf(fmt.Sprintf("source volume %s for subordinate volume %s has a different storage class",
			volumeConfig.ShareSourceVolume, volumeConfig.Name))
	} else if sourceVolume.Orphaned {
		return nil, fmt.Errorf(fmt.Sprintf("source volume %s for subordinate volume %s is orphaned",
			volumeConfig.ShareSourceVolume, volumeConfig.Name))
	} else if sourceVolume.State != storage.VolumeStateOnline {
		return nil, fmt.Errorf(fmt.Sprintf("source volume %s for subordinate volume %s is not online",
			volumeConfig.ShareSourceVolume, volumeConfig.Name))
	}

	// Get the backend and ensure it and the source volume are NFS
	if backend, ok := o.backends[sourceVolume.BackendUUID]; !ok {
		return nil, utils.NotFoundError(fmt.Sprintf("backend %s for source volume %s not found",
			sourceVolume.BackendUUID, sourceVolume.Config.Name))
	} else if backend.GetProtocol(ctx) != config.File || sourceVolume.Config.AccessInfo.NfsPath == "" {
		return nil, fmt.Errorf(fmt.Sprintf("source volume %s for subordinate volume %s must be NFS",
			volumeConfig.ShareSourceVolume, volumeConfig.Name))
	}

	// Ensure requested volume size is not larger than the source
	sourceVolumeSize, err := strconv.ParseUint(sourceVolume.Config.Size, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid size for source volume %s", sourceVolume.Config.Name)
	}
	if sourceVolumeSize < volumeSize {
		return nil, fmt.Errorf(fmt.Sprintf("source volume %s may not be smaller than subordinate volume %s",
			volumeConfig.ShareSourceVolume, volumeConfig.Name))
	}

	// Copy a few needed fields from the source volume
	volumeConfig.AccessInfo = sourceVolume.Config.AccessInfo
	volumeConfig.FileSystem = sourceVolume.Config.FileSystem
	volumeConfig.MountOptions = sourceVolume.Config.MountOptions
	volumeConfig.Protocol = sourceVolume.Config.Protocol
	volumeConfig.AccessInfo.PublishEnforcement = true

	volume = storage.NewVolume(volumeConfig, sourceVolume.BackendUUID, sourceVolume.Pool, false,
		storage.VolumeStateSubordinate)

	// Add new volume to persistent store
	if err = o.storeClient.AddVolume(ctx, volume); err != nil {
		return nil, err
	}

	// Register subordinate volume with source volume
	if sourceVolume.Config.SubordinateVolumes == nil {
		sourceVolume.Config.SubordinateVolumes = make(map[string]interface{})
	}
	sourceVolume.Config.SubordinateVolumes[volume.Config.Name] = nil

	// Update internal cache and return external form of the new volume
	o.subordinateVolumes[volume.Config.Name] = volume
	return volume.ConstructExternal(), nil
}

// deleteSubordinateVolume removes all references to a subordinate volume.  It does not delete any
// backing storage resources.
func (o *TridentOrchestrator) deleteSubordinateVolume(ctx context.Context, volumeName string) (err error) {
	volume, ok := o.subordinateVolumes[volumeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("subordinate volume %s not found", volumeName))
	}

	// Remove volume from persistent store
	if err = o.storeClient.DeleteVolume(ctx, volume); err != nil {
		return err
	}

	sourceVolume, ok := o.volumes[volume.Config.ShareSourceVolume]
	if ok {
		// Unregister subordinate volume with source volume
		if sourceVolume.Config.SubordinateVolumes != nil {
			delete(sourceVolume.Config.SubordinateVolumes, volumeName)
		}
		if len(sourceVolume.Config.SubordinateVolumes) == 0 {
			sourceVolume.Config.SubordinateVolumes = nil
		}

		// If this subordinate volume pinned its source volume in Deleting state, clean up the source volume
		// if it isn't still pinned by something else (snapshots).
		if sourceVolume.State.IsDeleting() && len(sourceVolume.Config.SubordinateVolumes) == 0 {
			if o.deleteVolume(ctx, sourceVolume.Config.Name) != nil {
				Logc(ctx).WithField("name", sourceVolume.Config.Name).Warning(
					"Could not delete volume in deleting state.")
			}
		}
	}

	delete(o.subordinateVolumes, volumeName)
	return nil
}

// ListSubordinateVolumes returns all subordinate volumes for all source volumes, or all subordinate
// volumes for a single source volume.
func (o *TridentOrchestrator) ListSubordinateVolumes(
	ctx context.Context, sourceVolumeName string,
) (volumes []*storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("subordinate_volume_list", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	var sourceVolumes map[string]*storage.Volume

	if sourceVolumeName == "" {
		// If listing all subordinate volumes, start with the entire source volume map
		sourceVolumes = o.volumes
	} else {
		// If listing subordinate volumes for a single source volume, make a map with just the source volume
		if sourceVolume, ok := o.volumes[sourceVolumeName]; !ok {
			return nil, utils.NotFoundError(fmt.Sprintf("source volume %s not found", sourceVolumeName))
		} else {
			sourceVolumes = make(map[string]*storage.Volume)
			sourceVolumes[sourceVolumeName] = sourceVolume
		}
	}

	volumes = make([]*storage.VolumeExternal, 0, len(sourceVolumes))

	for _, sourceVolume := range sourceVolumes {
		for subordinateVolumeName := range sourceVolume.Config.SubordinateVolumes {
			if subordinateVolume, ok := o.subordinateVolumes[subordinateVolumeName]; ok {
				volumes = append(volumes, subordinateVolume.ConstructExternal())
			}
		}
	}

	sort.Sort(storage.ByVolumeExternalName(volumes))

	return volumes, nil
}

// GetSubordinateSourceVolume returns the parent volume for a given subordinate volume.
func (o *TridentOrchestrator) GetSubordinateSourceVolume(
	ctx context.Context, subordinateVolumeName string,
) (volume *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("subordinate_source_volume_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	if subordinateVolume, ok := o.subordinateVolumes[subordinateVolumeName]; !ok {
		return nil, utils.NotFoundError(fmt.Sprintf("subordinate volume %s not found", subordinateVolumeName))
	} else {
		parentVolumeName := subordinateVolume.Config.ShareSourceVolume
		if sourceVolume, ok := o.volumes[parentVolumeName]; !ok {
			return nil, utils.NotFoundError(fmt.Sprintf("source volume %s not found", parentVolumeName))
		} else {
			volume = sourceVolume.ConstructExternal()
		}

	}

	return volume, nil
}

func (o *TridentOrchestrator) resizeSubordinateVolume(ctx context.Context, volumeName, newSize string) error {
	volume, ok := o.subordinateVolumes[volumeName]
	if !ok || volume == nil {
		return utils.NotFoundError(fmt.Sprintf("subordinate volume %s not found", volumeName))
	}

	sourceVolume, ok := o.volumes[volume.Config.ShareSourceVolume]
	if !ok || sourceVolume == nil {
		return utils.NotFoundError(fmt.Sprintf("source volume %s for subordinate volume %s not found",
			volume.Config.ShareSourceVolume, volumeName))
	}

	// Determine volume size in bytes
	newSizeStr, err := utils.ConvertSizeToBytes(newSize)
	if err != nil {
		return fmt.Errorf("could not convert volume size %s: %v", newSize, err)
	}
	newSizeBytes, err := strconv.ParseUint(newSizeStr, 10, 64)
	if err != nil {
		return fmt.Errorf("%v is an invalid volume size: %v", newSize, err)
	}

	// Determine source volume size in bytes
	sourceSizeStr, err := utils.ConvertSizeToBytes(sourceVolume.Config.Size)
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
			volume.Config.ShareSourceVolume)
	}

	// Save the new subordinate volume size before returning success
	volume.Config.Size = newSizeStr

	if err = o.updateVolumeOnPersistentStore(ctx, volume); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume": volume.Config.Name,
		}).Error("Unable to update the volume's size in persistent store.")
		return err
	}

	return nil
}

// CreateSnapshot creates a snapshot of the given volume
func (o *TridentOrchestrator) CreateSnapshot(
	ctx context.Context, snapshotConfig *storage.SnapshotConfig,
) (externalSnapshot *storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	var (
		ok       bool
		backend  storage.Backend
		volume   *storage.Volume
		snapshot *storage.Snapshot
	)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("snapshot_create", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	// Check if the snapshot already exists
	if _, ok := o.snapshots[snapshotConfig.ID()]; ok {
		return nil, fmt.Errorf("snapshot %s already exists", snapshotConfig.ID())
	}

	// Get the volume
	volume, ok = o.volumes[snapshotConfig.VolumeName]
	if !ok {
		if _, ok = o.subordinateVolumes[snapshotConfig.VolumeName]; ok {
			// TODO(sphadnis): Check if this block is dead code
			return nil, utils.NotFoundError(fmt.Sprintf("creating snapshot is not allowed on subordinate volume %s",
				snapshotConfig.VolumeName))
		}
		return nil, utils.NotFoundError(fmt.Sprintf("source volume %s not found", snapshotConfig.VolumeName))
	}
	if volume.State.IsDeleting() {
		return nil, utils.VolumeStateError(fmt.Sprintf("source volume %s is deleting", snapshotConfig.VolumeName))
	}

	// Get the backend
	if backend, ok = o.backends[volume.BackendUUID]; !ok {
		// Should never get here but just to be safe
		return nil, utils.NotFoundError(fmt.Sprintf("backend %s for the source volume not found: %s",
			volume.BackendUUID, snapshotConfig.VolumeName))
	}

	// Complete the snapshot config
	snapshotConfig.InternalName = snapshotConfig.Name
	snapshotConfig.VolumeInternalName = volume.Config.InternalName
	snapshotConfig.LUKSPassphraseNames = volume.Config.LUKSPassphraseNames

	// Ensure a snapshot is even possible before creating the transaction
	if err := backend.CanSnapshot(ctx, snapshotConfig, volume.Config); err != nil {
		return nil, err
	}

	// Add transaction in case the operation must be rolled back later
	txn := &storage.VolumeTransaction{
		Config:         volume.Config,
		SnapshotConfig: snapshotConfig,
		Op:             storage.AddSnapshot,
	}
	if err = o.AddVolumeTransaction(ctx, txn); err != nil {
		return nil, err
	}

	// Recovery function in case of error
	defer func() {
		err = o.addSnapshotCleanup(ctx, err, backend, snapshot, txn, snapshotConfig)
	}()

	// Create the snapshot
	snapshot, err = backend.CreateSnapshot(ctx, snapshotConfig, volume.Config)
	if err != nil {
		if utils.IsMaxLimitReachedError(err) {
			return nil, utils.MaxLimitReachedError(fmt.Sprintf("failed to create snapshot %s for volume %s on backend %s: %v",
				snapshotConfig.Name, snapshotConfig.VolumeName, backend.Name(), err))
		}
		return nil, fmt.Errorf("failed to create snapshot %s for volume %s on backend %s: %v",
			snapshotConfig.Name, snapshotConfig.VolumeName, backend.Name(), err)
	}

	// Ensure we store all potential LUKS passphrases, there may have been a passphrase rotation during the snapshot process
	volume = o.volumes[snapshotConfig.VolumeName]
	for _, v := range volume.Config.LUKSPassphraseNames {
		if !utils.SliceContainsString(snapshotConfig.LUKSPassphraseNames, v) {
			snapshotConfig.LUKSPassphraseNames = append(snapshotConfig.LUKSPassphraseNames, v)
		}
	}

	// Save references to new snapshot
	if err = o.storeClient.AddSnapshot(ctx, snapshot); err != nil {
		return nil, err
	}
	o.snapshots[snapshotConfig.ID()] = snapshot

	return snapshot.ConstructExternal(), nil
}

// addSnapshotCleanup is used as a deferred method from the snapshot create method
// to clean up in case anything goes wrong during the operation.
func (o *TridentOrchestrator) addSnapshotCleanup(
	ctx context.Context, err error, backend storage.Backend, snapshot *storage.Snapshot,
	volTxn *storage.VolumeTransaction, snapConfig *storage.SnapshotConfig,
) error {
	var cleanupErr, txErr error
	if err != nil {
		// We failed somewhere.  There are two possible cases:
		// 1.  We failed to create a snapshot and fell through to the
		//     end of the function.  In this case, we don't need to roll
		//     anything back.
		// 2.  We failed to save the snapshot to the persistent store.
		//     In this case, we need to remove the snapshot from the backend.
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
	if cleanupErr != nil || txErr != nil {
		// Remove the snapshot from memory, if it's there, so that the user
		// can try to re-add.  This will trigger recovery code.
		delete(o.snapshots, snapConfig.ID())

		// Report on all errors we encountered.
		errList := make([]string, 0, 3)
		for _, e := range []error{err, cleanupErr, txErr} {
			if e != nil {
				errList = append(errList, e.Error())
			}
		}
		err = fmt.Errorf(strings.Join(errList, ", "))
		Logc(ctx).Warnf(
			"Unable to clean up artifacts of snapshot creation: %v. Repeat creating the snapshot or restart %v.",
			err, config.OrchestratorName)
	}
	return err
}

func (o *TridentOrchestrator) GetSnapshot(
	ctx context.Context, volumeName, snapshotName string,
) (snapshotExternal *storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("snapshot_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return o.getSnapshot(ctx, volumeName, snapshotName)
}

func (o *TridentOrchestrator) getSnapshot(
	ctx context.Context, volumeName, snapshotName string,
) (*storage.SnapshotExternal, error) {
	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)
	snapshot, found := o.snapshots[snapshotID]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("snapshot %v was not found", snapshotName))
	} else if snapshot.State != storage.SnapshotStateCreating && snapshot.State != storage.SnapshotStateUploading {
		return snapshot.ConstructExternal(), nil
	}

	if updatedSnapshot, err := o.updateSnapshot(ctx, snapshot); err != nil {
		return snapshot.ConstructExternal(), err
	} else {
		return updatedSnapshot.ConstructExternal(), nil
	}
}

func (o *TridentOrchestrator) updateSnapshot(
	ctx context.Context, snapshot *storage.Snapshot,
) (*storage.Snapshot, error) {
	// If the snapshot is known to be ready, just return it
	if snapshot.State != storage.SnapshotStateCreating && snapshot.State != storage.SnapshotStateUploading {
		return snapshot, nil
	}

	snapshotID := snapshot.Config.ID()
	snapshot, ok := o.snapshots[snapshotID]
	if !ok {
		return snapshot, utils.NotFoundError(fmt.Sprintf("snapshot %s not found on volume %s",
			snapshot.Config.Name, snapshot.Config.VolumeName))
	}

	volume, ok := o.volumes[snapshot.Config.VolumeName]
	if !ok {
		return snapshot, utils.NotFoundError(fmt.Sprintf("volume %s not found", snapshot.Config.VolumeName))
	}

	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		return snapshot, utils.NotFoundError(fmt.Sprintf("backend %s not found", volume.BackendUUID))
	}

	updatedSnapshot, err := backend.GetSnapshot(ctx, snapshot.Config, volume.Config)
	if err != nil {
		return snapshot, err
	}

	// If the snapshot state has changed, persist it here and in the store
	if updatedSnapshot != nil && updatedSnapshot.State != snapshot.State {
		o.snapshots[snapshotID] = updatedSnapshot
		if err := o.storeClient.UpdateSnapshot(ctx, updatedSnapshot); err != nil {
			Logc(ctx).Errorf("could not update snapshot %s in persistent store; %v", snapshotID, err)
		}
	}

	return updatedSnapshot, nil
}

// deleteSnapshot does the necessary work to delete a snapshot entirely.  It does
// not construct a transaction, nor does it take locks; it assumes that the caller will
// take care of both of these.
func (o *TridentOrchestrator) deleteSnapshot(ctx context.Context, snapshotConfig *storage.SnapshotConfig) error {
	snapshotID := snapshotConfig.ID()
	snapshot, ok := o.snapshots[snapshotID]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("snapshot %s not found on volume %s",
			snapshotConfig.Name, snapshotConfig.VolumeName))
	}

	volume, ok := o.volumes[snapshotConfig.VolumeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", snapshotConfig.VolumeName))
	}

	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("backend %s not found", volume.BackendUUID))
	}

	// Note that this call will only return an error if the backend actually
	// fails to delete the snapshot.  If the snapshot does not exist on the backend,
	// the driver will not return an error.  Thus, we're fine.
	if err := backend.DeleteSnapshot(ctx, snapshot.Config, volume.Config); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume":   snapshot.Config.VolumeName,
			"snapshot": snapshot.Config.Name,
			"backend":  backend.Name(),
			"error":    err,
		}).Error("Unable to delete snapshot from backend.")
		return err
	}
	if err := o.storeClient.DeleteSnapshot(ctx, snapshot); err != nil {
		return err
	}

	delete(o.snapshots, snapshot.ID())

	// If this snapshot volume pinned its source volume in Deleting state, clean up the source volume
	// if it isn't still pinned by something else (subordinate volumes).
	if volume.State.IsDeleting() {
		if snapshotsForVolume, err := o.volumeSnapshots(snapshotConfig.VolumeName); err != nil {
			// TODO(sphadnis): Check if this block is dead code
			return err
		} else if len(snapshotsForVolume) == 0 {
			return o.deleteVolume(ctx, snapshotConfig.VolumeName)
		}
	}

	return nil
}

// DeleteSnapshot deletes a snapshot of the given volume
func (o *TridentOrchestrator) DeleteSnapshot(ctx context.Context, volumeName, snapshotName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("snapshot_delete", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)
	snapshot, ok := o.snapshots[snapshotID]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("snapshot %s not found on volume %s", snapshotName, volumeName))
	}

	volume, ok := o.volumes[volumeName]
	if !ok {
		if !snapshot.State.IsMissingVolume() {
			return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
		}
	}

	// Note that this block will only be entered in the case that the snapshot
	// is missing it's volume and the volume is nil. If the volume does not
	// exist, delete the snapshot and clean up, then return.
	if volume == nil {
		if err = o.storeClient.DeleteSnapshot(ctx, snapshot); err != nil {
			return err
		}
		delete(o.snapshots, snapshot.ID())
		return nil
	}

	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		if !snapshot.State.IsMissingBackend() {
			return utils.NotFoundError(fmt.Sprintf("backend %s not found", volume.BackendUUID))
		}
	}

	// Note that this block will only be entered in the case that the snapshot
	// is missing it's backend and the backend is nil. If the backend does not
	// exist, delete the snapshot and clean up, then return.
	if backend == nil {
		if err = o.storeClient.DeleteSnapshot(ctx, snapshot); err != nil {
			return err
		}
		delete(o.snapshots, snapshot.ID())
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

	volTxn := &storage.VolumeTransaction{
		Config:         volume.Config,
		SnapshotConfig: snapshot.Config,
		Op:             storage.DeleteSnapshot,
	}
	if err = o.AddVolumeTransaction(ctx, volTxn); err != nil {
		return err
	}

	defer func() {
		errTxn := o.DeleteVolumeTransaction(ctx, volTxn)
		if errTxn != nil {
			Logc(ctx).WithFields(LogFields{
				"volume":    volumeName,
				"snapshot":  snapshotName,
				"backend":   volume.BackendUUID,
				"error":     errTxn,
				"operation": volTxn.Op,
			}).Warnf("Unable to delete snapshot transaction. Repeat deletion using %s or restart %v.",
				config.OrchestratorClientName, config.OrchestratorName)
		}
		if err != nil || errTxn != nil {
			errList := make([]string, 0, 2)
			for _, e := range []error{err, errTxn} {
				if e != nil {
					errList = append(errList, e.Error())
				}
			}
			err = fmt.Errorf(strings.Join(errList, ", "))
		}
	}()

	// Delete the snapshot
	return o.deleteSnapshot(ctx, snapshot.Config)
}

func (o *TridentOrchestrator) ListSnapshots(ctx context.Context) (snapshots []*storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("snapshot_list", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	snapshots = make([]*storage.SnapshotExternal, 0, len(o.snapshots))
	for _, s := range o.snapshots {
		snapshots = append(snapshots, s.ConstructExternal())
	}
	sort.Sort(storage.BySnapshotExternalID(snapshots))
	return snapshots, nil
}

func (o *TridentOrchestrator) ListSnapshotsByName(
	ctx context.Context, snapshotName string,
) (snapshots []*storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("snapshot_list_by_snapshot_name", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	snapshots = make([]*storage.SnapshotExternal, 0)
	for _, s := range o.snapshots {
		if s.Config.Name == snapshotName {
			snapshots = append(snapshots, s.ConstructExternal())
		}
	}
	sort.Sort(storage.BySnapshotExternalID(snapshots))
	return snapshots, nil
}

func (o *TridentOrchestrator) ListSnapshotsForVolume(
	ctx context.Context, volumeName string,
) (snapshots []*storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("snapshot_list_by_volume_name", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	if _, ok := o.volumes[volumeName]; !ok {
		return nil, utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}

	snapshots = make([]*storage.SnapshotExternal, 0, len(o.snapshots))
	for _, s := range o.snapshots {
		if s.Config.VolumeName == volumeName {
			snapshots = append(snapshots, s.ConstructExternal())
		}
	}
	sort.Sort(storage.BySnapshotExternalID(snapshots))
	return snapshots, nil
}

func (o *TridentOrchestrator) ReadSnapshotsForVolume(
	ctx context.Context, volumeName string,
) (externalSnapshots []*storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("snapshot_read_by_volume", &err)()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return nil, utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}

	snapshots, err := o.backends[volume.BackendUUID].GetSnapshots(ctx, volume.Config)
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

func (o *TridentOrchestrator) ReloadVolumes(ctx context.Context) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_reload", &err)()

	// Lock out all other workflows while we reload the volumes
	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	// Make a temporary copy of backends in case anything goes wrong
	tempBackends := make(map[string]storage.Backend)
	for k, v := range o.backends {
		tempBackends[k] = v
	}
	// Make a temporary copy of volumes in case anything goes wrong
	tempVolumes := make(map[string]*storage.Volume)
	for k, v := range o.volumes {
		tempVolumes[k] = v
	}

	// Clear out cached volumes in the backends
	for _, backend := range o.backends {
		backend.SetVolumes(make(map[string]*storage.Volume))
	}

	// Re-run the volume bootstrapping code
	err = o.bootstrapVolumes(ctx)

	// If anything went wrong, reinstate the original volumes
	if err != nil {
		Logc(ctx).Errorf("Volume reload failed, restoring original volume list: %v", err)
		o.backends = tempBackends
		o.volumes = tempVolumes
	}

	return err
}

// ResizeVolume resizes a volume to the new size.
func (o *TridentOrchestrator) ResizeVolume(ctx context.Context, volumeName, newSize string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_resize", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	if _, ok := o.subordinateVolumes[volumeName]; ok {
		return o.resizeSubordinateVolume(ctx, volumeName, newSize)
	}

	volume, found := o.volumes[volumeName]
	if !found {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}
	if volume.Orphaned {
		Logc(ctx).WithFields(LogFields{
			"volume":      volumeName,
			"backendUUID": volume.BackendUUID,
		}).Warnf("Resize operation is likely to fail with an orphaned volume.")
	}
	if volume.State.IsDeleting() {
		return utils.VolumeStateError(fmt.Sprintf("volume %s is deleting", volumeName))
	}

	// Create a new config for the volume transaction
	cloneConfig := volume.Config.ConstructClone()
	cloneConfig.Size = newSize

	// Add a transaction in case the operation must be retried during bootstraping.
	volTxn := &storage.VolumeTransaction{
		Config: cloneConfig,
		Op:     storage.ResizeVolume,
	}
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

// resizeVolume does the necessary work to resize a volume. It doesn't
// construct a transaction, nor does it take locks; it assumes that the
// caller will take care of both of these. It also assumes that the volume
// exists in memory.
func (o *TridentOrchestrator) resizeVolume(ctx context.Context, volume *storage.Volume, newSize string) error {
	volumeBackend, found := o.backends[volume.BackendUUID]
	if !found {
		Logc(ctx).WithFields(LogFields{
			"volume":      volume.Config.Name,
			"backendUUID": volume.BackendUUID,
		}).Error("Unable to find backend during volume resize.")
		return fmt.Errorf("unable to find backend %v during volume resize", volume.BackendUUID)
	}

	if volume.Config.Size != newSize {
		// If the resize is successful the driver updates the volume.Config.Size, as a side effect, with the actual
		// byte size of the expanded volume.
		if err := volumeBackend.ResizeVolume(ctx, volume.Config, newSize); err != nil {
			Logc(ctx).WithFields(LogFields{
				"volume":          volume.Config.Name,
				"volume_internal": volume.Config.InternalName,
				"backendUUID":     volume.BackendUUID,
				"current_size":    volume.Config.Size,
				"new_size":        newSize,
				"error":           err,
			}).Error("Unable to resize the volume.")
			return fmt.Errorf("unable to resize the volume: %v", err)
		}
	}

	if err := o.updateVolumeOnPersistentStore(ctx, volume); err != nil {
		// It's ok not to revert volume size as we don't clean up the
		// transaction object in this situation.
		Logc(ctx).WithFields(LogFields{
			"volume": volume.Config.Name,
		}).Error("Unable to update the volume's size in persistent store.")
		return err
	}
	return nil
}

// resizeVolumeCleanup is used to clean up artifacts of volume resize in case
// anything goes wrong during the operation.
func (o *TridentOrchestrator) resizeVolumeCleanup(
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
		}
		errList := make([]string, 0, 2)
		for _, e := range []error{err, txErr} {
			if e != nil {
				errList = append(errList, e.Error())
			}
		}
		if len(errList) > 0 {
			err = fmt.Errorf(strings.Join(errList, ", "))
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

// getProtocol returns the appropriate protocol based on a specified volume mode, access mode and protocol, or
// an error if the two settings are incompatible.
// NOTE: 1. DO NOT ALLOW ROX and RWX for block on file
// 2. 'BlockOnFile' protocol set via the PVC annotation with multi-node access mode
// 3. We do not support raw block volumes with block on file or NFS protocol.
func (o *TridentOrchestrator) getProtocol(
	ctx context.Context, volumeMode config.VolumeMode, accessMode config.AccessMode, protocol config.Protocol,
) (config.Protocol, error) {
	Logc(ctx).WithFields(LogFields{
		"volumeMode": volumeMode,
		"accessMode": accessMode,
		"protocol":   protocol,
	}).Debug("Orchestrator#getProtocol")

	resultProtocol := protocol
	var err error = nil

	defer Logc(ctx).WithFields(LogFields{
		"resultProtocol": resultProtocol,
		"err":            err,
	}).Debug("Orchestrator#getProtocol")

	type accessVariables struct {
		volumeMode config.VolumeMode
		accessMode config.AccessMode
		protocol   config.Protocol
	}
	type protocolResult struct {
		protocol config.Protocol
		err      error
	}

	err = fmt.Errorf("incompatible volume mode (%s), access mode (%s) and protocol (%s)", volumeMode, accessMode,
		protocol)

	protocolTable := map[accessVariables]protocolResult{
		{config.Filesystem, config.ModeAny, config.ProtocolAny}: {config.ProtocolAny, nil},
		{config.Filesystem, config.ModeAny, config.File}:        {config.File, nil},
		{config.Filesystem, config.ModeAny, config.Block}:       {config.Block, nil},
		{config.Filesystem, config.ModeAny, config.BlockOnFile}: {config.BlockOnFile, nil},

		{config.Filesystem, config.ReadWriteOnce, config.ProtocolAny}: {config.ProtocolAny, nil},
		{config.Filesystem, config.ReadWriteOnce, config.File}:        {config.File, nil},
		{config.Filesystem, config.ReadWriteOnce, config.Block}:       {config.Block, nil},
		{config.Filesystem, config.ReadWriteOnce, config.BlockOnFile}: {config.BlockOnFile, nil},

		{config.Filesystem, config.ReadWriteOncePod, config.ProtocolAny}: {config.ProtocolAny, nil},
		{config.Filesystem, config.ReadWriteOncePod, config.File}:        {config.File, nil},
		{config.Filesystem, config.ReadWriteOncePod, config.Block}:       {config.Block, nil},
		{config.Filesystem, config.ReadWriteOncePod, config.BlockOnFile}: {config.BlockOnFile, nil},

		{config.Filesystem, config.ReadOnlyMany, config.ProtocolAny}: {config.ProtocolAny, nil},
		{config.Filesystem, config.ReadOnlyMany, config.File}:        {config.File, nil},
		{config.Filesystem, config.ReadOnlyMany, config.Block}:       {config.Block, nil},
		{config.Filesystem, config.ReadOnlyMany, config.BlockOnFile}: {config.ProtocolAny, err},

		{config.Filesystem, config.ReadWriteMany, config.ProtocolAny}: {config.File, nil},
		{config.Filesystem, config.ReadWriteMany, config.File}:        {config.File, nil},
		{config.Filesystem, config.ReadWriteMany, config.Block}:       {config.ProtocolAny, err},
		{config.Filesystem, config.ReadWriteMany, config.BlockOnFile}: {config.ProtocolAny, err},

		{config.RawBlock, config.ModeAny, config.ProtocolAny}: {config.Block, nil},
		{config.RawBlock, config.ModeAny, config.File}:        {config.ProtocolAny, err},
		{config.RawBlock, config.ModeAny, config.Block}:       {config.Block, nil},
		{config.RawBlock, config.ModeAny, config.BlockOnFile}: {config.ProtocolAny, err},

		{config.RawBlock, config.ReadWriteOnce, config.ProtocolAny}: {config.Block, nil},
		{config.RawBlock, config.ReadWriteOnce, config.File}:        {config.ProtocolAny, err},
		{config.RawBlock, config.ReadWriteOnce, config.Block}:       {config.Block, nil},
		{config.RawBlock, config.ReadWriteOnce, config.BlockOnFile}: {config.ProtocolAny, err},

		{config.RawBlock, config.ReadWriteOncePod, config.ProtocolAny}: {config.Block, nil},
		{config.RawBlock, config.ReadWriteOncePod, config.File}:        {config.ProtocolAny, err},
		{config.RawBlock, config.ReadWriteOncePod, config.Block}:       {config.Block, nil},
		{config.RawBlock, config.ReadWriteOncePod, config.BlockOnFile}: {config.ProtocolAny, err},

		{config.RawBlock, config.ReadOnlyMany, config.ProtocolAny}: {config.Block, nil},
		{config.RawBlock, config.ReadOnlyMany, config.File}:        {config.ProtocolAny, err},
		{config.RawBlock, config.ReadOnlyMany, config.Block}:       {config.Block, nil},
		{config.RawBlock, config.ReadOnlyMany, config.BlockOnFile}: {config.ProtocolAny, err},

		{config.RawBlock, config.ReadWriteMany, config.ProtocolAny}: {config.Block, nil},
		{config.RawBlock, config.ReadWriteMany, config.File}:        {config.ProtocolAny, err},
		{config.RawBlock, config.ReadWriteMany, config.Block}:       {config.Block, nil},
		{config.RawBlock, config.ReadWriteMany, config.BlockOnFile}: {config.ProtocolAny, err},
	}

	res, isValid := protocolTable[accessVariables{volumeMode, accessMode, protocol}]

	if !isValid {
		return config.ProtocolAny, fmt.Errorf("invalid volume mode (%s), access mode (%s) or protocol (%s)", volumeMode,
			accessMode, protocol)
	}

	return res.protocol, res.err
}

func (o *TridentOrchestrator) AddStorageClass(
	ctx context.Context, scConfig *storageclass.Config,
) (scExternal *storageclass.External, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("storageclass_add", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	sc := storageclass.New(scConfig)
	if _, ok := o.storageClasses[sc.GetName()]; ok {
		return nil, fmt.Errorf("storage class %s already exists", sc.GetName())
	}
	err = o.storeClient.AddStorageClass(ctx, sc)
	if err != nil {
		return nil, err
	}
	o.storageClasses[sc.GetName()] = sc
	added := 0
	for _, backend := range o.backends {
		added += sc.CheckAndAddBackend(ctx, backend)
	}
	if added == 0 {
		Logc(ctx).WithFields(LogFields{
			"storageClass": scConfig.Name,
		}).Info("No backends currently satisfy provided storage class.")
	} else {
		Logc(ctx).WithFields(LogFields{
			"storageClass": sc.GetName(),
		}).Infof("Storage class satisfied by %d storage pools.", added)
	}
	return sc.ConstructExternal(ctx), nil
}

func (o *TridentOrchestrator) GetStorageClass(
	ctx context.Context, scName string,
) (scExternal *storageclass.External, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("storageclass_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	sc, found := o.storageClasses[scName]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("storage class %v was not found", scName))
	}
	// Storage classes aren't threadsafe (we modify them during runtime),
	// so return a copy, rather than the original
	return sc.ConstructExternal(ctx), nil
}

func (o *TridentOrchestrator) ListStorageClasses(ctx context.Context) (
	scExternals []*storageclass.External, err error,
) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("storageclass_list", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	storageClasses := make([]*storageclass.External, 0, len(o.storageClasses))
	for _, sc := range o.storageClasses {
		storageClasses = append(storageClasses, sc.ConstructExternal(ctx))
	}
	return storageClasses, nil
}

func (o *TridentOrchestrator) DeleteStorageClass(ctx context.Context, scName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("storageclass_delete", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	sc, found := o.storageClasses[scName]
	if !found {
		return utils.NotFoundError(fmt.Sprintf("storage class %s not found", scName))
	}

	// Note that we don't need a tranasaction here.  If this crashes prior
	// to successful deletion, the storage class will be reloaded upon reboot
	// automatically, which is consistent with the method never having returned
	// successfully.
	err = o.storeClient.DeleteStorageClass(ctx, sc)
	if err != nil {
		return err
	}
	delete(o.storageClasses, scName)
	for _, storagePool := range sc.GetStoragePoolsForProtocol(ctx, config.ProtocolAny, config.ReadWriteOnce) {
		storagePool.RemoveStorageClass(scName)
	}
	return nil
}

func (o *TridentOrchestrator) reconcileNodeAccessOnAllBackends(ctx context.Context) error {
	if config.CurrentDriverContext != config.ContextCSI {
		return nil
	}

	Logc(ctx).Debug("Reconciling node access on current backends.")
	errored := false
	for _, b := range o.backends {
		err := o.reconcileNodeAccessOnBackend(ctx, b)
		if err != nil {
			Logc(ctx).WithError(err).WithField("backend", b.Name()).Warn("Error during node access reconciliation")
			errored = true
		}
	}
	if errored {
		return fmt.Errorf("one or more errors during node access reconciliation")
	}
	return nil
}

func (o *TridentOrchestrator) reconcileNodeAccessOnBackend(ctx context.Context, b storage.Backend) error {
	if config.CurrentDriverContext != config.ContextCSI {
		return nil
	}

	var nodes []*utils.Node
	if b.CanEnablePublishEnforcement() {
		nodes = o.publishedNodesForBackend(b)
	} else {
		nodes = o.nodes.List()
	}
	return b.ReconcileNodeAccess(ctx, nodes, o.uuid)
}

// publishedNodesForBackend returns the nodes that a backend has published volumes to
func (o *TridentOrchestrator) publishedNodesForBackend(b storage.Backend) []*utils.Node {
	pubs := o.volumePublications.ListPublications()
	volumes := b.Volumes()
	m := make(map[string]struct{})

	for _, pub := range pubs {
		if _, ok := volumes[pub.VolumeName]; ok {
			m[pub.NodeName] = struct{}{}
		}
	}

	nodes := make([]*utils.Node, 0, len(m))
	for n := range m {
		if node := o.nodes.Get(n); node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes
}

func (o *TridentOrchestrator) reconcileBackendState(ctx context.Context, b storage.Backend) error {
	if !b.CanGetState() {
		// This backend does not support polling backend for state.
		return nil
	}

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	reason, changeMap := b.GetBackendState(ctx)

	Logc(ctx).WithField("reason", reason).Debug("reconcileBackendState")

	// For now, skip modifying if there is change in pools list.
	if changeMap != nil && changeMap.Contains(storage.BackendStateReasonChange) {
		// Update CR.
		if err := o.storeClient.UpdateBackend(ctx, b); err != nil {
			return err
		}
	}

	return nil
}

// safeReconcileNodeAccessOnBackend wraps reconcileNodeAccessOnBackend in a mutex lock for use in functions that aren't
// already locked
func (o *TridentOrchestrator) safeReconcileNodeAccessOnBackend(ctx context.Context, b storage.Backend) error {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	return o.reconcileNodeAccessOnBackend(ctx, b)
}

// PeriodicallyReconcileNodeAccessOnBackends is intended to be run as a goroutine and will periodically run
// safeReconcileNodeAccessOnBackend for each backend in the orchestrator
func (o *TridentOrchestrator) PeriodicallyReconcileNodeAccessOnBackends() {
	o.stopNodeAccessLoop = make(chan bool)
	ctx := GenerateRequestContext(nil, "", ContextSourcePeriodic, WorkflowCoreNodeReconcile, LogLayerCore)

	Logc(ctx).Info("Starting periodic node access reconciliation service.")
	defer Logc(ctx).Info("Stopping periodic node access reconciliation service.")

	// Every period seconds after the last run
	ticker := time.NewTicker(NodeAccessReconcilePeriod)
	defer ticker.Stop()

	for {
		select {
		case <-o.stopNodeAccessLoop:
			// Exit on shutdown signal
			return

		case <-ticker.C:
			Logc(ctx).Trace("Periodic node access reconciliation loop beginning.")
			// Do nothing if it has not been at least cooldown seconds after the last node registration to prevent thundering herd
			if time.Now().After(o.lastNodeRegistration.Add(NodeRegistrationCooldownPeriod)) {
				for _, backend := range o.backends {
					if err := o.safeReconcileNodeAccessOnBackend(ctx, backend); err != nil {
						// If there's a problem log an error and keep going
						Logc(ctx).WithField("backend", backend.Name()).WithError(err).Error(
							"Problem encountered updating node access rules for backend.")
					}
				}
			} else {
				Logc(ctx).Trace(
					"Time is too soon since last node registration, delaying node access rule reconciliation.")
			}
		}
	}
}

// PeriodicallyReconcileBackendState is intended to be run as a goroutine and will periodically run
// reconcileBackendState for each backend in the orchestrator.
func (o *TridentOrchestrator) PeriodicallyReconcileBackendState(pollInterval time.Duration) {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourcePeriodic, WorkflowCoreBackendReconcile,
		LogLayerCore)

	// Provision to disable reconciling backend state, just in case
	if pollInterval <= 0 {
		Logc(ctx).Debug("Periodic reconciliation of backends is disabled.")
		return
	}

	Logc(ctx).Info("Starting periodic backend state reconciliation service.")
	defer Logc(ctx).Info("Stopping periodic backend state reconciliation service.")

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
			Logc(ctx).Trace("Periodic backend state reconciliation loop beginning.")
			for _, backend := range o.backends {
				if err := o.reconcileBackendState(ctx, backend); err != nil {
					// If there is a problem, log an error and keep going.
					Logc(ctx).WithField("backend", backend.Name()).WithError(err).Errorf(
						"Problem encountered while reconciling state for backend.")
				}
			}
			// reset the timer so that next poll would start after pollInterval.
			reconcileBackendTimer.Reset(pollInterval)
		}
	}
}

func (o *TridentOrchestrator) AddNode(ctx context.Context, node *utils.Node, nodeEventCallback NodeEventCallback) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("node_add", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()
	// Check if node services have changed
	if node.HostInfo != nil && len(node.HostInfo.Services) > 0 {
		nodeEventCallback(controllerhelpers.EventTypeNormal, "TridentServiceDiscovery",
			fmt.Sprintf("%s detected on host.", node.HostInfo.Services))
	}

	// Do not set publication state on existing nodes
	existingNode := o.nodes.Get(node.Name)
	if existingNode != nil {
		node.PublicationState = existingNode.PublicationState
	}

	if err = o.storeClient.AddOrUpdateNode(ctx, node); err != nil {
		return
	}

	o.nodes.Set(node.Name, node)

	o.lastNodeRegistration = time.Now()
	o.invalidateAllBackendNodeAccess()
	return nil
}

func (o *TridentOrchestrator) UpdateNode(
	ctx context.Context, nodeName string, flags *utils.NodePublicationStateFlags,
) (err error) {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	if !o.volumePublicationsSynced {
		return utils.NotReadyError()
	}

	defer recordTiming("node_update", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	node := o.nodes.Get(nodeName)
	if node == nil {
		return utils.NotFoundError(fmt.Sprintf("node %s not found", nodeName))
	}
	oldNode := node.Copy()

	// Update node publication state based on state flags
	if flags != nil {

		logFields := LogFields{
			"node":  nodeName,
			"flags": flags,
		}
		Logc(ctx).WithFields(logFields).WithField("state", node.PublicationState).Trace("Pre-state transition.")

		switch node.PublicationState {
		case utils.NodeClean:
			if flags.IsNodeDirty() {
				node.PublicationState = utils.NodeDirty
				Logc(ctx).WithFields(logFields).Info("Transitioning Node state from Clean to Dirty.")
			}
		case utils.NodeCleanable:
			if flags.IsNodeDirty() {
				node.PublicationState = utils.NodeDirty
				Logc(ctx).WithFields(logFields).Info("Transitioning Node state from Cleanable to Dirty.")
			} else if flags.IsNodeCleaned() {
				node.PublicationState = utils.NodeClean
				Logc(ctx).WithFields(logFields).Info("Transitioning Node state from Cleanable to Clean.")
			}
		case utils.NodeDirty:
			if flags.IsNodeCleanable() {
				node.PublicationState = utils.NodeCleanable
				Logc(ctx).WithFields(logFields).Info("Transitioning Node state from Dirty to Cleanable.")
			}
		}
	}

	// Update the node in the persistent store if anything changed
	if !reflect.DeepEqual(node, oldNode) {
		Logc(ctx).WithFields(LogFields{
			"node":  nodeName,
			"state": node.PublicationState,
		}).Debug("Updating node in persistence layer.")
		o.nodes.Set(nodeName, node)
		err = o.storeClient.AddOrUpdateNode(ctx, node)
	}

	return
}

func (o *TridentOrchestrator) invalidateAllBackendNodeAccess() {
	for _, backend := range o.backends {
		backend.InvalidateNodeAccess()
	}
}

func (o *TridentOrchestrator) GetNode(ctx context.Context, nodeName string) (node *utils.NodeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("node_get", &err)()

	n := o.nodes.Get(nodeName)
	if n == nil {
		Logc(ctx).WithField("node", nodeName).Info(
			"There may exist a networking or DNS issue preventing this node from registering with the" +
				" Trident controller")
		return nil, utils.NotFoundError(fmt.Sprintf("node %v was not found", nodeName))
	}
	return n.ConstructExternal(), nil
}

func (o *TridentOrchestrator) ListNodes(ctx context.Context) (nodes []*utils.NodeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("node_list", &err)()

	internalNodes := o.nodes.List()
	nodes = make([]*utils.NodeExternal, 0, len(internalNodes))
	for _, node := range internalNodes {
		nodes = append(nodes, node.ConstructExternal())
	}
	return nodes, nil
}

func (o *TridentOrchestrator) DeleteNode(ctx context.Context, nodeName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("node_delete", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	node := o.nodes.Get(nodeName)
	if node == nil {
		return utils.NotFoundError(fmt.Sprintf("node %s not found", nodeName))
	}

	// Only retrieves the number of publications for this node.
	publicationCount := len(o.volumePublications.ListPublicationsForNode(nodeName))
	logFields := LogFields{
		"name":         nodeName,
		"publications": publicationCount,
	}

	if publicationCount == 0 {

		// No volumes are published to this node, so delete the CR and update backends that support node-level access.
		Logc(ctx).WithFields(logFields).Debug("No volumes publications remain for node, removing node CR.")

		return o.deleteNode(ctx, nodeName)

	} else {

		// There are still volumes published to this node, so this is a sudden node removal.  Preserve the node CR
		// with a Deleted flag so we can handle the eventual unpublish calls for the affected volumes.
		Logc(ctx).WithFields(logFields).Debug("Volume publications remain for node, marking node CR as deleted.")

		node.Deleted = true
		if err = o.storeClient.AddOrUpdateNode(ctx, node); err != nil {
			return err
		}
	}

	return nil
}

func (o *TridentOrchestrator) deleteNode(ctx context.Context, nodeName string) (err error) {
	node := o.nodes.Get(nodeName)
	if node == nil {
		return utils.NotFoundError(fmt.Sprintf("node %s not found", nodeName))
	}

	if err = o.storeClient.DeleteNode(ctx, node); err != nil {
		return err
	}
	o.nodes.Delete(nodeName)
	o.invalidateAllBackendNodeAccess()
	return o.reconcileNodeAccessOnAllBackends(ctx)
}

// ReconcileVolumePublications takes a set of attached legacy volumes (as external volume publications) and
// reconciles them with Trident volumes and publication records.
//
//	Pre-conditions:
//	1. CO helpers are alive and have an up-to-date record of attached volumes.
//	2. Bootstrapping has bootstrapped all Trident volumes and known publications from CO persistence.
//	3. [0,n] legacy volumes are supplied.
func (o *TridentOrchestrator) ReconcileVolumePublications(
	ctx context.Context, attachedLegacyVolumes []*utils.VolumePublicationExternal,
) (reconcileErr error) {
	Logc(ctx).Debug(">>>>>> ReconcileVolumePublications")
	defer Logc(ctx).Debug("<<<<<< ReconcileVolumePublications")
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("reconcile_legacy_vol_pubs", &reconcileErr)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	for _, attachedLegacyVolume := range attachedLegacyVolumes {
		volumeName := attachedLegacyVolume.VolumeName
		nodeName := attachedLegacyVolume.NodeName
		fields := LogFields{
			"volume": volumeName,
			"node":   nodeName,
		}

		if publication, ok := o.volumePublications.TryGet(volumeName, nodeName); ok {
			fields["publication"] = publication.Name
			Logc(ctx).WithFields(fields).Debug("Trident publication already exists for legacy volume; ignoring.")
			continue
		}

		publication := &utils.VolumePublication{
			Name:       attachedLegacyVolume.Name,
			NodeName:   nodeName,
			VolumeName: volumeName,
			ReadOnly:   attachedLegacyVolume.ReadOnly,
			AccessMode: attachedLegacyVolume.AccessMode,
		}
		if err := o.addVolumePublication(ctx, publication); err != nil {
			Logc(ctx).WithFields(fields).Error("Failed to create Trident publication for attached legacy volume.")
			reconcileErr = multierr.Append(reconcileErr, err)
			continue
		}
		Logc(ctx).WithFields(fields).Debug("Added new publication record for attached legacy volume.")
	}

	if reconcileErr != nil {
		Logc(ctx).WithError(reconcileErr).Debug("Failed to sync volume publications.")
		return fmt.Errorf("failed to sync volume publications; %v", reconcileErr)
	}

	return o.setPublicationsSynced(ctx, true)
}

// addVolumePublication adds a volume publication to the persistent store and sets the value on the cache.
func (o *TridentOrchestrator) addVolumePublication(
	ctx context.Context, publication *utils.VolumePublication,
) error {
	Logc(ctx).WithFields(LogFields{
		"volumeName": publication.VolumeName,
		"nodeName":   publication.NodeName,
	}).Debug(">>>>>> Orchestrator#addVolumePublication")
	defer Logc(ctx).Debug("<<<<<< Orchestrator#addVolumePublication")

	if err := o.storeClient.AddVolumePublication(ctx, publication); err != nil {
		if !persistentstore.IsAlreadyExistsError(err) {
			return err
		}
		Logc(ctx).WithError(err).Debug("Volume publication already exists, adding to cache.")
	}

	return o.volumePublications.Set(publication.VolumeName, publication.NodeName, publication)
}

// GetVolumePublication returns the volume publication for a given volume/node pair
func (o *TridentOrchestrator) GetVolumePublication(
	ctx context.Context, volumeName, nodeName string,
) (publication *utils.VolumePublication, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_get", &err)()

	volumePublication, found := o.volumePublications.TryGet(volumeName, nodeName)
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("volume publication %v was not found",
			utils.GenerateVolumePublishName(volumeName, nodeName)))
	}
	return volumePublication, nil
}

// ListVolumePublications returns a list of all volume publications.
func (o *TridentOrchestrator) ListVolumePublications(
	ctx context.Context,
) (publications []*utils.VolumePublicationExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	Logc(ctx).Debug(">>>>>> ListVolumePublications")
	defer Logc(ctx).Debug("<<<<<< ListVolumePublications")

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_list", &err)()

	// Get all publications as a list.
	internalPubs := o.volumePublications.ListPublications()
	publications = make([]*utils.VolumePublicationExternal, 0, len(internalPubs))
	for _, pub := range internalPubs {
		publications = append(publications, pub.ConstructExternal())
	}
	return publications, nil
}

// ListVolumePublicationsForVolume returns a list of all volume publications for a given volume.
func (o *TridentOrchestrator) ListVolumePublicationsForVolume(
	ctx context.Context, volumeName string,
) (publications []*utils.VolumePublicationExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	fields := LogFields{"volumeName": volumeName}
	Logc(ctx).WithFields(fields).Trace(">>>> ListVolumePublicationsForVolume")
	defer Logc(ctx).Trace("<<<< ListVolumePublicationsForVolume")

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_list_for_vol", &err)()

	// Get all publications for a volume as a list.
	internalPubs := o.volumePublications.ListPublicationsForVolume(volumeName)
	publications = make([]*utils.VolumePublicationExternal, 0, len(internalPubs))
	for _, pub := range internalPubs {
		publications = append(publications, pub.ConstructExternal())
	}
	return
}

// Change the signature of the func here to return error.
func (o *TridentOrchestrator) listVolumePublicationsForVolumeAndSubordinates(
	_ context.Context, volumeName string,
) (publications []*utils.VolumePublication) {
	publications = []*utils.VolumePublication{}

	// Get volume
	volume, ok := o.volumes[volumeName]
	if !ok {
		if volume, ok = o.subordinateVolumes[volumeName]; !ok {
			return publications
		}
	}

	// If this is a subordinate volume, start with the source volume instead
	if volume.IsSubordinate() {
		if volume, ok = o.volumes[volume.Config.ShareSourceVolume]; !ok {
			return publications
		}
	}

	// Build a slice containing the volume and all of its subordinates
	allVolumes := []*storage.Volume{volume}
	for subordinateVolumeName := range volume.Config.SubordinateVolumes {
		if subordinateVolume, ok := o.subordinateVolumes[subordinateVolumeName]; ok {
			allVolumes = append(allVolumes, subordinateVolume)
		}
	}

	volumePublications := o.volumePublications.Map()
	for _, v := range allVolumes {
		if nodeToPublications, found := volumePublications[v.Config.Name]; found {
			for _, pub := range nodeToPublications {
				publications = append(publications, pub)
			}
		}
	}
	return publications
}

// ListVolumePublicationsForNode returns a list of all volume publications for a given node.
func (o *TridentOrchestrator) ListVolumePublicationsForNode(
	ctx context.Context, nodeName string,
) (publications []*utils.VolumePublicationExternal, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	if !o.volumePublicationsSynced {
		return nil, utils.NotReadyError()
	}
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	fields := LogFields{"nodeName": nodeName}
	Logc(ctx).WithFields(fields).Debug(">>>>>> ListVolumePublicationsForNode")
	defer Logc(ctx).Debug("<<<<<< ListVolumePublicationsForNode")

	defer recordTiming("vol_pub_list_for_node", &err)()

	// Retrieve only publications on the node.
	internalPubs := o.volumePublications.ListPublicationsForNode(nodeName)
	publications = make([]*utils.VolumePublicationExternal, 0, len(internalPubs))
	for _, pub := range internalPubs {
		publications = append(publications, pub.ConstructExternal())
	}
	return
}

// DeleteVolumePublication deletes the record of the volume publication for a given volume/node pair
func (o *TridentOrchestrator) DeleteVolumePublication(ctx context.Context, volumeName, nodeName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("vol_pub_delete", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	return o.deleteVolumePublication(ctx, volumeName, nodeName)
}

func (o *TridentOrchestrator) deleteVolumePublication(ctx context.Context, volumeName, nodeName string) (err error) {
	fields := LogFields{
		"volumeName": volumeName,
		"nodeName":   nodeName,
	}
	Logc(ctx).WithFields(fields).Debug(">>>>>> Orchestrator#deleteVolumePublication")
	defer Logc(ctx).Debug("<<<<<< Orchestrator#deleteVolumePublication")

	publication, found := o.volumePublications.TryGet(volumeName, nodeName)
	if !found {
		Logc(ctx).WithFields(fields).Debug("No volume publication found in the cache.")
		return nil
	}

	// DeleteVolumePublication is idempotent.
	if err = o.storeClient.DeleteVolumePublication(ctx, publication); err != nil {
		if !utils.IsNotFoundError(err) {
			return err
		}
	}

	if err = o.volumePublications.Delete(volumeName, nodeName); err != nil {
		return fmt.Errorf("unable to remove publication from cache; %v", err)
	}

	node := o.nodes.Get(nodeName)
	if node == nil {
		return utils.NotFoundError(fmt.Sprintf("node %s not found", nodeName))
	}

	// Check for publications remaining for just the node.
	nodePubFound := len(o.volumePublications.ListPublicationsForNode(nodeName)) > 0

	// If the node is known to be gone, and if we just unpublished the last volume on that node, delete the node.
	if !nodePubFound && node.Deleted {
		return o.deleteNode(ctx, nodeName)
	}
	return nil
}

func (o *TridentOrchestrator) updateBackendOnPersistentStore(
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

func (o *TridentOrchestrator) updateVolumeOnPersistentStore(ctx context.Context, vol *storage.Volume) error {
	// Update the volume information in persistent store
	Logc(ctx).WithFields(LogFields{
		"volume":          vol.Config.Name,
		"volume_orphaned": vol.Orphaned,
		"volume_size":     vol.Config.Size,
		"volumeState":     string(vol.State),
	}).Debug("Updating an existing volume.")
	return o.storeClient.UpdateVolume(ctx, vol)
}

func (o *TridentOrchestrator) replaceBackendAndUpdateVolumesOnPersistentStore(
	ctx context.Context, origBackend, newBackend storage.Backend,
) error {
	// Update both backend and volume information in persistent store
	Logc(ctx).WithFields(LogFields{
		"newBackendName": newBackend.Name(),
		"oldBackendName": origBackend.Name(),
	}).Debug("Renaming a backend and updating volumes.")
	return o.storeClient.ReplaceBackendAndUpdateVolumes(ctx, origBackend, newBackend)
}

func (o *TridentOrchestrator) isCRDContext(ctx context.Context) bool {
	ctxSource := ctx.Value(ContextKeyRequestSource)
	return ctxSource != nil && ctxSource == ContextSourceCRD
}

// EstablishMirror creates a net-new replication mirror relationship between 2 volumes on a backend
func (o *TridentOrchestrator) EstablishMirror(ctx context.Context, backendUUID, localInternalVolumeName, remoteVolumeHandle, replicationPolicy, replicationSchedule string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("mirror_establish", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	backend, err := o.getBackendByBackendUUID(backendUUID)
	if err != nil {
		return err
	}
	mirrorBackend, ok := backend.(storage.Mirrorer)
	if !ok {
		return fmt.Errorf("backend does not support mirroring")
	}
	return mirrorBackend.EstablishMirror(ctx, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
		replicationSchedule)
}

// ReestablishMirror recreates a previously existing replication mirror relationship between 2 volumes on a backend
func (o *TridentOrchestrator) ReestablishMirror(ctx context.Context, backendUUID, localInternalVolumeName, remoteVolumeHandle, replicationPolicy, replicationSchedule string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("mirror_reestablish", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	backend, err := o.getBackendByBackendUUID(backendUUID)
	if err != nil {
		return err
	}
	mirrorBackend, ok := backend.(storage.Mirrorer)
	if !ok {
		return fmt.Errorf("backend does not support mirroring")
	}
	return mirrorBackend.ReestablishMirror(ctx, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
		replicationSchedule)
}

// PromoteMirror makes the local volume the primary
func (o *TridentOrchestrator) PromoteMirror(ctx context.Context, backendUUID, localInternalVolumeName, remoteVolumeHandle, snapshotHandle string) (waitingForSnapshot bool, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return false, o.bootstrapError
	}
	defer recordTiming("mirror_promote", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	backend, err := o.getBackendByBackendUUID(backendUUID)
	if err != nil {
		return false, err
	}
	mirrorBackend, ok := backend.(storage.Mirrorer)
	if !ok {
		return false, fmt.Errorf("backend does not support mirroring")
	}
	return mirrorBackend.PromoteMirror(ctx, localInternalVolumeName, remoteVolumeHandle, snapshotHandle)
}

// GetMirrorStatus returns the current status of the mirror relationship
func (o *TridentOrchestrator) GetMirrorStatus(ctx context.Context, backendUUID, localInternalVolumeName, remoteVolumeHandle string) (status string, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return "", o.bootstrapError
	}
	defer recordTiming("mirror_status", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	backend, err := o.getBackendByBackendUUID(backendUUID)
	if err != nil {
		return "", err
	}
	mirrorBackend, ok := backend.(storage.Mirrorer)
	if !ok {
		return "", fmt.Errorf("backend does not support mirroring")
	}
	return mirrorBackend.GetMirrorStatus(ctx, localInternalVolumeName, remoteVolumeHandle)
}

func (o *TridentOrchestrator) CanBackendMirror(ctx context.Context, backendUUID string) (capable bool, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return false, o.bootstrapError
	}
	defer recordTiming("mirror_capable", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	backend, err := o.getBackendByBackendUUID(backendUUID)
	if err != nil {
		return false, err
	}
	return backend.CanMirror(), nil
}

// ReleaseMirror removes snapmirror relationship infromation and snapshots for a source volume in ONTAP
func (o *TridentOrchestrator) ReleaseMirror(ctx context.Context, backendUUID, localInternalVolumeName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("mirror_release", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	backend, err := o.getBackendByBackendUUID(backendUUID)
	if err != nil {
		return err
	}
	mirrorBackend, ok := backend.(storage.Mirrorer)
	if !ok {
		return fmt.Errorf("backend does not support mirroring")
	}
	return mirrorBackend.ReleaseMirror(ctx, localInternalVolumeName)
}

// GetReplicationDetails returns the current replication policy and schedule of a relationship and the SVM name and UUID
func (o *TridentOrchestrator) GetReplicationDetails(ctx context.Context, backendUUID, localInternalVolumeName, remoteVolumeHandle string) (policy, schedule, SVMName string, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return "", "", "", o.bootstrapError
	}
	defer recordTiming("replication_details", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	backend, err := o.getBackendByBackendUUID(backendUUID)
	if err != nil {
		return "", "", "", err
	}
	mirrorBackend, ok := backend.(storage.Mirrorer)
	if !ok {
		return "", "", "", fmt.Errorf("backend does not support mirroring")
	}
	return mirrorBackend.GetReplicationDetails(ctx, localInternalVolumeName, remoteVolumeHandle)
}

func (o *TridentOrchestrator) GetCHAP(
	ctx context.Context, volumeName, nodeName string,
) (chapInfo *utils.IscsiChapInfo, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("get_chap", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()

	volume, err := o.getVolume(ctx, volumeName)
	if err != nil {
		return nil, err
	}

	backend, err := o.getBackendByBackendUUID(volume.BackendUUID)
	if err != nil {
		return nil, err
	}

	return backend.GetChapInfo(ctx, volumeName, nodeName)
}

/******************************************************************************
REST API Handlers for retrieving and setting the current logging configuration.
******************************************************************************/

func (o *TridentOrchestrator) GetLogLevel(ctx context.Context) (string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return "", o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return GetDefaultLogLevel(), nil
}

func (o *TridentOrchestrator) SetLogLevel(ctx context.Context, level string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	if err := SetDefaultLogLevel(level); err != nil {
		return err
	}

	return nil
}

func (o *TridentOrchestrator) GetSelectedLoggingWorkflows(ctx context.Context) (string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return "", o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return GetSelectedWorkFlows(), nil
}

func (o *TridentOrchestrator) ListLoggingWorkflows(ctx context.Context) ([]string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return []string{}, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return ListWorkflowTypes(), nil
}

func (o *TridentOrchestrator) SetLoggingWorkflows(ctx context.Context, flows string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	err := SetWorkflows(flows)
	if err != nil {
		return err
	}

	return nil
}

func (o *TridentOrchestrator) GetSelectedLogLayers(ctx context.Context) (string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return "", o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return GetSelectedLogLayers(), nil
}

func (o *TridentOrchestrator) ListLogLayers(ctx context.Context) ([]string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return []string{}, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return ListLogLayers(), nil
}

func (o *TridentOrchestrator) SetLogLayers(ctx context.Context, layers string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	err := SetLogLayers(layers)
	if err != nil {
		return err
	}

	return nil
}
