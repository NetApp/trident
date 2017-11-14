// Copyright 2016 NetApp, Inc. All Rights Reserved.

package core

import (
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
)

type Orchestrator interface {
	Bootstrap() error
	AddFrontend(f frontend.FrontendPlugin)
	GetVersion() string

	AddStorageBackend(configJSON string) (*storage.StorageBackendExternal, error)
	GetBackend(backend string) *storage.StorageBackendExternal
	ListBackends() []*storage.StorageBackendExternal
	OfflineBackend(backend string) (bool, error)

	AddVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	CloneVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	GetVolume(volume string) *storage.VolumeExternal
	GetDriverTypeForVolume(vol *storage.VolumeExternal) string
	GetVolumeType(vol *storage.VolumeExternal) config.VolumeType
	ListVolumes() []*storage.VolumeExternal
	DeleteVolume(volume string) (found bool, err error)
	ListVolumesByPlugin(pluginName string) []*storage.VolumeExternal
	AttachVolume(volumeName, mountpoint string, options map[string]string) error
	DetachVolume(volumeName, mountpoint string) error

	AddStorageClass(scConfig *storage_class.Config) (*storage_class.StorageClassExternal, error)
	GetStorageClass(scName string) *storage_class.StorageClassExternal
	ListStorageClasses() []*storage_class.StorageClassExternal
	DeleteStorageClass(scName string) (bool, error)
}
