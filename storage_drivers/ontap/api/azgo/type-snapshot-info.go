package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapshotInfoType is a structure to represent a snapshot-info ZAPI object
type SnapshotInfoType struct {
	XMLName                              xml.Name                            `xml:"snapshot-info"`
	AccessTimePtr                        *int                                `xml:"access-time"`
	AfsUsedPtr                           *int                                `xml:"afs-used"`
	BusyPtr                              *bool                               `xml:"busy"`
	CommentPtr                           *string                             `xml:"comment"`
	CompressSavingsPtr                   *int                                `xml:"compress-savings"`
	CompressionTypePtr                   *string                             `xml:"compression-type"`
	ContainsLunClonesPtr                 *bool                               `xml:"contains-lun-clones"`
	CumulativePercentageOfTotalBlocksPtr *int                                `xml:"cumulative-percentage-of-total-blocks"`
	CumulativePercentageOfUsedBlocksPtr  *int                                `xml:"cumulative-percentage-of-used-blocks"`
	CumulativeTotalPtr                   *int                                `xml:"cumulative-total"`
	DedupSavingsPtr                      *int                                `xml:"dedup-savings"`
	DependencyPtr                        *string                             `xml:"dependency"`
	ExpiryTimePtr                        *int                                `xml:"expiry-time"`
	InfiniteSnaplockExpiryTimePtr        *bool                               `xml:"infinite-snaplock-expiry-time"`
	InofileVersionPtr                    *int                                `xml:"inofile-version"`
	Is7ModeSnapshotPtr                   *bool                               `xml:"is-7-mode-snapshot"`
	IsConstituentSnapshotPtr             *bool                               `xml:"is-constituent-snapshot"`
	NamePtr                              *string                             `xml:"name"`
	PercentageOfTotalBlocksPtr           *int                                `xml:"percentage-of-total-blocks"`
	PercentageOfUsedBlocksPtr            *int                                `xml:"percentage-of-used-blocks"`
	SnaplockExpiryTimePtr                *int                                `xml:"snaplock-expiry-time"`
	SnapmirrorLabelPtr                   *string                             `xml:"snapmirror-label"`
	SnapshotInstanceUuidPtr              *UUIDType                           `xml:"snapshot-instance-uuid"`
	SnapshotOwnersListPtr                *SnapshotInfoTypeSnapshotOwnersList `xml:"snapshot-owners-list"`
	// work in progress
	SnapshotVersionUuidPtr  *UUIDType `xml:"snapshot-version-uuid"`
	StatePtr                *string   `xml:"state"`
	TotalPtr                *int      `xml:"total"`
	Vbn0SavingsPtr          *int      `xml:"vbn0-savings"`
	VolumePtr               *string   `xml:"volume"`
	VolumeProvenanceUuidPtr *UUIDType `xml:"volume-provenance-uuid"`
	VserverPtr              *string   `xml:"vserver"`
}

// NewSnapshotInfoType is a factory method for creating new instances of SnapshotInfoType objects
func NewSnapshotInfoType() *SnapshotInfoType {
	return &SnapshotInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *SnapshotInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapshotInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AccessTime is a 'getter' method
func (o *SnapshotInfoType) AccessTime() int {
	r := *o.AccessTimePtr
	return r
}

// SetAccessTime is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetAccessTime(newValue int) *SnapshotInfoType {
	o.AccessTimePtr = &newValue
	return o
}

// AfsUsed is a 'getter' method
func (o *SnapshotInfoType) AfsUsed() int {
	r := *o.AfsUsedPtr
	return r
}

// SetAfsUsed is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetAfsUsed(newValue int) *SnapshotInfoType {
	o.AfsUsedPtr = &newValue
	return o
}

// Busy is a 'getter' method
func (o *SnapshotInfoType) Busy() bool {
	r := *o.BusyPtr
	return r
}

// SetBusy is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetBusy(newValue bool) *SnapshotInfoType {
	o.BusyPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *SnapshotInfoType) Comment() string {
	r := *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetComment(newValue string) *SnapshotInfoType {
	o.CommentPtr = &newValue
	return o
}

// CompressSavings is a 'getter' method
func (o *SnapshotInfoType) CompressSavings() int {
	r := *o.CompressSavingsPtr
	return r
}

// SetCompressSavings is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetCompressSavings(newValue int) *SnapshotInfoType {
	o.CompressSavingsPtr = &newValue
	return o
}

// CompressionType is a 'getter' method
func (o *SnapshotInfoType) CompressionType() string {
	r := *o.CompressionTypePtr
	return r
}

// SetCompressionType is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetCompressionType(newValue string) *SnapshotInfoType {
	o.CompressionTypePtr = &newValue
	return o
}

