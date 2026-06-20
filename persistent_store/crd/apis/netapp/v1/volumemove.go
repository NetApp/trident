// Copyright 2026 NetApp, Inc. All Rights Reserved.

package v1

import (
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/models"

	"github.com/netapp/trident/pkg/collection"
)

func (in *TridentVolumeMove) Persistent() (*storage.VolumeMoveExternal, error) {
	if in == nil {
		return nil, nil
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}

	var deleteAfterSuccess *time.Duration
	if in.Spec.DeleteAfterSuccess != nil {
		deleteAfterSuccess = &in.Spec.DeleteAfterSuccess.Duration
	}

	return &storage.VolumeMoveExternal{
		State:              in.Status.State,
		VolumeName:         in.Name,
		SourcePool:         in.Spec.SourcePool,
		SourceNode:         in.Spec.SourceNode,
		TargetPool:         in.Spec.TargetPool,
		TargetNode:         in.Spec.TargetNode,
		BackendContext:     in.Status.BackendContext.Raw,
		InitialAccessInfo:  in.Status.InitialAccessInfo,
		TargetAccessInfo:   in.Status.TargetAccessInfo,
		DeleteAfterSuccess: deleteAfterSuccess,
	}, nil
}

func (in *TridentVolumeMove) Validate() error {
	if deleteAfter := in.Spec.DeleteAfterSuccess; deleteAfter != nil && deleteAfter.Duration < time.Duration(0) {
		return fmt.Errorf("TridentVolumeMove delete after success is negative")
	}
	if in.Spec.TargetPool == "" {
		return fmt.Errorf("TridentVolumeMove target pool is empty")
	}
	if in.Spec.TargetNode == "" {
		return fmt.Errorf("TridentVolumeMove target node is empty")
	}
	if in.Spec.SourcePool == "" {
		return fmt.Errorf("TridentVolumeMove source pool is empty")
	}
	if in.Spec.SourceNode == "" {
		return fmt.Errorf("TridentVolumeMove source node is empty")
	}
	return nil
}

func (in *TridentVolumeMove) GetObjectMeta() metav1.ObjectMeta {
	return in.ObjectMeta
}

func (in *TridentVolumeMove) GetKind() string {
	return "TridentVolumeMove"
}

func (in *TridentVolumeMove) GetFinalizers() []string {
	if in.ObjectMeta.Finalizers != nil {
		return in.ObjectMeta.Finalizers
	}
	return []string{}
}

func (in *TridentVolumeMove) HasTridentFinalizers() bool {
	for _, finalizerName := range GetTridentFinalizers() {
		if collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			return true
		}
	}
	return false
}

func (in *TridentVolumeMove) AddTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		if !collection.ContainsString(in.ObjectMeta.Finalizers, finalizerName) {
			in.ObjectMeta.Finalizers = append(in.ObjectMeta.Finalizers, finalizerName)
		}
	}
}

func (in *TridentVolumeMove) RemoveTridentFinalizers() {
	for _, finalizerName := range GetTridentFinalizers() {
		in.ObjectMeta.Finalizers = collection.RemoveString(in.ObjectMeta.Finalizers, finalizerName)
	}
}

// IsNew indicates whether the volume move has not been started.
func (in *TridentVolumeMove) IsNew() bool {
	return in.Status.State == "" && in.Status.CompletionTime == nil && in.DeletionTimestamp == nil
}

// IsComplete indicates whether the volume move has been completed.
func (in *TridentVolumeMove) IsComplete() bool {
	return in.Status.CompletionTime != nil && !in.Status.CompletionTime.IsZero()
}

// Succeeded indicates whether the volume move succeeded.
func (in *TridentVolumeMove) Succeeded() bool {
	return in.Status.State == models.VolumeMoveStateSucceeded
}

// InProgress indicates whether the volume move is in progress.
func (in *TridentVolumeMove) InProgress() bool {
	return !in.IsNew() && !in.IsComplete()
}

// Failed indicates whether the volume move failed.
func (in *TridentVolumeMove) Failed() bool {
	return in.Status.State == models.VolumeMoveStateFailed
}

func (in *TridentVolumeMoveSpec) GetDeleteAfterSuccess() *time.Duration {
	if in == nil || in.DeleteAfterSuccess == nil {
		return nil
	}
	return &in.DeleteAfterSuccess.Duration
}

// AttachmentStatus returns a reference to a TridentVolumeMoveAttachmentStatus.
func (t *TridentVolumeMoveStatus) AttachmentStatus(nodeName string) *TridentVolumeMoveAttachmentStatus {
	for _, status := range t.Attachments {
		if status == nil {
			continue
		}
		if status.NodeName == nodeName || strings.EqualFold(status.NodeName, nodeName) {
			return status
		}
	}
	return nil
}

func (t *TridentVolumeMoveStatus) AttachmentMessages() string {
	if len(t.Attachments) == 0 {
		return ""
	}

	var messages []string
	for _, a := range t.Attachments {
		if a == nil {
			continue
		}
		messages = append(messages, a.Message)
	}

	if len(messages) == 0 {
		return ""
	}
	return strings.Join(messages, ", ")
}
