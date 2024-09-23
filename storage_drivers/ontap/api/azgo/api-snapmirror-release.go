// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorReleaseRequest is a structure to represent a snapmirror-release Request ZAPI object
type SnapmirrorReleaseRequest struct {
	XMLName                    xml.Name  `xml:"snapmirror-release"`
	DestinationEndpointUuidPtr *UuidType `xml:"destination-endpoint-uuid"`
	DestinationLocationPtr     *string   `xml:"destination-location"`
	DestinationVolumePtr       *string   `xml:"destination-volume"`
	DestinationVserverPtr      *string   `xml:"destination-vserver"`
	RelationshipIdPtr          *string   `xml:"relationship-id"`
	RelationshipInfoOnlyPtr    *bool     `xml:"relationship-info-only"`
	SourceLocationPtr          *string   `xml:"source-location"`
	SourceVolumePtr            *string   `xml:"source-volume"`
	SourceVserverPtr           *string   `xml:"source-vserver"`
}

// SnapmirrorReleaseResponse is a structure to represent a snapmirror-release Response ZAPI object
type SnapmirrorReleaseResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          SnapmirrorReleaseResponseResult `xml:"results"`
}

// NewSnapmirrorReleaseResponse is a factory method for creating new instances of SnapmirrorReleaseResponse objects
func NewSnapmirrorReleaseResponse() *SnapmirrorReleaseResponse {
	return &SnapmirrorReleaseResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorReleaseResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorReleaseResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorReleaseResponseResult is a structure to represent a snapmirror-release Response Result ZAPI object
type SnapmirrorReleaseResponseResult struct {
	XMLName              xml.Name `xml:"results"`
	ResultStatusAttr     string   `xml:"status,attr"`
	ResultReasonAttr     string   `xml:"reason,attr"`
	ResultErrnoAttr      string   `xml:"errno,attr"`
	ResultOperationIdPtr *string  `xml:"result-operation-id"`
}

// NewSnapmirrorReleaseRequest is a factory method for creating new instances of SnapmirrorReleaseRequest objects
func NewSnapmirrorReleaseRequest() *SnapmirrorReleaseRequest {
	return &SnapmirrorReleaseRequest{}
}

// NewSnapmirrorReleaseResponseResult is a factory method for creating new instances of SnapmirrorReleaseResponseResult objects
func NewSnapmirrorReleaseResponseResult() *SnapmirrorReleaseResponseResult {
	return &SnapmirrorReleaseResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorReleaseRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorReleaseResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorReleaseRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorReleaseResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorReleaseRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorReleaseResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorReleaseRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorReleaseResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorReleaseRequest", NewSnapmirrorReleaseResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorReleaseResponse), err
}

// DestinationEndpointUuid is a 'getter' method
func (o *SnapmirrorReleaseRequest) DestinationEndpointUuid() UuidType {
	var r UuidType
	if o.DestinationEndpointUuidPtr == nil {
		return r
	}
	r = *o.DestinationEndpointUuidPtr
	return r
}

// SetDestinationEndpointUuid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseRequest) SetDestinationEndpointUuid(newValue UuidType) *SnapmirrorReleaseRequest {
	o.DestinationEndpointUuidPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorReleaseRequest) DestinationLocation() string {
	var r string
	if o.DestinationLocationPtr == nil {
		return r
	}
	r = *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseRequest) SetDestinationLocation(newValue string) *SnapmirrorReleaseRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorReleaseRequest) DestinationVolume() string {
	var r string
	if o.DestinationVolumePtr == nil {
		return r
	}
	r = *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseRequest) SetDestinationVolume(newValue string) *SnapmirrorReleaseRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorReleaseRequest) DestinationVserver() string {
	var r string
	if o.DestinationVserverPtr == nil {
		return r
	}
	r = *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseRequest) SetDestinationVserver(newValue string) *SnapmirrorReleaseRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// RelationshipId is a 'getter' method
func (o *SnapmirrorReleaseRequest) RelationshipId() string {
	var r string
	if o.RelationshipIdPtr == nil {
		return r
	}
	r = *o.RelationshipIdPtr
	return r
}

// SetRelationshipId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseRequest) SetRelationshipId(newValue string) *SnapmirrorReleaseRequest {
	o.RelationshipIdPtr = &newValue
	return o
}

// RelationshipInfoOnly is a 'getter' method
func (o *SnapmirrorReleaseRequest) RelationshipInfoOnly() bool {
	var r bool
	if o.RelationshipInfoOnlyPtr == nil {
		return r
	}
	r = *o.RelationshipInfoOnlyPtr
	return r
}

// SetRelationshipInfoOnly is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseRequest) SetRelationshipInfoOnly(newValue bool) *SnapmirrorReleaseRequest {
	o.RelationshipInfoOnlyPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorReleaseRequest) SourceLocation() string {
	var r string
	if o.SourceLocationPtr == nil {
		return r
	}
	r = *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseRequest) SetSourceLocation(newValue string) *SnapmirrorReleaseRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorReleaseRequest) SourceVolume() string {
	var r string
	if o.SourceVolumePtr == nil {
		return r
	}
	r = *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseRequest) SetSourceVolume(newValue string) *SnapmirrorReleaseRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorReleaseRequest) SourceVserver() string {
	var r string
	if o.SourceVserverPtr == nil {
		return r
	}
	r = *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseRequest) SetSourceVserver(newValue string) *SnapmirrorReleaseRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// ResultOperationId is a 'getter' method
func (o *SnapmirrorReleaseResponseResult) ResultOperationId() string {
	var r string
	if o.ResultOperationIdPtr == nil {
		return r
	}
	r = *o.ResultOperationIdPtr
	return r
}

// SetResultOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseResponseResult) SetResultOperationId(newValue string) *SnapmirrorReleaseResponseResult {
	o.ResultOperationIdPtr = &newValue
	return o
}
