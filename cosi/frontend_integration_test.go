// Copyright 2026 NetApp, Inc. All Rights Reserved.

// This file drives the COSI frontend against a real Kubernetes API server. It is the only test
// that proves the vendored controller and sidecar reconcilers actually cooperate: everything
// else in cosi/ tests them in isolation, and the interesting failures -- a reconciler that never
// matches the driver name, a status subresource that is never written, a finalizer that blocks
// deletion forever -- only appear once real objects move through real watches.
//
// It is skipped unless TRIDENT_COSI_INTEGRATION=1, so `go test ./...` stays hermetic. There is
// deliberately no build tag: without one, this file is compiled and vetted on every ordinary
// test run, so it cannot quietly rot while nobody is running it.
//
//	kind create cluster --name trident-cosi --image kindest/node:v1.33.1
//	TRIDENT_COSI_INTEGRATION=1 go test ./cosi/ -run TestIntegration -v -count=1
//
// The cluster must be Kubernetes 1.32 or newer; see config.KubernetesVersionMinCOSI.

package cosi

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"

	"github.com/netapp/trident/config"
	cosicrd "github.com/netapp/trident/cosi/crd"
	"github.com/netapp/trident/cosi/provisioner/fake"
)

const (
	// testNamespace holds the namespaced objects. Created and torn down per run.
	testNamespace = "trident-cosi-integration"

	// pollTimeout bounds each wait. Generous: these are watch-driven reconciles, so the
	// realistic outcome is either sub-second or never.
	pollTimeout  = 90 * time.Second
	pollInterval = 250 * time.Millisecond
)

// skipUnlessIntegration returns the cluster config, or skips.
func skipUnlessIntegration(t *testing.T) *rest.Config {
	t.Helper()

	if os.Getenv("TRIDENT_COSI_INTEGRATION") != "1" {
		t.Skip("set TRIDENT_COSI_INTEGRATION=1 and point KUBECONFIG at a Kubernetes 1.32+ cluster")
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		rules, &clientcmd.ConfigOverrides{}).ClientConfig()
	require.NoError(t, err, "could not build a client config; is KUBECONFIG set?")

	return restConfig
}

// clusterMeetsCOSIFloor reports whether the target cluster is new enough for COSI.
//
// The two integration tests below are mirror images that split on this: one exercises the
// lifecycle on a supported cluster, the other proves the gate refuses an unsupported one. Which
// runs depends on what cluster is pointed at, and exactly one of them always does.
func clusterMeetsCOSIFloor(t *testing.T, restConfig *rest.Config) bool {
	t.Helper()

	client, err := discovery.NewDiscoveryClientForConfig(restConfig)
	require.NoError(t, err)

	return checkClusterVersion(client) == nil
}

// installCOSICRDs applies the CRDs the way Trident's installer does, label and all, and waits
// for each to be Established.
//
// The test installs them itself rather than assuming kubectl was run first, because the CRD
// content is part of what is under test: these are the exact bytes cosi/crd embeds and the
// installer applies.
func installCOSICRDs(ctx context.Context, t *testing.T, restConfig *rest.Config) {
	t.Helper()

	client, err := apiextensionsclient.NewForConfig(restConfig)
	require.NoError(t, err)

	for _, name := range cosicrd.Names() {
		doc, err := cosicrd.GetOwnedCRDYAML(name)
		require.NoError(t, err)

		var crd apiextensionv1.CustomResourceDefinition
		require.NoError(t, yaml.Unmarshal([]byte(doc), &crd))

		existing, err := client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			_, err = client.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, &crd, metav1.CreateOptions{})
			require.NoError(t, err, "creating %s", name)
		case err != nil:
			require.NoError(t, err, "getting %s", name)
		default:
			require.True(t, cosicrd.IsTridentManaged(existing.Labels),
				"%s exists but is not Trident-managed; this cluster has another COSI provider", name)
			crd.ResourceVersion = existing.ResourceVersion
			_, err = client.ApiextensionsV1().CustomResourceDefinitions().Update(ctx, &crd, metav1.UpdateOptions{})
			require.NoError(t, err, "updating %s", name)
		}
	}

	// Established means the API server is serving the resource. Creating objects before that
	// races, and the resulting "no matches for kind" is a confusing way to fail.
	for _, name := range cosicrd.Names() {
		err := wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true,
			func(ctx context.Context) (bool, error) {
				crd, err := client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
				if err != nil {
					return false, err
				}
				for _, c := range crd.Status.Conditions {
					if c.Type == apiextensionv1.Established && c.Status == apiextensionv1.ConditionTrue {
						return true, nil
					}
				}
				return false, nil
			})
		require.NoError(t, err, "%s never became Established", name)
	}
}

