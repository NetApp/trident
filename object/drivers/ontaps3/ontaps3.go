// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package ontaps3 implements a Trident object-store driver for NetApp ONTAP S3.
// It talks to one SVM through the ONTAP cluster management REST API, covering
// create/get/delete bucket and grant/revoke bucket access.
package ontaps3

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/google/uuid"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/object"
	"github.com/netapp/trident/object/factory"
	drivers "github.com/netapp/trident/storage_drivers"
	ontapapi "github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/utils/errors"
)

func init() {
	factory.RegisterStoreDriver(DriverName, NewObjectDriver)
}

const (
	DriverName           = "ontap-s3"
	defaultStoragePrefix = config.OrchestratorName + "-" // default prefix for all store resource names.
)

var (
	_ ontapapi.S3  = (*ontapapi.OntapAPIREST)(nil)
	_ object.Store = &ObjectDriver{}

	// storagePrefixRegex validates the storagePrefix field.
	// The prefix may contain alphanumerics, underscores, and dashes, and must start with
	// an alphanumeric or underscore. A trailing dash or underscore is permitted because
	//
	// makeInternalID treats them as the intended separator and does not add another one.
	// Consecutive dashes are rejected to prevent confusing names like "my--org-access".
	storagePrefixRegex = regexp.MustCompile(`^[0-9A-Za-z]([0-9A-Za-z]|-[0-9A-Za-z])*-?$`)
	inversePrefixRegex = regexp.MustCompile(`[^0-9A-Za-z-]`) // finds the invalid characters.

	// modesToActions maps each access mode to the S3 actions granted in the policy.
	modesToActions = map[object.AccessMode][]string{
		object.AccessModeRead: {
			// Object
			"GetObject",           // ONTAP 9.8+
			"GetObjectAcl",        // ONTAP 9.8+
			"GetObjectRetention",  // ONTAP 9.14.1+
			"GetObjectTagging",    // ONTAP 9.9.1+
			"ListBucket",          // ONTAP 9.8+; covers ListObjects/ListObjectsV2 per AWS IAM mapping
			"GetBucketVersioning", // ONTAP 9.11.1+
			// Multipart upload bookkeeping.
			"ListBucketMultipartUploads",
			"ListMultipartUploadParts",
		},
		object.AccessModeWrite: {
			// Object
			"PutObject",           // ONTAP 9.8+
			"DeleteObject",        // ONTAP 9.8+
			"PutObjectTagging",    // ONTAP 9.9.1+
			"DeleteObjectTagging", // ONTAP 9.9.1+
			"PutObjectRetention",  // ONTAP 9.14.1+
			"PutBucketVersioning", // ONTAP 9.11.1+
			// Multipart upload bookkeeping, needed for large writes.
			"ListBucketMultipartUploads",
			"ListMultipartUploadParts",
		},
		object.AccessModeReadWrite: {"*"},
	}
)

// newS3API is a package-level var so tests can swap in a fake.
//
// This delegates to NewRestClientFromOntapConfig. The same construction path every other
// ONTAP driver uses so SVM UUID resolution happens exactly once here, at construction,
// through EnsureSVMWithRest's synchronized (SetSVMUUID-backed) path.
//
// The client also picks up ONTAP version/capability discovery (SystemGetOntapVersion) for free.
// This reuses drivers.OntapStorageDriverConfig rather than a bespoke object-store config type, since SVM
// resolution isn't NAS/SAN-specific and EnsureSVMWithRest is already built around that type.
func newS3API(ctx context.Context, managementEndpoint, svmName, username, password string) (ontapapi.S3, error) {
	// For now, we only need the S3 management API, so we can keep the config simple.
	ontapConfig := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			StorageDriverName: DriverName,
		},
		ManagementLIF: managementEndpoint, // management endpoint is the management LIF (cluster or SVM).
		SVM:           svmName,            // svmName is the admin-specified name of the SVM.
		Username:      username,           // username is used for API authentication between the driver and ONTAP.
		Password:      password,           // password is used for API authentication between the driver and ONTAP.
	}

	ontapAPI, err := ontapapi.NewRestClientFromOntapConfig(ctx, ontapConfig)
	if err != nil {
		return nil, err
	}

	s3API, ok := ontapAPI.(ontapapi.S3)
	if !ok {
		return nil, fmt.Errorf("ONTAP REST client does not implement the S3 management API")
	}
	return s3API, nil
}

