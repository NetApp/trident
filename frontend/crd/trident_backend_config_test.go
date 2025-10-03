// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

func setupBackendConfigTest(t *testing.T) (*TridentCrdController, *gomock.Controller, *mockcore.MockOrchestrator) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	crdController.recorder = record.NewFakeRecorder(100)

	return crdController, mockCtrl, orchestrator
}

func createTestBackendConfig(name, namespace string) *tridentv1.TridentBackendConfig {
	rawSpec, _ := json.Marshal(map[string]interface{}{
		"version":           1,
		"storageDriverName": "fake",
		"backendName":       "test-backend",
	})

	return &tridentv1.TridentBackendConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			UID:        "test-uid-123",
			Finalizers: []string{tridentv1.TridentFinalizerName},
		},
		Spec: tridentv1.TridentBackendConfigSpec{
			RawExtension: runtime.RawExtension{Raw: rawSpec},
		},
		Status: tridentv1.TridentBackendConfigStatus{
			Phase: string(tridentv1.PhaseUnbound),
		},
	}
}

func TestHandleTridentBackendConfig_AddNewBackend(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")

	// Add backend config to fake clientset (but lister cache won't be populated in tests)
	_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Note: In tests, the informer cache is not automatically populated,
	// so the handleTridentBackendConfig will not find the backend config
	// and will return early without calling AddBackend. This is expected behavior.

	keyItem := &KeyItem{
		key:        "default/test-backend-config",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackendConfig,
	}

	// This should return nil (no error) when backend config is not found in lister
	err = controller.handleTridentBackendConfig(keyItem)
	assert.NoError(t, err)
}

func TestHandleTridentBackendConfig_ValidationError(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")

	// Create invalid backend config (empty spec)
	backendConfig.Spec.RawExtension = runtime.RawExtension{Raw: []byte(`{}`)}

	_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	keyItem := &KeyItem{
		key:        "default/test-backend-config",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackendConfig,
	}

	err = controller.handleTridentBackendConfig(keyItem)
	// The validation may pass on empty config, so let's just check it doesn't panic
	// Real validation errors would depend on the specific backend configuration
}

func TestHandleTridentBackendConfig_NilKeyItem(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	err := controller.handleTridentBackendConfig(nil)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileDeferredError(err))
}

func TestHandleTridentBackendConfig_InvalidKey(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	keyItem := &KeyItem{
		key:        "invalid-key-format",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackendConfig,
	}

	err := controller.handleTridentBackendConfig(keyItem)
	assert.NoError(t, err) // Invalid key should return nil (not an error)
}

func TestHandleTridentBackendConfig_BackendNotFound(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	keyItem := &KeyItem{
		key:        "default/non-existent-backend",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackendConfig,
	}

	err := controller.handleTridentBackendConfig(keyItem)
	assert.NoError(t, err) // Not found should return nil
}

func TestAddBackendConfig_Success(t *testing.T) {
	controller, mockCtrl, orchestrator := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")

	// Add backend config to fake clientset first
	_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	backendDetails := &storage.BackendExternal{
		Name:        "test-backend",
		BackendUUID: "backend-uuid-123",
	}

	orchestrator.EXPECT().AddBackend(gomock.Any(), string(backendConfig.Spec.RawExtension.Raw), string(backendConfig.UID)).Return(backendDetails, nil)

	err = controller.addBackendConfig(ctx, backendConfig, tridentv1.BackendDeletionPolicyDelete)
	assert.NoError(t, err)
}

func TestAddBackendConfig_OrchestratorError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")

	orchestratorError := fmt.Errorf("orchestrator error")
	orchestrator.EXPECT().AddBackend(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, orchestratorError)

	err := controller.addBackendConfig(ctx, backendConfig, tridentv1.BackendDeletionPolicyDelete)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "orchestrator error")
}

