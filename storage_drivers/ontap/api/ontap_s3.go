// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"fmt"

	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/object_store"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
)

// S3 is the management-plane interface the object driver calls.
// The production implementation is RestClient (methods added in this file).
// Tests inject a fake.
//
// This is analogous to OntapAPI, but intentionally trims down the interface
// surface to be precisely what an object driver needs.
type S3 interface {
	IsREST() bool
	SVMName() string
	APIVersion(ctx context.Context, cached bool) (string, error)
	ServiceGet(ctx context.Context, svmName string) (*Service, error)

	BucketGetByName(ctx context.Context, name string) (*Bucket, error)
	BucketCreate(ctx context.Context, config BucketCreateConfig) (*Bucket, error)
	BucketDelete(ctx context.Context, bucketUUID string) error
	BucketPolicySet(ctx context.Context, bucketUUID string, policy BucketPolicy) error

	UserGet(ctx context.Context, name string) (*User, error)
	UserCreate(ctx context.Context, name string) (*User, error)
	UserDelete(ctx context.Context, name string) error

	PolicyCreate(ctx context.Context, name string, statements []SVMPolicyStatement) error
	PolicyGet(ctx context.Context, name string) (*SVMPolicy, error)
	PolicyDelete(ctx context.Context, name string) error

	GroupCreate(ctx context.Context, name string, users []string, policies []string) error
	GroupGet(ctx context.Context, name string) (*Group, error)
	GroupDelete(ctx context.Context, name string) error
}

// Service is returned by ServiceGet.
// Projected from the generated models.S3Service — only what Initialize needs.
type Service struct {
	SVMUUID string // ONTAP UUID of the SVM; required on every /protocols/s3/services/{svm.uuid}/... call
	Name    string // S3 server FQDN, e.g. "s3.dev-shared.trident.netapp.com"
	Enabled bool   // false → Initialize should fail
}

// BucketCreateConfig is the driver's input for provisioning a bucket.
// Zero values mean "let ONTAP default".
type BucketCreateConfig struct {
	Name            string // required
	Size            int64  // size in bytes; if set, must be greater than 95MiB
	VersioningState string // "enabled" | "disabled" | "suspended"
	QoSPolicy       string // name of an existing ONTAP QoS policy; empty = none
}

// PolicyStatement is one statement in a bucket-level access policy.
// Principals are required here; they name the S3 users that the statement applies to.
type PolicyStatement struct {
	Sid        string   // optional stable identifier
	Effect     string   // "allow" | "deny"
	Principals []string // S3 user names or "*"
	Actions    []string // e.g. "GetObject", "PutObject"
	Resources  []string // bucket name and "bucket/..."
}

// BucketPolicy is the full access policy attached to a bucket.
type BucketPolicy struct {
	Statements []PolicyStatement
}

// SVMPolicyStatement is one statement in an SVM-level S3 policy.
// There are no principals; access is granted by adding a User to a Group that owns this policy.
type SVMPolicyStatement struct {
	Sid       string   // optional stable identifier; max 256 chars
	Effect    string   // "allow" | "deny"
	Actions   []string // e.g. "GetObject", "PutObject", or "*" for all
	Resources []string // e.g. "bucket/..." for all objects in a bucket
}

// SVMPolicy is an SVM-level S3 access policy.
// Projected from models.S3Policy.
type SVMPolicy struct {
	Name       string
	Statements []SVMPolicyStatement
}

// Group is an SVM-level S3 group.
// A group owns one or more policies and contains one or more users.
// Projected from models.S3Group.
type Group struct {
	Name string
}

// Bucket is returned by BucketGet / BucketCreate.
// Projected from the generated models.S3BucketSvm.
type Bucket struct {
	Name   string        // bucket name
	UUID   string        // ONTAP UUID; required by BucketDelete and BucketPolicySet
	Policy *BucketPolicy // populated only when fetched with fields=policy
}

// User is returned by UserGet.
// Keys are never present on a GET — ONTAP only returns them at creation time.
type User struct {
	Name      string
	Location  string
	KeyID     int64
	AccessKey string
	SecretKey string
}

// UserKeys is returned only from UserCreate.
// The secret is shown exactly once — store it immediately.
type UserKeys struct {
}

