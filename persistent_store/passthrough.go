// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	sc "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

type PassthroughClient struct {
	liveBackends map[string]*storage.Backend
	bootBackends []*storage.BackendPersistent
	version      *config.PersistentStateVersion
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

	ctx := GenerateRequestContext(nil, "", ContextSourceInternal)
	client := &PassthroughClient{
		liveBackends: make(map[string]*storage.Backend),
		bootBackends: make([]*storage.BackendPersistent, 0),
		version: &config.PersistentStateVersion{
			PersistentStoreVersion: "passthrough",
			OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		},
	}

	err := client.initialize(ctx, configPath)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// initialize loads one or more driver config files from the specified config path
func (c *PassthroughClient) initialize(ctx context.Context, configPath string) error {

	if configPath == "" {
		return errors.New("passthrough store initialization failed, config path must be specified")
	}

	// Check path
	configPathInfo, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.New("passthrough store initialization failed, config path does not exist")
		} else {
			return err
		}
	}

	if configPathInfo.Mode().IsDir() {

		// If config path is a directory, load all config files.
		Logc(ctx).WithField("configPath", configPath).Debug("Passthrough store loading config directory.")

		files, err := ioutil.ReadDir(configPath)
		if err != nil {
			return err
		}

		for _, file := range files {
			// Skip all non-regular files
			if !file.Mode().IsRegular() {
				continue
			}
			err = c.loadBackend(ctx, filepath.Join(configPath, file.Name()))
			if err != nil {
				return err
			}
		}
		return nil

	} else if configPathInfo.Mode().IsRegular() {

		// If config path is a single file, just load it.
		return c.loadBackend(ctx, configPath)

	} else {
		return errors.New("passthrough store initialization failed, invalid config path")
	}
}

// loadBackend loads a single driver config file from the specified path
func (c *PassthroughClient) loadBackend(ctx context.Context, configPath string) error {

	Logc(ctx).WithField("configPath", configPath).Debug("Passthrough store loading config file.")

	// Read config file
	fileContents, err := ioutil.ReadFile(configPath)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"configPath": configPath,
			"error":      err,
		}).Fatal("Passthrough store could not read configuration file.")
	}

	// Convert config file to persistent backend JSON
	backendJSON, err := c.unmarshalConfig(ctx, fileContents)
	if err != nil {
		return err
	}

	var backend storage.BackendPersistent
	err = json.Unmarshal([]byte(backendJSON), &backend)
	if err != nil {
		return err
	}

	c.bootBackends = append(c.bootBackends, &backend)
	return nil
}

// unmarshalConfig accepts a driver JSON/YAML config and converts it to a persistent backend
// JSON config as needed by the bootstrapping process.
func (c *PassthroughClient) unmarshalConfig(ctx context.Context, fileContents []byte) (string, error) {

	// Convert config (JSON or YAML) to JSON
	configJSONBytes, err := yaml.YAMLToJSON(fileContents)
	if err != nil {
		return "", err
	}
	configJSON := string(configJSONBytes)

	commonConfig, err := drivers.ValidateCommonSettings(ctx, configJSON)
	if err != nil {
		return "", fmt.Errorf("input failed validation: %v", err)
	}

	var configType string
	switch commonConfig.StorageDriverName {
	case drivers.OntapNASStorageDriverName,
		drivers.OntapNASQtreeStorageDriverName,
		drivers.OntapNASFlexGroupStorageDriverName,
		drivers.OntapSANStorageDriverName,
		drivers.OntapSANEconomyStorageDriverName:
		configType = "ontap_config"
	case drivers.SolidfireSANStorageDriverName:
		configType = "solidfire_config"
	case drivers.EseriesIscsiStorageDriverName:
		configType = "eseries_config"
	case drivers.AWSNFSStorageDriverName:
		configType = "aws_config"
	case drivers.AzureNFSStorageDriverName:
		configType = "azure_config"
	case drivers.GCPNFSStorageDriverName:
		configType = "gcp_config"
	case drivers.FakeStorageDriverName:
		configType = "fake_config"
	default:
		return "", fmt.Errorf("unknown storage driver: %v", commonConfig.StorageDriverName)
	}

	persistentBackend := &storage.BackendPersistent{
		Version: config.OrchestratorAPIVersion,
		Config:  storage.PersistentStorageBackendConfig{},
		Name:    "",
		//Online:  true,
		State: storage.Online,
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
	c.liveBackends = make(map[string]*storage.Backend)
	c.bootBackends = make([]*storage.BackendPersistent, 0)
	return nil
}

func (c *PassthroughClient) GetConfig() *ClientConfig {
	return &ClientConfig{}
}

func (c *PassthroughClient) GetVersion(context.Context) (*config.PersistentStateVersion, error) {
	return c.version, nil
}

func (c *PassthroughClient) SetVersion(context.Context, *config.PersistentStateVersion) error {
	return nil
}

func (c *PassthroughClient) AddBackend(ctx context.Context, backend *storage.Backend) error {

	// The passthrough store persists backends for the purpose of contacting
	// the storage controllers.  If the store ever needs to write backends
	// back to a file system for subsequent bootstrapping, that logic will live
	// here and in UpdateBackend().

	Logc(ctx).WithField("backend", backend.Name).Debugf("Passthrough store adding backend.")
	c.liveBackends[backend.Name] = backend
	return nil
}

func (c *PassthroughClient) AddBackendPersistent(context.Context, *storage.BackendPersistent) error {
	return NewPersistentStoreError(NotSupported, "")
}

func (c *PassthroughClient) GetBackend(ctx context.Context, backendName string) (*storage.BackendPersistent, error) {

	existingBackend, ok := c.liveBackends[backendName]
	if !ok {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}

	return existingBackend.ConstructPersistent(ctx), nil
}

func (c *PassthroughClient) UpdateBackend(ctx context.Context, backend *storage.Backend) error {

	if _, ok := c.liveBackends[backend.Name]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, backend.Name)
	}

	Logc(ctx).Debugf("Passthrough store updating backend: %s", backend.Name)
	c.liveBackends[backend.Name] = backend
	return nil
}

