// Copyright 2026 NetApp, Inc. All Rights Reserved.
package autogrow

import (
	"context"
	"regexp"
	"strings"

	. "github.com/netapp/trident/logging"
)

const (
	// AutogrowPolicyNone is a special keyword to explicitly disable Autogrow policy.
	// When used in PVC or SC annotation, it:
	//  - Blocks inheritance from lower-priority sources (PVC blocks SC)
	//  - Disables Autogrow for the volume
	//  - Case-insensitive: "none", "None", "NONE" all work
	AutogrowPolicyNone = "none"
)

// nameFixRegex matches characters NOT valid in K8s resource names
var nameFixRegex = regexp.MustCompile(`[^a-z0-9.\-]`)

// nameFix converts a string to valid K8s resource name format.
func NameFix(n string) string {
	n = strings.ToLower(n)
	n = nameFixRegex.ReplaceAllString(n, "-")
	return n
}

// ResolveEffectiveAutogrowPolicy determines the effective Autogrow policy for a volume
// based on the annotation hierarchy:
//  1. PVC annotation (trident.netapp.io/autogrowPolicy) - highest priority
//  2. StorageClass annotation (autogrowPolicy annotation on StorageClass)
//  3. No policy (empty string) - lowest priority
//
// Special keyword "none" (case-insensitive):
//   - Explicitly disables Autogrow
//   - Blocks inheritance from lower-priority sources
//   - Converted to empty string in the return value
//
// Policy names are normalized to valid K8s resource name format.
//
// Examples:
//   - PVC: "none", SC: "gold" → Returns "" (disabled, blocks SC)
//   - PVC: "", SC: "gold" → Returns "gold" (inherits from SC)
//   - PVC: "silver", SC: "gold" → Returns "silver" (PVC overrides)
//   - PVC: "My Policy", SC: "" → Returns "my-policy" (normalized)
//
// This function is stateless and can be called from:
// - Orchestrator (to compute effective policy during volume operations)
// - CSI Frontend (to compute effective policy before creating volumes)
// - Node scheduler (to compute effective policy during monitoring)
//
// Parameters:
//   - ctx: Context for logging
//   - volumeName: Name of the volume (for logging)
//   - pvcAnnotation: Value of trident.netapp.io/autogrowPolicy from PVC (empty if not set)
//   - scAnnotation: Value of autogrowPolicy annotation from StorageClass (empty if not set)
//
// Returns:
//   - Policy name if configured (e.g., "gold-policy")
//   - Empty string if no policy or explicitly disabled with "none"
func ResolveEffectiveAutogrowPolicy(ctx context.Context, volumeName, pvcAnnotation, scAnnotation string) string {
	// Priority 1: PVC annotation (explicit per-volume Autogrow policy)
	if pvcAnnotation != "" {
		// Special case: "none" means explicitly disable Autogrow (case-insensitive)
		// This blocks inheritance from lower-priority sources
		if strings.EqualFold(pvcAnnotation, AutogrowPolicyNone) {
			Logc(ctx).WithFields(LogFields{
				"volumeName": volumeName,
				"source":     "PVC",
			}).Debug("Autogrow explicitly disabled via PVC annotation (none)")
			return "" // Return empty = disabled
		}
		// PVC has an actual Autogrow policy name
		Logc(ctx).WithFields(LogFields{
			"volumeName":   volumeName,
			"agPolicyName": pvcAnnotation,
			"source":       "PVC",
		}).Info("Resolved effective Autogrow policy from PVC annotation")
		return NameFix(pvcAnnotation)
	}

	// Priority 2: StorageClass annotation (default Autogrow policy for all volumes using this SC)
	if scAnnotation != "" {
		// Special case: "none" at SC level disables for all volumes (case-insensitive)
		if strings.EqualFold(scAnnotation, AutogrowPolicyNone) {
			Logc(ctx).WithFields(LogFields{
				"volumeName": volumeName,
				"source":     "StorageClass",
			}).Debug("Autogrow explicitly disabled via StorageClass annotation (none)")
			return "" // Return empty = disabled
		}
		// SC has an actual Autogrow policy name
		Logc(ctx).WithFields(LogFields{
			"volumeName":   volumeName,
			"agPolicyName": scAnnotation,
			"source":       "StorageClass",
		}).Info("Resolved effective Autogrow policy from StorageClass annotation")
		return NameFix(scAnnotation)
	}

	// Priority 3: No Autogrow policy specified in SC or PVC annotation
	Logc(ctx).WithField("volumeName", volumeName).Info("No Autogrow policy configured for the volume.")
	return ""
}
