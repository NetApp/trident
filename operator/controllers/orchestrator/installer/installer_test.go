package installer

import (
	"fmt"
	"testing"

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
	labels[TridentVersionLabelKey] = "v22.04.0"

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
