// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ratelimit

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// testMetrics creates a fresh Metrics instance scoped to the test so parallel
// tests don't collide on global registrations.
var testMetricsInstance *Metrics

func init() {
	testMetricsInstance = NewMetrics("test", "key")
}

// fakeClock is a monotonically-advanceable clock used to drive AdaptiveRateLimiter
// deterministically in tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{now: start} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// testLimiterConfig returns a complete, valid LimiterConfig used as the baseline
// for tests. pkg/ratelimit ships no numeric defaults of its own, so each test
// supplies its own config; this helper keeps the values centralized. The
// numbers mirror the GCNV baseline but are local to the test file.
func testLimiterConfig() LimiterConfig {
	return LimiterConfig{
		Ceiling:          rate.Limit(20),
		Floor:            rate.Limit(1),
		Burst:            1000,
		DecreaseFactor:   0.5,
		DecreaseDebounce: 10 * time.Second,
		ProbeInterval:    30 * time.Second,
		ProbeStep:        rate.Limit(1),
	}
}

// newTestLimiter builds an AdaptiveRateLimiter with predictable values wired
// to a fake clock. We use a high burst so Wait() is essentially a no-op in
// tests; we are testing the AIMD math, not the token-bucket.
func newTestLimiter(t *testing.T) (*AdaptiveRateLimiter, *fakeClock, LimiterConfig) {
	t.Helper()
	clk := newFakeClock(time.Unix(0, 0))
	cfg := testLimiterConfig()
	l := NewAdaptiveRateLimiter("test-project", cfg, testMetricsInstance)
	l.now = clk.Now
	return l, clk, cfg
}

func TestLimiterConfig_FloorClampedToCeiling(t *testing.T) {
	cfg := LimiterConfig{Ceiling: 5, Floor: 100}
	cfg.normalize()
	assert.Equal(t, rate.Limit(5), cfg.Floor, "floor should be clamped down to ceiling")
}

func TestLimiterConfig_BurstScalesWithCeiling(t *testing.T) {
	cases := []struct {
		name      string
		ceiling   rate.Limit
		wantBurst int
	}{
		{"default-ceiling", 20, 60},
		{"halved-ceiling", 10, 30},
		{"raised-ceiling", 100, 300},
		{"tiny-ceiling-floors-to-1", 0.1, 1},
		{"absurd-ceiling-clamps-to-maxint32", rate.Limit(1e30), math.MaxInt32},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := LimiterConfig{Ceiling: tc.ceiling}
			cfg.normalize()
			assert.Equal(t, tc.wantBurst, cfg.Burst,
				"unset Burst should scale to ~3s of headroom at ceiling")
		})
	}
}

func TestLimiterConfig_ExplicitBurstWins(t *testing.T) {
	cfg := LimiterConfig{Ceiling: 100, Burst: 5}
	cfg.normalize()
	assert.Equal(t, 5, cfg.Burst, "explicit Burst must not be overridden by scaling")
}

func TestLimiterConfig_ProbeStepScalesWithCeiling(t *testing.T) {
	cases := []struct {
		name          string
		ceiling       rate.Limit
		wantProbeStep rate.Limit
	}{
		{"default-ceiling-1200pm", rate.Limit(20), rate.Limit(1)},       // 20/20 = 1 r/s = 60/min
		{"high-ceiling-3000pm", rate.Limit(50), rate.Limit(2.5)},        // 50/20 = 2.5 r/s = 150/min
		{"low-ceiling-120pm", rate.Limit(2), rate.Limit(0.1)},           // 2/20 = 0.1 r/s = 6/min
		{"very-low-ceiling", rate.Limit(0.001), rate.Limit(0.001) / 20}, // 0.001/20 = 0.00005 r/s
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := LimiterConfig{Ceiling: tc.ceiling}
			cfg.normalize()
			assert.InDelta(t, float64(tc.wantProbeStep), float64(cfg.ProbeStep), 1e-9,
				"ProbeStep should be Ceiling/20 (or clamped to 1 when tiny)")
		})
	}
}

func TestLimiterConfig_ExplicitProbeStepWins(t *testing.T) {
	cfg := LimiterConfig{Ceiling: 20, ProbeStep: rate.Limit(5)}
	cfg.normalize()
	assert.Equal(t, rate.Limit(5), cfg.ProbeStep,
		"explicit ProbeStep must not be overridden by scaling")
}

