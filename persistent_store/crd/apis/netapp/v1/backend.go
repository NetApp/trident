// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

// NewTridentBackend creates a new backend CRD object from an internal storage.BackendPersistent object
func NewTridentBackend(ctx context.Context, persistent *storage.BackendPersistent) (*TridentBackend, error) {
	backend := &TridentBackend{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentBackend",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "tbe-",
			Finalizers:   GetTridentFinalizers(),
		},
		BackendName: persistent.Name,
		BackendUUID: persistent.BackendUUID,
		ConfigRef:   persistent.ConfigRef,
	}

	if persistent.BackendUUID != "" {
		backend.BackendUUID = persistent.BackendUUID
	} else {
		backend.BackendUUID = uuid.New().String()
	}

	if err := backend.Apply(ctx, persistent); err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"backend.Name":        backend.Name,
		"backend.BackendName": backend.BackendName,
		"backend.BackendUUID": backend.BackendUUID,
	}).Debug("NewTridentBackend")

	return backend, nil
}

// Apply applies changes from an internal storage.BackendPersistent
// object to its Kubernetes CRD equivalent
func (in *TridentBackend) Apply(ctx context.Context, persistent *storage.BackendPersistent) error {
	Logc(ctx).WithFields(LogFields{
		"persistent.Name":   persistent.Name,
		"persistent.Online": persistent.Online,
		"persistent.State":  string(persistent.State),
		"in.Name":           in.Name,
		"in.BackendName":    in.BackendName,
		"in.BackendUUID":    in.BackendUUID,
		"in.State":          in.State,
		"in.ConfigRef":      in.ConfigRef,
	}).Debug("Applying backend update.")

	config, err := json.Marshal(persistent.Config)
	if err != nil {
		return err
	}

	in.Config.Raw = config
	in.BackendName = persistent.Name
	in.Online = persistent.Online
	in.Version = persistent.Version
	in.State = string(persistent.State)
	in.StateReason = persistent.StateReason
	in.ConfigRef = persistent.ConfigRef
	if in.BackendUUID == "" && persistent.BackendUUID != "" {
		in.BackendUUID = persistent.BackendUUID
	}

	return nil
}

// Persistent converts a Kubernetes CRD object into its internal
// storage.BackendPersistent equivalent
func (in *TridentBackend) Persistent() (*storage.BackendPersistent, error) {
	persistent := &storage.BackendPersistent{
		Name:        in.BackendName,
		BackendUUID: in.BackendUUID,
		Version:     in.Version,
		Online:      in.Online,
		State:       storage.BackendState(in.State),
		StateReason: in.StateReason,
		ConfigRef:   in.ConfigRef,
	}

	return persistent, json.Unmarshal(in.Config.Raw, &persistent.Config)
}

func (in *TridentBackend) CurrentState() storage.BackendState {
	return storage.BackendState(in.State)
}

func (in *TridentBackend) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentBackend) GetKind() string {
	return "TridentBackend"
}

func (in *TridentBackend) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentBackend) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentBackend) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = utils.RemoveStringFromSlice(in.ObjectMeta.Finalizers, finalizerName)
	}
}
