// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// LunInfoType is a structure to represent a lun-info ZAPI object
type LunInfoType struct {
	XMLName                      xml.Name       `xml:"lun-info"`
	AlignmentPtr                 *string        `xml:"alignment"`
	ApplicationPtr               *string        `xml:"application"`
	ApplicationUuidPtr           *string        `xml:"application-uuid"`
	BackingSnapshotPtr           *string        `xml:"backing-snapshot"`
	BlockSizePtr                 *int           `xml:"block-size"`
	CachingPolicyPtr             *string        `xml:"caching-policy"`
	ClassPtr                     *string        `xml:"class"`
	CloneBackingSnapshotPtr      *string        `xml:"clone-backing-snapshot"`
	CommentPtr                   *string        `xml:"comment"`
	CreationTimestampPtr         *int           `xml:"creation-timestamp"`
	DeviceBinaryIdPtr            *string        `xml:"device-binary-id"`
	DeviceIdPtr                  *int           `xml:"device-id"`
	DeviceTextIdPtr              *string        `xml:"device-text-id"`
	IsClonePtr                   *bool          `xml:"is-clone"`
	IsCloneAutodeleteEnabledPtr  *bool          `xml:"is-clone-autodelete-enabled"`
	IsInconsistentImportPtr      *bool          `xml:"is-inconsistent-import"`
	IsRestoreInaccessiblePtr     *bool          `xml:"is-restore-inaccessible"`
	IsSpaceAllocEnabledPtr       *bool          `xml:"is-space-alloc-enabled"`
	IsSpaceReservationEnabledPtr *bool          `xml:"is-space-reservation-enabled"`
	MappedPtr                    *bool          `xml:"mapped"`
	MultiprotocolTypePtr         *LunOsTypeType `xml:"multiprotocol-type"`
	NodePtr                      *NodeNameType  `xml:"node"`
	OnlinePtr                    *bool          `xml:"online"`
	PathPtr                      *string        `xml:"path"`
	PrefixSizePtr                *int           `xml:"prefix-size"`
	QosAdaptivePolicyGroupPtr    *string        `xml:"qos-adaptive-policy-group"`
	QosPolicyGroupPtr            *string        `xml:"qos-policy-group"`
	QtreePtr                     *string        `xml:"qtree"`
	ReadOnlyPtr                  *bool          `xml:"read-only"`
	Serial7ModePtr               *string        `xml:"serial-7-mode"`
	SerialNumberPtr              *string        `xml:"serial-number"`
	ShareStatePtr                *string        `xml:"share-state"`
	SizePtr                      *int           `xml:"size"`
	SizeUsedPtr                  *int           `xml:"size-used"`
	StagingPtr                   *bool          `xml:"staging"`
	StatePtr                     *string        `xml:"state"`
	SuffixSizePtr                *int           `xml:"suffix-size"`
	UuidPtr                      *string        `xml:"uuid"`
	VolumePtr                    *string        `xml:"volume"`
	VserverPtr                   *string        `xml:"vserver"`
}

// NewLunInfoType is a factory method for creating new instances of LunInfoType objects
func NewLunInfoType() *LunInfoType {
	return &LunInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *LunInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o LunInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Alignment is a 'getter' method
func (o *LunInfoType) Alignment() string {
	var r string
	if o.AlignmentPtr == nil {
		return r
	}
	r = *o.AlignmentPtr
	return r
}

// SetAlignment is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetAlignment(newValue string) *LunInfoType {
	o.AlignmentPtr = &newValue
	return o
}

// Application is a 'getter' method
func (o *LunInfoType) Application() string {
	var r string
	if o.ApplicationPtr == nil {
		return r
	}
	r = *o.ApplicationPtr
	return r
}

// SetApplication is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetApplication(newValue string) *LunInfoType {
	o.ApplicationPtr = &newValue
	return o
}

// ApplicationUuid is a 'getter' method
func (o *LunInfoType) ApplicationUuid() string {
	var r string
	if o.ApplicationUuidPtr == nil {
		return r
	}
	r = *o.ApplicationUuidPtr
	return r
}

// SetApplicationUuid is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetApplicationUuid(newValue string) *LunInfoType {
	o.ApplicationUuidPtr = &newValue
	return o
}

// BackingSnapshot is a 'getter' method
func (o *LunInfoType) BackingSnapshot() string {
	var r string
	if o.BackingSnapshotPtr == nil {
		return r
	}
	r = *o.BackingSnapshotPtr
	return r
}

