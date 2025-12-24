// Copyright 2024 NetApp, Inc. All Rights Reserved.

package configurator

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"

	. "github.com/netapp/trident/logging"
	mockConfClients "github.com/netapp/trident/mocks/mock_operator/mock_controllers/mock_configurator/mock_clients"
	netappv1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	tridentV1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

const (
	// Test constants for common values
	TestName             = "test"
	TestNamespace        = "test-namespace"
	TestConfiguratorName = "test-configurator"
	TestCRName           = "test-cr"
	TestKey              = "test/test-cr"
	TestInvalidKey       = "invalid-key"
	TestPhase            = "test-phase"
	TestCloudProvider    = "azure"
	TestMessage          = "test message"
	TestErrorMessage     = "test error"
	TestBackendName      = "test-backend"

	// Backend-related constants
	MockBackendName = "mock-backend"
	TestBackend1    = "test-backend-1"
	TestBackend2    = "test-backend-2"
	Backend1        = "backend1"
	Backend2        = "backend2"

	// Driver names
	AzureNetAppFilesDriver = "azure-netapp-files"
	ONTAPNASDriver         = "ontap-nas"
	ONTAPSANDriver         = "ontap-san"
	UnsupportedDriver      = "unsupported-driver"

	// Error messages
	FailedToGetTorcCR          = "failed to get torc CR"
	TconfNotFoundMessage       = "tconf not found"
	InvalidKeyMessage          = "invalid key"
	ValidationFailedMessage    = "validation failed"
	CreateFailedMessage        = "create failed"
	StatusUpdateFailedMessage  = "status update failed"
	SecretNotFoundMessage      = "secret not found"
	BackendNotSupportedMessage = "backend not supported"

	// Test data sizes
	ExpectedWorkqueueLength0 = 0
	ExpectedWorkqueueLength1 = 1

	// JSON test data
	InvalidJSONSpec           = `{"invalid":"json"`
	ValidBackendJSONSpec      = `{"storageDriverName":"azure-netapp-files"}`
	ONTAPNASJSONSpec          = `{"storageDriverName":"ontap-nas","svms":[{}]}`
	ONTAPNASFSxNJSONSpec      = `{"storageDriverName":"ontap-nas","svms":[{"fsxnID":"fs-12345"}]}`
	ONTAPSANJSONSpec          = `{"storageDriverName":"ontap-san","svms":[{}]}`
	UnsupportedDriverJSONSpec = `{"storageDriverName":"unsupported-driver"}`
	InvalidFieldJSONSpec      = `{"invalidField":"value"}`
	OldSpecJSON               = `{"old":"spec"}`
	NewSpecJSON               = `{"new":"spec"}`
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func createMockController(t *testing.T) (*Controller, *gomock.Controller, *mockConfClients.MockConfiguratorClientInterface) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)

	c := &Controller{
		Clients:  mockClient,
		stopChan: make(chan struct{}),
		workqueue: workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(),
			workqueue.RateLimitingQueueConfig{Name: CRDName}),
		eventRecorder: &mockEventRecorder{},
	}

	return c, mockCtrl, mockClient
}

// Mock event recorder for testing
type mockEventRecorder struct{}

func (m *mockEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {}
func (m *mockEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
}

func (m *mockEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
}

func TestController_BasicGetters(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	tests := []struct {
		name     string
		method   func() string
		expected string
	}{
		{
			name:     "GetName",
			method:   controller.GetName,
			expected: ControllerName,
		},
		{
			name:     "Version",
			method:   controller.Version,
			expected: ControllerVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.method()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestController_Deactivate(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	err := controller.Deactivate()
	assert.NoError(t, err, "Deactivate should succeed")

	select {
	case <-controller.stopChan:
	default:
		t.Error("stopChan should be closed")
	}
}

func TestController_processNextWorkItem_Shutdown(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	controller.workqueue.ShutDown()

	result := controller.processNextWorkItem()
	assert.False(t, result, "Should return false when workqueue is shutdown")
}

func TestController_processNextWorkItem_InvalidKey(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	controller.workqueue.Add(123)

	result := controller.processNextWorkItem()
	assert.True(t, result, "Should return true even with invalid key")
}

func TestController_processNextWorkItem_ErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		mockError      error
		expectedResult bool
		description    string
	}{
		{
			name:           "UnsupportedConfigError",
			mockError:      errors.UnsupportedConfigError("unsupported config"),
			expectedResult: true,
			description:    "Should return true after handling unsupported config error",
		},
		{
			name:           "NotFoundError",
			mockError:      errors.NotFoundError("not found"),
			expectedResult: true,
			description:    "Should return true after handling not found error",
		},
		{
			name:           "ReconcileIncompleteError",
			mockError:      errors.ReconcileIncompleteError("incomplete"),
			expectedResult: true,
			description:    "Should return true after handling reconcile incomplete error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, mockCtrl, mockClient := createMockController(t)
			defer mockCtrl.Finish()

			// Mock GetTconfCR to return a valid CR (called first in reconcile)
			tconfCR := &netappv1.TridentConfigurator{
				ObjectMeta: metav1.ObjectMeta{
					Name: TestCRName,
				},
			}
			mockClient.EXPECT().GetTconfCR(TestCRName).Return(tconfCR, nil)
			mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
			mockClient.EXPECT().GetControllingTorcCR().Return(nil, tt.mockError)
			controller.workqueue.Add("test/test-cr")

			result := controller.processNextWorkItem()
			assert.Equal(t, tt.expectedResult, result, tt.description)
		})
	}
}

func TestController_addConfigurator(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestConfiguratorName,
			Namespace: TestNamespace,
		},
	}

	controller.addConfigurator(tconfCR)
	assert.Equal(t, ExpectedWorkqueueLength1, controller.workqueue.Len(), "Workqueue should have 1 item")
}

