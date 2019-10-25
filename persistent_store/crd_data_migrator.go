// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v3"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
)

const crRegistrationTimeout = 3 * time.Minute

type CRDDataMigrator struct {
	etcdClient    EtcdClient
	crdClient     CRDClient
	dryRun        bool
	transformer   *EtcdDataTransformer
	totalCount    int
	migratedCount int
	startTime     time.Time
}

func NewCRDDataMigrator(etcdClient EtcdClient, crdClient CRDClient, dryRun bool, t *EtcdDataTransformer) *CRDDataMigrator {
	return &CRDDataMigrator{
		etcdClient:    etcdClient,
		crdClient:     crdClient,
		dryRun:        dryRun,
		transformer:   t,
		totalCount:    0,
		migratedCount: 0,
	}
}

func (m *CRDDataMigrator) RunPrechecks() error {

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

		if etcdVersion.PersistentStoreVersion != string(EtcdV3Store) {
			return fmt.Errorf("etcd persistent state version is %s, not %s",
				etcdVersion.PersistentStoreVersion, EtcdV3Store)
		}
	}

	// Ensure there are no Trident objects already stored in custom resources
	if err := m.ensureNoExistingCRs(); err != nil {
		return err
	}

	if err := m.transformer.RunPrechecks(); err != nil {
		return err
	}

	return nil
}

func (m *CRDDataMigrator) ensureNoExistingCRs() error {

	checkCRs := func() error {

		// Ensure there are no CRD-based backends
		if hasBackends, err := m.crdClient.HasBackends(); err != nil {
			return fmt.Errorf("could not check for CRD-based backends; %v", err)
		} else if hasBackends {
			return backoff.Permanent(errors.New("CRD-based backends are already present, aborting migration"))
		} else {
			log.Debug("No CRD-based backends found.")
		}

		// Ensure there are no CRD-based storage classes
		if hasStorageClasses, err := m.crdClient.HasStorageClasses(); err != nil {
			return fmt.Errorf("could not check for CRD-based storage classes; %v", err)
		} else if hasStorageClasses {
			return backoff.Permanent(errors.New("CRD-based storage classes are already present, aborting migration"))
		} else {
			log.Debug("No CRD-based storage classes found.")
		}

		// Ensure there are no CRD-based volumes
		if hasVolumes, err := m.crdClient.HasVolumes(); err != nil {
			return fmt.Errorf("could not check for CRD-based volumes; %v", err)
		} else if hasVolumes {
			return backoff.Permanent(errors.New("CRD-based volumes are already present, aborting migration"))
		} else {
			log.Debug("No CRD-based volumes found.")
		}

		// Ensure there are no CRD-based volume transactions
		if hasVolumeTransactions, err := m.crdClient.HasVolumeTransactions(); err != nil {
			return fmt.Errorf("could not check for CRD-based transactions; %v", err)
		} else if hasVolumeTransactions {
			return backoff.Permanent(errors.New("CRD-based transactions are already present, aborting migration"))
		} else {
			log.Debug("No CRD-based transactions found.")
		}

		// Ensure there is no CRD-based version
		crdVersion, err := m.crdClient.GetVersion()
		if err != nil {
			if MatchKeyNotFoundErr(err) {
				log.Debug("Trident CRDs not found, migration can proceed.")
			} else {
				return fmt.Errorf("could not check for Trident CRDs; %v", err)
			}
		} else {
			log.WithFields(log.Fields{
				"PersistentStoreVersion": crdVersion.PersistentStoreVersion,
				"OrchestratorAPIVersion": crdVersion.OrchestratorAPIVersion,
			}).Debug("Found CRD-based persistent state version.")
			return backoff.Permanent(errors.New("Trident CRDs are already present, aborting migration"))
		}

		return nil
	}

	checkCRsNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"err": err,
		}).Debug("Could not check for existing CRs, waiting.")
	}

	checkCRsBackoff := backoff.NewExponentialBackOff()
	checkCRsBackoff.MaxInterval = 5 * time.Second
	checkCRsBackoff.MaxElapsedTime = crRegistrationTimeout

	if err := backoff.RetryNotify(checkCRs, checkCRsBackoff, checkCRsNotify); err != nil {
		return err
	}

	log.Debug("No CRD-based Trident objects found.")
	return nil
}

