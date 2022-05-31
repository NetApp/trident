// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeStateAttributesType is a structure to represent a volume-state-attributes ZAPI object
type VolumeStateAttributesType struct {
	XMLName                       xml.Name                         `xml:"volume-state-attributes"`
	BecomeNodeRootAfterRebootPtr  *bool                            `xml:"become-node-root-after-reboot"`
	ConstituentCountPtr           *int                             `xml:"constituent-count"`
	ForceNvfailOnDrPtr            *bool                            `xml:"force-nvfail-on-dr"`
	IgnoreInconsistentPtr         *bool                            `xml:"ignore-inconsistent"`
	InNvfailedStatePtr            *bool                            `xml:"in-nvfailed-state"`
	IsClusterVolumePtr            *bool                            `xml:"is-cluster-volume"`
	IsConstituentPtr              *bool                            `xml:"is-constituent"`
	IsFlexgroupPtr                *bool                            `xml:"is-flexgroup"`
	IsFlexgroupQtreeEnabledPtr    *bool                            `xml:"is-flexgroup-qtree-enabled"`
	IsInconsistentPtr             *bool                            `xml:"is-inconsistent"`
	IsInvalidPtr                  *bool                            `xml:"is-invalid"`
	IsJunctionActivePtr           *bool                            `xml:"is-junction-active"`
	IsMoveDestinationInCutoverPtr *bool                            `xml:"is-move-destination-in-cutover"`
	IsMovingPtr                   *bool                            `xml:"is-moving"`
	IsNodeRootPtr                 *bool                            `xml:"is-node-root"`
	IsNvfailEnabledPtr            *bool                            `xml:"is-nvfail-enabled"`
	IsProtocolAccessFencedPtr     *bool                            `xml:"is-protocol-access-fenced"`
	IsQuiescedInMemoryPtr         *bool                            `xml:"is-quiesced-in-memory"`
	IsQuiescedOnDiskPtr           *bool                            `xml:"is-quiesced-on-disk"`
	IsSmbcFailoverCapablePtr      *bool                            `xml:"is-smbc-failover-capable"`
	IsSmbcMasterPtr               *bool                            `xml:"is-smbc-master"`
	IsUnrecoverablePtr            *bool                            `xml:"is-unrecoverable"`
	IsVolumeInCutoverPtr          *bool                            `xml:"is-volume-in-cutover"`
	IsVserverRootPtr              *bool                            `xml:"is-vserver-root"`
	ProtocolAccessFencedByPtr     *string                          `xml:"protocol-access-fenced-by"`
	SmbcConsensusPtr              *string                          `xml:"smbc-consensus"`
	StatePtr                      *string                          `xml:"state"`
	StatusPtr                     *VolumeStateAttributesTypeStatus `xml:"status"`
	// work in progress
}

// NewVolumeStateAttributesType is a factory method for creating new instances of VolumeStateAttributesType objects
func NewVolumeStateAttributesType() *VolumeStateAttributesType {
	return &VolumeStateAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeStateAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeStateAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// BecomeNodeRootAfterReboot is a 'getter' method
func (o *VolumeStateAttributesType) BecomeNodeRootAfterReboot() bool {
	var r bool
	if o.BecomeNodeRootAfterRebootPtr == nil {
		return r
	}
	r = *o.BecomeNodeRootAfterRebootPtr
	return r
}

// SetBecomeNodeRootAfterReboot is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetBecomeNodeRootAfterReboot(newValue bool) *VolumeStateAttributesType {
	o.BecomeNodeRootAfterRebootPtr = &newValue
	return o
}

// ConstituentCount is a 'getter' method
func (o *VolumeStateAttributesType) ConstituentCount() int {
	var r int
	if o.ConstituentCountPtr == nil {
		return r
	}
	r = *o.ConstituentCountPtr
	return r
}

// SetConstituentCount is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetConstituentCount(newValue int) *VolumeStateAttributesType {
	o.ConstituentCountPtr = &newValue
	return o
}

