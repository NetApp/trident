// Copyright 2026 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"

	tridentErrors "github.com/netapp/trident/utils/errors"
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

// readGauge returns the current value of a gauge for the given label values.
func readGauge(g *prometheus.GaugeVec, labels ...string) float64 {
	return testutil.ToFloat64(g.WithLabelValues(labels...))
}

// readCounter returns the current value of a counter for the given label values.
func readCounter(c *prometheus.CounterVec, labels ...string) float64 {
	return testutil.ToFloat64(c.WithLabelValues(labels...))
}

// readHistogramCount returns the number of observations recorded in a histogram for the given label values.
func readHistogramCount(h *prometheus.HistogramVec, labels ...string) uint64 {
	m := &dto.Metric{}
	if metric, ok := h.WithLabelValues(labels...).(prometheus.Metric); ok {
		_ = metric.Write(m)
	}
	return m.GetHistogram().GetSampleCount()
}

// uniqueLabel returns a label value that is unique to a test to prevent cross-test metric interference.
func uniqueLabel(t *testing.T, role string) string {
	t.Helper()
	return fmt.Sprintf("%s-%s", t.Name(), role)
}

// expiredCtx returns a context whose deadline has already passed.
func expiredCtx() (context.Context, context.CancelFunc) {
	return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
}

// ---- IncomingAPITelemeter ----

func TestIncomingAPITelemeter_InFlight_IncrementOnStart(t *testing.T) {
	client, method := uniqueLabel(t, "client"), uniqueLabel(t, "method")

	tel := IncomingAPITelemeter(client, method)

	assert.Equal(t, float64(0), readGauge(incomingAPIRequestsInFlight, client, method))
	_ = tel(context.Background())
	assert.Equal(t, float64(1), readGauge(incomingAPIRequestsInFlight, client, method))
}

func TestIncomingAPITelemeter_InFlight_DecrementOnRecord(t *testing.T) {
	client, method := uniqueLabel(t, "client"), uniqueLabel(t, "method")

	rec := IncomingAPITelemeter(client, method)(context.Background())
	assert.Equal(t, float64(1), readGauge(incomingAPIRequestsInFlight, client, method))

	var err error
	rec(&err)
	assert.Equal(t, float64(0), readGauge(incomingAPIRequestsInFlight, client, method))
}

func TestIncomingAPITelemeter_Duration_Success(t *testing.T) {
	client, method := uniqueLabel(t, "client"), uniqueLabel(t, "method")

	rec := IncomingAPITelemeter(client, method)(context.Background())

	var err error
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(incomingAPIRequestDurationSeconds, metricStatusSuccess, client, method),
		"expected one duration observation with status=success",
	)
}

func TestIncomingAPITelemeter_Duration_Failure(t *testing.T) {
	client, method := uniqueLabel(t, "client"), uniqueLabel(t, "method")

	rec := IncomingAPITelemeter(client, method)(context.Background())

	err := fmt.Errorf("request failed")
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(incomingAPIRequestDurationSeconds, metricStatusFailure, client, method),
		"expected one duration observation with status=failure",
	)
}

func TestIncomingAPITelemeter_Duration_Canceled(t *testing.T) {
	client, method := uniqueLabel(t, "client"), uniqueLabel(t, "method")

	ctx, cancel := context.WithCancel(context.Background())
	rec := IncomingAPITelemeter(client, method)(ctx)
	cancel()

	// resolveStatus derives "canceled" from ctx.Err(), not from the error value.
	// Any non-nil error causes it to fall through to the ctx.Err() check.
	err := fmt.Errorf("transport: read on canceled context")
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(incomingAPIRequestDurationSeconds, metricStatusCanceled, client, method),
		"expected one duration observation with status=canceled",
	)
}

func TestIncomingAPITelemeter_Duration_Canceled_NilError_StillSuccess(t *testing.T) {
	// resolveStatus short-circuits to "success" when err==nil, even if the context
	// is already canceled. This test documents that invariant explicitly.
	client, method := uniqueLabel(t, "client"), uniqueLabel(t, "method")

	ctx, cancel := context.WithCancel(context.Background())
	rec := IncomingAPITelemeter(client, method)(ctx)
	cancel()

	var err error
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(incomingAPIRequestDurationSeconds, metricStatusSuccess, client, method),
		"nil error should resolve to success even when the context is canceled",
	)
}

func TestIncomingAPITelemeter_Duration_DeadlineExceeded(t *testing.T) {
	client, method := uniqueLabel(t, "client"), uniqueLabel(t, "method")

	ctx, cancel := expiredCtx()
	defer cancel()
	rec := IncomingAPITelemeter(client, method)(ctx)

	// resolveStatus derives "deadline_exceeded" from ctx.Err(), not from the error value.
	err := fmt.Errorf("transport: read on expired context")
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(incomingAPIRequestDurationSeconds, metricStatusDeadlineExceeded, client, method),
		"expected one duration observation with status=deadline_exceeded",
	)
}

