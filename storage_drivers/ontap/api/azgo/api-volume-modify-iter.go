// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeModifyIterRequest is a structure to represent a volume-modify-iter Request ZAPI object
type VolumeModifyIterRequest struct {
	XMLName              xml.Name                           `xml:"volume-modify-iter"`
	AttributesPtr        *VolumeModifyIterRequestAttributes `xml:"attributes"`
	ContinueOnFailurePtr *bool                              `xml:"continue-on-failure"`
	MaxFailureCountPtr   *int                               `xml:"max-failure-count"`
	MaxRecordsPtr        *int                               `xml:"max-records"`
	QueryPtr             *VolumeModifyIterRequestQuery      `xml:"query"`
	ReturnFailureListPtr *bool                              `xml:"return-failure-list"`
	ReturnSuccessListPtr *bool                              `xml:"return-success-list"`
	TagPtr               *string                            `xml:"tag"`
}

// VolumeModifyIterResponse is a structure to represent a volume-modify-iter Response ZAPI object
type VolumeModifyIterResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          VolumeModifyIterResponseResult `xml:"results"`
}

// NewVolumeModifyIterResponse is a factory method for creating new instances of VolumeModifyIterResponse objects
func NewVolumeModifyIterResponse() *VolumeModifyIterResponse {
	return &VolumeModifyIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeModifyIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeModifyIterResponseResult is a structure to represent a volume-modify-iter Response Result ZAPI object
type VolumeModifyIterResponseResult struct {
	XMLName          xml.Name                                   `xml:"results"`
	ResultStatusAttr string                                     `xml:"status,attr"`
	ResultReasonAttr string                                     `xml:"reason,attr"`
	ResultErrnoAttr  string                                     `xml:"errno,attr"`
	FailureListPtr   *VolumeModifyIterResponseResultFailureList `xml:"failure-list"`
	NextTagPtr       *string                                    `xml:"next-tag"`
	NumFailedPtr     *int                                       `xml:"num-failed"`
	NumSucceededPtr  *int                                       `xml:"num-succeeded"`
	SuccessListPtr   *VolumeModifyIterResponseResultSuccessList `xml:"success-list"`
}

// NewVolumeModifyIterRequest is a factory method for creating new instances of VolumeModifyIterRequest objects
func NewVolumeModifyIterRequest() *VolumeModifyIterRequest {
	return &VolumeModifyIterRequest{}
}

// NewVolumeModifyIterResponseResult is a factory method for creating new instances of VolumeModifyIterResponseResult objects
func NewVolumeModifyIterResponseResult() *VolumeModifyIterResponseResult {
	return &VolumeModifyIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeModifyIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeModifyIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeModifyIterRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeModifyIterResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeModifyIterRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeModifyIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeModifyIterRequest", NewVolumeModifyIterResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeModifyIterResponse), err
}

// VolumeModifyIterRequestAttributes is a wrapper
type VolumeModifyIterRequestAttributes struct {
	XMLName             xml.Name              `xml:"attributes"`
	VolumeAttributesPtr *VolumeAttributesType `xml:"volume-attributes"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterRequestAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeAttributes is a 'getter' method
func (o *VolumeModifyIterRequestAttributes) VolumeAttributes() VolumeAttributesType {
	r := *o.VolumeAttributesPtr
	return r
}

// SetVolumeAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequestAttributes) SetVolumeAttributes(newValue VolumeAttributesType) *VolumeModifyIterRequestAttributes {
	o.VolumeAttributesPtr = &newValue
	return o
}

// Attributes is a 'getter' method
func (o *VolumeModifyIterRequest) Attributes() VolumeModifyIterRequestAttributes {
	r := *o.AttributesPtr
	return r
}

// SetAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetAttributes(newValue VolumeModifyIterRequestAttributes) *VolumeModifyIterRequest {
	o.AttributesPtr = &newValue
	return o
}

// ContinueOnFailure is a 'getter' method
func (o *VolumeModifyIterRequest) ContinueOnFailure() bool {
	r := *o.ContinueOnFailurePtr
	return r
}

// SetContinueOnFailure is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetContinueOnFailure(newValue bool) *VolumeModifyIterRequest {
	o.ContinueOnFailurePtr = &newValue
	return o
}

// MaxFailureCount is a 'getter' method
func (o *VolumeModifyIterRequest) MaxFailureCount() int {
	r := *o.MaxFailureCountPtr
	return r
}

// SetMaxFailureCount is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetMaxFailureCount(newValue int) *VolumeModifyIterRequest {
	o.MaxFailureCountPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *VolumeModifyIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetMaxRecords(newValue int) *VolumeModifyIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// VolumeModifyIterRequestQuery is a wrapper
type VolumeModifyIterRequestQuery struct {
	XMLName             xml.Name              `xml:"query"`
	VolumeAttributesPtr *VolumeAttributesType `xml:"volume-attributes"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeAttributes is a 'getter' method
func (o *VolumeModifyIterRequestQuery) VolumeAttributes() VolumeAttributesType {
	r := *o.VolumeAttributesPtr
	return r
}

// SetVolumeAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequestQuery) SetVolumeAttributes(newValue VolumeAttributesType) *VolumeModifyIterRequestQuery {
	o.VolumeAttributesPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *VolumeModifyIterRequest) Query() VolumeModifyIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetQuery(newValue VolumeModifyIterRequestQuery) *VolumeModifyIterRequest {
	o.QueryPtr = &newValue
	return o
}

// ReturnFailureList is a 'getter' method
func (o *VolumeModifyIterRequest) ReturnFailureList() bool {
	r := *o.ReturnFailureListPtr
	return r
}

// SetReturnFailureList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetReturnFailureList(newValue bool) *VolumeModifyIterRequest {
	o.ReturnFailureListPtr = &newValue
	return o
}

