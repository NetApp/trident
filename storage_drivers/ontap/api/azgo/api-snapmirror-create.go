package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorCreateRequest is a structure to represent a snapmirror-create Request ZAPI object
type SnapmirrorCreateRequest struct {
	XMLName                xml.Name `xml:"snapmirror-create"`
	DestinationClusterPtr  *string  `xml:"destination-cluster"`
	DestinationLocationPtr *string  `xml:"destination-location"`
	DestinationVolumePtr   *string  `xml:"destination-volume"`
	DestinationVserverPtr  *string  `xml:"destination-vserver"`
	IdentityPreservePtr    *bool    `xml:"identity-preserve"`
	IsAutoExpandEnabledPtr *bool    `xml:"is-auto-expand-enabled"`
	MaxTransferRatePtr     *int     `xml:"max-transfer-rate"`
	PolicyPtr              *string  `xml:"policy"`
	RelationshipTypePtr    *string  `xml:"relationship-type"`
	ReturnRecordPtr        *bool    `xml:"return-record"`
	SchedulePtr            *string  `xml:"schedule"`
	SourceClusterPtr       *string  `xml:"source-cluster"`
	SourceLocationPtr      *string  `xml:"source-location"`
	SourceVolumePtr        *string  `xml:"source-volume"`
	SourceVserverPtr       *string  `xml:"source-vserver"`
	TriesPtr               *string  `xml:"tries"`
	VserverPtr             *string  `xml:"vserver"`
}

// SnapmirrorCreateResponse is a structure to represent a snapmirror-create Response ZAPI object
type SnapmirrorCreateResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          SnapmirrorCreateResponseResult `xml:"results"`
}

// NewSnapmirrorCreateResponse is a factory method for creating new instances of SnapmirrorCreateResponse objects
func NewSnapmirrorCreateResponse() *SnapmirrorCreateResponse {
	return &SnapmirrorCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorCreateResponseResult is a structure to represent a snapmirror-create Response Result ZAPI object
type SnapmirrorCreateResponseResult struct {
	XMLName          xml.Name                              `xml:"results"`
	ResultStatusAttr string                                `xml:"status,attr"`
	ResultReasonAttr string                                `xml:"reason,attr"`
	ResultErrnoAttr  string                                `xml:"errno,attr"`
	ResultPtr        *SnapmirrorCreateResponseResultResult `xml:"result"`
}

// NewSnapmirrorCreateRequest is a factory method for creating new instances of SnapmirrorCreateRequest objects
func NewSnapmirrorCreateRequest() *SnapmirrorCreateRequest {
	return &SnapmirrorCreateRequest{}
}

// NewSnapmirrorCreateResponseResult is a factory method for creating new instances of SnapmirrorCreateResponseResult objects
func NewSnapmirrorCreateResponseResult() *SnapmirrorCreateResponseResult {
	return &SnapmirrorCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorCreateRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorCreateRequest", NewSnapmirrorCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorCreateResponse), err
}

// DestinationCluster is a 'getter' method
func (o *SnapmirrorCreateRequest) DestinationCluster() string {
	r := *o.DestinationClusterPtr
	return r
}

// SetDestinationCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetDestinationCluster(newValue string) *SnapmirrorCreateRequest {
	o.DestinationClusterPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorCreateRequest) DestinationLocation() string {
	r := *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetDestinationLocation(newValue string) *SnapmirrorCreateRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorCreateRequest) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetDestinationVolume(newValue string) *SnapmirrorCreateRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorCreateRequest) DestinationVserver() string {
	r := *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetDestinationVserver(newValue string) *SnapmirrorCreateRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// IdentityPreserve is a 'getter' method
func (o *SnapmirrorCreateRequest) IdentityPreserve() bool {
	r := *o.IdentityPreservePtr
	return r
}

// SetIdentityPreserve is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetIdentityPreserve(newValue bool) *SnapmirrorCreateRequest {
	o.IdentityPreservePtr = &newValue
	return o
}

// IsAutoExpandEnabled is a 'getter' method
func (o *SnapmirrorCreateRequest) IsAutoExpandEnabled() bool {
	r := *o.IsAutoExpandEnabledPtr
	return r
}

