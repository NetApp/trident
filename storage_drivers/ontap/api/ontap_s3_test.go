// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

// newOntapAPIREST is a test helper that wires a mock RestClientInterface into an OntapAPIREST.
func newOntapAPIREST(t *testing.T, ctrl *gomock.Controller) (*mockapi.MockRestClientInterface, api.OntapAPIREST) {
	t.Helper()
	rsi := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(rsi)
	if err != nil {
		t.Fatalf("NewOntapAPIRESTFromRestClientInterface: %v", err)
	}
	return rsi, oapi
}

// ---------------------------------------------------------------------------
// BucketGet
// ---------------------------------------------------------------------------

func TestOntapAPIREST_BucketGet_Found(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	name := "my-bucket"
	want := &api.Bucket{Name: name, UUID: "bucket-uuid-1"}

	rsi.EXPECT().S3BucketGet(ctx, name).Return(want, nil)

	got, err := oapi.BucketGetByName(ctx, name)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOntapAPIREST_BucketGet_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3BucketGet(ctx, "missing").Return(nil, nil)

	got, err := oapi.BucketGetByName(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestOntapAPIREST_BucketGet_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3BucketGet(ctx, "b").Return(nil, fmt.Errorf("transport error"))

	got, err := oapi.BucketGetByName(ctx, "b")
	assert.Error(t, err)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// BucketCreate
// ---------------------------------------------------------------------------

func TestOntapAPIREST_BucketCreate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	spec := api.BucketCreateConfig{Name: "new-bucket", Size: 1 << 30}
	want := &api.Bucket{Name: spec.Name, UUID: "bucket-uuid-new"}

	rsi.EXPECT().S3BucketCreate(ctx, spec).Return(want, nil)

	got, err := oapi.BucketCreate(ctx, spec)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOntapAPIREST_BucketCreate_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	spec := api.BucketCreateConfig{Name: "new-bucket"}

	rsi.EXPECT().S3BucketCreate(ctx, spec).Return(nil, fmt.Errorf("conflict"))

	got, err := oapi.BucketCreate(ctx, spec)
	assert.Error(t, err)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// BucketDelete
// ---------------------------------------------------------------------------

func TestOntapAPIREST_BucketDelete_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3BucketDelete(ctx, "bucket-uuid-1").Return(nil)

	err := oapi.BucketDelete(ctx, "bucket-uuid-1")
	assert.NoError(t, err)
}

func TestOntapAPIREST_BucketDelete_IdempotentNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// RestClient.S3BucketDelete absorbs 404 internally and returns nil.
	// OntapAPIREST just forwards whatever RestClient returns.
	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3BucketDelete(ctx, "gone-uuid").Return(nil)

	err := oapi.BucketDelete(ctx, "gone-uuid")
	assert.NoError(t, err)
}

func TestOntapAPIREST_BucketDelete_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3BucketDelete(ctx, "bucket-uuid-1").Return(fmt.Errorf("server error"))

	err := oapi.BucketDelete(ctx, "bucket-uuid-1")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// BucketPolicySet
// ---------------------------------------------------------------------------

func TestOntapAPIREST_BucketPolicySet_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	bucketUUID := "bucket-uuid-1"
	policy := api.BucketPolicy{
		Statements: []api.PolicyStatement{
			{
				Sid:        "AllowUser",
				Effect:     "allow",
				Principals: []string{"trident-user"},
				Actions:    []string{"GetObject", "PutObject"},
				Resources:  []string{"my-bucket", "my-bucket/*"},
			},
		},
	}

	rsi.EXPECT().S3BucketPolicySet(ctx, bucketUUID, policy).Return(nil)

	err := oapi.BucketPolicySet(ctx, bucketUUID, policy)
	assert.NoError(t, err)
}

func TestOntapAPIREST_BucketPolicySet_EmptyPolicy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	empty := api.BucketPolicy{}

	rsi.EXPECT().S3BucketPolicySet(ctx, "bucket-uuid-1", empty).Return(nil)

	err := oapi.BucketPolicySet(ctx, "bucket-uuid-1", empty)
	assert.NoError(t, err)
}

func TestOntapAPIREST_BucketPolicySet_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3BucketPolicySet(ctx, "bucket-uuid-1", gomock.Any()).Return(fmt.Errorf("patch failed"))

	err := oapi.BucketPolicySet(ctx, "bucket-uuid-1", api.BucketPolicy{})
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// ServiceGet
// ---------------------------------------------------------------------------

func TestOntapAPIREST_ServiceGet_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	want := &api.Service{SVMUUID: "svm-uuid-1", Name: "s3.example.com", Enabled: true}

	rsi.EXPECT().S3ServiceGet(ctx, "my-svm").Return(want, nil)

	got, err := oapi.ServiceGet(ctx, "my-svm")
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOntapAPIREST_ServiceGet_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3ServiceGet(ctx, "my-svm").Return(nil, fmt.Errorf("no S3 service configured"))

	got, err := oapi.ServiceGet(ctx, "my-svm")
	assert.Error(t, err)
	assert.Nil(t, got)
}

