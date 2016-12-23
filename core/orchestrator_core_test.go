// Copyright 2016 NetApp, Inc. All Rights Reserved.

package core

import (
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"strings"
	"testing"

	log "github.com/Sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/drivers/fake"
	"github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/storage"
	backend_fake "github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_class"
	tu "github.com/netapp/trident/storage_class/test_utils"
)

var (
	etcdV2 = flag.String("etcd_v2", "", "etcd server (v2 API)")
	debug  = flag.Bool("debug", false, "Enable debugging output")

	inMemoryClient *persistent_store.InMemoryClient
)

func init() {
	flag.Parse()
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
	if *etcdV2 == "" {
		inMemoryClient = persistent_store.NewInMemoryClient()
	}
}

type deleteTest struct {
	name            string
	pool            *storage.StoragePool
	expectedSuccess bool
}

type recoveryTest struct {
	name          string
	volumeConfig  *storage.VolumeConfig
	expectDestroy bool
}

func cleanup(t *testing.T, o *tridentOrchestrator) {
	err := o.storeClient.DeleteBackends()
	if err != nil && err.Error() != persistent_store.KeyErrorMsg {
		t.Fatal("Unable to clean up backends:  ", err)
	}
	storageClasses, err := o.storeClient.GetStorageClasses()
	if err != nil && err.Error() != persistent_store.KeyErrorMsg {
		t.Fatal("Unable to retrieve storage classes:  ", err)
	} else if err == nil {
		for _, psc := range storageClasses {
			sc := storage_class.NewFromPersistent(psc)
			err := o.storeClient.DeleteStorageClass(sc)
			if err != nil {
				t.Fatalf("Unable to clean up storage class %s:  %v", sc.GetName(),
					err)
			}
		}
	}
	err = o.storeClient.DeleteVolumes()
	if err != nil && err.Error() != persistent_store.KeyErrorMsg {
		t.Fatal("Unable to clean up volumes:  ", err)
	}
	if *etcdV2 == "" {
		// Clear the InMemoryClient state so that it looks like we're
		// bootstrapping afresh next time.
		inMemoryClient.ClearAdded()
	}
}

func diffConfig(expected reflect.Value, got reflect.Value, fieldToSkip string,
	diffs []string) {
	expectedType := expected.Type()
	for i := 0; i < expectedType.NumField(); i++ {
		typeName := expectedType.Field(i).Name
		if typeName == fieldToSkip {
			continue
		}
		expectedValue := expected.FieldByName(typeName).Interface()
		gotValue := got.FieldByName(typeName).Interface()
		if expectedValue != nil && !reflect.DeepEqual(expectedValue,
			gotValue) {
			diffs = append(diffs, fmt.Sprintf("%s: expected %v, got %v",
				typeName, expectedValue, gotValue))
		}
	}
}

// To be called after reflect.DeepEqual has failed.
func diffExternalBackends(
	t *testing.T, expected, got *storage.StorageBackendExternal,
) {
	diffs := make([]string, 0)
	if expected.Name != got.Name {
		diffs = append(diffs,
			fmt.Sprintf("Name:  expected %s, got %s", expected.Name, got.Name))
	}
	if expected.Online != got.Online {
		diffs = append(diffs, fmt.Sprintf("Online:  expected %s, got %s",
			expected.Name, got.Name))
	}
	// Diff configs
	gotVal := reflect.ValueOf(got.Config)
	expectedVal := reflect.ValueOf(expected.Config)
	diffConfig(expectedVal.FieldByName("CommonStorageDriverConfig"),
		gotVal.FieldByName("CommonStorageDriverConfig"), "", diffs)
	diffConfig(expectedVal, gotVal, "CommonStorageDriverConfig", diffs)

	// Diff storage
	for name, expectedVC := range expected.Storage {
		if gotVC, ok := got.Storage[name]; !ok {
			diffs = append(diffs,
				fmt.Sprintf("Storage: did not get expected VC %s", name))
		} else if !reflect.DeepEqual(expectedVC, gotVC) {
			expectedJSON, err := json.Marshal(expectedVC)
			if err != nil {
				t.Fatal("Unable to marshal expected JSON for VC ", name)
			}
			gotJSON, err := json.Marshal(gotVC)
			if err != nil {
				t.Fatal("Unable to marshal got JSON for VC ", name)
			}
			diffs = append(diffs, fmt.Sprintf("Storage:  vc %s differs:\n\t\t"+
				"Expected: %s\n\t\tGot: %s", name, string(expectedJSON),
				string(gotJSON)))
		}
	}
	for name, _ := range got.Storage {
		if _, ok := expected.Storage[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("Storage:  got unexpected VC %s",
				name))
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
	for name, _ := range expectedVolMap {
		if _, ok := gotVolMap[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("Volumes:  did not get expected "+
				"volume %s", name))
		}
	}
	for name, _ := range gotVolMap {
		if _, ok := expectedVolMap[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("Volumes:  got unexpected "+
				"volume %s", name))
		}
	}
	if len(diffs) > 0 {
		t.Errorf("External backends differ:\n\t%s", strings.Join(diffs, "\n\t"))
	}
}

