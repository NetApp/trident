// Copyright 2019 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"testing"

	"github.com/ghodss/yaml"
)

// TestYAML simple validation of the YAML
func TestYAML(t *testing.T) {
	yamls := []string{
		namespaceYAMLTemplate,
		serviceAccountYAML,
		clusterRoleOpenShiftYAML,
		clusterRoleOpenShiftCSIYAML,
		clusterRoleKubernetesV1YAML,
		clusterRoleKubernetesV1CSIYAML,
		clusterRoleBindingOpenShiftYAMLTemplate,
		clusterRoleBindingKubernetesV1YAMLTemplate,
		//deploymentYAMLTemplate,
		//serviceYAMLTemplate,
		//statefulSetYAMLTemplate,
		//daemonSetYAMLTemplate,
		installerServiceAccountYAML,
		installerClusterRoleOpenShiftYAML,
		installerClusterRoleKubernetesYAMLTemplate,
		installerClusterRoleBindingOpenShiftYAMLTemplate,
		installerClusterRoleBindingKubernetesV1YAMLTemplate,
		migratorPodYAMLTemplate,
		installerPodTemplate,
		uninstallerPodTemplate,
		openShiftSCCQueryYAMLTemplate,
		secretYAMLTemplate,
		customResourceDefinitionYAMLTemplate,
	}
	for i, yamlData := range yamls {
		//jsonData, err := yaml.YAMLToJSON([]byte(yamlData))
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected constant %v to be valid YAML", i)
		}
		//fmt.Printf("json: %v", string(jsonData))
	}
}