// ---------------------------------------------------------------------------
// ONTAP S3 API Adapter - Implementation for satisfying ONTAP S3 Interface
// ---------------------------------------------------------------------------

func (d OntapAPIREST) ServiceGet(ctx context.Context, svmName string) (*Service, error) {
	return d.api.S3ServiceGet(ctx, svmName)
}

func (d OntapAPIREST) BucketGetByName(ctx context.Context, name string) (*Bucket, error) {
	return d.api.S3BucketGet(ctx, name)
}

func (d OntapAPIREST) BucketCreate(ctx context.Context, config BucketCreateConfig) (*Bucket, error) {
	return d.api.S3BucketCreate(ctx, config)
}

func (d OntapAPIREST) BucketDelete(ctx context.Context, bucketUUID string) error {
	return d.api.S3BucketDelete(ctx, bucketUUID)
}

func (d OntapAPIREST) BucketPolicySet(ctx context.Context, bucketUUID string, policy BucketPolicy) error {
	return d.api.S3BucketPolicySet(ctx, bucketUUID, policy)
}

func (d OntapAPIREST) UserGet(ctx context.Context, name string) (*User, error) {
	return d.api.S3UserGet(ctx, name)
}

func (d OntapAPIREST) UserCreate(ctx context.Context, name string) (*User, error) {
	return d.api.S3UserCreate(ctx, name)
}

func (d OntapAPIREST) UserDelete(ctx context.Context, name string) error {
	return d.api.S3UserDelete(ctx, name)
}

func (d OntapAPIREST) PolicyCreate(ctx context.Context, name string, statements []SVMPolicyStatement) error {
	return d.api.S3PolicyCreate(ctx, name, statements)
}

func (d OntapAPIREST) PolicyGet(ctx context.Context, name string) (*SVMPolicy, error) {
	return d.api.S3PolicyGet(ctx, name)
}

func (d OntapAPIREST) PolicyDelete(ctx context.Context, name string) error {
	return d.api.S3PolicyDelete(ctx, name)
}

func (d OntapAPIREST) GroupCreate(ctx context.Context, name string, users []string, policies []string) error {
	return d.api.S3GroupCreate(ctx, name, users, policies)
}

func (d OntapAPIREST) GroupGet(ctx context.Context, name string) (*Group, error) {
	return d.api.S3GroupGet(ctx, name)
}

func (d OntapAPIREST) GroupDelete(ctx context.Context, name string) error {
	return d.api.S3GroupDelete(ctx, name)
}

// ---------------------------------------------------------------------------
// RestClient S3 methods - Implementation for raw API binding wrapper.
// ---------------------------------------------------------------------------

// S3ServiceGet returns the S3 service configuration for the named SVM.
func (c *RestClient) S3ServiceGet(ctx context.Context, svmName string) (*Service, error) {
	svmUUID := c.SVMUUID()
	if svmUUID == "" {
		return nil, fmt.Errorf("SVM UUID for %q is not resolved; the REST client must be constructed via "+
			"NewRestClientFromOntapConfig (or otherwise call EnsureSVMWithRest) before use", svmName)
	}

	params := object_store.NewS3ServiceGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = svmUUID

	resp, err := c.api.ObjectStore.S3ServiceGet(params, c.authInfo)
	if err != nil {
		if isRestNotFoundError(err) {
			return nil, fmt.Errorf("no S3 service configured on SVM %q", svmName)
		}
		return nil, err
	}
	if resp.Payload == nil {
		return nil, fmt.Errorf("S3 service on SVM %q returned no data", svmName)
	}

	svc := &Service{SVMUUID: svmUUID}
	if resp.Payload.Name != nil {
		svc.Name = *resp.Payload.Name
	}
	if resp.Payload.Enabled != nil {
		svc.Enabled = *resp.Payload.Enabled
	}
	return svc, nil
}

