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
	EseriesIscsiStorageDriverName      = "eseries-iscsi"
	OntapNASStorageDriverName          = "ontap-nas"
	OntapNASFlexGroupStorageDriverName = "ontap-nas-flexgroup"
	OntapNASQtreeStorageDriverName     = "ontap-nas-economy"
	OntapSANStorageDriverName          = "ontap-san"
	SolidfireSANStorageDriverName      = "solidfire-san"
	FakeStorageDriverName              = "fake"
)

const UnsetPool = ""
const DefaultVolumeSize = "1G"
