// Copyright 2020 NetApp, Inc. All Rights Reserved.

package config

import (
	"fmt"

	tridentutils "github.com/netapp/trident/utils"
	log "github.com/sirupsen/logrus"
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
	operatorVersion = "20.04.0"
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
	BuildImage = "netapp/trident-operator:" + operatorVersion + "-custom.0"

	OperatorVersion = tridentutils.MustParseDate(Version())

	OperatorTelemetry = Telemetry{Version: OperatorVersion.String()}
)

func PlatformAtLeast(platformName string, version string) bool {
	if OperatorTelemetry.Platform == platformName {
		platformVersion := tridentutils.MustParseSemantic(OperatorTelemetry.PlatformVersion)
		requiredVersion, err := tridentutils.ParseSemantic(version)
		if err != nil {
			log.WithFields(log.Fields{
				"platform": platformName,
				"version":  version,
			}).Errorf("Platform version check failed. %+v", err)
			return false
		}
		if platformVersion.AtLeast(requiredVersion) {
			return true
		}
	}
	return false
}

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