// ---------------------------------------------------------------------------
// User
// ---------------------------------------------------------------------------

func TestOntapAPIREST_UserGet_Found(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	want := &api.User{Name: "trident-user", KeyID: 1}

	rsi.EXPECT().S3UserGet(ctx, "trident-user").Return(want, nil)

	got, err := oapi.UserGet(ctx, "trident-user")
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOntapAPIREST_UserGet_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3UserGet(ctx, "missing").Return(nil, nil)

	got, err := oapi.UserGet(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestOntapAPIREST_UserGet_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3UserGet(ctx, "u").Return(nil, fmt.Errorf("transport error"))

	got, err := oapi.UserGet(ctx, "u")
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestOntapAPIREST_UserCreate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	want := &api.User{Name: "trident-user", AccessKey: "AK", SecretKey: "SK"}

	rsi.EXPECT().S3UserCreate(ctx, "trident-user").Return(want, nil)

	got, err := oapi.UserCreate(ctx, "trident-user")
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOntapAPIREST_UserCreate_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3UserCreate(ctx, "u").Return(nil, fmt.Errorf("conflict"))

	got, err := oapi.UserCreate(ctx, "u")
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestOntapAPIREST_UserDelete_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3UserDelete(ctx, "trident-user").Return(nil)

	err := oapi.UserDelete(ctx, "trident-user")
	assert.NoError(t, err)
}

func TestOntapAPIREST_UserDelete_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3UserDelete(ctx, "u").Return(fmt.Errorf("server error"))

	err := oapi.UserDelete(ctx, "u")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Policy
// ---------------------------------------------------------------------------

func TestOntapAPIREST_PolicyCreate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	statements := []api.SVMPolicyStatement{
		{Effect: "allow", Actions: []string{"*"}, Resources: []string{"b", "b/..."}},
	}

	rsi.EXPECT().S3PolicyCreate(ctx, "trident-access", statements).Return(nil)

	err := oapi.PolicyCreate(ctx, "trident-access", statements)
	assert.NoError(t, err)
}

func TestOntapAPIREST_PolicyCreate_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3PolicyCreate(ctx, "p", gomock.Any()).Return(fmt.Errorf("conflict"))

	err := oapi.PolicyCreate(ctx, "p", nil)
	assert.Error(t, err)
}

func TestOntapAPIREST_PolicyGet_Found(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	want := &api.SVMPolicy{Name: "trident-access"}

	rsi.EXPECT().S3PolicyGet(ctx, "trident-access").Return(want, nil)

	got, err := oapi.PolicyGet(ctx, "trident-access")
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOntapAPIREST_PolicyGet_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3PolicyGet(ctx, "missing").Return(nil, nil)

	got, err := oapi.PolicyGet(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestOntapAPIREST_PolicyDelete_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3PolicyDelete(ctx, "trident-access").Return(nil)

	err := oapi.PolicyDelete(ctx, "trident-access")
	assert.NoError(t, err)
}

func TestOntapAPIREST_PolicyDelete_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3PolicyDelete(ctx, "p").Return(fmt.Errorf("server error"))

	err := oapi.PolicyDelete(ctx, "p")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Group
// ---------------------------------------------------------------------------

func TestOntapAPIREST_GroupCreate_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	users := []string{"trident-access"}
	policies := []string{"trident-access"}

	rsi.EXPECT().S3GroupCreate(ctx, "trident-access", users, policies).Return(nil)

	err := oapi.GroupCreate(ctx, "trident-access", users, policies)
	assert.NoError(t, err)
}

func TestOntapAPIREST_GroupCreate_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3GroupCreate(ctx, "g", gomock.Any(), gomock.Any()).Return(fmt.Errorf("conflict"))

	err := oapi.GroupCreate(ctx, "g", nil, nil)
	assert.Error(t, err)
}

func TestOntapAPIREST_GroupGet_Found(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	want := &api.Group{Name: "trident-access"}

	rsi.EXPECT().S3GroupGet(ctx, "trident-access").Return(want, nil)

	got, err := oapi.GroupGet(ctx, "trident-access")
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestOntapAPIREST_GroupGet_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3GroupGet(ctx, "missing").Return(nil, nil)

	got, err := oapi.GroupGet(ctx, "missing")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestOntapAPIREST_GroupDelete_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3GroupDelete(ctx, "trident-access").Return(nil)

	err := oapi.GroupDelete(ctx, "trident-access")
	assert.NoError(t, err)
}

func TestOntapAPIREST_GroupDelete_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi, oapi := newOntapAPIREST(t, ctrl)
	rsi.EXPECT().S3GroupDelete(ctx, "g").Return(fmt.Errorf("server error"))

	err := oapi.GroupDelete(ctx, "g")
	assert.Error(t, err)
}
