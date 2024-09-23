// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunMapInfoType is a structure to represent a lun-map-info ZAPI object
type LunMapInfoType struct {
	XMLName               xml.Name                      `xml:"lun-map-info"`
	InitiatorGroupPtr     *string                       `xml:"initiator-group"`
	InitiatorGroupUuidPtr *string                       `xml:"initiator-group-uuid"`
	LunIdPtr              *int                          `xml:"lun-id"`
	LunUuidPtr            *string                       `xml:"lun-uuid"`
	NodePtr               *NodeNameType                 `xml:"node"`
	PathPtr               *string                       `xml:"path"`
	ReportingNodesPtr     *LunMapInfoTypeReportingNodes `xml:"reporting-nodes"`
	// work in progress
	VserverPtr *string `xml:"vserver"`
}

// NewLunMapInfoType is a factory method for creating new instances of LunMapInfoType objects
func NewLunMapInfoType() *LunMapInfoType {
	return &LunMapInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *LunMapInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// InitiatorGroup is a 'getter' method
func (o *LunMapInfoType) InitiatorGroup() string {
	var r string
	if o.InitiatorGroupPtr == nil {
		return r
	}
	r = *o.InitiatorGroupPtr
	return r
}

// SetInitiatorGroup is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetInitiatorGroup(newValue string) *LunMapInfoType {
	o.InitiatorGroupPtr = &newValue
	return o
}

// InitiatorGroupUuid is a 'getter' method
func (o *LunMapInfoType) InitiatorGroupUuid() string {
	var r string
	if o.InitiatorGroupUuidPtr == nil {
		return r
	}
	r = *o.InitiatorGroupUuidPtr
	return r
}

// SetInitiatorGroupUuid is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetInitiatorGroupUuid(newValue string) *LunMapInfoType {
	o.InitiatorGroupUuidPtr = &newValue
	return o
}

// LunId is a 'getter' method
func (o *LunMapInfoType) LunId() int {
	var r int
	if o.LunIdPtr == nil {
		return r
	}
	r = *o.LunIdPtr
	return r
}

// SetLunId is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetLunId(newValue int) *LunMapInfoType {
	o.LunIdPtr = &newValue
	return o
}

// LunUuid is a 'getter' method
func (o *LunMapInfoType) LunUuid() string {
	var r string
	if o.LunUuidPtr == nil {
		return r
	}
	r = *o.LunUuidPtr
	return r
}

// SetLunUuid is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetLunUuid(newValue string) *LunMapInfoType {
	o.LunUuidPtr = &newValue
	return o
}

// Node is a 'getter' method
func (o *LunMapInfoType) Node() NodeNameType {
	var r NodeNameType
	if o.NodePtr == nil {
		return r
	}
	r = *o.NodePtr
	return r
}

// SetNode is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetNode(newValue NodeNameType) *LunMapInfoType {
	o.NodePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunMapInfoType) Path() string {
	var r string
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetPath(newValue string) *LunMapInfoType {
	o.PathPtr = &newValue
	return o
}

// LunMapInfoTypeReportingNodes is a wrapper
type LunMapInfoTypeReportingNodes struct {
	XMLName     xml.Name       `xml:"reporting-nodes"`
	NodeNamePtr []NodeNameType `xml:"node-name"`
}

// NodeName is a 'getter' method
func (o *LunMapInfoTypeReportingNodes) NodeName() []NodeNameType {
	r := o.NodeNamePtr
	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *LunMapInfoTypeReportingNodes) SetNodeName(newValue []NodeNameType) *LunMapInfoTypeReportingNodes {
	newSlice := make([]NodeNameType, len(newValue))
	copy(newSlice, newValue)
	o.NodeNamePtr = newSlice
	return o
}

// ReportingNodes is a 'getter' method
func (o *LunMapInfoType) ReportingNodes() LunMapInfoTypeReportingNodes {
	var r LunMapInfoTypeReportingNodes
	if o.ReportingNodesPtr == nil {
		return r
	}
	r = *o.ReportingNodesPtr
	return r
}

// SetReportingNodes is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetReportingNodes(newValue LunMapInfoTypeReportingNodes) *LunMapInfoType {
	o.ReportingNodesPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *LunMapInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetVserver(newValue string) *LunMapInfoType {
	o.VserverPtr = &newValue
	return o
}
