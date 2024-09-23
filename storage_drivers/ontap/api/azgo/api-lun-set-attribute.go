// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunSetAttributeRequest is a structure to represent a lun-set-attribute Request ZAPI object
type LunSetAttributeRequest struct {
	XMLName  xml.Name `xml:"lun-set-attribute"`
	NamePtr  *string  `xml:"name"`
	PathPtr  *string  `xml:"path"`
	ValuePtr *string  `xml:"value"`
}

// LunSetAttributeResponse is a structure to represent a lun-set-attribute Response ZAPI object
type LunSetAttributeResponse struct {
	XMLName         xml.Name                      `xml:"netapp"`
	ResponseVersion string                        `xml:"version,attr"`
	ResponseXmlns   string                        `xml:"xmlns,attr"`
	Result          LunSetAttributeResponseResult `xml:"results"`
}

// NewLunSetAttributeResponse is a factory method for creating new instances of LunSetAttributeResponse objects
func NewLunSetAttributeResponse() *LunSetAttributeResponse {
	return &LunSetAttributeResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunSetAttributeResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunSetAttributeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunSetAttributeResponseResult is a structure to represent a lun-set-attribute Response Result ZAPI object
type LunSetAttributeResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewLunSetAttributeRequest is a factory method for creating new instances of LunSetAttributeRequest objects
func NewLunSetAttributeRequest() *LunSetAttributeRequest {
	return &LunSetAttributeRequest{}
}

// NewLunSetAttributeResponseResult is a factory method for creating new instances of LunSetAttributeResponseResult objects
func NewLunSetAttributeResponseResult() *LunSetAttributeResponseResult {
	return &LunSetAttributeResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunSetAttributeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunSetAttributeResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunSetAttributeRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunSetAttributeResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunSetAttributeRequest) ExecuteUsing(zr *ZapiRunner) (*LunSetAttributeResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunSetAttributeRequest) executeWithoutIteration(zr *ZapiRunner) (*LunSetAttributeResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunSetAttributeRequest", NewLunSetAttributeResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunSetAttributeResponse), err
}

// Name is a 'getter' method
func (o *LunSetAttributeRequest) Name() string {
	var r string
	if o.NamePtr == nil {
		return r
	}
	r = *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *LunSetAttributeRequest) SetName(newValue string) *LunSetAttributeRequest {
	o.NamePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunSetAttributeRequest) Path() string {
	var r string
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunSetAttributeRequest) SetPath(newValue string) *LunSetAttributeRequest {
	o.PathPtr = &newValue
	return o
}

// Value is a 'getter' method
func (o *LunSetAttributeRequest) Value() string {
	var r string
	if o.ValuePtr == nil {
		return r
	}
	r = *o.ValuePtr
	return r
}

// SetValue is a fluent style 'setter' method that can be chained
func (o *LunSetAttributeRequest) SetValue(newValue string) *LunSetAttributeRequest {
	o.ValuePtr = &newValue
	return o
}
