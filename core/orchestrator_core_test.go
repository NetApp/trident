// Copyright 2019 NetApp, Inc. All Rights Reserved.

package core

import (
	"encoding/json"
	"flag"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/netapp/trident/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
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
	etcdV2 = flag.String("etcd_v2", "", "etcd server (v2 API)")
	etcdV3 = flag.String("etcd_v3", "", "etcd server (v3 API)")
	debug  = flag.Bool("debug", false, "Enable debugging output")

	inMemoryClient *persistentstore.InMemoryClient
)

func init() {
	testing.Init()
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
	if *etcdV2 == "" && *etcdV3 == "" {
		inMemoryClient = persistentstore.NewInMemoryClient()
	}
}

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

func cleanup(t *testing.T, o *TridentOrchestrator) {
	err := o.storeClient.DeleteBackends()
	if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatal("Unable to clean up backends:  ", err)
	}
	storageClasses, err := o.storeClient.GetStorageClasses()
	if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatal("Unable to retrieve storage classes:  ", err)
	} else if err == nil {
		for _, psc := range storageClasses {
			sc := storageclass.NewFromPersistent(psc)
			err := o.storeClient.DeleteStorageClass(sc)
			if err != nil {
				t.Fatalf("Unable to clean up storage class %s:  %v", sc.GetName(),
					err)
			}
		}
	}
	err = o.storeClient.DeleteVolumes()
	if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatal("Unable to clean up volumes:  ", err)
	}
	err = o.storeClient.DeleteSnapshots()
	if err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatal("Unable to clean up snapshots:  ", err)
	}
	if *etcdV2 == "" && *etcdV3 == "" {
		// Clear the InMemoryClient state so that it looks like we're
		// bootstrapping afresh next time.
		inMemoryClient.Stop()
	}
}

func cleanupStoreVersion(t *testing.T, etcd string) {
	// Deleting etcdv2 persistent state version
	etcdv2Client, err := persistentstore.NewEtcdClientV2(etcd)
	if err != nil {
		t.Fatalf("Creating etcdv2 client failed: %v", err)
	}
	if err = etcdv2Client.Delete(config.StoreURL); err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatalf("Couldn't delete etcdv2 persistent state version: %v", err)
	}
	if err = etcdv2Client.Stop(); err != nil {
		t.Fatalf("Couldn't shut down etcdv2 client: %v", err)
	}
	// Deleting etcdv3 persistent state version
	etcdv3Client, err := persistentstore.NewEtcdClientV3(etcd)
	if err != nil {
		t.Fatalf("Creating etcdv3 client failed: %v", err)
	}
	if err = etcdv3Client.Delete(config.StoreURL); err != nil && !persistentstore.MatchKeyNotFoundErr(err) {
		t.Fatalf("Couldn't delete etcdv3 persistent state version: %v", err)
	}
	if err = etcdv3Client.Stop(); err != nil {
		t.Fatalf("Couldn't shut down etcdv3 client: %v", err)
	}
}

func diffConfig(expected, got interface{}, fieldToSkip string, t *testing.T) []string {

	diffs := make([]string, 0, 0)
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
			diffs = append(diffs, fmt.Sprintf("%s: expected %v, got %v",
				typeName, expectedField, gotField))
		}
	}

	return diffs
}

// To be called after reflect.DeepEqual has failed.
func diffExternalBackends(t *testing.T, expected, got *storage.BackendExternal) {

	diffs := make([]string, 0)

	if expected.Name != got.Name {
		diffs = append(diffs,
			fmt.Sprintf("Name:  expected %s, got %s", expected.Name, got.Name))
	}
	if expected.State != got.State {
		diffs = append(diffs,
			fmt.Sprintf("Online:  expected %s, got %s", expected.State, got.State))
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
	configDiffs = diffConfig(expectedCSDCIntf, gotCSDCIntf, "", t)
	diffs = append(diffs, configDiffs...)

	// Compare the base config, without the common storage driver config
	configDiffs = diffConfig(expectedConfig, gotConfig, "CommonStorageDriverConfig", t)
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
			diffs = append(diffs, fmt.Sprintf("Storage:  pool %s differs:\n\t\t"+
				"Expected: %s\n\t\tGot: %s", name, string(expectedJSON), string(gotJSON)))
		}
	}
	for name := range got.Storage {
		if _, ok := expected.Storage[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("Storage:  got unexpected VC %s", name))
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
			diffs = append(diffs, fmt.Sprintf("Volumes:  did not get expected volume %s", name))
		}
	}
	for name := range gotVolMap {
		if _, ok := expectedVolMap[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("Volumes:  got unexpected volume %s", name))
		}
	}
	if len(diffs) > 0 {
		t.Errorf("External backends differ:\n\t%s", strings.Join(diffs, "\n\t"))
	}
}

