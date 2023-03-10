// Copyright 2023 NetApp, Inc. All Rights Reserved.
package crd

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	fakesnapshots "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	persistentstore "github.com/netapp/trident/persistent_store"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	fakeStorage "github.com/netapp/trident/storage/fake"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/fake"
	fakeDriver "github.com/netapp/trident/storage_drivers/fake"
	testutils2 "github.com/netapp/trident/storage_drivers/fake/test_utils"
)

var (
	propagationPolicy = metav1.DeletePropagationBackground
	deleteOptions     = metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}
)

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Utility functions

func GetTestKubernetesClientset() *k8sfake.Clientset {
	return k8sfake.NewSimpleClientset()
}

func GetTestSnapshotClientset() *fakesnapshots.Clientset {
	return fakesnapshots.NewSimpleClientset()
}

func GetTestCrdClientset() *Clientset {
	return NewFakeClientset()
}

func delaySeconds(n time.Duration) {
	time.Sleep(n * time.Second)
}

func newFakeStorageDriverConfigJSON(name string) (string, error) {
	volumes := make([]fakeStorage.Volume, 0)
	return fakeDriver.NewFakeStorageDriverConfigJSON(name, config.File, testutils2.GenerateFakePools(2), volumes)
}

func TestCrdControllerBackendOperations(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// make sure these aren't nil
	assert.NotNil(t, kubeClient, "kubeClient is nil")
	assert.NotNil(t, crdClient, "crdClient is nil")
	assert.NotNil(t, crdController, "crdController is nil")
	assert.NotNil(t, crdController.crdInformerFactory, "crdController.crdInformerFactory is nil")

	expectedVersion := config.DefaultOrchestratorVersion
	if crdController.Version() != expectedVersion {
		t.Fatalf("%v differs:  '%v' != '%v'", "Version()", expectedVersion, crdController.Version())
	}

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(1)

	// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Setup work required for the crdController's logic to work
	// * create a fake backend
	// ** add it to the mock orchestrator
	// ** initialize it
	// * create a CRD version from the fake backend
	fakeConfig := fake.StorageDriver{
		Config: drivers.FakeStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: "fake",
			},
			Protocol: config.File,
		},
	}
	driver := fake.StorageDriver{
		Config: fakeConfig.Config,
	}
	fakeBackend := &storage.StorageBackend{}
	fakeBackend.SetDriver(&driver)
	fakeBackend.SetName("fake1")
	fakeBackend.SetBackendUUID(uuid.New().String())

	configJSON, jsonErr := newFakeStorageDriverConfigJSON(fakeBackend.Name())
	if jsonErr != nil {
		t.Fatalf("cannot generate JSON %v", jsonErr.Error())
	}
	commonConfig := fakeConfig.Config.CommonStorageDriverConfig

	if initializeErr := fakeBackend.Driver().Initialize(
		ctx(), "testing", configJSON, commonConfig, nil, uuid.New().String(),
	); initializeErr != nil {
		t.Fatalf("problem initializing storage driver '%s': %v", commonConfig.StorageDriverName, initializeErr)
	}
	fakeBackend.SetOnline(true)
	fakeBackend.SetState(storage.Online)

	// create a k8s CRD Object for use by the client-go bindings and crd persistence layer
	backendCRD, err := tridentv1.NewTridentBackend(ctx(), fakeBackend.ConstructPersistent(ctx()))
	if err != nil {
		t.Fatal("Unable to construct TridentBackend CRD: ", err)
	}
	if backendCRD.BackendName != fakeBackend.Name() {
		t.Fatalf("error creating backend backendCRD.BackendName '%v' != fakeBackend.Name '%v'",
			backendCRD.BackendName, fakeBackend.Name())
	}

	// create a new CRD object through the client-go api
	_, err = crdClient.TridentV1().TridentBackends(tridentNamespace).Create(ctx(), backendCRD, createOpts)
	if err != nil {
		t.Fatalf("error creating backend: %v", err.Error())
	}

	backendList, listErr := crdClient.TridentV1().TridentBackends(tridentNamespace).List(ctx(), listOpts)
	if listErr != nil {
		t.Fatalf("error listing CRD backends: %v", listErr)
	}
	var crdName string
	for _, backend := range backendList.Items {
		Log().WithFields(LogFields{
			"backend.Name":        backend.Name,
			"backend.BackendName": backend.BackendName,
			"backend.BackendUUID": backend.BackendUUID,
		}).Debug("Checking.")
		if backend.BackendName == fakeBackend.Name() {
			Log().WithFields(LogFields{
				"backend.Name":        backend.Name,
				"backend.BackendName": backend.BackendName,
				"backend.BackendUUID": backend.BackendUUID,
			}).Debug("Found.")
			crdName = backend.Name
		}
	}
	if crdName == "" {
		t.Fatalf("error finding CRD with backend.BackendName == '%v' via list", fakeBackend.Name())
	}

	crdByName, getErr := crdClient.TridentV1().TridentBackends(tridentNamespace).Get(ctx(), crdName, getOpts)
	if getErr != nil {
		t.Fatalf("error getting CRD backend '%v' error: %v", crdName, err)
	}
	if crdByName == nil {
		t.Fatalf("error getting CRD backend '%v'", crdName)
	}

	// validate we can detect and remove finalizers
	fmt.Printf(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>\n")
	if !crdByName.HasTridentFinalizers() {
		t.Fatalf("expected CRD to have finalizers")
	}
	err = crdController.removeBackendFinalizers(ctx(), crdByName)
	if err != nil {
		t.Fatalf("error removing backend finalizers: %v", err)
	}
	// to validate the finalizer removal, we must retrieve it again, after the update
	crdByName, getErr = crdClient.TridentV1().TridentBackends(tridentNamespace).Get(ctx(), crdName, getOpts)
	if getErr != nil {
		t.Fatalf("error getting CRD backend '%v' error: %v", crdName, err)
	}
	if crdByName.HasTridentFinalizers() {
		t.Fatalf("did NOT expect CRD to have finalizers, should've been force removed")
	}
	fmt.Printf("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<\n")

	// delete backend, make sure it gets removed
	deleteErr := crdClient.TridentV1().TridentBackends(tridentNamespace).Delete(ctx(), crdName, deleteOptions)
	if deleteErr != nil {
		t.Fatalf("error deleting CRD backend '%v': %v", crdName, deleteErr)
	}

	// validate it's gone
	crdByName, getErr = crdClient.TridentV1().TridentBackends(tridentNamespace).Get(ctx(), crdName, getOpts)
	Log().WithFields(LogFields{
		"crdByName": crdByName,
		"getErr":    getErr,
	}).Debug("Checking if backend CRD was deleted.")
	if getErr == nil {
		t.Fatalf("expected the CRD backend '%v' to be deleted", crdName)
	} else if !persistentstore.IsStatusNotFoundError(getErr) {
		t.Fatalf("unexpected error getting CRD backend '%v' error: %v", crdName, getErr)
	}

	// Clean up
	if err = crdController.Deactivate(); err != nil {
		t.Fatalf("error while deactivating: %v", err.Error())
	}
}

