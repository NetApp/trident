// Copyright 2022 NetApp, Inc. All Rights Reserved.

package orchestrator

import (
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"

	. "github.com/netapp/trident/logging"
	mockK8sClient "github.com/netapp/trident/mocks/mock_cli/mock_k8s_client"
	"github.com/netapp/trident/operator/clients"
	netappv1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	crdFake "github.com/netapp/trident/operator/crd/client/clientset/versioned/fake"
)

const (
	// Test constants for commonly used values
	TestOrchestratorName = "test-orchestrator"
	TestNamespace        = "trident"
	TestTridentCSIName   = "trident-csi"
	TestAPIErrorMessage  = "API error"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestController_GetName(t *testing.T) {
	controller := &Controller{}
	result := controller.GetName()
	assert.Equal(t, ControllerName, result, "GetName should return correct controller name")
	assert.Equal(t, "Trident Orchestrator", result, "GetName should return expected string")
}

func TestController_Version(t *testing.T) {
	controller := &Controller{}
	result := controller.Version()
	assert.Equal(t, ControllerVersion, result, "Version should return correct controller version")
	assert.Equal(t, "0.1", result, "Version should return expected version string")
}

func TestNewController_NilClients(t *testing.T) {
	t.Run("PanicsWithNilClients", func(t *testing.T) {
		assert.Panics(t, func() {
			NewController(nil)
		}, "NewController should panic with nil clients")
	})

	t.Run("EmptyClientsStruct", func(t *testing.T) {
		assert.Panics(t, func() {
			NewController(&clients.Clients{})
		}, "NewController should panic with empty clients struct due to nil KubeClient")
	})

	t.Run("ClientsWithNilKubeClient", func(t *testing.T) {
		assert.Panics(t, func() {
			NewController(&clients.Clients{KubeClient: nil, Namespace: "test"})
		}, "NewController should panic when KubeClient is nil")
	})
}

func TestController_LifecycleMethods(t *testing.T) {
	t.Run("DeactivateOnUninitializedController", func(t *testing.T) {
		controller := &Controller{}
		assert.Panics(t, func() {
			controller.Deactivate()
		}, "Deactivate should panic on uninitialized controller")
	})
}

func TestController_Activate_Comprehensive(t *testing.T) {
	t.Run("ActivationWithNilStopChan", func(t *testing.T) {
		controller := &Controller{}
		assert.Panics(t, func() {
			controller.Activate()
		}, "Activate should panic with nil stopChan")
	})

	t.Run("ActivationWithNilInformers", func(t *testing.T) {
		controller := &Controller{stopChan: make(chan struct{})}
		assert.Panics(t, func() {
			controller.Activate()
		}, "Activate should panic with nil informers")
	})
}

func TestController_EdgeCases(t *testing.T) {
	t.Run("DoubleDeactivate", func(t *testing.T) {
		controller := &Controller{
			stopChan:  make(chan struct{}),
			workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "test"),
		}

		err := controller.Deactivate()
		assert.NoError(t, err, "Should not error on first deactivation")

		assert.Panics(t, func() {
			controller.Deactivate()
		}, "Second deactivation should panic")
	})
}

// createTestController creates a minimal controller for testing basic functionality
func createTestController() *Controller {
	// Initialize basic controller fields that don't require Kubernetes connection
	controller := &Controller{}
	controller.workqueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Initialize Clients field to avoid nil pointer errors
	controller.Clients = &clients.Clients{}

	// Initialize K8SVersion to avoid nil pointer errors in version functions
	controller.K8SVersion = &version.Info{
		Major:      "1",
		Minor:      "20",
		GitVersion: "v1.20.0",
	}

	return controller
}

func TestController_ProcessNextWorkItem(t *testing.T) {
	t.Run("ProcessEmptyWorkqueue", func(t *testing.T) {
		controller := createTestController()
		// Should return false when workqueue is shutdown
		controller.workqueue.ShutDown()
		result := controller.processNextWorkItem()
		assert.False(t, result, "Should return false when workqueue is shutdown")
	})

	t.Run("ProcessInvalidWorkItem", func(t *testing.T) {
		controller := createTestController()

		// Add invalid item (not KeyItem)
		controller.workqueue.Add("invalid-string")

		result := controller.processNextWorkItem()
		assert.True(t, result, "Should return true and handle invalid item gracefully")

		// Verify the invalid item was forgotten
		assert.Equal(t, 0, controller.workqueue.Len(), "Workqueue should be empty after processing invalid item")
	})

	t.Run("WorkqueueBasicOperations", func(t *testing.T) {
		controller := createTestController()

		// Test workqueue operations that don't require reconciliation
		keyItem := KeyItem{
			keyDetails:   "test-key",
			resourceType: ResourceTridentOrchestratorCR,
		}

		// Add item
		controller.workqueue.Add(keyItem)
		assert.Equal(t, 1, controller.workqueue.Len(), "Workqueue should have 1 item")

		// ShutDown workqueue
		controller.workqueue.ShutDown()
		assert.True(t, controller.workqueue.ShuttingDown(), "Workqueue should be shutting down")
	})
}

