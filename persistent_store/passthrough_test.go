// Copyright 2022 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	sc "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	fakedriver "github.com/netapp/trident/storage_drivers/fake"
	testutils "github.com/netapp/trident/storage_drivers/fake/test_utils"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/models"
)

func getFakeBackend() *storage.StorageBackend {
	return getFakeBackendWithName("fake_backend")
}

func getFakeBackendWithName(name string) *storage.StorageBackend {
	fakeConfig := drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           drivers.ConfigVersion,
			StorageDriverName: config.FakeStorageDriverName,
		},
		Protocol:     config.File,
		Pools:        testutils.GenerateFakePools(2),
		InstanceName: name,
	}

	fakeStorageDriver := fakedriver.NewFakeStorageDriver(ctx(), fakeConfig)
	fakeBackend, _ := storage.NewStorageBackend(ctx(), fakeStorageDriver)
	return fakeBackend
}

func getFakeVolume(fakeBackend *storage.StorageBackend) *storage.Volume {
	return getFakeVolumeWithName("fake_volume", fakeBackend)
}

func getFakeVolumeWithName(name string, fakeBackend *storage.StorageBackend) *storage.Volume {
	volumeConfig := &storage.VolumeConfig{
		Name:         name,
		InternalName: name + "_internal",
	}

	return storage.NewVolume(volumeConfig, fakeBackend.BackendUUID(), fakeBackend.Storage()["pool-0"].Name(),
		false, storage.VolumeStateOnline)
}

func getFakeVolumeTransaction() *storage.VolumeTransaction {
	return getFakeVolumeTransactionWithName("fake_volume")
}

func getFakeVolumeTransactionWithName(name string) *storage.VolumeTransaction {
	volumeConfig := &storage.VolumeConfig{
		Name:         name,
		InternalName: name + "_internal",
	}
	snapshotConfig := &storage.SnapshotConfig{}

	return &storage.VolumeTransaction{
		Config:         volumeConfig,
		SnapshotConfig: snapshotConfig,
		Op:             storage.AddVolume,
	}
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

func getFakeNode() *models.Node {
	return &models.Node{
		Name:    "testNode",
		IQN:     "myIQN",
		IPs:     []string{"1.1.1.1", "2.2.2.2"},
		Deleted: false,
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
		Created:   time.Now().UTC().Format(utils.TimestampFormat),
		SizeBytes: 1000000000,
		State:     storage.SnapshotStateOnline,
	}
}

func newPassthroughClient() *PassthroughClient {
	return &PassthroughClient{
		bootBackends: make([]*storage.BackendPersistent, 0),
		liveBackends: make(map[string]storage.Backend),
		version: &config.PersistentStateVersion{
			PersistentStoreVersion: "passthrough",
			OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		},
	}
}

func TestPassthroughClient_NewPassthroughClientSingleFile(t *testing.T) {
	var configPath string
	if runtime.GOOS == "windows" {
		configPath = os.Getenv("TEMP") + "\\fake_backend"
	} else {
		configPath = "/tmp/fake_backend"
	}
	backendJSON, _ := getFakeBackend().ConstructPersistent(ctx()).MarshalConfig()
	err := os.WriteFile(configPath, []byte(backendJSON), 0o644)
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
	err = os.Remove(configPath)
	if err != nil {
		t.Fatalf("could not remove file: %v", err)
	}
}

func TestPassthroughClient_NewPassthroughClientDirectory(t *testing.T) {
	configPath := "/tmp/fake_backends"
	if err := os.MkdirAll(configPath, 0o755); err != nil {
		t.Errorf("Error creating dir path: %v %v", configPath, err)
	}
	backend1JSON, _ := getFakeBackend().ConstructPersistent(ctx()).MarshalConfig()
	err := os.WriteFile(configPath+"/backend1", []byte(backend1JSON), 0o644)
	if err != nil {
		t.Error(err.Error())
	}
	backend2JSON, _ := getFakeBackend().ConstructPersistent(ctx()).MarshalConfig()
	err = os.WriteFile(configPath+"/backend2", []byte(backend2JSON), 0o644)
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
	err = os.Remove(configPath + "/backend1")
	if err != nil {
		t.Fatalf("could not remove file: %v", err)
	}
	err = os.Remove(configPath + "/backend2")
	if err != nil {
		t.Fatalf("could not remove file: %v", err)
	}
	err = os.Remove(configPath)
	if err != nil {
		t.Fatalf("could not remove file: %v", err)
	}
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
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	if len(p.liveBackends) == 0 {
		t.Error("Could not add backend to passthrough client!")
	}

	if err := p.Stop(); err != nil {
		t.Errorf("Error stopping passthrough client: %v %v", p, err)
	}

	if len(p.liveBackends) != 0 {
		t.Errorf("Passthrough client failed to stop!")
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

	result, _ := p.GetVersion(ctx())

	expected := &config.PersistentStateVersion{
		PersistentStoreVersion: "passthrough",
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
	}
	if *result != *expected {
		t.Error("Passthrough client returned unexpected version!")
	}
}

func TestPassthroughClient_SetVersion(t *testing.T) {
	p := newPassthroughClient()

	if err := p.SetVersion(ctx(), &config.PersistentStateVersion{
		PersistentStoreVersion: "invalid",
		OrchestratorAPIVersion: "unknown",
	}); err != nil {
		t.Errorf("Error setting version: %v", err)
	}

	result, _ := p.GetVersion(ctx())
	expected := &config.PersistentStateVersion{
		PersistentStoreVersion: "passthrough",
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
	}
	if *result != *expected {
		t.Error("Passthrough client returned unexpected version!")
	}
}

func TestPassthroughClient_AddBackend(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()

	err := p.AddBackend(ctx(), fakeBackend)
	if err != nil {
		t.Error("Could not add backend to passthrough client!")
	}
	if len(p.liveBackends) != 1 {
		t.Error("Could not add backend to passthrough client!")
	}

	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	if len(p.liveBackends) != 1 {
		t.Error("Added backend a second time to passthrough client!")
	}
}

func TestPassthroughClient_GetBackend(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Error adding backend: %v %v", fakeBackend, err)
	}
	result, err := p.GetBackend(ctx(), fakeBackend.Name())
	if err != nil {
		t.Fatal("Could not get backend from passthrough client!")
	}
	if result.Name != fakeBackend.Name() {
		t.Error("Got incorrect backend from passthrough client!")
	}
}

func TestPassthroughClient_GetBackendNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()

	result, err := p.GetBackend(ctx(), fakeBackend.Name())

	if result != nil || err == nil {
		t.Error("Got incorrect backend from passthrough client!")
	}
}

