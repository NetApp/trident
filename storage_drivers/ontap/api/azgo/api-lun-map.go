// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunMapRequest is a structure to represent a lun-map Request ZAPI object
type LunMapRequest struct {
	XMLName                    xml.Name      `xml:"lun-map"`
	AdditionalReportingNodePtr *NodeNameType `xml:"additional-reporting-node"`
	ForcePtr                   *bool         `xml:"force"`
	InitiatorGroupPtr          *string       `xml:"initiator-group"`
	LunIdPtr                   *int          `xml:"lun-id"`
	PathPtr                    *string       `xml:"path"`
}

// LunMapResponse is a structure to represent a lun-map Response ZAPI object
type LunMapResponse struct {
	XMLName         xml.Name             `xml:"netapp"`
	ResponseVersion string               `xml:"version,attr"`
	ResponseXmlns   string               `xml:"xmlns,attr"`
	Result          LunMapResponseResult `xml:"results"`
}

// NewLunMapResponse is a factory method for creating new instances of LunMapResponse objects
func NewLunMapResponse() *LunMapResponse {
	return &LunMapResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunMapResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunMapResponseResult is a structure to represent a lun-map Response Result ZAPI object
type LunMapResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
	LunIdAssignedPtr *int     `xml:"lun-id-assigned"`
}

// NewLunMapRequest is a factory method for creating new instances of LunMapRequest objects
func NewLunMapRequest() *LunMapRequest {
	return &LunMapRequest{}
}

// NewLunMapResponseResult is a factory method for creating new instances of LunMapResponseResult objects
func NewLunMapResponseResult() *LunMapResponseResult {
	return &LunMapResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunMapRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunMapResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunMapResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunMapRequest) ExecuteUsing(zr *ZapiRunner) (*LunMapResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunMapRequest) executeWithoutIteration(zr *ZapiRunner) (*LunMapResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunMapRequest", NewLunMapResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunMapResponse), err
}

// AdditionalReportingNode is a 'getter' method
func (o *LunMapRequest) AdditionalReportingNode() NodeNameType {
	var r NodeNameType
	if o.AdditionalReportingNodePtr == nil {
		return r
	}
	r = *o.AdditionalReportingNodePtr
	return r
}

// SetAdditionalReportingNode is a fluent style 'setter' method that can be chained
func (o *LunMapRequest) SetAdditionalReportingNode(newValue NodeNameType) *LunMapRequest {
	o.AdditionalReportingNodePtr = &newValue
	return o
}

// Force is a 'getter' method
func (o *LunMapRequest) Force() bool {
	var r bool
	if o.ForcePtr == nil {
		return r
	}
	r = *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *LunMapRequest) SetForce(newValue bool) *LunMapRequest {
	o.ForcePtr = &newValue
	return o
}

// InitiatorGroup is a 'getter' method
func (o *LunMapRequest) InitiatorGroup() string {
	var r string
	if o.InitiatorGroupPtr == nil {
		return r
	}
	r = *o.InitiatorGroupPtr
	return r
}

// SetInitiatorGroup is a fluent style 'setter' method that can be chained
func (o *LunMapRequest) SetInitiatorGroup(newValue string) *LunMapRequest {
	o.InitiatorGroupPtr = &newValue
	return o
}

// LunId is a 'getter' method
func (o *LunMapRequest) LunId() int {
	var r int
	if o.LunIdPtr == nil {
		return r
	}
	r = *o.LunIdPtr
	return r
}

// SetLunId is a fluent style 'setter' method that can be chained
func (o *LunMapRequest) SetLunId(newValue int) *LunMapRequest {
	o.LunIdPtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunMapRequest) Path() string {
	var r string
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunMapRequest) SetPath(newValue string) *LunMapRequest {
	o.PathPtr = &newValue
	return o
}

// LunIdAssigned is a 'getter' method
func (o *LunMapResponseResult) LunIdAssigned() int {
	var r int
	if o.LunIdAssignedPtr == nil {
		return r
	}
	r = *o.LunIdAssignedPtr
	return r
}

// SetLunIdAssigned is a fluent style 'setter' method that can be chained
func (o *LunMapResponseResult) SetLunIdAssigned(newValue int) *LunMapResponseResult {
	o.LunIdAssignedPtr = &newValue
	return o
}
