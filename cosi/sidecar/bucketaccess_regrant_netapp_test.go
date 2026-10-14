// Copyright 2026 NetApp, Inc. All Rights Reserved.

// This file is NETAPP LOCAL. It is not vendored from upstream and is absent from
// cosi/upstream.manifest, so resync.sh ignores it. It covers SkipRegrantWhenGranted, the
// deliberate divergence in cosi/sidecar/bucketaccess.go. Upstream's re-grant behaviour, which
// still applies when the field is false, stays covered by the vendored bucketaccess_test.go.

package reconciler_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"

	sidecar "github.com/netapp/trident/cosi/sidecar"
	cositest "github.com/netapp/trident/cosi/third_party/test"
	controllertest "github.com/netapp/trident/cosi/third_party/test/controller"
	sidecartest "github.com/netapp/trident/cosi/third_party/test/sidecar"
)

// TestBucketAccessSkipRegrantWhenGranted starts each case from an access the driver has
// granted once, then reconciles it again. The driver mints a new key on every grant, the way
// StorageGRID does, so a second grant is visible as a changed access key ID in the Secret.
func TestBucketAccessSkipRegrantWhenGranted(t *testing.T) {
	const keyIDVar = string(cosiapi.CredentialVar_S3_AccessKeyId)
	const secretKeyVar = string(cosiapi.CredentialVar_S3_AccessSecretKey)

	accessClass := cositest.OpinionatedS3BucketAccessClass()
	claim := cositest.OpinionatedS3BucketClaim("my-ns", "my-bucket")
	baseAccess := cosiapi.BucketAccess{
		ObjectMeta: metav1.ObjectMeta{Name: "my-access", Namespace: "my-ns", UID: "zxcvbn"},
		Spec: cosiapi.BucketAccessSpec{
			BucketClaims: []cosiapi.BucketClaimAccess{{
				BucketClaimName:  claim.Name,
				AccessMode:       cosiapi.BucketAccessModeReadWrite,
				AccessSecretName: "my-bucket-creds",
			}},
			BucketAccessClassName: accessClass.Name,
			Protocol:              cosiapi.ObjectProtocolS3,
		},
	}
	secretName := cositest.SecretNsName(&baseAccess, 0)

	grants := 0
	fakeServer := cositest.FakeProvisionerServer{
		GrantBucketAccessFunc: func(
			_ context.Context, req *cosiproto.DriverGrantBucketAccessRequest,
		) (*cosiproto.DriverGrantBucketAccessResponse, error) {
			grants++
			buckets := make([]*cosiproto.DriverGrantBucketAccessResponse_BucketInfo, 0, len(req.Buckets))
			for _, b := range req.Buckets {
				buckets = append(buckets, &cosiproto.DriverGrantBucketAccessResponse_BucketInfo{
					BucketId: b.BucketId,
					BucketInfo: &cosiproto.ObjectProtocolAndBucketInfo{S3: &cosiproto.S3BucketInfo{
						BucketId:        b.BucketId,
						Endpoint:        "s3.test.net",
						Region:          "us-east-1",
						AddressingStyle: &cosiproto.S3AddressingStyle{Style: cosiproto.S3AddressingStyle_PATH},
					}},
				})
			}
			return &cosiproto.DriverGrantBucketAccessResponse{
				AccountId: "cosi-" + req.AccountName,
				Buckets:   buckets,
				Credentials: &cosiproto.CredentialInfo{S3: &cosiproto.S3CredentialInfo{
					AccessKeyId:     fmt.Sprintf("key-%d", grants),
					AccessSecretKey: fmt.Sprintf("secret-%d", grants),
				}},
			}, nil
		},
	}

	cleanup, serve, tmpSock, err := cositest.RpcServer(nil, &fakeServer)
	defer cleanup()
	require.NoError(t, err)
	go serve()
	conn, err := cositest.RpcClientConn(tmpSock)
	require.NoError(t, err)

	driverInfo := sidecartest.OpinionatedS3DriverInfo()
	driverInfo.ProvisionerClient = cosiproto.NewProvisionerClient(conn)

	// grantOnce returns a fresh cluster holding an access the driver has granted exactly once.
	grantOnce := func(t *testing.T) (*cositest.Dependencies, *sidecar.BucketAccessReconciler) {
		bootstrapped := cositest.MustBootstrap(t,
			baseAccess.DeepCopy(), accessClass, claim.DeepCopy(), cositest.OpinionatedS3BucketClass(),
		)
		_, err := controllertest.ReconcileBucketClaim(t, bootstrapped, cositest.NsName(claim))
		require.NoError(t, err)
		_, err = sidecartest.ReconcileOpinionatedS3Bucket(t, bootstrapped, cositest.BucketNsName(claim))
		require.NoError(t, err)
		_, err = controllertest.ReconcileBucketClaim(t, bootstrapped, cositest.NsName(claim))
		require.NoError(t, err)
		_, err = controllertest.ReconcileBucketAccess(t, bootstrapped, cositest.NsName(&baseAccess))
		require.NoError(t, err)

		grants = 0
		r := &sidecar.BucketAccessReconciler{
			Client:                 bootstrapped.Client,
			Scheme:                 bootstrapped.Client.Scheme(),
			DriverInfo:             driverInfo,
			SkipRegrantWhenGranted: true,
		}
		reconcileAccess(t, bootstrapped, r, &baseAccess)
		require.Equal(t, 1, grants)

		access := getAccess(t, bootstrapped, &baseAccess)
		require.True(t, *access.Status.ReadyToUse)
		require.NotEmpty(t, access.Status.AccountID)
		require.Equal(t, "key-1", getSecret(t, bootstrapped, secretName).StringData[keyIDVar])
		return bootstrapped, r
	}

	t.Run("granted access is not re-granted", func(t *testing.T) {
		bootstrapped, r := grantOnce(t)
		initAccess := getAccess(t, bootstrapped, &baseAccess)

		reconcileAccess(t, bootstrapped, r, &baseAccess)

		assert.Equal(t, 1, grants)
		assert.Equal(t, "key-1", getSecret(t, bootstrapped, secretName).StringData[keyIDVar])
		assert.Equal(t, initAccess.Status, getAccess(t, bootstrapped, &baseAccess).Status)
	})

	t.Run("credentials in Data, as the API server returns them, are not re-granted", func(t *testing.T) {
		bootstrapped, r := grantOnce(t)
		sec := getSecret(t, bootstrapped, secretName)
		sec.Data = map[string][]byte{}
		for k, v := range sec.StringData {
			sec.Data[k] = []byte(v)
		}
		sec.StringData = nil
		require.NoError(t, bootstrapped.Client.Update(bootstrapped.ContextWithLogger, sec))

		reconcileAccess(t, bootstrapped, r, &baseAccess)

		assert.Equal(t, 1, grants)
	})

	t.Run("a transient failure before the check does not re-grant, and its error is cleared", func(t *testing.T) {
		bootstrapped, r := grantOnce(t)
		r.Client = &failBucketGetOnce{Client: bootstrapped.Client}

		_, err := r.Reconcile(bootstrapped.ContextWithLogger, ctrl.Request{NamespacedName: cositest.NsName(&baseAccess)})
		require.Error(t, err)
		access := getAccess(t, bootstrapped, &baseAccess)
		require.NotNil(t, access.Status.Error)
		require.True(t, *access.Status.ReadyToUse)

		reconcileAccess(t, bootstrapped, r, &baseAccess)

		assert.Equal(t, 1, grants)
		assert.Equal(t, "key-1", getSecret(t, bootstrapped, secretName).StringData[keyIDVar])
		assert.Nil(t, getAccess(t, bootstrapped, &baseAccess).Status.Error)
	})

	t.Run("without the opt-in, upstream re-grants", func(t *testing.T) {
		bootstrapped, r := grantOnce(t)
		r.SkipRegrantWhenGranted = false

		reconcileAccess(t, bootstrapped, r, &baseAccess)

		assert.Equal(t, 2, grants)
		assert.Equal(t, "key-2", getSecret(t, bootstrapped, secretName).StringData[keyIDVar])
	})

	// Each case below breaks one condition of an existing grant. The driver must be called
	// again, and the Secret must end up with the new credentials.
	regrantCases := []struct {
		name   string
		mutate func(t *testing.T, bootstrapped *cositest.Dependencies)
	}{
		{
			name: "secret missing the secret key",
			mutate: func(t *testing.T, bootstrapped *cositest.Dependencies) {
				sec := getSecret(t, bootstrapped, secretName)
				delete(sec.StringData, secretKeyVar)
				require.NoError(t, bootstrapped.Client.Update(bootstrapped.ContextWithLogger, sec))
			},
		},
		{
			name: "secret with an empty access key ID",
			mutate: func(t *testing.T, bootstrapped *cositest.Dependencies) {
				sec := getSecret(t, bootstrapped, secretName)
				sec.StringData[keyIDVar] = ""
				require.NoError(t, bootstrapped.Client.Update(bootstrapped.ContextWithLogger, sec))
			},
		},
		{
			name: "access not ready",
			mutate: func(t *testing.T, bootstrapped *cositest.Dependencies) {
				access := getAccess(t, bootstrapped, &baseAccess)
				access.Status.ReadyToUse = ptr.To(false)
				require.NoError(t, bootstrapped.Client.Status().Update(bootstrapped.ContextWithLogger, access))
			},
		},
		{
			name: "access with no account ID",
			mutate: func(t *testing.T, bootstrapped *cositest.Dependencies) {
				access := getAccess(t, bootstrapped, &baseAccess)
				access.Status.AccountID = ""
				require.NoError(t, bootstrapped.Client.Status().Update(bootstrapped.ContextWithLogger, access))
			},
		},
	}
	for _, tc := range regrantCases {
		t.Run(tc.name+" is re-granted", func(t *testing.T) {
			bootstrapped, r := grantOnce(t)
			tc.mutate(t, bootstrapped)

			reconcileAccess(t, bootstrapped, r, &baseAccess)

			assert.Equal(t, 2, grants)
			sec := getSecret(t, bootstrapped, secretName)
			assert.Equal(t, "key-2", sec.StringData[keyIDVar])
			assert.Equal(t, "secret-2", sec.StringData[secretKeyVar])
			access := getAccess(t, bootstrapped, &baseAccess)
			assert.True(t, *access.Status.ReadyToUse)
			assert.NotEmpty(t, access.Status.AccountID)
			assert.Nil(t, access.Status.Error)
		})
	}
}

