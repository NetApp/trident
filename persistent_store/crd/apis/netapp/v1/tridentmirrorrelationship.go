// Copyright 2021 NetApp, Inc. All Rights Reserved.

package v1

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/utils"
)

const (
	MirrorStateEstablished    = "established"
	MirrorStateEstablishing   = "establishing"
	MirrorStateReestablished  = "reestablished"
	MirrorStateReestablishing = "reestablishing"
	MirrorStatePromoted       = "promoted"
	MirrorStatePromoting      = "promoting"
	// MirrorStateFailed means we could not reach the desired state
	MirrorStateFailed = "failed"
	// MirrorStateInvalid means we can never reach the desired state with the current spec
	// Invalid implies the user supplied a non-feasible TMR spec
	MirrorStateInvalid = "invalid"
)

func GetValidMirrorSpecStates() []string {
	return []string{MirrorStateEstablished, MirrorStateReestablished, MirrorStatePromoted}
}

func GetTransitioningMirrorStatusStates() []string {
	return []string{MirrorStateReestablishing, MirrorStateEstablishing, MirrorStatePromoting}
}

func (in *TridentMirrorRelationship) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentMirrorRelationship) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentMirrorRelationship) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentMirrorRelationship) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !utils.SliceContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentMirrorRelationship) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = utils.RemoveStringFromSlice(in.ObjectMeta.Finalizers, finalizerName)
	}
}

// isValid returns whether the TridentMirrorRelationship CR provided has its fields set to valid value combinations and
// any reason it is invalid as a string
func (in *TridentMirrorRelationship) IsValid() (isValid bool, reason string) {
	// If volumeMappings not set
	if in.Spec.VolumeMappings == nil {
		return false, ".spec.volumeMappings must be set"
	}
	// If volumeMappings != 1
	if len(in.Spec.VolumeMappings) != 1 {
		return false, ".spec.volumeMappings must contain exactly one element in the list"
	}
	// Require local-pvc is specified. It does not yet need to exist
	localPVCName := in.Spec.VolumeMappings[0].LocalPVCName
	if localPVCName == "" {
		return false, ".spec.volumeMappings must specify a localPVCName"
	}
	// Check values of state in spec
	validMirrorStates := GetValidMirrorSpecStates()
	if in.Spec.MirrorState != "" && !utils.SliceContainsString(validMirrorStates, in.Spec.MirrorState) {
		return false, fmt.Sprintf(".spec.state must be one of %v",
			strings.Join(validMirrorStates, ", "))
	}
	// If promotedSnapshotHandle is specified, ensure state is set to promoted
	if in.Spec.VolumeMappings[0].PromotedSnapshotHandle != "" && in.Spec.MirrorState != MirrorStatePromoted {
		return false, fmt.Sprintf(".spec.state must be set to '%v' "+
			"when providing a 'promotedSnapshotHandle'", MirrorStatePromoted)
	}

	// If state is established or reestablished ensure a remoteVolumeHandle has been set
	if (in.Spec.MirrorState == MirrorStateEstablished || in.Spec.MirrorState == MirrorStateReestablished) && in.Spec.
		VolumeMappings[0].RemoteVolumeHandle == "" {
		return false, fmt.Sprintf("A remoteVolumeHandle must be provided if .spec.state is %v or %v",
			MirrorStateEstablished, MirrorStateReestablished)
	}

	return true, ""
}
