// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorGetDestinationIterRequest is a structure to represent a snapmirror-get-destination-iter Request ZAPI object
type SnapmirrorGetDestinationIterRequest struct {
	XMLName              xml.Name                                              `xml:"snapmirror-get-destination-iter"`
	DesiredAttributesPtr *SnapmirrorGetDestinationIterRequestDesiredAttributes `xml:"desired-attributes"`
	ExpandPtr            *bool                                                 `xml:"expand"`
	MaxRecordsPtr        *int                                                  `xml:"max-records"`
	QueryPtr             *SnapmirrorGetDestinationIterRequestQuery             `xml:"query"`
	TagPtr               *string                                               `xml:"tag"`
}

// SnapmirrorGetDestinationIterResponse is a structure to represent a snapmirror-get-destination-iter Response ZAPI object
type SnapmirrorGetDestinationIterResponse struct {
	XMLName         xml.Name                                   `xml:"netapp"`
	ResponseVersion string                                     `xml:"version,attr"`
	ResponseXmlns   string                                     `xml:"xmlns,attr"`
	Result          SnapmirrorGetDestinationIterResponseResult `xml:"results"`
}

// NewSnapmirrorGetDestinationIterResponse is a factory method for creating new instances of SnapmirrorGetDestinationIterResponse objects
func NewSnapmirrorGetDestinationIterResponse() *SnapmirrorGetDestinationIterResponse {
	return &SnapmirrorGetDestinationIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetDestinationIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorGetDestinationIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorGetDestinationIterResponseResult is a structure to represent a snapmirror-get-destination-iter Response Result ZAPI object
type SnapmirrorGetDestinationIterResponseResult struct {
	XMLName           xml.Name                                                  `xml:"results"`
	ResultStatusAttr  string                                                    `xml:"status,attr"`
	ResultReasonAttr  string                                                    `xml:"reason,attr"`
	ResultErrnoAttr   string                                                    `xml:"errno,attr"`
	AttributesListPtr *SnapmirrorGetDestinationIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                                   `xml:"next-tag"`
	NumRecordsPtr     *int                                                      `xml:"num-records"`
}

// NewSnapmirrorGetDestinationIterRequest is a factory method for creating new instances of SnapmirrorGetDestinationIterRequest objects
func NewSnapmirrorGetDestinationIterRequest() *SnapmirrorGetDestinationIterRequest {
	return &SnapmirrorGetDestinationIterRequest{}
}

// NewSnapmirrorGetDestinationIterResponseResult is a factory method for creating new instances of SnapmirrorGetDestinationIterResponseResult objects
func NewSnapmirrorGetDestinationIterResponseResult() *SnapmirrorGetDestinationIterResponseResult {
	return &SnapmirrorGetDestinationIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorGetDestinationIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorGetDestinationIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetDestinationIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetDestinationIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorGetDestinationIterRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorGetDestinationIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorGetDestinationIterRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorGetDestinationIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorGetDestinationIterRequest", NewSnapmirrorGetDestinationIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorGetDestinationIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SnapmirrorGetDestinationIterRequest) executeWithIteration(zr *ZapiRunner) (*SnapmirrorGetDestinationIterResponse, error) {
	combined := NewSnapmirrorGetDestinationIterResponse()
	combined.Result.SetAttributesList(SnapmirrorGetDestinationIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(SnapmirrorGetDestinationIterResponseResultAttributesList{})
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

// SnapmirrorGetDestinationIterRequestDesiredAttributes is a wrapper
type SnapmirrorGetDestinationIterRequestDesiredAttributes struct {
	XMLName                      xml.Name                       `xml:"desired-attributes"`
	SnapmirrorDestinationInfoPtr *SnapmirrorDestinationInfoType `xml:"snapmirror-destination-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetDestinationIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorDestinationInfo is a 'getter' method
func (o *SnapmirrorGetDestinationIterRequestDesiredAttributes) SnapmirrorDestinationInfo() SnapmirrorDestinationInfoType {
	r := *o.SnapmirrorDestinationInfoPtr
	return r
}

// SetSnapmirrorDestinationInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterRequestDesiredAttributes) SetSnapmirrorDestinationInfo(newValue SnapmirrorDestinationInfoType) *SnapmirrorGetDestinationIterRequestDesiredAttributes {
	o.SnapmirrorDestinationInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *SnapmirrorGetDestinationIterRequest) DesiredAttributes() SnapmirrorGetDestinationIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterRequest) SetDesiredAttributes(newValue SnapmirrorGetDestinationIterRequestDesiredAttributes) *SnapmirrorGetDestinationIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// Expand is a 'getter' method
func (o *SnapmirrorGetDestinationIterRequest) Expand() bool {
	r := *o.ExpandPtr
	return r
}

// SetExpand is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterRequest) SetExpand(newValue bool) *SnapmirrorGetDestinationIterRequest {
	o.ExpandPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *SnapmirrorGetDestinationIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterRequest) SetMaxRecords(newValue int) *SnapmirrorGetDestinationIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// SnapmirrorGetDestinationIterRequestQuery is a wrapper
type SnapmirrorGetDestinationIterRequestQuery struct {
	XMLName                      xml.Name                       `xml:"query"`
	SnapmirrorDestinationInfoPtr *SnapmirrorDestinationInfoType `xml:"snapmirror-destination-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetDestinationIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorDestinationInfo is a 'getter' method
func (o *SnapmirrorGetDestinationIterRequestQuery) SnapmirrorDestinationInfo() SnapmirrorDestinationInfoType {
	r := *o.SnapmirrorDestinationInfoPtr
	return r
}

// SetSnapmirrorDestinationInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterRequestQuery) SetSnapmirrorDestinationInfo(newValue SnapmirrorDestinationInfoType) *SnapmirrorGetDestinationIterRequestQuery {
	o.SnapmirrorDestinationInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *SnapmirrorGetDestinationIterRequest) Query() SnapmirrorGetDestinationIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterRequest) SetQuery(newValue SnapmirrorGetDestinationIterRequestQuery) *SnapmirrorGetDestinationIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *SnapmirrorGetDestinationIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterRequest) SetTag(newValue string) *SnapmirrorGetDestinationIterRequest {
	o.TagPtr = &newValue
	return o
}

// SnapmirrorGetDestinationIterResponseResultAttributesList is a wrapper
type SnapmirrorGetDestinationIterResponseResultAttributesList struct {
	XMLName                      xml.Name                        `xml:"attributes-list"`
	SnapmirrorDestinationInfoPtr []SnapmirrorDestinationInfoType `xml:"snapmirror-destination-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetDestinationIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorDestinationInfo is a 'getter' method
func (o *SnapmirrorGetDestinationIterResponseResultAttributesList) SnapmirrorDestinationInfo() []SnapmirrorDestinationInfoType {
	r := o.SnapmirrorDestinationInfoPtr
	return r
}

// SetSnapmirrorDestinationInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterResponseResultAttributesList) SetSnapmirrorDestinationInfo(newValue []SnapmirrorDestinationInfoType) *SnapmirrorGetDestinationIterResponseResultAttributesList {
	newSlice := make([]SnapmirrorDestinationInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapmirrorDestinationInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *SnapmirrorGetDestinationIterResponseResultAttributesList) values() []SnapmirrorDestinationInfoType {
	r := o.SnapmirrorDestinationInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterResponseResultAttributesList) setValues(newValue []SnapmirrorDestinationInfoType) *SnapmirrorGetDestinationIterResponseResultAttributesList {
	newSlice := make([]SnapmirrorDestinationInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapmirrorDestinationInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *SnapmirrorGetDestinationIterResponseResult) AttributesList() SnapmirrorGetDestinationIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterResponseResult) SetAttributesList(newValue SnapmirrorGetDestinationIterResponseResultAttributesList) *SnapmirrorGetDestinationIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *SnapmirrorGetDestinationIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterResponseResult) SetNextTag(newValue string) *SnapmirrorGetDestinationIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *SnapmirrorGetDestinationIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetDestinationIterResponseResult) SetNumRecords(newValue int) *SnapmirrorGetDestinationIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
