// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SpaceInformationType is a structure to represent a space-information ZAPI object
type SpaceInformationType struct {
	XMLName                                 xml.Name `xml:"space-information"`
	AggregatePtr                            *string  `xml:"aggregate"`
	AggregateMetadataPtr                    *int     `xml:"aggregate-metadata"`
	AggregateMetadataPercentPtr             *int     `xml:"aggregate-metadata-percent"`
	AggregateSizePtr                        *int     `xml:"aggregate-size"`
	ObjectStoreMetadataPtr                  *int     `xml:"object-store-metadata"`
	ObjectStoreMetadataPercentPtr           *int     `xml:"object-store-metadata-percent"`
	ObjectStorePhysicalUsedPtr              *int     `xml:"object-store-physical-used"`
	ObjectStorePhysicalUsedPercentPtr       *int     `xml:"object-store-physical-used-percent"`
	ObjectStoreReferencedCapacityPtr        *int     `xml:"object-store-referenced-capacity"`
	ObjectStoreReferencedCapacityPercentPtr *int     `xml:"object-store-referenced-capacity-percent"`
	ObjectStoreSisSpaceSavedPtr             *int     `xml:"object-store-sis-space-saved"`
	ObjectStoreSisSpaceSavedPercentPtr      *int     `xml:"object-store-sis-space-saved-percent"`
	ObjectStoreSizePtr                      *int     `xml:"object-store-size"`
	ObjectStoreUnreclaimedSpacePtr          *int     `xml:"object-store-unreclaimed-space"`
	ObjectStoreUnreclaimedSpacePercentPtr   *int     `xml:"object-store-unreclaimed-space-percent"`
	PercentSnapshotSpacePtr                 *int     `xml:"percent-snapshot-space"`
	PhysicalUsedPtr                         *int     `xml:"physical-used"`
	PhysicalUsedPercentPtr                  *int     `xml:"physical-used-percent"`
	SnapSizeTotalPtr                        *int     `xml:"snap-size-total"`
	SnapshotReserveUnusablePtr              *int     `xml:"snapshot-reserve-unusable"`
	SnapshotReserveUnusablePercentPtr       *int     `xml:"snapshot-reserve-unusable-percent"`
	TierNamePtr                             *string  `xml:"tier-name"`
	UsedIncludingSnapshotReservePtr         *int     `xml:"used-including-snapshot-reserve"`
	UsedIncludingSnapshotReservePercentPtr  *int     `xml:"used-including-snapshot-reserve-percent"`
	VolumeFootprintsPtr                     *int     `xml:"volume-footprints"`
	VolumeFootprintsPercentPtr              *int     `xml:"volume-footprints-percent"`
}

// NewSpaceInformationType is a factory method for creating new instances of SpaceInformationType objects
func NewSpaceInformationType() *SpaceInformationType {
	return &SpaceInformationType{}
}

// ToXML converts this object into an xml string representation
func (o *SpaceInformationType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SpaceInformationType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Aggregate is a 'getter' method
func (o *SpaceInformationType) Aggregate() string {
	var r string
	if o.AggregatePtr == nil {
		return r
	}
	r = *o.AggregatePtr
	return r
}

// SetAggregate is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetAggregate(newValue string) *SpaceInformationType {
	o.AggregatePtr = &newValue
	return o
}

// AggregateMetadata is a 'getter' method
func (o *SpaceInformationType) AggregateMetadata() int {
	var r int
	if o.AggregateMetadataPtr == nil {
		return r
	}
	r = *o.AggregateMetadataPtr
	return r
}

// SetAggregateMetadata is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetAggregateMetadata(newValue int) *SpaceInformationType {
	o.AggregateMetadataPtr = &newValue
	return o
}

// AggregateMetadataPercent is a 'getter' method
func (o *SpaceInformationType) AggregateMetadataPercent() int {
	var r int
	if o.AggregateMetadataPercentPtr == nil {
		return r
	}
	r = *o.AggregateMetadataPercentPtr
	return r
}

