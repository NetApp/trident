package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// QuotaResizeRequest is a structure to represent a quota-resize Request ZAPI object
type QuotaResizeRequest struct {
	XMLName   xml.Name `xml:"quota-resize"`
	VolumePtr *string  `xml:"volume"`
}

// QuotaResizeResponse is a structure to represent a quota-resize Response ZAPI object
type QuotaResizeResponse struct {
	XMLName         xml.Name                  `xml:"netapp"`
	ResponseVersion string                    `xml:"version,attr"`
	ResponseXmlns   string                    `xml:"xmlns,attr"`
	Result          QuotaResizeResponseResult `xml:"results"`
}

// NewQuotaResizeResponse is a factory method for creating new instances of QuotaResizeResponse objects
func NewQuotaResizeResponse() *QuotaResizeResponse {
	return &QuotaResizeResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaResizeResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *QuotaResizeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// QuotaResizeResponseResult is a structure to represent a quota-resize Response Result ZAPI object
type QuotaResizeResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewQuotaResizeRequest is a factory method for creating new instances of QuotaResizeRequest objects
func NewQuotaResizeRequest() *QuotaResizeRequest {
	return &QuotaResizeRequest{}
}

// NewQuotaResizeResponseResult is a factory method for creating new instances of QuotaResizeResponseResult objects
func NewQuotaResizeResponseResult() *QuotaResizeResponseResult {
	return &QuotaResizeResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *QuotaResizeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *QuotaResizeResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaResizeRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QuotaResizeResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QuotaResizeRequest) ExecuteUsing(zr *ZapiRunner) (*QuotaResizeResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QuotaResizeRequest) executeWithoutIteration(zr *ZapiRunner) (*QuotaResizeResponse, error) {
	result, err := zr.ExecuteUsing(o, "QuotaResizeRequest", NewQuotaResizeResponse())
	if result == nil {
		return nil, err
	}
	return result.(*QuotaResizeResponse), err
}

// Volume is a 'getter' method
func (o *QuotaResizeRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *QuotaResizeRequest) SetVolume(newValue string) *QuotaResizeRequest {
	o.VolumePtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *QuotaResizeResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *QuotaResizeResponseResult) SetResultErrorCode(newValue int) *QuotaResizeResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *QuotaResizeResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *QuotaResizeResponseResult) SetResultErrorMessage(newValue string) *QuotaResizeResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *QuotaResizeResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *QuotaResizeResponseResult) SetResultJobid(newValue int) *QuotaResizeResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *QuotaResizeResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *QuotaResizeResponseResult) SetResultStatus(newValue string) *QuotaResizeResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
