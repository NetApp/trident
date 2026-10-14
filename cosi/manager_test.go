// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cosi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/cosi/adapter"
	"github.com/netapp/trident/cosi/provisioner/fake"
	cosisidecar "github.com/netapp/trident/cosi/sidecar"
)

// TestNewSchemeRegistersCOSIAndCoreTypes is the reason newScheme exists: the manager
// must speak every type the reconcilers read or write. Missing COSI types would fail at
// cache-sync with "no matches for kind"; missing corev1 would fail the first time the
// sidecar tried to write a credentials Secret.
func TestNewSchemeRegistersCOSIAndCoreTypes(t *testing.T) {
	scheme, err := newScheme()
	require.NoError(t, err)

	mustBeInScheme(t, scheme, &cosiapi.Bucket{})
	mustBeInScheme(t, scheme, &cosiapi.BucketClaim{})
	mustBeInScheme(t, scheme, &cosiapi.BucketClass{})
	mustBeInScheme(t, scheme, &cosiapi.BucketAccess{})
	mustBeInScheme(t, scheme, &cosiapi.BucketAccessClass{})
	mustBeInScheme(t, scheme, &corev1.Secret{})
}

func mustBeInScheme(t *testing.T, scheme *runtime.Scheme, obj runtime.Object) {
	t.Helper()

	gvks, _, err := scheme.ObjectKinds(obj)
	require.NoError(t, err, "%T", obj)
	require.NotEmpty(t, gvks, "%T has no GVK", obj)
}

// TestManagerOptionsSkipNameValidation pins the line that lets both BucketAccess
// reconcilers share a process. Removing it does not fail the build: it fails at
// Activate, on a cluster, with a name-collision error that looks like a controller-runtime
// bug rather than a configuration one.
func TestManagerOptionsSkipNameValidation(t *testing.T) {
	opts := managerOptions(runtime.NewScheme(), newLogger())

	require.NotNil(t, opts.Controller.SkipNameValidation)
	assert.True(t, *opts.Controller.SkipNameValidation)
}

// TestManagerOptionsRunReconcilersConcurrently pins the worker count the sidecar depends on.
// The sidecar reconcilers call the driver synchronously, so with controller-runtime's default
// of one worker a slow driver call blocks every other object. The default is set on the
// manager rather than per controller so the vendored reconcilers need no edit;
// controller-runtime applies it to any controller that does not set its own.
func TestManagerOptionsRunReconcilersConcurrently(t *testing.T) {
	opts := managerOptions(runtime.NewScheme(), newLogger())

	assert.Equal(t, maxConcurrentReconciles, opts.Controller.MaxConcurrentReconciles)
	assert.Greater(t, opts.Controller.MaxConcurrentReconciles, 1,
		"a single worker serializes every driver call behind the slowest one")
}

// TestManagerOptionsDisableSecretCache pins the cache exclusion the sidecar depends on.
// Without it the default client would list/watch every Secret in every namespace COSI
// touches, which is both a memory cost and an RBAC grant Trident does not want to make.
func TestManagerOptionsDisableSecretCache(t *testing.T) {
	opts := managerOptions(runtime.NewScheme(), newLogger())

	require.NotNil(t, opts.Client.Cache)
	require.Len(t, opts.Client.Cache.DisableFor, 1)
	_, isSecret := opts.Client.Cache.DisableFor[0].(*corev1.Secret)
	assert.True(t, isSecret, "DisableFor must name Secret, got %T", opts.Client.Cache.DisableFor[0])
}

// TestManagerOptionsDoNotBindPorts is the other half of hosting COSI in-process: the
// manager must not steal Trident's metrics, health, or leader-election endpoints.
func TestManagerOptionsDoNotBindPorts(t *testing.T) {
	opts := managerOptions(runtime.NewScheme(), newLogger())

	assert.False(t, opts.LeaderElection)
	assert.Equal(t, "0", opts.Metrics.BindAddress)
	assert.Equal(t, "0", opts.HealthProbeBindAddress)
	require.NotNil(t, opts.GracefulShutdownTimeout)
	assert.Equal(t, shutdownTimeout, *opts.GracefulShutdownTimeout)
}

