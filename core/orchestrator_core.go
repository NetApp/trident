// Copyright 2018 NetApp, Inc. All Rights Reserved.

package core

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/factory"
	"github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

type TridentOrchestrator struct {
	backends       map[string]*storage.Backend
	volumes        map[string]*storage.Volume
	frontends      map[string]frontend.Plugin
	mutex          *sync.Mutex
	storageClasses map[string]*storageclass.StorageClass
	storeClient    persistentstore.Client
	bootstrapped   bool
	bootstrapError error
}

// NewTridentOrchestrator returns a storage orchestrator instance
func NewTridentOrchestrator(client persistentstore.Client) *TridentOrchestrator {
	return &TridentOrchestrator{
		backends:       make(map[string]*storage.Backend),
		volumes:        make(map[string]*storage.Volume),
		frontends:      make(map[string]frontend.Plugin),
		storageClasses: make(map[string]*storageclass.StorageClass),
		mutex:          &sync.Mutex{},
		storeClient:    client,
		bootstrapped:   false,
		bootstrapError: notReadyError(),
	}
}

func (o *TridentOrchestrator) transformPersistentState() error {
	// Transforming persistent state happens under two scenarios:
	// 1) Change in the persistent store version (e.g., from etcdv2 to etcdv3)
	// 2) Change in the Trident API version (e.g., from Trident API v1 to v2)
	version, err := o.storeClient.GetVersion()
	if err != nil && persistentstore.MatchKeyNotFoundErr(err) {
		// Persistent store and Trident API versions should be etcdv2 and v1 respectively.
		version = &persistentstore.PersistentStateVersion{
			PersistentStoreVersion: string(persistentstore.EtcdV2Store),
			OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		}
	} else if err != nil {
		return fmt.Errorf("couldn't determine the orchestrator persistent state version: %v",
			err)
	}
	if config.OrchestratorAPIVersion != version.OrchestratorAPIVersion {
		log.WithFields(log.Fields{
			"current_api_version": version.OrchestratorAPIVersion,
			"desired_api_version": config.OrchestratorAPIVersion,
		}).Info("Transforming Trident API objects on the persistent store.")
		//TODO: transform Trident API objects
	}
	dataMigrator := persistentstore.NewDataMigrator(o.storeClient,
		persistentstore.StoreType(version.PersistentStoreVersion))
	if err = dataMigrator.Run("/"+config.OrchestratorName, false); err != nil {
		return fmt.Errorf("data migration failed: %v", err)
	}

	// Store the persistent store and API versions
	version.OrchestratorAPIVersion = config.OrchestratorAPIVersion
	version.PersistentStoreVersion = string(o.storeClient.GetType())
	if err = o.storeClient.SetVersion(version); err != nil {
		return fmt.Errorf("failed to set the persistent state version after migration: %v",
			err)
	}
	return nil
}

func (o *TridentOrchestrator) Bootstrap() error {
	var err error

	// Set up telemetry for use by the storage backends
	config.OrchestratorTelemetry = config.Telemetry{
		TridentVersion: config.OrchestratorVersion.String(),
	}
	config.OrchestratorTelemetry.Platform = string(config.CurrentDriverContext)
	if f, ok := o.frontends[string(config.CurrentDriverContext)]; ok {
		config.OrchestratorTelemetry.PlatformVersion = f.Version()
	} else {
		log.Warning("Trident is bootstrapping with no frontend.")
	}

	// Transform persistent state, if necessary
	if err = o.transformPersistentState(); err != nil {
		o.bootstrapError = bootstrapError(err)
		return o.bootstrapError
	}

	// Bootstrap state from persistent store
	if err = o.bootstrap(); err != nil {
		o.bootstrapError = bootstrapError(err)
		return o.bootstrapError
	}

	o.bootstrapped = true
	o.bootstrapError = nil
	log.Infof("%s bootstrapped successfully.", strings.Title(config.OrchestratorName))
	return nil
}

func (o *TridentOrchestrator) bootstrapBackends() error {
	persistentBackends, err := o.storeClient.GetBackends()
	if err != nil {
		return err
	}

	for _, b := range persistentBackends {
		// TODO:  If the API evolves, check the Version field here.
		serializedConfig, err := b.MarshalConfig()
		if err != nil {
			return err
		}

		newBackendExternal, backendErr := o.addBackend(serializedConfig)
		if backendErr != nil {

			errorLogFields := log.Fields{
				"handler":            "Bootstrap",
				"newBackendExternal": newBackendExternal,
				"backendErr":         backendErr.Error(),
			}

			// Trident for Docker supports one backend at a time, and the Docker volume plugin
			// should not start if the backend fails to initialize, so return any error here.
			if config.CurrentDriverContext == config.ContextDocker {
				log.WithFields(errorLogFields).Error("Problem adding backend.")
				return backendErr
			}

			log.WithFields(errorLogFields).Warn("Problem adding backend.")

			if newBackendExternal != nil {
				newBackend, _ := factory.NewStorageBackendForConfig(serializedConfig)
				newBackend.Name = b.Name
				newBackendExternal.Name = b.Name // have to set it explicitly, so it's not ""
				o.backends[newBackendExternal.Name] = newBackend

				log.WithFields(log.Fields{
					"newBackend":               newBackend,
					"newBackendExternal":       newBackendExternal,
					"newBackendExternal.Name":  newBackendExternal.Name,
					"newBackendExternal.State": newBackendExternal.State.String(),
				}).Debug("Backend information.")
			}
		}

		// Note that addBackend returns an external copy of the newly
		// added backend, so we have to go fetch it manually.
		newBackend := o.backends[newBackendExternal.Name]
		newBackend.Online = b.Online
		if backendErr != nil {
			newBackend.State = storage.Failed
		} else {
			newBackend.State = b.State
		}

		log.WithFields(log.Fields{
			"backend": newBackend.Name,
			"online":  newBackend.Online,
			"state":   newBackend.State,
			"handler": "Bootstrap",
		}).Info("Added an existing backend.")
	}
	return nil
}

func (o *TridentOrchestrator) bootstrapStorageClasses() error {
	persistentStorageClasses, err := o.storeClient.GetStorageClasses()
	if err != nil {
		return err
	}
	for _, psc := range persistentStorageClasses {
		// TODO:  If the API evolves, check the Version field here.
		sc := storageclass.NewFromPersistent(psc)
		log.WithFields(log.Fields{
			"storageClass": sc.GetName(),
			"handler":      "Bootstrap",
		}).Info("Added an existing storage class.")
		o.storageClasses[sc.GetName()] = sc
		for _, b := range o.backends {
			sc.CheckAndAddBackend(b)
		}
	}
	return nil
}

