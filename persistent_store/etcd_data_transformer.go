// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"fmt"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
)

type EtcdDataTransformer struct {
	etcdClient    Client
	dryRun        bool
	shouldPersist bool
}

type EtcdDataTransformerResult struct {
	Backends []*storage.BackendPersistent
	Volumes  []*storage.VolumeExternal
}

func NewEtcdDataTransformer(etcdClient Client, dryRun, shouldPersist bool) *EtcdDataTransformer {
	return &EtcdDataTransformer{
		etcdClient:    etcdClient,
		dryRun:        dryRun,
		shouldPersist: shouldPersist,
	}
}

func (m *EtcdDataTransformer) RunPrechecks() error {

	// Ensure we have valid etcd V3 data present
	etcdVersion, err := m.etcdClient.GetVersion()
	if err != nil {
		if MatchKeyNotFoundErr(err) {
			return fmt.Errorf("etcdv3 data not found, install an earlier Trident version in the range " +
				"[v18.01, v19.01] to automatically upgrade from etcdv2 to etcdv3")
		} else {
			return fmt.Errorf("could not check for etcdv3 data; %v", err)
		}
	} else {
		log.WithFields(log.Fields{
			"PersistentStoreVersion": etcdVersion.PersistentStoreVersion,
			"OrchestratorAPIVersion": etcdVersion.OrchestratorAPIVersion,
		}).Debug("Found etcdv3 persistent state version.")

		if etcdVersion.PersistentStoreVersion == string(EtcdV3Store) {
			// we should upgrade from EtcdV3Store -> EtcdV3bStore
			return nil
		} else if etcdVersion.PersistentStoreVersion == string(EtcdV3bStore) {
			// nothing to do at this time
			return nil
		} else {
			// can't upgrade
			return fmt.Errorf("etcd persistent state version is %s, not %s",
				etcdVersion.PersistentStoreVersion, EtcdV3Store)
		}
	}

	return nil
}

func (m *EtcdDataTransformer) Run() (*EtcdDataTransformerResult, error) {

	backends, err := m.upgradeBackendsToUseUUIDs()
	if err != nil {
		return nil, err
	}

	volumes, err := m.upgradeVolumesToUseBackendUUIDs(backends)
	if err != nil {
		return nil, err
	}

	if err := m.persistBackends(backends); err != nil {
		return nil, err
	}

	if err := m.persistVolumes(volumes); err != nil {
		return nil, err
	}

	if err := m.writeEtcdSchemaVersion(); err != nil {
		return nil, err
	}

	if m.dryRun {
		log.Info("Transformation dry run completed, no problems found.")
	} else {
		log.Info("Transformation succeeded.")
	}

	result := &EtcdDataTransformerResult{
		Backends: backends,
		Volumes:  volumes,
	}
	return result, nil
}

func (m *EtcdDataTransformer) upgradeBackendsToUseUUIDs() ([]*storage.BackendPersistent, error) {

	// get the backends
	backends, err := m.etcdClient.GetBackends()
	if err != nil {
		return nil, fmt.Errorf("could not read backends from etcd; %v", err)
	}

	if len(backends) == 0 {
		if m.dryRun {
			log.Info("Dry run: no backends found.")
		} else {
			log.Info("No backends found, none will be modified.")
		}
		return nil, nil
	}

	if m.dryRun {
		log.WithField("count", len(backends)).Info("Dry run: read all backends.")
		return nil, nil
	}

	for _, backend := range backends {
		if backend.BackendUUID == "" {
			backend.BackendUUID = uuid.New().String()
			log.WithFields(log.Fields{
				"backend":     backend.Name,
				"backendUUID": backend.BackendUUID,
			}).Debug("Transformed backend.")
		}
	}
	log.WithField("count", len(backends)).Info("Transformed backends to have UUIDs.")

	return backends, nil
}

func (m *EtcdDataTransformer) upgradeVolumesToUseBackendUUIDs(backends []*storage.BackendPersistent) ([]*storage.VolumeExternal, error) {

	// get the volumes
	volumes, err := m.etcdClient.GetVolumes()
	if err != nil {
		return nil, fmt.Errorf("could not read volumes from etcd; %v", err)
	}

	if len(volumes) == 0 {
		if m.dryRun {
			log.Info("Dry run: no volumes found.")
		} else {
			log.Info("No volumes found, none will be modified.")
		}
		return nil, nil
	}

	if m.dryRun {
		log.WithField("count", len(volumes)).Info("Dry run: read all volumes.")
		return nil, nil
	}

	// lookup UUIDs for backends
	backendUUIDsByName := make(map[string]string)
	for _, backend := range backends {
		backendUUIDsByName[backend.Name] = backend.BackendUUID
	}

	// update volumes that are missing backendUUIDs to use them
	for _, volume := range volumes {
		if volume.BackendUUID == "" {
			// look up the backend's UUID and modify the Volume to use the UUID
			backendName := volume.Backend
			backendUUID := backendUUIDsByName[backendName]
			volume.BackendUUID = backendUUID
			volume.Backend = ""
			log.WithFields(log.Fields{
				"volume":             volume.Config.Name,
				"volume.backend":     backendName,
				"volume.backendUUID": volume.BackendUUID,
			}).Debug("Transformed volume.")
		} else {
			log.WithFields(log.Fields{
				"volume":             volume.Config.Name,
				"volume.backend":     volume.Backend,
				"volume.backendUUID": volume.BackendUUID,
			}).Debug("Skipped volume.")
		}
	}
	log.WithField("count", len(volumes)).Info("Transformed volumes to use backend UUIDs.")

	return volumes, nil
}

func (m *EtcdDataTransformer) persistBackends(backends []*storage.BackendPersistent) error {

	if m.dryRun {
		return nil
	}

	if !m.shouldPersist {
		return nil
	}

	for _, backend := range backends {
		if err := m.etcdClient.UpdateBackendPersistent(backend); err != nil {
			return fmt.Errorf("could not write backend resource; %v", err)
		}
		log.WithFields(log.Fields{
			"backend":     backend.Name,
			"backendUUID": backend.BackendUUID,
		}).Debug("Persisted backend.")
	}
	return nil
}

func (m *EtcdDataTransformer) persistVolumes(volumes []*storage.VolumeExternal) error {

	if m.dryRun {
		return nil
	}

	if !m.shouldPersist {
		return nil
	}

	for _, volume := range volumes {
		if err := m.etcdClient.UpdateVolumePersistent(volume); err != nil {
			return fmt.Errorf("could not write volume resource; %v", err)
		}
		log.WithFields(log.Fields{
			"volume": volume.Config.Name,
		}).Debug("Persisted volume.")
	}
	return nil
}

func (m *EtcdDataTransformer) writeEtcdSchemaVersion() error {

	if m.dryRun {
		return nil
	}

	if !m.shouldPersist {
		return nil
	}

	etcdVersion3b := &config.PersistentStateVersion{
		PersistentStoreVersion: string(EtcdV3bStore),
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
	}
	if err := m.etcdClient.SetVersion(etcdVersion3b); err != nil {
		return fmt.Errorf("failed to set the persistent state version after transformation: %v", err)
	}

	log.WithFields(log.Fields{
		"PersistentStoreVersion": etcdVersion3b.PersistentStoreVersion,
		"OrchestratorAPIVersion": etcdVersion3b.OrchestratorAPIVersion,
	}).Info("Wrote Etcd-based persistent state version.")

	return nil
}
