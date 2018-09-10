package v1

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func TestNewVolumeTransaction(t *testing.T) {
	// Build volume transaction
	volConfig := &storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
		Name:         "volumeTransaction",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}

	// Convert to Kubernetes Object using NewVolumeTransaction
	volumeTransaction, err := NewVolumeTransaction("addVolume", volConfig)
	if err != nil {
		t.Fatal("Unable to construct VolumeTransaction CRD: ", err)
	}

	// Build expected Kubernetes Object
	expected := &VolumeTransaction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "VolumeTransaction",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(volConfig.Name),
		},
		Operation: "addVolume",
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(volConfig)),
		},
	}

	// Compare
	if !reflect.DeepEqual(volumeTransaction, expected) {
		t.Fatalf("VolumeTransaction does not match expected result, got %v expected %v", volumeTransaction, expected)
	}
}

func TestVolumeTransaction_Persistent(t *testing.T) {
	// Build volume transaction
	volConfig := &storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
		Name:         "volumeTransaction",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}

	// Build Kubernetes Object
	volumeTransaction := &VolumeTransaction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "VolumeTransaction",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(volConfig.Name),
		},
		Operation: "addVolume",
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(volConfig)),
		},
	}

	// Build persistent object by calling Backend.Persistent
	persistentOp, persistentConfig, err := volumeTransaction.Persistent()
	if err != nil {
		t.Fatal("Unable to construct VolumeTransaction persistent object: ", err)
	}

	// Build expected persistent object
	expectedOp := "addVolume"
	expectedConfig := volConfig

	// Compare
	if !reflect.DeepEqual(persistentOp, expectedOp) {
		t.Fatalf("VolumeTransaction does not match expected result, got %v expected %v", persistentOp, expectedOp)
	}
	if !reflect.DeepEqual(persistentConfig, expectedConfig) {
		t.Fatalf("VolumeTransaction does not match expected result, got %v expected %v", persistentConfig, expectedConfig)
	}
}
