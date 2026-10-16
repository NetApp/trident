// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ontaps3

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mock_api "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/object"
	ontapapi "github.com/netapp/trident/storage_drivers/ontap/api"
	tridentErrors "github.com/netapp/trident/utils/errors"
)

func newMockDriver(t *testing.T) (*ObjectDriver, *mock_api.MockS3) {
	t.Helper()
	ctrl := gomock.NewController(t)
	m := mock_api.NewMockS3(ctrl)
	d := &ObjectDriver{
		ObjectStoreName:    "test-store",
		ManagementEndpoint: "10.0.0.1",
		SVMName:            "svm1",
		UUID:               "uuid-1",
		ontap:              m,
		StoredState:        object.StoreStateOnline,
	}
	return d, m
}

func TestS3ActionsByMode_ReadContainsExpectedActions(t *testing.T) {
	actions := modesToActions[object.AccessModeRead]
	require.NotEmpty(t, actions)
	for _, expected := range []string{
		"GetObject",
		"ListBucket",
		"GetBucketVersioning",
		"ListBucketMultipartUploads", "ListMultipartUploadParts",
	} {
		assert.Contains(t, actions, expected, "Read mode missing action %s", expected)
	}
}

func TestS3ActionsByMode_ReadExcludesMutatingActions(t *testing.T) {
	actions := modesToActions[object.AccessModeRead]
	for _, forbidden := range []string{
		"PutObject", "DeleteObject", "*",
		"HeadObject", "HeadBucket", // not valid at SVM policy layer
	} {
		assert.NotContains(t, actions, forbidden, "Read mode must not contain %q", forbidden)
	}
}

func TestS3ActionsByMode_WriteContainsExpectedActions(t *testing.T) {
	actions := modesToActions[object.AccessModeWrite]
	require.NotEmpty(t, actions)
	for _, expected := range []string{
		"PutObject", "DeleteObject",
		"ListBucketMultipartUploads", "ListMultipartUploadParts",
	} {
		assert.Contains(t, actions, expected, "Write mode missing action %q", expected)
	}
}

func TestS3ActionsByMode_ReadWriteIsWildcard(t *testing.T) {
	actions := modesToActions[object.AccessModeReadWrite]
	require.Len(t, actions, 1, "ReadWrite should be exactly [\"*\"]")
	assert.Equal(t, "*", actions[0])
}

func TestS3ActionsByMode_AllModesPresent(t *testing.T) {
	for _, mode := range []object.AccessMode{
		object.AccessModeRead,
		object.AccessModeWrite,
		object.AccessModeReadWrite,
	} {
		_, ok := modesToActions[mode]
		assert.True(t, ok, "missing entry for mode %q", mode)
	}
}

func TestS3ActionsByMode_NoInvalidMultipartActions(t *testing.T) {
	// These are invalid at the SVM policy layer.
	invalid := []string{"AbortMultipartUpload", "CreateMultipartUpload", "CompleteMultipartUpload"}
	for mode, actions := range modesToActions {
		if mode == object.AccessModeReadWrite {
			continue // "*" covers everything intentionally
		}
		for _, a := range invalid {
			assert.NotContains(t, actions, a, "mode %q must not contain SVM-invalid action %q", mode, a)
		}
	}
}

func TestNewObjectDriver_Valid(t *testing.T) {
	cfg := `{"objectStoreName":"s","managementEndpoint":"10.0.0.1","svmName":"svm1"}`
	d, err := NewObjectDriver(context.Background(), nil, []byte(cfg))
	require.NoError(t, err)
	assert.NotNil(t, d)
}

