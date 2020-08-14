package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorInfoType is a structure to represent a snapmirror-info ZAPI object
type SnapmirrorInfoType struct {
	XMLName                      xml.Name                               `xml:"snapmirror-info"`
	BreakFailedCountPtr          *uint64                                `xml:"break-failed-count"`
	BreakSuccessfulCountPtr      *uint64                                `xml:"break-successful-count"`
	CurrentMaxTransferRatePtr    *uint                                  `xml:"current-max-transfer-rate"`
	CurrentOperationIdPtr        *string                                `xml:"current-operation-id"`
	CurrentTransferErrorPtr      *string                                `xml:"current-transfer-error"`
	CurrentTransferPriorityPtr   *string                                `xml:"current-transfer-priority"`
	CurrentTransferTypePtr       *string                                `xml:"current-transfer-type"`
	DestinationClusterPtr        *string                                `xml:"destination-cluster"`
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
	//          and returns hugh numerical values. Also keep other unint/unint64 types as it is.
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
	r := *o.BreakFailedCountPtr
	return r
}

// SetBreakFailedCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetBreakFailedCount(newValue uint64) *SnapmirrorInfoType {
	o.BreakFailedCountPtr = &newValue
	return o
}

// BreakSuccessfulCount is a 'getter' method
func (o *SnapmirrorInfoType) BreakSuccessfulCount() uint64 {
	r := *o.BreakSuccessfulCountPtr
	return r
}

// SetBreakSuccessfulCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetBreakSuccessfulCount(newValue uint64) *SnapmirrorInfoType {
	o.BreakSuccessfulCountPtr = &newValue
	return o
}

// CurrentMaxTransferRate is a 'getter' method
func (o *SnapmirrorInfoType) CurrentMaxTransferRate() uint {
	r := *o.CurrentMaxTransferRatePtr
	return r
}

// SetCurrentMaxTransferRate is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCurrentMaxTransferRate(newValue uint) *SnapmirrorInfoType {
	o.CurrentMaxTransferRatePtr = &newValue
	return o
}

// CurrentOperationId is a 'getter' method
func (o *SnapmirrorInfoType) CurrentOperationId() string {
	r := *o.CurrentOperationIdPtr
	return r
}

// SetCurrentOperationId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCurrentOperationId(newValue string) *SnapmirrorInfoType {
	o.CurrentOperationIdPtr = &newValue
	return o
}

// CurrentTransferError is a 'getter' method
func (o *SnapmirrorInfoType) CurrentTransferError() string {
	r := *o.CurrentTransferErrorPtr
	return r
}

// SetCurrentTransferError is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCurrentTransferError(newValue string) *SnapmirrorInfoType {
	o.CurrentTransferErrorPtr = &newValue
	return o
}

// CurrentTransferPriority is a 'getter' method
func (o *SnapmirrorInfoType) CurrentTransferPriority() string {
	r := *o.CurrentTransferPriorityPtr
	return r
}

// SetCurrentTransferPriority is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCurrentTransferPriority(newValue string) *SnapmirrorInfoType {
	o.CurrentTransferPriorityPtr = &newValue
	return o
}

// CurrentTransferType is a 'getter' method
func (o *SnapmirrorInfoType) CurrentTransferType() string {
	r := *o.CurrentTransferTypePtr
	return r
}

// SetCurrentTransferType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetCurrentTransferType(newValue string) *SnapmirrorInfoType {
	o.CurrentTransferTypePtr = &newValue
	return o
}

// DestinationCluster is a 'getter' method
func (o *SnapmirrorInfoType) DestinationCluster() string {
	r := *o.DestinationClusterPtr
	return r
}

// SetDestinationCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationCluster(newValue string) *SnapmirrorInfoType {
	o.DestinationClusterPtr = &newValue
	return o
}

// DestinationLocation is a 'getter' method
func (o *SnapmirrorInfoType) DestinationLocation() string {
	r := *o.DestinationLocationPtr
	return r
}

// SetDestinationLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationLocation(newValue string) *SnapmirrorInfoType {
	o.DestinationLocationPtr = &newValue
	return o
}

// DestinationVolume is a 'getter' method
func (o *SnapmirrorInfoType) DestinationVolume() string {
	r := *o.DestinationVolumePtr
	return r
}

// SetDestinationVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationVolume(newValue string) *SnapmirrorInfoType {
	o.DestinationVolumePtr = &newValue
	return o
}

// DestinationVolumeNode is a 'getter' method
func (o *SnapmirrorInfoType) DestinationVolumeNode() string {
	r := *o.DestinationVolumeNodePtr
	return r
}

// SetDestinationVolumeNode is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationVolumeNode(newValue string) *SnapmirrorInfoType {
	o.DestinationVolumeNodePtr = &newValue
	return o
}

// DestinationVserver is a 'getter' method
func (o *SnapmirrorInfoType) DestinationVserver() string {
	r := *o.DestinationVserverPtr
	return r
}

// SetDestinationVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationVserver(newValue string) *SnapmirrorInfoType {
	o.DestinationVserverPtr = &newValue
	return o
}

// DestinationVserverUuid is a 'getter' method
func (o *SnapmirrorInfoType) DestinationVserverUuid() string {
	r := *o.DestinationVserverUuidPtr
	return r
}

// SetDestinationVserverUuid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetDestinationVserverUuid(newValue string) *SnapmirrorInfoType {
	o.DestinationVserverUuidPtr = &newValue
	return o
}

// ExportedSnapshot is a 'getter' method
func (o *SnapmirrorInfoType) ExportedSnapshot() string {
	r := *o.ExportedSnapshotPtr
	return r
}

// SetExportedSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetExportedSnapshot(newValue string) *SnapmirrorInfoType {
	o.ExportedSnapshotPtr = &newValue
	return o
}

// ExportedSnapshotTimestamp is a 'getter' method
func (o *SnapmirrorInfoType) ExportedSnapshotTimestamp() uint {
	r := *o.ExportedSnapshotTimestampPtr
	return r
}

// SetExportedSnapshotTimestamp is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetExportedSnapshotTimestamp(newValue uint) *SnapmirrorInfoType {
	o.ExportedSnapshotTimestampPtr = &newValue
	return o
}

// FileRestoreFileCount is a 'getter' method
func (o *SnapmirrorInfoType) FileRestoreFileCount() uint64 {
	r := *o.FileRestoreFileCountPtr
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
	r := *o.FileRestoreFileListPtr
	return r
}

// SetFileRestoreFileList is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetFileRestoreFileList(newValue SnapmirrorInfoTypeFileRestoreFileList) *SnapmirrorInfoType {
	o.FileRestoreFileListPtr = &newValue
	return o
}

// IdentityPreserve is a 'getter' method
func (o *SnapmirrorInfoType) IdentityPreserve() bool {
	r := *o.IdentityPreservePtr
	return r
}

// SetIdentityPreserve is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetIdentityPreserve(newValue bool) *SnapmirrorInfoType {
	o.IdentityPreservePtr = &newValue
	return o
}

// IsAutoExpandEnabled is a 'getter' method
func (o *SnapmirrorInfoType) IsAutoExpandEnabled() bool {
	r := *o.IsAutoExpandEnabledPtr
	return r
}

// SetIsAutoExpandEnabled is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetIsAutoExpandEnabled(newValue bool) *SnapmirrorInfoType {
	o.IsAutoExpandEnabledPtr = &newValue
	return o
}

// IsConstituent is a 'getter' method
func (o *SnapmirrorInfoType) IsConstituent() bool {
	r := *o.IsConstituentPtr
	return r
}

// SetIsConstituent is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetIsConstituent(newValue bool) *SnapmirrorInfoType {
	o.IsConstituentPtr = &newValue
	return o
}

// IsHealthy is a 'getter' method
func (o *SnapmirrorInfoType) IsHealthy() bool {
	r := *o.IsHealthyPtr
	return r
}

// SetIsHealthy is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetIsHealthy(newValue bool) *SnapmirrorInfoType {
	o.IsHealthyPtr = &newValue
	return o
}

// LagTime is a 'getter' method
func (o *SnapmirrorInfoType) LagTime() uint {
	r := *o.LagTimePtr
	return r
}

// SetLagTime is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLagTime(newValue uint) *SnapmirrorInfoType {
	o.LagTimePtr = &newValue
	return o
}

// LastTransferDuration is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferDuration() uint {
	r := *o.LastTransferDurationPtr
	return r
}

// SetLastTransferDuration is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferDuration(newValue uint) *SnapmirrorInfoType {
	o.LastTransferDurationPtr = &newValue
	return o
}

