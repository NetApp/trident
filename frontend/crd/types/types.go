// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crdtypes

// EventType represents the type of event (add, update, delete)
type EventType string

const (
	EventAdd         EventType = "add"
	EventUpdate      EventType = "update"
	EventForceUpdate EventType = "forceupdate"
	EventDelete      EventType = "delete"
)

// ObjectType represents the type of Kubernetes resource
type ObjectType string

const (
	ObjectTypeStorageClass                   ObjectType = "StorageClass"
	ObjectTypeTridentVolumePublication       ObjectType = "TridentVolumePublication"
	ObjectTypeTridentBackend                 ObjectType = "TridentBackend"
	ObjectTypeTridentAutogrowPolicy          ObjectType = "TridentAutogrowPolicy"
	ObjectTypeTridentBackendConfig           ObjectType = "TridentBackendConfig"
	ObjectTypeSecret                         ObjectType = "secret"
	ObjectTypeTridentMirrorRelationship      ObjectType = "TridentMirrorRelationship"
	ObjectTypeTridentActionMirrorUpdate      ObjectType = "TridentActionMirrorUpdate"
	ObjectTypeTridentSnapshotInfo            ObjectType = "TridentSnapshotInfo"
	ObjectTypeTridentActionSnapshotRestore   ObjectType = "TridentActionSnapshotRestore"
	ObjectTypeTridentNodeRemediation         ObjectType = "TridentNodeRemediation"
	ObjectTypeTridentAutogrowRequestInternal ObjectType = "TridentAutogrowRequestInternal"
)
