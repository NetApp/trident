// Copyright 2026 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// UpdateVolume applies node-originated fields to the TridentVolume CR. It reads the CR to no-op
// cheaply when nothing has changed, then writes back with a JSON merge patch scoped to just the
// allowlisted config keys - never a full VolumeConfig
// decode/re-encode, so config keys this binary's struct doesn't recognize survive the write even
// across a version-skewed rolling upgrade (node and controller images at different versions during
// a rolling DaemonSet/Deployment upgrade).
//
// This intentionally does not use client-go's retry.RetryOnConflict/Update() pattern the approved
// RWX LUKS Design doc describes, even though it targets the same "Config is a shared field, no
// status subresource" hazard. Two invariants make a scoped merge patch equivalent in safety without
// the whole-config round-trip risk: (1) the design doc's per-volume Lease means at most one
// legitimate writer of these keys exists at any time, so there is no concurrent writer for a retry
// loop here to race against; (2) a concurrent controller-side write (resize, autogrow, ...) is
// handled on the controller's side of the wire by merge-preserve in persistent_store
// (CRDClientV1.UpdateVolume), not by CAS here.
func (c *Client) UpdateVolume(ctx context.Context, volumeName string, update *models.NodeVolumeUpdate) error {
	if update.IsEmpty() {
		return nil
	}
	if err := c.ensureTridentClient(); err != nil {
		return err
	}

	name := tridentv1.NameFix(volumeName)
	cr, err := c.tridentClient.TridentV1().TridentVolumes(c.tridentNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return errors.NotFoundError("volume %s not found", volumeName)
		}
		return err
	}

	patch, changed, err := tridentv1.NodeVolumeUpdateMergePatch(cr.Config.Raw, update)
	if err != nil {
		return err
	}
	if !changed {
		// GET is required to compare against the live CR's current values. PATCH is skipped when
		// the allowlisted keys already match, so extra RWX nodes that attach after the first
		// writer do not each generate another API write.
		return nil
	}

	_, err = c.tridentClient.TridentV1().TridentVolumes(c.tridentNamespace).Patch(
		ctx, name, types.MergePatchType, patch, metav1.PatchOptions{},
	)
	return err
}
