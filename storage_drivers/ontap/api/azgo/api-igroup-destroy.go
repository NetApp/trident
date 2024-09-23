// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IgroupDestroyRequest is a structure to represent a igroup-destroy Request ZAPI object
type IgroupDestroyRequest struct {
	XMLName               xml.Name `xml:"igroup-destroy"`
	ForcePtr              *bool    `xml:"force"`
	InitiatorGroupNamePtr *string  `xml:"initiator-group-name"`
}

// IgroupDestroyResponse is a structure to represent a igroup-destroy Response ZAPI object
type IgroupDestroyResponse struct {
	XMLName         xml.Name                    `xml:"netapp"`
	ResponseVersion string                      `xml:"version,attr"`
	ResponseXmlns   string                      `xml:"xmlns,attr"`
	Result          IgroupDestroyResponseResult `xml:"results"`
}

// NewIgroupDestroyResponse is a factory method for creating new instances of IgroupDestroyResponse objects
func NewIgroupDestroyResponse() *IgroupDestroyResponse {
	return &IgroupDestroyResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupDestroyResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IgroupDestroyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IgroupDestroyResponseResult is a structure to represent a igroup-destroy Response Result ZAPI object
type IgroupDestroyResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewIgroupDestroyRequest is a factory method for creating new instances of IgroupDestroyRequest objects
func NewIgroupDestroyRequest() *IgroupDestroyRequest {
	return &IgroupDestroyRequest{}
}

// NewIgroupDestroyResponseResult is a factory method for creating new instances of IgroupDestroyResponseResult objects
func NewIgroupDestroyResponseResult() *IgroupDestroyResponseResult {
	return &IgroupDestroyResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IgroupDestroyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IgroupDestroyResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupDestroyRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupDestroyResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IgroupDestroyRequest) ExecuteUsing(zr *ZapiRunner) (*IgroupDestroyResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IgroupDestroyRequest) executeWithoutIteration(zr *ZapiRunner) (*IgroupDestroyResponse, error) {
	result, err := zr.ExecuteUsing(o, "IgroupDestroyRequest", NewIgroupDestroyResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IgroupDestroyResponse), err
}

// Force is a 'getter' method
func (o *IgroupDestroyRequest) Force() bool {
	var r bool
	if o.ForcePtr == nil {
		return r
	}
	r = *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *IgroupDestroyRequest) SetForce(newValue bool) *IgroupDestroyRequest {
	o.ForcePtr = &newValue
	return o
}

// InitiatorGroupName is a 'getter' method
func (o *IgroupDestroyRequest) InitiatorGroupName() string {
	var r string
	if o.InitiatorGroupNamePtr == nil {
		return r
	}
	r = *o.InitiatorGroupNamePtr
	return r
}

// SetInitiatorGroupName is a fluent style 'setter' method that can be chained
func (o *IgroupDestroyRequest) SetInitiatorGroupName(newValue string) *IgroupDestroyRequest {
	o.InitiatorGroupNamePtr = &newValue
	return o
}
