// Copyright 2026 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"

	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func testVolumeCR(name, namespace string, configRaw []byte) *tridentv1.TridentVolume {
	return &tridentv1.TridentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Config:     k8sruntime.RawExtension{Raw: configRaw},
	}
}

func namesPtr(names []string) *[]string { return &names }

func onlyPatchActions(actions []k8stesting.Action) []k8stesting.PatchAction {
	var patches []k8stesting.PatchAction
	for _, action := range actions {
		if patch, ok := action.(k8stesting.PatchAction); ok {
			patches = append(patches, patch)
		}
	}
	return patches
}

// TestClient_UpdateVolume_PreservesUnknownConfigKeys is the regression test that justifies the
// scoped-merge-patch design over a full VolumeConfig decode/re-encode: a config key this test
// binary's storage.VolumeConfig doesn't recognize (as if written by a newer controller) must
// survive a node's write.
func TestClient_UpdateVolume_PreservesUnknownConfigKeys(t *testing.T) {
	const ns = "trident"
	configRaw, err := json.Marshal(map[string]any{
		"someFutureField":     "x",
		"luksPassphraseNames": []string{"a"},
	})
	require.NoError(t, err)
	tridentClient := tridentfake.NewSimpleClientset(testVolumeCR("vol1", ns, configRaw))
	c := &Client{tridentClient: tridentClient, tridentNamespace: ns}

	err = c.UpdateVolume(context.Background(), "vol1", &models.NodeVolumeUpdate{LUKSPassphraseNames: namesPtr([]string{"a", "b"})})
	require.NoError(t, err)

	updated, err := tridentClient.TridentV1().TridentVolumes(ns).Get(context.Background(), "vol1", metav1.GetOptions{})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(updated.Config.Raw, &decoded))
	assert.Equal(t, "x", decoded["someFutureField"], "unknown config keys must survive the write")
	assert.Equal(t, []any{"a", "b"}, decoded["luksPassphraseNames"])
}

func TestClient_UpdateVolume_NilOrEmptyUpdateIsNoOp(t *testing.T) {
	const ns = "trident"
	tridentClient := tridentfake.NewSimpleClientset(testVolumeCR("vol1", ns, []byte(`{}`)))
	c := &Client{tridentClient: tridentClient, tridentNamespace: ns}

	require.NoError(t, c.UpdateVolume(context.Background(), "vol1", nil))
	require.NoError(t, c.UpdateVolume(context.Background(), "vol1", &models.NodeVolumeUpdate{}))

	assert.Empty(t, tridentClient.Actions(), "an empty update must not even Get the CR")
}

func TestClient_UpdateVolume_UnchangedValueSuppressesWrite(t *testing.T) {
	const ns = "trident"
	configRaw, err := json.Marshal(map[string]any{"luksPassphraseNames": []string{"a"}})
	require.NoError(t, err)
	tridentClient := tridentfake.NewSimpleClientset(testVolumeCR("vol1", ns, configRaw))
	c := &Client{tridentClient: tridentClient, tridentNamespace: ns}

	err = c.UpdateVolume(context.Background(), "vol1", &models.NodeVolumeUpdate{LUKSPassphraseNames: namesPtr([]string{"a"})})
	require.NoError(t, err)

	assert.NotEmpty(t, tridentClient.Actions(), "the Get must still happen")
	assert.Empty(t, onlyPatchActions(tridentClient.Actions()), "no patch action expected for a no-op")
}

func TestClient_UpdateVolume_RealChangeSetsExactlyAllowlistedKeys(t *testing.T) {
	const ns = "trident"
	configRaw, err := json.Marshal(map[string]any{
		"luksPassphraseNames": []string{"a"}, "size": "1Gi",
	})
	require.NoError(t, err)
	tridentClient := tridentfake.NewSimpleClientset(testVolumeCR("vol1", ns, configRaw))
	c := &Client{tridentClient: tridentClient, tridentNamespace: ns}

	err = c.UpdateVolume(context.Background(), "vol1", &models.NodeVolumeUpdate{LUKSPassphraseNames: namesPtr([]string{"a", "b"})})
	require.NoError(t, err)

	patches := onlyPatchActions(tridentClient.Actions())
	require.Len(t, patches, 1, "expected exactly one patch action")
	assert.Equal(t, types.MergePatchType, patches[0].GetPatchType())

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(patches[0].GetPatch(), &decoded))
	assert.Equal(t, map[string]any{
		"config": map[string]any{
			"luksPassphraseNames": []any{"a", "b"},
		},
	}, decoded, "patch body must set only the allowlisted keys")
}

func TestClient_UpdateVolume_MixedCaseNameIsNameFixed(t *testing.T) {
	const ns = "trident"
	// TridentVolume CR names are always lowercase (NameFix); seed the fixed name and call with a
	// mixed-case volume name to confirm the client fixes it before looking the CR up.
	tridentClient := tridentfake.NewSimpleClientset(testVolumeCR("vol-mixedcase", ns, []byte(`{}`)))
	c := &Client{tridentClient: tridentClient, tridentNamespace: ns}

	err := c.UpdateVolume(context.Background(), "Vol-MixedCase", &models.NodeVolumeUpdate{LUKSPassphraseNames: namesPtr([]string{"a"})})

	assert.NoError(t, err)
}

func TestClient_UpdateVolume_NotFound(t *testing.T) {
	const ns = "trident"
	tridentClient := tridentfake.NewSimpleClientset()
	c := &Client{tridentClient: tridentClient, tridentNamespace: ns}

	err := c.UpdateVolume(context.Background(), "does-not-exist", &models.NodeVolumeUpdate{LUKSPassphraseNames: namesPtr([]string{"a"})})

	assert.Error(t, err)
	assert.True(t, errors.IsNotFoundError(err))
}

func TestClient_UpdateVolume_GetErrorPropagated(t *testing.T) {
	const ns = "trident"
	tridentClient := tridentfake.NewSimpleClientset()
	tridentClient.Fake.PrependReactor("get", "tridentvolumes", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, fmt.Errorf("boom")
	})
	c := &Client{tridentClient: tridentClient, tridentNamespace: ns}

	err := c.UpdateVolume(context.Background(), "vol1", &models.NodeVolumeUpdate{LUKSPassphraseNames: namesPtr([]string{"a"})})

	assert.Error(t, err)
	assert.False(t, errors.IsNotFoundError(err))
}

func TestClient_UpdateVolume_PatchErrorPropagated(t *testing.T) {
	const ns = "trident"
	tridentClient := tridentfake.NewSimpleClientset(testVolumeCR("vol1", ns, []byte(`{}`)))
	tridentClient.Fake.PrependReactor("patch", "tridentvolumes", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, fmt.Errorf("boom")
	})
	c := &Client{tridentClient: tridentClient, tridentNamespace: ns}

	err := c.UpdateVolume(context.Background(), "vol1", &models.NodeVolumeUpdate{LUKSPassphraseNames: namesPtr([]string{"a"})})

	assert.Error(t, err)
}
