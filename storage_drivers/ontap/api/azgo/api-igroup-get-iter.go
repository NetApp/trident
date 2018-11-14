package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IgroupGetIterRequest is a structure to represent a igroup-get-iter Request ZAPI object
type IgroupGetIterRequest struct {
	XMLName              xml.Name                               `xml:"igroup-get-iter"`
	DesiredAttributesPtr *IgroupGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                   `xml:"max-records"`
	QueryPtr             *IgroupGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                `xml:"tag"`
}

// IgroupGetIterResponse is a structure to represent a igroup-get-iter Response ZAPI object
type IgroupGetIterResponse struct {
	XMLName         xml.Name                    `xml:"netapp"`
	ResponseVersion string                      `xml:"version,attr"`
	ResponseXmlns   string                      `xml:"xmlns,attr"`
	Result          IgroupGetIterResponseResult `xml:"results"`
}

// NewIgroupGetIterResponse is a factory method for creating new instances of IgroupGetIterResponse objects
func NewIgroupGetIterResponse() *IgroupGetIterResponse {
	return &IgroupGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *IgroupGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// IgroupGetIterResponseResult is a structure to represent a igroup-get-iter Response Result ZAPI object
type IgroupGetIterResponseResult struct {
	XMLName           xml.Name                                   `xml:"results"`
	ResultStatusAttr  string                                     `xml:"status,attr"`
	ResultReasonAttr  string                                     `xml:"reason,attr"`
	ResultErrnoAttr   string                                     `xml:"errno,attr"`
	AttributesListPtr *IgroupGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                    `xml:"next-tag"`
	NumRecordsPtr     *int                                       `xml:"num-records"`
}

// NewIgroupGetIterRequest is a factory method for creating new instances of IgroupGetIterRequest objects
func NewIgroupGetIterRequest() *IgroupGetIterRequest {
	return &IgroupGetIterRequest{}
}

// NewIgroupGetIterResponseResult is a factory method for creating new instances of IgroupGetIterResponseResult objects
func NewIgroupGetIterResponseResult() *IgroupGetIterResponseResult {
	return &IgroupGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *IgroupGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *IgroupGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IgroupGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*IgroupGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IgroupGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*IgroupGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "IgroupGetIterRequest", NewIgroupGetIterResponse())
	return result.(*IgroupGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *IgroupGetIterRequest) executeWithIteration(zr *ZapiRunner) (*IgroupGetIterResponse, error) {
	combined := NewIgroupGetIterResponse()
	combined.Result.SetAttributesList(IgroupGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(IgroupGetIterResponseResultAttributesList{})
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

// IgroupGetIterRequestDesiredAttributes is a wrapper
type IgroupGetIterRequestDesiredAttributes struct {
	XMLName               xml.Name                `xml:"desired-attributes"`
	InitiatorGroupInfoPtr *InitiatorGroupInfoType `xml:"initiator-group-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// InitiatorGroupInfo is a 'getter' method
func (o *IgroupGetIterRequestDesiredAttributes) InitiatorGroupInfo() InitiatorGroupInfoType {
	r := *o.InitiatorGroupInfoPtr
	return r
}

// SetInitiatorGroupInfo is a fluent style 'setter' method that can be chained
func (o *IgroupGetIterRequestDesiredAttributes) SetInitiatorGroupInfo(newValue InitiatorGroupInfoType) *IgroupGetIterRequestDesiredAttributes {
	o.InitiatorGroupInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *IgroupGetIterRequest) DesiredAttributes() IgroupGetIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *IgroupGetIterRequest) SetDesiredAttributes(newValue IgroupGetIterRequestDesiredAttributes) *IgroupGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *IgroupGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *IgroupGetIterRequest) SetMaxRecords(newValue int) *IgroupGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// IgroupGetIterRequestQuery is a wrapper
type IgroupGetIterRequestQuery struct {
	XMLName               xml.Name                `xml:"query"`
	InitiatorGroupInfoPtr *InitiatorGroupInfoType `xml:"initiator-group-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// InitiatorGroupInfo is a 'getter' method
func (o *IgroupGetIterRequestQuery) InitiatorGroupInfo() InitiatorGroupInfoType {
	r := *o.InitiatorGroupInfoPtr
	return r
}

// SetInitiatorGroupInfo is a fluent style 'setter' method that can be chained
func (o *IgroupGetIterRequestQuery) SetInitiatorGroupInfo(newValue InitiatorGroupInfoType) *IgroupGetIterRequestQuery {
	o.InitiatorGroupInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *IgroupGetIterRequest) Query() IgroupGetIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *IgroupGetIterRequest) SetQuery(newValue IgroupGetIterRequestQuery) *IgroupGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *IgroupGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *IgroupGetIterRequest) SetTag(newValue string) *IgroupGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// IgroupGetIterResponseResultAttributesList is a wrapper
type IgroupGetIterResponseResultAttributesList struct {
	XMLName               xml.Name                 `xml:"attributes-list"`
	InitiatorGroupInfoPtr []InitiatorGroupInfoType `xml:"initiator-group-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IgroupGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// InitiatorGroupInfo is a 'getter' method
func (o *IgroupGetIterResponseResultAttributesList) InitiatorGroupInfo() []InitiatorGroupInfoType {
	r := o.InitiatorGroupInfoPtr
	return r
}

// SetInitiatorGroupInfo is a fluent style 'setter' method that can be chained
func (o *IgroupGetIterResponseResultAttributesList) SetInitiatorGroupInfo(newValue []InitiatorGroupInfoType) *IgroupGetIterResponseResultAttributesList {
	newSlice := make([]InitiatorGroupInfoType, len(newValue))
	copy(newSlice, newValue)
	o.InitiatorGroupInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *IgroupGetIterResponseResultAttributesList) values() []InitiatorGroupInfoType {
	r := o.InitiatorGroupInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *IgroupGetIterResponseResultAttributesList) setValues(newValue []InitiatorGroupInfoType) *IgroupGetIterResponseResultAttributesList {
	newSlice := make([]InitiatorGroupInfoType, len(newValue))
	copy(newSlice, newValue)
	o.InitiatorGroupInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *IgroupGetIterResponseResult) AttributesList() IgroupGetIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *IgroupGetIterResponseResult) SetAttributesList(newValue IgroupGetIterResponseResultAttributesList) *IgroupGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *IgroupGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *IgroupGetIterResponseResult) SetNextTag(newValue string) *IgroupGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *IgroupGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *IgroupGetIterResponseResult) SetNumRecords(newValue int) *IgroupGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
