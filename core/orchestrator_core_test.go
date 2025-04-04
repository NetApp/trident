// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/RoaringBitmap/roaring/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/logging"
	mockpersistentstore "github.com/netapp/trident/mocks/mock_persistent_store"
	mockstorage "github.com/netapp/trident/mocks/mock_storage"
	persistentstore "github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	fakedriver "github.com/netapp/trident/storage_drivers/fake"
	tu "github.com/netapp/trident/storage_drivers/fake/test_utils"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	logging.InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

var (
	inMemoryClient *persistentstore.InMemoryClient
	ctx            = context.Background
	coreCtx        = context.WithValue(ctx(), logging.ContextKeyLogLayer, logging.LogLayerCore)
)

func init() {
	testing.Init()
	inMemoryClient = persistentstore.NewInMemoryClient()
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

func getOrchestrator(t *testing.T, monitorTransactions bool) *TridentOrchestrator {
	var (
		storeClient persistentstore.Client
		err         error
	)
	// This will have been created as not nil in init
	// We can't create a new one here because tests that exercise
	// bootstrapping need to have their data persist.
	storeClient = inMemoryClient

	o, err := NewTridentOrchestrator(storeClient)
	if err != nil {
		t.Fatal("Unable to create orchestrator: ", err)
	}

	if err = o.Bootstrap(monitorTransactions); err != nil {
		t.Fatal("Failure occurred during bootstrapping: ", err)
	}

	// Set this to true. Tests to fail can set this to false to cause not ready errors.
	o.volumePublicationsSynced = true
	return o
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
		t.Errorf("%s: Storage class not found in backend.", name)
	}
	remaining := make([]*tu.PoolMatch, len(expected))
	copy(remaining, expected)
	for _, protocol := range []config.Protocol{config.File, config.Block} {
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
func TestAddStorageClassVolumes(t *testing.T) {
	mockPools := tu.GetFakePools()
	orchestrator := getOrchestrator(t, false)

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
			t.Fatalf("Unable to generate config JSON for %s: %v", c.name, err)
		}
		_, err = orchestrator.AddBackend(ctx(), fakeConfig, "")
		if err != nil {
			t.Errorf("Unable to add backend %s: %v", c.name, err)
			errored = true
		}
		orchestrator.mutex.Lock()
		backend, err := orchestrator.getBackendByBackendName(c.name)
		if err != nil {
			t.Fatalf("Backend %s not stored in orchestrator, err %s", c.name, err)
		}
		persistentBackend, err := orchestrator.storeClient.GetBackend(ctx(), c.name)
		if err != nil {
			t.Fatalf("Unable to get backend %s from persistent store: %v", c.name, err)
		} else if !reflect.DeepEqual(backend.ConstructPersistent(ctx()), persistentBackend) {
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
		_, err := orchestrator.AddStorageClass(ctx(), s.config)
		if err != nil {
			t.Errorf("Unable to add storage class %s: %v", s.config.Name, err)
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
			name:            "basic",
			config:          tu.GenerateVolumeConfig("basic", 1, "fast", config.File),
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
			name:            "large",
			config:          tu.GenerateVolumeConfig("large", 100, "fast", config.File),
			expectedSuccess: false,
			expectedMatches: []*tu.PoolMatch{},
			expectedCount:   0,
			deleteAfterSC:   false,
		},
		{
			name:            "block",
			config:          tu.GenerateVolumeConfig("block", 1, "pools", config.Block),
			expectedSuccess: true,
			expectedMatches: []*tu.PoolMatch{
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-block", Pool: tu.MediumOverlap},
			},
			expectedCount: 1,
			deleteAfterSC: false,
		},
		{
			name:            "block2",
			config:          tu.GenerateVolumeConfig("block2", 1, "additionalPools", config.Block),
			expectedSuccess: true,
			expectedMatches: []*tu.PoolMatch{
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-block", Pool: tu.MediumOverlap},
			},
			expectedCount: 1,
			deleteAfterSC: false,
		},
		{
			name:            "invalid-storage-class",
			config:          tu.GenerateVolumeConfig("invalid", 1, "nonexistent", config.File),
			expectedSuccess: false,
			expectedMatches: []*tu.PoolMatch{},
			expectedCount:   0,
			deleteAfterSC:   false,
		},
		{
			name:            "repeated",
			config:          tu.GenerateVolumeConfig("basic", 20, "fast", config.File),
			expectedSuccess: false,
			expectedMatches: []*tu.PoolMatch{},
			expectedCount:   1,
			deleteAfterSC:   false,
		},
		{
			name:            "postSCDelete",
			config:          tu.GenerateVolumeConfig("postSCDelete", 20, "fast", config.File),
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
		vol, err := orchestrator.AddVolume(ctx(), s.config)
		if err != nil && s.expectedSuccess {
			t.Errorf("%s: got unexpected error %v", s.name, err)
			continue
		} else if err == nil && !s.expectedSuccess {
			t.Errorf("%s: volume create succeeded unexpectedly.", s.name)
			continue
		}
		orchestrator.mutex.Lock()
		volume, found := orchestrator.volumes[s.config.Name]
		if s.expectedCount == 1 && !found {
			t.Errorf("%s: did not get volume where expected.", s.name)
		} else if s.expectedCount == 0 && found {
			t.Errorf("%s: got a volume where none expected.", s.name)
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
			if potentialMatch.Backend == volumeBackend.Name() &&
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
			t.Errorf(
				"%s: Volume placed on unexpected backend and storage pool: %s, %s",
				s.name,
				volume.BackendUUID,
				volume.Pool,
			)
		}

		externalVolume, err := orchestrator.storeClient.GetVolume(ctx(), s.config.Name)
		if err != nil {
			t.Errorf("%s: unable to communicate with backing store: %v", s.name, err)
		}
		if !reflect.DeepEqual(externalVolume, vol) {
			t.Errorf("%s: external volume %s stored in backend does not match created volume.", s.name,
				externalVolume.Config.Name)
			externalVolJSON, err := json.Marshal(externalVolume)
			if err != nil {
				t.Fatal("Unable to remarshal JSON: ", err)
			}
			origVolJSON, err := json.Marshal(vol)
			if err != nil {
				t.Fatal("Unable to remarshal JSON: ", err)
			}
			t.Logf("\tExpected: %s\n\tGot: %s\n", string(externalVolJSON), string(origVolJSON))
		}
		orchestrator.mutex.Unlock()
	}
	for _, d := range preSCDeleteTests {
		runDeleteTest(t, d, orchestrator)
	}

	// Delete storage classes.  Note: there are currently no error cases.
	for _, s := range scTests {
		err := orchestrator.DeleteStorageClass(ctx(), s.config.Name)
		if err != nil {
			t.Errorf("%s delete: Unable to remove storage class: %v", s.config.Name, err)
		}
		orchestrator.mutex.Lock()
		if _, ok := orchestrator.storageClasses[s.config.Name]; ok {
			t.Errorf("%s delete: Storage class still found in map.", s.config.Name)
		}
		// Ensure that the storage class was cleared from its backends.
		for _, poolMatch := range s.expected {
			b, err := orchestrator.getBackendByBackendName(poolMatch.Backend)
			if b == nil || err != nil {
				t.Errorf("%s delete: backend %s not found in orchestrator.", s.config.Name, poolMatch.Backend)
				continue
			}
			p, ok := b.Storage()[poolMatch.Pool]
			if !ok {
				t.Errorf("%s delete: storage pool %s not found for backend %s", s.config.Name, poolMatch.Pool,
					poolMatch.Backend)
				continue
			}
			for _, sc := range p.StorageClasses() {
				if sc == s.config.Name {
					t.Errorf("%s delete: storage class name not removed from backend %s, storage pool %s",
						s.config.Name, poolMatch.Backend, poolMatch.Pool)
				}
			}
		}
		externalSC, err := orchestrator.storeClient.GetStorageClass(ctx(), s.config.Name)
		if err != nil {
			if !persistentstore.MatchKeyNotFoundErr(err) {
				t.Errorf("%s: unable to communicate with backing store: %v", s.config.Name, err)
			}
			// We're successful if we get to here; we expect an
			// ErrorCodeKeyNotFound.
		} else if externalSC != nil {
			t.Errorf("%s: storageClass not properly deleted from backing store", s.config.Name)
		}
		orchestrator.mutex.Unlock()
	}
	for _, d := range postSCDeleteTests {
		runDeleteTest(t, d, orchestrator)
	}
	cleanup(t, orchestrator)
}

func TestUpdateVolume_SnapshotDir_Success(t *testing.T) {
	ctx := context.Background()
	backendUUID := "45e44b30-8f53-498d-8555-2cf006760ba6"

	volName := "test"
	internalId := "/svm/fakesvm/flexvol/fakevol/qtree/" + volName
	vol := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name:        volName,
			InternalID:  internalId,
			SnapshotDir: "false",
		},
		BackendUUID: backendUUID,
	}

	updateInfo := &models.VolumeUpdateInfo{
		SnapshotDirectory: "true",
		PoolLevel:         true,
	}

	updatedVol := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name:        volName,
			InternalID:  internalId,
			SnapshotDir: "true",
		},
		BackendUUID: backendUUID,
	}

	orchestrator := getOrchestrator(t, false)
	orchestrator.volumes[volName] = vol

	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	orchestrator.backends[backendUUID] = mockBackend
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
	mockBackend.EXPECT().GetDriverName().Return("ontap-nas-economy").AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().Name().Return("mockBackend").AnyTimes()
	mockBackend.EXPECT().UpdateVolume(gomock.Any(), vol.Config, updateInfo).Return(map[string]*storage.Volume{volName: updatedVol}, nil)

	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	orchestrator.storeClient = mockStoreClient
	mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), updatedVol).Return(nil)

	// Update volume
	result := orchestrator.UpdateVolume(ctx, volName, updateInfo)

	assert.NoError(t, result)
	assert.NotNil(t, orchestrator.volumes[volName])
	assert.Equal(t, updatedVol.Config.SnapshotDir, orchestrator.volumes[volName].Config.SnapshotDir)
}

func TestUpdateVolume_SnapshotDir_Failure(t *testing.T) {
	ctx := context.Background()
	backendUUID := "45e44b30-8f53-498d-8555-2cf006760ba6"
	fakeErr := errors.New("fake error")
	volName := "test"
	internalId := "/svm/fakesvm/flexvol/fakevol/qtree/" + volName
	vol := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name:        volName,
			InternalID:  internalId,
			SnapshotDir: "false",
		},
		BackendUUID: backendUUID,
	}

	updateInfo := &models.VolumeUpdateInfo{
		SnapshotDirectory: "true",
		PoolLevel:         true,
	}

	updatedVol := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name:        volName,
			InternalID:  internalId,
			SnapshotDir: "true",
		},
		BackendUUID: backendUUID,
	}

	orchestrator := getOrchestrator(t, false)
	orchestrator.volumes[vol.Config.Name] = vol

	// CASE 1: No volume update information provided
	result := orchestrator.UpdateVolume(ctx, volName, nil)

	assert.Error(t, result)
	assert.Equal(t, true, errors.IsInvalidInputError(result))

	// CASE 2: Volume not found
	result = orchestrator.UpdateVolume(ctx, "not-found-vol", updateInfo)

	assert.Error(t, result)
	assert.Equal(t, true, errors.IsNotFoundError(result))

	// CASE 3: Backend not found
	result = orchestrator.UpdateVolume(ctx, volName, updateInfo)

	assert.Error(t, result)
	assert.Equal(t, true, errors.IsNotFoundError(result))

	// CASE 4: Backend failed to update volume
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	orchestrator.backends[backendUUID] = mockBackend
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
	mockBackend.EXPECT().GetDriverName().Return("ontap-nas-economy").AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().Name().Return("mockBackend").AnyTimes()
	mockBackend.EXPECT().UpdateVolume(gomock.Any(), vol.Config, updateInfo).Return(nil, fakeErr)

	result = orchestrator.UpdateVolume(ctx, volName, updateInfo)

	assert.Error(t, result)
	assert.Equal(t, fakeErr.Error(), result.Error())

	// CASE 5: Persistence failed to update volume
	mockBackend.EXPECT().UpdateVolume(gomock.Any(), vol.Config, updateInfo).Return(map[string]*storage.Volume{volName: updatedVol}, nil)
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	orchestrator.storeClient = mockStoreClient
	mockStoreClient.EXPECT().UpdateVolume(gomock.Any(), updatedVol).Return(fakeErr)

	result = orchestrator.UpdateVolume(ctx, volName, updateInfo)

	assert.Error(t, result)
	assert.Equal(t, fakeErr.Error(), result.Error())
}

func TestUpdateVolumeLUKSPassphraseNames(t *testing.T) {
	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: luksPassphraseNames field updated
	orchestrator := getOrchestrator(t, false)
	vol := &storage.Volume{
		Config:      &storage.VolumeConfig{Name: "test-vol", LUKSPassphraseNames: []string{}},
		BackendUUID: "12345",
	}
	orchestrator.volumes[vol.Config.Name] = vol
	err := orchestrator.storeClient.AddVolume(context.TODO(), vol)
	assert.NoError(t, err)
	assert.Empty(t, orchestrator.volumes[vol.Config.Name].Config.LUKSPassphraseNames)

	err = orchestrator.UpdateVolumeLUKSPassphraseNames(context.TODO(), "test-vol", &[]string{"A"})
	desiredPassphraseNames := []string{"A"}
	assert.NoError(t, err)
	assert.Equal(t, desiredPassphraseNames, orchestrator.volumes[vol.Config.Name].Config.LUKSPassphraseNames)

	storedVol, err := orchestrator.storeClient.GetVolume(context.TODO(), "test-vol")
	assert.NoError(t, err)
	assert.Equal(t, desiredPassphraseNames, storedVol.Config.LUKSPassphraseNames)

	err = orchestrator.storeClient.DeleteVolume(context.TODO(), vol)
	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: luksPassphraseNames field nil
	orchestrator = getOrchestrator(t, false)
	vol = &storage.Volume{
		Config:      &storage.VolumeConfig{Name: "test-vol", LUKSPassphraseNames: []string{}},
		BackendUUID: "12345",
	}
	orchestrator.volumes[vol.Config.Name] = vol
	err = orchestrator.storeClient.AddVolume(context.TODO(), vol)
	assert.NoError(t, err)
	assert.Empty(t, orchestrator.volumes[vol.Config.Name].Config.LUKSPassphraseNames)

	err = orchestrator.UpdateVolumeLUKSPassphraseNames(context.TODO(), "test-vol", nil)
	desiredPassphraseNames = []string{}
	assert.NoError(t, err)
	assert.Equal(t, desiredPassphraseNames, orchestrator.volumes[vol.Config.Name].Config.LUKSPassphraseNames)

	storedVol, err = orchestrator.storeClient.GetVolume(context.TODO(), "test-vol")
	assert.NoError(t, err)
	assert.Equal(t, desiredPassphraseNames, storedVol.Config.LUKSPassphraseNames)

	err = orchestrator.storeClient.DeleteVolume(context.TODO(), vol)
	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: failed to update persistence, volume not found
	orchestrator = getOrchestrator(t, false)
	vol = &storage.Volume{
		Config:      &storage.VolumeConfig{Name: "test-vol", LUKSPassphraseNames: []string{}},
		BackendUUID: "12345",
	}
	orchestrator.volumes[vol.Config.Name] = vol

	err = orchestrator.UpdateVolumeLUKSPassphraseNames(context.TODO(), "test-vol", &[]string{"A"})
	desiredPassphraseNames = []string{}
	assert.Error(t, err)
	assert.Equal(t, desiredPassphraseNames, orchestrator.volumes[vol.Config.Name].Config.LUKSPassphraseNames)

	_, err = orchestrator.storeClient.GetVolume(context.TODO(), "test-vol")
	// Not found
	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: bootstrap error
	orchestrator = getOrchestrator(t, false)
	bootstrapError := fmt.Errorf("my bootstrap error")
	orchestrator.bootstrapError = bootstrapError

	err = orchestrator.UpdateVolumeLUKSPassphraseNames(context.TODO(), "test-vol", &[]string{"A"})
	assert.Error(t, err)
	assert.ErrorIs(t, err, bootstrapError)
}

func TestCloneVolume_SnapshotDataSource_LUKS(t *testing.T) {
	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: luksPassphraseNames field updated
	// // Setup
	mockPools := tu.GetFakePools()
	orchestrator := getOrchestrator(t, false)

	// Cloning a volume from a snapshot with LUKS is only supported with CSI.
	defer func(context config.DriverContext) {
		config.CurrentDriverContext = context
	}(config.CurrentDriverContext)
	config.CurrentDriverContext = config.ContextCSI

	// Make a backend
	poolNames := []string{tu.SlowSnapshots}
	pools := make(map[string]*fake.StoragePool, len(poolNames))
	for _, poolName := range poolNames {
		pools[poolName] = mockPools[poolName]
	}
	volumes := make([]fake.Volume, 0)
	cfg, err := fakedriver.NewFakeStorageDriverConfigJSON("slow-block", "block", pools, volumes)
	assert.NoError(t, err)
	_, err = orchestrator.AddBackend(ctx(), cfg, "")
	assert.NoError(t, err)
	defer orchestrator.DeleteBackend(ctx(), "slow-block")

	// Make a StorageClass
	storageClass := &storageclass.Config{Name: "specific"}
	_, err = orchestrator.AddStorageClass(ctx(), storageClass)
	defer orchestrator.DeleteStorageClass(ctx(), storageClass.Name)
	assert.NoError(t, err)

	// Create the original volume
	volConfig := tu.GenerateVolumeConfig("block", 1, "specific", config.Block)
	volConfig.LUKSEncryption = "true"
	volConfig.LUKSPassphraseNames = []string{"A", "B"}
	_, err = orchestrator.AddVolume(ctx(), volConfig)
	assert.NoError(t, err)
	defer orchestrator.DeleteVolume(ctx(), volConfig.Name)

	// Create a snapshot
	snapshotConfig := generateSnapshotConfig("test-snapshot", volConfig.Name, volConfig.InternalName)
	snapshot, err := orchestrator.CreateSnapshot(ctx(), snapshotConfig)
	assert.NoError(t, err)
	assert.Equal(t, snapshot.Config.LUKSPassphraseNames, []string{"A", "B"})
	defer orchestrator.DeleteSnapshot(ctx(), volConfig.Name, snapshotConfig.Name)

	// "rotate" the luksPassphraseNames of the volume
	err = orchestrator.UpdateVolumeLUKSPassphraseNames(ctx(), volConfig.Name, &[]string{"A"})
	assert.NoError(t, err)
	vol, err := orchestrator.GetVolume(ctx(), volConfig.Name)
	assert.NoError(t, err)
	volConfig = vol.Config
	assert.Equal(t, vol.Config.LUKSPassphraseNames, []string{"A"})

	// Now clone the snapshot and ensure everything looks fine
	cloneName := volConfig.Name + "_clone"
	cloneConfig := &storage.VolumeConfig{
		Name:                cloneName,
		StorageClass:        volConfig.StorageClass,
		CloneSourceVolume:   volConfig.Name,
		CloneSourceSnapshot: snapshotConfig.Name,
		VolumeMode:          volConfig.VolumeMode,
	}
	cloneResult, err := orchestrator.CloneVolume(ctx(), cloneConfig)
	assert.NoError(t, err)
	assert.Equal(t, []string{"A", "B"}, cloneResult.Config.LUKSPassphraseNames)
	defer orchestrator.DeleteVolume(ctx(), cloneResult.Config.Name)
}

func TestCloneVolume_VolumeDataSource_LUKS(t *testing.T) {
	// // Setup
	mockPools := tu.GetFakePools()
	orchestrator := getOrchestrator(t, false)

	// Create backend
	poolNames := []string{tu.SlowSnapshots}
	pools := make(map[string]*fake.StoragePool, len(poolNames))
	for _, poolName := range poolNames {
		pools[poolName] = mockPools[poolName]
	}
	volumes := make([]fake.Volume, 0)
	cfg, err := fakedriver.NewFakeStorageDriverConfigJSON("slow-block", "block", pools, volumes)
	assert.NoError(t, err)
	_, err = orchestrator.AddBackend(ctx(), cfg, "")
	assert.NoError(t, err)
	defer orchestrator.DeleteBackend(ctx(), "slow-block")

	// Create a StorageClass
	storageClass := &storageclass.Config{Name: "specific"}
	_, err = orchestrator.AddStorageClass(ctx(), storageClass)
	defer orchestrator.DeleteStorageClass(ctx(), storageClass.Name)
	assert.NoError(t, err)

	// Create the Volume
	volConfig := tu.GenerateVolumeConfig("block", 1, "specific", config.Block)
	volConfig.LUKSEncryption = "true"
	volConfig.LUKSPassphraseNames = []string{"A", "B"}
	// Create the source volume
	_, err = orchestrator.AddVolume(ctx(), volConfig)
	assert.NoError(t, err)
	defer orchestrator.DeleteVolume(ctx(), volConfig.Name)

	// Create a snapshot
	snapshotConfig := generateSnapshotConfig("test-snapshot", volConfig.Name, volConfig.InternalName)
	snapshot, err := orchestrator.CreateSnapshot(ctx(), snapshotConfig)
	assert.NoError(t, err)
	assert.Equal(t, []string{"A", "B"}, snapshot.Config.LUKSPassphraseNames)
	defer orchestrator.DeleteSnapshot(ctx(), volConfig.Name, snapshotConfig.Name)

	// Now clone the volume and ensure everything looks fine
	cloneName := volConfig.Name + "_clone"
	cloneConfig := &storage.VolumeConfig{
		Name:              cloneName,
		StorageClass:      volConfig.StorageClass,
		CloneSourceVolume: volConfig.Name,
		VolumeMode:        volConfig.VolumeMode,
	}
	cloneResult, err := orchestrator.CloneVolume(ctx(), cloneConfig)
	assert.NoError(t, err)
	assert.Equal(t, []string{"A", "B"}, cloneResult.Config.LUKSPassphraseNames)
	defer orchestrator.DeleteVolume(ctx(), cloneResult.Config.Name)
}

func TestCloneVolume_WithImportNotManaged(t *testing.T) {
	orchestrator := getOrchestrator(t, false)
	defer cleanup(t, orchestrator)

	cfg, err := fakedriver.NewFakeStorageDriverConfigJSON("storageDriver", "block", tu.GenerateFakePools(1), nil)
	if !assert.NoError(t, err) {
		return
	}

	_, err = orchestrator.AddBackend(ctx(), cfg, "")
	if !assert.NoError(t, err) {
		return
	}

	storageClass := &storageclass.Config{Name: "sc"}
	_, err = orchestrator.AddStorageClass(ctx(), storageClass)
	if !assert.NoError(t, err) {
		return
	}

	volConfig := tu.GenerateVolumeConfig("sourceVolume", 1, "sc", config.Block)
	volConfig.ImportNotManaged = true
	_, err = orchestrator.AddVolume(ctx(), volConfig)
	if !assert.NoError(t, err) {
		return
	}

	testCases := []struct {
		name string
		err  error
	}{
		{"Success", nil},
		{"Fail", fmt.Errorf("cannot clone an unmanaged volume without a snapshot")},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			cloneConfig := &storage.VolumeConfig{
				Name:              "clonedVolume",
				StorageClass:      volConfig.StorageClass,
				CloneSourceVolume: volConfig.Name,
				VolumeMode:        volConfig.VolumeMode,
			}

			if tt.name == "Fail" {
				config.CurrentDriverContext = config.ContextDocker
				defer func() { config.CurrentDriverContext = "" }()
			}

			if tt.name == "Success" {
				config.CurrentDriverContext = config.ContextCSI
				defer func() { config.CurrentDriverContext = "" }()

				// If the source is a snapshot, inject a snapshot into the core.
				cloneConfig.CloneSourceSnapshot = "sourceSnap"
				snapshot := &storage.Snapshot{
					Config: generateSnapshotConfig(
						cloneConfig.CloneSourceSnapshot, volConfig.Name, volConfig.InternalName,
					),
				}
				snapshot.Config.InternalName = "sourceSnap"
				orchestrator.snapshots[snapshot.ID()] = snapshot
			}

			cloneResult, err := orchestrator.CloneVolume(ctx(), cloneConfig)
			assert.Equal(t, tt.err, err)

			if tt.name == "Success" && assert.NotNil(t, cloneResult) && assert.NotNil(t, cloneResult.Config) {
				assert.False(t, cloneResult.Config.ImportNotManaged)
			}

			if tt.name != "Success" {
				assert.Nil(t, cloneResult)
			}

			_ = orchestrator.DeleteVolume(ctx(), cloneConfig.Name)
		})
	}
}

