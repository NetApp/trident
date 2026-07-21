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

func containsResource(resources []string, name string) bool {
	for _, r := range resources {
		if strings.TrimSpace(r) == name {
			return true
		}
	}
	return false
}
