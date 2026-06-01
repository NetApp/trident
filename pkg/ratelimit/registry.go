// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ratelimit

import "sync"

// Registry maps a scope key (typically a project identifier) to a single shared
// AdaptiveRateLimiter. When quota is enforced per-project, all backends in a
// given process that target the same project must share a limiter; otherwise N
// backends would each ramp to the project's per-minute quota and collectively
// over-shoot it.
type Registry struct {
	mu       sync.Mutex
	limiters map[string]*AdaptiveRateLimiter
	metrics  *Metrics
}

// NewRegistry creates a Registry that associates new limiters with the given
// metrics set. metrics may be nil if instrumentation is not desired.
func NewRegistry(metrics *Metrics) *Registry {
	return &Registry{
		limiters: make(map[string]*AdaptiveRateLimiter),
		metrics:  metrics,
	}
}

// GetOrCreate returns the AdaptiveRateLimiter for the given key, creating it on
// first use. The provided cfg is only consulted on first creation; later callers
// with the same key get the existing limiter regardless of their cfg. (Backends
// targeting the same project should agree on the rate-limit config; the first
// one wins.)
//
// An empty key returns a fresh, unregistered limiter so callers without a
// project number (e.g. tests or anomalous configs) still get a working limiter
// without polluting the registry.
func (r *Registry) GetOrCreate(key string, cfg LimiterConfig) *AdaptiveRateLimiter {
	if key == "" {
		return NewAdaptiveRateLimiter(key, cfg, r.metrics)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.limiters[key]; ok {
		return l
	}
	l := NewAdaptiveRateLimiter(key, cfg, r.metrics)
	r.limiters[key] = l
	return l
}
