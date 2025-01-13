// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/pkg/collection"
)

func (in *TridentActionMirrorUpdate) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentActionMirrorUpdate) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentActionMirrorUpdate) GetKind() string {
	return "TridentActionMirrorUpdate"
}

func (in *TridentActionMirrorUpdate) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentActionMirrorUpdate) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentActionMirrorUpdate) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}

// IsNew indicates whether the snapshot restore action has not been started.
func (in *TridentActionMirrorUpdate) IsNew() bool {
	return in.Status.State == "" && in.Status.CompletionTime == nil && in.DeletionTimestamp == nil
}

// IsComplete indicates whether the snapshot restore action has been completed.
func (in *TridentActionMirrorUpdate) IsComplete() bool {
	return in.Status.CompletionTime != nil && !in.Status.CompletionTime.IsZero()
}

// Succeeded indicates whether the snapshot restore action succeeded.
func (in *TridentActionMirrorUpdate) Succeeded() bool {
	return in.Status.State == TridentActionStateSucceeded
}

func (in *TridentActionMirrorUpdate) InProgress() bool {
	return in.Status.State == TridentActionStateInProgress
}

// Failed indicates whether the snapshot restore action failed.
func (in *TridentActionMirrorUpdate) Failed() bool {
	return in.Status.State == TridentActionStateFailed
}
