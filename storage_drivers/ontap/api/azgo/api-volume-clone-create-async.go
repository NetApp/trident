// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeCloneCreateAsyncRequest is a structure to represent a volume-clone-create-async Request ZAPI object
type VolumeCloneCreateAsyncRequest struct {
	XMLName                  xml.Name `xml:"volume-clone-create-async"`
	ParentSnapshotPtr        *string  `xml:"parent-snapshot"`
	ParentVolumePtr          *string  `xml:"parent-volume"`
	ParentVserverPtr         *string  `xml:"parent-vserver"`
	SpaceReservePtr          *string  `xml:"space-reserve"`
	UseSnaprestoreLicensePtr *bool    `xml:"use-snaprestore-license"`
	VolumePtr                *string  `xml:"volume"`
	VserverPtr               *string  `xml:"vserver"`
}

// VolumeCloneCreateAsyncResponse is a structure to represent a volume-clone-create-async Response ZAPI object
type VolumeCloneCreateAsyncResponse struct {
	XMLName         xml.Name                             `xml:"netapp"`
	ResponseVersion string                               `xml:"version,attr"`
	ResponseXmlns   string                               `xml:"xmlns,attr"`
	Result          VolumeCloneCreateAsyncResponseResult `xml:"results"`
}

// NewVolumeCloneCreateAsyncResponse is a factory method for creating new instances of VolumeCloneCreateAsyncResponse objects
func NewVolumeCloneCreateAsyncResponse() *VolumeCloneCreateAsyncResponse {
	return &VolumeCloneCreateAsyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateAsyncResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneCreateAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// VolumeCloneCreateAsyncResponseResult is a structure to represent a volume-clone-create-async Response Result ZAPI object
type VolumeCloneCreateAsyncResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewVolumeCloneCreateAsyncRequest is a factory method for creating new instances of VolumeCloneCreateAsyncRequest objects
func NewVolumeCloneCreateAsyncRequest() *VolumeCloneCreateAsyncRequest {
	return &VolumeCloneCreateAsyncRequest{}
}

// NewVolumeCloneCreateAsyncResponseResult is a factory method for creating new instances of VolumeCloneCreateAsyncResponseResult objects
func NewVolumeCloneCreateAsyncResponseResult() *VolumeCloneCreateAsyncResponseResult {
	return &VolumeCloneCreateAsyncResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneCreateAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *VolumeCloneCreateAsyncResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateAsyncRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeCloneCreateAsyncResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeCloneCreateAsyncRequest) ExecuteUsing(zr *ZapiRunner) (*VolumeCloneCreateAsyncResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *VolumeCloneCreateAsyncRequest) executeWithoutIteration(zr *ZapiRunner) (*VolumeCloneCreateAsyncResponse, error) {
	result, err := zr.ExecuteUsing(o, "VolumeCloneCreateAsyncRequest", NewVolumeCloneCreateAsyncResponse())
	if result == nil {
		return nil, err
	}
	return result.(*VolumeCloneCreateAsyncResponse), err
}

// ParentSnapshot is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) ParentSnapshot() string {
	r := *o.ParentSnapshotPtr
	return r
}

// SetParentSnapshot is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetParentSnapshot(newValue string) *VolumeCloneCreateAsyncRequest {
	o.ParentSnapshotPtr = &newValue
	return o
}

// ParentVolume is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) ParentVolume() string {
	r := *o.ParentVolumePtr
	return r
}

// SetParentVolume is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetParentVolume(newValue string) *VolumeCloneCreateAsyncRequest {
	o.ParentVolumePtr = &newValue
	return o
}

// ParentVserver is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) ParentVserver() string {
	r := *o.ParentVserverPtr
	return r
}

// SetParentVserver is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetParentVserver(newValue string) *VolumeCloneCreateAsyncRequest {
	o.ParentVserverPtr = &newValue
	return o
}

// SpaceReserve is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) SpaceReserve() string {
	r := *o.SpaceReservePtr
	return r
}

// SetSpaceReserve is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetSpaceReserve(newValue string) *VolumeCloneCreateAsyncRequest {
	o.SpaceReservePtr = &newValue
	return o
}

// UseSnaprestoreLicense is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) UseSnaprestoreLicense() bool {
	r := *o.UseSnaprestoreLicensePtr
	return r
}

// SetUseSnaprestoreLicense is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetUseSnaprestoreLicense(newValue bool) *VolumeCloneCreateAsyncRequest {
	o.UseSnaprestoreLicensePtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetVolume(newValue string) *VolumeCloneCreateAsyncRequest {
	o.VolumePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *VolumeCloneCreateAsyncRequest) Vserver() string {
	r := *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncRequest) SetVserver(newValue string) *VolumeCloneCreateAsyncRequest {
	o.VserverPtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *VolumeCloneCreateAsyncResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncResponseResult) SetResultErrorCode(newValue int) *VolumeCloneCreateAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *VolumeCloneCreateAsyncResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncResponseResult) SetResultErrorMessage(newValue string) *VolumeCloneCreateAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *VolumeCloneCreateAsyncResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncResponseResult) SetResultJobid(newValue int) *VolumeCloneCreateAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *VolumeCloneCreateAsyncResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *VolumeCloneCreateAsyncResponseResult) SetResultStatus(newValue string) *VolumeCloneCreateAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
