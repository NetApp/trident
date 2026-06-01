// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ratelimit

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/netapp/trident/logging"
)

// maxDerivedBurst caps the auto-derived Burst at a value far larger than any
// realistic workload (~2 billion tokens) but small enough to guarantee that
// arithmetic on Burst remains well-behaved on any platform. Used only when the
// customer leaves rateLimitBurst unset and we scale it from Ceiling; protects
// against absurd Ceiling values producing nonsensically large buckets after
// float-to-int saturation.
const maxDerivedBurst = math.MaxInt32

// isFinitePositive reports whether v is a finite number strictly greater than
// zero. strconv.ParseFloat accepts "NaN", "Inf", and "+Inf" without error, and
// NaN compares false against every numeric bound, so naive range checks like
// `v <= 0` let these values slip through and silently corrupt the limiter's
// rate arithmetic. ParseLimiterConfig uses this helper to reject them at parse
// time.
func isFinitePositive(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0
}

// AdaptiveRateLimiter is a token-bucket rate limiter that adapts its rate to the
// upstream's capacity using AIMD (Additive Increase, Multiplicative Decrease).
//
//   - On 429 / gRPC ResourceExhausted, the rate is multiplicatively decreased
//     (default: halved), bounded by Floor.
//   - On sustained success, the rate is additively increased every ProbeInterval
//     by ProbeStep, bounded by Ceiling.
//   - Concurrent decrease signals are coalesced via a debounce window so that
//     N goroutines hitting 429 simultaneously cause exactly one decrease.
//
// The limiter is safe for concurrent use.
type AdaptiveRateLimiter struct {
	// key is informational; identifies the scope this limiter governs (e.g. GCP project number).
	key     string
	cfg     LimiterConfig
	metrics *Metrics

	limiter *rate.Limiter

	mu              sync.Mutex
	lastDecrease    time.Time
	lastIncreaseTry time.Time
	decreaseCount   uint64
	increaseCount   uint64

	// now allows tests to inject a fake clock. If nil, time.Now is used.
	now func() time.Time
}

// LimiterConfig holds tunables for AdaptiveRateLimiter. The package itself has
// no numeric opinions: every field must be supplied by the caller (usually a
// backend-specific baseline that is then merged with operator overrides via
// ParseLimiterConfig). Two shape-only fallbacks are performed by normalize:
// Floor is clamped to (0, Ceiling], and Burst is auto-derived from Ceiling if
// the caller leaves it unset.
//
// Rate-like fields below use rate.Limit (Go's native "events per second"
// type from golang.org/x/time/rate). The customer-facing JSON surface is
// per-minute (e.g. GCP's 1200/min quota); ParseLimiterConfig converts.
type LimiterConfig struct {
	// Ceiling is the maximum allowed rate, in events per SECOND. Required.
	// To express in req/min — the wording GCP uses for its 1200/min quota —
	// multiply by 60: rate.Limit(20) == 1200 req/min.
	Ceiling rate.Limit

	// Floor is the minimum rate (events per SECOND) the limiter may drop to.
	// Required; clamped down to Ceiling if larger. rate.Limit(1) == 60 req/min.
	Floor rate.Limit

	// Burst is the token bucket capacity (token COUNT, not a rate). If left
	// <= 0, normalize derives it as 3*Ceiling (~3 seconds of headroom at the
	// ceiling rate) and clamps the result to [1, maxDerivedBurst].
	Burst int

	// DecreaseFactor is multiplied with the current limit on a 429. Required;
	// must satisfy 0 < f < 1. e.g. 0.5 halves the limit on each decrease.
	DecreaseFactor float64

	// DecreaseDebounce is the minimum interval between successive decreases;
	// concurrent 429s within this window collapse into a single decrease.
	// Required; must be > 0.
	DecreaseDebounce time.Duration

	// ProbeInterval is the minimum interval between successive additive
	// increases. Required; must be > 0.
	ProbeInterval time.Duration

	// ProbeStep is the additive increment, in events per SECOND, applied at
	// each probe when there has been no recent decrease. If <= 0, normalize
	// treats it as unset and auto-derives it as Ceiling/20.
	ProbeStep rate.Limit
}

