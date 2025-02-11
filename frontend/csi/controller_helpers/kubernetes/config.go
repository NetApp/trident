// Copyright 2022 NetApp, Inc. All Rights Reserved.

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

	// Kubernetes-defined storage class parameters

	K8sFsType                            = "fsType"
	CSIParameterPrefix                   = "csi.storage.k8s.io/"
	CSIParameterNodeStageSecretName      = CSIParameterPrefix + "node-stage-secret-name"
	CSIParameterNodeStageSecretNamespace = CSIParameterPrefix + "node-stage-secret-namespace"

	// Kubernetes-defined annotations
	// (Based on kubernetes/pkg/controller/volume/persistentvolume/controller.go)

	AnnStorageProvisioner = "volume.beta.kubernetes.io/storage-provisioner"

	// Orchestrator-defined annotations
	annPrefix               = config.OrchestratorName + ".netapp.io"
	AnnProtocol             = annPrefix + "/protocol"
	AnnSnapshotPolicy       = annPrefix + "/snapshotPolicy"
	AnnSnapshotReserve      = annPrefix + "/snapshotReserve"
	AnnSnapshotDir          = annPrefix + "/snapshotDirectory"
	AnnUnixPermissions      = annPrefix + "/unixPermissions"
	AnnExportPolicy         = annPrefix + "/exportPolicy"
	AnnBlockSize            = annPrefix + "/blockSize"
	AnnFileSystem           = annPrefix + "/fileSystem"
	AnnCloneFromPVC         = annPrefix + "/cloneFromPVC"
	AnnCloneFromSnapshot    = annPrefix + "/cloneFromSnapshot"
	AnnSplitOnClone         = annPrefix + "/splitOnClone"
	AnnNotManaged           = annPrefix + "/notManaged"
	AnnImportOriginalName   = annPrefix + "/importOriginalName"
	AnnImportBackendUUID    = annPrefix + "/importBackendUUID"
	AnnInternalSnapshotName = annPrefix + "/internalSnapshotName"
	AnnMirrorRelationship   = annPrefix + "/mirrorRelationship"
	AnnVolumeShareFromPVC   = annPrefix + "/shareFromPVC"
	AnnVolumeCloneToNS      = annPrefix + "/cloneToNamespace"
	AnnVolumeCloneFromNS    = annPrefix + "/cloneFromNamespace"
	AnnVolumeShareToNS      = annPrefix + "/shareToNamespace"
	AnnReadOnlyClone        = annPrefix + "/readOnlyClone"
	AnnLUKSEncryption       = annPrefix + "/luksEncryption" // import only
	AnnSkipRecoveryQueue    = annPrefix + "/skipRecoveryQueue"
)

var features = map[controllerhelpers.Feature]*versionutils.Version{
	csiConfig.ExpandCSIVolumes: versionutils.MustParseSemantic("1.16.0"),
	csiConfig.CSIBlockVolumes:  versionutils.MustParseSemantic("1.14.0"),
}

var (
	listOpts = metav1.ListOptions{}
	getOpts  = metav1.GetOptions{}
)