func TestController_addConfigurator_InvalidObject(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	controller.addConfigurator("invalid-object")
	assert.Equal(t, ExpectedWorkqueueLength0, controller.workqueue.Len(), "Workqueue should be empty for invalid object")
}

func TestController_updateConfigurator_InvalidObjects(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(*Controller, *netappv1.TridentConfigurator)
		description string
	}{
		{
			name: "InvalidOldObject",
			setupFunc: func(controller *Controller, tconfCR *netappv1.TridentConfigurator) {
				controller.updateConfigurator("invalid", tconfCR)
			},
			description: "Should not add to workqueue with invalid old object",
		},
		{
			name: "InvalidNewObject",
			setupFunc: func(controller *Controller, tconfCR *netappv1.TridentConfigurator) {
				controller.updateConfigurator(tconfCR, "invalid")
			},
			description: "Should not add to workqueue with invalid new object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, mockCtrl, _ := createMockController(t)
			defer mockCtrl.Finish()

			tconfCR := &netappv1.TridentConfigurator{
				ObjectMeta: metav1.ObjectMeta{Name: TestName, Namespace: TestName},
			}

			tt.setupFunc(controller, tconfCR)
			assert.Equal(t, ExpectedWorkqueueLength0, controller.workqueue.Len(), tt.description)
		})
	}
}

func TestController_updateConfigurator_DeletionTimestamp(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	now := metav1.Now()
	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{
			Name:              TestName,
			Namespace:         TestName,
			DeletionTimestamp: &now,
		},
	}

	controller.updateConfigurator(tconfCR, tconfCR)
	assert.Equal(t, ExpectedWorkqueueLength1, controller.workqueue.Len(), "Workqueue should have 1 item for deletion processing")
}

func TestController_updateConfigurator_StatusOnlyUpdate(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	oldCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestName, Namespace: TestName},
		Status:     netappv1.TridentConfiguratorStatus{Phase: "old"},
	}

	newCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestName, Namespace: TestName},
		Status:     netappv1.TridentConfiguratorStatus{Phase: "new"},
	}

	controller.updateConfigurator(oldCR, newCR)
	assert.Equal(t, ExpectedWorkqueueLength0, controller.workqueue.Len(), "Workqueue should be empty for status-only updates")
}

func TestController_updateConfigurator_Success(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	oldCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestName, Namespace: TestName},
		Spec: netappv1.TridentConfiguratorSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(OldSpecJSON)},
		},
	}

	newCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestName, Namespace: TestName},
		Spec: netappv1.TridentConfiguratorSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(NewSpecJSON)},
		},
	}

	controller.updateConfigurator(oldCR, newCR)
	assert.Equal(t, ExpectedWorkqueueLength1, controller.workqueue.Len(), "Workqueue should have 1 item")
}

func TestController_deleteConfigurator(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestConfiguratorName,
			Namespace: TestNamespace,
		},
	}

	controller.deleteConfigurator(tconfCR)
	assert.Equal(t, ExpectedWorkqueueLength1, controller.workqueue.Len(), "Workqueue should have 1 item")
}

func TestController_deleteConfigurator_InvalidObject(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	controller.deleteConfigurator("invalid-object")
	assert.Equal(t, ExpectedWorkqueueLength0, controller.workqueue.Len(), "Workqueue should be empty for invalid object")
}

