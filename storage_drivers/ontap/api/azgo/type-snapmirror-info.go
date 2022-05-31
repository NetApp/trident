// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SnapmirrorInfoType is a structure to represent a snapmirror-info ZAPI object
type SnapmirrorInfoType struct {
	XMLName                    xml.Name                          `xml:"snapmirror-info"`
	BreakFailedCountPtr        *uint64                           `xml:"break-failed-count"`
	BreakSuccessfulCountPtr    *uint64                           `xml:"break-successful-count"`
	CatalogStatusPtr           *string                           `xml:"catalog-status"`
	CatalogTotalProgressPtr    *uint64                           `xml:"catalog-total-progress"`
	CatalogTransferSnapshotPtr *string                           `xml:"catalog-transfer-snapshot"`
	CgItemMappingsPtr          *SnapmirrorInfoTypeCgItemMappings `xml:"cg-item-mappings"`
	// work in progress
	CurrentMaxTransferRatePtr    *uint                                  `xml:"current-max-transfer-rate"`
	CurrentOperationIdPtr        *string                                `xml:"current-operation-id"`
	CurrentTransferErrorPtr      *string                                `xml:"current-transfer-error"`
	CurrentTransferPriorityPtr   *string                                `xml:"current-transfer-priority"`
	CurrentTransferTypePtr       *string                                `xml:"current-transfer-type"`
	DestinationClusterPtr        *string                                `xml:"destination-cluster"`
	DestinationEndpointUuidPtr   *string                                `xml:"destination-endpoint-uuid"`
	DestinationLocationPtr       *string                                `xml:"destination-location"`
	DestinationVolumePtr         *string                                `xml:"destination-volume"`
	DestinationVolumeNodePtr     *string                                `xml:"destination-volume-node"`
	DestinationVserverPtr        *string                                `xml:"destination-vserver"`
	DestinationVserverUuidPtr    *string                                `xml:"destination-vserver-uuid"`
	ExportedSnapshotPtr          *string                                `xml:"exported-snapshot"`
	ExportedSnapshotTimestampPtr *uint                                  `xml:"exported-snapshot-timestamp"`
	FileRestoreFileCountPtr      *uint64                                `xml:"file-restore-file-count"`
	FileRestoreFileListPtr       *SnapmirrorInfoTypeFileRestoreFileList `xml:"file-restore-file-list"`
	// work in progress
	IdentityPreservePtr         *bool                                     `xml:"identity-preserve"`
	IsAutoExpandEnabledPtr      *bool                                     `xml:"is-auto-expand-enabled"`
	IsCatalogEnabledPtr         *bool                                     `xml:"is-catalog-enabled"`
	IsConstituentPtr            *bool                                     `xml:"is-constituent"`
	IsHealthyPtr                *bool                                     `xml:"is-healthy"`
	LagTimePtr                  *uint                                     `xml:"lag-time"`
	LastTransferDurationPtr     *uint                                     `xml:"last-transfer-duration"`
	LastTransferEndTimestampPtr *uint                                     `xml:"last-transfer-end-timestamp"`
	LastTransferErrorPtr        *string                                   `xml:"last-transfer-error"`
	LastTransferErrorCodesPtr   *SnapmirrorInfoTypeLastTransferErrorCodes `xml:"last-transfer-error-codes"`
	// work in progress
	LastTransferFromPtr                    *string `xml:"last-transfer-from"`
	LastTransferNetworkCompressionRatioPtr *string `xml:"last-transfer-network-compression-ratio"`
	LastTransferSizePtr                    *uint64 `xml:"last-transfer-size"`
	LastTransferTypePtr                    *string `xml:"last-transfer-type"`
	MaxTransferRatePtr                     *uint   `xml:"max-transfer-rate"`
	MirrorStatePtr                         *string `xml:"mirror-state"`
	NetworkCompressionRatioPtr             *string `xml:"network-compression-ratio"`
	NewestSnapshotPtr                      *string `xml:"newest-snapshot"`
	NewestSnapshotTimestampPtr             *uint   `xml:"newest-snapshot-timestamp"`
	// WARNING: Do not change opmask type to anything other than uint64, ZAPI param is of type uint64
	//          and returns hugh numerical values. Also keep other unint/unint64 types
	OpmaskPtr                   *uint64 `xml:"opmask"`
	PolicyPtr                   *string `xml:"policy"`
	PolicyTypePtr               *string `xml:"policy-type"`
	ProgressLastUpdatedPtr      *uint   `xml:"progress-last-updated"`
	RelationshipControlPlanePtr *string `xml:"relationship-control-plane"`
	RelationshipGroupTypePtr    *string `xml:"relationship-group-type"`
	RelationshipIdPtr           *string `xml:"relationship-id"`
	RelationshipProgressPtr     *uint64 `xml:"relationship-progress"`
	RelationshipStatusPtr       *string `xml:"relationship-status"`
	RelationshipTypePtr         *string `xml:"relationship-type"`
	ResyncFailedCountPtr        *uint64 `xml:"resync-failed-count"`
	ResyncSuccessfulCountPtr    *uint64 `xml:"resync-successful-count"`
	SchedulePtr                 *string `xml:"schedule"`
	SnapshotCheckpointPtr       *uint64 `xml:"snapshot-checkpoint"`
	SnapshotProgressPtr         *uint64 `xml:"snapshot-progress"`
	SourceClusterPtr            *string `xml:"source-cluster"`
	SourceEndpointUuidPtr       *string `xml:"source-endpoint-uuid"`
	SourceLocationPtr           *string `xml:"source-location"`
	SourceVolumePtr             *string `xml:"source-volume"`
	SourceVserverPtr            *string `xml:"source-vserver"`
	SourceVserverUuidPtr        *string `xml:"source-vserver-uuid"`
	TotalTransferBytesPtr       *uint64 `xml:"total-transfer-bytes"`
	TotalTransferTimeSecsPtr    *uint   `xml:"total-transfer-time-secs"`
	TransferSnapshotPtr         *string `xml:"transfer-snapshot"`
	TriesPtr                    *string `xml:"tries"`
	UnhealthyReasonPtr          *string `xml:"unhealthy-reason"`
	UpdateFailedCountPtr        *uint64 `xml:"update-failed-count"`
	UpdateSuccessfulCountPtr    *uint64 `xml:"update-successful-count"`
	VserverPtr                  *string `xml:"vserver"`
}

