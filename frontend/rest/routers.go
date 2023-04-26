// Copyright 2022 NetApp, Inc. All Rights Reserved.

package rest

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/kr/secureheader"

	"github.com/netapp/trident/frontend/csi"
)

// NewRouter is used to set up HTTP and HTTPS endpoints for the controller
func NewRouter(https bool) *mux.Router {
	return newRouter(controllerRoutes, https)
}

// NewNodeRouter is used to set up HTTPS liveness and readiness endpoints for the node
func NewNodeRouter(plugin *csi.Plugin) *mux.Router {
	return newRouter(nodeRoutes(plugin), true)
}

func newRouter(routes Routes, https bool) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)

	for _, route := range routes {
		handler := http.Handler(route.HandlerFunc)

		// Apply per-route middleware
		for i := len(route.Middleware) - 1; i >= 0; i-- {
			handler = route.Middleware[i](handler)
		}

		// Apply secure header middleware
		if https {
			handler = secureheader.Handler(handler)
		}

		// Apply logging middleware
		handler = Logger(handler, route.Name)

		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(handler)
	}

	return router
}
