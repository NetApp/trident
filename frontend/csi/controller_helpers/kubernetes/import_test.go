package kubernetes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

func createTestPVC(name, namespace, storageClassName string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
}

func createTestImportRequest(pvc *v1.PersistentVolumeClaim, backend, internalName string) *storage.ImportVolumeRequest {
	pvcData, _ := json.Marshal(pvc)
	pvcDataB64 := base64.StdEncoding.EncodeToString(pvcData)

	return &storage.ImportVolumeRequest{
		Backend:      backend,
		InternalName: internalName,
		PVCData:      pvcDataB64,
	}
}

func TestImportVolume_ErrorCases(t *testing.T) {
	tests := []struct {
		name         string
		setupRequest func() *storage.ImportVolumeRequest
		setupMocks   func(*mockcore.MockOrchestrator)
		expectError  string
	}{
		{
			name: "InvalidPVCData",
			setupRequest: func() *storage.ImportVolumeRequest {
				return &storage.ImportVolumeRequest{
					Backend:      "test-backend",
					InternalName: "test-volume",
					PVCData:      "invalid-base64-data!!!",
				}
			},
			setupMocks:  func(mock *mockcore.MockOrchestrator) {},
			expectError: "Expected error when PVC data contains invalid base64",
		},
		{
			name: "InvalidJSONPVC",
			setupRequest: func() *storage.ImportVolumeRequest {
				invalidJSON := base64.StdEncoding.EncodeToString([]byte("invalid-json"))
				return &storage.ImportVolumeRequest{
					Backend:      "test-backend",
					InternalName: "test-volume",
					PVCData:      invalidJSON,
				}
			},
			setupMocks:  func(mock *mockcore.MockOrchestrator) {},
			expectError: "Expected error when PVC data contains invalid JSON",
		},
		{
			name: "NoStorageClassSpecified",
			setupRequest: func() *storage.ImportVolumeRequest {
				pvc := createTestPVC("test-pvc", "default", "")
				pvc.Spec.StorageClassName = nil
				return createTestImportRequest(pvc, "test-backend", "test-volume")
			},
			setupMocks:  func(mock *mockcore.MockOrchestrator) {},
			expectError: "Expected error when PVC has no storage class specified",
		},
		{
			name: "NoNamespaceSpecified",
			setupRequest: func() *storage.ImportVolumeRequest {
				pvc := createTestPVC("test-pvc", "", "test-storage-class")
				return createTestImportRequest(pvc, "test-backend", "test-volume")
			},
			setupMocks:  func(mock *mockcore.MockOrchestrator) {},
			expectError: "Expected error when PVC has no namespace specified",
		},
		{
			name: "BackendNotFound",
			setupRequest: func() *storage.ImportVolumeRequest {
				pvc := createTestPVC("test-pvc", "default", "test-storage-class")
				return createTestImportRequest(pvc, "nonexistent-backend", "test-volume")
			},
			setupMocks: func(mock *mockcore.MockOrchestrator) {
				mock.EXPECT().GetBackend(gomock.Any(), "nonexistent-backend").
					Return(nil, errors.NotFoundError("backend not found"))
			},
			expectError: "Expected error when backend is not found",
		},
		{
			name: "VolumeNotFoundForImport",
			setupRequest: func() *storage.ImportVolumeRequest {
				pvc := createTestPVC("test-pvc", "default", "test-storage-class")
				return createTestImportRequest(pvc, "test-backend", "test-volume")
			},
			setupMocks: func(mock *mockcore.MockOrchestrator) {
				backend := &storage.BackendExternal{
					BackendUUID: "test-backend-uuid",
				}
				mock.EXPECT().GetBackend(gomock.Any(), "test-backend").Return(backend, nil)
				mock.EXPECT().GetVolumeForImport(gomock.Any(), "test-volume", "test-backend").
					Return(nil, errors.NotFoundError("volume not found"))
			},
			expectError: "Expected error when volume is not found for import",
		},
		{
			name: "VolumeAlreadyManaged",
			setupRequest: func() *storage.ImportVolumeRequest {
				pvc := createTestPVC("test-pvc", "default", "test-storage-class")
				return createTestImportRequest(pvc, "test-backend", "test-volume")
			},
			setupMocks: func(mock *mockcore.MockOrchestrator) {
				backend := &storage.BackendExternal{
					BackendUUID: "test-backend-uuid",
				}
				volumeExternal := &storage.VolumeExternal{
					Config: &storage.VolumeConfig{
						InternalName: "test-internal-volume",
						Size:         "1073741824", // 1Gi in bytes
					},
				}
				mock.EXPECT().GetBackend(gomock.Any(), "test-backend").Return(backend, nil)
				mock.EXPECT().GetVolumeForImport(gomock.Any(), "test-volume", "test-backend").
					Return(volumeExternal, nil)
				mock.EXPECT().GetVolumeByInternalName(gomock.Any(), "test-internal-volume").
					Return("existing-pv-name", nil) // Volume already managed
			},
			expectError: "Expected error when volume is already managed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOrchestrator := mockcore.NewMockOrchestrator(ctrl)
			kubeClient := k8sfake.NewSimpleClientset()

			helper := &helper{
				orchestrator: mockOrchestrator,
				kubeClient:   kubeClient,
			}

			request := tt.setupRequest()
			tt.setupMocks(mockOrchestrator)

			// Act
			_, err := helper.ImportVolume(context.Background(), request)

			// Assert
			assert.Error(t, err, tt.expectError)
		})
	}
}

