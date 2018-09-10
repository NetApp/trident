package v1

import (
	"encoding/json"

	"github.com/netapp/trident/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewBackend creates a new backend CRD object from a internal
// storage.BackendPersistent object
func NewBackend(persistent *storage.BackendPersistent) (*Backend, error) {
	backend := &Backend{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "Backend",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(persistent.Name),
		},
	}

	return backend, backend.Apply(persistent)
}

// Apply applies changes from an internal storage.BackendPersistnet
// object to its Kubernetes CRD equivalent
func (b *Backend) Apply(persistent *storage.BackendPersistent) error {
	if NameFix(persistent.Name) != b.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	config, err := json.Marshal(persistent.Config)
	if err != nil {
		return err
	}

	b.Config.Raw = config
	b.Name = persistent.Name
	b.Online = persistent.Online
	b.Version = persistent.Version

	return nil
}

// Persistent converts a Kubernetes CRD object into its internal
// storage.BackendPersistent equivalent
func (b *Backend) Persistent() (*storage.BackendPersistent, error) {
	persistent := &storage.BackendPersistent{
		Name:    b.Name,
		Version: b.Version,
		Online:  b.Online,
	}

	return persistent, json.Unmarshal(b.Config.Raw, &persistent.Config)
}