func TestPassthroughClient_UpdateBackend(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Error adding backend: %v %v", fakeBackend, err)
	}
	fakeBackend.SetState(storage.Deleting)

	err := p.UpdateBackend(ctx(), fakeBackend)
	if err != nil {
		t.Error("Could not update backend on passthrough client!")
	}
	result, err := p.GetBackend(ctx(), fakeBackend.Name())
	if err != nil {
		t.Fatal("Could not get backend from passthrough client!")
	}
	if !result.State.IsDeleting() {
		t.Error("Backend not updated on passthrough client!")
	}
}

func TestPassthroughClient_UpdateBackendNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()

	err := p.UpdateBackend(ctx(), fakeBackend)

	if err == nil {
		t.Error("Backend update should have failed on passthrough client!")
	}
}

func TestPassthroughClient_DeleteBackend(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	if len(p.liveBackends) != 1 {
		t.Error("Could not add backend to passthrough client!")
	}

	err := p.DeleteBackend(ctx(), fakeBackend)
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

	err := p.DeleteBackend(ctx(), fakeBackend)
	if err != nil {
		t.Error("Backend delete should have succeeded on passthrough client!")
	}
}

func TestPassthroughClient_GetBackends(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend1 := getFakeBackend()
	fakeBackend1.SetName("fake1")
	p.bootBackends = append(p.bootBackends, fakeBackend1.ConstructPersistent(ctx()))
	fakeBackend2 := getFakeBackend()
	fakeBackend2.SetName("fake2")
	p.bootBackends = append(p.bootBackends, fakeBackend2.ConstructPersistent(ctx()))

	result, err := p.GetBackends(ctx())
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
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	result, err := p.GetBackends(ctx())
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
	fakeBackend1.SetName("fake1")
	if err := p.AddBackend(ctx(), fakeBackend1); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend1, err)
	}
	p.bootBackends = append(p.bootBackends, fakeBackend1.ConstructPersistent(ctx()))
	fakeBackend2 := getFakeBackend()
	fakeBackend2.SetName("fake2")
	if err := p.AddBackend(ctx(), fakeBackend2); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend2, err)
	}
	p.bootBackends = append(p.bootBackends, fakeBackend2.ConstructPersistent(ctx()))

	err := p.DeleteBackends(ctx())
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

	err := p.DeleteBackends(ctx())
	if err != nil {
		t.Error("Could not delete backends from passthrough client!")
	}
	backends, _ := p.GetBackends(ctx())
	if len(backends) != 0 {
		t.Error("Could not delete backends from passthrough client!")
	}
}

func TestPassthroughClient_AddVolume(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeVolume := getFakeVolume(fakeBackend)

	err := p.AddVolume(ctx(), fakeVolume)
	if err != nil {
		t.Error("Could not add volume to passthrough client!")
	}
}

