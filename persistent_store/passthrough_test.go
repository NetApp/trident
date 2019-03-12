// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	sc "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	fakedriver "github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/utils"
)

func getFakePools(count int) map[string]*fake.StoragePool {

	ret := make(map[string]*fake.StoragePool, count)

	for i := 0; i < count; i++ {
		ret[fmt.Sprintf("pool-%d", i)] = &fake.StoragePool{
			Bytes: 100 * 1024 * 1024 * 1024,
			Attrs: map[string]sa.Offer{
				sa.IOPS:             sa.NewIntOffer(0, 100),
				sa.Snapshots:        sa.NewBoolOffer(false),
				sa.Encryption:       sa.NewBoolOffer(false),
				sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
			},
		}
	}

	return ret
}

func getFakeBackend() *storage.Backend {
	return getFakeBackendWithName("fake_backend")
}

func getFakeBackendWithName(name string) *storage.Backend {

	fakeConfig := drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           drivers.ConfigVersion,
			StorageDriverName: drivers.FakeStorageDriverName,
		},
		Protocol:     config.File,
		Pools:        getFakePools(2),
		InstanceName: name,
	}

	fakeStorageDriver := fakedriver.NewFakeStorageDriver(fakeConfig)
	fakeBackend, _ := storage.NewStorageBackend(fakeStorageDriver)
	return fakeBackend
}

func getFakeVolume(fakeBackend *storage.Backend) *storage.Volume {
	return getFakeVolumeWithName("fake_volume", fakeBackend)
}

func getFakeVolumeWithName(name string, fakeBackend *storage.Backend) *storage.Volume {

	volumeConfig := &storage.VolumeConfig{
		Name:         name,
		InternalName: name + "_internal",
	}

	return storage.NewVolume(volumeConfig, fakeBackend.BackendUUID,
		fakeBackend.Storage["pool-0"].Name, false)
}

func getFakeVolumeTransaction() *VolumeTransaction {
	return getFakeVolumeTransactionWithName("fake_volume")
}

func getFakeVolumeTransactionWithName(name string) *VolumeTransaction {

	volumeConfig := &storage.VolumeConfig{
		Name:         name,
		InternalName: name + "_internal",
	}

	return &VolumeTransaction{Config: volumeConfig, Op: AddVolume}
}

func getFakeStorageClass() *sc.StorageClass {
	return getFakeStorageClassWithName("fake_sc", 40, true, "thin")
}

func getFakeStorageClassWithName(name string, iops int, snapshots bool, provType string) *sc.StorageClass {

	scConfig := &sc.Config{
		Version: config.OrchestratorAPIVersion,
		Name:    name,
		Attributes: map[string]sa.Request{
			sa.IOPS:             sa.NewIntRequest(iops),
			sa.Snapshots:        sa.NewBoolRequest(snapshots),
			sa.ProvisioningType: sa.NewStringRequest(provType),
		},
	}
	return sc.New(scConfig)
}

func getFakeNode() *utils.Node {
	return &utils.Node{
		Name: "testNode",
		IQN:  "myIQN",
		IPs:  []string{"1.1.1.1", "2.2.2.2"},
	}
}

func getFakeSnapshot() *storage.Snapshot {
	snapConfig := &storage.SnapshotConfig{
		Version:            config.OrchestratorAPIVersion,
		Name:               "snap1",
		InternalName:       "snap1",
		VolumeName:         "vol1",
		VolumeInternalName: "trident_vol1",
	}
	return &storage.Snapshot{
		Config:    snapConfig,
		Created:   time.Now().UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: 1000000000,
	}
}

func newPassthroughClient() *PassthroughClient {
	return &PassthroughClient{
		bootBackends: make([]*storage.BackendPersistent, 0),
		liveBackends: make(map[string]*storage.Backend),
		version: &config.PersistentStateVersion{
			PersistentStoreVersion: "passthrough",
			OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		},
	}
}

func TestPassthroughClient_NewPassthroughClientSingleFile(t *testing.T) {

	configPath := "/tmp/fake_backend"
	backendJSON, _ := getFakeBackend().ConstructPersistent().MarshalConfig()
	err := ioutil.WriteFile(configPath, []byte(backendJSON), 0644)
	if err != nil {
		t.Error(err.Error())
	}

	p, err := NewPassthroughClient(configPath)

	if p == nil || err != nil {
		t.Errorf("Failed to create a working passthrough client! %v", err)
	}
	if len(p.bootBackends) != 1 {
		t.Error("Passthrough client failed to initialize one backend!")
	}
	if len(p.liveBackends) != 0 {
		t.Error("Passthrough client should not have created a live backend!")
	}
	os.Remove(configPath)
}

