// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrSpaceAttributesType is a structure to represent a aggr-space-attributes ZAPI object
type AggrSpaceAttributesType struct {
	XMLName                                   xml.Name `xml:"aggr-space-attributes"`
	AggregateMetadataPtr                      *int     `xml:"aggregate-metadata"`
	CapacityTierUsedPtr                       *int     `xml:"capacity-tier-used"`
	DataCompactedCountPtr                     *int     `xml:"data-compacted-count"`
	DataCompactionSpaceSavedPtr               *int     `xml:"data-compaction-space-saved"`
	DataCompactionSpaceSavedPercentPtr        *int     `xml:"data-compaction-space-saved-percent"`
	HybridCacheSizeTotalPtr                   *int     `xml:"hybrid-cache-size-total"`
	PercentUsedCapacityPtr                    *int     `xml:"percent-used-capacity"`
	PerformanceTierInactiveUserDataPtr        *int     `xml:"performance-tier-inactive-user-data"`
	PerformanceTierInactiveUserDataPercentPtr *int     `xml:"performance-tier-inactive-user-data-percent"`
	PhysicalUsedPtr                           *int     `xml:"physical-used"`
	PhysicalUsedPercentPtr                    *int     `xml:"physical-used-percent"`
	SisSharedCountPtr                         *int     `xml:"sis-shared-count"`
	SisSpaceSavedPtr                          *int     `xml:"sis-space-saved"`
	SisSpaceSavedPercentPtr                   *int     `xml:"sis-space-saved-percent"`
	SizeAvailablePtr                          *int     `xml:"size-available"`
	SizeTotalPtr                              *int     `xml:"size-total"`
	SizeUsedPtr                               *int     `xml:"size-used"`
	TotalReservedSpacePtr                     *int     `xml:"total-reserved-space"`
	UsedIncludingSnapshotReservePtr           *int     `xml:"used-including-snapshot-reserve"`
	VolumeFootprintsPtr                       *int     `xml:"volume-footprints"`
}

// NewAggrSpaceAttributesType is a factory method for creating new instances of AggrSpaceAttributesType objects
func NewAggrSpaceAttributesType() *AggrSpaceAttributesType {
	return &AggrSpaceAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrSpaceAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrSpaceAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AggregateMetadata is a 'getter' method
func (o *AggrSpaceAttributesType) AggregateMetadata() int {
	r := *o.AggregateMetadataPtr
	return r
}

// SetAggregateMetadata is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetAggregateMetadata(newValue int) *AggrSpaceAttributesType {
	o.AggregateMetadataPtr = &newValue
	return o
}

// CapacityTierUsed is a 'getter' method
func (o *AggrSpaceAttributesType) CapacityTierUsed() int {
	r := *o.CapacityTierUsedPtr
	return r
}

// SetCapacityTierUsed is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetCapacityTierUsed(newValue int) *AggrSpaceAttributesType {
	o.CapacityTierUsedPtr = &newValue
	return o
}

// DataCompactedCount is a 'getter' method
func (o *AggrSpaceAttributesType) DataCompactedCount() int {
	r := *o.DataCompactedCountPtr
	return r
}

// SetDataCompactedCount is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetDataCompactedCount(newValue int) *AggrSpaceAttributesType {
	o.DataCompactedCountPtr = &newValue
	return o
}

// DataCompactionSpaceSaved is a 'getter' method
func (o *AggrSpaceAttributesType) DataCompactionSpaceSaved() int {
	r := *o.DataCompactionSpaceSavedPtr
	return r
}

// SetDataCompactionSpaceSaved is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetDataCompactionSpaceSaved(newValue int) *AggrSpaceAttributesType {
	o.DataCompactionSpaceSavedPtr = &newValue
	return o
}

// DataCompactionSpaceSavedPercent is a 'getter' method
func (o *AggrSpaceAttributesType) DataCompactionSpaceSavedPercent() int {
	r := *o.DataCompactionSpaceSavedPercentPtr
	return r
}

// SetDataCompactionSpaceSavedPercent is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetDataCompactionSpaceSavedPercent(newValue int) *AggrSpaceAttributesType {
	o.DataCompactionSpaceSavedPercentPtr = &newValue
	return o
}

// HybridCacheSizeTotal is a 'getter' method
func (o *AggrSpaceAttributesType) HybridCacheSizeTotal() int {
	r := *o.HybridCacheSizeTotalPtr
	return r
}

// SetHybridCacheSizeTotal is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetHybridCacheSizeTotal(newValue int) *AggrSpaceAttributesType {
	o.HybridCacheSizeTotalPtr = &newValue
	return o
}

// PercentUsedCapacity is a 'getter' method
func (o *AggrSpaceAttributesType) PercentUsedCapacity() int {
	r := *o.PercentUsedCapacityPtr
	return r
}

// SetPercentUsedCapacity is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetPercentUsedCapacity(newValue int) *AggrSpaceAttributesType {
	o.PercentUsedCapacityPtr = &newValue
	return o
}

