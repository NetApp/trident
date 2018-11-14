package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// QtreeListIterRequest is a structure to represent a qtree-list-iter Request ZAPI object
type QtreeListIterRequest struct {
	XMLName              xml.Name                               `xml:"qtree-list-iter"`
	DesiredAttributesPtr *QtreeListIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                   `xml:"max-records"`
	QueryPtr             *QtreeListIterRequestQuery             `xml:"query"`
	TagPtr               *string                                `xml:"tag"`
}

// QtreeListIterResponse is a structure to represent a qtree-list-iter Response ZAPI object
type QtreeListIterResponse struct {
	XMLName         xml.Name                    `xml:"netapp"`
	ResponseVersion string                      `xml:"version,attr"`
	ResponseXmlns   string                      `xml:"xmlns,attr"`
	Result          QtreeListIterResponseResult `xml:"results"`
}

// NewQtreeListIterResponse is a factory method for creating new instances of QtreeListIterResponse objects
func NewQtreeListIterResponse() *QtreeListIterResponse {
	return &QtreeListIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeListIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *QtreeListIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// QtreeListIterResponseResult is a structure to represent a qtree-list-iter Response Result ZAPI object
type QtreeListIterResponseResult struct {
	XMLName           xml.Name                                   `xml:"results"`
	ResultStatusAttr  string                                     `xml:"status,attr"`
	ResultReasonAttr  string                                     `xml:"reason,attr"`
	ResultErrnoAttr   string                                     `xml:"errno,attr"`
	AttributesListPtr *QtreeListIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                    `xml:"next-tag"`
	NumRecordsPtr     *int                                       `xml:"num-records"`
}

// NewQtreeListIterRequest is a factory method for creating new instances of QtreeListIterRequest objects
func NewQtreeListIterRequest() *QtreeListIterRequest {
	return &QtreeListIterRequest{}
}

// NewQtreeListIterResponseResult is a factory method for creating new instances of QtreeListIterResponseResult objects
func NewQtreeListIterResponseResult() *QtreeListIterResponseResult {
	return &QtreeListIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *QtreeListIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *QtreeListIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeListIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeListIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *QtreeListIterRequest) ExecuteUsing(zr *ZapiRunner) (*QtreeListIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *QtreeListIterRequest) executeWithoutIteration(zr *ZapiRunner) (*QtreeListIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "QtreeListIterRequest", NewQtreeListIterResponse())
	return result.(*QtreeListIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *QtreeListIterRequest) executeWithIteration(zr *ZapiRunner) (*QtreeListIterResponse, error) {
	combined := NewQtreeListIterResponse()
	combined.Result.SetAttributesList(QtreeListIterResponseResultAttributesList{})
	var nextTagPtr *string
	done := false
	for done != true {
		n, err := o.executeWithoutIteration(zr)
		if err == nil {
			nextTagPtr = n.Result.NextTagPtr
			if nextTagPtr == nil {
				done = true
			} else {
				o.SetTag(*nextTagPtr)
			}
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
				combined.Result.SetAttributesList(QtreeListIterResponseResultAttributesList{})
			}
			combinedAttributesList := combined.Result.AttributesList()
			combinedAttributes := combinedAttributesList.values()

			resultAttributesList := n.Result.AttributesList()
			resultAttributes := resultAttributesList.values()

			combined.Result.AttributesListPtr.setValues(append(combinedAttributes, resultAttributes...))
		}

		if done == true {

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

// QtreeListIterRequestDesiredAttributes is a wrapper
type QtreeListIterRequestDesiredAttributes struct {
	XMLName      xml.Name       `xml:"desired-attributes"`
	QtreeInfoPtr *QtreeInfoType `xml:"qtree-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeListIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// QtreeInfo is a 'getter' method
func (o *QtreeListIterRequestDesiredAttributes) QtreeInfo() QtreeInfoType {
	r := *o.QtreeInfoPtr
	return r
}

// SetQtreeInfo is a fluent style 'setter' method that can be chained
func (o *QtreeListIterRequestDesiredAttributes) SetQtreeInfo(newValue QtreeInfoType) *QtreeListIterRequestDesiredAttributes {
	o.QtreeInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *QtreeListIterRequest) DesiredAttributes() QtreeListIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *QtreeListIterRequest) SetDesiredAttributes(newValue QtreeListIterRequestDesiredAttributes) *QtreeListIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *QtreeListIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *QtreeListIterRequest) SetMaxRecords(newValue int) *QtreeListIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// QtreeListIterRequestQuery is a wrapper
type QtreeListIterRequestQuery struct {
	XMLName      xml.Name       `xml:"query"`
	QtreeInfoPtr *QtreeInfoType `xml:"qtree-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeListIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// QtreeInfo is a 'getter' method
func (o *QtreeListIterRequestQuery) QtreeInfo() QtreeInfoType {
	r := *o.QtreeInfoPtr
	return r
}

// SetQtreeInfo is a fluent style 'setter' method that can be chained
func (o *QtreeListIterRequestQuery) SetQtreeInfo(newValue QtreeInfoType) *QtreeListIterRequestQuery {
	o.QtreeInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *QtreeListIterRequest) Query() QtreeListIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *QtreeListIterRequest) SetQuery(newValue QtreeListIterRequestQuery) *QtreeListIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *QtreeListIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *QtreeListIterRequest) SetTag(newValue string) *QtreeListIterRequest {
	o.TagPtr = &newValue
	return o
}

// QtreeListIterResponseResultAttributesList is a wrapper
type QtreeListIterResponseResultAttributesList struct {
	XMLName      xml.Name        `xml:"attributes-list"`
	QtreeInfoPtr []QtreeInfoType `xml:"qtree-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeListIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// QtreeInfo is a 'getter' method
func (o *QtreeListIterResponseResultAttributesList) QtreeInfo() []QtreeInfoType {
	r := o.QtreeInfoPtr
	return r
}

// SetQtreeInfo is a fluent style 'setter' method that can be chained
func (o *QtreeListIterResponseResultAttributesList) SetQtreeInfo(newValue []QtreeInfoType) *QtreeListIterResponseResultAttributesList {
	newSlice := make([]QtreeInfoType, len(newValue))
	copy(newSlice, newValue)
	o.QtreeInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *QtreeListIterResponseResultAttributesList) values() []QtreeInfoType {
	r := o.QtreeInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *QtreeListIterResponseResultAttributesList) setValues(newValue []QtreeInfoType) *QtreeListIterResponseResultAttributesList {
	newSlice := make([]QtreeInfoType, len(newValue))
	copy(newSlice, newValue)
	o.QtreeInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *QtreeListIterResponseResult) AttributesList() QtreeListIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *QtreeListIterResponseResult) SetAttributesList(newValue QtreeListIterResponseResultAttributesList) *QtreeListIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *QtreeListIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *QtreeListIterResponseResult) SetNextTag(newValue string) *QtreeListIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *QtreeListIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *QtreeListIterResponseResult) SetNumRecords(newValue int) *QtreeListIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
