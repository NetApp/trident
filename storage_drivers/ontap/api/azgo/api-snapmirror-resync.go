// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SnapmirrorResyncRequest is a structure to represent a snapmirror-resync Request ZAPI object
type SnapmirrorResyncRequest struct {
	XMLName                    xml.Name  `xml:"snapmirror-resync"`
	DestinationEndpointUuidPtr *UuidType `xml:"destination-endpoint-uuid"`
	DestinationLocationPtr     *string   `xml:"destination-location"`
	DestinationVolumePtr       *string   `xml:"destination-volume"`
	DestinationVserverPtr      *string   `xml:"destination-vserver"`
	IsAdaptivePtr              *bool     `xml:"is-adaptive"`
	IsCatalogEnabledPtr        *bool     `xml:"is-catalog-enabled"`
	MaxTransferRatePtr         *int      `xml:"max-transfer-rate"`
	PreservePtr                *bool     `xml:"preserve"`
	QuickResyncPtr             *bool     `xml:"quick-resync"`
	RemoveCatalogBaseOwnersPtr *bool     `xml:"remove-catalog-base-owners"`
	SourceLocationPtr          *string   `xml:"source-location"`
	SourceSnapshotPtr          *string   `xml:"source-snapshot"`
	SourceVolumePtr            *string   `xml:"source-volume"`
	SourceVserverPtr           *string   `xml:"source-vserver"`
	TransferPriorityPtr        *string   `xml:"transfer-priority"`
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

// DestinationEndpointUuid is a 'getter' method
func (o *SnapmirrorResyncRequest) DestinationEndpointUuid() UuidType {
	var r UuidType
	if o.DestinationEndpointUuidPtr == nil {
		return r
	}
	r = *o.DestinationEndpointUuidPtr
	return r
}

// SetDestinationEndpointUuid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetDestinationEndpointUuid(newValue UuidType) *SnapmirrorResyncRequest {
	o.DestinationEndpointUuidPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorResyncRequest) DestinationLocation() string {
	var r string
	if o.DestinationLocationPtr == nil {
		return r
	}
	r = *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetDestinationLocation(newValue string) *SnapmirrorResyncRequest {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorResyncRequest) DestinationVolume() string {
	var r string
	if o.DestinationVolumePtr == nil {
		return r
	}
	r = *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetDestinationVolume(newValue string) *SnapmirrorResyncRequest {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorResyncRequest) DestinationVserver() string {
	var r string
	if o.DestinationVserverPtr == nil {
		return r
	}
	r = *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetDestinationVserver(newValue string) *SnapmirrorResyncRequest {
	o.DestinationVserverPtr = &newValue
	return o
}

// IsAdaptive is a 'getter' method
func (o *SnapmirrorResyncRequest) IsAdaptive() bool {
	var r bool
	if o.IsAdaptivePtr == nil {
		return r
	}
	r = *o.IsAdaptivePtr
	return r
}

// SetIsAdaptive is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetIsAdaptive(newValue bool) *SnapmirrorResyncRequest {
	o.IsAdaptivePtr = &newValue
	return o
}

// IsCatalogEnabled is a 'getter' method
func (o *SnapmirrorResyncRequest) IsCatalogEnabled() bool {
	var r bool
	if o.IsCatalogEnabledPtr == nil {
		return r
	}
	r = *o.IsCatalogEnabledPtr
	return r
}

// SetIsCatalogEnabled is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetIsCatalogEnabled(newValue bool) *SnapmirrorResyncRequest {
	o.IsCatalogEnabledPtr = &newValue
	return o
}

// MaxTransferRate is a 'getter' method
func (o *SnapmirrorResyncRequest) MaxTransferRate() int {
	var r int
	if o.MaxTransferRatePtr == nil {
		return r
	}
	r = *o.MaxTransferRatePtr
	return r
}

// SetMaxTransferRate is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetMaxTransferRate(newValue int) *SnapmirrorResyncRequest {
	o.MaxTransferRatePtr = &newValue
	return o
}

// Preserve is a 'getter' method
func (o *SnapmirrorResyncRequest) Preserve() bool {
	var r bool
	if o.PreservePtr == nil {
		return r
	}
	r = *o.PreservePtr
	return r
}

// SetPreserve is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetPreserve(newValue bool) *SnapmirrorResyncRequest {
	o.PreservePtr = &newValue
	return o
}

// QuickResync is a 'getter' method
func (o *SnapmirrorResyncRequest) QuickResync() bool {
	var r bool
	if o.QuickResyncPtr == nil {
		return r
	}
	r = *o.QuickResyncPtr
	return r
}