func (o *TridentOrchestrator) bootstrapVolumes() error {
	volumes, err := o.storeClient.GetVolumes()
	if err != nil {
		return err
	}
	for _, v := range volumes {
		// TODO:  If the API evolves, check the Version field here.
		var backend *storage.Backend
		var ok bool
		backend, ok = o.backends[v.Backend]
		if !ok {
			return fmt.Errorf("couldn't find backend %s for volume %s",
				v.Backend, v.Config.Name)
		}
		vol := storage.NewVolume(v.Config, backend.Name, v.Pool, v.Orphaned)
		backend.Volumes[vol.Config.Name], o.volumes[vol.Config.Name] = vol, vol

		log.WithFields(log.Fields{
			"volume":       vol.Config.Name,
			"internalName": vol.Config.InternalName,
			"size":         vol.Config.Size,
			"backend":      vol.Backend,
			"pool":         vol.Pool,
			"orphaned":     vol.Orphaned,
			"handler":      "Bootstrap",
		}).Info("Added an existing volume.")
	}
	return nil
}

func (o *TridentOrchestrator) bootstrapVolTxns() error {
	volTxns, err := o.storeClient.GetVolumeTransactions()
	if err != nil {
		log.Warnf("Couldn't retrieve volume transaction logs: %s", err.Error())
	}
	for _, v := range volTxns {
		o.mutex.Lock()
		err = o.handleFailedTransaction(v)
		o.mutex.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *TridentOrchestrator) bootstrap() error {
	// Fetching backend information

	type bootstrapFunc func() error
	for _, f := range []bootstrapFunc{o.bootstrapBackends,
		o.bootstrapStorageClasses, o.bootstrapVolumes, o.bootstrapVolTxns} {
		err := f()
		if err != nil {
			if persistentstore.MatchKeyNotFoundErr(err) {
				keyError := err.(*persistentstore.Error)
				log.Warnf("Unable to find key %s.  Continuing bootstrap, but "+
					"consider checking integrity if Trident installation is "+
					"not new.", keyError.Key)
			} else {
				return err
			}
		}
	}

	// Clean up any offline backends that lack volumes.  This can happen if
	// a connection to etcd fails when attempting to delete a backend.
	for backendName, backend := range o.backends {
		if backend.State.IsFailed() {
			continue
		}

		if backend.State.IsDeleting() && !backend.HasVolumes() {
			backend.Terminate()
			delete(o.backends, backendName)
			err := o.storeClient.DeleteBackend(backend)
			if err != nil {
				return fmt.Errorf("failed to delete empty offline backend %s:"+
					"%v", backendName, err)
			}
		}
	}

	return nil
}

func (o *TridentOrchestrator) handleFailedTransaction(v *persistentstore.VolumeTransaction) error {
	log.WithFields(log.Fields{
		"volume":       v.Config.Name,
		"size":         v.Config.Size,
		"storageClass": v.Config.StorageClass,
		"op":           v.Op,
	}).Info("Processed volume transaction log.")
	switch v.Op {
	case persistentstore.AddVolume:
		// Regardless of whether the transaction succeeded or not, we need
		// to roll it back.  There are three possible states:
		// 1) Volume transaction created only
		// 2) Volume created on backend
		// 3) Volume created in etcd.
		if _, ok := o.volumes[v.Config.Name]; ok {
			// If the volume was added to etcd, we will have loaded the
			// volume into memory, and we can just delete it normally.
			// Handles case 3)
			err := o.deleteVolume(v.Config.Name)
			if err != nil {
				return fmt.Errorf("unable to clean up volume %s: %v", v.Config.Name, err)
			}
		} else {
			// If the volume wasn't added into etcd, we attempt to delete
			// it at each backend, since we don't know where it might have
			// landed.  We're guaranteed that the volume name will be
			// unique across backends, thanks to the StoragePrefix field,
			// so this should be idempotent.
			// Handles case 2)
			for _, backend := range o.backends {
				if !backend.State.IsOnline() {
					// Backend offlining is serialized with volume creation,
					// so we can safely skip offline backends.
					continue
				}
				// Volume deletion is an idempotent operation, so it's safe to
				// delete an already deleted volume.
				if err := backend.Driver.Destroy(
					backend.Driver.GetInternalVolumeName(v.Config.Name),
				); err != nil {
					return fmt.Errorf("error attempting to clean up volume %s from backend %s: %v", v.Config.Name,
						backend.Name, err)
				}
			}
		}
		// Finally, we need to clean up the volume transaction.
		// Necessary for all cases.
		if err := o.deleteVolumeTransaction(v); err != nil {
			return fmt.Errorf("failed to clean up volume addition transaction: %v", err)
		}
	case persistentstore.DeleteVolume:
		// Because we remove the volume from persistent store after we remove
		// it from the backend, we need to take any special measures only when
		// the volume is still in the persistent store. In this case, the
		// volume should have been loaded into memory when we bootstrapped.
		if _, ok := o.volumes[v.Config.Name]; ok {
			err := o.deleteVolume(v.Config.Name)
			if err != nil {
				log.WithFields(log.Fields{
					"volume": v.Config.Name,
					"error":  err,
				}).Errorf("Unable to finalize deletion of the volume! Repeat deleting the volume using %s.",
					config.OrchestratorClientName)
			}
		} else {
			log.WithFields(log.Fields{
				"volume": v.Config.Name,
			}).Info("Volume for the delete transaction wasn't found.")
		}
		if err := o.deleteVolumeTransaction(v); err != nil {
			return fmt.Errorf("failed to clean up volume deletion transaction: %v", err)
		}
	case persistentstore.ResizeVolume:
		// There are a few possible states:
		// 1) We failed to resize the volume on the backend.
		// 2) We successfully resized the volume on the backend.
		//    2a) We couldn't update the volume size in persistent store.
		//    2b) Persistent store was updated, but we couldn't delete the
		//        transaction object.
		var err error
		vol, ok := o.volumes[v.Config.Name]
		if ok {
			err = o.resizeVolume(vol, v.Config.Size)
			if err != nil {
				log.WithFields(log.Fields{
					"volume": v.Config.Name,
					"error":  err,
				}).Error("Unable to resize the volume! Repeat resizing the volume.")
			} else {
				log.WithFields(log.Fields{
					"volume":      vol.Config.Name,
					"volume_size": v.Config.Size,
				}).Info("Orchestrator resized the volume on the storage backend.")
			}
		} else {
			log.WithFields(log.Fields{
				"volume": v.Config.Name,
			}).Error("Volume for the resize transaction wasn't found.")
		}
		return o.resizeVolumeCleanup(err, vol, v)
	}

	return nil
}

func (o *TridentOrchestrator) AddFrontend(f frontend.Plugin) {
	name := f.GetName()
	if _, ok := o.frontends[name]; ok {
		log.WithFields(log.Fields{
			"name": name,
		}).Warn("Adding frontend already present.")
		return
	}
	log.WithFields(log.Fields{
		"name": name,
	}).Info("Added frontend.")
	o.frontends[name] = f
}

func (o *TridentOrchestrator) validateBackendUpdate(
	oldBackend *storage.Backend, newBackend *storage.Backend,
) error {
	// Validate that backend type isn't being changed as backend type has
	// implications for the internal volume names.
	if oldBackend.GetDriverName() != newBackend.GetDriverName() {
		return fmt.Errorf("cannot update the backend as the old backend is of type %s and the new backend is of type"+
			" %s", oldBackend.GetDriverName(), newBackend.GetDriverName())
	}
	return nil
}

func (o *TridentOrchestrator) GetVersion() (string, error) {
	return config.OrchestratorVersion.String(), o.bootstrapError
}

// AddBackend handles creation of a new storage backend
func (o *TridentOrchestrator) AddBackend(configJSON string) (*storage.BackendExternal, error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return o.addBackend(configJSON)
}

// addBackend creates a new storage backend. It assumes the mutex lock is
// already held or not required (e.g., during bootstrapping).
func (o *TridentOrchestrator) addBackend(configJSON string) (backendExternal *storage.BackendExternal, err error) {
	var (
		newBackend = true
		backend    *storage.Backend
	)

	defer func() {
		if backend != nil {
			if err != nil || !newBackend {
				backend.Terminate()
			}
		}
	}()

	backend, err = factory.NewStorageBackendForConfig(configJSON)
	if err != nil {
		log.WithFields(log.Fields{
			"err":     err.Error(),
			"backend": backend,
		}).Debug("NewStorageBackendForConfig had an error.")

		if backend != nil && backend.State.IsFailed() {
			o.backends[backend.Name] = backend
			return backend.ConstructExternal(), err
		}
		return nil, err
	}
	_, found := o.backends[backend.Name]
	if found {
		// Let the updateBackend method handle an existing backend
		newBackend = false
		return o.updateBackend(backend.Name, configJSON)
	}

	log.WithFields(log.Fields{
		"backend": backend.Name,
	}).Debug("Adding a new backend.")
	if err = o.updateBackendOnPersistentStore(backend, true); err != nil {
		return nil, err
	}
	o.backends[backend.Name] = backend

	// Update storage class information
	classes := make([]string, 0, len(o.storageClasses))
	for _, sc := range o.storageClasses {
		if added := sc.CheckAndAddBackend(backend); added > 0 {
			classes = append(classes, sc.GetName())
		}
	}
	if len(classes) == 0 {
		log.WithFields(log.Fields{
			"backend": backend.Name,
		}).Info("Newly added backend satisfies no storage classes.")
	} else {
		log.WithFields(log.Fields{
			"backend": backend.Name,
		}).Infof("Newly added backend satisfies storage classes %s.", strings.Join(classes, ", "))
	}

	return backend.ConstructExternal(), nil
}

// UpdateBackend updates an existing backend.
func (o *TridentOrchestrator) UpdateBackend(backendName, configJSON string) (
	backendExternal *storage.BackendExternal, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return o.updateBackend(backendName, configJSON)
}

// updateBackend updates an existing backend. It assumes the mutex lock is
// already held.
func (o *TridentOrchestrator) updateBackend(backendName, configJSON string) (
	backendExternal *storage.BackendExternal, err error) {
	var (
		backend *storage.Backend
	)

	defer func() {
		if backend != nil && err != nil {
			backend.Terminate()
		}
	}()

	// First, check whether the backend exists.
	originalBackend, found := o.backends[backendName]
	if !found {
		return nil, notFoundError(fmt.Sprintf("backend %v was not found", backendName))
	}

	// Second, validate the update.
	backend, err = factory.NewStorageBackendForConfig(configJSON)
	if err != nil {
		return nil, err
	}
	if err = o.validateBackendUpdate(originalBackend, backend); err != nil {
		return nil, err
	}
	log.WithFields(log.Fields{
		"backend": backend.Name,
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
	updateCode := backend.GetUpdateType(originalBackend)
	switch {
	case updateCode.Contains(storage.InvalidUpdate):
		return nil, fmt.Errorf("invalid backend update")
	case updateCode.Contains(storage.VolumeAccessInfoChange):
		return nil,
			fmt.Errorf("updating the data plane IP address isn't currently supported")
	case updateCode.Contains(storage.BackendRename):
		err = o.replaceBackendAndUpdateVolumesOnPersistentStore(originalBackend,
			backend)
		if err != nil {
			return nil, err
		}
	default:
		// Update backend information
		if err = o.updateBackendOnPersistentStore(backend, false); err != nil {
			return nil, err
		}
	}

	// Update the backend state in memory
	delete(o.backends, originalBackend.Name)
	originalBackend.Terminate()
	o.backends[backend.Name] = backend

	// Update the volume state in memory
	// Identify orphaned volumes (i.e., volumes that are not present on the
	// new backend). Such a scenario can happen if a subset of volumes are
	// replicated for DR or volumes get deleted out of band. Operations on
	// such volumes are likely to fail, so here we just warn the users about
	// such volumes and mark them as orphaned. This is a best effort activity,
	// so it doesn't have to be part of the persistent store transaction.
	for volName, vol := range o.volumes {
		if vol.Backend == originalBackend.Name {
			vol.Backend = backend.Name
			updatePersistentStore := false
			volumeExists := backend.Driver.Get(vol.Config.InternalName) == nil
			if !volumeExists {
				if vol.Orphaned == false {
					vol.Orphaned = true
					updatePersistentStore = true
					log.WithFields(log.Fields{
						"volume":  volName,
						"backend": backend.Name,
					}).Warn("Backend update resulted in an orphaned volume!")
				}
			} else {
				if vol.Orphaned == true {
					vol.Orphaned = false
					updatePersistentStore = true
					log.WithFields(log.Fields{
						"volume":  volName,
						"backend": backend.Name,
					}).Info("The volume is no longer orphaned as a result of the " +
						"backend update.")
				}
			}
			if updatePersistentStore {
				o.updateVolumeOnPersistentStore(vol)
			}
			o.backends[backend.Name].Volumes[volName] = vol
		}
	}

	// Update storage class information
	classes := make([]string, 0, len(o.storageClasses))
	for _, sc := range o.storageClasses {
		sc.RemovePoolsForBackend(originalBackend)
		if added := sc.CheckAndAddBackend(backend); added > 0 {
			classes = append(classes, sc.GetName())
		}
	}
	if len(classes) == 0 {
		log.WithFields(log.Fields{
			"backend": backend.Name,
		}).Info("Updated backend satisfies no storage classes.")
	} else {
		log.WithFields(log.Fields{
			"backend": backend.Name,
		}).Infof("Updated backend satisfies storage classes %s.",
			strings.Join(classes, ", "))
	}

	return backend.ConstructExternal(), nil
}

// UpdateBackend updates an existing backend.
func (o *TridentOrchestrator) UpdateBackendState(backendName, backendState string) (
	backendExternal *storage.BackendExternal, err error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	return o.updateBackendState(backendName, backendState)
}

// updateBackend updates an existing backend. It assumes the mutex lock is already held.
func (o *TridentOrchestrator) updateBackendState(backendName, backendState string) (
	backendExternal *storage.BackendExternal, err error) {
	var (
		backend *storage.Backend
	)

	log.WithFields(log.Fields{
		"backendName":  backendName,
		"backendState": backendState,
	}).Debug("UpdateBackendState")

	// First, check whether the backend exists.
	backend, found := o.backends[backendName]
	if !found {
		return nil, notFoundError(fmt.Sprintf("backend %v was not found", backendName))
	}

	newBackendState := storage.BackendState(backendState)

	// Limit the command to Failed
	if !newBackendState.IsFailed() {
		return nil, fmt.Errorf("unsupported backend state: %s", newBackendState)
	}

	if !newBackendState.IsOnline() {
		backend.Terminate()
	}
	backend.State = newBackendState

	return backend.ConstructExternal(), o.storeClient.UpdateBackend(backend)
}

func (o *TridentOrchestrator) GetBackend(backendName string) (*storage.BackendExternal, error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	backend, found := o.backends[backendName]
	if !found {
		return nil, notFoundError(fmt.Sprintf("backend %v was not found", backendName))
	}

	backendExternal := backend.ConstructExternal()
	log.WithFields(log.Fields{
		"backend":               backend,
		"backendExternal":       backendExternal,
		"backendExternal.Name":  backendExternal.Name,
		"backendExternal.State": backendExternal.State.String(),
	}).Info("GetBackend information.")
	return backendExternal, nil
}

func (o *TridentOrchestrator) ListBackends() ([]*storage.BackendExternal, error) {
	if o.bootstrapError != nil {
		log.WithFields(log.Fields{
			"bootstrapError": o.bootstrapError,
		}).Warn("ListBackends error")
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	log.Debugf("About to list backends: %v", o.backends)
	backends := make([]*storage.BackendExternal, 0)
	for _, b := range o.backends {
		backends = append(backends, b.ConstructExternal())
	}
	return backends, nil
}

func (o *TridentOrchestrator) DeleteBackend(backendName string) error {
	if o.bootstrapError != nil {
		log.WithFields(log.Fields{
			"bootstrapError": o.bootstrapError,
		}).Warn("DeleteBackend error")
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	backend, found := o.backends[backendName]
	if !found {
		return notFoundError(fmt.Sprintf("backend %s not found", backendName))
	}

	backend.Online = false // TODO eventually remove
	backend.State = storage.Deleting
	storageClasses := make(map[string]*storageclass.StorageClass, 0)
	for _, storagePool := range backend.Storage {
		for _, scName := range storagePool.StorageClasses {
			storageClasses[scName] = o.storageClasses[scName]
		}
		storagePool.StorageClasses = []string{}
	}
	for _, sc := range storageClasses {
		sc.RemovePoolsForBackend(backend)
	}
	if !backend.HasVolumes() {
		backend.Terminate()
		delete(o.backends, backendName)
		return o.storeClient.DeleteBackend(backend)
	}
	log.WithFields(log.Fields{
		"backend":        backend,
		"backend.Name":   backend.Name,
		"backend.State":  backend.State.String(),
		"backend.Online": backend.Online,
	}).Info("OfflineBackend information.")

	return o.storeClient.UpdateBackend(backend)
}

func (o *TridentOrchestrator) AddVolume(volumeConfig *storage.VolumeConfig) (
	externalVol *storage.VolumeExternal, err error) {

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	var (
		backend *storage.Backend
		vol     *storage.Volume
	)
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if _, ok := o.volumes[volumeConfig.Name]; ok {
		return nil, fmt.Errorf("volume %s already exists", volumeConfig.Name)
	}
	volumeConfig.Version = config.OrchestratorAPIVersion

	// Get the protocol based on the specified access mode & protocol
	protocol, err := o.getProtocol(volumeConfig.AccessMode, volumeConfig.Protocol)
	if err != nil {
		return nil, err
	}

	sc, ok := o.storageClasses[volumeConfig.StorageClass]
	if !ok {
		return nil, fmt.Errorf("unknown storage class: %s", volumeConfig.StorageClass)
	}
	pools := sc.GetStoragePoolsForProtocol(protocol)
	if len(pools) == 0 {
		return nil, fmt.Errorf("no available backends for storage class %s",
			volumeConfig.StorageClass)
	}

	// Add a transaction in case the operation must be rolled back later
	volTxn := &persistentstore.VolumeTransaction{
		Config: volumeConfig,
		Op:     persistentstore.AddVolume,
	}
	if err = o.addVolumeTransaction(volTxn); err != nil {
		return nil, err
	}

	// Recovery function in case of error
	defer func() {
		err = o.addVolumeCleanup(err, backend, vol, volTxn, volumeConfig)
	}()

	// Randomize the storage pool list for better distribution of load across all pools.
	rand.Seed(time.Now().UnixNano())

	log.WithFields(log.Fields{
		"volume": volumeConfig.Name,
	}).Debugf("Looking through %d storage pools.", len(pools))

	errorMessages := make([]string, 0)

	// Choose a pool at random.
	for _, num := range rand.Perm(len(pools)) {

		// Add volume to the backend of the selected pool
		backend = pools[num].Backend
		vol, err = backend.AddVolume(volumeConfig, pools[num], sc.GetAttributes())
		if err != nil {

			log.WithFields(log.Fields{
				"backend": backend.Name,
				"pool":    pools[num].Name,
				"volume":  volumeConfig.Name,
				"error":   err,
			}).Warn("Failed to create the volume on this backend!")
			errorMessages = append(errorMessages,
				fmt.Sprintf("[Failed to create volume %s on storage pool %s from backend %s: %s]",
					volumeConfig.Name, pools[num].Name, backend.Name, err.Error()))

		} else {

			if vol.Config.Protocol == config.ProtocolAny {
				vol.Config.Protocol = backend.GetProtocol()
			}

			// Add new volume to persistent store
			err = o.storeClient.AddVolume(vol)
			if err != nil {
				return nil, err
			}

			// Update internal cache and return external form of the new volume
			o.volumes[volumeConfig.Name] = vol
			externalVol = vol.ConstructExternal()
			return externalVol, nil
		}
	}

	externalVol = nil
	if len(errorMessages) == 0 {
		err = fmt.Errorf("no suitable %s backend with \"%s\" storage class and %s of free space was found",
			protocol, volumeConfig.StorageClass, volumeConfig.Size)
	} else {
		err = fmt.Errorf("encountered error(s) in creating the volume: %s",
			strings.Join(errorMessages, ", "))
	}
	return nil, err
}

func (o *TridentOrchestrator) CloneVolume(volumeConfig *storage.VolumeConfig) (
	externalVol *storage.VolumeExternal, err error) {

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	var (
		found   bool
		backend *storage.Backend
		vol     *storage.Volume
	)
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if _, ok := o.volumes[volumeConfig.Name]; ok {
		return nil, fmt.Errorf("volume %s already exists", volumeConfig.Name)
	}
	volumeConfig.Version = config.OrchestratorAPIVersion

	// Get the source volume
	sourceVolume, found := o.volumes[volumeConfig.CloneSourceVolume]
	if !found {
		return nil, notFoundError(fmt.Sprintf("source volume not found: %s", volumeConfig.CloneSourceVolume))
	}
	if sourceVolume.Orphaned {
		log.WithFields(log.Fields{
			"source_volume": sourceVolume.Config.Name,
			"volume":        volumeConfig.Name,
			"backend":       sourceVolume.Backend,
		}).Warnf("Clone operation is likely to fail with an orphaned " +
			"source volume!")
	}

	// Clone the source config, as most of its attributes will apply to the clone
	cloneConfig := sourceVolume.Config.ConstructClone()

	// Copy a few attributes from the request that will affect clone creation
	cloneConfig.Name = volumeConfig.Name
	cloneConfig.InternalName = ""
	cloneConfig.SplitOnClone = volumeConfig.SplitOnClone
	cloneConfig.CloneSourceVolume = volumeConfig.CloneSourceVolume
	cloneConfig.CloneSourceVolumeInternal = sourceVolume.Config.InternalName
	cloneConfig.CloneSourceSnapshot = volumeConfig.CloneSourceSnapshot
	cloneConfig.QoS = volumeConfig.QoS
	cloneConfig.QoSType = volumeConfig.QoSType

	// Add transaction in case the operation must be rolled back later
	volTxn := &persistentstore.VolumeTransaction{
		Config: cloneConfig,
		Op:     persistentstore.AddVolume,
	}
	if err = o.addVolumeTransaction(volTxn); err != nil {
		return nil, err
	}

	// Recovery function in case of error
	defer func() {
		err = o.addVolumeCleanup(err, backend, vol, volTxn, volumeConfig)
	}()

	backend, found = o.backends[sourceVolume.Backend]
	if !found {
		// Should never get here but just to be safe
		return nil, notFoundError(fmt.Sprintf("backend %s for the source volume not found: %s",
			sourceVolume.Backend, volumeConfig.CloneSourceVolume))
	}

	vol, err = backend.CloneVolume(cloneConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloned volume %s on backend %s: %v", cloneConfig.Name,
			backend.Name, err)
	}

	// Save references to new volume
	err = o.storeClient.AddVolume(vol)
	if err != nil {
		return nil, err
	}
	o.volumes[cloneConfig.Name] = vol

	return vol.ConstructExternal(), nil
}

// addVolumeTransaction is called from the volume create, clone, and resize
// methods to save a record of the operation in case it fails and must be
// cleaned up later.
func (o *TridentOrchestrator) addVolumeTransaction(volTxn *persistentstore.VolumeTransaction) error {

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
	oldTxn, err := o.storeClient.GetExistingVolumeTransaction(volTxn)
	if err != nil {
		log.Warningf("Unable to check for a preexisting volume transaction: %v", err)
		return err
	}
	if oldTxn != nil {
		err = o.handleFailedTransaction(oldTxn)
		if err != nil {
			return fmt.Errorf("unable to process the preexisting transaction "+
				"for volume %s:  %v", volTxn.Config.Name, err)
		}
		if oldTxn.Op == persistentstore.DeleteVolume {
			return fmt.Errorf("rejecting the %v transaction after successful completion of a preexisting %v transaction",
				volTxn.Op, oldTxn.Op)
		}
	}

	return o.storeClient.AddVolumeTransaction(volTxn)
}

// deleteVolumeTransaction deletes a volume transaction created by
// addVolumeTransaction.
func (o *TridentOrchestrator) deleteVolumeTransaction(volTxn *persistentstore.VolumeTransaction) error {
	return o.storeClient.DeleteVolumeTransaction(volTxn)
}

// addVolumeCleanup is used as a deferred method from the volume create/clone methods
// to clean up in case anything goes wrong during the operation.
func (o *TridentOrchestrator) addVolumeCleanup(
	err error, backend *storage.Backend, vol *storage.Volume,
	volTxn *persistentstore.VolumeTransaction, volumeConfig *storage.VolumeConfig) error {

	var (
		cleanupErr, txErr error
	)
	if err != nil {
		// We failed somewhere.  There are two possible cases:
		// 1.  We failed to allocate on a backend and fell through to the
		//     end of the function.  In this case, we don't need to roll
		//     anything back.
		// 2.  We failed to add the volume to etcd.  In this case, we need
		//     to remove the volume from the backend.
		if backend != nil && vol != nil {
			// We succeeded in adding the volume to the backend; now
			// delete it.
			cleanupErr = backend.RemoveVolume(vol)
			if cleanupErr != nil {
				cleanupErr = fmt.Errorf("unable to delete volume "+
					"from backend during cleanup:  %v", cleanupErr)
			}
		}
	}
	if cleanupErr == nil {
		// Only clean up the volume transaction if we've succeeded at
		// cleaning up on the backend or if we didn't need to do so in the
		// first place.
		txErr = o.deleteVolumeTransaction(volTxn)
		if txErr != nil {
			txErr = fmt.Errorf("unable to clean up transaction:  %v", txErr)
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
		log.Warnf("Unable to clean up artifacts of volume creation: %v. "+
			"Repeat creating the volume or restart %v.",
			err, config.OrchestratorName)
	}
	return err
}

func (o *TridentOrchestrator) GetVolume(volume string) (*storage.VolumeExternal, error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	vol, found := o.volumes[volume]
	if !found {
		return nil, notFoundError(fmt.Sprintf("volume %v was not found", volume))
	}
	return vol.ConstructExternal(), nil
}

func (o *TridentOrchestrator) GetDriverTypeForVolume(vol *storage.VolumeExternal) (string, error) {
	if o.bootstrapError != nil {
		return config.UnknownDriver, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	if b, ok := o.backends[vol.Backend]; ok {
		return b.Driver.Name(), nil
	}
	return config.UnknownDriver, nil
}

func (o *TridentOrchestrator) GetVolumeType(vol *storage.VolumeExternal) (
	volumeType config.VolumeType, err error,
) {
	if o.bootstrapError != nil {
		return config.UnknownVolumeType, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	// Since the caller has a valid VolumeExternal and we're disallowing
	// backend deletion, we can assume that this will not hit a nil pointer.
	driver := o.backends[vol.Backend].GetDriverName()
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

func (o *TridentOrchestrator) ListVolumes() ([]*storage.VolumeExternal, error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	volumes := make([]*storage.VolumeExternal, 0, len(o.volumes))
	for _, v := range o.volumes {
		volumes = append(volumes, v.ConstructExternal())
	}
	return volumes, nil
}

// deleteVolume does the necessary work to delete a volume entirely.  It does
// not construct a transaction, nor does it take locks; it assumes that the
// caller will take care of both of these.  It also assumes that the volume
// exists in memory.
func (o *TridentOrchestrator) deleteVolume(volumeName string) error {
	volume := o.volumes[volumeName]
	volumeBackend := o.backends[volume.Backend]

	// Note that this call will only return an error if the backend actually
	// fails to delete the volume.  If the volume does not exist on the backend,
	// the driver will not return an error.  Thus, we're fine.
	if err := volumeBackend.RemoveVolume(volume); err != nil {
		log.WithFields(log.Fields{
			"volume":  volumeName,
			"backend": volume.Backend,
			"error":   err,
		}).Error("Unable to delete volume from backend.")
		return err
	}
	// Ignore failures to find the volume being deleted, as this may be called
	// during recovery of a volume that has already been deleted from etcd.
	// During normal operation, checks on whether the volume is present in the
	// volume map should suffice to prevent deletion of non-existent volumes.
	if err := o.storeClient.DeleteVolumeIgnoreNotFound(volume); err != nil {
		log.WithFields(log.Fields{
			"volume": volumeName,
		}).Error("Unable to delete volume from persistent store.")
		return err
	}
	if volumeBackend.State.IsDeleting() && !volumeBackend.HasVolumes() {
		if err := o.storeClient.DeleteBackend(volumeBackend); err != nil {
			log.WithFields(log.Fields{
				"backend": volume.Backend,
				"volume":  volumeName,
			}).Error("Unable to delete offline backend from the backing store" +
				" after its last volume was deleted.  Delete the volume again" +
				" to remove the backend.")
			return err
		}
		volumeBackend.Terminate()
		delete(o.backends, volume.Backend)
	}
	delete(o.volumes, volumeName)
	return nil
}

// DeleteVolume does the necessary set up to delete a volume during the course
// of normal operation, verifying that the volume is present in Trident and
// creating a transaction to ensure that the delete eventually completes.
func (o *TridentOrchestrator) DeleteVolume(volumeName string) (err error) {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return notFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}
	if volume.Orphaned {
		log.WithFields(log.Fields{
			"volume":  volumeName,
			"backend": volume.Backend,
		}).Warnf("Delete operation is likely to fail with an orphaned volume!")
	}

	volTxn := &persistentstore.VolumeTransaction{
		Config: volume.Config,
		Op:     persistentstore.DeleteVolume,
	}
	if err := o.addVolumeTransaction(volTxn); err != nil {
		return err
	}

	defer func() {
		errTxn := o.deleteVolumeTransaction(volTxn)
		if errTxn != nil {
			log.WithFields(log.Fields{
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

	// Delete the volume
	return o.deleteVolume(volumeName)
}

func (o *TridentOrchestrator) ListVolumesByPlugin(pluginName string) ([]*storage.VolumeExternal, error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	volumes := make([]*storage.VolumeExternal, 0)
	for _, backend := range o.backends {
		if backendName := backend.GetDriverName(); pluginName != backendName {
			continue
		}
		for _, vol := range backend.Volumes {
			volumes = append(volumes, vol.ConstructExternal())
		}
	}
	return volumes, nil
}

func (o *TridentOrchestrator) PublishVolume(
	volumeName string, publishInfo *utils.VolumePublishInfo,
) error {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return notFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}

	return o.backends[volume.Backend].Driver.Publish(volume.Config.InternalName, publishInfo)
}

// AttachVolume mounts a volume to the local host.  This method is currently only used by Docker,
// and it should be able to accomplish its task using only the data passed in; it should not need to
// use the storage controller API.  It may be assumed that this method always runs on the host to
// which the volume will be attached.
func (o *TridentOrchestrator) AttachVolume(
	volumeName, mountpoint string, publishInfo *utils.VolumePublishInfo,
) error {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	if _, ok := o.volumes[volumeName]; !ok {
		return notFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}

	log.WithFields(log.Fields{"volume": volumeName, "mountpoint": mountpoint}).Debug("Mounting volume.")

	// Ensure mount point exists and is a directory
	fileInfo, err := os.Lstat(mountpoint)
	if os.IsNotExist(err) {
		// Make mount point if it doesn't exist
		if err := os.MkdirAll(mountpoint, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !fileInfo.IsDir() {
		return fmt.Errorf("%v already exists and it's not a directory", mountpoint)
	}

	// Check if volume is already mounted
	dfOutput, dfOuputErr := utils.GetDFOutput()
	if dfOuputErr != nil {
		err = fmt.Errorf("error checking if %v is already mounted: %v", mountpoint, dfOuputErr)
		return err
	}
	for _, e := range dfOutput {
		if e.Target == mountpoint {
			log.Debugf("%v is already mounted", mountpoint)
			return nil
		}
	}

	if publishInfo.FilesystemType == "nfs" {
		return utils.AttachNFSVolume(volumeName, mountpoint, publishInfo)
	} else {
		return utils.AttachISCSIVolume(volumeName, mountpoint, publishInfo)
	}
}

// DetachVolume unmounts a volume from the local host.  This method is currently only used by Docker,
// and it should be able to accomplish its task using only the data passed in; it should not need to
// use the storage controller API.  It may be assumed that this method always runs on the host to
// which the volume will be attached.  It ensures the volume is already mounted, and it attempts to
// delete the mount point.
func (o *TridentOrchestrator) DetachVolume(volumeName, mountpoint string) error {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	if _, ok := o.volumes[volumeName]; !ok {
		return notFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}

	log.WithFields(log.Fields{"volume": volumeName, "mountpoint": mountpoint}).Debug("Unmounting volume.")

	// Check if the mount point exists, so we know that it's attached and must be cleaned up
	_, err := os.Stat(mountpoint)
	if err != nil {
		// Not attached, so nothing to do
		return nil
	}

	// Unmount the volume
	if err := utils.Umount(mountpoint); err != nil {
		return err
	}

	// Best effort removal of the mount point
	os.Remove(mountpoint)
	return nil
}

// CreateVolumeSnapshot creates a snapshot of the given volume
func (o *TridentOrchestrator) CreateVolumeSnapshot(
	snapshotName string, volumeConfig *storage.VolumeConfig,
) (*storage.SnapshotExternal, error) {

	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	// Get the source volume
	sourceVolume, ok := o.volumes[volumeConfig.Name]
	if !ok {
		return nil, notFoundError(fmt.Sprintf("source volume %s not found", volumeConfig.Name))
	}
	volumeConfig.Version = config.OrchestratorAPIVersion

	// Get the corresponding backend
	backend, found := o.backends[sourceVolume.Backend]
	if !found {
		// Should never get here but just to be safe
		return nil, notFoundError(fmt.Sprintf("backend %s for the source volume not found: %s",
			sourceVolume.Backend, volumeConfig.Name))
	}

	// Create the snapshot
	snapshot, err := backend.CreateVolumeSnapshot(snapshotName, volumeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot for volume %s on backend %s: %v", volumeConfig.Name,
			backend.Name, err)
	}

	// Save references to new snapshot in the persistent store
	err = o.storeClient.AddSnapshot(snapshot)
	if err != nil {
		return nil, err
	}

	return snapshot.ConstructExternal(), nil
}

func (o *TridentOrchestrator) ListVolumeSnapshots(volumeName string) ([]*storage.SnapshotExternal, error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	volume, ok := o.volumes[volumeName]
	if !ok {
		return nil, notFoundError(fmt.Sprintf("volume %s not found", volumeName))
	}

	snapshots, err := o.backends[volume.Backend].Driver.SnapshotList(volume.Config.InternalName)
	if err != nil {
		return nil, err
	}

	externalSnapshots := make([]*storage.SnapshotExternal, 0)
	for _, snapshot := range snapshots {
		externalSnapshots = append(externalSnapshots, snapshot.ConstructExternal())
	}
	return externalSnapshots, nil
}

func (o *TridentOrchestrator) ReloadVolumes() error {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	// Lock out all other workflows while we reload the volumes
	o.mutex.Lock()
	defer o.mutex.Unlock()

	// Make a temporary copy of backends in case anything goes wrong
	tempBackends := make(map[string]*storage.Backend)
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
		backend.Volumes = make(map[string]*storage.Volume)
	}

	// Re-run the volume bootstrapping code
	o.volumes = make(map[string]*storage.Volume)
	err := o.bootstrapVolumes()

	// If anything went wrong, reinstate the original volumes
	if err != nil {
		log.Errorf("Volume reload failed, restoring original volume list: %v", err)
		o.backends = tempBackends
		o.volumes = tempVolumes
	}

	return err
}

// ResizeVolume resizes a volume to the new size.
func (o *TridentOrchestrator) ResizeVolume(volumeName, newSize string) (err error) {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	vol, found := o.volumes[volumeName]
	if !found {
		return fmt.Errorf("volume %s wasn't found", volumeName)
	}
	if vol.Orphaned {
		log.WithFields(log.Fields{
			"volume":  volumeName,
			"backend": vol.Backend,
		}).Warnf("Resize operation is likely to fail with an orphaned volume!")
	}

	// Create a new config to capture the volume size change.
	cloneConfig := vol.Config.ConstructClone()
	cloneConfig.Size = newSize

	// Add a transaction in case the operation must be retried during bootstraping.
	volTxn := &persistentstore.VolumeTransaction{
		Config: cloneConfig,
		Op:     persistentstore.ResizeVolume,
	}
	if err = o.addVolumeTransaction(volTxn); err != nil {
		return
	}

	defer func() {
		if err == nil {
			log.WithFields(log.Fields{
				"volume":      volumeName,
				"volume_size": newSize,
			}).Info("Orchestrator resized the volume on the storage backend.")
		}
		err = o.resizeVolumeCleanup(err, vol, volTxn)
	}()

	// Resize the volume.
	return o.resizeVolume(vol, newSize)
}

// resizeVolume does the necessary work to resize a volume. It doesn't
// construct a transaction, nor does it take locks; it assumes that the
// caller will take care of both of these. It also assumes that the volume
// exists in memory.
func (o *TridentOrchestrator) resizeVolume(volume *storage.Volume, newSize string) error {
	volumeBackend, found := o.backends[volume.Backend]
	if !found {
		log.WithFields(log.Fields{
			"volume":  volume.Config.Name,
			"backend": volume.Backend,
		}).Error("Unable to find backend during volume resize.")
		return fmt.Errorf("unable to find backend %v during volume resize",
			volume.Backend)
	}

	if volume.Config.Size != newSize {
		if err := volumeBackend.ResizeVolume(volume.Config.InternalName,
			newSize); err != nil {
			log.WithFields(log.Fields{
				"volume":          volume.Config.Name,
				"volume_internal": volume.Config.InternalName,
				"backend":         volume.Backend,
				"current_size":    volume.Config.Size,
				"new_size":        newSize,
				"error":           err,
			}).Error("Unable to resize the volume.")
			return fmt.Errorf("unable to resize the volume: %v", err)
		}
	}

	volume.Config.Size = newSize
	if err := o.updateVolumeOnPersistentStore(volume); err != nil {
		// It's ok not to revert volume size as we don't clean up the
		// transaction object in this situation.
		log.WithFields(log.Fields{
			"volume": volume.Config.Name,
		}).Error("Unable to update the volume's size in persistent store.")
		return err
	}
	return nil
}

// resizeVolumeCleanup is used to clean up artifacts of volume resize in case
// anything goes wrong during the operation.
func (o *TridentOrchestrator) resizeVolumeCleanup(
	err error, vol *storage.Volume, volTxn *persistentstore.VolumeTransaction) error {

	if vol == nil || err == nil ||
		(err != nil && vol.Config.Size != volTxn.Config.Size) {
		// We either succeeded or failed to resize the volume on the
		// backend. There are two possible failure cases:
		// 1.  We failed to resize the volume. In this case, we just need to
		//     remove the volume transaction.
		// 2.  We failed to update the volume on persistent store. In this
		//     case, we leave the volume transaction around so that we can
		//     update the persistent store later.
		txErr := o.deleteVolumeTransaction(volTxn)
		if txErr != nil {
			txErr = fmt.Errorf("unable to clean up transaction:  %v", txErr)
		}
		errList := make([]string, 0, 2)
		for _, e := range []error{err, txErr} {
			if e != nil {
				errList = append(errList, e.Error())
			}
		}
		if len(errList) > 0 {
			err = fmt.Errorf(strings.Join(errList, ", "))
			log.Warnf("Unable to clean up artifacts of volume resize: %v. "+
				"Repeat resizing the volume or restart %v.",
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

// getProtocol returns the appropriate protocol based on a specified volume access mode and protocol, or
// an error if the two settings are incompatible.
//
// Generally, the access mode maps to a protocol as follows:
//
//  ReadWriteOnce -> Any (File + Block)
//  ReadOnlyMany  -> Any (File + Block)
//  ReadWriteMany -> File
//
// But if the protocol is explicitly set to File or Block, then it may override ProtocolAny or generate a conflict.
// The truth table below yields two special cases (RWX/Block) and (RWX/Any); all other rows simply echo the protocol.
//
//   AccessMode     Protocol     Result
//      RWO          File        File
//      RWO          Block       Block
//      RWO          Any         Any
//      ROX          File        File
//      ROX          Block       Block
//      ROX          Any         Any
//      RWX          File        File
//      RWX          Block       *ERROR*
//      RWX          Any         *File*
//      Any          File        File
//      Any          Block       Block
//      Any          Any         Any
//
func (o *TridentOrchestrator) getProtocol(
	accessMode config.AccessMode, protocol config.Protocol,
) (config.Protocol, error) {

	if accessMode == config.ReadWriteMany {
		if protocol == config.Block {
			return config.ProtocolAny, fmt.Errorf("incompatible access mode (%s) and protocol (%s)",
				accessMode, protocol)
		} else if protocol == config.ProtocolAny {
			return config.File, nil
		}
	}

	return protocol, nil
}

func (o *TridentOrchestrator) AddStorageClass(scConfig *storageclass.Config) (*storageclass.External, error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()
	sc := storageclass.New(scConfig)
	if _, ok := o.storageClasses[sc.GetName()]; ok {
		return nil, fmt.Errorf("storage class %s already exists", sc.GetName())
	}
	err := o.storeClient.AddStorageClass(sc)
	if err != nil {
		return nil, err
	}
	o.storageClasses[sc.GetName()] = sc
	added := 0
	for _, backend := range o.backends {
		added += sc.CheckAndAddBackend(backend)
	}
	if added == 0 {
		log.WithFields(log.Fields{
			"storageClass": scConfig.Name,
		}).Info("No backends currently satisfy provided storage class.")
	} else {
		log.WithFields(log.Fields{
			"storageClass": sc.GetName(),
		}).Infof("Storage class satisfied by %d storage pools.", added)
	}
	return sc.ConstructExternal(), nil
}

func (o *TridentOrchestrator) GetStorageClass(scName string) (*storageclass.External, error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	sc, found := o.storageClasses[scName]
	if !found {
		return nil, notFoundError(fmt.Sprintf("storage class %v was not found", scName))
	}
	// Storage classes aren't threadsafe (we modify them during runtime),
	// so return a copy, rather than the original
	return sc.ConstructExternal(), nil
}

func (o *TridentOrchestrator) ListStorageClasses() ([]*storageclass.External, error) {
	if o.bootstrapError != nil {
		return nil, o.bootstrapError
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	storageClasses := make([]*storageclass.External, 0, len(o.storageClasses))
	for _, sc := range o.storageClasses {
		storageClasses = append(storageClasses, sc.ConstructExternal())
	}
	return storageClasses, nil
}

func (o *TridentOrchestrator) DeleteStorageClass(scName string) error {
	if o.bootstrapError != nil {
		return o.bootstrapError
	}

	sc, found := o.storageClasses[scName]
	if !found {
		return notFoundError(fmt.Sprintf("storage class %s not found", scName))
	}

	// Note that we don't need a tranasaction here.  If this crashes prior
	// to successful deletion, the storage class will be reloaded upon reboot
	// automatically, which is consistent with the method never having returned
	// successfully.
	if err := o.storeClient.DeleteStorageClass(sc); err != nil {
		return err
	}
	delete(o.storageClasses, scName)
	for _, storagePool := range sc.GetStoragePoolsForProtocol(config.ProtocolAny) {
		storagePool.RemoveStorageClass(scName)
	}
	return nil
}

func (o *TridentOrchestrator) updateBackendOnPersistentStore(
	backend *storage.Backend, newBackend bool,
) error {
	// Update the persistent store with the backend information
	if o.bootstrapped || config.UsingPassthroughStore {
		var err error
		if newBackend {
			err = o.storeClient.AddBackend(backend)
		} else {
			log.WithFields(log.Fields{
				"backend": backend.Name,
			}).Info("Updating an existing backend.")
			err = o.storeClient.UpdateBackend(backend)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *TridentOrchestrator) updateVolumeOnPersistentStore(
	vol *storage.Volume) error {
	// Update the volume information in persistent store
	log.WithFields(log.Fields{
		"volume":          vol.Config.Name,
		"volume_orphaned": vol.Orphaned,
		"volume_size":     vol.Config.Size,
	}).Debug("Updating an existing volume.")
	return o.storeClient.UpdateVolume(vol)
}

func (o *TridentOrchestrator) replaceBackendAndUpdateVolumesOnPersistentStore(
	origBackend, newBackend *storage.Backend) error {
	// Update both backend and volume information in persistent store
	log.WithFields(log.Fields{
		"newBackendName": newBackend.Name,
		"oldBackendName": origBackend.Name,
	}).Debug("Renaming a backend and updating volumes.")
	return o.storeClient.ReplaceBackendAndUpdateVolumes(origBackend, newBackend)
}

func notReadyError() error {
	return &NotReadyError{
		fmt.Sprintf("%s is initializing, please try again later",
			strings.Title(config.OrchestratorName)),
	}
}

func IsNotReadyError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*NotReadyError)
	return ok
}

func bootstrapError(err error) error {
	return &BootstrapError{
		fmt.Sprintf("%s initialization failed; %s",
			strings.Title(config.OrchestratorName), err.Error()),
	}
}

func IsBootstrapError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*BootstrapError)
	return ok
}

func notFoundError(message string) error {
	return &NotFoundError{message}
}

func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*NotFoundError)
	return ok
}
