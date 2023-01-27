// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

//go:generate mockgen -destination=../../../mocks/mock_storage_drivers/mock_gcp/mock_api.go github.com/netapp/trident/storage_drivers/gcp/api GCPClient

import (
	"context"
	"net/http"
	"time"

	versionutils "github.com/netapp/trident/utils/version"
)

type GCPClient interface {
	InvokeAPI(
		ctx context.Context, requestBody []byte, method, gcpURL string,
	) (*http.Response, []byte, error)
	GetVersion(ctx context.Context) (*versionutils.Version, *versionutils.Version, error)
	GetServiceLevels(ctx context.Context) (map[string]string, error)

	GetVolumes(ctx context.Context) (*[]Volume, error)
	GetVolumeByName(ctx context.Context, name string) (*Volume, error)
	GetVolumeByCreationToken(ctx context.Context, creationToken string) (*Volume, error)
	VolumeExistsByCreationToken(ctx context.Context, creationToken string) (bool, *Volume, error)
	GetVolumeByID(ctx context.Context, volumeId string) (*Volume, error)
	WaitForVolumeStates(
		ctx context.Context, volume *Volume, desiredStates, abortStates []string,
		maxElapsedTime time.Duration,
	) (string, error)
	CreateVolume(ctx context.Context, request *VolumeCreateRequest) error
	RenameVolume(ctx context.Context, volume *Volume, newName string) (*Volume, error)
	ChangeVolumeUnixPermissions(ctx context.Context, volume *Volume, newUnixPermissions string) (*Volume, error)
	RelabelVolume(ctx context.Context, volume *Volume, labels []string) (*Volume, error)
	RenameRelabelVolume(
		ctx context.Context, volume *Volume, newName string, labels []string,
	) (*Volume, error)
	ResizeVolume(ctx context.Context, volume *Volume, newSizeBytes int64) (*Volume, error)
	DeleteVolume(ctx context.Context, volume *Volume) error

	GetSnapshotsForVolume(ctx context.Context, volume *Volume) (*[]Snapshot, error)
	GetSnapshotForVolume(ctx context.Context, volume *Volume, snapshotName string) (*Snapshot, error)
	GetSnapshotByID(ctx context.Context, snapshotId string) (*Snapshot, error)
	WaitForSnapshotState(
		ctx context.Context, snapshot *Snapshot, desiredState string, abortStates []string,
		maxElapsedTime time.Duration,
	) error
	CreateSnapshot(ctx context.Context, request *SnapshotCreateRequest) error
	RestoreSnapshot(ctx context.Context, volume *Volume, snapshot *Snapshot) error
	DeleteSnapshot(ctx context.Context, volume *Volume, snapshot *Snapshot) error

	GetBackupsForVolume(ctx context.Context, volume *Volume) (*[]Backup, error)
	GetBackupForVolume(ctx context.Context, volume *Volume, backupName string) (*Backup, error)
	GetBackupByID(ctx context.Context, backupId string) (*Backup, error)
	WaitForBackupStates(
		ctx context.Context, backup *Backup, desiredStates, abortStates []string,
		maxElapsedTime time.Duration,
	) error
	CreateBackup(ctx context.Context, request *BackupCreateRequest) error
	DeleteBackup(ctx context.Context, volume *Volume, backup *Backup) error
	GetPools(ctx context.Context) (*[]*Pool, error)
}
