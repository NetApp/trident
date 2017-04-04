package main

import (
	"fmt"

	"github.com/netapp/trident/config"
)

// BuildHash is the git hash the binary was built from
var BuildHash string

// BuildType is the type of build: custom, beta or stable
var BuildType string

// BuildTypeRev is the revision of the build
var BuildTypeRev string

// BuildTime is the time the binary was built
var BuildTime string

func init() {
	if BuildHash == "" {
		BuildHash = "unknown"
	}
	if BuildType == "" {
		BuildType = "custom"
	}
	if BuildTypeRev == "" {
		BuildTypeRev = "0"
	}
	if BuildTime == "" {
		BuildTime = "unknown"
	}
	if BuildType != "stable" {
		if BuildType == "custom" {
			config.OrchestratorFullVersion = fmt.Sprintf("%v-%v", config.OrchestratorVersion, BuildType)
		} else {
			config.OrchestratorFullVersion = fmt.Sprintf("%v-%v.%v", config.OrchestratorVersion, BuildType, BuildTypeRev)
		}
	}
	config.OrchestratorBuildVersion = fmt.Sprintf("%v+%v", config.OrchestratorFullVersion, BuildHash)
	config.OrchestratorBuildTime = BuildTime
}
