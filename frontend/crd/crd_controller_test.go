package crd

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	testutils2 "github.com/netapp/trident/storage_drivers/fake/test_utils"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8s_fake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	persistentstore "github.com/netapp/trident/persistent_store"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	crdFake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	"github.com/netapp/trident/storage"
	fakeStorage "github.com/netapp/trident/storage/fake"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/fake"
	fakeDriver "github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/utils"
)

var (
	listOpts          = metav1.ListOptions{}
	getOpts           = metav1.GetOptions{}
	propagationPolicy = metav1.DeletePropagationBackground
	deleteOptions     = &metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}
)

/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Simple cache for our CRD objects, since we don't have a real database layer here

type TestingCache struct {
	backendCache      map[string]*tridentv1.TridentBackend
	nodeCache         map[string]*tridentv1.TridentNode
	storageClassCache map[string]*tridentv1.TridentStorageClass
	transactionCache  map[string]*tridentv1.TridentTransaction
	versionCache      map[string]*tridentv1.TridentVersion
	volumeCache       map[string]*tridentv1.TridentVolume
	snapshotCache     map[string]*tridentv1.TridentSnapshot
}

func NewTestingCache() *TestingCache {
	result := &TestingCache{
		backendCache:      make(map[string]*tridentv1.TridentBackend, 0),
		nodeCache:         make(map[string]*tridentv1.TridentNode, 0),
		storageClassCache: make(map[string]*tridentv1.TridentStorageClass, 0),
		transactionCache:  make(map[string]*tridentv1.TridentTransaction, 0),
		versionCache:      make(map[string]*tridentv1.TridentVersion, 0),
		volumeCache:       make(map[string]*tridentv1.TridentVolume, 0),
		snapshotCache:     make(map[string]*tridentv1.TridentSnapshot, 0),
	}
	return result
}

func (o *TestingCache) addBackend(backend *tridentv1.TridentBackend) {
	o.backendCache[backend.Name] = backend
}

func (o *TestingCache) updateBackend(updatedBackend *tridentv1.TridentBackend) {
	log.Debug(">>>> updateBackend")
	defer log.Debug("<<<< updateBackend")
	currentBackend := o.backendCache[updatedBackend.Name]
	if !cmp.Equal(updatedBackend, currentBackend) {
		if diff := cmp.Diff(currentBackend, updatedBackend); diff != "" {
			log.Debugf("updated object fields (-old +new):%s", diff)
			if currentBackend.ResourceVersion == "" {
				currentBackend.ResourceVersion = "1"
			}
			if currentResourceVersion, err := strconv.Atoi(currentBackend.ResourceVersion); err == nil {
				updatedBackend.ResourceVersion = strconv.Itoa(currentResourceVersion + 1)
			}
			log.WithFields(log.Fields{
				"currentBackend.ResourceVersion": currentBackend.ResourceVersion,
				"updatedBackend.ResourceVersion": updatedBackend.ResourceVersion,
			}).Debug("Incremented ResourceVersion.")
		}
	} else {
		log.Debug("No difference, leaving ResourceVersion unchanged.")
	}
	o.backendCache[updatedBackend.Name] = updatedBackend
}

func (o *TestingCache) addNode(node *tridentv1.TridentNode) {
	o.nodeCache[node.Name] = node
}

func (o *TestingCache) addStorageClass(storageClass *tridentv1.TridentStorageClass) {
	o.storageClassCache[storageClass.Name] = storageClass
}

func (o *TestingCache) addTransaction(transaction *tridentv1.TridentTransaction) {
	o.transactionCache[transaction.Name] = transaction
}

func (o *TestingCache) addVersion(version *tridentv1.TridentVersion) {
	o.versionCache[version.Name] = version
}

func (o *TestingCache) addVolume(volume *tridentv1.TridentVolume) {
	o.volumeCache[volume.Name] = volume
}

func (o *TestingCache) addSnapshot(snapshot *tridentv1.TridentSnapshot) {
	o.snapshotCache[snapshot.Name] = snapshot
}

/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Utility functions

func GetTestKubernetesClientset() *k8s_fake.Clientset {
	client := k8s_fake.NewSimpleClientset()
	return client
}

func GetTestCrdClientset() *crdFake.Clientset {
	client := crdFake.NewSimpleClientset()
	return client
}

func delaySeconds(n time.Duration) {
	time.Sleep(n * time.Second)
}

func assertNotNil(t *testing.T, name string, obj interface{}) {
	if obj == nil {
		t.Fatalf("%v is nil", name)
	}
}