// ContainsLunClones is a 'getter' method
func (o *SnapshotInfoType) ContainsLunClones() bool {
	r := *o.ContainsLunClonesPtr
	return r
}

// SetContainsLunClones is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetContainsLunClones(newValue bool) *SnapshotInfoType {
	o.ContainsLunClonesPtr = &newValue
	return o
}

// CumulativePercentageOfTotalBlocks is a 'getter' method
func (o *SnapshotInfoType) CumulativePercentageOfTotalBlocks() int {
	r := *o.CumulativePercentageOfTotalBlocksPtr
	return r
}

// SetCumulativePercentageOfTotalBlocks is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetCumulativePercentageOfTotalBlocks(newValue int) *SnapshotInfoType {
	o.CumulativePercentageOfTotalBlocksPtr = &newValue
	return o
}

// CumulativePercentageOfUsedBlocks is a 'getter' method
func (o *SnapshotInfoType) CumulativePercentageOfUsedBlocks() int {
	r := *o.CumulativePercentageOfUsedBlocksPtr
	return r
}

// SetCumulativePercentageOfUsedBlocks is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetCumulativePercentageOfUsedBlocks(newValue int) *SnapshotInfoType {
	o.CumulativePercentageOfUsedBlocksPtr = &newValue
	return o
}

// CumulativeTotal is a 'getter' method
func (o *SnapshotInfoType) CumulativeTotal() int {
	r := *o.CumulativeTotalPtr
	return r
}

// SetCumulativeTotal is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetCumulativeTotal(newValue int) *SnapshotInfoType {
	o.CumulativeTotalPtr = &newValue
	return o
}

// DedupSavings is a 'getter' method
func (o *SnapshotInfoType) DedupSavings() int {
	r := *o.DedupSavingsPtr
	return r
}

// SetDedupSavings is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetDedupSavings(newValue int) *SnapshotInfoType {
	o.DedupSavingsPtr = &newValue
	return o
}

// Dependency is a 'getter' method
func (o *SnapshotInfoType) Dependency() string {
	r := *o.DependencyPtr
	return r
}

// SetDependency is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetDependency(newValue string) *SnapshotInfoType {
	o.DependencyPtr = &newValue
	return o
}

// ExpiryTime is a 'getter' method
func (o *SnapshotInfoType) ExpiryTime() int {
	r := *o.ExpiryTimePtr
	return r
}

// SetExpiryTime is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetExpiryTime(newValue int) *SnapshotInfoType {
	o.ExpiryTimePtr = &newValue
	return o
}

// InfiniteSnaplockExpiryTime is a 'getter' method
func (o *SnapshotInfoType) InfiniteSnaplockExpiryTime() bool {
	r := *o.InfiniteSnaplockExpiryTimePtr
	return r
}

// SetInfiniteSnaplockExpiryTime is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetInfiniteSnaplockExpiryTime(newValue bool) *SnapshotInfoType {
	o.InfiniteSnaplockExpiryTimePtr = &newValue
	return o
}

// InofileVersion is a 'getter' method
func (o *SnapshotInfoType) InofileVersion() int {
	r := *o.InofileVersionPtr
	return r
}

// SetInofileVersion is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetInofileVersion(newValue int) *SnapshotInfoType {
	o.InofileVersionPtr = &newValue
	return o
}

// Is7ModeSnapshot is a 'getter' method
func (o *SnapshotInfoType) Is7ModeSnapshot() bool {
	r := *o.Is7ModeSnapshotPtr
	return r
}

// SetIs7ModeSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetIs7ModeSnapshot(newValue bool) *SnapshotInfoType {
	o.Is7ModeSnapshotPtr = &newValue
	return o
}

// IsConstituentSnapshot is a 'getter' method
func (o *SnapshotInfoType) IsConstituentSnapshot() bool {
	r := *o.IsConstituentSnapshotPtr
	return r
}

// SetIsConstituentSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetIsConstituentSnapshot(newValue bool) *SnapshotInfoType {
	o.IsConstituentSnapshotPtr = &newValue
	return o
}

// Name is a 'getter' method
func (o *SnapshotInfoType) Name() string {
	r := *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetName(newValue string) *SnapshotInfoType {
	o.NamePtr = &newValue
	return o
}

// PercentageOfTotalBlocks is a 'getter' method
func (o *SnapshotInfoType) PercentageOfTotalBlocks() int {
	r := *o.PercentageOfTotalBlocksPtr
	return r
}

// SetPercentageOfTotalBlocks is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetPercentageOfTotalBlocks(newValue int) *SnapshotInfoType {
	o.PercentageOfTotalBlocksPtr = &newValue
	return o
}

// PercentageOfUsedBlocks is a 'getter' method
func (o *SnapshotInfoType) PercentageOfUsedBlocks() int {
	r := *o.PercentageOfUsedBlocksPtr
	return r
}

