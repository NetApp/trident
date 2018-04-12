// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap"
	"github.com/netapp/trident/storage_drivers/solidfire"
)

var (
	etcdV3 = flag.String("etcd_v3", "http://127.0.0.1:8001", "etcd server (v3 API)")
)

func init() {
	flag.Parse()
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
}

func TestNewEtcdClientV3(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)
	if p == nil || err != nil {
		t.Errorf("Failed to create a working etcdv3 client in %v!",
			config.PersistentStoreBootstrapTimeout)
	}
}

func TestEtcdv3InvalidClient(t *testing.T) {
	_, err := NewEtcdClientV3("http://127.0.0.1:9999")
	if err != nil && MatchUnavailableClusterErr(err) {
	} else {
		t.Error("Invalid client wasn't caught!")
	}
}

func TestEtcdv3CRUD(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)

	// Testing Create
	err = p.Create("key1", "val1")
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}

	// Testing Read
	var val string
	val, err = p.Read("key1")
	if err != nil {
		t.Error(err.Error())
	}

	// Testing Update
	err = p.Update("key1", "val2")
	if err != nil {
		t.Error(err.Error())
	}
	val, err = p.Read("key1")
	if err != nil {
		t.Error(err.Error())
	} else if val != "val2" {
		t.Error("Update failed!")
	}

	// Testing Delete
	err = p.Delete("key1")
	if err != nil {
		t.Error(err.Error())
	}
}

func TestEtcdv3ReadDeleteKeys(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)

	for i := 1; i <= 5; i++ {
		err = p.Create("/volume/"+"vol"+strconv.Itoa(i), "val"+strconv.Itoa(i))
		if err != nil {
			t.Error(err.Error())
			t.FailNow()
		}
	}

	var keys []string
	keys, err = p.ReadKeys("/volume")
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	if len(keys) != 5 {
		t.Error("Reading all the keys failed!")
	}
	for i := 1; i <= 5; i++ {
		key := "/volume/" + "vol" + strconv.Itoa(i)
		if keys[i-1] != key {
			t.Error("")
		}
	}
	if err = p.DeleteKeys("/volume"); err != nil {
		t.Error("Deleting the keys failed!")
	}
	_, err = p.ReadKeys("/volume")
	if err != nil && MatchKeyNotFoundErr(err) {
	} else {
		t.Error(err.Error())
		t.FailNow()
	}
}

func TestEtcdv3RecursiveReadDelete(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)

	for i := 1; i <= 5; i++ {
		err = p.Set(fmt.Sprintf("/trident_etcd/v%d/volume/vol%d", i, i),
			"val"+strconv.Itoa(i))
		if err != nil {
			t.Error(err.Error())
			t.FailNow()
		}
	}
	for i := 1; i <= 3; i++ {
		err = p.Set(fmt.Sprintf("/trident_etcd/v%d/backend/backend%d", i, i),
			"BACKEND"+strconv.Itoa(i))
		if err != nil {
			t.Error(err.Error())
			t.FailNow()
		}
	}

	var keys []string
	keys, err = p.ReadKeys("/trident_etcd")
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	if len(keys) != 8 {
		t.Error("Reading all the keys failed!")
	}
	keys, err = p.ReadKeys("/trident_etcd/v1")
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	if len(keys) != 2 {
		t.Error("Reading all the keys failed!")
	}

	if err = p.DeleteKeys("/trident_etcd"); err != nil {
		t.Errorf("Failed to delete the keys: %v", err)
	}

	// Validate delete
	_, err = p.ReadKeys("/trident_etcd")
	if err != nil && MatchKeyNotFoundErr(err) {
	} else {
		t.Fatal("Failed to delete the keys: ", err)
	}
}

func TestEtcdv3NonexistentReadKeys(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)
	_, err = p.ReadKeys("/trident_102983471023")
	if err != nil && MatchKeyNotFoundErr(err) {
	} else {
		t.Fatal("Failed to return an error for the nonexistent keys: ", err)
	}
}

func TestEtcdv3NonexistentDeleteKeys(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)
	err = p.DeleteKeys("/trident_102983471023")
	if err != nil && MatchKeyNotFoundErr(err) {
	} else {
		t.Fatal("Failed to return an error for the nonexistent keys: ", err)
	}
}