// SetBackingSnapshot is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetBackingSnapshot(newValue string) *LunInfoType {
	o.BackingSnapshotPtr = &newValue
	return o
}

// BlockSize is a 'getter' method
func (o *LunInfoType) BlockSize() int {
	var r int
	if o.BlockSizePtr == nil {
		return r
	}
	r = *o.BlockSizePtr
	return r
}

// SetBlockSize is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetBlockSize(newValue int) *LunInfoType {
	o.BlockSizePtr = &newValue
	return o
}

// CachingPolicy is a 'getter' method
func (o *LunInfoType) CachingPolicy() string {
	var r string
	if o.CachingPolicyPtr == nil {
		return r
	}
	r = *o.CachingPolicyPtr
	return r
}

// SetCachingPolicy is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetCachingPolicy(newValue string) *LunInfoType {
	o.CachingPolicyPtr = &newValue
	return o
}

// Class is a 'getter' method
func (o *LunInfoType) Class() string {
	var r string
	if o.ClassPtr == nil {
		return r
	}
	r = *o.ClassPtr
	return r
}

// SetClass is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetClass(newValue string) *LunInfoType {
	o.ClassPtr = &newValue
	return o
}

// CloneBackingSnapshot is a 'getter' method
func (o *LunInfoType) CloneBackingSnapshot() string {
	var r string
	if o.CloneBackingSnapshotPtr == nil {
		return r
	}
	r = *o.CloneBackingSnapshotPtr
	return r
}

// SetCloneBackingSnapshot is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetCloneBackingSnapshot(newValue string) *LunInfoType {
	o.CloneBackingSnapshotPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *LunInfoType) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetComment(newValue string) *LunInfoType {
	o.CommentPtr = &newValue
	return o
}

// CreationTimestamp is a 'getter' method
func (o *LunInfoType) CreationTimestamp() int {
	var r int
	if o.CreationTimestampPtr == nil {
		return r
	}
	r = *o.CreationTimestampPtr
	return r
}

// SetCreationTimestamp is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetCreationTimestamp(newValue int) *LunInfoType {
	o.CreationTimestampPtr = &newValue
	return o
}

// DeviceBinaryId is a 'getter' method
func (o *LunInfoType) DeviceBinaryId() string {
	var r string
	if o.DeviceBinaryIdPtr == nil {
		return r
	}
	r = *o.DeviceBinaryIdPtr
	return r
}

// SetDeviceBinaryId is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetDeviceBinaryId(newValue string) *LunInfoType {
	o.DeviceBinaryIdPtr = &newValue
	return o
}

// DeviceId is a 'getter' method
func (o *LunInfoType) DeviceId() int {
	var r int
	if o.DeviceIdPtr == nil {
		return r
	}
	r = *o.DeviceIdPtr
	return r
}

// SetDeviceId is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetDeviceId(newValue int) *LunInfoType {
	o.DeviceIdPtr = &newValue
	return o
}

// DeviceTextId is a 'getter' method
func (o *LunInfoType) DeviceTextId() string {
	var r string
	if o.DeviceTextIdPtr == nil {
		return r
	}
	r = *o.DeviceTextIdPtr
	return r
}

// SetDeviceTextId is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetDeviceTextId(newValue string) *LunInfoType {
	o.DeviceTextIdPtr = &newValue
	return o
}

// IsClone is a 'getter' method
func (o *LunInfoType) IsClone() bool {
	var r bool
	if o.IsClonePtr == nil {
		return r
	}
	r = *o.IsClonePtr
	return r
}

// SetIsClone is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetIsClone(newValue bool) *LunInfoType {
	o.IsClonePtr = &newValue
	return o
}

// IsCloneAutodeleteEnabled is a 'getter' method
func (o *LunInfoType) IsCloneAutodeleteEnabled() bool {
	var r bool
	if o.IsCloneAutodeleteEnabledPtr == nil {
		return r
	}
	r = *o.IsCloneAutodeleteEnabledPtr
	return r
}

// SetIsCloneAutodeleteEnabled is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetIsCloneAutodeleteEnabled(newValue bool) *LunInfoType {
	o.IsCloneAutodeleteEnabledPtr = &newValue
	return o
}

