// Copyright 2026 NetApp, Inc. All Rights Reserved.

package scheduler

import (
	"context"
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
// Test: reconcileVolumeEffectivePolicy
// ============================================================================

func TestReconcileVolumeEffectivePolicy(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{})
		verify func(*testing.T, *Scheduler, error, map[string]interface{})
	}{
		{
			name: "Success_VolumeDeleted_RemovesFromCacheAndAssorter",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Pre-populate cache with the volume
				pvName := "pvc-123"
				autogrowCache.SetEffectivePolicyName(pvName, "policy-1")

				// Expect RemoveVolume to be called
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), pvName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":    pvName,
					"pvDeleted": true,
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.NoError(t, err, "reconcileVolumeEffectivePolicy should succeed for deleted volume")

				pvName := testData["pvName"].(string)
				_, getErr := s.autogrowCache.GetEffectivePolicyName(pvName)
				assert.Error(t, getErr, "effective policy should not exist in cache")
			},
		},
		{
			name: "Success_VolumeDeleted_AssorterRemoveFails_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Pre-populate cache with the volume
				pvName := "pvc-456"
				autogrowCache.SetEffectivePolicyName(pvName, "policy-2")

				// Expect RemoveVolume to fail
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), pvName).
					Return(assert.AnError).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":    pvName,
					"pvDeleted": true,
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.Error(t, err, "reconcileVolumeEffectivePolicy should return error if assorter fails")

				pvName := testData["pvName"].(string)
				_, getErr := s.autogrowCache.GetEffectivePolicyName(pvName)
				assert.Error(t, getErr, "effective policy should not exist in cache")
			},
		},
		{
			name: "Success_VolumeDeleted_CacheDeleteFails_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Volume not in cache, so DeleteEffectivePolicyName will fail
				pvName := "pvc-not-in-cache"

				// Expect RemoveVolume to be called (will succeed)
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), pvName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":    pvName,
					"pvDeleted": true,
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.Error(t, err, "reconcileVolumeEffectivePolicy should return error if cache delete fails")
			},
		},
		{
			name: "Success_VolumeDeleted_BothCacheAndAssorterFail_ReturnsMultipleErrors",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Volume not in cache, so DeleteEffectivePolicyName will fail
				pvName := "pvc-both-fail"

				// Expect RemoveVolume to also fail
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), pvName).
					Return(assert.AnError).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":    pvName,
					"pvDeleted": true,
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.Error(t, err, "reconcileVolumeEffectivePolicy should return error if both fail")
				// Verify that the error contains multiple errors (multiErr)
				assert.Contains(t, err.Error(), "key")
			},
		},
		{
			name: "Success_EmptyEffectivePolicy_CacheDeleteFails_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Volume not in cache, so DeleteEffectivePolicyName will fail
				pvName := "pvc-not-in-cache"

				// Expect RemoveVolume to be called (will succeed)
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), pvName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":       pvName,
					"pvDeleted":    false, // Volume exists but has no policy
					"pvPolicy":     "",    // No PV annotation
					"scAnnotation": "",    // No SC annotation
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.Error(t, err, "reconcileVolumeEffectivePolicy should return error if cache delete fails")
			},
		},
		{
			name: "Success_EmptyEffectivePolicy_RemovesFromCacheAndAssorter",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Pre-populate cache with the volume
				pvName := "pvc-789"
				autogrowCache.SetEffectivePolicyName(pvName, "policy-3")

				// Expect RemoveVolume to be called (no effective policy means remove from assorter)
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), pvName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":       pvName,
					"pvDeleted":    false, // Volume exists but has no policy
					"pvPolicy":     "",    // No PV annotation
					"scAnnotation": "",    // No SC annotation
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.NoError(t, err, "reconcileVolumeEffectivePolicy should succeed for empty effective policy")

				pvName := testData["pvName"].(string)
				effectivePolicy, getErr := s.autogrowCache.GetEffectivePolicyName(pvName)
				assert.Error(t, getErr, "effective policy should not exist in cache")
				assert.Empty(t, effectivePolicy, "effective policy should be removed from cache")
			},
		},
		{
			name: "Success_EmptyEffectivePolicy_AssorterRemoveFails_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Pre-populate cache with the volume
				pvName := "pvc-empty"
				autogrowCache.SetEffectivePolicyName(pvName, "old-policy")

				// Expect RemoveVolume to fail
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), pvName).
					Return(assert.AnError).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":       pvName,
					"pvDeleted":    false, // Volume exists but has no policy
					"pvPolicy":     "",    // No PV annotation
					"scAnnotation": "",    // No SC annotation
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.Error(t, err, "reconcileVolumeEffectivePolicy should return error if assorter fails")

				pvName := testData["pvName"].(string)
				_, getErr := s.autogrowCache.GetEffectivePolicyName(pvName)
				assert.Error(t, getErr, "effective policy should not exist in cache")
			},
		},
		{
			name: "Success_EmptyEffectivePolicy_BothCacheAndAssorterFail_ReturnsMultipleErrors",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Volume not in cache, so DeleteEffectivePolicyName will fail
				pvName := "pvc-both-fail-empty"

				// Expect RemoveVolume to also fail
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), pvName).
					Return(assert.AnError).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":       pvName,
					"pvDeleted":    false, // Volume exists but has no policy
					"pvPolicy":     "",    // No PV annotation
					"scAnnotation": "",    // No SC annotation
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.Error(t, err, "reconcileVolumeEffectivePolicy should return error if both fail")
				// Verify that the error contains multiple errors (multiErr)
				assert.Contains(t, err.Error(), "key")
			},
		},
		{
			name: "Success_WithEffectivePolicy_PVAnnotationTakesPrecedence_AddsToCache",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				pvName := "pvc-pv-precedence"
				pvPolicy := "pv-policy"
				scAnnotation := "sc-policy"

				// Expect AddVolume to be called
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), pvName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":         pvName,
					"pvDeleted":      false, // Volume exists
					"pvPolicy":       pvPolicy,
					"scAnnotation":   scAnnotation,
					"expectedPolicy": pvPolicy, // PV takes precedence
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.NoError(t, err, "reconcileVolumeEffectivePolicy should succeed")

				pvName := testData["pvName"].(string)
				expectedPolicy := testData["expectedPolicy"].(string)

				effectivePolicy, getErr := s.autogrowCache.GetEffectivePolicyName(pvName)
				assert.NoError(t, getErr, "effective policy should exist in cache")
				assert.Equal(t, expectedPolicy, effectivePolicy, "effective policy should be set from PV annotation")
			},
		},
		{
			name: "Success_WithEffectivePolicy_SCAnnotationUsedWhenNoPVPolicy_AddsToCache",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				pvName := "pvc-sc-fallback"
				pvPolicy := "" // No PV policy
				scAnnotation := "sc-policy"

				// Expect AddVolume to be called
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), pvName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":         pvName,
					"pvDeleted":      false, // Volume exists
					"pvPolicy":       pvPolicy,
					"scAnnotation":   scAnnotation,
					"expectedPolicy": scAnnotation, // SC is used when no PV policy
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.NoError(t, err, "reconcileVolumeEffectivePolicy should succeed")

				pvName := testData["pvName"].(string)
				expectedPolicy := testData["expectedPolicy"].(string)

				effectivePolicy, getErr := s.autogrowCache.GetEffectivePolicyName(pvName)
				assert.NoError(t, getErr, "effective policy should exist in cache")
				assert.Equal(t, expectedPolicy, effectivePolicy, "effective policy should be set from SC annotation")
			},
		},
		{
			name: "Success_WithEffectivePolicy_UpdatesExistingPolicy",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				pvName := "pvc-update"
				oldPolicy := "old-policy"
				newPolicy := "new-policy"

				// Pre-populate cache with old policy
				autogrowCache.SetEffectivePolicyName(pvName, oldPolicy)

				// Expect AddVolume to be called (idempotent)
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), pvName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":         pvName,
					"pvDeleted":      false, // Volume exists
					"pvPolicy":       newPolicy,
					"scAnnotation":   "",
					"expectedPolicy": newPolicy,
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.NoError(t, err, "reconcileVolumeEffectivePolicy should succeed")

				pvName := testData["pvName"].(string)
				expectedPolicy := testData["expectedPolicy"].(string)

				effectivePolicy, getErr := s.autogrowCache.GetEffectivePolicyName(pvName)
				assert.NoError(t, getErr, "effective policy should exist in cache")
				assert.Equal(t, expectedPolicy, effectivePolicy, "effective policy should be updated in cache")
			},
		},
		{
			name: "Success_WithEffectivePolicy_AssorterAddFails_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				pvName := "pvc-assorter-fail"
				pvPolicy := "test-policy"

				// Expect AddVolume to fail
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), pvName).
					Return(assert.AnError).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":         pvName,
					"pvDeleted":      false, // Volume exists
					"pvPolicy":       pvPolicy,
					"scAnnotation":   "",
					"expectedPolicy": pvPolicy,
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.Error(t, err, "reconcileVolumeEffectivePolicy should return error if assorter fails")

				pvName := testData["pvName"].(string)
				expectedPolicy := testData["expectedPolicy"].(string)

				effectivePolicy, getErr := s.autogrowCache.GetEffectivePolicyName(pvName)
				assert.NoError(t, getErr, "effective policy should exist in cache")
				assert.Equal(t, expectedPolicy, effectivePolicy, "effective policy should still be set in cache")
			},
		},
		{
			name: "Success_NewVolume_AddsToCache",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, map[string]interface{}) {
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				pvName := "pvc-new-volume"
				pvPolicy := "new-policy"

				// Volume should not exist in cache initially
				_, getErr := autogrowCache.GetEffectivePolicyName(pvName)
				require.Error(t, getErr, "volume should not exist in cache initially")

				// Expect AddVolume to be called
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), pvName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
				}

				ctx := getTestContext()

				testData := map[string]interface{}{
					"pvName":         pvName,
					"pvDeleted":      false, // Volume exists
					"pvPolicy":       pvPolicy,
					"scAnnotation":   "",
					"expectedPolicy": pvPolicy,
				}

				return scheduler, ctx, testData
			},
			verify: func(t *testing.T, s *Scheduler, err error, testData map[string]interface{}) {
				require.NoError(t, err, "reconcileVolumeEffectivePolicy should succeed")

				pvName := testData["pvName"].(string)
				expectedPolicy := testData["expectedPolicy"].(string)

				effectivePolicy, getErr := s.autogrowCache.GetEffectivePolicyName(pvName)
				assert.NoError(t, getErr, "effective policy should exist in cache")
				assert.Equal(t, expectedPolicy, effectivePolicy, "effective policy should be added to cache")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gomock controller for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup test dependencies
			scheduler, ctx, testData := tt.setup(ctrl)

			// Extract test parameters
			pvName := testData["pvName"].(string)
			pvDeleted := false
			if deleted, ok := testData["pvDeleted"].(bool); ok {
				pvDeleted = deleted
			}

			pvPolicy := ""
			if policy, ok := testData["pvPolicy"].(string); ok {
				pvPolicy = policy
			}

			scAnnotation := ""
			if annotation, ok := testData["scAnnotation"].(string); ok {
				scAnnotation = annotation
			}

			scName := ""
			if name, ok := testData["scName"].(string); ok {
				scName = name
			}

			// Call reconcileVolumeEffectivePolicy
			err := scheduler.reconcileVolumeEffectivePolicy(
				ctx,
				pvName,
				pvPolicy,
				pvDeleted,
				scName,
				scAnnotation,
			)

			// Verify results
			tt.verify(t, scheduler, err, testData)
		})
	}
}

