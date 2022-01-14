package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorInitializeRequest is a structure to represent a snapmirror-initialize Request ZAPI object
type SnapmirrorInitializeRequest struct {
	XMLName                xml.Name `xml:"snapmirror-initialize"`
	DestinationClusterPtr  *string  `xml:"destination-cluster"`
	DestinationLocationPtr *string  `xml:"destination-location"`
	DestinationVolumePtr   *string  `xml:"destination-volume"`
	DestinationVserverPtr  *string  `xml:"destination-vserver"`
	IsAutoExpandEnabledPtr *bool    `xml:"is-auto-expand-enabled"`
	MaxTransferRatePtr     *int     `xml:"max-transfer-rate"`
	SourceClusterPtr       *string  `xml:"source-cluster"`
	SourceLocationPtr      *string  `xml:"source-location"`
	SourceSnapshotPtr      *string  `xml:"source-snapshot"`
	SourceVolumePtr        *string  `xml:"source-volume"`
	SourceVserverPtr       *string  `xml:"source-vserver"`
	TransferPriorityPtr    *string  `xml:"transfer-priority"`
}

// SnapmirrorInitializeResponse is a structure to represent a snapmirror-initialize Response ZAPI object
type SnapmirrorInitializeResponse struct {
	XMLName         xml.Name                           `xml:"netapp"`
	ResponseVersion string                             `xml:"version,attr"`
	ResponseXmlns   string                             `xml:"xmlns,attr"`
	Result          SnapmirrorInitializeResponseResult `xml:"results"`
}

// NewSnapmirrorInitializeResponse is a factory method for creating new instances of SnapmirrorInitializeResponse objects
func NewSnapmirrorInitializeResponse() *SnapmirrorInitializeResponse {
	return &SnapmirrorInitializeResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorInitializeResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorInitializeResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorInitializeResponseResult is a structure to represent a snapmirror-initialize Response Result ZAPI object
type SnapmirrorInitializeResponseResult struct {
	XMLName               xml.Name `xml:"results"`
	ResultStatusAttr      string   `xml:"status,attr"`
	ResultReasonAttr      string   `xml:"reason,attr"`
	ResultErrnoAttr       string   `xml:"errno,attr"`
	ResultErrorCodePtr    *int     `xml:"result-error-code"`
	ResultErrorMessagePtr *string  `xml:"result-error-message"`
	ResultJobidPtr        *int     `xml:"result-jobid"`
	ResultOperationIdPtr  *string  `xml:"result-operation-id"`
	ResultStatusPtr       *string  `xml:"result-status"`
}

// NewSnapmirrorInitializeRequest is a factory method for creating new instances of SnapmirrorInitializeRequest objects
func NewSnapmirrorInitializeRequest() *SnapmirrorInitializeRequest {
	return &SnapmirrorInitializeRequest{}
}

// NewSnapmirrorInitializeResponseResult is a factory method for creating new instances of SnapmirrorInitializeResponseResult objects
func NewSnapmirrorInitializeResponseResult() *SnapmirrorInitializeResponseResult {
	return &SnapmirrorInitializeResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorInitializeRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorInitializeResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorInitializeRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorInitializeResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorInitializeRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorInitializeResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorInitializeRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorInitializeResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorInitializeRequest", NewSnapmirrorInitializeResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorInitializeResponse), err
}

// DestinationCluster is a 'getter' method
func (o *SnapmirrorInitializeRequest) DestinationCluster() string {
	r := *o.DestinationClusterPtr
	return r
}

// SetDestinationCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetDestinationCluster(newValue string) *SnapmirrorInitializeRequest {
	o.DestinationClusterPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorInitializeRequest) DestinationLocation() string {
	r := *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetDestinationLocation(newValue string) *SnapmirrorInitializeRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorInitializeRequest) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetDestinationVolume(newValue string) *SnapmirrorInitializeRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorInitializeRequest) DestinationVserver() string {
	r := *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetDestinationVserver(newValue string) *SnapmirrorInitializeRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// IsAutoExpandEnabled is a 'getter' method
func (o *SnapmirrorInitializeRequest) IsAutoExpandEnabled() bool {
	r := *o.IsAutoExpandEnabledPtr
	return r
}

// SetIsAutoExpandEnabled is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetIsAutoExpandEnabled(newValue bool) *SnapmirrorInitializeRequest {
	o.IsAutoExpandEnabledPtr = &newValue
	return o
}

// MaxTransferRate is a 'getter' method
func (o *SnapmirrorInitializeRequest) MaxTransferRate() int {
	r := *o.MaxTransferRatePtr
	return r
}

// SetMaxTransferRate is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetMaxTransferRate(newValue int) *SnapmirrorInitializeRequest {
	o.MaxTransferRatePtr = &newValue
	return o
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorInitializeRequest) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetSourceCluster(newValue string) *SnapmirrorInitializeRequest {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorInitializeRequest) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetSourceLocation(newValue string) *SnapmirrorInitializeRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceSnapshot is a 'getter' method
func (o *SnapmirrorInitializeRequest) SourceSnapshot() string {
	r := *o.SourceSnapshotPtr
	return r
}

// SetSourceSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetSourceSnapshot(newValue string) *SnapmirrorInitializeRequest {
	o.SourceSnapshotPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorInitializeRequest) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetSourceVolume(newValue string) *SnapmirrorInitializeRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorInitializeRequest) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetSourceVserver(newValue string) *SnapmirrorInitializeRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// TransferPriority is a 'getter' method
func (o *SnapmirrorInitializeRequest) TransferPriority() string {
	r := *o.TransferPriorityPtr
	return r
}

// SetTransferPriority is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeRequest) SetTransferPriority(newValue string) *SnapmirrorInitializeRequest {
	o.TransferPriorityPtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *SnapmirrorInitializeResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeResponseResult) SetResultErrorCode(newValue int) *SnapmirrorInitializeResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *SnapmirrorInitializeResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeResponseResult) SetResultErrorMessage(newValue string) *SnapmirrorInitializeResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *SnapmirrorInitializeResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeResponseResult) SetResultJobid(newValue int) *SnapmirrorInitializeResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultOperationId is a 'getter' method
func (o *SnapmirrorInitializeResponseResult) ResultOperationId() string {
	r := *o.ResultOperationIdPtr
	return r
}

// SetResultOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeResponseResult) SetResultOperationId(newValue string) *SnapmirrorInitializeResponseResult {
	o.ResultOperationIdPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *SnapmirrorInitializeResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInitializeResponseResult) SetResultStatus(newValue string) *SnapmirrorInitializeResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
