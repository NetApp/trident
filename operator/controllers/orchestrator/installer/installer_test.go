// Copyright 2023 NetApp, Inc. All Rights Reserved.

package installer

import (
	"fmt"
	"testing"

	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	mockExtendedK8sClient "github.com/netapp/trident/mocks/mock_operator/mock_controllers/mock_orchestrator/mock_installer"
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
	labels[TridentVersionLabelKey] = "v25.02.0"

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