func TestCreateImportPVC(t *testing.T) {
	tests := []struct {
		name           string
		setupClient    func() *k8sfake.Clientset
		assertErr      assert.ErrorAssertionFunc
		assertResult   assert.ValueAssertionFunc
		validateResult func(t *testing.T, result *v1.PersistentVolumeClaim, client *k8sfake.Clientset)
	}{
		{
			name: "Success",
			setupClient: func() *k8sfake.Clientset {
				return k8sfake.NewSimpleClientset()
			},
			assertErr:    assert.NoError,
			assertResult: assert.NotNil,
			validateResult: func(t *testing.T, result *v1.PersistentVolumeClaim, client *k8sfake.Clientset) {
				assert.Equal(t, "test-pvc", result.Name)
				assert.Equal(t, "default", result.Namespace)

				// Verify PVC was created in cluster
				createdPVC, err := client.CoreV1().PersistentVolumeClaims("default").Get(
					context.Background(), "test-pvc", metav1.GetOptions{})
				assert.NoError(t, err)
				assert.Equal(t, "test-pvc", createdPVC.Name)
			},
		},
		{
			name: "CreationError",
			setupClient: func() *k8sfake.Clientset {
				kubeClient := k8sfake.NewSimpleClientset()
				// Add a reactor to simulate creation failure
				kubeClient.PrependReactor("create", "persistentvolumeclaims", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, errors.New("simulated creation error")
				})
				return kubeClient
			},
			assertErr:    assert.Error,
			assertResult: assert.Nil,
			validateResult: func(t *testing.T, result *v1.PersistentVolumeClaim, client *k8sfake.Clientset) {
				// No additional validation needed for error case
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			kubeClient := tt.setupClient()
			helper := &helper{
				kubeClient: kubeClient,
			}

			pvc := createTestPVC("test-pvc", "default", "test-storage-class")

			// Act
			result, err := helper.createImportPVC(context.Background(), pvc)

			// Assert using function types - no conditional logic!
			tt.assertErr(t, err, "Unexpected error result for test case: %s", tt.name)
			tt.assertResult(t, result, "Unexpected result for test case: %s", tt.name)

			// Only run additional validation if we have a result
			if result != nil {
				tt.validateResult(t, result, kubeClient)
			}
		})
	}
}

func TestCheckAndHandleUnrecoverableError_Basic(t *testing.T) {
	tests := []struct {
		name                string
		inputError          error
		expectUnrecoverable bool
		assertErr           assert.ErrorAssertionFunc
		validateError       func(t *testing.T, inputErr, resultErr error)
	}{
		{
			name:                "NoError",
			inputError:          nil,
			expectUnrecoverable: false,
			assertErr:           assert.NoError,
			validateError: func(t *testing.T, inputErr, resultErr error) {
				// No additional validation needed for nil case
			},
		},
		{
			name:                "WithError",
			inputError:          errors.New("test PV error"),
			expectUnrecoverable: false,
			assertErr:           assert.Error,
			validateError: func(t *testing.T, inputErr, resultErr error) {
				assert.Equal(t, inputErr, resultErr, "Error should be passed through unchanged")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			kubeClient := k8sfake.NewSimpleClientset()
			helper := &helper{
				kubeClient: kubeClient,
			}

			pvc := &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "default",
					UID:       "test-uid-123",
				},
			}

			// Act
			unrecoverable, err := checkAndHandleUnrecoverableError(context.Background(), helper, pvc, tt.inputError)

			// Assert using function types - no conditional logic!
			assert.Equal(t, tt.expectUnrecoverable, unrecoverable)
			tt.assertErr(t, err, "Unexpected error result for test case: %s", tt.name)
			tt.validateError(t, tt.inputError, err)
		})
	}
}

