// Copyright 2021 NetApp, Inc. All Rights Reserved.

package v1

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	fake_storage "github.com/netapp/trident/storage/fake"
	"github.com/netapp/trident/storage_drivers/fake"
	tu "github.com/netapp/trident/storage_drivers/fake/test_utils"
)

var (
	debug = flag.Bool("debug", false, "Enable debugging output")
	ctx   = context.Background
)

func init() {
	testing.Init()
	if *debug {
		_ = InitLogLevel("debug")
	}
}

func TestNewBackend(t *testing.T) {
	// Build backend
	mockPools := tu.GetFakePools()
	volumes := make([]fake_storage.Volume, 0)
	fakeConfig, err := fake.NewFakeStorageDriverConfigJSON("mock", config.File, mockPools, volumes)
	if err != nil {
		t.Fatal("Unable to construct config JSON.")
	}
	nfsServer, err := fake.NewFakeStorageBackend(ctx(), fakeConfig, uuid.New().String())
	if err != nil {
		t.Fatalf("Unable to create fake storage backend: %v", err)
	}

	// Convert to Kubernetes Object using the NewTridentBackend method
	backend, err := NewTridentBackend(ctx(), nfsServer.ConstructPersistent(ctx()))
	if err != nil {
		t.Fatalf("Unable to construct TridentBackend CRD: %v", err)
	}

	// Build expected result
	expected := &TridentBackend{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentBackend",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tbe-",
		},
		BackendName: nfsServer.Name(),
		Online:      true,
		Version:     "1",
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(nfsServer.ConstructPersistent(ctx()).Config)),
		},
	}
	expected.ObjectMeta.Name = backend.ObjectMeta.Name
	expected.BackendUUID = backend.BackendUUID
	expected.State = "online"

	if expected.ObjectMeta.Name != backend.ObjectMeta.Name {
		t.Fatalf("%v differs:  '%v' != '%v'", "ObjectMeta.Name", expected.ObjectMeta.Name, backend.ObjectMeta.Name)
	}
	if expected.TypeMeta.APIVersion != backend.TypeMeta.APIVersion {
		t.Fatalf("%v differs:  '%v' != '%v'", "TypeMeta.APIVersion", expected.TypeMeta.APIVersion, backend.TypeMeta.APIVersion)
	}
	if expected.TypeMeta.Kind != backend.TypeMeta.Kind {
		t.Fatalf("%v differs:  '%v' != '%v'", "TypeMeta.Kind", expected.TypeMeta.Kind, backend.TypeMeta.Kind)
	}
	if expected.Name != backend.Name {
		t.Fatalf("%v differs:  '%v' != '%v'", "Name", expected.Name, backend.Name)
	}
	if expected.BackendUUID != backend.BackendUUID {
		t.Fatalf("%v differs:  '%v' != '%v'", "BackendUUID", expected.BackendUUID, backend.BackendUUID)
	}
	if expected.State != backend.State {
		t.Fatalf("%v differs:  '%v' != '%v'", "State", expected.State, backend.State)
	}
	if expected.Config.String() != backend.Config.String() {
		t.Fatalf("%v differs:  '%v' != '%v'", "Config", expected.Config.String(), backend.Config.String())
	}
}

func backendWithDataLIFs(dataLIFs *[]string) *TridentBackend {
	if dataLIFs == nil {
		return &TridentBackend{}
	}
	return &TridentBackend{DiscoveredState: &TridentBackendDiscoveredState{DataLIFs: dataLIFs}}
}

func TestTridentBackendApply_DataLIFs(t *testing.T) {
	published := []string{"192.0.2.10", "192.0.2.11"}
	refreshed := []string{"192.0.2.12"}

	tests := []struct {
		name     string
		existing *[]string
		update   *[]string
		expected *[]string
	}{
		{name: "first snapshot is stored", update: &refreshed, expected: &refreshed},
		{name: "new snapshot replaces the old one", existing: &published, update: &refreshed, expected: &refreshed},
		{name: "empty snapshot is stored as empty", existing: &published, update: &[]string{}, expected: &[]string{}},
		{name: "no snapshot keeps the published one", existing: &published, expected: &published},
		{name: "no snapshot and nothing published", expected: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			backend := backendWithDataLIFs(tc.existing)

			err := backend.Apply(ctx(), &storage.BackendPersistent{Name: "backend", DataLIFs: tc.update})

			require.NoError(t, err)
			assert.Equal(t, tc.expected, backend.PublishedDataLIFs())
		})
	}
}

func TestTridentBackendApply_NoSnapshotDoesNotCreateDiscoveredState(t *testing.T) {
	backend := &TridentBackend{}

	require.NoError(t, backend.Apply(ctx(), &storage.BackendPersistent{Name: "backend"}))

	assert.Nil(t, backend.DiscoveredState)
}

