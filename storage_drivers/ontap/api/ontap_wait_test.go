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

func TestWaitForJunctionPath(t *testing.T) {
	readErr := errors.New("read failed")

	tests := []struct {
		name         string
		expectedPath string
		responses    []*Volume
		err          error
		errAttempts  int
		cancel       bool
		timeout      time.Duration
		wantPath     string
		wantAttempts int
		minAttempts  int
		wantErr      string
		wantErrIs    error
	}{
		{
			name:         "immediate exact match",
			expectedPath: "/pool",
			responses:    []*Volume{{Name: "pool", JunctionPath: "/pool"}},
			wantPath:     "/pool",
			wantAttempts: 1,
		},
		{
			name:         "accepts trailing slash on reported path",
			expectedPath: "/pool",
			responses:    []*Volume{{Name: "pool", JunctionPath: "/pool/"}},
			wantPath:     "/pool/",
			wantAttempts: 1,
		},
		{
			name:         "accepts trailing slash on expected path",
			expectedPath: "/pool/",
			responses:    []*Volume{{Name: "pool", JunctionPath: "/pool"}},
			wantPath:     "/pool",
			wantAttempts: 1,
		},
		{
			name:         "retries empty slash-only and stale paths",
			expectedPath: "/pool",
			responses: []*Volume{
				{Name: "pool"},
				{Name: "pool", JunctionPath: "/"},
				{Name: "pool", JunctionPath: "/stale"},
				{Name: "pool", JunctionPath: "/pool"},
			},
			wantPath:     "/pool",
			wantAttempts: 4,
		},
		{
			name:         "accepts any non-empty path",
			responses:    []*Volume{{Name: "pool", JunctionPath: "/custom/import"}},
			wantPath:     "/custom/import",
			wantAttempts: 1,
		},
		{
			name:         "fails after retry budget",
			expectedPath: "/pool",
			responses:    []*Volume{{Name: "pool"}},
			timeout:      50 * time.Millisecond,
			minAttempts:  1,
			wantErr:      "timed out waiting for volume pool junction path",
		},
		{
			name:         "transient read errors are retried",
			expectedPath: "/pool",
			err:          readErr,
			errAttempts:  2,
			responses:    []*Volume{{Name: "pool", JunctionPath: "/pool"}},
			wantPath:     "/pool",
			wantAttempts: 3,
		},
		{
			name:         "transient read error fails after retry budget",
			expectedPath: "/pool",
			err:          readErr,
			errAttempts:  100,
			timeout:      50 * time.Millisecond,
			minAttempts:  1,
			wantErr:      "timed out waiting for volume pool junction path",
			wantErrIs:    readErr,
		},
		{
			name:         "not found read error fails immediately",
			expectedPath: "/pool",
			err:          terr.NotFoundError("pool not found"),
			errAttempts:  1,
			wantAttempts: 1,
			wantErr:      "get volume pool while waiting for junction path",
		},
		{
			name:         "canceled read fails immediately",
			expectedPath: "/pool",
			err:          context.Canceled,
			errAttempts:  1,
			wantAttempts: 1,
			wantErr:      "waiting for volume pool junction path interrupted",
			wantErrIs:    context.Canceled,
		},
		{
			name:         "nil volume fails immediately",
			expectedPath: "/pool",
			responses:    []*Volume{nil},
			wantAttempts: 1,
			wantErr:      "volume pool was not found",
		},
		{
			name:         "canceled context interrupts retry",
			expectedPath: "/pool",
			responses:    []*Volume{{Name: "pool"}},
			cancel:       true,
			wantAttempts: 1,
			wantErr:      "waiting for volume pool junction path interrupted",
			wantErrIs:    context.Canceled,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				ctx    context.Context
				cancel context.CancelFunc
			)
			switch {
			case test.cancel:
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			case test.timeout > 0:
				ctx, cancel = context.WithTimeout(context.Background(), test.timeout)
				defer cancel()
			default:
				ctx, cancel = context.WithCancel(context.Background())
				defer cancel()
			}

			attempts := 0
			getter := func(context.Context, string) (*Volume, error) {
				attempts++
				if attempts <= test.errAttempts {
					return nil, test.err
				}
				index := min(attempts-test.errAttempts-1, len(test.responses)-1)
				return test.responses[index], nil
			}

			volume, err := WaitForJunctionPath(ctx, getter, "pool", test.expectedPath)

			if test.wantErr != "" {
				assert.ErrorContains(t, err, test.wantErr)
				assert.Nil(t, volume)
				if test.wantErrIs != nil {
					assert.ErrorIs(t, err, test.wantErrIs)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.wantPath, volume.JunctionPath)
			}
			if test.minAttempts > 0 {
				assert.GreaterOrEqual(t, attempts, test.minAttempts)
			} else {
				assert.Equal(t, test.wantAttempts, attempts)
			}
		})
	}
}

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
