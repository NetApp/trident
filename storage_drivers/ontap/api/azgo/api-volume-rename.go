// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeRenameRequest is a structure to represent a volume-rename Request ZAPI object
type VolumeRenameRequest struct {
	XMLName          xml.Name `xml:"volume-rename"`
	NewVolumeNamePtr *string  `xml:"new-volume-name"`
	VolumePtr        *string  `xml:"volume"`
}

// VolumeRenameResponse is a structure to represent a volume-rename Response ZAPI object
type VolumeRenameResponse struct {
	XMLName         xml.Name                   `xml:"netapp"`
	ResponseVersion string                     `xml:"version,attr"`
	ResponseXmlns   string                     `xml:"xmlns,attr"`
	Result          VolumeRenameResponseResult `xml:"results"`
}

// NewVolumeRenameResponse is a factory method for creating new instances of VolumeRenameResponse objects
func NewVolumeRenameResponse() *VolumeRenameResponse {
	return &VolumeRenameResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRenameResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeRenameResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeRenameResponseResult is a structure to represent a volume-rename Response Result ZAPI object
type VolumeRenameResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewVolumeRenameRequest is a factory method for creating new instances of VolumeRenameRequest objects
func NewVolumeRenameRequest() *VolumeRenameRequest {
	return &VolumeRenameRequest{}
}

// NewVolumeRenameResponseResult is a factory method for creating new instances of VolumeRenameResponseResult objects
func NewVolumeRenameResponseResult() *VolumeRenameResponseResult {
	return &VolumeRenameResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeRenameRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeRenameResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRenameRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeRenameResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeRenameRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeRenameResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeRenameRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeRenameResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeRenameRequest", NewVolumeRenameResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeRenameResponse), err
}

// NewVolumeName is a 'getter' method
func (o *VolumeRenameRequest) NewVolumeName() string {
	var r string
	if o.NewVolumeNamePtr == nil {
		return r
	}
	r = *o.NewVolumeNamePtr
	return r
}

// SetNewVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeRenameRequest) SetNewVolumeName(newValue string) *VolumeRenameRequest {
	o.NewVolumeNamePtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *VolumeRenameRequest) Volume() string {
	var r string
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeRenameRequest) SetVolume(newValue string) *VolumeRenameRequest {
	o.VolumePtr = &newValue
	return o
}