// This test is modeled after TestAddStorageClassVolumes, but we don't need all the
// tests around storage class deletion, etc.
func TestCloneVolumes(t *testing.T) {
	mockPools := tu.GetFakePools()
	orchestrator := getOrchestrator(t, false)

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
		cfg, err := fakedriver.NewFakeStorageDriverConfigJSON(
			c.name, c.protocol,
			pools, volumes,
		)
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
		if backend == nil || err != nil {
			t.Fatalf("Backend %s not stored in orchestrator", c.name)
		}
		persistentBackend, err := orchestrator.storeClient.GetBackend(ctx(), c.name)
		if err != nil {
			t.Fatalf("Unable to get backend %s from persistent store: %v", c.name, err)
		} else if !reflect.DeepEqual(backend.ConstructPersistent(ctx()), persistentBackend) {
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
			config:          tu.GenerateVolumeConfig("block", 1, "specific", config.Block),
			expectedSuccess: true,
			expectedMatches: []*tu.PoolMatch{
				{Backend: "slow-block", Pool: tu.SlowNoSnapshots},
				{Backend: "slow-block", Pool: tu.MediumOverlap},
			},
		},
	} {
		// Create the source volume
		_, err := orchestrator.AddVolume(ctx(), s.config)
		if err != nil {
			t.Errorf("%s: got unexpected error %v", s.name, err)
			continue
		}

		// Now clone the volume and ensure everything looks fine
		cloneName := s.config.Name + "_clone"
		cloneConfig := &storage.VolumeConfig{
			Name:              cloneName,
			StorageClass:      s.config.StorageClass,
			CloneSourceVolume: s.config.Name,
			VolumeMode:        s.config.VolumeMode,
		}
		cloneResult, err := orchestrator.CloneVolume(ctx(), cloneConfig)
		if err != nil {
			t.Errorf("%s: got unexpected error %v", s.name, err)
			continue
		}

		orchestrator.mutex.Lock()

		volume, found := orchestrator.volumes[s.config.Name]
		if !found {
			t.Errorf("%s: did not get volume where expected.", s.name)
		}
		clone, found := orchestrator.volumes[cloneName]
		if !found {
			t.Errorf("%s: did not get volume clone where expected.", cloneName)
		}

		// Clone must reside in the same place as the source
		if clone.BackendUUID != volume.BackendUUID {
			t.Errorf("%s: Clone placed on unexpected backend: %s", cloneName, clone.BackendUUID)
		}

		// Clone should be registered in the store just like any other volume
		externalClone, err := orchestrator.storeClient.GetVolume(ctx(), cloneName)
		if err != nil {
			t.Errorf("%s: unable to communicate with backing store: %v", cloneName, err)
		}
		if !reflect.DeepEqual(externalClone, cloneResult) {
			t.Errorf("%s: external volume %s stored in backend does not match created volume.", cloneName,
				externalClone.Config.Name)
			externalCloneJSON, err := json.Marshal(externalClone)
			if err != nil {
				t.Fatal("Unable to remarshal JSON: ", err)
			}
			origCloneJSON, err := json.Marshal(cloneResult)
			if err != nil {
				t.Fatal("Unable to remarshal JSON: ", err)
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
		{Name: "origVolume01", RequestedPool: "primary", PhysicalPool: "primary", SizeBytes: 1000000000},
		{Name: "origVolume02", RequestedPool: "primary", PhysicalPool: "primary", SizeBytes: 1000000000},
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

func captureOutput(f func()) string {
	var buf bytes.Buffer
	logging.InitLogOutput(&buf)
	defer logging.InitLogOutput(io.Discard)
	f()
	return buf.String()
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
	orchestrator := getOrchestrator(t, false)
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
	logging.Log().WithFields(
		logging.LogFields{
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
				logging.Log().WithFields(
					logging.LogFields{
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
	newOrchestrator := getOrchestrator(t, false)
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

func backendPasswordsInLogsHelper(t *testing.T, debugTraceFlags map[string]bool) {
	level := logging.GetLogLevel()
	_ = logging.SetDefaultLogLevel("debug")
	defer func() { _ = logging.SetDefaultLogLevel(level) }()

	backendName := "passwordBackend"
	backendProtocol := config.File

	orchestrator := getOrchestrator(t, false)

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

func TestBackendPasswordsInLogs(t *testing.T) {
	backendPasswordsInLogsHelper(t, nil)
	backendPasswordsInLogsHelper(t, map[string]bool{"method": true})
}

func TestEmptyBackendDeletion(t *testing.T) {
	const (
		backendName     = "emptyBackend"
		backendProtocol = config.File
	)

	orchestrator := getOrchestrator(t, false)
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

func TestBootstrapSnapshotMissingVolume(t *testing.T) {
	const (
		offlineBackendName = "snapNoVolBackend"
		scName             = "snapNoVolSC"
		volumeName         = "snapNoVolVolume"
		snapName           = "snapNoVolSnapshot"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator(t, false)
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

	newOrchestrator := getOrchestrator(t, false)
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

func TestBootstrapSnapshotMissingBackend(t *testing.T) {
	const (
		offlineBackendName = "snapNoBackBackend"
		scName             = "snapNoBackSC"
		volumeName         = "snapNoBackVolume"
		snapName           = "snapNoBackSnapshot"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator(t, false)
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

	newOrchestrator := getOrchestrator(t, false)
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

func TestBootstrapVolumeMissingBackend(t *testing.T) {
	const (
		offlineBackendName = "bootstrapVolBackend"
		scName             = "bootstrapVolSC"
		volumeName         = "bootstrapVolVolume"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator(t, false)
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

	newOrchestrator := getOrchestrator(t, false)
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

func TestBackendCleanup(t *testing.T) {
	const (
		offlineBackendName = "cleanupBackend"
		onlineBackendName  = "onlineBackend"
		scName             = "cleanupBackendTest"
		volumeName         = "cleanupVolume"
		backendProtocol    = config.File
	)

	orchestrator := getOrchestrator(t, false)
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

	newOrchestrator := getOrchestrator(t, false)
	if bootstrappedBackend, _ := newOrchestrator.GetBackend(ctx(), offlineBackendName); bootstrappedBackend != nil {
		t.Error("Empty offline backend not deleted during bootstrap.")
	}
	if bootstrappedBackend, _ := newOrchestrator.GetBackend(ctx(), onlineBackendName); bootstrappedBackend == nil {
		t.Error("Empty online backend deleted during bootstrap.")
	}
}

func TestLoadBackend(t *testing.T) {
	const (
		backendName = "load-backend-test"
	)
	// volumes must be nil in order to satisfy reflect.DeepEqual comparison. It isn't recommended to compare slices with deepEqual
	var volumes []fake.Volume
	orchestrator := getOrchestrator(t, false)
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

	newOrchestrator := getOrchestrator(t, false)
	if bootstrappedBackend, _ := newOrchestrator.GetBackend(ctx(), backendName); bootstrappedBackend == nil {
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
		newOrchestrator := getOrchestrator(t, false)
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
			t.Fatalf("%e", errors.TypeAssertionError("backend.Driver().(*fakedriver.StorageDriver)"))
		}
		// Destroy should be always called on the backend
		if _, ok := f.DestroyedVolumes[f.GetInternalVolumeName(ctx(), c.volumeConfig, nil)]; !ok && c.expectDestroy {
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

func TestAddVolumeRecovery(t *testing.T) {
	const (
		backendName      = "addRecoveryBackend"
		scName           = "addRecoveryBackendSC"
		fullVolumeName   = "addRecoveryVolumeFull"
		txOnlyVolumeName = "addRecoveryVolumeTxOnly"
	)
	orchestrator := getOrchestrator(t, false)
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

func TestAddVolumeWithTMRNonONTAPNAS(t *testing.T) {
	// Add a single backend of fake
	// create volume with relationship annotation added
	// witness failure
	const (
		backendName    = "addRecoveryBackend"
		scName         = "addRecoveryBackendSC"
		fullVolumeName = "addRecoveryVolumeFull"
	)
	orchestrator := getOrchestrator(t, false)
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

func TestDeleteVolumeRecovery(t *testing.T) {
	const (
		backendName      = "deleteRecoveryBackend"
		scName           = "deleteRecoveryBackendSC"
		fullVolumeName   = "deleteRecoveryVolumeFull"
		txOnlyVolumeName = "deleteRecoveryVolumeTxOnly"
	)
	orchestrator := getOrchestrator(t, false)
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
		if err := orchestrator.storeClient.AddVolumeTransaction(ctx(), volTxn); err != nil {
			t.Fatalf("%s: Unable to create volume transaction: %v", c.name, err)
		}
		newOrchestrator := getOrchestrator(t, false)
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
			t.Fatalf("%e", errors.TypeAssertionError("backend.Driver().(*fakedriver.StorageDriver)"))
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

func TestAddSnapshotRecovery(t *testing.T) {
	const (
		backendName        = "addSnapshotRecoveryBackend"
		scName             = "addSnapshotRecoveryBackendSC"
		volumeName         = "addSnapshotRecoveryVolume"
		fullSnapshotName   = "addSnapshotRecoverySnapshotFull"
		txOnlySnapshotName = "addSnapshotRecoverySnapshotTxOnly"
	)
	orchestrator := getOrchestrator(t, false)
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

func TestDeleteSnapshotRecovery(t *testing.T) {
	const (
		backendName        = "deleteSnapshotRecoveryBackend"
		scName             = "deleteSnapshotRecoveryBackendSC"
		volumeName         = "deleteSnapshotRecoveryVolume"
		fullSnapshotName   = "deleteSnapshotRecoverySnapshotFull"
		txOnlySnapshotName = "deleteSnapshotRecoverySnapshotTxOnly"
	)
	orchestrator := getOrchestrator(t, false)
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
func TestStorageClassOnlyBootstrap(t *testing.T) {
	const scName = "storageclass-only"

	orchestrator := getOrchestrator(t, false)
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
	newOrchestrator := getOrchestrator(t, false)
	bootstrappedSC, err := newOrchestrator.GetStorageClass(ctx(), scName)
	if bootstrappedSC == nil || err != nil {
		t.Error("Unable to find storage class after bootstrapping.")
	} else if !reflect.DeepEqual(bootstrappedSC, originalSC) {
		t.Errorf("External storage classs differ:\n\tOriginal: %v\n\tBootstrapped: %v", originalSC, bootstrappedSC)
	}
	cleanup(t, orchestrator)
}

func TestFirstVolumeRecovery(t *testing.T) {
	const (
		backendName      = "firstRecoveryBackend"
		scName           = "firstRecoveryBackendSC"
		txOnlyVolumeName = "firstRecoveryVolumeTxOnly"
	)
	orchestrator := getOrchestrator(t, false)
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

	orchestrator := getOrchestrator(t, false)
	orchestrator.bootstrapped = false
	orchestrator.bootstrapError = errors.NotReadyError()

	backend, err = orchestrator.AddBackend(ctx(), "", "")
	if backend != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected AddBackend to return an error.")
	}

	backend, err = orchestrator.GetBackend(ctx(), "")
	if backend != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected GetBackend to return an error.")
	}

	backends, err = orchestrator.ListBackends(ctx())
	if backends != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected ListBackends to return an error.")
	}

	err = orchestrator.DeleteBackend(ctx(), "")
	if !errors.IsNotReadyError(err) {
		t.Errorf("Expected DeleteBackend to return an error.")
	}

	volume, err = orchestrator.AddVolume(ctx(), nil)
	if volume != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected AddVolume to return an error.")
	}

	volume, err = orchestrator.CloneVolume(ctx(), nil)
	if volume != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected CloneVolume to return an error.")
	}

	volume, err = orchestrator.GetVolume(ctx(), "")
	if volume != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected GetVolume to return an error.")
	}

	volumes, err = orchestrator.ListVolumes(ctx())
	if volumes != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected ListVolumes to return an error.")
	}

	err = orchestrator.DeleteVolume(ctx(), "")
	if !errors.IsNotReadyError(err) {
		t.Errorf("Expected DeleteVolume to return an error.")
	}

	err = orchestrator.AttachVolume(ctx(), "", "", nil)
	if !errors.IsNotReadyError(err) {
		t.Errorf("Expected AttachVolume to return an error.")
	}

	err = orchestrator.DetachVolume(ctx(), "", "")
	if !errors.IsNotReadyError(err) {
		t.Errorf("Expected DetachVolume to return an error.")
	}

	snapshot, err = orchestrator.CreateSnapshot(ctx(), nil)
	if snapshot != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected CreateSnapshot to return an error.")
	}

	snapshot, err = orchestrator.GetSnapshot(ctx(), "", "")
	if snapshot != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected GetSnapshot to return an error.")
	}

	snapshots, err = orchestrator.ListSnapshots(ctx())
	if snapshots != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected ListSnapshots to return an error.")
	}

	snapshots, err = orchestrator.ReadSnapshotsForVolume(ctx(), "")
	if snapshots != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected ReadSnapshotsForVolume to return an error.")
	}

	err = orchestrator.DeleteSnapshot(ctx(), "", "")
	if !errors.IsNotReadyError(err) {
		t.Errorf("Expected DeleteSnapshot to return an error.")
	}

	err = orchestrator.ReloadVolumes(ctx())
	if !errors.IsNotReadyError(err) {
		t.Errorf("Expected ReloadVolumes to return an error.")
	}

	storageClass, err = orchestrator.AddStorageClass(ctx(), nil)
	if storageClass != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected AddStorageClass to return an error.")
	}

	storageClass, err = orchestrator.UpdateStorageClass(ctx(), nil)
	if storageClass != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected UpdateStorageClass to return an error.")
	}

	storageClass, err = orchestrator.GetStorageClass(ctx(), "")
	if storageClass != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected GetStorageClass to return an error.")
	}

	storageClasses, err = orchestrator.ListStorageClasses(ctx())
	if storageClasses != nil || !errors.IsNotReadyError(err) {
		t.Errorf("Expected ListStorageClasses to return an error.")
	}

	err = orchestrator.DeleteStorageClass(ctx(), "")
	if !errors.IsNotReadyError(err) {
		t.Errorf("Expected DeleteStorageClass to return an error.")
	}
}

func importVolumeSetup(
	t *testing.T, backendName, scName, volumeName, importOriginalName string,
	backendProtocol config.Protocol,
) (*TridentOrchestrator, *storage.VolumeConfig) {
	// Object setup
	orchestrator := getOrchestrator(t, false)
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
	// Set size matching to fake driver.
	volumeConfig.Size = fmt.Sprintf("%d", 1000000000)
	volumeConfig.ImportOriginalName = importOriginalName
	volumeConfig.ImportBackendUUID = backendUUID
	return orchestrator, volumeConfig
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
		err = orchestrator.validateImportVolume(coreCtx, c.volumeConfig)
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

	sizeGreaterVolConfig := volumeConfig.ConstructClone()
	sizeGreaterVolConfig.Size = fmt.Sprintf("%d", 50*1024*1024*1024) // bigger size than fake driver reported size

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
		{
			name:         "invalidSize",
			volumeConfig: sizeGreaterVolConfig,
			valid:        false,
			error:        "requested size is more than actual size",
		},
	} {
		// The test code
		err = orchestrator.validateImportVolume(coreCtx, c.volumeConfig)
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

func TestAddVolumePublication(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Verify that the core calls the store client with the correct object, returning success
	mockStoreClient.EXPECT().AddVolumePublication(gomock.Any(), fakePub).Return(nil)

	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient

	err := orchestrator.addVolumePublication(context.Background(), fakePub)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error adding volume publication: %v", err))
	assert.Contains(t, orchestrator.volumePublications.Map(), fakePub.VolumeName,
		"volume publication missing from orchestrator's cache")
	assert.NotNil(t, orchestrator.volumePublications.Get(fakePub.VolumeName, fakePub.NodeName),
		"volume publication missing from orchestrator's cache")
	assert.Equal(t, fakePub, orchestrator.volumePublications.Get(fakePub.VolumeName, fakePub.NodeName),
		"volume publication was not correctly added")
}

func TestAddVolumePublicationError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Verify that the core calls the store client with the correct object, but return an error
	mockStoreClient.EXPECT().AddVolumePublication(gomock.Any(), fakePub).Return(fmt.Errorf("fake error"))

	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient

	err := orchestrator.addVolumePublication(context.Background(), fakePub)
	assert.NotNilf(t, err, "add volume publication did not return an error")
	assert.NotContains(t, orchestrator.volumePublications.Map(), fakePub.VolumeName,
		"volume publication was added orchestrator's cache")
}

func TestGetVolumePublication(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	if err := orchestrator.volumePublications.Set(fakePub.VolumeName, fakePub.NodeName, fakePub); err != nil {
		t.Fatal("unable to set cache value")
	}

	actualPub, err := orchestrator.GetVolumePublication(ctx(), fakePub.VolumeName, fakePub.NodeName)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error getting volume publication: %v", err))
	assert.Equal(t, fakePub, actualPub, "volume publication was not correctly retrieved")
}

func TestGetVolumePublicationNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient

	actualPub, err := orchestrator.GetVolumePublication(ctx(), "NotFound", "NotFound")
	assert.NotNilf(t, err, fmt.Sprintf("unexpected success getting volume publication: %v", err))
	assert.True(t, errors.IsNotFoundError(err), "incorrect error type returned")
	assert.Empty(t, actualPub, "non-empty publication returned")
}

func TestGetVolumePublicationError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	if err := orchestrator.volumePublications.Set(fakePub.VolumeName, fakePub.NodeName, fakePub); err != nil {
		t.Fatal("unable to set cache value")
	}

	// Simulate a bootstrap error
	orchestrator.bootstrapError = fmt.Errorf("some error")

	actualPub, err := orchestrator.GetVolumePublication(ctx(), fakePub.VolumeName, fakePub.NodeName)
	assert.NotNilf(t, err, fmt.Sprintf("unexpected success getting volume publication: %v", err))
	assert.False(t, errors.IsNotFoundError(err), "incorrect error type returned")
	assert.Empty(t, actualPub, "non-empty publication returned")
}

func TestListVolumePublications(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub1 := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub2 := &models.VolumePublication{
		Name:       "baz/biz",
		NodeName:   "biz",
		VolumeName: "baz",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub3 := &models.VolumePublication{
		Name:       fmt.Sprintf("%s/buz", fakePub1.VolumeName),
		NodeName:   "buz",
		VolumeName: fakePub1.VolumeName,
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	err := orchestrator.volumePublications.Set(fakePub1.VolumeName, fakePub1.NodeName, fakePub1)
	err = orchestrator.volumePublications.Set(fakePub2.VolumeName, fakePub2.NodeName, fakePub2)
	err = orchestrator.volumePublications.Set(fakePub3.VolumeName, fakePub3.NodeName, fakePub3)
	if err != nil {
		t.Fatal("unable to set cache value")
	}

	expectedAllPubs := []*models.VolumePublicationExternal{
		fakePub1.ConstructExternal(),
		fakePub2.ConstructExternal(),
		fakePub3.ConstructExternal(),
	}

	actualPubs, err := orchestrator.ListVolumePublications(context.Background())
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.ElementsMatch(t, expectedAllPubs, actualPubs, "incorrect publication list returned")
}

func TestListVolumePublicationsNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient

	actualPubs, err := orchestrator.ListVolumePublications(context.Background())
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

func TestListVolumePublicationsError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	if err := orchestrator.volumePublications.Set(fakePub.VolumeName, fakePub.NodeName, fakePub); err != nil {
		t.Fatal("unable to set cache value")
	}

	// Simulate a bootstrap error
	orchestrator.bootstrapError = fmt.Errorf("some error")

	actualPubs, err := orchestrator.ListVolumePublications(context.Background())
	assert.NotNil(t, err, fmt.Sprintf("unexpected success listing volume publications"))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

func TestListVolumePublicationsForVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub1 := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub2 := &models.VolumePublication{
		Name:       "baz/biz",
		NodeName:   "biz",
		VolumeName: "baz",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub3 := &models.VolumePublication{
		Name:       fmt.Sprintf("%s/buz", fakePub1.VolumeName),
		NodeName:   "buz",
		VolumeName: fakePub1.VolumeName,
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	err := orchestrator.volumePublications.Set(fakePub1.VolumeName, fakePub1.NodeName, fakePub1)
	err = orchestrator.volumePublications.Set(fakePub2.VolumeName, fakePub2.NodeName, fakePub2)
	err = orchestrator.volumePublications.Set(fakePub3.VolumeName, fakePub3.NodeName, fakePub3)
	if err != nil {
		t.Fatal("unable to set cache value")
	}

	expectedAllPubs := []*models.VolumePublicationExternal{fakePub1.ConstructExternal(), fakePub3.ConstructExternal()}

	actualPubs, err := orchestrator.ListVolumePublicationsForVolume(context.Background(), fakePub1.VolumeName)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.ElementsMatch(t, expectedAllPubs, actualPubs, "incorrect publication list returned")
}

func TestListVolumePublicationsForVolumeNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	if err := orchestrator.volumePublications.Set(fakePub.VolumeName, fakePub.NodeName, fakePub); err != nil {
		t.Fatal("unable to set cache value")
	}

	actualPubs, err := orchestrator.ListVolumePublicationsForVolume(context.Background(), "NotFound")
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

func TestListVolumePublicationsForVolumeError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	if err := orchestrator.volumePublications.Set(fakePub.VolumeName, fakePub.NodeName, fakePub); err != nil {
		t.Fatal("unable to set cache value")
	}

	// Simulate a bootstrap error
	orchestrator.bootstrapError = fmt.Errorf("some error")

	actualPubs, err := orchestrator.ListVolumePublicationsForVolume(context.Background(), fakePub.VolumeName)
	assert.NotNil(t, err, fmt.Sprintf("unexpected success listing volume publications"))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

func TestListVolumePublicationsForNode(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub1 := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub2 := &models.VolumePublication{
		Name:       "baz/biz",
		NodeName:   "biz",
		VolumeName: "baz",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub3 := &models.VolumePublication{
		Name:       fmt.Sprintf("%s/buz", fakePub1.VolumeName),
		NodeName:   "buz",
		VolumeName: fakePub1.VolumeName,
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	err := orchestrator.volumePublications.Set(fakePub1.VolumeName, fakePub1.NodeName, fakePub1)
	err = orchestrator.volumePublications.Set(fakePub2.VolumeName, fakePub2.NodeName, fakePub2)
	err = orchestrator.volumePublications.Set(fakePub3.VolumeName, fakePub3.NodeName, fakePub3)
	if err != nil {
		t.Fatal("unable to set cache value")
	}

	expectedAllPubs := []*models.VolumePublicationExternal{fakePub2.ConstructExternal()}

	actualPubs, err := orchestrator.ListVolumePublicationsForNode(context.Background(), fakePub2.NodeName)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.ElementsMatch(t, expectedAllPubs, actualPubs, "incorrect publication list returned")
}

func TestListVolumePublicationsForNodeNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	if err := orchestrator.volumePublications.Set(fakePub.VolumeName, fakePub.NodeName, fakePub); err != nil {
		t.Fatal("unable to set cache value")
	}

	actualPubs, err := orchestrator.ListVolumePublicationsForNode(context.Background(), "NotFound")
	assert.Nilf(t, err, fmt.Sprintf("unexpected error listing volume publications: %v", err))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

func TestListVolumePublicationsForNodeError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	if err := orchestrator.volumePublications.Set(fakePub.VolumeName, fakePub.NodeName, fakePub); err != nil {
		t.Fatal("unable to set cache value")
	}

	// Simulate a bootstrap error
	orchestrator.bootstrapError = fmt.Errorf("some error")

	actualPubs, err := orchestrator.ListVolumePublicationsForNode(context.Background(), fakePub.NodeName)
	assert.NotNil(t, err, fmt.Sprintf("unexpected success listing volume publications"))
	assert.Empty(t, actualPubs, "non-empty publication list returned")
}

func TestListVolumePublicationsForNodePublicationsNotSyncedError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Setting this to false will cause a not ready error.
	orchestrator.volumePublicationsSynced = false
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	err := orchestrator.volumePublications.Set(fakePub.VolumeName, fakePub.NodeName, fakePub)
	assert.NoError(t, err)

	actualPubs, err := orchestrator.ListVolumePublicationsForNode(context.Background(), fakePub.NodeName)
	assert.Error(t, err)
	assert.Nil(t, actualPubs, "non-empty publication list returned")
}

func TestDeleteVolumePublication(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Create a fake VolumePublication
	fakePub1 := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub2 := &models.VolumePublication{
		Name:       "baz/biz",
		NodeName:   "biz",
		VolumeName: "baz",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakePub3 := &models.VolumePublication{
		Name:       fmt.Sprintf("%s/buz", fakePub1.VolumeName),
		NodeName:   "buz",
		VolumeName: fakePub1.VolumeName,
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakeNode := &models.Node{Name: "biz"}
	fakeNode2 := &models.Node{Name: "buz"}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	err := orchestrator.volumePublications.Set(fakePub1.VolumeName, fakePub1.NodeName, fakePub1)
	err = orchestrator.volumePublications.Set(fakePub2.VolumeName, fakePub2.NodeName, fakePub2)
	err = orchestrator.volumePublications.Set(fakePub3.VolumeName, fakePub3.NodeName, fakePub3)
	if err != nil {
		t.Fatal("unable to set cache value")
	}

	orchestrator.nodes.Set(fakeNode.Name, fakeNode)
	orchestrator.nodes.Set(fakeNode2.Name, fakeNode2)

	// Verify if this is the last nodeID for a given volume the volume entry is completely removed from the cache
	mockStoreClient.EXPECT().DeleteVolumePublication(gomock.Any(), fakePub2).Return(nil)
	err = orchestrator.DeleteVolumePublication(ctx(), fakePub2.VolumeName, fakePub2.NodeName)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error deleting volume publication: %v", err))

	cachedPub, ok := orchestrator.volumePublications.TryGet(fakePub2.VolumeName, fakePub2.NodeName)
	assert.False(t, ok, "publication not removed from the cache")
	assert.Nil(t, cachedPub, "publication not removed from the cache")

	// Verify if this is not the last nodeID for a given volume the volume entry is not removed from the cache
	mockStoreClient.EXPECT().DeleteVolumePublication(gomock.Any(), fakePub3).Return(nil)
	err = orchestrator.DeleteVolumePublication(ctx(), fakePub3.VolumeName, fakePub3.NodeName)
	assert.NoError(t, err, fmt.Sprintf("unexpected error deleting volume publication: %v", err))

	cachedPub, ok = orchestrator.volumePublications.TryGet(fakePub3.VolumeName, fakePub3.NodeName)
	assert.False(t, ok, "publication not removed from the cache")
	assert.Nil(t, cachedPub, "publication not removed from the cache")

	cachedPub, ok = orchestrator.volumePublications.TryGet(fakePub1.VolumeName, fakePub1.NodeName)
	assert.True(t, ok, "publication improperly removed from the cache")
	assert.NotNil(t, cachedPub, "publication improperly removed from the cache")
}

func TestDeleteVolumePublicationNotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client.
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase.
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication.
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakeNode := &models.Node{Name: fakePub.NodeName}

	// Create an instance of the orchestrator for this test.
	orchestrator := getOrchestrator(t, false)
	orchestrator.nodes.Set(fakePub.NodeName, fakeNode)
	// Add the mocked objects to the orchestrator.
	orchestrator.storeClient = mockStoreClient
	if err := orchestrator.volumePublications.Set(fakePub.VolumeName, fakePub.NodeName, fakePub); err != nil {
		t.Fatal("unable to set publication in the cache.")
	}

	// When Trident can't find the publication in the cache, it should ask the persistent store.
	mockStoreClient.EXPECT().DeleteVolumePublication(coreCtx, fakePub).Return(nil).Times(1)
	err := orchestrator.DeleteVolumePublication(ctx(), fakePub.VolumeName, fakePub.NodeName)
	cachedPubs := orchestrator.volumePublications.Map()
	assert.NoError(t, err, fmt.Sprintf("unexpected error deleting volume publication"))
	assert.Nil(t, cachedPubs[fakePub.VolumeName][fakePub.NodeName], "expected no cache entry")
}

func TestDeleteVolumePublicationNotFoundPersistence(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	fakeNode := &models.Node{Name: fakePub.NodeName}

	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	if err := orchestrator.volumePublications.Set(fakePub.VolumeName, fakePub.NodeName, fakePub); err != nil {
		t.Fatal("unable to set cache value")
	}
	orchestrator.nodes.Set(fakeNode.Name, fakeNode)

	// Verify delete is idempotent when the persistence object is missing
	mockStoreClient.EXPECT().DeleteVolumePublication(gomock.Any(), fakePub).Return(errors.NotFoundError("not found"))
	err := orchestrator.DeleteVolumePublication(ctx(), fakePub.VolumeName, fakePub.NodeName)
	assert.Nilf(t, err, fmt.Sprintf("unexpected error deleting volume publication: %v", err))
	assert.NotContains(t, orchestrator.volumePublications.Map(), fakePub.VolumeName,
		"publication not properly removed from cache")
}

func TestDeleteVolumePublicationError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	// Set the store client behavior we don't care about for this testcase
	mockStoreClient.EXPECT().GetVolumeTransactions(gomock.Any()).Return([]*storage.VolumeTransaction{}, nil).AnyTimes()
	// Create a fake VolumePublication
	fakePub := &models.VolumePublication{
		Name:       "foo/bar",
		NodeName:   "bar",
		VolumeName: "foo",
		ReadOnly:   true,
		AccessMode: 1,
	}
	// Create an instance of the orchestrator for this test
	orchestrator := getOrchestrator(t, false)
	// Add the mocked objects to the orchestrator
	orchestrator.storeClient = mockStoreClient
	// Populate volume publications
	if err := orchestrator.volumePublications.Set(fakePub.VolumeName, fakePub.NodeName, fakePub); err != nil {
		t.Fatal("unable to set cache value")
	}

	mockStoreClient.EXPECT().DeleteVolumePublication(gomock.Any(), fakePub).Return(fmt.Errorf("some error"))

	err := orchestrator.DeleteVolumePublication(ctx(), fakePub.VolumeName, fakePub.NodeName)
	assert.NotNil(t, err, fmt.Sprintf("unexpected success deleting volume publication"))
	assert.False(t, errors.IsNotFoundError(err), "incorrect error type returned")
	assert.Equal(t, fakePub, orchestrator.volumePublications.Get(fakePub.VolumeName, fakePub.NodeName),
		"publication improperly removed/updated in cache")
}

func TestAddNode(t *testing.T) {
	node := &models.Node{
		Name:             "testNode",
		IQN:              "myIQN",
		IPs:              []string{"1.1.1.1", "2.2.2.2"},
		TopologyLabels:   map[string]string{"topology.kubernetes.io/region": "Region1"},
		Deleted:          false,
		PublicationState: models.NodeClean,
	}
	orchestrator := getOrchestrator(t, false)
	if err := orchestrator.AddNode(ctx(), node, nil); err != nil {
		t.Errorf("adding node failed; %v", err)
	}
}

func TestUpdateNode(t *testing.T) {
	nodeName := "FakeNode"

	// Test configuration
	tests := []struct {
		name              string
		initialNodeState  models.NodePublicationState
		expectedNodeState models.NodePublicationState
		flags             *models.NodePublicationStateFlags
	}{
		{
			name:              "noFlagsStayClean",
			initialNodeState:  models.NodeClean,
			expectedNodeState: models.NodeClean,
			flags:             nil,
		},
		{
			name:              "noFlagsStayCleanable",
			initialNodeState:  models.NodeCleanable,
			expectedNodeState: models.NodeCleanable,
			flags:             nil,
		},
		{
			name:              "noFlagsStayDirty",
			initialNodeState:  models.NodeDirty,
			expectedNodeState: models.NodeDirty,
			flags:             nil,
		},
		{
			name:              "readyStayClean",
			initialNodeState:  models.NodeClean,
			expectedNodeState: models.NodeClean,
			flags:             &models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(true), AdministratorReady: convert.ToPtr(false)},
		},
		{
			name:              "cleanedStayClean",
			initialNodeState:  models.NodeClean,
			expectedNodeState: models.NodeClean,
			flags:             &models.NodePublicationStateFlags{ProvisionerReady: convert.ToPtr(true)},
		},
		{
			name:              "cleanToDirty",
			initialNodeState:  models.NodeClean,
			expectedNodeState: models.NodeDirty,
			flags:             &models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(false)},
		},
		{
			name:              "cleanableToClean",
			initialNodeState:  models.NodeCleanable,
			expectedNodeState: models.NodeClean,
			flags: &models.NodePublicationStateFlags{
				OrchestratorReady:  convert.ToPtr(true),
				AdministratorReady: convert.ToPtr(true),
				ProvisionerReady:   convert.ToPtr(true),
			},
		},
		{
			name:              "readyStayCleanable",
			initialNodeState:  models.NodeCleanable,
			expectedNodeState: models.NodeCleanable,
			flags:             &models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(true), AdministratorReady: convert.ToPtr(true)},
		},
		{
			name:              "cleanableToDirty",
			initialNodeState:  models.NodeCleanable,
			expectedNodeState: models.NodeDirty,
			flags:             &models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(false)},
		},
		{
			name:              "dirtyToCleanable",
			initialNodeState:  models.NodeDirty,
			expectedNodeState: models.NodeCleanable,
			flags:             &models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(true), AdministratorReady: convert.ToPtr(true)},
		},
		{
			name:              "notReadyStayDirty",
			initialNodeState:  models.NodeDirty,
			expectedNodeState: models.NodeDirty,
			flags:             &models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false), AdministratorReady: convert.ToPtr(true)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

			orchestrator := getOrchestrator(t, false)
			orchestrator.storeClient = mockStoreClient

			node := &models.Node{
				Name:             nodeName,
				IQN:              "myIQN",
				IPs:              []string{"1.1.1.1", "2.2.2.2"},
				TopologyLabels:   map[string]string{"topology.kubernetes.io/region": "Region1"},
				Deleted:          false,
				PublicationState: test.initialNodeState,
			}
			orchestrator.nodes.Set(node.Name, node)

			if test.initialNodeState != test.expectedNodeState {
				mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			}

			result := orchestrator.UpdateNode(ctx(), nodeName, test.flags)

			assert.Nil(t, result, "UpdateNode failed")
			updatedNode, _ := orchestrator.GetNode(ctx(), nodeName)
			assert.Equal(t, test.expectedNodeState, updatedNode.PublicationState, "Unexpected node state")
		})
	}
}

func TestUpdateNode_BootstrapError(t *testing.T) {
	orchestrator := getOrchestrator(t, false)

	expectedError := errors.New("bootstrap_error")
	orchestrator.bootstrapError = expectedError

	result := orchestrator.UpdateNode(ctx(), "testNode1",
		&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false)})

	assert.Equal(t, expectedError, result, "UpdateNode did not return bootstrap error")
}

func TestUpdateNode_NodeNotFound(t *testing.T) {
	orchestrator := getOrchestrator(t, false)

	result := orchestrator.UpdateNode(ctx(), "testNode1",
		&models.NodePublicationStateFlags{OrchestratorReady: convert.ToPtr(false)})

	assert.True(t, errors.IsNotFoundError(result), "UpdateNode did not fail")
}

func TestUpdateNode_NodeStoreUpdateFailed(t *testing.T) {
	node1 := &models.Node{
		Name:             "testNode1",
		IQN:              "myIQN2",
		IPs:              []string{"1.1.1.1", "2.2.2.2"},
		TopologyLabels:   map[string]string{"topology.kubernetes.io/region": "Region1"},
		Deleted:          false,
		PublicationState: models.NodeClean,
	}
	expectedError := errors.New("failure")
	flags := &models.NodePublicationStateFlags{
		OrchestratorReady:  convert.ToPtr(false),
		AdministratorReady: convert.ToPtr(false),
	}

	mockCtrl := gomock.NewController(t)
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

	orchestrator := getOrchestrator(t, false)
	orchestrator.storeClient = mockStoreClient

	mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	mockStoreClient.EXPECT().AddOrUpdateNode(gomock.Any(), gomock.Any()).Return(expectedError).Times(1)

	_ = orchestrator.AddNode(ctx(), node1, nil)

	result := orchestrator.UpdateNode(ctx(), "testNode1", flags)
	assert.NotNil(t, result, "UpdateNode failed")

	updatedNode1, _ := orchestrator.GetNode(ctx(), "testNode1")
	assert.Equal(t, updatedNode1.PublicationState, models.NodeDirty, "Node not dirty")
}

func TestGetNode(t *testing.T) {
	orchestrator := getOrchestrator(t, false)
	expectedNode := &models.Node{
		Name:           "testNode",
		IQN:            "myIQN",
		IPs:            []string{"1.1.1.1", "2.2.2.2"},
		TopologyLabels: map[string]string{"topology.kubernetes.io/region": "Region1"},
		Deleted:        false,
	}
	unexpectedNode := &models.Node{
		Name:           "testNode2",
		IQN:            "myOtherIQN",
		IPs:            []string{"3.3.3.3", "4.4.4.4"},
		TopologyLabels: map[string]string{"topology.kubernetes.io/region": "Region2"},
		Deleted:        false,
	}
	orchestrator.nodes.Set(expectedNode.Name, expectedNode)
	orchestrator.nodes.Set(unexpectedNode.Name, unexpectedNode)

	actualNode, err := orchestrator.GetNode(ctx(), expectedNode.Name)
	if err != nil {
		t.Errorf("error getting node; %v", err)
	}

	expectedExternalNode := expectedNode.ConstructExternal()

	assert.Equalf(t, expectedExternalNode, actualNode,
		"Did not get expected node back; expected %+v, got %+v", expectedExternalNode, actualNode)
}

func TestListNodes(t *testing.T) {
	orchestrator := getOrchestrator(t, false)
	expectedNode1 := &models.Node{
		Name:    "testNode",
		IQN:     "myIQN",
		IPs:     []string{"1.1.1.1", "2.2.2.2"},
		Deleted: false,
	}
	expectedNode2 := &models.Node{
		Name:    "testNode2",
		IQN:     "myOtherIQN",
		IPs:     []string{"3.3.3.3", "4.4.4.4"},
		Deleted: false,
	}
	orchestrator.nodes.Set(expectedNode1.Name, expectedNode1)
	orchestrator.nodes.Set(expectedNode2.Name, expectedNode2)
	expectedNodes := []*models.Node{expectedNode1, expectedNode2}

	actualNodes, err := orchestrator.ListNodes(ctx())
	if err != nil {
		t.Errorf("error listing nodes; %v", err)
	}

	expectedExternalNodes := make([]*models.NodeExternal, 0, len(expectedNodes))
	for _, n := range expectedNodes {
		expectedExternalNodes = append(expectedExternalNodes, n.ConstructExternal())
	}

	assert.ElementsMatchf(t, actualNodes, expectedExternalNodes,
		"node list values do not match; expected %v, found %v", expectedExternalNodes, actualNodes)
}

func TestDeleteNode(t *testing.T) {
	orchestrator := getOrchestrator(t, false)
	initialNode := &models.Node{
		Name:    "testNode",
		IQN:     "myIQN",
		IPs:     []string{"1.1.1.1", "2.2.2.2"},
		Deleted: false,
	}
	orchestrator.nodes.Set(initialNode.Name, initialNode)

	if err := orchestrator.DeleteNode(ctx(), initialNode.Name); err != nil {
		t.Errorf("error deleting node; %v", err)
	}

	if n := orchestrator.nodes.Get(initialNode.Name); n != nil {
		t.Errorf("node was not properly deleted")
	}
}

func TestSnapshotVolumes(t *testing.T) {
	mockPools := tu.GetFakePools()
	orchestrator := getOrchestrator(t, false)

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

func TestGetProtocol(t *testing.T) {
	orchestrator := getOrchestrator(t, false)

	type accessVariables struct {
		volumeMode config.VolumeMode
		accessMode config.AccessMode
		protocol   config.Protocol
		expected   config.Protocol
	}

	accessModesPositiveTests := []accessVariables{
		{config.Filesystem, config.ModeAny, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ModeAny, config.File, config.File},
		{config.Filesystem, config.ModeAny, config.Block, config.Block},
		{config.Filesystem, config.ReadWriteOnce, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ReadWriteOnce, config.File, config.File},
		{config.Filesystem, config.ReadWriteOnce, config.Block, config.Block},
		{config.Filesystem, config.ReadWriteOncePod, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ReadWriteOncePod, config.File, config.File},
		{config.Filesystem, config.ReadWriteOncePod, config.Block, config.Block},
		{config.Filesystem, config.ReadOnlyMany, config.Block, config.Block},
		{config.Filesystem, config.ReadOnlyMany, config.ProtocolAny, config.ProtocolAny},
		{config.Filesystem, config.ReadOnlyMany, config.File, config.File},
		{config.Filesystem, config.ReadWriteMany, config.ProtocolAny, config.File},
		{config.Filesystem, config.ReadWriteMany, config.File, config.File},
		// {config.Filesystem, config.ReadWriteMany, config.Block, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ModeAny, config.File, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.Block, config.Block},
		{config.RawBlock, config.ReadWriteOnce, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ReadWriteOnce, config.File, config.ProtocolAny},
		// {config.RawBlock, config.ReadWriteOncePod, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteOnce, config.Block, config.Block},
		{config.RawBlock, config.ReadWriteOncePod, config.Block, config.Block},
		{config.RawBlock, config.ReadOnlyMany, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ReadOnlyMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadOnlyMany, config.Block, config.Block},
		{config.RawBlock, config.ReadWriteMany, config.ProtocolAny, config.Block},
		// {config.RawBlock, config.ReadWriteMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteMany, config.Block, config.Block},
	}

	accessModesNegativeTests := []accessVariables{
		{config.Filesystem, config.ReadWriteMany, config.Block, config.ProtocolAny},
		{config.RawBlock, config.ModeAny, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteOnce, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteOncePod, config.File, config.ProtocolAny},

		{config.RawBlock, config.ReadOnlyMany, config.File, config.ProtocolAny},
		{config.RawBlock, config.ReadWriteMany, config.File, config.ProtocolAny},
	}

	for _, tc := range accessModesPositiveTests {
		protocolLocal, err := orchestrator.getProtocol(coreCtx, tc.volumeMode, tc.accessMode, tc.protocol)
		assert.Nil(t, err, nil)
		assert.Equal(t, tc.expected, protocolLocal, "expected both the protocols to be equal!")
	}

	for _, tc := range accessModesNegativeTests {
		protocolLocal, err := orchestrator.getProtocol(coreCtx, tc.volumeMode, tc.accessMode, tc.protocol)
		assert.NotNil(t, err)
		assert.Equal(t, tc.expected, protocolLocal, "expected both the protocols to be equal!")
	}
}

func TestAddStorageClass(t *testing.T) {
	orchestrator := getOrchestrator(t, false)
	defer cleanup(t, orchestrator)

	storageClassConfig := &storageclass.Config{
		Name: "test-storage-class",
		Attributes: map[string]sa.Request{
			sa.IOPS: sa.NewIntRequest(1000),
		},
	}

	// First add should work
	storageClassExt, err := orchestrator.AddStorageClass(ctx(), storageClassConfig)
	if err != nil {
		t.Errorf("Failed to add storage class: %v", err)
	}

	if _, ok := orchestrator.storageClasses[storageClassExt.GetName()]; !ok {
		t.Errorf("Storage class %s not found in orchestrator", storageClassExt.GetName())
	}

	// Second add should fail
	storageClassExt, err = orchestrator.AddStorageClass(ctx(), storageClassConfig)

	assert.Nil(t, storageClassExt, "Value should be nil")
	assert.Error(t, err, "Add should have failed")
}

func TestAddStorageClass_PersistentStoreError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	orchestrator, err := NewTridentOrchestrator(mockStoreClient)
	orchestrator.bootstrapped = true
	orchestrator.bootstrapError = nil

	mockStoreClient.EXPECT().AddBackend(gomock.Any(), gomock.Any()).Return(nil)
	mockStoreClient.EXPECT().AddStorageClass(gomock.Any(), gomock.Any()).Return(errors.New("failed"))

	// Add backend with two pools
	mockPools := tu.GetFakePools()
	pools := map[string]*fake.StoragePool{
		"fast-small":     mockPools[tu.FastSmall],
		"slow-snapshots": mockPools[tu.SlowSnapshots],
	}
	cfg, err := fakedriver.NewFakeStorageDriverConfigJSON("backendWithTwoPools", config.File, pools, make([]fake.Volume, 0))
	if err != nil {
		t.Fatalf("Unable to generate cfg JSON: %v", err)
	}
	_, err = orchestrator.AddBackend(ctx(), cfg, "")
	if err != nil {
		t.Fatalf("Unable to add backend: %v", err)
	}

	// Add initial storage class
	initialStorageClassConfig := &storageclass.Config{
		Name: "initial-storage-class",
		Attributes: map[string]sa.Request{
			sa.IOPS: sa.NewIntRequest(2000),
		},
	}
	_, err = orchestrator.AddStorageClass(ctx(), initialStorageClassConfig)
	assert.Error(t, err, "AddStorageClass should have failed")
}

func TestUpdateStorageClass(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	orchestrator, err := NewTridentOrchestrator(mockStoreClient)
	orchestrator.bootstrapError = nil

	// Test 1: Update existing storage class
	scConfig := &storageclass.Config{Name: "testSC"}
	sc := storageclass.New(scConfig)
	orchestrator.storageClasses[sc.GetName()] = sc

	mockStoreClient.EXPECT().UpdateStorageClass(gomock.Any(), sc).Return(nil).Times(1)
	_, err = orchestrator.UpdateStorageClass(ctx(), scConfig)
	assert.NoError(t, err, "should not return an error when updating an existing storage class")

	// Test 2: Update non-existing storage class
	scConfig = &storageclass.Config{Name: "nonExistentSC"}
	_, err = orchestrator.UpdateStorageClass(ctx(), scConfig)
	assert.Error(t, err, "should return an error when updating a non-existing storage class")

	// Test 3: Error updating storage class in store
	scConfig = &storageclass.Config{Name: "testSC"}
	sc = storageclass.New(scConfig)
	orchestrator.storageClasses[sc.GetName()] = sc

	mockStoreClient.EXPECT().UpdateStorageClass(gomock.Any(), sc).Return(fmt.Errorf("store error")).Times(1)
	_, err = orchestrator.UpdateStorageClass(ctx(), scConfig)
	assert.Error(t, err, "should return an error when store client fails to update storage class")
}

func TestUpdateStorageClassWithBackends(t *testing.T) {
	// Ensure backend-to-storageclass mapping is updated

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	orchestrator, err := NewTridentOrchestrator(mockStoreClient)
	orchestrator.bootstrapped = true
	orchestrator.bootstrapError = nil

	mockStoreClient.EXPECT().AddBackend(gomock.Any(), gomock.Any()).Return(nil)
	mockStoreClient.EXPECT().AddStorageClass(gomock.Any(), gomock.Any()).Return(nil)
	mockStoreClient.EXPECT().UpdateStorageClass(gomock.Any(), gomock.Any()).Return(nil)

	// Add backend with two pools
	mockPools := tu.GetFakePools()
	pools := map[string]*fake.StoragePool{
		"fast-small":     mockPools[tu.FastSmall],
		"slow-snapshots": mockPools[tu.SlowSnapshots],
	}
	cfg, err := fakedriver.NewFakeStorageDriverConfigJSON("backendWithTwoPools", config.File, pools, make([]fake.Volume, 0))
	if err != nil {
		t.Fatalf("Unable to generate cfg JSON: %v", err)
	}
	_, err = orchestrator.AddBackend(ctx(), cfg, "")
	if err != nil {
		t.Fatalf("Unable to add backend: %v", err)
	}

	// Add initial storage class
	initialStorageClassConfig := &storageclass.Config{
		Name: "initial-storage-class",
		Attributes: map[string]sa.Request{
			sa.IOPS: sa.NewIntRequest(2000),
		},
	}
	_, err = orchestrator.AddStorageClass(ctx(), initialStorageClassConfig)
	if err != nil {
		t.Fatalf("Unable to add initial storage class: %v", err)
	}

	// Validate initial storage class matches pool1
	mockStoreClient.EXPECT().GetStorageClass(gomock.Any(), gomock.Any()).Return(
		orchestrator.storageClasses["initial-storage-class"].ConstructPersistent(), nil)

	validateStorageClass(t, orchestrator, initialStorageClassConfig.Name, []*tu.PoolMatch{
		{Backend: "backendWithTwoPools", Pool: tu.FastSmall},
	})

	// Update storage class to match pool2
	updatedStorageClassConfig := &storageclass.Config{
		Name: "initial-storage-class",
		Attributes: map[string]sa.Request{
			sa.IOPS: sa.NewIntRequest(40),
		},
	}
	_, err = orchestrator.UpdateStorageClass(ctx(), updatedStorageClassConfig)
	if err != nil {
		t.Fatalf("Unable to update storage class: %v", err)
	}

	// Validate updated storage class matches pool2
	mockStoreClient.EXPECT().GetStorageClass(gomock.Any(), gomock.Any()).Return(
		orchestrator.storageClasses["initial-storage-class"].ConstructPersistent(), nil)

	validateStorageClass(t, orchestrator, updatedStorageClassConfig.Name, []*tu.PoolMatch{
		{Backend: "backendWithTwoPools", Pool: tu.SlowSnapshots},
	})
}

func TestUpdateStorageClassWithBackends_PersistentStoreError(t *testing.T) {
	// Ensure backend-to-storageclass mapping is not updated if persistent store update fails
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	orchestrator, err := NewTridentOrchestrator(mockStoreClient)
	orchestrator.bootstrapped = true
	orchestrator.bootstrapError = nil

	mockStoreClient.EXPECT().AddBackend(gomock.Any(), gomock.Any()).Return(nil)
	mockStoreClient.EXPECT().AddStorageClass(gomock.Any(), gomock.Any()).Return(nil)
	mockStoreClient.EXPECT().UpdateStorageClass(gomock.Any(), gomock.Any()).Return(errors.New("failed"))

	// Add backend with two pools
	mockPools := tu.GetFakePools()
	pools := map[string]*fake.StoragePool{
		"fast-small":     mockPools[tu.FastSmall],
		"slow-snapshots": mockPools[tu.SlowSnapshots],
	}
	cfg, err := fakedriver.NewFakeStorageDriverConfigJSON("backendWithTwoPools", config.File, pools, make([]fake.Volume, 0))
	if err != nil {
		t.Fatalf("Unable to generate cfg JSON: %v", err)
	}
	_, err = orchestrator.AddBackend(ctx(), cfg, "")
	if err != nil {
		t.Fatalf("Unable to add backend: %v", err)
	}

	// Add initial storage class
	initialStorageClassConfig := &storageclass.Config{
		Name: "initial-storage-class",
		Attributes: map[string]sa.Request{
			sa.IOPS: sa.NewIntRequest(2000),
		},
	}
	_, err = orchestrator.AddStorageClass(ctx(), initialStorageClassConfig)
	if err != nil {
		t.Fatalf("Unable to add initial storage class: %v", err)
	}

	// Validate initial storage class matches pool1
	mockStoreClient.EXPECT().GetStorageClass(gomock.Any(), gomock.Any()).Return(
		orchestrator.storageClasses["initial-storage-class"].ConstructPersistent(), nil)

	validateStorageClass(t, orchestrator, initialStorageClassConfig.Name, []*tu.PoolMatch{
		{Backend: "backendWithTwoPools", Pool: tu.FastSmall},
	})

	// Update storage class to match pool2
	updatedStorageClassConfig := &storageclass.Config{
		Name: "initial-storage-class",
		Attributes: map[string]sa.Request{
			sa.IOPS: sa.NewIntRequest(40),
		},
	}
	_, err = orchestrator.UpdateStorageClass(ctx(), updatedStorageClassConfig)
	assert.Error(t, err, "UpdateStorageClass should have failed")

	// Validate storage class still matches pool1
	mockStoreClient.EXPECT().GetStorageClass(gomock.Any(), gomock.Any()).Return(
		orchestrator.storageClasses["initial-storage-class"].ConstructPersistent(), nil)

	validateStorageClass(t, orchestrator, updatedStorageClassConfig.Name, []*tu.PoolMatch{
		{Backend: "backendWithTwoPools", Pool: tu.FastSmall},
	})
}

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
	orchestrator := getOrchestrator(t, false)
	// Add the mocked backend to the orchestrator
	orchestrator.backends[backendUUID] = mockBackend

	// Run the test
	actualBackendExternal, err := orchestrator.GetBackend(ctx(), backendName)

	// Verify the results
	assert.Nilf(t, err, "Error getting backend; %v", err)
	assert.Equal(t, expectedBackendExternal, actualBackendExternal, "Did not get the expected backend object")
}

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
	orchestrator := getOrchestrator(t, false)
	// Add the mocked backend to the orchestrator
	orchestrator.backends[backendUUID] = mockBackend

	// Run the test
	actualBackendExternal, err := orchestrator.GetBackendByBackendUUID(ctx(), backendUUID)

	// Verify the results
	assert.Nilf(t, err, "Error getting backend; %v", err)
	assert.Equal(t, expectedBackendExternal, actualBackendExternal, "Did not get the expected backend object")
}

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
	orchestrator := getOrchestrator(t, false)
	// Add the mocked backends to the orchestrator
	orchestrator.backends[expectedBackendExternal1.BackendUUID] = mockBackend1
	orchestrator.backends[expectedBackendExternal2.BackendUUID] = mockBackend2

	// Perform the test
	actualBackendList, err := orchestrator.ListBackends(ctx())

	// Verify the results
	assert.Nilf(t, err, "Error listing backends; %v", err)
	assert.ElementsMatch(t, expectedBackendList, actualBackendList, "Did not get expected list of backends")
}

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
	orchestrator := getOrchestrator(t, false)
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