func TestCrdControllerFinalizerRemoval(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// make sure these aren't nil
	assert.NotNil(t, kubeClient, "kubeClient is nil")
	assert.NotNil(t, crdClient, "crdClient is nil")
	assert.NotNil(t, crdController, "crdController is nil")
	assert.NotNil(t, crdController.crdInformerFactory, "crdController.crdInformerFactory is nil")

	expectedVersion := config.DefaultOrchestratorVersion
	if crdController.Version() != expectedVersion {
		t.Fatalf("%v differs:  '%v' != '%v'", "Version()", expectedVersion, crdController.Version())
	}

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(1)

	// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Setup work required for the crdController's logic to work
	// * create a fake backend
	// ** add it to the mock orchestrator
	// ** initialize it
	// * create a CRD version from the fake backend
	fakeConfig := fake.StorageDriver{
		Config: drivers.FakeStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: "fake",
			},
			Protocol: config.File,
		},
	}
	storageDriver := fake.StorageDriver{
		Config: fakeConfig.Config,
	}
	fakeBackend := &storage.StorageBackend{}
	fakeBackend.SetDriver(&storageDriver)
	fakeBackend.SetName("fake1")
	fakeBackend.SetBackendUUID(uuid.New().String())

	configJSON, jsonErr := newFakeStorageDriverConfigJSON(fakeBackend.Name())
	if jsonErr != nil {
		t.Fatalf("cannot generate JSON %v", jsonErr.Error())
	}
	commonConfig := fakeConfig.Config.CommonStorageDriverConfig
	if initializeErr := fakeBackend.Driver().Initialize(
		ctx(), "testing", configJSON, commonConfig, nil, uuid.New().String(),
	); initializeErr != nil {
		t.Fatalf("problem initializing storage driver '%s': %v", commonConfig.StorageDriverName, initializeErr)
	}
	fakeBackend.SetOnline(true)
	fakeBackend.SetState(storage.Online)

	// create a k8s CRD Object for use by the client-go bindings and crd persistence layer
	backendCRD, err := tridentv1.NewTridentBackend(ctx(), fakeBackend.ConstructPersistent(ctx()))
	if err != nil {
		t.Fatal("Unable to construct TridentBackend CRD: ", err)
	}
	if backendCRD.BackendName != fakeBackend.Name() {
		t.Fatalf("error creating backend backendCRD.BackendName '%v' != fakeBackend.Name '%v'",
			backendCRD.BackendName, fakeBackend.Name())
	}

	Logc(ctx()).Debug("Creating backend.")

	// create a new Backend CRD object through the client-go api
	backend, err := crdClient.TridentV1().TridentBackends(tridentNamespace).Create(ctx(), backendCRD, createOpts)
	if err != nil {
		t.Fatalf("error creating backend: %v", err.Error())
	}

	Logc(ctx()).Debug("Created backend.")

	// Build a storage.volume
	volConfig := storage.VolumeConfig{
		Version:      config.OrchestratorAPIVersion,
		Name:         "vol1",
		InternalName: "internal_vol1",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	vol := &storage.Volume{
		Config:      &volConfig,
		BackendUUID: backendCRD.BackendUUID,
		Pool:        "aggr1",
		State:       storage.VolumeStateOnline,
	}

	// Convert to Kubernetes Object using NewTridentVolume
	volumeCRD, err := tridentv1.NewTridentVolume(ctx(), vol.ConstructExternal())
	if err != nil {
		t.Fatal("Unable to construct TridentVolume CRD: ", err)
	}

	Logc(ctx()).Debug("Creating volume.")

	// create a new Volume CRD object through the client-go api
	_, err = crdClient.TridentV1().TridentVolumes(tridentNamespace).Create(ctx(), volumeCRD, createOpts)
	if err != nil {
		t.Fatalf("error creating volume: %v", err.Error())
	}

	Logc(ctx()).Debug("Created volume.")

	// Build a storage.Snapshot
	testSnapshotConfig := &storage.SnapshotConfig{
		Version:            config.OrchestratorAPIVersion,
		Name:               "snap1",
		InternalName:       "internal_snap1",
		VolumeName:         volConfig.Name,
		VolumeInternalName: volConfig.InternalName,
	}
	now := time.Now().UTC().Format(storage.SnapshotNameFormat)
	size := int64(1000000000)
	snapshot := storage.NewSnapshot(testSnapshotConfig, now, size, storage.SnapshotStateOnline)
	snapshotCRD, err := tridentv1.NewTridentSnapshot(snapshot.ConstructPersistent())
	if err != nil {
		t.Fatal("Unable to construct TridentSnapshot CRD: ", err)
	}

	Logc(ctx()).Debug("Creating snapshot.")

	// create a new Snapshot CRD object through the client-go api
	_, err = crdClient.TridentV1().TridentSnapshots(tridentNamespace).Create(ctx(), snapshotCRD, createOpts)
	if err != nil {
		t.Fatalf("error creating volume: %v", err.Error())
	}

	Logc(ctx()).Debug("Created snapshot.")

	// validate our Volume is present
	volumeList, listErr := crdClient.TridentV1().TridentVolumes(tridentNamespace).List(ctx(), listOpts)
	if listErr != nil {
		t.Fatalf("error listing CRD volumes: %v", listErr.Error())
	}
	if len(volumeList.Items) != 1 {
		t.Fatalf("error while listing volumes, unexpected volume list length: %v", len(volumeList.Items))
	}

	// validate our Snapshot is present
	snapshotList, listErr := crdClient.TridentV1().TridentSnapshots(tridentNamespace).List(ctx(), listOpts)
	if listErr != nil {
		t.Fatalf("error listing CRD snapshots: %v", listErr.Error())
	}
	if len(snapshotList.Items) != 1 {
		t.Fatalf("error while listing snapshots, unexpected snapshot list length: %v", len(snapshotList.Items))
	}

	Logc(ctx()).Debug("Deleting snapshot.")

	// Delete the snapshot
	deleteErr := crdClient.TridentV1().TridentSnapshots(tridentNamespace).Delete(ctx(), "vol1-snap1", deleteOptions)
	if deleteErr != nil {
		t.Fatalf("unable to delete snapshot CRD: %v", deleteErr)
	}

	Logc(ctx()).Debug("Deleted snapshot.")

	snapshotGone := false

	// Wait until the snapshot disappears, which can only happen if the CRD controller removes the finalizers.
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)

		// Get the latest version of the CRD
		s, getErr := crdClient.TridentV1().TridentSnapshots(tridentNamespace).Get(ctx(), "vol1-snap1", getOpts)

		Log().WithFields(LogFields{
			"snapshot": s,
			"getErr":   getErr,
		}).Debug("Checking if snapshot was deleted.")

		if k8serrors.IsNotFound(getErr) {
			Logc(ctx()).Debug("Snapshot gone.")
			snapshotGone = true
			break
		}
	}

	Logc(ctx()).Debug("Deleting volume.")

	// Delete the snapshot
	deleteErr = crdClient.TridentV1().TridentVolumes(tridentNamespace).Delete(ctx(), "vol1", deleteOptions)
	if deleteErr != nil {
		t.Fatalf("unable to delete volume CRD: %v", deleteErr)
	}

	Logc(ctx()).Debug("Deleted volume.")

	volumeGone := false

	// Wait until the volume disappears, which can only happen if the CRD controller removes the finalizers.
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)

		// Get the latest version of the CRD
		v, getErr := crdClient.TridentV1().TridentVolumes(tridentNamespace).Get(ctx(), "vol1", getOpts)

		Log().WithFields(LogFields{
			"volume": v,
			"getErr": getErr,
		}).Debug("Checking if volume was deleted.")

		if k8serrors.IsNotFound(getErr) {
			Logc(ctx()).Debug("Volume gone.")
			volumeGone = true
			break
		}
	}

	Logc(ctx()).Debug("Deleting backend.")

	// Delete the snapshot
	deleteErr = crdClient.TridentV1().TridentBackends(tridentNamespace).Delete(ctx(), backend.Name, deleteOptions)
	if deleteErr != nil {
		t.Fatalf("unable to delete backnd CRD: %v", deleteErr)
	}

	Logc(ctx()).Debug("Deleted backend.")

	assert.True(t, snapshotGone, "expected snapshot to be deleted")
	assert.True(t, volumeGone, "expected volume to be deleted")

	// Clean up
	if err = crdController.Deactivate(); err != nil {
		t.Fatalf("error while deactivating: %v", err.Error())
	}
}

