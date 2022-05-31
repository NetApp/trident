// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeOfflineRequest is a structure to represent a volume-offline Request ZAPI object
type VolumeOfflineRequest struct {
	XMLName xml.Name `xml:"volume-offline"`
	NamePtr *string  `xml:"name"`
}

// VolumeOfflineResponse is a structure to represent a volume-offline Response ZAPI object
type VolumeOfflineResponse struct {
	XMLName         xml.Name                    `xml:"netapp"`
	ResponseVersion string                      `xml:"version,attr"`
	ResponseXmlns   string                      `xml:"xmlns,attr"`
	Result          VolumeOfflineResponseResult `xml:"results"`
}

// NewVolumeOfflineResponse is a factory method for creating new instances of VolumeOfflineResponse objects
func NewVolumeOfflineResponse() *VolumeOfflineResponse {
	return &VolumeOfflineResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeOfflineResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeOfflineResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeOfflineResponseResult is a structure to represent a volume-offline Response Result ZAPI object
type VolumeOfflineResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewVolumeOfflineRequest is a factory method for creating new instances of VolumeOfflineRequest objects
func NewVolumeOfflineRequest() *VolumeOfflineRequest {
	return &VolumeOfflineRequest{}
}

// NewVolumeOfflineResponseResult is a factory method for creating new instances of VolumeOfflineResponseResult objects
func NewVolumeOfflineResponseResult() *VolumeOfflineResponseResult {
	return &VolumeOfflineResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeOfflineRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeOfflineResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeOfflineRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeOfflineResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeOfflineRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeOfflineResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeOfflineRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeOfflineResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeOfflineRequest", NewVolumeOfflineResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeOfflineResponse), err
}

// Name is a 'getter' method
func (o *VolumeOfflineRequest) Name() string {
	var r string
	if o.NamePtr == nil {
		return r
	}
	r = *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *VolumeOfflineRequest) SetName(newValue string) *VolumeOfflineRequest {
	o.NamePtr = &newValue
	return o
}
