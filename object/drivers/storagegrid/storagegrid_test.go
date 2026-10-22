// Copyright 2026 NetApp, Inc. All Rights Reserved.

package storagegrid

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/object"
	"github.com/netapp/trident/object/drivers/storagegrid/sgapi"
	"github.com/netapp/trident/object/factory"
	"github.com/netapp/trident/utils/errors"
)

func TestDriverIsRegistered(t *testing.T) {
	store, err := factory.CreateStore(context.Background(), DriverName, nil, []byte(
		`{"objectStoreName":"sg","managementEndpoint":"https://grid.example"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, DriverName, store.DriverName())
	assert.Equal(t, "sg", store.Name())
}

func TestSecretsOmittedFromJSON(t *testing.T) {
	d := &ObjectDriver{
		ObjectStoreName:    "sg",
		ManagementEndpoint: "https://grid.example",
		AccountID:          "12345678901234567890",
		Credentials: Credentials{
			Username: "root",
			Password: "secret",
		},
	}
	raw, err := json.Marshal(d)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "secret")
	assert.Contains(t, string(raw), `"accountId":"12345678901234567890"`)

	secrets := d.Secrets().(map[string]string)
	assert.Equal(t, "root", secrets["username"])
	assert.Equal(t, "secret", secrets["password"])
	assert.NotContains(t, secrets, "accountId")
}

func TestNewObjectDriverUsesSecretConfig(t *testing.T) {
	store, err := NewObjectDriver(
		context.Background(),
		[]byte(`{"username":"root","password":"secret"}`),
		[]byte(`{"objectStoreName":"sg","managementEndpoint":"https://grid.example","accountId":"12345678901234567890"}`),
	)
	require.NoError(t, err)
	d := store.(*ObjectDriver)
	assert.Equal(t, "root", d.Username)
	assert.Equal(t, "secret", d.Password)
	assert.Equal(t, "12345678901234567890", d.AccountID)
}

func TestMatchParams(t *testing.T) {
	d := &ObjectDriver{
		ObjectStoreName: "sg",
		Labels: map[string]string{
			"performance": "gold",
			"protection":  "minimal",
			"cost":        "100",
		},
	}
	tests := []struct {
		name   string
		params map[string]string
		want   bool
	}{
		{name: "nil params", params: nil, want: true},
		{name: "empty params", params: map[string]string{}, want: true},
		{name: "equal match", params: map[string]string{"selector": "performance=gold"}, want: true},
		{name: "equal mismatch", params: map[string]string{"selector": "performance=silver"}, want: false},
		{name: "not equal match", params: map[string]string{"selector": "protection != full"}, want: true},
		{name: "not equal mismatch", params: map[string]string{"selector": "protection != minimal"}, want: false},
		{name: "in match", params: map[string]string{"selector": "performance in (gold, silver)"}, want: true},
		{name: "in mismatch", params: map[string]string{"selector": "performance in (silver, bronze)"}, want: false},
		{name: "notin match", params: map[string]string{"selector": "performance notin (silver, bronze)"}, want: true},
		{name: "notin mismatch", params: map[string]string{"selector": "performance notin (gold, silver)"}, want: false},
		{name: "exists match", params: map[string]string{"selector": "protection"}, want: true},
		{name: "exists mismatch", params: map[string]string{"selector": "cloud"}, want: false},
		{name: "not exists match", params: map[string]string{"selector": "!cloud"}, want: true},
		{name: "not exists mismatch", params: map[string]string{"selector": "!protection"}, want: false},
		{
			name:   "compound match",
			params: map[string]string{"selector": "performance=gold; protection!=full; !cloud"},
			want:   true,
		},
		{
			name:   "compound mismatch",
			params: map[string]string{"selector": "performance=gold; protection=full"},
			want:   false,
		},
		{name: "numeric label via selector", params: map[string]string{"selector": "cost=100"}, want: true},
		{name: "invalid selector", params: map[string]string{"selector": "performance in gold"}, want: false},
		{name: "empty selector", params: map[string]string{"selector": ""}, want: false},
		{name: "objectStoreType match", params: map[string]string{"objectStoreType": DriverName}, want: true},
		{name: "objectStoreType mismatch", params: map[string]string{"objectStoreType": "fake"}, want: false},
		{name: "unknown key rejects the store", params: map[string]string{"objectStoreName": "other"}, want: false},
		{
			name: "selector and objectStoreType",
			params: map[string]string{
				"objectStoreType": DriverName,
				"selector":        "performance=gold",
			},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, d.MatchParams(test.params))
		})
	}
}

func TestStateRoundTripsInJSON(t *testing.T) {
	d := newTestDriver(t)
	d.SetState(object.StoreStateDeleting)

	serialized, err := json.Marshal(d)
	require.NoError(t, err)
	assert.NotContains(t, string(serialized), "secret")
	assert.Contains(t, string(serialized), `"accountId":"12345678901234567890"`)
	assert.Contains(t, string(serialized), `"state":"deleting"`)
	assert.Contains(t, string(serialized), `"uuid":"`)

	roundTrip, err := NewObjectDriver(
		context.Background(),
		[]byte(`{"username":"root","password":"secret"}`), serialized,
	)
	require.NoError(t, err)
	assert.Equal(t, object.StoreStateDeleting, roundTrip.State())
	assert.Equal(t, d.StoreUUID(), roundTrip.StoreUUID())
	restored := roundTrip.(*ObjectDriver)
	assert.Equal(t, "root", restored.Username)
	assert.Equal(t, "secret", restored.Password)
	assert.Equal(t, "12345678901234567890", restored.AccountID)
	assert.True(t, restored.MatchParams(map[string]string{"selector": "performance=gold"}))
}

func TestInitializeCallsVersionsAndAuthorize(t *testing.T) {
	fake := newFakeAPI()
	restore := swapClientFactory(fake)
	t.Cleanup(restore)

	store, err := NewObjectDriver(context.Background(), []byte(`{"username":"root","password":"secret"}`), []byte(`{
		"objectStoreName":"sg",
		"managementEndpoint":"https://grid.example",
		"s3Endpoint":"https://s3.grid.example",
		"accountId":"12345678901234567890"
	}`))
	require.NoError(t, err)

	require.NoError(t, store.Initialize(context.Background()))
	assert.Equal(t, 1, fake.versionsCalls)
	assert.Equal(t, 1, fake.authorizeCalls)
	assert.NotEmpty(t, store.StoreUUID())
}

// TestInitializeTrustedCACertificate uses the real Tenant Management API client.
// The fake client swapped in by the other tests never opens TLS, so it cannot
// show that trustedCACertificate is what makes the server certificate trusted.
func TestInitializeTrustedCACertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case http.MethodGet + " /api/v4/versions":
			_, _ = w.Write([]byte(`{"status":"success","apiVersion":"4.0","data":[4]}`))
		case http.MethodPost + " /api/v4/authorize":
			_, _ = w.Write([]byte(`{"status":"success","apiVersion":"4.0","data":"test-token"}`))
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	cert := server.Certificate()
	require.NotNil(t, cert)
	trustedCA := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}))

	t.Run("trusted CA", func(t *testing.T) {
		d := newTLSTestDriver(server.URL, trustedCA)
		require.NoError(t, d.Initialize(context.Background()))
		assert.NotEmpty(t, d.StoreUUID())
	})

	t.Run("untrusted server certificate", func(t *testing.T) {
		d := newTLSTestDriver(server.URL, "")
		err := d.Initialize(context.Background())
		require.Error(t, err)
		assert.ErrorContains(t, err, "certificate signed by unknown authority")
	})
}

