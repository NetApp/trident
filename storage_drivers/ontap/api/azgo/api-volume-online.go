// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeOnlineRequest is a structure to represent a volume-online Request ZAPI object
type VolumeOnlineRequest struct {
	XMLName xml.Name `xml:"volume-online"`
	NamePtr *string  `xml:"name"`
}

// VolumeOnlineResponse is a structure to represent a volume-online Response ZAPI object
type VolumeOnlineResponse struct {
	XMLName         xml.Name                   `xml:"netapp"`
	ResponseVersion string                     `xml:"version,attr"`
	ResponseXmlns   string                     `xml:"xmlns,attr"`
	Result          VolumeOnlineResponseResult `xml:"results"`
}

// NewVolumeOnlineResponse is a factory method for creating new instances of VolumeOnlineResponse objects
func NewVolumeOnlineResponse() *VolumeOnlineResponse {
	return &VolumeOnlineResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeOnlineResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeOnlineResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeOnlineResponseResult is a structure to represent a volume-online Response Result ZAPI object
type VolumeOnlineResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewVolumeOnlineRequest is a factory method for creating new instances of VolumeOnlineRequest objects
func NewVolumeOnlineRequest() *VolumeOnlineRequest {
	return &VolumeOnlineRequest{}
}

// NewVolumeOnlineResponseResult is a factory method for creating new instances of VolumeOnlineResponseResult objects
func NewVolumeOnlineResponseResult() *VolumeOnlineResponseResult {
	return &VolumeOnlineResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeOnlineRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeOnlineResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeOnlineRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeOnlineResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeOnlineRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeOnlineResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeOnlineRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeOnlineResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeOnlineRequest", NewVolumeOnlineResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeOnlineResponse), err
}

// Name is a 'getter' method
func (o *VolumeOnlineRequest) Name() string {
	var r string
	if o.NamePtr == nil {
		return r
	}
	r = *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *VolumeOnlineRequest) SetName(newValue string) *VolumeOnlineRequest {
	o.NamePtr = &newValue
	return o
}
