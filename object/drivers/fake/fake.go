// Copyright 2026 NetApp, Inc. All Rights Reserved.

package fake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"sync"

	"github.com/google/uuid"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/object"
	"github.com/netapp/trident/object/factory"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
)

func init() {
	factory.RegisterStoreDriver(driverName, NewObjectDriver)
}

const (
	driverName   = "fake"
	fakeEndpoint = "https://localhost"
	fakeRegion   = "fake-region-1"
)

type Credentials struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// ObjectDriver is an in-memory fake object store driver for testing.
type ObjectDriver struct {
	ObjectStoreName   string `json:"objectStoreName"`
	ObjectStoreDriver string `json:"objectStoreDriver"`
	Endpoint          string `json:"endpoint"`
	Credentials       `json:"-"`
	UUID              string            `json:"uuid"`
	Labels            map[string]string `json:"labels"`
	StoreState        object.StoreState `json:"state"`
	accesses          map[string]object.BucketAccess
	mtx               *sync.Mutex
	buckets           map[string]object.Bucket
	initialized       bool
	calls             map[string]int
	errs              map[string]error
	errAfterCreate    error
}

// NewObjectDriver returns an uninitialized fake object store driver.
func NewObjectDriver(_ context.Context, secretConfig []byte, configs ...[]byte) (object.Store, error) {
	d := &ObjectDriver{
		initialized: false,
		accesses:    make(map[string]object.BucketAccess),
		buckets:     make(map[string]object.Bucket),
		mtx:         &sync.Mutex{},
		calls:       make(map[string]int),
		errs:        make(map[string]error),
	}
	for _, config := range configs {
		if len(config) == 0 {
			continue
		}
		if err := json.Unmarshal(config, d); err != nil {
			return nil, err
		}
	}
	d.ObjectStoreDriver = driverName
	if len(secretConfig) > 0 {
		if err := json.Unmarshal(secretConfig, &d.Credentials); err != nil {
			return nil, err
		}
	}
	// Name is optional here. REST puts objectStoreName in the request JSON; CR create
	// sets it from metadata.name via SetCommonConfig after construction.
	if d.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	return d, nil
}

// record notes a call and returns any injected error. Caller must hold d.mtx.
func (d *ObjectDriver) record(method string) error {
	d.calls[method]++
	return d.errs[method]
}

func (d *ObjectDriver) Name() string {
	return d.ObjectStoreName
}

func (d *ObjectDriver) DriverName() string {
	return driverName
}

func (d *ObjectDriver) SetCommonConfig(config object.CommonConfig) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
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

func (d *ObjectDriver) StoreUUID() string {
	return d.UUID
}

func (d *ObjectDriver) State() object.StoreState {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.StoreState
}

func (d *ObjectDriver) SetState(state object.StoreState) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.StoreState = state
}

func (d *ObjectDriver) SmartCopy() interface{} {
	copied := *d
	return &copied
}

func (d *ObjectDriver) GetUniqueKey() string {
	return d.ObjectStoreName
}

func (d *ObjectDriver) Accesses() map[string]object.BucketAccess {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return maps.Clone(d.accesses)
}

func (d *ObjectDriver) Buckets() map[string]object.Bucket {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return maps.Clone(d.buckets)
}

func (d *ObjectDriver) Initialize(_ context.Context) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if d.UUID == "" {
		d.UUID = uuid.NewString()
	}
	if d.Endpoint == "" || d.Credentials.User == "" || d.Credentials.Password == "" {
		d.StoreState = object.StoreStateOffline
		return fmt.Errorf("endpoint, user, and password are required")
	}
	d.StoreState = object.StoreStateOnline
	d.initialized = true
	return nil
}

func (d *ObjectDriver) Terminate(_ context.Context) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.initialized = false
	return nil
}