func newTLSTestDriver(endpoint, trustedCA string) *ObjectDriver {
	return &ObjectDriver{
		ObjectStoreName:      "sg",
		ManagementEndpoint:   endpoint,
		S3Endpoint:           "https://s3.grid.example",
		AccountID:            "12345678901234567890",
		Credentials:          Credentials{Username: "root", Password: "secret"},
		TrustedCACertificate: trustedCA,
	}
}

func TestCreateGetDeleteBucket(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)

	bucket := &object.Bucket{Name: "my-bucket"}
	err := d.CreateBucket(ctx, bucket)
	require.NoError(t, err)
	require.NotNil(t, bucket.Info.S3, "CreateBucket must populate Info so COSI can reach the bucket")
	assert.Equal(t, "https://s3.grid.example", bucket.Info.S3.Endpoint)
	assert.Equal(t, "us-east-1", bucket.Info.S3.Region)

	got, err := d.GetBucketByName(ctx, bucket.Name)
	require.NoError(t, err)
	assert.Equal(t, "my-bucket", got.Name)
	assert.Equal(t, bucket.InternalID, got.InternalID)
	assert.Equal(t, d.StoreUUID(), got.StoreUUID)
	require.NotNil(t, got.Info.S3, "GetBucketByName must populate Info the same as CreateBucket")
	assert.Equal(t, bucket.Info, got.Info)

	require.NoError(t, d.DeleteBucket(ctx, "my-bucket"))
	_, err = d.GetBucketByName(ctx, "my-bucket")
	assert.True(t, errors.IsNotFoundError(err))
}

