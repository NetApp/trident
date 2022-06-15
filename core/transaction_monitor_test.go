// Copyright 2020 NetApp, Inc. All Rights Reserved.

package core

import (
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	persistentstore "github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
	fakeDriver "github.com/netapp/trident/storage_drivers/fake"
	tu "github.com/netapp/trident/storage_drivers/fake/test_utils"
	"github.com/netapp/trident/utils"
)

const (
	period = 1 * time.Second
	maxAge = 3 * time.Second
)

func init() {
	testing.Init()
	log.SetLevel(log.DebugLevel)
}

func waitForTransactionMontitorToStart(o *TridentOrchestrator) {
	if o.txnMonitorChannel == nil {
		time.Sleep(1 * time.Second)
	}
}

func TestStartStop(t *testing.T) {
	storeClient := persistentstore.NewInMemoryClient()
	o := NewTridentOrchestrator(storeClient)
	if err := o.Bootstrap(true); err != nil {
		log.Fatal("Failure occurred during bootstrapping: ", err)
	}

	waitForTransactionMontitorToStart(o)

	assert.NotNil(t, o.txnMonitorChannel)
	assert.False(t, o.txnMonitorStopped)

	o.StopTransactionMonitor()
	assert.True(t, o.txnMonitorStopped)
}

// TestLongRunningTransaction uses the Fake driver to simulate Kubernetes sending multiple calls to create a volume.
func TestLongRunningTransaction(t *testing.T) {
	o, storeClient := setupOrchestratorAndBackend(t)

	volName := fakeDriver.PVC_creating_01
	volumeConfig := tu.GenerateVolumeConfig(volName, 1, "slow", config.File)

	_, err := o.AddVolume(ctx(), volumeConfig)
	if err != nil {
		assert.True(t, utils.IsVolumeCreatingError(err))
	}

	_, err = o.AddVolume(ctx(), volumeConfig)
	if err != nil {
		assert.True(t, utils.IsVolumeCreatingError(err))
	}

	volTxns, err := storeClient.GetVolumeTransactions(ctx())
	if err != nil {
		t.Errorf("failed to get volume transactions: %v", err)
	}
	assert.Equal(t, volName, volTxns[0].VolumeCreatingConfig.InternalName, "failed to find matching transaction")

	_, err = o.AddVolume(ctx(), volumeConfig)
	if err != nil {
		assert.True(t, utils.IsVolumeCreatingError(err))
	}

	vol, err := o.AddVolume(ctx(), volumeConfig)
	if err != nil {
		t.Errorf("Unable to create volume %s: %v", volName, err)
	}
	assert.Equal(t, volName, vol.Config.Name)

	volTxns, err = storeClient.GetVolumeTransactions(ctx())
	assert.True(t, len(volTxns) == 0, "number of volume transactions should be 0")
}

// TestCancelledLongRunningTransaction tests that a transaction older than the max age is cancelled.
func TestCancelledLongRunningTransaction(t *testing.T) {
	o, storeClient := setupOrchestratorAndBackend(t)
	restartTransactionMonitor(o)

	volName := fakeDriver.PVC_creating_01
	volumeConfig := tu.GenerateVolumeConfig(volName, 1, "slow", config.File)

	_, err := o.AddVolume(ctx(), volumeConfig)
	if err != nil {
		assert.True(t, utils.IsVolumeCreatingError(err))
	}

	volTxns, err := storeClient.GetVolumeTransactions(ctx())
	if err != nil {
		t.Errorf("failed to get volume transactions: %v", err)
	}

	volumeTransation := volTxns[0]
	assert.Equal(t, volName, volumeTransation.VolumeCreatingConfig.InternalName, "failed to find matching transaction")

	// Allow time to check for expired transaction and to reap the transaction
	time.Sleep(maxAge + (2 * time.Second))
	volTxns, err = storeClient.GetVolumeTransactions(ctx())
	if err != nil {
		t.Errorf("failed to get volume transactions: %v", err)
	}
	assert.True(t, len(volTxns) == 0, "number of volume transactions should be 0")
}

