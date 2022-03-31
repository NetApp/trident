// Copyright 2022 NetApp, Inc. All Rights Reserved.

package storagedrivers

// ConfigVersion is the expected version specified in the config file
const ConfigVersion = 1

// Default storage prefix
const DefaultDockerStoragePrefix = "netappdvp_"
const DefaultTridentStoragePrefix = "trident_"

// Default SAN igroup / host group names
const DefaultDockerIgroupName = "netappdvp"
const DefaultTridentIgroupName = "trident"

// Storage driver names specified in the config file, etc.
const (
	OntapNASStorageDriverName          = "ontap-nas"
	OntapNASFlexGroupStorageDriverName = "ontap-nas-flexgroup"
	OntapNASQtreeStorageDriverName     = "ontap-nas-economy"
	OntapSANStorageDriverName          = "ontap-san"
	OntapSANEconomyStorageDriverName   = "ontap-san-economy"
	SolidfireSANStorageDriverName      = "solidfire-san"
	AWSNFSStorageDriverName            = "aws-cvs"
	AzureNFSStorageDriverName          = "azure-netapp-files"
	GCPNFSStorageDriverName            = "gcp-cvs"
	AstraDSStorageDriverName           = "astrads-nas"
	FakeStorageDriverName              = "fake"
)

// Filesystem types
const (
	FsXfs  = "xfs"
	FsExt3 = "ext3"
	FsExt4 = "ext4"
	FsRaw  = "raw"
)

// Default Filesystem value
const DefaultFileSystemType = FsExt4

const UnsetPool = ""
const DefaultVolumeSize = "1G"

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
