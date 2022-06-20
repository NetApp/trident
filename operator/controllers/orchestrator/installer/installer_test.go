// Copyright 2022 NetApp, Inc. All Rights Reserved.

package installer

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mockExtendedK8sClient "github.com/netapp/trident/mocks/mock_operator/mock_controllers/mock_orchestrator/mock_installer"
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
	labels[K8sVersionLabelKey] = "v1.21.8"
	labels[TridentVersionLabelKey] = "v22.07.0"

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
