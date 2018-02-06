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

	// Testing constants
	RecoveryTest     = "recoveryTest"
	UniqueOptions    = "uniqueOptions"
	TestingAttribute = "testingAttribute"
	NonexistentBool  = "nonexistentBool"

	// Values for media
	HDD    = "hdd"
	SSD    = "ssd"
	Hybrid = "hybrid"

	RequiredStorage        = "requiredStorage" // deprecated, use additionalStoragePools
	StoragePools           = "storagePools"
	AdditionalStoragePools = "additionalStoragePools"
)

var attrTypes = map[string]Type{
	IOPS:             intType,
	Snapshots:        boolType,
	Clones:           boolType,
	Encryption:       boolType,
	ProvisioningType: stringType,
	BackendType:      stringType,
	Media:            stringType,
	RecoveryTest:     boolType,
	UniqueOptions:    stringType,
	TestingAttribute: boolType,
	NonexistentBool:  boolType,
}
