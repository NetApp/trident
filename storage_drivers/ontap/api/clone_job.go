// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils/errors"
)

// cloneJobVolumePathPrefix precedes the new volume's UUID in the description of an ONTAP volume create job.
// Measured against ONTAP 9.19.1 on 2026-10-13: a clone create job reads
// "POST /api/storage/volumes/<uuid>", 16 of 16 clones. Whatever follows the prefix is only trusted once it
// parses as a UUID, because a caller that took a wrong one would address a delete with the wrong identity.
const cloneJobVolumePathPrefix = "/api/storage/volumes/"

// CloneJobStatus is the state of the ONTAP job that creates a FlexClone.
type CloneJobStatus struct {
	// State is the ONTAP job state: queued, running, paused, success, or failure.
	State   string
	Code    string
	Message string
	// VolumeUUID is the UUID of the volume the job creates, taken from the job description. It is empty
	// when the description does not carry one.
	VolumeUUID string
}

// Failed reports whether the job ended without creating the volume.
func (s *CloneJobStatus) Failed() bool {
	return s.State == models.JobStateFailure
}

// Succeeded reports whether the job created the volume.
func (s *CloneJobStatus) Succeeded() bool {
	return s.State == models.JobStateSuccess
}

// CloneSplitState reports where a volume stands in the FlexClone lifecycle.
type CloneSplitState struct {
	// IsFlexclone is true while the volume still shares blocks with its parent.
	IsFlexclone bool
	// SplitInitiated is true once a split has been requested, including a split still in progress.
	SplitInitiated bool
	// ParentSnapshot is the snapshot the clone was created from. ONTAP reports it only while the volume is
	// still a FlexClone, so it is empty once a split has finished.
	ParentSnapshot string
}

// volumeUUIDFromCloneJobDescription extracts the new volume's UUID from a description such as
// "POST /api/storage/volumes/<uuid>".
func volumeUUIDFromCloneJobDescription(description string) string {
	idx := strings.Index(description, cloneJobVolumePathPrefix)
	if idx < 0 {
		return ""
	}
	candidate := description[idx+len(cloneJobVolumePathPrefix):]
	if end := strings.IndexAny(candidate, "/? "); end >= 0 {
		candidate = candidate[:end]
	}
	// A description in a shape not seen before must not become a volume identity that is trusted later.
	if _, err := uuid.Parse(candidate); err != nil {
		return ""
	}
	return candidate
}

// CloneJobStatus reads the state of a clone create job, and the UUID of the volume it creates, by job UUID.
// It never resolves a volume by name.
func (d OntapAPIREST) CloneJobStatus(ctx context.Context, jobUUID string) (*CloneJobStatus, error) {
	result, err := d.api.JobGet(ctx, jobUUID, []string{"state", "code", "message", "description"})
	if err != nil {
		if isRestNotFoundError(err) {
			// ONTAP drops a finished job from its history after a few minutes.
			return nil, errors.NotFoundError("job %s not found", jobUUID)
		}
		return nil, fmt.Errorf("get job %s: %w", jobUUID, err)
	}
	if result == nil || result.Payload == nil || result.Payload.State == nil {
		return nil, fmt.Errorf("job %s has no state", jobUUID)
	}

	job := result.Payload
	status := &CloneJobStatus{State: *job.State}
	if job.Code != nil {
		status.Code = strconv.FormatInt(*job.Code, 10)
	}
	if job.Message != nil {
		status.Message = *job.Message
	}
	if job.Description != nil {
		status.VolumeUUID = volumeUUIDFromCloneJobDescription(*job.Description)
		if status.VolumeUUID == "" && status.State == models.JobStateFailure {
			Logc(ctx).WithFields(LogFields{
				"jobUUID":     jobUUID,
				"description": *job.Description,
			}).Warning("Failed clone job description carries no volume UUID; its volume may be left behind.")
		}
	}
	return status, nil
}

// VolumeCloneSplitState reads whether a volume is still a FlexClone and whether a split has started.
func (d OntapAPIREST) VolumeCloneSplitState(ctx context.Context, name string) (*CloneSplitState, error) {
	volume, err := d.api.VolumeGetByName(ctx, name, []string{"clone.is_flexclone", "clone.split_initiated", "clone.parent_snapshot.name"})
	if err != nil {
		return nil, fmt.Errorf("get volume %s clone state: %w", name, err)
	}
	if volume == nil {
		return nil, errors.NotFoundError("could not find volume: %v", name)
	}

	// ONTAP reports is_flexclone explicitly, true while the volume shares blocks with its parent and false
	// once a split has finished. An absent value is not evidence of either, so it is an error.
	if volume.Clone == nil || volume.Clone.IsFlexclone == nil {
		return nil, fmt.Errorf("volume %s did not report whether it is a FlexClone", name)
	}

	state := &CloneSplitState{
		IsFlexclone:    *volume.Clone.IsFlexclone,
		SplitInitiated: convert.ToVal(volume.Clone.SplitInitiated),
	}
	if volume.Clone.ParentSnapshot != nil {
		state.ParentSnapshot = convert.ToVal(volume.Clone.ParentSnapshot.Name)
	}
	return state, nil
}
