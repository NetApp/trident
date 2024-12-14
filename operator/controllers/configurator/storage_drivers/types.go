// Copyright 2024 NetApp, Inc. All Rights Reserved.

package storage_drivers

type Backend interface {
	Validate() error
	Create() ([]string, error)
	CreateStorageClass() error
	CreateSnapshotClass() error
	GetCloudProvider() string
	DeleteBackend(map[string]interface{}) error
	DeleteStorageClass(map[string]interface{}) error
	DeleteSnapshotClass() error
}
