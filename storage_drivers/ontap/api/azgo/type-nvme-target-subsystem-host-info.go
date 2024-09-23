// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeTargetSubsystemHostInfoType is a structure to represent a nvme-target-subsystem-host-info ZAPI object
type NvmeTargetSubsystemHostInfoType struct {
	XMLName          xml.Name         `xml:"nvme-target-subsystem-host-info"`
	HostNqnPtr       *string          `xml:"host-nqn"`
	IoQueueCountPtr  *int             `xml:"io-queue-count"`
	IoQueueDepthPtr  *int             `xml:"io-queue-depth"`
	SubsystemPtr     *string          `xml:"subsystem"`
	SubsystemUuidPtr *UuidType        `xml:"subsystem-uuid"`
	VserverPtr       *VserverNameType `xml:"vserver"`
	VserverUuidPtr   *UuidType        `xml:"vserver-uuid"`
}

// NewNvmeTargetSubsystemHostInfoType is a factory method for creating new instances of NvmeTargetSubsystemHostInfoType objects
func NewNvmeTargetSubsystemHostInfoType() *NvmeTargetSubsystemHostInfoType {
	return &NvmeTargetSubsystemHostInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeTargetSubsystemHostInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeTargetSubsystemHostInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// HostNqn is a 'getter' method
func (o *NvmeTargetSubsystemHostInfoType) HostNqn() string {
	var r string
	if o.HostNqnPtr == nil {
		return r
	}
	r = *o.HostNqnPtr
	return r
}

// SetHostNqn is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemHostInfoType) SetHostNqn(newValue string) *NvmeTargetSubsystemHostInfoType {
	o.HostNqnPtr = &newValue
	return o
}

// IoQueueCount is a 'getter' method
func (o *NvmeTargetSubsystemHostInfoType) IoQueueCount() int {
	var r int
	if o.IoQueueCountPtr == nil {
		return r
	}
	r = *o.IoQueueCountPtr
	return r
}

// SetIoQueueCount is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemHostInfoType) SetIoQueueCount(newValue int) *NvmeTargetSubsystemHostInfoType {
	o.IoQueueCountPtr = &newValue
	return o
}

// IoQueueDepth is a 'getter' method
func (o *NvmeTargetSubsystemHostInfoType) IoQueueDepth() int {
	var r int
	if o.IoQueueDepthPtr == nil {
		return r
	}
	r = *o.IoQueueDepthPtr
	return r
}

// SetIoQueueDepth is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemHostInfoType) SetIoQueueDepth(newValue int) *NvmeTargetSubsystemHostInfoType {
	o.IoQueueDepthPtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeTargetSubsystemHostInfoType) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemHostInfoType) SetSubsystem(newValue string) *NvmeTargetSubsystemHostInfoType {
	o.SubsystemPtr = &newValue
	return o
}

// SubsystemUuid is a 'getter' method
func (o *NvmeTargetSubsystemHostInfoType) SubsystemUuid() UuidType {
	var r UuidType
	if o.SubsystemUuidPtr == nil {
		return r
	}
	r = *o.SubsystemUuidPtr
	return r
}

// SetSubsystemUuid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemHostInfoType) SetSubsystemUuid(newValue UuidType) *NvmeTargetSubsystemHostInfoType {
	o.SubsystemUuidPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *NvmeTargetSubsystemHostInfoType) Vserver() VserverNameType {
	var r VserverNameType
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemHostInfoType) SetVserver(newValue VserverNameType) *NvmeTargetSubsystemHostInfoType {
	o.VserverPtr = &newValue
	return o
}

// VserverUuid is a 'getter' method
func (o *NvmeTargetSubsystemHostInfoType) VserverUuid() UuidType {
	var r UuidType
	if o.VserverUuidPtr == nil {
		return r
	}
	r = *o.VserverUuidPtr
	return r
}

// SetVserverUuid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemHostInfoType) SetVserverUuid(newValue UuidType) *NvmeTargetSubsystemHostInfoType {
	o.VserverUuidPtr = &newValue
	return o
}
