// Copyright 2025 NetApp, Inc. All Rights Reserved.

package config

import (
	"crypto/tls"
	"fmt"
	"time"

	k8sversion "k8s.io/apimachinery/pkg/version"

	versionutils "github.com/netapp/trident/utils/version"
)

type (
	Protocol      string
	AccessMode    string
	VolumeMode    string
	VolumeType    string
	DriverContext string
	Platform      string
)

type Telemetry struct {
	TridentVersion     string `json:"version"`
	TridentBackendUUID string `json:"backendUUID"`
	Platform           string `json:"platform"`
	PlatformVersion    string `json:"platformVersion"`
}

type PersistentStateVersion struct {
	PersistentStoreVersion string `json:"store_version"`
	OrchestratorAPIVersion string `json:"orchestrator_api_version"`
	PublicationsSynced     bool   `json:"publications_synced,omitempty"`
}

const (
	/* Misc. orchestrator constants */
	OrchestratorName                 = "trident"
	OrchestratorClientName           = OrchestratorName + "ctl"
	OrchestratorAPIVersion           = "1"
	DefaultOrchestratorVersion       = "25.06.0"
	PersistentStoreBootstrapAttempts = 30
	PersistentStoreBootstrapTimeout  = PersistentStoreBootstrapAttempts * time.Second
	PersistentStoreTimeout           = 10 * time.Second
	DockerCreateTimeout              = 115 * time.Second
	DockerDefaultTimeout             = 55 * time.Second
	// CSIUnixSocketPermissions CSI socket file needs rw access only for user
	CSIUnixSocketPermissions = 0o600
	// CSISocketDirPermissions CSI socket directory needs rwx access only for user
	CSISocketDirPermissions = 0o700

	/* REST/HTTP constants */
	HTTPTimeout       = 90 * time.Second
	HTTPTimeoutString = "90s"

	CACertName     = "trident-ca"
	ServerCertName = "trident-csi" // Must match CSI service name
	ClientCertName = "trident-node"

	CAKeyFile      = "caKey"
	CACertFile     = "caCert"
	ServerKeyFile  = "serverKey"
	ServerCertFile = "serverCert"
	ClientKeyFile  = "clientKey"
	ClientCertFile = "clientCert"
	AESKeyFile     = "aesKey"

	certsPath = "/certs/"

	CAKeyPath      = certsPath + CAKeyFile
	CACertPath     = certsPath + CACertFile
	ServerKeyPath  = certsPath + ServerKeyFile
	ServerCertPath = certsPath + ServerCertFile
	ClientKeyPath  = certsPath + ClientKeyFile
	ClientCertPath = certsPath + ClientCertFile
	AESKeyPath     = certsPath + AESKeyFile

	/* Protocol constants. This value denotes a volume's backing storage protocol. For example,
	a Trident volume with  'file' protocol is most likely NFS, while a 'block' protocol volume is probably iSCSI. */
	File        Protocol = "file"
	Block       Protocol = "block"
	ProtocolAny Protocol = ""

	/* Access mode constants */
	ReadWriteOnce    AccessMode = "ReadWriteOnce"
	ReadWriteOncePod AccessMode = "ReadWriteOncePod"
	ReadOnlyMany     AccessMode = "ReadOnlyMany"
	ReadWriteMany    AccessMode = "ReadWriteMany"
	ModeAny          AccessMode = ""

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
	UnknownVolumeType VolumeType = ""

	/* Driver-related constants */
	DefaultSolidFireVAG      = OrchestratorName
	UnknownDriver            = "UnknownDriver"
	StorageAPITimeoutSeconds = 90
	SANResizeDelta           = 50000000 // 50mb

	// Storage driver names specified in the config file, etc.
	OntapNASStorageDriverName          = "ontap-nas"
	OntapNASFlexGroupStorageDriverName = "ontap-nas-flexgroup"
	OntapNASQtreeStorageDriverName     = "ontap-nas-economy"
	OntapSANStorageDriverName          = "ontap-san"
	OntapSANEconomyStorageDriverName   = "ontap-san-economy"
	SolidfireSANStorageDriverName      = "solidfire-san"
	AzureNASStorageDriverName          = "azure-netapp-files"
	GCPNFSStorageDriverName            = "gcp-cvs"
	GCNVNASStorageDriverName           = "google-cloud-netapp-volumes"
	FakeStorageDriverName              = "fake"

	/* REST frontend constants */
	MaxRESTRequestSize  = 40960
	MinServerTLSVersion = tls.VersionTLS13
	MinClientTLSVersion = tls.VersionTLS12

	/* Docker constants */
	DockerPluginModeEnvVariable = "DOCKER_PLUGIN_MODE" // set via contrib/docker/plugin/plugin.json
	DockerPluginConfigLocation  = "/etc/netappdvp"

	/* Kubernetes deployment constants */
	ContainerTrident = "trident-main"
	ContainerACP     = "trident-acp"

	ContextDocker DriverContext = "docker"
	ContextCSI    DriverContext = "csi"

	PlatformDocker     Platform = "docker"
	PlatformKubernetes Platform = "kubernetes"
	PlatformCSI        Platform = "csi" // plain CSI, no other CO present

	// OS Types
	Linux   = "linux"
	Windows = "windows"
	Darwin  = "darwin"

	// Path separators
	WindowsPathSeparator = `\`
	UnixPathSeparator    = "/"

	// Minimum and maximum supported Kubernetes versions
	KubernetesVersionMin = "v1.26"
	KubernetesVersionMax = "v1.32"

	// KubernetesCSISidecarRegistry is where the CSI sidecar images are hosted
	KubernetesCSISidecarRegistry          = "registry.k8s.io/sig-storage"
	CSISidecarProvisionerImageTag         = "csi-provisioner:v5.2.0"
	CSISidecarAttacherImageTag            = "csi-attacher:v4.8.0"
	CSISidecarResizerImageTag             = "csi-resizer:v1.13.1"
	CSISidecarSnapshotterImageTag         = "csi-snapshotter:v8.2.0"
	CSISidecarNodeDriverRegistrarImageTag = "csi-node-driver-registrar:v2.13.0"
	CSISidecarLivenessProbeImageTag       = "livenessprobe:v2.9.0"

	DefaultK8sAPIQPS   = 100.0
	DefaultK8sAPIBurst = 200

	NamespaceFile          = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	VolumeTrackingInfoPath = "/var/lib/trident/tracking"

	// Pod Security Standards
	PodSecurityStandardsEnforceLabel   = "pod-security.kubernetes.io/enforce"
	PodSecurityStandardsEnforceProfile = "privileged"

	PodSecurityPoliciesRemovedKubernetesVersion = "1.25"

	/* Kubernetes operator constants */
	OperatorContainerName = "trident-operator"

	// IscsiSelfHealingInterval is an interval with which the iSCSI self-healing thread is called periodically.
	IscsiSelfHealingInterval       = 300 * time.Second
	IscsiSelfHealingIntervalString = "5m0s"

	// ISCSISelfHealingWaitTime is an interval after which iSCSI self-healing attempts to fix stale sessions.
	ISCSISelfHealingWaitTime       = 420 * time.Second
	ISCSISelfHealingWaitTimeString = "7m0s"

	// BackendStoragePollInterval is an interval  that core layer attempts to poll storage backend periodically
	BackendStoragePollInterval = 300 * time.Second

	// NVMeSelfHealingInterval is an interval with which the NVMe self-healing thread is called periodically
	NVMeSelfHealingInterval = 300 * time.Second

	// REDACTED is replacement text for sensitive information that would otherwise be exposed by HTTP logging, etc.
	REDACTED = "<REDACTED>"
)

var (
	ValidProtocols = map[Protocol]bool{
		File:        true,
		Block:       true,
		ProtocolAny: true,
	}

	MultiNodeAccessModes = [...]AccessMode{ReadOnlyMany, ReadWriteMany}

	// BuildHash is the git hash the binary was built from
	BuildHash = "unknown"

	// BuildType is the type of build: custom, beta or stable
	BuildType = "custom"

	// BuildTypeRev is the revision of the build
	BuildTypeRev = "0"

	// BuildTime is the time the binary was built
	BuildTime = "unknown"

	// BuildImage is the Trident image that was built
	BuildImage = "docker.io/netapp/trident:" + DefaultOrchestratorVersion + "-custom.0"

	OrchestratorVersion = versionutils.MustParseDate(version())

	/* API Server and persistent store variables */
	BaseURL          = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion
	VersionURL       = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/version"
	BackendURL       = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/backend"
	BackendUUIDURL   = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/backendUUID"
	VolumeURL        = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/volume"
	TransactionURL   = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/txn"
	StorageClassURL  = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/storageclass"
	NodeURL          = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/node"
	SnapshotURL      = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/snapshot"
	ChapURL          = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/chap"
	PublicationURL   = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/publication"
	LoggingConfigURL = "/" + OrchestratorName + "/v" + OrchestratorAPIVersion + "/logging"

	UsingPassthroughStore bool
	CurrentDriverContext  DriverContext
	OrchestratorTelemetry = Telemetry{TridentVersion: OrchestratorVersion.String()}

	// CSIAccessModes are defined by CSI
	// See https://github.com/container-storage-interface/spec/blob/release-1.5/lib/go/csi/csi.pb.go#L135
	CSIAccessModes = map[int32]string{
		0: "UNKNOWN",
		1: "SINGLE_NODE_WRITER",
		2: "SINGLE_NODE_READER_ONLY",
		3: "MULTI_NODE_READER_ONLY",
		4: "MULTI_NODE_SINGLE_WRITER",
		5: "MULTI_NODE_MULTI_WRITER",
		6: "SINGLE_NODE_SINGLE_WRITER",
		7: "SINGLE_NODE_MULTI_WRITER",
	}

	// DefaultAutosupportImage default image used by tridentctl and operator for asup sidecar
	DefaultAutosupportImage = "docker.io/netapp/trident-autosupport:24.10"

	// DefaultACPImage default image used by tridentctl and operator for acp sidecar
	DefaultACPImage = "cr.astra.netapp.io/astra/trident-acp:24.10.0"

	// TopologyKeyPrefixes contains well-known topology label prefixes
	TopologyKeyPrefixes = []string{"topology.kubernetes.io"}

	// TopologyRegionKeys contains well-known keys for topology region labels
	TopologyRegionKeys = []string{"topology.kubernetes.io/region"}

	// TopologyZoneKeys contains well-known keys for topology zone labels
	TopologyZoneKeys = []string{"topology.kubernetes.io/zone"}

	// QPS and Burst for k8s clients
	K8sAPIQPS   float32
	K8sAPIBurst int
)

func IsValidProtocol(p Protocol) bool {
	_, ok := ValidProtocols[p]
	return ok
}

func GetValidProtocolNames() []string {
	ret := make([]string, len(ValidProtocols))
	for key := range ValidProtocols {
		ret = append(ret, string(key))
	}
	return ret
}

func PlatformAtLeast(platformName, version string) error {
	if OrchestratorTelemetry.Platform == platformName {
		platformVersion := versionutils.MustParseSemantic(OrchestratorTelemetry.PlatformVersion)
		requiredVersion, err := versionutils.ParseSemantic(version)
		if err != nil {
			return fmt.Errorf("platform: %s , version: %s", platformName, version)
		}
		if platformVersion.AtLeast(requiredVersion) {
			return nil
		}
	}
	return nil
}

func version() string {
	var version string

	if BuildType != "stable" {
		if BuildType == "custom" {
			version = fmt.Sprintf("%v-%v+%v", DefaultOrchestratorVersion, BuildType, BuildHash)
		} else {
			version = fmt.Sprintf("%v-%v.%v+%v", DefaultOrchestratorVersion, BuildType, BuildTypeRev, BuildHash)
		}
	} else {
		version = DefaultOrchestratorVersion
	}

	return version
}

func ValidateKubernetesVersion(k8sMinVersion string, k8sVersion *versionutils.Version) error {
	k8sMMVersion := k8sVersion.ToMajorMinorVersion()
	minSupportedMMVersion := versionutils.MustParseMajorMinorVersion(k8sMinVersion)
	maxSupportedMMVersion := versionutils.MustParseMajorMinorVersion(KubernetesVersionMax)

	if k8sMMVersion.LessThan(minSupportedMMVersion) || k8sMMVersion.GreaterThan(maxSupportedMMVersion) {
		return versionutils.UnsupportedKubernetesVersionError(
			fmt.Errorf("Trident supports Kubernetes versions in the range [%s, %s]",
				minSupportedMMVersion.ToMajorMinorString(), maxSupportedMMVersion.ToMajorMinorString()))
	}

	return nil
}

func ValidateKubernetesVersionFromInfo(k8sMinVersion string, versionInfo *k8sversion.Info) error {
	k8sVersion, err := versionutils.ParseSemantic(versionInfo.GitVersion)
	if err != nil {
		return err
	}

	return ValidateKubernetesVersion(k8sMinVersion, k8sVersion)
}
