// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/pkg/collection"
)

const PersistentStateVersionName = "trident"

// NewTridentVersion creates a new persistent state version CRD object from an internal
// persistent_store.PersistentStateVersion object.
func NewTridentVersion(persistent *config.PersistentStateVersion) (*TridentVersion, error) {
	version := &TridentVersion{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentVersion",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(PersistentStateVersionName),
			Finalizers: GetTridentFinalizers(),
		},
	}

	if err := version.Apply(persistent); err != nil {
		return nil, err
	}

	return version, nil
}

// Apply applies changes from an internal persistent_store.PersistentStateVersion
// object to its Kubernetes CRD equivalent.
func (in *TridentVersion) Apply(persistent *config.PersistentStateVersion) error {
	if NameFix(PersistentStateVersionName) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	in.TridentVersion = config.OrchestratorVersion.String()
	in.PersistentStoreVersion = persistent.PersistentStoreVersion
	in.OrchestratorAPIVersion = persistent.OrchestratorAPIVersion
	in.PublicationsSynced = persistent.PublicationsSynced

	return nil
}

// Persistent converts a Kubernetes CRD object into its internal
// persistent_store.PersistentStateVersion equivalent.
func (in *TridentVersion) Persistent() (*config.PersistentStateVersion, error) {
	persistent := &config.PersistentStateVersion{
		PersistentStoreVersion: in.PersistentStoreVersion,
		OrchestratorAPIVersion: in.OrchestratorAPIVersion,
		PublicationsSynced:     in.PublicationsSynced,
	}

	return persistent, nil
}

func (in *TridentVersion) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentVersion) GetKind() string {
	return "TridentVersion"
}

func (in *TridentVersion) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentVersion) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentVersion) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}