func TestNewObjectDriver_MissingObjectStoreName(t *testing.T) {
	cfg := `{"managementEndpoint":"10.0.0.1","svmName":"svm1"}`
	_, err := NewObjectDriver(context.Background(), nil, []byte(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "objectStoreName")
}

func TestNewObjectDriver_MissingManagementEndpoint(t *testing.T) {
	cfg := `{"objectStoreName":"s","svmName":"svm1"}`
	_, err := NewObjectDriver(context.Background(), nil, []byte(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "managementEndpoint")
}

func TestNewObjectDriver_MissingSVMName(t *testing.T) {
	cfg := `{"objectStoreName":"s","managementEndpoint":"10.0.0.1"}`
	_, err := NewObjectDriver(context.Background(), nil, []byte(cfg))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "svmName")
}

func TestNewObjectDriver_EmptyConfig(t *testing.T) {
	_, err := NewObjectDriver(context.Background(), nil)
	require.Error(t, err)
}

func TestNewObjectDriver_SecretConfigAppliesCredentials(t *testing.T) {
	cfg := `{"objectStoreName":"s","managementEndpoint":"10.0.0.1","svmName":"svm1"}`
	secretCfg := `{"username":"admin","password":"secret"}`
	store, err := NewObjectDriver(context.Background(), []byte(secretCfg), []byte(cfg))
	require.NoError(t, err)
	d, ok := store.(*ObjectDriver)
	require.True(t, ok)
	assert.Equal(t, "admin", d.Username)
	assert.Equal(t, "secret", d.Password)
}

func TestNewObjectDriver_LaterConfigWins(t *testing.T) {
	cfg1 := `{"objectStoreName":"s","managementEndpoint":"10.0.0.1","svmName":"svm1"}`
	cfg2 := `{"objectStoreName":"s","managementEndpoint":"10.0.0.2","svmName":"svm1"}`
	store, err := NewObjectDriver(context.Background(), nil, []byte(cfg1), []byte(cfg2))
	require.NoError(t, err)
	d, ok := store.(*ObjectDriver)
	require.True(t, ok)
	assert.Equal(t, "10.0.0.2", d.ManagementEndpoint, "later configs must win over earlier ones")
}

func TestDriverIsRegistered(t *testing.T) {
	d, _ := newMockDriver(t)
	assert.Equal(t, "test-store", d.Name())
	assert.Equal(t, DriverName, d.DriverName())
	assert.Equal(t, "uuid-1", d.StoreUUID())
	assert.Equal(t, "test-store", d.GetUniqueKey())
	assert.Equal(t, object.StoreStateOnline, d.State())
}

func TestSetState(t *testing.T) {
	d, _ := newMockDriver(t)
	d.SetState(object.StoreStateDeleting)
	assert.Equal(t, object.StoreStateDeleting, d.State())
}

func TestSmartCopy(t *testing.T) {
	d, _ := newMockDriver(t)
	copy := d.SmartCopy()
	assert.NotNil(t, copy)
	assert.IsType(t, &ObjectDriver{}, copy)
	assert.NotSame(t, d, copy)
}

func TestSecretsOmittedFromJSON(t *testing.T) {
	d := &ObjectDriver{
		ObjectStoreName:    "store",
		ManagementEndpoint: "10.0.0.1",
		SVMName:            "svm1",
		Credentials:        Credentials{Username: "admin", Password: "secret"},
	}
	data, err := json.Marshal(d)
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &m))
	assert.NotContains(t, m, "username", "username must not appear in marshalled JSON")
	assert.NotContains(t, m, "password", "password must not appear in marshalled JSON")
}

func TestUnmarshalJSON_RoundTrip(t *testing.T) {
	prefix := "myprefix"
	d := &ObjectDriver{
		ObjectStoreName:    "store",
		ManagementEndpoint: "10.0.0.1",
		SVMName:            "svm1",
		Credentials:        Credentials{Username: "admin", Password: "hunter2"},
		StoragePrefix:      &prefix,
	}
	data, err := json.Marshal(d)
	require.NoError(t, err)

	// Credentials are intentionally never part of this JSON document - even if a caller
	// injects username/password keys (e.g. a malformed CR spec), UnmarshalJSON must not
	// pick them up. Only a separate secretConfig document (see NewObjectDriver) does that.
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))
	raw["username"] = "admin"
	raw["password"] = "hunter2"
	data2, _ := json.Marshal(raw)

	d2 := &ObjectDriver{}
	require.NoError(t, json.Unmarshal(data2, d2))
	assert.Equal(t, d.ObjectStoreName, d2.ObjectStoreName)
	assert.Equal(t, d.ManagementEndpoint, d2.ManagementEndpoint)
	assert.Equal(t, d.SVMName, d2.SVMName)
	assert.Empty(t, d2.Username, "UnmarshalJSON must not pick up credentials from the main config document")
	assert.Empty(t, d2.Password, "UnmarshalJSON must not pick up credentials from the main config document")
	require.NotNil(t, d2.StoragePrefix)
	assert.Equal(t, prefix, *d2.StoragePrefix)

	// Credentials round-trip separately, the way NewObjectDriver applies secretConfig.
	secretData, err := json.Marshal(d.Credentials)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(secretData, &d2.Credentials))
	assert.Equal(t, "admin", d2.Username)
	assert.Equal(t, "hunter2", d2.Password)
}

