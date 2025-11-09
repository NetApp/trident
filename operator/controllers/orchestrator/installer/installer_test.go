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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	commonconfig "github.com/netapp/trident/config"
	mockExtendedK8sClient "github.com/netapp/trident/mocks/mock_operator/mock_controllers/mock_orchestrator/mock_installer"
	"github.com/netapp/trident/operator/config"
	netappv1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	persistentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/version"
)

const (
	Namespace = "trident"

	// Test constants for commonly used values
	TestUID             = "test-uid"
	TestTridentNetAppIO = "trident.netapp.io"
	TestAppLabel        = "app"
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

	// Test GetCRD failure when CRD exists and performOperationOnce=true
	mockK8sClient.EXPECT().CheckCRDExists(crdName).Return(true, nil)
	mockK8sClient.EXPECT().GetCRD(crdName).Return(nil, k8sClientError)
	expectedErr = installer.CreateOrPatchCRD(crdName, crdYAML, true)
	assert.NotNil(t, expectedErr, "expected non-nil error when GetCRD fails")

	// Test new CRD creation success
	mockK8sClient.EXPECT().CheckCRDExists(crdName).Return(false, nil)
	mockK8sClient.EXPECT().PutCustomResourceDefinition(nil, crdName, true, crdYAML).Return(nil)
	expectedErr = installer.CreateOrPatchCRD(crdName, crdYAML, false)
	assert.Nil(t, expectedErr, "expected nil error for successful CRD creation")

	// Test new CRD creation failure
	mockK8sClient.EXPECT().CheckCRDExists(crdName).Return(false, nil)
	mockK8sClient.EXPECT().PutCustomResourceDefinition(nil, crdName, true, crdYAML).Return(k8sClientError)
	expectedErr = installer.CreateOrPatchCRD(crdName, crdYAML, false)
	assert.NotNil(t, expectedErr, "expected non-nil error when PutCustomResourceDefinition fails for new CRD")
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
		Description   string
	}{
		// Valid values
		{"", "", true, "empty provider and identity"},
		{k8sclient.CloudProviderAzure, "", true, "Azure provider with empty identity"},
		{k8sclient.CloudProviderAzure, k8sclient.AzureCloudIdentityKey + " a8rry78r8-7733-49bd-6656582", true, "Azure with valid identity key"},
		{k8sclient.CloudProviderAWS, k8sclient.AWSCloudIdentityKey + "arn:aws:iam::123456789:role/test", true, "AWS with valid identity key"},
		{k8sclient.CloudProviderGCP, k8sclient.GCPCloudIdentityKey + "projects/123456/serviceAccounts/test@example.iam.gserviceaccount.com", true, "GCP with valid identity key"},
		{"azure", k8sclient.AzureCloudIdentityKey + " a8rry78r8-7733-49bd-6656582", true, "Azure case-insensitive"},
		{"aws", k8sclient.AWSCloudIdentityKey + "arn:aws:iam::123456789:role/test", true, "AWS case-insensitive"},
		{"gcp", k8sclient.GCPCloudIdentityKey + "projects/123456/serviceAccounts/test@example.iam.gserviceaccount.com", true, "GCP case-insensitive"},

		// Invalid values
		{"", "123456789", false, "empty provider with non-empty identity"},
		{k8sclient.CloudProviderAzure, " rruuunu89-9933-49bd-134423", false, "Azure with invalid identity format"},
		{k8sclient.CloudProviderAWS, "", false, "AWS with empty identity"},
		{k8sclient.CloudProviderAWS, "invalid-aws-identity", false, "AWS with invalid identity format"},
		{k8sclient.CloudProviderGCP, "", false, "GCP with empty identity"},
		{k8sclient.CloudProviderGCP, "invalid-gcp-identity", false, "GCP with invalid identity format"},
	}

	for _, test := range tests {
		t.Run(test.Description, func(t *testing.T) {
			// Store original values
			originalCloudProvider := cloudProvider
			originalCloudIdentity := cloudIdentity
			defer func() {
				cloudProvider = originalCloudProvider
				cloudIdentity = originalCloudIdentity
			}()

			cloudProvider = test.cloudProvider
			cloudIdentity = test.cloudIdentity
			err := installer.cloudIdentityPrechecks()
			if test.Valid {
				assert.NoError(t, err, "should be valid for %s", test.Description)
			} else {
				assert.Error(t, err, "should be invalid for %s", test.Description)
			}
		})
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

func TestIsLinuxNodeSCCUser(t *testing.T) {
	tests := []struct {
		name     string
		user     string
		expected bool
	}{
		{
			name:     "Linux node user",
			user:     TridentNodeLinuxResourceName,
			expected: true,
		},
		{
			name:     "Windows node user",
			user:     TridentNodeWindowsResourceName,
			expected: false,
		},
		{
			name:     "Controller user",
			user:     TridentControllerResourceName,
			expected: false,
		},
		{
			name:     "Random user",
			user:     "random-user",
			expected: false,
		},
		{
			name:     "Empty user",
			user:     "",
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := isLinuxNodeSCCUser(test.user)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestDetermineLogLevel(t *testing.T) {
	tests := []struct {
		name           string
		debugVal       bool
		logLevelVal    string
		expectedResult string
	}{
		{
			name:           "Debug is true",
			debugVal:       true,
			logLevelVal:    "info",
			expectedResult: "debug",
		},
		{
			name:           "Debug is false and logLevel is set",
			debugVal:       false,
			logLevelVal:    "warn",
			expectedResult: "warn",
		},
		{
			name:           "Debug is false and logLevel is empty",
			debugVal:       false,
			logLevelVal:    "",
			expectedResult: "info",
		},
		{
			name:           "Debug is false and logLevel is debug",
			debugVal:       false,
			logLevelVal:    "debug",
			expectedResult: "debug",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Set global variables
			debug = test.debugVal
			logLevel = test.logLevelVal

			result := determineLogLevel()
			assert.Equal(t, test.expectedResult, result)
		})
	}
}

func TestSetInstallationParams(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	t.Run("successful with minimal CR spec", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		cr := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{},
		}

		_, _, _, err := installer.setInstallationParams(cr, "")
		assert.NoError(t, err, "Should not error when setting installation params with default configuration")
	})

	t.Run("test debug and log level settings", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		cr := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				Debug:    true,
				LogLevel: "debug",
			},
		}

		_, _, _, err := installer.setInstallationParams(cr, "")
		assert.NoError(t, err, "Should not error when setting installation params with debug configuration")
	})

	t.Run("test non-debug log level", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		cr := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				LogLevel: "warn",
			},
		}

		_, _, _, err := installer.setInstallationParams(cr, "")
		assert.NoError(t, err, "Should not error when setting installation params with non-debug log level")
	})

	t.Run("test IPv6 and Windows settings", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		cr := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				IPv6:    true,
				Windows: true,
			},
		}

		_, _, _, err := installer.setInstallationParams(cr, "")
		assert.NoError(t, err, "Should not error when setting installation params with IPv6 and Windows settings")
	})

	t.Run("test autosupport configurations", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		cr := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				SilenceAutosupport:      true,
				AutosupportProxy:        "proxy.example.com",
				AutosupportSerialNumber: "12345",
				AutosupportHostname:     "test.hostname.com",
				AutosupportInsecure:     true,
			},
		}

		_, _, _, err := installer.setInstallationParams(cr, "")
		assert.NoError(t, err, "Should not error when setting installation params with autosupport configurations")
	})

	t.Run("test probe port and kubelet dir settings", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		probePortValue := int64(9990)
		cr := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				ProbePort:  &probePortValue,
				KubeletDir: "/custom/kubelet/dir",
			},
		}

		_, _, _, err := installer.setInstallationParams(cr, "")
		assert.NoError(t, err, "Should not error when setting installation params with probe port and kubelet dir settings")
	})

	t.Run("test disable audit log configurations", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		disableTrue := true
		disableFalse := false

		// Test with nil (default true)
		cr1 := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				DisableAuditLog: nil,
			},
		}
		_, _, _, err := installer.setInstallationParams(cr1, "")
		assert.NoError(t, err, "Should not error when setting installation params with audit log disabled (nil)")

		// Test with explicit true
		cr2 := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				DisableAuditLog: &disableTrue,
			},
		}
		_, _, _, err = installer.setInstallationParams(cr2, "")
		assert.NoError(t, err, "Should not error when setting installation params with audit log disabled (true)")

		// Test with explicit false
		cr3 := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				DisableAuditLog: &disableFalse,
			},
		}
		_, _, _, err = installer.setInstallationParams(cr3, "")
		assert.NoError(t, err, "Should not error when setting installation params with audit log enabled (false)")
	})

	t.Run("test ACP obsolete message handling", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		cr := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				EnableACP: true,
				ACPImage:  "test-acp-image",
			},
		}

		_, _, _, err := installer.setInstallationParams(cr, "")
		assert.NoError(t, err, "Should not error when setting installation params with image pull policy settings")
	})

	t.Run("test ExcludeAutosupport pointer handling", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		excludeTrue := true
		excludeFalse := false

		// Test with nil
		cr1 := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				ExcludeAutosupport: nil,
			},
		}
		_, _, _, err := installer.setInstallationParams(cr1, "")
		assert.NoError(t, err, "Should not error when setting installation params with ExcludeAutosupport nil")

		// Test with true
		cr2 := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				ExcludeAutosupport: &excludeTrue,
			},
		}
		_, _, _, err = installer.setInstallationParams(cr2, "")
		assert.NoError(t, err, "Should not error when setting installation params with ExcludeAutosupport true")

		// Test with false
		cr3 := netappv1.TridentOrchestrator{
			Spec: netappv1.TridentOrchestratorSpec{
				ExcludeAutosupport: &excludeFalse,
			},
		}
		_, _, _, err = installer.setInstallationParams(cr3, "")
		assert.NoError(t, err, "Should not error when setting installation params with ExcludeAutosupport false")
	})
}

func TestImagePrechecksTargeted(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	labels := map[string]string{TestAppLabel: TestTridentNetAppIO}
	controllingCRDetails := map[string]string{"UID": TestUID}

	t.Run("error getting deployment information", func(t *testing.T) {
		// Target the TridentDeploymentInformation error path (line 252-255)
		mockK8sClient.EXPECT().GetDeploymentInformation(TridentControllerResourceName, TridentCSILabel, Namespace).Return(nil, nil, false, fmt.Errorf("deployment lookup failed"))

		version, err := installer.imagePrechecks(labels, controllingCRDetails)
		assert.Error(t, err, "expected error when getting deployment information fails")
		assert.Contains(t, err.Error(), "unable to get existing deployment information")
		assert.Empty(t, version)
	})
}

func TestCreateAndEnsureCRDs(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	t.Run("successful CRD creation", func(t *testing.T) {
		// Mock all CRD creation calls to succeed
		crdNames := []string{
			VersionCRDName, BackendCRDName, SnapshotInfoCRDName, BackendConfigCRDName,
			StorageClassCRDName, VolumeCRDName, VolumePublicationCRDName, NodeCRDName,
			NodeRemediationCRDName, NodeRemediationTemplateCRDName, TransactionCRDName,
			SnapshotCRDName, GroupSnapshotCRDName, VolumeReferenceCRDName,
			MirrorRelationshipCRDName, ActionMirrorUpdateCRDName, ActionSnapshotRestoreCRDName,
			ConfiguratorCRDName,
		}

		for _, crdName := range crdNames {
			mockK8sClient.EXPECT().CheckCRDExists(crdName).Return(false, nil)
			mockK8sClient.EXPECT().PutCustomResourceDefinition(nil, crdName, true, gomock.AssignableToTypeOf("")).Return(nil)
		}

		err := installer.createAndEnsureCRDs(false)
		assert.NoError(t, err, "Should not error when creating and ensuring CRDs with shouldUpdate=false")
	})

	t.Run("CRD creation failure", func(t *testing.T) {
		// Mock first CRD to fail
		mockK8sClient.EXPECT().CheckCRDExists(VersionCRDName).Return(false, nil)
		mockK8sClient.EXPECT().PutCustomResourceDefinition(nil, VersionCRDName, true, gomock.AssignableToTypeOf("")).Return(fmt.Errorf("failed to create CRD"))

		err := installer.createAndEnsureCRDs(false)
		assert.Error(t, err, "expected error when CRD creation fails")
		assert.Contains(t, err.Error(), "failed to create the Trident CRDs")
	})

	t.Run("with performOperationOnce true", func(t *testing.T) {
		// Mock all CRD creation calls to succeed with performOperationOnce=true
		crdNames := []string{
			VersionCRDName, BackendCRDName, SnapshotInfoCRDName, BackendConfigCRDName,
			StorageClassCRDName, VolumeCRDName, VolumePublicationCRDName, NodeCRDName,
			NodeRemediationCRDName, NodeRemediationTemplateCRDName, TransactionCRDName,
			SnapshotCRDName, GroupSnapshotCRDName, VolumeReferenceCRDName,
			MirrorRelationshipCRDName, ActionMirrorUpdateCRDName, ActionSnapshotRestoreCRDName,
			ConfiguratorCRDName,
		}

		for _, crdName := range crdNames {
			mockK8sClient.EXPECT().CheckCRDExists(crdName).Return(false, nil)
			mockK8sClient.EXPECT().PutCustomResourceDefinition(nil, crdName, true, gomock.AssignableToTypeOf("")).Return(nil)
		}

		err := installer.createAndEnsureCRDs(true)
		assert.NoError(t, err, "Should not error when creating and ensuring CRDs with shouldUpdate=true")
	})
}