// failBucketGetOnce fails the first Bucket read, as a transient API error would.
type failBucketGetOnce struct {
	client.Client
	failed bool
}

func (c *failBucketGetOnce) Get(
	ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption,
) error {
	if _, ok := obj.(*cosiapi.Bucket); ok && !c.failed {
		c.failed = true
		return errors.New("transient API error")
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func reconcileAccess(
	t *testing.T, bootstrapped *cositest.Dependencies, r *sidecar.BucketAccessReconciler, access *cosiapi.BucketAccess,
) {
	t.Helper()
	res, err := r.Reconcile(bootstrapped.ContextWithLogger, ctrl.Request{NamespacedName: cositest.NsName(access)})
	require.NoError(t, err)
	require.Empty(t, res)
}

func getAccess(t *testing.T, bootstrapped *cositest.Dependencies, base *cosiapi.BucketAccess) *cosiapi.BucketAccess {
	t.Helper()
	access := &cosiapi.BucketAccess{}
	require.NoError(t, bootstrapped.Client.Get(bootstrapped.ContextWithLogger, cositest.NsName(base), access))
	return access
}

func getSecret(t *testing.T, bootstrapped *cositest.Dependencies, nsName types.NamespacedName) *corev1.Secret {
	t.Helper()
	sec := &corev1.Secret{}
	require.NoError(t, bootstrapped.Client.Get(bootstrapped.ContextWithLogger, nsName, sec))
	return sec
}