func TestCheckClusterVersion(t *testing.T) {
	t.Run("supported", func(t *testing.T) {
		err := checkClusterVersion(fakeVersionClient{info: &k8sversion.Info{GitVersion: "v1.36.0"}})
		assert.NoError(t, err)
	})

	t.Run("below COSI floor", func(t *testing.T) {
		err := checkClusterVersion(fakeVersionClient{info: &k8sversion.Info{GitVersion: "v1.31.9"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), config.KubernetesVersionMinCOSI[1:])
		assert.Contains(t, err.Error(), "COSI requires Kubernetes")
	})

	t.Run("discovery failure", func(t *testing.T) {
		err := checkClusterVersion(fakeVersionClient{err: assert.AnError})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "query the Kubernetes server version")
	})
}

func TestCheckAPIInstalled(t *testing.T) {
	t.Run("CRDs present on a supported cluster", func(t *testing.T) {
		cfg := discoveryServer(t, "v1.36.0", true)
		assert.NoError(t, checkAPIInstalled(cfg))
	})

	t.Run("missing CRDs", func(t *testing.T) {
		cfg := discoveryServer(t, "v1.36.0", false)
		err := checkAPIInstalled(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), cosiGroupVersion)
		assert.Contains(t, err.Error(), "install the COSI CRDs")
		assert.True(t, apierrors.IsNotFound(err), "the wrapped cause should remain NotFound")
	})

	t.Run("cluster below the COSI floor", func(t *testing.T) {
		cfg := discoveryServer(t, "v1.30.0", true)
		err := checkAPIInstalled(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "COSI requires Kubernetes")
	})
}

// TestNewManagerRegistersCollidingReconcilers is the runtime proof of SkipNameValidation:
// both BucketAccess reconcilers register as "bucketaccess", and without the skip
// controller-runtime would reject the second. Construction does not need an API server;
// the manager only talks to the cluster once Start is called.
func TestNewManagerRegistersCollidingReconcilers(t *testing.T) {
	mgr, err := newManager(dummyRESTConfig(), testDriverInfo(t), newLogger())
	require.NoError(t, err)
	require.NotNil(t, mgr)

	assert.NotNil(t, mgr.GetScheme().AllKnownTypes()[cosiapi.GroupVersion.WithKind("Bucket")])
}

func TestWaitForCacheSyncHonoursCancel(t *testing.T) {
	mgr, err := newManager(dummyRESTConfig(), testDriverInfo(t), newLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = waitForCacheSync(ctx, mgr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "COSI informer caches did not sync")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWaitForCacheSyncHonoursTimeout(t *testing.T) {
	mgr, err := newManager(dummyRESTConfig(), testDriverInfo(t), newLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	err = waitForCacheSync(ctx, mgr)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

type fakeVersionClient struct {
	info *k8sversion.Info
	err  error
}

func (f fakeVersionClient) ServerVersion() (*k8sversion.Info, error) {
	return f.info, f.err
}

// discoveryServer is enough of a kube-apiserver for checkAPIInstalled: /version and
// the pinned COSI group version. Anything else returns 404, which is what a cluster
// that has not installed the CRDs looks like.
func discoveryServer(t *testing.T, gitVersion string, serveCOSI bool) *rest.Config {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&k8sversion.Info{GitVersion: gitVersion, Major: "1", Minor: "36"})
	})
	mux.HandleFunc("/apis/"+cosiGroupVersion, func(w http.ResponseWriter, _ *http.Request) {
		if !serveCOSI {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(&metav1.Status{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
				Status:   metav1.StatusFailure,
				Reason:   metav1.StatusReasonNotFound,
				Code:     http.StatusNotFound,
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(&metav1.APIResourceList{
			GroupVersion: cosiGroupVersion,
			APIResources: []metav1.APIResource{{Name: "buckets", Kind: "Bucket", Namespaced: false}},
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return &rest.Config{Host: server.URL}
}

func dummyRESTConfig() *rest.Config {
	return &rest.Config{Host: "https://127.0.0.1:1"}
}

func testDriverInfo(t *testing.T) cosisidecar.DriverInfo {
	t.Helper()

	client := adapter.New(fake.New())
	reported, err := client.DriverGetInfo(context.Background(), &cosiproto.DriverGetInfoRequest{})
	require.NoError(t, err)

	info, err := cosisidecar.NewDriverInfo(reported, client)
	require.NoError(t, err)

	return *info
}
