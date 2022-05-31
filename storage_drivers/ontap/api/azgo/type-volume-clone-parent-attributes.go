// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeCloneParentAttributesType is a structure to represent a volume-clone-parent-attributes ZAPI object
type VolumeCloneParentAttributesType struct {
	XMLName         xml.Name         `xml:"volume-clone-parent-attributes"`
	DsidPtr         *int             `xml:"dsid"`
	MsidPtr         *int             `xml:"msid"`
	NamePtr         *VolumeNameType  `xml:"name"`
	SnapshotIdPtr   *int             `xml:"snapshot-id"`
	SnapshotNamePtr *string          `xml:"snapshot-name"`
	UuidPtr         *UuidType        `xml:"uuid"`
	VserverNamePtr  *VserverNameType `xml:"vserver-name"`
}

// NewVolumeCloneParentAttributesType is a factory method for creating new instances of VolumeCloneParentAttributesType objects
func NewVolumeCloneParentAttributesType() *VolumeCloneParentAttributesType {
	return &VolumeCloneParentAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneParentAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneParentAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Dsid is a 'getter' method
func (o *VolumeCloneParentAttributesType) Dsid() int {
	var r int
	if o.DsidPtr == nil {
		return r
	}
	r = *o.DsidPtr
	return r
}

// SetDsid is a fluent style 'setter' method that can be chained
func (o *VolumeCloneParentAttributesType) SetDsid(newValue int) *VolumeCloneParentAttributesType {
	o.DsidPtr = &newValue
	return o
}

// Msid is a 'getter' method
func (o *VolumeCloneParentAttributesType) Msid() int {
	var r int
	if o.MsidPtr == nil {
		return r
	}
	r = *o.MsidPtr
	return r
}

// SetMsid is a fluent style 'setter' method that can be chained
func (o *VolumeCloneParentAttributesType) SetMsid(newValue int) *VolumeCloneParentAttributesType {
	o.MsidPtr = &newValue
	return o
}

// Name is a 'getter' method
func (o *VolumeCloneParentAttributesType) Name() VolumeNameType {
	var r VolumeNameType
	if o.NamePtr == nil {
		return r
	}
	r = *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *VolumeCloneParentAttributesType) SetName(newValue VolumeNameType) *VolumeCloneParentAttributesType {
	o.NamePtr = &newValue
	return o
}

// SnapshotId is a 'getter' method
func (o *VolumeCloneParentAttributesType) SnapshotId() int {
	var r int
	if o.SnapshotIdPtr == nil {
		return r
	}
	r = *o.SnapshotIdPtr
	return r
}

// SetSnapshotId is a fluent style 'setter' method that can be chained
func (o *VolumeCloneParentAttributesType) SetSnapshotId(newValue int) *VolumeCloneParentAttributesType {
	o.SnapshotIdPtr = &newValue
	return o
}

// SnapshotName is a 'getter' method
func (o *VolumeCloneParentAttributesType) SnapshotName() string {
	var r string
	if o.SnapshotNamePtr == nil {
		return r
	}
	r = *o.SnapshotNamePtr
	return r
}

// SetSnapshotName is a fluent style 'setter' method that can be chained
func (o *VolumeCloneParentAttributesType) SetSnapshotName(newValue string) *VolumeCloneParentAttributesType {
	o.SnapshotNamePtr = &newValue
	return o
}

// Uuid is a 'getter' method
func (o *VolumeCloneParentAttributesType) Uuid() UuidType {
	var r UuidType
	if o.UuidPtr == nil {
		return r
	}
	r = *o.UuidPtr
	return r
}

// SetUuid is a fluent style 'setter' method that can be chained
func (o *VolumeCloneParentAttributesType) SetUuid(newValue UuidType) *VolumeCloneParentAttributesType {
	o.UuidPtr = &newValue
	return o
}

// VserverName is a 'getter' method
func (o *VolumeCloneParentAttributesType) VserverName() VserverNameType {
	var r VserverNameType
	if o.VserverNamePtr == nil {
		return r
	}
	r = *o.VserverNamePtr
	return r
}

// SetVserverName is a fluent style 'setter' method that can be chained
func (o *VolumeCloneParentAttributesType) SetVserverName(newValue VserverNameType) *VolumeCloneParentAttributesType {
	o.VserverNamePtr = &newValue
	return o
}
