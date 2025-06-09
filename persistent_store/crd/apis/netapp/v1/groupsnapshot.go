// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

func NewTridentGroupSnapshot(persistent *storage.GroupSnapshotPersistent) (*TridentGroupSnapshot, error) {
	tgs := &TridentGroupSnapshot{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentGroupSnapshot",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(persistent.ID()),
			Finalizers: GetTridentFinalizers(),
		},
	}

	if err := tgs.Apply(persistent); err != nil {
		return nil, err
	}

	return tgs, nil
}

// Apply applies changes from an internal GroupSnapshotPersistent object to its Kubernetes CRD equivalent.
func (in *TridentGroupSnapshot) Apply(persistent *storage.GroupSnapshotPersistent) error {
	if NameFix(persistent.ID()) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	// Fill in the config into the spec.
	config, err := json.Marshal(persistent.Config())
	if err != nil {
		return err
	}
	in.Spec.Raw = config

	// Append top-level fields to the CR.
	in.Snapshots = persistent.GetSnapshotIDs()
	in.Created = persistent.GetCreated()
	return nil
}

// Persistent converts a TridentGroupSnapshot CRD object into its GroupSnapshotPersistent equivalent.
func (in *TridentGroupSnapshot) Persistent() (*storage.GroupSnapshotPersistent, error) {
	config := &storage.GroupSnapshotConfig{}
	if err := json.Unmarshal(in.Spec.Raw, config); err != nil {
		return nil, err
	} else if config == nil {
		return nil, errors.New("persisted config is nil")
	}

	// Create a group snapshot using options.
	groupSnapshot := &storage.GroupSnapshot{
		GroupSnapshotConfig: config,
		Created:             in.Created,
		SnapshotIDs:         in.Snapshots,
	}

	return storage.NewGroupSnapshotPersistent(groupSnapshot), nil
}

func (in *TridentGroupSnapshot) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentGroupSnapshot) GetKind() string {
	return "TridentGroupSnapshot"
}

func (in *TridentGroupSnapshot) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentGroupSnapshot) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentGroupSnapshot) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}

// Persistent converts a list of TridentGroupSnapshot CRD objects into their GroupSnapshotPersistent equivalents.
func (in *TridentGroupSnapshotList) Persistent() ([]*storage.GroupSnapshotPersistent, error) {
	persistedGroupSnapshots := make([]*storage.GroupSnapshotPersistent, 0, len(in.Items))
	for _, item := range in.Items {
		persistedGroupSnapshot, err := item.Persistent()
		if err != nil {
			return nil, err
		}
		persistedGroupSnapshots = append(persistedGroupSnapshots, persistedGroupSnapshot)
	}
	return persistedGroupSnapshots, nil
}