// ReturnSuccessList is a 'getter' method
func (o *VolumeModifyIterRequest) ReturnSuccessList() bool {
	r := *o.ReturnSuccessListPtr
	return r
}

// SetReturnSuccessList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetReturnSuccessList(newValue bool) *VolumeModifyIterRequest {
	o.ReturnSuccessListPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *VolumeModifyIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterRequest) SetTag(newValue string) *VolumeModifyIterRequest {
	o.TagPtr = &newValue
	return o
}

// VolumeModifyIterResponseResultFailureList is a wrapper
type VolumeModifyIterResponseResultFailureList struct {
	XMLName                 xml.Name                   `xml:"failure-list"`
	VolumeModifyIterInfoPtr []VolumeModifyIterInfoType `xml:"volume-modify-iter-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterResponseResultFailureList) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeModifyIterInfo is a 'getter' method
func (o *VolumeModifyIterResponseResultFailureList) VolumeModifyIterInfo() []VolumeModifyIterInfoType {
	r := o.VolumeModifyIterInfoPtr
	return r
}

// SetVolumeModifyIterInfo is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResultFailureList) SetVolumeModifyIterInfo(newValue []VolumeModifyIterInfoType) *VolumeModifyIterResponseResultFailureList {
	newSlice := make([]VolumeModifyIterInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeModifyIterInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *VolumeModifyIterResponseResultFailureList) values() []VolumeModifyIterInfoType {
	r := o.VolumeModifyIterInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResultFailureList) setValues(newValue []VolumeModifyIterInfoType) *VolumeModifyIterResponseResultFailureList {
	newSlice := make([]VolumeModifyIterInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeModifyIterInfoPtr = newSlice
	return o
}

// FailureList is a 'getter' method
func (o *VolumeModifyIterResponseResult) FailureList() VolumeModifyIterResponseResultFailureList {
	r := *o.FailureListPtr
	return r
}

// SetFailureList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResult) SetFailureList(newValue VolumeModifyIterResponseResultFailureList) *VolumeModifyIterResponseResult {
	o.FailureListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *VolumeModifyIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResult) SetNextTag(newValue string) *VolumeModifyIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumFailed is a 'getter' method
func (o *VolumeModifyIterResponseResult) NumFailed() int {
	r := *o.NumFailedPtr
	return r
}

// SetNumFailed is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResult) SetNumFailed(newValue int) *VolumeModifyIterResponseResult {
	o.NumFailedPtr = &newValue
	return o
}

// NumSucceeded is a 'getter' method
func (o *VolumeModifyIterResponseResult) NumSucceeded() int {
	r := *o.NumSucceededPtr
	return r
}

// SetNumSucceeded is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResult) SetNumSucceeded(newValue int) *VolumeModifyIterResponseResult {
	o.NumSucceededPtr = &newValue
	return o
}

// VolumeModifyIterResponseResultSuccessList is a wrapper
type VolumeModifyIterResponseResultSuccessList struct {
	XMLName                 xml.Name                   `xml:"success-list"`
	VolumeModifyIterInfoPtr []VolumeModifyIterInfoType `xml:"volume-modify-iter-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterResponseResultSuccessList) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeModifyIterInfo is a 'getter' method
func (o *VolumeModifyIterResponseResultSuccessList) VolumeModifyIterInfo() []VolumeModifyIterInfoType {
	r := o.VolumeModifyIterInfoPtr
	return r
}

// SetVolumeModifyIterInfo is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResultSuccessList) SetVolumeModifyIterInfo(newValue []VolumeModifyIterInfoType) *VolumeModifyIterResponseResultSuccessList {
	newSlice := make([]VolumeModifyIterInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeModifyIterInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *VolumeModifyIterResponseResultSuccessList) values() []VolumeModifyIterInfoType {
	r := o.VolumeModifyIterInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResultSuccessList) setValues(newValue []VolumeModifyIterInfoType) *VolumeModifyIterResponseResultSuccessList {
	newSlice := make([]VolumeModifyIterInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeModifyIterInfoPtr = newSlice
	return o
}

// SuccessList is a 'getter' method
func (o *VolumeModifyIterResponseResult) SuccessList() VolumeModifyIterResponseResultSuccessList {
	r := *o.SuccessListPtr
	return r
}

// SetSuccessList is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterResponseResult) SetSuccessList(newValue VolumeModifyIterResponseResultSuccessList) *VolumeModifyIterResponseResult {
	o.SuccessListPtr = &newValue
	return o
}