func TestPublishVolumeFailedToUpdatePersistentStore(t *testing.T) {
	config.CurrentDriverContext = config.ContextCSI
	defer func() { config.CurrentDriverContext = "" }()

	// Boilerplate mocking code
	mockCtrl := gomock.NewController(t)

	// Set fake values
	backendUUID := "1234"
	expectedError := fmt.Errorf("failure")

	// Create mocked backend that returns the expected object
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockBackend.EXPECT().CanEnablePublishEnforcement().Return(false)

	// Create a mocked persistent store client
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	mockStoreClient.EXPECT().AddVolumePublication(gomock.Any(), gomock.Any()).Return(expectedError)

	// Create an instance of the orchestrator
	orchestrator := getOrchestrator(t, false)
	orchestrator.storeClient = mockStoreClient
	// Add the mocked backend to the orchestrator
	orchestrator.backends[backendUUID] = mockBackend
	volConfig := tu.GenerateVolumeConfig("fake-volume", 1, "fast", config.File)
	orchestrator.volumes["fake-volume"] = &storage.Volume{BackendUUID: backendUUID, Config: volConfig}

	// Run the test
	err := orchestrator.PublishVolume(ctx(), "fake-volume", &models.VolumePublishInfo{})
	assert.Error(t, err, "Unexpected success publishing volume.")
}

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
	expectedChapInfo := &models.IscsiChapInfo{
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
	orchestrator := getOrchestrator(t, false)
	// Add the mocked backend and fake volume to the orchestrator
	orchestrator.backends[backendUUID] = mockBackend
	orchestrator.volumes[volumeName] = volume
	actualChapInfo, err := orchestrator.GetCHAP(ctx(), volumeName, nodeName)
	assert.Nil(t, err, "Unexpected error")
	assert.Equal(t, expectedChapInfo, actualChapInfo, "Unexpected chap info returned.")
}

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
	orchestrator := getOrchestrator(t, false)
	// Add the mocked backend and fake volume to the orchestrator
	orchestrator.backends[backendUUID] = mockBackend
	orchestrator.volumes[volumeName] = volume
	actualChapInfo, actualErr := orchestrator.GetCHAP(ctx(), volumeName, nodeName)
	assert.Nil(t, actualChapInfo, "Unexpected CHAP info")
	assert.Equal(t, expectedError, actualErr, "Unexpected error")
}

