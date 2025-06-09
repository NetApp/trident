// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroupSnapshotConfig_Validate(t *testing.T) {
	tt := map[string]struct {
		modifyCfg func(*GroupSnapshotConfig)
		assertErr assert.ErrorAssertionFunc
	}{
		"with valid group snapshot config": {
			modifyCfg: func(cfg *GroupSnapshotConfig) {
				cfg.Version = "1.0"
				cfg.Name = "groupsnapshot-12345"
				cfg.InternalName = "groupsnapshot-12345"
				cfg.VolumeNames = []string{"volume1", "volume2"}
			},
			assertErr: assert.NoError,
		},
		"with empty group snapshot name": {
			modifyCfg: func(cfg *GroupSnapshotConfig) {
				cfg.Name = ""
			},
			assertErr: assert.Error,
		},
		"with empty group snapshot volume names": {
			modifyCfg: func(cfg *GroupSnapshotConfig) {
				cfg.VolumeNames = nil
			},
			assertErr: assert.Error,
		},
	}

	for name, tc := range tt {
		t.Run(name, func(t *testing.T) {
			// Create a dummy config with all values.
			cfg := &GroupSnapshotConfig{
				Version:      "1.0",
				Name:         "groupsnapshot-12345",
				InternalName: "internal_groupsnapshot-12345",
				VolumeNames:  []string{"volume1", "volume2"},
			}

			// Mutate the cfg to manipulate the test case.
			tc.modifyCfg(cfg)
			tc.assertErr(t, cfg.Validate())
		})
	}
}

func TestGroupSnapshotConfig_ConstructClone(t *testing.T) {
	tt := map[string]struct {
		modifyCfg func(*GroupSnapshotConfig) assert.ComparisonAssertionFunc
		assertPtr assert.ComparisonAssertionFunc
	}{
		"with valid group snapshot config": {
			modifyCfg: func(cfg *GroupSnapshotConfig) assert.ComparisonAssertionFunc {
				// Ensure that if no modification is done at a value level, the clone is equal to the original.
				return assert.Equal
			},
			assertPtr: assert.NotSame,
		},
		"with empty group snapshot name": {
			modifyCfg: func(cfg *GroupSnapshotConfig) assert.ComparisonAssertionFunc {
				cfg.VolumeNames = nil
				return assert.NotEqual
			},
			assertPtr: assert.NotSame,
		},
		"with empty group snapshot volume names": {
			modifyCfg: func(cfg *GroupSnapshotConfig) assert.ComparisonAssertionFunc {
				cfg.VolumeNames = nil
				return assert.NotEqual
			},
			assertPtr: assert.NotSame,
		},
	}

	for name, tc := range tt {
		t.Run(name, func(t *testing.T) {
			// Create a dummy config with all values.
			cfg := &GroupSnapshotConfig{
				Version:      "1.0",
				Name:         "groupsnapshot-12345",
				InternalName: "internal_groupsnapshot-12345",
				VolumeNames:  []string{"volume1", "volume2"},
			}

			// Construct the clone
			clone := cfg.ConstructClone()
			tc.assertPtr(t, cfg, clone)
			tc.assertPtr(t, &cfg.VolumeNames, &clone.VolumeNames)

			// Mutate the cfg to manipulate the test case and get comparison func.
			assertCmp := tc.modifyCfg(cfg)
			if assertCmp == nil {
				t.Fatalf("failed to get comparison function for test case %s", name)
			}

			assertCmp(t, cfg, clone)
		})
	}
}