func TestCreateBucketIdempotentOnThisTenant(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)
	bucket1 := &object.Bucket{Name: "dup-bucket"}
	bucket2 := &object.Bucket{Name: "dup-bucket"}
	err1 := d.CreateBucket(ctx, bucket1)
	require.NoError(t, err1)
	err2 := d.CreateBucket(ctx, bucket2)
	require.NoError(t, err2)
	require.NotNil(t, bucket2.Info.S3, "the idempotent-retry path must also populate Info, not just the first create")
	assert.Equal(t, bucket1.Info, bucket2.Info)
}

// TestCreateBucketAlertErrorStillPopulatesInfo guards the case core.AddBucket's cleanup
// depends on: StorageGRID can create the bucket successfully while still reporting an
// error-severity alert (e.g. a failed default-retention setup). Info must be populated on
// the caller's pointer despite the error return, so a resumed retry short-circuits instead
// of needlessly re-fetching it.
func TestCreateBucketAlertErrorStillPopulatesInfo(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)
	fake := d.client.(*fakeAPI)
	fake.alertOnCreate = &sgapi.AlertError{
		Op:     "creating bucket",
		Alerts: []sgapi.Alert{{Severity: sgapi.AlertSeverityError, Text: "default retention configuration failed"}},
	}

	bucket := &object.Bucket{Name: "alert-bucket"}
	err := d.CreateBucket(ctx, bucket)
	require.Error(t, err)
	require.NotNil(t, bucket.Info.S3, "Info must be populated on the input pointer even though an error was returned")
	assert.Equal(t, "https://s3.grid.example", bucket.Info.S3.Endpoint)
	assert.Equal(t, d.StoreUUID(), bucket.StoreUUID)

	// The bucket actually exists on the backend despite the reported error.
	got, err := d.GetBucketByName(ctx, "alert-bucket")
	require.NoError(t, err, "the bucket must have actually been created despite the alert error")
	assert.Equal(t, bucket.Info, got.Info)
}

func TestCreateBucketInvalidNameIsInvalidInput(t *testing.T) {
	d := newTestDriver(t)

	err := d.CreateBucket(context.Background(), &object.Bucket{Name: "Not_A_Valid_Name"})
	require.Error(t, err)
	assert.True(t, errors.IsInvalidInputError(err), "a bad bucket name must be classified as invalid input: %v", err)
	assert.True(t, sgapi.IsBucketNameInvalid(err), "the underlying validation error must be preserved")
}

// TestDeleteBucketAbsentIsSuccess: a bucket this tenant does not have is a
// successful delete. That includes a name another tenant already owns: create
// reports a conflict, and the cleanup delete must still succeed because the
// bucket is not here.
func TestDeleteBucketAbsentIsSuccess(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)
	fake := d.client.(*fakeAPI)

	require.NoError(t, d.DeleteBucket(ctx, "never-created"))
	assert.Equal(t, 0, fake.deleteCalls, "a bucket that is not on this tenant must not be deleted on the grid")
}

func TestDeleteBucketNameConflictIsNotThisTenant(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)
	fake := d.client.(*fakeAPI)
	// The name is taken on the grid, but not by this tenant. Create and the
	// region read both return 409, which is what the grid does for a foreign name.
	fake.foreignNames = map[string]struct{}{"trident-taken-elsewhere": {}}

	err := d.CreateBucket(ctx, &object.Bucket{Name: "taken-elsewhere"})
	require.Error(t, err)
	assert.True(t, errors.IsAlreadyExistsError(err), "a name owned by another tenant is a conflict: %v", err)

	require.NoError(t, d.DeleteBucket(ctx, "taken-elsewhere"),
		"delete after that conflict must succeed, because the bucket is not on this tenant")
	assert.Equal(t, 0, fake.deleteCalls, "another tenant's bucket must not be deleted")
	_, stillGone := fake.buckets["taken-elsewhere"]
	assert.False(t, stillGone)
}