func TestPassthroughClient_NewPassthroughClientDirectory(t *testing.T) {

	configPath := "/tmp/fake_backends"
	os.MkdirAll(configPath, 0755)
	backend1JSON, _ := getFakeBackend().ConstructPersistent().MarshalConfig()
	err := ioutil.WriteFile(configPath+"/backend1", []byte(backend1JSON), 0644)
	if err != nil {
		t.Error(err.Error())
	}
	backend2JSON, _ := getFakeBackend().ConstructPersistent().MarshalConfig()
	err = ioutil.WriteFile(configPath+"/backend2", []byte(backend2JSON), 0644)
	if err != nil {
		t.Error(err.Error())
	}

	p, err := NewPassthroughClient(configPath)

	if p == nil || err != nil {
		t.Errorf("Failed to create a working passthrough client! %v", err)
	}
	if len(p.bootBackends) != 2 {
		t.Error("Passthrough client failed to initialize two backends!")
	}
	if len(p.liveBackends) != 0 {
		t.Error("Passthrough client should not have created a live backend!")
	}
	os.Remove(configPath + "/backend1")
	os.Remove(configPath + "/backend2")
	os.Remove(configPath)
}

func TestPassthroughClient_GetType(t *testing.T) {
	p := newPassthroughClient()

	storeType := p.GetType()

	if storeType != PassthroughStore {
		t.Error("Passthrough client returned the wrong type!")
	}
}

func TestPassthroughClient_Stop(t *testing.T) {
	p := newPassthroughClient()
	p.AddBackend(getFakeBackend())

	if len(p.liveBackends) == 0 {
		t.Error("Could not add backend to passthrough client!")
	}

	p.Stop()

	if len(p.liveBackends) != 0 {
		t.Error("Passthrough client failed to stop!")
	}
}

func TestPassthroughClient_GetConfig(t *testing.T) {
	p := newPassthroughClient()

	result := p.GetConfig()

	expected := &ClientConfig{}
	if *result != *expected {
		t.Error("Passthrough client returned unexpected config!")
	}
}

func TestPassthroughClient_GetVersion(t *testing.T) {
	p := newPassthroughClient()

	result, _ := p.GetVersion()

	expected := &config.PersistentStateVersion{
		"passthrough",
		config.OrchestratorAPIVersion,
	}
	if *result != *expected {
		t.Error("Passthrough client returned unexpected version!")
	}
}

func TestPassthroughClient_SetVersion(t *testing.T) {
	p := newPassthroughClient()

	p.SetVersion(&config.PersistentStateVersion{"invalid", "unknown"})

	result, _ := p.GetVersion()
	expected := &config.PersistentStateVersion{
		"passthrough",
		config.OrchestratorAPIVersion,
	}
	if *result != *expected {
		t.Error("Passthrough client returned unexpected version!")
	}
}

func TestPassthroughClient_AddBackend(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()

	err := p.AddBackend(fakeBackend)

	if err != nil {
		t.Error("Could not add backend to passthrough client!")
	}
	if len(p.liveBackends) != 1 {
		t.Error("Could not add backend to passthrough client!")
	}

	p.AddBackend(fakeBackend)

	if len(p.liveBackends) != 1 {
		t.Error("Added backend a second time to passthrough client!")
	}
}

func TestPassthroughClient_GetBackend(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	err := p.AddBackend(fakeBackend)

	result, err := p.GetBackend(fakeBackend.Name)

	if err != nil {
		t.Error("Could not get backend from passthrough client!")
	}
	if result.Name != fakeBackend.Name {
		t.Error("Got incorrect backend from passthrough client!")
	}
}

func TestPassthroughClient_GetBackendNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()

	result, err := p.GetBackend(fakeBackend.Name)

	if result != nil || err == nil {
		t.Error("Got incorrect backend from passthrough client!")
	}
}

func TestPassthroughClient_UpdateBackend(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeBackend.State = storage.Deleting

	err := p.UpdateBackend(fakeBackend)

	if err != nil {
		t.Error("Could not update backend on passthrough client!")
	}
	result, err := p.GetBackend(fakeBackend.Name)
	if err != nil {
		t.Error("Could not get backend from passthrough client!")
	}
	if !result.State.IsDeleting() {
		t.Error("Backend not updated on passthrough client!")
	}
}

func TestPassthroughClient_UpdateBackendNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()

	err := p.UpdateBackend(fakeBackend)

	if err == nil {
		t.Error("Backend update should have failed on passthrough client!")
	}
}