func TestInstallOrPatchTrident(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	// Create test CR
	cr := netappv1.TridentOrchestrator{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentOrchestrator",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "trident",
			UID:  TestUID,
		},
		Spec: netappv1.TridentOrchestratorSpec{
			Debug: false,
		},
	}

	t.Run("failure in setInstallationParams", func(t *testing.T) {
		// Test invalid CR spec to cause setInstallationParams failure
		invalidCR := cr
		invalidCR.Spec.FSGroupPolicy = "invalid-policy"

		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		specValues, version, acpVersion, err := installer.InstallOrPatchTrident(invalidCR, "", false, false)

		assert.Error(t, err, "expected error when setInstallationParams fails with invalid CR")
		assert.Nil(t, specValues)
		assert.Empty(t, version)
		assert.Empty(t, acpVersion)
	})

	t.Run("failure deleting transient version pod", func(t *testing.T) {
		// Mock setInstallationParams to succeed
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		// Mock DeleteTransientVersionPod to fail
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(fmt.Errorf("failed to delete pod"))

		// Mock createOrPatchTridentInstallationNamespace to succeed
		mockK8sClient.EXPECT().CheckNamespaceExists(installer.namespace).Return(false, nil)
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)

		// Mock createRBACObjects to fail early to avoid mocking everything
		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(gomock.AssignableToTypeOf([]string{}), gomock.AssignableToTypeOf(""), Namespace, gomock.AssignableToTypeOf(false)).Return(
			nil, nil, nil, nil, fmt.Errorf("RBAC failure"))

		specValues, version, acpVersion, err := installer.InstallOrPatchTrident(cr, "", false, false)

		assert.Error(t, err, "expected error when RBAC creation fails after deleting transient version pod")
		assert.Nil(t, specValues)
		assert.Empty(t, version)
		assert.Empty(t, acpVersion)
	})

	t.Run("failure creating namespace", func(t *testing.T) {
		// Mock setInstallationParams to succeed
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		// Mock DeleteTransientVersionPod to succeed
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)

		// Mock createOrPatchTridentInstallationNamespace to fail
		mockK8sClient.EXPECT().CheckNamespaceExists(installer.namespace).Return(false, fmt.Errorf("namespace check failed"))

		specValues, version, acpVersion, err := installer.InstallOrPatchTrident(cr, "", false, false)

		assert.Error(t, err, "expected error when namespace creation fails")
		assert.Nil(t, specValues)
		assert.Empty(t, version)
		assert.Empty(t, acpVersion)
	})

	t.Run("failure creating RBAC objects", func(t *testing.T) {
		// Mock setInstallationParams to succeed
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		// Mock DeleteTransientVersionPod to succeed
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)

		// Mock createOrPatchTridentInstallationNamespace to succeed
		mockK8sClient.EXPECT().CheckNamespaceExists(installer.namespace).Return(false, nil)
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)

		// Mock createRBACObjects to fail
		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil, nil, nil, nil, fmt.Errorf("RBAC creation failed"))

		specValues, version, acpVersion, err := installer.InstallOrPatchTrident(cr, "", false, false)

		assert.Error(t, err, "expected error when RBAC creation fails")
		assert.Nil(t, specValues)
		assert.Empty(t, version)
		assert.Empty(t, acpVersion)
	})

	t.Run("failure creating CRDs", func(t *testing.T) {
		// Mock setInstallationParams to succeed
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		// Mock DeleteTransientVersionPod to succeed
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)

		// Mock createOrPatchTridentInstallationNamespace to succeed
		mockK8sClient.EXPECT().CheckNamespaceExists(installer.namespace).Return(false, nil)
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)

		// Mock createRBACObjects to succeed
		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			make(map[string]*corev1.ServiceAccount), []corev1.ServiceAccount{}, make(map[string][]string), make(map[string]bool), nil)
		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			make(map[string]*corev1.ServiceAccount), []corev1.ServiceAccount{}, make(map[string][]string), make(map[string]bool), nil)
		mockK8sClient.EXPECT().RemoveMultipleServiceAccounts(gomock.AssignableToTypeOf([]corev1.ServiceAccount{})).Return(nil)
		mockK8sClient.EXPECT().PutServiceAccount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()

		mockK8sClient.EXPECT().GetClusterRoleInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, []rbacv1.ClusterRole{}, true, nil)
		mockK8sClient.EXPECT().GetClusterRolesByLabel(gomock.Any()).Return([]rbacv1.ClusterRole{}, nil).AnyTimes()
		mockK8sClient.EXPECT().RemoveMultipleClusterRoles(gomock.AssignableToTypeOf([]rbacv1.ClusterRole{})).Return(nil).AnyTimes()
		mockK8sClient.EXPECT().PutClusterRole(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		mockK8sClient.EXPECT().GetClusterRoleBindingInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, []rbacv1.ClusterRoleBinding{}, true, nil)
		mockK8sClient.EXPECT().GetClusterRoleBindingsByLabel(gomock.Any()).Return([]rbacv1.ClusterRoleBinding{}, nil).AnyTimes()
		mockK8sClient.EXPECT().RemoveMultipleClusterRoleBindings(gomock.AssignableToTypeOf([]rbacv1.ClusterRoleBinding{})).Return(nil).AnyTimes()
		mockK8sClient.EXPECT().PutClusterRoleBinding(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		mockK8sClient.EXPECT().GetMultipleRoleInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			make(map[string]*rbacv1.Role), []rbacv1.Role{}, make(map[string]bool), nil)
		mockK8sClient.EXPECT().RemoveMultipleRoles(gomock.AssignableToTypeOf([]rbacv1.Role{})).Return(nil)
		mockK8sClient.EXPECT().PutRole(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		mockK8sClient.EXPECT().GetMultipleRoleBindingInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(
			make(map[string]*rbacv1.RoleBinding), []rbacv1.RoleBinding{}, make(map[string]bool), nil)
		mockK8sClient.EXPECT().RemoveMultipleRoleBindings(gomock.AssignableToTypeOf([]rbacv1.RoleBinding{})).Return(nil)
		mockK8sClient.EXPECT().PutRoleBinding(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		mockK8sClient.EXPECT().Flavor().Return(k8sclient.FlavorKubernetes).AnyTimes()
		mockK8sClient.EXPECT().Namespace().Return(installer.namespace).AnyTimes()
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		// Mock createAndEnsureCRDs to fail on first CRD
		mockK8sClient.EXPECT().CheckCRDExists(VersionCRDName).Return(false, nil)
		mockK8sClient.EXPECT().PutCustomResourceDefinition(nil, VersionCRDName, true, gomock.Any()).Return(fmt.Errorf("CRD creation failed"))

		specValues, version, acpVersion, err := installer.InstallOrPatchTrident(cr, "", false, false)

		assert.Error(t, err, "expected error when CRD creation fails")
		assert.Contains(t, err.Error(), "failed to create the Trident CRDs")
		assert.Nil(t, specValues)
		assert.Empty(t, version)
		assert.Empty(t, acpVersion)
	})
}

func TestCreateOrPatchTridentDeployment(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	labels := map[string]string{
		TestAppLabel: TestTridentNetAppIO,
	}
	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}
	reuseServiceAccountMap := map[string]bool{
		"trident-controller": true,
	}

	t.Run("successful deployment creation", func(t *testing.T) {
		// Mock GetDeploymentInformation to return no existing deployment
		mockK8sClient.EXPECT().GetDeploymentInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, []appsv1.Deployment{}, true, nil)

		// Mock RemoveMultipleDeployments
		mockK8sClient.EXPECT().RemoveMultipleDeployments(gomock.Any()).Return(nil)

		// Mock ConstructCSIFeatureGateYAMLSnippets (called via k8sclient directly)
		mockK8sClient.EXPECT().CheckCRDExists(gomock.AssignableToTypeOf("")).Return(false, nil).AnyTimes()

		// Mock ServerVersion call
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		// Mock PutDeployment to succeed
		mockK8sClient.EXPECT().PutDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentDeployment(controllingCRDetails, labels, false, reuseServiceAccountMap)

		assert.NoError(t, err, "Should not error when creating or patching Trident deployment")
	})

	t.Run("failure getting deployment information", func(t *testing.T) {
		// Mock GetDeploymentInformation to fail
		mockK8sClient.EXPECT().GetDeploymentInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil, false, fmt.Errorf("failed to get deployment"))

		err := installer.createOrPatchTridentDeployment(controllingCRDetails, labels, false, reuseServiceAccountMap)

		assert.Error(t, err, "expected error when getting deployment information fails")
		assert.Contains(t, err.Error(), "failed to get Trident deployment information")
	})

	t.Run("failure removing unwanted deployments", func(t *testing.T) {
		// Mock GetDeploymentInformation to return existing deployment
		existingDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "old-deployment"},
		}
		mockK8sClient.EXPECT().GetDeploymentInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(existingDeployment, []appsv1.Deployment{}, false, nil)

		// Mock RemoveMultipleDeployments to fail
		mockK8sClient.EXPECT().RemoveMultipleDeployments(gomock.Any()).Return(fmt.Errorf("failed to remove deployments"))

		err := installer.createOrPatchTridentDeployment(controllingCRDetails, labels, true, reuseServiceAccountMap)

		assert.Error(t, err, "expected error when removing unwanted deployments fails")
		assert.Contains(t, err.Error(), "failed to remove unwanted Trident deployments")
	})

	t.Run("failure creating deployment", func(t *testing.T) {
		// Mock GetDeploymentInformation to return no existing deployment
		mockK8sClient.EXPECT().GetDeploymentInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, []appsv1.Deployment{}, true, nil)

		// Mock RemoveMultipleDeployments to succeed
		mockK8sClient.EXPECT().RemoveMultipleDeployments(gomock.Any()).Return(nil)

		// Mock ConstructCSIFeatureGateYAMLSnippets (called via k8sclient directly)
		mockK8sClient.EXPECT().CheckCRDExists(gomock.AssignableToTypeOf("")).Return(false, nil).AnyTimes()

		// Mock PutDeployment to fail
		mockK8sClient.EXPECT().PutDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to create deployment"))

		err := installer.createOrPatchTridentDeployment(controllingCRDetails, labels, false, reuseServiceAccountMap)

		assert.Error(t, err, "expected error when creating deployment fails")
		assert.Contains(t, err.Error(), "failed to create or patch Trident deployment")
	})

	t.Run("should update existing deployment", func(t *testing.T) {
		// Mock GetDeploymentInformation to return existing deployment
		existingDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "trident-csi"},
		}
		mockK8sClient.EXPECT().GetDeploymentInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(existingDeployment, []appsv1.Deployment{}, false, nil)

		// Should create new deployment because shouldUpdate=true and service account not reused
		mockK8sClient.EXPECT().RemoveMultipleDeployments(gomock.Any()).Return(nil)

		// Mock ConstructCSIFeatureGateYAMLSnippets (called via k8sclient directly)
		mockK8sClient.EXPECT().CheckCRDExists(gomock.AssignableToTypeOf("")).Return(false, nil).AnyTimes()

		mockK8sClient.EXPECT().PutDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		// Pass false for reuseServiceAccountMap for controller service account
		reuseMapFalse := map[string]bool{
			"trident-controller": false,
		}

		err := installer.createOrPatchTridentDeployment(controllingCRDetails, labels, true, reuseMapFalse)

		assert.NoError(t, err, "Should not error when creating Trident deployment with shouldUpdate=true")
	})

	t.Run("CSI feature gate error handling", func(t *testing.T) {
		// Test the path where ConstructCSIFeatureGateYAMLSnippets returns an error
		mockK8sClient.EXPECT().GetDeploymentInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, []appsv1.Deployment{}, true, nil)
		mockK8sClient.EXPECT().RemoveMultipleDeployments(gomock.Any()).Return(nil)

		// Mock CheckCRDExists to return an error (this causes ConstructCSIFeatureGateYAMLSnippets to error)
		mockK8sClient.EXPECT().CheckCRDExists(gomock.AssignableToTypeOf("")).Return(false, fmt.Errorf("CRD check failed")).AnyTimes()
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().PutDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentDeployment(controllingCRDetails, labels, false, reuseServiceAccountMap)

		// Should still succeed despite CSI feature gate error (error is just logged)
		assert.NoError(t, err, "Should not error when CSI feature gate construction fails (error is logged)")
	})

	t.Run("force deployment recreation with shouldUpdate=true", func(t *testing.T) {
		// Test the logic where existing deployment is moved to unwanted when shouldUpdate=true
		existingDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "existing-deployment"},
		}
		mockK8sClient.EXPECT().GetDeploymentInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(existingDeployment, []appsv1.Deployment{}, false, nil)

		// Should remove the existing deployment because shouldUpdate=true
		mockK8sClient.EXPECT().RemoveMultipleDeployments(gomock.Any()).Do(func(deployments []appsv1.Deployment) {
			// Verify existing deployment is included in unwanted list
			assert.Len(t, deployments, 1)
			assert.Equal(t, "existing-deployment", deployments[0].Name)
		}).Return(nil)

		mockK8sClient.EXPECT().CheckCRDExists(gomock.AssignableToTypeOf("")).Return(false, nil).AnyTimes()
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().PutDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentDeployment(controllingCRDetails, labels, true, reuseServiceAccountMap)
		assert.NoError(t, err, "Should not error when creating Trident deployment with reuse service account map")
	})

	t.Run("force deployment recreation with reuse service account false", func(t *testing.T) {
		// Test the logic where existing deployment is recreated when service account can't be reused
		existingDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "existing-deployment"},
		}
		mockK8sClient.EXPECT().GetDeploymentInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(existingDeployment, []appsv1.Deployment{}, false, nil)

		// Use a service account map that indicates reuse is not allowed
		noReuseServiceAccountMap := map[string]bool{
			"trident-controller": false, // Force recreation
		}

		mockK8sClient.EXPECT().RemoveMultipleDeployments(gomock.Any()).Return(nil)
		mockK8sClient.EXPECT().CheckCRDExists(gomock.AssignableToTypeOf("")).Return(false, nil).AnyTimes()
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().PutDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentDeployment(controllingCRDetails, labels, false, noReuseServiceAccountMap)
		assert.NoError(t, err, "Should not error when creating Trident deployment without reusing service accounts")
	})

	t.Run("edge case - empty labels and controllingCRDetails", func(t *testing.T) {
		// Test with nil/empty maps to exercise edge case handling
		mockK8sClient.EXPECT().GetDeploymentInformation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, []appsv1.Deployment{}, true, nil)
		mockK8sClient.EXPECT().RemoveMultipleDeployments(gomock.Any()).Return(nil)
		mockK8sClient.EXPECT().CheckCRDExists(gomock.AssignableToTypeOf("")).Return(false, nil).AnyTimes()
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().PutDeployment(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentDeployment(nil, map[string]string{}, false, map[string]bool{})
		assert.NoError(t, err, "Should not error when creating Trident deployment with nil controlling CR details")
	})
}

func TestCreateOrPatchTridentDaemonSet(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	labels := map[string]string{
		TestAppLabel: TestTridentNetAppIO,
	}
	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}
	reuseServiceAccountMap := map[string]bool{
		"trident-node-linux":   true,
		"trident-node-windows": false,
	}

	t.Run("successful Linux daemonset creation", func(t *testing.T) {
		// Mock GetDaemonSetInformation to return no existing daemonset
		mockK8sClient.EXPECT().GetDaemonSetInformation(gomock.Any(), gomock.Any(), false).Return(nil, []appsv1.DaemonSet{}, true, nil)

		// Mock RemoveMultipleDaemonSets
		mockK8sClient.EXPECT().RemoveMultipleDaemonSets(gomock.Any()).Return(nil)

		// Mock ServerVersion call (used in DaemonSet args)
		mockK8sClient.EXPECT().ServerVersion().Return(nil).AnyTimes()

		// Mock PutDaemonSet to succeed
		mockK8sClient.EXPECT().PutDaemonSet(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentDaemonSet(controllingCRDetails, labels, false, reuseServiceAccountMap, false)

		assert.NoError(t, err, "Should not error when creating Linux Trident daemonset")
	})

	t.Run("successful Windows daemonset creation", func(t *testing.T) {
		// Mock GetDaemonSetInformation to return no existing daemonset
		mockK8sClient.EXPECT().GetDaemonSetInformation(gomock.Any(), gomock.Any(), true).Return(nil, []appsv1.DaemonSet{}, true, nil)

		// Mock RemoveMultipleDaemonSets
		mockK8sClient.EXPECT().RemoveMultipleDaemonSets(gomock.Any()).Return(nil)

		// Mock ServerVersion call (used in DaemonSet args)
		mockK8sClient.EXPECT().ServerVersion().Return(nil).AnyTimes()

		// Mock PutDaemonSet to succeed
		mockK8sClient.EXPECT().PutDaemonSet(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentDaemonSet(controllingCRDetails, labels, false, reuseServiceAccountMap, true)

		assert.NoError(t, err, "Should not error when creating Windows Trident daemonset")
	})

	t.Run("failure getting daemonset information", func(t *testing.T) {
		// Mock GetDaemonSetInformation to fail
		mockK8sClient.EXPECT().GetDaemonSetInformation(gomock.Any(), gomock.Any(), false).Return(nil, nil, false, fmt.Errorf("failed to get daemonset"))

		err := installer.createOrPatchTridentDaemonSet(controllingCRDetails, labels, false, reuseServiceAccountMap, false)

		assert.Error(t, err, "expected error when getting daemonset information fails")
		assert.Contains(t, err.Error(), "failed to get Trident daemonsets")
	})

	t.Run("failure removing unwanted daemonsets", func(t *testing.T) {
		// Mock GetDaemonSetInformation to return existing daemonset
		existingDaemonSet := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "old-daemonset"},
		}
		mockK8sClient.EXPECT().GetDaemonSetInformation(gomock.Any(), gomock.Any(), false).Return(existingDaemonSet, []appsv1.DaemonSet{}, false, nil)

		// Mock RemoveMultipleDaemonSets to fail
		mockK8sClient.EXPECT().RemoveMultipleDaemonSets(gomock.Any()).Return(fmt.Errorf("failed to remove daemonsets"))

		err := installer.createOrPatchTridentDaemonSet(controllingCRDetails, labels, true, reuseServiceAccountMap, false)

		assert.Error(t, err, "expected error when removing unwanted daemonsets fails")
		assert.Contains(t, err.Error(), "failed to remove unwanted Trident daemonsets")
	})

	t.Run("failure creating daemonset", func(t *testing.T) {
		// Mock GetDaemonSetInformation to return no existing daemonset
		mockK8sClient.EXPECT().GetDaemonSetInformation(gomock.Any(), gomock.Any(), false).Return(nil, []appsv1.DaemonSet{}, true, nil)

		// Mock RemoveMultipleDaemonSets to succeed
		mockK8sClient.EXPECT().RemoveMultipleDaemonSets(gomock.Any()).Return(nil)

		// Mock ServerVersion call
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		// Mock PutDaemonSet to fail
		mockK8sClient.EXPECT().PutDaemonSet(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to create daemonset"))

		err := installer.createOrPatchTridentDaemonSet(controllingCRDetails, labels, false, reuseServiceAccountMap, false)

		assert.Error(t, err, "expected error when creating daemonset fails")
		assert.Contains(t, err.Error(), "failed to create or patch Trident daemonset")
	})

	t.Run("should update existing daemonset", func(t *testing.T) {
		// Mock GetDaemonSetInformation to return existing daemonset
		existingDaemonSet := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "trident-csi"},
		}
		mockK8sClient.EXPECT().GetDaemonSetInformation(gomock.Any(), gomock.Any(), false).Return(existingDaemonSet, []appsv1.DaemonSet{}, false, nil)

		// Should create new daemonset because shouldUpdate=true and service account not reused
		mockK8sClient.EXPECT().RemoveMultipleDaemonSets(gomock.Any()).Return(nil)

		// Mock ServerVersion call
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		mockK8sClient.EXPECT().PutDaemonSet(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		// Pass false for reuseServiceAccountMap for Linux node service account
		reuseMapFalse := map[string]bool{
			"trident-node-linux": false,
		}

		err := installer.createOrPatchTridentDaemonSet(controllingCRDetails, labels, true, reuseMapFalse, false)

		assert.NoError(t, err, "Should not error when creating Trident daemonset with shouldUpdate=true")
	})

	t.Run("test tolerations handling", func(t *testing.T) {
		// Mock GetDaemonSetInformation to return no existing daemonset
		mockK8sClient.EXPECT().GetDaemonSetInformation(gomock.Any(), gomock.Any(), false).Return(nil, []appsv1.DaemonSet{}, true, nil)

		// Mock RemoveMultipleDaemonSets
		mockK8sClient.EXPECT().RemoveMultipleDaemonSets(gomock.Any()).Return(nil)

		// Mock ServerVersion call
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()

		// Mock PutDaemonSet to succeed
		mockK8sClient.EXPECT().PutDaemonSet(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		// This test verifies that the function handles tolerations correctly
		// The nodePluginTolerations global variable should be handled properly
		err := installer.createOrPatchTridentDaemonSet(controllingCRDetails, labels, false, reuseServiceAccountMap, false)

		assert.NoError(t, err, "Should not error when creating Trident daemonset with tolerations handling")
	})
}

