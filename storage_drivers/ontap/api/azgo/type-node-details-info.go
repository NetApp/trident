// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
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
	IsAllFlashSelectOptimizedPtr   *bool                              `xml:"is-all-flash-select-optimized"`
	IsCapacityOptimizedPtr         *bool                              `xml:"is-capacity-optimized"`
	IsCloudOptimizedPtr            *bool                              `xml:"is-cloud-optimized"`
	IsDiffSvcsPtr                  *bool                              `xml:"is-diff-svcs"`
	IsEpsilonNodePtr               *bool                              `xml:"is-epsilon-node"`
	IsNodeClusterEligiblePtr       *bool                              `xml:"is-node-cluster-eligible"`
	IsNodeHealthyPtr               *bool                              `xml:"is-node-healthy"`
	IsPerfOptimizedPtr             *bool                              `xml:"is-perf-optimized"`
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
	Sas2Sas3MixedStackSupportPtr   *Sas2Sas3MixedStackSupportType     `xml:"sas2-sas3-mixed-stack-support"`
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
	var r int
	if o.CpuBusytimePtr == nil {
		return r
	}
	r = *o.CpuBusytimePtr
	return r
}

// SetCpuBusytime is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetCpuBusytime(newValue int) *NodeDetailsInfoType {
	o.CpuBusytimePtr = &newValue
	return o
}

// CpuFirmwareRelease is a 'getter' method
func (o *NodeDetailsInfoType) CpuFirmwareRelease() string {
	var r string
	if o.CpuFirmwareReleasePtr == nil {
		return r
	}
	r = *o.CpuFirmwareReleasePtr
	return r
}

// SetCpuFirmwareRelease is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetCpuFirmwareRelease(newValue string) *NodeDetailsInfoType {
	o.CpuFirmwareReleasePtr = &newValue
	return o
}

// EnvFailedFanCount is a 'getter' method
func (o *NodeDetailsInfoType) EnvFailedFanCount() int {
	var r int
	if o.EnvFailedFanCountPtr == nil {
		return r
	}
	r = *o.EnvFailedFanCountPtr
	return r
}

// SetEnvFailedFanCount is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetEnvFailedFanCount(newValue int) *NodeDetailsInfoType {
	o.EnvFailedFanCountPtr = &newValue
	return o
}

// EnvFailedFanMessage is a 'getter' method
func (o *NodeDetailsInfoType) EnvFailedFanMessage() string {
	var r string
	if o.EnvFailedFanMessagePtr == nil {
		return r
	}
	r = *o.EnvFailedFanMessagePtr
	return r
}

// SetEnvFailedFanMessage is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetEnvFailedFanMessage(newValue string) *NodeDetailsInfoType {
	o.EnvFailedFanMessagePtr = &newValue
	return o
}

// EnvFailedPowerSupplyCount is a 'getter' method
func (o *NodeDetailsInfoType) EnvFailedPowerSupplyCount() int {
	var r int
	if o.EnvFailedPowerSupplyCountPtr == nil {
		return r
	}
	r = *o.EnvFailedPowerSupplyCountPtr
	return r
}

// SetEnvFailedPowerSupplyCount is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetEnvFailedPowerSupplyCount(newValue int) *NodeDetailsInfoType {
	o.EnvFailedPowerSupplyCountPtr = &newValue
	return o
}

// EnvFailedPowerSupplyMessage is a 'getter' method
func (o *NodeDetailsInfoType) EnvFailedPowerSupplyMessage() string {
	var r string
	if o.EnvFailedPowerSupplyMessagePtr == nil {
		return r
	}
	r = *o.EnvFailedPowerSupplyMessagePtr
	return r
}

// SetEnvFailedPowerSupplyMessage is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetEnvFailedPowerSupplyMessage(newValue string) *NodeDetailsInfoType {
	o.EnvFailedPowerSupplyMessagePtr = &newValue
	return o
}

// EnvOverTemperature is a 'getter' method
func (o *NodeDetailsInfoType) EnvOverTemperature() bool {
	var r bool
	if o.EnvOverTemperaturePtr == nil {
		return r
	}
	r = *o.EnvOverTemperaturePtr
	return r
}

// SetEnvOverTemperature is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetEnvOverTemperature(newValue bool) *NodeDetailsInfoType {
	o.EnvOverTemperaturePtr = &newValue
	return o
}

