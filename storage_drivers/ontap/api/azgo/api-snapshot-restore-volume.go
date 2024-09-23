// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapshotRestoreVolumeRequest is a structure to represent a snapshot-restore-volume Request ZAPI object
type SnapshotRestoreVolumeRequest struct {
	XMLName                 xml.Name  `xml:"snapshot-restore-volume"`
	ForcePtr                *bool     `xml:"force"`
	PreserveLunIdsPtr       *bool     `xml:"preserve-lun-ids"`
	SnapshotPtr             *string   `xml:"snapshot"`
	SnapshotInstanceUuidPtr *UUIDType `xml:"snapshot-instance-uuid"`
	VolumePtr               *string   `xml:"volume"`
}

// SnapshotRestoreVolumeResponse is a structure to represent a snapshot-restore-volume Response ZAPI object
type SnapshotRestoreVolumeResponse struct {
	XMLName         xml.Name                            `xml:"netapp"`
	ResponseVersion string                              `xml:"version,attr"`
	ResponseXmlns   string                              `xml:"xmlns,attr"`
	Result          SnapshotRestoreVolumeResponseResult `xml:"results"`
}

// NewSnapshotRestoreVolumeResponse is a factory method for creating new instances of SnapshotRestoreVolumeResponse objects
func NewSnapshotRestoreVolumeResponse() *SnapshotRestoreVolumeResponse {
	return &SnapshotRestoreVolumeResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotRestoreVolumeResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapshotRestoreVolumeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapshotRestoreVolumeResponseResult is a structure to represent a snapshot-restore-volume Response Result ZAPI object
type SnapshotRestoreVolumeResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewSnapshotRestoreVolumeRequest is a factory method for creating new instances of SnapshotRestoreVolumeRequest objects
func NewSnapshotRestoreVolumeRequest() *SnapshotRestoreVolumeRequest {
	return &SnapshotRestoreVolumeRequest{}
}

// NewSnapshotRestoreVolumeResponseResult is a factory method for creating new instances of SnapshotRestoreVolumeResponseResult objects
func NewSnapshotRestoreVolumeResponseResult() *SnapshotRestoreVolumeResponseResult {
	return &SnapshotRestoreVolumeResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapshotRestoreVolumeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapshotRestoreVolumeResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotRestoreVolumeRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotRestoreVolumeResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotRestoreVolumeRequest) ExecuteUsing(zr *ZapiRunner) (*SnapshotRestoreVolumeResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotRestoreVolumeRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapshotRestoreVolumeResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapshotRestoreVolumeRequest", NewSnapshotRestoreVolumeResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapshotRestoreVolumeResponse), err
}

// Force is a 'getter' method
func (o *SnapshotRestoreVolumeRequest) Force() bool {
	var r bool
	if o.ForcePtr == nil {
		return r
	}
	r = *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeRequest) SetForce(newValue bool) *SnapshotRestoreVolumeRequest {
	o.ForcePtr = &newValue
	return o
}

// PreserveLunIds is a 'getter' method
func (o *SnapshotRestoreVolumeRequest) PreserveLunIds() bool {
	var r bool
	if o.PreserveLunIdsPtr == nil {
		return r
	}
	r = *o.PreserveLunIdsPtr
	return r
}

// SetPreserveLunIds is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeRequest) SetPreserveLunIds(newValue bool) *SnapshotRestoreVolumeRequest {
	o.PreserveLunIdsPtr = &newValue
	return o
}

// Snapshot is a 'getter' method
func (o *SnapshotRestoreVolumeRequest) Snapshot() string {
	var r string
	if o.SnapshotPtr == nil {
		return r
	}
	r = *o.SnapshotPtr
	return r
}

// SetSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeRequest) SetSnapshot(newValue string) *SnapshotRestoreVolumeRequest {
	o.SnapshotPtr = &newValue
	return o
}

// SnapshotInstanceUuid is a 'getter' method
func (o *SnapshotRestoreVolumeRequest) SnapshotInstanceUuid() UUIDType {
	var r UUIDType
	if o.SnapshotInstanceUuidPtr == nil {
		return r
	}
	r = *o.SnapshotInstanceUuidPtr
	return r
}

// SetSnapshotInstanceUuid is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeRequest) SetSnapshotInstanceUuid(newValue UUIDType) *SnapshotRestoreVolumeRequest {
	o.SnapshotInstanceUuidPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *SnapshotRestoreVolumeRequest) Volume() string {
	var r string
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeRequest) SetVolume(newValue string) *SnapshotRestoreVolumeRequest {
	o.VolumePtr = &newValue
	return o
}
