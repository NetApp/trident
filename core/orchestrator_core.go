// Copyright 2021 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"go.uber.org/multierr"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/csi/helpers"
	. "github.com/netapp/trident/logger"
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
)

// recordTiming is used to record in Prometheus the total time taken for an operation as follows:
//   defer recordTiming("backend_add")()
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
	backends             map[string]storage.Backend // key is UUID, not name
	volumes              map[string]*storage.Volume
	frontends            map[string]frontend.Plugin
	mutex                *sync.Mutex
	storageClasses       map[string]*storageclass.StorageClass
	nodes                map[string]*utils.Node
	volumePublications   map[string]map[string]*utils.VolumePublication
	snapshots            map[string]*storage.Snapshot
	storeClient          persistentstore.Client
	bootstrapped         bool
	bootstrapError       error
	txnMonitorTicker     *time.Ticker
	txnMonitorChannel    chan struct{}
	txnMonitorStopped    bool
	lastNodeRegistration time.Time
	stopNodeAccessLoop   chan bool
}

// NewTridentOrchestrator returns a storage orchestrator instance
func NewTridentOrchestrator(client persistentstore.Client) *TridentOrchestrator {
	return &TridentOrchestrator{
		backends:           make(map[string]storage.Backend), // key is UUID, not name
		volumes:            make(map[string]*storage.Volume),
		frontends:          make(map[string]frontend.Plugin),
		storageClasses:     make(map[string]*storageclass.StorageClass),
		nodes:              make(map[string]*utils.Node),
		volumePublications: make(map[string]map[string]*utils.VolumePublication),
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
		}
		Logc(ctx).WithFields(log.Fields{
			"PersistentStoreVersion": version.PersistentStoreVersion,
			"OrchestratorAPIVersion": version.OrchestratorAPIVersion,
		}).Warning("Persistent state version not found, creating.")
	} else if err != nil {
		return fmt.Errorf("couldn't determine the orchestrator persistent state version: %v", err)
	}

	if config.OrchestratorAPIVersion != version.OrchestratorAPIVersion {
		Logc(ctx).WithFields(log.Fields{
			"current_api_version": version.OrchestratorAPIVersion,
			"desired_api_version": config.OrchestratorAPIVersion,
		}).Info("Transforming Trident API objects on the persistent store.")
		// TODO: transform Trident API objects
	}

	// Store the persistent store and API versions
	version.PersistentStoreVersion = string(o.storeClient.GetType())
	version.OrchestratorAPIVersion = config.OrchestratorAPIVersion
	if err = o.storeClient.SetVersion(ctx, version); err != nil {
		return fmt.Errorf("failed to set the persistent state version after migration: %v", err)
	}
	return nil
}

func (o *TridentOrchestrator) Bootstrap() error {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceInternal)
	var err error

	if len(o.frontends) == 0 {
		log.Warning("Trident is bootstrapping with no frontend.")
	}

	// Transform persistent state, if necessary
	if err = o.transformPersistentState(ctx); err != nil {
		o.bootstrapError = utils.BootstrapError(err)
		return o.bootstrapError
	}

	// Bootstrap state from persistent store
	if err = o.bootstrap(ctx); err != nil {
		o.bootstrapError = utils.BootstrapError(err)
		return o.bootstrapError
	}

	// Start transaction monitor
	o.StartTransactionMonitor(ctx, txnMonitorPeriod, txnMonitorMaxAge)

	o.bootstrapped = true
	o.bootstrapError = nil
	log.Infof("%s bootstrapped successfully.", strings.Title(config.OrchestratorName))
	return nil
}

