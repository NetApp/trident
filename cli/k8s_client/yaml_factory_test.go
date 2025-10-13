// Copyright 2025 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
	scc "github.com/openshift/api/security/v1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	csiv1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/config"
	commonconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/pkg/convert"
	versionutils "github.com/netapp/trident/utils/version"
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

	CRAPIVersionKey = "apiVersion"
	CRController    = "controller"
	CRKind          = "kind"
	CRName          = "name"
	CRUID           = "uid"
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

	version := versionutils.MustParseSemantic("1.21.0")
	cloudIdentity := AzureCloudIdentityKey + " a8rry78r8-7733-49bd-6656582"

	deploymentArgs := &DeploymentYAMLArguments{
		DeploymentName:          Name,
		TridentImage:            ImageName,
		AutosupportImage:        AutosupportImageName,
		AutosupportProxy:        "",
		AutosupportCustomURL:    "",
		AutosupportSerialNumber: "",
		AutosupportHostname:     "",
		LogFormat:               LogFormat,
		LogLevel:                "info",
		ImageRegistry:           "",
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    ownerRef,
		UseIPv6:                 false,
		SilenceAutosupport:      false,
		Version:                 version,
		HTTPRequestTimeout:      config.HTTPTimeoutString,
		EnableACP:               true,
		HTTPSMetrics:            true,
		IdentityLabel:           true,
		K8sAPIQPS:               100,
	}

	yamlsOutputs := []string{
		GetServiceAccountYAML(Name, nil, nil, nil, ""),
		GetServiceAccountYAML(Name, Secrets, labels, ownerRef, cloudIdentity),
		GetRoleYAML(Namespace, Name, labels, ownerRef),
		GetRoleBindingYAML(Namespace, Name, labels, ownerRef),
		GetClusterRoleYAML(Name, nil, nil),
		GetClusterRoleYAML(Name, labels, ownerRef),
		GetClusterRoleBindingYAML(Namespace, Name, FlavorOpenshift, nil, ownerRef),
		GetClusterRoleBindingYAML(Namespace, Name, FlavorK8s, labels, ownerRef),
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
	controllerRBACLabels := map[string]string{"app": "controller"}

	yamlsOutputs := map[string]string{
		GetClusterRoleYAML(Name, nil, nil):                                    "rbac.authorization.k8s.io/v1",
		GetRoleYAML(Namespace, Name, nil, nil):                                "rbac.authorization.k8s.io/v1",
		GetRoleYAML(Namespace, Name, controllerRBACLabels, nil):               "rbac.authorization.k8s.io/v1",
		GetRoleBindingYAML(Namespace, Name, nil, nil):                         "rbac.authorization.k8s.io/v1",
		GetClusterRoleBindingYAML(Namespace, Name, FlavorK8s, nil, nil):       "rbac.authorization.k8s.io/v1",
		GetClusterRoleBindingYAML(Namespace, Name, FlavorK8s, nil, nil):       "rbac.authorization.k8s.io/v1",
		GetClusterRoleBindingYAML(Namespace, Name, FlavorOpenshift, nil, nil): "rbac.authorization.k8s.io/v1",
	}

	for result, value := range yamlsOutputs {
		assert.Contains(t, result, value, fmt.Sprintf("Incorrect API Version returned %s", value))
	}
}

func TestGetCSIDeploymentAutosupportYAML(t *testing.T) {
	t.Run("when ExcludeAutosupport is true", func(t *testing.T) {
		t.Parallel()
		args := &DeploymentYAMLArguments{
			ExcludeAutosupport: true,
		}
		assert.Empty(t, getCSIDeploymentAutosupportYAML(args), "expected empty volume YAML when ExcludeAutosupport is true")
		assert.Empty(t, getCSIDeploymentAutosupportVolumeYAML(args), "expected empty volume YAML when ExcludeAutosupport is true")
	})

	t.Run("when ExcludeAutosupport is false", func(t *testing.T) {
		args := &DeploymentYAMLArguments{
			ExcludeAutosupport: false,
		}
		result := getCSIDeploymentAutosupportYAML(args)
		assert.Contains(t, result, config.DefaultAutosupportName, "expected autosupport container name when ExcludeAutosupport is false")
		assert.Contains(t, result, config.DefaultAutosupportImage, "expected autosupport image when ExcludeAutosupport is false")
		assert.Contains(t, result, "- -debug", "expected autosupport debug flag to be uncommented when ExcludeAutosupport is false")
		assert.NotEmpty(t, getCSIDeploymentAutosupportVolumeYAML(args), "expected non-empty volume YAML when ExcludeAutosupport is false")
	})
}

// Simple validation of the CSI Deployment YAML
func TestGetCSIDeploymentYAML_WithExcludeAutosupport(t *testing.T) {
	labels := make(map[string]string)
	labels["app"] = "trident"
	ownerRef := make(map[string]string)
	ownerRef["uid"] = "123456789"
	ownerRef["kind"] = "TridentProvisioner"
	imagePullSecrets := []string{"thisisasecret"}
	version := versionutils.MustParseSemantic("1.26.0")

	excludeASUPDeploymentArgs := &DeploymentYAMLArguments{
		DeploymentName:          "trident-csi",
		TridentImage:            "netapp/trident:20.10.0-custom",
		AutosupportImage:        "netapp/trident-autosupport:20.10.0-custom",
		AutosupportSerialNumber: "0000-0000",
		AutosupportHostname:     "21e160d3-721f-4ec4-bcd4-c5e0d31d1a6e",
		AutosupportProxy:        "http://127.0.0.1/",
		AutosupportCustomURL:    "http://172.16.150.125:8888/",
		ImageRegistry:           "registry.k8s.io",
		LogFormat:               "text",
		LogLevel:                "",
		Debug:                   true,
		AutosupportInsecure:     true,
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    map[string]string{},
		Version:                 version,
		HTTPRequestTimeout:      config.HTTPTimeoutString,
		UseIPv6:                 true,
		SilenceAutosupport:      false,
		ExcludeAutosupport:      true,
		EnableACP:               true,
		HTTPSMetrics:            true,
		K8sAPIQPS:               100,
	}
	installASUPDeploymentArgs := &DeploymentYAMLArguments{
		DeploymentName:          "trident-csi",
		TridentImage:            "netapp/trident:20.10.0-custom",
		AutosupportImage:        "netapp/trident-autosupport:20.10.0-custom",
		AutosupportSerialNumber: "0000-0000",
		AutosupportHostname:     "21e160d3-721f-4ec4-bcd4-c5e0d31d1a6e",
		AutosupportProxy:        "http://127.0.0.1/",
		AutosupportCustomURL:    "http://172.16.150.125:8888/",
		ImageRegistry:           "registry.k8s.io",
		LogFormat:               "text",
		LogLevel:                "",
		Debug:                   true,
		AutosupportInsecure:     true,
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    map[string]string{},
		Version:                 version,
		HTTPRequestTimeout:      config.HTTPTimeoutString,
		UseIPv6:                 true,
		SilenceAutosupport:      false,
		ExcludeAutosupport:      false,
		EnableACP:               true,
		HTTPSMetrics:            true,
		K8sAPIQPS:               100,
	}

	deploymentYAML := GetCSIDeploymentYAML(excludeASUPDeploymentArgs)
	assert.NotContains(t, deploymentYAML, getCSIDeploymentAutosupportYAML(installASUPDeploymentArgs))
	assert.NotContains(t, deploymentYAML, getCSIDeploymentAutosupportYAML(installASUPDeploymentArgs))
	assert.NotContains(t, deploymentYAML, getCSIDeploymentAutosupportVolumeYAML(installASUPDeploymentArgs))
}

