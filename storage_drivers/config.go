// Copyright 2022 NetApp, Inc. All Rights Reserved.

package storagedrivers

// Backend Credentials specific
type CredentialStore string

// ConfigVersion is the expected version specified in the config file
const (
	ConfigVersion = 1

	// Default storage prefix
	DefaultDockerStoragePrefix  = "netappdvp_"
	DefaultTridentStoragePrefix = "trident_"

	// Default SAN igroup / host group names
	DefaultDockerIgroupName  = "netappdvp"
	DefaultTridentIgroupName = "trident"

	// Default Filesystem value, duplicated from config/config.go because of import dependencies.
	DefaultFileSystemType = "ext4"

	UnsetPool         = ""
	DefaultVolumeSize = "1G"

	// Volume label names
	TridentLabelTag = "trident"

	// Topology label names
	TopologyLabelPrefix = "topology.kubernetes.io"

	CredentialStoreK8sSecret CredentialStore = "secret"
	CredentialStoreAWSARN    CredentialStore = "awsarn"

	KeyName string = "name"
	KeyType string = "type"

	// Mount options managed by drivers
	MountOptionNoUUID = "nouuid"
)