// IsAllFlashOptimized is a 'getter' method
func (o *NodeDetailsInfoType) IsAllFlashOptimized() bool {
	var r bool
	if o.IsAllFlashOptimizedPtr == nil {
		return r
	}
	r = *o.IsAllFlashOptimizedPtr
	return r
}

// SetIsAllFlashOptimized is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsAllFlashOptimized(newValue bool) *NodeDetailsInfoType {
	o.IsAllFlashOptimizedPtr = &newValue
	return o
}

// IsAllFlashSelectOptimized is a 'getter' method
func (o *NodeDetailsInfoType) IsAllFlashSelectOptimized() bool {
	var r bool
	if o.IsAllFlashSelectOptimizedPtr == nil {
		return r
	}
	r = *o.IsAllFlashSelectOptimizedPtr
	return r
}

// SetIsAllFlashSelectOptimized is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsAllFlashSelectOptimized(newValue bool) *NodeDetailsInfoType {
	o.IsAllFlashSelectOptimizedPtr = &newValue
	return o
}

// IsCapacityOptimized is a 'getter' method
func (o *NodeDetailsInfoType) IsCapacityOptimized() bool {
	var r bool
	if o.IsCapacityOptimizedPtr == nil {
		return r
	}
	r = *o.IsCapacityOptimizedPtr
	return r
}

// SetIsCapacityOptimized is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsCapacityOptimized(newValue bool) *NodeDetailsInfoType {
	o.IsCapacityOptimizedPtr = &newValue
	return o
}

// IsCloudOptimized is a 'getter' method
func (o *NodeDetailsInfoType) IsCloudOptimized() bool {
	var r bool
	if o.IsCloudOptimizedPtr == nil {
		return r
	}
	r = *o.IsCloudOptimizedPtr
	return r
}

// SetIsCloudOptimized is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsCloudOptimized(newValue bool) *NodeDetailsInfoType {
	o.IsCloudOptimizedPtr = &newValue
	return o
}

// IsDiffSvcs is a 'getter' method
func (o *NodeDetailsInfoType) IsDiffSvcs() bool {
	var r bool
	if o.IsDiffSvcsPtr == nil {
		return r
	}
	r = *o.IsDiffSvcsPtr
	return r
}

// SetIsDiffSvcs is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsDiffSvcs(newValue bool) *NodeDetailsInfoType {
	o.IsDiffSvcsPtr = &newValue
	return o
}

// IsEpsilonNode is a 'getter' method
func (o *NodeDetailsInfoType) IsEpsilonNode() bool {
	var r bool
	if o.IsEpsilonNodePtr == nil {
		return r
	}
	r = *o.IsEpsilonNodePtr
	return r
}

// SetIsEpsilonNode is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsEpsilonNode(newValue bool) *NodeDetailsInfoType {
	o.IsEpsilonNodePtr = &newValue
	return o
}

// IsNodeClusterEligible is a 'getter' method
func (o *NodeDetailsInfoType) IsNodeClusterEligible() bool {
	var r bool
	if o.IsNodeClusterEligiblePtr == nil {
		return r
	}
	r = *o.IsNodeClusterEligiblePtr
	return r
}

// SetIsNodeClusterEligible is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsNodeClusterEligible(newValue bool) *NodeDetailsInfoType {
	o.IsNodeClusterEligiblePtr = &newValue
	return o
}

// IsNodeHealthy is a 'getter' method
func (o *NodeDetailsInfoType) IsNodeHealthy() bool {
	var r bool
	if o.IsNodeHealthyPtr == nil {
		return r
	}
	r = *o.IsNodeHealthyPtr
	return r
}

// SetIsNodeHealthy is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsNodeHealthy(newValue bool) *NodeDetailsInfoType {
	o.IsNodeHealthyPtr = &newValue
	return o
}

// IsPerfOptimized is a 'getter' method
func (o *NodeDetailsInfoType) IsPerfOptimized() bool {
	var r bool
	if o.IsPerfOptimizedPtr == nil {
		return r
	}
	r = *o.IsPerfOptimizedPtr
	return r
}

// SetIsPerfOptimized is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetIsPerfOptimized(newValue bool) *NodeDetailsInfoType {
	o.IsPerfOptimizedPtr = &newValue
	return o
}

// MaximumAggregateSize is a 'getter' method
func (o *NodeDetailsInfoType) MaximumAggregateSize() SizeType {
	var r SizeType
	if o.MaximumAggregateSizePtr == nil {
		return r
	}
	r = *o.MaximumAggregateSizePtr
	return r
}

