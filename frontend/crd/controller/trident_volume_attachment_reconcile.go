// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"fmt"
	"strings"

	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"

	"github.com/netapp/trident/frontend/csi"
	. "github.com/netapp/trident/logging"
	storageattribute "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
)

// volumeAttachmentTargetIPHandler maps a SAN protocol to the VolumeAttachment status
// metadata key that stores its comma-joined target IPs. Register a protocol here to
// have backend data-LIF changes written into attached VolumeAttachments of that type.
// Protocols that encode targets differently (for example iSCSI portal lists) can grow
// dedicated encode/apply helpers later without changing the reconcile loop.
type volumeAttachmentTargetIPHandler struct {
	sanType     string
	metadataKey string
}

func (h volumeAttachmentTargetIPHandler) encode(dataLIFs []string) string {
	return strings.Join(dataLIFs, ",")
}

func (h volumeAttachmentTargetIPHandler) current(attachment *storagev1.VolumeAttachment) string {
	if attachment == nil || attachment.Status.AttachmentMetadata == nil {
		return ""
	}
	return attachment.Status.AttachmentMetadata[h.metadataKey]
}

func (h volumeAttachmentTargetIPHandler) apply(attachment *storagev1.VolumeAttachment, dataLIFs []string) {
	if attachment.Status.AttachmentMetadata == nil {
		attachment.Status.AttachmentMetadata = make(map[string]string)
	}
	attachment.Status.AttachmentMetadata[h.metadataKey] = h.encode(dataLIFs)
}

// CSI publish-context keys for protocols that store target IPs as a single comma-joined value.
const nvmeTargetIPsMetadataKey = "nvmeTargetIPs"

var volumeAttachmentTargetIPHandlers = map[string]volumeAttachmentTargetIPHandler{
	storageattribute.NVMe: {
		sanType:     storageattribute.NVMe,
		metadataKey: nvmeTargetIPsMetadataKey,
	},
}

func targetIPHandlerForAttachment(attachment *storagev1.VolumeAttachment) (volumeAttachmentTargetIPHandler, bool) {
	if attachment == nil || len(attachment.Status.AttachmentMetadata) == 0 {
		return volumeAttachmentTargetIPHandler{}, false
	}
	handler, ok := volumeAttachmentTargetIPHandlers[attachment.Status.AttachmentMetadata["SANType"]]
	return handler, ok
}

// handleBackendDataLIFsChange reconciles the data LIFs the core published on a TridentBackend
// into attached Trident VolumeAttachments for that backend, for every registered SAN protocol.
// The LIFs are read from the informer cache at work time, so a backend that changed repeatedly
// before the worker ran is reconciled to its latest snapshot. Node tracking and host-path healing
// remain node-owned.
func (c *TridentCrdController) handleBackendDataLIFsChange(ctx context.Context, backendUUID string) error {
	if backendUUID == "" {
		return fmt.Errorf("backend UUID is empty")
	}

	dataLIFs, err := c.publishedTridentBackendDataLIFs(backendUUID)
	if err != nil {
		return errors.ReconcileDeferredError(
			"could not read TridentBackend data LIFs for backend %s: %v", backendUUID, err)
	}
	if dataLIFs == nil {
		return nil
	}

	vaIndexer := c.indexers.VolumeAttachmentIndexer()
	if !vaIndexer.WaitForCacheSync(ctx) {
		return errors.ReconcileDeferredError("volume attachment cache is not synced")
	}

	publications, err := c.volumePublicationsLister.List(labels.Everything())
	if err != nil {
		return errors.ReconcileDeferredError("could not list volume publications: %v", err)
	}

	seen := make(map[string]struct{})
	for _, publication := range publications {
		if publication == nil || publication.BackendUUID != backendUUID ||
			publication.VolumeID == "" || publication.NodeID == "" {
			continue
		}

		publicationKey := publication.VolumeID + "\x00" + publication.NodeID
		if _, ok := seen[publicationKey]; ok {
			continue
		}
		seen[publicationKey] = struct{}{}

		attachments, err := vaIndexer.GetCachedVolumeAttachmentsByVolume(ctx, publication.VolumeID)
		if err != nil {
			return errors.ReconcileDeferredError(
				"could not list volume attachments for volume %s: %v", publication.VolumeID, err)
		}

		for _, attachment := range attachments {
			if !isAttachedTridentVolumeAttachment(attachment, publication.VolumeID, publication.NodeID) {
				continue
			}
			handler, ok := targetIPHandlerForAttachment(attachment)
			if !ok || handler.current(attachment) == handler.encode(dataLIFs) {
				continue
			}

			if err := c.updateTridentVolumeAttachmentTargetIPs(
				ctx, attachment.Name, publication.VolumeID, publication.NodeID, dataLIFs,
			); err != nil {
				return errors.ReconcileDeferredError(
					"could not update volume attachment %s: %v", attachment.Name, err)
			}
		}
	}

	return nil
}

func isAttachedTridentVolumeAttachment(
	attachment *storagev1.VolumeAttachment, volumeID, nodeID string,
) bool {
	return attachment != nil && attachment.Spec.Attacher == csi.Provisioner &&
		attachment.Status.Attached && len(attachment.Status.AttachmentMetadata) > 0 &&
		attachment.Spec.NodeName == nodeID &&
		attachment.Spec.Source.PersistentVolumeName != nil &&
		*attachment.Spec.Source.PersistentVolumeName == volumeID
}

func (c *TridentCrdController) updateTridentVolumeAttachmentTargetIPs(
	ctx context.Context, attachmentName, volumeID, nodeID string, dataLIFs []string,
) error {
	updated := false
	var sanType, metadataKey string

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		attachment, err := c.kubeClientset.StorageV1().VolumeAttachments().Get(
			ctx, attachmentName, metav1.GetOptions{},
		)
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !isAttachedTridentVolumeAttachment(attachment, volumeID, nodeID) {
			return nil
		}
		handler, ok := targetIPHandlerForAttachment(attachment)
		if !ok {
			return nil
		}
		desired := handler.encode(dataLIFs)
		if handler.current(attachment) == desired {
			return nil
		}

		handler.apply(attachment, dataLIFs)
		if _, err = c.kubeClientset.StorageV1().VolumeAttachments().UpdateStatus(
			ctx, attachment, metav1.UpdateOptions{},
		); err != nil {
			return err
		}
		updated = true
		sanType = handler.sanType
		metadataKey = handler.metadataKey
		return nil
	})
	if err != nil {
		return err
	}

	if updated {
		Logc(ctx).WithFields(LogFields{
			"volumeAttachment": attachmentName,
			"volume":           volumeID,
			"node":             nodeID,
			"sanType":          sanType,
			"metadataKey":      metadataKey,
			"dataLIFs":         dataLIFs,
		}).Info("Updated target IPs in VolumeAttachment.")
	}
	return nil
}
