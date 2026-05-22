// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import "strings"

const policyDRVariantSuffix = "-DR"

// expandPolicyForVolumeListQuery returns a policy filter that matches both the configured
// value and its MetroCluster / SVM-DR counterpart. ONTAP may rename policies on the DR site
// by appending "-DR" (for example "none" becomes "none-DR"). ZAPI and REST volume list
// queries accept pipe-separated values, so callers can use this for snapshot policy,
// tiering policy, or any other list filter that follows the same naming convention.
func expandPolicyForVolumeListQuery(policy string) string {
	if policy == "" || strings.Contains(policy, "|") {
		return policy
	}
	if strings.HasSuffix(policy, policyDRVariantSuffix) {
		base := strings.TrimSuffix(policy, policyDRVariantSuffix)
		return policy + "|" + base
	}
	return policy + "|" + policy + policyDRVariantSuffix
}
