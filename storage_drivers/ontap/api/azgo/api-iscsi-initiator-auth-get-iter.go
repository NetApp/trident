// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiInitiatorAuthGetIterRequest is a structure to represent a iscsi-initiator-auth-get-iter Request ZAPI object
type IscsiInitiatorAuthGetIterRequest struct {
	XMLName              xml.Name                                           `xml:"iscsi-initiator-auth-get-iter"`
	DesiredAttributesPtr *IscsiInitiatorAuthGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                               `xml:"max-records"`
	QueryPtr             *IscsiInitiatorAuthGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                            `xml:"tag"`
}

// IscsiInitiatorAuthGetIterResponse is a structure to represent a iscsi-initiator-auth-get-iter Response ZAPI object
type IscsiInitiatorAuthGetIterResponse struct {
	XMLName         xml.Name                                `xml:"netapp"`
	ResponseVersion string                                  `xml:"version,attr"`
	ResponseXmlns   string                                  `xml:"xmlns,attr"`
	Result          IscsiInitiatorAuthGetIterResponseResult `xml:"results"`
}

// NewIscsiInitiatorAuthGetIterResponse is a factory method for creating new instances of IscsiInitiatorAuthGetIterResponse objects
func NewIscsiInitiatorAuthGetIterResponse() *IscsiInitiatorAuthGetIterResponse {
	return &IscsiInitiatorAuthGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorAuthGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorAuthGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IscsiInitiatorAuthGetIterResponseResult is a structure to represent a iscsi-initiator-auth-get-iter Response Result ZAPI object
type IscsiInitiatorAuthGetIterResponseResult struct {
	XMLName           xml.Name                                               `xml:"results"`
	ResultStatusAttr  string                                                 `xml:"status,attr"`
	ResultReasonAttr  string                                                 `xml:"reason,attr"`
	ResultErrnoAttr   string                                                 `xml:"errno,attr"`
	AttributesListPtr *IscsiInitiatorAuthGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                                `xml:"next-tag"`
	NumRecordsPtr     *int                                                   `xml:"num-records"`
}

// NewIscsiInitiatorAuthGetIterRequest is a factory method for creating new instances of IscsiInitiatorAuthGetIterRequest objects
func NewIscsiInitiatorAuthGetIterRequest() *IscsiInitiatorAuthGetIterRequest {
	return &IscsiInitiatorAuthGetIterRequest{}
}

// NewIscsiInitiatorAuthGetIterResponseResult is a factory method for creating new instances of IscsiInitiatorAuthGetIterResponseResult objects
func NewIscsiInitiatorAuthGetIterResponseResult() *IscsiInitiatorAuthGetIterResponseResult {
	return &IscsiInitiatorAuthGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorAuthGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IscsiInitiatorAuthGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorAuthGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorAuthGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorAuthGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*IscsiInitiatorAuthGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *IscsiInitiatorAuthGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*IscsiInitiatorAuthGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "IscsiInitiatorAuthGetIterRequest", NewIscsiInitiatorAuthGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*IscsiInitiatorAuthGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IscsiInitiatorAuthGetIterRequest) executeWithIteration(zr *ZapiRunner) (*IscsiInitiatorAuthGetIterResponse, error) {
	combined := NewIscsiInitiatorAuthGetIterResponse()
	combined.Result.SetAttributesList(IscsiInitiatorAuthGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(IscsiInitiatorAuthGetIterResponseResultAttributesList{})
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

// IscsiInitiatorAuthGetIterRequestDesiredAttributes is a wrapper
type IscsiInitiatorAuthGetIterRequestDesiredAttributes struct {
	XMLName                   xml.Name                    `xml:"desired-attributes"`
	IscsiSecurityEntryInfoPtr *IscsiSecurityEntryInfoType `xml:"iscsi-security-entry-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorAuthGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiSecurityEntryInfo is a 'getter' method
func (o *IscsiInitiatorAuthGetIterRequestDesiredAttributes) IscsiSecurityEntryInfo() IscsiSecurityEntryInfoType {
	r := *o.IscsiSecurityEntryInfoPtr
	return r
}

// SetIscsiSecurityEntryInfo is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAuthGetIterRequestDesiredAttributes) SetIscsiSecurityEntryInfo(newValue IscsiSecurityEntryInfoType) *IscsiInitiatorAuthGetIterRequestDesiredAttributes {
	o.IscsiSecurityEntryInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *IscsiInitiatorAuthGetIterRequest) DesiredAttributes() IscsiInitiatorAuthGetIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAuthGetIterRequest) SetDesiredAttributes(newValue IscsiInitiatorAuthGetIterRequestDesiredAttributes) *IscsiInitiatorAuthGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *IscsiInitiatorAuthGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAuthGetIterRequest) SetMaxRecords(newValue int) *IscsiInitiatorAuthGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// IscsiInitiatorAuthGetIterRequestQuery is a wrapper
type IscsiInitiatorAuthGetIterRequestQuery struct {
	XMLName                   xml.Name                    `xml:"query"`
	IscsiSecurityEntryInfoPtr *IscsiSecurityEntryInfoType `xml:"iscsi-security-entry-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorAuthGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiSecurityEntryInfo is a 'getter' method
func (o *IscsiInitiatorAuthGetIterRequestQuery) IscsiSecurityEntryInfo() IscsiSecurityEntryInfoType {
	r := *o.IscsiSecurityEntryInfoPtr
	return r
}

// SetIscsiSecurityEntryInfo is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAuthGetIterRequestQuery) SetIscsiSecurityEntryInfo(newValue IscsiSecurityEntryInfoType) *IscsiInitiatorAuthGetIterRequestQuery {
	o.IscsiSecurityEntryInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *IscsiInitiatorAuthGetIterRequest) Query() IscsiInitiatorAuthGetIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAuthGetIterRequest) SetQuery(newValue IscsiInitiatorAuthGetIterRequestQuery) *IscsiInitiatorAuthGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *IscsiInitiatorAuthGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAuthGetIterRequest) SetTag(newValue string) *IscsiInitiatorAuthGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// IscsiInitiatorAuthGetIterResponseResultAttributesList is a wrapper
type IscsiInitiatorAuthGetIterResponseResultAttributesList struct {
	XMLName                   xml.Name                     `xml:"attributes-list"`
	IscsiSecurityEntryInfoPtr []IscsiSecurityEntryInfoType `xml:"iscsi-security-entry-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiInitiatorAuthGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// IscsiSecurityEntryInfo is a 'getter' method
func (o *IscsiInitiatorAuthGetIterResponseResultAttributesList) IscsiSecurityEntryInfo() []IscsiSecurityEntryInfoType {
	r := o.IscsiSecurityEntryInfoPtr
	return r
}

// SetIscsiSecurityEntryInfo is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAuthGetIterResponseResultAttributesList) SetIscsiSecurityEntryInfo(newValue []IscsiSecurityEntryInfoType) *IscsiInitiatorAuthGetIterResponseResultAttributesList {
	newSlice := make([]IscsiSecurityEntryInfoType, len(newValue))
	copy(newSlice, newValue)
	o.IscsiSecurityEntryInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *IscsiInitiatorAuthGetIterResponseResultAttributesList) values() []IscsiSecurityEntryInfoType {
	r := o.IscsiSecurityEntryInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAuthGetIterResponseResultAttributesList) setValues(newValue []IscsiSecurityEntryInfoType) *IscsiInitiatorAuthGetIterResponseResultAttributesList {
	newSlice := make([]IscsiSecurityEntryInfoType, len(newValue))
	copy(newSlice, newValue)
	o.IscsiSecurityEntryInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *IscsiInitiatorAuthGetIterResponseResult) AttributesList() IscsiInitiatorAuthGetIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAuthGetIterResponseResult) SetAttributesList(newValue IscsiInitiatorAuthGetIterResponseResultAttributesList) *IscsiInitiatorAuthGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *IscsiInitiatorAuthGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAuthGetIterResponseResult) SetNextTag(newValue string) *IscsiInitiatorAuthGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *IscsiInitiatorAuthGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *IscsiInitiatorAuthGetIterResponseResult) SetNumRecords(newValue int) *IscsiInitiatorAuthGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