// SetAggregateMetadataPercent is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetAggregateMetadataPercent(newValue int) *SpaceInformationType {
	o.AggregateMetadataPercentPtr = &newValue
	return o
}

// AggregateSize is a 'getter' method
func (o *SpaceInformationType) AggregateSize() int {
	var r int
	if o.AggregateSizePtr == nil {
		return r
	}
	r = *o.AggregateSizePtr
	return r
}

// SetAggregateSize is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetAggregateSize(newValue int) *SpaceInformationType {
	o.AggregateSizePtr = &newValue
	return o
}

// ObjectStoreMetadata is a 'getter' method
func (o *SpaceInformationType) ObjectStoreMetadata() int {
	var r int
	if o.ObjectStoreMetadataPtr == nil {
		return r
	}
	r = *o.ObjectStoreMetadataPtr
	return r
}

// SetObjectStoreMetadata is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetObjectStoreMetadata(newValue int) *SpaceInformationType {
	o.ObjectStoreMetadataPtr = &newValue
	return o
}

// ObjectStoreMetadataPercent is a 'getter' method
func (o *SpaceInformationType) ObjectStoreMetadataPercent() int {
	var r int
	if o.ObjectStoreMetadataPercentPtr == nil {
		return r
	}
	r = *o.ObjectStoreMetadataPercentPtr
	return r
}

// SetObjectStoreMetadataPercent is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetObjectStoreMetadataPercent(newValue int) *SpaceInformationType {
	o.ObjectStoreMetadataPercentPtr = &newValue
	return o
}

// ObjectStorePhysicalUsed is a 'getter' method
func (o *SpaceInformationType) ObjectStorePhysicalUsed() int {
	var r int
	if o.ObjectStorePhysicalUsedPtr == nil {
		return r
	}
	r = *o.ObjectStorePhysicalUsedPtr
	return r
}

// SetObjectStorePhysicalUsed is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetObjectStorePhysicalUsed(newValue int) *SpaceInformationType {
	o.ObjectStorePhysicalUsedPtr = &newValue
	return o
}

// ObjectStorePhysicalUsedPercent is a 'getter' method
func (o *SpaceInformationType) ObjectStorePhysicalUsedPercent() int {
	var r int
	if o.ObjectStorePhysicalUsedPercentPtr == nil {
		return r
	}
	r = *o.ObjectStorePhysicalUsedPercentPtr
	return r
}

// SetObjectStorePhysicalUsedPercent is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetObjectStorePhysicalUsedPercent(newValue int) *SpaceInformationType {
	o.ObjectStorePhysicalUsedPercentPtr = &newValue
	return o
}

// ObjectStoreReferencedCapacity is a 'getter' method
func (o *SpaceInformationType) ObjectStoreReferencedCapacity() int {
	var r int
	if o.ObjectStoreReferencedCapacityPtr == nil {
		return r
	}
	r = *o.ObjectStoreReferencedCapacityPtr
	return r
}

// SetObjectStoreReferencedCapacity is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetObjectStoreReferencedCapacity(newValue int) *SpaceInformationType {
	o.ObjectStoreReferencedCapacityPtr = &newValue
	return o
}

// ObjectStoreReferencedCapacityPercent is a 'getter' method
func (o *SpaceInformationType) ObjectStoreReferencedCapacityPercent() int {
	var r int
	if o.ObjectStoreReferencedCapacityPercentPtr == nil {
		return r
	}
	r = *o.ObjectStoreReferencedCapacityPercentPtr
	return r
}

// SetObjectStoreReferencedCapacityPercent is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetObjectStoreReferencedCapacityPercent(newValue int) *SpaceInformationType {
	o.ObjectStoreReferencedCapacityPercentPtr = &newValue
	return o
}

