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
	ctx := context.Background()
	lun, err := WaitForLunToExist(ctx, g, "/vol/v/lun0")
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
	ctx := context.Background()
	lun, err := WaitForLunToExist(ctx, g, "/vol/v/lun0")
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
	ctx := context.Background()
	lun, err := WaitForLunToExist(ctx, g, "/vol/v/lun0")
	assert.Error(t, err)
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
	ctx := context.Background()
	ns, err := WaitForNVMeNamespaceToExist(ctx, g, "/vol/flex/namespace0", false)
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
	ctx := context.Background()
	ns, err := WaitForNVMeNamespaceToExist(ctx, g, "/vol/flex/namespace0", true)
	assert.NoError(t, err)
	assert.NotNil(t, ns)
	assert.Equal(t, 2, g.i)
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
	ctx := context.Background()
	ns, err := WaitForNVMeNamespaceToExist(ctx, g, "/vol/flex/namespace0", false)
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
	ctx := context.Background()
	size, err := WaitForNVMeNamespaceSize(ctx, g, "/vol/flex/*")
	assert.NoError(t, err)
	assert.Equal(t, 1024, size)
	assert.Equal(t, 2, g.i)
}
