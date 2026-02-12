// Copyright 2025 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/storage"
)

type TridentAutogrowPolicyState string

const (
	// TridentAutogrowPolicyStateSuccess indicates the policy has been validated and is ready to use
	TridentAutogrowPolicyStateSuccess TridentAutogrowPolicyState = "Success"
	// TridentAutogrowPolicyStateFailed indicates the policy failed validation
	TridentAutogrowPolicyStateFailed TridentAutogrowPolicyState = "Failed"
	// TridentAutogrowPolicyStateDeleting indicates the policy is being deleted
	TridentAutogrowPolicyStateDeleting TridentAutogrowPolicyState = "Deleting"
)

// Persistent converts a Kubernetes CRD object into its internal
// storage.AutogrowPolicyPersistent equivalent
func (in *TridentAutogrowPolicy) Persistent() (*storage.AutogrowPolicyPersistent, error) {
	return &storage.AutogrowPolicyPersistent{
		Name:          in.ObjectMeta.Name,
		UsedThreshold: in.Spec.UsedThreshold,
		GrowthAmount:  in.Spec.GrowthAmount,
		MaxSize:       in.Spec.MaxSize,
		State:         storage.AutogrowPolicyState(in.Status.State),
	}, nil
}

// Helper methods for TridentAutogrowPolicy CRD

func (in *TridentAutogrowPolicy) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentAutogrowPolicy) GetKind() string {
	return "TridentAutogrowPolicy"
}

func (in *TridentAutogrowPolicy) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentAutogrowPolicy) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentAutogrowPolicy) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentAutogrowPolicy) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}
