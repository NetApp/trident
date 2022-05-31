// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VmhostInfoType is a structure to represent a vmhost-info ZAPI object
type VmhostInfoType struct {
	XMLName                  xml.Name `xml:"vmhost-info"`
	VmCustomMaxCapacityPtr   *int     `xml:"vm-custom-max-capacity"`
	VmUuidPtr                *string  `xml:"vm-uuid"`
	VmhostBiosReleaseDatePtr *string  `xml:"vmhost-bios-release-date"`
	VmhostBiosVersionPtr     *string  `xml:"vmhost-bios-version"`
	VmhostBootTimePtr        *string  `xml:"vmhost-boot-time"`
	VmhostCpuClockRatePtr    *int     `xml:"vmhost-cpu-clock-rate"`
	VmhostCpuCoreCountPtr    *int     `xml:"vmhost-cpu-core-count"`
	VmhostCpuSocketCountPtr  *int     `xml:"vmhost-cpu-socket-count"`
	VmhostCpuThreadCountPtr  *int     `xml:"vmhost-cpu-thread-count"`
	VmhostErrorPtr           *string  `xml:"vmhost-error"`
	VmhostGatewayPtr         *string  `xml:"vmhost-gateway"`
	VmhostHardwareVendorPtr  *string  `xml:"vmhost-hardware-vendor"`
	VmhostHypervisorPtr      *string  `xml:"vmhost-hypervisor"`
	VmhostIpAddressPtr       *string  `xml:"vmhost-ip-address"`
	VmhostMemoryPtr          *int     `xml:"vmhost-memory"`
	VmhostModelPtr           *string  `xml:"vmhost-model"`
	VmhostNamePtr            *string  `xml:"vmhost-name"`
	VmhostNetmaskPtr         *string  `xml:"vmhost-netmask"`
	VmhostProcessorIdPtr     *string  `xml:"vmhost-processor-id"`
	VmhostProcessorTypePtr   *string  `xml:"vmhost-processor-type"`
	VmhostSoftwareVendorPtr  *string  `xml:"vmhost-software-vendor"`
	VmhostUuidPtr            *string  `xml:"vmhost-uuid"`
}

// NewVmhostInfoType is a factory method for creating new instances of VmhostInfoType objects
func NewVmhostInfoType() *VmhostInfoType {
	return &VmhostInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *VmhostInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VmhostInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// VmCustomMaxCapacity is a 'getter' method
func (o *VmhostInfoType) VmCustomMaxCapacity() int {
	var r int
	if o.VmCustomMaxCapacityPtr == nil {
		return r
	}
	r = *o.VmCustomMaxCapacityPtr
	return r
}

// SetVmCustomMaxCapacity is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmCustomMaxCapacity(newValue int) *VmhostInfoType {
	o.VmCustomMaxCapacityPtr = &newValue
	return o
}

// VmUuid is a 'getter' method
func (o *VmhostInfoType) VmUuid() string {
	var r string
	if o.VmUuidPtr == nil {
		return r
	}
	r = *o.VmUuidPtr
	return r
}

// SetVmUuid is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmUuid(newValue string) *VmhostInfoType {
	o.VmUuidPtr = &newValue
	return o
}

// VmhostBiosReleaseDate is a 'getter' method
func (o *VmhostInfoType) VmhostBiosReleaseDate() string {
	var r string
	if o.VmhostBiosReleaseDatePtr == nil {
		return r
	}
	r = *o.VmhostBiosReleaseDatePtr
	return r
}

// SetVmhostBiosReleaseDate is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostBiosReleaseDate(newValue string) *VmhostInfoType {
	o.VmhostBiosReleaseDatePtr = &newValue
	return o
}

// VmhostBiosVersion is a 'getter' method
func (o *VmhostInfoType) VmhostBiosVersion() string {
	var r string
	if o.VmhostBiosVersionPtr == nil {
		return r
	}
	r = *o.VmhostBiosVersionPtr
	return r
}

