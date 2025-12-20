// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

// helper to build a context with common labels
func makeIncomingCtx() context.Context {
	return NewContextBuilder(context.Background()).
		WithClient(ContextRequestClientTridentCLI).
		WithMethod("List").
		BuildContext()
}

func makeOutgoingCtx() context.Context {
	return NewContextBuilder(context.Background()).
		WithTarget(ContextRequestTargetONTAP).
		WithMethod("GET").
		BuildContext()
}

func TestIncomingAPIRequestInFlightTelemeter_IncrementsAndDecrementsOnce(t *testing.T) {
	// Arrange
	ctx := makeIncomingCtx()
	client := getContextClient(ctx)
	method := getContextMethod(ctx)
	g := incomingAPIRequestsInFlight.WithLabelValues(client, method)
	before := testutil.ToFloat64(g)

	// Act
	rec := IncomingAPIRequestInFlightTelemeter(ctx)
	afterInc := testutil.ToFloat64(g)

	// Assert incremented by 1
	assert.Equal(t, before+1, afterInc)

	// Act: call recorder multiple times; due to once.Do, it should only decrement once
	var noErr error
	rec(&noErr)
	rec(&noErr)
	afterDec := testutil.ToFloat64(g)

	// Assert decremented by 1 overall (back to original value)
	assert.Equal(t, before, afterDec)
}

func TestIncomingAPIRequestDurationTelemeter_RecordsOnce_WithErrorAndCancellation(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(makeIncomingCtx())
	rec := IncomingAPIRequestDurationTelemeter(ctx)
	// give some time so duration is non-zero
	time.Sleep(5 * time.Millisecond)
	// simulate operation error and cancellation
	err := assert.AnError
	cancel()

	// Act
	before := testutil.CollectAndCount(incomingAPIRequestDurationSeconds)
	rec(&err)
	rec(&err)
	after := testutil.CollectAndCount(incomingAPIRequestDurationSeconds)

	// Assert: observed exactly once
	assert.Equal(t, before+1, after)
}

func TestOutgoingAPIRequestDurationTelemeter_RecordsOnce_WithDeadlineExceeded(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithTimeout(makeOutgoingCtx(), 1*time.Nanosecond)
	defer cancel()
	// Let the deadline pass
	time.Sleep(1 * time.Millisecond)
	rec := OutgoingAPIRequestDurationTelemeter(ctx)
	// give some time
	time.Sleep(5 * time.Millisecond)

	// Act
	before := testutil.CollectAndCount(outgoingAPIRequestDurationSeconds)
	rec(nil)
	rec(nil)
	after := testutil.CollectAndCount(outgoingAPIRequestDurationSeconds)

	// Assert
	assert.Equal(t, before+1, after)
}

func TestOutgoingAPIRequestInFlightTelemeter_IncrementsAndDecrementsOnce(t *testing.T) {
	// Arrange
	ctx := makeOutgoingCtx()
	target := getContextTarget(ctx)
	address := getContextAddress(ctx)
	method := getContextMethod(ctx)
	g := outgoingAPIRequestsInFlight.WithLabelValues(target, address, method)
	before := testutil.ToFloat64(g)

	// Act
	rec := OutgoingAPIRequestInFlightTelemeter(ctx)
	afterInc := testutil.ToFloat64(g)
	assert.Equal(t, before+1, afterInc)

	// Act: call recorder multiple times; only one decrement
	var noErr error
	rec(&noErr)
	rec(&noErr)
	afterDec := testutil.ToFloat64(g)
	assert.Equal(t, before, afterDec)
}