// UpdateBackendPersistent updates a backend's persistent state
func (c *PassthroughClient) UpdateBackendPersistent(context.Context, *storage.BackendPersistent) error {
	return nil
}

func (c *PassthroughClient) DeleteBackend(_ context.Context, backend *storage.Backend) error {

	if _, ok := c.liveBackends[backend.Name]; !ok {
		return NewPersistentStoreError(KeyNotFoundErr, backend.Name)
	}

	delete(c.liveBackends, backend.Name)
	return nil
}

// ReplaceBackendAndUpdateVolumes renames a backend and updates all volumes to
// reflect the new backend name
func (c *PassthroughClient) ReplaceBackendAndUpdateVolumes(
	context.Context, *storage.Backend, *storage.Backend,
) error {
	//TODO
	return NewPersistentStoreError(NotSupported, "")
}

// GetBackends is called by the orchestrator during bootstrapping, so the
// passthrough store returns the persistent backend objects it read from config
// files.
func (c *PassthroughClient) GetBackends(context.Context) ([]*storage.BackendPersistent, error) {

	backendList := make([]*storage.BackendPersistent, 0)

	for _, backend := range c.bootBackends {
		backendList = append(backendList, backend)
	}

	return backendList, nil
}

func (c *PassthroughClient) DeleteBackends(context.Context) error {
	c.liveBackends = make(map[string]*storage.Backend)
	return nil
}

func (c *PassthroughClient) AddVolume(context.Context, *storage.Volume) error {
	return nil
}

// AddVolumePersistent saves a volume's persistent state to the persistent store
func (c *PassthroughClient) AddVolumePersistent(context.Context, *storage.VolumeExternal) error {
	return nil
}

// GetVolume is not called by the orchestrator, which caches all volumes in
// memory after bootstrapping.  So this method need not do anything.
func (c *PassthroughClient) GetVolume(_ context.Context, volName string) (*storage.VolumeExternal, error) {
	return nil, NewPersistentStoreError(KeyNotFoundErr, volName)
}

func (c *PassthroughClient) UpdateVolume(context.Context, *storage.Volume) error {
	return nil
}

// UpdateVolumePersistent updates a volume's persistent state
func (c *PassthroughClient) UpdateVolumePersistent(context.Context, *storage.VolumeExternal) error {
	return nil
}

func (c *PassthroughClient) DeleteVolume(context.Context, *storage.Volume) error {
	return nil
}

func (c *PassthroughClient) DeleteVolumeIgnoreNotFound(context.Context, *storage.Volume) error {
	return nil
}

