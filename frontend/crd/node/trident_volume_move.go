// Copyright 2026 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

const volumeMoveContextTimeout = 90 * time.Second

func (c *TridentNodeCrdController) handleTridentVolumeMove(keyItem *KeyItem) error {
	key := keyItem.key
	ctx := keyItem.ctx

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logc(ctx).WithField("key", key).Error("Invalid key")
		return nil
	}

	volumeMove, err := c.tridentVolumeMoveLister.TridentVolumeMoves(namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			Logc(ctx).WithField("key", key).Debug("Object in work queue no longer exists")
			return nil
		}
		return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}
	if len(volumeMove.Status.Attachments) == 0 {
		Logc(ctx).WithField("key", key).Debug("TVM has no attachments specified.")
		return errors.ReconcileDeferredError("TVM has no attachments specified")
	}
	if volumeMove.Status.TargetAccessInfo == nil {
		Logc(ctx).WithField("key", key).Debug("TVM has no access info.")
		return errors.ReconcileDeferredError("TVM has no access info")
	}

	tvm := volumeMove.DeepCopy()
	tvmName := tvm.Name
	tvmState := tvm.Status.State
	attachStatus := tvm.Status.AttachmentStatus(c.nodeName)
	if attachStatus == nil {
		Logc(ctx).Warn("Volume move NodeStaging does not include this node; " +
			"node staging will be skipped on this daemon. " +
			"Confirm CSI node name (--csi_node_name / KUBE_NODE_NAME) matches Trident volume publication node IDs.")
		return nil
	}
	attachState := attachStatus.State

	fields := LogFields{
		"tvmName":     tvmName,
		"tvmState":    tvmState,
		"attachState": attachState,
		"nodeName":    c.nodeName,
	}

	var handleErr error
	ctx, cancel := context.WithTimeout(ctx, volumeMoveContextTimeout)
	defer cancel()

	switch attachState {
	case models.VolumeMoveAttachmentStatePending:
		if tvmState != models.VolumeMoveStateNodeStaging {
			Logc(ctx).WithFields(fields).Tracef("Attachment state cannot transition while TVM is in the %q state", tvmState)
			return errors.ReconcileDeferredError("attachment state cannot transition while TVM is in the %q state; requeuing...", tvmState)
		}

		if err := c.handleTridentVolumeMoveNodeStaging(ctx, tvm); err != nil {
			attachStatus.Message = err.Error()
			if c.isTerminalError(err) {
				attachStatus.State = models.VolumeMoveAttachmentStateFailed
			} else {
				handleErr = errors.WrapWithReconcileDeferredError(err, "node staging failed")
			}
			break
		}
		attachStatus.Message = "Successfully bridged attachment"
		attachStatus.State = models.VolumeMoveAttachmentStateBridged

	case models.VolumeMoveAttachmentStateBridged:
		if tvmState != models.VolumeMoveStateNodeUnstaging {
			Logc(ctx).WithFields(fields).Tracef("Attachment state cannot transition while TVM is in the %q state", tvmState)
			return errors.ReconcileDeferredError("attachment state cannot transition while TVM is in the %q state; requeuing...", tvmState)
		}

		if err := c.handleTridentVolumeMoveNodeUnstaging(ctx, tvm); err != nil {
			attachStatus.Message = err.Error()
			if c.isTerminalError(err) {
				attachStatus.State = models.VolumeMoveAttachmentStateFailed
			} else {
				handleErr = errors.WrapWithReconcileDeferredError(err, "node unstaging failed")
			}
			break
		}
		attachStatus.Message = "Successfully migrated attachment"
		attachStatus.State = models.VolumeMoveAttachmentStateMigrated

	case models.VolumeMoveAttachmentStateDetached:
		Logc(ctx).WithFields(fields).Warn("Volume was detached during a volume move; removing stale attachment.")
		if err := c.handleTridentVolumeMoveNodeDetachment(ctx, tvm); err != nil {
			attachStatus.Message = fmt.Sprintf("Could not cleanup stale attachment for volume %q; error: %s", tvm.Name, err.Error())
			attachStatus.State = models.VolumeMoveAttachmentStateFailed
			break
		}
		attachStatus.Message = "Attachment detached during an active move; stale attachment removed."
		attachStatus.State = models.VolumeMoveAttachmentStateCleaned

	case models.VolumeMoveAttachmentStateMigrated, models.VolumeMoveAttachmentStateCleaned:
		return nil

	case models.VolumeMoveAttachmentStateFailed:
		return errors.TerminalReconciliationError("updating volume attachment failed; halting volume move.")
	}

	if err := c.updateTridentVolumeMoveAttachmentStatus(ctx, tvm); err != nil {
		handleErr = errors.Join(handleErr, err)
	}

	return handleErr
}

func (c *TridentNodeCrdController) handleTridentVolumeMoveNodeStaging(ctx context.Context, tvm *v1.TridentVolumeMove) error {
	fields := LogFields{
		"tvmName":  tvm.Name,
		"nodeName": c.nodeName,
	}
	Logc(ctx).WithFields(fields).Info("Running volume move node staging (grafting attachments).")

	if tvm.Status.TargetAccessInfo == nil {
		Logc(ctx).WithFields(fields).Error("Cannot run node staging: status targetAccessInfo is nil.")
		return errors.InvalidInputError("tvm %q status targetAccessInfo should not be nil", tvm.Name)
	}
	res, err := c.plugin.NodeGraftAttachment(ctx, &models.GraftAttachmentRequest{
		VolumeAccessInfo: *tvm.Status.TargetAccessInfo,
		VolumeName:       tvm.Name,
		Protocol:         config.Block,
	})
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to graft volume attachment.")
		return err
	}

	Logc(ctx).WithFields(fields).WithField("graftAttachmentResponse", res).Debug("Grafted volume attachment.")
	return nil
}

