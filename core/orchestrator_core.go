// Copyright 2016 NetApp, Inc. All Rights Reserved.

package core

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	dvp "github.com/netapp/netappdvp/storage_drivers"
	dvp_utils "github.com/netapp/netappdvp/utils"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/factory"
	"github.com/netapp/trident/storage_class"
)

type tridentOrchestrator struct {
	backends       map[string]*storage.StorageBackend
	volumes        map[string]*storage.Volume
	frontends      map[string]frontend.FrontendPlugin
	mutex          *sync.Mutex
	storageClasses map[string]*storage_class.StorageClass
	storeClient    persistent_store.Client
	bootstrapped   bool
}

// returns a storage orchestrator instance
func NewTridentOrchestrator(client persistent_store.Client) *tridentOrchestrator {
	orchestrator := tridentOrchestrator{
		backends:       make(map[string]*storage.StorageBackend),
		volumes:        make(map[string]*storage.Volume),
		frontends:      make(map[string]frontend.FrontendPlugin),
		storageClasses: make(map[string]*storage_class.StorageClass),
		mutex:          &sync.Mutex{},
		storeClient:    client,
		bootstrapped:   false,
	}
	return &orchestrator
}

func (o *tridentOrchestrator) transformPersistentState() error {
	// Transforming persistent state happens under two scenarios:
	// 1) Change in the persistent store version (e.g., from etcdv2 to etcdv3)
	// 2) Change in the Trident API version (e.g., from Trident API v1 to v2)
	version, err := o.storeClient.GetVersion()
	if err != nil && persistent_store.MatchKeyNotFoundErr(err) {
		// Persistent store and Trident API versions should be etcdv2 and v1 respectively.
		version = &persistent_store.PersistentStateVersion{
			string(persistent_store.EtcdV2Store),
			config.OrchestratorAPIVersion,
		}
	} else if err != nil {
		return fmt.Errorf("Couldn't determine the orchestrator persistent state version: %v",
			err)
	}
	if config.OrchestratorAPIVersion != version.OrchestratorAPIVersion {
		log.WithFields(log.Fields{
			"current_api_version": version.OrchestratorAPIVersion,
			"desired_api_version": config.OrchestratorAPIVersion,
		}).Info("Transforming Trident API objects on the persistent store.")
		//TODO: transform Trident API objects
	}
	dataMigrator := persistent_store.NewDataMigrator(o.storeClient,
		persistent_store.StoreType(version.PersistentStoreVersion))
	if err = dataMigrator.Run("/"+config.OrchestratorName, false); err != nil {
		return fmt.Errorf("Data migration failed: %v", err)
	}

	// Store the persistent store and API versions
	version.OrchestratorAPIVersion = config.OrchestratorAPIVersion
	version.PersistentStoreVersion = string(o.storeClient.GetType())
	if err = o.storeClient.SetVersion(version); err != nil {
		return fmt.Errorf("Failed to set the persistent state version after migration: %v",
			err)
	}
	return nil
}

func (o *tridentOrchestrator) Bootstrap() error {
	var err error = nil
	dvp.DefaultStoragePrefix = config.OrchestratorName
	dvp.ExtendedDriverVersion = config.OrchestratorName + "-" + config.OrchestratorVersion.String()
	if kubeFrontend, found := o.frontends["kubernetes"]; found {
		dvp.ExtendedDriverVersion =
			dvp.ExtendedDriverVersion + " " + kubeFrontend.Version()
	}

	// Transform persistent state, if necessary
	if err = o.transformPersistentState(); err != nil {
		return err
	}

	// Bootstrap state from persistent store
	if err = o.bootstrap(); err != nil {
		errMsg := fmt.Sprintf("Failed during bootstrapping: %s",
			err.Error())
		return fmt.Errorf(errMsg)
	}
	o.bootstrapped = true
	log.Infof("%s bootstrapped successfully.", config.OrchestratorName)
	return err
}

func (o *tridentOrchestrator) bootstrapBackends() error {
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
		newBackendExternal, err := o.AddStorageBackend(serializedConfig)
		if err != nil {
			return err
		}

		// Note that AddStorageBackend returns an external copy of the newly
		// added backend, so we have to go fetch it manually.
		newBackend := o.backends[newBackendExternal.Name]
		newBackend.Online = b.Online
		log.WithFields(log.Fields{
			"backend": newBackend.Name,
			"handler": "Bootstrap",
		}).Info("Added an existing backend.")
	}
	return nil
}

