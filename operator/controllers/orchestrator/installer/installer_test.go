// Copyright 2023 NetApp, Inc. All Rights Reserved.

package installer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/brunoga/deep"
	"github.com/distribution/reference"
	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	commonconfig "github.com/netapp/trident/config"
	mockExtendedK8sClient "github.com/netapp/trident/mocks/mock_operator/mock_controllers/mock_orchestrator/mock_installer"
	"github.com/netapp/trident/operator/config"
	netappv1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/version"
)

const (
	Namespace = "trident"
)

var (
	encryptSecretName = getEncryptionSecretName()
	k8sClientError    = fmt.Errorf("k8s error")
)

func newMockKubeClient(t *testing.T) *mockExtendedK8sClient.MockExtendedK8sClient {
	mockCtrl := gomock.NewController(t)
	mockK8sClient := mockExtendedK8sClient.NewMockExtendedK8sClient(mockCtrl)
	return mockK8sClient
}

func newTestInstaller(client *mockExtendedK8sClient.MockExtendedK8sClient) *Installer {
	return &Installer{
		client:           client,
		tridentCRDClient: nil,
		namespace:        Namespace,
	}
}

func createTestControllingCRDetails() map[string]string {
	controllingCRDetails := make(map[string]string)
	controllingCRDetails[CRAPIVersionKey] = "v01.01.01"
	controllingCRDetails[CRController] = "trident-orchestrator"
	controllingCRDetails[CRKind] = "deployment"
	controllingCRDetails[CRName] = "trident-csi"
	controllingCRDetails[CRUID] = CRUID

	return controllingCRDetails
}

func createTestLabels() map[string]string {
	labels := make(map[string]string)
	labels[appLabelKey] = appLabelValue
	labels[K8sVersionLabelKey] = "v1.28.8"
	labels[TridentVersionLabelKey] = "v26.02.0"

	return labels
}

func TestInstaller_createOrConsumeTridentEncryptionSecret(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)
	controllingCRDetails := createTestControllingCRDetails()
	labels := createTestLabels()

	// K8s error at CheckSecretExists
	mockK8sClient.EXPECT().CheckSecretExists(encryptSecretName).Return(false, k8sClientError)
	expectedErr := installer.createOrConsumeTridentEncryptionSecret(controllingCRDetails, labels, false)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	// Secret already exists and no K8s error
	mockK8sClient.EXPECT().CheckSecretExists(encryptSecretName).Return(true, nil)
	expectedErr = installer.createOrConsumeTridentEncryptionSecret(controllingCRDetails, labels, false)
	assert.Nil(t, expectedErr, "expected nil error")

	// Create secret with no failure at PutSecret
	mockK8sClient.EXPECT().CheckSecretExists(encryptSecretName).Return(false, nil)
	mockK8sClient.EXPECT().PutSecret(true, gomock.Any(), encryptSecretName).Return(nil)
	expectedErr = installer.createOrConsumeTridentEncryptionSecret(controllingCRDetails, labels, false)
	assert.Nil(t, expectedErr, "expected nil error")

	// Create secret with a failure at PutSecret
	mockK8sClient.EXPECT().CheckSecretExists(encryptSecretName).Return(false, nil)
	mockK8sClient.EXPECT().PutSecret(true, gomock.Any(), encryptSecretName).Return(k8sClientError)
	expectedErr = installer.createOrConsumeTridentEncryptionSecret(controllingCRDetails, labels, false)
	assert.NotNil(t, expectedErr, "expected no nil error")
}

func TestInstaller_createOrPatchTridentResourceQuota(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	// setup values for inputs and outputs of mocked functions.
	controllingCRDetails := createTestControllingCRDetails()
	labels := createTestLabels()
	labels[appLabelKey] = TridentNodeLabelValue
	resourceQuotaName := getResourceQuotaName()
	nodeLabel := TridentNodeLabel
	resourceQuota := &corev1.ResourceQuota{}
	unwantedResourceQuotas := []corev1.ResourceQuota{
		*resourceQuota,
	}
	emptyResourceQuotaList := make([]corev1.ResourceQuota, 0)

	// K8s error at GetResourceQuotaInformation
	mockK8sClient.EXPECT().GetResourceQuotaInformation(resourceQuotaName, nodeLabel, installer.namespace).Return(nil, nil, true, k8sClientError)
	expectedErr := installer.createOrPatchTridentResourceQuota(controllingCRDetails, labels, false)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	// "shouldUpdate" is true; failure at RemoveMultipleResourceQuotas
	mockK8sClient.EXPECT().GetResourceQuotaInformation(resourceQuotaName, nodeLabel, installer.namespace).Return(resourceQuota, emptyResourceQuotaList, false, nil)
	mockK8sClient.EXPECT().RemoveMultipleResourceQuotas(unwantedResourceQuotas).Return(k8sClientError)
	expectedErr = installer.createOrPatchTridentResourceQuota(controllingCRDetails, labels, true)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	// "shouldUpdate" is false; "createResourceQuota" is true
	mockK8sClient.EXPECT().GetResourceQuotaInformation(resourceQuotaName, nodeLabel, installer.namespace).Return(resourceQuota, emptyResourceQuotaList, true, nil)
	mockK8sClient.EXPECT().RemoveMultipleResourceQuotas(emptyResourceQuotaList).Return(nil)
	mockK8sClient.EXPECT().PutResourceQuota(resourceQuota, true, gomock.Any(), nodeLabel).Return(k8sClientError)
	expectedErr = installer.createOrPatchTridentResourceQuota(controllingCRDetails, labels, false)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	// "shouldUpdate" is false; "createResourceQuota" is false
	mockK8sClient.EXPECT().GetResourceQuotaInformation(resourceQuotaName, nodeLabel, installer.namespace).Return(resourceQuota, emptyResourceQuotaList, false, nil)
	mockK8sClient.EXPECT().RemoveMultipleResourceQuotas(emptyResourceQuotaList).Return(nil)
	mockK8sClient.EXPECT().PutResourceQuota(resourceQuota, false, gomock.Any(), nodeLabel).Return(k8sClientError)
	expectedErr = installer.createOrPatchTridentResourceQuota(controllingCRDetails, labels, false)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	// "shouldUpdate" is true; "createResourceQuota" is true
	mockK8sClient.EXPECT().GetResourceQuotaInformation(resourceQuotaName, nodeLabel, installer.namespace).Return(resourceQuota, emptyResourceQuotaList, true, nil)
	mockK8sClient.EXPECT().RemoveMultipleResourceQuotas(unwantedResourceQuotas).Return(nil)
	mockK8sClient.EXPECT().PutResourceQuota(resourceQuota, true, gomock.Any(), nodeLabel).Return(nil)
	expectedErr = installer.createOrPatchTridentResourceQuota(controllingCRDetails, labels, true)
	assert.Nil(t, expectedErr, "expected nil error")
}

