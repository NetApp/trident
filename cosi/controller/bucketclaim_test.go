/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconciler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"

	cosierr "github.com/netapp/trident/cosi/third_party/errors"
	cositest "github.com/netapp/trident/cosi/third_party/test"
)

func Test_determineBucketName(t *testing.T) {
	baseClaim := cosiapi.BucketClaim{
		ObjectMeta: meta.ObjectMeta{
			Name:      "test-bucket",
			Namespace: "user-ns",
			UID:       types.UID("qwerty"), // not realistic but good enough for tests
		},
		Spec:   cosiapi.BucketClaimSpec{},
		Status: cosiapi.BucketClaimStatus{},
	}

	t.Run("dynamic first provision", func(t *testing.T) {
		claim := baseClaim.DeepCopy()
		claim.Spec.BucketClassName = "some-class"

		n, err := determineBucketName(claim)
		assert.NoError(t, err)
		assert.Equal(t, "bc-qwerty", n)
	})

	t.Run("dynamic subsequent provision", func(t *testing.T) {
		claim := baseClaim.DeepCopy()
		claim.Spec.BucketClassName = "some-class"
		claim.Status.BoundBucketName = "bc-qwerty"

		n, err := determineBucketName(claim)
		assert.NoError(t, err)
		assert.Equal(t, "bc-qwerty", n)
	})

	t.Run("dynamic degraded provision", func(t *testing.T) {
		claim := baseClaim.DeepCopy()
		claim.Spec.BucketClassName = "some-class"
		claim.Status.BoundBucketName = "deliberately-unique"

		n, err := determineBucketName(claim)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "unrecoverable degradation")
		assert.ErrorContains(t, err, "bc-qwerty")
		assert.ErrorContains(t, err, "deliberately-unique")
		assert.Empty(t, n)
	})

	t.Run("static first provision", func(t *testing.T) {
		claim := baseClaim.DeepCopy()
		claim.Spec.ExistingBucketName = "admin-created-bucket"

		n, err := determineBucketName(claim)
		assert.NoError(t, err)
		assert.Equal(t, "admin-created-bucket", n)
	})

	t.Run("static subsequent provision", func(t *testing.T) {
		claim := baseClaim.DeepCopy()
		claim.Spec.ExistingBucketName = "admin-created-bucket"
		claim.Status.BoundBucketName = "admin-created-bucket"

		n, err := determineBucketName(claim)
		assert.NoError(t, err)
		assert.Equal(t, "admin-created-bucket", n)
	})

	t.Run("static degraded provision", func(t *testing.T) {
		claim := baseClaim.DeepCopy()
		claim.Spec.ExistingBucketName = "admin-created-bucket"
		claim.Status.BoundBucketName = "deliberately-unique"

		n, err := determineBucketName(claim)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "unrecoverable degradation")
		assert.ErrorContains(t, err, "admin-created-bucket")
		assert.ErrorContains(t, err, "deliberately-unique")
		assert.Empty(t, n)
	})
}

func Test_mapBucketToBucketClaim(t *testing.T) {
	t.Run("bucket references a claim", func(t *testing.T) {
		bucket := &cosiapi.Bucket{
			ObjectMeta: meta.ObjectMeta{Name: "admin-bucket"},
			Spec: cosiapi.BucketSpec{
				BucketClaimRef: cosiapi.BucketClaimReference{Name: "static-claim", Namespace: "user-ns"},
			},
		}

		requests := mapBucketToBucketClaim(context.Background(), bucket)

		assert.Equal(t, []reconcile.Request{
			{NamespacedName: types.NamespacedName{Name: "static-claim", Namespace: "user-ns"}},
		}, requests)
	})

	t.Run("bucketClaimRef name is empty", func(t *testing.T) {
		bucket := &cosiapi.Bucket{
			ObjectMeta: meta.ObjectMeta{Name: "admin-bucket"},
			Spec: cosiapi.BucketSpec{
				BucketClaimRef: cosiapi.BucketClaimReference{Namespace: "user-ns"},
			},
		}

		assert.Empty(t, mapBucketToBucketClaim(context.Background(), bucket))
	})

	t.Run("bucketClaimRef namespace is empty", func(t *testing.T) {
		bucket := &cosiapi.Bucket{
			ObjectMeta: meta.ObjectMeta{Name: "admin-bucket"},
			Spec: cosiapi.BucketSpec{
				BucketClaimRef: cosiapi.BucketClaimReference{Name: "static-claim"},
			},
		}

		assert.Empty(t, mapBucketToBucketClaim(context.Background(), bucket))
	})

	t.Run("non-Bucket object is ignored", func(t *testing.T) {
		claim := &cosiapi.BucketClaim{ObjectMeta: meta.ObjectMeta{Name: "static-claim", Namespace: "user-ns"}}

		assert.Empty(t, mapBucketToBucketClaim(context.Background(), claim))
	})
}