func TestEtcdv3CRUDFailure(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)
	if err != nil {
		t.Error(err.Error())
	}
	var backend string
	backend, err = p.Read("/backend/nonexistent_server_10.0.0.1")
	if backend != "" || !(err != nil && MatchKeyNotFoundErr(err)) {
		t.Error("Failed to catch an invalid read!")
	}
	if err = p.Update("/backend/nonexistent_server_10.0.0.1", ""); !(err != nil && MatchKeyNotFoundErr(err)) {
		t.Error("Failed to catch an invalid update!")
	}
	if err = p.Delete("/backend/nonexistent_server_10.0.0.1"); !(err != nil && MatchKeyNotFoundErr(err)) {
		t.Error("Failed to catch an invalid delete!")
	}
}

func TestEtcdv3Backend(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)

	// Adding storage backend
	NFSServerConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: drivers.OntapNASStorageDriverName,
		},
		ManagementLIF: "10.0.0.4",
		DataLIF:       "10.0.0.100",
		SVM:           "svm1",
		Username:      "admin",
		Password:      "netapp",
	}
	NFSDriver := ontap.NASStorageDriver{
		Config: NFSServerConfig,
	}
	NFSServer := &storage.Backend{
		Driver: &NFSDriver,
		Name:   "NFS_server_1-" + NFSServerConfig.ManagementLIF,
	}
	err = p.AddBackend(NFSServer)
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}

	// Getting a storage backend
	//var recoveredBackend *storage.BackendPersistent
	var ontapConfig drivers.OntapStorageDriverConfig
	recoveredBackend, err := p.GetBackend(NFSServer.Name)
	if err != nil {
		t.Error(err.Error())
	}
	configJSON, err := recoveredBackend.MarshalConfig()
	if err != nil {
		t.Fatal("Unable to marshal recovered backend config: ", err)
	}
	if err = json.Unmarshal([]byte(configJSON), &ontapConfig); err != nil {
		t.Error("Unable to unmarshal backend into ontap configuration: ", err)
	} else if ontapConfig.SVM != NFSServerConfig.SVM {
		t.Error("Backend state doesn't match!")
	}

	// Updating a storage backend
	NFSServerNewConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: drivers.OntapNASStorageDriverName,
		},
		ManagementLIF: "10.0.0.4",
		DataLIF:       "10.0.0.100",
		SVM:           "svm1",
		Username:      "admin",
		Password:      "NETAPP",
	}
	NFSDriver.Config = NFSServerNewConfig
	err = p.UpdateBackend(NFSServer)
	if err != nil {
		t.Error(err.Error())
	}
	recoveredBackend, err = p.GetBackend(NFSServer.Name)
	if err != nil {
		t.Error(err.Error())
	}
	configJSON, err = recoveredBackend.MarshalConfig()
	if err != nil {
		t.Fatal("Unable to marshal recovered backend config: ", err)
	}
	if err = json.Unmarshal([]byte(configJSON), &ontapConfig); err != nil {
		t.Error("Unable to unmarshal backend into ontap configuration: ", err)
	} else if ontapConfig.SVM != NFSServerConfig.SVM {
		t.Error("Backend state doesn't match!")
	}

	// Deleting a storage backend
	err = p.DeleteBackend(NFSServer)
	if err != nil {
		t.Error(err.Error())
	}
}

