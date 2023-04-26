// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeTargetSubsystemControllerInfoType is a structure to represent a nvme-target-subsystem-controller-info ZAPI object
type NvmeTargetSubsystemControllerInfoType struct {
	XMLName                      xml.Name                                           `xml:"nvme-target-subsystem-controller-info"`
	AdminQueueDepthPtr           *int                                               `xml:"admin-queue-depth"`
	ControllerIdPtr              *int                                               `xml:"controller-id"`
	DataDigestEnabledPtr         *bool                                              `xml:"data-digest-enabled"`
	HeaderDigestEnabledPtr       *bool                                              `xml:"header-digest-enabled"`
	HostNqnPtr                   *string                                            `xml:"host-nqn"`
	InitiatorTransportAddressPtr *string                                            `xml:"initiator-transport-address"`
	IoQueueCountPtr              *int                                               `xml:"io-queue-count"`
	IoQueueDepthPtr              *NvmeTargetSubsystemControllerInfoTypeIoQueueDepth `xml:"io-queue-depth"`
	// work in progress
	KatoPtr              *int               `xml:"kato"`
	LifPtr               *string            `xml:"lif"`
	LifUuidPtr           *UuidType          `xml:"lif-uuid"`
	MaxIoSizePtr         *int               `xml:"max-io-size"`
	NodePtr              *FilerIdType       `xml:"node"`
	SubsystemPtr         *string            `xml:"subsystem"`
	SubsystemUuidPtr     *UuidType          `xml:"subsystem-uuid"`
	TransportProtocolPtr *NvmeTransportType `xml:"transport-protocol"`
	TrsvcidPtr           *string            `xml:"trsvcid"`
	VserverPtr           *VserverNameType   `xml:"vserver"`
	VserverUuidPtr       *UuidType          `xml:"vserver-uuid"`
}

// NewNvmeTargetSubsystemControllerInfoType is a factory method for creating new instances of NvmeTargetSubsystemControllerInfoType objects
func NewNvmeTargetSubsystemControllerInfoType() *NvmeTargetSubsystemControllerInfoType {
	return &NvmeTargetSubsystemControllerInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeTargetSubsystemControllerInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeTargetSubsystemControllerInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AdminQueueDepth is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) AdminQueueDepth() int {
	var r int
	if o.AdminQueueDepthPtr == nil {
		return r
	}
	r = *o.AdminQueueDepthPtr
	return r
}

// SetAdminQueueDepth is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetAdminQueueDepth(newValue int) *NvmeTargetSubsystemControllerInfoType {
	o.AdminQueueDepthPtr = &newValue
	return o
}

// ControllerId is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) ControllerId() int {
	var r int
	if o.ControllerIdPtr == nil {
		return r
	}
	r = *o.ControllerIdPtr
	return r
}

// SetControllerId is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetControllerId(newValue int) *NvmeTargetSubsystemControllerInfoType {
	o.ControllerIdPtr = &newValue
	return o
}

// DataDigestEnabled is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) DataDigestEnabled() bool {
	var r bool
	if o.DataDigestEnabledPtr == nil {
		return r
	}
	r = *o.DataDigestEnabledPtr
	return r
}

// SetDataDigestEnabled is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetDataDigestEnabled(newValue bool) *NvmeTargetSubsystemControllerInfoType {
	o.DataDigestEnabledPtr = &newValue
	return o
}

// HeaderDigestEnabled is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) HeaderDigestEnabled() bool {
	var r bool
	if o.HeaderDigestEnabledPtr == nil {
		return r
	}
	r = *o.HeaderDigestEnabledPtr
	return r
}

// SetHeaderDigestEnabled is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetHeaderDigestEnabled(newValue bool) *NvmeTargetSubsystemControllerInfoType {
	o.HeaderDigestEnabledPtr = &newValue
	return o
}

// HostNqn is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) HostNqn() string {
	var r string
	if o.HostNqnPtr == nil {
		return r
	}
	r = *o.HostNqnPtr
	return r
}

// SetHostNqn is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetHostNqn(newValue string) *NvmeTargetSubsystemControllerInfoType {
	o.HostNqnPtr = &newValue
	return o
}

// InitiatorTransportAddress is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) InitiatorTransportAddress() string {
	var r string
	if o.InitiatorTransportAddressPtr == nil {
		return r
	}
	r = *o.InitiatorTransportAddressPtr
	return r
}

// SetInitiatorTransportAddress is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetInitiatorTransportAddress(newValue string) *NvmeTargetSubsystemControllerInfoType {
	o.InitiatorTransportAddressPtr = &newValue
	return o
}

// IoQueueCount is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) IoQueueCount() int {
	var r int
	if o.IoQueueCountPtr == nil {
		return r
	}
	r = *o.IoQueueCountPtr
	return r
}

// SetIoQueueCount is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetIoQueueCount(newValue int) *NvmeTargetSubsystemControllerInfoType {
	o.IoQueueCountPtr = &newValue
	return o
}

