package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrGetIterRequest is a structure to represent a aggr-get-iter Request ZAPI object
type AggrGetIterRequest struct {
	XMLName              xml.Name                             `xml:"aggr-get-iter"`
	DesiredAttributesPtr *AggrGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                 `xml:"max-records"`
	QueryPtr             *AggrGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                              `xml:"tag"`
}

// AggrGetIterResponse is a structure to represent a aggr-get-iter Response ZAPI object
type AggrGetIterResponse struct {
	XMLName         xml.Name                  `xml:"netapp"`
	ResponseVersion string                    `xml:"version,attr"`
	ResponseXmlns   string                    `xml:"xmlns,attr"`
	Result          AggrGetIterResponseResult `xml:"results"`
}

// NewAggrGetIterResponse is a factory method for creating new instances of AggrGetIterResponse objects
func NewAggrGetIterResponse() *AggrGetIterResponse {
	return &AggrGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *AggrGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// AggrGetIterResponseResult is a structure to represent a aggr-get-iter Response Result ZAPI object
type AggrGetIterResponseResult struct {
	XMLName           xml.Name                                 `xml:"results"`
	ResultStatusAttr  string                                   `xml:"status,attr"`
	ResultReasonAttr  string                                   `xml:"reason,attr"`
	ResultErrnoAttr   string                                   `xml:"errno,attr"`
	AttributesListPtr *AggrGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                  `xml:"next-tag"`
	NumRecordsPtr     *int                                     `xml:"num-records"`
}

// NewAggrGetIterRequest is a factory method for creating new instances of AggrGetIterRequest objects
func NewAggrGetIterRequest() *AggrGetIterRequest {
	return &AggrGetIterRequest{}
}

// NewAggrGetIterResponseResult is a factory method for creating new instances of AggrGetIterResponseResult objects
func NewAggrGetIterResponseResult() *AggrGetIterResponseResult {
	return &AggrGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *AggrGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *AggrGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *AggrGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*AggrGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *AggrGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*AggrGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "AggrGetIterRequest", NewAggrGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*AggrGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *AggrGetIterRequest) executeWithIteration(zr *ZapiRunner) (*AggrGetIterResponse, error) {
	combined := NewAggrGetIterResponse()
	combined.Result.SetAttributesList(AggrGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(AggrGetIterResponseResultAttributesList{})
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

// AggrGetIterRequestDesiredAttributes is a wrapper
type AggrGetIterRequestDesiredAttributes struct {
	XMLName           xml.Name            `xml:"desired-attributes"`
	AggrAttributesPtr *AggrAttributesType `xml:"aggr-attributes"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggrAttributes is a 'getter' method
func (o *AggrGetIterRequestDesiredAttributes) AggrAttributes() AggrAttributesType {
	r := *o.AggrAttributesPtr
	return r
}

// SetAggrAttributes is a fluent style 'setter' method that can be chained
func (o *AggrGetIterRequestDesiredAttributes) SetAggrAttributes(newValue AggrAttributesType) *AggrGetIterRequestDesiredAttributes {
	o.AggrAttributesPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *AggrGetIterRequest) DesiredAttributes() AggrGetIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *AggrGetIterRequest) SetDesiredAttributes(newValue AggrGetIterRequestDesiredAttributes) *AggrGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *AggrGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *AggrGetIterRequest) SetMaxRecords(newValue int) *AggrGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// AggrGetIterRequestQuery is a wrapper
type AggrGetIterRequestQuery struct {
	XMLName           xml.Name            `xml:"query"`
	AggrAttributesPtr *AggrAttributesType `xml:"aggr-attributes"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggrAttributes is a 'getter' method
func (o *AggrGetIterRequestQuery) AggrAttributes() AggrAttributesType {
	r := *o.AggrAttributesPtr
	return r
}

// SetAggrAttributes is a fluent style 'setter' method that can be chained
func (o *AggrGetIterRequestQuery) SetAggrAttributes(newValue AggrAttributesType) *AggrGetIterRequestQuery {
	o.AggrAttributesPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *AggrGetIterRequest) Query() AggrGetIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *AggrGetIterRequest) SetQuery(newValue AggrGetIterRequestQuery) *AggrGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *AggrGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *AggrGetIterRequest) SetTag(newValue string) *AggrGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// AggrGetIterResponseResultAttributesList is a wrapper
type AggrGetIterResponseResultAttributesList struct {
	XMLName           xml.Name             `xml:"attributes-list"`
	AggrAttributesPtr []AggrAttributesType `xml:"aggr-attributes"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggrAttributes is a 'getter' method
func (o *AggrGetIterResponseResultAttributesList) AggrAttributes() []AggrAttributesType {
	r := o.AggrAttributesPtr
	return r
}

// SetAggrAttributes is a fluent style 'setter' method that can be chained
func (o *AggrGetIterResponseResultAttributesList) SetAggrAttributes(newValue []AggrAttributesType) *AggrGetIterResponseResultAttributesList {
	newSlice := make([]AggrAttributesType, len(newValue))
	copy(newSlice, newValue)
	o.AggrAttributesPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *AggrGetIterResponseResultAttributesList) values() []AggrAttributesType {
	r := o.AggrAttributesPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *AggrGetIterResponseResultAttributesList) setValues(newValue []AggrAttributesType) *AggrGetIterResponseResultAttributesList {
	newSlice := make([]AggrAttributesType, len(newValue))
	copy(newSlice, newValue)
	o.AggrAttributesPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *AggrGetIterResponseResult) AttributesList() AggrGetIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *AggrGetIterResponseResult) SetAttributesList(newValue AggrGetIterResponseResultAttributesList) *AggrGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *AggrGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *AggrGetIterResponseResult) SetNextTag(newValue string) *AggrGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *AggrGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *AggrGetIterResponseResult) SetNumRecords(newValue int) *AggrGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
