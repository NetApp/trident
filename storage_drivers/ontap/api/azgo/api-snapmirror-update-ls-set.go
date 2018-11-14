package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorUpdateLsSetRequest is a structure to represent a snapmirror-update-ls-set Request ZAPI object
type SnapmirrorUpdateLsSetRequest struct {
	XMLName           xml.Name `xml:"snapmirror-update-ls-set"`
	SourceClusterPtr  *string  `xml:"source-cluster"`
	SourceLocationPtr *string  `xml:"source-location"`
	SourceVolumePtr   *string  `xml:"source-volume"`
	SourceVserverPtr  *string  `xml:"source-vserver"`
}

// SnapmirrorUpdateLsSetResponse is a structure to represent a snapmirror-update-ls-set Response ZAPI object
type SnapmirrorUpdateLsSetResponse struct {
	XMLName         xml.Name                            `xml:"netapp"`
	ResponseVersion string                              `xml:"version,attr"`
	ResponseXmlns   string                              `xml:"xmlns,attr"`
	Result          SnapmirrorUpdateLsSetResponseResult `xml:"results"`
}

// NewSnapmirrorUpdateLsSetResponse is a factory method for creating new instances of SnapmirrorUpdateLsSetResponse objects
func NewSnapmirrorUpdateLsSetResponse() *SnapmirrorUpdateLsSetResponse {
	return &SnapmirrorUpdateLsSetResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorUpdateLsSetResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorUpdateLsSetResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorUpdateLsSetResponseResult is a structure to represent a snapmirror-update-ls-set Response Result ZAPI object
type SnapmirrorUpdateLsSetResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewSnapmirrorUpdateLsSetRequest is a factory method for creating new instances of SnapmirrorUpdateLsSetRequest objects
func NewSnapmirrorUpdateLsSetRequest() *SnapmirrorUpdateLsSetRequest {
	return &SnapmirrorUpdateLsSetRequest{}
}

// NewSnapmirrorUpdateLsSetResponseResult is a factory method for creating new instances of SnapmirrorUpdateLsSetResponseResult objects
func NewSnapmirrorUpdateLsSetResponseResult() *SnapmirrorUpdateLsSetResponseResult {
	return &SnapmirrorUpdateLsSetResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorUpdateLsSetRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorUpdateLsSetResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorUpdateLsSetRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorUpdateLsSetResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SnapmirrorUpdateLsSetRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorUpdateLsSetResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *SnapmirrorUpdateLsSetRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorUpdateLsSetResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorUpdateLsSetRequest", NewSnapmirrorUpdateLsSetResponse())
	return result.(*SnapmirrorUpdateLsSetResponse), err
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorUpdateLsSetRequest) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SetSourceCluster(newValue string) *SnapmirrorUpdateLsSetRequest {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorUpdateLsSetRequest) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SetSourceLocation(newValue string) *SnapmirrorUpdateLsSetRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorUpdateLsSetRequest) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SetSourceVolume(newValue string) *SnapmirrorUpdateLsSetRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorUpdateLsSetRequest) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetRequest) SetSourceVserver(newValue string) *SnapmirrorUpdateLsSetRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *SnapmirrorUpdateLsSetResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) SetResultErrorCode(newValue int) *SnapmirrorUpdateLsSetResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *SnapmirrorUpdateLsSetResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) SetResultErrorMessage(newValue string) *SnapmirrorUpdateLsSetResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *SnapmirrorUpdateLsSetResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) SetResultJobid(newValue int) *SnapmirrorUpdateLsSetResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *SnapmirrorUpdateLsSetResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateLsSetResponseResult) SetResultStatus(newValue string) *SnapmirrorUpdateLsSetResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