func newFakeStorageDriverConfigJSON(name string) (string, error) {
	volumes := make([]fakeStorage.Volume, 0)
	return fakeDriver.NewFakeStorageDriverConfigJSON(name, config.File, testutils2.GenerateFakePools(2), volumes)
}

func addCrdTestReactors(crdFakeClient *crdFake.Clientset, testingCache *TestingCache) {

	crdFakeClient.Fake.PrependReactor(
		"*" /* all operations */, "*", /* all object types */
		//"create" /* create operations only */, "tridentbackends", /* tridentbackends object types only */
		func(actionCopy k8stesting.Action) (handled bool, ret runtime.Object, err error) {

			fmt.Printf("actionCopy: %T\n", actionCopy) // use this to find any other types to add
			switch action := actionCopy.(type) {

			case k8stesting.CreateActionImpl:
				obj := action.GetObject()
				fmt.Printf("~~ obj: %T\n", obj)
				fmt.Printf("~~ obj: %v\n", obj)
				switch crd := obj.(type) {
				case *tridentv1.TridentBackend:
					fmt.Printf("~~ crd: %T\n", crd)
					if crd.ObjectMeta.GenerateName != "" {
						if crd.Name == "" {
							crd.Name = crd.ObjectMeta.GenerateName + strings.ToLower(utils.RandomString(5))
							fmt.Printf("~~~ generated crd.Name: %v\n", crd.Name)
						}
					}
					if crd.ResourceVersion == "" {
						crd.ResourceVersion = "1"
						fmt.Printf("~~~ generated crd.ResourceVersion: %v\n", crd.ResourceVersion)
					}
					crd.ObjectMeta.Namespace = action.GetNamespace()
					testingCache.addBackend(crd)
					return false, crd, nil

				default:
					fmt.Printf("~~ crd: %T\n", crd)
				}

			case k8stesting.DeleteActionImpl:
				name := action.GetName()
				fmt.Printf("~~ name: %v\n", name)

			case k8stesting.GetActionImpl:
				name := action.GetName()
				fmt.Printf("~~ name: %v\n", name)

			case k8stesting.ListActionImpl:
				kind := action.GetKind()
				listRestrictions := action.GetListRestrictions()
				fmt.Printf("~~ kind: %T\n", kind)
				fmt.Printf("~~ listRestrictions: %v\n", listRestrictions)

			case k8stesting.PatchActionImpl:
				name := action.GetName()
				patch := action.GetPatch()
				patchType := action.GetPatchType()
				fmt.Printf("~~ name: %v\n", name)
				fmt.Printf("~~ patch: %v\n", patch)
				fmt.Printf("~~ patchType: %v\n", patchType)

			case k8stesting.UpdateActionImpl:
				obj := action.GetObject()
				fmt.Printf("~~ obj: %T\n", obj)
				fmt.Printf("~~ obj: %v\n", obj)

				switch crd := obj.(type) {
				case *tridentv1.TridentBackend:
					testingCache.updateBackend(crd)
					return false, crd, nil

				default:
				}

			default:
				fmt.Printf("~~~ unhandled type: %T\n", actionCopy) // use this to find any other types to add
			}
			return false, nil, nil
		})
}

