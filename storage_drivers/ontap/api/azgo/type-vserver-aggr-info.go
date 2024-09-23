// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VserverAggrInfoType is a structure to represent a vserver-aggr-info ZAPI object
type VserverAggrInfoType struct {
	XMLName               xml.Name      `xml:"vserver-aggr-info"`
	AggrAvailsizePtr      *SizeType     `xml:"aggr-availsize"`
	AggrIsCftPrecommitPtr *bool         `xml:"aggr-is-cft-precommit"`
	AggrNamePtr           *AggrNameType `xml:"aggr-name"`
}

// NewVserverAggrInfoType is a factory method for creating new instances of VserverAggrInfoType objects
func NewVserverAggrInfoType() *VserverAggrInfoType {
	return &VserverAggrInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *VserverAggrInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverAggrInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggrAvailsize is a 'getter' method
func (o *VserverAggrInfoType) AggrAvailsize() SizeType {
	var r SizeType
	if o.AggrAvailsizePtr == nil {
		return r
	}
	r = *o.AggrAvailsizePtr
	return r
}

// SetAggrAvailsize is a fluent style 'setter' method that can be chained
func (o *VserverAggrInfoType) SetAggrAvailsize(newValue SizeType) *VserverAggrInfoType {
	o.AggrAvailsizePtr = &newValue
	return o
}

// AggrIsCftPrecommit is a 'getter' method
func (o *VserverAggrInfoType) AggrIsCftPrecommit() bool {
	var r bool
	if o.AggrIsCftPrecommitPtr == nil {
		return r
	}
	r = *o.AggrIsCftPrecommitPtr
	return r
}

// SetAggrIsCftPrecommit is a fluent style 'setter' method that can be chained
func (o *VserverAggrInfoType) SetAggrIsCftPrecommit(newValue bool) *VserverAggrInfoType {
	o.AggrIsCftPrecommitPtr = &newValue
	return o
}

// AggrName is a 'getter' method
func (o *VserverAggrInfoType) AggrName() AggrNameType {
	var r AggrNameType
	if o.AggrNamePtr == nil {
		return r
	}
	r = *o.AggrNamePtr
	return r
}

// SetAggrName is a fluent style 'setter' method that can be chained
func (o *VserverAggrInfoType) SetAggrName(newValue AggrNameType) *VserverAggrInfoType {
	o.AggrNamePtr = &newValue
	return o
}
