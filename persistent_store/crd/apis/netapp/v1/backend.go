// Copyright 2019 NetApp, Inc. All Rights Reserved.

package v1

import (
	"encoding/json"

	uuid "github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewTridentBackend creates a new backend CRD object from an internal storage.BackendPersistent object
func NewTridentBackend(persistent *storage.BackendPersistent) (*TridentBackend, error) {

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
	}

	if persistent.BackendUUID != "" {
		backend.BackendUUID = persistent.BackendUUID
	} else {
		backend.BackendUUID = uuid.New().String()
	}

	if err := backend.Apply(persistent); err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"backend.Name":        backend.Name,
		"backend.BackendName": backend.BackendName,
		"backend.BackendUUID": backend.BackendUUID,
	}).Debug("NewTridentBackend")

	return backend, nil
}

// Apply applies changes from an internal storage.BackendPersistent
// object to its Kubernetes CRD equivalent
func (in *TridentBackend) Apply(persistent *storage.BackendPersistent) error {

	log.WithFields(log.Fields{
		"persistent.Name":   persistent.Name,
		"persistent.Online": persistent.Online,
		"persistent.State":  string(persistent.State),
		"in.Name":           in.Name,
		"in.BackendName":    in.BackendName,
		"in.BackendUUID":    in.BackendUUID,
		"in.State":          in.State,
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
	}

	return persistent, json.Unmarshal(in.Config.Raw, &persistent.Config)
}

func (in *TridentBackend) CurrentState() storage.BackendState {
	return storage.BackendState(in.State)
}

func (in *TridentBackend) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
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