// ============================================================================
// Test: handleTVPEvent
// ============================================================================

func TestHandleTVPEvent(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) (*Scheduler, context.Context, string)
		verify func(*testing.T, *Scheduler, error, string)
	}{
		{
			name: "Success_TVPFound_WithPolicyAndSC_CallsReconcileWithCorrectParams",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string) {
				// Create TVP with policy and SC
				tvpName := "test-tvp"
				pvName := "pv-123"
				tvpPolicy := "tvp-policy"
				scName := "test-sc"
				scAnnotation := "sc-policy"
				namespace := "trident"

				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID:       pvName,
					AutogrowPolicy: tvpPolicy,
					StorageClass:   scName,
				}

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: scName,
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: scAnnotation,
						},
					},
				}

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect AddVolume to be called with tvpName (not pvName)
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), tvpName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, tvpName
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpName string) {
				require.NoError(t, err, "handleTVPEvent should succeed")

				// Verify cache has the effective policy (TVP policy takes precedence)
				effectivePolicy, getErr := s.autogrowCache.GetEffectivePolicyName(tvpName)
				assert.NoError(t, getErr, "effective policy should exist in cache")
				assert.Equal(t, "tvp-policy", effectivePolicy, "effective policy should be tvp-policy (TVP annotation takes precedence)")
			},
		},
		{
			name: "Success_TVPFound_NoPolicyNoSC_CallsReconcileWithEmptyParams",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string) {
				tvpName := "test-tvp-no-policy"
				pvName := "pv-456"
				namespace := "trident"

				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID:       pvName,
					AutogrowPolicy: "", // No policy
					StorageClass:   "", // No SC
				}

				scLister, tvpLister, _ := setupTestListers(t, nil, []*v1.TridentVolumePublication{tvp})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Pre-populate cache so that delete succeeds
				autogrowCache.SetEffectivePolicyName(tvpName, "old-policy")

				// Expect RemoveVolume with tvpName (empty policy means remove)
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), tvpName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, tvpName
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpName string) {
				require.NoError(t, err, "handleTVPEvent should succeed with empty params")

				// Verify cache does NOT have the effective policy (no policy means removed)
				_, getErr := s.autogrowCache.GetEffectivePolicyName(tvpName)
				assert.Error(t, getErr, "effective policy should not exist in cache when no policy is set")
			},
		},
		{
			name: "Success_TVPBeingDeleted_CallsReconcileWithDeletionFlag",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string) {
				tvpName := "test-tvp-deleting"
				pvName := "pv-789"
				namespace := "trident"
				now := metav1.Now()

				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:              tvpName,
						Namespace:         namespace,
						DeletionTimestamp: &now, // Being deleted
					},
					VolumeID:       pvName,
					AutogrowPolicy: "some-policy",
					StorageClass:   "some-sc",
				}

				scLister, tvpLister, _ := setupTestListers(t, nil, []*v1.TridentVolumePublication{tvp})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Pre-populate cache with tvpName (not pvName)
				autogrowCache.SetEffectivePolicyName(tvpName, "old-policy")

				// Expect RemoveVolume with tvpName (deletion path)
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), tvpName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, tvpName
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpName string) {
				require.NoError(t, err, "handleTVPEvent should succeed for deleting TVP")

				// Verify cache does NOT have the effective policy (TVP deleted)
				_, getErr := s.autogrowCache.GetEffectivePolicyName(tvpName)
				assert.Error(t, getErr, "effective policy should not exist in cache when TVP is deleted")
			},
		},
		{
			name: "Success_TVPHasSCButSCNotFound_UsesEmptySCAnnotation",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string) {
				tvpName := "test-tvp-sc-not-found"
				pvName := "pv-sc-missing"
				tvpPolicy := "tvp-policy"
				scName := "non-existent-sc"
				namespace := "trident"

				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID:       pvName,
					AutogrowPolicy: tvpPolicy,
					StorageClass:   scName, // SC doesn't exist
				}

				// Don't add SC to lister
				scLister, tvpLister, _ := setupTestListers(t, nil, []*v1.TridentVolumePublication{tvp})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect AddVolume with tvpName (TVP policy is used, no SC annotation available)
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), tvpName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, tvpName
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpName string) {
				require.NoError(t, err, "handleTVPEvent should succeed even if SC not found")

				// Verify cache has the effective policy (TVP policy is used since SC not found)
				effectivePolicy, getErr := s.autogrowCache.GetEffectivePolicyName(tvpName)
				assert.NoError(t, getErr, "effective policy should exist in cache")
				assert.Equal(t, "tvp-policy", effectivePolicy, "effective policy should be tvp-policy (SC not found)")
			},
		},
		{
			name: "Success_TVPHasSCButSCBeingDeleted_UsesEmptySCAnnotation",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string) {
				tvpName := "test-tvp-sc-deleting"
				pvName := "pv-sc-deleting"
				tvpPolicy := "tvp-policy"
				scName := "deleting-sc"
				namespace := "trident"
				now := metav1.Now()

				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID:       pvName,
					AutogrowPolicy: tvpPolicy,
					StorageClass:   scName,
				}

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:              scName,
						DeletionTimestamp: &now, // Being deleted
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: "sc-policy", // Should be ignored
						},
					},
				}

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect AddVolume with tvpName (SC being deleted, annotation ignored)
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), tvpName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, tvpName
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpName string) {
				require.NoError(t, err, "handleTVPEvent should succeed with deleting SC")

				// Verify cache has the effective policy (TVP policy is used since SC is being deleted)
				effectivePolicy, getErr := s.autogrowCache.GetEffectivePolicyName(tvpName)
				assert.NoError(t, getErr, "effective policy should exist in cache")
				assert.Equal(t, "tvp-policy", effectivePolicy, "effective policy should be tvp-policy (SC being deleted)")
			},
		},
		{
			name: "Success_TVPHasSCButSCHasNoAnnotations_UsesEmptySCAnnotation",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string) {
				tvpName := "test-tvp-sc-no-annotations"
				pvName := "pv-sc-no-ann"
				tvpPolicy := "tvp-policy"
				scName := "sc-no-annotations"
				namespace := "trident"

				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID:       pvName,
					AutogrowPolicy: tvpPolicy,
					StorageClass:   scName,
				}

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:        scName,
						Annotations: nil, // No annotations
					},
				}

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect AddVolume with tvpName (no SC annotation)
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), tvpName).
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, tvpName
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpName string) {
				require.NoError(t, err, "handleTVPEvent should succeed with SC that has no annotations")

				// Verify cache has the effective policy (TVP policy is used since SC has no annotation)
				effectivePolicy, getErr := s.autogrowCache.GetEffectivePolicyName(tvpName)
				assert.NoError(t, getErr, "effective policy should exist in cache")
				assert.Equal(t, "tvp-policy", effectivePolicy, "effective policy should be tvp-policy (SC has no annotation)")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gomock controller for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup test dependencies
			scheduler, ctx, tvpName := tt.setup(ctrl)

			// Call handleTVPEvent
			err := scheduler.handleTVPEvent(ctx, tvpName)

			// Verify results (pass scheduler and tvpName for cache assertions)
			tt.verify(t, scheduler, err, tvpName)
		})
	}
}

