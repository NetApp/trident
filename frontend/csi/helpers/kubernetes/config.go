// Copyright 2019 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/v21/config"
	csiConfig "github.com/netapp/trident/v21/frontend/csi"
	"github.com/netapp/trident/v21/frontend/csi/helpers"
	"github.com/netapp/trident/v21/utils"
)

const (
	CacheSyncPeriod         = 60 * time.Second
	PreSyncCacheWaitPeriod  = 10 * time.Second
	PostSyncCacheWaitPeriod = 30 * time.Second
	ResizeSyncPeriod        = 3 * time.Minute
	PVDeleteWaitPeriod      = 30 * time.Second
	PodDeleteWaitPeriod     = 60 * time.Second
	ImportPVCacheWaitPeriod = 180 * time.Second

	CacheBackoffInitialInterval     = 1 * time.Second
	CacheBackoffRandomizationFactor = 0.1
	CacheBackoffMultiplier          = 1.414
	CacheBackoffMaxInterval         = 5 * time.Second

	// Kubernetes-defined storage class parameters
	K8sFsType = "fsType"

	// Kubernetes-defined annotations
	// (Based on kubernetes/pkg/controller/volume/persistentvolume/controller.go)
	AnnClass                  = "volume.beta.kubernetes.io/storage-class"
	AnnDynamicallyProvisioned = "pv.kubernetes.io/provisioned-by"
	AnnBindCompleted          = "pv.kubernetes.io/bind-completed"
	AnnStorageProvisioner     = "volume.beta.kubernetes.io/storage-provisioner"

	// Kubernetes-defined finalizers
	FinalizerPVProtection = "kubernetes.io/pv-protection"

	// Orchestrator-defined annotations
	annPrefix             = config.OrchestratorName + ".netapp.io"
	AnnProtocol           = annPrefix + "/protocol"
	AnnSnapshotPolicy     = annPrefix + "/snapshotPolicy"
	AnnSnapshotReserve    = annPrefix + "/snapshotReserve"
	AnnSnapshotDir        = annPrefix + "/snapshotDirectory"
	AnnUnixPermissions    = annPrefix + "/unixPermissions"
	AnnExportPolicy       = annPrefix + "/exportPolicy"
	AnnBlockSize          = annPrefix + "/blockSize"
	AnnFileSystem         = annPrefix + "/fileSystem"
	AnnCloneFromPVC       = annPrefix + "/cloneFromPVC"
	AnnSplitOnClone       = annPrefix + "/splitOnClone"
	AnnNotManaged         = annPrefix + "/notManaged"
	AnnImportOriginalName = annPrefix + "/importOriginalName"
	AnnImportBackendUUID  = annPrefix + "/importBackendUUID"
	AnnMirrorRelationship = annPrefix + "/mirrorRelationship"
)

var features = map[helpers.Feature]*utils.Version{
	csiConfig.ExpandCSIVolumes: utils.MustParseSemantic("1.16.0"),
	csiConfig.CSIBlockVolumes:  utils.MustParseSemantic("1.14.0"),
}

var (
	listOpts   = metav1.ListOptions{}
	getOpts    = metav1.GetOptions{}
	createOpts = metav1.CreateOptions{}
	updateOpts = metav1.UpdateOptions{}
	patchOpts  = metav1.PatchOptions{}
	deleteOpts = metav1.DeleteOptions{}
)
