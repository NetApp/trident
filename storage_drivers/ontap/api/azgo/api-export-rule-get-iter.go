package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// ExportRuleGetIterRequest is a structure to represent a export-rule-get-iter Request ZAPI object
type ExportRuleGetIterRequest struct {
	XMLName              xml.Name                                   `xml:"export-rule-get-iter"`
	DesiredAttributesPtr *ExportRuleGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                       `xml:"max-records"`
	QueryPtr             *ExportRuleGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                    `xml:"tag"`
}

// ExportRuleGetIterResponse is a structure to represent a export-rule-get-iter Response ZAPI object
type ExportRuleGetIterResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          ExportRuleGetIterResponseResult `xml:"results"`
}

// NewExportRuleGetIterResponse is a factory method for creating new instances of ExportRuleGetIterResponse objects
func NewExportRuleGetIterResponse() *ExportRuleGetIterResponse {
	return &ExportRuleGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ExportRuleGetIterResponseResult is a structure to represent a export-rule-get-iter Response Result ZAPI object
type ExportRuleGetIterResponseResult struct {
	XMLName           xml.Name                                       `xml:"results"`
	ResultStatusAttr  string                                         `xml:"status,attr"`
	ResultReasonAttr  string                                         `xml:"reason,attr"`
	ResultErrnoAttr   string                                         `xml:"errno,attr"`
	AttributesListPtr *ExportRuleGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                        `xml:"next-tag"`
	NumRecordsPtr     *int                                           `xml:"num-records"`
}

// NewExportRuleGetIterRequest is a factory method for creating new instances of ExportRuleGetIterRequest objects
func NewExportRuleGetIterRequest() *ExportRuleGetIterRequest {
	return &ExportRuleGetIterRequest{}
}

// NewExportRuleGetIterResponseResult is a factory method for creating new instances of ExportRuleGetIterResponseResult objects
func NewExportRuleGetIterResponseResult() *ExportRuleGetIterResponseResult {
	return &ExportRuleGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportRuleGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*ExportRuleGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportRuleGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*ExportRuleGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "ExportRuleGetIterRequest", NewExportRuleGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*ExportRuleGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *ExportRuleGetIterRequest) executeWithIteration(zr *ZapiRunner) (*ExportRuleGetIterResponse, error) {
	combined := NewExportRuleGetIterResponse()
	combined.Result.SetAttributesList(ExportRuleGetIterResponseResultAttributesList{})
	var nextTagPtr *string
	done := false
	for done != true {
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
				combined.Result.SetAttributesList(ExportRuleGetIterResponseResultAttributesList{})
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

// ExportRuleGetIterRequestDesiredAttributes is a wrapper
type ExportRuleGetIterRequestDesiredAttributes struct {
	XMLName           xml.Name            `xml:"desired-attributes"`
	ExportRuleInfoPtr *ExportRuleInfoType `xml:"export-rule-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExportRuleInfo is a 'getter' method
func (o *ExportRuleGetIterRequestDesiredAttributes) ExportRuleInfo() ExportRuleInfoType {
	r := *o.ExportRuleInfoPtr
	return r
}

// SetExportRuleInfo is a fluent style 'setter' method that can be chained
func (o *ExportRuleGetIterRequestDesiredAttributes) SetExportRuleInfo(newValue ExportRuleInfoType) *ExportRuleGetIterRequestDesiredAttributes {
	o.ExportRuleInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *ExportRuleGetIterRequest) DesiredAttributes() ExportRuleGetIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *ExportRuleGetIterRequest) SetDesiredAttributes(newValue ExportRuleGetIterRequestDesiredAttributes) *ExportRuleGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *ExportRuleGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *ExportRuleGetIterRequest) SetMaxRecords(newValue int) *ExportRuleGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// ExportRuleGetIterRequestQuery is a wrapper
type ExportRuleGetIterRequestQuery struct {
	XMLName           xml.Name            `xml:"query"`
	ExportRuleInfoPtr *ExportRuleInfoType `xml:"export-rule-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExportRuleInfo is a 'getter' method
func (o *ExportRuleGetIterRequestQuery) ExportRuleInfo() ExportRuleInfoType {
	r := *o.ExportRuleInfoPtr
	return r
}

// SetExportRuleInfo is a fluent style 'setter' method that can be chained
func (o *ExportRuleGetIterRequestQuery) SetExportRuleInfo(newValue ExportRuleInfoType) *ExportRuleGetIterRequestQuery {
	o.ExportRuleInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *ExportRuleGetIterRequest) Query() ExportRuleGetIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *ExportRuleGetIterRequest) SetQuery(newValue ExportRuleGetIterRequestQuery) *ExportRuleGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *ExportRuleGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *ExportRuleGetIterRequest) SetTag(newValue string) *ExportRuleGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// ExportRuleGetIterResponseResultAttributesList is a wrapper
type ExportRuleGetIterResponseResultAttributesList struct {
	XMLName           xml.Name             `xml:"attributes-list"`
	ExportRuleInfoPtr []ExportRuleInfoType `xml:"export-rule-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExportRuleInfo is a 'getter' method
func (o *ExportRuleGetIterResponseResultAttributesList) ExportRuleInfo() []ExportRuleInfoType {
	r := o.ExportRuleInfoPtr
	return r
}

// SetExportRuleInfo is a fluent style 'setter' method that can be chained
func (o *ExportRuleGetIterResponseResultAttributesList) SetExportRuleInfo(newValue []ExportRuleInfoType) *ExportRuleGetIterResponseResultAttributesList {
	newSlice := make([]ExportRuleInfoType, len(newValue))
	copy(newSlice, newValue)
	o.ExportRuleInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *ExportRuleGetIterResponseResultAttributesList) values() []ExportRuleInfoType {
	r := o.ExportRuleInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *ExportRuleGetIterResponseResultAttributesList) setValues(newValue []ExportRuleInfoType) *ExportRuleGetIterResponseResultAttributesList {
	newSlice := make([]ExportRuleInfoType, len(newValue))
	copy(newSlice, newValue)
	o.ExportRuleInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *ExportRuleGetIterResponseResult) AttributesList() ExportRuleGetIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *ExportRuleGetIterResponseResult) SetAttributesList(newValue ExportRuleGetIterResponseResultAttributesList) *ExportRuleGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *ExportRuleGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *ExportRuleGetIterResponseResult) SetNextTag(newValue string) *ExportRuleGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *ExportRuleGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *ExportRuleGetIterResponseResult) SetNumRecords(newValue int) *ExportRuleGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