func TestCreateOrPatchCRD(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	// Setup values for inputs and outputs of mocked functions.
	crdName := VersionCRDName // use any validFunc CRD name here
	crdYAML := k8sclient.GetVersionCRDYAML()

	var crd v1.CustomResourceDefinition
	if err := yaml.Unmarshal([]byte(crdYAML), &crd); err != nil {
		t.Fatalf("unable to create CRD for tests")
	}

	mockK8sClient.EXPECT().CheckCRDExists(crdName).Return(false, k8sClientError)
	expectedErr := installer.CreateOrPatchCRD(crdName, crdYAML, false)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	mockK8sClient.EXPECT().CheckCRDExists(crdName).Return(true, k8sClientError)
	expectedErr = installer.CreateOrPatchCRD(crdName, crdYAML, false)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	// If CRD exists and crdUpdateNeeded is false, don't get the crd or patch it.
	mockK8sClient.EXPECT().CheckCRDExists(crdName).Return(true, nil)
	expectedErr = installer.CreateOrPatchCRD(crdName, crdYAML, false)
	assert.Nil(t, expectedErr, "expected nil error")

	// crdUpdateNeeded is true here, so getCRD will get called.
	mockK8sClient.EXPECT().CheckCRDExists(crdName).Return(true, nil)
	mockK8sClient.EXPECT().GetCRD(crdName).Return(&crd, nil)
	mockK8sClient.EXPECT().PutCustomResourceDefinition(&crd, crdName, false, crdYAML).Return(k8sClientError)
	expectedErr = installer.CreateOrPatchCRD(crdName, crdYAML, true)
	assert.NotNil(t, expectedErr, "expected non-nil error")

	mockK8sClient.EXPECT().CheckCRDExists(crdName).Return(true, nil)
	mockK8sClient.EXPECT().GetCRD(crdName).Return(&crd, nil)
	mockK8sClient.EXPECT().PutCustomResourceDefinition(&crd, crdName, false, crdYAML).Return(nil)
	expectedErr = installer.CreateOrPatchCRD(crdName, crdYAML, true)
	assert.Nil(t, expectedErr, "expected nil error")
}

func Test_getAppLabelForResource(t *testing.T) {
	labelMap, labelValue := getAppLabelForResource(getControllerRBACResourceName())
	assert.True(t, cmp.Equal(labelMap, map[string]string{TridentAppLabelKey: TridentCSILabelValue}))
	assert.Equal(t, labelValue, TridentCSILabel)

	labelMap, labelValue = getAppLabelForResource(getNodeRBACResourceName(true))
	assert.True(t, cmp.Equal(labelMap, map[string]string{TridentAppLabelKey: TridentNodeLabelValue}))
	assert.Equal(t, labelValue, TridentNodeLabel)

	labelMap, labelValue = getAppLabelForResource(getNodeRBACResourceName(false))
	assert.True(t, cmp.Equal(labelMap, map[string]string{TridentAppLabelKey: TridentNodeLabelValue}))
	assert.Equal(t, labelValue, TridentNodeLabel)
}

func TestCloudProviderPrechecks(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	tests := []struct {
		cloudProvider string
		Valid         bool
	}{
		// Valid values
		{"", true},
		{k8sclient.CloudProviderAzure, true},
		{k8sclient.CloudProviderAWS, true},
		{"azure", true},
		{"aws", true},
		{"GCP", true},

		// Invalid values
		{"test", false},
		{"AZ", false},
		{"Oracle", false},
	}

	for _, test := range tests {
		cloudProvider = test.cloudProvider
		err := installer.cloudProviderPrechecks()
		if test.Valid {
			assert.NoError(t, err, "should be validFunc")
		} else {
			assert.Error(t, err, "should be invalid")
		}
	}
}

func TestCloudIdentityPrechecks(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	tests := []struct {
		cloudProvider string
		cloudIdentity string
		Valid         bool
	}{
		// Valid values
		{"", "", true},
		{k8sclient.CloudProviderAzure, "", true},
		{k8sclient.CloudProviderAzure, k8sclient.AzureCloudIdentityKey + " a8rry78r8-7733-49bd-6656582", true},
		{k8sclient.CloudProviderAWS, k8sclient.AWSCloudIdentityKey + "arn:aws:iam::123456789:role/test", true},
		{"azure", k8sclient.AzureCloudIdentityKey + " a8rry78r8-7733-49bd-6656582", true},
		{"aws", k8sclient.AWSCloudIdentityKey + "arn:aws:iam::123456789:role/test", true},

		// Invalid values
		{"", "123456789", false},
		{k8sclient.CloudProviderAzure, " rruuunu89-9933-49bd-134423", false},
		{k8sclient.CloudProviderAWS, "", false},
	}

	for _, test := range tests {
		cloudProvider = test.cloudProvider
		cloudIdentity = test.cloudIdentity
		err := installer.cloudIdentityPrechecks()
		if test.Valid {
			assert.NoError(t, err, "should be validFunc")
		} else {
			assert.Error(t, err, "should be invalid")
		}
	}
}

func TestFSGroupPolicyPrechecks(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	tests := []struct {
		fsGroupPolicy string
		Valid         bool
	}{
		// Valid values
		{"", true},
		{string(storagev1.ReadWriteOnceWithFSTypeFSGroupPolicy), true},
		{strings.ToLower(string(storagev1.ReadWriteOnceWithFSTypeFSGroupPolicy)), true},
		{string(storagev1.FileFSGroupPolicy), true},
		{strings.ToLower(string(storagev1.FileFSGroupPolicy)), true},
		{string(storagev1.NoneFSGroupPolicy), true},
		{strings.ToLower(string(storagev1.NoneFSGroupPolicy)), true},

		// Invalid values
		{"test", false},
	}

	for _, test := range tests {
		fsGroupPolicy = test.fsGroupPolicy
		err := installer.fsGroupPolicyPrechecks()
		if test.Valid {
			assert.NoError(t, err, "should be valid")
		} else {
			assert.Error(t, err, "should be invalid")
		}
	}
}

