/*
Copyright 2026 The Kubernetes Authors.

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

// Package test contains utilities for unit testing.
package test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
)

// Dependencies contains bootstrapped COSI unit test dependencies.
type Dependencies struct {
	ContextWithLogger context.Context // context containing the test logger
	Client            client.Client   // test client containing bootstrapped objects
	Logger            logr.Logger     // the test logger, for convenience
}

// Create a new fake client with given initial objects/lists, or panic if an error occurs.
func mustNewClient(
	initialClientObjects []client.Object,
	initialClientObjectLists []client.ObjectList,
) client.WithWatch {
	scheme := runtime.NewScheme()
	err := cosiapi.AddToScheme(scheme)
	if err != nil {
		panic(err)
	}
	err = corev1.AddToScheme(scheme) // required for bucketaccess tests with secrets
	if err != nil {
		panic(err)
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initialClientObjects...).
		WithLists(initialClientObjectLists...).
		WithStatusSubresource(
			&cosiapi.Bucket{},
			&cosiapi.BucketClaim{},
			&cosiapi.BucketAccess{},
		).
		Build()

	return client
}

// MustBootstrap bootstraps new COSI unit test dependencies, or panics if an error occurs.
func MustBootstrap(t *testing.T, initialClientObjects ...client.Object) *Dependencies {
	logger := testr.NewWithOptions(t, testr.Options{
		Verbosity: 1,
	})
	contextWithLogger := logr.NewContext(context.Background(), logger)

	client := mustNewClient(initialClientObjects, []client.ObjectList{})

	return &Dependencies{
		ContextWithLogger: contextWithLogger,
		Client:            client,
		Logger:            logger,
	}
}

// MustCopy returns a copy of the Dependencies with the same client objects, or panics if an error
// occurs. Only copies client objects kinds that COSI is expected to manage.
func (d *Dependencies) MustCopy() *Dependencies {
	new := &Dependencies{
		ContextWithLogger: d.ContextWithLogger, // safe to copy
		Logger:            d.Logger,            // safe to copy
		// Client is not safe to copy
	}

	// List all objects in the client that COSI manages, and populate a new client with the lists

	buckets := &cosiapi.BucketList{}
	err := d.Client.List(d.ContextWithLogger, buckets)
	if err != nil {
		panic(err)
	}

	bClaims := &cosiapi.BucketClaimList{}
	err = d.Client.List(d.ContextWithLogger, bClaims)
	if err != nil {
		panic(err)
	}

	bClasses := &cosiapi.BucketClassList{}
	err = d.Client.List(d.ContextWithLogger, bClasses)
	if err != nil {
		panic(err)
	}

	bAccesses := &cosiapi.BucketAccessList{}
	err = d.Client.List(d.ContextWithLogger, bAccesses)
	if err != nil {
		panic(err)
	}

	bAccessesClasses := &cosiapi.BucketAccessClassList{}
	err = d.Client.List(d.ContextWithLogger, bAccessesClasses)
	if err != nil {
		panic(err)
	}

	secrets := &corev1.SecretList{}
	err = d.Client.List(d.ContextWithLogger, secrets)
	if err != nil {
		panic(err)
	}

	new.Client = mustNewClient(
		[]client.Object{},
		[]client.ObjectList{
			buckets,
			bClaims,
			bClasses,
			bAccesses,
			bAccessesClasses,
			secrets,
		},
	)

	return new
}

// AssertResourceDoesNotExist asserts that the given resource does not exist in the Dependencies's client.
func (d *Dependencies) AssertResourceDoesNotExist(t *testing.T, key types.NamespacedName, obj client.Object) {
	t.Helper()
	err := d.Client.Get(d.ContextWithLogger, key, obj)
	assert.True(t, kerrors.IsNotFound(err), "resource %T %v should not exist:\n%#v", obj, key, obj)
}

// NsName returns the NamespacedName for the given object.
func NsName(o client.Object) types.NamespacedName {
	return types.NamespacedName{
		Namespace: o.GetNamespace(),
		Name:      o.GetName(),
	}
}

// BucketNsName returns the NamespacedName for the Bucket bound to the given BucketClaim.
func BucketNsName(claim *cosiapi.BucketClaim) types.NamespacedName {
	name := claim.Status.BoundBucketName
	if name == "" {
		name = claim.Spec.ExistingBucketName
	}
	if name == "" {
		name = "bc-" + string(claim.UID)
	}
	return types.NamespacedName{
		Namespace: "",
		Name:      name,
	}
}

// SecretNsName returns the NamespacedName for the Secret defined in the given BucketAccess
// selected from the access's bucketClaims by index.
func SecretNsName(access *cosiapi.BucketAccess, claimIndex int) types.NamespacedName {
	return types.NamespacedName{
		Namespace: access.Namespace,
		Name:      access.Spec.BucketClaims[claimIndex].AccessSecretName,
	}
}

// ObjectMetaWithUID returns ObjectMeta with the given namespace and name, and a deterministic UID
// suitable for unit testing.
func ObjectMetaWithUID(namespace, name string) ctrl.ObjectMeta {
	return ctrl.ObjectMeta{
		Namespace: namespace,
		Name:      name,
		// not a real UID, but is deterministic and fine for unit testing
		UID: types.UID(namespace + "-" + name),
	}
}

// OpinionatedBucketClass returns a BucketClass with opinionated, working defaults for unit tests.
// It is suitable for unit testing behavior that relies on a BucketClaim to be reconciled as a
// prerequisite. It is not suitable for unit testing BucketClaim reconciliation.
func OpinionatedBucketClass(identifier string) *cosiapi.BucketClass {
	return &cosiapi.BucketClass{
		ObjectMeta: ctrl.ObjectMeta{
			Name: "opinionated-" + identifier,
		},
		Spec: cosiapi.BucketClassSpec{
			DriverName:     identifier + ".cosi.test",
			DeletionPolicy: cosiapi.BucketDeletionPolicyDelete,
		},
	}
}

// OpinionatedBucketClaim returns a BucketClaim with opinionated, working defaults for unit tests.
// It is suitable for unit testing behavior that relies on a BucketClaim to be reconciled as a
// prerequisite. It is not suitable for unit testing BucketClaim reconciliation.
// nolint:lll // long line is fine for test code
func OpinionatedBucketClaim(namespace, name, className string, protocols ...cosiapi.ObjectProtocol) *cosiapi.BucketClaim {
	return &cosiapi.BucketClaim{
		ObjectMeta: ObjectMetaWithUID(namespace, name),
		Spec: cosiapi.BucketClaimSpec{
			BucketClassName: className,
			Protocols:       protocols,
		},
	}
}

// OpinionatedBucketAccessClass returns a BucketAccessClass with opinionated, working defaults for unit tests.
// It is suitable for unit testing behavior that relies on a BucketAccess to be reconciled as a
// prerequisite. It is not suitable for unit testing BucketAccess reconciliation.
func OpinionatedBucketAccessClass(identifier string) *cosiapi.BucketAccessClass {
	bClass := OpinionatedBucketClass(identifier)
	return &cosiapi.BucketAccessClass{
		ObjectMeta: ctrl.ObjectMeta{
			Name: bClass.Name, // reuse same name as opinionated BucketClass for convenience
		},
		Spec: cosiapi.BucketAccessClassSpec{
			DriverName:                  bClass.Spec.DriverName, // use same driver as opinionated BucketClass
			AuthenticationType:          cosiapi.BucketAccessAuthenticationTypeKey,
			DisallowedBucketAccessModes: []cosiapi.BucketAccessMode{},             // all allowed
			MultiBucketAccess:           cosiapi.MultiBucketAccessMultipleBuckets, // allow multi
		},
	}
}

// OpinionatedS3BucketClass returns a BucketClass configured for an opinionated S3 driver.
// It is suitable for unit testing behavior that relies on a BucketClaim to be reconciled as a
// prerequisite. It is not suitable for unit testing BucketClaim reconciliation.
func OpinionatedS3BucketClass() *cosiapi.BucketClass {
	return OpinionatedBucketClass("s3")
}

// OpinionatedS3BucketClaim returns an BucketClaim configured to use the opinionated S3 BucketClass.
// It is suitable for unit testing behavior that relies on a BucketClaim to be reconciled as a
// prerequisite. It is not suitable for unit testing BucketClaim reconciliation.
func OpinionatedS3BucketClaim(namespace, name string) *cosiapi.BucketClaim {
	return OpinionatedBucketClaim(namespace, name, OpinionatedS3BucketClass().Name, cosiapi.ObjectProtocolS3)
}

// OpinionatedS3BucketAccessClass returns a BucketAccessClass configured for an opinionated S3 driver.
// It is suitable for unit testing behavior that relies on a BucketAccess to be reconciled as a
// prerequisite. It is not suitable for unit testing BucketAccess reconciliation.
func OpinionatedS3BucketAccessClass() *cosiapi.BucketAccessClass {
	return OpinionatedBucketAccessClass("s3")
}

// OpinionatedGcsBucketClass returns a BucketClass configured for an opinionated GCS driver.
// It is suitable for unit testing behavior that relies on a BucketClaim to be reconciled as a
// prerequisite. It is not suitable for unit testing BucketClaim reconciliation.
func OpinionatedGcsBucketClass() *cosiapi.BucketClass {
	return OpinionatedBucketClass("gcs")
}

// OpinionatedGcsBucketClaim returns an BucketClaim configured to use the opinionated GCS BucketClass.
// It is suitable for unit testing behavior that relies on a BucketClaim to be reconciled as a
// prerequisite. It is not suitable for unit testing BucketClaim reconciliation.
func OpinionatedGcsBucketClaim(namespace, name string) *cosiapi.BucketClaim {
	return OpinionatedBucketClaim(namespace, name, OpinionatedGcsBucketClass().Name, cosiapi.ObjectProtocolGcs)
}

// OpinionatedGcsBucketAccessClass returns a BucketAccessClass configured for an opinionated CS driver.
// It is suitable for unit testing behavior that relies on a BucketAccess to be reconciled as a
// prerequisite. It is not suitable for unit testing BucketAccess reconciliation.
func OpinionatedGcsBucketAccessClass() *cosiapi.BucketAccessClass {
	return OpinionatedBucketAccessClass("gcs")
}

// OpinionatedAzureBucketClass returns a BucketClass configured for an opinionated Azure driver.
func OpinionatedAzureBucketClass() *cosiapi.BucketClass {
	return OpinionatedBucketClass("azure")
}

// OpinionatedAzureBucketClaim returns an BucketClaim configured to use the opinionated Azure BucketClass.
// It is suitable for unit testing behavior that relies on a BucketClaim to be reconciled as a
// prerequisite. It is not suitable for unit testing BucketClaim reconciliation.
func OpinionatedAzureBucketClaim(namespace, name string) *cosiapi.BucketClaim {
	return OpinionatedBucketClaim(namespace, name, OpinionatedAzureBucketClass().Name, cosiapi.ObjectProtocolAzure)
}

// OpinionatedAzureBucketAccessClass returns a BucketAccessClass configured for an opinionated Azure driver.
// It is suitable for unit testing behavior that relies on a BucketAccess to be reconciled as a
// prerequisite. It is not suitable for unit testing BucketAccess reconciliation.
func OpinionatedAzureBucketAccessClass() *cosiapi.BucketAccessClass {
	return OpinionatedBucketAccessClass("azure")
}
