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

// newMockKubeClient creates a new mock kubernetes client
// Parameters:
//    t *testing.T - test object
// Returns:
//    *mockExtendedK8sClient.MockExtendedK8sClient - mock kubernetes client
// It returns a mock kubernetes client
// Example:
//     mockK8sClient := newMockKubeClient(t)

func newMockKubeClient(t *testing.T) *mockExtendedK8sClient.MockExtendedK8sClient {
	mockCtrl := gomock.NewController(t)
	mockK8sClient := mockExtendedK8sClient.NewMockExtendedK8sClient(mockCtrl)
	return mockK8sClient
}

// newTestInstaller creates a new Installer for testing purposes.
// Parameters:
//   client - a mock client for the installer to use
// It returns the new Installer.
// Example:
//   client := mockExtendedK8sClient.NewMockClient()
//   installer := newTestInstaller(client)

func newTestInstaller(client *mockExtendedK8sClient.MockExtendedK8sClient) *Installer {

	return &Installer{
		client:           client,
		tridentCRDClient: nil,
		namespace:        Namespace,
	}
}

// createTestControllingCRDetails creates a map of the controlling CR details
// It returns a map of the controlling CR details
// Example:
//   controllingCRDetails := createTestControllingCRDetails()
//   fmt.Println(controllingCRDetails)
//
// Output:
//   map[trident.netapp.io/apiVersion:v01.01.01 trident.netapp.io/controller:trident-orchestrator trident.netapp.io/kind:deployment trident.netapp.io/name:trident-csi trident.netapp.io/uid:1]

func createTestControllingCRDetails() map[string]string {

	controllingCRDetails := make(map[string]string)
	controllingCRDetails[CRAPIVersionKey] = "v01.01.01"
	controllingCRDetails[CRController] = "trident-orchestrator"
	controllingCRDetails[CRKind] = "deployment"
	controllingCRDetails[CRName] = "trident-csi"
	controllingCRDetails[CRUID] = CRUID

	return controllingCRDetails
}

// createTestLabels creates a set of labels for testing
// It returns a map of labels
// Example:
//    labels := createTestLabels()

func createTestLabels() map[string]string {

	labels := make(map[string]string)
	labels[appLabelKey] = appLabelValue
	labels[K8sVersionLabelKey] = "v1.21.8"
	labels[TridentVersionLabelKey] = "v22.04.0"

	return labels
}

// TestInstaller_createOrConsumeTridentEncryptionSecret tests the creation of the Trident encryption secret
// It checks for the following conditions:
// 1. K8s error at CheckSecretExists
// 2. Secret already exists and no K8s error
// 3. Create secret with no failure at PutSecret
// 4. Create secret with a failure at PutSecret
// Parameters:
//   t *testing.T
// Returns:
//   none
// Example:
//   TestInstaller_createOrConsumeTridentEncryptionSecret(t)

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
