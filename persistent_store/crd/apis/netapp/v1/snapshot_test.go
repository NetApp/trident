// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/netapp/trident/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewSnapshot(t *testing.T) {

	// Build snapshot
	testSnapshot := getFakeSnapshot()

	// Convert to Kubernetes Object using NewTridentSnapshot
	snapshotCRD, err := NewTridentSnapshot(testSnapshot.ConstructPersistent())
	if err != nil {
		t.Fatal("Unable to construct TridentSnapshot CRD: ", err)
	}

	// Build expected Kubernetes Object
	expectedCRD := getFakeSnapshotCRD(testSnapshot)

	// Compare
	if !reflect.DeepEqual(snapshotCRD, expectedCRD) {
		t.Fatalf("TridentSnapshot does not match expected result, got %v expected %v", snapshotCRD, expectedCRD)
	}
}

func TestSnapshot_Persistent(t *testing.T) {

	// Build snapshot
	testSnapshot := getFakeSnapshot()

	// Build expected Kubernetes Object
	snapshotCRD := getFakeSnapshotCRD(testSnapshot)

	// Build persistent object by calling TridentSnapshot.Persistent
	persistent, err := snapshotCRD.Persistent()
	if err != nil {
		t.Fatal("Unable to construct TridentSnapshot persistent object: ", err)
	}

	// Build expected persistent object
	expected := testSnapshot.ConstructPersistent()

	// Compare
	if !reflect.DeepEqual(persistent, expected) {
		t.Fatalf("TridentSnapshot does not match expected result, got %v expected %v", persistent, expected)
	}
}

func getFakeSnapshot() *storage.Snapshot {

	testSnapshotConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               "testsnap1",
		InternalName:       "internal_testsnap1",
		VolumeName:         "vol1",
		VolumeInternalName: "internal_vol1",
	}

	now := time.Now().UTC().Format(storage.SnapshotNameFormat)
	size := int64(1000000000)
	testSnapshot := storage.NewSnapshot(testSnapshotConfig, now, size, storage.SnapshotStateOnline)

	return testSnapshot
}

func getFakeSnapshotCRD(snapshot *storage.Snapshot) *TridentSnapshot {

	crd := &TridentSnapshot{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentSnapshot",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(snapshot.ID()),
			Finalizers: GetTridentFinalizers(),
		},
		Spec: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(snapshot.ConstructPersistent().Config)),
		},
		Created:   snapshot.Created,
		SizeBytes: snapshot.SizeBytes,
		State:     string(storage.SnapshotStateOnline),
	}

	return crd
}
