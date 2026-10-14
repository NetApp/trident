// Copyright 2026 NetApp, Inc. All Rights Reserved.

package sgapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const containersPath = "/org/containers"

// CreateBucket provisions a bucket. POST /org/containers accepts only a name, a
// region, and the Object Lock setting; everything else is a separate PUT against
// a sub-resource once the bucket exists.
//
// A 201 is not unconditionally a success. The response carries alerts, and an
// error-severity alert means the bucket was created while part of its
// configuration failed, so the result is returned alongside an AlertError to let
// the caller reconcile.
func (c *Client) CreateBucket(ctx context.Context, spec ContainerCreate) (*CreateBucketResult, error) {
	if err := ValidateBucketName(spec.Name); err != nil {
		return nil, err
	}
	if spec.Region != "" {
		if err := ValidateRegionName(spec.Region); err != nil {
			return nil, err
		}
	}

	resp, err := c.doRetrying(ctx, request{
		method: http.MethodPost,
		path:   containersPath,
		body:   spec,
	})
	if err != nil {
		return nil, err
	}

	result := &CreateBucketResult{Name: spec.Name, Region: spec.Region}
	// Report the region as the grid resolved it rather than as the request
	// phrased it, so a bucket created without an explicit region reports the
	// grid default instead of an empty string.
	var created ContainerCreate
	if decodeData(resp, &created) == nil && created.Region != "" {
		result.Region = created.Region
		result.RegionResolved = true
	}
	result.Alerts = responseAlerts(resp)

	if bad := errorAlerts(result.Alerts); len(bad) > 0 {
		return result, &AlertError{Op: fmt.Sprintf("creating bucket %q", spec.Name), Alerts: bad}
	}
	return result, nil
}

// DeleteOutcome is how a delete attempt resolved.
type DeleteOutcome int

const (
	// BucketDeleted means the grid removed the bucket.
	BucketDeleted DeleteOutcome = iota
	// BucketAlreadyGone means the bucket did not exist, which the COSI contract
	// requires be treated as success.
	BucketAlreadyGone
	// BucketNotEmpty means the bucket still holds objects.
	BucketNotEmpty
)

func (o DeleteOutcome) String() string {
	switch o {
	case BucketDeleted:
		return "deleted"
	case BucketAlreadyGone:
		return "already gone"
	case BucketNotEmpty:
		return "not empty"
	default:
		return "unknown"
	}
}

// DeleteBucket removes an empty bucket. The three expected non-success statuses
// are mapped rather than returned: 404 is success because delete must be
// idempotent, and 409 is reported as BucketNotEmpty so the caller can decide
// whether emptying is permitted. A 403 is returned as an error, since it means
// the credentials lack a required permission.
func (c *Client) DeleteBucket(ctx context.Context, name string) (DeleteOutcome, error) {
	if err := ValidateBucketName(name); err != nil {
		return BucketDeleted, err
	}

	_, err := c.doRetrying(ctx, request{
		method: http.MethodDelete,
		path:   escapePath("org", "containers", name),
	})
	switch {
	case err == nil:
		return BucketDeleted, nil
	case IsNotFound(err):
		return BucketAlreadyGone, nil
	case IsConflict(err):
		return BucketNotEmpty, nil
	default:
		return BucketDeleted, err
	}
}

// GetBucket reads a bucket's state, performing only the optional reads that
// reads selects.
//
// The region read comes first because it doubles as the existence and ownership
// check: it cannot succeed for a bucket this tenant does not have, and since
// bucket names are globally unique, a successful read is itself proof of
// ownership. A caller that validates nothing beyond the region stays at one call.
func (c *Client) GetBucket(ctx context.Context, name string, reads BucketReads) (*BucketState, error) {
	if err := ValidateBucketName(name); err != nil {
		return nil, err
	}

	region, err := c.GetRegion(ctx, name)
	if err != nil {
		return nil, err
	}
	state := &BucketState{Name: name, Region: region}

	if reads.Versioning {
		versioning, err := c.GetVersioning(ctx, name)
		if err != nil {
			return nil, err
		}
		state.Versioning = versioning
	}
	if reads.QuotaObjectBytes {
		quota, err := c.GetQuotaObjectBytes(ctx, name)
		if err != nil {
			return nil, err
		}
		state.QuotaObjectBytes = quota
	}
	if reads.QuotaObjectCount {
		quota, err := c.GetQuotaObjectCount(ctx, name)
		if err != nil {
			return nil, err
		}
		state.QuotaObjectCount = quota
	}
	if reads.ObjectLock {
		lock, err := c.GetObjectLock(ctx, name)
		if err != nil {
			return nil, err
		}
		state.ObjectLock = lock
	}
	if reads.Consistency {
		consistency, err := c.GetConsistency(ctx, name)
		if err != nil {
			return nil, err
		}
		state.Consistency = consistency
	}
	return state, nil
}

