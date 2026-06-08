// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	tridentconfig "github.com/netapp/trident/config"
	mocklocks "github.com/netapp/trident/mocks/mock_pkg/mock_locks"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

// Shared sizes and names for economy flexvol autosize unit tests.
const (
	testEcoFlexvolName = "eco_bucket"

	testEcoFlexvolBytes        = uint64(10_000_000_000)
	testEcoPrimaryLunBytes     = uint64(5_000_000_000)
	testEcoRemainingLunBytes   = uint64(3_000_000_000)
	testEcoDeleteOtherLunBytes = uint64(1_073_741_824)
	testEcoDeletedLunBytes     = uint64(2_000_000_000)
	testEcoGrowFallbackBytes   = uint64(1_073_741_824)
)

var allVolumeAutosizeModes = []string{
	api.VolumeAutosizeModeOff,
	api.VolumeAutosizeModeGrow,
	api.VolumeAutosizeModeGrowShrink,
}

func testEcoFlexvolBytesAfterDelete() uint64 {
	return testEcoFlexvolBytes - testEcoDeletedLunBytes
}

func economyFlexvolLUNListPath(flexvol string) string {
	return fmt.Sprintf("/vol/%s/*", flexvol)
}

func economySizeString(sizeBytes uint64) string {
	return strconv.FormatUint(sizeBytes, 10)
}

func economyLun(sizeBytes uint64, name string) api.Lun {
	return api.Lun{Size: economySizeString(sizeBytes), Name: name}
}

func economyLuns(sizeBytes uint64, name string) []api.Lun {
	return []api.Lun{economyLun(sizeBytes, name)}
}

func economyFlexvolForTest(sizeBytes uint64, autosizeMode string) *api.Volume {
	return &api.Volume{
		Size:              economySizeString(sizeBytes),
		AutosizeMode:      autosizeMode,
		SnapshotReserve:   0,
		SnapshotSpaceUsed: 0,
	}
}

func expectEconomyFlexvolOptimalResize(
	mockAPI *mockapi.MockOntapAPI,
	testCtx context.Context,
	flexvol string,
	vol *api.Volume,
	luns []api.Lun,
	expectedSetSize string,
	expectVolumeSetSize bool,
) {
	mockAPI.EXPECT().VolumeInfo(testCtx, flexvol).Return(vol, nil).Times(1)
	mockAPI.EXPECT().LunList(testCtx, economyFlexvolLUNListPath(flexvol)).Return(luns, nil).Times(1)
	if expectVolumeSetSize {
		mockAPI.EXPECT().VolumeSetSize(testCtx, flexvol, expectedSetSize).Return(nil).Times(1)
	}
}

// expectEconomyFlexvolOptimalResizeWithCachedLUNs expects VolumeInfo and VolumeSetSize only (LUN list already on bucket).
func expectEconomyFlexvolOptimalResizeWithCachedLUNs(
	mockAPI *mockapi.MockOntapAPI,
	testCtx context.Context,
	flexvol string,
	vol *api.Volume,
	expectedSetSize string,
) {
	mockAPI.EXPECT().VolumeInfo(testCtx, flexvol).Return(vol, nil).Times(1)
	mockAPI.EXPECT().VolumeSetSize(testCtx, flexvol, expectedSetSize).Return(nil).Times(1)
}

// expectEconomyFlexvolOptimalResizeFromBucket expects only VolumeSetSize (VolumeInfo and LUN list already on bucket).
func expectEconomyFlexvolOptimalResizeFromBucket(
	mockAPI *mockapi.MockOntapAPI,
	testCtx context.Context,
	flexvol string,
	expectedSetSize string,
) {
	mockAPI.EXPECT().VolumeSetSize(testCtx, flexvol, expectedSetSize).Return(nil).Times(1)
}

func TestSumLUNBytes(t *testing.T) {
	total, err := sumLUNBytes(api.Luns{
		{Size: "1073741824"},
		{Size: "2147483648"},
	})
	assert.NoError(t, err)
	assert.Equal(t, uint64(3221225472), total)

	_, err = sumLUNBytes(api.Luns{{Size: "0"}})
	assert.Error(t, err)
}

