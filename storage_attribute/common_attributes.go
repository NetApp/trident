// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage_attribute

const (
	// Constants for integer storage category attributes
	IOPS = "IOPS"

	// Constants for boolean storage category attributes
	Snapshots  = "snapshots"
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

	BackendStoragePools = "requiredStorage"
)

var attrTypes = map[string]StorageAttributeType{
	IOPS:             intType,
	Snapshots:        boolType,
	Encryption:       boolType,
	ProvisioningType: stringType,
	BackendType:      stringType,
	Media:            stringType,
	RecoveryTest:     boolType,
	UniqueOptions:    stringType,
	TestingAttribute: boolType,
	NonexistentBool:  boolType,
}
