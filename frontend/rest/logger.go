// Copyright 2020 NetApp, Inc. All Rights Reserved.

package rest

import (
	"net/http"
	"strconv"
	"time"

	. "github.com/netapp/trident/logging"
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
		ctx := GenerateRequestContext(r.Context(), requestId, ContextSourceREST, WorkflowTridentRESTLogger, LogLayerRESTFrontend)
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
	subjects := []string{}
	if r.TLS != nil {
		for _, certs := range r.TLS.PeerCertificates {
			subjects = append(subjects, certs.Subject.String())
		}
	}

	logFields := LogFields{
		"Method":     r.Method,
		"Route":      name,
		"RequestURL": r.URL,
		"Duration":   time.Since(start),
	}

	if statusCode != "" {
		logFields["StatusCode"] = statusCode
	}

	Logc(r.Context()).WithFields(logFields).Debug(msg)

	logFields["ClientCertSubjects"] = subjects
	logFields["SourceIP"] = r.RemoteAddr
	logFields["Referer"] = r.Referer()
	logFields["UserAgent"] = r.UserAgent()
	logFields["Host"] = r.Host

	Audit().Logf(r.Context(), AuditRESTAccess, logFields, msg)
}
