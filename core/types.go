// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

//go:generate mockgen -destination=../mocks/mock_core/mock_core.go github.com/netapp/trident/core Orchestrator

import (
	"context"
	"time"

	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/models"
)

type Orchestrator interface {
	Bootstrap(monitorTransactions bool) error
	AddFrontend(ctx context.Context, f frontend.Plugin)
	GetFrontend(ctx context.Context, name string) (frontend.Plugin, error)
	GetVersion(ctx context.Context) (string, error)

	AddBackend(ctx context.Context, configJSON, configRef string) (*storage.BackendExternal, error)
	DeleteBackend(ctx context.Context, backend string) error
	DeleteBackendByBackendUUID(ctx context.Context, backendName, backendUUID string) error
	GetBackend(ctx context.Context, backend string) (*storage.BackendExternal, error)
	GetBackendByBackendUUID(ctx context.Context, backendUUID string) (*storage.BackendExternal, error)
	ListBackends(ctx context.Context) ([]*storage.BackendExternal, error)
	UpdateBackend(
		ctx context.Context, backendName, configJSON, configRef string,
	) (storageBackendExternal *storage.BackendExternal, err error)
	UpdateBackendByBackendUUID(
		ctx context.Context, backendName, configJSON, backendUUID, configRef string,
	) (storageBackendExternal *storage.BackendExternal, err error)
	UpdateBackendState(
		ctx context.Context, backendName, backendState, userBackendState string,
	) (storageBackendExternal *storage.BackendExternal, err error)
	RemoveBackendConfigRef(ctx context.Context, backendUUID, configRef string) (err error)

	AddVolume(ctx context.Context, volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	UpdateVolume(ctx context.Context, volume string, volumeUpdateInfo *models.VolumeUpdateInfo) error
	UpdateVolumeLUKSPassphraseNames(ctx context.Context, volume string, passphraseNames *[]string) error
	AttachVolume(ctx context.Context, volumeName, mountpoint string, publishInfo *models.VolumePublishInfo) error
	CloneVolume(ctx context.Context, volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	DetachVolume(ctx context.Context, volumeName, mountpoint string) error
	DeleteVolume(ctx context.Context, volume string) error
	GetVolume(ctx context.Context, volumeName string) (*storage.VolumeExternal, error)
	GetVolumeByInternalName(ctx context.Context, volumeInternal string) (volume string, err error)
	GetVolumeForImport(ctx context.Context, volumeID, backendName string) (*storage.VolumeExternal, error)
	ImportVolume(ctx context.Context, volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	ListVolumes(ctx context.Context) ([]*storage.VolumeExternal, error)
	PublishVolume(ctx context.Context, volumeName string, publishInfo *models.VolumePublishInfo) error
	UnpublishVolume(ctx context.Context, volumeName, nodeName string) error
	ResizeVolume(ctx context.Context, volumeName, newSize string) error
	ReloadVolumes(ctx context.Context) error

	ListSubordinateVolumes(ctx context.Context, sourceVolumeName string) ([]*storage.VolumeExternal, error)
	GetSubordinateSourceVolume(ctx context.Context, subordinateVolumeName string) (*storage.VolumeExternal, error)

	CreateSnapshot(ctx context.Context, snapshotConfig *storage.SnapshotConfig) (*storage.SnapshotExternal, error)
	ImportSnapshot(
		ctx context.Context, snapshotConfig *storage.SnapshotConfig,
	) (*storage.SnapshotExternal, error)
	GetSnapshot(ctx context.Context, volumeName, snapshotName string) (*storage.SnapshotExternal, error)
	ListSnapshots(ctx context.Context) ([]*storage.SnapshotExternal, error)
	ListSnapshotsByName(ctx context.Context, snapshotName string) ([]*storage.SnapshotExternal, error)
	ListSnapshotsForGroup(ctx context.Context, groupName string) ([]*storage.SnapshotExternal, error)
	ListSnapshotsForVolume(ctx context.Context, volumeName string) ([]*storage.SnapshotExternal, error)
	ReadSnapshotsForVolume(ctx context.Context, volumeName string) ([]*storage.SnapshotExternal, error)
	RestoreSnapshot(ctx context.Context, volumeName, snapshotName string) error
	DeleteSnapshot(ctx context.Context, volumeName, snapshotName string) error

	CreateGroupSnapshot(ctx context.Context, config *storage.GroupSnapshotConfig) (*storage.GroupSnapshotExternal, error)
	DeleteGroupSnapshot(ctx context.Context, groupName string) error
	GetGroupSnapshot(ctx context.Context, groupName string) (*storage.GroupSnapshotExternal, error)
	ListGroupSnapshots(ctx context.Context) ([]*storage.GroupSnapshotExternal, error)

	AddStorageClass(ctx context.Context, scConfig *storageclass.Config) (*storageclass.External, error)
	UpdateStorageClass(ctx context.Context, scConfig *storageclass.Config) (*storageclass.External, error)
	DeleteStorageClass(ctx context.Context, scName string) error
	GetStorageClass(ctx context.Context, scName string) (*storageclass.External, error)
	ListStorageClasses(ctx context.Context) ([]*storageclass.External, error)

	AddNode(ctx context.Context, node *models.Node, nodeEventCallback NodeEventCallback) error
	UpdateNode(ctx context.Context, nodeName string, flags *models.NodePublicationStateFlags) error
	GetNode(ctx context.Context, nodeName string) (*models.NodeExternal, error)
	ListNodes(ctx context.Context) ([]*models.NodeExternal, error)
	DeleteNode(ctx context.Context, nodeName string) error
	PeriodicallyReconcileNodeAccessOnBackends()
	PeriodicallyReconcileBackendState(pollInterval time.Duration)

	ReconcileVolumePublications(ctx context.Context, attachedLegacyVolumes []*models.VolumePublicationExternal) error
	GetVolumePublication(ctx context.Context, volumeName, nodeName string) (*models.VolumePublication, error)
	ListVolumePublications(ctx context.Context) ([]*models.VolumePublicationExternal, error)
	ListVolumePublicationsForVolume(
		ctx context.Context, volumeName string,
	) (publications []*models.VolumePublicationExternal, err error)
	ListVolumePublicationsForNode(
		ctx context.Context, nodeName string,
	) (publications []*models.VolumePublicationExternal, err error)

	AddVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error
	GetVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) (*storage.VolumeTransaction, error)
	DeleteVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) error

	EstablishMirror(ctx context.Context, backendUUID, volumeName, localInternalVolumeName, remoteVolumeHandle, replicationPolicy,
		replicationSchedule string) error
	ReestablishMirror(ctx context.Context, backendUUID, volumeName, localInternalVolumeName, remoteVolumeHandle,
		replicationPolicy, replicationSchedule string) error
	PromoteMirror(ctx context.Context, backendUUID, volumeName, localInternalVolumeName, remoteVolumeHandle,
		snapshotHandle string) (bool, error)
	GetMirrorStatus(ctx context.Context, backendUUID, localInternalVolumeName, remoteVolumeHandle string) (string, error)
	CanBackendMirror(ctx context.Context, backendUUID string) (bool, error)
	ReleaseMirror(ctx context.Context, backendUUID, volumeName, localInternalVolumeName string) error
	GetReplicationDetails(ctx context.Context, backendUUID, localInternalVolumeName, remoteVolumeHandle string) (string, string, string, error)
	UpdateMirror(ctx context.Context, volumeName, snapshotName string) error
	CheckMirrorTransferState(ctx context.Context, volumeName string) (*time.Time, error)
	GetMirrorTransferTime(ctx context.Context, volumeName string) (*time.Time, error)

	GetCHAP(ctx context.Context, volumeName, nodeName string) (*models.IscsiChapInfo, error)

	GetLogLevel(ctx context.Context) (string, error)
	SetLogLevel(ctx context.Context, level string) error
	GetSelectedLoggingWorkflows(ctx context.Context) (string, error)
	ListLoggingWorkflows(ctx context.Context) ([]string, error)
	SetLoggingWorkflows(ctx context.Context, workflows string) error
	GetSelectedLogLayers(ctx context.Context) (string, error)
	ListLogLayers(ctx context.Context) ([]string, error)
	SetLogLayers(ctx context.Context, workflows string) error

	Stop()
}

type (
	VolumeCallback    func(*storage.VolumeExternal, string) error
	NodeEventCallback func(eventType, reason, message string)
)
