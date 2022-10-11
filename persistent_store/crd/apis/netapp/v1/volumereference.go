// Copyright 2022 NetApp, Inc. All Rights Reserved.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/utils"
)

func (in *TridentVolumeReference) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentVolumeReference) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentVolumeReference) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentVolumeReference) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentVolumeReference) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = utils.RemoveStringFromSlice(in.ObjectMeta.Finalizers, finalizerName)
	}
}

func (in *TridentVolumeReference) CacheKey() string {
	return in.Namespace + "_" + in.Spec.PVCNamespace + "/" + in.Spec.PVCName
}