// SetMaximumAggregateSize is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetMaximumAggregateSize(newValue SizeType) *NodeDetailsInfoType {
	o.MaximumAggregateSizePtr = &newValue
	return o
}

// MaximumNumberOfVolumes is a 'getter' method
func (o *NodeDetailsInfoType) MaximumNumberOfVolumes() int {
	var r int
	if o.MaximumNumberOfVolumesPtr == nil {
		return r
	}
	r = *o.MaximumNumberOfVolumesPtr
	return r
}

// SetMaximumNumberOfVolumes is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetMaximumNumberOfVolumes(newValue int) *NodeDetailsInfoType {
	o.MaximumNumberOfVolumesPtr = &newValue
	return o
}

// MaximumVolumeSize is a 'getter' method
func (o *NodeDetailsInfoType) MaximumVolumeSize() SizeType {
	var r SizeType
	if o.MaximumVolumeSizePtr == nil {
		return r
	}
	r = *o.MaximumVolumeSizePtr
	return r
}

// SetMaximumVolumeSize is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetMaximumVolumeSize(newValue SizeType) *NodeDetailsInfoType {
	o.MaximumVolumeSizePtr = &newValue
	return o
}

// Node is a 'getter' method
func (o *NodeDetailsInfoType) Node() NodeNameType {
	var r NodeNameType
	if o.NodePtr == nil {
		return r
	}
	r = *o.NodePtr
	return r
}

// SetNode is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNode(newValue NodeNameType) *NodeDetailsInfoType {
	o.NodePtr = &newValue
	return o
}

// NodeAssetTag is a 'getter' method
func (o *NodeDetailsInfoType) NodeAssetTag() string {
	var r string
	if o.NodeAssetTagPtr == nil {
		return r
	}
	r = *o.NodeAssetTagPtr
	return r
}

// SetNodeAssetTag is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeAssetTag(newValue string) *NodeDetailsInfoType {
	o.NodeAssetTagPtr = &newValue
	return o
}

// NodeLocation is a 'getter' method
func (o *NodeDetailsInfoType) NodeLocation() string {
	var r string
	if o.NodeLocationPtr == nil {
		return r
	}
	r = *o.NodeLocationPtr
	return r
}

// SetNodeLocation is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeLocation(newValue string) *NodeDetailsInfoType {
	o.NodeLocationPtr = &newValue
	return o
}

// NodeModel is a 'getter' method
func (o *NodeDetailsInfoType) NodeModel() string {
	var r string
	if o.NodeModelPtr == nil {
		return r
	}
	r = *o.NodeModelPtr
	return r
}

// SetNodeModel is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeModel(newValue string) *NodeDetailsInfoType {
	o.NodeModelPtr = &newValue
	return o
}

// NodeNvramId is a 'getter' method
func (o *NodeDetailsInfoType) NodeNvramId() int {
	var r int
	if o.NodeNvramIdPtr == nil {
		return r
	}
	r = *o.NodeNvramIdPtr
	return r
}

// SetNodeNvramId is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeNvramId(newValue int) *NodeDetailsInfoType {
	o.NodeNvramIdPtr = &newValue
	return o
}

// NodeOwner is a 'getter' method
func (o *NodeDetailsInfoType) NodeOwner() string {
	var r string
	if o.NodeOwnerPtr == nil {
		return r
	}
	r = *o.NodeOwnerPtr
	return r
}

// SetNodeOwner is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeOwner(newValue string) *NodeDetailsInfoType {
	o.NodeOwnerPtr = &newValue
	return o
}

// NodeSerialNumber is a 'getter' method
func (o *NodeDetailsInfoType) NodeSerialNumber() string {
	var r string
	if o.NodeSerialNumberPtr == nil {
		return r
	}
	r = *o.NodeSerialNumberPtr
	return r
}

// SetNodeSerialNumber is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeSerialNumber(newValue string) *NodeDetailsInfoType {
	o.NodeSerialNumberPtr = &newValue
	return o
}

// NodeStorageConfiguration is a 'getter' method
func (o *NodeDetailsInfoType) NodeStorageConfiguration() StorageConfigurationStateEnumType {
	var r StorageConfigurationStateEnumType
	if o.NodeStorageConfigurationPtr == nil {
		return r
	}
	r = *o.NodeStorageConfigurationPtr
	return r
}