func (c *TridentNodeCrdController) handleTridentVolumeMoveNodeUnstaging(ctx context.Context, tvm *v1.TridentVolumeMove) error {
	fields := LogFields{
		"tvmName":  tvm.Name,
		"nodeName": c.nodeName,
	}

	if tvm.Status.TargetAccessInfo == nil {
		Logc(ctx).WithFields(fields).Error("Cannot run node unstaging: status targetAccessInfo is nil.")
		return errors.InvalidInputError("tvm %q status targetAccessInfo should not be nil", tvm.Name)
	}
	Logc(ctx).WithFields(fields).Info("Running volume move node unstaging (pruning attachments).")

	res, err := c.plugin.NodePruneAttachment(ctx, &models.PruneAttachmentRequest{
		VolumeAccessInfo: *tvm.Status.TargetAccessInfo,
		VolumeName:       tvm.Name,
		Protocol:         config.Block,
	})
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to prune volume attachment.")
		return err
	}

	Logc(ctx).WithFields(fields).WithField("pruneAttachmentResponse", res).Debug("Pruned volume attachment.")
	return nil
}

// handleTridentVolumeMoveNodeDetachment is a special handler for the case where a volume attachment
// has been detached at the orchestrator level during a TVM lifecycle.
// This is called only when the TVM attachment state is Detached and any priorly grafted attachment artifacts need to be removed.
func (c *TridentNodeCrdController) handleTridentVolumeMoveNodeDetachment(ctx context.Context, tvm *v1.TridentVolumeMove) error {
	fields := LogFields{
		"tvmName":           tvm.Name,
		"nodeName":          c.nodeName,
		"targetAccessInfo":  tvm.Status.TargetAccessInfo,
		"initialAccessInfo": tvm.Status.InitialAccessInfo,
	}
	Logc(ctx).WithFields(fields).Warn("Cleanup may be required. Attempting to automatically remove any stale attachment.")

	var errs error
	if tvm.Status.TargetAccessInfo != nil {
		Logc(ctx).WithFields(fields).Debug("Running node unstaging for status targetAccessInfo.")
		_, pruneTargetAccessInfoError := c.plugin.NodePruneAttachment(ctx, &models.PruneAttachmentRequest{
			VolumeAccessInfo: *tvm.Status.TargetAccessInfo,
			VolumeName:       tvm.Name,
			Protocol:         config.Block,
		})
		if pruneTargetAccessInfoError != nil {
			Logc(ctx).WithError(pruneTargetAccessInfoError).Warn("Failed to prune target access info from attachment.")
			errs = errors.Join(errs, pruneTargetAccessInfoError)
		}
	}

	if tvm.Status.InitialAccessInfo != nil {
		Logc(ctx).WithFields(fields).Debug("Running node unstaging for status initialAccessInfo.")
		_, pruneInitialAccessInfoError := c.plugin.NodePruneAttachment(ctx, &models.PruneAttachmentRequest{
			VolumeAccessInfo: *tvm.Status.InitialAccessInfo,
			VolumeName:       tvm.Name,
			Protocol:         config.Block,
		})
		if pruneInitialAccessInfoError != nil {
			Logc(ctx).WithError(pruneInitialAccessInfoError).Warn("Failed to prune initial access info from attachment.")
			errs = errors.Join(errs, pruneInitialAccessInfoError)
		}
	}

	return errs
}

func (c *TridentNodeCrdController) updateTridentVolumeMoveAttachmentStatus(
	ctx context.Context, tvm *v1.TridentVolumeMove,
) error {
	fields := LogFields{
		"tvmName":  tvm.Name,
		"nodeName": c.nodeName,
		"tvmState": tvm.Status.State,
	}
	Logx(ctx).WithFields(fields).Info("Updating TridentVolumeMove status from node.")

	localAttach := tvm.Status.AttachmentStatus(c.nodeName)
	if localAttach == nil {
		return errors.NotFoundError("attachment for node %q not found in TVM %q", c.nodeName, tvm.Name)
	}

	// Multiple controllers update the status concurrently. A 409 Conflict
	// (resourceVersion mismatch) should be resolved with an immediate re-read
	// and retry rather than a full reconcile cycle through the work queue.
	//
	// RetryOnConflict only retries errors where apierrors.IsConflict is true,
	// so the closure must return raw API errors - not wrapped ones.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fresh, err := c.crdClientset.TridentV1().TridentVolumeMoves(tvm.Namespace).Get(ctx, tvm.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		freshAttach := fresh.Status.AttachmentStatus(c.nodeName)
		if freshAttach == nil {
			return errors.NotFoundError("attachment for node %q not found in fresh TVM %q", c.nodeName, tvm.Name)
		}
		freshAttach.State = localAttach.State
		freshAttach.Message = localAttach.Message

		_, err = c.crdClientset.TridentV1().TridentVolumeMoves(fresh.Namespace).UpdateStatus(ctx, fresh, metav1.UpdateOptions{})
		return err
	})

	if err != nil {
		if apierrors.IsNotFound(err) {
			Logc(ctx).WithField("name", tvm.Name).Error("TridentVolumeMove resource no longer exists.")
			return errors.NotFoundError(err.Error())
		}
		return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}
	return nil
}

func (c *TridentNodeCrdController) isTerminalError(err error) bool {
	// If an auth error bubbles up to the CRD frontend,
	// consider it a terminal error. The only way this can happen
	// is if a graft continually fails to establish sessions.
	if errors.IsAuthError(err) {
		return true
	}
	if errors.IsTerminalReconciliationError(err) {
		return true
	}
	if errors.IsInvalidInputError(err) {
		return true
	}
	return false
}
