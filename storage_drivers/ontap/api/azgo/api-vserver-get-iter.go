package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VserverGetIterRequest is a structure to represent a vserver-get-iter Request ZAPI object
type VserverGetIterRequest struct {
	XMLName              xml.Name                                `xml:"vserver-get-iter"`
	DesiredAttributesPtr *VserverGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                    `xml:"max-records"`
	QueryPtr             *VserverGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                 `xml:"tag"`
}

// VserverGetIterResponse is a structure to represent a vserver-get-iter Response ZAPI object
type VserverGetIterResponse struct {
	XMLName         xml.Name                     `xml:"netapp"`
	ResponseVersion string                       `xml:"version,attr"`
	ResponseXmlns   string                       `xml:"xmlns,attr"`
	Result          VserverGetIterResponseResult `xml:"results"`
}

// NewVserverGetIterResponse is a factory method for creating new instances of VserverGetIterResponse objects
func NewVserverGetIterResponse() *VserverGetIterResponse {
	return &VserverGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VserverGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VserverGetIterResponseResult is a structure to represent a vserver-get-iter Response Result ZAPI object
type VserverGetIterResponseResult struct {
	XMLName           xml.Name                                    `xml:"results"`
	ResultStatusAttr  string                                      `xml:"status,attr"`
	ResultReasonAttr  string                                      `xml:"reason,attr"`
	ResultErrnoAttr   string                                      `xml:"errno,attr"`
	AttributesListPtr *VserverGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                     `xml:"next-tag"`
	NumRecordsPtr     *int                                        `xml:"num-records"`
}

// NewVserverGetIterRequest is a factory method for creating new instances of VserverGetIterRequest objects
func NewVserverGetIterRequest() *VserverGetIterRequest {
	return &VserverGetIterRequest{}
}

// NewVserverGetIterResponseResult is a factory method for creating new instances of VserverGetIterResponseResult objects
func NewVserverGetIterResponseResult() *VserverGetIterResponseResult {
	return &VserverGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VserverGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VserverGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VserverGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*VserverGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VserverGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*VserverGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "VserverGetIterRequest", NewVserverGetIterResponse())
	return result.(*VserverGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *VserverGetIterRequest) executeWithIteration(zr *ZapiRunner) (*VserverGetIterResponse, error) {
	combined := NewVserverGetIterResponse()
	combined.Result.SetAttributesList(VserverGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(VserverGetIterResponseResultAttributesList{})
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

// VserverGetIterRequestDesiredAttributes is a wrapper
type VserverGetIterRequestDesiredAttributes struct {
	XMLName        xml.Name         `xml:"desired-attributes"`
	VserverInfoPtr *VserverInfoType `xml:"vserver-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// VserverInfo is a 'getter' method
func (o *VserverGetIterRequestDesiredAttributes) VserverInfo() VserverInfoType {
	r := *o.VserverInfoPtr
	return r
}

// SetVserverInfo is a fluent style 'setter' method that can be chained
func (o *VserverGetIterRequestDesiredAttributes) SetVserverInfo(newValue VserverInfoType) *VserverGetIterRequestDesiredAttributes {
	o.VserverInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *VserverGetIterRequest) DesiredAttributes() VserverGetIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *VserverGetIterRequest) SetDesiredAttributes(newValue VserverGetIterRequestDesiredAttributes) *VserverGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *VserverGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *VserverGetIterRequest) SetMaxRecords(newValue int) *VserverGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// VserverGetIterRequestQuery is a wrapper
type VserverGetIterRequestQuery struct {
	XMLName        xml.Name         `xml:"query"`
	VserverInfoPtr *VserverInfoType `xml:"vserver-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// VserverInfo is a 'getter' method
func (o *VserverGetIterRequestQuery) VserverInfo() VserverInfoType {
	r := *o.VserverInfoPtr
	return r
}

// SetVserverInfo is a fluent style 'setter' method that can be chained
func (o *VserverGetIterRequestQuery) SetVserverInfo(newValue VserverInfoType) *VserverGetIterRequestQuery {
	o.VserverInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *VserverGetIterRequest) Query() VserverGetIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *VserverGetIterRequest) SetQuery(newValue VserverGetIterRequestQuery) *VserverGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *VserverGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *VserverGetIterRequest) SetTag(newValue string) *VserverGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// VserverGetIterResponseResultAttributesList is a wrapper
type VserverGetIterResponseResultAttributesList struct {
	XMLName        xml.Name          `xml:"attributes-list"`
	VserverInfoPtr []VserverInfoType `xml:"vserver-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// VserverInfo is a 'getter' method
func (o *VserverGetIterResponseResultAttributesList) VserverInfo() []VserverInfoType {
	r := o.VserverInfoPtr
	return r
}

// SetVserverInfo is a fluent style 'setter' method that can be chained
func (o *VserverGetIterResponseResultAttributesList) SetVserverInfo(newValue []VserverInfoType) *VserverGetIterResponseResultAttributesList {
	newSlice := make([]VserverInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VserverInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *VserverGetIterResponseResultAttributesList) values() []VserverInfoType {
	r := o.VserverInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *VserverGetIterResponseResultAttributesList) setValues(newValue []VserverInfoType) *VserverGetIterResponseResultAttributesList {
	newSlice := make([]VserverInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VserverInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *VserverGetIterResponseResult) AttributesList() VserverGetIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *VserverGetIterResponseResult) SetAttributesList(newValue VserverGetIterResponseResultAttributesList) *VserverGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *VserverGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *VserverGetIterResponseResult) SetNextTag(newValue string) *VserverGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *VserverGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *VserverGetIterResponseResult) SetNumRecords(newValue int) *VserverGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
