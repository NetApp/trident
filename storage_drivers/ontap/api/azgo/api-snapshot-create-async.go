package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SnapshotCreateAsyncRequest is a structure to represent a snapshot-create-async Request ZAPI object
type SnapshotCreateAsyncRequest struct {
	XMLName     xml.Name `xml:"snapshot-create-async"`
	CommentPtr  *string  `xml:"comment"`
	SnapshotPtr *string  `xml:"snapshot"`
	VolumePtr   *string  `xml:"volume"`
}

// SnapshotCreateAsyncResponse is a structure to represent a snapshot-create-async Response ZAPI object
type SnapshotCreateAsyncResponse struct {
	XMLName         xml.Name                          `xml:"netapp"`
	ResponseVersion string                            `xml:"version,attr"`
	ResponseXmlns   string                            `xml:"xmlns,attr"`
	Result          SnapshotCreateAsyncResponseResult `xml:"results"`
}

// NewSnapshotCreateAsyncResponse is a factory method for creating new instances of SnapshotCreateAsyncResponse objects
func NewSnapshotCreateAsyncResponse() *SnapshotCreateAsyncResponse {
	return &SnapshotCreateAsyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotCreateAsyncResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapshotCreateAsyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapshotCreateAsyncResponseResult is a structure to represent a snapshot-create-async Response Result ZAPI object
type SnapshotCreateAsyncResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewSnapshotCreateAsyncRequest is a factory method for creating new instances of SnapshotCreateAsyncRequest objects
func NewSnapshotCreateAsyncRequest() *SnapshotCreateAsyncRequest {
	return &SnapshotCreateAsyncRequest{}
}

// NewSnapshotCreateAsyncResponseResult is a factory method for creating new instances of SnapshotCreateAsyncResponseResult objects
func NewSnapshotCreateAsyncResponseResult() *SnapshotCreateAsyncResponseResult {
	return &SnapshotCreateAsyncResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapshotCreateAsyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapshotCreateAsyncResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotCreateAsyncRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotCreateAsyncResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotCreateAsyncRequest) ExecuteUsing(zr *ZapiRunner) (*SnapshotCreateAsyncResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapshotCreateAsyncRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapshotCreateAsyncResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapshotCreateAsyncRequest", NewSnapshotCreateAsyncResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapshotCreateAsyncResponse), err
}

// Comment is a 'getter' method
func (o *SnapshotCreateAsyncRequest) Comment() string {
	r := *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateAsyncRequest) SetComment(newValue string) *SnapshotCreateAsyncRequest {
	o.CommentPtr = &newValue
	return o
}

// Snapshot is a 'getter' method
func (o *SnapshotCreateAsyncRequest) Snapshot() string {
	r := *o.SnapshotPtr
	return r
}

// SetSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateAsyncRequest) SetSnapshot(newValue string) *SnapshotCreateAsyncRequest {
	o.SnapshotPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *SnapshotCreateAsyncRequest) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateAsyncRequest) SetVolume(newValue string) *SnapshotCreateAsyncRequest {
	o.VolumePtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *SnapshotCreateAsyncResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateAsyncResponseResult) SetResultErrorCode(newValue int) *SnapshotCreateAsyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *SnapshotCreateAsyncResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateAsyncResponseResult) SetResultErrorMessage(newValue string) *SnapshotCreateAsyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *SnapshotCreateAsyncResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateAsyncResponseResult) SetResultJobid(newValue int) *SnapshotCreateAsyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *SnapshotCreateAsyncResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *SnapshotCreateAsyncResponseResult) SetResultStatus(newValue string) *SnapshotCreateAsyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}

