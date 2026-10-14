// Copyright 2026 NetApp, Inc. All Rights Reserved.

package objectprovisioner

import (
	"github.com/netapp/trident/cosi/provisioner"
	"github.com/netapp/trident/object"
)

// toProvisionerBucketInfo converts object.BucketInfo to the provisioner shape. bucketID is
// passed in separately because provisioner.S3BucketInfo repeats it per-protocol, while
// object.S3BucketInfo does not carry it.
func toProvisionerBucketInfo(bucketID string, info object.BucketInfo) provisioner.BucketInfo {
	var out provisioner.BucketInfo
	if info.S3 != nil {
		out.S3 = &provisioner.S3BucketInfo{
			BucketID:        bucketID,
			Endpoint:        info.S3.Endpoint,
			Region:          info.S3.Region,
			AddressingStyle: provisioner.S3AddressingStyle(info.S3.AddressingStyle),
		}
	}
	return out
}

// toProvisionerCredentials converts the secret GrantBucketAccess returns to the
// provisioner-facing shape. A nil secret (no driver currently returns one for a successful
// grant) converts to a zero-value Credentials.
func toProvisionerCredentials(secret *object.S3BucketAccessSecret) provisioner.Credentials {
	var out provisioner.Credentials
	if secret != nil {
		out.S3 = &provisioner.S3Credentials{
			AccessKeyID:     secret.AccessKeyID,
			AccessSecretKey: secret.SecretAccessKey,
		}
	}
	return out
}

// toObjectAccessModes converts a GrantAccessRequest's buckets to core's shape. BucketID and
// BucketName are the same value under different names; AccessMode's string values already
// match, so this is a direct cast.
func toObjectAccessModes(buckets []provisioner.AccessedBucket) []object.BucketAccessMode {
	modes := make([]object.BucketAccessMode, 0, len(buckets))
	for _, b := range buckets {
		modes = append(modes, object.BucketAccessMode{
			BucketName: b.BucketID,
			AccessMode: object.AccessMode(b.AccessMode),
		})
	}
	return modes
}
