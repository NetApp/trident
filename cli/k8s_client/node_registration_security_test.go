// Copyright 2026 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"
)

func TestNodeClusterRoleCSI_StatusSubresourceReadOnly(t *testing.T) {
	t.Parallel()

	labels := map[string]string{TridentAppLabelKey: "node.csi.trident.netapp.io"}
	yamlDoc := GetClusterRoleYAML("trident-node-linux", labels, nil)

	var role rbacv1.ClusterRole
	require.NoError(t, yaml.Unmarshal([]byte(yamlDoc), &role))

	var specRule *rbacv1.PolicyRule
	for i := range role.Rules {
		rule := &role.Rules[i]
		if containsResource(rule.Resources, "tridentnodes/status") {
			t.Fatalf("node ClusterRole must not grant tridentnodes/status; node reads status via spec GET")
		}
		if containsResource(rule.Resources, "tridentnodes") && !containsResource(rule.Resources, "tridentnodes/status") {
			specRule = rule
			break
		}
	}

	require.NotNil(t, specRule, "expected tridentnodes spec rule")
	assert.Contains(t, specRule.Verbs, "create")
	assert.Contains(t, specRule.Verbs, "update")
}

func TestNodeClusterRoleCSI_ReadsOwnNodeForTopology(t *testing.T) {
	t.Parallel()

	labels := map[string]string{TridentAppLabelKey: "node.csi.trident.netapp.io"}
	yamlDoc := GetClusterRoleYAML("trident-node-linux", labels, nil)

	var role rbacv1.ClusterRole
	require.NoError(t, yaml.Unmarshal([]byte(yamlDoc), &role))

	var nodeRules []rbacv1.PolicyRule
	for _, rule := range role.Rules {
		if containsAPIGroup(rule.APIGroups, "") && grantsNodesResource(rule.Resources) {
			nodeRules = append(nodeRules, rule)
		}
	}

	require.Len(t, nodeRules, 1, "node ClusterRole must grant core Node access through exactly one rule")
	assert.Equal(t, []string{""}, nodeRules[0].APIGroups)
	assert.Equal(t, []string{"nodes"}, nodeRules[0].Resources, "node must not reach Node subresources")
	assert.Equal(t, []string{"get"}, nodeRules[0].Verbs, "reading topology labels needs 'get' and nothing more")
}

func containsAPIGroup(groups []string, name string) bool {
	for _, g := range groups {
		if strings.TrimSpace(g) == name {
			return true
		}
	}
	return false
}

// grantsNodesResource matches Nodes, any Node subresource, and wildcards so permission creep in a
// separate rule fails the least-privilege assertions rather than slipping past them.
func grantsNodesResource(resources []string) bool {
	for _, r := range resources {
		r = strings.TrimSpace(r)
		if r == "nodes" || r == "*" || strings.HasPrefix(r, "nodes/") {
			return true
		}
	}
	return false
}

func containsResource(resources []string, name string) bool {
	for _, r := range resources {
		if strings.TrimSpace(r) == name {
			return true
		}
	}
	return false
}