func TestOutgoingAPIRequestLimitedDurationTelemeter(t *testing.T) {
	tests := map[string]struct {
		injectLatency time.Duration
	}{
		"with known duration": {
			injectLatency: 25 * time.Millisecond,
		},
		"with measured duration": {
			injectLatency: 25 * time.Millisecond,
		},
		"with zero duration": {
			injectLatency: 0,
		},
		"with negative duration": {
			injectLatency: -10 * time.Millisecond,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Arrange context with labels
			ctx := context.Background()
			ctx = setContextTarget(ctx, ContextRequestTargetONTAP)
			ctx = setContextAddress(ctx, "10.0.0.2")
			ctx = setContextMethod(ctx, "POST-"+name)
			ctx = setContextDuration(ctx, tc.injectLatency)

			// Baseline
			before := testutil.CollectAndCount(outgoingAPIRequestLimitedDurationSeconds)

			// Act: build recorder and simulate a limited event
			rec := OutgoingAPIRequestLimitedDurationTelemeter(ctx)
			err := errors.TooManyRequestsError("throttled")
			rec(&err)
			after := testutil.CollectAndCount(outgoingAPIRequestLimitedDurationSeconds)

			// If the injected latency is zero or negative, no metrics should be recorded
			if tc.injectLatency <= 0 {
				assert.Equal(t, before, after)
				return
			}
			// Assert: one observation recorded
			assert.Equal(t, before+1, after)
		})
	}
}

func TestIncomingTelemeters(t *testing.T) {
	// Build test table
	tests := map[string]struct {
		setupCtx func() context.Context
		setErr   func() *error
	}{
		"success": {
			setupCtx: func() context.Context { return makeIncomingCtx() },
			setErr:   func() *error { return nil },
		},
		"failure": {
			setupCtx: func() context.Context { return makeIncomingCtx() },
			setErr:   func() *error { e := assert.AnError; return &e },
		},
		"canceled": {
			setupCtx: func() context.Context {
				ctx, cancel := context.WithCancel(makeIncomingCtx())
				// cancel before record
				cancel()
				return ctx
			},
			setErr: func() *error { return nil },
		},
		"deadline_exceeded": {
			setupCtx: func() context.Context {
				ctx, cancel := context.WithTimeout(makeIncomingCtx(), 1*time.Nanosecond)
				defer cancel()
				// let deadline pass
				time.Sleep(1 * time.Millisecond)
				return ctx
			},
			setErr: func() *error { return nil },
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := tc.setupCtx()
			// Ensure unique label set per test to avoid collisions with prior series
			ctx = setContextMethod(ctx, "List-"+name)
			// Labels for specific gauge vector
			caller := getContextClient(ctx)
			method := getContextMethod(ctx)
			g := incomingAPIRequestsInFlight.WithLabelValues(caller, method)

			// Baselines
			gBefore := testutil.ToFloat64(g)
			// For histogram, count total metrics; we only verify it increments by one per record
			hBefore := testutil.CollectAndCount(incomingAPIRequestDurationSeconds)

			// Compose telemeters
			telemeters := []Telemeter{
				IncomingAPIRequestInFlightTelemeter,
				IncomingAPIRequestDurationTelemeter,
			}
			// Create recorders and perform the in-flight increment (happens on creation)
			recorders := make([]Recorder, 0, len(telemeters))
			for _, tele := range telemeters {
				recorders = append(recorders, tele(ctx))
			}

			// Verify in-flight increment occurred
			gAfterInc := testutil.ToFloat64(g)
			assert.Equal(t, gBefore+1, gAfterInc)

			// Record once for each telemeter with configured error
			errPtr := tc.setErr()
			for _, rec := range recorders {
				rec(errPtr)
			}

			// Gauge should be back to baseline
			gAfterDec := testutil.ToFloat64(g)
			assert.Equal(t, gBefore, gAfterDec)

			// Histogram should have one more observation
			hAfter := testutil.CollectAndCount(incomingAPIRequestDurationSeconds)
			assert.Equal(t, hBefore+1, hAfter)
		})
	}
}

func TestIncomingAPIRequestDurationTelemeter_UsesContextDurationWhenSet(t *testing.T) {
	tests := map[string]struct {
		duration time.Duration
		sleep    time.Duration
	}{
		"with known duration": {
			duration: 40 * time.Millisecond,
			sleep:    5 * time.Millisecond,
		},
		"with short known duration": {
			duration: 2 * time.Millisecond,
			sleep:    10 * time.Millisecond,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Arrange: context with client/method and an explicit duration
			ctx := NewContextBuilder(context.Background()).
				WithClient(ContextRequestClientTridentCLI).
				WithMethod("POST-" + name).
				WithDuration(tc.duration).
				BuildContext()

			// Build recorder and add a small sleep to simulate runtime
			rec := IncomingAPIRequestDurationTelemeter(ctx)
			time.Sleep(tc.sleep)

			// Act
			before := testutil.CollectAndCount(incomingAPIRequestDurationSeconds)
			rec(nil)
			after := testutil.CollectAndCount(incomingAPIRequestDurationSeconds)

			// Assert: exactly one observation recorded
			assert.Equal(t, before+1, after)
		})
	}
}

