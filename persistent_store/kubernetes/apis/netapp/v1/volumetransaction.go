// Copyright 2018 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	"github.com/netapp/trident/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewVolumeTransaction creates a new storage class CRD object from an operation and internal
// storage.VolumeConfig object
func NewVolumeTransaction(op string, volumeConfig *storage.VolumeConfig) (*VolumeTransaction, error) {
	volumeTransaction := &VolumeTransaction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "VolumeTransaction",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(volumeConfig.Name),
		},
	}

	return volumeTransaction, volumeTransaction.Apply(op, volumeConfig)
}

// Apply applies changes from an operation and internal
// storage.VolumeConfig object to its Kubernetes CRD equivalent
func (v *VolumeTransaction) Apply(op string, volumeConfig *storage.VolumeConfig) error {
	if NameFix(volumeConfig.Name) != v.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	config, err := json.Marshal(volumeConfig)
	if err != nil {
		return err
	}

	v.Operation = op
	v.Config.Raw = config

	return nil
}

// Persistent converts a Kubernetes CRD object into its
// operation and internal storage.VolumeConfig
func (v *VolumeTransaction) Persistent() (string, *storage.VolumeConfig, error) {
	volumeConfig := &storage.VolumeConfig{}

	return v.Operation, volumeConfig, json.Unmarshal(v.Config.Raw, volumeConfig)
}