func TestNewFlexvolBucket(t *testing.T) {
	luns := api.Luns{{Size: "5000000000"}, {Size: "3000000000"}}
	bucket, err := newFlexvolBucket(luns)
	assert.NoError(t, err)
	assert.Equal(t, uint64(8000000000), bucket.totalLUNBytes)
	assert.Equal(t, luns, bucket.luns)
}

func TestGetOptimalSizeForFlexvol_ReusesBucket(t *testing.T) {
	testCtx := context.Background()
	vol := economyFlexvolForTest(testEcoPrimaryLunBytes, api.VolumeAutosizeModeGrowShrink)
	luns := economyLuns(testEcoPrimaryLunBytes, "lun_a")
	bucket, err := newFlexvolBucket(luns)
	assert.NoError(t, err)
	bucket.vol = vol

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)

	optimal, volOut, err := (&SANEconomyStorageDriver{API: mockAPI}).getOptimalSizeForFlexvol(
		testCtx, testEcoFlexvolName, 0, bucket,
	)
	assert.NoError(t, err)
	assert.Equal(t, vol, volOut)
	assert.Equal(t, testEcoPrimaryLunBytes, optimal)
}

func TestResizeFlexvol_AutosizeModes(t *testing.T) {
	testCtx := context.Background()
	luns := economyLuns(testEcoPrimaryLunBytes, "lun_a")
	optimalSize := economySizeString(testEcoPrimaryLunBytes)
	currentSize := economySizeString(testEcoFlexvolBytes)

	for _, tc := range []struct {
		name                string
		autosizeMode        string
		expectedSetSize     string
		expectVolumeSetSize bool
	}{
		{name: "off", autosizeMode: api.VolumeAutosizeModeOff, expectedSetSize: optimalSize, expectVolumeSetSize: true},
		{name: "grow_shrink", autosizeMode: api.VolumeAutosizeModeGrowShrink, expectedSetSize: optimalSize, expectVolumeSetSize: true},
		{name: "grow", autosizeMode: api.VolumeAutosizeModeGrow, expectedSetSize: currentSize, expectVolumeSetSize: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI, driver := newMockOntapSanEcoDriver(t)
			vol := economyFlexvolForTest(testEcoFlexvolBytes, tc.autosizeMode)
			expectEconomyFlexvolOptimalResize(
				mockAPI, testCtx, testEcoFlexvolName, vol, luns, tc.expectedSetSize, tc.expectVolumeSetSize,
			)

			err := driver.resizeFlexvol(testCtx, testEcoFlexvolName, 0, true, nil)
			assert.NoError(t, err)
		})
	}

	t.Run("grow mode with unreadable volume size returns error", func(t *testing.T) {
		mockAPI, driver := newMockOntapSanEcoDriver(t)
		vol := &api.Volume{AutosizeMode: api.VolumeAutosizeModeGrow, SnapshotReserve: 0}
		mockAPI.EXPECT().VolumeInfo(testCtx, testEcoFlexvolName).Return(vol, nil)
		mockAPI.EXPECT().LunList(testCtx, economyFlexvolLUNListPath(testEcoFlexvolName)).Return(luns, nil)

		err := driver.resizeFlexvol(testCtx, testEcoFlexvolName, 0, true, nil)
		assert.Error(t, err)
	})
}

func TestResizeFlexvol_OptimalWithoutGrowPolicy(t *testing.T) {
	testCtx := context.Background()
	luns := economyLuns(testEcoPrimaryLunBytes, "lun_a")
	optimalSize := economySizeString(testEcoPrimaryLunBytes)

	for _, autosizeMode := range allVolumeAutosizeModes {
		t.Run(autosizeMode, func(t *testing.T) {
			mockAPI, driver := newMockOntapSanEcoDriver(t)
			vol := economyFlexvolForTest(testEcoFlexvolBytes, autosizeMode)
			bucket, err := newFlexvolBucket(luns)
			assert.NoError(t, err)
			bucket.vol = vol
			expectEconomyFlexvolOptimalResizeFromBucket(mockAPI, testCtx, testEcoFlexvolName, optimalSize)

			err = driver.resizeFlexvol(testCtx, testEcoFlexvolName, 0, false, bucket)
			assert.NoError(t, err)
		})
	}
}

