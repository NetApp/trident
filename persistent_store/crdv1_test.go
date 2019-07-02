// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	"github.com/netapp/trident/storage"
	storageattribute "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap"
	"github.com/netapp/trident/storage_drivers/solidfire"
	"github.com/netapp/trident/utils"
)

func init() {
	flag.Parse()
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
}

func GetTestKubernetesClient() *CRDClientV1 {
	client := fake.NewSimpleClientset()

	return &CRDClientV1{
		client: client,
		version: &config.PersistentStateVersion{
			PersistentStoreVersion: string(CRDV1Store),
			OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		},
	}
}

func TestKubernetesBackend(t *testing.T) {
	p := GetTestKubernetesClient()

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
		Name:   "nfs-server-1-" + NFSServerConfig.ManagementLIF,
	}

	err := p.AddBackend(NFSServer)
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

func TestKubernetesBackends(t *testing.T) {
	p := GetTestKubernetesClient()

	var backends []*storage.BackendPersistent
	var err error

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
			Name: "nfs-server-" + strconv.Itoa(i) + "-" + NFSServerConfig.ManagementLIF,
		}

		persistentBackend, err := v1.NewTridentBackend(NFSServer.ConstructPersistent())
		persistentBackend.ObjectMeta.Name = NFSServer.Name
		err = p.addBackendCRD(persistentBackend)
		if err != nil {
			t.Error(err.Error())
			t.FailNow()
		}

		log.Info(">>CHECKING BACKENDS.")
		backends, err = p.GetBackends()
		for _, backend := range backends {
			log.WithFields(log.Fields{
				"backend.Name": backend.Name,
				"backend":      backend,
			}).Info("Currently have.")
		}
		log.Info("<<CHECKING BACKENDS.")

		if err != nil {
			t.Error(err.Error())
			t.FailNow()
		}
	}

	// Retrieving all backends
	if backends, err = p.GetBackends(); err != nil {
		t.Error(err.Error())
		t.FailNow()
	}

	if len(backends) != 5 {
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

func TestKubernetesDuplicateBackend(t *testing.T) {
	p := GetTestKubernetesClient()

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
		Name: "nfs-server-1-" + NFSServerConfig.ManagementLIF,
	}

	persistentBackend, err := v1.NewTridentBackend(NFSServer.ConstructPersistent())
	persistentBackend.ObjectMeta.Name = NFSServer.Name
	err = p.addBackendCRD(persistentBackend)
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	err = p.addBackendCRD(persistentBackend)
	if err == nil {
		t.Error("Second Create should have failed!")
	}
	err = p.DeleteBackend(NFSServer)
	if err != nil {
		t.Error(err.Error())
	}
}

