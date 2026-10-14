// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package adapter presents a provisioner.Provisioner to the COSI reconcilers.
//
// It implements cosiproto.ProvisionerClient directly over Go method calls, so COSI runs
// in-process with no gRPC server or sidecar container. This is Trident's only exposure to the
// pinned v1alpha2 proto.
package adapter

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"

	"github.com/netapp/trident/cosi/provisioner"
)

// Adapter wraps a Provisioner so the COSI reconcilers can call it.
type Adapter struct {
	provisioner provisioner.Provisioner
}

var (
	_ cosiproto.IdentityClient    = &Adapter{}
	_ cosiproto.ProvisionerClient = &Adapter{}
)

// New returns an Adapter serving the given Provisioner.
func New(p provisioner.Provisioner) *Adapter {
	return &Adapter{provisioner: p}
}

// DriverGetInfo reports the driver name and supported protocols.
func (a *Adapter) DriverGetInfo(
	ctx context.Context, _ *cosiproto.DriverGetInfoRequest, _ ...grpc.CallOption,
) (*cosiproto.DriverGetInfoResponse, error) {
	info, err := a.provisioner.DriverInfo(ctx)
	if err != nil {
		return nil, toStatus(err)
	}

	protocols, err := protocolsToProto(info.SupportedProtocols)
	if err != nil {
		return nil, toStatus(fmt.Errorf("driver reported an unusable protocol: %w", err))
	}

	return &cosiproto.DriverGetInfoResponse{
		Name:               info.Name,
		SupportedProtocols: protocols,
	}, nil
}

// DriverCreateBucket provisions a bucket on the backend.
func (a *Adapter) DriverCreateBucket(
	ctx context.Context, in *cosiproto.DriverCreateBucketRequest, _ ...grpc.CallOption,
) (*cosiproto.DriverCreateBucketResponse, error) {
	protocols, err := protocolsFromProto(in.GetProtocols())
	if err != nil {
		return nil, toStatus(err)
	}

	details, err := a.provisioner.CreateBucket(ctx, &provisioner.CreateBucketRequest{
		Name:       in.GetName(),
		Protocols:  protocols,
		Parameters: in.GetParameters(),
	})
	if err != nil {
		return nil, toStatus(err)
	}

	return &cosiproto.DriverCreateBucketResponse{
		BucketId:  details.BucketID,
		Protocols: bucketInfoToProto(details.Info),
	}, nil
}

// DriverGetBucket looks up a statically provisioned bucket.
func (a *Adapter) DriverGetBucket(
	ctx context.Context, in *cosiproto.DriverGetBucketRequest, _ ...grpc.CallOption,
) (*cosiproto.DriverGetBucketResponse, error) {
	protocols, err := protocolsFromProto(in.GetProtocols())
	if err != nil {
		return nil, toStatus(err)
	}

	details, err := a.provisioner.GetBucket(ctx, &provisioner.GetBucketRequest{
		BucketID:   in.GetBucketId(),
		Protocols:  protocols,
		Parameters: in.GetParameters(),
	})
	if err != nil {
		return nil, toStatus(err)
	}

	return &cosiproto.DriverGetBucketResponse{
		BucketId:  details.BucketID,
		Protocols: bucketInfoToProto(details.Info),
	}, nil
}

// DriverDeleteBucket removes a bucket from the backend.
func (a *Adapter) DriverDeleteBucket(
	ctx context.Context, in *cosiproto.DriverDeleteBucketRequest, _ ...grpc.CallOption,
) (*cosiproto.DriverDeleteBucketResponse, error) {
	err := a.provisioner.DeleteBucket(ctx, &provisioner.DeleteBucketRequest{
		BucketID:   in.GetBucketId(),
		Parameters: in.GetParameters(),
	})
	if err != nil {
		return nil, toStatus(err)
	}

	return &cosiproto.DriverDeleteBucketResponse{}, nil
}

// DriverGrantBucketAccess provisions an account with access to the given buckets.
func (a *Adapter) DriverGrantBucketAccess(
	ctx context.Context, in *cosiproto.DriverGrantBucketAccessRequest, _ ...grpc.CallOption,
) (*cosiproto.DriverGrantBucketAccessResponse, error) {
	protocol, err := protocolFromProto(in.GetProtocol())
	if err != nil {
		return nil, toStatus(err)
	}

	authType, err := authTypeFromProto(in.GetAuthenticationType())
	if err != nil {
		return nil, toStatus(err)
	}

	buckets := make([]provisioner.AccessedBucket, 0, len(in.GetBuckets()))
	for _, b := range in.GetBuckets() {
		mode, modeErr := accessModeFromProto(b.GetAccessMode())
		if modeErr != nil {
			return nil, toStatus(fmt.Errorf("bucket %q: %w", b.GetBucketId(), modeErr))
		}
		buckets = append(buckets, provisioner.AccessedBucket{
			BucketID:   b.GetBucketId(),
			AccessMode: mode,
		})
	}

	details, err := a.provisioner.GrantAccess(ctx, &provisioner.GrantAccessRequest{
		AccountName:        in.GetAccountName(),
		Protocol:           protocol,
		AuthenticationType: authType,
		ServiceAccountName: in.GetServiceAccountName(),
		Buckets:            buckets,
		Parameters:         in.GetParameters(),
	})
	if err != nil {
		return nil, toStatus(err)
	}

	granted := make([]*cosiproto.DriverGrantBucketAccessResponse_BucketInfo, 0, len(details.Buckets))
	for _, b := range details.Buckets {
		granted = append(granted, &cosiproto.DriverGrantBucketAccessResponse_BucketInfo{
			BucketId:   b.BucketID,
			BucketInfo: bucketInfoToProto(b.Info),
		})
	}

	return &cosiproto.DriverGrantBucketAccessResponse{
		AccountId:   details.AccountID,
		Buckets:     granted,
		Credentials: credentialsToProto(details.Credentials),
	}, nil
}

// DriverRevokeBucketAccess removes a previously granted account.
func (a *Adapter) DriverRevokeBucketAccess(
	ctx context.Context, in *cosiproto.DriverRevokeBucketAccessRequest, _ ...grpc.CallOption,
) (*cosiproto.DriverRevokeBucketAccessResponse, error) {
	protocol, err := protocolFromProto(in.GetProtocol())
	if err != nil {
		return nil, toStatus(err)
	}

	authType, err := authTypeFromProto(in.GetAuthenticationType())
	if err != nil {
		return nil, toStatus(err)
	}

	bucketIDs := make([]string, 0, len(in.GetBuckets()))
	for _, b := range in.GetBuckets() {
		bucketIDs = append(bucketIDs, b.GetBucketId())
	}

	err = a.provisioner.RevokeAccess(ctx, &provisioner.RevokeAccessRequest{
		AccountID:          in.GetAccountId(),
		Protocol:           protocol,
		AuthenticationType: authType,
		ServiceAccountName: in.GetServiceAccountName(),
		BucketIDs:          bucketIDs,
		Parameters:         in.GetParameters(),
	})
	if err != nil {
		return nil, toStatus(err)
	}

	return &cosiproto.DriverRevokeBucketAccessResponse{}, nil
}
