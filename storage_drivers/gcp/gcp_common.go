// Copyright 2022 NetApp, Inc. All Rights Reserved.

package gcp

import (
	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/gcp/api"
)

const (
	MinimumVolumeSizeBytes = uint64(1073741824) // 1 GiB

	defaultNfsMountOptions = "nfsvers=3"
	defaultSecurityStyle   = "unix"
	defaultSnapshotDir     = "false"
	defaultSnapshotReserve = ""
	defaultUnixPermissions = "0777"
	defaultStorageClass    = api.StorageClassHardware
	defaultLimitVolumeSize = ""
	defaultExportRule      = "0.0.0.0/0"

	// Constants for internal pool attributes

	Size            = "size"
	ServiceLevel    = "serviceLevel"
	SnapshotDir     = "snapshotDir"
	SnapshotReserve = "snapshotReserve"
	ExportRule      = "exportRule"
	Network         = "network"
	Region          = "region"
	Zone            = "zone"
	StorageClass    = "storageClass"
	UnixPermissions = "unixPermissions"
	StoragePools    = "storagePools"

	// Topology label names
	topologyZoneLabel   = drivers.TopologyLabelPrefix + "/" + Zone
	topologyRegionLabel = drivers.TopologyLabelPrefix + "/" + Region
)

type Telemetry struct {
	tridentconfig.Telemetry
	Plugin string `json:"plugin"`
}
