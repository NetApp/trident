// Copyright 2026 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"strings"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestNodeClusterRoleCSIReadsNodeForTopology(t *testing.T) {
	t.Parallel()

	labels := map[string]string{"app": "node.csi.trident.netapp.io"}
	yamlDoc := GetClusterRoleYAML("trident-node-linux", labels, nil)

	var role rbacv1.ClusterRole
	require.NoError(t, yaml.Unmarshal([]byte(yamlDoc), &role))

	var nodeRules []rbacv1.PolicyRule
	for _, rule := range role.Rules {
		if containsRBACValue(rule.APIGroups, "") && grantsNodesResource(rule.Resources) {
			nodeRules = append(nodeRules, rule)
		}
	}

	require.Len(t, nodeRules, 1, "node ClusterRole must grant core Node access through exactly one rule")
	assert.Equal(t, []string{""}, nodeRules[0].APIGroups)
	assert.Equal(t, []string{"nodes"}, nodeRules[0].Resources, "node must not reach Node subresources")
	assert.Equal(t, []string{"get"}, nodeRules[0].Verbs, "reading topology labels needs 'get' and nothing more")
}

func containsRBACValue(values []string, expected string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
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