func (m *CRDDataMigrator) Run() error {

	var (
		backends       []*storage.BackendPersistent
		storageClasses []*storageclass.Persistent
		volumes        []*storage.VolumeExternal
		transactions   []*storage.VolumeTransaction
	)

	// Transform data into the latest schema
	transformerResult, err := m.transformer.Run()
	if err != nil {
		return err
	}

	// Backends and volumes are returned by the schema transformer
	backends = transformerResult.Backends
	volumes = transformerResult.Volumes

	// Read storage classes from etcd
	storageClasses, err = m.etcdClient.GetStorageClasses()
	if err != nil {
		return fmt.Errorf("could not read storage classes from etcd; %v", err)
	}

	// Read transactions from etcd
	transactions, err = m.etcdClient.GetVolumeTransactions()
	if err != nil {
		return fmt.Errorf("could not read transactions from etcd; %v", err)
	}

	// Determine number of objects to migrate
	m.totalCount = len(backends) + len(storageClasses) + len(volumes) + len(transactions)

	// Save start time
	m.startTime = time.Now()

	log.Infof("Migrating %d objects. Please do not interrupt this operation!", m.totalCount)

	// Migrate backends
	if err := m.migrateBackends(backends); err != nil {
		return err
	}

	// Migrate storage classes
	if err := m.migrateStorageClasses(storageClasses); err != nil {
		return err
	}

	// Migrate volumes
	if err := m.migrateVolumes(volumes); err != nil {
		return err
	}

	// Migrate transactions
	if err := m.migrateTransactions(transactions); err != nil {
		return err
	}

	// Write schema version to prevent future migrations
	if err := m.writeCRDSchemaVersion(); err != nil {
		return err
	}

	if m.dryRun {
		log.Info("Migration dry run completed, no problems found.")
	} else {
		log.WithField("count", m.migratedCount).Info("Migration succeeded.")
	}

	return nil
}

func (m *CRDDataMigrator) migrateBackends(backends []*storage.BackendPersistent) error {

	if backends == nil || len(backends) == 0 {
		if m.dryRun {
			log.Info("Dry run: no backends found.")
		} else {
			log.Info("No backends found, none will be migrated.")
		}
		return nil
	}

	if m.dryRun {
		log.WithField("count", len(backends)).Info("Dry run: read all backends.")
		return nil
	}

	for _, backend := range backends {
		if err := m.crdClient.AddBackendPersistent(backend); err != nil {
			return fmt.Errorf("could not write backend resource; %v", err)
		}
		log.WithField("backend", backend.Name).Debug("Copied backend.")

		m.migratedCount++
		m.logTimeRemainingEstimate()
	}
	log.WithField("count", len(backends)).Info("Copied all backends to CRD resources.")

	return nil
}

func (m *CRDDataMigrator) migrateStorageClasses(storageClasses []*storageclass.Persistent) error {

	if len(storageClasses) == 0 {
		if m.dryRun {
			log.Info("Dry run: no storage classes found.")
		} else {
			log.Info("No storage classes found, none will be migrated.")
		}
		return nil
	}

	if m.dryRun {
		log.WithField("count", len(storageClasses)).Info("Dry run: read all storage classes.")
		return nil
	}

	for _, sc := range storageClasses {
		if err := m.crdClient.AddStorageClassPersistent(sc); err != nil {
			return fmt.Errorf("could not write storage class resource; %v", err)
		}
		log.WithField("sc", sc.GetName()).Debug("Copied storage class.")

		m.migratedCount++
		m.logTimeRemainingEstimate()
	}
	log.WithField("count", len(storageClasses)).Info("Copied all storage classes to CRD resources.")

	return nil
}

