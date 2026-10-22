// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package storagegrid implements a Trident object-store driver for NetApp
// StorageGRID. It talks to one tenant through the Tenant Management API
// client in sgapi, covering create/get/delete bucket and grant/revoke access.
package storagegrid

import (
	"context"
	"encoding/base64"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/object"
	"github.com/netapp/trident/object/drivers/storagegrid/sgapi"
	"github.com/netapp/trident/object/factory"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
)

// init registers this driver with the object-store factory.
func init() {
	factory.RegisterStoreDriver(DriverName, NewObjectDriver)
}

const (
	// DriverName is the objectStoreDriver value that selects this backend.
	DriverName           = "storage-grid"
	defaultStoragePrefix = config.OrchestratorName + "-"

	requiredMajorAPIVersion = 4
	defaultDeletePoll       = 2 * time.Second
)

// tenantAPI is the StorageGRID Tenant Management API surface this driver.
type tenantAPI interface {
	Versions(context.Context) ([]int, error)
	Authorize(context.Context) (string, error)
	ReleaseToken(context.Context) error
	CreateBucket(context.Context, sgapi.ContainerCreate) (*sgapi.CreateBucketResult, error)
	GetBucket(context.Context, string, sgapi.BucketReads) (*sgapi.BucketState, error)
	DeleteBucket(context.Context, string) (sgapi.DeleteOutcome, error)
	GetRegion(context.Context, string) (string, error)
	CreateGroup(context.Context, sgapi.GroupCreate) (*sgapi.Group, error)
	GetGroupByShortName(context.Context, string) (*sgapi.Group, map[string]any, error)
	SetGroupS3Policy(context.Context, string, *sgapi.S3Policy) (*sgapi.Group, error)
	DeleteGroup(context.Context, string) error
	CreateUser(context.Context, sgapi.UserCreate) (*sgapi.User, error)
	GetUserByShortName(context.Context, string) (*sgapi.User, map[string]any, error)
	EnsureUserInGroup(context.Context, string, string) (*sgapi.User, error)
	DeleteUser(context.Context, string) error
	DeleteAllAccessKeys(context.Context, string) (int, error)
	CreateAccessKey(context.Context, string, *string) (*sgapi.AccessKey, error)
}

var (
	_ object.Store = &ObjectDriver{}
	_ tenantAPI    = (*sgapi.Client)(nil)

	newTenantClient = func(cfg sgapi.Config) (tenantAPI, error) {
		return sgapi.New(cfg)
	}
)

