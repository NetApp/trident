package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrSpaceGetIterRequest is a structure to represent a aggr-space-get-iter Request ZAPI object
type AggrSpaceGetIterRequest struct {
	XMLName              xml.Name                                  `xml:"aggr-space-get-iter"`
	DesiredAttributesPtr *AggrSpaceGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                      `xml:"max-records"`
	QueryPtr             *AggrSpaceGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                   `xml:"tag"`
}

// AggrSpaceGetIterResponse is a structure to represent a aggr-space-get-iter Response ZAPI object
type AggrSpaceGetIterResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          AggrSpaceGetIterResponseResult `xml:"results"`
}

// NewAggrSpaceGetIterResponse is a factory method for creating new instances of AggrSpaceGetIterResponse objects
func NewAggrSpaceGetIterResponse() *AggrSpaceGetIterResponse {
	return &AggrSpaceGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrSpaceGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *AggrSpaceGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// AggrSpaceGetIterResponseResult is a structure to represent a aggr-space-get-iter Response Result ZAPI object
type AggrSpaceGetIterResponseResult struct {
	XMLName           xml.Name                                      `xml:"results"`
	ResultStatusAttr  string                                        `xml:"status,attr"`
	ResultReasonAttr  string                                        `xml:"reason,attr"`
	ResultErrnoAttr   string                                        `xml:"errno,attr"`
	AttributesListPtr *AggrSpaceGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                       `xml:"next-tag"`
	NumRecordsPtr     *int                                          `xml:"num-records"`
}

// NewAggrSpaceGetIterRequest is a factory method for creating new instances of AggrSpaceGetIterRequest objects
func NewAggrSpaceGetIterRequest() *AggrSpaceGetIterRequest {
	return &AggrSpaceGetIterRequest{}
}

// NewAggrSpaceGetIterResponseResult is a factory method for creating new instances of AggrSpaceGetIterResponseResult objects
func NewAggrSpaceGetIterResponseResult() *AggrSpaceGetIterResponseResult {
	return &AggrSpaceGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *AggrSpaceGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *AggrSpaceGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrSpaceGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrSpaceGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *AggrSpaceGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*AggrSpaceGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *AggrSpaceGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*AggrSpaceGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "AggrSpaceGetIterRequest", NewAggrSpaceGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*AggrSpaceGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *AggrSpaceGetIterRequest) executeWithIteration(zr *ZapiRunner) (*AggrSpaceGetIterResponse, error) {
	combined := NewAggrSpaceGetIterResponse()
	combined.Result.SetAttributesList(AggrSpaceGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(AggrSpaceGetIterResponseResultAttributesList{})
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

// AggrSpaceGetIterRequestDesiredAttributes is a wrapper
type AggrSpaceGetIterRequestDesiredAttributes struct {
	XMLName             xml.Name              `xml:"desired-attributes"`
	SpaceInformationPtr *SpaceInformationType `xml:"space-information"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrSpaceGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// SpaceInformation is a 'getter' method
func (o *AggrSpaceGetIterRequestDesiredAttributes) SpaceInformation() SpaceInformationType {
	r := *o.SpaceInformationPtr
	return r
}

// SetSpaceInformation is a fluent style 'setter' method that can be chained
func (o *AggrSpaceGetIterRequestDesiredAttributes) SetSpaceInformation(newValue SpaceInformationType) *AggrSpaceGetIterRequestDesiredAttributes {
	o.SpaceInformationPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *AggrSpaceGetIterRequest) DesiredAttributes() AggrSpaceGetIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *AggrSpaceGetIterRequest) SetDesiredAttributes(newValue AggrSpaceGetIterRequestDesiredAttributes) *AggrSpaceGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *AggrSpaceGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *AggrSpaceGetIterRequest) SetMaxRecords(newValue int) *AggrSpaceGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// AggrSpaceGetIterRequestQuery is a wrapper
type AggrSpaceGetIterRequestQuery struct {
	XMLName             xml.Name              `xml:"query"`
	SpaceInformationPtr *SpaceInformationType `xml:"space-information"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrSpaceGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// SpaceInformation is a 'getter' method
func (o *AggrSpaceGetIterRequestQuery) SpaceInformation() SpaceInformationType {
	r := *o.SpaceInformationPtr
	return r
}

// SetSpaceInformation is a fluent style 'setter' method that can be chained
func (o *AggrSpaceGetIterRequestQuery) SetSpaceInformation(newValue SpaceInformationType) *AggrSpaceGetIterRequestQuery {
	o.SpaceInformationPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *AggrSpaceGetIterRequest) Query() AggrSpaceGetIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *AggrSpaceGetIterRequest) SetQuery(newValue AggrSpaceGetIterRequestQuery) *AggrSpaceGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *AggrSpaceGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *AggrSpaceGetIterRequest) SetTag(newValue string) *AggrSpaceGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// AggrSpaceGetIterResponseResultAttributesList is a wrapper
type AggrSpaceGetIterResponseResultAttributesList struct {
	XMLName             xml.Name               `xml:"attributes-list"`
	SpaceInformationPtr []SpaceInformationType `xml:"space-information"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrSpaceGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// SpaceInformation is a 'getter' method
func (o *AggrSpaceGetIterResponseResultAttributesList) SpaceInformation() []SpaceInformationType {
	r := o.SpaceInformationPtr
	return r
}

// SetSpaceInformation is a fluent style 'setter' method that can be chained
func (o *AggrSpaceGetIterResponseResultAttributesList) SetSpaceInformation(newValue []SpaceInformationType) *AggrSpaceGetIterResponseResultAttributesList {
	newSlice := make([]SpaceInformationType, len(newValue))
	copy(newSlice, newValue)
	o.SpaceInformationPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *AggrSpaceGetIterResponseResultAttributesList) values() []SpaceInformationType {
	r := o.SpaceInformationPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *AggrSpaceGetIterResponseResultAttributesList) setValues(newValue []SpaceInformationType) *AggrSpaceGetIterResponseResultAttributesList {
	newSlice := make([]SpaceInformationType, len(newValue))
	copy(newSlice, newValue)
	o.SpaceInformationPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *AggrSpaceGetIterResponseResult) AttributesList() AggrSpaceGetIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *AggrSpaceGetIterResponseResult) SetAttributesList(newValue AggrSpaceGetIterResponseResultAttributesList) *AggrSpaceGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *AggrSpaceGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *AggrSpaceGetIterResponseResult) SetNextTag(newValue string) *AggrSpaceGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *AggrSpaceGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *AggrSpaceGetIterResponseResult) SetNumRecords(newValue int) *AggrSpaceGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