// LastTransferEndTimestamp is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferEndTimestamp() uint {
	r := *o.LastTransferEndTimestampPtr
	return r
}

// SetLastTransferEndTimestamp is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferEndTimestamp(newValue uint) *SnapmirrorInfoType {
	o.LastTransferEndTimestampPtr = &newValue
	return o
}

// LastTransferError is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferError() string {
	r := *o.LastTransferErrorPtr
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
	r := *o.LastTransferErrorCodesPtr
	return r
}

// SetLastTransferErrorCodes is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferErrorCodes(newValue SnapmirrorInfoTypeLastTransferErrorCodes) *SnapmirrorInfoType {
	o.LastTransferErrorCodesPtr = &newValue
	return o
}

// LastTransferFrom is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferFrom() string {
	r := *o.LastTransferFromPtr
	return r
}

// SetLastTransferFrom is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferFrom(newValue string) *SnapmirrorInfoType {
	o.LastTransferFromPtr = &newValue
	return o
}

// LastTransferNetworkCompressionRatio is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferNetworkCompressionRatio() string {
	r := *o.LastTransferNetworkCompressionRatioPtr
	return r
}

// SetLastTransferNetworkCompressionRatio is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferNetworkCompressionRatio(newValue string) *SnapmirrorInfoType {
	o.LastTransferNetworkCompressionRatioPtr = &newValue
	return o
}

// LastTransferSize is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferSize() uint64 {
	r := *o.LastTransferSizePtr
	return r
}

// SetLastTransferSize is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferSize(newValue uint64) *SnapmirrorInfoType {
	o.LastTransferSizePtr = &newValue
	return o
}

// LastTransferType is a 'getter' method
func (o *SnapmirrorInfoType) LastTransferType() string {
	r := *o.LastTransferTypePtr
	return r
}

// SetLastTransferType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetLastTransferType(newValue string) *SnapmirrorInfoType {
	o.LastTransferTypePtr = &newValue
	return o
}

// MaxTransferRate is a 'getter' method
func (o *SnapmirrorInfoType) MaxTransferRate() uint {
	r := *o.MaxTransferRatePtr
	return r
}

// SetMaxTransferRate is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetMaxTransferRate(newValue uint) *SnapmirrorInfoType {
	o.MaxTransferRatePtr = &newValue
	return o
}

// MirrorState is a 'getter' method
func (o *SnapmirrorInfoType) MirrorState() string {
	r := *o.MirrorStatePtr
	return r
}

// SetMirrorState is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetMirrorState(newValue string) *SnapmirrorInfoType {
	o.MirrorStatePtr = &newValue
	return o
}

// NetworkCompressionRatio is a 'getter' method
func (o *SnapmirrorInfoType) NetworkCompressionRatio() string {
	r := *o.NetworkCompressionRatioPtr
	return r
}

// SetNetworkCompressionRatio is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetNetworkCompressionRatio(newValue string) *SnapmirrorInfoType {
	o.NetworkCompressionRatioPtr = &newValue
	return o
}

// NewestSnapshot is a 'getter' method
func (o *SnapmirrorInfoType) NewestSnapshot() string {
	r := *o.NewestSnapshotPtr
	return r
}

// SetNewestSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetNewestSnapshot(newValue string) *SnapmirrorInfoType {
	o.NewestSnapshotPtr = &newValue
	return o
}

// NewestSnapshotTimestamp is a 'getter' method
func (o *SnapmirrorInfoType) NewestSnapshotTimestamp() uint {
	r := *o.NewestSnapshotTimestampPtr
	return r
}

// SetNewestSnapshotTimestamp is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetNewestSnapshotTimestamp(newValue uint) *SnapmirrorInfoType {
	o.NewestSnapshotTimestampPtr = &newValue
	return o
}

// Opmask is a 'getter' method
func (o *SnapmirrorInfoType) Opmask() uint64 {
	r := *o.OpmaskPtr
	return r
}

// SetOpmask is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetOpmask(newValue uint64) *SnapmirrorInfoType {
	o.OpmaskPtr = &newValue
	return o
}

// Policy is a 'getter' method
func (o *SnapmirrorInfoType) Policy() string {
	r := *o.PolicyPtr
	return r
}