// GetRegion reads a bucket's region. A failure means the bucket does not exist in
// this tenant, which is what makes this the existence check.
func (c *Client) GetRegion(ctx context.Context, name string) (string, error) {
	resp, err := c.doRetrying(ctx, request{
		method: http.MethodGet,
		path:   escapePath("org", "containers", name, "region"),
	})
	if err != nil {
		return "", err
	}

	var wrapped struct {
		Region string `json:"region"`
	}
	if decodeData(resp, &wrapped) == nil && wrapped.Region != "" {
		return wrapped.Region, nil
	}
	// Tolerate the region arriving as a bare string rather than an object.
	var bare string
	if decodeData(resp, &bare) == nil {
		return bare, nil
	}
	return "", nil
}

// GetVersioning reads a bucket's versioning state. This sub-resource behaves the
// same on API 4.2 and 4.3, unlike the bucket list, where versioning is absent
// from the older release.
func (c *Client) GetVersioning(ctx context.Context, name string) (*Versioning, error) {
	var versioning Versioning
	if err := c.getSubResource(ctx, name, "versioning", &versioning); err != nil {
		return nil, err
	}
	return &versioning, nil
}

// GetQuotaObjectBytes reads a bucket's capacity quota. A nil result means no
// quota is set.
func (c *Client) GetQuotaObjectBytes(ctx context.Context, name string) (*int64, error) {
	var quota QuotaObjectBytes
	if err := c.getSubResource(ctx, name, "quota-object-bytes", &quota); err != nil {
		return nil, err
	}
	return quota.QuotaObjectBytes, nil
}

// GetQuotaObjectCount reads a bucket's object count quota. A nil result means no
// quota is set.
func (c *Client) GetQuotaObjectCount(ctx context.Context, name string) (*int64, error) {
	var quota QuotaObjectCount
	if err := c.getSubResource(ctx, name, "quota-object-count", &quota); err != nil {
		return nil, err
	}
	return quota.QuotaObjectCount, nil
}

// GetObjectLock reads a bucket's S3 Object Lock configuration.
func (c *Client) GetObjectLock(ctx context.Context, name string) (*ObjectLock, error) {
	var lock ObjectLock
	if err := c.getSubResource(ctx, name, "object-lock", &lock); err != nil {
		return nil, err
	}
	return &lock, nil
}

// GetConsistency reads a bucket's consistency level.
func (c *Client) GetConsistency(ctx context.Context, name string) (string, error) {
	var consistency Consistency
	if err := c.getSubResource(ctx, name, "consistency", &consistency); err != nil {
		return "", err
	}
	return consistency.Consistency, nil
}

// SetVersioning enables or suspends versioning on an existing bucket.
func (c *Client) SetVersioning(ctx context.Context, name string, enabled bool) error {
	return c.putSubResource(ctx, name, "versioning", Versioning{VersioningEnabled: enabled})
}

// SetConsistency sets a bucket's consistency level.
func (c *Client) SetConsistency(ctx context.Context, name, consistency string) error {
	return c.putSubResource(ctx, name, "consistency", Consistency{Consistency: consistency})
}

// SetQuotaObjectBytes sets a bucket's capacity quota. A nil quota clears it.
func (c *Client) SetQuotaObjectBytes(ctx context.Context, name string, quota *int64) error {
	return c.putSubResource(ctx, name, "quota-object-bytes", QuotaObjectBytes{QuotaObjectBytes: quota})
}

// SetQuotaObjectCount sets a bucket's object count quota. A nil quota clears it.
func (c *Client) SetQuotaObjectCount(ctx context.Context, name string, quota *int64) error {
	return c.putSubResource(ctx, name, "quota-object-count", QuotaObjectCount{QuotaObjectCount: quota})
}

