package cmd

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mockK8sClient "github.com/netapp/trident/mocks/mock_cli/mock_k8s_client"
)

var (
	testCtx               = context.Background()
	obliviateCRDTestMutex sync.RWMutex
)

// Constants for test timeout values
const (
	shortTimeout  = 50 * time.Millisecond
	normalTimeout = 2 * time.Second
)

// Test helper functions
func withObliviateCRDTestMode(t *testing.T, fn func()) {
	t.Helper()

	obliviateCRDTestMutex.Lock()
	defer obliviateCRDTestMutex.Unlock()

	// Save original state
	originalK8sClient := k8sClient
	originalK8sTimeout := k8sTimeout
	originalOperatingMode := OperatingMode
	originalForceObliviate := forceObliviate
	originalSkipCRDs := skipCRDs

	defer func() {
		// Restore original state
		k8sClient = originalK8sClient
		k8sTimeout = originalK8sTimeout
		OperatingMode = originalOperatingMode
		forceObliviate = originalForceObliviate
		skipCRDs = originalSkipCRDs
	}()

	fn()
}

// Mock setup helpers
func setupCRDWithFinalizers(mockK8s *mockK8sClient.MockKubernetesClient, crdName string) {
	crd := createTestCRD(crdName, true, false)
	mockK8s.EXPECT().CheckCRDExists(crdName).Return(true, nil)
	mockK8s.EXPECT().GetCRD(crdName).Return(crd, nil)
	mockK8s.EXPECT().RemoveFinalizerFromCRD(crdName).Return(nil)
	mockK8s.EXPECT().DeleteCRD(crdName).Return(nil)
	mockK8s.EXPECT().CheckCRDExists(crdName).Return(false, nil)
}

func setupCRDWithoutFinalizers(mockK8s *mockK8sClient.MockKubernetesClient, crdName string) {
	crd := createTestCRD(crdName, false, false)
	mockK8s.EXPECT().CheckCRDExists(crdName).Return(true, nil)
	mockK8s.EXPECT().GetCRD(crdName).Return(crd, nil)
	mockK8s.EXPECT().DeleteCRD(crdName).Return(nil)
	mockK8s.EXPECT().CheckCRDExists(crdName).Return(false, nil)
}

func setupCRDNotExists(mockK8s *mockK8sClient.MockKubernetesClient, crdName string) {
	mockK8s.EXPECT().CheckCRDExists(crdName).Return(false, nil)
}

// getAllTridentCRDs returns the complete list of Trident CRDs from the implementation
func getAllTridentCRDs() []string {
	return []string{
		"tridentversions.trident.netapp.io",
		"tridentbackendconfigs.trident.netapp.io",
		"tridentbackends.trident.netapp.io",
		"tridentstorageclasses.trident.netapp.io",
		"tridentmirrorrelationships.trident.netapp.io",
		"tridentactionmirrorupdates.trident.netapp.io",
		"tridentsnapshotinfos.trident.netapp.io",
		"tridentvolumes.trident.netapp.io",
		"tridentnodes.trident.netapp.io",
		"tridenttransactions.trident.netapp.io",
		"tridentsnapshots.trident.netapp.io",
		"tridentgroupsnapshots.trident.netapp.io",
		"tridentvolumepublications.trident.netapp.io",
		"tridentvolumereferences.trident.netapp.io",
		"tridentactionsnapshotrestores.trident.netapp.io",
		"tridentconfigurators.trident.netapp.io",
		"tridentorchestrators.trident.netapp.io",
	}
}

// filterCRDs returns CRDs from getAllTridentCRDs that are not in the skip list
func filterCRDs(skipCRDs []string) []string {
	allTridentCRDs := getAllTridentCRDs()
	skipMap := make(map[string]bool)
	for _, skip := range skipCRDs {
		skipMap[skip] = true
	}

	var filtered []string
	for _, crd := range allTridentCRDs {
		if !skipMap[crd] {
			filtered = append(filtered, crd)
		}
	}
	return filtered
}