// SetPercentageOfUsedBlocks is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetPercentageOfUsedBlocks(newValue int) *SnapshotInfoType {
	o.PercentageOfUsedBlocksPtr = &newValue
	return o
}

// SnaplockExpiryTime is a 'getter' method
func (o *SnapshotInfoType) SnaplockExpiryTime() int {
	r := *o.SnaplockExpiryTimePtr
	return r
}

// SetSnaplockExpiryTime is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetSnaplockExpiryTime(newValue int) *SnapshotInfoType {
	o.SnaplockExpiryTimePtr = &newValue
	return o
}

// SnapmirrorLabel is a 'getter' method
func (o *SnapshotInfoType) SnapmirrorLabel() string {
	r := *o.SnapmirrorLabelPtr
	return r
}

// SetSnapmirrorLabel is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetSnapmirrorLabel(newValue string) *SnapshotInfoType {
	o.SnapmirrorLabelPtr = &newValue
	return o
}

// SnapshotInstanceUuid is a 'getter' method
func (o *SnapshotInfoType) SnapshotInstanceUuid() UUIDType {
	r := *o.SnapshotInstanceUuidPtr
	return r
}

// SetSnapshotInstanceUuid is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetSnapshotInstanceUuid(newValue UUIDType) *SnapshotInfoType {
	o.SnapshotInstanceUuidPtr = &newValue
	return o
}

// SnapshotInfoTypeSnapshotOwnersList is a wrapper
type SnapshotInfoTypeSnapshotOwnersList struct {
	XMLName          xml.Name            `xml:"snapshot-owners-list"`
	SnapshotOwnerPtr []SnapshotOwnerType `xml:"snapshot-owner"`
}

// SnapshotOwner is a 'getter' method
func (o *SnapshotInfoTypeSnapshotOwnersList) SnapshotOwner() []SnapshotOwnerType {
	r := o.SnapshotOwnerPtr
	return r
}

// SetSnapshotOwner is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoTypeSnapshotOwnersList) SetSnapshotOwner(newValue []SnapshotOwnerType) *SnapshotInfoTypeSnapshotOwnersList {
	newSlice := make([]SnapshotOwnerType, len(newValue))
	copy(newSlice, newValue)
	o.SnapshotOwnerPtr = newSlice
	return o
}

// SnapshotOwnersList is a 'getter' method
func (o *SnapshotInfoType) SnapshotOwnersList() SnapshotInfoTypeSnapshotOwnersList {
	r := *o.SnapshotOwnersListPtr
	return r
}

// SetSnapshotOwnersList is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetSnapshotOwnersList(newValue SnapshotInfoTypeSnapshotOwnersList) *SnapshotInfoType {
	o.SnapshotOwnersListPtr = &newValue
	return o
}

// SnapshotVersionUuid is a 'getter' method
func (o *SnapshotInfoType) SnapshotVersionUuid() UUIDType {
	r := *o.SnapshotVersionUuidPtr
	return r
}

// SetSnapshotVersionUuid is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetSnapshotVersionUuid(newValue UUIDType) *SnapshotInfoType {
	o.SnapshotVersionUuidPtr = &newValue
	return o
}

// State is a 'getter' method
func (o *SnapshotInfoType) State() string {
	r := *o.StatePtr
	return r
}

// SetState is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetState(newValue string) *SnapshotInfoType {
	o.StatePtr = &newValue
	return o
}

// Total is a 'getter' method
func (o *SnapshotInfoType) Total() int {
	r := *o.TotalPtr
	return r
}

// SetTotal is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetTotal(newValue int) *SnapshotInfoType {
	o.TotalPtr = &newValue
	return o
}

// Vbn0Savings is a 'getter' method
func (o *SnapshotInfoType) Vbn0Savings() int {
	r := *o.Vbn0SavingsPtr
	return r
}

// SetVbn0Savings is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetVbn0Savings(newValue int) *SnapshotInfoType {
	o.Vbn0SavingsPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *SnapshotInfoType) Volume() string {
	r := *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetVolume(newValue string) *SnapshotInfoType {
	o.VolumePtr = &newValue
	return o
}

// VolumeProvenanceUuid is a 'getter' method
func (o *SnapshotInfoType) VolumeProvenanceUuid() UUIDType {
	r := *o.VolumeProvenanceUuidPtr
	return r
}

// SetVolumeProvenanceUuid is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetVolumeProvenanceUuid(newValue UUIDType) *SnapshotInfoType {
	o.VolumeProvenanceUuidPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *SnapshotInfoType) Vserver() string {
	r := *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *SnapshotInfoType) SetVserver(newValue string) *SnapshotInfoType {
	o.VserverPtr = &newValue
	return o
}
