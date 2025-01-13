// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/utils/models"
)

// NewTridentVolumePublication creates a new volume publication CRD object from an internal
// utils.VolumePublication object.
func NewTridentVolumePublication(persistent *models.VolumePublication) (*TridentVolumePublication, error) {
	publication := &TridentVolumePublication{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentVolumePublication",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(persistent.Name),
			Finalizers: GetTridentFinalizers(),
		},
	}

	if err := publication.Apply(persistent); err != nil {
		return nil, err
	}

	return publication, nil
}

// Apply applies changes from an internal utils.VolumePublication
// object to its Kubernetes CRD equivalent.
func (in *TridentVolumePublication) Apply(persistent *models.VolumePublication) error {
	if NameFix(persistent.Name) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	in.Name = persistent.Name
	in.NodeID = persistent.NodeName
	in.VolumeID = persistent.VolumeName
	in.ReadOnly = persistent.ReadOnly
	in.AccessMode = persistent.AccessMode

	return nil
}

func (in *TridentVolumePublication) Persistent() (*models.VolumePublication, error) {
	persistent := &models.VolumePublication{
		Name:       in.Name,
		NodeName:   in.NodeID,
		VolumeName: in.VolumeID,
		ReadOnly:   in.ReadOnly,
		AccessMode: in.AccessMode,
	}

	return persistent, nil
}

func (in *TridentVolumePublication) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentVolumePublication) GetKind() string {
	return "TridentVolumePublication"
}

func (in *TridentVolumePublication) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentVolumePublication) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentVolumePublication) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentVolumePublication) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}