// ============================================================================
// Test: handleStorageClassEvent
// ============================================================================

func TestHandleStorageClassEvent(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) (*Scheduler, context.Context, string, []string)
		verify func(*testing.T, *Scheduler, error, []string)
	}{
		{
			name: "Success_NoTVPsUsingStorageClass_ReturnsSuccessImmediately",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string, []string) {
				scName := "unused-sc"
				namespace := "trident"

				// Create SC but no TVPs using it
				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: scName,
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: "sc-policy",
						},
					},
				}

				// Create some TVPs but they use different SCs
				tvp1 := createTestTVP("tvp1", namespace, "vol1", "other-sc")
				tvp2 := createTestTVP("tvp2", namespace, "vol2", "another-sc")

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1, tvp2})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// No assorter expectations - no TVPs to process

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, scName, nil
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.NoError(t, err, "handleStorageClassEvent should succeed when no TVPs use the SC")
			},
		},
		{
			name: "Success_SCWithAnnotation_UpdatesAllAffectedTVPs",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string, []string) {
				scName := "test-sc"
				scAnnotation := "sc-policy"
				namespace := "trident"

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: scName,
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: scAnnotation,
						},
					},
				}

				// Create TVPs using this SC - some with TVP policy, some without
				tvp1 := createTestTVP("tvp1", namespace, "vol1", scName)
				tvp1.AutogrowPolicy = "" // No TVP policy, will use SC annotation

				tvp2 := createTestTVP("tvp2", namespace, "vol2", scName)
				tvp2.AutogrowPolicy = "tvp-policy" // Has TVP policy, takes precedence

				tvp3 := createTestTVP("tvp3", namespace, "vol3", "other-sc") // Different SC, should not be affected

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1, tvp2, tvp3})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect AddVolume for tvp1 (uses SC annotation) and tvp2 (uses TVP policy)
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp2").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, scName, []string{"tvp1", "tvp2"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.NoError(t, err, "handleStorageClassEvent should succeed")

				// Verify cache for tvp1 (should have SC annotation)
				policy1, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, err1, "tvp1 should have effective policy in cache")
				assert.Equal(t, "sc-policy", policy1, "tvp1 should use SC annotation")

				// Verify cache for tvp2 (should have TVP policy)
				policy2, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.NoError(t, err2, "tvp2 should have effective policy in cache")
				assert.Equal(t, "tvp-policy", policy2, "tvp2 should use TVP policy (precedence)")

				// Verify cache for tvp3 (should not be affected)
				_, err3 := s.autogrowCache.GetEffectivePolicyName("tvp3")
				assert.Error(t, err3, "tvp3 should not be in cache (different SC)")
			},
		},
		{
			name: "PartialSuccess_SCDeleted_RemovesPolicyFromAffectedTVPs_CacheDeleteFails",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string, []string) {
				scName := "deleted-sc"
				namespace := "trident"

				// Don't add SC to lister (simulates NotFound/deleted)

				// Create TVPs that were using this SC
				tvp1 := createTestTVP("tvp1", namespace, "vol1", scName)
				tvp1.AutogrowPolicy = "" // No TVP policy, was relying on SC

				tvp2 := createTestTVP("tvp2", namespace, "vol2", scName)
				tvp2.AutogrowPolicy = "tvp-policy" // Has TVP policy

				scLister, tvpLister, _ := setupTestListers(t, nil, []*v1.TridentVolumePublication{tvp1, tvp2})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect RemoveVolume for tvp1 (no policy now that SC is deleted)
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				// Expect AddVolume for tvp2 (still has TVP policy)
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp2").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, scName, []string{"tvp1", "tvp2"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "handleStorageClassEvent should return error when cache delete fails")

				// Verify cache for tvp1 (should be removed)
				_, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.Error(t, err1, "tvp1 should not have effective policy (no TVP policy, SC deleted)")

				// Verify cache for tvp2 (should have TVP policy)
				policy2, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.NoError(t, err2, "tvp2 should have effective policy in cache")
				assert.Equal(t, "tvp-policy", policy2, "tvp2 should still use TVP policy")
			},
		},
		{
			name: "PartialSuccess_SCBeingDeleted_RemovesSCAnnotationFromAffectedTVPs_CacheDeleteFails",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string, []string) {
				scName := "deleting-sc"
				namespace := "trident"
				now := metav1.Now()

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:              scName,
						DeletionTimestamp: &now, // Being deleted
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: "sc-policy", // Should be ignored
						},
					},
				}

				tvp1 := createTestTVP("tvp1", namespace, "vol1", scName)
				tvp1.AutogrowPolicy = "" // No TVP policy

				tvp2 := createTestTVP("tvp2", namespace, "vol2", scName)
				tvp2.AutogrowPolicy = "tvp-policy"

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1, tvp2})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect RemoveVolume for tvp1 (SC annotation ignored)
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				// Expect AddVolume for tvp2
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp2").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, scName, []string{"tvp1", "tvp2"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "handleStorageClassEvent should return error when cache delete fails")

				// Verify tvp1 removed from cache
				_, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.Error(t, err1, "tvp1 should not have effective policy (SC being deleted)")

				// Verify tvp2 still has TVP policy
				policy2, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.NoError(t, err2, "tvp2 should have effective policy")
				assert.Equal(t, "tvp-policy", policy2, "tvp2 should use TVP policy")
			},
		},
		{
			name: "PartialSuccess_SCHasNoAnnotation_TVPsWithoutPolicyRemoved_CacheDeleteFails",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string, []string) {
				scName := "no-annotation-sc"
				namespace := "trident"

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:        scName,
						Annotations: nil, // No annotations
					},
				}

				tvp1 := createTestTVP("tvp1", namespace, "vol1", scName)
				tvp1.AutogrowPolicy = "" // No TVP policy

				tvp2 := createTestTVP("tvp2", namespace, "vol2", scName)
				tvp2.AutogrowPolicy = "tvp-policy"

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1, tvp2})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect RemoveVolume for tvp1
				mockAssorter.EXPECT().
					RemoveVolume(gomock.Any(), "tvp1").
					Return(nil).
					Times(1)

				// Expect AddVolume for tvp2
				mockAssorter.EXPECT().
					AddVolume(gomock.Any(), "tvp2").
					Return(nil).
					Times(1)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, scName, []string{"tvp1", "tvp2"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "handleStorageClassEvent should return error when cache delete fails")

				// Verify tvp1 removed from cache
				_, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.Error(t, err1, "tvp1 should not have effective policy (no SC annotation, no TVP policy)")

				// Verify tvp2 has TVP policy
				policy2, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.NoError(t, err2, "tvp2 should have effective policy")
				assert.Equal(t, "tvp-policy", policy2, "tvp2 should use TVP policy")
			},
		},
		{
			name: "Success_TVPBeingDeleted_RemovedFromCache",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string, []string) {
				scName := "test-sc"
				namespace := "trident"
				now := metav1.Now()

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: scName,
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: "sc-policy",
						},
					},
				}

				tvp1 := createTestTVP("tvp1", namespace, "vol1", scName)
				tvp1.AutogrowPolicy = "tvp-policy"
				tvp1.ObjectMeta.DeletionTimestamp = &now // Being deleted

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Pre-populate cache
				autogrowCache.SetEffectivePolicyName("tvp1", "old-policy")

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

				ctx := getTestContext()
				return scheduler, ctx, scName, []string{"tvp1"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.NoError(t, err, "handleStorageClassEvent should succeed for deleting TVP")

				// Verify tvp1 removed from cache
				_, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.Error(t, err1, "tvp1 should not have effective policy (TVP being deleted)")
			},
		},
		{
			name: "PartialSuccess_AssorterFailures_ReturnsErrorButContinuesProcessingOtherTVPs",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, string, []string) {
				scName := "test-sc"
				namespace := "trident"

				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: scName,
						Annotations: map[string]string{
							kubernetes.AnnAutogrowPolicy: "sc-policy",
						},
					},
				}

				tvp1 := createTestTVP("tvp1", namespace, "vol1", scName)
				tvp1.AutogrowPolicy = ""

				tvp2 := createTestTVP("tvp2", namespace, "vol2", scName)
				tvp2.AutogrowPolicy = ""

				scLister, tvpLister, _ := setupTestListers(t, []*storagev1.StorageClass{sc}, []*v1.TridentVolumePublication{tvp1, tvp2})
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

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
				}

				ctx := getTestContext()
				return scheduler, ctx, scName, []string{"tvp1", "tvp2"}
			},
			verify: func(t *testing.T, s *Scheduler, err error, tvpNames []string) {
				require.Error(t, err, "handleStorageClassEvent should return error when assorter fails")

				// Both should still be in cache (cache update is independent of assorter)
				policy1, err1 := s.autogrowCache.GetEffectivePolicyName("tvp1")
				assert.NoError(t, err1, "tvp1 should be in cache")
				assert.Equal(t, "sc-policy", policy1)

				policy2, err2 := s.autogrowCache.GetEffectivePolicyName("tvp2")
				assert.NoError(t, err2, "tvp2 should be in cache")
				assert.Equal(t, "sc-policy", policy2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gomock controller for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup test dependencies
			scheduler, ctx, scName, tvpNames := tt.setup(ctrl)

			// Call handleStorageClassEvent
			err := scheduler.handleStorageClassEvent(ctx, scName)

			// Verify results (pass scheduler and tvpNames for cache assertions)
			tt.verify(t, scheduler, err, tvpNames)
		})
	}
}
