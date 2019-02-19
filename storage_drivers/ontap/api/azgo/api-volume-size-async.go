package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeSizeAsyncRequest is a structure to represent a volume-size-async Request ZAPI object
type VolumeSizeAsyncRequest struct {
	XMLName       xml.Name `xml:"volume-size-async"`
	NewSizePtr    *string  `xml:"new-size"`
	VolumeNamePtr *string  `xml:"volume-name"`
}

// VolumeSizeAsyncResponse is a structure to represent a volume-size-async Response ZAPI object
type VolumeSizeAsyncResponse struct {
	XMLName         xml.Name                      `xml:"netapp"`
	ResponseVersion string                        `xml:"version,attr"`
	ResponseXmlns   string                        `xml:"xmlns,attr"`
	Result          VolumeSizeAsyncResponseResult `xml:"results"`
}

// NewVolumeSizeAsyncResponse is a factory method for creating new instances of VolumeSizeAsyncResponse objects
func NewVolumeSizeAsyncResponse() *VolumeSizeAsyncResponse {
	return &VolumeSizeAsyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeAsyncResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeSizeAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeSizeAsyncResponseResult is a structure to represent a volume-size-async Response Result ZAPI object
type VolumeSizeAsyncResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
	VolumeSizePtr         *string  `xml:"volume-size"`
}

// NewVolumeSizeAsyncRequest is a factory method for creating new instances of VolumeSizeAsyncRequest objects
func NewVolumeSizeAsyncRequest() *VolumeSizeAsyncRequest {
	return &VolumeSizeAsyncRequest{}
}

// NewVolumeSizeAsyncResponseResult is a factory method for creating new instances of VolumeSizeAsyncResponseResult objects
func NewVolumeSizeAsyncResponseResult() *VolumeSizeAsyncResponseResult {
	return &VolumeSizeAsyncResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeSizeAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeSizeAsyncResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeAsyncRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSizeAsyncResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeSizeAsyncRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeSizeAsyncResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeSizeAsyncRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeSizeAsyncResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeSizeAsyncRequest", NewVolumeSizeAsyncResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeSizeAsyncResponse), err
}

// NewSize is a 'getter' method
func (o *VolumeSizeAsyncRequest) NewSize() string {
	r := *o.NewSizePtr
	return r
}

// SetNewSize is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncRequest) SetNewSize(newValue string) *VolumeSizeAsyncRequest {
	o.NewSizePtr = &newValue
	return o
}

// VolumeName is a 'getter' method
func (o *VolumeSizeAsyncRequest) VolumeName() string {
	r := *o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncRequest) SetVolumeName(newValue string) *VolumeSizeAsyncRequest {
	o.VolumeNamePtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *VolumeSizeAsyncResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) SetResultErrorCode(newValue int) *VolumeSizeAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *VolumeSizeAsyncResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) SetResultErrorMessage(newValue string) *VolumeSizeAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *VolumeSizeAsyncResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) SetResultJobid(newValue int) *VolumeSizeAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *VolumeSizeAsyncResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) SetResultStatus(newValue string) *VolumeSizeAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}

// VolumeSize is a 'getter' method
func (o *VolumeSizeAsyncResponseResult) VolumeSize() string {
	r := *o.VolumeSizePtr
	return r
}

// SetVolumeSize is a fluent style 'setter' method that can be chained
func (o *VolumeSizeAsyncResponseResult) SetVolumeSize(newValue string) *VolumeSizeAsyncResponseResult {
	o.VolumeSizePtr = &newValue
	return o
}
