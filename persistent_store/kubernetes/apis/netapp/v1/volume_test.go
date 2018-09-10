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
		Config:  &volConfig,
		Backend: "ontapnas_10.0.0.100",
		Pool:    "aggr1",
	}

	// Convert to Kubernetes Object using NewVolume
	volume, err := NewVolume(vol.ConstructExternal())
	if err != nil {
		t.Fatal("Unable to construct Volume CRD: ", err)
	}

	// Build expected Kubernetes Object
	expected := &Volume{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "Volume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(volConfig.Name),
		},
		Backend:  vol.Backend,
		Orphaned: false,
		Pool:     vol.Pool,
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(vol.ConstructExternal().Config)),
		},
	}

	// Compare
	if !reflect.DeepEqual(volume, expected) {
		t.Fatalf("Volume does not match expected result, got %v expected %v", volume, expected)
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
		Config:  &volConfig,
		Backend: "ontapnas_10.0.0.100",
		Pool:    "aggr1",
	}

	// Build Kubernetes Object
	volume := &Volume{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "Volume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(volConfig.Name),
		},
		Backend:  vol.Backend,
		Orphaned: false,
		Pool:     vol.Pool,
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(vol.ConstructExternal().Config)),
		},
	}

	// Build persistent object by calling Backend.Persistent
	peristent, err := volume.Persistent()
	if err != nil {
		t.Fatal("Unable to construct Volume persistent object: ", err)
	}

	// Build expected persistent object
	expected := vol.ConstructExternal()

	// Compare
	if !reflect.DeepEqual(peristent, expected) {
		t.Fatalf("Volume does not match expected result, got %v expected %v", peristent, expected)
	}
}
