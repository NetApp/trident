// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// CifsShareDeleteRequest is a structure to represent a cifs-share-delete Request ZAPI object
type CifsShareDeleteRequest struct {
	XMLName      xml.Name `xml:"cifs-share-delete"`
	ShareNamePtr *string  `xml:"share-name"`
}

// CifsShareDeleteResponse is a structure to represent a cifs-share-delete Response ZAPI object
type CifsShareDeleteResponse struct {
	XMLName         xml.Name                      `xml:"netapp"`
	ResponseVersion string                        `xml:"version,attr"`
	ResponseXmlns   string                        `xml:"xmlns,attr"`
	Result          CifsShareDeleteResponseResult `xml:"results"`
}

// NewCifsShareDeleteResponse is a factory method for creating new instances of CifsShareDeleteResponse objects
func NewCifsShareDeleteResponse() *CifsShareDeleteResponse {
	return &CifsShareDeleteResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareDeleteResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *CifsShareDeleteResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// CifsShareDeleteResponseResult is a structure to represent a cifs-share-delete Response Result ZAPI object
type CifsShareDeleteResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewCifsShareDeleteRequest is a factory method for creating new instances of CifsShareDeleteRequest objects
func NewCifsShareDeleteRequest() *CifsShareDeleteRequest {
	return &CifsShareDeleteRequest{}
}

// NewCifsShareDeleteResponseResult is a factory method for creating new instances of CifsShareDeleteResponseResult objects
func NewCifsShareDeleteResponseResult() *CifsShareDeleteResponseResult {
	return &CifsShareDeleteResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *CifsShareDeleteRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *CifsShareDeleteResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareDeleteRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareDeleteResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CifsShareDeleteRequest) ExecuteUsing(zr *ZapiRunner) (*CifsShareDeleteResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CifsShareDeleteRequest) executeWithoutIteration(zr *ZapiRunner) (*CifsShareDeleteResponse, error) {
	result, err := zr.ExecuteUsing(o, "CifsShareDeleteRequest", NewCifsShareDeleteResponse())
	if result == nil {
		return nil, err
	}
	return result.(*CifsShareDeleteResponse), err
}

// ShareName is a 'getter' method
func (o *CifsShareDeleteRequest) ShareName() string {
	var r string
	if o.ShareNamePtr == nil {
		return r
	}
	r = *o.ShareNamePtr
	return r
}

// SetShareName is a fluent style 'setter' method that can be chained
func (o *CifsShareDeleteRequest) SetShareName(newValue string) *CifsShareDeleteRequest {
	o.ShareNamePtr = &newValue
	return o
}