// newAPI is a package-level var so tests can swap in a fake.
var newAPI = newS3API

// Credentials are omitted from ObjectDriver JSON (json:"-") so they can be stored
// separately from the serialized store, e.g. in a Kubernetes Secret.
type Credentials struct {
	Username string `json:"username"` // Username is used for API authentication between the driver and ONTAP.
	Password string `json:"password"` // Password is used for API authentication between the driver and ONTAP.
}

// ObjectDriver manages S3 buckets and bucket access in an ONTAP SVM.
type ObjectDriver struct {
	ObjectStoreName    string            `json:"objectStoreName"`         // ObjectStoreName is the admin-specified name of this driver instance.
	ObjectStoreDriver  string            `json:"objectStoreDriver"`       // ObjectStoreDriver is always DriverName; persisted so a CR can be reconstructed without external routing info.
	ManagementEndpoint string            `json:"managementEndpoint"`      // ManagementEndpoint is the management LIF (cluster or SVM); this is deliberately named ManagementEndpoint to maintain consistency with other object drivers.
	S3Endpoint         string            `json:"s3Endpoint,omitempty"`    // S3Endpoint is the data plane endpoint. Discovered at Initialize and should NOT change for the lifecycle of the Driver. e.g. "https://s3.test-fqdn-server.trident.netapp.com"
	SVMName            string            `json:"svmName"`                 // SVMName is the admin-specified name of the SVM.
	SVMUUID            string            `json:"svmUUID,omitempty"`       // SVMUUID is the ONTAP cluster-wise unique identifier for this SVM. Discovered at Initialize and should NOT change for the lifecycle of the Driver.
	UUID               string            `json:"uuid"`                    // UUID is the assigned unique identifier for this driver instance. UUID and SVMUUID are unique should not be the same.
	StoragePrefix      *string           `json:"storagePrefix,omitempty"` // StoragePrefix is a custom admin-specified storagePrefix. Defaults to defaultStoragePrefix.
	StoredState        object.StoreState `json:"state"`                   // State is current StoredState of the ObjectDriver instance.
	ontap              ontapapi.S3       // ontap is the ONTAP S3 APIs. This interface is intentionally trimmed down to management APIs for S3.
	Credentials        `json:"-"`
}

// NewObjectDriver constructs an ObjectDriver from backend JSON.
// Each config document is applied in order (later keys win); secretConfig supplies
// credentials separately so they are never present in the configs documents.
func NewObjectDriver(_ context.Context, secretConfig []byte, configs ...[]byte) (object.Store, error) {
	d := &ObjectDriver{}
	if len(secretConfig) > 0 {
		if err := json.Unmarshal(secretConfig, &d.Credentials); err != nil {
			return nil, fmt.Errorf("failed to parse secret config: %w", err)
		}
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("at least one config is required")
	}

	for _, cfg := range configs {
		if len(cfg) == 0 {
			continue
		}
		if err := json.Unmarshal(cfg, d); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}
	}

	if d.ObjectStoreName == "" {
		return nil, fmt.Errorf("objectStoreName is required")
	}
	if d.ManagementEndpoint == "" {
		return nil, fmt.Errorf("managementEndpoint is required")
	}
	if d.SVMName == "" {
		return nil, fmt.Errorf("svmName is required")
	}

	d.ObjectStoreDriver = DriverName
	return d, nil
}

func (d *ObjectDriver) Name() string                 { return d.ObjectStoreName }
func (d *ObjectDriver) DriverName() string           { return DriverName }
func (d *ObjectDriver) StoreUUID() string            { return d.UUID }
func (d *ObjectDriver) State() object.StoreState     { return d.StoredState }
func (d *ObjectDriver) SetState(s object.StoreState) { d.StoredState = s }
func (d *ObjectDriver) GetUniqueKey() string         { return d.ObjectStoreName }

func (d *ObjectDriver) SmartCopy() interface{} {
	copied := *d
	return &copied
}

// SetCommonConfig applies values supplied outside driver JSON, e.g. the store name from a
// Kubernetes CR's metadata.name, or the UUID/State preserved across an update.
func (d *ObjectDriver) SetCommonConfig(config object.CommonConfig) {
	if config.StoreName != "" {
		d.ObjectStoreName = config.StoreName
	}
	if config.UUID != "" {
		d.UUID = config.UUID
	}
	if config.State != "" {
		d.StoredState = config.State
	}
}

