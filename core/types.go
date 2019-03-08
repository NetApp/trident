// Copyright 2019 NetApp, Inc. All Rights Reserved.

package core

import (
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils"
)

type Orchestrator interface {
	Bootstrap() error
	AddFrontend(f frontend.Plugin)
	GetVersion() (string, error)

	AddBackend(configJSON string) (*storage.BackendExternal, error)
	UpdateBackend(backendName, configJSON string) (storageBackendExternal *storage.BackendExternal, err error)
	UpdateBackendState(backendName, backendState string) (storageBackendExternal *storage.BackendExternal, err error)
	GetBackend(backend string) (*storage.BackendExternal, error)
	ListBackends() ([]*storage.BackendExternal, error)
	DeleteBackend(backend string) error

	AddVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	CloneVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	GetVolume(volume string) (*storage.VolumeExternal, error)
	GetDriverTypeForVolume(vol *storage.VolumeExternal) (string, error)
	GetVolumeType(vol *storage.VolumeExternal) (config.VolumeType, error)
	ListVolumes() ([]*storage.VolumeExternal, error)
	DeleteVolume(volume string) error
	ListVolumesByPlugin(pluginName string) ([]*storage.VolumeExternal, error)
	PublishVolume(volumeName string, publishInfo *utils.VolumePublishInfo) error
	AttachVolume(volumeName, mountpoint string, publishInfo *utils.VolumePublishInfo) error
	DetachVolume(volumeName, mountpoint string) error
	ListVolumeSnapshots(volumeName string) ([]*storage.SnapshotExternal, error)
	ReloadVolumes() error
	ResizeVolume(volumeName, newSize string) error

	AddStorageClass(scConfig *storageclass.Config) (*storageclass.External, error)
	GetStorageClass(scName string) (*storageclass.External, error)
	ListStorageClasses() ([]*storageclass.External, error)
	DeleteStorageClass(scName string) error

	AddNode(node *utils.Node) error
	GetNode(nName string) (*utils.Node, error)
	ListNodes() ([]*utils.Node, error)
	DeleteNode(nName string) error
}

type NotReadyError struct {
	message string
}

func (e *NotReadyError) Error() string { return e.message }

type BootstrapError struct {
	message string
}

func (e *BootstrapError) Error() string { return e.message }

type NotFoundError struct {
	message string
}

func (e *NotFoundError) Error() string { return e.message }
