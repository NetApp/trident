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
