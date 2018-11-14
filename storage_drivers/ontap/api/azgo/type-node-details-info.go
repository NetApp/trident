package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NodeDetailsInfoType is a structure to represent a node-details-info ZAPI object
type NodeDetailsInfoType struct {
	XMLName                        xml.Name                           `xml:"node-details-info"`
	CpuBusytimePtr                 *int                               `xml:"cpu-busytime"`
	CpuFirmwareReleasePtr          *string                            `xml:"cpu-firmware-release"`
	EnvFailedFanCountPtr           *int                               `xml:"env-failed-fan-count"`
	EnvFailedFanMessagePtr         *string                            `xml:"env-failed-fan-message"`
	EnvFailedPowerSupplyCountPtr   *int                               `xml:"env-failed-power-supply-count"`
	EnvFailedPowerSupplyMessagePtr *string                            `xml:"env-failed-power-supply-message"`
	EnvOverTemperaturePtr          *bool                              `xml:"env-over-temperature"`
	IsAllFlashOptimizedPtr         *bool                              `xml:"is-all-flash-optimized"`
	IsCloudOptimizedPtr            *bool                              `xml:"is-cloud-optimized"`
	IsDiffSvcsPtr                  *bool                              `xml:"is-diff-svcs"`
	IsEpsilonNodePtr               *bool                              `xml:"is-epsilon-node"`
	IsNodeClusterEligiblePtr       *bool                              `xml:"is-node-cluster-eligible"`
	IsNodeHealthyPtr               *bool                              `xml:"is-node-healthy"`
	MaximumAggregateSizePtr        *SizeType                          `xml:"maximum-aggregate-size"`
	MaximumNumberOfVolumesPtr      *int                               `xml:"maximum-number-of-volumes"`
	MaximumVolumeSizePtr           *SizeType                          `xml:"maximum-volume-size"`
	NodePtr                        *NodeNameType                      `xml:"node"`
	NodeAssetTagPtr                *string                            `xml:"node-asset-tag"`
	NodeLocationPtr                *string                            `xml:"node-location"`
	NodeModelPtr                   *string                            `xml:"node-model"`
	NodeNvramIdPtr                 *int                               `xml:"node-nvram-id"`
	NodeOwnerPtr                   *string                            `xml:"node-owner"`
	NodeSerialNumberPtr            *string                            `xml:"node-serial-number"`
	NodeStorageConfigurationPtr    *StorageConfigurationStateEnumType `xml:"node-storage-configuration"`
	NodeSystemIdPtr                *string                            `xml:"node-system-id"`
	NodeUptimePtr                  *int                               `xml:"node-uptime"`
	NodeUuidPtr                    *string                            `xml:"node-uuid"`
	NodeVendorPtr                  *string                            `xml:"node-vendor"`
	NvramBatteryStatusPtr          *NvramBatteryStatusEnumType        `xml:"nvram-battery-status"`
	ProductVersionPtr              *string                            `xml:"product-version"`
	VmSystemDisksPtr               *VmSystemDisksType                 `xml:"vm-system-disks"`
	VmhostInfoPtr                  *VmhostInfoType                    `xml:"vmhost-info"`
}

// NewNodeDetailsInfoType is a factory method for creating new instances of NodeDetailsInfoType objects
func NewNodeDetailsInfoType() *NodeDetailsInfoType {
	return &NodeDetailsInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NodeDetailsInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NodeDetailsInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// CpuBusytime is a 'getter' method
func (o *NodeDetailsInfoType) CpuBusytime() int {
	r := *o.CpuBusytimePtr
	return r
}

// SetCpuBusytime is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetCpuBusytime(newValue int) *NodeDetailsInfoType {
	o.CpuBusytimePtr = &newValue
	return o
}

// CpuFirmwareRelease is a 'getter' method
func (o *NodeDetailsInfoType) CpuFirmwareRelease() string {
	r := *o.CpuFirmwareReleasePtr
	return r
}

// SetCpuFirmwareRelease is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetCpuFirmwareRelease(newValue string) *NodeDetailsInfoType {
	o.CpuFirmwareReleasePtr = &newValue
	return o
}

// EnvFailedFanCount is a 'getter' method
func (o *NodeDetailsInfoType) EnvFailedFanCount() int {
	r := *o.EnvFailedFanCountPtr
	return r
}

// SetEnvFailedFanCount is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetEnvFailedFanCount(newValue int) *NodeDetailsInfoType {
	o.EnvFailedFanCountPtr = &newValue
	return o
}

// EnvFailedFanMessage is a 'getter' method
func (o *NodeDetailsInfoType) EnvFailedFanMessage() string {
	r := *o.EnvFailedFanMessagePtr
	return r
}

// SetEnvFailedFanMessage is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetEnvFailedFanMessage(newValue string) *NodeDetailsInfoType {
	o.EnvFailedFanMessagePtr = &newValue
	return o
}

// EnvFailedPowerSupplyCount is a 'getter' method
func (o *NodeDetailsInfoType) EnvFailedPowerSupplyCount() int {
	r := *o.EnvFailedPowerSupplyCountPtr
	return r
}

