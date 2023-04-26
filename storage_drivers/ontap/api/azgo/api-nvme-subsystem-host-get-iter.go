// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeSubsystemHostGetIterRequest is a structure to represent a nvme-subsystem-host-get-iter Request ZAPI object
type NvmeSubsystemHostGetIterRequest struct {
	XMLName              xml.Name                                          `xml:"nvme-subsystem-host-get-iter"`
	DesiredAttributesPtr *NvmeSubsystemHostGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                              `xml:"max-records"`
	QueryPtr             *NvmeSubsystemHostGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                           `xml:"tag"`
}

// NvmeSubsystemHostGetIterResponse is a structure to represent a nvme-subsystem-host-get-iter Response ZAPI object
type NvmeSubsystemHostGetIterResponse struct {
	XMLName         xml.Name                               `xml:"netapp"`
	ResponseVersion string                                 `xml:"version,attr"`
	ResponseXmlns   string                                 `xml:"xmlns,attr"`
	Result          NvmeSubsystemHostGetIterResponseResult `xml:"results"`
}

// NewNvmeSubsystemHostGetIterResponse is a factory method for creating new instances of NvmeSubsystemHostGetIterResponse objects
func NewNvmeSubsystemHostGetIterResponse() *NvmeSubsystemHostGetIterResponse {
	return &NvmeSubsystemHostGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemHostGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemHostGetIterResponseResult is a structure to represent a nvme-subsystem-host-get-iter Response Result ZAPI object
type NvmeSubsystemHostGetIterResponseResult struct {
	XMLName           xml.Name                                              `xml:"results"`
	ResultStatusAttr  string                                                `xml:"status,attr"`
	ResultReasonAttr  string                                                `xml:"reason,attr"`
	ResultErrnoAttr   string                                                `xml:"errno,attr"`
	AttributesListPtr *NvmeSubsystemHostGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                               `xml:"next-tag"`
	NumRecordsPtr     *int                                                  `xml:"num-records"`
}

// NewNvmeSubsystemHostGetIterRequest is a factory method for creating new instances of NvmeSubsystemHostGetIterRequest objects
func NewNvmeSubsystemHostGetIterRequest() *NvmeSubsystemHostGetIterRequest {
	return &NvmeSubsystemHostGetIterRequest{}
}

// NewNvmeSubsystemHostGetIterResponseResult is a factory method for creating new instances of NvmeSubsystemHostGetIterResponseResult objects
func NewNvmeSubsystemHostGetIterResponseResult() *NvmeSubsystemHostGetIterResponseResult {
	return &NvmeSubsystemHostGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemHostGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemHostGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemHostGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemHostGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemHostGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemHostGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemHostGetIterRequest", NewNvmeSubsystemHostGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemHostGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *NvmeSubsystemHostGetIterRequest) executeWithIteration(zr *ZapiRunner) (*NvmeSubsystemHostGetIterResponse, error) {
	combined := NewNvmeSubsystemHostGetIterResponse()
	combined.Result.SetAttributesList(NvmeSubsystemHostGetIterResponseResultAttributesList{})
	var nextTagPtr *string
	done := false
	for !done {
		n, err := o.executeWithoutIteration(zr)

		if err != nil {
			return nil, err
		}
		nextTagPtr = n.Result.NextTagPtr
		if nextTagPtr == nil {
			done = true
		} else {
			o.SetTag(*nextTagPtr)
		}

		if n.Result.NumRecordsPtr == nil {
			done = true
		} else {
			recordsRead := n.Result.NumRecords()
			if recordsRead == 0 {
				done = true
			}
		}

		if n.Result.AttributesListPtr != nil {
			if combined.Result.AttributesListPtr == nil {
				combined.Result.SetAttributesList(NvmeSubsystemHostGetIterResponseResultAttributesList{})
			}
			combinedAttributesList := combined.Result.AttributesList()
			combinedAttributes := combinedAttributesList.values()

			resultAttributesList := n.Result.AttributesList()
			resultAttributes := resultAttributesList.values()

			combined.Result.AttributesListPtr.setValues(append(combinedAttributes, resultAttributes...))
		}

		if done {

			combined.Result.ResultErrnoAttr = n.Result.ResultErrnoAttr
			combined.Result.ResultReasonAttr = n.Result.ResultReasonAttr
			combined.Result.ResultStatusAttr = n.Result.ResultStatusAttr

			combinedAttributesList := combined.Result.AttributesList()
			combinedAttributes := combinedAttributesList.values()
			combined.Result.SetNumRecords(len(combinedAttributes))

		}
	}
	return combined, nil
}

// NvmeSubsystemHostGetIterRequestDesiredAttributes is a wrapper
type NvmeSubsystemHostGetIterRequestDesiredAttributes struct {
	XMLName                        xml.Name                         `xml:"desired-attributes"`
	NvmeTargetSubsystemHostInfoPtr *NvmeTargetSubsystemHostInfoType `xml:"nvme-target-subsystem-host-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetSubsystemHostInfo is a 'getter' method
func (o *NvmeSubsystemHostGetIterRequestDesiredAttributes) NvmeTargetSubsystemHostInfo() NvmeTargetSubsystemHostInfoType {
	var r NvmeTargetSubsystemHostInfoType
	if o.NvmeTargetSubsystemHostInfoPtr == nil {
		return r
	}
	r = *o.NvmeTargetSubsystemHostInfoPtr
	return r
}

// SetNvmeTargetSubsystemHostInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostGetIterRequestDesiredAttributes) SetNvmeTargetSubsystemHostInfo(newValue NvmeTargetSubsystemHostInfoType) *NvmeSubsystemHostGetIterRequestDesiredAttributes {
	o.NvmeTargetSubsystemHostInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *NvmeSubsystemHostGetIterRequest) DesiredAttributes() NvmeSubsystemHostGetIterRequestDesiredAttributes {
	var r NvmeSubsystemHostGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostGetIterRequest) SetDesiredAttributes(newValue NvmeSubsystemHostGetIterRequestDesiredAttributes) *NvmeSubsystemHostGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *NvmeSubsystemHostGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostGetIterRequest) SetMaxRecords(newValue int) *NvmeSubsystemHostGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// NvmeSubsystemHostGetIterRequestQuery is a wrapper
type NvmeSubsystemHostGetIterRequestQuery struct {
	XMLName                        xml.Name                         `xml:"query"`
	NvmeTargetSubsystemHostInfoPtr *NvmeTargetSubsystemHostInfoType `xml:"nvme-target-subsystem-host-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetSubsystemHostInfo is a 'getter' method
func (o *NvmeSubsystemHostGetIterRequestQuery) NvmeTargetSubsystemHostInfo() NvmeTargetSubsystemHostInfoType {
	var r NvmeTargetSubsystemHostInfoType
	if o.NvmeTargetSubsystemHostInfoPtr == nil {
		return r
	}
	r = *o.NvmeTargetSubsystemHostInfoPtr
	return r
}

// SetNvmeTargetSubsystemHostInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostGetIterRequestQuery) SetNvmeTargetSubsystemHostInfo(newValue NvmeTargetSubsystemHostInfoType) *NvmeSubsystemHostGetIterRequestQuery {
	o.NvmeTargetSubsystemHostInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *NvmeSubsystemHostGetIterRequest) Query() NvmeSubsystemHostGetIterRequestQuery {
	var r NvmeSubsystemHostGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostGetIterRequest) SetQuery(newValue NvmeSubsystemHostGetIterRequestQuery) *NvmeSubsystemHostGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *NvmeSubsystemHostGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostGetIterRequest) SetTag(newValue string) *NvmeSubsystemHostGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// NvmeSubsystemHostGetIterResponseResultAttributesList is a wrapper
