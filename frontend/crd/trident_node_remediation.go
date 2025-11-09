// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

const (
	listLimit = 100
)

// handleNodeRemediation handles the node remediation action and keeps the CR up to date
func (c *TridentCrdController) handleTridentNodeRemediation(keyItem *KeyItem) (remediationErr error) {
	key := keyItem.key
	ctx := keyItem.ctx

	Logc(ctx).Debug(">>>> TridentCrdController#handleNodeRemediation")
	defer Logc(ctx).Debug("<<<< TridentCrdController#handleNodeRemediation")

	// Ensure the volume attachment indexer cache is synced
	if ok := c.indexers.VolumeAttachmentIndexer().WaitForCacheSync(ctx); !ok {
		Logc(ctx).Warn("Failed to sync volume attachment cache. Not all volume attachments may be found.")
	}

	// Convert the namespace/name string into a distinct namespace and name.
	// If this fails, no retry is likely to succeed, so return the error to forget this action.
	// We can't determine the CR, so no CR update is possible.
	// The namespace should be in the Trident namespace because it is an admin action.
	// The name of the CR should be the node name.
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logc(ctx).WithField("key", key).Error("Invalid key.")
		return err
	}

	// Ensure the CR is new and valid. This may return ReconcileDeferred if it should be retried.
	// Any other error returned here indicates a problem with the CR (i.e. it's not new or has been
	// deleted), so no CR update is needed.
	actionCR, err := c.validateTridentNodeRemediationCR(ctx, namespace, name)
	if err != nil {
		return err
	}

	// Ensure that the force detach feature flag is enabled
	// If not enabled, update the CR status to failed and return error
	if !c.enableForceDetach {
		Logc(ctx).Warn("Force detach feature flag is not enabled; node remediation is not available.")

		// Update the CR status to failed
		updateStatus := &actionCR.Status
		updateStatus.State = netappv1.TridentActionStateFailed
		updateStatus.Message = "Node remediation requires '--enable-force-detach' flag to be enabled"
		completionTime := metav1.Now()
		updateStatus.CompletionTime = &completionTime
		// Update the CR and return error
		err = c.updateNodeRemediationCR(ctx, namespace, name, updateStatus)
		if err != nil {
			return err
		}
		return fmt.Errorf("node remediation requires '--enable-force-detach' flag to be enabled")
	}
	nodeName := actionCR.Name
	updateStatus := &actionCR.Status

	switch actionCR.Status.State {
	case netappv1.TridentNodeRemediatingState:
		// The action is already in progress. Check if it is done.
		Logc(ctx).Info("Trident node remediation state in progress.")
		// Check if the node is dirty
		node, err := c.orchestrator.GetNode(ctx, nodeName)
		if err != nil {
			Logc(ctx).WithField("node", nodeName).WithError(err).Error("Could not get node.")
			if errors.IsNotFoundError(err) {
				return c.handleNodeDeletedDuringRemediation(ctx, namespace, name, nodeName)
			}
			updateStatus.State = netappv1.TridentActionStateFailed
			updateStatus.Message = fmt.Sprintf("Could not get node, %s", err.Error())
			break
		} else if node == nil {
			Logc(ctx).Error("node is nil after GetNode")
			updateStatus.State = netappv1.TridentActionStateFailed
			updateStatus.Message = "node is nil after GetNode"
			break
		} else if node.PublicationState == models.NodeDirty {
			err = c.failoverDetach(ctx, actionCR, nodeName)
			if err != nil {
				Logc(ctx).WithField("node", nodeName).WithError(err).
					Error("Not all workloads removed from node, requeuing.")
				updateStatus.Message = fmt.Sprintf("Workloads not yet removed from node, %s", err.Error())
				remediationErr = errors.ReconcileDeferredError("workloads not yet removed from node, requeuing")
				break
			}

			Logc(ctx).Debugf("Node %s is dirty, checking for volume publications.", nodeName)
			pubsRemoved, err := c.areVolPublicationsRemoved(ctx, actionCR, nodeName)
			if err != nil {
				Logc(ctx).WithField("node", nodeName).WithError(err).Error("Could not get volume publications.")
				break
			}
			if !pubsRemoved {
				// Requeue to check again
				return errors.ReconcileDeferredError(fmt.Sprintf(
					"Node %s still has volume attachments to remove, requeuing", nodeName))
			}
			// no more volume attachments; remediation is complete; update node with admin ready true
			Logc(ctx).WithField("node", nodeName).Info("Volume attachments removed from node.")
			flags := &models.NodePublicationStateFlags{
				AdministratorReady: convert.ToPtr(true),
			}
			err = c.orchestrator.UpdateNode(ctx, nodeName, flags)
			if err != nil {
				return err
			}
			updateStatus.State = netappv1.NodeRecoveryPending

		} else if node.PublicationState == models.NodeClean {
			// If node is clean, set to dirty and start force detach work
			dirtyFlags := &models.NodePublicationStateFlags{
				OrchestratorReady:  convert.ToPtr(false),
				AdministratorReady: convert.ToPtr(false),
			}
			err = c.orchestrator.UpdateNode(ctx, nodeName, dirtyFlags)
			if err != nil {
				return err
			}
			return errors.ReconcileDeferredError("node set to dirty, requeuing")
		}
	case netappv1.NodeRecoveryPending:
		// There are no more volume publications that Trident can clean; we are now waiting for the node to be cleaned
		// by the publication reconcile loop so that the node may be reintroduced to the cluster.
		node, err := c.orchestrator.GetNode(ctx, nodeName)
		if err != nil {
			Logc(ctx).WithField("node", nodeName).WithError(err).Error("Could not get node.")
			if errors.IsNotFoundError(err) {
				return c.handleNodeDeletedDuringRemediation(ctx, namespace, name, nodeName)
			}
			updateStatus.State = netappv1.TridentActionStateFailed
			updateStatus.Message = fmt.Sprintf("Could not get node, %s", err.Error())
			break
		} else if node == nil {
			Logc(ctx).Error("node is nil after GetNode")
			updateStatus.State = netappv1.TridentActionStateFailed
			updateStatus.Message = "node is nil after GetNode"
			break
		} else if node.PublicationState == models.NodeClean {
			// If node is already dirty, we are waiting for remediation to complete; check for volume attachments to be gone
			// Node is clean and ready to receive publications again
			Logc(ctx).Debugf("Node %s is clean; remediation complete.", nodeName)
			updateStatus.State = netappv1.TridentActionStateSucceeded
		} else {
			Logc(ctx).Debugf("Node %s is not clean yet; remediation still in progress", nodeName)
			return errors.ReconcileDeferredError("node is not clean yet, requeuing")
		}
	case netappv1.TridentActionStateSucceeded, netappv1.TridentActionStateFailed:
		// State succeeded or failed; update completion time
		Logc(ctx).Infof("Trident node remediation state %v.", actionCR.Status.State)
		completionTime := metav1.Now()
		updateStatus.CompletionTime = &completionTime
	default:
		Logc(ctx).Infof("Trident node remediation state %v, updating to `remediating`.", actionCR.Status.State)
		updateStatus.State = netappv1.TridentNodeRemediatingState

	}

	err = c.updateNodeRemediationCR(ctx, namespace, name, updateStatus)
	if err != nil {
		return err
	}

	return remediationErr
}

