// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeMirrorAttributesType is a structure to represent a volume-mirror-attributes ZAPI object
type VolumeMirrorAttributesType struct {
	XMLName                     xml.Name `xml:"volume-mirror-attributes"`
	IsDataProtectionMirrorPtr   *bool    `xml:"is-data-protection-mirror"`
	IsLoadSharingMirrorPtr      *bool    `xml:"is-load-sharing-mirror"`
	IsMoveMirrorPtr             *bool    `xml:"is-move-mirror"`
	IsReplicaVolumePtr          *bool    `xml:"is-replica-volume"`
	IsSnapmirrorSourcePtr       *bool    `xml:"is-snapmirror-source"`
	MirrorTransferInProgressPtr *bool    `xml:"mirror-transfer-in-progress"`
	RedirectSnapshotIdPtr       *int     `xml:"redirect-snapshot-id"`
}

// NewVolumeMirrorAttributesType is a factory method for creating new instances of VolumeMirrorAttributesType objects
func NewVolumeMirrorAttributesType() *VolumeMirrorAttributesType {
	return &VolumeMirrorAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeMirrorAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeMirrorAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// IsDataProtectionMirror is a 'getter' method
func (o *VolumeMirrorAttributesType) IsDataProtectionMirror() bool {
	r := *o.IsDataProtectionMirrorPtr
	return r
}

// SetIsDataProtectionMirror is a fluent style 'setter' method that can be chained
func (o *VolumeMirrorAttributesType) SetIsDataProtectionMirror(newValue bool) *VolumeMirrorAttributesType {
	o.IsDataProtectionMirrorPtr = &newValue
	return o
}

// IsLoadSharingMirror is a 'getter' method
func (o *VolumeMirrorAttributesType) IsLoadSharingMirror() bool {
	r := *o.IsLoadSharingMirrorPtr
	return r
}

// SetIsLoadSharingMirror is a fluent style 'setter' method that can be chained
func (o *VolumeMirrorAttributesType) SetIsLoadSharingMirror(newValue bool) *VolumeMirrorAttributesType {
	o.IsLoadSharingMirrorPtr = &newValue
	return o
}

// IsMoveMirror is a 'getter' method
func (o *VolumeMirrorAttributesType) IsMoveMirror() bool {
	r := *o.IsMoveMirrorPtr
	return r
}

// SetIsMoveMirror is a fluent style 'setter' method that can be chained
func (o *VolumeMirrorAttributesType) SetIsMoveMirror(newValue bool) *VolumeMirrorAttributesType {
	o.IsMoveMirrorPtr = &newValue
	return o
}

// IsReplicaVolume is a 'getter' method
func (o *VolumeMirrorAttributesType) IsReplicaVolume() bool {
	r := *o.IsReplicaVolumePtr
	return r
}

// SetIsReplicaVolume is a fluent style 'setter' method that can be chained
func (o *VolumeMirrorAttributesType) SetIsReplicaVolume(newValue bool) *VolumeMirrorAttributesType {
	o.IsReplicaVolumePtr = &newValue
	return o
}

// IsSnapmirrorSource is a 'getter' method
func (o *VolumeMirrorAttributesType) IsSnapmirrorSource() bool {
	r := *o.IsSnapmirrorSourcePtr
	return r
}

// SetIsSnapmirrorSource is a fluent style 'setter' method that can be chained
func (o *VolumeMirrorAttributesType) SetIsSnapmirrorSource(newValue bool) *VolumeMirrorAttributesType {
	o.IsSnapmirrorSourcePtr = &newValue
	return o
}

// MirrorTransferInProgress is a 'getter' method
func (o *VolumeMirrorAttributesType) MirrorTransferInProgress() bool {
	r := *o.MirrorTransferInProgressPtr
	return r
}

// SetMirrorTransferInProgress is a fluent style 'setter' method that can be chained
func (o *VolumeMirrorAttributesType) SetMirrorTransferInProgress(newValue bool) *VolumeMirrorAttributesType {
	o.MirrorTransferInProgressPtr = &newValue
	return o
}

// RedirectSnapshotId is a 'getter' method
func (o *VolumeMirrorAttributesType) RedirectSnapshotId() int {
	r := *o.RedirectSnapshotIdPtr
	return r
}

// SetRedirectSnapshotId is a fluent style 'setter' method that can be chained
func (o *VolumeMirrorAttributesType) SetRedirectSnapshotId(newValue int) *VolumeMirrorAttributesType {
	o.RedirectSnapshotIdPtr = &newValue
	return o
}
