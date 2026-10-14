// Copyright 2026 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakedynamic "k8s.io/client-go/dynamic/fake"
	sigsyaml "sigs.k8s.io/yaml"

	cosicrd "github.com/netapp/trident/cosi/crd"
)

func TestPlanCOSICRD(t *testing.T) {
	crdWithLabels := func(labels map[string]string) *apiextensionv1.CustomResourceDefinition {
		return &apiextensionv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: cosicrd.BucketCRDName, Labels: labels},
		}
	}

	tests := []struct {
		name     string
		existing *apiextensionv1.CustomResourceDefinition
		want     COSICRDAction
		why      string
	}{
		{
			name:     "absent",
			existing: nil,
			want:     COSICRDCreate,
			why:      "nothing there, install it",
		},
		{
			name:     "installed by Trident",
			existing: crdWithLabels(map[string]string{cosicrd.ManagedByLabel: cosicrd.ManagedByValue}),
			want:     COSICRDUpdate,
			why:      "Trident's own marking, safe to update",
		},
		{
			name:     "installed by the community COSI controller",
			existing: crdWithLabels(map[string]string{cosicrd.ManagedByLabel: "cosi-controller"}),
			want:     COSICRDConflict,
			why:      "someone else owns this API",
		},
		{
			name:     "installed by an unknown actor",
			existing: crdWithLabels(nil),
			want:     COSICRDConflict,
			why:      "unmarked means not ours; refuse rather than assume",
		},
		{
			name:     "labelled but not managed-by",
			existing: crdWithLabels(map[string]string{"app": "trident"}),
			want:     COSICRDConflict,
			why:      "a coincidental label is not ownership",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, PlanCOSICRD(tt.existing), tt.why)
		})
	}
}

// fakeCOSICRDReader answers the two CRD lookups TridentCOSIObjectsExist makes.
//
// crds holds the CRDs that exist; anything absent from it reports not-found, which is the
// ordinary state of a cluster that never enabled COSI.
type fakeCOSICRDReader struct {
	crds map[string]*apiextensionv1.CustomResourceDefinition
	err  error
}

func (f *fakeCOSICRDReader) CheckCRDExists(crdName string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	_, ok := f.crds[crdName]
	return ok, nil
}

func (f *fakeCOSICRDReader) GetCRD(crdName string) (*apiextensionv1.CustomResourceDefinition, error) {
	if crd, ok := f.crds[crdName]; ok {
		return crd, nil
	}
	return nil, fmt.Errorf("no such CRD %s", crdName)
}

// cosiCRD builds a CRD as the cluster would hold it, with the plural name the dynamic client
// needs to resolve a GVR.
func cosiCRD(name, plural string, tridentManaged bool) *apiextensionv1.CustomResourceDefinition {
	labels := map[string]string{}
	if tridentManaged {
		labels[cosicrd.ManagedByLabel] = cosicrd.ManagedByValue
	}

	return &apiextensionv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec: apiextensionv1.CustomResourceDefinitionSpec{
			Group: cosicrd.Group,
			Names: apiextensionv1.CustomResourceDefinitionNames{Plural: plural},
		},
	}
}

// cosiObject builds a stored custom resource of the given kind.
func cosiObject(kind, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": cosicrd.Group + "/" + cosicrd.Version,
			"kind":       kind,
			"metadata":   map[string]interface{}{"name": name},
		},
	}
}

// dynamicClientWith returns a fake dynamic client holding the given COSI objects.
//
// The list kinds have to be registered explicitly. The fake derives a list kind from the scheme,
// and with these types unregistered it panics inside List rather than returning an error.
func dynamicClientWith(objects ...runtime.Object) *fakedynamic.FakeDynamicClient {
	gvr := func(resource string) schema.GroupVersionResource {
		return schema.GroupVersionResource{Group: cosicrd.Group, Version: cosicrd.Version, Resource: resource}
	}

	listKinds := map[schema.GroupVersionResource]string{
		gvr("buckets"):             "BucketList",
		gvr("bucketclaims"):        "BucketClaimList",
		gvr("bucketclasses"):       "BucketClassList",
		gvr("bucketaccesses"):      "BucketAccessList",
		gvr("bucketaccessclasses"): "BucketAccessClassList",
	}

	return fakedynamic.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objects...)
}