func TestIncomingAPITelemeter_Duration_DeadlineExceeded_NilError_StillSuccess(t *testing.T) {
	// resolveStatus short-circuits to "success" when err==nil, even if the deadline
	// has passed. This test documents that invariant explicitly.
	client, method := uniqueLabel(t, "client"), uniqueLabel(t, "method")

	ctx, cancel := expiredCtx()
	defer cancel()
	rec := IncomingAPITelemeter(client, method)(ctx)

	var err error
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(incomingAPIRequestDurationSeconds, metricStatusSuccess, client, method),
		"nil error should resolve to success even when the deadline has passed",
	)
}

// ---- OutgoingAPITelemeter ----

func TestOutgoingAPITelemeter_InFlight_IncrementOnStart(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	tel := OutgoingAPITelemeter(target, address, method)

	assert.Equal(t, float64(0), readGauge(outgoingAPIRequestsInFlight, target, address, method))
	_ = tel(context.Background())
	assert.Equal(t, float64(1), readGauge(outgoingAPIRequestsInFlight, target, address, method))
}

func TestOutgoingAPITelemeter_InFlight_DecrementOnRecord(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	rec := OutgoingAPITelemeter(target, address, method)(context.Background())
	assert.Equal(t, float64(1), readGauge(outgoingAPIRequestsInFlight, target, address, method))

	var err error
	rec(&err)
	assert.Equal(t, float64(0), readGauge(outgoingAPIRequestsInFlight, target, address, method))
}

func TestOutgoingAPITelemeter_Duration_Success(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	rec := OutgoingAPITelemeter(target, address, method)(context.Background())

	var err error
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusSuccess, target, address, method),
		"expected one duration observation with status=success",
	)
}

func TestOutgoingAPITelemeter_Duration_Failure(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	rec := OutgoingAPITelemeter(target, address, method)(context.Background())

	err := fmt.Errorf("backend unavailable")
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusFailure, target, address, method),
		"expected one duration observation with status=failure",
	)
}

func TestOutgoingAPITelemeter_Duration_Canceled(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	ctx, cancel := context.WithCancel(context.Background())
	rec := OutgoingAPITelemeter(target, address, method)(ctx)
	cancel()

	// resolveStatus derives "canceled" from ctx.Err(), not from the error value.
	err := fmt.Errorf("transport: read on canceled context")
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusCanceled, target, address, method),
	)
}

func TestOutgoingAPITelemeter_Duration_Canceled_NilError_StillSuccess(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	ctx, cancel := context.WithCancel(context.Background())
	rec := OutgoingAPITelemeter(target, address, method)(ctx)
	cancel()

	var err error
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusSuccess, target, address, method),
		"nil error should resolve to success even when the context is canceled",
	)
}

func TestOutgoingAPITelemeter_Duration_DeadlineExceeded(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	ctx, cancel := expiredCtx()
	defer cancel()
	rec := OutgoingAPITelemeter(target, address, method)(ctx)

	// resolveStatus derives "deadline_exceeded" from ctx.Err(), not from the error value.
	err := fmt.Errorf("transport: read on expired context")
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusDeadlineExceeded, target, address, method),
	)
}

func TestOutgoingAPITelemeter_Duration_DeadlineExceeded_NilError_StillSuccess(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	ctx, cancel := expiredCtx()
	defer cancel()
	rec := OutgoingAPITelemeter(target, address, method)(ctx)

	var err error
	rec(&err)

	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusSuccess, target, address, method),
		"nil error should resolve to success even when the deadline has passed",
	)
}

func TestOutgoingAPITelemeter_RetryTotal_NoIncrementOnSuccess(t *testing.T) {
	// The telemeter no longer manages the retry counter — that is done directly by
	// CaptureOutgoingAPIRequestRetryTotal inside LimitedRetryTransport on each EOF retry.
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	rec := OutgoingAPITelemeter(target, address, method)(context.Background())

	var err error
	rec(&err)

	assert.Equal(t, float64(0),
		readCounter(outgoingAPIRequestRetryTotal, target, address, method),
		"telemeter must not increment the retry counter — that is LimitedRetryTransport's job",
	)
}

func TestOutgoingAPITelemeter_RetryTotal_NoIncrementOnOrdinaryError(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	rec := OutgoingAPITelemeter(target, address, method)(context.Background())

	err := fmt.Errorf("ordinary error, not retryable")
	rec(&err)

	assert.Equal(t, float64(0),
		readCounter(outgoingAPIRequestRetryTotal, target, address, method),
		"telemeter must not increment the retry counter on a non-retryable error",
	)
}

// ---- OutgoingAPITelemeter — backpressure counter ----