func TestPassthroughClient_DeleteBackend(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	if len(p.liveBackends) != 1 {
		t.Error("Could not add backend to passthrough client!")
	}

	err := p.DeleteBackend(fakeBackend)

	if err != nil {
		t.Error("Could not delete backend on passthrough client!")
	}
	if len(p.liveBackends) != 0 {
		t.Error("Could not delete backend on passthrough client!")
	}
}

func TestPassthroughClient_DeleteBackendNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()

	err := p.DeleteBackend(fakeBackend)

	if err == nil {
		t.Error("Backend delete should have failed on passthrough client!")
	}
}

func TestPassthroughClient_GetBackends(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend1 := getFakeBackend()
	fakeBackend1.Name = "fake1"
	p.bootBackends = append(p.bootBackends, fakeBackend1.ConstructPersistent())
	fakeBackend2 := getFakeBackend()
	fakeBackend2.Name = "fake2"
	p.bootBackends = append(p.bootBackends, fakeBackend2.ConstructPersistent())

	result, err := p.GetBackends()

	if err != nil {
		t.Error("Could not get backends from passthrough client!")
	}
	if len(result) != 2 {
		t.Error("Got wrong number of boot backends from passthrough client!")
	}
	if len(p.liveBackends) != 0 {
		t.Error("Got wrong number of live backends from passthrough client!")
	}
}

func TestPassthroughClient_GetBackendsNonexistent(t *testing.T) {
	p := newPassthroughClient()
	p.AddBackend(getFakeBackend())

	result, err := p.GetBackends()

	if err != nil {
		t.Error("Could not get backends from passthrough client!")
	}
	if len(result) != 0 {
		t.Error("Got wrong number of backends from passthrough client!")
	}
}

func TestPassthroughClient_DeleteBackends(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend1 := getFakeBackend()
	fakeBackend1.Name = "fake1"
	p.AddBackend(fakeBackend1)
	p.bootBackends = append(p.bootBackends, fakeBackend1.ConstructPersistent())
	fakeBackend2 := getFakeBackend()
	fakeBackend2.Name = "fake2"
	p.AddBackend(fakeBackend2)
	p.bootBackends = append(p.bootBackends, fakeBackend2.ConstructPersistent())

	err := p.DeleteBackends()

	if err != nil {
		t.Error("Could not delete backends from passthrough client!")
	}
	if len(p.liveBackends) != 0 {
		t.Error("Could not delete live backends from passthrough client!")
	}
	if len(p.bootBackends) != 2 {
		t.Error("Got wrong number of boot backends from passthrough client!")
	}
}

func TestPassthroughClient_DeleteBackendsNonexistent(t *testing.T) {
	p := newPassthroughClient()

	err := p.DeleteBackends()

	if err != nil {
		t.Error("Could not delete backends from passthrough client!")
	}
	backends, _ := p.GetBackends()
	if len(backends) != 0 {
		t.Error("Could not delete backends from passthrough client!")
	}
}

func TestPassthroughClient_AddVolume(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeVolume := getFakeVolume(fakeBackend)

	err := p.AddVolume(fakeVolume)

	if err != nil {
		t.Error("Could not add volume to passthrough client!")
	}
}

func TestPassthroughClient_GetVolume(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeVolume := getFakeVolume(fakeBackend)
	p.AddVolume(fakeVolume)

	vol, err := p.GetVolume("fake_volume")

	if vol != nil || err == nil {
		t.Error("Get volume should have failed on passthrough client!")
	}
}

func TestPassthroughClient_GetVolumeNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)

	vol, err := p.GetVolume("fake_volume")

	if vol != nil || err == nil {
		t.Error("Get volume should have failed on passthrough client!")
	}
}

func TestPassthroughClient_UpdateVolume(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeVolume := getFakeVolume(fakeBackend)
	p.AddVolume(fakeVolume)

	err := p.UpdateVolume(fakeVolume)

	if err != nil {
		t.Error("Could not update volume on passthrough client!")
	}
}

func TestPassthroughClient_UpdateVolumeNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeVolume := getFakeVolume(fakeBackend)

	err := p.UpdateVolume(fakeVolume)

	if err != nil {
		t.Error("Could not update volume on passthrough client!")
	}
}

func TestPassthroughClient_DeleteVolume(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeVolume := getFakeVolume(fakeBackend)
	p.AddVolume(fakeVolume)

	err := p.DeleteVolume(fakeVolume)

	if err != nil {
		t.Error("Could not delete volume from passthrough client!")
	}
}

func TestPassthroughClient_DeleteVolumeNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeVolume := getFakeVolume(fakeBackend)

	err := p.DeleteVolume(fakeVolume)

	if err != nil {
		t.Error("Could not delete volume on passthrough client!")
	}
}

