// Copyright 2025 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/config"
	csiConfig "github.com/netapp/trident/frontend/csi"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	versionutils "github.com/netapp/trident/utils/version"
)

const (
	CacheSyncPeriod         = 60 * time.Second
	PreSyncCacheWaitPeriod  = 10 * time.Second
	PostSyncCacheWaitPeriod = 30 * time.Second
	ImportPVCacheWaitPeriod = 180 * time.Second

	CacheBackoffInitialInterval     = 1 * time.Second
	CacheBackoffRandomizationFactor = 0.1
	CacheBackoffMultiplier          = 1.414
	CacheBackoffMaxInterval         = 5 * time.Second

	NASTypeSMB = "smb"

	SMBShareFullControlPermission = "full_control" // AD user Full Control permission
	SMBShareReadPermission        = "read"         // AD user Read Only permission
	SMBShareNoPermission          = "no_access"    // AD user No Access permission
	SMBShareChangePermission      = "change"       // AD user Change permission

	// Kubernetes-defined storage class parameters

	K8sFsType                            = "fsType"
	CSIParameterPrefix                   = "csi.storage.k8s.io/"
	CSIParameterNodeStageSecretName      = CSIParameterPrefix + "node-stage-secret-name"
	CSIParameterNodeStageSecretNamespace = CSIParameterPrefix + "node-stage-secret-namespace"

	// Kubernetes-defined annotations
	// (Based on kubernetes/pkg/controller/volume/persistentvolume/controller.go)

	AnnStorageProvisioner = "volume.beta.kubernetes.io/storage-provisioner"

	// Orchestrator-defined annotations
	prefix                      = config.OrchestratorName + ".netapp.io"
	AnnProtocol                 = prefix + "/protocol"
	AnnSnapshotPolicy           = prefix + "/snapshotPolicy"
	AnnSnapshotReserve          = prefix + "/snapshotReserve"
	AnnSnapshotDir              = prefix + "/snapshotDirectory"
	AnnUnixPermissions          = prefix + "/unixPermissions"
	AnnExportPolicy             = prefix + "/exportPolicy"
	AnnBlockSize                = prefix + "/blockSize"
	AnnFileSystem               = prefix + "/fileSystem"
	AnnCloneFromPVC             = prefix + "/cloneFromPVC"
	AnnCloneFromSnapshot        = prefix + "/cloneFromSnapshot"
	AnnSplitOnClone             = prefix + "/splitOnClone"
	AnnNotManaged               = prefix + "/notManaged"
	AnnImportOriginalName       = prefix + "/importOriginalName"
	AnnImportBackendUUID        = prefix + "/importBackendUUID"
	AnnInternalSnapshotName     = prefix + "/internalSnapshotName"
	AnnMirrorRelationship       = prefix + "/mirrorRelationship"
	AnnVolumeShareFromPVC       = prefix + "/shareFromPVC"
	AnnVolumeCloneToNS          = prefix + "/cloneToNamespace"
	AnnVolumeCloneFromNS        = prefix + "/cloneFromNamespace"
	AnnVolumeShareToNS          = prefix + "/shareToNamespace"
	AnnReadOnlyClone            = prefix + "/readOnlyClone"
	AnnLUKSEncryption           = prefix + "/luksEncryption" // import only
	AnnSkipRecoveryQueue        = prefix + "/skipRecoveryQueue"
	AnnSelector                 = prefix + "/selector"
	AnnSMBShareAdUserPermission = prefix + "/smbShareAdUserPermission"
	AnnSMBShareAdUser           = prefix + "/smbShareAdUser"
	AnnSMBShareAccessControl    = prefix + "/smbShareAccessControl"

	// Orchestrator-defined storage class parameters
	SCParameterNASType = prefix + "/nasType"
)

var features = map[controllerhelpers.Feature]*versionutils.Version{
	csiConfig.ExpandCSIVolumes: versionutils.MustParseSemantic("1.16.0"),
	csiConfig.CSIBlockVolumes:  versionutils.MustParseSemantic("1.14.0"),
}

var (
	listOpts = metav1.ListOptions{}
	getOpts  = metav1.GetOptions{}
)
