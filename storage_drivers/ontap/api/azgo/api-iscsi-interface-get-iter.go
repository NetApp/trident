// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiInterfaceGetIterRequest is a structure to represent a iscsi-interface-get-iter Request ZAPI object
type IscsiInterfaceGetIterRequest struct {
	XMLName              xml.Name                                       `xml:"iscsi-interface-get-iter"`
	DesiredAttributesPtr *IscsiInterfaceGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                           `xml:"max-records"`
	QueryPtr             *IscsiInterfaceGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                        `xml:"tag"`
}

// IscsiInterfaceGetIterResponse is a structure to represent a iscsi-interface-get-iter Response ZAPI object
type IscsiInterfaceGetIterResponse struct {
	XMLName         xml.Name                            `xml:"netapp"`
	ResponseVersion string                              `xml:"version,attr"`
	ResponseXmlns   string                              `xml:"xmlns,attr"`
	Result          IscsiInterfaceGetIterResponseResult `xml:"results"`
}

// NewIscsiInterfaceGetIterResponse is a factory method for creating new instances of IscsiInterfaceGetIterResponse objects
func NewIscsiInterfaceGetIterResponse() *IscsiInterfaceGetIterResponse {
	return &IscsiInterfaceGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInterfaceGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IscsiInterfaceGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IscsiInterfaceGetIterResponseResult is a structure to represent a iscsi-interface-get-iter Response Result ZAPI object
type IscsiInterfaceGetIterResponseResult struct {
	XMLName           xml.Name                                           `xml:"results"`
	ResultStatusAttr  string                                             `xml:"status,attr"`
	ResultReasonAttr  string                                             `xml:"reason,attr"`
	ResultErrnoAttr   string                                             `xml:"errno,attr"`
	AttributesListPtr *IscsiInterfaceGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                            `xml:"next-tag"`
	NumRecordsPtr     *int                                               `xml:"num-records"`
}

// NewIscsiInterfaceGetIterRequest is a factory method for creating new instances of IscsiInterfaceGetIterRequest objects
func NewIscsiInterfaceGetIterRequest() *IscsiInterfaceGetIterRequest {
	return &IscsiInterfaceGetIterRequest{}
}

// NewIscsiInterfaceGetIterResponseResult is a factory method for creating new instances of IscsiInterfaceGetIterResponseResult objects
func NewIscsiInterfaceGetIterResponseResult() *IscsiInterfaceGetIterResponseResult {
	return &IscsiInterfaceGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiInterfaceGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IscsiInterfaceGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInterfaceGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInterfaceGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInterfaceGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*IscsiInterfaceGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInterfaceGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*IscsiInterfaceGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "IscsiInterfaceGetIterRequest", NewIscsiInterfaceGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IscsiInterfaceGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IscsiInterfaceGetIterRequest) executeWithIteration(zr *ZapiRunner) (*IscsiInterfaceGetIterResponse, error) {
	combined := NewIscsiInterfaceGetIterResponse()
	combined.Result.SetAttributesList(IscsiInterfaceGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(IscsiInterfaceGetIterResponseResultAttributesList{})
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

// IscsiInterfaceGetIterRequestDesiredAttributes is a wrapper
type IscsiInterfaceGetIterRequestDesiredAttributes struct {
	XMLName                        xml.Name                         `xml:"desired-attributes"`
	IscsiInterfaceListEntryInfoPtr *IscsiInterfaceListEntryInfoType `xml:"iscsi-interface-list-entry-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInterfaceGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiInterfaceListEntryInfo is a 'getter' method
func (o *IscsiInterfaceGetIterRequestDesiredAttributes) IscsiInterfaceListEntryInfo() IscsiInterfaceListEntryInfoType {
	r := *o.IscsiInterfaceListEntryInfoPtr
	return r
}

// SetIscsiInterfaceListEntryInfo is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceGetIterRequestDesiredAttributes) SetIscsiInterfaceListEntryInfo(newValue IscsiInterfaceListEntryInfoType) *IscsiInterfaceGetIterRequestDesiredAttributes {
	o.IscsiInterfaceListEntryInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *IscsiInterfaceGetIterRequest) DesiredAttributes() IscsiInterfaceGetIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceGetIterRequest) SetDesiredAttributes(newValue IscsiInterfaceGetIterRequestDesiredAttributes) *IscsiInterfaceGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *IscsiInterfaceGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceGetIterRequest) SetMaxRecords(newValue int) *IscsiInterfaceGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// IscsiInterfaceGetIterRequestQuery is a wrapper
type IscsiInterfaceGetIterRequestQuery struct {
	XMLName                        xml.Name                         `xml:"query"`
	IscsiInterfaceListEntryInfoPtr *IscsiInterfaceListEntryInfoType `xml:"iscsi-interface-list-entry-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInterfaceGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiInterfaceListEntryInfo is a 'getter' method
func (o *IscsiInterfaceGetIterRequestQuery) IscsiInterfaceListEntryInfo() IscsiInterfaceListEntryInfoType {
	r := *o.IscsiInterfaceListEntryInfoPtr
	return r
}

// SetIscsiInterfaceListEntryInfo is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceGetIterRequestQuery) SetIscsiInterfaceListEntryInfo(newValue IscsiInterfaceListEntryInfoType) *IscsiInterfaceGetIterRequestQuery {
	o.IscsiInterfaceListEntryInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *IscsiInterfaceGetIterRequest) Query() IscsiInterfaceGetIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceGetIterRequest) SetQuery(newValue IscsiInterfaceGetIterRequestQuery) *IscsiInterfaceGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *IscsiInterfaceGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceGetIterRequest) SetTag(newValue string) *IscsiInterfaceGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// IscsiInterfaceGetIterResponseResultAttributesList is a wrapper
type IscsiInterfaceGetIterResponseResultAttributesList struct {
	XMLName                        xml.Name                          `xml:"attributes-list"`
	IscsiInterfaceListEntryInfoPtr []IscsiInterfaceListEntryInfoType `xml:"iscsi-interface-list-entry-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInterfaceGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiInterfaceListEntryInfo is a 'getter' method
func (o *IscsiInterfaceGetIterResponseResultAttributesList) IscsiInterfaceListEntryInfo() []IscsiInterfaceListEntryInfoType {
	r := o.IscsiInterfaceListEntryInfoPtr
	return r
}

// SetIscsiInterfaceListEntryInfo is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceGetIterResponseResultAttributesList) SetIscsiInterfaceListEntryInfo(newValue []IscsiInterfaceListEntryInfoType) *IscsiInterfaceGetIterResponseResultAttributesList {
	newSlice := make([]IscsiInterfaceListEntryInfoType, len(newValue))
	copy(newSlice, newValue)
	o.IscsiInterfaceListEntryInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *IscsiInterfaceGetIterResponseResultAttributesList) values() []IscsiInterfaceListEntryInfoType {
	r := o.IscsiInterfaceListEntryInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceGetIterResponseResultAttributesList) setValues(newValue []IscsiInterfaceListEntryInfoType) *IscsiInterfaceGetIterResponseResultAttributesList {
	newSlice := make([]IscsiInterfaceListEntryInfoType, len(newValue))
	copy(newSlice, newValue)
	o.IscsiInterfaceListEntryInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *IscsiInterfaceGetIterResponseResult) AttributesList() IscsiInterfaceGetIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceGetIterResponseResult) SetAttributesList(newValue IscsiInterfaceGetIterResponseResultAttributesList) *IscsiInterfaceGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *IscsiInterfaceGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceGetIterResponseResult) SetNextTag(newValue string) *IscsiInterfaceGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *IscsiInterfaceGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *IscsiInterfaceGetIterResponseResult) SetNumRecords(newValue int) *IscsiInterfaceGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