// TestDeleteBucketInvalidNameIsSuccess: a name the grid would reject was never
// created. Delete is a no-op so a failed create of that name can be cleaned up.
func TestDeleteBucketInvalidNameIsSuccess(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)
	fake := d.client.(*fakeAPI)

	require.NoError(t, d.DeleteBucket(ctx, "Not_A_Valid_Name"))
	assert.Equal(t, 0, fake.deleteCalls)
}

// TestDeleteBucketLookupFailureDoesNotDelete: if the existence check fails for
// a reason other than not-found, the bucket might be ours. Do not delete.
func TestDeleteBucketLookupFailureDoesNotDelete(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)
	fake := d.client.(*fakeAPI)
	fake.buckets["maybe-ours"] = "us-east-1"
	fake.getErr = &sgapi.APIError{StatusCode: http.StatusServiceUnavailable, Key: "unavailable"}

	err := d.DeleteBucket(ctx, "maybe-ours")
	require.Error(t, err)
	assert.Equal(t, 0, fake.deleteCalls, "an inconclusive lookup must not delete")
	_, stillThere := fake.buckets["maybe-ours"]
	assert.True(t, stillThere, "the bucket must still be present")
}

func TestDeleteBucketNotEmptyIsBucketNotEmpty(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)
	err := d.CreateBucket(ctx, &object.Bucket{Name: "full-bucket"})
	require.NoError(t, err)
	d.client.(*fakeAPI).notEmptyOnDelete = true

	err = d.DeleteBucket(ctx, "full-bucket")
	require.Error(t, err)
	assert.True(t, errors.IsBucketNotEmptyError(err), "a non-empty bucket must be classified as not empty: %v", err)
	assert.ErrorIs(t, err, sgapi.ErrBucketNotEmpty, "the underlying sgapi error must be preserved")
}

func TestGrantRevokeCallsIdentityAPIs(t *testing.T) {
	ctx := context.Background()
	d := newTestDriver(t)
	err := d.CreateBucket(ctx, &object.Bucket{Name: "cred-bucket"})
	require.NoError(t, err)

	modes := []object.BucketAccessMode{{BucketName: "cred-bucket", AccessMode: object.AccessModeReadWrite}}
	access := &object.BucketAccess{Name: "access-account"}
	secret, err := d.GrantBucketAccess(ctx, modes, access)
	require.NoError(t, err)
	fake := d.client.(*fakeAPI)
	assert.Equal(t, 1, fake.createGroupCalls)
	assert.Equal(t, 1, fake.createUserCalls)
	assert.Equal(t, 1, fake.createKeyCalls)
	assert.Equal(t, "trident-access-account", fake.lastGroupShortName)
	require.NotNil(t, fake.lastPolicy)
	require.NotEmpty(t, fake.lastPolicy.Statement)
	assert.Equal(t, []string{"arn:aws:s3:::trident-cred-bucket"}, fake.lastPolicy.Statement[0].Resource)
	require.NotNil(t, secret)
	assert.Equal(t, "AKIA", secret.AccessKeyID)
	assert.Equal(t, "secret", secret.SecretAccessKey)

	require.NoError(t, d.RevokeBucketAccess(ctx, access))
	assert.Equal(t, 1, fake.deleteUserCalls)
	assert.Equal(t, 1, fake.deleteGroupCalls)
}

func TestGrantAccessMissingBucket(t *testing.T) {
	d := newTestDriver(t)

	_, err := d.GrantBucketAccess(
		context.Background(),
		[]object.BucketAccessMode{{BucketName: "no-such-bucket", AccessMode: object.AccessModeReadWrite}},
		&object.BucketAccess{Name: "access-missing"},
	)
	assert.True(t, errors.IsNotFoundError(err))
}