// Simple validation of the CSI Deployment YAML
func TestValidateGetCSIDeploymentYAMLSuccess(t *testing.T) {
	labels := make(map[string]string)
	labels["app"] = "trident"

	ownerRef := make(map[string]string)
	ownerRef["uid"] = "123456789"
	ownerRef["kind"] = "TridentProvisioner"

	imagePullSecrets := []string{"thisisasecret"}

	version := versionutils.MustParseSemantic("1.26.0")

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
		LogLevel:                "",
		Debug:                   true,
		AutosupportInsecure:     true,
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    map[string]string{},
		Version:                 version,
		HTTPRequestTimeout:      config.HTTPTimeoutString,
		UseIPv6:                 true,
		SilenceAutosupport:      false,
		ExcludeAutosupport:      false,
		EnableACP:               true,
		HTTPSMetrics:            true,
		K8sAPIQPS:               100,
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

	version := versionutils.MustParseSemantic("1.26.0")

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
		LogLevel:                "debug",
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    map[string]string{},
		UseIPv6:                 true,
		SilenceAutosupport:      false,
		Version:                 version,
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
		version := versionutils.MustParseSemantic(versionString)
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
		NodeSelector: map[string]string{"node-label-key": "test1"},
	}

	expectedNodeSelectorString := `
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/arch
                    operator: In
                    values:
                    - arm64
                    - amd64
                  - key: kubernetes.io/os
                    operator: In
                    values:
                    - linux
                  - key: node-label-key
                    operator: In
                    values:
                    - 'test1'
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
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/arch
                    operator: In
                    values:
                    - arm64
                    - amd64
                  - key: kubernetes.io/os
                    operator: In
                    values:
                    - linux
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
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/arch
                    operator: In
                    values:
                    - arm64
                    - amd64
                  - key: kubernetes.io/os
                    operator: In
                    values:
                    - linux
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
	versions := []string{"1.26.0"}
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
			version := versionutils.MustParseSemantic(versionString)
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

func TestGetCSIDeploymentYAMLAzure(t *testing.T) {
	args := &DeploymentYAMLArguments{
		CloudProvider: CloudProviderAzure,
	}

	yamlData := GetCSIDeploymentYAML(args)
	deployment := appsv1.Deployment{}
	err := yaml.Unmarshal([]byte(yamlData), &deployment)
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}

	var envExist, volumeExist, volumeMountExist bool
	for _, env := range deployment.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "AZURE_CREDENTIAL_FILE" {
			envExist = true
			break
		}
	}
	for _, volumeMount := range deployment.Spec.Template.Spec.Containers[0].VolumeMounts {
		if volumeMount.Name == "azure-cred" {
			volumeMountExist = true
			break
		}
	}
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == "azure-cred" {
			volumeExist = true
			break
		}
	}
	assert.True(t, envExist && volumeExist && volumeMountExist, "expected env var AZURE_CREDENTIAL_FILE to exist")
}

func TestGetCSIDeploymentYAML_AutosupportYAML_ExcludeAutosupport(t *testing.T) {
	labels := make(map[string]string)
	labels["app"] = "trident"

	ownerRef := make(map[string]string)
	ownerRef["uid"] = "123456789"
	ownerRef["kind"] = "TridentProvisioner"

	imagePullSecrets := []string{"thisisasecret"}

	version := versionutils.MustParseSemantic("1.26.0")
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
		LogLevel:                "",
		Debug:                   true,
		AutosupportInsecure:     true,
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    map[string]string{},
		Version:                 version,
		HTTPRequestTimeout:      config.HTTPTimeoutString,
		UseIPv6:                 true,
		SilenceAutosupport:      false,
		ExcludeAutosupport:      true,
		EnableACP:               true,
		HTTPSMetrics:            true,
		K8sAPIQPS:               100,
	}

	deploymentYAML := GetCSIDeploymentYAML(deploymentArgs)
	_, err := yaml.YAMLToJSON([]byte(deploymentYAML))
	assert.NoError(t, err)

	// Check that the autosupport container is not present
	assert.NotContains(t, deploymentYAML, config.DefaultAutosupportName)
	assert.NotContains(t, deploymentYAML, config.DefaultAutosupportImage)
}

func TestGetCSIDeploymentYAML_AutosupportYAML_EnableButSilenceAutosupport(t *testing.T) {
	labels := make(map[string]string)
	labels["app"] = "trident"

	ownerRef := make(map[string]string)
	ownerRef["uid"] = "123456789"
	ownerRef["kind"] = "TridentProvisioner"

	imagePullSecrets := []string{"thisisasecret"}
	asupImage := "netapp/trident-autosupport:20.10.0-custom"
	silenceASUP := true

	version := versionutils.MustParseSemantic("1.26.0")
	deploymentArgs := &DeploymentYAMLArguments{
		DeploymentName:          "trident-csi",
		TridentImage:            "netapp/trident:20.10.0-custom",
		AutosupportImage:        asupImage,
		AutosupportSerialNumber: "0000-0000",
		AutosupportHostname:     "21e160d3-721f-4ec4-bcd4-c5e0d31d1a6e",
		AutosupportProxy:        "http://127.0.0.1/",
		AutosupportCustomURL:    "http://172.16.150.125:8888/",
		ImageRegistry:           "registry.k8s.io",
		LogFormat:               "text",
		LogLevel:                "",
		Debug:                   true,
		AutosupportInsecure:     true,
		ImagePullSecrets:        imagePullSecrets,
		Labels:                  labels,
		ControllingCRDetails:    map[string]string{},
		Version:                 version,
		HTTPRequestTimeout:      config.HTTPTimeoutString,
		UseIPv6:                 true,
		SilenceAutosupport:      silenceASUP,
		ExcludeAutosupport:      false,
		EnableACP:               true,
		HTTPSMetrics:            true,
		K8sAPIQPS:               100,
	}

	deploymentYAML := GetCSIDeploymentYAML(deploymentArgs)
	bytes, err := yaml.YAMLToJSON([]byte(deploymentYAML))
	assert.NoError(t, err)

	// Check that the autosupport container is present
	deployment := appsv1.Deployment{}
	assert.NoError(t, yaml.Unmarshal(bytes, &deployment))
	assert.Contains(t, deploymentYAML, config.DefaultAutosupportName)
	assert.Contains(t, deploymentYAML, asupImage)
	assert.Contains(t, deploymentYAML, fmt.Sprintf("--trident-silence-collector=%v", silenceASUP))
}

func TestGetCSIDeploymentYAML_K8sAPIQPS(t *testing.T) {
	const k8sAPIQPS = 100
	args := &DeploymentYAMLArguments{
		K8sAPIQPS: k8sAPIQPS,
	}
	yamlData := GetCSIDeploymentYAML(args)
	deployment := appsv1.Deployment{}
	err := yaml.Unmarshal([]byte(yamlData), &deployment)
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}

	containerHasArgument := func(container v1.Container, expectedArg string) bool {
		for _, arg := range container.Args {
			if arg == expectedArg {
				return true
			}
		}
		return false
	}

	// trident-main container flag validation
	tridentMainQPSFlag := fmt.Sprintf("--k8s_api_qps=%d", k8sAPIQPS)
	tridentMainBurstFlag := fmt.Sprintf("--k8s_api_burst=%d", getBurstValueForQPS(k8sAPIQPS))
	tridentMainContainer := deployment.Spec.Template.Spec.Containers[0]
	assert.True(t, containerHasArgument(tridentMainContainer, tridentMainQPSFlag),
		"expected trident-main to have k8s_api_qps flag")
	assert.True(t, containerHasArgument(tridentMainContainer, tridentMainBurstFlag),
		"expected trident-main to have k8s_api_burst flag")

	sidecarQPSFlag := fmt.Sprintf("--kube-api-qps=%d", k8sAPIQPS)
	sidecarBurstFlag := fmt.Sprintf("--kube-api-burst=%d", getBurstValueForQPS(k8sAPIQPS))
	// trident-main and trident-autosupport containers will not have sidecar flags
	for _, container := range deployment.Spec.Template.Spec.Containers[2:] {
		assert.True(t, containerHasArgument(container, sidecarQPSFlag),
			"expected sidecar %v to have kube-api-qps flag", container.Name)
		assert.True(t, containerHasArgument(container, sidecarBurstFlag),
			"expected sidecar %v to have kube-api-burst flag", container.Name)
	}
}

func TestGetCSIDeploymentYAML_HostNetwork(t *testing.T) {
	deploymentArgs := &DeploymentYAMLArguments{
		HostNetwork: true,
	}

	deploymentYAML := GetCSIDeploymentYAML(deploymentArgs)
	deployment := appsv1.Deployment{}
	err := yaml.Unmarshal([]byte(deploymentYAML), &deployment)
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", deploymentYAML)
	}

	// trident controller flag validation
	tridentMainHostNetworkFlag := "hostNetwork: true"
	assert.Contains(t, deploymentYAML, tridentMainHostNetworkFlag)
}

func TestGetCSIDaemonSetYAMLLinux(t *testing.T) {
	versions := []string{"1.21.0", "1.23.0", "1.25.0"}

	for _, versionString := range versions {
		version := versionutils.MustParseSemantic(versionString)
		daemonsetArgs := &DaemonsetYAMLArguments{Version: version, LogLevel: "", Debug: true}

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
		version := versionutils.MustParseSemantic(versionString)
		daemonsetArgs := &DaemonsetYAMLArguments{Version: version, LogLevel: "debug"}

		yamlData := GetCSIDaemonSetYAMLLinux(daemonsetArgs)
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected valid YAML for version %s", versionString)
		}
	}
}

func TestGetCSIDaemonSetYAMLLinux_ForceDetach(t *testing.T) {
	versions := []string{"1.26.0"}
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
			version := versionutils.MustParseSemantic(versionString)
			daemonsetArgs := &DaemonsetYAMLArguments{Version: version, EnableForceDetach: args.Enabled}

			yamlData := GetCSIDaemonSetYAMLLinux(daemonsetArgs)
			_, err := yaml.YAMLToJSON([]byte(yamlData))
			if err != nil {
				t.Fatalf("expected valid YAML for version %s", versionString)
			}
			failMsg := fmt.Sprintf("expected enableForceDetach to be %s in final YAML: %s", args.FailStr, yamlData)
			assert.Contains(t, yamlData, args.Expected, failMsg)
			failMsg2 := fmt.Sprintf("did not expect enableForceDetach to be %s in final YAML: %s", args.FailStr2,
				yamlData)
			assert.NotContains(t, yamlData, args.Unexpected, failMsg2)
		}
	}
}

func TestGetCSIDaemonSetYAMLLinuxImagePullPolicy(t *testing.T) {
	versions := []string{"1.26.0"}
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
			version := versionutils.MustParseSemantic(versionString)
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

func TestGetCSIDaemonSetYAMLLinux_NodePrep(t *testing.T) {
	defaultVersions := []string{"1.26.0"}
	expected := "initContainers:"
	expectedParam := "--node-prep=%s"

	tests := []struct {
		Name           string
		Versions       []string
		NodePrep       []string
		ComparisonFunc assert.ComparisonAssertionFunc
		ExpectedParam  string
	}{
		{
			Name:           "nodePrep nil",
			Versions:       defaultVersions,
			NodePrep:       nil,
			ComparisonFunc: assert.NotContains,
		},
		{
			Name:           "nodePrep empty",
			Versions:       defaultVersions,
			NodePrep:       []string{},
			ComparisonFunc: assert.NotContains,
		},
		{
			Name:           "nodePrep list",
			Versions:       defaultVersions,
			NodePrep:       []string{"iSCSI", "NVME"},
			ComparisonFunc: assert.Contains,
			ExpectedParam:  fmt.Sprintf(expectedParam, "iSCSI,NVME"),
		},
		{
			Name:           "nodePrep single",
			Versions:       defaultVersions,
			NodePrep:       []string{"iSCSI"},
			ComparisonFunc: assert.Contains,
			ExpectedParam:  fmt.Sprintf(expectedParam, "iSCSI"),
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			for _, versionString := range test.Versions {
				version := versionutils.MustParseSemantic(versionString)
				daemonsetArgs := &DaemonsetYAMLArguments{Version: version, NodePrep: test.NodePrep}
				yamlData := GetCSIDaemonSetYAMLLinux(daemonsetArgs)
				_, err := yaml.YAMLToJSON([]byte(yamlData))
				assert.Nilf(t, err, "expected valid YAML for version %s", versionString)
				test.ComparisonFunc(t, yamlData, expected)
				assert.Contains(t, yamlData, test.ExpectedParam)
				// this tag should never be left in the yaml
				assert.NotContains(t, yamlData, "{PREPARE_NODE}")
			}
		})
	}
}

func TestGetCSIDaemonSetYAMLLinux_NodeSelectors(t *testing.T) {
	daemonsetArgs := &DaemonsetYAMLArguments{
		NodeSelector: map[string]string{"node-label-key": "test1"},
	}

	expectedNodeSelectorString := `
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/arch
                    operator: In
                    values:
                    - arm64
                    - amd64
                  - key: kubernetes.io/os
                    operator: In
                    values:
                    - linux
                  - key: node-label-key
                    operator: In
                    values:
                    - 'test1'
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
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/arch
                    operator: In
                    values:
                    - arm64
                    - amd64
                  - key: kubernetes.io/os
                    operator: In
                    values:
                    - linux
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
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
              - matchExpressions:
                  - key: kubernetes.io/arch
                    operator: In
                    values:
                    - arm64
                    - amd64
                  - key: kubernetes.io/os
                    operator: In
                    values:
                    - linux
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
		version := versionutils.MustParseSemantic(versionString)
		daemonsetArgs := &DaemonsetYAMLArguments{Version: version, LogLevel: "debug"}

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
		version := versionutils.MustParseSemantic(versionString)
		daemonsetArgs := &DaemonsetYAMLArguments{Version: version, LogLevel: "info"}

		yamlData := GetCSIDaemonSetYAMLWindows(daemonsetArgs)
		_, err := yaml.YAMLToJSON([]byte(yamlData))
		if err != nil {
			t.Fatalf("expected valid YAML for version %s", versionString)
		}
	}
}

