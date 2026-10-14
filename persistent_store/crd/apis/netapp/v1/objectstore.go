// Copyright 2026 NetApp, Inc. All Rights Reserved.

package v1

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/object"
	objectfactory "github.com/netapp/trident/object/factory"
	"github.com/netapp/trident/pkg/collection"
)

const (
	objectStoreCredsKey = "credentials"
)

func NewTridentObjectStore(store object.Store) (*TridentObjectStore, error) {
	tos := &TridentObjectStore{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentObjectStore",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(store.Name()),
			Finalizers: GetTridentFinalizers(),
		},
	}
	spec, err := json.Marshal(store)
	if err != nil {
		return nil, err
	}
	tos.Spec.Raw = spec
	tos.Status.Raw = spec
	return tos, nil
}

// Apply writes status only. Spec is left unchanged, and status is the store's own JSON
// with no key filtering or overlay.
func (in *TridentObjectStore) Apply(store object.Store) error {
	if NameFix(store.Name()) != in.Name {
		return ErrNamesDontMatch
	}
	raw, err := json.Marshal(store)
	if err != nil {
		return err
	}
	in.Status.Raw = raw
	return nil
}

// Persistent constructs the object store by unmarshaling status, then spec, onto one driver,
// so user spec wins over status. The CR name is applied from metadata.name and wins over both.
func (in *TridentObjectStore) Persistent(ctx context.Context, secretConfigs ...[]byte) (object.Store, error) {
	driverName := driverNameFromJSON(in.Spec.Raw)
	if driverName == "" {
		driverName = driverNameFromJSON(in.Status.Raw)
	}
	if driverName == "" {
		return nil, fmt.Errorf("objectStoreDriver is required")
	}
	var secretConfig []byte
	if len(secretConfigs) > 0 {
		secretConfig = secretConfigs[0]
	}
	store, err := objectfactory.CreateStore(ctx, driverName, secretConfig, in.Status.Raw, in.Spec.Raw)
	if err != nil {
		return nil, err
	}
	store.SetCommonConfig(object.CommonConfig{StoreName: in.Name})
	return store, nil
}

func (in *TridentObjectStoreSpec) GetDriverName() (string, error) {
	driverName := driverNameFromJSON(in.Raw)
	if driverName == "" {
		return "", fmt.Errorf("objectStoreDriver is required")
	}
	return driverName, nil
}

func (in *TridentObjectStoreSpec) GetSecretName() (string, error) {
	var spec map[string]interface{}
	if err := json.Unmarshal(in.Raw, &spec); err != nil {
		return "", fmt.Errorf("could not parse object store configuration: %w", err)
	}
	credentials, ok := spec[objectStoreCredsKey]
	if !ok {
		return "", nil
	}
	credentialsMap, ok := credentials.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("credentials must be an object")
	}
	secretName, ok := credentialsMap["secretName"].(string)
	if !ok || secretName == "" {
		return "", fmt.Errorf("credentials.secretName is required")
	}
	return secretName, nil
}

// SecretConfig returns Kubernetes Secret data as JSON for the object-store driver.
func SecretConfig(secretData map[string][]byte) ([]byte, error) {
	if len(secretData) == 0 {
		return nil, nil
	}
	config := make(map[string]string, len(secretData))
	for key, value := range secretData {
		config[key] = string(value)
	}
	return json.Marshal(config)
}

// driverNameFromJSON returns the 'objectStoreDriver' field, or empty if it is not present.
func driverNameFromJSON(raw []byte) string {
	spec := struct {
		DriverName string `json:"objectStoreDriver"`
	}{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return ""
	}
	return spec.DriverName
}

func (in *TridentObjectStore) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentObjectStore) GetKind() string {
	return "TridentObjectStore"
}

func (in *TridentObjectStore) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentObjectStore) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentObjectStore) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentObjectStore) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}
