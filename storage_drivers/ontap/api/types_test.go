// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSnapmirrorTypes tests all snapmirror type methods and constants
func TestSnapmirrorTypes(t *testing.T) {
	// Test data for all snapmirror types
	states := map[SnapmirrorState]bool{
		SnapmirrorStateUninitialized: true, // Only this should be uninitialized
		SnapmirrorStateSnapmirrored:  false,
		SnapmirrorStateBrokenOffZapi: false,
		SnapmirrorStateBrokenOffRest: false,
		SnapmirrorStateSynchronizing: false,
		SnapmirrorStateInSync:        false,
		SnapmirrorState("invalid"):   false,
	}

	statuses := map[SnapmirrorStatus]map[string]bool{
		SnapmirrorStatusIdle:         {"IsIdle": true},
		SnapmirrorStatusAborting:     {"IsAborting": true},
		SnapmirrorStatusBreaking:     {"IsBreaking": true},
		SnapmirrorStatusTransferring: {"IsTransferring": true},
		SnapmirrorStatusQuiescing:    {},
		SnapmirrorStatusFinalizing:   {},
		SnapmirrorStatusAborted:      {},
		SnapmirrorStatusFailed:       {},
		SnapmirrorStatusHardAborted:  {},
		SnapmirrorStatusQueued:       {},
		SnapmirrorStatusSuccess:      {},
		SnapmirrorStatus("invalid"):  {},
	}

	policyTypes := []struct {
		policyType SnapmirrorPolicyType
		isSync     bool
		isAsync    bool
	}{
		{SnapmirrorPolicyZAPITypeSync, true, false},
		{SnapmirrorPolicyRESTTypeSync, true, false},
		{SnapmirrorPolicyZAPITypeAsync, false, true},
		{SnapmirrorPolicyRESTTypeAsync, false, true},
		{SnapmirrorPolicyType("invalid"), false, false},
	}

	// Test state methods
	for state, expectUninitialized := range states {
		assert.Equal(t, expectUninitialized, state.IsUninitialized(),
			"State %s IsUninitialized should return %v", state, expectUninitialized)
	}

	// Test status methods
	for status, methods := range statuses {
		assert.Equal(t, methods["IsIdle"], status.IsIdle())
		assert.Equal(t, methods["IsAborting"], status.IsAborting())
		assert.Equal(t, methods["IsBreaking"], status.IsBreaking())
		assert.Equal(t, methods["IsTransferring"], status.IsTransferring())
	}

	// Test policy type methods
	for _, tc := range policyTypes {
		assert.Equal(t, tc.isSync, tc.policyType.IsSnapmirrorPolicyTypeSync())
		assert.Equal(t, tc.isAsync, tc.policyType.IsSnapmirrorPolicyTypeAsync())
		if tc.policyType != "invalid" {
			assert.NotEqual(t, tc.isSync, tc.isAsync, "Cannot be both sync and async")
		}
	}
}