func TestSecrets(t *testing.T) {
	d := &ObjectDriver{Credentials: Credentials{Username: "u", Password: "p"}}
	s := d.Secrets()
	data, err := json.Marshal(s)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"username":"u"`)
	assert.Contains(t, string(data), `"password":"p"`)
}

// storagePrefix

func TestStoragePrefix_Default(t *testing.T) {
	d := &ObjectDriver{}
	assert.Equal(t, defaultStoragePrefix, d.storagePrefix())
}

func TestStoragePrefix_Custom(t *testing.T) {
	p := "myorg"
	d := &ObjectDriver{StoragePrefix: &p}
	assert.Equal(t, "myorg", d.storagePrefix())
}

func TestStoragePrefix_EmptyStringFallsBackToDefault(t *testing.T) {
	p := ""
	d := &ObjectDriver{StoragePrefix: &p}
	assert.Equal(t, defaultStoragePrefix, d.storagePrefix())
}

func TestMakeAccessID(t *testing.T) {
	// No trailing separator — dash is injected.
	name, err := makeInternalID("trident", "myaccess")
	require.NoError(t, err)
	assert.Equal(t, "trident-myaccess", name)
}

func TestMakeAccessID_TrailingDashPrefix(t *testing.T) {
	// Trailing dash — no extra dash added.
	name, err := makeInternalID("trident-", "myaccess")
	require.NoError(t, err)
	assert.Equal(t, "trident-myaccess", name)
}

func TestMakeAccessID_TrailingUnderscorePrefix(t *testing.T) {
	// Trailing underscore — no dash added (like NAS trident_ prefix convention).
	name, err := makeInternalID("trident_", "myaccess")
	require.NoError(t, err)
	assert.Equal(t, "trident_myaccess", name)
}

func TestMakeAccessID_CustomPrefix(t *testing.T) {
	name, err := makeInternalID("myorg", "myaccess")
	require.NoError(t, err)
	assert.Equal(t, "myorg-myaccess", name)
}

func TestMakeAccessID_UnderscoreInPrefix(t *testing.T) {
	name, err := makeInternalID("my_org", "myaccess")
	require.NoError(t, err)
	assert.Equal(t, "my_org-myaccess", name)
}

func TestMakeAccessID_EmptySuffixGeneratesUUID(t *testing.T) {
	name, err := makeInternalID("trident", "")
	require.NoError(t, err)
	assert.True(t, len(name) > len("trident-"), "should have generated a suffix")
}

func TestMakeAccessID_TooLong(t *testing.T) {
	// 57 'a's + "trident-" (8) = 65 chars — over limit
	_, err := makeInternalID("trident", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too long")
}

func TestMakeAccessID_MaxValid(t *testing.T) {
	// "trident-" = 8 chars; 64 - 8 = 56 chars max suffix
	suffix := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 56 chars
	name, err := makeInternalID("trident", suffix)
	require.NoError(t, err)
	assert.Equal(t, "trident-"+suffix, name)
}

// checkBucketName

func TestCheckBucketName_Valid(t *testing.T) {
	for _, name := range []string{"my-bucket", "bucket123", "a.b.c", "abc"} {
		assert.NoError(t, validateName(name), "expected valid: %s", name)
	}
}

func TestCheckBucketName_TooLong(t *testing.T) {
	require.Error(t, validateName(string(make([]byte, 64))))
}

func TestCheckBucketName_InvalidChars(t *testing.T) {
	for _, name := range []string{"MyBucket", "bucket_name", "bucket name", "bucket/path"} {
		assert.Error(t, validateName(name), "expected invalid: %s", name)
	}
}

// mapBucketsByModes

func TestMapBucketsByModes_GroupsMultipleBucketsUnderSameMode(t *testing.T) {
	d, _ := newMockDriver(t)
	prefix := d.storagePrefix()
	wantA, _ := makeInternalID(prefix, "a")
	wantB, _ := makeInternalID(prefix, "b")
	wantC, _ := makeInternalID(prefix, "c")
	modes := []object.BucketAccessMode{
		{AccessMode: object.AccessModeReadWrite, BucketName: "a"},
		{AccessMode: object.AccessModeReadWrite, BucketName: "b"},
		{AccessMode: object.AccessModeReadWrite, BucketName: "c"},
	}
	got, err := mapBucketsByModes(prefix, modes)
	require.NoError(t, err)
	require.Len(t, got, 1, "multiple buckets sharing a mode must collapse into one map entry")
	assert.ElementsMatch(t, []string{wantA, wantB, wantC}, got[object.AccessModeReadWrite])
}

func TestMapBucketsByModes_SeparatesDistinctModes(t *testing.T) {
	d, _ := newMockDriver(t)
	prefix := d.storagePrefix()
	wantA, _ := makeInternalID(prefix, "a")
	wantB, _ := makeInternalID(prefix, "b")
	modes := []object.BucketAccessMode{
		{AccessMode: object.AccessModeRead, BucketName: "a"},
		{AccessMode: object.AccessModeWrite, BucketName: "b"},
	}
	got, err := mapBucketsByModes(prefix, modes)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, []string{wantA}, got[object.AccessModeRead])
	assert.Equal(t, []string{wantB}, got[object.AccessModeWrite])
}

func TestMapBucketsByModes_TranslatesNamesToInternalIDs(t *testing.T) {
	// The K8s bucket name "bc-abc" with prefix "myorg-" must become "myorg-bc-abc" in the
	// policy resource list — matching the ONTAP resource created by CreateBucket.
	modes := []object.BucketAccessMode{
		{AccessMode: object.AccessModeReadWrite, BucketName: "bc-abc"},
	}
	got, err := mapBucketsByModes("myorg-", modes)
	require.NoError(t, err)
	assert.Equal(t, []string{"myorg-bc-abc"}, got[object.AccessModeReadWrite])
}

func TestMapBucketsByModes_EmptyAccessMode(t *testing.T) {
	_, err := mapBucketsByModes("test-", []object.BucketAccessMode{{AccessMode: "", BucketName: "a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty access mode")
}

func TestMapBucketsByModes_EmptyBucketName(t *testing.T) {
	_, err := mapBucketsByModes("test-", []object.BucketAccessMode{{AccessMode: object.AccessModeRead, BucketName: ""}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty bucket name")
}

func TestMapBucketsByModes_NoModes(t *testing.T) {
	_, err := mapBucketsByModes("test-", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no valid bucket access modes")
}

// makePolicyStatements

func TestMakePolicyStatements_ReadWrite(t *testing.T) {
	modesToBuckets := map[object.AccessMode][]string{object.AccessModeReadWrite: {"my-bucket"}}
	stmts, err := makePolicyStatements(modesToBuckets, modesToActions)
	require.NoError(t, err)
	require.Len(t, stmts, 1)
	assert.Equal(t, "allow", stmts[0].Effect)
	assert.Contains(t, stmts[0].Resources, "my-bucket")
	assert.Contains(t, stmts[0].Resources, "my-bucket/*")
}

func TestMakePolicyStatements_ReadAndWrite(t *testing.T) {
	modesToBuckets := map[object.AccessMode][]string{
		object.AccessModeRead:  {"b"},
		object.AccessModeWrite: {"b"},
	}
	stmts, err := makePolicyStatements(modesToBuckets, modesToActions)
	require.NoError(t, err)
	assert.Len(t, stmts, 2)
}

func TestMakePolicyStatements_MultipleBucketsSameMode(t *testing.T) {
	modesToBuckets := map[object.AccessMode][]string{object.AccessModeReadWrite: {"a", "b", "c"}}
	stmts, err := makePolicyStatements(modesToBuckets, modesToActions)
	require.NoError(t, err)
	require.Len(t, stmts, 1, "all buckets sharing a mode must collapse into one statement")
	for _, bucket := range []string{"a", "b", "c"} {
		assert.Contains(t, stmts[0].Resources, bucket)
		assert.Contains(t, stmts[0].Resources, bucket+"/*")
	}
}

func TestMakePolicyStatements_InvalidMode(t *testing.T) {
	modesToBuckets := map[object.AccessMode][]string{"bogus": {"b"}}
	_, err := makePolicyStatements(modesToBuckets, modesToActions)
	require.Error(t, err)
}

func TestMakePolicyStatements_NoBucketModes(t *testing.T) {
	_, err := makePolicyStatements(nil, modesToActions)
	require.Error(t, err)
}

func TestMakePolicyStatements_NoActions(t *testing.T) {
	modesToBuckets := map[object.AccessMode][]string{object.AccessModeRead: {"b"}}
	_, err := makePolicyStatements(modesToBuckets, nil)
	require.Error(t, err)
}

func TestInitialize_AlreadyInitialized(t *testing.T) {
	d, _ := newMockDriver(t)
	// ontap is already set; Initialize should be a no-op.
	err := d.Initialize(context.Background())
	assert.NoError(t, err)
}

func TestInitialize_ServiceDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock_api.NewMockS3(ctrl)

	// Override newAPI to return our mock.
	origNewAPI := newAPI
	t.Cleanup(func() { newAPI = origNewAPI })
	newAPI = func(_ context.Context, _, _, _, _ string) (ontapapi.S3, error) { return m, nil }

	m.EXPECT().ServiceGet(gomock.Any(), "svm1").Return(&ontapapi.Service{Enabled: false}, nil)

	d := &ObjectDriver{
		ObjectStoreName:    "store",
		ManagementEndpoint: "10.0.0.1",
		SVMName:            "svm1",
	}
	err := d.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

func TestInitialize_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock_api.NewMockS3(ctrl)

	origNewAPI := newAPI
	t.Cleanup(func() { newAPI = origNewAPI })
	newAPI = func(_ context.Context, _, _, _, _ string) (ontapapi.S3, error) { return m, nil }

	m.EXPECT().ServiceGet(gomock.Any(), "svm1").Return(&ontapapi.Service{
		SVMUUID: "svm-uuid",
		Name:    "s3.svm1",
		Enabled: true,
	}, nil)

	d := &ObjectDriver{
		ObjectStoreName:    "store",
		ManagementEndpoint: "10.0.0.1",
		SVMName:            "svm1",
	}
	require.NoError(t, d.Initialize(context.Background()))
	assert.Equal(t, "svm-uuid", d.SVMUUID)
	assert.Equal(t, "https://s3.svm1", d.S3Endpoint)
	assert.NotEmpty(t, d.UUID)
}

func TestInitialize_StoragePrefixValidation(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := mock_api.NewMockS3(ctrl)
	origNewAPI := newAPI
	t.Cleanup(func() { newAPI = origNewAPI })
	newAPI = func(_ context.Context, _, _, _, _ string) (ontapapi.S3, error) { return m, nil }
	// Valid prefixes reach newAPI/ServiceGet; stub it so the test stays hermetic
	// and doesn't attempt a real network call.
	m.EXPECT().ServiceGet(gomock.Any(), "svm1").Return(&ontapapi.Service{
		SVMUUID: "svm-uuid", Name: "s3.svm1", Enabled: true,
	}, nil).AnyTimes()

	tests := []struct {
		prefix  string
		wantErr bool
	}{
		// valid
		{"trident", false},
		{"a", false},
		{"Z", false},
		{"my-org", false},
		{"my-org-2024", false},
		{"abc123", false},
		{"A1-B2", false},
		{"trident-", false}, // trailing dash — valid, treated as separator
		// invalid — must start with alphanumeric, not a dash
		{"-trident", true},
		// invalid — consecutive dashes produce confusing ONTAP resource names
		{"my--org", true},
		// invalid — underscore not in the allowed character set
		{"trident_", true},
		{"my_org", true},
		{"_private", true},
		// invalid — disallowed characters
		{"trident prefix", true},
		{"trident.prefix", true},
		{"trident+prefix", true},
		{"trident@org", true},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			p := tt.prefix
			d := &ObjectDriver{
				ObjectStoreName:    "store",
				ManagementEndpoint: "10.0.0.1",
				SVMName:            "svm1",
				StoragePrefix:      &p,
			}
			err := d.Initialize(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "storagePrefix")
			} else {
				// Valid prefix passes the regex; subsequent newAPI call will fail in unit
				// tests (no mock), but the prefix-validation error must not be returned.
				if err != nil {
					assert.NotContains(t, err.Error(), "storagePrefix")
				}
			}
		})
	}
}

func TestTerminate(t *testing.T) {
	d, _ := newMockDriver(t)
	err := d.Terminate(context.Background())
	require.NoError(t, err)
	assert.Nil(t, d.ontap)
}

func TestGetBucketByName_NotInitialized(t *testing.T) {
	d := &ObjectDriver{}
	_, err := d.GetBucketByName(context.Background(), "b")
	require.Error(t, err)
}

func TestGetBucketByName_NotFound(t *testing.T) {
	d, m := newMockDriver(t)
	ontapName, err := makeInternalID(d.storagePrefix(), "b")
	require.NoError(t, err)
	m.EXPECT().BucketGetByName(gomock.Any(), ontapName).Return(nil, nil)
	_, err = d.GetBucketByName(context.Background(), "b")
	require.Error(t, err)
	assert.True(t, tridentErrors.IsNotFoundError(err))
}

func TestGetBucketByName_Found(t *testing.T) {
	d, m := newMockDriver(t)
	ontapName, err := makeInternalID(d.storagePrefix(), "b")
	require.NoError(t, err)
	m.EXPECT().BucketGetByName(gomock.Any(), ontapName).Return(&ontapapi.Bucket{Name: ontapName, UUID: "uuid-b"}, nil)
	bucket, err := d.GetBucketByName(context.Background(), "b")
	require.NoError(t, err)
	require.NotNil(t, bucket)
	assert.Equal(t, "b", bucket.Name)
	assert.Equal(t, ontapName, bucket.InternalID)
	assert.Equal(t, d.UUID, bucket.StoreUUID)
}

func TestCreateBucket_NotInitialized(t *testing.T) {
	d := &ObjectDriver{}
	err := d.CreateBucket(context.Background(), &object.Bucket{Name: "b"})
	require.Error(t, err)
}

func TestCreateBucket_InvalidName(t *testing.T) {
	d, _ := newMockDriver(t)
	err := d.CreateBucket(context.Background(), &object.Bucket{Name: "UPPERCASE"})
	require.Error(t, err)
}

func TestCreateBucket_AlreadyExists(t *testing.T) {
	d, m := newMockDriver(t)
	ontapName, err := makeInternalID(d.storagePrefix(), "b")
	require.NoError(t, err)
	m.EXPECT().BucketGetByName(gomock.Any(), ontapName).Return(&ontapapi.Bucket{Name: ontapName}, nil)
	err = d.CreateBucket(context.Background(), &object.Bucket{Name: "b"})
	require.NoError(t, err)
}

func TestCreateBucket_Created(t *testing.T) {
	d, m := newMockDriver(t)
	d.S3Endpoint = "https://s3.svm1"
	d.SVMUUID = "svm-uuid"
	ontapName, err := makeInternalID(d.storagePrefix(), "b")
	require.NoError(t, err)
	m.EXPECT().BucketGetByName(gomock.Any(), ontapName).Return(nil, nil)
	m.EXPECT().BucketCreate(gomock.Any(), ontapapi.BucketCreateConfig{Name: ontapName}).Return(&ontapapi.Bucket{Name: ontapName}, nil)
	bucket := &object.Bucket{Name: "b"}
	require.NoError(t, d.CreateBucket(context.Background(), bucket))
	assert.Equal(t, "b", bucket.Name)
	assert.Equal(t, ontapName, bucket.InternalID)
	assert.Equal(t, d.UUID, bucket.StoreUUID)
	require.NotNil(t, bucket.Info.S3)
	assert.Equal(t, "https://s3.svm1", bucket.Info.S3.Endpoint)
	assert.Equal(t, "svm-uuid", bucket.Info.S3.Region)
	assert.Equal(t, "Path", bucket.Info.S3.AddressingStyle)
}

func TestDeleteBucket_NotInitialized(t *testing.T) {
	d := &ObjectDriver{}
	err := d.DeleteBucket(context.Background(), "b")
	require.Error(t, err)
}

func TestDeleteBucket_NotFound(t *testing.T) {
	d, m := newMockDriver(t)
	ontapName, err := makeInternalID(d.storagePrefix(), "b")
	require.NoError(t, err)
	m.EXPECT().BucketGetByName(gomock.Any(), ontapName).Return(nil, nil)
	assert.NoError(t, d.DeleteBucket(context.Background(), "b"))
}

func TestDeleteBucket_Success(t *testing.T) {
	d, m := newMockDriver(t)
	ontapName, err := makeInternalID(d.storagePrefix(), "b")
	require.NoError(t, err)
	m.EXPECT().BucketGetByName(gomock.Any(), ontapName).Return(&ontapapi.Bucket{Name: ontapName, UUID: "uuid-b"}, nil)
	m.EXPECT().BucketDelete(gomock.Any(), "uuid-b").Return(nil)
	assert.NoError(t, d.DeleteBucket(context.Background(), "b"))
}

func TestGrantBucketAccess_NotInitialized(t *testing.T) {
	d := &ObjectDriver{}
	_, err := d.GrantBucketAccess(context.Background(), nil, &object.BucketAccess{Name: "a"})
	require.Error(t, err)
}

func TestGrantBucketAccess_NoModes(t *testing.T) {
	d, _ := newMockDriver(t)
	_, err := d.GrantBucketAccess(context.Background(), nil, &object.BucketAccess{Name: "a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mode")
}

func TestGrantBucketAccess_GroupAlreadyExists(t *testing.T) {
	d, m := newMockDriver(t)
	name, err := makeInternalID(d.storagePrefix(), "a")
	require.NoError(t, err)
	m.EXPECT().GroupGet(gomock.Any(), name).Return(&ontapapi.Group{Name: name}, nil)
	modes := []object.BucketAccessMode{{AccessMode: object.AccessModeReadWrite, BucketName: "b"}}
	_, err = d.GrantBucketAccess(context.Background(), modes, &object.BucketAccess{Name: "a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoke bucket access a before re-granting")
}

func TestGrantBucketAccess_Success(t *testing.T) {
	d, m := newMockDriver(t)
	name, err := makeInternalID(d.storagePrefix(), "a")
	require.NoError(t, err)

	m.EXPECT().GroupGet(gomock.Any(), name).Return(nil, nil)
	m.EXPECT().PolicyGet(gomock.Any(), name).Return(nil, nil)
	m.EXPECT().PolicyCreate(gomock.Any(), name, gomock.Any()).Return(nil)
	m.EXPECT().PolicyGet(gomock.Any(), name).Return(&ontapapi.SVMPolicy{Name: name}, nil)
	m.EXPECT().UserGet(gomock.Any(), name).Return(nil, nil)
	m.EXPECT().UserCreate(gomock.Any(), name).Return(&ontapapi.User{
		Name: name, AccessKey: "AK", SecretKey: "SK",
	}, nil)
	m.EXPECT().GroupCreate(gomock.Any(), name, []string{name}, []string{name}).Return(nil)
	m.EXPECT().GroupGet(gomock.Any(), name).Return(&ontapapi.Group{Name: name}, nil)

	modes := []object.BucketAccessMode{{AccessMode: object.AccessModeReadWrite, BucketName: "b"}}
	secret, err := d.GrantBucketAccess(context.Background(), modes, &object.BucketAccess{Name: "a"})
	require.NoError(t, err)
	assert.Equal(t, "AK", secret.AccessKeyID)
	assert.Equal(t, "SK", secret.SecretAccessKey)
}

// TestGrantBucketAccess_PolicyAlreadyExists simulates a retry after a prior grant
// failed after PolicyCreate succeeded but before the group was created. PolicyCreate
// must not be called again, or ONTAP would return a conflict and the grant could
// never complete.
func TestGrantBucketAccess_PolicyAlreadyExists(t *testing.T) {
	d, m := newMockDriver(t)
	name, err := makeInternalID(d.storagePrefix(), "a")
	require.NoError(t, err)

	m.EXPECT().GroupGet(gomock.Any(), name).Return(nil, nil)
	m.EXPECT().PolicyGet(gomock.Any(), name).Return(&ontapapi.SVMPolicy{Name: name}, nil)
	m.EXPECT().UserGet(gomock.Any(), name).Return(nil, nil)
	m.EXPECT().UserCreate(gomock.Any(), name).Return(&ontapapi.User{
		Name: name, AccessKey: "AK", SecretKey: "SK",
	}, nil)
	m.EXPECT().GroupCreate(gomock.Any(), name, []string{name}, []string{name}).Return(nil)
	m.EXPECT().GroupGet(gomock.Any(), name).Return(&ontapapi.Group{Name: name}, nil)

	modes := []object.BucketAccessMode{{AccessMode: object.AccessModeReadWrite, BucketName: "b"}}
	secret, err := d.GrantBucketAccess(context.Background(), modes, &object.BucketAccess{Name: "a"})
	require.NoError(t, err)
	assert.Equal(t, "AK", secret.AccessKeyID)
}

func TestGrantBucketAccess_StaleUserDeletedAndRecreated(t *testing.T) {
	d, m := newMockDriver(t)
	name, err := makeInternalID(d.storagePrefix(), "a")
	require.NoError(t, err)

	m.EXPECT().GroupGet(gomock.Any(), name).Return(nil, nil)
	m.EXPECT().PolicyGet(gomock.Any(), name).Return(nil, nil)
	m.EXPECT().PolicyCreate(gomock.Any(), name, gomock.Any()).Return(nil)
	m.EXPECT().PolicyGet(gomock.Any(), name).Return(&ontapapi.SVMPolicy{Name: name}, nil)
	m.EXPECT().UserGet(gomock.Any(), name).Return(&ontapapi.User{Name: name}, nil)
	m.EXPECT().UserDelete(gomock.Any(), name).Return(nil)
	m.EXPECT().UserCreate(gomock.Any(), name).Return(&ontapapi.User{
		Name: name, AccessKey: "AK2", SecretKey: "SK2",
	}, nil)
	m.EXPECT().GroupCreate(gomock.Any(), name, []string{name}, []string{name}).Return(nil)
	m.EXPECT().GroupGet(gomock.Any(), name).Return(&ontapapi.Group{Name: name}, nil)

	modes := []object.BucketAccessMode{{AccessMode: object.AccessModeReadWrite, BucketName: "b"}}
	secret, err := d.GrantBucketAccess(context.Background(), modes, &object.BucketAccess{Name: "a"})
	require.NoError(t, err)
	assert.Equal(t, "AK2", secret.AccessKeyID)
}

func TestRevokeBucketAccess_NotInitialized(t *testing.T) {
	d := &ObjectDriver{}
	err := d.RevokeBucketAccess(context.Background(), &object.BucketAccess{Name: "a"})
	require.Error(t, err)
}

func TestRevokeBucketAccess_Success(t *testing.T) {
	d, m := newMockDriver(t)
	name, err := makeInternalID(d.storagePrefix(), "a")
	require.NoError(t, err)

	gomock.InOrder(
		m.EXPECT().GroupDelete(gomock.Any(), name).Return(nil),
		m.EXPECT().UserDelete(gomock.Any(), name).Return(nil),
		m.EXPECT().PolicyDelete(gomock.Any(), name).Return(nil),
	)

	err = d.RevokeBucketAccess(context.Background(), &object.BucketAccess{Name: "a"})
	require.NoError(t, err)
}

func TestRevokeBucketAccess_IdempotentOn404(t *testing.T) {
	d, m := newMockDriver(t)
	name, err := makeInternalID(d.storagePrefix(), "a")
	require.NoError(t, err)

	// All deletes return nil (404 is swallowed at the API layer).
	gomock.InOrder(
		m.EXPECT().GroupDelete(gomock.Any(), name).Return(nil),
		m.EXPECT().UserDelete(gomock.Any(), name).Return(nil),
		m.EXPECT().PolicyDelete(gomock.Any(), name).Return(nil),
	)

	err = d.RevokeBucketAccess(context.Background(), &object.BucketAccess{Name: "a"})
	require.NoError(t, err)
}