func TestLimiterConfig_Validate(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(*LimiterConfig)
		wantSubstr string
	}{
		{"valid", func(c *LimiterConfig) {}, ""},
		{"zero-ceiling", func(c *LimiterConfig) { c.Ceiling = 0 }, "Ceiling must be > 0"},
		{"zero-floor", func(c *LimiterConfig) { c.Floor = 0 }, "Floor must be > 0"},
		{"decrease-factor-zero", func(c *LimiterConfig) { c.DecreaseFactor = 0 }, "DecreaseFactor must be in (0,1)"},
		{"decrease-factor-one", func(c *LimiterConfig) { c.DecreaseFactor = 1 }, "DecreaseFactor must be in (0,1)"},
		{"zero-debounce", func(c *LimiterConfig) { c.DecreaseDebounce = 0 }, "DecreaseDebounce must be > 0"},
		{"zero-probe-interval", func(c *LimiterConfig) { c.ProbeInterval = 0 }, "ProbeInterval must be > 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testLimiterConfig()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if tc.wantSubstr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestNewAdaptiveRateLimiter_StartsAtCeiling(t *testing.T) {
	l, _, cfg := newTestLimiter(t)
	assert.Equal(t, cfg.Ceiling, l.Limit())
	assert.Equal(t, cfg.Burst, l.Burst())
}

func TestOnRateLimited_HalvesOnce(t *testing.T) {
	l, _, cfg := newTestLimiter(t)
	ctx := context.Background()

	l.OnRateLimited(ctx)
	assert.Equal(t, cfg.Ceiling*rate.Limit(cfg.DecreaseFactor), l.Limit(),
		"first 429 should halve the limit")

	dec, _ := l.Stats()
	assert.Equal(t, uint64(1), dec)
}

func TestOnRateLimited_DebouncesConcurrentSignals(t *testing.T) {
	l, _, cfg := newTestLimiter(t)
	ctx := context.Background()

	const goroutines = 50
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			l.OnRateLimited(ctx)
		}()
	}
	close(start)
	wg.Wait()

	dec, _ := l.Stats()
	assert.Equal(t, uint64(1), dec, "50 concurrent 429s within debounce window must produce exactly 1 decrease")
	assert.Equal(t, cfg.Ceiling*rate.Limit(cfg.DecreaseFactor), l.Limit())
}

func TestOnRateLimited_DecreasesAgainAfterDebounce(t *testing.T) {
	l, clk, cfg := newTestLimiter(t)
	ctx := context.Background()

	l.OnRateLimited(ctx)
	first := l.Limit()

	l.OnRateLimited(ctx)
	assert.Equal(t, first, l.Limit(), "second 429 within debounce must be a no-op")

	clk.Advance(cfg.DecreaseDebounce + time.Second)
	l.OnRateLimited(ctx)
	assert.Equal(t, first*rate.Limit(cfg.DecreaseFactor), l.Limit(),
		"after debounce a fresh 429 should halve again")

	dec, _ := l.Stats()
	assert.Equal(t, uint64(2), dec)
}

func TestOnRateLimited_ClampsAtFloor(t *testing.T) {
	l, clk, cfg := newTestLimiter(t)
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		l.OnRateLimited(ctx)
		clk.Advance(cfg.DecreaseDebounce + time.Second)
	}
	assert.Equal(t, cfg.Floor, l.Limit(), "limit must clamp at floor")
}

func TestOnSuccess_IncreasesAfterProbeInterval(t *testing.T) {
	l, clk, cfg := newTestLimiter(t)
	ctx := context.Background()

	l.OnRateLimited(ctx)
	afterDecrease := l.Limit()

	l.OnSuccess(ctx)
	assert.Equal(t, afterDecrease, l.Limit(),
		"OnSuccess immediately after a decrease must NOT increase the limit")

	clk.Advance(cfg.ProbeInterval + time.Second)
	l.OnSuccess(ctx)
	assert.Equal(t, afterDecrease+cfg.ProbeStep, l.Limit(),
		"after ProbeInterval a successful probe should additively increase by ProbeStep")

	_, inc := l.Stats()
	assert.Equal(t, uint64(1), inc)
}

func TestOnSuccess_ClampsAtCeiling(t *testing.T) {
	l, clk, cfg := newTestLimiter(t)
	ctx := context.Background()

	l.OnRateLimited(ctx)

	for i := 0; i < 100; i++ {
		clk.Advance(cfg.ProbeInterval + time.Second)
		l.OnSuccess(ctx)
	}
	assert.Equal(t, cfg.Ceiling, l.Limit(), "limit must not exceed ceiling")
}

func TestOnSuccess_NoOpWhenAlreadyAtCeiling(t *testing.T) {
	l, _, _ := newTestLimiter(t)
	ctx := context.Background()
	before := l.Limit()
	l.OnSuccess(ctx)
	assert.Equal(t, before, l.Limit())
	_, inc := l.Stats()
	assert.Equal(t, uint64(0), inc)
}