func TestResizeFlexvol_FallbackGrowWhenOptimalUnavailable(t *testing.T) {
	testCtx := context.Background()

	mockAPI, driver := newMockOntapSanEcoDriver(t)
	mockAPI.EXPECT().VolumeInfo(testCtx, testEcoFlexvolName).Return(nil, errors.New("volume info unavailable"))
	mockAPI.EXPECT().VolumeSetSize(
		testCtx, testEcoFlexvolName, "+"+economySizeString(testEcoGrowFallbackBytes),
	).Return(nil)

	err := driver.resizeFlexvol(testCtx, testEcoFlexvolName, testEcoGrowFallbackBytes, true, nil)
	assert.NoError(t, err)
}

func TestResizeFlexvolAfterLUNDelete_AutosizeModes(t *testing.T) {
	testCtx := context.Background()
	remainingLuns := economyLuns(testEcoRemainingLunBytes, "lun_remaining")
	optimalSize := economySizeString(testEcoRemainingLunBytes)
	afterDeleteSize := economySizeString(testEcoFlexvolBytesAfterDelete())

	for _, tc := range []struct {
		name            string
		autosizeMode    string
		deletedLunBytes uint64
		subtractDeleted bool
	}{
		{
			name: "grow with known deleted LUN size", autosizeMode: api.VolumeAutosizeModeGrow,
			deletedLunBytes: testEcoDeletedLunBytes, subtractDeleted: true,
		},
		{
			name: "grow with unknown deleted LUN size", autosizeMode: api.VolumeAutosizeModeGrow,
			deletedLunBytes: 0, subtractDeleted: false,
		},
		{
			name: "off with known deleted LUN size", autosizeMode: api.VolumeAutosizeModeOff,
			deletedLunBytes: testEcoDeletedLunBytes, subtractDeleted: false,
		},
		{
			name: "grow_shrink with known deleted LUN size", autosizeMode: api.VolumeAutosizeModeGrowShrink,
			deletedLunBytes: testEcoDeletedLunBytes, subtractDeleted: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI, driver := newMockOntapSanEcoDriver(t)
			preDeleteVol := economyFlexvolForTest(testEcoFlexvolBytes, tc.autosizeMode)

			if tc.subtractDeleted {
				mockAPI.EXPECT().VolumeSetSize(testCtx, testEcoFlexvolName, afterDeleteSize).Return(nil)
				err := driver.resizeFlexvolAfterLUNDelete(
					testCtx, testEcoFlexvolName, tc.deletedLunBytes, preDeleteVol, nil,
				)
				assert.NoError(t, err)
				return
			}

			bucket, err := newFlexvolBucket(remainingLuns)
			assert.NoError(t, err)
			expectEconomyFlexvolOptimalResizeWithCachedLUNs(
				mockAPI, testCtx, testEcoFlexvolName, preDeleteVol, optimalSize,
			)
			err = driver.resizeFlexvolAfterLUNDelete(
				testCtx, testEcoFlexvolName, tc.deletedLunBytes, preDeleteVol, bucket,
			)
			assert.NoError(t, err)
		})
	}

	t.Run("volume info failure returns error", func(t *testing.T) {
		mockAPI, driver := newMockOntapSanEcoDriver(t)
		mockAPI.EXPECT().VolumeInfo(testCtx, testEcoFlexvolName).Return(nil, errors.New("volume info failed"))

		err := driver.resizeFlexvolAfterLUNDelete(testCtx, testEcoFlexvolName, testEcoDeletedLunBytes, nil, nil)
		assert.Error(t, err)
	})

	t.Run("grow mode avoids over-shrink when ONTAP already shrank flexvol", func(t *testing.T) {
		mockAPI, driver := newMockOntapSanEcoDriver(t)
		preDeleteVol := economyFlexvolForTest(testEcoFlexvolBytes, api.VolumeAutosizeModeGrow)
		targetFromPreDelete := economySizeString(testEcoFlexvolBytesAfterDelete())
		optimalWouldShrinkTo := economySizeString(testEcoRemainingLunBytes)

		assert.NotEqual(t, optimalWouldShrinkTo, targetFromPreDelete,
			"test must model over-shrink if optimal sizing used post-delete state")

		mockAPI.EXPECT().VolumeSetSize(testCtx, testEcoFlexvolName, targetFromPreDelete).Return(nil).Times(1)

		err := driver.resizeFlexvolAfterLUNDelete(
			testCtx, testEcoFlexvolName, testEcoDeletedLunBytes, preDeleteVol, nil,
		)
		assert.NoError(t, err)
	})
}

