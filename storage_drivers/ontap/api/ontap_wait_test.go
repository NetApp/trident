// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	terr "github.com/netapp/trident/utils/errors"
)

type seqLunGetter struct {
	responses []struct {
		lun *Lun
		err error
	}
	i int
}

func (s *seqLunGetter) LunGetByName(ctx context.Context, name string) (*Lun, error) {
	if s.i >= len(s.responses) {
		return nil, terr.NotFoundError("stub exhausted")
	}
	r := s.responses[s.i]
	s.i++
	return r.lun, r.err
}

func TestWaitForLunToExist_RetriesNotFoundThenSucceeds(t *testing.T) {
	g := &seqLunGetter{
		responses: []struct {
			lun *Lun
			err error
		}{
			{nil, terr.NotFoundError("not found")},
			{&Lun{Name: "/vol/v/lun0", Size: "1073741824"}, nil},
		},
	}
	lun, err := WaitForLunToExist(context.Background(), g, "/vol/v/lun0")
	assert.NoError(t, err)
	assert.NotNil(t, lun)
	assert.Equal(t, "/vol/v/lun0", lun.Name)
	assert.Equal(t, 2, g.i)
}

func TestWaitForLunToExist_NonNotFoundFailsImmediately(t *testing.T) {
	g := &seqLunGetter{
		responses: []struct {
			lun *Lun
			err error
		}{
			{nil, errors.New("rpc failed")},
		},
	}
	lun, err := WaitForLunToExist(context.Background(), g, "/vol/v/lun0")
	assert.Error(t, err)
	assert.Nil(t, lun)
	assert.Equal(t, 1, g.i)
}

func TestWaitForLunToExist_NilLunWithoutErrorFailsImmediately(t *testing.T) {
	g := &seqLunGetter{
		responses: []struct {
			lun *Lun
			err error
		}{
			{nil, nil},
		},
	}
	lun, err := WaitForLunToExist(context.Background(), g, "/vol/v/lun0")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "unexpected empty result looking up LUN")
	assert.NotContains(t, err.Error(), "timed out")
	assert.Nil(t, lun)
	assert.Equal(t, 1, g.i)
}

