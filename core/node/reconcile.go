// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"slices"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/utils/models"
)

// ReconcileAttachmentRequest carries the desired data addresses for one published volume.
// It names no storage protocol; Prune and Graft decide that from the volume's tracking info.
type ReconcileAttachmentRequest struct {
	TargetIPs []string
}

// ReconcileAttachment converges one published volume onto a backend's data LIF snapshot.
// Prune and Graft read the tracking file under the volume lock and apply the target IPs to an
// NVMe attachment; a volume that is not tracked as NVMe is left alone. An empty target set
// prunes obsolete paths and does not graft.
func (c *Core) ReconcileAttachment(ctx context.Context, volumeID string, req ReconcileAttachmentRequest) error {
	accessInfo := models.VolumeAccessInfo{
		NVMeAccessInfo: models.NVMeAccessInfo{NVMeTargetIPs: slices.Clone(req.TargetIPs)},
	}
	if _, err := c.Prune(ctx, volumeID, PruneRequest{
		VolumeAccessInfo: accessInfo,
		Protocol:         tridentconfig.Block,
	}); err != nil {
		return err
	}
	if len(req.TargetIPs) == 0 {
		return nil
	}
	_, err := c.Graft(ctx, volumeID, GraftRequest{
		VolumeAccessInfo: accessInfo,
		Protocol:         tridentconfig.Block,
	})
	return err
}