func TestCrdControllerTransactionFinalizerRemoval(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// make sure these aren't nil
	assert.NotNil(t, kubeClient, "kubeClient is nil")
	assert.NotNil(t, crdClient, "crdClient is nil")
	assert.NotNil(t, crdController, "crdController is nil")
	assert.NotNil(t, crdController.txnInformerFactory, "crdController.txnInformerFactory is nil")

	expectedVersion := config.DefaultOrchestratorVersion
	if crdController.Version() != expectedVersion {
		t.Fatalf("%v differs:  '%v' != '%v'", "Version()", expectedVersion, crdController.Version())
	}

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(1)

	// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Setup work required for the crdController's logic to work
	// * create a fake transaction
	// * create a CRD version from the fake transaction
	// * ensure CRD has Trident finalizer
	// * delete the transaction CRD
	// * ensure the CRD disappears, which can only happen if the controller removes the Trident finalizer

	// Build a volume transaction
	volConfig := storage.VolumeConfig{
		Version:      config.OrchestratorAPIVersion,
		Name:         "vol1",
		InternalName: "internal_vol1",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	volume := &storage.Volume{
		Config:      &volConfig,
		BackendUUID: uuid.New().String(),
		Pool:        "aggr1",
		State:       storage.VolumeStateOnline,
	}
	txn := &storage.VolumeTransaction{
		Config: volume.Config,
		Op:     storage.DeleteVolume,
	}

	// Create a k8s CRD Object for use by the client-go bindings and CRD persistence layer
	txnCRD, err := tridentv1.NewTridentTransaction(txn)
	if err != nil {
		t.Fatal("Unable to construct TridentTransaction CRD: ", err)
	}

	Logc(ctx()).Debug("Creating transaction.")

	// Create the CRD
	_, err = crdClient.TridentV1().TridentTransactions(tridentNamespace).Create(ctx(), txnCRD, createOpts)
	if err != nil {
		t.Fatal("Unable to create TridentTransaction CRD: ", err)
	}

	Logc(ctx()).Debug("Created transaction.")
	time.Sleep(1 * time.Second)

	// Get the CRD
	savedTxn, getErr := crdClient.TridentV1().TridentTransactions(tridentNamespace).Get(ctx(), "vol1", getOpts)
	if getErr != nil {
		t.Fatalf("unable to get transaction CRD: %v", getErr)
	}

	Log().WithFields(LogFields{
		"txn": savedTxn,
	}).Debug("Got transaction.")

	// Ensure the CRD was saved with a Trident finalizer
	if !savedTxn.HasTridentFinalizers() {
		t.Fatalf("expected transaction CRD to have Trident finalizer")
	}

	Logc(ctx()).Debug("Deleting transaction.")

	// Delete the CRD
	deleteErr := crdClient.TridentV1().TridentTransactions(tridentNamespace).Delete(ctx(), "vol1", deleteOptions)
	if deleteErr != nil {
		t.Fatalf("unable to delete transaction CRD: %v", getErr)
	}

	Logc(ctx()).Debug("Deleted transaction.")

	transactionGone := false

	// Wait until the CRD disappears, which can only happen if the CRD controller removes the finalizers.
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)

		// Get the latest version of the CRD
		updatedTxn, getErr := crdClient.TridentV1().TridentTransactions(tridentNamespace).Get(ctx(), "vol1", getOpts)

		Log().WithFields(LogFields{
			"txn":    updatedTxn,
			"getErr": getErr,
		}).Debug("Checking if transaction was deleted.")

		if k8serrors.IsNotFound(getErr) {
			Logc(ctx()).Debug("Transaction gone.")
			transactionGone = true
			break
		}
	}

	assert.True(t, transactionGone, "expected transaction to be deleted")

	// Clean up
	if err = crdController.Deactivate(); err != nil {
		t.Fatalf("error while deactivating: %v", err.Error())
	}
}