type NvmeSubsystemHostGetIterResponseResultAttributesList struct {
	XMLName                        xml.Name                          `xml:"attributes-list"`
	NvmeTargetSubsystemHostInfoPtr []NvmeTargetSubsystemHostInfoType `xml:"nvme-target-subsystem-host-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemHostGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetSubsystemHostInfo is a 'getter' method
func (o *NvmeSubsystemHostGetIterResponseResultAttributesList) NvmeTargetSubsystemHostInfo() []NvmeTargetSubsystemHostInfoType {
	r := o.NvmeTargetSubsystemHostInfoPtr
	return r
}

// SetNvmeTargetSubsystemHostInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostGetIterResponseResultAttributesList) SetNvmeTargetSubsystemHostInfo(newValue []NvmeTargetSubsystemHostInfoType) *NvmeSubsystemHostGetIterResponseResultAttributesList {
	newSlice := make([]NvmeTargetSubsystemHostInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeTargetSubsystemHostInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *NvmeSubsystemHostGetIterResponseResultAttributesList) values() []NvmeTargetSubsystemHostInfoType {
	r := o.NvmeTargetSubsystemHostInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostGetIterResponseResultAttributesList) setValues(newValue []NvmeTargetSubsystemHostInfoType) *NvmeSubsystemHostGetIterResponseResultAttributesList {
	newSlice := make([]NvmeTargetSubsystemHostInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeTargetSubsystemHostInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *NvmeSubsystemHostGetIterResponseResult) AttributesList() NvmeSubsystemHostGetIterResponseResultAttributesList {
	var r NvmeSubsystemHostGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostGetIterResponseResult) SetAttributesList(newValue NvmeSubsystemHostGetIterResponseResultAttributesList) *NvmeSubsystemHostGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *NvmeSubsystemHostGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostGetIterResponseResult) SetNextTag(newValue string) *NvmeSubsystemHostGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *NvmeSubsystemHostGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemHostGetIterResponseResult) SetNumRecords(newValue int) *NvmeSubsystemHostGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