func TestEtcdv3Backends(t *testing.T) {
	var backends []*storage.BackendPersistent
	p, err := NewEtcdClientV3(*etcdV3)
	if backends, err = p.GetBackends(); err != nil {
		t.Fatal("Unable to list backends at test start: ", err)
	}
	initialCount := len(backends)

	// Adding storage backends
	for i := 1; i <= 5; i++ {
		NFSServerConfig := drivers.OntapStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				StorageDriverName: drivers.OntapNASStorageDriverName,
			},
			ManagementLIF: "10.0.0." + strconv.Itoa(i),
			DataLIF:       "10.0.0.100",
			SVM:           "svm" + strconv.Itoa(i),
			Username:      "admin",
			Password:      "netapp",
		}
		NFSServer := &storage.Backend{
			Driver: &ontap.NASStorageDriver{
				Config: NFSServerConfig,
			},
			Name: "NFS_server_" + strconv.Itoa(i) + "-" + NFSServerConfig.ManagementLIF,
		}
		err = p.AddBackend(NFSServer)
	}

	// Retrieving all backends
	if backends, err = p.GetBackends(); err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	if len(backends)-initialCount != 5 {
		t.Error("Didn't retrieve all the backends!")
	}

	// Deleting all backends
	if err = p.DeleteBackends(); err != nil {
		t.Error(err.Error())
	}
	if backends, err = p.GetBackends(); err != nil {
		t.Error(err.Error())
	} else if len(backends) != 0 {
		t.Error("Deleting backends failed!")
	}
}

func TestEtcdv3DuplicateBackend(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)

	NFSServerConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: drivers.OntapNASStorageDriverName,
		},
		ManagementLIF: "10.0.0.4",
		DataLIF:       "10.0.0.100",
		SVM:           "svm1",
		Username:      "admin",
		Password:      "netapp",
	}
	NFSServer := &storage.Backend{
		Driver: &ontap.NASStorageDriver{
			Config: NFSServerConfig,
		},
		Name: "NFS_server_1-" + NFSServerConfig.ManagementLIF,
	}
	err = p.AddBackend(NFSServer)
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	err = p.AddBackend(NFSServer)
	if err == nil {
		t.Error("Second Create should have failed!")
	}
	err = p.DeleteBackend(NFSServer)
	if err != nil {
		t.Error(err.Error())
	}
}

func TestEtcdv3Volume(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)

	// Adding a volume
	NFSServerConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: drivers.OntapNASStorageDriverName,
		},
		ManagementLIF: "10.0.0.4",
		DataLIF:       "10.0.0.100",
		SVM:           "svm1",
		Username:      "admin",
		Password:      "netapp",
	}
	NFSServer := &storage.Backend{
		Driver: &ontap.NASStorageDriver{
			Config: NFSServerConfig,
		},
		Name: "NFS_server-" + NFSServerConfig.ManagementLIF,
	}
	vol1Config := storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
		Name:         "vol1",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	vol1 := &storage.Volume{
		Config:  &vol1Config,
		Backend: NFSServer.Name,
		Pool:    storagePool,
	}
	err = p.AddVolume(vol1)
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}

	// Getting a volume
	var recoveredVolume *storage.VolumeExternal
	recoveredVolume, err = p.GetVolume(vol1.Config.Name)
	if err != nil {
		t.Error(err.Error())
	}
	if recoveredVolume.Backend != vol1.Backend || recoveredVolume.Config.Size != vol1.Config.Size {
		t.Error("Recovered volume does not match!")
	}

	// Updating a volume
	vol1Config.Size = "2GB"
	err = p.UpdateVolume(vol1)
	if err != nil {
		t.Error(err.Error())
	}
	recoveredVolume, err = p.GetVolume(vol1.Config.Name)
	if err != nil {
		t.Error(err.Error())
	}
	if recoveredVolume.Config.Size != vol1Config.Size {
		t.Error("Volume update failed!")
	}

	// Deleting a volume
	err = p.DeleteVolume(vol1)
	if err != nil {
		t.Error(err.Error())
	}
}