func TestGroupSnapshot(t *testing.T) {
	// Create a dummy config with all values.
	sourceConfig := &GroupSnapshotConfig{
		Version:      "1.0",
		Name:         "groupsnapshot-12345",
		InternalName: "internal_groupsnapshot-12345",
		VolumeNames:  []string{"volume1", "volume2"},
	}

	// Create a dummy group snapshot with the config.
	groupsnapshot := &GroupSnapshot{
		GroupSnapshotConfig: sourceConfig, // Set the config initially.
		SnapshotIDs: []string{
			"volume1" + "/" + "snapshot-12345",
			"volume2" + "/" + "snapshot-12345",
		},
	}
	storedConfig := groupsnapshot.Config()

	assert.NotNil(t, storedConfig)
	assert.Equal(t, sourceConfig, storedConfig)
	assert.NotSame(t, sourceConfig, storedConfig)

	// Modify the original config, and ensure the returned config from Config() is not modified.
	sourceConfig.Name = "modified-groupsnapshot-12345"
	assert.NotEqual(t, sourceConfig, storedConfig)

	// Set the config on the group snapshot and ensure it picks up the changes.
	groupsnapshot.SetConfig(sourceConfig)
	storedConfig = groupsnapshot.Config()
	assert.Equal(t, sourceConfig, storedConfig)

	// Construct a clone of the group snapshot
	clone := groupsnapshot.ConstructClone()
	assert.Equal(t, groupsnapshot, clone)
	assert.NotSame(t, groupsnapshot, clone)

	// Ensure the clone has the same values but different points as the original
	groupSnaps := groupsnapshot.GetSnapshotIDs()
	cloneSnaps := clone.GetSnapshotIDs()
	assert.Equal(t, clone.SnapshotIDs, groupsnapshot.SnapshotIDs)
	assert.Equal(t, clone.GetSnapshotIDs(), groupsnapshot.GetSnapshotIDs())
	assert.Equal(t, groupSnaps, cloneSnaps)
	assert.NotSame(t, &groupSnaps, &cloneSnaps)

	// Ensure created is equivalent.
	assert.Equal(t, groupsnapshot.GetCreated(), clone.GetCreated())

	// Get the external representations of the snapshots and ensure they are different.
	externalSnaps := groupsnapshot.GetSnapshotIDs()
	clonedSnapshots := clone.GetSnapshotIDs()
	assert.NotSame(t, &externalSnaps, &clonedSnapshots)

	persistent := groupsnapshot.ConstructPersistent()
	assert.NotNil(t, persistent)
	assert.Equal(t, groupsnapshot.ID(), persistent.ID())
	assert.Equal(t, groupsnapshot.Config(), persistent.Config())
}

func TestGroupSnapshotPersistent(t *testing.T) {
	// Create a dummy config with all values.
	sourceConfig := &GroupSnapshotConfig{
		Version:      "1.0",
		Name:         "groupsnapshot-12345",
		InternalName: "internal_groupsnapshot-12345",
		VolumeNames:  []string{"volume1", "volume2"},
	}

	// Create a dummy group snapshot with the config.
	groupsnapshot := &GroupSnapshot{
		GroupSnapshotConfig: sourceConfig, // Set the config initially.
		SnapshotIDs: []string{
			"volume1" + "/" + "snapshot-12345",
			"volume2" + "/" + "snapshot-12345",
		},
	}

	persistent := groupsnapshot.ConstructPersistent()
	assert.NotNil(t, persistent.Config())
	assert.NotSame(t, groupsnapshot, persistent)
	assert.Equal(t, groupsnapshot.ID(), persistent.ID())
	assert.Equal(t, groupsnapshot.Config(), persistent.Config())

	// Persistent group snapshots treat the snapshots as a list of IDs, not objects.
	// Ensure the IDs are equal but do are not the same memory.
	snapshotIDs := groupsnapshot.GetSnapshotIDs()
	persistentIDs := persistent.GetSnapshotIDs()
	assert.Equal(t, snapshotIDs, persistentIDs)
	assert.NotSame(t, &snapshotIDs, &persistentIDs)
}

func TestGroupSnapshotExternal(t *testing.T) {
	// Create a dummy config with all values.
	sourceConfig := &GroupSnapshotConfig{
		Version:      "1.0",
		Name:         "groupsnapshot-12345",
		InternalName: "internal_groupsnapshot-12345",
		VolumeNames:  []string{"volume1", "volume2"},
	}

	// Create a dummy group snapshot with the config.
	groupSnapshot := &GroupSnapshot{
		GroupSnapshotConfig: sourceConfig, // Set the config initially.
		SnapshotIDs: []string{
			"volume1" + "/" + "snapshot-12345",
			"volume2" + "/" + "snapshot-12345",
		},
	}

	external := groupSnapshot.ConstructExternal()
	assert.NotNil(t, external.Config())
	assert.NotSame(t, groupSnapshot, external)
	assert.Equal(t, groupSnapshot.ID(), external.ID())
	assert.Equal(t, groupSnapshot.Config(), external.Config())
	assert.Equal(t, groupSnapshot.GetCreated(), external.GetCreated())
}
