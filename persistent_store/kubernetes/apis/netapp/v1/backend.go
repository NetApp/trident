package v1

import (
	"encoding/json"

	"github.com/netapp/trident/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func BackendFromBackendPersistent(persistent *storage.BackendPersistent) (*Backend, error) {
	config, err := json.Marshal(persistent.Config)
	if err != nil {
		return nil, err
	}

	return &Backend{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "Backend",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: persistent.Name,
		},
		Spec: runtime.RawExtension{
			Raw: config,
		},
		Status: BackendStatus{
			Version: persistent.Version,
			Online:  persistent.Online,
		},
	}, nil
}

func BackendPersistentFromBackend(backend *Backend) (*storage.BackendPersistent, error) {
	persistent := &storage.BackendPersistent{
		Name:    backend.ObjectMeta.Name,
		Version: backend.Status.Version,
		Online:  backend.Status.Online,
	}

	return persistent, json.Unmarshal(backend.Spec.Raw, persistent.Config)
}