func TestDeleteBucketIfEmpty_AutosizeModes(t *testing.T) {
	testCtx := context.Background()
	luns := []api.Lun{
		economyLun(testEcoDeleteOtherLunBytes, "lun_other"),
	}
	luns[0].VolumeName = testEcoFlexvolName
	optimalSize := economySizeString(testEcoDeleteOtherLunBytes)
	afterDeleteSize := economySizeString(testEcoFlexvolBytesAfterDelete())

	for _, tc := range []struct {
		name            string
		autosizeMode    string
		deletedLunBytes uint64
		subtractDeleted bool
	}{
		{
			name: "off with known deleted LUN size", autosizeMode: api.VolumeAutosizeModeOff,
			deletedLunBytes: testEcoDeletedLunBytes, subtractDeleted: false,
		},
		{
			name: "grow_shrink with known deleted LUN size", autosizeMode: api.VolumeAutosizeModeGrowShrink,
			deletedLunBytes: testEcoDeletedLunBytes, subtractDeleted: false,
		},
		{
			name: "grow with unknown deleted LUN size", autosizeMode: api.VolumeAutosizeModeGrow,
			deletedLunBytes: 0, subtractDeleted: false,
		},
		{
			name: "grow with known deleted LUN size", autosizeMode: api.VolumeAutosizeModeGrow,
			deletedLunBytes: testEcoDeletedLunBytes, subtractDeleted: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI, driver := newMockOntapSanEcoDriver(t)
			preDeleteVol := economyFlexvolForTest(testEcoFlexvolBytes, tc.autosizeMode)

			mockAPI.EXPECT().LunList(testCtx, gomock.Any()).Return(luns, nil).Times(1)

			if tc.subtractDeleted {
				mockAPI.EXPECT().VolumeSetSize(testCtx, testEcoFlexvolName, afterDeleteSize).Return(nil).Times(1)
			} else {
				expectEconomyFlexvolOptimalResizeWithCachedLUNs(
					mockAPI, testCtx, testEcoFlexvolName, preDeleteVol, optimalSize,
				)
			}

			err := driver.deleteBucketIfEmpty(testCtx, testEcoFlexvolName, tc.deletedLunBytes, preDeleteVol)
			assert.NoError(t, err)
		})
	}
}

func TestResizeFlexvol_VolumeSetSizeError(t *testing.T) {
	testCtx := context.Background()
	luns := economyLuns(testEcoPrimaryLunBytes, "lun_a")
	optimalSize := economySizeString(testEcoPrimaryLunBytes)
	setSizeErr := errors.New("volume set size failed")

	mockAPI, driver := newMockOntapSanEcoDriver(t)
	vol := economyFlexvolForTest(testEcoFlexvolBytes, api.VolumeAutosizeModeGrowShrink)
	mockAPI.EXPECT().VolumeInfo(testCtx, testEcoFlexvolName).Return(vol, nil).Times(1)
	mockAPI.EXPECT().LunList(testCtx, economyFlexvolLUNListPath(testEcoFlexvolName)).Return(luns, nil).Times(1)
	mockAPI.EXPECT().VolumeSetSize(testCtx, testEcoFlexvolName, optimalSize).Return(setSizeErr).Times(1)

	err := driver.resizeFlexvol(testCtx, testEcoFlexvolName, 0, true, nil)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "flexvol resize failed")
}

type trackedFlexvolGuard struct {
	name    string
	tracker *flexvolLockTracker
}

func (g *trackedFlexvolGuard) Name() string {
	return g.name
}

func (g *trackedFlexvolGuard) Unlock() {
	g.tracker.release(g.name)
}

type flexvolLockTracker struct {
	mu     sync.Mutex
	counts map[string]int
}

func newFlexvolLockTracker() *flexvolLockTracker {
	return &flexvolLockTracker{counts: make(map[string]int)}
}

func (tr *flexvolLockTracker) acquire(name string) {
	tr.mu.Lock()
	tr.counts[name]++
	tr.mu.Unlock()
}

func (tr *flexvolLockTracker) release(name string) {
	tr.mu.Lock()
	tr.counts[name]--
	tr.mu.Unlock()
}