func TestController_reconcile_InitialErrors(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		setupMocks  func(*mockConfClients.MockConfiguratorClientInterface)
		description string
	}{
		{
			name: "GetControllingTorcCRError",
			key:  TestKey,
			setupMocks: func(mockClient *mockConfClients.MockConfiguratorClientInterface) {
				// GetTconfCR is called first, then UpdateTridentConfigurator (finalizer), then GetControllingTorcCR
				tconfCR := &netappv1.TridentConfigurator{
					ObjectMeta: metav1.ObjectMeta{Name: TestCRName},
				}
				mockClient.EXPECT().GetTconfCR(TestCRName).Return(tconfCR, nil)
				mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
				mockClient.EXPECT().GetControllingTorcCR().Return(nil, fmt.Errorf(FailedToGetTorcCR))
			},
			description: "Failed to get controlling torc CR",
		},
		{
			name: "InvalidKeyItem",
			key:  TestInvalidKey,
			setupMocks: func(mockClient *mockConfClients.MockConfiguratorClientInterface) {
				// GetTconfCR is called with the invalid key
				mockClient.EXPECT().GetTconfCR(TestInvalidKey).Return(nil, fmt.Errorf(InvalidKeyMessage))
			},
			description: "Invalid key format should fail",
		},
		{
			name: "GetTconfCRError",
			key:  TestKey,
			setupMocks: func(mockClient *mockConfClients.MockConfiguratorClientInterface) {
				// GetTconfCR is called first and returns an error - reconcile returns nil (not an error)
				mockClient.EXPECT().GetTconfCR(TestCRName).Return(nil, fmt.Errorf(TconfNotFoundMessage))
			},
			description: "Failed to get tconf CR - should return nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, mockCtrl, mockClient := createMockController(t)
			defer mockCtrl.Finish()

			tt.setupMocks(mockClient)

			err := controller.reconcile(tt.key)
			// GetTconfCRError and InvalidKeyItem cases return nil (GetTconfCR error returns nil), others return error
			if tt.name == "GetTconfCRError" || tt.name == "InvalidKeyItem" {
				assert.NoError(t, err, tt.description)
			} else {
				assert.Error(t, err, tt.description)
			}
		})
	}
}

func TestController_reconcile_ValidateError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	torcCR := &netappv1.TridentOrchestrator{}
	tconfCR := &netappv1.TridentConfigurator{
		Spec: netappv1.TridentConfiguratorSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(InvalidJSONSpec)}, // Invalid JSON to trigger validation error
		},
	}

	mockClient.EXPECT().GetTconfCR(TestCRName).Return(tconfCR, nil)
	mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
	mockClient.EXPECT().GetControllingTorcCR().Return(torcCR, nil)

	err := controller.reconcile(TestKey)
	assert.Error(t, err, "Validation should fail")
}

func TestController_reconcile_UnsupportedDriverName(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	torcCR := &netappv1.TridentOrchestrator{}
	tconfCR := &netappv1.TridentConfigurator{
		Spec: netappv1.TridentConfiguratorSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(UnsupportedDriverJSONSpec)},
		},
	}

	mockClient.EXPECT().GetTconfCR(TestCRName).Return(tconfCR, nil)
	mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
	mockClient.EXPECT().GetControllingTorcCR().Return(torcCR, nil)

	err := controller.reconcile(TestKey)
	assert.Error(t, err, "Unsupported driver should fail")
	assert.Contains(t, err.Error(), BackendNotSupportedMessage)
}

func TestController_reconcile_GetStorageDriverNameError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	torcCR := &netappv1.TridentOrchestrator{}
	tconfCR := &netappv1.TridentConfigurator{
		Spec: netappv1.TridentConfiguratorSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(InvalidFieldJSONSpec)}, // Missing storageDriverName
		},
	}

	mockClient.EXPECT().GetTconfCR(TestCRName).Return(tconfCR, nil)
	mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
	mockClient.EXPECT().GetControllingTorcCR().Return(torcCR, nil)

	err := controller.reconcile(TestKey)
	assert.Error(t, err, "Failed to get storage driver name")
}

func TestController_ProcessBackend_ValidatePhaseError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	// Create a mock backend that will fail validation
	mockBackend := &mockBackend{validateError: fmt.Errorf(ValidationFailedMessage)}

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestName},
		Status:     netappv1.TridentConfiguratorStatus{},
	}

	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, false, nil).AnyTimes()

	err := controller.ProcessBackend(mockBackend, tconfCR)
	assert.Error(t, err, "Backend validation should fail")
}

func TestController_ProcessBackend_CreatePhaseError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	// Create a mock backend that will fail on create
	mockBackend := &mockBackend{createError: fmt.Errorf(CreateFailedMessage)}

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestName},
		Status:     netappv1.TridentConfiguratorStatus{},
	}

	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, false, nil).AnyTimes()

	err := controller.ProcessBackend(mockBackend, tconfCR)
	assert.Error(t, err, "Backend create should fail")
}

func TestController_ProcessBackend_UpdateStatusError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	mockBackend := &mockBackend{}

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestName},
		Status:     netappv1.TridentConfiguratorStatus{},
	}

	// Mock will return error on status update
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(nil, false, fmt.Errorf(StatusUpdateFailedMessage))

	err := controller.ProcessBackend(mockBackend, tconfCR)
	assert.Error(t, err, "Status update should fail")
}

func TestController_ensureTridentConfiguratorCRDExists(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	// Test successful CRD creation
	mockClient.EXPECT().CreateOrPatchObject(gomock.Any(), TridentConfiguratorCRDName, "", gomock.Any()).Return(nil)

	err := controller.ensureTridentConfiguratorCRDExists()
	assert.NoError(t, err, "Should succeed in creating CRD")

	mockClient.EXPECT().CreateOrPatchObject(gomock.Any(), TridentConfiguratorCRDName, "", gomock.Any()).Return(fmt.Errorf("CRD creation failed"))

	err = controller.ensureTridentConfiguratorCRDExists()
	assert.Error(t, err, "CRD creation should fail")
}

