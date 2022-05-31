// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeGetIterRequest is a structure to represent a volume-get-iter Request ZAPI object
type VolumeGetIterRequest struct {
	XMLName              xml.Name                               `xml:"volume-get-iter"`
	DesiredAttributesPtr *VolumeGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                   `xml:"max-records"`
	QueryPtr             *VolumeGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                `xml:"tag"`
}

// VolumeGetIterResponse is a structure to represent a volume-get-iter Response ZAPI object
type VolumeGetIterResponse struct {
	XMLName         xml.Name                    `xml:"netapp"`
	ResponseVersion string                      `xml:"version,attr"`
	ResponseXmlns   string                      `xml:"xmlns,attr"`
	Result          VolumeGetIterResponseResult `xml:"results"`
}

// NewVolumeGetIterResponse is a factory method for creating new instances of VolumeGetIterResponse objects
func NewVolumeGetIterResponse() *VolumeGetIterResponse {
	return &VolumeGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeGetIterResponseResult is a structure to represent a volume-get-iter Response Result ZAPI object
type VolumeGetIterResponseResult struct {
	XMLName           xml.Name                                   `xml:"results"`
	ResultStatusAttr  string                                     `xml:"status,attr"`
	ResultReasonAttr  string                                     `xml:"reason,attr"`
	ResultErrnoAttr   string                                     `xml:"errno,attr"`
	AttributesListPtr *VolumeGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                    `xml:"next-tag"`
	NumRecordsPtr     *int                                       `xml:"num-records"`
}

// NewVolumeGetIterRequest is a factory method for creating new instances of VolumeGetIterRequest objects
func NewVolumeGetIterRequest() *VolumeGetIterRequest {
	return &VolumeGetIterRequest{}
}

// NewVolumeGetIterResponseResult is a factory method for creating new instances of VolumeGetIterResponseResult objects
func NewVolumeGetIterResponseResult() *VolumeGetIterResponseResult {
	return &VolumeGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeGetIterRequest", NewVolumeGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VolumeGetIterRequest) executeWithIteration(zr *ZapiRunner) (*VolumeGetIterResponse, error) {
	combined := NewVolumeGetIterResponse()
	combined.Result.SetAttributesList(VolumeGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(VolumeGetIterResponseResultAttributesList{})
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

// VolumeGetIterRequestDesiredAttributes is a wrapper
type VolumeGetIterRequestDesiredAttributes struct {
	XMLName             xml.Name              `xml:"desired-attributes"`
	VolumeAttributesPtr *VolumeAttributesType `xml:"volume-attributes"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeAttributes is a 'getter' method
func (o *VolumeGetIterRequestDesiredAttributes) VolumeAttributes() VolumeAttributesType {
	var r VolumeAttributesType
	if o.VolumeAttributesPtr == nil {
		return r
	}
	r = *o.VolumeAttributesPtr
	return r
}

// SetVolumeAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeGetIterRequestDesiredAttributes) SetVolumeAttributes(newValue VolumeAttributesType) *VolumeGetIterRequestDesiredAttributes {
	o.VolumeAttributesPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *VolumeGetIterRequest) DesiredAttributes() VolumeGetIterRequestDesiredAttributes {
	var r VolumeGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeGetIterRequest) SetDesiredAttributes(newValue VolumeGetIterRequestDesiredAttributes) *VolumeGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *VolumeGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *VolumeGetIterRequest) SetMaxRecords(newValue int) *VolumeGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// VolumeGetIterRequestQuery is a wrapper
type VolumeGetIterRequestQuery struct {
	XMLName             xml.Name              `xml:"query"`
	VolumeAttributesPtr *VolumeAttributesType `xml:"volume-attributes"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeAttributes is a 'getter' method
func (o *VolumeGetIterRequestQuery) VolumeAttributes() VolumeAttributesType {
	var r VolumeAttributesType
	if o.VolumeAttributesPtr == nil {
		return r
	}
	r = *o.VolumeAttributesPtr
	return r
}

// SetVolumeAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeGetIterRequestQuery) SetVolumeAttributes(newValue VolumeAttributesType) *VolumeGetIterRequestQuery {
	o.VolumeAttributesPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *VolumeGetIterRequest) Query() VolumeGetIterRequestQuery {
	var r VolumeGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *VolumeGetIterRequest) SetQuery(newValue VolumeGetIterRequestQuery) *VolumeGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *VolumeGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *VolumeGetIterRequest) SetTag(newValue string) *VolumeGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// VolumeGetIterResponseResultAttributesList is a wrapper
type VolumeGetIterResponseResultAttributesList struct {
	XMLName             xml.Name               `xml:"attributes-list"`
	VolumeAttributesPtr []VolumeAttributesType `xml:"volume-attributes"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeAttributes is a 'getter' method
func (o *VolumeGetIterResponseResultAttributesList) VolumeAttributes() []VolumeAttributesType {
	r := o.VolumeAttributesPtr
	return r
}

// SetVolumeAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeGetIterResponseResultAttributesList) SetVolumeAttributes(newValue []VolumeAttributesType) *VolumeGetIterResponseResultAttributesList {
	newSlice := make([]VolumeAttributesType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeAttributesPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *VolumeGetIterResponseResultAttributesList) values() []VolumeAttributesType {
	r := o.VolumeAttributesPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *VolumeGetIterResponseResultAttributesList) setValues(newValue []VolumeAttributesType) *VolumeGetIterResponseResultAttributesList {
	newSlice := make([]VolumeAttributesType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeAttributesPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *VolumeGetIterResponseResult) AttributesList() VolumeGetIterResponseResultAttributesList {
	var r VolumeGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *VolumeGetIterResponseResult) SetAttributesList(newValue VolumeGetIterResponseResultAttributesList) *VolumeGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *VolumeGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *VolumeGetIterResponseResult) SetNextTag(newValue string) *VolumeGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *VolumeGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *VolumeGetIterResponseResult) SetNumRecords(newValue int) *VolumeGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
