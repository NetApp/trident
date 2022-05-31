// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SystemNodeGetIterRequest is a structure to represent a system-node-get-iter Request ZAPI object
type SystemNodeGetIterRequest struct {
	XMLName              xml.Name                                   `xml:"system-node-get-iter"`
	DesiredAttributesPtr *SystemNodeGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                       `xml:"max-records"`
	QueryPtr             *SystemNodeGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                    `xml:"tag"`
}

// SystemNodeGetIterResponse is a structure to represent a system-node-get-iter Response ZAPI object
type SystemNodeGetIterResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          SystemNodeGetIterResponseResult `xml:"results"`
}

// NewSystemNodeGetIterResponse is a factory method for creating new instances of SystemNodeGetIterResponse objects
func NewSystemNodeGetIterResponse() *SystemNodeGetIterResponse {
	return &SystemNodeGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemNodeGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SystemNodeGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SystemNodeGetIterResponseResult is a structure to represent a system-node-get-iter Response Result ZAPI object
type SystemNodeGetIterResponseResult struct {
	XMLName           xml.Name                                       `xml:"results"`
	ResultStatusAttr  string                                         `xml:"status,attr"`
	ResultReasonAttr  string                                         `xml:"reason,attr"`
	ResultErrnoAttr   string                                         `xml:"errno,attr"`
	AttributesListPtr *SystemNodeGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                        `xml:"next-tag"`
	NumRecordsPtr     *int                                           `xml:"num-records"`
}

// NewSystemNodeGetIterRequest is a factory method for creating new instances of SystemNodeGetIterRequest objects
func NewSystemNodeGetIterRequest() *SystemNodeGetIterRequest {
	return &SystemNodeGetIterRequest{}
}

// NewSystemNodeGetIterResponseResult is a factory method for creating new instances of SystemNodeGetIterResponseResult objects
func NewSystemNodeGetIterResponseResult() *SystemNodeGetIterResponseResult {
	return &SystemNodeGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SystemNodeGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SystemNodeGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemNodeGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemNodeGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SystemNodeGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*SystemNodeGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SystemNodeGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*SystemNodeGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "SystemNodeGetIterRequest", NewSystemNodeGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SystemNodeGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SystemNodeGetIterRequest) executeWithIteration(zr *ZapiRunner) (*SystemNodeGetIterResponse, error) {
	combined := NewSystemNodeGetIterResponse()
	combined.Result.SetAttributesList(SystemNodeGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(SystemNodeGetIterResponseResultAttributesList{})
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

// SystemNodeGetIterRequestDesiredAttributes is a wrapper
type SystemNodeGetIterRequestDesiredAttributes struct {
	XMLName            xml.Name             `xml:"desired-attributes"`
	NodeDetailsInfoPtr *NodeDetailsInfoType `xml:"node-details-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemNodeGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// NodeDetailsInfo is a 'getter' method
func (o *SystemNodeGetIterRequestDesiredAttributes) NodeDetailsInfo() NodeDetailsInfoType {
	var r NodeDetailsInfoType
	if o.NodeDetailsInfoPtr == nil {
		return r
	}
	r = *o.NodeDetailsInfoPtr
	return r
}

// SetNodeDetailsInfo is a fluent style 'setter' method that can be chained
func (o *SystemNodeGetIterRequestDesiredAttributes) SetNodeDetailsInfo(newValue NodeDetailsInfoType) *SystemNodeGetIterRequestDesiredAttributes {
	o.NodeDetailsInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *SystemNodeGetIterRequest) DesiredAttributes() SystemNodeGetIterRequestDesiredAttributes {
	var r SystemNodeGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *SystemNodeGetIterRequest) SetDesiredAttributes(newValue SystemNodeGetIterRequestDesiredAttributes) *SystemNodeGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *SystemNodeGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *SystemNodeGetIterRequest) SetMaxRecords(newValue int) *SystemNodeGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// SystemNodeGetIterRequestQuery is a wrapper
type SystemNodeGetIterRequestQuery struct {
	XMLName            xml.Name             `xml:"query"`
	NodeDetailsInfoPtr *NodeDetailsInfoType `xml:"node-details-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemNodeGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// NodeDetailsInfo is a 'getter' method
func (o *SystemNodeGetIterRequestQuery) NodeDetailsInfo() NodeDetailsInfoType {
	var r NodeDetailsInfoType
	if o.NodeDetailsInfoPtr == nil {
		return r
	}
	r = *o.NodeDetailsInfoPtr
	return r
}

// SetNodeDetailsInfo is a fluent style 'setter' method that can be chained
func (o *SystemNodeGetIterRequestQuery) SetNodeDetailsInfo(newValue NodeDetailsInfoType) *SystemNodeGetIterRequestQuery {
	o.NodeDetailsInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *SystemNodeGetIterRequest) Query() SystemNodeGetIterRequestQuery {
	var r SystemNodeGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *SystemNodeGetIterRequest) SetQuery(newValue SystemNodeGetIterRequestQuery) *SystemNodeGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *SystemNodeGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *SystemNodeGetIterRequest) SetTag(newValue string) *SystemNodeGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// SystemNodeGetIterResponseResultAttributesList is a wrapper
type SystemNodeGetIterResponseResultAttributesList struct {
	XMLName            xml.Name              `xml:"attributes-list"`
	NodeDetailsInfoPtr []NodeDetailsInfoType `xml:"node-details-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemNodeGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// NodeDetailsInfo is a 'getter' method
func (o *SystemNodeGetIterResponseResultAttributesList) NodeDetailsInfo() []NodeDetailsInfoType {
	r := o.NodeDetailsInfoPtr
	return r
}

// SetNodeDetailsInfo is a fluent style 'setter' method that can be chained
func (o *SystemNodeGetIterResponseResultAttributesList) SetNodeDetailsInfo(newValue []NodeDetailsInfoType) *SystemNodeGetIterResponseResultAttributesList {
	newSlice := make([]NodeDetailsInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NodeDetailsInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *SystemNodeGetIterResponseResultAttributesList) values() []NodeDetailsInfoType {
	r := o.NodeDetailsInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SystemNodeGetIterResponseResultAttributesList) setValues(newValue []NodeDetailsInfoType) *SystemNodeGetIterResponseResultAttributesList {
	newSlice := make([]NodeDetailsInfoType, len(newValue))
	copy(newSlice, newValue)
	o.NodeDetailsInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *SystemNodeGetIterResponseResult) AttributesList() SystemNodeGetIterResponseResultAttributesList {
	var r SystemNodeGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *SystemNodeGetIterResponseResult) SetAttributesList(newValue SystemNodeGetIterResponseResultAttributesList) *SystemNodeGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *SystemNodeGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *SystemNodeGetIterResponseResult) SetNextTag(newValue string) *SystemNodeGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *SystemNodeGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *SystemNodeGetIterResponseResult) SetNumRecords(newValue int) *SystemNodeGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