func TestGetCSIDaemonSetYAMLWindowsImagePullPolicy(t *testing.T) {
	versions := []string{"1.26.0"}
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
			version := versionutils.MustParseSemantic(versionString)
			daemonsetArgs := &DaemonsetYAMLArguments{Version: version, ImagePullPolicy: args.imagePullPolicy, LogLevel: "", Debug: true}

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

func TestGetCSIDaemonSetYAMLWindows_Tolerations(t *testing.T) {
	daemonsetArgs := &DaemonsetYAMLArguments{
		Version: versionutils.MustParseSemantic("1.26.0"),
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

	yamlData := GetCSIDaemonSetYAMLWindows(daemonsetArgs)
	_, err := yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedTolerationString,
		fmt.Sprintf("expected toleration in final YAML: %s", yamlData))

	// Test default
	daemonsetArgs = &DaemonsetYAMLArguments{
		Version: versionutils.MustParseSemantic("1.26.0"),
	}
	expectedTolerationString = `
      tolerations:
      - effect: "NoExecute"
        operator: "Exists"
      - effect: "NoSchedule"
        operator: "Exists"
`

	yamlData = GetCSIDaemonSetYAMLWindows(daemonsetArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedTolerationString,
		fmt.Sprintf("expected toleration in final YAML: %s", yamlData))

	// Test empty tolerations specified
	daemonsetArgs = &DaemonsetYAMLArguments{
		Version:     versionutils.MustParseSemantic("1.26.0"),
		Tolerations: []map[string]string{},
	}
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

	yamlData = GetCSIDaemonSetYAMLWindows(daemonsetArgs)
	_, err = yaml.YAMLToJSON([]byte(yamlData))
	if err != nil {
		t.Fatalf("expected valid YAML, got %s", yamlData)
	}
	assert.Contains(t, yamlData, expectedTolerationString,
		fmt.Sprintf("expected toleration in final YAML: %s", yamlData))
	assert.NotContains(t, yamlData, defaultTolerationString,
		fmt.Sprintf("expected default tolerations to not appear in final YAML: %s", yamlData))
}

func TestConstructNodeSelector(t *testing.T) {
	nodeSelMap := map[string]string{
		"node-label-name":     "master",
		"node-label-number":   "20",
		"node-label-bool":     "true",
		"node-label-alphanum": "alph20",
	}

	expectedNodeSelStrings := []string{
		"- key: node-label-name\n  operator: In\n  values:\n  - 'master'\n",
		"- key: node-label-number\n  operator: In\n  values:\n  - '20'\n",
		"- key: node-label-bool\n  operator: In\n  values:\n  - 'true'\n",
		"- key: node-label-alphanum\n  operator: In\n  values:\n  - 'alph20'\n",
	}

	result := constructNodeSelector(nodeSelMap)
	// verify all the expected strings are present in the result
	for _, expectedString := range expectedNodeSelStrings {
		assert.Contains(t, result, expectedString)
		before, after, _ := strings.Cut(result, expectedString)
		result = before + after
	}

	// verify that the result does not contain anything else other than what is expected.
	assert.Empty(t, result)
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

func getTestTridentVersionPodYAML(toleration []v1.Toleration) v1.Pod {
	return v1.Pod{
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
					SecurityContext: &v1.SecurityContext{
						Capabilities: &v1.Capabilities{
							Drop: []v1.Capability{
								"all",
							},
						},
					},
				},
			},
			ImagePullSecrets: []v1.LocalObjectReference{
				{
					Name: "trident-csi-image-secret",
				},
			},
			Affinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "kubernetes.io/arch",
										Operator: v1.NodeSelectorOperator("In"),
										Values:   []string{"arm64", "amd64"},
									},
									{
										Key:      "kubernetes.io/os",
										Operator: v1.NodeSelectorOperator("In"),
										Values:   []string{"linux"},
									},
								},
							},
						},
					},
				},
			},
			Tolerations: toleration,
		},
	}
}