func (tr *flexvolLockTracker) guard(name string) locks.Guard {
	tr.acquire(name)
	return &trackedFlexvolGuard{name: name, tracker: tr}
}

func (tr *flexvolLockTracker) isHeld(name string) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.counts[name] > 0
}

func assertFlexvolHeld(t *testing.T, tr *flexvolLockTracker, name, operation string) {
	t.Helper()
	if !tr.isHeld(name) {
		t.Fatalf("%s: expected flexvol %q locked", operation, name)
	}
}

func assertFlexvolNotHeld(t *testing.T, tr *flexvolLockTracker, name, operation string) {
	t.Helper()
	if tr.isHeld(name) {
		t.Fatalf("%s: expected flexvol %q not locked", operation, name)
	}
}

func economyDriverWithMockFlexvolLocks(t *testing.T) (
	*mockapi.MockOntapAPI, *SANEconomyStorageDriver, *mocklocks.MockNamedLocker, *flexvolLockTracker,
) {
	t.Helper()
	mockAPI, driver := newMockOntapSanEcoDriver(t)
	lockCtrl := gomock.NewController(t)
	mockLocks := mocklocks.NewMockNamedLocker(lockCtrl)
	tracker := newFlexvolLockTracker()
	driver.flexvolLocks = mockLocks
	return mockAPI, driver, mockLocks, tracker
}

func TestDestroy_PreDeleteSizingUnderFlexvolLock(t *testing.T) {
	testCtx := context.Background()
	mockAPI, driver, mockLocks, tracker := economyDriverWithMockFlexvolLocks(t)
	driver.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	bucketVol := testEcoFlexvolName
	internalName := "storagePrefix_vol1"
	lunPath := GetLUNPathEconomy(bucketVol, internalName)
	helper := driver.helper
	snapPattern := helper.GetSnapPathPatternForVolume(helper.GetExternalVolumeNameFromPath(lunPath))

	mainLun := economyLun(testEcoDeletedLunBytes, "lun_"+internalName)
	mainLun.VolumeName = bucketVol
	remainingLuns := []api.Lun{economyLun(testEcoRemainingLunBytes, "lun_other")}
	remainingLuns[0].VolumeName = bucketVol
	flexVol := economyFlexvolForTest(testEcoFlexvolBytes, api.VolumeAutosizeModeGrow)

	mockLocks.EXPECT().LockWithGuard(bucketVol).DoAndReturn(func(name string) locks.Guard {
		return tracker.guard(name)
	}).Times(1)

	mockAPI.EXPECT().LunList(testCtx, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string) (api.Luns, error) {
			assertFlexvolNotHeld(t, tracker, bucketVol, "LUN exists check")
			return api.Luns{mainLun}, nil
		},
	)
	mockAPI.EXPECT().LunList(testCtx, snapPattern).DoAndReturn(
		func(_ context.Context, _ string) (api.Luns, error) {
			assertFlexvolNotHeld(t, tracker, bucketVol, "snapshot enumeration")
			return api.Luns{}, nil
		},
	)
	mockAPI.EXPECT().LunGetByName(testCtx, lunPath).DoAndReturn(
		func(_ context.Context, _ string) (*api.Lun, error) {
			assertFlexvolHeld(t, tracker, bucketVol, "pre-delete LUN size read")
			return &api.Lun{Size: economySizeString(testEcoDeletedLunBytes), Name: internalName}, nil
		},
	)
	mockAPI.EXPECT().VolumeInfo(testCtx, bucketVol).DoAndReturn(
		func(_ context.Context, _ string) (*api.Volume, error) {
			assertFlexvolHeld(t, tracker, bucketVol, "pre-delete VolumeInfo read")
			return flexVol, nil
		},
	)
	mockAPI.EXPECT().LunListIgroupsMapped(testCtx, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string) ([]string, error) {
			assertFlexvolHeld(t, tracker, bucketVol, "LUN unmap preparation")
			return []string{"igroup1"}, nil
		},
	)
	mockAPI.EXPECT().LunUnmap(testCtx, "igroup1", gomock.Any()).Return(nil)
	mockAPI.EXPECT().LunDestroy(testCtx, lunPath).DoAndReturn(
		func(_ context.Context, _ string) error {
			assertFlexvolHeld(t, tracker, bucketVol, "LUN destroy")
			return nil
		},
	)
	mockAPI.EXPECT().LunList(testCtx, economyFlexvolLUNListPath(bucketVol)).DoAndReturn(
		func(_ context.Context, _ string) (api.Luns, error) {
			assertFlexvolHeld(t, tracker, bucketVol, "post-delete flexvol LUN list")
			return remainingLuns, nil
		},
	)
	mockAPI.EXPECT().VolumeSetSize(testCtx, bucketVol, economySizeString(testEcoFlexvolBytesAfterDelete())).Return(nil)

	err := driver.Destroy(testCtx, &storage.VolumeConfig{
		InternalName: internalName,
		Size:         "1g",
		Encryption:   "false",
		FileSystem:   "xfs",
	})
	assert.NoError(t, err)
	assertFlexvolNotHeld(t, tracker, bucketVol, "after Destroy")
}