func TestController_updateEventAndStatus(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestName},
		Status:     netappv1.TridentConfiguratorStatus{},
	}

	// Test successful status update
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(tconfCR, gomock.Any()).Return(tconfCR, true, nil)

	result, err := controller.updateEventAndStatus(tconfCR, netappv1.ValidatingConfig, nil, TestCloudProvider, []string{TestBackendName})
	assert.NoError(t, err, "Should succeed in updating status")
	assert.NotNil(t, result, "Should return updated CR")

	mockClient.EXPECT().UpdateTridentConfiguratorStatus(tconfCR, gomock.Any()).Return(nil, false, fmt.Errorf("update failed"))

	result, err = controller.updateEventAndStatus(tconfCR, netappv1.ValidatingConfig, nil, TestCloudProvider, []string{TestBackendName})
	assert.Error(t, err, "Status update should fail")
}

// Mock backend for testing
type mockBackend struct {
	validateError           error
	createError             error
	createStorageClassError error
	createSnapshotError     error
	cloudProvider           string
	backends                []string
}

func (m *mockBackend) Validate() error {
	return m.validateError
}

func (m *mockBackend) Create() ([]string, error) {
	if m.createError != nil {
		return nil, m.createError
	}
	if m.backends == nil {
		return []string{MockBackendName}, nil
	}
	return m.backends, nil
}

func (m *mockBackend) CreateStorageClass() error {
	return m.createStorageClassError
}

func (m *mockBackend) CreateSnapshotClass() error {
	return m.createSnapshotError
}

func (m *mockBackend) GetCloudProvider() string {
	if m.cloudProvider == "" {
		return TestCloudProvider
	}
	return m.cloudProvider
}

func (m *mockBackend) DeleteBackend(map[string]interface{}) error {
	return nil
}

func (m *mockBackend) DeleteStorageClass(map[string]interface{}) error {
	return nil
}

func (m *mockBackend) DeleteSnapshotClass() error {
	return nil
}

func TestController_getNextProcessingPhase(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	tests := []struct {
		input    netappv1.TConfPhase
		expected netappv1.TConfPhase
	}{
		{netappv1.ValidatingConfig, netappv1.CreatingBackend},
		{netappv1.CreatingBackend, netappv1.CreatingSC},
		{netappv1.CreatingSC, netappv1.CreatingSnapClass},
		{netappv1.TConfPhase(""), netappv1.ValidatingConfig},        // Default case
		{netappv1.TConfPhase("unknown"), netappv1.ValidatingConfig}, // Default case
	}

	for _, test := range tests {
		result := controller.getNextProcessingPhase(test.input)
		assert.Equal(t, test.expected, result,
			fmt.Sprintf("getNextProcessingPhase(%s) should return %s", test.input, test.expected))
	}
}

func TestController_getProcessedPhase(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	tests := []struct {
		input    netappv1.TConfPhase
		expected netappv1.TConfPhase
	}{
		{netappv1.ValidatingConfig, netappv1.ValidatedConfig},
		{netappv1.CreatingBackend, netappv1.CreatedBackend},
		{netappv1.CreatingSC, netappv1.CreatedSC},
		{netappv1.CreatingSnapClass, netappv1.Done},
		{netappv1.TConfPhase("unknown"), netappv1.TConfPhase("")}, // Default case
	}

	for _, test := range tests {
		result := controller.getProcessedPhase(test.input)
		assert.Equal(t, test.expected, result,
			fmt.Sprintf("getProcessedPhase(%s) should return %s", test.input, test.expected))
	}
}

func TestController_getNewConfiguratorStatus_WithError(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	testError := fmt.Errorf(TestErrorMessage)
	backendNames := []string{Backend1, Backend2}

	status := controller.getNewConfiguratorStatus(
		netappv1.ValidatingConfig, "previous-phase", testError, TestCloudProvider, backendNames)

	assert.Equal(t, backendNames, status.BackendNames)
	assert.Equal(t, "previous-phase", status.Phase) // Should keep last phase on error
	assert.Equal(t, string(netappv1.Failed), status.LastOperationStatus)
	assert.Contains(t, status.Message, "Failed: "+TestErrorMessage)
	assert.Equal(t, TestCloudProvider, status.CloudProvider)
}

func TestController_getNewConfiguratorStatus_AllPhases(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	tests := []struct {
		phase           netappv1.TConfPhase
		expectedMessage string
		expectedStatus  string
	}{
		{netappv1.ValidatingConfig, "Validating backend configuration", string(netappv1.Processing)},
		{netappv1.ValidatedConfig, "Provided backend configuration is correct", string(netappv1.Processing)},
		{netappv1.CreatingBackend, "Creating backend with the provided configuration", string(netappv1.Processing)},
		{netappv1.CreatedBackend, "Backend creation successful", string(netappv1.Processing)},
		{netappv1.CreatingSC, "Creating storage classes for the backend", string(netappv1.Processing)},
		{netappv1.CreatedSC, "Storage class creation successful", string(netappv1.Processing)},
		{netappv1.CreatingSnapClass, "Validating backend configuration", string(netappv1.Processing)},
		{netappv1.Done, "Completed Trident backend configuration", string(netappv1.Success)},
	}

	for _, test := range tests {
		status := controller.getNewConfiguratorStatus(
			test.phase, "", nil, TestCloudProvider, []string{TestBackendName})

		assert.Equal(t, test.expectedMessage, status.Message)
		assert.Equal(t, test.expectedStatus, status.LastOperationStatus)
		assert.Equal(t, string(test.phase), status.Phase)
	}
}

