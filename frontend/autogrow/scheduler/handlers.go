// Copyright 2026 NetApp, Inc. All Rights Reserved.

package scheduler

import (
	"context"

	"go.uber.org/multierr"
	k8serror "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/netapp/trident/frontend/csi/controller_helpers/kubernetes"
	. "github.com/netapp/trident/logging"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/autogrow"
	"github.com/netapp/trident/utils/errors"
)

// handleStorageClassEvent handles any StorageClass event (add/update/delete)
// When a StorageClass event occurs, all pvs(tvps) using that SC need to be re-evaluated
// We don't differentiate by action - precedence check will determine current state
func (s *Scheduler) handleStorageClassEvent(
	ctx context.Context,
	scName string,
) error {
	Logc(ctx).WithField("scName", scName).Debug("scheduler.handleStorageClassEvent")
	defer Logc(ctx).WithField("scName", scName).Debug("<<<< scheduler.handleStorageClassEvent")

	// Get all TVPs on this node using the lister
	allTVPs, err := s.tvpLister.List(labels.Everything())
	if err != nil {
		Logc(ctx).WithError(err).WithField("scName", scName).
			Warn("Failed to list TVPs for StorageClass event")
		return errors.WrapWithReconcileDeferredError(err, "failed to list TVPs")
	}

	// Filter TVPs that use this StorageClass
	var affectedTVPs []*v1.TridentVolumePublication
	for _, tvp := range allTVPs {
		if tvp.StorageClass == scName {
			affectedTVPs = append(affectedTVPs, tvp)
		}
	}

	if len(affectedTVPs) == 0 {
		Logc(ctx).WithField("scName", scName).
			Debug("No tvps found for StorageClass")
		return nil
	}

	// Get SC annotation once for all tvps
	// If SC is deleted or being deleted, scAnnotation will be empty string
	var scAnnotation string
	var scDeleted bool
	sc, err := s.scLister.Get(scName)
	if err != nil {
		if k8serror.IsNotFound(err) {
			// SC deleted - reconcile tvp with empty SC annotation
			scDeleted = true
			Logc(ctx).WithField("scName", scName).
				Debug("StorageClass not found (deleted), reconciling tvps with empty SC annotation")
		} else {
			Logc(ctx).WithError(err).WithField("scName", scName).
				Warn("Failed to get StorageClass")
			return errors.WrapWithReconcileDeferredError(err, "failed to get StorageClass")
		}
	} else if sc != nil {
		// Check if SC is being deleted
		if !sc.ObjectMeta.DeletionTimestamp.IsZero() {
			scDeleted = true
			Logc(ctx).WithFields(LogFields{
				"scName":            scName,
				"deletionTimestamp": sc.ObjectMeta.DeletionTimestamp,
			}).Debug("StorageClass is being deleted, reconciling tvps with empty SC annotation")
		} else if sc.Annotations != nil {
			// SC exists and is active - get annotation
			scAnnotation = sc.Annotations[kubernetes.AnnAutogrowPolicy]
		}
	}

	logFields := LogFields{
		"scName":    scName,
		"tvpCount":  len(affectedTVPs),
		"scDeleted": scDeleted,
	}
	Logc(ctx).WithFields(logFields).Info("Updating effective policies for all volumes in StorageClass")

	// Re-evaluate effective policy for each volume
	var errList error
	for _, tvp := range affectedTVPs {
		tvpName := tvp.Name

		// Check if TVP is being deleted
		tvpDeleted := !tvp.ObjectMeta.DeletionTimestamp.IsZero()

		if tvp.AutogrowIneligible {
			Logc(ctx).WithFields(LogFields{
				"tvpName": tvpName,
			}).Debug("TVP is ineligible for autogrow monitoring, skipping adding (removing) to effective policy cache and assorter")
			tvpDeleted = true
		}

		// Reconcile the TVP's effective policy
		// Pass extracted fields from TVP
		if err := s.reconcileVolumeEffectivePolicy(ctx, tvpName, tvp.AutogrowPolicy, tvpDeleted, scName, scAnnotation); err != nil {
			Logc(ctx).WithError(err).WithFields(LogFields{
				"tvpName": tvpName,
				"scName":  scName,
			}).Warn("Failed to reconcile volume effective policy")
			errList = multierr.Append(errList, err)
			continue
		}
	}

	Logc(ctx).WithFields(LogFields{
		"scName":   scName,
		"tvpCount": len(affectedTVPs),
	}).Info("Completed updating effective policies for StorageClass")

	return errList
}