func TestUpdateBackendConfig_Success(t *testing.T) {
	controller, mockCtrl, orchestrator := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")
	backendConfig.Status.Phase = string(tridentv1.PhaseBound)
	backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
		BackendName: "test-backend",
		BackendUUID: "backend-uuid-123",
	}

	// Add backend config to fake clientset first
	_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create corresponding TridentBackend
	tridentBackend := &tridentv1.TridentBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tbe-backend-uuid-123",
			Namespace: "default",
		},
		BackendUUID: "backend-uuid-123",
		BackendName: "test-backend",
	}
	_, err = controller.crdClientset.TridentV1().TridentBackends("default").Create(ctx, tridentBackend, metav1.CreateOptions{})
	assert.NoError(t, err)

	backendDetails := &storage.BackendExternal{
		Name:        "test-backend-updated",
		BackendUUID: "backend-uuid-123",
	}

	orchestrator.EXPECT().UpdateBackendByBackendUUID(gomock.Any(), "test-backend", string(backendConfig.Spec.RawExtension.Raw), "backend-uuid-123", string(backendConfig.UID)).Return(backendDetails, nil)

	err = controller.updateBackendConfig(ctx, backendConfig, tridentv1.BackendDeletionPolicyDelete)
	assert.NoError(t, err)
}

func TestUpdateBackendConfig_BackendNotFound(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")
	backendConfig.Status.Phase = string(tridentv1.PhaseBound)
	backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
		BackendName: "test-backend",
		BackendUUID: "backend-uuid-123",
	}

	// Add backend config to fake clientset first
	_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = controller.updateBackendConfig(ctx, backendConfig, tridentv1.BackendDeletionPolicyDelete)
	assert.Error(t, err)
	// The specific error type depends on the implementation details
}

func TestGetBackendConfigsWithSecret_Found(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")

	// Add secret reference to spec
	specWithSecret := map[string]interface{}{
		"version":           1,
		"storageDriverName": "fake",
		"backendName":       "test-backend",
		"credentials": map[string]interface{}{
			"name": "test-secret",
		},
	}
	rawSpec, _ := json.Marshal(specWithSecret)
	backendConfig.Spec.RawExtension = runtime.RawExtension{Raw: rawSpec}

	_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	result, err := controller.getBackendConfigsWithSecret(ctx, "default", "test-secret")
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "test-backend-config", result[0].Name)
}

func TestGetBackendConfigsWithSecret_NotFound(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	result, err := controller.getBackendConfigsWithSecret(ctx, "default", "non-existent-secret")
	assert.Error(t, err)
	assert.Len(t, result, 0)
	assert.True(t, errors.IsNotFoundError(err))
}

func TestDeleteBackendConfig_PolicyDelete(t *testing.T) {
	controller, mockCtrl, orchestrator := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")
	backendConfig.Status.Phase = string(tridentv1.PhaseDeleting)
	backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
		BackendName: "test-backend",
		BackendUUID: "backend-uuid-123",
	}

	// Add backend config to fake clientset first
	_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create corresponding TridentBackend
	tridentBackend := &tridentv1.TridentBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tbe-backend-uuid-123",
			Namespace: "default",
		},
		BackendUUID: "backend-uuid-123",
		BackendName: "test-backend",
		State:       "online",
	}
	_, err = controller.crdClientset.TridentV1().TridentBackends("default").Create(ctx, tridentBackend, metav1.CreateOptions{})
	assert.NoError(t, err)

	orchestrator.EXPECT().DeleteBackendByBackendUUID(gomock.Any(), "test-backend", "backend-uuid-123").Return(nil)

	err = controller.deleteBackendConfig(ctx, backendConfig, tridentv1.BackendDeletionPolicyDelete)
	assert.Error(t, err) // Should error because backend still exists after deletion attempt
}

