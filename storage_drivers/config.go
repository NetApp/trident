// Copyright 2022 NetApp, Inc. All Rights Reserved.

package storagedrivers

import "github.com/netapp/trident/config"

// ConfigVersion is the expected version specified in the config file
const ConfigVersion = 1

// Default storage prefix
const (
	DefaultDockerStoragePrefix  = "netappdvp_"
	DefaultTridentStoragePrefix = "trident_"
)

// Default SAN igroup / host group names
const (
	DefaultDockerIgroupName  = "netappdvp"
	DefaultTridentIgroupName = "trident"
)

// Storage driver names specified in the config file, etc.
const (
	OntapNASStorageDriverName          = "ontap-nas"
	OntapNASFlexGroupStorageDriverName = "ontap-nas-flexgroup"
	OntapNASQtreeStorageDriverName     = "ontap-nas-economy"
	OntapSANStorageDriverName          = "ontap-san"
	OntapSANEconomyStorageDriverName   = "ontap-san-economy"
	SolidfireSANStorageDriverName      = "solidfire-san"
	AzureNASStorageDriverName          = "azure-netapp-files"
	AzureNASBlockStorageDriverName     = "azure-netapp-files-subvolume"
	GCPNFSStorageDriverName            = "gcp-cvs"
	FakeStorageDriverName              = "fake"
)

// Default Filesystem value
const DefaultFileSystemType = config.FsExt4

const (
	UnsetPool         = ""
	DefaultVolumeSize = "1G"
)

// Volume label names
const TridentLabelTag = "trident"

// Topology label names
const TopologyLabelPrefix = "topology.kubernetes.io"

// Backend Credentials specific
type CredentialStore string

const (
	CredentialStoreK8sSecret CredentialStore = "secret"

	KeyName string = "name"
	KeyType string = "type"
)

// Mount options managed by drivers
const MountOptionNoUUID = "nouuid"
