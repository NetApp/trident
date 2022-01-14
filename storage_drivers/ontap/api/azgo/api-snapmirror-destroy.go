package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorDestroyRequest is a structure to represent a snapmirror-destroy Request ZAPI object
type SnapmirrorDestroyRequest struct {
	XMLName                xml.Name `xml:"snapmirror-destroy"`
	DestinationClusterPtr  *string  `xml:"destination-cluster"`
	DestinationLocationPtr *string  `xml:"destination-location"`
	DestinationVolumePtr   *string  `xml:"destination-volume"`
	DestinationVserverPtr  *string  `xml:"destination-vserver"`
	SourceClusterPtr       *string  `xml:"source-cluster"`
	SourceLocationPtr      *string  `xml:"source-location"`
	SourceVolumePtr        *string  `xml:"source-volume"`
	SourceVserverPtr       *string  `xml:"source-vserver"`
}

// SnapmirrorDestroyResponse is a structure to represent a snapmirror-destroy Response ZAPI object
type SnapmirrorDestroyResponse struct {
	XMLName         xml.Name                        `xml:"netapp"`
	ResponseVersion string                          `xml:"version,attr"`
	ResponseXmlns   string                          `xml:"xmlns,attr"`
	Result          SnapmirrorDestroyResponseResult `xml:"results"`
}

// NewSnapmirrorDestroyResponse is a factory method for creating new instances of SnapmirrorDestroyResponse objects
func NewSnapmirrorDestroyResponse() *SnapmirrorDestroyResponse {
	return &SnapmirrorDestroyResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorDestroyResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorDestroyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorDestroyResponseResult is a structure to represent a snapmirror-destroy Response Result ZAPI object
type SnapmirrorDestroyResponseResult struct {
	XMLName              xml.Name  `xml:"results"`
	ResultStatusAttr     string    `xml:"status,attr"`
	ResultReasonAttr     string    `xml:"reason,attr"`
	ResultErrnoAttr      string    `xml:"errno,attr"`
	ResultOperationIdPtr *UuidType `xml:"result-operation-id"`
}

// NewSnapmirrorDestroyRequest is a factory method for creating new instances of SnapmirrorDestroyRequest objects
func NewSnapmirrorDestroyRequest() *SnapmirrorDestroyRequest {
	return &SnapmirrorDestroyRequest{}
}

// NewSnapmirrorDestroyResponseResult is a factory method for creating new instances of SnapmirrorDestroyResponseResult objects
func NewSnapmirrorDestroyResponseResult() *SnapmirrorDestroyResponseResult {
	return &SnapmirrorDestroyResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorDestroyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorDestroyResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorDestroyRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorDestroyResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorDestroyRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorDestroyResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorDestroyRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorDestroyResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorDestroyRequest", NewSnapmirrorDestroyResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorDestroyResponse), err
}

// DestinationCluster is a 'getter' method
func (o *SnapmirrorDestroyRequest) DestinationCluster() string {
	r := *o.DestinationClusterPtr
	return r
}

// SetDestinationCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestroyRequest) SetDestinationCluster(newValue string) *SnapmirrorDestroyRequest {
	o.DestinationClusterPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorDestroyRequest) DestinationLocation() string {
	r := *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestroyRequest) SetDestinationLocation(newValue string) *SnapmirrorDestroyRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorDestroyRequest) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestroyRequest) SetDestinationVolume(newValue string) *SnapmirrorDestroyRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorDestroyRequest) DestinationVserver() string {
	r := *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestroyRequest) SetDestinationVserver(newValue string) *SnapmirrorDestroyRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorDestroyRequest) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestroyRequest) SetSourceCluster(newValue string) *SnapmirrorDestroyRequest {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorDestroyRequest) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestroyRequest) SetSourceLocation(newValue string) *SnapmirrorDestroyRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorDestroyRequest) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestroyRequest) SetSourceVolume(newValue string) *SnapmirrorDestroyRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorDestroyRequest) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestroyRequest) SetSourceVserver(newValue string) *SnapmirrorDestroyRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// ResultOperationId is a 'getter' method
func (o *SnapmirrorDestroyResponseResult) ResultOperationId() UuidType {
	r := *o.ResultOperationIdPtr
	return r
}

// SetResultOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorDestroyResponseResult) SetResultOperationId(newValue UuidType) *SnapmirrorDestroyResponseResult {
	o.ResultOperationIdPtr = &newValue
	return o
}