// SetNodeStorageConfiguration is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeStorageConfiguration(newValue StorageConfigurationStateEnumType) *NodeDetailsInfoType {
	o.NodeStorageConfigurationPtr = &newValue
	return o
}

// NodeSystemId is a 'getter' method
func (o *NodeDetailsInfoType) NodeSystemId() string {
	var r string
	if o.NodeSystemIdPtr == nil {
		return r
	}
	r = *o.NodeSystemIdPtr
	return r
}

// SetNodeSystemId is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeSystemId(newValue string) *NodeDetailsInfoType {
	o.NodeSystemIdPtr = &newValue
	return o
}

// NodeUptime is a 'getter' method
func (o *NodeDetailsInfoType) NodeUptime() int {
	var r int
	if o.NodeUptimePtr == nil {
		return r
	}
	r = *o.NodeUptimePtr
	return r
}

// SetNodeUptime is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeUptime(newValue int) *NodeDetailsInfoType {
	o.NodeUptimePtr = &newValue
	return o
}

// NodeUuid is a 'getter' method
func (o *NodeDetailsInfoType) NodeUuid() string {
	var r string
	if o.NodeUuidPtr == nil {
		return r
	}
	r = *o.NodeUuidPtr
	return r
}

// SetNodeUuid is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeUuid(newValue string) *NodeDetailsInfoType {
	o.NodeUuidPtr = &newValue
	return o
}

// NodeVendor is a 'getter' method
func (o *NodeDetailsInfoType) NodeVendor() string {
	var r string
	if o.NodeVendorPtr == nil {
		return r
	}
	r = *o.NodeVendorPtr
	return r
}

// SetNodeVendor is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNodeVendor(newValue string) *NodeDetailsInfoType {
	o.NodeVendorPtr = &newValue
	return o
}

// NvramBatteryStatus is a 'getter' method
func (o *NodeDetailsInfoType) NvramBatteryStatus() NvramBatteryStatusEnumType {
	var r NvramBatteryStatusEnumType
	if o.NvramBatteryStatusPtr == nil {
		return r
	}
	r = *o.NvramBatteryStatusPtr
	return r
}

// SetNvramBatteryStatus is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetNvramBatteryStatus(newValue NvramBatteryStatusEnumType) *NodeDetailsInfoType {
	o.NvramBatteryStatusPtr = &newValue
	return o
}

// ProductVersion is a 'getter' method
func (o *NodeDetailsInfoType) ProductVersion() string {
	var r string
	if o.ProductVersionPtr == nil {
		return r
	}
	r = *o.ProductVersionPtr
	return r
}

// SetProductVersion is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetProductVersion(newValue string) *NodeDetailsInfoType {
	o.ProductVersionPtr = &newValue
	return o
}

// Sas2Sas3MixedStackSupport is a 'getter' method
func (o *NodeDetailsInfoType) Sas2Sas3MixedStackSupport() Sas2Sas3MixedStackSupportType {
	var r Sas2Sas3MixedStackSupportType
	if o.Sas2Sas3MixedStackSupportPtr == nil {
		return r
	}
	r = *o.Sas2Sas3MixedStackSupportPtr
	return r
}

// SetSas2Sas3MixedStackSupport is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetSas2Sas3MixedStackSupport(newValue Sas2Sas3MixedStackSupportType) *NodeDetailsInfoType {
	o.Sas2Sas3MixedStackSupportPtr = &newValue
	return o
}

// VmSystemDisks is a 'getter' method
func (o *NodeDetailsInfoType) VmSystemDisks() VmSystemDisksType {
	var r VmSystemDisksType
	if o.VmSystemDisksPtr == nil {
		return r
	}
	r = *o.VmSystemDisksPtr
	return r
}

// SetVmSystemDisks is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetVmSystemDisks(newValue VmSystemDisksType) *NodeDetailsInfoType {
	o.VmSystemDisksPtr = &newValue
	return o
}

// VmhostInfo is a 'getter' method
func (o *NodeDetailsInfoType) VmhostInfo() VmhostInfoType {
	var r VmhostInfoType
	if o.VmhostInfoPtr == nil {
		return r
	}
	r = *o.VmhostInfoPtr
	return r
}

// SetVmhostInfo is a fluent style 'setter' method that can be chained
func (o *NodeDetailsInfoType) SetVmhostInfo(newValue VmhostInfoType) *NodeDetailsInfoType {
	o.VmhostInfoPtr = &newValue
	return o
}
