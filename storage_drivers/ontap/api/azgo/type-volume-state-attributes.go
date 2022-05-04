// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeStateAttributesType is a structure to represent a volume-state-attributes ZAPI object
type VolumeStateAttributesType struct {
	XMLName                       xml.Name                         `xml:"volume-state-attributes"`
	BecomeNodeRootAfterRebootPtr  *bool                            `xml:"become-node-root-after-reboot"`
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
	IsUnrecoverablePtr            *bool                            `xml:"is-unrecoverable"`
	IsVolumeInCutoverPtr          *bool                            `xml:"is-volume-in-cutover"`
	IsVserverRootPtr              *bool                            `xml:"is-vserver-root"`
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
	r := *o.BecomeNodeRootAfterRebootPtr
	return r
}

// SetBecomeNodeRootAfterReboot is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetBecomeNodeRootAfterReboot(newValue bool) *VolumeStateAttributesType {
	o.BecomeNodeRootAfterRebootPtr = &newValue
	return o
}

// ForceNvfailOnDr is a 'getter' method
func (o *VolumeStateAttributesType) ForceNvfailOnDr() bool {
	r := *o.ForceNvfailOnDrPtr
	return r
}

// SetForceNvfailOnDr is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetForceNvfailOnDr(newValue bool) *VolumeStateAttributesType {
	o.ForceNvfailOnDrPtr = &newValue
	return o
}

// IgnoreInconsistent is a 'getter' method
func (o *VolumeStateAttributesType) IgnoreInconsistent() bool {
	r := *o.IgnoreInconsistentPtr
	return r
}

// SetIgnoreInconsistent is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIgnoreInconsistent(newValue bool) *VolumeStateAttributesType {
	o.IgnoreInconsistentPtr = &newValue
	return o
}

// InNvfailedState is a 'getter' method
func (o *VolumeStateAttributesType) InNvfailedState() bool {
	r := *o.InNvfailedStatePtr
	return r
}

// SetInNvfailedState is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetInNvfailedState(newValue bool) *VolumeStateAttributesType {
	o.InNvfailedStatePtr = &newValue
	return o
}

// IsClusterVolume is a 'getter' method
func (o *VolumeStateAttributesType) IsClusterVolume() bool {
	r := *o.IsClusterVolumePtr
	return r
}

// SetIsClusterVolume is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsClusterVolume(newValue bool) *VolumeStateAttributesType {
	o.IsClusterVolumePtr = &newValue
	return o
}

// IsConstituent is a 'getter' method
func (o *VolumeStateAttributesType) IsConstituent() bool {
	r := *o.IsConstituentPtr
	return r
}

// SetIsConstituent is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsConstituent(newValue bool) *VolumeStateAttributesType {
	o.IsConstituentPtr = &newValue
	return o
}

// IsFlexgroup is a 'getter' method
func (o *VolumeStateAttributesType) IsFlexgroup() bool {
	r := *o.IsFlexgroupPtr
	return r
}

// SetIsFlexgroup is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsFlexgroup(newValue bool) *VolumeStateAttributesType {
	o.IsFlexgroupPtr = &newValue
	return o
}

// IsFlexgroupQtreeEnabled is a 'getter' method
func (o *VolumeStateAttributesType) IsFlexgroupQtreeEnabled() bool {
	r := *o.IsFlexgroupQtreeEnabledPtr
	return r
}

// SetIsFlexgroupQtreeEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsFlexgroupQtreeEnabled(newValue bool) *VolumeStateAttributesType {
	o.IsFlexgroupQtreeEnabledPtr = &newValue
	return o
}

// IsInconsistent is a 'getter' method
func (o *VolumeStateAttributesType) IsInconsistent() bool {
	r := *o.IsInconsistentPtr
	return r
}

// SetIsInconsistent is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsInconsistent(newValue bool) *VolumeStateAttributesType {
	o.IsInconsistentPtr = &newValue
	return o
}

// IsInvalid is a 'getter' method
func (o *VolumeStateAttributesType) IsInvalid() bool {
	r := *o.IsInvalidPtr
	return r
}

// SetIsInvalid is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsInvalid(newValue bool) *VolumeStateAttributesType {
	o.IsInvalidPtr = &newValue
	return o
}

// IsJunctionActive is a 'getter' method
func (o *VolumeStateAttributesType) IsJunctionActive() bool {
	r := *o.IsJunctionActivePtr
	return r
}