func TestSetInstallationParams_NodePrep(t *testing.T) {
	tests := []struct {
		name        string
		nodePrep    []string
		assertValid assert.ErrorAssertionFunc
	}{
		{name: "validFunc nil", nodePrep: nil, assertValid: assert.NoError},
		{name: "validFunc empty", nodePrep: []string{}, assertValid: assert.NoError},
		{name: "validFunc one", nodePrep: []string{"iSCSI"}, assertValid: assert.NoError},
		{name: "invalid one", nodePrep: []string{"NVME"}, assertValid: assert.Error},
		{name: "invalid list", nodePrep: []string{"iSCSI", "NVME"}, assertValid: assert.Error},
	}

	mockK8sClient := newMockKubeClient(t)
	mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
	installer := newTestInstaller(mockK8sClient)

	to := netappv1.TridentOrchestrator{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       netappv1.TridentOrchestratorSpec{},
		Status:     netappv1.TridentOrchestratorStatus{},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			to.Spec.NodePrep = test.nodePrep
			_, _, _, err := installer.setInstallationParams(to, "")
			test.assertValid(t, err)
		})
	}
}

func TestSetInstallationParams_Images(t *testing.T) {
	tag := ":v1.2.3"
	images := map[string]struct {
		image  *string
		envval string
	}{
		config.TridentImageEnv: {
			&tridentImage,
			"test-registry-2/" + strings.ToLower(config.TridentImageEnv) + tag,
		},
		config.AutosupportImageEnv: {
			&autosupportImage,
			"test-registry-2/" + strings.ToLower(config.AutosupportImageEnv) + tag,
		},
		config.CSISidecarProvisionerImageEnv: {
			&csiSidecarProvisionerImage,
			"test-registry-2/" + strings.ToLower(config.CSISidecarProvisionerImageEnv) + tag,
		},
		config.CSISidecarAttacherImageEnv: {
			&csiSidecarAttacherImage,
			"test-registry-2/" + strings.ToLower(config.CSISidecarAttacherImageEnv) + tag,
		},
		config.CSISidecarResizerImageEnv: {
			&csiSidecarResizerImage,
			"test-registry-2/" + strings.ToLower(config.CSISidecarResizerImageEnv) + tag,
		},
		config.CSISidecarSnapshotterImageEnv: {
			&csiSidecarSnapshotterImage,
			"test-registry-2/" + strings.ToLower(config.CSISidecarSnapshotterImageEnv) + tag,
		},
		config.CSISidecarNodeDriverRegistrarImageEnv: {
			&csiSidecarNodeDriverRegistrarImage,
			"test-registry-2/" + strings.ToLower(config.CSISidecarNodeDriverRegistrarImageEnv) + tag,
		},
		config.CSISidecarLivenessProbeImageEnv: {
			&csiSidecarLivenessProbeImage,
			"test-registry-2/" + strings.ToLower(config.CSISidecarLivenessProbeImageEnv) + tag,
		},
	}
	tests := []struct {
		name           string
		imageRegistry  string
		setEnvironment bool
	}{
		{
			name: "default",
		},
		{
			name:          "image registry set",
			imageRegistry: "test-registry.example.com",
		},
		{
			name:           "environment set",
			setEnvironment: true,
		},
		{
			name:           "environment and image registry set",
			imageRegistry:  "test-registry.example.com",
			setEnvironment: true,
		},
	}

	mockK8sClient := newMockKubeClient(t)
	mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, image := range images {
				*image.image = ""
			}

			to := netappv1.TridentOrchestrator{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec:       netappv1.TridentOrchestratorSpec{},
				Status:     netappv1.TridentOrchestratorStatus{},
			}
			installer := newTestInstaller(mockK8sClient)

			if test.setEnvironment {
				for env, image := range images {
					t.Setenv(env, image.envval)
				}
			}

			to.Spec.ImageRegistry = test.imageRegistry
			_, _, _, err := installer.setInstallationParams(to, "")
			assert.NoError(t, err)

			// Check that all images are valid or empty
			for env, image := range images {
				_, err := reference.Parse(*image.image)
				assert.NoError(t, err, "invalid image %s", *image.image)

				if test.setEnvironment {
					assert.Contains(t, *image.image, strings.ToLower(env))
					assert.True(t, strings.HasSuffix(*image.image, tag))

					if test.imageRegistry != "" {
						assert.True(t, strings.HasPrefix(*image.image, test.imageRegistry), "invalid image: %s",
							*image.image)
					}
				}
			}
		})
	}
}

func TestSetInstallationParams_FSGroupPolicy(t *testing.T) {
	tests := []struct {
		name          string
		fsGroupPolicy string
		assertValid   assert.ErrorAssertionFunc
	}{
		{name: "validFunc nil", fsGroupPolicy: "", assertValid: assert.NoError},
		{name: "validFunc empty", fsGroupPolicy: string(storagev1.ReadWriteOnceWithFSTypeFSGroupPolicy), assertValid: assert.NoError},
		{name: "validFunc empty", fsGroupPolicy: string(storagev1.FileFSGroupPolicy), assertValid: assert.NoError},
		{name: "validFunc empty", fsGroupPolicy: string(storagev1.NoneFSGroupPolicy), assertValid: assert.NoError},
		{name: "invalid one", fsGroupPolicy: "invalid", assertValid: assert.Error},
	}

	mockK8sClient := newMockKubeClient(t)
	mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
	installer := newTestInstaller(mockK8sClient)

	to := netappv1.TridentOrchestrator{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       netappv1.TridentOrchestratorSpec{},
		Status:     netappv1.TridentOrchestratorStatus{},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			to.Spec.FSGroupPolicy = test.fsGroupPolicy
			_, _, _, err := installer.setInstallationParams(to, "")
			test.assertValid(t, err)
		})
	}
}

