// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SystemGetVersionRequest is a structure to represent a system-get-version Request ZAPI object
type SystemGetVersionRequest struct {
	XMLName xml.Name `xml:"system-get-version"`
}

// SystemGetVersionResponse is a structure to represent a system-get-version Response ZAPI object
type SystemGetVersionResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          SystemGetVersionResponseResult `xml:"results"`
}

// NewSystemGetVersionResponse is a factory method for creating new instances of SystemGetVersionResponse objects
func NewSystemGetVersionResponse() *SystemGetVersionResponse {
	return &SystemGetVersionResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetVersionResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SystemGetVersionResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SystemGetVersionResponseResult is a structure to represent a system-get-version Response Result ZAPI object
type SystemGetVersionResponseResult struct {
	XMLName               xml.Name                                          `xml:"results"`
	ResultStatusAttr      string                                            `xml:"status,attr"`
	ResultReasonAttr      string                                            `xml:"reason,attr"`
	ResultErrnoAttr       string                                            `xml:"errno,attr"`
	BuildTimestampPtr     *int                                              `xml:"build-timestamp"`
	IsClusteredPtr        *bool                                             `xml:"is-clustered"`
	NodeVersionDetailsPtr *SystemGetVersionResponseResultNodeVersionDetails `xml:"node-version-details"`
	VersionPtr            *string                                           `xml:"version"`
	VersionTuplePtr       *SystemGetVersionResponseResultVersionTuple       `xml:"version-tuple"`
}

// NewSystemGetVersionRequest is a factory method for creating new instances of SystemGetVersionRequest objects
func NewSystemGetVersionRequest() *SystemGetVersionRequest {
	return &SystemGetVersionRequest{}
}

// NewSystemGetVersionResponseResult is a factory method for creating new instances of SystemGetVersionResponseResult objects
func NewSystemGetVersionResponseResult() *SystemGetVersionResponseResult {
	return &SystemGetVersionResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SystemGetVersionRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SystemGetVersionResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetVersionRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetVersionResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SystemGetVersionRequest) ExecuteUsing(zr *ZapiRunner) (*SystemGetVersionResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SystemGetVersionRequest) executeWithoutIteration(zr *ZapiRunner) (*SystemGetVersionResponse, error) {
	result, err := zr.ExecuteUsing(o, "SystemGetVersionRequest", NewSystemGetVersionResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SystemGetVersionResponse), err
}

// BuildTimestamp is a 'getter' method
func (o *SystemGetVersionResponseResult) BuildTimestamp() int {
	var r int
	if o.BuildTimestampPtr == nil {
		return r
	}
	r = *o.BuildTimestampPtr
	return r
}

// SetBuildTimestamp is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResult) SetBuildTimestamp(newValue int) *SystemGetVersionResponseResult {
	o.BuildTimestampPtr = &newValue
	return o
}

// IsClustered is a 'getter' method
func (o *SystemGetVersionResponseResult) IsClustered() bool {
	var r bool
	if o.IsClusteredPtr == nil {
		return r
	}
	r = *o.IsClusteredPtr
	return r
}

// SetIsClustered is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResult) SetIsClustered(newValue bool) *SystemGetVersionResponseResult {
	o.IsClusteredPtr = &newValue
	return o
}

// SystemGetVersionResponseResultNodeVersionDetails is a wrapper
type SystemGetVersionResponseResultNodeVersionDetails struct {
	XMLName                  xml.Name                    `xml:"node-version-details"`
	NodeVersionDetailInfoPtr []NodeVersionDetailInfoType `xml:"node-version-detail-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetVersionResponseResultNodeVersionDetails) String() string {
	return ToString(reflect.ValueOf(o))
}

// NodeVersionDetailInfo is a 'getter' method
func (o *SystemGetVersionResponseResultNodeVersionDetails) NodeVersionDetailInfo() []NodeVersionDetailInfoType {
	r := o.NodeVersionDetailInfoPtr
	return r
}

// SetNodeVersionDetailInfo is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResultNodeVersionDetails) SetNodeVersionDetailInfo(newValue []NodeVersionDetailInfoType) *SystemGetVersionResponseResultNodeVersionDetails {
	newSlice := make([]NodeVersionDetailInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NodeVersionDetailInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *SystemGetVersionResponseResultNodeVersionDetails) values() []NodeVersionDetailInfoType {
	r := o.NodeVersionDetailInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResultNodeVersionDetails) setValues(newValue []NodeVersionDetailInfoType) *SystemGetVersionResponseResultNodeVersionDetails {
	newSlice := make([]NodeVersionDetailInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NodeVersionDetailInfoPtr = newSlice
	return o
}

// NodeVersionDetails is a 'getter' method
func (o *SystemGetVersionResponseResult) NodeVersionDetails() SystemGetVersionResponseResultNodeVersionDetails {
	var r SystemGetVersionResponseResultNodeVersionDetails
	if o.NodeVersionDetailsPtr == nil {
		return r
	}
	r = *o.NodeVersionDetailsPtr
	return r
}

// SetNodeVersionDetails is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResult) SetNodeVersionDetails(newValue SystemGetVersionResponseResultNodeVersionDetails) *SystemGetVersionResponseResult {
	o.NodeVersionDetailsPtr = &newValue
	return o
}

// Version is a 'getter' method
func (o *SystemGetVersionResponseResult) Version() string {
	var r string
	if o.VersionPtr == nil {
		return r
	}
	r = *o.VersionPtr
	return r
}

// SetVersion is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResult) SetVersion(newValue string) *SystemGetVersionResponseResult {
	o.VersionPtr = &newValue
	return o
}

// SystemGetVersionResponseResultVersionTuple is a wrapper
type SystemGetVersionResponseResultVersionTuple struct {
	XMLName               xml.Name                `xml:"version-tuple"`
	SystemVersionTuplePtr *SystemVersionTupleType `xml:"system-version-tuple"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemGetVersionResponseResultVersionTuple) String() string {
	return ToString(reflect.ValueOf(o))
}

// SystemVersionTuple is a 'getter' method
func (o *SystemGetVersionResponseResultVersionTuple) SystemVersionTuple() SystemVersionTupleType {
	var r SystemVersionTupleType
	if o.SystemVersionTuplePtr == nil {
		return r
	}
	r = *o.SystemVersionTuplePtr
	return r
}

// SetSystemVersionTuple is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResultVersionTuple) SetSystemVersionTuple(newValue SystemVersionTupleType) *SystemGetVersionResponseResultVersionTuple {
	o.SystemVersionTuplePtr = &newValue
	return o
}

// values is a 'getter' method
func (o *SystemGetVersionResponseResultVersionTuple) values() SystemVersionTupleType {
	var r SystemVersionTupleType
	if o.SystemVersionTuplePtr == nil {
		return r
	}
	r = *o.SystemVersionTuplePtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResultVersionTuple) setValues(newValue SystemVersionTupleType) *SystemGetVersionResponseResultVersionTuple {
	o.SystemVersionTuplePtr = &newValue
	return o
}

// VersionTuple is a 'getter' method
func (o *SystemGetVersionResponseResult) VersionTuple() SystemGetVersionResponseResultVersionTuple {
	var r SystemGetVersionResponseResultVersionTuple
	if o.VersionTuplePtr == nil {
		return r
	}
	r = *o.VersionTuplePtr
	return r
}

// SetVersionTuple is a fluent style 'setter' method that can be chained
func (o *SystemGetVersionResponseResult) SetVersionTuple(newValue SystemGetVersionResponseResultVersionTuple) *SystemGetVersionResponseResult {
	o.VersionTuplePtr = &newValue
	return o
}