func (o *TridentOrchestrator) bootstrapBackends(ctx context.Context) error {

	persistentBackends, err := o.storeClient.GetBackends(ctx)
	if err != nil {
		return err
	}

	for _, b := range persistentBackends {
		Logc(ctx).WithFields(log.Fields{
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

			errorLogFields := log.Fields{
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

				Logc(ctx).WithFields(log.Fields{
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
			Logc(ctx).WithFields(log.Fields{
				"backend":                        newBackend.Name(),
				"backendUUID":                    newBackend.BackendUUID(),
				"configRef":                      newBackend.ConfigRef(),
				"persistentBackends.BackendUUID": b.BackendUUID,
				"online":                         newBackend.Online(),
				"state":                          newBackend.State(),
				"handler":                        "Bootstrap",
			}).Info("Added an existing backend.")
		} else {
			Logc(ctx).WithFields(log.Fields{
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
		Logc(ctx).WithFields(log.Fields{
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
		vol := storage.NewVolume(v.Config, v.BackendUUID, v.Pool, v.Orphaned)
		o.volumes[vol.Config.Name] = vol

		if backend, ok = o.backends[v.BackendUUID]; !ok {
			Logc(ctx).WithFields(log.Fields{
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

		Logc(ctx).WithFields(log.Fields{
			"volume":       vol.Config.Name,
			"internalName": vol.Config.InternalName,
			"size":         vol.Config.Size,
			"backendUUID":  vol.BackendUUID,
			"pool":         vol.Pool,
			"orphaned":     vol.Orphaned,
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

		Logc(ctx).WithFields(log.Fields{
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
		Logc(ctx).WithFields(log.Fields{
			"node":    n.Name,
			"handler": "Bootstrap",
		}).Info("Added an existing node.")
		o.nodes[n.Name] = n
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
		Logc(ctx).WithFields(log.Fields{
			"volume":  vp.VolumeName,
			"node":    vp.NodeName,
			"handler": "Bootstrap",
		}).Info("Added an existing volume publication.")
		o.addVolumePublicationToCache(vp)
	}
	return nil
}

func (o *TridentOrchestrator) addVolumePublicationToCache(vp *utils.VolumePublication) {
	// If the volume has no entry we need to initialize the inner map
	if o.volumePublications[vp.VolumeName] == nil {
		o.volumePublications[vp.VolumeName] = map[string]*utils.VolumePublication{}
	}
	o.volumePublications[vp.VolumeName][vp.NodeName] = vp
}

func (o *TridentOrchestrator) bootstrap(ctx context.Context) error {

	// Fetching backend information

	type bootstrapFunc func(context.Context) error
	for _, f := range []bootstrapFunc{
		o.bootstrapBackends, o.bootstrapStorageClasses, o.bootstrapVolumes,
		o.bootstrapSnapshots, o.bootstrapVolTxns, o.bootstrapNodes, o.bootstrapVolumePublications,
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

	// If nothing failed during bootstrapping, initialize the core metrics
	o.updateMetrics()

	return nil
}

// Stop stops the orchestrator core.
func (o *TridentOrchestrator) Stop() {

	// Stop the node access reconciliation background task
	if o.stopNodeAccessLoop != nil {
		o.stopNodeAccessLoop <- true
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
		backendsGauge.WithLabelValues(backend.GetDriverName(), backend.State().String()).Inc()
		tridentBackendInfo.WithLabelValues(backend.GetDriverName(), backend.Name(),
			backend.BackendUUID()).Set(float64(1))
	}

	volumesGauge.Reset()
	volumesTotalBytes := float64(0)
	volumeAllocatedBytesGauge.Reset()
	for _, volume := range o.volumes {
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
	nodeGauge.Set(float64(len(o.nodes)))
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

	switch v.Op {
	case storage.AddVolume, storage.DeleteVolume,
		storage.ImportVolume, storage.ResizeVolume:
		Logc(ctx).WithFields(log.Fields{
			"volume":       v.Config.Name,
			"size":         v.Config.Size,
			"storageClass": v.Config.StorageClass,
			"op":           v.Op,
		}).Info("Processed volume transaction log.")
	case storage.AddSnapshot, storage.DeleteSnapshot:
		Logc(ctx).WithFields(log.Fields{
			"volume":   v.SnapshotConfig.VolumeName,
			"snapshot": v.SnapshotConfig.Name,
			"op":       v.Op,
		}).Info("Processed snapshot transaction log.")
	case storage.UpgradeVolume:
		Logc(ctx).WithFields(log.Fields{
			"volume": v.Config.Name,
			"PVC":    v.PVUpgradeConfig.PVCConfig.Name,
			"PV":     v.PVUpgradeConfig.PVConfig.Name,
			"op":     v.Op,
		}).Info("Processed volume upgrade transaction log.")
	case storage.VolumeCreating:
		Logc(ctx).WithFields(log.Fields{
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
				Logc(ctx).WithFields(log.Fields{
					"volume": v.Config.Name,
					"error":  err,
				}).Errorf("Unable to finalize deletion of the volume! Repeat deleting the volume using %s.",
					config.OrchestratorClientName)
			}
		} else {
			Logc(ctx).WithFields(log.Fields{
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

		logFields := log.Fields{"volume": v.SnapshotConfig.VolumeName, "snapshot": v.SnapshotConfig.Name}

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
				Logc(ctx).WithFields(log.Fields{
					"volume": v.Config.Name,
					"error":  err,
				}).Error("Unable to resize the volume! Repeat resizing the volume.")
			} else {
				Logc(ctx).WithFields(log.Fields{
					"volume":      vol.Config.Name,
					"volume_size": v.Config.Size,
				}).Info("Orchestrator resized the volume on the storage backend.")
			}
		} else {
			Logc(ctx).WithFields(log.Fields{
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
			if err := o.deleteVolumeFromPersistentStoreIgnoreError(ctx, volume); err != nil {
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

func (o *TridentOrchestrator) AddFrontend(f frontend.Plugin) {
	name := f.GetName()
	if _, ok := o.frontends[name]; ok {
		log.WithField("name", name).Warn("Adding frontend already present.")
		return
	}
	log.WithField("name", name).Info("Added frontend.")
	o.frontends[name] = f
}

func (o *TridentOrchestrator) GetFrontend(ctx context.Context, name string) (frontend.Plugin, error) {

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

func (o *TridentOrchestrator) GetVersion(context.Context) (string, error) {
	return config.OrchestratorVersion.String(), o.bootstrapError
}

// AddBackend handles creation of a new storage backend
func (o *TridentOrchestrator) AddBackend(
	ctx context.Context, configJSON, configRef string,
) (backendExternal *storage.BackendExternal, err error) {
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
		Logc(ctx).WithFields(log.Fields{
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
			Logc(ctx).WithFields(log.Fields{
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
	Logc(ctx).WithFields(log.Fields{
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
		Logc(ctx).WithFields(log.Fields{
			"backend": backend.Name(),
		}).Info("Newly added backend satisfies no storage classes.")
	} else {
		Logc(ctx).WithFields(log.Fields{
			"backend": backend.Name(),
		}).Infof("Newly added backend satisfies storage classes %s.", strings.Join(classes, ", "))
	}

	return backend.ConstructExternal(ctx), nil
}

// validateAndCreateBackendFromConfig validates config and creates backend based on Config
func (o *TridentOrchestrator) validateAndCreateBackendFromConfig(
	ctx context.Context, configJSON string, configRef string, backendUUID string,
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

	var (
		backend storage.Backend
	)

	// Check whether the backend exists.
	originalBackend, found := o.backends[backendUUID]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("backend %v was not found", backendUUID))
	}

	logFields := log.Fields{"backendName": backendName, "backendUUID": backendUUID, "configJSON": "<suppressed>"}

	Logc(ctx).WithFields(log.Fields{
		"originalBackend.Name":        originalBackend.Name(),
		"originalBackend.BackendUUID": originalBackend.BackendUUID(),
		"originalBackend.ConfigRef":   originalBackend.ConfigRef(),
		"GetExternalConfig":           originalBackend.Driver().GetExternalConfig(ctx),
	}).Debug("found original backend")

	originalConfigRef := originalBackend.ConfigRef()

	// Do not allow update of TridentBackendConfig-based backends using tridentctl
	if originalConfigRef != "" {
		if !o.isCRDContext(ctx) {
			Logc(ctx).WithFields(log.Fields{
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
			Logc(ctx).WithFields(log.Fields{
				"backendName":             originalBackend.Name(),
				"backendUUID":             originalBackend.BackendUUID(),
				"originalConfigRef":       originalConfigRef,
				"TridentBackendConfigUID": callingConfigRef,
			}).Infof("Backend is not bound to any Trident Backend Config, attempting to bind it.")
		} else {
			Logc(ctx).WithFields(log.Fields{
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
	Logc(ctx).WithFields(log.Fields{
		"originalBackend.Name":        originalBackend.Name(),
		"originalBackend.BackendUUID": originalBackend.BackendUUID(),
		"backend":                     backend.Name(),
		"backend.BackendUUID":         backend.BackendUUID(),
		"backendUUID":                 backendUUID,
	}).Debug("Updating an existing backend.")

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
	case updateCode.Contains(storage.VolumeAccessInfoChange):
		err := errors.New("updating the data plane IP address isn't currently supported")
		Logc(ctx).WithField("error", err).Error("Backend update failed.")
		return nil, err
	case updateCode.Contains(storage.BackendRename):
		checkingBackend, lookupErr := o.getBackendByBackendName(backend.Name())
		if lookupErr == nil {
			// Don't rename if the name is already in use
			err := fmt.Errorf("backend name %v is already in use by %v", backend.Name(), checkingBackend.BackendUUID())
			Logc(ctx).WithField("error", err).Error("Backend update failed.")
			return nil, err
		} else if utils.IsNotFoundError(lookupErr) {
			// We couldn't find it so it's not in use, let's rename
			if err := o.replaceBackendAndUpdateVolumesOnPersistentStore(ctx, originalBackend, backend); err != nil {
				Logc(ctx).WithField("error", err).Errorf(
					"Could not rename backend from %v to %v", originalBackend.Name(), backend.Name())
				return nil, err
			}
		} else {
			// Unexpected error while checking if the backend is already in use
			err := fmt.Errorf("unexpected problem while renaming backend from %v to %v; %v",
				originalBackend.Name(), backend.Name(), lookupErr)
			Logc(ctx).WithField("error", err).Error("Backend update failed.")
			return nil, err
		}
	case updateCode.Contains(storage.PrefixChange):
		err := utils.UnsupportedConfigError(errors.New("updating the storage prefix isn't currently supported"))
		Logc(ctx).WithField("error", err).Error("Backend update failed.")
		return nil, err
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
					Logc(ctx).WithFields(log.Fields{
						"volume":                  volName,
						"vol.Config.InternalName": vol.Config.InternalName,
						"backend":                 backend.Name(),
					}).Warn("Backend update resulted in an orphaned volume.")
				}
			} else {
				if vol.Orphaned {
					vol.Orphaned = false
					updatePersistentStore = true
					Logc(ctx).WithFields(log.Fields{
						"volume":                  volName,
						"vol.Config.InternalName": vol.Config.InternalName,
						"backend":                 backend.Name(),
					}).Info("The volume is no longer orphaned as a result of the backend update.")
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
		Logc(ctx).WithFields(log.Fields{
			"backend": backend.Name(),
		}).Info("Updated backend satisfies no storage classes.")
	} else {
		Logc(ctx).WithFields(log.Fields{
			"backend": backend.Name(),
		}).Infof("Updated backend satisfies storage classes %s.",
			strings.Join(classes, ", "))
	}

	Logc(ctx).WithFields(log.Fields{
		"backend": backend,
	}).Debug("Returning external version")
	return backend.ConstructExternal(ctx), nil
}

// UpdateBackendState updates an existing backend's state.
func (o *TridentOrchestrator) UpdateBackendState(
	ctx context.Context, backendName, backendState string,
) (backendExternal *storage.BackendExternal, err error) {
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

	var (
		backend storage.Backend
	)

	Logc(ctx).WithFields(log.Fields{
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
	Logc(ctx).WithFields(log.Fields{
		"backend":               backend,
		"backendExternal":       backendExternal,
		"backendExternal.Name":  backendExternal.Name,
		"backendExternal.State": backendExternal.State.String(),
	}).Debug("GetBackend information.")
	return backendExternal, nil
}

func (o *TridentOrchestrator) GetBackendByBackendUUID(
	ctx context.Context, backendUUID string,
) (backendExternal *storage.BackendExternal, err error) {

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
	Logc(ctx).WithFields(log.Fields{
		"backend":               backend,
		"backendUUID":           backendUUID,
		"backendExternal":       backendExternal,
		"backendExternal.Name":  backendExternal.Name,
		"backendExternal.State": backendExternal.State.String(),
	}).Debug("GetBackend information.")
	return backendExternal, nil
}

func (o *TridentOrchestrator) ListBackends(
	ctx context.Context,
) (backendExternals []*storage.BackendExternal, err error) {

	if o.bootstrapError != nil {
		Logc(ctx).WithFields(log.Fields{
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

	if o.bootstrapError != nil {
		Logc(ctx).WithFields(log.Fields{
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

	if o.bootstrapError != nil {
		Logc(ctx).WithFields(log.Fields{
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

	Logc(ctx).WithFields(log.Fields{
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
			Logc(ctx).WithFields(log.Fields{
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
	Logc(ctx).WithFields(log.Fields{
		"backend":        backend,
		"backend.Name":   backend.Name(),
		"backend.State":  backend.State().String(),
		"backend.Online": backend.Online(),
	}).Debug("OfflineBackend information.")

	return o.storeClient.UpdateBackend(ctx, backend)
}

// RemoveBackendConfigRef sets backend configRef to empty and updates it.
func (o *TridentOrchestrator) RemoveBackendConfigRef(ctx context.Context, backendUUID, configRef string) (err error) {
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

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_add", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	volumeConfig.Version = config.OrchestratorAPIVersion

	if _, ok := o.volumes[volumeConfig.Name]; ok {
		return nil, fmt.Errorf("volume %s already exists", volumeConfig.Name)
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

	// Get the protocol based on the specified access mode & protocol
	protocol, err := o.getProtocol(ctx, volumeConfig.VolumeMode, volumeConfig.AccessMode, volumeConfig.Protocol)
	if err != nil {
		return nil, err
	}

	sc, ok := o.storageClasses[volumeConfig.StorageClass]
	if !ok {
		return nil, fmt.Errorf("unknown storage class: %s", volumeConfig.StorageClass)
	}
	pools := sc.GetStoragePoolsForProtocolByBackend(ctx, protocol, volumeConfig.RequisiteTopologies,
		volumeConfig.PreferredTopologies)
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

	// Recovery functions in case of error
	defer func() {
		err = o.addVolumeCleanup(ctx, err, backend, vol, txn, volumeConfig)
	}()
	defer func() {
		err = o.addVolumeRetryCleanup(ctx, err, backend, pool, txn, volumeConfig)
	}()

	Logc(ctx).WithField("volume", volumeConfig.Name).Debugf("Looking through %d storage pools.", len(pools))

	volumeCreationErrors := make([]error, 0)
	ineligibleBackends := make(map[string]struct{})

	// The pool lists are already shuffled, so just try them in order.
	// The loop terminates when creation on all matching pools has failed.
	for _, pool = range pools {

		backend = pool.Backend()

		// Mirror destinations can only be placed on mirroring enabled backends
		if volumeConfig.IsMirrorDestination && !backend.CanMirror() {
			Logc(ctx).Debugf("MirrorDestinations can only be placed on mirroring enabled backends")
			ineligibleBackends[backend.BackendUUID()] = struct{}{}
		}

		// If the pool's backend cannot possibly work, skip trying
		if _, ok := ineligibleBackends[backend.BackendUUID()]; ok {
			continue
		}

		// CreatePrepare has a side effect that updates the volumeConfig with the backend-specific internal name
		backend.Driver().CreatePrepare(ctx, volumeConfig)

		// Update transaction with updated volumeConfig
		txn = &storage.VolumeTransaction{
			Config: volumeConfig,
			Op:     storage.AddVolume,
		}
		if err = o.storeClient.UpdateVolumeTransaction(ctx, txn); err != nil {
			return nil, err
		}

		vol, err = backend.AddVolume(ctx, volumeConfig, pool, sc.GetAttributes(), false)
		if err != nil {

			logFields := log.Fields{
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
				return nil, err
			}

			// Log failure and continue for loop to find a pool that can create the volume.
			Logc(ctx).WithFields(logFields).Warn("Failed to create the volume on this pool.")

			volumeCreationErrors = append(volumeCreationErrors,
				fmt.Errorf("[Failed to create volume %s on storage pool %s from backend %s: %w]",
					volumeConfig.Name, pool.Name(), backend.Name(), err))

			// If this backend cannot handle the new volume on any pool, remove it from further consideration.
			if drivers.IsBackendIneligibleError(err) {
				ineligibleBackends[backend.BackendUUID()] = struct{}{}
			}
		} else {
			// Volume creation succeeded, so register it and return the result
			return o.addVolumeFinish(ctx, txn, vol, backend, pool)
		}
	}

	externalVol = nil
	if len(volumeCreationErrors) == 0 {
		err = fmt.Errorf("no suitable %s backend with \"%s\" storage class and %s of free space was found",
			protocol, volumeConfig.StorageClass, volumeConfig.Size)
	} else {
		// combine all errors within the list into a single err
		multiErr := multierr.Combine(volumeCreationErrors...)
		err = fmt.Errorf("encountered error(s) in creating the volume: %w", multiErr)
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

		logFields := log.Fields{
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

func (o *TridentOrchestrator) CloneVolume(
	ctx context.Context, volumeConfig *storage.VolumeConfig,
) (externalVol *storage.VolumeExternal, err error) {

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

	// Get the source volume
	sourceVolume, found := o.volumes[volumeConfig.CloneSourceVolume]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("source volume not found: %s", volumeConfig.CloneSourceVolume))
	}

	if volumeConfig.Size != "" {
		cloneSourceVolumeSize, err := strconv.ParseInt(sourceVolume.Config.Size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not get size of the clone source volume")
		}

		cloneVolumeSize, err := strconv.ParseInt(volumeConfig.Size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not get size of the clone volume")
		}

		if cloneSourceVolumeSize < cloneVolumeSize {
			Logc(ctx).WithFields(log.Fields{
				"source_volume": sourceVolume.Config.Name,
				"volume":        volumeConfig.Name,
				"backendUUID":   sourceVolume.BackendUUID,
			}).Error("requested PVC size is too large for the clone source")
			return nil, fmt.Errorf("requested PVC size '%d' is too large for the clone source '%d'",
				cloneVolumeSize, cloneSourceVolumeSize)
		}
	}

	if sourceVolume.Orphaned {
		Logc(ctx).WithFields(log.Fields{
			"source_volume": sourceVolume.Config.Name,
			"volume":        volumeConfig.Name,
			"backendUUID":   sourceVolume.BackendUUID,
		}).Warnf("Clone operation is likely to fail with an orphaned source volume.")
	}

	sourceVolumeMode := sourceVolume.Config.VolumeMode
	if sourceVolumeMode != "" && sourceVolumeMode != volumeConfig.VolumeMode {
		return nil, utils.NotFoundError(fmt.Sprintf(
			"source volume's volume-mode (%s) is incompatible with requested clone's volume-mode (%s)",
			sourceVolume.Config.VolumeMode, volumeConfig.VolumeMode))
	}

	// Get the source backend
	if backend, found = o.backends[sourceVolume.BackendUUID]; !found {
		// Should never get here but just to be safe
		return nil, utils.NotFoundError(fmt.Sprintf("backend %s for the source volume not found: %s",
			sourceVolume.BackendUUID, volumeConfig.CloneSourceVolume))
	}

	pool = storage.NewStoragePool(backend, "")

	// Clone the source config, as most of its attributes will apply to the clone
	cloneConfig := sourceVolume.Config.ConstructClone()

	// Copy a few attributes from the request that will affect clone creation
	cloneConfig.Name = volumeConfig.Name
	cloneConfig.InternalName = ""
	cloneConfig.InternalID = ""
	cloneConfig.CloneSourceVolume = volumeConfig.CloneSourceVolume
	cloneConfig.CloneSourceVolumeInternal = sourceVolume.Config.InternalName
	cloneConfig.CloneSourceSnapshot = volumeConfig.CloneSourceSnapshot
	cloneConfig.Qos = volumeConfig.Qos
	cloneConfig.QosType = volumeConfig.QosType

	// Override this value only if SplitOnClone has been defined in clone volume's config
	if volumeConfig.SplitOnClone != "" {
		cloneConfig.SplitOnClone = volumeConfig.SplitOnClone
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

	// Create the clone
	if vol, err = backend.CloneVolume(ctx, sourceVolume.Config, cloneConfig, pool, false); err != nil {

		logFields := log.Fields{
			"backend":      backend.Name(),
			"backendUUID":  backend.BackendUUID(),
			"volume":       cloneConfig.Name,
			"sourceVolume": cloneConfig.CloneSourceVolume,
			"error":        err,
		}

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

	// Volume creation succeeded, so register it and return the result
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

		logFields := log.Fields{
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

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_get_external", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	Logc(ctx).WithFields(log.Fields{
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
	volumeInternal string, _ context.Context,
) (volume string, err error) {

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
		if volumeConfig.FileSystem != "" && volumeConfig.FileSystem != drivers.FsRaw {
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

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_import_legacy", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	Logc(ctx).WithFields(log.Fields{
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

	driverType, err := o.getDriverTypeForVolume(volExternal.BackendUUID)
	if err != nil {
		return nil, fmt.Errorf("unable to determine driver type from volume %v", volExternal)
	}

	Logc(ctx).WithFields(log.Fields{
		"backendUUID":    volExternal.BackendUUID,
		"name":           volExternal.Config.Name,
		"internalName":   volExternal.Config.InternalName,
		"size":           volExternal.Config.Size,
		"protocol":       volExternal.Config.Protocol,
		"fileSystem":     volExternal.Config.FileSystem,
		"accessInfo":     volExternal.Config.AccessInfo,
		"accessMode":     volExternal.Config.AccessMode,
		"encryption":     volExternal.Config.Encryption,
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

	Logc(ctx).WithFields(log.Fields{
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

	driverType, err := o.getDriverTypeForVolume(volExternal.Backend)
	if err != nil {
		return nil, fmt.Errorf("unable to determine driver type from volume %v", volExternal)
	}

	Logc(ctx).WithFields(log.Fields{
		"backend":        volExternal.Backend,
		"name":           volExternal.Config.Name,
		"internalName":   volExternal.Config.InternalName,
		"size":           volExternal.Config.Size,
		"protocol":       volExternal.Config.Protocol,
		"fileSystem":     volExternal.Config.FileSystem,
		"accessInfo":     volExternal.Config.AccessInfo,
		"accessMode":     volExternal.Config.AccessMode,
		"encryption":     volExternal.Config.Encryption,
		"exportPolicy":   volExternal.Config.ExportPolicy,
		"snapshotPolicy": volExternal.Config.SnapshotPolicy,
		"driverType":     driverType,
	}).Debug("Driver processed volume import.")

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
	oldTxn, err := o.storeClient.GetExistingVolumeTransaction(ctx, volTxn)
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

	volTxn := &storage.VolumeTransaction{
		VolumeCreatingConfig: &storage.VolumeCreatingConfig{
			VolumeConfig: *config,
		},
		Op: storage.VolumeCreating,
	}

	if txn, err := o.storeClient.GetExistingVolumeTransaction(ctx, volTxn); err != nil {
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
	return o.storeClient.GetExistingVolumeTransaction(ctx, volTxn)
}

// DeleteVolumeTransaction deletes a volume transaction created by
// addVolumeTransaction.
func (o *TridentOrchestrator) DeleteVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error {
	return o.storeClient.DeleteVolumeTransaction(ctx, volTxn)
}

// addVolumeCleanup is used as a deferred method from the volume create/clone methods
// to clean up in case anything goes wrong during the operation.
func (o *TridentOrchestrator) addVolumeCleanup(
	ctx context.Context, err error, backend storage.Backend, vol *storage.Volume,
	volTxn *storage.VolumeTransaction, volumeConfig *storage.VolumeConfig,
) error {

	var (
		cleanupErr, txErr error
	)

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
			txErr = fmt.Errorf("unable to clean up add volume transaction:  %v", txErr)
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
			"Unable to clean up artifacts of volume creation: %v. Repeat creating the volume or restart %v.",
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

	existingTxn, getTxnErr := o.storeClient.GetExistingVolumeTransaction(ctx, volTxn)

	if getTxnErr != nil || existingTxn == nil {
		Logc(ctx).WithFields(log.Fields{
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

	var (
		cleanupErr, txErr error
	)

	backend, ok := o.backends[volumeConfig.ImportBackendUUID]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("backend %s not found", volumeConfig.ImportBackendUUID))
	}

	if err != nil {
		// We failed somewhere. Most likely we failed to rename the volume or retrieve its size.
		// Rename the volume
		if !volumeConfig.ImportNotManaged {
			cleanupErr = backend.RenameVolume(ctx, volumeConfig, volumeConfig.ImportOriginalName)
			Logc(ctx).WithFields(log.Fields{
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
			if err = o.deleteVolumeFromPersistentStoreIgnoreError(ctx, volume); err != nil {
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
	_ context.Context, volume string,
) (volExternal *storage.VolumeExternal, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	vol, found := o.volumes[volume]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("volume %v was not found", volume))
	}
	return vol.ConstructExternal(), nil
}

func (o *TridentOrchestrator) GetDriverTypeForVolume(_ context.Context, vol *storage.VolumeExternal) (string, error) {
	if o.bootstrapError != nil {
		return config.UnknownDriver, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return o.getDriverTypeForVolume(vol.BackendUUID)
}

// getDriverTypeForVolume does the necessary work to get the driver type.  It does
// not construct a transaction, nor does it take locks; it assumes that the
// caller will take care of both of these.  It also assumes that the backend
// exists in memory.
func (o *TridentOrchestrator) getDriverTypeForVolume(backendUUID string) (string, error) {
	if b, ok := o.backends[backendUUID]; ok {
		return b.Driver().Name(), nil
	}
	return config.UnknownDriver, nil
}

func (o *TridentOrchestrator) GetVolumeType(
	_ context.Context, vol *storage.VolumeExternal,
) (volumeType config.VolumeType, err error) {
	if o.bootstrapError != nil {
		return config.UnknownVolumeType, o.bootstrapError
	}

	defer recordTiming("volume_get_type", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	// Since the caller has a valid VolumeExternal and we're disallowing
	// backend deletion, we can assume that this will not hit a nil pointer.
	driver := o.backends[vol.BackendUUID].GetDriverName()
	switch {
	case driver == drivers.OntapNASStorageDriverName:
		volumeType = config.OntapNFS
	case driver == drivers.OntapNASQtreeStorageDriverName:
		volumeType = config.OntapNFS
	case driver == drivers.OntapNASFlexGroupStorageDriverName:
		volumeType = config.OntapNFS
	case driver == drivers.OntapSANStorageDriverName:
		volumeType = config.OntapISCSI
	case driver == drivers.SolidfireSANStorageDriverName:
		volumeType = config.SolidFireISCSI
	case driver == drivers.EseriesIscsiStorageDriverName:
		volumeType = config.ESeriesISCSI
	default:
		volumeType = config.UnknownVolumeType
	}

	return
}

func (o *TridentOrchestrator) ListVolumes(context.Context) (volumes []*storage.VolumeExternal, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_list", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	volumes = make([]*storage.VolumeExternal, 0, len(o.volumes))
	for _, v := range o.volumes {
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

	// if there are any snapshots for this volume, we need to "soft" delete.
	// only hard delete this volume when its last snapshot is deleted.
	snapshotsForVolume, err := o.volumeSnapshots(volumeName)
	if err != nil {
		return err
	}
	if len(snapshotsForVolume) > 0 {
		Logc(ctx).WithFields(log.Fields{
			"volume":                  volumeName,
			"backendUUID":             volume.BackendUUID,
			"len(snapshotsForVolume)": len(snapshotsForVolume),
		}).Debug("Soft deleting.")
		volume.State = storage.VolumeStateDeleting
		if updateErr := o.updateVolumeOnPersistentStore(ctx, volume); updateErr != nil {
			Logc(ctx).WithFields(log.Fields{
				"volume":    volume.Config.Name,
				"updateErr": updateErr.Error(),
			}).Error("Unable to update the volume's state to deleting in the persistent store.")
			return updateErr
		}
		return nil
	}

	// Note that this block will only be entered in the case that the volume
	// is missing it's backend and the backend is nil. If the backend does not
	// exist, delete the volume and clean up, then return.
	if volumeBackend == nil {
		if err := o.deleteVolumeFromPersistentStoreIgnoreError(ctx, volume); err != nil {
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
			Logc(ctx).WithFields(log.Fields{
				"volume":      volumeName,
				"backendUUID": volume.BackendUUID,
				"error":       err,
			}).Error("Unable to delete volume from backend.")
			return err
		} else {
			Logc(ctx).WithFields(log.Fields{
				"volume":      volumeName,
				"backendUUID": volume.BackendUUID,
				"error":       err,
			}).Debug("Skipping backend deletion of volume.")
		}
	}
	if err := o.deleteVolumeFromPersistentStoreIgnoreError(ctx, volume); err != nil {
		return err
	}

	if volumeBackend.State().IsDeleting() && !volumeBackend.HasVolumes() {
		if err := o.storeClient.DeleteBackend(ctx, volumeBackend); err != nil {
			Logc(ctx).WithFields(log.Fields{
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

func (o *TridentOrchestrator) deleteVolumeFromPersistentStoreIgnoreError(
	ctx context.Context, volume *storage.Volume,
) error {

	// Ignore failures to find the volume being deleted, as this may be called
	// during recovery of a volume that has already been deleted from the store.
	// During normal operation, checks on whether the volume is present in the
	// volume map should suffice to prevent deletion of non-existent volumes.
	if err := o.storeClient.DeleteVolumeIgnoreNotFound(ctx, volume); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"volume": volume.Config.Name,
		}).Error("Unable to delete volume from persistent store.")
		return err
	}
	return nil
}

// DeleteVolume does the necessary set up to delete a volume during the course
// of normal operation, verifying that the volume is present in Trident and
// creating a transaction to ensure that the delete eventually completes.
func (o *TridentOrchestrator) DeleteVolume(ctx context.Context, volumeName string) (err error) {

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_delete", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}
	if volume.Orphaned {
		Logc(ctx).WithFields(log.Fields{
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
			Logc(ctx).WithFields(log.Fields{
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

func (o *TridentOrchestrator) ListVolumesByPlugin(
	_ context.Context, pluginName string,
) (volumes []*storage.VolumeExternal, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_list_by_plugin", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	volumes = make([]*storage.VolumeExternal, 0)
	for _, backend := range o.backends {
		if backendName := backend.GetDriverName(); pluginName != backendName {
			continue
		}
		for _, vol := range backend.Volumes() {
			volumes = append(volumes, vol.ConstructExternal())
		}
	}
	return volumes, nil
}

func (o *TridentOrchestrator) PublishVolume(
	ctx context.Context, volumeName string, publishInfo *utils.VolumePublishInfo,
) (err error) {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_publish", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}
	if volume.State.IsDeleting() {
		return utils.VolumeDeletingError(fmt.Sprintf("volume %s is deleting", volumeName))
	}

	nodes := make([]*utils.Node, 0)
	for _, node := range o.nodes {
		nodes = append(nodes, node)
	}
	publishInfo.Nodes = nodes
	publishInfo.BackendUUID = volume.BackendUUID
	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		// Not a not found error because this is not user input
		return fmt.Errorf("backend %s not found", volume.BackendUUID)
	}
	if err := o.reconcileNodeAccessOnBackend(ctx, backend); err != nil {
		err = fmt.Errorf("unable to update node access rules on backend %s; %v", backend.Name(), err)
		Logc(ctx).Error(err)
		return err
	}

	err = backend.PublishVolume(ctx, volume.Config, publishInfo)
	if err != nil {
		return err
	}
	if err := o.updateVolumeOnPersistentStore(ctx, volume); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"volume": volume.Config.Name,
		}).Error("Unable to update the volume in persistent store.")
		return err
	}

	return nil
}

func (o *TridentOrchestrator) UnpublishVolume(ctx context.Context, volumeName, nodeName string) (err error) {

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_unpublish", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}

	node, ok := o.nodes[nodeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("node %s not found", nodeName))
	}

	// Build list of nodes to which the volume remains published
	nodeMap := make(map[string]*utils.Node)
	for _, pub := range o.listVolumePublicationsForVolume(ctx, volumeName) {
		if pub.NodeName == nodeName {
			continue
		}
		if n, found := o.nodes[pub.NodeName]; !found {
			Logc(ctx).WithField("node", pub.NodeName).Warning("Node not found during volume unpublish.")
			continue
		} else {
			nodeMap[pub.NodeName] = n
		}
	}
	nodes := make([]*utils.Node, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}

	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		// Not a not found error because this is not user input
		return fmt.Errorf("backend %s not found", volume.BackendUUID)
	}

	// Unpublish the volume
	if err = backend.UnpublishVolume(ctx, volume.Config, nodes); err != nil {
		return err
	}

	// Check for publications remaining on the node, not counting the one we just unpublished.
	nodePubFound := false
	for _, pub := range o.listVolumePublicationsForNode(ctx, nodeName) {
		if pub.VolumeName == volumeName {
			continue
		}
		nodePubFound = true
		break
	}

	// If the node is known to be gone, and if we just unpublished the last volume on that node, delete the node.
	if !nodePubFound && node.Deleted {
		return o.deleteNode(ctx, nodeName)
	}

	return nil
}

// AttachVolume mounts a volume to the local host.  This method is currently only used by Docker,
// and it should be able to accomplish its task using only the data passed in; it should not need to
// use the storage controller API.  It may be assumed that this method always runs on the host to
// which the volume will be attached.
func (o *TridentOrchestrator) AttachVolume(
	ctx context.Context, volumeName, mountpoint string, publishInfo *utils.VolumePublishInfo,
) (err error) {

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
		return utils.VolumeDeletingError(fmt.Sprintf("volume %s is deleting", volumeName))
	}

	hostMountpoint := mountpoint
	isDockerPluginModeSet := false
	if os.Getenv(config.DockerPluginModeEnvVariable) != "" {
		isDockerPluginModeSet = true
		hostMountpoint = filepath.Join("/host", mountpoint)
	}

	Logc(ctx).WithFields(log.Fields{
		"volume":                volumeName,
		"mountpoint":            mountpoint,
		"hostMountpoint":        hostMountpoint,
		"isDockerPluginModeSet": isDockerPluginModeSet,
	}).Debug("Mounting volume.")

	// Ensure mount point exists and is a directory
	fileInfo, err := os.Lstat(hostMountpoint)
	if os.IsNotExist(err) {
		// Create the mount point if it doesn't exist
		if err := os.MkdirAll(hostMountpoint, 0755); err != nil {
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
	} else {
		return utils.AttachISCSIVolume(ctx, volumeName, mountpoint, publishInfo)
	}
}

// DetachVolume unmounts a volume from the local host.  This method is currently only used by Docker,
// and it should be able to accomplish its task using only the data passed in; it should not need to
// use the storage controller API.  It may be assumed that this method always runs on the host to
// which the volume will be attached.  It ensures the volume is already mounted, and it attempts to
// delete the mount point.
func (o *TridentOrchestrator) DetachVolume(ctx context.Context, volumeName, mountpoint string) (err error) {

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_detach", &err)()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}
	if volume.State.IsDeleting() {
		return utils.VolumeDeletingError(fmt.Sprintf("volume %s is deleting", volumeName))
	}

	Logc(ctx).WithFields(log.Fields{
		"volume":     volumeName,
		"mountpoint": mountpoint,
	}).Debug("Unmounting volume.")

	// Check if the mount point exists, so we know that it's attached and must be cleaned up
	_, err = os.Stat(mountpoint)
	if err != nil {
		// Not attached, so nothing to do
		return nil
	}

	// Unmount the volume
	if err := utils.Umount(ctx, mountpoint); err != nil {
		return err
	}

	// Best effort removal of the mount point
	_ = os.Remove(mountpoint)
	return nil
}

// SetVolumeState sets the state of a volume to a given value
func (o *TridentOrchestrator) SetVolumeState(
	ctx context.Context, volumeName string, state storage.VolumeState,
) (err error) {

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

// CreateSnapshot creates a snapshot of the given volume
func (o *TridentOrchestrator) CreateSnapshot(
	ctx context.Context, snapshotConfig *storage.SnapshotConfig,
) (externalSnapshot *storage.SnapshotExternal, err error) {

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
		return nil, utils.NotFoundError(fmt.Sprintf("source volume %s not found", snapshotConfig.VolumeName))
	}
	if volume.State.IsDeleting() {
		return nil, utils.VolumeDeletingError(fmt.Sprintf("source volume %s is deleting", snapshotConfig.VolumeName))
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

	var (
		cleanupErr, txErr error
	)
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
		Logc(ctx).WithFields(log.Fields{
			"volume":   snapshot.Config.VolumeName,
			"snapshot": snapshot.Config.Name,
			"backend":  backend.Name(),
			"error":    err,
		}).Error("Unable to delete snapshot from backend.")
		return err
	}
	if err := o.deleteSnapshotFromPersistentStoreIgnoreError(ctx, snapshot); err != nil {
		return err
	}

	delete(o.snapshots, snapshot.ID())

	snapshotsForVolume, err := o.volumeSnapshots(snapshotConfig.VolumeName)
	if err != nil {
		return err
	}

	if len(snapshotsForVolume) == 0 && volume.State.IsDeleting() {
		Logc(ctx).WithFields(log.Fields{
			"snapshotConfig.VolumeName": snapshotConfig.VolumeName,
			"backendUUID":               volume.BackendUUID,
			"volume.State":              volume.State,
		}).Debug("Hard deleting volume.")
		return o.deleteVolume(ctx, snapshotConfig.VolumeName)
	}

	return nil
}

func (o *TridentOrchestrator) deleteSnapshotFromPersistentStoreIgnoreError(
	ctx context.Context, snapshot *storage.Snapshot,
) error {

	// Ignore failures to find the snapshot being deleted, as this may be called
	// during recovery of a snapshot that has already been deleted from the store.
	// During normal operation, checks on whether the snapshot is present in the
	// snapshot map should suffice to prevent deletion of non-existent snapshots.
	if err := o.storeClient.DeleteSnapshotIgnoreNotFound(ctx, snapshot); err != nil {
		Logc(ctx).WithFields(log.Fields{
			"snapshot": snapshot.Config.Name,
			"volume":   snapshot.Config.VolumeName,
		}).Error("Unable to delete snapshot from persistent store.")
		return err
	}
	return nil
}

// DeleteSnapshot deletes a snapshot of the given volume
func (o *TridentOrchestrator) DeleteSnapshot(ctx context.Context, volumeName, snapshotName string) (err error) {

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
		if err = o.deleteSnapshotFromPersistentStoreIgnoreError(ctx, snapshot); err != nil {
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
		if err = o.deleteSnapshotFromPersistentStoreIgnoreError(ctx, snapshot); err != nil {
			return err
		}
		delete(o.snapshots, snapshot.ID())
		return nil
	}

	// TODO: Is this needed?
	if volume.Orphaned {
		Logc(ctx).WithFields(log.Fields{
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
			Logc(ctx).WithFields(log.Fields{
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

func (o *TridentOrchestrator) ListSnapshots(context.Context) (snapshots []*storage.SnapshotExternal, err error) {
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
	_ context.Context, snapshotName string,
) (snapshots []*storage.SnapshotExternal, err error) {
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
	_ context.Context, volumeName string,
) (snapshots []*storage.SnapshotExternal, err error) {
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

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_resize", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	volume, found := o.volumes[volumeName]
	if !found {
		return utils.NotFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}
	if volume.Orphaned {
		Logc(ctx).WithFields(log.Fields{
			"volume":      volumeName,
			"backendUUID": volume.BackendUUID,
		}).Warnf("Resize operation is likely to fail with an orphaned volume.")
	}
	if volume.State.IsDeleting() {
		return utils.VolumeDeletingError(fmt.Sprintf("volume %s is deleting", volumeName))
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
			Logc(ctx).WithFields(log.Fields{
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
		Logc(ctx).WithFields(log.Fields{
			"volume":      volume.Config.Name,
			"backendUUID": volume.BackendUUID,
		}).Error("Unable to find backend during volume resize.")
		return fmt.Errorf("unable to find backend %v during volume resize", volume.BackendUUID)
	}

	if volume.Config.Size != newSize {
		// If the resize is successful the driver updates the volume.Config.Size, as a side effect, with the actual
		// byte size of the expanded volume.
		if err := volumeBackend.ResizeVolume(ctx, volume.Config, newSize); err != nil {
			Logc(ctx).WithFields(log.Fields{
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
		Logc(ctx).WithFields(log.Fields{
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
//
// The below truth table depicts these combinations:
//
//  VolumeMode/AccessType   AccessMode    Protocol(block or file)       Result Protocol
//  Filesystem              Any           Any                           Any
//  Filesystem              Any           NFS                           NFS
//  Filesystem              Any           iSCSI                         iSCSI
//  Filesystem              RWO           Any                           Any
//  Filesystem              RWO           NFS                           NFS
//  Filesystem              RWO           iSCSI                         iSCSI
//  Filesystem              ROX           Any                           Any
//  Filesystem              ROX           NFS                           NFS
//  Filesystem              ROX           iSCSI                         iSCSI
//  Filesystem              RWX           Any                           **NFS**
//  Filesystem              RWX           NFS                           NFS
//  Filesystem              RWX           iSCSI                         **Error**
//  RawBlock                Any           Any                           **iSCSI**
//  RawBlock                Any           NFS                           **Error**
//  RawBlock                Any           iSCSI                         iSCSI
//  RawBlock                RWO           Any                           **iSCSI**
//  RawBlock                RWO           NFS                           **Error**
//  RawBlock                RWO           iSCSI                         iSCSI
//  RawBlock                ROX           Any                           **iSCSI**
//  RawBlock                ROX           NFS                           **Error**
//  RawBlock                ROX           iSCSI                         iSCSI
//  RawBlock                RWX           Any                           **iSCSI**
//  RawBlock                RWX           NFS                           **Error**
//  RawBlock                RWX           iSCSI                         iSCSI
//
func (o *TridentOrchestrator) getProtocol(
	ctx context.Context, volumeMode config.VolumeMode, accessMode config.AccessMode, protocol config.Protocol,
) (config.Protocol, error) {

	Logc(ctx).WithFields(log.Fields{
		"volumeMode": volumeMode,
		"accessMode": accessMode,
		"protocol":   protocol,
	}).Debug("Orchestrator#getProtocol")

	resultProtocol := protocol
	var err error = nil

	if volumeMode == config.RawBlock {
		// In `Block` volume-mode, Protocol: file (NFS) is unsupported i.e. we do not support raw block volumes
		// with NFS protocol.
		if protocol == config.File {
			resultProtocol = config.ProtocolAny
			err = fmt.Errorf("incompatible volume mode (%s) and protocol (%s)", volumeMode, protocol)
		} else if protocol == config.ProtocolAny {
			resultProtocol = config.Block
		}
	} else {
		// In `Filesystem` volume-mode, Protocol: block (iSCSI), AccessMode: ReadWriteMany, is an unsupported config
		// i.e.  RWX is only supported by file and file-like protocols only, such as NFS.
		if accessMode == config.ReadWriteMany {
			if protocol == config.Block {
				resultProtocol = config.ProtocolAny
				err = fmt.Errorf("incompatible volume mode (%s), access mode (%s) and protocol (%s)",
					volumeMode, accessMode, protocol)
			} else if protocol == config.ProtocolAny {
				resultProtocol = config.File
			}
		}
	}

	Logc(ctx).Debugf("Result Protocol: %v", resultProtocol)

	return resultProtocol, err
}

func (o *TridentOrchestrator) AddStorageClass(
	ctx context.Context, scConfig *storageclass.Config,
) (scExternal *storageclass.External, err error) {

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
		Logc(ctx).WithFields(log.Fields{
			"storageClass": scConfig.Name,
		}).Info("No backends currently satisfy provided storage class.")
	} else {
		Logc(ctx).WithFields(log.Fields{
			"storageClass": sc.GetName(),
		}).Infof("Storage class satisfied by %d storage pools.", added)
	}
	return sc.ConstructExternal(ctx), nil
}

func (o *TridentOrchestrator) GetStorageClass(
	ctx context.Context, scName string,
) (scExternal *storageclass.External, err error) {
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
	for _, storagePool := range sc.GetStoragePoolsForProtocol(ctx, config.ProtocolAny) {
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

	nodes := make([]*utils.Node, 0)

	for _, n := range o.nodes {
		nodes = append(nodes, n)
	}

	return b.ReconcileNodeAccess(ctx, nodes)
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
	ctx := GenerateRequestContext(context.Background(), "", ContextSourcePeriodic)

	Logc(ctx).Info("Starting periodic node access reconciliation service.")
	defer Logc(ctx).Info("Stopping periodic node access reconciliation service.")

	// Every period seconds after the last run
	for range time.Tick(NodeAccessReconcilePeriod) {
		select {
		case <-o.stopNodeAccessLoop:
			// Exit on shutdown signal
			return

		default:
			Logc(ctx).Trace("Periodic node access reconciliation loop beginning.")
			// Wait at least cooldown seconds after the last node registration to prevent thundering herd
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

func (o *TridentOrchestrator) AddNode(
	ctx context.Context, node *utils.Node, nodeEventCallback NodeEventCallback,
) (err error) {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("node_add", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	if node.NodePrep != nil && node.NodePrep.Enabled {
		// Check if node prep status has changed
		oldNode, found := o.nodes[node.Name]
		if found && oldNode.NodePrep != nil {
			// NFS
			if node.NodePrep.NFS != oldNode.NodePrep.NFS {
				o.handleUpdatedNodePrep(ctx, "NFS", node, nodeEventCallback)
			}
			// iSCSI
			if node.NodePrep.ISCSI != oldNode.NodePrep.ISCSI {
				o.handleUpdatedNodePrep(ctx, "ISCSI", node, nodeEventCallback)
			}
		} else {
			o.handleUpdatedNodePrep(ctx, "NFS", node, nodeEventCallback)
			o.handleUpdatedNodePrep(ctx, "ISCSI", node, nodeEventCallback)
		}
	}

	if err := o.storeClient.AddOrUpdateNode(ctx, node); err != nil {
		return err
	}

	o.nodes[node.Name] = node

	o.lastNodeRegistration = time.Now()
	o.invalidateAllBackendNodeAccess()
	return nil
}

func (o *TridentOrchestrator) invalidateAllBackendNodeAccess() {
	for _, backend := range o.backends {
		backend.InvalidateNodeAccess()
	}
}

func (o *TridentOrchestrator) handleUpdatedNodePrep(
	ctx context.Context, protocol string, node *utils.Node, nodeEventCallback NodeEventCallback,
) {

	var status utils.NodePrepStatus
	var message string

	switch protocol {
	case "NFS":
		status = node.NodePrep.NFS
		message = node.NodePrep.NFSStatusMessage
	case "ISCSI":
		status = node.NodePrep.ISCSI
		message = node.NodePrep.ISCSIStatusMessage
	default:
		Logc(ctx).WithField("protocol", protocol).Error("Cannot report node prep status: unsupported protocol.")
		return
	}

	fields := log.Fields{
		"node":    node.Name,
		"message": message,
	}

	switch status {
	case utils.PrepFailed:
		Logc(ctx).WithFields(fields).Warnf("Node prep for %s failed on node.", protocol)
		nodeEventCallback(helpers.EventTypeWarning, fmt.Sprintf("%sNodePrepFailed", protocol), message)
	case utils.PrepPreConfigured:
		Logc(ctx).WithFields(fields).Warnf("Node was preconfigured for %s.", protocol)
		nodeEventCallback(helpers.EventTypeWarning, fmt.Sprintf("%sNodePrepPreconfigured", protocol), message)
	case utils.PrepCompleted:
		Logc(ctx).WithFields(fields).Infof("Node prep for %s completed on node.", protocol)
		nodeEventCallback(helpers.EventTypeNormal, fmt.Sprintf("%sNodePrepCompleted", protocol), message)
	case utils.PrepRunning:
		Logc(ctx).WithFields(fields).Debugf("Node prep for %s started on node.", protocol)
		if log.GetLevel() == log.DebugLevel {
			nodeEventCallback(helpers.EventTypeNormal, fmt.Sprintf("%sNodePrepStarted", protocol), message)
		}
	}
}

func (o *TridentOrchestrator) GetNode(ctx context.Context, nName string) (node *utils.Node, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("node_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	node, found := o.nodes[nName]
	if !found {
		Logc(ctx).WithFields(log.Fields{"node": nName}).Info(
			"There may exist a networking or DNS issue preventing this node from registering with the" +
				" Trident controller")
		return nil, utils.NotFoundError(fmt.Sprintf("node %v was not found", nName))
	}
	return node, nil
}

func (o *TridentOrchestrator) ListNodes(context.Context) (nodes []*utils.Node, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("node_list", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	nodes = make([]*utils.Node, 0, len(o.nodes))
	for _, node := range o.nodes {
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (o *TridentOrchestrator) DeleteNode(ctx context.Context, nodeName string) (err error) {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("node_delete", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	node, found := o.nodes[nodeName]
	if !found {
		return utils.NotFoundError(fmt.Sprintf("node %s not found", nodeName))
	}

	publicationCount := len(o.listVolumePublicationsForNode(ctx, nodeName))
	logFields := log.Fields{
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

	node, found := o.nodes[nodeName]
	if !found {
		return utils.NotFoundError(fmt.Sprintf("node %s not found", nodeName))
	}

	if err = o.storeClient.DeleteNode(ctx, node); err != nil {
		return err
	}
	delete(o.nodes, nodeName)
	o.invalidateAllBackendNodeAccess()
	return o.reconcileNodeAccessOnAllBackends(ctx)
}

// AddVolumePublication records the volume publication for a given volume/node pair
func (o *TridentOrchestrator) AddVolumePublication(
	ctx context.Context, publication *utils.VolumePublication,
) (err error) {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("vol_pub_add", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	if err := o.storeClient.AddVolumePublication(ctx, publication); err != nil {
		return err
	}

	o.addVolumePublicationToCache(publication)

	return nil
}

// GetVolumePublication returns the volume publication for a given volume/node pair
func (o *TridentOrchestrator) GetVolumePublication(
	_ context.Context, volumeName, nodeName string,
) (publication *utils.VolumePublication, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	publication, found := o.volumePublications[volumeName][nodeName]
	if !found {
		return nil, utils.NotFoundError(fmt.Sprintf("volume publication %v was not found",
			utils.GenerateVolumePublishName(volumeName, nodeName)))
	}
	return publication, nil
}

// ListVolumePublications returns a list of all volume publications
func (o *TridentOrchestrator) ListVolumePublications(
	context.Context,
) (publications []*utils.VolumePublication, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_list", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	publications = []*utils.VolumePublication{}
	for _, pubs := range o.volumePublications {
		for _, pub := range pubs {
			publications = append(publications, pub)
		}
	}
	return publications, nil
}

// ListVolumePublicationsForVolume returns a list of all volume publications for a given volume
func (o *TridentOrchestrator) ListVolumePublicationsForVolume(
	ctx context.Context, volumeName string,
) (publications []*utils.VolumePublication, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_list_for_vol", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	publications = o.listVolumePublicationsForVolume(ctx, volumeName)
	return
}

func (o *TridentOrchestrator) listVolumePublicationsForVolume(
	_ context.Context, volumeName string,
) (publications []*utils.VolumePublication) {

	publications = []*utils.VolumePublication{}
	for _, pub := range o.volumePublications[volumeName] {
		publications = append(publications, pub)
	}
	return publications
}

// ListVolumePublicationsForNode returns a list of all volume publications for a given node
func (o *TridentOrchestrator) ListVolumePublicationsForNode(
	ctx context.Context, nodeName string,
) (publications []*utils.VolumePublication, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_list_for_node", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()

	publications = o.listVolumePublicationsForNode(ctx, nodeName)
	return
}

func (o *TridentOrchestrator) listVolumePublicationsForNode(
	_ context.Context, nodeName string,
) (publications []*utils.VolumePublication) {

	publications = []*utils.VolumePublication{}
	for _, pubs := range o.volumePublications {
		if pubs[nodeName] != nil {
			publications = append(publications, pubs[nodeName])
		}
	}
	return
}

// DeleteVolumePublication deletes the record of the volume publication for a given volume/node pair
func (o *TridentOrchestrator) DeleteVolumePublication(ctx context.Context, volumeName, nodeName string) (err error) {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("vol_pub_delete", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	defer o.updateMetrics()

	publication, found := o.volumePublications[volumeName][nodeName]
	if !found {
		return utils.NotFoundError(fmt.Sprintf("volume publication %s not found",
			utils.GenerateVolumePublishName(volumeName, nodeName)))
	}
	if err = o.storeClient.DeleteVolumePublication(ctx, publication); err != nil {
		if !utils.IsNotFoundError(err) {
			return err
		}
	}
	o.removeVolumePublicationFromCache(volumeName, nodeName)
	return nil
}

func (o *TridentOrchestrator) removeVolumePublicationFromCache(volumeID string, nodeID string) {
	delete(o.volumePublications[volumeID], nodeID)
	// If there are no more nodes for this volume, remove the volume's entry
	if len(o.volumePublications[volumeID]) == 0 {
		delete(o.volumePublications, volumeID)
	}
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
			Logc(ctx).WithFields(log.Fields{
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
	Logc(ctx).WithFields(log.Fields{
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
	Logc(ctx).WithFields(log.Fields{
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
func (o *TridentOrchestrator) EstablishMirror(
	ctx context.Context, backendUUID, localVolumeHandle, remoteVolumeHandle string,
) (err error) {

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
	return mirrorBackend.EstablishMirror(ctx, localVolumeHandle, remoteVolumeHandle)
}

// ReestablishMirror recreates a previously existing replication mirror relationship between 2 volumes on a backend
func (o *TridentOrchestrator) ReestablishMirror(
	ctx context.Context, backendUUID, localVolumeHandle, remoteVolumeHandle string,
) (err error) {

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
	return mirrorBackend.ReestablishMirror(ctx, localVolumeHandle, remoteVolumeHandle)
}

// PromoteMirror makes the local volume the primary
func (o *TridentOrchestrator) PromoteMirror(
	ctx context.Context, backendUUID, localVolumeHandle, remoteVolumeHandle, snapshotHandle string,
) (waitingForSnapshot bool, err error) {

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
	return mirrorBackend.PromoteMirror(ctx, localVolumeHandle, remoteVolumeHandle, snapshotHandle)
}

// GetMirrorStatus returns the current status of the mirror relationship
func (o *TridentOrchestrator) GetMirrorStatus(
	ctx context.Context, backendUUID, localVolumeHandle, remoteVolumeHandle string,
) (status string, err error) {

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
	return mirrorBackend.GetMirrorStatus(ctx, localVolumeHandle, remoteVolumeHandle)
}

func (o *TridentOrchestrator) CanBackendMirror(_ context.Context, backendUUID string) (capable bool, err error) {

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
