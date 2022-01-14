package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorBreakRequest is a structure to represent a snapmirror-break Request ZAPI object
type SnapmirrorBreakRequest struct {
	XMLName                         xml.Name `xml:"snapmirror-break"`
	DestinationClusterPtr           *string  `xml:"destination-cluster"`
	DestinationLocationPtr          *string  `xml:"destination-location"`
	DestinationVolumePtr            *string  `xml:"destination-volume"`
	DestinationVserverPtr           *string  `xml:"destination-vserver"`
	RecoverPtr                      *bool    `xml:"recover"`
	RestoreDestinationToSnapshotPtr *string  `xml:"restore-destination-to-snapshot"`
	SourceClusterPtr                *string  `xml:"source-cluster"`
	SourceLocationPtr               *string  `xml:"source-location"`
	SourceVolumePtr                 *string  `xml:"source-volume"`
	SourceVserverPtr                *string  `xml:"source-vserver"`
}

// SnapmirrorBreakResponse is a structure to represent a snapmirror-break Response ZAPI object
type SnapmirrorBreakResponse struct {
	XMLName         xml.Name                      `xml:"netapp"`
	ResponseVersion string                        `xml:"version,attr"`
	ResponseXmlns   string                        `xml:"xmlns,attr"`
	Result          SnapmirrorBreakResponseResult `xml:"results"`
}

// NewSnapmirrorBreakResponse is a factory method for creating new instances of SnapmirrorBreakResponse objects
func NewSnapmirrorBreakResponse() *SnapmirrorBreakResponse {
	return &SnapmirrorBreakResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorBreakResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorBreakResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorBreakResponseResult is a structure to represent a snapmirror-break Response Result ZAPI object
type SnapmirrorBreakResponseResult struct {
	XMLName              xml.Name `xml:"results"`
	ResultStatusAttr     string   `xml:"status,attr"`
	ResultReasonAttr     string   `xml:"reason,attr"`
	ResultErrnoAttr      string   `xml:"errno,attr"`
	ResultOperationIdPtr *string  `xml:"result-operation-id"`
}

// NewSnapmirrorBreakRequest is a factory method for creating new instances of SnapmirrorBreakRequest objects
func NewSnapmirrorBreakRequest() *SnapmirrorBreakRequest {
	return &SnapmirrorBreakRequest{}
}

// NewSnapmirrorBreakResponseResult is a factory method for creating new instances of SnapmirrorBreakResponseResult objects
func NewSnapmirrorBreakResponseResult() *SnapmirrorBreakResponseResult {
	return &SnapmirrorBreakResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorBreakRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorBreakResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorBreakRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorBreakResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorBreakRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorBreakResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorBreakRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorBreakResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorBreakRequest", NewSnapmirrorBreakResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorBreakResponse), err
}

// DestinationCluster is a 'getter' method
func (o *SnapmirrorBreakRequest) DestinationCluster() string {
	r := *o.DestinationClusterPtr
	return r
}

// SetDestinationCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorBreakRequest) SetDestinationCluster(newValue string) *SnapmirrorBreakRequest {
	o.DestinationClusterPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorBreakRequest) DestinationLocation() string {
	r := *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorBreakRequest) SetDestinationLocation(newValue string) *SnapmirrorBreakRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorBreakRequest) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorBreakRequest) SetDestinationVolume(newValue string) *SnapmirrorBreakRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorBreakRequest) DestinationVserver() string {
	r := *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorBreakRequest) SetDestinationVserver(newValue string) *SnapmirrorBreakRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// Recover is a 'getter' method
func (o *SnapmirrorBreakRequest) Recover() bool {
	r := *o.RecoverPtr
	return r
}

// SetRecover is a fluent style 'setter' method that can be chained
func (o *SnapmirrorBreakRequest) SetRecover(newValue bool) *SnapmirrorBreakRequest {
	o.RecoverPtr = &newValue
	return o
}

// RestoreDestinationToSnapshot is a 'getter' method
func (o *SnapmirrorBreakRequest) RestoreDestinationToSnapshot() string {
	r := *o.RestoreDestinationToSnapshotPtr
	return r
}

// SetRestoreDestinationToSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorBreakRequest) SetRestoreDestinationToSnapshot(newValue string) *SnapmirrorBreakRequest {
	o.RestoreDestinationToSnapshotPtr = &newValue
	return o
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorBreakRequest) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorBreakRequest) SetSourceCluster(newValue string) *SnapmirrorBreakRequest {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorBreakRequest) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorBreakRequest) SetSourceLocation(newValue string) *SnapmirrorBreakRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorBreakRequest) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorBreakRequest) SetSourceVolume(newValue string) *SnapmirrorBreakRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorBreakRequest) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorBreakRequest) SetSourceVserver(newValue string) *SnapmirrorBreakRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// ResultOperationId is a 'getter' method
func (o *SnapmirrorBreakResponseResult) ResultOperationId() string {
	r := *o.ResultOperationIdPtr
	return r
}

// SetResultOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorBreakResponseResult) SetResultOperationId(newValue string) *SnapmirrorBreakResponseResult {
	o.ResultOperationIdPtr = &newValue
	return o
}
