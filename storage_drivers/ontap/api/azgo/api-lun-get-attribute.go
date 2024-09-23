// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunGetAttributeRequest is a structure to represent a lun-get-attribute Request ZAPI object
type LunGetAttributeRequest struct {
	XMLName xml.Name `xml:"lun-get-attribute"`
	NamePtr *string  `xml:"name"`
	PathPtr *string  `xml:"path"`
}

// LunGetAttributeResponse is a structure to represent a lun-get-attribute Response ZAPI object
type LunGetAttributeResponse struct {
	XMLName         xml.Name                      `xml:"netapp"`
	ResponseVersion string                        `xml:"version,attr"`
	ResponseXmlns   string                        `xml:"xmlns,attr"`
	Result          LunGetAttributeResponseResult `xml:"results"`
}

// NewLunGetAttributeResponse is a factory method for creating new instances of LunGetAttributeResponse objects
func NewLunGetAttributeResponse() *LunGetAttributeResponse {
	return &LunGetAttributeResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetAttributeResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunGetAttributeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunGetAttributeResponseResult is a structure to represent a lun-get-attribute Response Result ZAPI object
type LunGetAttributeResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
	ValuePtr         *string  `xml:"value"`
}

// NewLunGetAttributeRequest is a factory method for creating new instances of LunGetAttributeRequest objects
func NewLunGetAttributeRequest() *LunGetAttributeRequest {
	return &LunGetAttributeRequest{}
}

// NewLunGetAttributeResponseResult is a factory method for creating new instances of LunGetAttributeResponseResult objects
func NewLunGetAttributeResponseResult() *LunGetAttributeResponseResult {
	return &LunGetAttributeResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunGetAttributeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunGetAttributeResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetAttributeRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetAttributeResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunGetAttributeRequest) ExecuteUsing(zr *ZapiRunner) (*LunGetAttributeResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *LunGetAttributeRequest) executeWithoutIteration(zr *ZapiRunner) (*LunGetAttributeResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunGetAttributeRequest", NewLunGetAttributeResponse())
	if result == nil {
		return nil, err
	}
	return result.(*LunGetAttributeResponse), err
}

// Name is a 'getter' method
func (o *LunGetAttributeRequest) Name() string {
	var r string
	if o.NamePtr == nil {
		return r
	}
	r = *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *LunGetAttributeRequest) SetName(newValue string) *LunGetAttributeRequest {
	o.NamePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunGetAttributeRequest) Path() string {
	var r string
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunGetAttributeRequest) SetPath(newValue string) *LunGetAttributeRequest {
	o.PathPtr = &newValue
	return o
}

// Value is a 'getter' method
func (o *LunGetAttributeResponseResult) Value() string {
	var r string
	if o.ValuePtr == nil {
		return r
	}
	r = *o.ValuePtr
	return r
}

// SetValue is a fluent style 'setter' method that can be chained
func (o *LunGetAttributeResponseResult) SetValue(newValue string) *LunGetAttributeResponseResult {
	o.ValuePtr = &newValue
	return o
}
