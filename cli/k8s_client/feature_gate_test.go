// Copyright 2025 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	fakeext "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

// reactor defines a single reactor to be added to the fake clientset.
type reactor struct {
	Verb     string
	Resource string
	Reaction k8stesting.ReactionFunc
}

const (
	testOldSnapshotterImage = "docker.repo.eng.netapp.com/globalcicd/trident/csi-snapshotter:v7.2.0"
	testV84SnapshotterImage = "docker.repo.eng.netapp.com/globalcicd/trident/csi-snapshotter:v8.4.0"
	testV85SnapshotterImage = "docker.repo.eng.netapp.com/globalcicd/trident/csi-snapshotter:v8.5.0"
	testNewSnapshotterImage = "docker.repo.eng.netapp.com/globalcicd/trident/csi-snapshotter:v8.6.0"
)

// fakeKubeClientWithReactors constructs a KubeClient with the given reactors.
func fakeKubeClientWithReactors(t *testing.T, reactors []reactor) *KubeClient {
	t.Helper()
	fakeExt := fakeext.NewClientset()
	for _, r := range reactors {
		fakeExt.Fake.PrependReactor(r.Verb, r.Resource, r.Reaction)
	}
	return &KubeClient{extClientset: fakeExt}
}

func newFakeCRD(t *testing.T, name string, versions []string) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	specVersions := make([]apiextensionsv1.CustomResourceDefinitionVersion, len(versions))
	for i, v := range versions {
		specVersions[i] = apiextensionsv1.CustomResourceDefinitionVersion{Name: v}
	}
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Versions: specVersions,
		},
	}
}