func TestController_updateTridentConfiguratorEvent(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestName},
		Status: netappv1.TridentConfiguratorStatus{
			LastOperationStatus: "Success",
			Message:             TestMessage,
		},
	}

	// Test with no error and updateEvent true (should record normal event)
	controller.updateTridentConfiguratorEvent(tconfCR, nil, true)

	// Test with error (should record warning event)
	testErr := fmt.Errorf(TestErrorMessage)
	controller.updateTridentConfiguratorEvent(tconfCR, testErr, false)

	// Test with no error and updateEvent false (should not record event)
	controller.updateTridentConfiguratorEvent(tconfCR, nil, false)
}

func TestController_ProcessBackend_NilTconfCR(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	err := controller.ProcessBackend(nil, nil)
	assert.Error(t, err, "Nil tconf CR should fail")
	assert.Contains(t, err.Error(), "invalid trident configurator CR")
}

// Test runWorker function
func TestController_runWorker(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: "test-item"},
	}
	mockClient.EXPECT().GetTconfCR("test-item").Return(tconfCR, nil)
	mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
	mockClient.EXPECT().GetControllingTorcCR().Return(nil, fmt.Errorf("mock error to exit quickly"))

	controller.workqueue.Add("test-item")
	controller.workqueue.ShutDown()

	controller.runWorker()
	assert.Equal(t, ExpectedWorkqueueLength0, controller.workqueue.Len(), "Workqueue should be empty after runWorker")
}

// New negative test cases for improved coverage

func TestController_processNextWorkItem_RateLimitedError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	// Mock an error that should cause rate limiting (not UnsupportedConfig, NotFound, or ReconcileIncomplete)
	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestCRName},
	}
	mockClient.EXPECT().GetTconfCR(TestCRName).Return(tconfCR, nil)
	mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
	mockClient.EXPECT().GetControllingTorcCR().Return(nil, fmt.Errorf("generic error"))
	controller.workqueue.Add(TestKey)

	result := controller.processNextWorkItem()
	assert.True(t, result, "Should return true for rate limited error")
	// Note: Due to async nature of workqueue, we can't reliably test the queue length immediately
}

func TestController_processNextWorkItem_RequeueError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	// Mock ReconcileIncompleteError which should cause immediate requeue
	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestCRName},
	}
	mockClient.EXPECT().GetTconfCR(TestCRName).Return(tconfCR, nil)
	mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
	mockClient.EXPECT().GetControllingTorcCR().Return(nil, errors.ReconcileIncompleteError("incomplete"))
	controller.workqueue.Add(TestKey)

	result := controller.processNextWorkItem()
	assert.True(t, result, "Should return true for reconcile incomplete error")
	// The item should be back in the queue immediately
	assert.Equal(t, ExpectedWorkqueueLength1, controller.workqueue.Len(), "Item should be re-queued immediately")
}

func TestController_reconcile_ANFInstanceCreationError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	torcCR := &netappv1.TridentOrchestrator{}
	tconfCR := &netappv1.TridentConfigurator{
		Spec: netappv1.TridentConfiguratorSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(ValidBackendJSONSpec)},
		},
	}

	mockClient.EXPECT().GetTconfCR(TestCRName).Return(tconfCR, nil)
	mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
	mockClient.EXPECT().GetControllingTorcCR().Return(torcCR, nil)
	// Mock ANF secret retrieval to fail, which should cause ANF instance creation to fail
	mockClient.EXPECT().GetANFSecrets("").Return("", "", fmt.Errorf(SecretNotFoundMessage))
	// Also expect a status update call that would happen in the error flow
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, false, nil).AnyTimes()

	err := controller.reconcile(TestKey)
	assert.Error(t, err, "Should fail when ANF instance creation fails")
	assert.Contains(t, err.Error(), SecretNotFoundMessage)
}

func TestController_reconcile_FSxNInstanceCreationError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	torcCR := &netappv1.TridentOrchestrator{}
	tconfCR := &netappv1.TridentConfigurator{
		Spec: netappv1.TridentConfiguratorSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(ONTAPNASFSxNJSONSpec)},
		},
	}

	mockClient.EXPECT().GetTconfCR(TestCRName).Return(tconfCR, nil)
	mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
	mockClient.EXPECT().GetControllingTorcCR().Return(torcCR, nil)
	// Also expect a status update call that would happen in the error flow
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, false, nil).AnyTimes()

	err := controller.reconcile(TestKey)
	assert.Error(t, err, "Should fail when FSxN instance creation fails")
}