func TestEtcdv3Volumes(t *testing.T) {
	var (
		newVolumeCount  = 5
		expectedVolumes = newVolumeCount
	)
	p, err := NewEtcdClientV3(*etcdV3)

	// Because we don't reset database state between tests, get an initial
	// count of the existing volumes
	if initialVols, err := p.GetVolumes(); err != nil {
		t.Errorf("Unable to get initial volumes.")
	} else {
		expectedVolumes += len(initialVols)
	}

	// Adding volumes
	NFSServerConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: drivers.OntapNASStorageDriverName,
		},
		ManagementLIF: "10.0.0.4",
		DataLIF:       "10.0.0.100",
		SVM:           "svm1",
		Username:      "admin",
		Password:      "netapp",
	}
	NFSServer := &storage.Backend{
		Driver: &ontap.NASStorageDriver{
			Config: NFSServerConfig,
		},
		Name: "NFS_server-" + NFSServerConfig.ManagementLIF,
	}

	for i := 1; i <= newVolumeCount; i++ {
		volConfig := storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         "vol" + strconv.Itoa(i),
			Size:         strconv.Itoa(i) + "GB",
			Protocol:     config.File,
			StorageClass: "gold",
		}
		vol := &storage.Volume{
			Config:  &volConfig,
			Backend: NFSServer.Name,
		}
		err = p.AddVolume(vol)
		if err != nil {
			t.Error(err.Error())
			t.FailNow()
		}
	}

	// Retrieving all volumes
	var volumes []*storage.VolumeExternal
	if volumes, err = p.GetVolumes(); err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	if len(volumes) != expectedVolumes {
		t.Errorf("Expected %d volumes; retrieved %d", expectedVolumes,
			len(volumes))
	}

	// Deleting all volumes
	if err = p.DeleteVolumes(); err != nil {
		t.Error(err.Error())
	}
	if volumes, err = p.GetVolumes(); err != nil {
		t.Error(err.Error())
	} else if len(volumes) != 0 {
		t.Error("Deleting volumes failed!")
	}
}

func TestEtcdv3VolumeTransactions(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)

	// Adding volume transactions
	for i := 1; i <= 5; i++ {
		volConfig := storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         "vol" + strconv.Itoa(i),
			Size:         strconv.Itoa(i) + "GB",
			Protocol:     config.File,
			StorageClass: "gold",
		}
		volTxn := &VolumeTransaction{
			Config: &volConfig,
			Op:     AddVolume,
		}
		if err = p.AddVolumeTransaction(volTxn); err != nil {
			t.Error(err.Error())
			t.FailNow()
		}
	}

	// Retrieving volume transactions
	volTxns, err := p.GetVolumeTransactions()
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	if len(volTxns) != 5 {
		t.Error("Did n't retrieve all volume transactions!")
	}

	// Getting and Deleting volume transactions
	for _, v := range volTxns {
		txn, err := p.GetExistingVolumeTransaction(v)
		if err != nil {
			t.Errorf("Unable to get existing volume transaction %s:  %v",
				v.Config.Name, err)
		}
		if !reflect.DeepEqual(txn, v) {
			t.Errorf("Got incorrect volume transaction for %s (got %s)",
				v.Config.Name, txn.Config.Name)
		}
		p.DeleteVolumeTransaction(v)
	}
	volTxns, err = p.GetVolumeTransactions()
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	if len(volTxns) != 0 {
		t.Error("Didn't delete all volume transactions!")
	}
}

func TestEtcdv3DuplicateVolumeTransaction(t *testing.T) {
	firstTxn := &VolumeTransaction{
		Config: &storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         "testVol",
			Size:         "1 GB",
			Protocol:     config.File,
			StorageClass: "gold",
		},
		Op: AddVolume,
	}
	secondTxn := &VolumeTransaction{
		Config: &storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         "testVol",
			Size:         "1 GB",
			Protocol:     config.File,
			StorageClass: "silver",
		},
		Op: AddVolume,
	}
	p, err := NewEtcdClientV3(*etcdV3)
	if err != nil {
		t.Fatal("Unable to create persistent store client:  ", err)
	}
	if err = p.AddVolumeTransaction(firstTxn); err != nil {
		t.Error("Unable to add first volume transaction: ", err)
	}
	if err = p.AddVolumeTransaction(secondTxn); err != nil {
		t.Error("Unable to add second volume transaction: ", err)
	}
	volTxns, err := p.GetVolumeTransactions()
	if err != nil {
		t.Fatal("Unable to retrieve volume transactions: ", err)
	}
	if len(volTxns) != 1 {
		t.Errorf("Expected one volume transaction; got %d", len(volTxns))
	} else if volTxns[0].Config.StorageClass != "silver" {
		t.Errorf("Vol transaction not updated.  Expected storage class silver;"+
			" got %s.", volTxns[0].Config.StorageClass)
	}
	for i, volTxn := range volTxns {
		if err = p.DeleteVolumeTransaction(volTxn); err != nil {
			t.Errorf("Unable to delete volume transaction %d:  %v", i+1, err)
		}
	}
}

