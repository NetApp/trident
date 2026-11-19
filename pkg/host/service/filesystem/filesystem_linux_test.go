// Copyright 2026 NetApp, Inc. All Rights Reserved.

//go:build linux

package filesystem

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/pkg/host/adapter/filesystem"
	"github.com/netapp/trident/pkg/host/test"
)

func TestEnsureExpand_OverFakeHost(t *testing.T) {
	ctx := context.Background()
	fa := filesystem.NewFake(test.NewFakeHost())
	fa.StageFilesystemSize(testTmpMount, 100)
	s := New(WithMountAdapter(fa))

	before, after, err := s.EnsureExpand(ctx, nvmeSource, testStagingPath, "discard", Xfs)

	require.NoError(t, err)
	assert.Equal(t, uint64(100), before)
	assert.Equal(t, uint64(100), after, "the fake resize does not change the staged size")
	mounted, err := fa.IsMounted(ctx, "", testTmpMount)
	require.NoError(t, err)
	assert.False(t, mounted, "the temporary mount is removed")
}

func TestEnsureExpand_OverFakeHostMountsAtTemporaryPath(t *testing.T) {
	ctx := context.Background()
	fa := filesystem.NewFake(test.NewFakeHost())
	s := New(WithMountAdapter(fa))

	// No staged size: the first Size read fails with the mount still in place, which must then be removed.
	_, _, err := s.EnsureExpand(ctx, nvmeSource, testStagingPath, "", Ext4)

	require.ErrorContains(t, err, "fake filesystem size not staged")
	mounted, err := fa.IsMounted(ctx, "", testTmpMount)
	require.NoError(t, err)
	assert.False(t, mounted)
}