func TestController_RunWorker(t *testing.T) {
	t.Run("RunWorkerWithEmptyQueue", func(t *testing.T) {
		controller := createTestController()

		// Shutdown the workqueue immediately
		controller.workqueue.ShutDown()

		// This should exit immediately without processing anything
		controller.runWorker()

		// If we get here, runWorker handled the empty/shutdown queue correctly
		assert.True(t, true, "runWorker completed without panicking on empty queue")
	})

	t.Run("RunWorkerBasicLoop", func(t *testing.T) {
		controller := createTestController()

		// Add an invalid item that will be forgotten quickly
		controller.workqueue.Add("invalid-item")

		// Shutdown after a moment
		go func() {
			time.Sleep(10 * time.Millisecond)
			controller.workqueue.ShutDown()
		}()

		// This should process the invalid item and then stop
		controller.runWorker()

		assert.True(t, true, "runWorker completed successfully")
	})
}

func TestController_AddOrchestrator(t *testing.T) {
	mockCR := &netappv1.TridentOrchestrator{}
	mockCR.Name = TestOrchestratorName
	mockCR.Namespace = TestNamespace

	controller := createTestController()
	controller.addOrchestrator(mockCR)
	assert.Equal(t, 1, controller.workqueue.Len())

	obj, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown)
	if keyItem, ok := obj.(KeyItem); ok {
		assert.Equal(t, ResourceTridentOrchestratorCR, keyItem.resourceType)
		assert.Contains(t, keyItem.keyDetails, TestOrchestratorName)
	}

	// Test invalid object type
	controller.addOrchestrator("invalid-object")
	assert.Equal(t, 0, controller.workqueue.Len())
}

func TestController_UpdateOrchestrator(t *testing.T) {
	oldCR := &netappv1.TridentOrchestrator{}
	oldCR.Name = TestOrchestratorName
	oldCR.Namespace = TestNamespace

	newCR := &netappv1.TridentOrchestrator{}
	newCR.Name = TestOrchestratorName
	newCR.Namespace = TestNamespace

	controller := createTestController()
	controller.updateOrchestrator(oldCR, newCR)
	assert.Equal(t, 1, controller.workqueue.Len())

	// Test with deletion timestamp
	deletingCR := &netappv1.TridentOrchestrator{}
	deletingCR.Name = TestOrchestratorName
	deletingCR.Namespace = TestNamespace
	deletingCR.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}

	controller.updateOrchestrator(oldCR, deletingCR)
	assert.Equal(t, 1, controller.workqueue.Len()) // No additional item

	// Test with invalid old object
	controller.updateOrchestrator("invalid-old-object", newCR)
	assert.Equal(t, 1, controller.workqueue.Len())

	// Test with invalid new object
	controller.updateOrchestrator(oldCR, "invalid-new-object")
	assert.Equal(t, 1, controller.workqueue.Len())
}

func TestController_DeleteOrchestrator(t *testing.T) {
	mockCR := &netappv1.TridentOrchestrator{}
	mockCR.Name = TestOrchestratorName
	mockCR.Namespace = TestNamespace

	controller := createTestController()
	controller.deleteOrchestrator(mockCR)
	assert.Equal(t, 1, controller.workqueue.Len())

	obj, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown)
	if keyItem, ok := obj.(KeyItem); ok {
		assert.Equal(t, ResourceTridentOrchestratorCR, keyItem.resourceType)
		assert.Contains(t, keyItem.keyDetails, TestOrchestratorName)
	}

	// Test invalid object type
	controller.deleteOrchestrator("invalid-object")
	assert.Equal(t, 0, controller.workqueue.Len())
}