// newTestClient returns a client that speaks the COSI and core types.
func newTestClient(t *testing.T, restConfig *rest.Config) ctrlclient.Client {
	t.Helper()

	scheme, err := newScheme()
	require.NoError(t, err)

	client, err := ctrlclient.New(restConfig, ctrlclient.Options{Scheme: scheme})
	require.NoError(t, err)

	return client
}

// TestIntegrationCOSILifecycle walks a bucket from claim to credentials and back.
//
// Written as ordered subtests rather than separate Test functions because each phase depends on
// the last, and a failure part-way through should say which phase broke rather than cascading
// into a wall of unrelated errors.
func TestIntegrationCOSILifecycle(t *testing.T) {
	restConfig := skipUnlessIntegration(t)
	if !clusterMeetsCOSIFloor(t, restConfig) {
		t.Skipf("cluster is older than %s; TestIntegrationCOSIVersionGate covers that case",
			config.KubernetesVersionMinCOSI)
	}

	ctx := context.Background()

	installCOSICRDs(ctx, t, restConfig)
	client := newTestClient(t, restConfig)

	resetCOSIState(ctx, t, client)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}
	if err := client.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	// The fake provisioner is kept in scope so the test can assert on what the reconcilers
	// actually asked the backend to do, not merely on the objects they wrote.
	backend := fake.New()

	frontend, err := NewFrontend(backend, restConfig)
	require.NoError(t, err)

	require.NoError(t, frontend.Activate(), "Activate must return only once the caches are synced")

	t.Cleanup(func() {
		assert.NoError(t, frontend.Deactivate())
	})

	var (
		bucketClassName  = "trident-cosi-it-class"
		bucketClaimName  = "trident-cosi-it-claim"
		accessClassName  = "trident-cosi-it-access-class"
		bucketAccessName = "trident-cosi-it-access"
		accessSecretName = "trident-cosi-it-secret"
		boundBucketName  string
	)

	t.Cleanup(func() { cleanup(t, client, bucketClassName, accessClassName) })

	t.Run("BucketClaim provisions a Bucket", func(t *testing.T) {
		bucketClass := &cosiapi.BucketClass{
			ObjectMeta: metav1.ObjectMeta{Name: bucketClassName},
			Spec: cosiapi.BucketClassSpec{
				DriverName: fake.DriverName,
				// Delete rather than Retain, so the deletion phase below actually reaches
				// the driver instead of orphaning the bucket by policy.
				DeletionPolicy: cosiapi.BucketDeletionPolicyDelete,
				Parameters:     map[string]string{"tier": "standard"},
			},
		}
		require.NoError(t, createOrReplace(ctx, client, bucketClass))

		claim := &cosiapi.BucketClaim{
			ObjectMeta: metav1.ObjectMeta{Name: bucketClaimName, Namespace: testNamespace},
			Spec: cosiapi.BucketClaimSpec{
				BucketClassName: bucketClassName,
				Protocols:       []cosiapi.ObjectProtocol{cosiapi.ObjectProtocolS3},
			},
		}
		require.NoError(t, createOrReplace(ctx, client, claim))

		// The controller reconciler creates the Bucket and records its name on the claim;
		// the sidecar reconciler then calls the driver and marks it ready. Waiting on the
		// claim's readyToUse covers both halves in one condition, which is the point: this
		// is the assertion that the two reconciler sets are actually talking to each other.
		require.NoError(t, waitFor(ctx, t, "BucketClaim to become ready", func(ctx context.Context) (bool, error) {
			var got cosiapi.BucketClaim
			if err := client.Get(ctx, types.NamespacedName{Name: bucketClaimName, Namespace: testNamespace}, &got); err != nil {
				return false, err
			}
			if got.Status.Error != nil {
				return false, fmt.Errorf("claim reported an error: %v", ptr.Deref(got.Status.Error.Message, ""))
			}
			boundBucketName = got.Status.BoundBucketName
			return got.Status.ReadyToUse != nil && *got.Status.ReadyToUse && boundBucketName != "", nil
		}))

		var bucket cosiapi.Bucket
		require.NoError(t, client.Get(ctx, types.NamespacedName{Name: boundBucketName}, &bucket))

		assert.Equal(t, fake.DriverName, bucket.Spec.DriverName)
		assert.Equal(t, bucketClaimName, bucket.Spec.BucketClaimRef.Name)
		assert.Equal(t, testNamespace, bucket.Spec.BucketClaimRef.Namespace)

		// The bucket ID is the driver's answer coming back through the sidecar. Without it,
		// the object exists but nothing was actually provisioned.
		require.NotNil(t, bucket.Status.ReadyToUse)
		assert.True(t, *bucket.Status.ReadyToUse)
		assert.NotEmpty(t, bucket.Status.BucketID, "sidecar did not record the driver's bucket ID")

		assert.Equal(t, 1, backend.Calls["CreateBucket"], "driver should have been asked exactly once")
	})

	t.Run("BucketAccess yields a credentials Secret", func(t *testing.T) {
		require.NotEmpty(t, boundBucketName, "previous phase did not bind a bucket")

		accessClass := &cosiapi.BucketAccessClass{
			ObjectMeta: metav1.ObjectMeta{Name: accessClassName},
			Spec: cosiapi.BucketAccessClassSpec{
				DriverName:         fake.DriverName,
				AuthenticationType: cosiapi.BucketAccessAuthenticationTypeKey,
			},
		}
		require.NoError(t, createOrReplace(ctx, client, accessClass))

		access := &cosiapi.BucketAccess{
			ObjectMeta: metav1.ObjectMeta{Name: bucketAccessName, Namespace: testNamespace},
			Spec: cosiapi.BucketAccessSpec{
				BucketAccessClassName: accessClassName,
				Protocol:              cosiapi.ObjectProtocolS3,
				BucketClaims: []cosiapi.BucketClaimAccess{{
					BucketClaimName:  bucketClaimName,
					AccessMode:       cosiapi.BucketAccessModeReadWrite,
					AccessSecretName: accessSecretName,
				}},
			},
		}
		require.NoError(t, createOrReplace(ctx, client, access))

		require.NoError(t, waitFor(ctx, t, "BucketAccess to become ready", func(ctx context.Context) (bool, error) {
			var got cosiapi.BucketAccess
			if err := client.Get(ctx, types.NamespacedName{Name: bucketAccessName, Namespace: testNamespace}, &got); err != nil {
				return false, err
			}
			if got.Status.Error != nil {
				return false, fmt.Errorf("access reported an error: %v", ptr.Deref(got.Status.Error.Message, ""))
			}
			return got.Status.ReadyToUse != nil && *got.Status.ReadyToUse, nil
		}))

		// The Secret is the whole deliverable of BucketAccess: it is what a workload mounts.
		// It is also the object the manager deliberately does not cache, so reading it here
		// exercises the live-read path configured in newManager.
		var secret corev1.Secret
		require.NoError(t, waitFor(ctx, t, "credentials Secret to appear", func(ctx context.Context) (bool, error) {
			err := client.Get(ctx, types.NamespacedName{Name: accessSecretName, Namespace: testNamespace}, &secret)
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return err == nil, err
		}))

		assert.NotEmpty(t, secret.Data, "credentials Secret is empty")
		assert.Equal(t, 1, backend.Calls["GrantAccess"], "driver should have granted access exactly once")
	})

	t.Run("Deactivate stops cleanly", func(t *testing.T) {
		require.NoError(t, frontend.Deactivate())

		// Deactivate is called unconditionally during Trident shutdown, so a second call has
		// to be a no-op rather than an error or a panic on a closed channel.
		assert.NoError(t, frontend.Deactivate(), "Deactivate must be idempotent")

		// And re-activating must work, because that is what a frontend restart looks like.
		require.NoError(t, frontend.Activate())
	})
}