func TestPublishVolume(t *testing.T) {
	var (
		backendUUID        = "1234"
		nodeName           = "foo"
		volumeName         = "bar"
		subordinateVolName = "subvol"
		volume             = &storage.Volume{
			BackendUUID: backendUUID,
			Config:      &storage.VolumeConfig{AccessInfo: models.VolumeAccessInfo{}},
		}
		subordinatevolume = &storage.Volume{
			BackendUUID: backendUUID,
			Config:      &storage.VolumeConfig{ShareSourceVolume: volumeName},
		}
		node = &models.Node{Deleted: false, PublicationState: models.NodeClean}
	)
	tt := []struct {
		name               string
		volumeName         string
		shareSourceName    string
		subordinateVolName string
		subordinateVolumes map[string]*storage.Volume
		volumes            map[string]*storage.Volume
		subVolConfig       *storage.VolumeConfig
		nodes              map[string]*models.Node
		pubsSynced         bool
		lastPub            time.Time
		volumeEnforceable  bool
		mocks              func(
			mockBackend *mockstorage.MockBackend,
			mockStoreClient *mockpersistentstore.MockStoreClient, volume *storage.Volume,
		)
		wantErr     assert.ErrorAssertionFunc
		pubTime     assert.ValueAssertionFunc
		pubEnforced assert.BoolAssertionFunc
		synced      assert.BoolAssertionFunc
	}{
		{
			name:              "LegacyVolumePubsNotSyncedNoPublicationsYet",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: node},
			pubsSynced:        false,
			volumeEnforceable: false,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(false)
				mockBackend.EXPECT().ReconcileNodeAccess(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().SetNodeAccessUpToDate()
				mockBackend.EXPECT().PublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(false)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, volume).Return(nil)
			},
			wantErr:     assert.NoError,
			pubTime:     assert.IsIncreasing,
			pubEnforced: assert.False,
			synced:      assert.False,
		},
		{
			name:              "LegacyVolumePubsNotSyncedTooSoon",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: node},
			pubsSynced:        false,
			volumeEnforceable: false,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().ReconcileNodeAccess(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().SetNodeAccessUpToDate()
				mockBackend.EXPECT().PublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(false).Times(2)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, volume).Return(nil)
			},
			wantErr:     assert.NoError,
			pubTime:     assert.IsIncreasing,
			pubEnforced: assert.False,
			synced:      assert.False,
		},
		{
			name:              "LegacyVolumePubsSynced",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: node},
			pubsSynced:        true,
			volumeEnforceable: false,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().EnablePublishEnforcement(coreCtx, volume).DoAndReturn(
					func(ctx context.Context, volume *storage.Volume) error {
						volume.Config.AccessInfo.PublishEnforcement = true
						return nil
					})
				mockBackend.EXPECT().ReconcileNodeAccess(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().SetNodeAccessUpToDate()
				mockBackend.EXPECT().Volumes().Return(map[string]*storage.Volume{volumeName: volume}).Times(2)
				mockBackend.EXPECT().PublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true).Times(2)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, volume).Return(nil)
			},
			wantErr:     assert.NoError,
			pubTime:     assert.IsIncreasing,
			pubEnforced: assert.True,
			synced:      assert.True,
		},
		{
			name:              "EnforcedVolumePubsNotSyncedNoPubsYet",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: node},
			pubsSynced:        false,
			volumeEnforceable: true,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().ReconcileNodeAccess(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().SetNodeAccessUpToDate()
				mockBackend.EXPECT().Volumes().Return(map[string]*storage.Volume{volumeName: volume}).Times(2)
				mockBackend.EXPECT().PublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true).Times(2)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, volume).Return(nil)
			},
			wantErr:     assert.NoError,
			pubTime:     assert.IsIncreasing,
			pubEnforced: assert.True,
			synced:      assert.False,
		},
		{
			name:              "EnforcedVolumePubsNotSyncedTooSoon",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: node},
			pubsSynced:        false,
			volumeEnforceable: true,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().ReconcileNodeAccess(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().SetNodeAccessUpToDate()
				mockBackend.EXPECT().Volumes().Return(map[string]*storage.Volume{volumeName: volume}).Times(2)
				mockBackend.EXPECT().PublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true).Times(2)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, volume).Return(nil)
			},
			wantErr:     assert.NoError,
			pubTime:     assert.IsIncreasing,
			pubEnforced: assert.True,
			synced:      assert.False,
		},
		{
			name:              "EnforcedVolumePubsSynced",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: node},
			pubsSynced:        true,
			volumeEnforceable: true,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().ReconcileNodeAccess(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().SetNodeAccessUpToDate()
				mockBackend.EXPECT().Volumes().Return(map[string]*storage.Volume{volumeName: volume}).Times(2)
				mockBackend.EXPECT().PublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true).Times(2)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, volume).Return(nil)
			},
			wantErr:     assert.NoError,
			pubTime:     assert.IsIncreasing,
			pubEnforced: assert.True,
			synced:      assert.True,
		},
		{
			name:              "VolumeNotFound",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{},
			pubsSynced:        false,
			volumeEnforceable: false,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
			},
			wantErr:     assert.Error,
			pubTime:     assert.IsNonIncreasing,
			pubEnforced: assert.False,
			synced:      assert.False,
		},
		{
			name:              "VolumeIsDeleting",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: node},
			pubsSynced:        false,
			volumeEnforceable: false,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				volume.State = storage.VolumeStateDeleting
			},
			wantErr:     assert.Error,
			pubTime:     assert.IsNonIncreasing,
			pubEnforced: assert.False,
			synced:      assert.False,
		},
		{
			name:              "ErrorEnablingEnforcement",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: node},
			pubsSynced:        true,
			volumeEnforceable: false,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().EnablePublishEnforcement(coreCtx, gomock.Any()).Return(fmt.Errorf("some error"))
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true)
			},
			wantErr:     assert.Error,
			pubTime:     assert.IsIncreasing,
			pubEnforced: assert.False,
			synced:      assert.True,
		},
		{
			name:       "DoesNotErrorButFailsToEnableEnforcementOnUnsupportedBackend",
			volumeName: volumeName,
			volumes:    map[string]*storage.Volume{volumeName: volume},
			nodes:      map[string]*models.Node{nodeName: node},
			pubsSynced: true,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true).Times(2)
				mockBackend.EXPECT().EnablePublishEnforcement(coreCtx, gomock.Any()).Return(
					errors.UnsupportedError("unsupported error"))
				mockBackend.EXPECT().ReconcileNodeAccess(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().SetNodeAccessUpToDate()
				mockBackend.EXPECT().Volumes().Return(map[string]*storage.Volume{volumeName: volume}).Times(2)
				mockBackend.EXPECT().PublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, volume).Return(nil)
			},
			wantErr:     assert.NoError,
			pubTime:     assert.IsIncreasing,
			pubEnforced: assert.False,
			synced:      assert.True,
		},
		{
			name:       "ErrorEnablingEnforcementOnBackend",
			volumeName: volumeName,
			volumes:    map[string]*storage.Volume{volumeName: volume},
			nodes:      map[string]*models.Node{nodeName: node},
			pubsSynced: true,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true)
				mockBackend.EXPECT().EnablePublishEnforcement(coreCtx, gomock.Any()).Return(
					fmt.Errorf("unexpected error"))
			},
			wantErr:     assert.Error,
			pubTime:     assert.IsIncreasing,
			pubEnforced: assert.False,
			synced:      assert.True,
		},
		{
			name:              "ErrorReconcilingNodeAccessEnforcement",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: node},
			pubsSynced:        true,
			volumeEnforceable: false,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().EnablePublishEnforcement(coreCtx, volume).DoAndReturn(
					func(ctx context.Context, volume *storage.Volume) error {
						volume.Config.AccessInfo.PublishEnforcement = true
						return nil
					})
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true).Times(2)
				mockBackend.EXPECT().ReconcileNodeAccess(coreCtx, gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("some error"))
				mockBackend.EXPECT().Volumes().Return(map[string]*storage.Volume{volumeName: volume}).Times(2)
				mockBackend.EXPECT().Name().Return("").AnyTimes()
			},
			wantErr:     assert.Error,
			pubTime:     assert.IsNonIncreasing,
			pubEnforced: assert.True,
			synced:      assert.True,
		},
		{
			name:              "ErrorPublishingVolume",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: node},
			pubsSynced:        true,
			volumeEnforceable: false,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().EnablePublishEnforcement(coreCtx, volume).DoAndReturn(
					func(ctx context.Context, volume *storage.Volume) error {
						volume.Config.AccessInfo.PublishEnforcement = true
						return nil
					})
				mockBackend.EXPECT().ReconcileNodeAccess(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().SetNodeAccessUpToDate()
				mockBackend.EXPECT().Volumes().Return(map[string]*storage.Volume{volumeName: volume}).Times(2)
				mockBackend.EXPECT().PublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(fmt.Errorf("some error"))
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true).Times(2)
			},
			wantErr:     assert.Error,
			pubTime:     assert.IsNonIncreasing,
			pubEnforced: assert.True,
			synced:      assert.True,
		},
		{
			name:              "ErrorUpdatingVolume",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: node},
			pubsSynced:        true,
			volumeEnforceable: false,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().EnablePublishEnforcement(coreCtx, volume).DoAndReturn(
					func(ctx context.Context, volume *storage.Volume) error {
						volume.Config.AccessInfo.PublishEnforcement = true
						return nil
					})
				mockBackend.EXPECT().ReconcileNodeAccess(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().SetNodeAccessUpToDate()
				mockBackend.EXPECT().Volumes().Return(map[string]*storage.Volume{volumeName: volume}).Times(2)
				mockBackend.EXPECT().PublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true).Times(2)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, volume).Return(fmt.Errorf("some error"))
			},
			wantErr:     assert.Error,
			pubTime:     assert.IsNonIncreasing,
			pubEnforced: assert.True,
			synced:      assert.True,
		},
		{
			name:               "SubordinateVolumeTest",
			shareSourceName:    volumeName,
			volumeName:         subordinateVolName,
			subordinateVolumes: map[string]*storage.Volume{subordinateVolName: subordinatevolume},
			volumes:            map[string]*storage.Volume{volumeName: volume},
			subVolConfig:       tu.GenerateVolumeConfig("subvol", 1, "fakeSC", config.File),
			nodes:              map[string]*models.Node{nodeName: node},
			pubsSynced:         false,
			volumeEnforceable:  false,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockStoreClient.EXPECT().AddVolumePublication(coreCtx, gomock.Any()).Return(nil)
				mockBackend.EXPECT().ReconcileNodeAccess(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().SetNodeAccessUpToDate()
				mockBackend.EXPECT().Volumes().Return(map[string]*storage.Volume{volumeName: volume}).Times(2)
				mockBackend.EXPECT().PublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true).Times(2)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, volume).Return(nil)
			},
			wantErr:     assert.NoError,
			pubTime:     assert.IsIncreasing,
			pubEnforced: assert.False,
			synced:      assert.False,
		},
		{
			name:               "SubordinateVolumeTestFail",
			shareSourceName:    volumeName,
			volumeName:         subordinateVolName,
			subordinateVolumes: map[string]*storage.Volume{subordinateVolName: subordinatevolume},
			volumes:            map[string]*storage.Volume{"newsrcvol": volume},
			subVolConfig:       tu.GenerateVolumeConfig("subvol", 1, "fakeSC", config.File),
			nodes:              map[string]*models.Node{nodeName: node},
			pubsSynced:         false,
			volumeEnforceable:  false,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
			},
			wantErr:     assert.Error,
			pubTime:     assert.IsNonIncreasing,
			pubEnforced: assert.False,
			synced:      assert.False,
		},
		{
			name:              "RejectPublicationOnDirtyNode",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: {PublicationState: models.NodeDirty}},
			pubsSynced:        true,
			volumeEnforceable: true,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true)
				mockBackend.EXPECT().GetDriverName().Return("ontap-san")
			},
			wantErr:     assert.Error,
			pubTime:     assert.IsNonIncreasing,
			pubEnforced: assert.True,
			synced:      assert.True,
		},
		{
			name:              "RejectPublicationOnCleanableNode",
			volumeName:        volumeName,
			volumes:           map[string]*storage.Volume{volumeName: volume},
			nodes:             map[string]*models.Node{nodeName: {PublicationState: models.NodeCleanable}},
			pubsSynced:        true,
			volumeEnforceable: true,
			mocks: func(
				mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient,
				volume *storage.Volume,
			) {
				mockBackend.EXPECT().CanEnablePublishEnforcement().Return(true)
				mockBackend.EXPECT().GetDriverName().Return("ontap-san")
			},
			wantErr:     assert.Error,
			pubTime:     assert.IsNonIncreasing,
			pubEnforced: assert.True,
			synced:      assert.True,
		},
	}

	for _, tr := range tt {
		t.Run(tr.name, func(t *testing.T) {
			config.CurrentDriverContext = config.ContextCSI
			defer func() { config.CurrentDriverContext = "" }()
			volume.State = storage.VolumeStateOnline
			// Boilerplate mocking code
			mockCtrl := gomock.NewController(t)

			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

			// Create an instance of the orchestrator
			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.volumes = tr.volumes
			o.subordinateVolumes = tr.subordinateVolumes
			o.volumePublicationsSynced = tr.pubsSynced
			volume.Config.AccessInfo.PublishEnforcement = tr.volumeEnforceable
			subordinatevolume.Config.AccessInfo.PublishEnforcement = tr.volumeEnforceable
			subordinatevolume.Config.ShareSourceVolume = tr.shareSourceName
			tr.mocks(mockBackend, mockStoreClient, volume)
			for name, n := range tr.nodes {
				o.nodes.Set(name, n)
			}

			// Run the test
			err := o.publishVolume(coreCtx, tr.volumeName, &models.VolumePublishInfo{HostName: nodeName})
			if !tr.wantErr(t, err, "Unexpected Result") {
				return
			}
			if !tr.pubEnforced(t, volume.Config.AccessInfo.PublishEnforcement) {
				return
			}
			if !tr.synced(t, o.volumePublicationsSynced) {
				return
			}
		})
	}
}

func TestUnpublishVolume(t *testing.T) {
	var (
		backendUUID     = "1234"
		nodeName        = "foo"
		otherNodeName   = "fiz"
		volumeName      = "bar"
		otherVolumeName = "baz"
		parentSubVols   = map[string]interface{}{"dummy": nil}
		volConfig       = &storage.VolumeConfig{Name: volumeName, SubordinateVolumes: parentSubVols}
		volume          = &storage.Volume{BackendUUID: backendUUID, Config: volConfig}
		subordinateVol  = &storage.Volume{Config: &storage.VolumeConfig{Name: "dummy", ShareSourceVolume: "abc"}}
		volumeNoBackend = &storage.Volume{Config: volConfig}
		node            = &models.Node{Deleted: false}
		deletedNode     = &models.Node{Deleted: true}
		publication     = &models.VolumePublication{
			NodeName:   nodeName,
			VolumeName: volumeName,
		}
		otherVolPublication = &models.VolumePublication{
			NodeName:   nodeName,
			VolumeName: otherVolumeName,
		}
		otherNodePublication = &models.VolumePublication{
			NodeName:   otherNodeName,
			VolumeName: volumeName,
		}
		otherNodeAndVolPublication = &models.VolumePublication{
			NodeName:   otherNodeName,
			VolumeName: otherVolumeName,
		}
	)
	tt := []struct {
		name                   string
		volumeName             string
		nodeName               string
		driverContext          config.DriverContext
		volumes                map[string]*storage.Volume
		nodes                  map[string]*models.Node
		publications           map[string]map[string]*models.VolumePublication
		mocks                  func(mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient)
		wantErr                assert.ErrorAssertionFunc
		publicationShouldExist bool
	}{
		{
			name:          "NoOtherPublications",
			volumeName:    volumeName,
			nodeName:      nodeName,
			driverContext: config.ContextCSI,
			volumes:       map[string]*storage.Volume{volumeName: volume},
			nodes:         map[string]*models.Node{nodeName: node},
			publications:  map[string]map[string]*models.VolumePublication{volumeName: {nodeName: publication}},
			mocks: func(mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend.EXPECT().UnpublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumePublication(coreCtx, gomock.Any()).Return(nil)
			},
			wantErr:                assert.NoError,
			publicationShouldExist: false,
		},
		{
			name:          "OtherPublications",
			volumeName:    volumeName,
			nodeName:      nodeName,
			driverContext: config.ContextCSI,
			volumes:       map[string]*storage.Volume{volumeName: volume},
			nodes:         map[string]*models.Node{nodeName: deletedNode},
			publications: map[string]map[string]*models.VolumePublication{
				volumeName: {
					nodeName: publication,
				},
				otherVolumeName: {
					nodeName: otherVolPublication,
				},
			},
			mocks: func(mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend.EXPECT().UnpublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumePublication(coreCtx, gomock.Any()).Return(nil)
			},
			wantErr:                assert.NoError,
			publicationShouldExist: false,
		},
		{
			name:          "OtherPublicationsDifferentNode",
			volumeName:    volumeName,
			nodeName:      nodeName,
			driverContext: config.ContextCSI,
			volumes:       map[string]*storage.Volume{volumeName: volume},
			nodes: map[string]*models.Node{
				nodeName:      node,
				otherNodeName: node,
			},
			publications: map[string]map[string]*models.VolumePublication{
				volumeName: {
					nodeName:      publication,
					otherNodeName: otherNodePublication,
				},
			},
			mocks: func(mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend.EXPECT().UnpublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumePublication(coreCtx, gomock.Any()).Return(nil)
			},
			wantErr:                assert.NoError,
			publicationShouldExist: false,
		},
		{
			name:          "VolumeNotFound",
			volumeName:    volumeName,
			nodeName:      nodeName,
			driverContext: config.ContextCSI,
			volumes:       map[string]*storage.Volume{},
			nodes:         map[string]*models.Node{nodeName: node},
			publications:  map[string]map[string]*models.VolumePublication{volumeName: {nodeName: publication}},
			mocks: func(mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient) {
			},
			wantErr:                assert.Error,
			publicationShouldExist: true,
		},
		{
			name:          "BackendNotFound",
			volumeName:    volumeName,
			nodeName:      nodeName,
			driverContext: config.ContextCSI,
			volumes:       map[string]*storage.Volume{volumeName: volumeNoBackend},
			nodes:         map[string]*models.Node{nodeName: node},
			publications:  map[string]map[string]*models.VolumePublication{volumeName: {nodeName: publication}},
			mocks: func(mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient) {
			},
			wantErr:                assert.Error,
			publicationShouldExist: true,
		},
		{
			name:          "PublicationNotFound_CSI",
			volumeName:    volumeName,
			nodeName:      nodeName,
			driverContext: config.ContextCSI,
			volumes:       map[string]*storage.Volume{volumeName: volume},
			nodes:         map[string]*models.Node{nodeName: node},
			publications:  map[string]map[string]*models.VolumePublication{},
			mocks: func(mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// There is no publication, so there is nothing to unpublish, so no calls should be made
			},
			wantErr:                assert.NoError,
			publicationShouldExist: false,
		},
		{
			name:          "PublicationNotFound_Docker",
			volumeName:    volumeName,
			nodeName:      nodeName,
			driverContext: config.ContextDocker,
			volumes:       map[string]*storage.Volume{volumeName: volume},
			nodes:         map[string]*models.Node{nodeName: node},
			publications:  map[string]map[string]*models.VolumePublication{},
			mocks: func(mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient) {
				// There is no publication, but there might still be a volume published, so call it anyway
				mockBackend.EXPECT().UnpublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, gomock.Any()).Return(nil)
			},
			wantErr:                assert.NoError,
			publicationShouldExist: false,
		},
		{
			name:          "BackendUnpublishError",
			volumeName:    volumeName,
			nodeName:      nodeName,
			driverContext: config.ContextCSI,
			volumes:       map[string]*storage.Volume{volumeName: volume},
			nodes:         map[string]*models.Node{nodeName: node},
			publications:  map[string]map[string]*models.VolumePublication{volumeName: {nodeName: publication}},
			mocks: func(mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend.EXPECT().UnpublishVolume(coreCtx, gomock.Any(),
					gomock.Any()).Return(fmt.Errorf("some error"))
			},
			wantErr:                assert.Error,
			publicationShouldExist: true,
		},
		{
			name:          "SubordinateVolumeParentNotFound",
			volumeName:    volumeName,
			nodeName:      nodeName,
			driverContext: config.ContextCSI,
			volumes:       map[string]*storage.Volume{volumeName: volume},
			nodes:         map[string]*models.Node{nodeName: node},
			publications:  map[string]map[string]*models.VolumePublication{volumeName: {nodeName: publication}},
			mocks: func(mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient) {
			},
			wantErr:                assert.Error,
			publicationShouldExist: true,
		},
		{
			name:          "NodeNotFoundWarning",
			volumeName:    volumeName,
			nodeName:      nodeName,
			driverContext: config.ContextCSI,
			volumes:       map[string]*storage.Volume{volumeName: volume},
			nodes:         map[string]*models.Node{nodeName: node},
			publications:  map[string]map[string]*models.VolumePublication{volumeName: {nodeName: publication}, "dummy": {"dummy": otherNodeAndVolPublication}},
			mocks: func(mockBackend *mockstorage.MockBackend, mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockBackend.EXPECT().UnpublishVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().DeleteVolumePublication(coreCtx,
					gomock.Any()).Return(errors.New("failed to delete"))
			},
			wantErr:                assert.Error,
			publicationShouldExist: true,
		},
	}

	for _, tr := range tt {
		t.Run(tr.name, func(t *testing.T) {
			config.CurrentDriverContext = tr.driverContext
			defer func() { config.CurrentDriverContext = "" }()
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			// Create a fake backend with UUID
			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

			// Create orchestrator with fake backend and initial values
			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend

			o.volumes = tr.volumes
			for name, n := range tr.nodes {
				o.nodes.Set(name, n)
			}
			if len(tr.publications) != 0 {
				o.volumePublications.SetMap(tr.publications)
			}
			o.subordinateVolumes["dummy"] = subordinateVol
			if tr.name == "SubordinateVolumeParentNotFound" {
				o.subordinateVolumes[tr.volumeName] = subordinateVol
			}

			tr.mocks(mockBackend, mockStoreClient)

			err := o.unpublishVolume(coreCtx, tr.volumeName, tr.nodeName)
			if !tr.wantErr(t, err, "Unexpected Result") {
				return
			}

			pub, ok := o.volumePublications.TryGet(volumeName, nodeName)
			if tr.publicationShouldExist {
				assert.True(t, ok, "expected publication to exist")
				assert.NotNil(t, pub, "expected publication to exist")
			} else {
				assert.False(t, ok, "expected publication to no longer exist")
				assert.Nil(t, pub, "expected publication to no longer exist")
			}
		})
	}

	// Tests for Public Unpublish Volume
	config.CurrentDriverContext = config.ContextDocker
	defer func() { config.CurrentDriverContext = "" }()
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

	// Create orchestrator with fake backend and initial values
	o := getOrchestrator(t, false)
	o.storeClient = mockStoreClient
	o.backends[backendUUID] = mockBackend

	o.bootstrapError = errors.New("bootstrap error")
	err := o.UnpublishVolume(ctx(), volumeName, nodeName)
	assert.Error(t, err, "bootstrap error")

	o.bootstrapError = nil
	err = o.UnpublishVolume(ctx(), volumeName, nodeName)
	assert.Error(t, err, "volume not found")
}

func TestBootstrapSubordinateVolumes(t *testing.T) {
	var (
		backendUUID      = "1234"
		subVolumeName    = "sub_abc"
		sourceVolumeName = "source_abc"
		sourceVolConfig  = &storage.VolumeConfig{Name: sourceVolumeName}
		sourceVolume     = &storage.Volume{Config: sourceVolConfig}
		subVolConfig     = &storage.VolumeConfig{Name: subVolumeName, ShareSourceVolume: sourceVolumeName}
		subVolume        = &storage.Volume{Config: subVolConfig}
		subvolconfigFail = &storage.VolumeConfig{Name: subVolumeName}
		subvolumeFail    = &storage.Volume{Config: subvolconfigFail}
	)

	tests := []struct {
		name               string
		sourceVolumeName   string
		subVolumeName      string
		volumes            map[string]*storage.Volume
		subordinateVolumes map[string]*storage.Volume
		wantErr            assert.ErrorAssertionFunc
	}{
		{
			name:               "BootStrapSubordinateVolumes",
			sourceVolumeName:   sourceVolumeName,
			subVolumeName:      subVolumeName,
			volumes:            map[string]*storage.Volume{sourceVolumeName: sourceVolume},
			subordinateVolumes: map[string]*storage.Volume{subVolumeName: subVolume},
			wantErr:            assert.NoError,
		},
		{
			name:               "BootStrapSubordinateVolumesNotFound",
			sourceVolumeName:   sourceVolumeName,
			subVolumeName:      subVolumeName,
			volumes:            map[string]*storage.Volume{sourceVolumeName: sourceVolume},
			subordinateVolumes: map[string]*storage.Volume{subVolumeName: subvolumeFail},
			wantErr:            assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.CurrentDriverContext = config.ContextCSI
			defer func() { config.CurrentDriverContext = "" }()
			mockCtrl := gomock.NewController(t)

			// Create a fake backend with UUID
			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

			// Create orchestrator with fake backend and initial values
			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.subordinateVolumes = tt.subordinateVolumes
			o.volumes = tt.volumes

			err := o.bootstrapSubordinateVolumes(ctx())
			if !tt.wantErr(t, err, "Unexpected Result") {
				return
			}
		})
	}
}

func TestAddSubordinateVolume(t *testing.T) {
	backendUUID := "1234"
	tests := []struct {
		name                  string
		sourceVolumeName      string
		subVolumeName         string
		shareSourceName       string
		ImportNotManaged      bool
		Orphaned              bool
		IsMirrorDestination   bool
		SourceVolStorageClass string
		SubVolStorageClass    string
		NFSPath               string
		backendId             string
		CloneSourceVolume     string
		ImportOriginalName    string
		State                 storage.VolumeState
		sourceVolConfig       *storage.VolumeConfig
		subVolConfig          *storage.VolumeConfig
		AddVolErr             error
		wantErr               assert.ErrorAssertionFunc
		wantVolumes           *storage.VolumeExternal
	}{
		{
			name:             "AddSubordinateVolumesNotRegularVolume",
			sourceVolumeName: "fake_vol",
			subVolumeName:    "fake_vol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fake_vol", 1, "fakeSC", config.File),
			subVolConfig:     tu.GenerateVolumeConfig("fake_vol", 1, "fakeSC", config.File),
			wantErr:          assert.Error,
		},
		{
			name:             "AddSubordinateVolumesNotAlreadySubordinate",
			sourceVolumeName: "fake_vol",
			subVolumeName:    "fake_sub_vol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:     tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			wantErr:          assert.Error,
		},
		{
			name:             "AddSubordinateVolumesSourceDoesNotExists",
			sourceVolumeName: "fake_vol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:     tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			wantErr:          assert.Error,
		},
		{
			name:             "AddSubordinateVolumesImportNotManaged",
			sourceVolumeName: "fake_vol",
			shareSourceName:  "fake_vol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:     tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			ImportNotManaged: true,
			wantErr:          assert.Error,
		},
		{
			name:             "AddSubordinateVolumesOrphaned",
			sourceVolumeName: "fake_vol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:     tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			shareSourceName:  "fake_vol",
			Orphaned:         true,
			wantErr:          assert.Error,
		},
		{
			name:             "AddSubordinateVolumesStateNotOnline",
			sourceVolumeName: "fake_vol",
			shareSourceName:  "fake_vol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:     tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			State:            storage.VolumeStateDeleting,
			wantErr:          assert.Error,
		},
		{
			name:                  "AddSubordinateVolumesSCNotSame",
			sourceVolumeName:      "fake_vol",
			shareSourceName:       "fake_vol",
			sourceVolConfig:       tu.GenerateVolumeConfig("fake_Source_vol", 1, "sourceSC", config.File),
			subVolConfig:          tu.GenerateVolumeConfig("fake_sub_vol", 1, "subSC", config.File),
			State:                 storage.VolumeStateOnline,
			SourceVolStorageClass: "sourceSC",
			SubVolStorageClass:    "subSC",
			wantErr:               assert.Error,
		},
		{
			name:                  "AddSubordinateVolumesNoBackendFound",
			sourceVolumeName:      "fake_vol",
			shareSourceName:       "fake_vol",
			sourceVolConfig:       tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:          tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			State:                 storage.VolumeStateOnline,
			SourceVolStorageClass: "fakeSC",
			SubVolStorageClass:    "fakeSC",
			backendId:             "fakebackend",
			wantErr:               assert.Error,
		},
		{
			name:                  "AddSubordinateVolumesSourceNotNFS",
			sourceVolumeName:      "fake_vol",
			shareSourceName:       "fake_vol",
			sourceVolConfig:       tu.GenerateVolumeConfig("fake_vol", 1, "fakeSC", config.File),
			subVolConfig:          tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			State:                 storage.VolumeStateOnline,
			SourceVolStorageClass: "fakeSC",
			SubVolStorageClass:    "fakeSC",
			backendId:             backendUUID,
			NFSPath:               "",
			wantErr:               assert.Error,
		},
		{
			name:                  "AddSubordinateVolumesSubVolSizeInvalid",
			sourceVolumeName:      "fake_vol",
			shareSourceName:       "fake_vol",
			sourceVolConfig:       tu.GenerateVolumeConfig("fake_vol", 1, "fakeSC", config.File),
			subVolConfig:          tu.GenerateVolumeConfig("fake_sub_vol", 99999999999999, "fakeSC", config.File),
			State:                 storage.VolumeStateOnline,
			NFSPath:               "fakepath",
			SourceVolStorageClass: "fakeSC",
			SubVolStorageClass:    "fakeSC",
			backendId:             backendUUID,
			wantErr:               assert.Error,
		},
		{
			name:                  "AddSubordinateVolumesSrcVolSizeInvalid",
			sourceVolumeName:      "fake_vol",
			shareSourceName:       "fake_vol",
			sourceVolConfig:       tu.GenerateVolumeConfig("fake_vol", 999999999999999, "fakeSC", config.File),
			subVolConfig:          tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			State:                 storage.VolumeStateOnline,
			NFSPath:               "fakepath",
			SourceVolStorageClass: "fakeSC",
			SubVolStorageClass:    "fakeSC",
			backendId:             backendUUID,
			wantErr:               assert.Error,
		},
		{
			name:                  "AddSubordinateVolumesSubVolSizeLarger",
			sourceVolumeName:      "fake_vol",
			shareSourceName:       "fake_vol",
			sourceVolConfig:       tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:          tu.GenerateVolumeConfig("fake_sub_vol", 10, "fakeSC", config.File),
			State:                 storage.VolumeStateOnline,
			NFSPath:               "fakepath",
			SourceVolStorageClass: "fakeSC",
			SubVolStorageClass:    "fakeSC",
			backendId:             backendUUID,
			wantErr:               assert.Error,
		},
		{
			name:                  "AddSubordinateVolumesCloneTest",
			sourceVolumeName:      "fake_vol",
			shareSourceName:       "fake_vol",
			sourceVolConfig:       tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:          tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			State:                 storage.VolumeStateOnline,
			NFSPath:               "fakepath",
			SourceVolStorageClass: "fakeSC",
			SubVolStorageClass:    "fakeSC",
			CloneSourceVolume:     "fakeclone",
			backendId:             backendUUID,
			wantErr:               assert.Error,
		},
		{
			name:                  "AddSubordinateVolumesMirrorTest",
			sourceVolumeName:      "fake_vol",
			shareSourceName:       "fake_vol",
			sourceVolConfig:       tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:          tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			State:                 storage.VolumeStateOnline,
			NFSPath:               "fakepath",
			IsMirrorDestination:   true,
			SourceVolStorageClass: "fakeSC",
			SubVolStorageClass:    "fakeSC",
			CloneSourceVolume:     "",
			backendId:             backendUUID,
			wantErr:               assert.Error,
		},
		{
			name:                  "AddSubordinateVolumesImportNameTest",
			sourceVolumeName:      "fake_vol",
			shareSourceName:       "fake_vol",
			sourceVolConfig:       tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:          tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			State:                 storage.VolumeStateOnline,
			NFSPath:               "fakepath",
			IsMirrorDestination:   false,
			ImportOriginalName:    "fakeImportName",
			SourceVolStorageClass: "fakeSC",
			SubVolStorageClass:    "fakeSC",
			CloneSourceVolume:     "",
			backendId:             backendUUID,
			wantErr:               assert.Error,
		},
		{
			name:                  "AddSubordinateVolumes",
			sourceVolumeName:      "fake_vol",
			shareSourceName:       "fake_vol",
			sourceVolConfig:       tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:          tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			State:                 storage.VolumeStateOnline,
			NFSPath:               "fakepath",
			IsMirrorDestination:   false,
			ImportOriginalName:    "",
			SourceVolStorageClass: "fakeSC",
			SubVolStorageClass:    "fakeSC",
			CloneSourceVolume:     "",
			backendId:             backendUUID,
			AddVolErr:             nil,
			wantErr:               assert.NoError,
		},
		{
			name:                  "AddSubordinateVolumesFail",
			sourceVolumeName:      "fake_vol",
			shareSourceName:       "fake_vol",
			sourceVolConfig:       tu.GenerateVolumeConfig("fake_Source_vol", 1, "fakeSC", config.File),
			subVolConfig:          tu.GenerateVolumeConfig("fake_sub_vol", 1, "fakeSC", config.File),
			State:                 storage.VolumeStateOnline,
			NFSPath:               "fakepath",
			IsMirrorDestination:   false,
			ImportOriginalName:    "",
			SourceVolStorageClass: "fakeSC",
			SubVolStorageClass:    "fakeSC",
			CloneSourceVolume:     "",
			backendId:             backendUUID,
			wantErr:               assert.Error,
			AddVolErr:             errors.New("failed to add volume"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.CurrentDriverContext = config.ContextCSI
			defer func() { config.CurrentDriverContext = "" }()
			mockCtrl := gomock.NewController(t)

			// Create a fake backend with UUID
			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
			mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			mockStoreClient.EXPECT().AddVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(tt.AddVolErr).AnyTimes()

			sourceVolume := &storage.Volume{Config: tt.sourceVolConfig}
			subVolume := &storage.Volume{Config: tt.subVolConfig}
			tt.subVolConfig.ShareSourceVolume = tt.shareSourceName
			tt.sourceVolConfig.ImportNotManaged = tt.ImportNotManaged
			tt.sourceVolConfig.StorageClass = tt.SourceVolStorageClass
			tt.sourceVolConfig.AccessInfo.NfsPath = tt.NFSPath
			tt.subVolConfig.StorageClass = tt.SubVolStorageClass
			tt.subVolConfig.CloneSourceVolume = tt.CloneSourceVolume
			tt.subVolConfig.IsMirrorDestination = tt.IsMirrorDestination
			tt.subVolConfig.ImportOriginalName = tt.ImportOriginalName
			sourceVolume.Orphaned = tt.Orphaned
			sourceVolume.State = tt.State
			subVolume.State = storage.VolumeStateSubordinate
			subVolume.BackendUUID = tt.backendId
			sourceVolume.BackendUUID = tt.backendId

			volumes := map[string]*storage.Volume{tt.sourceVolumeName: sourceVolume}
			subordinateVolumes := map[string]*storage.Volume{tt.subVolumeName: subVolume}

			// Create orchestrator with fake backend and initial values
			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.subordinateVolumes = subordinateVolumes
			o.volumes = volumes

			gotVolumes, err := o.addSubordinateVolume(coreCtx, tt.subVolConfig)
			if err == nil {
				tt.wantVolumes = subVolume.ConstructExternal()
			} else {
				tt.wantVolumes = nil
			}
			if !tt.wantErr(t, err, "Unexpected Result") {
				return
			}
			if !reflect.DeepEqual(gotVolumes, tt.wantVolumes) {
				t.Errorf("TridentOrchestrator.ListSubordinateVolumes() = %v, expected %v", gotVolumes, tt.wantVolumes)
			}
		})
	}
}

