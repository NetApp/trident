// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SnapmirrorDestinationInfoType is a structure to represent a snapmirror-destination-info ZAPI object
type SnapmirrorDestinationInfoType struct {
	XMLName           xml.Name                                     `xml:"snapmirror-destination-info"`
	CgItemMappingsPtr *SnapmirrorDestinationInfoTypeCgItemMappings `xml:"cg-item-mappings"`
	// work in progress
	DestinationLocationPtr   *string `xml:"destination-location"`
	DestinationVolumePtr     *string `xml:"destination-volume"`
	DestinationVserverPtr    *string `xml:"destination-vserver"`
	IsConstituentPtr         *bool   `xml:"is-constituent"`
	PolicyTypePtr            *string `xml:"policy-type"`
	ProgressLastUpdatedPtr   *uint   `xml:"progress-last-updated"`
	RelationshipGroupTypePtr *string `xml:"relationship-group-type"`
	RelationshipIdPtr        *string `xml:"relationship-id"`
	RelationshipStatusPtr    *string `xml:"relationship-status"`
	RelationshipTypePtr      *string `xml:"relationship-type"`
	SourceLocationPtr        *string `xml:"source-location"`
	SourceVolumePtr          *string `xml:"source-volume"`
	SourceVolumeNodePtr      *string `xml:"source-volume-node"`
	SourceVserverPtr         *string `xml:"source-vserver"`
	TransferProgressPtr      *uint64 `xml:"transfer-progress"`
}

// NewSnapmirrorDestinationInfoType is a factory method for creating new instances of SnapmirrorDestinationInfoType objects
func NewSnapmirrorDestinationInfoType() *SnapmirrorDestinationInfoType {
	return &SnapmirrorDestinationInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorDestinationInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorDestinationInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorDestinationInfoTypeCgItemMappings is a wrapper
type SnapmirrorDestinationInfoTypeCgItemMappings struct {
	XMLName   xml.Name `xml:"cg-item-mappings"`
	StringPtr []string `xml:"string"`
}

// String is a 'getter' method
func (o *SnapmirrorDestinationInfoTypeCgItemMappings) String() []string {
	r := o.StringPtr
	return r
}

// SetString is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoTypeCgItemMappings) SetString(newValue []string) *SnapmirrorDestinationInfoTypeCgItemMappings {
	newSlice := make([]string, len(newValue))
	copy(newSlice, newValue)
	o.StringPtr = newSlice
	return o
}

// CgItemMappings is a 'getter' method
func (o *SnapmirrorDestinationInfoType) CgItemMappings() SnapmirrorDestinationInfoTypeCgItemMappings {
	var r SnapmirrorDestinationInfoTypeCgItemMappings
	if o.CgItemMappingsPtr == nil {
		return r
	}
	r = *o.CgItemMappingsPtr
	return r
}

// SetCgItemMappings is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetCgItemMappings(newValue SnapmirrorDestinationInfoTypeCgItemMappings) *SnapmirrorDestinationInfoType {
	o.CgItemMappingsPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorDestinationInfoType) DestinationLocation() string {
	var r string
	if o.DestinationLocationPtr == nil {
		return r
	}
	r = *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetDestinationLocation(newValue string) *SnapmirrorDestinationInfoType {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorDestinationInfoType) DestinationVolume() string {
	var r string
	if o.DestinationVolumePtr == nil {
		return r
	}
	r = *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetDestinationVolume(newValue string) *SnapmirrorDestinationInfoType {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorDestinationInfoType) DestinationVserver() string {
	var r string
	if o.DestinationVserverPtr == nil {
		return r
	}
	r = *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetDestinationVserver(newValue string) *SnapmirrorDestinationInfoType {
	o.DestinationVserverPtr = &newValue
	return o
}

// IsConstituent is a 'getter' method
func (o *SnapmirrorDestinationInfoType) IsConstituent() bool {
	var r bool
	if o.IsConstituentPtr == nil {
		return r
	}
	r = *o.IsConstituentPtr
	return r
}

// SetIsConstituent is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetIsConstituent(newValue bool) *SnapmirrorDestinationInfoType {
	o.IsConstituentPtr = &newValue
	return o
}

// PolicyType is a 'getter' method
func (o *SnapmirrorDestinationInfoType) PolicyType() string {
	var r string
	if o.PolicyTypePtr == nil {
		return r
	}
	r = *o.PolicyTypePtr
	return r
}

// SetPolicyType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetPolicyType(newValue string) *SnapmirrorDestinationInfoType {
	o.PolicyTypePtr = &newValue
	return o
}

// ProgressLastUpdated is a 'getter' method
func (o *SnapmirrorDestinationInfoType) ProgressLastUpdated() uint {
	var r uint
	if o.ProgressLastUpdatedPtr == nil {
		return r
	}
	r = *o.ProgressLastUpdatedPtr
	return r
}

// SetProgressLastUpdated is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetProgressLastUpdated(newValue uint) *SnapmirrorDestinationInfoType {
	o.ProgressLastUpdatedPtr = &newValue
	return o
}