// Phase 4: Test deployment event handlers
func TestController_DeploymentAddedOrDeleted(t *testing.T) {
	// Create a mock deployment
	deployment := &appsv1.Deployment{}
	deployment.Name = TestTridentCSIName
	deployment.Namespace = TestNamespace

	t.Run("DeploymentAddedOrDeletedValidDeployment", func(t *testing.T) {
		controller := createTestController()
		initialQueueLength := controller.workqueue.Len()

		// Call deploymentAddedOrDeleted
		controller.deploymentAddedOrDeleted(deployment)

		// Verify item was added to workqueue
		assert.Equal(t, initialQueueLength+1, controller.workqueue.Len(), "Workqueue should have one more item")

		// Get the item and verify
		obj, shutdown := controller.workqueue.Get()
		assert.False(t, shutdown, "Workqueue should not be shutdown")

		if keyItem, ok := obj.(KeyItem); ok {
			assert.Equal(t, ResourceDeployment, keyItem.resourceType, "Resource type should be Deployment")
			assert.Contains(t, keyItem.keyDetails, TestTridentCSIName, "Key should contain deployment name")
		} else {
			t.Errorf("Expected KeyItem, got %T", obj)
		}
	})

	t.Run("DeploymentAddedOrDeletedInvalidObject", func(t *testing.T) {
		controller := createTestController()
		initialQueueLength := controller.workqueue.Len()

		// Try with an invalid object
		controller.deploymentAddedOrDeleted("invalid-deployment")

		// Workqueue length should remain the same
		assert.Equal(t, initialQueueLength, controller.workqueue.Len(), "Workqueue should not change for invalid object")
	})
}

func TestController_DeploymentUpdated(t *testing.T) {
	// Create mock deployments with different resource versions AND meaningful differences
	oldDeployment := &appsv1.Deployment{}
	oldDeployment.Name = TestTridentCSIName
	oldDeployment.Namespace = TestNamespace
	oldDeployment.ResourceVersion = "1"
	oldDeployment.Spec.Replicas = &[]int32{1}[0] // 1 replica initially

	newDeployment := &appsv1.Deployment{}
	newDeployment.Name = TestTridentCSIName
	newDeployment.Namespace = TestNamespace
	newDeployment.ResourceVersion = "2"          // Different resource version to trigger update
	newDeployment.Spec.Replicas = &[]int32{2}[0] // 2 replicas - meaningful difference

	t.Run("DeploymentUpdatedValidDeployments", func(t *testing.T) {
		controller := createTestController()
		initialQueueLength := controller.workqueue.Len()

		// Call deploymentUpdated
		controller.deploymentUpdated(oldDeployment, newDeployment)

		// Verify item was added to workqueue
		assert.Equal(t, initialQueueLength+1, controller.workqueue.Len(), "Workqueue should have one more item")

		// Get the item and verify
		obj, shutdown := controller.workqueue.Get()
		assert.False(t, shutdown, "Workqueue should not be shutdown")

		if keyItem, ok := obj.(KeyItem); ok {
			assert.Equal(t, ResourceDeployment, keyItem.resourceType, "Resource type should be Deployment")
			assert.Contains(t, keyItem.keyDetails, TestTridentCSIName, "Key should contain deployment name")
		} else {
			t.Errorf("Expected KeyItem, got %T", obj)
		}
	})

	t.Run("DeploymentUpdatedSameResourceVersion", func(t *testing.T) {
		controller := createTestController()
		initialQueueLength := controller.workqueue.Len()

		// Create deployments with same resource version
		sameDeplOld := &appsv1.Deployment{}
		sameDeplOld.Name = TestTridentCSIName
		sameDeplOld.Namespace = TestNamespace
		sameDeplOld.ResourceVersion = "1"

		sameDeplNew := &appsv1.Deployment{}
		sameDeplNew.Name = TestTridentCSIName
		sameDeplNew.Namespace = TestNamespace
		sameDeplNew.ResourceVersion = "1" // Same resource version should be ignored

		// Call deploymentUpdated
		controller.deploymentUpdated(sameDeplOld, sameDeplNew)

		// Workqueue should remain unchanged due to same resource version check
		assert.Equal(t, initialQueueLength, controller.workqueue.Len(), "Workqueue should not change for same resource version")
	})

	t.Run("DeploymentUpdatedNoMeaningfulChange", func(t *testing.T) {
		controller := createTestController()
		initialQueueLength := controller.workqueue.Len()

		// Create deployments that differ only in resource version (no meaningful change)
		noDiffOld := &appsv1.Deployment{}
		noDiffOld.Name = TestTridentCSIName
		noDiffOld.Namespace = TestNamespace
		noDiffOld.ResourceVersion = "1"

		noDiffNew := &appsv1.Deployment{}
		noDiffNew.Name = TestTridentCSIName
		noDiffNew.Namespace = TestNamespace
		noDiffNew.ResourceVersion = "2" // Different RV but otherwise identical

		// Call deploymentUpdated
		controller.deploymentUpdated(noDiffOld, noDiffNew)

		// Should be ignored due to no meaningful changes after normalization
		assert.Equal(t, initialQueueLength, controller.workqueue.Len(), "Workqueue should not change for no meaningful differences")
	})
}