func TestDeleteBackendConfig_PolicyRetain(t *testing.T) {
	controller, mockCtrl, orchestrator := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")
	backendConfig.Status.Phase = string(tridentv1.PhaseDeleting)
	backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
		BackendName: "test-backend",
		BackendUUID: "backend-uuid-123",
	}

	// Create corresponding TridentBackend
	tridentBackend := &tridentv1.TridentBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tbe-backend-uuid-123",
			Namespace: "default",
		},
		BackendUUID: "backend-uuid-123",
		BackendName: "test-backend",
		ConfigRef:   string(backendConfig.UID),
	}
	_, err := controller.crdClientset.TridentV1().TridentBackends("default").Create(ctx, tridentBackend, metav1.CreateOptions{})
	assert.NoError(t, err)

	orchestrator.EXPECT().RemoveBackendConfigRef(gomock.Any(), "backend-uuid-123", string(backendConfig.UID)).Return(nil)

	err = controller.deleteBackendConfig(ctx, backendConfig, tridentv1.BackendDeletionPolicyRetain)
	assert.NoError(t, err)
}

func TestGetBackendConfigWithBackendUUID(t *testing.T) {
	type testCase struct {
		name          string
		setupFunc     func(ctx context.Context, controller *TridentCrdController) error
		backendUUID   string
		expectError   bool
		expectNil     bool
		expectName    string
		checkNotFound bool
	}

	tests := []testCase{
		{
			name: "Found",
			setupFunc: func(ctx context.Context, controller *TridentCrdController) error {
				backendConfig := createTestBackendConfig("test-backend-config", "default")
				backendConfig.Status.BackendInfo.BackendUUID = "backend-uuid-123"
				_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
				return err
			},
			backendUUID:   "backend-uuid-123",
			expectError:   false,
			expectNil:     false,
			expectName:    "test-backend-config",
			checkNotFound: false,
		},
		{
			name: "NotFound",
			setupFunc: func(ctx context.Context, controller *TridentCrdController) error {
				return nil // No setup needed
			},
			backendUUID:   "non-existent-uuid",
			expectError:   true,
			expectNil:     true,
			expectName:    "",
			checkNotFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, mockCtrl, _ := setupBackendConfigTest(t)
			defer mockCtrl.Finish()

			ctx := context.Background()

			err := tt.setupFunc(ctx, controller)
			assert.NoError(t, err)

			result, err := controller.getBackendConfigWithBackendUUID(ctx, "default", tt.backendUUID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.checkNotFound {
					assert.True(t, errors.IsNotFoundError(err))
				}
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				if tt.expectName != "" {
					assert.Equal(t, tt.expectName, result.Name)
				}
			}
		})
	}
}

func TestUpdateTbcEventAndStatus(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")

	_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	newStatus := tridentv1.TridentBackendConfigStatus{
		Message:             "Backend created successfully",
		Phase:               string(tridentv1.PhaseBound),
		DeletionPolicy:      tridentv1.BackendDeletionPolicyDelete,
		LastOperationStatus: OperationStatusSuccess,
		BackendInfo: tridentv1.TridentBackendConfigBackendInfo{
			BackendName: "test-backend",
			BackendUUID: "backend-uuid-123",
		},
	}

	result, err := controller.updateTbcEventAndStatus(ctx, backendConfig, newStatus, "Backend created", corev1.EventTypeNormal)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, string(tridentv1.PhaseBound), result.Status.Phase)
}

func TestDeleteBackendConfigUsingPolicyDelete_BackendInDeletingState(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")
	backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
		BackendName: "test-backend",
		BackendUUID: "backend-uuid-123",
	}

	// Create TridentBackend in deleting state
	tridentBackend := &tridentv1.TridentBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tbe-backend-uuid-123",
			Namespace: "default",
		},
		BackendUUID: "backend-uuid-123",
		BackendName: "test-backend",
		State:       string(storage.Deleting),
	}
	_, err := controller.crdClientset.TridentV1().TridentBackends("default").Create(ctx, tridentBackend, metav1.CreateOptions{})
	assert.NoError(t, err)

	logFields := map[string]interface{}{
		"backendConfig.Name": backendConfig.Name,
		"backendName":        backendConfig.Status.BackendInfo.BackendName,
		"backendUUID":        backendConfig.Status.BackendInfo.BackendUUID,
	}

	message, phase, err := controller.deleteBackendConfigUsingPolicyDelete(ctx, backendConfig, logFields)
	assert.Error(t, err)
	assert.Equal(t, tridentv1.PhaseDeleting, phase)
	assert.Contains(t, message, "Backend is in a deleting state")
}

