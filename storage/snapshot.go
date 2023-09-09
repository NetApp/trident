// Copyright 2020 NetApp, Inc. All Rights Reserved.

package storage

import (
	"fmt"
	"regexp"
)

const (
	SnapshotNameFormat = "20060102T150405Z"
)

var snapshotIDRegex = regexp.MustCompile(`^(?P<volume>[^\s/]+)/(?P<snapshot>[^\s/]+)$`)

type SnapshotConfig struct {
	Version             string   `json:"version,omitempty"`
	Name                string   `json:"name,omitempty"`
	InternalName        string   `json:"internalName,omitempty"`
	VolumeName          string   `json:"volumeName,omitempty"`
	VolumeInternalName  string   `json:"volumeInternalName,omitempty"`
	LUKSPassphraseNames []string `json:"luksPassphraseNames,omitempty"`
	ImportNotManaged    bool     `json:"importNotManaged"`
}

func (c *SnapshotConfig) ID() string {
	return MakeSnapshotID(c.VolumeName, c.Name)
}

func (c *SnapshotConfig) Validate() error {
	if c.Name == "" || c.VolumeName == "" {
		return fmt.Errorf("the following fields for \"Snapshot\" are mandatory: name and volumeName")
	}
	return nil
}

type Snapshot struct {
	Config    *SnapshotConfig
	Created   string        `json:"dateCreated"` // The UTC time that the snapshot was created, in RFC3339 format
	SizeBytes int64         `json:"size"`        // The size of the volume at the time the snapshot was created
	State     SnapshotState `json:"state"`
}

type SnapshotState string

const (
	SnapshotStateCreating       = SnapshotState("creating")  // Request accepted, data upload not started
	SnapshotStateUploading      = SnapshotState("uploading") // Snapshot cut, being uploaded, not ready for use
	SnapshotStateOnline         = SnapshotState("online")
	SnapshotStateMissingBackend = SnapshotState("missing_backend")
	SnapshotStateMissingVolume  = SnapshotState("missing_volume")
)

func (s SnapshotState) IsCreating() bool {
	return s == SnapshotStateCreating
}

func (s SnapshotState) IsUploading() bool {
	return s == SnapshotStateUploading
}

func (s SnapshotState) IsOnline() bool {
	return s == SnapshotStateOnline || s == ""
}

func (s SnapshotState) IsMissingBackend() bool {
	return s == SnapshotStateMissingBackend
}

func (s SnapshotState) IsMissingVolume() bool {
	return s == SnapshotStateMissingVolume
}

type SnapshotExternal struct {
	Snapshot
}

func (s *SnapshotExternal) ID() string {
	return MakeSnapshotID(s.Config.VolumeName, s.Config.Name)
}

type SnapshotPersistent struct {
	Snapshot
}

func NewSnapshot(config *SnapshotConfig, created string, sizeBytes int64, state SnapshotState) *Snapshot {
	return &Snapshot{
		Config:    config,
		Created:   created,
		SizeBytes: sizeBytes,
		State:     state,
	}
}

func (s *Snapshot) ConstructExternal() *SnapshotExternal {
	clone := s.ConstructClone()
	return &SnapshotExternal{Snapshot: *clone}
}

func (s *Snapshot) ConstructPersistent() *SnapshotPersistent {
	clone := s.ConstructClone()
	return &SnapshotPersistent{Snapshot: *clone}
}

func (s *Snapshot) ConstructClone() *Snapshot {
	return &Snapshot{
		Config: &SnapshotConfig{
			Version:             s.Config.Version,
			Name:                s.Config.Name,
			InternalName:        s.Config.InternalName,
			VolumeName:          s.Config.VolumeName,
			VolumeInternalName:  s.Config.VolumeInternalName,
			LUKSPassphraseNames: s.Config.LUKSPassphraseNames,
			ImportNotManaged:    s.Config.ImportNotManaged,
		},
		Created:   s.Created,
		SizeBytes: s.SizeBytes,
		State:     s.State,
	}
}

func (s *Snapshot) ID() string {
	return MakeSnapshotID(s.Config.VolumeName, s.Config.Name)
}

func (s *SnapshotPersistent) ConstructExternal() *SnapshotExternal {
	clone := s.ConstructClone()
	return &SnapshotExternal{Snapshot: *clone}
}

func MakeSnapshotID(volumeName, snapshotName string) string {
	return fmt.Sprintf("%s/%s", volumeName, snapshotName)
}

func ParseSnapshotID(snapshotID string) (string, string, error) {
	match := snapshotIDRegex.FindStringSubmatch(snapshotID)

	paramsMap := make(map[string]string)
	for i, name := range snapshotIDRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	volumeName, ok := paramsMap["volume"]
	if !ok {
		return "", "", fmt.Errorf("snapshot ID %s does not contain a volume name", volumeName)
	}
	snapshotName, ok := paramsMap["snapshot"]
	if !ok {
		return "", "", fmt.Errorf("snapshot ID %s does not contain a snapshot name", volumeName)
	}

	return volumeName, snapshotName, nil
}

type BySnapshotExternalID []*SnapshotExternal

func (a BySnapshotExternalID) Len() int { return len(a) }
func (a BySnapshotExternalID) Less(i, j int) bool {
	return MakeSnapshotID(a[i].Config.VolumeName, a[i].Config.Name) < MakeSnapshotID(a[j].Config.VolumeName, a[j].Config.Name)
}
func (a BySnapshotExternalID) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