func TestOutgoingAPIRequestDurationTelemeter_UsesContextDurationWhenSet(t *testing.T) {
	tests := map[string]struct {
		duration time.Duration
		sleep    time.Duration
	}{
		"with known duration": {
			duration: 30 * time.Millisecond,
			sleep:    5 * time.Millisecond,
		},
		"with short known duration": {
			duration: 1 * time.Millisecond,
			sleep:    10 * time.Millisecond,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Arrange: context with target/address/method and an explicit duration
			ctx := NewContextBuilder(context.Background()).
				WithTarget(ContextRequestTargetKubernetes).
				WithAddress("10.43.0.1:443").
				WithMethod("GET-" + name).
				WithDuration(tc.duration).
				BuildContext()

			// Build recorder and add a small sleep to simulate runtime
			rec := OutgoingAPIRequestDurationTelemeter(ctx)
			time.Sleep(tc.sleep)

			// Act
			before := testutil.CollectAndCount(outgoingAPIRequestDurationSeconds)
			rec(nil)
			after := testutil.CollectAndCount(outgoingAPIRequestDurationSeconds)

			// Assert: exactly one observation recorded
			assert.Equal(t, before+1, after)
		})
	}
}

func TestOutgoingAPIRequestRetryTotalTelemeter(t *testing.T) {
	tests := map[string]struct {
		errFunc         func() error
		expectIncrement bool
	}{
		"with MustRetryError": {
			errFunc: func() error {
				return errors.MustRetryError("retry required")
			},
			expectIncrement: true,
		},
		"with non-retry error": {
			errFunc: func() error {
				return assert.AnError
			},
			expectIncrement: false,
		},
		"with nil error": {
			errFunc: func() error {
				return nil
			},
			expectIncrement: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Arrange context with unique method to avoid label collision
			ctx := NewContextBuilder(context.Background()).
				WithTarget(ContextRequestTargetKubernetes).
				WithAddress("10.43.0.1:443").
				WithMethod("POST-" + name).
				BuildContext()

			// Baseline
			before := testutil.CollectAndCount(outgoingAPIRequestRetryTotal)

			// Act: build recorder and invoke with the configured error
			rec := OutgoingAPIRequestRetryTotalTelemeter(ctx)
			err := tc.errFunc()
			rec(&err)

			// Assert: counter incremented only if MustRetryError
			after := testutil.CollectAndCount(outgoingAPIRequestRetryTotal)
			if tc.expectIncrement {
				assert.Greater(t, after, before)
			} else {
				assert.Equal(t, before, after)
			}
		})
	}
}

func TestOutgoingAPIRequestRetryTotalTelemeter_UnsetLabels(t *testing.T) {
	// Arrange: context with missing required labels (target="unknown", method="unknown")
	rec := NewContextBuilder(context.Background()).
		WithTelemetry(OutgoingAPIRequestRetryTotalTelemeter).
		BuildTelemetry()

	// Baseline
	before := testutil.CollectAndCount(outgoingAPIRequestRetryTotal)

	// Act: telemeter should return a no-op recorder due to invalid labels
	err := errors.MustRetryError("retry required")
	rec(&err)

	// Assert: Should increment with default labels.
	after := testutil.CollectAndCount(outgoingAPIRequestRetryTotal)
	assert.Greater(t, after, before)
}

func TestOutgoingAPIRequestLimitedDurationTelemeter_NonThrottleError(t *testing.T) {
	// Arrange
	ctx := NewContextBuilder(context.Background()).
		WithTarget(ContextRequestTargetKubernetes).
		WithAddress("10.43.0.1:443").
		WithMethod("GET").
		BuildContext()

	// Baseline
	before := testutil.CollectAndCount(outgoingAPIRequestLimitedDurationSeconds)

	// Act: record with a non-throttle error
	rec := OutgoingAPIRequestLimitedDurationTelemeter(ctx)
	err := assert.AnError
	rec(&err)

	// Assert: no observation because error wasn't a throttle/retry error
	after := testutil.CollectAndCount(outgoingAPIRequestLimitedDurationSeconds)
	assert.Equal(t, before, after)
}