// NewSnapmirrorInfoType is a factory method for creating new instances of SnapmirrorInfoType objects
func NewSnapmirrorInfoType() *SnapmirrorInfoType {
	return &SnapmirrorInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// BreakFailedCount is a 'getter' method
func (o *SnapmirrorInfoType) BreakFailedCount() uint64 {
	var r uint64
	if o.BreakFailedCountPtr == nil {
		return r
	}
	r = *o.BreakFailedCountPtr
	return r
}

// SetBreakFailedCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetBreakFailedCount(newValue uint64) *SnapmirrorInfoType {
	o.BreakFailedCountPtr = &newValue
	return o
}

// BreakSuccessfulCount is a 'getter' method
func (o *SnapmirrorInfoType) BreakSuccessfulCount() uint64 {
	var r uint64
	if o.BreakSuccessfulCountPtr == nil {
		return r
	}
	r = *o.BreakSuccessfulCountPtr
	return r
}

// SetBreakSuccessfulCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetBreakSuccessfulCount(newValue uint64) *SnapmirrorInfoType {
	o.BreakSuccessfulCountPtr = &newValue
	return o
}

// CatalogStatus is a 'getter' method
func (o *SnapmirrorInfoType) CatalogStatus() string {
	var r string
	if o.CatalogStatusPtr == nil {
		return r
	}
	r = *o.CatalogStatusPtr
	return r
}

// SetCatalogStatus is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCatalogStatus(newValue string) *SnapmirrorInfoType {
	o.CatalogStatusPtr = &newValue
	return o
}

// CatalogTotalProgress is a 'getter' method
func (o *SnapmirrorInfoType) CatalogTotalProgress() uint64 {
	var r uint64
	if o.CatalogTotalProgressPtr == nil {
		return r
	}
	r = *o.CatalogTotalProgressPtr
	return r
}

// SetCatalogTotalProgress is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCatalogTotalProgress(newValue uint64) *SnapmirrorInfoType {
	o.CatalogTotalProgressPtr = &newValue
	return o
}

// CatalogTransferSnapshot is a 'getter' method
func (o *SnapmirrorInfoType) CatalogTransferSnapshot() string {
	var r string
	if o.CatalogTransferSnapshotPtr == nil {
		return r
	}
	r = *o.CatalogTransferSnapshotPtr
	return r
}

// SetCatalogTransferSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCatalogTransferSnapshot(newValue string) *SnapmirrorInfoType {
	o.CatalogTransferSnapshotPtr = &newValue
	return o
}

// SnapmirrorInfoTypeCgItemMappings is a wrapper
type SnapmirrorInfoTypeCgItemMappings struct {
	XMLName   xml.Name `xml:"cg-item-mappings"`
	StringPtr []string `xml:"string"`
}

// String is a 'getter' method
func (o *SnapmirrorInfoTypeCgItemMappings) String() []string {
	r := o.StringPtr
	return r
}