func TestCheckAndHandleUnrecoverableError_ProvisioningFailedEvent(t *testing.T) {
	// Arrange
	kubeClient := k8sfake.NewSimpleClientset()
	helper := &helper{
		kubeClient: kubeClient,
	}

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
			UID:       "test-uid-123",
		},
	}

	// Create the PVC in the cluster first
	kubeClient.CoreV1().PersistentVolumeClaims("default").Create(context.Background(), pvc, metav1.CreateOptions{})

	// Create a provisioning failed event
	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
		},
		InvolvedObject: v1.ObjectReference{
			UID:  "test-uid-123",
			Kind: "PersistentVolumeClaim",
		},
		Reason:  "ProvisioningFailed",
		Message: "import volume failed - backend error",
	}

	kubeClient.CoreV1().Events("default").Create(context.Background(), event, metav1.CreateOptions{})

	testError := errors.New("PV not found")

	unrecoverable, err := checkAndHandleUnrecoverableError(context.Background(), helper, pvc, testError)

	assert.True(t, unrecoverable)
	assert.Error(t, err, "Expected error when provisioning failed event is found")
}

func TestWaitForCachedPVByName_Success(t *testing.T) {
	// Test for basic functionality of waitForCachedPVByName
	// This test exercises the timeout behavior when PV is not found in cache

	ctx := context.Background()

	// Create a mock helper with empty cache
	h := &helper{
		pvIndexer: cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{}),
	}

	// Create test PVC
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
	}

	// Create a simple error handler that returns the error as-is
	handler := func(ctx context.Context, h *helper, pvc *v1.PersistentVolumeClaim, err error) (bool, error) {
		return false, err
	}

	// Test that the function times out when PV is not in cache
	// Use a very short timeout to make test fast
	pv, err := h.waitForCachedPVByName(ctx, pvc, "non-existent-pv", 50*time.Millisecond, handler)

	assert.Nil(t, pv)
	assert.Error(t, err, "Expected error when PV is not found in cache and times out")
}

func TestImportVolume_InvalidVolumeSize(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrchestrator := mockcore.NewMockOrchestrator(ctrl)
	kubeClient := k8sfake.NewSimpleClientset()

	helper := &helper{
		orchestrator: mockOrchestrator,
		kubeClient:   kubeClient,
	}

	pvc := createTestPVC("test-pvc", "default", "test-storage-class")
	request := createTestImportRequest(pvc, "test-backend", "test-volume")

	// Mock backend
	backend := &storage.BackendExternal{
		BackendUUID: "test-backend-uuid",
	}

	// Mock volume with invalid size
	volumeExternal := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			InternalName: "test-internal-volume",
			Size:         "invalid-size", // Invalid size format
		},
	}

	// Mock orchestrator calls
	mockOrchestrator.EXPECT().GetBackend(gomock.Any(), "test-backend").Return(backend, nil)
	mockOrchestrator.EXPECT().GetVolumeForImport(gomock.Any(), "test-volume", "test-backend").
		Return(volumeExternal, nil)
	mockOrchestrator.EXPECT().GetVolumeByInternalName(gomock.Any(), "test-internal-volume").
		Return("", errors.NotFoundError("not managed")) // Volume not already managed

	_, err := helper.ImportVolume(context.Background(), request)

	assert.Error(t, err, "Expected error when volume has invalid size")
}

