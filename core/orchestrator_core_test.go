// Copyright 2022 NetApp, Inc. All Rights Reserved.

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	mockpersistentstore "github.com/netapp/trident/mocks/mock_persistent_store"
	mockstorage "github.com/netapp/trident/mocks/mock_storage"
	persistentstore "github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
	fakedriver "github.com/netapp/trident/storage_drivers/fake"
	tu "github.com/netapp/trident/storage_drivers/fake/test_utils"
	"github.com/netapp/trident/utils"
)

var (
	debug = flag.Bool("debug", false, "Enable debugging output")

	inMemoryClient *persistentstore.InMemoryClient
	ctx            = context.Background
)


type deleteTest struct {
	name            string
	expectedSuccess bool
}

type recoveryTest struct {
	name           string
	volumeConfig   *storage.VolumeConfig
	snapshotConfig *storage.SnapshotConfig
	expectDestroy  bool
}

// cleanup is a helper function to clean up the persistent store.
// Parameters:
//   t - the test object
//   o - the orchestrator object
// Example:
//   defer cleanup(t, o)

func cleanup(t *testing.T, o *TridentOrchestrator) {
	err := o.storeClient.DeleteBackends(ctx())
	if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatal("Unable to clean up backends: ", err)
	}
	storageClasses, err := o.storeClient.GetStorageClasses(ctx())
	if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatal("Unable to retrieve storage classes: ", err)
	} else if err == nil {
		for _, psc := range storageClasses {
			sc := storageclass.NewFromPersistent(psc)
			err := o.storeClient.DeleteStorageClass(ctx(), sc)
			if err != nil {
				t.Fatalf("Unable to clean up storage class %s: %v", sc.GetName(), err)
			}
		}
	}
	err = o.storeClient.DeleteVolumes(ctx())
	if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatal("Unable to clean up volumes: ", err)
	}
	err = o.storeClient.DeleteSnapshots(ctx())
	if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatal("Unable to clean up snapshots: ", err)
	}

	// Clear the InMemoryClient state so that it looks like we're
	// bootstrapping afresh next time.
	if err = inMemoryClient.Stop(); err != nil {
		t.Fatalf("Unable to stop in memory client for orchestrator: %v", o)
	}
}

// diffConfig compares two structs, ignoring the fieldToSkip
// and returns a list of differences
// Parameters:
//   expected: the expected struct
//   got: the actual struct
//   fieldToSkip: the name of a field to skip
// Returns:
//   a list of differences
// It returns an empty list if there are no differences
// Example:
//   diffs := diffConfig(expected, got, "")
//   if len(diffs) > 0 {
//     t.Errorf("Config differs: %v", diffs)
//   }

func diffConfig(expected, got interface{}, fieldToSkip string) []string {

	diffs := make([]string, 0)
	expectedStruct := reflect.Indirect(reflect.ValueOf(expected))
	gotStruct := reflect.Indirect(reflect.ValueOf(got))

	for i := 0; i < expectedStruct.NumField(); i++ {

		// Optionally skip a field
		typeName := expectedStruct.Type().Field(i).Name
		if typeName == fieldToSkip {
			continue
		}

		// Compare each field in the structs
		expectedField := expectedStruct.FieldByName(typeName).Interface()
		gotField := gotStruct.FieldByName(typeName).Interface()

		if !reflect.DeepEqual(expectedField, gotField) {
			diffs = append(diffs, fmt.Sprintf("%s: expected %v, got %v", typeName, expectedField, gotField))
		}
	}

	return diffs
}

// To be called after reflect.DeepEqual has failed.
// diffExternalBackends compares two external backends and returns a list of differences
// Parameters:
//   expected: the expected backend
//   got: the backend to compare against
// Returns:
//   A list of differences
// Example:
//   diffs := diffExternalBackends(expected, got)
//   if len(diffs) > 0 {
//     t.Errorf("External backends differ:\n\t%s", strings.Join(diffs, "\n\t"))
//   }

func diffExternalBackends(t *testing.T, expected, got *storage.BackendExternal) {

	diffs := make([]string, 0)

	if expected.Name != got.Name {
		diffs = append(diffs, fmt.Sprintf("Name: expected %s, got %s", expected.Name, got.Name))
	}
	if expected.State != got.State {
		diffs = append(diffs, fmt.Sprintf("Online: expected %s, got %s", expected.State, got.State))
	}

	// Diff configs
	expectedConfig := expected.Config
	gotConfig := got.Config

	expectedConfigTypeName := reflect.TypeOf(expectedConfig).Name()
	gotConfigTypeName := reflect.TypeOf(gotConfig).Name()
	if expectedConfigTypeName != gotConfigTypeName {
		t.Errorf("Config type mismatch: %v != %v", expectedConfigTypeName, gotConfigTypeName)
	}

	expectedConfigValue := reflect.ValueOf(expectedConfig)
	gotConfigValue := reflect.ValueOf(gotConfig)

	expectedCSDCIntf := expectedConfigValue.FieldByName("CommonStorageDriverConfig").Interface()
	gotCSDCIntf := gotConfigValue.FieldByName("CommonStorageDriverConfig").Interface()

	var configDiffs []string

	// Compare the common storage driver config
	configDiffs = diffConfig(expectedCSDCIntf, gotCSDCIntf, "")
	diffs = append(diffs, configDiffs...)

	// Compare the base config, without the common storage driver config
	configDiffs = diffConfig(expectedConfig, gotConfig, "CommonStorageDriverConfig")
	diffs = append(diffs, configDiffs...)

	t.Logf("expectedConfig %v", expectedConfig)
	t.Logf("gotConfig %v", gotConfig)
	t.Logf("diffs %v", diffs)
	// Diff storage
	for name, expectedVC := range expected.Storage {
		if gotVC, ok := got.Storage[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("Storage: did not get expected VC %s", name))
		} else if !reflect.DeepEqual(expectedVC, gotVC) {
			expectedJSON, err := json.Marshal(expectedVC)
			if err != nil {
				t.Fatal("Unable to marshal expected JSON for VC ", name)
			}
			gotJSON, err := json.Marshal(gotVC)
			if err != nil {
				t.Fatal("Unable to marshal got JSON for VC ", name)
			}
			diffs = append(
				diffs, fmt.Sprintf(
					"Storage: pool %s differs:\n\t\t"+
						"Expected: %s\n\t\tGot: %s", name, string(expectedJSON), string(gotJSON),
				),
			)
		}
	}
	for name := range got.Storage {
		if _, ok := expected.Storage[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("Storage: got unexpected VC %s", name))
		}
	}

	// Diff volumes
	expectedVolMap := make(map[string]bool, len(expected.Volumes))
	gotVolMap := make(map[string]bool, len(got.Volumes))
	for _, v := range expected.Volumes {
		expectedVolMap[v] = true
	}
	for _, v := range got.Volumes {
		gotVolMap[v] = true
	}
	for name := range expectedVolMap {
		if _, ok := gotVolMap[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("Volumes: did not get expected volume %s", name))
		}
	}
	for name := range gotVolMap {
		if _, ok := expectedVolMap[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("Volumes: got unexpected volume %s", name))
		}
	}
	if len(diffs) > 0 {
		t.Errorf("External backends differ:\n\t%s", strings.Join(diffs, "\n\t"))
	}
}

// runDeleteTest runs a delete test.
// Parameters:
//   t: the test object
//   d: the delete test to run
//   orchestrator: the orchestrator to use
// Example:
//   d := &deleteTest{name: "test1", expectedSuccess: true}
//   runDeleteTest(t, d, orchestrator)

func runDeleteTest(
	t *testing.T, d *deleteTest, orchestrator *TridentOrchestrator,
) {
	var (
		backendUUID string
		backend     storage.Backend
		found       bool
	)
	if d.expectedSuccess {
		orchestrator.mutex.Lock()

		backendUUID = orchestrator.volumes[d.name].BackendUUID
		backend, found = orchestrator.backends[backendUUID]
		if !found {
			t.Errorf("Backend %v isn't managed by the orchestrator!", backendUUID)
		}
		if _, found = backend.Volumes()[d.name]; !found {
			t.Errorf("Volume %s doesn't exist on backend %s!", d.name, backendUUID)
		}
		orchestrator.mutex.Unlock()
	}
	err := orchestrator.DeleteVolume(ctx(), d.name)
	if err == nil && !d.expectedSuccess {
		t.Errorf("%s: volume delete succeeded when it should not have.", d.name)
	} else if err != nil && d.expectedSuccess {
		t.Errorf("%s: delete failed: %v", d.name, err)
	} else if d.expectedSuccess {
		volume, err := orchestrator.GetVolume(ctx(), d.name)
		if volume != nil || err == nil {
			t.Errorf("%s: got volume where none expected.", d.name)
		}
		orchestrator.mutex.Lock()
		if _, found = backend.Volumes()[d.name]; found {
			t.Errorf("Volume %s shouldn't exist on backend %s!", d.name, backendUUID)
		}
		externalVol, err := orchestrator.storeClient.GetVolume(ctx(), d.name)
		if err != nil {
			if !persistentstore.MatchKeyNotFoundErr(err) {
				t.Errorf(
					"%s: unable to communicate with backing store: "+
						"%v", d.name, err,
				)
			}
			// We're successful if we get to here; we expect an
			// ErrorCodeKeyNotFound.
		} else if externalVol != nil {
			t.Errorf("%s: volume not properly deleted from backing store", d.name)
		}
		orchestrator.mutex.Unlock()
	}
}

type storageClassTest struct {
	config   *storageclass.Config
	expected []*tu.PoolMatch
}

// getOrchestrator returns an orchestrator with a bootstrapped backend
// and a single node.
// Parameters:
//   t - the test object
// Returns:
//   *TridentOrchestrator - the orchestrator
// Example:
//   o := getOrchestrator(t)
//   defer o.Terminate()

func getOrchestrator(t *testing.T) *TridentOrchestrator {
	var (
		storeClient persistentstore.Client
		err         error
	)
	// This will have been created as not nil in init
	// We can't create a new one here because tests that exercise
	// bootstrapping need to have their data persist.
	storeClient = inMemoryClient

	o := NewTridentOrchestrator(storeClient)
	if err = o.Bootstrap(); err != nil {
		t.Fatal("Failure occurred during bootstrapping: ", err)
	}
	return o
}

// validateStorageClass checks that the storage class matches the expected pools.
// Parameters:
//   t: The test object.
//   o: The orchestrator.
//   name: The name of the storage class.
//   expected: A list of expected pool matches.
// Example:
//   validateStorageClass(t, o, "sc1", []*tu.PoolMatch{
//       {BackendName: "be1", PoolName: "pool1"},
//       {BackendName: "be2", PoolName: "pool2"},
//   })

func validateStorageClass(
	t *testing.T,
	o *TridentOrchestrator,
	name string,
	expected []*tu.PoolMatch,
) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	sc, ok := o.storageClasses[name]
	if !ok {
		t.Errorf("%s: Storage class not found in backend.", name)
	}
	remaining := make([]*tu.PoolMatch, len(expected))
	copy(remaining, expected)
	for _, protocol := range []config.Protocol{config.File, config.Block, config.BlockOnFile} {
		for _, pool := range sc.GetStoragePoolsForProtocol(ctx(), protocol, config.ReadWriteOnce) {
			nameFound := false
			for _, scName := range pool.StorageClasses() {
				if scName == name {
					nameFound = true
					break
				}
			}
			if !nameFound {
				t.Errorf("%s: Storage class name not found in storage pool %s", name, pool.Name())
			}
			matchIndex := -1
			for i, r := range remaining {
				if r.Matches(pool) {
					matchIndex = i
					break
				}
			}
			if matchIndex >= 0 {
				// If we match, remove the match from the potential matches.
				remaining[matchIndex] = remaining[len(remaining)-1]
				remaining[len(remaining)-1] = nil
				remaining = remaining[:len(remaining)-1]
			} else {
				t.Errorf("%s: Found unexpected match for storage class: %s:%s", name, pool.Backend().Name(),
					pool.Name())
			}
		}
	}
	if len(remaining) > 0 {
		remainingNames := make([]string, len(remaining))
		for i, r := range remaining {
			remainingNames[i] = r.String()
		}
		t.Errorf("%s: Storage class failed to match storage pools %s", name, strings.Join(remainingNames, ", "))
	}
	persistentSC, err := o.storeClient.GetStorageClass(ctx(), name)
	if err != nil {
		t.Fatalf("Unable to get storage class %s from backend: %v", name, err)
	}
	if !reflect.DeepEqual(
		persistentSC,
		sc.ConstructPersistent(),
	) {
		gotSCJSON, err := json.Marshal(persistentSC)
		if err != nil {
			t.Fatalf("Unable to marshal persisted storage class %s: %v", name, err)
		}
		expectedSCJSON, err := json.Marshal(sc.ConstructPersistent())
		if err != nil {
			t.Fatalf("Unable to marshal expected persistent storage class %s: %v", name, err)
		}
		t.Errorf("%s: Storage class persisted incorrectly.\n\tExpected %s\n\tGot %s", name, expectedSCJSON, gotSCJSON)
	}
}

// This test is fairly heavyweight, but, due to the need to accumulate state
// to run the later tests, it's easier to do this all in one go at the moment.
// Consider breaking this up if it gets unwieldy, though.

// This test is modeled after TestAddStorageClassVolumes, but we don't need all the
// tests around storage class deletion, etc.

// addBackend adds a backend to the orchestrator.
// Parameters:
//   t: the test object
//   orchestrator: the orchestrator to add the backend to
//   backendName: the name of the backend to add
//   backendProtocol: the protocol of the backend to add
// Example:
//   addBackend(t, orchestrator, "fake", config.File)
//   addBackend(t, orchestrator, "fake", config.Block)

func addBackend(
	t *testing.T, orchestrator *TridentOrchestrator, backendName string, backendProtocol config.Protocol,
) {
	volumes := []fake.Volume{
		{"origVolume01", "primary", "primary", 1000000000},
		{"origVolume02", "primary", "primary", 1000000000},
	}
	configJSON, err := fakedriver.NewFakeStorageDriverConfigJSON(
		backendName,
		backendProtocol,
		map[string]*fake.StoragePool{
			"primary": {
				Attrs: map[string]sa.Offer{
					sa.Media:            sa.NewStringOffer("hdd"),
					sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
					// testingAttribute is here to ensure that only one
					// storage class will match this backend.
					sa.TestingAttribute: sa.NewBoolOffer(true),
				},
				Bytes: 100 * 1024 * 1024 * 1024,
			},
		},
		volumes,
	)
	if err != nil {
		t.Fatal("Unable to create mock driver config JSON: ", err)
	}
	_, err = orchestrator.AddBackend(ctx(), configJSON, "")
	if err != nil {
		t.Fatalf("Unable to add initial backend: %v", err)
	}
}

// addBackendStorageClass creates a backend and storage class for tests
// that don't care deeply about this functionality.
func addBackendStorageClass(
	t *testing.T,
	orchestrator *TridentOrchestrator,
	backendName string,
	scName string,
	backendProtocol config.Protocol,
) {
	addBackend(t, orchestrator, backendName, backendProtocol)
	_, err := orchestrator.AddStorageClass(
		ctx(), &storageclass.Config{
			Name: scName,
			Attributes: map[string]sa.Request{
				sa.Media:            sa.NewStringRequest("hdd"),
				sa.ProvisioningType: sa.NewStringRequest("thick"),
				sa.TestingAttribute: sa.NewBoolRequest(true),
			},
		},
	)
	if err != nil {
		t.Fatal("Unable to add storage class: ", err)
	}
}

// captureOutput captures the output of the log package.
// It returns the captured output as a string.
// Parameters:
//     f: function to execute
// It returns the captured output as a string.
// Example:
//     output := captureOutput(func() {
//         log.Println("Hello, world")
//     })
//     fmt.Println(output)

func captureOutput(f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(ioutil.Discard)
	f()
	return buf.String()
}

