// Copyright 2019 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"testing"

	"github.com/ghodss/yaml"
)

const (
	Name            = "trident"
	Namespace       = "trident"
	FlavorK8s       = "k8s"
	FlavorOpenshift = "openshift"
	ImageName       = "trident-image"
	LogFormat       = "text"
)

var Secrets = []string {"thisisasecret1", "thisisasecret2"}

// TestYAML simple validation of the YAML
func TestYAML(t *testing.T) {
	yamls := []string{
		namespaceYAMLTemplate,
		installerServiceAccountYAML,
		installerClusterRoleOpenShiftYAML,
		installerClusterRoleKubernetesYAMLTemplate,
		installerClusterRoleBindingOpenShiftYAMLTemplate,
		installerClusterRoleBindingKubernetesV1YAMLTemplate,
		installerPodTemplate,
		uninstallerPodTemplate,
		openShiftSCCQueryYAMLTemplate,
		customResourceDefinitionYAML_v1beta1,
		customResourceDefinitionYAML_v1,
		CSIDriverCRDYAML,
		CSINodeInfoCRDYAML,
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

// TestYAMLFactory simple validation of the YAML factory functions
func TestYAMLFactory(t *testing.T) {

	labels := make(map[string]string)
	labels["app"] = "trident"

	ownerRef := make(map[string]string)
	ownerRef["uid"] = "123456789"
	ownerRef["kind"] = "TridentProvisioner"

	imagePullSecrets := []string{"thisisasecret"}

	yamlsOutputs := []string{
		GetServiceAccountYAML(Name, nil, nil, nil),
		GetServiceAccountYAML(Name, Secrets, labels, ownerRef),
		GetClusterRoleYAML(FlavorK8s, Name, nil, nil, false),
		GetClusterRoleYAML(FlavorOpenshift, Name, labels, ownerRef, true),
		GetClusterRoleBindingYAML(Namespace, FlavorOpenshift, Name, nil, ownerRef),
		GetClusterRoleBindingYAML(Namespace, FlavorK8s, Name, labels, ownerRef),
		GetDeploymentYAML(Name, ImageName, LogFormat, imagePullSecrets, labels, ownerRef, true),
		GetCSIServiceYAML(Name, labels, ownerRef),
		GetSecretYAML(Name, Namespace, labels, ownerRef, nil, nil),
	}
	for i, yamlData := range yamlsOutputs {
		//jsonData, err := yaml.YAMLToJSON([]byte(yamlData))
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected constant %v to be valid YAML", i)
		}
		//fmt.Printf("json: %v", string(jsonData))
	}
}
