// Copyright 2020 NetApp, Inc. All Rights Reserved.

package config

import (
	"fmt"
	"time"

	"github.com/netapp/trident/config"
	versionutils "github.com/netapp/trident/utils/version"
)

type Platform string

type Telemetry struct {
	Version         string `json:"version"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platformVersion"`
}

const (
	/* Misc. orchestrator constants */
	OperatorName    = "trident-operator"
	operatorVersion = config.DefaultOrchestratorVersion

	// ConfiguratorReconcileInterval is the resource refresh rate for the auto generated backends.
	ConfiguratorReconcileInterval time.Duration = 30 * time.Minute

	TridentImageEnv                       = "DEFAULT_TRIDENT_IMAGE"
	AutosupportImageEnv                   = "DEFAULT_TRIDENT_AUTOSUPPORT_IMAGE"
	CSISidecarProvisionerImageEnv         = "TRIDENT_CSI_SIDECAR_PROVISIONER_IMAGE"
	CSISidecarAttacherImageEnv            = "TRIDENT_CSI_SIDECAR_ATTACHER_IMAGE"
	CSISidecarResizerImageEnv             = "TRIDENT_CSI_SIDECAR_RESIZER_IMAGE"
	CSISidecarSnapshotterImageEnv         = "TRIDENT_CSI_SIDECAR_SNAPSHOTTER_IMAGE"
	CSISidecarNodeDriverRegistrarImageEnv = "TRIDENT_CSI_SIDECAR_NODE_DRIVER_REGISTRAR_IMAGE"
	CSISidecarLivenessProbeImageEnv       = "TRIDENT_CSI_SIDECAR_LIVENESS_PROBE_IMAGE"
)

var (
	// BuildHash is the git hash the binary was built from
	BuildHash = "unknown"

	// BuildType is the type of build: custom, beta or stable
	BuildType = "custom"

	// BuildTypeRev is the revision of the build
	BuildTypeRev = "0"

	// BuildTime is the time the binary was built
	BuildTime = "unknown"

	// BuildImage is the Trident Operator image that was built
	BuildImage = "docker.io/netapp/trident-operator:" + operatorVersion + "-custom.0"

	OperatorVersion = versionutils.MustParseDate(Version())
)

func Version() string {
	var version string

	if BuildType != "stable" {
		if BuildType == "custom" {
			version = fmt.Sprintf("%v-%v+%v", operatorVersion, BuildType, BuildHash)
		} else {
			version = fmt.Sprintf("%v-%v.%v+%v", operatorVersion, BuildType, BuildTypeRev, BuildHash)
		}
	} else {
		version = operatorVersion
	}

	return version
}