// SetString is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoTypeCgItemMappings) SetString(newValue []string) *SnapmirrorInfoTypeCgItemMappings {
	newSlice := make([]string, len(newValue))
	copy(newSlice, newValue)
	o.StringPtr = newSlice
	return o
}

// CgItemMappings is a 'getter' method
func (o *SnapmirrorInfoType) CgItemMappings() SnapmirrorInfoTypeCgItemMappings {
	var r SnapmirrorInfoTypeCgItemMappings
	if o.CgItemMappingsPtr == nil {
		return r
	}
	r = *o.CgItemMappingsPtr
	return r
}

// SetCgItemMappings is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCgItemMappings(newValue SnapmirrorInfoTypeCgItemMappings) *SnapmirrorInfoType {
	o.CgItemMappingsPtr = &newValue
	return o
}

// CurrentMaxTransferRate is a 'getter' method
func (o *SnapmirrorInfoType) CurrentMaxTransferRate() uint {
	var r uint
	if o.CurrentMaxTransferRatePtr == nil {
		return r
	}
	r = *o.CurrentMaxTransferRatePtr
	return r
}

// SetCurrentMaxTransferRate is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCurrentMaxTransferRate(newValue uint) *SnapmirrorInfoType {
	o.CurrentMaxTransferRatePtr = &newValue
	return o
}

// CurrentOperationId is a 'getter' method
func (o *SnapmirrorInfoType) CurrentOperationId() string {
	var r string
	if o.CurrentOperationIdPtr == nil {
		return r
	}
	r = *o.CurrentOperationIdPtr
	return r
}

// SetCurrentOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCurrentOperationId(newValue string) *SnapmirrorInfoType {
	o.CurrentOperationIdPtr = &newValue
	return o
}

// CurrentTransferError is a 'getter' method
func (o *SnapmirrorInfoType) CurrentTransferError() string {
	var r string
	if o.CurrentTransferErrorPtr == nil {
		return r
	}
	r = *o.CurrentTransferErrorPtr
	return r
}

// SetCurrentTransferError is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCurrentTransferError(newValue string) *SnapmirrorInfoType {
	o.CurrentTransferErrorPtr = &newValue
	return o
}

// CurrentTransferPriority is a 'getter' method
func (o *SnapmirrorInfoType) CurrentTransferPriority() string {
	var r string
	if o.CurrentTransferPriorityPtr == nil {
		return r
	}
	r = *o.CurrentTransferPriorityPtr
	return r
}

// SetCurrentTransferPriority is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCurrentTransferPriority(newValue string) *SnapmirrorInfoType {
	o.CurrentTransferPriorityPtr = &newValue
	return o
}

// CurrentTransferType is a 'getter' method
func (o *SnapmirrorInfoType) CurrentTransferType() string {
	var r string
	if o.CurrentTransferTypePtr == nil {
		return r
	}
	r = *o.CurrentTransferTypePtr
	return r
}

// SetCurrentTransferType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCurrentTransferType(newValue string) *SnapmirrorInfoType {
	o.CurrentTransferTypePtr = &newValue
	return o
}

// DestinationCluster is a 'getter' method
func (o *SnapmirrorInfoType) DestinationCluster() string {
	var r string
	if o.DestinationClusterPtr == nil {
		return r
	}
	r = *o.DestinationClusterPtr
	return r
}

// SetDestinationCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationCluster(newValue string) *SnapmirrorInfoType {
	o.DestinationClusterPtr = &newValue
	return o
}

// DestinationEndpointUuid is a 'getter' method
func (o *SnapmirrorInfoType) DestinationEndpointUuid() string {
	var r string
	if o.DestinationEndpointUuidPtr == nil {
		return r
	}
	r = *o.DestinationEndpointUuidPtr
	return r
}

