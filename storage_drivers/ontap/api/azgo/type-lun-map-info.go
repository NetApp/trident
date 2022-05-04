// Code generated automatically. DO NOT EDIT.
/*
 * Copyright 2019 NetApp, Inc. All Rights Reserved
 */

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunMapInfoType is a structure to represent a lun-map-info ZAPI object
type LunMapInfoType struct {
	XMLName xml.Name `xml:"lun-map-info"`

	InitiatorGroupPtr     *string        `xml:"initiator-group"`
	InitiatorGroupUuidPtr *string        `xml:"initiator-group-uuid"`
	LunIdPtr              *int           `xml:"lun-id"`
	LunUuidPtr            *string        `xml:"lun-uuid"`
	NodePtr               *NodeNameType  `xml:"node"`
	PathPtr               *string        `xml:"path"`
	ReportingNodesPtr     []NodeNameType `xml:"reporting-nodes>node-name"`
	VserverPtr            *string        `xml:"vserver"`
}

// ToXML converts this object into an xml string representation
func (o *LunMapInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NewLunMapInfoType is a factory method for creating new instances of LunMapInfoType objects
func NewLunMapInfoType() *LunMapInfoType { return &LunMapInfoType{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// InitiatorGroup is a fluent style 'getter' method that can be chained
func (o *LunMapInfoType) InitiatorGroup() string {
	r := *o.InitiatorGroupPtr
	return r
}

// SetInitiatorGroup is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetInitiatorGroup(newValue string) *LunMapInfoType {
	o.InitiatorGroupPtr = &newValue
	return o
}

// InitiatorGroupUuid is a fluent style 'getter' method that can be chained
func (o *LunMapInfoType) InitiatorGroupUuid() string {
	r := *o.InitiatorGroupUuidPtr
	return r
}

// SetInitiatorGroupUuid is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetInitiatorGroupUuid(newValue string) *LunMapInfoType {
	o.InitiatorGroupUuidPtr = &newValue
	return o
}

// LunId is a fluent style 'getter' method that can be chained
func (o *LunMapInfoType) LunId() int {
	r := *o.LunIdPtr
	return r
}

// SetLunId is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetLunId(newValue int) *LunMapInfoType {
	o.LunIdPtr = &newValue
	return o
}

// LunUuid is a fluent style 'getter' method that can be chained
func (o *LunMapInfoType) LunUuid() string {
	r := *o.LunUuidPtr
	return r
}

// SetLunUuid is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetLunUuid(newValue string) *LunMapInfoType {
	o.LunUuidPtr = &newValue
	return o
}

// Node is a fluent style 'getter' method that can be chained
func (o *LunMapInfoType) Node() NodeNameType {
	r := *o.NodePtr
	return r
}

// SetNode is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetNode(newValue NodeNameType) *LunMapInfoType {
	o.NodePtr = &newValue
	return o
}

// Path is a fluent style 'getter' method that can be chained
func (o *LunMapInfoType) Path() string {
	r := *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetPath(newValue string) *LunMapInfoType {
	o.PathPtr = &newValue
	return o
}

// ReportingNodes is a fluent style 'getter' method that can be chained
func (o *LunMapInfoType) ReportingNodes() []NodeNameType {
	r := o.ReportingNodesPtr
	return r
}

// SetReportingNodes is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetReportingNodes(newValue []NodeNameType) *LunMapInfoType {
	newSlice := make([]NodeNameType, len(newValue))
	copy(newSlice, newValue)
	o.ReportingNodesPtr = newSlice
	return o
}

// Vserver is a fluent style 'getter' method that can be chained
func (o *LunMapInfoType) Vserver() string {
	r := *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *LunMapInfoType) SetVserver(newValue string) *LunMapInfoType {
	o.VserverPtr = &newValue
	return o
}