// Phase 5: Test daemonset event handlers
func TestController_DaemonsetAddedOrDeleted(t *testing.T) {
	// Create a daemonset with valid Trident-related name
	daemonset := &appsv1.DaemonSet{}
	daemonset.Name = TestTridentCSIName
	daemonset.Namespace = TestNamespace

	t.Run("DaemonsetAddedOrDeletedValidDaemonset", func(t *testing.T) {
		controller := createTestController()
		initialQueueLength := controller.workqueue.Len()

		// Call daemonsetAddedOrDeleted
		controller.daemonsetAddedOrDeleted(daemonset)

		// Verify item was added to workqueue
		assert.Equal(t, initialQueueLength+1, controller.workqueue.Len(), "Workqueue should have one more item")

		// Get the item and verify
		obj, shutdown := controller.workqueue.Get()
		assert.False(t, shutdown, "Workqueue should not be shutdown")

		if keyItem, ok := obj.(KeyItem); ok {
			assert.Equal(t, ResourceDaemonSet, keyItem.resourceType, "Resource type should be DaemonSet")
			assert.Contains(t, keyItem.keyDetails, TestTridentCSIName, "Key should contain daemonset name")
		} else {
			t.Errorf("Expected KeyItem, got %T", obj)
		}
	})

	t.Run("DaemonsetAddedOrDeletedInvalidObject", func(t *testing.T) {
		controller := createTestController()
		initialQueueLength := controller.workqueue.Len()

		// Try with an invalid object
		controller.daemonsetAddedOrDeleted("invalid-daemonset")

		// Workqueue length should remain the same since invalid object causes error
		assert.Equal(t, initialQueueLength, controller.workqueue.Len(), "Workqueue should not change for invalid object")
	})
}

func TestController_DaemonsetUpdated(t *testing.T) {
	// Create daemonsets
	oldDaemonset := &appsv1.DaemonSet{}
	oldDaemonset.Name = TestTridentCSIName
	oldDaemonset.Namespace = TestNamespace

	newDaemonset := &appsv1.DaemonSet{}
	newDaemonset.Name = TestTridentCSIName
	newDaemonset.Namespace = TestNamespace

	t.Run("DaemonsetUpdatedValidDaemonsets", func(t *testing.T) {
		controller := createTestController()
		initialQueueLength := controller.workqueue.Len()

		// Call daemonsetUpdated
		controller.daemonsetUpdated(oldDaemonset, newDaemonset)

		// Verify item was added to workqueue
		assert.Equal(t, initialQueueLength+1, controller.workqueue.Len(), "Workqueue should have one more item")

		// Get the item and verify
		obj, shutdown := controller.workqueue.Get()
		assert.False(t, shutdown, "Workqueue should not be shutdown")

		if keyItem, ok := obj.(KeyItem); ok {
			assert.Equal(t, ResourceDaemonSet, keyItem.resourceType, "Resource type should be DaemonSet")
			assert.Contains(t, keyItem.keyDetails, TestTridentCSIName, "Key should contain daemonset name")
		} else {
			t.Errorf("Expected KeyItem, got %T", obj)
		}
	})

	t.Run("DaemonsetUpdatedInvalidNewDaemonset", func(t *testing.T) {
		controller := createTestController()
		initialQueueLength := controller.workqueue.Len()

		// Try with invalid new daemonset
		controller.daemonsetUpdated(oldDaemonset, "invalid-daemonset")

		// Workqueue should remain unchanged since invalid object causes error
		assert.Equal(t, initialQueueLength, controller.workqueue.Len(), "Workqueue should not change for invalid new object")
	})
}