func runDeleteTest(
	t *testing.T, d *deleteTest, orchestrator *tridentOrchestrator,
) {
	found, err := orchestrator.DeleteVolume(d.name)
	if err == nil && !d.expectedSuccess {
		t.Errorf("%s:  volume delete succeeded when it should not have.",
			d.name)
	} else if err != nil && d.expectedSuccess {
		t.Errorf("%s:  delete failed with found %t:  %v", d.name,
			found, err)
	} else if d.expectedSuccess {
		volume := orchestrator.GetVolume(d.name)
		if volume != nil {
			t.Errorf("%s:  got volume where none expected.", d.name)
		}
		// d.pool is protected by the orchestrator mutex, so we need to
		// acquire it
		orchestrator.mutex.Lock()
		if _, ok := d.pool.Volumes[d.name]; ok {
			t.Errorf("%s:  volume not removed from parent pool.",
				d.name)
		}
		externalVol, err := orchestrator.storeClient.GetVolume(d.name)
		if err != nil {
			if err.Error() != persistent_store.KeyErrorMsg {
				t.Errorf("%s:  unable to communicate with backing store:  "+
					"%v", d.name, err)
			}
			// We're successful if we get to here; we expect an
			// ErrorCodeKeyNotFound.
		} else if externalVol != nil {
			t.Errorf("%s:  volume not properly deleted from backing "+
				"store", d.name)
		}
		orchestrator.mutex.Unlock()
	}
}

type storageClassTest struct {
	config   *storage_class.Config
	expected []*tu.PoolMatch
}

func getOrchestrator() *tridentOrchestrator {
	var (
		storeClient persistent_store.Client
		err         error
	)

	// If the user specified an etcd store, use that; otherwise, use an
	// in-memory store.  Keep both options available to avoid semantic drift
	// between the two stores (e.g., differing error conditions) causing
	// problems at a later time.
	if *etcdV2 != "" {
		log.Debug("Creating new etcd client.")
		// Note that this will panic if the etcd connection fails.
		storeClient, err = persistent_store.NewEtcdClient(*etcdV2)
		if err != nil {
			panic(err)
		}
	} else {
		log.Debug("Using in-memory client.")
		// This will have been created as not nil in init
		// We can't create a new one here because tests that exercise
		// bootstrapping need to have their data persist.
		storeClient = inMemoryClient
	}
	o := NewTridentOrchestrator(storeClient)
	// Ignore the return value here.
	o.Bootstrap()
	return o
}

func generateVolumeConfig(
	name string, gb int, storageClass string, protocol config.Protocol,
) *storage.VolumeConfig {
	return &storage.VolumeConfig{
		Name:            name,
		Size:            fmt.Sprintf("%d", gb*1024*1024*1024),
		Protocol:        protocol,
		StorageClass:    storageClass,
		SnapshotPolicy:  "none",
		SnapshotDir:     "none",
		UnixPermissions: "",
	}
}

func validateStorageClass(
	t *testing.T,
	o *tridentOrchestrator,
	name string,
	expected []*tu.PoolMatch,
) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	sc, ok := o.storageClasses[name]
	if !ok {
		t.Errorf("%s:  Storage class not found in backend.", name)
	}
	remaining := make([]*tu.PoolMatch, len(expected))
	for i, match := range expected {
		remaining[i] = match
	}
	for _, protocol := range []config.Protocol{config.File, config.Block} {
		for _, vc := range sc.GetStoragePoolsForProtocol(protocol) {
			nameFound := false
			for _, scName := range vc.StorageClasses {
				if scName == name {
					nameFound = true
					break
				}
			}
			if !nameFound {
				t.Errorf("%s: Storage class name not found in storage "+
					"pool %s", name, vc.Name)
			}
			matchIndex := -1
			for i, r := range remaining {
				if r.Matches(vc) {
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
				t.Errorf("%s:  Found unexpected match for storage class:  "+
					"%s:%s", name, vc.Backend.Name, vc.Name)
			}
		}
	}
	if len(remaining) > 0 {
		remainingNames := make([]string, len(remaining))
		for i, r := range remaining {
			remainingNames[i] = r.String()
		}
		t.Errorf("%s:  Storage class failed to match storage pools %s",
			name, strings.Join(remainingNames, ", "))
	}
	persistentSC, err := o.storeClient.GetStorageClass(name)
	if err != nil {
		t.Fatalf("Unable to get storage class %s from backend:  %v", name,
			err)
	}
	if !reflect.DeepEqual(persistentSC,
		sc.ConstructPersistent()) {
		gotSCJSON, err := json.Marshal(persistentSC)
		if err != nil {
			t.Fatalf("Unable to marshal persisted storage class %s:  %v",
				name, err)
		}
		expectedSCJSON, err := json.Marshal(sc.ConstructPersistent())
		if err != nil {
			t.Fatalf("Unable to marshal expected persistent storage class %s:"+
				"%v", name, err)
		}
		t.Errorf("%s:  Storage class persisted incorrectly.\n\tExpected %s\n\t"+
			"Got %s", name, expectedSCJSON, gotSCJSON)
	}
}

