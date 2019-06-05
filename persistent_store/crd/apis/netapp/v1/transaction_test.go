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

	// Convert to Kubernetes Object using NewTridentTransaction
	volumeTransaction, err := NewTridentTransaction("addVolume", volConfig)
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
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(volConfig)),
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
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(volConfig)),
		},
	}

	// Build persistent object by calling TridentBackend.Persistent
	persistentOp, persistentConfig, err := volumeTransaction.Persistent()
	if err != nil {
		t.Fatal("Unable to construct TridentTransaction persistent object: ", err)
	}

	// Build expected persistent object
	expectedOp := "addVolume"
	expectedConfig := volConfig

	// Compare
	if !reflect.DeepEqual(persistentOp, expectedOp) {
		t.Fatalf("TridentTransaction does not match expected result, got %v expected %v", persistentOp, expectedOp)
	}
	if !reflect.DeepEqual(persistentConfig, expectedConfig) {
		t.Fatalf("TridentTransaction does not match expected result, got %v expected %v", persistentConfig, expectedConfig)
	}
}
