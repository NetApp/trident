// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeSnapshotAutodeleteAttributesType is a structure to represent a volume-snapshot-autodelete-attributes ZAPI object
type VolumeSnapshotAutodeleteAttributesType struct {
	XMLName                xml.Name `xml:"volume-snapshot-autodelete-attributes"`
	CommitmentPtr          *string  `xml:"commitment"`
	DeferDeletePtr         *string  `xml:"defer-delete"`
	DeleteOrderPtr         *string  `xml:"delete-order"`
	DestroyListPtr         *string  `xml:"destroy-list"`
	IsAutodeleteEnabledPtr *bool    `xml:"is-autodelete-enabled"`
	PrefixPtr              *string  `xml:"prefix"`
	TargetFreeSpacePtr     *int     `xml:"target-free-space"`
	TriggerPtr             *string  `xml:"trigger"`
}

// NewVolumeSnapshotAutodeleteAttributesType is a factory method for creating new instances of VolumeSnapshotAutodeleteAttributesType objects
func NewVolumeSnapshotAutodeleteAttributesType() *VolumeSnapshotAutodeleteAttributesType {
	return &VolumeSnapshotAutodeleteAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeSnapshotAutodeleteAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSnapshotAutodeleteAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Commitment is a 'getter' method
func (o *VolumeSnapshotAutodeleteAttributesType) Commitment() string {
	r := *o.CommitmentPtr
	return r
}

// SetCommitment is a fluent style 'setter' method that can be chained
func (o *VolumeSnapshotAutodeleteAttributesType) SetCommitment(newValue string) *VolumeSnapshotAutodeleteAttributesType {
	o.CommitmentPtr = &newValue
	return o
}

// DeferDelete is a 'getter' method
func (o *VolumeSnapshotAutodeleteAttributesType) DeferDelete() string {
	r := *o.DeferDeletePtr
	return r
}

// SetDeferDelete is a fluent style 'setter' method that can be chained
func (o *VolumeSnapshotAutodeleteAttributesType) SetDeferDelete(newValue string) *VolumeSnapshotAutodeleteAttributesType {
	o.DeferDeletePtr = &newValue
	return o
}

// DeleteOrder is a 'getter' method
func (o *VolumeSnapshotAutodeleteAttributesType) DeleteOrder() string {
	r := *o.DeleteOrderPtr
	return r
}

// SetDeleteOrder is a fluent style 'setter' method that can be chained
func (o *VolumeSnapshotAutodeleteAttributesType) SetDeleteOrder(newValue string) *VolumeSnapshotAutodeleteAttributesType {
	o.DeleteOrderPtr = &newValue
	return o
}

// DestroyList is a 'getter' method
func (o *VolumeSnapshotAutodeleteAttributesType) DestroyList() string {
	r := *o.DestroyListPtr
	return r
}

// SetDestroyList is a fluent style 'setter' method that can be chained
func (o *VolumeSnapshotAutodeleteAttributesType) SetDestroyList(newValue string) *VolumeSnapshotAutodeleteAttributesType {
	o.DestroyListPtr = &newValue
	return o
}

// IsAutodeleteEnabled is a 'getter' method
func (o *VolumeSnapshotAutodeleteAttributesType) IsAutodeleteEnabled() bool {
	r := *o.IsAutodeleteEnabledPtr
	return r
}

// SetIsAutodeleteEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeSnapshotAutodeleteAttributesType) SetIsAutodeleteEnabled(newValue bool) *VolumeSnapshotAutodeleteAttributesType {
	o.IsAutodeleteEnabledPtr = &newValue
	return o
}

// Prefix is a 'getter' method
func (o *VolumeSnapshotAutodeleteAttributesType) Prefix() string {
	r := *o.PrefixPtr
	return r
}

// SetPrefix is a fluent style 'setter' method that can be chained
func (o *VolumeSnapshotAutodeleteAttributesType) SetPrefix(newValue string) *VolumeSnapshotAutodeleteAttributesType {
	o.PrefixPtr = &newValue
	return o
}

// TargetFreeSpace is a 'getter' method
func (o *VolumeSnapshotAutodeleteAttributesType) TargetFreeSpace() int {
	r := *o.TargetFreeSpacePtr
	return r
}

// SetTargetFreeSpace is a fluent style 'setter' method that can be chained
func (o *VolumeSnapshotAutodeleteAttributesType) SetTargetFreeSpace(newValue int) *VolumeSnapshotAutodeleteAttributesType {
	o.TargetFreeSpacePtr = &newValue
	return o
}

// Trigger is a 'getter' method
func (o *VolumeSnapshotAutodeleteAttributesType) Trigger() string {
	r := *o.TriggerPtr
	return r
}

// SetTrigger is a fluent style 'setter' method that can be chained
func (o *VolumeSnapshotAutodeleteAttributesType) SetTrigger(newValue string) *VolumeSnapshotAutodeleteAttributesType {
	o.TriggerPtr = &newValue
	return o
}
