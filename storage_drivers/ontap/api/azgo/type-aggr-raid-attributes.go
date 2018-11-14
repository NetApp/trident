package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrRaidAttributesType is a structure to represent a aggr-raid-attributes ZAPI object
type AggrRaidAttributesType struct {
	XMLName               xml.Name                      `xml:"aggr-raid-attributes"`
	AggregateTypePtr      *string                       `xml:"aggregate-type"`
	CacheRaidGroupSizePtr *int                          `xml:"cache-raid-group-size"`
	ChecksumStatusPtr     *string                       `xml:"checksum-status"`
	ChecksumStylePtr      *string                       `xml:"checksum-style"`
	DiskCountPtr          *int                          `xml:"disk-count"`
	EncryptionKeyIdPtr    *string                       `xml:"encryption-key-id"`
	HaPolicyPtr           *string                       `xml:"ha-policy"`
	HasLocalRootPtr       *bool                         `xml:"has-local-root"`
	HasPartnerRootPtr     *bool                         `xml:"has-partner-root"`
	IsChecksumEnabledPtr  *bool                         `xml:"is-checksum-enabled"`
	IsCompositePtr        *bool                         `xml:"is-composite"`
	IsEncryptedPtr        *bool                         `xml:"is-encrypted"`
	IsHybridPtr           *bool                         `xml:"is-hybrid"`
	IsHybridEnabledPtr    *bool                         `xml:"is-hybrid-enabled"`
	IsInconsistentPtr     *bool                         `xml:"is-inconsistent"`
	IsMirroredPtr         *bool                         `xml:"is-mirrored"`
	IsRootAggregatePtr    *bool                         `xml:"is-root-aggregate"`
	MirrorStatusPtr       *string                       `xml:"mirror-status"`
	MountStatePtr         *string                       `xml:"mount-state"`
	PlexCountPtr          *int                          `xml:"plex-count"`
	PlexesPtr             *AggrRaidAttributesTypePlexes `xml:"plexes"`
	// work in progress
	RaidLostWriteStatePtr *string `xml:"raid-lost-write-state"`
	RaidSizePtr           *int    `xml:"raid-size"`
	RaidStatusPtr         *string `xml:"raid-status"`
	RaidTypePtr           *string `xml:"raid-type"`
	StatePtr              *string `xml:"state"`
	UsesSharedDisksPtr    *bool   `xml:"uses-shared-disks"`
}

// NewAggrRaidAttributesType is a factory method for creating new instances of AggrRaidAttributesType objects
func NewAggrRaidAttributesType() *AggrRaidAttributesType {
	return &AggrRaidAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrRaidAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrRaidAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggregateType is a 'getter' method
func (o *AggrRaidAttributesType) AggregateType() string {
	r := *o.AggregateTypePtr
	return r
}

// SetAggregateType is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetAggregateType(newValue string) *AggrRaidAttributesType {
	o.AggregateTypePtr = &newValue
	return o
}

// CacheRaidGroupSize is a 'getter' method
func (o *AggrRaidAttributesType) CacheRaidGroupSize() int {
	r := *o.CacheRaidGroupSizePtr
	return r
}

// SetCacheRaidGroupSize is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetCacheRaidGroupSize(newValue int) *AggrRaidAttributesType {
	o.CacheRaidGroupSizePtr = &newValue
	return o
}

// ChecksumStatus is a 'getter' method
func (o *AggrRaidAttributesType) ChecksumStatus() string {
	r := *o.ChecksumStatusPtr
	return r
}

// SetChecksumStatus is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetChecksumStatus(newValue string) *AggrRaidAttributesType {
	o.ChecksumStatusPtr = &newValue
	return o
}

// ChecksumStyle is a 'getter' method
func (o *AggrRaidAttributesType) ChecksumStyle() string {
	r := *o.ChecksumStylePtr
	return r
}

// SetChecksumStyle is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetChecksumStyle(newValue string) *AggrRaidAttributesType {
	o.ChecksumStylePtr = &newValue
	return o
}

// DiskCount is a 'getter' method
func (o *AggrRaidAttributesType) DiskCount() int {
	r := *o.DiskCountPtr
	return r
}