func TestImportVolume_InvalidSnapshotReserve(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrchestrator := mockcore.NewMockOrchestrator(ctrl)
	kubeClient := k8sfake.NewSimpleClientset()

	helper := &helper{
		orchestrator: mockOrchestrator,
		kubeClient:   kubeClient,
	}

	pvc := createTestPVC("test-pvc", "default", "test-storage-class")
	request := createTestImportRequest(pvc, "test-backend", "test-volume")

	// Mock backend
	backend := &storage.BackendExternal{
		BackendUUID: "test-backend-uuid",
	}

	// Mock volume with invalid snapshot reserve
	volumeExternal := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			InternalName:    "test-internal-volume",
			Size:            "1073741824",         // 1Gi in bytes
			SnapshotReserve: "invalid-percentage", // Invalid percentage format
		},
	}

	// Mock orchestrator calls
	mockOrchestrator.EXPECT().GetBackend(gomock.Any(), "test-backend").Return(backend, nil)
	mockOrchestrator.EXPECT().GetVolumeForImport(gomock.Any(), "test-volume", "test-backend").
		Return(volumeExternal, nil)
	mockOrchestrator.EXPECT().GetVolumeByInternalName(gomock.Any(), "test-internal-volume").
		Return("", errors.NotFoundError("not managed")) // Volume not already managed

	_, err := helper.ImportVolume(context.Background(), request)

	assert.Error(t, err, "Expected error when volume has invalid snapshot reserve")
}

func TestImportVolume_LUKSEncryptionDataSizeCalculation(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrchestrator := mockcore.NewMockOrchestrator(ctrl)
	kubeClient := k8sfake.NewSimpleClientset()

	helper := &helper{
		orchestrator: mockOrchestrator,
		kubeClient:   kubeClient,
	}

	// Create PVC with LUKS encryption annotation
	pvc := createTestPVC("test-pvc", "default", "test-storage-class")
	pvc.Annotations = map[string]string{
		"trident.netapp.io/luksEncryption": "true",
	}
	request := createTestImportRequest(pvc, "test-backend", "test-volume")

	// Mock backend
	backend := &storage.BackendExternal{
		BackendUUID: "test-backend-uuid",
	}

	// Mock volume with valid config - 1GiB total size
	volumeExternal := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			InternalName:    "test-internal-volume",
			Size:            "1073741824", // 1GiB in bytes
			SnapshotReserve: "10",         // 10% snapshot reserve
		},
	}

	// Mock orchestrator calls
	mockOrchestrator.EXPECT().GetBackend(gomock.Any(), "test-backend").Return(backend, nil)
	mockOrchestrator.EXPECT().GetVolumeForImport(gomock.Any(), "test-volume", "test-backend").
		Return(volumeExternal, nil)
	mockOrchestrator.EXPECT().GetVolumeByInternalName(gomock.Any(), "test-internal-volume").
		Return("", errors.NotFoundError("not managed")) // Volume not already managed

	// Mock PVC creation to fail quickly so we can test the size calculation
	kubeClient.PrependReactor("create", "persistentvolumeclaims", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdPVC := createAction.GetObject().(*v1.PersistentVolumeClaim)

		// Verify that LUKS metadata size was subtracted from data size
		// Expected calculation: floor(1073741824 * 0.9) (90% after 10% snapshot reserve) - LUKS metadata
		dataAfterSnapReserve := uint64(966367641)                   // floor(1073741824 * 0.9)
		expectedDataSizeWithLUKS := dataAfterSnapReserve - 18874368 // Subtract LUKS metadata size

		requestedSize := createdPVC.Spec.Resources.Requests[v1.ResourceStorage]
		requestedSizeBytes, _ := requestedSize.AsInt64()

		assert.Equal(t, int64(expectedDataSizeWithLUKS), requestedSizeBytes, "Data size should be reduced by LUKS metadata size")

		return true, nil, errors.New("intended failure for testing")
	})

	_, err := helper.ImportVolume(context.Background(), request)

	assert.Error(t, err, "Expected error when PVC creation fails during LUKS encryption size calculation test")
}

