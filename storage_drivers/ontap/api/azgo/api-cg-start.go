// Code generated automatically. DO NOT EDIT.
// Copyright 2025 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// CgStartRequest is a structure to represent a cg-start Request ZAPI object
type CgStartRequest struct {
	XMLName            xml.Name               `xml:"cg-start"`
	SnapmirrorLabelPtr *string                `xml:"snapmirror-label"`
	SnapshotPtr        *string                `xml:"snapshot"`
	TimeoutPtr         *string                `xml:"timeout"`
	UserTimeoutPtr     *int                   `xml:"user-timeout"`
	VolumesPtr         *CgStartRequestVolumes `xml:"volumes"`
}

// CgStartResponse is a structure to represent a cg-start Response ZAPI object
type CgStartResponse struct {
	XMLName         xml.Name              `xml:"netapp"`
	ResponseVersion string                `xml:"version,attr"`
	ResponseXmlns   string                `xml:"xmlns,attr"`
	Result          CgStartResponseResult `xml:"results"`
}

// NewCgStartResponse is a factory method for creating new instances of CgStartResponse objects
func NewCgStartResponse() *CgStartResponse {
	return &CgStartResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CgStartResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *CgStartResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// CgStartResponseResult is a structure to represent a cg-start Response Result ZAPI object
type CgStartResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
	CgIdPtr          *int     `xml:"cg-id"`
}

// NewCgStartRequest is a factory method for creating new instances of CgStartRequest objects
func NewCgStartRequest() *CgStartRequest {
	return &CgStartRequest{}
}

// NewCgStartResponseResult is a factory method for creating new instances of CgStartResponseResult objects
func NewCgStartResponseResult() *CgStartResponseResult {
	return &CgStartResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *CgStartRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *CgStartResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CgStartRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CgStartResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CgStartRequest) ExecuteUsing(zr *ZapiRunner) (*CgStartResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *CgStartRequest) executeWithoutIteration(zr *ZapiRunner) (*CgStartResponse, error) {
	result, err := zr.ExecuteUsing(o, "CgStartRequest", NewCgStartResponse())
	if result == nil {
		return nil, err
	}
	return result.(*CgStartResponse), err
}

// SnapmirrorLabel is a 'getter' method
func (o *CgStartRequest) SnapmirrorLabel() string {
	var r string
	if o.SnapmirrorLabelPtr == nil {
		return r
	}
	r = *o.SnapmirrorLabelPtr
	return r
}

// SetSnapmirrorLabel is a fluent style 'setter' method that can be chained
func (o *CgStartRequest) SetSnapmirrorLabel(newValue string) *CgStartRequest {
	o.SnapmirrorLabelPtr = &newValue
	return o
}

// Snapshot is a 'getter' method
func (o *CgStartRequest) Snapshot() string {
	var r string
	if o.SnapshotPtr == nil {
		return r
	}
	r = *o.SnapshotPtr
	return r
}

// SetSnapshot is a fluent style 'setter' method that can be chained
func (o *CgStartRequest) SetSnapshot(newValue string) *CgStartRequest {
	o.SnapshotPtr = &newValue
	return o
}

// Timeout is a 'getter' method
func (o *CgStartRequest) Timeout() string {
	var r string
	if o.TimeoutPtr == nil {
		return r
	}
	r = *o.TimeoutPtr
	return r
}

// SetTimeout is a fluent style 'setter' method that can be chained
func (o *CgStartRequest) SetTimeout(newValue string) *CgStartRequest {
	o.TimeoutPtr = &newValue
	return o
}

// UserTimeout is a 'getter' method
func (o *CgStartRequest) UserTimeout() int {
	var r int
	if o.UserTimeoutPtr == nil {
		return r
	}
	r = *o.UserTimeoutPtr
	return r
}

// SetUserTimeout is a fluent style 'setter' method that can be chained
func (o *CgStartRequest) SetUserTimeout(newValue int) *CgStartRequest {
	o.UserTimeoutPtr = &newValue
	return o
}

// CgStartRequestVolumes is a wrapper
type CgStartRequestVolumes struct {
	XMLName       xml.Name         `xml:"volumes"`
	VolumeNamePtr []VolumeNameType `xml:"volume-name"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o CgStartRequestVolumes) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeName is a 'getter' method
func (o *CgStartRequestVolumes) VolumeName() []VolumeNameType {
	r := o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *CgStartRequestVolumes) SetVolumeName(newValue []VolumeNameType) *CgStartRequestVolumes {
	newSlice := make([]VolumeNameType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeNamePtr = newSlice
	return o
}

// Volumes is a 'getter' method
func (o *CgStartRequest) Volumes() CgStartRequestVolumes {
	var r CgStartRequestVolumes
	if o.VolumesPtr == nil {
		return r
	}
	r = *o.VolumesPtr
	return r
}

// SetVolumes is a fluent style 'setter' method that can be chained
func (o *CgStartRequest) SetVolumes(newValue CgStartRequestVolumes) *CgStartRequest {
	o.VolumesPtr = &newValue
	return o
}

// CgId is a 'getter' method
func (o *CgStartResponseResult) CgId() int {
	var r int
	if o.CgIdPtr == nil {
		return r
	}
	r = *o.CgIdPtr
	return r
}

// SetCgId is a fluent style 'setter' method that can be chained
func (o *CgStartResponseResult) SetCgId(newValue int) *CgStartResponseResult {
	o.CgIdPtr = &newValue
	return o
}
