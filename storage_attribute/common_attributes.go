// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storageattribute

const (
	// Constants for integer storage category attributes
	IOPS = "IOPS"

	// Constants for boolean storage category attributes
	Snapshots  = "snapshots"
	Clones     = "clones"
	Encryption = "encryption"

	// Constants for string list attributes
	ProvisioningType = "provisioningType"
	BackendType      = "backendType"
	Media            = "media"
	Region           = "region"
	Zone             = "zone"

	// Constants for label attributes
	Labels   = "labels"
	Selector = "selector"

	// Testing constants
	RecoveryTest     = "recoveryTest"
	UniqueOptions    = "uniqueOptions"
	TestingAttribute = "testingAttribute"
	NonexistentBool  = "nonexistentBool"

	// Values for media
	HDD    = "hdd"
	SSD    = "ssd"
	Hybrid = "hybrid"

	// Values for provisioning type
	Thick = "thick"
	Thin  = "thin"

	RequiredStorage        = "requiredStorage" // deprecated, use additionalStoragePools
	StoragePools           = "storagePools"
	AdditionalStoragePools = "additionalStoragePools"
	ExcludeStoragePools    = "excludeStoragePools"
)

var attrTypes = map[string]Type{
	IOPS:             intType,
	Snapshots:        boolType,
	Clones:           boolType,
	Encryption:       boolType,
	ProvisioningType: stringType,
	BackendType:      stringType,
	Media:            stringType,
	Region:           stringType,
	Zone:             stringType,
	Labels:           labelType,
	Selector:         labelType,
	RecoveryTest:     boolType,
	UniqueOptions:    stringType,
	TestingAttribute: boolType,
	NonexistentBool:  boolType,
}