// SetDiskCount is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetDiskCount(newValue int) *AggrRaidAttributesType {
	o.DiskCountPtr = &newValue
	return o
}

// EncryptionKeyId is a 'getter' method
func (o *AggrRaidAttributesType) EncryptionKeyId() string {
	r := *o.EncryptionKeyIdPtr
	return r
}

// SetEncryptionKeyId is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetEncryptionKeyId(newValue string) *AggrRaidAttributesType {
	o.EncryptionKeyIdPtr = &newValue
	return o
}

// HaPolicy is a 'getter' method
func (o *AggrRaidAttributesType) HaPolicy() string {
	r := *o.HaPolicyPtr
	return r
}

// SetHaPolicy is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetHaPolicy(newValue string) *AggrRaidAttributesType {
	o.HaPolicyPtr = &newValue
	return o
}

// HasLocalRoot is a 'getter' method
func (o *AggrRaidAttributesType) HasLocalRoot() bool {
	r := *o.HasLocalRootPtr
	return r
}

// SetHasLocalRoot is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetHasLocalRoot(newValue bool) *AggrRaidAttributesType {
	o.HasLocalRootPtr = &newValue
	return o
}

// HasPartnerRoot is a 'getter' method
func (o *AggrRaidAttributesType) HasPartnerRoot() bool {
	r := *o.HasPartnerRootPtr
	return r
}

// SetHasPartnerRoot is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetHasPartnerRoot(newValue bool) *AggrRaidAttributesType {
	o.HasPartnerRootPtr = &newValue
	return o
}

// IsChecksumEnabled is a 'getter' method
func (o *AggrRaidAttributesType) IsChecksumEnabled() bool {
	r := *o.IsChecksumEnabledPtr
	return r
}

// SetIsChecksumEnabled is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetIsChecksumEnabled(newValue bool) *AggrRaidAttributesType {
	o.IsChecksumEnabledPtr = &newValue
	return o
}

// IsComposite is a 'getter' method
func (o *AggrRaidAttributesType) IsComposite() bool {
	r := *o.IsCompositePtr
	return r
}

// SetIsComposite is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetIsComposite(newValue bool) *AggrRaidAttributesType {
	o.IsCompositePtr = &newValue
	return o
}

// IsEncrypted is a 'getter' method
func (o *AggrRaidAttributesType) IsEncrypted() bool {
	r := *o.IsEncryptedPtr
	return r
}

// SetIsEncrypted is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetIsEncrypted(newValue bool) *AggrRaidAttributesType {
	o.IsEncryptedPtr = &newValue
	return o
}

// IsHybrid is a 'getter' method
func (o *AggrRaidAttributesType) IsHybrid() bool {
	r := *o.IsHybridPtr
	return r
}

// SetIsHybrid is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetIsHybrid(newValue bool) *AggrRaidAttributesType {
	o.IsHybridPtr = &newValue
	return o
}

// IsHybridEnabled is a 'getter' method
func (o *AggrRaidAttributesType) IsHybridEnabled() bool {
	r := *o.IsHybridEnabledPtr
	return r
}

// SetIsHybridEnabled is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetIsHybridEnabled(newValue bool) *AggrRaidAttributesType {
	o.IsHybridEnabledPtr = &newValue
	return o
}

// IsInconsistent is a 'getter' method
func (o *AggrRaidAttributesType) IsInconsistent() bool {
	r := *o.IsInconsistentPtr
	return r
}

// SetIsInconsistent is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetIsInconsistent(newValue bool) *AggrRaidAttributesType {
	o.IsInconsistentPtr = &newValue
	return o
}

// IsMirrored is a 'getter' method
func (o *AggrRaidAttributesType) IsMirrored() bool {
	r := *o.IsMirroredPtr
	return r
}

// SetIsMirrored is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetIsMirrored(newValue bool) *AggrRaidAttributesType {
	o.IsMirroredPtr = &newValue
	return o
}

// IsRootAggregate is a 'getter' method
func (o *AggrRaidAttributesType) IsRootAggregate() bool {
	r := *o.IsRootAggregatePtr
	return r
}

