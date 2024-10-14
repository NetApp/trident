package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// FcpInterfaceGetIterRequest is a structure to represent a fcp-interface-get-iter Request ZAPI object
type FcpInterfaceGetIterRequest struct {
	XMLName              xml.Name                                     `xml:"fcp-interface-get-iter"`
	DesiredAttributesPtr *FcpInterfaceGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                         `xml:"max-records"`
	QueryPtr             *FcpInterfaceGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                      `xml:"tag"`
}

// FcpInterfaceGetIterResponse is a structure to represent a fcp-interface-get-iter Response ZAPI object
type FcpInterfaceGetIterResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          FcpInterfaceGetIterResponseResult `xml:"results"`
}

// NewFcpInterfaceGetIterResponse is a factory method for creating new instances of FcpInterfaceGetIterResponse objects
func NewFcpInterfaceGetIterResponse() *FcpInterfaceGetIterResponse {
	return &FcpInterfaceGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpInterfaceGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *FcpInterfaceGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// FcpInterfaceGetIterResponseResult is a structure to represent a fcp-interface-get-iter Response Result ZAPI object
type FcpInterfaceGetIterResponseResult struct {
	XMLName           xml.Name                                         `xml:"results"`
	ResultStatusAttr  string                                           `xml:"status,attr"`
	ResultReasonAttr  string                                           `xml:"reason,attr"`
	ResultErrnoAttr   string                                           `xml:"errno,attr"`
	AttributesListPtr *FcpInterfaceGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                          `xml:"next-tag"`
	NumRecordsPtr     *int                                             `xml:"num-records"`
}

// NewFcpInterfaceGetIterRequest is a factory method for creating new instances of FcpInterfaceGetIterRequest objects
func NewFcpInterfaceGetIterRequest() *FcpInterfaceGetIterRequest {
	return &FcpInterfaceGetIterRequest{}
}

// NewFcpInterfaceGetIterResponseResult is a factory method for creating new instances of FcpInterfaceGetIterResponseResult objects
func NewFcpInterfaceGetIterResponseResult() *FcpInterfaceGetIterResponseResult {
	return &FcpInterfaceGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *FcpInterfaceGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *FcpInterfaceGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpInterfaceGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpInterfaceGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *FcpInterfaceGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*FcpInterfaceGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *FcpInterfaceGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*FcpInterfaceGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "FcpInterfaceGetIterRequest", NewFcpInterfaceGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*FcpInterfaceGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *FcpInterfaceGetIterRequest) executeWithIteration(zr *ZapiRunner) (*FcpInterfaceGetIterResponse, error) {
	combined := NewFcpInterfaceGetIterResponse()
	combined.Result.SetAttributesList(FcpInterfaceGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(FcpInterfaceGetIterResponseResultAttributesList{})
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

// FcpInterfaceGetIterRequestDesiredAttributes is a wrapper
type FcpInterfaceGetIterRequestDesiredAttributes struct {
	XMLName             xml.Name              `xml:"desired-attributes"`
	FcpInterfaceInfoPtr *FcpInterfaceInfoType `xml:"fcp-interface-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpInterfaceGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// FcpInterfaceInfo is a 'getter' method
func (o *FcpInterfaceGetIterRequestDesiredAttributes) FcpInterfaceInfo() FcpInterfaceInfoType {
	var r FcpInterfaceInfoType
	if o.FcpInterfaceInfoPtr == nil {
		return r
	}
	r = *o.FcpInterfaceInfoPtr

	return r
}

// SetFcpInterfaceInfo is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceGetIterRequestDesiredAttributes) SetFcpInterfaceInfo(newValue FcpInterfaceInfoType) *FcpInterfaceGetIterRequestDesiredAttributes {
	o.FcpInterfaceInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *FcpInterfaceGetIterRequest) DesiredAttributes() FcpInterfaceGetIterRequestDesiredAttributes {
	var r FcpInterfaceGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr

	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceGetIterRequest) SetDesiredAttributes(newValue FcpInterfaceGetIterRequestDesiredAttributes) *FcpInterfaceGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *FcpInterfaceGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr

	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceGetIterRequest) SetMaxRecords(newValue int) *FcpInterfaceGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// FcpInterfaceGetIterRequestQuery is a wrapper
type FcpInterfaceGetIterRequestQuery struct {
	XMLName             xml.Name              `xml:"query"`
	FcpInterfaceInfoPtr *FcpInterfaceInfoType `xml:"fcp-interface-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpInterfaceGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// FcpInterfaceInfo is a 'getter' method
func (o *FcpInterfaceGetIterRequestQuery) FcpInterfaceInfo() FcpInterfaceInfoType {
	var r FcpInterfaceInfoType
	if o.FcpInterfaceInfoPtr == nil {
		return r
	}
	r = *o.FcpInterfaceInfoPtr

	return r
}

// SetFcpInterfaceInfo is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceGetIterRequestQuery) SetFcpInterfaceInfo(newValue FcpInterfaceInfoType) *FcpInterfaceGetIterRequestQuery {
	o.FcpInterfaceInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *FcpInterfaceGetIterRequest) Query() FcpInterfaceGetIterRequestQuery {
	var r FcpInterfaceGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr

	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceGetIterRequest) SetQuery(newValue FcpInterfaceGetIterRequestQuery) *FcpInterfaceGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *FcpInterfaceGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr

	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceGetIterRequest) SetTag(newValue string) *FcpInterfaceGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// FcpInterfaceGetIterResponseResultAttributesList is a wrapper
type FcpInterfaceGetIterResponseResultAttributesList struct {
	XMLName             xml.Name               `xml:"attributes-list"`
	FcpInterfaceInfoPtr []FcpInterfaceInfoType `xml:"fcp-interface-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpInterfaceGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// FcpInterfaceInfo is a 'getter' method
func (o *FcpInterfaceGetIterResponseResultAttributesList) FcpInterfaceInfo() []FcpInterfaceInfoType {
	r := o.FcpInterfaceInfoPtr
	return r
}

// SetFcpInterfaceInfo is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceGetIterResponseResultAttributesList) SetFcpInterfaceInfo(newValue []FcpInterfaceInfoType) *FcpInterfaceGetIterResponseResultAttributesList {
	newSlice := make([]FcpInterfaceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.FcpInterfaceInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *FcpInterfaceGetIterResponseResultAttributesList) values() []FcpInterfaceInfoType {
	r := o.FcpInterfaceInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceGetIterResponseResultAttributesList) setValues(newValue []FcpInterfaceInfoType) *FcpInterfaceGetIterResponseResultAttributesList {
	newSlice := make([]FcpInterfaceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.FcpInterfaceInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *FcpInterfaceGetIterResponseResult) AttributesList() FcpInterfaceGetIterResponseResultAttributesList {
	var r FcpInterfaceGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr

	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceGetIterResponseResult) SetAttributesList(newValue FcpInterfaceGetIterResponseResultAttributesList) *FcpInterfaceGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *FcpInterfaceGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr

	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceGetIterResponseResult) SetNextTag(newValue string) *FcpInterfaceGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *FcpInterfaceGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr

	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *FcpInterfaceGetIterResponseResult) SetNumRecords(newValue int) *FcpInterfaceGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