func TestPassthroughClient_GetVolume(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeVolume := getFakeVolume(fakeBackend)
	if err := p.AddVolume(ctx(), fakeVolume); err != nil {
		t.Errorf("Error adding volume: %v %v", fakeVolume, err)
	}
	vol, err := p.GetVolume(ctx(), "fake_volume")

	if vol != nil || err == nil {
		t.Error("Get volume should have failed on passthrough client!")
	}
}

func TestPassthroughClient_GetVolumeNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	vol, err := p.GetVolume(ctx(), "fake_volume")

	if vol != nil || err == nil {
		t.Error("Get volume should have failed on passthrough client!")
	}
}

func TestPassthroughClient_UpdateVolume(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeVolume := getFakeVolume(fakeBackend)
	if err := p.AddVolume(ctx(), fakeVolume); err != nil {
		t.Errorf("Error adding volume: %v %v", fakeVolume, err)
	}
	err := p.UpdateVolume(ctx(), fakeVolume)
	if err != nil {
		t.Error("Could not update volume on passthrough client!")
	}
}

func TestPassthroughClient_UpdateVolumeNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeVolume := getFakeVolume(fakeBackend)

	err := p.UpdateVolume(ctx(), fakeVolume)
	if err != nil {
		t.Error("Could not update volume on passthrough client!")
	}
}

func TestPassthroughClient_DeleteVolume(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeVolume := getFakeVolume(fakeBackend)
	if err := p.AddVolume(ctx(), fakeVolume); err != nil {
		t.Errorf("Error adding volume: %v %v", fakeVolume, err)
	}

	err := p.DeleteVolume(ctx(), fakeVolume)
	if err != nil {
		t.Error("Could not delete volume from passthrough client!")
	}
}

func TestPassthroughClient_DeleteVolumeNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeVolume := getFakeVolume(fakeBackend)

	err := p.DeleteVolume(ctx(), fakeVolume)
	if err != nil {
		t.Error("Could not delete volume on passthrough client!")
	}
}

func TestPassthroughClient_DeleteVolumes(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeVolume := getFakeVolume(fakeBackend)
	if err := p.AddVolume(ctx(), fakeVolume); err != nil {
		t.Errorf("Error adding volume: %v %v", fakeVolume, err)
	}
	err := p.DeleteVolumes(ctx())
	if err != nil {
		t.Error("Could not delete volumes on passthrough client!")
	}
}