func TestPassthroughClient_DeleteVolumeIgnoreNotFound(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeVolume := getFakeVolume(fakeBackend)
	p.AddVolume(fakeVolume)

	err := p.DeleteVolumeIgnoreNotFound(fakeVolume)

	if err != nil {
		t.Error("Could not delete volume from passthrough client!")
	}
}

func TestPassthroughClient_DeleteVolumeIgnoreNotFoundNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeVolume := getFakeVolume(fakeBackend)

	err := p.DeleteVolumeIgnoreNotFound(fakeVolume)

	if err != nil {
		t.Error("Could not delete volume on passthrough client!")
	}
}

func TestPassthroughClient_GetVolumes(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()

	volConfig := &storage.VolumeConfig{
		Name:         "fake_volume_1",
		InternalName: "fake_volume_1",
		Size:         "1000000000",
	}
	err := fakeBackend.Driver.Create(volConfig, fakeBackend.Storage["pool-0"], make(map[string]sa.Request))
	if err != nil {
		t.Error(err)
	}

	volConfig = &storage.VolumeConfig{
		Name:         "fake_volume_2",
		InternalName: "fake_volume_2",
		Size:         "2000000000",
	}
	err = fakeBackend.Driver.Create(volConfig, fakeBackend.Storage["pool-0"], make(map[string]sa.Request))
	if err != nil {
		t.Error(err)
	}

	p.AddBackend(fakeBackend)

	result, err := p.GetVolumes()
	if err != nil {
		t.Error("Could not get volumes from passthrough client!")
	}
	volMap := make(map[string]*storage.VolumeExternal)
	for _, vol := range result {
		volMap[vol.Config.Name] = vol
	}
	if _, ok := volMap["fake_volume_1"]; !ok {
		t.Error("Could not get fake_volume_1 from passthrough client!")
	}
	if _, ok := volMap["fake_volume_2"]; !ok {
		t.Error("Could not get fake_volume_2 from passthrough client!")
	}
	if len(volMap) != 2 {
		t.Error("Got wrong number of volumes from passthrough client!")
	}
}

func TestPassthroughClient_GetVolumesNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)

	result, err := p.GetVolumes()

	if err != nil {
		t.Error("Could not get volumes from passthrough client!")
	}
	if len(result) != 0 {
		t.Error("Got wrong number of volumes from passthrough client!")
	}
}

func TestPassthroughClient_DeleteVolumes(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeVolume := getFakeVolume(fakeBackend)
	p.AddVolume(fakeVolume)

	err := p.DeleteVolumes()

	if err != nil {
		t.Error("Could not delete volumes on passthrough client!")
	}
}

func TestPassthroughClient_DeleteVolumesNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)

	err := p.DeleteVolumes()

	if err != nil {
		t.Error("Could not delete volumes on passthrough client!")
	}
}

func TestPassthroughClient_AddVolumeTransaction(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeTransaction := getFakeVolumeTransaction()

	err := p.AddVolumeTransaction(fakeTransaction)

	if err != nil {
		t.Error("Could not add volume transaction to passthrough client!")
	}
}

func TestPassthroughClient_GetVolumeTransactions(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeTransaction := getFakeVolumeTransaction()
	p.AddVolumeTransaction(fakeTransaction)

	result, err := p.GetVolumeTransactions()

	if err != nil {
		t.Error("Could not get volume transactions from passthrough client!")
	}
	if len(result) != 0 {
		t.Error("Did not expect to get volume transaction from passthrough client!")
	}
}

func TestPassthroughClient_GetExistingVolumeTransaction(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeTransaction := getFakeVolumeTransaction()
	p.AddVolumeTransaction(fakeTransaction)

	result, err := p.GetExistingVolumeTransaction(fakeTransaction)

	if err != nil {
		t.Error("Could not get volume transaction from passthrough client!")
	}
	if result != nil {
		t.Error("Did not expect to get volume transaction from passthrough client!")
	}
}

func TestPassthroughClient_DeleteVolumeTransaction(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeTransaction := getFakeVolumeTransaction()
	p.AddVolumeTransaction(fakeTransaction)

	err := p.DeleteVolumeTransaction(fakeTransaction)

	if err != nil {
		t.Error("Could not delete volume transaction from passthrough client!")
	}
}

func TestPassthroughClient_AddStorageClass(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeSC := getFakeStorageClass()

	err := p.AddStorageClass(fakeSC)

	if err != nil {
		t.Error("Could not add storage class to passthrough client!")
	}
}

