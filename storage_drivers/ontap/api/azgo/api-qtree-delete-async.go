package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// QtreeDeleteAsyncRequest is a structure to represent a qtree-delete-async Request ZAPI object
type QtreeDeleteAsyncRequest struct {
	XMLName  xml.Name `xml:"qtree-delete-async"`
	ForcePtr *bool    `xml:"force"`
	QtreePtr *string  `xml:"qtree"`
}

// QtreeDeleteAsyncResponse is a structure to represent a qtree-delete-async Response ZAPI object
type QtreeDeleteAsyncResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          QtreeDeleteAsyncResponseResult `xml:"results"`
}

// NewQtreeDeleteAsyncResponse is a factory method for creating new instances of QtreeDeleteAsyncResponse objects
func NewQtreeDeleteAsyncResponse() *QtreeDeleteAsyncResponse {
	return &QtreeDeleteAsyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeDeleteAsyncResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *QtreeDeleteAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// QtreeDeleteAsyncResponseResult is a structure to represent a qtree-delete-async Response Result ZAPI object
type QtreeDeleteAsyncResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewQtreeDeleteAsyncRequest is a factory method for creating new instances of QtreeDeleteAsyncRequest objects
func NewQtreeDeleteAsyncRequest() *QtreeDeleteAsyncRequest {
	return &QtreeDeleteAsyncRequest{}
}

// NewQtreeDeleteAsyncResponseResult is a factory method for creating new instances of QtreeDeleteAsyncResponseResult objects
func NewQtreeDeleteAsyncResponseResult() *QtreeDeleteAsyncResponseResult {
	return &QtreeDeleteAsyncResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *QtreeDeleteAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *QtreeDeleteAsyncResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeDeleteAsyncRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeDeleteAsyncResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QtreeDeleteAsyncRequest) ExecuteUsing(zr *ZapiRunner) (*QtreeDeleteAsyncResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *QtreeDeleteAsyncRequest) executeWithoutIteration(zr *ZapiRunner) (*QtreeDeleteAsyncResponse, error) {
	result, err := zr.ExecuteUsing(o, "QtreeDeleteAsyncRequest", NewQtreeDeleteAsyncResponse())
	if result == nil {
		return nil, err
	}
	return result.(*QtreeDeleteAsyncResponse), err
}

// Force is a 'getter' method
func (o *QtreeDeleteAsyncRequest) Force() bool {
	r := *o.ForcePtr
	return r
}

// SetForce is a fluent style 'setter' method that can be chained
func (o *QtreeDeleteAsyncRequest) SetForce(newValue bool) *QtreeDeleteAsyncRequest {
	o.ForcePtr = &newValue
	return o
}

// Qtree is a 'getter' method
func (o *QtreeDeleteAsyncRequest) Qtree() string {
	r := *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *QtreeDeleteAsyncRequest) SetQtree(newValue string) *QtreeDeleteAsyncRequest {
	o.QtreePtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *QtreeDeleteAsyncResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *QtreeDeleteAsyncResponseResult) SetResultErrorCode(newValue int) *QtreeDeleteAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *QtreeDeleteAsyncResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *QtreeDeleteAsyncResponseResult) SetResultErrorMessage(newValue string) *QtreeDeleteAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *QtreeDeleteAsyncResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *QtreeDeleteAsyncResponseResult) SetResultJobid(newValue int) *QtreeDeleteAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *QtreeDeleteAsyncResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *QtreeDeleteAsyncResponseResult) SetResultStatus(newValue string) *QtreeDeleteAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