// MatchParams checks if all provided params match this store. ONTAP S3 stores currently only
// support matching on the driver type; label-based selection is not yet implemented.
func (d *ObjectDriver) MatchParams(params map[string]string) bool {
	for k, v := range params {
		switch k {
		case "objectStoreType":
			if v != DriverName {
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

func (d *ObjectDriver) Initialize(ctx context.Context) error {
	if d.ontap != nil {
		return nil
	}

	// S3 has been supported since ONTAP 9.8.1+. Versions before 9.10.1+ require a minimum size.
	// Object store frontends in Trident do not support supplying a size.
	// For now, omit the version check.
	if pfx := d.storagePrefix(); !storagePrefixRegex.MatchString(pfx) {
		invalidChars := inversePrefixRegex.FindAllString(pfx, -1)
		return fmt.Errorf("storagePrefix %s is invalid; contains unsanctioned characters: %q", pfx, invalidChars)
	}

	// UUID is the ObjectDriver instance UUID, not the SVM UUID.
	// These are distinct and should not be interchangeable.
	if d.UUID == "" {
		d.UUID = uuid.NewString()
	}
	ontapAPI, err := newAPI(ctx, d.ManagementEndpoint, d.SVMName, d.Username, d.Password)
	if err != nil {
		return fmt.Errorf("build management client: %w", err)
	}

	svc, err := ontapAPI.ServiceGet(ctx, d.SVMName)
	if err != nil {
		return fmt.Errorf("failed to query for S3 service on SVM %s: %w", d.SVMName, err)
	}
	if !svc.Enabled {
		return fmt.Errorf("required S3 service on SVM %s is disabled", d.SVMName)
	}

	d.ontap = ontapAPI                   // ONTAP S3 API
	d.SVMUUID = svc.SVMUUID              // SVM UUID
	d.S3Endpoint = "https://" + svc.Name // S3 server endpoint, e.g. "https://" + "s3.trident.netapp.com"
	Logc(ctx).WithFields(LogFields{
		"svm":        d.SVMName,
		"svmUUID":    d.SVMUUID,
		"storeName":  d.ObjectStoreName,
		"storeUUID":  d.UUID,
		"s3Endpoint": d.S3Endpoint,
	}).Debug("Initialized ONTAP S3 object store.")
	return nil
}

// storagePrefix returns the configured prefix or the package default.
func (d *ObjectDriver) storagePrefix() string {
	if d.StoragePrefix != nil && *d.StoragePrefix != "" {
		return *d.StoragePrefix
	}
	return defaultStoragePrefix
}

func (d *ObjectDriver) Terminate(_ context.Context) error {
	d.ontap = nil
	return nil
}

func (d *ObjectDriver) GetBucketByName(ctx context.Context, name string) (*object.Bucket, error) {
	if d.ontap == nil {
		return nil, fmt.Errorf("ONTAP S3 API is not initialized")
	}

	// bucketID is the name of a bucket in the storage system.
	bucketID, err := makeInternalID(d.storagePrefix(), name)
	if err != nil {
		return nil, err
	}

	b, err := d.ontap.BucketGetByName(ctx, bucketID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.NotFoundError("bucket %s not found", name)
	}
	return &object.Bucket{
		Name:       name,
		InternalID: bucketID,
		StoreUUID:  d.UUID,
	}, nil
}

func (d *ObjectDriver) CreateBucket(ctx context.Context, bucket *object.Bucket) error {
	if d.ontap == nil {
		return fmt.Errorf("ONTAP S3 API is not initialized")
	}
	if bucket == nil {
		return fmt.Errorf("bucket is required")
	}
	if bucket.Name == "" {
		return fmt.Errorf("bucket name is required")
	}

	// name = supplied bucket name; bucketName = d.StoragePrefix + supplied bucket name.
	bucketID, err := makeInternalID(d.storagePrefix(), bucket.Name)
	if err != nil {
		return err
	}

	extantBucket, err := d.ontap.BucketGetByName(ctx, bucketID)
	switch {
	case err != nil:
		Logc(ctx).WithFields(LogFields{
			"bucketName": bucketID,
			"storeName":  d.ObjectStoreName,
		}).WithError(err).Tracef("Failed to get bucket %s.", bucketID)
		return err
	case extantBucket != nil:
		// Do nothing; the bucket exists; return what's there.
		Logc(ctx).WithFields(LogFields{
			"bucketName": bucketID,
			"storeName":  d.ObjectStoreName,
		}).Debugf("Bucket with name %s already exists.", bucketID)
	default:
		bucketCfg := ontapapi.BucketCreateConfig{Name: bucketID}
		extantBucket, err = d.ontap.BucketCreate(ctx, bucketCfg)
		if err != nil {
			return fmt.Errorf("failed to create bucket %s: %w", bucketID, err)
		}
		Logc(ctx).WithFields(LogFields{
			"bucketName": bucket.Name,
			"storeName":  d.ObjectStoreName,
		}).Debug("Created ONTAP S3 storage bucket.")
	}

	// Fill in what we've learned.
	// Do NOT overwrite the bucket.Name.
	bucket.InternalID = bucketID
	bucket.StoreUUID = d.UUID
	bucket.Info = object.BucketInfo{
		S3: &object.S3BucketInfo{
			Endpoint:        d.S3Endpoint,
			Region:          d.SVMUUID, // This is a hack - ONTAP S3 has no concept of a Region, but object store frontends require a region. Use the SVM UUID.
			AddressingStyle: "Path",
		},
	}

	Logc(ctx).WithFields(LogFields{
		"bucket":     bucket,
		"bucketName": bucketID,
		"storeName":  d.ObjectStoreName,
	}).Debug("Created object storage bucket.")
	return nil
}

func (d *ObjectDriver) DeleteBucket(ctx context.Context, name string) error {
	if d.ontap == nil {
		return fmt.Errorf("ONTAP S3 API is not initialized")
	}
	if name == "" {
		return fmt.Errorf("bucket name is required")
	}

	bucketID, err := makeInternalID(d.storagePrefix(), name)
	if err != nil {
		return err
	}

	bucket, err := d.ontap.BucketGetByName(ctx, bucketID)
	switch {
	case err != nil:
		Logc(ctx).WithFields(LogFields{
			"requestName": name,
			"bucketName":  bucketID,
			"storeName":   d.ObjectStoreName,
		}).WithError(err).Tracef("Failed to get bucket %s.", bucketID)
		return err
	case bucket == nil:
		Logc(ctx).WithFields(LogFields{
			"bucketName": name,
			"storeName":  d.ObjectStoreName,
		}).Debugf("Bucket with name %s not found; returning success.", bucketID)
		return nil
	default:
		Logc(ctx).WithFields(LogFields{
			"bucketName": bucketID,
			"bucketUUID": bucket.UUID,
			"storeName":  d.ObjectStoreName,
		}).Debugf("Deleting ONTAP S3 Bucket %s.", bucketID)
		return d.ontap.BucketDelete(ctx, bucket.UUID)
	}
}

// GrantBucketAccess creates an SVM-level policy, an S3 user, and a group that binds them.
// One policy statement is added per mode; each names the bucket and grants the corresponding actions.
// Credentials can only be read at user-creation time; callers must revoke and re-grant to rotate them.
func (d *ObjectDriver) GrantBucketAccess(ctx context.Context, modes []object.BucketAccessMode, access *object.BucketAccess) (*object.S3BucketAccessSecret, error) {
	if d.ontap == nil {
		return nil, fmt.Errorf("ONTAP S3 API is not initialized")
	}
	if access == nil || access.Name == "" {
		return nil, fmt.Errorf("bucket access name is required")
	}
	if len(modes) == 0 {
		return nil, fmt.Errorf("at least one bucket access mode is required")
	}

	accessID, err := makeInternalID(d.storagePrefix(), access.Name)
	if err != nil {
		return nil, err
	}

	// If the group already exists, the grant already completed but credentials cannot be
	// re-read from ONTAP. The caller must revoke first.
	extantGroup, err := d.ontap.GroupGet(ctx, accessID)
	if err != nil {
		return nil, fmt.Errorf("could not check for group %s: %w", accessID, err)
	}
	if extantGroup != nil {
		return nil, fmt.Errorf("group %s already exists; revoke bucket access %s before re-granting", accessID, access.Name)
	}

	// Replace the BucketName in each BucketAccessMode with the
	// internalID /  storage bucket name and organize them by access mode.
	modesToBuckets, err := mapBucketsByModes(d.storagePrefix(), modes)
	if err != nil {
		return nil, err
	}

	// Create the Policy and Statements.
	// Check for an existing policy first so a retry after a prior grant that failed
	// partway through (after PolicyCreate but before the group was created) does not
	// hit a conflict on PolicyCreate and get stuck forever.
	statements, err := makePolicyStatements(modesToBuckets, modesToActions)
	if err != nil {
		return nil, err
	}
	extantPolicy, err := d.ontap.PolicyGet(ctx, accessID)
	if err != nil {
		return nil, fmt.Errorf("failed to get policy %s: %w", accessID, err)
	}
	if extantPolicy == nil {
		if err = d.ontap.PolicyCreate(ctx, accessID, statements); err != nil {
			return nil, fmt.Errorf("failed to create policy %s: %w", accessID, err)
		}
		extantPolicy, err = d.ontap.PolicyGet(ctx, accessID)
		if err != nil {
			return nil, fmt.Errorf("failed to get policy %s: %w", accessID, err)
		}
		if extantPolicy == nil {
			return nil, fmt.Errorf("policy %s not found after creation", accessID)
		}
	}

	// Credentials cannot be re-read from ONTAP.
	// If we're here after a prior grant that failed at some point
	// after creating the user, we need to delete the user and create a new one.
	extantUser, err := d.ontap.UserGet(ctx, accessID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user %s: %w", accessID, err)
	}
	if extantUser != nil {
		// Revoke access by deleting the user if we're getting a repeat call for bucket access.
		if err := d.ontap.UserDelete(ctx, accessID); err != nil {
			return nil, fmt.Errorf("failed to delete user %s: %w", accessID, err)
		}
	}
	user, err := d.ontap.UserCreate(ctx, accessID)
	if err != nil {
		return nil, fmt.Errorf("failed to create user %s: %w", accessID, err)
	}

	// ONTAP requires at least one user at group-creation time.
	if err := d.ontap.GroupCreate(ctx, accessID, []string{user.Name}, []string{extantPolicy.Name}); err != nil {
		return nil, fmt.Errorf("failed to create group %s: %w", accessID, err)
	}
	group, err := d.ontap.GroupGet(ctx, accessID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group %s: %w", accessID, err)
	}
	if group == nil {
		return nil, fmt.Errorf("group %s not found after creation", accessID)
	}

	// Do NOT overwrite the access.Name;
	// Storage specific name goes into the accessID.
	access.InternalID = accessID
	access.StoreUUID = d.UUID
	Logc(ctx).WithFields(LogFields{
		"bucketAccess": access.Name,
		"accessModes":  modes,
		"userName":     user.Name,
		"groupName":    group.Name,
		"policyName":   accessID,
		"statements":   statements,
		"storeName":    d.ObjectStoreName,
	}).Debug("Granted ONTAP S3 bucket access.")
	return &object.S3BucketAccessSecret{
		AccessKeyID:     user.AccessKey,
		SecretAccessKey: user.SecretKey,
	}, nil
}

// RevokeBucketAccess deletes the group, user, and SVM-level policy created during grant.
// Deletion order is Group -> User -> Policy so that each object is unlinked before removal.
// Each delete is idempotent; 404 responses are treated as success.
func (d *ObjectDriver) RevokeBucketAccess(ctx context.Context, access *object.BucketAccess) error {
	if d.ontap == nil {
		return fmt.Errorf("ONTAP S3 API is not initialized")
	}
	if access == nil || access.Name == "" {
		return fmt.Errorf("bucket access name is required")
	}

	accessID, err := makeInternalID(d.storagePrefix(), access.Name)
	if err != nil {
		return err
	}

	if err := d.ontap.GroupDelete(ctx, accessID); err != nil {
		return fmt.Errorf("delete group %s: %w", accessID, err)
	}
	if err := d.ontap.UserDelete(ctx, accessID); err != nil {
		return fmt.Errorf("delete user %s: %w", accessID, err)
	}
	if err := d.ontap.PolicyDelete(ctx, accessID); err != nil {
		return fmt.Errorf("delete policy %s: %w", accessID, err)
	}

	Logc(ctx).WithFields(LogFields{
		"name":   accessID,
		"access": access.Name,
		"store":  d.ObjectStoreName,
	}).Debug("Revoked ONTAP S3 bucket access.")
	return nil
}

// validateName returns an error if the supplied bucket or bucket access name contains
// characters outside the S3-compatible set ([0-9a-z.-]) or exceeds 63 characters.
func validateName(name string) error {
	if len(name) > 63 {
		return fmt.Errorf("bucket name %s exceeds 63-character limit", name)
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '.') {
			return fmt.Errorf("bucket name %s contains invalid character %c; only [0-9a-z.-] are allowed", name, c)
		}
	}
	return nil
}

// makeInternalID returns the name used for the ONTAP S3 bucket, policy, user, and group.
// All ONTAP S3 resources share a common prefix (default of "trident-") so revoke can find
// them without additional state. Returns an error if the derived name exceeds the 64-char ONTAP name limit.
func makeInternalID(prefix, suffix string) (string, error) {
	if prefix == "" {
		prefix = defaultStoragePrefix
	}
	if suffix == "" {
		suffix = uuid.NewString()
	}
	if err := validateName(suffix); err != nil {
		return "", err
	}

	// If the prefix already ends with a separator, concatenate directly;
	// otherwise insert a dash so the caller doesn't have to include one.
	sep := "-"
	if last := prefix[len(prefix)-1]; last == '-' || last == '_' {
		sep = ""
	}
	name := prefix + sep + suffix
	if len(name) > 64 {
		maxLen := 64 - len(prefix) - len(sep)
		return "", fmt.Errorf("supplied name %s is too long; must be %d characters or fewer with prefix %s", suffix, maxLen, prefix)
	}
	return name, nil
}

func mapBucketsByModes(prefix string, modes []object.BucketAccessMode) (map[object.AccessMode][]string, error) {
	// Build a map of modes to an array of buckets.
	// This lets the object.AccessMode be the classifier.
	modesToBuckets := make(map[object.AccessMode][]string)
	for _, m := range modes {
		if m.AccessMode == "" {
			return nil, fmt.Errorf("bucket access mode %v has empty access mode", m)
		}
		if m.BucketName == "" {
			return nil, fmt.Errorf("bucket access mode %v has empty bucket name", m)
		}

		// Make the internal bucket name; this is the storage-specific bucket name.
		bucketName, err := makeInternalID(prefix, m.BucketName)
		if err != nil {
			return nil, err
		}

		if _, ok := modesToBuckets[m.AccessMode]; !ok {
			modesToBuckets[m.AccessMode] = make([]string, 0)
		}
		modesToBuckets[m.AccessMode] = append(modesToBuckets[m.AccessMode], bucketName)
	}

	if len(modesToBuckets) == 0 {
		return nil, errors.New("no valid bucket access modes specified")
	}
	return modesToBuckets, nil
}

// makePolicyStatements converts BucketAccessModes into SVM-level policy statements.
// Resources use the "bucket/*" wildcard for every object in the bucket.
func makePolicyStatements(modesToBuckets map[object.AccessMode][]string, modesToActions map[object.AccessMode][]string) ([]ontapapi.SVMPolicyStatement, error) {
	if len(modesToBuckets) == 0 {
		return nil, errors.New("no bucket access modes specified")
	}
	if len(modesToActions) == 0 {
		return nil, errors.New("no bucket access s3 actions specified")
	}

	statements := make([]ontapapi.SVMPolicyStatement, 0, len(modesToBuckets))
	for mode, buckets := range modesToBuckets {
		// Organize all buckets that map to this access mode into a flat array.
		// Example:
		//	resources = ["bucketA", "bucketA/*", "bucketB", "bucketB/*",...]
		resources := make([]string, 0)
		for _, bucket := range buckets {
			resources = append(resources, []string{
				bucket,        // bucketName
				bucket + "/*", // bucketName/*
			}...)
		}

		actions, ok := modesToActions[mode]
		if !ok {
			return nil, fmt.Errorf("bucket access mode %v has no supported s3 actions", mode)
		}

		statements = append(statements, ontapapi.SVMPolicyStatement{
			Effect:    "allow",
			Actions:   actions,   // ["*"], ["PutObject", "DeleteObject",...],...
			Resources: resources, // ["bucketA", "bucketA/*", "bucketB", "bucketB/*",...]
		})
	}

	if len(statements) == 0 {
		return nil, errors.New("no bucket access modes specified")
	}
	return statements, nil
}