// TestTridentCOSIObjectsExist covers the guard that warns an administrator when COSI is turned
// off while objects are still out there. Both answers matter: a false negative means the silent
// abandonment Decision 11 describes, and a false positive means crying wolf on every reconcile
// of the overwhelmingly common cluster that never enabled COSI.
func TestTridentCOSIObjectsExist(t *testing.T) {
	tridentBuckets := cosiCRD(cosicrd.BucketCRDName, "buckets", true)
	tridentClaims := cosiCRD(cosicrd.BucketClaimCRDName, "bucketclaims", true)
	foreignBuckets := cosiCRD(cosicrd.BucketCRDName, "buckets", false)

	tests := []struct {
		name    string
		crds    map[string]*apiextensionv1.CustomResourceDefinition
		objects []runtime.Object
		want    bool
		why     string
	}{
		{
			name: "no COSI CRDs at all",
			crds: nil,
			want: false,
			why:  "COSI was never enabled; nothing to abandon",
		},
		{
			name: "Trident CRDs present but empty",
			crds: map[string]*apiextensionv1.CustomResourceDefinition{
				cosicrd.BucketCRDName:      tridentBuckets,
				cosicrd.BucketClaimCRDName: tridentClaims,
			},
			want: false,
			why:  "the API is installed but nobody used it",
		},
		{
			// Buckets comes first in the canonical order and is empty here, so this also
			// covers the loop carrying on past a kind with nothing in it.
			name: "an object exists under a Trident CRD",
			crds: map[string]*apiextensionv1.CustomResourceDefinition{
				cosicrd.BucketCRDName:      tridentBuckets,
				cosicrd.BucketClaimCRDName: tridentClaims,
			},
			objects: []runtime.Object{cosiObject("BucketClaim", "claim-1")},
			want:    true,
			why:     "a BucketClaim is exactly what hangs a namespace on delete",
		},
		{
			name: "objects exist but the CRD is not Trident's",
			crds: map[string]*apiextensionv1.CustomResourceDefinition{
				cosicrd.BucketCRDName: foreignBuckets,
			},
			objects: []runtime.Object{cosiObject("Bucket", "someone-elses-bucket")},
			want:    false,
			why:     "another provider owns that API and its controller is presumably still running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeCOSICRDReader{crds: tt.crds}

			got, err := TridentCOSIObjectsExist(context.Background(), reader, dynamicClientWith(tt.objects...))

			require.NoError(t, err)
			assert.Equal(t, tt.want, got, tt.why)
		})
	}
}

// TestTridentCOSIObjectsExistPropagatesError checks a broken lookup is reported rather than
// silently answering "nothing there", which would suppress the warning on exactly the clusters
// that are already unhealthy.
func TestTridentCOSIObjectsExistPropagatesError(t *testing.T) {
	reader := &fakeCOSICRDReader{err: fmt.Errorf("API server unreachable")}

	_, err := TridentCOSIObjectsExist(context.Background(), reader, dynamicClientWith())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "API server unreachable")
}

// TestCOSICRDConflictErrorIsActionable checks the error names the CRD and says what to do.
// The whole point of refusing is that an administrator can tell what happened.
func TestCOSICRDConflictErrorIsActionable(t *testing.T) {
	err := &COSICRDConflictError{CRDName: cosicrd.BucketCRDName}

	assert.Contains(t, err.Error(), cosicrd.BucketCRDName)
	assert.Contains(t, err.Error(), "not installed by Trident")
	assert.Contains(t, err.Error(), "only COSI provider")
}

func TestCOSICRDNames(t *testing.T) {
	names := COSICRDNames()

	assert.Len(t, names, 5)
	assert.Contains(t, names, "buckets.objectstorage.k8s.io")
	assert.Contains(t, names, "bucketclaims.objectstorage.k8s.io")
	assert.Contains(t, names, "bucketclasses.objectstorage.k8s.io")
	assert.Contains(t, names, "bucketaccesses.objectstorage.k8s.io")
	assert.Contains(t, names, "bucketaccessclasses.objectstorage.k8s.io")
}

// TestCOSICRDsAreNotInTridentCRDNames is the guard for the finalizer decision. CRDnames drives
// AddFinalizerToCRDs; COSI CRDs must stay out of it, because Trident should not hold a deletion
// lock on an API shared with the rest of the cluster.
func TestCOSICRDsAreNotInTridentCRDBundle(t *testing.T) {
	bundle := GetCRDsYAML()

	for _, name := range COSICRDNames() {
		assert.NotContains(t, bundle, name,
			"%s leaked into the Trident CRD bundle; it must stay gated behind enable-cosi", name)
	}
}

