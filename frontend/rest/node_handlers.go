// Copyright 2021 NetApp, Inc. All Rights Reserved.

package rest

import (
	"net/http"

	"github.com/netapp/trident/frontend/csi"
)

// Node endpoint for startup and liveness probe
func NodeLivenessCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// Node endpoint for readiness probe
func NodeReadinessCheck(plugin *csi.Plugin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isReady := plugin.IsReady()
		if isReady {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("error: not ready"))
		}
	}
}
