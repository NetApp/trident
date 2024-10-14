package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// FcpServiceGetIterRequest is a structure to represent a fcp-service-get-iter Request ZAPI object
type FcpServiceGetIterRequest struct {
	XMLName              xml.Name                                   `xml:"fcp-service-get-iter"`
	DesiredAttributesPtr *FcpServiceGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                       `xml:"max-records"`
	QueryPtr             *FcpServiceGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                                    `xml:"tag"`
}

// FcpServiceGetIterResponse is a structure to represent a fcp-service-get-iter Response ZAPI object
type FcpServiceGetIterResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          FcpServiceGetIterResponseResult `xml:"results"`
}

// NewFcpServiceGetIterResponse is a factory method for creating new instances of FcpServiceGetIterResponse objects
func NewFcpServiceGetIterResponse() *FcpServiceGetIterResponse {
	return &FcpServiceGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpServiceGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *FcpServiceGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// FcpServiceGetIterResponseResult is a structure to represent a fcp-service-get-iter Response Result ZAPI object
type FcpServiceGetIterResponseResult struct {
	XMLName           xml.Name                                       `xml:"results"`
	ResultStatusAttr  string                                         `xml:"status,attr"`
	ResultReasonAttr  string                                         `xml:"reason,attr"`
	ResultErrnoAttr   string                                         `xml:"errno,attr"`
	AttributesListPtr *FcpServiceGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                        `xml:"next-tag"`
	NumRecordsPtr     *int                                           `xml:"num-records"`
}

// NewFcpServiceGetIterRequest is a factory method for creating new instances of FcpServiceGetIterRequest objects
func NewFcpServiceGetIterRequest() *FcpServiceGetIterRequest {
	return &FcpServiceGetIterRequest{}
}

// NewFcpServiceGetIterResponseResult is a factory method for creating new instances of FcpServiceGetIterResponseResult objects
func NewFcpServiceGetIterResponseResult() *FcpServiceGetIterResponseResult {
	return &FcpServiceGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *FcpServiceGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *FcpServiceGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpServiceGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpServiceGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *FcpServiceGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*FcpServiceGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *FcpServiceGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*FcpServiceGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "FcpServiceGetIterRequest", NewFcpServiceGetIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*FcpServiceGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *FcpServiceGetIterRequest) executeWithIteration(zr *ZapiRunner) (*FcpServiceGetIterResponse, error) {
	combined := NewFcpServiceGetIterResponse()
	combined.Result.SetAttributesList(FcpServiceGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(FcpServiceGetIterResponseResultAttributesList{})
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

// FcpServiceGetIterRequestDesiredAttributes is a wrapper
type FcpServiceGetIterRequestDesiredAttributes struct {
	XMLName           xml.Name            `xml:"desired-attributes"`
	FcpServiceInfoPtr *FcpServiceInfoType `xml:"fcp-service-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpServiceGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// FcpServiceInfo is a 'getter' method
func (o *FcpServiceGetIterRequestDesiredAttributes) FcpServiceInfo() FcpServiceInfoType {
	var r FcpServiceInfoType
	if o.FcpServiceInfoPtr == nil {
		return r
	}
	r = *o.FcpServiceInfoPtr

	return r
}

// SetFcpServiceInfo is a fluent style 'setter' method that can be chained
func (o *FcpServiceGetIterRequestDesiredAttributes) SetFcpServiceInfo(newValue FcpServiceInfoType) *FcpServiceGetIterRequestDesiredAttributes {
	o.FcpServiceInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *FcpServiceGetIterRequest) DesiredAttributes() FcpServiceGetIterRequestDesiredAttributes {
	var r FcpServiceGetIterRequestDesiredAttributes
	if o.DesiredAttributesPtr == nil {
		return r
	}
	r = *o.DesiredAttributesPtr

	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *FcpServiceGetIterRequest) SetDesiredAttributes(newValue FcpServiceGetIterRequestDesiredAttributes) *FcpServiceGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *FcpServiceGetIterRequest) MaxRecords() int {
	var r int
	if o.MaxRecordsPtr == nil {
		return r
	}
	r = *o.MaxRecordsPtr

	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *FcpServiceGetIterRequest) SetMaxRecords(newValue int) *FcpServiceGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// FcpServiceGetIterRequestQuery is a wrapper
type FcpServiceGetIterRequestQuery struct {
	XMLName           xml.Name            `xml:"query"`
	FcpServiceInfoPtr *FcpServiceInfoType `xml:"fcp-service-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpServiceGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// FcpServiceInfo is a 'getter' method
func (o *FcpServiceGetIterRequestQuery) FcpServiceInfo() FcpServiceInfoType {
	var r FcpServiceInfoType
	if o.FcpServiceInfoPtr == nil {
		return r
	}
	r = *o.FcpServiceInfoPtr

	return r
}

// SetFcpServiceInfo is a fluent style 'setter' method that can be chained
func (o *FcpServiceGetIterRequestQuery) SetFcpServiceInfo(newValue FcpServiceInfoType) *FcpServiceGetIterRequestQuery {
	o.FcpServiceInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *FcpServiceGetIterRequest) Query() FcpServiceGetIterRequestQuery {
	var r FcpServiceGetIterRequestQuery
	if o.QueryPtr == nil {
		return r
	}
	r = *o.QueryPtr

	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *FcpServiceGetIterRequest) SetQuery(newValue FcpServiceGetIterRequestQuery) *FcpServiceGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *FcpServiceGetIterRequest) Tag() string {
	var r string
	if o.TagPtr == nil {
		return r
	}
	r = *o.TagPtr

	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *FcpServiceGetIterRequest) SetTag(newValue string) *FcpServiceGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// FcpServiceGetIterResponseResultAttributesList is a wrapper
type FcpServiceGetIterResponseResultAttributesList struct {
	XMLName           xml.Name             `xml:"attributes-list"`
	FcpServiceInfoPtr []FcpServiceInfoType `xml:"fcp-service-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o FcpServiceGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// FcpServiceInfo is a 'getter' method
func (o *FcpServiceGetIterResponseResultAttributesList) FcpServiceInfo() []FcpServiceInfoType {
	r := o.FcpServiceInfoPtr
	return r
}

// SetFcpServiceInfo is a fluent style 'setter' method that can be chained
func (o *FcpServiceGetIterResponseResultAttributesList) SetFcpServiceInfo(newValue []FcpServiceInfoType) *FcpServiceGetIterResponseResultAttributesList {
	newSlice := make([]FcpServiceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.FcpServiceInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *FcpServiceGetIterResponseResultAttributesList) values() []FcpServiceInfoType {
	r := o.FcpServiceInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *FcpServiceGetIterResponseResultAttributesList) setValues(newValue []FcpServiceInfoType) *FcpServiceGetIterResponseResultAttributesList {
	newSlice := make([]FcpServiceInfoType, len(newValue))
	copy(newSlice, newValue)
	o.FcpServiceInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *FcpServiceGetIterResponseResult) AttributesList() FcpServiceGetIterResponseResultAttributesList {
	var r FcpServiceGetIterResponseResultAttributesList
	if o.AttributesListPtr == nil {
		return r
	}
	r = *o.AttributesListPtr

	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *FcpServiceGetIterResponseResult) SetAttributesList(newValue FcpServiceGetIterResponseResultAttributesList) *FcpServiceGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *FcpServiceGetIterResponseResult) NextTag() string {
	var r string
	if o.NextTagPtr == nil {
		return r
	}
	r = *o.NextTagPtr

	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *FcpServiceGetIterResponseResult) SetNextTag(newValue string) *FcpServiceGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *FcpServiceGetIterResponseResult) NumRecords() int {
	var r int
	if o.NumRecordsPtr == nil {
		return r
	}
	r = *o.NumRecordsPtr

	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *FcpServiceGetIterResponseResult) SetNumRecords(newValue int) *FcpServiceGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}