func TestBuildGroupPolicyReadWrite(t *testing.T) {
	policy, err := BuildGroupPolicy([]object.BucketAccessMode{
		{BucketName: "a-bucket", AccessMode: object.AccessModeReadWrite},
	})
	require.NoError(t, err)
	require.Len(t, policy.Statement, 2)
	assert.Equal(t, []string{"arn:aws:s3:::a-bucket"}, policy.Statement[0].Resource)
	assert.Equal(t, []string{"arn:aws:s3:::a-bucket/*"}, policy.Statement[1].Resource)
	assert.Contains(t, policy.Statement[1].Action, "s3:PutObject")
	assert.Contains(t, policy.Statement[1].Action, "s3:GetObject")
}

func newTestDriver(t *testing.T) *ObjectDriver {
	t.Helper()
	fake := newFakeAPI()
	restore := swapClientFactory(fake)
	t.Cleanup(restore)

	store, err := NewObjectDriver(context.Background(), []byte(`{"username":"root","password":"secret"}`), []byte(`{
		"objectStoreName":"sg",
		"managementEndpoint":"https://grid.example",
		"s3Endpoint":"https://s3.grid.example",
		"labels":{"performance":"gold"},
		"accountId":"12345678901234567890"
	}`))
	require.NoError(t, err)
	require.NoError(t, store.Initialize(context.Background()))
	return store.(*ObjectDriver)
}

func swapClientFactory(fake *fakeAPI) func() {
	prev := newTenantClient
	newTenantClient = func(sgapi.Config) (tenantAPI, error) { return fake, nil }
	return func() { newTenantClient = prev }
}

type fakeAPI struct {
	buckets            map[string]string // name -> region, tracked so GetBucket/GetRegion report real values
	groups             map[string]*sgapi.Group
	users              map[string]*sgapi.User
	versionsCalls      int
	authorizeCalls     int
	createGroupCalls   int
	createUserCalls    int
	createKeyCalls     int
	deleteUserCalls    int
	deleteGroupCalls   int
	deleteCalls        int
	lastGroupShortName string
	lastPolicy         *sgapi.S3Policy

	// foreignNames are bucket names that exist on the grid but not on this
	// tenant. Create and Get both report 409: the name is taken elsewhere, and
	// the region read is not a 404.
	foreignNames map[string]struct{}

	// getErr, if set, is returned by GetBucket before the bucket map is consulted.
	getErr error

	// alertOnCreate, if set, is returned by CreateBucket after the bucket is created and
	// tracked, simulating StorageGRID's error-severity alert on an otherwise successful
	// create. Only fires on the create that adds the bucket - like the real API, a retry
	// against an existing name takes the conflict path instead.
	alertOnCreate error

	// notEmptyOnDelete, if set, makes DeleteBucket report an existing bucket as not empty.
	notEmptyOnDelete bool
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		buckets: make(map[string]string),
		groups:  make(map[string]*sgapi.Group),
		users:   make(map[string]*sgapi.User),
	}
}

func (f *fakeAPI) Versions(context.Context) ([]int, error) {
	f.versionsCalls++
	return []int{4}, nil
}

func (f *fakeAPI) Authorize(context.Context) (string, error) {
	f.authorizeCalls++
	return "token", nil
}

func (f *fakeAPI) ReleaseToken(context.Context) error { return nil }

func (f *fakeAPI) CreateBucket(_ context.Context, spec sgapi.ContainerCreate) (*sgapi.CreateBucketResult, error) {
	// The real client validates the name locally before sending anything.
	if err := sgapi.ValidateBucketName(spec.Name); err != nil {
		return nil, err
	}
	if _, ok := f.foreignNames[spec.Name]; ok {
		return nil, &sgapi.APIError{StatusCode: http.StatusConflict, Key: "conflict"}
	}
	if _, ok := f.buckets[spec.Name]; ok {
		return nil, &sgapi.APIError{StatusCode: http.StatusConflict, Key: "conflict"}
	}
	region := spec.Region
	if region == "" {
		region = "us-east-1" // the grid's default when none is requested
	}
	f.buckets[spec.Name] = region
	result := &sgapi.CreateBucketResult{Name: spec.Name, Region: region, RegionResolved: spec.Region != ""}
	if f.alertOnCreate != nil {
		return result, f.alertOnCreate
	}
	return result, nil
}

