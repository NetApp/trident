// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	storageattribute "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
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

	// Convert to Kubernetes Object using NewTridentStorageClass
	storageClass, err := NewTridentStorageClass(bronzeStorageClass.ConstructPersistent())
	if err != nil {
		t.Fatal("Unable to construct TridentStorageClass CRD: ", err)
	}

	// Build expected Kubernetes Object
	expected := &TridentStorageClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentStorageClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(bronzeStorageConfig.Name),
			Finalizers: GetTridentFinalizers(),
		},
		Spec: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(bronzeStorageClass.ConstructPersistent().Config)),
		},
	}

	// Compare
	if !reflect.DeepEqual(storageClass, expected) {
		t.Fatalf("TridentStorageClass does not match expected result, got %v expected %v", storageClass, expected)
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
	storageClass := &TridentStorageClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentStorageClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(bronzeStorageConfig.Name),
			Finalizers: GetTridentFinalizers(),
		},
		Spec: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(bronzeStorageClass.ConstructPersistent().Config)),
		},
	}

	// Build persistent object by calling TridentStorageClass.Persistent
	persistent, err := storageClass.Persistent()
	if err != nil {
		t.Fatal("Unable to construct TridentStorageClass persistent object: ", err)
	}

	// Build expected persistent object
	expected := bronzeStorageClass.ConstructPersistent()

	// Compare
	if !reflect.DeepEqual(persistent, expected) {
		t.Fatalf("TridentStorageClass does not match expected result, got %v expected %v", persistent, expected)
	}
}
