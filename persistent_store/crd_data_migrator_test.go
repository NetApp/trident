// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"flag"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sc "github.com/netapp/trident/storage_class"
)

var (
	etcdv2Version = &config.PersistentStateVersion{
		PersistentStoreVersion: string(EtcdV2Store),
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
	}
	etcdv3Version = &config.PersistentStateVersion{
		PersistentStoreVersion: string(EtcdV3Store),
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
	}
	crdv1Version = &config.PersistentStateVersion{
		PersistentStoreVersion: string(CRDV1Store),
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
	}
)

func init() {
	flag.Parse()
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
}

func getClients() (*EtcdClientV3, *CRDClientV1, error) {

	// Set up etcdv3 client
	etcdClient, err := NewEtcdClientV3(*etcdSrc)
	if err != nil {
		return nil, nil, fmt.Errorf("Creating etcd client failed: %v", err)
	}
	_ = etcdClient.DeleteKeys("/trident")

	// Set up crdv1 client
	crdClient, _ := GetTestKubernetesClient()

	return etcdClient, crdClient, nil
}

func populateEtcdCluster(client EtcdClient) error {

	// The fake client doesn't support GenerateName in K8S objects, so we're limited to one backend here.
	fakeBackend := getFakeBackendWithName("fake_backend")
	if err := client.AddBackend(fakeBackend); err != nil {
		return err
	}

	for i := 1; i <= 3; i++ {
		vol := getFakeVolumeWithName(fmt.Sprintf("fake_volume_%d", i), fakeBackend)
		if err := client.AddVolume(vol); err != nil {
			return err
		}

		// we need a Backend name set to be able to test upgrades
		volExternal, err := client.GetVolume(vol.Config.Name)
		if err != nil {
			return err
		}
		volExternal.Backend = fakeBackend.Name
		if err := client.UpdateVolumePersistent(volExternal); err != nil {
			return err
		}
	}

	for i := 1; i <= 4; i++ {
		storageClass := getFakeStorageClassWithName(fmt.Sprintf("fake_sc_%d", i), 10*i, true, "thin")
		if err := client.AddStorageClass(storageClass); err != nil {
			return err
		}
	}

	for i := 1; i <= 5; i++ {
		txn := getFakeVolumeTransactionWithName(fmt.Sprintf("fake_txn_%d", i))
		if err := client.AddVolumeTransaction(txn); err != nil {
			return err
		}
	}

	if err := client.SetVersion(etcdv3Version); err != nil {
		return err
	}

	return nil
}

