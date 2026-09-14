// Copyright 2026 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	clientgotesting "k8s.io/client-go/testing"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func luksNamesPtr(names []string) *[]string { return &names }

func newTestVolumeConfigWithLUKS(name string, luksNames []string) *storage.VolumeConfig {
	return &storage.VolumeConfig{
		Version:             config.OrchestratorAPIVersion,
		Name:                name,
		Size:                "1GB",
		Protocol:            config.File,
		StorageClass:        "gold",
		LUKSPassphraseNames: luksNames,
	}
}

// seedTestVolumeCR builds a CRDClientV1 backed by a fake clientset preloaded with a TridentVolume
// CR for volConfig, and returns the concrete Clientset alongside it so tests can inspect Actions().
func seedTestVolumeCR(t *testing.T, volConfig *storage.VolumeConfig) (*CRDClientV1, *Clientset) {
	t.Helper()

	vol := &storage.Volume{
		Config:      volConfig,
		BackendUUID: "test-backend-uuid",
		Pool:        storagePool,
	}
	cr, err := netappv1.NewTridentVolume(ctx(), vol.ConstructExternal())
	require.NoError(t, err)

	client := NewFakeClientset(cr)
	k8sclientFake, _ := k8sclient.NewFakeKubeClient()

	return &CRDClientV1{
		crdClient: client,
		k8sClient: k8sclientFake,
		version: &config.PersistentStateVersion{
			PersistentStoreVersion: string(CRDV1Store),
			OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		},
	}, client
}

func assertNoPatchAction(t *testing.T, client *Clientset) {
	t.Helper()
	for _, action := range client.Actions() {
		if _, ok := action.(clientgotesting.PatchAction); ok {
			t.Fatalf("expected no patch action, got %#v", action)
		}
	}
}

// TestCRDClientV1_UpdateVolume_PreservesNodeConfig is the regression test for merge-preserve:
// a controller-side write built from a config that has never learned about the node's LUKS fields
// (e.g. resize, autogrow, any cache staleness) must not clobber them.
func TestCRDClientV1_UpdateVolume_PreservesNodeConfig(t *testing.T) {
	seeded := newTestVolumeConfigWithLUKS("vol1", []string{"A"})
	p, _ := seedTestVolumeCR(t, seeded)

	staleConfig := newTestVolumeConfigWithLUKS("vol1", nil)
	staleConfig.Size = "2GB"
	updateVol := &storage.Volume{Config: staleConfig, BackendUUID: "test-backend-uuid", Pool: storagePool}

	err := p.UpdateVolume(ctx(), updateVol)
	require.NoError(t, err)

	got, err := p.GetVolume(ctx(), "vol1")
	require.NoError(t, err)
	assert.Equal(t, "2GB", got.Config.Size, "the actual update must still take effect")
	assert.Equal(t, []string{"A"}, got.Config.LUKSPassphraseNames,
		"node-owned field must survive a controller write built from a stale cache")
}

// TestCRDClientV1_UpdateVolume_NoNodeConfigIsUnaffected confirms merge-preserve is a true
// no-op when the volume has never had node-owned fields written.
func TestCRDClientV1_UpdateVolume_NoNodeConfigIsUnaffected(t *testing.T) {
	seeded := newTestVolumeConfigWithLUKS("vol1", nil)
	p, _ := seedTestVolumeCR(t, seeded)

	updateConfig := newTestVolumeConfigWithLUKS("vol1", nil)
	updateConfig.Size = "9GB"
	updateVol := &storage.Volume{Config: updateConfig, BackendUUID: "test-backend-uuid", Pool: storagePool}

	err := p.UpdateVolume(ctx(), updateVol)
	require.NoError(t, err)

	got, err := p.GetVolume(ctx(), "vol1")
	require.NoError(t, err)
	assert.Equal(t, "9GB", got.Config.Size)
	assert.Empty(t, got.Config.LUKSPassphraseNames)
}

