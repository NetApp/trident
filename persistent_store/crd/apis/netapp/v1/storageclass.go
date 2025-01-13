// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/pkg/collection"
	storageclass "github.com/netapp/trident/storage_class"
)

// NewTridentStorageClass creates a new storage class CRD object from a internal
// storageclass.Persistent object
func NewTridentStorageClass(persistent *storageclass.Persistent) (*TridentStorageClass, error) {
	tsc := &TridentStorageClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentStorageClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(persistent.Config.Name),
			Finalizers: GetTridentFinalizers(),
		},
	}

	if err := tsc.Apply(persistent); err != nil {
		return nil, err
	}

	return tsc, nil
}

// Apply applies changes from an internal storageclass.Persistent
// object to its Kubernetes CRD equivalent
func (in *TridentStorageClass) Apply(persistent *storageclass.Persistent) error {
	if NameFix(persistent.Config.Name) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	config, err := json.Marshal(persistent.Config)
	if err != nil {
		return err
	}

	in.Spec.Raw = config

	return nil
}

// Persistent converts a Kubernetes CRD object into its internal
// storageclass.Persistent equivalent
func (in *TridentStorageClass) Persistent() (*storageclass.Persistent, error) {
	persistent := &storageclass.Persistent{
		Config: &storageclass.Config{},
	}

	return persistent, json.Unmarshal(in.Spec.Raw, persistent.Config)
}

func (in *TridentStorageClass) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentStorageClass) GetKind() string {
	return "TridentStorageClass"
}

func (in *TridentStorageClass) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentStorageClass) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentStorageClass) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}
