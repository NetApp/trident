// Copyright 2023 NetApp, Inc. All Rights Reserved.

package awsapi

//go:generate mockgen -package mock_api -destination=../../../mocks/mock_storage_drivers/mock_ontap/mock_awsapi.go github.com/netapp/trident/storage_drivers/ontap/awsapi AWSAPI

import (
	"context"
	"time"

	"github.com/netapp/trident/storage"
)

type AWSAPI interface {
	CreateSecret(ctx context.Context, request *SecretCreateRequest) (*Secret, error)
	GetSecret(ctx context.Context, secretARN string) (*Secret, error)
	DeleteSecret(ctx context.Context, secretARN string) error

	GetFilesystems(ctx context.Context) (*[]*Filesystem, error)
	GetFilesystemByID(ctx context.Context, ID string) (*Filesystem, error)

	CreateSVM(ctx context.Context, request *SVMCreateRequest) (*SVM, error)
	GetSVMs(ctx context.Context) (*[]*SVM, error)
	GetSVMByID(ctx context.Context, ID string) (*SVM, error)

	GetVolumes(ctx context.Context) (*[]*Volume, error)
	GetVolumeByName(ctx context.Context, name string) (*Volume, error)
	GetVolumeByARN(ctx context.Context, volumeARN string) (*Volume, error)
	GetVolumeByID(ctx context.Context, volumeID string) (*Volume, error)
	GetVolume(ctx context.Context, volConfig *storage.VolumeConfig) (*Volume, error)
	VolumeExistsByName(ctx context.Context, name string) (bool, *Volume, error)
	VolumeExistsByARN(ctx context.Context, volumeARN string) (bool, *Volume, error)
	VolumeExistsByID(ctx context.Context, volumeID string) (bool, *Volume, error)
	VolumeExists(ctx context.Context, volConfig *storage.VolumeConfig) (bool, *Volume, error)
	WaitForVolumeStates(
		ctx context.Context, volume *Volume, desiredStates, abortStates []string, maxElapsedTime time.Duration,
	) (string, error)
	CreateVolume(ctx context.Context, request *VolumeCreateRequest) (*Volume, error)
	RelabelVolume(ctx context.Context, volume *Volume, labels map[string]string) error
	ResizeVolume(ctx context.Context, volume *Volume, newSizeBytes uint64) (*Volume, error)
	DeleteVolume(ctx context.Context, volume *Volume) error
}