// TestBackendUpdateAndDelete tests the update and delete functionality of the
// orchestrator.
// It checks that the orchestrator can update a backend with a non-conflicting
// change, and that the backend is correctly updated in the orchestrator and
// persistent store.
// It also checks that the orchestrator can offline a backend, and that the
// backend is correctly offlined in the orchestrator and persistent store.
// Parameters:
//   - New pool: Add a new storage pool to the backend.
//   - Removed pool: Remove a storage pool from the backend.
//   - Expanded offer: Expand the offer of an existing storage pool.
// Example:
//   - New pool:
//     - Starting state:
//         - Backend:
//           - Storage pools: "primary"
//     - Updated state:
//         - Backend:
//           - Storage pools: "primary", "secondary"
//   - Removed pool:
//     - Starting state:
//         - Backend:
//           - Storage pools: "primary", "secondary"
//     - Updated state:
//         - Backend:
//           - Storage pools: "primary"
//   - Expanded offer:
//     - Starting state:
//         - Backend:
//           - Storage pools: "primary"
//           - Storage pool "primary":
//             - Offer: "hdd"
//     - Updated state:
//         - Backend:
//           - Storage pools: "primary"
//           - Storage pool "primary":
//             - Offer: "hdd", "ssd"

func TestBackendUpdateAndDelete(t *testing.T) {
	const (
		backendName       = "updateBackend"
		scName            = "updateBackendTest"
		newSCName         = "updateBackendTest2"
		volumeName        = "updateVolume"
		offlineVolumeName = "offlineVolume"
		backendProtocol   = config.File
	)
	// Test setup
	orchestrator := getOrchestrator(t)
	addBackendStorageClass(t, orchestrator, backendName, scName, backendProtocol)

	orchestrator.mutex.Lock()
	sc, ok := orchestrator.storageClasses[scName]
	if !ok {
		t.Fatal("Storage class not found in orchestrator map")
	}
	orchestrator.mutex.Unlock()

	_, err := orchestrator.AddVolume(ctx(), tu.GenerateVolumeConfig(volumeName, 50, scName, config.File))
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}

	orchestrator.mutex.Lock()
	volume, ok := orchestrator.volumes[volumeName]
	if !ok {
		t.Fatalf("Volume %s not tracked by the orchestrator!", volumeName)
	}
	log.WithFields(
		log.Fields{
			"volume.BackendUUID": volume.BackendUUID,
			"volume.Config.Name": volume.Config.Name,
			"volume.Config":      volume.Config,
		},
	).Debug("Found volume.")
	startingBackend, err := orchestrator.getBackendByBackendName(backendName)
	if startingBackend == nil || err != nil {
		t.Fatalf("Backend %s not stored in orchestrator", backendName)
	}
	if _, ok = startingBackend.Volumes()[volumeName]; !ok {
		t.Fatalf("Volume %s not tracked by the backend %s!", volumeName, backendName)
	}
	orchestrator.mutex.Unlock()

	// Test updates that should succeed
	previousBackends := make([]storage.Backend, 1)
	previousBackends[0] = startingBackend
	for _, c := range []struct {
		name  string
		pools map[string]*fake.StoragePool
	}{
		{
			name: "New pool",
			pools: map[string]*fake.StoragePool{
				"primary": {
					Attrs: map[string]sa.Offer{
						sa.Media:            sa.NewStringOffer("hdd"),
						sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
						sa.TestingAttribute: sa.NewBoolOffer(true),
					},
					Bytes: 100 * 1024 * 1024 * 1024,
				},
				"secondary": {
					Attrs: map[string]sa.Offer{
						sa.Media:            sa.NewStringOffer("ssd"),
						sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
						sa.TestingAttribute: sa.NewBoolOffer(true),
					},
					Bytes: 100 * 1024 * 1024 * 1024,
				},
			},
		},
		{
			name: "Removed pool",
			pools: map[string]*fake.StoragePool{
				"primary": {
					Attrs: map[string]sa.Offer{
						sa.Media:            sa.NewStringOffer("hdd"),
						sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
						sa.TestingAttribute: sa.NewBoolOffer(true),
					},
					Bytes: 100 * 1024 * 1024 * 1024,
				},
			},
		},
		{
			name: "Expanded offer",
			pools: map[string]*fake.StoragePool{
				"primary": {
					Attrs: map[string]sa.Offer{
						sa.Media:            sa.NewStringOffer("ssd", "hdd"),
						sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
						sa.TestingAttribute: sa.NewBoolOffer(true),
					},
					Bytes: 100 * 1024 * 1024 * 1024,
				},
			},
		},
	} {
		// Make sure we're starting with an active backend
		previousBackend, err := orchestrator.getBackendByBackendName(backendName)
		if previousBackend == nil || err != nil {
			t.Fatalf("Backend %s not stored in orchestrator", backendName)
		}
		if !previousBackend.Driver().Initialized() {
			t.Errorf("Backend %s is not initialized", backendName)
		}

		var volumes []fake.Volume
		newConfigJSON, err := fakedriver.NewFakeStorageDriverConfigJSON(
			backendName,
			config.File, c.pools, volumes,
		)
		if err != nil {
			t.Errorf("%s: unable to generate new backend config: %v", c.name, err)
			continue
		}

		_, err = orchestrator.UpdateBackend(ctx(), backendName, newConfigJSON, "")

		if err != nil {
			t.Errorf("%s: unable to update backend with a nonconflicting change: %v", c.name, err)
			continue
		}

		orchestrator.mutex.Lock()
		newBackend, err := orchestrator.getBackendByBackendName(backendName)
		if newBackend == nil || err != nil {
			t.Fatalf("Backend %s not stored in orchestrator", backendName)
		}
		if previousBackend.Driver().Initialized() {
			t.Errorf("Previous backend %s still initialized", backendName)
		}
		if !newBackend.Driver().Initialized() {
			t.Errorf("Updated backend %s is not initialized.", backendName)
		}
		pools := sc.GetStoragePoolsForProtocol(ctx(), config.File, config.ReadWriteMany)
		foundNewBackend := false
		for _, pool := range pools {
			for i, b := range previousBackends {
				if pool.Backend() == b {
					t.Errorf(
						"%s: backend %d not cleared from storage class",
						c.name, i+1,
					)
				}
				if pool.Backend() == newBackend {
					foundNewBackend = true
				}
			}
		}
		if !foundNewBackend {
			t.Errorf("%s: Storage class does not point to new backend.", c.name)
		}
		matchingPool, ok := newBackend.Storage()["primary"]
		if !ok {
			t.Errorf("%s: storage pool for volume not found", c.name)
			continue
		}
		if len(matchingPool.StorageClasses()) != 1 {
			t.Errorf("%s: unexpected number of storage classes for main storage pool: %d", c.name,
				len(matchingPool.StorageClasses()))
		}
		volumeBackend, err := orchestrator.getBackendByBackendUUID(volume.BackendUUID)
		if volumeBackend == nil || err != nil {
			for backendUUID, backend := range orchestrator.backends {
				log.WithFields(
					log.Fields{
						"volume.BackendUUID":  volume.BackendUUID,
						"backend":             backend,
						"backend.BackendUUID": backend.BackendUUID(),
						"uuid":                backendUUID,
					},
				).Debug("Found backend.")
			}
			t.Fatalf("Backend %s not stored in orchestrator, err: %v", volume.BackendUUID, err)
		}
		if volumeBackend != newBackend {
			t.Errorf("%s: volume backend does not point to the new backend", c.name)
		}
		if volume.Pool != matchingPool.Name() {
			t.Errorf("%s: volume does not point to the right storage pool.", c.name)
		}
		persistentBackend, err := orchestrator.storeClient.GetBackend(ctx(), backendName)
		if err != nil {
			t.Error("Unable to retrieve backend from store client: ", err)
		} else if !cmp.Equal(newBackend.ConstructPersistent(ctx()), persistentBackend) {
			if diff := cmp.Diff(newBackend.ConstructPersistent(ctx()), persistentBackend); diff != "" {
				t.Errorf("Failure for %v; mismatch (-want +got):\n%s", c.name, diff)
			}
			t.Errorf("Backend not correctly updated in persistent store.")
		}
		previousBackends = append(previousBackends, newBackend)
		orchestrator.mutex.Unlock()
	}

	backend := previousBackends[len(previousBackends)-1]
	pool := volume.Pool

	// Test backend offlining.
	err = orchestrator.DeleteBackend(ctx(), backendName)
	if err != nil {
		t.Fatalf("Unable to delete backend: %v", err)
	}
	if !backend.Driver().Initialized() {
		t.Errorf("Deleted backend with volumes %s is not initialized.", backendName)
	}
	_, err = orchestrator.AddVolume(ctx(), tu.GenerateVolumeConfig(offlineVolumeName, 50, scName, config.File))
	if err == nil {
		t.Error("Created volume volume on offline backend.")
	}
	orchestrator.mutex.Lock()
	pools := sc.GetStoragePoolsForProtocol(ctx(), config.File, config.ReadWriteOnce)
	if len(pools) == 1 {
		t.Error("Offline backend not removed from storage pool in storage class.")
	}
	foundBackend, err := orchestrator.getBackendByBackendUUID(volume.BackendUUID)
	if err != nil {
		t.Errorf("Couldn't find backend: %v", err)
	}
	if foundBackend != backend {
		t.Error("Backend changed for volume after offlining.")
	}
	if volume.Pool != pool {
		t.Error("Storage pool changed for volume after backend offlined.")
	}
	persistentBackend, err := orchestrator.storeClient.GetBackend(ctx(), backendName)
	if err != nil {
		t.Errorf("Unable to retrieve backend from store client after offlining: %v", err)
	} else if persistentBackend.State.IsOnline() {
		t.Error("Online not set to true in the backend.")
	}
	orchestrator.mutex.Unlock()

	// Ensure that new storage classes do not get the offline backend assigned
	// to them.
	newSCExternal, err := orchestrator.AddStorageClass(
		ctx(), &storageclass.Config{
			Name: newSCName,
			Attributes: map[string]sa.Request{
				sa.Media:            sa.NewStringRequest("hdd"),
				sa.TestingAttribute: sa.NewBoolRequest(true),
			},
		},
	)
	if err != nil {
		t.Fatal("Unable to add new storage class after offlining: ", err)
	}
	if _, ok = newSCExternal.StoragePools[backendName]; ok {
		t.Error("Offline backend added to new storage class.")
	}

	// Test that online gets set properly after bootstrapping.
	newOrchestrator := getOrchestrator(t)
	// We need to lock the orchestrator mutex here because we call
	// ConstructExternal on the original backend in the else if clause.
	orchestrator.mutex.Lock()
	if bootstrappedBackend, _ := newOrchestrator.GetBackend(ctx(), backendName); bootstrappedBackend == nil {
		t.Error("Unable to find backend after bootstrapping.")
	} else if !reflect.DeepEqual(bootstrappedBackend, backend.ConstructExternal(ctx())) {
		diffExternalBackends(t, backend.ConstructExternal(ctx()), bootstrappedBackend)
	}

	orchestrator.mutex.Unlock()

	newOrchestrator.mutex.Lock()
	for _, name := range []string{scName, newSCName} {
		newSC, ok := newOrchestrator.storageClasses[name]
		if !ok {
			t.Fatalf(
				"Unable to find storage class %s after bootstrapping.",
				name,
			)
		}
		pools = newSC.GetStoragePoolsForProtocol(ctx(), config.File, config.ReadOnlyMany)
		if len(pools) == 1 {
			t.Errorf(
				"Offline backend readded to storage class %s after "+
					"bootstrapping.", name,
			)
		}
	}
	newOrchestrator.mutex.Unlock()

	// Test that deleting the volume causes the backend to be deleted.
	err = orchestrator.DeleteVolume(ctx(), volumeName)
	if err != nil {
		t.Fatal("Unable to delete volume for offline backend: ", err)
	}
	if backend.Driver().Initialized() {
		t.Errorf("Deleted backend %s is still initialized.", backendName)
	}
	persistentBackend, err = orchestrator.storeClient.GetBackend(ctx(), backendName)
	if err == nil {
		t.Error(
			"Backend remained on store client after deleting the last " +
				"volume present.",
		)
	}
	orchestrator.mutex.Lock()

	missingBackend, _ := orchestrator.getBackendByBackendName(backendName)
	if missingBackend != nil {
		t.Error("Empty offlined backend not removed from memory.")
	}
	orchestrator.mutex.Unlock()
	cleanup(t, orchestrator)
}

// backendPasswordsInLogsHelper is a helper function for backendPasswordsInLogs
// and backendPasswordsInLogsWithDebugTraceFlags tests.
// Parameters:
//   t: test object
//   debugTraceFlags: map of debug trace flags
// Example:
//   backendPasswordsInLogsHelper(t, map[string]bool{
//       "orchestrator": true,
//   })

func backendPasswordsInLogsHelper(t *testing.T, debugTraceFlags map[string]bool) {

	backendName := "passwordBackend"
	backendProtocol := config.File

	orchestrator := getOrchestrator(t)

	fakeConfig, err := fakedriver.NewFakeStorageDriverConfigJSONWithDebugTraceFlags(
		backendName, backendProtocol,
		debugTraceFlags, "prefix1_",
	)
	if err != nil {
		t.Fatalf("Unable to generate config JSON for %s: %v", backendName, err)
	}

	_, err = orchestrator.AddBackend(ctx(), fakeConfig, "")
	if err != nil {
		t.Errorf("Unable to add backend %s: %v", backendName, err)
	}

	newConfigJSON, err := fakedriver.NewFakeStorageDriverConfigJSONWithDebugTraceFlags(
		backendName, backendProtocol,
		debugTraceFlags, "prefix2_",
	)
	if err != nil {
		t.Errorf("%s: unable to generate new backend config: %v", backendName, err)
	}

	output := captureOutput(
		func() {
			_, err = orchestrator.UpdateBackend(ctx(), backendName, newConfigJSON, "")
		},
	)

	if err != nil {
		t.Errorf("%s: unable to update backend with a nonconflicting change: %v", backendName, err)
	}

	assert.Contains(t, output, "configJSON")
	outputArr := strings.Split(output, "configJSON")
	outputArr = strings.Split(outputArr[1], "=\"")
	outputArr = strings.Split(outputArr[1], "\"")

	assert.Equal(t, outputArr[0], "<suppressed>")
	cleanup(t, orchestrator)
}

// TestBackendPasswordsInLogs tests that backend passwords are not logged
// when the "method" field is present in the config.
// It checks that backend passwords are logged when the "method" field is not present.
// Parameters:
//   t: testing.T
//   config: map[string]bool
//     "method": true if the "method" field is present in the config
// Example:
//   backendPasswordsInLogsHelper(t, map[string]bool{"method": true})

func TestBackendPasswordsInLogs(t *testing.T) {
	backendPasswordsInLogsHelper(t, nil)
	backendPasswordsInLogsHelper(t, map[string]bool{"method": true})
}

// TestEmptyBackendDeletion tests that an empty backend can be deleted.
// It checks that the backend is removed from the orchestrator and the
// store client.
// Parameters:
//   backendName: The name of the backend to be deleted.
//   backendProtocol: The protocol of the backend to be deleted.
// Example:
//   TestEmptyBackendDeletion(t, "emptyBackend", config.File)

func TestEmptyBackendDeletion(t *testing.T) {
	const (
		backendName     = "emptyBackend"
		backendProtocol = config.File
	)

	orchestrator := getOrchestrator(t)
	// Note that we don't care about the storage class here, but it's easier
	// to reuse functionality.
	addBackendStorageClass(t, orchestrator, backendName, "none", backendProtocol)
	backend, errLookup := orchestrator.getBackendByBackendName(backendName)
	if backend == nil || errLookup != nil {
		t.Fatalf("Backend %s not stored in orchestrator", backendName)
	}

	err := orchestrator.DeleteBackend(ctx(), backendName)
	if err != nil {
		t.Fatalf("Unable to delete backend: %v", err)
	}
	if backend.Driver().Initialized() {
		t.Errorf("Deleted backend %s is still initialized.", backendName)
	}
	_, err = orchestrator.storeClient.GetBackend(ctx(), backendName)
	if err == nil {
		t.Error("Empty backend remained on store client after offlining")
	}
	orchestrator.mutex.Lock()
	missingBackend, _ := orchestrator.getBackendByBackendName(backendName)
	if missingBackend != nil {
		t.Error("Empty offlined backend not removed from memory.")
	}
	orchestrator.mutex.Unlock()
	cleanup(t, orchestrator)
}

