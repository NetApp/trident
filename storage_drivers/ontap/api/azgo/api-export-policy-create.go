package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// ExportPolicyCreateRequest is a structure to represent a export-policy-create Request ZAPI object
type ExportPolicyCreateRequest struct {
	XMLName         xml.Name              `xml:"export-policy-create"`
	PolicyNamePtr   *ExportPolicyNameType `xml:"policy-name"`
	ReturnRecordPtr *bool                 `xml:"return-record"`
}

// ExportPolicyCreateResponse is a structure to represent a export-policy-create Response ZAPI object
type ExportPolicyCreateResponse struct {
	XMLName         xml.Name                         `xml:"netapp"`
	ResponseVersion string                           `xml:"version,attr"`
	ResponseXmlns   string                           `xml:"xmlns,attr"`
	Result          ExportPolicyCreateResponseResult `xml:"results"`
}

// NewExportPolicyCreateResponse is a factory method for creating new instances of ExportPolicyCreateResponse objects
func NewExportPolicyCreateResponse() *ExportPolicyCreateResponse {
	return &ExportPolicyCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *ExportPolicyCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ExportPolicyCreateResponseResult is a structure to represent a export-policy-create Response Result ZAPI object
type ExportPolicyCreateResponseResult struct {
	XMLName          xml.Name                                `xml:"results"`
	ResultStatusAttr string                                  `xml:"status,attr"`
	ResultReasonAttr string                                  `xml:"reason,attr"`
	ResultErrnoAttr  string                                  `xml:"errno,attr"`
	ResultPtr        *ExportPolicyCreateResponseResultResult `xml:"result"`
}

// NewExportPolicyCreateRequest is a factory method for creating new instances of ExportPolicyCreateRequest objects
func NewExportPolicyCreateRequest() *ExportPolicyCreateRequest {
	return &ExportPolicyCreateRequest{}
}

// NewExportPolicyCreateResponseResult is a factory method for creating new instances of ExportPolicyCreateResponseResult objects
func NewExportPolicyCreateResponseResult() *ExportPolicyCreateResponseResult {
	return &ExportPolicyCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *ExportPolicyCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *ExportPolicyCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *ExportPolicyCreateRequest) ExecuteUsing(zr *ZapiRunner) (*ExportPolicyCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *ExportPolicyCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*ExportPolicyCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "ExportPolicyCreateRequest", NewExportPolicyCreateResponse())
	return result.(*ExportPolicyCreateResponse), err
}

// PolicyName is a 'getter' method
func (o *ExportPolicyCreateRequest) PolicyName() ExportPolicyNameType {
	r := *o.PolicyNamePtr
	return r
}

// SetPolicyName is a fluent style 'setter' method that can be chained
func (o *ExportPolicyCreateRequest) SetPolicyName(newValue ExportPolicyNameType) *ExportPolicyCreateRequest {
	o.PolicyNamePtr = &newValue
	return o
}

// ReturnRecord is a 'getter' method
func (o *ExportPolicyCreateRequest) ReturnRecord() bool {
	r := *o.ReturnRecordPtr
	return r
}

// SetReturnRecord is a fluent style 'setter' method that can be chained
func (o *ExportPolicyCreateRequest) SetReturnRecord(newValue bool) *ExportPolicyCreateRequest {
	o.ReturnRecordPtr = &newValue
	return o
}

// ExportPolicyCreateResponseResultResult is a wrapper
type ExportPolicyCreateResponseResultResult struct {
	XMLName             xml.Name              `xml:"result"`
	ExportPolicyInfoPtr *ExportPolicyInfoType `xml:"export-policy-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportPolicyCreateResponseResultResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExportPolicyInfo is a 'getter' method
func (o *ExportPolicyCreateResponseResultResult) ExportPolicyInfo() ExportPolicyInfoType {
	r := *o.ExportPolicyInfoPtr
	return r
}

// SetExportPolicyInfo is a fluent style 'setter' method that can be chained
func (o *ExportPolicyCreateResponseResultResult) SetExportPolicyInfo(newValue ExportPolicyInfoType) *ExportPolicyCreateResponseResultResult {
	o.ExportPolicyInfoPtr = &newValue
	return o
}

// values is a 'getter' method
func (o *ExportPolicyCreateResponseResultResult) values() ExportPolicyInfoType {
	r := *o.ExportPolicyInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *ExportPolicyCreateResponseResultResult) setValues(newValue ExportPolicyInfoType) *ExportPolicyCreateResponseResultResult {
	o.ExportPolicyInfoPtr = &newValue
	return o
}

// Result is a 'getter' method
func (o *ExportPolicyCreateResponseResult) Result() ExportPolicyCreateResponseResultResult {
	r := *o.ResultPtr
	return r
}

// SetResult is a fluent style 'setter' method that can be chained
func (o *ExportPolicyCreateResponseResult) SetResult(newValue ExportPolicyCreateResponseResultResult) *ExportPolicyCreateResponseResult {
	o.ResultPtr = &newValue
	return o
}