func createTestCRD(name string, withFinalizers bool,
	withDeletionTimestamp bool,
) *apiextensionsv1.CustomResourceDefinition {
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	if withFinalizers {
		crd.Finalizers = []string{"test-finalizer"}
	}
	if withDeletionTimestamp {
		now := metav1.NewTime(time.Now())
		crd.DeletionTimestamp = &now
	}
	return crd
}

func TestObliviateCRDs(t *testing.T) {
	tests := []struct {
		name     string
		skipCRDs []string
		err      bool
		mockFunc func(*mockK8sClient.MockKubernetesClient)
	}{
		{
			name:     "SuccessNoSkipCRDs",
			skipCRDs: []string{},
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				allTridentCRDs := getAllTridentCRDs()
				for _, crdName := range allTridentCRDs {
					mockK8s.EXPECT().CheckCRDExists(crdName).Return(false, nil).Times(2)
				}
			},
		},
		{
			name:     "SuccessWithSkipCRDs",
			skipCRDs: []string{"tridentversions.trident.netapp.io", "tridentbackends.trident.netapp.io"},
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				nonSkippedCRDs := filterCRDs([]string{"tridentversions.trident.netapp.io", "tridentbackends.trident.netapp.io"})

				for _, crdName := range nonSkippedCRDs {
					mockK8s.EXPECT().CheckCRDExists(crdName).Return(false, nil).Times(2)
				}
			},
		},
		{
			name:     "ErrorInDeleteCRs",
			skipCRDs: []string{},
			err:      true,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().CheckCRDExists("tridentversions.trident.netapp.io").Return(false, fmt.Errorf("api error"))
			},
		},
		{
			name:     "ErrorInDeleteCRDs",
			skipCRDs: []string{},
			err:      true,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				// deleteCRs succeeds (all CRDs don't exist)
				allTridentCRDs := getAllTridentCRDs()
				for _, crdName := range allTridentCRDs {
					mockK8s.EXPECT().CheckCRDExists(crdName).Return(false, nil).Times(1)
				}

				// deleteCRDs fails on first CRD
				mockK8s.EXPECT().CheckCRDExists("tridentversions.trident.netapp.io").Return(false, fmt.Errorf("delete crd error"))
			},
		},
		{
			name:     "SkipAllCRDs",
			skipCRDs: getAllTridentCRDs(),
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withObliviateCRDTestMode(t, func() {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				mockK8s := mockK8sClient.NewMockKubernetesClient(ctrl)
				k8sClient = mockK8s

				tt.mockFunc(mockK8s)

				err := obliviateCRDs(tt.skipCRDs)

				if tt.err {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestDeleteCRDs(t *testing.T) {
	tests := []struct {
		name     string
		crdNames []string
		err      bool
		mockFunc func(*mockK8sClient.MockKubernetesClient)
	}{
		{
			name:     "SuccessNoCRDsExist",
			crdNames: []string{"test1.example.com", "test2.example.com"},
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().CheckCRDExists("test1.example.com").Return(false, nil)
				mockK8s.EXPECT().CheckCRDExists("test2.example.com").Return(false, nil)
			},
		},
		{
			name:     "SuccessCRDWithFinalizers",
			crdNames: []string{"test.example.com"},
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				crd := createTestCRD("test.example.com", true, false)

				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(true, nil)
				mockK8s.EXPECT().GetCRD("test.example.com").Return(crd, nil)
				mockK8s.EXPECT().RemoveFinalizerFromCRD("test.example.com").Return(nil)
				mockK8s.EXPECT().DeleteCRD("test.example.com").Return(nil)
				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(false, nil)
			},
		},
		{
			name:     "SuccessCRDWithoutFinalizers",
			crdNames: []string{"test.example.com"},
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				crd := createTestCRD("test.example.com", false, false)

				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(true, nil)
				mockK8s.EXPECT().GetCRD("test.example.com").Return(crd, nil)
				mockK8s.EXPECT().DeleteCRD("test.example.com").Return(nil)
				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(false, nil)
			},
		},
		{
			name:     "SuccessCRDWithDeletionTimestamp",
			crdNames: []string{"test.example.com"},
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				crd := createTestCRD("test.example.com", false, true)

				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(true, nil)
				mockK8s.EXPECT().GetCRD("test.example.com").Return(crd, nil)
				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(false, nil)
			},
		},
		{
			name:     "CRDNotFoundDuringGet",
			crdNames: []string{"test.example.com"},
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				notFoundErr := &apierrors.StatusError{
					ErrStatus: metav1.Status{
						Reason: metav1.StatusReasonNotFound,
					},
				}

				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(true, nil)
				mockK8s.EXPECT().GetCRD("test.example.com").Return(nil, notFoundErr)
			},
		},
		{
			name:     "ErrorCheckingCRDExists",
			crdNames: []string{"test.example.com"},
			err:      true,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(false, fmt.Errorf("api error"))
			},
		},
		{
			name:     "ErrorRemovingFinalizers",
			crdNames: []string{"test.example.com"},
			err:      true,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				crd := createTestCRD("test.example.com", true, false)

				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(true, nil)
				mockK8s.EXPECT().GetCRD("test.example.com").Return(crd, nil)
				mockK8s.EXPECT().RemoveFinalizerFromCRD("test.example.com").Return(fmt.Errorf("finalizer error"))
			},
		},
		{
			name:     "ErrorDeletingCRD",
			crdNames: []string{"test.example.com"},
			err:      true,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				crd := createTestCRD("test.example.com", false, false)

				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(true, nil)
				mockK8s.EXPECT().GetCRD("test.example.com").Return(crd, nil)
				mockK8s.EXPECT().DeleteCRD("test.example.com").Return(fmt.Errorf("delete error"))
			},
		},
		{
			name:     "CRDNotFoundDuringDeletion",
			crdNames: []string{"test.example.com"},
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				crd := createTestCRD("test.example.com", false, false)
				notFoundErr := &apierrors.StatusError{
					ErrStatus: metav1.Status{
						Reason: metav1.StatusReasonNotFound,
					},
				}

				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(true, nil)
				mockK8s.EXPECT().GetCRD("test.example.com").Return(crd, nil)
				mockK8s.EXPECT().DeleteCRD("test.example.com").Return(notFoundErr)
			},
		},
		{
			name:     "WaitForDeletionTimeout",
			crdNames: []string{"test.example.com"},
			err:      true,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				crd := createTestCRD("test.example.com", false, false)

				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(true, nil)
				mockK8s.EXPECT().GetCRD("test.example.com").Return(crd, nil)
				mockK8s.EXPECT().DeleteCRD("test.example.com").Return(nil)
				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(true, nil).AnyTimes()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withObliviateCRDTestMode(t, func() {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				mockK8s := mockK8sClient.NewMockKubernetesClient(ctrl)
				k8sClient = mockK8s
				k8sTimeout = shortTimeout

				tt.mockFunc(mockK8s)

				err := deleteCRDs(tt.crdNames)

				if tt.err {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestWaitForCRDDeletion(t *testing.T) {
	tests := []struct {
		name     string
		crdName  string
		timeout  time.Duration
		err      bool
		mockFunc func(*mockK8sClient.MockKubernetesClient)
	}{
		{
			name:    "SuccessCRDDeletedImmediately",
			crdName: "test.example.com",
			timeout: 1 * time.Second,
			err:     false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(false, nil)
			},
		},
		{
			name:    "SuccessCRDNotFoundError",
			crdName: "test.example.com",
			timeout: 1 * time.Second,
			err:     false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				notFoundErr := &apierrors.StatusError{
					ErrStatus: metav1.Status{
						Reason: metav1.StatusReasonNotFound,
					},
				}
				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(false, notFoundErr)
			},
		},
		{
			name:    "SuccessAfterRetry",
			crdName: "test.example.com",
			timeout: 2 * time.Second,
			err:     false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				// Use AnyTimes to handle the unpredictable nature of exponential backoff
				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(true, nil).Times(1)
				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(false, nil).MinTimes(1)
			},
		},
		{
			name:    "TimeoutCRDStillExists",
			crdName: "test.example.com",
			timeout: 50 * time.Millisecond, // Very short timeout
			err:     true,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				// Use AnyTimes to handle multiple retry calls
				mockK8s.EXPECT().CheckCRDExists("test.example.com").Return(true, nil).AnyTimes()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withObliviateCRDTestMode(t, func() {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				mockK8s := mockK8sClient.NewMockKubernetesClient(ctrl)
				k8sClient = mockK8s

				tt.mockFunc(mockK8s)

				err := waitForCRDDeletion(tt.crdName, tt.timeout)

				if tt.err {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "NilError",
			err:      nil,
			expected: false,
		},
		{
			name:     "NotStatusError",
			err:      fmt.Errorf("regular error"),
			expected: false,
		},
		{
			name: "StatusErrorNotFound",
			err: &apierrors.StatusError{
				ErrStatus: metav1.Status{
					Reason: metav1.StatusReasonNotFound,
				},
			},
			expected: true,
		},
		{
			name: "StatusErrorOtherReason",
			err: &apierrors.StatusError{
				ErrStatus: metav1.Status{
					Reason: metav1.StatusReasonForbidden,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNotFoundError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeleteWithRetry(t *testing.T) {
	tests := []struct {
		name     string
		err      bool
		mockFunc func() crDeleter
	}{
		{
			name: "SuccessImmediate",
			err:  false,
			mockFunc: func() crDeleter {
				return func(ctx context.Context, name string, opts metav1.DeleteOptions) error {
					return nil
				}
			},
		},
		{
			name: "SuccessNotFoundError",
			err:  false,
			mockFunc: func() crDeleter {
				return func(ctx context.Context, name string, opts metav1.DeleteOptions) error {
					return &apierrors.StatusError{
						ErrStatus: metav1.Status{
							Reason: metav1.StatusReasonNotFound,
						},
					}
				}
			},
		},
		{
			name: "SuccessAfterRetry",
			err:  false,
			mockFunc: func() crDeleter {
				callCount := 0
				return func(ctx context.Context, name string, opts metav1.DeleteOptions) error {
					callCount++
					if callCount == 1 {
						return fmt.Errorf("temporary error")
					}
					return nil
				}
			},
		},
		{
			name: "TimeoutPersistentError",
			err:  true,
			mockFunc: func() crDeleter {
				return func(ctx context.Context, name string, opts metav1.DeleteOptions) error {
					return fmt.Errorf("persistent error")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleteFunc := tt.mockFunc()

			err := deleteWithRetry(deleteFunc, testCtx, "test-resource", nil)

			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestObliviateCRDCommand(t *testing.T) {
	tests := []struct {
		name           string
		operatingMode  string
		forceObliviate bool
		skipCRDs       []string
		err            bool
	}{
		{
			name:           "TunnelModeForceTrue",
			operatingMode:  ModeTunnel,
			forceObliviate: true,
			skipCRDs:       []string{},
			err:            true,
		},
		{
			name:           "TunnelModeWithSkipCRDs",
			operatingMode:  ModeTunnel,
			forceObliviate: true,
			skipCRDs:       []string{"tridentversions.trident.netapp.io"},
			err:            true,
		},
		{
			name:          "DirectModeSuccess",
			operatingMode: "direct",
			skipCRDs:      []string{},
			err:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withObliviateCRDTestMode(t, func() {
				OperatingMode = tt.operatingMode
				forceObliviate = tt.forceObliviate
				skipCRDs = tt.skipCRDs

				cmd := &cobra.Command{}

				err := obliviateCRDCmd.RunE(cmd, []string{})
				if tt.err {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestDeleteCRs(t *testing.T) {
	tests := []struct {
		name         string
		filteredCRDs []string
		err          bool
		mockFunc     func(*mockK8sClient.MockKubernetesClient)
	}{
		{
			name:         "SuccessNoCRDs",
			filteredCRDs: []string{},
			err:          false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
			},
		},
		{
			name:         "SuccessUnknownCRD",
			filteredCRDs: []string{"unknown.example.com"},
			err:          false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
			},
		},
		{
			name:         "SuccessTridentVersionsNotPresent",
			filteredCRDs: []string{"tridentversions.trident.netapp.io"},
			err:          false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().CheckCRDExists("tridentversions.trident.netapp.io").Return(false, nil)
			},
		},
		{
			name:         "ErrorInDeleteVersions",
			filteredCRDs: []string{"tridentversions.trident.netapp.io"},
			err:          true,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().CheckCRDExists("tridentversions.trident.netapp.io").Return(false, fmt.Errorf("check error"))
			},
		},
		{
			name:         "SuccessMultipleCRDs",
			filteredCRDs: []string{"tridentversions.trident.netapp.io", "tridentbackends.trident.netapp.io"},
			err:          false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				mockK8s.EXPECT().CheckCRDExists("tridentversions.trident.netapp.io").Return(false, nil)
				mockK8s.EXPECT().CheckCRDExists("tridentbackends.trident.netapp.io").Return(false, nil)
			},
		},
		{
			name:         "SuccessAllCRDTypes",
			filteredCRDs: getAllTridentCRDs(),
			err:          false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				// All CRDs don't exist
				allTridentCRDs := getAllTridentCRDs()
				for _, crdName := range allTridentCRDs {
					mockK8s.EXPECT().CheckCRDExists(crdName).Return(false, nil)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withObliviateCRDTestMode(t, func() {
				ctrl := gomock.NewController(t)
				mockK8s := mockK8sClient.NewMockKubernetesClient(ctrl)
				k8sClient = mockK8s

				tt.mockFunc(mockK8s)

				err := deleteCRs(tt.filteredCRDs)

				if tt.err {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}

func TestFilterCRDs(t *testing.T) {
	allTridentCRDs := getAllTridentCRDs()
	tests := []struct {
		name     string
		skipCRDs []string
		expected int // expected number of CRDs after filtering
	}{
		{
			name:     "NoSkipCRDs",
			skipCRDs: []string{},
			expected: len(allTridentCRDs),
		},
		{
			name:     "SkipSome",
			skipCRDs: []string{"tridentversions.trident.netapp.io", "tridentbackends.trident.netapp.io"},
			expected: len(allTridentCRDs) - 2,
		},
		{
			name:     "SkipAll",
			skipCRDs: allTridentCRDs,
			expected: 0,
		},
		{
			name:     "SkipNonExistent",
			skipCRDs: []string{"nonexistent.example.com"},
			expected: len(allTridentCRDs),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterCRDs(tt.skipCRDs)
			assert.Len(t, result, tt.expected)

			// Verify that skipped CRDs are not in the result
			skipMap := make(map[string]bool)
			for _, skip := range tt.skipCRDs {
				skipMap[skip] = true
			}

			for _, crd := range result {
				assert.False(t, skipMap[crd], "CRD %s should have been filtered out", crd)
			}
		})
	}
}

func TestObliviateCRDsPublicFunction(t *testing.T) {
	tests := []struct {
		name     string
		skipCRDs []string
		err      bool
		mockFunc func(*mockK8sClient.MockKubernetesClient)
	}{
		{
			name:     "PublicFunctionSuccess",
			skipCRDs: []string{},
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				allTridentCRDs := getAllTridentCRDs()
				for _, crdName := range allTridentCRDs {
					mockK8s.EXPECT().CheckCRDExists(crdName).Return(false, nil).Times(2)
				}
			},
		},
		{
			name:     "PublicFunctionWithTimeout",
			skipCRDs: []string{"tridentversions.trident.netapp.io"},
			err:      false,
			mockFunc: func(mockK8s *mockK8sClient.MockKubernetesClient) {
				nonSkippedCRDs := filterCRDs([]string{"tridentversions.trident.netapp.io"})
				for _, crdName := range nonSkippedCRDs {
					mockK8s.EXPECT().CheckCRDExists(crdName).Return(false, nil).Times(2)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withObliviateCRDTestMode(t, func() {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				mockK8s := mockK8sClient.NewMockKubernetesClient(ctrl)
				tt.mockFunc(mockK8s)

				err := ObliviateCRDs(mockK8s, nil, nil, normalTimeout, tt.skipCRDs)

				if tt.err {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		})
	}
}