func TestEtcdv3AddSolidFireBackend(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)
	sfConfig := drivers.SolidfireStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: drivers.SolidfireSANStorageDriverName,
		},
		SolidfireStorageDriverConfigDefaults: drivers.SolidfireStorageDriverConfigDefaults{
			CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
				Size: "1GiB",
			},
		},
		TenantName: "docker",
	}
	sfBackend := &storage.Backend{
		Driver: &solidfire.SANStorageDriver{
			Config: sfConfig,
		},
		Name: "solidfire" + "_10.0.0.9",
	}
	if err = p.AddBackend(sfBackend); err != nil {
		t.Fatal(err.Error())
	}

	var retrievedConfig drivers.SolidfireStorageDriverConfig
	recoveredBackend, err := p.GetBackend(sfBackend.Name)
	if err != nil {
		t.Error(err.Error())
	}
	configJSON, err := recoveredBackend.MarshalConfig()
	if err != nil {
		t.Fatal("Unable to marshal recovered backend config: ", err)
	}
	if err = json.Unmarshal([]byte(configJSON), &retrievedConfig); err != nil {
		t.Error("Unable to unmarshal backend into ontap configuration: ", err)
	} else if retrievedConfig.Size != sfConfig.Size {
		t.Errorf("Backend state doesn't match: %v != %v",
			retrievedConfig.Size, sfConfig.Size)
	}

	if err = p.DeleteBackend(sfBackend); err != nil {
		t.Fatal(err.Error())
	}
}

func TestEtcdv3AddStorageClass(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)
	bronzeConfig := &storageclass.Config{
		Name:            "bronze",
		Attributes:      make(map[string]storageattribute.Request),
		AdditionalPools: make(map[string][]string),
	}
	bronzeConfig.Attributes["media"] = storageattribute.NewStringRequest("hdd")
	bronzeConfig.AdditionalPools["ontapnas_10.0.207.101"] = []string{"aggr1"}
	bronzeConfig.AdditionalPools["ontapsan_10.0.207.103"] = []string{"aggr1"}
	bronzeClass := storageclass.New(bronzeConfig)

	if err := p.AddStorageClass(bronzeClass); err != nil {
		t.Fatal(err.Error())
	}

	retrievedSC, err := p.GetStorageClass(bronzeConfig.Name)
	if err != nil {
		t.Error(err.Error())
	}

	sc := storageclass.NewFromPersistent(retrievedSC)
	// Validating correct retrieval of storage class attributes
	retrievedAttrs := sc.GetAttributes()
	if _, ok := retrievedAttrs["media"]; !ok {
		t.Error("Could not find storage class attribute!")
	}
	if retrievedAttrs["media"].Value().(string) != "hdd" || retrievedAttrs["media"].GetType() != "string" {
		t.Error("Could not retrieve storage class attribute!")
	}

	// Validating correct retrieval of storage pools in a storage class
	backendVCs := sc.GetAdditionalStoragePools()
	for k, v := range bronzeConfig.AdditionalPools {
		if vcs, ok := backendVCs[k]; !ok {
			t.Errorf("Could not find backend %s for the storage class!", k)
		} else {
			if len(vcs) != len(v) {
				t.Errorf("Could not retrieve the correct number of storage pools!")
			} else {
				for i, vc := range vcs {
					if vc != v[i] {
						t.Errorf("Could not find storage pools %s for the storage class!", vc)
					}
				}
			}
		}
	}

	if err := p.DeleteStorageClass(bronzeClass); err != nil {
		t.Fatal(err.Error())
	}
}

