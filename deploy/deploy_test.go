// Copyright 2023 Netapp, Inc. All Rights Reserved.

package deploy

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/rbac/v1"
)

// getClusterRoleFromYAML returns ClusterRole YAML esp. when YAML contains multiple definitions
func getClusterRoleFromYAML(YAMLs string) []byte {
	labelEqualRegex := regexp.MustCompile(`\s*kind:( )?ClusterRole\s*`)
	yamls := strings.Split(YAMLs, "---")
	var match []string

	for i := range yamls {
		match = labelEqualRegex.FindStringSubmatch(yamls[i])

		if len(match) == 0 {
			continue
		}

		matchValue := strings.TrimSpace(match[0])
		matchValue = strings.TrimLeft(strings.ToLower(matchValue), "kind")
		matchValue = strings.TrimLeft(strings.ToLower(matchValue), ":")
		matchValue = strings.TrimSpace(matchValue)

		return []byte(yamls[i])
	}

	return []byte{}
}

// getClusterRole returns cluster role struct based on the input file path
func getClusterRole(filePath string) (*v1.ClusterRole, error) {
	var clusterRole v1.ClusterRole

	yamlBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	clusterRoleYAMLBytes := getClusterRoleFromYAML(string(yamlBytes))

	err = yaml.Unmarshal(clusterRoleYAMLBytes, &clusterRole)
	if err != nil {
		return nil, err
	}
	return &clusterRole, nil
}

func TestBundleInSyncWithClusterRole(t *testing.T) {
	type InputFilePaths struct {
		ClusterRoleFilePath string
		BundleFilePath      string
	}
	inputPaths := []InputFilePaths{
		{
			ClusterRoleFilePath: "clusterrole_post_1_25.yaml",
			BundleFilePath:      "bundle_post_1_25.yaml",
		},
	}

	for _, inputPath := range inputPaths {

		clusterRoleFilePath := inputPath.ClusterRoleFilePath
		bundleFilePath := inputPath.BundleFilePath

		t.Run(fmt.Sprintf("Comparing %v with %v.", clusterRoleFilePath, bundleFilePath), func(t *testing.T) {
			bundleClusterRole, err := getClusterRole(bundleFilePath)
			assert.Nil(t, err, "failed to get cluster role from bundle file")
			assert.NotNil(t, bundleClusterRole, "cluster role from bundle file should not be nil")

			clusterRole, err := getClusterRole(clusterRoleFilePath)
			assert.Nil(t, err, "failed to get cluster role from clusterrole file")
			assert.NotNil(t, bundleClusterRole, "cluster role from clusterrole file should not be nil")

			assert.True(t, len(clusterRole.Rules) > 0, "cluster role from clusterrole file should contain rules")
			assert.True(t, len(bundleClusterRole.Rules) > 0, "cluster role from bundle file should contain rules")

			assert.Equal(t, len(clusterRole.Rules), len(bundleClusterRole.Rules), "identified unequal number of rules")

			for _, rule := range clusterRole.Rules {
				assert.Contains(t, bundleClusterRole.Rules, rule, fmt.Sprintf("rule %v not found", rule))
			}
		})
	}
}