// SetPolicy is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetPolicy(newValue string) *SnapmirrorInfoType {
	o.PolicyPtr = &newValue
	return o
}

// PolicyType is a 'getter' method
func (o *SnapmirrorInfoType) PolicyType() string {
	r := *o.PolicyTypePtr
	return r
}

// SetPolicyType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetPolicyType(newValue string) *SnapmirrorInfoType {
	o.PolicyTypePtr = &newValue
	return o
}

// ProgressLastUpdated is a 'getter' method
func (o *SnapmirrorInfoType) ProgressLastUpdated() uint {
	r := *o.ProgressLastUpdatedPtr
	return r
}

// SetProgressLastUpdated is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetProgressLastUpdated(newValue uint) *SnapmirrorInfoType {
	o.ProgressLastUpdatedPtr = &newValue
	return o
}

// RelationshipControlPlane is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipControlPlane() string {
	r := *o.RelationshipControlPlanePtr
	return r
}

// SetRelationshipControlPlane is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipControlPlane(newValue string) *SnapmirrorInfoType {
	o.RelationshipControlPlanePtr = &newValue
	return o
}

// RelationshipGroupType is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipGroupType() string {
	r := *o.RelationshipGroupTypePtr
	return r
}

// SetRelationshipGroupType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipGroupType(newValue string) *SnapmirrorInfoType {
	o.RelationshipGroupTypePtr = &newValue
	return o
}

// RelationshipId is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipId() string {
	r := *o.RelationshipIdPtr
	return r
}

// SetRelationshipId is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipId(newValue string) *SnapmirrorInfoType {
	o.RelationshipIdPtr = &newValue
	return o
}

// RelationshipProgress is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipProgress() uint64 {
	r := *o.RelationshipProgressPtr
	return r
}

// SetRelationshipProgress is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipProgress(newValue uint64) *SnapmirrorInfoType {
	o.RelationshipProgressPtr = &newValue
	return o
}

// RelationshipStatus is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipStatus() string {
	r := *o.RelationshipStatusPtr
	return r
}

// SetRelationshipStatus is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipStatus(newValue string) *SnapmirrorInfoType {
	o.RelationshipStatusPtr = &newValue
	return o
}

// RelationshipType is a 'getter' method
func (o *SnapmirrorInfoType) RelationshipType() string {
	r := *o.RelationshipTypePtr
	return r
}

// SetRelationshipType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetRelationshipType(newValue string) *SnapmirrorInfoType {
	o.RelationshipTypePtr = &newValue
	return o
}

// ResyncFailedCount is a 'getter' method
func (o *SnapmirrorInfoType) ResyncFailedCount() uint64 {
	r := *o.ResyncFailedCountPtr
	return r
}

// SetResyncFailedCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetResyncFailedCount(newValue uint64) *SnapmirrorInfoType {
	o.ResyncFailedCountPtr = &newValue
	return o
}

// ResyncSuccessfulCount is a 'getter' method
func (o *SnapmirrorInfoType) ResyncSuccessfulCount() uint64 {
	r := *o.ResyncSuccessfulCountPtr
	return r
}

// SetResyncSuccessfulCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetResyncSuccessfulCount(newValue uint64) *SnapmirrorInfoType {
	o.ResyncSuccessfulCountPtr = &newValue
	return o
}

// Schedule is a 'getter' method
func (o *SnapmirrorInfoType) Schedule() string {
	r := *o.SchedulePtr
	return r
}

// SetSchedule is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSchedule(newValue string) *SnapmirrorInfoType {
	o.SchedulePtr = &newValue
	return o
}

// SnapshotCheckpoint is a 'getter' method
func (o *SnapmirrorInfoType) SnapshotCheckpoint() uint64 {
	r := *o.SnapshotCheckpointPtr
	return r
}

// SetSnapshotCheckpoint is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSnapshotCheckpoint(newValue uint64) *SnapmirrorInfoType {
	o.SnapshotCheckpointPtr = &newValue
	return o
}

// SnapshotProgress is a 'getter' method
func (o *SnapmirrorInfoType) SnapshotProgress() uint64 {
	r := *o.SnapshotProgressPtr
	return r
}

// SetSnapshotProgress is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSnapshotProgress(newValue uint64) *SnapmirrorInfoType {
	o.SnapshotProgressPtr = &newValue
	return o
}

