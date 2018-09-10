package v1

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_class"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func TestNewStorageClass(t *testing.T) {
	// Build storage class
	bronzeStorageConfig := &storageclass.Config{
		Name:            "bronze",
		Attributes:      make(map[string]storageattribute.Request),
		AdditionalPools: make(map[string][]string),
	}
	bronzeStorageConfig.Attributes["media"] = storageattribute.NewStringRequest("hdd")
	bronzeStorageConfig.AdditionalPools["ontapnas_10.0.207.101"] = []string{"aggr1"}
	bronzeStorageConfig.AdditionalPools["ontapsan_10.0.207.103"] = []string{"aggr1"}

	bronzeStorageClass := storageclass.New(bronzeStorageConfig)

	// Convert to Kubernetes Object using NewStorageClass
	storageClass, err := NewStorageClass(bronzeStorageClass.ConstructPersistent())
	if err != nil {
		t.Fatal("Unable to construct Backend CRD: ", err)
	}

	// Build expected Kubernetes Object
	expected := &StorageClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "StorageClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(bronzeStorageConfig.Name),
		},
		Spec: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(bronzeStorageClass.ConstructPersistent().Config)),
		},
	}

	// Compare
	if !reflect.DeepEqual(storageClass, expected) {
		t.Fatalf("StorageClass does not match expected result, got %v expected %v", storageClass, expected)
	}
}

func TestStorageClass_Persistent(t *testing.T) {
	// Build storage class
	bronzeStorageConfig := &storageclass.Config{
		Name:            "bronze",
		Attributes:      make(map[string]storageattribute.Request),
		AdditionalPools: make(map[string][]string),
	}
	bronzeStorageConfig.Attributes["media"] = storageattribute.NewStringRequest("hdd")
	bronzeStorageConfig.AdditionalPools["ontapnas_10.0.207.101"] = []string{"aggr1"}
	bronzeStorageConfig.AdditionalPools["ontapsan_10.0.207.103"] = []string{"aggr1"}

	bronzeStorageClass := storageclass.New(bronzeStorageConfig)

	// Build Kubernetes Object
	storageClass := &StorageClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "StorageClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(bronzeStorageConfig.Name),
		},
		Spec: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(bronzeStorageClass.ConstructPersistent().Config)),
		},
	}

	// Build persistent object by calling Backend.Persistent
	peristent, err := storageClass.Persistent()
	if err != nil {
		t.Fatal("Unable to construct StorageClass persistent object: ", err)
	}

	// Build expected persistent object
	expected := bronzeStorageClass.ConstructPersistent()

	// Compare
	if !reflect.DeepEqual(peristent, expected) {
		t.Fatalf("StorageClass does not match expected result, got %v expected %v", peristent, expected)
	}
}
