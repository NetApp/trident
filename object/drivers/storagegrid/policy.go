// Copyright 2026 NetApp, Inc. All Rights Reserved.

package storagegrid

import (
	"fmt"

	"github.com/netapp/trident/object"
	"github.com/netapp/trident/object/drivers/storagegrid/sgapi"
)

// Bucket scoping happens entirely in the group's S3 policy, so the action sets
// below are the whole of what an access mode means.
//
// Each bucket contributes two statements. Bucket-level actions such as
// s3:ListBucket are evaluated against the bucket ARN while object actions are
// evaluated against the object ARN, and a policy carrying only one of the two
// forms fails silently rather than being rejected.
var (
	readWriteBucketActions = []string{
		"s3:ListBucket",
		"s3:ListBucketVersions",
		"s3:ListBucketMultipartUploads",
		"s3:GetBucketLocation",
	}
	readWriteObjectActions = []string{
		"s3:GetObject",
		"s3:GetObjectVersion",
		"s3:PutObject",
		"s3:DeleteObject",
		"s3:DeleteObjectVersion",
		"s3:ListMultipartUploadParts",
		"s3:AbortMultipartUpload",
		"s3:GetObjectTagging",
		"s3:PutObjectTagging",
		"s3:DeleteObjectTagging",
	}

	readOnlyBucketActions = []string{
		"s3:ListBucket",
		"s3:ListBucketVersions",
		"s3:GetBucketLocation",
	}
	readOnlyObjectActions = []string{
		"s3:GetObject",
		"s3:GetObjectVersion",
		"s3:GetObjectTagging",
	}

	writeOnlyBucketActions = []string{
		"s3:ListBucketMultipartUploads",
		"s3:GetBucketLocation",
	}
	writeOnlyObjectActions = []string{
		"s3:PutObject",
		"s3:AbortMultipartUpload",
		"s3:ListMultipartUploadParts",
		"s3:PutObjectTagging",
	}
)

func actionsFor(mode object.AccessMode) (bucketActions, objectActions []string, err error) {
	switch mode {
	case object.AccessModeReadWrite:
		return readWriteBucketActions, readWriteObjectActions, nil
	case object.AccessModeRead:
		return readOnlyBucketActions, readOnlyObjectActions, nil
	case object.AccessModeWrite:
		return writeOnlyBucketActions, writeOnlyObjectActions, nil
	default:
		return nil, nil, fmt.Errorf("unsupported access mode %q", mode)
	}
}

// BuildGroupPolicy renders the scoped S3 policy for a set of accessed buckets.
func BuildGroupPolicy(modes []object.BucketAccessMode) (*sgapi.S3Policy, error) {
	if len(modes) == 0 {
		return nil, fmt.Errorf("access to at least one bucket must be requested")
	}

	statements := make([]sgapi.S3Statement, 0, len(modes)*2)
	for i, mode := range modes {
		bucketActions, objectActions, err := actionsFor(mode.AccessMode)
		if err != nil {
			return nil, fmt.Errorf("bucket %q: %w", mode.BucketName, err)
		}

		bucketARN := fmt.Sprintf("arn:aws:s3:::%s", mode.BucketName)
		statements = append(statements,
			sgapi.S3Statement{
				Sid:      fmt.Sprintf("BucketLevel%d", i),
				Effect:   "Allow",
				Action:   bucketActions,
				Resource: []string{bucketARN},
			},
			sgapi.S3Statement{
				Sid:      fmt.Sprintf("ObjectLevel%d", i),
				Effect:   "Allow",
				Action:   objectActions,
				Resource: []string{bucketARN + "/*"},
			},
		)
	}
	return &sgapi.S3Policy{Statement: statements}, nil
}
