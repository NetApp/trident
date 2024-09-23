// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NodeVersionDetailInfoType is a structure to represent a node-version-detail-info ZAPI object
type NodeVersionDetailInfoType struct {
	XMLName           xml.Name `xml:"node-version-detail-info"`
	BuildTimestampPtr *int     `xml:"build-timestamp"`
	NodeNamePtr       *string  `xml:"node-name"`
	NodeUuidPtr       *string  `xml:"node-uuid"`
	VersionPtr        *string  `xml:"version"`
}

// NewNodeVersionDetailInfoType is a factory method for creating new instances of NodeVersionDetailInfoType objects
func NewNodeVersionDetailInfoType() *NodeVersionDetailInfoType {
	return &NodeVersionDetailInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NodeVersionDetailInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NodeVersionDetailInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// BuildTimestamp is a 'getter' method
func (o *NodeVersionDetailInfoType) BuildTimestamp() int {
	var r int
	if o.BuildTimestampPtr == nil {
		return r
	}
	r = *o.BuildTimestampPtr
	return r
}

// SetBuildTimestamp is a fluent style 'setter' method that can be chained
func (o *NodeVersionDetailInfoType) SetBuildTimestamp(newValue int) *NodeVersionDetailInfoType {
	o.BuildTimestampPtr = &newValue
	return o
}

// NodeName is a 'getter' method
func (o *NodeVersionDetailInfoType) NodeName() string {
	var r string
	if o.NodeNamePtr == nil {
		return r
	}
	r = *o.NodeNamePtr
	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *NodeVersionDetailInfoType) SetNodeName(newValue string) *NodeVersionDetailInfoType {
	o.NodeNamePtr = &newValue
	return o
}

// NodeUuid is a 'getter' method
func (o *NodeVersionDetailInfoType) NodeUuid() string {
	var r string
	if o.NodeUuidPtr == nil {
		return r
	}
	r = *o.NodeUuidPtr
	return r
}

// SetNodeUuid is a fluent style 'setter' method that can be chained
func (o *NodeVersionDetailInfoType) SetNodeUuid(newValue string) *NodeVersionDetailInfoType {
	o.NodeUuidPtr = &newValue
	return o
}

// Version is a 'getter' method
func (o *NodeVersionDetailInfoType) Version() string {
	var r string
	if o.VersionPtr == nil {
		return r
	}
	r = *o.VersionPtr
	return r
}

// SetVersion is a fluent style 'setter' method that can be chained
func (o *NodeVersionDetailInfoType) SetVersion(newValue string) *NodeVersionDetailInfoType {
	o.VersionPtr = &newValue
	return o
}