// This test is fairly heavyweight, but, due to the need to accumulate state
// to run the later tests, it's easier to do this all in one go at the moment.
// Consider breaking this up if it gets unwieldy, though.
func TestAddStorageClassVolumes(t *testing.T) {
	mockPools := tu.GetFakePools()
	orchestrator := getOrchestrator()

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
			name:     "slow-block",
			protocol: config.Block,
			poolNames: []string{tu.SlowNoSnapshots, tu.SlowSnapshots,
				"medium-overlap"},
		},
	} {
		pools := make(map[string]*fake.FakeStoragePool, len(c.poolNames))
		for _, poolName := range c.poolNames {
			pools[poolName] = mockPools[poolName]
		}
		config, err := fake.NewFakeStorageDriverConfigJSON(c.name, c.protocol,
			pools)
		if err != nil {
			t.Fatalf("Unable to generate config JSON for %s:  %v", c.name, err)
		}
		_, err = orchestrator.AddStorageBackend(config)
		if err != nil {
			t.Errorf("Unable to add backend %s:  %v", c.name, err)
			errored = true
		}
		orchestrator.mutex.Lock()
		backend, ok := orchestrator.backends[c.name]
		if !ok {
			t.Fatalf("Backend %s not stored in orchestrator", c.name)
		}
		persistentBackend, err := orchestrator.storeClient.GetBackend(
			c.name)
		if err != nil {
			t.Fatalf("Unable to get backend %s from persistent store:  %v",
				c.name, err)
		} else if !reflect.DeepEqual(backend.ConstructPersistent(),
			persistentBackend) {
			t.Error("Wrong data stored for backend ", c.name)
		}
		orchestrator.mutex.Unlock()
	}
	if errored {
		t.Fatal("Failed to add all backends; aborting remaining tests.")
	}

	// Add storage classes
	scTests := []storageClassTest{
		{
			config: &storage_class.Config{
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
			config: &storage_class.Config{
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
		{
			config: &storage_class.Config{
				Name: "fast-unique",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(2000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
					sa.UniqueOptions:    sa.NewStringRequest("baz"),
				},
			},
			expected: []*tu.PoolMatch{
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
		},
		{
			config: &storage_class.Config{
				Name: "specific",
				BackendStoragePools: map[string][]string{
					"fast-a":     []string{tu.FastThinOnly},
					"slow-block": []string{tu.SlowNoSnapshots, tu.MediumOverlap},
				},
			},
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-block", Pool: tu.MediumOverlap},
			},
		},
		{
			config: &storage_class.Config{
				Name: "mixed",
				BackendStoragePools: map[string][]string{
					"slow-file": []string{tu.SlowNoSnapshots},
					"fast-b":    []string{tu.FastThinOnly, tu.FastUniqueAttr},
				},
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
				{Backend: "slow-file", Pool: tu.SlowNoSnapshots},
			},
		},
	}
	for _, s := range scTests {
		_, err := orchestrator.AddStorageClass(s.config)
		if err != nil {
			t.Errorf("Unable to add storage class %s:  %v", s.config.Name, err)
			continue
		}
		validateStorageClass(t, orchestrator, s.config.Name, s.expected)
	}
	preSCDeleteTests := make([]*deleteTest, 0)
	postSCDeleteTests := make([]*deleteTest, 0)
	for _, s := range []struct {
		name            string
		config          *storage.VolumeConfig
		expectedSuccess bool
		expectedMatches []*tu.PoolMatch
		expectedCount   int
		deleteAfterSC   bool
	}{
		{
			name: "basic",
			config: generateVolumeConfig("basic", 1, "fast",
				config.File),
			expectedSuccess: true,
			expectedMatches: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
			expectedCount: 1,
			deleteAfterSC: false,
		},
		{
			name: "large",
			config: generateVolumeConfig("large", 100, "fast",
				config.File),
			expectedSuccess: false,
			expectedMatches: []*tu.PoolMatch{},
			expectedCount:   0,
			deleteAfterSC:   false,
		},
		{
			name: "block",
			config: generateVolumeConfig("block", 1, "specific",
				config.Block),
			expectedSuccess: true,
			expectedMatches: []*tu.PoolMatch{
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-block", Pool: tu.MediumOverlap},
			},
			expectedCount: 1,
			deleteAfterSC: false,
		},
		{
			name: "invalid-storage-class",
			config: generateVolumeConfig("invalid", 1, "nonexistent",
				config.File),
			expectedSuccess: false,
			expectedMatches: []*tu.PoolMatch{},
			expectedCount:   0,
			deleteAfterSC:   false,
		},
		{
			name: "repeated",
			config: generateVolumeConfig("basic", 20, "fast",
				config.File),
			expectedSuccess: false,
			expectedMatches: []*tu.PoolMatch{},
			expectedCount:   1,
			deleteAfterSC:   false,
		},
		{
			name: "postSCDelete",
			config: generateVolumeConfig("postSCDelete", 20, "fast",
				config.File),
			expectedSuccess: true,
			expectedMatches: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
			expectedCount: 1,
			deleteAfterSC: false,
		},
	} {
		vol, err := orchestrator.AddVolume(s.config)
		if err != nil && s.expectedSuccess {
			t.Errorf("%s:  got unexpected error %v", s.name, err)
			continue
		} else if err == nil && !s.expectedSuccess {
			t.Errorf("%s:  volume create succeeded unexpectedly.", s.name)
			continue
		}
		orchestrator.mutex.Lock()
		volume, found := orchestrator.volumes[s.config.Name]
		if s.expectedCount == 1 && !found {
			t.Errorf("%s:  did not get volume where expected.", s.name)
		} else if s.expectedCount == 0 && found {
			t.Errorf("%s:  got a volume where none expected.", s.name)
		}
		if !s.expectedSuccess {
			deleteTest := &deleteTest{
				name:            s.config.Name,
				pool:            nil,
				expectedSuccess: false,
			}
			if s.deleteAfterSC {
				postSCDeleteTests = append(postSCDeleteTests, deleteTest)
			} else {
				preSCDeleteTests = append(preSCDeleteTests, deleteTest)
			}
			orchestrator.mutex.Unlock()
			continue
		}
		matched := false
		for _, potentialMatch := range s.expectedMatches {
			if potentialMatch.Backend == volume.Backend.Name && potentialMatch.Pool == volume.Pool.Name {
				matched = true
				deleteTest := &deleteTest{
					name:            s.config.Name,
					pool:            volume.Pool,
					expectedSuccess: true,
				}
				if s.deleteAfterSC {
					postSCDeleteTests = append(postSCDeleteTests, deleteTest)
				} else {
					preSCDeleteTests = append(preSCDeleteTests, deleteTest)
				}
				break
			}
		}
		if !matched {
			t.Errorf("%s: Volume placed on unexpected backend and storage "+
				"pool:  %s, %s", s.name, volume.Backend.Name,
				volume.Pool.Name)
		}
		if poolVol, ok := volume.Pool.Volumes[s.config.Name]; !ok {
			t.Errorf("%s: Parent pool does not point to volume.", s.name)
		} else if !reflect.DeepEqual(poolVol, volume) {
			t.Errorf("%s: Parent pool points to different volume.", s.name)
		}

		externalVolume, err := orchestrator.storeClient.GetVolume(s.config.Name)
		if err != nil {
			t.Errorf("%s:  unable to communicate with backing store:  %v",
				s.name, err)
		}
		if !reflect.DeepEqual(externalVolume, vol) {
			t.Errorf("%s:  external volume %s stored in backend does not match"+
				" created volume.", s.name, externalVolume.Config.Name)
			externalVolJSON, err := json.Marshal(externalVolume)
			if err != nil {
				t.Fatal("Unable to remarshal JSON:  ", err)
			}
			origVolJSON, err := json.Marshal(vol)
			if err != nil {
				t.Fatal("Unable to remarshal JSON:  ", err)
			}
			t.Logf("\tExpected: %s\n\tGot: %s\n", string(externalVolJSON),
				string(origVolJSON))
		}
		orchestrator.mutex.Unlock()
	}
	for _, d := range preSCDeleteTests {
		runDeleteTest(t, d, orchestrator)
	}

	// Delete service classes.  Note:  there are currently no error cases.
	for _, s := range scTests {
		found, err := orchestrator.DeleteStorageClass(s.config.Name)
		if err != nil {
			t.Errorf("%s delete: Unable to remove storage class", s.config.Name)
		}
		if !found {
			t.Errorf("%s delete: Storage class not found", s.config.Name)
		}
		orchestrator.mutex.Lock()
		if _, ok := orchestrator.storageClasses[s.config.Name]; ok {
			t.Errorf("%s delete: Storage class still found in map.",
				s.config.Name)
		}
		// Ensure that the storage class was cleared from its backends.
		for _, poolMatch := range s.expected {
			b, ok := orchestrator.backends[poolMatch.Backend]
			if !ok {
				t.Errorf("%s delete:  backend %s not found in orchestrator.",
					s.config.Name, poolMatch.Backend)
				continue
			}
			p, ok := b.Storage[poolMatch.Pool]
			if !ok {
				t.Errorf("%s delete: storage pool %s not found for backend"+
					" %s", s.config.Name, poolMatch.Pool, poolMatch.Backend)
				continue
			}
			found = false
			for _, sc := range p.StorageClasses {
				if sc == s.config.Name {
					t.Errorf("%s delete:  storage class name not removed "+
						"from backend %s, storage pool %s", s.config.Name,
						poolMatch.Backend, poolMatch.Pool)
				}
			}
		}
		externalSC, err := orchestrator.storeClient.GetStorageClass(
			s.config.Name)
		if err != nil {
			if err.Error() != persistent_store.KeyErrorMsg {
				t.Errorf("%s:  unable to communicate with backing store:  "+
					"%v", s.config.Name, err)
			}
			// We're successful if we get to here; we expect an
			// ErrorCodeKeyNotFound.
		} else if externalSC != nil {
			t.Errorf("%s:  storageClass not properly deleted from backing "+
				"store", s.config.Name)
		}
		orchestrator.mutex.Unlock()
	}
	for _, d := range postSCDeleteTests {
		runDeleteTest(t, d, orchestrator)
	}
	cleanup(t, orchestrator)
}

func addBackend(
	t *testing.T, orchestrator *tridentOrchestrator, backendName string,
) {
	configJSON, err := fake.NewFakeStorageDriverConfigJSON(
		backendName,
		config.File,
		map[string]*fake.FakeStoragePool{
			"primary": &fake.FakeStoragePool{
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
	)
	if err != nil {
		t.Fatal("Unable to create mock driver config JSON: ", err)
	}
	_, err = orchestrator.AddStorageBackend(configJSON)
	if err != nil {
		t.Fatal("Unable to add initial backend:  ", err)
	}
}

// addBackendStorageClass creates a backend and storage class for tests
// that don't care deeply about this functionality.
func addBackendStorageClass(
	t *testing.T,
	orchestrator *tridentOrchestrator,
	backendName string,
	scName string,
) {
	addBackend(t, orchestrator, backendName)
	_, err := orchestrator.AddStorageClass(
		&storage_class.Config{
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

func TestBackendUpdateAndDelete(t *testing.T) {
	const (
		backendName       = "updateBackend"
		scName            = "updateBackendTest"
		newSCName         = "updateBackendTest2"
		volumeName        = "updateVolume"
		offlineVolumeName = "offlineVolume"
	)
	// Test setup
	orchestrator := getOrchestrator()
	addBackendStorageClass(t, orchestrator, backendName, scName)

	orchestrator.mutex.Lock()
	sc, ok := orchestrator.storageClasses[scName]
	if !ok {
		t.Fatal("Storage class not found in orchestrator map")
	}
	orchestrator.mutex.Unlock()

	_, err := orchestrator.AddVolume(generateVolumeConfig(volumeName, 50, scName,
		config.File))
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}
	orchestrator.mutex.Lock()
	volume, ok := orchestrator.volumes[volumeName]
	if !ok {
		t.Fatal("Volume name found in orchestrator map.")
	}
	orchestrator.mutex.Unlock()

	// Test updates that should succeed
	previousBackends := make([]*storage.StorageBackend, 1)
	previousBackends[0] = orchestrator.backends[backendName]
	for _, c := range []struct {
		name  string
		pools map[string]*fake.FakeStoragePool
	}{
		{
			name: "New pool",
			pools: map[string]*fake.FakeStoragePool{
				"primary": &fake.FakeStoragePool{
					Attrs: map[string]sa.Offer{
						sa.Media:            sa.NewStringOffer("hdd"),
						sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
						sa.TestingAttribute: sa.NewBoolOffer(true),
					},
					Bytes: 100 * 1024 * 1024 * 1024,
				},
				"secondary": &fake.FakeStoragePool{
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
			pools: map[string]*fake.FakeStoragePool{
				"primary": &fake.FakeStoragePool{
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
			pools: map[string]*fake.FakeStoragePool{
				"primary": &fake.FakeStoragePool{
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
		newConfigJSON, err := fake.NewFakeStorageDriverConfigJSON(backendName,
			config.File, c.pools)
		if err != nil {
			t.Errorf("%s:  unable to generate new backend config:  %v", c.name,
				err)
			continue
		}
		_, err = orchestrator.AddStorageBackend(newConfigJSON)
		if err != nil {
			t.Errorf("%s:  unable to update backend with a nonconflicting "+
				"change:  %v", c.name, err)
			continue
		}
		orchestrator.mutex.Lock()
		newBackend := orchestrator.backends[backendName]
		vcs := sc.GetStoragePoolsForProtocol(config.File)
		foundNewBackend := false
		for _, vc := range vcs {
			for i, b := range previousBackends {
				if vc.Backend == b {
					t.Errorf("%s:  backend %d not cleared from storage class",
						c.name, i+1)
				}
				if vc.Backend == newBackend {
					foundNewBackend = true
				}
			}
		}
		if !foundNewBackend {
			t.Errorf("%s:  Storage class does not point to new backend.",
				c.name)
		}
		matchingVC, ok := newBackend.Storage["primary"]
		if !ok {
			t.Errorf("%s: storage pool for volume not found", c.name)
			continue
		}
		if len(matchingVC.Volumes) != 1 {
			t.Errorf("%s: unexpected number of volumes found in main "+
				"storage pool: %d", c.name, len(matchingVC.Volumes))
		}
		if len(matchingVC.StorageClasses) != 1 {
			t.Errorf("%s: unexpected number of storage classes for main "+
				"storage pool: %d", c.name, len(matchingVC.StorageClasses))
		}
		if _, ok := matchingVC.Volumes[volumeName]; !ok {
			t.Errorf("%s: volume not found in expected storage pool.", c.name)
		}
		if volume.Backend != newBackend {
			t.Errorf("%s:  volume backend does not point to the new backend",
				c.name)
		}
		if volume.Pool != matchingVC {
			t.Errorf("%s: volume does not point to the right storage pool.",
				c.name)
		}
		persistentBackend, err := orchestrator.storeClient.GetBackend(
			backendName)
		if err != nil {
			t.Error("Unable to retrieve backend from store client:  ", err)
		} else if !reflect.DeepEqual(newBackend.ConstructPersistent(),
			persistentBackend) {
			t.Errorf("Backend not correctly updated in persistent store.")
		}
		previousBackends = append(previousBackends, newBackend)
		orchestrator.mutex.Unlock()
	}
	backend := previousBackends[len(previousBackends)-1]
	pool := volume.Pool

	// Test updates that should fail.
	for _, c := range []struct {
		name     string
		pools    map[string]*fake.FakeStoragePool
		protocol config.Protocol
	}{
		{
			name: "Renamed pool",
			pools: map[string]*fake.FakeStoragePool{
				"disk": &fake.FakeStoragePool{
					Attrs: map[string]sa.Offer{
						sa.Media:            sa.NewStringOffer("hdd"),
						sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
						sa.TestingAttribute: sa.NewBoolOffer(true),
					},
					Bytes: 100 * 1024 * 1024 * 1024,
				},
			},
			protocol: config.File,
		},
		{
			name: "Wrong attributes",
			pools: map[string]*fake.FakeStoragePool{
				"primary": &fake.FakeStoragePool{
					Attrs: map[string]sa.Offer{
						sa.Media:            sa.NewStringOffer("hdd"),
						sa.ProvisioningType: sa.NewStringOffer("thin"),
						sa.TestingAttribute: sa.NewBoolOffer(true),
					},
					Bytes: 100 * 1024 * 1024 * 1024,
				},
			},
			protocol: config.File,
		},
		{
			name: "Wrong protocol",
			pools: map[string]*fake.FakeStoragePool{
				"primary": &fake.FakeStoragePool{
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
			protocol: config.Block,
		},
	} {
		newConfigJSON, err := fake.NewFakeStorageDriverConfigJSON(backendName,
			c.protocol, c.pools)
		if err != nil {
			t.Errorf("%s:  unable to generate new backend config:  %v", c.name,
				err)
			continue
		}
		_, err = orchestrator.AddStorageBackend(newConfigJSON)
		if err == nil {
			t.Errorf("%s:  invalid backend update completed successfully.",
				c.name)
		}
		orchestrator.mutex.Lock()
		if volume.Backend != backend {
			t.Errorf("%s: backend changed for volume.", c.name)
		}
		if volume.Pool != pool {
			t.Errorf("%s: storage pool changed for volume.", c.name)
		}
		vcs := sc.GetStoragePoolsForProtocol(config.File)
		if len(vcs) != 1 {
			t.Errorf("%s:  %d storage pools found for storage class; "+
				"expected 1", c.name, len(vcs))
		} else if vcs[0] != pool {
			t.Errorf("%s:  found wrong storage pool in storage class.",
				c.name)
		}
		persistentBackend, err := orchestrator.storeClient.GetBackend(
			backendName)
		if err != nil {
			t.Error("Unable to retrieve backend from store client:  ", err)
		} else if !reflect.DeepEqual(backend.ConstructPersistent(),
			persistentBackend) {
			t.Errorf("Backend erroneously updated in persistent store.")
		}
		orchestrator.mutex.Unlock()
	}

	// Test backend offlining.
	found, err := orchestrator.OfflineBackend(backendName)
	if !found {
		t.Fatal("Backend not found in orchestrator.")
	}
	if err != nil {
		t.Fatal("Unable to offline backend:  ", err)
	}
	_, err = orchestrator.AddVolume(generateVolumeConfig(offlineVolumeName, 50,
		scName, config.File))
	if err == nil {
		t.Error("Created volume volume on offline backend.")
	}
	orchestrator.mutex.Lock()
	vcs := sc.GetStoragePoolsForProtocol(config.File)
	if len(vcs) == 1 {
		t.Error("Offline backend not removed from storage pool in " +
			"storage class.")
	}
	if volume.Backend != backend {
		t.Error("Backend changed for volume after offlining.")
	}
	if volume.Pool != pool {
		t.Error("Storage pool changed for volume after backend offlined.")
	}
	persistentBackend, err := orchestrator.storeClient.GetBackend(backendName)
	if err != nil {
		t.Error("Unable to retrieve backend from store client after offlining:"+
			"  ", err)
	} else if persistentBackend.Online {
		t.Error("Online not set to true in the backend.")
	}
	orchestrator.mutex.Unlock()

	// Ensure that new storage classes do not get the offline backend assigned
	// to them.
	newSCExternal, err := orchestrator.AddStorageClass(
		&storage_class.Config{
			Name: newSCName,
			Attributes: map[string]sa.Request{
				sa.Media:            sa.NewStringRequest("hdd"),
				sa.TestingAttribute: sa.NewBoolRequest(true),
			},
		},
	)
	if err != nil {
		t.Fatal("Unable to add new storage class after offlining:  ", err)
	}
	if _, ok = newSCExternal.StoragePools[backendName]; ok {
		t.Error("Offline backend added to new storage class.")
	}

	// Test that online gets set properly after bootstrapping.
	newOrchestrator := getOrchestrator()
	// We need to lock the orchestrator mutex here because we call
	// ConstructExternal on the original backend in the else if clause.
	orchestrator.mutex.Lock()
	if bootstrappedBackend := newOrchestrator.GetBackend(backendName); bootstrappedBackend == nil {
		t.Error("Unable to find backend after bootstrapping.")
	} else if !reflect.DeepEqual(bootstrappedBackend,
		backend.ConstructExternal()) {
		diffExternalBackends(t, backend.ConstructExternal(),
			bootstrappedBackend)
	}
	orchestrator.mutex.Unlock()

	newOrchestrator.mutex.Lock()
	for _, name := range []string{scName, newSCName} {
		newSC, ok := newOrchestrator.storageClasses[name]
		if !ok {
			t.Fatalf("Unable to find storage class %s after bootstrapping.",
				name)
		}
		vcs = newSC.GetStoragePoolsForProtocol(config.File)
		if len(vcs) == 1 {
			t.Errorf("Offline backend readded to storage class %s after "+
				"bootstrapping.", name)
		}
	}
	newOrchestrator.mutex.Unlock()

	// Test that deleting the volume causes the backend to be deleted.
	_, err = orchestrator.DeleteVolume(volumeName)
	if err != nil {
		t.Fatal("Unable to delete volume for offline backend:  ", err)
	}
	persistentBackend, err = orchestrator.storeClient.GetBackend(backendName)
	if err == nil {
		t.Error("Backend remained on store client after deleting the last " +
			"volume present.")
	}
	orchestrator.mutex.Lock()
	_, ok = orchestrator.backends[backendName]
	if ok {
		t.Error("Empty offlined backend not removed from memory.")
	}
	orchestrator.mutex.Unlock()
	cleanup(t, orchestrator)
}

func TestEmptyBackendDeletion(t *testing.T) {
	const (
		backendName = "emptyBackend"
	)

	orchestrator := getOrchestrator()
	// Note that we don't care about the storage class here, but it's easier
	// to reuse functionality.
	addBackendStorageClass(t, orchestrator, backendName, "none")
	found, err := orchestrator.OfflineBackend(backendName)
	if err != nil {
		t.Fatal("Unable to offline backend:  ", err)
	} else if !found {
		t.Fatalf("Backend %s not found in orchestrator", backendName)
	}
	_, err = orchestrator.storeClient.GetBackend(backendName)
	if err == nil {
		t.Error("Empty backend remained on store client after offlining")
	}
	orchestrator.mutex.Lock()
	_, ok := orchestrator.backends[backendName]
	if ok {
		t.Error("Empty offlined backend not removed from memory.")
	}
	orchestrator.mutex.Unlock()
	cleanup(t, orchestrator)
}

func TestBackendCleanup(t *testing.T) {
	const (
		offlineBackendName = "cleanupBackend"
		onlineBackendName  = "onlineBackend"
		scName             = "cleanupBackendTest"
		volumeName         = "cleanupVolume"
	)

	orchestrator := getOrchestrator()
	addBackendStorageClass(t, orchestrator, offlineBackendName, scName)
	_, err := orchestrator.AddVolume(generateVolumeConfig(volumeName, 50,
		scName, config.File))
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}

	// This needs to go after the volume addition to ensure that the volume
	// ends up on the backend to be offflined.
	addBackend(t, orchestrator, onlineBackendName)

	found, err := orchestrator.OfflineBackend(offlineBackendName)
	if err != nil {
		t.Fatal("Unable to offline backend.")
	}
	if !found {
		t.Fatal("Backend %s not found when trying to offline.",
			offlineBackendName)
	}
	// Simulate deleting the existing volume and then bootstrapping
	orchestrator.mutex.Lock()
	vol, ok := orchestrator.volumes[volumeName]
	if !ok {
		t.Fatal("Unable to find volume %s in backend.", volumeName)
	}
	err = orchestrator.storeClient.DeleteVolume(vol)
	if err != nil {
		t.Fatal("Unable to delete volume from etcd:  ", err)
	}
	orchestrator.mutex.Unlock()

	newOrchestrator := getOrchestrator()
	if bootstrappedBackend := newOrchestrator.GetBackend(offlineBackendName); bootstrappedBackend != nil {
		t.Error("Empty offline backend not deleted during bootstrap.")
	}
	if bootstrappedBackend := newOrchestrator.GetBackend(onlineBackendName); bootstrappedBackend == nil {
		t.Error("Empty online backend deleted during bootstrap.")
	}
}

func TestLoadBackend(t *testing.T) {
	const (
		backendName = "load-backend-test"
	)
	orchestrator := getOrchestrator()
	configJSON, err := fake.NewFakeStorageDriverConfigJSON(
		backendName,
		config.File,
		map[string]*fake.FakeStoragePool{
			"primary": &fake.FakeStoragePool{
				Attrs: map[string]sa.Offer{
					sa.Media:            sa.NewStringOffer("hdd"),
					sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
					sa.TestingAttribute: sa.NewBoolOffer(true),
				},
				Bytes: 100 * 1024 * 1024 * 1024,
			},
		},
	)
	originalBackend, err := orchestrator.AddStorageBackend(configJSON)
	if err != nil {
		t.Fatal("Unable to initially add backend:  ", err)
	}
	persistentBackend, err := orchestrator.storeClient.GetBackend(backendName)
	if err != nil {
		t.Fatal("Unable to retrieve backend from store client:  ", err)
	}
	// Note that this will register as an update, but it should be close enough
	newConfig, err := persistentBackend.MarshalConfig()
	if err != nil {
		t.Fatal("Unable to marshal config from stored backend:  ", err)
	}
	newBackend, err := orchestrator.AddStorageBackend(newConfig)
	if err != nil {
		t.Error("Unable to add backend from config:  ", err)
	} else if !reflect.DeepEqual(newBackend, originalBackend) {
		t.Error("Newly loaded backend differs.")
	}

	newOrchestrator := getOrchestrator()
	if bootstrappedBackend := newOrchestrator.GetBackend(backendName); bootstrappedBackend == nil {
		t.Error("Unable to find backend after bootstrapping.")
	} else if !reflect.DeepEqual(bootstrappedBackend, originalBackend) {
		t.Errorf("External backends differ.")
		diffExternalBackends(t, originalBackend, bootstrappedBackend)
	}
	cleanup(t, orchestrator)
}

func prepRecoveryTest(
	t *testing.T, orchestrator *tridentOrchestrator, backendName, scName string,
) {
	configJSON, err := fake.NewFakeStorageDriverConfigJSON(
		backendName,
		config.File,
		map[string]*fake.FakeStoragePool{
			"primary": &fake.FakeStoragePool{
				Attrs: map[string]sa.Offer{
					sa.Media:            sa.NewStringOffer("hdd"),
					sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
					sa.RecoveryTest:     sa.NewBoolOffer(true),
				},
				Bytes: 100 * 1024 * 1024 * 1024,
			},
		},
	)
	_, err = orchestrator.AddStorageBackend(configJSON)
	if err != nil {
		t.Fatal("Unable to initialize backend: ", err)
	}
	_, err = orchestrator.AddStorageClass(
		&storage_class.Config{
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

func runRecoveryTests(
	t *testing.T,
	orchestrator *tridentOrchestrator,
	backendName string,
	op persistent_store.VolumeOperation,
	testCases []recoveryTest,
) {
	for _, c := range testCases {
		// Manipulate the persistent store directly, since it's
		// easier to store the results of a partially completed volume addition
		// than to actually inject a failure.
		volTxn := &persistent_store.VolumeTransaction{
			Config: c.volumeConfig,
			Op:     op,
		}
		err := orchestrator.storeClient.AddVolumeTransaction(volTxn)
		if err != nil {
			t.Fatalf("%s: Unable to create volume transaction:  %v", c.name,
				err)
		}
		newOrchestrator := getOrchestrator()
		newOrchestrator.mutex.Lock()
		if _, ok := newOrchestrator.volumes[c.volumeConfig.Name]; ok {
			t.Errorf("%s: volume still present in orchestrator.", c.name)
			// Note:  assume that if the volume's still present in the
			// top-level map, it's present everywhere else and that, if it's
			// absent there, it's absent everywhere else in memory
		}
		backend, ok := newOrchestrator.backends[backendName]
		if !ok {
			t.Fatalf("%s:  Backend not found after bootstrapping.", c.name)
		}
		f := backend.Driver.(*backend_fake.FakeStorageDriver)
		// Destroy should be always called on the backend
		if _, ok = f.DestroyedVolumes[f.GetInternalVolumeName(
			c.volumeConfig.Name)]; !ok && c.expectDestroy {
			t.Errorf("%s:  Destroy not called on volume.", c.name)
		}
		_, err = newOrchestrator.storeClient.GetVolume(c.volumeConfig.Name)
		if err != nil {
			if err.Error() != persistent_store.KeyErrorMsg {
				t.Errorf("%s:  unable to communicate with backing store:  "+
					"%v", c.name, err)
			}
		} else {
			t.Errorf("%s:  Found VolumeConfig still stored in etcd.", c.name)
		}
		if txns, err := newOrchestrator.storeClient.GetVolumeTransactions(); err != nil {
			t.Errorf("%s: Unable to retrieve transactions from backing store: "+
				" %v", c.name, err)
		} else if len(txns) > 0 {
			t.Errorf("%s:  Transaction not cleared from the backing store",
				c.name)
		}
		newOrchestrator.mutex.Unlock()
	}
}

func TestAddVolumeRecovery(t *testing.T) {
	const (
		backendName      = "addRecoveryBackend"
		scName           = "addRecoveryBackendSC"
		fullVolumeName   = "addRecoveryVolumeFull"
		txOnlyVolumeName = "addRecoveryVolumeTxOnly"
	)
	orchestrator := getOrchestrator()
	prepRecoveryTest(t, orchestrator, backendName, scName)
	// It's easier to add the volume and then reinject the transaction begin
	// afterwards
	fullVolumeConfig := generateVolumeConfig(fullVolumeName, 50, scName,
		config.File)
	_, err := orchestrator.AddVolume(fullVolumeConfig)
	if err != nil {
		t.Fatal("Unable to add volume: ", err)
	}
	txOnlyVolumeConfig := generateVolumeConfig(txOnlyVolumeName, 50, scName,
		config.File)
	// BEGIN actual test
	runRecoveryTests(t, orchestrator, backendName, persistent_store.AddVolume,
		[]recoveryTest{
			{name: "full", volumeConfig: fullVolumeConfig, expectDestroy: true},
			{name: "txOnly", volumeConfig: txOnlyVolumeConfig, expectDestroy: true},
		})
	cleanup(t, orchestrator)
}

func TestDeleteVolumeRecovery(t *testing.T) {
	const (
		backendName      = "deleteRecoveryBackend"
		scName           = "deleteRecoveryBackendSC"
		fullVolumeName   = "deleteRecoveryVolumeFull"
		txOnlyVolumeName = "deleteRecoveryVolumeTxOnly"
	)
	orchestrator := getOrchestrator()
	prepRecoveryTest(t, orchestrator, backendName, scName)
	// For the full test, we delete everything but the ending transaction.
	fullVolumeConfig := generateVolumeConfig(fullVolumeName, 50, scName,
		config.File)
	_, err := orchestrator.AddVolume(fullVolumeConfig)
	if err != nil {
		t.Fatal("Unable to add volume: ", err)
	}
	_, err = orchestrator.DeleteVolume(fullVolumeName)
	if err != nil {
		t.Fatal("Unable to remove full volume:  ", err)
	}
	txOnlyVolumeConfig := generateVolumeConfig(txOnlyVolumeName, 50, scName,
		config.File)
	_, err = orchestrator.AddVolume(txOnlyVolumeConfig)
	if err != nil {
		t.Fatal("Unable to add tx only volume: ", err)
	}
	// BEGIN actual test
	runRecoveryTests(t, orchestrator, backendName,
		persistent_store.DeleteVolume, []recoveryTest{
			{name: "full", volumeConfig: fullVolumeConfig,
				expectDestroy: false},
			{name: "txOnly", volumeConfig: txOnlyVolumeConfig,
				expectDestroy: true},
		})
	cleanup(t, orchestrator)
}

func TestBadBootstrap(t *testing.T) {
	storeClient, err := persistent_store.NewEtcdClient("invalidIPAddress")
	if err != nil {
		panic(err)
	}
	o := NewTridentOrchestrator(storeClient)
	err = o.Bootstrap()
	if err == nil {
		t.Errorf("Did not get error for invalid bootstrap attempt.")
	}
}

// The next series of tests test that bootstrap doesn't exit early if it
// encounters a key error for one of the main types of entries.
func TestStorageClassOnlyBootstrap(t *testing.T) {
	const scName = "storageclass-only"

	orchestrator := getOrchestrator()
	originalSC, err := orchestrator.AddStorageClass(
		&storage_class.Config{
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
	newOrchestrator := getOrchestrator()
	if bootstrappedSC := newOrchestrator.GetStorageClass(scName); bootstrappedSC == nil {
		t.Error("Unable to find storage class after bootstrapping.")
	} else if !reflect.DeepEqual(bootstrappedSC, originalSC) {
		t.Errorf("External storage classs differ:\n\tOriginal:  %v\n\t."+
			"Bootstrapped:  %v", originalSC, bootstrappedSC)
	}
	cleanup(t, orchestrator)
}

func TestFirstVolumeRecovery(t *testing.T) {
	const (
		backendName      = "firstRecoveryBackend"
		scName           = "firstRecoveryBackendSC"
		fullVolumeName   = "firstRecoveryVolumeFull"
		txOnlyVolumeName = "firstRecoveryVolumeTxOnly"
	)
	orchestrator := getOrchestrator()
	prepRecoveryTest(t, orchestrator, backendName, scName)
	txOnlyVolumeConfig := generateVolumeConfig(txOnlyVolumeName, 50, scName,
		config.File)
	// BEGIN actual test
	runRecoveryTests(t, orchestrator, backendName, persistent_store.AddVolume,
		[]recoveryTest{
			{name: "firstTXOnly", volumeConfig: txOnlyVolumeConfig,
				expectDestroy: true},
		})
	cleanup(t, orchestrator)
}