func TestListSubordinateVolumes(t *testing.T) {
	backendUUID := "1234"
	tests := []struct {
		name                string
		sourceVolumeName    string
		sourceVolConfigName string
		wrongSrcVolName     string
		subVolumeName       string
		bootstrapError      error
		sourceVolConfig     *storage.VolumeConfig
		subVolConfig        *storage.VolumeConfig
		SubordinateVolumes  map[string]interface{}
		wantErr             assert.ErrorAssertionFunc
	}{
		{
			name:                "ListSubordinateVolumesError",
			sourceVolumeName:    "fakeSrcVol",
			sourceVolConfigName: "fakeSrcVol",
			subVolumeName:       "fakeSubordinateVol",
			sourceVolConfig:     tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subVolConfig:        tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:      fmt.Errorf("fake error"),
			wantErr:             assert.Error,
		},
		{
			name:                "ListSubordinateVolumesNoError",
			sourceVolumeName:    "fakeSrcVol",
			sourceVolConfigName: "fakeSrcVol",
			subVolumeName:       "fakeSubordinateVol",
			sourceVolConfig:     tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subVolConfig:        tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:      nil,
			wantErr:             assert.NoError,
		},
		{
			name:                "ListSubordinateVolumesNoVolPassed",
			sourceVolumeName:    "",
			sourceVolConfigName: "",
			subVolumeName:       "fakeSubordinateVol",
			sourceVolConfig:     tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subVolConfig:        tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:      nil,
			wantErr:             assert.NoError,
		},
		{
			name:                "ListSubordinateVolumesWrongVolPassed",
			sourceVolumeName:    "fakeSrcVol",
			sourceVolConfigName: "WrongfakeSrcVol",
			wrongSrcVolName:     "fakeWrongName",
			subVolumeName:       "fakeSubordinateVol",
			sourceVolConfig:     tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subVolConfig:        tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:      nil,
			wantErr:             assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.CurrentDriverContext = config.ContextCSI
			defer func() { config.CurrentDriverContext = "" }()
			mockCtrl := gomock.NewController(t)

			// Create a fake backend with UUID
			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
			mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

			sourceVolume := &storage.Volume{Config: tt.sourceVolConfig}
			subVolume := &storage.Volume{Config: tt.subVolConfig}
			tt.sourceVolConfig.SubordinateVolumes = make(map[string]interface{})
			sourceVolume.Config.SubordinateVolumes[tt.subVolumeName] = nil
			volumes := map[string]*storage.Volume{tt.sourceVolumeName: sourceVolume}
			subordinateVolumes := map[string]*storage.Volume{tt.subVolumeName: subVolume}
			wantVolumes := make([]*storage.VolumeExternal, 0, len(volumes))

			// Create orchestrator with fake backend and initial values
			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.subordinateVolumes = subordinateVolumes
			o.volumes = volumes
			o.bootstrapError = tt.bootstrapError

			gotVolumes, err := o.ListSubordinateVolumes(ctx(), tt.sourceVolConfigName)
			if err == nil {
				wantVolumes = append(wantVolumes, subVolume.ConstructExternal())
			} else {
				wantVolumes = nil
			}
			if !tt.wantErr(t, err, "Unexpected Result") {
				return
			}
			if !reflect.DeepEqual(gotVolumes, wantVolumes) {
				t.Errorf("TridentOrchestrator.ListSubordinateVolumes() = %v, expected %v", gotVolumes, wantVolumes)
			}
		})
	}
}

func TestGetSubordinateSourceVolume(t *testing.T) {
	backendUUID := "1234"
	tests := []struct {
		name                string
		sourceVolumeName    string
		sourceVolConfigName string
		subordVolumeName    string
		bootstrapError      error
		sourceVolConfig     *storage.VolumeConfig
		subordVolConfig     *storage.VolumeConfig
		wantErr             assert.ErrorAssertionFunc
	}{
		{
			name:                "FindParentVolumeNoError",
			sourceVolumeName:    "fakeSrcVol",
			sourceVolConfigName: "fakeSrcVol",
			subordVolumeName:    "fakeSubordinateVol",
			sourceVolConfig:     tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subordVolConfig:     tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:      nil,
			wantErr:             assert.NoError,
		},
		{
			name:                "FindParentVolumeError",
			sourceVolumeName:    "fakeSrcVol",
			sourceVolConfigName: "fakeSrcVol",
			subordVolumeName:    "fakeSubordinateVol",
			sourceVolConfig:     tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subordVolConfig:     tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:      fmt.Errorf("fake error"),
			wantErr:             assert.Error,
		},
		{
			name:                "FindParentVolumeError_ParentVolumeNotFound",
			sourceVolumeName:    "fakeSrcVol1",
			sourceVolConfigName: "fakeSrcVol",
			subordVolumeName:    "fakeSubordinateVol",
			sourceVolConfig:     tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subordVolConfig:     tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:      nil,
			wantErr:             assert.Error,
		},
		{
			name:                "FindParentVolumeError_SubordinateVolumeNotFound",
			sourceVolumeName:    "fakeSrcVol1",
			sourceVolConfigName: "fakeSrcVol",
			subordVolumeName:    "fakeSubordinateVol1",
			sourceVolConfig:     tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subordVolConfig:     tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:      nil,
			wantErr:             assert.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.CurrentDriverContext = config.ContextCSI
			defer func() { config.CurrentDriverContext = "" }()
			mockCtrl := gomock.NewController(t)

			// Create a fake backend with UUID
			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
			mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

			sourceVolume := &storage.Volume{Config: tt.sourceVolConfig}
			subordVolume := &storage.Volume{Config: tt.subordVolConfig}
			subordVolume.Config.ShareSourceVolume = sourceVolume.Config.Name
			tt.sourceVolConfig.SubordinateVolumes = make(map[string]interface{})
			volumes := map[string]*storage.Volume{tt.sourceVolumeName: sourceVolume}
			subordinateVolumes := map[string]*storage.Volume{tt.subordVolumeName: subordVolume}
			if tt.subordVolumeName == "fakeSubordinateVol1" {
				subordinateVolumes = map[string]*storage.Volume{}
			}
			var wantVolume *storage.VolumeExternal

			// Create orchestrator with fake backend and initial values
			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.subordinateVolumes = subordinateVolumes
			o.volumes = volumes
			o.bootstrapError = tt.bootstrapError

			gotVolume, err := o.GetSubordinateSourceVolume(ctx(), tt.subordVolumeName)
			if err == nil {
				wantVolume = sourceVolume.ConstructExternal()
			} else {
				wantVolume = nil
			}
			if !tt.wantErr(t, err, "Unexpected Result") {
				return
			}
			if !reflect.DeepEqual(gotVolume, wantVolume) {
				t.Errorf("TridentOrchestrator.FindParentVolume() = %v, expected %v", gotVolume, wantVolume)
			}
		})
	}
}

func TestListVolumePublicationsForVolumeAndSubordinates(t *testing.T) {
	backendUUID := "1234"
	tests := []struct {
		name                   string
		sourceVolumeName       string
		listVolName            string
		subVolumeName          string
		subVolumeState         storage.VolumeState
		bootstrapError         error
		sourceVolConfig        *storage.VolumeConfig
		subVolConfig           *storage.VolumeConfig
		SubordinateVolumes     map[string]interface{}
		wantVolumePublications []*models.VolumePublication
	}{
		{
			name:                   "TestListVolumePublicationsForVolumeAndSubordinatesWrongVolume",
			sourceVolumeName:       "fakeSrcVol",
			listVolName:            "WrongSrcVolName",
			subVolumeName:          "fakeSubordinateVol",
			subVolumeState:         storage.VolumeStateSubordinate,
			sourceVolConfig:        tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subVolConfig:           tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			wantVolumePublications: []*models.VolumePublication{},
		},
		{
			name:                   "TestListVolumePublicationsForVolumeAndSubordinatesSubVol",
			sourceVolumeName:       "fakeSrcVol",
			listVolName:            "fakeSubordinateVol",
			subVolumeName:          "fakeSubordinateVol",
			subVolumeState:         storage.VolumeStateSubordinate,
			sourceVolConfig:        tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subVolConfig:           tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			wantVolumePublications: []*models.VolumePublication{},
		},
		{
			name:             "TestListVolumePublicationsForVolumeAndSubordinates",
			sourceVolumeName: "fakeSrcVol",
			listVolName:      "fakeSrcVol",
			subVolumeName:    "fakeSubordinateVol",
			subVolumeState:   storage.VolumeStateSubordinate,
			sourceVolConfig:  tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subVolConfig:     tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			wantVolumePublications: []*models.VolumePublication{
				{
					Name:       "foo1/bar1",
					NodeName:   "bar1",
					VolumeName: "fakeSrcVol",
					ReadOnly:   true,
					AccessMode: 1,
				},
				{
					Name:       "foo2/bar2",
					NodeName:   "bar2",
					VolumeName: "fakeSubordinateVol",
					ReadOnly:   true,
					AccessMode: 1,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.CurrentDriverContext = config.ContextCSI
			defer func() { config.CurrentDriverContext = "" }()
			mockCtrl := gomock.NewController(t)

			// Create a fake backend with UUID
			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
			mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

			// create source and subordinate volumes
			sourceVolume := &storage.Volume{Config: tt.sourceVolConfig}
			subVolume := &storage.Volume{Config: tt.subVolConfig}
			subVolume.State = tt.subVolumeState
			tt.sourceVolConfig.SubordinateVolumes = make(map[string]interface{})
			sourceVolume.Config.SubordinateVolumes[tt.subVolumeName] = nil
			volumes := map[string]*storage.Volume{tt.sourceVolumeName: sourceVolume}
			subordinateVolumes := map[string]*storage.Volume{tt.subVolumeName: subVolume}

			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.subordinateVolumes = subordinateVolumes
			o.volumes = volumes

			// add the fake publications to orchestrator
			if len(tt.wantVolumePublications) > 0 {
				_ = o.volumePublications.Set(tt.wantVolumePublications[0].VolumeName,
					tt.wantVolumePublications[0].NodeName, tt.wantVolumePublications[0])
				_ = o.volumePublications.Set(tt.wantVolumePublications[1].VolumeName,
					tt.wantVolumePublications[1].NodeName, tt.wantVolumePublications[1])
			}

			gotVolumesPublications := o.listVolumePublicationsForVolumeAndSubordinates(coreCtx, tt.listVolName)

			if !reflect.DeepEqual(gotVolumesPublications, tt.wantVolumePublications) {
				t.Errorf("gotVolumesPublications = %v, expected %v", gotVolumesPublications, tt.wantVolumePublications)
			}
		})
	}
}

func TestAddVolumeWithSubordinateVolume(t *testing.T) {
	const (
		scName         = "fakeSC"
		fullVolumeName = "fakeVolume"
	)
	var wantVolume *storage.VolumeExternal
	var wantErr assert.ErrorAssertionFunc
	wantErr = assert.Error
	orchestrator := getOrchestrator(t, false)

	fullVolumeConfig := tu.GenerateVolumeConfig(
		fullVolumeName, 50, scName,
		config.File,
	)

	fullVolumeConfig.ShareSourceVolume = "fakeSourceVolume"
	Volume := &storage.Volume{Config: fullVolumeConfig}
	gotVolume, err := orchestrator.AddVolume(ctx(), fullVolumeConfig)

	if err == nil {
		wantVolume = Volume.ConstructExternal()
	} else {
		wantVolume = nil
	}
	if !wantErr(t, err, "Unexpected Result") {
		return
	}
	if !reflect.DeepEqual(gotVolume, wantVolume) {
		t.Errorf("GotVolume = %v, expected %v", gotVolume, wantVolume)
	}
	cleanup(t, orchestrator)
}

func TestAddVolumeWhenVolumeExistsAsSubordinateVolume(t *testing.T) {
	const (
		scName         = "fakeSC"
		fullVolumeName = "fakeVolume"
	)
	var wantVolume *storage.VolumeExternal
	var wantErr assert.ErrorAssertionFunc
	wantErr = assert.Error
	orchestrator := getOrchestrator(t, false)

	fullVolumeConfig := tu.GenerateVolumeConfig(
		fullVolumeName, 50, scName,
		config.File,
	)

	Volume := &storage.Volume{Config: fullVolumeConfig}
	orchestrator.subordinateVolumes[fullVolumeConfig.Name] = Volume
	gotVolume, err := orchestrator.AddVolume(ctx(), fullVolumeConfig)

	if err == nil {
		wantVolume = Volume.ConstructExternal()
	} else {
		wantVolume = nil
	}
	if !wantErr(t, err, "Unexpected Result") {
		return
	}
	if !reflect.DeepEqual(gotVolume, wantVolume) {
		t.Errorf("GotVolume = %v, expected %v", gotVolume, wantVolume)
	}
	cleanup(t, orchestrator)
}

func TestResizeSubordinateVolume(t *testing.T) {
	backendUUID := "abcd"
	tests := []struct {
		name             string
		resizeVal        string
		sourceVolumeName string
		subordVolumeName string
		bootstrapError   error
		sourceVolConfig  *storage.VolumeConfig
		subordVolConfig  *storage.VolumeConfig
		wantErr          assert.ErrorAssertionFunc
	}{
		{
			name:             "BootstrapError",
			resizeVal:        "1gi",
			sourceVolumeName: "fakeSrcVol",
			subordVolumeName: "fakeSubordinateVol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subordVolConfig:  tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:   errors.New("bootstrap error"),
			wantErr:          assert.Error,
		},
		{
			name:             "NilSubordinateVolumeError",
			resizeVal:        "1gi",
			sourceVolumeName: "fakeSrcVol",
			subordVolumeName: "fakeSubordinateVol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subordVolConfig:  tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:   nil,
			wantErr:          assert.Error,
		},
		{
			name:             "NilParentVolumeError",
			resizeVal:        "1gi",
			sourceVolumeName: "fakeSrcVol",
			subordVolumeName: "fakeSubordinateVol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subordVolConfig:  tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:   nil,
			wantErr:          assert.Error,
		},
		{
			name:             "UnsupportedResizeValue1",
			resizeVal:        "2xi",
			sourceVolumeName: "fakeSrcVol",
			subordVolumeName: "fakeSubordinateVol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subordVolConfig:  tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:   nil,
			wantErr:          assert.Error,
		},
		{
			name:             "UnsupportedResizeValue2",
			resizeVal:        "-2gi",
			sourceVolumeName: "fakeSrcVol",
			subordVolumeName: "fakeSubordinateVol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subordVolConfig:  tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:   nil,
			wantErr:          assert.Error,
		},
		{
			name:             "UnsupportedSourceSizeValue1",
			resizeVal:        "2gi",
			sourceVolumeName: "fakeSrcVol",
			subordVolumeName: "fakeSubordinateVol",
			sourceVolConfig:  &storage.VolumeConfig{Name: "fakeSrcVol", InternalName: "fakeSrcVol", Size: "1xi", Protocol: config.File, StorageClass: "fakeSC", SnapshotPolicy: "none", SnapshotDir: "none", UnixPermissions: "", VolumeMode: config.Filesystem},
			subordVolConfig:  tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:   nil,
			wantErr:          assert.Error,
		},
		{
			name:             "UnsupportedSourceSizeValue2",
			resizeVal:        "2gi",
			sourceVolumeName: "fakeSrcVol",
			subordVolumeName: "fakeSubordinateVol",
			sourceVolConfig:  &storage.VolumeConfig{Name: "fakeSrcVol", InternalName: "fakeSrcVol", Size: "-1gi", Protocol: config.File, StorageClass: "fakeSC", SnapshotPolicy: "none", SnapshotDir: "none", UnixPermissions: "", VolumeMode: config.Filesystem},
			subordVolConfig:  tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:   nil,
			wantErr:          assert.Error,
		},
		{
			name:             "SubordinateSizeGreaterError",
			resizeVal:        "3Gi",
			sourceVolumeName: "fakeSrcVol",
			subordVolumeName: "fakeSubordinateVol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			subordVolConfig:  tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:   nil,
			wantErr:          assert.Error,
		},
		{
			name:             "PersistentStoreUpdateError",
			resizeVal:        "2Gi",
			sourceVolumeName: "fakeSrcVol",
			subordVolumeName: "fakeSubordinateVol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fakeSrcVol", 2, "fakeSC", config.File),
			subordVolConfig:  tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:   nil,
			wantErr:          assert.Error,
		},
		{
			name:             "PersistentStoreUpdateSuccess",
			resizeVal:        "2Gi",
			sourceVolumeName: "fakeSrcVol",
			subordVolumeName: "fakeSubordinateVol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fakeSrcVol", 2, "fakeSC", config.File),
			subordVolConfig:  tu.GenerateVolumeConfig("fakeSubordinateVol", 1, "fakeSC", config.File),
			bootstrapError:   nil,
			wantErr:          assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceVolume := &storage.Volume{Config: tt.sourceVolConfig}
			subordVolume := &storage.Volume{Config: tt.subordVolConfig}
			subordVolume.Config.ShareSourceVolume = tt.sourceVolumeName
			tt.sourceVolConfig.SubordinateVolumes = make(map[string]interface{})
			volumes := map[string]*storage.Volume{tt.sourceVolumeName: sourceVolume}
			subordinateVolumes := map[string]*storage.Volume{tt.subordVolumeName: subordVolume}
			if tt.name == "NilSubordinateVolumeError" {
				subordinateVolumes = map[string]*storage.Volume{tt.subordVolumeName: nil}
			}
			if tt.name == "NilParentVolumeError" {
				volumes = map[string]*storage.Volume{tt.sourceVolumeName: nil}
			}

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			// Create a fake backend with UUID
			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
			mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
			mockBackend.EXPECT().GetDriverName().Return("baz").AnyTimes()
			mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
			mockBackend.EXPECT().Name().Return("mockBackend").AnyTimes()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			if tt.name == "PersistentStoreUpdateError" {
				mockStoreClient.EXPECT().UpdateVolume(coreCtx,
					gomock.Any()).Return(errors.New("persistent store update failed"))
			} else {
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}

			// Create orchestrator with fake backend and initial values
			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.subordinateVolumes = subordinateVolumes
			o.volumes = volumes
			o.bootstrapError = tt.bootstrapError

			err := o.ResizeVolume(ctx(), tt.subordVolumeName, tt.resizeVal)
			tt.wantErr(t, err, "Unexpected Result")
		})
	}
}

func TestResizeVolume(t *testing.T) {
	backendUUID := "abcd"
	volName := "fakeSrcVol"
	volConfig := tu.GenerateVolumeConfig(volName, 1, "fakeSC", config.File)
	volConfig.SubordinateVolumes = make(map[string]interface{})

	tests := []struct {
		name    string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "VolumeNotFoundError",
			wantErr: assert.Error,
		},
		{
			name:    "VolumeStateIsDeletingError",
			wantErr: assert.Error,
		},
		{
			name:    "AddVolumeTransactionError",
			wantErr: assert.Error,
		},
		{
			name:    "BackendNotFoundError",
			wantErr: assert.Error,
		},
		{
			name:    "BackendResizeVolumeError",
			wantErr: assert.Error,
		},
		{
			name:    "PersistentStoreUpdateError",
			wantErr: assert.Error,
		},
		{
			name:    "DeleteVolumeTranxError",
			wantErr: assert.Error,
		},
		{
			name:    "PersistentStoreUpdateSuccess",
			wantErr: assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vol := &storage.Volume{Config: volConfig}
			vol.Orphaned = true
			volumes := map[string]*storage.Volume{}
			if tt.name != "VolumeNotFoundError" {
				volumes[volName] = vol
			}
			if tt.name == "VolumeStateIsDeletingError" {
				vol.State = storage.VolumeStateDeleting
			}
			if tt.name != "BackendNotFoundError" {
				vol.BackendUUID = backendUUID
			}

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			// Create a fake backend with UUID
			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
			mockBackend.EXPECT().GetProtocol(gomock.Any()).Return(config.File).AnyTimes()
			mockBackend.EXPECT().GetDriverName().Return("baz").AnyTimes()
			mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
			mockBackend.EXPECT().Name().Return("mockBackend").AnyTimes()
			if tt.name == "BackendResizeVolumeError" {
				mockBackend.EXPECT().ResizeVolume(coreCtx, gomock.Any(),
					gomock.Any()).Return(errors.New("unable to resize"))
			} else {
				mockBackend.EXPECT().ResizeVolume(coreCtx, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			}

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			mockStoreClient.EXPECT().GetVolumeTransaction(coreCtx, gomock.Any()).Return(nil, nil).AnyTimes()
			if tt.name == "DeleteVolumeTranxError" {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx,
					gomock.Any()).Return(errors.New("delete failed"))
			} else {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}
			if tt.name == "AddVolumeTransactionError" {
				mockStoreClient.EXPECT().AddVolumeTransaction(coreCtx,
					gomock.Any()).Return(errors.New("failed to add to transaction"))
			} else {
				mockStoreClient.EXPECT().AddVolumeTransaction(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}
			if tt.name == "PersistentStoreUpdateError" {
				mockStoreClient.EXPECT().UpdateVolume(coreCtx,
					gomock.Any()).Return(errors.New("persistent store update failed"))
			} else {
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}

			// Create orchestrator with fake backend and initial values
			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.volumes = volumes

			err := o.ResizeVolume(ctx(), volName, "2gi")
			tt.wantErr(t, err, "Unexpected Result")
		})
	}
}

func TestHandleFailedTranxResizeVolume(t *testing.T) {
	backendUUID := "abcd"
	tests := []struct {
		name    string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "VolumeNotPresent",
			wantErr: assert.Error,
		},
		{
			name:    "BackendNotFoundError",
			wantErr: assert.NoError,
		},
		{
			name:    "ResizeVolumeSuccess",
			wantErr: assert.NoError,
		},
		{
			name:    "ResizeVolumeFail",
			wantErr: assert.Error,
		},
	}
	vc := tu.GenerateVolumeConfig("fakeVol", 1, "fakeSC", config.File)
	svt := &storage.VolumeTransaction{Op: storage.ResizeVolume, Config: vc}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vol := &storage.Volume{Config: vc}
			if tt.name == "VolumeNotPresent" || tt.name == "ResizeVolumeSuccess" {
				vol.BackendUUID = backendUUID
			}
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			// Create a fake backend with UUID
			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			mockStoreClient.EXPECT().UpdateVolume(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			if tt.name == "VolumeNotPresent" {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx,
					gomock.Any()).Return(errors.New("delete failed"))
			} else {
				mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}
			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			if tt.name == "ResizeVolumeSuccess" || tt.name == "ResizeVolumeFail" {
				o.volumes = map[string]*storage.Volume{"fakeVol": vol}
			}
			err := o.handleFailedTransaction(ctx(), svt)
			tt.wantErr(t, err, "Unexpected Result")
		})
	}
}

func TestDeleteSubordinateVolume(t *testing.T) {
	backendUUID := "abcd"
	sc := "fakeSC"
	srcVolName := "fakeSrcVol"
	subVolName := "fakeSubordinateVol"

	tests := []struct {
		name    string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "VolumeNotFound",
			wantErr: assert.Error,
		},
		{
			name:    "FailedToDeleteVolumeFromPersistentStore",
			wantErr: assert.Error,
		},
		{
			name:    "SharedSourceVolumeNotFound",
			wantErr: assert.NoError,
		},
		{
			name:    "ErrorDeletingSourceVolume",
			wantErr: assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceVolume := &storage.Volume{State: storage.VolumeStateDeleting, BackendUUID: backendUUID}
			sourceVolume.Config = tu.GenerateVolumeConfig(srcVolName, 1, sc, config.File)
			sourceVolume.Config.SubordinateVolumes = map[string]interface{}{subVolName: nil}

			subordVolume := &storage.Volume{Config: tu.GenerateVolumeConfig(subVolName, 1, sc, config.File)}
			if tt.name != "SharedSourceVolumeNotFound" {
				subordVolume.Config.ShareSourceVolume = srcVolName
			}

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().RemoveVolume(coreCtx, gomock.Any()).Return(errors.New("error")).AnyTimes()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			if tt.name == "FailedToDeleteVolumeFromPersistentStore" {
				mockStoreClient.EXPECT().DeleteVolume(coreCtx, gomock.Any()).Return(errors.New("delete failed"))
			} else {
				mockStoreClient.EXPECT().DeleteVolume(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}

			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.volumes[srcVolName] = sourceVolume
			if tt.name != "VolumeNotFound" {
				o.subordinateVolumes[subVolName] = subordVolume
			}

			err := o.deleteSubordinateVolume(coreCtx, subVolName)
			tt.wantErr(t, err, "Unexpected result")
		})
	}

	// Calling deleteSubordinateVolume from DeleteVolume function
	t.Run("DeleteSubVolFromDeleteVol", func(t *testing.T) {
		srcVolConfig := tu.GenerateVolumeConfig(srcVolName, 1, sc, config.File)
		srcVol := &storage.Volume{Config: srcVolConfig, BackendUUID: backendUUID}
		srcVol.Config.SubordinateVolumes = map[string]interface{}{subVolName: nil}

		subVolConfig := tu.GenerateVolumeConfig(subVolName, 1, sc, config.File)
		subVol := &storage.Volume{Config: subVolConfig}
		subVol.Config.ShareSourceVolume = srcVolName

		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		mockBackend := mockstorage.NewMockBackend(mockCtrl)
		mockBackend.EXPECT().RemoveVolume(ctx(), gomock.Any()).Return(errors.New("error")).AnyTimes()
		mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
		mockBackend.EXPECT().GetDriverName().Return("baz").AnyTimes()
		mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
		mockBackend.EXPECT().Name().Return("mockBackend").AnyTimes()

		mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
		mockStoreClient.EXPECT().DeleteVolume(coreCtx, gomock.Any()).Return(errors.New("delete failed"))

		o := getOrchestrator(t, false)
		o.storeClient = mockStoreClient
		o.backends[backendUUID] = mockBackend
		o.subordinateVolumes[subVolName] = subVol
		o.volumes[srcVolName] = srcVol

		err := o.DeleteVolume(ctx(), subVolName)
		assert.Error(t, err, "delete failed")
	})
}

