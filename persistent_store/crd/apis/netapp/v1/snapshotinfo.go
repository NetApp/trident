// Copyright 2021 NetApp, Inc. All Rights Reserved.

package v1

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/utils"
)

const (
	// SnapshotInfoInvalid implies the user supplied a non-feasible TSI spec
	SnapshotInfoInvalid      = "invalid"
	SnapshotInfoUpdateFailed = "updateFailed"
	SnapshotInfoUpdated      = "updated"
)

func (in *TridentSnapshotInfo) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentSnapshotInfo) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentSnapshotInfo) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentSnapshotInfo) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentSnapshotInfo) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = utils.RemoveStringFromSlice(in.ObjectMeta.Finalizers, finalizerName)
	}
}

// isValid returns whether the TridentSnapshotInfo CR provided has it's fields set to valid value combinations and
// any reason it is invalid as a string
func (in *TridentSnapshotInfo) IsValid() (isValid bool, reason string) {
	// If snapshotName not set
	if strings.TrimSpace(in.Spec.SnapshotName) == "" {
		return false, "snapshotName must be set"
	}
	return true, ""
}
