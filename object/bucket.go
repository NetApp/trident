// Copyright 2026 NetApp, Inc. All Rights Reserved.

package object

import "github.com/brunoga/deep"

type Bucket struct {
	Name       string
	InternalID string
	StoreUUID  string
	// Parameters are transient BucketClass selectors and are not persisted with the bucket.
	Parameters map[string]any
	// Info is populated by the driver on CreateBucket/GetBucket.
	Info BucketInfo
}

// BucketInfo carries connection details, one field per protocol.
type BucketInfo struct {
	S3 *S3BucketInfo
}

// S3BucketInfo describes how to reach a bucket over S3.
type S3BucketInfo struct {
	Endpoint        string
	Region          string
	AddressingStyle string
}

func (b *Bucket) SmartCopy() interface{} { return deep.MustCopy(b) }

// ProvisionerID returns the Trident key identifier for this bucket.
// This is distinct from StorageID.
func (b *Bucket) ProvisionerID() string { return b.Name }

// StorageID returns the storage key identifier for this bucket.
// This is distinct from ProvisionerID.
func (b *Bucket) StorageID() string { return b.InternalID }

func (b *Bucket) GetObjectStoreID() string { return b.StoreUUID }
