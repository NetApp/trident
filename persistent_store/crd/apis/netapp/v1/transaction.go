// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	"github.com/netapp/trident/v21/storage"
	"github.com/netapp/trident/v21/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewTridentTransaction creates a new storage class CRD object from a VolumeTransaction object
func NewTridentTransaction(txn *storage.VolumeTransaction) (*TridentTransaction, error) {

	transaction := &TridentTransaction{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentTransaction",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(txn.Name()),
			Finalizers: GetTridentFinalizers(),
		},
	}

	if err := transaction.Apply(txn); err != nil {
		return nil, err
	}

	return transaction, nil
}

// Apply applies changes from an operation and internal
// storage.VolumeConfig object to its Kubernetes CRD equivalent
func (in *TridentTransaction) Apply(txn *storage.VolumeTransaction) error {
	if NameFix(txn.Name()) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	transaction, err := json.Marshal(txn)
	if err != nil {
		return err
	}

	in.Transaction.Raw = transaction

	return nil
}

// Persistent converts a Kubernetes CRD object into its
// operation and internal storage.VolumeConfig
func (in *TridentTransaction) Persistent() (*storage.VolumeTransaction, error) {

	persistent := &storage.VolumeTransaction{}

	if err := json.Unmarshal(in.Transaction.Raw, persistent); err != nil {
		return nil, err
	}

	return persistent, nil
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