func TestInstaller_populateResources(t *testing.T) {
	// Initialize default resources for testing
	setupDefaultResources := func() {
		resourcesValues = deep.MustCopy(&commonconfig.DefaultResources)
	}

	tests := []struct {
		name           string
		inputCR        netappv1.TridentOrchestrator
		expectedError  bool
		errorSubstring string
		validateFunc   func(t *testing.T)
	}{
		// Positive test cases:
		{
			name:          "nil resources - should return nil",
			inputCR:       *NewTridentOrchestrator(),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				// Should not modify resourcesValues when Resources is nil
				assert.True(t, cmp.Equal(resourcesValues, &commonconfig.DefaultResources))
			},
		},
		{
			name: "valid controller resources - requests only",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "100m", "200Mi", "", ""),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				expected := mustParseQuantity("100m")
				assert.Equal(t, expected, *resourcesValues.Controller[commonconfig.TridentControllerMain].Requests.CPU)
				expected = mustParseQuantity("200Mi")
				assert.Equal(t, expected, *resourcesValues.Controller[commonconfig.TridentControllerMain].Requests.Memory)

				// Now I should also check that it didn't change other values.
				for containerName, container := range resourcesValues.Controller {
					if containerName != commonconfig.TridentControllerMain {
						assert.True(t, cmp.Equal(container, commonconfig.DefaultResources.Controller[containerName]),
							"Container %s should not have been modified", containerName)
					}
				}

				// Verify that node resources weren't modified at all
				assert.True(t, cmp.Equal(resourcesValues.Node, commonconfig.DefaultResources.Node),
					"Node resources should remain unchanged")
			},
		},
		{
			name: "valid controller resources - requests and limits",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "100m", "200Mi", "500m", "1Gi"),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				container := resourcesValues.Controller[commonconfig.TridentControllerMain]

				// Validate requests
				expected := mustParseQuantity("100m")
				assert.Equal(t, expected, *container.Requests.CPU)
				expected = mustParseQuantity("200Mi")
				assert.Equal(t, expected, *container.Requests.Memory)

				// Validate limits
				expected = mustParseQuantity("500m")
				assert.Equal(t, expected, *container.Limits.CPU)
				expected = mustParseQuantity("1Gi")
				assert.Equal(t, expected, *container.Limits.Memory)

				for containerName, container := range resourcesValues.Controller {
					if containerName != commonconfig.TridentControllerMain {
						assert.True(t, cmp.Equal(container, commonconfig.DefaultResources.Controller[containerName]),
							"Container %s should not have been modified", containerName)
					}
				}

				// Verify that node resources weren't modified at all
				assert.True(t, cmp.Equal(resourcesValues.Node, commonconfig.DefaultResources.Node),
					"Node resources should remain unchanged")
			},
		},
		{
			name: "valid node linux resources",
			inputCR: *NewTridentOrchestrator(
				WithNodeLinuxContainer(commonconfig.TridentNodeMain, "150m", "300Mi", "600m", "2Gi"),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				container := resourcesValues.Node.Linux[commonconfig.TridentNodeMain]

				// Validate requests
				expected := mustParseQuantity("150m")
				assert.Equal(t, expected, *container.Requests.CPU)
				expected = mustParseQuantity("300Mi")
				assert.Equal(t, expected, *container.Requests.Memory)

				// Validate limits
				expected = mustParseQuantity("600m")
				assert.Equal(t, expected, *container.Limits.CPU)
				expected = mustParseQuantity("2Gi")
				assert.Equal(t, expected, *container.Limits.Memory)

				for containerName, container := range resourcesValues.Node.Linux {
					if containerName != commonconfig.TridentNodeMain {
						assert.True(t, cmp.Equal(container, commonconfig.DefaultResources.Node.Linux[containerName]),
							"Container %s should not have been modified", containerName)
					}
				}

				// Verify that node resources weren't modified at all
				assert.True(t, cmp.Equal(resourcesValues.Controller, commonconfig.DefaultResources.Controller),
					"Node resources should remain unchanged")
				assert.True(t, cmp.Equal(resourcesValues.Node.Windows, commonconfig.DefaultResources.Node.Windows),
					"Node resources should remain unchanged")
			},
		},
		{
			name: "valid node windows resources",
			inputCR: *NewTridentOrchestrator(
				WithNodeWindowsContainer(commonconfig.TridentNodeMain, "120m", "250Mi", "700m", "1.5Gi"),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				container := resourcesValues.Node.Windows[commonconfig.TridentNodeMain]

				// Validate requests
				expected := mustParseQuantity("120m")
				assert.Equal(t, expected, *container.Requests.CPU)
				expected = mustParseQuantity("250Mi")
				assert.Equal(t, expected, *container.Requests.Memory)

				// Validate limits
				expected = mustParseQuantity("700m")
				assert.Equal(t, expected, *container.Limits.CPU)
				expected = mustParseQuantity("1.5Gi")
				assert.Equal(t, expected, *container.Limits.Memory)

				for containerName, container := range resourcesValues.Node.Windows {
					if containerName != commonconfig.TridentNodeMain {
						assert.True(t, cmp.Equal(container, commonconfig.DefaultResources.Node.Windows[containerName]),
							"Container %s should not have been modified", containerName)
					}
				}

				// Verify that node resources weren't modified at all
				assert.True(t, cmp.Equal(resourcesValues.Controller, commonconfig.DefaultResources.Controller),
					"Node resources should remain unchanged")
				assert.True(t, cmp.Equal(resourcesValues.Node.Linux, commonconfig.DefaultResources.Node.Linux),
					"Node resources should remain unchanged")
			},
		},
		{
			name: "multiple container resources",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "100m", "200Mi", "500m", "1Gi"),
				WithControllerContainer(commonconfig.CSISidecarProvisioner, "50m", "100Mi", "200m", "500Mi"),
				WithNodeLinuxContainer(commonconfig.TridentNodeMain, "150m", "300Mi", "600m", "2Gi"),
				WithNodeLinuxContainer(commonconfig.CSISidecarRegistrar, "25m", "50Mi", "100m", "200Mi"),
				WithNodeWindowsContainer(commonconfig.TridentNodeMain, "120m", "250Mi", "700m", "1.5Gi"),
				WithNodeWindowsContainer(commonconfig.CSISidecarRegistrar, "30m", "60Mi", "150m", "300Mi"),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				// Validate controller main
				container := resourcesValues.Controller[commonconfig.TridentControllerMain]
				assert.Equal(t, mustParseQuantity("100m"), *container.Requests.CPU)
				assert.Equal(t, mustParseQuantity("200Mi"), *container.Requests.Memory)
				assert.Equal(t, mustParseQuantity("500m"), *container.Limits.CPU)
				assert.Equal(t, mustParseQuantity("1Gi"), *container.Limits.Memory)

				// Validate controller provisioner
				container = resourcesValues.Controller[commonconfig.CSISidecarProvisioner]
				assert.Equal(t, mustParseQuantity("50m"), *container.Requests.CPU)
				assert.Equal(t, mustParseQuantity("100Mi"), *container.Requests.Memory)
				assert.Equal(t, mustParseQuantity("200m"), *container.Limits.CPU)
				assert.Equal(t, mustParseQuantity("500Mi"), *container.Limits.Memory)

				// Validate node linux main
				container = resourcesValues.Node.Linux[commonconfig.TridentNodeMain]
				assert.Equal(t, mustParseQuantity("150m"), *container.Requests.CPU)
				assert.Equal(t, mustParseQuantity("300Mi"), *container.Requests.Memory)
				assert.Equal(t, mustParseQuantity("600m"), *container.Limits.CPU)
				assert.Equal(t, mustParseQuantity("2Gi"), *container.Limits.Memory)

				// Validate node linux registrar
				container = resourcesValues.Node.Linux[commonconfig.CSISidecarRegistrar]
				assert.Equal(t, mustParseQuantity("25m"), *container.Requests.CPU)
				assert.Equal(t, mustParseQuantity("50Mi"), *container.Requests.Memory)
				assert.Equal(t, mustParseQuantity("100m"), *container.Limits.CPU)
				assert.Equal(t, mustParseQuantity("200Mi"), *container.Limits.Memory)

				// Validate node windows main
				container = resourcesValues.Node.Windows[commonconfig.TridentNodeMain]
				assert.Equal(t, mustParseQuantity("120m"), *container.Requests.CPU)
				assert.Equal(t, mustParseQuantity("250Mi"), *container.Requests.Memory)
				assert.Equal(t, mustParseQuantity("700m"), *container.Limits.CPU)
				assert.Equal(t, mustParseQuantity("1.5Gi"), *container.Limits.Memory)

				// Validate node windows registrar
				container = resourcesValues.Node.Windows[commonconfig.CSISidecarRegistrar]
				assert.Equal(t, mustParseQuantity("30m"), *container.Requests.CPU)
				assert.Equal(t, mustParseQuantity("60Mi"), *container.Requests.Memory)
				assert.Equal(t, mustParseQuantity("150m"), *container.Limits.CPU)
				assert.Equal(t, mustParseQuantity("300Mi"), *container.Limits.Memory)
			},
		},
		{
			name: "only requests specified",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "100m", "200Mi", "", ""),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				container := resourcesValues.Controller[commonconfig.TridentControllerMain]
				assert.Equal(t, mustParseQuantity("100m"), *container.Requests.CPU)
				assert.Equal(t, mustParseQuantity("200Mi"), *container.Requests.Memory)
				// Limits should remain nil if not specified
				if container.Limits != nil {
					assert.Nil(t, container.Limits.CPU)
					assert.Nil(t, container.Limits.Memory)
				}
			},
		},
		{
			name: "only limits specified",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "", "", "500m", "1Gi"),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				container := resourcesValues.Controller[commonconfig.TridentControllerMain]
				// Requests should have default values
				assert.NotNil(t, container.Requests.CPU)
				assert.NotNil(t, container.Requests.Memory)
				// Limits should be set
				assert.NotNil(t, container.Limits)
				assert.Equal(t, mustParseQuantity("500m"), *container.Limits.CPU)
				assert.Equal(t, mustParseQuantity("1Gi"), *container.Limits.Memory)
			},
		},
		{
			name: "partial requests - CPU only",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "100m", "", "", ""),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				container := resourcesValues.Controller[commonconfig.TridentControllerMain]
				assert.Equal(t, mustParseQuantity("100m"), *container.Requests.CPU)
				// Memory should keep default value
				assert.Equal(t, *commonconfig.DefaultResources.Controller[commonconfig.TridentControllerMain].Requests.Memory, *container.Requests.Memory)
			},
		},
		{
			name: "partial requests - Memory only",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "", "500Mi", "", ""),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				container := resourcesValues.Controller[commonconfig.TridentControllerMain]
				// CPU should keep default value
				assert.Equal(t, mustParseQuantity("10m"), *container.Requests.CPU)
				assert.Equal(t, mustParseQuantity("500Mi"), *container.Requests.Memory)
			},
		},
		{
			name: "partial limits - CPU only",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "", "", "800m", ""),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				container := resourcesValues.Controller[commonconfig.TridentControllerMain]
				assert.NotNil(t, container.Limits)
				assert.Equal(t, mustParseQuantity("800m"), *container.Limits.CPU)
				assert.Nil(t, container.Limits.Memory)
			},
		},
		{
			name: "partial limits - Memory only",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "", "", "", "2Gi"),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				container := resourcesValues.Controller[commonconfig.TridentControllerMain]
				assert.NotNil(t, container.Limits)
				assert.Nil(t, container.Limits.CPU)
				assert.Equal(t, mustParseQuantity("2Gi"), *container.Limits.Memory)
			},
		},
		{
			name: "empty resource strings",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "", "", "", ""),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				container := resourcesValues.Controller[commonconfig.TridentControllerMain]
				// Should keep default values when empty strings are provided
				assert.Equal(t, *commonconfig.DefaultResources.Controller[commonconfig.TridentControllerMain].Requests.CPU, *container.Requests.CPU)
				assert.Equal(t, *commonconfig.DefaultResources.Controller[commonconfig.TridentControllerMain].Requests.Memory, *container.Requests.Memory)
				// Limits should remain nil as empty strings are ignored
				if container.Limits != nil {
					assert.Nil(t, container.Limits.CPU)
					assert.Nil(t, container.Limits.Memory)
				}
			},
		},
		{
			name: "all sidecar containers",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "100m", "200Mi", "500m", "1Gi"),
				WithControllerContainer(commonconfig.CSISidecarProvisioner, "50m", "100Mi", "200m", "500Mi"),
				WithControllerContainer(commonconfig.CSISidecarAttacher, "40m", "80Mi", "150m", "300Mi"),
				WithControllerContainer(commonconfig.CSISidecarResizer, "30m", "60Mi", "120m", "250Mi"),
				WithControllerContainer(commonconfig.CSISidecarSnapshotter, "35m", "70Mi", "140m", "280Mi"),
				WithControllerContainer(commonconfig.TridentAutosupport, "20m", "40Mi", "100m", "200Mi"),
			),
			expectedError: false,
			validateFunc: func(t *testing.T) {
				// Validate all controller containers
				containers := map[string]struct{ cpu, mem, cpuLimit, memLimit string }{
					commonconfig.TridentControllerMain: {"100m", "200Mi", "500m", "1Gi"},
					commonconfig.CSISidecarProvisioner: {"50m", "100Mi", "200m", "500Mi"},
					commonconfig.CSISidecarAttacher:    {"40m", "80Mi", "150m", "300Mi"},
					commonconfig.CSISidecarResizer:     {"30m", "60Mi", "120m", "250Mi"},
					commonconfig.CSISidecarSnapshotter: {"35m", "70Mi", "140m", "280Mi"},
					commonconfig.TridentAutosupport:    {"20m", "40Mi", "100m", "200Mi"},
				}

				for containerName, expected := range containers {
					container := resourcesValues.Controller[containerName]
					assert.Equal(t, mustParseQuantity(expected.cpu), *container.Requests.CPU, "CPU request for %s", containerName)
					assert.Equal(t, mustParseQuantity(expected.mem), *container.Requests.Memory, "Memory request for %s", containerName)
					assert.Equal(t, mustParseQuantity(expected.cpuLimit), *container.Limits.CPU, "CPU limit for %s", containerName)
					assert.Equal(t, mustParseQuantity(expected.memLimit), *container.Limits.Memory, "Memory limit for %s", containerName)
				}
			},
		},

		// Negative test cases:
		{
			name: "invalid CPU request format",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "invalid-cpu", "200Mi", "", ""),
			),
			expectedError:  true,
			errorSubstring: "invalid CPU request for Controller container 'trident-main'",
		},
		{
			name: "invalid memory request format",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "100m", "invalid-memory", "", ""),
			),
			expectedError:  true,
			errorSubstring: "invalid Memory request for Controller container 'trident-main'",
		},
		{
			name: "invalid CPU limit format",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "100m", "200Mi", "invalid-cpu-limit", "1Gi"),
			),
			expectedError:  true,
			errorSubstring: "invalid CPU limit for Controller container 'trident-main'",
		},
		{
			name: "invalid memory limit format",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "100m", "200Mi", "500m", "invalid-memory-limit"),
			),
			expectedError:  true,
			errorSubstring: "invalid Memory limit for Controller container 'trident-main'",
		},
		{
			name: "invalid controller container name",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer("invalid-container", "100m", "200Mi", "", ""),
			),
			expectedError:  true,
			errorSubstring: "invalid container name 'invalid-container' for 'Controller'",
		},
		{
			name: "invalid node linux container name",
			inputCR: *NewTridentOrchestrator(
				WithNodeLinuxContainer("invalid-node-container", "100m", "200Mi", "", ""),
			),
			expectedError:  true,
			errorSubstring: "invalid container name 'invalid-node-container' for 'NodeLinux'",
		},
		{
			name: "invalid node windows container name",
			inputCR: *NewTridentOrchestrator(
				WithNodeWindowsContainer("invalid-windows-container", "100m", "200Mi", "", ""),
			),
			expectedError:  true,
			errorSubstring: "invalid container name 'invalid-windows-container' for 'NodeWindows'",
		},
		{
			name: "multiple errors",
			inputCR: *NewTridentOrchestrator(
				WithControllerContainer(commonconfig.TridentControllerMain, "invalid-cpu", "invalid-memory", "", ""),
				WithNodeLinuxContainer("invalid-container", "100m", "200Mi", "", ""),
			),
			expectedError: true,
			errorSubstring: "invalid CPU request for Controller container 'trident-main'" +
				": quantities must match the regular expression '^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$'; invalid Memory request for Controller container 'trident-main'" +
				": quantities must match the regular expression '^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$'; invalid container name 'invalid-container' for 'NodeLinux'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fresh installer and default resources for each test
			mockK8sClient := newMockKubeClient(t)
			installer := newTestInstaller(mockK8sClient)
			setupDefaultResources()

			// Execute the function
			err := installer.populateResources(tt.inputCR)

			// Validate error expectations
			if tt.expectedError {
				assert.Error(t, err, "expected error but got none")
				if tt.errorSubstring != "" {
					tempErr := err.Error()
					fmt.Println(tempErr)
					assert.Contains(t, err.Error(), tt.errorSubstring, "error message should contain expected substring")
				}
			} else {
				assert.NoError(t, err, "expected no error but got: %v", err)
			}

			// Run custom validation if provided
			if tt.validateFunc != nil {
				tt.validateFunc(t)
			}
		})
	}
}