func TestGetTridentVersionPodYAML(t *testing.T) {
	name := "trident-csi"
	image := "trident-csi-image"
	secrets := []string{"trident-csi-image-secret"}
	labels := map[string]string{"app": "controller.csi.trident.netapp.io"}
	crdDetails := map[string]string{"kind": "ReplicaSet"}

	type Args struct {
		toleration []map[string]string
		Expected   []v1.Toleration
	}

	testArgs := []*Args{
		{
			nil, // Default toleration
			[]v1.Toleration{},
		},
		{
			[]map[string]string{
				{"effect": "NoExecute", "operator": "Exists"},
			},
			[]v1.Toleration{
				{Effect: "NoExecute", Operator: "Exists"},
			},
		},
	}
	for _, args := range testArgs {
		expected := getTestTridentVersionPodYAML(args.Expected)

		var actual v1.Pod
		versionPodArgs := &TridentVersionPodYAMLArguments{
			TridentVersionPodName: name,
			TridentImage:          image,
			Labels:                labels,
			ControllingCRDetails:  crdDetails,
			ImagePullSecrets:      secrets,
			ImagePullPolicy:       "IfNotPresent",
			ServiceAccountName:    "service",
			Tolerations:           args.toleration,
		}
		actualYAML := GetTridentVersionPodYAML(versionPodArgs)
		assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
		assert.Equal(t, expected, actual)
	}
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
		AllowedCapabilities: []v1.Capability{
			"SYS_ADMIN",
		},
		DefaultAddCapabilities: nil,
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

	actualYAML := GetOpenShiftSCCYAML(sccName, user, namespace, labels, crdDetails, true, false)
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.Equal(t, expected, actual)
}

func TestGetOpenShiftSCCYAML_UnprivilegedUser(t *testing.T) {
	sccName := "trident-controller"
	user := "trident-controller"
	namespace := "trident"
	labels := map[string]string{"app": "controller.trident.netapp.io"}
	crdDetails := map[string]string{"kind": "ReplicaSet"}

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
		AllowHostDirVolumePlugin: true,
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
		Priority:                 nil,
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

	actualYAML := GetOpenShiftSCCYAML(sccName, user, namespace, labels, crdDetails, false, false)
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
	crdList := strings.Split(GetCRDsYAML(), "---")
	namesToDefinitions := make(map[string]string, len(crdList))
	for _, crdYAML := range crdList {
		var crd apiextensionsv1.CustomResourceDefinition
		assert.Nil(t, yaml.Unmarshal([]byte(crdYAML), &crd), "invalid YAML")
		namesToDefinitions[crd.Name] = crdYAML
	}

	expectedAPIs := map[string]apiextensionsv1.CustomResourceDefinition{
		"tridentversions.trident.netapp.io":               getVersionAPIExtension(t),
		"tridentbackends.trident.netapp.io":               getBackendAPIExtension(t),
		"tridentbackendconfigs.trident.netapp.io":         getBackendConfigAPIExtension(t),
		"tridentmirrorrelationships.trident.netapp.io":    getMirrorRelationshipAPIExtension(t),
		"tridentactionmirrorupdates.trident.netapp.io":    getActionMirrorUpdateAPIExtension(t),
		"tridentsnapshotinfos.trident.netapp.io":          getSnapshotInfoAPIExtension(t),
		"tridentstorageclasses.trident.netapp.io":         getStorageClassAPIExtension(t),
		"tridentvolumes.trident.netapp.io":                getVolumeAPIExtension(t),
		"tridentvolumepublications.trident.netapp.io":     getVolumePublicationAPIExtension(t),
		"tridentnodes.trident.netapp.io":                  getNodeAPIExtension(t),
		"tridenttransactions.trident.netapp.io":           getTransactionAPIExtension(t),
		"tridentsnapshots.trident.netapp.io":              getSnapshotAPIExtension(t),
		"tridentgroupsnapshots.trident.netapp.io":         getGroupSnapshotAPIExtension(t),
		"tridentvolumereferences.trident.netapp.io":       getVolumeReferenceAPIExtension(t),
		"tridentactionsnapshotrestores.trident.netapp.io": getActionSnapshotRestoreAPIExtension(t),
		"tridentconfigurators.trident.netapp.io":          getConfiguratorsAPIExtension(t),
	}

	// Verify that all expected CRD names are present
	for name, expected := range expectedAPIs {
		t.Run(name, func(t *testing.T) {
			// Ensure the expected CRD is the same as marshaling and unmarshaling the YAML.
			var actual apiextensionsv1.CustomResourceDefinition
			crdYAML := namesToDefinitions[name]
			assert.Nil(t, yaml.Unmarshal([]byte(crdYAML), &actual), "invalid YAML")

			assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
			assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
			assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
		})
	}
}