func (c *Client) getSubResource(ctx context.Context, name, resource string, out any) error {
	resp, err := c.doRetrying(ctx, request{
		method: http.MethodGet,
		path:   escapePath("org", "containers", name, resource),
	})
	if err != nil {
		return err
	}
	if err := decodeData(resp, out); err != nil {
		return fmt.Errorf("reading %s of bucket %q: %w", resource, name, err)
	}
	return nil
}

func (c *Client) putSubResource(ctx context.Context, name, resource string, body any) error {
	if err := ValidateBucketName(name); err != nil {
		return err
	}
	_, err := c.doRetrying(ctx, request{
		method: http.MethodPut,
		path:   escapePath("org", "containers", name, resource),
		body:   body,
	})
	return err
}

// ListBuckets returns every bucket in the tenant. The endpoint takes no name
// filter and no pagination, so the response grows without bound with the size of
// the tenant. It is useful for inspection and cleanup, but a control loop should
// read individual buckets from their own sub-resources instead.
func (c *Client) ListBuckets(ctx context.Context, include []string) ([]Container, error) {
	r := request{method: http.MethodGet, path: containersPath}
	if len(include) > 0 {
		r.query = url.Values{"include": {strings.Join(include, ",")}}
	}

	resp, err := c.doRetrying(ctx, r)
	if err != nil {
		return nil, err
	}
	var containers []Container
	if err := decodeData(resp, &containers); err != nil {
		return nil, fmt.Errorf("listing buckets: %w", err)
	}
	return containers, nil
}

// Regions returns the regions defined on the grid. Regions cannot be created
// through the tenant API, so this is a diagnostic for naming the configured
// regions after the grid has rejected one, not a pre-flight check.
func (c *Client) Regions(ctx context.Context) ([]string, error) {
	resp, err := c.doRetrying(ctx, request{method: http.MethodGet, path: "/org/regions"})
	if err != nil {
		return nil, err
	}
	var regions []string
	if decodeData(resp, &regions) == nil {
		return regions, nil
	}
	var wrapped struct {
		Regions []string `json:"regions"`
	}
	if err := decodeData(resp, &wrapped); err != nil {
		return nil, fmt.Errorf("listing regions: %w", err)
	}
	return wrapped.Regions, nil
}