func TestGetOwnedCOSICRDYAML(t *testing.T) {
	yaml, err := GetOwnedCOSICRDYAML(cosicrd.BucketCRDName)

	assert.NoError(t, err)
	assert.Contains(t, yaml, cosicrd.ManagedByValue)

	_, err = GetOwnedCOSICRDYAML("not-a-cosi-crd")
	assert.Error(t, err)
}

// TestControllerClusterRoleCOSIRulesAreGated guards a privilege boundary, not a formatting
// detail. Serving COSI needs cluster-wide create/update/delete on Secrets, because that is where
// bucket credentials land. A Trident that is not serving COSI has no business holding it, so the
// rules must appear only when the feature is on.
func TestControllerClusterRoleCOSIRulesAreGated(t *testing.T) {
	controllerLabels := map[string]string{TridentAppLabelKey: "controller.csi.trident.netapp.io"}

	parse := func(t *testing.T, enableCOSI bool) rbacv1.ClusterRole {
		t.Helper()

		var role rbacv1.ClusterRole
		doc := GetClusterRoleYAML("trident-controller", controllerLabels, nil, enableCOSI)
		require.NoError(t, sigsyaml.Unmarshal([]byte(doc), &role))

		return role
	}

	grants := func(role rbacv1.ClusterRole, group, resource, verb string) bool {
		for _, rule := range role.Rules {
			if !slices.Contains(rule.APIGroups, group) || !slices.Contains(rule.Resources, resource) {
				continue
			}
			if slices.Contains(rule.Verbs, verb) {
				return true
			}
		}
		return false
	}

	t.Run("disabled", func(t *testing.T) {
		role := parse(t, false)

		assert.False(t, grants(role, "objectstorage.k8s.io", "buckets", "create"),
			"COSI is off; the controller must not be granted the COSI API")
		assert.False(t, grants(role, "", "secrets", "delete"),
			"COSI is off; the controller must not be granted cluster-wide Secret deletion")
		assert.NotEmpty(t, role.Rules, "gating COSI must not empty the role")
	})

	t.Run("enabled", func(t *testing.T) {
		role := parse(t, true)

		assert.True(t, grants(role, "objectstorage.k8s.io", "buckets", "create"))
		assert.True(t, grants(role, "objectstorage.k8s.io", "bucketclaims", "watch"))
		assert.True(t, grants(role, "objectstorage.k8s.io", "buckets/finalizers", "update"))
		assert.True(t, grants(role, "", "secrets", "create"),
			"the BucketAccess reconciler writes the credentials Secret")
	})
}

// TestOperatorClusterRoleCoversControllerCOSIRules encodes the rule that broke the first real
// deployment: Kubernetes refuses to let a service account grant permissions it does not itself
// hold, so every COSI rule the operator writes into the controller role must also appear in the
// operator's own static role. Without this the failure surfaces only on a live cluster, as a
// 180-second timeout with the whole role dumped into the message.
func TestOperatorClusterRoleCoversControllerCOSIRules(t *testing.T) {
	operatorRoleYAML, err := os.ReadFile(filepath.Join("..", "..", "deploy", "clusterrole.yaml"))
	require.NoError(t, err)

	var operatorRole rbacv1.ClusterRole
	require.NoError(t, sigsyaml.Unmarshal(operatorRoleYAML, &operatorRole))

	var controllerRole rbacv1.ClusterRole
	controllerLabels := map[string]string{TridentAppLabelKey: "controller.csi.trident.netapp.io"}
	require.NoError(t, sigsyaml.Unmarshal(
		[]byte(GetClusterRoleYAML("trident-controller", controllerLabels, nil, true)), &controllerRole))

	covers := func(group, resource, verb string) bool {
		for _, rule := range operatorRole.Rules {
			if !slices.Contains(rule.APIGroups, group) || !slices.Contains(rule.Resources, resource) {
				continue
			}
			if slices.Contains(rule.Verbs, verb) || slices.Contains(rule.Verbs, "*") {
				return true
			}
		}
		return false
	}

	for _, rule := range controllerRole.Rules {
		for _, group := range rule.APIGroups {
			if group != "objectstorage.k8s.io" {
				continue
			}
			for _, resource := range rule.Resources {
				for _, verb := range rule.Verbs {
					assert.True(t, covers(group, resource, verb),
						"operator role cannot grant %s/%s %q that it does not hold", group, resource, verb)
				}
			}
		}
	}
}