func TestEtcdV3ToCRDV1Migration(t *testing.T) {

	// Set up clients
	etcdClient, crdClient, err := getClients()
	if err != nil {
		t.Fatal(err)
	}

	// Populate the source cluster
	if err := populateEtcdCluster(etcdClient); err != nil {
		t.Fatalf("Populating the source etcd cluster failed: %v", err)
	}

	// Run prechecks
	dryRun := false
	shouldPersist := true
	transformer := NewEtcdDataTransformer(etcdClient, dryRun, shouldPersist)
	dataMigrator := NewCRDDataMigrator(etcdClient, crdClient, dryRun, transformer)
	if err = dataMigrator.RunPrechecks(); err != nil {
		t.Fatalf("CRD data migration prechecks failed: %v", err)
	}

	// Migrate the data
	if err = dataMigrator.Run(); err != nil {
		t.Fatalf("CRD data migration failed: %v", err)
	}

	// Assert the backends are present and match
	etcdBackends, err := etcdClient.GetBackends()
	if err != nil {
		t.Fatalf("Error reading backends from etcd: %v", err)
	}
	etcdBackendMap := make(map[string]*storage.BackendPersistent)
	for _, b := range etcdBackends {
		etcdBackendMap[b.Name] = b
	}

	crdBackends, err := crdClient.GetBackends()
	if err != nil {
		t.Fatalf("Error reading backends from K8S: %v", err)
	}
	crdBackendMap := make(map[string]*storage.BackendPersistent)
	for _, b := range crdBackends {
		crdBackendMap[b.Name] = b
	}

	if len(crdBackends) == 0 {
		t.Fatal("No CRD backends found")
	} else if len(crdBackends) != len(etcdBackends) {
		t.Fatal("Backend count mismatch")
	}

	for backendName, etcdBackend := range etcdBackendMap {
		crdBackend, ok := crdBackendMap[backendName]
		if !ok {
			t.Fatalf("Backend not found in CRD: %v", backendName)
		}
		log.WithFields(log.Fields{
			"etcdBackend": etcdBackend,
			"crdBackend":  crdBackend,
		}).Info("Comparing.")
		if !reflect.DeepEqual(etcdBackend, crdBackend) {
			t.Fatal("Backends are not equal")
		}
	}

	// Assert the volumes are present and match
	etcdVolumes, err := etcdClient.GetVolumes()
	if err != nil {
		t.Fatalf("Error reading volumes from etcd: %v", err)
	}
	etcdVolumeMap := make(map[string]*storage.VolumeExternal)
	for _, v := range etcdVolumes {
		etcdVolumeMap[v.Config.Name] = v
	}

	crdVolumes, err := crdClient.GetVolumes()
	if err != nil {
		t.Fatalf("Error reading volumes from K8S: %v", err)
	}
	crdVolumeMap := make(map[string]*storage.VolumeExternal)
	for _, v := range crdVolumes {
		crdVolumeMap[v.Config.Name] = v
	}

	if len(crdVolumes) == 0 {
		t.Fatal("No CRD volumes found")
	} else if len(crdVolumes) != len(etcdVolumes) {
		t.Fatal("Volume count mismatch")
	}

	for volumeName, etcdVolume := range etcdVolumeMap {
		crdVolume, ok := crdVolumeMap[volumeName]
		if !ok {
			t.Fatalf("Volume not found in CRD: %v", volumeName)
		}
		if !reflect.DeepEqual(etcdVolume, crdVolume) {
			t.Fatal("Volumes are not equal")
		}
	}

	// Assert the storage classes are present and match
	etcdSCs, err := etcdClient.GetStorageClasses()
	if err != nil {
		t.Fatalf("Error reading storage classes from etcd: %v", err)
	}
	etcdSCMap := make(map[string]*sc.Persistent)
	for _, s := range etcdSCs {
		etcdSCMap[s.Config.Name] = s
	}

	crdSCs, err := crdClient.GetStorageClasses()
	if err != nil {
		t.Fatalf("Error reading storage classes from K8S: %v", err)
	}
	crdSCMap := make(map[string]*sc.Persistent)
	for _, s := range crdSCs {
		crdSCMap[s.Config.Name] = s
	}

	if len(crdSCs) == 0 {
		t.Fatal("No CRD storage classes found")
	} else if len(crdSCs) != len(etcdSCs) {
		t.Fatal("Storage class count mismatch")
	}

	for scName, etcdSC := range etcdSCMap {
		crdSC, ok := crdSCMap[scName]
		if !ok {
			t.Fatalf("Storage class not found in CRD: %v", scName)
		}
		if !reflect.DeepEqual(etcdSC, crdSC) {
			t.Fatal("Storage classes are not equal")
		}
	}

	// Assert the volume transactions are present and match
	etcdTxns, err := etcdClient.GetVolumeTransactions()
	if err != nil {
		t.Fatalf("Error reading transactions from etcd: %v", err)
	}
	etcdTxnMap := make(map[string]*storage.VolumeTransaction)
	for _, t := range etcdTxns {
		etcdTxnMap[t.Config.Name] = t
	}

	crdTxns, err := crdClient.GetVolumeTransactions()
	if err != nil {
		t.Fatalf("Error reading transactions from K8S: %v", err)
	}
	crdTxnMap := make(map[string]*storage.VolumeTransaction)
	for _, t := range crdTxns {
		crdTxnMap[t.Config.Name] = t
	}

	if len(crdTxns) == 0 {
		t.Fatal("No CRD transactions found")
	} else if len(crdTxns) != len(etcdTxns) {
		t.Fatal("Transaction count mismatch")
	}

	for txnName, etcdTxn := range etcdTxnMap {
		crdTxn, ok := crdTxnMap[txnName]
		if !ok {
			t.Fatalf("Transaction not found in CRD: %v", txnName)
		}
		if !reflect.DeepEqual(etcdTxn, crdTxn) {
			log.WithFields(log.Fields{
				"etcdTxn": etcdTxn,
				"crdTxn":  crdTxn,
			}).Error("Comparing.")
			t.Fatal("Transactions are not equal")
		}
	}

	// Assert new version is present
	version, err := crdClient.GetVersion()
	if err != nil || version == nil {
		t.Fatalf("Error reading version from K8S: %v", err)
	}
	if version.PersistentStoreVersion != string(CRDV1Store) {
		t.Fatalf("Incorrect version after migration: %v", version.PersistentStoreVersion)
	}
}

