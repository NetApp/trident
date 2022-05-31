// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// ShowAggregatesType is a structure to represent a show-aggregates ZAPI object
type ShowAggregatesType struct {
	XMLName          xml.Name           `xml:"show-aggregates"`
	AggregateNamePtr *AggrNameType      `xml:"aggregate-name"`
	AggregateTypePtr *AggregatetypeType `xml:"aggregate-type"`
	AvailableSizePtr *SizeType          `xml:"available-size"`
	IsNveCapablePtr  *bool              `xml:"is-nve-capable"`
	SnaplockTypePtr  *SnaplocktypeType  `xml:"snaplock-type"`
	VserverNamePtr   *string            `xml:"vserver-name"`
}

// NewShowAggregatesType is a factory method for creating new instances of ShowAggregatesType objects
func NewShowAggregatesType() *ShowAggregatesType {
	return &ShowAggregatesType{}
}

// ToXML converts this object into an xml string representation
func (o *ShowAggregatesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ShowAggregatesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggregateName is a 'getter' method
func (o *ShowAggregatesType) AggregateName() AggrNameType {
	var r AggrNameType
	if o.AggregateNamePtr == nil {
		return r
	}
	r = *o.AggregateNamePtr
	return r
}

// SetAggregateName is a fluent style 'setter' method that can be chained
func (o *ShowAggregatesType) SetAggregateName(newValue AggrNameType) *ShowAggregatesType {
	o.AggregateNamePtr = &newValue
	return o
}

// AggregateType is a 'getter' method
func (o *ShowAggregatesType) AggregateType() AggregatetypeType {
	var r AggregatetypeType
	if o.AggregateTypePtr == nil {
		return r
	}
	r = *o.AggregateTypePtr
	return r
}

// SetAggregateType is a fluent style 'setter' method that can be chained
func (o *ShowAggregatesType) SetAggregateType(newValue AggregatetypeType) *ShowAggregatesType {
	o.AggregateTypePtr = &newValue
	return o
}

// AvailableSize is a 'getter' method
func (o *ShowAggregatesType) AvailableSize() SizeType {
	var r SizeType
	if o.AvailableSizePtr == nil {
		return r
	}
	r = *o.AvailableSizePtr
	return r
}

// SetAvailableSize is a fluent style 'setter' method that can be chained
func (o *ShowAggregatesType) SetAvailableSize(newValue SizeType) *ShowAggregatesType {
	o.AvailableSizePtr = &newValue
	return o
}

// IsNveCapable is a 'getter' method
func (o *ShowAggregatesType) IsNveCapable() bool {
	var r bool
	if o.IsNveCapablePtr == nil {
		return r
	}
	r = *o.IsNveCapablePtr
	return r
}

// SetIsNveCapable is a fluent style 'setter' method that can be chained
func (o *ShowAggregatesType) SetIsNveCapable(newValue bool) *ShowAggregatesType {
	o.IsNveCapablePtr = &newValue
	return o
}

// SnaplockType is a 'getter' method
func (o *ShowAggregatesType) SnaplockType() SnaplocktypeType {
	var r SnaplocktypeType
	if o.SnaplockTypePtr == nil {
		return r
	}
	r = *o.SnaplockTypePtr
	return r
}

// SetSnaplockType is a fluent style 'setter' method that can be chained
func (o *ShowAggregatesType) SetSnaplockType(newValue SnaplocktypeType) *ShowAggregatesType {
	o.SnaplockTypePtr = &newValue
	return o
}

// VserverName is a 'getter' method
func (o *ShowAggregatesType) VserverName() string {
	var r string
	if o.VserverNamePtr == nil {
		return r
	}
	r = *o.VserverNamePtr
	return r
}

// SetVserverName is a fluent style 'setter' method that can be chained
func (o *ShowAggregatesType) SetVserverName(newValue string) *ShowAggregatesType {
	o.VserverNamePtr = &newValue
	return o
}
