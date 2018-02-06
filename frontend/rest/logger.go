// Copyright 2018 NetApp, Inc. All Rights Reserved.

package rest

import (
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

func Logger(inner http.Handler, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		inner.ServeHTTP(w, r)
		log.WithFields(log.Fields{
			"method":   r.Method,
			"uri":      r.RequestURI,
			"route":    name,
			"duration": time.Since(start),
		}).Info("API server REST call.")
	})
}