// SetIsAutoExpandEnabled is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetIsAutoExpandEnabled(newValue bool) *SnapmirrorCreateRequest {
	o.IsAutoExpandEnabledPtr = &newValue
	return o
}

// MaxTransferRate is a 'getter' method
func (o *SnapmirrorCreateRequest) MaxTransferRate() int {
	r := *o.MaxTransferRatePtr
	return r
}

// SetMaxTransferRate is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetMaxTransferRate(newValue int) *SnapmirrorCreateRequest {
	o.MaxTransferRatePtr = &newValue
	return o
}

// Policy is a 'getter' method
func (o *SnapmirrorCreateRequest) Policy() string {
	r := *o.PolicyPtr
	return r
}

// SetPolicy is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetPolicy(newValue string) *SnapmirrorCreateRequest {
	o.PolicyPtr = &newValue
	return o
}

// RelationshipType is a 'getter' method
func (o *SnapmirrorCreateRequest) RelationshipType() string {
	r := *o.RelationshipTypePtr
	return r
}

// SetRelationshipType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetRelationshipType(newValue string) *SnapmirrorCreateRequest {
	o.RelationshipTypePtr = &newValue
	return o
}

// ReturnRecord is a 'getter' method
func (o *SnapmirrorCreateRequest) ReturnRecord() bool {
	r := *o.ReturnRecordPtr
	return r
}

// SetReturnRecord is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetReturnRecord(newValue bool) *SnapmirrorCreateRequest {
	o.ReturnRecordPtr = &newValue
	return o
}

// Schedule is a 'getter' method
func (o *SnapmirrorCreateRequest) Schedule() string {
	r := *o.SchedulePtr
	return r
}

// SetSchedule is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetSchedule(newValue string) *SnapmirrorCreateRequest {
	o.SchedulePtr = &newValue
	return o
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorCreateRequest) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetSourceCluster(newValue string) *SnapmirrorCreateRequest {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorCreateRequest) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetSourceLocation(newValue string) *SnapmirrorCreateRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorCreateRequest) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetSourceVolume(newValue string) *SnapmirrorCreateRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorCreateRequest) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetSourceVserver(newValue string) *SnapmirrorCreateRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// Tries is a 'getter' method
func (o *SnapmirrorCreateRequest) Tries() string {
	r := *o.TriesPtr
	return r
}

// SetTries is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetTries(newValue string) *SnapmirrorCreateRequest {
	o.TriesPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *SnapmirrorCreateRequest) Vserver() string {
	r := *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateRequest) SetVserver(newValue string) *SnapmirrorCreateRequest {
	o.VserverPtr = &newValue
	return o
}

// SnapmirrorCreateResponseResultResult is a wrapper
type SnapmirrorCreateResponseResultResult struct {
	XMLName           xml.Name            `xml:"result"`
	SnapmirrorInfoPtr *SnapmirrorInfoType `xml:"snapmirror-info"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorCreateResponseResultResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// SnapmirrorInfo is a 'getter' method
func (o *SnapmirrorCreateResponseResultResult) SnapmirrorInfo() SnapmirrorInfoType {
	r := *o.SnapmirrorInfoPtr
	return r
}

// SetSnapmirrorInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateResponseResultResult) SetSnapmirrorInfo(newValue SnapmirrorInfoType) *SnapmirrorCreateResponseResultResult {
	o.SnapmirrorInfoPtr = &newValue
	return o
}

// values is a 'getter' method
func (o *SnapmirrorCreateResponseResultResult) values() SnapmirrorInfoType {
	r := *o.SnapmirrorInfoPtr
	return r
}

// setValues is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateResponseResultResult) setValues(newValue SnapmirrorInfoType) *SnapmirrorCreateResponseResultResult {
	o.SnapmirrorInfoPtr = &newValue
	return o
}

// Result is a 'getter' method
func (o *SnapmirrorCreateResponseResult) Result() SnapmirrorCreateResponseResultResult {
	r := *o.ResultPtr
	return r
}

// SetResult is a fluent style 'setter' method that can be chained
func (o *SnapmirrorCreateResponseResult) SetResult(newValue SnapmirrorCreateResponseResultResult) *SnapmirrorCreateResponseResult {
	o.ResultPtr = &newValue
	return o
}
