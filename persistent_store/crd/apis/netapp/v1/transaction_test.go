// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewTransaction(t *testing.T) {

	// Build volume transaction
	volConfig := &storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
		Name:         "volumeTransaction",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	txn := &storage.VolumeTransaction{
		Op:     "addVolume",
		Config: volConfig,
	}

	// Convert to Kubernetes Object using NewTridentTransaction
	volumeTransaction, err := NewTridentTransaction(txn)
	if err != nil {
		t.Fatal("Unable to construct TridentTransaction CRD: ", err)
	}

	// Build expected Kubernetes Object
	expected := &TridentTransaction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentTransaction",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(volConfig.Name),
			Finalizers: GetTridentFinalizers(),
		},
		Operation: "addVolume",
		VolumeConfig: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(volConfig)),
		},
		SnapshotConfig: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(nil)),
		},
	}

	// Compare
	if !reflect.DeepEqual(volumeTransaction, expected) {
		t.Fatalf("TridentTransaction does not match expected result, got %v expected %v", volumeTransaction, expected)
	}
}

func TestNewSnapshotTransaction(t *testing.T) {

	// Build volume transaction
	volConfig := &storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
		Name:         "volumeTransaction",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	snapConfig := &storage.SnapshotConfig{
		Version:    string(config.OrchestratorAPIVersion),
		Name:       "snapshotTransaction",
		VolumeName: "volumeTransaction",
	}
	txn := &storage.VolumeTransaction{
		Op:             "addVolume",
		Config:         volConfig,
		SnapshotConfig: snapConfig,
	}

	// Convert to Kubernetes Object using NewTridentTransaction
	volumeTransaction, err := NewTridentTransaction(txn)
	if err != nil {
		t.Fatal("Unable to construct TridentTransaction CRD: ", err)
	}

	// Build expected Kubernetes Object
	expected := &TridentTransaction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentTransaction",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(volConfig.Name),
			Finalizers: GetTridentFinalizers(),
		},
		Operation: "addVolume",
		VolumeConfig: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(volConfig)),
		},
		SnapshotConfig: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(snapConfig)),
		},
	}

	// Compare
	if !reflect.DeepEqual(volumeTransaction, expected) {
		t.Fatalf("TridentTransaction does not match expected result, got %v expected %v", volumeTransaction, expected)
	}
}

func TestTransaction_Persistent(t *testing.T) {
	// Build volume transaction
	volConfig := &storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
		Name:         "volumeTransaction",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}

	// Build Kubernetes Object
	volumeTransaction := &TridentTransaction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentTransaction",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(volConfig.Name),
			Finalizers: GetTridentFinalizers(),
		},
		Operation: "addVolume",
		VolumeConfig: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(volConfig)),
		},
		SnapshotConfig: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(nil)),
		},
	}

	// Build persistent object by calling TridentBackend.Persistent
	txn, err := volumeTransaction.Persistent()
	if err != nil {
		t.Fatal("Unable to construct TridentTransaction persistent object: ", err)
	}

	persistentOp := txn.Op
	persistentConfig := txn.Config

	// Build expected persistent object
	expectedOp := "addVolume"
	expectedConfig := volConfig

	// Compare
	if string(persistentOp) != expectedOp {
		t.Fatalf("TridentTransaction does not match expected result, got %v expected %v", persistentOp, expectedOp)
	}
	if !reflect.DeepEqual(persistentConfig, expectedConfig) {
		t.Fatalf("TridentTransaction does not match expected result, got %v expected %v", persistentConfig, expectedConfig)
	}
}

func TestSnapshotTransaction_Persistent(t *testing.T) {
	// Build volume transaction
	volConfig := &storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
		Name:         "volumeTransaction",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	snapConfig := &storage.SnapshotConfig{
		Version:    string(config.OrchestratorAPIVersion),
		Name:       "snapshotTransaction",
		VolumeName: "volumeTransaction",
	}

	// Build Kubernetes Object
	volumeTransaction := &TridentTransaction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentTransaction",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(volConfig.Name),
			Finalizers: GetTridentFinalizers(),
		},
		Operation: "addVolume",
		VolumeConfig: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(volConfig)),
		},
		SnapshotConfig: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(snapConfig)),
		},
	}

	// Build persistent object by calling TridentBackend.Persistent
	txn, err := volumeTransaction.Persistent()
	if err != nil {
		t.Fatal("Unable to construct TridentTransaction persistent object: ", err)
	}

	persistentOp := txn.Op
	persistentVolumeConfig := txn.Config
	persistentSnapshotConfig := txn.SnapshotConfig

	// Build expected persistent object
	expectedOp := "addVolume"
	expectedVolumeConfig := volConfig
	expectedSnapshotConfig := snapConfig

	// Compare
	if string(persistentOp) != expectedOp {
		t.Fatalf("TridentTransaction does not match expected result, got %v expected %v",
			persistentOp, expectedOp)
	}
	if !reflect.DeepEqual(persistentVolumeConfig, expectedVolumeConfig) {
		t.Fatalf("TridentTransaction does not match expected result, got %v expected %v",
			persistentVolumeConfig, expectedVolumeConfig)
	}
	if !reflect.DeepEqual(persistentSnapshotConfig, expectedSnapshotConfig) {
		t.Fatalf("TridentTransaction does not match expected result, got %v expected %v",
			persistentSnapshotConfig, expectedSnapshotConfig)
	}
}
