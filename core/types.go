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
	GetFrontend(name string) (frontend.Plugin, error)
	GetVersion() (string, error)

	AddBackend(configJSON string) (*storage.BackendExternal, error)
	DeleteBackend(backend string) error
	GetBackend(backend string) (*storage.BackendExternal, error)
	ListBackends() ([]*storage.BackendExternal, error)
	UpdateBackend(backendName, configJSON string) (storageBackendExternal *storage.BackendExternal, err error)
	UpdateBackendState(backendName, backendState string) (storageBackendExternal *storage.BackendExternal, err error)

	AddVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	AttachVolume(volumeName, mountpoint string, publishInfo *utils.VolumePublishInfo) error
	CloneVolume(volumeConfig *storage.VolumeConfig) (*storage.VolumeExternal, error)
	CreateVolumeFromSnapshot(snapshotID string, volumeConfig *storage.VolumeConfig) (
		externalVol *storage.VolumeExternal, err error)
	DetachVolume(volumeName, mountpoint string) error
	DeleteVolume(volume string) error
	GetVolume(volume string) (*storage.VolumeExternal, error)
	GetVolumeExternal(volumeName string, backendName string) (*storage.VolumeExternal, error)
	GetVolumeType(vol *storage.VolumeExternal) (config.VolumeType, error)
	ImportVolume(volumeConfig *storage.VolumeConfig, originalVolName string, backendName string, notManaged bool, createPVandPVC Operation) (*storage.VolumeExternal, error)
	ListVolumes() ([]*storage.VolumeExternal, error)
	ListVolumesByPlugin(pluginName string) ([]*storage.VolumeExternal, error)
	ListVolumeSnapshots(volumeName string) ([]*storage.SnapshotExternal, error)
	PublishVolume(volumeName string, publishInfo *utils.VolumePublishInfo) error
	ResizeVolume(volumeName, newSize string) error

	GetDriverTypeForVolume(vol *storage.VolumeExternal) (string, error)
	ReloadVolumes() error

	AddStorageClass(scConfig *storageclass.Config) (*storageclass.External, error)
	DeleteStorageClass(scName string) error
	GetStorageClass(scName string) (*storageclass.External, error)
	ListStorageClasses() ([]*storageclass.External, error)

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

type FoundError struct {
	message string
}

func (e *FoundError) Error() string { return e.message }

type UnsupportedError struct {
	message string
}

func (e *UnsupportedError) Error() string { return e.message }

type Operation func(*storage.VolumeExternal, string) error