func runDeleteTest(
	t *testing.T, d *deleteTest, orchestrator *TridentOrchestrator,
) {
	var (
		backendUUID string
		backend     *storage.Backend
		found       bool
	)
	if d.expectedSuccess {
		orchestrator.mutex.Lock()

		backendUUID = orchestrator.volumes[d.name].BackendUUID
		backend, found = orchestrator.backends[backendUUID]
		if !found {
			t.Errorf("Backend %v isn't managed by the orchestrator!", backendUUID)
		}
		if _, found = backend.Volumes[d.name]; !found {
			t.Errorf("Volume %s doesn't exist on backend %s!", d.name, backendUUID)
		}
		orchestrator.mutex.Unlock()
	}
	err := orchestrator.DeleteVolume(d.name)
	if err == nil && !d.expectedSuccess {
		t.Errorf("%s:  volume delete succeeded when it should not have.", d.name)
	} else if err != nil && d.expectedSuccess {
		t.Errorf("%s:  delete failed:  %v", d.name, err)
	} else if d.expectedSuccess {
		volume, err := orchestrator.GetVolume(d.name)
		if volume != nil || err == nil {
			t.Errorf("%s:  got volume where none expected.", d.name)
		}
		orchestrator.mutex.Lock()
		if _, found = backend.Volumes[d.name]; found {
			t.Errorf("Volume %s shouldn't exist on backend %s!", d.name, backendUUID)
		}
		externalVol, err := orchestrator.storeClient.GetVolume(d.name)
		if err != nil {
			if !persistentstore.MatchKeyNotFoundErr(err) {
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
	config   *storageclass.Config
	expected []*tu.PoolMatch
}

func getOrchestrator() *TridentOrchestrator {
	var (
		storeClient persistentstore.Client
		err         error
	)

	// If the user specified an etcd store, use that; otherwise, use an
	// in-memory store.  Keep both options available to avoid semantic drift
	// between the two stores (e.g., differing error conditions) causing
	// problems at a later time.
	if *etcdV2 != "" {
		log.Debug("Creating new etcdv2 client.")
		// Note that this will panic if the etcd connection fails.
		storeClient, err = persistentstore.NewEtcdClientV2(*etcdV2)
		if err != nil {
			panic(err)
		}
	} else if *etcdV3 != "" {
		log.Debug("Creating new etcdv3 client.")
		// Note that this will panic if the etcd connection fails.
		storeClient, err = persistentstore.NewEtcdClientV3(*etcdV3)
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
	if err = o.Bootstrap(); err != nil {
		log.Fatal("Failure occurred during bootstrapping: ", err)
	}
	return o
}

func generateVolumeConfig(
	name string, gb int, storageClass string, protocol config.Protocol,
) *storage.VolumeConfig {
	return &storage.VolumeConfig{
		Name:            name,
		InternalName:    name,
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
	o *TridentOrchestrator,
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
		for _, pool := range sc.GetStoragePoolsForProtocol(protocol) {
			nameFound := false
			for _, scName := range pool.StorageClasses {
				if scName == name {
					nameFound = true
					break
				}
			}
			if !nameFound {
				t.Errorf("%s: Storage class name not found in storage "+
					"pool %s", name, pool.Name)
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
				t.Errorf("%s:  Found unexpected match for storage class:  "+
					"%s:%s", name, pool.Backend.Name, pool.Name)
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
			name:      "slow-block",
			protocol:  config.Block,
			poolNames: []string{tu.SlowNoSnapshots, tu.SlowSnapshots, tu.MediumOverlap},
		},
	} {
		pools := make(map[string]*fake.StoragePool, len(c.poolNames))
		for _, poolName := range c.poolNames {
			pools[poolName] = mockPools[poolName]
		}
		volumes := make([]fake.Volume, 0)
		fakeConfig, err := fakedriver.NewFakeStorageDriverConfigJSON(c.name, c.protocol, pools, volumes)
		if err != nil {
			t.Fatalf("Unable to generate config JSON for %s:  %v", c.name, err)
		}
		_, err = orchestrator.AddBackend(fakeConfig)
		if err != nil {
			t.Errorf("Unable to add backend %s:  %v", c.name, err)
			errored = true
		}
		orchestrator.mutex.Lock()
		backend, err := orchestrator.getBackendByBackendName(c.name)
		if err != nil {
			t.Fatalf("Backend %s not stored in orchestrator, err %s", c.name, err)
		}
		persistentBackend, err := orchestrator.storeClient.GetBackend(c.name)
		if err != nil {
			t.Fatalf("Unable to get backend %s from persistent store:  %v",
				c.name, err)
		} else if !reflect.DeepEqual(backend.ConstructPersistent(), persistentBackend) {
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
		{
			config: &storageclass.Config{
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
			config: &storageclass.Config{
				Name: "pools",
				Pools: map[string][]string{
					"fast-a":     {tu.FastSmall},
					"slow-block": {tu.SlowNoSnapshots, tu.MediumOverlap},
				},
			},
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-block", Pool: tu.MediumOverlap},
			},
		},
		{
			config: &storageclass.Config{
				Name: "additionalPools",
				AdditionalPools: map[string][]string{
					"fast-a":     {tu.FastThinOnly},
					"slow-block": {tu.SlowNoSnapshots, tu.MediumOverlap},
				},
			},
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-block", Pool: tu.MediumOverlap},
			},
		},
		{
			config: &storageclass.Config{
				Name: "poolsWithAttributes",
				Attributes: map[string]sa.Request{
					sa.IOPS:      sa.NewIntRequest(2000),
					sa.Snapshots: sa.NewBoolRequest(true),
				},
				Pools: map[string][]string{
					"fast-a":     {tu.FastThinOnly},
					"slow-block": {tu.SlowNoSnapshots, tu.MediumOverlap},
				},
			},
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastThinOnly},
			},
		},
		{
			config: &storageclass.Config{
				Name: "additionalPoolsWithAttributes",
				Attributes: map[string]sa.Request{
					sa.IOPS:      sa.NewIntRequest(2000),
					sa.Snapshots: sa.NewBoolRequest(true),
				},
				AdditionalPools: map[string][]string{
					"fast-a":     {tu.FastThinOnly},
					"slow-block": {tu.SlowNoSnapshots},
				},
			},
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
			},
		},
		{
			config: &storageclass.Config{
				Name: "additionalPoolsWithAttributesAndPools",
				Attributes: map[string]sa.Request{
					sa.IOPS:      sa.NewIntRequest(2000),
					sa.Snapshots: sa.NewBoolRequest(true),
				},
				Pools: map[string][]string{
					"fast-a":     {tu.FastThinOnly},
					"slow-block": {tu.SlowNoSnapshots, tu.MediumOverlap},
				},
				AdditionalPools: map[string][]string{
					"fast-b":     {tu.FastThinOnly},
					"slow-block": {tu.SlowNoSnapshots},
				},
			},
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
			},
		},
		{
			config: &storageclass.Config{
				Name: "additionalPoolsNoMatch",
				AdditionalPools: map[string][]string{
					"unknown": {tu.FastThinOnly},
				},
			},
			expected: []*tu.PoolMatch{},
		},
		{
			config: &storageclass.Config{
				Name: "mixed",
				AdditionalPools: map[string][]string{
					"slow-file": {tu.SlowNoSnapshots},
					"fast-b":    {tu.FastThinOnly, tu.FastUniqueAttr},
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
		{
			config: &storageclass.Config{
				Name: "emptyStorageClass",
			},
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
				{Backend: "slow-file", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-file", Pool: tu.SlowSnapshots},
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-block", Pool: tu.SlowSnapshots},
				{Backend: "slow-block", Pool: tu.MediumOverlap},
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
			config: generateVolumeConfig("block", 1, "pools",
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
			name: "block2",
			config: generateVolumeConfig("block2", 1, "additionalPools",
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
			volumeBackend, err := orchestrator.getBackendByBackendUUID(volume.BackendUUID)
			if volumeBackend == nil || err != nil {
				continue
			}
			//if potentialMatch.Backend == volume.Backend &&
			if potentialMatch.Backend == volumeBackend.Name &&
				potentialMatch.Pool == volume.Pool {
				matched = true
				deleteTest := &deleteTest{
					name:            s.config.Name,
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
			t.Errorf("%s: Volume placed on unexpected backend and storage pool:  %s, %s",
				s.name,
				volume.BackendUUID,
				volume.Pool)
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

	// Delete storage classes.  Note:  there are currently no error cases.
	for _, s := range scTests {
		err := orchestrator.DeleteStorageClass(s.config.Name)
		if err != nil {
			t.Errorf("%s delete: Unable to remove storage class: %v", s.config.Name, err)
		}
		orchestrator.mutex.Lock()
		if _, ok := orchestrator.storageClasses[s.config.Name]; ok {
			t.Errorf("%s delete: Storage class still found in map.",
				s.config.Name)
		}
		// Ensure that the storage class was cleared from its backends.
		for _, poolMatch := range s.expected {
			b, err := orchestrator.getBackendByBackendName(poolMatch.Backend)
			if b == nil || err != nil {
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
			for _, sc := range p.StorageClasses {
				if sc == s.config.Name {
					t.Errorf("%s delete:  storage class name not removed "+
						"from backend %s, storage pool %s", s.config.Name,
						poolMatch.Backend, poolMatch.Pool)
				}
			}
		}
		externalSC, err := orchestrator.storeClient.GetStorageClass(s.config.Name)
		if err != nil {
			if !persistentstore.MatchKeyNotFoundErr(err) {
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

// This test is modeled after TestAddStorageClassVolumes, but we don't need all the
// tests around storage class deletion, etc.
func TestCloneVolumes(t *testing.T) {
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
			name:      "slow-block",
			protocol:  config.Block,
			poolNames: []string{tu.SlowNoSnapshots, tu.SlowSnapshots, tu.MediumOverlap},
		},
	} {
		pools := make(map[string]*fake.StoragePool, len(c.poolNames))
		for _, poolName := range c.poolNames {
			pools[poolName] = mockPools[poolName]
		}

		volumes := make([]fake.Volume, 0)
		cfg, err := fakedriver.NewFakeStorageDriverConfigJSON(c.name, c.protocol,
			pools, volumes)
		if err != nil {
			t.Fatalf("Unable to generate cfg JSON for %s:  %v", c.name, err)
		}
		_, err = orchestrator.AddBackend(cfg)
		if err != nil {
			t.Errorf("Unable to add backend %s:  %v", c.name, err)
			errored = true
		}
		orchestrator.mutex.Lock()
		backend, err := orchestrator.getBackendByBackendName(c.name)
		if backend == nil || err != nil {
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
		{
			config: &storageclass.Config{
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
			config: &storageclass.Config{
				Name: "specific",
				AdditionalPools: map[string][]string{
					"fast-a":     {tu.FastThinOnly},
					"slow-block": {tu.SlowNoSnapshots, tu.MediumOverlap},
				},
			},
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-block", Pool: tu.MediumOverlap},
			},
		},
		{
			config: &storageclass.Config{
				Name: "specificNoMatch",
				AdditionalPools: map[string][]string{
					"unknown": {tu.FastThinOnly},
				},
			},
			expected: []*tu.PoolMatch{},
		},
		{
			config: &storageclass.Config{
				Name: "mixed",
				AdditionalPools: map[string][]string{
					"slow-file": {tu.SlowNoSnapshots},
					"fast-b":    {tu.FastThinOnly, tu.FastUniqueAttr},
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
		{
			config: &storageclass.Config{
				Name: "emptyStorageClass",
			},
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
				{Backend: "slow-file", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-file", Pool: tu.SlowSnapshots},
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-block", Pool: tu.SlowSnapshots},
				{Backend: "slow-block", Pool: tu.MediumOverlap},
			},
		},
	}
	for _, s := range storageClasses {
		_, err := orchestrator.AddStorageClass(s.config)
		if err != nil {
			t.Errorf("Unable to add storage class %s:  %v", s.config.Name, err)
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
			config:          generateVolumeConfig("file", 1, "fast", config.File),
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
			config:          generateVolumeConfig("block", 1, "specific", config.Block),
			expectedSuccess: true,
			expectedMatches: []*tu.PoolMatch{
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-block", Pool: tu.MediumOverlap},
			},
		},
	} {
		// Create the source volume
		_, err := orchestrator.AddVolume(s.config)
		if err != nil {
			t.Errorf("%s:  got unexpected error %v", s.name, err)
			continue
		}

		// Now clone the volume and ensure everything looks fine
		cloneName := s.config.Name + "_clone"
		cloneConfig := &storage.VolumeConfig{
			Name:              cloneName,
			StorageClass:      s.config.StorageClass,
			CloneSourceVolume: s.config.Name,
		}
		cloneResult, err := orchestrator.CloneVolume(cloneConfig)
		if err != nil {
			t.Errorf("%s:  got unexpected error %v", s.name, err)
			continue
		}

		orchestrator.mutex.Lock()

		volume, found := orchestrator.volumes[s.config.Name]
		if !found {
			t.Errorf("%s:  did not get volume where expected.", s.name)
		}
		clone, found := orchestrator.volumes[cloneName]
		if !found {
			t.Errorf("%s:  did not get volume clone where expected.", cloneName)
		}

		// Clone must reside in the same place as the source
		if clone.BackendUUID != volume.BackendUUID {
			t.Errorf("%s: Clone placed on unexpected backend: %s", cloneName, clone.BackendUUID)
		}

		// Clone should be registered in the store just like any other volume
		externalClone, err := orchestrator.storeClient.GetVolume(cloneName)
		if err != nil {
			t.Errorf("%s:  unable to communicate with backing store:  %v", cloneName, err)
		}
		if !reflect.DeepEqual(externalClone, cloneResult) {
			t.Errorf("%s:  external volume %s stored in backend does not match"+
				" created volume.", cloneName, externalClone.Config.Name)
			externalCloneJSON, err := json.Marshal(externalClone)
			if err != nil {
				t.Fatal("Unable to remarshal JSON:  ", err)
			}
			origCloneJSON, err := json.Marshal(cloneResult)
			if err != nil {
				t.Fatal("Unable to remarshal JSON:  ", err)
			}
			t.Logf("\tExpected: %s\n\tGot: %s\n", string(externalCloneJSON), string(origCloneJSON))
		}

		orchestrator.mutex.Unlock()
	}

	cleanup(t, orchestrator)
}

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
	_, err = orchestrator.AddBackend(configJSON)
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
		&storageclass.Config{
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
		backendProtocol   = config.File
	)
	// Test setup
	orchestrator := getOrchestrator()
	addBackendStorageClass(t, orchestrator, backendName, scName, backendProtocol)

	orchestrator.mutex.Lock()
	sc, ok := orchestrator.storageClasses[scName]
	if !ok {
		t.Fatal("Storage class not found in orchestrator map")
	}
	orchestrator.mutex.Unlock()

	_, err := orchestrator.AddVolume(generateVolumeConfig(volumeName, 50, scName, config.File))
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}

	orchestrator.mutex.Lock()
	volume, ok := orchestrator.volumes[volumeName]
	if !ok {
		t.Fatalf("Volume %s not tracked by the orchestrator!", volumeName)
	}
	log.WithFields(log.Fields{
		"volume.BackendUUID": volume.BackendUUID,
		"volume.Config.Name": volume.Config.Name,
		"volume.Config":      volume.Config,
	}).Debug("Found volume.")
	startingBackend, err := orchestrator.getBackendByBackendName(backendName)
	if startingBackend == nil || err != nil {
		t.Fatalf("Backend %s not stored in orchestrator", backendName)
	}
	if _, ok = startingBackend.Volumes[volumeName]; !ok {
		t.Fatalf("Volume %s not tracked by the backend %s!", volumeName, backendName)
	}
	orchestrator.mutex.Unlock()

	// Test updates that should succeed
	previousBackends := make([]*storage.Backend, 1)
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
		if !previousBackend.Driver.Initialized() {
			t.Errorf("Backend %s is not initialized", backendName)
		}

		var volumes []fake.Volume
		newConfigJSON, err := fakedriver.NewFakeStorageDriverConfigJSON(backendName,
			config.File, c.pools, volumes)
		if err != nil {
			t.Errorf("%s:  unable to generate new backend config:  %v", c.name, err)
			continue
		}
		_, err = orchestrator.UpdateBackend(backendName, newConfigJSON)
		if err != nil {
			t.Errorf("%s:  unable to update backend with a nonconflicting change:  %v", c.name, err)
			continue
		}

		orchestrator.mutex.Lock()
		newBackend, err := orchestrator.getBackendByBackendName(backendName)
		if newBackend == nil || err != nil {
			t.Fatalf("Backend %s not stored in orchestrator", backendName)
		}
		if previousBackend.Driver.Initialized() {
			t.Errorf("Previous backend %s still initialized", backendName)
		}
		if !newBackend.Driver.Initialized() {
			t.Errorf("Updated backend %s is not initialized.", backendName)
		}
		pools := sc.GetStoragePoolsForProtocol(config.File)
		foundNewBackend := false
		for _, pool := range pools {
			for i, b := range previousBackends {
				if pool.Backend == b {
					t.Errorf("%s:  backend %d not cleared from storage class",
						c.name, i+1)
				}
				if pool.Backend == newBackend {
					foundNewBackend = true
				}
			}
		}
		if !foundNewBackend {
			t.Errorf("%s:  Storage class does not point to new backend.", c.name)
		}
		matchingPool, ok := newBackend.Storage["primary"]
		if !ok {
			t.Errorf("%s: storage pool for volume not found", c.name)
			continue
		}
		if len(matchingPool.StorageClasses) != 1 {
			t.Errorf("%s: unexpected number of storage classes for main "+
				"storage pool: %d", c.name, len(matchingPool.StorageClasses))
		}
		volumeBackend, err := orchestrator.getBackendByBackendUUID(volume.BackendUUID)
		if volumeBackend == nil || err != nil {
			for backendUUID, backend := range orchestrator.backends {
				log.WithFields(log.Fields{
					"volume.BackendUUID":  volume.BackendUUID,
					"backend":             backend,
					"backend.BackendUUID": backend.BackendUUID,
					"uuid":                backendUUID,
				}).Debug("Found backend.")
			}
			t.Fatalf("Backend %s not stored in orchestrator, err: %v", volume.BackendUUID, err)
		}
		if volumeBackend != newBackend {
			t.Errorf("%s:  volume backend does not point to the new backend", c.name)
		}
		if volume.Pool != matchingPool.Name {
			t.Errorf("%s: volume does not point to the right storage pool.", c.name)
		}
		persistentBackend, err := orchestrator.storeClient.GetBackend(backendName)
		if err != nil {
			t.Error("Unable to retrieve backend from store client:  ", err)
		} else if !cmp.Equal(newBackend.ConstructPersistent(), persistentBackend) {
			if diff := cmp.Diff(newBackend.ConstructPersistent(), persistentBackend); diff != "" {
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
	err = orchestrator.DeleteBackend(backendName)
	if err != nil {
		t.Fatalf("Unable to delete backend:  %v", err)
	}
	if !backend.Driver.Initialized() {
		t.Errorf("Deleted backend with volumes %s is not initialized.", backendName)
	}
	_, err = orchestrator.AddVolume(generateVolumeConfig(offlineVolumeName, 50, scName, config.File))
	if err == nil {
		t.Error("Created volume volume on offline backend.")
	}
	orchestrator.mutex.Lock()
	pools := sc.GetStoragePoolsForProtocol(config.File)
	if len(pools) == 1 {
		t.Error("Offline backend not removed from storage pool in " +
			"storage class.")
	}
	foundBackend, err := orchestrator.getBackendByBackendUUID(volume.BackendUUID)
	if err != nil {
		t.Error("Couldn't find backend.", err)
	}
	if foundBackend != backend {
		t.Error("Backend changed for volume after offlining.")
	}
	if volume.Pool != pool {
		t.Error("Storage pool changed for volume after backend offlined.")
	}
	persistentBackend, err := orchestrator.storeClient.GetBackend(backendName)
	if err != nil {
		t.Error("Unable to retrieve backend from store client after offlining:"+
			"  ", err)
	} else if persistentBackend.State.IsOnline() {
		t.Error("Online not set to true in the backend.")
	}
	orchestrator.mutex.Unlock()

	// Ensure that new storage classes do not get the offline backend assigned
	// to them.
	newSCExternal, err := orchestrator.AddStorageClass(
		&storageclass.Config{
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
	if bootstrappedBackend, _ := newOrchestrator.GetBackend(backendName); bootstrappedBackend == nil {
		t.Error("Unable to find backend after bootstrapping.")
	} else if !reflect.DeepEqual(bootstrappedBackend, backend.ConstructExternal()) {
		diffExternalBackends(t, backend.ConstructExternal(), bootstrappedBackend)
	}

	orchestrator.mutex.Unlock()

	newOrchestrator.mutex.Lock()
	for _, name := range []string{scName, newSCName} {
		newSC, ok := newOrchestrator.storageClasses[name]
		if !ok {
			t.Fatalf("Unable to find storage class %s after bootstrapping.",
				name)
		}
		pools = newSC.GetStoragePoolsForProtocol(config.File)
		if len(pools) == 1 {
			t.Errorf("Offline backend readded to storage class %s after "+
				"bootstrapping.", name)
		}
	}
	newOrchestrator.mutex.Unlock()

	// Test that deleting the volume causes the backend to be deleted.
	err = orchestrator.DeleteVolume(volumeName)
	if err != nil {
		t.Fatal("Unable to delete volume for offline backend:  ", err)
	}
	if backend.Driver.Initialized() {
		t.Errorf("Deleted backend %s is still initialized.", backendName)
	}
	persistentBackend, err = orchestrator.storeClient.GetBackend(backendName)
	if err == nil {
		t.Error("Backend remained on store client after deleting the last " +
			"volume present.")
	}
	orchestrator.mutex.Lock()

	missingBackend, _ := orchestrator.getBackendByBackendName(backendName)
	if missingBackend != nil {
		t.Error("Empty offlined backend not removed from memory.")
	}
	orchestrator.mutex.Unlock()
	cleanup(t, orchestrator)
}

func TestEmptyBackendDeletion(t *testing.T) {
	const (
		backendName     = "emptyBackend"
		backendProtocol = config.File
	)

	orchestrator := getOrchestrator()
	// Note that we don't care about the storage class here, but it's easier
	// to reuse functionality.
	addBackendStorageClass(t, orchestrator, backendName, "none", backendProtocol)
	backend, errLookup := orchestrator.getBackendByBackendName(backendName)
	if backend == nil || errLookup != nil {
		t.Fatalf("Backend %s not stored in orchestrator", backendName)
	}

	err := orchestrator.DeleteBackend(backendName)
	if err != nil {
		t.Fatalf("Unable to delete backend: %v", err)
	}
	if backend.Driver.Initialized() {
		t.Errorf("Deleted backend %s is still initialized.", backendName)
	}
	_, err = orchestrator.storeClient.GetBackend(backendName)
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

func TestBootstrapSnapshotMissingVolume(t *testing.T) {
	const (
		offlineBackendName = "snapNoVolBackend"
		scName             = "snapNoVolSC"
		volumeName         = "snapNoVolVolume"
		snapName           = "snapNoVolSnapshot"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator()
	defer cleanup(t, orchestrator)
	addBackendStorageClass(t, orchestrator, offlineBackendName, scName, backendProtocol)
	_, err := orchestrator.AddVolume(generateVolumeConfig(volumeName, 50,
		scName, config.File))
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}

	// For the full test, we create everything and recreate the AddSnapshot transaction.
	snapshotConfig := generateSnapshotConfig(snapName, volumeName, volumeName)
	if _, err := orchestrator.CreateSnapshot(snapshotConfig); err != nil {
		t.Fatal("Unable to add snapshot: ", err)
	}

	// Simulate deleting the existing volume without going through Trident then bootstrapping
	vol, ok := orchestrator.volumes[volumeName]
	if !ok {
		t.Fatalf("Unable to find volume %s in backend.", volumeName)
	}
	orchestrator.mutex.Lock()
	err = orchestrator.storeClient.DeleteVolume(vol)
	if err != nil {
		t.Fatalf("Unable to delete volume from etcd: %v", err)
	}
	orchestrator.mutex.Unlock()

	newOrchestrator := getOrchestrator()
	bootstrappedSnapshot, _ := newOrchestrator.GetSnapshot(snapshotConfig.VolumeName, snapshotConfig.Name)
	if bootstrappedSnapshot == nil {
		t.Error("Volume not found during bootstrap.")
	}
	if !bootstrappedSnapshot.State.IsMissingVolume() {
		t.Error("Unexpected snapshot state.")
	}
	//delete volume in missing_volume state
	err = newOrchestrator.DeleteSnapshot(volumeName, snapName)
	if err != nil {
		t.Error("could not delete snapshot with missing volume")
	}
}

func TestBootstrapSnapshotMissingBackend(t *testing.T) {
	const (
		offlineBackendName = "snapNoBackBackend"
		scName             = "snapNoBackSC"
		volumeName         = "snapNoBackVolume"
		snapName           = "snapNoBackSnapshot"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator()
	defer cleanup(t, orchestrator)
	addBackendStorageClass(t, orchestrator, offlineBackendName, scName, backendProtocol)
	_, err := orchestrator.AddVolume(generateVolumeConfig(volumeName, 50,
		scName, config.File))
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}

	// For the full test, we create everything and recreate the AddSnapshot transaction.
	snapshotConfig := generateSnapshotConfig(snapName, volumeName, volumeName)
	if _, err := orchestrator.CreateSnapshot(snapshotConfig); err != nil {
		t.Fatal("Unable to add snapshot: ", err)
	}

	// Simulate deleting the existing backend without going through Trident then bootstrapping
	backend, err := orchestrator.getBackendByBackendName(offlineBackendName)
	if err != nil {
		t.Fatalf("Unable to get backend from etcd: %v", err)
	}
	orchestrator.mutex.Lock()
	err = orchestrator.storeClient.DeleteBackend(backend)
	if err != nil {
		t.Fatalf("Unable to delete volume from etcd: %v", err)
	}
	orchestrator.mutex.Unlock()

	newOrchestrator := getOrchestrator()
	bootstrappedSnapshot, _ := newOrchestrator.GetSnapshot(snapshotConfig.VolumeName, snapshotConfig.Name)
	if bootstrappedSnapshot == nil {
		t.Error("Volume not found during bootstrap.")
	}
	if !bootstrappedSnapshot.State.IsMissingBackend() {
		t.Error("Unexpected snapshot state.")
	}
	//delete snapshot in missing_backend state
	err = newOrchestrator.DeleteSnapshot(volumeName, snapName)
	if err != nil {
		t.Error("could not delete snapshot with missing backend")
	}
}

func TestBootstrapVolumeMissingBackend(t *testing.T) {
	const (
		offlineBackendName = "bootstrapVolBackend"
		scName             = "bootstrapVolSC"
		volumeName         = "bootstrapVolVolume"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator()
	defer cleanup(t, orchestrator)
	addBackendStorageClass(t, orchestrator, offlineBackendName, scName, backendProtocol)
	_, err := orchestrator.AddVolume(generateVolumeConfig(volumeName, 50,
		scName, config.File))
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}

	// Simulate deleting the existing backend without going through Trident then bootstrapping
	backend, err := orchestrator.getBackendByBackendName(offlineBackendName)
	if err != nil {
		t.Fatalf("Unable to get backend from etcd: %v", err)
	}
	orchestrator.mutex.Lock()
	err = orchestrator.storeClient.DeleteBackend(backend)
	if err != nil {
		t.Fatalf("Unable to delete volume from etcd: %v", err)
	}
	orchestrator.mutex.Unlock()

	newOrchestrator := getOrchestrator()
	bootstrappedVolume, _ := newOrchestrator.GetVolume(volumeName)
	if bootstrappedVolume == nil {
		t.Error("volume not found during bootstrap")
	}
	if !bootstrappedVolume.State.IsMissingBackend() {
		t.Error("unexpected volume state")
	}

	//delete volume in missing_backend state
	err = newOrchestrator.DeleteVolume(volumeName)
	if err != nil {
		t.Error("could not delete volume with missing backend")
	}
}

func TestBackendCleanup(t *testing.T) {
	const (
		offlineBackendName = "cleanupBackend"
		onlineBackendName  = "onlineBackend"
		scName             = "cleanupBackendTest"
		volumeName         = "cleanupVolume"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator()
	addBackendStorageClass(t, orchestrator, offlineBackendName, scName, backendProtocol)
	_, err := orchestrator.AddVolume(generateVolumeConfig(volumeName, 50,
		scName, config.File))
	if err != nil {
		t.Fatal("Unable to create volume: ", err)
	}

	// This needs to go after the volume addition to ensure that the volume
	// ends up on the backend to be offlined.
	addBackend(t, orchestrator, onlineBackendName, backendProtocol)

	err = orchestrator.DeleteBackend(offlineBackendName)
	if err != nil {
		t.Fatalf("Unable to delete backend %s: %v", offlineBackendName, err)
	}
	// Simulate deleting the existing volume and then bootstrapping
	orchestrator.mutex.Lock()
	vol, ok := orchestrator.volumes[volumeName]
	if !ok {
		t.Fatalf("Unable to find volume %s in backend.", volumeName)
	}
	err = orchestrator.storeClient.DeleteVolume(vol)
	if err != nil {
		t.Fatalf("Unable to delete volume from etcd: %v", err)
	}
	orchestrator.mutex.Unlock()

	newOrchestrator := getOrchestrator()
	if bootstrappedBackend, _ := newOrchestrator.GetBackend(offlineBackendName); bootstrappedBackend != nil {
		t.Error("Empty offline backend not deleted during bootstrap.")
	}
	if bootstrappedBackend, _ := newOrchestrator.GetBackend(onlineBackendName); bootstrappedBackend == nil {
		t.Error("Empty online backend deleted during bootstrap.")
	}
}

func TestLoadBackend(t *testing.T) {
	const (
		backendName = "load-backend-test"
	)
	// volumes must be nil in order to satisfy reflect.DeepEqual comparision. It isn't recommended to compare slices with deepEqual
	var volumes []fake.Volume
	orchestrator := getOrchestrator()
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
	originalBackend, err := orchestrator.AddBackend(configJSON)
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
	newBackend, err := orchestrator.AddBackend(newConfig)
	if err != nil {
		t.Error("Unable to update backend from config:  ", err)
	} else if !reflect.DeepEqual(newBackend, originalBackend) {
		t.Error("Newly loaded backend differs.")
	}

	newOrchestrator := getOrchestrator()
	if bootstrappedBackend, _ := newOrchestrator.GetBackend(backendName); bootstrappedBackend == nil {
		t.Error("Unable to find backend after bootstrapping.")
	} else if !reflect.DeepEqual(bootstrappedBackend, originalBackend) {
		t.Errorf("External backends differ.")
		diffExternalBackends(t, originalBackend, bootstrappedBackend)
	}
	cleanup(t, orchestrator)
}

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
	_, err = orchestrator.AddBackend(configJSON)
	if err != nil {
		t.Fatal("Unable to initialize backend: ", err)
	}
	_, err = orchestrator.AddStorageClass(
		&storageclass.Config{
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
		backend, err := newOrchestrator.getBackendByBackendName(backendName)
		if backend == nil || err != nil {
			t.Fatalf("%s:  Backend not found after bootstrapping.", c.name)
		}
		f := backend.Driver.(*fakedriver.StorageDriver)
		// Destroy should be always called on the backend
		if _, ok := f.DestroyedVolumes[f.GetInternalVolumeName(
			c.volumeConfig.Name)]; !ok && c.expectDestroy {
			t.Errorf("%s:  Destroy not called on volume.", c.name)
		}
		_, err = newOrchestrator.storeClient.GetVolume(c.volumeConfig.Name)
		if err != nil {
			if !persistentstore.MatchKeyNotFoundErr(err) {
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
	runRecoveryTests(t, orchestrator, backendName, storage.AddVolume,
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
	fullVolumeConfig := generateVolumeConfig(fullVolumeName, 50, scName, config.File)
	if _, err := orchestrator.AddVolume(fullVolumeConfig); err != nil {
		t.Fatal("Unable to add volume: ", err)
	}
	if err := orchestrator.DeleteVolume(fullVolumeName); err != nil {
		t.Fatal("Unable to remove full volume:  ", err)
	}

	txOnlyVolumeConfig := generateVolumeConfig(txOnlyVolumeName, 50, scName, config.File)
	if _, err := orchestrator.AddVolume(txOnlyVolumeConfig); err != nil {
		t.Fatal("Unable to add tx only volume: ", err)
	}

	// BEGIN actual test
	runRecoveryTests(t, orchestrator, backendName,
		storage.DeleteVolume, []recoveryTest{
			{name: "full", volumeConfig: fullVolumeConfig, expectDestroy: false},
			{name: "txOnly", volumeConfig: txOnlyVolumeConfig, expectDestroy: true},
		})
	cleanup(t, orchestrator)
}

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
		if err := orchestrator.storeClient.AddVolumeTransaction(volTxn); err != nil {
			t.Fatalf("%s: Unable to create volume transaction:  %v", c.name, err)
		}
		newOrchestrator := getOrchestrator()
		newOrchestrator.mutex.Lock()
		if _, ok := newOrchestrator.snapshots[c.snapshotConfig.ID()]; ok {
			t.Errorf("%s: snapshot still present in orchestrator.", c.name)
			// Note:  assume that if the snapshot's still present in the
			// top-level map, it's present everywhere else and that, if it's
			// absent there, it's absent everywhere else in memory
		}
		backend, err := newOrchestrator.getBackendByBackendName(backendName)
		if err != nil {
			t.Fatalf("%s: Backend not found after bootstrapping.", c.name)
		}
		f := backend.Driver.(*fakedriver.StorageDriver)

		_, ok := f.DestroyedSnapshots[c.snapshotConfig.ID()]
		if !ok && c.expectDestroy {
			t.Errorf("%s:  Destroy not called on snapshot.", c.name)
		} else if ok && !c.expectDestroy {
			t.Errorf("%s:  Destroy should not have been called on snapshot.", c.name)
		}

		_, err = newOrchestrator.storeClient.GetSnapshot(c.snapshotConfig.VolumeName, c.snapshotConfig.Name)
		if err != nil {
			if !persistentstore.MatchKeyNotFoundErr(err) {
				t.Errorf("%s: unable to communicate with backing store: %v", c.name, err)
			}
		} else {
			t.Errorf("%s: Found SnapshotConfig still stored in etcd.", c.name)
		}
		if txns, err := newOrchestrator.storeClient.GetVolumeTransactions(); err != nil {
			t.Errorf("%s: Unable to retrieve transactions from backing store: %v", c.name, err)
		} else if len(txns) > 0 {
			t.Errorf("%s:  Transaction not cleared from the backing store", c.name)
		}
		newOrchestrator.mutex.Unlock()
	}
}

func TestAddSnapshotRecovery(t *testing.T) {
	const (
		backendName        = "addSnapshotRecoveryBackend"
		scName             = "addSnapshotRecoveryBackendSC"
		volumeName         = "addSnapshotRecoveryVolume"
		fullSnapshotName   = "addSnapshotRecoverySnapshotFull"
		txOnlySnapshotName = "addSnapshotRecoverySnapshotTxOnly"
	)
	orchestrator := getOrchestrator()
	prepRecoveryTest(t, orchestrator, backendName, scName)

	// It's easier to add the volume/snapshot and then reinject the transaction again afterwards.
	volumeConfig := generateVolumeConfig(volumeName, 50, scName, config.File)
	if _, err := orchestrator.AddVolume(volumeConfig); err != nil {
		t.Fatal("Unable to add volume: ", err)
	}

	// For the full test, we create everything and recreate the AddSnapshot transaction.
	fullSnapshotConfig := generateSnapshotConfig(fullSnapshotName, volumeName, volumeName)
	if _, err := orchestrator.CreateSnapshot(fullSnapshotConfig); err != nil {
		t.Fatal("Unable to add snapshot: ", err)
	}

	// For the partial test, we add only the AddSnapshot transaction.
	txOnlySnapshotConfig := generateSnapshotConfig(txOnlySnapshotName, volumeName, volumeName)

	// BEGIN actual test.  Note that the delete idempotency is handled at the backend layer
	// (above the driver), so if the snapshot doesn't exist after bootstrapping, the driver
	// will not be called to delete the snapshot.
	runSnapshotRecoveryTests(t, orchestrator, backendName, storage.AddSnapshot,
		[]recoveryTest{
			{name: "full", volumeConfig: volumeConfig, snapshotConfig: fullSnapshotConfig, expectDestroy: true},
			{name: "txOnly", volumeConfig: volumeConfig, snapshotConfig: txOnlySnapshotConfig, expectDestroy: false},
		})
	cleanup(t, orchestrator)
}

func TestDeleteSnapshotRecovery(t *testing.T) {
	const (
		backendName        = "deleteSnapshotRecoveryBackend"
		scName             = "deleteSnapshotRecoveryBackendSC"
		volumeName         = "deleteSnapshotRecoveryVolume"
		fullSnapshotName   = "deleteSnapshotRecoverySnapshotFull"
		txOnlySnapshotName = "deleteSnapshotRecoverySnapshotTxOnly"
	)
	orchestrator := getOrchestrator()
	prepRecoveryTest(t, orchestrator, backendName, scName)

	// For the full test, we delete everything and recreate the delete transaction.
	volumeConfig := generateVolumeConfig(volumeName, 50, scName, config.File)
	if _, err := orchestrator.AddVolume(volumeConfig); err != nil {
		t.Fatal("Unable to add volume: ", err)
	}
	fullSnapshotConfig := generateSnapshotConfig(fullSnapshotName, volumeName, volumeName)
	if _, err := orchestrator.CreateSnapshot(fullSnapshotConfig); err != nil {
		t.Fatal("Unable to add snapshot: ", err)
	}
	if err := orchestrator.DeleteSnapshot(volumeName, fullSnapshotName); err != nil {
		t.Fatal("Unable to remove full snapshot: ", err)
	}

	// For the partial test, we ensure the snapshot will be restored during bootstrapping,
	// and the delete transaction will ensure everything is deleted.
	txOnlySnapshotConfig := generateSnapshotConfig(txOnlySnapshotName, volumeName, volumeName)
	if _, err := orchestrator.CreateSnapshot(txOnlySnapshotConfig); err != nil {
		t.Fatal("Unable to add snapshot: ", err)
	}

	// BEGIN actual test.  Note that the delete idempotency is handled at the backend layer
	// (above the driver), so if the snapshot doesn't exist after bootstrapping, the driver
	// will not be called to delete the snapshot.
	runSnapshotRecoveryTests(t, orchestrator, backendName, storage.DeleteSnapshot,
		[]recoveryTest{
			{name: "full", snapshotConfig: fullSnapshotConfig, expectDestroy: false},
			{name: "txOnly", snapshotConfig: txOnlySnapshotConfig, expectDestroy: true},
		})
	cleanup(t, orchestrator)
}

func TestBadBootstrapEtcdV2(t *testing.T) {
	if *etcdV2 == "" {
		t.SkipNow()
	}
	_, err := persistentstore.NewEtcdClientV2("invalidIPAddress")
	if err != nil && persistentstore.MatchUnavailableClusterErr(err) {
	} else {
		panic(fmt.Errorf("didn't catch invalid etcdv2 client"))
	}
}

func TestBadBootstrapEtcdV3(t *testing.T) {
	if *etcdV3 == "" {
		t.SkipNow()
	}
	_, err := persistentstore.NewEtcdClientV3("invalidIPAddress")
	if err != nil && persistentstore.MatchUnavailableClusterErr(err) {
	} else {
		panic(fmt.Errorf("didn't catch invalid etcdv3 client"))
	}
}

//func TestBootstrapEtcdV2ToEtcdV3Migration(t *testing.T) {
//	const (
//		backendName = "bootstrapV2toV3_backend"
//		scName      = "bootstrapV2toV3_class"
//		volumeName  = "bootstrapV2toV3_vol"
//	)
//	if *etcdV3 == "" {
//		t.SkipNow()
//	}
//	cleanupStoreVersion(t, *etcdV3)
//
//	// Populate the etcdv2 cluster with one backend, one storage class, and one volume
//	etcdV2Orig := etcdV2
//	etcdV2 = etcdV3
//	orchestratorV2 := getOrchestrator()
//	addBackendStorageClass(t, orchestratorV2, backendName, scName)
//
//	orchestratorV2.mutex.Lock()
//	if _, ok := orchestratorV2.storageClasses[scName]; !ok {
//		t.Fatal("Storage class not found in the orchestrator map!")
//	}
//	orchestratorV2.mutex.Unlock()
//
//	v2Volume, err := orchestratorV2.AddVolume(generateVolumeConfig(volumeName, 50, scName,
//		config.File))
//	if err != nil {
//		t.Fatal("Unable to create volume: ", err)
//	}
//	orchestratorV2.mutex.Lock()
//	if _, ok := orchestratorV2.volumes[volumeName]; !ok {
//		t.Fatal("Volume name found in orchestrator map!")
//	}
//	orchestratorV2.mutex.Unlock()
//
//	// Bootstrap etcdv3 orchestrator with etcdv2 data
//	etcdV2 = etcdV2Orig
//	orchestratorV3 := getOrchestrator()
//
//	// Verify etcdv2 to etcdv3 transformation
//	if v3Backend, err := orchestratorV3.GetBackend(backendName); v3Backend == nil {
//		t.Fatalf("Failed to find backend %s after bootstrapping! %v", backendName, err)
//	}
//	//TODO: Optionally we can diff etcdv2 and etcdv3 backends here
//	v3StorageClass, err := orchestratorV3.GetStorageClass(scName)
//	if v3StorageClass == nil || err != nil {
//		t.Fatalf("Failed to find storage class %s after bootstrapping!", scName)
//	}
//	//TODO: Optionally we can diff etcdv2 and etcdv3 storage classes here
//	v3Volume, err := orchestratorV3.GetVolume(volumeName)
//	if v3Volume == nil || err != nil {
//		t.Fatalf("Failed to find storage class %s after bootstrapping!", volumeName)
//	}
//	// Verifying the same volume exists on both clusters
//	if !reflect.DeepEqual(v2Volume, v3Volume) {
//		t.Fatalf("etcdv2 volume (%v) doesn't match the etcdv3 volume (%v)!",
//			v2Volume, v3Volume)
//	}
//	etcdv3Client, err := persistentstore.NewEtcdClientV3(*etcdV3)
//	if err != nil {
//		t.Fatalf("Creating etcdv3 client failed: %v", err)
//	}
//	versionJSON, err := etcdv3Client.Read(config.StoreURL)
//	if err != nil {
//		t.Fatalf("Couldn't determine the orchestrator persistent state version: %v",
//			err)
//	}
//	version := &config.PersistentStateVersion{}
//	err = json.Unmarshal([]byte(versionJSON), version)
//	if err != nil {
//		t.Fatalf("Couldn't unmarshal the orchestrator persistent state version: %v", err)
//	}
//	if config.OrchestratorAPIVersion != version.OrchestratorAPIVersion ||
//		string(orchestratorV3.storeClient.GetType()) != version.PersistentStoreVersion {
//		t.Fatalf("Failed to set the orchestrator persistent state version after bootsrapping: %v",
//			err)
//	}
//
//	// cleanup
//	if err = etcdv3Client.Stop(); err != nil {
//		t.Fatalf("Couldn't shut down etcdv3 client: %v", err)
//	}
//	cleanup(t, orchestratorV2)
//	cleanup(t, orchestratorV3)
//	cleanupStoreVersion(t, *etcdV3)
//}

// The next series of tests test that bootstrap doesn't exit early if it
// encounters a key error for one of the main types of entries.
func TestStorageClassOnlyBootstrap(t *testing.T) {
	const scName = "storageclass-only"

	orchestrator := getOrchestrator()
	originalSC, err := orchestrator.AddStorageClass(
		&storageclass.Config{
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
	bootstrappedSC, err := newOrchestrator.GetStorageClass(scName)
	if bootstrappedSC == nil || err != nil {
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
		txOnlyVolumeName = "firstRecoveryVolumeTxOnly"
	)
	orchestrator := getOrchestrator()
	prepRecoveryTest(t, orchestrator, backendName, scName)
	txOnlyVolumeConfig := generateVolumeConfig(txOnlyVolumeName, 50, scName,
		config.File)
	// BEGIN actual test
	runRecoveryTests(t, orchestrator, backendName, storage.AddVolume,
		[]recoveryTest{
			{name: "firstTXOnly", volumeConfig: txOnlyVolumeConfig,
				expectDestroy: true},
		})
	cleanup(t, orchestrator)
}

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

	orchestrator := getOrchestrator()
	orchestrator.bootstrapped = false
	orchestrator.bootstrapError = notReadyError()

	backend, err = orchestrator.AddBackend("")
	if backend != nil || !IsNotReadyError(err) {
		t.Errorf("Expected AddBackend to return an error.")
	}

	backend, err = orchestrator.GetBackend("")
	if backend != nil || !IsNotReadyError(err) {
		t.Errorf("Expected GetBackend to return an error.")
	}

	backends, err = orchestrator.ListBackends()
	if backends != nil || !IsNotReadyError(err) {
		t.Errorf("Expected ListBackends to return an error.")
	}

	err = orchestrator.DeleteBackend("")
	if !IsNotReadyError(err) {
		t.Errorf("Expected DeleteBackend to return an error.")
	}

	volume, err = orchestrator.AddVolume(nil)
	if volume != nil || !IsNotReadyError(err) {
		t.Errorf("Expected AddVolume to return an error.")
	}

	volume, err = orchestrator.CloneVolume(nil)
	if volume != nil || !IsNotReadyError(err) {
		t.Errorf("Expected CloneVolume to return an error.")
	}

	volume, err = orchestrator.GetVolume("")
	if volume != nil || !IsNotReadyError(err) {
		t.Errorf("Expected GetVolume to return an error.")
	}

	_, err = orchestrator.GetDriverTypeForVolume(nil)
	if !IsNotReadyError(err) {
		t.Errorf("Expected GetDriverTypeForVolume to return an error.")
	}

	_, err = orchestrator.GetVolumeType(nil)
	if !IsNotReadyError(err) {
		t.Errorf("Expected GetVolumeType to return an error.")
	}

	volumes, err = orchestrator.ListVolumes()
	if volumes != nil || !IsNotReadyError(err) {
		t.Errorf("Expected ListVolumes to return an error.")
	}

	err = orchestrator.DeleteVolume("")
	if !IsNotReadyError(err) {
		t.Errorf("Expected DeleteVolume to return an error.")
	}

	volumes, err = orchestrator.ListVolumesByPlugin("")
	if volumes != nil || !IsNotReadyError(err) {
		t.Errorf("Expected ListVolumesByPlugin to return an error.")
	}

	err = orchestrator.AttachVolume("", "", nil)
	if !IsNotReadyError(err) {
		t.Errorf("Expected AttachVolume to return an error.")
	}

	err = orchestrator.DetachVolume("", "")
	if !IsNotReadyError(err) {
		t.Errorf("Expected DetachVolume to return an error.")
	}

	snapshot, err = orchestrator.CreateSnapshot(nil)
	if snapshot != nil || !IsNotReadyError(err) {
		t.Errorf("Expected CreateSnapshot to return an error.")
	}

	snapshot, err = orchestrator.GetSnapshot("", "")
	if snapshot != nil || !IsNotReadyError(err) {
		t.Errorf("Expected GetSnapshot to return an error.")
	}

	snapshots, err = orchestrator.ListSnapshots()
	if snapshots != nil || !IsNotReadyError(err) {
		t.Errorf("Expected ListSnapshots to return an error.")
	}

	snapshots, err = orchestrator.ReadSnapshotsForVolume("")
	if snapshots != nil || !IsNotReadyError(err) {
		t.Errorf("Expected ReadSnapshotsForVolume to return an error.")
	}

	err = orchestrator.DeleteSnapshot("", "")
	if !IsNotReadyError(err) {
		t.Errorf("Expected DeleteSnapshot to return an error.")
	}

	err = orchestrator.ReloadVolumes()
	if !IsNotReadyError(err) {
		t.Errorf("Expected ReloadVolumes to return an error.")
	}

	storageClass, err = orchestrator.AddStorageClass(nil)
	if storageClass != nil || !IsNotReadyError(err) {
		t.Errorf("Expected AddStorageClass to return an error.")
	}

	storageClass, err = orchestrator.GetStorageClass("")
	if storageClass != nil || !IsNotReadyError(err) {
		t.Errorf("Expected GetStorageClass to return an error.")
	}

	storageClasses, err = orchestrator.ListStorageClasses()
	if storageClasses != nil || !IsNotReadyError(err) {
		t.Errorf("Expected ListStorageClasses to return an error.")
	}

	err = orchestrator.DeleteStorageClass("")
	if !IsNotReadyError(err) {
		t.Errorf("Expected DeleteStorageClass to return an error.")
	}
}

func importVolumeSetup(t *testing.T, backendName string, scName string, volumeName string, importOriginalName string,
	backendProtocol config.Protocol) (*TridentOrchestrator, *storage.VolumeConfig) {
	// Object setup
	orchestrator := getOrchestrator()
	addBackendStorageClass(t, orchestrator, backendName, scName, backendProtocol)

	orchestrator.mutex.Lock()
	_, ok := orchestrator.storageClasses[scName]
	if !ok {
		t.Fatal("Storageclass not found in orchestrator map")
	}
	orchestrator.mutex.Unlock()

	backendUUID := ""
	for _, b := range orchestrator.backends {
		if b.Name == backendName {
			backendUUID = b.BackendUUID
			break
		}
	}

	if backendUUID == "" {
		t.Fatal("BackendUUID not found")
	}

	volumeConfig := generateVolumeConfig(volumeName, 50, scName, backendProtocol)
	volumeConfig.ImportOriginalName = importOriginalName
	volumeConfig.ImportBackendUUID = backendUUID
	return orchestrator, volumeConfig
}

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

	_, err := orchestrator.LegacyImportVolume(volumeConfig, backendName, false, createPVandPVCError)

	// verify that importVolumeCleanup renamed volume to originalName
	backend, _ := orchestrator.getBackendByBackendName(backendName)
	volExternal, err := backend.Driver.GetVolumeExternal(originalName01)
	if err != nil {
		t.Errorf("falied to get volumeExternal for %s", originalName01)
	}
	if volExternal.Config.Size != "1000000000" {
		t.Errorf("falied to verify %s size %s", originalName01, volExternal.Config.Size)
	}

	// verify that we cleaned up the persisted state
	if _, ok := orchestrator.volumes[volumeConfig.Name]; ok {
		t.Errorf("volume %s should not exist in orchestrator's volume cache", volumeConfig.Name)
	}
	persistedVolume, err := orchestrator.storeClient.GetVolume(volumeConfig.Name)
	if persistedVolume != nil {
		t.Errorf("volume %s should not be persisted", volumeConfig.Name)
	}

	cleanup(t, orchestrator)
}

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
		{name: "managed", volumeConfig: volumeConfig, notManaged: false,
			createFunc: createPVandPVCNoOp, expectedInternalName: volumeConfig.InternalName},
		{name: "notManaged", volumeConfig: notManagedVolConfig, notManaged: true,
			createFunc: createPVandPVCNoOp, expectedInternalName: originalName02},
	} {
		// The test code
		volExternal, err := orchestrator.LegacyImportVolume(c.volumeConfig, backendName, c.notManaged, c.createFunc)
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		} else {
			if volExternal.Config.InternalName != c.expectedInternalName {
				t.Errorf("%s: expected matching internal names %s - %s",
					c.name, c.expectedInternalName, volExternal.Config.InternalName)
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

	orchestrator, volumeConfig := importVolumeSetup(t, backendName, scName, volumeName01, originalName01, backendProtocol)

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
		{name: "managed", volumeConfig: volumeConfig, expectedInternalName: volumeName01},
		{name: "notManaged", volumeConfig: notManagedVolConfig, expectedInternalName: originalName02},
	} {
		// The test code
		volExternal, err := orchestrator.ImportVolume(c.volumeConfig)
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		} else {
			if volExternal.Config.InternalName != c.expectedInternalName {
				t.Errorf("%s: expected matching internal names %s - %s",
					c.name, c.expectedInternalName, volExternal.Config.InternalName)
			}
			if _, ok := orchestrator.volumes[volExternal.Config.Name]; !ok {
				t.Errorf("%s: managed volume %s should be persisted", c.name, volExternal.Config.Name)
			}
		}

	}
	cleanup(t, orchestrator)
}

func TestValidateImportVolumeNasBackend(t *testing.T) {
	const (
		backendName     = "backend01"
		scName          = "sc01"
		volumeName      = "volume01"
		originalName    = "origVolume01"
		backendProtocol = config.File
	)

	orchestrator, volumeConfig := importVolumeSetup(t, backendName, scName, volumeName, originalName, backendProtocol)

	_, err := orchestrator.AddVolume(volumeConfig)
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
		{name: "accessMode", volumeConfig: accessModeVolConfig, valid: false, error: "incompatible volume mode (), access mode"},
		{name: "volumeMode", volumeConfig: volumeModeVolConfig, valid: false, error: "incompatible volume mode "},
		{name: "protocol", volumeConfig: protocolVolConfig, valid: false, error: "incompatible with the backend"},
	} {
		// The test code
		err = orchestrator.validateImportVolume(c.volumeConfig)
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

func TestValidateImportVolumeSanBackend(t *testing.T) {
	const (
		backendName     = "backend01"
		scName          = "sc01"
		volumeName      = "volume01"
		originalName    = "origVolume01"
		backendProtocol = config.Block
	)

	orchestrator, volumeConfig := importVolumeSetup(t, backendName, scName, volumeName, originalName, backendProtocol)

	_, err := orchestrator.AddVolume(volumeConfig)
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
		{name: "invalidFS", volumeConfig: ext4RawBlockFSVolConfig, valid: false, error: "cannot create raw-block volume"},
	} {
		// The test code
		err = orchestrator.validateImportVolume(c.volumeConfig)
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

func TestAddNode(t *testing.T) {
	node := &utils.Node{
		Name: "testNode",
		IQN:  "myIQN",
		IPs:  []string{"1.1.1.1", "2.2.2.2"},
	}
	orchestrator := getOrchestrator()
	if err := orchestrator.AddNode(node); err != nil {
		t.Errorf("adding node failed; %v", err)
	}
}

func TestGetNode(t *testing.T) {
	orchestrator := getOrchestrator()
	expectedNode := &utils.Node{
		Name: "testNode",
		IQN:  "myIQN",
		IPs:  []string{"1.1.1.1", "2.2.2.2"},
	}
	unexpectedNode := &utils.Node{
		Name: "testNode2",
		IQN:  "myOtherIQN",
		IPs:  []string{"3.3.3.3", "4.4.4.4"},
	}
	initialNodes := map[string]*utils.Node{}
	initialNodes[expectedNode.Name] = expectedNode
	initialNodes[unexpectedNode.Name] = unexpectedNode
	orchestrator.nodes = initialNodes

	actualNode, err := orchestrator.GetNode(expectedNode.Name)
	if err != nil {
		t.Errorf("error getting node; %v", err)
	}

	if actualNode != expectedNode {
		t.Errorf("Did not get expected node back; expected %+v, got %+v", expectedNode, actualNode)
	}
}

func TestListNodes(t *testing.T) {
	orchestrator := getOrchestrator()
	expectedNode1 := &utils.Node{
		Name: "testNode",
		IQN:  "myIQN",
		IPs:  []string{"1.1.1.1", "2.2.2.2"},
	}
	expectedNode2 := &utils.Node{
		Name: "testNode2",
		IQN:  "myOtherIQN",
		IPs:  []string{"3.3.3.3", "4.4.4.4"},
	}
	initialNodes := map[string]*utils.Node{}
	initialNodes[expectedNode1.Name] = expectedNode1
	initialNodes[expectedNode2.Name] = expectedNode2
	orchestrator.nodes = initialNodes
	expectedNodes := []*utils.Node{expectedNode1, expectedNode2}

	actualNodes, err := orchestrator.ListNodes()
	if err != nil {
		t.Errorf("error listing nodes; %v", err)
	}

	if !unorderedNodeSlicesEqual(actualNodes, expectedNodes) {
		t.Errorf("node list values do not match; expected %v, found %v", expectedNodes, actualNodes)
	}
}

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
	if len(diff) == 0 {
		return true
	}
	return false
}

func TestDeleteNode(t *testing.T) {
	orchestrator := getOrchestrator()
	initialNode := &utils.Node{
		Name: "testNode",
		IQN:  "myIQN",
		IPs:  []string{"1.1.1.1", "2.2.2.2"},
	}
	initialNodes := map[string]*utils.Node{}
	initialNodes[initialNode.Name] = initialNode
	orchestrator.nodes = initialNodes

	if err := orchestrator.DeleteNode(initialNode.Name); err != nil {
		t.Errorf("error deleting node; %v", err)
	}

	if _, ok := orchestrator.nodes[initialNode.Name]; ok {
		t.Errorf("node was not properly deleted")
	}
}

func TestSnapshotVolumes(t *testing.T) {
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
			t.Fatalf("Unable to generate cfg JSON for %s:  %v", c.name, err)
		}
		_, err = orchestrator.AddBackend(cfg)
		if err != nil {
			t.Errorf("Unable to add backend %s:  %v", c.name, err)
			errored = true
		}
		orchestrator.mutex.Lock()
		backend, err := orchestrator.getBackendByBackendName(c.name)
		if err != nil {
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
		_, err := orchestrator.AddStorageClass(s.config)
		if err != nil {
			t.Errorf("Unable to add storage class %s:  %v", s.config.Name, err)
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
			config:          generateVolumeConfig("file", 1, "fast", config.File),
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
			config:          generateVolumeConfig("block", 1, "slow", config.Block),
			expectedSuccess: true,
			expectedMatches: []*tu.PoolMatch{
				{Backend: "slow-block", Pool: tu.SlowSnapshots},
				{Backend: "slow-block", Pool: tu.SlowSnapshots},
			},
		},
	} {
		// Create the source volume
		_, err := orchestrator.AddVolume(s.config)
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
		snapshotExternal, err := orchestrator.CreateSnapshot(snapshotConfig)
		if err != nil {
			t.Fatalf("%s: got unexpected error creating snapshot: %v", s.name, err)
		}

		orchestrator.mutex.Lock()
		// Snapshot should be registered in the store
		persistentSnapshot, err := orchestrator.storeClient.GetSnapshot(volume.Config.Name, snapshotName)
		if err != nil {
			t.Errorf("%s: unable to communicate with backing store: %v", snapshotName, err)
		}
		persistentSnapshotExternal := persistentSnapshot.ConstructExternal()
		if !reflect.DeepEqual(persistentSnapshotExternal, snapshotExternal) {
			t.Errorf("%s: external snapshot %s stored in backend does not match created snapshot.",
				snapshotName, persistentSnapshot.Config.Name)
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

		err = orchestrator.DeleteSnapshot(volume.Config.Name, snapshotName)
		if err != nil {
			t.Fatalf("%s: got unexpected error deleting snapshot: %v", s.name, err)
		}

		orchestrator.mutex.Lock()
		// Snapshot should not be registered in the store
		persistentSnapshot, err = orchestrator.storeClient.GetSnapshot(volume.Config.Name, snapshotName)
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

func TestGetProtocol(t *testing.T) {
	orchestrator := getOrchestrator()

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
		{config.Filesystem, config.ReadWriteOnce, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ReadWriteOnce, config.File, config.File},
		{config.Filesystem, config.ReadWriteOnce, config.Block, config.Block},
		{config.Filesystem, config.ReadOnlyMany, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ReadOnlyMany, config.File, config.File},
		{config.Filesystem, config.ReadOnlyMany, config.Block, config.Block},
		{config.Filesystem, config.ReadWriteMany, config.ProtocolAny, config.File},
		{config.Filesystem, config.ReadWriteMany, config.File, config.File},
		//{config.Filesystem, config.ReadWriteMany, config.Block, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.ProtocolAny, config.Block},
		//{config.RawBlock, config.ModeAny, config.File, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.Block, config.Block},
		{config.RawBlock, config.ReadWriteOnce, config.ProtocolAny, config.Block},
		//{config.RawBlock, config.ReadWriteOnce, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteOnce, config.Block, config.Block},
		{config.RawBlock, config.ReadOnlyMany, config.ProtocolAny, config.Block},
		//{config.RawBlock, config.ReadOnlyMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadOnlyMany, config.Block, config.Block},
		{config.RawBlock, config.ReadWriteMany, config.ProtocolAny, config.Block},
		//{config.RawBlock, config.ReadWriteMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteMany, config.Block, config.Block},
	}

	var accessModesNegativeTests = []accessVariables{
		{config.Filesystem, config.ReadWriteMany, config.Block, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteOnce, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadOnlyMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteMany, config.File, config.ProtocolAny},
	}

	for _, tc := range accessModesPositiveTests {
		protocolLocal, err := orchestrator.getProtocol(tc.volumeMode, tc.accessMode, tc.protocol)
		testutils.AssertTrue(t, "error should be nil!", err == nil)
		testutils.AssertTrue(t, "expected both the protocols to be equal!", tc.expected == protocolLocal)
	}

	for _, tc := range accessModesNegativeTests {
		protocolLocal, err := orchestrator.getProtocol(tc.volumeMode, tc.accessMode, tc.protocol)
		testutils.AssertTrue(t, "error should not be nil!", err != nil)
		testutils.AssertTrue(t, "expected both the protocols to be equal!", tc.expected == protocolLocal)
	}
}
