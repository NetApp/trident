// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NodeOntapiDetailInfoType is a structure to represent a node-ontapi-detail-info ZAPI object
type NodeOntapiDetailInfoType struct {
	XMLName         xml.Name `xml:"node-ontapi-detail-info"`
	MajorVersionPtr *int     `xml:"major-version"`
	MinorVersionPtr *int     `xml:"minor-version"`
	NodeNamePtr     *string  `xml:"node-name"`
	NodeUuidPtr     *string  `xml:"node-uuid"`
}

// NewNodeOntapiDetailInfoType is a factory method for creating new instances of NodeOntapiDetailInfoType objects
func NewNodeOntapiDetailInfoType() *NodeOntapiDetailInfoType {
	return &NodeOntapiDetailInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NodeOntapiDetailInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NodeOntapiDetailInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// MajorVersion is a 'getter' method
func (o *NodeOntapiDetailInfoType) MajorVersion() int {
	var r int
	if o.MajorVersionPtr == nil {
		return r
	}
	r = *o.MajorVersionPtr
	return r
}

// SetMajorVersion is a fluent style 'setter' method that can be chained
func (o *NodeOntapiDetailInfoType) SetMajorVersion(newValue int) *NodeOntapiDetailInfoType {
	o.MajorVersionPtr = &newValue
	return o
}

// MinorVersion is a 'getter' method
func (o *NodeOntapiDetailInfoType) MinorVersion() int {
	var r int
	if o.MinorVersionPtr == nil {
		return r
	}
	r = *o.MinorVersionPtr
	return r
}

// SetMinorVersion is a fluent style 'setter' method that can be chained
func (o *NodeOntapiDetailInfoType) SetMinorVersion(newValue int) *NodeOntapiDetailInfoType {
	o.MinorVersionPtr = &newValue
	return o
}

// NodeName is a 'getter' method
func (o *NodeOntapiDetailInfoType) NodeName() string {
	var r string
	if o.NodeNamePtr == nil {
		return r
	}
	r = *o.NodeNamePtr
	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *NodeOntapiDetailInfoType) SetNodeName(newValue string) *NodeOntapiDetailInfoType {
	o.NodeNamePtr = &newValue
	return o
}

// NodeUuid is a 'getter' method
func (o *NodeOntapiDetailInfoType) NodeUuid() string {
	var r string
	if o.NodeUuidPtr == nil {
		return r
	}
	r = *o.NodeUuidPtr
	return r
}

// SetNodeUuid is a fluent style 'setter' method that can be chained
func (o *NodeOntapiDetailInfoType) SetNodeUuid(newValue string) *NodeOntapiDetailInfoType {
	o.NodeUuidPtr = &newValue
	return o
}