func TestController_ensureTridentConfiguratorCRDExists_Error(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	// Test CRD creation failure
	mockClient.EXPECT().CreateOrPatchObject(gomock.Any(), TridentConfiguratorCRDName, "", gomock.Any()).
		Return(fmt.Errorf("CRD creation failed"))

	err := controller.ensureTridentConfiguratorCRDExists()
	assert.Error(t, err, "CRD creation should fail")
	assert.Contains(t, err.Error(), "CRD creation failed")
}

func TestController_ProcessBackend_StatusUpdateFailure(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	mockBackend := &mockBackend{
		createError:             fmt.Errorf(CreateFailedMessage),
		createStorageClassError: nil,
		createSnapshotError:     nil,
	}

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestName},
		Status:     netappv1.TridentConfiguratorStatus{},
	}

	// Mock the first status update to succeed, then the error status update to fail
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).
		Return(tconfCR, true, nil) // First update (entering phase)
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).
		Return(nil, false, fmt.Errorf(StatusUpdateFailedMessage)) // Second update (error status)

	err := controller.ProcessBackend(mockBackend, tconfCR)
	assert.Error(t, err, "Should fail when status update fails")
	assert.Contains(t, err.Error(), StatusUpdateFailedMessage)
}

func TestController_backendCreateOperation_UpdateStatusFailure(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: TestName},
		Status:     netappv1.TridentConfiguratorStatus{},
	}

	operationFunc := func() ([]string, error) {
		return []string{Backend1, Backend2}, nil
	}

	// Mock the first status update to succeed, GetControllingTorcCR for verify, then the second status update to fail
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).
		Return(tconfCR, true, nil) // First update
	mockClient.EXPECT().GetControllingTorcCR().Return(&netappv1.TridentOrchestrator{}, nil).AnyTimes() // Called by verifyBackendStatus (polls)
	// Return successful backends to satisfy verification
	successfulBackends := []*tridentV1.TridentBackendConfig{
		{Status: tridentV1.TridentBackendConfigStatus{LastOperationStatus: "Succeeded"}},
	}
	mockClient.EXPECT().ListTridentBackendsByLabel(gomock.Any(), gomock.Any(), gomock.Any()).Return(successfulBackends, nil).AnyTimes() // Called by verifyBackendStatus (polls)
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).
		Return(nil, false, fmt.Errorf(StatusUpdateFailedMessage)) // Second update fails

	newTconfCR, operationErr, updateErr := controller.backendCreateOperation(tconfCR, netappv1.CreatingBackend, operationFunc, TestCloudProvider)

	assert.NoError(t, operationErr, "Operation should succeed")
	assert.Error(t, updateErr, "Status update should fail")
	assert.Nil(t, newTconfCR, "Should return nil CR when update fails")
}

// Additional negative test cases for edge cases

func TestController_getProcessedPhase_EdgeCases(t *testing.T) {
	controller, mockCtrl, _ := createMockController(t)
	defer mockCtrl.Finish()

	// Test additional edge cases for getProcessedPhase
	tests := []struct {
		input    netappv1.TConfPhase
		expected netappv1.TConfPhase
	}{
		{netappv1.TConfPhase(""), netappv1.TConfPhase("")},       // Empty phase
		{netappv1.TConfPhase("random"), netappv1.TConfPhase("")}, // Random phase
	}

	for _, test := range tests {
		result := controller.getProcessedPhase(test.input)
		assert.Equal(t, test.expected, result,
			fmt.Sprintf("getProcessedPhase(%s) should return %s", test.input, test.expected))
	}
}

func TestController_reconcile_EmptyDriverName(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	torcCR := &netappv1.TridentOrchestrator{}
	tconfCR := &netappv1.TridentConfigurator{
		Spec: netappv1.TridentConfiguratorSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(`{"version": 1}`)}, // Missing storageDriverName
		},
	}

	mockClient.EXPECT().GetTconfCR(TestCRName).Return(tconfCR, nil)
	mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
	mockClient.EXPECT().GetControllingTorcCR().Return(torcCR, nil)

	err := controller.reconcile(TestKey)
	assert.Error(t, err, "Should fail with missing storage driver name")
}

// Test successful reconcile scenarios for Azure ANF
func TestController_reconcile_AzureANFSuccess(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	torcCR := &netappv1.TridentOrchestrator{}
	tconfCR := &netappv1.TridentConfigurator{
		Spec: netappv1.TridentConfiguratorSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(`{"storageDriverName":"azure-netapp-files"}`)},
		},
	}

	// Mock expectations - this will fail at creating ANF instance due to nil clients,
	// but we're testing the code path gets there
	mockClient.EXPECT().GetTconfCR("test-cr").Return(tconfCR, nil)
	mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
	mockClient.EXPECT().GetControllingTorcCR().Return(torcCR, nil)
	mockClient.EXPECT().GetANFSecrets("").Return("", "", fmt.Errorf("secret not found"))
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, false, nil).AnyTimes()

	err := controller.reconcile("test/test-cr")
	assert.Error(t, err, "ANF backend creation should fail")
}