func TestOnSuccess_SuccessesCounterTicksEvenAtCeiling(t *testing.T) {
	const key = "test-project-success-counter"
	cfg := LimiterConfig{
		Ceiling:          rate.Limit(20),
		Floor:            rate.Limit(1),
		Burst:            1000,
		DecreaseFactor:   0.5,
		DecreaseDebounce: 10 * time.Second,
		ProbeInterval:    30 * time.Second,
		ProbeStep:        rate.Limit(1),
	}
	l := NewAdaptiveRateLimiter(key, cfg, testMetricsInstance)
	ctx := context.Background()

	before := testutil.ToFloat64(testMetricsInstance.SuccessesTotal.WithLabelValues(key))
	for i := 0; i < 5; i++ {
		l.OnSuccess(ctx)
	}
	after := testutil.ToFloat64(testMetricsInstance.SuccessesTotal.WithLabelValues(key))
	assert.Equal(t, before+5, after,
		"successes counter must tick on every OnSuccess invocation, including no-op-at-ceiling paths")

	_, inc := l.Stats()
	assert.Equal(t, uint64(0), inc,
		"raw successes must not be conflated with the additive-increase counter")
}

func TestOnSuccess_BurstOfSuccessesProducesOneIncreasePerInterval(t *testing.T) {
	l, clk, cfg := newTestLimiter(t)
	ctx := context.Background()

	l.OnRateLimited(ctx)
	clk.Advance(cfg.ProbeInterval + time.Second)

	for i := 0; i < 100; i++ {
		l.OnSuccess(ctx)
	}
	_, inc := l.Stats()
	assert.Equal(t, uint64(1), inc, "many rapid successes within one ProbeInterval must produce only one increase")
}

func TestUnaryInterceptor_DecreasesOnResourceExhausted(t *testing.T) {
	l, _, cfg := newTestLimiter(t)
	interceptor := l.UnaryInterceptor()

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return status.Error(codes.ResourceExhausted, "boom")
	}

	err := interceptor(context.Background(), "/test/Method", nil, nil, nil, invoker)
	assert.Error(t, err)
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	assert.Equal(t, cfg.Ceiling*rate.Limit(cfg.DecreaseFactor), l.Limit())

	dec, _ := l.Stats()
	assert.Equal(t, uint64(1), dec)
}

func TestUnaryInterceptor_AppLevelErrorCountsAsSuccess(t *testing.T) {
	l, clk, cfg := newTestLimiter(t)
	interceptor := l.UnaryInterceptor()
	ctx := context.Background()

	// Drive below ceiling so OnSuccess can trigger an increase.
	l.OnRateLimited(ctx)
	afterDecrease := l.Limit()
	assert.Equal(t, cfg.Ceiling*rate.Limit(cfg.DecreaseFactor), afterDecrease)

	// Advance past ProbeInterval so the next OnSuccess triggers an increase.
	clk.Advance(cfg.ProbeInterval + time.Second)

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return status.Error(codes.NotFound, "volume not found")
	}

	err := interceptor(ctx, "/test/Method", nil, nil, nil, invoker)
	assert.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Equal(t, afterDecrease+cfg.ProbeStep, l.Limit(),
		"NotFound (app-level error) should count as success and drive additive increase")

	dec, inc := l.Stats()
	assert.Equal(t, uint64(1), dec)
	assert.Equal(t, uint64(1), inc)
}

func TestUnaryInterceptor_TransportErrorsAreIgnored(t *testing.T) {
	transportCodes := []codes.Code{
		codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.Unknown, codes.Canceled,
	}
	for _, code := range transportCodes {
		t.Run(code.String(), func(t *testing.T) {
			l, clk, cfg := newTestLimiter(t)
			interceptor := l.UnaryInterceptor()
			ctx := context.Background()

			// Drive below ceiling.
			l.OnRateLimited(ctx)
			afterDecrease := l.Limit()
			clk.Advance(cfg.ProbeInterval + time.Second)

			invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
				return status.Error(code, "transport failure")
			}

			err := interceptor(ctx, "/test/Method", nil, nil, nil, invoker)
			assert.Error(t, err)
			assert.Equal(t, afterDecrease, l.Limit(),
				"transport error %s must not change the limit", code)

			dec, inc := l.Stats()
			assert.Equal(t, uint64(1), dec, "only the initial OnRateLimited should count")
			assert.Equal(t, uint64(0), inc, "transport errors must not count as success")
		})
	}
}

