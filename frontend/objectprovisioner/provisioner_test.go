// Copyright 2026 NetApp, Inc. All Rights Reserved.

package objectprovisioner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/cosi/provisioner"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	"github.com/netapp/trident/object"
	"github.com/netapp/trident/utils/errors"
)

func newTestProvisioner(t *testing.T) (*Provisioner, *mockcore.MockOrchestrator) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(ctrl)
	return New(mockOrchestrator), mockOrchestrator
}

func TestDriverInfo(t *testing.T) {
	p, _ := newTestProvisioner(t)

	info, err := p.DriverInfo(context.Background())

	require.NoError(t, err)
	assert.Equal(t, DriverName, info.Name)
	assert.Equal(t, []provisioner.Protocol{provisioner.ProtocolS3}, info.SupportedProtocols)
}

func TestCreateBucket(t *testing.T) {
	t.Run("maps request fields into object.Bucket and result into BucketDetails", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().AddBucket(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, bucket *object.Bucket, params map[string]string) (*object.Bucket, error) {
				assert.Equal(t, "my-bucket", bucket.Name)
				assert.Equal(t, map[string]string{"objectStoreName": "store-a"}, params)
				return &object.Bucket{
					Name:       bucket.Name,
					InternalID: bucket.Name, // StorageID() returns InternalID; set it so S3BucketInfo.BucketID is populated.
					StoreUUID:  "uuid-store-a",
					Info: object.BucketInfo{
						S3: &object.S3BucketInfo{Endpoint: "https://s3.example.com", Region: "us-east-1", AddressingStyle: "Path"},
					},
				}, nil
			})

		details, err := p.CreateBucket(context.Background(), &provisioner.CreateBucketRequest{
			Name:       "my-bucket",
			Parameters: map[string]string{"objectStoreName": "store-a"},
		})

		require.NoError(t, err)
		assert.Equal(t, "my-bucket", details.BucketID)
		require.NotNil(t, details.Info.S3)
		assert.Equal(t, "my-bucket", details.Info.S3.BucketID)
		assert.Equal(t, "https://s3.example.com", details.Info.S3.Endpoint)
		assert.Equal(t, "us-east-1", details.Info.S3.Region)
		assert.Equal(t, provisioner.S3AddressingStylePath, details.Info.S3.AddressingStyle)
	})

	t.Run("missing objectStoreName parameter leaves the bucket unplaced", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().AddBucket(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, bucket *object.Bucket, params map[string]string) (*object.Bucket, error) {
				assert.Empty(t, bucket.StoreUUID)
				assert.Empty(t, params)
				return bucket, nil
			})

		_, err := p.CreateBucket(context.Background(), &provisioner.CreateBucketRequest{Name: "b"})
		require.NoError(t, err)
	})

	t.Run("translates an already-exists error", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().AddBucket(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.AlreadyExistsError("bucket b already exists"))

		_, err := p.CreateBucket(context.Background(), &provisioner.CreateBucketRequest{Name: "b"})
		assert.ErrorIs(t, err, provisioner.ErrAlreadyExists)
	})
}

func TestGetBucket(t *testing.T) {
	t.Run("returns bucket details on success", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().GetBucket(gomock.Any(), "b").Return(&object.Bucket{
			Name: "b", StoreUUID: "uuid-store-a",
			Info: object.BucketInfo{S3: &object.S3BucketInfo{Endpoint: "https://s3.example.com"}},
		}, nil)

		details, err := p.GetBucket(context.Background(), &provisioner.GetBucketRequest{BucketID: "b"})

		require.NoError(t, err)
		assert.Equal(t, "b", details.BucketID)
		require.NotNil(t, details.Info.S3)
		assert.Equal(t, "https://s3.example.com", details.Info.S3.Endpoint)
	})

	t.Run("translates not found", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().GetBucket(gomock.Any(), "missing").
			Return(nil, errors.NotFoundError("bucket missing not found"))

		_, err := p.GetBucket(context.Background(), &provisioner.GetBucketRequest{BucketID: "missing"})
		assert.ErrorIs(t, err, provisioner.ErrNotFound)
	})
}

func TestDeleteBucket(t *testing.T) {
	t.Run("passes BucketID through and succeeds", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().DeleteBucket(gomock.Any(), "b").Return(nil)

		err := p.DeleteBucket(context.Background(), &provisioner.DeleteBucketRequest{BucketID: "b"})
		require.NoError(t, err)
	})

	t.Run("translates an invalid-argument error", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().DeleteBucket(gomock.Any(), "b").
			Return(errors.InvalidInputError("bad bucket name"))

		err := p.DeleteBucket(context.Background(), &provisioner.DeleteBucketRequest{BucketID: "b"})
		assert.ErrorIs(t, err, provisioner.ErrInvalidArgument)
	})

	t.Run("translates a bucket-not-empty error", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().DeleteBucket(gomock.Any(), "b").
			Return(errors.BucketNotEmptyError("bucket b is not empty"))

		err := p.DeleteBucket(context.Background(), &provisioner.DeleteBucketRequest{BucketID: "b"})
		assert.ErrorIs(t, err, provisioner.ErrBucketNotEmpty)
	})
}

