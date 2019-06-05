// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewTridentTransaction creates a new storage class CRD object from an operation and internal
// storage.VolumeConfig object
func NewTridentTransaction(op string, volumeConfig *storage.VolumeConfig) (*TridentTransaction, error) {

	transaction := &TridentTransaction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentTransaction",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(volumeConfig.Name),
			Finalizers: GetTridentFinalizers(),
		},
	}

	if err := transaction.Apply(op, volumeConfig); err != nil {
		return nil, err
	}

	return transaction, nil
}

// Apply applies changes from an operation and internal
// storage.VolumeConfig object to its Kubernetes CRD equivalent
func (in *TridentTransaction) Apply(op string, volumeConfig *storage.VolumeConfig) error {
	if NameFix(volumeConfig.Name) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	config, err := json.Marshal(volumeConfig)
	if err != nil {
		return err
	}

	in.Operation = op
	in.Config.Raw = config

	return nil
}

// Persistent converts a Kubernetes CRD object into its
// operation and internal storage.VolumeConfig
func (in *TridentTransaction) Persistent() (string, *storage.VolumeConfig, error) {
	volumeConfig := &storage.VolumeConfig{}

	return in.Operation, volumeConfig, json.Unmarshal(in.Config.Raw, volumeConfig)
}

func (in *TridentTransaction) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentTransaction) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentTransaction) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentTransaction) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = utils.RemoveStringFromSlice(in.ObjectMeta.Finalizers, finalizerName)
	}
}
