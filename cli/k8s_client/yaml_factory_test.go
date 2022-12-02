// Copyright 2022 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
	scc "github.com/openshift/api/security/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	pspv1beta1 "k8s.io/api/policy/v1beta1"
	csiv1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	TridentAppKey   = "app"
	TridentAppValue = "csi.trident.netapp.io"
	TridentAppLabel = TridentAppKey + "=" + TridentAppValue

	CRAPIVersionKey = "apiVersion"
	CRController    = "controller"
	CRKind          = "kind"
	CRName          = "name"
	CRUID           = "uid"
)

var (
	Secrets      = []string{"thisisasecret1", "thisisasecret2"}
	SubjectNames = []string{"name1", "name2"}
)

// TestYAML simple validation of the YAML
func TestYAML(t *testing.T) {
	yamls := []string{
		namespaceYAMLTemplate,
		openShiftSCCQueryYAMLTemplate,
		customResourceDefinitionYAMLv1,
	}
	for i, yamlData := range yamls {
		// jsonData, err := yaml.YAMLToJSON([]byte(yamlData))
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected constant %v to be valid YAML", i)
		}
		// fmt.Printf("json: %v", string(jsonData))
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
		GetRoleYAML(FlavorK8s, Namespace, Name, labels, ownerRef, false),
		GetRoleYAML(FlavorOpenshift, Namespace, Name, labels, ownerRef, true),
		GetRoleBindingYAML(FlavorK8s, Namespace, Name, labels, ownerRef, false),
		GetRoleBindingYAML(FlavorOpenshift, Namespace, Name, labels, ownerRef, true),
		GetClusterRoleYAML(FlavorK8s, Name, nil, nil, false),
		GetClusterRoleYAML(FlavorOpenshift, Name, labels, ownerRef, true),
		GetClusterRoleBindingYAML(Namespace, Name, FlavorOpenshift, nil, ownerRef, false),
		GetClusterRoleBindingYAML(Namespace, Name, FlavorK8s, labels, ownerRef, true),
		GetCSIDeploymentYAML(deploymentArgs),
		GetCSIServiceYAML(Name, labels, ownerRef),
		GetSecretYAML(Name, Namespace, labels, ownerRef, nil, nil),
		GetResourceQuotaYAML(Name, Namespace, labels, nil),
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
		GetClusterRoleYAML(FlavorK8s, Name, nil, nil, true):                         "rbac.authorization.k8s.io/v1",
		GetClusterRoleYAML(FlavorOpenshift, Name, nil, nil, true):                   "rbac.authorization.k8s.io/v1",
		GetRoleYAML(FlavorK8s, Namespace, Name, nil, nil, false):                    "rbac.authorization.k8s.io/v1",
		GetRoleYAML(FlavorK8s, Namespace, Name, nil, nil, true):                     "rbac.authorization.k8s.io/v1",
		GetRoleYAML(FlavorOpenshift, Namespace, Name, nil, nil, true):               "rbac.authorization.k8s.io/v1",
		GetRoleBindingYAML(FlavorK8s, Namespace, Name, nil, nil, false):             "rbac.authorization.k8s.io/v1",
		GetRoleBindingYAML(FlavorK8s, Namespace, Name, nil, nil, true):              "rbac.authorization.k8s.io/v1",
		GetRoleBindingYAML(FlavorOpenshift, Namespace, Name, nil, nil, true):        "rbac.authorization.k8s.io/v1",
		GetClusterRoleBindingYAML(Namespace, Name, FlavorK8s, nil, nil, false):      "rbac.authorization.k8s.io/v1",
		GetClusterRoleBindingYAML(Namespace, Name, FlavorK8s, nil, nil, true):       "rbac.authorization.k8s.io/v1",
		GetClusterRoleBindingYAML(Namespace, Name, FlavorOpenshift, nil, nil, true): "rbac.authorization.k8s.io/v1",
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

	version := utils.MustParseSemantic("1.20.0")

	deploymentArgs := &DeploymentYAMLArguments{
		DeploymentName:          "trident-csi",
		TridentImage:            "netapp/trident:20.10.0-custom",
		AutosupportImage:        "netapp/trident-autosupport:20.10.0-custom",
		AutosupportSerialNumber: "0000-0000",
		AutosupportHostname:     "21e160d3-721f-4ec4-bcd4-c5e0d31d1a6e",
		AutosupportProxy:        "http://127.0.0.1/",
		AutosupportCustomURL:    "http://172.16.150.125:8888/",
		ImageRegistry:           "registry.k8s.io",
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

	version := utils.MustParseSemantic("1.20.0")

	deploymentArgs := &DeploymentYAMLArguments{
		DeploymentName:          "\ntrident-csi",
		TridentImage:            "netapp/trident:20.10.0-custom",
		AutosupportImage:        "netapp/trident-autosupport:20.10.0-custom",
		AutosupportProxy:        "http://127.0.0.1/",
		AutosupportCustomURL:    "http://172.16.150.125:8888/",
		AutosupportSerialNumber: "0000-0000",
		AutosupportHostname:     "21e160d3-721f-4ec4-bcd4-c5e0d31d1a6e",
		ImageRegistry:           "registry.k8s.io",
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
	versions := []string{"1.21.0", "1.23.0", "1.25.0"}

	for _, versionString := range versions {
		version := utils.MustParseSemantic(versionString)
		deploymentArgs := &DeploymentYAMLArguments{Version: version, SnapshotCRDVersion: "v1"}

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
        foo: 'bar'
`

	yamlData := GetCSIDeploymentYAML(deploymentArgs)
	_, err := yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedNodeSelectorString,
		fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))

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
	assert.Contains(t, yamlData, expectedNodeSelectorString,
		fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))

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
	assert.Contains(t, yamlData, expectedNodeSelectorString,
		fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))
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
	assert.Contains(t, yamlData, expectedTolerationString,
		fmt.Sprintf("expected toleration in final YAML: %s", yamlData))

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
	assert.Contains(t, yamlData, expectedTolerationString,
		fmt.Sprintf("expected toleration in final YAML: %s", yamlData))
	assert.NotContains(t, yamlData, defaultTolerationString,
		fmt.Sprintf("expected default tolerations to not appear in final YAML: %s", yamlData))
}

func TestGetCSIDeploymentYAMLImagePullPolicy(t *testing.T) {
	versions := []string{"1.20.0"}
	expectedStr := `imagePullPolicy: %s`

	expectedIfNotPresent := fmt.Sprintf(expectedStr, v1.PullIfNotPresent)
	expectedAlways := fmt.Sprintf(expectedStr, v1.PullAlways)
	expectedNever := fmt.Sprintf(expectedStr, v1.PullNever)

	type Args struct {
		imagePullPolicy string
		Expected        string
		FailStr         string
	}

	testArgs := []*Args{
		{
			imagePullPolicy: string(v1.PullIfNotPresent),
			Expected:        expectedIfNotPresent,
			FailStr:         string(v1.PullIfNotPresent),
		},
		{
			imagePullPolicy: string(v1.PullAlways),
			Expected:        expectedAlways,
			FailStr:         string(v1.PullAlways),
		},
		{
			imagePullPolicy: string(v1.PullNever),
			Expected:        expectedNever,
			FailStr:         string(v1.PullNever),
		},
	}

	for _, args := range testArgs {
		for _, versionString := range versions {
			version := utils.MustParseSemantic(versionString)
			daemonsetArgs := &DeploymentYAMLArguments{Version: version, ImagePullPolicy: args.imagePullPolicy}

			yamlData := GetCSIDeploymentYAML(daemonsetArgs)
			_, err := yaml.YAMLToJSON([]byte(yamlData))
			if err != nil {
				t.Fatalf("expected valid YAML for version %s", versionString)
			}
			failMsg := fmt.Sprintf("expected imagPullPolicy to be %s in final YAML: %s", args.FailStr, yamlData)
			assert.Contains(t, yamlData, args.Expected, failMsg)
		}
	}
}

func TestGetCSIDaemonSetYAMLLinux(t *testing.T) {
	versions := []string{"1.21.0", "1.23.0", "1.25.0"}

	for _, versionString := range versions {
		version := utils.MustParseSemantic(versionString)
		daemonsetArgs := &DaemonsetYAMLArguments{Version: version}

		yamlData := GetCSIDaemonSetYAMLLinux(daemonsetArgs)
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected valid YAML for version %s", versionString)
		}
	}
}

func TestGetCSIDaemonSetYAMLLinux_DebugIsTrue(t *testing.T) {
	versions := []string{"1.17.0"}

	for _, versionString := range versions {
		version := utils.MustParseSemantic(versionString)
		daemonsetArgs := &DaemonsetYAMLArguments{Version: version, Debug: true}

		yamlData := GetCSIDaemonSetYAMLLinux(daemonsetArgs)
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected valid YAML for version %s", versionString)
		}
	}
}

func TestGetCSIDaemonSetYAMLLinux_ForceDetach(t *testing.T) {
	versions := []string{"1.20.0"}
	expectedStr := `- "--enable_force_detach=%s"`
	disabled := "false"
	enabled := "true"
	expectedDisabled := fmt.Sprintf(expectedStr, disabled)
	expectedEnabled := fmt.Sprintf(expectedStr, enabled)

	type Args struct {
		Enabled    bool
		Expected   string
		Unexpected string
		FailStr    string
		FailStr2   string
	}

	testArgs := []*Args{
		{
			Enabled:    false,
			Expected:   expectedDisabled,
			Unexpected: expectedEnabled,
			FailStr:    disabled,
			FailStr2:   enabled,
		},
		{
			Enabled:    true,
			Expected:   expectedEnabled,
			Unexpected: expectedDisabled,
			FailStr:    enabled,
			FailStr2:   disabled,
		},
	}

	for _, args := range testArgs {
		for _, versionString := range versions {
			version := utils.MustParseSemantic(versionString)
			daemonsetArgs := &DaemonsetYAMLArguments{Version: version, EnableForceDetach: args.Enabled}

			yamlData := GetCSIDaemonSetYAMLLinux(daemonsetArgs)
			_, err := yaml.YAMLToJSON([]byte(yamlData))
			if err != nil {
				t.Fatalf("expected valid YAML for version %s", versionString)
			}
			failMsg := fmt.Sprintf("expected enableForceDetach to be %s in final YAML: %s", args.FailStr, yamlData)
			assert.Contains(t, yamlData, args.Expected, failMsg)
			failMsg2 := fmt.Sprintf("did not expect enableForceDetach to be %s in final YAML: %s", args.FailStr2, yamlData)
			assert.NotContains(t, yamlData, args.Unexpected, failMsg2)
		}
	}
}

func TestGetCSIDaemonSetYAMLLinuxImagePullPolicy(t *testing.T) {
	versions := []string{"1.20.0"}
	expectedStr := `imagePullPolicy: %s`

	expectedIfNotPresent := fmt.Sprintf(expectedStr, v1.PullIfNotPresent)
	expectedAlways := fmt.Sprintf(expectedStr, v1.PullAlways)
	expectedNever := fmt.Sprintf(expectedStr, v1.PullNever)

	type Args struct {
		imagePullPolicy string
		Expected        string
		FailStr         string
	}

	testArgs := []*Args{
		{
			imagePullPolicy: string(v1.PullIfNotPresent),
			Expected:        expectedIfNotPresent,
			FailStr:         string(v1.PullIfNotPresent),
		},
		{
			imagePullPolicy: string(v1.PullAlways),
			Expected:        expectedAlways,
			FailStr:         string(v1.PullAlways),
		},
		{
			imagePullPolicy: string(v1.PullNever),
			Expected:        expectedNever,
			FailStr:         string(v1.PullNever),
		},
	}

	for _, args := range testArgs {
		for _, versionString := range versions {
			version := utils.MustParseSemantic(versionString)
			daemonsetArgs := &DaemonsetYAMLArguments{Version: version, ImagePullPolicy: args.imagePullPolicy}

			yamlData := GetCSIDaemonSetYAMLLinux(daemonsetArgs)
			_, err := yaml.YAMLToJSON([]byte(yamlData))
			if err != nil {
				t.Fatalf("expected valid YAML for version %s", versionString)
			}
			failMsg := fmt.Sprintf("expected imagPullPolicy to be %s in final YAML: %s", args.FailStr, yamlData)
			assert.Contains(t, yamlData, args.Expected, failMsg)
		}
	}
}

func TestGetCSIDaemonSetYAMLLinux_NodeSelectors(t *testing.T) {
	daemonsetArgs := &DaemonsetYAMLArguments{
		NodeSelector: map[string]string{"foo": "bar"},
	}
	expectedNodeSelectorString := `
      nodeSelector:
        kubernetes.io/os: linux
        kubernetes.io/arch: amd64
        foo: 'bar'
`

	yamlData := GetCSIDaemonSetYAMLLinux(daemonsetArgs)
	_, err := yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedNodeSelectorString,
		fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))

	// Defaults
	daemonsetArgs = &DaemonsetYAMLArguments{}
	expectedNodeSelectorString = `
      nodeSelector:
        kubernetes.io/os: linux
        kubernetes.io/arch: amd64
`

	yamlData = GetCSIDaemonSetYAMLLinux(daemonsetArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedNodeSelectorString,
		fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))

	// Defaults used when empty list specified
	daemonsetArgs = &DaemonsetYAMLArguments{
		NodeSelector: map[string]string{},
	}
	expectedNodeSelectorString = `
      nodeSelector:
        kubernetes.io/os: linux
        kubernetes.io/arch: amd64
`

	yamlData = GetCSIDaemonSetYAMLLinux(daemonsetArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedNodeSelectorString,
		fmt.Sprintf("expected nodeSelector in final YAML: %s", yamlData))
}

func TestGetCSIDaemonSetYAMLLinux_Tolerations(t *testing.T) {
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

	yamlData := GetCSIDaemonSetYAMLLinux(daemonsetArgs)
	_, err := yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedTolerationString,
		fmt.Sprintf("expected toleration in final YAML: %s", yamlData))

	// Test default
	daemonsetArgs = &DaemonsetYAMLArguments{}
	expectedTolerationString = `
      tolerations:
      - effect: "NoExecute"
        operator: "Exists"
      - effect: "NoSchedule"
        operator: "Exists"
`

	yamlData = GetCSIDaemonSetYAMLLinux(daemonsetArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedTolerationString,
		fmt.Sprintf("expected toleration in final YAML: %s", yamlData))

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

	yamlData = GetCSIDaemonSetYAMLLinux(daemonsetArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedTolerationString,
		fmt.Sprintf("expected toleration in final YAML: %s", yamlData))
	assert.NotContains(t, yamlData, defaultTolerationString,
		fmt.Sprintf("expected default tolerations to not appear in final YAML: %s", yamlData))
}

func TestGetCSIDaemonSetYAMLWindows(t *testing.T) {
	versions := []string{"1.21.0", "1.23.0", "1.25.0"}

	for _, versionString := range versions {
		version := utils.MustParseSemantic(versionString)
		daemonsetArgs := &DaemonsetYAMLArguments{Version: version, Debug: true}

		yamlData := GetCSIDaemonSetYAMLWindows(daemonsetArgs)
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected valid YAML for version %s, error: %v", versionString, err)
		}
	}
}

func TestGetCSIDaemonSetYAMLWindows_DebugIsFalse(t *testing.T) {
	versions := []string{"1.21.0", "1.23.0", "1.25.0"}

	for _, versionString := range versions {
		version := utils.MustParseSemantic(versionString)
		daemonsetArgs := &DaemonsetYAMLArguments{Version: version, Debug: false}

		yamlData := GetCSIDaemonSetYAMLWindows(daemonsetArgs)
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected valid YAML for version %s", versionString)
		}
	}
}

func TestGetCSIDaemonSetYAMLWindowsImagePullPolicy(t *testing.T) {
	versions := []string{"1.20.0"}
	expectedStr := `imagePullPolicy: %s`

	expectedIfNotPresent := fmt.Sprintf(expectedStr, string(v1.PullIfNotPresent))
	expectedAlways := fmt.Sprintf(expectedStr, string(v1.PullAlways))
	expectedNever := fmt.Sprintf(expectedStr, string(v1.PullNever))

	type Args struct {
		imagePullPolicy string
		Expected        string
		FailStr         string
	}

	testArgs := []*Args{
		{
			imagePullPolicy: string(v1.PullIfNotPresent),
			Expected:        expectedIfNotPresent,
			FailStr:         string(v1.PullIfNotPresent),
		},
		{
			imagePullPolicy: string(v1.PullAlways),
			Expected:        expectedAlways,
			FailStr:         string(v1.PullAlways),
		},
		{
			imagePullPolicy: string(v1.PullNever),
			Expected:        expectedNever,
			FailStr:         string(v1.PullNever),
		},
	}

	for _, args := range testArgs {
		for _, versionString := range versions {
			version := utils.MustParseSemantic(versionString)
			daemonsetArgs := &DaemonsetYAMLArguments{Version: version, ImagePullPolicy: args.imagePullPolicy}

			yamlData := GetCSIDaemonSetYAMLWindows(daemonsetArgs)
			_, err := yaml.YAMLToJSON([]byte(yamlData))
			if err != nil {
				t.Fatalf("expected valid YAML for version %s", versionString)
			}
			failMsg := fmt.Sprintf("expected imagPullPolicy to be %s in final YAML: %s", args.FailStr, yamlData)
			assert.Contains(t, yamlData, args.Expected, failMsg)
		}
	}
}

func TestConstructNodeSelector(t *testing.T) {
	nodeSelMap := map[string]string{"worker": "true", "master": "20"}
	expectedNodeSelString := []string{"worker: 'true'\nmaster: '20'\n", "master: '20'\nworker: 'true'\n"}
	result := constructNodeSelector(nodeSelMap)
	isResultExpected := false
	for _, v := range expectedNodeSelString {
		if v == result {
			isResultExpected = true
		}
	}
	assert.True(t, isResultExpected)
}

func TestGetNamespaceYAML(t *testing.T) {
	expected := v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "trident",
			Labels: map[string]string{
				"pod-security.kubernetes.io/enforce": "privileged",
			},
		},
	}

	var actual v1.Namespace
	actualYAML := GetNamespaceYAML("trident")

	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.Equal(t, "trident", actual.Name)
}

func TestGetTridentVersionPodYAML(t *testing.T) {
	name := "trident-csi"
	image := "trident-csi-image"
	secrets := []string{"trident-csi-image-secret"}
	labels := map[string]string{"app": "controller.csi.trident.netapp.io"}
	crdDetails := map[string]string{"kind": "ReplicaSet"}

	expected := v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "trident-csi",
			Labels: map[string]string{
				"app": "controller.csi.trident.netapp.io",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
				},
			},
		},
		Spec: v1.PodSpec{
			DeprecatedServiceAccount: "service",
			RestartPolicy:            "Never",
			Containers: []v1.Container{
				{
					Name:            "trident-main",
					ImagePullPolicy: "IfNotPresent",
					Image:           "trident-csi-image",
					Command:         []string{"tridentctl"},
					Args:            []string{"pause"},
				},
			},
			ImagePullSecrets: []v1.LocalObjectReference{
				{
					Name: "trident-csi-image-secret",
				},
			},
			NodeSelector: map[string]string{
				"beta.kubernetes.io/os":   "linux",
				"beta.kubernetes.io/arch": "amd64",
			},
		},
	}

	var actual v1.Pod
	actualYAML := GetTridentVersionPodYAML(name, image, "service", "IfNotPresent", secrets, labels, crdDetails)
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetOpenShiftSCCYAML(t *testing.T) {
	sccName := "trident-node-linux"
	user := "trident-node-linux"
	namespace := "trident"
	labels := map[string]string{"app": "node.csi.trident.netapp.io"}
	crdDetails := map[string]string{"kind": "ReplicaSet"}
	allowPrivilegeEscalation := true

	expected := scc.SecurityContextConstraints{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SecurityContextConstraints",
			APIVersion: "security.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/description": "trident-node-linux is a clone of the privileged built-in, " +
					"and is meant just for use with trident.",
			},
			Name: "trident-node-linux",
			Labels: map[string]string{
				"app": "node.csi.trident.netapp.io",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
				},
			},
		},
		AllowHostDirVolumePlugin: true,
		AllowHostIPC:             true,
		AllowHostNetwork:         true,
		AllowHostPID:             true,
		AllowHostPorts:           false,
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		AllowPrivilegedContainer: true,
		DefaultAddCapabilities:   nil,
		FSGroup: scc.FSGroupStrategyOptions{
			Type: "RunAsAny",
		},
		Groups:                   []string{},
		Priority:                 nil,
		ReadOnlyRootFilesystem:   false,
		RequiredDropCapabilities: nil,
		RunAsUser: scc.RunAsUserStrategyOptions{
			Type: "RunAsAny",
		},
		SELinuxContext: scc.SELinuxContextStrategyOptions{
			Type: "RunAsAny",
		},
		SupplementalGroups: scc.SupplementalGroupsStrategyOptions{
			Type: "RunAsAny",
		},
		Users:   []string{"system:serviceaccount:trident:trident-node-linux"},
		Volumes: []scc.FSType{"hostPath", "downwardAPI", "projected", "emptyDir"},
	}

	var actual scc.SecurityContextConstraints

	actualYAML := GetOpenShiftSCCYAML(sccName, user, namespace, labels, crdDetails, true)
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.Equal(t, expected, actual)
}

func TestGetOpenShiftSCCYAML_UnprivilegedUser(t *testing.T) {
	sccName := "trident-controller"
	user := "trident-controller"
	namespace := "trident"
	labels := map[string]string{"app": "controller.trident.netapp.io"}
	crdDetails := map[string]string{"kind": "ReplicaSet"}
	priority := int32(10)

	expected := scc.SecurityContextConstraints{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SecurityContextConstraints",
			APIVersion: "security.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"kubernetes.io/description": "trident-controller is a clone of the anyuid built-in, and is meant just for use with trident.",
			},
			Name: "trident-controller",
			Labels: map[string]string{
				"app": "controller.trident.netapp.io",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
				},
			},
		},
		AllowHostDirVolumePlugin: false,
		AllowHostIPC:             false,
		AllowHostNetwork:         false,
		AllowHostPID:             false,
		AllowHostPorts:           false,
		AllowPrivilegedContainer: false,
		AllowedCapabilities:      []v1.Capability{},
		DefaultAddCapabilities:   []v1.Capability{},
		FSGroup: scc.FSGroupStrategyOptions{
			Type: "RunAsAny",
		},
		Groups:                   []string{},
		Priority:                 &priority,
		ReadOnlyRootFilesystem:   false,
		RequiredDropCapabilities: []v1.Capability{"MKNOD"},
		RunAsUser: scc.RunAsUserStrategyOptions{
			Type: "RunAsAny",
		},
		SELinuxContext: scc.SELinuxContextStrategyOptions{
			Type: "MustRunAs",
		},
		SupplementalGroups: scc.SupplementalGroupsStrategyOptions{
			Type: "RunAsAny",
		},
		Users:   []string{"system:serviceaccount:trident:trident-controller"},
		Volumes: []scc.FSType{"hostPath", "downwardAPI", "projected", "emptyDir"},
	}

	var actual scc.SecurityContextConstraints

	actualYAML := GetOpenShiftSCCYAML(sccName, user, namespace, labels, crdDetails, false)
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.AllowHostDirVolumePlugin, actual.AllowHostDirVolumePlugin))
	assert.True(t, reflect.DeepEqual(expected.AllowHostIPC, actual.AllowHostIPC))
	assert.True(t, reflect.DeepEqual(expected.AllowHostNetwork, actual.AllowHostNetwork))
	assert.True(t, reflect.DeepEqual(expected.AllowHostPID, actual.AllowHostPID))
	assert.True(t, reflect.DeepEqual(expected.AllowHostPorts, actual.AllowHostPorts))
	assert.True(t, reflect.DeepEqual(expected.AllowPrivilegedContainer, actual.AllowPrivilegedContainer))
	assert.Empty(t, expected.AllowedCapabilities, actual.AllowedCapabilities)
	assert.Empty(t, expected.DefaultAddCapabilities, actual.DefaultAddCapabilities)
	assert.True(t, reflect.DeepEqual(expected.FSGroup, actual.FSGroup))
	assert.True(t, reflect.DeepEqual(expected.Groups, actual.Groups))
	assert.True(t, reflect.DeepEqual(expected.Priority, actual.Priority))
	assert.True(t, reflect.DeepEqual(expected.ReadOnlyRootFilesystem, actual.ReadOnlyRootFilesystem))
	assert.True(t, reflect.DeepEqual(expected.RequiredDropCapabilities, actual.RequiredDropCapabilities))
	assert.True(t, reflect.DeepEqual(expected.RunAsUser, actual.RunAsUser))
	assert.True(t, reflect.DeepEqual(expected.SELinuxContext, actual.SELinuxContext))
	assert.True(t, reflect.DeepEqual(expected.SeccompProfiles, actual.SeccompProfiles))
	assert.True(t, reflect.DeepEqual(expected.SupplementalGroups, actual.SupplementalGroups))
	assert.True(t, reflect.DeepEqual(expected.Users, actual.Users))
	assert.True(t, reflect.DeepEqual(expected.Volumes, actual.Volumes))
}

func TestGetOpenShiftSCCQueryYAML(t *testing.T) {
	expected := scc.SecurityContextConstraints{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SecurityContextConstraints",
			APIVersion: "security.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "trident-scc",
		},
	}

	var actual scc.SecurityContextConstraints
	actualYAML := GetOpenShiftSCCQueryYAML("trident-scc")
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
}

func TestGetSecretYAML(t *testing.T) {
	secretName := "trident-csi-secret"
	namespace := "trident"
	labels := map[string]string{"app": "trident"}
	crdDetails := map[string]string{"kind": "ReplicaSet"}
	data := map[string]string{"caKey": "95AE119A6C3A07F56C4A"}
	stringData := map[string]string{"key": "value"}

	expected := v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "trident-csi-secret",
			Namespace: "trident",
			Labels: map[string]string{
				"app": "trident",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
				},
			},
		},
		Data: map[string][]byte{
			"caKey": []byte("95AE119A6C3A07F56C4A"),
		},
		StringData: map[string]string{
			"key": "value",
		},
	}

	var actual v1.Secret
	actualYAML := GetSecretYAML(secretName, namespace, labels, crdDetails, data, stringData)
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.StringData, actual.StringData))
}

func TestGetCRDsYAML(t *testing.T) {
	result := strings.Split(GetCRDsYAML(), "---")

	preserveValue := true
	minItems := int64(1)
	maxItems := int64(1)

	schema1 := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}
	schema2 := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				"spec": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"state": {
							Type: "string",
							Enum: []apiextensionsv1.JSON{
								{
									Raw: []byte("\"\""),
								},
								{
									Raw: []byte("\"promoted\""),
								},
								{
									Raw: []byte("\"established\""),
								},
								{
									Raw: []byte("\"reestablished\""),
								},
							},
						},
						"replicationPolicy": {
							Type: "string",
						},
						"replicationSchedule": {
							Type: "string",
						},
						"volumeMappings": {
							Items: &apiextensionsv1.JSONSchemaPropsOrArray{
								Schema: &apiextensionsv1.JSONSchemaProps{
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"promotedSnapshotHandle": {
											Type: "string",
										},
										"localPVCName": {
											Type: "string",
										},
										"remoteVolumeHandle": {
											Type: "string",
										},
									},
									Required: []string{"localPVCName"},
								},
							},
							MinItems: &minItems,
							MaxItems: &maxItems,
							Type:     "array",
						},
					},
					Required: []string{"volumeMappings"},
				},
				"status": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"conditions": {
							Type: "array",
							Items: &apiextensionsv1.JSONSchemaPropsOrArray{
								Schema: &apiextensionsv1.JSONSchemaProps{
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"lastTransitionTime": {
											Type: "string",
										},
										"localPVCName": {
											Type: "string",
										},
										"localVolumeHandle": {
											Type: "string",
										},
										"remoteVolumeHandle": {
											Type: "string",
										},
										"message": {
											Type: "string",
										},
										"observedGeneration": {
											Type: "integer",
										},
										"state": {
											Type: "string",
										},
										"replicationPolicy": {
											Type: "string",
										},
										"replicationSchedule": {
											Type: "string",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	schema3 := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				"spec": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"snapshotName": {
							Type: "string",
						},
					},
					Required: []string{"snapshotName"},
				},
				"status": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"lastTransitionTime": {
							Type: "string",
						},
						"observedGeneration": {
							Type: "integer",
						},
						"snapshotHandle": {
							Type: "string",
						},
					},
				},
			},
		},
	}
	schema4 := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				"volumeID": {
					Type: "string",
				},
				"nodeID": {
					Type: "string",
				},
				"readOnly": {
					Type: "boolean",
				},
				"accessMode": {
					Type:   "integer",
					Format: "int32",
				},
				"notSafeToAttach": {
					Type: "boolean",
				},
				"unpublished": {
					Type: "boolean",
				},
			},
			Required: []string{"volumeID", "nodeID", "readOnly"},
		},
	}
	schema5 := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				"spec": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"pvcName": {
							Type: "string",
						},
						"pvcNamespace": {
							Type: "string",
						},
					},
					Required: []string{"pvcName", "pvcNamespace"},
				},
			},
		},
	}

	expected1 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentversions.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentversions",
				Singular:   "tridentversion",
				Kind:       "TridentVersion",
				ShortNames: []string{"tver", "tversion"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema1,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Version",
							Type:        "string",
							Description: "The Trident version",
							Priority:    int32(0),
							JSONPath:    ".trident_version",
						},
					},
				},
			},
		},
	}
	expected2 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentbackends.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentbackends",
				Singular:   "tridentbackend",
				Kind:       "TridentBackend",
				ShortNames: []string{"tbe", "tbackend"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema1,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Backend",
							Type:        "string",
							Description: "The backend name",
							Priority:    int32(0),
							JSONPath:    ".backendName",
						},
						{
							Name:        "Backend UUID",
							Type:        "string",
							Description: "The backend UUID",
							Priority:    int32(0),
							JSONPath:    ".backendUUID",
						},
					},
				},
			},
		},
	}
	expected3 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentbackendconfigs.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentbackendconfigs",
				Singular:   "tridentbackendconfig",
				Kind:       "TridentBackendConfig",
				ShortNames: []string{"tbc", "tbconfig", "tbackendconfig"},
				Categories: []string{"trident", "trident-internal", "trident-external"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema1,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
						Scale:  nil,
					},
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Backend Name",
							Type:        "string",
							Description: "The backend name",
							Priority:    int32(0),
							JSONPath:    ".status.backendInfo.backendName",
						},
						{
							Name:        "Backend UUID",
							Type:        "string",
							Description: "The backend UUID",
							Priority:    int32(0),
							JSONPath:    ".status.backendInfo.backendUUID",
						},
						{
							Name:        "Phase",
							Type:        "string",
							Description: "The backend config phase",
							Priority:    int32(0),
							JSONPath:    ".status.phase",
						},
						{
							Name:        "Status",
							Type:        "string",
							Description: "The result of the last operation",
							Priority:    int32(0),
							JSONPath:    ".status.lastOperationStatus",
						},
						{
							Name:        "Storage Driver",
							Type:        "string",
							Description: "The storage driver type",
							Priority:    int32(1),
							JSONPath:    ".spec.storageDriverName",
						},
						{
							Name:        "Deletion Policy",
							Type:        "string",
							Description: "The deletion policy",
							Priority:    int32(1),
							JSONPath:    ".status.deletionPolicy",
						},
					},
				},
			},
		},
	}
	expected4 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentmirrorrelationships.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentmirrorrelationships",
				Singular:   "tridentmirrorrelationship",
				Kind:       "TridentMirrorRelationship",
				ShortNames: []string{"tmr", "tmrelationship", "tmirrorrelationship"},
				Categories: []string{"trident", "trident-internal", "trident-external"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema2,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
						Scale:  nil,
					},
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Desired State",
							Type:        "string",
							Description: "The desired mirror state",
							JSONPath:    ".spec.state",
						},
						{
							Name:        "Local PVC",
							Type:        "string",
							Description: "Local PVCs for the mirror",
							JSONPath:    ".spec.volumeMappings[*].localPVCName",
						},
						{
							Name:        "Actual state",
							Type:        "string",
							Description: "Status",
							JSONPath:    ".status.conditions[*].state",
						},
						{
							Name:        "Message",
							Type:        "string",
							Description: "Status message",
							JSONPath:    ".status.conditions[*].message",
						},
					},
				},
			},
		},
	}
	expected5 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentsnapshotinfos.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentsnapshotinfos",
				Singular:   "tridentsnapshotinfo",
				Kind:       "TridentSnapshotInfo",
				ShortNames: []string{"tsi", "tsinfo", "tsnapshotinfo"},
				Categories: []string{"trident", "trident-internal", "trident-external"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema3,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
						Scale:  nil,
					},
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Snapshot Handle",
							Type:        "string",
							Description: "VolumeSnapshotContent Handle",
							Priority:    int32(0),
							JSONPath:    ".status.snapshotHandle",
						},
					},
				},
			},
		},
	}
	expected6 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentstorageclasses.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentstorageclasses",
				Singular:   "tridentstorageclass",
				Kind:       "TridentStorageClass",
				ShortNames: []string{"tsc", "tstorageclass"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema1,
				},
			},
		},
	}
	expected7 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentvolumes.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentvolumes",
				Singular:   "tridentvolume",
				Kind:       "TridentVolume",
				ShortNames: []string{"tvol", "tvolume"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:         "v1",
					Served:       true,
					Storage:      true,
					Schema:       &schema1,
					Subresources: nil,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:     "Age",
							Type:     "date",
							Priority: int32(0),
							JSONPath: ".metadata.creationTimestamp",
						},
						{
							Name:        "Size",
							Type:        "string",
							Description: "The volume's size",
							Priority:    int32(1),
							JSONPath:    ".config.size",
						},
						{
							Name:        "Storage Class",
							Type:        "string",
							Description: "The volume's storage class",
							Priority:    int32(1),
							JSONPath:    ".config.storageClass",
						},
						{
							Name:        "State",
							Type:        "string",
							Description: "The volume's state",
							Priority:    int32(1),
							JSONPath:    ".state",
						},
						{
							Name:        "Protocol",
							Type:        "string",
							Description: "The volume's protocol",
							Priority:    int32(1),
							JSONPath:    ".config.protocol",
						},
						{
							Name:        "Backend UUID",
							Type:        "string",
							Description: "The volume's backend UUID",
							Priority:    int32(1),
							JSONPath:    ".backendUUID",
						},
						{
							Name:        "Pool",
							Type:        "string",
							Description: "The volume's pool",
							Priority:    int32(1),
							JSONPath:    ".pool",
						},
					},
				},
			},
		},
	}
	expected8 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentvolumepublications.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "tridentvolumepublications",
				Singular: "tridentvolumepublication",
				Kind:     "TridentVolumePublication",
				ShortNames: []string{
					"tvp",
					"tvpub",
					"tvpublication",
					"tvolpub",
					"tvolumepub",
					"tvolpublication",
					"tvolumepublication",
				},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema4,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Volume",
							Type:        "string",
							Description: "Volume ID",
							Priority:    int32(0),
							JSONPath:    ".volumeID",
						},
						{
							Name:        "Node",
							Type:        "string",
							Description: "Node ID",
							Priority:    int32(0),
							JSONPath:    ".nodeID",
						},
					},
				},
			},
		},
	}
	expected9 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentnodes.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentnodes",
				Singular:   "tridentnode",
				Kind:       "TridentNode",
				ShortNames: []string{"tnode"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema1,
				},
			},
		},
	}
	expected10 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridenttransactions.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridenttransactions",
				Singular:   "tridenttransaction",
				Kind:       "TridentTransaction",
				ShortNames: []string{"ttx", "ttransaction"},
				Categories: []string{"trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema1,
				},
			},
		},
	}
	expected11 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentsnapshots.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentsnapshots",
				Singular:   "tridentsnapshot",
				Kind:       "TridentSnapshot",
				ShortNames: []string{"tss", "tsnap", "tsnapshot"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema1,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "State",
							Type:        "string",
							Description: "The snapshot's state",
							Priority:    int32(1),
							JSONPath:    ".state",
						},
					},
				},
			},
		},
	}
	expected12 := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentvolumereferences.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentvolumereferences",
				Singular:   "tridentvolumereference",
				Kind:       "TridentVolumeReference",
				ShortNames: []string{"tvr", "tvref"},
				Categories: []string{"trident", "trident-external", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:                     "v1",
					Served:                   true,
					Storage:                  true,
					Schema:                   &schema5,
					AdditionalPrinterColumns: nil,
				},
			},
		},
	}

	var actual1 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[0]), &actual1), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected1.TypeMeta, actual1.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected1.ObjectMeta, actual1.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected1.Spec, actual1.Spec))

	var actual2 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[1]), &actual2), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected2.TypeMeta, actual2.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected2.ObjectMeta, actual2.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected2.Spec, actual2.Spec))

	var actual3 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[2]), &actual3), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected3.TypeMeta, actual3.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected3.ObjectMeta, actual3.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected3.Spec, actual3.Spec))

	var actual4 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[3]), &actual4), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected4.TypeMeta, actual4.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected4.ObjectMeta, actual4.ObjectMeta))
	assert.Equal(t, expected4.Spec, actual4.Spec)

	var actual5 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[4]), &actual5), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected5.TypeMeta, actual5.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected5.ObjectMeta, actual5.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected5.Spec, actual5.Spec))

	var actual6 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[5]), &actual6), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected6.TypeMeta, actual6.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected6.ObjectMeta, actual6.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected6.Spec, actual6.Spec))

	var actual7 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[6]), &actual7), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected7.TypeMeta, actual7.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected7.ObjectMeta, actual7.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected7.Spec, actual7.Spec))

	var actual8 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[7]), &actual8), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected8.TypeMeta, actual8.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected8.ObjectMeta, actual8.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected8.Spec, actual8.Spec))

	var actual9 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[8]), &actual9), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected9.TypeMeta, actual9.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected9.ObjectMeta, actual9.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected9.Spec, actual9.Spec))

	var actual10 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[9]), &actual10), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected10.TypeMeta, actual10.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected10.ObjectMeta, actual10.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected10.Spec, actual10.Spec))

	var actual11 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[10]), &actual11), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected11.TypeMeta, actual11.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected11.ObjectMeta, actual11.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected11.Spec, actual11.Spec))

	var actual12 apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(result[11]), &actual12), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected12.TypeMeta, actual12.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected12.ObjectMeta, actual12.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected12.Spec, actual12.Spec))
}

func TestGetVersionCRDYAML(t *testing.T) {
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentversions.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentversions",
				Singular:   "tridentversion",
				Kind:       "TridentVersion",
				ShortNames: []string{"tver", "tversion"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Version",
							Type:        "string",
							Description: "The Trident version",
							Priority:    int32(0),
							JSONPath:    ".trident_version",
						},
					},
				},
			},
		},
	}

	actualYAML := GetVersionCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetVolumeReferenceCRDYAML(t *testing.T) {
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				"spec": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"pvcName": {
							Type: "string",
						},
						"pvcNamespace": {
							Type: "string",
						},
					},
					Required: []string{"pvcName", "pvcNamespace"},
				},
			},
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentvolumereferences.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentvolumereferences",
				Singular:   "tridentvolumereference",
				Kind:       "TridentVolumeReference",
				ShortNames: []string{"tvr", "tvref"},
				Categories: []string{"trident", "trident-external", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:                     "v1",
					Served:                   true,
					Storage:                  true,
					Schema:                   &schema,
					AdditionalPrinterColumns: nil,
				},
			},
		},
	}
	actualYAML := GetVolumeReferenceCRDYAML()
	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetBackendCRDYAML(t *testing.T) {
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentbackends.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentbackends",
				Singular:   "tridentbackend",
				Kind:       "TridentBackend",
				ShortNames: []string{"tbe", "tbackend"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Backend",
							Type:        "string",
							Description: "The backend name",
							Priority:    int32(0),
							JSONPath:    ".backendName",
						},
						{
							Name:        "Backend UUID",
							Type:        "string",
							Description: "The backend UUID",
							Priority:    int32(0),
							JSONPath:    ".backendUUID",
						},
					},
				},
			},
		},
	}

	actualYAML := GetBackendCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetBackendConfigCRDYAML(t *testing.T) {
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentbackendconfigs.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentbackendconfigs",
				Singular:   "tridentbackendconfig",
				Kind:       "TridentBackendConfig",
				ShortNames: []string{"tbc", "tbconfig", "tbackendconfig"},
				Categories: []string{"trident", "trident-internal", "trident-external"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
						Scale:  nil,
					},
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Backend Name",
							Type:        "string",
							Description: "The backend name",
							Priority:    int32(0),
							JSONPath:    ".status.backendInfo.backendName",
						},
						{
							Name:        "Backend UUID",
							Type:        "string",
							Description: "The backend UUID",
							Priority:    int32(0),
							JSONPath:    ".status.backendInfo.backendUUID",
						},
						{
							Name:        "Phase",
							Type:        "string",
							Description: "The backend config phase",
							Priority:    int32(0),
							JSONPath:    ".status.phase",
						},
						{
							Name:        "Status",
							Type:        "string",
							Description: "The result of the last operation",
							Priority:    int32(0),
							JSONPath:    ".status.lastOperationStatus",
						},
						{
							Name:        "Storage Driver",
							Type:        "string",
							Description: "The storage driver type",
							Priority:    int32(1),
							JSONPath:    ".spec.storageDriverName",
						},
						{
							Name:        "Deletion Policy",
							Type:        "string",
							Description: "The deletion policy",
							Priority:    int32(1),
							JSONPath:    ".status.deletionPolicy",
						},
					},
				},
			},
		},
	}

	actualYAML := GetBackendConfigCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetMirrorRelationshipCRDYAML(t *testing.T) {
	minItems := int64(1)
	maxItems := int64(1)
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				"spec": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"state": {
							Type: "string",
							Enum: []apiextensionsv1.JSON{
								{
									Raw: []byte("\"\""),
								},
								{
									Raw: []byte("\"promoted\""),
								},
								{
									Raw: []byte("\"established\""),
								},
								{
									Raw: []byte("\"reestablished\""),
								},
							},
						},
						"replicationPolicy": {
							Type: "string",
						},
						"replicationSchedule": {
							Type: "string",
						},
						"volumeMappings": {
							Items: &apiextensionsv1.JSONSchemaPropsOrArray{
								Schema: &apiextensionsv1.JSONSchemaProps{
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"promotedSnapshotHandle": {
											Type: "string",
										},
										"localPVCName": {
											Type: "string",
										},
										"remoteVolumeHandle": {
											Type: "string",
										},
									},
									Required: []string{"localPVCName"},
								},
							},
							MinItems: &minItems,
							MaxItems: &maxItems,
							Type:     "array",
						},
					},
					Required: []string{"volumeMappings"},
				},
				"status": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"conditions": {
							Type: "array",
							Items: &apiextensionsv1.JSONSchemaPropsOrArray{
								Schema: &apiextensionsv1.JSONSchemaProps{
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"lastTransitionTime": {
											Type: "string",
										},
										"localPVCName": {
											Type: "string",
										},
										"localVolumeHandle": {
											Type: "string",
										},
										"remoteVolumeHandle": {
											Type: "string",
										},
										"message": {
											Type: "string",
										},
										"observedGeneration": {
											Type: "integer",
										},
										"state": {
											Type: "string",
										},
										"replicationPolicy": {
											Type: "string",
										},
										"replicationSchedule": {
											Type: "string",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentmirrorrelationships.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentmirrorrelationships",
				Singular:   "tridentmirrorrelationship",
				Kind:       "TridentMirrorRelationship",
				ShortNames: []string{"tmr", "tmrelationship", "tmirrorrelationship"},
				Categories: []string{"trident", "trident-internal", "trident-external"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
						Scale:  nil,
					},
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Desired State",
							Type:        "string",
							Description: "The desired mirror state",
							JSONPath:    ".spec.state",
						},
						{
							Name:        "Local PVC",
							Type:        "string",
							Description: "Local PVCs for the mirror",
							JSONPath:    ".spec.volumeMappings[*].localPVCName",
						},
						{
							Name:        "Actual state",
							Type:        "string",
							Description: "Status",
							JSONPath:    ".status.conditions[*].state",
						},
						{
							Name:        "Message",
							Type:        "string",
							Description: "Status message",
							JSONPath:    ".status.conditions[*].message",
						},
					},
				},
			},
		},
	}

	actualYAML := GetMirrorRelationshipCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.Equal(t, expected.Spec, actual.Spec)
}

func TestGetSnapshotInfoCRDYAML(t *testing.T) {
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				"spec": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"snapshotName": {
							Type: "string",
						},
					},
					Required: []string{"snapshotName"},
				},
				"status": {
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"lastTransitionTime": {
							Type: "string",
						},
						"observedGeneration": {
							Type: "integer",
						},
						"snapshotHandle": {
							Type: "string",
						},
					},
				},
			},
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentsnapshotinfos.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentsnapshotinfos",
				Singular:   "tridentsnapshotinfo",
				Kind:       "TridentSnapshotInfo",
				ShortNames: []string{"tsi", "tsinfo", "tsnapshotinfo"},
				Categories: []string{"trident", "trident-internal", "trident-external"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
						Scale:  nil,
					},
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Snapshot Handle",
							Type:        "string",
							Description: "VolumeSnapshotContent Handle",
							Priority:    int32(0),
							JSONPath:    ".status.snapshotHandle",
						},
					},
				},
			},
		},
	}

	actualYAML := GetSnapshotInfoCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetStorageClassCRDYAML(t *testing.T) {
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentstorageclasses.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentstorageclasses",
				Singular:   "tridentstorageclass",
				Kind:       "TridentStorageClass",
				ShortNames: []string{"tsc", "tstorageclass"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema,
				},
			},
		},
	}

	actualYAML := GetStorageClassCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetVolumeCRDYAML(t *testing.T) {
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentvolumes.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentvolumes",
				Singular:   "tridentvolume",
				Kind:       "TridentVolume",
				ShortNames: []string{"tvol", "tvolume"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:         "v1",
					Served:       true,
					Storage:      true,
					Schema:       &schema,
					Subresources: nil,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:     "Age",
							Type:     "date",
							Priority: int32(0),
							JSONPath: ".metadata.creationTimestamp",
						},
						{
							Name:        "Size",
							Type:        "string",
							Description: "The volume's size",
							Priority:    int32(1),
							JSONPath:    ".config.size",
						},
						{
							Name:        "Storage Class",
							Type:        "string",
							Description: "The volume's storage class",
							Priority:    int32(1),
							JSONPath:    ".config.storageClass",
						},
						{
							Name:        "State",
							Type:        "string",
							Description: "The volume's state",
							Priority:    int32(1),
							JSONPath:    ".state",
						},
						{
							Name:        "Protocol",
							Type:        "string",
							Description: "The volume's protocol",
							Priority:    int32(1),
							JSONPath:    ".config.protocol",
						},
						{
							Name:        "Backend UUID",
							Type:        "string",
							Description: "The volume's backend UUID",
							Priority:    int32(1),
							JSONPath:    ".backendUUID",
						},
						{
							Name:        "Pool",
							Type:        "string",
							Description: "The volume's pool",
							Priority:    int32(1),
							JSONPath:    ".pool",
						},
					},
				},
			},
		},
	}

	actualYAML := GetVolumeCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetVolumePublicationCRDYAML(t *testing.T) {
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				"volumeID": {
					Type: "string",
				},
				"nodeID": {
					Type: "string",
				},
				"readOnly": {
					Type: "boolean",
				},
				"accessMode": {
					Type:   "integer",
					Format: "int32",
				},
				"notSafeToAttach": {
					Type: "boolean",
				},
				"unpublished": {
					Type: "boolean",
				},
			},
			Required: []string{"volumeID", "nodeID", "readOnly"},
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentvolumepublications.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "tridentvolumepublications",
				Singular: "tridentvolumepublication",
				Kind:     "TridentVolumePublication",
				ShortNames: []string{
					"tvp",
					"tvpub",
					"tvpublication",
					"tvolpub",
					"tvolumepub",
					"tvolpublication",
					"tvolumepublication",
				},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "Volume",
							Type:        "string",
							Description: "Volume ID",
							Priority:    int32(0),
							JSONPath:    ".volumeID",
						},
						{
							Name:        "Node",
							Type:        "string",
							Description: "Node ID",
							Priority:    int32(0),
							JSONPath:    ".nodeID",
						},
					},
				},
			},
		},
	}

	actualYAML := GetVolumePublicationCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetNodeCRDYAML(t *testing.T) {
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentnodes.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentnodes",
				Singular:   "tridentnode",
				Kind:       "TridentNode",
				ShortNames: []string{"tnode"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema,
				},
			},
		},
	}

	actualYAML := GetNodeCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetTransactionCRDYAML(t *testing.T) {
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridenttransactions.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridenttransactions",
				Singular:   "tridenttransaction",
				Kind:       "TridentTransaction",
				ShortNames: []string{"ttx", "ttransaction"},
				Categories: []string{"trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema,
				},
			},
		},
	}

	actualYAML := GetTransactionCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetSnapshotCRDYAML(t *testing.T) {
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentsnapshots.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentsnapshots",
				Singular:   "tridentsnapshot",
				Kind:       "TridentSnapshot",
				ShortNames: []string{"tss", "tsnap", "tsnapshot"},
				Categories: []string{"trident", "trident-internal"},
			},
			Scope: "Namespaced",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema,
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
						{
							Name:        "State",
							Type:        "string",
							Description: "The snapshot's state",
							Priority:    int32(1),
							JSONPath:    ".state",
						},
					},
				},
			},
		},
	}

	actualYAML := GetSnapshotCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetOrchestratorCRDYAML(t *testing.T) {
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}
	expected := apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentorchestrators.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentorchestrators",
				Singular:   "tridentorchestrator",
				Kind:       "TridentOrchestrator",
				ListKind:   "TridentOrchestratorList",
				ShortNames: []string{"torc", "torchestrator"},
			},
			Scope: "Cluster",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema:  &schema,
					Subresources: &apiextensionsv1.CustomResourceSubresources{
						Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
						Scale:  nil,
					},
				},
			},
		},
	}

	actualYAML := GetOrchestratorCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetCSIDriverYAML(t *testing.T) {
	name := "csi.trident.netapp.io"
	required := true
	expected := csiv1.CSIDriver{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CSIDriver",
			APIVersion: "storage.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "csi.trident.netapp.io",
		},
		Spec: csiv1.CSIDriverSpec{
			AttachRequired: &required,
		},
	}

	var actual csiv1.CSIDriver
	actualYAML := GetCSIDriverYAML(name, nil, nil)

	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.Equal(t, "csi.trident.netapp.io", actual.Name)
}

func TestGetPrivilegedPodSecurityPolicyYAML(t *testing.T) {
	name := "tridentpods"
	allow := true

	expected := pspv1beta1.PodSecurityPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodSecurityPolicy",
			APIVersion: "policy/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentpods",
		},
		Spec: pspv1beta1.PodSecurityPolicySpec{
			Privileged:               true,
			AllowPrivilegeEscalation: &allow,
			HostIPC:                  true,
			HostPID:                  true,
			HostNetwork:              true,
			SELinux: pspv1beta1.SELinuxStrategyOptions{
				Rule: "RunAsAny",
			},
			SupplementalGroups: pspv1beta1.SupplementalGroupsStrategyOptions{
				Rule: "RunAsAny",
			},
			RunAsUser: pspv1beta1.RunAsUserStrategyOptions{
				Rule: "RunAsAny",
			},
			FSGroup: pspv1beta1.FSGroupStrategyOptions{
				Rule: "RunAsAny",
			},
			Volumes: []pspv1beta1.FSType{"hostPath", " projected", "emptyDir"},
		},
	}

	var actual pspv1beta1.PodSecurityPolicy
	actualYAML := GetPrivilegedPodSecurityPolicyYAML(name, nil, nil)

	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.Equal(t, "tridentpods", actual.Name)
}

func TestGetUnprivilegedPodSecurityPolicyYAML(t *testing.T) {
	name := "psp.flannel.unprivileged"

	expected := pspv1beta1.PodSecurityPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PodSecurityPolicy",
			APIVersion: "policy/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "psp.flannel.unprivileged",
		},
		Spec: pspv1beta1.PodSecurityPolicySpec{
			Privileged: false,
			SELinux: pspv1beta1.SELinuxStrategyOptions{
				Rule: "RunAsAny",
			},
			SupplementalGroups: pspv1beta1.SupplementalGroupsStrategyOptions{
				Rule: "RunAsAny",
			},
			RunAsUser: pspv1beta1.RunAsUserStrategyOptions{
				Rule: "RunAsAny",
			},
			FSGroup: pspv1beta1.FSGroupStrategyOptions{
				Rule: "RunAsAny",
			},
			Volumes: []pspv1beta1.FSType{"hostPath", " projected", "emptyDir"},
		},
	}

	var actual pspv1beta1.PodSecurityPolicy
	actualYAML := GetUnprivilegedPodSecurityPolicyYAML(name, nil, nil)

	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.Equal(t, "psp.flannel.unprivileged", actual.Name)
}

func TestGetResourceQuotaYAML(t *testing.T) {
	// setup labels and controlling cr details for the test cases
	appLabels := map[string]string{
		TridentAppKey: TridentAppValue,
	}

	controllingCRDetails := map[string]string{
		CRAPIVersionKey: "v1",
		CRController:    "true",
		CRKind:          "TridentOrchestrator",
		CRName:          "trident",
		CRUID:           "1ec3fba0-ed52-43a4-a7e3-3e9b227f0ead",
	}

	// create the labels and controllingCRDetails strings as they would exist in the YAML
	appLabelString := constructLabels(appLabels)
	controllingCRString := constructOwnerRef(controllingCRDetails)

	// get the YAML
	resourceQuotaYAML := GetResourceQuotaYAML(Name, Namespace, appLabels, controllingCRDetails)

	// assert the appropriate substrings exist
	assert.Equal(t, strings.Contains(resourceQuotaYAML, Name), true)
	assert.Equal(t, strings.Contains(resourceQuotaYAML, Namespace), true)
	assert.Equal(t, strings.ContainsAny(resourceQuotaYAML, appLabelString), true)
	assert.Equal(t, strings.ContainsAny(resourceQuotaYAML, controllingCRString), true)

	// setup different labels and controllingCRDetails without "trident" as "trident" is used for Name and Namespace
	appLabels = map[string]string{
		"key": "value",
	}

	controllingCRDetails = map[string]string{
		CRAPIVersionKey: "v1",
		CRController:    "true",
		CRKind:          "torc",
		CRName:          "torc",
		CRUID:           "1ec3fba0-ed52-43a4-a7e3-3e9b227f0ead",
	}

	// create the labels and controllingCRDetails strings as they would exist in the YAML
	appLabelString = constructLabels(appLabels)
	controllingCRString = constructOwnerRef(controllingCRDetails)

	// get the YAML
	resourceQuotaYAML = GetResourceQuotaYAML("any", "default", appLabels, controllingCRDetails)

	// assert ONLY the appropriate substrings exist
	assert.Equal(t, strings.Contains(resourceQuotaYAML, Name), false)
	assert.Equal(t, strings.Contains(resourceQuotaYAML, Namespace), false)
	assert.Equal(t, strings.ContainsAny(resourceQuotaYAML, appLabelString), true)
	assert.Equal(t, strings.ContainsAny(resourceQuotaYAML, controllingCRString), true)
}
