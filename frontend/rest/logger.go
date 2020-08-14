// Copyright 2020 NetApp, Inc. All Rights Reserved.

package rest

import (
	"net/http"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/utils"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	// WriteHeader(int) is not called if our response implicitly returns 200 OK, so
	// we default to that status code.
	return &loggingResponseWriter{w, http.StatusOK}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

func Logger(inner http.Handler, routeName string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestId := ""
		// Use the request's request ID if it already exists
		if reqID := r.Header.Get("X-Request-ID"); reqID != "" {
			requestId = reqID
		}
		ctx := utils.GenerateRequestContext(r.Context(), requestId, utils.ContextSourceREST)
		r = r.WithContext(ctx)
		logRestCallInfo("REST API call received.", r, start, routeName, "")

		lrw := NewLoggingResponseWriter(w)
		inner.ServeHTTP(lrw, r)

		statusCode := strconv.Itoa(lrw.statusCode)
		restOpsTotal.WithLabelValues(r.Method, routeName, statusCode).Inc()
		endTime := float64(time.Since(start).Milliseconds())
		restOpsSecondsTotal.WithLabelValues(r.Method, routeName, statusCode).Observe(endTime)

		logRestCallInfo("REST API call complete.", r, start, routeName, statusCode)
	})
}

func logRestCallInfo(msg string, r *http.Request, start time.Time, name, statusCode string) {
	logc := utils.GetLogWithRequestContext(r.Context())
	logFields := log.Fields{
		"method":   r.Method,
		"uri":      r.RequestURI,
		"route":    name,
		"duration": time.Since(start),
	}
	if statusCode != "" {
		logFields["status_code"] = statusCode
	}
	logc.WithFields(logFields).Debug(msg)
}