// TestBootstrapSnapshotMissingVolume tests that a snapshot in missing_volume state is bootstrapped correctly.
// It checks that the snapshot is in the correct state and that it can be deleted.
// Parameters:
//   offlineBackendName: Name of the backend to use for the test
//   scName: Name of the storage class to use for the test
//   volumeName: Name of the volume to use for the test
//   snapName: Name of the snapshot to use for the test
//   backendProtocol: Protocol of the backend to use for the test
// Example:
//   TestBootstrapSnapshotMissingVolume(t, "snapNoVolBackend", "snapNoVolSC", "snapNoVolVolume", "snapNoVolSnapshot", config.File)

func TestBootstrapSnapshotMissingVolume(t *testing.T) {
	const (
		offlineBackendName = "snapNoVolBackend"
		scName             = "snapNoVolSC"
		volumeName         = "snapNoVolVolume"
		snapName           = "snapNoVolSnapshot"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator(t)
	defer cleanup(t, orchestrator)
	addBackendStorageClass(t, orchestrator, offlineBackendName, scName, backendProtocol)
	_, err := orchestrator.AddVolume(
		ctx(), tu.GenerateVolumeConfig(
			volumeName, 50,
			scName, config.File,
		),
	)
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}

	// For the full test, we create everything and recreate the AddSnapshot transaction.
	snapshotConfig := generateSnapshotConfig(snapName, volumeName, volumeName)
	if _, err := orchestrator.CreateSnapshot(ctx(), snapshotConfig); err != nil {
		t.Fatal("Unable to add snapshot: ", err)
	}

	// Simulate deleting the existing volume without going through Trident then bootstrapping
	vol, ok := orchestrator.volumes[volumeName]
	if !ok {
		t.Fatalf("Unable to find volume %s in backend.", volumeName)
	}
	orchestrator.mutex.Lock()
	err = orchestrator.storeClient.DeleteVolume(ctx(), vol)
	if err != nil {
		t.Fatalf("Unable to delete volume from store: %v", err)
	}
	orchestrator.mutex.Unlock()

	newOrchestrator := getOrchestrator(t)
	bootstrappedSnapshot, err := newOrchestrator.GetSnapshot(ctx(), snapshotConfig.VolumeName, snapshotConfig.Name)
	if err != nil {
		t.Fatalf("error getting snapshot: %v", err)
	}
	if bootstrappedSnapshot == nil {
		t.Error("Volume not found during bootstrap.")
	}
	if !bootstrappedSnapshot.State.IsMissingVolume() {
		t.Error("Unexpected snapshot state.")
	}
	// Delete volume in missing_volume state
	err = newOrchestrator.DeleteSnapshot(ctx(), volumeName, snapName)
	if err != nil {
		t.Error("could not delete snapshot with missing volume")
	}
}

// TestBootstrapSnapshotMissingBackend tests that a snapshot in missing_backend state is bootstrapped correctly.
// It checks that the snapshot is bootstrapped and that it is in the correct state.
// Parameters:
//   offlineBackendName - name of the backend that will be deleted
//   scName - name of the storage class
//   volumeName - name of the volume
//   snapName - name of the snapshot
// Example:
//   TestBootstrapSnapshotMissingBackend(t, "snapNoBackBackend", "snapNoBackSC", "snapNoBackVolume", "snapNoBackSnapshot")

func TestBootstrapSnapshotMissingBackend(t *testing.T) {
	const (
		offlineBackendName = "snapNoBackBackend"
		scName             = "snapNoBackSC"
		volumeName         = "snapNoBackVolume"
		snapName           = "snapNoBackSnapshot"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator(t)
	defer cleanup(t, orchestrator)
	addBackendStorageClass(t, orchestrator, offlineBackendName, scName, backendProtocol)
	_, err := orchestrator.AddVolume(
		ctx(), tu.GenerateVolumeConfig(
			volumeName, 50,
			scName, config.File,
		),
	)
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}

	// For the full test, we create everything and recreate the AddSnapshot transaction.
	snapshotConfig := generateSnapshotConfig(snapName, volumeName, volumeName)
	if _, err := orchestrator.CreateSnapshot(ctx(), snapshotConfig); err != nil {
		t.Fatal("Unable to add snapshot: ", err)
	}

	// Simulate deleting the existing backend without going through Trident then bootstrapping
	backend, err := orchestrator.getBackendByBackendName(offlineBackendName)
	if err != nil {
		t.Fatalf("Unable to get backend from store: %v", err)
	}
	orchestrator.mutex.Lock()
	err = orchestrator.storeClient.DeleteBackend(ctx(), backend)
	if err != nil {
		t.Fatalf("Unable to delete volume from store: %v", err)
	}
	orchestrator.mutex.Unlock()

	newOrchestrator := getOrchestrator(t)
	bootstrappedSnapshot, err := newOrchestrator.GetSnapshot(ctx(), snapshotConfig.VolumeName, snapshotConfig.Name)
	if err != nil {
		t.Fatalf("error getting snapshot: %v", err)
	}
	if bootstrappedSnapshot == nil {
		t.Error("Volume not found during bootstrap.")
	}
	if !bootstrappedSnapshot.State.IsMissingBackend() {
		t.Error("Unexpected snapshot state.")
	}
	// Delete snapshot in missing_backend state
	err = newOrchestrator.DeleteSnapshot(ctx(), volumeName, snapName)
	if err != nil {
		t.Error("could not delete snapshot with missing backend")
	}
}

// TestBootstrapVolumeMissingBackend tests that a volume in missing_backend state is bootstrapped correctly
// It checks that the volume is in the correct state and can be deleted
// Parameters:
//   offlineBackendName - name of the backend to be deleted
//   scName - name of the storage class to be created
//   volumeName - name of the volume to be created
// Example:
//   TestBootstrapVolumeMissingBackend("bootstrapVolBackend", "bootstrapVolSC", "bootstrapVolVolume")

func TestBootstrapVolumeMissingBackend(t *testing.T) {
	const (
		offlineBackendName = "bootstrapVolBackend"
		scName             = "bootstrapVolSC"
		volumeName         = "bootstrapVolVolume"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator(t)
	defer cleanup(t, orchestrator)
	addBackendStorageClass(t, orchestrator, offlineBackendName, scName, backendProtocol)
	_, err := orchestrator.AddVolume(
		ctx(), tu.GenerateVolumeConfig(
			volumeName, 50,
			scName, config.File,
		),
	)
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}

	// Simulate deleting the existing backend without going through Trident then bootstrapping
	backend, err := orchestrator.getBackendByBackendName(offlineBackendName)
	if err != nil {
		t.Fatalf("Unable to get backend from store: %v", err)
	}
	orchestrator.mutex.Lock()
	err = orchestrator.storeClient.DeleteBackend(ctx(), backend)
	if err != nil {
		t.Fatalf("Unable to delete volume from store: %v", err)
	}
	orchestrator.mutex.Unlock()

	newOrchestrator := getOrchestrator(t)
	bootstrappedVolume, err := newOrchestrator.GetVolume(ctx(), volumeName)
	if err != nil {
		t.Fatalf("error getting volume: %v", err)
	}
	if bootstrappedVolume == nil {
		t.Error("volume not found during bootstrap")
	}
	if !bootstrappedVolume.State.IsMissingBackend() {
		t.Error("unexpected volume state")
	}

	// Delete volume in missing_backend state
	err = newOrchestrator.DeleteVolume(ctx(), volumeName)
	if err != nil {
		t.Error("could not delete volume with missing backend")
	}
}

// TestBackendCleanup tests that empty offline backends are cleaned up during bootstrap.
// It checks that empty online backends are not cleaned up during bootstrap.
// Parameters:
//   offlineBackendName: Name of the backend to be offlined.
//   onlineBackendName: Name of the backend to be left online.
//   scName: Name of the storage class to be used.
//   volumeName: Name of the volume to be created.
//   backendProtocol: Protocol of the backends to be created.
// Example:
//   TestBackendCleanup(t, "cleanupBackend", "onlineBackend", "cleanupBackendTest", "cleanupVolume", config.File)

func TestBackendCleanup(t *testing.T) {
	const (
		offlineBackendName = "cleanupBackend"
		onlineBackendName  = "onlineBackend"
		scName             = "cleanupBackendTest"
		volumeName         = "cleanupVolume"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator(t)
	addBackendStorageClass(t, orchestrator, offlineBackendName, scName, backendProtocol)
	_, err := orchestrator.AddVolume(
		ctx(), tu.GenerateVolumeConfig(
			volumeName, 50,
			scName, config.File,
		),
	)
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}

	// This needs to go after the volume addition to ensure that the volume
	// ends up on the backend to be offlined.
	addBackend(t, orchestrator, onlineBackendName, backendProtocol)

	err = orchestrator.DeleteBackend(ctx(), offlineBackendName)
	if err != nil {
		t.Fatalf("Unable to delete backend %s: %v", offlineBackendName, err)
	}
	// Simulate deleting the existing volume and then bootstrapping
	orchestrator.mutex.Lock()
	vol, ok := orchestrator.volumes[volumeName]
	if !ok {
		t.Fatalf("Unable to find volume %s in backend.", volumeName)
	}
	err = orchestrator.storeClient.DeleteVolume(ctx(), vol)
	if err != nil {
		t.Fatalf("Unable to delete volume from store: %v", err)
	}
	orchestrator.mutex.Unlock()

	newOrchestrator := getOrchestrator(t)
	if bootstrappedBackend, _ := newOrchestrator.GetBackend(ctx(), offlineBackendName); bootstrappedBackend != nil {
		t.Error("Empty offline backend not deleted during bootstrap.")
	}
	if bootstrappedBackend, _ := newOrchestrator.GetBackend(ctx(), onlineBackendName); bootstrappedBackend == nil {
		t.Error("Empty online backend deleted during bootstrap.")
	}
}

// TestLoadBackend tests that a backend can be loaded from a config and that it matches the original.
// It checks that the backend is also loaded after bootstrapping.
// Parameters:
//    t *testing.T
// Example:
//    TestLoadBackend(t)

func TestLoadBackend(t *testing.T) {
	const (
		backendName = "load-backend-test"
	)
	// volumes must be nil in order to satisfy reflect.DeepEqual comparison. It isn't recommended to compare slices with deepEqual
	var volumes []fake.Volume
	orchestrator := getOrchestrator(t)
	configJSON, err := fakedriver.NewFakeStorageDriverConfigJSON(
		backendName,
		config.File,
		map[string]*fake.StoragePool{
			"primary": {
				Attrs: map[string]sa.Offer{
					sa.Media:            sa.NewStringOffer("hdd"),
					sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
					sa.TestingAttribute: sa.NewBoolOffer(true),
				},
				Bytes: 100 * 1024 * 1024 * 1024,
			},
		},
		volumes,
	)
	originalBackend, err := orchestrator.AddBackend(ctx(), configJSON, "")
	if err != nil {
		t.Fatal("Unable to initially add backend: ", err)
	}
	persistentBackend, err := orchestrator.storeClient.GetBackend(ctx(), backendName)
	if err != nil {
		t.Fatal("Unable to retrieve backend from store client: ", err)
	}
	// Note that this will register as an update, but it should be close enough
	newConfig, err := persistentBackend.MarshalConfig()
	if err != nil {
		t.Fatal("Unable to marshal config from stored backend: ", err)
	}
	newBackend, err := orchestrator.AddBackend(ctx(), newConfig, "")
	if err != nil {
		t.Error("Unable to update backend from config: ", err)
	} else if !reflect.DeepEqual(newBackend, originalBackend) {
		t.Error("Newly loaded backend differs.")
	}

	newOrchestrator := getOrchestrator(t)
	if bootstrappedBackend, _ := newOrchestrator.GetBackend(ctx(), backendName); bootstrappedBackend == nil {
		t.Error("Unable to find backend after bootstrapping.")
	} else if !reflect.DeepEqual(bootstrappedBackend, originalBackend) {
		t.Errorf("External backends differ.")
		diffExternalBackends(t, originalBackend, bootstrappedBackend)
	}
	cleanup(t, orchestrator)
}

// prepRecoveryTest sets up a backend and storage class for testing recovery
// Parameters:
//   t: test object
//   orchestrator: orchestrator object
//   backendName: name of backend to create
//   scName: name of storage class to create
// Example:
//   prepRecoveryTest(t, orchestrator, "ontap-nas-test", "ontap-nas-sc")

func prepRecoveryTest(
	t *testing.T, orchestrator *TridentOrchestrator, backendName, scName string,
) {
	configJSON, err := fakedriver.NewFakeStorageDriverConfigJSON(
		backendName,
		config.File,
		map[string]*fake.StoragePool{
			"primary": {
				Attrs: map[string]sa.Offer{
					sa.Media:            sa.NewStringOffer("hdd"),
					sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
					sa.RecoveryTest:     sa.NewBoolOffer(true),
				},
				Bytes: 100 * 1024 * 1024 * 1024,
			},
		},
		[]fake.Volume{},
	)
	_, err = orchestrator.AddBackend(ctx(), configJSON, "")
	if err != nil {
		t.Fatal("Unable to initialize backend: ", err)
	}
	_, err = orchestrator.AddStorageClass(
		ctx(), &storageclass.Config{
			Name: scName,
			Attributes: map[string]sa.Request{
				sa.Media:            sa.NewStringRequest("hdd"),
				sa.ProvisioningType: sa.NewStringRequest("thick"),
				sa.RecoveryTest:     sa.NewBoolRequest(true),
			},
		},
	)
	if err != nil {
		t.Fatal("Unable to add storage class: ", err)
	}
}

// runRecoveryTests runs a set of tests to verify that the orchestrator
// recovers from a partially completed volume addition.
// Parameters:
//   t: Test object
//   orchestrator: The orchestrator to test
//   backendName: The name of the backend to use
//   op: The operation to test
//   testCases: The set of test cases to run
// Example:
//   runRecoveryTests(t, orchestrator, "fake", storage.AddVolume,
//     []recoveryTest{
//       {
//         "volumeConfig",
//         &storage.VolumeConfig{
//           Name: "test",
//           Size: "10GiB",
//           Attributes: map[string]sa.Request{},
//         },
//         true,
//       },
//       ...
//     })

func runRecoveryTests(
	t *testing.T,
	orchestrator *TridentOrchestrator,
	backendName string,
	op storage.VolumeOperation,
	testCases []recoveryTest,
) {
	for _, c := range testCases {
		// Manipulate the persistent store directly, since it's
		// easier to store the results of a partially completed volume addition
		// than to actually inject a failure.
		volTxn := &storage.VolumeTransaction{
			Config: c.volumeConfig,
			Op:     op,
		}
		err := orchestrator.storeClient.AddVolumeTransaction(ctx(), volTxn)
		if err != nil {
			t.Fatalf("%s: Unable to create volume transaction: %v", c.name, err)
		}
		newOrchestrator := getOrchestrator(t)
		newOrchestrator.mutex.Lock()
		if _, ok := newOrchestrator.volumes[c.volumeConfig.Name]; ok {
			t.Errorf("%s: volume still present in orchestrator.", c.name)
			// Note: assume that if the volume's still present in the
			// top-level map, it's present everywhere else and that, if it's
			// absent there, it's absent everywhere else in memory
		}
		backend, err := newOrchestrator.getBackendByBackendName(backendName)
		if backend == nil || err != nil {
			t.Fatalf("%s: Backend not found after bootstrapping.", c.name)
		}
		f, ok := backend.Driver().(*fakedriver.StorageDriver)
		if !ok {
			t.Fatalf("%e", utils.TypeAssertionError("backend.Driver().(*fakedriver.StorageDriver)"))
		}
		// Destroy should be always called on the backend
		if _, ok := f.DestroyedVolumes[f.GetInternalVolumeName(ctx(), c.volumeConfig.Name)]; !ok && c.expectDestroy {
			t.Errorf("%s: Destroy not called on volume.", c.name)
		}
		_, err = newOrchestrator.storeClient.GetVolume(ctx(), c.volumeConfig.Name)
		if err != nil {
			if !persistentstore.MatchKeyNotFoundErr(err) {
				t.Errorf("%s: unable to communicate with backing store: %v", c.name, err)
			}
		} else {
			t.Errorf("%s: Found VolumeConfig still stored in store.", c.name)
		}
		if txns, err := newOrchestrator.storeClient.GetVolumeTransactions(ctx()); err != nil {
			t.Errorf("%s: Unable to retrieve transactions from backing store: %v", c.name, err)
		} else if len(txns) > 0 {
			t.Errorf("%s: Transaction not cleared from the backing store", c.name)
		}
		newOrchestrator.mutex.Unlock()
	}
}

