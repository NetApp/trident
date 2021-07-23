// Copyright 2019 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"fmt"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils"
)

const (
	Name                 = "trident"
	Namespace            = "trident"
	FlavorK8s            = "k8s"
	FlavorOpenshift      = "openshift"
	ImageName            = "trident-image"
	AutosupportImageName = "trident-asup-images"
	LogFormat            = "text"
)

var Secrets = []string{"thisisasecret1", "thisisasecret2"}

// TestYAML simple validation of the YAML
func TestYAML(t *testing.T) {
	yamls := []string{
		namespaceYAMLTemplate,
		openShiftSCCQueryYAMLTemplate,
		customResourceDefinitionYAMLv1,
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
	ownerRef["kind"] = "TridentOrchestrator"

	imagePullSecrets := []string{"thisisasecret"}

	version := utils.MustParseSemantic("1.21.0")

	yamlsOutputs := []string{
		GetServiceAccountYAML(Name, nil, nil, nil),
		GetServiceAccountYAML(Name, Secrets, labels, ownerRef),
		GetClusterRoleYAML(FlavorK8s, Name, nil, nil, false),
		GetClusterRoleYAML(FlavorOpenshift, Name, labels, ownerRef, true),
		GetClusterRoleBindingYAML(Namespace, FlavorOpenshift, Name, nil, ownerRef, false),
		GetClusterRoleBindingYAML(Namespace, FlavorK8s, Name, labels, ownerRef, true),
		GetCSIDeploymentYAML(Name, ImageName, AutosupportImageName, "", "",
			"", "", "", "text", "", imagePullSecrets,
			labels, ownerRef, false, false, false, version, false),
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

// TestAPIVersion validates that we get correct APIVersion value
func TestAPIVersion(t *testing.T) {

	yamlsOutputs := map[string]string{
		GetClusterRoleYAML(FlavorK8s, Name, nil, nil, false):                         "rbac.authorization.k8s.io/v1",
		GetClusterRoleYAML(FlavorK8s, Name, nil, nil, true):                          "rbac.authorization.k8s.io/v1",
		GetClusterRoleYAML(FlavorOpenshift, Name, nil, nil, false):                   "authorization.openshift.io/v1",
		GetClusterRoleYAML(FlavorOpenshift, Name, nil, nil, true):                    "rbac.authorization.k8s.io/v1",
		GetClusterRoleBindingYAML(Namespace, FlavorK8s, Name, nil, nil, false):       "rbac.authorization.k8s.io/v1",
		GetClusterRoleBindingYAML(Namespace, FlavorK8s, Name, nil, nil, true):        "rbac.authorization.k8s.io/v1",
		GetClusterRoleBindingYAML(Namespace, FlavorOpenshift, Name, nil, nil, false): "authorization.openshift.io/v1",
		GetClusterRoleBindingYAML(Namespace, FlavorOpenshift, Name, nil, nil, true):  "rbac.authorization.k8s.io/v1",
	}

	for result, value := range yamlsOutputs {
		assert.Contains(t, result, value, fmt.Sprintf("Incorrect API Version returned %s", value))
	}
}

// Simple validation of the CSI Deployment YAML
func TestValidateGetCSIDeploymentYAMLSuccess(t *testing.T) {

	labels := make(map[string]string)
	labels["app"] = "trident"

	ownerRef := make(map[string]string)
	ownerRef["uid"] = "123456789"
	ownerRef["kind"] = "TridentProvisioner"

	imagePullSecrets := []string{"thisisasecret"}

	version := utils.MustParseSemantic("1.17.0")

	yamlsOutputs := []string{
		GetCSIDeploymentYAML("trident-csi", "netapp/trident:20.10.0-custom", "netapp/trident-autosupport:20.10.0-custom",
			"http://127.0.0.1/", "http://172.16.150.125:8888/", "0000-0000", "21e160d3-721f-4ec4-bcd4-c5e0d31d1a6e",
			"k8s.gcr.io", "text", "v1", imagePullSecrets, labels, nil, true, true, false, version, true),
	}
	for i, yamlData := range yamlsOutputs {

		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected constant %v to be valid YAML", i)
		}
	}
}

// Simple validation of the CSI Deployment YAML
func TestValidateGetCSIDeploymentYAMLFail(t *testing.T) {

	labels := make(map[string]string)
	labels["app"] = "trident"

	ownerRef := make(map[string]string)
	ownerRef["uid"] = "123456789"
	ownerRef["kind"] = "TridentProvisioner"

	imagePullSecrets := []string{"thisisasecret"}

	version := utils.MustParseSemantic("1.17.0")

	yamlsOutputs := []string{
		GetCSIDeploymentYAML("\ntrident-csi", "netapp/trident:20.10.0-custom", "netapp/trident-autosupport:20.10.0-custom",
			"http://127.0.0.1/", "http://172.16.150.125:8888/", "0000-0000", "21e160d3-721f-4ec4-bcd4-c5e0d31d1a6e",
			"k8s.gcr.io", "text", "v1beta1", imagePullSecrets, labels, nil, true, true, false, version, true),
	}

	for i, yamlData := range yamlsOutputs {

		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err == nil {
			t.Fatalf("expected constant %v to be invalid YAML", i)
		}
	}
}