// createMockController creates a fully mocked controller for testing complex functions
func createMockController(t *testing.T, ctrl *gomock.Controller) (*Controller, *mockK8sClient.MockKubernetesClient, *crdFake.Clientset, *fake.Clientset) {
	mockK8SClient := mockK8sClient.NewMockKubernetesClient(ctrl)
	fakeCRDClient := crdFake.NewSimpleClientset()
	fakeKubeClient := fake.NewSimpleClientset()

	// Create controller with mocked clients
	clientsStruct := &clients.Clients{
		K8SClient: mockK8SClient,
		K8SVersion: &version.Info{
			Major:      "1",
			Minor:      "20",
			GitVersion: "v1.20.0",
		},
		Namespace: TestNamespace,
	}

	controller := &Controller{
		Clients:   clientsStruct,
		stopChan:  make(chan struct{}),
		workqueue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		mutex:     &sync.Mutex{},
	}

	return controller, mockK8SClient, fakeCRDClient, fakeKubeClient
}

func TestController_DoesCRDExist_WithMocking(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	controller, mockK8SClient, _, _ := createMockController(t, ctrl)

	t.Run("CRDExistsTrue", func(t *testing.T) {
		mockK8SClient.EXPECT().
			CheckCRDExists("tridentorchestrators.trident.netapp.io").
			Return(true, nil)

		exists, err := controller.doesCRDExist("tridentorchestrators.trident.netapp.io")
		assert.NoError(t, err, "Should not error when checking existing CRD")
		assert.True(t, exists, "CRD should exist when mock returns true")
	})

	t.Run("CRDExistsFalse", func(t *testing.T) {
		mockK8SClient.EXPECT().
			CheckCRDExists("nonexistent.crd.io").
			Return(false, nil)

		exists, err := controller.doesCRDExist("nonexistent.crd.io")
		assert.NoError(t, err, "Should not error when checking non-existent CRD")
		assert.False(t, exists, "CRD should not exist when mock returns false")
	})

	t.Run("CRDCheckError", func(t *testing.T) {
		mockK8SClient.EXPECT().
			CheckCRDExists("error.crd.io").
			Return(false, stderrors.New("API server error"))

		exists, err := controller.doesCRDExist("error.crd.io")
		assert.Error(t, err, "Should return error when CRD check fails")
		assert.False(t, exists)
		assert.Contains(t, err.Error(), "API server error")
	})
}

func TestController_RemoveNonTorcBasedCSIInstallation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	controller, mockK8SClient, _, _ := createMockController(t, ctrl)

	// Create a test TridentOrchestrator CR
	tridentCR := &netappv1.TridentOrchestrator{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestOrchestratorName,
			Namespace: TestNamespace,
		},
	}

	t.Run("NoCSIInstallationFound", func(t *testing.T) {
		// Mock isCSITridentInstalled to return false (no installation found)
		mockK8SClient.EXPECT().
			CheckDeploymentExistsByLabel("app=controller.csi.trident.netapp.io", true).
			Return(false, "", nil)

		// Call the function - should not need to uninstall anything
		err := controller.removeNonTorcBasedCSIInstallation(tridentCR)
		// Should succeed because no uninstallation is required
		assert.NoError(t, err, "Should not error when no CSI installation found")
	})

	t.Run("CSIInstallationCheckError", func(t *testing.T) {
		// Mock isCSITridentInstalled to return an error
		mockK8SClient.EXPECT().
			CheckDeploymentExistsByLabel("app=controller.csi.trident.netapp.io", true).
			Return(false, "", stderrors.New(TestAPIErrorMessage))

		// Call the function - should return wrapped error
		err := controller.removeNonTorcBasedCSIInstallation(tridentCR)
		assert.Error(t, err, "Should return error when CSI installation check fails")
		assert.Contains(t, err.Error(), "reconcile failed")
	})
}

