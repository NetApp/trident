// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorQuiesceRequest is a structure to represent a snapmirror-quiesce Request ZAPI object
type SnapmirrorQuiesceRequest struct {
	XMLName                xml.Name `xml:"snapmirror-quiesce"`
	DestinationClusterPtr  *string  `xml:"destination-cluster"`
	DestinationLocationPtr *string  `xml:"destination-location"`
	DestinationVolumePtr   *string  `xml:"destination-volume"`
	DestinationVserverPtr  *string  `xml:"destination-vserver"`
	SourceClusterPtr       *string  `xml:"source-cluster"`
	SourceLocationPtr      *string  `xml:"source-location"`
	SourceVolumePtr        *string  `xml:"source-volume"`
	SourceVserverPtr       *string  `xml:"source-vserver"`
}

// SnapmirrorQuiesceResponse is a structure to represent a snapmirror-quiesce Response ZAPI object
type SnapmirrorQuiesceResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          SnapmirrorQuiesceResponseResult `xml:"results"`
}

// NewSnapmirrorQuiesceResponse is a factory method for creating new instances of SnapmirrorQuiesceResponse objects
func NewSnapmirrorQuiesceResponse() *SnapmirrorQuiesceResponse {
	return &SnapmirrorQuiesceResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorQuiesceResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorQuiesceResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorQuiesceResponseResult is a structure to represent a snapmirror-quiesce Response Result ZAPI object
type SnapmirrorQuiesceResponseResult struct {
	XMLName              xml.Name `xml:"results"`
	ResultStatusAttr     string   `xml:"status,attr"`
	ResultReasonAttr     string   `xml:"reason,attr"`
	ResultErrnoAttr      string   `xml:"errno,attr"`
	ResultOperationIdPtr *string  `xml:"result-operation-id"`
}

// NewSnapmirrorQuiesceRequest is a factory method for creating new instances of SnapmirrorQuiesceRequest objects
func NewSnapmirrorQuiesceRequest() *SnapmirrorQuiesceRequest {
	return &SnapmirrorQuiesceRequest{}
}

// NewSnapmirrorQuiesceResponseResult is a factory method for creating new instances of SnapmirrorQuiesceResponseResult objects
func NewSnapmirrorQuiesceResponseResult() *SnapmirrorQuiesceResponseResult {
	return &SnapmirrorQuiesceResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorQuiesceRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorQuiesceResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorQuiesceRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorQuiesceResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorQuiesceRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorQuiesceResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorQuiesceRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorQuiesceResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorQuiesceRequest", NewSnapmirrorQuiesceResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorQuiesceResponse), err
}

// DestinationCluster is a 'getter' method
func (o *SnapmirrorQuiesceRequest) DestinationCluster() string {
	r := *o.DestinationClusterPtr
	return r
}

// SetDestinationCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetDestinationCluster(newValue string) *SnapmirrorQuiesceRequest {
	o.DestinationClusterPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorQuiesceRequest) DestinationLocation() string {
	r := *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetDestinationLocation(newValue string) *SnapmirrorQuiesceRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorQuiesceRequest) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetDestinationVolume(newValue string) *SnapmirrorQuiesceRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorQuiesceRequest) DestinationVserver() string {
	r := *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetDestinationVserver(newValue string) *SnapmirrorQuiesceRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorQuiesceRequest) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetSourceCluster(newValue string) *SnapmirrorQuiesceRequest {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorQuiesceRequest) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetSourceLocation(newValue string) *SnapmirrorQuiesceRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorQuiesceRequest) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetSourceVolume(newValue string) *SnapmirrorQuiesceRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorQuiesceRequest) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceRequest) SetSourceVserver(newValue string) *SnapmirrorQuiesceRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// ResultOperationId is a 'getter' method
func (o *SnapmirrorQuiesceResponseResult) ResultOperationId() string {
	r := *o.ResultOperationIdPtr
	return r
}

// SetResultOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorQuiesceResponseResult) SetResultOperationId(newValue string) *SnapmirrorQuiesceResponseResult {
	o.ResultOperationIdPtr = &newValue
	return o
}