// handleTVPEvent handles any TridentVolumePublication event (add/update/delete)
// TVP events indicate that something changed for a tvp (PVC annotation, deletion, etc.)
// We don't differentiate by action - precedence check will determine current state
func (s *Scheduler) handleTVPEvent(
	ctx context.Context,
	tvpName string,
) error {

	Logc(ctx).WithField("tvpName", tvpName).Debug("Handling TVP event")

	// Fetch TVP to extract fields needed for reconciliation
	tvp, err := s.tvpLister.TridentVolumePublications(s.config.TridentNamespace).Get(tvpName)
	if err != nil {
		if k8serror.IsNotFound(err) {
			// TVP deleted - let reconcile handle cleanup with deletion flag
			return s.reconcileVolumeEffectivePolicy(ctx, tvpName, "", true, "", "")
		}
		// API error - retry
		Logc(ctx).WithError(err).WithField("tvpName", tvpName).
			Warn("Failed to get TVP for event")
		return errors.WrapWithReconcileDeferredError(err, "failed to get TVP")
	}

	// Check if TVP is being deleted
	tvpDeleted := !tvp.ObjectMeta.DeletionTimestamp.IsZero()

	if tvp.AutogrowIneligible {
		Logc(ctx).WithFields(LogFields{
			"tvpName": tvpName,
		}).Debug("TVP is ineligible for autogrow monitoring, skipping adding (removing) to effective policy cache and assorter")
		tvpDeleted = true
	}

	// Fetch SC annotation if TVP has a StorageClass
	var scAnnotation string
	scName := tvp.StorageClass
	if scName != "" {
		sc, err := s.scLister.Get(scName)
		if err == nil && sc != nil && sc.ObjectMeta.DeletionTimestamp.IsZero() {
			if sc.Annotations != nil {
				scAnnotation = sc.Annotations[kubernetes.AnnAutogrowPolicy]
			}
		}
		// If SC not found or error, scAnnotation remains empty
	}

	// Pass all resolved fields to reconcile
	return s.reconcileVolumeEffectivePolicy(
		ctx,
		tvpName,            // TVP name (volumeID.nodeID format)
		tvp.AutogrowPolicy, // TVP-level policy annotation
		tvpDeleted,
		scName,       // SC name
		scAnnotation, // SC annotation (already fetched)
	)
}