func TestImportVolume_NilResourcesRequestsInitialization(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrchestrator := mockcore.NewMockOrchestrator(ctrl)
	kubeClient := k8sfake.NewSimpleClientset()

	helper := &helper{
		orchestrator: mockOrchestrator,
		kubeClient:   kubeClient,
	}

	// Create PVC with nil Resources.Requests
	pvc := createTestPVC("test-pvc", "default", "test-storage-class")
	pvc.Spec.Resources.Requests = nil // Set to nil to test initialization
	request := createTestImportRequest(pvc, "test-backend", "test-volume")

	// Mock backend
	backend := &storage.BackendExternal{
		BackendUUID: "test-backend-uuid",
	}

	// Mock volume with valid config
	volumeExternal := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			InternalName:    "test-internal-volume",
			Size:            "1073741824", // 1GiB in bytes
			SnapshotReserve: "0",          // No snapshot reserve for simpler calculation
		},
	}

	// Mock orchestrator calls
	mockOrchestrator.EXPECT().GetBackend(gomock.Any(), "test-backend").Return(backend, nil)
	mockOrchestrator.EXPECT().GetVolumeForImport(gomock.Any(), "test-volume", "test-backend").
		Return(volumeExternal, nil)
	mockOrchestrator.EXPECT().GetVolumeByInternalName(gomock.Any(), "test-internal-volume").
		Return("", errors.NotFoundError("not managed")) // Volume not already managed

	// Mock PVC creation to verify that Resources.Requests was properly initialized
	kubeClient.PrependReactor("create", "persistentvolumeclaims", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdPVC := createAction.GetObject().(*v1.PersistentVolumeClaim)

		// Verify that Resources.Requests was initialized (not nil)
		assert.NotNil(t, createdPVC.Spec.Resources.Requests, "Resources.Requests should be initialized when it was nil")

		// Verify that the storage request was set
		storageRequest, exists := createdPVC.Spec.Resources.Requests[v1.ResourceStorage]
		assert.True(t, exists, "Storage request should be set")
		assert.Equal(t, "1073741824", storageRequest.String(), "Storage request should match volume size")

		return true, nil, errors.New("intended failure for testing")
	})

	_, err := helper.ImportVolume(context.Background(), request)

	assert.Error(t, err, "Expected error when PVC creation fails during volume import")
}

func TestImportVolume_SuccessfulImportFlow(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrchestrator := mockcore.NewMockOrchestrator(ctrl)
	kubeClient := k8sfake.NewSimpleClientset()

	// Create a helper with proper indexer mock for PV caching
	pvIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	helper := &helper{
		orchestrator: mockOrchestrator,
		kubeClient:   kubeClient,
		pvIndexer:    pvIndexer,
	}

	pvc := createTestPVC("test-pvc", "default", "test-storage-class")
	request := createTestImportRequest(pvc, "test-backend", "test-volume")

	// Mock backend
	backend := &storage.BackendExternal{
		BackendUUID: "test-backend-uuid",
	}

	// Mock volume config
	volumeExternal := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			InternalName:    "test-internal-volume",
			Size:            "1073741824", // 1GiB in bytes
			SnapshotReserve: "0",          // No snapshot reserve
		},
	}

	// Expected final volume result
	expectedVolume := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			InternalName: "test-internal-volume",
			Size:         "1073741824",
		},
	}

	// Mock orchestrator calls
	mockOrchestrator.EXPECT().GetBackend(gomock.Any(), "test-backend").Return(backend, nil)
	mockOrchestrator.EXPECT().GetVolumeForImport(gomock.Any(), "test-volume", "test-backend").
		Return(volumeExternal, nil)
	mockOrchestrator.EXPECT().GetVolumeByInternalName(gomock.Any(), "test-internal-volume").
		Return("", errors.NotFoundError("not managed")) // Volume not already managed

	var createdPVCUID string

	// Mock successful PVC creation
	kubeClient.PrependReactor("create", "persistentvolumeclaims", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdPVC := createAction.GetObject().(*v1.PersistentVolumeClaim)

		// Set a UID for the created PVC
		createdPVC.UID = "test-pvc-uid-12345"
		createdPVCUID = string(createdPVC.UID)

		// Add the PVC to our test client
		return false, nil, nil // Let the default handler create it
	})

	go func() {
		// Give some time for the import to start, then add a PV to the cache
		time.Sleep(100 * time.Millisecond)

		expectedPVName := fmt.Sprintf("pvc-%s", createdPVCUID)

		// Create a mock PV and add it to the cache
		mockPV := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: expectedPVName,
			},
			Spec: v1.PersistentVolumeSpec{
				Capacity: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1073741824"),
				},
			},
		}

		// Add PV to the cache to simulate successful creation
		pvIndexer.Add(mockPV)
	}()

	// Mock the final GetVolume call
	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, pvName string) (*storage.VolumeExternal, error) {
			// Verify we're getting the expected PV name format
			expectedPVName := fmt.Sprintf("pvc-%s", createdPVCUID)
			assert.Equal(t, expectedPVName, pvName, "PV name should match expected format")
			return expectedVolume, nil
		},
	)

	result, err := helper.ImportVolume(context.Background(), request)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, expectedVolume.Config.InternalName, result.Config.InternalName)
	assert.Equal(t, expectedVolume.Config.Size, result.Config.Size)
}

