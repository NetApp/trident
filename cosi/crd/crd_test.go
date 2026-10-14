// Copyright 2026 NetApp, Inc. All Rights Reserved.

package crd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// TestEmbeddedCRDsAreComplete guards the embed directives. A renamed or deleted YAML file is a
// compile error, but a file that is present and empty is not, and an empty CRD would fail at
// install time in a cluster rather than here.
func TestEmbeddedCRDsAreComplete(t *testing.T) {
	byName := YAMLByName()
	require.Len(t, byName, 5)

	for _, name := range Names() {
		doc, ok := byName[name]
		require.True(t, ok, "no YAML embedded for %s", name)
		require.NotEmpty(t, doc, "%s YAML is empty", name)

		var crd apiextensionv1.CustomResourceDefinition
		require.NoError(t, yaml.Unmarshal([]byte(doc), &crd), "%s is not valid CRD YAML", name)

		assert.Equal(t, name, crd.Name)
		assert.Equal(t, Group, crd.Spec.Group)

		// Trident pins exactly one version. More than one served version would mean the
		// cluster could store an object Trident's reconcilers do not understand.
		require.Len(t, crd.Spec.Versions, 1, "%s serves more than one version", name)
		assert.Equal(t, Version, crd.Spec.Versions[0].Name)
		assert.True(t, crd.Spec.Versions[0].Served)
		assert.True(t, crd.Spec.Versions[0].Storage)
	}
}

// TestVendoredCRDsAreUnlabelled is the counterpart to GetOwnedCRDYAML: the files on disk must
// stay exactly as upstream wrote them, because cosi/hack/resync.sh compares them. If someone
// "helpfully" adds the label to the YAML instead, resync starts reporting drift forever.
func TestVendoredCRDsAreUnlabelled(t *testing.T) {
	for _, name := range Names() {
		doc, err := GetCRDYAML(name)
		require.NoError(t, err)
		assert.NotContains(t, doc, ManagedByValue,
			"%s carries the Trident label on disk; it belongs in GetOwnedCRDYAML instead", name)
	}
}

func TestGetOwnedCRDYAML(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			owned, err := GetOwnedCRDYAML(name)
			require.NoError(t, err)

			var crd apiextensionv1.CustomResourceDefinition
			require.NoError(t, yaml.Unmarshal([]byte(owned), &crd))

			assert.True(t, IsTridentManaged(crd.Labels), "label was not applied")

			// The rest of the document must survive the round trip. The schema is the part
			// that matters and the part most easily lost, so check it is still there.
			assert.Equal(t, name, crd.Name)
			assert.Equal(t, Group, crd.Spec.Group)
			require.Len(t, crd.Spec.Versions, 1)
			require.NotNil(t, crd.Spec.Versions[0].Schema, "OpenAPI schema was dropped")
			require.NotNil(t, crd.Spec.Versions[0].Schema.OpenAPIV3Schema)
			assert.NotEmpty(t, crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties)

			// The api-approved annotation is not decoration: the API server rejects CRDs in
			// *.k8s.io groups without it.
			assert.Contains(t, crd.Annotations, "api-approved.kubernetes.io")
		})
	}
}

func TestGetOwnedCRDYAMLUnknownName(t *testing.T) {
	_, err := GetOwnedCRDYAML("nonexistent.objectstorage.k8s.io")
	assert.Error(t, err)
}

func TestGetCRDsYAMLContainsEveryCRD(t *testing.T) {
	bundle := GetCRDsYAML()

	// Split the way an installer would, and confirm each document is a CRD we expect.
	var found []string
	for _, doc := range strings.Split(bundle, "\n---\n") {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		var crd apiextensionv1.CustomResourceDefinition
		require.NoError(t, yaml.Unmarshal([]byte(doc), &crd))
		found = append(found, crd.Name)
	}

	assert.ElementsMatch(t, Names(), found)
}

func TestIsTridentManaged(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"nil labels", nil, false},
		{"empty labels", map[string]string{}, false},
		{"trident managed", map[string]string{ManagedByLabel: ManagedByValue}, true},
		{"managed by someone else", map[string]string{ManagedByLabel: "cosi-controller"}, false},
		{"unrelated labels only", map[string]string{"app": "cosi"}, false},
		{
			"trident label among others",
			map[string]string{"app": "cosi", ManagedByLabel: ManagedByValue},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsTridentManaged(tt.labels))
		})
	}
}
