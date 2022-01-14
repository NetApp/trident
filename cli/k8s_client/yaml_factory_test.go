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
	for _, yamlData := range yamlsOutputs {
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf(err.Error(), yamlData)
		}
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

func TestGetCSIDeploymentYAML(t *testing.T) {
	versions := []string{"1.17.0", "1.18.0", "1.19.1", "1.21.0"}

	for _, versionString := range versions {
		version := utils.MustParseSemantic(versionString)
		deploymentArgs := &DeploymentYAMLArguments{Version: version}

		yamlData := GetCSIDeploymentYAML(deploymentArgs)
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected valid YAML for version %s", versionString)
		}
	}
}

func TestGetCSIDeploymentYAML_NodeSelectors(t *testing.T) {
	deploymentArgs := &DeploymentYAMLArguments{
		NodeSelector: map[string]string{"foo": "bar"},
	}
	expectedNodeSelectorString := `
      nodeSelector:
        kubernetes.io/os: linux
        kubernetes.io/arch: amd64
        foo: bar
`

	yamlData := GetCSIDeploymentYAML(deploymentArgs)
	_, err := yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedNodeSelectorString, fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))

	// Defaults
	deploymentArgs = &DeploymentYAMLArguments{}
	expectedNodeSelectorString = `
      nodeSelector:
        kubernetes.io/os: linux
        kubernetes.io/arch: amd64
`

	yamlData = GetCSIDeploymentYAML(deploymentArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedNodeSelectorString, fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))

	// Defaults used when empty list specified
	deploymentArgs = &DeploymentYAMLArguments{
		NodeSelector: map[string]string{},
	}
	expectedNodeSelectorString = `
      nodeSelector:
        kubernetes.io/os: linux
        kubernetes.io/arch: amd64
`

	yamlData = GetCSIDeploymentYAML(deploymentArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedNodeSelectorString, fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))
}

func TestGetCSIDeploymentYAMLTolerations(t *testing.T) {
	deploymentArgs := &DeploymentYAMLArguments{
		Tolerations: []map[string]string{
			{"key": "foo", "value": "bar", "operator": "Exists", "effect": "NoSchedule"},
			{"key": "foo2", "value": "bar2", "operator": "Equals", "effect": "NoExecute", "tolerationSeconds": "20"},
		},
	}
	expectedTolerationString := `
      tolerations:
      - key: "foo"
        value: "bar"
        effect: "NoSchedule"
        operator: "Exists"
      - key: "foo2"
        value: "bar2"
        effect: "NoExecute"
        operator: "Equals"
        tolerationSeconds: 20
`

	yamlData := GetCSIDeploymentYAML(deploymentArgs)
	_, err := yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedTolerationString, fmt.Sprintf("expected toleration in final YAML: %s", yamlData))

	// Test empty tolerations specified
	deploymentArgs = &DeploymentYAMLArguments{Tolerations: []map[string]string{}}
	expectedTolerationString = `
      tolerations: []
`
	defaultTolerationString := `
      tolerations:
      - effect: "NoExecute"
        operator: "Exists"
      - effect: "NoSchedule"
        operator: "Exists"
`

	yamlData = GetCSIDeploymentYAML(deploymentArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedTolerationString, fmt.Sprintf("expected toleration in final YAML: %s", yamlData))
	assert.NotContains(t, yamlData, defaultTolerationString, fmt.Sprintf("expected default tolerations to not appear in final YAML: %s", yamlData))
}

func TestGetCSIDaemonSetYAML(t *testing.T) {
	versions := []string{"1.17.0", "1.18.0", "1.21.0"}

	for _, versionString := range versions {
		version := utils.MustParseSemantic(versionString)
		daemonsetArgs := &DaemonsetYAMLArguments{Version: version}

		yamlData := GetCSIDaemonSetYAML(daemonsetArgs)
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected valid YAML for version %s", versionString)
		}
	}
}

func TestGetCSIDaemonSetYAML_NodeSelectors(t *testing.T) {
	daemonsetArgs := &DaemonsetYAMLArguments{
		NodeSelector: map[string]string{"foo": "bar"},
	}
	expectedNodeSelectorString := `
      nodeSelector:
        kubernetes.io/os: linux
        kubernetes.io/arch: amd64
        foo: bar
`

	yamlData := GetCSIDaemonSetYAML(daemonsetArgs)
	_, err := yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedNodeSelectorString, fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))

	// Defaults
	daemonsetArgs = &DaemonsetYAMLArguments{}
	expectedNodeSelectorString = `
      nodeSelector:
        kubernetes.io/os: linux
        kubernetes.io/arch: amd64
`

	yamlData = GetCSIDaemonSetYAML(daemonsetArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedNodeSelectorString, fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))

	// Defaults used when empty list specified
	daemonsetArgs = &DaemonsetYAMLArguments{
		NodeSelector: map[string]string{},
	}
	expectedNodeSelectorString = `
      nodeSelector:
        kubernetes.io/os: linux
        kubernetes.io/arch: amd64
`

	yamlData = GetCSIDaemonSetYAML(daemonsetArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedNodeSelectorString, fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))
}

func TestGetCSIDaemonSetYAMLTolerations(t *testing.T) {
	daemonsetArgs := &DaemonsetYAMLArguments{
		Tolerations: []map[string]string{
			{"key": "foo", "value": "bar", "operator": "Exists", "effect": "NoSchedule"},
			{"key": "foo2", "value": "bar2", "operator": "Equals", "effect": "NoExecute", "tolerationSeconds": "20"},
		},
	}
	expectedTolerationString := `
      tolerations:
      - key: "foo"
        value: "bar"
        effect: "NoSchedule"
        operator: "Exists"
      - key: "foo2"
        value: "bar2"
        effect: "NoExecute"
        operator: "Equals"
        tolerationSeconds: 20
`

	yamlData := GetCSIDaemonSetYAML(daemonsetArgs)
	_, err := yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedTolerationString, fmt.Sprintf("expected toleration in final YAML: %s", yamlData))

	// Test default
	daemonsetArgs = &DaemonsetYAMLArguments{}
	expectedTolerationString = `
      tolerations:
      - effect: "NoExecute"
        operator: "Exists"
      - effect: "NoSchedule"
        operator: "Exists"
`

	yamlData = GetCSIDaemonSetYAML(daemonsetArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedTolerationString, fmt.Sprintf("expected toleration in final YAML: %s", yamlData))

	// Test empty tolerations specified
	daemonsetArgs = &DaemonsetYAMLArguments{Tolerations: []map[string]string{}}
	expectedTolerationString = `
      tolerations: []
`
	defaultTolerationString := `
      tolerations:
      - effect: "NoExecute"
        operator: "Exists"
      - effect: "NoSchedule"
        operator: "Exists"
`

	yamlData = GetCSIDaemonSetYAML(daemonsetArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedTolerationString, fmt.Sprintf("expected toleration in final YAML: %s", yamlData))
	assert.NotContains(t, yamlData, defaultTolerationString, fmt.Sprintf("expected default tolerations to not appear in final YAML: %s", yamlData))
}
