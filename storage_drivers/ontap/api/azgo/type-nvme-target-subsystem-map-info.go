// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NvmeTargetSubsystemMapInfoType is a structure to represent a nvme-target-subsystem-map-info ZAPI object
type NvmeTargetSubsystemMapInfoType struct {
	XMLName          xml.Name         `xml:"nvme-target-subsystem-map-info"`
	AnagrpidPtr      *int             `xml:"anagrpid"`
	NamespaceUuidPtr *UuidType        `xml:"namespace-uuid"`
	NsidPtr          *int             `xml:"nsid"`
	PathPtr          *LunPathType     `xml:"path"`
	SubsystemPtr     *string          `xml:"subsystem"`
	SubsystemUuidPtr *UuidType        `xml:"subsystem-uuid"`
	VserverPtr       *VserverNameType `xml:"vserver"`
	VserverUuidPtr   *UuidType        `xml:"vserver-uuid"`
}

// NewNvmeTargetSubsystemMapInfoType is a factory method for creating new instances of NvmeTargetSubsystemMapInfoType objects
func NewNvmeTargetSubsystemMapInfoType() *NvmeTargetSubsystemMapInfoType {
	return &NvmeTargetSubsystemMapInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeTargetSubsystemMapInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeTargetSubsystemMapInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Anagrpid is a 'getter' method
func (o *NvmeTargetSubsystemMapInfoType) Anagrpid() int {
	var r int
	if o.AnagrpidPtr == nil {
		return r
	}
	r = *o.AnagrpidPtr
	return r
}

// SetAnagrpid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemMapInfoType) SetAnagrpid(newValue int) *NvmeTargetSubsystemMapInfoType {
	o.AnagrpidPtr = &newValue
	return o
}

// NamespaceUuid is a 'getter' method
func (o *NvmeTargetSubsystemMapInfoType) NamespaceUuid() UuidType {
	var r UuidType
	if o.NamespaceUuidPtr == nil {
		return r
	}
	r = *o.NamespaceUuidPtr
	return r
}

// SetNamespaceUuid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemMapInfoType) SetNamespaceUuid(newValue UuidType) *NvmeTargetSubsystemMapInfoType {
	o.NamespaceUuidPtr = &newValue
	return o
}

// Nsid is a 'getter' method
func (o *NvmeTargetSubsystemMapInfoType) Nsid() int {
	var r int
	if o.NsidPtr == nil {
		return r
	}
	r = *o.NsidPtr
	return r
}

// SetNsid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemMapInfoType) SetNsid(newValue int) *NvmeTargetSubsystemMapInfoType {
	o.NsidPtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *NvmeTargetSubsystemMapInfoType) Path() LunPathType {
	var r LunPathType
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemMapInfoType) SetPath(newValue LunPathType) *NvmeTargetSubsystemMapInfoType {
	o.PathPtr = &newValue
	return o
}

// Subsystem is a 'getter' method
func (o *NvmeTargetSubsystemMapInfoType) Subsystem() string {
	var r string
	if o.SubsystemPtr == nil {
		return r
	}
	r = *o.SubsystemPtr
	return r
}

// SetSubsystem is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemMapInfoType) SetSubsystem(newValue string) *NvmeTargetSubsystemMapInfoType {
	o.SubsystemPtr = &newValue
	return o
}

// SubsystemUuid is a 'getter' method
func (o *NvmeTargetSubsystemMapInfoType) SubsystemUuid() UuidType {
	var r UuidType
	if o.SubsystemUuidPtr == nil {
		return r
	}
	r = *o.SubsystemUuidPtr
	return r
}

// SetSubsystemUuid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemMapInfoType) SetSubsystemUuid(newValue UuidType) *NvmeTargetSubsystemMapInfoType {
	o.SubsystemUuidPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *NvmeTargetSubsystemMapInfoType) Vserver() VserverNameType {
	var r VserverNameType
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemMapInfoType) SetVserver(newValue VserverNameType) *NvmeTargetSubsystemMapInfoType {
	o.VserverPtr = &newValue
	return o
}

// VserverUuid is a 'getter' method
func (o *NvmeTargetSubsystemMapInfoType) VserverUuid() UuidType {
	var r UuidType
	if o.VserverUuidPtr == nil {
		return r
	}
	r = *o.VserverUuidPtr
	return r
}

// SetVserverUuid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetSubsystemMapInfoType) SetVserverUuid(newValue UuidType) *NvmeTargetSubsystemMapInfoType {
	o.VserverUuidPtr = &newValue
	return o
}
