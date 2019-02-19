package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeDestroyAsyncRequest is a structure to represent a volume-destroy-async Request ZAPI object
type VolumeDestroyAsyncRequest struct {
	XMLName              xml.Name `xml:"volume-destroy-async"`
	UnmountAndOfflinePtr *bool    `xml:"unmount-and-offline"`
	VolumeNamePtr        *string  `xml:"volume-name"`
}

// VolumeDestroyAsyncResponse is a structure to represent a volume-destroy-async Response ZAPI object
type VolumeDestroyAsyncResponse struct {
	XMLName         xml.Name                         `xml:"netapp"`
	ResponseVersion string                           `xml:"version,attr"`
	ResponseXmlns   string                           `xml:"xmlns,attr"`
	Result          VolumeDestroyAsyncResponseResult `xml:"results"`
}

// NewVolumeDestroyAsyncResponse is a factory method for creating new instances of VolumeDestroyAsyncResponse objects
func NewVolumeDestroyAsyncResponse() *VolumeDestroyAsyncResponse {
	return &VolumeDestroyAsyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyAsyncResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeDestroyAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeDestroyAsyncResponseResult is a structure to represent a volume-destroy-async Response Result ZAPI object
type VolumeDestroyAsyncResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewVolumeDestroyAsyncRequest is a factory method for creating new instances of VolumeDestroyAsyncRequest objects
func NewVolumeDestroyAsyncRequest() *VolumeDestroyAsyncRequest {
	return &VolumeDestroyAsyncRequest{}
}

// NewVolumeDestroyAsyncResponseResult is a factory method for creating new instances of VolumeDestroyAsyncResponseResult objects
func NewVolumeDestroyAsyncResponseResult() *VolumeDestroyAsyncResponseResult {
	return &VolumeDestroyAsyncResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeDestroyAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeDestroyAsyncResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyAsyncRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDestroyAsyncResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeDestroyAsyncRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeDestroyAsyncResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeDestroyAsyncRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeDestroyAsyncResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeDestroyAsyncRequest", NewVolumeDestroyAsyncResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeDestroyAsyncResponse), err
}

// UnmountAndOffline is a 'getter' method
func (o *VolumeDestroyAsyncRequest) UnmountAndOffline() bool {
	r := *o.UnmountAndOfflinePtr
	return r
}

// SetUnmountAndOffline is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncRequest) SetUnmountAndOffline(newValue bool) *VolumeDestroyAsyncRequest {
	o.UnmountAndOfflinePtr = &newValue
	return o
}

// VolumeName is a 'getter' method
func (o *VolumeDestroyAsyncRequest) VolumeName() string {
	r := *o.VolumeNamePtr
	return r
}

// SetVolumeName is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncRequest) SetVolumeName(newValue string) *VolumeDestroyAsyncRequest {
	o.VolumeNamePtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *VolumeDestroyAsyncResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncResponseResult) SetResultErrorCode(newValue int) *VolumeDestroyAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *VolumeDestroyAsyncResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncResponseResult) SetResultErrorMessage(newValue string) *VolumeDestroyAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *VolumeDestroyAsyncResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncResponseResult) SetResultJobid(newValue int) *VolumeDestroyAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *VolumeDestroyAsyncResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *VolumeDestroyAsyncResponseResult) SetResultStatus(newValue string) *VolumeDestroyAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