// ObjectStoreSisSpaceSaved is a 'getter' method
func (o *SpaceInformationType) ObjectStoreSisSpaceSaved() int {
	var r int
	if o.ObjectStoreSisSpaceSavedPtr == nil {
		return r
	}
	r = *o.ObjectStoreSisSpaceSavedPtr
	return r
}

// SetObjectStoreSisSpaceSaved is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetObjectStoreSisSpaceSaved(newValue int) *SpaceInformationType {
	o.ObjectStoreSisSpaceSavedPtr = &newValue
	return o
}

// ObjectStoreSisSpaceSavedPercent is a 'getter' method
func (o *SpaceInformationType) ObjectStoreSisSpaceSavedPercent() int {
	var r int
	if o.ObjectStoreSisSpaceSavedPercentPtr == nil {
		return r
	}
	r = *o.ObjectStoreSisSpaceSavedPercentPtr
	return r
}

// SetObjectStoreSisSpaceSavedPercent is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetObjectStoreSisSpaceSavedPercent(newValue int) *SpaceInformationType {
	o.ObjectStoreSisSpaceSavedPercentPtr = &newValue
	return o
}

// ObjectStoreSize is a 'getter' method
func (o *SpaceInformationType) ObjectStoreSize() int {
	var r int
	if o.ObjectStoreSizePtr == nil {
		return r
	}
	r = *o.ObjectStoreSizePtr
	return r
}

// SetObjectStoreSize is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetObjectStoreSize(newValue int) *SpaceInformationType {
	o.ObjectStoreSizePtr = &newValue
	return o
}

// ObjectStoreUnreclaimedSpace is a 'getter' method
func (o *SpaceInformationType) ObjectStoreUnreclaimedSpace() int {
	var r int
	if o.ObjectStoreUnreclaimedSpacePtr == nil {
		return r
	}
	r = *o.ObjectStoreUnreclaimedSpacePtr
	return r
}

// SetObjectStoreUnreclaimedSpace is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetObjectStoreUnreclaimedSpace(newValue int) *SpaceInformationType {
	o.ObjectStoreUnreclaimedSpacePtr = &newValue
	return o
}

// ObjectStoreUnreclaimedSpacePercent is a 'getter' method
func (o *SpaceInformationType) ObjectStoreUnreclaimedSpacePercent() int {
	var r int
	if o.ObjectStoreUnreclaimedSpacePercentPtr == nil {
		return r
	}
	r = *o.ObjectStoreUnreclaimedSpacePercentPtr
	return r
}

// SetObjectStoreUnreclaimedSpacePercent is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetObjectStoreUnreclaimedSpacePercent(newValue int) *SpaceInformationType {
	o.ObjectStoreUnreclaimedSpacePercentPtr = &newValue
	return o
}

// PercentSnapshotSpace is a 'getter' method
func (o *SpaceInformationType) PercentSnapshotSpace() int {
	var r int
	if o.PercentSnapshotSpacePtr == nil {
		return r
	}
	r = *o.PercentSnapshotSpacePtr
	return r
}

// SetPercentSnapshotSpace is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetPercentSnapshotSpace(newValue int) *SpaceInformationType {
	o.PercentSnapshotSpacePtr = &newValue
	return o
}

// PhysicalUsed is a 'getter' method
func (o *SpaceInformationType) PhysicalUsed() int {
	var r int
	if o.PhysicalUsedPtr == nil {
		return r
	}
	r = *o.PhysicalUsedPtr
	return r
}

// SetPhysicalUsed is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetPhysicalUsed(newValue int) *SpaceInformationType {
	o.PhysicalUsedPtr = &newValue
	return o
}

// PhysicalUsedPercent is a 'getter' method
func (o *SpaceInformationType) PhysicalUsedPercent() int {
	var r int
	if o.PhysicalUsedPercentPtr == nil {
		return r
	}
	r = *o.PhysicalUsedPercentPtr
	return r
}

// SetPhysicalUsedPercent is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetPhysicalUsedPercent(newValue int) *SpaceInformationType {
	o.PhysicalUsedPercentPtr = &newValue
	return o
}