// S3BucketGet returns the named bucket on the SVM, or (nil, nil) if not found.
func (c *RestClient) S3BucketGet(ctx context.Context, name string) (*Bucket, error) {
	params := object_store.NewBucketsCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.Name = &name
	params.SetFields([]string{"name", "uuid", "policy"})

	resp, err := c.api.ObjectStore.BucketsCollectionGet(params, c.authInfo)
	if err != nil {
		if isRestNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	if resp.Payload == nil || len(resp.Payload.S3BucketSvmResponseInlineRecords) == 0 {
		return nil, nil
	}
	return bucketFromModel(resp.Payload.S3BucketSvmResponseInlineRecords[0]), nil
}

// S3BucketCreate provisions a new S3 bucket on the SVM, then fetches and returns it.
func (c *RestClient) S3BucketCreate(ctx context.Context, spec BucketCreateConfig) (*Bucket, error) {
	body := &models.S3BucketSvm{}
	name := spec.Name
	body.Name = &name
	if spec.Size > 0 {
		body.Size = &spec.Size
	}
	if spec.VersioningState != "" {
		body.VersioningState = &spec.VersioningState
	}
	if spec.QoSPolicy != "" {
		body.QosPolicy = &models.S3BucketSvmInlineQosPolicy{Name: &spec.QoSPolicy}
	}

	params := object_store.NewS3BucketSvmCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.Info = body

	created, accepted, err := c.api.ObjectStore.S3BucketSvmCreate(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	switch {
	case accepted != nil:
		jobLink := getGenericJobLinkFromS3BucketSvmJobLinkResponse(accepted.Payload)
		if err := c.PollJobStatus(ctx, jobLink); err != nil {
			return nil, err
		}
	case created != nil:
		// 201 synchronous — operation complete
	default:
		return nil, fmt.Errorf("unexpected response from S3 bucket create")
	}

	bucket, err := c.S3BucketGet(ctx, spec.Name)
	if err != nil {
		return nil, err
	}
	if bucket == nil {
		return nil, fmt.Errorf("bucket %q not found after creation", spec.Name)
	}
	return bucket, nil
}

// S3BucketDelete removes an S3 bucket. 404 is treated as success (idempotent).
func (c *RestClient) S3BucketDelete(ctx context.Context, bucketUUID string) error {
	params := object_store.NewS3BucketSvmDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.UUID = bucketUUID

	_, accepted, err := c.api.ObjectStore.S3BucketSvmDelete(params, c.authInfo)
	if err != nil {
		if isRestNotFoundError(err) {
			return nil
		}
		return err
	}
	if accepted != nil {
		jobLink := getGenericJobLinkFromS3BucketSvmJobLinkResponse(accepted.Payload)
		return c.PollJobStatus(ctx, jobLink)
	}
	return nil
}

// S3BucketPolicySet replaces the bucket policy in full via PATCH.
func (c *RestClient) S3BucketPolicySet(ctx context.Context, bucketUUID string, policy BucketPolicy) error {
	statements := make([]*models.S3BucketPolicyStatement, 0, len(policy.Statements))
	for i := range policy.Statements {
		s := &policy.Statements[i]
		stmt := &models.S3BucketPolicyStatement{Effect: &s.Effect}
		if s.Sid != "" {
			stmt.Sid = &s.Sid
		}
		for _, p := range s.Principals {
			p := p
			stmt.S3BucketPolicyStatementInlinePrincipals = append(stmt.S3BucketPolicyStatementInlinePrincipals, &p)
		}
		for _, a := range s.Actions {
			a := a
			stmt.S3BucketPolicyStatementInlineActions = append(stmt.S3BucketPolicyStatementInlineActions, &a)
		}
		for _, r := range s.Resources {
			r := r
			stmt.S3BucketPolicyStatementInlineResources = append(stmt.S3BucketPolicyStatementInlineResources, &r)
		}
		statements = append(statements, stmt)
	}

	body := &models.S3BucketSvm{
		Policy: &models.S3BucketSvmInlinePolicy{Statements: statements},
	}

	params := object_store.NewS3BucketSvmModifyParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.UUID = bucketUUID
	params.Info = body

	_, accepted, err := c.api.ObjectStore.S3BucketSvmModify(params, c.authInfo)
	if err != nil {
		return err
	}
	if accepted != nil {
		jobLink := getGenericJobLinkFromS3BucketSvmJobLinkResponse(accepted.Payload)
		return c.PollJobStatus(ctx, jobLink)
	}
	return nil
}

// S3UserGet returns the named S3 user, or (nil, nil) if not found.
func (c *RestClient) S3UserGet(ctx context.Context, name string) (*User, error) {
	params := object_store.NewS3UserGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.Name = name

	user := &User{}
	resp, err := c.api.ObjectStore.S3UserGet(params, c.authInfo)
	if err != nil {
		if isRestNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	if resp.Payload == nil {
		return nil, nil
	}

	if resp.Payload.Name != nil {
		user.Name = *resp.Payload.Name
	}
	if resp.Payload.KeyID != nil {
		user.KeyID = *resp.Payload.KeyID
	}
	if resp.Payload.AccessKey != nil {
		user.AccessKey = *resp.Payload.AccessKey
	}
	if resp.Payload.SecretKey != nil {
		user.SecretKey = *resp.Payload.SecretKey
	}
	return user, nil
}

// S3UserCreate creates an S3 user and returns its credentials.
// The secret key is shown exactly once — the caller must store it immediately.
func (c *RestClient) S3UserCreate(ctx context.Context, name string) (*User, error) {
	userName := name
	body := &models.S3User{Name: new(userName)}

	params := object_store.NewS3UserCreateParamsWithTimeout(c.httpClient.Timeout)
	params.SvmUUID = c.svmUUID
	params.HTTPClient = c.httpClient
	params.Context = ctx
	params.Info = body

	user := &User{Name: userName}
	resp, err := c.api.ObjectStore.S3UserCreate(params, c.authInfo)
	if err != nil {
		return nil, err
	}
	if resp.Payload == nil {
		return nil, fmt.Errorf("ONTAP S3 user create returned no payload")
	}
	user.Location = resp.Location

	records := resp.Payload.S3UserPostPatchResponseInlineRecords
	if len(records) == 0 || records[0] == nil {
		return nil, fmt.Errorf("ONTAP S3 user create returned no credential record")
	}
	rec := records[0]

	if rec.AccessKey != nil {
		user.AccessKey = *rec.AccessKey
	}
	if rec.SecretKey != nil {
		user.SecretKey = *rec.SecretKey
	}
	if user.AccessKey == "" || user.SecretKey == "" {
		return nil, fmt.Errorf("ONTAP S3 user create for %s returned empty credentials", name)
	}
	return user, nil
}

// S3UserDelete removes an S3 user. 404 is treated as success (idempotent).
func (c *RestClient) S3UserDelete(ctx context.Context, name string) error {
	params := object_store.NewS3UserDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.SvmUUID = c.svmUUID
	params.HTTPClient = c.httpClient

	params.Name = name
	params.Context = ctx

	_, err := c.api.ObjectStore.S3UserDelete(params, c.authInfo)
	if err != nil && !isRestNotFoundError(err) {
		return err
	}
	return nil
}

// S3PolicyCreate creates an SVM-level S3 policy with the given statements.
func (c *RestClient) S3PolicyCreate(ctx context.Context, name string, statements []SVMPolicyStatement) error {
	stmts := make([]*models.S3PolicyStatement, 0, len(statements))
	for i := range statements {
		s := &statements[i]
		stmt := &models.S3PolicyStatement{Effect: &s.Effect}
		if s.Sid != "" {
			stmt.Sid = &s.Sid
		}
		for _, a := range s.Actions {
			a := a
			stmt.S3PolicyStatementInlineActions = append(stmt.S3PolicyStatementInlineActions, &a)
		}
		for _, r := range s.Resources {
			r := r
			stmt.S3PolicyStatementInlineResources = append(stmt.S3PolicyStatementInlineResources, &r)
		}
		stmts = append(stmts, stmt)
	}

	body := &models.S3Policy{Name: &name, S3PolicyInlineStatements: stmts}

	params := object_store.NewS3PolicyCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.Info = body

	_, err := c.api.ObjectStore.S3PolicyCreate(params, c.authInfo)
	return err
}

// S3PolicyGet returns the named SVM-level policy, or (nil, nil) if not found.
func (c *RestClient) S3PolicyGet(ctx context.Context, name string) (*SVMPolicy, error) {
	params := object_store.NewS3PolicyGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.Name = name

	resp, err := c.api.ObjectStore.S3PolicyGet(params, c.authInfo)
	if err != nil {
		if isRestNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	if resp.Payload == nil || resp.Payload.Name == nil {
		return nil, nil
	}
	return &SVMPolicy{Name: *resp.Payload.Name}, nil
}

// S3PolicyDelete removes an SVM-level S3 policy. 404 is treated as success (idempotent).
func (c *RestClient) S3PolicyDelete(ctx context.Context, name string) error {
	params := object_store.NewS3PolicyDeleteParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.Name = name

	_, err := c.api.ObjectStore.S3PolicyDelete(params, c.authInfo)
	if err != nil && !isRestNotFoundError(err) {
		return err
	}
	return nil
}

// S3GroupCreate creates a group with the given users and policies in one call.
// ONTAP requires at least one user; create the user before calling this.
func (c *RestClient) S3GroupCreate(ctx context.Context, name string, users []string, policies []string) error {
	body := &models.S3Group{}
	n := name
	body.Name = &n

	for _, u := range users {
		u := u
		body.S3GroupInlineUsers = append(body.S3GroupInlineUsers, &models.S3GroupInlineUsersInlineArrayItem{Name: &u})
	}
	for _, p := range policies {
		p := p
		body.S3GroupInlinePolicies = append(body.S3GroupInlinePolicies, &models.S3GroupInlinePoliciesInlineArrayItem{Name: &p})
	}

	params := object_store.NewS3GroupCreateParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.Info = body

	_, err := c.api.ObjectStore.S3GroupCreate(params, c.authInfo)
	return err
}

// S3GroupGet returns the named group, or (nil, nil) if not found.
func (c *RestClient) S3GroupGet(ctx context.Context, name string) (*Group, error) {
	params := object_store.NewS3GroupCollectionGetParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.Name = &name

	resp, err := c.api.ObjectStore.S3GroupCollectionGet(params, c.authInfo)
	if err != nil {
		if isRestNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	if resp.Payload == nil || len(resp.Payload.S3GroupResponseInlineRecords) == 0 {
		return nil, nil
	}
	rec := resp.Payload.S3GroupResponseInlineRecords[0]
	if rec.Name == nil {
		return nil, nil
	}
	return &Group{Name: *rec.Name}, nil
}

// S3GroupDelete removes a group by name via collection DELETE. 404 is treated as success (idempotent).
func (c *RestClient) S3GroupDelete(ctx context.Context, name string) error {
	params := object_store.NewS3GroupDeleteCollectionParamsWithTimeout(c.httpClient.Timeout)
	params.Context = ctx
	params.HTTPClient = c.httpClient
	params.SvmUUID = c.svmUUID
	params.Name = &name

	_, err := c.api.ObjectStore.S3GroupDeleteCollection(params, c.authInfo)
	if err != nil && !isRestNotFoundError(err) {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func getGenericJobLinkFromS3BucketSvmJobLinkResponse(r *models.S3BucketSvmJobLinkResponse) *models.JobLinkResponse {
	jobLink := &models.JobLinkResponse{}
	if r != nil {
		jobLink.Job = r.Job
	}
	return jobLink
}

func bucketFromModel(b *models.S3BucketSvm) *Bucket {
	if b == nil {
		return nil
	}
	bucket := &Bucket{}
	if b.Name != nil {
		bucket.Name = *b.Name
	}
	if b.UUID != nil {
		bucket.UUID = b.UUID.String()
	}
	if b.Policy != nil {
		bucket.Policy = bucketPolicyFromModel(b.Policy)
	}
	return bucket
}

func bucketPolicyFromModel(p *models.S3BucketSvmInlinePolicy) *BucketPolicy {
	if p == nil {
		return nil
	}
	policy := &BucketPolicy{}
	for _, s := range p.Statements {
		if s == nil {
			continue
		}
		stmt := PolicyStatement{}
		if s.Effect != nil {
			stmt.Effect = *s.Effect
		}
		if s.Sid != nil {
			stmt.Sid = *s.Sid
		}
		for _, principal := range s.S3BucketPolicyStatementInlinePrincipals {
			if principal != nil {
				stmt.Principals = append(stmt.Principals, *principal)
			}
		}
		for _, action := range s.S3BucketPolicyStatementInlineActions {
			if action != nil {
				stmt.Actions = append(stmt.Actions, *action)
			}
		}
		for _, resource := range s.S3BucketPolicyStatementInlineResources {
			if resource != nil {
				stmt.Resources = append(stmt.Resources, *resource)
			}
		}
		policy.Statements = append(policy.Statements, stmt)
	}
	return policy
}