func TestCRDClientV1_UpdateVolumeNodeConfig_NotFound(t *testing.T) {
	p, _ := GetTestKubernetesClient()

	err := p.UpdateVolumeNodeConfig(ctx(), "does-not-exist", &models.NodeVolumeUpdate{LUKSPassphraseNames: luksNamesPtr([]string{"a"})})

	require.Error(t, err)
	assert.True(t, tridenterrors.IsNotFoundError(err))
}

func TestCRDClientV1_UpdateVolumeNodeConfig_EmptyUpdateIsNoOp(t *testing.T) {
	seeded := newTestVolumeConfigWithLUKS("vol1", []string{"A"})
	p, client := seedTestVolumeCR(t, seeded)

	err := p.UpdateVolumeNodeConfig(ctx(), "vol1", &models.NodeVolumeUpdate{})

	require.NoError(t, err)
	assertNoPatchAction(t, client)
}

func TestCRDClientV1_UpdateVolumeNodeConfig_UnchangedIsNoOp(t *testing.T) {
	seeded := newTestVolumeConfigWithLUKS("vol1", []string{"A"})
	p, client := seedTestVolumeCR(t, seeded)

	err := p.UpdateVolumeNodeConfig(ctx(), "vol1", &models.NodeVolumeUpdate{LUKSPassphraseNames: luksNamesPtr([]string{"A"})})

	require.NoError(t, err)
	assertNoPatchAction(t, client)

	got, err := p.GetVolume(ctx(), "vol1")
	require.NoError(t, err)
	assert.Equal(t, []string{"A"}, got.Config.LUKSPassphraseNames, "the stored value must be unchanged")
}

func TestCRDClientV1_UpdateVolumeNodeConfig_Changed(t *testing.T) {
	seeded := newTestVolumeConfigWithLUKS("vol1", []string{"A"})
	seeded.Size = "5GB"
	p, client := seedTestVolumeCR(t, seeded)

	err := p.UpdateVolumeNodeConfig(ctx(), "vol1", &models.NodeVolumeUpdate{LUKSPassphraseNames: luksNamesPtr([]string{"A", "B"})})
	require.NoError(t, err)

	var patchActions int
	for _, action := range client.Actions() {
		if patchAction, ok := action.(clientgotesting.PatchAction); ok {
			patchActions++
			assert.Equal(t, types.MergePatchType, patchAction.GetPatchType())
		}
	}
	assert.Equal(t, 1, patchActions, "expected exactly one patch action")

	got, err := p.GetVolume(ctx(), "vol1")
	require.NoError(t, err)
	assert.Equal(t, []string{"A", "B"}, got.Config.LUKSPassphraseNames)
	assert.Equal(t, "5GB", got.Config.Size, "unrelated fields must be untouched by the scoped patch")
}

func TestCRDClientV1_UpdateVolumeNodeConfig_UnknownConfigKeysSurvive(t *testing.T) {
	seeded := newTestVolumeConfigWithLUKS("vol1", []string{"A"})
	p, _ := seedTestVolumeCR(t, seeded)

	// Simulate a newer controller having written a config key this test binary's storage.VolumeConfig
	// doesn't (yet) know about.
	cr, err := p.crdClient.TridentV1().TridentVolumes(p.namespace).Get(ctx(), netappv1.NameFix("vol1"), getOpts)
	require.NoError(t, err)
	cr = cr.DeepCopy()
	raw := append([]byte(nil), cr.Config.Raw[:len(cr.Config.Raw)-1]...)
	raw = append(raw, []byte(`,"someFutureField":"x"}`)...)
	cr.Config.Raw = raw
	_, err = p.crdClient.TridentV1().TridentVolumes(p.namespace).Update(ctx(), cr, updateOpts)
	require.NoError(t, err)

	err = p.UpdateVolumeNodeConfig(ctx(), "vol1", &models.NodeVolumeUpdate{LUKSPassphraseNames: luksNamesPtr([]string{"A", "B"})})
	require.NoError(t, err)

	updated, err := p.crdClient.TridentV1().TridentVolumes(p.namespace).Get(ctx(), netappv1.NameFix("vol1"), getOpts)
	require.NoError(t, err)
	assert.Contains(t, string(updated.Config.Raw), `"someFutureField":"x"`, "unknown config keys must survive a merge patch")
}
