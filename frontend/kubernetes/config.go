// Copyright 2018 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"time"

	"github.com/netapp/trident/config"
)

const (
	KubernetesSyncPeriod = 60 * time.Second

	// Kubernetes-defined storage class parameters
	K8sFsType = "fsType"

	// Kubernetes-defined annotations
	// (Based on kubernetes/pkg/controller/volume/persistentvolume/controller.go)
	AnnClass                  = "volume.beta.kubernetes.io/storage-class"
	AnnDynamicallyProvisioned = "pv.kubernetes.io/provisioned-by"
	AnnStorageProvisioner     = "volume.beta.kubernetes.io/storage-provisioner"
	AnnDefaultStorageClass    = "storageclass.kubernetes.io/is-default-class"
	AnnMountOptions           = "volume.beta.kubernetes.io/mount-options"

	// Orchestrator-defined annotations
	AnnOrchestrator    = "netapp.io/" + config.OrchestratorName
	AnnPrefix          = config.OrchestratorName + ".netapp.io"
	AnnReclaimPolicy   = AnnPrefix + "/reclaimPolicy"
	AnnProtocol        = AnnPrefix + "/protocol"
	AnnSpaceReserve    = AnnPrefix + "/spaceReserve"
	AnnSnapshotPolicy  = AnnPrefix + "/snapshotPolicy"
	AnnSnapshotDir     = AnnPrefix + "/snapshotDirectory"
	AnnUnixPermissions = AnnPrefix + "/unixPermissions"
	AnnVendor          = AnnPrefix + "/vendor"
	AnnBackendID       = AnnPrefix + "/backendID"
	AnnExportPolicy    = AnnPrefix + "/exportPolicy"
	AnnBlockSize       = AnnPrefix + "/blockSize"
	AnnFileSystem      = AnnPrefix + "/fileSystem"
	AnnCloneFromPVC    = AnnPrefix + "/cloneFromPVC"
	AnnSplitOnClone    = AnnPrefix + "/splitOnClone"
)
