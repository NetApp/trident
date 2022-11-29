package rest

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRateLimiterMiddleware(t *testing.T) {
	const (
		rateLimit = 0.001
		burst     = 1
		count     = 5
	)
	handler := rateLimiterMiddleware(rateLimit, burst)(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {}),
	)
	ts := httptest.NewServer(handler)

	var wg sync.WaitGroup
	wg.Add(count)
	s := make([]int, count)
	e := make([]error, count)
	h := make([]string, count)
	for i := 0; i < count; i++ {
		go func(k int) {
			defer wg.Done()
			res, err := http.Get(ts.URL)
			s[k], h[k], e[k] = res.StatusCode, res.Header.Get("Retry-After"), err
		}(i)
	}
	wg.Wait()
	var someRetryAfter bool
	for i := 0; i < count; i++ {
		assert.NoError(t, e[i])
		if h[i] != "" {
			someRetryAfter = true
		}
	}
	assert.True(t, someRetryAfter, "no retry after in any response")
	assert.Contains(t, s, http.StatusTooManyRequests)
}