// TestAddVolumeRecovery tests that adding a volume is properly recovered
// after a crash.
// It checks that the volume is properly destroyed if the transaction was
// not committed, and that the volume is properly created if the transaction
// was committed.
// Parameters:
//   - full: the volume was created and the transaction was committed
//   - txOnly: the volume was not created and the transaction was not committed
// Example:
//   - TestAddVolumeRecovery(full)
//   - TestAddVolumeRecovery(txOnly)

func TestAddVolumeRecovery(t *testing.T) {
	const (
		backendName      = "addRecoveryBackend"
		scName           = "addRecoveryBackendSC"
		fullVolumeName   = "addRecoveryVolumeFull"
		txOnlyVolumeName = "addRecoveryVolumeTxOnly"
	)
	orchestrator := getOrchestrator(t)
	prepRecoveryTest(t, orchestrator, backendName, scName)
	// It's easier to add the volume and then reinject the transaction begin
	// afterwards
	fullVolumeConfig := tu.GenerateVolumeConfig(fullVolumeName, 50, scName, config.File)
	_, err := orchestrator.AddVolume(ctx(), fullVolumeConfig)
	if err != nil {
		t.Fatal("Unable to add volume: ", err)
	}
	txOnlyVolumeConfig := tu.GenerateVolumeConfig(txOnlyVolumeName, 50, scName, config.File)
	// BEGIN actual test
	runRecoveryTests(
		t, orchestrator, backendName, storage.AddVolume,
		[]recoveryTest{
			{name: "full", volumeConfig: fullVolumeConfig, expectDestroy: true},
			{name: "txOnly", volumeConfig: txOnlyVolumeConfig, expectDestroy: true},
		},
	)
	cleanup(t, orchestrator)
}

// TestAddVolumeWithTMRNonONTAPNAS tests that a volume with a relationship
// annotation that is not ONTAP NAS is rejected
// It checks that the volume is rejected
// Parameters:
//    backendName: name of the backend
//    scName: name of the storage class
//    fullVolumeName: name of the full volume
// Example:
//    TestAddVolumeWithTMRNonONTAPNAS("be1", "sc1", "vol1")

func TestAddVolumeWithTMRNonONTAPNAS(t *testing.T) {
	// Add a single backend of fake
	// create volume with relationship annotation added
	// witness failure
	const (
		backendName    = "addRecoveryBackend"
		scName         = "addRecoveryBackendSC"
		fullVolumeName = "addRecoveryVolumeFull"
	)
	orchestrator := getOrchestrator(t)
	prepRecoveryTest(t, orchestrator, backendName, scName)
	// It's easier to add the volume and then reinject the transaction begin
	// afterwards
	fullVolumeConfig := tu.GenerateVolumeConfig(
		fullVolumeName, 50, scName,
		config.File,
	)
	fullVolumeConfig.PeerVolumeHandle = "fakesvm:fakevolume"
	fullVolumeConfig.IsMirrorDestination = true
	_, err := orchestrator.AddVolume(ctx(), fullVolumeConfig)
	if err == nil || !strings.Contains(err.Error(), "no suitable") {
		t.Fatal("Unexpected failure")
	}
	cleanup(t, orchestrator)
}

// TestDeleteVolumeRecovery tests the recovery of a volume delete operation.
// It checks that a volume that was deleted but not fully committed is
// recovered, and that a volume that was only created in the transaction
// is cleaned up.
// Parameters:
//   backendName: The name of the backend to use for the test.
//   scName: The name of the storage class to use for the test.
// Example:
//   TestDeleteVolumeRecovery(t, "ontap-san", "ontap-san-sc")

func TestDeleteVolumeRecovery(t *testing.T) {
	const (
		backendName      = "deleteRecoveryBackend"
		scName           = "deleteRecoveryBackendSC"
		fullVolumeName   = "deleteRecoveryVolumeFull"
		txOnlyVolumeName = "deleteRecoveryVolumeTxOnly"
	)
	orchestrator := getOrchestrator(t)
	prepRecoveryTest(t, orchestrator, backendName, scName)

	// For the full test, we delete everything but the ending transaction.
	fullVolumeConfig := tu.GenerateVolumeConfig(fullVolumeName, 50, scName, config.File)
	if _, err := orchestrator.AddVolume(ctx(), fullVolumeConfig); err != nil {
		t.Fatal("Unable to add volume: ", err)
	}
	if err := orchestrator.DeleteVolume(ctx(), fullVolumeName); err != nil {
		t.Fatal("Unable to remove full volume: ", err)
	}

	txOnlyVolumeConfig := tu.GenerateVolumeConfig(txOnlyVolumeName, 50, scName, config.File)
	if _, err := orchestrator.AddVolume(ctx(), txOnlyVolumeConfig); err != nil {
		t.Fatal("Unable to add tx only volume: ", err)
	}

	// BEGIN actual test
	runRecoveryTests(
		t, orchestrator, backendName,
		storage.DeleteVolume, []recoveryTest{
			{name: "full", volumeConfig: fullVolumeConfig, expectDestroy: false},
			{name: "txOnly", volumeConfig: txOnlyVolumeConfig, expectDestroy: true},
		},
	)
	cleanup(t, orchestrator)
}

// generateSnapshotConfig generates a SnapshotConfig object
// Parameters:
//   name - the name of the snapshot
//   volumeName - the name of the volume
//   volumeInternalName - the internal name of the volume
// Returns:
//   a SnapshotConfig object
// It returns an error if the volume name is not set
// Example:
//   config := generateSnapshotConfig("snap1", "vol1", "pvc-xxx-xx-xx")

func generateSnapshotConfig(
	name, volumeName, volumeInternalName string,
) *storage.SnapshotConfig {
	return &storage.SnapshotConfig{
		Version:            config.OrchestratorAPIVersion,
		Name:               name,
		VolumeName:         volumeName,
		VolumeInternalName: volumeInternalName,
	}
}

// runSnapshotRecoveryTests runs a set of recovery tests.
// Parameters:
//   t: the test object
//   orchestrator: the orchestrator to use for the tests
//   backendName: the name of the backend to use for the tests
//   op: the operation to test
//   testCases: the set of test cases to run
// Example:
//   runSnapshotRecoveryTests(
//     t,
//     orchestrator,
//     "ontap-nas",
//     storage.AddSnapshot,
//     []recoveryTest{
//       {
//         name: "VolumePresent",
//         volumeConfig: &storage.VolumeConfig{
//           Version:      tridentconfig.OrchestratorAPIVersion,
//           Name:         "myvol",
//           BackendUUID:  "ontap-nas-0",
//           Backend:      "ontap-nas",
//           Size:         "1GiB",
//           InternalName: "myvol",
//           Attributes:   map[string]sa.Request{},
//         },
//         snapshotConfig: &storage.SnapshotConfig{
//           Version:      tridentconfig.OrchestratorAPIVersion,
//           Name:         "mysnap",
//           VolumeName:   "myvol",
//           VolumeInternalName: "myvol",
//           Attributes:   map[string]sa.Request{},
//         },
//         expectDestroy: true,
//       },
//     },
//   )

func runSnapshotRecoveryTests(
	t *testing.T,
	orchestrator *TridentOrchestrator,
	backendName string,
	op storage.VolumeOperation,
	testCases []recoveryTest,
) {
	for _, c := range testCases {
		// Manipulate the persistent store directly, since it's
		// easier to store the results of a partially completed snapshot addition
		// than to actually inject a failure.
		volTxn := &storage.VolumeTransaction{
			Config:         c.volumeConfig,
			SnapshotConfig: c.snapshotConfig,
			Op:             op,
		}
		if err := orchestrator.storeClient.AddVolumeTransaction(ctx(), volTxn); err != nil {
			t.Fatalf("%s: Unable to create volume transaction: %v", c.name, err)
		}
		newOrchestrator := getOrchestrator(t)
		newOrchestrator.mutex.Lock()
		if _, ok := newOrchestrator.snapshots[c.snapshotConfig.ID()]; ok {
			t.Errorf("%s: snapshot still present in orchestrator.", c.name)
			// Note: assume that if the snapshot's still present in the
			// top-level map, it's present everywhere else and that, if it's
			// absent there, it's absent everywhere else in memory
		}
		backend, err := newOrchestrator.getBackendByBackendName(backendName)
		if err != nil {
			t.Fatalf("%s: Backend not found after bootstrapping.", c.name)
		}
		f, ok := backend.Driver().(*fakedriver.StorageDriver)
		if !ok {
			t.Fatalf("%e", utils.TypeAssertionError("backend.Driver().(*fakedriver.StorageDriver)"))
		}

		_, ok = f.DestroyedSnapshots[c.snapshotConfig.ID()]
		if !ok && c.expectDestroy {
			t.Errorf("%s: Destroy not called on snapshot.", c.name)
		} else if ok && !c.expectDestroy {
			t.Errorf("%s: Destroy should not have been called on snapshot.", c.name)
		}

		_, err = newOrchestrator.storeClient.GetSnapshot(ctx(), c.snapshotConfig.VolumeName, c.snapshotConfig.Name)
		if err != nil {
			if !persistentstore.MatchKeyNotFoundErr(err) {
				t.Errorf("%s: unable to communicate with backing store: %v", c.name, err)
			}
		} else {
			t.Errorf("%s: Found SnapshotConfig still stored in store.", c.name)
		}
		if txns, err := newOrchestrator.storeClient.GetVolumeTransactions(ctx()); err != nil {
			t.Errorf("%s: Unable to retrieve transactions from backing store: %v", c.name, err)
		} else if len(txns) > 0 {
			t.Errorf("%s: Transaction not cleared from the backing store", c.name)
		}
		newOrchestrator.mutex.Unlock()
	}
}

// TestAddSnapshotRecovery tests that the orchestrator can recover from a failure
// during the AddSnapshot transaction.
// It checks that the snapshot is deleted if the transaction is complete, and that
// it is not deleted if the transaction is not complete.
// Parameters:
//   backendName: Name of the backend to use for the test
//   scName: Name of the storage class to use for the test
//   volumeName: Name of the volume to use for the test
//   fullSnapshotName: Name of the snapshot to use for the full test
//   txOnlySnapshotName: Name of the snapshot to use for the partial test
// Example:
//   TestAddSnapshotRecovery(t, "addSnapshotRecoveryBackend", "addSnapshotRecoveryBackendSC", "addSnapshotRecoveryVolume", "addSnapshotRecoverySnapshotFull", "addSnapshotRecoverySnapshotTxOnly")

func TestAddSnapshotRecovery(t *testing.T) {
	const (
		backendName        = "addSnapshotRecoveryBackend"
		scName             = "addSnapshotRecoveryBackendSC"
		volumeName         = "addSnapshotRecoveryVolume"
		fullSnapshotName   = "addSnapshotRecoverySnapshotFull"
		txOnlySnapshotName = "addSnapshotRecoverySnapshotTxOnly"
	)
	orchestrator := getOrchestrator(t)
	prepRecoveryTest(t, orchestrator, backendName, scName)

	// It's easier to add the volume/snapshot and then reinject the transaction again afterwards.
	volumeConfig := tu.GenerateVolumeConfig(volumeName, 50, scName, config.File)
	if _, err := orchestrator.AddVolume(ctx(), volumeConfig); err != nil {
		t.Fatal("Unable to add volume: ", err)
	}

	// For the full test, we create everything and recreate the AddSnapshot transaction.
	fullSnapshotConfig := generateSnapshotConfig(fullSnapshotName, volumeName, volumeName)
	if _, err := orchestrator.CreateSnapshot(ctx(), fullSnapshotConfig); err != nil {
		t.Fatal("Unable to add snapshot: ", err)
	}

	// For the partial test, we add only the AddSnapshot transaction.
	txOnlySnapshotConfig := generateSnapshotConfig(txOnlySnapshotName, volumeName, volumeName)

	// BEGIN actual test.  Note that the delete idempotency is handled at the backend layer
	// (above the driver), so if the snapshot doesn't exist after bootstrapping, the driver
	// will not be called to delete the snapshot.
	runSnapshotRecoveryTests(
		t, orchestrator, backendName, storage.AddSnapshot,
		[]recoveryTest{
			{name: "full", volumeConfig: volumeConfig, snapshotConfig: fullSnapshotConfig, expectDestroy: true},
			{name: "txOnly", volumeConfig: volumeConfig, snapshotConfig: txOnlySnapshotConfig, expectDestroy: false},
		},
	)
	cleanup(t, orchestrator)
}

// TestDeleteSnapshotRecovery tests that the orchestrator can recover from a delete snapshot
// transaction.
// It checks that the snapshot is deleted if the transaction was fully committed, and that
// the snapshot is not deleted if the transaction was only partially committed.
// Parameters:
//   - full: the transaction was fully committed, so the snapshot should be deleted
//   - txOnly: the transaction was only partially committed, so the snapshot should not be deleted
// Example:
//   - TestDeleteSnapshotRecovery full
//   - TestDeleteSnapshotRecovery txOnly

func TestDeleteSnapshotRecovery(t *testing.T) {
	const (
		backendName        = "deleteSnapshotRecoveryBackend"
		scName             = "deleteSnapshotRecoveryBackendSC"
		volumeName         = "deleteSnapshotRecoveryVolume"
		fullSnapshotName   = "deleteSnapshotRecoverySnapshotFull"
		txOnlySnapshotName = "deleteSnapshotRecoverySnapshotTxOnly"
	)
	orchestrator := getOrchestrator(t)
	prepRecoveryTest(t, orchestrator, backendName, scName)

	// For the full test, we delete everything and recreate the delete transaction.
	volumeConfig := tu.GenerateVolumeConfig(volumeName, 50, scName, config.File)
	if _, err := orchestrator.AddVolume(ctx(), volumeConfig); err != nil {
		t.Fatal("Unable to add volume: ", err)
	}
	fullSnapshotConfig := generateSnapshotConfig(fullSnapshotName, volumeName, volumeName)
	if _, err := orchestrator.CreateSnapshot(ctx(), fullSnapshotConfig); err != nil {
		t.Fatal("Unable to add snapshot: ", err)
	}
	if err := orchestrator.DeleteSnapshot(ctx(), volumeName, fullSnapshotName); err != nil {
		t.Fatal("Unable to remove full snapshot: ", err)
	}

	// For the partial test, we ensure the snapshot will be restored during bootstrapping,
	// and the delete transaction will ensure everything is deleted.
	txOnlySnapshotConfig := generateSnapshotConfig(txOnlySnapshotName, volumeName, volumeName)
	if _, err := orchestrator.CreateSnapshot(ctx(), txOnlySnapshotConfig); err != nil {
		t.Fatal("Unable to add snapshot: ", err)
	}

	// BEGIN actual test.  Note that the delete idempotency is handled at the backend layer
	// (above the driver), so if the snapshot doesn't exist after bootstrapping, the driver
	// will not be called to delete the snapshot.
	runSnapshotRecoveryTests(
		t, orchestrator, backendName, storage.DeleteSnapshot,
		[]recoveryTest{
			{name: "full", snapshotConfig: fullSnapshotConfig, expectDestroy: false},
			{name: "txOnly", snapshotConfig: txOnlySnapshotConfig, expectDestroy: true},
		},
	)
	cleanup(t, orchestrator)
}

// The next series of tests test that bootstrap doesn't exit early if it
// encounters a key error for one of the main types of entries.
// TestStorageClassOnlyBootstrap tests that a storage class can be bootstrapped
// without any volumes.
// It checks that the storage class is bootstrapped correctly.
// Parameters:
//   t *testing.T
// Example:
//   TestStorageClassOnlyBootstrap(t)