func TestTridentBackendApply_DataLIFsAreCopied(t *testing.T) {
	update := []string{"192.0.2.10"}
	backend := &TridentBackend{}

	require.NoError(t, backend.Apply(ctx(), &storage.BackendPersistent{Name: "backend", DataLIFs: &update}))
	update[0] = "mutated"

	assert.Equal(t, []string{"192.0.2.10"}, *backend.PublishedDataLIFs())
}

func TestTridentBackendPersistent_DataLIFs(t *testing.T) {
	published := []string{"192.0.2.10", "192.0.2.11"}
	backend := &TridentBackend{BackendName: "backend", Config: runtime.RawExtension{Raw: []byte("{}")}}

	persistent, err := backend.Persistent()
	require.NoError(t, err)
	assert.Nil(t, persistent.DataLIFs)

	backend.DiscoveredState = &TridentBackendDiscoveredState{}
	persistent, err = backend.Persistent()
	require.NoError(t, err)
	assert.Nil(t, persistent.DataLIFs)

	backend.DiscoveredState.DataLIFs = &published
	persistent, err = backend.Persistent()
	require.NoError(t, err)
	require.NotNil(t, persistent.DataLIFs)
	assert.Equal(t, published, *persistent.DataLIFs)

	(*persistent.DataLIFs)[0] = "mutated"
	assert.Equal(t, "192.0.2.10", published[0], "Persistent must copy the snapshot")
}

func TestTridentBackendDeepCopy_DataLIFs(t *testing.T) {
	published := []string{"192.0.2.10"}
	backend := backendWithDataLIFs(&published)

	copied := backend.DeepCopy()
	(*copied.DiscoveredState.DataLIFs)[0] = "mutated"

	assert.Equal(t, "192.0.2.10", published[0])
}

func TestTridentBackendDataLIFsJSONDistinguishesUnknownFromEmpty(t *testing.T) {
	marshal := func(backend *TridentBackend) map[string]any {
		t.Helper()
		raw, err := json.Marshal(backend)
		require.NoError(t, err)
		var object map[string]any
		require.NoError(t, json.Unmarshal(raw, &object))
		return object
	}

	empty := []string{}
	discovered, ok := marshal(backendWithDataLIFs(&empty))["discoveredState"].(map[string]any)
	require.True(t, ok, "expected discoveredState to be published")
	dataLIFs, ok := discovered["dataLIFs"].([]any)
	assert.True(t, ok && len(dataLIFs) == 0, "expected an explicit empty dataLIFs array, got %v", discovered)

	_, ok = marshal(&TridentBackend{})["discoveredState"]
	assert.False(t, ok, "expected unknown discoveredState to be omitted")

	discovered, ok = marshal(&TridentBackend{DiscoveredState: &TridentBackendDiscoveredState{}})["discoveredState"].(map[string]any)
	require.True(t, ok)
	_, ok = discovered["dataLIFs"]
	assert.False(t, ok, "expected unknown dataLIFs to be omitted")

	var roundTripped TridentBackend
	raw, err := json.Marshal(backendWithDataLIFs(&empty))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &roundTripped))
	require.NotNil(t, roundTripped.PublishedDataLIFs())
	assert.Empty(t, *roundTripped.PublishedDataLIFs())
}

func TestBackend_Persistent(t *testing.T) {
	// Build backend
	mockPools := tu.GetFakePools()
	volumes := make([]fake_storage.Volume, 0)
	fakeConfig, err := fake.NewFakeStorageDriverConfigJSON("mock", config.File, mockPools, volumes)
	if err != nil {
		t.Fatal("Unable to construct config JSON.")
	}
	nfsServer, err := fake.NewFakeStorageBackend(ctx(), fakeConfig, uuid.New().String())
	if err != nil {
		t.Fatalf("Unable to create fake storage backend: %v", err)
	}

	// Build Kubernetes Object
	backend := &TridentBackend{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentBackend",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(nfsServer.Name()),
		},
		BackendName: nfsServer.Name(),
		Online:      true,
		State:       "online",
		UserState:   "normal",
		Version:     "1",
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(nfsServer.ConstructPersistent(ctx()).Config)),
		},
	}

	// Build persistent object by calling TridentBackend.Persistent
	persistent, err := backend.Persistent()
	if err != nil {
		t.Fatal("Unable to construct TridentBackend persistent object: ", err)
	}

	// Build expected persistent object
	expected := nfsServer.ConstructPersistent(ctx())

	// Compare
	if !cmp.Equal(persistent, expected) {
		msg := fmt.Sprintf("TridentBackend does not match expected result, got: %v expected: %v", persistent, expected)
		t.Fatal(msg)
	}
}
