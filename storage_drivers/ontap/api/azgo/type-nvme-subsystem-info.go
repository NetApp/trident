// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeSubsystemInfoType is a structure to represent a nvme-subsystem-info ZAPI object
type NvmeSubsystemInfoType struct {
	XMLName                xml.Name             `xml:"nvme-subsystem-info"`
	CommentPtr             *string              `xml:"comment"`
	DefaultIoQueueCountPtr *int                 `xml:"default-io-queue-count"`
	DefaultIoQueueDepthPtr *int                 `xml:"default-io-queue-depth"`
	DeleteOnUnmapPtr       *bool                `xml:"delete-on-unmap"`
	OstypePtr              *NvmeSubsystemOsType `xml:"ostype"`
	SerialNumberPtr        *string              `xml:"serial-number"`
	SubsystemPtr           *string              `xml:"subsystem"`
	TargetNqnPtr           *string              `xml:"target-nqn"`
	UuidPtr                *UuidType            `xml:"uuid"`
	VserverPtr             *VserverNameType     `xml:"vserver"`
	VserverUuidPtr         *UuidType            `xml:"vserver-uuid"`
}

// NewNvmeSubsystemInfoType is a factory method for creating new instances of NvmeSubsystemInfoType objects
func NewNvmeSubsystemInfoType() *NvmeSubsystemInfoType {
	return &NvmeSubsystemInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Comment is a 'getter' method
func (o *NvmeSubsystemInfoType) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemInfoType) SetComment(newValue string) *NvmeSubsystemInfoType {
	o.CommentPtr = &newValue
	return o
}

// DefaultIoQueueCount is a 'getter' method
func (o *NvmeSubsystemInfoType) DefaultIoQueueCount() int {
	var r int
	if o.DefaultIoQueueCountPtr == nil {
		return r
	}
	r = *o.DefaultIoQueueCountPtr
	return r
}

// SetDefaultIoQueueCount is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemInfoType) SetDefaultIoQueueCount(newValue int) *NvmeSubsystemInfoType {
	o.DefaultIoQueueCountPtr = &newValue
	return o
}

// DefaultIoQueueDepth is a 'getter' method
func (o *NvmeSubsystemInfoType) DefaultIoQueueDepth() int {
	var r int
	if o.DefaultIoQueueDepthPtr == nil {
		return r
	}
	r = *o.DefaultIoQueueDepthPtr
	return r
}

// SetDefaultIoQueueDepth is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemInfoType) SetDefaultIoQueueDepth(newValue int) *NvmeSubsystemInfoType {
	o.DefaultIoQueueDepthPtr = &newValue
	return o
}

// DeleteOnUnmap is a 'getter' method
func (o *NvmeSubsystemInfoType) DeleteOnUnmap() bool {
	var r bool
	if o.DeleteOnUnmapPtr == nil {
		return r
	}
	r = *o.DeleteOnUnmapPtr
	return r
}

// SetDeleteOnUnmap is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemInfoType) SetDeleteOnUnmap(newValue bool) *NvmeSubsystemInfoType {
	o.DeleteOnUnmapPtr = &newValue
	return o
}

// Ostype is a 'getter' method
func (o *NvmeSubsystemInfoType) Ostype() NvmeSubsystemOsType {
	var r NvmeSubsystemOsType
	if o.OstypePtr == nil {
		return r
	}
	r = *o.OstypePtr
	return r
}

// SetOstype is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemInfoType) SetOstype(newValue NvmeSubsystemOsType) *NvmeSubsystemInfoType {
	o.OstypePtr = &newValue
	return o
}

// SerialNumber is a 'getter' method
func (o *NvmeSubsystemInfoType) SerialNumber() string {
	var r string
	if o.SerialNumberPtr == nil {
		return r
	}
	r = *o.SerialNumberPtr
	return r
}

// SetSerialNumber is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemInfoType) SetSerialNumber(newValue string) *NvmeSubsystemInfoType {
	o.SerialNumberPtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeSubsystemInfoType) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemInfoType) SetSubsystem(newValue string) *NvmeSubsystemInfoType {
	o.SubsystemPtr = &newValue
	return o
}

// TargetNqn is a 'getter' method
func (o *NvmeSubsystemInfoType) TargetNqn() string {
	var r string
	if o.TargetNqnPtr == nil {
		return r
	}
	r = *o.TargetNqnPtr
	return r
}

// SetTargetNqn is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemInfoType) SetTargetNqn(newValue string) *NvmeSubsystemInfoType {
	o.TargetNqnPtr = &newValue
	return o
}

// Uuid is a 'getter' method
func (o *NvmeSubsystemInfoType) Uuid() UuidType {
	var r UuidType
	if o.UuidPtr == nil {
		return r
	}
	r = *o.UuidPtr
	return r
}

// SetUuid is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemInfoType) SetUuid(newValue UuidType) *NvmeSubsystemInfoType {
	o.UuidPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *NvmeSubsystemInfoType) Vserver() VserverNameType {
	var r VserverNameType
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemInfoType) SetVserver(newValue VserverNameType) *NvmeSubsystemInfoType {
	o.VserverPtr = &newValue
	return o
}

// VserverUuid is a 'getter' method
func (o *NvmeSubsystemInfoType) VserverUuid() UuidType {
	var r UuidType
	if o.VserverUuidPtr == nil {
		return r
	}
	r = *o.VserverUuidPtr
	return r
}

// SetVserverUuid is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemInfoType) SetVserverUuid(newValue UuidType) *NvmeSubsystemInfoType {
	o.VserverUuidPtr = &newValue
	return o
}