func TestStorageClassOnlyBootstrap(t *testing.T) {
	const scName = "storageclass-only"

	orchestrator := getOrchestrator(t)
	originalSC, err := orchestrator.AddStorageClass(
		ctx(), &storageclass.Config{
			Name: scName,
			Attributes: map[string]sa.Request{
				sa.Media:            sa.NewStringRequest("hdd"),
				sa.ProvisioningType: sa.NewStringRequest("thick"),
				sa.RecoveryTest:     sa.NewBoolRequest(true),
			},
		},
	)
	if err != nil {
		t.Fatal("Unable to add storage class: ", err)
	}
	newOrchestrator := getOrchestrator(t)
	bootstrappedSC, err := newOrchestrator.GetStorageClass(ctx(), scName)
	if bootstrappedSC == nil || err != nil {
		t.Error("Unable to find storage class after bootstrapping.")
	} else if !reflect.DeepEqual(bootstrappedSC, originalSC) {
		t.Errorf("External storage classs differ:\n\tOriginal: %v\n\tBootstrapped: %v", originalSC, bootstrappedSC)
	}
	cleanup(t, orchestrator)
}

// TestFirstVolumeRecovery tests that the first volume added to a backend is
// recovered correctly.
// It checks that the volume is destroyed, and that the backend is not.
// Parameters:
//   backendName: name of the backend to be used
//   scName: name of the storage class to be used
//   txOnlyVolumeName: name of the volume to be created
// Example:
//   TestFirstVolumeRecovery(t, "firstRecoveryBackend", "firstRecoveryBackendSC", "firstRecoveryVolumeTxOnly")

func TestFirstVolumeRecovery(t *testing.T) {
	const (
		backendName      = "firstRecoveryBackend"
		scName           = "firstRecoveryBackendSC"
		txOnlyVolumeName = "firstRecoveryVolumeTxOnly"
	)
	orchestrator := getOrchestrator(t)
	prepRecoveryTest(t, orchestrator, backendName, scName)
	txOnlyVolumeConfig := tu.GenerateVolumeConfig(txOnlyVolumeName, 50, scName, config.File)
	// BEGIN actual test
	runRecoveryTests(
		t, orchestrator, backendName, storage.AddVolume, []recoveryTest{
			{
				name: "firstTXOnly", volumeConfig: txOnlyVolumeConfig,
				expectDestroy: true,
			},
		},
	)
	cleanup(t, orchestrator)
}

// TestOrchestratorNotReady tests that the orchestrator returns an error when not ready.
// It checks that all orchestrator methods return an error.
// Parameters:
//   t *testing.T : go test helper
// It returns nothing.
// Example:
//   TestOrchestratorNotReady(t *testing.T)

func TestOrchestratorNotReady(t *testing.T) {

	var (
		err            error
		backend        *storage.BackendExternal
		backends       []*storage.BackendExternal
		volume         *storage.VolumeExternal
		volumes        []*storage.VolumeExternal
		snapshot       *storage.SnapshotExternal
		snapshots      []*storage.SnapshotExternal
		storageClass   *storageclass.External
		storageClasses []*storageclass.External
	)

	orchestrator := getOrchestrator(t)
	orchestrator.bootstrapped = false
	orchestrator.bootstrapError = utils.NotReadyError()

	backend, err = orchestrator.AddBackend(ctx(), "", "")
	if backend != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected AddBackend to return an error.")
	}

	backend, err = orchestrator.GetBackend(ctx(), "")
	if backend != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected GetBackend to return an error.")
	}

	backends, err = orchestrator.ListBackends(ctx())
	if backends != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected ListBackends to return an error.")
	}

	err = orchestrator.DeleteBackend(ctx(), "")
	if !utils.IsNotReadyError(err) {
		t.Errorf("Expected DeleteBackend to return an error.")
	}

	volume, err = orchestrator.AddVolume(ctx(), nil)
	if volume != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected AddVolume to return an error.")
	}

	volume, err = orchestrator.CloneVolume(ctx(), nil)
	if volume != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected CloneVolume to return an error.")
	}

	volume, err = orchestrator.GetVolume(ctx(), "")
	if volume != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected GetVolume to return an error.")
	}

	_, err = orchestrator.GetDriverTypeForVolume(ctx(), nil)
	if !utils.IsNotReadyError(err) {
		t.Errorf("Expected GetDriverTypeForVolume to return an error.")
	}

	_, err = orchestrator.GetVolumeType(ctx(), nil)
	if !utils.IsNotReadyError(err) {
		t.Errorf("Expected GetVolumeType to return an error.")
	}

	volumes, err = orchestrator.ListVolumes(ctx())
	if volumes != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected ListVolumes to return an error.")
	}

	err = orchestrator.DeleteVolume(ctx(), "")
	if !utils.IsNotReadyError(err) {
		t.Errorf("Expected DeleteVolume to return an error.")
	}

	volumes, err = orchestrator.ListVolumesByPlugin(ctx(), "")
	if volumes != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected ListVolumesByPlugin to return an error.")
	}

	err = orchestrator.AttachVolume(ctx(), "", "", nil)
	if !utils.IsNotReadyError(err) {
		t.Errorf("Expected AttachVolume to return an error.")
	}

	err = orchestrator.DetachVolume(ctx(), "", "")
	if !utils.IsNotReadyError(err) {
		t.Errorf("Expected DetachVolume to return an error.")
	}

	snapshot, err = orchestrator.CreateSnapshot(ctx(), nil)
	if snapshot != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected CreateSnapshot to return an error.")
	}

	snapshot, err = orchestrator.GetSnapshot(ctx(), "", "")
	if snapshot != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected GetSnapshot to return an error.")
	}

	snapshots, err = orchestrator.ListSnapshots(ctx())
	if snapshots != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected ListSnapshots to return an error.")
	}

	snapshots, err = orchestrator.ReadSnapshotsForVolume(ctx(), "")
	if snapshots != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected ReadSnapshotsForVolume to return an error.")
	}

	err = orchestrator.DeleteSnapshot(ctx(), "", "")
	if !utils.IsNotReadyError(err) {
		t.Errorf("Expected DeleteSnapshot to return an error.")
	}

	err = orchestrator.ReloadVolumes(ctx())
	if !utils.IsNotReadyError(err) {
		t.Errorf("Expected ReloadVolumes to return an error.")
	}

	storageClass, err = orchestrator.AddStorageClass(ctx(), nil)
	if storageClass != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected AddStorageClass to return an error.")
	}

	storageClass, err = orchestrator.GetStorageClass(ctx(), "")
	if storageClass != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected GetStorageClass to return an error.")
	}

	storageClasses, err = orchestrator.ListStorageClasses(ctx())
	if storageClasses != nil || !utils.IsNotReadyError(err) {
		t.Errorf("Expected ListStorageClasses to return an error.")
	}

	err = orchestrator.DeleteStorageClass(ctx(), "")
	if !utils.IsNotReadyError(err) {
		t.Errorf("Expected DeleteStorageClass to return an error.")
	}
}

// importVolumeSetup creates a backend, storage class, and volume config for
// Parameters:
//   t - test object
//   backendName - name of the backend to create
//   scName - name of the storage class to create
//   volumeName - name of the volume to create
//   importOriginalName - original name of the volume to import
//   backendProtocol - protocol of the backend to create
// Returns:
//   *TridentOrchestrator - orchestrator object
//   *storage.VolumeConfig - volume config object
// It returns the orchestrator and volume config objects.
// Example:
//   orchestrator, volumeConfig := importVolumeSetup(t, "testBackend", "testSC",
//     "testVolume", "testVolumeOrig", config.File)

func importVolumeSetup(
	t *testing.T, backendName string, scName string, volumeName string, importOriginalName string,
	backendProtocol config.Protocol,
) (*TridentOrchestrator, *storage.VolumeConfig) {
	// Object setup
	orchestrator := getOrchestrator(t)
	addBackendStorageClass(t, orchestrator, backendName, scName, backendProtocol)

	orchestrator.mutex.Lock()
	_, ok := orchestrator.storageClasses[scName]
	if !ok {
		t.Fatal("Storageclass not found in orchestrator map")
	}
	orchestrator.mutex.Unlock()

	backendUUID := ""
	for _, b := range orchestrator.backends {
		if b.Name() == backendName {
			backendUUID = b.BackendUUID()
			break
		}
	}

	if backendUUID == "" {
		t.Fatal("BackendUUID not found")
	}

	volumeConfig := tu.GenerateVolumeConfig(volumeName, 50, scName, backendProtocol)
	volumeConfig.ImportOriginalName = importOriginalName
	volumeConfig.ImportBackendUUID = backendUUID
	return orchestrator, volumeConfig
}

// TestImportVolumeFailures tests the failure paths of importVolume
// It checks that the volume is renamed to the original name and that the persisted state is cleaned up
// Parameters:
//   volumeName: the name of the volume to import
//   originalName: the original name of the volume
//   backendProtocol: the protocol of the backend
//   createPVandPVCError: function to inject an error into the PV/PVC creation
// It returns the orchestrator and the volumeConfig
// Example:
//   orchestrator, volumeConfig := importVolumeSetup(t, "backend82", "sc01", "volume82", "origVolume01", config.File)
//   _, err := orchestrator.LegacyImportVolume(ctx(), volumeConfig, "backend82", false, createPVandPVCError)

func TestImportVolumeFailures(t *testing.T) {
	const (
		backendName     = "backend82"
		scName          = "sc01"
		volumeName      = "volume82"
		originalName01  = "origVolume01"
		backendProtocol = config.File
	)

	createPVandPVCError := func(volExternal *storage.VolumeExternal, driverType string) error {
		return fmt.Errorf("failed to create PV")
	}

	orchestrator, volumeConfig := importVolumeSetup(t, backendName, scName, volumeName, originalName01, backendProtocol)

	_, err := orchestrator.LegacyImportVolume(ctx(), volumeConfig, backendName, false, createPVandPVCError)

	// verify that importVolumeCleanup renamed volume to originalName
	backend, _ := orchestrator.getBackendByBackendName(backendName)
	volExternal, err := backend.Driver().GetVolumeExternal(ctx(), originalName01)
	if err != nil {
		t.Fatalf("failed to get volumeExternal for %s", originalName01)
	}
	if volExternal.Config.Size != "1000000000" {
		t.Errorf("falied to verify %s size %s", originalName01, volExternal.Config.Size)
	}

	// verify that we cleaned up the persisted state
	if _, ok := orchestrator.volumes[volumeConfig.Name]; ok {
		t.Errorf("volume %s should not exist in orchestrator's volume cache", volumeConfig.Name)
	}
	persistedVolume, err := orchestrator.storeClient.GetVolume(ctx(), volumeConfig.Name)
	if persistedVolume != nil {
		t.Errorf("volume %s should not be persisted", volumeConfig.Name)
	}

	cleanup(t, orchestrator)
}

// TestLegacyImportVolume tests the legacy
// It checks that the volume is imported with the correct internal name.
// It checks that the volume is persisted if it is managed.
// It checks that the volume is not persisted if it is not managed.
// Parameters:
//   volumeConfig: the volume config to use for the import
//   notManaged: true if the volume is not managed
//   createFunc: the function to use to create the PV and PVC
//   expectedInternalName: the expected internal name of the volume
// It returns the volumeExternal returned by the import.
// Example:
//   TestLegacyImportVolume(t, volumeConfig, false, createPVandPVCNoOp, volumeConfig.InternalName)

func TestLegacyImportVolume(t *testing.T) {
	const (
		backendName     = "backend02"
		scName          = "sc01"
		volumeName01    = "volume01"
		volumeName02    = "volume02"
		originalName01  = "origVolume01"
		originalName02  = "origVolume02"
		backendProtocol = config.File
	)

	createPVandPVCNoOp := func(volExternal *storage.VolumeExternal, driverType string) error {
		return nil
	}

	orchestrator, volumeConfig := importVolumeSetup(t, backendName, scName, volumeName01, originalName01,
		backendProtocol)

	// The volume exists on the backend with the original name.
	// Set volumeConfig.InternalName to the expected volumeName post import.
	volumeConfig.InternalName = volumeName01

	notManagedVolConfig := volumeConfig.ConstructClone()
	notManagedVolConfig.Name = volumeName02
	notManagedVolConfig.InternalName = volumeName02
	notManagedVolConfig.ImportOriginalName = originalName02
	notManagedVolConfig.ImportNotManaged = true

	// Test configuration
	for _, c := range []struct {
		name                 string
		volumeConfig         *storage.VolumeConfig
		notManaged           bool
		createFunc           VolumeCallback
		expectedInternalName string
	}{
		{
			name:                 "managed",
			volumeConfig:         volumeConfig,
			notManaged:           false,
			createFunc:           createPVandPVCNoOp,
			expectedInternalName: volumeConfig.InternalName,
		},
		{
			name:                 "notManaged",
			volumeConfig:         notManagedVolConfig,
			notManaged:           true,
			createFunc:           createPVandPVCNoOp,
			expectedInternalName: originalName02,
		},
	} {
		// The test code
		volExternal, err := orchestrator.LegacyImportVolume(ctx(), c.volumeConfig, backendName, c.notManaged,
			c.createFunc)
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		} else {
			if volExternal.Config.InternalName != c.expectedInternalName {
				t.Errorf(
					"%s: expected matching internal names %s - %s",
					c.name, c.expectedInternalName, volExternal.Config.InternalName,
				)
			}
			if _, ok := orchestrator.volumes[volExternal.Config.Name]; ok {
				if c.notManaged {
					t.Errorf("%s: notManaged volume %s should not be persisted", c.name, volExternal.Config.Name)
				}
			} else if !c.notManaged {
				t.Errorf("%s: managed volume %s should be persisted", c.name, volExternal.Config.Name)
			}

		}

	}
	cleanup(t, orchestrator)
}

// TestImportVolume tests the
// It checks that the volume is imported with the correct internal name
// and that the volume is persisted in the orchestrator.
// Parameters:
//   backendName: name of the backend
//   scName: name of the storage class
//   volumeName: name of the volume to be imported
//   originalName: original name of the volume to be imported
//   backendProtocol: protocol of the backend
// Example:
//   TestImportVolume(t, "backend02", "sc01", "volume01", "origVolume01", config.File)

func TestImportVolume(t *testing.T) {
	const (
		backendName     = "backend02"
		scName          = "sc01"
		volumeName01    = "volume01"
		volumeName02    = "volume02"
		originalName01  = "origVolume01"
		originalName02  = "origVolume02"
		backendProtocol = config.File
	)

	orchestrator, volumeConfig := importVolumeSetup(
		t, backendName, scName, volumeName01, originalName01, backendProtocol,
	)

	notManagedVolConfig := volumeConfig.ConstructClone()
	notManagedVolConfig.Name = volumeName02
	notManagedVolConfig.ImportOriginalName = originalName02
	notManagedVolConfig.ImportNotManaged = true

	// Test configuration
	for _, c := range []struct {
		name                 string
		volumeConfig         *storage.VolumeConfig
		expectedInternalName string
	}{
		{
			name:                 "managed",
			volumeConfig:         volumeConfig,
			expectedInternalName: volumeName01,
		},
		{
			name:                 "notManaged",
			volumeConfig:         notManagedVolConfig,
			expectedInternalName: originalName02,
		},
	} {
		// The test code
		volExternal, err := orchestrator.ImportVolume(ctx(), c.volumeConfig)
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		} else {
			if volExternal.Config.InternalName != c.expectedInternalName {
				t.Errorf(
					"%s: expected matching internal names %s - %s",
					c.name, c.expectedInternalName, volExternal.Config.InternalName,
				)
			}
			if _, ok := orchestrator.volumes[volExternal.Config.Name]; !ok {
				t.Errorf("%s: managed volume %s should be persisted", c.name, volExternal.Config.Name)
			}
		}

	}
	cleanup(t, orchestrator)
}

