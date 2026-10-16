// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package objectprovisioner implements cosi/provisioner.Provisioner by calling
// core.Orchestrator, so the COSI frontend reaches real storage instead of
// cosi/provisioner/fake's in-memory maps. It lives outside cosi/, the same way
// frontend/csi.Plugin holds a core.Orchestrator directly, keeping cosi/'s vendored
// tree self-contained.
package objectprovisioner

import (
	"context"

	"github.com/netapp/trident/core"
	"github.com/netapp/trident/cosi/provisioner"
	"github.com/netapp/trident/object"
	"github.com/netapp/trident/utils/errors"
)

// DriverName is Trident's single COSI driver identity, matched against every
// Bucket.spec.driverName regardless of which object store backs the bucket.
const DriverName = "cosi.trident.netapp.io"

// Provisioner implements provisioner.Provisioner by calling core.Orchestrator.
type Provisioner struct {
	orchestrator core.Orchestrator
}

var _ provisioner.Provisioner = &Provisioner{}

// New returns a Provisioner that provisions buckets through orchestrator.
func New(orchestrator core.Orchestrator) *Provisioner {
	return &Provisioner{orchestrator: orchestrator}
}

// DriverInfo reports Trident's COSI driver identity. Static: it does not depend on which
// object stores are configured.
func (p *Provisioner) DriverInfo(context.Context) (*provisioner.DriverInfo, error) {
	return &provisioner.DriverInfo{
		Name:               DriverName,
		SupportedProtocols: []provisioner.Protocol{provisioner.ProtocolS3},
	}, nil
}

// CreateBucket provisions a bucket via core.Orchestrator.AddBucket. Parameters are passed
// through for AddBucket to match against each object store.
func (p *Provisioner) CreateBucket(
	ctx context.Context, req *provisioner.CreateBucketRequest,
) (*provisioner.BucketDetails, error) {
	bucket := &object.Bucket{
		Name: req.Name,
	}

	created, err := p.orchestrator.AddBucket(ctx, bucket, req.Parameters)
	if err != nil {
		return nil, translateError(err)
	}

	return &provisioner.BucketDetails{
		BucketID: created.ProvisionerID(),
		Info:     toProvisionerBucketInfo(created.StorageID(), created.Info),
	}, nil
}

// GetBucket looks up a bucket that is expected to already exist, for COSI static provisioning.
func (p *Provisioner) GetBucket(
	ctx context.Context, req *provisioner.GetBucketRequest,
) (*provisioner.BucketDetails, error) {
	bucket, err := p.orchestrator.GetBucket(ctx, req.BucketID)
	if err != nil {
		return nil, translateError(err)
	}

	return &provisioner.BucketDetails{
		BucketID: bucket.ProvisionerID(),
		Info:     toProvisionerBucketInfo(bucket.StorageID(), bucket.Info),
	}, nil
}

// DeleteBucket removes a bucket via core.Orchestrator.DeleteBucket, which is already
// idempotent on an absent bucket.
func (p *Provisioner) DeleteBucket(ctx context.Context, req *provisioner.DeleteBucketRequest) error {
	return translateError(p.orchestrator.DeleteBucket(ctx, req.BucketID))
}

// GrantAccess provisions access via core.Orchestrator.GrantBucketAccess, then fetches each
// granted bucket's connection info with one GetBucket call per bucket, since GrantBucketAccess
// does not return it directly.
func (p *Provisioner) GrantAccess(
	ctx context.Context, req *provisioner.GrantAccessRequest,
) (*provisioner.AccessDetails, error) {
	modes := toObjectAccessModes(req.Buckets)

	access, secret, err := p.orchestrator.GrantBucketAccess(ctx, modes, req.AccountName)
	if err != nil {
		return nil, translateGrantAccessError(err)
	}

	granted := make([]provisioner.GrantedBucket, 0, len(modes))
	for _, mode := range modes {
		// Use the bucket name to look up the bucket.
		bucket, err := p.orchestrator.GetBucket(ctx, mode.BucketName)
		if err != nil {
			return nil, translateError(err)
		}
		granted = append(granted, provisioner.GrantedBucket{
			BucketID: bucket.ProvisionerID(),
			Info:     toProvisionerBucketInfo(bucket.StorageID(), bucket.Info),
		})
	}

	return &provisioner.AccessDetails{
		AccountID:   access.ProvisionerID(),
		Buckets:     granted,
		Credentials: toProvisionerCredentials(secret),
	}, nil
}

// RevokeAccess removes a grant via core.Orchestrator.RevokeBucketAccess.
//
// Unlike DeleteBucket, RevokeBucketAccess returns NotFoundError for an already-absent grant.
// This method must be idempotent, so that case is treated as success here.
func (p *Provisioner) RevokeAccess(ctx context.Context, req *provisioner.RevokeAccessRequest) error {
	err := p.orchestrator.RevokeBucketAccess(ctx, req.AccountID)
	if errors.IsNotFoundError(err) {
		return nil
	}
	return translateError(err)
}