func TestImportVolume_WaitForCachedPVTimeout(t *testing.T) {
	// This test verifies that waitForCachedPVByName properly times out
	// We test it directly rather than through ImportVolume to avoid the 180s timeout

	// Arrange
	kubeClient := k8sfake.NewSimpleClientset()
	pvIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})

	helper := &helper{
		kubeClient: kubeClient,
		pvIndexer:  pvIndexer,
	}

	pvc := createTestPVC("test-pvc", "default", "test-storage-class")
	pvc.UID = "test-pvc-uid-timeout"

	result, err := helper.waitForCachedPVByName(
		context.Background(),
		pvc,
		"pvc-test-pvc-uid-timeout",
		100*time.Millisecond, // Short timeout for test
		checkAndHandleUnrecoverableError,
	)

	assert.Error(t, err, "Expected error when waiting for imported volume times out")
	assert.Nil(t, result)
}

func TestImportVolume_GetVolumeFinalFailure(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrchestrator := mockcore.NewMockOrchestrator(ctrl)
	kubeClient := k8sfake.NewSimpleClientset()

	// Create a helper with proper indexer mock for PV caching
	pvIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	helper := &helper{
		orchestrator: mockOrchestrator,
		kubeClient:   kubeClient,
		pvIndexer:    pvIndexer,
	}

	pvc := createTestPVC("test-pvc", "default", "test-storage-class")
	request := createTestImportRequest(pvc, "test-backend", "test-volume")

	// Mock backend
	backend := &storage.BackendExternal{
		BackendUUID: "test-backend-uuid",
	}

	// Mock volume config
	volumeExternal := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			InternalName:    "test-internal-volume",
			Size:            "1073741824", // 1GiB in bytes
			SnapshotReserve: "0",          // No snapshot reserve
		},
	}

	// Mock orchestrator calls
	mockOrchestrator.EXPECT().GetBackend(gomock.Any(), "test-backend").Return(backend, nil)
	mockOrchestrator.EXPECT().GetVolumeForImport(gomock.Any(), "test-volume", "test-backend").
		Return(volumeExternal, nil)
	mockOrchestrator.EXPECT().GetVolumeByInternalName(gomock.Any(), "test-internal-volume").
		Return("", errors.NotFoundError("not managed")) // Volume not already managed

	var createdPVCUID string

	// Mock successful PVC creation
	kubeClient.PrependReactor("create", "persistentvolumeclaims", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		createAction := action.(k8stesting.CreateAction)
		createdPVC := createAction.GetObject().(*v1.PersistentVolumeClaim)

		// Set a UID for the created PVC
		createdPVC.UID = "test-pvc-uid-getvolume-fail"
		createdPVCUID = string(createdPVC.UID)

		return false, nil, nil // Let the default handler create it
	})

	go func() {
		// Give some time for the import to start, then add a PV to the cache
		time.Sleep(100 * time.Millisecond)

		expectedPVName := fmt.Sprintf("pvc-%s", createdPVCUID)

		// Create a mock PV and add it to the cache
		mockPV := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: expectedPVName,
			},
			Spec: v1.PersistentVolumeSpec{
				Capacity: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1073741824"),
				},
			},
		}

		// Add PV to the cache to simulate successful creation
		pvIndexer.Add(mockPV)
	}()

	// Mock the final GetVolume call to fail
	mockOrchestrator.EXPECT().GetVolume(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, pvName string) (*storage.VolumeExternal, error) {
			// Verify we're getting the expected PV name format
			expectedPVName := fmt.Sprintf("pvc-%s", createdPVCUID)
			assert.Equal(t, expectedPVName, pvName, "PV name should match expected format")
			return nil, errors.New("orchestrator GetVolume failed")
		},
	)

	result, err := helper.ImportVolume(context.Background(), request)

	assert.Error(t, err, "Expected error when orchestrator GetVolume fails during volume import")
	assert.Nil(t, result)
}
