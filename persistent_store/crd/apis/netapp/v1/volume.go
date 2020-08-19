// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"context"
	"encoding/json"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

// NewTridentVolume creates a new storage class CRD object from a internal
// storage.VolumeExternal object
func NewTridentVolume(ctx context.Context, persistent *storage.VolumeExternal) (*TridentVolume, error) {

	volume := &TridentVolume{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentVolume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       NameFix(persistent.Config.Name),
			Finalizers: GetTridentFinalizers(),
		},
		BackendUUID: persistent.BackendUUID,
	}

	if err := volume.Apply(ctx, persistent); err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"volume.Name":        volume.Name,
		"volume.BackendUUID": volume.BackendUUID,
		"volume.Orphaned":    volume.Orphaned,
		"volume.Pool":        volume.Pool,
	}).Debug("NewTridentVolume")

	return volume, nil
}

// Apply applies changes from an internal storage.VolumeExternal
// object to its Kubernetes CRD equivalent
func (in *TridentVolume) Apply(ctx context.Context, persistent *storage.VolumeExternal) error {

	Logc(ctx).WithFields(log.Fields{
		"persistent.BackendUUID": persistent.BackendUUID,
		"persistent.Orphaned":    persistent.Orphaned,
		"persistent.Pool":        persistent.Pool,
		"persistent.State":       string(persistent.State),
	}).Debug("Applying volume update.")

	if NameFix(persistent.Config.Name) != in.ObjectMeta.Name {
		return ErrNamesDontMatch
	}

	config, err := json.Marshal(persistent.Config)
	if err != nil {
		return err
	}

	in.Config.Raw = config
	in.BackendUUID = persistent.BackendUUID
	in.Orphaned = persistent.Orphaned
	in.Pool = persistent.Pool
	in.State = string(persistent.State)

	return nil
}

// Persistent converts a Kubernetes CRD object into its internal
// storage.VolumeExternal equivalent
func (in *TridentVolume) Persistent() (*storage.VolumeExternal, error) {
	persistent := &storage.VolumeExternal{
		BackendUUID: in.BackendUUID,
		Orphaned:    in.Orphaned,
		Pool:        in.Pool,
		Config:      &storage.VolumeConfig{},
		State:       storage.VolumeState(in.State),
	}

	return persistent, json.Unmarshal(in.Config.Raw, persistent.Config)
}

func (in *TridentVolume) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentVolume) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentVolume) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentVolume) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = utils.RemoveStringFromSlice(in.ObjectMeta.Finalizers, finalizerName)
	}
}
