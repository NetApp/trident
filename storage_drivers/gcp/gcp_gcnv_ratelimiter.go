// Copyright 2026 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"context"
	"time"

	"golang.org/x/time/rate"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/ratelimit"
	drivers "github.com/netapp/trident/storage_drivers"
)

// GCNV's per-project quota is enforced server-side, so all GCNV backends in
// this Trident process that target the same project must share a single
// limiter; otherwise N backends would each ramp toward the project's
// per-minute quota and collectively over-shoot it. We register GCNV-specific
// Prometheus metrics under subsystem="gcnv" with a "project" label, and hold a
// process-wide registry keyed by project number.
var (
	gcnvLimiterMetrics  = ratelimit.NewMetrics("gcnv", "project")
	gcnvLimiterRegistry = ratelimit.NewRegistry(gcnvLimiterMetrics)
)

// gcnvLimiterDefaults returns the GCNV-calibrated baseline used as the
// starting point for parsing operator overrides.
//
// The numbers below are tuned for GCP's per-project quota of 1200
// requests/minute. GCP enforces that quota as a 60-second sliding window,
// which permits short bursts well above the steady-state 20 req/s as long as
// the rolling-minute total stays under 1200. Our token bucket approximates
// that with a Burst of 60 (~3 seconds of headroom at the ceiling rate), which
// is big enough to absorb a handful of parallel volume operations without
// local Wait() pacing, while still being tiny compared to the full per-minute
// budget. The AIMD shape (halve on 429, 10s decrease-debounce, 30s probe
// interval) is also tuned to the GCP quota window.
//
// ProbeStep is left at 0 so that normalize() derives it as Ceiling/20; with a
// non-zero floor, recovery from floor to ceiling takes about
// (Ceiling-Floor)/ProbeStep probes (18 probes, or ~9 min at
// ProbeInterval=30s, with the defaults below), while still scaling with the
// customer's rateLimitPerMinute override.
//
// LimiterConfig fields are in events-per-SECOND (Go stdlib's native unit);
// the per-minute comments here are the customer-facing equivalents.
//
// pkg/ratelimit deliberately ships no numeric defaults of its own; this
// function is the canonical place for the GCNV policy.
func gcnvLimiterDefaults() ratelimit.LimiterConfig {
	return ratelimit.LimiterConfig{
		Ceiling:          rate.Limit(20),   //   20 r/s = 1200 req/min  (GCP quota)
		Floor:            rate.Limit(2),    //    2 r/s =  120 req/min
		Burst:            0,                //   0 → normalize() derives ~3s of headroom at effective ceiling
		DecreaseFactor:   0.5,              //   halve the limit on each 429 burst
		DecreaseDebounce: 10 * time.Second, //   coalesce 429 storms within 10s
		ProbeInterval:    30 * time.Second, //   wait 30s between probe-ups
		// ProbeStep: 0 — derived by normalize() as Ceiling/20 (~1 r/s = 60 req/min for the default ceiling)
	}
}

// gcnvLimiterFor parses the rate-limit knobs on the GCNV driver config and
// returns the per-project AdaptiveRateLimiter from the shared registry. The
// registry guarantees one limiter per project across NAS+SAN backends in this
// process. The first caller's config wins; subsequent callers for the same
// project receive the existing limiter.
//
// On parse error the limiter is not constructed and the error is logged.
func gcnvLimiterFor(
	ctx context.Context, config *drivers.GCNVStorageDriverConfig,
) (*ratelimit.AdaptiveRateLimiter, error) {
	cfg, err := ratelimit.ParseLimiterConfig(gcnvLimiterDefaults(), ratelimit.LimiterConfigInput{
		PerMinute: config.RateLimitPerMinute,
	})
	if err != nil {
		Logc(ctx).WithError(err).Error("Invalid GCNV rate limit configuration.")
		return nil, err
	}
	return gcnvLimiterRegistry.GetOrCreate(config.ProjectNumber, cfg), nil
}
