// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiInitiatorGetIterRequest is a structure to represent a iscsi-initiator-get-iter Request ZAPI object
type IscsiInitiatorGetIterRequest struct {
	XMLName              xml.Name                                       `xml:"iscsi-initiator-get-iter"`
	DesiredAttributesPtr *IscsiInitiatorGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                           `xml:"max-records"`
	QueryPtr             *IscsiInitiatorGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                        `xml:"tag"`
}

// IscsiInitiatorGetIterResponse is a structure to represent a iscsi-initiator-get-iter Response ZAPI object
type IscsiInitiatorGetIterResponse struct {
	XMLName         xml.Name                            `xml:"netapp"`
	ResponseVersion string                              `xml:"version,attr"`
	ResponseXmlns   string                              `xml:"xmlns,attr"`
	Result          IscsiInitiatorGetIterResponseResult `xml:"results"`
}

// NewIscsiInitiatorGetIterResponse is a factory method for creating new instances of IscsiInitiatorGetIterResponse objects
func NewIscsiInitiatorGetIterResponse() *IscsiInitiatorGetIterResponse {
	return &IscsiInitiatorGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IscsiInitiatorGetIterResponseResult is a structure to represent a iscsi-initiator-get-iter Response Result ZAPI object
type IscsiInitiatorGetIterResponseResult struct {
	XMLName           xml.Name                                           `xml:"results"`
	ResultStatusAttr  string                                             `xml:"status,attr"`
	ResultReasonAttr  string                                             `xml:"reason,attr"`
	ResultErrnoAttr   string                                             `xml:"errno,attr"`
	AttributesListPtr *IscsiInitiatorGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                            `xml:"next-tag"`
	NumRecordsPtr     *int                                               `xml:"num-records"`
}

// NewIscsiInitiatorGetIterRequest is a factory method for creating new instances of IscsiInitiatorGetIterRequest objects
func NewIscsiInitiatorGetIterRequest() *IscsiInitiatorGetIterRequest {
	return &IscsiInitiatorGetIterRequest{}
}

// NewIscsiInitiatorGetIterResponseResult is a factory method for creating new instances of IscsiInitiatorGetIterResponseResult objects
func NewIscsiInitiatorGetIterResponseResult() *IscsiInitiatorGetIterResponseResult {
	return &IscsiInitiatorGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*IscsiInitiatorGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*IscsiInitiatorGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "IscsiInitiatorGetIterRequest", NewIscsiInitiatorGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IscsiInitiatorGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IscsiInitiatorGetIterRequest) executeWithIteration(zr *ZapiRunner) (*IscsiInitiatorGetIterResponse, error) {
	combined := NewIscsiInitiatorGetIterResponse()
	combined.Result.SetAttributesList(IscsiInitiatorGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(IscsiInitiatorGetIterResponseResultAttributesList{})
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

// IscsiInitiatorGetIterRequestDesiredAttributes is a wrapper
type IscsiInitiatorGetIterRequestDesiredAttributes struct {
	XMLName                        xml.Name                         `xml:"desired-attributes"`
	IscsiInitiatorListEntryInfoPtr *IscsiInitiatorListEntryInfoType `xml:"iscsi-initiator-list-entry-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiInitiatorListEntryInfo is a 'getter' method
func (o *IscsiInitiatorGetIterRequestDesiredAttributes) IscsiInitiatorListEntryInfo() IscsiInitiatorListEntryInfoType {
	r := *o.IscsiInitiatorListEntryInfoPtr
	return r
}

// SetIscsiInitiatorListEntryInfo is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetIterRequestDesiredAttributes) SetIscsiInitiatorListEntryInfo(newValue IscsiInitiatorListEntryInfoType) *IscsiInitiatorGetIterRequestDesiredAttributes {
	o.IscsiInitiatorListEntryInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *IscsiInitiatorGetIterRequest) DesiredAttributes() IscsiInitiatorGetIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetIterRequest) SetDesiredAttributes(newValue IscsiInitiatorGetIterRequestDesiredAttributes) *IscsiInitiatorGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *IscsiInitiatorGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetIterRequest) SetMaxRecords(newValue int) *IscsiInitiatorGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// IscsiInitiatorGetIterRequestQuery is a wrapper
type IscsiInitiatorGetIterRequestQuery struct {
	XMLName                        xml.Name                         `xml:"query"`
	IscsiInitiatorListEntryInfoPtr *IscsiInitiatorListEntryInfoType `xml:"iscsi-initiator-list-entry-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiInitiatorListEntryInfo is a 'getter' method
func (o *IscsiInitiatorGetIterRequestQuery) IscsiInitiatorListEntryInfo() IscsiInitiatorListEntryInfoType {
	r := *o.IscsiInitiatorListEntryInfoPtr
	return r
}

// SetIscsiInitiatorListEntryInfo is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetIterRequestQuery) SetIscsiInitiatorListEntryInfo(newValue IscsiInitiatorListEntryInfoType) *IscsiInitiatorGetIterRequestQuery {
	o.IscsiInitiatorListEntryInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *IscsiInitiatorGetIterRequest) Query() IscsiInitiatorGetIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetIterRequest) SetQuery(newValue IscsiInitiatorGetIterRequestQuery) *IscsiInitiatorGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *IscsiInitiatorGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetIterRequest) SetTag(newValue string) *IscsiInitiatorGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// IscsiInitiatorGetIterResponseResultAttributesList is a wrapper
type IscsiInitiatorGetIterResponseResultAttributesList struct {
	XMLName                        xml.Name                          `xml:"attributes-list"`
	IscsiInitiatorListEntryInfoPtr []IscsiInitiatorListEntryInfoType `xml:"iscsi-initiator-list-entry-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiInitiatorListEntryInfo is a 'getter' method
func (o *IscsiInitiatorGetIterResponseResultAttributesList) IscsiInitiatorListEntryInfo() []IscsiInitiatorListEntryInfoType {
	r := o.IscsiInitiatorListEntryInfoPtr
	return r
}

// SetIscsiInitiatorListEntryInfo is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetIterResponseResultAttributesList) SetIscsiInitiatorListEntryInfo(newValue []IscsiInitiatorListEntryInfoType) *IscsiInitiatorGetIterResponseResultAttributesList {
	newSlice := make([]IscsiInitiatorListEntryInfoType, len(newValue))
	copy(newSlice, newValue)
	o.IscsiInitiatorListEntryInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *IscsiInitiatorGetIterResponseResultAttributesList) values() []IscsiInitiatorListEntryInfoType {
	r := o.IscsiInitiatorListEntryInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetIterResponseResultAttributesList) setValues(newValue []IscsiInitiatorListEntryInfoType) *IscsiInitiatorGetIterResponseResultAttributesList {
	newSlice := make([]IscsiInitiatorListEntryInfoType, len(newValue))
	copy(newSlice, newValue)
	o.IscsiInitiatorListEntryInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *IscsiInitiatorGetIterResponseResult) AttributesList() IscsiInitiatorGetIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetIterResponseResult) SetAttributesList(newValue IscsiInitiatorGetIterResponseResultAttributesList) *IscsiInitiatorGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *IscsiInitiatorGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetIterResponseResult) SetNextTag(newValue string) *IscsiInitiatorGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *IscsiInitiatorGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorGetIterResponseResult) SetNumRecords(newValue int) *IscsiInitiatorGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