func TestEtcdv3ReplaceBackendAndUpdateVolumes(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)

	// Initialize the state by adding a storage backend and volumes
	NFSServerConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: drivers.OntapNASStorageDriverName,
		},
		ManagementLIF: "10.0.0.4",
		DataLIF:       "10.0.0.100",
		SVM:           "svm1",
		Username:      "admin",
		Password:      "netapp",
	}
	NFSDriver := ontap.NASStorageDriver{
		Config: NFSServerConfig,
	}
	NFSServer := &storage.Backend{
		Driver: &NFSDriver,
		Name:   "ontapnas_" + NFSServerConfig.DataLIF,
	}
	err = p.AddBackend(NFSServer)
	if err != nil {
		t.Fatalf("Backend creation failed: %v\n", err)
	}
	for i := 0; i < 5; i++ {
		volConfig := storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         fmt.Sprintf("vol%d", i),
			Size:         "1GB",
			Protocol:     config.File,
			StorageClass: "gold",
		}
		vol := &storage.Volume{
			Config:  &volConfig,
			Backend: NFSServer.Name,
			Pool:    storagePool,
		}
		err = p.AddVolume(vol)
	}

	backends, err := p.GetBackends()
	if err != nil || len(backends) != 1 {
		t.Fatalf("Backend retrieval failed; backends:%v err:%v\n", backends, err)
	}
	log.Debugf("GetBackends: %v, %v\n", backends, err)
	backend, err := p.GetBackend(backends[0].Name)
	if err != nil ||
		backend.Name != NFSServer.Name {
		t.Fatalf("Backend retrieval failed; backend:%v err:%v\n", backend.Name, err)
	}
	log.Debugf("GetBackend(%v): %v, %v\n", backends[0].Name, backend, err)
	volumes, err := p.GetVolumes()
	if err != nil || len(volumes) != 5 {
		t.Fatalf("Volume retrieval failed; volumes:%v err:%v\n", volumes, err)
	}
	log.Debugf("GetVolumes: %v, %v\n", volumes, err)
	for i := 0; i < 5; i++ {
		volume, err := p.GetVolume(fmt.Sprintf("vol%d", i))
		if err != nil ||
			volume.Backend != NFSServer.Name {
			t.Fatalf("Volume retrieval failed; volume:%v err:%v\n", volume, err)
		}
		log.Debugf("GetVolume(vol%v): %v, %v\n", i, volume, err)
	}

	newNFSServer := &storage.Backend{
		Driver: &NFSDriver,
		// Renaming the NFS server
		Name: "AFF",
	}
	err = p.ReplaceBackendAndUpdateVolumes(NFSServer, newNFSServer)
	if err != nil {
		t.Fatalf("ReplaceBackendAndUpdateVolumes failed: %v\n", err)
	}

	// Validate successful renaming of the backend
	backends, err = p.GetBackends()
	if err != nil || len(backends) != 1 {
		t.Fatalf("Backend retrieval failed; backends:%v err:%v\n", backends, err)
	}
	log.Debugf("GetBackends: %v, %v\n", backends, err)
	backend, err = p.GetBackend(backends[0].Name)
	if err != nil ||
		backend.Name != newNFSServer.Name {
		t.Fatalf("Backend retrieval failed; backend:%v err:%v\n", backend.Name, err)
	}
	log.Debugf("GetBackend(%v): %v, %v\n", backends[0].Name, backend, err)

	// Validate successful renaming of the volumes
	volumes, err = p.GetVolumes()
	if err != nil || len(volumes) != 5 {
		t.Fatalf("Volume retrieval failed; volumes:%v err:%v\n", volumes, err)
	}
	log.Debugf("GetVolumes: %v, %v\n", volumes, err)
	for i := 0; i < 5; i++ {
		volume, err := p.GetVolume(fmt.Sprintf("vol%d", i))
		if err != nil ||
			volume.Backend != newNFSServer.Name {
			t.Fatalf("Volume retrieval failed; volume:%v err:%v\n", volume, err)
		}
		log.Debugf("GetVolume(vol%v): %v, %v\n", i, volume, err)
	}

	// Deleting the storage backend
	err = p.DeleteBackend(newNFSServer)
	if err != nil {
		t.Error(err.Error())
	}

	// Deleting all volumes
	if err = p.DeleteVolumes(); err != nil {
		t.Error(err.Error())
	}
}