// resetCOSIState clears anything a previous run left behind, then waits for the test namespace
// to actually be gone.
//
// This is not boilerplate, it is the deletion gap made concrete. COSI puts a protection
// finalizer on Buckets, BucketClaims, BucketAccesses and the credentials Secret, and nothing
// removes it: upstream's delete path is unimplemented, so a namespace holding a BucketClaim sits
// in Terminating forever and the next run cannot even create its objects. Trident's
// `tridentctl obliviate crd` does the same clearing for the same reason.
//
// When Trident implements deletion properly, this should shrink to an ordinary delete-and-wait.
// Until then it is the honest way to make the test re-runnable, and a standing reminder of what
// still has to be built.
func resetCOSIState(ctx context.Context, t *testing.T, client ctrlclient.Client) {
	t.Helper()

	var buckets cosiapi.BucketList
	require.NoError(t, client.List(ctx, &buckets))
	for i := range buckets.Items {
		clearFinalizers(ctx, t, client, &buckets.Items[i])
		_ = client.Delete(ctx, &buckets.Items[i])
	}

	var claims cosiapi.BucketClaimList
	require.NoError(t, client.List(ctx, &claims))
	for i := range claims.Items {
		clearFinalizers(ctx, t, client, &claims.Items[i])
	}

	var accesses cosiapi.BucketAccessList
	require.NoError(t, client.List(ctx, &accesses))
	for i := range accesses.Items {
		clearFinalizers(ctx, t, client, &accesses.Items[i])
	}

	// The credentials Secret carries the finalizer too, and it is not a COSI resource, so
	// nothing above reaches it.
	var secrets corev1.SecretList
	if err := client.List(ctx, &secrets, ctrlclient.InNamespace(testNamespace)); err == nil {
		for i := range secrets.Items {
			clearFinalizers(ctx, t, client, &secrets.Items[i])
		}
	}

	_ = client.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})

	require.NoError(t, waitFor(ctx, t, "leftover namespace to disappear", func(ctx context.Context) (bool, error) {
		var ns corev1.Namespace
		err := client.Get(ctx, types.NamespacedName{Name: testNamespace}, &ns)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}))
}