// GetVolumes gets up-to-date volume info from each storage backend.  To increase
// efficiency, it contacts each backend in a separate goroutine.  Because multiple
// backends may be managed by the orchestrator, the passthrough layer should remain
// as responsive as possible even if a backend is unavailable or returns an error
// during volume discovery.
func (c *PassthroughClient) GetVolumes(ctx context.Context) ([]*storage.VolumeExternal, error) {

	volumeChannel := make(chan *storage.VolumeExternalWrapper)

	var waitGroup sync.WaitGroup
	waitGroup.Add(len(c.liveBackends))

	// Get volumes from each backend in a goroutine
	for _, backend := range c.liveBackends {
		go c.getVolumesFromBackend(ctx, backend, volumeChannel, &waitGroup)
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
			Logc(ctx).Error(wrapper.Error)
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
	ctx context.Context, backend *storage.Backend, volumeChannel chan *storage.VolumeExternalWrapper,
	waitGroup *sync.WaitGroup,
) {
	defer waitGroup.Done()

	// Create a channel that each backend can use, then copy values from
	// there to the common channel until the backend channel is closed.
	backendChannel := make(chan *storage.VolumeExternalWrapper)
	go backend.Driver.GetVolumeExternalWrappers(ctx, backendChannel)
	for volume := range backendChannel {
		if volume.Volume != nil {
			//volume.Volume.Backend = backend.Name
			volume.Volume.BackendUUID = backend.BackendUUID
		}
		volumeChannel <- volume
	}
}

func (c *PassthroughClient) DeleteVolumes(context.Context) error {
	return nil
}

func (c *PassthroughClient) AddVolumeTransaction(context.Context, *storage.VolumeTransaction) error {
	return nil
}

func (c *PassthroughClient) GetVolumeTransactions(context.Context) ([]*storage.VolumeTransaction, error) {
	return make([]*storage.VolumeTransaction, 0), nil
}

func (c *PassthroughClient) GetExistingVolumeTransaction(
	context.Context, *storage.VolumeTransaction,
) (*storage.VolumeTransaction, error) {
	return nil, nil
}

func (c *PassthroughClient) UpdateVolumeTransaction(context.Context, *storage.VolumeTransaction) error {
	return nil
}

func (c *PassthroughClient) DeleteVolumeTransaction(context.Context, *storage.VolumeTransaction) error {
	return nil
}

func (c *PassthroughClient) AddStorageClass(context.Context, *sc.StorageClass) error {
	return nil
}

func (c *PassthroughClient) GetStorageClass(_ context.Context, scName string) (*sc.Persistent, error) {
	return nil, NewPersistentStoreError(KeyNotFoundErr, scName)
}

func (c *PassthroughClient) GetStorageClasses(context.Context) ([]*sc.Persistent, error) {
	return make([]*sc.Persistent, 0), nil
}

func (c *PassthroughClient) DeleteStorageClass(context.Context, *sc.StorageClass) error {
	return nil
}

func (c *PassthroughClient) AddOrUpdateNode(context.Context, *utils.Node) error {
	return nil
}

func (c *PassthroughClient) GetNode(_ context.Context, nName string) (*utils.Node, error) {
	return nil, NewPersistentStoreError(KeyNotFoundErr, nName)
}

func (c *PassthroughClient) GetNodes(context.Context) ([]*utils.Node, error) {
	return make([]*utils.Node, 0), nil
}

func (c *PassthroughClient) DeleteNode(context.Context, *utils.Node) error {
	return nil
}

func (c *PassthroughClient) AddSnapshot(context.Context, *storage.Snapshot) error {
	return nil
}

func (c *PassthroughClient) GetSnapshot(
	_ context.Context, _, snapshotName string,
) (*storage.SnapshotPersistent, error) {
	return nil, NewPersistentStoreError(KeyNotFoundErr, snapshotName)
}

// GetSnapshots retrieves all snapshots
func (c *PassthroughClient) GetSnapshots(context.Context) ([]*storage.SnapshotPersistent, error) {
	return make([]*storage.SnapshotPersistent, 0), nil
}

func (c *PassthroughClient) UpdateSnapshot(context.Context, *storage.Snapshot) error {
	return nil
}

func (c *PassthroughClient) DeleteSnapshot(context.Context, *storage.Snapshot) error {
	return nil
}

func (c *PassthroughClient) DeleteSnapshotIgnoreNotFound(context.Context, *storage.Snapshot) error {
	return nil
}

func (c *PassthroughClient) DeleteSnapshots(context.Context) error {
	return nil
}