// TestConstants validates all snapmirror constants and type definitions
func TestConstants(t *testing.T) {
	// Test constant values and slice types in a single table
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{"PolicyRule", func(t *testing.T) {
			assert.Equal(t, "all_source_snapshots", SnapmirrorPolicyRuleAll)
		}},
		{"StateConstants", func(t *testing.T) {
			expected := map[SnapmirrorState]string{
				SnapmirrorStateUninitialized: "uninitialized",
				SnapmirrorStateSnapmirrored:  "snapmirrored",
				SnapmirrorStateBrokenOffZapi: "broken-off",
				SnapmirrorStateBrokenOffRest: "broken_off",
				SnapmirrorStateSynchronizing: "synchronizing",
				SnapmirrorStateInSync:        "in_sync",
			}
			for state, expectedValue := range expected {
				assert.Equal(t, expectedValue, string(state))
			}
			assert.NotEqual(t, SnapmirrorStateBrokenOffZapi, SnapmirrorStateBrokenOffRest)
		}},
		{"StatusConstants", func(t *testing.T) {
			statuses := []SnapmirrorStatus{
				SnapmirrorStatusIdle, SnapmirrorStatusAborting, SnapmirrorStatusBreaking,
				SnapmirrorStatusQuiescing, SnapmirrorStatusTransferring, SnapmirrorStatusFinalizing,
				SnapmirrorStatusAborted, SnapmirrorStatusFailed, SnapmirrorStatusHardAborted,
				SnapmirrorStatusQueued, SnapmirrorStatusSuccess,
			}
			uniqueValues := make(map[SnapmirrorStatus]bool)
			for _, status := range statuses {
				assert.NotEmpty(t, string(status))
				assert.False(t, uniqueValues[status], "Status should be unique")
				uniqueValues[status] = true
			}
		}},
		{"PolicyTypeConstants", func(t *testing.T) {
			expected := map[SnapmirrorPolicyType]string{
				SnapmirrorPolicyZAPITypeSync:  "sync_mirror",
				SnapmirrorPolicyZAPITypeAsync: "async_mirror",
				SnapmirrorPolicyRESTTypeSync:  "sync",
				SnapmirrorPolicyRESTTypeAsync: "async",
			}
			for policyType, expectedValue := range expected {
				assert.Equal(t, expectedValue, string(policyType))
			}
		}},
		{"SliceTypes", func(t *testing.T) {
			// Test all slice type definitions
			volumes := Volumes{&Volume{Name: "vol1"}}
			luns := Luns{Lun{Name: "lun1"}}
			snapshots := Snapshots{Snapshot{Name: "snap1"}}
			qtrees := Qtrees{&Qtree{Name: "qtree1"}}
			quotas := QuotaEntries{&QuotaEntry{Target: "quota1"}}
			namespaces := NVMeNamespaces{&NVMeNamespace{Name: "ns1"}}
			volumeNames := VolumeNameList{"vol1", "vol2"}
			qtreeNames := QtreeNameList{"qtree1", "qtree2"}

			assert.Len(t, volumes, 1)
			assert.Len(t, luns, 1)
			assert.Len(t, snapshots, 1)
			assert.Len(t, qtrees, 1)
			assert.Len(t, quotas, 1)
			assert.Len(t, namespaces, 1)
			assert.Len(t, volumeNames, 2)
			assert.Len(t, qtreeNames, 2)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}

// TestStructValidation validates business logic and edge cases for all structs
func TestStructValidation(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{"VolumeLogic", func(t *testing.T) {
			encryptTrue, snapshotDirFalse := true, false
			vol := Volume{
				Aggregates:      []string{"aggr1", "aggr2"},
				Encrypt:         &encryptTrue,
				SnapshotDir:     &snapshotDirFalse,
				SnapshotReserve: 5,
			}
			require.NotNil(t, vol.Encrypt)
			assert.True(t, *vol.Encrypt)
			require.NotNil(t, vol.SnapshotDir)
			assert.False(t, *vol.SnapshotDir)
			assert.Len(t, vol.Aggregates, 2)
			assert.True(t, vol.SnapshotReserve >= 0 && vol.SnapshotReserve <= 100)
		}},
		{"SnapmirrorLogic", func(t *testing.T) {
			endTime := time.Now()
			sm := Snapmirror{
				State:              SnapmirrorStateSnapmirrored,
				RelationshipStatus: SnapmirrorStatusIdle,
				IsHealthy:          true,
				UnhealthyReason:    "",
				EndTransferTime:    &endTime,
			}
			assert.False(t, sm.State.IsUninitialized())
			assert.True(t, sm.RelationshipStatus.IsIdle())
			assert.Empty(t, sm.UnhealthyReason)
			require.NotNil(t, sm.EndTransferTime)
		}},
		{"LunMappingLogic", func(t *testing.T) {
			spaceReserved, spaceAllocated := true, false
			lun := Lun{
				LunMaps:        []LunMap{{IgroupName: "ig1", LunID: 0}, {IgroupName: "ig2", LunID: 1}},
				Mapped:         true,
				SpaceReserved:  &spaceReserved,
				SpaceAllocated: &spaceAllocated,
			}
			assert.Equal(t, len(lun.LunMaps) > 0, lun.Mapped)
			lunIDs := make(map[int]bool)
			for _, lunMap := range lun.LunMaps {
				assert.False(t, lunIDs[lunMap.LunID], "LUN IDs should be unique")
				lunIDs[lunMap.LunID] = true
				assert.NotEmpty(t, lunMap.IgroupName)
			}
			require.NotNil(t, lun.SpaceReserved)
			assert.True(t, *lun.SpaceReserved)
		}},
		{"QuotaLogic", func(t *testing.T) {
			quota := QuotaEntry{Target: "/vol/test/qtree1", DiskLimitBytes: 1073741824}
			assert.NotEmpty(t, quota.Target)
			assert.True(t, quota.DiskLimitBytes > 0)
			assert.True(t, strings.HasPrefix(quota.Target, "/"))
		}},
		{"EdgeCases", func(t *testing.T) {
			// Test empty structs and nil pointers
			var volume Volume
			var lun Lun
			assert.Empty(t, volume.Name)
			assert.Nil(t, volume.Encrypt)
			assert.Empty(t, lun.Name)

			// Test pointer field behavior
			encrypt := true
			volume.Encrypt = &encrypt
			require.NotNil(t, volume.Encrypt)
			assert.True(t, *volume.Encrypt)

			// Test slice boundaries
			lun.LunMaps = []LunMap{}
			assert.NotNil(t, lun.LunMaps)
			assert.Len(t, lun.LunMaps, 0)
		}},
		{"TypeCompatibility", func(t *testing.T) {
			// Test reflection and string conversion
			volume := Volume{Name: "test"}
			volumeType := reflect.TypeOf(volume)
			assert.Equal(t, "Volume", volumeType.Name())
			assert.Equal(t, reflect.Struct, volumeType.Kind())

			nameField, exists := volumeType.FieldByName("Name")
			assert.True(t, exists)
			assert.Equal(t, "string", nameField.Type.String())

			// Test enum string conversion
			state := SnapmirrorStateUninitialized
			assert.Equal(t, "uninitialized", string(state))
			customState := SnapmirrorState("custom_state")
			assert.Equal(t, "custom_state", string(customState))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}