func TestInstaller_validateResources(t *testing.T) {
	tests := []struct {
		name            string
		resources       *commonconfig.Resources
		expectedError   bool
		errorSubstrings []string // For multiple error validation
	}{
		{
			name:          "nil resources",
			resources:     nil,
			expectedError: false,
		},
		{
			name:          "empty resources",
			resources:     &commonconfig.Resources{},
			expectedError: false,
		},
		{
			name: "valid controller resources - requests only",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: createCommonConfigContainerResource("100m", "200Mi", "", ""),
				},
			},
			expectedError: false,
		},
		{
			name: "valid controller resources - requests and limits",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: createCommonConfigContainerResource("100m", "200Mi", "500m", "1Gi"),
				},
			},
			expectedError: false,
		},
		{
			name: "valid node linux resources",
			resources: &commonconfig.Resources{
				Node: &commonconfig.NodeResources{
					Linux: map[string]*commonconfig.ContainerResource{
						commonconfig.TridentNodeMain: createCommonConfigContainerResource("150m", "300Mi", "600m", "2Gi"),
					},
				},
			},
			expectedError: false,
		},
		{
			name: "valid node windows resources",
			resources: &commonconfig.Resources{
				Node: &commonconfig.NodeResources{
					Windows: map[string]*commonconfig.ContainerResource{
						commonconfig.TridentNodeMain: createCommonConfigContainerResource("120m", "250Mi", "700m", "1.5Gi"),
					},
				},
			},
			expectedError: false,
		},
		{
			name: "valid multiple containers",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: createCommonConfigContainerResource("100m", "200Mi", "500m", "1Gi"),
					commonconfig.CSISidecarProvisioner: createCommonConfigContainerResource("50m", "100Mi", "200m", "500Mi"),
				},
				Node: &commonconfig.NodeResources{
					Linux: map[string]*commonconfig.ContainerResource{
						commonconfig.TridentNodeMain:     createCommonConfigContainerResource("150m", "300Mi", "600m", "2Gi"),
						commonconfig.CSISidecarRegistrar: createCommonConfigContainerResource("25m", "50Mi", "100m", "200Mi"),
					},
					Windows: map[string]*commonconfig.ContainerResource{
						commonconfig.TridentNodeMain:     createCommonConfigContainerResource("120m", "250Mi", "700m", "1.5Gi"),
						commonconfig.CSISidecarRegistrar: createCommonConfigContainerResource("30m", "60Mi", "150m", "300Mi"),
					},
				},
			},
			expectedError: false,
		},
		{
			name: "negative CPU request in controller",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: {
						Requests: &commonconfig.ResourceRequirements{
							CPU:    createNegativeQuantity("100m"),
							Memory: mustParseQuantityPtr("200Mi"),
						},
					},
				},
			},
			expectedError:   true,
			errorSubstrings: []string{"invalid CPU request for container \"trident-main\" in Controller"},
		},
		{
			name: "negative memory request in controller",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: {
						Requests: &commonconfig.ResourceRequirements{
							CPU:    mustParseQuantityPtr("100m"),
							Memory: createNegativeQuantity("200Mi"),
						},
					},
				},
			},
			expectedError:   true,
			errorSubstrings: []string{"invalid memory request for container \"trident-main\" in Controller"},
		},
		{
			name: "negative CPU limit in controller",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: {
						Limits: &commonconfig.ResourceRequirements{
							CPU:    createNegativeQuantity("500m"),
							Memory: mustParseQuantityPtr("1Gi"),
						},
						Requests: &commonconfig.ResourceRequirements{
							CPU:    mustParseQuantityPtr("100m"),
							Memory: mustParseQuantityPtr("200Mi"),
						},
					},
				},
			},
			expectedError:   true,
			errorSubstrings: []string{"invalid CPU limit for container \"trident-main\" in Controller"},
		},
		{
			name: "negative memory limit in controller",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: {
						Limits: &commonconfig.ResourceRequirements{
							CPU:    mustParseQuantityPtr("500m"),
							Memory: createNegativeQuantity("1Gi"),
						},
						Requests: &commonconfig.ResourceRequirements{
							CPU:    mustParseQuantityPtr("100m"),
							Memory: mustParseQuantityPtr("200Mi"),
						},
					},
				},
			},
			expectedError:   true,
			errorSubstrings: []string{"invalid memory limit for container \"trident-main\" in Controller"},
		},
		{
			name: "CPU request exceeds limit",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: createCommonConfigContainerResource("500m", "200Mi", "100m", "1Gi"),
				},
			},
			expectedError:   true,
			errorSubstrings: []string{"CPU request 500m exceeds limit 100m for container \"trident-main\" in Controller"},
		},
		{
			name: "memory request exceeds limit",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: createCommonConfigContainerResource("100m", "2Gi", "500m", "1Gi"),
				},
			},
			expectedError:   true,
			errorSubstrings: []string{"memory request 2Gi exceeds limit 1Gi for container \"trident-main\" in Controller"},
		},
		{
			name: "negative resources in node linux",
			resources: &commonconfig.Resources{
				Node: &commonconfig.NodeResources{
					Linux: map[string]*commonconfig.ContainerResource{
						commonconfig.TridentNodeMain: {
							Requests: &commonconfig.ResourceRequirements{
								CPU:    createNegativeQuantity("150m"),
								Memory: mustParseQuantityPtr("300Mi"),
							},
						},
					},
				},
			},
			expectedError:   true,
			errorSubstrings: []string{"invalid CPU request for container \"trident-main\" in NodeLinux"},
		},
		{
			name: "negative resources in node windows",
			resources: &commonconfig.Resources{
				Node: &commonconfig.NodeResources{
					Windows: map[string]*commonconfig.ContainerResource{
						commonconfig.TridentNodeMain: {
							Limits: &commonconfig.ResourceRequirements{
								CPU:    mustParseQuantityPtr("700m"),
								Memory: createNegativeQuantity("1.5Gi"),
							},
							Requests: &commonconfig.ResourceRequirements{
								CPU:    mustParseQuantityPtr("700m"),
								Memory: mustParseQuantityPtr("1.5Gi"),
							},
						},
					},
				},
			},
			expectedError:   true,
			errorSubstrings: []string{"invalid memory limit for container \"trident-main\" in NodeWindows"},
		},
		{
			name: "request exceeds limit in node linux",
			resources: &commonconfig.Resources{
				Node: &commonconfig.NodeResources{
					Linux: map[string]*commonconfig.ContainerResource{
						commonconfig.TridentNodeMain: createCommonConfigContainerResource("600m", "300Mi", "150m", "2Gi"),
					},
				},
			},
			expectedError:   true,
			errorSubstrings: []string{"CPU request 600m exceeds limit 150m for container \"trident-main\" in NodeLinux"},
		},
		{
			name: "request exceeds limit in node windows",
			resources: &commonconfig.Resources{
				Node: &commonconfig.NodeResources{
					Windows: map[string]*commonconfig.ContainerResource{
						commonconfig.TridentNodeMain: createCommonConfigContainerResource("120m", "2Gi", "700m", "1.5Gi"),
					},
				},
			},
			expectedError:   true,
			errorSubstrings: []string{"memory request 2Gi exceeds limit 1536Mi for container \"trident-main\" in NodeWindows"},
		},
		{
			name: "zero resources are valid",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: createCommonConfigContainerResource("0", "0", "0", "0"),
				},
			},
			expectedError: false,
		},
		{
			name: "equal requests and limits are valid",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: createCommonConfigContainerResource("500m", "1Gi", "500m", "1Gi"),
				},
			},
			expectedError: false,
		},
		{
			name: "nil container resource",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: nil,
				},
			},
			expectedError: false,
		},
		{
			name: "mixed valid and invalid containers",
			resources: &commonconfig.Resources{
				Controller: map[string]*commonconfig.ContainerResource{
					commonconfig.TridentControllerMain: createCommonConfigContainerResource("100m", "200Mi", "500m", "1Gi"),
					commonconfig.CSISidecarProvisioner: {
						Requests: &commonconfig.ResourceRequirements{
							CPU:    createNegativeQuantity("50m"),
							Memory: mustParseQuantityPtr("100Mi"),
						},
					},
				},
			},
			expectedError:   true,
			errorSubstrings: []string{"invalid CPU request for container \"csi-provisioner\" in Controller"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockK8sClient := newMockKubeClient(t)
			installer := newTestInstaller(mockK8sClient)

			err := installer.validateResources(test.resources)

			if test.expectedError {
				assert.Error(t, err, "Expected error for test case: %s", test.name)
				if len(test.errorSubstrings) > 0 {
					for _, substr := range test.errorSubstrings {
						assert.Contains(t, err.Error(), substr, "Error should contain substring: %s", substr)
					}
				}
			} else {
				assert.NoError(t, err, "Expected no error for test case: %s", test.name)
			}
		})
	}
}

