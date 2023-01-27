// Copyright 2021 NetApp, Inc. All Rights Reserved.

package rest

import (
	"context"
	"net/http"

	"github.com/netapp/trident/frontend/csi"
	. "github.com/netapp/trident/logging"
)

// helper method to log HTTP write failures if an error is seen
func logerr(n int, err error) {
	if err != nil {
		Logc(context.Background()).Errorf("HTTP response write failed: %v", err)
	}
}

func sendErrResp(w http.ResponseWriter, err error, code int) {
	w.WriteHeader(code)
	logerr(w.Write([]byte(err.Error())))
}

// Node endpoint for startup and liveness probe
func NodeLivenessCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	logerr(w.Write([]byte("ok")))
}

// Node endpoint for readiness probe
func NodeReadinessCheck(plugin *csi.Plugin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isReady := plugin.IsReady()
		if isReady {
			w.WriteHeader(http.StatusOK)
			logerr(w.Write([]byte("ready")))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			logerr(w.Write([]byte("error: not ready")))
		}
	}
}