// This function is used by handleStorageClassEvent, handleTVPEvent, and syncCacheWithAPIServer.
// It is the ONLY function that mutates the effectivePolicyCache and assorter.
// All callers must extract and resolve the necessary fields before calling.
// This function focuses purely on policy resolution and cache mutation.
func (s *Scheduler) reconcileVolumeEffectivePolicy(
	ctx context.Context,
	tvpName string, // TridentVolumePublication name (volumeID.nodeID format)
	tvpAutogrowPolicy string, // AutogrowPolicy field from TVP
	tvpDeleted bool, // Caller detected TVP deletion
	scName string, // StorageClass name (empty = no SC)
	scAnnotation string, // SC autogrow annotation (already resolved by caller)
) error {
	Logc(ctx).WithFields(LogFields{
		"tvpName":           tvpName,
		"tvpAutogrowPolicy": tvpAutogrowPolicy,
		"scName":            scName,
		"scAnnotation":      scAnnotation,
		"tvpDeleted":        tvpDeleted,
	}).Debug(">>>> scheduler.reconcileVolumeEffectivePolicy")
	defer Logc(ctx).WithFields(LogFields{
		"tvpName":           tvpName,
		"tvpAutogrowPolicy": tvpAutogrowPolicy,
		"scName":            scName,
		"scAnnotation":      scAnnotation,
		"tvpDeleted":        tvpDeleted,
	}).Debug("<<<< scheduler.reconcileVolumeEffectivePolicy")

	var errList error

	// Handle deletion first (cleanup path)
	if tvpDeleted {
		if err := s.autogrowCache.DeleteEffectivePolicyName(tvpName); err != nil {
			Logc(ctx).WithError(err).WithFields(LogFields{
				"tvpName": tvpName,
			}).Debug("Failed to remove deleted volume (tvp) from effectivePolicyCache")
			errList = multierr.Append(errList, err)
		}
		if err := s.assorter.RemoveVolume(ctx, tvpName); err != nil {
			Logc(ctx).WithError(err).WithFields(LogFields{
				"tvpName": tvpName,
			}).Debug("Failed to remove deleted volume (tvp) from assorter")
			errList = multierr.Append(errList, err)
		}
		Logc(ctx).WithFields(LogFields{
			"tvpName": tvpName,
		}).Debug("Cleaned up deleted volume (tvp) from cache and assorter")
		return errList
	}

	// Run precedence check to determine effective policy
	// All inputs are already resolved by the caller
	// Note: ResolveEffectiveAutogrowPolicy expects volumeID (not tvpName) for logging purposes
	// We can extract volumeID from tvpName if needed, but for now pass tvpName
	effectivePolicyName := autogrow.ResolveEffectiveAutogrowPolicy(ctx, tvpName, tvpAutogrowPolicy, scAnnotation)

	// Manage effectivePolicyName cache state based on effective policy
	// If effectivePolicyName is empty, we need to remove the volume from the caches and assorter
	// As that volume is no longer subject to autogrow
	if effectivePolicyName == "" {
		if err := s.autogrowCache.DeleteEffectivePolicyName(tvpName); err != nil {
			Logc(ctx).WithError(err).WithField("tvpName", tvpName).
				Debug("Failed to remove volume (tvp) from effectivePolicyCache")
			errList = multierr.Append(errList, err)
		}
		if err := s.assorter.RemoveVolume(ctx, tvpName); err != nil {
			Logc(ctx).WithError(err).WithField("tvpName", tvpName).
				Debug("Failed to remove volume (tvp) from assorter")
			errList = multierr.Append(errList, err)
		}
		Logc(ctx).WithFields(LogFields{
			"tvpName":      tvpName,
			"storageClass": scName,
		}).Debug("Removed volume (tvp) from cache (no effective policy)")
		return errList
	}

	// Has effective policy - add/update volume (tvp) in ALL caches and assorter
	// We should not check either the existence of AGP or whether it's in failed or deleting state.
	// Otherwise, we would have to add AGP handlers too, to capture the events and update accordingly
	// Instead we rely on poller and requester layers to discard such events.

	// Set effective policy in cache (idempotent)
	if err := s.autogrowCache.SetEffectivePolicyName(tvpName, effectivePolicyName); err != nil {
		Logc(ctx).WithError(err).WithField("tvpName", tvpName).
			Warn("Failed to add volume (tvp) to effectivePolicyCache")
		errList = multierr.Append(errList, err)
	}

	// Add to assorter (idempotent)
	if err := s.assorter.AddVolume(ctx, tvpName); err != nil {
		Logc(ctx).WithError(err).WithField("tvpName", tvpName).
			Warn("Failed to add volume (tvp) to assorter")
		errList = multierr.Append(errList, err)
	} else {
		Logc(ctx).WithFields(LogFields{
			"tvpName":      tvpName,
			"storageClass": scName,
			"policyName":   effectivePolicyName,
		}).Debug("Reconciled volume (tvp) effective policy")
	}

	return errList
}