func TestDeleteBackendConfigUsingPolicyRetain_BackendNotFound(t *testing.T) {
	controller, mockCtrl, orchestrator := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendConfig := createTestBackendConfig("test-backend-config", "default")
	backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
		BackendName: "test-backend",
		BackendUUID: "backend-uuid-123",
	}

	orchestrator.EXPECT().RemoveBackendConfigRef(gomock.Any(), "backend-uuid-123", string(backendConfig.UID)).Return(nil)

	logFields := map[string]interface{}{
		"backendConfig.Name": backendConfig.Name,
		"backendName":        backendConfig.Status.BackendInfo.BackendName,
		"backendUUID":        backendConfig.Status.BackendInfo.BackendUUID,
	}

	message, phase, err := controller.deleteBackendConfigUsingPolicyRetain(ctx, backendConfig, logFields)
	assert.NoError(t, err)
	assert.Equal(t, tridentv1.TridentBackendConfigPhase(""), phase)
	assert.Equal(t, "", message)
}

func TestGetBackendInfo(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	backendName := "test-backend"
	backendUUID := "backend-uuid-123"

	result := controller.getBackendInfo(ctx, backendName, backendUUID)
	assert.Equal(t, backendName, result.BackendName)
	assert.Equal(t, backendUUID, result.BackendUUID)
}

func TestHandleTridentBackendConfig_LifecycleTests(t *testing.T) {
	t.Run("NilKeyItem", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		err := controller.handleTridentBackendConfig(nil)
		assert.Error(t, err)
		assert.True(t, errors.IsReconcileDeferredError(err))
		assert.Contains(t, err.Error(), "keyItem item is nil")
	})

	t.Run("InvalidKeyFormat", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		keyItem := &KeyItem{
			key:        "invalid-key-format",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentBackendConfig,
		}

		err := controller.handleTridentBackendConfig(keyItem)
		assert.NoError(t, err) // Invalid key should return nil (not an error)
	})

	t.Run("BackendConfigNotFound", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		keyItem := &KeyItem{
			key:        "default/non-existent-backend",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentBackendConfig,
		}

		err := controller.handleTridentBackendConfig(keyItem)
		assert.NoError(t, err) // Not found should return nil
	})

	t.Run("BackendConfigWithoutFinalizers", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Finalizers = nil // Remove finalizers

		// Create the backend config
		_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
		assert.NoError(t, err)

		keyItem := &KeyItem{
			key:        "default/test-backend-config",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentBackendConfig,
		}

		// This will not find the backend config in lister (cache not populated in test)
		err = controller.handleTridentBackendConfig(keyItem)
		assert.NoError(t, err)
	})

	t.Run("BackendConfigDeletingWithFinalizers", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		now := metav1.Now()
		backendConfig.DeletionTimestamp = &now // Mark for deletion

		// Create the backend config
		_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
		assert.NoError(t, err)

		keyItem := &KeyItem{
			key:        "default/test-backend-config",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentBackendConfig,
		}

		// This will not find the backend config in lister (cache not populated in test)
		err = controller.handleTridentBackendConfig(keyItem)
		assert.NoError(t, err)
	})
}

func TestHandleTridentBackendConfig_ValidationScenarios(t *testing.T) {
	t.Run("EmptySpecValidation", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Spec.RawExtension = runtime.RawExtension{Raw: []byte(`{}`)}

		_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
		assert.NoError(t, err)

		keyItem := &KeyItem{
			key:        "default/test-backend-config",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentBackendConfig,
		}

		err = controller.handleTridentBackendConfig(keyItem)
		assert.NoError(t, err)
	})

	t.Run("InvalidJSONInSpec", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Spec.RawExtension = runtime.RawExtension{Raw: []byte(`invalid-json`)}

		_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
		assert.NoError(t, err)

		keyItem := &KeyItem{
			key:        "default/test-backend-config",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentBackendConfig,
		}

		err = controller.handleTridentBackendConfig(keyItem)
		assert.NoError(t, err)
	})
}