func TestKubernetesVolume(t *testing.T) {
	p := GetTestKubernetesClient()

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
	NFSServer.BackendUUID = uuid.New().String()
	vol1Config := storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
		Name:         "vol1",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	vol1 := &storage.Volume{
		Config:      &vol1Config,
		BackendUUID: NFSServer.BackendUUID,
		Pool:        storagePool,
	}
	err := p.AddVolume(vol1)
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
	if recoveredVolume.BackendUUID != vol1.BackendUUID || recoveredVolume.Config.Size != vol1.Config.Size {
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

func TestKubernetesVolumes(t *testing.T) {
	var err error

	p := GetTestKubernetesClient()

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
	NFSServer.BackendUUID = uuid.New().String()

	for i := 1; i <= 5; i++ {
		volConfig := storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         "vol" + strconv.Itoa(i),
			Size:         strconv.Itoa(i) + "GB",
			Protocol:     config.File,
			StorageClass: "gold",
		}
		vol := &storage.Volume{
			Config:      &volConfig,
			BackendUUID: NFSServer.BackendUUID,
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
	if len(volumes) != 5 {
		t.Errorf("Expected %d volumes; retrieved %d", 5,
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

func TestKubernetesVolumeTransactions(t *testing.T) {
	var err error

	p := GetTestKubernetesClient()

	// Adding volume transactions
	for i := 1; i <= 5; i++ {
		volConfig := storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         "vol" + strconv.Itoa(i),
			Size:         strconv.Itoa(i) + "GB",
			Protocol:     config.File,
			StorageClass: "gold",
		}
		volTxn := &storage.VolumeTransaction{
			Config: &volConfig,
			Op:     storage.AddVolume,
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

func TestKubernetesDuplicateVolumeTransaction(t *testing.T) {
	var err error

	p := GetTestKubernetesClient()

	firstTxn := &storage.VolumeTransaction{
		Config: &storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         "testVol",
			Size:         "1 GB",
			Protocol:     config.File,
			StorageClass: "gold",
		},
		Op: storage.AddVolume,
	}
	secondTxn := &storage.VolumeTransaction{
		Config: &storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         "testVol",
			Size:         "1 GB",
			Protocol:     config.File,
			StorageClass: "silver",
		},
		Op: storage.AddVolume,
	}

	if err = p.AddVolumeTransaction(firstTxn); err != nil {
		t.Error("Unable to add first volume transaction: ", err)
	}
	if err = p.AddVolumeTransaction(secondTxn); err == nil {
		t.Error("Should not have been able to add second volume transaction: ", err)
	}
	volTxns, err := p.GetVolumeTransactions()
	if err != nil {
		t.Fatal("Unable to retrieve volume transactions: ", err)
	}
	if len(volTxns) != 1 {
		t.Errorf("Expected one volume transaction; got %d", len(volTxns))
	} else if volTxns[0].Config.StorageClass != "gold" {
		t.Errorf("Vol transaction changed.  Expected storage class gold;"+
			" got %s.", volTxns[0].Config.StorageClass)
	}
	for i, volTxn := range volTxns {
		if err = p.DeleteVolumeTransaction(volTxn); err != nil {
			t.Errorf("Unable to delete volume transaction %d:  %v", i+1, err)
		}
	}
}

func TestKubernetesAddSolidFireBackend(t *testing.T) {
	var err error

	p := GetTestKubernetesClient()

	sfConfig := drivers.SolidfireStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: drivers.SolidfireSANStorageDriverName,
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

func TestKubernetesAddStorageClass(t *testing.T) {
	var err error

	p := GetTestKubernetesClient()

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

func TestKubernetesReplaceBackendAndUpdateVolumes(t *testing.T) {
	var err error

	p := GetTestKubernetesClient()

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

	recoveredBackend, err := p.GetBackend(NFSServer.Name)
	if err != nil {
		t.Error(err.Error())
		t.Fatalf("Backend lookup failed for:%v err: %v\n", recoveredBackend, err)
	}
	log.WithFields(log.Fields{
		"recoveredBackend.Name":        recoveredBackend.Name,
		"recoveredBackend.BackendUUID": recoveredBackend.BackendUUID,
	}).Debug("recoveredBackend")

	for i := 0; i < 5; i++ {
		volConfig := storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         fmt.Sprintf("vol%d", i),
			Size:         "1GB",
			Protocol:     config.File,
			StorageClass: "gold",
		}
		vol := &storage.Volume{
			Config:      &volConfig,
			BackendUUID: recoveredBackend.BackendUUID,
			Pool:        storagePool,
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
		log.WithFields(log.Fields{
			"volume": volume,
		}).Debug("GetVolumes.")
		if err != nil {
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
		t.Fatalf("Backend retrieval failed; backend.Name:%v newNFSServer.Name:%v err:%v\n", backend.Name, newNFSServer.Name, err)
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
		log.WithFields(log.Fields{
			"volume":                   volume,
			"volume.BackendUUID":       volume.BackendUUID,
			"newNFSServer.Name":        newNFSServer.Name,
			"newNFSServer.BackendUUID": newNFSServer.BackendUUID,
			"NFSServer.Name":           newNFSServer.Name,
			"NFSServer.BackendUUID":    NFSServer.BackendUUID,
		}).Debug("GetVolumes.")

		if err != nil || volume.BackendUUID != recoveredBackend.BackendUUID {
			t.Fatalf("Volume retrieval failed; volume:%v err:%v\n",
				volume, err)
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

func TestKubernetesAddOrUpdateNode(t *testing.T) {
	p := GetTestKubernetesClient()

	utilsNode := &utils.Node{
		Name: "test",
		IQN:  "iqn",
		IPs: []string{
			"192.168.0.1",
		},
	}

	// should not exist
	_, err := p.GetNode(utilsNode.Name)
	if !IsStatusNotFoundError(err) {
		t.Fatal(err.Error())
	}

	// should be added
	if err := p.AddOrUpdateNode(utilsNode); err != nil {
		t.Fatal(err.Error())
	}

	// validate we can find what we added
	node, err := p.GetNode(utilsNode.Name)
	if err != nil {
		t.Fatal(err.Error())
	}

	if node.Name != utilsNode.Name {
		t.Fatalf("%v differs:  '%v' != '%v'", "Name", node.Name, utilsNode.Name)
	}

	if node.IQN != utilsNode.IQN {
		t.Fatalf("%v differs:  '%v' != '%v'", "IQN", node.IQN, utilsNode.IQN)
	}

	if len(node.IPs) != len(utilsNode.IPs) {
		t.Fatalf("%v differs:  '%v' != '%v'", "IPs", node.IPs, utilsNode.IPs)
	}

	for i, _ := range node.IPs {
		if node.IPs[i] != utilsNode.IPs[i] {
			t.Fatalf("%v differs:  '%v' != '%v'", "IPs", node.IPs, utilsNode.IPs)
		}
	}

	// update it
	utilsNode.IQN = "iqnUpdated"
	if err := p.AddOrUpdateNode(utilsNode); err != nil {
		t.Fatal(err.Error())
	}
	node, err = p.GetNode(utilsNode.Name)
	if err != nil {
		t.Fatal(err.Error())
	}
	if node.IQN != "iqnUpdated" {
		t.Fatalf("%v differs:  '%v' != '%v'", "IQN", node.IQN, "iqnUpdated")
	}

	// remove it
	if err := p.DeleteNode(utilsNode); err != nil {
		t.Fatal(err.Error())
	}

	// validate it's gone
	_, err = p.GetNode(utilsNode.Name)
	if !IsStatusNotFoundError(err) {
		t.Fatal(err.Error())
	}
}

func TestKubernetesSnapshot(t *testing.T) {
	p := GetTestKubernetesClient()

	// Adding a snapshot
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
		Name:        "NFS_server-" + NFSServerConfig.ManagementLIF,
		BackendUUID: uuid.New().String(),
	}
	vol1Config := storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
		Name:         "vol1",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	vol1 := &storage.Volume{
		Config:      &vol1Config,
		BackendUUID: NFSServer.BackendUUID,
		Pool:        storagePool,
	}
	err := p.AddVolume(vol1)
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	snap1Config := &storage.SnapshotConfig{
		Version:            "1",
		Name:               "testsnap1",
		InternalName:       "internal_testsnap1",
		VolumeName:         "vol1",
		VolumeInternalName: "internal_vol1",
	}
	now := time.Now().UTC().Format(storage.SnapshotNameFormat)
	size := int64(1000000000)
	snap1 := storage.NewSnapshot(snap1Config, now, size)
	err = p.AddSnapshot(snap1)
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}

	// Getting a snapshot
	recoveredSnapshot, err := p.GetSnapshot(snap1.Config.VolumeName, snap1.Config.Name)
	if err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	if recoveredSnapshot.SizeBytes != snap1.SizeBytes || recoveredSnapshot.Created != snap1.Created ||
		!reflect.DeepEqual(recoveredSnapshot.Config, snap1.Config) {
		t.Error("Recovered snapshot does not match!")
	}

	// Deleting a snapshot
	err = p.DeleteSnapshot(snap1)
	if err != nil {
		t.Error(err.Error())
	}

	_, err = p.GetSnapshot(snap1.Config.VolumeName, snap1.Config.Name)
	if err == nil {
		t.Error("Snapshot should have been deleted.")
		t.FailNow()
	}

	// Deleting a non-existent snapshot
	err = p.DeleteSnapshot(snap1)
	if err == nil {
		t.Error("DeleteSnapshot should have failed.")
	}
	err = p.DeleteSnapshotIgnoreNotFound(snap1)
	if err != nil {
		t.Error("DeleteSnapshotIgnoreNotFound should have succeeded.")
	}
}

func TestKubernetesSnapshots(t *testing.T) {
	var err error

	p := GetTestKubernetesClient()

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
		Name:        "NFS_server-" + NFSServerConfig.ManagementLIF,
		BackendUUID: uuid.New().String(),
	}

	for i := 1; i <= 5; i++ {
		volConfig := storage.VolumeConfig{
			Version:      string(config.OrchestratorAPIVersion),
			Name:         "vol" + strconv.Itoa(i),
			Size:         strconv.Itoa(i) + "GB",
			Protocol:     config.File,
			StorageClass: "gold",
		}
		vol := &storage.Volume{
			Config:      &volConfig,
			BackendUUID: NFSServer.BackendUUID,
		}
		err = p.AddVolume(vol)
		if err != nil {
			t.Error(err.Error())
			t.FailNow()
		}
		snapConfig := &storage.SnapshotConfig{
			Version:            "1",
			Name:               "snap" + strconv.Itoa(i),
			InternalName:       "internal_snap1" + strconv.Itoa(i),
			VolumeName:         "vol" + strconv.Itoa(i),
			VolumeInternalName: "internal_vol" + strconv.Itoa(i),
		}
		now := time.Now().UTC().Format(storage.SnapshotNameFormat)
		size := int64(1000000000)
		snap := storage.NewSnapshot(snapConfig, now, size)
		err = p.AddSnapshot(snap)
		if err != nil {
			t.Error(err.Error())
			t.FailNow()
		}
	}

	// Retrieving all snapshots
	var snapshots []*storage.SnapshotPersistent
	if snapshots, err = p.GetSnapshots(); err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
	if len(snapshots) != 5 {
		t.Errorf("Expected %d snapshots; retrieved %d", 5, len(snapshots))
	}

	// Deleting all snapshots
	if err = p.DeleteSnapshots(); err != nil {
		t.Error(err.Error())
	}
	if snapshots, err = p.GetSnapshots(); err != nil {
		t.Error(err.Error())
	} else if len(snapshots) != 0 {
		t.Error("Deleting snapshots failed!")
	}
}

/*
func TestBackend_RemoveFinalizers(t *testing.T) {

	var err error

	p := GetTestKubernetesClient()

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

	backendList, err := p.client.TridentV1().TridentBackends(p.namespace).List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Backend list failed: %v\n", err)
	}

	// Check the Finalizers exist, removing them as we go
	for _, item := range backendList.Items {
		log.WithFields(log.Fields{
			"Name":    item.Name,
			"ObjectMeta.Finalizers": item.ObjectMeta.Finalizers,
		}).Debug("Found Item")

		if !utils.SliceContainsString(item.ObjectMeta.Finalizers, v1.TridentFinalizerName) {
			t.Fatalf("Expected to find Trident finalizer %v in list: %v\n", v1.TridentFinalizerName, item.ObjectMeta.Finalizers)
		}

		// remove our finalizer and update
		item.ObjectMeta.Finalizers = utils.RemoveStringFromSlice(item.ObjectMeta.Finalizers, v1.TridentFinalizerName)
		_, err = p.client.TridentV1().TridentBackends(p.namespace).Update(item)
	}

	// Now, spin back through, validating the Finalizers were removed
	backendList, err = p.client.TridentV1().TridentBackends(p.namespace).List(metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Backend list failed: %v\n", err)
	}
	for _, item := range backendList.Items {
		log.WithFields(log.Fields{
			"Name":    item.Name,
			"ObjectMeta.Finalizers": item.ObjectMeta.Finalizers,
		}).Debug("Found Item")

		if utils.SliceContainsString(item.ObjectMeta.Finalizers, v1.TridentFinalizerName) {
			t.Fatalf("Did NOT expect to find Trident finalizer %v in list: %v\n", v1.TridentFinalizerName, item.ObjectMeta.Finalizers)
		}
	}
}
*/
