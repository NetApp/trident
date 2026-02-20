// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	"github.com/netapp/trident/core/cache"
	"github.com/netapp/trident/core/metrics"
	"github.com/netapp/trident/frontend"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	persistentstore "github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/pkg/capacity"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/factory"
	sa "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/utils/autogrow"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/fcp"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
	"github.com/netapp/trident/utils/nvme"
)

var addVolumeAfterAddVolumeTxn = fiji.Register("addVolumeAfterAddVolumeTransaction", "orchestrator_core")

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
	groupSnapshots           map[string]*storage.GroupSnapshot
	autogrowPolicies         map[string]*storage.AutogrowPolicy
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
	iscsi                    iscsi.ISCSI
	fs                       filesystem.Filesystem
	fcp                      fcp.FCP
	mount                    mount.Mount
}

// NewTridentOrchestrator returns a storage orchestrator instance
func NewTridentOrchestrator(client persistentstore.Client) (*TridentOrchestrator, error) {
	// NewClient() must plugin default implementation of the various package clients.
	iscsiClient, err := iscsi.New()
	if err != nil {
		return nil, err
	}

	mountClient, err := mount.New()
	if err != nil {
		return nil, err
	}

	fcpClent, err := fcp.New()
	if err != nil {
		return nil, err
	}

	vpSyncRateLimiter = rate.NewLimiter(vpUpdateRateLimit, vpUpdateBurst)

	return &TridentOrchestrator{
		backends:           make(map[string]storage.Backend), // key is UUID, not name
		volumes:            make(map[string]*storage.Volume),
		subordinateVolumes: make(map[string]*storage.Volume),
		frontends:          make(map[string]frontend.Plugin),
		storageClasses:     make(map[string]*storageclass.StorageClass),
		nodes:              *cache.NewNodeCache(),
		volumePublications: cache.NewVolumePublicationCache(),
		snapshots:          make(map[string]*storage.Snapshot), // key is ID, not name
		groupSnapshots:     make(map[string]*storage.GroupSnapshot),
		autogrowPolicies:   make(map[string]*storage.AutogrowPolicy),
		mutex:              &sync.Mutex{},
		storeClient:        client,
		bootstrapped:       false,
		bootstrapError:     errors.NotReadyError(),
		iscsi:              iscsiClient,
		fcp:                fcpClent,
		mount:              mountClient,
		fs:                 filesystem.New(mountClient),
	}, nil
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
		o.bootstrapError = errors.BootstrapError(err)
		return o.bootstrapError
	}

	// Set UUID
	o.uuid, err = o.storeClient.GetTridentUUID(ctx)
	if err != nil {
		o.bootstrapError = errors.BootstrapError(err)
		return o.bootstrapError
	}

	// Bootstrap state from persistent store
	if err = o.bootstrap(ctx); err != nil {
		o.bootstrapError = errors.BootstrapError(err)
		return o.bootstrapError
	}

	if monitorTransactions {
		// Start transaction monitor
		o.StartTransactionMonitor(ctx, txnMonitorPeriod, txnMonitorMaxAge)
	}

	o.bootstrapped = true
	o.bootstrapError = nil

	Logc(ctx).Infof("%s bootstrapped successfully.", convert.ToTitle(config.OrchestratorName))
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

			newBackend.SetUserState(b.UserState)

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
		if !collection.ContainsString(volNames, k) {
			delete(o.volumes, k)
		}
	}
	volCount := 0
	for _, v := range volumes {
		// TODO:  If the API evolves, check the Version field here.
		var backend storage.Backend
		var ok bool
		vol := storage.NewVolume(v.Config, v.BackendUUID, v.Pool, v.Orphaned, v.State)
		vol.AutogrowStatus = v.AutogrowStatus
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
				backend.Volumes().Store(vol.Config.Name, vol)
				if fakeDriver, ok := backend.Driver().(*fake.StorageDriver); ok {
					fakeDriver.BootstrapVolume(ctx, vol)
				}
			}
		}

		// Rebuild Autogrow policy associations (CSI mode only)
		if config.CurrentDriverContext == config.ContextCSI {
			effectiveAGPolicy, policyErr := o.resolveAndSetEffectiveAutogrowPolicy(ctx, vol, "bootstrap", true)
			// Associate only if policy exists AND not disabled
			if policyErr == nil && effectiveAGPolicy.PolicyName != "" {
				o.associateVolumeWithAutogrowPolicyInternal(ctx, vol.Config.Name, effectiveAGPolicy.PolicyName)
			}
		}

		// Set the publish enforcement flag on the subordinate volume if supported by the backend and drvier.
		// This is needed for nas and nas eco volumes. Legacy volumes may not have this flag set. Needed for
		// automatic force-detach.
		err = o.healTridentVolumePublishEnforcement(ctx, vol, backend)
		if err != nil {
			Logc(ctx).WithError(err).Warning("Unable to heal Trident volume publish enforcement.")
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

func (o *TridentOrchestrator) healTridentVolumePublishEnforcement(
	ctx context.Context, vol *storage.Volume, backend storage.Backend,
) error {
	if vol.Config.AccessInfo.PublishEnforcement {
		// If publish enforcement is already enabled on the volume, nothing to do.
		return nil
	}

	// If this backend cannot enable publish enforcement, then, no volume on this backend
	// can have publish enforcement enabled.
	if backend == nil {
		Logc(ctx).WithField("volume", vol.Config.Name).
			Info("Volume cannot have publish enforcement enabled, backend missing.")
		return nil
	}

	// Enable publish enforcement on the volume.
	updated := backend.HealVolumePublishEnforcement(ctx, vol)

	if updated {
		_, exists := o.volumes[vol.Config.Name]
		if !exists {
			return fmt.Errorf("volume %s not found in cache during healing publish enforcement", vol.Config.Name)
		}
		o.volumes[vol.Config.Name] = vol
	}
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

func (o *TridentOrchestrator) bootstrapGroupSnapshots(ctx context.Context) error {
	groupSnapshots, err := o.storeClient.GetGroupSnapshots(ctx)
	if err != nil {
		return err
	}
	for _, gs := range groupSnapshots {
		// TODO:  If the API evolves, check the Version field here.
		groupSnapshot := storage.NewGroupSnapshot(gs.Config(), gs.GetSnapshotIDs(), gs.GetCreated())
		o.groupSnapshots[groupSnapshot.ID()] = groupSnapshot

		// Check if there are any missing snapshots or any that are missing a group ID when they should have one.
		groupID, snapshotIDs, created := groupSnapshot.ID(), groupSnapshot.GetSnapshotIDs(), groupSnapshot.GetCreated()
		for _, snapID := range snapshotIDs {
			// Get the snapshot by ID.
			snapshot, ok := o.snapshots[snapID]
			if !ok {
				Logc(ctx).Warnf("Snapshot '%s' from group snapshot '%s'.", snapID, groupID)
				continue // Log a warning and skip if we can't find a constituent snapshot.
			}

			if snapshot.Config.GroupSnapshotName != groupID {
				Logc(ctx).WithFields(LogFields{
					"snapshotGroupRef":  snapshot.Config.GroupSnapshotName,
					"groupSnapshotName": groupID,
				}).Warn("Snapshot created time does not match group snapshot created time.")
			}

			if snapshot.Created != created {
				Logc(ctx).WithFields(LogFields{
					"snapshotCreated": snapshot.Created,
					"groupCreatedAt":  created,
				}).Warn("Snapshot created time does not match group snapshot created time.")
			}
		}

		Logc(ctx).WithFields(LogFields{
			"groupSnapshot": groupSnapshot.ID(),
			"snapshotIDs":   groupSnapshot.GetSnapshotIDs(),
			"handler":       "Bootstrap",
		}).Info("Added an existing group snapshot.")
	}
	return nil
}

func (o *TridentOrchestrator) bootstrapAutogrowPolicies(ctx context.Context) error {
	persistentAGPolicies, err := o.storeClient.GetAutogrowPolicies(ctx)
	if err != nil {
		return err
	}

	for _, persistentAGPolicy := range persistentAGPolicies {
		// Create Autogrow policy object with empty volume list (associations rebuilt during bootstrapVolumes)
		agPolicy := storage.NewAutogrowPolicyFromPersistent(persistentAGPolicy)
		o.autogrowPolicies[agPolicy.Name()] = agPolicy
		Logc(ctx).WithFields(LogFields{
			"autogrowPolicyName":  agPolicy.Name(),
			"autogrowPolicyState": agPolicy.State(),
			"handler":             "Bootstrap",
		}).Info("Added an existing Autogrow policy.")
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

	vpsToBeSynced := make([]*models.VolumePublication, 0, len(volumePublications))

	for _, vp := range volumePublications {
		// Update VP fields from corresponding TridentVolume (check both regular and subordinate volumes)
		var vol *storage.Volume
		var ok bool
		if vol, ok = o.volumes[vp.VolumeName]; !ok {
			vol, ok = o.subordinateVolumes[vp.VolumeName]
		}
		if ok {
			// syncVolumePublicationFields modifies vp in place and returns true if sync is needed
			syncNeeded := syncVolumePublicationFields(vol, vp)

			// Add it to the list of vpsToBeSynced if syncNeeded is true
			if syncNeeded {
				vpsToBeSynced = append(vpsToBeSynced, vp)
			}
		}

		// Add VP to cache with updated information
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

	// Asynchronously persist VPs that need syncing (e.g., after upgrade)
	if len(vpsToBeSynced) > 0 {
		go o.SyncVolumePublications(ctx, vpsToBeSynced)
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
		// Autogrow policies loaded before volumes so volumes can be associated with policies
		o.bootstrapAutogrowPolicies,
		// Volumes, storage classes, and snapshots require backends to be bootstrapped.
		o.bootstrapStorageClasses, o.bootstrapVolumes, o.bootstrapSnapshots,
		// Volume group snapshots require snapshots to be bootstrapped.
		o.bootstrapGroupSnapshots,
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
	metrics.TridentBuildInfo.WithLabelValues(config.BuildHash,
		config.OrchestratorVersion.ShortString(),
		config.BuildType).Set(float64(1))

	metrics.BackendsGauge.Reset()
	metrics.TridentBackendInfo.Reset()
	for _, backend := range o.backends {
		if backend == nil {
			continue
		}
		metrics.BackendsGauge.WithLabelValues(backend.GetDriverName(), backend.State().String()).Inc()
		metrics.TridentBackendInfo.WithLabelValues(backend.GetDriverName(), backend.Name(),
			backend.BackendUUID()).Set(float64(1))
	}

	metrics.VolumesGauge.Reset()
	volumesTotalBytes := float64(0)
	metrics.VolumeAllocatedBytesGauge.Reset()
	for _, volume := range o.volumes {
		if volume == nil {
			continue
		}
		bytes, _ := strconv.ParseFloat(volume.Config.Size, 64)
		volumesTotalBytes += bytes
		if backend, err := o.getBackendByBackendUUID(volume.BackendUUID); err == nil {
			driverName := backend.GetDriverName()
			metrics.VolumesGauge.WithLabelValues(driverName,
				volume.BackendUUID,
				string(volume.State),
				string(volume.Config.VolumeMode)).Inc()

			metrics.VolumeAllocatedBytesGauge.WithLabelValues(driverName, backend.BackendUUID(), string(volume.State),
				string(volume.Config.VolumeMode)).Add(bytes)
		}
	}
	metrics.VolumesTotalBytesGauge.Set(volumesTotalBytes)

	metrics.SCGauge.Set(float64(len(o.storageClasses)))
	metrics.NodeGauge.Set(float64(o.nodes.Len()))
	metrics.SnapshotGauge.Reset()
	metrics.SnapshotAllocatedBytesGauge.Reset()
	for _, snapshot := range o.snapshots {
		vol := o.volumes[snapshot.Config.VolumeName]
		if vol != nil {
			if backend, err := o.getBackendByBackendUUID(vol.BackendUUID); err == nil {
				driverName := backend.GetDriverName()
				metrics.SnapshotGauge.WithLabelValues(
					driverName,
					vol.BackendUUID).Inc()
				metrics.SnapshotAllocatedBytesGauge.WithLabelValues(driverName, backend.BackendUUID()).
					Add(float64(snapshot.SizeBytes))
			}
		}
	}

	metrics.AutogrowPolicyGauge.Set(float64(len(o.autogrowPolicies)))

	metrics.AutogrowVolumes.Reset()
	for _, volume := range o.volumes {
		if volume == nil {
			continue
		}
		if volume.EffectiveAGPolicy.PolicyName != "" {
			metrics.AutogrowVolumes.WithLabelValues(
				volume.EffectiveAGPolicy.PolicyName,
				volume.Config.StorageClass,
			).Inc()
		}
	}
	for _, volume := range o.subordinateVolumes {
		if volume == nil {
			continue
		}
		if volume.EffectiveAGPolicy.PolicyName != "" {
			metrics.AutogrowVolumes.WithLabelValues(
				volume.EffectiveAGPolicy.PolicyName,
				volume.Config.StorageClass,
			).Inc()
		}
	}
}

func (o *TridentOrchestrator) handleFailedTransaction(ctx context.Context, v *storage.VolumeTransaction) error {
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
				// If the volume gets deleted before the snapshot, the snapshot deletion returns "NotFoundError".
				err := backend.DeleteSnapshot(ctx, v.SnapshotConfig, v.Config)
				if err != nil && !errors.IsUnsupportedError(err) && !errors.IsNotFoundError(err) {
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

	case storage.VolumeCreating:
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
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

	commonConfig.Flags[FlagConcurrent] = "false"

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

	sb, err := factory.NewStorageBackendForConfig(
		ctx, configInJSON, configRef, backendUUID, commonConfig, backendSecret,
	)

	if commonConfig.UserState != "" {
		// If the userState field is present in tbc/backend.json, then update the userBackendState.
		if err = o.updateUserBackendState(ctx, &sb, commonConfig.UserState, false); err != nil {
			return nil, err
		}
	}

	return sb, err
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
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
		return nil, errors.NotFoundError("backend %v was not found", backendUUID)
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
		if !isCRDContext(ctx) && !isPeriodicContext(ctx) {
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
			return nil, errors.UnsupportedConfigError(
				"backend '%v' update initiated using an invalid configRef, it is associated with configRef "+
					"'%v' and not '%v'", originalBackend.Name(), originalConfigRef, callingConfigRef)
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
		err := errors.New("invalid backend update")
		Logc(ctx).WithField("error", err).Error("Backend update failed.")
		return nil, err
	case updateCode.Contains(storage.InvalidVolumeAccessInfoChange):
		err := errors.New("updating the data plane IP address isn't currently supported")
		Logc(ctx).WithField("error", err).Error("Backend update failed.")
		return nil, err
	case updateCode.Contains(storage.PrefixChange):
		err := errors.UnsupportedConfigError("updating the storage prefix isn't currently supported")
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
			volumeExists := backend.Driver().Get(ctx, vol.Config) == nil
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
			o.backends[backend.BackendUUID()].Volumes().Store(volName, vol)
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
	ctx context.Context, backendName, backendState, userBackendState string,
) (backendExternal *storage.BackendExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("backend_update_state", &err)()

	// Extra check to ensure exactly one is set.
	if (backendState == "" && userBackendState == "") || (backendState != "" && userBackendState != "") {
		return nil, fmt.Errorf("exactly one of backendState or userBackendState must be set")
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	defer o.updateMetrics()

	// First, check whether the backend exists.
	backendUUID, err := o.getBackendUUIDByBackendName(backendName)
	if err != nil {
		return nil, err
	}

	backend, found := o.backends[backendUUID]
	if !found {
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

	return backend.ConstructExternal(ctx), o.storeClient.UpdateBackend(ctx, backend)
}

func (o *TridentOrchestrator) updateUserBackendState(
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
				return fmt.Errorf("updating via tridentctl is not allowed when `userState` field is set in the tbc of the backend")
			} else {
				// If the userState has been updated via tridentctl,
				//    then in the config section of tbe, userState will be shown empty.
				// We will only come to this section of the code when a backend is created via tridentctl using backend.json,
				//    and hasn't been linked to any of the tbc yet.
				commonConfig.UserState = ""
			}
		}
	}

	userBackendState = strings.ToLower(userBackendState)
	newUserBackendState := storage.UserBackendState(userBackendState)

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
			return fmt.Errorf("the backend '%s' is currently not in any of the expected states: "+
				"offline, online, or failed. Its current state is '%s'", backend.Name(), backend.State())
		}
	}

	// Update the user-backend state.
	backend.SetUserState(newUserBackendState)

	return nil
}

func (o *TridentOrchestrator) updateBackendState(
	ctx context.Context, sb *storage.Backend, backendState string,
) (err error) {
	backend := *sb
	Logc(ctx).WithFields(LogFields{
		"backendName":      backend.Name(),
		"userBackendState": backendState,
	}).Debug("updateBackendState")

	backendState = strings.ToLower(backendState)
	newBackendState := storage.BackendState(backendState)

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

func (o *TridentOrchestrator) getBackendUUIDByBackendName(backendName string) (string, error) {
	backendUUID := ""
	for _, b := range o.backends {
		if b.Name() == backendName {
			backendUUID = b.BackendUUID()
			return backendUUID, nil
		}
	}
	return "", errors.NotFoundError("backend %v was not found", backendName)
}

func (o *TridentOrchestrator) getBackendByBackendName(backendName string) (storage.Backend, error) {
	for _, b := range o.backends {
		if b.Name() == backendName {
			return b, nil
		}
	}
	return nil, errors.NotFoundError("backend %v was not found", backendName)
}

func (o *TridentOrchestrator) getBackendByConfigRef(configRef string) (storage.Backend, error) {
	for _, b := range o.backends {
		if b.ConfigRef() == configRef {
			return b, nil
		}
	}
	return nil, errors.NotFoundError("backend based on configRef '%v' was not found", configRef)
}

func (o *TridentOrchestrator) getBackendByBackendUUID(backendUUID string) (storage.Backend, error) {
	backend := o.backends[backendUUID]
	if backend != nil {
		return backend, nil
	}
	return nil, errors.NotFoundError("backend uuid %v was not found", backendUUID)
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	backendUUID, err := o.getBackendUUIDByBackendName(backendName)
	if err != nil {
		return nil, err
	}
	backend, found := o.backends[backendUUID]
	if !found {
		return nil, errors.NotFoundError("backend %v was not found", backendName)
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

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

// GetResizeDeltaForBackend returns the resize delta (bytes) for the backend.
// Returns (0, err) when the orchestrator is not ready (e.g. bootstrap error).
func (o *TridentOrchestrator) GetResizeDeltaForBackend(ctx context.Context, backendUUID string) (int64, error) {
	if o.bootstrapError != nil {
		return 0, o.bootstrapError
	}
	o.mutex.Lock()
	defer o.mutex.Unlock()
	backend, err := o.getBackendByBackendUUID(backendUUID)
	if err != nil {
		return 0, nil
	}
	return backend.GetResizeDeltaBytes(), nil
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

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
	if ctx.Err() != nil {
		return ctx.Err()
	}
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
	if ctx.Err() != nil {
		return ctx.Err()
	}
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
		return errors.NotFoundError("backend %s not found", backendName)
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

	backend.SetOnline(false) // TODO eventually remove
	backend.SetState(storage.Deleting)
	storageClasses := make(map[string]*storageclass.StorageClass)
	backend.StoragePools().Range(func(_, v interface{}) bool {
		storagePool := v.(storage.Pool)
		for _, scName := range storagePool.StorageClasses() {
			storageClasses[scName] = o.storageClasses[scName]
		}
		storagePool.SetStorageClasses([]string{})
		return true
	})
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
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	var b storage.Backend
	if b, err = o.getBackendByBackendUUID(backendUUID); err != nil {
		return errors.NotFoundError("backend with UUID '%s' not found", backendUUID)
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
	ctx = NewContextBuilder(ctx).WithLayer(LogLayerCore).BuildContext()

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_add", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
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
	protocol, err := getProtocol(ctx, volumeConfig.VolumeMode, volumeConfig.AccessMode, volumeConfig.Protocol)
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

	// addVolumeAfterAddVolumeTxn allows fault injection for automated testing.
	if err := addVolumeAfterAddVolumeTxn.Inject(); err != nil {
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
		backend.Driver().CreatePrepare(ctx, mutableConfig, pool)

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
			if errors.IsVolumeCreatingError(err) {
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
		vol     *storage.Volume
	)

	if txn.Op != storage.VolumeCreating {
		return nil, errors.New("wrong transaction type for addVolumeRetry")
	}

	volumeConfig := &txn.VolumeCreatingConfig.VolumeConfig

	backend, found := o.backends[txn.VolumeCreatingConfig.BackendUUID]
	if !found {
		// Should never get here but just to be safe
		return nil, errors.NotFoundError("backend %s for volume %s not found",
			txn.VolumeCreatingConfig.BackendUUID, volumeConfig.Name)
	}

	p, found := backend.StoragePools().Load(txn.VolumeCreatingConfig.Pool)
	if !found {
		return nil, errors.NotFoundError("pool %s for backend %s not found",
			txn.VolumeCreatingConfig.Pool, txn.VolumeCreatingConfig.BackendUUID)
	}
	pool := p.(storage.Pool)

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
		if errors.IsVolumeCreatingError(err) {
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

	// Resolve and set the EffectiveAGPolicy before saving to persistent store
	effectiveAGPolicy, policyErr := o.resolveAndSetEffectiveAutogrowPolicy(ctx, vol, "volume creation", false)

	// Add new volume to persistent store
	if err = o.storeClient.AddVolume(ctx, vol); err != nil {
		return nil, err
	}

	// Update internal cache and return external form of the new volume
	o.volumes[vol.Config.Name] = vol

	// Associate volume with Autogrow policy if Autogrow policy is present and Autogrow policy name is not empty
	if policyErr == nil && effectiveAGPolicy.PolicyName != "" {
		o.associateVolumeWithAutogrowPolicyInternal(ctx, vol.Config.Name, vol.EffectiveAGPolicy.PolicyName)
	}

	externalVol = vol.ConstructExternal()
	return externalVol, nil
}

// UpdateVolume updates the allowed fields of a volume in the backend, persistent store and cache.
func (o *TridentOrchestrator) UpdateVolume(
	ctx context.Context, volumeName string,
	volumeUpdateInfo *models.VolumeUpdateInfo,
) error {
	fields := LogFields{
		"Method":     "UpdateVolume",
		"Type":       "TridentOrchestrator",
		"volume":     volumeName,
		"updateInfo": volumeUpdateInfo,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> UpdateVolume")
	defer Logc(ctx).WithFields(fields).Debug("<<<< UpdateVolume")

	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	if volumeUpdateInfo == nil {
		err := errors.InvalidInputError(fmt.Sprintf("no volume update information provided for volume %v", volumeName))
		Logc(ctx).WithError(err).Error("Failed to update volume")
		return err
	}

	genericUpdateErr := fmt.Sprintf("failed to update volume %v", volumeName)

	// Get the volume
	volume, found := o.volumes[volumeName]
	if !found {
		err := errors.NotFoundError("volume %v was not found", volumeName)
		Logc(ctx).WithError(err).Error(genericUpdateErr)
		return err
	}

	// Get the backend
	backend, err := o.getBackendByBackendUUID(volume.BackendUUID)
	if err != nil {
		Logc(ctx).WithError(err).Error(genericUpdateErr)
		return err
	}

	// Update volume
	updatedVols, err := backend.UpdateVolume(ctx, volume.Config, volumeUpdateInfo)
	if err != nil {
		Logc(ctx).WithError(err).Error(genericUpdateErr)
		return err
	}

	if updatedVols != nil {
		// Update persistent layer
		for _, v := range updatedVols {
			err := o.updateVolumeOnPersistentStore(ctx, v)
			if err != nil {
				Logc(ctx).WithError(err).Errorf(genericUpdateErr)
				return err
			}
		}

		// Update cache
		for name, updatedVol := range updatedVols {
			o.volumes[name] = updatedVol
		}
	}

	return nil
}

// UpdateVolumeLUKSPassphraseNames updates the LUKS passphrase names stored on a volume in the cache and persistent store.
func (o *TridentOrchestrator) UpdateVolumeLUKSPassphraseNames(
	ctx context.Context, volume string, passphraseNames *[]string,
) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
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
			return nil, errors.NotFoundError("cloning subordinate volume %s is not allowed",
				volumeConfig.CloneSourceVolume)
		}
		return nil, errors.NotFoundError("source volume not found: %s", volumeConfig.CloneSourceVolume)
	}

	// Check if the source volume's backend is honored by the target storage class, only if the orchestrator
	// is not in Docker plugin mode. In Docker plugin mode, the storage class of source and clone volume
	// will be different.
	if !isDockerPluginMode() && volumeConfig.StorageClass != sourceVolume.Config.StorageClass {
		srcBackend, srcBackendFound := o.backends[sourceVolume.BackendUUID]
		dstSC, dstSCFound := o.storageClasses[volumeConfig.StorageClass]
		// If the source backend is not found in the cache, or the destination storage class is not found in the cache,
		// or the destination storage class is not added to the source backend,
		// then return an error as cloning across different storage classes that have no common backends is not allowed.
		if !srcBackendFound || !dstSCFound || !dstSC.IsAddedToBackend(srcBackend, volumeConfig.StorageClass) {
			return nil, errors.MismatchedStorageClassError("clone volume %s from source volume %s with"+
				" different storage classes that have no common backends is not allowed",
				volumeConfig.Name, volumeConfig.CloneSourceVolume)
		}
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
		return nil, errors.NotFoundError(
			"source volume's volume-mode (%s) is incompatible with requested clone's volume-mode (%s)",
			sourceVolume.Config.VolumeMode, volumeConfig.VolumeMode)
	}

	// Get the source backend
	Logc(ctx).WithFields(LogFields{
		"backendUUID": sourceVolume.BackendUUID,
	}).Trace("Checking if the source volume's backend is in the backends cache.")
	if backend, found = o.backends[sourceVolume.BackendUUID]; !found {
		// Should never get here but just to be safe
		return nil, errors.NotFoundError("backend %s for the source volume not found: %s",
			sourceVolume.BackendUUID, volumeConfig.CloneSourceVolume)
	}

	pool = storage.NewStoragePool(backend, "")

	// Clone the source config, as most of its attributes will apply to the clone
	cloneConfig := sourceVolume.Config.ConstructClone()

	internalName := ""
	internalID := ""

	Logc(ctx).WithFields(LogFields{
		"CloneName":              volumeConfig.Name,
		"SourceVolume":           volumeConfig.CloneSourceVolume,
		"SourceVolumeInternal":   sourceVolume.Config.InternalName,
		"SourceSnapshot":         volumeConfig.CloneSourceSnapshot,
		"SourceSnapshotInternal": volumeConfig.CloneSourceSnapshotInternal,
		"Qos":                    volumeConfig.Qos,
		"QosType":                volumeConfig.QosType,
	}).Trace("Adding attributes from the request which will affect clone creation.")

	cloneConfig.Name = volumeConfig.Name
	cloneConfig.InternalName = internalName
	cloneConfig.InternalID = internalID
	cloneConfig.CloneSourceVolume = volumeConfig.CloneSourceVolume
	cloneConfig.CloneSourceVolumeInternal = sourceVolume.Config.InternalName
	cloneConfig.CloneSourceSnapshot = volumeConfig.CloneSourceSnapshot
	cloneConfig.CloneSourceSnapshotInternal = volumeConfig.CloneSourceSnapshotInternal
	cloneConfig.Qos = volumeConfig.Qos
	cloneConfig.QosType = volumeConfig.QosType
	cloneConfig.RequestedAutogrowPolicy = volumeConfig.RequestedAutogrowPolicy
	// Clear these values as they were copied from the source volume Config
	cloneConfig.SubordinateVolumes = make(map[string]interface{})
	cloneConfig.ShareSourceVolume = ""
	cloneConfig.LUKSPassphraseNames = sourceVolume.Config.LUKSPassphraseNames
	// Override this value only if SplitOnClone has been defined in clone volume's config
	if volumeConfig.SplitOnClone != "" {
		cloneConfig.SplitOnClone = volumeConfig.SplitOnClone
	}
	// Override this value only if SecureSMB is enabled in clone volume's config
	if volumeConfig.SecureSMBEnabled {
		cloneConfig.SecureSMBEnabled = volumeConfig.SecureSMBEnabled
		cloneConfig.SMBShareACL = volumeConfig.SMBShareACL
	}
	cloneConfig.ReadOnlyClone = volumeConfig.ReadOnlyClone
	cloneConfig.Namespace = volumeConfig.Namespace
	cloneConfig.RequestName = volumeConfig.RequestName

	// If skipRecoveryQueue is set for the clone, use it. If not, default to the source volume's setting.
	if volumeConfig.SkipRecoveryQueue != "" {
		cloneConfig.SkipRecoveryQueue = volumeConfig.SkipRecoveryQueue
	}

	// Override tiering settings from clone volume config if provided
	if volumeConfig.TieringPolicy != "" {
		cloneConfig.TieringPolicy = volumeConfig.TieringPolicy
	}
	if volumeConfig.TieringMinimumCoolingDays != "" {
		cloneConfig.TieringMinimumCoolingDays = volumeConfig.TieringMinimumCoolingDays
	}

	// Empty out the export policy. It will be set in the backend driver.
	cloneConfig.ExportPolicy = ""

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
			snapshotID := storage.MakeSnapshotID(cloneConfig.CloneSourceVolume, cloneConfig.CloneSourceSnapshot)
			sourceSnapshot, found := o.snapshots[snapshotID]
			if !found || sourceSnapshot == nil {
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

		// If no internal snapshot name can be set by this point, fail immediately.
		// Attempting a clone will fail because backends rely on the internal snapshot name.
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
	// is because pool names are autogenerated based and are based on their relative arrangement.
	sourceVolumePoolName := sourceVolume.Pool
	if sourceVolumePoolName != drivers.UnsetPool {
		if sourceVolumeStoragePool, ok := backend.StoragePools().Load(sourceVolumePoolName); ok {
			pool = sourceVolumeStoragePool.(storage.Pool)
		}
	}

	// Create the backend-specific internal names so they are saved in the transaction
	backend.Driver().CreatePrepare(ctx, cloneConfig, pool)

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
		if errors.IsVolumeCreatingError(err) {
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
		return nil, errors.NotFoundError("source volume not found: %s", cloneConfig.CloneSourceVolume)
	}

	backend, found = o.backends[txn.VolumeCreatingConfig.BackendUUID]
	if !found {
		// Should never get here but just to be safe
		return nil, errors.NotFoundError("backend %s for volume %s not found",
			txn.VolumeCreatingConfig.BackendUUID, cloneConfig.Name)
	}

	// Try to place the cloned volume in the same pool as the source.  This doesn't always work,
	// as may be the case with imported volumes or virtual pools, so drivers must tolerate a nil
	// or non-existent pool.
	sourceVolumePoolName := txn.VolumeCreatingConfig.Pool
	if sourceVolumePoolName != drivers.UnsetPool {
		if sourceVolumeStoragePool, ok := backend.StoragePools().Load(sourceVolumePoolName); ok {
			pool = sourceVolumeStoragePool.(storage.Pool)
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
		if errors.IsVolumeCreatingError(err) {
			Logc(ctx).WithFields(logFields).Warn("Volume still creating on this backend.")
		} else {
			Logc(ctx).WithFields(logFields).Error("addVolumeRetry failed on this backend.")
		}

		return nil, err
	}

	// Volume creation succeeded, so register it and return the result
	return o.addVolumeFinish(ctx, txn, vol, backend, pool)
}

// GetVolumeForImport is used by volume import so it doesn't check core's o.volumes to see if the
// volume exists or not. Instead it asks the driver if the volume exists before requesting
// the volume size.  Accepts a backend-specific string that a driver may use to find a volume.
// Returns the VolumeExternal representation of the volume.
func (o *TridentOrchestrator) GetVolumeForImport(
	ctx context.Context, volumeID, backendName string,
) (volExternal *storage.VolumeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("volume_get_for_import", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	Logc(ctx).WithFields(LogFields{
		"volumeID":    volumeID,
		"backendName": backendName,
	}).Debug("Orchestrator#GetVolumeForImport")

	backendUUID, err := o.getBackendUUIDByBackendName(backendName)
	if err != nil {
		return nil, err
	}
	backend, ok := o.backends[backendUUID]
	if !ok {
		return nil, errors.NotFoundError("backend %s not found", backendName)
	}

	volExternal, err = backend.GetVolumeForImport(ctx, volumeID)
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
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	for _, vol := range o.volumes {
		if vol.Config.InternalName == volumeInternal {
			return vol.Config.Name, nil
		}
	}
	return "", errors.NotFoundError("volume %s not found", volumeInternal)
}

func (o *TridentOrchestrator) validateImportVolume(ctx context.Context, volumeConfig *storage.VolumeConfig) error {
	backend, err := o.getBackendByBackendUUID(volumeConfig.ImportBackendUUID)
	if err != nil {
		return fmt.Errorf("could not find backend; %v", err)
	}

	originalName := volumeConfig.ImportOriginalName
	backendUUID := volumeConfig.ImportBackendUUID

	extantVol, err := backend.GetVolumeForImport(ctx, originalName)
	if err != nil {
		return errors.NotFoundError("volume %s was not found", originalName)
	}

	requestedSize, err := strconv.ParseInt(volumeConfig.Size, 10, 64)
	if err != nil {
		return fmt.Errorf("could not determine requested size to import")
	}
	actualSize, err := strconv.ParseInt(extantVol.Config.Size, 10, 64)
	if err != nil {
		return fmt.Errorf("could not determine actual size of the volume being imported")
	}
	if actualSize < requestedSize {
		Logc(ctx).WithFields(LogFields{
			"requestedSize": requestedSize,
			"actualSize":    actualSize,
		}).Error("Import request size is more than actual size.")
		return errors.UnsupportedCapacityRangeError(errors.New("requested size is more than actual size"))
	}
	for volumeName, managedVol := range o.volumes {
		if managedVol.Config.InternalName == extantVol.Config.InternalName && managedVol.BackendUUID == backendUUID {
			return errors.FoundError("PV %s already exists for volume %s", originalName, volumeName)
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
	if len(volumeConfig.Protocol) == 0 {
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	defer o.updateMetrics()

	Logc(ctx).WithFields(LogFields{
		"volumeConfig": volumeConfig,
		"backendUUID":  volumeConfig.ImportBackendUUID,
	}).Debug("Orchestrator#ImportVolume")

	backend, ok := o.backends[volumeConfig.ImportBackendUUID]
	if !ok {
		return nil, errors.NotFoundError("backend %s not found", volumeConfig.ImportBackendUUID)
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

	// Resolve the effective Autogrow policy for the volume and associate the volume with the Autogrow policy
	effectiveAGPolicy, policyErr := o.resolveAndSetEffectiveAutogrowPolicy(ctx, volume, "volume import", false)

	err = o.storeClient.AddVolume(ctx, volume)
	if err != nil {
		return nil, fmt.Errorf("failed to persist imported volume data: %v", err)
	}
	o.volumes[volumeConfig.Name] = volume

	// Associate only if policy exists and if effectiveAGPolicy.PolicyName is not empty
	if policyErr == nil && effectiveAGPolicy.PolicyName != "" {
		o.associateVolumeWithAutogrowPolicyInternal(ctx, volume.Config.Name, volume.EffectiveAGPolicy.PolicyName)
	}

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
	if errors.IsVolumeCreatingError(err) {
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
		err = errors.New(strings.Join(errList, ", "))
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
	if err == nil || !errors.IsVolumeCreatingError(err) {
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
		return errors.NotFoundError("backend %s not found", volumeConfig.ImportBackendUUID)
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
		err = errors.New(strings.Join(errList, ", "))
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

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
	return nil, errors.NotFoundError("volume %v was not found", volumeName)
}

// UpdateVolumeAutogrowStatus updates the volume's AutogrowStatus in the in-memory cache
// and in the persistent store.
func (o *TridentOrchestrator) UpdateVolumeAutogrowStatus(
	ctx context.Context, volumeName string, status *models.VolumeAutogrowStatus,
) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)
	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var volume *storage.Volume
	if v, found := o.volumes[volumeName]; found {
		volume = v
	} else if v, found := o.subordinateVolumes[volumeName]; found {
		volume = v
	} else {
		return errors.NotFoundError("volume %v was not found", volumeName)
	}
	volume.AutogrowStatus = status
	return o.updateVolumeOnPersistentStore(ctx, volume)
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
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

		// Store deletion succeeded - now safe to disassociate from Autogrow policy
		if volume.EffectiveAGPolicy.PolicyName != "" {
			o.disassociateVolumeFromAutogrowPolicyInternal(ctx, volumeName, volume.EffectiveAGPolicy.PolicyName)
		}

		delete(o.volumes, volumeName)
		return nil
	}

	// Note that this call will only return an error if the backend actually
	// fails to delete the volume.  If the volume does not exist on the backend,
	// the driver will not return an error.  Thus, we're fine.
	if err := volumeBackend.RemoveVolume(ctx, volume.Config); err != nil {
		if !errors.IsNotManagedError(err) {
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

	// Store deletion succeeded - now safe to disassociate from Autogrow policy
	if volume.EffectiveAGPolicy.PolicyName != "" {
		o.disassociateVolumeFromAutogrowPolicyInternal(ctx, volumeName, volume.EffectiveAGPolicy.PolicyName)
	}

	// Check if we need to remove a soft-deleted source volume because a subordinate was deleted first.
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
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	if _, ok := o.subordinateVolumes[volumeName]; ok {
		return o.deleteSubordinateVolume(ctx, volumeName)
	}

	volume, ok := o.volumes[volumeName]
	if !ok {
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
			err = errors.New(strings.Join(errList, ", "))
		}
	}()

	return o.deleteVolume(ctx, volumeName)
}

func (o *TridentOrchestrator) PublishVolume(
	ctx context.Context, volumeName string, publishInfo *models.VolumePublishInfo,
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
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return o.publishVolume(ctx, volumeName, publishInfo)
}

func (o *TridentOrchestrator) publishVolume(
	ctx context.Context, volumeName string, publishInfo *models.VolumePublishInfo,
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
		// The inclusion of all volume publications for the source and its subordinates will produce the correct
		// set of exported nodes.
		if volume, ok = o.volumes[volume.Config.ShareSourceVolume]; !ok {
			return errors.NotFoundError("source volume for subordinate volumes %s not found",
				volumeName)
		}
	} else if volume, ok = o.volumes[volumeName]; !ok {
		return errors.NotFoundError("volume %s not found", volumeName)
	}

	if volume.State.IsDeleting() {
		return errors.VolumeStateError(fmt.Sprintf("volume %s is deleting", volumeName))
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
		if nodeInfo.PublicationState != models.NodeClean {
			return errors.NodeNotSafeToPublishForBackendError(publishInfo.HostName, backend.GetDriverName())
		}
	}

	// Check if the publication already exists.
	publication, found := o.volumePublications.TryGet(volumeName, publishInfo.HostName)
	if !found {
		Logc(ctx).WithFields(fields).Debug("Volume publication record not found; generating a new one.")
		publication = generateVolumePublication(volumeName, publishInfo, volume.Config)

		// Bail out if the publication is nil at this point, or we fail to add the publication to the store and cache.
		if err := o.addVolumePublication(ctx, publication); err != nil || publication == nil {
			msg := "error saving volume publication record"
			Logc(ctx).WithError(err).Error(msg)
			return errors.New(msg)
		}
	} else {
		// VP already exists, we may use this opportunity to update the VP if it's stale
		syncNeeded := syncVolumePublicationFields(volume, publication)
		if syncNeeded {
			// Update the cache
			if err := o.volumePublications.Set(publication.VolumeName, publication.NodeName, publication); err != nil {
				Logc(ctx).WithFields(LogFields{
					"volumeName": publication.VolumeName,
					"nodeName":   publication.NodeName,
				}).WithError(err).Error("Failed to update VP cache")
				return fmt.Errorf("failed to update volume publication record to cache")
			}

			// Update the store
			if err := o.storeClient.UpdateVolumePublication(ctx, publication); err != nil {
				Logc(ctx).WithFields(LogFields{
					"publication": publication.Name,
					"volumeName":  publication.VolumeName,
					"nodeName":    publication.NodeName,
				}).WithError(err).Error("Failed to sync VP fields to store")
				return fmt.Errorf("failed to update volume publication record to store")
			}
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
		return errors.FoundError("%s", msg)
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

// setPublicationsSynced marks if volume publications have been synced or not and updates it in the store.
func (o *TridentOrchestrator) setPublicationsSynced(ctx context.Context, volumePublicationsSynced bool) error {
	version, err := o.storeClient.GetVersion(ctx)
	if err != nil {
		msg := "error getting Trident version object"
		Logc(ctx).WithError(err).Error(msg)
		return errors.New(msg)
	}
	version.PublicationsSynced = volumePublicationsSynced
	err = o.storeClient.SetVersion(ctx, version)
	if err != nil {
		msg := "error updating Trident version object"
		Logc(ctx).WithError(err).Error(msg)
		return errors.New(msg)
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
	if ctx.Err() != nil {
		return ctx.Err()
	}

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
			Logc(ctx).WithError(errors.NotFoundError("unable to get volume publication record")).WithFields(fields).Warn("Volume is not published to node; doing nothing.")
			return nil
		}
	} else if publication == nil {
		msg := "volumePublication is nil"
		Logc(ctx).WithFields(fields).Errorf(msg)
		return errors.New(msg)
	}

	// Get node attributes from the node ID
	nodeInfo, err := o.GetNode(ctx, nodeName)
	if err != nil {
		Logc(ctx).WithError(err).WithField("Node info not found for node ", nodeName)
		return err
	}
	publishInfo := &models.VolumePublishInfo{
		HostName:    nodeName,
		TridentUUID: o.uuid,
		HostNQN:     nodeInfo.NQN,
		HostIP:      nodeInfo.IPs,
	}

	volume, ok := o.subordinateVolumes[volumeName]
	if ok {
		// If volume is a subordinate, replace it with its source volume since that is what we will manipulate.
		// The inclusion of all volume publications for the source and it subordinates will produce the correct
		// set of exported nodes.
		sourceVol := volume.Config.ShareSourceVolume
		if volume, ok = o.volumes[volume.Config.ShareSourceVolume]; !ok {
			return errors.NotFoundError("source volume %s for subordinate volumes %s not found",
				sourceVol, volumeName)
		}
	} else if volume, ok = o.volumes[volumeName]; !ok {
		return errors.NotFoundError("volume %s not found", volumeName)
	}

	// Build list of nodes to which the volume remains published.
	// For read-only clones, we need to also consider publications of the source volume
	// since they share the same export policy.
	nodeMap := make(map[string]*models.Node)

	// Collect all volume names whose publications should be considered
	volumeNamesToCheck := []string{volume.Config.Name}
	unpublishVolumeName := volumeName

	// If this is a read-only clone, also check the source volume's publications
	sourceVolumeName := ""
	if volume.Config.ReadOnlyClone && volume.Config.CloneSourceVolume != "" {
		sourceVolumeName = volume.Config.CloneSourceVolume
		// Verify the source volume still exists before checking its publications
		if _, found := o.volumes[sourceVolumeName]; found {
			volumeNamesToCheck = append(volumeNamesToCheck, sourceVolumeName)
			Logc(ctx).WithFields(LogFields{
				"clone":  unpublishVolumeName,
				"source": sourceVolumeName,
			}).Debug("Read-only clone detected; checking source volume publications too.")
		} else {
			Logc(ctx).WithFields(LogFields{
				"clone":  unpublishVolumeName,
				"source": sourceVolumeName,
			}).Warn("Source volume for read-only clone not found; may have been deleted.")
		}
	}

	// Single loop to find:
	// 1. Sibling read-only clones (if this is a clone) - other clones of the same source
	// 2. Child read-only clones (if this is a source) - clones that have this volume as their source
	for volName, vol := range o.volumes {
		if !vol.Config.ReadOnlyClone || volName == unpublishVolumeName {
			continue
		}

		// Check if this is a sibling clone (same source as the volume being unpublished)
		if sourceVolumeName != "" && vol.Config.CloneSourceVolume == sourceVolumeName {
			volumeNamesToCheck = append(volumeNamesToCheck, volName)
			Logc(ctx).WithFields(LogFields{
				"clone":   unpublishVolumeName,
				"sibling": volName,
				"source":  sourceVolumeName,
			}).Debug("Found sibling read-only clone; checking its publications too.")
		} else if vol.Config.CloneSourceVolume == unpublishVolumeName {
			// Check if the volume being unpublished is the source of a clone
			volumeNamesToCheck = append(volumeNamesToCheck, volName)
			Logc(ctx).WithFields(LogFields{
				"source": unpublishVolumeName,
				"clone":  volName,
			}).Debug("Found read-only clone of this volume; checking its publications too.")
		}
	}

	// Collect publications from all related volumes
	for _, volName := range volumeNamesToCheck {
		for _, pub := range o.listVolumePublicationsForVolumeAndSubordinates(ctx, volName) {
			// Exclude the publication we are unpublishing.
			if pub.VolumeName == unpublishVolumeName && pub.NodeName == nodeName {
				continue
			}

			if n := o.nodes.Get(pub.NodeName); n == nil {
				Logc(ctx).WithField("node", pub.NodeName).Warning("Node not found during volume unpublish.")
				continue
			} else {
				nodeMap[pub.NodeName] = n
			}
		}
	}

	for _, n := range nodeMap {
		publishInfo.Nodes = append(publishInfo.Nodes, n)
	}

	// Build list of NVMe namespace UUIDs that remain published to this host.
	// This is used to determine if the host should be removed from a SuperSubsystem.
	// Only collect namespace UUIDs if this volume is NVMe (has a namespace UUID).
	if volume.Config.AccessInfo.NVMeNamespaceUUID != "" {
		publishInfo.HostNVMeNamespaceUUIDs = o.getPublishedNVMeNamespaceUUIDsOnNode(ctx, nodeName, volumeName)
	}
	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		// Not a not found error because this is not user input.
		return fmt.Errorf("backend %s not found", volume.BackendUUID)
	}

	// Unpublish the volume.
	if err := backend.UnpublishVolume(ctx, volume.Config, publishInfo); err != nil {
		if !errors.IsNotFoundError(err) {
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
		if !errors.IsNotFoundError(err) {
			Logc(ctx).WithError(err).Error(msg)
			return errors.New(msg)
		}
		Logc(ctx).WithError(err).Debugf("%s; publication not found, ignoring.", msg)
	}

	return nil
}

// getPublishedNVMeNamespaceUUIDsOnNode returns namespace UUIDs for all NVMe volumes
// that remain published to this node, excluding the specified volume.
func (o *TridentOrchestrator) getPublishedNVMeNamespaceUUIDsOnNode(
	ctx context.Context, nodeName, excludeVolumeName string,
) []string {
	namespaceUUIDs := make([]string, 0)

	// Get all publications for this node
	publications := o.volumePublications.ListPublicationsForNode(nodeName)

	for _, pub := range publications {
		// Skip the volume being unpublished
		if pub.VolumeName == excludeVolumeName {
			continue
		}

		vol, found := o.volumes[pub.VolumeName]
		if !found {
			if vol, found = o.subordinateVolumes[pub.VolumeName]; !found {
				continue
			}
		}

		// Check if this volume has an NVMe namespace UUID
		if vol.Config.AccessInfo.NVMeNamespaceUUID != "" {
			namespaceUUIDs = append(namespaceUUIDs, vol.Config.AccessInfo.NVMeNamespaceUUID)
		}
	}

	return namespaceUUIDs
}

// AttachVolume mounts a volume to the local host.  This method is currently only used by Docker,
// and it should be able to accomplish its task using only the data passed in; it should not need to
// use the storage controller API.  It may be assumed that this method always runs on the host to
// which the volume will be attached.
func (o *TridentOrchestrator) AttachVolume(
	ctx context.Context, volumeName, mountpoint string, publishInfo *models.VolumePublishInfo,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("volume_attach", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}

	volume, ok := o.volumes[volumeName]
	if !ok {
		return errors.NotFoundError("volume %s not found", volumeName)
	}
	if volume.State.IsDeleting() {
		return errors.VolumeStateError(fmt.Sprintf("volume %s is deleting", volumeName))
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
	dfOutput, dfOuputErr := o.fs.GetDFOutput(ctx)
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
		return o.mount.AttachNFSVolume(ctx, volumeName, mountpoint, publishInfo)
	} else {
		var err error
		if publishInfo.SANType == sa.NVMe {
			nvmeHandler := nvme.NewNVMeHandler()
			err = nvmeHandler.AttachNVMeVolumeRetry(ctx, volumeName, mountpoint, publishInfo, map[string]string{},
				nvme.NVMeAttachTimeout)
		}

		if publishInfo.SANType == sa.ISCSI {
			_, err = o.iscsi.AttachVolumeRetry(ctx, volumeName, mountpoint, publishInfo, map[string]string{},
				AttachISCSIVolumeTimeoutLong)
		}
		return err
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
		return errors.NotFoundError("volume %s not found", volumeName)
	}
	if volume.State.IsDeleting() {
		return errors.VolumeStateError(fmt.Sprintf("volume %s is deleting", volumeName))
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
	if err := o.mount.Umount(ctx, mountpoint); err != nil {
		// utils.Unmount is chrooted, therefore it does NOT need the /host
		return err
	}

	// Best effort removal of the mount point
	_ = os.Remove(hostMountpoint) // trident is not chrooted, therefore we must the /host if in plugin mode
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
			return nil, errors.NotFoundError("creating subordinate to a subordinate volume %s is not allowed",
				volumeConfig.ShareSourceVolume)
		}
		return nil, errors.NotFoundError("source volume %s for subordinate volume %s not found",
			volumeConfig.ShareSourceVolume, volumeConfig.Name)
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
	if backend, ok := o.backends[sourceVolume.BackendUUID]; !ok {
		return nil, errors.NotFoundError("backend %s for source volume %s not found",
			sourceVolume.BackendUUID, sourceVolume.Config.Name)
	} else if backend.GetProtocol(ctx) != config.File || sourceVolume.Config.AccessInfo.NfsPath == "" {
		return nil, fmt.Errorf("source volume %s for subordinate volume %s must be NFS",
			volumeConfig.ShareSourceVolume, volumeConfig.Name)
	}

	// Ensure requested volume size is not larger than the source
	sourceVolumeSize, err := strconv.ParseUint(sourceVolume.Config.Size, 10, 64)
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

	volume = storage.NewVolume(volumeConfig, sourceVolume.BackendUUID, sourceVolume.Pool, false,
		storage.VolumeStateSubordinate)

	// Resolve and set the EffectiveAGPolicy before saving to persistent store
	effectiveAGPolicy, policyErr := o.resolveAndSetEffectiveAutogrowPolicy(ctx, volume, "subordinate volume creation", false)

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
	// Associate volume with Autogrow policy if Autogrow policy is present and Autogrow policy name is not empty
	if policyErr == nil && effectiveAGPolicy.PolicyName != "" {
		o.associateVolumeWithAutogrowPolicyInternal(ctx, volume.Config.Name, volume.EffectiveAGPolicy.PolicyName)
	}

	return volume.ConstructExternal(), nil
}

// deleteSubordinateVolume removes all references to a subordinate volume.  It does not delete any
// backing storage resources.
func (o *TridentOrchestrator) deleteSubordinateVolume(ctx context.Context, volumeName string) (err error) {
	volume, ok := o.subordinateVolumes[volumeName]
	if !ok {
		return errors.NotFoundError("subordinate volume %s not found", volumeName)
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

	// Store deletion succeeded - now safe to disassociate from Autogrow policy
	if volume.EffectiveAGPolicy.PolicyName != "" {
		o.disassociateVolumeFromAutogrowPolicyInternal(ctx, volumeName, volume.EffectiveAGPolicy.PolicyName)
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var sourceVolumes map[string]*storage.Volume

	if sourceVolumeName == "" {
		// If listing all subordinate volumes, start with the entire source volume map
		sourceVolumes = o.volumes
	} else {
		// If listing subordinate volumes for a single source volume, make a map with just the source volume
		if sourceVolume, ok := o.volumes[sourceVolumeName]; !ok {
			return nil, errors.NotFoundError("source volume %s not found", sourceVolumeName)
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if subordinateVolume, ok := o.subordinateVolumes[subordinateVolumeName]; !ok {
		return nil, errors.NotFoundError("subordinate volume %s not found", subordinateVolumeName)
	} else {
		parentVolumeName := subordinateVolume.Config.ShareSourceVolume
		if sourceVolume, ok := o.volumes[parentVolumeName]; !ok {
			return nil, errors.NotFoundError("source volume %s not found", parentVolumeName)
		} else {
			volume = sourceVolume.ConstructExternal()
		}

	}

	return volume, nil
}

func (o *TridentOrchestrator) resizeSubordinateVolume(ctx context.Context, volumeName, newSize string) error {
	volume, ok := o.subordinateVolumes[volumeName]
	if !ok || volume == nil {
		return errors.NotFoundError("subordinate volume %s not found", volumeName)
	}

	sourceVolume, ok := o.volumes[volume.Config.ShareSourceVolume]
	if !ok || sourceVolume == nil {
		return errors.NotFoundError("source volume %s for subordinate volume %s not found",
			volume.Config.ShareSourceVolume, volumeName)
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
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
			return nil, errors.NotFoundError("creating snapshot is not allowed on subordinate volume %s",
				snapshotConfig.VolumeName)
		}
		return nil, errors.NotFoundError("source volume %s not found", snapshotConfig.VolumeName)
	}
	if volume.State.IsDeleting() {
		return nil, errors.VolumeStateError(fmt.Sprintf("source volume %s is deleting", snapshotConfig.VolumeName))
	}

	// Get the backend
	if backend, ok = o.backends[volume.BackendUUID]; !ok {
		// Should never get here but just to be safe
		return nil, errors.NotFoundError("backend %s for the source volume not found: %s",
			volume.BackendUUID, snapshotConfig.VolumeName)
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
		if errors.IsMaxLimitReachedError(err) {
			return nil, errors.MaxLimitReachedError(fmt.Sprintf("failed to create snapshot %s for volume %s on backend %s: %v",
				snapshotConfig.Name, snapshotConfig.VolumeName, backend.Name(), err))
		}
		return nil, fmt.Errorf("failed to create snapshot %s for volume %s on backend %s: %v",
			snapshotConfig.Name, snapshotConfig.VolumeName, backend.Name(), err)
	}

	// Ensure we store all potential LUKS passphrases, there may have been a passphrase rotation during the snapshot process
	volume = o.volumes[snapshotConfig.VolumeName]
	for _, v := range volume.Config.LUKSPassphraseNames {
		if !collection.ContainsString(snapshotConfig.LUKSPassphraseNames, v) {
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

// CreateGroupSnapshot creates a snapshot of a volume group
func (o *TridentOrchestrator) CreateGroupSnapshot(
	ctx context.Context, config *storage.GroupSnapshotConfig,
) (externalGroupSnapshot *storage.GroupSnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	var (
		groupSnapshotTarget *storage.GroupSnapshotTargetInfo
		groupSnapshotter    storage.Backend
		groupSnapshot       *storage.GroupSnapshot
		snapshots           []*storage.Snapshot
		backendsToVolumes   map[string][]*storage.VolumeConfig
	)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("group_snapshot_create", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	defer o.updateMetrics()

	// pvc-UUID names of all pvcs in a volume group
	sourceVolumeIDs := config.GetVolumeNames()
	if len(sourceVolumeIDs) == 0 {
		return nil, errors.InvalidInputError("group snapshot must have at least one source volume")
	}

	// Get the backends from the volumes and determine if they support group snapshots
	// Map [backends] -> volumes
	backendsToVolumes = make(map[string][]*storage.VolumeConfig)
	// Get the volume and do simple validation
	for _, volumeID := range sourceVolumeIDs {

		// Check if the snapshot already exists, the snapshot will exist in our cache
		// snapshots are stored by their ID pvc-UUID/snapshot-GroupSnapshotUUID
		// For group snapshots, the snapshot name will be the same for every volume in the group but we'll be able to
		// differentiate based on the pvc if needed
		snapName, err := storage.ConvertGroupSnapshotID(config.ID())
		if err != nil {
			return nil, err
		}
		snapshotID := storage.MakeSnapshotID(volumeID, snapName)
		if _, ok := o.snapshots[snapshotID]; ok {
			return nil, fmt.Errorf("snapshot %s already exists", snapshotID)
		}

		v, ok := o.volumes[volumeID]
		if !ok {
			if _, ok = o.subordinateVolumes[volumeID]; ok {
				return nil, errors.NotFoundError(
					"creating snapshot is not allowed on subordinate volume %s", volumeID)
			}
			return nil, errors.NotFoundError("source volume %s not found", volumeID)
		}
		if v.State.IsDeleting() {
			return nil, errors.VolumeStateError(fmt.Sprintf("source volume %s is deleting", volumeID))
		}

		// Get the backend and check it can group snapshot
		b, ok := o.backends[v.BackendUUID]
		if !ok {
			// Should never get here but just to be safe
			return nil, errors.NotFoundError("backend for volume %s not found", volumeID)
		} else if !b.CanGroupSnapshot() {
			// Pre-filter backend support for group snapshot capability.
			// Fail fast if even a single backend cannot support group snapshot.
			return nil, errors.UnsupportedError(fmt.Sprintf(
				"backend %s for volume %s does not support group snapshots", b.Name(), volumeID))
		} else if !b.Online() {
			return nil, errors.New(fmt.Sprintf("backend for volume %s not online", volumeID))
		}
		backendsToVolumes[b.BackendUUID()] = append(backendsToVolumes[b.BackendUUID()], v.Config)
	}

	// Get the target info for each backend and the internal volume name from each source volume ID.
	for backendUUID, volumes := range backendsToVolumes {
		// Find the backend in cache.
		b, ok := o.backends[backendUUID]
		if !ok {
			return nil, errors.NotFoundError("backend not found")
		}

		// Ensure all the targets match.
		currentInfo, err := b.GetGroupSnapshotTarget(ctx, volumes)
		if err != nil {
			return nil, errors.New("failed to get storage target")
		}

		if groupSnapshotTarget == nil {
			groupSnapshotTarget = currentInfo
		}

		if err = currentInfo.Validate(); err != nil {
			// If the storage target is invalid, we cannot create a group snapshot, snapshotter should not retry.
			return nil, errors.InvalidInputError("invalid storage target for backend %s", b.Name())
		} else if !currentInfo.IsShared(groupSnapshotTarget) {
			// If the storage target is not shared, we cannot create a group snapshot, snapshotter should not retry.
			return nil, errors.InvalidInputError("storage target is not the same for all backends")
		}

		// Identify and remove duplicates for any source volumes that are shared across multiple backends.
		groupSnapshotTarget.AddVolumes(currentInfo.GetVolumes())

		// Set the group snapshotter to the last backend in the list since all the targets are the same,
		// entry point shouldn't matter.
		groupSnapshotter = b
	}

	// Fail fast if we don't have a group snapshotter by now.
	if groupSnapshotter == nil {
		return nil, errors.New("failed to set group snapshotter")
	}

	// Add a vol transaction with enough info about the whole group
	// Add transaction in case the operation must be rolled back later
	txn := &storage.VolumeTransaction{
		GroupSnapshotConfig: config,
		Op:                  storage.AddGroupSnapshot,
	}
	if err = o.AddVolumeTransaction(ctx, txn); err != nil {
		return nil, err
	}

	// Recovery function in case of error
	defer func() {
		err = o.addGroupSnapshotCleanup(ctx, err, backendsToVolumes, groupSnapshot, snapshots, txn, config)
	}()

	// Create the group snapshot in the backend.
	err = groupSnapshotter.CreateGroupSnapshot(ctx, config, groupSnapshotTarget)
	if err != nil {
		if errors.IsMaxLimitReachedError(err) {
			return nil, errors.MaxLimitReachedError(fmt.Sprintf("failed to create group snapshot %s: %v",
				config.ID(), err))
		}
		return nil, fmt.Errorf("failed to create group snapshot %s: %v", config.ID(), err)
	}

	// Process the group snapshot by backend and the set volumes relative to that backend.
	// Defer handling errors until after all backends have processed their snapshots because
	// the orchestrator needs to know which snapshots are present for each backend.
	var processingErrs error
	for backendUUID, volumes := range backendsToVolumes {
		b, ok := o.backends[backendUUID]
		if !ok {
			return nil, errors.NotFoundError("backend not found")
		}

		// Each backend should handle backend-specific processing with its own slice of volumes.
		snapshotsSlice, err := b.ProcessGroupSnapshot(ctx, config, volumes)
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"volumes":     volumes,
				"backendUUID": backendUUID,
			}).WithError(err).Debugf("Failed to process grouped snapshots for backend %s", b.Name())
			processingErrs = errors.Join(processingErrs, err)
		}

		// Build up the set of grouped snapshots across all volumes in each backend.
		snapshots = append(snapshots, snapshotsSlice...)
	}
	if processingErrs != nil {
		return nil, errors.Join(err, processingErrs)
	}

	// Construct the group snapshot from the config and grouped snapshots across all backends.
	groupSnapshot, err = groupSnapshotter.ConstructGroupSnapshot(ctx, config, snapshots)
	if err != nil {
		return nil, fmt.Errorf("failed to construct group snapshot %s: %v", config.ID(), err)
	}

	// Save references to new group snapshot and each snapshot
	if err = o.storeClient.AddGroupSnapshot(ctx, groupSnapshot); err != nil {
		return nil, err
	}
	o.groupSnapshots[groupSnapshot.ID()] = groupSnapshot

	// Save references to new snapshots and build a list of external snapshots.
	snapshotExternal := make([]*storage.SnapshotExternal, 0, len(groupSnapshot.GetSnapshotIDs()))
	for _, snap := range snapshots {
		if err = o.storeClient.AddSnapshot(ctx, snap); err != nil {
			return nil, err
		}
		o.snapshots[snap.ID()] = snap
		snapshotExternal = append(snapshotExternal, snap.ConstructExternal())
	}

	return groupSnapshot.ConstructExternal(), nil
}

// addGroupSnapshotCleanup is used as a deferred method from the group snapshot create method
// to clean up in case anything goes wrong during the operation.
func (o *TridentOrchestrator) addGroupSnapshotCleanup(
	ctx context.Context, err error, backendsToVolumes map[string][]*storage.VolumeConfig,
	groupSnapshot *storage.GroupSnapshot, snapshots []*storage.Snapshot, volTxn *storage.VolumeTransaction,
	groupSnapshotConfig *storage.GroupSnapshotConfig,
) error {
	var cleanupErr, txErr error
	if err != nil {
		// We failed somewhere. There are two possible cases:
		// 1.  We failed to create a snapshot and fell through to the end of the function.
		//     In this case, we don't need to roll anything back.
		// 2.  We failed to save the snapshots or group snapshot to the persistent store.
		//     In this case, we need to remove the snapshots from the backend.

		// We may have succeeded in adding some snapshots to the backends.
		// Because something failed, we must delete them.
		for backendUUID, volConfigs := range backendsToVolumes {
			backend, ok := o.backends[backendUUID]
			if !ok {
				cleanupErr = errors.Join(cleanupErr, errors.NotFoundError("backend not found"))
			}

			for _, volConfig := range volConfigs {
				var volName string
				for _, snap := range snapshots {
					volName, _, err = storage.ParseSnapshotID(snap.ID())
					if err != nil {
						cleanupErr = errors.Join(cleanupErr, err)
					} else if volName != volConfig.Name {
						continue
					}

					if deleteErr := backend.DeleteSnapshot(ctx, snap.Config, volConfig); deleteErr != nil {
						cleanupErr = fmt.Errorf("unable to delete snapshot from backend during cleanup: %w", deleteErr)
					}
				}
			}
		}
	}

	if cleanupErr == nil {
		// Only clean up the snapshot transaction if we've succeeded at cleaning up on the backend or if we didn't
		// need to do so in the first place.
		if deleteErr := o.DeleteVolumeTransaction(ctx, volTxn); deleteErr != nil {
			txErr = fmt.Errorf("unable to clean up group snapshot transaction: %w", deleteErr)
		}
	}

	if cleanupErr != nil || txErr != nil {
		// Remove the snapshot from memory, if it's there, so that the user can try to re-add.
		// This will trigger recovery code.
		delete(o.groupSnapshots, groupSnapshotConfig.ID())
		if groupSnapshot != nil {
			for _, snapID := range groupSnapshot.GetSnapshotIDs() {
				delete(o.snapshots, snapID)
			}
		}

		// Report on all errors we encountered.
		err = multierr.Append(err, cleanupErr)
		err = multierr.Append(err, txErr)
		Logc(ctx).Warnf(
			"Unable to clean up artifacts of group snapshot creation: %v. "+
				"Repeat creating the group snapshot or restart %v.",
			err, config.OrchestratorName)
	}

	return err
}

func (o *TridentOrchestrator) GetGroupSnapshot(
	ctx context.Context, groupSnapshotID string,
) (groupSnapshotExternal *storage.GroupSnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("group_snapshot_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return o.getGroupSnapshot(ctx, groupSnapshotID)
}

// getGroupSnapshot accepts a group snapshot ID and returns the group snapshot object.
// It assumes the caller has already acquired the orchestrator mutex.
func (o *TridentOrchestrator) getGroupSnapshot(
	_ context.Context, groupSnapshotID string,
) (*storage.GroupSnapshotExternal, error) {
	groupSnapshot, found := o.groupSnapshots[groupSnapshotID]
	if !found {
		return nil, errors.NotFoundError("group snapshot %s was not found", groupSnapshotID)
	}

	return groupSnapshot.ConstructExternal(), nil
}

func (o *TridentOrchestrator) ListGroupSnapshots(
	ctx context.Context,
) (groupSnapshots []*storage.GroupSnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("group_snapshot_list", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var errs error
	groupSnapshots = make([]*storage.GroupSnapshotExternal, 0, len(o.groupSnapshots))
	for id := range o.groupSnapshots {
		gs, err := o.getGroupSnapshot(ctx, id)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		groupSnapshots = append(groupSnapshots, gs)
	}

	if errs != nil {
		Logc(ctx).WithError(err).Error("Could not retrieve some group snapshots.")
		return nil, errs
	}

	sort.Sort(storage.ByGroupSnapshotExternalID(groupSnapshots))
	return groupSnapshots, nil
}

func (o *TridentOrchestrator) DeleteGroupSnapshot(ctx context.Context, groupName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("group_snapshot_delete", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	// Check if the group snapshot exists. If it doesn't, log an error and fail early.
	groupSnapshot, ok := o.groupSnapshots[groupName]
	if !ok {
		Logc(ctx).Warnf("Group snapshot '%s' not found.", groupName)
		return errors.NotFoundError("group snapshot '%s' not found", groupName)
	}
	snapshotIDs := groupSnapshot.GetSnapshotIDs()

	// Start a transaction to delete the group snapshot.
	txn := &storage.VolumeTransaction{
		GroupSnapshotConfig: groupSnapshot.GroupSnapshotConfig,
		Op:                  storage.DeleteGroupSnapshot,
	}
	if err = o.AddVolumeTransaction(ctx, txn); err != nil {
		return err
	}

	// Ensure delete transaction is updated or cleaned up.
	defer func() {
		errTxn := o.DeleteVolumeTransaction(ctx, txn)
		if errTxn != nil {
			Logc(ctx).WithFields(LogFields{
				"snapshotIDs": snapshotIDs,
				"groupName":   groupName,
				"error":       errTxn,
				"operation":   txn.Op,
			}).Warnf("Unable to delete group snapshot transaction. Repeat deletion using %s or restart %v.",
				config.OrchestratorClientName, config.OrchestratorName)
		}
		if err != nil || errTxn != nil {
			err = multierr.Append(err, errTxn)
		}
	}()

	// Delete the group snapshot
	return o.deleteGroupSnapshot(ctx, groupName)
}

// deleteGroupSnapshot does the necessary work to delete a group snapshot and all constituents entirely.
// It does not construct a transaction, nor does it take locks; it assumes that the caller will take care of both of these.
func (o *TridentOrchestrator) deleteGroupSnapshot(ctx context.Context, groupName string) error {
	if groupName == "" {
		return errors.New("empty group snapshot name")
	}

	groupSnapshot, ok := o.groupSnapshots[groupName]
	if !ok {
		return errors.NotFoundError("group snapshot %s not found", groupName)
	}

	Logc(ctx).WithFields(LogFields{
		"groupName":   groupName,
		"snapshotIDs": groupSnapshot.GetSnapshotIDs(),
	}).Debug("Deleting group snapshot.")

	// Delete each constituent snapshot from the backend and the store.
	var errs error
	for _, snapID := range groupSnapshot.GetSnapshotIDs() {
		// There are several things to check for this:
		//  1. Check if the snapshot is in the cache. If it's a cache-miss, do not delete.
		//  2. Check if the snapshot is a source for a read-only volume. If so, bail out.
		//  3. Check if the backend and source volume are non-existent. If so, delete the snapshot reference here.
		//  4. Delete each constituent snapshot from the backend, store and cache.

		// Check if this snapshot is in the cache. If not, log a warning and continue.
		snapshot, ok := o.snapshots[snapID]
		if !ok {
			Logc(ctx).WithFields(LogFields{
				"groupName":  groupName,
				"snapshotID": snapID,
			}).Warn("Could not find snapshot in group.")
			continue
		}
		volumeName := snapshot.Config.VolumeName
		snapshotName := snapshot.Config.Name

		// Check if this grouped snapshot is a source for a read-only volume. If so, return error.
		for _, vol := range o.volumes {
			volName := vol.Config.Name
			if vol.Config.ReadOnlyClone && vol.Config.CloneSourceSnapshot == snapshotName {
				Logc(ctx).WithFields(LogFields{
					"readyOnlyClone": volName,
					"snapshotSource": snapshotName,
				}).Error("Unable to safely delete source snapshot for read-only clone.")
				return errors.ConflictError("unable to delete source snapshot %s "+
					"for read-only clone %s", snapshotName, volName)
			}
		}

		// The next section handles the case where the source volume AND the backend for this snapshot
		// have been lost to Trident (possibly between installations) but the snapshot itself has not.
		// If either the backend or volume resources do not exist, delete the snapshot immediately.

		// Check if the backend and source volume are non-existent.
		// If so, we can delete the snapshot here and move onto the next one.
		volume, ok := o.volumes[volumeName]
		if !ok && !snapshot.State.IsMissingVolume() {
			Logc(ctx).WithFields(LogFields{
				"snapshotID":    snapID,
				"snapshotState": snapshot.State,
				"volumeName":    volumeName,
				"volumeExists":  ok,
			}).Error("Cannot delete snapshot; inaccurate snapshot state.")
			return errors.NotFoundError("volume %s not found", volumeName)
		}

		// At this point, we cannot rediscover the backend to remove the snapshot.
		var backend storage.Backend
		var backendUUID string
		if volume != nil {
			backendUUID = volume.BackendUUID
			backend, ok = o.backends[backendUUID]
			if !ok && !snapshot.State.IsMissingBackend() {
				Logc(ctx).WithFields(LogFields{
					"snapshotID":    snapID,
					"snapshotState": snapshot.State,
					"backendUUID":   backendUUID,
					"backendExists": ok,
				}).Error("Cannot delete snapshot; inaccurate snapshot state.")
				return errors.NotFoundError("backend %s not found", backendUUID)
			}
		}

		// Note that this block will only be entered in the case that the snapshot
		// is missing its source volume and backend.
		// In other words, the snapshot cannot be deleted in a backend if that backend reference doesn't exist
		// or cannot be deduced via source volume reference.
		if backend == nil || volume == nil {
			Logc(ctx).WithFields(LogFields{
				"snapshotID":           snapID,
				"snapshotInternalName": snapshot.Config.InternalName,
				"snapshotState":        snapshot.State,
				"volumeName":           volumeName,
				"internalVolumeName":   snapshot.Config.VolumeInternalName,
				"backendUUID":          backendUUID,
			}).Warn("Deleting snapshot reference without source volume or backend; snapshot may persist in storage system.")

			// Remove the snapshot from Trident cache and store and continue.
			if err := o.storeClient.DeleteSnapshot(ctx, snapshot); err != nil {
				return err
			}
			delete(o.snapshots, snapshot.ID())
			continue
		}

		// Delete each constituent snapshot from the backend, store and cache.
		// Reuse the pre-existing deleteSnapshot method for this.
		if err := o.deleteSnapshot(ctx, snapshot.Config); err != nil {
			if !errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(LogFields{
					"snapshotID": snapID,
					"groupName":  groupName,
					"volumeName": volumeName,
					"error":      err,
				}).WithError(err).Warn("Unable to delete snapshot.")
				errs = errors.Join(errs, err)
			}
			continue
		}
	}

	if errs != nil {
		Logc(ctx).Debug("Unable to delete some grouped snapshots.")
		return errs
	}

	// Only after deleting all constituent snapshots is it safe to delete the group snapshot reference.
	if err := o.storeClient.DeleteGroupSnapshot(ctx, groupSnapshot); err != nil {
		return err
	}
	delete(o.groupSnapshots, groupSnapshot.ID())

	return nil
}

// ImportSnapshot will retrieve the snapshot from the backend and persist the snapshot information.
func (o *TridentOrchestrator) ImportSnapshot(
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

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	defer o.updateMetrics()

	// Fail immediately if the internal name isn't set.
	if snapshotConfig.InternalName == "" {
		return nil, errors.InvalidInputError("snapshot internal name not found")
	}
	fields["internalName"] = snapshotConfig.InternalName

	if _, ok := o.snapshots[snapshotConfig.ID()]; ok {
		Logc(ctx).WithFields(fields).Warn("Snapshot already exists.")
		return nil, errors.FoundError("snapshot %s already exists", snapshotConfig.ID())
	}

	volume, err := o.getVolume(ctx, snapshotConfig.VolumeName)
	if err != nil {
		return nil, err
	}

	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		return nil, errors.NotFoundError("backend %s for volume %s not found",
			volume.BackendUUID, snapshotConfig.VolumeName)
	}
	fields["backendName"] = backend.Name()

	// Complete the snapshot config.
	snapshotConfig.VolumeInternalName = volume.Config.InternalName
	snapshotConfig.LUKSPassphraseNames = volume.Config.LUKSPassphraseNames
	snapshotConfig.ImportNotManaged = volume.Config.ImportNotManaged // Snapshots inherit the managed state of their volume

	// Query the storage backend for the snapshot.
	snapshot, err := backend.GetSnapshot(ctx, snapshotConfig, volume.Config)
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
		err = errors.New(strings.Join(errList, ", "))
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return o.getSnapshot(ctx, volumeName, snapshotName)
}

func (o *TridentOrchestrator) getSnapshot(
	ctx context.Context, volumeName, snapshotName string,
) (*storage.SnapshotExternal, error) {
	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)
	snapshot, found := o.snapshots[snapshotID]
	if !found {
		return nil, errors.NotFoundError("snapshot %v was not found", snapshotName)
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
		return snapshot, errors.NotFoundError("snapshot %s not found on volume %s",
			snapshot.Config.Name, snapshot.Config.VolumeName)
	}

	volume, ok := o.volumes[snapshot.Config.VolumeName]
	if !ok {
		return snapshot, errors.NotFoundError("volume %s not found", snapshot.Config.VolumeName)
	}

	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		return snapshot, errors.NotFoundError("backend %s not found", volume.BackendUUID)
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

// RestoreSnapshot restores a volume to the specified snapshot.  The caller is responsible for ensuring this is
// the newest snapshot known to the container orchestrator.  Any other snapshots that are newer than the specified
// one that are not known to the container orchestrator may be lost.
func (o *TridentOrchestrator) RestoreSnapshot(ctx context.Context, volumeName, snapshotName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("snapshot_restore", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return errors.NotFoundError("volume %s not found", volumeName)
	}

	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)
	snapshot, ok := o.snapshots[snapshotID]
	if !ok {
		return errors.NotFoundError("snapshot %s not found on volume %s", snapshotName, volumeName)
	}

	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		return errors.NotFoundError("backend %s not found", volume.BackendUUID)
	}

	logFields := LogFields{
		"volume":   snapshot.Config.VolumeName,
		"snapshot": snapshot.Config.Name,
		"backend":  backend.Name(),
	}

	// Ensure volume is not attached to any containers
	if len(o.volumePublications.ListPublicationsForVolume(volumeName)) > 0 {
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

// deleteSnapshot does the necessary work to delete a snapshot entirely.  It does
// not construct a transaction, nor does it take locks; it assumes that the caller will
// take care of both of these.
func (o *TridentOrchestrator) deleteSnapshot(ctx context.Context, snapshotConfig *storage.SnapshotConfig) error {
	snapshotID := snapshotConfig.ID()
	snapshot, ok := o.snapshots[snapshotID]
	if !ok {
		return errors.NotFoundError("snapshot %s not found on volume %s",
			snapshotConfig.Name, snapshotConfig.VolumeName)
	}

	volume, ok := o.volumes[snapshotConfig.VolumeName]
	if !ok {
		return errors.NotFoundError("volume %s not found", snapshotConfig.VolumeName)
	}

	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		return errors.NotFoundError("backend %s not found", volume.BackendUUID)
	}

	// Note that this call will only return an error if the backend actually
	// fails to delete the snapshot. If the snapshot does not exist on the backend,
	// the driver will not return an error. Thus, we're fine.
	if err := backend.DeleteSnapshot(ctx, snapshot.Config, volume.Config); err != nil {
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
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)
	snapshot, ok := o.snapshots[snapshotID]
	if !ok {
		return errors.NotFoundError("snapshot %s not found on volume %s", snapshotName, volumeName)
	} else if snapshot.IsGrouped() {
		// Only allow deletion of a grouped snapshot if the group snapshot is gone.
		groupName := snapshot.GroupSnapshotName()
		if _, ok := o.groupSnapshots[groupName]; ok {
			Logc(ctx).WithFields(LogFields{
				"snapshot":  snapshotID,
				"groupName": groupName,
			}).Warn("Snapshot could not be deleted because it is part of a group snapshot. Delete the group snapshot.")
			return errors.ConflictError("unable to delete snapshot: '%s' "+
				"while group: '%s' exists", snapshotID, groupName)
		}

		Logc(ctx).WithFields(LogFields{
			"snapshot":  snapshotID,
			"groupName": groupName,
		}).Debug("Snapshot is grouped but group snapshot no longer exists. Proceeding with deletion.")
	}

	volume, ok := o.volumes[volumeName]
	if !ok {
		if !snapshot.State.IsMissingVolume() {
			return errors.NotFoundError("volume %s not found", volumeName)
		}
	}

	// Note that this block will only be entered in the case that the snapshot
	// is missing its volume and the volume is nil. If the volume does not
	// exist, delete the snapshot and clean up, then return.
	if volume == nil {
		if err = o.storeClient.DeleteSnapshot(ctx, snapshot); err != nil {
			return err
		}
		delete(o.snapshots, snapshot.ID())
		return nil
	}

	// Check if the snapshot is a source for a read-only volume. If so, return error.
	for _, vol := range o.volumes {
		if vol.Config.ReadOnlyClone && vol.Config.CloneSourceSnapshot == snapshotName {
			return errors.ConflictError("unable to delete snapshot %s as it is a source "+
				"for read-only clone %s", snapshotName, vol.Config.Name)
		}
	}

	backend, ok := o.backends[volume.BackendUUID]
	if !ok {
		if !snapshot.State.IsMissingBackend() {
			return errors.NotFoundError("backend %s not found", volume.BackendUUID)
		}
	}

	// Note that this block will only be entered in the case that the snapshot
	// is missing its backend and the backend is nil. If the backend does not
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
			err = errors.New(strings.Join(errList, ", "))
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	snapshots = make([]*storage.SnapshotExternal, 0)
	for _, s := range o.snapshots {
		if s.Config.Name == snapshotName {
			snapshots = append(snapshots, s.ConstructExternal())
		}
	}
	sort.Sort(storage.BySnapshotExternalID(snapshots))
	return snapshots, nil
}

func (o *TridentOrchestrator) ListSnapshotsForGroup(
	ctx context.Context, groupName string,
) (snapshots []*storage.SnapshotExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("snapshot_list_by_group_name", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	fields := LogFields{"groupName": groupName}

	// Pre-provisioned grouped snapshots may be imported prior to importing the group snapshot reference.
	// Handle that case gracefully.
	if _, ok := o.groupSnapshots[groupName]; !ok {
		Logc(ctx).WithFields(fields).Warn("Group snapshot not found; attempting to list snapshots by name.")
	}

	snapshots = make([]*storage.SnapshotExternal, 0)
	for _, snapshot := range o.snapshots {
		if snapshot.Config.GroupSnapshotName == groupName {
			snapshots = append(snapshots, snapshot.ConstructExternal())
		}
	}

	if len(snapshots) == 0 {
		Logc(ctx).Errorf("No snapshots found for group snapshot '%s'.", groupName)
		return nil, errors.NotFoundError("no snapshots found for group snapshot '%s'", groupName)
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if _, ok := o.volumes[volumeName]; !ok {
		return nil, errors.NotFoundError("volume %s not found", volumeName)
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
		return nil, errors.NotFoundError("volume %s not found", volumeName)
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
	if ctx.Err() != nil {
		return ctx.Err()
	}
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
		backend.ClearVolumes()
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
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	if _, ok := o.subordinateVolumes[volumeName]; ok {
		return o.resizeSubordinateVolume(ctx, volumeName, newSize)
	}

	volume, found := o.volumes[volumeName]
	if !found {
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
			err = errors.New(strings.Join(errList, ", "))
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
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

	// Check if Storage Class has Autogrow policy
	autogrowPolicy := sc.GetAutogrowPolicy()

	var policyErr error

	if autogrowPolicy != "" {
		Logc(ctx).WithFields(LogFields{
			"storageClass":   sc.GetName(),
			"autogrowPolicy": autogrowPolicy,
		}).Info("StorageClass Autogrow policy added; updating affected volumes.")

		policyErr = o.upsertStorageClassAutogrowPolicyInternal(ctx, sc.GetName())
	}

	return sc.ConstructExternal(ctx), policyErr
}

func (o *TridentOrchestrator) UpdateStorageClass(
	ctx context.Context, scConfig *storageclass.Config,
) (scExternal *storageclass.External, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("storageclass_update", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	defer o.updateMetrics()

	newSC := storageclass.New(scConfig)
	scName := newSC.GetName()
	oldSC, ok := o.storageClasses[scName]
	if !ok {
		return nil, errors.NotFoundError("storage class %s not found", scName)
	}

	if err = o.storeClient.UpdateStorageClass(ctx, newSC); err != nil {
		return nil, err
	}

	// Remove storage class from backend map
	for _, storagePool := range oldSC.GetStoragePoolsForProtocol(ctx, config.ProtocolAny, config.ReadWriteOnce) {
		storagePool.RemoveStorageClass(scName)
	}

	// Add storage class to backend map
	added := 0
	for _, backend := range o.backends {
		added += newSC.CheckAndAddBackend(ctx, backend)
	}
	Logc(ctx).WithField("storageClass", scName).Infof("Storage class satisfied by %d storage pools.", added)

	// Update internal cache
	o.storageClasses[scName] = newSC

	//Check if Autogrow policy changed
	oldAutogrowPolicy := oldSC.GetAutogrowPolicy()
	newAutogrowPolicy := newSC.GetAutogrowPolicy()

	var policyErr error

	if oldAutogrowPolicy != newAutogrowPolicy {
		Logc(ctx).WithFields(LogFields{
			"storageClass":      scName,
			"oldAutogrowPolicy": oldAutogrowPolicy,
			"newAutogrowPolicy": newAutogrowPolicy,
		}).Info("StorageClass Autogrow policy changed; updating affected volumes.")

		policyErr = o.upsertStorageClassAutogrowPolicyInternal(ctx, scName)
	}

	return newSC.ConstructExternal(ctx), policyErr
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	sc, found := o.storageClasses[scName]
	if !found {
		return nil, errors.NotFoundError("storage class %v was not found", scName)
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
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

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
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	sc, found := o.storageClasses[scName]
	if !found {
		return errors.NotFoundError("storage class %s not found", scName)
	}

	// Get StorageClass Autogrow policy before deletion
	// This is needed to know which volumes might be affected
	sc.GetName()
	scAutogrowPolicy := sc.GetAutogrowPolicy()

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

	// Re-resolve effective Autogrow policy for affected volumes
	// This handles the case where SC had an Autogrow policy and volumes were inheriting from it
	// After SC deletion, volumes should fall back to next priority (TBC in future, or empty)
	if scAutogrowPolicy != "" {
		Logc(ctx).WithFields(LogFields{
			"storageClass":   scName,
			"autogrowPolicy": scAutogrowPolicy,
		}).Debug("Re-resolving Autogrow policy for volumes after StorageClass deletion.")

		// Find all volumes using this deleted SC
		for _, volume := range o.getAllVolumes() {
			// Only update volumes that:
			//  1. Were using this SC
			//  2. Have NO volume-level autogrow policy (were inheriting from SC)
			if volume.Config.StorageClass == scName && volume.Config.RequestedAutogrowPolicy == "" {
				oldEffectiveAGPolicy := volume.EffectiveAGPolicy

				// Re-resolve effective Autogrow policy (SC now gone, will fall to TBC or empty)
				// Current: Returns "" (no TBC support yet)
				// Future: Will check TBC and return TBC policy if configured
				newEffectiveAGPolicy, policyErr := o.resolveEffectiveAutogrowPolicy(ctx, volume.Config)

				// Update effective policy
				volume.EffectiveAGPolicy = newEffectiveAGPolicy

				// Update associations if Autogrow policy changed
				if oldEffectiveAGPolicy.PolicyName != newEffectiveAGPolicy.PolicyName {
					// Disassociate from old Autogrow policy
					if oldEffectiveAGPolicy.PolicyName != "" {
						o.disassociateVolumeFromAutogrowPolicyInternal(ctx, volume.Config.Name, oldEffectiveAGPolicy.PolicyName)
					}

					// Associate with new policy (if exists - may be from TBC in future)
					if policyErr == nil && newEffectiveAGPolicy.PolicyName != "" {
						o.associateVolumeWithAutogrowPolicyInternal(ctx, volume.Config.Name, newEffectiveAGPolicy.PolicyName)
					}
					Logc(ctx).WithFields(LogFields{
						"volume":                volume.Config.Name,
						"oldAutogrowPolicyName": oldEffectiveAGPolicy.PolicyName,
						"newAutogrowPolicyName": newEffectiveAGPolicy.PolicyName,
					}).Debug("Re-resolved effective Autogrow policy after StorageClass deletion.")
				}
			}
		}
		Logc(ctx).WithFields(LogFields{
			"storageClass": scName,
		}).Info("Re-resolved Autogrow policy for volumes after StorageClass deletion.")
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

	var nodes []*models.Node

	if b.CanEnablePublishEnforcement() {

		nodes = o.publishedNodesForBackend(b)
		volToNodePublications := o.volumePublicationsForBackend(b)

		nodeMap := make(map[string]*models.Node)
		for _, n := range nodes {
			nodeMap[n.Name] = n
		}

		// reconcile every volume if publish enforcement is enabled for the backend
		for volName, volPublications := range volToNodePublications {
			volume, ok := o.volumes[volName]
			if !ok {
				continue
			}

			volNodes := make([]*models.Node, 0)
			for _, pub := range volPublications {
				if node, found := nodeMap[pub.NodeName]; found {
					volNodes = append(volNodes, node)
				}
			}

			if err := b.ReconcileVolumeNodeAccess(ctx, volume.Config, volNodes); err != nil {
				Logc(ctx).WithError(err).WithFields(LogFields{
					"volume": volume.Config.Name,
					"nodes":  volNodes,
				}).Error("Unable to reconcile node access for volume.")
				return err
			}
		}

	} else {
		nodes = o.nodes.List()
	}

	if err := b.ReconcileNodeAccess(ctx, nodes, o.uuid); err != nil {
		return err
	}

	b.SetNodeAccessUpToDate()
	return nil
}

// publishedNodesForBackend returns the nodes that a backend has published volumes to
func (o *TridentOrchestrator) publishedNodesForBackend(b storage.Backend) []*models.Node {
	pubs := o.volumePublications.ListPublications()
	volumes := b.Volumes()
	m := make(map[string]struct{})

	for _, pub := range pubs {
		if _, ok := volumes.Load(pub.VolumeName); ok {
			m[pub.NodeName] = struct{}{}
		}
	}

	nodes := make([]*models.Node, 0, len(m))
	for n := range m {
		if node := o.nodes.Get(n); node != nil {
			nodes = append(nodes, node)
		}
	}

	return nodes
}

func (o *TridentOrchestrator) volumePublicationsForBackend(b storage.Backend) map[string][]*models.VolumePublication {
	volumes := b.Volumes()
	volumeToNodePublications := make(map[string][]*models.VolumePublication)

	volumes.Range(func(k, _ interface{}) bool {
		volName := k.(string)
		volumeToNodePublications[volName] = o.volumePublications.ListPublicationsForVolume(volName)
		return true
	})
	return volumeToNodePublications
}

func (o *TridentOrchestrator) reconcileBackendState(ctx context.Context, b storage.Backend) error {
	Logc(ctx).WithField("backend", b.Name()).Debug(">>>>>> reconcileBackendState")
	defer Logc(ctx).WithField("backend", b.Name()).Debug("<<<<<<< reconcileBackendState")
	if !b.CanGetState() {
		// This backend does not support polling backend for state.
		return nil
	}

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	reason, changeMap := b.GetBackendState(ctx)
	b.UpdateBackendState(ctx, reason)

	if changeMap != nil {
		if changeMap.Contains(storage.BackendStateReasonChange) {
			// Update CR.
			Logc(ctx).WithField("reason", reason).Debugf("Backend state reason change detected for %s.", b.Name())
			if err := o.storeClient.UpdateBackend(ctx, b); err != nil {
				return err
			}
		}

		// If reason is not-empty, which means backend is offline,
		// Doesn't make sense to call updateBackend() when backend is offline itself.
		// So, return early.
		// Additionally, calling updateBackend() might fail for various reasons, such as
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
			logMessage = "Change in physical pools and API version detected for the backend %s."
		case changeMap.Contains(storage.BackendStatePoolsChange):
			logMessage = "Change in physical pools detected for the backend %s."
		case changeMap.Contains(storage.BackendStateAPIVersionChange):
			logMessage = "Change in API version detected for the backend %s."
		}

		if logMessage != "" {
			Logc(ctx).Debugf(logMessage, b.Name())

			// Getting the marshaled driver's config.
			bytes, err := b.MarshalDriverConfig()
			if err != nil {
				return err
			}

			_, err = o.updateBackend(ctx, b.Name(), string(bytes), b.ConfigRef())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// safeReconcileNodeAccessOnBackend wraps reconcileNodeAccessOnBackend in a mutex lock for use in functions that aren't
// already locked
func (o *TridentOrchestrator) safeReconcileNodeAccessOnBackend(ctx context.Context, b storage.Backend) error {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
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

func (o *TridentOrchestrator) AddNode(
	ctx context.Context, node *models.Node, nodeEventCallback NodeEventCallback,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("node_add", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
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

	// Setting log level, log workflows and log layers on the node same as to what is set on the controller.
	logLevel := GetDefaultLogLevel()
	node.LogLevel = logLevel

	logWorkflows := GetSelectedWorkFlows()
	node.LogWorkflows = logWorkflows

	logLayers := GetSelectedLogLayers()
	node.LogLayers = logLayers

	if err = o.storeClient.AddOrUpdateNode(ctx, node); err != nil {
		return
	}

	o.nodes.Set(node.Name, node)

	o.lastNodeRegistration = time.Now()
	o.invalidateAllBackendNodeAccess()
	return nil
}

func (o *TridentOrchestrator) UpdateNode(
	ctx context.Context, nodeName string, flags *models.NodePublicationStateFlags,
) (err error) {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	if !o.volumePublicationsSynced {
		return errors.NotReadyError()
	}

	defer recordTiming("node_update", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	node := o.nodes.Get(nodeName)
	if node == nil {
		return errors.NotFoundError("node %s not found", nodeName)
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
	if !reflect.DeepEqual(node, oldNode) {
		Logc(ctx).WithFields(LogFields{
			"node":  nodeName,
			"state": node.PublicationState,
		}).Debug("Updating node in persistence layer.")
		o.nodes.Set(nodeName, node)
		err = o.storeClient.AddOrUpdateNode(ctx, node)
	}

	return err
}

func (o *TridentOrchestrator) invalidateAllBackendNodeAccess() {
	for _, backend := range o.backends {
		backend.InvalidateNodeAccess()
	}
}

func (o *TridentOrchestrator) GetNode(ctx context.Context, nodeName string) (node *models.NodeExternal, err error) {
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
		return nil, errors.NotFoundError("node %v was not found", nodeName)
	}
	return n.ConstructExternal(), nil
}

func (o *TridentOrchestrator) ListNodes(ctx context.Context) (nodes []*models.NodeExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("node_list", &err)()

	internalNodes := o.nodes.List()
	nodes = make([]*models.NodeExternal, 0, len(internalNodes))
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
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	node := o.nodes.Get(nodeName)
	if node == nil {
		return errors.NotFoundError("node %s not found", nodeName)
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

		o.nodes.Set(nodeName, node)
	}

	return nil
}

func (o *TridentOrchestrator) deleteNode(ctx context.Context, nodeName string) (err error) {
	node := o.nodes.Get(nodeName)
	if node == nil {
		return errors.NotFoundError("node %s not found", nodeName)
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
	ctx context.Context, attachedLegacyVolumes []*models.VolumePublicationExternal,
) (reconcileErr error) {
	Logc(ctx).Debug(">>>>>> ReconcileVolumePublications")
	defer Logc(ctx).Debug("<<<<<< ReconcileVolumePublications")
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	defer recordTiming("reconcile_legacy_vol_pubs", &reconcileErr)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
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

		publication := &models.VolumePublication{
			Name:         attachedLegacyVolume.Name,
			NodeName:     nodeName,
			VolumeName:   volumeName,
			ReadOnly:     attachedLegacyVolume.ReadOnly,
			AccessMode:   attachedLegacyVolume.AccessMode,
			StorageClass: attachedLegacyVolume.StorageClass,
			BackendUUID:  attachedLegacyVolume.BackendUUID,
			Pool:         attachedLegacyVolume.Pool,
		}

		// Compute AutogrowIneligible if volume exists in cache
		if vol, ok := o.volumes[volumeName]; ok {
			publication.AutogrowIneligible = isVolumeAutogrowIneligible(vol.Config)
		}

		// Initialize labels with node name label
		publication.Labels = map[string]string{
			config.TridentNodeNameLabel: nodeName,
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
	ctx context.Context, publication *models.VolumePublication,
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
) (publication *models.VolumePublication, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_get", &err)()

	volumePublication, found := o.volumePublications.TryGet(volumeName, nodeName)
	if !found {
		return nil, errors.NotFoundError("volume publication %v was not found",
			models.GenerateVolumePublishName(volumeName, nodeName))
	}
	return volumePublication, nil
}

// ListVolumePublications returns a list of all volume publications.
func (o *TridentOrchestrator) ListVolumePublications(
	ctx context.Context,
) (publications []*models.VolumePublicationExternal, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	Logc(ctx).Debug(">>>>>> ListVolumePublications")
	defer Logc(ctx).Debug("<<<<<< ListVolumePublications")

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("vol_pub_list", &err)()

	// Get all publications as a list.
	internalPubs := o.volumePublications.ListPublications()
	publications = make([]*models.VolumePublicationExternal, 0, len(internalPubs))
	for _, pub := range internalPubs {
		publications = append(publications, pub.ConstructExternal())
	}
	return publications, nil
}

// ListVolumePublicationsForVolume returns a list of all volume publications for a given volume.
func (o *TridentOrchestrator) ListVolumePublicationsForVolume(
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

	// Get all publications for a volume as a list.
	internalPubs := o.volumePublications.ListPublicationsForVolume(volumeName)
	publications = make([]*models.VolumePublicationExternal, 0, len(internalPubs))
	for _, pub := range internalPubs {
		publications = append(publications, pub.ConstructExternal())
	}
	return
}

// Change the signature of the func here to return error.
func (o *TridentOrchestrator) listVolumePublicationsForVolumeAndSubordinates(
	_ context.Context, volumeName string,
) (publications []*models.VolumePublication) {
	publications = []*models.VolumePublication{}

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
) (publications []*models.VolumePublicationExternal, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	if !o.volumePublicationsSynced {
		return nil, errors.NotReadyError()
	}
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	fields := LogFields{"nodeName": nodeName}
	Logc(ctx).WithFields(fields).Debug(">>>>>> ListVolumePublicationsForNode")
	defer Logc(ctx).Debug("<<<<<< ListVolumePublicationsForNode")

	defer recordTiming("vol_pub_list_for_node", &err)()

	// Retrieve only publications on the node.
	internalPubs := o.volumePublications.ListPublicationsForNode(nodeName)
	publications = make([]*models.VolumePublicationExternal, 0, len(internalPubs))
	for _, pub := range internalPubs {
		publications = append(publications, pub.ConstructExternal())
	}
	return
}

// SyncVolumePublications asynchronously persists a batch of VolumePublications to the store.
//
// The cache has already been updated synchronously by the caller (bootstrap or update operation).
// This function rate-limits and persists each VP's current cached state to the persistent store.
//
// Each VP is processed independently with rate limiting to avoid overwhelming the API server.
// If a VP is deleted from cache before persistence, it is safely skipped.
//
// On persistence failures, retries with exponential backoff without holding the mutex lock,
// allowing other operations to proceed during retry delays.
//
// Used by:
//   - Bootstrap: Syncing/migrating all VPs (labels, AGP, legacy fields)
//   - UpdateVolumeAutogrowPolicy: Propagating AGP changes to RWX/RO volume's VPs
func (o *TridentOrchestrator) SyncVolumePublications(
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
	rateLimiterBackoff.MaxInterval = 2 * time.Second
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
			// Acquire lock for this attempt
			o.mutex.Lock()
			defer o.mutex.Unlock()

			// Try to persist this VP
			err := o.syncVolumePublications(ctx, vpsToBeSynced[i])
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

// syncVolumePublications persists a single VP from cache to store.
//
// This function reads the latest VP state from cache (source of truth) and persists it to the store.
// If the VP was deleted from cache between queueing and persistence, the update is safely skipped.
// Returns an error if the store update fails.
func (o *TridentOrchestrator) syncVolumePublications(
	ctx context.Context, vp *models.VolumePublication,
) error {
	Logc(ctx).WithFields(LogFields{
		"volumeName": vp.VolumeName,
		"nodeName":   vp.NodeName,
	}).Debug(">>>>>> syncVolumePublications")
	defer Logc(ctx).Debug("<<<<<< syncVolumePublications")

	// Fetch the latest VP from cache - this is the source of truth
	// The cache was already updated synchronously (during bootstrap or update operation)
	// Our job here is just to persist the latest state to the store
	// and not update the store with the stale data
	latestVP := o.volumePublications.Get(vp.VolumeName, vp.NodeName)
	if latestVP == nil {
		// VP was deleted while we were waiting in rate limiter - skip update
		Logc(ctx).WithFields(LogFields{
			"volumeName": vp.VolumeName,
			"nodeName":   vp.NodeName,
		}).Debug("VP not found in cache, skipping update (may have been deleted)")
		return nil
	}

	// Persist the vp to the store
	if err := o.storeClient.UpdateVolumePublication(ctx, latestVP); err != nil {
		Logc(ctx).WithFields(LogFields{
			"publication": latestVP.Name,
			"volumeName":  latestVP.VolumeName,
			"nodeName":    latestVP.NodeName,
		}).WithError(err).Debug("Failed to sync VP fields to store")
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"volumeName": latestVP.VolumeName,
		"nodeName":   latestVP.NodeName,
		"labels":     latestVP.Labels,
	}).Debug("VP fields and labels synced")

	return nil
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
		if !errors.IsNotFoundError(err) {
			return err
		}
	}

	if err = o.volumePublications.Delete(volumeName, nodeName); err != nil {
		return fmt.Errorf("unable to remove publication from cache; %v", err)
	}

	node := o.nodes.Get(nodeName)
	if node == nil {
		return errors.NotFoundError("node %s not found", nodeName)
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

// EstablishMirror creates a net-new replication mirror relationship between 2 volumes on a backend
func (o *TridentOrchestrator) EstablishMirror(
	ctx context.Context, backendUUID, volumeName, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
	replicationSchedule string,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("mirror_establish", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
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
func (o *TridentOrchestrator) ReestablishMirror(
	ctx context.Context, backendUUID, volumeName, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
	replicationSchedule string,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("mirror_reestablish", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
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
func (o *TridentOrchestrator) PromoteMirror(
	ctx context.Context, backendUUID, volumeName, localInternalVolumeName, remoteVolumeHandle, snapshotHandle string,
) (waitingForSnapshot bool, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return false, o.bootstrapError
	}
	defer recordTiming("mirror_promote", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
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
func (o *TridentOrchestrator) GetMirrorStatus(
	ctx context.Context, backendUUID, localInternalVolumeName, remoteVolumeHandle string,
) (status string, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return "", o.bootstrapError
	}
	defer recordTiming("mirror_status", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
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
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	defer o.updateMetrics()

	backend, err := o.getBackendByBackendUUID(backendUUID)
	if err != nil {
		return false, err
	}
	return backend.CanMirror(), nil
}

// ReleaseMirror removes snapmirror relationship infromation and snapshots for a source volume in ONTAP
func (o *TridentOrchestrator) ReleaseMirror(
	ctx context.Context, backendUUID, volumeName, localInternalVolumeName string,
) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("mirror_release", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
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
func (o *TridentOrchestrator) GetReplicationDetails(
	ctx context.Context, backendUUID, localInternalVolumeName, remoteVolumeHandle string,
) (policy, schedule, SVMName string, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return "", "", "", o.bootstrapError
	}
	defer recordTiming("replication_details", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return "", "", "", ctx.Err()
	}
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

// UpdateMirror ensures the specified snapshot is copied to the destination, if specified.
// If snapshot name is empty, the current state of the source is sent instead.
func (o *TridentOrchestrator) UpdateMirror(ctx context.Context, volumeName, snapshotName string) (err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}
	defer recordTiming("update_mirror", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	// Get volume
	tridentVolume, err := o.getVolume(ctx, volumeName)
	if err != nil {
		return fmt.Errorf("could not find volume '%v' in Trident; %v", volumeName, err)
	}

	// Get backend to ensure it can mirror
	backend, err := o.getBackendByBackendUUID(tridentVolume.BackendUUID)
	if err != nil {
		return fmt.Errorf("backend %s not found", tridentVolume.BackendUUID)
	}
	if !backend.CanMirror() {
		return fmt.Errorf("backend does not support mirroring")
	}

	logFields := LogFields{
		"volume":   volumeName,
		"snapshot": snapshotName,
	}

	// Mirror update
	Logc(ctx).WithFields(logFields).Info("Mirror update in progress.")
	return backend.UpdateMirror(ctx, tridentVolume.Config.InternalName, snapshotName)
}

// CheckMirrorTransferState returns the last completed transfer time and an error if mirror relationship transfer
// is failed or in progress
func (o *TridentOrchestrator) CheckMirrorTransferState(
	ctx context.Context, volumeName string,
) (endTime *time.Time, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	defer recordTiming("check_mirror_transfer_state", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	defer o.updateMetrics()

	// Get volume
	tridentVolume, err := o.getVolume(ctx, volumeName)
	if err != nil {
		return nil, fmt.Errorf("could not find volume '%v' in Trident; %v", volumeName, err)
	}

	// Get backend to ensure it can mirror
	backend, err := o.getBackendByBackendUUID(tridentVolume.BackendUUID)
	if err != nil {
		return nil, fmt.Errorf("backend %s not found", tridentVolume.BackendUUID)
	}
	if !backend.CanMirror() {
		return nil, fmt.Errorf("backend does not support mirroring")
	}

	// Check transfer state of mirror relationship
	return backend.CheckMirrorTransferState(ctx, tridentVolume.Config.InternalName)
}

// GetMirrorTransferTime returns the last completed transfer time
func (o *TridentOrchestrator) GetMirrorTransferTime(
	ctx context.Context, volumeName string,
) (endTime *time.Time, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}
	defer recordTiming("check_mirror_transfer_state", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	defer o.updateMetrics()

	// Get volume
	tridentVolume, err := o.getVolume(ctx, volumeName)
	if err != nil {
		return nil, fmt.Errorf("could not find volume '%v' in Trident; %v", volumeName, err)
	}

	// Get backend to ensure it can mirror
	backend, err := o.getBackendByBackendUUID(tridentVolume.BackendUUID)
	if err != nil {
		return nil, fmt.Errorf("backend %s not found", tridentVolume.BackendUUID)
	}
	if !backend.CanMirror() {
		return nil, fmt.Errorf("backend does not support mirroring")
	}

	// Get last transfer time of mirror relationship
	return backend.GetMirrorTransferTime(ctx, tridentVolume.Config.InternalName)
}

func (o *TridentOrchestrator) GetCHAP(
	ctx context.Context, volumeName, nodeName string,
) (chapInfo *models.IscsiChapInfo, err error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	defer recordTiming("get_chap", &err)()
	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

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
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	return GetDefaultLogLevel(), nil
}

func (o *TridentOrchestrator) SetLogLevel(ctx context.Context, level string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}

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
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	return GetSelectedWorkFlows(), nil
}

func (o *TridentOrchestrator) ListLoggingWorkflows(ctx context.Context) ([]string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return []string{}, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return ListWorkflowTypes(), nil
}

func (o *TridentOrchestrator) SetLoggingWorkflows(ctx context.Context, flows string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}

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
	if ctx.Err() != nil {
		return "", ctx.Err()
	}

	return GetSelectedLogLayers(), nil
}

func (o *TridentOrchestrator) ListLogLayers(ctx context.Context) ([]string, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return []string{}, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return ListLogLayers(), nil
}

func (o *TridentOrchestrator) SetLogLayers(ctx context.Context, layers string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}

	err := SetLogLayers(layers)
	if err != nil {
		return err
	}

	return nil
}

// ============================================================================
// Autogrow Policy CRUD Operations
// ============================================================================

// AddAutogrowPolicy creates a new Autogrow policy in the orchestrator. The policy can be referenced
// by volumes directly or as a default in StorageClass.
func (o *TridentOrchestrator) AddAutogrowPolicy(ctx context.Context, config *storage.AutogrowPolicyConfig) (*storage.AutogrowPolicyExternal, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	var err error
	defer recordTiming("autogrowpolicy_add", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	defer o.updateMetrics()

	Logc(ctx).WithFields(LogFields{
		"autogrowPolicyName": config.Name,
		"usedThreshold":      config.UsedThreshold,
		"growthAmount":       config.GrowthAmount,
		"maxSize":            config.MaxSize,
	}).Debug("Adding Autogrow policy.")

	if _, exists := o.autogrowPolicies[config.Name]; exists {
		return nil, errors.AlreadyExistsError("Autogrow policy %s already exists", config.Name)
	}

	autogrowPolicy := storage.NewAutogrowPolicyFromConfig(config)

	o.autogrowPolicies[config.Name] = autogrowPolicy

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
	return autogrowPolicy.ConstructExternal(ctx), nil
}

// UpdateAutogrowPolicy updates an existing Autogrow policy with new configuration values.
// Volumes already associated with the policy will use the updated settings. When the policy
// state changes (Success <-> Failed/Deleting), volumes are automatically re-evaluated:
//   - Success → Failed/Deleting: Existing associations are invalidated
//   - Failed/Deleting → Success: All volumes are re-evaluated for potential association
//
// This ensures volume associations remain consistent with policy state transitions.
func (o *TridentOrchestrator) UpdateAutogrowPolicy(ctx context.Context, config *storage.AutogrowPolicyConfig) (*storage.AutogrowPolicyExternal, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	var err error
	defer recordTiming("autogrowpolicy_update", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	defer o.updateMetrics()

	Logc(ctx).WithFields(LogFields{
		"autogrowPolicyName": config.Name,
		"usedThreshold":      config.UsedThreshold,
		"growthAmount":       config.GrowthAmount,
		"maxSize":            config.MaxSize,
	}).Debug("Updating Autogrow policy.")

	policy, err := o.getAutogrowPolicy(config.Name)
	if err != nil {
		return nil, err
	}

	// Store old state to detect state transitions
	oldAGPState := policy.State()

	// Update the policy spec + status in cache to match with the external world (CR)
	policy.UpdateFromConfig(config)

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
			o.invalidateVolumesForPolicy(ctx, config.Name, policy.GetVolumes())
		}
	}

	Logc(ctx).WithField("autogrowPolicyName", config.Name).Info("Autogrow policy updated.")
	return policy.ConstructExternal(ctx), nil
}

// DeleteAutogrowPolicy removes an Autogrow policy from the orchestrator.
//
// Behavior:
//   - If policy has no volumes: Immediately deletes from memory (hard delete)
//   - If policy has volumes: Sets state to Deleting (soft delete), returns nil
//   - Soft-deleted policies are automatically cleaned up when the last volume is disassociated
//
// This matches the backend deletion pattern for consistency.
func (o *TridentOrchestrator) DeleteAutogrowPolicy(ctx context.Context, agPolicyName string) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	var err error
	defer recordTiming("autogrowpolicy_delete", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	defer o.updateMetrics()

	Logc(ctx).WithField("autogrowPolicyName", agPolicyName).Info("Deleting autogrow policy.")

	policy, err := o.getAutogrowPolicy(agPolicyName)
	if err != nil {
		return err
	}

	if policy.HasVolumes() {
		volumes := policy.GetVolumes()
		// // Set Deleting state (soft delete)
		policy.UpdateFromConfig(&storage.AutogrowPolicyConfig{
			Name:  policy.Name(),
			State: storage.AutogrowPolicyStateDeleting,
		})

		Logc(ctx).WithFields(LogFields{
			"autogrowPolicyName": agPolicyName,
			"volumeCount":        len(volumes),
			"volumes":            volumes,
		}).Info("Autogrow policy has associated volumes, setting to Deleting state.")

		// Return nil (success) - policy is now soft-deleted
		// The CR will remain until volumes are disassociated
		return nil

	}

	// No volumes, proceed with hard delete
	delete(o.autogrowPolicies, agPolicyName)

	Logc(ctx).WithField("autogrowPolicyName", agPolicyName).Info("Autogrow policy deleted.")
	return nil
}

// GetAutogrowPolicy retrieves a single Autogrow policy by name. The returned external representation
// includes the policy configuration and a list of all volumes currently using the policy.
func (o *TridentOrchestrator) GetAutogrowPolicy(ctx context.Context, agPolicyName string) (*storage.AutogrowPolicyExternal, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	var err error
	defer recordTiming("autogrowpolicy_get", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	defer o.updateMetrics()

	policy, err := o.getAutogrowPolicy(agPolicyName)
	if err != nil {
		return nil, err
	}

	return policy.ConstructExternal(ctx), nil
}

// ListAutogrowPolicies returns all Autogrow policies currently registered in the orchestrator. Each
// policy includes its configuration and the list of volumes using it.
func (o *TridentOrchestrator) ListAutogrowPolicies(ctx context.Context) ([]*storage.AutogrowPolicyExternal, error) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	var err error
	defer recordTiming("autogrowpolicy_list", &err)()

	o.mutex.Lock()
	defer o.mutex.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	defer o.updateMetrics()

	policies := make([]*storage.AutogrowPolicyExternal, 0)
	for _, policy := range o.autogrowPolicies {
		policies = append(policies, policy.ConstructExternal(ctx))
	}

	return policies, nil
}

// UpdateVolumeAutogrowPolicy updates a volume when its autogrow policy changes.
// This resolves the new effective policy, validates it exists, and updates associations.
// Returns NotFoundError or AutogrowPolicyNotUsableError for event recording.
func (o *TridentOrchestrator) UpdateVolumeAutogrowPolicy(
	ctx context.Context, volumeName, requestedAutogrowPolicy string,
) error {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCore)

	Logc(ctx).WithFields(LogFields{
		"volumeName":              volumeName,
		"requestedAutogrowPolicy": requestedAutogrowPolicy,
	}).Debug(">>>> UpdateVolumeAutogrowPolicy")
	defer Logc(ctx).WithFields(LogFields{
		"volumeName":              volumeName,
		"requestedAutogrowPolicy": requestedAutogrowPolicy,
	}).Debug("<<<< UpdateVolumeAutogrowPolicy")

	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	var err error
	defer recordTiming("volume_autogrow_policy_update", &err)()

	// IMPORTANT: No deferred unlock here (unlike most serial core functions).
	// This function releases the lock before spawning a goroutine to avoid blocking.
	// ALL return paths MUST explicitly call o.mutex.Unlock() before returning.
	o.mutex.Lock()
	if ctx.Err() != nil {
		o.mutex.Unlock()
		return ctx.Err()
	}
	defer o.updateMetrics()

	// Get the volume
	volume, ok := o.volumes[volumeName]
	if !ok {
		// Check if the volume is a subordinate volume
		volume, ok = o.subordinateVolumes[volumeName]
		if !ok {
			o.mutex.Unlock()
			return errors.NotFoundError("volume %s not found", volumeName)
		}
	}

	// Store old effective Autogrow policy for comparison
	oldEffectiveAGPolicy := volume.EffectiveAGPolicy
	// Store old requested autogrow policy for fallback
	oldRequestedAutogrowPolicy := volume.Config.RequestedAutogrowPolicy

	// Update the requested autogrow policy in volume config
	volume.Config.RequestedAutogrowPolicy = requestedAutogrowPolicy

	// Persist the updated volume to the store
	if err := o.updateVolumeOnPersistentStore(ctx, volume); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeName": volumeName,
		}).Error("Failed to persist volume Autogrow policy update to store.")
		volume.Config.RequestedAutogrowPolicy = oldRequestedAutogrowPolicy
		o.mutex.Unlock()
		return err
	}

	// Resolve new effective policy
	newEffectiveAGPolicy, policyErr := o.resolveEffectiveAutogrowPolicy(ctx, volume.Config)
	if policyErr != nil {
		if errors.IsAutogrowPolicyNotFoundError(policyErr) {
			Logc(ctx).WithField("volume", volume.Config.Name).
				Warn("Referenced Autogrow policy not found during volume autogrow policy update.")
		} else if errors.IsAutogrowPolicyNotUsableError(policyErr) {
			Logc(ctx).WithField("volume", volume.Config.Name).
				Warn("Referenced Autogrow policy not usable during volume autogrow policy update.")
		}
	}

	// Update effective Autogrow policy for the volume changed
	volume.EffectiveAGPolicy = newEffectiveAGPolicy

	// Update associations if policy changed
	if oldEffectiveAGPolicy.PolicyName != newEffectiveAGPolicy.PolicyName {
		// Step 1: ALWAYS dissociate from old policy
		// This handles ALL transition cases:
		//  - "gold" → "silver" (changing policy)
		//  - "gold" → "none" (disabling, "none" converts to "")
		//  - "gold" → "" (removing volume-level policy, falls to SC or "")
		//  - "gold" → "nonexistent" (policy doesn't exist yet)
		if oldEffectiveAGPolicy.PolicyName != "" {
			o.disassociateVolumeFromAutogrowPolicyInternal(ctx, volumeName, oldEffectiveAGPolicy.PolicyName)
		}

		// Step 2: ONLY associate with new policy if it's valid
		// Requires BOTH conditions:
		//  - policyErr == nil (policy exists in orchestrator)
		//  - newEffectiveAGPolicy != "" (not empty/disabled)
		// This correctly skips:
		//  - "none" (converted to "", policyErr == nil but empty)
		//  - "" (empty, either no policy or removed)
		//  - "nonexistent" (policyErr != nil)
		if policyErr == nil && newEffectiveAGPolicy.PolicyName != "" {
			o.associateVolumeWithAutogrowPolicyInternal(ctx, volumeName, newEffectiveAGPolicy.PolicyName)
		}

		Logc(ctx).WithFields(LogFields{
			"volumeName":            volumeName,
			"oldAutogrowPolicyName": oldEffectiveAGPolicy.PolicyName,
			"newAutogrowPolicyName": newEffectiveAGPolicy.PolicyName,
		}).Info("Updated volume's effective Autogrow policy.")
	}

	// Propagate AGP annotation changes to all the VPs associated with this volume
	vps := o.volumePublications.ListPublicationsForVolume(volumeName)
	syncNeededVPS := make([]*models.VolumePublication, 0, len(vps))

	// Detect and modify VPs that need syncing
	for i := range vps {
		syncNeeded := syncVolumePublicationFields(volume, vps[i])
		if syncNeeded {
			syncNeededVPS = append(syncNeededVPS, vps[i])
			// Update cache with modified VP
			if err := o.volumePublications.Set(vps[i].VolumeName, vps[i].NodeName, vps[i]); err != nil {
				Logc(ctx).WithFields(LogFields{
					"volumeName": vps[i].VolumeName,
					"nodeName":   vps[i].NodeName,
				}).WithError(err).Error("Failed to update VP cache")
			}
		}
	}

	// Sync changes asynchronously to avoid blocking the orchestrator during the entire batch operation.
	// The goroutine allows SyncVolumePublications to acquire/release the lock per attempt,
	// keeping the orchestrator responsive during retries and rate limiting.
	// NOTE: We release lock before spawning goroutine because SyncVolumePublications
	// will acquire o.mutex internally
	if len(syncNeededVPS) > 1 {
		o.mutex.Unlock()
		go o.SyncVolumePublications(ctx, syncNeededVPS)
		return policyErr
	}

	// For single VP, persist directly (already have lock)
	if len(syncNeededVPS) == 1 {
		err := o.syncVolumePublications(ctx, syncNeededVPS[0])
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"volumeName": volumeName,
				"nodeName":   syncNeededVPS[0].NodeName,
			}).WithError(err).Errorf("failed to update Autogrow policy change to volume publication store")
		}
	}

	o.mutex.Unlock()
	return policyErr
}

// getAutogrowPolicy retrieves a policy from the map by name.
// Returns the policy and nil error if found.
// Returns nil and NotFoundError if not found.
// Assumes mutex is already held by the caller.
func (o *TridentOrchestrator) getAutogrowPolicy(agPolicyName string) (*storage.AutogrowPolicy, error) {
	policy := o.autogrowPolicies[agPolicyName]
	if policy != nil {
		return policy, nil
	}
	return nil, errors.AutogrowPolicyNotFoundError(agPolicyName)
}

// upsertStorageClassAutogrowPolicyInternal handles Autogrow policy changes in StorageClass.
// This is an internal helper that assumes the orchestrator mutex is already held by the caller.
// It updates effective policies for all volumes that use this StorageClass and have no volume-level autogrow policy.
func (o *TridentOrchestrator) upsertStorageClassAutogrowPolicyInternal(
	ctx context.Context,
	scName string,
) error {
	// Pre-validate the policy ONCE instead of for each volume
	// This avoids redundant lookups when processing multiple volumes
	var newEffectiveAGPolicy models.EffectiveAutogrowPolicyInfo
	var policyError error

	// Create a temporary VolumeConfig to resolve the effective policy
	// (all volumes with this SC will resolve to the same policy)
	tempConfig := &storage.VolumeConfig{
		StorageClass:            scName,
		RequestedAutogrowPolicy: "", // No volume-level autogrow policy
	}
	newEffectiveAGPolicy, policyError = o.resolveEffectiveAutogrowPolicy(ctx, tempConfig)
	// Find all volumes using this storage class and update their effective policy
	for _, volume := range o.getAllVolumes() {
		// Only update volumes that use this SC and have no volume-level autogrow policy
		if volume.Config.StorageClass != scName || volume.Config.RequestedAutogrowPolicy != "" {
			continue // Early continue for non-matching volumes
		}

		oldEffectiveAGPolicy := volume.EffectiveAGPolicy
		// Skip if policy hasn't actually changed
		if oldEffectiveAGPolicy.PolicyName == newEffectiveAGPolicy.PolicyName {
			continue
		}

		volume.EffectiveAGPolicy = newEffectiveAGPolicy

		// Update associations if Autogrow policy changed
		if oldEffectiveAGPolicy.PolicyName != newEffectiveAGPolicy.PolicyName {
			// Step 1: ALWAYS dissociate from old
			if oldEffectiveAGPolicy.PolicyName != "" {
				o.disassociateVolumeFromAutogrowPolicyInternal(ctx, volume.Config.Name, oldEffectiveAGPolicy.PolicyName)
			}

			// Step 2: Associate ONLY if new policy exists and is Success
			if policyError == nil && newEffectiveAGPolicy.PolicyName != "" {
				o.associateVolumeWithAutogrowPolicyInternal(ctx, volume.Config.Name, newEffectiveAGPolicy.PolicyName)
			}

			Logc(ctx).WithFields(LogFields{
				"volumeName":            volume.Config.Name,
				"oldAutogrowPolicyName": oldEffectiveAGPolicy.PolicyName,
				"newAutogrowPolicyName": newEffectiveAGPolicy.PolicyName,
			}).Debug("Updated volume effective Autogrow policy due to StorageClass change.")
		}

	}

	Logc(ctx).WithFields(LogFields{
		"storageClass": scName,
	}).Info("Completed updating volumes for StorageClass Autogrow policy change.")

	return policyError
}

// associateVolumeWithAutogrowPolicyInternal creates an association between a volume and an Autogrow policy.
// This is an internal helper method that assumes the orchestrator mutex is already held by the caller.
// The policy must exist in the orchestrator; this method does not validate.
func (o *TridentOrchestrator) associateVolumeWithAutogrowPolicyInternal(ctx context.Context, volumeName, agPolicyName string) {
	if agPolicyName == "" {
		Logc(ctx).WithFields(LogFields{
			"volumeName": volumeName,
		}).Debug("No Autogrow policy specified for volume. Skipping association.")
		return
	}

	// Get the Autogrow policy and omit the error as Autogrow policy existence has been validated prior to this
	// call
	policy, err := o.getAutogrowPolicy(agPolicyName)
	if err != nil {
		// This should never happen as callers validate, but handle gracefully
		Logc(ctx).WithError(err).WithFields(LogFields{
			"volumeName":         volumeName,
			"autogrowPolicyName": agPolicyName,
		}).Warn("Autogrow policy not found during association; this is unexpected.")
		return
	}

	policy.AddVolume(volumeName)
	Logc(ctx).WithFields(LogFields{
		"volumeName":     volumeName,
		"autogrowPolicy": agPolicyName,
	}).Debug("Associated volume with Autogrow policy.")
}

// disassociateVolumeFromAutogrowPolicyInternal removes the association between a volume and an Autogrow policy.
// This is an internal helper method that assumes the orchestrator mutex is already held by the caller.
func (o *TridentOrchestrator) disassociateVolumeFromAutogrowPolicyInternal(ctx context.Context, volumeName,
	agPolicyName string) {

	if agPolicyName == "" {
		Logc(ctx).WithFields(LogFields{
			"volumeName": volumeName,
		}).Debug("No Autogrow policy specified for volume. Skipping disassociation.")
		return
	}

	policy, err := o.getAutogrowPolicy(agPolicyName)
	if err != nil {
		// Policy already deleted - nothing to do
		Logc(ctx).WithError(err).WithFields(LogFields{
			"volumeName":         volumeName,
			"autogrowPolicyName": agPolicyName,
		}).Debug("Autogrow policy not found during disassociation; skipping.")
		return
	}

	policy.RemoveVolume(volumeName)
	Logc(ctx).WithFields(LogFields{
		"volumeName":         volumeName,
		"autogrowPolicyName": agPolicyName,
	}).Debug("Disassociated volume from Autogrow policy.")
}

// resolveEffectiveAutogrowPolicy determines the effective Autogrow policy for a volume.
// It uses the stateless utils function and orchestrator's StorageClass data.
// Returns policy info (name + reason)  and error if the policy doesn't exist or is not usable.
// Assumes orchestrator mutex is already held by the caller.
func (o *TridentOrchestrator) resolveEffectiveAutogrowPolicy(
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

	// Get StorageClass Autogrow policy from orchestrator's cache
	scAutogrowPolicy := ""
	if volumeConfig.StorageClass != "" {
		sc, exists := o.storageClasses[volumeConfig.StorageClass]
		if exists {
			scAutogrowPolicy = sc.GetAutogrowPolicy()
		} else {
			Logc(ctx).WithField("storageClass", volumeConfig.StorageClass).Warn("StorageClass not found in orchestrator while resolving Autogrow policy.")
		}
	}

	// Get autogrow policy on volume
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

	// Validate policy exists in orchestrator
	autogrowPolicy, err := o.getAutogrowPolicy(effectiveAGPolicyName)
	if err != nil {
		// Policy doesn't exist - return empty string and AutogrowPolicyNotFoundError
		return models.EffectiveAutogrowPolicyInfo{
			PolicyName: "",
			Reason:     models.AutogrowPolicyReasonNotFound,
		}, err
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

// invalidateVolumesForPolicy disassociates volumes from a policy and re-resolves their effective policies.
// Used when a policy transitions to Failed or Deleting state.
func (o *TridentOrchestrator) invalidateVolumesForPolicy(ctx context.Context, agPolicyName string, associatedVolumes []string) {
	for _, associatedVolumeName := range associatedVolumes {
		associatedVolume, ok := o.volumes[associatedVolumeName]
		if !ok {
			// Check if the volume is a subordinate volume
			associatedVolume, ok = o.subordinateVolumes[associatedVolumeName]
			if !ok {
				continue
			}
		}

		// Disassociate from this policy
		o.disassociateVolumeFromAutogrowPolicyInternal(ctx, associatedVolumeName, agPolicyName)

		// Invalidate directly (no re-resolution - we know there's no fallback)
		associatedVolume.EffectiveAGPolicy = models.EffectiveAutogrowPolicyInfo{
			PolicyName: "",
			Reason:     models.AutogrowPolicyReasonUnusable,
		}

	}

	Logc(ctx).WithFields(LogFields{
		"policyName":   agPolicyName,
		"volumesTotal": len(associatedVolumes),
	}).Info("Completed invalidating volumes for policy.")
}

// reevaluateVolumesForPolicy checks all volumes to see if they should now use the given policy.
// Used when:
//   - A new policy is added (Success or Failed state)
//   - A policy transitions to Success state from Failed or Deleting
//   - A policy transitions to Failed/Deleting from Success (to update reasons)
//
// For Success policies: volumes are associated
// For Failed/Deleting policies: volume reasons are updated to "Unusable" without association
func (o *TridentOrchestrator) reevaluateVolumesForPolicy(ctx context.Context, agPolicyName string) {
	for _, volume := range o.getAllVolumes() {
		// Skip volumes that explicitly reference a different policy
		// If volume has autogrow policy for a different policy, it can't use this one
		if volume.Config.RequestedAutogrowPolicy != "" {
			// Skip if autogrow policy is "none" (any case variation)
			if strings.EqualFold(volume.Config.RequestedAutogrowPolicy, autogrow.AutogrowPolicyNone) {
				continue
			}
			// Skip if autogrow policy explicitly references a different policy
			if !strings.EqualFold(volume.Config.RequestedAutogrowPolicy, agPolicyName) {
				continue
			}
		}

		// Skip if already using this policy
		if volume.EffectiveAGPolicy.PolicyName == agPolicyName {
			continue
		}
		// Re-resolve effective policy
		effectiveAGPolicy, policyErr := o.resolveEffectiveAutogrowPolicy(ctx, volume.Config)

		// Check if this volume should now use the newly-Success policy
		if policyErr == nil && effectiveAGPolicy.PolicyName == agPolicyName &&
			volume.EffectiveAGPolicy.PolicyName != agPolicyName {
			// Disassociate from old policy if exists
			if volume.EffectiveAGPolicy.PolicyName != "" {
				o.disassociateVolumeFromAutogrowPolicyInternal(ctx, volume.Config.Name,
					volume.EffectiveAGPolicy.PolicyName)
			}

			// Associate with the newly-Success policy
			o.associateVolumeWithAutogrowPolicyInternal(ctx, volume.Config.Name, effectiveAGPolicy.PolicyName)

			Logc(ctx).WithFields(LogFields{
				"volume":                volume.Config.Name,
				"effectiveAGPolicyName": effectiveAGPolicy.PolicyName,
			}).Debug("Associated volume with newly-Success policy.")
		}

		// Update effective policy regardless of association outcome
		volume.EffectiveAGPolicy = effectiveAGPolicy
	}

	Logc(ctx).WithFields(LogFields{
		"policyName": agPolicyName,
	}).Info("Completed re-evaluating volumes for policy.")
}

// getAllVolumes returns all volumes (regular + subordinate) as a slice.
// Assumes the orchestrator mutex is already held by the caller.
func (o *TridentOrchestrator) getAllVolumes() []*storage.Volume {
	allVolumes := make([]*storage.Volume, 0, len(o.volumes)+len(o.subordinateVolumes))
	for _, vol := range o.volumes {
		allVolumes = append(allVolumes, vol)
	}
	for _, vol := range o.subordinateVolumes {
		allVolumes = append(allVolumes, vol)
	}
	return allVolumes
}

// resolveAndSetEffectiveAutogrowPolicy resolves the effective Autogrow policy for a volume,
// logs any errors appropriately, and sets it on the volume.
// Returns the policy info and any error for the caller to handle association logic.
// Assumes the orchestrator mutex is already held by the caller.
func (o *TridentOrchestrator) resolveAndSetEffectiveAutogrowPolicy(
	ctx context.Context, vol *storage.Volume, operationContext string, isBootstrap bool,
) (models.EffectiveAutogrowPolicyInfo, error) {
	// Resolve effective Autogrow policy
	effectiveAGPolicy, policyErr := o.resolveEffectiveAutogrowPolicy(ctx, vol.Config)

	// Log any resolution errors
	if policyErr != nil {
		logFields := LogFields{"volume": vol.Config.Name}
		if errors.IsAutogrowPolicyNotFoundError(policyErr) {
			msg := fmt.Sprintf("Referenced Autogrow policy not found during %s.", operationContext)
			if isBootstrap {
				Logc(ctx).WithFields(logFields).Debug(msg)
			} else {
				Logc(ctx).WithFields(logFields).Warn(msg)
			}
		} else if errors.IsAutogrowPolicyNotUsableError(policyErr) {
			msg := fmt.Sprintf("Referenced Autogrow policy not usable during %s.", operationContext)
			if isBootstrap {
				Logc(ctx).WithFields(logFields).Debug(msg)
			} else {
				Logc(ctx).WithFields(logFields).Warn(msg)
			}
		}
	}

	// Always set effective Autogrow policy even if it does not exist yet
	vol.EffectiveAGPolicy = effectiveAGPolicy

	return effectiveAGPolicy, policyErr
}
