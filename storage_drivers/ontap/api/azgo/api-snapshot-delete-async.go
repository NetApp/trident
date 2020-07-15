package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SnapshotDeleteAsyncRequest is a structure to represent a snapshot-delete-async Request ZAPI object
type SnapshotDeleteAsyncRequest struct {
	XMLName                 xml.Name  `xml:"snapshot-delete-async"`
	IgnoreOwnersPtr         *bool     `xml:"ignore-owners"`
	SnapshotPtr             *string   `xml:"snapshot"`
	SnapshotInstanceUuidPtr *UUIDType `xml:"snapshot-instance-uuid"`
	VolumePtr               *string   `xml:"volume"`
}

// SnapshotDeleteAsyncResponse is a structure to represent a snapshot-delete-async Response ZAPI object
type SnapshotDeleteAsyncResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          SnapshotDeleteAsyncResponseResult `xml:"results"`
}

// NewSnapshotDeleteAsyncResponse is a factory method for creating new instances of SnapshotDeleteAsyncResponse objects
func NewSnapshotDeleteAsyncResponse() *SnapshotDeleteAsyncResponse {
	return &SnapshotDeleteAsyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotDeleteAsyncResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapshotDeleteAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapshotDeleteAsyncResponseResult is a structure to represent a snapshot-delete-async Response Result ZAPI object
type SnapshotDeleteAsyncResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewSnapshotDeleteAsyncRequest is a factory method for creating new instances of SnapshotDeleteAsyncRequest objects
func NewSnapshotDeleteAsyncRequest() *SnapshotDeleteAsyncRequest {
	return &SnapshotDeleteAsyncRequest{}
}

// NewSnapshotDeleteAsyncResponseResult is a factory method for creating new instances of SnapshotDeleteAsyncResponseResult objects
func NewSnapshotDeleteAsyncResponseResult() *SnapshotDeleteAsyncResponseResult {
	return &SnapshotDeleteAsyncResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapshotDeleteAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapshotDeleteAsyncResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotDeleteAsyncRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotDeleteAsyncResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotDeleteAsyncRequest) ExecuteUsing(zr *ZapiRunner) (*SnapshotDeleteAsyncResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotDeleteAsyncRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapshotDeleteAsyncResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapshotDeleteAsyncRequest", NewSnapshotDeleteAsyncResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapshotDeleteAsyncResponse), err
}

// IgnoreOwners is a 'getter' method
func (o *SnapshotDeleteAsyncRequest) IgnoreOwners() bool {
	r := *o.IgnoreOwnersPtr
	return r
}

// SetIgnoreOwners is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteAsyncRequest) SetIgnoreOwners(newValue bool) *SnapshotDeleteAsyncRequest {
	o.IgnoreOwnersPtr = &newValue
	return o
}

// Snapshot is a 'getter' method
func (o *SnapshotDeleteAsyncRequest) Snapshot() string {
	r := *o.SnapshotPtr
	return r
}

// SetSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteAsyncRequest) SetSnapshot(newValue string) *SnapshotDeleteAsyncRequest {
	o.SnapshotPtr = &newValue
	return o
}

// SnapshotInstanceUuid is a 'getter' method
func (o *SnapshotDeleteAsyncRequest) SnapshotInstanceUuid() UUIDType {
	r := *o.SnapshotInstanceUuidPtr
	return r
}

// SetSnapshotInstanceUuid is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteAsyncRequest) SetSnapshotInstanceUuid(newValue UUIDType) *SnapshotDeleteAsyncRequest {
	o.SnapshotInstanceUuidPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *SnapshotDeleteAsyncRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteAsyncRequest) SetVolume(newValue string) *SnapshotDeleteAsyncRequest {
	o.VolumePtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *SnapshotDeleteAsyncResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteAsyncResponseResult) SetResultErrorCode(newValue int) *SnapshotDeleteAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *SnapshotDeleteAsyncResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteAsyncResponseResult) SetResultErrorMessage(newValue string) *SnapshotDeleteAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *SnapshotDeleteAsyncResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteAsyncResponseResult) SetResultJobid(newValue int) *SnapshotDeleteAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *SnapshotDeleteAsyncResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *SnapshotDeleteAsyncResponseResult) SetResultStatus(newValue string) *SnapshotDeleteAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}