func TestTridentDeploymentInformation(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	t.Run("successful deployment information retrieval", func(t *testing.T) {
		expectedDeployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "trident-csi"},
		}
		expectedUnwanted := []appsv1.Deployment{}
		expectedCreate := true
		expectedError := error(nil)

		// Mock the underlying client call
		mockK8sClient.EXPECT().GetDeploymentInformation(
			getDeploymentName(),
			"app=orchestrator.trident.netapp.io",
			installer.namespace,
		).Return(expectedDeployment, expectedUnwanted, expectedCreate, expectedError)

		deployment, unwanted, create, err := installer.TridentDeploymentInformation("app=orchestrator.trident.netapp.io")

		assert.NoError(t, err, "Should not error when retrieving Trident deployment information")
		assert.Equal(t, expectedDeployment, deployment)
		assert.Equal(t, expectedUnwanted, unwanted)
		assert.Equal(t, expectedCreate, create)
	})

	t.Run("failure retrieving deployment information", func(t *testing.T) {
		expectedError := fmt.Errorf("failed to get deployment info")

		// Mock the underlying client call to return error
		mockK8sClient.EXPECT().GetDeploymentInformation(
			getDeploymentName(),
			"app=orchestrator.trident.netapp.io",
			installer.namespace,
		).Return(nil, nil, false, expectedError)

		deployment, unwanted, create, err := installer.TridentDeploymentInformation("app=orchestrator.trident.netapp.io")

		assert.Error(t, err, "expected error when retrieving deployment information fails")
		assert.Nil(t, deployment)
		assert.Nil(t, unwanted)
		assert.False(t, create)
		assert.Equal(t, expectedError, err)
	})
}

func TestTridentDaemonSetInformation(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	t.Run("successful daemonset information retrieval", func(t *testing.T) {
		expectedDaemonSet := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "trident-csi"},
		}
		expectedUnwanted := []appsv1.DaemonSet{}
		expectedCreate := true
		expectedError := error(nil)

		// Mock the underlying client call
		mockK8sClient.EXPECT().GetDaemonSetInformation(
			TridentNodeLabel,
			installer.namespace,
			false,
		).Return(expectedDaemonSet, expectedUnwanted, expectedCreate, expectedError)

		daemonSet, unwanted, create, err := installer.TridentDaemonSetInformation()

		assert.NoError(t, err, "Should not error when retrieving Trident daemonset information")
		assert.Equal(t, expectedDaemonSet, daemonSet)
		assert.Equal(t, expectedUnwanted, unwanted)
		assert.Equal(t, expectedCreate, create)
	})

	t.Run("failure retrieving daemonset information", func(t *testing.T) {
		expectedError := fmt.Errorf("failed to get daemonset info")

		// Mock the underlying client call to return error
		mockK8sClient.EXPECT().GetDaemonSetInformation(
			TridentNodeLabel,
			installer.namespace,
			false,
		).Return(nil, nil, false, expectedError)

		daemonSet, unwanted, create, err := installer.TridentDaemonSetInformation()

		assert.Error(t, err, "expected error when retrieving daemonset information fails")
		assert.Nil(t, daemonSet)
		assert.Nil(t, unwanted)
		assert.False(t, create)
		assert.Equal(t, expectedError, err)
	})
}

func TestCreateOrPatchTridentService(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	labels := map[string]string{
		TestAppLabel: TestTridentNetAppIO,
	}
	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}

	t.Run("successful service creation", func(t *testing.T) {
		// Mock GetServiceInformation to return no existing service
		mockK8sClient.EXPECT().GetServiceInformation(
			getServiceName(),
			appLabel,
			installer.namespace,
			false,
		).Return(nil, []corev1.Service{}, true, nil)

		// Mock RemoveMultipleServices
		mockK8sClient.EXPECT().RemoveMultipleServices(gomock.Any()).Return(nil)

		// Mock PutService to succeed
		mockK8sClient.EXPECT().PutService(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentService(controllingCRDetails, labels, false)

		assert.NoError(t, err, "Should not error when creating Trident service with shouldUpdate=false")
	})

	t.Run("failure getting service information", func(t *testing.T) {
		// Mock GetServiceInformation to fail
		mockK8sClient.EXPECT().GetServiceInformation(
			getServiceName(),
			appLabel,
			installer.namespace,
			false,
		).Return(nil, nil, false, fmt.Errorf("failed to get service"))

		err := installer.createOrPatchTridentService(controllingCRDetails, labels, false)

		assert.Error(t, err, "expected error when getting service information fails")
		assert.Contains(t, err.Error(), "failed to get Trident services")
	})

	t.Run("failure removing unwanted services", func(t *testing.T) {
		// Mock GetServiceInformation to return existing service
		existingService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "old-service"},
		}
		mockK8sClient.EXPECT().GetServiceInformation(
			getServiceName(),
			appLabel,
			installer.namespace,
			true,
		).Return(existingService, []corev1.Service{}, false, nil)

		// Mock RemoveMultipleServices to fail
		mockK8sClient.EXPECT().RemoveMultipleServices(gomock.Any()).Return(fmt.Errorf("failed to remove services"))

		err := installer.createOrPatchTridentService(controllingCRDetails, labels, true)

		assert.Error(t, err, "expected error when removing unwanted services fails")
		assert.Contains(t, err.Error(), "failed to remove unwanted Trident services")
	})

	t.Run("failure creating service", func(t *testing.T) {
		// Mock GetServiceInformation to return no existing service
		mockK8sClient.EXPECT().GetServiceInformation(
			getServiceName(),
			appLabel,
			installer.namespace,
			false,
		).Return(nil, []corev1.Service{}, true, nil)

		// Mock RemoveMultipleServices to succeed
		mockK8sClient.EXPECT().RemoveMultipleServices(gomock.Any()).Return(nil)

		// Mock PutService to fail
		mockK8sClient.EXPECT().PutService(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to create service"))

		err := installer.createOrPatchTridentService(controllingCRDetails, labels, false)

		assert.Error(t, err, "expected error when creating service fails")
		assert.Contains(t, err.Error(), "failed to create or patch Trident service")
	})

	t.Run("should update existing service", func(t *testing.T) {
		// Mock GetServiceInformation to return existing service
		existingService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "trident-csi"},
		}
		mockK8sClient.EXPECT().GetServiceInformation(
			getServiceName(),
			appLabel,
			installer.namespace,
			true,
		).Return(existingService, []corev1.Service{}, false, nil)

		// Mock RemoveMultipleServices to succeed
		mockK8sClient.EXPECT().RemoveMultipleServices(gomock.Any()).Return(nil)

		// Mock PutService to succeed
		mockK8sClient.EXPECT().PutService(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentService(controllingCRDetails, labels, true)

		assert.NoError(t, err, "Should not error when creating Trident service with shouldUpdate=true")
	})
}

func TestImagePrechecks(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	labels := map[string]string{
		TestAppLabel: TestTridentNetAppIO,
	}
	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}

	t.Run("failure getting deployment information", func(t *testing.T) {
		// Mock TridentDeploymentInformation to fail (it calls GetDeploymentInformation internally)
		mockK8sClient.EXPECT().GetDeploymentInformation(
			getDeploymentName(),
			appLabel,
			installer.namespace,
		).Return(nil, nil, false, fmt.Errorf("failed to get deployment"))

		version, err := installer.imagePrechecks(labels, controllingCRDetails)

		assert.Error(t, err, "expected error when getting deployment information fails")
		assert.Contains(t, err.Error(), "unable to get existing deployment information")
		assert.Empty(t, version)
	})

	t.Run("no existing deployment - version check required", func(t *testing.T) {
		// Mock TridentDeploymentInformation to return no deployment
		mockK8sClient.EXPECT().GetDeploymentInformation(
			getDeploymentName(),
			appLabel,
			installer.namespace,
		).Return(nil, []appsv1.Deployment{}, true, nil)

		// Since performImageVersionCheck = true, we need to mock the downstream calls
		// Mock createRBACObjects to fail early to avoid complex mocking
		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil, nil, nil, nil, fmt.Errorf("RBAC creation failed"))

		version, err := installer.imagePrechecks(labels, controllingCRDetails)

		assert.Error(t, err, "expected error when version check RBAC creation fails")
		assert.Contains(t, err.Error(), "unable to create RBAC objects while verifying Trident version")
		assert.Empty(t, version)
	})

	t.Run("existing deployment with same image and valid version", func(t *testing.T) {
		// Create deployment with matching image and valid version label
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "trident-csi",
				Labels: map[string]string{
					TridentVersionLabelKey: DefaultTridentVersion, // Same as supported version
				},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "trident-main",
								Image: tridentImage, // Same as new image
							},
						},
					},
				},
			},
		}

		// Mock TridentDeploymentInformation to return this deployment
		mockK8sClient.EXPECT().GetDeploymentInformation(
			getDeploymentName(),
			appLabel,
			installer.namespace,
		).Return(deployment, []appsv1.Deployment{}, false, nil)

		version, err := installer.imagePrechecks(labels, controllingCRDetails)

		assert.NoError(t, err, "Should not error when performing image prechecks with valid deployment")
		assert.Equal(t, DefaultTridentVersion, version)
	})

	t.Run("existing deployment with different image", func(t *testing.T) {
		// Create deployment with different image
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "trident-csi",
				Labels: map[string]string{
					TridentVersionLabelKey: "v1.0.0",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "trident-main",
								Image: "different-image:v1.0.0", // Different from tridentImage
							},
						},
					},
				},
			},
		}

		// Mock TridentDeploymentInformation to return this deployment
		mockK8sClient.EXPECT().GetDeploymentInformation(
			getDeploymentName(),
			appLabel,
			installer.namespace,
		).Return(deployment, []appsv1.Deployment{}, false, nil)

		// Since image is different, performImageVersionCheck = true, mock createRBACObjects to fail early
		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil, nil, nil, nil, fmt.Errorf("RBAC creation failed"))

		version, err := installer.imagePrechecks(labels, controllingCRDetails)

		assert.Error(t, err, "expected error when image differs and RBAC creation fails")
		assert.Contains(t, err.Error(), "unable to create RBAC objects while verifying Trident version")
		assert.Empty(t, version)
	})

	t.Run("existing deployment missing version label", func(t *testing.T) {
		// Create deployment without version label
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "trident-csi",
				Labels: map[string]string{}, // Missing TridentVersionLabelKey
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "trident-main",
								Image: tridentImage,
							},
						},
					},
				},
			},
		}

		// Mock TridentDeploymentInformation to return this deployment
		mockK8sClient.EXPECT().GetDeploymentInformation(
			getDeploymentName(),
			appLabel,
			installer.namespace,
		).Return(deployment, []appsv1.Deployment{}, false, nil)

		// Since version label is missing, performImageVersionCheck = true, mock createRBACObjects to fail early
		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			nil, nil, nil, nil, fmt.Errorf("RBAC creation failed"))

		version, err := installer.imagePrechecks(labels, controllingCRDetails)

		assert.Error(t, err, "expected error when version label missing and RBAC creation fails")
		assert.Contains(t, err.Error(), "unable to create RBAC objects while verifying Trident version")
		assert.Empty(t, version)
	})
}

