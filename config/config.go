// Copyright 2018 NetApp, Inc. All Rights Reserved.

package config

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/utils"
)

type Protocol string
type AccessMode string
type VolumeType string
type DriverContext string

type Telemetry struct {
	TridentVersion  string `json:"version"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platformVersion"`
}

const (
	/* Misc. orchestrator constants */
	OrchestratorName                 = "trident"
	orchestratorVersion              = "18.07.0"
	OrchestratorAPIVersion           = "1"
	PersistentStoreBootstrapAttempts = 30
	PersistentStoreBootstrapTimeout  = PersistentStoreBootstrapAttempts * time.Second
	PersistentStoreTimeout           = 10 * time.Second

	/* Protocol constants */
	File        Protocol = "file"
	Block       Protocol = "block"
	ProtocolAny Protocol = ""

	/* Access mode constants */
	ReadWriteOnce AccessMode = "ReadWriteOnce"
	ReadOnlyMany  AccessMode = "ReadOnlyMany"
	ReadWriteMany AccessMode = "ReadWriteMany"
	ModeAny       AccessMode = ""

	/* Volume type constants */
	OntapNFS          VolumeType = "ONTAP_NFS"
	OntapISCSI        VolumeType = "ONTAP_iSCSI"
	SolidFireISCSI    VolumeType = "SolidFire_iSCSI"
	ESeriesISCSI      VolumeType = "Eseries_iSCSI"
	UnknownVolumeType VolumeType = ""

	/* Driver-related constants */
	DefaultSolidFireVAG      = OrchestratorName
	UnknownDriver            = "UnknownDriver"
	StorageAPITimeoutSeconds = 90

	/* REST frontend constants */
	MaxRESTRequestSize = 10240

	/* Kubernetes deployment constants */
	ContainerTrident = "trident-main"
	ContainerEtcd    = "etcd"

	ContextDocker     DriverContext = "docker"
	ContextKubernetes DriverContext = "kubernetes"

	// Minimum and maximum supported Kubernetes versions
	KubernetesVersionMin = "v1.5.0"
	KubernetesVersionMax = "v1.10.0"
)

var (
	validProtocols = map[Protocol]bool{
		File:        true,
		Block:       true,
		ProtocolAny: true,
	}

	// BuildHash is the git hash the binary was built from
	BuildHash = "unknown"

	// BuildType is the type of build: custom, beta or stable
	BuildType = "custom"

	// BuildTypeRev is the revision of the build
	BuildTypeRev = "0"

	// BuildTime is the time the binary was built
	BuildTime = "unknown"

	// BuildImage is the Trident image that was built
	BuildImage = "netapp/trident:" + orchestratorVersion + "-custom.0"

	// EtcdImage is the etcd image that Trident should be deployed with
	BuildEtcdImage = "quay.io/coreos/etcd:v3.2.19"

	OrchestratorVersion = utils.MustParseDate(version())

	/* API Server and persistent store variables */
	BaseURL         = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion
	VersionURL      = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/version"
	BackendURL      = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/backend"
	VolumeURL       = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/volume"
	TransactionURL  = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/txn"
	StorageClassURL = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/storageclass"
	StoreURL        = "/" + OrchestratorName + "/store"

	UsingPassthroughStore bool
	CurrentDriverContext  DriverContext
	OrchestratorTelemetry = Telemetry{}
)

func IsValidProtocol(p Protocol) bool {
	_, ok := validProtocols[p]
	return ok
}

func GetValidProtocolNames() []string {
	ret := make([]string, len(validProtocols))
	for key := range validProtocols {
		ret = append(ret, string(key))
	}
	return ret
}

func PlatformAtLeast(platformName string, version string) bool {
	if OrchestratorTelemetry.Platform == platformName {
		platformVersion := utils.MustParseSemantic(OrchestratorTelemetry.PlatformVersion)
		requiredVersion, err := utils.ParseSemantic(version)
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

func version() string {

	var version string

	if BuildType != "stable" {
		if BuildType == "custom" {
			version = fmt.Sprintf("%v-%v+%v", orchestratorVersion, BuildType, BuildHash)
		} else {
			version = fmt.Sprintf("%v-%v.%v+%v", orchestratorVersion, BuildType, BuildTypeRev, BuildHash)
		}
	} else {
		version = orchestratorVersion
	}

	return version
}