// Credentials are omitted from ObjectDriver JSON (json:"-") so they can be stored
// separately from the serialized store, e.g. in a Kubernetes Secret.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ObjectDriver provisions buckets on one StorageGRID tenant.
type ObjectDriver struct {
	ObjectStoreName    string `json:"objectStoreName"`
	ObjectStoreDriver  string `json:"objectStoreDriver"`
	ManagementEndpoint string `json:"managementEndpoint"`
	// AccountID is the 20-digit tenant account ID, not the tenant display name.
	AccountID            string `json:"accountId"`
	Credentials          `json:"-"`
	S3Endpoint           string            `json:"s3Endpoint"`
	Region               string            `json:"region,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
	UUID                 string            `json:"uuid"`
	StoragePrefix        string            `json:"storagePrefix,omitempty"`
	InsecureSkipVerify   bool              `json:"insecureSkipVerify,omitempty"`
	TrustedCACertificate string            `json:"trustedCACertificate,omitempty"`
	EmptyBucketOnDelete  bool              `json:"emptyBucketOnDelete,omitempty"`
	StoreState           object.StoreState `json:"state"`
	client               tenantAPI
}

// NewObjectDriver constructs an ObjectDriver from backend JSON.
// Input is the raw backend config documents, applied in order. Output is an object.Store.
// Credentials are stored but the Tenant Management API client is not opened until Initialize.
func NewObjectDriver(_ context.Context, secretConfig []byte, configs ...[]byte) (object.Store, error) {
	d := &ObjectDriver{}
	for _, config := range configs {
		if len(config) == 0 {
			continue
		}
		if err := json.Unmarshal(config, d); err != nil {
			return nil, err
		}
	}
	d.ObjectStoreDriver = DriverName
	if len(secretConfig) > 0 {
		if err := json.Unmarshal(secretConfig, &d.Credentials); err != nil {
			return nil, err
		}
	}
	if d.ObjectStoreName == "" {
		d.ObjectStoreName = DriverName
	}
	return d, nil
}

func (d *ObjectDriver) Name() string { return d.ObjectStoreName }

func (d *ObjectDriver) DriverName() string { return DriverName }

func (d *ObjectDriver) StoreUUID() string { return d.UUID }

func (d *ObjectDriver) SetCommonConfig(config object.CommonConfig) {
	if config.StoreName != "" {
		d.ObjectStoreName = config.StoreName
	}
	if config.UUID != "" {
		d.UUID = config.UUID
	}
	if config.State != "" {
		d.StoreState = config.State
	}
}

func (d *ObjectDriver) State() object.StoreState { return d.StoreState }

func (d *ObjectDriver) SetState(state object.StoreState) { d.StoreState = state }

func (d *ObjectDriver) SmartCopy() interface{} {
	copied := *d
	return &copied
}

func (d *ObjectDriver) GetUniqueKey() string { return d.ObjectStoreName }

func (d *ObjectDriver) MatchParams(params map[string]string) bool {
	for k, v := range params {
		switch k {
		case "objectStoreType":
			if v != DriverName {
				return false
			}
		case sa.Selector:
			request, err := sa.NewLabelRequest(v)
			if err != nil {
				return false
			}
			if !sa.NewLabelOffer(d.Labels).Matches(request) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (d *ObjectDriver) Secrets() interface{} {
	return map[string]string{
		"username": d.Username,
		"password": d.Password,
	}
}

// Initialize builds the Tenant Management API client and confirms the StorageGRID version.
// Return is nil on success, or an error if the client cannot be built, the version is incompatible,
// or tenant login fails.
func (d *ObjectDriver) Initialize(ctx context.Context) error {
	if d.client != nil {
		return nil
	}
	// Assign before any fallible call: persistence rejects a store with an empty UUID.
	if d.UUID == "" {
		d.UUID = uuid.NewString()
	}

	if err := d.openClient(ctx); err != nil {
		return err
	}
	// StoreState is left unchanged. Core sets online after a successful Initialize.

	Logc(ctx).WithFields(LogFields{
		"store":    d.ObjectStoreName,
		"endpoint": d.ManagementEndpoint,
		"account":  d.AccountID,
	}).Debug("Initialized StorageGRID object store.")
	return nil
}

// openClient builds the Tenant Management API client and confirms the StorageGRID version.
func (d *ObjectDriver) openClient(ctx context.Context) error {

	if d.InsecureSkipVerify && d.TrustedCACertificate != "" {
		return fmt.Errorf("insecureSkipVerify and trustedCACertificate cannot both be set")
	}
	certBytes, err := base64.StdEncoding.DecodeString(d.TrustedCACertificate)
	if err != nil {
		return fmt.Errorf("decode trustedCACertificate error: %w", err)
	}

	client, err := newTenantClient(sgapi.Config{
		Endpoint:  d.ManagementEndpoint,
		AccountID: d.AccountID,
		Username:  d.Username,
		Password:  d.Password,
		Insecure:  d.InsecureSkipVerify,
		CACert:    certBytes,
	})
	if err != nil {
		return fmt.Errorf("storage-grid client: %w", err)
	}

	// Version check
	versions, err := client.Versions(ctx)
	if err != nil {
		return fmt.Errorf("query StorageGRID API versions: %w", err)
	}
	if !supportsMajor(versions, requiredMajorAPIVersion) {
		return fmt.Errorf("StorageGRID API major version %d is required, grid advertised %v",
			requiredMajorAPIVersion, versions)
	}

	if d.S3Endpoint == "" {
		return errors.New("s3 endpoint required")
	}

	if d.Password != "" {
		// Prove the tenant credentials work.
		// Bad credentials fails at initialize instead of on the first bucket request.
		if _, err := client.Authorize(ctx); err != nil {
			return fmt.Errorf("authorize StorageGRID tenant: %w", err)
		}
	}

	d.client = client
	return nil
}

// Terminate drops the Tenant Management API session.
// An error is currently never returned.
// A failed token release is logged and ignored so shutdown still completes.
func (d *ObjectDriver) Terminate(ctx context.Context) error {
	if d.client == nil {
		return nil
	}
	// Give up the session there is not a live token for a backend that is going away.
	if err := d.client.ReleaseToken(ctx); err != nil {
		Logc(ctx).WithError(err).Debug("StorageGRID token release failed.")
	}
	d.client = nil
	return nil
}

// GetBucketByName checks that a bucket exists on this tenant.
// Input is the bucket name. Output is a Bucket with Name, InternalID, StoreUUID, and Info populated.
// NotFoundError if the tenant does not own it, or another error if
// the API call fails. Extra bucket properties beyond region are not fetched.
func (d *ObjectDriver) GetBucketByName(ctx context.Context, name string) (*object.Bucket, error) {
	if err := d.requireClient(); err != nil {
		return nil, err
	}
	// Ask only whether this tenant owns the bucket; the region read is the existence check
	// itself, and comes back on the same call - no properties beyond it are fetched.
	internalID := makeName(defaultStoragePrefix, name)
	state, err := d.client.GetBucket(ctx, internalID, sgapi.BucketReads{})
	if err != nil {
		if sgapi.IsNotFound(err) {
			return nil, errors.NotFoundError("bucket %s not found", internalID)
		}
		return nil, fmt.Errorf("get bucket %s: %w", internalID, err)
	}
	return &object.Bucket{
		Name:       name,
		InternalID: internalID,
		StoreUUID:  d.StoreUUID(),
		Info:       d.bucketInfo(state.Region),
	}, nil
}

// bucketInfo builds the S3 connection details COSI needs to reach a bucket in region.
func (d *ObjectDriver) bucketInfo(region string) object.BucketInfo {
	return object.BucketInfo{
		S3: &object.S3BucketInfo{
			Endpoint: d.S3Endpoint,
			Region:   region,
			// Bucket names may contain dots (see ValidateBucketName), which break TLS SNI
			// matching under virtual-hosted-style addressing - path-style avoids that.
			AddressingStyle: "Path",
		},
	}
}

// CreateBucket provisions a bucket on this tenant.
// Input is a Bucket. Output is nil if the bucket was created or already exists on this tenant.
// A conflict that does not resolve to this tenant is an AlreadyExistsError.
func (d *ObjectDriver) CreateBucket(ctx context.Context, bucket *object.Bucket) error {
	if err := d.requireClient(); err != nil {
		return err
	}
	if bucket == nil || bucket.Name == "" {
		return fmt.Errorf("bucket name is required")
	}

	bucket.InternalID = makeName(defaultStoragePrefix, bucket.Name)

	// Provision the bucket in the backend's configured region.
	spec := sgapi.ContainerCreate{Name: bucket.InternalID, Region: d.Region}
	result, err := d.client.CreateBucket(ctx, spec)
	switch {
	case err == nil:
		bucket.StoreUUID = d.StoreUUID()
		region := result.Region
		if region == "" {
			region, err = d.client.GetRegion(ctx, bucket.InternalID)
			if err != nil {
				return fmt.Errorf("get region %s: %w", bucket.InternalID, err)
			}
		}
		bucket.Info = d.bucketInfo(region)
		Logc(ctx).WithFields(LogFields{
			"name":  bucket.InternalID,
			"store": d.ObjectStoreName,
		}).Debug("Created StorageGRID bucket.")
		return nil
	case sgapi.IsAlertError(err):
		// An error alert means the bucket exists and needs a later reconcile. Info is
		// still populated on the caller's pointer despite the error return below: the
		// container's reachability (endpoint/region) is unaffected by whichever piece of
		// its configuration the alert flags, and a caller resuming on a cached partial
		// attempt (core.AddBucket does) benefits from not re-fetching it on the next call.
		bucket.StoreUUID = d.StoreUUID()
		bucket.Info = d.bucketInfo(result.Region)
		Logc(ctx).WithFields(LogFields{
			"name":  bucket.InternalID,
			"store": d.ObjectStoreName,
		}).Debug("Created StorageGRID bucket, but there was an error.")
		return fmt.Errorf("create bucket %s: %w", bucket.InternalID, err)
	case sgapi.IsConflict(err), sgapi.IsUnprocessable(err):
		// Bucket name may already exist. Check if it is in this tenant, which
		// makes create idempotent, or by another one, which is a real conflict.
		existing, getErr := d.client.GetBucket(ctx, bucket.InternalID, sgapi.BucketReads{})
		if getErr == nil {
			bucket.StoreUUID = d.StoreUUID()
			bucket.Info = d.bucketInfo(existing.Region)
			Logc(ctx).WithField("name", bucket.InternalID).Debug("Bucket already exists on this tenant.")
			return nil
		}
		Logc(ctx).WithFields(LogFields{
			"name":  bucket.InternalID,
			"store": d.ObjectStoreName,
		}).WithError(getErr).Debug("Could not confirm bucket ownership after create conflict.")
		// Keep the API conflict as the cause so callers can see why this is terminal.
		return errors.WrapWithAlreadyExistsError(err, "bucket %s already exists", bucket.InternalID)
	case sgapi.IsBucketNameInvalid(err):
		// Rejected locally before any request was sent; every backend would reject it too.
		return errors.WrapWithInvalidInputError(err, "invalid bucket name %s", bucket.InternalID)
	default:
		return fmt.Errorf("create bucket %s: %w", bucket.InternalID, err)
	}
}

// DeleteBucket removes a bucket from this tenant.
// Input is the bucket name. Output is nil if the bucket was deleted or was not
// on this tenant. Delete is idempotent: a missing bucket is success.
//
// Bucket names are unique across every tenant on the grid. A create that fails
// because the name is already taken may refer to another tenant's bucket, which
// is not present here, so the delete that follows that failure must succeed.
// The lookup is what makes that distinction. Not-found means nothing was
// created. A 409 on the lookup means the name is taken by another tenant, which
// is also nothing of ours to remove — and it must not be confused with the 409
// DELETE returns for a bucket this tenant owns that still has objects. A name
// the grid would reject was never created. Any other lookup failure is
// returned, because it is not safe to delete without knowing whether the
// bucket is on this tenant.
//
// A non-empty bucket that is on this tenant is an error unless EmptyBucketOnDelete
// is set, in which case objects are drained first via the delete-objects API.
func (d *ObjectDriver) DeleteBucket(ctx context.Context, name string) error {
	if err := d.requireClient(); err != nil {
		return err
	}

	internalID := makeName(defaultStoragePrefix, name)

	// See the comment above. A 409 here is another tenant's name, not "not empty".
	_, getErr := d.client.GetBucket(ctx, internalID, sgapi.BucketReads{})
	switch {
	case getErr == nil:
		// Present on this tenant. Fall through and delete it.
	case sgapi.IsNotFound(getErr), sgapi.IsConflict(getErr), sgapi.IsBucketNameInvalid(getErr):
		Logc(ctx).WithFields(LogFields{
			"name":  internalID,
			"store": d.ObjectStoreName,
		}).Debug("Bucket is not on this tenant, so there is nothing to delete.")
		return nil
	default:
		return fmt.Errorf("checking bucket %s before delete: %w", internalID, getErr)
	}

	if req, ok := d.client.(*sgapi.Client); ok {
		// Remove the bucket, first draining its objects and waiting for that to
		// finish when the backend allows emptying on delete.
		status, err := req.NewBucketDeleter(internalID, d.EmptyBucketOnDelete).Run(ctx, defaultDeletePoll, nil)
		if err != nil {
			if stderrors.Is(err, sgapi.ErrBucketNotEmpty) {
				return errors.WrapWithBucketNotEmptyError(err, "delete bucket %s", internalID)
			}
			return fmt.Errorf("delete bucket %s: %w", internalID, err)
		}
		Logc(ctx).WithFields(LogFields{
			"name":    internalID,
			"store":   d.ObjectStoreName,
			"message": status.Message,
		}).Debug("Deleted StorageGRID bucket.")
		return nil
	}

	// Plain single-shot delete, with no emptying step, for clients that cannot
	// run the drain-and-poll sequence above.
	outcome, err := d.client.DeleteBucket(ctx, internalID)
	if err != nil {
		return fmt.Errorf("delete bucket %s: %w", internalID, err)
	}
	switch outcome {
	case sgapi.BucketDeleted, sgapi.BucketAlreadyGone:
		return nil
	case sgapi.BucketNotEmpty:
		return errors.WrapWithBucketNotEmptyError(sgapi.ErrBucketNotEmpty, "delete bucket %s", internalID)
	default:
		return fmt.Errorf("delete bucket %s: unexpected outcome %s", internalID, outcome)
	}
}

// GrantBucketAccess creates a tenant group, user, and S3 access key for the
// requested buckets and modes.
// Input is BucketAccess: Name becomes the group/user short name; BucketAccessModes must list at least one bucket.
// Output is the generated S3 secret with StoreUUID set on access.
// Existing keys for the user are revoked before a new key is minted.
func (d *ObjectDriver) GrantBucketAccess(
	ctx context.Context, bucketAccessModes []object.BucketAccessMode, access *object.BucketAccess,
) (*object.S3BucketAccessSecret, error) {
	if err := d.requireClient(); err != nil {
		return nil, err
	}
	if access == nil {
		return nil, fmt.Errorf("bucket access is required")
	}
	name := makeName(defaultStoragePrefix, access.Name)
	if err := sgapi.ValidateAccountName(name); err != nil {
		return nil, fmt.Errorf("bucket access name: %w", err)
	}
	if len(bucketAccessModes) == 0 {
		return nil, fmt.Errorf("access to at least one bucket must be requested")
	}

	// Check that all the buckets exist before creating identities for a policy that can never apply.
	for _, mode := range bucketAccessModes {
		bucketInternalID := makeName(defaultStoragePrefix, mode.BucketName)
		if region, err := d.client.GetRegion(ctx, bucketInternalID); err != nil {
			if sgapi.IsNotFound(err) {
				return nil, errors.NotFoundError("bucket %s not found", bucketInternalID)
			}
			return nil, fmt.Errorf("reading region of bucket %s: %w", bucketInternalID, err)
		} else {
			d.Region = region
		}
	}

	// The grid bucket name is the prefixed internal ID, so the policy must name that
	// resource rather than the caller-facing bucket name.
	scopedModes := make([]object.BucketAccessMode, len(bucketAccessModes))
	for i, mode := range bucketAccessModes {
		scopedModes[i] = mode
		scopedModes[i].BucketName = makeName(defaultStoragePrefix, mode.BucketName)
	}
	policy, err := BuildGroupPolicy(scopedModes)
	if err != nil {
		return nil, err
	}

	group, err := d.ensureGroup(ctx, name, policy)
	if err != nil {
		return nil, err
	}
	user, err := d.ensureUser(ctx, name, group.ID)
	if err != nil {
		return nil, err
	}

	// Clear credentials a previous attempt left behind, so this ends with exactly one usable key and no orphans.
	if _, err := d.client.DeleteAllAccessKeys(ctx, user.ID); err != nil {
		return nil, fmt.Errorf("removing existing access keys of user %q: %w", name, err)
	}
	// Mint the key the consumer will use. It never expires,
	// since nothing in COSI rotates a credential before it lapses.
	key, err := d.client.CreateAccessKey(ctx, user.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("minting an access key for user %q: %w", name, err)
	}
	secret := &object.S3BucketAccessSecret{
		AccessKeyID:     key.AccessKey,
		SecretAccessKey: key.SecretAccessKey,
	}

	access.InternalID = name
	access.StoreUUID = d.StoreUUID()
	Logc(ctx).WithFields(LogFields{
		"access": access.Name,
		"store":  d.ObjectStoreName,
	}).Debug("Granted StorageGRID bucket access.")
	return secret, nil
}

// makeName returns the name used for the StorageGrid resources: bucket, user, and group.
func makeName(prefix, suffix string) string {
	if suffix == "" {
		suffix = uuid.NewString()
	}

	// If the prefix already ends with a separator, concatenate directly;
	// otherwise insert a dash so the caller doesn't have to include one.
	sep := "-"
	if last := prefix[len(prefix)-1]; last == '-' {
		sep = ""
	}
	name := prefix + sep + suffix
	return name
}

// RevokeBucketAccess deletes the S3 keys, user, and group created for bucket access.
// Input is BucketAccess whose Name matches the user short name.
// Output is nil if user was removed or already gone.
func (d *ObjectDriver) RevokeBucketAccess(ctx context.Context, access *object.BucketAccess) error {
	if err := d.requireClient(); err != nil {
		return err
	}
	if access == nil || access.Name == "" {
		return fmt.Errorf("bucket access is required")
	}

	name := makeName(defaultStoragePrefix, access.Name)
	// Find the user this access created; it carries the ID that the deletes need.
	user, _, err := d.client.GetUserByShortName(ctx, name)
	switch {
	case err == nil:
		// Invalidate the S3 credentials first, so access stops even if removing the user itself fails.
		if _, err := d.client.DeleteAllAccessKeys(ctx, user.ID); err != nil {
			return fmt.Errorf("deleting access keys of user %q: %w", name, err)
		}

		// Retire the user itself.
		if err := d.client.DeleteUser(ctx, user.ID); err != nil {
			return fmt.Errorf("deleting user %s: %w", sgapi.UserUniqueName(name), err)
		}
	case sgapi.IsNotFound(err):
		Logc(ctx).WithField("user", sgapi.UserUniqueName(name)).Debug("User already gone.")
	default:
		return fmt.Errorf("reading user %s: %w", sgapi.UserUniqueName(name), err)
	}

	// Look up the group that holds the S3 policy.
	group, _, err := d.client.GetGroupByShortName(ctx, name)
	switch {
	case err == nil:
		// Drop the policy with the group, leaving nothing behind.
		if err := d.client.DeleteGroup(ctx, group.ID); err != nil {
			return fmt.Errorf("deleting group %s: %w", sgapi.GroupUniqueName(name), err)
		}
	case sgapi.IsNotFound(err):
		Logc(ctx).WithField("group", sgapi.GroupUniqueName(name)).Debug("Group already gone.")
	default:
		return fmt.Errorf("reading group %s: %w", sgapi.GroupUniqueName(name), err)
	}

	Logc(ctx).WithFields(LogFields{
		"access": access.Name,
		"store":  d.ObjectStoreName,
	}).Debug("Revoked StorageGRID bucket access.")
	return nil
}

// ensureGroup creates the tenant group for accountName, or updates its S3 policy if the group already exists.
// Input is the account short name and the S3 policy to attach.
// Output is the group (including its ID) or an error.
func (d *ObjectDriver) ensureGroup(
	ctx context.Context, accountName string, policy *sgapi.S3Policy,
) (*sgapi.Group, error) {
	// Try create first and treat a conflict as already has access.
	group, err := d.client.CreateGroup(ctx, sgapi.GroupCreate{
		UniqueName:  sgapi.GroupUniqueName(accountName),
		DisplayName: sgapi.DerivedIdentityName(accountName),
		Policies:    &sgapi.GroupPolicies{S3: policy},
	})
	if err == nil {
		return group, nil
	}
	if !mayAlreadyExist(err) {
		return nil, fmt.Errorf("creating group %s: %w", sgapi.GroupUniqueName(accountName), err)
	}

	// Confirm the conflict is for this group before treating it as existing;
	// otherwise the original create error is returned.
	if _, _, getErr := d.client.GetGroupByShortName(ctx, accountName); getErr != nil {
		return nil, fmt.Errorf("creating group %s: %w", sgapi.GroupUniqueName(accountName), err)
	}

	// Bring the existing group's S3 policy up to what access is being requested.
	group, err = d.client.SetGroupS3Policy(ctx, accountName, policy)
	if err != nil {
		return nil, fmt.Errorf("updating policy of group %s: %w", sgapi.GroupUniqueName(accountName), err)
	}
	return group, nil
}

// ensureUser creates the tenant user for accountName in groupID, or adds the existing user to that group.
// Input is the account short name and the group's UUID.
// Output is the user (including its ID) or an error.
func (d *ObjectDriver) ensureUser(
	ctx context.Context, accountName, groupID string,
) (*sgapi.User, error) {
	if groupID == "" {
		return nil, fmt.Errorf("group for %q reported no id", accountName)
	}
	// Create the user already joined to the group, it inherits the policy without a second membership call.
	user, err := d.client.CreateUser(ctx, sgapi.UserCreate{
		UniqueName: sgapi.UserUniqueName(accountName),
		FullName:   sgapi.DerivedIdentityName(accountName),
		MemberOf:   []string{groupID},
	})
	if err == nil {
		return user, nil
	}
	if !mayAlreadyExist(err) {
		return nil, fmt.Errorf("creating user %s: %w", sgapi.UserUniqueName(accountName), err)
	}

	// Confirm the conflict is for this user before treating it as existing;
	// otherwise the original create error is returned.
	if _, _, getErr := d.client.GetUserByShortName(ctx, accountName); getErr != nil {
		return nil, fmt.Errorf("creating user %s: %w", sgapi.UserUniqueName(accountName), err)
	}

	// Make sure the pre-existing user belongs to this group.
	user, err = d.client.EnsureUserInGroup(ctx, accountName, groupID)
	if err != nil {
		return nil, fmt.Errorf("updating membership of user %s: %w", sgapi.UserUniqueName(accountName), err)
	}
	if user.ID == "" {
		return nil, fmt.Errorf("user %q reported no id", accountName)
	}
	return user, nil
}

// requireClient returns an error if Initialize has not yet attached a client.
func (d *ObjectDriver) requireClient() error {
	if d.client == nil {
		return fmt.Errorf("storage-grid driver is not initialized")
	}
	return nil
}

// mayAlreadyExist reports whether err is a 409 Conflict or 422 Unprocessable Entity,
// which StorageGRID uses when a group or user already exists.
func mayAlreadyExist(err error) bool {
	return sgapi.IsConflict(err) || sgapi.IsUnprocessable(err)
}

// supportsMajor reports whether the API majors the grid advertised include the
// one this driver requires.
func supportsMajor(versions []int, major int) bool {
	for _, v := range versions {
		if v == major {
			return true
		}
	}
	return false
}