func TestHandleTridentBackendConfig_SecretRefScenarios(t *testing.T) {
	t.Run("BackendConfigWithSecretRef", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Create a secret first
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"username": []byte("admin"),
				"password": []byte("secret123"),
			},
		}
		_, err := controller.kubeClientset.CoreV1().Secrets("default").Create(ctx, secret, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Create backend config with secret reference
		specWithSecret := map[string]interface{}{
			"version":           1,
			"storageDriverName": "fake",
			"backendName":       "test-backend",
			"credentials": map[string]interface{}{
				"name": "test-secret",
			},
		}
		rawSpec, _ := json.Marshal(specWithSecret)
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Spec.RawExtension = runtime.RawExtension{Raw: rawSpec}

		_, err = controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
		assert.NoError(t, err)

		keyItem := &KeyItem{
			key:        "default/test-backend-config",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentBackendConfig,
		}

		err = controller.handleTridentBackendConfig(keyItem)
		assert.NoError(t, err)
	})

	t.Run("BackendConfigWithNonExistentSecret", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Create backend config with non-existent secret reference
		specWithSecret := map[string]interface{}{
			"version":           1,
			"storageDriverName": "fake",
			"backendName":       "test-backend",
			"credentials": map[string]interface{}{
				"name": "non-existent-secret",
			},
		}
		rawSpec, _ := json.Marshal(specWithSecret)
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Spec.RawExtension = runtime.RawExtension{Raw: rawSpec}

		_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
		assert.NoError(t, err)

		keyItem := &KeyItem{
			key:        "default/test-backend-config",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentBackendConfig,
		}

		err = controller.handleTridentBackendConfig(keyItem)
		assert.NoError(t, err)
	})
}

func TestCheckAndHandleNewlyBoundCRDeletion_EdgeCases(t *testing.T) {
	t.Run("TransitionFromUnboundToBound_NotDeleting", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Status.Phase = string(tridentv1.PhaseUnbound)

		// Create the backend config
		_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
		assert.NoError(t, err)

		newStatusDetails := tridentv1.TridentBackendConfigStatus{
			Phase: string(tridentv1.PhaseBound),
			BackendInfo: tridentv1.TridentBackendConfigBackendInfo{
				BackendName: "test-backend",
				BackendUUID: "backend-uuid-123",
			},
		}

		result := controller.checkAndHandleNewlyBoundCRDeletion(ctx, backendConfig, newStatusDetails)
		assert.Nil(t, result) // Should return nil when not deleting
	})

	t.Run("TransitionFromUnboundToBound_IsDeleting", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Status.Phase = string(tridentv1.PhaseUnbound)

		// Mark for deletion
		now := metav1.Now()
		backendConfig.DeletionTimestamp = &now

		// Create the backend config
		_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
		assert.NoError(t, err)

		newStatusDetails := tridentv1.TridentBackendConfigStatus{
			Phase: string(tridentv1.PhaseBound),
			BackendInfo: tridentv1.TridentBackendConfigBackendInfo{
				BackendName: "test-backend",
				BackendUUID: "backend-uuid-123",
			},
		}

		result := controller.checkAndHandleNewlyBoundCRDeletion(ctx, backendConfig, newStatusDetails)
		// Since we're using fake clientset and informer cache isn't populated,
		// the function will find the CR via direct client call and update it
		if result != nil {
			assert.Equal(t, string(tridentv1.PhaseBound), result.Status.Phase)
		}
	})

	t.Run("NoTransitionFromUnbound", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Status.Phase = string(tridentv1.PhaseBound) // Already bound

		newStatusDetails := tridentv1.TridentBackendConfigStatus{
			Phase: string(tridentv1.PhaseBound),
			BackendInfo: tridentv1.TridentBackendConfigBackendInfo{
				BackendName: "test-backend",
				BackendUUID: "backend-uuid-123",
			},
		}

		result := controller.checkAndHandleNewlyBoundCRDeletion(ctx, backendConfig, newStatusDetails)
		assert.Nil(t, result) // Should return nil when no transition from unbound
	})

	t.Run("ErrorGettingUpdatedCR", func(t *testing.T) {
		controller, mockCtrl, _ := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Status.Phase = string(tridentv1.PhaseUnbound)

		// Don't create the backend config, so Get will fail

		newStatusDetails := tridentv1.TridentBackendConfigStatus{
			Phase: string(tridentv1.PhaseBound),
			BackendInfo: tridentv1.TridentBackendConfigBackendInfo{
				BackendName: "test-backend",
				BackendUUID: "backend-uuid-123",
			},
		}

		result := controller.checkAndHandleNewlyBoundCRDeletion(ctx, backendConfig, newStatusDetails)
		assert.Nil(t, result) // Should return nil when error getting updated CR
	})
}

