// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorReleaseIterRequest is a structure to represent a snapmirror-release-iter Request ZAPI object
type SnapmirrorReleaseIterRequest struct {
	XMLName                    xml.Name                           `xml:"snapmirror-release-iter"`
	ContinueOnFailurePtr       *bool                              `xml:"continue-on-failure"`
	DestinationEndpointUuidPtr *UuidType                          `xml:"destination-endpoint-uuid"`
	MaxFailureCountPtr         *int                               `xml:"max-failure-count"`
	MaxRecordsPtr              *int                               `xml:"max-records"`
	QueryPtr                   *SnapmirrorReleaseIterRequestQuery `xml:"query"`
	RelationshipInfoOnlyPtr    *bool                              `xml:"relationship-info-only"`
	ReturnFailureListPtr       *bool                              `xml:"return-failure-list"`
	ReturnSuccessListPtr       *bool                              `xml:"return-success-list"`
	TagPtr                     *string                            `xml:"tag"`
}

// SnapmirrorReleaseIterResponse is a structure to represent a snapmirror-release-iter Response ZAPI object
type SnapmirrorReleaseIterResponse struct {
	XMLName         xml.Name                            `xml:"netapp"`
	ResponseVersion string                              `xml:"version,attr"`
	ResponseXmlns   string                              `xml:"xmlns,attr"`
	Result          SnapmirrorReleaseIterResponseResult `xml:"results"`
}

// NewSnapmirrorReleaseIterResponse is a factory method for creating new instances of SnapmirrorReleaseIterResponse objects
func NewSnapmirrorReleaseIterResponse() *SnapmirrorReleaseIterResponse {
	return &SnapmirrorReleaseIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorReleaseIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorReleaseIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorReleaseIterResponseResult is a structure to represent a snapmirror-release-iter Response Result ZAPI object
type SnapmirrorReleaseIterResponseResult struct {
	XMLName          xml.Name                                        `xml:"results"`
	ResultStatusAttr string                                          `xml:"status,attr"`
	ResultReasonAttr string                                          `xml:"reason,attr"`
	ResultErrnoAttr  string                                          `xml:"errno,attr"`
	FailureListPtr   *SnapmirrorReleaseIterResponseResultFailureList `xml:"failure-list"`
	NextTagPtr       *string                                         `xml:"next-tag"`
	NumFailedPtr     *int                                            `xml:"num-failed"`
	NumSucceededPtr  *int                                            `xml:"num-succeeded"`
	SuccessListPtr   *SnapmirrorReleaseIterResponseResultSuccessList `xml:"success-list"`
}

// NewSnapmirrorReleaseIterRequest is a factory method for creating new instances of SnapmirrorReleaseIterRequest objects
func NewSnapmirrorReleaseIterRequest() *SnapmirrorReleaseIterRequest {
	return &SnapmirrorReleaseIterRequest{}
}

// NewSnapmirrorReleaseIterResponseResult is a factory method for creating new instances of SnapmirrorReleaseIterResponseResult objects
func NewSnapmirrorReleaseIterResponseResult() *SnapmirrorReleaseIterResponseResult {
	return &SnapmirrorReleaseIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorReleaseIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorReleaseIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorReleaseIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorReleaseIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorReleaseIterRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorReleaseIterResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorReleaseIterRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorReleaseIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorReleaseIterRequest", NewSnapmirrorReleaseIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorReleaseIterResponse), err
}

// ContinueOnFailure is a 'getter' method
func (o *SnapmirrorReleaseIterRequest) ContinueOnFailure() bool {
	var r bool
	if o.ContinueOnFailurePtr == nil {
		return r
	}
	r = *o.ContinueOnFailurePtr
	return r
}

// SetContinueOnFailure is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterRequest) SetContinueOnFailure(newValue bool) *SnapmirrorReleaseIterRequest {
	o.ContinueOnFailurePtr = &newValue
	return o
}

// DestinationEndpointUuid is a 'getter' method
func (o *SnapmirrorReleaseIterRequest) DestinationEndpointUuid() UuidType {
	var r UuidType
	if o.DestinationEndpointUuidPtr == nil {
		return r
	}
	r = *o.DestinationEndpointUuidPtr
	return r
}

// SetDestinationEndpointUuid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterRequest) SetDestinationEndpointUuid(newValue UuidType) *SnapmirrorReleaseIterRequest {
	o.DestinationEndpointUuidPtr = &newValue
	return o
}

