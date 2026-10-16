// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ontaps3

import (
	"context"
	"os"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/object"
	"github.com/netapp/trident/utils/errors"
)

// Required environment variables to activate this suite:
//
//	TRIDENT_ONTAP_S3_MGMT_ENDPOINT
//	TRIDENT_ONTAP_S3_USERNAME
//	TRIDENT_ONTAP_S3_PASSWORD
//	TRIDENT_ONTAP_S3_SVM_NAME
const (
	envMgmtEndpoint = "TRIDENT_TEST_ONTAP_S3_MGMT_ENDPOINT"
	envUsername     = "TRIDENT_TEST_ONTAP_S3_USERNAME"
	envPassword     = "TRIDENT_TEST_ONTAP_S3_PASSWORD"
	envSVMName      = "TRIDENT_TEST_ONTAP_S3_SVM_NAME"
)

// testRunID is a short identifier for this test run, derived from the current
// git commit hash (first 8 chars) or a random UUID if VCS info is unavailable.
// It is embedded in all ONTAP resource names so parallel or repeated runs do
// not collide and leftover resources are traceable to a specific build.
var testRunID = func() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 8 {
				return s.Value[:8]
			}
		}
	}
	return uuid.NewString()[:8]
}()

func makeTestResourceID(testName string) string {
	sanitized := strings.ToLower(strings.NewReplacer("/", "-", "_", "-", ".", "-").Replace(testName))
	resourceID := testRunID + "-" + sanitized + "-" + uuid.NewString()
	if len(resourceID) > 64 {
		resourceID = resourceID[:63-len(defaultStoragePrefix)]
	}
	return resourceID
}

// testConfig can be populated directly for local dev runs.
// Leave all fields empty to use env vars instead.
var testConfig = struct {
	mgmtLIF  string
	svmName  string
	username string
	password string
}{
	mgmtLIF:  "", // ONTAP Management IP; Fill in locally; never commit
	svmName:  "", // S3 Enabled SVM; Fill in locally; never commit
	username: "", // ONTAP Admin; Fill in locally; never commit
	password: "", // ONTAP Password; Fill in locally; never commit
}

// newDriver reads credentials from env, skips the calling test if any required var is
// absent, and returns an initialised ObjectDriver ready to use.
func newDriver(t *testing.T) (*ObjectDriver, context.Context) {
	t.Helper()

	get := func(hardcoded, envKey string) string {
		if hardcoded != "" {
			return hardcoded
		}
		return os.Getenv(envKey)
	}

	mgmt := get(testConfig.mgmtLIF, envMgmtEndpoint)
	user := get(testConfig.username, envUsername)
	pass := get(testConfig.password, envPassword)
	svm := get(testConfig.svmName, envSVMName)
	if mgmt == "" || user == "" || pass == "" || svm == "" {
		t.Skipf("integration test skipped: set %s, %s, %s, %s to run",
			envMgmtEndpoint, envUsername, envPassword, envSVMName)
	}

	ctx := context.Background()
	d := &ObjectDriver{
		ObjectStoreName:    "integration-test-store",
		ManagementEndpoint: mgmt,
		SVMName:            svm,
		Credentials:        Credentials{Username: user, Password: pass},
	}
	require.NoError(t, d.Initialize(ctx), "driver Initialize")
	return d, ctx
}

func TestONTAPS3_CreateBucket(t *testing.T) {
	d, ctx := newDriver(t)
	name := makeTestResourceID(t.Name())
	t.Cleanup(func() {
		if err := d.DeleteBucket(ctx, name); err != nil {
			t.Logf("cleanup DeleteBucket %q: %v", name, err)
		}
	})

	bucket := &object.Bucket{Name: name}
	err := d.CreateBucket(ctx, bucket)
	require.NoError(t, err)
	assert.Equal(t, d.UUID, bucket.StoreUUID)

	// Idempotent second call must succeed.
	err = d.CreateBucket(ctx, bucket)
	require.NoError(t, err, "CreateBucket (idempotent)")
}

func TestONTAPS3_DeleteBucket(t *testing.T) {
	d, ctx := newDriver(t)
	name := makeTestResourceID(t.Name())

	bucket := &object.Bucket{Name: name}
	err := d.CreateBucket(ctx, bucket)
	require.NoError(t, err, "setup CreateBucket")
	require.NoError(t, d.DeleteBucket(ctx, name))

	b, err := d.GetBucketByName(ctx, name)
	assert.True(t, errors.IsNotFoundError(err), "expected NotFoundError after delete, got: %v", err)
	assert.Nil(t, b)
	require.NoError(t, d.DeleteBucket(ctx, name), "DeleteBucket not idempotent")
}