// SourceCluster is a 'getter' method
func (o *SnapmirrorInfoType) SourceCluster() string {
	r := *o.SourceClusterPtr
	return r
}

// SetSourceCluster is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSourceCluster(newValue string) *SnapmirrorInfoType {
	o.SourceClusterPtr = &newValue
	return o
}

// SourceLocation is a 'getter' method
func (o *SnapmirrorInfoType) SourceLocation() string {
	r := *o.SourceLocationPtr
	return r
}

// SetSourceLocation is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSourceLocation(newValue string) *SnapmirrorInfoType {
	o.SourceLocationPtr = &newValue
	return o
}

// SourceVolume is a 'getter' method
func (o *SnapmirrorInfoType) SourceVolume() string {
	r := *o.SourceVolumePtr
	return r
}

// SetSourceVolume is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSourceVolume(newValue string) *SnapmirrorInfoType {
	o.SourceVolumePtr = &newValue
	return o
}

// SourceVserver is a 'getter' method
func (o *SnapmirrorInfoType) SourceVserver() string {
	r := *o.SourceVserverPtr
	return r
}

// SetSourceVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSourceVserver(newValue string) *SnapmirrorInfoType {
	o.SourceVserverPtr = &newValue
	return o
}

// SourceVserverUuid is a 'getter' method
func (o *SnapmirrorInfoType) SourceVserverUuid() string {
	r := *o.SourceVserverUuidPtr
	return r
}

// SetSourceVserverUuid is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetSourceVserverUuid(newValue string) *SnapmirrorInfoType {
	o.SourceVserverUuidPtr = &newValue
	return o
}

// TotalTransferBytes is a 'getter' method
func (o *SnapmirrorInfoType) TotalTransferBytes() uint64 {
	r := *o.TotalTransferBytesPtr
	return r
}

// SetTotalTransferBytes is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetTotalTransferBytes(newValue uint64) *SnapmirrorInfoType {
	o.TotalTransferBytesPtr = &newValue
	return o
}

// TotalTransferTimeSecs is a 'getter' method
func (o *SnapmirrorInfoType) TotalTransferTimeSecs() uint {
	r := *o.TotalTransferTimeSecsPtr
	return r
}

// SetTotalTransferTimeSecs is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetTotalTransferTimeSecs(newValue uint) *SnapmirrorInfoType {
	o.TotalTransferTimeSecsPtr = &newValue
	return o
}

// TransferSnapshot is a 'getter' method
func (o *SnapmirrorInfoType) TransferSnapshot() string {
	r := *o.TransferSnapshotPtr
	return r
}

// SetTransferSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetTransferSnapshot(newValue string) *SnapmirrorInfoType {
	o.TransferSnapshotPtr = &newValue
	return o
}

// Tries is a 'getter' method
func (o *SnapmirrorInfoType) Tries() string {
	r := *o.TriesPtr
	return r
}

// SetTries is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetTries(newValue string) *SnapmirrorInfoType {
	o.TriesPtr = &newValue
	return o
}

// UnhealthyReason is a 'getter' method
func (o *SnapmirrorInfoType) UnhealthyReason() string {
	r := *o.UnhealthyReasonPtr
	return r
}

// SetUnhealthyReason is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetUnhealthyReason(newValue string) *SnapmirrorInfoType {
	o.UnhealthyReasonPtr = &newValue
	return o
}

// UpdateFailedCount is a 'getter' method
func (o *SnapmirrorInfoType) UpdateFailedCount() uint64 {
	r := *o.UpdateFailedCountPtr
	return r
}

// SetUpdateFailedCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetUpdateFailedCount(newValue uint64) *SnapmirrorInfoType {
	o.UpdateFailedCountPtr = &newValue
	return o
}

// UpdateSuccessfulCount is a 'getter' method
func (o *SnapmirrorInfoType) UpdateSuccessfulCount() uint64 {
	r := *o.UpdateSuccessfulCountPtr
	return r
}

// SetUpdateSuccessfulCount is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetUpdateSuccessfulCount(newValue uint64) *SnapmirrorInfoType {
	o.UpdateSuccessfulCountPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *SnapmirrorInfoType) Vserver() string {
	r := *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *SnapmirrorInfoType) SetVserver(newValue string) *SnapmirrorInfoType {
	o.VserverPtr = &newValue
	return o
}
