// Copyright 2018 NetApp, Inc. All Rights Reserved.

package rest

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/netapp/trident/frontend/csi"
)

// NewRouter is used to set up HTTP and HTTPS endpoints for the controller
func NewRouter() *mux.Router {

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range controllerRoutes {
		var handler http.Handler

		handler = route.HandlerFunc
		handler = Logger(handler, route.Name)

		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(handler)
	}

	return router
}

// NewNodeRouter is used to set up HTTPS liveness and readiness endpoints for the node
func NewNodeRouter(plugin *csi.Plugin) *mux.Router {

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range nodeRoutes(plugin) {
		var handler http.Handler

		handler = route.HandlerFunc
		handler = Logger(handler, route.Name)

		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(handler)
	}

	return router
}
