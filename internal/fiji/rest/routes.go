package rest

import (
	"net/http"
)

const (
	apiPrefix      = "/" + apiName
	faultURL       = apiPrefix + "/" + "fault"
	faultByNameURL = faultURL + "/{name}"
)

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type Routes []Route

func makeRoutes(store FaultStore) Routes {
	return Routes{
		Route{
			"ListFaultConfigs",
			"GET",
			faultURL,
			makeListFaultsHandler(store),
		},
		Route{
			"GetFaultConfig",
			"GET",
			faultByNameURL,
			makeGetFaultHandler(store),
		},
		Route{
			"SetFaultConfig",
			"PATCH",
			faultByNameURL,
			makeSetFaultConfigHandler(store),
		},
		Route{
			"DeleteFaultConfig",
			"DELETE",
			faultByNameURL,
			makeResetFaultConfigHandler(store),
		},
	}
}
