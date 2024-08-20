package rest

import (
	"net/http"

	"github.com/gorilla/mux"
)

// NewRouter is used to set up HTTP and HTTPS endpoints for the controller
func NewRouter(store FaultStore) *mux.Router {
	router := mux.NewRouter().StrictSlash(true)
	for _, route := range makeRoutes(store) {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(http.Handler(route.HandlerFunc))
	}

	return router
}
