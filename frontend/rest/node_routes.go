// Copyright 2021 NetApp, Inc. All Rights Reserved.

package rest

import (
	"github.com/netapp/trident/v21/frontend/csi"
)

func nodeRoutes(plugin *csi.Plugin) []Route {
	return Routes{
		Route{
			"LivenessProbe",
			"GET",
			"/liveness",
			NodeLivenessCheck,
		},
		Route{
			"ReadinessProbe",
			"GET",
			"/readiness",
			NodeReadinessCheck(plugin),
		},
	}
}
