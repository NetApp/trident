// Copyright 2017 NetApp, Inc. All Rights Reserved.

package persistent_store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	dvp "github.com/netapp/netappdvp/storage_drivers"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/drivers/fake"
	"github.com/netapp/trident/storage"
	sc "github.com/netapp/trident/storage_class"
)

type PassthroughClient struct {
	liveBackends map[string]*storage.StorageBackend
	bootBackends []*storage.StorageBackendPersistent
	version      *PersistentStateVersion
}

// NewPassthroughClient returns a client that satisfies the
// persistent_store.Client interface, which is used by the orchestrator
// during bootstrapping.  The passthrough store uses the storage as
// the source of truth, so it doesn't actually store anything.  Instead,
// the store is pre-populated with backend objects from one or more backend
// config files prior to bootstrapping.  The volume info is then read
// directly from the storage controllers during the bootstrapping process.
// The passthrough store does not need to persist any objects, including
// transactions and storage classes, once the orchestrator has started.
// The passthrough store is primarily useful for the Docker Volume Plugin
// use case, which doesn't easily support a separate persistence layer
// and has no support for storage classes.
func NewPassthroughClient(configPath string) (*PassthroughClient, error) {

	client := &PassthroughClient{
		liveBackends: make(map[string]*storage.StorageBackend),
		bootBackends: make([]*storage.StorageBackendPersistent, 0),
		version: &PersistentStateVersion{
			"passthrough",
			config.OrchestratorAPIVersion,
		},
	}

	err := client.initialize(configPath)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// initialize loads one or more driver config files from the specified config path
func (c *PassthroughClient) initialize(configPath string) error {

	if configPath == "" {
		return errors.New("Passthrough store initialization failed, config path must be specified.")
	}

	// Check path
	configPathInfo, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("Passthrough store initialization failed, config path does not exist.")
		} else {
			return err
		}
	}

	if configPathInfo.Mode().IsDir() {

		// If config path is a directory, load all config files.
		log.WithField("configPath", configPath).Debug("Passthrough store loading config directory.")

		files, err := ioutil.ReadDir(configPath)
		if err != nil {
			return err
		}

		for _, file := range files {
			// Skip all non-regular files
			if !file.Mode().IsRegular() {
				continue
			}
			err = c.loadBackend(filepath.Join(configPath, file.Name()))
			if err != nil {
				return err
			}
		}
		return nil

	} else if configPathInfo.Mode().IsRegular() {

		// If config path is a single file, just load it.
		return c.loadBackend(configPath)

	} else {
		return errors.New("Passthrough store initialization failed, invalid config path.")
	}
}

// loadBackend loads a single driver config file from the specified path
func (c *PassthroughClient) loadBackend(configPath string) error {

	log.WithField("configPath", configPath).Debug("Passthrough store loading config file.")

	// Read config file
	fileContents, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.WithFields(log.Fields{
			"configPath": configPath,
			"error":      err,
		}).Fatal("Passthrough store could not read configuration file.")
	}
	configJSON := string(fileContents)

	// Convert config JSON to persistent backend JSON
	backendJSON, err := c.unmarshalConfig(configJSON)

	var backend storage.StorageBackendPersistent
	err = json.Unmarshal([]byte(backendJSON), &backend)
	if err != nil {
		return err
	}

	c.bootBackends = append(c.bootBackends, &backend)
	return nil
}

// unmarshalConfig accepts a driver JSON config and converts it to a persistent backend
// JSON config as needed by the bootstrapping process.
func (c *PassthroughClient) unmarshalConfig(configJSON string) (string, error) {

	commonConfig, err := dvp.ValidateCommonSettings(configJSON)
	if err != nil {
		return "", fmt.Errorf("Input failed validation: %v", err)
	}

	var configType string
	switch commonConfig.StorageDriverName {
	case dvp.OntapNASStorageDriverName, dvp.OntapNASQtreeStorageDriverName, dvp.OntapSANStorageDriverName:
		configType = "ontap_config"
	case dvp.SolidfireSANStorageDriverName:
		configType = "solidfire_config"
	case dvp.EseriesIscsiStorageDriverName:
		configType = "eseries_config"
	case fake.FakeStorageDriverName:
		configType = "fake_config"
	default:
		return "", fmt.Errorf("Unknown storage driver: %v", commonConfig.StorageDriverName)
	}

	persistentBackend := &storage.StorageBackendPersistent{
		Version: config.OrchestratorAPIVersion,
		Config:  storage.PersistentStorageBackendConfig{},
		Name:    "",
		Online:  true,
	}
	persistentBackendJSON, _ := json.Marshal(persistentBackend)

	oldConfig := `"config":{}`
	newConfig := `"config":{"` + configType + `":` + configJSON + "}"
	return strings.Replace(string(persistentBackendJSON), oldConfig, newConfig, 1), nil
}

func (c *PassthroughClient) GetType() StoreType {
	return PassthroughStore
}

func (c *PassthroughClient) Stop() error {
	c.liveBackends = make(map[string]*storage.StorageBackend)
	c.bootBackends = make([]*storage.StorageBackendPersistent, 0)
	return nil
}

func (c *PassthroughClient) GetConfig() *ClientConfig {
	return &ClientConfig{}
}

func (c *PassthroughClient) GetVersion() (*PersistentStateVersion, error) {
	return c.version, nil
}

func (c *PassthroughClient) SetVersion(version *PersistentStateVersion) error {
	return nil
}