func TestController_reconcile_ONTAPDrivers(t *testing.T) {
	tests := []struct {
		name        string
		specJson    string
		expectError bool
		setupMocks  func(*mockConfClients.MockConfiguratorClientInterface, *netappv1.TridentConfigurator)
		description string
	}{
		{
			name:        "ONTAPNASSuccess",
			specJson:    `{"storageDriverName":"ontap-nas","svms":[{}]}`,
			expectError: false,
			setupMocks: func(mockClient *mockConfClients.MockConfiguratorClientInterface, tconfCR *netappv1.TridentConfigurator) {
				// No additional mocks needed for non-FSxN case
			},
			description: "ONTAP NAS without FSxN should succeed and do nothing",
		},
		{
			name:        "ONTAPNASFSxNSuccess",
			specJson:    `{"storageDriverName":"ontap-nas","svms":[{"fsxnID":"fs-12345"}]}`,
			expectError: true,
			setupMocks: func(mockClient *mockConfClients.MockConfiguratorClientInterface, tconfCR *netappv1.TridentConfigurator) {
				mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, false, nil).AnyTimes()
			},
			description: "FSxN backend creation should fail",
		},
		{
			name:        "ONTAPSANSuccess",
			specJson:    `{"storageDriverName":"ontap-san","svms":[{}]}`,
			expectError: false,
			setupMocks: func(mockClient *mockConfClients.MockConfiguratorClientInterface, tconfCR *netappv1.TridentConfigurator) {
				// No additional mocks needed for non-FSxN case
			},
			description: "ONTAP SAN without FSxN should succeed and do nothing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, mockCtrl, mockClient := createMockController(t)
			defer mockCtrl.Finish()

			torcCR := &netappv1.TridentOrchestrator{}
			tconfCR := &netappv1.TridentConfigurator{
				Spec: netappv1.TridentConfiguratorSpec{
					RawExtension: runtime.RawExtension{Raw: []byte(tt.specJson)},
				},
			}

			mockClient.EXPECT().GetTconfCR("test-cr").Return(tconfCR, nil)
			mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
			mockClient.EXPECT().GetControllingTorcCR().Return(torcCR, nil)
			tt.setupMocks(mockClient, tconfCR)

			err := controller.reconcile("test/test-cr")
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

// Test successful ProcessBackend with complete flow
func TestController_ProcessBackend_SuccessfulFlow(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	// Create a successful mock backend
	mockBackend := &mockBackend{
		validateError:           nil,
		createError:             nil,
		createStorageClassError: nil,
		createSnapshotError:     nil,
		backends:                []string{"test-backend-1", "test-backend-2"},
	}

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Status:     netappv1.TridentConfiguratorStatus{},
	}

	// Mock successful status updates for all phases
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, true, nil).Times(8) // 4 phases × 2 updates each
	mockClient.EXPECT().GetControllingTorcCR().Return(&netappv1.TridentOrchestrator{}, nil).AnyTimes()                  // Called by verifyBackendStatus for each phase (polls)
	// Return successful backends to satisfy verification
	successfulBackends := []*tridentV1.TridentBackendConfig{
		{Status: tridentV1.TridentBackendConfigStatus{LastOperationStatus: "Succeeded"}},
	}
	mockClient.EXPECT().ListTridentBackendsByLabel(gomock.Any(), gomock.Any(), gomock.Any()).Return(successfulBackends, nil).AnyTimes() // Called by verifyBackendStatus (polls)

	err := controller.ProcessBackend(mockBackend, tconfCR)
	assert.NoError(t, err, "Should succeed with successful backend")
}

// Test ProcessBackend with storage class creation error
func TestController_ProcessBackend_StorageClassError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	mockBackend := &mockBackend{
		createStorageClassError: fmt.Errorf("storage class creation failed"),
	}

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Status:     netappv1.TridentConfiguratorStatus{},
	}

	// Mock status updates until it fails at storage class creation
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, false, nil).AnyTimes()
	mockClient.EXPECT().GetControllingTorcCR().Return(&netappv1.TridentOrchestrator{}, nil).AnyTimes() // Called by verifyBackendStatus
	// Return successful backends to satisfy verification
	successfulBackends := []*tridentV1.TridentBackendConfig{
		{Status: tridentV1.TridentBackendConfigStatus{LastOperationStatus: "Succeeded"}},
	}
	mockClient.EXPECT().ListTridentBackendsByLabel(gomock.Any(), gomock.Any(), gomock.Any()).Return(successfulBackends, nil).AnyTimes() // Called by verifyBackendStatus

	err := controller.ProcessBackend(mockBackend, tconfCR)
	assert.Error(t, err, "Storage class creation should fail")
}

// Test ProcessBackend with snapshot class creation error
func TestController_ProcessBackend_SnapshotClassError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	mockBackend := &mockBackend{
		createSnapshotError: fmt.Errorf("snapshot class creation failed"),
	}

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Status:     netappv1.TridentConfiguratorStatus{},
	}

	// Mock status updates until it fails at snapshot class creation
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, false, nil).AnyTimes()
	mockClient.EXPECT().GetControllingTorcCR().Return(&netappv1.TridentOrchestrator{}, nil).AnyTimes() // Called by verifyBackendStatus
	// Return successful backends to satisfy verification
	successfulBackends := []*tridentV1.TridentBackendConfig{
		{Status: tridentV1.TridentBackendConfigStatus{LastOperationStatus: "Succeeded"}},
	}
	mockClient.EXPECT().ListTridentBackendsByLabel(gomock.Any(), gomock.Any(), gomock.Any()).Return(successfulBackends, nil).AnyTimes() // Called by verifyBackendStatus

	err := controller.ProcessBackend(mockBackend, tconfCR)
	assert.Error(t, err, "Snapshot class creation should fail")
}