func TestDeleteSnapshot_PreDeleteSizingUnderFlexvolLock(t *testing.T) {
	testCtx := context.Background()
	mockAPI, driver, mockLocks, tracker := economyDriverWithMockFlexvolLocks(t)
	driver.helper = NewTestLUNHelper("storagePrefix_", tridentconfig.ContextCSI)

	bucketVol := testEcoFlexvolName
	internalVolName := "storagePrefix_vol1"
	snapInternalName := "snap_1"
	snapLunName := driver.helper.GetSnapshotName(internalVolName, snapInternalName)
	snapPath := GetLUNPathEconomy(bucketVol, snapLunName)
	existsPattern := "/vol/*/" + snapLunName

	snapLun := economyLun(testEcoDeletedLunBytes, snapPath)
	snapLun.VolumeName = bucketVol
	remainingLuns := []api.Lun{economyLun(testEcoRemainingLunBytes, "lun_other")}
	remainingLuns[0].VolumeName = bucketVol
	flexVol := economyFlexvolForTest(testEcoFlexvolBytes, api.VolumeAutosizeModeGrow)

	mockLocks.EXPECT().LockWithGuard(bucketVol).DoAndReturn(func(name string) locks.Guard {
		return tracker.guard(name)
	}).Times(1)

	mockAPI.EXPECT().LunList(testCtx, existsPattern).DoAndReturn(
		func(_ context.Context, _ string) (api.Luns, error) {
			assertFlexvolNotHeld(t, tracker, bucketVol, "snapshot LUN exists check")
			return api.Luns{snapLun}, nil
		},
	)
	mockAPI.EXPECT().LunGetByName(testCtx, snapPath).DoAndReturn(
		func(_ context.Context, _ string) (*api.Lun, error) {
			assertFlexvolHeld(t, tracker, bucketVol, "pre-delete snapshot LUN size read")
			return &snapLun, nil
		},
	)
	mockAPI.EXPECT().VolumeInfo(testCtx, bucketVol).DoAndReturn(
		func(_ context.Context, _ string) (*api.Volume, error) {
			assertFlexvolHeld(t, tracker, bucketVol, "pre-delete VolumeInfo read")
			return flexVol, nil
		},
	)
	mockAPI.EXPECT().LunDestroy(testCtx, snapPath).DoAndReturn(
		func(_ context.Context, _ string) error {
			assertFlexvolHeld(t, tracker, bucketVol, "snapshot LUN destroy")
			return nil
		},
	)
	mockAPI.EXPECT().LunList(testCtx, economyFlexvolLUNListPath(bucketVol)).DoAndReturn(
		func(_ context.Context, _ string) (api.Luns, error) {
			assertFlexvolHeld(t, tracker, bucketVol, "post-delete flexvol LUN list")
			return remainingLuns, nil
		},
	)
	mockAPI.EXPECT().VolumeSetSize(testCtx, bucketVol, economySizeString(testEcoFlexvolBytesAfterDelete())).Return(nil)

	err := driver.DeleteSnapshot(testCtx, &storage.SnapshotConfig{
		InternalName:       snapInternalName,
		VolumeName:         bucketVol,
		VolumeInternalName: internalVolName,
	}, nil)
	assert.NoError(t, err)
	assertFlexvolNotHeld(t, tracker, bucketVol, "after DeleteSnapshot")
}