func TestController_IsCSITridentInstalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	controller, mockK8SClient, _, _ := createMockController(t, ctrl)

	t.Run("TridentInstalledTrue", func(t *testing.T) {
		mockK8SClient.EXPECT().
			CheckDeploymentExistsByLabel("app=controller.csi.trident.netapp.io", true).
			Return(true, TestTridentCSIName, nil)

		installed, namespace, err := controller.isCSITridentInstalled()
		assert.NoError(t, err, "Should not error when checking Trident installation")
		assert.True(t, installed, "Trident should be installed when mock returns true")
		assert.Equal(t, TestTridentCSIName, namespace, "Should return correct namespace when Trident is installed")
	})

	t.Run("TridentInstalledFalse", func(t *testing.T) {
		mockK8SClient.EXPECT().
			CheckDeploymentExistsByLabel("app=controller.csi.trident.netapp.io", true).
			Return(false, "", nil)

		installed, namespace, err := controller.isCSITridentInstalled()
		assert.NoError(t, err, "Should not error when Trident is not installed")
		assert.False(t, installed, "Trident should not be installed when mock returns false")
		assert.Empty(t, namespace, "Namespace should be empty when Trident is not installed")
	})

	t.Run("TridentInstallationCheckError", func(t *testing.T) {
		mockK8SClient.EXPECT().
			CheckDeploymentExistsByLabel("app=controller.csi.trident.netapp.io", true).
			Return(false, "", stderrors.New(TestAPIErrorMessage))

		installed, namespace, err := controller.isCSITridentInstalled()
		assert.Error(t, err, "Should return error when deployment check fails")
		assert.False(t, installed)
		assert.Empty(t, namespace)
	})
}

func TestController_ResourceTypeToK8sKind_WithMocking(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	controller, _, _, _ := createMockController(t, ctrl)

	testCases := []struct {
		resourceType ResourceType
		expectedKind string
	}{
		{ResourceTridentOrchestratorCR, "Trident Orchestrator CR"},
		{ResourceDeployment, "deployment"},
		{ResourceDaemonSet, "daemonset"},
		{ResourceType("invalid"), "invalid object"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.resourceType), func(t *testing.T) {
			kind := controller.resourceTypeToK8sKind(tc.resourceType)
			assert.Equal(t, tc.expectedKind, kind)
		})
	}
}

func TestController_TridentUpgradeNeeded(t *testing.T) {
	controller := createTestController()

	// Test with empty version string (should return false immediately)
	result := controller.tridentUpgradeNeeded("")
	assert.False(t, result, "Should return false for empty version")

	// Test with same version (should return false)
	result = controller.tridentUpgradeNeeded("v1.20.0")
	assert.False(t, result, "Should return false for same version")

	// Test with different version (should return true)
	result = controller.tridentUpgradeNeeded("v1.19.0")
	assert.True(t, result, "Should return true for different version")
}

func TestController_Wipeout(t *testing.T) {
	controller := createTestController()

	// Test with empty wipeout list - should return immediately
	mockCR := netappv1.TridentOrchestrator{}
	mockCR.Name = TestOrchestratorName
	mockCR.Namespace = TestNamespace
	// Spec.Wipeout is empty by default

	deletedCRDs, err := controller.wipeout(mockCR)
	assert.NoError(t, err, "Should not return error for empty wipeout list")
	assert.False(t, deletedCRDs, "Should return false when no CRDs deleted")

	// Test with wipeout list containing invalid entry
	mockCR.Spec.Wipeout = []string{"invalid-entry"}
	deletedCRDs, err = controller.wipeout(mockCR)
	assert.NoError(t, err, "Should not return error for invalid wipeout entry")
	assert.False(t, deletedCRDs, "Should return false when no CRDs deleted")
}

