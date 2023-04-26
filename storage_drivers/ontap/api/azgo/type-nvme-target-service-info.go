// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeTargetServiceInfoType is a structure to represent a nvme-target-service-info ZAPI object
type NvmeTargetServiceInfoType struct {
	XMLName        xml.Name         `xml:"nvme-target-service-info"`
	IsAvailablePtr *bool            `xml:"is-available"`
	VserverPtr     *VserverNameType `xml:"vserver"`
	VserverUuidPtr *UuidType        `xml:"vserver-uuid"`
}

// NewNvmeTargetServiceInfoType is a factory method for creating new instances of NvmeTargetServiceInfoType objects
func NewNvmeTargetServiceInfoType() *NvmeTargetServiceInfoType {
	return &NvmeTargetServiceInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeTargetServiceInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeTargetServiceInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// IsAvailable is a 'getter' method
func (o *NvmeTargetServiceInfoType) IsAvailable() bool {
	var r bool
	if o.IsAvailablePtr == nil {
		return r
	}
	r = *o.IsAvailablePtr
	return r
}

// SetIsAvailable is a fluent style 'setter' method that can be chained
func (o *NvmeTargetServiceInfoType) SetIsAvailable(newValue bool) *NvmeTargetServiceInfoType {
	o.IsAvailablePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *NvmeTargetServiceInfoType) Vserver() VserverNameType {
	var r VserverNameType
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *NvmeTargetServiceInfoType) SetVserver(newValue VserverNameType) *NvmeTargetServiceInfoType {
	o.VserverPtr = &newValue
	return o
}

// VserverUuid is a 'getter' method
func (o *NvmeTargetServiceInfoType) VserverUuid() UuidType {
	var r UuidType
	if o.VserverUuidPtr == nil {
		return r
	}
	r = *o.VserverUuidPtr
	return r
}

// SetVserverUuid is a fluent style 'setter' method that can be chained
func (o *NvmeTargetServiceInfoType) SetVserverUuid(newValue UuidType) *NvmeTargetServiceInfoType {
	o.VserverUuidPtr = &newValue
	return o
}
