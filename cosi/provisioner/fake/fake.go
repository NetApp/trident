// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package fake provides an in-memory provisioner.Provisioner for tests and for exercising the
// COSI reconcilers without a storage backend. It honors the idempotency contract: CreateBucket
// and GrantAccess return existing state for a name or account already seen, rather than
// minting a fresh identity that would let stranded-resource bugs pass every test.
package fake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	. "github.com/netapp/trident/logging"

	"github.com/netapp/trident/cosi/provisioner"
)

const (
	// DriverName is the driver name the fake reports. Bucket objects must carry
	// it in spec.driverName to be reconciled.
	DriverName = "fake.cosi.trident.netapp.io"

	defaultEndpoint = "https://fake-s3.trident.netapp.io"
	defaultRegion   = "fake-region-1"
)

// Provisioner is an in-memory Provisioner. The zero value is not usable; call
// New. It is safe for concurrent use.
type Provisioner struct {
	mutex    sync.Mutex
	buckets  map[string]*bucket
	accounts map[string]*account

	// Calls counts invocations by method name, for assertions about
	// idempotency and retry behavior.
	Calls map[string]int

	// Errors injects a failure for the named method ("CreateBucket", "GetBucket",
	// "DeleteBucket", "GrantAccess", "RevokeAccess", "DriverInfo"). Wrap a provisioner
	// sentinel to exercise a specific status code.
	Errors map[string]error

	// SupportedProtocols is reported by DriverInfo. Defaults to S3.
	SupportedProtocols []provisioner.Protocol
}

type bucket struct {
	id         string
	name       string
	protocols  []provisioner.Protocol
	parameters map[string]string
}

type account struct {
	id          string
	name        string
	credentials provisioner.Credentials
	bucketIDs   []string
}

var _ provisioner.Provisioner = &Provisioner{}

// New returns an empty fake provisioner supporting S3.
func New() *Provisioner {
	return &Provisioner{
		buckets:            make(map[string]*bucket),
		accounts:           make(map[string]*account),
		Calls:              make(map[string]int),
		Errors:             make(map[string]error),
		SupportedProtocols: []provisioner.Protocol{provisioner.ProtocolS3},
	}
}

// record notes a call and returns any injected error. The caller must hold the
// mutex.
func (p *Provisioner) record(method string) error {
	p.Calls[method]++
	return p.Errors[method]
}

// DriverInfo reports the fake driver name and its protocols.
func (p *Provisioner) DriverInfo(ctx context.Context) (*provisioner.DriverInfo, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if err := p.record("DriverInfo"); err != nil {
		return nil, err
	}

	return &provisioner.DriverInfo{
		Name:               DriverName,
		SupportedProtocols: p.SupportedProtocols,
	}, nil
}

// CreateBucket provisions a bucket, or returns the existing one if the name has
// been seen before.
func (p *Provisioner) CreateBucket(
	ctx context.Context, req *provisioner.CreateBucketRequest,
) (*provisioner.BucketDetails, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if err := p.record("CreateBucket"); err != nil {
		return nil, err
	}

	if req.Name == "" {
		return nil, fmt.Errorf("%w: bucket name is required", provisioner.ErrInvalidArgument)
	}

	for _, protocol := range req.Protocols {
		if !p.supports(protocol) {
			return nil, fmt.Errorf("%w: protocol %q is not supported", provisioner.ErrInvalidArgument, protocol)
		}
	}

	id := bucketID(req.Name)

	if existing, ok := p.buckets[id]; ok {
		if !sameParameters(existing.parameters, req.Parameters) {
			return nil, fmt.Errorf("%w: bucket %q exists with different parameters",
				provisioner.ErrAlreadyExists, req.Name)
		}
		Logc(ctx).WithField("bucketID", id).Debug("Fake provisioner returning existing bucket.")
		return details(existing), nil
	}

	created := &bucket{
		id:         id,
		name:       req.Name,
		protocols:  req.Protocols,
		parameters: req.Parameters,
	}
	p.buckets[id] = created

	Logc(ctx).WithFields(LogFields{
		"bucketID": id,
		"name":     req.Name,
	}).Debug("Fake provisioner created bucket.")

	return details(created), nil
}

// GetBucket returns a bucket that already exists.
func (p *Provisioner) GetBucket(
	ctx context.Context, req *provisioner.GetBucketRequest,
) (*provisioner.BucketDetails, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if err := p.record("GetBucket"); err != nil {
		return nil, err
	}

	existing, ok := p.buckets[req.BucketID]
	if !ok {
		return nil, fmt.Errorf("%w: bucket %q", provisioner.ErrNotFound, req.BucketID)
	}

	return details(existing), nil
}