// IsInconsistentImport is a 'getter' method
func (o *LunInfoType) IsInconsistentImport() bool {
	var r bool
	if o.IsInconsistentImportPtr == nil {
		return r
	}
	r = *o.IsInconsistentImportPtr
	return r
}

// SetIsInconsistentImport is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetIsInconsistentImport(newValue bool) *LunInfoType {
	o.IsInconsistentImportPtr = &newValue
	return o
}

// IsRestoreInaccessible is a 'getter' method
func (o *LunInfoType) IsRestoreInaccessible() bool {
	var r bool
	if o.IsRestoreInaccessiblePtr == nil {
		return r
	}
	r = *o.IsRestoreInaccessiblePtr
	return r
}

// SetIsRestoreInaccessible is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetIsRestoreInaccessible(newValue bool) *LunInfoType {
	o.IsRestoreInaccessiblePtr = &newValue
	return o
}

// IsSpaceAllocEnabled is a 'getter' method
func (o *LunInfoType) IsSpaceAllocEnabled() bool {
	var r bool
	if o.IsSpaceAllocEnabledPtr == nil {
		return r
	}
	r = *o.IsSpaceAllocEnabledPtr
	return r
}

// SetIsSpaceAllocEnabled is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetIsSpaceAllocEnabled(newValue bool) *LunInfoType {
	o.IsSpaceAllocEnabledPtr = &newValue
	return o
}

// IsSpaceReservationEnabled is a 'getter' method
func (o *LunInfoType) IsSpaceReservationEnabled() bool {
	var r bool
	if o.IsSpaceReservationEnabledPtr == nil {
		return r
	}
	r = *o.IsSpaceReservationEnabledPtr
	return r
}

// SetIsSpaceReservationEnabled is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetIsSpaceReservationEnabled(newValue bool) *LunInfoType {
	o.IsSpaceReservationEnabledPtr = &newValue
	return o
}

// Mapped is a 'getter' method
func (o *LunInfoType) Mapped() bool {
	var r bool
	if o.MappedPtr == nil {
		return r
	}
	r = *o.MappedPtr
	return r
}

// SetMapped is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetMapped(newValue bool) *LunInfoType {
	o.MappedPtr = &newValue
	return o
}

// MultiprotocolType is a 'getter' method
func (o *LunInfoType) MultiprotocolType() LunOsTypeType {
	var r LunOsTypeType
	if o.MultiprotocolTypePtr == nil {
		return r
	}
	r = *o.MultiprotocolTypePtr
	return r
}

// SetMultiprotocolType is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetMultiprotocolType(newValue LunOsTypeType) *LunInfoType {
	o.MultiprotocolTypePtr = &newValue
	return o
}

// Node is a 'getter' method
func (o *LunInfoType) Node() NodeNameType {
	var r NodeNameType
	if o.NodePtr == nil {
		return r
	}
	r = *o.NodePtr
	return r
}

// SetNode is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetNode(newValue NodeNameType) *LunInfoType {
	o.NodePtr = &newValue
	return o
}

// Online is a 'getter' method
func (o *LunInfoType) Online() bool {
	var r bool
	if o.OnlinePtr == nil {
		return r
	}
	r = *o.OnlinePtr
	return r
}

// SetOnline is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetOnline(newValue bool) *LunInfoType {
	o.OnlinePtr = &newValue
	return o
}

// Path is a 'getter' method
func (o *LunInfoType) Path() string {
	var r string
	if o.PathPtr == nil {
		return r
	}
	r = *o.PathPtr
	return r
}

// SetPath is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetPath(newValue string) *LunInfoType {
	o.PathPtr = &newValue
	return o
}

// PrefixSize is a 'getter' method
func (o *LunInfoType) PrefixSize() int {
	var r int
	if o.PrefixSizePtr == nil {
		return r
	}
	r = *o.PrefixSizePtr
	return r
}

// SetPrefixSize is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetPrefixSize(newValue int) *LunInfoType {
	o.PrefixSizePtr = &newValue
	return o
}

// QosAdaptivePolicyGroup is a 'getter' method
func (o *LunInfoType) QosAdaptivePolicyGroup() string {
	var r string
	if o.QosAdaptivePolicyGroupPtr == nil {
		return r
	}
	r = *o.QosAdaptivePolicyGroupPtr
	return r
}

// SetQosAdaptivePolicyGroup is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetQosAdaptivePolicyGroup(newValue string) *LunInfoType {
	o.QosAdaptivePolicyGroupPtr = &newValue
	return o
}

