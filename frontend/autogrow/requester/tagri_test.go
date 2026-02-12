// Copyright 2026 NetApp, Inc. All Rights Reserved.

package requester

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	agCache "github.com/netapp/trident/frontend/autogrow/cache"
	autogrowTypes "github.com/netapp/trident/frontend/autogrow/types"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	tridentinformers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	utilsErrors "github.com/netapp/trident/utils/errors"
)

// Test generateTAGRIName
func TestGenerateTAGRIName(t *testing.T) {
	tests := []struct {
		name         string
		pvName       string
		expectedName string
	}{
		{
			name:         "Simple PV name",
			pvName:       "pvc-12345",
			expectedName: "tagri-pvc-12345",
		},
		{
			name:         "PV with dashes",
			pvName:       "pvc-abc-def-123",
			expectedName: "tagri-pvc-abc-def-123",
		},
		{
			name:         "Long PV name",
			pvName:       "pvc-very-long-name-with-many-segments-12345678",
			expectedName: "tagri-pvc-very-long-name-with-many-segments-12345678",
		},
		{
			name:         "Empty PV name",
			pvName:       "",
			expectedName: "tagri-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateTAGRIName(tt.pvName)
			assert.Equal(t, tt.expectedName, result)
		})
	}
}

// Test getTAGRI
func TestGetTAGRI(t *testing.T) {
	tests := []struct {
		name           string
		pvName         string
		expectError    bool
		expectNotFound bool
		createTAGRI    bool // Whether to pre-create TAGRI
	}{
		{
			name:           "TAGRI exists - returns it",
			pvName:         "pv-exists",
			expectError:    false,
			expectNotFound: false,
			createTAGRI:    true,
		},
		{
			name:           "TAGRI does not exist - returns NotFound",
			pvName:         "pv-missing",
			expectError:    true,
			expectNotFound: true,
			createTAGRI:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
			defer close(stopCh)

			agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
			tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

			requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, nil)
			require.NoError(t, err)

			// Create TAGRI if needed
			if tt.createTAGRI {
				tagri := &v1.TridentAutogrowRequestInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      generateTAGRIName(tt.pvName),
						Namespace: requester.config.TridentNamespace,
					},
					Spec: v1.TridentAutogrowRequestInternalSpec{
						Volume: tt.pvName,
					},
					Status: v1.TridentAutogrowRequestInternalStatus{
						Phase: string(v1.TridentAutogrowRequestInternalPending),
					},
				}
				_, err := tridentClient.TridentV1().TridentAutogrowRequestInternals(requester.config.TridentNamespace).Create(
					ctx, tagri, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			// Call getTAGRI with volumeID
			// In tests, pvName represents the volumeID
			volumeID := tt.pvName
			tagri, err := requester.getTAGRI(ctx, volumeID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectNotFound {
					assert.True(t, k8serrors.IsNotFound(err))
				}
				assert.Nil(t, tagri)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tagri)
				assert.Equal(t, tt.pvName, tagri.Spec.Volume)
				assert.Equal(t, generateTAGRIName(tt.pvName), tagri.Name)
			}
		})
	}
}