// normalize performs shape-only fixes that are independent of any backend's
// policy: Floor is clamped to (0, Ceiling], Burst is auto-derived from Ceiling
// when unset, and ProbeStep is scaled to Ceiling/20 when unset. Callers are
// responsible for choosing numeric defaults for the remaining fields (typically
// via a backend-specific baseline LimiterConfig passed to ParseLimiterConfig).
func (c *LimiterConfig) normalize() {
	if c.Floor > c.Ceiling {
		c.Floor = c.Ceiling
	}
	if c.Burst <= 0 {
		// Scale burst to ~3 seconds of headroom at the (possibly overridden) ceiling,
		// so a customer that only sets rateLimitPerMinute still gets a sensible burst.
		// Do the arithmetic in float64 and clamp to [1, maxDerivedBurst] before
		// converting to int. The lower bound prevents a tiny ceiling from
		// degenerating to a zero-burst (deadlock) limiter; the upper bound prevents
		// an absurd ceiling from producing a nonsensically large burst via
		// float-to-int saturation (e.g. rateLimitPerMinute="1e30").
		scaled := float64(c.Ceiling) * 3
		switch {
		case scaled < 1:
			c.Burst = 1
		case scaled > float64(maxDerivedBurst):
			c.Burst = maxDerivedBurst
		default:
			c.Burst = int(scaled)
		}
	}
	if c.ProbeStep <= 0 {
		// Scale the additive-increase step to Ceiling/20 so the recovery step size
		// grows with the customer's ceiling value. The actual number of probes
		// needed to recover from floor to ceiling still depends on the configured
		// Floor (roughly (Ceiling-Floor)/ProbeStep).
		c.ProbeStep = c.Ceiling / 20
		if c.ProbeStep <= 0 {
			c.ProbeStep = 1
		}
	}
}

// Validate returns an error if any required field is missing or out of range.
// Shape-only fields (Floor, Burst, ProbeStep) are not validated here because
// normalize will fix them; callers that want a fully-checked config should call
// Validate before normalize.
func (c LimiterConfig) Validate() error {
	if c.Ceiling <= 0 {
		return fmt.Errorf("rate limiter: Ceiling must be > 0, got %v", c.Ceiling)
	}
	if c.Floor <= 0 {
		return fmt.Errorf("rate limiter: Floor must be > 0, got %v", c.Floor)
	}
	if c.DecreaseFactor <= 0 || c.DecreaseFactor >= 1 {
		return fmt.Errorf("rate limiter: DecreaseFactor must be in (0,1), got %v", c.DecreaseFactor)
	}
	if c.DecreaseDebounce <= 0 {
		return fmt.Errorf("rate limiter: DecreaseDebounce must be > 0, got %v", c.DecreaseDebounce)
	}
	if c.ProbeInterval <= 0 {
		return fmt.Errorf("rate limiter: ProbeInterval must be > 0, got %v", c.ProbeInterval)
	}
	// ProbeStep is not validated: normalize() derives it from Ceiling/20 when unset.
	return nil
}

// NewAdaptiveRateLimiter constructs a limiter starting at cfg.Ceiling.
// key is used both for logging (e.g. project number) and as the Prometheus
// label value for the rate_limit_* metrics.
// metrics may be nil if Prometheus instrumentation is not desired.
func NewAdaptiveRateLimiter(key string, cfg LimiterConfig, metrics *Metrics) *AdaptiveRateLimiter {
	cfg.normalize()
	l := &AdaptiveRateLimiter{
		key:     key,
		cfg:     cfg,
		metrics: metrics,
		limiter: rate.NewLimiter(cfg.Ceiling, cfg.Burst),
	}
	metrics.SetCurrentPerSecond(l.metricKey(), float64(cfg.Ceiling))
	return l
}