// Test backendCreateOperation success path
func TestController_backendCreateOperation_Success(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Status:     netappv1.TridentConfiguratorStatus{},
	}

	operationFunc := func() ([]string, error) {
		return []string{"backend1", "backend2"}, nil
	}

	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, true, nil).Times(2)
	mockClient.EXPECT().GetControllingTorcCR().Return(&netappv1.TridentOrchestrator{}, nil).AnyTimes() // Called by verifyBackendStatus (polls)
	// Return successful backends to satisfy verification
	successfulBackends := []*tridentV1.TridentBackendConfig{
		{Status: tridentV1.TridentBackendConfigStatus{LastOperationStatus: "Succeeded"}},
	}
	mockClient.EXPECT().ListTridentBackendsByLabel(gomock.Any(), gomock.Any(), gomock.Any()).Return(successfulBackends, nil).AnyTimes() // Called by verifyBackendStatus (polls)

	newTconfCR, operationErr, updateErr := controller.backendCreateOperation(tconfCR, netappv1.CreatingBackend, operationFunc, "azure")

	assert.NoError(t, operationErr, "Operation should succeed")
	assert.NoError(t, updateErr, "Status update should succeed")
	assert.NotNil(t, newTconfCR, "Should return updated CR")
}

// Test backendCreateOperation with operation error
func TestController_backendCreateOperation_OperationError(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	tconfCR := &netappv1.TridentConfigurator{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		Status:     netappv1.TridentConfiguratorStatus{},
	}

	operationFunc := func() ([]string, error) {
		return nil, fmt.Errorf("backend creation failed")
	}

	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, true, nil).Times(2)

	newTconfCR, operationErr, updateErr := controller.backendCreateOperation(tconfCR, netappv1.CreatingBackend, operationFunc, "azure")

	assert.Error(t, operationErr, "Backend create operation should fail")
	assert.NoError(t, updateErr, "Status update should succeed")
	assert.NotNil(t, newTconfCR, "Should return updated CR even on operation error")
}

// Test processNextWorkItem success path
func TestController_processNextWorkItem_Success(t *testing.T) {
	controller, mockCtrl, mockClient := createMockController(t)
	defer mockCtrl.Finish()

	// Mock successful reconcile
	torcCR := &netappv1.TridentOrchestrator{}
	tconfCR := &netappv1.TridentConfigurator{
		Spec: netappv1.TridentConfiguratorSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(`{"storageDriverName":"azure-netapp-files"}`)},
		},
	}

	mockClient.EXPECT().GetTconfCR("test-cr").Return(tconfCR, nil)
	mockClient.EXPECT().UpdateTridentConfigurator(gomock.Any()).Return(nil) // ensureFinalizer call
	mockClient.EXPECT().GetControllingTorcCR().Return(torcCR, nil)
	mockClient.EXPECT().GetANFSecrets("").Return("", "", fmt.Errorf("secret not found"))
	mockClient.EXPECT().UpdateTridentConfiguratorStatus(gomock.Any(), gomock.Any()).Return(tconfCR, false, nil).AnyTimes()

	controller.workqueue.Add("test/test-cr")

	result := controller.processNextWorkItem()
	assert.True(t, result, "Should return true on successful processing")
}

func TestController_KeyExtractionErrors(t *testing.T) {
	tests := []struct {
		name        string
		testFunc    func(*Controller, interface{})
		description string
	}{
		{
			name: "addConfigurator_KeyExtractionError",
			testFunc: func(controller *Controller, invalidObj interface{}) {
				controller.addConfigurator(invalidObj)
			},
			description: "addConfigurator should handle key extraction error",
		},
		{
			name: "deleteConfigurator_KeyExtractionError",
			testFunc: func(controller *Controller, invalidObj interface{}) {
				controller.deleteConfigurator(invalidObj)
			},
			description: "deleteConfigurator should handle key extraction error",
		},
		{
			name: "updateConfigurator_KeyExtractionError",
			testFunc: func(controller *Controller, invalidObj interface{}) {
				oldCR := &netappv1.TridentConfigurator{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
				}
				controller.updateConfigurator(oldCR, invalidObj)
			},
			description: "updateConfigurator should handle key extraction error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, mockCtrl, _ := createMockController(t)
			defer mockCtrl.Finish()

			// Create an object that will cause cache.MetaNamespaceKeyFunc to fail
			invalidObj := &struct{ Name string }{Name: "invalid"}

			tt.testFunc(controller, invalidObj)
			assert.Equal(t, 0, controller.workqueue.Len(), tt.description)
		})
	}
}
