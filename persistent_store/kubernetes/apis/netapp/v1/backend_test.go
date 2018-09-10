package v1

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func TestNewBackend(t *testing.T) {
	// Build backend
	nfsServerConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: drivers.OntapNASStorageDriverName,
		},
		ManagementLIF: "10.0.0.4",
		DataLIF:       "10.0.0.100",
		SVM:           "svm1",
		Username:      "admin",
		Password:      "netapp",
	}
	nfsDriver := ontap.NASStorageDriver{
		Config: nfsServerConfig,
	}
	nfsServer := &storage.Backend{
		Driver: &nfsDriver,
		Name:   "nfs_server_1-" + nfsServerConfig.ManagementLIF,
	}

	// Convert to Kubernetes Object using the NewBackend method
	backend, err := NewBackend(nfsServer.ConstructPersistent())
	if err != nil {
		t.Fatal("Unable to construct Backend CRD: ", err)
	}

	// Build expected result
	expected := &Backend{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "Backend",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(nfsServer.Name),
		},
		Name:    nfsServer.Name,
		Online:  false,
		Version: "1",
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(nfsServer.ConstructPersistent().Config)),
		},
	}

	// Compare
	if !reflect.DeepEqual(backend, expected) {
		t.Fatalf("Backend does not match expected result, got %v expected %v", backend, expected)
	}
}

func TestBackend_Persistent(t *testing.T) {
	// Build backend
	nfsServerConfig := drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: drivers.OntapNASStorageDriverName,
		},
		ManagementLIF: "10.0.0.4",
		DataLIF:       "10.0.0.100",
		SVM:           "svm1",
		Username:      "admin",
		Password:      "netapp",
	}
	nfsDriver := ontap.NASStorageDriver{
		Config: nfsServerConfig,
	}
	nfsServer := &storage.Backend{
		Driver: &nfsDriver,
		Name:   "nfs_server_1-" + nfsServerConfig.ManagementLIF,
	}

	// Build Kubernetes Object
	backend := &Backend{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "Backend",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(nfsServer.Name),
		},
		Name:    nfsServer.Name,
		Online:  false,
		Version: "1",
		Config: runtime.RawExtension{
			Raw: MustEncode(json.Marshal(nfsServer.ConstructPersistent().Config)),
		},
	}

	// Build persistent object by calling Backend.Persistent
	persistent, err := backend.Persistent()
	if err != nil {
		t.Fatal("Unable to construct Backend persistent object: ", err)
	}

	// Build expected persistent object
	expected := nfsServer.ConstructPersistent()

	// Compare
	if !reflect.DeepEqual(persistent, expected) {
		t.Fatalf("Backend does not match expected result, got %v expected %v", persistent, expected)
	}
}