// MaxFailureCount is a 'getter' method
func (o *SnapmirrorReleaseIterRequest) MaxFailureCount() int {
	var r int
	if o.MaxFailureCountPtr == nil {
		return r
	}
	r = *o.MaxFailureCountPtr
	return r
}

// SetMaxFailureCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterRequest) SetMaxFailureCount(newValue int) *SnapmirrorReleaseIterRequest {
	o.MaxFailureCountPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *SnapmirrorReleaseIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterRequest) SetMaxRecords(newValue int) *SnapmirrorReleaseIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// SnapmirrorReleaseIterRequestQuery is a wrapper
type SnapmirrorReleaseIterRequestQuery struct {
	XMLName                      xml.Name                       `xml:"query"`
	SnapmirrorDestinationInfoPtr *SnapmirrorDestinationInfoType `xml:"snapmirror-destination-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorReleaseIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorDestinationInfo is a 'getter' method
func (o *SnapmirrorReleaseIterRequestQuery) SnapmirrorDestinationInfo() SnapmirrorDestinationInfoType {
	var r SnapmirrorDestinationInfoType
	if o.SnapmirrorDestinationInfoPtr == nil {
		return r
	}
	r = *o.SnapmirrorDestinationInfoPtr
	return r
}

// SetSnapmirrorDestinationInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterRequestQuery) SetSnapmirrorDestinationInfo(newValue SnapmirrorDestinationInfoType) *SnapmirrorReleaseIterRequestQuery {
	o.SnapmirrorDestinationInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *SnapmirrorReleaseIterRequest) Query() SnapmirrorReleaseIterRequestQuery {
	var r SnapmirrorReleaseIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterRequest) SetQuery(newValue SnapmirrorReleaseIterRequestQuery) *SnapmirrorReleaseIterRequest {
	o.QueryPtr = &newValue
	return o
}

// RelationshipInfoOnly is a 'getter' method
func (o *SnapmirrorReleaseIterRequest) RelationshipInfoOnly() bool {
	var r bool
	if o.RelationshipInfoOnlyPtr == nil {
		return r
	}
	r = *o.RelationshipInfoOnlyPtr
	return r
}

// SetRelationshipInfoOnly is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterRequest) SetRelationshipInfoOnly(newValue bool) *SnapmirrorReleaseIterRequest {
	o.RelationshipInfoOnlyPtr = &newValue
	return o
}

// ReturnFailureList is a 'getter' method
func (o *SnapmirrorReleaseIterRequest) ReturnFailureList() bool {
	var r bool
	if o.ReturnFailureListPtr == nil {
		return r
	}
	r = *o.ReturnFailureListPtr
	return r
}

// SetReturnFailureList is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterRequest) SetReturnFailureList(newValue bool) *SnapmirrorReleaseIterRequest {
	o.ReturnFailureListPtr = &newValue
	return o
}

// ReturnSuccessList is a 'getter' method
func (o *SnapmirrorReleaseIterRequest) ReturnSuccessList() bool {
	var r bool
	if o.ReturnSuccessListPtr == nil {
		return r
	}
	r = *o.ReturnSuccessListPtr
	return r
}

// SetReturnSuccessList is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterRequest) SetReturnSuccessList(newValue bool) *SnapmirrorReleaseIterRequest {
	o.ReturnSuccessListPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *SnapmirrorReleaseIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterRequest) SetTag(newValue string) *SnapmirrorReleaseIterRequest {
	o.TagPtr = &newValue
	return o
}

// SnapmirrorReleaseIterResponseResultFailureList is a wrapper
type SnapmirrorReleaseIterResponseResultFailureList struct {
	XMLName                      xml.Name                        `xml:"failure-list"`
	SnapmirrorReleaseIterInfoPtr []SnapmirrorReleaseIterInfoType `xml:"snapmirror-release-iter-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorReleaseIterResponseResultFailureList) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorReleaseIterInfo is a 'getter' method
func (o *SnapmirrorReleaseIterResponseResultFailureList) SnapmirrorReleaseIterInfo() []SnapmirrorReleaseIterInfoType {
	r := o.SnapmirrorReleaseIterInfoPtr
	return r
}

// SetSnapmirrorReleaseIterInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterResponseResultFailureList) SetSnapmirrorReleaseIterInfo(newValue []SnapmirrorReleaseIterInfoType) *SnapmirrorReleaseIterResponseResultFailureList {
	newSlice := make([]SnapmirrorReleaseIterInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapmirrorReleaseIterInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *SnapmirrorReleaseIterResponseResultFailureList) values() []SnapmirrorReleaseIterInfoType {
	r := o.SnapmirrorReleaseIterInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterResponseResultFailureList) setValues(newValue []SnapmirrorReleaseIterInfoType) *SnapmirrorReleaseIterResponseResultFailureList {
	newSlice := make([]SnapmirrorReleaseIterInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapmirrorReleaseIterInfoPtr = newSlice
	return o
}

// FailureList is a 'getter' method
func (o *SnapmirrorReleaseIterResponseResult) FailureList() SnapmirrorReleaseIterResponseResultFailureList {
	var r SnapmirrorReleaseIterResponseResultFailureList
	if o.FailureListPtr == nil {
		return r
	}
	r = *o.FailureListPtr
	return r
}

// SetFailureList is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterResponseResult) SetFailureList(newValue SnapmirrorReleaseIterResponseResultFailureList) *SnapmirrorReleaseIterResponseResult {
	o.FailureListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *SnapmirrorReleaseIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterResponseResult) SetNextTag(newValue string) *SnapmirrorReleaseIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumFailed is a 'getter' method
func (o *SnapmirrorReleaseIterResponseResult) NumFailed() int {
	var r int
	if o.NumFailedPtr == nil {
		return r
	}
	r = *o.NumFailedPtr
	return r
}

// SetNumFailed is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterResponseResult) SetNumFailed(newValue int) *SnapmirrorReleaseIterResponseResult {
	o.NumFailedPtr = &newValue
	return o
}