func TestDeleteVolume(t *testing.T) {
	backendUUID := "abcd"
	srcVolName := "fakeSrcVol"
	cloneVolName := "cloneVol"
	sc := "fakeSC"

	publicDeleteVolumeTests := []struct {
		name             string
		sourceVolumeName string
		sourceVolConfig  *storage.VolumeConfig
		wantErr          assert.ErrorAssertionFunc
	}{
		{
			name:             "FailedToAddTranx",
			sourceVolumeName: "fakeSrcVol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			wantErr:          assert.Error,
		},
		{
			name:             "FailedToDeleteTranx",
			sourceVolumeName: "fakeSrcVol",
			sourceVolConfig:  tu.GenerateVolumeConfig("fakeSrcVol", 1, "fakeSC", config.File),
			wantErr:          assert.Error,
		},
	}

	for _, tt := range publicDeleteVolumeTests {
		t.Run(tt.name, func(t *testing.T) {
			sourceVolume := &storage.Volume{Config: tt.sourceVolConfig, BackendUUID: backendUUID, Orphaned: true}
			sourceVolume.State = storage.VolumeStateDeleting

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().RemoveVolume(coreCtx, gomock.Any()).Return(errors.New("error")).AnyTimes()
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
			mockBackend.EXPECT().GetDriverName().Return("baz").AnyTimes()
			mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
			mockBackend.EXPECT().Name().Return("mockBackend").AnyTimes()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx,
				gomock.Any()).Return(errors.New("failed to delete tranx")).AnyTimes()
			if tt.name == "FailedToAddTranx" {
				mockStoreClient.EXPECT().GetVolumeTransaction(coreCtx, gomock.Any()).Return(nil,
					errors.New("falied to get tranx"))
			} else {
				mockStoreClient.EXPECT().GetVolumeTransaction(coreCtx, gomock.Any()).Return(nil, nil)
				mockStoreClient.EXPECT().AddVolumeTransaction(coreCtx, gomock.Any()).Return(nil)
			}

			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.volumes[tt.sourceVolumeName] = sourceVolume

			err := o.DeleteVolume(ctx(), tt.sourceVolumeName)
			tt.wantErr(t, err, "Unexpected result")
		})
	}

	privateDeleteVolumeTests := []struct {
		name    string
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "HasSubordinateVolume",
			wantErr: assert.NoError,
		},
		{
			name:    "HasSubordinateVolumeError",
			wantErr: assert.Error,
		},
		{
			name:    "BackendNilError",
			wantErr: assert.Error,
		},
		{
			name:    "DeleteFromPersistentStoreError",
			wantErr: assert.Error,
		},
		{
			name:    "DeleteClone",
			wantErr: assert.NoError,
		},
		{
			name:    "DeleteBackendError",
			wantErr: assert.Error,
		},
	}

	for _, tt := range privateDeleteVolumeTests {
		t.Run(tt.name, func(t *testing.T) {
			sourceVolume := &storage.Volume{State: storage.VolumeStateDeleting, BackendUUID: backendUUID, Orphaned: true}
			sourceVolume.Config = tu.GenerateVolumeConfig(srcVolName, 1, sc, config.File)
			sourceVolume.Config.CloneSourceVolume = cloneVolName
			if tt.name == "HasSubordinateVolume" || tt.name == "HasSubordinateVolumeError" {
				sourceVolume.Config.SubordinateVolumes = map[string]interface{}{"dummyVol": nil}
			}
			if tt.name == "BackendNilError" {
				sourceVolume.BackendUUID = "dummyBackend"
			}

			var cloneVolume *storage.Volume
			if tt.name == "DeleteClone" {
				cloneVolume = &storage.Volume{Config: tu.GenerateVolumeConfig(cloneVolName, 1, sc, config.File)}
				cloneVolume.State = storage.VolumeStateDeleting
				cloneVolume.Config.SubordinateVolumes = map[string]interface{}{"dummyVol": nil}
			}

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
			mockBackend.EXPECT().GetDriverName().Return("baz").AnyTimes()
			mockBackend.EXPECT().Name().Return("mockBackend").AnyTimes()
			if tt.name == "DeleteBackendError" {
				mockBackend.EXPECT().HasVolumes().Return(false)
				mockBackend.EXPECT().State().Return(storage.Deleting)
			} else {
				mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
			}
			if tt.name == "DeleteFromPersistentStoreError" {
				mockBackend.EXPECT().RemoveVolume(coreCtx, gomock.Any()).Return(errors.NotManagedError("not managed"))
			} else {
				mockBackend.EXPECT().RemoveVolume(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx,
				gomock.Any()).Return(errors.New("failed to delete tranx")).AnyTimes()
			if tt.name == "HasSubordinateVolumeError" || tt.name == "DeleteClone" {
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, gomock.Any()).Return(errors.New("error updating volume"))
			} else {
				mockStoreClient.EXPECT().UpdateVolume(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}
			if tt.name == "BackendNilError" || tt.name == "DeleteFromPersistentStoreError" {
				mockStoreClient.EXPECT().DeleteVolume(coreCtx, gomock.Any()).Return(errors.New("unable to delete"))
			} else {
				mockStoreClient.EXPECT().DeleteVolume(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}
			if tt.name == "DeleteBackendError" {
				mockStoreClient.EXPECT().DeleteBackend(coreCtx,
					gomock.Any()).Return(errors.New("delete backend failed"))
			}

			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.volumes[srcVolName] = sourceVolume
			if tt.name == "DeleteClone" {
				o.volumes[cloneVolName] = cloneVolume
			}

			err := o.deleteVolume(coreCtx, srcVolName)
			tt.wantErr(t, err, "Unexpected result")
		})
	}
}

func TestHandleFailedDeleteVolumeError(t *testing.T) {
	backendUUID := "abcd"
	srcVolName := "fakeSrcVol"
	sc := "fakeSC"
	volConfig := tu.GenerateVolumeConfig(srcVolName, 1, sc, config.File)
	tnx := &storage.VolumeTransaction{Op: storage.DeleteVolume, Config: volConfig}
	sourceVolume := &storage.Volume{Config: volConfig, BackendUUID: backendUUID}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().GetDriverName().Return("baz").AnyTimes()
	mockBackend.EXPECT().Name().Return("mockBackend").AnyTimes()
	mockBackend.EXPECT().RemoveVolume(coreCtx, gomock.Any()).Return(nil)

	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	mockStoreClient.EXPECT().DeleteVolume(coreCtx, gomock.Any()).Return(errors.New("unable to delete volume"))
	mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx,
		gomock.Any()).Return(errors.New("unable to delete transaction"))

	o := getOrchestrator(t, false)
	o.storeClient = mockStoreClient
	o.backends[backendUUID] = mockBackend
	o.volumes[srcVolName] = sourceVolume

	err := o.handleFailedTransaction(ctx(), tnx)
	assert.Error(t, err, "failed to delete volume transaction")
}

func TestCreateSnapshotError(t *testing.T) {
	backendUUID := "abcd"
	// snap that already exists in o.snapshots
	sc1 := &storage.SnapshotConfig{Name: "snap1", VolumeName: "vol1"}
	id1 := sc1.ID()
	// snap of volume in deleting state
	sc2 := &storage.SnapshotConfig{Name: "snap2", VolumeName: "vol2"}
	vol2 := &storage.Volume{Config: &storage.VolumeConfig{Name: "vol2"}, State: storage.VolumeStateDeleting}
	// snap and vol used in other tests
	sc3 := &storage.SnapshotConfig{Name: "snap3", VolumeName: "vol3", Version: "1", InternalName: "snap3", VolumeInternalName: "snap3"}
	vol3 := &storage.Volume{Config: &storage.VolumeConfig{Name: "vol3", InternalName: "vol3"}, BackendUUID: backendUUID}
	snap3 := &storage.Snapshot{Config: sc3, Created: "1pm", SizeBytes: 100, State: storage.SnapshotStateCreating}

	tests := []struct {
		name       string
		snapConfig *storage.SnapshotConfig
		vol        *storage.Volume
	}{
		{
			name:       "SnapshotExists",
			snapConfig: sc1,
			vol:        vol2,
		},
		{
			name:       "VolumeNotFound",
			snapConfig: &storage.SnapshotConfig{Name: "dummy", VolumeName: "dummy"},
			vol:        vol2,
		},
		{
			name:       "VolumeInDeletingState",
			snapConfig: sc2,
			vol:        vol2,
		},
		{
			name:       "BackendNotFound",
			snapConfig: &storage.SnapshotConfig{Name: "snap3", VolumeName: "vol3"},
			vol:        &storage.Volume{Config: &storage.VolumeConfig{Name: "vol3"}, BackendUUID: "dummy"},
		},
		{
			name:       "SnapshotNotPossible",
			snapConfig: sc3,
			vol:        vol3,
		},
		{
			name:       "AddVolumeTranxError",
			snapConfig: sc3,
			vol:        vol3,
		},
		{
			name:       "MaxLimitError",
			snapConfig: sc3,
			vol:        vol3,
		},
		{
			name:       "CreateSnapshotError",
			snapConfig: sc3,
			vol:        vol3,
		},
		{
			name:       "AddSnapshotError",
			snapConfig: sc3,
			vol:        vol3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
			mockBackend.EXPECT().Name().Return("backend").AnyTimes()
			mockBackend.EXPECT().GetDriverName().Return("driver").AnyTimes()
			mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
			if tt.name == "SnapshotNotPossible" {
				mockBackend.EXPECT().CanSnapshot(coreCtx, gomock.Any(),
					gomock.Any()).Return(errors.New("cannot take snapshot"))
			} else {
				mockBackend.EXPECT().CanSnapshot(coreCtx, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			}

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			if tt.name == "AddVolumeTranxError" {
				mockStoreClient.EXPECT().GetVolumeTransaction(coreCtx, gomock.Any()).Return(nil,
					errors.New("error getting transaction"))
			} else {
				mockStoreClient.EXPECT().GetVolumeTransaction(coreCtx, gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}
			if tt.name == "MaxLimitError" {
				mockBackend.EXPECT().CreateSnapshot(coreCtx, gomock.Any(), gomock.Any()).Return(nil,
					errors.MaxLimitReachedError("error"))
				mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx,
					gomock.Any()).Return(errors.New("failed to delete transaction"))
			}
			if tt.name == "CreateSnapshotError" {
				mockBackend.EXPECT().CreateSnapshot(coreCtx, gomock.Any(), gomock.Any()).Return(nil,
					errors.New("failed to create snapshot"))
				mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx, gomock.Any()).Return(nil)
			}

			if tt.name == "AddSnapshotError" {
				mockBackend.EXPECT().CreateSnapshot(coreCtx, gomock.Any(), gomock.Any()).Return(snap3, nil)
				mockBackend.EXPECT().DeleteSnapshot(coreCtx, gomock.Any(),
					gomock.Any()).Return(errors.New("cleanup error"))
				mockStoreClient.EXPECT().AddSnapshot(coreCtx, gomock.Any()).Return(errors.New("failed to add snapshot"))
			}

			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.backends[backendUUID] = mockBackend
			o.snapshots[id1] = &storage.Snapshot{Config: sc1}
			o.volumes[tt.vol.Config.Name] = tt.vol

			_, err := o.CreateSnapshot(ctx(), tt.snapConfig)
			assert.Error(t, err, "unexpected error")
		})
	}
}

func TestRestoreSnapshot_BootstrapError(t *testing.T) {
	snapName := "snap"
	volName := "vol"

	o := getOrchestrator(t, false)
	o.bootstrapError = errors.New("failed")

	err := o.RestoreSnapshot(ctx(), volName, snapName)

	assert.Error(t, err, "RestoreSnapshot should have failed")
}

func TestRestoreSnapshot_VolumeNotFound(t *testing.T) {
	snapName := "snap"
	volName := "vol"

	o := getOrchestrator(t, false)

	err := o.RestoreSnapshot(ctx(), volName, snapName)

	assert.Error(t, err, "RestoreSnapshot should have failed")
}

func TestRestoreSnapshot_SnapshotNotFound(t *testing.T) {
	o := getOrchestrator(t, false)

	backendUUID := "abcd"
	snapName := "snap"
	volName := "vol"
	volConfig := &storage.VolumeConfig{Name: volName}

	o.volumes[volName] = &storage.Volume{Config: volConfig, BackendUUID: backendUUID}

	err := o.RestoreSnapshot(ctx(), volName, snapName)

	assert.Error(t, err, "RestoreSnapshot should have failed")
}

func TestRestoreSnapshot_BackendNotFound(t *testing.T) {
	o := getOrchestrator(t, false)

	backendUUID := "abcd"
	snapName := "snap"
	volName := "vol"
	snapID := storage.MakeSnapshotID(volName, snapName)
	snapConfig := &storage.SnapshotConfig{Name: snapName, VolumeName: volName}
	volConfig := &storage.VolumeConfig{Name: volName}

	o.snapshots[snapID] = &storage.Snapshot{Config: snapConfig}
	o.volumes[volName] = &storage.Volume{Config: volConfig, BackendUUID: backendUUID}

	err := o.RestoreSnapshot(ctx(), volName, snapName)

	assert.Error(t, err, "RestoreSnapshot should have failed")
}

func TestRestoreSnapshot_VolumeHasPublications(t *testing.T) {
	o := getOrchestrator(t, false)

	backendUUID := "abcd"
	backendName := "xyz"
	snapName := "snap"
	volName := "vol"
	nodeName := "node"
	snapID := storage.MakeSnapshotID(volName, snapName)
	snapConfig := &storage.SnapshotConfig{Name: snapName, VolumeName: volName}
	volConfig := &storage.VolumeConfig{Name: volName}
	node := &models.Node{Name: nodeName}
	publication := &models.VolumePublication{
		Name:       volName + "/" + nodeName,
		NodeName:   nodeName,
		VolumeName: volName,
		ReadOnly:   false,
		AccessMode: 1,
	}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	mockBackend.EXPECT().Name().Return(backendName).AnyTimes()
	mockBackend.EXPECT().GetDriverName().Return("driverName").AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

	o.snapshots[snapID] = &storage.Snapshot{Config: snapConfig}
	o.volumes[volName] = &storage.Volume{Config: volConfig, BackendUUID: backendUUID}
	o.backends[backendUUID] = mockBackend
	o.nodes.Set(nodeName, node)
	_ = o.volumePublications.Set(volName, nodeName, publication)

	err := o.RestoreSnapshot(ctx(), volName, snapName)

	assert.Error(t, err, "RestoreSnapshot should have failed")
}

func TestRestoreSnapshot_RestoreFailed(t *testing.T) {
	o := getOrchestrator(t, false)

	backendUUID := "abcd"
	backendName := "xyz"
	snapName := "snap"
	volName := "vol"
	snapID := storage.MakeSnapshotID(volName, snapName)
	snapConfig := &storage.SnapshotConfig{Name: snapName, VolumeName: volName}
	volConfig := &storage.VolumeConfig{Name: volName}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	mockBackend.EXPECT().Name().Return(backendName).AnyTimes()
	mockBackend.EXPECT().GetDriverName().Return("driverName").AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().RestoreSnapshot(coreCtx, snapConfig, volConfig).Return(errors.New("failed"))

	o.snapshots[snapID] = &storage.Snapshot{Config: snapConfig}
	o.volumes[volName] = &storage.Volume{Config: volConfig, BackendUUID: backendUUID}
	o.backends[backendUUID] = mockBackend

	err := o.RestoreSnapshot(ctx(), volName, snapName)

	assert.Error(t, err, "RestoreSnapshot should have failed")
}

func TestRestoreSnapshot(t *testing.T) {
	o := getOrchestrator(t, false)

	backendUUID := "abcd"
	backendName := "xyz"
	snapName := "snap"
	volName := "vol"
	snapID := storage.MakeSnapshotID(volName, snapName)
	snapConfig := &storage.SnapshotConfig{Name: snapName, VolumeName: volName}
	volConfig := &storage.VolumeConfig{Name: volName}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	mockBackend.EXPECT().Name().Return(backendName).AnyTimes()
	mockBackend.EXPECT().GetDriverName().Return("driverName").AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().RestoreSnapshot(coreCtx, snapConfig, volConfig).Return(nil)

	o.snapshots[snapID] = &storage.Snapshot{Config: snapConfig}
	o.volumes[volName] = &storage.Volume{Config: volConfig, BackendUUID: backendUUID}
	o.backends[backendUUID] = mockBackend

	err := o.RestoreSnapshot(ctx(), volName, snapName)

	assert.NoError(t, err, "RestoreSnapshot should not have failed")
}

func TestImportSnapshot_SnapshotAlreadyExists(t *testing.T) {
	o := getOrchestrator(t, false)

	backendUUID := "test-backend"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	snapName := "snapshot-import"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
		ImportNotManaged:   true,
	}
	snapshot := &storage.Snapshot{
		Config:    snapConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	// Initialize mocks.
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	// Set up common mock expectations between test cases.
	mockBackend.EXPECT().GetDriverName().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().Name().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

	// Set up test case specific mock expectations and inject mocks into core.
	o.snapshots[snapConfig.ID()] = snapshot

	// Call method under test and make assertions.
	importedSnap, err := o.ImportSnapshot(ctx(), snapConfig)
	assert.Error(t, err)
	assert.True(t, errors.IsFoundError(err))
	assert.Nil(t, importedSnap)
}

func TestImportSnapshot_SourceVolumeNotFound(t *testing.T) {
	o := getOrchestrator(t, false)

	backendUUID := "test-backend"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Version:             "",
			Name:                volumeName,
			InternalName:        volumeInternalName,
			ImportOriginalName:  "import-" + volumeName,
			ImportBackendUUID:   "import-" + backendUUID,
			ImportNotManaged:    false,
			LUKSPassphraseNames: nil,
		},
		BackendUUID: backendUUID,
	}
	snapName := "snapshot-import"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
		ImportNotManaged:   true,
	}

	// Initialize mocks.
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	// Set up common mock expectations between test cases.
	mockBackend.EXPECT().GetDriverName().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().Name().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

	// Set up test case specific mock expectations and inject mocks into core.
	o.backends[volume.BackendUUID] = mockBackend

	// Call method under test and make assertions.
	importedSnap, err := o.ImportSnapshot(ctx(), snapConfig)
	assert.Error(t, err)
	assert.True(t, errors.IsNotFoundError(err))
	assert.Nil(t, importedSnap)
}

func TestImportSnapshot_BackendNotfound(t *testing.T) {
	o := getOrchestrator(t, false)

	// Initialize variables used in all subtests.
	backendUUID := "test-backend"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Version:             "",
			Name:                volumeName,
			InternalName:        volumeInternalName,
			ImportOriginalName:  "import-" + volumeName,
			ImportBackendUUID:   "import-" + backendUUID,
			ImportNotManaged:    false,
			LUKSPassphraseNames: nil,
		},
		BackendUUID: backendUUID,
	}
	snapName := "snapshot-import"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
		ImportNotManaged:   true,
	}

	// Initialize mocks.
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	// Set up common mock expectations between test cases.
	mockBackend.EXPECT().GetDriverName().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().Name().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

	// Set up test case specific mock expectations and inject mocks into core.
	o.volumes[snapConfig.VolumeName] = volume

	// Call method under test and make assertions.
	importedSnap, err := o.ImportSnapshot(ctx(), snapConfig)
	assert.Error(t, err)
	assert.True(t, errors.IsNotFoundError(err))
	assert.Nil(t, importedSnap)
}

func TestImportSnapshot_SnapshotNotFound(t *testing.T) {
	o := getOrchestrator(t, false)

	// Initialize variables used in all subtests.
	backendUUID := "test-backend"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Version:             "",
			Name:                volumeName,
			InternalName:        volumeInternalName,
			ImportOriginalName:  "import-" + volumeName,
			ImportBackendUUID:   "import-" + backendUUID,
			ImportNotManaged:    false,
			LUKSPassphraseNames: nil,
		},
		BackendUUID: backendUUID,
	}
	snapName := "snapshot-import"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
		ImportNotManaged:   true,
	}

	// Initialize mocks.
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	// Set up common mock expectations between test cases.
	mockBackend.EXPECT().GetDriverName().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().Name().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

	// Set up test case specific mock expectations and inject mocks into core.
	mockBackend.EXPECT().GetSnapshot(
		gomock.Any(), snapConfig, volume.Config,
	).Return(nil, errors.NotFoundError("not found"))

	o.backends[volume.BackendUUID] = mockBackend
	o.volumes[snapConfig.VolumeName] = volume

	// Call method under test and make assertions.
	importedSnap, err := o.ImportSnapshot(ctx(), snapConfig)
	assert.Error(t, err)
	assert.True(t, errors.IsNotFoundError(err))
	assert.Nil(t, importedSnap)
}

func TestImportSnapshot_FailToGetBackendSnapshot(t *testing.T) {
	o := getOrchestrator(t, false)

	// Initialize variables used in all subtests.
	backendUUID := "test-backend"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Version:             "",
			Name:                volumeName,
			InternalName:        volumeInternalName,
			ImportOriginalName:  "import-" + volumeName,
			ImportBackendUUID:   "import-" + backendUUID,
			ImportNotManaged:    false,
			LUKSPassphraseNames: nil,
		},
		BackendUUID: backendUUID,
	}
	snapName := "snapshot-import"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
		ImportNotManaged:   true,
	}

	// Initialize mocks.
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	// Set up common mock expectations between test cases.
	mockBackend.EXPECT().GetDriverName().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().Name().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

	// Set up test case specific mock expectations and inject mocks into core.
	mockBackend.EXPECT().GetSnapshot(
		gomock.Any(), snapConfig, volume.Config,
	).Return(nil, errors.New("backend error"))

	o.backends[volume.BackendUUID] = mockBackend
	o.volumes[snapConfig.VolumeName] = volume

	// Call method under test and make assertions.
	importedSnap, err := o.ImportSnapshot(ctx(), snapConfig)
	assert.Error(t, err)
	assert.Nil(t, importedSnap)
}

func TestImportSnapshot_FailToAddSnapshot(t *testing.T) {
	o := getOrchestrator(t, false)

	// Initialize variables used in all subtests.
	backendUUID := "test-backend"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Version:             "",
			Name:                volumeName,
			InternalName:        volumeInternalName,
			ImportOriginalName:  "import-" + volumeName,
			ImportBackendUUID:   "import-" + backendUUID,
			ImportNotManaged:    false,
			LUKSPassphraseNames: nil,
		},
		BackendUUID: backendUUID,
	}
	snapName := "snapshot-import"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
		ImportNotManaged:   true,
	}
	snapshot := &storage.Snapshot{
		Config:    snapConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	// Initialize mocks.
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)

	// Set up common mock expectations between test cases.
	mockBackend.EXPECT().GetDriverName().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().Name().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

	// Set up test case specific mock expectations and inject mocks into core.
	mockBackend.EXPECT().GetSnapshot(
		gomock.Any(), snapConfig, volume.Config,
	).Return(snapshot, nil)
	mockStore.EXPECT().AddSnapshot(gomock.Any(), snapshot).Return(fmt.Errorf("store error"))

	o.storeClient = mockStore
	o.backends[volume.BackendUUID] = mockBackend
	o.volumes[snapConfig.VolumeName] = volume

	// Call method under test and make assertions.
	importedSnap, err := o.ImportSnapshot(ctx(), snapConfig)
	assert.Error(t, err)
	assert.Nil(t, importedSnap)
}

func TestImportSnapshot(t *testing.T) {
	o := getOrchestrator(t, false)

	// Initialize variables used in all subtests.
	backendUUID := "test-backend"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Version:             "",
			Name:                volumeName,
			InternalName:        volumeInternalName,
			ImportOriginalName:  "import-" + volumeName,
			ImportBackendUUID:   "import-" + backendUUID,
			ImportNotManaged:    false,
			LUKSPassphraseNames: nil,
		},
		BackendUUID: backendUUID,
	}
	snapName := "snapshot-import"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
		ImportNotManaged:   false,
	}
	snapshot := &storage.Snapshot{
		Config:    snapConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	// Initialize mocks.
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)

	// Set up common mock expectations between test cases.
	mockBackend.EXPECT().GetDriverName().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().Name().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

	// Set up test case specific mock expectations and inject mocks into core.
	mockBackend.EXPECT().GetSnapshot(
		gomock.Any(), snapConfig, volume.Config,
	).Return(snapshot, nil)
	mockStore.EXPECT().AddSnapshot(gomock.Any(), snapshot).Return(nil)

	o.storeClient = mockStore
	o.backends[volume.BackendUUID] = mockBackend
	o.volumes[snapConfig.VolumeName] = volume

	// Call method under test and make assertions.
	importedSnap, err := o.ImportSnapshot(ctx(), snapConfig)
	assert.NoError(t, err)
	assert.NotNil(t, importedSnap)
	assert.EqualValues(t, snapshot.ConstructExternal(), importedSnap)
}

func TestImportSnapshot_VolumeNotManaged(t *testing.T) {
	o := getOrchestrator(t, false)

	// Initialize variables used in all subtests.
	backendUUID := "test-backend"
	volumeName := "pvc-e9748b6b-8240-4fd8-97bc-868bf064ecd4"
	volumeInternalName := "trident_pvc_e9748b6b_8240_4fd8_97bc_868bf064ecd4"
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Version:             "",
			Name:                volumeName,
			InternalName:        volumeInternalName,
			ImportOriginalName:  "import-" + volumeName,
			ImportBackendUUID:   "import-" + backendUUID,
			ImportNotManaged:    true,
			LUKSPassphraseNames: nil,
		},
		BackendUUID: backendUUID,
	}
	snapName := "snapshot-import"
	snapInternalName := "snap.2023-05-23_175116"
	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               snapName,
		VolumeName:         volumeName,
		InternalName:       snapInternalName,
		VolumeInternalName: volumeInternalName,
		ImportNotManaged:   true,
	}
	snapshot := &storage.Snapshot{
		Config:    snapConfig,
		Created:   "2023-05-15T17:04:09Z",
		SizeBytes: 1024,
	}

	// Initialize mocks.
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockStore := mockpersistentstore.NewMockStoreClient(mockCtrl)

	// Set up common mock expectations between test cases.
	mockBackend.EXPECT().GetDriverName().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().Name().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

	// Set up test case specific mock expectations and inject mocks into core.
	mockBackend.EXPECT().GetSnapshot(
		gomock.Any(), snapConfig, volume.Config,
	).Return(snapshot, nil)
	mockStore.EXPECT().AddSnapshot(gomock.Any(), snapshot).Return(nil)

	o.storeClient = mockStore
	o.backends[volume.BackendUUID] = mockBackend
	o.volumes[snapConfig.VolumeName] = volume

	// Call method under test and make assertions.
	importedSnap, err := o.ImportSnapshot(ctx(), snapConfig)
	assert.NoError(t, err)
	assert.NotNil(t, importedSnap)
	assert.EqualValues(t, snapshot.ConstructExternal(), importedSnap)
}