func (d *ObjectDriver) MatchParams(params map[string]string) bool {
	for k, v := range params {
		switch k {
		case "objectStoreType":
			if v != driverName {
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
			// All parameters must match. A driver should not claim support for unknown params.
			return false
		}
	}
	return true
}

func (d *ObjectDriver) GetBucketByName(_ context.Context, name string) (*object.Bucket, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if err := d.record("GetBucketByName"); err != nil {
		return nil, err
	}
	bucket, ok := d.buckets[name]
	if !ok {
		return nil, errors.NotFoundError("bucket %s not found", name)
	}
	copied := bucket
	return &copied, nil
}

// CreateBucket is idempotent on bucket.Name: a repeat call returns the existing bucket's Info.
func (d *ObjectDriver) CreateBucket(ctx context.Context, bucket *object.Bucket) (*object.Bucket, error) {
	if bucket == nil || bucket.Name == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	d.mtx.Lock()
	defer d.mtx.Unlock()
	if err := d.record("CreateBucket"); err != nil {
		return nil, err
	}

	if existing, ok := d.buckets[bucket.Name]; ok {
		Logc(ctx).WithFields(LogFields{
			"name":  bucket.Name,
			"store": d.ObjectStoreName,
		}).Debug("Bucket already exists; returning existing bucket.")
		bucket.Info = existing.Info
		return bucket, nil
	}

	// Populate on the caller's pointer: core.AddBucket returns this same *object.Bucket back.
	bucket.Info = object.BucketInfo{
		S3: &object.S3BucketInfo{
			Endpoint:        d.Endpoint,
			Region:          fakeRegion,
			AddressingStyle: "Path", // matches provisioner.S3AddressingStylePath's string value
		},
	}

	stored := object.Bucket{
		Name:      bucket.Name,
		StoreName: d.ObjectStoreName,
		Info:      bucket.Info,
	}
	d.buckets[bucket.Name] = stored

	Logc(ctx).WithFields(LogFields{
		"name":  bucket.Name,
		"store": d.ObjectStoreName,
	}).Debug("Created fake bucket.")

	if d.errAfterCreate != nil {
		return bucket, d.errAfterCreate
	}
	return bucket, nil
}

func (d *ObjectDriver) DeleteBucket(ctx context.Context, name string) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if err := d.record("DeleteBucket"); err != nil {
		return err
	}
	delete(d.buckets, name)

	Logc(ctx).WithFields(LogFields{
		"name":  name,
		"store": d.ObjectStoreName,
	}).Debug("Deleted fake bucket.")

	return nil
}

func (d *ObjectDriver) GrantBucketAccess(
	ctx context.Context, modes []object.BucketAccessMode, access *object.BucketAccess,
) (*object.S3BucketAccessSecret, error) {
	if access == nil {
		return nil, fmt.Errorf("bucket access is required")
	}
	accessName := access.Name
	if accessName == "" {
		accessName = uuid.NewString()
		access.Name = accessName
	}

	d.mtx.Lock()
	defer d.mtx.Unlock()
	for _, mode := range modes {
		if _, ok := d.buckets[mode.BucketName]; !ok {
			return nil, errors.NotFoundError("bucket %s not found", mode.BucketName)
		}
	}

	stored := *access
	stored.StoreName = d.ObjectStoreName
	d.accesses[accessName] = stored

	Logc(ctx).WithFields(LogFields{
		"access": accessName,
		"store":  d.ObjectStoreName,
	}).Debug("Granted fake bucket access.")

	// Deterministic on accessName so a repeat grant returns the same credentials.
	return &object.S3BucketAccessSecret{
		AccessKeyID:     "AKIAFAKE" + shortHash(accessName),
		SecretAccessKey: "fake-secret-" + shortHash(accessName),
	}, nil
}

func (d *ObjectDriver) RevokeBucketAccess(ctx context.Context, access *object.BucketAccess) error {
	if access == nil {
		return fmt.Errorf("bucket access is required")
	}

	d.mtx.Lock()
	defer d.mtx.Unlock()
	delete(d.accesses, access.Name)

	Logc(ctx).WithFields(LogFields{
		"access": access.Name,
		"store":  d.ObjectStoreName,
	}).Debug("Revoked fake bucket access.")

	return nil
}

// Secrets returns credentials omitted from ObjectDriver JSON (json:"-") so they
// can be stored separately from the serialized store, e.g. in a Kubernetes Secret.
func (d *ObjectDriver) Secrets() interface{} {
	return map[string]string{
		"user":     d.User,
		"password": d.Password,
	}
}

func (d *ObjectDriver) Initialized() bool {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.initialized
}

// Calls reports how many times each method has been invoked.
func (d *ObjectDriver) Calls() map[string]int {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return maps.Clone(d.calls)
}

// SetError injects a failure for the named method (e.g. "CreateBucket"). Pass nil to clear it.
func (d *ObjectDriver) SetError(method string, err error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.errs[method] = err
}

// SetErrorAfterCreate injects a failure that CreateBucket returns after actually creating and
// storing the bucket, simulating a driver that reports a partial-success error for an
// otherwise-successful create. Pass a nil err to clear it.
func (d *ObjectDriver) SetErrorAfterCreate(err error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.errAfterCreate = err
}

// shortHash derives a short, deterministic string from s for fake credential material.
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}
