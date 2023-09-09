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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
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
		t.Fatalf(msg)
	}
}