func (o *tridentOrchestrator) bootstrapStorageClasses() error {
	persistentStorageClasses, err := o.storeClient.GetStorageClasses()
	if err != nil {
		return err
	}
	for _, psc := range persistentStorageClasses {
		// TODO:  If the API evolves, check the Version field here.
		sc := storage_class.NewFromPersistent(psc)
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

func (o *tridentOrchestrator) bootstrapVolumes() error {
	volumes, err := o.storeClient.GetVolumes()
	if err != nil {
		return err
	}
	for _, v := range volumes {

		log.WithFields(log.Fields{
			"name":         v.Config.Name,
			"internalName": v.Config.InternalName,
			"size":         v.Config.Size,
			"backend":      v.Backend,
			"pool":         v.Pool,
		}).Debug("Bootstrapping volume.")

		// TODO:  If the API evolves, check the Version field here.
		var backend *storage.StorageBackend
		var ok bool
		backend, ok = o.backends[v.Backend]
		if !ok {
			return fmt.Errorf("Couldn't find backend %s for volume %s!",
				v.Backend, v.Config.Name)
		}
		storagePool, ok := backend.Storage[v.Pool]
		if !ok {
			return fmt.Errorf("Couldn't find storage pool %s on backend %s!",
				v.Pool, v.Backend)
		}
		vol := storage.NewVolume(v.Config, backend, storagePool)
		vol.Pool.AddVolume(vol, true)
		o.volumes[vol.Config.Name] = vol
		log.WithFields(log.Fields{
			"volume":  vol.Config.Name,
			"handler": "Bootstrap",
		}).Info("Added an existing volume.")
	}
	return nil
}

func (o *tridentOrchestrator) bootstrapVolTxns() error {
	volTxns, err := o.storeClient.GetVolumeTransactions()
	if err != nil {
		log.Warnf("Couldn't retrieve volume transaction logs: %s", err.Error())
	}
	for _, v := range volTxns {
		o.mutex.Lock()
		err = o.rollBackTransaction(v)
		o.mutex.Unlock()
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *tridentOrchestrator) bootstrap() error {
	// Fetching backend information

	type bootstrapFunc func() error
	for _, f := range []bootstrapFunc{o.bootstrapBackends,
		o.bootstrapStorageClasses, o.bootstrapVolumes, o.bootstrapVolTxns} {
		err := f()
		if err != nil {
			if persistent_store.MatchKeyNotFoundErr(err) {
				keyError := err.(*persistent_store.PersistentStoreError)
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
		if !backend.Online && !backend.HasVolumes() {
			delete(o.backends, backendName)
			err := o.storeClient.DeleteBackend(backend)
			if err != nil {
				return fmt.Errorf("Failed to delete empty offline backend %s:"+
					"%v", backendName, err)
			}
		}
	}

	return nil
}

func (o *tridentOrchestrator) rollBackTransaction(v *persistent_store.VolumeTransaction) error {
	log.WithFields(log.Fields{
		"volume":       v.Config.Name,
		"size":         v.Config.Size,
		"storageClass": v.Config.StorageClass,
		"op":           v.Op,
	}).Info("Processed volume transaction log.")
	switch v.Op {
	case persistent_store.AddVolume:
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
				return fmt.Errorf("Unable to clean up volume %s:  %v",
					v.Config.Name, err)
			}
		} else {
			// If the volume wasn't added into etcd, we attempt to delete
			// it at each backend, since we don't know where it might have
			// landed.  We're guaranteed that the volume name will be
			// unique across backends, thanks to the StoragePrefix field,
			// so this should be idempotent.
			// Handles case 2)
			for _, backend := range o.backends {
				if !backend.Online {
					// Backend offlining is serialized with volume creation,
					// so we can safely skip offline backends.
					continue
				}
				// TODO:  Change this to check the error type when backends
				// return a standardized error when a volume is not found.
				// For now, though, fail on an error, since backends currently
				// do not report errors for volumes not present.
				if err := backend.Driver.Destroy(
					backend.Driver.GetInternalVolumeName(v.Config.Name),
				); err != nil {
					return fmt.Errorf("Error attempting to clean up volume %s "+
						"from backend %s:  %v", v.Config.Name, backend.Name,
						err)
				}
			}
		}
		// Finally, we need to clean up the volume transaction.
		// Necessary for all cases.
		if err := o.storeClient.DeleteVolumeTransaction(v); err != nil {
			return fmt.Errorf("Failed to clean up volume addition transaction:"+
				" %v", err)
		}
	case persistent_store.DeleteVolume:
		// Because we remove the volume from etcd after we remove it from
		// the backend, we only need to take any special measures if
		// the volume is still in etcd.  In this case, it will have been
		// loaded into memory when previously bootstrapping.
		if _, ok := o.volumes[v.Config.Name]; ok {
			// Ignore errors, since the volume may no longer exist on the
			// backend
			log.WithFields(log.Fields{
				"name": v.Config.Name,
			}).Info("Volume for delete transaction found.")
			err := o.deleteVolume(v.Config.Name)
			if err != nil {
				return fmt.Errorf("Unable to clean up deleted volume %s:  %v",
					v.Config.Name, err)
			}
		} else {
			log.WithFields(log.Fields{
				"name": v.Config.Name,
			}).Info("Volume for delete transaction not found.")
		}
		if err := o.storeClient.DeleteVolumeTransaction(v); err != nil {
			return fmt.Errorf("Failed to clean up volume deletion transaction:"+
				"  %v", err)
		}
	}
	return nil
}

func (o *tridentOrchestrator) AddFrontend(f frontend.FrontendPlugin) {
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

func (o *tridentOrchestrator) validateBackendUpdate(
	oldBackend *storage.StorageBackend, newBackend *storage.StorageBackend,
) error {
	var err error
	err = nil
	errorList := make([]string, 0)

	// In case Trident supports custom backend names in the future, validate
	// that backend type isn't being changed as backend type has implications
	// for the internal volume name.
	if oldBackend.GetDriverName() != newBackend.GetDriverName() {
		errorList = append(errorList, "Cannot update the backend type")
	}

	// Identifying all storage pools in the old backend that have been used for
	// provisioning volumes.
	usedPools := make([]string, 0)
	for name, storagePool := range oldBackend.Storage {
		if len(storagePool.Volumes) > 0 {
			usedPools = append(usedPools, name)
		}
	}
	for _, storagePoolName := range usedPools {
		if _, ok := newBackend.Storage[storagePoolName]; !ok {
			// If a storage pool that contained volumes isn't present in
			// the new config, we can't use the new config.
			errorList = append(errorList,
				fmt.Sprintf("In-use storage pool %s not present in the "+
					"updated backend", storagePoolName))
		}
	}

	// Now validate that all of the previous storage classes work.
	storagePoolsForStorageClasses := make(map[string][]string)
	for storagePoolName, storagePool := range oldBackend.Storage {
		// We could just process the storage class list, but this way
		// we only care about the storage classes in use.
		for _, volume := range storagePool.Volumes {
			scName := volume.Config.StorageClass
			if _, found := storagePoolsForStorageClasses[scName]; !found {
				storagePoolsForStorageClasses[scName] = make([]string, 0, 1)
			}
			storagePoolsForStorageClasses[scName] =
				append(storagePoolsForStorageClasses[scName], storagePoolName)
		}
	}
	for scName, storagePoolList := range storagePoolsForStorageClasses {
		sc, found := o.storageClasses[scName]
		if !found {
			// The storage class for the volume was deleted. The volume remains
			// associated with the storage pool of the same name in the new
			// backend.
			continue
		}
		for _, storagePoolName := range storagePoolList {
			if newStoragePool, ok := newBackend.Storage[storagePoolName]; ok && !sc.Matches(newStoragePool) {
				log.WithFields(log.Fields{
					"backend":      newBackend.Name,
					"storagePool":  storagePoolName,
					"storageClass": scName,
				}).Debug("The storage pool in the new backend doesn't satisfy " +
					"the storage class.")
				errorList = append(errorList, fmt.Sprintf("Storage pool "+
					"%s has volumes with storage class %s, but it no longer "+
					"satisfies that storage class", storagePoolName, scName))
			} else if ok {
				log.WithFields(log.Fields{
					"backend":      newBackend.Name,
					"storagePool":  storagePoolName,
					"storageClass": scName,
				}).Debug("The storage pool in the new backend satisfies the " +
					"storage class.")
			}
		}
	}
	if len(errorList) > 0 {
		err = fmt.Errorf("Cannot update backend: %s", strings.Join(errorList,
			"; "))
	}
	return err
}

func (o *tridentOrchestrator) GetVersion() string {
	return config.OrchestratorVersion.String()
}

func (o *tridentOrchestrator) AddStorageBackend(configJSON string) (
	*storage.StorageBackendExternal, error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	storageBackend, err := factory.NewStorageBackendForConfig(configJSON)
	if err != nil {
		return nil, err
	}
	newBackend := true
	originalBackend, ok := o.backends[storageBackend.Name]
	if ok {
		newBackend = false
		if err = o.validateBackendUpdate(originalBackend, storageBackend); err != nil {
			return nil, err
		}
	}

	log.WithFields(log.Fields{
		"backend":       storageBackend.Name,
		"backendUpdate": !newBackend,
	}).Debug("Adding backend.")
	if err = o.updateBackendOnPersistentStore(storageBackend, newBackend); err != nil {
		return nil, err
	}
	o.backends[storageBackend.Name] = storageBackend

	classes := make([]string, 0, len(o.storageClasses))
	for _, storageClass := range o.storageClasses {
		if !newBackend {
			storageClass.RemovePoolsForBackend(originalBackend)
		}
		if added := storageClass.CheckAndAddBackend(storageBackend); added > 0 {
			classes = append(classes, storageClass.GetName())
		}
	}
	if len(classes) == 0 {
		log.WithFields(log.Fields{
			"backend": storageBackend.Name,
		}).Info("Newly added backend satisfies no storage classes.")
	} else {
		log.WithFields(log.Fields{
			"backend": storageBackend.Name,
		}).Infof("Newly added backend satisfies storage classes %s.",
			strings.Join(classes, ", "))
	}
	if !newBackend {
		for storagePoolName, storagePool := range originalBackend.Storage {
			for volName, vol := range storagePool.Volumes {
				vol.Backend = storageBackend
				// Note that the validation ensures that the storage pool
				// will exist in the new backend, as well.
				vol.Pool = storageBackend.Storage[storagePoolName]
				storageBackend.Storage[storagePoolName].Volumes[volName] = vol
			}
		}
	}
	return storageBackend.ConstructExternal(), nil
}

func (o *tridentOrchestrator) GetBackend(backend string) *storage.StorageBackendExternal {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	var storageBackend *storage.StorageBackend
	var found bool
	if storageBackend, found = o.backends[backend]; !found {
		return nil
	}
	return storageBackend.ConstructExternal()
}

func (o *tridentOrchestrator) ListBackends() []*storage.StorageBackendExternal {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	backends := make([]*storage.StorageBackendExternal, 0)
	for _, b := range o.backends {
		if b.Online {
			backends = append(backends, b.ConstructExternal())
		}
	}
	return backends
}

func (o *tridentOrchestrator) OfflineBackend(backendName string) (bool, error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	backend, found := o.backends[backendName]
	if !found {
		return false, fmt.Errorf("Backend %s not found.", backendName)
	}
	backend.Online = false
	storageClasses := make(map[string]*storage_class.StorageClass, 0)
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
		delete(o.backends, backendName)
		return true, o.storeClient.DeleteBackend(backend)
	}
	return true, o.storeClient.UpdateBackend(backend)
}

func (o *tridentOrchestrator) AddVolume(volumeConfig *storage.VolumeConfig) (
	externalVol *storage.VolumeExternal, err error) {
	var (
		backend *storage.StorageBackend
		vol     *storage.Volume
	)
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if _, ok := o.volumes[volumeConfig.Name]; ok {
		return nil, fmt.Errorf("Volume %s already exists.", volumeConfig.Name)
	}
	volumeConfig.Version = config.OrchestratorAPIVersion

	storageClass, ok := o.storageClasses[volumeConfig.StorageClass]
	if !ok {
		return nil, fmt.Errorf("Unknown storage class:  %s",
			volumeConfig.StorageClass)
	}
	protocol := volumeConfig.Protocol
	if protocol == config.ProtocolAny {
		protocol = o.getProtocol(volumeConfig.AccessMode)
	}
	pools := storageClass.GetStoragePoolsForProtocol(volumeConfig.Protocol)
	if len(pools) == 0 {
		return nil, fmt.Errorf("No available backends for storage class %s!",
			volumeConfig.StorageClass)
	}

	// Add transaction in case the operation must be rolled back later
	volTxn, err := o.addVolumeTransaction(volumeConfig)
	if err != nil {
		return nil, err
	}

	// Recovery function in case of error
	defer func() { o.addVolumeCleanup(err, backend, vol, volTxn, volumeConfig) }()

	// Randomize the storage pool list for better distribution of load across all pools.
	rand.Seed(time.Now().UnixNano())

	log.WithFields(log.Fields{
		"volume": volumeConfig.Name,
	}).Debugf("Looking through %d storage pools.", len(pools))

	errorMessages := make([]string, 0)

	// Choose a pool at random.
	for _, num := range rand.Perm(len(pools)) {
		backend = pools[num].Backend
		vol, err = backend.AddVolume(volumeConfig, pools[num], storageClass.GetAttributes())
		if vol != nil && err == nil {
			if vol.Config.Protocol == config.ProtocolAny {
				vol.Config.Protocol = backend.GetProtocol()
			}
			err = o.storeClient.AddVolume(vol)
			if err != nil {
				return nil, err
			}
			o.volumes[volumeConfig.Name] = vol
			externalVol = vol.ConstructExternal()
			return externalVol, nil
		} else if err != nil {
			log.WithFields(log.Fields{
				"backend": backend.Name,
				"pool":    pools[num].Name,
				"volume":  volumeConfig.Name,
				"error":   err,
			}).Warn("Failed to create the volume on this backend!")
			errorMessages = append(errorMessages,
				fmt.Sprintf("[Failed to create volume %s "+
					"on storage pool %s from backend %s: %s]",
					volumeConfig.Name, pools[num].Name, backend.Name,
					err.Error()))
		}
	}

	externalVol = nil
	if len(errorMessages) == 0 {
		err = fmt.Errorf("No suitable %s backend with \"%s\" "+
			"storage class and %s of free space was found! Find available backends"+
			" under %s.", volumeConfig.Protocol,
			volumeConfig.StorageClass, volumeConfig.Size, config.BackendURL)
	} else {
		err = fmt.Errorf("Encountered error(s) in creating the volume: %s",
			strings.Join(errorMessages, ", "))
	}
	return nil, err
}

func (o *tridentOrchestrator) CloneVolume(
	volumeConfig *storage.VolumeConfig,
) (*storage.VolumeExternal, error) {

	var (
		backend *storage.StorageBackend
		vol     *storage.Volume
	)
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if _, ok := o.volumes[volumeConfig.Name]; ok {
		return nil, fmt.Errorf("Volume %s already exists.", volumeConfig.Name)
	}
	volumeConfig.Version = config.OrchestratorAPIVersion

	// Get the source volume
	sourceVolume, found := o.volumes[volumeConfig.CloneSourceVolume]
	if !found {
		return nil, fmt.Errorf("Source volume not found: %s",
			volumeConfig.CloneSourceVolume)
	}
	sourceVolumeConfig := sourceVolume.Config

	// Clone the source config, as most of its attributes will apply to the clone
	cloneConfig := &storage.VolumeConfig{}
	sourceVolumeConfig.ConstructClone(cloneConfig)

	// Copy a few attributes from the request that will affect clone creation
	cloneConfig.Name = volumeConfig.Name
	cloneConfig.SplitOnClone = volumeConfig.SplitOnClone
	cloneConfig.CloneSourceVolume = volumeConfig.CloneSourceVolume
	cloneConfig.CloneSourceSnapshot = volumeConfig.CloneSourceSnapshot

	// Add transaction in case the operation must be rolled back later
	volTxn, err := o.addVolumeTransaction(volumeConfig)
	if err != nil {
		return nil, err
	}

	// Recovery function in case of error
	defer func() { o.addVolumeCleanup(err, backend, vol, volTxn, volumeConfig) }()

	backend = sourceVolume.Pool.Backend
	vol, err = backend.CloneVolume(cloneConfig, sourceVolume.Pool)
	if err != nil {
		return nil, fmt.Errorf("Failed to create cloned volume %s "+
			"on storage pool %s from backend %s: %v",
			cloneConfig.Name, sourceVolume.Pool.Name, backend.Name, err)
	}

	// Save references to new volume
	err = o.storeClient.AddVolume(vol)
	if err != nil {
		return nil, err
	}
	o.volumes[cloneConfig.Name] = vol

	return vol.ConstructExternal(), nil
}

// addVolumeTransaction is called from the volume create/clone methods to save
// a record of the operation in case it fails and must be cleaned up later.
func (o *tridentOrchestrator) addVolumeTransaction(
	volumeConfig *storage.VolumeConfig,
) (*persistent_store.VolumeTransaction, error) {

	// Check if an addVolume transaction already exists for this name.
	// If so, we failed earlier and we need to call the bootstrap cleanup code.
	// If this fails, return an error.  If it succeeds or no transaction
	// existed, log a new transaction in the persistent store and proceed.
	volTxn := &persistent_store.VolumeTransaction{
		Config: volumeConfig,
		Op:     persistent_store.AddVolume,
	}
	oldTxn, err := o.storeClient.GetExistingVolumeTransaction(volTxn)
	if err != nil {
		log.Warningf("Unable to check for existing volume transactions: %v", err)
		return nil, err
	}
	if oldTxn != nil {
		err = o.rollBackTransaction(oldTxn)
		if err != nil {
			return nil, fmt.Errorf("Unable to roll back existing transaction "+
				"for volume %s:  %v", volumeConfig.Name, err)
		}
	}

	err = o.storeClient.AddVolumeTransaction(volTxn)
	if err != nil {
		return nil, err
	}

	return volTxn, nil
}

// addVolumeCleanup is used as a deferred method from the volume create/clone methods
// to clean up in case anything goes wrong during the operation.
func (o *tridentOrchestrator) addVolumeCleanup(
	err error, backend *storage.StorageBackend, vol *storage.Volume,
	volTxn *persistent_store.VolumeTransaction, volumeConfig *storage.VolumeConfig) {

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
		// If we succeeded in adding the volume to etcd, err will not be
		// nil by the time we get here; we can only fail at removing the
		// volume txn at this point.
		if backend != nil && vol != nil {
			// We succeeded in adding the volume to the backend; now
			// delete it
			cleanupErr = backend.RemoveVolume(vol)
			if cleanupErr != nil {
				cleanupErr = fmt.Errorf("Unable to delete volume "+
					"from backend during cleanup:  %v", cleanupErr)
			}
		}
	}
	if cleanupErr == nil {
		// Only clean up the volume transaction if we've succeeded at
		// cleaning up on the backend or if we didn't need to do so in the
		// first place.
		txErr = o.storeClient.DeleteVolumeTransaction(volTxn)
		if txErr != nil {
			fmt.Errorf("Unable to clean up transaction:  %v", txErr)
		}
	}
	if cleanupErr != nil || txErr != nil {
		// Remove the volume from memory, if it's there, so that the user
		// can try to re-add.  This will trigger recovery code.
		delete(o.volumes, volumeConfig.Name)
		//externalVol = nil
		// Report on all errors we encountered.
		errList := make([]string, 0, 3)
		for _, e := range []error{err, cleanupErr, txErr} {
			if e != nil {
				errList = append(errList, e.Error())
			}
		}
		err = fmt.Errorf("%s", strings.Join(errList, "\n\t"))
	}
	return
}

func (o *tridentOrchestrator) GetVolume(volume string) *storage.VolumeExternal {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	vol, found := o.volumes[volume]
	if !found {
		return nil
	}
	return vol.ConstructExternal()
}

func (o *tridentOrchestrator) GetDriverTypeForVolume(
	vol *storage.VolumeExternal,
) string {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if b, ok := o.backends[vol.Backend]; ok {
		return b.Driver.Name()
	}
	return config.UnknownDriver
}

func (o *tridentOrchestrator) GetVolumeType(vol *storage.VolumeExternal) config.VolumeType {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	// Since the caller has a valid VolumeExternal and we're disallowing
	// backend deletion, we can assume that this will not hit a nil pointer.
	driver := o.backends[vol.Backend].GetDriverName()
	switch {
	case driver == dvp.OntapNASStorageDriverName:
		return config.ONTAP_NFS
	case driver == dvp.OntapSANStorageDriverName:
		return config.ONTAP_iSCSI
	case driver == dvp.SolidfireSANStorageDriverName:
		return config.SolidFire_iSCSI
	case driver == dvp.EseriesIscsiStorageDriverName:
		return config.Eseries_iSCSI
	default:
		return config.UnknownVolumeType
	}
}

func (o *tridentOrchestrator) ListVolumes() []*storage.VolumeExternal {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	volumes := make([]*storage.VolumeExternal, 0, len(o.volumes))
	for _, v := range o.volumes {
		volumes = append(volumes, v.ConstructExternal())
	}
	return volumes
}

// deleteVolume does the necessary work to delete a volume entirely.  It does
// not construct a transaction, nor does it take locks; it assumes that the
// caller will take care of both of these.  It also assumes that the volume
// exists in memory.
func (o *tridentOrchestrator) deleteVolume(volumeName string) error {

	volume := o.volumes[volumeName]

	// Note that this call will only return an error if the backend actually
	// fails to delete the volume.  If the volume does not exist on the backend,
	// the nDVP will not return an error.  Thus, we're fine.
	if err := volume.Backend.RemoveVolume(volume); err != nil {
		log.WithFields(log.Fields{
			"volume":  volumeName,
			"backend": volume.Backend.Name,
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
	if !volume.Backend.Online && !volume.Backend.HasVolumes() {
		if err := o.storeClient.DeleteBackend(volume.Backend); err != nil {
			log.WithFields(log.Fields{
				"backend": volume.Backend.Name,
				"volume":  volumeName,
			}).Error("Unable to delete offline backend from the backing store" +
				" after its last volume was deleted.  Delete the volume again" +
				" to remove the backend.")
			return err
		}
		delete(o.backends, volume.Backend.Name)
	}
	delete(o.volumes, volumeName)
	return nil
}

// DeleteVolume does the necessary set up to delete a volume during the course
// of normal operation, verifying that the volume is present in Trident and
// creating a transaction to ensure that the delete eventually completes.  It
// only resolves the transaction if all stages of deletion complete
// successfully, ensuring that the deletion will complete either upon retrying
// the delete or upon reboot of Trident.
// Returns true if the volume is found and false otherwise.
func (o *tridentOrchestrator) DeleteVolume(volumeName string) (found bool, err error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	volume, ok := o.volumes[volumeName]
	if !ok {
		return false, fmt.Errorf("Volume %s not found.", volumeName)
	}

	volTxn := &persistent_store.VolumeTransaction{
		Config: volume.Config,
		Op:     persistent_store.DeleteVolume,
	}
	if err = o.storeClient.AddVolumeTransaction(volTxn); err != nil {
		return true, err
	}
	if err = o.deleteVolume(volumeName); err != nil {
		// Do not try to delete the volume transaction here; instead, if we
		// fail, leave the transaction around and let the deletion be attempted
		// again.
		return true, err
	}
	err = o.storeClient.DeleteVolumeTransaction(volTxn)
	if err != nil {
		log.WithFields(log.Fields{
			"volume": volume,
		}).Warn("Unable to delete volume transaction.  Repeat deletion to " +
			"finalize.")
		// Reinsert the volume so that it can be deleted again
		o.volumes[volumeName] = volume
	}
	return true, nil
}

func (o *tridentOrchestrator) ListVolumesByPlugin(pluginName string) []*storage.VolumeExternal {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	volumes := make([]*storage.VolumeExternal, 0)
	for _, backend := range o.backends {
		if backendName := backend.GetDriverName(); pluginName != backendName {
			continue
		}
		for _, pool := range backend.Storage {
			for _, vol := range pool.Volumes {
				volumes = append(volumes, vol.ConstructExternal())
			}
		}
	}
	return volumes
}

// AttachVolume mounts a volume to the local host.  It ensures the mount point exists,
// and it calls the underlying storage driver to perform the attach operation as appropriate
// for the protocol and storage controller type.
func (o *tridentOrchestrator) AttachVolume(volumeName, mountpoint string, options map[string]string) error {

	volume, ok := o.volumes[volumeName]
	if !ok {
		return fmt.Errorf("Volume %s not found.", volumeName)
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
	dfOutput, dfOuputErr := dvp_utils.GetDFOutput()
	if dfOuputErr != nil {
		err = fmt.Errorf("Error checking if %v is already mounted: %v", mountpoint, dfOuputErr)
		return err
	}
	for _, e := range dfOutput {
		if e.Target == mountpoint {
			log.Debugf("%v is already mounted", mountpoint)
			return nil
		}
	}

	return volume.Backend.Driver.Attach(volume.Config.InternalName, mountpoint, options)
}

// DetachVolume unmounts a volume from the local host.  It ensures the volume is already
// mounted, and it calls the underlying storage driver to perform the detach operation as
// appropriate for the protocol and storage controller type.
func (o *tridentOrchestrator) DetachVolume(volumeName, mountpoint string) error {

	volume, ok := o.volumes[volumeName]
	if !ok {
		return fmt.Errorf("Volume %s not found.", volumeName)
	}

	log.WithFields(log.Fields{"volume": volumeName, "mountpoint": mountpoint}).Debug("Unmounting volume.")

	// Check if the mount point exists, so we know that it's attached and must be cleaned up
	_, err := os.Stat(mountpoint)
	if err != nil {
		// Not attached, so nothing to do
		return nil
	}

	// Unmount the volume
	err = volume.Backend.Driver.Detach(volume.Config.InternalName, mountpoint)
	if err != nil {
		return err
	}

	// Best effort removal of the mount point
	os.Remove(mountpoint)
	return nil
}

// getProtocol returns the appropriate protocol name based on volume access mode
//or an empty string if all protocols are applicable.
// ReadWriteOnce -> Any (File + Block)
// ReadOnlyMany -> File
// ReadWriteMany -> File
func (o *tridentOrchestrator) getProtocol(mode config.AccessMode) config.Protocol {
	switch mode {
	case config.ReadWriteOnce:
		return config.ProtocolAny
	case config.ReadOnlyMany:
		return config.File
	case config.ReadWriteMany:
		return config.File
	default:
		return config.ProtocolAny
	}
}

func (o *tridentOrchestrator) AddStorageClass(scConfig *storage_class.Config) (*storage_class.StorageClassExternal, error) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	sc := storage_class.New(scConfig)
	if _, ok := o.storageClasses[sc.GetName()]; ok {
		return nil, fmt.Errorf("Storage class %s already exists.", sc.GetName())
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

func (o *tridentOrchestrator) GetStorageClass(scName string) *storage_class.StorageClassExternal {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	sc, ok := o.storageClasses[scName]
	if !ok {
		return nil
	}
	// Storage classes aren't threadsafe (we modify them during runtime),
	// so return a copy, rather than the original
	return sc.ConstructExternal()
}

func (o *tridentOrchestrator) ListStorageClasses() []*storage_class.StorageClassExternal {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	ret := make([]*storage_class.StorageClassExternal, 0, len(o.storageClasses))
	for _, sc := range o.storageClasses {
		ret = append(ret, sc.ConstructExternal())
	}
	return ret
}

// Delete storage class deletes a storage class from the orchestrator iff
// no volumes exist that use that storage class.
func (o *tridentOrchestrator) DeleteStorageClass(scName string) (bool, error) {
	sc, found := o.storageClasses[scName]
	if !found {
		return found, fmt.Errorf("Storage class %s not found.", scName)
	}
	volumes := sc.GetVolumes()
	if len(volumes) > 0 {
		volNames := make([]string, len(volumes))
		for i, vol := range volumes {
			volNames[i] = vol.Config.Name
		}
		log.WithFields(log.Fields{
			"storageClass": scName,
			"volumes":      strings.Join(volNames, ","),
		}).Warn("The storage class being removed still has volumes in use. " +
			"These volumes will continue to refer to the storage class.")
	}
	// Note that we don't need a tranasaction here.  If this crashes prior
	// to successful deletion, the storage class will be reloaded upon reboot
	// automatically, which is consistent with the method never having returned
	// successfully.
	err := o.storeClient.DeleteStorageClass(sc)
	if err != nil {
		return found, err
	}
	delete(o.storageClasses, scName)
	for _, storagePool := range sc.GetStoragePoolsForProtocol(config.ProtocolAny) {
		storagePool.RemoveStorageClass(scName)
	}
	return found, nil
}

func (o *tridentOrchestrator) updateBackendOnPersistentStore(
	backend *storage.StorageBackend, newBackend bool,
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