func (c *PassthroughClient) AddBackend(backend *storage.StorageBackend) error {

	// The passthrough store persists backends for the purpose of contacting
	// the storage controllers.  If the store ever needs to write backends
	// back to a file system for subsequent bootstrapping, that logic will live
	// here and in UpdateBackend().
	log.WithField("backend", backend.Name).Debugf("Passthrough store adding backend.")
	c.liveBackends[backend.Name] = backend
	return nil
}

func (c *PassthroughClient) GetBackend(backendName string) (*storage.StorageBackendPersistent, error) {

	existingBackend, ok := c.liveBackends[backendName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}

	return existingBackend.ConstructPersistent(), nil
}

func (c *PassthroughClient) UpdateBackend(backend *storage.StorageBackend) error {

	if _, ok := c.liveBackends[backend.Name]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, backend.Name)
	}

	log.Debugf("Passthrough store updating backend: %s", backend.Name)
	c.liveBackends[backend.Name] = backend
	return nil
}

func (c *PassthroughClient) DeleteBackend(backend *storage.StorageBackend) error {

	if _, ok := c.liveBackends[backend.Name]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, backend.Name)
	}

	delete(c.liveBackends, backend.Name)
	return nil
}

// GetBackends is called by the orchestrator during bootstrapping, so the
// passthrough store returns the persistent backend objects it read from config
// files.
func (c *PassthroughClient) GetBackends() ([]*storage.StorageBackendPersistent, error) {

	backendList := make([]*storage.StorageBackendPersistent, 0)

	for _, backend := range c.bootBackends {
		backendList = append(backendList, backend)
	}

	return backendList, nil
}

func (c *PassthroughClient) DeleteBackends() error {
	c.liveBackends = make(map[string]*storage.StorageBackend)
	return nil
}

func (c *PassthroughClient) AddVolume(vol *storage.Volume) error {
	return nil
}

// GetVolume is not called by the orchestrator, which caches all volumes in
// memory after bootstrapping.  So this method need not do anything.
func (c *PassthroughClient) GetVolume(volName string) (*storage.VolumeExternal, error) {
	return nil, NewPersistentStoreError(KeyNotFoundErr, volName)
}

func (c *PassthroughClient) UpdateVolume(vol *storage.Volume) error {
	return nil
}

func (c *PassthroughClient) DeleteVolume(vol *storage.Volume) error {
	return nil
}

func (c *PassthroughClient) DeleteVolumeIgnoreNotFound(vol *storage.Volume) error {
	return nil
}

// GetVolumes gets up-to-date volume info from each storage backend.  To increase
// efficiency, it contacts each backend in a separate goroutine.  Because multiple
// backends may be managed by the orchestrator, the passthrough layer should remain
// as responsive as possible even if a backend is unavailable or returns an error
// during volume discovery.
func (c *PassthroughClient) GetVolumes() ([]*storage.VolumeExternal, error) {

	volumeChannel := make(chan *storage.VolumeExternalWrapper)

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(c.liveBackends))

	// Get volumes from each backend in a goroutine
	for _, backend := range c.liveBackends {
		go c.getVolumesFromBackend(backend, volumeChannel, &waitGroup)
	}

	// Close the channel when all other goroutines are done
	go func() {
		waitGroup.Wait()
		close(volumeChannel)
	}()

	// Read the volumes as they come in from the goroutines
	volumes := make([]*storage.VolumeExternal, 0)
	for wrapper := range volumeChannel {
		if wrapper.Error != nil {
			log.Error(wrapper.Error)
		} else {
			volumes = append(volumes, wrapper.Volume)
		}
	}

	return volumes, nil
}

// getVolumesFromBackend reads all of the volumes managed by a single backend.
// This method is designed to run in a goroutine, so it passes its results back
// via a channel that is shared by all such goroutines.
func (c *PassthroughClient) getVolumesFromBackend(
	backend *storage.StorageBackend, volumeChannel chan *storage.VolumeExternalWrapper,
	waitGroup *sync.WaitGroup,
) {
	defer waitGroup.Done()

	// Create a channel that each backend can use, then copy values from
	// there to the common channel until the backend channel is closed.
	backendChannel := make(chan *storage.VolumeExternalWrapper)
	go backend.Driver.GetVolumeExternalWrappers(backendChannel)
	for volume := range backendChannel {
		if volume.Volume != nil {
			volume.Volume.Backend = backend.Name
		}
		volumeChannel <- volume
	}
}

func (c *PassthroughClient) DeleteVolumes() error {
	return nil
}

func (c *PassthroughClient) AddVolumeTransaction(volTxn *VolumeTransaction) error {
	return nil
}

func (c *PassthroughClient) GetVolumeTransactions() ([]*VolumeTransaction, error) {
	return make([]*VolumeTransaction, 0), nil
}

func (c *PassthroughClient) GetExistingVolumeTransaction(volTxn *VolumeTransaction) (*VolumeTransaction, error) {
	return nil, nil
}

func (c *PassthroughClient) DeleteVolumeTransaction(volTxn *VolumeTransaction) error {
	return nil
}

func (c *PassthroughClient) AddStorageClass(sc *sc.StorageClass) error {
	return nil
}

func (c *PassthroughClient) GetStorageClass(scName string) (*sc.StorageClassPersistent, error) {
	return nil, NewPersistentStoreError(KeyNotFoundErr, scName)
}

func (c *PassthroughClient) GetStorageClasses() ([]*sc.StorageClassPersistent, error) {
	return make([]*sc.StorageClassPersistent, 0), nil
}

func (c *PassthroughClient) DeleteStorageClass(sc *sc.StorageClass) error {
	return nil
}
