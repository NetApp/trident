// Copyright 2018 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	"github.com/netapp/trident/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewVolume creates a new storage class CRD object from a internal
// storage.VolumeExternal object
func NewVolume(persistent *storage.VolumeExternal) (*Volume, error) {
	volume := &Volume{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "Volume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: NameFix(persistent.Config.Name),
		},
	}

	return volume, volume.Apply(persistent)
}

// Apply applies changes from an internal storage.VolumeExternal
// object to its Kubernetes CRD equivalent
func (v *Volume) Apply(persistent *storage.VolumeExternal) error {
	if NameFix(persistent.Config.Name) != v.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	config, err := json.Marshal(persistent.Config)
	if err != nil {
		return err
	}

	v.Config.Raw = config
	v.Backend = persistent.Backend
	v.Orphaned = persistent.Orphaned
	v.Pool = persistent.Pool

	return nil
}

// Persistent converts a Kubernetes CRD object into its internal
// storage.VolumeExternal equivalent
func (v *Volume) Persistent() (*storage.VolumeExternal, error) {
	persistent := &storage.VolumeExternal{
		Backend:  v.Backend,
		Orphaned: v.Orphaned,
		Pool:     v.Pool,
		Config:   &storage.VolumeConfig{},
	}

	return persistent, json.Unmarshal(v.Config.Raw, persistent.Config)
}
