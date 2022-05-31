// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeInfinitevolAttributesType is a structure to represent a volume-infinitevol-attributes ZAPI object
type VolumeInfinitevolAttributesType struct {
	XMLName                        xml.Name                                                `xml:"volume-infinitevol-attributes"`
	ConstituentRolePtr             *ReposConstituentRoleType                               `xml:"constituent-role"`
	EnableSnapdiffPtr              *bool                                                   `xml:"enable-snapdiff"`
	IsManagedByServicePtr          *bool                                                   `xml:"is-managed-by-service"`
	MaxDataConstituentSizePtr      *SizeType                                               `xml:"max-data-constituent-size"`
	MaxNamespaceConstituentSizePtr *SizeType                                               `xml:"max-namespace-constituent-size"`
	NamespaceMirrorAggrListPtr     *VolumeInfinitevolAttributesTypeNamespaceMirrorAggrList `xml:"namespace-mirror-aggr-list"`
	// work in progress
	StorageServicePtr *string `xml:"storage-service"`
}

// NewVolumeInfinitevolAttributesType is a factory method for creating new instances of VolumeInfinitevolAttributesType objects
func NewVolumeInfinitevolAttributesType() *VolumeInfinitevolAttributesType {
	return &VolumeInfinitevolAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeInfinitevolAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeInfinitevolAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// ConstituentRole is a 'getter' method
func (o *VolumeInfinitevolAttributesType) ConstituentRole() ReposConstituentRoleType {
	var r ReposConstituentRoleType
	if o.ConstituentRolePtr == nil {
		return r
	}
	r = *o.ConstituentRolePtr
	return r
}

// SetConstituentRole is a fluent style 'setter' method that can be chained
func (o *VolumeInfinitevolAttributesType) SetConstituentRole(newValue ReposConstituentRoleType) *VolumeInfinitevolAttributesType {
	o.ConstituentRolePtr = &newValue
	return o
}

// EnableSnapdiff is a 'getter' method
func (o *VolumeInfinitevolAttributesType) EnableSnapdiff() bool {
	var r bool
	if o.EnableSnapdiffPtr == nil {
		return r
	}
	r = *o.EnableSnapdiffPtr
	return r
}

// SetEnableSnapdiff is a fluent style 'setter' method that can be chained
func (o *VolumeInfinitevolAttributesType) SetEnableSnapdiff(newValue bool) *VolumeInfinitevolAttributesType {
	o.EnableSnapdiffPtr = &newValue
	return o
}

// IsManagedByService is a 'getter' method
func (o *VolumeInfinitevolAttributesType) IsManagedByService() bool {
	var r bool
	if o.IsManagedByServicePtr == nil {
		return r
	}
	r = *o.IsManagedByServicePtr
	return r
}

// SetIsManagedByService is a fluent style 'setter' method that can be chained
func (o *VolumeInfinitevolAttributesType) SetIsManagedByService(newValue bool) *VolumeInfinitevolAttributesType {
	o.IsManagedByServicePtr = &newValue
	return o
}

// MaxDataConstituentSize is a 'getter' method
func (o *VolumeInfinitevolAttributesType) MaxDataConstituentSize() SizeType {
	var r SizeType
	if o.MaxDataConstituentSizePtr == nil {
		return r
	}
	r = *o.MaxDataConstituentSizePtr
	return r
}

// SetMaxDataConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeInfinitevolAttributesType) SetMaxDataConstituentSize(newValue SizeType) *VolumeInfinitevolAttributesType {
	o.MaxDataConstituentSizePtr = &newValue
	return o
}

// MaxNamespaceConstituentSize is a 'getter' method
func (o *VolumeInfinitevolAttributesType) MaxNamespaceConstituentSize() SizeType {
	var r SizeType
	if o.MaxNamespaceConstituentSizePtr == nil {
		return r
	}
	r = *o.MaxNamespaceConstituentSizePtr
	return r
}

// SetMaxNamespaceConstituentSize is a fluent style 'setter' method that can be chained
func (o *VolumeInfinitevolAttributesType) SetMaxNamespaceConstituentSize(newValue SizeType) *VolumeInfinitevolAttributesType {
	o.MaxNamespaceConstituentSizePtr = &newValue
	return o
}

// VolumeInfinitevolAttributesTypeNamespaceMirrorAggrList is a wrapper
type VolumeInfinitevolAttributesTypeNamespaceMirrorAggrList struct {
	XMLName     xml.Name       `xml:"namespace-mirror-aggr-list"`
	AggrNamePtr []AggrNameType `xml:"aggr-name"`
}

// AggrName is a 'getter' method
func (o *VolumeInfinitevolAttributesTypeNamespaceMirrorAggrList) AggrName() []AggrNameType {
	r := o.AggrNamePtr
	return r
}

// SetAggrName is a fluent style 'setter' method that can be chained
func (o *VolumeInfinitevolAttributesTypeNamespaceMirrorAggrList) SetAggrName(newValue []AggrNameType) *VolumeInfinitevolAttributesTypeNamespaceMirrorAggrList {
	newSlice := make([]AggrNameType, len(newValue))
	copy(newSlice, newValue)
	o.AggrNamePtr = newSlice
	return o
}

// NamespaceMirrorAggrList is a 'getter' method
func (o *VolumeInfinitevolAttributesType) NamespaceMirrorAggrList() VolumeInfinitevolAttributesTypeNamespaceMirrorAggrList {
	var r VolumeInfinitevolAttributesTypeNamespaceMirrorAggrList
	if o.NamespaceMirrorAggrListPtr == nil {
		return r
	}
	r = *o.NamespaceMirrorAggrListPtr
	return r
}

// SetNamespaceMirrorAggrList is a fluent style 'setter' method that can be chained
func (o *VolumeInfinitevolAttributesType) SetNamespaceMirrorAggrList(newValue VolumeInfinitevolAttributesTypeNamespaceMirrorAggrList) *VolumeInfinitevolAttributesType {
	o.NamespaceMirrorAggrListPtr = &newValue
	return o
}

// StorageService is a 'getter' method
func (o *VolumeInfinitevolAttributesType) StorageService() string {
	var r string
	if o.StorageServicePtr == nil {
		return r
	}
	r = *o.StorageServicePtr
	return r
}

// SetStorageService is a fluent style 'setter' method that can be chained
func (o *VolumeInfinitevolAttributesType) SetStorageService(newValue string) *VolumeInfinitevolAttributesType {
	o.StorageServicePtr = &newValue
	return o
}