func TestEtcdV3ToCRDV1MigrationMissingEtcdVersion(t *testing.T) {

	// Set up clients
	etcdClient, crdClient, err := getClients()
	if err != nil {
		t.Fatal(err)
	}

	// Run prechecks
	dryRun := false
	shouldPersist := true
	transformer := NewEtcdDataTransformer(etcdClient, dryRun, shouldPersist)
	dataMigrator := NewCRDDataMigrator(etcdClient, crdClient, dryRun, transformer)
	if err = dataMigrator.RunPrechecks(); err == nil {
		t.Fatal("CRD data migration prechecks should have failed!")
	} else {
		t.Log(err)
	}
}

func TestEtcdV3ToCRDV1MigrationWrongEtcdVersion(t *testing.T) {

	// Set up clients
	etcdClient, crdClient, err := getClients()
	if err != nil {
		t.Fatal(err)
	}

	if err := etcdClient.SetVersion(etcdv2Version); err != nil {
		t.Fatalf("Could not set etcd store version: %v", err)
	}

	// Run prechecks
	dryRun := false
	shouldPersist := true
	transformer := NewEtcdDataTransformer(etcdClient, dryRun, shouldPersist)
	dataMigrator := NewCRDDataMigrator(etcdClient, crdClient, dryRun, transformer)
	if err = dataMigrator.RunPrechecks(); err == nil {
		t.Fatal("CRD data migration prechecks should have failed!")
	} else {
		t.Log(err)
	}
}

func TestEtcdV3ToCRDV1MigrationCRDBackendsExist(t *testing.T) {

	// Set up clients
	etcdClient, crdClient, err := getClients()
	if err != nil {
		t.Fatal(err)
	}

	if err := etcdClient.SetVersion(etcdv3Version); err != nil {
		t.Fatal("Could not set etcd store version!")
	}

	fakeBackend := getFakeBackend()
	fakeBackend.BackendUUID = uuid.New().String()
	if err := crdClient.AddBackend(fakeBackend); err != nil {
		t.Fatalf("Could not add CRD backend: %v", err)
	}

	// Run prechecks
	dryRun := false
	shouldPersist := true
	transformer := NewEtcdDataTransformer(etcdClient, dryRun, shouldPersist)
	dataMigrator := NewCRDDataMigrator(etcdClient, crdClient, dryRun, transformer)
	if err = dataMigrator.RunPrechecks(); err == nil {
		t.Fatal("CRD data migration prechecks should have failed!")
	} else {
		t.Log(err)
	}
}

