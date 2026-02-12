// Copyright 2026 NetApp, Inc. All Rights Reserved.

package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/frontend/csi/controller_helpers/kubernetes"
	mockAssorterTypes "github.com/netapp/trident/mocks/mock_frontend/mock_autogrow/mock_scheduler/mock_assorter"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

// ============================================================================
// Test: syncCacheWithAPIServer
// ============================================================================

func TestSyncCacheWithAPIServer(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) (*Scheduler, int)
		verify func(*testing.T, *Scheduler, error, int)
	}{
		{
			name: "Success_WithLister_ProcessesTVPs",
			setup: func(ctrl *gomock.Controller) (*Scheduler, int) {
				namespace := "trident"

				tvp1 := createTestTVP("tvp1", namespace, "vol1", "")
				tvp1.AutogrowPolicy = "tvp-policy"

				scLister, tvpLister, _ := setupTestListers(t, nil, []*v1.TridentVolumePublication{tvp1})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, 1
			},
			verify: func(t *testing.T, s *Scheduler, err error, expectedCount int) {
				require.NoError(t, err, "syncCacheWithAPIServer should succeed")

				// Verify cache has the policy
				policy, getErr := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, getErr, "tvp1 should have effective policy in cache")
				assert.Equal(t, "tvp-policy", policy)
			},
		},
		{
			name: "Success_WithLister_NoTVPs_ReturnsSuccess",
			setup: func(ctrl *gomock.Controller) (*Scheduler, int) {
				namespace := "trident"

				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// No assorter expectations - no TVPs

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, 0
			},
			verify: func(t *testing.T, s *Scheduler, err error, expectedCount int) {
				require.NoError(t, err, "syncCacheWithAPIServer should succeed with no TVPs")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			scheduler, expectedCount := tt.setup(ctrl)
			ctx := getTestContext()

			err := scheduler.syncCacheWithAPIServer(ctx)

			tt.verify(t, scheduler, err, expectedCount)
		})
	}
}

// ============================================================================
// Test: processTVPsForSync
// ============================================================================