func (f *fakeAPI) GetBucket(_ context.Context, name string, _ sgapi.BucketReads) (*sgapi.BucketState, error) {
	// The real client rejects a bad name locally, before any request.
	if err := sgapi.ValidateBucketName(name); err != nil {
		return nil, err
	}
	if f.getErr != nil {
		return nil, f.getErr
	}
	if _, foreign := f.foreignNames[name]; foreign {
		return nil, &sgapi.APIError{StatusCode: http.StatusConflict, Key: "error.409"}
	}
	region, ok := f.buckets[name]
	if !ok {
		return nil, &sgapi.APIError{StatusCode: http.StatusNotFound, Key: "notFound"}
	}
	return &sgapi.BucketState{Name: name, Region: region}, nil
}

func (f *fakeAPI) DeleteBucket(_ context.Context, name string) (sgapi.DeleteOutcome, error) {
	f.deleteCalls++
	if _, ok := f.buckets[name]; !ok {
		return sgapi.BucketAlreadyGone, nil
	}
	if f.notEmptyOnDelete {
		return sgapi.BucketNotEmpty, nil
	}
	delete(f.buckets, name)
	return sgapi.BucketDeleted, nil
}

func (f *fakeAPI) GetRegion(_ context.Context, name string) (string, error) {
	region, ok := f.buckets[name]
	if !ok {
		return "", &sgapi.APIError{StatusCode: http.StatusNotFound, Key: "notFound"}
	}
	return region, nil
}

func (f *fakeAPI) CreateGroup(_ context.Context, spec sgapi.GroupCreate) (*sgapi.Group, error) {
	f.createGroupCalls++
	f.lastGroupShortName = spec.UniqueName[len("group/"):]
	if spec.Policies != nil {
		f.lastPolicy = spec.Policies.S3
	}
	group := &sgapi.Group{ID: "group-id", UniqueName: spec.UniqueName, DisplayName: spec.DisplayName}
	f.groups[f.lastGroupShortName] = group
	return group, nil
}

func (f *fakeAPI) GetGroupByShortName(_ context.Context, shortName string) (*sgapi.Group, map[string]any, error) {
	group, ok := f.groups[shortName]
	if !ok {
		return nil, nil, &sgapi.APIError{StatusCode: http.StatusNotFound, Key: "notFound"}
	}
	return group, map[string]any{"id": group.ID}, nil
}

func (f *fakeAPI) SetGroupS3Policy(_ context.Context, shortName string, _ *sgapi.S3Policy) (*sgapi.Group, error) {
	return f.groups[shortName], nil
}

func (f *fakeAPI) DeleteGroup(_ context.Context, id string) error {
	f.deleteGroupCalls++
	for name, group := range f.groups {
		if group.ID == id {
			delete(f.groups, name)
		}
	}
	return nil
}

func (f *fakeAPI) CreateUser(_ context.Context, spec sgapi.UserCreate) (*sgapi.User, error) {
	f.createUserCalls++
	short := spec.UniqueName[len("user/"):]
	user := &sgapi.User{ID: "user-id", UniqueName: spec.UniqueName, MemberOf: spec.MemberOf}
	f.users[short] = user
	return user, nil
}

func (f *fakeAPI) GetUserByShortName(_ context.Context, shortName string) (*sgapi.User, map[string]any, error) {
	user, ok := f.users[shortName]
	if !ok {
		return nil, nil, &sgapi.APIError{StatusCode: http.StatusNotFound, Key: "notFound"}
	}
	return user, map[string]any{"id": user.ID}, nil
}

func (f *fakeAPI) EnsureUserInGroup(_ context.Context, shortName, groupID string) (*sgapi.User, error) {
	user := f.users[shortName]
	user.MemberOf = append(user.MemberOf, groupID)
	return user, nil
}

func (f *fakeAPI) DeleteUser(_ context.Context, id string) error {
	f.deleteUserCalls++
	for name, user := range f.users {
		if user.ID == id {
			delete(f.users, name)
		}
	}
	return nil
}

func (f *fakeAPI) DeleteAllAccessKeys(context.Context, string) (int, error) { return 0, nil }

func (f *fakeAPI) CreateAccessKey(context.Context, string, *string) (*sgapi.AccessKey, error) {
	f.createKeyCalls++
	return &sgapi.AccessKey{AccessKey: "AKIA", SecretAccessKey: "secret"}, nil
}

var _ tenantAPI = (*fakeAPI)(nil)