// Test createTAGRI
func TestCreateTAGRI(t *testing.T) {
	tests := []struct {
		name         string
		pvName       string
		policyName   string
		event        autogrowTypes.VolumeThresholdBreached
		setupFunc    func(*testing.T, *Requester, *fake.Clientset, *agCache.AutogrowCache)
		verifyResult func(*testing.T, *Requester, *fake.Clientset, string)
		expectError  bool
	}{
		{
			name:       "UsedPercentage provided in event",
			pvName:     "pv-with-percentage",
			policyName: "test-policy",
			event: autogrowTypes.VolumeThresholdBreached{
				Ctx:              context.Background(),
				TVPName:          "pv-with-percentage",
				TAGPName:         "test-policy",
				CurrentTotalSize: 1000,
				UsedSize:         800,
				UsedPercentage:   85.5, // Explicit percentage
				Timestamp:        time.Now(),
			},
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// Volume metadata no longer cached - not needed for test
			},
			verifyResult: func(t *testing.T, req *Requester, client *fake.Clientset, pvName string) {
				tagriName := generateTAGRIName(pvName)
				tagri, err := client.TridentV1().TridentAutogrowRequestInternals(req.config.TridentNamespace).
					Get(context.Background(), tagriName, metav1.GetOptions{})
				require.NoError(t, err)
				require.NotNil(t, tagri)

				assert.Equal(t, pvName, tagri.Spec.Volume)
				// Should use provided percentage, not calculate from sizes
				assert.Equal(t, float32(85.5), tagri.Spec.ObservedUsedPercent)
				assert.Equal(t, "1000", tagri.Spec.ObservedCapacityBytes)
			},
			expectError: false,
		},
		{
			name:       "UsedPercentage NOT provided - calculates from sizes",
			pvName:     "pv-calculate-percentage",
			policyName: "test-policy",
			event: autogrowTypes.VolumeThresholdBreached{
				Ctx:              context.Background(),
				TVPName:          "pv-calculate-percentage",
				TAGPName:         "test-policy",
				CurrentTotalSize: 1000,
				UsedSize:         800,
				UsedPercentage:   0, // NOT provided, should calculate: 800/1000*100 = 80%
				Timestamp:        time.Now(),
			},
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// Volume metadata no longer cached - not needed for test
			},
			verifyResult: func(t *testing.T, req *Requester, client *fake.Clientset, pvName string) {
				tagriName := generateTAGRIName(pvName)
				tagri, err := client.TridentV1().TridentAutogrowRequestInternals(req.config.TridentNamespace).
					Get(context.Background(), tagriName, metav1.GetOptions{})
				require.NoError(t, err)
				require.NotNil(t, tagri)

				assert.Equal(t, pvName, tagri.Spec.Volume)
				// Should calculate: (800 / 1000) * 100 = 80.0%
				assert.Equal(t, float32(80.0), tagri.Spec.ObservedUsedPercent)
				assert.Equal(t, "1000", tagri.Spec.ObservedCapacityBytes)
			},
			expectError: false,
		},
		{
			name:       "Zero capacity - usage defaults to 0%",
			pvName:     "pv-zero-capacity",
			policyName: "test-policy",
			event:      createTestEvent("pv-zero-capacity", 0, 0),
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// Volume metadata no longer cached - not needed for test
			},
			verifyResult: func(t *testing.T, req *Requester, client *fake.Clientset, pvName string) {
				tagriName := generateTAGRIName(pvName)
				tagri, err := client.TridentV1().TridentAutogrowRequestInternals(req.config.TridentNamespace).
					Get(context.Background(), tagriName, metav1.GetOptions{})
				require.NoError(t, err)
				require.NotNil(t, tagri)

				assert.Equal(t, pvName, tagri.Spec.Volume)
				assert.Equal(t, float32(0.0), tagri.Spec.ObservedUsedPercent)
				assert.Equal(t, "0", tagri.Spec.ObservedCapacityBytes)
			},
			expectError: false,
		},
		{
			name:       "API rate limit with RetryAfterSeconds",
			pvName:     "pv-rate-limited",
			policyName: "test-policy",
			event:      createTestEvent("pv-rate-limited", 1000, 800),
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// Add reactor to return TooManyRequests with RetryAfterSeconds
				client.PrependReactor("create", "tridentautogrowrequestinternals", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, &k8serrors.StatusError{
						ErrStatus: metav1.Status{
							Status:  metav1.StatusFailure,
							Code:    429,
							Reason:  metav1.StatusReasonTooManyRequests,
							Message: "rate limited",
							Details: &metav1.StatusDetails{
								RetryAfterSeconds: 10,
							},
						},
					}
				})
			},
			verifyResult: func(t *testing.T, req *Requester, client *fake.Clientset, pvName string) {
				// TAGRI should not exist due to rate limit error
				tagriName := generateTAGRIName(pvName)
				_, err := client.TridentV1().TridentAutogrowRequestInternals(req.config.TridentNamespace).
					Get(context.Background(), tagriName, metav1.GetOptions{})
				assert.Error(t, err, "TAGRI should not exist due to rate limit")
				assert.True(t, k8serrors.IsNotFound(err), "Error should be NotFound")
			},
			expectError: true,
		},
		{
			name:       "API rate limit without RetryAfterSeconds",
			pvName:     "pv-rate-limited-no-retry",
			policyName: "test-policy",
			event:      createTestEvent("pv-rate-limited-no-retry", 1000, 800),
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// Add reactor to return TooManyRequests with Details but RetryAfterSeconds = 0
				client.PrependReactor("create", "tridentautogrowrequestinternals", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, &k8serrors.StatusError{
						ErrStatus: metav1.Status{
							Status:  metav1.StatusFailure,
							Code:    429,
							Reason:  metav1.StatusReasonTooManyRequests,
							Message: "rate limited",
							Details: &metav1.StatusDetails{
								RetryAfterSeconds: 0, // Zero, so should skip the AddAfter logic
							},
						},
					}
				})
			},
			verifyResult: func(t *testing.T, req *Requester, client *fake.Clientset, pvName string) {
				// TAGRI should not exist due to rate limit error
				tagriName := generateTAGRIName(pvName)
				_, err := client.TridentV1().TridentAutogrowRequestInternals(req.config.TridentNamespace).
					Get(context.Background(), tagriName, metav1.GetOptions{})
				assert.Error(t, err, "TAGRI should not exist due to rate limit")
				assert.True(t, k8serrors.IsNotFound(err), "Error should be NotFound")
			},
			expectError: true,
		},
		{
			name:       "Generic API error - wrapped as ReconcileDeferredError",
			pvName:     "pv-generic-error",
			policyName: "test-policy",
			event:      createTestEvent("pv-generic-error", 1000, 800),
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// Add reactor to return a generic error (not rate limit)
				client.PrependReactor("create", "tridentautogrowrequestinternals", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, fmt.Errorf("generic API error")
				})
			},
			verifyResult: func(t *testing.T, req *Requester, client *fake.Clientset, pvName string) {
				// TAGRI should not exist due to error
				tagriName := generateTAGRIName(pvName)
				_, err := client.TridentV1().TridentAutogrowRequestInternals(req.config.TridentNamespace).
					Get(context.Background(), tagriName, metav1.GetOptions{})
				assert.Error(t, err, "TAGRI should not exist due to error")
				assert.True(t, k8serrors.IsNotFound(err), "Error should be NotFound")
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Create policy
			policy := createTestPolicy(tt.policyName, string(v1.TridentAutogrowPolicyStateSuccess))
			tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t, policy)
			defer close(stopCh)

			agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
			tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

			requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, nil)
			require.NoError(t, err)

			// Setup test scenario
			tt.setupFunc(t, requester, tridentClient, autogrowCache)

			// Calculate usage percentage (same as done in processWorkItem)
			var usagePercent float32
			if tt.event.CurrentTotalSize > 0 {
				if tt.event.UsedPercentage > 0 {
					usagePercent = tt.event.UsedPercentage
				} else {
					usagePercent = (float32(tt.event.UsedSize) / float32(tt.event.CurrentTotalSize)) * 100.0
				}
			} else {
				usagePercent = 0.0
			}

			// Call createTAGRI with volumeID and calculated usage percent
			// In tests, pvName represents the volumeID
			volumeID := tt.pvName
			err = requester.createTAGRI(ctx, tt.event, policy, volumeID, usagePercent)

			if tt.expectError {
				assert.Error(t, err)
				// Verify error is wrapped as ReconcileDeferredError
				assert.True(t, utilsErrors.IsReconcileDeferredError(err), "error should be wrapped as ReconcileDeferredError")
			} else {
				assert.NoError(t, err)
			}

			// Verify result
			if tt.verifyResult != nil {
				tt.verifyResult(t, requester, tridentClient, tt.pvName)
			}
		})
	}
}
