// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// JobScheduleGetIterRequest is a structure to represent a job-schedule-get-iter Request ZAPI object
type JobScheduleGetIterRequest struct {
	XMLName              xml.Name                                    `xml:"job-schedule-get-iter"`
	DesiredAttributesPtr *JobScheduleGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                        `xml:"max-records"`
	QueryPtr             *JobScheduleGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                     `xml:"tag"`
}

// JobScheduleGetIterResponse is a structure to represent a job-schedule-get-iter Response ZAPI object
type JobScheduleGetIterResponse struct {
	XMLName         xml.Name                         `xml:"netapp"`
	ResponseVersion string                           `xml:"version,attr"`
	ResponseXmlns   string                           `xml:"xmlns,attr"`
	Result          JobScheduleGetIterResponseResult `xml:"results"`
}

// NewJobScheduleGetIterResponse is a factory method for creating new instances of JobScheduleGetIterResponse objects
func NewJobScheduleGetIterResponse() *JobScheduleGetIterResponse {
	return &JobScheduleGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobScheduleGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *JobScheduleGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// JobScheduleGetIterResponseResult is a structure to represent a job-schedule-get-iter Response Result ZAPI object
type JobScheduleGetIterResponseResult struct {
	XMLName           xml.Name                                        `xml:"results"`
	ResultStatusAttr  string                                          `xml:"status,attr"`
	ResultReasonAttr  string                                          `xml:"reason,attr"`
	ResultErrnoAttr   string                                          `xml:"errno,attr"`
	AttributesListPtr *JobScheduleGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                         `xml:"next-tag"`
	NumRecordsPtr     *int                                            `xml:"num-records"`
}

// NewJobScheduleGetIterRequest is a factory method for creating new instances of JobScheduleGetIterRequest objects
func NewJobScheduleGetIterRequest() *JobScheduleGetIterRequest {
	return &JobScheduleGetIterRequest{}
}

// NewJobScheduleGetIterResponseResult is a factory method for creating new instances of JobScheduleGetIterResponseResult objects
func NewJobScheduleGetIterResponseResult() *JobScheduleGetIterResponseResult {
	return &JobScheduleGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *JobScheduleGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *JobScheduleGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobScheduleGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobScheduleGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *JobScheduleGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*JobScheduleGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *JobScheduleGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*JobScheduleGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "JobScheduleGetIterRequest", NewJobScheduleGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*JobScheduleGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *JobScheduleGetIterRequest) executeWithIteration(zr *ZapiRunner) (*JobScheduleGetIterResponse, error) {
	combined := NewJobScheduleGetIterResponse()
	combined.Result.SetAttributesList(JobScheduleGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(JobScheduleGetIterResponseResultAttributesList{})
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

// JobScheduleGetIterRequestDesiredAttributes is a wrapper
type JobScheduleGetIterRequestDesiredAttributes struct {
	XMLName            xml.Name             `xml:"desired-attributes"`
	JobScheduleInfoPtr *JobScheduleInfoType `xml:"job-schedule-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobScheduleGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// JobScheduleInfo is a 'getter' method
func (o *JobScheduleGetIterRequestDesiredAttributes) JobScheduleInfo() JobScheduleInfoType {
	var r JobScheduleInfoType
	if o.JobScheduleInfoPtr == nil {
		return r
	}
	r = *o.JobScheduleInfoPtr
	return r
}

// SetJobScheduleInfo is a fluent style 'setter' method that can be chained
func (o *JobScheduleGetIterRequestDesiredAttributes) SetJobScheduleInfo(newValue JobScheduleInfoType) *JobScheduleGetIterRequestDesiredAttributes {
	o.JobScheduleInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *JobScheduleGetIterRequest) DesiredAttributes() JobScheduleGetIterRequestDesiredAttributes {
	var r JobScheduleGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *JobScheduleGetIterRequest) SetDesiredAttributes(newValue JobScheduleGetIterRequestDesiredAttributes) *JobScheduleGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *JobScheduleGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *JobScheduleGetIterRequest) SetMaxRecords(newValue int) *JobScheduleGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// JobScheduleGetIterRequestQuery is a wrapper
type JobScheduleGetIterRequestQuery struct {
	XMLName            xml.Name             `xml:"query"`
	JobScheduleInfoPtr *JobScheduleInfoType `xml:"job-schedule-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobScheduleGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// JobScheduleInfo is a 'getter' method
func (o *JobScheduleGetIterRequestQuery) JobScheduleInfo() JobScheduleInfoType {
	var r JobScheduleInfoType
	if o.JobScheduleInfoPtr == nil {
		return r
	}
	r = *o.JobScheduleInfoPtr
	return r
}

// SetJobScheduleInfo is a fluent style 'setter' method that can be chained
func (o *JobScheduleGetIterRequestQuery) SetJobScheduleInfo(newValue JobScheduleInfoType) *JobScheduleGetIterRequestQuery {
	o.JobScheduleInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *JobScheduleGetIterRequest) Query() JobScheduleGetIterRequestQuery {
	var r JobScheduleGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *JobScheduleGetIterRequest) SetQuery(newValue JobScheduleGetIterRequestQuery) *JobScheduleGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *JobScheduleGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *JobScheduleGetIterRequest) SetTag(newValue string) *JobScheduleGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// JobScheduleGetIterResponseResultAttributesList is a wrapper
type JobScheduleGetIterResponseResultAttributesList struct {
	XMLName            xml.Name              `xml:"attributes-list"`
	JobScheduleInfoPtr []JobScheduleInfoType `xml:"job-schedule-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o JobScheduleGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// JobScheduleInfo is a 'getter' method
func (o *JobScheduleGetIterResponseResultAttributesList) JobScheduleInfo() []JobScheduleInfoType {
	r := o.JobScheduleInfoPtr
	return r
}

// SetJobScheduleInfo is a fluent style 'setter' method that can be chained
func (o *JobScheduleGetIterResponseResultAttributesList) SetJobScheduleInfo(newValue []JobScheduleInfoType) *JobScheduleGetIterResponseResultAttributesList {
	newSlice := make([]JobScheduleInfoType, len(newValue))
	copy(newSlice, newValue)
	o.JobScheduleInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *JobScheduleGetIterResponseResultAttributesList) values() []JobScheduleInfoType {
	r := o.JobScheduleInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *JobScheduleGetIterResponseResultAttributesList) setValues(newValue []JobScheduleInfoType) *JobScheduleGetIterResponseResultAttributesList {
	newSlice := make([]JobScheduleInfoType, len(newValue))
	copy(newSlice, newValue)
	o.JobScheduleInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *JobScheduleGetIterResponseResult) AttributesList() JobScheduleGetIterResponseResultAttributesList {
	var r JobScheduleGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *JobScheduleGetIterResponseResult) SetAttributesList(newValue JobScheduleGetIterResponseResultAttributesList) *JobScheduleGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *JobScheduleGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *JobScheduleGetIterResponseResult) SetNextTag(newValue string) *JobScheduleGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *JobScheduleGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *JobScheduleGetIterResponseResult) SetNumRecords(newValue int) *JobScheduleGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