// DeleteBucket removes a bucket. Deleting an absent bucket succeeds.
func (p *Provisioner) DeleteBucket(ctx context.Context, req *provisioner.DeleteBucketRequest) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if err := p.record("DeleteBucket"); err != nil {
		return err
	}

	delete(p.buckets, req.BucketID)
	Logc(ctx).WithField("bucketID", req.BucketID).Debug("Fake provisioner deleted bucket.")

	return nil
}

// GrantAccess issues credentials, reusing them if the account name is known.
func (p *Provisioner) GrantAccess(
	ctx context.Context, req *provisioner.GrantAccessRequest,
) (*provisioner.AccessDetails, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if err := p.record("GrantAccess"); err != nil {
		return nil, err
	}

	if req.AccountName == "" {
		return nil, fmt.Errorf("%w: account name is required", provisioner.ErrInvalidArgument)
	}

	granted := make([]provisioner.GrantedBucket, 0, len(req.Buckets))
	bucketIDs := make([]string, 0, len(req.Buckets))
	for _, requested := range req.Buckets {
		existing, ok := p.buckets[requested.BucketID]
		if !ok {
			return nil, fmt.Errorf("%w: bucket %q", provisioner.ErrNotFound, requested.BucketID)
		}
		granted = append(granted, provisioner.GrantedBucket{
			BucketID: existing.id,
			Info:     info(existing),
		})
		bucketIDs = append(bucketIDs, existing.id)
	}

	id := accountID(req.AccountName)

	existing, ok := p.accounts[id]
	if !ok {
		existing = &account{
			id:          id,
			name:        req.AccountName,
			credentials: credentialsFor(req.AuthenticationType, req.AccountName),
			bucketIDs:   bucketIDs,
		}
		p.accounts[id] = existing
		Logc(ctx).WithField("accountID", id).Debug("Fake provisioner granted access.")
	} else {
		Logc(ctx).WithField("accountID", id).Debug("Fake provisioner reissued existing access.")
	}

	return &provisioner.AccessDetails{
		AccountID:   existing.id,
		Buckets:     granted,
		Credentials: existing.credentials,
	}, nil
}

// RevokeAccess removes an account. Revoking an absent account succeeds.
func (p *Provisioner) RevokeAccess(ctx context.Context, req *provisioner.RevokeAccessRequest) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if err := p.record("RevokeAccess"); err != nil {
		return err
	}

	delete(p.accounts, req.AccountID)
	Logc(ctx).WithField("accountID", req.AccountID).Debug("Fake provisioner revoked access.")

	return nil
}

// BucketCount reports how many buckets the fake currently holds, so a test can
// detect a reconcile that stranded one.
func (p *Provisioner) BucketCount() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return len(p.buckets)
}

// AccountCount reports how many accounts the fake currently holds.
func (p *Provisioner) AccountCount() int {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return len(p.accounts)
}

func (p *Provisioner) supports(protocol provisioner.Protocol) bool {
	for _, supported := range p.SupportedProtocols {
		if supported == protocol {
			return true
		}
	}
	return false
}

func details(b *bucket) *provisioner.BucketDetails {
	return &provisioner.BucketDetails{
		BucketID: b.id,
		Info:     info(b),
	}
}

// info reports connection details for every protocol the bucket was created
// with, and for none that it was not.
func info(b *bucket) provisioner.BucketInfo {
	var out provisioner.BucketInfo

	for _, protocol := range b.protocols {
		switch protocol {
		case provisioner.ProtocolS3:
			out.S3 = &provisioner.S3BucketInfo{
				BucketID:        b.id,
				Endpoint:        defaultEndpoint,
				Region:          defaultRegion,
				AddressingStyle: provisioner.S3AddressingStylePath,
			}
		case provisioner.ProtocolAzure:
			out.Azure = &provisioner.AzureBucketInfo{StorageAccount: "fake" + shortHash(b.name)}
		case provisioner.ProtocolGCS:
			out.GCS = &provisioner.GCSBucketInfo{
				ProjectID:  "fake-project",
				BucketName: b.name,
			}
		}
	}

	return out
}

// credentialsFor derives credentials from the account name so they are stable
// across restarts. A service account grant carries no credential material.
func credentialsFor(authType provisioner.AuthenticationType, accountName string) provisioner.Credentials {
	if authType == provisioner.AuthenticationTypeServiceAccount {
		return provisioner.Credentials{}
	}

	hash := shortHash(accountName)
	return provisioner.Credentials{
		S3: &provisioner.S3Credentials{
			AccessKeyID:     "AKIAFAKE" + strings.ToUpper(hash),
			AccessSecretKey: "fake-secret-" + hash,
		},
	}
}

func bucketID(name string) string {
	return "fake-bucket-" + shortHash(name)
}

func accountID(name string) string {
	return "fake-account-" + shortHash(name)
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

func sameParameters(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if other, ok := b[k]; !ok || other != v {
			return false
		}
	}
	return true
}