func TestEtcdV3ToCRDV1MigrationCRDVolumesExist(t *testing.T) {

	// Set up clients
	etcdClient, crdClient, err := getClients()
	if err != nil {
		t.Fatal(err)
	}

	if err := etcdClient.SetVersion(etcdv3Version); err != nil {
		t.Fatal("Could not set etcd store version!")
	}

	volume := getFakeVolume(getFakeBackend())
	if err := crdClient.AddVolume(volume); err != nil {
		t.Fatalf("Could not add CRD volume: %v", err)
	}

	// Run prechecks
	dryRun := false
	shouldPersist := true
	transformer := NewEtcdDataTransformer(etcdClient, dryRun, shouldPersist)
	dataMigrator := NewCRDDataMigrator(etcdClient, crdClient, dryRun, transformer)
	if err = dataMigrator.RunPrechecks(); err == nil {
		t.Fatal("CRD data migration prechecks should have failed!")
	} else {
		t.Log(err)
	}
}

func TestEtcdV3ToCRDV1MigrationCRDStorageClassesExist(t *testing.T) {

	// Set up clients
	etcdClient, crdClient, err := getClients()
	if err != nil {
		t.Fatal(err)
	}

	if err := etcdClient.SetVersion(etcdv3Version); err != nil {
		t.Fatal("Could not set etcd store version!")
	}

	storageClass := getFakeStorageClass()
	if err := crdClient.AddStorageClass(storageClass); err != nil {
		t.Fatalf("Could not add CRD storage class: %v", err)
	}

	// Run prechecks
	dryRun := false
	shouldPersist := true
	transformer := NewEtcdDataTransformer(etcdClient, dryRun, shouldPersist)
	dataMigrator := NewCRDDataMigrator(etcdClient, crdClient, dryRun, transformer)
	if err = dataMigrator.RunPrechecks(); err == nil {
		t.Fatal("CRD data migration prechecks should have failed!")
	} else {
		t.Log(err)
	}
}

func TestEtcdV3ToCRDV1MigrationCRDTransactionsExist(t *testing.T) {

	// Set up clients
	etcdClient, crdClient, err := getClients()
	if err != nil {
		t.Fatal(err)
	}

	if err := etcdClient.SetVersion(etcdv3Version); err != nil {
		t.Fatal("Could not set etcd store version!")
	}

	txn := getFakeVolumeTransaction()
	if err := crdClient.AddVolumeTransaction(txn); err != nil {
		t.Fatalf("Could not add CRD transaction: %v", err)
	}

	// Run prechecks
	dryRun := false
	shouldPersist := true
	transformer := NewEtcdDataTransformer(etcdClient, dryRun, shouldPersist)
	dataMigrator := NewCRDDataMigrator(etcdClient, crdClient, dryRun, transformer)
	if err = dataMigrator.RunPrechecks(); err == nil {
		t.Fatal("CRD data migration prechecks should have failed!")
	} else {
		t.Log(err)
	}
}

func TestEtcdV3ToCRDV1MigrationCRDVersionExists(t *testing.T) {

	// Set up clients
	etcdClient, crdClient, err := getClients()
	if err != nil {
		t.Fatal(err)
	}

	if err := etcdClient.SetVersion(etcdv3Version); err != nil {
		t.Fatal("Could not set etcd store version!")
	}

	if err := crdClient.SetVersion(crdv1Version); err != nil {
		t.Fatal("Could not set CRD store version!")
	}

	// Run prechecks
	dryRun := false
	shouldPersist := true
	transformer := NewEtcdDataTransformer(etcdClient, dryRun, shouldPersist)
	dataMigrator := NewCRDDataMigrator(etcdClient, crdClient, dryRun, transformer)
	if err = dataMigrator.RunPrechecks(); err == nil {
		t.Fatal("CRD data migration prechecks should have failed!")
	} else {
		t.Log(err)
	}
}