// TestUpdateTransactionVolumeCreatingTransaction tests that a VolumeCreatingTransaction can be updated.
func TestUpdateVolumeCreatingTransaction(t *testing.T) {
	o, storeClient := setupOrchestratorAndBackend(t)
	restartTransactionMonitor(o)

	volName := fakeDriver.PVC_creating_01
	volumeConfig := tu.GenerateVolumeConfig(volName, 1, "slow", config.File)

	_, err := o.AddVolume(ctx(), volumeConfig)
	if err != nil {
		assert.True(t, utils.IsVolumeCreatingError(err))
	}

	volTxns, err := storeClient.GetVolumeTransactions(ctx())
	if err != nil {
		t.Errorf("failed to get volume transactions: %v", err)
	}

	volumeTransaction := volTxns[0]
	assert.Equal(t, volName, volumeTransaction.VolumeCreatingConfig.InternalName, "failed to find matching transaction")
	assert.Equal(t, "1073741824", volumeTransaction.VolumeCreatingConfig.Size)

	// Update the transaction
	volumeTransaction.VolumeCreatingConfig.Size = "5000000000"
	err = storeClient.UpdateVolumeTransaction(ctx(), volumeTransaction)
	if err != nil {
		t.Errorf("failed to update volume transactions: %v", err)
	}

	// Verify updated size
	volTxns, err = storeClient.GetVolumeTransactions(ctx())
	if err != nil {
		t.Errorf("failed second get volume transactions call: %v", err)
	}
	assert.Equal(t, "5000000000", volTxns[0].VolumeCreatingConfig.Size)
}

// TestErrorVolumeCreatingTransaction tests that the VolumeCreatingTransaction is deleted if an error is thrown
func TestErrorVolumeCreatingTransaction(t *testing.T) {
	o, storeClient := setupOrchestratorAndBackend(t)
	restartTransactionMonitor(o)

	volName := fakeDriver.PVC_creating_02
	volumeConfig := tu.GenerateVolumeConfig(volName, 1, "slow", config.File)

	_, err := o.AddVolume(ctx(), volumeConfig)
	if err != nil {
		assert.True(t, utils.IsVolumeCreatingError(err))
	}

	// Verify transaction exists
	volTxns, err := storeClient.GetVolumeTransactions(ctx())
	if err != nil {
		t.Errorf("failed to get volume transactions: %v", err)
	}
	assert.Equal(t, volName, volTxns[0].VolumeCreatingConfig.InternalName, "failed to find matching transaction")

	// Call AddVolume again to receive volume creation error
	_, err = o.AddVolume(ctx(), volumeConfig)
	if err != nil {
		assert.Equal(t, "error occurred during creation on backend", err.Error())
	}

	// Verify transaction deleted after error returned
	volTxns, err = storeClient.GetVolumeTransactions(ctx())
	if err != nil {
		t.Errorf("failed to get volume transactions: %v", err)
	}

	assert.Equal(t, 0, len(volTxns), "did not expect to find any volume transactions")
}