func TestWaitForLunToExist_ContextTimeout(t *testing.T) {
	g := &seqLunGetter{
		responses: []struct {
			lun *Lun
			err error
		}{
			{nil, terr.NotFoundError("not found")},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	lun, err := WaitForLunToExist(ctx, g, "/vol/v/lun0")
	assert.Error(t, err)
	assert.Nil(t, lun)
	assert.GreaterOrEqual(t, g.i, 1)
}

func TestWaitForLunToExist_ContextCancelledBeforeRetry(t *testing.T) {
	g := &seqLunGetter{
		responses: []struct {
			lun *Lun
			err error
		}{
			{nil, terr.NotFoundError("not found")},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lun, err := WaitForLunToExist(ctx, g, "/vol/v/lun0")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "interrupted")
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, lun)
}

type seqVolumeExistenceChecker struct {
	responses []struct {
		exists bool
		err    error
	}
	i int
}

func (s *seqVolumeExistenceChecker) VolumeExists(ctx context.Context, name string) (bool, error) {
	if s.i >= len(s.responses) {
		return s.responses[len(s.responses)-1].exists, s.responses[len(s.responses)-1].err
	}
	r := s.responses[s.i]
	s.i++
	return r.exists, r.err
}

func TestWaitForVolumeToBeDeleted_RetriesUntilVolumeIsGone(t *testing.T) {
	c := &seqVolumeExistenceChecker{
		responses: []struct {
			exists bool
			err    error
		}{
			{true, nil},
			{true, nil},
			{false, nil},
		},
	}
	err := WaitForVolumeToBeDeleted(context.Background(), c, "vol1")
	assert.NoError(t, err)
	assert.Equal(t, 3, c.i)
}

type styleAwareVolumeExistenceChecker struct {
	flexvolChecks   int
	flexgroupChecks int
}

func (c *styleAwareVolumeExistenceChecker) VolumeExists(context.Context, string) (bool, error) {
	c.flexvolChecks++
	return false, nil
}

func (c *styleAwareVolumeExistenceChecker) FlexgroupExists(context.Context, string) (bool, error) {
	c.flexgroupChecks++
	return false, nil
}

func TestWaitForFlexgroupToBeDeleted_UsesFlexgroupExistence(t *testing.T) {
	c := &styleAwareVolumeExistenceChecker{}

	err := WaitForFlexgroupToBeDeleted(context.Background(), c, "fg1")

	assert.NoError(t, err)
	assert.Zero(t, c.flexvolChecks)
	assert.Equal(t, 1, c.flexgroupChecks)
}

func TestWaitForVolumeToBeDeleted_NotFoundCountsAsDeleted(t *testing.T) {
	c := &seqVolumeExistenceChecker{
		responses: []struct {
			exists bool
			err    error
		}{
			{false, terr.NotFoundError("volume not found")},
		},
	}
	err := WaitForVolumeToBeDeleted(context.Background(), c, "vol1")
	assert.NoError(t, err)
	assert.Equal(t, 1, c.i)
}

func TestWaitForVolumeToBeDeleted_TimesOutWhileVolumeExists(t *testing.T) {
	c := &seqVolumeExistenceChecker{
		responses: []struct {
			exists bool
			err    error
		}{
			{true, nil},
		},
	}
	err := WaitForVolumeToBeDeleted(context.Background(), c, "vol1")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "timed out waiting for volume vol1 to be deleted")
}

func TestWaitForVolumeToBeDeleted_RetriesReadFailures(t *testing.T) {
	c := &seqVolumeExistenceChecker{
		responses: []struct {
			exists bool
			err    error
		}{
			{false, errors.New("rpc failed")},
			{false, nil},
		},
	}
	err := WaitForVolumeToBeDeleted(context.Background(), c, "vol1")
	assert.NoError(t, err)
	assert.Equal(t, 2, c.i)
}

func TestWaitForVolumeToBeDeleted_ContextCancelled(t *testing.T) {
	c := &seqVolumeExistenceChecker{
		responses: []struct {
			exists bool
			err    error
		}{
			{true, nil},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := WaitForVolumeToBeDeleted(ctx, c, "vol1")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "interrupted")
	assert.ErrorIs(t, err, context.Canceled)
}

type seqNVMeNamespaceGetter struct {
	responses []struct {
		ns  *NVMeNamespace
		err error
	}
	i int
}

func (s *seqNVMeNamespaceGetter) NVMeNamespaceGetByName(
	ctx context.Context, name string,
) (*NVMeNamespace, error) {
	if s.i >= len(s.responses) {
		return nil, terr.NotFoundError("stub exhausted")
	}
	r := s.responses[s.i]
	s.i++
	return r.ns, r.err
}

type seqNVMeNamespaceSizeGetter struct {
	responses []struct {
		size int
		err  error
	}
	i int
}

func (s *seqNVMeNamespaceSizeGetter) NVMeNamespaceGetSize(
	ctx context.Context, name string,
) (int, error) {
	if s.i >= len(s.responses) {
		return 0, terr.NotFoundError("stub exhausted")
	}
	r := s.responses[s.i]
	s.i++
	return r.size, r.err
}

func TestWaitForNVMeNamespaceToExist_RetriesNotFoundThenSucceeds(t *testing.T) {
	g := &seqNVMeNamespaceGetter{
		responses: []struct {
			ns  *NVMeNamespace
			err error
		}{
			{nil, terr.NotFoundError("not found")},
			{&NVMeNamespace{Name: "/vol/flex/namespace0", UUID: "uuid-1"}, nil},
		},
	}
	ns, err := WaitForNVMeNamespaceToExist(context.Background(), g, "/vol/flex/namespace0", false)
	assert.NoError(t, err)
	assert.NotNil(t, ns)
	assert.Equal(t, 2, g.i)
}

func TestWaitForNVMeNamespaceToExist_RetriesEmptyResultWhenEnabled(t *testing.T) {
	g := &seqNVMeNamespaceGetter{
		responses: []struct {
			ns  *NVMeNamespace
			err error
		}{
			{nil, nil},
			{&NVMeNamespace{Name: "/vol/flex/namespace0", UUID: "uuid-1"}, nil},
		},
	}
	ns, err := WaitForNVMeNamespaceToExist(context.Background(), g, "/vol/flex/namespace0", true)
	assert.NoError(t, err)
	assert.NotNil(t, ns)
	assert.Equal(t, 2, g.i)
}

func TestWaitForNVMeNamespaceToExist_NilWithoutErrorFailsImmediately(t *testing.T) {
	g := &seqNVMeNamespaceGetter{
		responses: []struct {
			ns  *NVMeNamespace
			err error
		}{
			{nil, nil},
		},
	}
	ns, err := WaitForNVMeNamespaceToExist(context.Background(), g, "/vol/flex/namespace0", false)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "unexpected empty result looking up NVMe namespace")
	assert.NotContains(t, err.Error(), "timed out")
	assert.Nil(t, ns)
	assert.Equal(t, 1, g.i)
}

func TestWaitForNVMeNamespaceToExist_NonNotFoundFailsImmediately(t *testing.T) {
	g := &seqNVMeNamespaceGetter{
		responses: []struct {
			ns  *NVMeNamespace
			err error
		}{
			{nil, errors.New("permission denied")},
		},
	}
	ns, err := WaitForNVMeNamespaceToExist(context.Background(), g, "/vol/flex/namespace0", false)
	assert.Error(t, err)
	assert.Nil(t, ns)
	assert.Equal(t, 1, g.i)
}

func TestWaitForNVMeNamespaceSize_RetriesNotFoundThenSucceeds(t *testing.T) {
	g := &seqNVMeNamespaceSizeGetter{
		responses: []struct {
			size int
			err  error
		}{
			{0, terr.NotFoundError("not found")},
			{1024, nil},
		},
	}
	size, err := WaitForNVMeNamespaceSize(context.Background(), g, "/vol/flex/*")
	assert.NoError(t, err)
	assert.Equal(t, 1024, size)
	assert.Equal(t, 2, g.i)
}