func TestOutgoingAPIRequestLimitedDurationTelemeter_BothErrorTypes(t *testing.T) {
	tests := map[string]struct {
		errFunc           func() error
		expectObservation bool
	}{
		"with TooManyRequestsError": {
			errFunc: func() error {
				return errors.TooManyRequestsError("429 too many requests")
			},
			expectObservation: true,
		},
		"with generic error": {
			errFunc: func() error {
				return assert.AnError
			},
			expectObservation: false,
		},
		"with nil error": {
			errFunc: func() error {
				return nil
			},
			expectObservation: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Arrange
			ctx := NewContextBuilder(context.Background()).
				WithTarget(ContextRequestTargetKubernetes).
				WithAddress("10.43.0.1:443").
				WithMethod("GET-" + name).
				WithDuration(10 * time.Millisecond).
				BuildContext()

			// Baseline
			before := testutil.CollectAndCount(outgoingAPIRequestLimitedDurationSeconds)

			// Act
			rec := OutgoingAPIRequestLimitedDurationTelemeter(ctx)
			err := tc.errFunc()
			rec(&err)

			// Assert
			after := testutil.CollectAndCount(outgoingAPIRequestLimitedDurationSeconds)
			if tc.expectObservation {
				assert.Equal(t, before+1, after)
			} else {
				assert.Equal(t, before, after)
			}
		})
	}
}

func TestOutgoingTelemeters(t *testing.T) {
	tests := map[string]struct {
		setupCtx func() context.Context
		setErr   func() *error
	}{
		"success": {
			setupCtx: func() context.Context { return makeOutgoingCtx() },
			setErr:   func() *error { return nil },
		},
		"failure": {
			setupCtx: func() context.Context { return makeOutgoingCtx() },
			setErr:   func() *error { e := assert.AnError; return &e },
		},
		"canceled": {
			setupCtx: func() context.Context {
				ctx, cancel := context.WithCancel(makeOutgoingCtx())
				cancel()
				return ctx
			},
			setErr: func() *error { return nil },
		},
		"deadline_exceeded": {
			setupCtx: func() context.Context {
				ctx, cancel := context.WithTimeout(makeOutgoingCtx(), 1*time.Nanosecond)
				defer cancel()
				time.Sleep(1 * time.Millisecond)
				return ctx
			},
			setErr: func() *error { return nil },
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := tc.setupCtx()
			// Unique method per test to avoid label collision
			ctx = setContextMethod(ctx, "GET-"+name)
			target := getContextTarget(ctx)
			address := getContextAddress(ctx)
			method := getContextMethod(ctx)
			g := outgoingAPIRequestsInFlight.WithLabelValues(target, address, method)

			// Baselines
			gBefore := testutil.ToFloat64(g)
			hBefore := testutil.CollectAndCount(outgoingAPIRequestDurationSeconds)

			// Compose telemeters (excluding limited and retry since they need specific errors)
			telemeters := []Telemeter{
				OutgoingAPIRequestInFlightTelemeter,
				OutgoingAPIRequestDurationTelemeter,
			}
			recorders := make([]Recorder, 0, len(telemeters))
			for _, tele := range telemeters {
				recorders = append(recorders, tele(ctx))
			}

			// Verify in-flight increment
			gAfterInc := testutil.ToFloat64(g)
			assert.Equal(t, gBefore+1, gAfterInc)

			// Record once for each telemeter
			errPtr := tc.setErr()
			for _, rec := range recorders {
				rec(errPtr)
			}

			// Gauge back to baseline
			gAfterDec := testutil.ToFloat64(g)
			assert.Equal(t, gBefore, gAfterDec)

			// Histogram incremented by one
			hAfter := testutil.CollectAndCount(outgoingAPIRequestDurationSeconds)
			assert.Equal(t, hBefore+1, hAfter)
		})
	}
}