func TestDeleteSnapshotError(t *testing.T) {
	backendUUID := "abcd"
	volName := "vol"
	snapName := "snap"
	snapID := storage.MakeSnapshotID(volName, snapName)
	snapConfig := &storage.SnapshotConfig{Name: snapName, VolumeName: volName}

	publicTests := []struct {
		name     string
		volume   *storage.Volume
		snapshot *storage.Snapshot
	}{
		{
			name:     "SnapshotNotFound",
			volume:   &storage.Volume{Config: &storage.VolumeConfig{Name: volName}},
			snapshot: &storage.Snapshot{Config: &storage.SnapshotConfig{Name: snapName, VolumeName: volName}, State: storage.SnapshotStateCreating},
		},
		{
			name:     "VolumeNotFound",
			volume:   nil,
			snapshot: &storage.Snapshot{Config: &storage.SnapshotConfig{Name: snapName, VolumeName: volName}, State: storage.SnapshotStateCreating},
		},
		{
			name:     "NilVolumeFound",
			volume:   nil,
			snapshot: &storage.Snapshot{Config: &storage.SnapshotConfig{Name: snapName, VolumeName: volName}, State: storage.SnapshotStateMissingVolume},
		},
		{
			name:     "BackendNotFound",
			volume:   &storage.Volume{Config: &storage.VolumeConfig{Name: volName}, BackendUUID: backendUUID},
			snapshot: &storage.Snapshot{Config: &storage.SnapshotConfig{Name: snapName, VolumeName: volName}, State: storage.SnapshotStateCreating},
		},
		{
			name:     "NilBackendFound",
			volume:   &storage.Volume{Config: &storage.VolumeConfig{Name: volName}, BackendUUID: backendUUID},
			snapshot: &storage.Snapshot{Config: &storage.SnapshotConfig{Name: snapName, VolumeName: volName}, State: storage.SnapshotStateMissingBackend},
		},
		{
			name:     "AddVolumeTranxFail",
			volume:   &storage.Volume{Config: &storage.VolumeConfig{Name: volName}, BackendUUID: backendUUID, Orphaned: true},
			snapshot: &storage.Snapshot{Config: &storage.SnapshotConfig{Name: snapName, VolumeName: volName}, State: storage.SnapshotStateCreating},
		},
		{
			name:     "DeleteSnapshotFail",
			volume:   &storage.Volume{Config: &storage.VolumeConfig{Name: volName}, BackendUUID: backendUUID, Orphaned: true},
			snapshot: &storage.Snapshot{Config: &storage.SnapshotConfig{Name: snapName, VolumeName: volName}, State: storage.SnapshotStateCreating},
		},
	}

	for _, tt := range publicTests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
			mockBackend.EXPECT().Name().Return("backend").AnyTimes()
			mockBackend.EXPECT().GetDriverName().Return("driver").AnyTimes()
			mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()
			if tt.name == "DeleteSnapshotFail" {
				mockBackend.EXPECT().DeleteSnapshot(coreCtx, gomock.Any(),
					gomock.Any()).Return(errors.New("delete snapshot failed"))
			} else {
				mockBackend.EXPECT().DeleteSnapshot(coreCtx, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			}

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx,
				gomock.Any()).Return(errors.New("failed to delete transaction")).AnyTimes()
			if tt.name == "NilVolumeFound" || tt.name == "NilBackendFound" {
				mockStoreClient.EXPECT().DeleteSnapshot(coreCtx, gomock.Any()).Return(errors.New("delete failed"))
			}
			if tt.name == "AddVolumeTranxFail" {
				mockStoreClient.EXPECT().GetVolumeTransaction(coreCtx, gomock.Any()).Return(nil,
					errors.New("failed to get transaction"))
			} else {
				mockStoreClient.EXPECT().GetVolumeTransaction(coreCtx, gomock.Any()).Return(nil, nil).AnyTimes()
				mockStoreClient.EXPECT().AddVolumeTransaction(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}

			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			if tt.name != "BackendNotFound" {
				o.backends[backendUUID] = mockBackend
			}
			if tt.name == "NilBackendFound" {
				o.backends[backendUUID] = nil
			}
			if tt.name != "SnapshotNotFound" {
				o.snapshots[snapID] = tt.snapshot
			}
			if tt.name != "VolumeNotFound" {
				o.volumes[volName] = tt.volume
			}

			err := o.DeleteSnapshot(ctx(), volName, snapName)
			assert.Error(t, err, "Unexpected error")
		})
	}

	privateTests := []struct {
		name     string
		volume   *storage.Volume
		snapshot *storage.Snapshot
	}{
		{
			name:     "VolumeNotFound2",
			volume:   nil,
			snapshot: nil,
		},
		{
			name:     "BackendNotFound2",
			volume:   &storage.Volume{Config: &storage.VolumeConfig{Name: volName}, BackendUUID: backendUUID},
			snapshot: nil,
		},
		{
			name:     "DeleteFromPersistentStoreFail",
			volume:   &storage.Volume{Config: &storage.VolumeConfig{Name: volName}, BackendUUID: backendUUID},
			snapshot: &storage.Snapshot{Config: &storage.SnapshotConfig{Name: snapName, VolumeName: volName}, State: storage.SnapshotStateCreating},
		},
		{
			name:     "EmptyVolumeSnapshots",
			volume:   &storage.Volume{Config: &storage.VolumeConfig{Name: volName}, BackendUUID: backendUUID, State: storage.VolumeStateDeleting},
			snapshot: &storage.Snapshot{Config: &storage.SnapshotConfig{Name: snapName, VolumeName: volName}, State: storage.SnapshotStateCreating},
		},
	}

	for _, tt := range privateTests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockBackend := mockstorage.NewMockBackend(mockCtrl)
			mockBackend.EXPECT().DeleteSnapshot(coreCtx, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			mockBackend.EXPECT().RemoveVolume(coreCtx, gomock.Any()).Return(nil).AnyTimes()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			mockStoreClient.EXPECT().DeleteVolume(coreCtx, gomock.Any()).Return(errors.New("delete failed")).AnyTimes()
			if tt.name == "DeleteFromPersistentStoreFail" {
				mockStoreClient.EXPECT().DeleteSnapshot(coreCtx, gomock.Any()).Return(errors.New("delete failed"))
			} else {
				mockStoreClient.EXPECT().DeleteSnapshot(coreCtx, gomock.Any()).Return(nil).AnyTimes()
			}

			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			o.snapshots[snapID] = tt.snapshot
			if tt.name != "BackendNotFound2" {
				o.backends[backendUUID] = mockBackend
			}
			if tt.name != "VolumeNotFound2" {
				o.volumes[volName] = tt.volume
			}

			err := o.deleteSnapshot(coreCtx, snapConfig)
			assert.Error(t, err, "Unexpected error")
		})
	}
}

func TestHandleFailedSnapshot(t *testing.T) {
	backendUUID := "abcd"
	snapName := "snap"
	volName := "vol"
	snapID := storage.MakeSnapshotID(volName, snapName)
	snapConfig := &storage.SnapshotConfig{Name: snapName, VolumeName: volName}
	volConfig := &storage.VolumeConfig{Name: volName}
	vt := &storage.VolumeTransaction{Config: volConfig, SnapshotConfig: snapConfig}

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockBackend2 := mockstorage.NewMockBackend(mockCtrl)
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

	o := getOrchestrator(t, false)
	o.storeClient = mockStoreClient
	o.snapshots[snapID] = &storage.Snapshot{Config: snapConfig}
	o.backends["xyz"] = mockBackend2
	o.backends[backendUUID] = mockBackend
	o.volumes[volName] = &storage.Volume{Config: volConfig, BackendUUID: backendUUID}

	// storage.AddSnapshot switch case tests
	vt.Op = storage.AddSnapshot

	mockBackend.EXPECT().DeleteSnapshot(coreCtx, gomock.Any(),
		gomock.Any()).Return(errors.New("failed to delete snapshot"))
	mockBackend.EXPECT().Name().Return("abc")
	err := o.handleFailedTransaction(ctx(), vt)
	assert.Error(t, err, "Delete volume error")

	delete(o.snapshots, snapID)
	// As sequence of iteration in a map is not fixed, mockBackend2.State() may or may not get called
	mockBackend2.EXPECT().State().Return(storage.Unknown).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online)
	mockBackend.EXPECT().Name().Return("abc")
	mockBackend.EXPECT().DeleteSnapshot(coreCtx, gomock.Any(),
		gomock.Any()).Return(errors.New("failed to delete snapshot"))
	err = o.handleFailedTransaction(ctx(), vt)
	assert.Error(t, err, "Delete snapshot error")

	// DeleteSnapshot returns UnSupported error
	mockBackend2.EXPECT().State().Return(storage.Unknown).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online)
	mockBackend.EXPECT().DeleteSnapshot(coreCtx, gomock.Any(),
		gomock.Any()).Return(errors.UnsupportedError("failed to delete snapshot"))
	mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx,
		gomock.Any()).Return(errors.New("failed to delete transaction"))
	err = o.handleFailedTransaction(ctx(), vt)
	assert.Error(t, err, "Delete snapshot error")

	// DeleteSnapshot returns NotFound error
	mockBackend2.EXPECT().State().Return(storage.Unknown).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online)
	mockBackend.EXPECT().DeleteSnapshot(coreCtx, gomock.Any(),
		gomock.Any()).Return(errors.NotFoundError("failed to delete snapshot"))
	mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx,
		gomock.Any()).Return(errors.New("failed to delete transaction"))
	err = o.handleFailedTransaction(ctx(), vt)
	assert.Error(t, err, "Delete snapshot error")

	delete(o.backends, "xyz")
	mockBackend.EXPECT().State().Return(storage.Online)
	mockBackend.EXPECT().DeleteSnapshot(coreCtx, gomock.Any(), gomock.Any()).Return(nil)
	mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx,
		gomock.Any()).Return(errors.New("failed to delete transaction"))
	err = o.handleFailedTransaction(ctx(), vt)
	assert.Error(t, err, "Delete volume transaction error")

	// storage.DeleteSnapshot switch case tests
	vt.Op = storage.DeleteSnapshot

	o.snapshots[snapID] = &storage.Snapshot{Config: snapConfig}
	mockBackend.EXPECT().DeleteSnapshot(coreCtx, gomock.Any(),
		gomock.Any()).Return(errors.New("failed to delete snapshot"))
	mockBackend.EXPECT().Name().Return("abc")
	mockStoreClient.EXPECT().DeleteVolumeTransaction(coreCtx,
		gomock.Any()).Return(errors.New("failed to delete transaction"))
	err = o.handleFailedTransaction(ctx(), vt)
	assert.Error(t, err, "Delete volume transaction error")
}

func TestDeleteMountedSnapshot(t *testing.T) {
	volName := "vol"
	snapName := "snap"
	snapshot := &storage.Snapshot{Config: &storage.SnapshotConfig{Name: snapName, VolumeName: volName}}
	snapID := storage.MakeSnapshotID(volName, snapName)
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{Name: volName, ReadOnlyClone: true, CloneSourceSnapshot: snapName},
	}

	o := getOrchestrator(t, false)
	o.volumes[volName] = volume
	o.snapshots[snapID] = snapshot

	err := o.DeleteSnapshot(ctx(), volName, snapName)
	assert.Error(t, err, "An error is expected")
	assert.Equal(t, err.Error(), "unable to delete snapshot snap as it is a source for read-only clone vol")
}

func TestListLogWorkflows(t *testing.T) {
	o := getOrchestrator(t, false)

	flows, err := o.ListLoggingWorkflows(ctx())
	expected := []string{
		"backend=create,delete,get,list,update", "controller=get_capabilities,publish,unpublish",
		"core=bootstrap,init,node_reconcile,version", "cr=reconcile", "crd_controller=create", "grpc=trace",
		"k8s_client=trace_api,trace_factory", "node=create,delete,get,get_capabilities,get_info,get_response,list,update",
		"node_server=publish,stage,unpublish,unstage", "plugin=activate,create,deactivate,get,list",
		"snapshot=clone_from,create,delete,get,list,update", "storage_class=create,delete,get,list,update",
		"storage_client=create", "trident_rest=logger",
		"volume=clone,create,delete,get,get_capabilities,get_path,get_stats,import,list,mount,resize,unmount,update,upgrade",
	}
	assert.Equal(t, expected, flows)
	assert.NoError(t, err)
}

func TestListLogLayers(t *testing.T) {
	o := getOrchestrator(t, false)

	layers, err := o.ListLogLayers(ctx())
	expected := []string{
		"all", "azure-netapp-files", "core", "crd_frontend", "csi_frontend", "docker_frontend",
		"fake", "gcp-cvs", "ontap-nas", "ontap-nas-economy", "ontap-nas-flexgroup",
		"ontap-san", "ontap-san-economy", "persistent_store", "rest_frontend", "solidfire-san",
	}
	assert.Equal(t, expected, layers)
	assert.NoError(t, err)
}

func TestSetGetLogLevel(t *testing.T) {
	o := getOrchestrator(t, false)

	level, _ := o.GetLogLevel(ctx())
	defer func(level string) { _ = o.SetLogLevel(ctx(), level) }(level)

	err := o.SetLogLevel(ctx(), "trace")
	assert.NoError(t, err)

	level, err = o.GetLogLevel(ctx())
	assert.Equal(t, "trace", level)
	assert.NoError(t, err)

	err = o.SetLogLevel(ctx(), "foo")
	assert.Error(t, err)
}

func TestSetGetLogWorkflows(t *testing.T) {
	o := getOrchestrator(t, false)

	flows, err := o.GetSelectedLoggingWorkflows(ctx())
	assert.Equal(t, "", flows)
	assert.NoError(t, err)

	err = o.SetLoggingWorkflows(ctx(), "core=init:trident_rest=logger")
	assert.NoError(t, err)

	flows, err = o.GetSelectedLoggingWorkflows(ctx())
	assert.Equal(t, "core=init:trident_rest=logger", flows)
	assert.NoError(t, err)

	err = o.SetLoggingWorkflows(ctx(), "foo=bar")
	assert.Error(t, err)
}

func TestSetGetLogLayers(t *testing.T) {
	o := getOrchestrator(t, false)

	layers, err := o.GetSelectedLogLayers(ctx())
	assert.Equal(t, "", layers)
	assert.NoError(t, err)

	err = o.SetLogLayers(ctx(), "core,csi_frontend")
	assert.NoError(t, err)

	layers, err = o.GetSelectedLogLayers(ctx())
	assert.Equal(t, "core,csi_frontend", layers)
	assert.NoError(t, err)

	err = o.SetLogLayers(ctx(), "foo")
	assert.Error(t, err)
}

func TestUpdateBackendByBackendUUID(t *testing.T) {
	bName := "fake-backend"
	bConfig := map[string]interface{}{
		"version":           1,
		"storageDriverName": "fake",
		"backendName":       bName,
		"protocol":          config.File,
	}

	tests := []struct {
		name             string
		bootstrapErr     error
		backendName      string
		newBackendConfig map[string]interface{}
		contextValue     string
		callingConfigRef string
		mocks            func(mockStoreClient *mockpersistentstore.MockStoreClient)
		wantErr          assert.ErrorAssertionFunc
	}{
		{
			name:             "BootstrapError",
			bootstrapErr:     errors.New("bootstrap error"),
			backendName:      bName,
			newBackendConfig: bConfig,
			mocks:            func(mockStoreClient *mockpersistentstore.MockStoreClient) {},
			wantErr:          assert.Error,
		},
		{
			name:             "BackendNotFound",
			backendName:      bName,
			newBackendConfig: bConfig,
			mocks:            func(mockStoreClient *mockpersistentstore.MockStoreClient) {},
			wantErr:          assert.Error,
		},
		{
			name:             "BackendCRError",
			backendName:      bName,
			newBackendConfig: bConfig,
			mocks:            func(mockStoreClient *mockpersistentstore.MockStoreClient) {},
			wantErr:          assert.Error,
		},
		{
			name:             "InvalidCallingConfig",
			backendName:      bName,
			newBackendConfig: bConfig,
			callingConfigRef: "test",
			mocks:            func(mockStoreClient *mockpersistentstore.MockStoreClient) {},
			wantErr:          assert.Error,
		},
		{
			name:        "BadCredentials",
			backendName: bName,
			newBackendConfig: map[string]interface{}{
				"version": 1, "storageDriverName": "fake", "backendName": bName,
				"username": "", "protocol": config.File,
			},
			contextValue:     logging.ContextSourceCRD,
			callingConfigRef: "test",
			mocks:            func(mockStoreClient *mockpersistentstore.MockStoreClient) {},
			wantErr:          assert.Error,
		},
		// TODO: (victorir) to unblock updating a backend 25.02. Needs refactoring.
		// {
		// 	name:        "UpdateStoragePrefixError",
		// 	backendName: bName,
		// 	newBackendConfig: map[string]interface{}{
		// 		"version": 1, "storageDriverName": "fake", "backendName": bName,
		// 		"storagePrefix": "new", "protocol": config.File,
		// 	},
		// 	mocks:   func(mockStoreClient *mockpersistentstore.MockStoreClient) {},
		// 	wantErr: assert.Error,
		// },
		{
			name:        "BackendRenameWithExistingNameError",
			backendName: bName,
			newBackendConfig: map[string]interface{}{
				"version": 1, "storageDriverName": "fake",
				"backendName": "new", "protocol": config.File,
			},
			mocks:   func(mockStoreClient *mockpersistentstore.MockStoreClient) {},
			wantErr: assert.Error,
		},
		{
			name:        "BackendRenameError",
			backendName: bName,
			newBackendConfig: map[string]interface{}{
				"version": 1, "storageDriverName": "fake",
				"backendName": "new", "protocol": config.File,
			},
			mocks: func(mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().ReplaceBackendAndUpdateVolumes(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("rename error"))
			},
			wantErr: assert.Error,
		},
		{
			name:        "InvalidUpdateError",
			backendName: bName,
			newBackendConfig: map[string]interface{}{
				"version": 1, "storageDriverName": "fake",
				"backendName": bName, "protocol": config.Block,
			},
			mocks:   func(mockStoreClient *mockpersistentstore.MockStoreClient) {},
			wantErr: assert.Error,
		},
		{
			name:             "DefaultUpdateError",
			backendName:      bName,
			newBackendConfig: bConfig,
			mocks: func(mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).
					Return(errors.New("error updating backend"))
			},
			wantErr: assert.Error,
		},
		{
			name:        "UpdateVolumeAccessError",
			backendName: bName,
			newBackendConfig: map[string]interface{}{
				"version": 1, "storageDriverName": "fake",
				"backendName": bName, "volumeAccess": "1.1.1.1", "protocol": config.File,
			},
			mocks:   func(mockStoreClient *mockpersistentstore.MockStoreClient) {},
			wantErr: assert.Error,
		},
		{
			name:             "UpdateNonOrphanVolumeError",
			backendName:      bName,
			newBackendConfig: bConfig,
			mocks: func(mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil)
				mockStoreClient.EXPECT().UpdateVolume(gomock.Any(),
					gomock.Any()).Return(errors.New("error updating non-orphan volume"))
			},
			wantErr: assert.Error,
		},
		{
			name:             "BackendUpdateSuccess",
			backendName:      bName,
			newBackendConfig: bConfig,
			mocks: func(mockStoreClient *mockpersistentstore.MockStoreClient) {
				mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil)
			},
			wantErr: assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var oldBackend storage.Backend
			var oldBackendExt *storage.BackendExternal
			var configJSON []byte
			var backendUUID string
			var err error

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			mockStoreClient.EXPECT().AddBackend(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			configJSON, err = json.Marshal(bConfig)
			if err != nil {
				t.Fatal("failed to unmarshal", err)
			}

			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient
			if tt.name != "BackendNotFound" {
				if oldBackendExt, err = o.AddBackend(ctx(), string(configJSON), ""); err != nil {
					t.Fatal("unable to create mock backend: ", err)
				}
				backendUUID = oldBackendExt.BackendUUID
				if tt.name == "UpdateNonOrphanVolumeError" {
					o.volumes["vol1"] = &storage.Volume{
						Config:      &storage.VolumeConfig{InternalName: "vol1"},
						BackendUUID: backendUUID, Orphaned: false,
					}
				}
			}

			o.bootstrapError = tt.bootstrapErr
			if tt.name == "BackendCRError" {
				// Use case where Backend ConfigRef is non-empty
				oldBackend, _ = o.getBackendByBackendUUID(backendUUID)
				oldBackend.SetConfigRef("test")
			}

			configJSON, err = json.Marshal(tt.newBackendConfig)
			if err != nil {
				t.Fatal("failed to unmarshal newBackendConfig", err)
			}

			if tt.name == "BackendRenameWithExistingNameError" || tt.name == "UpdateVolumeAccessWithExistingNameError" {
				// Adding backend with the same name as newBackendConfig to get this error
				if _, err = o.AddBackend(ctx(), string(configJSON), ""); err != nil {
					t.Fatal("unable to create mock backend: ", err)
				}
			}

			tt.mocks(mockStoreClient)
			c := context.WithValue(ctx(), logging.ContextKeyRequestSource, tt.contextValue)

			_, err = o.UpdateBackendByBackendUUID(c, bName, string(configJSON), backendUUID, tt.callingConfigRef)
			tt.wantErr(t, err, "Unexpected result")
		})
	}
}

func TestReconcileVolumePublications_AddsPublicationForLegacyVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

	// Set up fake values for testing against.
	ctx := context.TODO()
	legacyVolumeName := "foo"
	standardVolumeName := "bar"
	nodeName := "baz"
	attachedLegacyVolumes := []*models.VolumePublicationExternal{
		{
			Name:       models.GenerateVolumePublishName(legacyVolumeName, nodeName),
			NodeName:   nodeName,
			VolumeName: legacyVolumeName,
			ReadOnly:   true,
			AccessMode: 2,
		},
		{
			Name:       models.GenerateVolumePublishName(standardVolumeName, nodeName),
			NodeName:   nodeName,
			VolumeName: standardVolumeName,
		},
	}
	legacyVolume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name: legacyVolumeName,
			AccessInfo: models.VolumeAccessInfo{
				ReadOnly:   true,
				AccessMode: 2,
			},
			SubordinateVolumes: nil,
		},
	}
	standardVolume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name: standardVolumeName,
			AccessInfo: models.VolumeAccessInfo{
				ReadOnly:   false,
				AccessMode: 6,
			},
			SubordinateVolumes: nil,
		},
	}
	standardPublication := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(standardVolumeName, nodeName),
		NodeName:   nodeName,
		VolumeName: standardVolumeName,
		ReadOnly:   false,
		AccessMode: 6,
	}
	storeVersion := &config.PersistentStateVersion{
		PersistentStoreVersion: string(persistentstore.CRDV1Store),
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		PublicationsSynced:     false,
	}

	// Set up caches and inject mocks to test if a publication is created or the legacy volume.
	o := getOrchestrator(t, false)
	o.storeClient = mockStoreClient
	o.volumes[legacyVolumeName] = legacyVolume
	o.volumes[standardVolumeName] = standardVolume
	err := o.volumePublications.Set(standardVolumeName, nodeName, standardPublication)
	assert.NoError(t, err)

	// Set up expected mock calls.
	// Only 1 legacy volume should exist.
	mockStoreClient.EXPECT().AddVolumePublication(ctx, gomock.Any()).Return(nil).Times(1)
	mockStoreClient.EXPECT().GetVersion(ctx).Return(storeVersion, nil).Times(1)
	storeVersion.PublicationsSynced = true
	mockStoreClient.EXPECT().SetVersion(ctx, storeVersion).Return(nil).Times(1)
	err = o.ReconcileVolumePublications(ctx, attachedLegacyVolumes)
	assert.NoError(t, err)

	// Get the publications from the cache.
	cachedPublications := o.volumePublications.Map()
	assert.NotEmpty(t, cachedPublications)

	// Get the legacy volumes publication and ensure it contains the expected values.
	existingStandardPublication, ok := cachedPublications[standardVolumeName][nodeName]
	assert.True(t, ok)
	assert.Equal(t, standardPublication.AccessMode, existingStandardPublication.AccessMode)
	assert.Equal(t, standardPublication.ReadOnly, existingStandardPublication.ReadOnly)

	freshPublication, ok := cachedPublications[legacyVolumeName][nodeName]
	assert.True(t, ok)
	assert.Equal(t, legacyVolume.Config.AccessInfo.AccessMode, freshPublication.AccessMode)
	assert.Equal(t, legacyVolume.Config.AccessInfo.ReadOnly, freshPublication.ReadOnly)

	// Expect this to be true.
	assert.True(t, o.volumePublicationsSynced)
}

func TestReconcileVolumePublications_SupportsMultiAttachedLegacyVolumes(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

	// Set up fake values for testing against.
	ctx := context.TODO()
	volumeName := "foo"
	nodeNameOne := "bar"
	nodeNameTwo := "baz"
	attachedLegacyVolumeOne := &models.VolumePublicationExternal{
		Name:       models.GenerateVolumePublishName(volumeName, nodeNameOne),
		NodeName:   nodeNameOne,
		VolumeName: volumeName,
		ReadOnly:   false,
		AccessMode: 5,
	}
	attachedLegacyVolumeTwo := &models.VolumePublicationExternal{
		Name:       models.GenerateVolumePublishName(volumeName, nodeNameTwo),
		NodeName:   nodeNameTwo,
		VolumeName: volumeName,
		ReadOnly:   false,
		AccessMode: 5,
	}
	attachedLegacyVolumeThree := &models.VolumePublicationExternal{
		Name:       models.GenerateVolumePublishName(volumeName, nodeNameTwo),
		NodeName:   "buz",
		VolumeName: volumeName,
		ReadOnly:   false,
		AccessMode: 5,
	}

	// Simulate multi-attached volumes. 1 of these has a publication, 2 lack one.
	attachedLegacyVolumes := []*models.VolumePublicationExternal{
		attachedLegacyVolumeOne,
		attachedLegacyVolumeTwo,
		attachedLegacyVolumeThree,
	}
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name: volumeName,
			AccessInfo: models.VolumeAccessInfo{
				ReadOnly:   false,
				AccessMode: 5,
			},
		},
	}

	// Suppose the volume supports RWX, but Trident lacks a publication record for the publication on node two.
	// AccessMode - https://github.com/container-storage-interface/spec/blob/release-1.5/lib/go/csi/csi.pb.go#L152
	publication := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeName, nodeNameTwo),
		NodeName:   nodeNameTwo,
		VolumeName: volumeName,
		ReadOnly:   false,
		AccessMode: 5,
	}
	storeVersion := &config.PersistentStateVersion{
		PersistentStoreVersion: string(persistentstore.CRDV1Store),
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		PublicationsSynced:     false,
	}

	// Set up caches and inject mocks to test if a publication is created or the legacy volume.
	o := getOrchestrator(t, false)
	o.storeClient = mockStoreClient
	o.volumes[volumeName] = volume
	err := o.volumePublications.Set(publication.VolumeName, publication.NodeName, publication)
	assert.NoError(t, err)

	// Set up expected mock calls.
	mockStoreClient.EXPECT().AddVolumePublication(ctx, gomock.Any()).Return(nil).AnyTimes()
	mockStoreClient.EXPECT().GetVersion(ctx).Return(storeVersion, nil).Times(1)
	storeVersion.PublicationsSynced = true
	mockStoreClient.EXPECT().SetVersion(ctx, storeVersion).Return(nil).Times(1)
	err = o.ReconcileVolumePublications(ctx, attachedLegacyVolumes)
	assert.NoError(t, err)

	// Get the publications from the cache.
	cachedPublications := o.volumePublications.Map()
	assert.NotEmpty(t, cachedPublications)

	// This is multi-attached volume, so all publications can have the same access info.
	for volName, nodesToPublications := range cachedPublications {
		assert.Equal(t, volName, volume.Config.Name)
		for _, publication := range nodesToPublications {
			assert.NotNil(t, publication)
			assert.Equal(t, publication.ReadOnly, volume.Config.AccessInfo.ReadOnly)
			assert.Equal(t, publication.AccessMode, volume.Config.AccessInfo.AccessMode)
		}
	}

	// Expect this to be true.
	assert.True(t, o.volumePublicationsSynced)
}

func TestReconcileVolumePublications_FailsToAddMissingVolumePublications(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

	// Set up fake values for testing against.
	ctx := context.TODO()
	volumeName := "foo"
	nodeNameOne := "bar"
	nodeNameTwo := "baz"
	attachedLegacyVolumeOne := &models.VolumePublicationExternal{
		Name:       models.GenerateVolumePublishName(volumeName, nodeNameOne),
		NodeName:   nodeNameOne,
		VolumeName: volumeName,
	}
	attachedLegacyVolumeTwo := &models.VolumePublicationExternal{
		Name:       models.GenerateVolumePublishName(volumeName, nodeNameTwo),
		NodeName:   nodeNameTwo,
		VolumeName: volumeName,
	}
	attachedLegacyVolumeWithNoTridentVolume := &models.VolumePublicationExternal{
		Name:       models.GenerateVolumePublishName("buz", nodeNameTwo),
		NodeName:   nodeNameTwo,
		VolumeName: "buz",
	}

	attachedLegacyVolumes := []*models.VolumePublicationExternal{
		attachedLegacyVolumeOne,
		attachedLegacyVolumeTwo,
		attachedLegacyVolumeWithNoTridentVolume,
	}
	volume := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name: volumeName,
			AccessInfo: models.VolumeAccessInfo{
				ReadOnly:   false,
				AccessMode: 5,
			},
		},
	}
	publication := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeName, nodeNameTwo),
		NodeName:   nodeNameTwo,
		VolumeName: volumeName,
		ReadOnly:   false,
		AccessMode: 5,
	}

	// Set up caches and inject mocks to test if a publication is created or the legacy volume.
	o := getOrchestrator(t, false)
	// Reset this to false to ensure reconcile fails to update it.
	o.volumePublicationsSynced = false
	o.storeClient = mockStoreClient
	o.volumes[volumeName] = volume
	err := o.volumePublications.Set(publication.VolumeName, publication.NodeName, publication)
	assert.NoError(t, err)

	// Set up expected mock calls.
	mockStoreClient.EXPECT().AddVolumePublication(ctx,
		gomock.Any()).Return(fmt.Errorf("store error")).Times(len(attachedLegacyVolumes) - 1)
	err = o.ReconcileVolumePublications(ctx, attachedLegacyVolumes)
	assert.Error(t, err)

	// Get the publications from the cache.
	cachedPublications := o.volumePublications.Map()
	assert.NotEmpty(t, cachedPublications)

	// The legacy attached volume shouldn't have been added to the cache if there was a store error.
	_, ok := cachedPublications[attachedLegacyVolumeOne.Name][attachedLegacyVolumeOne.NodeName]
	assert.False(t, ok)
	assert.False(t, o.volumePublicationsSynced)
}