func TestConstructCSIFeatureGateYAMLSnippets(t *testing.T) {
	// Outer key: feature name
	features := map[string]map[string]struct {
		reactors     []reactor // reactors define a set of props to be added to the fake clientset for the feature.
		snapshotter  string
		assertError  assert.ErrorAssertionFunc
		assertResult func(*testing.T, map[string]string)
	}{
		AutoFeatureGateVolumeGroupSnapshot: {
			"all CRDs present and supported": {
				reactors: []reactor{
					{
						Verb:     "get",
						Resource: "customresourcedefinitions",
						Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
							get := action.(k8stesting.GetAction)
							name := get.GetName()
							crd := newFakeCRD(t, name, []string{"v1beta1", "v1beta2", "v1"})
							return true, crd, nil
						},
					},
				},
				snapshotter: testNewSnapshotterImage,
				assertError: assert.NoError,
				assertResult: func(t *testing.T, snip map[string]string) {
					assert.Contains(t, snip, "{FEATURE_GATES_CSI_SNAPSHOTTER}")
					assert.Equal(t, "CSIVolumeGroupSnapshot=true", snip["{FEATURE_GATES_CSI_SNAPSHOTTER}"])
				},
			},
			"old snapshotter with beta CRDs": {
				reactors: []reactor{
					{
						Verb:     "get",
						Resource: "customresourcedefinitions",
						Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
							get := action.(k8stesting.GetAction)
							name := get.GetName()
							crd := newFakeCRD(t, name, []string{"v1beta1", "v1beta2", "v1"})
							return true, crd, nil
						},
					},
				},
				snapshotter: testOldSnapshotterImage,
				assertError: assert.NoError,
				assertResult: func(t *testing.T, snip map[string]string) {
					assert.Contains(t, snip, "{FEATURE_GATES_CSI_SNAPSHOTTER}")
					assert.Equal(t, "CSIVolumeGroupSnapshot=true", snip["{FEATURE_GATES_CSI_SNAPSHOTTER}"])
				},
			},
			"one CRD missing": {
				reactors: []reactor{
					{
						Verb:     "get",
						Resource: "customresourcedefinitions",
						Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
							get := action.(k8stesting.GetAction)
							name := get.GetName()
							if name == volumeGroupSnapshotClassCRDName {
								return true, nil, apierrors.NewNotFound(apiextensionsv1.Resource("customresourcedefinitions"), name)
							}
							crd := newFakeCRD(t, name, []string{"v1beta1"})
							return true, crd, nil
						},
					},
				},
				snapshotter: testNewSnapshotterImage,
				assertError: assert.Error,
				assertResult: func(t *testing.T, snip map[string]string) {
					assert.Empty(t, snip)
				},
			},
			"unsupported v1beta1 version": {
				reactors: []reactor{
					{
						Verb:     "get",
						Resource: "customresourcedefinitions",
						Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
							get := action.(k8stesting.GetAction)
							name := get.GetName()
							crd := newFakeCRD(t, name, []string{"v1beta1"})
							return true, crd, nil
						},
					},
				},
				snapshotter: testNewSnapshotterImage,
				assertError: assert.Error,
				assertResult: func(t *testing.T, snip map[string]string) {
					assert.Empty(t, snip)
				},
			},
			"v8.5 snapshotter with v1beta2 CRDs": {
				reactors: []reactor{
					{
						Verb:     "get",
						Resource: "customresourcedefinitions",
						Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
							get := action.(k8stesting.GetAction)
							name := get.GetName()
							crd := newFakeCRD(t, name, []string{"v1beta2"})
							return true, crd, nil
						},
					},
				},
				snapshotter: testV85SnapshotterImage,
				assertError: assert.NoError,
				assertResult: func(t *testing.T, snip map[string]string) {
					assert.Contains(t, snip, "{FEATURE_GATES_CSI_SNAPSHOTTER}")
					assert.Equal(t, "CSIVolumeGroupSnapshot=true", snip["{FEATURE_GATES_CSI_SNAPSHOTTER}"])
				},
			},
			"v8.6 snapshotter with v1beta2 CRDs": {
				reactors: []reactor{
					{
						Verb:     "get",
						Resource: "customresourcedefinitions",
						Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
							get := action.(k8stesting.GetAction)
							name := get.GetName()
							crd := newFakeCRD(t, name, []string{"v1beta2"})
							return true, crd, nil
						},
					},
				},
				snapshotter: testNewSnapshotterImage,
				assertError: assert.Error,
				assertResult: func(t *testing.T, snip map[string]string) {
					assert.Empty(t, snip)
				},
			},
			"client error": {
				reactors: []reactor{
					{
						Verb:     "get",
						Resource: "customresourcedefinitions",
						Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
							get := action.(k8stesting.GetAction)
							name := get.GetName()
							if name == volumeGroupSnapshotCRDName {
								return true, nil, errors.New("API error")
							}
							crd := newFakeCRD(t, name, []string{"v1beta1"})
							return true, crd, nil
						},
					},
				},
				snapshotter: testNewSnapshotterImage,
				assertResult: func(t *testing.T, snip map[string]string) {
					assert.Empty(t, snip)
				},
				assertError: assert.Error,
			},
			"nil client": {
				reactors:     nil,
				snapshotter:  testNewSnapshotterImage,
				assertResult: func(t *testing.T, snip map[string]string) { assert.Nil(t, snip) },
				assertError:  assert.Error,
			},
		},
		// Example for a future feature:
		/*
			"AnotherFeature": {
				"all CRDs present and supported": {
					reactors: []reactor{ ... },
					assertSnip: func(t *testing.T, snip map[string]string) {
						assert.Contains(t, snip, "{FEATURE_GATES_ANOTHER_FEATURE}")
						assert.Equal(t, "CSIAnotherFeature=true", snip["{FEATURE_GATES_ANOTHER_FEATURE}"])
					},
					assertError: assert.NoError,
				},
				...
			},
		*/
	}

	for feature, cases := range features {
		for name, tc := range cases {
			t.Run(feature+"/"+name, func(t *testing.T) {
				var client KubernetesClient
				if tc.reactors != nil {
					client = fakeKubeClientWithReactors(t, tc.reactors)
				} else {
					client = nil
				}
				snippets, err := ConstructCSIFeatureGateYAMLSnippets(client, tc.snapshotter)
				tc.assertError(t, err)
				tc.assertResult(t, snippets)
			})
		}
	}
}