// TestVolumeCreatingTwoTransaction tests that two volumeCreatingTransactions work as expected
func TestVolumeCreatingTwoTransactions(t *testing.T) {
	o, storeClient := setupOrchestratorAndBackend(t)
	restartTransactionMonitor(o)

	volName := "volToClone_01"
	cloneName := fakeDriver.PVC_creating_clone_03
	volumeConfig := tu.GenerateVolumeConfig(volName, 1, "slow", config.File)
	cloneVolumeConfig := tu.GenerateVolumeConfig(cloneName, 1, "slow", config.File)

	_, err := o.AddVolume(ctx(), volumeConfig)
	if err != nil {
		t.Errorf("failed to create volume: %v", err)
	}

	cloneVolumeConfig.CloneSourceVolume = volName
	log.Debugf("CloneSourceVolume %s", cloneVolumeConfig.CloneSourceVolume)

	_, err = o.CloneVolume(ctx(), cloneVolumeConfig)
	if err != nil {
		assert.True(t, utils.IsVolumeCreatingError(err))
	}
	// Verify transaction exists
	volTxns, err := storeClient.GetVolumeTransactions(ctx())
	if err != nil {
		t.Errorf("failed to get volume transactions: %v", err)
	}
	assert.Equal(t, cloneName, volTxns[0].VolumeCreatingConfig.InternalName, "failed to find matching transaction")

	// With existing volumeCreatingTransaction for a clone operation in place verify the same for create volume.
	volName02 := fakeDriver.PVC_creating_01
	volumeConfig02 := tu.GenerateVolumeConfig(volName02, 1, "slow", config.File)

	_, err = o.AddVolume(ctx(), volumeConfig02)
	if err != nil {
		assert.True(t, utils.IsVolumeCreatingError(err))
	}

	// Verify transactions exist
	volTxns, err = storeClient.GetVolumeTransactions(ctx())
	if err != nil {
		t.Errorf("failed to get volume transactions: %v", err)
	}

	assert.Equal(t, 2, len(volTxns))
	for _, v := range volTxns {
		volTxnName := v.VolumeCreatingConfig.InternalName
		if (cloneName != volTxnName) && (volName02 != volTxnName) {
			t.Errorf("did not find expected transaction name %s", volTxnName)
		}
	}
	_, err = o.CloneVolume(ctx(), cloneVolumeConfig)
	if err != nil {
		t.Errorf("failed to clone volume: %v", err)
	}

	// Verify clone transaction is deleted
	volTxns, err = storeClient.GetVolumeTransactions(ctx())
	if err != nil {
		t.Errorf("failed to get volume transactions: %v", err)
	}
	assert.Equal(t, 1, len(volTxns))
	assert.Equal(t, volName02, volTxns[0].VolumeCreatingConfig.InternalName, "failed to find matching transaction")
}

func setupOrchestratorAndBackend(t *testing.T) (*TridentOrchestrator, *persistentstore.InMemoryClient) {
	storeClient := persistentstore.NewInMemoryClient()
	o := NewTridentOrchestrator(storeClient)
	if err := o.Bootstrap(true); err != nil {
		t.Errorf("Failure occurred during bootstrapping %v", err)
	}
	// Wait for bootstrap to complete
	time.Sleep(2 * time.Second)

	backendName := "fakeOne"
	volumes := make([]fake.Volume, 0)
	fakeConfig, err := fakeDriver.NewFakeStorageDriverConfigJSON(backendName, config.File, tu.GenerateFakePools(1), volumes)
	_, err = o.AddBackend(ctx(), fakeConfig, "")
	if err != nil {
		t.Errorf("Unable to add backend %s: %v", backendName, err)
	}

	backends, err := storeClient.GetBackends(ctx())
	if err != nil {
		t.Errorf("Unable to get backends: %v", err)
	}
	backend := backends[0]
	assert.Equal(t, backendName, backend.Name)

	sc := &storageclass.Config{
		Name: "slow",
		Attributes: map[string]sa.Request{
			sa.IOPS:             sa.NewIntRequest(50),
			sa.Snapshots:        sa.NewBoolRequest(false),
			sa.ProvisioningType: sa.NewStringRequest("thin"),
		},
	}

	_, err = o.AddStorageClass(ctx(), sc)
	if err != nil {
		t.Errorf("Unable to add storage class %s: %v", sc.Name, err)
	}
	return o, storeClient
}

func restartTransactionMonitor(o *TridentOrchestrator) {
	// Bootstrap starts the transaction monitor.
	// Need to stop and reinitialize transaction monitor with testable limits.
	o.StopTransactionMonitor()
	o.StartTransactionMonitor(ctx(), period, maxAge)
	time.Sleep(1 * time.Second)
}
