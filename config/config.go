// Copyright 2020 NetApp, Inc. All Rights Reserved.

package config

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/utils"
)

type Protocol string
type AccessMode string
type VolumeMode string
type VolumeType string
type DriverContext string
type Platform string

type Telemetry struct {
	TridentVersion  string `json:"version"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platformVersion"`
}

type PersistentStateVersion struct {
	PersistentStoreVersion string `json:"store_version"`
	OrchestratorAPIVersion string `json:"orchestrator_api_version"`
}

const (
	/* Misc. orchestrator constants */
	OrchestratorName                 = "trident"
	OrchestratorClientName           = OrchestratorName + "ctl"
	orchestratorVersion              = "19.10.1"
	OrchestratorAPIVersion           = "1"
	PersistentStoreBootstrapAttempts = 30
	PersistentStoreBootstrapTimeout  = PersistentStoreBootstrapAttempts * time.Second
	PersistentStoreTimeout           = 10 * time.Second

	/* REST/HTTP constants */
	HTTPTimeout = 90 * time.Second

	CACertName     = "trident-ca"
	ServerCertName = "trident-csi" // Must match CSI service name
	ClientCertName = "trident-node"

	CAKeyFile      = "caKey"
	CACertFile     = "caCert"
	ServerKeyFile  = "serverKey"
	ServerCertFile = "serverCert"
	ClientKeyFile  = "clientKey"
	ClientCertFile = "clientCert"

	certsPath = "/certs/"

	CAKeyPath      = certsPath + CAKeyFile
	CACertPath     = certsPath + CACertFile
	ServerKeyPath  = certsPath + ServerKeyFile
	ServerCertPath = certsPath + ServerCertFile
	ClientKeyPath  = certsPath + ClientKeyFile
	ClientCertPath = certsPath + ClientCertFile

	/* Protocol constants. This value denotes a volume's backing storage protocol. For example,
	a Trident volume with  'file' protocol is most likely NFS, while a 'block' protocol volume is probably iSCSI. */
	File        Protocol = "file"
	Block       Protocol = "block"
	ProtocolAny Protocol = ""

	/* Access mode constants */
	ReadWriteOnce AccessMode = "ReadWriteOnce"
	ReadOnlyMany  AccessMode = "ReadOnlyMany"
	ReadWriteMany AccessMode = "ReadWriteMany"
	ModeAny       AccessMode = ""

	/* Volume mode constants. This value describes how a volume will be consumed by application containers.
	Most Trident volumes (regardless of protocol) probably use the 'Filesystem' mode, where the volume contains
	a filesystem and is mounted into a container. By contrast, volumes with 'Block' mode always use 'block' protocol
	and are attached to a container as raw block devices. */
	RawBlock   VolumeMode = "Block"
	Filesystem VolumeMode = "Filesystem"

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
	SANResizeDelta           = 50000000 // 50mb

	/* REST frontend constants */
	MaxRESTRequestSize = 10240

	/* Kubernetes deployment constants */
	ContainerTrident = "trident-main"
	ContainerEtcd    = "etcd"

	ContextDocker     DriverContext = "docker"
	ContextKubernetes DriverContext = "kubernetes"
	ContextCSI        DriverContext = "csi"
	ContextCRD        DriverContext = "crd"

	PlatformDocker     Platform = "docker"
	PlatformKubernetes Platform = "kubernetes"
	PlatformCSI        Platform = "csi" // plain CSI, no other CO present

	// Minimum and maximum supported Kubernetes versions
	KubernetesVersionMin = "v1.11.0"
	KubernetesVersionMax = "v1.16.0"

	// Minimum Kubernetes version for CSI Trident (non-CSI is the default)
	KubernetesCSIVersionMinOptional = "v1.13.0"

	// Minimum Kubernetes version for CSI Trident default (non-CSI not supported)
	KubernetesCSIVersionMinForced = "v1.14.0"

	TridentNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
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

	// BuildEtcdVersion is the etcd version that Trident should be deployed with
	BuildEtcdVersion = "v3.3.13"

	// BuildEtcdImage is the etcd image that Trident should be deployed with
	BuildEtcdImage = "quay.io/coreos/etcd:" + BuildEtcdVersion

	OrchestratorVersion = utils.MustParseDate(version())

	/* API Server and persistent store variables */
	BaseURL         = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion
	VersionURL      = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/version"
	BackendURL      = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/backend"
	BackendUUIDURL  = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/backendUUID"
	VolumeURL       = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/volume"
	TransactionURL  = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/txn"
	StorageClassURL = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/storageclass"
	NodeURL         = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/node"
	SnapshotURL     = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/snapshot"
	StoreURL        = "/" + OrchestratorName + "/store"

	UsingPassthroughStore bool
	CurrentDriverContext  DriverContext
	OrchestratorTelemetry = Telemetry{TridentVersion: OrchestratorVersion.String()}
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
