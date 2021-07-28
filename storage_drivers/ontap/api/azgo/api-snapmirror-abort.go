package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SnapmirrorAbortRequest is a structure to represent a snapmirror-abort Request ZAPI object
type SnapmirrorAbortRequest struct {
	XMLName                xml.Name `xml:"snapmirror-abort"`
	CheckOnlyPtr           *bool    `xml:"check-only"`
	ClearCheckpointPtr     *bool    `xml:"clear-checkpoint"`
	DestinationClusterPtr  *string  `xml:"destination-cluster"`
	DestinationLocationPtr *string  `xml:"destination-location"`
	DestinationVolumePtr   *string  `xml:"destination-volume"`
	DestinationVserverPtr  *string  `xml:"destination-vserver"`
	SourceClusterPtr       *string  `xml:"source-cluster"`
	SourceLocationPtr      *string  `xml:"source-location"`
	SourceVolumePtr        *string  `xml:"source-volume"`
	SourceVserverPtr       *string  `xml:"source-vserver"`
}

// SnapmirrorAbortResponse is a structure to represent a snapmirror-abort Response ZAPI object
type SnapmirrorAbortResponse struct {
	XMLName         xml.Name                      `xml:"netapp"`
	ResponseVersion string                        `xml:"version,attr"`
	ResponseXmlns   string                        `xml:"xmlns,attr"`
	Result          SnapmirrorAbortResponseResult `xml:"results"`
}

// NewSnapmirrorAbortResponse is a factory method for creating new instances of SnapmirrorAbortResponse objects
func NewSnapmirrorAbortResponse() *SnapmirrorAbortResponse {
	return &SnapmirrorAbortResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorAbortResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorAbortResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorAbortResponseResult is a structure to represent a snapmirror-abort Response Result ZAPI object
type SnapmirrorAbortResponseResult struct {
	XMLName              xml.Name `xml:"results"`
	ResultStatusAttr     string   `xml:"status,attr"`
	ResultReasonAttr     string   `xml:"reason,attr"`
	ResultErrnoAttr      string   `xml:"errno,attr"`
	ResultOperationIdPtr *string  `xml:"result-operation-id"`
}

// NewSnapmirrorAbortRequest is a factory method for creating new instances of SnapmirrorAbortRequest objects
func NewSnapmirrorAbortRequest() *SnapmirrorAbortRequest {
	return &SnapmirrorAbortRequest{}
}

// NewSnapmirrorAbortResponseResult is a factory method for creating new instances of SnapmirrorAbortResponseResult objects
func NewSnapmirrorAbortResponseResult() *SnapmirrorAbortResponseResult {
	return &SnapmirrorAbortResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorAbortRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorAbortResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorAbortRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorAbortResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorAbortRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorAbortResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorAbortRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorAbortResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorAbortRequest", NewSnapmirrorAbortResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorAbortResponse), err
}

// CheckOnly is a 'getter' method
func (o *SnapmirrorAbortRequest) CheckOnly() bool {
	r := *o.CheckOnlyPtr
	return r
}

// SetCheckOnly is a fluent style 'setter' method that can be chained
func (o *SnapmirrorAbortRequest) SetCheckOnly(newValue bool) *SnapmirrorAbortRequest {
	o.CheckOnlyPtr = &newValue
	return o
}

// ClearCheckpoint is a 'getter' method
func (o *SnapmirrorAbortRequest) ClearCheckpoint() bool {
	r := *o.ClearCheckpointPtr
	return r
}

// SetClearCheckpoint is a fluent style 'setter' method that can be chained
func (o *SnapmirrorAbortRequest) SetClearCheckpoint(newValue bool) *SnapmirrorAbortRequest {
	o.ClearCheckpointPtr = &newValue
	return o
}

// DestinationCluster is a 'getter' method
func (o *SnapmirrorAbortRequest) DestinationCluster() string {
	r := *o.DestinationClusterPtr
	return r
}

// SetDestinationCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorAbortRequest) SetDestinationCluster(newValue string) *SnapmirrorAbortRequest {
	o.DestinationClusterPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorAbortRequest) DestinationLocation() string {
	r := *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorAbortRequest) SetDestinationLocation(newValue string) *SnapmirrorAbortRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorAbortRequest) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorAbortRequest) SetDestinationVolume(newValue string) *SnapmirrorAbortRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorAbortRequest) DestinationVserver() string {
	r := *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorAbortRequest) SetDestinationVserver(newValue string) *SnapmirrorAbortRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorAbortRequest) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorAbortRequest) SetSourceCluster(newValue string) *SnapmirrorAbortRequest {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorAbortRequest) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorAbortRequest) SetSourceLocation(newValue string) *SnapmirrorAbortRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorAbortRequest) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorAbortRequest) SetSourceVolume(newValue string) *SnapmirrorAbortRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorAbortRequest) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorAbortRequest) SetSourceVserver(newValue string) *SnapmirrorAbortRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// ResultOperationId is a 'getter' method
func (o *SnapmirrorAbortResponseResult) ResultOperationId() string {
	r := *o.ResultOperationIdPtr
	return r
}

// SetResultOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorAbortResponseResult) SetResultOperationId(newValue string) *SnapmirrorAbortResponseResult {
	o.ResultOperationIdPtr = &newValue
	return o
}