// SetQuickResync is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetQuickResync(newValue bool) *SnapmirrorResyncRequest {
	o.QuickResyncPtr = &newValue
	return o
}

// RemoveCatalogBaseOwners is a 'getter' method
func (o *SnapmirrorResyncRequest) RemoveCatalogBaseOwners() bool {
	var r bool
	if o.RemoveCatalogBaseOwnersPtr == nil {
		return r
	}
	r = *o.RemoveCatalogBaseOwnersPtr
	return r
}

// SetRemoveCatalogBaseOwners is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetRemoveCatalogBaseOwners(newValue bool) *SnapmirrorResyncRequest {
	o.RemoveCatalogBaseOwnersPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorResyncRequest) SourceLocation() string {
	var r string
	if o.SourceLocationPtr == nil {
		return r
	}
	r = *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetSourceLocation(newValue string) *SnapmirrorResyncRequest {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceSnapshot is a 'getter' method
func (o *SnapmirrorResyncRequest) SourceSnapshot() string {
	var r string
	if o.SourceSnapshotPtr == nil {
		return r
	}
	r = *o.SourceSnapshotPtr
	return r
}

// SetSourceSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetSourceSnapshot(newValue string) *SnapmirrorResyncRequest {
	o.SourceSnapshotPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorResyncRequest) SourceVolume() string {
	var r string
	if o.SourceVolumePtr == nil {
		return r
	}
	r = *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetSourceVolume(newValue string) *SnapmirrorResyncRequest {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorResyncRequest) SourceVserver() string {
	var r string
	if o.SourceVserverPtr == nil {
		return r
	}
	r = *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetSourceVserver(newValue string) *SnapmirrorResyncRequest {
	o.SourceVserverPtr = &newValue
	return o
}

// TransferPriority is a 'getter' method
func (o *SnapmirrorResyncRequest) TransferPriority() string {
	var r string
	if o.TransferPriorityPtr == nil {
		return r
	}
	r = *o.TransferPriorityPtr
	return r
}

// SetTransferPriority is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncRequest) SetTransferPriority(newValue string) *SnapmirrorResyncRequest {
	o.TransferPriorityPtr = &newValue
	return o
}

// ResultErrorCode is a 'getter' method
func (o *SnapmirrorResyncResponseResult) ResultErrorCode() int {
	var r int
	if o.ResultErrorCodePtr == nil {
		return r
	}
	r = *o.ResultErrorCodePtr
	return r
}

// SetResultErrorCode is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncResponseResult) SetResultErrorCode(newValue int) *SnapmirrorResyncResponseResult {
	o.ResultErrorCodePtr = &newValue
	return o
}

// ResultErrorMessage is a 'getter' method
func (o *SnapmirrorResyncResponseResult) ResultErrorMessage() string {
	var r string
	if o.ResultErrorMessagePtr == nil {
		return r
	}
	r = *o.ResultErrorMessagePtr
	return r
}

// SetResultErrorMessage is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncResponseResult) SetResultErrorMessage(newValue string) *SnapmirrorResyncResponseResult {
	o.ResultErrorMessagePtr = &newValue
	return o
}

// ResultJobid is a 'getter' method
func (o *SnapmirrorResyncResponseResult) ResultJobid() int {
	var r int
	if o.ResultJobidPtr == nil {
		return r
	}
	r = *o.ResultJobidPtr
	return r
}

// SetResultJobid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncResponseResult) SetResultJobid(newValue int) *SnapmirrorResyncResponseResult {
	o.ResultJobidPtr = &newValue
	return o
}

// ResultOperationId is a 'getter' method
func (o *SnapmirrorResyncResponseResult) ResultOperationId() string {
	var r string
	if o.ResultOperationIdPtr == nil {
		return r
	}
	r = *o.ResultOperationIdPtr
	return r
}

// SetResultOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncResponseResult) SetResultOperationId(newValue string) *SnapmirrorResyncResponseResult {
	o.ResultOperationIdPtr = &newValue
	return o
}

// ResultStatus is a 'getter' method
func (o *SnapmirrorResyncResponseResult) ResultStatus() string {
	var r string
	if o.ResultStatusPtr == nil {
		return r
	}
	r = *o.ResultStatusPtr
	return r
}

// SetResultStatus is a fluent style 'setter' method that can be chained
func (o *SnapmirrorResyncResponseResult) SetResultStatus(newValue string) *SnapmirrorResyncResponseResult {
	o.ResultStatusPtr = &newValue
	return o
}