// SetEnvFailedPowerSupplyCount is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetEnvFailedPowerSupplyCount(newValue int) *NodeDetailsInfoType {
	o.EnvFailedPowerSupplyCountPtr = &newValue
	return o
}

// EnvFailedPowerSupplyMessage is a 'getter' method
func (o *NodeDetailsInfoType) EnvFailedPowerSupplyMessage() string {
	r := *o.EnvFailedPowerSupplyMessagePtr
	return r
}

// SetEnvFailedPowerSupplyMessage is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetEnvFailedPowerSupplyMessage(newValue string) *NodeDetailsInfoType {
	o.EnvFailedPowerSupplyMessagePtr = &newValue
	return o
}

// EnvOverTemperature is a 'getter' method
func (o *NodeDetailsInfoType) EnvOverTemperature() bool {
	r := *o.EnvOverTemperaturePtr
	return r
}

// SetEnvOverTemperature is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetEnvOverTemperature(newValue bool) *NodeDetailsInfoType {
	o.EnvOverTemperaturePtr = &newValue
	return o
}

// IsAllFlashOptimized is a 'getter' method
func (o *NodeDetailsInfoType) IsAllFlashOptimized() bool {
	r := *o.IsAllFlashOptimizedPtr
	return r
}

// SetIsAllFlashOptimized is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsAllFlashOptimized(newValue bool) *NodeDetailsInfoType {
	o.IsAllFlashOptimizedPtr = &newValue
	return o
}

// IsCloudOptimized is a 'getter' method
func (o *NodeDetailsInfoType) IsCloudOptimized() bool {
	r := *o.IsCloudOptimizedPtr
	return r
}

// SetIsCloudOptimized is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsCloudOptimized(newValue bool) *NodeDetailsInfoType {
	o.IsCloudOptimizedPtr = &newValue
	return o
}

// IsDiffSvcs is a 'getter' method
func (o *NodeDetailsInfoType) IsDiffSvcs() bool {
	r := *o.IsDiffSvcsPtr
	return r
}

// SetIsDiffSvcs is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsDiffSvcs(newValue bool) *NodeDetailsInfoType {
	o.IsDiffSvcsPtr = &newValue
	return o
}

// IsEpsilonNode is a 'getter' method
func (o *NodeDetailsInfoType) IsEpsilonNode() bool {
	r := *o.IsEpsilonNodePtr
	return r
}

// SetIsEpsilonNode is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsEpsilonNode(newValue bool) *NodeDetailsInfoType {
	o.IsEpsilonNodePtr = &newValue
	return o
}

// IsNodeClusterEligible is a 'getter' method
func (o *NodeDetailsInfoType) IsNodeClusterEligible() bool {
	r := *o.IsNodeClusterEligiblePtr
	return r
}

// SetIsNodeClusterEligible is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsNodeClusterEligible(newValue bool) *NodeDetailsInfoType {
	o.IsNodeClusterEligiblePtr = &newValue
	return o
}

// IsNodeHealthy is a 'getter' method
func (o *NodeDetailsInfoType) IsNodeHealthy() bool {
	r := *o.IsNodeHealthyPtr
	return r
}

// SetIsNodeHealthy is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsNodeHealthy(newValue bool) *NodeDetailsInfoType {
	o.IsNodeHealthyPtr = &newValue
	return o
}

// MaximumAggregateSize is a 'getter' method
func (o *NodeDetailsInfoType) MaximumAggregateSize() SizeType {
	r := *o.MaximumAggregateSizePtr
	return r
}

// SetMaximumAggregateSize is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetMaximumAggregateSize(newValue SizeType) *NodeDetailsInfoType {
	o.MaximumAggregateSizePtr = &newValue
	return o
}

// MaximumNumberOfVolumes is a 'getter' method
func (o *NodeDetailsInfoType) MaximumNumberOfVolumes() int {
	r := *o.MaximumNumberOfVolumesPtr
	return r
}

// SetMaximumNumberOfVolumes is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetMaximumNumberOfVolumes(newValue int) *NodeDetailsInfoType {
	o.MaximumNumberOfVolumesPtr = &newValue
	return o
}

// MaximumVolumeSize is a 'getter' method
func (o *NodeDetailsInfoType) MaximumVolumeSize() SizeType {
	r := *o.MaximumVolumeSizePtr
	return r
}

// SetMaximumVolumeSize is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetMaximumVolumeSize(newValue SizeType) *NodeDetailsInfoType {
	o.MaximumVolumeSizePtr = &newValue
	return o
}

// Node is a 'getter' method
func (o *NodeDetailsInfoType) Node() NodeNameType {
	r := *o.NodePtr
	return r
}

// SetNode is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNode(newValue NodeNameType) *NodeDetailsInfoType {
	o.NodePtr = &newValue
	return o
}

// NodeAssetTag is a 'getter' method
func (o *NodeDetailsInfoType) NodeAssetTag() string {
	r := *o.NodeAssetTagPtr
	return r
}