func TestProcessTVPsForSync(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string)
		verify func(*testing.T, *Scheduler, error, []string)
	}{
		{
			name: "Success_EmptyTVPList_ReturnsSuccessImmediately",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"

				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// No expectations - no TVPs to process

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, nil, nil
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.NoError(t, err, "processTVPsForSync should succeed with empty list")
			},
		},
		{
			name: "PartialSuccess_SingleTVPWithTVPPolicy_AddsToCache_SCLookupFails",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"

				tvp1 := createTestTVP("tvp1", namespace, "vol1", "test-sc")
				tvp1.AutogrowPolicy = "tvp-policy"

				scLister, tvpLister, _ := setupTestListers(t, nil, []*v1.TridentVolumePublication{tvp1})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect AddVolume for tvp1
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, []*v1.TridentVolumePublication{tvp1}, []string{"tvp1"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "processTVPsForSync should return error when SC lookup fails")

				// Verify cache has the policy (TVP policy takes precedence, so sync succeeds for the volume)
				policy, getErr := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, getErr, "tvp1 should have effective policy in cache")
				assert.Equal(t, "tvp-policy", policy, "tvp1 should use TVP policy")
			},
		},
		{
			name: "PartialSuccess_MultipleTVPsWithDifferentPolicies_AllAddedToCache_SomeErrors",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sc",
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: "sc-policy",
						},
					},
				}

				// TVP with TVP policy
				tvp1 := createTestTVP("tvp1", namespace, "vol1", "test-sc")
				tvp1.AutogrowPolicy = "tvp-policy"

				// TVP with SC policy
				tvp2 := createTestTVP("tvp2", namespace, "vol2", "test-sc")
				tvp2.AutogrowPolicy = ""

				// TVP with no policy
				tvp3 := createTestTVP("tvp3", namespace, "vol3", "")
				tvp3.AutogrowPolicy = ""

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1, tvp2, tvp3})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect AddVolume for tvp1 and tvp2
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp2").
					Return(nil).
					Times(1)

				// Expect RemoveVolume for tvp3 (no policy) - this will cause cache delete error
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), "tvp3").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, []*v1.TridentVolumePublication{tvp1, tvp2, tvp3}, []string{"tvp1", "tvp2", "tvp3"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "processTVPsForSync should return error when cache delete fails")

				// Verify tvp1 has TVP policy
				policy1, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, err1, "tvp1 should have effective policy in cache")
				assert.Equal(t, "tvp-policy", policy1, "tvp1 should use TVP policy")

				// Verify tvp2 has SC policy
				policy2, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.NoError(t, err2, "tvp2 should have effective policy in cache")
				assert.Equal(t, "sc-policy", policy2, "tvp2 should use SC policy")

				// Verify tvp3 is not in cache (no policy)
				_, err3 := s.autogrowCache.GetEffectivePolicyName("tvp3")
				assert.Error(t, err3, "tvp3 should not have effective policy in cache")
			},
		},
		{
			name: "Success_TVPWithSCAnnotation_UsesSCPolicy",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sc",
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: "sc-policy",
						},
					},
				}

				tvp1 := createTestTVP("tvp1", namespace, "vol1", "test-sc")
				tvp1.AutogrowPolicy = "" // No TVP policy, will use SC

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, []*v1.TridentVolumePublication{tvp1}, []string{"tvp1"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.NoError(t, err, "processTVPsForSync should succeed")

				policy, getErr := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, getErr, "tvp1 should have effective policy in cache")
				assert.Equal(t, "sc-policy", policy, "tvp1 should use SC policy")
			},
		},
		{
			name: "Success_TVPPolicyTakesPrecedenceOverSCPolicy",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sc",
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: "sc-policy",
						},
					},
				}

				tvp1 := createTestTVP("tvp1", namespace, "vol1", "test-sc")
				tvp1.AutogrowPolicy = "tvp-policy" // TVP policy takes precedence

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, []*v1.TridentVolumePublication{tvp1}, []string{"tvp1"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.NoError(t, err, "processTVPsForSync should succeed")

				policy, getErr := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, getErr, "tvp1 should have effective policy in cache")
				assert.Equal(t, "tvp-policy", policy, "tvp1 should use TVP policy (precedence)")
			},
		},
		{
			name: "PartialSuccess_TVPWithNoStorageClass_OnlyTVPPolicyConsidered_CacheDeleteFails",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"

				tvp1 := createTestTVP("tvp1", namespace, "vol1", "")
				tvp1.AutogrowPolicy = "tvp-policy"

				tvp2 := createTestTVP("tvp2", namespace, "vol2", "")
				tvp2.AutogrowPolicy = "" // No policy at all

				scLister, tvpLister, _ := setupTestListers(t, nil, []*v1.TridentVolumePublication{tvp1, tvp2})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), "tvp2").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, []*v1.TridentVolumePublication{tvp1, tvp2}, []string{"tvp1", "tvp2"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "processTVPsForSync should return error when cache delete fails")

				// Verify tvp1 has policy
				policy1, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, err1, "tvp1 should have effective policy in cache")
				assert.Equal(t, "tvp-policy", policy1, "tvp1 should use TVP policy")

				// Verify tvp2 is not in cache
				_, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.Error(t, err2, "tvp2 should not have effective policy in cache")
			},
		},
		{
			name: "PartialSuccess_SCNotFound_OnlyTVPPolicyConsidered_ReturnsErrors",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"

				// SC doesn't exist in lister
				tvp1 := createTestTVP("tvp1", namespace, "vol1", "missing-sc")
				tvp1.AutogrowPolicy = "tvp-policy"

				tvp2 := createTestTVP("tvp2", namespace, "vol2", "missing-sc")
				tvp2.AutogrowPolicy = ""

				scLister, tvpLister, _ := setupTestListers(t, nil, []*v1.TridentVolumePublication{tvp1, tvp2})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), "tvp2").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, []*v1.TridentVolumePublication{tvp1, tvp2}, []string{"tvp1", "tvp2"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "processTVPsForSync should return errors for SC lookup failures and cache delete failures")

				// Verify tvp1 has TVP policy
				policy1, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, err1, "tvp1 should have effective policy in cache")
				assert.Equal(t, "tvp-policy", policy1, "tvp1 should use TVP policy")

				// Verify tvp2 is not in cache
				_, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.Error(t, err2, "tvp2 should not have effective policy (SC not found, no TVP policy)")
			},
		},
		{
			name: "PartialSuccess_SCBeingDeleted_IgnoresSCAnnotation_CacheDeleteFails",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"
				now := metav1.Now()

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "deleting-sc",
						DeletionTimestamp: &now,
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: "sc-policy", // Should be ignored
						},
					},
				}

				tvp1 := createTestTVP("tvp1", namespace, "vol1", "deleting-sc")
				tvp1.AutogrowPolicy = "tvp-policy"

				tvp2 := createTestTVP("tvp2", namespace, "vol2", "deleting-sc")
				tvp2.AutogrowPolicy = ""

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1, tvp2})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), "tvp2").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, []*v1.TridentVolumePublication{tvp1, tvp2}, []string{"tvp1", "tvp2"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "processTVPsForSync should return error when cache delete fails")

				// Verify tvp1 has TVP policy
				policy1, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, err1, "tvp1 should have effective policy in cache")
				assert.Equal(t, "tvp-policy", policy1, "tvp1 should use TVP policy")

				// Verify tvp2 is not in cache (SC being deleted)
				_, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.Error(t, err2, "tvp2 should not have effective policy (SC being deleted)")
			},
		},
		{
			name: "PartialSuccess_SCHasNoAnnotation_OnlyTVPPolicyConsidered_CacheDeleteFails",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:        "no-annotation-sc",
						Annotations: nil, // No annotations
					},
				}

				tvp1 := createTestTVP("tvp1", namespace, "vol1", "no-annotation-sc")
				tvp1.AutogrowPolicy = "tvp-policy"

				tvp2 := createTestTVP("tvp2", namespace, "vol2", "no-annotation-sc")
				tvp2.AutogrowPolicy = ""

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1, tvp2})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), "tvp2").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, []*v1.TridentVolumePublication{tvp1, tvp2}, []string{"tvp1", "tvp2"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "processTVPsForSync should return error when cache delete fails")

				// Verify tvp1 has TVP policy
				policy1, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, err1, "tvp1 should have effective policy in cache")
				assert.Equal(t, "tvp-policy", policy1, "tvp1 should use TVP policy")

				// Verify tvp2 is not in cache
				_, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.Error(t, err2, "tvp2 should not have effective policy (no SC annotation, no TVP policy)")
			},
		},
		{
			name: "PartialSuccess_TVPBeingDeleted_RemovedFromCache_SCLookupFails",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"
				now := metav1.Now()

				tvp1 := createTestTVP("tvp1", namespace, "vol1", "test-sc")
				tvp1.AutogrowPolicy = "tvp-policy"
				tvp1.ObjectMeta.DeletionTimestamp = &now // Being deleted

				scLister, tvpLister, _ := setupTestListers(t, nil, []*v1.TridentVolumePublication{tvp1})
				autogrowCache := getTestAutogrowCache()

				// Pre-populate cache
				autogrowCache.SetEffectivePolicyName("tvp1", "old-policy")

				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect RemoveVolume (TVP being deleted)
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, []*v1.TridentVolumePublication{tvp1}, []string{"tvp1"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "processTVPsForSync should return error when SC lookup fails")

				// Verify tvp1 is removed from cache
				_, getErr := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.Error(t, getErr, "tvp1 should not have effective policy (TVP being deleted)")
			},
		},
		{
			name: "PartialSuccess_AssorterFailures_ReturnsErrorButContinuesProcessingOtherTVPs",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"

				tvp1 := createTestTVP("tvp1", namespace, "vol1", "")
				tvp1.AutogrowPolicy = "tvp-policy"

				tvp2 := createTestTVP("tvp2", namespace, "vol2", "")
				tvp2.AutogrowPolicy = "tvp-policy"

				tvp3 := createTestTVP("tvp3", namespace, "vol3", "")
				tvp3.AutogrowPolicy = "tvp-policy"

				scLister, tvpLister, _ := setupTestListers(t, nil, []*v1.TridentVolumePublication{tvp1, tvp2, tvp3})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// First assorter call fails
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(assert.AnError).
					Times(1)

				// Second assorter call succeeds
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp2").
					Return(nil).
					Times(1)

				// Third assorter call succeeds
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp3").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, []*v1.TridentVolumePublication{tvp1, tvp2, tvp3}, []string{"tvp1", "tvp2", "tvp3"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "processTVPsForSync should return error when assorter fails")

				// All should be in cache (cache update is independent of assorter)
				policy1, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, err1, "tvp1 should be in cache")
				assert.Equal(t, "tvp-policy", policy1)

				policy2, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.NoError(t, err2, "tvp2 should be in cache")
				assert.Equal(t, "tvp-policy", policy2)

				policy3, err3 := s.autogrowCache.GetEffectivePolicyName("tvp3")
				assert.NoError(t, err3, "tvp3 should be in cache")
				assert.Equal(t, "tvp-policy", policy3)
			},
		},
		{
			name: "PartialSuccess_MixedScenarios_HandlesAllCorrectly_SomeCacheDeletesFail",
			setup: func(ctrl *gomock.Controller) (*Scheduler, []*v1.TridentVolumePublication, []string) {
				namespace := "trident"
				now := metav1.Now()

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sc",
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: "sc-policy",
						},
					},
				}

				// TVP with TVP policy + SC
				tvp1 := createTestTVP("tvp1", namespace, "vol1", "test-sc")
				tvp1.AutogrowPolicy = "tvp-policy"

				// TVP with SC policy only
				tvp2 := createTestTVP("tvp2", namespace, "vol2", "test-sc")
				tvp2.AutogrowPolicy = ""

				// TVP being deleted
				tvp3 := createTestTVP("tvp3", namespace, "vol3", "test-sc")
				tvp3.AutogrowPolicy = "tvp-policy"
				tvp3.ObjectMeta.DeletionTimestamp = &now

				// TVP with no policy at all
				tvp4 := createTestTVP("tvp4", namespace, "vol4", "")
				tvp4.AutogrowPolicy = ""

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1, tvp2, tvp3, tvp4})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect AddVolume for tvp1 and tvp2
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp2").
					Return(nil).
					Times(1)

				// Expect RemoveVolume for tvp3 (being deleted) and tvp4 (no policy)
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), "tvp3").
					Return(nil).
					Times(1)

				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), "tvp4").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				return scheduler, []*v1.TridentVolumePublication{tvp1, tvp2, tvp3, tvp4}, []string{"tvp1", "tvp2", "tvp3", "tvp4"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "processTVPsForSync should return error when cache deletes fail")

				// Verify tvp1 has TVP policy
				policy1, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, err1, "tvp1 should have effective policy in cache")
				assert.Equal(t, "tvp-policy", policy1, "tvp1 should use TVP policy")

				// Verify tvp2 has SC policy
				policy2, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.NoError(t, err2, "tvp2 should have effective policy in cache")
				assert.Equal(t, "sc-policy", policy2, "tvp2 should use SC policy")

				// Verify tvp3 is not in cache (being deleted)
				_, err3 := s.autogrowCache.GetEffectivePolicyName("tvp3")
				assert.Error(t, err3, "tvp3 should not have effective policy (being deleted)")

				// Verify tvp4 is not in cache (no policy)
				_, err4 := s.autogrowCache.GetEffectivePolicyName("tvp4")
				assert.Error(t, err4, "tvp4 should not have effective policy (no policy)")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gomock controller for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup test dependencies
			scheduler, tvps, tvpNames := tt.setup(ctrl)

			// Get context
			ctx := getTestContext()

			// Call processTVPsForSync
			err := scheduler.processTVPsForSync(ctx, tvps)

			// Verify results (pass scheduler and tvpNames for cache assertions)
			tt.verify(t, scheduler, err, tvpNames)
		})
	}
}
