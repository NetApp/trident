// Copyright 2026 NetApp, Inc. All Rights Reserved.

package scheduler

import (
	"context"
	"fmt"

	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/netapp/trident/frontend/csi/controller_helpers/kubernetes"
	. "github.com/netapp/trident/logging"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

// syncCacheWithAPIServer is the generic function that syncs the Autogrow cache with the
// Kubernetes API server state. This is used by both:
// - bootstrap() during scheduler activation (ctx has source="bootstrap")
// - reconciliationLoop() for periodic drift correction (ctx has source="reconciliation")
//
// The source is extracted from the context for logging.
func (s *Scheduler) syncCacheWithAPIServer(ctx context.Context) error {
	Logc(ctx).Debug(">>>> scheduler.syncCacheWithAPIServer")
	defer Logc(ctx).Debug("<<<< scheduler.syncCacheWithAPIServer")

	// Defensive checks
	if s.tvpLister == nil {
		Logc(ctx).Warn("TVP lister is nil")
		return fmt.Errorf("TVP lister is nil")
	}

	// Try to list items using lister directly
	tvps, err := s.tvpLister.List(labels.Everything())
	if err != nil {
		Logc(ctx).WithError(err).Warn("TVP lister.List() failed, will wait till the next reconcile")
		// We won't try to list directly from the kube-api server, instead will wait for the next cycle.
		return err
	}

	Logc(ctx).WithField("tvpCount", len(tvps)).Debug("Retrieved TVPs from lister")

	// Process items from lister
	return s.processTVPsForSync(ctx, tvps)
}

// processTVPsForSync processes a list of TVPs and populates the Autogrow cache.
// This is shared by bootstrap, reconciliation, and any other sync operations.
// The source (bootstrap/reconciliation) is extracted from the context for logging.
func (s *Scheduler) processTVPsForSync(ctx context.Context, tvps []*tridentv1.TridentVolumePublication) error {
	if len(tvps) == 0 {
		Logc(ctx).Debug("No TVPs to process")
		return nil
	}

	Logc(ctx).WithField("tvpCount", len(tvps)).Info("Syncing Autogrow cache from TVPs")

	var processedCount int
	var skippedCount int
	var errorCount int
	var errList error

	for _, tvp := range tvps {
		tvpName := tvp.Name
		storageClass := tvp.StorageClass

		// Check if TVP is being deleted
		tvpDeleted := !tvp.ObjectMeta.DeletionTimestamp.IsZero()
		if tvpDeleted {
			Logc(ctx).WithFields(LogFields{
				"tvpName":           tvpName,
				"storageClass":      storageClass,
				"deletionTimestamp": tvp.ObjectMeta.DeletionTimestamp,
			}).Debug("TVP is being deleted, will cleanup via reconcile")
		}

		// Get StorageClass annotation (empty if SC deleted/deleting or if TVP has no SC)
		// Note: Even if StorageClass is empty, we still process the TVP because it might
		// have a Volume-level AutogrowPolicy (highest precedence in the hierarchy)
		var scAnnotation string
		if storageClass != "" {
			sc, err := s.scLister.Get(storageClass)
			if err != nil {
				errList = multierr.Append(errList, err)
			} else if sc != nil && sc.ObjectMeta.DeletionTimestamp.IsZero() {
				if sc.Annotations != nil {
					scAnnotation = sc.Annotations[kubernetes.AnnAutogrowPolicy]
				}
			}
		}

		// Use reconcileVolumeEffectivePolicy as single source of truth
		// This handles:
		// - Resolving effective policy using precedence rules (TVP policy, SC annotation)
		// - Adding/removing from effective policy cache and assorter
		// Pass extracted fields from TVP (avoid passing whole object)
		// tvpName is the primary identifier (volumeID.nodeID format)
		if err := s.reconcileVolumeEffectivePolicy(ctx, tvpName, tvp.AutogrowPolicy, tvpDeleted, storageClass, scAnnotation); err != nil {
			Logc(ctx).WithError(err).WithFields(LogFields{
				"tvpName":      tvpName,
				"storageClass": storageClass,
			}).Warn("Failed to reconcile volume during sync")
			errList = multierr.Append(errList, err)
			errorCount++

			// Continue processing other volumes - don't fail entire sync
			continue
		}

		if tvpDeleted {
			skippedCount++
		} else {
			processedCount++
		}

		Logc(ctx).WithFields(LogFields{
			"tvpName":      tvpName,
			"storageClass": storageClass,
		}).Debug("Synced volume from TVP")
	}

	Logc(ctx).WithFields(LogFields{
		"totalTVPs":      len(tvps),
		"processedCount": processedCount,
		"skippedCount":   skippedCount,
		"errorCount":     errorCount,
	}).Info("Completed sync of Autogrow cache from TVPs")

	return errList
}
