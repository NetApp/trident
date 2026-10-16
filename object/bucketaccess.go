// Copyright 2026 NetApp, Inc. All Rights Reserved.

package object

import "github.com/brunoga/deep"

type AccessMode string

const (
	AccessModeRead      AccessMode = "ReadOnly"
	AccessModeWrite     AccessMode = "WriteOnly"
	AccessModeReadWrite AccessMode = "ReadWrite"
)

type BucketAccess struct {
	Name       string
	StoreUUID  string
	InternalID string
}

type BucketAccessMode struct {
	BucketName string
	AccessMode AccessMode
}

// S3BucketAccessSecret contains the credentials returned when bucket access is granted.
// It is intentionally excluded from Trident persistence and must never be logged.
type S3BucketAccessSecret struct {
	AccessKeyID     string
	SecretAccessKey string
}

func (a *BucketAccess) SmartCopy() interface{} {
	return deep.MustCopy(a)
}

// ProvisionerID returns the Trident key identifier for this
// BucketAccess. This is distinct from StorageID.
func (a *BucketAccess) ProvisionerID() string { return a.Name }

// StorageID is the storage key identifier for this BucketAccess.
// This is distinct from ProvisionerID.
func (a *BucketAccess) StorageID() string { return a.InternalID }

func (a *BucketAccess) GetObjectStoreID() string {
	return a.StoreUUID
}