func TestController_MatchDeploymentControllingCR(t *testing.T) {
	controller := createTestController()

	// Create a test deployment
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestTridentCSIName,
			Namespace: TestNamespace,
			UID:       "deployment-uid-123",
		},
	}

	t.Run("NoControllingCR", func(t *testing.T) {
		// Create CRs that don't control the deployment
		cr1 := &netappv1.TridentOrchestrator{
			ObjectMeta: metav1.ObjectMeta{
				Name: "trident-1",
				UID:  "cr-uid-1",
			},
		}
		cr2 := &netappv1.TridentOrchestrator{
			ObjectMeta: metav1.ObjectMeta{
				Name: "trident-2",
				UID:  "cr-uid-2",
			},
		}

		tridentCRs := []*netappv1.TridentOrchestrator{cr1, cr2}

		result, err := controller.matchDeploymentControllingCR(tridentCRs, deployment)
		assert.NoError(t, err, "Should not error when no controlling CR found")
		assert.Nil(t, result, "Should return nil when no controlling CR found")
	})

	t.Run("WithControllingCR", func(t *testing.T) {
		// Create a CR that controls the deployment
		controllingCR := &netappv1.TridentOrchestrator{
			ObjectMeta: metav1.ObjectMeta{
				Name: "controlling-trident",
				UID:  "controlling-cr-uid",
			},
		}

		// Set up the deployment to be controlled by this CR
		trueVal := true
		deployment.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: "trident.netapp.io/v1",
				Kind:       "TridentOrchestrator",
				Name:       controllingCR.Name,
				UID:        controllingCR.UID,
				Controller: &trueVal,
			},
		}

		// Create other non-controlling CRs
		nonControllingCR := &netappv1.TridentOrchestrator{
			ObjectMeta: metav1.ObjectMeta{
				Name: "non-controlling-trident",
				UID:  "non-controlling-cr-uid",
			},
		}

		tridentCRs := []*netappv1.TridentOrchestrator{nonControllingCR, controllingCR}

		result, err := controller.matchDeploymentControllingCR(tridentCRs, deployment)
		assert.NoError(t, err, "Should not error when controlling CR found")
		assert.NotNil(t, result, "Should return controlling CR")
		assert.Equal(t, "controlling-trident", result.Name, "Should return the correct controlling CR")
		assert.Equal(t, controllingCR.UID, result.UID, "Should return CR with correct UID")
	})

	t.Run("EmptyCRList", func(t *testing.T) {
		var emptyCRs []*netappv1.TridentOrchestrator

		result, err := controller.matchDeploymentControllingCR(emptyCRs, deployment)
		assert.NoError(t, err, "Should not error with empty CR list")
		assert.Nil(t, result, "Should return nil with empty CR list")
	})

	t.Run("MultipleNonControllingCRs", func(t *testing.T) {
		// Create multiple CRs, none of which control the deployment
		crs := make([]*netappv1.TridentOrchestrator, 5)
		for i := 0; i < 5; i++ {
			crs[i] = &netappv1.TridentOrchestrator{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("trident-%d", i),
					UID:  types.UID(fmt.Sprintf("cr-uid-%d", i)),
				},
			}
		}

		// Clear owner references to ensure no control relationship
		deployment.ObjectMeta.OwnerReferences = nil

		result, err := controller.matchDeploymentControllingCR(crs, deployment)
		assert.NoError(t, err, "Should not error with multiple non-controlling CRs")
		assert.Nil(t, result, "Should return nil when no CR controls deployment")
	})
}

func TestController_GetCRDBasedCSIDeployments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	controller, mockK8SClient, _, _ := createMockController(t, ctrl)

	t.Run("GetCRDBasedCSIDeploymentsNoDeployments", func(t *testing.T) {
		// Mock GetDeploymentsByLabel to return no deployments
		mockK8SClient.EXPECT().
			GetDeploymentsByLabel("app=controller.csi.trident.netapp.io", true).
			Return([]appsv1.Deployment{}, nil)

		deployments, namespace, err := controller.getCRDBasedCSIDeployments("TridentOrchestrator")
		assert.NoError(t, err, "Should not error when no deployments found")
		assert.Empty(t, deployments, "Should return empty deployment list")
		assert.Empty(t, namespace, "Should return empty namespace")
	})

	t.Run("GetCRDBasedCSIDeploymentsWithError", func(t *testing.T) {
		// Mock GetDeploymentsByLabel to return an error
		mockK8SClient.EXPECT().
			GetDeploymentsByLabel("app=controller.csi.trident.netapp.io", true).
			Return([]appsv1.Deployment{}, stderrors.New(TestAPIErrorMessage))

		deployments, namespace, err := controller.getCRDBasedCSIDeployments("TridentOrchestrator")
		assert.Error(t, err, "Should return error when API call fails")
		assert.Empty(t, deployments, "Should return empty deployment list on error")
		assert.Empty(t, namespace, "Should return empty namespace on error")
		assert.Contains(t, err.Error(), "unable to get list of deployments", "Error should contain expected message")
	})

	t.Run("GetCRDBasedCSIDeploymentsWithValidDeployment", func(t *testing.T) {
		// Create a deployment with proper owner reference
		deployment := appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      TestTridentCSIName,
				Namespace: TestNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "TridentOrchestrator",
						Name: TestOrchestratorName,
					},
				},
			},
		}

		mockK8SClient.EXPECT().
			GetDeploymentsByLabel("app=controller.csi.trident.netapp.io", true).
			Return([]appsv1.Deployment{deployment}, nil)

		deployments, namespace, err := controller.getCRDBasedCSIDeployments("TridentOrchestrator")
		assert.NoError(t, err, "Should not error with valid deployment")
		assert.Len(t, deployments, 1, "Should return one deployment")
		assert.Equal(t, TestNamespace, namespace, "Should return correct namespace")
		assert.Equal(t, TestTridentCSIName, deployments[0].Name, "Should return correct deployment name")
	})

	t.Run("GetCRDBasedCSIDeploymentsWithMultipleDeployments", func(t *testing.T) {
		// Create multiple deployments with proper owner references
		// Note: The function has a break statement, so it only processes the first matching deployment
		deployment1 := appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "trident-csi-1",
				Namespace: TestNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "TridentOrchestrator",
						Name: "test-orchestrator-1",
					},
				},
			},
		}
		deployment2 := appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "trident-csi-2",
				Namespace: "trident2",
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "TridentOrchestrator",
						Name: "test-orchestrator-2",
					},
				},
			},
		}

		mockK8SClient.EXPECT().
			GetDeploymentsByLabel("app=controller.csi.trident.netapp.io", true).
			Return([]appsv1.Deployment{deployment1, deployment2}, nil)

		deployments, namespace, err := controller.getCRDBasedCSIDeployments("TridentOrchestrator")
		assert.NoError(t, err, "Should not error with multiple deployments")
		// The function breaks after finding the first match, so we get only 1 deployment
		assert.Len(t, deployments, 1, "Should return one deployment due to break statement")
		assert.Equal(t, TestNamespace, namespace, "Should return first deployment's namespace")
	})

	t.Run("GetCRDBasedCSIDeploymentsWithNonMatchingOwner", func(t *testing.T) {
		// Create a deployment with non-matching owner reference
		deployment := appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      TestTridentCSIName,
				Namespace: TestNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						Kind: "SomeOtherKind",
						Name: TestOrchestratorName,
					},
				},
			},
		}

		mockK8SClient.EXPECT().
			GetDeploymentsByLabel("app=controller.csi.trident.netapp.io", true).
			Return([]appsv1.Deployment{deployment}, nil)

		deployments, namespace, err := controller.getCRDBasedCSIDeployments("TridentOrchestrator")
		assert.NoError(t, err, "Should not error with non-matching owner")
		assert.Empty(t, deployments, "Should return empty deployment list for non-matching owner")
		assert.Empty(t, namespace, "Should return empty namespace for non-matching owner")
	})
}