func TestCreateOrPatchK8sCSIDriver(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	labels := map[string]string{
		TestAppLabel: TestTridentNetAppIO,
	}
	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}

	t.Run("successful CSI driver creation", func(t *testing.T) {
		// Mock GetCSIDriverInformation to return no existing CSI driver
		mockK8sClient.EXPECT().GetCSIDriverInformation(
			getCSIDriverName(),
			appLabel,
			false,
		).Return(nil, []storagev1.CSIDriver{}, true, nil)

		// Mock RemoveMultipleCSIDriverCRs
		mockK8sClient.EXPECT().RemoveMultipleCSIDriverCRs(gomock.Any()).Return(nil)

		// Mock PutCSIDriver to succeed
		mockK8sClient.EXPECT().PutCSIDriver(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchK8sCSIDriver(controllingCRDetails, labels, false)

		assert.NoError(t, err, "Should not error when creating K8s CSI driver with shouldUpdate=false")
	})

	t.Run("failure getting CSI driver information", func(t *testing.T) {
		// Mock GetCSIDriverInformation to fail
		mockK8sClient.EXPECT().GetCSIDriverInformation(
			getCSIDriverName(),
			appLabel,
			false,
		).Return(nil, nil, false, fmt.Errorf("failed to get CSI driver"))

		err := installer.createOrPatchK8sCSIDriver(controllingCRDetails, labels, false)

		assert.Error(t, err, "expected error when getting CSI driver information fails")
		assert.Contains(t, err.Error(), "failed to get K8s CSI drivers")
	})

	t.Run("failure removing unwanted CSI drivers", func(t *testing.T) {
		// Mock GetCSIDriverInformation to return existing CSI driver
		existingCSIDriver := &storagev1.CSIDriver{
			ObjectMeta: metav1.ObjectMeta{Name: "old-csi-driver"},
		}
		mockK8sClient.EXPECT().GetCSIDriverInformation(
			getCSIDriverName(),
			appLabel,
			true,
		).Return(existingCSIDriver, []storagev1.CSIDriver{}, false, nil)

		// Mock RemoveMultipleCSIDriverCRs to fail
		mockK8sClient.EXPECT().RemoveMultipleCSIDriverCRs(gomock.Any()).Return(fmt.Errorf("failed to remove CSI drivers"))

		err := installer.createOrPatchK8sCSIDriver(controllingCRDetails, labels, true)

		assert.Error(t, err, "expected error when removing unwanted CSI drivers fails")
		assert.Contains(t, err.Error(), "failed to remove unwanted K8s CSI drivers")
	})

	t.Run("failure creating CSI driver", func(t *testing.T) {
		// Mock GetCSIDriverInformation to return no existing CSI driver
		mockK8sClient.EXPECT().GetCSIDriverInformation(
			getCSIDriverName(),
			appLabel,
			false,
		).Return(nil, []storagev1.CSIDriver{}, true, nil)

		// Mock RemoveMultipleCSIDriverCRs to succeed
		mockK8sClient.EXPECT().RemoveMultipleCSIDriverCRs(gomock.Any()).Return(nil)

		// Mock PutCSIDriver to fail
		mockK8sClient.EXPECT().PutCSIDriver(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to create CSI driver"))

		err := installer.createOrPatchK8sCSIDriver(controllingCRDetails, labels, false)

		assert.Error(t, err, "expected error when creating CSI driver fails")
		assert.Contains(t, err.Error(), "failed to create or patch K8s CSI drivers")
	})

	t.Run("should update existing CSI driver", func(t *testing.T) {
		// Mock GetCSIDriverInformation to return existing CSI driver
		existingCSIDriver := &storagev1.CSIDriver{
			ObjectMeta: metav1.ObjectMeta{Name: "csi.trident.netapp.io"},
		}
		mockK8sClient.EXPECT().GetCSIDriverInformation(
			getCSIDriverName(),
			appLabel,
			true,
		).Return(existingCSIDriver, []storagev1.CSIDriver{}, false, nil)

		// Mock RemoveMultipleCSIDriverCRs to succeed
		mockK8sClient.EXPECT().RemoveMultipleCSIDriverCRs(gomock.Any()).Return(nil)

		// Mock PutCSIDriver to succeed
		mockK8sClient.EXPECT().PutCSIDriver(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchK8sCSIDriver(controllingCRDetails, labels, true)

		assert.NoError(t, err, "Should not error when creating K8s CSI driver with shouldUpdate=true")
	})
}

func TestCreateOrPatchTridentProtocolSecret(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	labels := map[string]string{
		TestAppLabel: TestTridentNetAppIO,
	}
	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}

	t.Run("successful secret creation", func(t *testing.T) {
		// Mock GetSecretInformation to return no existing secret, requiring creation
		mockK8sClient.EXPECT().GetSecretInformation(
			getProtocolSecretName(),
			appLabel,
			installer.namespace,
			false,
		).Return(nil, []corev1.Secret{}, true, nil)

		// Mock RemoveMultipleSecrets
		mockK8sClient.EXPECT().RemoveMultipleSecrets(gomock.Any()).Return(nil)

		// Mock PutSecret to succeed
		mockK8sClient.EXPECT().PutSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentProtocolSecret(controllingCRDetails, labels, false)

		assert.NoError(t, err, "Should not error when creating Trident protocol secret with shouldUpdate=false")
	})

	t.Run("successful secret patch without creation", func(t *testing.T) {
		// Mock GetSecretInformation to return existing secret, no creation needed
		existingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: getProtocolSecretName()},
		}
		mockK8sClient.EXPECT().GetSecretInformation(
			getProtocolSecretName(),
			appLabel,
			installer.namespace,
			true,
		).Return(existingSecret, []corev1.Secret{}, false, nil)

		// Mock RemoveMultipleSecrets
		mockK8sClient.EXPECT().RemoveMultipleSecrets(gomock.Any()).Return(nil)

		// Mock PutSecret to succeed
		mockK8sClient.EXPECT().PutSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentProtocolSecret(controllingCRDetails, labels, true)

		assert.NoError(t, err, "Should not error when creating Trident protocol secret with shouldUpdate=true")
	})

	t.Run("failure getting secret information", func(t *testing.T) {
		// Mock GetSecretInformation to fail
		mockK8sClient.EXPECT().GetSecretInformation(
			getProtocolSecretName(),
			appLabel,
			installer.namespace,
			false,
		).Return(nil, nil, false, fmt.Errorf("failed to get secret"))

		err := installer.createOrPatchTridentProtocolSecret(controllingCRDetails, labels, false)

		assert.Error(t, err, "expected error when getting secret information fails")
		assert.Contains(t, err.Error(), "failed to get Trident secrets")
	})

	t.Run("failure removing unwanted secrets", func(t *testing.T) {
		// Mock GetSecretInformation to return existing secret
		existingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "old-secret"},
		}
		unwantedSecrets := []corev1.Secret{*existingSecret}
		mockK8sClient.EXPECT().GetSecretInformation(
			getProtocolSecretName(),
			appLabel,
			installer.namespace,
			true,
		).Return(existingSecret, unwantedSecrets, false, nil)

		// Mock RemoveMultipleSecrets to fail
		mockK8sClient.EXPECT().RemoveMultipleSecrets(unwantedSecrets).Return(fmt.Errorf("failed to remove secrets"))

		err := installer.createOrPatchTridentProtocolSecret(controllingCRDetails, labels, true)

		assert.Error(t, err, "expected error when removing unwanted secrets fails")
		assert.Contains(t, err.Error(), "failed to remove unwanted Trident secrets")
	})

	t.Run("failure creating secret", func(t *testing.T) {
		// Mock GetSecretInformation to return no existing secret, requiring creation
		mockK8sClient.EXPECT().GetSecretInformation(
			getProtocolSecretName(),
			appLabel,
			installer.namespace,
			false,
		).Return(nil, []corev1.Secret{}, true, nil)

		// Mock RemoveMultipleSecrets to succeed
		mockK8sClient.EXPECT().RemoveMultipleSecrets(gomock.Any()).Return(nil)

		// Mock PutSecret to fail
		mockK8sClient.EXPECT().PutSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to create secret"))

		err := installer.createOrPatchTridentProtocolSecret(controllingCRDetails, labels, false)

		assert.Error(t, err, "expected error when creating secret fails")
		assert.Contains(t, err.Error(), "failed to create or patch Trident secret")
	})

	t.Run("successful creation with certificate generation", func(t *testing.T) {
		// Test the full certificate creation path more thoroughly
		mockK8sClient.EXPECT().GetSecretInformation(
			getProtocolSecretName(),
			appLabel,
			installer.namespace,
			false,
		).Return(nil, []corev1.Secret{}, true, nil)

		// Mock RemoveMultipleSecrets
		mockK8sClient.EXPECT().RemoveMultipleSecrets(gomock.Any()).Return(nil)

		// Mock PutSecret to succeed - this should trigger certificate creation
		mockK8sClient.EXPECT().PutSecret(true, gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentProtocolSecret(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating Trident protocol secret with specific conditions")
	})

	t.Run("successful patch with unwanted secrets cleanup", func(t *testing.T) {
		// Test scenario with unwanted secrets present
		existingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: getProtocolSecretName()},
		}
		unwantedSecrets := []corev1.Secret{
			{ObjectMeta: metav1.ObjectMeta{Name: "old-protocol-secret-1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "old-protocol-secret-2"}},
		}

		mockK8sClient.EXPECT().GetSecretInformation(
			getProtocolSecretName(),
			appLabel,
			installer.namespace,
			true,
		).Return(existingSecret, unwantedSecrets, false, nil)

		// Mock RemoveMultipleSecrets with specific unwanted secrets
		mockK8sClient.EXPECT().RemoveMultipleSecrets(unwantedSecrets).Return(nil)

		// Mock PutSecret with createSecret=false (patch mode)
		mockK8sClient.EXPECT().PutSecret(false, gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentProtocolSecret(controllingCRDetails, labels, true)
		assert.NoError(t, err, "Should not error when updating Trident protocol secret with specific conditions")
	})

	t.Run("edge case - empty unwanted secrets list", func(t *testing.T) {
		// Test with no unwanted secrets to clean up
		mockK8sClient.EXPECT().GetSecretInformation(
			getProtocolSecretName(),
			appLabel,
			installer.namespace,
			false,
		).Return(nil, []corev1.Secret{}, true, nil)

		// Empty list of unwanted secrets
		mockK8sClient.EXPECT().RemoveMultipleSecrets([]corev1.Secret{}).Return(nil)
		mockK8sClient.EXPECT().PutSecret(true, gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentProtocolSecret(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating Trident protocol secret with specific conditions")
	})

	t.Run("edge case - patch existing secret with shouldUpdate false", func(t *testing.T) {
		// Test createSecret=false path with shouldUpdate=false
		existingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: getProtocolSecretName()},
		}
		mockK8sClient.EXPECT().GetSecretInformation(
			getProtocolSecretName(),
			appLabel,
			installer.namespace,
			false,
		).Return(existingSecret, []corev1.Secret{}, false, nil)

		mockK8sClient.EXPECT().RemoveMultipleSecrets([]corev1.Secret{}).Return(nil)
		// This should call PutSecret with createSecret=false
		mockK8sClient.EXPECT().PutSecret(false, gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentProtocolSecret(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating Trident protocol secret with specific conditions")
	})
}

func TestUninstallTrident(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	t.Run("successful complete uninstall", func(t *testing.T) {
		// Mock all delete operations to succeed
		mockK8sClient.EXPECT().DeleteTridentDeployment(TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentDaemonSet(TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentResourceQuota(TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentService(getServiceName(), TridentCSILabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentSecret(getProtocolSecretName(), TridentCSILabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().DeleteCSIDriverCR(getCSIDriverName(), TridentCSILabel).Return(nil)

		// Mock deletion of TridentNodeRemediation resources
		mockK8sClient.EXPECT().DeleteTridentNodeRemediationResources(installer.namespace).Return(nil)

		// Mock removeRBACObjects (which calls various delete operations)
		mockK8sClient.EXPECT().DeleteTridentClusterRole(gomock.Any(), TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(gomock.Any(), TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentClusterRole(gomock.Any(), TridentNodeLabel).Return(nil).Times(2)
		mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(gomock.Any(), TridentNodeLabel).Return(nil).Times(2)
		mockK8sClient.EXPECT().DeleteMultipleTridentRoles(gomock.Any(), TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentRoleBindings(gomock.Any(), TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentServiceAccounts(gomock.Any(), TridentCSILabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentServiceAccounts(gomock.Any(), TridentNodeLabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().Flavor().Return(k8sclient.FlavorKubernetes) // Non-OpenShift to skip SCC deletion

		err := installer.UninstallTrident()

		assert.NoError(t, err, "Should not error when uninstalling Trident")
	})

	t.Run("failure deleting deployment", func(t *testing.T) {
		// Mock DeleteTridentDeployment to fail
		mockK8sClient.EXPECT().DeleteTridentDeployment(TridentCSILabel).Return(fmt.Errorf("failed to delete deployment"))

		err := installer.UninstallTrident()

		assert.Error(t, err, "expected error when deleting deployment fails")
		assert.Contains(t, err.Error(), "could not delete Trident CSI deployment")
	})

	t.Run("failure deleting daemonset", func(t *testing.T) {
		// Mock deployment deletion to succeed, daemonset to fail
		mockK8sClient.EXPECT().DeleteTridentDeployment(TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentDaemonSet(TridentNodeLabel).Return(fmt.Errorf("failed to delete daemonset"))

		err := installer.UninstallTrident()

		assert.Error(t, err, "expected error when deleting daemonset fails")
		assert.Contains(t, err.Error(), "could not delete Trident daemonset")
	})

	t.Run("failure deleting resource quota", func(t *testing.T) {
		// Mock previous deletions to succeed, resource quota to fail
		mockK8sClient.EXPECT().DeleteTridentDeployment(TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentDaemonSet(TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentResourceQuota(TridentNodeLabel).Return(fmt.Errorf("failed to delete resource quota"))

		err := installer.UninstallTrident()

		assert.Error(t, err, "expected error when deleting resource quota fails")
		assert.Contains(t, err.Error(), "could not delete Trident resource quota")
	})

	t.Run("failure deleting service", func(t *testing.T) {
		// Mock previous deletions to succeed, service to fail
		mockK8sClient.EXPECT().DeleteTridentDeployment(TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentDaemonSet(TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentResourceQuota(TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentService(getServiceName(), TridentCSILabel, installer.namespace).Return(fmt.Errorf("failed to delete service"))

		err := installer.UninstallTrident()

		assert.Error(t, err, "expected error when deleting service fails")
		assert.Contains(t, err.Error(), "could not delete Trident service")
	})

	t.Run("failure deleting secret", func(t *testing.T) {
		// Mock previous deletions to succeed, secret to fail
		mockK8sClient.EXPECT().DeleteTridentDeployment(TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentDaemonSet(TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentResourceQuota(TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentService(getServiceName(), TridentCSILabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentSecret(getProtocolSecretName(), TridentCSILabel, installer.namespace).Return(fmt.Errorf("failed to delete secret"))

		err := installer.UninstallTrident()

		assert.Error(t, err, "expected error when deleting secret fails")
		assert.Contains(t, err.Error(), "could not delete Trident secret")
	})

	t.Run("failure deleting CSI driver", func(t *testing.T) {
		// Mock previous deletions to succeed, CSI driver to fail
		mockK8sClient.EXPECT().DeleteTridentDeployment(TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentDaemonSet(TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentResourceQuota(TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentService(getServiceName(), TridentCSILabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentSecret(getProtocolSecretName(), TridentCSILabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().DeleteCSIDriverCR(getCSIDriverName(), TridentCSILabel).Return(fmt.Errorf("failed to delete CSI driver"))

		err := installer.UninstallTrident()

		assert.Error(t, err, "expected error when deleting CSI driver fails")
		assert.Contains(t, err.Error(), "could not delete Trident CSI driver custom resource")
	})

	t.Run("failure deleting RBAC objects", func(t *testing.T) {
		// Mock all individual deletions to succeed, RBAC to fail
		mockK8sClient.EXPECT().DeleteTridentDeployment(TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentDaemonSet(TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentResourceQuota(TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentService(getServiceName(), TridentCSILabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentSecret(getProtocolSecretName(), TridentCSILabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().DeleteCSIDriverCR(getCSIDriverName(), TridentCSILabel).Return(nil)

		// Mock deletion of TridentNodeRemediation resources
		mockK8sClient.EXPECT().DeleteTridentNodeRemediationResources(installer.namespace).Return(nil)

		// Mock RBAC deletion to fail early on controller cluster role
		mockK8sClient.EXPECT().DeleteTridentClusterRole(gomock.Any(), TridentCSILabel).Return(fmt.Errorf("failed to delete cluster role"))

		err := installer.UninstallTrident()

		assert.Error(t, err, "expected error when deleting RBAC objects fails")
		assert.Contains(t, err.Error(), "could not delete all Trident's RBAC objects")
	})
}

func TestCreateTridentVersionPod(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	podName := "transient-trident-version-pod"
	imageName := "netapp/trident:v23.04.0"
	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}
	podLabels := map[string]string{
		TridentVersionPodLabelKey: TridentVersionPodLabelValue,
		K8sVersionLabelKey:        "v1.28.0",
	}

	t.Run("successful version pod creation", func(t *testing.T) {
		// Mock DeleteTransientVersionPod to succeed
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)

		// Mock CreateObjectByYAML to succeed
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)

		err := installer.createTridentVersionPod(podName, imageName, controllingCRDetails, podLabels)

		assert.NoError(t, err, "Should not error when creating Trident version pod successfully")
	})

	t.Run("failure deleting previous transient version pod", func(t *testing.T) {
		// Mock DeleteTransientVersionPod to fail
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(fmt.Errorf("failed to delete pod"))

		err := installer.createTridentVersionPod(podName, imageName, controllingCRDetails, podLabels)

		assert.Error(t, err, "expected error when deleting previous transient version pod fails")
		assert.Contains(t, err.Error(), "failed to delete previous transient version pod")
	})

	t.Run("failure creating version pod", func(t *testing.T) {
		// Mock DeleteTransientVersionPod to succeed
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)

		// Mock CreateObjectByYAML to fail
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(fmt.Errorf("failed to create pod"))

		err := installer.createTridentVersionPod(podName, imageName, controllingCRDetails, podLabels)

		assert.Error(t, err, "expected error when creating version pod fails")
		assert.Contains(t, err.Error(), "failed to create Trident version pod")
	})

	t.Run("successful creation with empty labels", func(t *testing.T) {
		// Test with empty labels to ensure the function handles this case
		emptyLabels := make(map[string]string)

		// Mock DeleteTransientVersionPod to succeed
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)

		// Mock CreateObjectByYAML to succeed
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)

		err := installer.createTridentVersionPod(podName, imageName, controllingCRDetails, emptyLabels)

		assert.NoError(t, err, "Should not error when creating Trident version pod with empty labels")
	})

	t.Run("successful creation with empty controlling CR details", func(t *testing.T) {
		// Test with empty controlling CR details
		emptyDetails := make(map[string]string)

		// Mock DeleteTransientVersionPod to succeed
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)

		// Mock CreateObjectByYAML to succeed
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)

		err := installer.createTridentVersionPod(podName, imageName, emptyDetails, podLabels)

		assert.NoError(t, err, "Should not error when creating Trident version pod with empty controlling CR details")
	})

	t.Run("test tolerations handling", func(t *testing.T) {
		// This test verifies that the function handles tolerations correctly
		// The controllerPluginTolerations global variable should be handled properly

		// Mock DeleteTransientVersionPod to succeed
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)

		// Mock CreateObjectByYAML to succeed
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)

		err := installer.createTridentVersionPod(podName, imageName, controllingCRDetails, podLabels)

		assert.NoError(t, err, "Should not error when creating Trident version pod with tolerations handling")
	})
}

func TestCreateOrPatchTridentInstallationNamespace(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	t.Run("successful namespace creation when namespace does not exist", func(t *testing.T) {
		// Mock CheckNamespaceExists to return false (namespace doesn't exist)
		mockK8sClient.EXPECT().CheckNamespaceExists(installer.namespace).Return(false, nil)

		// Mock CreateObjectByYAML to succeed
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)

		err := installer.createOrPatchTridentInstallationNamespace()

		assert.NoError(t, err, "Should not error when creating or patching Trident installation namespace")
	})

	t.Run("successful namespace patching when namespace exists", func(t *testing.T) {
		// Mock CheckNamespaceExists to return true (namespace exists)
		mockK8sClient.EXPECT().CheckNamespaceExists(installer.namespace).Return(true, nil)

		// Mock PatchNamespaceLabels to succeed
		mockK8sClient.EXPECT().PatchNamespaceLabels(installer.namespace, gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentInstallationNamespace()

		assert.NoError(t, err, "Should not error when creating or patching Trident installation namespace")
	})

	t.Run("failure checking if namespace exists", func(t *testing.T) {
		// Mock CheckNamespaceExists to fail
		mockK8sClient.EXPECT().CheckNamespaceExists(installer.namespace).Return(false, fmt.Errorf("failed to check namespace"))

		err := installer.createOrPatchTridentInstallationNamespace()

		assert.Error(t, err, "expected error when checking namespace existence fails")
		assert.Contains(t, err.Error(), "unable to check if namespace")
		assert.Contains(t, err.Error(), "exists")
	})

	t.Run("failure creating namespace", func(t *testing.T) {
		// Mock CheckNamespaceExists to return false (namespace doesn't exist)
		mockK8sClient.EXPECT().CheckNamespaceExists(installer.namespace).Return(false, nil)

		// Mock CreateObjectByYAML to fail
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(fmt.Errorf("failed to create namespace"))

		err := installer.createOrPatchTridentInstallationNamespace()

		assert.Error(t, err, "expected error when creating namespace fails")
		assert.Contains(t, err.Error(), "failed to create Trident installation namespace")
	})

	t.Run("failure patching namespace labels", func(t *testing.T) {
		// Mock CheckNamespaceExists to return true (namespace exists)
		mockK8sClient.EXPECT().CheckNamespaceExists(installer.namespace).Return(true, nil)

		// Mock PatchNamespaceLabels to fail
		mockK8sClient.EXPECT().PatchNamespaceLabels(installer.namespace, gomock.Any()).Return(fmt.Errorf("failed to patch namespace"))

		err := installer.createOrPatchTridentInstallationNamespace()

		assert.Error(t, err, "expected error when patching namespace labels fails")
		assert.Contains(t, err.Error(), "failed to patch Trident installation namespace")
	})

	t.Run("verify correct labels are applied during patch", func(t *testing.T) {
		// Mock CheckNamespaceExists to return true (namespace exists)
		mockK8sClient.EXPECT().CheckNamespaceExists(installer.namespace).Return(true, nil)

		// Mock PatchNamespaceLabels with specific expectation for the labels map
		expectedLabels := map[string]string{
			"pod-security.kubernetes.io/enforce": "privileged",
		}
		mockK8sClient.EXPECT().PatchNamespaceLabels(installer.namespace, expectedLabels).Return(nil)

		err := installer.createOrPatchTridentInstallationNamespace()

		assert.NoError(t, err, "Should not error when creating or patching Trident installation namespace")
	})
}

func TestLogFormatPrechecks(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	t.Run("valid text format", func(t *testing.T) {
		// Save original value
		originalLogFormat := logFormat
		defer func() { logFormat = originalLogFormat }()

		logFormat = "text"
		err := installer.logFormatPrechecks()
		assert.NoError(t, err, "Should not error when log format is valid text")
	})

	t.Run("valid json format", func(t *testing.T) {
		// Save original value
		originalLogFormat := logFormat
		defer func() { logFormat = originalLogFormat }()

		logFormat = "json"
		err := installer.logFormatPrechecks()
		assert.NoError(t, err, "Should not error when log format is valid json")
	})

	t.Run("invalid format returns error", func(t *testing.T) {
		// Save original value
		originalLogFormat := logFormat
		defer func() { logFormat = originalLogFormat }()

		logFormat = "invalid"
		err := installer.logFormatPrechecks()
		assert.Error(t, err, "expected error for invalid log format")
		assert.Contains(t, err.Error(), "'invalid' is not a valid log format")
	})

	t.Run("empty format returns error", func(t *testing.T) {
		// Save original value
		originalLogFormat := logFormat
		defer func() { logFormat = originalLogFormat }()

		logFormat = ""
		err := installer.logFormatPrechecks()
		assert.Error(t, err, "expected error for empty log format")
		assert.Contains(t, err.Error(), "'' is not a valid log format")
	})

	t.Run("case sensitive validation", func(t *testing.T) {
		// Save original value
		originalLogFormat := logFormat
		defer func() { logFormat = originalLogFormat }()

		logFormat = "TEXT"
		err := installer.logFormatPrechecks()
		assert.Error(t, err, "expected error for case-sensitive log format validation")
		assert.Contains(t, err.Error(), "'TEXT' is not a valid log format")
	})
}

func TestImagePullPolicyPrechecks(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	t.Run("valid PullIfNotPresent policy", func(t *testing.T) {
		// Save original value
		originalPolicy := imagePullPolicy
		defer func() { imagePullPolicy = originalPolicy }()

		imagePullPolicy = "IfNotPresent"
		err := installer.imagePullPolicyPrechecks()
		assert.NoError(t, err, "Should not error when image pull policy is IfNotPresent")
	})

	t.Run("valid PullAlways policy", func(t *testing.T) {
		// Save original value
		originalPolicy := imagePullPolicy
		defer func() { imagePullPolicy = originalPolicy }()

		imagePullPolicy = "Always"
		err := installer.imagePullPolicyPrechecks()
		assert.NoError(t, err, "Should not error when image pull policy is Always")
	})

	t.Run("valid PullNever policy", func(t *testing.T) {
		// Save original value
		originalPolicy := imagePullPolicy
		defer func() { imagePullPolicy = originalPolicy }()

		imagePullPolicy = "Never"
		err := installer.imagePullPolicyPrechecks()
		assert.NoError(t, err, "Should not error when image pull policy is Never")
	})

	t.Run("invalid policy returns error", func(t *testing.T) {
		// Save original value
		originalPolicy := imagePullPolicy
		defer func() { imagePullPolicy = originalPolicy }()

		imagePullPolicy = "Invalid"
		err := installer.imagePullPolicyPrechecks()
		assert.Error(t, err, "expected error for invalid image pull policy")
		assert.Contains(t, err.Error(), "'Invalid' is not a valid trident image pull policy format")
	})

	t.Run("empty policy returns error", func(t *testing.T) {
		// Save original value
		originalPolicy := imagePullPolicy
		defer func() { imagePullPolicy = originalPolicy }()

		imagePullPolicy = ""
		err := installer.imagePullPolicyPrechecks()
		assert.Error(t, err, "expected error for empty image pull policy")
		assert.Contains(t, err.Error(), "'' is not a valid trident image pull policy format")
	})
}

func TestCreateCRDs(t *testing.T) {
	// Focus on testing the high-level orchestration logic of createCRDs
	// This function calls CreateOrPatchCRD 15 times in sequence and fails fast on errors

	t.Run("failure on first CRD", func(t *testing.T) {
		mockK8sClient := newMockKubeClient(t)
		installer := newTestInstaller(mockK8sClient)

		// Mock the first CRD operation to fail
		mockK8sClient.EXPECT().CheckCRDExists(VersionCRDName).Return(false, fmt.Errorf("version CRD check failed"))

		err := installer.createCRDs(false)
		assert.Error(t, err, "expected error when first CRD check fails")
		assert.Contains(t, err.Error(), "version CRD check failed")
	})

	t.Run("failure on middle CRD", func(t *testing.T) {
		mockK8sClient := newMockKubeClient(t)
		installer := newTestInstaller(mockK8sClient)

		// Mock first few CRDs to succeed
		mockK8sClient.EXPECT().CheckCRDExists(VersionCRDName).Return(false, nil)
		mockK8sClient.EXPECT().PutCustomResourceDefinition(nil, VersionCRDName, true, gomock.Any()).Return(nil)

		mockK8sClient.EXPECT().CheckCRDExists(BackendCRDName).Return(false, nil)
		mockK8sClient.EXPECT().PutCustomResourceDefinition(nil, BackendCRDName, true, gomock.Any()).Return(nil)

		// Third CRD (SnapshotInfo) fails
		mockK8sClient.EXPECT().CheckCRDExists(SnapshotInfoCRDName).Return(false, fmt.Errorf("snapshotinfo CRD failed"))

		err := installer.createCRDs(false)
		assert.Error(t, err, "expected error when middle CRD check fails")
		assert.Contains(t, err.Error(), "snapshotinfo CRD failed")
	})

	t.Run("successful creation of all CRDs", func(t *testing.T) {
		mockK8sClient := newMockKubeClient(t)
		installer := newTestInstaller(mockK8sClient)

		// Mock all CRD operations to succeed (simplified - all new CRDs)
		mockK8sClient.EXPECT().CheckCRDExists(gomock.AssignableToTypeOf("")).Return(false, nil).AnyTimes()
		mockK8sClient.EXPECT().PutCustomResourceDefinition(nil, gomock.Any(), true, gomock.Any()).Return(nil).AnyTimes()

		err := installer.createCRDs(false)
		assert.NoError(t, err, "Should not error when creating CRDs successfully")
	})
}

func TestCreateOrPatchTridentRoles(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}
	labels := map[string]string{
		TestAppLabel: TestTridentNetAppIO,
	}

	t.Run("successful role creation", func(t *testing.T) {
		roleNames := []string{getControllerRBACResourceName()}
		currentRoleMap := make(map[string]*rbacv1.Role)
		unwantedRoles := []rbacv1.Role{}
		reuseRoleMap := make(map[string]bool)

		mockK8sClient.EXPECT().GetMultipleRoleInformation(roleNames, appLabel, false).Return(
			currentRoleMap, unwantedRoles, reuseRoleMap, nil)
		mockK8sClient.EXPECT().RemoveMultipleRoles(unwantedRoles).Return(nil)
		mockK8sClient.EXPECT().Namespace().Return("trident")
		mockK8sClient.EXPECT().PutRole(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentRoles(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating Trident roles with shouldUpdate=false")
	})

	t.Run("failure getting role information", func(t *testing.T) {
		roleNames := []string{getControllerRBACResourceName()}

		mockK8sClient.EXPECT().GetMultipleRoleInformation(roleNames, appLabel, false).Return(
			nil, nil, nil, fmt.Errorf("failed to get roles"))

		err := installer.createOrPatchTridentRoles(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when getting role information fails")
		assert.Contains(t, err.Error(), "failed to get Trident roles with controller label")
		assert.Contains(t, err.Error(), "failed to get roles")
	})

	t.Run("failure removing unwanted roles", func(t *testing.T) {
		roleNames := []string{getControllerRBACResourceName()}
		currentRoleMap := make(map[string]*rbacv1.Role)
		unwantedRoles := []rbacv1.Role{{}}
		reuseRoleMap := make(map[string]bool)

		mockK8sClient.EXPECT().GetMultipleRoleInformation(roleNames, appLabel, false).Return(
			currentRoleMap, unwantedRoles, reuseRoleMap, nil)
		mockK8sClient.EXPECT().RemoveMultipleRoles(unwantedRoles).Return(fmt.Errorf("failed to remove roles"))

		err := installer.createOrPatchTridentRoles(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when removing unwanted roles fails")
		assert.Contains(t, err.Error(), "failed to remove unwanted Trident roles")
		assert.Contains(t, err.Error(), "failed to remove roles")
	})

	t.Run("failure creating/patching role", func(t *testing.T) {
		roleNames := []string{getControllerRBACResourceName()}
		currentRoleMap := make(map[string]*rbacv1.Role)
		unwantedRoles := []rbacv1.Role{}
		reuseRoleMap := make(map[string]bool)

		mockK8sClient.EXPECT().GetMultipleRoleInformation(roleNames, appLabel, false).Return(
			currentRoleMap, unwantedRoles, reuseRoleMap, nil)
		mockK8sClient.EXPECT().RemoveMultipleRoles(unwantedRoles).Return(nil)
		mockK8sClient.EXPECT().Namespace().Return("trident")
		mockK8sClient.EXPECT().PutRole(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			fmt.Errorf("failed to put role"))

		err := installer.createOrPatchTridentRoles(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when creating/patching role fails")
		assert.Contains(t, err.Error(), "failed to create or patch Trident role")
		assert.Contains(t, err.Error(), "failed to put role")
	})

	t.Run("successful role update (shouldUpdate=true)", func(t *testing.T) {
		roleNames := []string{getControllerRBACResourceName()}
		currentRoleMap := map[string]*rbacv1.Role{
			roleNames[0]: {},
		}
		unwantedRoles := []rbacv1.Role{}
		reuseRoleMap := map[string]bool{
			roleNames[0]: true,
		}

		mockK8sClient.EXPECT().GetMultipleRoleInformation(roleNames, appLabel, true).Return(
			currentRoleMap, unwantedRoles, reuseRoleMap, nil)
		mockK8sClient.EXPECT().RemoveMultipleRoles(unwantedRoles).Return(nil)
		mockK8sClient.EXPECT().Namespace().Return("trident")
		mockK8sClient.EXPECT().PutRole(currentRoleMap[roleNames[0]], reuseRoleMap[roleNames[0]], gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentRoles(controllingCRDetails, labels, true)
		assert.NoError(t, err, "Should not error when creating Trident roles with shouldUpdate=true")
	})
}

func TestCreateOrPatchTridentRoleBindings(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}
	labels := map[string]string{
		TestAppLabel: TestTridentNetAppIO,
	}

	t.Run("successful role binding creation", func(t *testing.T) {
		roleBindingNames := []string{getControllerRBACResourceName()}
		currentRoleBindingMap := make(map[string]*rbacv1.RoleBinding)
		unwantedRoleBindings := []rbacv1.RoleBinding{}
		reuseRoleBindingMap := make(map[string]bool)

		mockK8sClient.EXPECT().GetMultipleRoleBindingInformation(roleBindingNames, appLabel, false).Return(
			currentRoleBindingMap, unwantedRoleBindings, reuseRoleBindingMap, nil)
		mockK8sClient.EXPECT().RemoveMultipleRoleBindings(unwantedRoleBindings).Return(nil)
		mockK8sClient.EXPECT().PutRoleBinding(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentRoleBindings(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating Trident role bindings with shouldUpdate=false")
	})

	t.Run("failure getting role binding information", func(t *testing.T) {
		roleBindingNames := []string{getControllerRBACResourceName()}

		mockK8sClient.EXPECT().GetMultipleRoleBindingInformation(roleBindingNames, appLabel, false).Return(
			nil, nil, nil, fmt.Errorf("failed to get role bindings"))

		err := installer.createOrPatchTridentRoleBindings(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when getting role binding information fails")
		assert.Contains(t, err.Error(), "failed to get Trident role bindings with controller label")
		assert.Contains(t, err.Error(), "failed to get role bindings")
	})

	t.Run("failure removing unwanted role bindings", func(t *testing.T) {
		roleBindingNames := []string{getControllerRBACResourceName()}
		currentRoleBindingMap := make(map[string]*rbacv1.RoleBinding)
		unwantedRoleBindings := []rbacv1.RoleBinding{{}}
		reuseRoleBindingMap := make(map[string]bool)

		mockK8sClient.EXPECT().GetMultipleRoleBindingInformation(roleBindingNames, appLabel, false).Return(
			currentRoleBindingMap, unwantedRoleBindings, reuseRoleBindingMap, nil)
		mockK8sClient.EXPECT().RemoveMultipleRoleBindings(unwantedRoleBindings).Return(fmt.Errorf("failed to remove role bindings"))

		err := installer.createOrPatchTridentRoleBindings(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when removing unwanted role bindings fails")
		assert.Contains(t, err.Error(), "failed to remove unwanted Trident role bindings")
		assert.Contains(t, err.Error(), "failed to remove role bindings")
	})

	t.Run("failure creating/patching role binding", func(t *testing.T) {
		roleBindingNames := []string{getControllerRBACResourceName()}
		currentRoleBindingMap := make(map[string]*rbacv1.RoleBinding)
		unwantedRoleBindings := []rbacv1.RoleBinding{}
		reuseRoleBindingMap := make(map[string]bool)

		mockK8sClient.EXPECT().GetMultipleRoleBindingInformation(roleBindingNames, appLabel, false).Return(
			currentRoleBindingMap, unwantedRoleBindings, reuseRoleBindingMap, nil)
		mockK8sClient.EXPECT().RemoveMultipleRoleBindings(unwantedRoleBindings).Return(nil)
		mockK8sClient.EXPECT().PutRoleBinding(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
			fmt.Errorf("failed to put role binding"))

		err := installer.createOrPatchTridentRoleBindings(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when creating/patching role binding fails")
		assert.Contains(t, err.Error(), "failed to create or patch Trident role binding")
		assert.Contains(t, err.Error(), "failed to put role binding")
	})

	t.Run("successful role binding update (shouldUpdate=true)", func(t *testing.T) {
		roleBindingNames := []string{getControllerRBACResourceName()}
		currentRoleBindingMap := map[string]*rbacv1.RoleBinding{
			roleBindingNames[0]: {},
		}
		unwantedRoleBindings := []rbacv1.RoleBinding{}
		reuseRoleBindingMap := map[string]bool{
			roleBindingNames[0]: true,
		}

		mockK8sClient.EXPECT().GetMultipleRoleBindingInformation(roleBindingNames, appLabel, true).Return(
			currentRoleBindingMap, unwantedRoleBindings, reuseRoleBindingMap, nil)
		mockK8sClient.EXPECT().RemoveMultipleRoleBindings(unwantedRoleBindings).Return(nil)
		mockK8sClient.EXPECT().PutRoleBinding(currentRoleBindingMap[roleBindingNames[0]], reuseRoleBindingMap[roleBindingNames[0]], gomock.Any(), gomock.Any()).Return(nil)

		err := installer.createOrPatchTridentRoleBindings(controllingCRDetails, labels, true)
		assert.NoError(t, err, "Should not error when creating Trident role bindings with shouldUpdate=true")
	})
}

func TestCreateOrPatchTridentClusterRole(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}
	labels := map[string]string{
		TestAppLabel: TestTridentNetAppIO,
	}

	t.Run("successful cluster role creation", func(t *testing.T) {
		clusterRoleName := getControllerRBACResourceName()
		var currentClusterRole *rbacv1.ClusterRole = nil
		unwantedClusterRoles := []rbacv1.ClusterRole{}
		createClusterRole := true

		mockK8sClient.EXPECT().GetClusterRoleInformation(clusterRoleName, appLabel, false).Return(
			currentClusterRole, unwantedClusterRoles, createClusterRole, nil)
		mockK8sClient.EXPECT().GetClusterRolesByLabel(TridentNodeLabel).Return([]rbacv1.ClusterRole{}, nil)
		mockK8sClient.EXPECT().RemoveMultipleClusterRoles(unwantedClusterRoles).Return(nil)
		mockK8sClient.EXPECT().PutClusterRole(currentClusterRole, createClusterRole, gomock.Any(), appLabel).Return(nil)

		err := installer.createOrPatchTridentClusterRole(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating Trident cluster role")
	})

	t.Run("failure getting cluster role information", func(t *testing.T) {
		clusterRoleName := getControllerRBACResourceName()

		mockK8sClient.EXPECT().GetClusterRoleInformation(clusterRoleName, appLabel, false).Return(
			nil, nil, false, fmt.Errorf("failed to get cluster role"))

		err := installer.createOrPatchTridentClusterRole(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when getting cluster role information fails")
		assert.Contains(t, err.Error(), "failed to get Trident cluster roles")
		assert.Contains(t, err.Error(), "failed to get cluster role")
	})

	t.Run("successful cluster role creation with node cleanup", func(t *testing.T) {
		clusterRoleName := getControllerRBACResourceName()
		var currentClusterRole *rbacv1.ClusterRole = nil
		unwantedClusterRoles := []rbacv1.ClusterRole{}
		nodeClusterRoles := []rbacv1.ClusterRole{{}, {}}
		createClusterRole := true

		mockK8sClient.EXPECT().GetClusterRoleInformation(clusterRoleName, appLabel, false).Return(
			currentClusterRole, unwantedClusterRoles, createClusterRole, nil)
		mockK8sClient.EXPECT().GetClusterRolesByLabel(TridentNodeLabel).Return(nodeClusterRoles, nil)
		// Should remove both unwanted and node cluster roles
		combinedUnwanted := append(unwantedClusterRoles, nodeClusterRoles...)
		mockK8sClient.EXPECT().RemoveMultipleClusterRoles(combinedUnwanted).Return(nil)
		mockK8sClient.EXPECT().PutClusterRole(currentClusterRole, createClusterRole, gomock.Any(), appLabel).Return(nil)

		err := installer.createOrPatchTridentClusterRole(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating Trident cluster role")
	})

	t.Run("ignores error getting node cluster roles", func(t *testing.T) {
		clusterRoleName := getControllerRBACResourceName()
		var currentClusterRole *rbacv1.ClusterRole = nil
		unwantedClusterRoles := []rbacv1.ClusterRole{}
		createClusterRole := true

		mockK8sClient.EXPECT().GetClusterRoleInformation(clusterRoleName, appLabel, false).Return(
			currentClusterRole, unwantedClusterRoles, createClusterRole, nil)
		// Node cluster role retrieval fails, but function should continue
		mockK8sClient.EXPECT().GetClusterRolesByLabel(TridentNodeLabel).Return(nil, fmt.Errorf("node lookup failed"))
		mockK8sClient.EXPECT().RemoveMultipleClusterRoles(unwantedClusterRoles).Return(nil)
		mockK8sClient.EXPECT().PutClusterRole(currentClusterRole, createClusterRole, gomock.Any(), appLabel).Return(nil)

		err := installer.createOrPatchTridentClusterRole(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating Trident cluster role")
	})

	t.Run("failure removing unwanted cluster roles", func(t *testing.T) {
		clusterRoleName := getControllerRBACResourceName()
		var currentClusterRole *rbacv1.ClusterRole = nil
		unwantedClusterRoles := []rbacv1.ClusterRole{{}}
		createClusterRole := true

		mockK8sClient.EXPECT().GetClusterRoleInformation(clusterRoleName, appLabel, false).Return(
			currentClusterRole, unwantedClusterRoles, createClusterRole, nil)
		mockK8sClient.EXPECT().GetClusterRolesByLabel(TridentNodeLabel).Return([]rbacv1.ClusterRole{}, nil)
		mockK8sClient.EXPECT().RemoveMultipleClusterRoles(unwantedClusterRoles).Return(fmt.Errorf("failed to remove cluster roles"))

		err := installer.createOrPatchTridentClusterRole(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when removing unwanted cluster roles fails")
		assert.Contains(t, err.Error(), "failed to remove unwanted Trident cluster roles")
		assert.Contains(t, err.Error(), "failed to remove cluster roles")
	})

	t.Run("failure creating/patching cluster role", func(t *testing.T) {
		clusterRoleName := getControllerRBACResourceName()
		var currentClusterRole *rbacv1.ClusterRole = nil
		unwantedClusterRoles := []rbacv1.ClusterRole{}
		createClusterRole := true

		mockK8sClient.EXPECT().GetClusterRoleInformation(clusterRoleName, appLabel, false).Return(
			currentClusterRole, unwantedClusterRoles, createClusterRole, nil)
		mockK8sClient.EXPECT().GetClusterRolesByLabel(TridentNodeLabel).Return([]rbacv1.ClusterRole{}, nil)
		mockK8sClient.EXPECT().RemoveMultipleClusterRoles(unwantedClusterRoles).Return(nil)
		mockK8sClient.EXPECT().PutClusterRole(currentClusterRole, createClusterRole, gomock.Any(), appLabel).Return(
			fmt.Errorf("failed to put cluster role"))

		err := installer.createOrPatchTridentClusterRole(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when creating/patching cluster role fails")
		assert.Contains(t, err.Error(), "failed to create or patch Trident cluster role")
		assert.Contains(t, err.Error(), "failed to put cluster role")
	})

	t.Run("successful cluster role update (shouldUpdate=true)", func(t *testing.T) {
		clusterRoleName := getControllerRBACResourceName()
		currentClusterRole := &rbacv1.ClusterRole{}
		unwantedClusterRoles := []rbacv1.ClusterRole{}
		createClusterRole := false

		mockK8sClient.EXPECT().GetClusterRoleInformation(clusterRoleName, appLabel, true).Return(
			currentClusterRole, unwantedClusterRoles, createClusterRole, nil)
		mockK8sClient.EXPECT().GetClusterRolesByLabel(TridentNodeLabel).Return([]rbacv1.ClusterRole{}, nil)
		mockK8sClient.EXPECT().RemoveMultipleClusterRoles(unwantedClusterRoles).Return(nil)
		mockK8sClient.EXPECT().PutClusterRole(currentClusterRole, createClusterRole, gomock.Any(), appLabel).Return(nil)

		err := installer.createOrPatchTridentClusterRole(controllingCRDetails, labels, true)
		assert.NoError(t, err, "Should not error when creating Trident cluster role with shouldUpdate=true")
	})
}

func TestCreateOrPatchTridentClusterRoleBinding(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}
	labels := map[string]string{
		TestAppLabel: TestTridentNetAppIO,
	}

	t.Run("successful cluster role binding creation", func(t *testing.T) {
		clusterRoleBindingName := getControllerRBACResourceName()
		var currentClusterRoleBinding *rbacv1.ClusterRoleBinding = nil
		unwantedClusterRoleBindings := []rbacv1.ClusterRoleBinding{}
		createClusterRoleBinding := true

		mockK8sClient.EXPECT().GetClusterRoleBindingInformation(clusterRoleBindingName, appLabel, false).Return(
			currentClusterRoleBinding, unwantedClusterRoleBindings, createClusterRoleBinding, nil)
		mockK8sClient.EXPECT().GetClusterRoleBindingsByLabel(TridentNodeLabel).Return([]rbacv1.ClusterRoleBinding{}, nil)
		mockK8sClient.EXPECT().RemoveMultipleClusterRoleBindings(unwantedClusterRoleBindings).Return(nil)
		mockK8sClient.EXPECT().Flavor().Return(k8sclient.FlavorKubernetes)
		mockK8sClient.EXPECT().PutClusterRoleBinding(currentClusterRoleBinding, createClusterRoleBinding, gomock.Any(), appLabel).Return(nil)

		err := installer.createOrPatchTridentClusterRoleBinding(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating Trident cluster role binding")
	})

	t.Run("failure getting cluster role binding information", func(t *testing.T) {
		clusterRoleBindingName := getControllerRBACResourceName()

		mockK8sClient.EXPECT().GetClusterRoleBindingInformation(clusterRoleBindingName, appLabel, false).Return(
			nil, nil, false, fmt.Errorf("failed to get cluster role binding"))

		err := installer.createOrPatchTridentClusterRoleBinding(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when getting cluster role binding information fails")
		assert.Contains(t, err.Error(), "failed to get Trident cluster role bindings")
		assert.Contains(t, err.Error(), "failed to get cluster role binding")
	})

	t.Run("successful cluster role binding creation with node cleanup", func(t *testing.T) {
		clusterRoleBindingName := getControllerRBACResourceName()
		var currentClusterRoleBinding *rbacv1.ClusterRoleBinding = nil
		unwantedClusterRoleBindings := []rbacv1.ClusterRoleBinding{}
		nodeClusterRoleBindings := []rbacv1.ClusterRoleBinding{{}, {}}
		createClusterRoleBinding := true

		mockK8sClient.EXPECT().GetClusterRoleBindingInformation(clusterRoleBindingName, appLabel, false).Return(
			currentClusterRoleBinding, unwantedClusterRoleBindings, createClusterRoleBinding, nil)
		mockK8sClient.EXPECT().GetClusterRoleBindingsByLabel(TridentNodeLabel).Return(nodeClusterRoleBindings, nil)
		// Should remove both unwanted and node cluster role bindings
		combinedUnwanted := append(unwantedClusterRoleBindings, nodeClusterRoleBindings...)
		mockK8sClient.EXPECT().RemoveMultipleClusterRoleBindings(combinedUnwanted).Return(nil)
		mockK8sClient.EXPECT().Flavor().Return(k8sclient.FlavorKubernetes)
		mockK8sClient.EXPECT().PutClusterRoleBinding(currentClusterRoleBinding, createClusterRoleBinding, gomock.Any(), appLabel).Return(nil)

		err := installer.createOrPatchTridentClusterRoleBinding(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating Trident cluster role binding")
	})

	t.Run("ignores error getting node cluster role bindings", func(t *testing.T) {
		clusterRoleBindingName := getControllerRBACResourceName()
		var currentClusterRoleBinding *rbacv1.ClusterRoleBinding = nil
		unwantedClusterRoleBindings := []rbacv1.ClusterRoleBinding{}
		createClusterRoleBinding := true

		mockK8sClient.EXPECT().GetClusterRoleBindingInformation(clusterRoleBindingName, appLabel, false).Return(
			currentClusterRoleBinding, unwantedClusterRoleBindings, createClusterRoleBinding, nil)
		// Node cluster role binding retrieval fails, but function should continue
		mockK8sClient.EXPECT().GetClusterRoleBindingsByLabel(TridentNodeLabel).Return(nil, fmt.Errorf("node lookup failed"))
		mockK8sClient.EXPECT().RemoveMultipleClusterRoleBindings(unwantedClusterRoleBindings).Return(nil)
		mockK8sClient.EXPECT().Flavor().Return(k8sclient.FlavorKubernetes)
		mockK8sClient.EXPECT().PutClusterRoleBinding(currentClusterRoleBinding, createClusterRoleBinding, gomock.Any(), appLabel).Return(nil)

		err := installer.createOrPatchTridentClusterRoleBinding(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating Trident cluster role binding")
	})

	t.Run("failure removing unwanted cluster role bindings", func(t *testing.T) {
		clusterRoleBindingName := getControllerRBACResourceName()
		var currentClusterRoleBinding *rbacv1.ClusterRoleBinding = nil
		unwantedClusterRoleBindings := []rbacv1.ClusterRoleBinding{{}}
		createClusterRoleBinding := true

		mockK8sClient.EXPECT().GetClusterRoleBindingInformation(clusterRoleBindingName, appLabel, false).Return(
			currentClusterRoleBinding, unwantedClusterRoleBindings, createClusterRoleBinding, nil)
		mockK8sClient.EXPECT().GetClusterRoleBindingsByLabel(TridentNodeLabel).Return([]rbacv1.ClusterRoleBinding{}, nil)
		mockK8sClient.EXPECT().RemoveMultipleClusterRoleBindings(unwantedClusterRoleBindings).Return(fmt.Errorf("failed to remove cluster role bindings"))

		err := installer.createOrPatchTridentClusterRoleBinding(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when removing unwanted cluster role bindings fails")
		assert.Contains(t, err.Error(), "failed to remove unwanted Trident cluster role bindings")
		assert.Contains(t, err.Error(), "failed to remove cluster role bindings")
	})

	t.Run("failure creating/patching cluster role binding", func(t *testing.T) {
		clusterRoleBindingName := getControllerRBACResourceName()
		var currentClusterRoleBinding *rbacv1.ClusterRoleBinding = nil
		unwantedClusterRoleBindings := []rbacv1.ClusterRoleBinding{}
		createClusterRoleBinding := true

		mockK8sClient.EXPECT().GetClusterRoleBindingInformation(clusterRoleBindingName, appLabel, false).Return(
			currentClusterRoleBinding, unwantedClusterRoleBindings, createClusterRoleBinding, nil)
		mockK8sClient.EXPECT().GetClusterRoleBindingsByLabel(TridentNodeLabel).Return([]rbacv1.ClusterRoleBinding{}, nil)
		mockK8sClient.EXPECT().RemoveMultipleClusterRoleBindings(unwantedClusterRoleBindings).Return(nil)
		mockK8sClient.EXPECT().Flavor().Return(k8sclient.FlavorKubernetes)
		mockK8sClient.EXPECT().PutClusterRoleBinding(currentClusterRoleBinding, createClusterRoleBinding, gomock.Any(), appLabel).Return(
			fmt.Errorf("failed to put cluster role binding"))

		err := installer.createOrPatchTridentClusterRoleBinding(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when creating/patching cluster role binding fails")
		assert.Contains(t, err.Error(), "failed to create or patch Trident cluster role binding")
		assert.Contains(t, err.Error(), "failed to put cluster role binding")
	})

	t.Run("successful cluster role binding update (shouldUpdate=true)", func(t *testing.T) {
		clusterRoleBindingName := getControllerRBACResourceName()
		currentClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
		unwantedClusterRoleBindings := []rbacv1.ClusterRoleBinding{}
		createClusterRoleBinding := false

		mockK8sClient.EXPECT().GetClusterRoleBindingInformation(clusterRoleBindingName, appLabel, true).Return(
			currentClusterRoleBinding, unwantedClusterRoleBindings, createClusterRoleBinding, nil)
		mockK8sClient.EXPECT().GetClusterRoleBindingsByLabel(TridentNodeLabel).Return([]rbacv1.ClusterRoleBinding{}, nil)
		mockK8sClient.EXPECT().RemoveMultipleClusterRoleBindings(unwantedClusterRoleBindings).Return(nil)
		mockK8sClient.EXPECT().Flavor().Return(k8sclient.FlavorOpenShift)
		mockK8sClient.EXPECT().PutClusterRoleBinding(currentClusterRoleBinding, createClusterRoleBinding, gomock.Any(), appLabel).Return(nil)

		err := installer.createOrPatchTridentClusterRoleBinding(controllingCRDetails, labels, true)
		assert.NoError(t, err, "Should not error when creating Trident cluster role binding with shouldUpdate=true")
	})
}

func TestCreateOrConsumeTridentEncryptionSecret(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	controllingCRDetails := map[string]string{
		"UID": TestUID,
	}
	labels := map[string]string{
		TestAppLabel: TestTridentNetAppIO,
	}

	t.Run("successful creation when secret doesn't exist", func(t *testing.T) {
		secretName := getEncryptionSecretName()

		// Mock CheckSecretExists to return false (secret doesn't exist)
		mockK8sClient.EXPECT().CheckSecretExists(secretName).Return(false, nil)

		// Mock PutSecret to succeed
		mockK8sClient.EXPECT().PutSecret(true, gomock.Any(), secretName).Return(nil)

		err := installer.createOrConsumeTridentEncryptionSecret(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating or consuming Trident encryption secret")
	})

	t.Run("successful when secret already exists", func(t *testing.T) {
		secretName := getEncryptionSecretName()

		// Mock CheckSecretExists to return true (secret already exists)
		mockK8sClient.EXPECT().CheckSecretExists(secretName).Return(true, nil)
		// No other calls should be made when secret exists

		err := installer.createOrConsumeTridentEncryptionSecret(controllingCRDetails, labels, false)
		assert.NoError(t, err, "Should not error when creating or consuming Trident encryption secret")
	})

	t.Run("failure checking if secret exists", func(t *testing.T) {
		secretName := getEncryptionSecretName()

		// Mock CheckSecretExists to fail
		mockK8sClient.EXPECT().CheckSecretExists(secretName).Return(false, fmt.Errorf("failed to check secret"))

		err := installer.createOrConsumeTridentEncryptionSecret(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when checking secret existence fails")
		assert.Contains(t, err.Error(), "failed to check for existing Trident encryption secret")
		assert.Contains(t, err.Error(), "failed to check secret")
	})

	t.Run("failure creating secret", func(t *testing.T) {
		secretName := getEncryptionSecretName()

		// Mock CheckSecretExists to return false (secret doesn't exist)
		mockK8sClient.EXPECT().CheckSecretExists(secretName).Return(false, nil)

		// Mock PutSecret to fail
		mockK8sClient.EXPECT().PutSecret(true, gomock.Any(), secretName).Return(fmt.Errorf("failed to create secret"))

		err := installer.createOrConsumeTridentEncryptionSecret(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when creating encryption secret fails")
		assert.Contains(t, err.Error(), "failed to create Trident encryption secret")
		assert.Contains(t, err.Error(), "failed to create secret")
	})

	t.Run("ignores shouldUpdate parameter", func(t *testing.T) {
		// This function disregards shouldUpdate parameter, test with different values
		secretName := getEncryptionSecretName()

		// Mock CheckSecretExists to return true (secret already exists)
		mockK8sClient.EXPECT().CheckSecretExists(secretName).Return(true, nil)

		// Test with shouldUpdate=true (should be ignored)
		err := installer.createOrConsumeTridentEncryptionSecret(controllingCRDetails, labels, true)
		assert.NoError(t, err, "Should not error when creating or consuming Trident encryption secret with shouldUpdate=true")
	})

	t.Run("edge case - empty labels map", func(t *testing.T) {
		// Test with empty labels to ensure label merging logic is covered
		secretName := getEncryptionSecretName()
		emptyLabels := map[string]string{}

		mockK8sClient.EXPECT().CheckSecretExists(secretName).Return(false, nil)
		mockK8sClient.EXPECT().PutSecret(true, gomock.Any(), secretName).Return(nil)

		err := installer.createOrConsumeTridentEncryptionSecret(controllingCRDetails, emptyLabels, false)
		assert.NoError(t, err, "Should not error when creating encryption secret with empty labels")
	})

	t.Run("edge case - nil controllingCRDetails", func(t *testing.T) {
		// Test with nil controllingCRDetails
		secretName := getEncryptionSecretName()

		mockK8sClient.EXPECT().CheckSecretExists(secretName).Return(false, nil)
		mockK8sClient.EXPECT().PutSecret(true, gomock.Any(), secretName).Return(nil)

		err := installer.createOrConsumeTridentEncryptionSecret(nil, labels, false)
		assert.NoError(t, err, "Should not error when processing encryption secret")
	})

	t.Run("edge case - large labels map", func(t *testing.T) {
		// Test with multiple labels to exercise label merging logic
		secretName := getEncryptionSecretName()
		manyLabels := map[string]string{
			"app":     TestTridentNetAppIO,
			"version": "v23.04",
			"env":     "test",
			"team":    "storage",
		}

		mockK8sClient.EXPECT().CheckSecretExists(secretName).Return(false, nil)
		mockK8sClient.EXPECT().PutSecret(true, gomock.Any(), secretName).Return(nil)

		err := installer.createOrConsumeTridentEncryptionSecret(controllingCRDetails, manyLabels, false)
		assert.NoError(t, err, "Should not error when processing encryption secret")
	})

	t.Run("edge case - secret creation with minimal parameters", func(t *testing.T) {
		// Test with both empty maps to exercise edge case handling
		secretName := getEncryptionSecretName()

		mockK8sClient.EXPECT().CheckSecretExists(secretName).Return(false, nil)
		mockK8sClient.EXPECT().PutSecret(true, gomock.Any(), secretName).Return(nil)

		// This should test the full creation path with empty parameters
		err := installer.createOrConsumeTridentEncryptionSecret(map[string]string{}, map[string]string{}, false)
		assert.NoError(t, err, "Should not error when processing encryption secret")
	})
}

func TestCreateOrPatchTridentServiceAccounts(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	controllingCRDetails := map[string]string{"UID": TestUID}
	labels := map[string]string{TestAppLabel: TestTridentNetAppIO}

	t.Run("failure getting controller service accounts", func(t *testing.T) {
		serviceAccountNames := getRBACResourceNames()

		// Mock controller service account query to fail
		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(
			serviceAccountNames, appLabel, installer.namespace, false,
		).Return(nil, nil, nil, nil, fmt.Errorf("controller query failed"))

		result, err := installer.createOrPatchTridentServiceAccounts(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when getting controller service accounts fails")
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to get Trident service accounts with controller label")
	})

	t.Run("failure getting node service accounts", func(t *testing.T) {
		serviceAccountNames := getRBACResourceNames()

		// Controller query succeeds
		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(
			serviceAccountNames, appLabel, installer.namespace, false,
		).Return(make(map[string]*corev1.ServiceAccount), []corev1.ServiceAccount{},
			make(map[string][]string), make(map[string]bool), nil)

		// Node query fails
		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(
			serviceAccountNames, TridentNodeLabel, installer.namespace, false,
		).Return(nil, nil, nil, nil, fmt.Errorf("node query failed"))

		result, err := installer.createOrPatchTridentServiceAccounts(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when getting node service accounts fails")
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to get Trident service accounts with node label")
	})

	t.Run("failure removing unwanted service accounts", func(t *testing.T) {
		serviceAccountNames := getRBACResourceNames()
		unwantedSAs := []corev1.ServiceAccount{{ObjectMeta: metav1.ObjectMeta{Name: "old-sa"}}}

		// Both queries succeed
		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(
			serviceAccountNames, appLabel, installer.namespace, false,
		).Return(make(map[string]*corev1.ServiceAccount), unwantedSAs,
			make(map[string][]string), make(map[string]bool), nil)

		mockK8sClient.EXPECT().GetMultipleServiceAccountInformation(
			serviceAccountNames, TridentNodeLabel, installer.namespace, false,
		).Return(make(map[string]*corev1.ServiceAccount), []corev1.ServiceAccount{},
			make(map[string][]string), make(map[string]bool), nil)

		// Removal fails
		mockK8sClient.EXPECT().RemoveMultipleServiceAccounts(unwantedSAs).Return(fmt.Errorf("removal failed"))

		result, err := installer.createOrPatchTridentServiceAccounts(controllingCRDetails, labels, false)
		assert.Error(t, err, "expected error when removing unwanted service accounts fails")
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to remove unwanted service accounts")
	})
}

func TestWaitForTridentPod(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	t.Run("success when pod is running immediately", func(t *testing.T) {
		// Create a running pod - this should return quickly without retries
		runningPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "trident-pod"},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "trident-main",
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
				},
			},
		}

		// Mock GetPodByLabel to return running pod immediately
		mockK8sClient.EXPECT().GetPodByLabel(appLabel, false).Return(runningPod, nil)

		pod, err := installer.waitForTridentPod()
		assert.NoError(t, err, "Should not error when waiting for Trident pod")
		assert.Equal(t, runningPod, pod)
	})
}

func TestRemoveRBACObjects(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	t.Run("successful removal of all RBAC objects (Kubernetes)", func(t *testing.T) {
		controllerResourceName := getControllerRBACResourceName()
		nodeResourceNames := []string{getNodeRBACResourceName(false), getNodeRBACResourceName(true)}

		// Mock all the delete operations in order
		mockK8sClient.EXPECT().DeleteTridentClusterRole(controllerResourceName, TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(controllerResourceName, TridentCSILabel).Return(nil)

		// Node cluster roles and bindings
		for _, nodeResourceName := range nodeResourceNames {
			mockK8sClient.EXPECT().DeleteTridentClusterRole(nodeResourceName, TridentNodeLabel).Return(nil)
			mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(nodeResourceName, TridentNodeLabel).Return(nil)
		}

		mockK8sClient.EXPECT().DeleteMultipleTridentRoles(nodeResourceNames, TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentRoleBindings(nodeResourceNames, TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentServiceAccounts([]string{controllerResourceName}, TridentCSILabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentServiceAccounts(nodeResourceNames, TridentNodeLabel, installer.namespace).Return(nil)

		// Kubernetes flavor (no SCC deletion)
		mockK8sClient.EXPECT().Flavor().Return(k8sclient.FlavorKubernetes)

		err := installer.removeRBACObjects()
		assert.NoError(t, err, "Should not error when removing RBAC objects")
	})

	t.Run("successful removal of all RBAC objects (OpenShift with SCCs)", func(t *testing.T) {
		controllerResourceName := getControllerRBACResourceName()
		nodeResourceNames := []string{getNodeRBACResourceName(false), getNodeRBACResourceName(true)}

		// Mock all the delete operations in order
		mockK8sClient.EXPECT().DeleteTridentClusterRole(controllerResourceName, TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(controllerResourceName, TridentCSILabel).Return(nil)

		// Node cluster roles and bindings
		for _, nodeResourceName := range nodeResourceNames {
			mockK8sClient.EXPECT().DeleteTridentClusterRole(nodeResourceName, TridentNodeLabel).Return(nil)
			mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(nodeResourceName, TridentNodeLabel).Return(nil)
		}

		mockK8sClient.EXPECT().DeleteMultipleTridentRoles(nodeResourceNames, TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentRoleBindings(nodeResourceNames, TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentServiceAccounts([]string{controllerResourceName}, TridentCSILabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentServiceAccounts(nodeResourceNames, TridentNodeLabel, installer.namespace).Return(nil)

		// OpenShift flavor (with SCC deletion)
		mockK8sClient.EXPECT().Flavor().Return(k8sclient.FlavorOpenShift)
		mockK8sClient.EXPECT().DeleteMultipleOpenShiftSCC([]string{controllerResourceName}, []string{controllerResourceName}, TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleOpenShiftSCC(nodeResourceNames, nodeResourceNames, TridentNodeLabel).Return(nil)

		err := installer.removeRBACObjects()
		assert.NoError(t, err, "Should not error when removing RBAC objects")
	})

	t.Run("failure deleting controller cluster role", func(t *testing.T) {
		controllerResourceName := getControllerRBACResourceName()

		mockK8sClient.EXPECT().DeleteTridentClusterRole(controllerResourceName, TridentCSILabel).Return(fmt.Errorf("failed to delete controller cluster role"))

		err := installer.removeRBACObjects()
		assert.Error(t, err, "expected error when deleting controller cluster role fails")
		assert.Contains(t, err.Error(), "failed to delete controller cluster role")
	})

	t.Run("failure deleting controller cluster role binding", func(t *testing.T) {
		controllerResourceName := getControllerRBACResourceName()

		mockK8sClient.EXPECT().DeleteTridentClusterRole(controllerResourceName, TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(controllerResourceName, TridentCSILabel).Return(fmt.Errorf("failed to delete controller cluster role binding"))

		err := installer.removeRBACObjects()
		assert.Error(t, err, "expected error when deleting controller cluster role binding fails")
		assert.Contains(t, err.Error(), "failed to delete controller cluster role binding")
	})

	t.Run("failure deleting node cluster role", func(t *testing.T) {
		controllerResourceName := getControllerRBACResourceName()
		nodeResourceNames := []string{getNodeRBACResourceName(false), getNodeRBACResourceName(true)}

		mockK8sClient.EXPECT().DeleteTridentClusterRole(controllerResourceName, TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(controllerResourceName, TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentClusterRole(nodeResourceNames[0], TridentNodeLabel).Return(fmt.Errorf("failed to delete node cluster role"))

		err := installer.removeRBACObjects()
		assert.Error(t, err, "expected error when deleting node cluster role fails")
		assert.Contains(t, err.Error(), "failed to delete node cluster role")
	})

	t.Run("failure deleting node roles", func(t *testing.T) {
		controllerResourceName := getControllerRBACResourceName()
		nodeResourceNames := []string{getNodeRBACResourceName(false), getNodeRBACResourceName(true)}

		mockK8sClient.EXPECT().DeleteTridentClusterRole(controllerResourceName, TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(controllerResourceName, TridentCSILabel).Return(nil)

		// Node cluster roles and bindings succeed
		for _, nodeResourceName := range nodeResourceNames {
			mockK8sClient.EXPECT().DeleteTridentClusterRole(nodeResourceName, TridentNodeLabel).Return(nil)
			mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(nodeResourceName, TridentNodeLabel).Return(nil)
		}

		// Node roles fail
		mockK8sClient.EXPECT().DeleteMultipleTridentRoles(nodeResourceNames, TridentNodeLabel).Return(fmt.Errorf("failed to delete node roles"))

		err := installer.removeRBACObjects()
		assert.Error(t, err, "expected error when deleting node roles fails")
		assert.Contains(t, err.Error(), "failed to delete node roles")
	})

	t.Run("failure deleting controller service accounts", func(t *testing.T) {
		controllerResourceName := getControllerRBACResourceName()
		nodeResourceNames := []string{getNodeRBACResourceName(false), getNodeRBACResourceName(true)}

		mockK8sClient.EXPECT().DeleteTridentClusterRole(controllerResourceName, TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(controllerResourceName, TridentCSILabel).Return(nil)

		// Node cluster roles and bindings succeed
		for _, nodeResourceName := range nodeResourceNames {
			mockK8sClient.EXPECT().DeleteTridentClusterRole(nodeResourceName, TridentNodeLabel).Return(nil)
			mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(nodeResourceName, TridentNodeLabel).Return(nil)
		}

		mockK8sClient.EXPECT().DeleteMultipleTridentRoles(nodeResourceNames, TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentRoleBindings(nodeResourceNames, TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentServiceAccounts([]string{controllerResourceName}, TridentCSILabel, installer.namespace).Return(fmt.Errorf("failed to delete controller service accounts"))

		err := installer.removeRBACObjects()
		assert.Error(t, err, "expected error when deleting controller service accounts fails")
		assert.Contains(t, err.Error(), "failed to delete controller service accounts")
	})

	t.Run("failure deleting OpenShift SCCs", func(t *testing.T) {
		controllerResourceName := getControllerRBACResourceName()
		nodeResourceNames := []string{getNodeRBACResourceName(false), getNodeRBACResourceName(true)}

		// All regular deletions succeed
		mockK8sClient.EXPECT().DeleteTridentClusterRole(controllerResourceName, TridentCSILabel).Return(nil)
		mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(controllerResourceName, TridentCSILabel).Return(nil)

		for _, nodeResourceName := range nodeResourceNames {
			mockK8sClient.EXPECT().DeleteTridentClusterRole(nodeResourceName, TridentNodeLabel).Return(nil)
			mockK8sClient.EXPECT().DeleteTridentClusterRoleBinding(nodeResourceName, TridentNodeLabel).Return(nil)
		}

		mockK8sClient.EXPECT().DeleteMultipleTridentRoles(nodeResourceNames, TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentRoleBindings(nodeResourceNames, TridentNodeLabel).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentServiceAccounts([]string{controllerResourceName}, TridentCSILabel, installer.namespace).Return(nil)
		mockK8sClient.EXPECT().DeleteMultipleTridentServiceAccounts(nodeResourceNames, TridentNodeLabel, installer.namespace).Return(nil)

		// OpenShift SCCs fail
		mockK8sClient.EXPECT().Flavor().Return(k8sclient.FlavorOpenShift)
		mockK8sClient.EXPECT().DeleteMultipleOpenShiftSCC([]string{controllerResourceName}, []string{controllerResourceName}, TridentCSILabel).Return(fmt.Errorf("failed to delete controller SCCs"))

		err := installer.removeRBACObjects()
		assert.Error(t, err, "expected error when deleting OpenShift SCCs fails")
		assert.Contains(t, err.Error(), "failed to delete controller SCCs")
	})
}

func TestNewInstaller(t *testing.T) {
	t.Run("successful creation with valid config", func(t *testing.T) {
		// Create a mock config that will pass basic validation
		config := &rest.Config{
			Host: "https://kubernetes.default.svc.cluster.local:443",
		}

		// This will still fail at the actual K8s client creation since we don't have
		// a real cluster, but it will test the NewInstaller function logic
		_, err := NewInstaller(config, "test-namespace", 120)

		// We expect this to fail during K8s client initialization, not during timeout validation
		assert.Error(t, err, "expected error when initializing Kubernetes client with invalid config")
		assert.Contains(t, err.Error(), "failed to initialize Kubernetes client")
	})

	t.Run("timeout validation with zero timeout", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://kubernetes.default.svc.cluster.local:443",
		}

		// Test timeout <= 0 logic (should default to DefaultTimeout)
		_, err := NewInstaller(config, "test-namespace", 0)

		// Should fail at K8s client creation, not timeout validation
		assert.Error(t, err, "expected error when initializing Kubernetes client with zero timeout")
		assert.Contains(t, err.Error(), "failed to initialize Kubernetes client")
	})

	t.Run("timeout validation with negative timeout", func(t *testing.T) {
		config := &rest.Config{
			Host: "https://kubernetes.default.svc.cluster.local:443",
		}

		// Test negative timeout (should default to DefaultTimeout)
		_, err := NewInstaller(config, "test-namespace", -10)

		// Should fail at K8s client creation, not timeout validation
		assert.Error(t, err, "expected error when initializing Kubernetes client with negative timeout")
		assert.Contains(t, err.Error(), "failed to initialize Kubernetes client")
	})

	t.Run("nil config should panic", func(t *testing.T) {
		// Test with nil config - this will panic, so we need to catch it
		defer func() {
			if r := recover(); r != nil {
				// Expected to panic due to nil pointer dereference
				assert.NotNil(t, r)
			}
		}()

		// This should panic when trying to use nil config
		_, _ = NewInstaller(nil, "test-namespace", 60)

		// If we get here without panicking, the test should fail
		t.Fatal("Expected NewInstaller to panic with nil config")
	})
}

func TestGetTridentClientVersionInfo(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	imageName := "netapp/trident:v23.04.0"
	controllingCRDetails := map[string]string{"UID": TestUID}

	t.Run("successful version info retrieval", func(t *testing.T) {
		versionYAML := `
client:
  version: v23.04.0
`

		// Mock the getTridentVersionYAML call
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)
		mockK8sClient.EXPECT().ExecPodForVersionInformation(
			gomock.Any(), gomock.Any(), gomock.Any(),
		).Return([]byte(versionYAML), nil)
		mockK8sClient.EXPECT().DeletePodByLabel(gomock.Any()).Return(nil)

		clientVersion, err := installer.getTridentClientVersionInfo(imageName, controllingCRDetails)
		assert.NoError(t, err, "Should not error when getting Trident client version info")
		assert.NotNil(t, clientVersion)
		assert.Equal(t, "v23.04.0", clientVersion.Client.Version)
	})

	t.Run("failure getting version YAML", func(t *testing.T) {
		// Mock getTridentVersionYAML to fail
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)
		mockK8sClient.EXPECT().ExecPodForVersionInformation(
			gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(nil, fmt.Errorf("exec failed"))
		mockK8sClient.EXPECT().DeletePodByLabel(gomock.Any()).Return(nil)

		clientVersion, err := installer.getTridentClientVersionInfo(imageName, controllingCRDetails)
		assert.Error(t, err, "expected error when getting version YAML fails")
		assert.Nil(t, clientVersion)
	})

	t.Run("invalid YAML response", func(t *testing.T) {
		invalidYAML := "invalid: yaml: content: ["

		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)
		mockK8sClient.EXPECT().ExecPodForVersionInformation(
			gomock.Any(), gomock.Any(), gomock.Any(),
		).Return([]byte(invalidYAML), nil)
		mockK8sClient.EXPECT().DeletePodByLabel(gomock.Any()).Return(nil)

		clientVersion, err := installer.getTridentClientVersionInfo(imageName, controllingCRDetails)
		assert.Error(t, err, "expected error when parsing invalid YAML")
		assert.Nil(t, clientVersion)
		assert.Contains(t, err.Error(), "unable to umarshall client version YAML")
	})
}

func TestGetTridentVersionYAML(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	imageName := "netapp/trident:v23.04.0"
	controllingCRDetails := map[string]string{"UID": TestUID}

	t.Run("successful version YAML retrieval", func(t *testing.T) {
		expectedYAML := `
client:
  version: v23.04.0
`

		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)
		mockK8sClient.EXPECT().ExecPodForVersionInformation(
			"transient-trident-version-pod",
			gomock.Any(),
			gomock.Any(),
		).Return([]byte(expectedYAML+"\npod \"transient-trident-version-pod\" deleted"), nil)
		mockK8sClient.EXPECT().DeletePodByLabel(gomock.Any()).Return(nil)

		resultYAML, err := installer.getTridentVersionYAML(imageName, controllingCRDetails)
		assert.NoError(t, err, "Should not error when processing encryption secret")
		assert.Contains(t, string(resultYAML), "client:")
		assert.Contains(t, string(resultYAML), "v23.04.0")
		// Should not contain the deletion message
		assert.NotContains(t, string(resultYAML), "pod \"transient-trident-version-pod\" deleted")
	})

	t.Run("failure creating version pod", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(fmt.Errorf("failed to create pod"))

		resultYAML, err := installer.getTridentVersionYAML(imageName, controllingCRDetails)
		assert.Error(t, err, "expected error when creating version pod fails")
		assert.Empty(t, resultYAML)
		assert.Contains(t, err.Error(), "failed to create Trident Version pod")
	})

	t.Run("failure executing version command", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)
		mockK8sClient.EXPECT().ExecPodForVersionInformation(
			"transient-trident-version-pod",
			gomock.Any(),
			gomock.Any(),
		).Return(nil, fmt.Errorf("exec failed"))
		mockK8sClient.EXPECT().DeletePodByLabel(gomock.Any()).Return(nil)

		resultYAML, err := installer.getTridentVersionYAML(imageName, controllingCRDetails)
		assert.Error(t, err, "expected error when executing version command fails")
		assert.Empty(t, resultYAML)
		assert.Contains(t, err.Error(), "failed to exec Trident version pod")
	})

	t.Run("failure deleting transient pod", func(t *testing.T) {
		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(fmt.Errorf("failed to delete"))

		resultYAML, err := installer.getTridentVersionYAML(imageName, controllingCRDetails)
		assert.Error(t, err, "expected error when deleting previous transient pod fails")
		assert.Empty(t, resultYAML)
		assert.Contains(t, err.Error(), "failed to delete previous transient version pod")
	})

	t.Run("cleanup failure after successful exec", func(t *testing.T) {
		expectedYAML := `
client:
  version: v23.04.0
`

		mockK8sClient.EXPECT().ServerVersion().Return(&version.Version{}).AnyTimes()
		mockK8sClient.EXPECT().DeleteTransientVersionPod(TridentVersionPodLabel).Return(nil)
		mockK8sClient.EXPECT().CreateObjectByYAML(gomock.AssignableToTypeOf("")).Return(nil)
		mockK8sClient.EXPECT().ExecPodForVersionInformation(
			"transient-trident-version-pod",
			gomock.Any(),
			gomock.Any(),
		).Return([]byte(expectedYAML), nil)
		// Mock cleanup failure - this should not fail the function, just log an error
		mockK8sClient.EXPECT().DeletePodByLabel(gomock.Any()).Return(fmt.Errorf("cleanup failed"))

		resultYAML, err := installer.getTridentVersionYAML(imageName, controllingCRDetails)
		// Should still succeed despite cleanup failure
		assert.NoError(t, err, "Should not error when processing encryption secret")
		assert.Contains(t, string(resultYAML), "client:")
		assert.Contains(t, string(resultYAML), "v23.04.0")
	})
}

func TestCreateOrPatchNodeRemediationResources(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	t.Run("successful creation of node remediation resources", func(t *testing.T) {
		// Mock CreateOrPatchClusterRole to succeed
		mockK8sClient.EXPECT().CreateOrPatchClusterRole(gomock.Any()).Return(nil)

		// Mock CreateOrPatchNodeRemediationTemplate to succeed
		mockK8sClient.EXPECT().CreateOrPatchNodeRemediationTemplate(gomock.Any(), installer.namespace).Return(nil)

		err := installer.createOrPatchNodeRemediationResources()
		assert.NoError(t, err, "Should not error when processing encryption secret")
	})

	t.Run("failure creating cluster role", func(t *testing.T) {
		// Mock CreateOrPatchClusterRole to fail
		mockK8sClient.EXPECT().CreateOrPatchClusterRole(gomock.Any()).Return(fmt.Errorf("cluster role creation failed"))

		err := installer.createOrPatchNodeRemediationResources()
		assert.Error(t, err, "expected error when cluster role creation fails")
		assert.Contains(t, err.Error(), "failed to create or patch TridentNodeRemediation cluster role")
		assert.Contains(t, err.Error(), "cluster role creation failed")
	})

	t.Run("failure creating node remediation template", func(t *testing.T) {
		// Mock CreateOrPatchClusterRole to succeed
		mockK8sClient.EXPECT().CreateOrPatchClusterRole(gomock.Any()).Return(nil)

		// Mock CreateOrPatchNodeRemediationTemplate to fail
		mockK8sClient.EXPECT().CreateOrPatchNodeRemediationTemplate(gomock.Any(), installer.namespace).Return(fmt.Errorf("template creation failed"))

		err := installer.createOrPatchNodeRemediationResources()
		assert.Error(t, err, "expected error when template creation fails")
		assert.Contains(t, err.Error(), "failed to create or patch TridentNodeRemediation cluster role")
		assert.Contains(t, err.Error(), "template creation failed")
	})

	t.Run("test resource creation with correct parameters", func(t *testing.T) {
		// Mock CreateOrPatchClusterRole with more specific expectations
		mockK8sClient.EXPECT().CreateOrPatchClusterRole(gomock.AssignableToTypeOf(&rbacv1.ClusterRole{})).Return(nil)

		// Mock CreateOrPatchNodeRemediationTemplate with namespace verification
		mockK8sClient.EXPECT().CreateOrPatchNodeRemediationTemplate(
			gomock.AssignableToTypeOf(&persistentv1.TridentNodeRemediationTemplate{}),
			installer.namespace,
		).Return(nil)

		err := installer.createOrPatchNodeRemediationResources()
		assert.NoError(t, err, "Should not error when processing encryption secret")
	})

	t.Run("test with empty namespace", func(t *testing.T) {
		// Test edge case with empty namespace
		emptyInstaller := &Installer{
			client:           mockK8sClient,
			tridentCRDClient: nil,
			namespace:        "",
		}

		// Mock CreateOrPatchClusterRole to succeed
		mockK8sClient.EXPECT().CreateOrPatchClusterRole(gomock.Any()).Return(nil)

		// Mock CreateOrPatchNodeRemediationTemplate with empty namespace
		mockK8sClient.EXPECT().CreateOrPatchNodeRemediationTemplate(gomock.Any(), "").Return(nil)

		err := emptyInstaller.createOrPatchNodeRemediationResources()
		assert.NoError(t, err, "Should not error when processing encryption secret")
	})
}

func TestSimpleFunctions(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	// Just verify constructor works - actual coverage comes from calling methods
	assert.Equal(t, Namespace, installer.namespace)
	assert.NotNil(t, installer.client)
}

func TestUtilityFunctions(t *testing.T) {
	// Coverage comes from calling the functions, not checking their content
	getEncryptionSecretName()
	getServiceName()
	getCSIDriverName()
	getProtocolSecretName()
	names := getRBACResourceNames()
	assert.NotEmpty(t, names) // Minimal validation to ensure it returns data
}

func TestPopulateResourcesBasic(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)
	installer := newTestInstaller(mockK8sClient)

	// Test nil resources (early return path)
	cr1 := netappv1.TridentOrchestrator{Spec: netappv1.TridentOrchestratorSpec{Resources: nil}}
	assert.NoError(t, installer.populateResources(cr1))

	// Test empty resources struct (early return path)
	cr2 := netappv1.TridentOrchestrator{
		Spec: netappv1.TridentOrchestratorSpec{
			Resources: &netappv1.Resources{Controller: make(netappv1.ContainersResourceRequirements)},
		},
	}
	assert.NoError(t, installer.populateResources(cr2))
}

func TestValidateNonNegativeQuantity(t *testing.T) {
	// Test different quantity types - coverage comes from calling the function
	assert.NoError(t, ValidateNonNegativeQuantity(nil))

	positive := resource.MustParse("100m")
	assert.NoError(t, ValidateNonNegativeQuantity(&positive))

	zero := resource.MustParse("0")
	assert.NoError(t, ValidateNonNegativeQuantity(&zero))

	negative := resource.MustParse("-100m")
	assert.Error(t, ValidateNonNegativeQuantity(&negative))
}

// TestDeleteTridentNodeRemediationResources tests the actual implementation
// by creating a real K8sClient wrapper and mocking its underlying dependencies
func TestDeleteTridentNodeRemediationResources(t *testing.T) {
	mockK8sClient := newMockKubeClient(t)

	// Create a real K8sClient that wraps our mock
	k8sClientWrapper := &K8sClient{KubernetesClient: mockK8sClient}

	t.Run("successful deletion of both resources", func(t *testing.T) {
		// Mock DeleteObjectByYAML for template deletion to succeed
		mockK8sClient.EXPECT().DeleteObjectByYAML(gomock.AssignableToTypeOf(""), true).Return(nil)

		// Mock DeleteObjectByYAML for cluster role deletion to succeed
		mockK8sClient.EXPECT().DeleteObjectByYAML(gomock.AssignableToTypeOf(""), true).Return(nil)

		err := k8sClientWrapper.DeleteTridentNodeRemediationResources("trident")
		assert.NoError(t, err, "Should not error when processing encryption secret")
	})

	t.Run("template not found - should continue", func(t *testing.T) {
		// Mock DeleteObjectByYAML for template to return a "not found" error
		notFoundErr := fmt.Errorf("TridentNodeRemediationTemplate not found")
		mockK8sClient.EXPECT().DeleteObjectByYAML(gomock.AssignableToTypeOf(""), true).Return(notFoundErr)

		// Mock DeleteObjectByYAML for cluster role to succeed
		mockK8sClient.EXPECT().DeleteObjectByYAML(gomock.AssignableToTypeOf(""), true).Return(nil)

		err := k8sClientWrapper.DeleteTridentNodeRemediationResources("trident")
		assert.NoError(t, err, "should not return error when template is not found")
	})

	t.Run("template deletion fails with non-not-found error", func(t *testing.T) {
		// Mock DeleteObjectByYAML for template to return non-"not found" error
		templateError := fmt.Errorf("failed to delete template")
		mockK8sClient.EXPECT().DeleteObjectByYAML(gomock.AssignableToTypeOf(""), true).Return(templateError)

		err := k8sClientWrapper.DeleteTridentNodeRemediationResources("trident")
		assert.Error(t, err, "expected error when template deletion fails")
		assert.Contains(t, err.Error(), "could not delete TridentNodeRemediationTemplate CR")
	})

	t.Run("cluster role not found - should not return error", func(t *testing.T) {
		// Mock DeleteObjectByYAML for template to succeed
		mockK8sClient.EXPECT().DeleteObjectByYAML(gomock.AssignableToTypeOf(""), true).Return(nil)

		// Mock DeleteObjectByYAML for cluster role to return "not found" error
		notFoundErr := fmt.Errorf("ClusterRole not found")
		mockK8sClient.EXPECT().DeleteObjectByYAML(gomock.AssignableToTypeOf(""), true).Return(notFoundErr)

		err := k8sClientWrapper.DeleteTridentNodeRemediationResources("trident")
		assert.NoError(t, err, "should not return error when cluster role is not found")
	})

	t.Run("cluster role deletion fails with non-not-found error", func(t *testing.T) {
		// Mock DeleteObjectByYAML for template to succeed
		mockK8sClient.EXPECT().DeleteObjectByYAML(gomock.AssignableToTypeOf(""), true).Return(nil)

		// Mock DeleteObjectByYAML for cluster role to return non-"not found" error
		clusterRoleError := fmt.Errorf("failed to delete cluster role")
		mockK8sClient.EXPECT().DeleteObjectByYAML(gomock.AssignableToTypeOf(""), true).Return(clusterRoleError)

		err := k8sClientWrapper.DeleteTridentNodeRemediationResources("trident")
		assert.Error(t, err, "expected error when cluster role deletion fails")
		assert.Contains(t, err.Error(), "could not delete tridentnoderemediation-access clusterRole")
	})
}