func Test_createIntermediateBucket(t *testing.T) {
	// valid base claim used for subtests
	baseClaim := cosiapi.BucketClaim{
		ObjectMeta: meta.ObjectMeta{
			Name:      "my-bucket",
			Namespace: "my-ns",
			UID:       "qwerty",
		},
		Spec: cosiapi.BucketClaimSpec{
			BucketClassName: "s3-class",
			Protocols: []cosiapi.ObjectProtocol{
				cosiapi.ObjectProtocolS3,
			},
		},
	}

	// valid base class used by subests
	baseClass := cosiapi.BucketClass{
		ObjectMeta: meta.ObjectMeta{
			Name: "s3-class",
		},
		Spec: cosiapi.BucketClassSpec{
			DriverName:     "cosi.s3.internal",
			DeletionPolicy: cosiapi.BucketDeletionPolicyDelete,
			Parameters: map[string]string{
				"maxSize": "100Gi",
				"maxIops": "10",
			},
		},
	}

	t.Run("valid claim and existing class", func(t *testing.T) {
		claim := baseClaim.DeepCopy()
		bootstrapped := cositest.MustBootstrap(t,
			baseClass.DeepCopy(),
		)

		bucket, err := createIntermediateBucket(
			bootstrapped.ContextWithLogger, bootstrapped.Logger, bootstrapped.Client,
			claim, "bc-qwerty",
		)
		assert.NoError(t, err)

		assert.Empty(t, bucket.Finalizers) // NO finalizers pre-applied

		// from the KEP: these values are just copy-pasted from claim and class
		// any additional unit tests around them won't add much additional value
		assert.Equal(t, "cosi.s3.internal", bucket.Spec.DriverName)
		assert.Equal(t, "Delete", string(bucket.Spec.DeletionPolicy))
		assert.Len(t, bucket.Spec.Protocols, 1)
		assert.Equal(t, "S3", string(bucket.Spec.Protocols[0]))
		assert.Equal(t, map[string]string{"maxSize": "100Gi", "maxIops": "10"}, bucket.Spec.Parameters)

		claimRef := bucket.Spec.BucketClaimRef
		assert.Equal(t, "my-bucket", claimRef.Name)
		assert.Equal(t, "my-ns", claimRef.Namespace)
		assert.Equal(t, "qwerty", string(claimRef.UID))
	})

	t.Run("bucketClass does not exist", func(t *testing.T) {
		claim := baseClaim.DeepCopy()
		bootstrapped := cositest.MustBootstrap(t) // no bucketclass exists

		bucket, err := createIntermediateBucket(
			bootstrapped.ContextWithLogger, bootstrapped.Logger, bootstrapped.Client,
			claim, "bc-qwerty",
		)
		assert.Error(t, err)
		assert.ErrorContains(t, err, "s3-class") // the class name
		assert.NotErrorIs(t, err, cosierr.NonRetryableError(nil))
		assert.Nil(t, bucket)
	})

	t.Run("claim specifies no class", func(t *testing.T) {
		claim := baseClaim.DeepCopy()
		claim.Spec.BucketClassName = ""
		bootstrapped := cositest.MustBootstrap(t,
			baseClass.DeepCopy(),
		)

		bucket, err := createIntermediateBucket(
			bootstrapped.ContextWithLogger, bootstrapped.Logger, bootstrapped.Client,
			claim, "bc-qwerty",
		)
		assert.Error(t, err)
		assert.ErrorIs(t, err, cosierr.NonRetryableError(nil))
		assert.Nil(t, bucket)
	})

	t.Run("bucket already exists (race condition)", func(t *testing.T) {
		claim := baseClaim.DeepCopy()
		raceBucket := &cosiapi.Bucket{
			ObjectMeta: meta.ObjectMeta{
				Name: "bc-qwerty",
			},
			Spec: cosiapi.BucketSpec{},
		}
		bootstrapped := cositest.MustBootstrap(t,
			baseClass.DeepCopy(),
			raceBucket,
		)

		bucket, err := createIntermediateBucket(
			bootstrapped.ContextWithLogger, bootstrapped.Logger, bootstrapped.Client,
			claim, "bc-qwerty",
		)
		assert.Error(t, err)
		assert.NotErrorIs(t, err, cosierr.NonRetryableError(nil))
		assert.Nil(t, bucket)
	})
}
