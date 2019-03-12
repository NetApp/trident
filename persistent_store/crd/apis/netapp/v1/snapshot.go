// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

// NewTridentSnapshot creates a new snapshot CRD object from an internal SnapshotPersistent object
func NewTridentSnapshot(persistent *storage.SnapshotPersistent) (*TridentSnapshot, error) {

	tsc := &TridentSnapshot{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentSnapshot",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(persistent.ID()),
			Finalizers: GetTridentFinalizers(),
		},
	}

	if err := tsc.Apply(persistent); err != nil {
		return nil, err
	}

	return tsc, nil
}

// Apply applies changes from an internal SnapshotPersistent object to its Kubernetes CRD equivalent
func (in *TridentSnapshot) Apply(persistent *storage.SnapshotPersistent) error {
	if NameFix(persistent.ID()) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	config, err := json.Marshal(persistent.Config)
	if err != nil {
		return err
	}

	in.Spec.Raw = config
	in.SizeBytes = persistent.SizeBytes
	in.Created = persistent.Created

	return nil
}

// Persistent converts a Kubernetes CRD object into its internal SnapshotPersistent equivalent
func (in *TridentSnapshot) Persistent() (*storage.SnapshotPersistent, error) {

	persistent := &storage.SnapshotPersistent{}

	persistent.Config = &storage.SnapshotConfig{}
	persistent.SizeBytes = in.SizeBytes
	persistent.Created = in.Created

	return persistent, json.Unmarshal(in.Spec.Raw, persistent.Config)
}

func (in *TridentSnapshot) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentSnapshot) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentSnapshot) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentSnapshot) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = utils.RemoveStringFromSlice(in.ObjectMeta.Finalizers, finalizerName)
	}
}