// ForceNvfailOnDr is a 'getter' method
func (o *VolumeStateAttributesType) ForceNvfailOnDr() bool {
	var r bool
	if o.ForceNvfailOnDrPtr == nil {
		return r
	}
	r = *o.ForceNvfailOnDrPtr
	return r
}

// SetForceNvfailOnDr is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetForceNvfailOnDr(newValue bool) *VolumeStateAttributesType {
	o.ForceNvfailOnDrPtr = &newValue
	return o
}

// IgnoreInconsistent is a 'getter' method
func (o *VolumeStateAttributesType) IgnoreInconsistent() bool {
	var r bool
	if o.IgnoreInconsistentPtr == nil {
		return r
	}
	r = *o.IgnoreInconsistentPtr
	return r
}

// SetIgnoreInconsistent is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIgnoreInconsistent(newValue bool) *VolumeStateAttributesType {
	o.IgnoreInconsistentPtr = &newValue
	return o
}

// InNvfailedState is a 'getter' method
func (o *VolumeStateAttributesType) InNvfailedState() bool {
	var r bool
	if o.InNvfailedStatePtr == nil {
		return r
	}
	r = *o.InNvfailedStatePtr
	return r
}

// SetInNvfailedState is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetInNvfailedState(newValue bool) *VolumeStateAttributesType {
	o.InNvfailedStatePtr = &newValue
	return o
}

// IsClusterVolume is a 'getter' method
func (o *VolumeStateAttributesType) IsClusterVolume() bool {
	var r bool
	if o.IsClusterVolumePtr == nil {
		return r
	}
	r = *o.IsClusterVolumePtr
	return r
}

// SetIsClusterVolume is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsClusterVolume(newValue bool) *VolumeStateAttributesType {
	o.IsClusterVolumePtr = &newValue
	return o
}

// IsConstituent is a 'getter' method
func (o *VolumeStateAttributesType) IsConstituent() bool {
	var r bool
	if o.IsConstituentPtr == nil {
		return r
	}
	r = *o.IsConstituentPtr
	return r
}

// SetIsConstituent is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsConstituent(newValue bool) *VolumeStateAttributesType {
	o.IsConstituentPtr = &newValue
	return o
}

// IsFlexgroup is a 'getter' method
func (o *VolumeStateAttributesType) IsFlexgroup() bool {
	var r bool
	if o.IsFlexgroupPtr == nil {
		return r
	}
	r = *o.IsFlexgroupPtr
	return r
}

// SetIsFlexgroup is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsFlexgroup(newValue bool) *VolumeStateAttributesType {
	o.IsFlexgroupPtr = &newValue
	return o
}

// IsFlexgroupQtreeEnabled is a 'getter' method
func (o *VolumeStateAttributesType) IsFlexgroupQtreeEnabled() bool {
	var r bool
	if o.IsFlexgroupQtreeEnabledPtr == nil {
		return r
	}
	r = *o.IsFlexgroupQtreeEnabledPtr
	return r
}

// SetIsFlexgroupQtreeEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsFlexgroupQtreeEnabled(newValue bool) *VolumeStateAttributesType {
	o.IsFlexgroupQtreeEnabledPtr = &newValue
	return o
}

// IsInconsistent is a 'getter' method
func (o *VolumeStateAttributesType) IsInconsistent() bool {
	var r bool
	if o.IsInconsistentPtr == nil {
		return r
	}
	r = *o.IsInconsistentPtr
	return r
}

// SetIsInconsistent is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsInconsistent(newValue bool) *VolumeStateAttributesType {
	o.IsInconsistentPtr = &newValue
	return o
}

// IsInvalid is a 'getter' method
func (o *VolumeStateAttributesType) IsInvalid() bool {
	var r bool
	if o.IsInvalidPtr == nil {
		return r
	}
	r = *o.IsInvalidPtr
	return r
}

// SetIsInvalid is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsInvalid(newValue bool) *VolumeStateAttributesType {
	o.IsInvalidPtr = &newValue
	return o
}

// IsJunctionActive is a 'getter' method
func (o *VolumeStateAttributesType) IsJunctionActive() bool {
	var r bool
	if o.IsJunctionActivePtr == nil {
		return r
	}
	r = *o.IsJunctionActivePtr
	return r
}

