// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package provisioner defines the contract between the COSI reconcilers and a Trident storage
// backend, using Trident-native types rather than COSI protobuf so a proto revision only
// touches the adapter and not backend code.
package provisioner

import "context"

// Provisioner is implemented by a Trident storage backend that serves object buckets. Its five
// methods correspond to the COSI provisioner RPCs; package adapter wires an implementation to
// the reconcilers in-process.
type Provisioner interface {
	// DriverInfo reports the provisioner name and the protocols it can serve.
	// The name must be an RFC-1035 label of at most 63 characters, because the
	// reconcilers use it to decide which objects belong to this driver.
	DriverInfo(ctx context.Context) (*DriverInfo, error)

	// CreateBucket provisions a bucket on the backend.
	//
	// Must be idempotent on Name: COSI retries with the same name, so an implementation that
	// appends its own randomness, or creates unconditionally, strands buckets on retry. If a
	// bucket with that name already exists and is compatible, return it with no error; if
	// incompatible, return ErrAlreadyExists.
	CreateBucket(ctx context.Context, req *CreateBucketRequest) (*BucketDetails, error)

	// GetBucket looks up a bucket that is expected to already exist on the
	// backend, for static provisioning. It returns ErrNotFound if it does not.
	GetBucket(ctx context.Context, req *GetBucketRequest) (*BucketDetails, error)

	// DeleteBucket removes a bucket from the backend. It must be idempotent:
	// deleting an already-absent bucket succeeds.
	DeleteBucket(ctx context.Context, req *DeleteBucketRequest) error

	// GrantAccess creates or reuses a backend account with access to the given buckets and
	// returns credentials for it.
	//
	// Must be idempotent on AccountName: re-issuing an existing grant returns the same account
	// and working credentials rather than rotating them underneath a running workload.
	//
	// req.Buckets may name more than one bucket; that alone is not unsupported. If the backend
	// cannot service the given set together (e.g. the buckets span backends), return
	// ErrMultiBucketAccessUnsupported rather than ErrInvalidArgument: per the COSI spec this is
	// the one condition that must map to OUT_OF_RANGE, which the reconciler treats as a
	// clearer, terminal failure.
	GrantAccess(ctx context.Context, req *GrantAccessRequest) (*AccessDetails, error)

	// RevokeAccess removes a previously granted account. It must be idempotent:
	// revoking an already-absent account succeeds.
	RevokeAccess(ctx context.Context, req *RevokeAccessRequest) error
}

// Protocol identifies an object storage access protocol.
type Protocol string

const (
	ProtocolS3    Protocol = "S3"
	ProtocolAzure Protocol = "Azure"
	ProtocolGCS   Protocol = "GCS"
)

// AccessMode is the level of access granted to a bucket.
type AccessMode string

const (
	AccessModeReadWrite AccessMode = "ReadWrite"
	AccessModeReadOnly  AccessMode = "ReadOnly"
	AccessModeWriteOnly AccessMode = "WriteOnly"
)

// AuthenticationType is the mechanism by which a grantee authenticates.
type AuthenticationType string

const (
	// AuthenticationTypeKey issues long-lived credentials, such as an S3 access
	// key pair, returned in the Credentials of a grant.
	AuthenticationTypeKey AuthenticationType = "Key"

	// AuthenticationTypeServiceAccount binds access to a Kubernetes service
	// account, so no credential material is returned.
	AuthenticationTypeServiceAccount AuthenticationType = "ServiceAccount"
)

// S3AddressingStyle selects between path-style and virtual-hosted-style S3 URLs.
type S3AddressingStyle string

const (
	S3AddressingStylePath    S3AddressingStyle = "Path"
	S3AddressingStyleVirtual S3AddressingStyle = "Virtual"
)

// DriverInfo identifies the provisioner to the reconcilers.
type DriverInfo struct {
	// Name is the driver name matched against Bucket.spec.driverName.
	Name string

	// SupportedProtocols must contain at least one protocol. A request for a
	// protocol not listed here is rejected before it reaches the backend.
	SupportedProtocols []Protocol
}