func TestReconcileVolumePublications_FailsWithBootstrapError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

	// Set up fake values for testing against.
	ctx := context.TODO()
	attachedLegacyVolumes := []*models.VolumePublicationExternal{}

	// Set up caches and inject mocks to test if a publication is created or the legacy volume.
	o := getOrchestrator(t, false)
	o.storeClient = mockStoreClient
	o.bootstrapError = errors.NotReadyError()
	o.volumePublicationsSynced = false

	// Set up expected mock calls.
	mockStoreClient.EXPECT().AddVolumePublication(ctx, gomock.Any()).Return(nil).Times(0)
	err := o.ReconcileVolumePublications(ctx, attachedLegacyVolumes)
	assert.Error(t, err)
	assert.False(t, o.volumePublicationsSynced)
}

func TestReconcileVolumePublications_WhenNoLegacyAttachedVolumesExist(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)

	// Set up fake values for testing against.
	ctx := context.TODO()
	attachedLegacyVolumes := []*models.VolumePublicationExternal{}
	storeVersion := &config.PersistentStateVersion{
		PersistentStoreVersion: string(persistentstore.CRDV1Store),
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		PublicationsSynced:     false,
	}

	// Set up caches and inject mocks to test if a publication is created or the legacy volume.
	o := getOrchestrator(t, false)
	o.storeClient = mockStoreClient

	// Set up expected mock calls.
	mockStoreClient.EXPECT().AddVolumePublication(ctx, gomock.Any()).Return(nil).Times(0)
	mockStoreClient.EXPECT().GetVersion(ctx).Return(storeVersion, nil).Times(1)
	storeVersion.PublicationsSynced = true
	mockStoreClient.EXPECT().SetVersion(ctx, storeVersion).Return(nil).Times(1)
	err := o.ReconcileVolumePublications(ctx, attachedLegacyVolumes)
	assert.NoError(t, err)
	assert.True(t, o.volumePublicationsSynced)
}

func TestUpdatePublicationSyncStatus(t *testing.T) {
	storeErr := errors.New("store error")
	storeVersion := &config.PersistentStateVersion{
		PersistentStoreVersion: string(persistentstore.CRDV1Store),
		OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		PublicationsSynced:     false,
	}
	tests := map[string]struct {
		mocks      func(s *mockpersistentstore.MockStoreClient)
		shouldFail bool
	}{
		"when store client can't get the version": {
			func(s *mockpersistentstore.MockStoreClient) {
				s.EXPECT().GetVersion(gomock.Any()).Return(storeVersion, storeErr).Times(1)
			},
			true,
		},
		"when store client can't set the version": {
			func(s *mockpersistentstore.MockStoreClient) {
				s.EXPECT().GetVersion(gomock.Any()).Return(storeVersion, nil).Times(1)
				storeVersion.PublicationsSynced = true
				s.EXPECT().SetVersion(gomock.Any(), storeVersion).Return(storeErr).Times(1)
			},
			true,
		},
		"when store client does can get and set the version": {
			func(s *mockpersistentstore.MockStoreClient) {
				s.EXPECT().GetVersion(gomock.Any()).Return(storeVersion, nil).Times(1)
				storeVersion.PublicationsSynced = true
				s.EXPECT().SetVersion(gomock.Any(), storeVersion).Return(nil).Times(1)
			},
			false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			defer func(c *config.PersistentStateVersion) {
				storeVersion = c
			}(storeVersion)

			mockCtrl := gomock.NewController(t)
			mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
			test.mocks(mockStoreClient)

			// Set up caches and inject mocks to test if a publication is created or the legacy volume.
			o := getOrchestrator(t, false)
			o.storeClient = mockStoreClient

			err := o.setPublicationsSynced(context.Background(), test.shouldFail)
			assert.Equal(t, o.volumePublicationsSynced, test.shouldFail)
			if test.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPublishedNodesForBackend(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	o := getOrchestrator(t, false)

	// orchestrator has nodeA and nodeB, backend has published to nodeA
	o.nodes.Set("nodeA", &models.Node{Name: "nodeA"})
	o.nodes.Set("nodeB", &models.Node{Name: "nodeB"})
	o.volumePublications.Set("vol1", "nodeA", &models.VolumePublication{
		NodeName:   "nodeA",
		VolumeName: "vol1",
	})
	o.volumePublications.Set("vol3", "nodeB", &models.VolumePublication{
		NodeName:   "nodeB",
		VolumeName: "vol3",
	})
	mockBackend.EXPECT().Volumes().Return(map[string]*storage.Volume{
		"vol1": {Config: &storage.VolumeConfig{Name: "vol1", ExportPolicy: "pol1"}},
		"vol2": {Config: &storage.VolumeConfig{Name: "vol2", ExportPolicy: "pol2"}},
	})
	expectedNodes := []*models.Node{o.nodes.Get("nodeA")}

	actualNodes := o.publishedNodesForBackend(mockBackend)
	assert.Equal(t, expectedNodes, actualNodes)
}

func TestVolumePublicationsForBackend(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	o := getOrchestrator(t, false)

	// orchestrator has nodeA and nodeB, backend has published to nodeA
	o.nodes.Set("nodeA", &models.Node{Name: "nodeA"})
	o.nodes.Set("nodeB", &models.Node{Name: "nodeB"})
	o.volumePublications.Set("vol1", "nodeA", &models.VolumePublication{
		NodeName:   "nodeA",
		VolumeName: "vol1",
	})
	o.volumePublications.Set("vol3", "nodeB", &models.VolumePublication{
		NodeName:   "nodeB",
		VolumeName: "vol3",
	})
	mockBackend.EXPECT().Volumes().Return(map[string]*storage.Volume{
		"vol1": {Config: &storage.VolumeConfig{Name: "vol1", ExportPolicy: "pol1"}},
		"vol2": {Config: &storage.VolumeConfig{Name: "vol2", ExportPolicy: "pol2"}},
	})

	expectedVolToPubs := map[string][]*models.VolumePublication{
		"vol1": {
			{
				NodeName:   "nodeA",
				VolumeName: "vol1",
			},
		},
		"vol2": {},
	}

	actualVolToPubs := o.volumePublicationsForBackend(mockBackend)
	assert.Equal(t, expectedVolToPubs, actualVolToPubs)
}

func TestReconcileBackendState(t *testing.T) {
	// Set fake values
	backendUUID := "1234"
	backendName := "Fake Backend"
	testReason := "Test Reason"

	// Creating a roaring map
	changeMap := roaring.New()

	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	o := getOrchestrator(t, false)
	o.storeClient = mockStoreClient

	gomock.InOrder(
		mockBackend.EXPECT().CanGetState().Return(false),
		mockBackend.EXPECT().CanGetState().Return(true).AnyTimes(),
	)
	gomock.InOrder(
		// For Test 3
		mockBackend.EXPECT().GetBackendState(ctx()).Return("", nil),
		// For Test 4
		mockBackend.EXPECT().GetBackendState(ctx()).Return("", changeMap),
		// For Test 5
		mockBackend.EXPECT().GetBackendState(ctx()).Return(testReason, changeMap).Times(1),
		// For Test 6,7,8
		mockBackend.EXPECT().GetBackendState(ctx()).Return("", changeMap).AnyTimes(),
	)

	mockStoreClient.EXPECT().AddBackend(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()
	mockBackend.EXPECT().GetDriverName().Return("fake").AnyTimes()
	mockBackend.EXPECT().Name().Return(backendName).AnyTimes()
	mockBackend.EXPECT().State().Return(storage.Online).AnyTimes()

	mockUpdateError := fmt.Errorf("returning error on UpdateBackend")
	gomock.InOrder(
		// For Test 4
		mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(mockUpdateError),
		// For Test 5
		mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).Return(nil).AnyTimes(),
	)

	fakeConfig, err := fakedriver.NewFakeStorageDriverConfigJSONWithDebugTraceFlags(
		backendName, "",
		nil, "fakeBackend_",
	)
	if err != nil {
		t.Fatalf("Unable to generate config JSON for %s: %v", backendName, err)
	}

	// Adding that fakeBackend.
	_, err = o.AddBackend(ctx(), fakeConfig, "")
	if err != nil {
		t.Errorf("Unable to add backend %s: %v", backendName, err)
	}

	// Fetching the added backend.
	fakeBackend, err := o.getBackendByBackendName(backendName)
	if err != nil {
		t.Errorf("Unable to get backend %s: by its name, %v", backendName, err)
	}

	// Test 1 - unsupported driver
	err = o.reconcileBackendState(ctx(), mockBackend)
	assert.NoError(t, err, "should skip un-supportive drivers")

	// Test 2 - bootstrap error
	o.bootstrapError = fmt.Errorf("returning bootstrap error")
	err = o.reconcileBackendState(ctx(), mockBackend)
	assert.Error(t, err, "should return bootstrap error")

	o.bootstrapError = nil
	// Test 3 - changeMap nil
	err = o.reconcileBackendState(ctx(), mockBackend)
	assert.NoError(t, err, "should be no error")

	// Test 4 - changeMap contains reasonChange, nil reason, error on storeclient.UpdateBackend
	changeMap.Add(storage.BackendStateReasonChange)
	err = o.reconcileBackendState(ctx(), mockBackend)
	assert.Error(t, err, "should be error")

	// Test 5 - changeMap contains reasonChange, non-nil reason
	err = o.reconcileBackendState(ctx(), mockBackend)
	assert.NoError(t, err, "should be no error")

	commonConfig := fakeBackend.Driver().GetCommonConfig(ctx())
	commonConfig.BackendName = backendName

	fakeDriverConfig := &drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
	}
	out, _ := fakeDriverConfig.Marshal()

	mockBackend.EXPECT().MarshalDriverConfig().Return(out, nil).AnyTimes()

	// Test 6 - changeMap contains BackendStateReasonChange and BackendStatePoolsChange
	mockBackend.EXPECT().ConfigRef().Return("").Times(1)
	changeMap.Add(storage.BackendStatePoolsChange)
	err = o.reconcileBackendState(ctx(), mockBackend)
	assert.NoError(t, err, "should be no error")

	// Test 7 - changeMap contains BackendStateReasonChange, BackendStatePoolsChange and BackendStateAPIVersionChange
	mockBackend.EXPECT().ConfigRef().Return("").Times(1)
	changeMap.Add(storage.BackendStateAPIVersionChange)
	err = o.reconcileBackendState(ctx(), mockBackend)
	assert.NoError(t, err, "should be no error")

	// Test 8: configRef that is passed doesn't match the one that is stored in the backend.
	mockBackend.EXPECT().ConfigRef().Return("123456789").Times(1)
	err = o.reconcileBackendState(ctx(), mockBackend)
	assert.Error(t, err, "should be no error")
}

// TestPeriodicallyReconcileBackendState is majorly for code coverage, as all other called functions have respective
// unit tests and no need to test once again here.
func TestPeriodicallyReconcileBackendState(t *testing.T) {
	// Set fake values
	backendUUID := "1234"

	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	o := getOrchestrator(t, false)
	o.storeClient = mockStoreClient

	// Test 1: poll interval 0, would not create the loop
	o.PeriodicallyReconcileBackendState(0)
	assert.Nil(t, o.stopReconcileBackendLoop, "reconcile backend loop should not have started")

	// Test 2: with one backend added and interval 0.1s
	mockBackend.EXPECT().CanGetState().Return(true).MinTimes(1)
	mockBackend.EXPECT().Name().Return(backendUUID).MinTimes(1)
	o.bootstrapError = fmt.Errorf("test error")
	o.backends[backendUUID] = mockBackend
	go o.PeriodicallyReconcileBackendState(100 * time.Millisecond)

	// Wait for loop to run at least once.
	time.Sleep(1 * time.Second)

	// stop orchestrator
	o.Stop()
}

func TestUpdateMirror_BootstrapError(t *testing.T) {
	pvcVolumeName := "vol"
	snapshotName := "snapshot-123"

	o := getOrchestrator(t, false)
	o.bootstrapError = errors.New("failed")

	err := o.UpdateMirror(ctx(), pvcVolumeName, snapshotName)
	assert.Error(t, err, "UpdateMirror should have failed")
}

func TestUpdateMirror_VolumeNotFound(t *testing.T) {
	pvcVolumeName := "vol"
	snapshotName := "snapshot-123"

	o := getOrchestrator(t, false)

	err := o.UpdateMirror(ctx(), pvcVolumeName, snapshotName)
	assert.Error(t, err, "UpdateMirror should have failed")
}

func TestUpdateMirror_BackendNotFound(t *testing.T) {
	pvcVolumeName := "vol"
	snapshotName := ""

	o := getOrchestrator(t, false)
	vol := &storage.Volume{
		Config:      &storage.VolumeConfig{Name: pvcVolumeName},
		BackendUUID: "12345",
	}
	o.volumes[pvcVolumeName] = vol

	err := o.UpdateMirror(ctx(), pvcVolumeName, snapshotName)
	assert.Error(t, err, "UpdateMirror should have failed")
}

func TestUpdateMirror_BackendCannotMirror(t *testing.T) {
	pvcVolumeName := "vol"
	snapshotName := ""
	backendUUID := "12345"

	o := getOrchestrator(t, false)
	vol := &storage.Volume{
		Config:      &storage.VolumeConfig{Name: pvcVolumeName},
		BackendUUID: backendUUID,
	}
	o.volumes[pvcVolumeName] = vol

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	o.backends[backendUUID] = mockBackend

	mockBackend.EXPECT().GetDriverName().AnyTimes()
	mockBackend.EXPECT().State()
	mockBackend.EXPECT().Name()
	mockBackend.EXPECT().BackendUUID().Times(2)
	mockBackend.EXPECT().CanMirror().Return(false)

	err := o.UpdateMirror(ctx(), pvcVolumeName, snapshotName)
	assert.Error(t, err, "UpdateMirror should have failed")
}

func TestUpdateMirror_UpdateSucceeded(t *testing.T) {
	pvcVolumeName := "vol"
	snapshotName := ""
	backendUUID := "12345"
	internalName := "pvc_123"

	o := getOrchestrator(t, false)
	vol := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name:         pvcVolumeName,
			InternalName: internalName,
		},
		BackendUUID: backendUUID,
	}
	o.volumes[pvcVolumeName] = vol

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	o.backends[backendUUID] = mockBackend

	mockBackend.EXPECT().GetDriverName().Times(3)
	mockBackend.EXPECT().State()
	mockBackend.EXPECT().Name()
	mockBackend.EXPECT().BackendUUID().Times(2)
	mockBackend.EXPECT().CanMirror().Return(true)
	mockBackend.EXPECT().UpdateMirror(gomock.Any(), internalName, snapshotName)

	err := o.UpdateMirror(ctx(), pvcVolumeName, snapshotName)
	assert.NoError(t, err, "UpdateMirror should have succeeded")
}

func TestCheckMirrorTransferState_BootstrapError(t *testing.T) {
	pvcVolumeName := "vol"

	o := getOrchestrator(t, false)
	o.bootstrapError = errors.New("failed")

	_, err := o.CheckMirrorTransferState(ctx(), pvcVolumeName)
	assert.Error(t, err, "CheckMirrorTransferState should have failed")
}

func TestCheckMirrorTransferState_VolumeNotFound(t *testing.T) {
	pvcVolumeName := "vol"

	o := getOrchestrator(t, false)

	_, err := o.CheckMirrorTransferState(ctx(), pvcVolumeName)
	assert.Error(t, err, "CheckMirrorTransferState should have failed")
}

func TestCheckMirrorTransferState_BackendNotFound(t *testing.T) {
	pvcVolumeName := "vol"

	o := getOrchestrator(t, false)
	vol := &storage.Volume{
		Config:      &storage.VolumeConfig{Name: pvcVolumeName},
		BackendUUID: "12345",
	}
	o.volumes[pvcVolumeName] = vol

	_, err := o.CheckMirrorTransferState(ctx(), pvcVolumeName)
	assert.Error(t, err, "CheckMirrorTransferState should have failed")
}

func TestCheckMirrorTransferState_BackendCannotMirror(t *testing.T) {
	pvcVolumeName := "vol"
	backendUUID := "12345"

	o := getOrchestrator(t, false)
	vol := &storage.Volume{
		Config:      &storage.VolumeConfig{Name: pvcVolumeName},
		BackendUUID: backendUUID,
	}
	o.volumes[pvcVolumeName] = vol

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	o.backends[backendUUID] = mockBackend

	mockBackend.EXPECT().GetDriverName().AnyTimes()
	mockBackend.EXPECT().State()
	mockBackend.EXPECT().Name()
	mockBackend.EXPECT().BackendUUID().Times(2)
	mockBackend.EXPECT().CanMirror().Return(false)

	_, err := o.CheckMirrorTransferState(ctx(), pvcVolumeName)
	assert.Error(t, err, "CheckMirrorTransferState should have failed")
}

func TestCheckMirrorTransferState_CheckSucceeded(t *testing.T) {
	pvcVolumeName := "vol"
	backendUUID := "12345"
	internalName := "pvc_123"

	o := getOrchestrator(t, false)
	vol := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name:         pvcVolumeName,
			InternalName: internalName,
		},
		BackendUUID: backendUUID,
	}
	o.volumes[pvcVolumeName] = vol

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	o.backends[backendUUID] = mockBackend

	mockBackend.EXPECT().GetDriverName().AnyTimes()
	mockBackend.EXPECT().State()
	mockBackend.EXPECT().Name()
	mockBackend.EXPECT().BackendUUID().Times(2)
	mockBackend.EXPECT().CanMirror().Return(true)
	mockBackend.EXPECT().CheckMirrorTransferState(gomock.Any(), internalName)

	_, err := o.CheckMirrorTransferState(ctx(), pvcVolumeName)
	assert.NoError(t, err, "CheckMirrorTransferState should have succeeded")
}

func TestGetMirrorTransferTime_BootstrapError(t *testing.T) {
	pvcVolumeName := "vol"

	o := getOrchestrator(t, false)
	o.bootstrapError = errors.New("failed")

	_, err := o.GetMirrorTransferTime(ctx(), pvcVolumeName)
	assert.Error(t, err, "GetMirrorTransferTime should have failed")
}

func TestGetMirrorTransferTime_VolumeNotFound(t *testing.T) {
	pvcVolumeName := "vol"

	o := getOrchestrator(t, false)

	_, err := o.GetMirrorTransferTime(ctx(), pvcVolumeName)
	assert.Error(t, err, "GetMirrorTransferTime should have failed")
}

func TestGetMirrorTransferTime_BackendNotFound(t *testing.T) {
	pvcVolumeName := "vol"

	o := getOrchestrator(t, false)
	vol := &storage.Volume{
		Config:      &storage.VolumeConfig{Name: pvcVolumeName},
		BackendUUID: "12345",
	}
	o.volumes[pvcVolumeName] = vol

	_, err := o.GetMirrorTransferTime(ctx(), pvcVolumeName)
	assert.Error(t, err, "GetMirrorTransferTime should have failed")
}

func TestGetMirrorTransferTime_BackendCannotMirror(t *testing.T) {
	pvcVolumeName := "vol"
	backendUUID := "12345"

	o := getOrchestrator(t, false)
	vol := &storage.Volume{
		Config:      &storage.VolumeConfig{Name: pvcVolumeName},
		BackendUUID: backendUUID,
	}
	o.volumes[pvcVolumeName] = vol

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	o.backends[backendUUID] = mockBackend

	mockBackend.EXPECT().GetDriverName().AnyTimes()
	mockBackend.EXPECT().State()
	mockBackend.EXPECT().Name()
	mockBackend.EXPECT().BackendUUID().Times(2)
	mockBackend.EXPECT().CanMirror().Return(false)

	_, err := o.GetMirrorTransferTime(ctx(), pvcVolumeName)
	assert.Error(t, err, "GetMirrorTransferTime should have failed")
}

func TestGetMirrorTransferTime_CheckSucceeded(t *testing.T) {
	pvcVolumeName := "vol"
	backendUUID := "12345"
	internalName := "pvc_123"

	o := getOrchestrator(t, false)
	vol := &storage.Volume{
		Config: &storage.VolumeConfig{
			Name:         pvcVolumeName,
			InternalName: internalName,
		},
		BackendUUID: backendUUID,
	}
	o.volumes[pvcVolumeName] = vol

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	mockBackend := mockstorage.NewMockBackend(mockCtrl)

	o.backends[backendUUID] = mockBackend

	mockBackend.EXPECT().GetDriverName().AnyTimes()
	mockBackend.EXPECT().State()
	mockBackend.EXPECT().Name()
	mockBackend.EXPECT().BackendUUID().Times(2)
	mockBackend.EXPECT().CanMirror().Return(true)
	mockBackend.EXPECT().GetMirrorTransferTime(gomock.Any(), internalName)

	_, err := o.GetMirrorTransferTime(ctx(), pvcVolumeName)
	assert.NoError(t, err, "GetMirrorTransferTime should have succeeded")
}

func TestUpdateBackendState(t *testing.T) {
	// Setting up for the test cases.
	backendUUID := "1234"
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockBackend := mockstorage.NewMockBackend(mockCtrl)
	mockStoreClient := mockpersistentstore.NewMockStoreClient(mockCtrl)
	fakeStorageDriver := fakedriver.NewFakeStorageDriverWithDebugTraceFlags(nil)
	o := getOrchestrator(t, false)
	o.storeClient = mockStoreClient
	o.bootstrapError = nil
	o.backends[backendUUID] = mockBackend

	mockBackend.EXPECT().Name().Return("something").AnyTimes()
	mockBackend.EXPECT().Driver().Return(fakeStorageDriver).AnyTimes()
	mockBackend.EXPECT().GetDriverName().Return("ontap").AnyTimes()
	mockBackend.EXPECT().SetState(gomock.Any()).AnyTimes()
	mockBackend.EXPECT().SetUserState(gomock.Any()).AnyTimes()
	mockBackend.EXPECT().ConstructExternal(gomock.Any()).AnyTimes()
	mockBackend.EXPECT().Terminate(gomock.Any()).AnyTimes()
	mockStoreClient.EXPECT().UpdateBackend(gomock.Any(), gomock.Any()).AnyTimes()

	// Test 1 - where backendName is correct, but it returns a backendUUID which is not mapped in backends map.
	mockBackend.EXPECT().BackendUUID().Return("7890").Times(2)
	mockBackend.EXPECT().State().Return(storage.Online).Times(1)
	_, err := o.UpdateBackendState(ctx, "something", "", "suspended")
	assert.Error(t, err, "should return an error as backendUUID is not mapped in backends map")

	// From here on we need the following for every test cases.
	mockBackend.EXPECT().BackendUUID().Return(backendUUID).AnyTimes()

	// Test 2 - where we provide wrong backend name.
	mockBackend.EXPECT().State().Return(storage.Online).Times(1)
	_, err = o.UpdateBackendState(ctx, "fake", "", "suspended")
	assert.Error(t, err, "should return error backend not found")

	// Test 3 - where we provide states to both userBackendState and backendState.
	_, err = o.UpdateBackendState(ctx, "something", "suspended", "online")
	assert.Error(t, err, "should return error as we are trying to update both userBackendState and backendState")

	// Test 4 - where we don't provide any state.
	_, err = o.UpdateBackendState(ctx, "something", "", "")
	assert.Error(t, err, "should return error as we are not providing any state")

	// Test 5 - where we provide wrong backendState.
	mockBackend.EXPECT().State().Return(storage.Online).Times(1)
	_, err = o.UpdateBackendState(ctx, "something", "fake", "")
	assert.Error(t, err, "should return error, we are providing wrong backendState")

	// Test 6 - where we provide correct backendState.
	mockBackend.EXPECT().State().Return(storage.Failed).Times(1)
	_, err = o.UpdateBackendState(ctx, "something", "failed", "")
	assert.NoError(t, err, "should not return any error")

	// Test 7 - where we provide wrong userBackendState
	mockBackend.EXPECT().State().Return(storage.Online).Times(1)
	_, err = o.UpdateBackendState(ctx, "something", "", "fake")
	assert.Error(t, err, "should return error, we are providing wrong userBackendState")

	// Test 8 - we are trying to suspend a backend when backend itself is neither online,offline nor failed.
	mockBackend.EXPECT().State().Return(storage.Deleting).Times(5)
	mockBackend.EXPECT().UserState().Return(storage.UserNormal).Times(1)
	_, err = o.UpdateBackendState(ctx, "something", "", "suspended")
	assert.Error(t, err, "should return error, we are trying to suspend a backend which is neither online,offline or failed.")

	// Test 9 - set backend to userSuspended when state is online.
	mockBackend.EXPECT().State().Return(storage.Online).Times(2)
	mockBackend.EXPECT().UserState().Return(storage.UserNormal).Times(1)
	_, err = o.UpdateBackendState(ctx, "something", "", "suspended")
	assert.NoError(t, err, "should be no error")

	// Test 10 - set backend to userSuspended when state is offline.
	mockBackend.EXPECT().State().Return(storage.Offline).Times(3)
	mockBackend.EXPECT().UserState().Return(storage.UserNormal).Times(1)
	_, err = o.UpdateBackendState(ctx, "something", "", "Suspended")
	assert.NoError(t, err, "should be no error")

	// Test 11 - set backend to userSuspended when state is failed.
	mockBackend.EXPECT().State().Return(storage.Failed).Times(4)
	mockBackend.EXPECT().UserState().Return(storage.UserNormal).Times(1)
	_, err = o.UpdateBackendState(ctx, "something", "", "SUSpended")
	assert.NoError(t, err, "should be no error")

	// Test 12 - set backend to userNormal when it is in userSuspended and state is online.
	mockBackend.EXPECT().State().Return(storage.Online).Times(1)
	mockBackend.EXPECT().UserState().Return(storage.UserSuspended).Times(1)
	_, err = o.UpdateBackendState(ctx, "something", "", "nOrmal")
	assert.NoError(t, err, "should be no error")

	// Test 13 - idempotent check.
	mockBackend.EXPECT().State().Return(storage.Online).Times(1)
	mockBackend.EXPECT().UserState().Return(storage.UserSuspended).Times(1)
	_, err = o.UpdateBackendState(ctx, "something", "", "suspended")
	assert.NoError(t, err, "should be no error")

	// Test 14 - bootstrap error
	o.bootstrapError = fmt.Errorf("bootstrap error")
	_, err = o.UpdateBackendState(ctx, "something", "", "suspended")
	assert.Error(t, err, "should return error, bootstrap error")
	o.bootstrapError = nil

	// Test 15 - when commonConfig.userState in tbc is set
	fakeStorageDriver.Config.UserState = "suspended"
	mockBackend.EXPECT().State().Return(storage.Online).Times(1)
	mockBackend.EXPECT().ConfigRef().Return("1234").Times(1)
	_, err = o.UpdateBackendState(ctx, "something", "", "suspended")
	assert.Errorf(t, err, "should return error, userBackendState is set in commonConfig, so modifying via cli is not allowed")

	// Test 16 - when commonConfig.userState in tbe is set, and there's no tbc linked to this tbe yet.
	fakeStorageDriver.Config.UserState = "suspended"
	mockBackend.EXPECT().ConfigRef().Return("").Times(1)
	mockBackend.EXPECT().State().Return(storage.Online).Times(1)
	mockBackend.EXPECT().UserState().Return(storage.UserSuspended).Times(1)
	_, err = o.UpdateBackendState(ctx, "something", "", "suspended")
	assert.NoError(t, err, "update to userState via tridentctl should be allowed when there's no tbc linked to this tbe yet")
}
