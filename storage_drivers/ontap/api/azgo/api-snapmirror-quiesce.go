// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SnapmirrorQuiesceRequest is a structure to represent a snapmirror-quiesce Request ZAPI object
type SnapmirrorQuiesceRequest struct {
	XMLName                xml.Name `xml:"snapmirror-quiesce"`
	DestinationLocationPtr *string  `xml:"destination-location"`
	DestinationVolumePtr   *string  `xml:"destination-volume"`
	DestinationVserverPtr  *string  `xml:"destination-vserver"`
	SourceLocationPtr      *string  `xml:"source-location"`
	SourceVolumePtr        *string  `xml:"source-volume"`
	SourceVserverPtr       *string  `xml:"source-vserver"`
}

// SnapmirrorQuiesceResponse is a structure to represent a snapmirror-quiesce Response ZAPI object
type SnapmirrorQuiesceResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          SnapmirrorQuiesceResponseResult `xml:"results"`
}

// NewSnapmirrorQuiesceResponse is a factory method for creating new instances of SnapmirrorQuiesceResponse objects
func NewSnapmirrorQuiesceResponse() *SnapmirrorQuiesceResponse {
	return &SnapmirrorQuiesceResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorQuiesceResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorQuiesceResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorQuiesceResponseResult is a structure to represent a snapmirror-quiesce Response Result ZAPI object
type SnapmirrorQuiesceResponseResult struct {
	XMLName              xml.Name `xml:"results"`
	ResultStatusAttr     string   `xml:"status,attr"`
	ResultReasonAttr     string   `xml:"reason,attr"`
	ResultErrnoAttr      string   `xml:"errno,attr"`
	ResultOperationIdPtr *string  `xml:"result-operation-id"`
}

// NewSnapmirrorQuiesceRequest is a factory method for creating new instances of SnapmirrorQuiesceRequest objects
func NewSnapmirrorQuiesceRequest() *SnapmirrorQuiesceRequest {
	return &SnapmirrorQuiesceRequest{}
}

// NewSnapmirrorQuiesceResponseResult is a factory method for creating new instances of SnapmirrorQuiesceResponseResult objects
func NewSnapmirrorQuiesceResponseResult() *SnapmirrorQuiesceResponseResult {
	return &SnapmirrorQuiesceResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorQuiesceRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorQuiesceResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorQuiesceRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorQuiesceResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorQuiesceRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorQuiesceResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorQuiesceRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorQuiesceResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorQuiesceRequest", NewSnapmirrorQuiesceResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorQuiesceResponse), err
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorQuiesceRequest) DestinationLocation() string {
	var r string
	if o.DestinationLocationPtr == nil {
		return r
	}
	r = *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetDestinationLocation(newValue string) *SnapmirrorQuiesceRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorQuiesceRequest) DestinationVolume() string {
	var r string
	if o.DestinationVolumePtr == nil {
		return r
	}
	r = *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetDestinationVolume(newValue string) *SnapmirrorQuiesceRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorQuiesceRequest) DestinationVserver() string {
	var r string
	if o.DestinationVserverPtr == nil {
		return r
	}
	r = *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetDestinationVserver(newValue string) *SnapmirrorQuiesceRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorQuiesceRequest) SourceLocation() string {
	var r string
	if o.SourceLocationPtr == nil {
		return r
	}
	r = *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetSourceLocation(newValue string) *SnapmirrorQuiesceRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorQuiesceRequest) SourceVolume() string {
	var r string
	if o.SourceVolumePtr == nil {
		return r
	}
	r = *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetSourceVolume(newValue string) *SnapmirrorQuiesceRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorQuiesceRequest) SourceVserver() string {
	var r string
	if o.SourceVserverPtr == nil {
		return r
	}
	r = *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetSourceVserver(newValue string) *SnapmirrorQuiesceRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// ResultOperationId is a 'getter' method
func (o *SnapmirrorQuiesceResponseResult) ResultOperationId() string {
	var r string
	if o.ResultOperationIdPtr == nil {
		return r
	}
	r = *o.ResultOperationIdPtr
	return r
}

// SetResultOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceResponseResult) SetResultOperationId(newValue string) *SnapmirrorQuiesceResponseResult {
	o.ResultOperationIdPtr = &newValue
	return o
}
