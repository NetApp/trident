// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapshotOwnerType is a structure to represent a snapshot-owner ZAPI object
type SnapshotOwnerType struct {
	XMLName  xml.Name `xml:"snapshot-owner"`
	OwnerPtr *string  `xml:"owner"`
}

// NewSnapshotOwnerType is a factory method for creating new instances of SnapshotOwnerType objects
func NewSnapshotOwnerType() *SnapshotOwnerType {
	return &SnapshotOwnerType{}
}

// ToXML converts this object into an xml string representation
func (o *SnapshotOwnerType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotOwnerType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Owner is a 'getter' method
func (o *SnapshotOwnerType) Owner() string {
	var r string
	if o.OwnerPtr == nil {
		return r
	}
	r = *o.OwnerPtr
	return r
}

// SetOwner is a fluent style 'setter' method that can be chained
func (o *SnapshotOwnerType) SetOwner(newValue string) *SnapshotOwnerType {
	o.OwnerPtr = &newValue
	return o
}