func (m *CRDDataMigrator) migrateVolumes(volumes []*storage.VolumeExternal) error {

	if volumes == nil || len(volumes) == 0 {
		if m.dryRun {
			log.Info("Dry run: no volumes found.")
		} else {
			log.Info("No volumes found, none will be migrated.")
		}
		return nil
	}

	if m.dryRun {
		log.WithField("count", len(volumes)).Info("Dry run: read all volumes.")
		return nil
	}

	for _, volume := range volumes {
		if err := m.crdClient.AddVolumePersistent(volume); err != nil {
			return fmt.Errorf("could not write volume resource; %v", err)
		}
		log.WithField("volume", volume.Config.Name).Debug("Copied volume.")

		m.migratedCount++
		m.logTimeRemainingEstimate()
	}
	log.WithField("count", len(volumes)).Info("Copied all volumes to CRD resources.")

	return nil
}

func (m *CRDDataMigrator) migrateTransactions(transactions []*storage.VolumeTransaction) error {

	if len(transactions) == 0 {
		if m.dryRun {
			log.Info("Dry run: no transactions found.")
		} else {
			log.Info("No transactions found, none will be migrated.")
		}
		return nil
	}

	if m.dryRun {
		log.WithField("count", len(transactions)).Info("Dry run: read all transactions.")
		return nil
	}

	for _, txn := range transactions {
		if err := m.crdClient.AddVolumeTransaction(txn); err != nil {
			return fmt.Errorf("could not write transaction resource; %v", err)
		}
		log.WithField("volume", txn.Config.Name).Debug("Copied transaction.")

		m.migratedCount++
		m.logTimeRemainingEstimate()
	}
	log.WithField("count", len(transactions)).Info("Copied all transactions to CRD resources.")

	return nil
}

func (m *CRDDataMigrator) writeCRDSchemaVersion() error {

	if m.dryRun {
		return nil
	}

	crdVersion := &config.PersistentStateVersion{
		PersistentStoreVersion: string(CRDV1Store),
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
	}
	if err := m.crdClient.SetVersion(crdVersion); err != nil {
		return fmt.Errorf("failed to set the persistent state version after migration: %v", err)
	}

	log.WithFields(log.Fields{
		"PersistentStoreVersion": crdVersion.PersistentStoreVersion,
		"OrchestratorAPIVersion": crdVersion.OrchestratorAPIVersion,
	}).Info("Wrote CRD-based persistent state version.")

	return nil
}

func (m *CRDDataMigrator) writeEtcdSchemaVersion() error {

	if m.dryRun {
		return nil
	}

	etcdVersion := &config.PersistentStateVersion{
		PersistentStoreVersion: string(EtcdV3bStore),
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
	}
	if err := m.etcdClient.SetVersion(etcdVersion); err != nil {
		return fmt.Errorf("failed to set the persistent state version after migration: %v", err)
	}

	log.WithFields(log.Fields{
		"PersistentStoreVersion": etcdVersion.PersistentStoreVersion,
		"OrchestratorAPIVersion": etcdVersion.OrchestratorAPIVersion,
	}).Info("Wrote Etcd-based persistent state version.")

	return nil
}

func (m *CRDDataMigrator) logTimeRemainingEstimate() {

	// Ensure we have migrated something, and log the time remaining every 100 objects
	if (m.migratedCount%100) != 0 || m.migratedCount <= 0 {
		return
	}

	// Determine time elapsed and ensure it is positive
	timeElapsed := time.Since(m.startTime)
	if timeElapsed <= 0 {
		return
	}

	// Determine average time per object
	nanosecondsPerObject := timeElapsed.Nanoseconds() / int64(m.migratedCount)

	// Determine time remaining
	objectsRemaining := m.totalCount - m.migratedCount
	timeRemaining := time.Duration(nanosecondsPerObject * int64(objectsRemaining)).Truncate(time.Second)

	log.WithField("estimatedTimeRemaining", timeRemaining).Infof("Migrated %d of %d objects.",
		m.migratedCount, m.totalCount)
}
