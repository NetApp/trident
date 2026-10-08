// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core/node"
	. "github.com/netapp/trident/logging"
	tridenterrors "github.com/netapp/trident/utils/errors"
)

// handleTridentBackends handles the business logic for TridentBackend events
func (c *TridentNodeCrdController) handleTridentBackends(keyItem *KeyItem) error {
	key := keyItem.key
	ctx := keyItem.ctx

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logc(ctx).WithField("key", key).Error("Invalid key")
		return err
	}

	if namespace == "" {
		Logc(ctx).WithField("key", key).Error("Invalid key; no namespace present.")
		return apierrors.NewBadRequest(fmt.Sprintf("invalid key, no namespace present: %v", key))
	}

	Logc(ctx).WithFields(LogFields{
		"TridentBackend": name,
		"Namespace":      namespace,
		"Event":          keyItem.event,
	}).Debug("Processing TridentBackend event")

	// Get the TridentBackend object
	backend, err := c.tridentBackendLister.TridentBackends(namespace).Get(name)
	if err != nil {
		if keyItem.event == EventDelete {
			// For delete events, we still want to notify autogrow even if object is gone
			// Send the full key (namespace/name)
			c.autogrowOrchestrator.HandleTBEEvent(ctx, EventDelete, key)

			Logc(ctx).WithFields(LogFields{
				"TridentBackend": name,
				"Namespace":      namespace,
				"Event":          keyItem.event,
			}).Debug("TridentBackend already deleted")
			return nil
		}
		Logc(ctx).WithFields(LogFields{
			"TridentBackend": name,
			"Namespace":      namespace,
			"Event":          keyItem.event,
		}).WithError(err).Error("Failed to get TridentBackend")
		return err
	}

	fields := LogFields{
		"TridentBackend": name,
		"Namespace":      namespace,
		"BackendUUID":    backend.BackendUUID,
		"Event":          keyItem.event,
	}
	Logc(ctx).WithFields(fields).Debug("Processing TridentBackend event")

	// Let autogrow controller handle this event (publishes to ControllerEventBus)
	// Send the full key (namespace/name) so scheduler has namespace info
	c.autogrowOrchestrator.HandleTBEEvent(ctx, keyItem.event, key)

	// Data LIF refresh is opt-in. A nil snapshot means none has been published. A non-nil
	// snapshot, including an empty slice, is authoritative.
	if dataLIFs := backend.PublishedDataLIFs(); config.EnableDataLIFRefresh && dataLIFs != nil {
		if err := c.reconcilePublishedDataLIFs(ctx, backend.BackendUUID, *dataLIFs); err != nil {
			return tridenterrors.WrapWithReconcileDeferredError(err, "data LIF reconciliation failed")
		}
	}

	Logc(ctx).WithFields(fields).Info("Successfully processed TridentBackend event")

	return nil
}

// reconcilePublishedDataLIFs asks the node core to converge each volume this node has published
// from the backend onto the backend's data LIFs. The request carries only the target IPs. The
// node core owns the tracking info and decides, under the volume lock, which protocol applies and
// which paths to prune or graft. A nil TridentBackend DataLIFs pointer is filtered by the caller;
// an empty slice here is an authoritative snapshot that all data LIFs were removed.
func (c *TridentNodeCrdController) reconcilePublishedDataLIFs(
	ctx context.Context, backendUUID string, desiredIPs []string,
) error {
	publications, err := c.tridentVolumePublicationLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("could not list volume publications: %w", err)
	}

	// A failing volume must not block the others. Reconciliation is idempotent per volume, so
	// retrying the whole set after a partial failure is safe.
	var errs []error
	for _, publication := range publications {
		if publication == nil || publication.NodeID != c.nodeName ||
			publication.BackendUUID != backendUUID || publication.VolumeID == "" {
			continue
		}
		if err := c.orchestrator.ReconcileAttachment(ctx, publication.VolumeID, node.ReconcileAttachmentRequest{
			TargetIPs: desiredIPs,
		}); err != nil {
			errs = append(errs, fmt.Errorf(
				"could not reconcile data LIFs for volume %s: %w", publication.VolumeID, err))
		}
	}

	return errors.Join(errs...)
}