// TestValidateImportVolumeNasBackend tests the validateImportVolume function for NAS backends.
// It checks for the following error conditions:
// - The volume exists on the backend with the original name
// - The storage class is unknown
// - The volume does not exist on the backend
// - The access mode is incompatible with the backend
// - The volume mode is incompatible with the backend
// - The protocol is incompatible with the backend
// Parameters:
//   backendName - the name of the backend
//   scName - the name of the storage class
//   volumeName - the name of the volume to import
//   originalName - the original name of the volume to import
//   backendProtocol - the protocol of the backend
// Example:
//   backendName := "backend01"
//   scName := "sc01"
//   volumeName := "volume01"
//   originalName := "origVolume01"
//   backendProtocol := config.File
//   TestValidateImportVolumeNasBackend(t, backendName, scName, volumeName, originalName, backendProtocol)

func TestValidateImportVolumeNasBackend(t *testing.T) {
	const (
		backendName     = "backend01"
		scName          = "sc01"
		volumeName      = "volume01"
		originalName    = "origVolume01"
		backendProtocol = config.File
	)

	orchestrator, volumeConfig := importVolumeSetup(t, backendName, scName, volumeName, originalName, backendProtocol)

	_, err := orchestrator.AddVolume(ctx(), volumeConfig)
	if err != nil {
		t.Fatal("Unable to add volume: ", err)
	}

	// The volume exists on the backend with the original name since we added it above.
	// Set volumeConfig.InternalName to the expected volumeName post import.
	volumeConfig.InternalName = volumeName

	// Create VolumeConfig objects for the remaining error conditions
	pvExistsVolConfig := volumeConfig.ConstructClone()
	pvExistsVolConfig.ImportOriginalName = volumeName

	unknownSCVolConfig := volumeConfig.ConstructClone()
	unknownSCVolConfig.StorageClass = "sc02"

	missingVolConfig := volumeConfig.ConstructClone()
	missingVolConfig.ImportOriginalName = "noVol"

	accessModeVolConfig := volumeConfig.ConstructClone()
	accessModeVolConfig.AccessMode = config.ReadWriteMany
	accessModeVolConfig.Protocol = config.Block

	volumeModeVolConfig := volumeConfig.ConstructClone()
	volumeModeVolConfig.VolumeMode = config.RawBlock
	volumeModeVolConfig.Protocol = config.File

	protocolVolConfig := volumeConfig.ConstructClone()
	protocolVolConfig.Protocol = config.Block

	for _, c := range []struct {
		name         string
		volumeConfig *storage.VolumeConfig
		valid        bool
		error        string
	}{
		{name: "volumeConfig", volumeConfig: volumeConfig, valid: true, error: ""},
		{name: "pvExists", volumeConfig: pvExistsVolConfig, valid: false, error: "already exists"},
		{name: "unknownSC", volumeConfig: unknownSCVolConfig, valid: false, error: "unknown storage class"},
		{name: "missingVolume", volumeConfig: missingVolConfig, valid: false, error: "volume noVol was not found"},
		{
			name:         "accessMode",
			volumeConfig: accessModeVolConfig,
			valid:        false,
			error:        "incompatible",
		},
		{name: "volumeMode", volumeConfig: volumeModeVolConfig, valid: false, error: "incompatible volume mode "},
		{name: "protocol", volumeConfig: protocolVolConfig, valid: false, error: "incompatible with the backend"},
	} {
		// The test code
		err = orchestrator.validateImportVolume(ctx(), c.volumeConfig)
		if err != nil {
			if c.valid {
				t.Errorf("%s: unexpected error %v", c.name, err)
			} else {
				if !strings.Contains(err.Error(), c.error) {
					t.Errorf("%s: expected %s but received error %v", c.name, c.error, err)
				}
			}
		} else if !c.valid {
			t.Errorf("%s: expected error but passed test", c.name)
		}
	}

	cleanup(t, orchestrator)
}

// TestValidateImportVolumeSanBackend tests the validateImportVolume function for
// SAN backends.
// It checks for the following error conditions:
// - protocol mismatch
// - file system mismatch
// Parameters:
//   - backendName: name of the backend to use
//   - scName: name of the storage class to use
//   - volumeName: name of the volume to create
//   - originalName: name of the volume to import
//   - backendProtocol: protocol of the backend to use
// Example:
//   TestValidateImportVolumeSanBackend("backend01", "sc01", "volume01", "origVolume01", config.Block)

func TestValidateImportVolumeSanBackend(t *testing.T) {
	const (
		backendName     = "backend01"
		scName          = "sc01"
		volumeName      = "volume01"
		originalName    = "origVolume01"
		backendProtocol = config.Block
	)

	orchestrator, volumeConfig := importVolumeSetup(t, backendName, scName, volumeName, originalName, backendProtocol)

	_, err := orchestrator.AddVolume(ctx(), volumeConfig)
	if err != nil {
		t.Fatal("Unable to add volume: ", err)
	}

	// The volume exists on the backend with the original name since we added it above.
	// Set volumeConfig.InternalName to the expected volumeName post import.
	volumeConfig.InternalName = volumeName

	// Create VolumeConfig objects for the remaining error conditions
	protocolVolConfig := volumeConfig.ConstructClone()
	protocolVolConfig.Protocol = config.File

	ext4RawBlockFSVolConfig := volumeConfig.ConstructClone()
	ext4RawBlockFSVolConfig.VolumeMode = config.RawBlock
	ext4RawBlockFSVolConfig.Protocol = config.Block
	ext4RawBlockFSVolConfig.FileSystem = "ext4"

	for _, c := range []struct {
		name         string
		volumeConfig *storage.VolumeConfig
		valid        bool
		error        string
	}{
		{name: "protocol", volumeConfig: protocolVolConfig, valid: false, error: "incompatible with the backend"},
		{
			name:         "invalidFS",
			volumeConfig: ext4RawBlockFSVolConfig,
			valid:        false,
			error:        "cannot create raw-block volume",
		},
	} {
		// The test code
		err = orchestrator.validateImportVolume(ctx(), c.volumeConfig)
		if err != nil {
			if c.valid {
				t.Errorf("%s: unexpected error %v", c.name, err)
			} else {
				if !strings.Contains(err.Error(), c.error) {
					t.Errorf("%s: expected %s but received error %v", c.name, c.error, err)
				}
			}
		} else if !c.valid {
			t.Errorf("%s: expected error but passed test", c.name)
		}
	}

	cleanup(t, orchestrator)
}

// TestAddVolumePublication tests that the orchestrator correctly adds a volume publication
// to its cache and calls the store client to add the volume publication to the persistent store
// It checks that the orchestrator's cache is updated correctly
// Parameters:
//    t *testing.T - test object
// Example:
//    TestAddVolumePublication(t)
// Returns:
//    None

func TestAddVolumePublication(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Verify that the core calls the store client with the correct object, returning success
	mockStoreClient.EXPECT().AddVolumePublication(gomock.Any(), fakePub).Return(nil)

	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient

	err := orchestrator.AddVolumePublication(context.Background(), fakePub)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error adding volume publication: %v", err))
	assert.Contains(t, orchestrator.volumePublications, fakePub.VolumeName,
		"volume publication missing from orchestrator's cache")
	assert.Contains(t, orchestrator.volumePublications[fakePub.VolumeName], fakePub.NodeName,
		"volume publication missing from orchestrator's cache")
	assert.Equal(t, fakePub, orchestrator.volumePublications[fakePub.VolumeName][fakePub.NodeName],
		"volume publication was not correctly added")
}

// TestAddVolumePublicationError tests the error path for AddVolumePublication
// It checks that the orchestrator does not add the volume publication to its cache
// when the store client returns an error
// Parameters:
//   t *testing.T
//     The test object
// It returns nothing
// Example:
//   TestAddVolumePublicationError(t)

func TestAddVolumePublicationError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Verify that the core calls the store client with the correct object, but return an error
	mockStoreClient.EXPECT().AddVolumePublication(gomock.Any(), fakePub).Return(fmt.Errorf("fake error"))

	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient

	err := orchestrator.AddVolumePublication(context.Background(), fakePub)
	assert.NotNilf(t, err, "add volume publication did not return an error")
	assert.NotContains(t, orchestrator.volumePublications, fakePub.VolumeName,
		"volume publication was added orchestrator's cache")
}

// TestGetVolumePublication tests the GetVolumePublication method
// It checks that the volume publication is correctly retrieved from the cache
// Parameters:
//    t *testing.T: go test helper
// Example:
//    TestGetVolumePublication(t *testing.T)

func TestGetVolumePublication(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	orchestrator.addVolumePublicationToCache(fakePub)

	actualPub, err := orchestrator.GetVolumePublication(context.Background(), fakePub.VolumeName, fakePub.NodeName)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error getting volume publication: %v", err))
	assert.Equal(t, fakePub, actualPub, "volume publication was not correctly retrieved")
}

// TestGetVolumePublicationNotFound tests that a volume publication is not found
// when the volume is not found
// It checks that the correct error is returned
// Parameters:
//   t *testing.T
// Example:
//   TestGetVolumePublicationNotFound(t)

func TestGetVolumePublicationNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient

	actualPub, err := orchestrator.GetVolumePublication(context.Background(), "NotFound", "NotFound")
	assert.NotNilf(t, err, fmt.Sprintf("unexpected success getting volume publication: %v", err))
	assert.True(t, utils.IsNotFoundError(err), "incorrect error type returned")
	assert.Empty(t, actualPub, "non-empty publication returned")
}

// TestGetVolumePublicationError tests the GetVolumePublication function when
// there is an error during bootstrap
// It checks that the correct error is returned
// Parameters:
//    t *testing.T - go test helper object
// Example:
//   TestGetVolumePublicationError(t *testing.T)

func TestGetVolumePublicationError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	orchestrator.addVolumePublicationToCache(fakePub)

	// Simulate a bootstrap error
	orchestrator.bootstrapError = fmt.Errorf("some error")

	actualPub, err := orchestrator.GetVolumePublication(context.Background(), fakePub.VolumeName, fakePub.NodeName)
	assert.NotNilf(t, err, fmt.Sprintf("unexpected success getting volume publication: %v", err))
	assert.False(t, utils.IsNotFoundError(err), "incorrect error type returned")
	assert.Empty(t, actualPub, "non-empty publication returned")
}

// TestListVolumePublications tests the ListVolumePublications method
// It checks that the correct list of publications is returned
// Parameters:
//     t *testing.T : go test framework object used for setup, teardown, etc.
// Example:
//     TestListVolumePublications(t)
// Returns:
//     None

func TestListVolumePublications(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub2 := &utils.VolumePublication{
		Name:       "baz/biz",
		NodeName:   "biz",
		VolumeName: "baz",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub3 := &utils.VolumePublication{
		Name:       fmt.Sprintf("%s/buz", fakePub.VolumeName),
		NodeName:   "buz",
		VolumeName: fakePub.VolumeName,
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)
	orchestrator.addVolumePublicationToCache(fakePub2)
	orchestrator.addVolumePublicationToCache(fakePub3)

	expectedPubs := []*utils.VolumePublication{fakePub, fakePub2, fakePub3}
	actualPubs, err := orchestrator.ListVolumePublications(context.Background())
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.ElementsMatch(t, expectedPubs, actualPubs, "incorrect publication list returned")
}

// TestListVolumePublicationsNotFound tests that ListVolumePublications returns an empty list when no publications are found
// It checks that the orchestrator does not return an error when no publications are found
// Parameters:
//    t *testing.T : go test framework object used to generate test failures
// Example:
//    TestListVolumePublicationsNotFound(t *testing.T)

func TestListVolumePublicationsNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient

	actualPubs, err := orchestrator.ListVolumePublications(context.Background())
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

// TestListVolumePublicationsError tests the error path for ListVolumePublications
// It checks that the error is returned and the publication list is empty
// Parameters:
//    t *testing.T - go test handler
// Example:
//    TestListVolumePublicationsError(t)

func TestListVolumePublicationsError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)

	// Simulate a bootstrap error
	orchestrator.bootstrapError = fmt.Errorf("some error")

	actualPubs, err := orchestrator.ListVolumePublications(context.Background())
	assert.NotNil(t, err, fmt.Sprintf("unexpected success listing volume publications"))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

// TestListVolumePublicationsForVolume tests the ListVolumePublicationsForVolume method
// It checks that the correct list of publications is returned for a given volume
// Parameters:
//   t *testing.T - test object
// Example:
//   TestListVolumePublicationsForVolume(t)

func TestListVolumePublicationsForVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub2 := &utils.VolumePublication{
		Name:       "baz/biz",
		NodeName:   "biz",
		VolumeName: "baz",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub3 := &utils.VolumePublication{
		Name:       fmt.Sprintf("%s/buz", fakePub.VolumeName),
		NodeName:   "buz",
		VolumeName: fakePub.VolumeName,
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)
	orchestrator.addVolumePublicationToCache(fakePub2)
	orchestrator.addVolumePublicationToCache(fakePub3)

	expectedPubs := []*utils.VolumePublication{fakePub, fakePub3}
	actualPubs, err := orchestrator.ListVolumePublicationsForVolume(context.Background(), fakePub.VolumeName)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.ElementsMatch(t, expectedPubs, actualPubs, "incorrect publication list returned")
}

// TestListVolumePublicationsForVolumeNotFound tests that ListVolumePublicationsForVolume
// returns an empty list when the volume is not found
// It checks that the orchestrator does not return an error
// Parameters:
//   t *testing.T : go test helper
// Example:
//   TestListVolumePublicationsForVolumeNotFound(t)
// Returns:
//   nothing
// Side Effects:
//   none

func TestListVolumePublicationsForVolumeNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)

	actualPubs, err := orchestrator.ListVolumePublicationsForVolume(context.Background(), "NotFound")
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

// TestListVolumePublicationsForVolumeError tests the error path for ListVolumePublicationsForVolume
// It checks that the error is returned if the orchestrator is in an error state
// Parameters:
//    t *testing.T : go test helper
// Example:
//    TestListVolumePublicationsForVolumeError(t *testing.T)

func TestListVolumePublicationsForVolumeError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)

	// Simulate a bootstrap error
	orchestrator.bootstrapError = fmt.Errorf("some error")

	actualPubs, err := orchestrator.ListVolumePublicationsForVolume(context.Background(), fakePub.VolumeName)
	assert.NotNil(t, err, fmt.Sprintf("unexpected success listing volume publications"))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

// TestListVolumePublicationsForNode tests the ListVolumePublicationsForNode method
// It checks that a list of publications for a given node is returned
// Parameters:
//   t *testing.T - go test object
// Example:
//   TestListVolumePublicationsForNode(t *testing.T)

func TestListVolumePublicationsForNode(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub2 := &utils.VolumePublication{
		Name:       "baz/biz",
		NodeName:   "biz",
		VolumeName: "baz",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub3 := &utils.VolumePublication{
		Name:       fmt.Sprintf("%s/buz", fakePub.VolumeName),
		NodeName:   "buz",
		VolumeName: fakePub.VolumeName,
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)
	orchestrator.addVolumePublicationToCache(fakePub2)
	orchestrator.addVolumePublicationToCache(fakePub3)

	expectedPubs := []*utils.VolumePublication{fakePub2}
	actualPubs, err := orchestrator.ListVolumePublicationsForNode(context.Background(), fakePub2.NodeName)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.ElementsMatch(t, expectedPubs, actualPubs, "incorrect publication list returned")
}

// TestListVolumePublicationsForNodeNotFound tests the ListVolumePublicationsForNode function
// when the node is not found
// It checks that an empty list is returned
// Parameters:
//   t *testing.T
// Example:
//   TestListVolumePublicationsForNodeNotFound(t)

func TestListVolumePublicationsForNodeNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)

	actualPubs, err := orchestrator.ListVolumePublicationsForNode(context.Background(), "NotFound")
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

// TestListVolumePublicationsForNodeError tests that ListVolumePublicationsForNode
// returns an error when the orchestrator is in an error state
// It checks that the error is returned and that the publication list is empty
// Parameters:
//   t *testing.T : go test helper object
// Example:
//   TestListVolumePublicationsForNodeError(t)

