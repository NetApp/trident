package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorResyncRequest is a structure to represent a snapmirror-resync Request ZAPI object
type SnapmirrorResyncRequest struct {
	XMLName                xml.Name `xml:"snapmirror-resync"`
	DestinationClusterPtr  *string  `xml:"destination-cluster"`
	DestinationLocationPtr *string  `xml:"destination-location"`
	DestinationVolumePtr   *string  `xml:"destination-volume"`
	DestinationVserverPtr  *string  `xml:"destination-vserver"`
	IsAutoExpandEnabledPtr *bool    `xml:"is-auto-expand-enabled"`
	MaxTransferRatePtr     *int     `xml:"max-transfer-rate"`
	PreservePtr            *bool    `xml:"preserve"`
	QuickResyncPtr         *bool    `xml:"quick-resync"`
	SourceClusterPtr       *string  `xml:"source-cluster"`
	SourceLocationPtr      *string  `xml:"source-location"`
	SourceSnapshotPtr      *string  `xml:"source-snapshot"`
	SourceVolumePtr        *string  `xml:"source-volume"`
	SourceVserverPtr       *string  `xml:"source-vserver"`
	TransferPriorityPtr    *string  `xml:"transfer-priority"`
}

// SnapmirrorResyncResponse is a structure to represent a snapmirror-resync Response ZAPI object
type SnapmirrorResyncResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          SnapmirrorResyncResponseResult `xml:"results"`
}

// NewSnapmirrorResyncResponse is a factory method for creating new instances of SnapmirrorResyncResponse objects
func NewSnapmirrorResyncResponse() *SnapmirrorResyncResponse {
	return &SnapmirrorResyncResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorResyncResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorResyncResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorResyncResponseResult is a structure to represent a snapmirror-resync Response Result ZAPI object
type SnapmirrorResyncResponseResult struct {
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

// NewSnapmirrorResyncRequest is a factory method for creating new instances of SnapmirrorResyncRequest objects
func NewSnapmirrorResyncRequest() *SnapmirrorResyncRequest {
	return &SnapmirrorResyncRequest{}
}

// NewSnapmirrorResyncResponseResult is a factory method for creating new instances of SnapmirrorResyncResponseResult objects
func NewSnapmirrorResyncResponseResult() *SnapmirrorResyncResponseResult {
	return &SnapmirrorResyncResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorResyncRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorResyncResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorResyncRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorResyncResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorResyncRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorResyncResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorResyncRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorResyncResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorResyncRequest", NewSnapmirrorResyncResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorResyncResponse), err
}

// DestinationCluster is a 'getter' method
func (o *SnapmirrorResyncRequest) DestinationCluster() string {
	r := *o.DestinationClusterPtr
	return r
}

// SetDestinationCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetDestinationCluster(newValue string) *SnapmirrorResyncRequest {
	o.DestinationClusterPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorResyncRequest) DestinationLocation() string {
	r := *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetDestinationLocation(newValue string) *SnapmirrorResyncRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorResyncRequest) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetDestinationVolume(newValue string) *SnapmirrorResyncRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorResyncRequest) DestinationVserver() string {
	r := *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetDestinationVserver(newValue string) *SnapmirrorResyncRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// IsAutoExpandEnabled is a 'getter' method
func (o *SnapmirrorResyncRequest) IsAutoExpandEnabled() bool {
	r := *o.IsAutoExpandEnabledPtr
	return r
}

// SetIsAutoExpandEnabled is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetIsAutoExpandEnabled(newValue bool) *SnapmirrorResyncRequest {
	o.IsAutoExpandEnabledPtr = &newValue
	return o
}

// MaxTransferRate is a 'getter' method
func (o *SnapmirrorResyncRequest) MaxTransferRate() int {
	r := *o.MaxTransferRatePtr
	return r
}

// SetMaxTransferRate is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetMaxTransferRate(newValue int) *SnapmirrorResyncRequest {
	o.MaxTransferRatePtr = &newValue
	return o
}

// Preserve is a 'getter' method
func (o *SnapmirrorResyncRequest) Preserve() bool {
	r := *o.PreservePtr
	return r
}

// SetPreserve is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetPreserve(newValue bool) *SnapmirrorResyncRequest {
	o.PreservePtr = &newValue
	return o
}

// QuickResync is a 'getter' method
func (o *SnapmirrorResyncRequest) QuickResync() bool {
	r := *o.QuickResyncPtr
	return r
}

// SetQuickResync is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetQuickResync(newValue bool) *SnapmirrorResyncRequest {
	o.QuickResyncPtr = &newValue
	return o
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorResyncRequest) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetSourceCluster(newValue string) *SnapmirrorResyncRequest {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorResyncRequest) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetSourceLocation(newValue string) *SnapmirrorResyncRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceSnapshot is a 'getter' method
func (o *SnapmirrorResyncRequest) SourceSnapshot() string {
	r := *o.SourceSnapshotPtr
	return r
}

// SetSourceSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetSourceSnapshot(newValue string) *SnapmirrorResyncRequest {
	o.SourceSnapshotPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorResyncRequest) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetSourceVolume(newValue string) *SnapmirrorResyncRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorResyncRequest) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetSourceVserver(newValue string) *SnapmirrorResyncRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// TransferPriority is a 'getter' method
func (o *SnapmirrorResyncRequest) TransferPriority() string {
	r := *o.TransferPriorityPtr
	return r
}

// SetTransferPriority is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetTransferPriority(newValue string) *SnapmirrorResyncRequest {
	o.TransferPriorityPtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *SnapmirrorResyncResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncResponseResult) SetResultErrorCode(newValue int) *SnapmirrorResyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *SnapmirrorResyncResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncResponseResult) SetResultErrorMessage(newValue string) *SnapmirrorResyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *SnapmirrorResyncResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncResponseResult) SetResultJobid(newValue int) *SnapmirrorResyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultOperationId is a 'getter' method
func (o *SnapmirrorResyncResponseResult) ResultOperationId() string {
	r := *o.ResultOperationIdPtr
	return r
}

// SetResultOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncResponseResult) SetResultOperationId(newValue string) *SnapmirrorResyncResponseResult {
	o.ResultOperationIdPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *SnapmirrorResyncResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncResponseResult) SetResultStatus(newValue string) *SnapmirrorResyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