// SetDestinationEndpointUuid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationEndpointUuid(newValue string) *SnapmirrorInfoType {
	o.DestinationEndpointUuidPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorInfoType) DestinationLocation() string {
	var r string
	if o.DestinationLocationPtr == nil {
		return r
	}
	r = *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationLocation(newValue string) *SnapmirrorInfoType {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorInfoType) DestinationVolume() string {
	var r string
	if o.DestinationVolumePtr == nil {
		return r
	}
	r = *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationVolume(newValue string) *SnapmirrorInfoType {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVolumeNode is a 'getter' method
func (o *SnapmirrorInfoType) DestinationVolumeNode() string {
	var r string
	if o.DestinationVolumeNodePtr == nil {
		return r
	}
	r = *o.DestinationVolumeNodePtr
	return r
}

// SetDestinationVolumeNode is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationVolumeNode(newValue string) *SnapmirrorInfoType {
	o.DestinationVolumeNodePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorInfoType) DestinationVserver() string {
	var r string
	if o.DestinationVserverPtr == nil {
		return r
	}
	r = *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationVserver(newValue string) *SnapmirrorInfoType {
	o.DestinationVserverPtr = &newValue
	return o
}

// DestinationVserverUuid is a 'getter' method
func (o *SnapmirrorInfoType) DestinationVserverUuid() string {
	var r string
	if o.DestinationVserverUuidPtr == nil {
		return r
	}
	r = *o.DestinationVserverUuidPtr
	return r
}

// SetDestinationVserverUuid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationVserverUuid(newValue string) *SnapmirrorInfoType {
	o.DestinationVserverUuidPtr = &newValue
	return o
}

// ExportedSnapshot is a 'getter' method
func (o *SnapmirrorInfoType) ExportedSnapshot() string {
	var r string
	if o.ExportedSnapshotPtr == nil {
		return r
	}
	r = *o.ExportedSnapshotPtr
	return r
}

// SetExportedSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetExportedSnapshot(newValue string) *SnapmirrorInfoType {
	o.ExportedSnapshotPtr = &newValue
	return o
}

// ExportedSnapshotTimestamp is a 'getter' method
func (o *SnapmirrorInfoType) ExportedSnapshotTimestamp() uint {
	var r uint
	if o.ExportedSnapshotTimestampPtr == nil {
		return r
	}
	r = *o.ExportedSnapshotTimestampPtr
	return r
}

// SetExportedSnapshotTimestamp is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetExportedSnapshotTimestamp(newValue uint) *SnapmirrorInfoType {
	o.ExportedSnapshotTimestampPtr = &newValue
	return o
}

// FileRestoreFileCount is a 'getter' method
func (o *SnapmirrorInfoType) FileRestoreFileCount() uint64 {
	var r uint64
	if o.FileRestoreFileCountPtr == nil {
		return r
	}
	r = *o.FileRestoreFileCountPtr
	return r
}

// SetFileRestoreFileCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetFileRestoreFileCount(newValue uint64) *SnapmirrorInfoType {
	o.FileRestoreFileCountPtr = &newValue
	return o
}

// SnapmirrorInfoTypeFileRestoreFileList is a wrapper
type SnapmirrorInfoTypeFileRestoreFileList struct {
	XMLName   xml.Name `xml:"file-restore-file-list"`
	StringPtr []string `xml:"string"`
}

// String is a 'getter' method
func (o *SnapmirrorInfoTypeFileRestoreFileList) String() []string {
	r := o.StringPtr
	return r
}

// SetString is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoTypeFileRestoreFileList) SetString(newValue []string) *SnapmirrorInfoTypeFileRestoreFileList {
	newSlice := make([]string, len(newValue))
	copy(newSlice, newValue)
	o.StringPtr = newSlice
	return o
}

// FileRestoreFileList is a 'getter' method
func (o *SnapmirrorInfoType) FileRestoreFileList() SnapmirrorInfoTypeFileRestoreFileList {
	var r SnapmirrorInfoTypeFileRestoreFileList
	if o.FileRestoreFileListPtr == nil {
		return r
	}
	r = *o.FileRestoreFileListPtr
	return r
}

// SetFileRestoreFileList is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetFileRestoreFileList(newValue SnapmirrorInfoTypeFileRestoreFileList) *SnapmirrorInfoType {
	o.FileRestoreFileListPtr = &newValue
	return o
}

// IdentityPreserve is a 'getter' method
func (o *SnapmirrorInfoType) IdentityPreserve() bool {
	var r bool
	if o.IdentityPreservePtr == nil {
		return r
	}
	r = *o.IdentityPreservePtr
	return r
}

// SetIdentityPreserve is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetIdentityPreserve(newValue bool) *SnapmirrorInfoType {
	o.IdentityPreservePtr = &newValue
	return o
}

// IsAutoExpandEnabled is a 'getter' method
func (o *SnapmirrorInfoType) IsAutoExpandEnabled() bool {
	var r bool
	if o.IsAutoExpandEnabledPtr == nil {
		return r
	}
	r = *o.IsAutoExpandEnabledPtr
	return r
}

// SetIsAutoExpandEnabled is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetIsAutoExpandEnabled(newValue bool) *SnapmirrorInfoType {
	o.IsAutoExpandEnabledPtr = &newValue
	return o
}

// IsCatalogEnabled is a 'getter' method
func (o *SnapmirrorInfoType) IsCatalogEnabled() bool {
	var r bool
	if o.IsCatalogEnabledPtr == nil {
		return r
	}
	r = *o.IsCatalogEnabledPtr
	return r
}

