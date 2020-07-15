package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SnapshotRestoreVolumeAsyncRequest is a structure to represent a snapshot-restore-volume-async Request ZAPI object
type SnapshotRestoreVolumeAsyncRequest struct {
	XMLName                 xml.Name  `xml:"snapshot-restore-volume-async"`
	SnapshotPtr             *string   `xml:"snapshot"`
	SnapshotInstanceUuidPtr *UUIDType `xml:"snapshot-instance-uuid"`
	VolumePtr               *string   `xml:"volume"`
}

// SnapshotRestoreVolumeAsyncResponse is a structure to represent a snapshot-restore-volume-async Response ZAPI object
type SnapshotRestoreVolumeAsyncResponse struct {
	XMLName         xml.Name                                 `xml:"netapp"`
	ResponseVersion string                                   `xml:"version,attr"`
	ResponseXmlns   string                                   `xml:"xmlns,attr"`
	Result          SnapshotRestoreVolumeAsyncResponseResult `xml:"results"`
}

// NewSnapshotRestoreVolumeAsyncResponse is a factory method for creating new instances of SnapshotRestoreVolumeAsyncResponse objects
func NewSnapshotRestoreVolumeAsyncResponse() *SnapshotRestoreVolumeAsyncResponse {
	return &SnapshotRestoreVolumeAsyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotRestoreVolumeAsyncResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapshotRestoreVolumeAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapshotRestoreVolumeAsyncResponseResult is a structure to represent a snapshot-restore-volume-async Response Result ZAPI object
type SnapshotRestoreVolumeAsyncResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewSnapshotRestoreVolumeAsyncRequest is a factory method for creating new instances of SnapshotRestoreVolumeAsyncRequest objects
func NewSnapshotRestoreVolumeAsyncRequest() *SnapshotRestoreVolumeAsyncRequest {
	return &SnapshotRestoreVolumeAsyncRequest{}
}

// NewSnapshotRestoreVolumeAsyncResponseResult is a factory method for creating new instances of SnapshotRestoreVolumeAsyncResponseResult objects
func NewSnapshotRestoreVolumeAsyncResponseResult() *SnapshotRestoreVolumeAsyncResponseResult {
	return &SnapshotRestoreVolumeAsyncResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapshotRestoreVolumeAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapshotRestoreVolumeAsyncResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotRestoreVolumeAsyncRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotRestoreVolumeAsyncResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotRestoreVolumeAsyncRequest) ExecuteUsing(zr *ZapiRunner) (*SnapshotRestoreVolumeAsyncResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotRestoreVolumeAsyncRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapshotRestoreVolumeAsyncResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapshotRestoreVolumeAsyncRequest", NewSnapshotRestoreVolumeAsyncResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapshotRestoreVolumeAsyncResponse), err
}

// Snapshot is a 'getter' method
func (o *SnapshotRestoreVolumeAsyncRequest) Snapshot() string {
	r := *o.SnapshotPtr
	return r
}

// SetSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeAsyncRequest) SetSnapshot(newValue string) *SnapshotRestoreVolumeAsyncRequest {
	o.SnapshotPtr = &newValue
	return o
}

// SnapshotInstanceUuid is a 'getter' method
func (o *SnapshotRestoreVolumeAsyncRequest) SnapshotInstanceUuid() UUIDType {
	r := *o.SnapshotInstanceUuidPtr
	return r
}

// SetSnapshotInstanceUuid is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeAsyncRequest) SetSnapshotInstanceUuid(newValue UUIDType) *SnapshotRestoreVolumeAsyncRequest {
	o.SnapshotInstanceUuidPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *SnapshotRestoreVolumeAsyncRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeAsyncRequest) SetVolume(newValue string) *SnapshotRestoreVolumeAsyncRequest {
	o.VolumePtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *SnapshotRestoreVolumeAsyncResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeAsyncResponseResult) SetResultErrorCode(newValue int) *SnapshotRestoreVolumeAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *SnapshotRestoreVolumeAsyncResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeAsyncResponseResult) SetResultErrorMessage(newValue string) *SnapshotRestoreVolumeAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *SnapshotRestoreVolumeAsyncResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeAsyncResponseResult) SetResultJobid(newValue int) *SnapshotRestoreVolumeAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *SnapshotRestoreVolumeAsyncResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *SnapshotRestoreVolumeAsyncResponseResult) SetResultStatus(newValue string) *SnapshotRestoreVolumeAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}