// NvmeTargetSubsystemControllerInfoTypeIoQueueDepth is a wrapper
type NvmeTargetSubsystemControllerInfoTypeIoQueueDepth struct {
	XMLName    xml.Name `xml:"io-queue-depth"`
	IntegerPtr []int    `xml:"integer"`
}

// Integer is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoTypeIoQueueDepth) Integer() []int {
	r := o.IntegerPtr
	return r
}

// SetInteger is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoTypeIoQueueDepth) SetInteger(newValue []int) *NvmeTargetSubsystemControllerInfoTypeIoQueueDepth {
	newSlice := make([]int, len(newValue))
	copy(newSlice, newValue)
	o.IntegerPtr = newSlice
	return o
}

// IoQueueDepth is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) IoQueueDepth() NvmeTargetSubsystemControllerInfoTypeIoQueueDepth {
	var r NvmeTargetSubsystemControllerInfoTypeIoQueueDepth
	if o.IoQueueDepthPtr == nil {
		return r
	}
	r = *o.IoQueueDepthPtr
	return r
}

// SetIoQueueDepth is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetIoQueueDepth(newValue NvmeTargetSubsystemControllerInfoTypeIoQueueDepth) *NvmeTargetSubsystemControllerInfoType {
	o.IoQueueDepthPtr = &newValue
	return o
}

// Kato is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) Kato() int {
	var r int
	if o.KatoPtr == nil {
		return r
	}
	r = *o.KatoPtr
	return r
}

// SetKato is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetKato(newValue int) *NvmeTargetSubsystemControllerInfoType {
	o.KatoPtr = &newValue
	return o
}

// Lif is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) Lif() string {
	var r string
	if o.LifPtr == nil {
		return r
	}
	r = *o.LifPtr
	return r
}

// SetLif is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetLif(newValue string) *NvmeTargetSubsystemControllerInfoType {
	o.LifPtr = &newValue
	return o
}

// LifUuid is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) LifUuid() UuidType {
	var r UuidType
	if o.LifUuidPtr == nil {
		return r
	}
	r = *o.LifUuidPtr
	return r
}

// SetLifUuid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetLifUuid(newValue UuidType) *NvmeTargetSubsystemControllerInfoType {
	o.LifUuidPtr = &newValue
	return o
}

// MaxIoSize is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) MaxIoSize() int {
	var r int
	if o.MaxIoSizePtr == nil {
		return r
	}
	r = *o.MaxIoSizePtr
	return r
}

// SetMaxIoSize is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetMaxIoSize(newValue int) *NvmeTargetSubsystemControllerInfoType {
	o.MaxIoSizePtr = &newValue
	return o
}

// Node is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) Node() FilerIdType {
	var r FilerIdType
	if o.NodePtr == nil {
		return r
	}
	r = *o.NodePtr
	return r
}

// SetNode is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetNode(newValue FilerIdType) *NvmeTargetSubsystemControllerInfoType {
	o.NodePtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetSubsystem(newValue string) *NvmeTargetSubsystemControllerInfoType {
	o.SubsystemPtr = &newValue
	return o
}

// SubsystemUuid is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) SubsystemUuid() UuidType {
	var r UuidType
	if o.SubsystemUuidPtr == nil {
		return r
	}
	r = *o.SubsystemUuidPtr
	return r
}

// SetSubsystemUuid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetSubsystemUuid(newValue UuidType) *NvmeTargetSubsystemControllerInfoType {
	o.SubsystemUuidPtr = &newValue
	return o
}

// TransportProtocol is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) TransportProtocol() NvmeTransportType {
	var r NvmeTransportType
	if o.TransportProtocolPtr == nil {
		return r
	}
	r = *o.TransportProtocolPtr
	return r
}

// SetTransportProtocol is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetTransportProtocol(newValue NvmeTransportType) *NvmeTargetSubsystemControllerInfoType {
	o.TransportProtocolPtr = &newValue
	return o
}

// Trsvcid is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) Trsvcid() string {
	var r string
	if o.TrsvcidPtr == nil {
		return r
	}
	r = *o.TrsvcidPtr
	return r
}

// SetTrsvcid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetTrsvcid(newValue string) *NvmeTargetSubsystemControllerInfoType {
	o.TrsvcidPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) Vserver() VserverNameType {
	var r VserverNameType
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetVserver(newValue VserverNameType) *NvmeTargetSubsystemControllerInfoType {
	o.VserverPtr = &newValue
	return o
}

// VserverUuid is a 'getter' method
func (o *NvmeTargetSubsystemControllerInfoType) VserverUuid() UuidType {
	var r UuidType
	if o.VserverUuidPtr == nil {
		return r
	}
	r = *o.VserverUuidPtr
	return r
}

// SetVserverUuid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemControllerInfoType) SetVserverUuid(newValue UuidType) *NvmeTargetSubsystemControllerInfoType {
	o.VserverUuidPtr = &newValue
	return o
}
