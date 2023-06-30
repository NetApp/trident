// Copyright 2023 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/utils"
)

func (in *TridentActionSnapshotRestore) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentActionSnapshotRestore) GetKind() string {
	return "TridentActionSnapshotRestore"
}

func (in *TridentActionSnapshotRestore) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentActionSnapshotRestore) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentActionSnapshotRestore) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentActionSnapshotRestore) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = utils.RemoveStringFromSlice(in.ObjectMeta.Finalizers, finalizerName)
	}
}

// IsNew indicates whether the snapshot restore action has not been started.
func (in *TridentActionSnapshotRestore) IsNew() bool {
	return in.Status.State == "" && in.Status.CompletionTime == nil && in.DeletionTimestamp == nil
}

// IsComplete indicates whether the snapshot restore action has been completed.
func (in *TridentActionSnapshotRestore) IsComplete() bool {
	return in.Status.CompletionTime != nil && !in.Status.CompletionTime.IsZero()
}

// Succeeded indicates whether the snapshot restore action succeeded.
func (in *TridentActionSnapshotRestore) Succeeded() bool {
	return in.Status.State == TridentActionStateSucceeded
}

func (in *TridentActionSnapshotRestore) InProgress() bool {
	return in.Status.State == TridentActionStateInProgress
}

// Failed indicates whether the snapshot restore action failed.
func (in *TridentActionSnapshotRestore) Failed() bool {
	return in.Status.State == TridentActionStateFailed
}