// SetVmhostBiosVersion is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostBiosVersion(newValue string) *VmhostInfoType {
	o.VmhostBiosVersionPtr = &newValue
	return o
}

// VmhostBootTime is a 'getter' method
func (o *VmhostInfoType) VmhostBootTime() string {
	var r string
	if o.VmhostBootTimePtr == nil {
		return r
	}
	r = *o.VmhostBootTimePtr
	return r
}

// SetVmhostBootTime is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostBootTime(newValue string) *VmhostInfoType {
	o.VmhostBootTimePtr = &newValue
	return o
}

// VmhostCpuClockRate is a 'getter' method
func (o *VmhostInfoType) VmhostCpuClockRate() int {
	var r int
	if o.VmhostCpuClockRatePtr == nil {
		return r
	}
	r = *o.VmhostCpuClockRatePtr
	return r
}

// SetVmhostCpuClockRate is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostCpuClockRate(newValue int) *VmhostInfoType {
	o.VmhostCpuClockRatePtr = &newValue
	return o
}

// VmhostCpuCoreCount is a 'getter' method
func (o *VmhostInfoType) VmhostCpuCoreCount() int {
	var r int
	if o.VmhostCpuCoreCountPtr == nil {
		return r
	}
	r = *o.VmhostCpuCoreCountPtr
	return r
}

// SetVmhostCpuCoreCount is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostCpuCoreCount(newValue int) *VmhostInfoType {
	o.VmhostCpuCoreCountPtr = &newValue
	return o
}

// VmhostCpuSocketCount is a 'getter' method
func (o *VmhostInfoType) VmhostCpuSocketCount() int {
	var r int
	if o.VmhostCpuSocketCountPtr == nil {
		return r
	}
	r = *o.VmhostCpuSocketCountPtr
	return r
}

// SetVmhostCpuSocketCount is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostCpuSocketCount(newValue int) *VmhostInfoType {
	o.VmhostCpuSocketCountPtr = &newValue
	return o
}

// VmhostCpuThreadCount is a 'getter' method
func (o *VmhostInfoType) VmhostCpuThreadCount() int {
	var r int
	if o.VmhostCpuThreadCountPtr == nil {
		return r
	}
	r = *o.VmhostCpuThreadCountPtr
	return r
}

// SetVmhostCpuThreadCount is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostCpuThreadCount(newValue int) *VmhostInfoType {
	o.VmhostCpuThreadCountPtr = &newValue
	return o
}

// VmhostError is a 'getter' method
func (o *VmhostInfoType) VmhostError() string {
	var r string
	if o.VmhostErrorPtr == nil {
		return r
	}
	r = *o.VmhostErrorPtr
	return r
}

// SetVmhostError is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostError(newValue string) *VmhostInfoType {
	o.VmhostErrorPtr = &newValue
	return o
}

// VmhostGateway is a 'getter' method
func (o *VmhostInfoType) VmhostGateway() string {
	var r string
	if o.VmhostGatewayPtr == nil {
		return r
	}
	r = *o.VmhostGatewayPtr
	return r
}

// SetVmhostGateway is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostGateway(newValue string) *VmhostInfoType {
	o.VmhostGatewayPtr = &newValue
	return o
}

// VmhostHardwareVendor is a 'getter' method
func (o *VmhostInfoType) VmhostHardwareVendor() string {
	var r string
	if o.VmhostHardwareVendorPtr == nil {
		return r
	}
	r = *o.VmhostHardwareVendorPtr
	return r
}

// SetVmhostHardwareVendor is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostHardwareVendor(newValue string) *VmhostInfoType {
	o.VmhostHardwareVendorPtr = &newValue
	return o
}

// VmhostHypervisor is a 'getter' method
func (o *VmhostInfoType) VmhostHypervisor() string {
	var r string
	if o.VmhostHypervisorPtr == nil {
		return r
	}
	r = *o.VmhostHypervisorPtr
	return r
}

