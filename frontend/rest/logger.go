// Copyright 2020 NetApp, Inc. All Rights Reserved.

package rest

import (
	"net/http"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
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

func Logger(inner http.Handler, routeName string, logLevel log.Level) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestId := ""
		// Use the request's request ID if it already exists
		if reqID := r.Header.Get("X-Request-ID"); reqID != "" {
			requestId = reqID
		}
		ctx := GenerateRequestContext(r.Context(), requestId, ContextSourceREST)
		r = r.WithContext(ctx)
		logRestCallInfo("REST API call received.", r, start, routeName, "", logLevel)

		lrw := NewLoggingResponseWriter(w)
		inner.ServeHTTP(lrw, r)

		statusCode := strconv.Itoa(lrw.statusCode)
		restOpsTotal.WithLabelValues(r.Method, routeName, statusCode).Inc()
		endTime := float64(time.Since(start).Milliseconds())
		restOpsSecondsTotal.WithLabelValues(r.Method, routeName, statusCode).Observe(endTime)

		logRestCallInfo("REST API call complete.", r, start, routeName, statusCode, logLevel)
	})
}

func logRestCallInfo(msg string, r *http.Request, start time.Time, name, statusCode string, logLevel log.Level) {

	logFields := log.Fields{
		"method":   r.Method,
		"uri":      r.RequestURI,
		"route":    name,
		"duration": time.Since(start),
	}
	if statusCode != "" {
		logFields["status_code"] = statusCode
	}

	switch logLevel {
	case log.TraceLevel:
		Logc(r.Context()).WithFields(logFields).Trace(msg)
	case log.DebugLevel:
		fallthrough
	default:
		Logc(r.Context()).WithFields(logFields).Debug(msg)
	}
}