// SetIsJunctionActive is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsJunctionActive(newValue bool) *VolumeStateAttributesType {
	o.IsJunctionActivePtr = &newValue
	return o
}

// IsMoveDestinationInCutover is a 'getter' method
func (o *VolumeStateAttributesType) IsMoveDestinationInCutover() bool {
	r := *o.IsMoveDestinationInCutoverPtr
	return r
}

// SetIsMoveDestinationInCutover is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsMoveDestinationInCutover(newValue bool) *VolumeStateAttributesType {
	o.IsMoveDestinationInCutoverPtr = &newValue
	return o
}

// IsMoving is a 'getter' method
func (o *VolumeStateAttributesType) IsMoving() bool {
	r := *o.IsMovingPtr
	return r
}

// SetIsMoving is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsMoving(newValue bool) *VolumeStateAttributesType {
	o.IsMovingPtr = &newValue
	return o
}

// IsNodeRoot is a 'getter' method
func (o *VolumeStateAttributesType) IsNodeRoot() bool {
	r := *o.IsNodeRootPtr
	return r
}

// SetIsNodeRoot is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsNodeRoot(newValue bool) *VolumeStateAttributesType {
	o.IsNodeRootPtr = &newValue
	return o
}

// IsNvfailEnabled is a 'getter' method
func (o *VolumeStateAttributesType) IsNvfailEnabled() bool {
	r := *o.IsNvfailEnabledPtr
	return r
}

// SetIsNvfailEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsNvfailEnabled(newValue bool) *VolumeStateAttributesType {
	o.IsNvfailEnabledPtr = &newValue
	return o
}

// IsProtocolAccessFenced is a 'getter' method
func (o *VolumeStateAttributesType) IsProtocolAccessFenced() bool {
	r := *o.IsProtocolAccessFencedPtr
	return r
}

// SetIsProtocolAccessFenced is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsProtocolAccessFenced(newValue bool) *VolumeStateAttributesType {
	o.IsProtocolAccessFencedPtr = &newValue
	return o
}

// IsQuiescedInMemory is a 'getter' method
func (o *VolumeStateAttributesType) IsQuiescedInMemory() bool {
	r := *o.IsQuiescedInMemoryPtr
	return r
}

// SetIsQuiescedInMemory is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsQuiescedInMemory(newValue bool) *VolumeStateAttributesType {
	o.IsQuiescedInMemoryPtr = &newValue
	return o
}

// IsQuiescedOnDisk is a 'getter' method
func (o *VolumeStateAttributesType) IsQuiescedOnDisk() bool {
	r := *o.IsQuiescedOnDiskPtr
	return r
}

// SetIsQuiescedOnDisk is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsQuiescedOnDisk(newValue bool) *VolumeStateAttributesType {
	o.IsQuiescedOnDiskPtr = &newValue
	return o
}

// IsUnrecoverable is a 'getter' method
func (o *VolumeStateAttributesType) IsUnrecoverable() bool {
	r := *o.IsUnrecoverablePtr
	return r
}

// SetIsUnrecoverable is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsUnrecoverable(newValue bool) *VolumeStateAttributesType {
	o.IsUnrecoverablePtr = &newValue
	return o
}

// IsVolumeInCutover is a 'getter' method
func (o *VolumeStateAttributesType) IsVolumeInCutover() bool {
	r := *o.IsVolumeInCutoverPtr
	return r
}

// SetIsVolumeInCutover is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsVolumeInCutover(newValue bool) *VolumeStateAttributesType {
	o.IsVolumeInCutoverPtr = &newValue
	return o
}

// IsVserverRoot is a 'getter' method
func (o *VolumeStateAttributesType) IsVserverRoot() bool {
	r := *o.IsVserverRootPtr
	return r
}

// SetIsVserverRoot is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetIsVserverRoot(newValue bool) *VolumeStateAttributesType {
	o.IsVserverRootPtr = &newValue
	return o
}

// State is a 'getter' method
func (o *VolumeStateAttributesType) State() string {
	r := *o.StatePtr
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
	r := *o.StatusPtr
	return r
}

// SetStatus is a fluent style 'setter' method that can be chained
func (o *VolumeStateAttributesType) SetStatus(newValue VolumeStateAttributesTypeStatus) *VolumeStateAttributesType {
	o.StatusPtr = &newValue
	return o
}