// SetIsCatalogEnabled is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetIsCatalogEnabled(newValue bool) *SnapmirrorInfoType {
	o.IsCatalogEnabledPtr = &newValue
	return o
}

// IsConstituent is a 'getter' method
func (o *SnapmirrorInfoType) IsConstituent() bool {
	var r bool
	if o.IsConstituentPtr == nil {
		return r
	}
	r = *o.IsConstituentPtr
	return r
}

// SetIsConstituent is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetIsConstituent(newValue bool) *SnapmirrorInfoType {
	o.IsConstituentPtr = &newValue
	return o
}

// IsHealthy is a 'getter' method
func (o *SnapmirrorInfoType) IsHealthy() bool {
	var r bool
	if o.IsHealthyPtr == nil {
		return r
	}
	r = *o.IsHealthyPtr
	return r
}

// SetIsHealthy is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetIsHealthy(newValue bool) *SnapmirrorInfoType {
	o.IsHealthyPtr = &newValue
	return o
}

// LagTime is a 'getter' method
func (o *SnapmirrorInfoType) LagTime() uint {
	var r uint
	if o.LagTimePtr == nil {
		return r
	}
	r = *o.LagTimePtr
	return r
}

// SetLagTime is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLagTime(newValue uint) *SnapmirrorInfoType {
	o.LagTimePtr = &newValue
	return o
}

// LastTransferDuration is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferDuration() uint {
	var r uint
	if o.LastTransferDurationPtr == nil {
		return r
	}
	r = *o.LastTransferDurationPtr
	return r
}

// SetLastTransferDuration is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferDuration(newValue uint) *SnapmirrorInfoType {
	o.LastTransferDurationPtr = &newValue
	return o
}

// LastTransferEndTimestamp is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferEndTimestamp() uint {
	var r uint
	if o.LastTransferEndTimestampPtr == nil {
		return r
	}
	r = *o.LastTransferEndTimestampPtr
	return r
}

// SetLastTransferEndTimestamp is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferEndTimestamp(newValue uint) *SnapmirrorInfoType {
	o.LastTransferEndTimestampPtr = &newValue
	return o
}

// LastTransferError is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferError() string {
	var r string
	if o.LastTransferErrorPtr == nil {
		return r
	}
	r = *o.LastTransferErrorPtr
	return r
}

// SetLastTransferError is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferError(newValue string) *SnapmirrorInfoType {
	o.LastTransferErrorPtr = &newValue
	return o
}

// SnapmirrorInfoTypeLastTransferErrorCodes is a wrapper
type SnapmirrorInfoTypeLastTransferErrorCodes struct {
	XMLName    xml.Name `xml:"last-transfer-error-codes"`
	IntegerPtr []int    `xml:"integer"`
}

// Integer is a 'getter' method
func (o *SnapmirrorInfoTypeLastTransferErrorCodes) Integer() []int {
	r := o.IntegerPtr
	return r
}

// SetInteger is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoTypeLastTransferErrorCodes) SetInteger(newValue []int) *SnapmirrorInfoTypeLastTransferErrorCodes {
	newSlice := make([]int, len(newValue))
	copy(newSlice, newValue)
	o.IntegerPtr = newSlice
	return o
}

// LastTransferErrorCodes is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferErrorCodes() SnapmirrorInfoTypeLastTransferErrorCodes {
	var r SnapmirrorInfoTypeLastTransferErrorCodes
	if o.LastTransferErrorCodesPtr == nil {
		return r
	}
	r = *o.LastTransferErrorCodesPtr
	return r
}

// SetLastTransferErrorCodes is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferErrorCodes(newValue SnapmirrorInfoTypeLastTransferErrorCodes) *SnapmirrorInfoType {
	o.LastTransferErrorCodesPtr = &newValue
	return o
}

// LastTransferFrom is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferFrom() string {
	var r string
	if o.LastTransferFromPtr == nil {
		return r
	}
	r = *o.LastTransferFromPtr
	return r
}

// SetLastTransferFrom is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferFrom(newValue string) *SnapmirrorInfoType {
	o.LastTransferFromPtr = &newValue
	return o
}

// LastTransferNetworkCompressionRatio is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferNetworkCompressionRatio() string {
	var r string
	if o.LastTransferNetworkCompressionRatioPtr == nil {
		return r
	}
	r = *o.LastTransferNetworkCompressionRatioPtr
	return r
}

