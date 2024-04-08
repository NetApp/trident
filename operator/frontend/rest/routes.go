// Copyright 2024 NetApp, Inc. All Rights Reserved.

package rest

import (
	"net/http"

	"github.com/gorilla/mux"

	logger "github.com/netapp/trident/frontend/rest"
	"github.com/netapp/trident/operator/clients"
)

type route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

// NewRouter is used to set up HTTP and HTTPS endpoints for the controller
func NewRouter(clientFactory *clients.Clients) *mux.Router {
	return newRouter(clientFactory)
}

func newRouter(clientFactory *clients.Clients) *mux.Router {
	opHandler := NewOperatorHandler(clientFactory)

	router := mux.NewRouter()
	routes := []route{
		{
			"readiness",
			"GET",
			"/operator/status",
			opHandler.GetStatus,
		},
	}

	for _, r := range routes {
		var handler http.Handler
		handler = r.HandlerFunc
		handler = logger.Logger(handler, r.Name)

		router.
			Methods(r.Method).
			Path(r.Pattern).
			Name(r.Name).
			Handler(r.HandlerFunc)
	}

	return router
}