// PerformanceTierInactiveUserData is a 'getter' method
func (o *AggrSpaceAttributesType) PerformanceTierInactiveUserData() int {
	r := *o.PerformanceTierInactiveUserDataPtr
	return r
}

// SetPerformanceTierInactiveUserData is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetPerformanceTierInactiveUserData(newValue int) *AggrSpaceAttributesType {
	o.PerformanceTierInactiveUserDataPtr = &newValue
	return o
}

// PerformanceTierInactiveUserDataPercent is a 'getter' method
func (o *AggrSpaceAttributesType) PerformanceTierInactiveUserDataPercent() int {
	r := *o.PerformanceTierInactiveUserDataPercentPtr
	return r
}

// SetPerformanceTierInactiveUserDataPercent is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetPerformanceTierInactiveUserDataPercent(newValue int) *AggrSpaceAttributesType {
	o.PerformanceTierInactiveUserDataPercentPtr = &newValue
	return o
}

// PhysicalUsed is a 'getter' method
func (o *AggrSpaceAttributesType) PhysicalUsed() int {
	r := *o.PhysicalUsedPtr
	return r
}

// SetPhysicalUsed is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetPhysicalUsed(newValue int) *AggrSpaceAttributesType {
	o.PhysicalUsedPtr = &newValue
	return o
}

// PhysicalUsedPercent is a 'getter' method
func (o *AggrSpaceAttributesType) PhysicalUsedPercent() int {
	r := *o.PhysicalUsedPercentPtr
	return r
}

// SetPhysicalUsedPercent is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetPhysicalUsedPercent(newValue int) *AggrSpaceAttributesType {
	o.PhysicalUsedPercentPtr = &newValue
	return o
}

// SisSharedCount is a 'getter' method
func (o *AggrSpaceAttributesType) SisSharedCount() int {
	r := *o.SisSharedCountPtr
	return r
}

// SetSisSharedCount is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetSisSharedCount(newValue int) *AggrSpaceAttributesType {
	o.SisSharedCountPtr = &newValue
	return o
}

// SisSpaceSaved is a 'getter' method
func (o *AggrSpaceAttributesType) SisSpaceSaved() int {
	r := *o.SisSpaceSavedPtr
	return r
}

// SetSisSpaceSaved is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetSisSpaceSaved(newValue int) *AggrSpaceAttributesType {
	o.SisSpaceSavedPtr = &newValue
	return o
}

// SisSpaceSavedPercent is a 'getter' method
func (o *AggrSpaceAttributesType) SisSpaceSavedPercent() int {
	r := *o.SisSpaceSavedPercentPtr
	return r
}

// SetSisSpaceSavedPercent is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetSisSpaceSavedPercent(newValue int) *AggrSpaceAttributesType {
	o.SisSpaceSavedPercentPtr = &newValue
	return o
}

// SizeAvailable is a 'getter' method
func (o *AggrSpaceAttributesType) SizeAvailable() int {
	r := *o.SizeAvailablePtr
	return r
}

// SetSizeAvailable is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetSizeAvailable(newValue int) *AggrSpaceAttributesType {
	o.SizeAvailablePtr = &newValue
	return o
}

// SizeTotal is a 'getter' method
func (o *AggrSpaceAttributesType) SizeTotal() int {
	r := *o.SizeTotalPtr
	return r
}

// SetSizeTotal is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetSizeTotal(newValue int) *AggrSpaceAttributesType {
	o.SizeTotalPtr = &newValue
	return o
}

// SizeUsed is a 'getter' method
func (o *AggrSpaceAttributesType) SizeUsed() int {
	r := *o.SizeUsedPtr
	return r
}

// SetSizeUsed is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetSizeUsed(newValue int) *AggrSpaceAttributesType {
	o.SizeUsedPtr = &newValue
	return o
}

// TotalReservedSpace is a 'getter' method
func (o *AggrSpaceAttributesType) TotalReservedSpace() int {
	r := *o.TotalReservedSpacePtr
	return r
}

// SetTotalReservedSpace is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetTotalReservedSpace(newValue int) *AggrSpaceAttributesType {
	o.TotalReservedSpacePtr = &newValue
	return o
}

// UsedIncludingSnapshotReserve is a 'getter' method
func (o *AggrSpaceAttributesType) UsedIncludingSnapshotReserve() int {
	r := *o.UsedIncludingSnapshotReservePtr
	return r
}

// SetUsedIncludingSnapshotReserve is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetUsedIncludingSnapshotReserve(newValue int) *AggrSpaceAttributesType {
	o.UsedIncludingSnapshotReservePtr = &newValue
	return o
}

// VolumeFootprints is a 'getter' method
func (o *AggrSpaceAttributesType) VolumeFootprints() int {
	r := *o.VolumeFootprintsPtr
	return r
}

// SetVolumeFootprints is a fluent style 'setter' method that can be chained
func (o *AggrSpaceAttributesType) SetVolumeFootprints(newValue int) *AggrSpaceAttributesType {
	o.VolumeFootprintsPtr = &newValue
	return o
}
