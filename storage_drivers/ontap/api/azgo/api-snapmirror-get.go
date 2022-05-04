// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorGetRequest is a structure to represent a snapmirror-get Request ZAPI object
type SnapmirrorGetRequest struct {
	XMLName                xml.Name                               `xml:"snapmirror-get"`
	DesiredAttributesPtr   *SnapmirrorGetRequestDesiredAttributes `xml:"desired-attributes"`
	DestinationClusterPtr  *string                                `xml:"destination-cluster"`
	DestinationLocationPtr *string                                `xml:"destination-location"`
	DestinationVolumePtr   *string                                `xml:"destination-volume"`
	DestinationVserverPtr  *string                                `xml:"destination-vserver"`
	SourceClusterPtr       *string                                `xml:"source-cluster"`
	SourceLocationPtr      *string                                `xml:"source-location"`
	SourceVolumePtr        *string                                `xml:"source-volume"`
	SourceVserverPtr       *string                                `xml:"source-vserver"`
}

// SnapmirrorGetResponse is a structure to represent a snapmirror-get Response ZAPI object
type SnapmirrorGetResponse struct {
	XMLName         xml.Name                    `xml:"netapp"`
	ResponseVersion string                      `xml:"version,attr"`
	ResponseXmlns   string                      `xml:"xmlns,attr"`
	Result          SnapmirrorGetResponseResult `xml:"results"`
}

// NewSnapmirrorGetResponse is a factory method for creating new instances of SnapmirrorGetResponse objects
func NewSnapmirrorGetResponse() *SnapmirrorGetResponse {
	return &SnapmirrorGetResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorGetResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorGetResponseResult is a structure to represent a snapmirror-get Response Result ZAPI object
type SnapmirrorGetResponseResult struct {
	XMLName          xml.Name                               `xml:"results"`
	ResultStatusAttr string                                 `xml:"status,attr"`
	ResultReasonAttr string                                 `xml:"reason,attr"`
	ResultErrnoAttr  string                                 `xml:"errno,attr"`
	AttributesPtr    *SnapmirrorGetResponseResultAttributes `xml:"attributes"`
}

// NewSnapmirrorGetRequest is a factory method for creating new instances of SnapmirrorGetRequest objects
func NewSnapmirrorGetRequest() *SnapmirrorGetRequest {
	return &SnapmirrorGetRequest{}
}

// NewSnapmirrorGetResponseResult is a factory method for creating new instances of SnapmirrorGetResponseResult objects
func NewSnapmirrorGetResponseResult() *SnapmirrorGetResponseResult {
	return &SnapmirrorGetResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorGetRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorGetResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorGetRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorGetResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorGetRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorGetResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorGetRequest", NewSnapmirrorGetResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorGetResponse), err
}

// SnapmirrorGetRequestDesiredAttributes is a wrapper
type SnapmirrorGetRequestDesiredAttributes struct {
	XMLName           xml.Name            `xml:"desired-attributes"`
	SnapmirrorInfoPtr *SnapmirrorInfoType `xml:"snapmirror-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetRequestDesiredAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorInfo is a 'getter' method
func (o *SnapmirrorGetRequestDesiredAttributes) SnapmirrorInfo() SnapmirrorInfoType {
	r := *o.SnapmirrorInfoPtr
	return r
}

// SetSnapmirrorInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetRequestDesiredAttributes) SetSnapmirrorInfo(newValue SnapmirrorInfoType) *SnapmirrorGetRequestDesiredAttributes {
	o.SnapmirrorInfoPtr = &newValue
	return o
}

// DesiredAttributes is a 'getter' method
func (o *SnapmirrorGetRequest) DesiredAttributes() SnapmirrorGetRequestDesiredAttributes {
	r := *o.DesiredAttributesPtr
	return r
}

// SetDesiredAttributes is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetRequest) SetDesiredAttributes(newValue SnapmirrorGetRequestDesiredAttributes) *SnapmirrorGetRequest {
	o.DesiredAttributesPtr = &newValue
	return o
}

// DestinationCluster is a 'getter' method
func (o *SnapmirrorGetRequest) DestinationCluster() string {
	r := *o.DestinationClusterPtr
	return r
}

// SetDestinationCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetRequest) SetDestinationCluster(newValue string) *SnapmirrorGetRequest {
	o.DestinationClusterPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorGetRequest) DestinationLocation() string {
	r := *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetRequest) SetDestinationLocation(newValue string) *SnapmirrorGetRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorGetRequest) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetRequest) SetDestinationVolume(newValue string) *SnapmirrorGetRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorGetRequest) DestinationVserver() string {
	r := *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetRequest) SetDestinationVserver(newValue string) *SnapmirrorGetRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorGetRequest) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetRequest) SetSourceCluster(newValue string) *SnapmirrorGetRequest {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorGetRequest) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetRequest) SetSourceLocation(newValue string) *SnapmirrorGetRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorGetRequest) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetRequest) SetSourceVolume(newValue string) *SnapmirrorGetRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorGetRequest) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetRequest) SetSourceVserver(newValue string) *SnapmirrorGetRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// SnapmirrorGetResponseResultAttributes is a wrapper
type SnapmirrorGetResponseResultAttributes struct {
	XMLName           xml.Name            `xml:"attributes"`
	SnapmirrorInfoPtr *SnapmirrorInfoType `xml:"snapmirror-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorGetResponseResultAttributes) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorInfo is a 'getter' method
func (o *SnapmirrorGetResponseResultAttributes) SnapmirrorInfo() SnapmirrorInfoType {
	r := *o.SnapmirrorInfoPtr
	return r
}

// SetSnapmirrorInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetResponseResultAttributes) SetSnapmirrorInfo(newValue SnapmirrorInfoType) *SnapmirrorGetResponseResultAttributes {
	o.SnapmirrorInfoPtr = &newValue
	return o
}

// values is a 'getter' method
func (o *SnapmirrorGetResponseResultAttributes) values() SnapmirrorInfoType {
	r := *o.SnapmirrorInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetResponseResultAttributes) setValues(newValue SnapmirrorInfoType) *SnapmirrorGetResponseResultAttributes {
	o.SnapmirrorInfoPtr = &newValue
	return o
}

// Attributes is a 'getter' method
func (o *SnapmirrorGetResponseResult) Attributes() SnapmirrorGetResponseResultAttributes {
	r := *o.AttributesPtr
	return r
}

// SetAttributes is a fluent style 'setter' method that can be chained
func (o *SnapmirrorGetResponseResult) SetAttributes(newValue SnapmirrorGetResponseResultAttributes) *SnapmirrorGetResponseResult {
	o.AttributesPtr = &newValue
	return o
}