// GetDeleteObjects reports whether a server-side emptying operation is running
// on the bucket.
func (c *Client) GetDeleteObjects(ctx context.Context, name string) (*DeleteObjectsStatus, error) {
	var status DeleteObjectsStatus
	if err := c.getSubResource(ctx, name, "delete-objects", &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// StartDeleteObjects begins a server-side deletion of every object in the
// bucket. It runs inside the grid, needs no S3 credentials, and handles versioned
// objects and delete markers, but it is asynchronous and unbounded, so the caller
// must not block on it.
//
// Partial failures are reported through alerts rather than the status code, so an
// error-severity alert is returned as an AlertError.
func (c *Client) StartDeleteObjects(ctx context.Context, name string) (*DeleteObjectsStatus, []Alert, error) {
	return c.setDeleteObjects(ctx, name, true)
}

// StopDeleteObjects halts a running emptying operation.
func (c *Client) StopDeleteObjects(ctx context.Context, name string) (*DeleteObjectsStatus, []Alert, error) {
	return c.setDeleteObjects(ctx, name, false)
}

func (c *Client) setDeleteObjects(
	ctx context.Context, name string, deleting bool,
) (*DeleteObjectsStatus, []Alert, error) {
	if err := ValidateBucketName(name); err != nil {
		return nil, nil, err
	}

	resp, err := c.doRetrying(ctx, request{
		method: http.MethodPost,
		path:   escapePath("org", "containers", name, "delete-objects"),
		body:   map[string]bool{"deleteObjects": deleting},
	})
	if err != nil {
		return nil, nil, err
	}

	var status DeleteObjectsStatus
	_ = decodeData(resp, &status)
	alerts := responseAlerts(resp)

	if bad := errorAlerts(alerts); len(bad) > 0 {
		return &status, alerts, &AlertError{
			Op:     fmt.Sprintf("emptying bucket %q", name),
			Alerts: bad,
		}
	}
	return &status, alerts, nil
}

// StepStatus is the result of one pass of the bucket delete state machine.
type StepStatus struct {
	Outcome DeleteOutcome `json:"outcome"`
	// Done means the bucket is gone and no further passes are needed.
	Done bool `json:"done"`
	// Retryable means progress was made but another pass is required.
	Retryable bool                 `json:"retryable"`
	Message   string               `json:"message"`
	Alerts    []Alert              `json:"alerts,omitempty"`
	Objects   *DeleteObjectsStatus `json:"objects,omitempty"`
}

// BucketDeleter drives the delete-and-optionally-empty sequence. Each Step
// advances the state machine by one pass and returns, which is what lets a
// caller with a deadline, such as a COSI sidecar, make progress across repeated
// invocations rather than blocking on an unbounded operation.
type BucketDeleter struct {
	client        *Client
	name          string
	emptyOnDelete bool
	// emptyingRequested records that this deleter already asked the grid to
	// empty the bucket, which is what distinguishes "emptying has not started"
	// from "emptying finished and objects remain".
	emptyingRequested bool
}

// NewBucketDeleter builds a deleter for one bucket. Emptying destroys data that
// was never explicitly nominated for destruction, so it must be opted into.
func (c *Client) NewBucketDeleter(name string, emptyOnDelete bool) *BucketDeleter {
	return &BucketDeleter{client: c, name: name, emptyOnDelete: emptyOnDelete}
}

// Step attempts the delete and advances the emptying state machine at most one
// step. It returns a status with Done set once the bucket is gone, a status with
// Retryable set when another pass is needed, or a terminal error.
func (d *BucketDeleter) Step(ctx context.Context) (*StepStatus, error) {
	outcome, err := d.client.DeleteBucket(ctx, d.name)
	if err != nil {
		return nil, err
	}

	switch outcome {
	case BucketDeleted:
		return &StepStatus{Outcome: outcome, Done: true, Message: "bucket deleted"}, nil
	case BucketAlreadyGone:
		return &StepStatus{
			Outcome: outcome,
			Done:    true,
			Message: "bucket does not exist, so there is nothing to delete",
		}, nil
	}

	// The bucket is not empty. Emptying is irreversible, so without the opt-in
	// this is a failed precondition for a human to resolve.
	if !d.emptyOnDelete {
		return nil, fmt.Errorf("%w: re-run with --empty-on-delete to remove its contents first",
			ErrBucketNotEmpty)
	}

	status, err := d.client.GetDeleteObjects(ctx, d.name)
	if err != nil {
		return nil, err
	}
	if status.IsDeletingObjects {
		d.emptyingRequested = true
		return &StepStatus{
			Outcome:   outcome,
			Retryable: true,
			Message:   "emptying in progress",
			Objects:   status,
		}, nil
	}
	if d.emptyingRequested {
		// Emptying finished and yet the delete still reports a non-empty bucket.
		// That is not transient, so stop rather than retry forever.
		return nil, d.objectsRemainError(ctx)
	}

	status, alerts, err := d.client.StartDeleteObjects(ctx, d.name)
	if err != nil {
		return nil, err
	}
	d.emptyingRequested = true
	return &StepStatus{
		Outcome:   outcome,
		Retryable: true,
		Message:   "emptying started",
		Alerts:    alerts,
		Objects:   status,
	}, nil
}

// Run drives Step until the bucket is gone or a terminal condition is reached,
// invoking onStep, if given, after every pass.
func (d *BucketDeleter) Run(
	ctx context.Context, poll time.Duration, onStep func(*StepStatus),
) (*StepStatus, error) {
	if poll <= 0 {
		poll = 10 * time.Second
	}
	for {
		status, err := d.Step(ctx)
		if err != nil {
			return nil, err
		}
		if onStep != nil {
			onStep(status)
		}
		if status.Done {
			return status, nil
		}

		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// objectsRemainError names Object Lock as the cause when it is enabled, since
// objects under an active retention period cannot be deleted until it expires.
func (d *BucketDeleter) objectsRemainError(ctx context.Context) error {
	lock, err := d.client.GetObjectLock(ctx, d.name)
	if err == nil && lock != nil && lock.Enabled {
		return fmt.Errorf("%w: S3 Object Lock is enabled on this bucket, so objects still under a "+
			"retention period cannot be removed until it expires", ErrObjectsRemain)
	}
	return fmt.Errorf("%w: objects may be being written to the bucket while it is being emptied",
		ErrObjectsRemain)
}