// SetVmhostHypervisor is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostHypervisor(newValue string) *VmhostInfoType {
	o.VmhostHypervisorPtr = &newValue
	return o
}

// VmhostIpAddress is a 'getter' method
func (o *VmhostInfoType) VmhostIpAddress() string {
	var r string
	if o.VmhostIpAddressPtr == nil {
		return r
	}
	r = *o.VmhostIpAddressPtr
	return r
}

// SetVmhostIpAddress is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostIpAddress(newValue string) *VmhostInfoType {
	o.VmhostIpAddressPtr = &newValue
	return o
}

// VmhostMemory is a 'getter' method
func (o *VmhostInfoType) VmhostMemory() int {
	var r int
	if o.VmhostMemoryPtr == nil {
		return r
	}
	r = *o.VmhostMemoryPtr
	return r
}

// SetVmhostMemory is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostMemory(newValue int) *VmhostInfoType {
	o.VmhostMemoryPtr = &newValue
	return o
}

// VmhostModel is a 'getter' method
func (o *VmhostInfoType) VmhostModel() string {
	var r string
	if o.VmhostModelPtr == nil {
		return r
	}
	r = *o.VmhostModelPtr
	return r
}

// SetVmhostModel is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostModel(newValue string) *VmhostInfoType {
	o.VmhostModelPtr = &newValue
	return o
}

// VmhostName is a 'getter' method
func (o *VmhostInfoType) VmhostName() string {
	var r string
	if o.VmhostNamePtr == nil {
		return r
	}
	r = *o.VmhostNamePtr
	return r
}

// SetVmhostName is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostName(newValue string) *VmhostInfoType {
	o.VmhostNamePtr = &newValue
	return o
}

// VmhostNetmask is a 'getter' method
func (o *VmhostInfoType) VmhostNetmask() string {
	var r string
	if o.VmhostNetmaskPtr == nil {
		return r
	}
	r = *o.VmhostNetmaskPtr
	return r
}

// SetVmhostNetmask is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostNetmask(newValue string) *VmhostInfoType {
	o.VmhostNetmaskPtr = &newValue
	return o
}

// VmhostProcessorId is a 'getter' method
func (o *VmhostInfoType) VmhostProcessorId() string {
	var r string
	if o.VmhostProcessorIdPtr == nil {
		return r
	}
	r = *o.VmhostProcessorIdPtr
	return r
}

// SetVmhostProcessorId is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostProcessorId(newValue string) *VmhostInfoType {
	o.VmhostProcessorIdPtr = &newValue
	return o
}

// VmhostProcessorType is a 'getter' method
func (o *VmhostInfoType) VmhostProcessorType() string {
	var r string
	if o.VmhostProcessorTypePtr == nil {
		return r
	}
	r = *o.VmhostProcessorTypePtr
	return r
}

// SetVmhostProcessorType is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostProcessorType(newValue string) *VmhostInfoType {
	o.VmhostProcessorTypePtr = &newValue
	return o
}

// VmhostSoftwareVendor is a 'getter' method
func (o *VmhostInfoType) VmhostSoftwareVendor() string {
	var r string
	if o.VmhostSoftwareVendorPtr == nil {
		return r
	}
	r = *o.VmhostSoftwareVendorPtr
	return r
}

// SetVmhostSoftwareVendor is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostSoftwareVendor(newValue string) *VmhostInfoType {
	o.VmhostSoftwareVendorPtr = &newValue
	return o
}

// VmhostUuid is a 'getter' method
func (o *VmhostInfoType) VmhostUuid() string {
	var r string
	if o.VmhostUuidPtr == nil {
		return r
	}
	r = *o.VmhostUuidPtr
	return r
}

// SetVmhostUuid is a fluent style 'setter' method that can be chained
func (o *VmhostInfoType) SetVmhostUuid(newValue string) *VmhostInfoType {
	o.VmhostUuidPtr = &newValue
	return o
}
