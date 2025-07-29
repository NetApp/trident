// Copyright 2022 NetApp, Inc. All Rights Reserved.
package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"

	"github.com/netapp/trident/frontend/csi"
)

func TestSupportsFeature(t *testing.T) {
	supportedTests := []struct {
		versionInfo version.Info
		expected    bool
	}{
		{version.Info{GitVersion: "v1.14.3"}, false},
		{version.Info{GitVersion: "v1.16.2"}, true},
		{version.Info{GitVersion: "garbage"}, false},
	}

	for _, tc := range supportedTests {
		plugin := helper{kubeVersion: &tc.versionInfo}
		supported := plugin.SupportsFeature(context.Background(), csi.ExpandCSIVolumes)
		if tc.expected {
			assert.True(t, supported, "Expected true")
		} else {
			assert.False(t, supported, "Expected false")
		}
	}
}

func TestGetDataSizeFromTotalSize(t *testing.T) {
	var totalSize uint64 = 100
	var snapshotReservePercent int = 20
	var expectedDataSize uint64 = 80

	h := helper{}
	result := h.getDataSizeFromTotalSize(context.Background(), totalSize, snapshotReservePercent)

	assert.Equal(t, expectedDataSize, result, "Data size not as expected")
}

func TestValidateKubeVersion(t *testing.T) {
	testCases := []struct {
		name        string
		kubeVersion *version.Info
		assertErr   assert.ErrorAssertionFunc
	}{
		{
			name: "Valid version",
			kubeVersion: &version.Info{
				GitVersion: "v1.28.0",
				Major:      "1",
				Minor:      "28",
			},
			assertErr: assert.NoError,
		},
		{
			name: "Unsupported version",
			kubeVersion: &version.Info{
				GitVersion: "v1.15.0", // Below supported version, triggers warning
				Major:      "1",
				Minor:      "15",
			},
			assertErr: assert.NoError, // Warning is logged, but no error returned
		},
		{
			name: "Invalid semantic version",
			kubeVersion: &version.Info{
				GitVersion: "invalid-version",
				Major:      "1",
				Minor:      "xx",
			},
			assertErr: assert.Error,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := &helper{
				kubeVersion: tc.kubeVersion,
			}
			err := h.validateKubeVersion()

			tc.assertErr(t, err)
		})
	}
}

func TestGetStorageClassForPVC(t *testing.T) {
	class := "gold"

	testCases := []struct {
		name          string
		pvc           *v1.PersistentVolumeClaim
		expectedClass string
	}{
		{
			name: "StorageClass set",
			pvc: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: &class,
				},
			},
			expectedClass: "gold",
		},
		{
			name: "StorageClass nil",
			pvc: &v1.PersistentVolumeClaim{
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: nil,
				},
			},
			expectedClass: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getStorageClassForPVC(tc.pvc)
			assert.Equal(t, tc.expectedClass, result)
		})
	}
}

func TestCheckValidStorageClassReceived(t *testing.T) {
	ctx := context.Background()
	class := "standard"

	testCases := []struct {
		name      string
		pvc       *v1.PersistentVolumeClaim
		assertErr assert.ErrorAssertionFunc
	}{
		{
			name: "Valid storage class",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "pvc1"},
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: &class,
				},
			},
			assertErr: assert.NoError,
		},
		{
			name: "Nil storage class",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "pvc2"},
				Spec:       v1.PersistentVolumeClaimSpec{},
			},
			assertErr: assert.Error,
		},
		{
			name: "Empty storage class",
			pvc: &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{Name: "pvc3"},
				Spec: v1.PersistentVolumeClaimSpec{
					StorageClassName: new(string), // empty string pointer
				},
			},
			assertErr: assert.Error,
		},
	}

	h := &helper{}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := h.checkValidStorageClassReceived(ctx, tc.pvc)

			tc.assertErr(t, err)
		})
	}
}
