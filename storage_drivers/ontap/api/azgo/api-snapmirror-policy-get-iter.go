package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SnapmirrorPolicyGetIterRequest is a structure to represent a snapmirror-policy-get-iter Request ZAPI object
type SnapmirrorPolicyGetIterRequest struct {
	XMLName              xml.Name                                         `xml:"snapmirror-policy-get-iter"`
	DesiredAttributesPtr *SnapmirrorPolicyGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                             `xml:"max-records"`
	QueryPtr             *SnapmirrorPolicyGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                          `xml:"tag"`
}

// SnapmirrorPolicyGetIterResponse is a structure to represent a snapmirror-policy-get-iter Response ZAPI object
type SnapmirrorPolicyGetIterResponse struct {
	XMLName         xml.Name                              `xml:"netapp"`
	ResponseVersion string                                `xml:"version,attr"`
	ResponseXmlns   string                                `xml:"xmlns,attr"`
	Result          SnapmirrorPolicyGetIterResponseResult `xml:"results"`
}

// NewSnapmirrorPolicyGetIterResponse is a factory method for creating new instances of SnapmirrorPolicyGetIterResponse objects
func NewSnapmirrorPolicyGetIterResponse() *SnapmirrorPolicyGetIterResponse {
	return &SnapmirrorPolicyGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorPolicyGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorPolicyGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorPolicyGetIterResponseResult is a structure to represent a snapmirror-policy-get-iter Response Result ZAPI object
type SnapmirrorPolicyGetIterResponseResult struct {
	XMLName           xml.Name                                             `xml:"results"`
	ResultStatusAttr  string                                               `xml:"status,attr"`
	ResultReasonAttr  string                                               `xml:"reason,attr"`
	ResultErrnoAttr   string                                               `xml:"errno,attr"`
	AttributesListPtr *SnapmirrorPolicyGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                              `xml:"next-tag"`
	NumRecordsPtr     *int                                                 `xml:"num-records"`
}

// NewSnapmirrorPolicyGetIterRequest is a factory method for creating new instances of SnapmirrorPolicyGetIterRequest objects
func NewSnapmirrorPolicyGetIterRequest() *SnapmirrorPolicyGetIterRequest {
	return &SnapmirrorPolicyGetIterRequest{}
}

// NewSnapmirrorPolicyGetIterResponseResult is a factory method for creating new instances of SnapmirrorPolicyGetIterResponseResult objects
func NewSnapmirrorPolicyGetIterResponseResult() *SnapmirrorPolicyGetIterResponseResult {
	return &SnapmirrorPolicyGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorPolicyGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorPolicyGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorPolicyGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorPolicyGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorPolicyGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorPolicyGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorPolicyGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorPolicyGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorPolicyGetIterRequest", NewSnapmirrorPolicyGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorPolicyGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SnapmirrorPolicyGetIterRequest) executeWithIteration(zr *ZapiRunner) (*SnapmirrorPolicyGetIterResponse, error) {
	combined := NewSnapmirrorPolicyGetIterResponse()
	combined.Result.SetAttributesList(SnapmirrorPolicyGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(SnapmirrorPolicyGetIterResponseResultAttributesList{})
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

// SnapmirrorPolicyGetIterRequestDesiredAttributes is a wrapper
type SnapmirrorPolicyGetIterRequestDesiredAttributes struct {
	XMLName                 xml.Name                  `xml:"desired-attributes"`
	SnapmirrorPolicyInfoPtr *SnapmirrorPolicyInfoType `xml:"snapmirror-policy-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorPolicyGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorPolicyInfo is a 'getter' method
func (o *SnapmirrorPolicyGetIterRequestDesiredAttributes) SnapmirrorPolicyInfo() SnapmirrorPolicyInfoType {

	var r SnapmirrorPolicyInfoType
	if o.SnapmirrorPolicyInfoPtr == nil {
		return r
	}
	r = *o.SnapmirrorPolicyInfoPtr

	return r
}

// SetSnapmirrorPolicyInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyGetIterRequestDesiredAttributes) SetSnapmirrorPolicyInfo(newValue SnapmirrorPolicyInfoType) *SnapmirrorPolicyGetIterRequestDesiredAttributes {
	o.SnapmirrorPolicyInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *SnapmirrorPolicyGetIterRequest) DesiredAttributes() SnapmirrorPolicyGetIterRequestDesiredAttributes {

	var r SnapmirrorPolicyGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr

	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyGetIterRequest) SetDesiredAttributes(newValue SnapmirrorPolicyGetIterRequestDesiredAttributes) *SnapmirrorPolicyGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *SnapmirrorPolicyGetIterRequest) MaxRecords() int {

	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr

	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyGetIterRequest) SetMaxRecords(newValue int) *SnapmirrorPolicyGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// SnapmirrorPolicyGetIterRequestQuery is a wrapper
type SnapmirrorPolicyGetIterRequestQuery struct {
	XMLName                 xml.Name                  `xml:"query"`
	SnapmirrorPolicyInfoPtr *SnapmirrorPolicyInfoType `xml:"snapmirror-policy-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorPolicyGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorPolicyInfo is a 'getter' method
func (o *SnapmirrorPolicyGetIterRequestQuery) SnapmirrorPolicyInfo() SnapmirrorPolicyInfoType {

	var r SnapmirrorPolicyInfoType
	if o.SnapmirrorPolicyInfoPtr == nil {
		return r
	}
	r = *o.SnapmirrorPolicyInfoPtr

	return r
}

// SetSnapmirrorPolicyInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyGetIterRequestQuery) SetSnapmirrorPolicyInfo(newValue SnapmirrorPolicyInfoType) *SnapmirrorPolicyGetIterRequestQuery {
	o.SnapmirrorPolicyInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *SnapmirrorPolicyGetIterRequest) Query() SnapmirrorPolicyGetIterRequestQuery {

	var r SnapmirrorPolicyGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr

	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyGetIterRequest) SetQuery(newValue SnapmirrorPolicyGetIterRequestQuery) *SnapmirrorPolicyGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *SnapmirrorPolicyGetIterRequest) Tag() string {

	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr

	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyGetIterRequest) SetTag(newValue string) *SnapmirrorPolicyGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// SnapmirrorPolicyGetIterResponseResultAttributesList is a wrapper
type SnapmirrorPolicyGetIterResponseResultAttributesList struct {
	XMLName                 xml.Name                   `xml:"attributes-list"`
	SnapmirrorPolicyInfoPtr []SnapmirrorPolicyInfoType `xml:"snapmirror-policy-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorPolicyGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorPolicyInfo is a 'getter' method
func (o *SnapmirrorPolicyGetIterResponseResultAttributesList) SnapmirrorPolicyInfo() []SnapmirrorPolicyInfoType {
	r := o.SnapmirrorPolicyInfoPtr
	return r
}

// SetSnapmirrorPolicyInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyGetIterResponseResultAttributesList) SetSnapmirrorPolicyInfo(newValue []SnapmirrorPolicyInfoType) *SnapmirrorPolicyGetIterResponseResultAttributesList {
	newSlice := make([]SnapmirrorPolicyInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapmirrorPolicyInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *SnapmirrorPolicyGetIterResponseResultAttributesList) values() []SnapmirrorPolicyInfoType {
	r := o.SnapmirrorPolicyInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyGetIterResponseResultAttributesList) setValues(newValue []SnapmirrorPolicyInfoType) *SnapmirrorPolicyGetIterResponseResultAttributesList {
	newSlice := make([]SnapmirrorPolicyInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapmirrorPolicyInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *SnapmirrorPolicyGetIterResponseResult) AttributesList() SnapmirrorPolicyGetIterResponseResultAttributesList {

	var r SnapmirrorPolicyGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr

	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyGetIterResponseResult) SetAttributesList(newValue SnapmirrorPolicyGetIterResponseResultAttributesList) *SnapmirrorPolicyGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *SnapmirrorPolicyGetIterResponseResult) NextTag() string {

	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr

	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyGetIterResponseResult) SetNextTag(newValue string) *SnapmirrorPolicyGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *SnapmirrorPolicyGetIterResponseResult) NumRecords() int {

	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr

	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyGetIterResponseResult) SetNumRecords(newValue int) *SnapmirrorPolicyGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
