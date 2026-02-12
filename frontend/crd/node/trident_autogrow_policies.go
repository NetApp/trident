// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	stderrors "errors"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"

	"github.com/netapp/trident/frontend/autogrow"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// handleTridentAutogrowPolicies handles the business logic for TridentAutogrowPolicy events
// This manages system-level activation/deactivation of the autogrow orchestrator:
// - First AGP created (0→1): Activates autogrow system
// - Last AGP deleted (1→0): Deactivates autogrow system
// - Updates to existing AGPs: No system state change (orchestrator handles internally)
func (c *TridentNodeCrdController) handleTridentAutogrowPolicies(keyItem *KeyItem) error {
	key := keyItem.key
	ctx := keyItem.ctx

	// TridentAutogrowPolicy is cluster-scoped, but still has a namespace in the key
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logc(ctx).WithField("key", key).Error("Invalid key")
		return err
	}

	// For cluster-scoped resources, namespace may be empty
	if namespace == "" {
		namespace = "<cluster-scoped>"
	}

	Logc(ctx).WithFields(LogFields{
		"TridentAutogrowPolicy": name,
		"Namespace":             namespace,
		"Event":                 keyItem.event,
	}).Debug("Processing TridentAutogrowPolicy event")

	// ========================================
	// System Activation/Deactivation Logic
	// ========================================

	// Count current AGPs in the system
	policies, err := c.tridentAutogrowPolicyLister.List(labels.Everything())
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to list TridentAutogrowPolicies")
		return err
	}

	currentCount := len(policies)
	isActive := c.autogrowOrchestrator.GetState() == autogrow.StateRunning
	isStopped := c.autogrowOrchestrator.GetState() == autogrow.StateStopped

	Logc(ctx).WithFields(LogFields{
		"currentCount": currentCount,
		"isActive":     isActive,
		"event":        keyItem.event,
	}).Debug("AGP count check for system activation")

	// State transition logic
	shouldBeActive := currentCount > 0

	if shouldBeActive {
		if !isActive {
			// 0 → 1: First AGP exists, activate system
			Logc(ctx).WithField("agpCount", currentCount).
				Info("First AGP detected, activating autogrow system")

			if err := c.autogrowOrchestrator.Activate(ctx); err != nil {
				// Check if activate() errored out because autogrow orchestrator was in stopping state.
				if errors.IsStateError(err) {
					var stateErr *errors.StateError
					stderrors.As(err, &stateErr)
					switch stateErr.State {
					case string(autogrow.StateStopping):
						// Stopping state - retry
						Logc(ctx).WithField("state", stateErr.State).Info("Autogrow in stopping state, will retry to activate")
						return errors.WrapWithReconcileDeferredError(err, "Autogrow in state %s", stateErr.State)

					case string(autogrow.StateStarting):
						Logc(ctx).WithField("state", stateErr.State).Info("Autogrow in starting state, not retrying to activate")
						return nil

						// Don't need to write a case for StateStopped, as Activate() wouldn't return StateError if
						// autogrow orchestrator is in StatesStopped state to begin with.
					}
				}

				// Not a state error - fail permanently
				Logc(ctx).WithError(err).Error("Failed to activate autogrow system")
				return err
			}
			Logc(ctx).Info("Autogrow system activated successfully")
		}
	} else if !isStopped {
		// 1 → 0: Last AGP deleted, deactivate system
		Logc(ctx).Info("Last AGP deleted, deactivating autogrow system")

		if err := c.autogrowOrchestrator.Deactivate(ctx); err != nil {
			// Check if Deactivate() errored out because autogrow orchestrator was in starting state.
			if errors.IsStateError(err) {
				var stateErr *errors.StateError
				stderrors.As(err, &stateErr)
				switch stateErr.State {
				case string(autogrow.StateStarting):
					// Stating state - retry
					Logc(ctx).WithField("state", stateErr.State).Info("Autogrow in starting state, will retry to deactivate")
					return errors.WrapWithReconcileDeferredError(err, "autogrow in state %s", stateErr.State)

				case string(autogrow.StateStopping):
					Logc(ctx).WithField("state", stateErr.State).Info("Autogrow in stopping state, not retrying to activate")
					return nil

					// Don't need to write a case for StateRunning, as Deactivate() wouldn't return StateError if
					// autogrow orchestrator is in StateRunning state to begin with.
				}
			}

			// Not a state error - fail permanently
			Logc(ctx).WithError(err).Error("Failed to deactivate autogrow system")
			return err
		}

		Logc(ctx).Info("Autogrow system deactivated successfully")

	} else {
		// No state transition - system continues as-is
		// Updates to existing AGPs are handled internally by the autogrow orchestrator
		Logc(ctx).WithFields(LogFields{
			"agpCount":  currentCount,
			"isActive":  isActive,
			"isStopped": isStopped,
		}).Debug("No system state change needed")
	}

	return nil
}