func TestCrdController(t *testing.T) {

	testingCache := NewTestingCache()
	orchestrator := core.NewMockOrchestrator()

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	crdClient := GetTestCrdClientset()
	addCrdTestReactors(crdClient, testingCache)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// make sure these aren't nil
	assertNotNil(t, "kubeClient", kubeClient)
	assertNotNil(t, "crdClient", crdClient)
	assertNotNil(t, "crdController", crdController)
	assertNotNil(t, "crdController.crdInformerFactory", crdController.crdInformerFactory)

	expectedVersion := "0.1"
	if crdController.Version() != expectedVersion {
		t.Fatalf("%v differs:  '%v' != '%v'", "Version()", expectedVersion, crdController.Version())
	}

	/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Setup work required for the crdController's logic to work
	// * create a fake backend
	// ** add it to the mock orchestator
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
	fakeBackend := &storage.Backend{
		Driver:      &driver,
		Name:        "fake1",
		BackendUUID: uuid.New().String(),
	}
	orchestrator.AddFakeBackend(fakeBackend)
	fakeBackendFound, err := orchestrator.GetBackend(fakeBackend.Name)
	if err != nil {
		t.Fatalf("cannot find backend in orchestrator '%v' error: %v", "fake1", err.Error())
	}

	configJSON, jsonErr := newFakeStorageDriverConfigJSON(fakeBackendFound.Name)
	if jsonErr != nil {
		t.Fatalf("cannot generate JSON %v", jsonErr.Error())
	}
	commonConfig := fakeConfig.Config.CommonStorageDriverConfig
	if initializeErr := fakeBackend.Driver.Initialize("testing", configJSON, commonConfig); initializeErr != nil {
		t.Fatalf("problem initializing storage driver '%s': %v", commonConfig.StorageDriverName, initializeErr)
	}
	fakeBackend.Online = true
	fakeBackend.State = storage.BackendState("online")

	// create a k8s CRD Object for use by the client-go bindings and crd persistence layer
	backendCRD, err := tridentv1.NewTridentBackend(fakeBackend.ConstructPersistent())
	if err != nil {
		t.Fatal("Unable to construct TridentBackend CRD: ", err)
	}
	if backendCRD.BackendName != fakeBackend.Name {
		t.Fatalf("error creating backend backendCRD.BackendName '%v' != fakeBackend.Name '%v'",
			backendCRD.BackendName, fakeBackend.Name)
	}

	// create a new CRD object through the client-go api
	_, err = crdClient.TridentV1().TridentBackends(tridentNamespace).Create(backendCRD)
	if err != nil {
		t.Fatalf("error creating backend: %v", err.Error())
	}

	/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// good to go with setup, now we can activate and start monitoring
	if err := crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(2)

	backendList, listErr := crdClient.TridentV1().TridentBackends(tridentNamespace).List(listOpts)
	if listErr != nil {
		t.Fatalf("error listing CRD backends: %v", listErr)
	}
	var crdName string
	for _, backend := range backendList.Items {
		log.WithFields(log.Fields{
			"backend.Name":        backend.Name,
			"backend.BackendName": backend.BackendName,
			"backend.BackendUUID": backend.BackendUUID,
		}).Debug("Checking.")
		if backend.BackendName == fakeBackend.Name {
			log.WithFields(log.Fields{
				"backend.Name":        backend.Name,
				"backend.BackendName": backend.BackendName,
				"backend.BackendUUID": backend.BackendUUID,
			}).Debug("Found.")
			crdName = backend.Name
		}
	}
	if crdName == "" {
		t.Fatalf("error finding CRD with backend.BackendName == '%v' via list", fakeBackend.Name)
	}

	crdByName, getErr := crdClient.TridentV1().TridentBackends(tridentNamespace).Get(crdName, getOpts)
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
	crdController.removeFinalizers(crdByName, true)
	// to validate the finalizer removal, we must retrieve it again, after the update
	crdByName, getErr = crdClient.TridentV1().TridentBackends(tridentNamespace).Get(crdName, getOpts)
	if getErr != nil {
		t.Fatalf("error getting CRD backend '%v' error: %v", crdName, err)
	}
	if crdByName.HasTridentFinalizers() {
		t.Fatalf("did NOT expect CRD to have finalizers, should've been force removed")
	}
	fmt.Printf("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<\n")

	// delete backend, make sure it gets removed
	deleteErr := crdClient.TridentV1().TridentBackends(tridentNamespace).Delete(crdName, deleteOptions)
	if deleteErr != nil {
		t.Fatalf("error deleting CRD backend '%v': %v", crdName, deleteErr)
	}

	// validate it's gone
	crdByName, getErr = crdClient.TridentV1().TridentBackends(tridentNamespace).Get(crdName, getOpts)
	log.WithFields(log.Fields{
		"crdByName": crdByName,
		"getErr":    getErr,
	}).Debug("Checking if backend CRD was deleted.")
	if getErr == nil {
		t.Fatalf("expected the CRD backend '%v' to be deleted", crdName)
	} else if !persistentstore.IsStatusNotFoundError(getErr) {
		t.Fatalf("unexpected error getting CRD backend '%v' error: %v", crdName, getErr)
	}

	//	delaySeconds(2)
	if err := crdController.Deactivate(); err != nil {
		t.Fatalf("error while deactivating: %v", err.Error())
	}
}

