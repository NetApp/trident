package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// LunGetIterRequest is a structure to represent a lun-get-iter Request ZAPI object
type LunGetIterRequest struct {
	XMLName              xml.Name                            `xml:"lun-get-iter"`
	DesiredAttributesPtr *LunGetIterRequestDesiredAttributes `xml:"desired-attributes"`
	MaxRecordsPtr        *int                                `xml:"max-records"`
	QueryPtr             *LunGetIterRequestQuery             `xml:"query"`
	TagPtr               *string                             `xml:"tag"`
}

// LunGetIterResponse is a structure to represent a lun-get-iter Response ZAPI object
type LunGetIterResponse struct {
	XMLName         xml.Name                 `xml:"netapp"`
	ResponseVersion string                   `xml:"version,attr"`
	ResponseXmlns   string                   `xml:"xmlns,attr"`
	Result          LunGetIterResponseResult `xml:"results"`
}

// NewLunGetIterResponse is a factory method for creating new instances of LunGetIterResponse objects
func NewLunGetIterResponse() *LunGetIterResponse {
	return &LunGetIterResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetIterResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *LunGetIterResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// LunGetIterResponseResult is a structure to represent a lun-get-iter Response Result ZAPI object
type LunGetIterResponseResult struct {
	XMLName           xml.Name                                `xml:"results"`
	ResultStatusAttr  string                                  `xml:"status,attr"`
	ResultReasonAttr  string                                  `xml:"reason,attr"`
	ResultErrnoAttr   string                                  `xml:"errno,attr"`
	AttributesListPtr *LunGetIterResponseResultAttributesList `xml:"attributes-list"`
	NextTagPtr        *string                                 `xml:"next-tag"`
	NumRecordsPtr     *int                                    `xml:"num-records"`
	VolumeErrorsPtr   *LunGetIterResponseResultVolumeErrors   `xml:"volume-errors"`
}

// NewLunGetIterRequest is a factory method for creating new instances of LunGetIterRequest objects
func NewLunGetIterRequest() *LunGetIterRequest {
	return &LunGetIterRequest{}
}

// NewLunGetIterResponseResult is a factory method for creating new instances of LunGetIterResponseResult objects
func NewLunGetIterResponseResult() *LunGetIterResponseResult {
	return &LunGetIterResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *LunGetIterRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *LunGetIterResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetIterRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetIterResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunGetIterRequest) ExecuteUsing(zr *ZapiRunner) (*LunGetIterResponse, error) {
	return o.executeWithIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunGetIterRequest) executeWithoutIteration(zr *ZapiRunner) (*LunGetIterResponse, error) {
	result, err := zr.ExecuteUsing(o, "LunGetIterRequest", NewLunGetIterResponse())
	return result.(*LunGetIterResponse), err
}

// executeWithIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *LunGetIterRequest) executeWithIteration(zr *ZapiRunner) (*LunGetIterResponse, error) {
	combined := NewLunGetIterResponse()
	combined.Result.SetAttributesList(LunGetIterResponseResultAttributesList{})
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
				combined.Result.SetAttributesList(LunGetIterResponseResultAttributesList{})
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

// LunGetIterRequestDesiredAttributes is a wrapper
type LunGetIterRequestDesiredAttributes struct {
	XMLName    xml.Name     `xml:"desired-attributes"`
	LunInfoPtr *LunInfoType `xml:"lun-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetIterRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// LunInfo is a 'getter' method
func (o *LunGetIterRequestDesiredAttributes) LunInfo() LunInfoType {
	r := *o.LunInfoPtr
	return r
}

// SetLunInfo is a fluent style 'setter' method that can be chained
func (o *LunGetIterRequestDesiredAttributes) SetLunInfo(newValue LunInfoType) *LunGetIterRequestDesiredAttributes {
	o.LunInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *LunGetIterRequest) DesiredAttributes() LunGetIterRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *LunGetIterRequest) SetDesiredAttributes(newValue LunGetIterRequestDesiredAttributes) *LunGetIterRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// MaxRecords is a 'getter' method
func (o *LunGetIterRequest) MaxRecords() int {
	r := *o.MaxRecordsPtr
	return r
}

// SetMaxRecords is a fluent style 'setter' method that can be chained
func (o *LunGetIterRequest) SetMaxRecords(newValue int) *LunGetIterRequest {
	o.MaxRecordsPtr = &newValue
	return o
}

// LunGetIterRequestQuery is a wrapper
type LunGetIterRequestQuery struct {
	XMLName    xml.Name     `xml:"query"`
	LunInfoPtr *LunInfoType `xml:"lun-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetIterRequestQuery) String() string {
	return ToString(reflect.ValueOf(o))
}

// LunInfo is a 'getter' method
func (o *LunGetIterRequestQuery) LunInfo() LunInfoType {
	r := *o.LunInfoPtr
	return r
}

// SetLunInfo is a fluent style 'setter' method that can be chained
func (o *LunGetIterRequestQuery) SetLunInfo(newValue LunInfoType) *LunGetIterRequestQuery {
	o.LunInfoPtr = &newValue
	return o
}

// Query is a 'getter' method
func (o *LunGetIterRequest) Query() LunGetIterRequestQuery {
	r := *o.QueryPtr
	return r
}

// SetQuery is a fluent style 'setter' method that can be chained
func (o *LunGetIterRequest) SetQuery(newValue LunGetIterRequestQuery) *LunGetIterRequest {
	o.QueryPtr = &newValue
	return o
}

// Tag is a 'getter' method
func (o *LunGetIterRequest) Tag() string {
	r := *o.TagPtr
	return r
}

// SetTag is a fluent style 'setter' method that can be chained
func (o *LunGetIterRequest) SetTag(newValue string) *LunGetIterRequest {
	o.TagPtr = &newValue
	return o
}

// LunGetIterResponseResultAttributesList is a wrapper
type LunGetIterResponseResultAttributesList struct {
	XMLName    xml.Name      `xml:"attributes-list"`
	LunInfoPtr []LunInfoType `xml:"lun-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetIterResponseResultAttributesList) String() string {
	return ToString(reflect.ValueOf(o))
}

// LunInfo is a 'getter' method
func (o *LunGetIterResponseResultAttributesList) LunInfo() []LunInfoType {
	r := o.LunInfoPtr
	return r
}

// SetLunInfo is a fluent style 'setter' method that can be chained
func (o *LunGetIterResponseResultAttributesList) SetLunInfo(newValue []LunInfoType) *LunGetIterResponseResultAttributesList {
	newSlice := make([]LunInfoType, len(newValue))
	copy(newSlice, newValue)
	o.LunInfoPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *LunGetIterResponseResultAttributesList) values() []LunInfoType {
	r := o.LunInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *LunGetIterResponseResultAttributesList) setValues(newValue []LunInfoType) *LunGetIterResponseResultAttributesList {
	newSlice := make([]LunInfoType, len(newValue))
	copy(newSlice, newValue)
	o.LunInfoPtr = newSlice
	return o
}

// AttributesList is a 'getter' method
func (o *LunGetIterResponseResult) AttributesList() LunGetIterResponseResultAttributesList {
	r := *o.AttributesListPtr
	return r
}

// SetAttributesList is a fluent style 'setter' method that can be chained
func (o *LunGetIterResponseResult) SetAttributesList(newValue LunGetIterResponseResultAttributesList) *LunGetIterResponseResult {
	o.AttributesListPtr = &newValue
	return o
}

// NextTag is a 'getter' method
func (o *LunGetIterResponseResult) NextTag() string {
	r := *o.NextTagPtr
	return r
}

// SetNextTag is a fluent style 'setter' method that can be chained
func (o *LunGetIterResponseResult) SetNextTag(newValue string) *LunGetIterResponseResult {
	o.NextTagPtr = &newValue
	return o
}

// NumRecords is a 'getter' method
func (o *LunGetIterResponseResult) NumRecords() int {
	r := *o.NumRecordsPtr
	return r
}

// SetNumRecords is a fluent style 'setter' method that can be chained
func (o *LunGetIterResponseResult) SetNumRecords(newValue int) *LunGetIterResponseResult {
	o.NumRecordsPtr = &newValue
	return o
}

// LunGetIterResponseResultVolumeErrors is a wrapper
type LunGetIterResponseResultVolumeErrors struct {
	XMLName        xml.Name          `xml:"volume-errors"`
	VolumeErrorPtr []VolumeErrorType `xml:"volume-error"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunGetIterResponseResultVolumeErrors) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeError is a 'getter' method
func (o *LunGetIterResponseResultVolumeErrors) VolumeError() []VolumeErrorType {
	r := o.VolumeErrorPtr
	return r
}

// SetVolumeError is a fluent style 'setter' method that can be chained
func (o *LunGetIterResponseResultVolumeErrors) SetVolumeError(newValue []VolumeErrorType) *LunGetIterResponseResultVolumeErrors {
	newSlice := make([]VolumeErrorType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeErrorPtr = newSlice
	return o
}

// values is a 'getter' method
func (o *LunGetIterResponseResultVolumeErrors) values() []VolumeErrorType {
	r := o.VolumeErrorPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *LunGetIterResponseResultVolumeErrors) setValues(newValue []VolumeErrorType) *LunGetIterResponseResultVolumeErrors {
	newSlice := make([]VolumeErrorType, len(newValue))
	copy(newSlice, newValue)
	o.VolumeErrorPtr = newSlice
	return o
}

// VolumeErrors is a 'getter' method
func (o *LunGetIterResponseResult) VolumeErrors() LunGetIterResponseResultVolumeErrors {
	r := *o.VolumeErrorsPtr
	return r
}

// SetVolumeErrors is a fluent style 'setter' method that can be chained
func (o *LunGetIterResponseResult) SetVolumeErrors(newValue LunGetIterResponseResultVolumeErrors) *LunGetIterResponseResult {
	o.VolumeErrorsPtr = &newValue
	return o
}