// metricKey returns the Prometheus label value for this limiter. We use a
// non-empty placeholder when the configured key is empty so the cardinality
// stays bounded.
func (l *AdaptiveRateLimiter) metricKey() string {
	if l.key == "" {
		return "unknown"
	}
	return l.key
}

func (l *AdaptiveRateLimiter) clock() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

// Wait blocks until a token is available or ctx is done.
func (l *AdaptiveRateLimiter) Wait(ctx context.Context) error {
	return l.limiter.Wait(ctx)
}

// Limit returns the limiter's current rate.
func (l *AdaptiveRateLimiter) Limit() rate.Limit {
	return l.limiter.Limit()
}

// Burst returns the limiter's current burst size.
func (l *AdaptiveRateLimiter) Burst() int {
	return l.limiter.Burst()
}

// Stats returns counters useful for observability/tests.
func (l *AdaptiveRateLimiter) Stats() (decreases, increases uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.decreaseCount, l.increaseCount
}

// OnRateLimited is called whenever a request was rejected by the upstream with a
// 429 / ResourceExhausted. It multiplicatively decreases the limit, but only
// once per DecreaseDebounce window so that concurrent failures collapse into one
// decrease.
func (l *AdaptiveRateLimiter) OnRateLimited(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock()
	if !l.lastDecrease.IsZero() && now.Sub(l.lastDecrease) < l.cfg.DecreaseDebounce {
		return
	}

	old := l.limiter.Limit()
	newLimit := rate.Limit(float64(old) * l.cfg.DecreaseFactor)
	if newLimit < l.cfg.Floor {
		newLimit = l.cfg.Floor
	}
	if newLimit == old {
		// already at floor
		l.lastDecrease = now
		return
	}

	l.limiter.SetLimit(newLimit)
	l.lastDecrease = now
	// Reset the probe clock so we don't immediately try to climb back up.
	l.lastIncreaseTry = now
	l.decreaseCount++

	if l.metrics != nil {
		l.metrics.DecreasesTotal.WithLabelValues(l.metricKey()).Inc()
	}
	l.metrics.SetCurrentPerSecond(l.metricKey(), float64(newLimit))

	// Log per-minute so the numbers match the GCP-quota wording customers see
	// in their dashboards (e.g. "1200/min"). The native rate.Limit is per-sec.
	Logc(ctx).WithFields(LogFields{
		"key":            l.key,
		"oldLimitPerMin": fmt.Sprintf("%.2f", float64(old)*60),
		"newLimitPerMin": fmt.Sprintf("%.2f", float64(newLimit)*60),
		"floorPerMin":    fmt.Sprintf("%.2f", float64(l.cfg.Floor)*60),
	}).Warn("Adaptive rate limit decreased due to upstream throttling.")
}

