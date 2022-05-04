// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// QuotaStatusRequest is a structure to represent a quota-status Request ZAPI object
type QuotaStatusRequest struct {
	XMLName   xml.Name `xml:"quota-status"`
	VolumePtr *string  `xml:"volume"`
}

// QuotaStatusResponse is a structure to represent a quota-status Response ZAPI object
type QuotaStatusResponse struct {
	XMLName         xml.Name                  `xml:"netapp"`
	ResponseVersion string                    `xml:"version,attr"`
	ResponseXmlns   string                    `xml:"xmlns,attr"`
	Result          QuotaStatusResponseResult `xml:"results"`
}

// NewQuotaStatusResponse is a factory method for creating new instances of QuotaStatusResponse objects
func NewQuotaStatusResponse() *QuotaStatusResponse {
	return &QuotaStatusResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaStatusResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *QuotaStatusResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// QuotaStatusResponseResult is a structure to represent a quota-status Response Result ZAPI object
type QuotaStatusResponseResult struct {
	XMLName            xml.Name `xml:"results"`
	ResultStatusAttr   string   `xml:"status,attr"`
	ResultReasonAttr   string   `xml:"reason,attr"`
	ResultErrnoAttr    string   `xml:"errno,attr"`
	PercentCompletePtr *int     `xml:"percent-complete"`
	QuotaErrorsPtr     *string  `xml:"quota-errors"`
	ReasonPtr          *string  `xml:"reason"`
	StatusPtr          *string  `xml:"status"`
	SubstatusPtr       *string  `xml:"substatus"`
}

// NewQuotaStatusRequest is a factory method for creating new instances of QuotaStatusRequest objects
func NewQuotaStatusRequest() *QuotaStatusRequest {
	return &QuotaStatusRequest{}
}

// NewQuotaStatusResponseResult is a factory method for creating new instances of QuotaStatusResponseResult objects
func NewQuotaStatusResponseResult() *QuotaStatusResponseResult {
	return &QuotaStatusResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *QuotaStatusRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *QuotaStatusResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaStatusRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaStatusResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QuotaStatusRequest) ExecuteUsing(zr *ZapiRunner) (*QuotaStatusResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QuotaStatusRequest) executeWithoutIteration(zr *ZapiRunner) (*QuotaStatusResponse, error) {
	result, err := zr.ExecuteUsing(o, "QuotaStatusRequest", NewQuotaStatusResponse())
	if result == nil {
		return nil, err
	}
	return result.(*QuotaStatusResponse), err
}

// Volume is a 'getter' method
func (o *QuotaStatusRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *QuotaStatusRequest) SetVolume(newValue string) *QuotaStatusRequest {
	o.VolumePtr = &newValue
	return o
}

// PercentComplete is a 'getter' method
func (o *QuotaStatusResponseResult) PercentComplete() int {
	r := *o.PercentCompletePtr
	return r
}

// SetPercentComplete is a fluent style 'setter' method that can be chained
func (o *QuotaStatusResponseResult) SetPercentComplete(newValue int) *QuotaStatusResponseResult {
	o.PercentCompletePtr = &newValue
	return o
}

// QuotaErrors is a 'getter' method
func (o *QuotaStatusResponseResult) QuotaErrors() string {
	r := *o.QuotaErrorsPtr
	return r
}

// SetQuotaErrors is a fluent style 'setter' method that can be chained
func (o *QuotaStatusResponseResult) SetQuotaErrors(newValue string) *QuotaStatusResponseResult {
	o.QuotaErrorsPtr = &newValue
	return o
}

// Reason is a 'getter' method
func (o *QuotaStatusResponseResult) Reason() string {
	r := *o.ReasonPtr
	return r
}

// SetReason is a fluent style 'setter' method that can be chained
func (o *QuotaStatusResponseResult) SetReason(newValue string) *QuotaStatusResponseResult {
	o.ReasonPtr = &newValue
	return o
}

// Status is a 'getter' method
func (o *QuotaStatusResponseResult) Status() string {
	r := *o.StatusPtr
	return r
}

// SetStatus is a fluent style 'setter' method that can be chained
func (o *QuotaStatusResponseResult) SetStatus(newValue string) *QuotaStatusResponseResult {
	o.StatusPtr = &newValue
	return o
}

// Substatus is a 'getter' method
func (o *QuotaStatusResponseResult) Substatus() string {
	r := *o.SubstatusPtr
	return r
}

// SetSubstatus is a fluent style 'setter' method that can be chained
func (o *QuotaStatusResponseResult) SetSubstatus(newValue string) *QuotaStatusResponseResult {
	o.SubstatusPtr = &newValue
	return o
}