// SnapSizeTotal is a 'getter' method
func (o *SpaceInformationType) SnapSizeTotal() int {
	var r int
	if o.SnapSizeTotalPtr == nil {
		return r
	}
	r = *o.SnapSizeTotalPtr
	return r
}

// SetSnapSizeTotal is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetSnapSizeTotal(newValue int) *SpaceInformationType {
	o.SnapSizeTotalPtr = &newValue
	return o
}

// SnapshotReserveUnusable is a 'getter' method
func (o *SpaceInformationType) SnapshotReserveUnusable() int {
	var r int
	if o.SnapshotReserveUnusablePtr == nil {
		return r
	}
	r = *o.SnapshotReserveUnusablePtr
	return r
}

// SetSnapshotReserveUnusable is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetSnapshotReserveUnusable(newValue int) *SpaceInformationType {
	o.SnapshotReserveUnusablePtr = &newValue
	return o
}

// SnapshotReserveUnusablePercent is a 'getter' method
func (o *SpaceInformationType) SnapshotReserveUnusablePercent() int {
	var r int
	if o.SnapshotReserveUnusablePercentPtr == nil {
		return r
	}
	r = *o.SnapshotReserveUnusablePercentPtr
	return r
}

// SetSnapshotReserveUnusablePercent is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetSnapshotReserveUnusablePercent(newValue int) *SpaceInformationType {
	o.SnapshotReserveUnusablePercentPtr = &newValue
	return o
}

// TierName is a 'getter' method
func (o *SpaceInformationType) TierName() string {
	var r string
	if o.TierNamePtr == nil {
		return r
	}
	r = *o.TierNamePtr
	return r
}

// SetTierName is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetTierName(newValue string) *SpaceInformationType {
	o.TierNamePtr = &newValue
	return o
}

// UsedIncludingSnapshotReserve is a 'getter' method
func (o *SpaceInformationType) UsedIncludingSnapshotReserve() int {
	var r int
	if o.UsedIncludingSnapshotReservePtr == nil {
		return r
	}
	r = *o.UsedIncludingSnapshotReservePtr
	return r
}

// SetUsedIncludingSnapshotReserve is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetUsedIncludingSnapshotReserve(newValue int) *SpaceInformationType {
	o.UsedIncludingSnapshotReservePtr = &newValue
	return o
}

// UsedIncludingSnapshotReservePercent is a 'getter' method
func (o *SpaceInformationType) UsedIncludingSnapshotReservePercent() int {
	var r int
	if o.UsedIncludingSnapshotReservePercentPtr == nil {
		return r
	}
	r = *o.UsedIncludingSnapshotReservePercentPtr
	return r
}

// SetUsedIncludingSnapshotReservePercent is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetUsedIncludingSnapshotReservePercent(newValue int) *SpaceInformationType {
	o.UsedIncludingSnapshotReservePercentPtr = &newValue
	return o
}

// VolumeFootprints is a 'getter' method
func (o *SpaceInformationType) VolumeFootprints() int {
	var r int
	if o.VolumeFootprintsPtr == nil {
		return r
	}
	r = *o.VolumeFootprintsPtr
	return r
}

// SetVolumeFootprints is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetVolumeFootprints(newValue int) *SpaceInformationType {
	o.VolumeFootprintsPtr = &newValue
	return o
}

// VolumeFootprintsPercent is a 'getter' method
func (o *SpaceInformationType) VolumeFootprintsPercent() int {
	var r int
	if o.VolumeFootprintsPercentPtr == nil {
		return r
	}
	r = *o.VolumeFootprintsPercentPtr
	return r
}

// SetVolumeFootprintsPercent is a fluent style 'setter' method that can be chained
func (o *SpaceInformationType) SetVolumeFootprintsPercent(newValue int) *SpaceInformationType {
	o.VolumeFootprintsPercentPtr = &newValue
	return o
}