// SetIsJunctionActive is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsJunctionActive(newValue bool) *VolumeStateAttributesType {
	o.IsJunctionActivePtr = &newValue
	return o
}

// IsMoveDestinationInCutover is a 'getter' method
func (o *VolumeStateAttributesType) IsMoveDestinationInCutover() bool {
	var r bool
	if o.IsMoveDestinationInCutoverPtr == nil {
		return r
	}
	r = *o.IsMoveDestinationInCutoverPtr
	return r
}

// SetIsMoveDestinationInCutover is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsMoveDestinationInCutover(newValue bool) *VolumeStateAttributesType {
	o.IsMoveDestinationInCutoverPtr = &newValue
	return o
}

// IsMoving is a 'getter' method
func (o *VolumeStateAttributesType) IsMoving() bool {
	var r bool
	if o.IsMovingPtr == nil {
		return r
	}
	r = *o.IsMovingPtr
	return r
}

// SetIsMoving is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsMoving(newValue bool) *VolumeStateAttributesType {
	o.IsMovingPtr = &newValue
	return o
}

// IsNodeRoot is a 'getter' method
func (o *VolumeStateAttributesType) IsNodeRoot() bool {
	var r bool
	if o.IsNodeRootPtr == nil {
		return r
	}
	r = *o.IsNodeRootPtr
	return r
}

// SetIsNodeRoot is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsNodeRoot(newValue bool) *VolumeStateAttributesType {
	o.IsNodeRootPtr = &newValue
	return o
}

// IsNvfailEnabled is a 'getter' method
func (o *VolumeStateAttributesType) IsNvfailEnabled() bool {
	var r bool
	if o.IsNvfailEnabledPtr == nil {
		return r
	}
	r = *o.IsNvfailEnabledPtr
	return r
}

// SetIsNvfailEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsNvfailEnabled(newValue bool) *VolumeStateAttributesType {
	o.IsNvfailEnabledPtr = &newValue
	return o
}

// IsProtocolAccessFenced is a 'getter' method
func (o *VolumeStateAttributesType) IsProtocolAccessFenced() bool {
	var r bool
	if o.IsProtocolAccessFencedPtr == nil {
		return r
	}
	r = *o.IsProtocolAccessFencedPtr
	return r
}

// SetIsProtocolAccessFenced is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsProtocolAccessFenced(newValue bool) *VolumeStateAttributesType {
	o.IsProtocolAccessFencedPtr = &newValue
	return o
}

// IsQuiescedInMemory is a 'getter' method
func (o *VolumeStateAttributesType) IsQuiescedInMemory() bool {
	var r bool
	if o.IsQuiescedInMemoryPtr == nil {
		return r
	}
	r = *o.IsQuiescedInMemoryPtr
	return r
}

// SetIsQuiescedInMemory is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsQuiescedInMemory(newValue bool) *VolumeStateAttributesType {
	o.IsQuiescedInMemoryPtr = &newValue
	return o
}

// IsQuiescedOnDisk is a 'getter' method
func (o *VolumeStateAttributesType) IsQuiescedOnDisk() bool {
	var r bool
	if o.IsQuiescedOnDiskPtr == nil {
		return r
	}
	r = *o.IsQuiescedOnDiskPtr
	return r
}

// SetIsQuiescedOnDisk is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsQuiescedOnDisk(newValue bool) *VolumeStateAttributesType {
	o.IsQuiescedOnDiskPtr = &newValue
	return o
}

// IsSmbcFailoverCapable is a 'getter' method
func (o *VolumeStateAttributesType) IsSmbcFailoverCapable() bool {
	var r bool
	if o.IsSmbcFailoverCapablePtr == nil {
		return r
	}
	r = *o.IsSmbcFailoverCapablePtr
	return r
}

// SetIsSmbcFailoverCapable is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsSmbcFailoverCapable(newValue bool) *VolumeStateAttributesType {
	o.IsSmbcFailoverCapablePtr = &newValue
	return o
}