// OnSuccess is called after a successful request. If enough time has passed since
// the last successful probe and there is no recent decrease, the limit is
// additively increased by ProbeStep up to Ceiling.
func (l *AdaptiveRateLimiter) OnSuccess(ctx context.Context) {
	if l.metrics != nil {
		l.metrics.SuccessesTotal.WithLabelValues(l.metricKey()).Inc()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock()
	cur := l.limiter.Limit()
	if cur >= l.cfg.Ceiling {
		// Pin the probe clock at the ceiling so a decrease followed by a single
		// success doesn't accidentally trigger an immediate increase later.
		l.lastIncreaseTry = now
		return
	}
	// Don't probe sooner than ProbeInterval after the last attempt, AND don't
	// probe sooner than ProbeInterval after a decrease.
	if !l.lastIncreaseTry.IsZero() && now.Sub(l.lastIncreaseTry) < l.cfg.ProbeInterval {
		return
	}
	if !l.lastDecrease.IsZero() && now.Sub(l.lastDecrease) < l.cfg.ProbeInterval {
		return
	}

	newLimit := cur + l.cfg.ProbeStep
	if newLimit > l.cfg.Ceiling {
		newLimit = l.cfg.Ceiling
	}
	l.limiter.SetLimit(newLimit)
	l.lastIncreaseTry = now
	l.increaseCount++

	if l.metrics != nil {
		l.metrics.IncreasesTotal.WithLabelValues(l.metricKey()).Inc()
	}
	l.metrics.SetCurrentPerSecond(l.metricKey(), float64(newLimit))

	Logc(ctx).WithFields(LogFields{
		"key":            l.key,
		"oldLimitPerMin": fmt.Sprintf("%.2f", float64(cur)*60),
		"newLimitPerMin": fmt.Sprintf("%.2f", float64(newLimit)*60),
		"ceilingPerMin":  fmt.Sprintf("%.2f", float64(l.cfg.Ceiling)*60),
	}).Info("Adaptive rate limit increased after sustained success.")
}

// LimiterConfigInput is the string-typed view of LimiterConfig consumed from
// the customer-facing JSON backend config. Only the ceiling is exposed; all
// other AIMD tunables are hardcoded in the backend-specific baseline.
type LimiterConfigInput struct {
	PerMinute string // ceiling, requests per minute (the only customer knob)
}

// ParseLimiterConfig merges the single customer-supplied override (ceiling)
// onto the caller's baseline LimiterConfig and returns the result. The
// returned config is Validate()'d so that a broken baseline doesn't silently
// propagate to NewAdaptiveRateLimiter.
//
// The baseline encodes backend-specific policy (e.g. GCNV's 1200/min ceiling);
// pkg/ratelimit deliberately ships no numeric defaults of its own.
func ParseLimiterConfig(baseline LimiterConfig, in LimiterConfigInput) (LimiterConfig, error) {
	cfg := baseline

	if in.PerMinute != "" {
		v, err := strconv.ParseFloat(in.PerMinute, 64)
		if err != nil {
			return cfg, fmt.Errorf("invalid rateLimitPerMinute %q: %w", in.PerMinute, err)
		}
		if !isFinitePositive(v) {
			return cfg, fmt.Errorf("invalid rateLimitPerMinute %q: must be a finite number > 0", in.PerMinute)
		}
		cfg.Ceiling = rate.Limit(v / 60.0)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// isTransportError reports whether the gRPC status code indicates a
// transport-level or server-health failure that does NOT prove the upstream had
// capacity to process the request. These are excluded from the "success" signal
// for additive increase.
func isTransportError(code codes.Code) bool {
	switch code {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.Unknown, codes.Canceled:
		return true
	}
	return false
}

// UnaryInterceptor returns a grpc.UnaryClientInterceptor that paces every RPC
// through this limiter, decreases on ResourceExhausted, and increases on
// success. Wire it via option.WithGRPCDialOption(grpc.WithUnaryInterceptor(...))
// when constructing a gRPC client.
//
// Success signal: any response where the server clearly processed the request
// (err == nil, or application-level gRPC errors like NotFound, InvalidArgument,
// AlreadyExists, PermissionDenied, etc.). These all prove the server had
// capacity. Transport-level errors (Unavailable, DeadlineExceeded, Internal,
// Unknown, Canceled) are ambiguous and ignored.
func (l *AdaptiveRateLimiter) UnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		if err := l.Wait(ctx); err != nil {
			return err
		}
		err := invoker(ctx, method, req, reply, cc, opts...)
		code := status.Code(err)
		switch {
		case code == codes.ResourceExhausted:
			l.OnRateLimited(ctx)
		case isTransportError(code):
			// Ambiguous — don't count as success or throttle.
		default:
			// Server processed the request (success or app-level error like NotFound).
			l.OnSuccess(ctx)
		}
		return err
	}
}
