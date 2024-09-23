// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapshotCreateRequest is a structure to represent a snapshot-create Request ZAPI object
type SnapshotCreateRequest struct {
	XMLName            xml.Name `xml:"snapshot-create"`
	AsyncPtr           *bool    `xml:"async"`
	CommentPtr         *string  `xml:"comment"`
	SnapmirrorLabelPtr *string  `xml:"snapmirror-label"`
	SnapshotPtr        *string  `xml:"snapshot"`
	VolumePtr          *string  `xml:"volume"`
}

// SnapshotCreateResponse is a structure to represent a snapshot-create Response ZAPI object
type SnapshotCreateResponse struct {
	XMLName         xml.Name                     `xml:"netapp"`
	ResponseVersion string                       `xml:"version,attr"`
	ResponseXmlns   string                       `xml:"xmlns,attr"`
	Result          SnapshotCreateResponseResult `xml:"results"`
}

// NewSnapshotCreateResponse is a factory method for creating new instances of SnapshotCreateResponse objects
func NewSnapshotCreateResponse() *SnapshotCreateResponse {
	return &SnapshotCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapshotCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapshotCreateResponseResult is a structure to represent a snapshot-create Response Result ZAPI object
type SnapshotCreateResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewSnapshotCreateRequest is a factory method for creating new instances of SnapshotCreateRequest objects
func NewSnapshotCreateRequest() *SnapshotCreateRequest {
	return &SnapshotCreateRequest{}
}

// NewSnapshotCreateResponseResult is a factory method for creating new instances of SnapshotCreateResponseResult objects
func NewSnapshotCreateResponseResult() *SnapshotCreateResponseResult {
	return &SnapshotCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapshotCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapshotCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotCreateRequest) ExecuteUsing(zr *ZapiRunner) (*SnapshotCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapshotCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapshotCreateRequest", NewSnapshotCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapshotCreateResponse), err
}

// Async is a 'getter' method
func (o *SnapshotCreateRequest) Async() bool {
	var r bool
	if o.AsyncPtr == nil {
		return r
	}
	r = *o.AsyncPtr
	return r
}

// SetAsync is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateRequest) SetAsync(newValue bool) *SnapshotCreateRequest {
	o.AsyncPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *SnapshotCreateRequest) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateRequest) SetComment(newValue string) *SnapshotCreateRequest {
	o.CommentPtr = &newValue
	return o
}

// SnapmirrorLabel is a 'getter' method
func (o *SnapshotCreateRequest) SnapmirrorLabel() string {
	var r string
	if o.SnapmirrorLabelPtr == nil {
		return r
	}
	r = *o.SnapmirrorLabelPtr
	return r
}

// SetSnapmirrorLabel is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateRequest) SetSnapmirrorLabel(newValue string) *SnapshotCreateRequest {
	o.SnapmirrorLabelPtr = &newValue
	return o
}

// Snapshot is a 'getter' method
func (o *SnapshotCreateRequest) Snapshot() string {
	var r string
	if o.SnapshotPtr == nil {
		return r
	}
	r = *o.SnapshotPtr
	return r
}

// SetSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateRequest) SetSnapshot(newValue string) *SnapshotCreateRequest {
	o.SnapshotPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *SnapshotCreateRequest) Volume() string {
	var r string
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateRequest) SetVolume(newValue string) *SnapshotCreateRequest {
	o.VolumePtr = &newValue
	return o
}
