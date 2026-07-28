// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/utils/limiter"
)

func TestInitializeLimiters_CreatesOneLimiterPerWorkflowAndProtocol(t *testing.T) {
	core := NewCore()

	require.NoError(t, core.initializeLimiters(context.Background()))

	expectedLimiters := []string{
		attachNFSVolumeKey, attachSMBVolumeKey, attachISCSIVolumeKey, attachFCPVolumeKey, attachNVMeVolumeKey,
		detachNFSVolumeKey, detachSMBVolumeKey, detachISCSIVolumeKey, detachFCPVolumeKey, detachNVMeVolumeKey,
		mountNFSVolumeKey, mountSMBVolumeKey, mountISCSIVolumeKey, mountFCPVolumeKey, mountNVMeVolumeKey,
		unmountVolumeKey, expandVolumeKey,
		graftISCSIAttachmentKey, pruneISCSIAttachmentKey,
	}

	for _, name := range expectedLimiters {
		assert.Contains(t, core.protocolLimiters, name)
		assert.NotNil(t, core.protocolLimiters[name])
	}
	assert.Len(t, core.protocolLimiters, len(expectedLimiters))
}

func TestAcquireLimiter_UnknownKeySkipsAdmissionControl(t *testing.T) {
	core, _ := newTestCore(t)
	require.NoError(t, core.initializeLimiters(context.Background()))

	release, err := core.acquireLimiter(context.Background(), "bogus-key")
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
}

func TestAcquireLimiter_NilLimiterMapSkipsAdmissionControl(t *testing.T) {
	core, _ := newTestCore(t)

	release, err := core.acquireLimiter(context.Background(), attachNFSVolumeKey)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
}

func TestAcquireLimiter_ExhaustedLimiterBlocksUntilReleased(t *testing.T) {
	core, _ := newTestCore(t)
	ctx := context.Background()

	l, err := limiter.New(ctx, "test-key", limiter.TypeSemaphoreN, limiter.WithSemaphoreNSize(ctx, 1))
	require.NoError(t, err)
	core.protocolLimiters = map[string]limiter.Limiter{"test-key": l}

	release1, err := core.acquireLimiter(ctx, "test-key")
	require.NoError(t, err)

	type result struct {
		release func()
		err     error
	}
	done := make(chan result, 1)
	go func() {
		release2, err := core.acquireLimiter(ctx, "test-key")
		done <- result{release2, err}
	}()

	select {
	case <-done:
		t.Fatal("second acquireLimiter returned before first was released")
	case <-time.After(50 * time.Millisecond):
	}

	release1()

	select {
	case res := <-done:
		require.NoError(t, res.err)
		res.release()
	case <-time.After(2 * time.Second):
		t.Fatal("second acquireLimiter did not unblock after release")
	}
}

func TestAcquireLimiter_ContextCanceled_ReturnsError(t *testing.T) {
	core, _ := newTestCore(t)
	ctx := context.Background()

	l, err := limiter.New(ctx, "test-key", limiter.TypeSemaphoreN, limiter.WithSemaphoreNSize(ctx, 1))
	require.NoError(t, err)
	core.protocolLimiters = map[string]limiter.Limiter{"test-key": l}

	release, err := core.acquireLimiter(ctx, "test-key")
	require.NoError(t, err)
	defer release()

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	_, err = core.acquireLimiter(canceledCtx, "test-key")
	assert.Error(t, err)
}