func TestGetVersionCRDYAML(t *testing.T) {
	expected := getVersionAPIExtension(t)
	actualYAML := GetVersionCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetVolumeReferenceCRDYAML(t *testing.T) {
	expected := getVolumeReferenceAPIExtension(t)
	actualYAML := GetVolumeReferenceCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetBackendCRDYAML(t *testing.T) {
	expected := getBackendAPIExtension(t)
	actualYAML := GetBackendCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func getVersionAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
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
}

func getBackendAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
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
}

func getBackendConfigAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
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
}

func getMirrorRelationshipAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
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

	return apiextensionsv1.CustomResourceDefinition{
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
}

func getActionMirrorUpdateAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentactionmirrorupdates.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentactionmirrorupdates",
				Singular:   "tridentactionmirrorupdate",
				Kind:       "TridentActionMirrorUpdate",
				ShortNames: []string{"tamu", "tamupdate", "tamirrorupdate"},
				Categories: []string{"trident", "trident-external"},
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
							Name:        "Namespace",
							Type:        "string",
							Description: "Namespace",
							JSONPath:    ".metadata.namespace",
							Priority:    0,
						},
						{
							Name:        "State",
							Type:        "string",
							Description: "State",
							JSONPath:    ".status.state",
							Priority:    0,
						},
						{
							Name:        "CompletionTime",
							Type:        "date",
							Description: "CompletionTime",
							JSONPath:    ".status.completionTime",
							Priority:    0,
						},
						{
							Name:        "Message",
							Type:        "string",
							Description: "Message",
							JSONPath:    ".status.message",
							Priority:    1,
						},
						{
							Name:        "LocalVolumeHandle",
							Type:        "string",
							Description: "LocalVolumeHandle",
							JSONPath:    ".status.localVolumeHandle",
							Priority:    1,
						},
						{
							Name:        "RemoteVolumeHandle",
							Type:        "string",
							Description: "RemoteVolumeHandle",
							JSONPath:    ".status.remoteVolumeHandle",
							Priority:    1,
						},
					},
				},
			},
		},
	}
}

func getSnapshotInfoAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
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

	return apiextensionsv1.CustomResourceDefinition{
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
}

func getStorageClassAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}
	return apiextensionsv1.CustomResourceDefinition{
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
}

func getVolumeAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema1 := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
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
}

func getVolumePublicationAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
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
			},
			Required: []string{"volumeID", "nodeID", "readOnly"},
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
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
}

func getNodeAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
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
}

func getTransactionAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
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
}

func getSnapshotAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema1 := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
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
}

func getGroupSnapshotAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema1 := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentgroupsnapshots.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentgroupsnapshots",
				Singular:   "tridentgroupsnapshot",
				Kind:       "TridentGroupSnapshot",
				ShortNames: []string{"tgroupsnapshot", "tgroupsnap", "tgsnapshot", "tgsnap", "tgss"},
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
							Name:        "Created",
							Type:        "date",
							Description: "Creation time of the group snapshot",
							JSONPath:    ".dateCreated",
						},
					},
				},
			},
		},
	}
}

func getVolumeReferenceAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
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

	return apiextensionsv1.CustomResourceDefinition{
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
}

func getActionSnapshotRestoreAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentactionsnapshotrestores.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentactionsnapshotrestores",
				Singular:   "tridentactionsnapshotrestore",
				Kind:       "TridentActionSnapshotRestore",
				ShortNames: []string{"tasr"},
				Categories: []string{"trident", "trident-external"},
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
							Name:        "Namespace",
							Type:        "string",
							Description: "Namespace",
							JSONPath:    ".metadata.namespace",
							Priority:    0,
						},
						{
							Name:        "PVC",
							Type:        "string",
							Description: "PVC",
							JSONPath:    ".spec.pvcName",
							Priority:    0,
						},
						{
							Name:        "Snapshot",
							Type:        "string",
							Description: "Snapshot",
							JSONPath:    ".spec.volumeSnapshotName",
							Priority:    0,
						},
						{
							Name:        "State",
							Type:        "string",
							Description: "State",
							JSONPath:    ".status.state",
							Priority:    0,
						},
						{
							Name:        "CompletionTime",
							Type:        "date",
							Description: "CompletionTime",
							JSONPath:    ".status.completionTime",
							Priority:    0,
						},
						{
							Name:        "Message",
							Type:        "string",
							Description: "Message",
							JSONPath:    ".status.message",
							Priority:    1,
						},
					},
				},
			},
		},
	}
}

func getConfiguratorsAPIExtension(t *testing.T) apiextensionsv1.CustomResourceDefinition {
	t.Helper()
	preserveValue := true
	schema := apiextensionsv1.CustomResourceValidation{
		OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
			Type:                   "object",
			XPreserveUnknownFields: &preserveValue,
		},
	}

	return apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CustomResourceDefinition",
			APIVersion: "apiextensions.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "tridentconfigurators.trident.netapp.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "trident.netapp.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:     "tridentconfigurators",
				Singular:   "tridentconfigurator",
				Kind:       "TridentConfigurator",
				ShortNames: []string{"tconf", "tconfigurator"},
				Categories: []string{"trident", "trident-internal", "trident-external"},
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
					AdditionalPrinterColumns: []apiextensionsv1.CustomResourceColumnDefinition{
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
							Name:        "Cloud Provider",
							Type:        "string",
							Description: "The name of cloud provider",
							Priority:    int32(0),
							JSONPath:    ".status.cloudProvider",
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
}

