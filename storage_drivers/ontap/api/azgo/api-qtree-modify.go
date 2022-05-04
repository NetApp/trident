// Code generated automatically. DO NOT EDIT.
// Copyright 2020 NetApp, Inc. All Rights Reserved.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// QtreeModifyRequest is a structure to represent a qtree-modify Request ZAPI object
type QtreeModifyRequest struct {
	XMLName          xml.Name `xml:"qtree-modify"`
	ExportPolicyPtr  *string  `xml:"export-policy"`
	ModePtr          *string  `xml:"mode"`
	OplocksPtr       *string  `xml:"oplocks"`
	QtreePtr         *string  `xml:"qtree"`
	SecurityStylePtr *string  `xml:"security-style"`
	VolumePtr        *string  `xml:"volume"`
}

// QtreeModifyResponse is a structure to represent a qtree-modify Response ZAPI object
type QtreeModifyResponse struct {
	XMLName         xml.Name                  `xml:"netapp"`
	ResponseVersion string                    `xml:"version,attr"`
	ResponseXmlns   string                    `xml:"xmlns,attr"`
	Result          QtreeModifyResponseResult `xml:"results"`
}

// NewQtreeModifyResponse is a factory method for creating new instances of QtreeModifyResponse objects
func NewQtreeModifyResponse() *QtreeModifyResponse {
	return &QtreeModifyResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeModifyResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *QtreeModifyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// QtreeModifyResponseResult is a structure to represent a qtree-modify Response Result ZAPI object
type QtreeModifyResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewQtreeModifyRequest is a factory method for creating new instances of QtreeModifyRequest objects
func NewQtreeModifyRequest() *QtreeModifyRequest {
	return &QtreeModifyRequest{}
}

// NewQtreeModifyResponseResult is a factory method for creating new instances of QtreeModifyResponseResult objects
func NewQtreeModifyResponseResult() *QtreeModifyResponseResult {
	return &QtreeModifyResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *QtreeModifyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *QtreeModifyResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeModifyRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeModifyResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QtreeModifyRequest) ExecuteUsing(zr *ZapiRunner) (*QtreeModifyResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QtreeModifyRequest) executeWithoutIteration(zr *ZapiRunner) (*QtreeModifyResponse, error) {
	result, err := zr.ExecuteUsing(o, "QtreeModifyRequest", NewQtreeModifyResponse())
	if result == nil {
		return nil, err
	}
	return result.(*QtreeModifyResponse), err
}

// ExportPolicy is a 'getter' method
func (o *QtreeModifyRequest) ExportPolicy() string {
	r := *o.ExportPolicyPtr
	return r
}

// SetExportPolicy is a fluent style 'setter' method that can be chained
func (o *QtreeModifyRequest) SetExportPolicy(newValue string) *QtreeModifyRequest {
	o.ExportPolicyPtr = &newValue
	return o
}

// Mode is a 'getter' method
func (o *QtreeModifyRequest) Mode() string {
	r := *o.ModePtr
	return r
}

// SetMode is a fluent style 'setter' method that can be chained
func (o *QtreeModifyRequest) SetMode(newValue string) *QtreeModifyRequest {
	o.ModePtr = &newValue
	return o
}

// Oplocks is a 'getter' method
func (o *QtreeModifyRequest) Oplocks() string {
	r := *o.OplocksPtr
	return r
}

// SetOplocks is a fluent style 'setter' method that can be chained
func (o *QtreeModifyRequest) SetOplocks(newValue string) *QtreeModifyRequest {
	o.OplocksPtr = &newValue
	return o
}

// Qtree is a 'getter' method
func (o *QtreeModifyRequest) Qtree() string {
	r := *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *QtreeModifyRequest) SetQtree(newValue string) *QtreeModifyRequest {
	o.QtreePtr = &newValue
	return o
}

// SecurityStyle is a 'getter' method
func (o *QtreeModifyRequest) SecurityStyle() string {
	r := *o.SecurityStylePtr
	return r
}

// SetSecurityStyle is a fluent style 'setter' method that can be chained
func (o *QtreeModifyRequest) SetSecurityStyle(newValue string) *QtreeModifyRequest {
	o.SecurityStylePtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *QtreeModifyRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *QtreeModifyRequest) SetVolume(newValue string) *QtreeModifyRequest {
	o.VolumePtr = &newValue
	return o
}