func TestOutgoingAPITelemeter_Backpressure_IncrementOnServerBackPressure(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	rec := OutgoingAPITelemeter(target, address, method)(context.Background())

	err := tridentErrors.ServerBackPressureError("received status: 429 from server")
	rec(&err)

	assert.Equal(t, float64(1),
		readCounter(outgoingAPIRequestBackpressureTotal, target, address, method),
		"backpressure counter should be incremented on ServerBackPressureError",
	)
}

func TestOutgoingAPITelemeter_Backpressure_IncrementOnWrappedServerBackPressure(t *testing.T) {
	// Errors produced by WrapWithServerBackPressureError (e.g. EOF wrapped as backpressure)
	// must also trigger the counter because IsServerBackPressureError uses errors.As.
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	rec := OutgoingAPITelemeter(target, address, method)(context.Background())

	err := tridentErrors.WrapWithServerBackPressureError(fmt.Errorf("EOF"), "received EOF from server")
	rec(&err)

	assert.Equal(t, float64(1),
		readCounter(outgoingAPIRequestBackpressureTotal, target, address, method),
		"backpressure counter should be incremented on wrapped ServerBackPressureError",
	)
}

func TestOutgoingAPITelemeter_Backpressure_NoIncrementOnSuccess(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	rec := OutgoingAPITelemeter(target, address, method)(context.Background())

	var err error
	rec(&err)

	assert.Equal(t, float64(0),
		readCounter(outgoingAPIRequestBackpressureTotal, target, address, method),
		"backpressure counter should not be incremented on success",
	)
}

func TestOutgoingAPITelemeter_Backpressure_NoIncrementOnOrdinaryError(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	rec := OutgoingAPITelemeter(target, address, method)(context.Background())

	err := fmt.Errorf("ordinary error, not backpressure")
	rec(&err)

	assert.Equal(t, float64(0),
		readCounter(outgoingAPIRequestBackpressureTotal, target, address, method),
		"backpressure counter should not be incremented on a non-backpressure error",
	)
}

// ---- OutgoingAPITelemeter — idempotency (sync.Once) ----

func TestOutgoingAPITelemeter_RecorderIsIdempotent(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	rec := OutgoingAPITelemeter(target, address, method)(context.Background())

	var err error
	rec(&err)
	rec(&err) // second call must be a no-op

	assert.Equal(t, float64(0), readGauge(outgoingAPIRequestsInFlight, target, address, method),
		"in-flight gauge must not go negative when the recorder is called more than once",
	)
	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusSuccess, target, address, method),
		"duration must only be observed once even when the recorder is called multiple times",
	)
}

func TestIncomingAPITelemeter_RecorderIsIdempotent(t *testing.T) {
	client, method := uniqueLabel(t, "client"), uniqueLabel(t, "method")

	rec := IncomingAPITelemeter(client, method)(context.Background())

	var err error
	rec(&err)
	rec(&err) // second call must be a no-op

	assert.Equal(t, float64(0), readGauge(incomingAPIRequestsInFlight, client, method),
		"in-flight gauge must not go negative when the recorder is called more than once",
	)
	assert.Equal(t, uint64(1),
		readHistogramCount(incomingAPIRequestDurationSeconds, metricStatusSuccess, client, method),
		"duration must only be observed once even when the recorder is called multiple times",
	)
}

// ---- CaptureOutgoingAPIRequestTokenDuration ----

func TestCaptureOutgoingAPIRequestTokenDuration_ObservesAboveThreshold(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	CaptureOutgoingAPIRequestTokenDuration(context.Background(), target, address, method, 5*time.Millisecond)

	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestTokenDurationSeconds, target, address, method),
		"token duration histogram should be observed for waits at or above 1ms",
	)
}

func TestCaptureOutgoingAPIRequestTokenDuration_IgnoresSubMillisecond(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	// Capture the count before the call to establish a baseline, then verify it is unchanged.
	before := readHistogramCount(outgoingAPIRequestTokenDurationSeconds, target, address, method)
	CaptureOutgoingAPIRequestTokenDuration(context.Background(), target, address, method, 500*time.Microsecond)
	after := readHistogramCount(outgoingAPIRequestTokenDurationSeconds, target, address, method)

	assert.Equal(t, before, after, "sub-millisecond waits should be silently ignored to avoid noise")
}

// ---- CaptureOutgoingAPIRequestRetryTotal ----

func TestCaptureOutgoingAPIRequestRetryTotal_Increments(t *testing.T) {
	target, address, method := uniqueLabel(t, "target"), uniqueLabel(t, "address"), uniqueLabel(t, "method")

	CaptureOutgoingAPIRequestRetryTotal(context.Background(), target, address, method)
	CaptureOutgoingAPIRequestRetryTotal(context.Background(), target, address, method)

	assert.Equal(t, float64(2),
		readCounter(outgoingAPIRequestRetryTotal, target, address, method),
		"each call to CaptureOutgoingAPIRequestRetryTotal should increment the retry counter by 1",
	)
}