// CreateBucketRequest asks the backend to provision a bucket.
type CreateBucketRequest struct {
	// Name is COSI's suggested bucket name and its idempotency key. It is a DNS
	// subdomain of at most 253 characters.
	Name string

	// Protocols lists every protocol the bucket must support. If a protocol
	// cannot be supported, return ErrInvalidArgument.
	Protocols []Protocol

	// Parameters are opaque backend-specific options from the BucketClass.
	Parameters map[string]string
}

// GetBucketRequest asks the backend about an existing bucket.
type GetBucketRequest struct {
	BucketID   string
	Protocols  []Protocol
	Parameters map[string]string
}

// DeleteBucketRequest asks the backend to remove a bucket.
type DeleteBucketRequest struct {
	BucketID   string
	Parameters map[string]string
}

// BucketDetails describes a bucket on the backend. It is returned by both
// CreateBucket and GetBucket.
type BucketDetails struct {
	// BucketID is the backend's identifier for the bucket, used for every
	// subsequent call about it. At most 2048 characters of [a-zA-Z0-9.-].
	BucketID string

	// Info must carry a non-nil entry for each protocol the bucket supports,
	// and must not claim support for a protocol the backend cannot serve.
	Info BucketInfo
}

// BucketInfo carries non-credential connection details, one field per protocol.
// A nil field means the protocol is not supported for this bucket.
type BucketInfo struct {
	S3    *S3BucketInfo
	Azure *AzureBucketInfo
	GCS   *GCSBucketInfo
}

// S3BucketInfo describes how to reach a bucket over S3.
type S3BucketInfo struct {
	BucketID        string
	Endpoint        string
	Region          string
	AddressingStyle S3AddressingStyle
}

// AzureBucketInfo describes how to reach a container over Azure Blob Storage.
type AzureBucketInfo struct {
	StorageAccount string
}

// GCSBucketInfo describes how to reach a bucket over Google Cloud Storage.
type GCSBucketInfo struct {
	ProjectID  string
	BucketName string
}

// GrantAccessRequest asks the backend to provision access to one or more buckets.
type GrantAccessRequest struct {
	// AccountName is the requested backend account name and the idempotency key
	// for the grant.
	AccountName string

	Protocol           Protocol
	AuthenticationType AuthenticationType

	// ServiceAccountName is set only when AuthenticationType is
	// AuthenticationTypeServiceAccount.
	ServiceAccountName string

	// Buckets is the set of buckets the account is granted access to. COSI v1alpha2 permits
	// more than one per grant; see GrantAccess for what to return if the backend does not.
	Buckets []AccessedBucket

	Parameters map[string]string
}

// AccessedBucket names a bucket in a grant and the access level for it.
type AccessedBucket struct {
	BucketID   string
	AccessMode AccessMode
}

// RevokeAccessRequest asks the backend to remove a previously granted account.
type RevokeAccessRequest struct {
	AccountID          string
	Protocol           Protocol
	AuthenticationType AuthenticationType
	ServiceAccountName string

	// BucketIDs are the buckets the account was granted access to.
	BucketIDs []string

	Parameters map[string]string
}

// AccessDetails is the result of a successful grant.
type AccessDetails struct {
	// AccountID is the backend's identifier for the account, used later to
	// revoke it.
	AccountID string

	// Buckets describes each bucket the account can reach.
	Buckets []GrantedBucket

	// Credentials is empty for AuthenticationTypeServiceAccount, where access
	// is bound to the service account rather than to issued secrets.
	Credentials Credentials
}

// GrantedBucket pairs a bucket with its connection details within a grant.
type GrantedBucket struct {
	BucketID string
	Info     BucketInfo
}

// Credentials carries issued credential material, one field per protocol. These
// values reach a Secret in the user's namespace, so they must never be logged.
type Credentials struct {
	S3    *S3Credentials
	Azure *AzureCredentials
	GCS   *GCSCredentials
}

// S3Credentials is an S3 access key pair.
type S3Credentials struct {
	AccessKeyID     string
	AccessSecretKey string
}

// AzureCredentials is an Azure access token and its RFC-3339 expiry.
type AzureCredentials struct {
	AccessToken     string
	ExpiryTimestamp string
}

// GCSCredentials is a Google Cloud Storage service account credential.
type GCSCredentials struct {
	AccessID       string
	AccessSecret   string
	PrivateKeyName string
	ServiceAccount string
}