// SetLastTransferNetworkCompressionRatio is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferNetworkCompressionRatio(newValue string) *SnapmirrorInfoType {
	o.LastTransferNetworkCompressionRatioPtr = &newValue
	return o
}

// LastTransferSize is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferSize() uint64 {
	var r uint64
	if o.LastTransferSizePtr == nil {
		return r
	}
	r = *o.LastTransferSizePtr
	return r
}

// SetLastTransferSize is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferSize(newValue uint64) *SnapmirrorInfoType {
	o.LastTransferSizePtr = &newValue
	return o
}

// LastTransferType is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferType() string {
	var r string
	if o.LastTransferTypePtr == nil {
		return r
	}
	r = *o.LastTransferTypePtr
	return r
}

// SetLastTransferType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferType(newValue string) *SnapmirrorInfoType {
	o.LastTransferTypePtr = &newValue
	return o
}

// MaxTransferRate is a 'getter' method
func (o *SnapmirrorInfoType) MaxTransferRate() uint {
	var r uint
	if o.MaxTransferRatePtr == nil {
		return r
	}
	r = *o.MaxTransferRatePtr
	return r
}

// SetMaxTransferRate is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetMaxTransferRate(newValue uint) *SnapmirrorInfoType {
	o.MaxTransferRatePtr = &newValue
	return o
}

// MirrorState is a 'getter' method
func (o *SnapmirrorInfoType) MirrorState() string {
	var r string
	if o.MirrorStatePtr == nil {
		return r
	}
	r = *o.MirrorStatePtr
	return r
}

// SetMirrorState is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetMirrorState(newValue string) *SnapmirrorInfoType {
	o.MirrorStatePtr = &newValue
	return o
}

// NetworkCompressionRatio is a 'getter' method
func (o *SnapmirrorInfoType) NetworkCompressionRatio() string {
	var r string
	if o.NetworkCompressionRatioPtr == nil {
		return r
	}
	r = *o.NetworkCompressionRatioPtr
	return r
}

// SetNetworkCompressionRatio is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetNetworkCompressionRatio(newValue string) *SnapmirrorInfoType {
	o.NetworkCompressionRatioPtr = &newValue
	return o
}

// NewestSnapshot is a 'getter' method
func (o *SnapmirrorInfoType) NewestSnapshot() string {
	var r string
	if o.NewestSnapshotPtr == nil {
		return r
	}
	r = *o.NewestSnapshotPtr
	return r
}

// SetNewestSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetNewestSnapshot(newValue string) *SnapmirrorInfoType {
	o.NewestSnapshotPtr = &newValue
	return o
}

// NewestSnapshotTimestamp is a 'getter' method
func (o *SnapmirrorInfoType) NewestSnapshotTimestamp() uint {
	var r uint
	if o.NewestSnapshotTimestampPtr == nil {
		return r
	}
	r = *o.NewestSnapshotTimestampPtr
	return r
}

// SetNewestSnapshotTimestamp is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetNewestSnapshotTimestamp(newValue uint) *SnapmirrorInfoType {
	o.NewestSnapshotTimestampPtr = &newValue
	return o
}

// Opmask is a 'getter' method
func (o *SnapmirrorInfoType) Opmask() uint64 {
	var r uint64
	if o.OpmaskPtr == nil {
		return r
	}
	r = *o.OpmaskPtr
	return r
}

// SetOpmask is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetOpmask(newValue uint64) *SnapmirrorInfoType {
	o.OpmaskPtr = &newValue
	return o
}

// Policy is a 'getter' method
func (o *SnapmirrorInfoType) Policy() string {
	var r string
	if o.PolicyPtr == nil {
		return r
	}
	r = *o.PolicyPtr
	return r
}

// SetPolicy is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetPolicy(newValue string) *SnapmirrorInfoType {
	o.PolicyPtr = &newValue
	return o
}

// PolicyType is a 'getter' method
func (o *SnapmirrorInfoType) PolicyType() string {
	var r string
	if o.PolicyTypePtr == nil {
		return r
	}
	r = *o.PolicyTypePtr
	return r
}

// SetPolicyType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetPolicyType(newValue string) *SnapmirrorInfoType {
	o.PolicyTypePtr = &newValue
	return o
}

// ProgressLastUpdated is a 'getter' method
func (o *SnapmirrorInfoType) ProgressLastUpdated() uint {
	var r uint
	if o.ProgressLastUpdatedPtr == nil {
		return r
	}
	r = *o.ProgressLastUpdatedPtr
	return r
}

// SetProgressLastUpdated is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetProgressLastUpdated(newValue uint) *SnapmirrorInfoType {
	o.ProgressLastUpdatedPtr = &newValue
	return o
}

