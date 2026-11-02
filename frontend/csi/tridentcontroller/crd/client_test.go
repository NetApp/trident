// Copyright 2026 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/netapp/trident/config"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	"github.com/netapp/trident/utils/models"
)

// testPublicationCR builds a TridentVolumePublication the way the controller persists it. When
// labeled is false the node-name label is removed, as on publications that predate the label.
func testPublicationCR(t *testing.T, namespace, volume, node string, labeled bool) *tridentv1.TridentVolumePublication {
	t.Helper()

	cr, err := tridentv1.NewTridentVolumePublication(&models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volume, node),
		VolumeName: volume,
		NodeName:   node,
	})
	require.NoError(t, err)
	cr.Namespace = namespace
	if !labeled {
		delete(cr.Labels, config.TridentNodeNameLabel)
	}
	return cr
}

func TestClient_GetDesiredPublications(t *testing.T) {
	const (
		ns   = "trident"
		node = "node-a"
	)

	type publication struct {
		volume  string
		node    string
		labeled bool
	}

	tests := []struct {
		name            string
		queryNode       string // defaults to node
		publications    []publication
		wantVolumeNames []string
	}{
		{
			name:            "no publications",
			publications:    nil,
			wantVolumeNames: []string{},
		},
		{
			name:            "this node, labeled",
			publications:    []publication{{"vol1", node, true}},
			wantVolumeNames: []string{"vol1"},
		},
		{
			name:            "this node, unlabeled",
			publications:    []publication{{"vol1", node, false}},
			wantVolumeNames: []string{"vol1"},
		},
		{
			name:            "other node, labeled",
			publications:    []publication{{"vol1", "node-b", true}},
			wantVolumeNames: []string{},
		},
		{
			name:            "other node, unlabeled",
			publications:    []publication{{"vol1", "node-b", false}},
			wantVolumeNames: []string{},
		},
		{
			name: "mix of labeled and unlabeled across nodes",
			publications: []publication{
				{"vol1", node, true},
				{"vol2", node, false},
				{"vol3", "node-b", true},
				{"vol4", "node-b", false},
				{"vol5", node, false},
			},
			wantVolumeNames: []string{"vol1", "vol2", "vol5"},
		},
		{
			name: "same volume published to this node and another node",
			publications: []publication{
				{"vol1", node, false},
				{"vol1", "node-b", false},
			},
			wantVolumeNames: []string{"vol1"},
		},
		{
			name:      "node name that is a dotted suffix of another node name",
			queryNode: "b",
			publications: []publication{
				{"vol1", "b", false},
				{"vol2", "a.b", false},
			},
			wantVolumeNames: []string{"vol1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			objects := make([]k8sruntime.Object, 0, len(tc.publications))
			for _, p := range tc.publications {
				objects = append(objects, testPublicationCR(t, ns, p.volume, p.node, p.labeled))
			}
			c := &Client{tridentClient: tridentfake.NewSimpleClientset(objects...), tridentNamespace: ns}

			queryNode := node
			if tc.queryNode != "" {
				queryNode = tc.queryNode
			}
			desired, err := c.GetDesiredPublications(context.Background(), queryNode)
			require.NoError(t, err)

			gotVolumeNames := make([]string, 0, len(desired))
			for volumeName, pub := range desired {
				assert.Equal(t, volumeName, pub.VolumeName)
				assert.Equal(t, queryNode, pub.NodeName)
				gotVolumeNames = append(gotVolumeNames, volumeName)
			}
			sort.Strings(gotVolumeNames)
			assert.Equal(t, tc.wantVolumeNames, gotVolumeNames)
		})
	}
}

func TestClient_GetDesiredPublications_ListError(t *testing.T) {
	const ns = "trident"

	tridentClient := tridentfake.NewSimpleClientset()
	tridentClient.PrependReactor("list", "tridentvolumepublications",
		func(k8stesting.Action) (bool, k8sruntime.Object, error) {
			return true, nil, fmt.Errorf("apiserver unavailable")
		})
	c := &Client{tridentClient: tridentClient, tridentNamespace: ns}

	desired, err := c.GetDesiredPublications(context.Background(), "node-a")

	assert.Error(t, err)
	assert.ErrorContains(t, err, "list volume publications")
	assert.ErrorContains(t, err, "apiserver unavailable")
	assert.Nil(t, desired)
}