// ------------------------- Helper functions -------------------------

// TridentOrchestratorBuilder provides a fluent interface for building TridentOrchestrator objects
type TridentOrchestratorBuilder struct {
	to *netappv1.TridentOrchestrator
}

// TridentOrchestratorOption is a function that modifies the TridentOrchestrator
type TridentOrchestratorOption func(*TridentOrchestratorBuilder)

// NewTridentOrchestrator creates a new TridentOrchestratorBuilder with default values
func NewTridentOrchestrator(opts ...TridentOrchestratorOption) *netappv1.TridentOrchestrator {
	builder := &TridentOrchestratorBuilder{
		to: &netappv1.TridentOrchestrator{
			TypeMeta:   metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{},
			Spec:       netappv1.TridentOrchestratorSpec{},
			Status:     netappv1.TridentOrchestratorStatus{},
		},
	}

	for _, opt := range opts {
		opt(builder)
	}

	return builder.to
}

// WithControllerContainer adds a controller container with specified resources
func WithControllerContainer(containerName, cpuReq, memReq, cpuLimit, memLimit string) TridentOrchestratorOption {
	return func(b *TridentOrchestratorBuilder) {
		if b.to.Spec.Resources == nil {
			b.to.Spec.Resources = &netappv1.Resources{}
		}
		if b.to.Spec.Resources.Controller == nil {
			b.to.Spec.Resources.Controller = make(netappv1.ContainersResourceRequirements)
		}
		b.to.Spec.Resources.Controller[containerName] = createContainerResource(cpuReq, memReq, cpuLimit, memLimit)
	}
}