// IsSmbcMaster is a 'getter' method
func (o *VolumeStateAttributesType) IsSmbcMaster() bool {
	var r bool
	if o.IsSmbcMasterPtr == nil {
		return r
	}
	r = *o.IsSmbcMasterPtr
	return r
}

// SetIsSmbcMaster is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsSmbcMaster(newValue bool) *VolumeStateAttributesType {
	o.IsSmbcMasterPtr = &newValue
	return o
}

// IsUnrecoverable is a 'getter' method
func (o *VolumeStateAttributesType) IsUnrecoverable() bool {
	var r bool
	if o.IsUnrecoverablePtr == nil {
		return r
	}
	r = *o.IsUnrecoverablePtr
	return r
}

// SetIsUnrecoverable is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsUnrecoverable(newValue bool) *VolumeStateAttributesType {
	o.IsUnrecoverablePtr = &newValue
	return o
}

// IsVolumeInCutover is a 'getter' method
func (o *VolumeStateAttributesType) IsVolumeInCutover() bool {
	var r bool
	if o.IsVolumeInCutoverPtr == nil {
		return r
	}
	r = *o.IsVolumeInCutoverPtr
	return r
}

// SetIsVolumeInCutover is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsVolumeInCutover(newValue bool) *VolumeStateAttributesType {
	o.IsVolumeInCutoverPtr = &newValue
	return o
}

// IsVserverRoot is a 'getter' method
func (o *VolumeStateAttributesType) IsVserverRoot() bool {
	var r bool
	if o.IsVserverRootPtr == nil {
		return r
	}
	r = *o.IsVserverRootPtr
	return r
}

// SetIsVserverRoot is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsVserverRoot(newValue bool) *VolumeStateAttributesType {
	o.IsVserverRootPtr = &newValue
	return o
}

// ProtocolAccessFencedBy is a 'getter' method
func (o *VolumeStateAttributesType) ProtocolAccessFencedBy() string {
	var r string
	if o.ProtocolAccessFencedByPtr == nil {
		return r
	}
	r = *o.ProtocolAccessFencedByPtr
	return r
}

// SetProtocolAccessFencedBy is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetProtocolAccessFencedBy(newValue string) *VolumeStateAttributesType {
	o.ProtocolAccessFencedByPtr = &newValue
	return o
}

// SmbcConsensus is a 'getter' method
func (o *VolumeStateAttributesType) SmbcConsensus() string {
	var r string
	if o.SmbcConsensusPtr == nil {
		return r
	}
	r = *o.SmbcConsensusPtr
	return r
}

// SetSmbcConsensus is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetSmbcConsensus(newValue string) *VolumeStateAttributesType {
	o.SmbcConsensusPtr = &newValue
	return o
}

// State is a 'getter' method
func (o *VolumeStateAttributesType) State() string {
	var r string
	if o.StatePtr == nil {
		return r
	}
	r = *o.StatePtr
	return r
}

// SetState is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetState(newValue string) *VolumeStateAttributesType {
	o.StatePtr = &newValue
	return o
}

// VolumeStateAttributesTypeStatus is a wrapper
type VolumeStateAttributesTypeStatus struct {
	XMLName   xml.Name `xml:"status"`
	StringPtr []string `xml:"string"`
}

// String is a 'getter' method
func (o *VolumeStateAttributesTypeStatus) String() []string {
	r := o.StringPtr
	return r
}

// SetString is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesTypeStatus) SetString(newValue []string) *VolumeStateAttributesTypeStatus {
	newSlice := make([]string, len(newValue))
	copy(newSlice, newValue)
	o.StringPtr = newSlice
	return o
}

// Status is a 'getter' method
func (o *VolumeStateAttributesType) Status() VolumeStateAttributesTypeStatus {
	var r VolumeStateAttributesTypeStatus
	if o.StatusPtr == nil {
		return r
	}
	r = *o.StatusPtr
	return r
}

// SetStatus is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetStatus(newValue VolumeStateAttributesTypeStatus) *VolumeStateAttributesType {
	o.StatusPtr = &newValue
	return o
}