func TestCrdController2(t *testing.T) {

	testingCache := NewTestingCache()
	orchestrator := core.NewMockOrchestrator()

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	crdClient := GetTestCrdClientset()
	addCrdTestReactors(crdClient, testingCache)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// make sure these aren't nil
	assertNotNil(t, "kubeClient", kubeClient)
	assertNotNil(t, "crdClient", crdClient)
	assertNotNil(t, "crdController", crdController)
	assertNotNil(t, "crdController.crdInformerFactory", crdController.crdInformerFactory)

	expectedVersion := "0.1"
	if crdController.Version() != expectedVersion {
		t.Fatalf("%v differs:  '%v' != '%v'", "Version()", expectedVersion, crdController.Version())
	}

	/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Setup work required for the crdController's logic to work
	// * create a fake backend
	// ** add it to the mock orchestator
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
	fakeDriver := fake.StorageDriver{
		Config: fakeConfig.Config,
	}
	fakeBackend := &storage.Backend{
		Driver:      &fakeDriver,
		Name:        "fake1",
		BackendUUID: uuid.New().String(),
	}
	orchestrator.AddFakeBackend(fakeBackend)
	fakeBackendFound, err := orchestrator.GetBackend(fakeBackend.Name)
	if err != nil {
		t.Fatalf("cannot find backend in orchestrator '%v' error: %v", "fake1", err.Error())
	}

	configJSON, jsonErr := newFakeStorageDriverConfigJSON(fakeBackendFound.Name)
	if jsonErr != nil {
		t.Fatalf("cannot generate JSON %v", jsonErr.Error())
	}
	commonConfig := fakeConfig.Config.CommonStorageDriverConfig
	if initializeErr := fakeBackend.Driver.Initialize("testing", configJSON, commonConfig); initializeErr != nil {
		t.Fatalf("problem initializing storage driver '%s': %v", commonConfig.StorageDriverName, initializeErr)
	}
	fakeBackend.Online = true
	fakeBackend.State = storage.BackendState("online")

	// create a k8s CRD Object for use by the client-go bindings and crd persistence layer
	backendCRD, err := tridentv1.NewTridentBackend(fakeBackend.ConstructPersistent())
	if err != nil {
		t.Fatal("Unable to construct TridentBackend CRD: ", err)
	}
	if backendCRD.BackendName != fakeBackend.Name {
		t.Fatalf("error creating backend backendCRD.BackendName '%v' != fakeBackend.Name '%v'",
			backendCRD.BackendName, fakeBackend.Name)
	}

	// create a new Backend CRD object through the client-go api
	_, err = crdClient.TridentV1().TridentBackends(tridentNamespace).Create(backendCRD)
	if err != nil {
		t.Fatalf("error creating backend: %v", err.Error())
	}

	// Build a storage.volume
	volConfig := storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
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
	volumeCRD, err := tridentv1.NewTridentVolume(vol.ConstructExternal())
	if err != nil {
		t.Fatal("Unable to construct TridentVolume CRD: ", err)
	}

	// create a new Volume CRD object through the client-go api
	_, err = crdClient.TridentV1().TridentVolumes(tridentNamespace).Create(volumeCRD)
	if err != nil {
		t.Fatalf("error creating volume: %v", err.Error())
	}

	// Build a storage.Snapshot
	testSnapshotConfig := &storage.SnapshotConfig{
		Version:            config.OrchestratorAPIVersion,
		Name:               "testsnap1",
		InternalName:       "internal_testsnap1",
		VolumeName:         volConfig.Name,
		VolumeInternalName: volConfig.InternalName,
	}
	now := time.Now().UTC().Format(storage.SnapshotNameFormat)
	size := int64(1000000000)
	snapshot := storage.NewSnapshot(testSnapshotConfig, now, size)
	snapshotCRD, err := tridentv1.NewTridentSnapshot(snapshot.ConstructPersistent())
	if err != nil {
		t.Fatal("Unable to construct TridentSnapshot CRD: ", err)
	}

	// create a new Snapshot CRD object through the client-go api
	_, err = crdClient.TridentV1().TridentSnapshots(tridentNamespace).Create(snapshotCRD)
	if err != nil {
		t.Fatalf("error creating volume: %v", err.Error())
	}

	/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// good to go with setup, now we can activate and start monitoring
	if err := crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(2)

	// validate our Volume is present
	volumeList, listErr := crdClient.TridentV1().TridentVolumes(tridentNamespace).List(listOpts)
	if listErr != nil {
		t.Fatalf("error listing CRD volumes: %v", err.Error())
	}
	if len(volumeList.Items) != 1 {
		t.Fatalf("error while listing volumes, unexpected volume list length: %v", len(volumeList.Items))
	}

	// validate our Snapshot is present
	snapshotList, listErr := crdClient.TridentV1().TridentSnapshots(tridentNamespace).List(listOpts)
	if listErr != nil {
		t.Fatalf("error listing CRD snapshots: %v", err.Error())
	}
	if len(snapshotList.Items) != 1 {
		t.Fatalf("error while listing snapshots, unexpected snapshot list length: %v", len(snapshotList.Items))
	}

	//	delaySeconds(2)
	if err := crdController.Deactivate(); err != nil {
		t.Fatalf("error while deactivating: %v", err.Error())
	}
}