func TestPassthroughClient_GetStorageClass(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeSC := getFakeStorageClass()
	p.AddStorageClass(fakeSC)

	result, err := p.GetStorageClass(fakeSC.GetName())

	if result != nil || err == nil {
		t.Error("Did not expect to get storage class from passthrough client!")
	}
}

func TestPassthroughClient_GetStorageClasses(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeSC := getFakeStorageClass()
	p.AddStorageClass(fakeSC)

	result, err := p.GetStorageClasses()

	if err != nil {
		t.Error("Could not get storage classes from passthrough client!")
	}
	if len(result) != 0 {
		t.Error("Did not expect to get storage classes from passthrough client!")
	}
}

func TestPassthroughClient_DeleteStorageClass(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	p.AddBackend(fakeBackend)
	fakeSC := getFakeStorageClass()
	p.AddStorageClass(fakeSC)

	err := p.DeleteStorageClass(fakeSC)

	if err != nil {
		t.Error("Could not delete storage class from passthrough client!")
	}
}

func TestPassthroughClient_AddNode(t *testing.T) {
	p := newPassthroughClient()
	fakeNode := getFakeNode()

	err := p.AddOrUpdateNode(fakeNode)

	if err != nil {
		t.Error("Could not add node to passthrough client!")
	}
}

func TestPassthroughClient_GetNode(t *testing.T) {
	p := newPassthroughClient()
	fakeNode := getFakeNode()
	p.AddOrUpdateNode(fakeNode)

	result, err := p.GetNode(fakeNode.Name)

	if result != nil || err == nil {
		t.Error("Did not expect to get node from passthrough client!")
	}
}

func TestPassthroughClient_GetNodes(t *testing.T) {
	p := newPassthroughClient()
	fakeNode := getFakeNode()
	p.AddOrUpdateNode(fakeNode)

	result, err := p.GetNodes()

	if err != nil {
		t.Error("Could not get nodes from passthrough client!")
	}
	if len(result) != 0 {
		t.Error("Did not expect to get nodes from passthrough client!")
	}
}

func TestPassthroughClient_DeleteNode(t *testing.T) {
	p := newPassthroughClient()
	fakeNode := getFakeNode()
	p.AddOrUpdateNode(fakeNode)

	err := p.DeleteNode(fakeNode)

	if err != nil {
		t.Error("Could not delete node from passthrough client!")
	}
}

func TestPassthroughClient_AddSnapshot(t *testing.T) {
	p := newPassthroughClient()
	fakeSnapshot := getFakeSnapshot()

	err := p.AddSnapshot(fakeSnapshot)

	if err != nil {
		t.Error("Could not add snapshot to passthrough client!")
	}
}

func TestPassthroughClient_GetSnapshot(t *testing.T) {
	p := newPassthroughClient()
	fakeSnapshot := getFakeSnapshot()
	_ = p.AddSnapshot(fakeSnapshot)

	result, err := p.GetSnapshot(fakeSnapshot.Config.VolumeName, fakeSnapshot.Config.Name)

	if result != nil || err == nil {
		t.Error("Did not expect to get snapshot from passthrough client!")
	}
}

func TestPassthroughClient_GetSnapshots(t *testing.T) {
	p := newPassthroughClient()
	fakeSnapshot := getFakeSnapshot()
	_ = p.AddSnapshot(fakeSnapshot)

	result, err := p.GetSnapshots()

	if err != nil {
		t.Error("Could not get snapshots from passthrough client!")
	}
	if len(result) != 0 {
		t.Error("Did not expect to get snapshots from passthrough client!")
	}
}

func TestPassthroughClient_DeleteSnapshot(t *testing.T) {
	p := newPassthroughClient()
	fakeSnapshot := getFakeSnapshot()
	_ = p.AddSnapshot(fakeSnapshot)

	err := p.DeleteSnapshot(fakeSnapshot)

	if err != nil {
		t.Error("Could not delete snapshot from passthrough client!")
	}
}

func TestPassthroughClient_DeleteSnapshotIgnoreNotFound(t *testing.T) {
	p := newPassthroughClient()
	fakeSnapshot := getFakeSnapshot()
	_ = p.AddSnapshot(fakeSnapshot)

	err := p.DeleteSnapshotIgnoreNotFound(fakeSnapshot)

	if err != nil {
		t.Error("Could not delete snapshot from passthrough client!")
	}
}

func TestPassthroughClient_DeleteSnapshots(t *testing.T) {
	p := newPassthroughClient()
	fakeSnapshot := getFakeSnapshot()
	_ = p.AddSnapshot(fakeSnapshot)

	err := p.DeleteSnapshots()

	if err != nil {
		t.Error("Could not delete snapshots from passthrough client!")
	}
}