// SetNodeAssetTag is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeAssetTag(newValue string) *NodeDetailsInfoType {
	o.NodeAssetTagPtr = &newValue
	return o
}

// NodeLocation is a 'getter' method
func (o *NodeDetailsInfoType) NodeLocation() string {
	r := *o.NodeLocationPtr
	return r
}

// SetNodeLocation is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeLocation(newValue string) *NodeDetailsInfoType {
	o.NodeLocationPtr = &newValue
	return o
}

// NodeModel is a 'getter' method
func (o *NodeDetailsInfoType) NodeModel() string {
	r := *o.NodeModelPtr
	return r
}

// SetNodeModel is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeModel(newValue string) *NodeDetailsInfoType {
	o.NodeModelPtr = &newValue
	return o
}

// NodeNvramId is a 'getter' method
func (o *NodeDetailsInfoType) NodeNvramId() int {
	r := *o.NodeNvramIdPtr
	return r
}

// SetNodeNvramId is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeNvramId(newValue int) *NodeDetailsInfoType {
	o.NodeNvramIdPtr = &newValue
	return o
}

// NodeOwner is a 'getter' method
func (o *NodeDetailsInfoType) NodeOwner() string {
	r := *o.NodeOwnerPtr
	return r
}

// SetNodeOwner is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeOwner(newValue string) *NodeDetailsInfoType {
	o.NodeOwnerPtr = &newValue
	return o
}

// NodeSerialNumber is a 'getter' method
func (o *NodeDetailsInfoType) NodeSerialNumber() string {
	r := *o.NodeSerialNumberPtr
	return r
}

// SetNodeSerialNumber is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeSerialNumber(newValue string) *NodeDetailsInfoType {
	o.NodeSerialNumberPtr = &newValue
	return o
}

// NodeStorageConfiguration is a 'getter' method
func (o *NodeDetailsInfoType) NodeStorageConfiguration() StorageConfigurationStateEnumType {
	r := *o.NodeStorageConfigurationPtr
	return r
}

// SetNodeStorageConfiguration is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeStorageConfiguration(newValue StorageConfigurationStateEnumType) *NodeDetailsInfoType {
	o.NodeStorageConfigurationPtr = &newValue
	return o
}

// NodeSystemId is a 'getter' method
func (o *NodeDetailsInfoType) NodeSystemId() string {
	r := *o.NodeSystemIdPtr
	return r
}

// SetNodeSystemId is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeSystemId(newValue string) *NodeDetailsInfoType {
	o.NodeSystemIdPtr = &newValue
	return o
}

// NodeUptime is a 'getter' method
func (o *NodeDetailsInfoType) NodeUptime() int {
	r := *o.NodeUptimePtr
	return r
}

// SetNodeUptime is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeUptime(newValue int) *NodeDetailsInfoType {
	o.NodeUptimePtr = &newValue
	return o
}

// NodeUuid is a 'getter' method
func (o *NodeDetailsInfoType) NodeUuid() string {
	r := *o.NodeUuidPtr
	return r
}

// SetNodeUuid is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeUuid(newValue string) *NodeDetailsInfoType {
	o.NodeUuidPtr = &newValue
	return o
}

// NodeVendor is a 'getter' method
func (o *NodeDetailsInfoType) NodeVendor() string {
	r := *o.NodeVendorPtr
	return r
}

// SetNodeVendor is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeVendor(newValue string) *NodeDetailsInfoType {
	o.NodeVendorPtr = &newValue
	return o
}

// NvramBatteryStatus is a 'getter' method
func (o *NodeDetailsInfoType) NvramBatteryStatus() NvramBatteryStatusEnumType {
	r := *o.NvramBatteryStatusPtr
	return r
}

// SetNvramBatteryStatus is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNvramBatteryStatus(newValue NvramBatteryStatusEnumType) *NodeDetailsInfoType {
	o.NvramBatteryStatusPtr = &newValue
	return o
}

// ProductVersion is a 'getter' method
func (o *NodeDetailsInfoType) ProductVersion() string {
	r := *o.ProductVersionPtr
	return r
}

// SetProductVersion is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetProductVersion(newValue string) *NodeDetailsInfoType {
	o.ProductVersionPtr = &newValue
	return o
}

// VmSystemDisks is a 'getter' method
func (o *NodeDetailsInfoType) VmSystemDisks() VmSystemDisksType {
	r := *o.VmSystemDisksPtr
	return r
}

// SetVmSystemDisks is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetVmSystemDisks(newValue VmSystemDisksType) *NodeDetailsInfoType {
	o.VmSystemDisksPtr = &newValue
	return o
}

// VmhostInfo is a 'getter' method
func (o *NodeDetailsInfoType) VmhostInfo() VmhostInfoType {
	r := *o.VmhostInfoPtr
	return r
}

// SetVmhostInfo is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetVmhostInfo(newValue VmhostInfoType) *NodeDetailsInfoType {
	o.VmhostInfoPtr = &newValue
	return o
}
