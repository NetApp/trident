// Copyright 2026 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/tools/metrics"

	. "github.com/netapp/trident/logging"
)

// registerK8sClientGoMetricsAdapter registers client-go metric adapters to feed Trident telemeters.
// This is currently limited to rate limiter latency and request retries.
func registerK8sClientGoMetricsAdapter() {
	metrics.Register(metrics.RegisterOpts{
		RateLimiterLatency: rateLimiterLatencyAdapter{},
		RequestRetry:       requestRetryAdapter{},
	})
}

// rateLimiterLatencyAdapter plugs Trident telemeters into client-go rate limiter latency metrics.
// This reflects the amount of time the client had to wait for a token from the limiter.
// This is directly indicates throttling due to QPS/burst limits.
type rateLimiterLatencyAdapter struct{}

func (rateLimiterLatencyAdapter) Observe(ctx context.Context, verb string, u url.URL, latency time.Duration) {
	CaptureOutgoingAPIRequestTokenDuration(ctx, ContextRequestTargetKubernetes, u.Host, verb, latency)
}

// requestRetryAdapter plugs Trident telemeters into client-go request retry metrics.
// This adapter only captures retries, not initial requests.
// Each retry indicates a failure that triggered the retry.
type requestRetryAdapter struct{}

func (requestRetryAdapter) IncrementRetry(ctx context.Context, code string, method string, host string) {
	err := assertErrorForCode(code)
	if err != nil {
		CaptureOutgoingAPIRequestRetryTotal(ctx, ContextRequestTargetKubernetes, host, method)
	}
}

// assertErrorForCode returns nil for 2xx/3xx HTTP status codes, error otherwise.
func assertErrorForCode(code string) error {
	c := strings.TrimSpace(code)
	if c == "" {
		return fmt.Errorf("missing http status code")
	}

	// Require a numeric HTTP status code.
	n, err := strconv.Atoi(c)
	if err != nil {
		return fmt.Errorf("invalid http status %s", c)
	}

	// Validate range: HTTP status codes are 100–599.
	if n < 100 || n > 599 {
		return fmt.Errorf("invalid http status %d", n)
	}

	// Return success for codes in the range: [200, 399], 200 <= n <= 399.
	if http.StatusOK <= n && n < http.StatusBadRequest {
		return nil
	}

	return fmt.Errorf("http status %d", n)
}
