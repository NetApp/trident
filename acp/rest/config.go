// Copyright 2023 NetApp, Inc. All Rights Reserved.

package rest

import (
	"fmt"

	"github.com/netapp/trident/config"
)

const (
	// Trident-ACP REST API constants.

	appName    = "trident-acp"
	apiVersion = "1"

	entitledEndpoint = "/" + appName + "/v" + apiVersion + "/entitled"
	versionEndpoint  = "/" + appName + "/v" + apiVersion + "/version"
)

// userAgent is a helper for adding a User-Agent header for Trident to Trident-ACP API calls.
// Example: trident/v23.07.0
func userAgent() string {
	return fmt.Sprintf("%s/v%s", config.OrchestratorName, config.OrchestratorVersion.String())
}