// RelationshipGroupType is a 'getter' method
func (o *SnapmirrorDestinationInfoType) RelationshipGroupType() string {
	var r string
	if o.RelationshipGroupTypePtr == nil {
		return r
	}
	r = *o.RelationshipGroupTypePtr
	return r
}

// SetRelationshipGroupType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetRelationshipGroupType(newValue string) *SnapmirrorDestinationInfoType {
	o.RelationshipGroupTypePtr = &newValue
	return o
}

// RelationshipId is a 'getter' method
func (o *SnapmirrorDestinationInfoType) RelationshipId() string {
	var r string
	if o.RelationshipIdPtr == nil {
		return r
	}
	r = *o.RelationshipIdPtr
	return r
}

// SetRelationshipId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetRelationshipId(newValue string) *SnapmirrorDestinationInfoType {
	o.RelationshipIdPtr = &newValue
	return o
}

// RelationshipStatus is a 'getter' method
func (o *SnapmirrorDestinationInfoType) RelationshipStatus() string {
	var r string
	if o.RelationshipStatusPtr == nil {
		return r
	}
	r = *o.RelationshipStatusPtr
	return r
}

// SetRelationshipStatus is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetRelationshipStatus(newValue string) *SnapmirrorDestinationInfoType {
	o.RelationshipStatusPtr = &newValue
	return o
}

// RelationshipType is a 'getter' method
func (o *SnapmirrorDestinationInfoType) RelationshipType() string {
	var r string
	if o.RelationshipTypePtr == nil {
		return r
	}
	r = *o.RelationshipTypePtr
	return r
}

// SetRelationshipType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetRelationshipType(newValue string) *SnapmirrorDestinationInfoType {
	o.RelationshipTypePtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorDestinationInfoType) SourceLocation() string {
	var r string
	if o.SourceLocationPtr == nil {
		return r
	}
	r = *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetSourceLocation(newValue string) *SnapmirrorDestinationInfoType {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorDestinationInfoType) SourceVolume() string {
	var r string
	if o.SourceVolumePtr == nil {
		return r
	}
	r = *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetSourceVolume(newValue string) *SnapmirrorDestinationInfoType {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVolumeNode is a 'getter' method
func (o *SnapmirrorDestinationInfoType) SourceVolumeNode() string {
	var r string
	if o.SourceVolumeNodePtr == nil {
		return r
	}
	r = *o.SourceVolumeNodePtr
	return r
}

// SetSourceVolumeNode is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetSourceVolumeNode(newValue string) *SnapmirrorDestinationInfoType {
	o.SourceVolumeNodePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorDestinationInfoType) SourceVserver() string {
	var r string
	if o.SourceVserverPtr == nil {
		return r
	}
	r = *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetSourceVserver(newValue string) *SnapmirrorDestinationInfoType {
	o.SourceVserverPtr = &newValue
	return o
}

// TransferProgress is a 'getter' method
func (o *SnapmirrorDestinationInfoType) TransferProgress() uint64 {
	var r uint64
	if o.TransferProgressPtr == nil {
		return r
	}
	r = *o.TransferProgressPtr
	return r
}

// SetTransferProgress is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestinationInfoType) SetTransferProgress(newValue uint64) *SnapmirrorDestinationInfoType {
	o.TransferProgressPtr = &newValue
	return o
}