func TestListVolumePublicationsForNodeError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)

	// Simulate a bootstrap error
	orchestrator.bootstrapError = fmt.Errorf("some error")

	actualPubs, err := orchestrator.ListVolumePublicationsForVolume(context.Background(), fakePub.NodeName)
	assert.NotNil(t, err, fmt.Sprintf("unexpected success listing volume publications"))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

// TestDeleteVolumePublication tests the DeleteVolumePublication method
// It checks that the volume publication is removed from the cache
// and that the volume entry is removed from the cache if this is the last nodeID for a given volume
// Parameters:
//    t *testing.T: go test helper
// Example:
//   TestDeleteVolumePublication(t)

func TestDeleteVolumePublication(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub2 := &utils.VolumePublication{
		Name:       "baz/biz",
		NodeName:   "biz",
		VolumeName: "baz",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub3 := &utils.VolumePublication{
		Name:       fmt.Sprintf("%s/buz", fakePub.VolumeName),
		NodeName:   "buz",
		VolumeName: fakePub.VolumeName,
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)
	orchestrator.addVolumePublicationToCache(fakePub2)
	orchestrator.addVolumePublicationToCache(fakePub3)

	// Verify if this is the last nodeID for a given volume the volume entry is completely removed from the cache
	mockStoreClient.EXPECT().DeleteVolumePublication(gomock.Any(), fakePub2).Return(nil)
	err := orchestrator.DeleteVolumePublication(context.Background(), fakePub2.VolumeName, fakePub2.NodeName)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error deleting volume publication: %v", err))
	assert.NotContains(t, orchestrator.volumePublications, fakePub2.VolumeName,
		"publication not properly removed from cache")

	// Verify if this is not the last nodeID for a given volume the volume entry is not removed from the cache
	mockStoreClient.EXPECT().DeleteVolumePublication(gomock.Any(), fakePub3).Return(nil)
	err = orchestrator.DeleteVolumePublication(context.Background(), fakePub3.VolumeName, fakePub3.NodeName)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error deleting volume publication: %v", err))
	assert.NotNil(t, orchestrator.volumePublications[fakePub3.VolumeName],
		"publication not properly removed from cache")
	assert.NotContains(t, orchestrator.volumePublications[fakePub3.VolumeName], fakePub3.NodeName,
		"publication not properly removed from cache")
	assert.Contains(t, orchestrator.volumePublications[fakePub.VolumeName], fakePub.NodeName,
		"publication not properly removed from cache")
}

// TestDeleteVolumePublicationNotFound tests that the DeleteVolumePublication method returns a not found error
// when the volume publication is not found
// It checks that the volume publication is not deleted from the cache
// Parameters:
//    t *testing.T : go test helper
// Example:
//    TestDeleteVolumePublicationNotFound(t *testing.T)

func TestDeleteVolumePublicationNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)

	err := orchestrator.DeleteVolumePublication(context.Background(), "NotFound", "NotFound")
	assert.NotNil(t, err, fmt.Sprintf("unexpected success deleting volume publication"))
	assert.True(t, utils.IsNotFoundError(err), "incorrect error type returned")
}

// TestDeleteVolumePublicationNotFoundPersistence tests that delete volume publication is idempotent
// when the persistence object is missing
// It checks that the cache is updated and that the orchestrator does not panic
// Parameters:
//   t *testing.T : go test framework object
// Example:
//   TestDeleteVolumePublicationNotFoundPersistence(t *testing.T)

func TestDeleteVolumePublicationNotFoundPersistence(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	mockStoreClient.EXPECT().DeleteVolumePublication(gomock.Any(), fakePub).Return(utils.NotFoundError("not found"))
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)

	// Verify delete is idempotent when the persistence object is missing
	err := orchestrator.DeleteVolumePublication(context.Background(), fakePub.VolumeName, fakePub.NodeName)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error deleting volume publication: %v", err))
	assert.NotContains(t, orchestrator.volumePublications, fakePub.VolumeName,
		"publication not properly removed from cache")
}

// TestDeleteVolumePublicationError tests the error path of DeleteVolumePublication
// It checks that the orchestrator returns an error when the store client returns an error
// Parameters:
//    t *testing.T : go test helper
// Example:
//     TestDeleteVolumePublicationError(t)

func TestDeleteVolumePublicationError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &utils.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	orchestrator.addVolumePublicationToCache(fakePub)

	mockStoreClient.EXPECT().DeleteVolumePublication(gomock.Any(), fakePub).Return(fmt.Errorf("some error"))

	err := orchestrator.DeleteVolumePublication(context.Background(), fakePub.VolumeName, fakePub.NodeName)
	assert.NotNil(t, err, fmt.Sprintf("unexpected success deleting volume publication"))
	assert.False(t, utils.IsNotFoundError(err), "incorrect error type returned")
	assert.Equal(t, fakePub, orchestrator.volumePublications[fakePub.VolumeName][fakePub.NodeName],
		"publication improperly removed/updated in cache")
}

// TestAddNode tests adding a node to the orchestrator
// It checks that the node is added to the orchestrator and that the orchestrator
// is able to retrieve the node
// Parameters:
//   t *testing.T : go test helper object for running tests
// Example:
//   TestAddNode(t *testing.T)

func TestAddNode(t *testing.T) {
	node := &utils.Node{
		Name:           "testNode",
		IQN:            "myIQN",
		IPs:            []string{"1.1.1.1", "2.2.2.2"},
		TopologyLabels: map[string]string{"topology.kubernetes.io/region": "Region1"},
		Deleted:        false,
	}
	orchestrator := getOrchestrator(t)
	if err := orchestrator.AddNode(ctx(), node, nil); err != nil {
		t.Errorf("adding node failed; %v", err)
	}
}

// TestGetNode tests the GetNode method
// It checks that the correct node is returned
// Parameters:
//   t *testing.T
// Example:
//   TestGetNode(t)

func TestGetNode(t *testing.T) {
	orchestrator := getOrchestrator(t)
	expectedNode := &utils.Node{
		Name:           "testNode",
		IQN:            "myIQN",
		IPs:            []string{"1.1.1.1", "2.2.2.2"},
		TopologyLabels: map[string]string{"topology.kubernetes.io/region": "Region1"},
		Deleted:        false,
	}
	unexpectedNode := &utils.Node{
		Name:           "testNode2",
		IQN:            "myOtherIQN",
		IPs:            []string{"3.3.3.3", "4.4.4.4"},
		TopologyLabels: map[string]string{"topology.kubernetes.io/region": "Region2"},
		Deleted:        false,
	}
	initialNodes := map[string]*utils.Node{}
	initialNodes[expectedNode.Name] = expectedNode
	initialNodes[unexpectedNode.Name] = unexpectedNode
	orchestrator.nodes = initialNodes

	actualNode, err := orchestrator.GetNode(ctx(), expectedNode.Name)
	if err != nil {
		t.Errorf("error getting node; %v", err)
	}

	if actualNode != expectedNode {
		t.Errorf("Did not get expected node back; expected %+v, got %+v", expectedNode, actualNode)
	}
}

// TestListNodes tests the ListNodes function
// It checks that the list of nodes returned is the same as the list of nodes in the orchestrator
// Parameters:
//     t *testing.T : go test framework object used for running the test
// Example:
//     TestListNodes(t)

func TestListNodes(t *testing.T) {
	orchestrator := getOrchestrator(t)
	expectedNode1 := &utils.Node{
		Name:    "testNode",
		IQN:     "myIQN",
		IPs:     []string{"1.1.1.1", "2.2.2.2"},
		Deleted: false,
	}
	expectedNode2 := &utils.Node{
		Name:    "testNode2",
		IQN:     "myOtherIQN",
		IPs:     []string{"3.3.3.3", "4.4.4.4"},
		Deleted: false,
	}
	initialNodes := map[string]*utils.Node{}
	initialNodes[expectedNode1.Name] = expectedNode1
	initialNodes[expectedNode2.Name] = expectedNode2
	orchestrator.nodes = initialNodes
	expectedNodes := []*utils.Node{expectedNode1, expectedNode2}

	actualNodes, err := orchestrator.ListNodes(ctx())
	if err != nil {
		t.Errorf("error listing nodes; %v", err)
	}

	if !unorderedNodeSlicesEqual(actualNodes, expectedNodes) {
		t.Errorf("node list values do not match; expected %v, found %v", expectedNodes, actualNodes)
	}
}

// unorderedNodeSlicesEqual returns true if the two slices contain the same nodes,
// regardless of order.
// Parameters:
//   x, y - the slices to compare
// Returns:
//   bool - true if the slices contain the same nodes, regardless of order
// Example:
//   x := []*utils.Node{&utils.Node{Name: "node1"}, &utils.Node{Name: "node2"}}
//   y := []*utils.Node{&utils.Node{Name: "node2"}, &utils.Node{Name: "node1"}}
//   unorderedNodeSlicesEqual(x, y)
//   >> true

func unorderedNodeSlicesEqual(x, y []*utils.Node) bool {
	if len(x) != len(y) {
		return false
	}
	// create a map of node pointers -> int
	diff := make(map[*utils.Node]int, len(x))
	for _, _x := range x {
		// 0 value for int is 0, so just increment a counter for the string
		diff[_x]++
	}
	for _, _y := range y {
		// If the node _y is not in diff bail out early
		if _, ok := diff[_y]; !ok {
			return false
		}
		diff[_y] -= 1
		if diff[_y] == 0 {
			delete(diff, _y)
		}
	}
	return len(diff) == 0
}

// TestDeleteNode tests the deletion of a node
// It checks that the node is properly deleted from the orchestrator
// Parameters:
//     t *testing.T : go testing object used to handle logging
// Example:
//     TestDeleteNode(t *testing.T)

func TestDeleteNode(t *testing.T) {
	orchestrator := getOrchestrator(t)
	initialNode := &utils.Node{
		Name:    "testNode",
		IQN:     "myIQN",
		IPs:     []string{"1.1.1.1", "2.2.2.2"},
		Deleted: false,
	}
	initialNodes := map[string]*utils.Node{}
	initialNodes[initialNode.Name] = initialNode
	orchestrator.nodes = initialNodes

	if err := orchestrator.DeleteNode(ctx(), initialNode.Name); err != nil {
		t.Errorf("error deleting node; %v", err)
	}

	if _, ok := orchestrator.nodes[initialNode.Name]; ok {
		t.Errorf("node was not properly deleted")
	}
}

// TestSnapshotVolumes tests snapshotting of volumes
// It checks that the snapshot is created, and that it is stored in the persistent store
// It also checks that the snapshot can be deleted
// Parameters:
//   t: the test object
// Example:
//   TestSnapshotVolumes(t)

func TestSnapshotVolumes(t *testing.T) {
	mockPools := tu.GetFakePools()
	orchestrator := getOrchestrator(t)

	errored := false
	for _, c := range []struct {
		name      string
		protocol  config.Protocol
		poolNames []string
	}{
		{
			name:      "fast-a",
			protocol:  config.File,
			poolNames: []string{tu.FastSmall, tu.FastThinOnly},
		},
		{
			name:      "fast-b",
			protocol:  config.File,
			poolNames: []string{tu.FastThinOnly, tu.FastUniqueAttr},
		},
		{
			name:      "slow-file",
			protocol:  config.File,
			poolNames: []string{tu.SlowNoSnapshots, tu.SlowSnapshots},
		},
		{
			name:      "slow-block",
			protocol:  config.Block,
			poolNames: []string{tu.SlowNoSnapshots, tu.SlowSnapshots, tu.MediumOverlap},
		},
	} {
		pools := make(map[string]*fake.StoragePool, len(c.poolNames))
		for _, poolName := range c.poolNames {
			pools[poolName] = mockPools[poolName]
		}
		cfg, err := fakedriver.NewFakeStorageDriverConfigJSON(c.name, c.protocol, pools, make([]fake.Volume, 0))
		if err != nil {
			t.Fatalf("Unable to generate cfg JSON for %s: %v", c.name, err)
		}
		_, err = orchestrator.AddBackend(ctx(), cfg, "")
		if err != nil {
			t.Errorf("Unable to add backend %s: %v", c.name, err)
			errored = true
		}
		orchestrator.mutex.Lock()
		backend, err := orchestrator.getBackendByBackendName(c.name)
		if err != nil {
			t.Fatalf("Backend %s not stored in orchestrator", c.name)
		}
		persistentBackend, err := orchestrator.storeClient.GetBackend(ctx(), c.name)
		if err != nil {
			t.Fatalf("Unable to get backend %s from persistent store: %v", c.name, err)
		} else if !reflect.DeepEqual(
			backend.ConstructPersistent(ctx()),
			persistentBackend,
		) {
			t.Error("Wrong data stored for backend ", c.name)
		}
		orchestrator.mutex.Unlock()
	}
	if errored {
		t.Fatal("Failed to add all backends; aborting remaining tests.")
	}

	// Add storage classes
	storageClasses := []storageClassTest{
		{
			config: &storageclass.Config{
				Name: "slow",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(40),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			},
			expected: []*tu.PoolMatch{
				{Backend: "slow-file", Pool: tu.SlowSnapshots},
				{Backend: "slow-block", Pool: tu.SlowSnapshots},
			},
		},
		{
			config: &storageclass.Config{
				Name: "fast",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(2000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			},
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
		},
	}
	for _, s := range storageClasses {
		_, err := orchestrator.AddStorageClass(ctx(), s.config)
		if err != nil {
			t.Errorf("Unable to add storage class %s: %v", s.config.Name, err)
			continue
		}
		validateStorageClass(t, orchestrator, s.config.Name, s.expected)
	}

	for _, s := range []struct {
		name            string
		config          *storage.VolumeConfig
		expectedSuccess bool
		expectedMatches []*tu.PoolMatch
	}{
		{
			name:            "file",
			config:          tu.GenerateVolumeConfig("file", 1, "fast", config.File),
			expectedSuccess: true,
			expectedMatches: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
		},
		{
			name:            "block",
			config:          tu.GenerateVolumeConfig("block", 1, "slow", config.Block),
			expectedSuccess: true,
			expectedMatches: []*tu.PoolMatch{
				{Backend: "slow-block", Pool: tu.SlowSnapshots},
				{Backend: "slow-block", Pool: tu.SlowSnapshots},
			},
		},
	} {
		// Create the source volume
		_, err := orchestrator.AddVolume(ctx(), s.config)
		if err != nil {
			t.Errorf("%s: could not add volume: %v", s.name, err)
			continue
		}

		orchestrator.mutex.Lock()
		volume, found := orchestrator.volumes[s.config.Name]
		if s.expectedSuccess && !found {
			t.Errorf("%s: did not get volume where expected.", s.name)
			continue
		}
		orchestrator.mutex.Unlock()

		// Now take a snapshot and ensure everything looks fine
		snapshotName := "snapshot-" + uuid.New().String()
		snapshotConfig := &storage.SnapshotConfig{
			Version:    config.OrchestratorAPIVersion,
			Name:       snapshotName,
			VolumeName: volume.Config.Name,
		}
		snapshotExternal, err := orchestrator.CreateSnapshot(ctx(), snapshotConfig)
		if err != nil {
			t.Fatalf("%s: got unexpected error creating snapshot: %v", s.name, err)
		}

		orchestrator.mutex.Lock()
		// Snapshot should be registered in the store
		persistentSnapshot, err := orchestrator.storeClient.GetSnapshot(ctx(), volume.Config.Name, snapshotName)
		if err != nil {
			t.Errorf("%s: unable to communicate with backing store: %v", snapshotName, err)
		}
		persistentSnapshotExternal := persistentSnapshot.ConstructExternal()
		if !reflect.DeepEqual(persistentSnapshotExternal, snapshotExternal) {
			t.Errorf(
				"%s: external snapshot %s stored in backend does not match created snapshot.",
				snapshotName, persistentSnapshot.Config.Name,
			)
			externalSnapshotJSON, err := json.Marshal(persistentSnapshotExternal)
			if err != nil {
				t.Fatal("Unable to remarshal JSON: ", err)
			}
			origSnapshotJSON, err := json.Marshal(snapshotExternal)
			if err != nil {
				t.Fatal("Unable to remarshal JSON: ", err)
			}
			t.Logf("\tExpected: %s\n\tGot: %s\n", string(externalSnapshotJSON), string(origSnapshotJSON))
		}
		orchestrator.mutex.Unlock()

		err = orchestrator.DeleteSnapshot(ctx(), volume.Config.Name, snapshotName)
		if err != nil {
			t.Fatalf("%s: got unexpected error deleting snapshot: %v", s.name, err)
		}

		orchestrator.mutex.Lock()
		// Snapshot should not be registered in the store
		persistentSnapshot, err = orchestrator.storeClient.GetSnapshot(ctx(), volume.Config.Name, snapshotName)
		if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
			t.Errorf("%s: unable to communicate with backing store: %v", snapshotName, err)
		}
		if persistentSnapshot != nil {
			t.Errorf("%s: got snapshot when not expected.", snapshotName)
			continue
		}
		orchestrator.mutex.Unlock()
	}

	cleanup(t, orchestrator)
}

