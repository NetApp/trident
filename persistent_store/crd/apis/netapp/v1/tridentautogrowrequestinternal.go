// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/pkg/collection"
)

type TridentAutogrowRequestInternalPhase string

const (
	TridentAutogrowRequestInternalPending   TridentAutogrowRequestInternalPhase = "Pending"
	TridentAutogrowRequestInternalAccepted  TridentAutogrowRequestInternalPhase = "Accepted"
	TridentAutogrowRequestInternalCompleted TridentAutogrowRequestInternalPhase = "Completed"
	TridentAutogrowRequestInternalRejected  TridentAutogrowRequestInternalPhase = "Rejected"
)

// Helper methods for TridentAutogrowRequestInternal CRD

func (in *TridentAutogrowRequestInternal) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentAutogrowRequestInternal) GetKind() string {
	if in.Kind != "" {
		return in.Kind
	}
	return "TridentAutogrowRequestInternal"
}

func (in *TridentAutogrowRequestInternal) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentAutogrowRequestInternal) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentAutogrowRequestInternal) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentAutogrowRequestInternal) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}