func TestPassthroughClient_DeleteVolumesNonexistent(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	err := p.DeleteVolumes(ctx())
	if err != nil {
		t.Error("Could not delete volumes on passthrough client!")
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
	err := fakeBackend.Driver().Create(ctx(), volConfig, fakeBackend.Storage()["pool-0"], make(map[string]sa.Request))
	if err != nil {
		t.Error(err)
	}

	volConfig = &storage.VolumeConfig{
		Name:         "fake_volume_2",
		InternalName: "fake_volume_2",
		Size:         "2000000000",
	}
	err = fakeBackend.Driver().Create(ctx(), volConfig, fakeBackend.Storage()["pool-0"], make(map[string]sa.Request))
	if err != nil {
		t.Error(err)
	}

	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	result, err := p.GetVolumes(ctx())
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
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	result, err := p.GetVolumes(ctx())
	if err != nil {
		t.Error("Could not get volumes from passthrough client!")
	}
	if len(result) != 0 {
		t.Error("Got wrong number of volumes from passthrough client!")
	}
}

func TestPassthroughClient_AddVolumeTransaction(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeTransaction := getFakeVolumeTransaction()

	err := p.AddVolumeTransaction(ctx(), fakeTransaction)
	if err != nil {
		t.Error("Could not add volume transaction to passthrough client!")
	}
}

func TestPassthroughClient_GetVolumeTransactions(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeTransaction := getFakeVolumeTransaction()
	if err := p.AddVolumeTransaction(ctx(), fakeTransaction); err != nil {
		t.Errorf("Error adding volume transaction: %v %v", fakeTransaction, err)
	}
	result, err := p.GetVolumeTransactions(ctx())
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
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeTransaction := getFakeVolumeTransaction()
	if err := p.AddVolumeTransaction(ctx(), fakeTransaction); err != nil {
		t.Errorf("Error adding volume transaction: %v %v", fakeTransaction, err)
	}
	result, err := p.GetVolumeTransaction(ctx(), fakeTransaction)
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
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeTransaction := getFakeVolumeTransaction()
	if err := p.AddVolumeTransaction(ctx(), fakeTransaction); err != nil {
		t.Errorf("Error adding volume transaction: %v %v", fakeTransaction, err)
	}

	err := p.DeleteVolumeTransaction(ctx(), fakeTransaction)
	if err != nil {
		t.Error("Could not delete volume transaction from passthrough client!")
	}
}

func TestPassthroughClient_AddStorageClass(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeSC := getFakeStorageClass()

	err := p.AddStorageClass(ctx(), fakeSC)
	if err != nil {
		t.Error("Could not add storage class to passthrough client!")
	}
}

func TestPassthroughClient_GetStorageClass(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeSC := getFakeStorageClass()
	if err := p.AddStorageClass(ctx(), fakeSC); err != nil {
		t.Errorf("Failure adding storage class: %v %v", fakeSC, err)
	}
	result, err := p.GetStorageClass(ctx(), fakeSC.GetName())

	if result != nil || err == nil {
		t.Error("Did not expect to get storage class from passthrough client!")
	}
}

func TestPassthroughClient_GetStorageClasses(t *testing.T) {
	p := newPassthroughClient()
	fakeBackend := getFakeBackend()
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeSC := getFakeStorageClass()
	if err := p.AddStorageClass(ctx(), fakeSC); err != nil {
		t.Errorf("Failure adding storage class: %v %v", fakeSC, err)
	}
	result, err := p.GetStorageClasses(ctx())
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
	if err := p.AddBackend(ctx(), fakeBackend); err != nil {
		t.Errorf("Failure adding backend: %v %v", fakeBackend, err)
	}
	fakeSC := getFakeStorageClass()
	if err := p.AddStorageClass(ctx(), fakeSC); err != nil {
		t.Errorf("Failure adding storage class: %v %v", fakeSC, err)
	}

	err := p.DeleteStorageClass(ctx(), fakeSC)
	if err != nil {
		t.Error("Could not delete storage class from passthrough client!")
	}
}

func TestPassthroughClient_AddNode(t *testing.T) {
	p := newPassthroughClient()
	fakeNode := getFakeNode()

	err := p.AddOrUpdateNode(ctx(), fakeNode)
	if err != nil {
		t.Error("Could not add node to passthrough client!")
	}
}

func TestPassthroughClient_GetNode(t *testing.T) {
	p := newPassthroughClient()
	fakeNode := getFakeNode()
	if err := p.AddOrUpdateNode(ctx(), fakeNode); err != nil {
		t.Errorf("Failure adding or updating node: %v %v", fakeNode, err)
	}
	result, err := p.GetNode(ctx(), fakeNode.Name)

	if result != nil || err == nil {
		t.Error("Did not expect to get node from passthrough client!")
	}
}

func TestPassthroughClient_GetNodes(t *testing.T) {
	p := newPassthroughClient()
	fakeNode := getFakeNode()
	if err := p.AddOrUpdateNode(ctx(), fakeNode); err != nil {
		t.Errorf("Failure adding or updating node: %v %v", fakeNode, err)
	}
	result, err := p.GetNodes(ctx())
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
	if err := p.AddOrUpdateNode(ctx(), fakeNode); err != nil {
		t.Errorf("Failure adding or updating node: %v %v", fakeNode, err)
	}

	err := p.DeleteNode(ctx(), fakeNode)
	if err != nil {
		t.Error("Could not delete node from passthrough client!")
	}
}

func TestPassthroughClient_AddSnapshot(t *testing.T) {
	p := newPassthroughClient()
	fakeSnapshot := getFakeSnapshot()

	err := p.AddSnapshot(ctx(), fakeSnapshot)
	if err != nil {
		t.Error("Could not add snapshot to passthrough client!")
	}
}

func TestPassthroughClient_GetSnapshot(t *testing.T) {
	p := newPassthroughClient()
	fakeSnapshot := getFakeSnapshot()
	_ = p.AddSnapshot(ctx(), fakeSnapshot)

	result, err := p.GetSnapshot(ctx(), fakeSnapshot.Config.VolumeName, fakeSnapshot.Config.Name)

	if result != nil || err == nil {
		t.Error("Did not expect to get snapshot from passthrough client!")
	}
}

func TestPassthroughClient_GetSnapshots(t *testing.T) {
	p := newPassthroughClient()
	fakeSnapshot := getFakeSnapshot()
	_ = p.AddSnapshot(ctx(), fakeSnapshot)

	result, err := p.GetSnapshots(ctx())
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
	_ = p.AddSnapshot(ctx(), fakeSnapshot)

	err := p.DeleteSnapshot(ctx(), fakeSnapshot)
	if err != nil {
		t.Error("Could not delete snapshot from passthrough client!")
	}
}

func TestPassthroughClient_DeleteSnapshots(t *testing.T) {
	p := newPassthroughClient()
	fakeSnapshot := getFakeSnapshot()
	_ = p.AddSnapshot(ctx(), fakeSnapshot)

	err := p.DeleteSnapshots(ctx())
	if err != nil {
		t.Error("Could not delete snapshots from passthrough client!")
	}
}