func TestONTAPS3_GrantBucketAccess_ReadMode(t *testing.T) {
	d, ctx := newDriver(t)
	bucketName := makeTestResourceID(t.Name())
	accessName := makeTestResourceID(t.Name())

	bucket := &object.Bucket{Name: bucketName}
	err := d.CreateBucket(ctx, bucket)
	require.NoError(t, err, "setup CreateBucket")
	t.Cleanup(func() {
		_ = d.RevokeBucketAccess(ctx, &object.BucketAccess{Name: accessName})
		if err := d.DeleteBucket(ctx, bucketName); err != nil {
			t.Logf("cleanup DeleteBucket: %v", err)
		}
	})

	modes := []object.BucketAccessMode{{BucketName: bucketName, AccessMode: object.AccessModeRead}}
	secret, err := d.GrantBucketAccess(ctx, modes, &object.BucketAccess{Name: accessName})
	require.NoError(t, err)
	require.NotEmpty(t, secret.AccessKeyID)
	require.NotEmpty(t, secret.SecretAccessKey)
}

func TestONTAPS3_GrantBucketAccess_WriteMode(t *testing.T) {
	d, ctx := newDriver(t)
	bucketName := makeTestResourceID(t.Name())
	accessName := makeTestResourceID(t.Name())

	bucket := &object.Bucket{Name: bucketName}
	err := d.CreateBucket(ctx, bucket)
	require.NoError(t, err, "setup CreateBucket")
	t.Cleanup(func() {
		_ = d.RevokeBucketAccess(ctx, &object.BucketAccess{Name: accessName})
		if err := d.DeleteBucket(ctx, bucketName); err != nil {
			t.Logf("cleanup DeleteBucket: %v", err)
		}
	})

	modes := []object.BucketAccessMode{{BucketName: bucketName, AccessMode: object.AccessModeWrite}}
	secret, err := d.GrantBucketAccess(ctx, modes, &object.BucketAccess{Name: accessName})
	require.NoError(t, err)
	require.NotEmpty(t, secret.AccessKeyID)
	require.NotEmpty(t, secret.SecretAccessKey)
}

func TestONTAPS3_GrantBucketAccess(t *testing.T) {
	d, ctx := newDriver(t)
	bucketName := makeTestResourceID(t.Name())
	accessName := makeTestResourceID(t.Name())

	bucket := &object.Bucket{Name: bucketName}
	err := d.CreateBucket(ctx, bucket)
	require.NoError(t, err, "setup CreateBucket")
	t.Cleanup(func() {
		_ = d.RevokeBucketAccess(ctx, &object.BucketAccess{Name: accessName})
		if err := d.DeleteBucket(ctx, bucketName); err != nil {
			t.Logf("cleanup DeleteBucket: %v", err)
		}
	})

	modes := []object.BucketAccessMode{{BucketName: bucketName, AccessMode: object.AccessModeReadWrite}}
	access := &object.BucketAccess{Name: accessName}

	secret, err := d.GrantBucketAccess(ctx, modes, access)
	require.NoError(t, err)
	require.NotEmpty(t, secret.AccessKeyID)
	require.NotEmpty(t, secret.SecretAccessKey)
	assert.Equal(t, d.UUID, access.StoreUUID)

	// Second grant must fail — credentials cannot be re-read from ONTAP.
	_, err = d.GrantBucketAccess(ctx, modes, &object.BucketAccess{Name: accessName})
	assert.Error(t, err, "expected error on duplicate grant")
}

func TestONTAPS3_RevokeBucketAccess(t *testing.T) {
	d, ctx := newDriver(t)
	bucketName := makeTestResourceID(t.Name())
	accessName := makeTestResourceID(t.Name())

	bucket := &object.Bucket{Name: bucketName}
	err := d.CreateBucket(ctx, bucket)
	require.NoError(t, err, "setup CreateBucket")
	t.Cleanup(func() {
		if err := d.DeleteBucket(ctx, bucketName); err != nil {
			t.Logf("cleanup DeleteBucket: %v", err)
		}
	})

	modes := []object.BucketAccessMode{{BucketName: bucketName, AccessMode: object.AccessModeReadWrite}}
	_, err = d.GrantBucketAccess(ctx, modes, &object.BucketAccess{Name: accessName})
	require.NoError(t, err, "setup GrantBucketAccess")
	require.NoError(t, d.RevokeBucketAccess(ctx, &object.BucketAccess{Name: accessName}))
	require.NoError(t, d.RevokeBucketAccess(ctx, &object.BucketAccess{Name: accessName}), "RevokeBucketAccess not idempotent")
}
