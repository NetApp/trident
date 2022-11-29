// Copyright 2022 NetApp, Inc. All Rights Reserved.

package rest

import (
	"math"
	"net/http"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
)

const logInterval = 10 * time.Second

// rateLimiterMiddleware creates a new rate limiter that wraps an http.Handler. To avoid spamming logs, sustained
// rejections will be logged at a rate of 1/logInterval
func rateLimiterMiddleware(r rate.Limit, b int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(r, b)
	logSometimes := rate.Sometimes{
		First:    1,
		Interval: logInterval,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res := limiter.Reserve()
			ok, delay := res.OK(), res.Delay()
			if ok && delay > 0 || !ok {
				logSometimes.Do(func() {
					log.WithField("path", r.URL.Path).Warn("Too many requests")
				})

				// retryAfter value in seconds. If not ok, delay is infinite
				if ok {
					retryAfter := strconv.Itoa(int(math.Ceil(delay.Seconds())))
					w.Header().Add("Retry-After", retryAfter)
				}
				w.WriteHeader(http.StatusTooManyRequests)

				// do not call next handler if too many requests
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