func TestEtcdV3ToCRDV1MigrationDryRun(t *testing.T) {

	// Set up clients
	etcdClient, crdClient, err := getClients()
	if err != nil {
		t.Fatal(err)
	}

	// Populate the source cluster
	if err := populateEtcdCluster(etcdClient); err != nil {
		t.Fatalf("Populating the source etcd cluster failed: %v", err)
	}

	// Run prechecks
	dryRun := true
	shouldPersist := true
	transformer := NewEtcdDataTransformer(etcdClient, dryRun, shouldPersist)
	dataMigrator := NewCRDDataMigrator(etcdClient, crdClient, dryRun, transformer)
	if err = dataMigrator.RunPrechecks(); err != nil {
		t.Fatalf("CRD data migration prechecks failed: %v", err)
	}

	// Migrate the data
	if err = dataMigrator.Run(); err != nil {
		t.Fatalf("CRD data migration failed: %v", err)
	}

	hasBackends, err := crdClient.HasBackends()
	if err != nil {
		t.Fatalf("Error reading backends from K8S: %v", err)
	} else if hasBackends {
		t.Fatal("No backends should have been migrated.")
	}

	hasVolumes, err := crdClient.HasVolumes()
	if err != nil {
		t.Fatalf("Error reading volumes from K8S: %v", err)
	} else if hasVolumes {
		t.Fatal("No volumes should have been migrated.")
	}

	hasSCs, err := crdClient.HasStorageClasses()
	if err != nil {
		t.Fatalf("Error reading storage classes from K8S: %v", err)
	} else if hasSCs {
		t.Fatal("No storage classes should have been migrated.")
	}

	hasTxns, err := crdClient.HasVolumeTransactions()
	if err != nil {
		t.Fatalf("Error reading transactions from K8S: %v", err)
	} else if hasTxns {
		t.Fatal("No transactions should have been migrated.")
	}

	// Assert new version is not present
	version, err := crdClient.GetVersion()
	if err != nil {
		if MatchKeyNotFoundErr(err) {
			t.Log("CRD store version not found")
		} else {
			t.Fatalf("Error reading version from K8S: %v", err)
		}
	} else {
		t.Fatalf("No CRD store version should have been written, found %v.", version.PersistentStoreVersion)
	}
}

func TestEtcdV3ToCRDV1MigrationNoData(t *testing.T) {

	// Set up clients
	etcdClient, crdClient, err := getClients()
	if err != nil {
		t.Fatal(err)
	}

	if err := etcdClient.SetVersion(etcdv3Version); err != nil {
		t.Fatal("Could not set etcd store version!")
	}

	// Run prechecks
	dryRun := false
	shouldPersist := true
	transformer := NewEtcdDataTransformer(etcdClient, dryRun, shouldPersist)
	dataMigrator := NewCRDDataMigrator(etcdClient, crdClient, dryRun, transformer)
	if err = dataMigrator.RunPrechecks(); err != nil {
		t.Fatalf("CRD data migration prechecks failed: %v", err)
	}

	// Migrate the data
	if err = dataMigrator.Run(); err != nil {
		t.Fatalf("CRD data migration failed: %v", err)
	}

	hasBackends, err := crdClient.HasBackends()
	if err != nil {
		t.Fatalf("Error reading backends from K8S: %v", err)
	} else if hasBackends {
		t.Fatal("No backends should have been migrated.")
	}

	hasVolumes, err := crdClient.HasVolumes()
	if err != nil {
		t.Fatalf("Error reading volumes from K8S: %v", err)
	} else if hasVolumes {
		t.Fatal("No volumes should have been migrated.")
	}

	hasSCs, err := crdClient.HasStorageClasses()
	if err != nil {
		t.Fatalf("Error reading storage classes from K8S: %v", err)
	} else if hasSCs {
		t.Fatal("No storage classes should have been migrated.")
	}

	hasTxns, err := crdClient.HasVolumeTransactions()
	if err != nil {
		t.Fatalf("Error reading transactions from K8S: %v", err)
	} else if hasTxns {
		t.Fatal("No transactions should have been migrated.")
	}

	// Assert new version is present
	version, err := crdClient.GetVersion()
	if err != nil || version == nil {
		t.Fatalf("Error reading version from K8S: %v", err)
	}
	if version.PersistentStoreVersion != string(CRDV1Store) {
		t.Fatalf("Incorrect version after migration: %v", version.PersistentStoreVersion)
	}
}
