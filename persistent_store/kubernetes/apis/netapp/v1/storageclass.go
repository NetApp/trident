// Copyright 2018 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	"github.com/netapp/trident/storage_class"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewStorageClass creates a new storage class CRD object from a internal
// storageclass.Persistent object
func NewStorageClass(persistent *storageclass.Persistent) (*StorageClass, error) {
	storageclass := &StorageClass{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "StorageClass",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(persistent.Config.Name),
		},
	}

	return storageclass, storageclass.Apply(persistent)
}

// Apply applies changes from an internal storageclass.Persistent
// object to its Kubernetes CRD equivalent
func (s *StorageClass) Apply(persistent *storageclass.Persistent) error {
	if NameFix(persistent.Config.Name) != s.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	config, err := json.Marshal(persistent.Config)
	if err != nil {
		return err
	}

	s.Spec.Raw = config

	return nil
}

// Persistent converts a Kubernetes CRD object into its internal
// storageclass.Persistent equivalent
func (s *StorageClass) Persistent() (*storageclass.Persistent, error) {
	persistent := &storageclass.Persistent{
		Config: &storageclass.Config{},
	}

	return persistent, json.Unmarshal(s.Spec.Raw, persistent.Config)
}