func TestUnaryInterceptor_HonorsCancelledContextDuringWait(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	cfg := LimiterConfig{
		Ceiling:          rate.Limit(0.5),
		Floor:            rate.Limit(0.1),
		Burst:            1,
		DecreaseFactor:   0.5,
		DecreaseDebounce: time.Second,
		ProbeInterval:    time.Second,
		ProbeStep:        rate.Limit(0.1),
	}
	l := NewAdaptiveRateLimiter("slow", cfg, testMetricsInstance)
	l.now = clk.Now

	ctx, cancel := context.WithCancel(context.Background())

	invokerCalls := int32(0)
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		atomic.AddInt32(&invokerCalls, 1)
		return nil
	}

	interceptor := l.UnaryInterceptor()
	_ = interceptor(ctx, "/test/Method", nil, nil, nil, invoker)
	assert.Equal(t, int32(1), atomic.LoadInt32(&invokerCalls), "first call should consume the burst token")

	cancel()
	err := interceptor(ctx, "/test/Method", nil, nil, nil, invoker)
	assert.Error(t, err, "second call should fail because Wait sees a cancelled ctx")
	assert.Equal(t, int32(1), atomic.LoadInt32(&invokerCalls), "invoker must not run when Wait fails")
}

func TestRegistry_ReturnsSameLimiterForSameKey(t *testing.T) {
	reg := NewRegistry(testMetricsInstance)
	cfg := testLimiterConfig()
	a := reg.GetOrCreate("project-1", cfg)
	b := reg.GetOrCreate("project-1", cfg)
	c := reg.GetOrCreate("project-2", cfg)

	assert.Same(t, a, b, "same project key must yield the same limiter")
	assert.NotSame(t, a, c, "different project keys must yield different limiters")
}

func TestRegistry_IgnoresLaterCfgForExistingKey(t *testing.T) {
	reg := NewRegistry(testMetricsInstance)
	first := reg.GetOrCreate("p", LimiterConfig{Ceiling: 5, Burst: 1})
	second := reg.GetOrCreate("p", LimiterConfig{Ceiling: 50, Burst: 100})

	assert.Same(t, first, second)
	assert.Equal(t, rate.Limit(5), second.Limit(), "first writer wins on cfg")
}

func TestRegistry_EmptyKeyReturnsFreshLimiter(t *testing.T) {
	reg := NewRegistry(testMetricsInstance)
	a := reg.GetOrCreate("", testLimiterConfig())
	b := reg.GetOrCreate("", testLimiterConfig())
	assert.NotSame(t, a, b, "empty key must NOT be registered globally")
}

func TestParseLimiterConfig_AllEmptyReturnsBaseline(t *testing.T) {
	baseline := testLimiterConfig()
	cfg, err := ParseLimiterConfig(baseline, LimiterConfigInput{})
	assert.NoError(t, err)
	assert.Equal(t, baseline, cfg)
}

func TestParseLimiterConfig_RejectsInvalidBaseline(t *testing.T) {
	baseline := testLimiterConfig()
	baseline.Ceiling = 0 // would slip past the parse-time checks otherwise
	_, err := ParseLimiterConfig(baseline, LimiterConfigInput{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Ceiling must be > 0")
}

func TestParseLimiterConfig_OverridesCeiling(t *testing.T) {
	cfg, err := ParseLimiterConfig(testLimiterConfig(), LimiterConfigInput{
		PerMinute: "600",
	})
	assert.NoError(t, err)
	assert.InDelta(t, 10.0, float64(cfg.Ceiling), 1e-6, "600/min should parse as 10 r/s")
	// Other fields unchanged from baseline
	assert.InDelta(t, 1.0, float64(cfg.Floor), 1e-6)
	assert.Equal(t, 1000, cfg.Burst)
	assert.InDelta(t, 0.5, cfg.DecreaseFactor, 1e-6)
	assert.Equal(t, 10*time.Second, cfg.DecreaseDebounce)
	assert.Equal(t, 30*time.Second, cfg.ProbeInterval)
}

func TestParseLimiterConfig_RejectsBadValues(t *testing.T) {
	cases := []struct {
		name       string
		in         LimiterConfigInput
		wantSubstr string
	}{
		{"neg-per-minute", LimiterConfigInput{PerMinute: "-1"}, "must be a finite number > 0"},
		{"nan-per-minute", LimiterConfigInput{PerMinute: "NaN"}, "must be a finite number > 0"},
		{"inf-per-minute", LimiterConfigInput{PerMinute: "Inf"}, "must be a finite number > 0"},
		{"neg-inf-per-minute", LimiterConfigInput{PerMinute: "-Inf"}, "must be a finite number > 0"},
		{"non-numeric-per-minute", LimiterConfigInput{PerMinute: "lots"}, "invalid rateLimitPerMinute"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseLimiterConfig(testLimiterConfig(), tc.in)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantSubstr)
			assert.NotContains(t, err.Error(), "%!w(<nil>)")
		})
	}
}

func TestNewAdaptiveRateLimiter_NilMetrics(t *testing.T) {
	cfg := testLimiterConfig()
	l := NewAdaptiveRateLimiter("test", cfg, nil)
	ctx := context.Background()

	l.OnRateLimited(ctx)
	l.OnSuccess(ctx)
	assert.Equal(t, cfg.Ceiling*rate.Limit(cfg.DecreaseFactor), l.Limit(),
		"limiter should work without metrics")
}