// RelationshipControlPlane is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipControlPlane() string {
	var r string
	if o.RelationshipControlPlanePtr == nil {
		return r
	}
	r = *o.RelationshipControlPlanePtr
	return r
}

// SetRelationshipControlPlane is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipControlPlane(newValue string) *SnapmirrorInfoType {
	o.RelationshipControlPlanePtr = &newValue
	return o
}

// RelationshipGroupType is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipGroupType() string {
	var r string
	if o.RelationshipGroupTypePtr == nil {
		return r
	}
	r = *o.RelationshipGroupTypePtr
	return r
}

// SetRelationshipGroupType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipGroupType(newValue string) *SnapmirrorInfoType {
	o.RelationshipGroupTypePtr = &newValue
	return o
}

// RelationshipId is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipId() string {
	var r string
	if o.RelationshipIdPtr == nil {
		return r
	}
	r = *o.RelationshipIdPtr
	return r
}

// SetRelationshipId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipId(newValue string) *SnapmirrorInfoType {
	o.RelationshipIdPtr = &newValue
	return o
}

// RelationshipProgress is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipProgress() uint64 {
	var r uint64
	if o.RelationshipProgressPtr == nil {
		return r
	}
	r = *o.RelationshipProgressPtr
	return r
}

// SetRelationshipProgress is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipProgress(newValue uint64) *SnapmirrorInfoType {
	o.RelationshipProgressPtr = &newValue
	return o
}

// RelationshipStatus is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipStatus() string {
	var r string
	if o.RelationshipStatusPtr == nil {
		return r
	}
	r = *o.RelationshipStatusPtr
	return r
}

// SetRelationshipStatus is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipStatus(newValue string) *SnapmirrorInfoType {
	o.RelationshipStatusPtr = &newValue
	return o
}

// RelationshipType is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipType() string {
	var r string
	if o.RelationshipTypePtr == nil {
		return r
	}
	r = *o.RelationshipTypePtr
	return r
}

// SetRelationshipType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipType(newValue string) *SnapmirrorInfoType {
	o.RelationshipTypePtr = &newValue
	return o
}

// ResyncFailedCount is a 'getter' method
func (o *SnapmirrorInfoType) ResyncFailedCount() uint64 {
	var r uint64
	if o.ResyncFailedCountPtr == nil {
		return r
	}
	r = *o.ResyncFailedCountPtr
	return r
}

// SetResyncFailedCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetResyncFailedCount(newValue uint64) *SnapmirrorInfoType {
	o.ResyncFailedCountPtr = &newValue
	return o
}

// ResyncSuccessfulCount is a 'getter' method
func (o *SnapmirrorInfoType) ResyncSuccessfulCount() uint64 {
	var r uint64
	if o.ResyncSuccessfulCountPtr == nil {
		return r
	}
	r = *o.ResyncSuccessfulCountPtr
	return r
}

// SetResyncSuccessfulCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetResyncSuccessfulCount(newValue uint64) *SnapmirrorInfoType {
	o.ResyncSuccessfulCountPtr = &newValue
	return o
}

// Schedule is a 'getter' method
func (o *SnapmirrorInfoType) Schedule() string {
	var r string
	if o.SchedulePtr == nil {
		return r
	}
	r = *o.SchedulePtr
	return r
}

// SetSchedule is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSchedule(newValue string) *SnapmirrorInfoType {
	o.SchedulePtr = &newValue
	return o
}

// SnapshotCheckpoint is a 'getter' method
func (o *SnapmirrorInfoType) SnapshotCheckpoint() uint64 {
	var r uint64
	if o.SnapshotCheckpointPtr == nil {
		return r
	}
	r = *o.SnapshotCheckpointPtr
	return r
}

// SetSnapshotCheckpoint is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSnapshotCheckpoint(newValue uint64) *SnapmirrorInfoType {
	o.SnapshotCheckpointPtr = &newValue
	return o
}

// SnapshotProgress is a 'getter' method
func (o *SnapmirrorInfoType) SnapshotProgress() uint64 {
	var r uint64
	if o.SnapshotProgressPtr == nil {
		return r
	}
	r = *o.SnapshotProgressPtr
	return r
}