// TestEtcdv3FailedReplaceBackendAndUpdateVolumes tests that a backend doesn't
// get updated if one of the volume updates fail.
func TestEtcdv3FailedReplaceBackendAndUpdateVolumes(t *testing.T) {
	p, err := NewEtcdClientV3(*etcdV3)

	// Initialize the state by adding a storage backend and volumes
	NFSServerConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: drivers.OntapNASStorageDriverName,
		},
		ManagementLIF: "10.0.0.4",
		DataLIF:       "10.0.0.100",
		SVM:           "svm1",
		Username:      "admin",
		Password:      "netapp",
	}
	NFSDriver := ontap.NASStorageDriver{
		Config: NFSServerConfig,
	}
	NFSServer := &storage.Backend{
		Driver: &NFSDriver,
		Name:   "ontapnas_" + NFSServerConfig.DataLIF,
	}
	err = p.AddBackend(NFSServer)
	if err != nil {
		t.Fatalf("Backend creation failed: %v\n", err)
	}
	for i := 0; i < 5; i++ {
		volConfig := storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         fmt.Sprintf("vol%d", i),
			Size:         "1GB",
			Protocol:     config.File,
			StorageClass: "gold",
		}
		vol := &storage.Volume{
			Config:  &volConfig,
			Backend: NFSServer.Name,
			Pool:    storagePool,
		}
		err = p.AddVolume(vol)
	}

	backends, err := p.GetBackends()
	if err != nil || len(backends) != 1 {
		t.Fatalf("Backend retrieval failed; backends:%v err:%v\n", backends, err)
	}
	backend, err := p.GetBackend(backends[0].Name)
	if err != nil ||
		backend.Name != NFSServer.Name {
		t.Fatalf("Backend retrieval failed; backend:%v err:%v\n", backend.Name, err)
	}
	volumes, err := p.GetVolumes()
	if err != nil || len(volumes) != 5 {
		t.Fatalf("Volume retrieval failed; volumes:%v err:%v\n", volumes, err)
	}
	for i := 0; i < 5; i++ {
		volume, err := p.GetVolume(fmt.Sprintf("vol%d", i))
		if err != nil ||
			volume.Backend != NFSServer.Name {
			t.Fatalf("Volume retrieval failed; volume:%v err:%v\n", volume, err)
		}
	}

	// Renaming the NFS server
	newNFSServer := &storage.Backend{
		Driver: &NFSDriver,
		Name:   "AFF",
	}

	// Testing a failed transaction
	err = p.failedReplaceBackendAndUpdateVolumes(NFSServer, newNFSServer)
	if err == nil {
		t.Fatalf("failedReplaceBackendAndUpdateVolumes should have failed: %v\n", err)
	}

	// Validating the new backend doesn't exist
	backends, err = p.GetBackends()
	if err != nil || len(backends) != 1 {
		t.Fatalf("Backend retrieval failed; backends:%v err:%v\n", backends, err)
	}
	log.Debugf("GetBackends: %v, %v\n", backends, err)
	backend, err = p.GetBackend(backends[0].Name)
	if err != nil ||
		backend.Name != NFSServer.Name {
		t.Fatalf("Backend retrieval failed; backend:%v err:%v\n", backend.Name, err)
	}
	log.Debugf("GetBackend(%v): %v, %v\n", backends[0].Name, backend, err)

	// Validating the volumes haven't been updated
	volumes, err = p.GetVolumes()
	if err != nil || len(volumes) != 5 {
		t.Fatalf("Volume retrieval failed; volumes:%v err:%v\n", volumes, err)
	}
	log.Debugf("GetVolumes: %v, %v\n", volumes, err)
	for i := 0; i < 5; i++ {
		volume, err := p.GetVolume(fmt.Sprintf("vol%d", i))
		if err != nil ||
			volume.Backend != NFSServer.Name {
			t.Fatalf("Volume retrieval failed; volume:%v err:%v\n", volume, err)
		}
		log.Debugf("GetVolume(vol%v): %v, %v\n", i, volume, err)
	}

	// Deleting the storage backend
	err = p.DeleteBackend(NFSServer)
	if err != nil {
		t.Error(err.Error())
	}

	// Deleting all volumes
	if err = p.DeleteVolumes(); err != nil {
		t.Error(err.Error())
	}
}