func TestGrantAccess(t *testing.T) {
	t.Run("grants access and fetches per-bucket info for every bucket in the request", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().GrantBucketAccess(gomock.Any(), gomock.Any(), "acct").DoAndReturn(
			func(_ context.Context, modes []object.BucketAccessMode, name string) (*object.BucketAccess, *object.S3BucketAccessSecret, error) {
				require.Len(t, modes, 2)
				assert.Equal(t, "bucket-1", modes[0].BucketName)
				assert.Equal(t, object.AccessModeReadWrite, modes[0].AccessMode)
				assert.Equal(t, "bucket-2", modes[1].BucketName)
				assert.Equal(t, object.AccessModeRead, modes[1].AccessMode)
				return &object.BucketAccess{
					Name:      name,
					StoreUUID: "uuid-store-a",
				}, &object.S3BucketAccessSecret{AccessKeyID: "AKID", SecretAccessKey: "SECRET"}, nil
			})
		mockOrchestrator.EXPECT().GetBucket(gomock.Any(), "bucket-1").
			Return(&object.Bucket{Name: "bucket-1", Info: object.BucketInfo{S3: &object.S3BucketInfo{Endpoint: "e1"}}}, nil)
		mockOrchestrator.EXPECT().GetBucket(gomock.Any(), "bucket-2").
			Return(&object.Bucket{Name: "bucket-2", Info: object.BucketInfo{S3: &object.S3BucketInfo{Endpoint: "e2"}}}, nil)

		details, err := p.GrantAccess(context.Background(), &provisioner.GrantAccessRequest{
			AccountName: "acct",
			Buckets: []provisioner.AccessedBucket{
				{BucketID: "bucket-1", AccessMode: provisioner.AccessModeReadWrite},
				{BucketID: "bucket-2", AccessMode: provisioner.AccessModeReadOnly},
			},
		})

		require.NoError(t, err)
		assert.Equal(t, "acct", details.AccountID)
		require.Len(t, details.Buckets, 2)
		assert.Equal(t, "bucket-1", details.Buckets[0].BucketID)
		assert.Equal(t, "e1", details.Buckets[0].Info.S3.Endpoint)
		assert.Equal(t, "bucket-2", details.Buckets[1].BucketID)
		assert.Equal(t, "e2", details.Buckets[1].Info.S3.Endpoint)
		require.NotNil(t, details.Credentials.S3)
		assert.Equal(t, "AKID", details.Credentials.S3.AccessKeyID)
		assert.Equal(t, "SECRET", details.Credentials.S3.AccessSecretKey)
	})

	t.Run("translates a cross-store conflict to ErrMultiBucketAccessUnsupported", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().GrantBucketAccess(gomock.Any(), gomock.Any(), "acct").
			Return(nil, nil, errors.ConflictError("bucket bucket-2 is on a different store than bucket-1"))

		_, err := p.GrantAccess(context.Background(), &provisioner.GrantAccessRequest{
			AccountName: "acct",
			Buckets: []provisioner.AccessedBucket{
				{BucketID: "bucket-1", AccessMode: provisioner.AccessModeReadWrite},
				{BucketID: "bucket-2", AccessMode: provisioner.AccessModeReadWrite},
			},
		})

		assert.ErrorIs(t, err, provisioner.ErrMultiBucketAccessUnsupported)
	})

	t.Run("propagates a failure fetching per-bucket info after a successful grant", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().GrantBucketAccess(gomock.Any(), gomock.Any(), "acct").
			Return(&object.BucketAccess{Name: "acct"}, nil, nil)
		mockOrchestrator.EXPECT().GetBucket(gomock.Any(), "bucket-1").
			Return(nil, errors.NotFoundError("bucket bucket-1 not found"))

		_, err := p.GrantAccess(context.Background(), &provisioner.GrantAccessRequest{
			AccountName: "acct",
			Buckets:     []provisioner.AccessedBucket{{BucketID: "bucket-1", AccessMode: provisioner.AccessModeReadWrite}},
		})

		assert.ErrorIs(t, err, provisioner.ErrNotFound)
	})
}

func TestRevokeAccess(t *testing.T) {
	t.Run("passes AccountID through and succeeds", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().RevokeBucketAccess(gomock.Any(), "acct").Return(nil)

		err := p.RevokeAccess(context.Background(), &provisioner.RevokeAccessRequest{AccountID: "acct"})
		require.NoError(t, err)
	})

	t.Run("revoking an already-absent account succeeds, per the interface's idempotency contract", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().RevokeBucketAccess(gomock.Any(), "acct").
			Return(errors.NotFoundError("bucket access acct not found"))

		err := p.RevokeAccess(context.Background(), &provisioner.RevokeAccessRequest{AccountID: "acct"})
		assert.NoError(t, err, "RevokeAccess must be idempotent on an already-absent account")
	})

	t.Run("translates a non-not-found error", func(t *testing.T) {
		p, mockOrchestrator := newTestProvisioner(t)

		mockOrchestrator.EXPECT().RevokeBucketAccess(gomock.Any(), "acct").
			Return(errors.InvalidInputError("bad account id"))

		err := p.RevokeAccess(context.Background(), &provisioner.RevokeAccessRequest{AccountID: "acct"})
		assert.ErrorIs(t, err, provisioner.ErrInvalidArgument)
	})
}