// SetSnapshotProgress is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSnapshotProgress(newValue uint64) *SnapmirrorInfoType {
	o.SnapshotProgressPtr = &newValue
	return o
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorInfoType) SourceCluster() string {
	var r string
	if o.SourceClusterPtr == nil {
		return r
	}
	r = *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSourceCluster(newValue string) *SnapmirrorInfoType {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceEndpointUuid is a 'getter' method
func (o *SnapmirrorInfoType) SourceEndpointUuid() string {
	var r string
	if o.SourceEndpointUuidPtr == nil {
		return r
	}
	r = *o.SourceEndpointUuidPtr
	return r
}

// SetSourceEndpointUuid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSourceEndpointUuid(newValue string) *SnapmirrorInfoType {
	o.SourceEndpointUuidPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorInfoType) SourceLocation() string {
	var r string
	if o.SourceLocationPtr == nil {
		return r
	}
	r = *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSourceLocation(newValue string) *SnapmirrorInfoType {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorInfoType) SourceVolume() string {
	var r string
	if o.SourceVolumePtr == nil {
		return r
	}
	r = *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSourceVolume(newValue string) *SnapmirrorInfoType {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorInfoType) SourceVserver() string {
	var r string
	if o.SourceVserverPtr == nil {
		return r
	}
	r = *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSourceVserver(newValue string) *SnapmirrorInfoType {
	o.SourceVserverPtr = &newValue
	return o
}

// SourceVserverUuid is a 'getter' method
func (o *SnapmirrorInfoType) SourceVserverUuid() string {
	var r string
	if o.SourceVserverUuidPtr == nil {
		return r
	}
	r = *o.SourceVserverUuidPtr
	return r
}

// SetSourceVserverUuid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSourceVserverUuid(newValue string) *SnapmirrorInfoType {
	o.SourceVserverUuidPtr = &newValue
	return o
}

// TotalTransferBytes is a 'getter' method
func (o *SnapmirrorInfoType) TotalTransferBytes() uint64 {
	var r uint64
	if o.TotalTransferBytesPtr == nil {
		return r
	}
	r = *o.TotalTransferBytesPtr
	return r
}

// SetTotalTransferBytes is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetTotalTransferBytes(newValue uint64) *SnapmirrorInfoType {
	o.TotalTransferBytesPtr = &newValue
	return o
}

// TotalTransferTimeSecs is a 'getter' method
func (o *SnapmirrorInfoType) TotalTransferTimeSecs() uint {
	var r uint
	if o.TotalTransferTimeSecsPtr == nil {
		return r
	}
	r = *o.TotalTransferTimeSecsPtr
	return r
}

// SetTotalTransferTimeSecs is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetTotalTransferTimeSecs(newValue uint) *SnapmirrorInfoType {
	o.TotalTransferTimeSecsPtr = &newValue
	return o
}

// TransferSnapshot is a 'getter' method
func (o *SnapmirrorInfoType) TransferSnapshot() string {
	var r string
	if o.TransferSnapshotPtr == nil {
		return r
	}
	r = *o.TransferSnapshotPtr
	return r
}

// SetTransferSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetTransferSnapshot(newValue string) *SnapmirrorInfoType {
	o.TransferSnapshotPtr = &newValue
	return o
}

// Tries is a 'getter' method
func (o *SnapmirrorInfoType) Tries() string {
	var r string
	if o.TriesPtr == nil {
		return r
	}
	r = *o.TriesPtr
	return r
}

// SetTries is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetTries(newValue string) *SnapmirrorInfoType {
	o.TriesPtr = &newValue
	return o
}

// UnhealthyReason is a 'getter' method
func (o *SnapmirrorInfoType) UnhealthyReason() string {
	var r string
	if o.UnhealthyReasonPtr == nil {
		return r
	}
	r = *o.UnhealthyReasonPtr
	return r
}

// SetUnhealthyReason is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetUnhealthyReason(newValue string) *SnapmirrorInfoType {
	o.UnhealthyReasonPtr = &newValue
	return o
}

// UpdateFailedCount is a 'getter' method
func (o *SnapmirrorInfoType) UpdateFailedCount() uint64 {
	var r uint64
	if o.UpdateFailedCountPtr == nil {
		return r
	}
	r = *o.UpdateFailedCountPtr
	return r
}

// SetUpdateFailedCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetUpdateFailedCount(newValue uint64) *SnapmirrorInfoType {
	o.UpdateFailedCountPtr = &newValue
	return o
}

// UpdateSuccessfulCount is a 'getter' method
func (o *SnapmirrorInfoType) UpdateSuccessfulCount() uint64 {
	var r uint64
	if o.UpdateSuccessfulCountPtr == nil {
		return r
	}
	r = *o.UpdateSuccessfulCountPtr
	return r
}

// SetUpdateSuccessfulCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetUpdateSuccessfulCount(newValue uint64) *SnapmirrorInfoType {
	o.UpdateSuccessfulCountPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *SnapmirrorInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetVserver(newValue string) *SnapmirrorInfoType {
	o.VserverPtr = &newValue
	return o
}
