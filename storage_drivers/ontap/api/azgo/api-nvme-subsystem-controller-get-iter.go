// Code generated automatically. DO NOT EDIT.
// Copyright 2023 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NvmeSubsystemControllerGetIterRequest is a structure to represent a nvme-subsystem-controller-get-iter Request ZAPI object
type NvmeSubsystemControllerGetIterRequest struct {
	XMLName              xml.Name                                                `xml:"nvme-subsystem-controller-get-iter"`
	DesiredAttributesPtr *NvmeSubsystemControllerGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                                    `xml:"max-records"`
	QueryPtr             *NvmeSubsystemControllerGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                                 `xml:"tag"`
}

// NvmeSubsystemControllerGetIterResponse is a structure to represent a nvme-subsystem-controller-get-iter Response ZAPI object
type NvmeSubsystemControllerGetIterResponse struct {
	XMLName         xml.Name                                     `xml:"netapp"`
	ResponseVersion string                                       `xml:"version,attr"`
	ResponseXmlns   string                                       `xml:"xmlns,attr"`
	Result          NvmeSubsystemControllerGetIterResponseResult `xml:"results"`
}

// NewNvmeSubsystemControllerGetIterResponse is a factory method for creating new instances of NvmeSubsystemControllerGetIterResponse objects
func NewNvmeSubsystemControllerGetIterResponse() *NvmeSubsystemControllerGetIterResponse {
	return &NvmeSubsystemControllerGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemControllerGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemControllerGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// NvmeSubsystemControllerGetIterResponseResult is a structure to represent a nvme-subsystem-controller-get-iter Response Result ZAPI object
type NvmeSubsystemControllerGetIterResponseResult struct {
	XMLName           xml.Name                                                    `xml:"results"`
	ResultStatusAttr  string                                                      `xml:"status,attr"`
	ResultReasonAttr  string                                                      `xml:"reason,attr"`
	ResultErrnoAttr   string                                                      `xml:"errno,attr"`
	AttributesListPtr *NvmeSubsystemControllerGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                                     `xml:"next-tag"`
	NumRecordsPtr     *int                                                        `xml:"num-records"`
}

// NewNvmeSubsystemControllerGetIterRequest is a factory method for creating new instances of NvmeSubsystemControllerGetIterRequest objects
func NewNvmeSubsystemControllerGetIterRequest() *NvmeSubsystemControllerGetIterRequest {
	return &NvmeSubsystemControllerGetIterRequest{}
}

// NewNvmeSubsystemControllerGetIterResponseResult is a factory method for creating new instances of NvmeSubsystemControllerGetIterResponseResult objects
func NewNvmeSubsystemControllerGetIterResponseResult() *NvmeSubsystemControllerGetIterResponseResult {
	return &NvmeSubsystemControllerGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemControllerGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *NvmeSubsystemControllerGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemControllerGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemControllerGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemControllerGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*NvmeSubsystemControllerGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *NvmeSubsystemControllerGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*NvmeSubsystemControllerGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "NvmeSubsystemControllerGetIterRequest", NewNvmeSubsystemControllerGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*NvmeSubsystemControllerGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *NvmeSubsystemControllerGetIterRequest) executeWithIteration(zr *ZapiRunner) (*NvmeSubsystemControllerGetIterResponse, error) {
	combined := NewNvmeSubsystemControllerGetIterResponse()
	combined.Result.SetAttributesList(NvmeSubsystemControllerGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(NvmeSubsystemControllerGetIterResponseResultAttributesList{})
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

// NvmeSubsystemControllerGetIterRequestDesiredAttributes is a wrapper
type NvmeSubsystemControllerGetIterRequestDesiredAttributes struct {
	XMLName                              xml.Name                               `xml:"desired-attributes"`
	NvmeTargetSubsystemControllerInfoPtr *NvmeTargetSubsystemControllerInfoType `xml:"nvme-target-subsystem-controller-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemControllerGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetSubsystemControllerInfo is a 'getter' method
func (o *NvmeSubsystemControllerGetIterRequestDesiredAttributes) NvmeTargetSubsystemControllerInfo() NvmeTargetSubsystemControllerInfoType {
	var r NvmeTargetSubsystemControllerInfoType
	if o.NvmeTargetSubsystemControllerInfoPtr == nil {
		return r
	}
	r = *o.NvmeTargetSubsystemControllerInfoPtr
	return r
}

// SetNvmeTargetSubsystemControllerInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerGetIterRequestDesiredAttributes) SetNvmeTargetSubsystemControllerInfo(newValue NvmeTargetSubsystemControllerInfoType) *NvmeSubsystemControllerGetIterRequestDesiredAttributes {
	o.NvmeTargetSubsystemControllerInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *NvmeSubsystemControllerGetIterRequest) DesiredAttributes() NvmeSubsystemControllerGetIterRequestDesiredAttributes {
	var r NvmeSubsystemControllerGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerGetIterRequest) SetDesiredAttributes(newValue NvmeSubsystemControllerGetIterRequestDesiredAttributes) *NvmeSubsystemControllerGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *NvmeSubsystemControllerGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerGetIterRequest) SetMaxRecords(newValue int) *NvmeSubsystemControllerGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// NvmeSubsystemControllerGetIterRequestQuery is a wrapper
type NvmeSubsystemControllerGetIterRequestQuery struct {
	XMLName                              xml.Name                               `xml:"query"`
	NvmeTargetSubsystemControllerInfoPtr *NvmeTargetSubsystemControllerInfoType `xml:"nvme-target-subsystem-controller-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemControllerGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetSubsystemControllerInfo is a 'getter' method
func (o *NvmeSubsystemControllerGetIterRequestQuery) NvmeTargetSubsystemControllerInfo() NvmeTargetSubsystemControllerInfoType {
	var r NvmeTargetSubsystemControllerInfoType
	if o.NvmeTargetSubsystemControllerInfoPtr == nil {
		return r
	}
	r = *o.NvmeTargetSubsystemControllerInfoPtr
	return r
}

// SetNvmeTargetSubsystemControllerInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerGetIterRequestQuery) SetNvmeTargetSubsystemControllerInfo(newValue NvmeTargetSubsystemControllerInfoType) *NvmeSubsystemControllerGetIterRequestQuery {
	o.NvmeTargetSubsystemControllerInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *NvmeSubsystemControllerGetIterRequest) Query() NvmeSubsystemControllerGetIterRequestQuery {
	var r NvmeSubsystemControllerGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerGetIterRequest) SetQuery(newValue NvmeSubsystemControllerGetIterRequestQuery) *NvmeSubsystemControllerGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *NvmeSubsystemControllerGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerGetIterRequest) SetTag(newValue string) *NvmeSubsystemControllerGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// NvmeSubsystemControllerGetIterResponseResultAttributesList is a wrapper
type NvmeSubsystemControllerGetIterResponseResultAttributesList struct {
	XMLName                              xml.Name                                `xml:"attributes-list"`
	NvmeTargetSubsystemControllerInfoPtr []NvmeTargetSubsystemControllerInfoType `xml:"nvme-target-subsystem-controller-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NvmeSubsystemControllerGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// NvmeTargetSubsystemControllerInfo is a 'getter' method
func (o *NvmeSubsystemControllerGetIterResponseResultAttributesList) NvmeTargetSubsystemControllerInfo() []NvmeTargetSubsystemControllerInfoType {
	r := o.NvmeTargetSubsystemControllerInfoPtr
	return r
}

// SetNvmeTargetSubsystemControllerInfo is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerGetIterResponseResultAttributesList) SetNvmeTargetSubsystemControllerInfo(newValue []NvmeTargetSubsystemControllerInfoType) *NvmeSubsystemControllerGetIterResponseResultAttributesList {
	newSlice := make([]NvmeTargetSubsystemControllerInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeTargetSubsystemControllerInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *NvmeSubsystemControllerGetIterResponseResultAttributesList) values() []NvmeTargetSubsystemControllerInfoType {
	r := o.NvmeTargetSubsystemControllerInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerGetIterResponseResultAttributesList) setValues(newValue []NvmeTargetSubsystemControllerInfoType) *NvmeSubsystemControllerGetIterResponseResultAttributesList {
	newSlice := make([]NvmeTargetSubsystemControllerInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NvmeTargetSubsystemControllerInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *NvmeSubsystemControllerGetIterResponseResult) AttributesList() NvmeSubsystemControllerGetIterResponseResultAttributesList {
	var r NvmeSubsystemControllerGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerGetIterResponseResult) SetAttributesList(newValue NvmeSubsystemControllerGetIterResponseResultAttributesList) *NvmeSubsystemControllerGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *NvmeSubsystemControllerGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerGetIterResponseResult) SetNextTag(newValue string) *NvmeSubsystemControllerGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *NvmeSubsystemControllerGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *NvmeSubsystemControllerGetIterResponseResult) SetNumRecords(newValue int) *NvmeSubsystemControllerGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