func Test_canAutoEnableFeatureGate(t *testing.T) {
	gate := autoFeatureGateVolumeGroupSnapshot

	tests := map[string]struct {
		reactors    []reactor
		snapshotter string
		assertBool  assert.BoolAssertionFunc
		assertError assert.ErrorAssertionFunc
	}{
		"when all CRDs exist and have the supported versions": {
			reactors: []reactor{
				{
					Verb:     "get",
					Resource: "customresourcedefinitions",
					Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
						get := action.(k8stesting.GetAction)
						name := get.GetName()
						crd := newFakeCRD(t, name, []string{"v1beta1", "v1beta2", "v1"})
						return true, crd, nil
					},
				},
			},
			snapshotter: testNewSnapshotterImage,
			assertBool:  assert.True,
			assertError: assert.NoError,
		},
		"with old snapshotter and beta CRDs": {
			reactors: []reactor{
				{
					Verb:     "get",
					Resource: "customresourcedefinitions",
					Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
						get := action.(k8stesting.GetAction)
						name := get.GetName()
						crd := newFakeCRD(t, name, []string{"v1beta2"})
						return true, crd, nil
					},
				},
			},
			snapshotter: testOldSnapshotterImage,
			assertBool:  assert.True,
			assertError: assert.NoError,
		},
		"with one CRD missing": {
			reactors: []reactor{
				{
					Verb:     "get",
					Resource: "customresourcedefinitions",
					Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
						get := action.(k8stesting.GetAction)
						name := get.GetName()
						if name == volumeGroupSnapshotClassCRDName {
							return true, nil, apierrors.NewNotFound(apiextensionsv1.Resource("customresourcedefinitions"), name)
						}
						crd := newFakeCRD(t, name, []string{"v1beta1", "v1beta2", "v1"})
						return true, crd, nil
					},
				},
			},
			snapshotter: testNewSnapshotterImage,
			assertBool:  assert.False,
			assertError: assert.Error,
		},
		"with unsupported v1beta1 version": {
			reactors: []reactor{
				{
					Verb:     "get",
					Resource: "customresourcedefinitions",
					Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
						get := action.(k8stesting.GetAction)
						name := get.GetName()
						crd := newFakeCRD(t, name, []string{"v1beta1"})
						return true, crd, nil
					},
				},
			},
			snapshotter: testNewSnapshotterImage,
			assertBool:  assert.False,
			assertError: assert.Error,
		},
		"with old snapshotter and v1-only CRDs": {
			reactors: []reactor{
				{
					Verb:     "get",
					Resource: "customresourcedefinitions",
					Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
						get := action.(k8stesting.GetAction)
						name := get.GetName()
						crd := newFakeCRD(t, name, []string{"v1"})
						return true, crd, nil
					},
				},
			},
			snapshotter: testOldSnapshotterImage,
			assertBool:  assert.False,
			assertError: assert.Error,
		},
		"with v8.4 snapshotter and v1beta2 CRDs": {
			reactors: []reactor{
				{
					Verb:     "get",
					Resource: "customresourcedefinitions",
					Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
						get := action.(k8stesting.GetAction)
						name := get.GetName()
						crd := newFakeCRD(t, name, []string{"v1beta2"})
						return true, crd, nil
					},
				},
			},
			snapshotter: testV84SnapshotterImage,
			assertBool:  assert.True,
			assertError: assert.NoError,
		},
		"with v8.6 snapshotter and v1beta2 CRDs": {
			reactors: []reactor{
				{
					Verb:     "get",
					Resource: "customresourcedefinitions",
					Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
						get := action.(k8stesting.GetAction)
						name := get.GetName()
						crd := newFakeCRD(t, name, []string{"v1beta2"})
						return true, crd, nil
					},
				},
			},
			snapshotter: testNewSnapshotterImage,
			assertBool:  assert.False,
			assertError: assert.Error,
		},
		"with client error": {
			reactors: []reactor{
				{
					Verb:     "get",
					Resource: "customresourcedefinitions",
					Reaction: func(action k8stesting.Action) (bool, runtime.Object, error) {
						get := action.(k8stesting.GetAction)
						name := get.GetName()
						if name == volumeGroupSnapshotCRDName {
							return true, nil, errors.New("API error")
						}
						crd := newFakeCRD(t, name, []string{"v1"})
						return true, crd, nil
					},
				},
			},
			snapshotter: testNewSnapshotterImage,
			assertBool:  assert.False,
			assertError: assert.Error,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := fakeKubeClientWithReactors(t, tc.reactors)
			canEnable, err := canAutoEnableFeatureGate(client, gate, tc.snapshotter)
			tc.assertError(t, err)
			tc.assertBool(t, canEnable)
		})
	}
}
