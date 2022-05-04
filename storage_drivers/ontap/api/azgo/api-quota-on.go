// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// QuotaOnRequest is a structure to represent a quota-on Request ZAPI object
type QuotaOnRequest struct {
	XMLName   xml.Name `xml:"quota-on"`
	VolumePtr *string  `xml:"volume"`
}

// QuotaOnResponse is a structure to represent a quota-on Response ZAPI object
type QuotaOnResponse struct {
	XMLName         xml.Name              `xml:"netapp"`
	ResponseVersion string                `xml:"version,attr"`
	ResponseXmlns   string                `xml:"xmlns,attr"`
	Result          QuotaOnResponseResult `xml:"results"`
}

// NewQuotaOnResponse is a factory method for creating new instances of QuotaOnResponse objects
func NewQuotaOnResponse() *QuotaOnResponse {
	return &QuotaOnResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaOnResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *QuotaOnResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// QuotaOnResponseResult is a structure to represent a quota-on Response Result ZAPI object
type QuotaOnResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewQuotaOnRequest is a factory method for creating new instances of QuotaOnRequest objects
func NewQuotaOnRequest() *QuotaOnRequest {
	return &QuotaOnRequest{}
}

// NewQuotaOnResponseResult is a factory method for creating new instances of QuotaOnResponseResult objects
func NewQuotaOnResponseResult() *QuotaOnResponseResult {
	return &QuotaOnResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *QuotaOnRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *QuotaOnResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaOnRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaOnResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QuotaOnRequest) ExecuteUsing(zr *ZapiRunner) (*QuotaOnResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QuotaOnRequest) executeWithoutIteration(zr *ZapiRunner) (*QuotaOnResponse, error) {
	result, err := zr.ExecuteUsing(o, "QuotaOnRequest", NewQuotaOnResponse())
	if result == nil {
		return nil, err
	}
	return result.(*QuotaOnResponse), err
}

// Volume is a 'getter' method
func (o *QuotaOnRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *QuotaOnRequest) SetVolume(newValue string) *QuotaOnRequest {
	o.VolumePtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *QuotaOnResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *QuotaOnResponseResult) SetResultErrorCode(newValue int) *QuotaOnResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *QuotaOnResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *QuotaOnResponseResult) SetResultErrorMessage(newValue string) *QuotaOnResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *QuotaOnResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *QuotaOnResponseResult) SetResultJobid(newValue int) *QuotaOnResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *QuotaOnResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *QuotaOnResponseResult) SetResultStatus(newValue string) *QuotaOnResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
