// Copyright 2019 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"fmt"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
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

	deploymentArgs := &DeploymentYAMLArguments{
		DeploymentName:          Name,
		TridentImage:            ImageName,
		AutosupportImage:        AutosupportImageName,
		AutosupportProxy:        "",
		AutosupportCustomURL:    "",
		AutosupportSerialNumber: "",
		AutosupportHostname:     "",
		LogFormat:               LogFormat,
		ImageRegistry:           "",
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    ownerRef,
		Debug:                   false,
		UseIPv6:                 false,
		SilenceAutosupport:      false,
		Version:                 version,
		TopologyEnabled:         false,
		HTTPRequestTimeout:      config.HTTPTimeoutString,
	}

	yamlsOutputs := []string{
		GetServiceAccountYAML(Name, nil, nil, nil),
		GetServiceAccountYAML(Name, Secrets, labels, ownerRef),
		GetClusterRoleYAML(FlavorK8s, Name, nil, nil, false),
		GetClusterRoleYAML(FlavorOpenshift, Name, labels, ownerRef, true),
		GetClusterRoleBindingYAML(Namespace, FlavorOpenshift, Name, nil, ownerRef, false),
		GetClusterRoleBindingYAML(Namespace, FlavorK8s, Name, labels, ownerRef, true),
		GetCSIDeploymentYAML(deploymentArgs),
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

	deploymentArgs := &DeploymentYAMLArguments{
		DeploymentName:          "trident-csi",
		TridentImage:            "netapp/trident:20.10.0-custom",
		AutosupportImage:        "netapp/trident-autosupport:20.10.0-custom",
		AutosupportSerialNumber: "0000-0000",
		AutosupportHostname:     "21e160d3-721f-4ec4-bcd4-c5e0d31d1a6e",
		AutosupportProxy:        "http://127.0.0.1/",
		AutosupportCustomURL:    "http://172.16.150.125:8888/",
		ImageRegistry:           "k8s.gcr.io",
		LogFormat:               "text",
		SnapshotCRDVersion:      "v1",
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    map[string]string{},
		Version:                 version,
		HTTPRequestTimeout:      config.HTTPTimeoutString,
		TopologyEnabled:         true,
		Debug:                   true,
		UseIPv6:                 true,
		SilenceAutosupport:      false,
	}

	yamlsOutputs := []string{
		GetCSIDeploymentYAML(deploymentArgs),
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

	deploymentArgs := &DeploymentYAMLArguments{
		DeploymentName:          "\ntrident-csi",
		TridentImage:            "netapp/trident:20.10.0-custom",
		AutosupportImage:        "netapp/trident-autosupport:20.10.0-custom",
		AutosupportProxy:        "http://127.0.0.1/",
		AutosupportCustomURL:    "http://172.16.150.125:8888/",
		AutosupportSerialNumber: "0000-0000",
		AutosupportHostname:     "21e160d3-721f-4ec4-bcd4-c5e0d31d1a6e",
		ImageRegistry:           "k8s.gcr.io",
		LogFormat:               "text",
		SnapshotCRDVersion:      "v1beta1",
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    map[string]string{},
		Debug:                   true,
		UseIPv6:                 true,
		SilenceAutosupport:      false,
		Version:                 version,
		TopologyEnabled:         true,
		HTTPRequestTimeout:      config.HTTPTimeoutString,
	}

	yamlsOutputs := []string{
		GetCSIDeploymentYAML(deploymentArgs),
	}

	for i, yamlData := range yamlsOutputs {

		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err == nil {
			t.Fatalf("expected constant %v to be invalid YAML", i)
		}
	}
}
