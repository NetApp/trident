// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapshotDeleteRequest is a structure to represent a snapshot-delete Request ZAPI object
type SnapshotDeleteRequest struct {
	XMLName                 xml.Name  `xml:"snapshot-delete"`
	IgnoreOwnersPtr         *bool     `xml:"ignore-owners"`
	SnapshotPtr             *string   `xml:"snapshot"`
	SnapshotInstanceUuidPtr *UUIDType `xml:"snapshot-instance-uuid"`
	VolumePtr               *string   `xml:"volume"`
}

// SnapshotDeleteResponse is a structure to represent a snapshot-delete Response ZAPI object
type SnapshotDeleteResponse struct {
	XMLName         xml.Name                     `xml:"netapp"`
	ResponseVersion string                       `xml:"version,attr"`
	ResponseXmlns   string                       `xml:"xmlns,attr"`
	Result          SnapshotDeleteResponseResult `xml:"results"`
}

// NewSnapshotDeleteResponse is a factory method for creating new instances of SnapshotDeleteResponse objects
func NewSnapshotDeleteResponse() *SnapshotDeleteResponse {
	return &SnapshotDeleteResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotDeleteResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapshotDeleteResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapshotDeleteResponseResult is a structure to represent a snapshot-delete Response Result ZAPI object
type SnapshotDeleteResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewSnapshotDeleteRequest is a factory method for creating new instances of SnapshotDeleteRequest objects
func NewSnapshotDeleteRequest() *SnapshotDeleteRequest {
	return &SnapshotDeleteRequest{}
}

// NewSnapshotDeleteResponseResult is a factory method for creating new instances of SnapshotDeleteResponseResult objects
func NewSnapshotDeleteResponseResult() *SnapshotDeleteResponseResult {
	return &SnapshotDeleteResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapshotDeleteRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapshotDeleteResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotDeleteRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotDeleteResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotDeleteRequest) ExecuteUsing(zr *ZapiRunner) (*SnapshotDeleteResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotDeleteRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapshotDeleteResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapshotDeleteRequest", NewSnapshotDeleteResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapshotDeleteResponse), err
}

// IgnoreOwners is a 'getter' method
func (o *SnapshotDeleteRequest) IgnoreOwners() bool {
	var r bool
	if o.IgnoreOwnersPtr == nil {
		return r
	}
	r = *o.IgnoreOwnersPtr
	return r
}

// SetIgnoreOwners is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteRequest) SetIgnoreOwners(newValue bool) *SnapshotDeleteRequest {
	o.IgnoreOwnersPtr = &newValue
	return o
}

// Snapshot is a 'getter' method
func (o *SnapshotDeleteRequest) Snapshot() string {
	var r string
	if o.SnapshotPtr == nil {
		return r
	}
	r = *o.SnapshotPtr
	return r
}

// SetSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteRequest) SetSnapshot(newValue string) *SnapshotDeleteRequest {
	o.SnapshotPtr = &newValue
	return o
}

// SnapshotInstanceUuid is a 'getter' method
func (o *SnapshotDeleteRequest) SnapshotInstanceUuid() UUIDType {
	var r UUIDType
	if o.SnapshotInstanceUuidPtr == nil {
		return r
	}
	r = *o.SnapshotInstanceUuidPtr
	return r
}

// SetSnapshotInstanceUuid is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteRequest) SetSnapshotInstanceUuid(newValue UUIDType) *SnapshotDeleteRequest {
	o.SnapshotInstanceUuidPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *SnapshotDeleteRequest) Volume() string {
	var r string
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteRequest) SetVolume(newValue string) *SnapshotDeleteRequest {
	o.VolumePtr = &newValue
	return o
}