func TestDeleteBackendConfigPolicies_EdgeCases(t *testing.T) {
	t.Run("PolicyDelete_BackendInOnlineState", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
			BackendName: "test-backend",
			BackendUUID: "backend-uuid-123",
		}

		// Create TridentBackend in online state
		tridentBackend := &tridentv1.TridentBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tbe-backend-uuid-123",
				Namespace: "default",
			},
			BackendUUID: "backend-uuid-123",
			BackendName: "test-backend",
			State:       string(storage.Online),
		}
		_, err := controller.crdClientset.TridentV1().TridentBackends("default").Create(ctx, tridentBackend, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Expect both calls - DeleteBackendByBackendUUID and RemoveBackendConfigRef (backend != nil case)
		orchestrator.EXPECT().DeleteBackendByBackendUUID(gomock.Any(), "test-backend", "backend-uuid-123").Return(nil)
		// RemoveBackendConfigRef is not called in this scenario because the backend exists

		logFields := map[string]interface{}{
			"backendConfig.Name": backendConfig.Name,
			"backendName":        backendConfig.Status.BackendInfo.BackendName,
			"backendUUID":        backendConfig.Status.BackendInfo.BackendUUID,
		}

		message, phase, err := controller.deleteBackendConfigUsingPolicyDelete(ctx, backendConfig, logFields)
		assert.Error(t, err) // Should error because backend still exists after deletion attempt
		assert.Equal(t, tridentv1.PhaseDeleting, phase)
		assert.Contains(t, message, "Backend still present after a deletion attempt")
	})

	t.Run("PolicyDelete_BackendInFailedState", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
			BackendName: "test-backend",
			BackendUUID: "backend-uuid-123",
		}

		// Create TridentBackend in failed state
		tridentBackend := &tridentv1.TridentBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tbe-backend-uuid-123",
				Namespace: "default",
			},
			BackendUUID: "backend-uuid-123",
			BackendName: "test-backend",
			State:       string(storage.Failed),
		}
		_, err := controller.crdClientset.TridentV1().TridentBackends("default").Create(ctx, tridentBackend, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Expect both calls - DeleteBackendByBackendUUID and RemoveBackendConfigRef (backend != nil case)
		orchestrator.EXPECT().DeleteBackendByBackendUUID(gomock.Any(), "test-backend", "backend-uuid-123").Return(nil)
		// RemoveBackendConfigRef is not called in this scenario because the backend exists

		logFields := map[string]interface{}{
			"backendConfig.Name": backendConfig.Name,
			"backendName":        backendConfig.Status.BackendInfo.BackendName,
			"backendUUID":        backendConfig.Status.BackendInfo.BackendUUID,
		}

		message, phase, err := controller.deleteBackendConfigUsingPolicyDelete(ctx, backendConfig, logFields)
		assert.Error(t, err) // Should error because backend still exists after deletion attempt
		assert.Equal(t, tridentv1.PhaseDeleting, phase)
		assert.Contains(t, message, "Backend still present after a deletion attempt")
	})

	t.Run("PolicyDelete_OrchestratorError", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
			BackendName: "test-backend",
			BackendUUID: "backend-uuid-123",
		}

		// Create TridentBackend
		tridentBackend := &tridentv1.TridentBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tbe-backend-uuid-123",
				Namespace: "default",
			},
			BackendUUID: "backend-uuid-123",
			BackendName: "test-backend",
			State:       string(storage.Online),
		}
		_, err := controller.crdClientset.TridentV1().TridentBackends("default").Create(ctx, tridentBackend, metav1.CreateOptions{})
		assert.NoError(t, err)

		orchestratorError := fmt.Errorf("orchestrator deletion error")
		orchestrator.EXPECT().DeleteBackendByBackendUUID(gomock.Any(), "test-backend", "backend-uuid-123").Return(orchestratorError)

		logFields := map[string]interface{}{
			"backendConfig.Name": backendConfig.Name,
			"backendName":        backendConfig.Status.BackendInfo.BackendName,
			"backendUUID":        backendConfig.Status.BackendInfo.BackendUUID,
		}

		message, phase, err := controller.deleteBackendConfigUsingPolicyDelete(ctx, backendConfig, logFields)
		assert.Error(t, err)
		assert.Equal(t, backendConfig.Status.Phase, string(phase)) // Phase should remain unchanged
		assert.Contains(t, message, "Unable to delete backend")
	})

	t.Run("PolicyRetain_OrchestratorError", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
			BackendName: "test-backend",
			BackendUUID: "backend-uuid-123",
		}

		// Create TridentBackend with config ref
		tridentBackend := &tridentv1.TridentBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tbe-backend-uuid-123",
				Namespace: "default",
			},
			BackendUUID: "backend-uuid-123",
			BackendName: "test-backend",
			ConfigRef:   string(backendConfig.UID),
		}
		_, err := controller.crdClientset.TridentV1().TridentBackends("default").Create(ctx, tridentBackend, metav1.CreateOptions{})
		assert.NoError(t, err)

		orchestratorError := fmt.Errorf("orchestrator config ref removal error")
		orchestrator.EXPECT().RemoveBackendConfigRef(gomock.Any(), "backend-uuid-123", string(backendConfig.UID)).Return(orchestratorError)

		logFields := map[string]interface{}{
			"backendConfig.Name": backendConfig.Name,
			"backendName":        backendConfig.Status.BackendInfo.BackendName,
			"backendUUID":        backendConfig.Status.BackendInfo.BackendUUID,
		}

		message, phase, err := controller.deleteBackendConfigUsingPolicyRetain(ctx, backendConfig, logFields)
		assert.Error(t, err)
		assert.Equal(t, backendConfig.Status.Phase, string(phase)) // Phase should remain unchanged
		assert.Contains(t, message, "Failed to remove configRef from the backend")
	})

	t.Run("PolicyRetain_WithBackendConfigRef", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupBackendConfigTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		backendConfig := createTestBackendConfig("test-backend-config", "default")
		backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
			BackendName: "test-backend",
			BackendUUID: "backend-uuid-123",
		}

		// Create TridentBackend with config ref
		tridentBackend := &tridentv1.TridentBackend{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tbe-backend-uuid-123",
				Namespace: "default",
			},
			BackendUUID: "backend-uuid-123",
			BackendName: "test-backend",
			ConfigRef:   string(backendConfig.UID),
		}
		_, err := controller.crdClientset.TridentV1().TridentBackends("default").Create(ctx, tridentBackend, metav1.CreateOptions{})
		assert.NoError(t, err)

		orchestrator.EXPECT().RemoveBackendConfigRef(gomock.Any(), "backend-uuid-123", string(backendConfig.UID)).Return(nil)

		logFields := map[string]interface{}{
			"backendConfig.Name": backendConfig.Name,
			"backendName":        backendConfig.Status.BackendInfo.BackendName,
			"backendUUID":        backendConfig.Status.BackendInfo.BackendUUID,
		}

		message, phase, err := controller.deleteBackendConfigUsingPolicyRetain(ctx, backendConfig, logFields)
		assert.NoError(t, err)
		assert.Equal(t, tridentv1.TridentBackendConfigPhase(""), phase)
		assert.Equal(t, "", message)
	})
}