// NumSucceeded is a 'getter' method
func (o *SnapmirrorReleaseIterResponseResult) NumSucceeded() int {
	var r int
	if o.NumSucceededPtr == nil {
		return r
	}
	r = *o.NumSucceededPtr
	return r
}

// SetNumSucceeded is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterResponseResult) SetNumSucceeded(newValue int) *SnapmirrorReleaseIterResponseResult {
	o.NumSucceededPtr = &newValue
	return o
}

// SnapmirrorReleaseIterResponseResultSuccessList is a wrapper
type SnapmirrorReleaseIterResponseResultSuccessList struct {
	XMLName                      xml.Name                        `xml:"success-list"`
	SnapmirrorReleaseIterInfoPtr []SnapmirrorReleaseIterInfoType `xml:"snapmirror-release-iter-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorReleaseIterResponseResultSuccessList) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorReleaseIterInfo is a 'getter' method
func (o *SnapmirrorReleaseIterResponseResultSuccessList) SnapmirrorReleaseIterInfo() []SnapmirrorReleaseIterInfoType {
	r := o.SnapmirrorReleaseIterInfoPtr
	return r
}

// SetSnapmirrorReleaseIterInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterResponseResultSuccessList) SetSnapmirrorReleaseIterInfo(newValue []SnapmirrorReleaseIterInfoType) *SnapmirrorReleaseIterResponseResultSuccessList {
	newSlice := make([]SnapmirrorReleaseIterInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapmirrorReleaseIterInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *SnapmirrorReleaseIterResponseResultSuccessList) values() []SnapmirrorReleaseIterInfoType {
	r := o.SnapmirrorReleaseIterInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterResponseResultSuccessList) setValues(newValue []SnapmirrorReleaseIterInfoType) *SnapmirrorReleaseIterResponseResultSuccessList {
	newSlice := make([]SnapmirrorReleaseIterInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapmirrorReleaseIterInfoPtr = newSlice
	return o
}

// SuccessList is a 'getter' method
func (o *SnapmirrorReleaseIterResponseResult) SuccessList() SnapmirrorReleaseIterResponseResultSuccessList {
	var r SnapmirrorReleaseIterResponseResultSuccessList
	if o.SuccessListPtr == nil {
		return r
	}
	r = *o.SuccessListPtr
	return r
}

// SetSuccessList is a fluent style 'setter' method that can be chained
func (o *SnapmirrorReleaseIterResponseResult) SetSuccessList(newValue SnapmirrorReleaseIterResponseResultSuccessList) *SnapmirrorReleaseIterResponseResult {
	o.SuccessListPtr = &newValue
	return o
}
