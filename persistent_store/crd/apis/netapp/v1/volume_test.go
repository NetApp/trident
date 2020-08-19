// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"

	"github.com/google/go-cmp/cmp"
)

func TestNewVolume(t *testing.T) {
	// Build volume
	volConfig := storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
		Name:         "volume",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	vol := &storage.Volume{
		Config:      &volConfig,
		BackendUUID: "686979c7-6960-4380-a14d-2d740a13f0f5",
		Pool:        "aggr1",
		State:       storage.VolumeStateOnline,
	}

	// Convert to Kubernetes Object using NewTridentVolume
	volume, err := NewTridentVolume(ctx(), vol.ConstructExternal())
	if err != nil {
		t.Fatal("Unable to construct TridentVolume CRD: ", err)
	}

	// Build expected Kubernetes Object
	expected := &TridentVolume{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentVolume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(volConfig.Name),
			Finalizers: GetTridentFinalizers(),
		},
		BackendUUID: "686979c7-6960-4380-a14d-2d740a13f0f5",
		Orphaned:    false,
		Pool:        vol.Pool,
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(vol.ConstructExternal().Config)),
		},
		State: string(storage.VolumeStateOnline),
	}
	expected.BackendUUID = volume.BackendUUID

	// Compare
	if !cmp.Equal(volume, expected) {
		if diff := cmp.Diff(volume, expected); diff != "" {
			t.Fatalf("TridentVolume does not match expected result, difference (-expected +actual):%s", diff)
		}
	}
}

func TestVolume_Persistent(t *testing.T) {
	// Build volume
	volConfig := storage.VolumeConfig{
		Version:      string(config.OrchestratorAPIVersion),
		Name:         "volume",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	vol := &storage.Volume{
		Config:      &volConfig,
		BackendUUID: "686979c7-6960-4380-a14d-2d740a13f0f5",
		Pool:        "aggr1",
		State:       storage.VolumeStateOnline,
	}

	// Build Kubernetes Object
	volume := &TridentVolume{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentVolume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(volConfig.Name),
			Finalizers: GetTridentFinalizers(),
		},
		BackendUUID: vol.BackendUUID,
		Orphaned:    false,
		State:       string(storage.VolumeStateOnline),
		Pool:        vol.Pool,
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(vol.ConstructExternal().Config)),
		},
	}

	// Build persistent object by calling TridentBackend.Persistent
	peristent, err := volume.Persistent()
	if err != nil {
		t.Fatal("Unable to construct TridentVolume persistent object: ", err)
	}

	// Build expected persistent object
	expected := vol.ConstructExternal()

	// Compare
	if !cmp.Equal(peristent, expected) {
		if diff := cmp.Diff(peristent, expected); diff != "" {
			t.Fatalf("TridentVolume does not match expected result, difference (-expected +actual):%s", diff)
		}
	}
}