func TestGetBackendConfigCRDYAML(t *testing.T) {
	expected := getBackendConfigAPIExtension(t)
	actualYAML := GetBackendConfigCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetConfiguratorCRDYAML(t *testing.T) {
	expected := getConfiguratorsAPIExtension(t)
	actualYAML := GetConfiguratorCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetMirrorRelationshipCRDYAML(t *testing.T) {
	expected := getMirrorRelationshipAPIExtension(t)
	actualYAML := GetMirrorRelationshipCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.Equal(t, expected.Spec, actual.Spec)
}

func TestGetSnapshotInfoCRDYAML(t *testing.T) {
	expected := getSnapshotInfoAPIExtension(t)
	actualYAML := GetSnapshotInfoCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetStorageClassCRDYAML(t *testing.T) {
	expected := getStorageClassAPIExtension(t)
	actualYAML := GetStorageClassCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetVolumeCRDYAML(t *testing.T) {
	expected := getVolumeAPIExtension(t)
	actualYAML := GetVolumeCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetVolumePublicationCRDYAML(t *testing.T) {
	expected := getVolumePublicationAPIExtension(t)
	actualYAML := GetVolumePublicationCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetNodeCRDYAML(t *testing.T) {
	expected := getNodeAPIExtension(t)
	actualYAML := GetNodeCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetTransactionCRDYAML(t *testing.T) {
	expected := getTransactionAPIExtension(t)
	actualYAML := GetTransactionCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetSnapshotCRDYAML(t *testing.T) {
	expected := getSnapshotAPIExtension(t)
	actualYAML := GetSnapshotCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetGroupSnapshotCRDYAML(t *testing.T) {
	expected := getGroupSnapshotAPIExtension(t)
	actualYAML := GetGroupSnapshotCRDYAML()

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

func TestGetActionMirrorUpdateCRDYAML(t *testing.T) {
	expected := getActionMirrorUpdateAPIExtension(t)
	actualYAML := GetActionMirrorUpdateCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetActionSnapshotRestoreCRDYAML(t *testing.T) {
	expected := getActionSnapshotRestoreAPIExtension(t)
	actualYAML := GetActionSnapshotRestoreCRDYAML()

	var actual apiextensionsv1.CustomResourceDefinition
	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
}

func TestGetCSIDriverYAML(t *testing.T) {
	name := "csi.trident.netapp.io"
	required := true
	fsGroupPolicy := csiv1.ReadWriteOnceWithFSTypeFSGroupPolicy
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
			FSGroupPolicy:  &fsGroupPolicy,
		},
	}

	var actual csiv1.CSIDriver
	actualYAML := GetCSIDriverYAML(name, strings.ToLower(string(csiv1.ReadWriteOnceWithFSTypeFSGroupPolicy)), nil, nil)

	assert.Nil(t, yaml.Unmarshal([]byte(actualYAML), &actual), "invalid YAML")
	assert.True(t, reflect.DeepEqual(expected.TypeMeta, actual.TypeMeta))
	assert.True(t, reflect.DeepEqual(expected.ObjectMeta, actual.ObjectMeta))
	assert.True(t, reflect.DeepEqual(expected.Spec, actual.Spec))
	assert.Equal(t, "csi.trident.netapp.io", actual.Name)
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

// TestGenerateResourcesYAML tests the generateResourcesYAML function
func TestGenerateResourcesYAML(t *testing.T) {
	tests := []struct {
		name              string
		containerResource *commonconfig.ContainerResource
		indent            string
		expectedYAML      string
	}{
		{
			name:              "nil container resource",
			containerResource: nil,
			indent:            "",
			expectedYAML:      "",
		},
		{
			name:              "empty container resource",
			containerResource: &commonconfig.ContainerResource{},
			indent:            "",
			expectedYAML:      "",
		},
		{
			name: "nil requests and limits",
			containerResource: &commonconfig.ContainerResource{
				Requests: nil,
				Limits:   nil,
			},
			indent:       "",
			expectedYAML: "",
		},
		{
			name: "empty requests - both CPU and memory zero",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")),
					Memory: convert.ToPtr(resource.MustParse("0")),
				},
			},
			indent:       "",
			expectedYAML: "",
		},
		{
			name: "empty limits - both CPU and memory zero",
			containerResource: &commonconfig.ContainerResource{
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")),
					Memory: convert.ToPtr(resource.MustParse("0")),
				},
			},
			indent:       "",
			expectedYAML: "",
		},
		{
			name: "requests only - CPU and memory",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    cpu: 100m
    memory: 128Mi
`,
		},
		{
			name: "limits only - CPU and memory",
			containerResource: &commonconfig.ContainerResource{
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("500m")),
					Memory: convert.ToPtr(resource.MustParse("512Mi")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  limits:
    cpu: 500m
    memory: 512Mi
`,
		},
		{
			name: "requests and limits - both CPU and memory",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("500m")),
					Memory: convert.ToPtr(resource.MustParse("512Mi")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
`,
		},
		{
			name: "requests only - CPU only",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("200m")),
					Memory: convert.ToPtr(resource.MustParse("0")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    cpu: 200m
`,
		},
		{
			name: "requests only - memory only",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")),
					Memory: convert.ToPtr(resource.MustParse("256Mi")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    memory: 256Mi
`,
		},
		{
			name: "limits only - CPU only",
			containerResource: &commonconfig.ContainerResource{
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("1")),
					Memory: convert.ToPtr(resource.MustParse("0")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  limits:
    cpu: 1
`,
		},
		{
			name: "limits only - memory only",
			containerResource: &commonconfig.ContainerResource{
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")),
					Memory: convert.ToPtr(resource.MustParse("1Gi")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  limits:
    memory: 1Gi
`,
		},
		{
			name: "mixed - requests CPU only, limits memory only",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("150m")),
					Memory: convert.ToPtr(resource.MustParse("0")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")),
					Memory: convert.ToPtr(resource.MustParse("2Gi")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    cpu: 150m
  limits:
    memory: 2Gi
`,
		},
		{
			name: "mixed - requests memory only, limits CPU only",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")),
					Memory: convert.ToPtr(resource.MustParse("64Mi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("2")),
					Memory: convert.ToPtr(resource.MustParse("0")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    memory: 64Mi
  limits:
    cpu: 2
`,
		},
		{
			name: "with custom indent",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
			},
			indent: "        ",
			expectedYAML: `resources:
          requests:
            cpu: 100m
            memory: 128Mi
`,
		},
		{
			name: "with tabs as indent",
			containerResource: &commonconfig.ContainerResource{
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("500m")),
					Memory: convert.ToPtr(resource.MustParse("512Mi")),
				},
			},
			indent: "\t\t",
			expectedYAML: `resources:
		  limits:
		    cpu: 500m
		    memory: 512Mi
`,
		},
		{
			name: "fractional CPU values",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0.5")),
					Memory: convert.ToPtr(resource.MustParse("0")), // Must provide Memory even if zero
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("1.5")),
					Memory: convert.ToPtr(resource.MustParse("0")), // Must provide Memory even if zero
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    cpu: 500m
  limits:
    cpu: 1500m
`,
		},
		{
			name: "large memory values",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")), // Must provide CPU even if zero
					Memory: convert.ToPtr(resource.MustParse("4Gi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")), // Must provide CPU even if zero
					Memory: convert.ToPtr(resource.MustParse("8Gi")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    memory: 4Gi
  limits:
    memory: 8Gi
`,
		},
		{
			name: "small memory values in bytes",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")), // Must provide CPU even if zero
					Memory: convert.ToPtr(resource.MustParse("1024")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")), // Must provide CPU even if zero
					Memory: convert.ToPtr(resource.MustParse("2048")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    memory: 1024
  limits:
    memory: 2048
`,
		},
		{
			name: "zero CPU in requests - only memory",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    memory: 128Mi
`,
		},
		{
			name: "zero memory in limits - only CPU",
			containerResource: &commonconfig.ContainerResource{
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("500m")),
					Memory: convert.ToPtr(resource.MustParse("0")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  limits:
    cpu: 500m
`,
		},
		{
			name: "zero values and non-zero mixed",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("500m")),
					Memory: convert.ToPtr(resource.MustParse("0")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    memory: 128Mi
  limits:
    cpu: 500m
`,
		},
		{
			name: "CPU cores as integers",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("1")),
					Memory: convert.ToPtr(resource.MustParse("0")), // Must provide Memory even if zero
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("4")),
					Memory: convert.ToPtr(resource.MustParse("0")), // Must provide Memory even if zero
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    cpu: 1
  limits:
    cpu: 4
`,
		},
		{
			name: "edge case - very small CPU values",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("1m")),
					Memory: convert.ToPtr(resource.MustParse("0")), // Must provide Memory even if zero
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("10m")),
					Memory: convert.ToPtr(resource.MustParse("0")), // Must provide Memory even if zero
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    cpu: 1m
  limits:
    cpu: 10m
`,
		},
		{
			name: "edge case - very small memory values",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")), // Must provide CPU even if zero
					Memory: convert.ToPtr(resource.MustParse("1Ki")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")), // Must provide CPU even if zero
					Memory: convert.ToPtr(resource.MustParse("1Mi")),
				},
			},
			indent: "",
			expectedYAML: `resources:
  requests:
    memory: 1Ki
  limits:
    memory: 1Mi
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := generateResourcesYAML(tt.containerResource, tt.indent)
			assert.Equal(t, tt.expectedYAML, result, "YAML output should match expected")
		})
	}
}

// TestReplaceResourcesInTemplate tests the replaceResourcesInTemplate function.
func TestReplaceResourcesInTemplate(t *testing.T) {
	tests := []struct {
		name              string
		template          string
		containerName     string
		containerResource *commonconfig.ContainerResource
		expectedTemplate  string
	}{
		{
			name:          "template without resource tag",
			template:      "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: test",
			containerName: "trident-main",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
			},
			expectedTemplate: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: test",
		},
		{
			name: "replace simple requests and limits",
			template: `containers:
- name: trident-main
  image: netapp/trident:latest
  {TRIDENT_MAIN_RESOURCES}
  env:
  - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("500m")),
					Memory: convert.ToPtr(resource.MustParse("512Mi")),
				},
			},
			expectedTemplate: `containers:
- name: trident-main
  image: netapp/trident:latest
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi
  env:
  - name: TZ`,
		},
		{
			name: "replace requests only",
			template: `containers:
- name: trident-main
  image: netapp/trident:latest
  {TRIDENT_MAIN_RESOURCES}
  env:
  - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("200m")),
					Memory: convert.ToPtr(resource.MustParse("256Mi")),
				},
			},
			expectedTemplate: `containers:
- name: trident-main
  image: netapp/trident:latest
  resources:
    requests:
      cpu: 200m
      memory: 256Mi
  env:
  - name: TZ`,
		},
		{
			name: "replace limits only",
			template: `containers:
- name: trident-main
  image: netapp/trident:latest
  {TRIDENT_MAIN_RESOURCES}
  env:
  - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("1")),
					Memory: convert.ToPtr(resource.MustParse("1Gi")),
				},
			},
			expectedTemplate: `containers:
- name: trident-main
  image: netapp/trident:latest
  resources:
    limits:
      cpu: 1
      memory: 1Gi
  env:
  - name: TZ`,
		},
		{
			name: "replace with different indentation - 2 spaces",
			template: `spec:
  containers:
  - name: trident-main
    image: netapp/trident:latest
    {TRIDENT_MAIN_RESOURCES}
    env:
    - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
			},
			expectedTemplate: `spec:
  containers:
  - name: trident-main
    image: netapp/trident:latest
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
    env:
    - name: TZ`,
		},
		{
			name: "replace with different indentation - 4 spaces",
			template: `spec:
    containers:
    - name: trident-main
      image: netapp/trident:latest
      {TRIDENT_MAIN_RESOURCES}
      env:
      - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("500m")),
					Memory: convert.ToPtr(resource.MustParse("512Mi")),
				},
			},
			expectedTemplate: `spec:
    containers:
    - name: trident-main
      image: netapp/trident:latest
      resources:
        limits:
          cpu: 500m
          memory: 512Mi
      env:
      - name: TZ`,
		},
		{
			name: "replace with tab indentation",
			template: `spec:
	containers:
	- name: trident-main
	  image: netapp/trident:latest
	  {TRIDENT_MAIN_RESOURCES}
	  env:
	  - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("150m")),
					Memory: convert.ToPtr(resource.MustParse("64Mi")),
				},
			},
			expectedTemplate: `spec:
	containers:
	- name: trident-main
	  image: netapp/trident:latest
	  resources:
	    requests:
	      cpu: 150m
	      memory: 64Mi
	  env:
	  - name: TZ`,
		},
		{
			name: "multiple container resource tags - replace only matching one",
			template: `containers:
- name: trident-main
  image: netapp/trident:latest
  {TRIDENT_MAIN_RESOURCES}
  env:
  - name: TZ
- name: trident-autosupport
  image: netapp/trident-autosupport:latest
  {TRIDENT_AUTOSUPPORT_RESOURCES}
  env:
  - name: INTERVAL`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
			},
			expectedTemplate: `containers:
- name: trident-main
  image: netapp/trident:latest
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
  env:
  - name: TZ
- name: trident-autosupport
  image: netapp/trident-autosupport:latest
  {TRIDENT_AUTOSUPPORT_RESOURCES}
  env:
  - name: INTERVAL`,
		},
		{
			name: "resource tag at start of line",
			template: `containers:
- name: trident-main
  image: netapp/trident:latest
{TRIDENT_MAIN_RESOURCES}
  env:
  - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("0")),
				},
			},
			expectedTemplate: `containers:
- name: trident-main
  image: netapp/trident:latest
resources:
  requests:
    cpu: 100m
  env:
  - name: TZ`,
		},
		{
			name: "resource tag with mixed indentation (spaces and tabs)",
			template: `spec:
	containers:
	- name: trident-main
	  image: netapp/trident:latest
		{TRIDENT_MAIN_RESOURCES}
	  env:
	  - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")),
					Memory: convert.ToPtr(resource.MustParse("1Gi")),
				},
			},
			expectedTemplate: `spec:
	containers:
	- name: trident-main
	  image: netapp/trident:latest
		resources:
		  limits:
		    memory: 1Gi
	  env:
	  - name: TZ`,
		},
		{
			name: "complex template with multiple sections",
			template: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: trident-csi
spec:
  template:
    spec:
      containers:
      - name: trident-main
        image: netapp/trident:latest
        {TRIDENT_MAIN_RESOURCES}
        env:
        - name: TZ
        ports:
        - containerPort: 8443`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("500m")),
					Memory: convert.ToPtr(resource.MustParse("512Mi")),
				},
			},
			expectedTemplate: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: trident-csi
spec:
  template:
    spec:
      containers:
      - name: trident-main
        image: netapp/trident:latest
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
        env:
        - name: TZ
        ports:
        - containerPort: 8443`,
		},
		{
			name: "resource tag with CPU only",
			template: `containers:
- name: trident-main
  image: netapp/trident:latest
  {TRIDENT_MAIN_RESOURCES}
  env:
  - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("250m")),
					Memory: convert.ToPtr(resource.MustParse("0")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("1")),
					Memory: convert.ToPtr(resource.MustParse("0")),
				},
			},
			expectedTemplate: `containers:
- name: trident-main
  image: netapp/trident:latest
  resources:
    requests:
      cpu: 250m
    limits:
      cpu: 1
  env:
  - name: TZ`,
		},
		{
			name: "resource tag with memory only",
			template: `containers:
- name: trident-main
  image: netapp/trident:latest
  {TRIDENT_MAIN_RESOURCES}
  env:
  - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")),
					Memory: convert.ToPtr(resource.MustParse("64Mi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0")),
					Memory: convert.ToPtr(resource.MustParse("2Gi")),
				},
			},
			expectedTemplate: `containers:
- name: trident-main
  image: netapp/trident:latest
  resources:
    requests:
      memory: 64Mi
    limits:
      memory: 2Gi
  env:
  - name: TZ`,
		},
		{
			name: "autosupport container resources",
			template: `containers:
- name: trident-autosupport
  image: netapp/trident-autosupport:latest
  {TRIDENT_AUTOSUPPORT_RESOURCES}
  command:
  - /usr/local/bin/trident-autosupport`,
			containerName: "{TRIDENT_AUTOSUPPORT_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("10m")),
					Memory: convert.ToPtr(resource.MustParse("16Mi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
			},
			expectedTemplate: `containers:
- name: trident-autosupport
  image: netapp/trident-autosupport:latest
  resources:
    requests:
      cpu: 10m
      memory: 16Mi
    limits:
      cpu: 100m
      memory: 128Mi
  command:
  - /usr/local/bin/trident-autosupport`,
		},
		{
			name: "node container resources",
			template: `containers:
- name: trident-node
  image: netapp/trident:latest
  {TRIDENT_NODE_RESOURCES}
  securityContext:
    privileged: true`,
			containerName: "{TRIDENT_NODE_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("200m")),
					Memory: convert.ToPtr(resource.MustParse("256Mi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("2")),
					Memory: convert.ToPtr(resource.MustParse("4Gi")),
				},
			},
			expectedTemplate: `containers:
- name: trident-node
  image: netapp/trident:latest
  resources:
    requests:
      cpu: 200m
      memory: 256Mi
    limits:
      cpu: 2
      memory: 4Gi
  securityContext:
    privileged: true`,
		},
		{
			name: "case sensitive container name mismatch",
			template: `containers:
- name: trident-main
  image: netapp/trident:latest
  {trident_main_resources}
  env:
  - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
			},
			expectedTemplate: `containers:
- name: trident-main
  image: netapp/trident:latest
  {trident_main_resources}
  env:
  - name: TZ`,
		},
		{
			name: "fractional CPU and large memory values",
			template: `containers:
- name: trident-main
  image: netapp/trident:latest
  {TRIDENT_MAIN_RESOURCES}
  env:
  - name: TZ`,
			containerName: "{TRIDENT_MAIN_RESOURCES}",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("0.5")),
					Memory: convert.ToPtr(resource.MustParse("8Gi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("2.5")),
					Memory: convert.ToPtr(resource.MustParse("16Gi")),
				},
			},
			expectedTemplate: `containers:
- name: trident-main
  image: netapp/trident:latest
  resources:
    requests:
      cpu: 500m
      memory: 8Gi
    limits:
      cpu: 2500m
      memory: 16Gi
  env:
  - name: TZ`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := replaceResourcesInTemplate(tt.template, tt.containerName, tt.containerResource)
			assert.Equal(t, tt.expectedTemplate, result, "Template replacement should match expected output")
		})
	}
}

// TestReplaceResourcesInTemplate_Integration tests the integration with real-world template scenarios
func TestReplaceResourcesInTemplate_Integration(t *testing.T) {
	tests := []struct {
		name                string
		template            string
		containerName       string
		containerResource   *commonconfig.ContainerResource
		expectedContains    []string
		expectedNotContains []string
	}{
		{
			name: "deployment template with full resource specification",
			template: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: trident-csi
  namespace: trident
spec:
  replicas: 1
  template:
    metadata:
      labels:
        app: controller.csi.trident.netapp.io
    spec:
      serviceAccount: trident-csi
      containers:
      - name: trident-main
        image: netapp/trident:latest
        ports:
        - containerPort: 8443
        {TRIDENT_MAIN_RESOURCES}
        command:
        - /trident_orchestrator
        env:
        - name: TZ
          value: UTC`,
			containerName: "TRIDENT_MAIN",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("100m")),
					Memory: convert.ToPtr(resource.MustParse("128Mi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("500m")),
					Memory: convert.ToPtr(resource.MustParse("512Mi")),
				},
			},
			expectedContains: []string{
				"resources:",
				"requests:",
				"cpu: 100m",
				"memory: 128Mi",
				"limits:",
				"cpu: 500m",
				"memory: 512Mi",
				"containerPort: 8443",
				"command:",
			},
			expectedNotContains: []string{
				"{TRIDENT_MAIN_RESOURCES}",
			},
		},
		{
			name: "daemonset template with node resources",
			template: `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: trident-csi
  namespace: trident
spec:
  template:
    spec:
      hostNetwork: true
      hostIPC: true
      containers:
      - name: trident-node
        image: netapp/trident:latest
        securityContext:
          privileged: true
        {TRIDENT_NODE_RESOURCES}
        env:
        - name: KUBE_NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName`,
			containerName: "TRIDENT_NODE",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("200m")),
					Memory: convert.ToPtr(resource.MustParse("256Mi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("2")),
					Memory: convert.ToPtr(resource.MustParse("4Gi")),
				},
			},
			expectedContains: []string{
				"resources:",
				"requests:",
				"cpu: 200m",
				"memory: 256Mi",
				"limits:",
				"cpu: 2",
				"memory: 4Gi",
				"privileged: true",
				"KUBE_NODE_NAME",
			},
			expectedNotContains: []string{
				"{TRIDENT_NODE_RESOURCES}",
			},
		},
		{
			name: "multi-container deployment with autosupport",
			template: `containers:
- name: trident-main
  image: netapp/trident:latest
  {TRIDENT_MAIN_RESOURCES}
  ports:
  - containerPort: 8443
- name: trident-autosupport
  image: netapp/trident-autosupport:latest
  {TRIDENT_AUTOSUPPORT_RESOURCES}
  command:
  - /usr/local/bin/trident-autosupport`,
			containerName: "TRIDENT_AUTOSUPPORT",
			containerResource: &commonconfig.ContainerResource{
				Requests: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("10m")),
					Memory: convert.ToPtr(resource.MustParse("16Mi")),
				},
				Limits: &commonconfig.ResourceRequirements{
					CPU:    convert.ToPtr(resource.MustParse("50m")),
					Memory: convert.ToPtr(resource.MustParse("64Mi")),
				},
			},
			expectedContains: []string{
				"- name: trident-autosupport",
				"resources:",
				"requests:",
				"cpu: 10m",
				"memory: 16Mi",
				"limits:",
				"cpu: 50m",
				"memory: 64Mi",
				"trident-autosupport:latest",
				"{TRIDENT_MAIN_RESOURCES}", // This should remain untouched
			},
			expectedNotContains: []string{
				"{TRIDENT_AUTOSUPPORT_RESOURCES}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := replaceResourcesInTemplate(tt.template, tt.containerName, tt.containerResource)

			// Check that all expected strings are present
			for _, expected := range tt.expectedContains {
				assert.Contains(t, result, expected, "Result should contain expected string: %s", expected)
			}

			// Check that none of the unwanted strings are present
			for _, notExpected := range tt.expectedNotContains {
				assert.NotContains(t, result, notExpected, "Result should not contain string: %s", notExpected)
			}

			if tt.containerResource != nil &&
				((tt.containerResource.Requests != nil && (!tt.containerResource.Requests.CPU.IsZero() || !tt.containerResource.Requests.Memory.IsZero())) ||
					(tt.containerResource.Limits != nil && (!tt.containerResource.Limits.CPU.IsZero() || !tt.containerResource.Limits.Memory.IsZero()))) {
				assert.Contains(t, result, "resources:", "Should contain resources section when resources are specified")
			}
		})
	}
}
