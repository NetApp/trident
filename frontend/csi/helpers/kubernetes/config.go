// Copyright 2019 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"time"

	"github.com/netapp/trident/config"
)

const (
	CacheSyncPeriod         = 60 * time.Second
	PreSyncCacheWaitPeriod  = 10 * time.Second
	PostSyncCacheWaitPeriod = 30 * time.Second
	ResizeSyncPeriod        = 3 * time.Minute

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
	AnnStorageProvisioner     = "volume.beta.kubernetes.io/storage-provisioner"

	// Orchestrator-defined annotations
	annPrefix          = config.OrchestratorName + ".netapp.io"
	AnnProtocol        = annPrefix + "/protocol"
	AnnSnapshotPolicy  = annPrefix + "/snapshotPolicy"
	AnnSnapshotReserve = annPrefix + "/snapshotReserve"
	AnnSnapshotDir     = annPrefix + "/snapshotDirectory"
	AnnUnixPermissions = annPrefix + "/unixPermissions"
	AnnExportPolicy    = annPrefix + "/exportPolicy"
	AnnBlockSize       = annPrefix + "/blockSize"
	AnnFileSystem      = annPrefix + "/fileSystem"
	AnnCloneFromPVC    = annPrefix + "/cloneFromPVC"
	AnnSplitOnClone    = annPrefix + "/splitOnClone"
	AnnNotManaged      = annPrefix + "/notManaged"
)