// WithNodeLinuxContainer adds a node Linux container with specified resources
func WithNodeLinuxContainer(containerName, cpuReq, memReq, cpuLimit, memLimit string) TridentOrchestratorOption {
	return func(b *TridentOrchestratorBuilder) {
		if b.to.Spec.Resources == nil {
			b.to.Spec.Resources = &netappv1.Resources{}
		}
		if b.to.Spec.Resources.Node == nil {
			b.to.Spec.Resources.Node = &netappv1.NodeResources{}
		}
		if b.to.Spec.Resources.Node.Linux == nil {
			b.to.Spec.Resources.Node.Linux = make(netappv1.ContainersResourceRequirements)
		}
		b.to.Spec.Resources.Node.Linux[containerName] = createContainerResource(cpuReq, memReq, cpuLimit, memLimit)
	}
}

// WithNodeWindowsContainer adds a node Windows container with specified resources
func WithNodeWindowsContainer(containerName, cpuReq, memReq, cpuLimit, memLimit string) TridentOrchestratorOption {
	return func(b *TridentOrchestratorBuilder) {
		if b.to.Spec.Resources == nil {
			b.to.Spec.Resources = &netappv1.Resources{}
		}
		if b.to.Spec.Resources.Node == nil {
			b.to.Spec.Resources.Node = &netappv1.NodeResources{}
		}
		if b.to.Spec.Resources.Node.Windows == nil {
			b.to.Spec.Resources.Node.Windows = make(netappv1.ContainersResourceRequirements)
		}
		b.to.Spec.Resources.Node.Windows[containerName] = createContainerResource(cpuReq, memReq, cpuLimit, memLimit)
	}
}