// TestGetProtocol tests the getProtocol function
// It checks for positive and negative cases
// Parameters:
//   volumeMode: Filesystem or RawBlock
//   accessMode: ReadWriteOnce, ReadOnlyMany, ReadWriteMany
//   protocol: File, Block, BlockOnFile
//   expected: File, Block, BlockOnFile
// Example:
//   volumeMode: RawBlock
//   accessMode: ReadWriteOnce
//   protocol: File
//   expected: ProtocolAny

func TestGetProtocol(t *testing.T) {
	orchestrator := getOrchestrator(t)

	type accessVariables struct {
		volumeMode config.VolumeMode
		accessMode config.AccessMode
		protocol   config.Protocol
		expected   config.Protocol
	}

	var accessModesPositiveTests = []accessVariables{
		{config.Filesystem, config.ModeAny, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ModeAny, config.File, config.File},
		{config.Filesystem, config.ModeAny, config.Block, config.Block},
		{config.Filesystem, config.ModeAny, config.BlockOnFile, config.BlockOnFile},
		{config.Filesystem, config.ReadWriteOnce, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ReadWriteOnce, config.File, config.File},
		{config.Filesystem, config.ReadWriteOnce, config.Block, config.Block},
		{config.Filesystem, config.ReadWriteOnce, config.BlockOnFile, config.BlockOnFile},
		{config.Filesystem, config.ReadOnlyMany, config.Block, config.Block},
		{config.Filesystem, config.ReadOnlyMany, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ReadOnlyMany, config.File, config.File},
		{config.Filesystem, config.ReadWriteMany, config.ProtocolAny, config.File},
		{config.Filesystem, config.ReadWriteMany, config.File, config.File},
		// {config.Filesystem, config.ReadWriteMany, config.Block, config.ProtocolAny},
		// {config.Filesystem, config.ReadWriteMany, config.BlockOnFile, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ModeAny, config.File, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.Block, config.Block},
		{config.RawBlock, config.ReadWriteOnce, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ReadWriteOnce, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteOnce, config.Block, config.Block},
		{config.RawBlock, config.ReadOnlyMany, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ReadOnlyMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadOnlyMany, config.Block, config.Block},
		{config.RawBlock, config.ReadWriteMany, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ReadWriteMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteMany, config.Block, config.Block},
	}

	var accessModesNegativeTests = []accessVariables{
		{config.Filesystem, config.ReadOnlyMany, config.BlockOnFile, config.ProtocolAny},
		{config.Filesystem, config.ReadWriteMany, config.Block, config.ProtocolAny},
		{config.Filesystem, config.ReadWriteMany, config.BlockOnFile, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.File, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.BlockOnFile, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteOnce, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteOnce, config.BlockOnFile, config.ProtocolAny},

		{config.RawBlock, config.ReadOnlyMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteMany, config.File, config.ProtocolAny},

		{config.RawBlock, config.ReadOnlyMany, config.BlockOnFile, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteMany, config.BlockOnFile, config.ProtocolAny},
	}

	for _, tc := range accessModesPositiveTests {
		protocolLocal, err := orchestrator.getProtocol(ctx(), tc.volumeMode, tc.accessMode, tc.protocol)
		assert.Nil(t, err, nil)
		assert.Equal(t, tc.expected, protocolLocal, "expected both the protocols to be equal!")
	}

	for _, tc := range accessModesNegativeTests {
		protocolLocal, err := orchestrator.getProtocol(ctx(), tc.volumeMode, tc.accessMode, tc.protocol)
		assert.NotNil(t, err)
		assert.Equal(t, tc.expected, protocolLocal, "expected both the protocols to be equal!")
	}
}

// TestGetBackend tests the GetBackend function
// It checks that the expected object is returned
// Parameters:
//    t *testing.T: The test object
// It returns nothing
// Example:
//     TestGetBackend(t)

func TestGetBackend(t *testing.T) {
	// Boilerplate mocking code
	mockCtrl := gomock.NewController(t)

	// Set fake values
	backendName := "foobar"
	backendUUID := "1234"
	// Create the expected return object
	expectedBackendExternal := &storage.BackendExternal{
		Name:        backendName,
		BackendUUID: backendUUID,
	}

	// Create a mocked backend
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	// Set backend behavior we don't care about for this testcase
	mockBackend.EXPECT().Name().Return(backendName).AnyTimes()        // Always return the fake name
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes() // Always return the fake uuid
	// Set backend behavior we do care about for this testcase
	mockBackend.EXPECT().ConstructExternal(gomock.Any()).Return(expectedBackendExternal) // Return the expected object

	// Create an instance of the orchestrator
	orchestrator := getOrchestrator(t)
	// Add the mocked backend to the orchestrator
	orchestrator.backends[backendUUID] = mockBackend

	// Run the test
	actualBackendExternal, err := orchestrator.GetBackend(ctx(), backendName)

	// Verify the results
	assert.Nilf(t, err, "Error getting backend; %v", err)
	assert.Equal(t, expectedBackendExternal, actualBackendExternal, "Did not get the expected backend object")
}

// TestGetBackendByBackendUUID tests the GetBackendByBackendUUID method
// It checks that the correct backend is returned
// Parameters:
//   backendName: Name of the backend to return
//   backendUUID: UUID of the backend to return
// It returns the expected backend object
// Example:
//   backend:=TestGetBackendByBackendUUID("foobar","1234")

func TestGetBackendByBackendUUID(t *testing.T) {
	// Boilerplate mocking code
	mockCtrl := gomock.NewController(t)

	// Set fake values
	backendName := "foobar"
	backendUUID := "1234"
	// Create the expected return object
	expectedBackendExternal := &storage.BackendExternal{
		Name:        backendName,
		BackendUUID: backendUUID,
	}

	// Create mocked backend that returns the expected object
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockBackend.EXPECT().ConstructExternal(gomock.Any()).Times(1).Return(expectedBackendExternal)

	// Create an instance of the orchestrator
	orchestrator := getOrchestrator(t)
	// Add the mocked backend to the orchestrator
	orchestrator.backends[backendUUID] = mockBackend

	// Run the test
	actualBackendExternal, err := orchestrator.GetBackendByBackendUUID(ctx(), backendUUID)

	// Verify the results
	assert.Nilf(t, err, "Error getting backend; %v", err)
	assert.Equal(t, expectedBackendExternal, actualBackendExternal, "Did not get the expected backend object")
}

// TestListBackends tests the ListBackends function
// It checks that the function returns a list of backends that matches the list of backends that were added to the orchestrator
// Parameters:
//     t *testing.T : go testing object used to control test flow
// It returns nothing
// Example:
//     TestListBackends(t *testing.T)

func TestListBackends(t *testing.T) {
	// Boilerplate mocking code
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Create list of 2 fake objects that we expect to be returned
	expectedBackendExternal1 := &storage.BackendExternal{
		Name:        "foo",
		BackendUUID: "12345",
	}
	expectedBackendExternal2 := &storage.BackendExternal{
		Name:        "bar",
		BackendUUID: "67890",
	}
	expectedBackendList := []*storage.BackendExternal{expectedBackendExternal1, expectedBackendExternal2}

	// Create 2 mocked backends that each return one of the expected fake objects when called
	mockBackend1 := mockstorage.NewMockBackend(mockCtrl)
	mockBackend1.EXPECT().ConstructExternal(gomock.Any()).Return(expectedBackendExternal1)
	mockBackend2 := mockstorage.NewMockBackend(mockCtrl)
	mockBackend2.EXPECT().ConstructExternal(gomock.Any()).Return(expectedBackendExternal2)

	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked backends to the orchestrator
	orchestrator.backends[expectedBackendExternal1.BackendUUID] = mockBackend1
	orchestrator.backends[expectedBackendExternal2.BackendUUID] = mockBackend2

	// Perform the test
	actualBackendList, err := orchestrator.ListBackends(ctx())

	// Verify the results
	assert.Nilf(t, err, "Error listing backends; %v", err)
	assert.ElementsMatch(t, expectedBackendList, actualBackendList, "Did not get expected list of backends")
}

// TestDeleteBackend tests the DeleteBackend method
// It checks that the backend is properly deleted from the orchestrator
// Parameters:
//    t *testing.T - test object
// It returns nothing
// Example:
//    TestDeleteBackend(t *testing.T)

func TestDeleteBackend(t *testing.T) {
	// Boilerplate mocking code
	mockCtrl := gomock.NewController(t)

	// Set fake values
	backendName := "foobar"
	backendUUID := "1234"

	// Create a mocked storage backend
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	// Set backend behavior we don't care about for this testcase
	mockBackend.EXPECT().Name().Return(backendName).AnyTimes()                  // Always return the fake name
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()           // Always return the fake UUID
	mockBackend.EXPECT().ConfigRef().Return("").AnyTimes()                      // Always return an empty configRef
	mockBackend.EXPECT().GetDriverName().Return("baz").AnyTimes()               // Always return a fake driver name
	mockBackend.EXPECT().Storage().Return(map[string]storage.Pool{}).AnyTimes() // Always return an empty storage list
	mockBackend.EXPECT().HasVolumes().Return(false).AnyTimes()                  // Always return no volumes
	// Set the backend behavior we do care about for this testcase
	mockBackend.EXPECT().SetState(storage.Deleting) // The backend should be set to deleting
	mockBackend.EXPECT().SetOnline(false)           // The backend should be set offline
	mockBackend.EXPECT().Terminate(gomock.Any())    // The backend should be terminated

	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Set the store client behavior we do care about for this testcase
	mockStoreClient.EXPECT().DeleteBackend(gomock.Any(), mockBackend).Return(nil)

	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	orchestrator.backends[backendUUID] = mockBackend

	// Perform the test
	err := orchestrator.DeleteBackend(ctx(), backendName)

	// Verify the results
	assert.Nilf(t, err, "Error getting backend; %v", err)
	_, ok := orchestrator.backends[backendUUID]
	assert.False(t, ok, "Backend was not properly deleted")
}

// TestPublishVolume tests the PublishVolume method
// It checks that the correct backend method is called
// Parameters:
//   t *testing.T
// Example:
//   TestPublishVolume(t)

func TestPublishVolume(t *testing.T) {
	// Boilerplate mocking code
	mockCtrl := gomock.NewController(t)

	// Set fake values
	backendUUID := "1234"

	// Create mocked backend that returns the expected object
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockBackend.EXPECT().PublishVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Times(1).Return(nil)

	// Create an instance of the orchestrator
	orchestrator := getOrchestrator(t)
	orchestrator.storeClient = mockStoreClient
	// Add the mocked backend to the orchestrator
	orchestrator.backends[backendUUID] = mockBackend
	volConfig := tu.GenerateVolumeConfig("fake-volume", 1, "fast", config.File)
	orchestrator.volumes["fake-volume"] = &storage.Volume{BackendUUID: backendUUID, Config: volConfig}

	// Run the test
	err := orchestrator.PublishVolume(ctx(), "fake-volume", &utils.VolumePublishInfo{})

	// Verify the results
	assert.Nilf(t, err, "Error publishing volume; %v", err)
}

// TestPublishVolumeFailedToUpdatePersistentStore tests the case where the
// orchestrator fails to update the persistent store after a successful
// publish operation.
// It checks that the orchestrator returns the error from the persistent
// store client.
// Parameters:
//    mockCtrl - the mocked controller
//    backendUUID - the UUID of the backend to use for the test
//    expectedError - the error expected from the persistent store client
// Example:
//    TestPublishVolumeFailedToUpdatePersistentStore(t, "1234", fmt.Errorf("failure"))

func TestPublishVolumeFailedToUpdatePersistentStore(t *testing.T) {
	// Boilerplate mocking code
	mockCtrl := gomock.NewController(t)

	// Set fake values
	backendUUID := "1234"
	expectedError := fmt.Errorf("failure")

	// Create mocked backend that returns the expected object
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockBackend.EXPECT().PublishVolume(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), gomock.Any()).Times(1).Return(expectedError)

	// Create an instance of the orchestrator
	orchestrator := getOrchestrator(t)
	orchestrator.storeClient = mockStoreClient
	// Add the mocked backend to the orchestrator
	orchestrator.backends[backendUUID] = mockBackend
	volConfig := tu.GenerateVolumeConfig("fake-volume", 1, "fast", config.File)
	orchestrator.volumes["fake-volume"] = &storage.Volume{BackendUUID: backendUUID, Config: volConfig}

	// Run the test
	err := orchestrator.PublishVolume(ctx(), "fake-volume", &utils.VolumePublishInfo{})

	// Verify the results
	if err != expectedError {
		t.Log("Did not get expected error from Publish Volume")
		t.Fail()
	}
}

// TestGetCHAP tests the GetCHAP method of the orchestrator
// It checks that the orchestrator calls the correct backend method
// and returns the expected result
// Parameters:
//    volumeName: the name of the volume to get CHAP info for
//    nodeName: the name of the node to get CHAP info for
// Example:
//    TestGetCHAP("foobar", "foobar")

func TestGetCHAP(t *testing.T) {
	// Boilerplate mocking code
	mockCtrl := gomock.NewController(t)

	// Set fake values
	backendUUID := "1234"
	volumeName := "foobar"
	volume := &storage.Volume{
		BackendUUID: backendUUID,
	}
	nodeName := "foobar"
	expectedChapInfo := &utils.IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "foo",
		IscsiInitiatorSecret: "bar",
		IscsiTargetUsername:  "baz",
		IscsiTargetSecret:    "biz",
	}

	// Create mocked backend that returns the expected object
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockBackend.EXPECT().GetChapInfo(gomock.Any(), volumeName, nodeName).Return(expectedChapInfo, nil)
	// Create an instance of the orchestrator
	orchestrator := getOrchestrator(t)
	// Add the mocked backend and fake volume to the orchestrator
	orchestrator.backends[backendUUID] = mockBackend
	orchestrator.volumes[volumeName] = volume
	actualChapInfo, err := orchestrator.GetCHAP(ctx(), volumeName, nodeName)
	assert.Nil(t, err, "Unexpected error")
	assert.Equal(t, expectedChapInfo, actualChapInfo, "Unexpected chap info returned.")
}

// TestGetCHAPFailure tests the failure path of GetCHAP
// It checks that the orchestrator returns an error when the backend returns an error
// Parameters:
//    t *testing.T: go test helper
// Example:
//    TestGetCHAPFailure(t *testing.T)

func TestGetCHAPFailure(t *testing.T) {
	// Boilerplate mocking code
	mockCtrl := gomock.NewController(t)

	// Set fake values
	backendUUID := "1234"
	volumeName := "foobar"
	volume := &storage.Volume{
		BackendUUID: backendUUID,
	}
	nodeName := "foobar"
	expectedError := fmt.Errorf("some error")

	// Create mocked backend that returns the expected object
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockBackend.EXPECT().GetChapInfo(gomock.Any(), volumeName, nodeName).Return(nil, expectedError)
	// Create an instance of the orchestrator
	orchestrator := getOrchestrator(t)
	// Add the mocked backend and fake volume to the orchestrator
	orchestrator.backends[backendUUID] = mockBackend
	orchestrator.volumes[volumeName] = volume
	actualChapInfo, actualErr := orchestrator.GetCHAP(ctx(), volumeName, nodeName)
	assert.Nil(t, actualChapInfo, "Unexpected CHAP info")
	assert.Equal(t, expectedError, actualErr, "Unexpected error")
}
