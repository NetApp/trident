// Copyright 2023 NetApp, Inc. All Rights Reserved.

package acp

import (
	"fmt"
	"time"

	"github.com/netapp/trident/config"
)

const (
	// ACP REST API constants.

	appName    = "trident-acp"
	apiVersion = "1"

	DefaultBaseURL = "http://127.0.0.1:8100"

	entitledEndpoint = "/" + appName + "/v" + apiVersion + "/entitled"
	versionEndpoint  = "/" + appName + "/v" + apiVersion + "/version"

	httpClientTimeout = time.Second * 30

	// Feature constants.

	FeatureSnapshotRestore      = "SnapshotRestore"
	FeatureSnapshotMirrorUpdate = "SnapshotMirrorUpdate"
	FeatureInFlightEncryption   = "InFlightEncryption"
	FeatureReadOnlyClone        = "ReadOnlyClone"
)

// userAgent is a helper for adding a User-Agent header for Trident to ACP API calls.
// Example: trident/v23.07.0
func userAgent() string {
	return fmt.Sprintf("%s/v%s", config.OrchestratorName, config.OrchestratorVersion.String())
}