func TestController_GetTridentOrchestratorCSIDeployments(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	controller, mockK8SClient, _, _ := createMockController(t, ctrl)

	t.Run("GetTridentOrchestratorCSIDeploymentsCallsCRDBasedFunction", func(t *testing.T) {
		// This function should just delegate to getCRDBasedCSIDeployments
		mockK8SClient.EXPECT().
			GetDeploymentsByLabel("app=controller.csi.trident.netapp.io", true).
			Return([]appsv1.Deployment{}, nil)

		deployments, namespace, err := controller.getTridentOrchestratorCSIDeployments()
		assert.NoError(t, err, "Should not error")
		assert.Empty(t, deployments, "Should return empty deployment list")
		assert.Empty(t, namespace, "Should return empty namespace")
	})
}

func TestController_Deactivate(t *testing.T) {
	t.Run("DeactivateWithProperSetup", func(t *testing.T) {
		controller := createTestController()
		controller.stopChan = make(chan struct{})
		controller.workqueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

		controller.Deactivate()

		// Verify stopChan was closed
		select {
		case <-controller.stopChan:
			// Good - channel was closed
		default:
			t.Error("stopChan should have been closed")
		}

		assert.True(t, controller.workqueue.ShuttingDown(), "Workqueue should be shutting down")
	})

	t.Run("DeactivateAlreadyClosed", func(t *testing.T) {
		controller := createTestController()
		controller.stopChan = make(chan struct{})
		controller.workqueue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

		// First deactivation
		controller.Deactivate()

		// Second deactivation should panic (close of closed channel)
		assert.Panics(t, func() {
			controller.Deactivate()
		}, "Second deactivation should panic")
	})
}

func TestController_IdentifyControllingCRForTridentDeployments(t *testing.T) {
	t.Run("IdentifyControllingCRForMultipleDeployments", func(t *testing.T) {
		controller := createTestController()

		deployment1 := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "trident-csi-1"}}
		deployment2 := appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "trident-csi-2"}}

		result, err := controller.identifyControllingCRForTridentDeployments([]appsv1.Deployment{deployment1, deployment2})
		assert.NoError(t, err, "Should not error when identifying controlling CR for multiple deployments")
		assert.Nil(t, result, "Should return nil for multiple deployments")
	})
}
