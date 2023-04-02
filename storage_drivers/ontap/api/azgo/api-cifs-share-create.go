// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// CifsShareCreateRequest is a structure to represent a cifs-share-create Request ZAPI object
type CifsShareCreateRequest struct {
	XMLName       xml.Name `xml:"cifs-share-create"`
	ShareNamePtr  *string  `xml:"share-name"`
	PathPtr       *string  `xml:"path"`
	VolumeNamePtr *string  `xml:"volume-name"`
}

// CifsShareCreateResponse is a structure to represent a cifs-share-create Response ZAPI object
type CifsShareCreateResponse struct {
	XMLName         xml.Name                      `xml:"netapp"`
	ResponseVersion string                        `xml:"version,attr"`
	ResponseXmlns   string                        `xml:"xmlns,attr"`
	Result          CifsShareCreateResponseResult `xml:"results"`
}

// NewCifsShareCreateResponse is a factory method for creating new instances of CifsShareCreateResponse objects
func NewCifsShareCreateResponse() *CifsShareCreateResponse {
	return &CifsShareCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *CifsShareCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// CifsShareCreateResponseResult is a structure to represent a cifs-share-create Response Result ZAPI object
type CifsShareCreateResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewCifsShareCreateRequest is a factory method for creating new instances of CifsShareCreateRequest objects
func NewCifsShareCreateRequest() *CifsShareCreateRequest {
	return &CifsShareCreateRequest{}
}

// NewCifsShareCreateResponseResult is a factory method for creating new instances of CifsShareCreateResponseResult objects
func NewCifsShareCreateResponseResult() *CifsShareCreateResponseResult {
	return &CifsShareCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *CifsShareCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *CifsShareCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CifsShareCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CifsShareCreateRequest) ExecuteUsing(zr *ZapiRunner) (*CifsShareCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CifsShareCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*CifsShareCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "CifsShareCreateRequest", NewCifsShareCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*CifsShareCreateResponse), err
}

// ShareName is a 'getter' method
func (o *CifsShareCreateRequest) ShareName() string {
	var r string
	if o.ShareNamePtr == nil {
		return r
	}
	r = *o.ShareNamePtr
	return r
}

// SetShareName is a fluent style 'setter' method that can be chained
func (o *CifsShareCreateRequest) SetShareName(newValue string) *CifsShareCreateRequest {
	o.ShareNamePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *CifsShareCreateRequest) Path() string {
	var r string
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *CifsShareCreateRequest) SetPath(newValue string) *CifsShareCreateRequest {
	o.PathPtr = &newValue
	return o
}

// VolumeName is a 'getter' method
func (o *CifsShareCreateRequest) VolumeName() string {
	var r string
	if o.VolumeNamePtr == nil {
		return r
	}
	r = *o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *CifsShareCreateRequest) SetVolumeName(newValue string) *CifsShareCreateRequest {
	o.VolumeNamePtr = &newValue
	return o
}