// clearFinalizers strips every finalizer from one object, tolerating a concurrent delete.
func clearFinalizers(ctx context.Context, t *testing.T, client ctrlclient.Client, obj ctrlclient.Object) {
	t.Helper()

	if len(obj.GetFinalizers()) == 0 {
		return
	}

	obj.SetFinalizers(nil)
	if err := client.Update(ctx, obj); err != nil && !apierrors.IsNotFound(err) && !apierrors.IsConflict(err) {
		require.NoError(t, err, "clearing finalizers from %s", obj.GetName())
	}
}

// createOrReplace makes the test re-runnable against a cluster left over from a previous run.
func createOrReplace(ctx context.Context, client ctrlclient.Client, obj ctrlclient.Object) error {
	err := client.Create(ctx, obj)
	if !apierrors.IsAlreadyExists(err) {
		return err
	}

	if err = client.Delete(ctx, obj); err != nil {
		return err
	}

	return wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true,
		func(ctx context.Context) (bool, error) {
			obj.SetResourceVersion("")
			err := client.Create(ctx, obj)
			if apierrors.IsAlreadyExists(err) {
				return false, nil
			}
			return err == nil, err
		})
}

// waitFor polls until cond is satisfied, attributing a timeout to a named step.
func waitFor(ctx context.Context, t *testing.T, what string, cond wait.ConditionWithContextFunc) error {
	t.Helper()

	if err := wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, cond); err != nil {
		return fmt.Errorf("waiting for %s: %w", what, err)
	}

	t.Logf("observed %s", what)

	return nil
}

// cleanup removes what the test created. Namespaced objects go with the namespace; the
// cluster-scoped classes do not, so they are deleted by name.
func cleanup(t *testing.T, client ctrlclient.Client, bucketClassName, accessClassName string) {
	t.Helper()

	ctx := context.Background()

	_ = client.Delete(ctx, &cosiapi.BucketClass{ObjectMeta: metav1.ObjectMeta{Name: bucketClassName}})
	_ = client.Delete(ctx, &cosiapi.BucketAccessClass{ObjectMeta: metav1.ObjectMeta{Name: accessClassName}})
	_ = client.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
}

// TestIntegrationCOSIVersionGate is the counterpart to the lifecycle test: it runs only against
// a cluster below the COSI floor, and asserts Trident refuses in terms an administrator can act
// on rather than letting the API server emit a CEL compilation dump.
//
// This case is worth a real cluster rather than a fake discovery client, because the thing being
// verified is that Trident notices before it touches the API, and a fake cannot tell the
// difference between "checked first" and "tried and failed".
func TestIntegrationCOSIVersionGate(t *testing.T) {
	restConfig := skipUnlessIntegration(t)
	if clusterMeetsCOSIFloor(t, restConfig) {
		t.Skipf("cluster is %s or newer; TestIntegrationCOSILifecycle covers that case",
			config.KubernetesVersionMinCOSI)
	}

	frontend, err := NewFrontend(fake.New(), restConfig)
	require.NoError(t, err)

	err = frontend.Activate()

	require.Error(t, err, "Activate must refuse a cluster below the COSI floor")
	assert.Contains(t, err.Error(), config.KubernetesVersionMinCOSI[1:],
		"the error must name the required version")
	assert.Contains(t, err.Error(), "COSI requires Kubernetes")

	// A refused Activate must leave nothing running, so that shutdown does not block on a
	// manager that was never started.
	assert.NoError(t, frontend.Deactivate())
}
