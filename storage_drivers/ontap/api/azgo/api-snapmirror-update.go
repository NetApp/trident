package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorUpdateRequest is a structure to represent a snapmirror-update Request ZAPI object
type SnapmirrorUpdateRequest struct {
	XMLName                    xml.Name `xml:"snapmirror-update"`
	DestinationClusterPtr      *string  `xml:"destination-cluster"`
	DestinationLocationPtr     *string  `xml:"destination-location"`
	DestinationVolumePtr       *string  `xml:"destination-volume"`
	DestinationVserverPtr      *string  `xml:"destination-vserver"`
	EnableStorageEfficiencyPtr *bool    `xml:"enable-storage-efficiency"`
	MaxTransferRatePtr         *int     `xml:"max-transfer-rate"`
	SourceClusterPtr           *string  `xml:"source-cluster"`
	SourceLocationPtr          *string  `xml:"source-location"`
	SourceSnapshotPtr          *string  `xml:"source-snapshot"`
	SourceVolumePtr            *string  `xml:"source-volume"`
	SourceVserverPtr           *string  `xml:"source-vserver"`
	TransferPriorityPtr        *string  `xml:"transfer-priority"`
}

// SnapmirrorUpdateResponse is a structure to represent a snapmirror-update Response ZAPI object
type SnapmirrorUpdateResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          SnapmirrorUpdateResponseResult `xml:"results"`
}

// NewSnapmirrorUpdateResponse is a factory method for creating new instances of SnapmirrorUpdateResponse objects
func NewSnapmirrorUpdateResponse() *SnapmirrorUpdateResponse {
	return &SnapmirrorUpdateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorUpdateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorUpdateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// SnapmirrorUpdateResponseResult is a structure to represent a snapmirror-update Response Result ZAPI object
type SnapmirrorUpdateResponseResult struct {
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

// NewSnapmirrorUpdateRequest is a factory method for creating new instances of SnapmirrorUpdateRequest objects
func NewSnapmirrorUpdateRequest() *SnapmirrorUpdateRequest {
	return &SnapmirrorUpdateRequest{}
}

// NewSnapmirrorUpdateResponseResult is a factory method for creating new instances of SnapmirrorUpdateResponseResult objects
func NewSnapmirrorUpdateResponseResult() *SnapmirrorUpdateResponseResult {
	return &SnapmirrorUpdateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorUpdateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorUpdateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorUpdateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorUpdateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorUpdateRequest) ExecuteUsing(zr *ZapiRunner) (*SnapmirrorUpdateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *SnapmirrorUpdateRequest) executeWithoutIteration(zr *ZapiRunner) (*SnapmirrorUpdateResponse, error) {
	result, err := zr.ExecuteUsing(o, "SnapmirrorUpdateRequest", NewSnapmirrorUpdateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*SnapmirrorUpdateResponse), err
}

// DestinationCluster is a 'getter' method
func (o *SnapmirrorUpdateRequest) DestinationCluster() string {
	r := *o.DestinationClusterPtr
	return r
}

// SetDestinationCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetDestinationCluster(newValue string) *SnapmirrorUpdateRequest {
	o.DestinationClusterPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorUpdateRequest) DestinationLocation() string {
	r := *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetDestinationLocation(newValue string) *SnapmirrorUpdateRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorUpdateRequest) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetDestinationVolume(newValue string) *SnapmirrorUpdateRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorUpdateRequest) DestinationVserver() string {
	r := *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetDestinationVserver(newValue string) *SnapmirrorUpdateRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// EnableStorageEfficiency is a 'getter' method
func (o *SnapmirrorUpdateRequest) EnableStorageEfficiency() bool {
	r := *o.EnableStorageEfficiencyPtr
	return r
}

// SetEnableStorageEfficiency is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetEnableStorageEfficiency(newValue bool) *SnapmirrorUpdateRequest {
	o.EnableStorageEfficiencyPtr = &newValue
	return o
}

// MaxTransferRate is a 'getter' method
func (o *SnapmirrorUpdateRequest) MaxTransferRate() int {
	r := *o.MaxTransferRatePtr
	return r
}

// SetMaxTransferRate is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetMaxTransferRate(newValue int) *SnapmirrorUpdateRequest {
	o.MaxTransferRatePtr = &newValue
	return o
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorUpdateRequest) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetSourceCluster(newValue string) *SnapmirrorUpdateRequest {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorUpdateRequest) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetSourceLocation(newValue string) *SnapmirrorUpdateRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceSnapshot is a 'getter' method
func (o *SnapmirrorUpdateRequest) SourceSnapshot() string {
	r := *o.SourceSnapshotPtr
	return r
}

// SetSourceSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetSourceSnapshot(newValue string) *SnapmirrorUpdateRequest {
	o.SourceSnapshotPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorUpdateRequest) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetSourceVolume(newValue string) *SnapmirrorUpdateRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorUpdateRequest) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetSourceVserver(newValue string) *SnapmirrorUpdateRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// TransferPriority is a 'getter' method
func (o *SnapmirrorUpdateRequest) TransferPriority() string {
	r := *o.TransferPriorityPtr
	return r
}

// SetTransferPriority is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateRequest) SetTransferPriority(newValue string) *SnapmirrorUpdateRequest {
	o.TransferPriorityPtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *SnapmirrorUpdateResponseResult) ResultErrorCode() int {
	r := *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateResponseResult) SetResultErrorCode(newValue int) *SnapmirrorUpdateResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *SnapmirrorUpdateResponseResult) ResultErrorMessage() string {
	r := *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateResponseResult) SetResultErrorMessage(newValue string) *SnapmirrorUpdateResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *SnapmirrorUpdateResponseResult) ResultJobid() int {
	r := *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateResponseResult) SetResultJobid(newValue int) *SnapmirrorUpdateResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultOperationId is a 'getter' method
func (o *SnapmirrorUpdateResponseResult) ResultOperationId() string {
	r := *o.ResultOperationIdPtr
	return r
}

// SetResultOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateResponseResult) SetResultOperationId(newValue string) *SnapmirrorUpdateResponseResult {
	o.ResultOperationIdPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *SnapmirrorUpdateResponseResult) ResultStatus() string {
	r := *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *SnapmirrorUpdateResponseResult) SetResultStatus(newValue string) *SnapmirrorUpdateResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