// QosPolicyGroup is a 'getter' method
func (o *LunInfoType) QosPolicyGroup() string {
	var r string
	if o.QosPolicyGroupPtr == nil {
		return r
	}
	r = *o.QosPolicyGroupPtr
	return r
}

// SetQosPolicyGroup is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetQosPolicyGroup(newValue string) *LunInfoType {
	o.QosPolicyGroupPtr = &newValue
	return o
}

// Qtree is a 'getter' method
func (o *LunInfoType) Qtree() string {
	var r string
	if o.QtreePtr == nil {
		return r
	}
	r = *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetQtree(newValue string) *LunInfoType {
	o.QtreePtr = &newValue
	return o
}

// ReadOnly is a 'getter' method
func (o *LunInfoType) ReadOnly() bool {
	var r bool
	if o.ReadOnlyPtr == nil {
		return r
	}
	r = *o.ReadOnlyPtr
	return r
}

// SetReadOnly is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetReadOnly(newValue bool) *LunInfoType {
	o.ReadOnlyPtr = &newValue
	return o
}

// Serial7Mode is a 'getter' method
func (o *LunInfoType) Serial7Mode() string {
	var r string
	if o.Serial7ModePtr == nil {
		return r
	}
	r = *o.Serial7ModePtr
	return r
}

// SetSerial7Mode is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetSerial7Mode(newValue string) *LunInfoType {
	o.Serial7ModePtr = &newValue
	return o
}

// SerialNumber is a 'getter' method
func (o *LunInfoType) SerialNumber() string {
	var r string
	if o.SerialNumberPtr == nil {
		return r
	}
	r = *o.SerialNumberPtr
	return r
}

// SetSerialNumber is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetSerialNumber(newValue string) *LunInfoType {
	o.SerialNumberPtr = &newValue
	return o
}

// ShareState is a 'getter' method
func (o *LunInfoType) ShareState() string {
	var r string
	if o.ShareStatePtr == nil {
		return r
	}
	r = *o.ShareStatePtr
	return r
}

// SetShareState is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetShareState(newValue string) *LunInfoType {
	o.ShareStatePtr = &newValue
	return o
}

// Size is a 'getter' method
func (o *LunInfoType) Size() int {
	var r int
	if o.SizePtr == nil {
		return r
	}
	r = *o.SizePtr
	return r
}

// SetSize is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetSize(newValue int) *LunInfoType {
	o.SizePtr = &newValue
	return o
}

// SizeUsed is a 'getter' method
func (o *LunInfoType) SizeUsed() int {
	var r int
	if o.SizeUsedPtr == nil {
		return r
	}
	r = *o.SizeUsedPtr
	return r
}

// SetSizeUsed is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetSizeUsed(newValue int) *LunInfoType {
	o.SizeUsedPtr = &newValue
	return o
}

// Staging is a 'getter' method
func (o *LunInfoType) Staging() bool {
	var r bool
	if o.StagingPtr == nil {
		return r
	}
	r = *o.StagingPtr
	return r
}

// SetStaging is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetStaging(newValue bool) *LunInfoType {
	o.StagingPtr = &newValue
	return o
}

// State is a 'getter' method
func (o *LunInfoType) State() string {
	var r string
	if o.StatePtr == nil {
		return r
	}
	r = *o.StatePtr
	return r
}

// SetState is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetState(newValue string) *LunInfoType {
	o.StatePtr = &newValue
	return o
}

// SuffixSize is a 'getter' method
func (o *LunInfoType) SuffixSize() int {
	var r int
	if o.SuffixSizePtr == nil {
		return r
	}
	r = *o.SuffixSizePtr
	return r
}

// SetSuffixSize is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetSuffixSize(newValue int) *LunInfoType {
	o.SuffixSizePtr = &newValue
	return o
}

// Uuid is a 'getter' method
func (o *LunInfoType) Uuid() string {
	var r string
	if o.UuidPtr == nil {
		return r
	}
	r = *o.UuidPtr
	return r
}

// SetUuid is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetUuid(newValue string) *LunInfoType {
	o.UuidPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *LunInfoType) Volume() string {
	var r string
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetVolume(newValue string) *LunInfoType {
	o.VolumePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *LunInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *LunInfoType) SetVserver(newValue string) *LunInfoType {
	o.VserverPtr = &newValue
	return o
}
