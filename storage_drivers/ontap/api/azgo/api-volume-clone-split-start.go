package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeCloneSplitStartRequest is a structure to represent a volume-clone-split-start Request ZAPI object
type VolumeCloneSplitStartRequest struct {
	XMLName   xml.Name `xml:"volume-clone-split-start"`
	VolumePtr *string  `xml:"volume"`
}

// VolumeCloneSplitStartResponse is a structure to represent a volume-clone-split-start Response ZAPI object
type VolumeCloneSplitStartResponse struct {
	XMLName         xml.Name                            `xml:"netapp"`
	ResponseVersion string                              `xml:"version,attr"`
	ResponseXmlns   string                              `xml:"xmlns,attr"`
	Result          VolumeCloneSplitStartResponseResult `xml:"results"`
}

// NewVolumeCloneSplitStartResponse is a factory method for creating new instances of VolumeCloneSplitStartResponse objects
func NewVolumeCloneSplitStartResponse() *VolumeCloneSplitStartResponse {
	return &VolumeCloneSplitStartResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneSplitStartResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneSplitStartResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeCloneSplitStartResponseResult is a structure to represent a volume-clone-split-start Response Result ZAPI object
type VolumeCloneSplitStartResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewVolumeCloneSplitStartRequest is a factory method for creating new instances of VolumeCloneSplitStartRequest objects
func NewVolumeCloneSplitStartRequest() *VolumeCloneSplitStartRequest {
	return &VolumeCloneSplitStartRequest{}
}

// NewVolumeCloneSplitStartResponseResult is a factory method for creating new instances of VolumeCloneSplitStartResponseResult objects
func NewVolumeCloneSplitStartResponseResult() *VolumeCloneSplitStartResponseResult {
	return &VolumeCloneSplitStartResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneSplitStartRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneSplitStartResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneSplitStartRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneSplitStartResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeCloneSplitStartRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeCloneSplitStartResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeCloneSplitStartRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeCloneSplitStartResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeCloneSplitStartRequest", NewVolumeCloneSplitStartResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeCloneSplitStartResponse), err
}

// Volume is a 'getter' method
func (o *VolumeCloneSplitStartRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeCloneSplitStartRequest) SetVolume(newValue string) *VolumeCloneSplitStartRequest {
	o.VolumePtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *VolumeCloneSplitStartResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) SetResultErrorCode(newValue int) *VolumeCloneSplitStartResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *VolumeCloneSplitStartResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) SetResultErrorMessage(newValue string) *VolumeCloneSplitStartResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *VolumeCloneSplitStartResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) SetResultJobid(newValue int) *VolumeCloneSplitStartResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *VolumeCloneSplitStartResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *VolumeCloneSplitStartResponseResult) SetResultStatus(newValue string) *VolumeCloneSplitStartResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