// WithImageRegistry sets the image registry
func WithImageRegistry(registry string) TridentOrchestratorOption {
	return func(b *TridentOrchestratorBuilder) {
		b.to.Spec.ImageRegistry = registry
	}
}

// WithDebug sets the debug flag
func WithDebug(debug bool) TridentOrchestratorOption {
	return func(b *TridentOrchestratorBuilder) {
		b.to.Spec.Debug = debug
	}
}

// WithLogLevel sets the log level
func WithLogLevel(logLevel string) TridentOrchestratorOption {
	return func(b *TridentOrchestratorBuilder) {
		b.to.Spec.LogLevel = logLevel
	}
}

// WithNamespace sets the namespace
func WithNamespace(namespace string) TridentOrchestratorOption {
	return func(b *TridentOrchestratorBuilder) {
		b.to.Spec.Namespace = namespace
	}
}

// WithFSGroupPolicy sets the FSGroupPolicy
func WithFSGroupPolicy(policy string) TridentOrchestratorOption {
	return func(b *TridentOrchestratorBuilder) {
		b.to.Spec.FSGroupPolicy = policy
	}
}

// WithNodePrep sets the node preparation protocols
func WithNodePrep(protocols []string) TridentOrchestratorOption {
	return func(b *TridentOrchestratorBuilder) {
		b.to.Spec.NodePrep = protocols
	}
}

// WithIPv6 sets the IPv6 flag
func WithIPv6(ipv6 bool) TridentOrchestratorOption {
	return func(b *TridentOrchestratorBuilder) {
		b.to.Spec.IPv6 = ipv6
	}
}

// WithWindows sets the Windows flag
func WithWindows(windows bool) TridentOrchestratorOption {
	return func(b *TridentOrchestratorBuilder) {
		b.to.Spec.Windows = windows
	}
}

// Helper function to create resource quantity
func mustParseQuantity(value string) resource.Quantity {
	qty, err := resource.ParseQuantity(value)
	if err != nil {
		panic(fmt.Sprintf("failed to parse quantity %s: %v", value, err))
	}
	return qty
}

// Helper function to create pointer to resource quantity
func mustParseQuantityPtr(value string) *resource.Quantity {
	qty := mustParseQuantity(value)
	return &qty
}

// Helper function to create container resource specification
func createContainerResource(cpuReq, memReq, cpuLimit, memLimit string) *netappv1.ContainerResource {
	cr := &netappv1.ContainerResource{}

	if cpuReq != "" || memReq != "" {
		cr.Requests = &netappv1.ResourceRequirements{}
		if cpuReq != "" {
			cr.Requests.CPU = cpuReq
		}
		if memReq != "" {
			cr.Requests.Memory = memReq
		}
	}

	if cpuLimit != "" || memLimit != "" {
		cr.Limits = &netappv1.ResourceRequirements{}
		if cpuLimit != "" {
			cr.Limits.CPU = cpuLimit
		}
		if memLimit != "" {
			cr.Limits.Memory = memLimit
		}
	}

	return cr
}

// Helper function to create commonconfig.ContainerResource
func createCommonConfigContainerResource(cpuReq, memReq, cpuLimit, memLimit string) *commonconfig.ContainerResource {
	cr := &commonconfig.ContainerResource{}

	if cpuReq != "" || memReq != "" {
		cr.Requests = &commonconfig.ResourceRequirements{}
		if cpuReq != "" {
			cr.Requests.CPU = mustParseQuantityPtr(cpuReq)
		}
		if memReq != "" {
			cr.Requests.Memory = mustParseQuantityPtr(memReq)
		}
	}

	if cpuLimit != "" || memLimit != "" {
		cr.Limits = &commonconfig.ResourceRequirements{}
		if cpuLimit != "" {
			cr.Limits.CPU = mustParseQuantityPtr(cpuLimit)
		}
		if memLimit != "" {
			cr.Limits.Memory = mustParseQuantityPtr(memLimit)
		}
	}

	return cr
}

// Helper function to create negative quantity
func createNegativeQuantity(value string) *resource.Quantity {
	qty := mustParseQuantity(value)
	negQty := qty.DeepCopy()
	negQty.Neg()
	return &negQty
}