// SetIsRootAggregate is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetIsRootAggregate(newValue bool) *AggrRaidAttributesType {
	o.IsRootAggregatePtr = &newValue
	return o
}

// MirrorStatus is a 'getter' method
func (o *AggrRaidAttributesType) MirrorStatus() string {
	r := *o.MirrorStatusPtr
	return r
}

// SetMirrorStatus is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetMirrorStatus(newValue string) *AggrRaidAttributesType {
	o.MirrorStatusPtr = &newValue
	return o
}

// MountState is a 'getter' method
func (o *AggrRaidAttributesType) MountState() string {
	r := *o.MountStatePtr
	return r
}

// SetMountState is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetMountState(newValue string) *AggrRaidAttributesType {
	o.MountStatePtr = &newValue
	return o
}

// PlexCount is a 'getter' method
func (o *AggrRaidAttributesType) PlexCount() int {
	r := *o.PlexCountPtr
	return r
}

// SetPlexCount is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetPlexCount(newValue int) *AggrRaidAttributesType {
	o.PlexCountPtr = &newValue
	return o
}

// AggrRaidAttributesTypePlexes is a wrapper
type AggrRaidAttributesTypePlexes struct {
	XMLName           xml.Name             `xml:"plexes"`
	PlexAttributesPtr []PlexAttributesType `xml:"plex-attributes"`
}

// PlexAttributes is a 'getter' method
func (o *AggrRaidAttributesTypePlexes) PlexAttributes() []PlexAttributesType {
	r := o.PlexAttributesPtr
	return r
}

// SetPlexAttributes is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesTypePlexes) SetPlexAttributes(newValue []PlexAttributesType) *AggrRaidAttributesTypePlexes {
	newSlice := make([]PlexAttributesType, len(newValue))
	copy(newSlice, newValue)
	o.PlexAttributesPtr = newSlice
	return o
}

// Plexes is a 'getter' method
func (o *AggrRaidAttributesType) Plexes() AggrRaidAttributesTypePlexes {
	r := *o.PlexesPtr
	return r
}

// SetPlexes is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetPlexes(newValue AggrRaidAttributesTypePlexes) *AggrRaidAttributesType {
	o.PlexesPtr = &newValue
	return o
}

// RaidLostWriteState is a 'getter' method
func (o *AggrRaidAttributesType) RaidLostWriteState() string {
	r := *o.RaidLostWriteStatePtr
	return r
}

// SetRaidLostWriteState is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetRaidLostWriteState(newValue string) *AggrRaidAttributesType {
	o.RaidLostWriteStatePtr = &newValue
	return o
}

// RaidSize is a 'getter' method
func (o *AggrRaidAttributesType) RaidSize() int {
	r := *o.RaidSizePtr
	return r
}

// SetRaidSize is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetRaidSize(newValue int) *AggrRaidAttributesType {
	o.RaidSizePtr = &newValue
	return o
}

// RaidStatus is a 'getter' method
func (o *AggrRaidAttributesType) RaidStatus() string {
	r := *o.RaidStatusPtr
	return r
}

// SetRaidStatus is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetRaidStatus(newValue string) *AggrRaidAttributesType {
	o.RaidStatusPtr = &newValue
	return o
}

// RaidType is a 'getter' method
func (o *AggrRaidAttributesType) RaidType() string {
	r := *o.RaidTypePtr
	return r
}

// SetRaidType is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetRaidType(newValue string) *AggrRaidAttributesType {
	o.RaidTypePtr = &newValue
	return o
}

// State is a 'getter' method
func (o *AggrRaidAttributesType) State() string {
	r := *o.StatePtr
	return r
}

// SetState is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetState(newValue string) *AggrRaidAttributesType {
	o.StatePtr = &newValue
	return o
}

// UsesSharedDisks is a 'getter' method
func (o *AggrRaidAttributesType) UsesSharedDisks() bool {
	r := *o.UsesSharedDisksPtr
	return r
}

// SetUsesSharedDisks is a fluent style 'setter' method that can be chained
func (o *AggrRaidAttributesType) SetUsesSharedDisks(newValue bool) *AggrRaidAttributesType {
	o.UsesSharedDisksPtr = &newValue
	return o
}