func (c *TridentCrdController) validateTridentNodeRemediationCR(
	ctx context.Context, namespace, name string,
) (*netappv1.TridentNodeRemediation, error) {
	// Get the resource with this namespace/name
	actionCR, err := c.crdClientset.TridentV1().TridentNodeRemediations(namespace).Get(ctx, name, getOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Node remediation in work queue no longer exists.")
		return nil, err
	}

	if err != nil {
		return nil, errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}
	if actionCR.Status.VolumeAttachments == nil {
		actionCR.Status.VolumeAttachments = map[string]string{}
	}
	return actionCR, nil
}

// updateNodeRemediationCRWithErrorHandling performs a CR update and handles common error cases
func (c *TridentCrdController) updateNodeRemediationCRWithErrorHandling(
	ctx context.Context, cr *netappv1.TridentNodeRemediation, namespace string,
) (*netappv1.TridentNodeRemediation, error) {
	updatedCR, err := c.crdClientset.TridentV1().TridentNodeRemediations(namespace).Update(ctx, cr, updateOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Node remediation in work queue no longer exists.")
		return nil, err
	}
	if err != nil {
		return nil, errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}
	return updatedCR, nil
}

func (c *TridentCrdController) updateNodeRemediationCR(
	ctx context.Context, namespace, name string, statusUpdate *netappv1.TridentNodeRemediationStatus,
) error {
	// Get the resource with this namespace/name
	actionCR, err := c.crdClientset.TridentV1().TridentNodeRemediations(namespace).Get(ctx, name, getOpts)
	if apierrors.IsNotFound(err) {
		Logc(ctx).Debug("Node remediation in work queue no longer exists.")
		return err
	}
	if err != nil {
		return errors.WrapWithReconcileDeferredError(err, "reconcile deferred")
	}

	// Add finalizers for in-progress states
	if statusUpdate.State != netappv1.TridentActionStateSucceeded && statusUpdate.State != netappv1.TridentActionStateFailed {
		if !actionCR.HasTridentFinalizers() {
			actionCR.AddTridentFinalizers()
		}
	}

	actionCR.Status = *statusUpdate

	updatedCR, err := c.updateNodeRemediationCRWithErrorHandling(ctx, actionCR, namespace)
	if err != nil {
		return err
	}

	// For terminal states, remove finalizers in a separate update after
	// status is set, to let state update before NHC deletes CR
	if statusUpdate.State == netappv1.TridentActionStateSucceeded || statusUpdate.State == netappv1.TridentActionStateFailed {
		if updatedCR.HasTridentFinalizers() {
			updatedCR.RemoveTridentFinalizers()
			_, err = c.updateNodeRemediationCRWithErrorHandling(ctx, updatedCR, namespace)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *TridentCrdController) handleNodeDeletedDuringRemediation(ctx context.Context, namespace, name, nodeName string) error {
	Logc(ctx).WithField("node", nodeName).Info("Node deleted during remediation; deleting remediation CR.")
	delErr := c.crdClientset.TridentV1().TridentNodeRemediations(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if delErr != nil && !apierrors.IsNotFound(delErr) {
		Logc(ctx).WithError(delErr).Error("Failed to delete remediation CR after node deletion.")
		return delErr
	}
	return nil // CR deleted or already gone, nothing more to do
}

// areVolPublicationsRemoved returns true if all the expected volume publications are removed from the node.
func (c *TridentCrdController) areVolPublicationsRemoved(
	ctx context.Context, actionCR *netappv1.TridentNodeRemediation, nodeName string,
) (bool, error) {
	publications, err := c.orchestrator.ListVolumePublicationsForNode(ctx, nodeName)
	if err != nil {
		Logc(ctx).WithField("node", nodeName).WithError(err).Error("Could not get volume publications.")
		return false, err
	}

	// Create a map for fast lookup.
	expectedRemovedVolPubs := map[string]struct{}{}
	for _, vol := range actionCR.Status.VolumeAttachments {
		expectedRemovedVolPubs[vol] = struct{}{}
	}

	publicationsLeft := 0
	// Check if this publication is expected to be removed.
	for _, pub := range publications {
		volName := pub.VolumeName
		if _, exists := expectedRemovedVolPubs[volName]; exists {
			publicationsLeft++
		}
	}

	if publicationsLeft > 0 {
		Logc(ctx).WithField("publicationCount", publicationsLeft).
			Info("Volume publications awaiting removal.")
		return false, nil
	}
	return true, nil
}

// failoverDetach performs the force detach of supported workloads from the specified node
func (c *TridentCrdController) failoverDetach(
	ctx context.Context, actionCR *netappv1.TridentNodeRemediation, nodeName string,
) error {
	Logc(ctx).WithField("tridentNodeRemediation", actionCR.Name).Info("Beginning failed node cleanup.")
	nodePods, err := c.nodeRemediationUtils.GetNodePods(ctx, nodeName)
	if err != nil {
		Logc(ctx).WithField("node", nodeName).WithError(err).Error("Failed to list pods on node.")
		return err
	}

	pvcToTvolMap, err := c.nodeRemediationUtils.GetPvcToTvolMap(ctx, nodeName)
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to get PVC to Tvol map.")
		return err
	}

	// Get pods to delete
	Logc(ctx).WithField("node", nodeName).Info("Finding pods to remove from failed node.")
	pods := c.nodeRemediationUtils.GetPodsToDelete(ctx, nodePods, pvcToTvolMap)

	// Get VAs to delete
	Logc(ctx).WithField("node", nodeName).Info("Finding volume attachments to remove from failed node.")
	vaToVolMap, err := c.nodeRemediationUtils.GetVolumeAttachmentsToDelete(ctx, pods, pvcToTvolMap, nodeName)
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed get volume attachments for deletion.")
		return err
	}

	// Update the CR status with VAs to delete.
	// This is additive, not a replacement. This way if we have already deleted pods and then crash, we will still
	// know which VAs to delete when we restart.
	Logc(ctx).WithField("tridentNodeRemediation", actionCR.Name).Info(
		"Adding volume attachments to delete to TridentNodeRemediation CR status.")
	for key, val := range vaToVolMap {
		actionCR.Status.VolumeAttachments[key] = val
	}
	err = c.updateNodeRemediationCR(ctx, actionCR.Namespace, actionCR.Name, &actionCR.Status)
	if err != nil {
		Logc(ctx).WithError(err).WithField("tridentNodeRemediation", actionCR.Name).Error(
			"Failed to update TridentNodeRemediation CR status.")
		return err
	}

	// Delete the pods
	for _, pod := range pods {
		err := c.nodeRemediationUtils.ForceDeletePod(ctx, pod)
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"pod":       pod.Name,
				"namespace": pod.Namespace,
			}).WithError(err).Error("Failed to delete Pod.")
			return err
		}
	}

	// Delete the volume attachments
	for attachmentName := range actionCR.Status.VolumeAttachments {
		err := c.nodeRemediationUtils.DeleteVolumeAttachment(ctx, attachmentName)
		if err != nil {
			Logc(ctx).WithField("attachment", attachmentName).WithError(err).
				Error("Failed to delete VolumeAttachment.")
			return err
		}
	}

	// Give Kubernetes time to process the deletions
	time.Sleep(5 * time.Second)

	return nil
}
