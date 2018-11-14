package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeAttributesType is a structure to represent a volume-attributes ZAPI object
type VolumeAttributesType struct {
	XMLName                                xml.Name                                 `xml:"volume-attributes"`
	EncryptPtr                             *bool                                    `xml:"encrypt"`
	KeyIdPtr                               *string                                  `xml:"key-id"`
	VolumeAntivirusAttributesPtr           *VolumeAntivirusAttributesType           `xml:"volume-antivirus-attributes"`
	VolumeAutobalanceAttributesPtr         *VolumeAutobalanceAttributesType         `xml:"volume-autobalance-attributes"`
	VolumeAutosizeAttributesPtr            *VolumeAutosizeAttributesType            `xml:"volume-autosize-attributes"`
	VolumeCloneAttributesPtr               *VolumeCloneAttributesType               `xml:"volume-clone-attributes"`
	VolumeCompAggrAttributesPtr            *VolumeCompAggrAttributesType            `xml:"volume-comp-aggr-attributes"`
	VolumeDirectoryAttributesPtr           *VolumeDirectoryAttributesType           `xml:"volume-directory-attributes"`
	VolumeExportAttributesPtr              *VolumeExportAttributesType              `xml:"volume-export-attributes"`
	VolumeFlexcacheAttributesPtr           *VolumeFlexcacheAttributesType           `xml:"volume-flexcache-attributes"`
	VolumeHybridCacheAttributesPtr         *VolumeHybridCacheAttributesType         `xml:"volume-hybrid-cache-attributes"`
	VolumeIdAttributesPtr                  *VolumeIdAttributesType                  `xml:"volume-id-attributes"`
	VolumeInfinitevolAttributesPtr         *VolumeInfinitevolAttributesType         `xml:"volume-infinitevol-attributes"`
	VolumeInodeAttributesPtr               *VolumeInodeAttributesType               `xml:"volume-inode-attributes"`
	VolumeLanguageAttributesPtr            *VolumeLanguageAttributesType            `xml:"volume-language-attributes"`
	VolumeMirrorAttributesPtr              *VolumeMirrorAttributesType              `xml:"volume-mirror-attributes"`
	VolumePerformanceAttributesPtr         *VolumePerformanceAttributesType         `xml:"volume-performance-attributes"`
	VolumeQosAttributesPtr                 *VolumeQosAttributesType                 `xml:"volume-qos-attributes"`
	VolumeSecurityAttributesPtr            *VolumeSecurityAttributesType            `xml:"volume-security-attributes"`
	VolumeSisAttributesPtr                 *VolumeSisAttributesType                 `xml:"volume-sis-attributes"`
	VolumeSnaplockAttributesPtr            *VolumeSnaplockAttributesType            `xml:"volume-snaplock-attributes"`
	VolumeSnapshotAttributesPtr            *VolumeSnapshotAttributesType            `xml:"volume-snapshot-attributes"`
	VolumeSnapshotAutodeleteAttributesPtr  *VolumeSnapshotAutodeleteAttributesType  `xml:"volume-snapshot-autodelete-attributes"`
	VolumeSpaceAttributesPtr               *VolumeSpaceAttributesType               `xml:"volume-space-attributes"`
	VolumeStateAttributesPtr               *VolumeStateAttributesType               `xml:"volume-state-attributes"`
	VolumeTransitionAttributesPtr          *VolumeTransitionAttributesType          `xml:"volume-transition-attributes"`
	VolumeVmAlignAttributesPtr             *VolumeVmAlignAttributesType             `xml:"volume-vm-align-attributes"`
	VolumeVserverDrProtectionAttributesPtr *VolumeVserverDrProtectionAttributesType `xml:"volume-vserver-dr-protection-attributes"`
}

// NewVolumeAttributesType is a factory method for creating new instances of VolumeAttributesType objects
func NewVolumeAttributesType() *VolumeAttributesType {
	return &VolumeAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Encrypt is a 'getter' method
func (o *VolumeAttributesType) Encrypt() bool {
	r := *o.EncryptPtr
	return r
}

// SetEncrypt is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetEncrypt(newValue bool) *VolumeAttributesType {
	o.EncryptPtr = &newValue
	return o
}

// KeyId is a 'getter' method
func (o *VolumeAttributesType) KeyId() string {
	r := *o.KeyIdPtr
	return r
}

// SetKeyId is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetKeyId(newValue string) *VolumeAttributesType {
	o.KeyIdPtr = &newValue
	return o
}

// VolumeAntivirusAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeAntivirusAttributes() VolumeAntivirusAttributesType {
	r := *o.VolumeAntivirusAttributesPtr
	return r
}

// SetVolumeAntivirusAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeAntivirusAttributes(newValue VolumeAntivirusAttributesType) *VolumeAttributesType {
	o.VolumeAntivirusAttributesPtr = &newValue
	return o
}

// VolumeAutobalanceAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeAutobalanceAttributes() VolumeAutobalanceAttributesType {
	r := *o.VolumeAutobalanceAttributesPtr
	return r
}

// SetVolumeAutobalanceAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeAutobalanceAttributes(newValue VolumeAutobalanceAttributesType) *VolumeAttributesType {
	o.VolumeAutobalanceAttributesPtr = &newValue
	return o
}

// VolumeAutosizeAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeAutosizeAttributes() VolumeAutosizeAttributesType {
	r := *o.VolumeAutosizeAttributesPtr
	return r
}

// SetVolumeAutosizeAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeAutosizeAttributes(newValue VolumeAutosizeAttributesType) *VolumeAttributesType {
	o.VolumeAutosizeAttributesPtr = &newValue
	return o
}

// VolumeCloneAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeCloneAttributes() VolumeCloneAttributesType {
	r := *o.VolumeCloneAttributesPtr
	return r
}

// SetVolumeCloneAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeCloneAttributes(newValue VolumeCloneAttributesType) *VolumeAttributesType {
	o.VolumeCloneAttributesPtr = &newValue
	return o
}

// VolumeCompAggrAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeCompAggrAttributes() VolumeCompAggrAttributesType {
	r := *o.VolumeCompAggrAttributesPtr
	return r
}

// SetVolumeCompAggrAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeCompAggrAttributes(newValue VolumeCompAggrAttributesType) *VolumeAttributesType {
	o.VolumeCompAggrAttributesPtr = &newValue
	return o
}

// VolumeDirectoryAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeDirectoryAttributes() VolumeDirectoryAttributesType {
	r := *o.VolumeDirectoryAttributesPtr
	return r
}

// SetVolumeDirectoryAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeDirectoryAttributes(newValue VolumeDirectoryAttributesType) *VolumeAttributesType {
	o.VolumeDirectoryAttributesPtr = &newValue
	return o
}

// VolumeExportAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeExportAttributes() VolumeExportAttributesType {
	r := *o.VolumeExportAttributesPtr
	return r
}

// SetVolumeExportAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeExportAttributes(newValue VolumeExportAttributesType) *VolumeAttributesType {
	o.VolumeExportAttributesPtr = &newValue
	return o
}

// VolumeFlexcacheAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeFlexcacheAttributes() VolumeFlexcacheAttributesType {
	r := *o.VolumeFlexcacheAttributesPtr
	return r
}

// SetVolumeFlexcacheAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeFlexcacheAttributes(newValue VolumeFlexcacheAttributesType) *VolumeAttributesType {
	o.VolumeFlexcacheAttributesPtr = &newValue
	return o
}

// VolumeHybridCacheAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeHybridCacheAttributes() VolumeHybridCacheAttributesType {
	r := *o.VolumeHybridCacheAttributesPtr
	return r
}

// SetVolumeHybridCacheAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeHybridCacheAttributes(newValue VolumeHybridCacheAttributesType) *VolumeAttributesType {
	o.VolumeHybridCacheAttributesPtr = &newValue
	return o
}

// VolumeIdAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeIdAttributes() VolumeIdAttributesType {
	r := *o.VolumeIdAttributesPtr
	return r
}

// SetVolumeIdAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeIdAttributes(newValue VolumeIdAttributesType) *VolumeAttributesType {
	o.VolumeIdAttributesPtr = &newValue
	return o
}

// VolumeInfinitevolAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeInfinitevolAttributes() VolumeInfinitevolAttributesType {
	r := *o.VolumeInfinitevolAttributesPtr
	return r
}

// SetVolumeInfinitevolAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeInfinitevolAttributes(newValue VolumeInfinitevolAttributesType) *VolumeAttributesType {
	o.VolumeInfinitevolAttributesPtr = &newValue
	return o
}

// VolumeInodeAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeInodeAttributes() VolumeInodeAttributesType {
	r := *o.VolumeInodeAttributesPtr
	return r
}

// SetVolumeInodeAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeInodeAttributes(newValue VolumeInodeAttributesType) *VolumeAttributesType {
	o.VolumeInodeAttributesPtr = &newValue
	return o
}

// VolumeLanguageAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeLanguageAttributes() VolumeLanguageAttributesType {
	r := *o.VolumeLanguageAttributesPtr
	return r
}

// SetVolumeLanguageAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeLanguageAttributes(newValue VolumeLanguageAttributesType) *VolumeAttributesType {
	o.VolumeLanguageAttributesPtr = &newValue
	return o
}

// VolumeMirrorAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeMirrorAttributes() VolumeMirrorAttributesType {
	r := *o.VolumeMirrorAttributesPtr
	return r
}

// SetVolumeMirrorAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeMirrorAttributes(newValue VolumeMirrorAttributesType) *VolumeAttributesType {
	o.VolumeMirrorAttributesPtr = &newValue
	return o
}

// VolumePerformanceAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumePerformanceAttributes() VolumePerformanceAttributesType {
	r := *o.VolumePerformanceAttributesPtr
	return r
}

// SetVolumePerformanceAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumePerformanceAttributes(newValue VolumePerformanceAttributesType) *VolumeAttributesType {
	o.VolumePerformanceAttributesPtr = &newValue
	return o
}

// VolumeQosAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeQosAttributes() VolumeQosAttributesType {
	r := *o.VolumeQosAttributesPtr
	return r
}

// SetVolumeQosAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeQosAttributes(newValue VolumeQosAttributesType) *VolumeAttributesType {
	o.VolumeQosAttributesPtr = &newValue
	return o
}

// VolumeSecurityAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeSecurityAttributes() VolumeSecurityAttributesType {
	r := *o.VolumeSecurityAttributesPtr
	return r
}

// SetVolumeSecurityAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeSecurityAttributes(newValue VolumeSecurityAttributesType) *VolumeAttributesType {
	o.VolumeSecurityAttributesPtr = &newValue
	return o
}

// VolumeSisAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeSisAttributes() VolumeSisAttributesType {
	r := *o.VolumeSisAttributesPtr
	return r
}

// SetVolumeSisAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeSisAttributes(newValue VolumeSisAttributesType) *VolumeAttributesType {
	o.VolumeSisAttributesPtr = &newValue
	return o
}

// VolumeSnaplockAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeSnaplockAttributes() VolumeSnaplockAttributesType {
	r := *o.VolumeSnaplockAttributesPtr
	return r
}

// SetVolumeSnaplockAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeSnaplockAttributes(newValue VolumeSnaplockAttributesType) *VolumeAttributesType {
	o.VolumeSnaplockAttributesPtr = &newValue
	return o
}

// VolumeSnapshotAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeSnapshotAttributes() VolumeSnapshotAttributesType {
	r := *o.VolumeSnapshotAttributesPtr
	return r
}

// SetVolumeSnapshotAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeSnapshotAttributes(newValue VolumeSnapshotAttributesType) *VolumeAttributesType {
	o.VolumeSnapshotAttributesPtr = &newValue
	return o
}

// VolumeSnapshotAutodeleteAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeSnapshotAutodeleteAttributes() VolumeSnapshotAutodeleteAttributesType {
	r := *o.VolumeSnapshotAutodeleteAttributesPtr
	return r
}

// SetVolumeSnapshotAutodeleteAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeSnapshotAutodeleteAttributes(newValue VolumeSnapshotAutodeleteAttributesType) *VolumeAttributesType {
	o.VolumeSnapshotAutodeleteAttributesPtr = &newValue
	return o
}

// VolumeSpaceAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeSpaceAttributes() VolumeSpaceAttributesType {
	r := *o.VolumeSpaceAttributesPtr
	return r
}

// SetVolumeSpaceAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeSpaceAttributes(newValue VolumeSpaceAttributesType) *VolumeAttributesType {
	o.VolumeSpaceAttributesPtr = &newValue
	return o
}

// VolumeStateAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeStateAttributes() VolumeStateAttributesType {
	r := *o.VolumeStateAttributesPtr
	return r
}

// SetVolumeStateAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeStateAttributes(newValue VolumeStateAttributesType) *VolumeAttributesType {
	o.VolumeStateAttributesPtr = &newValue
	return o
}

// VolumeTransitionAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeTransitionAttributes() VolumeTransitionAttributesType {
	r := *o.VolumeTransitionAttributesPtr
	return r
}

// SetVolumeTransitionAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeTransitionAttributes(newValue VolumeTransitionAttributesType) *VolumeAttributesType {
	o.VolumeTransitionAttributesPtr = &newValue
	return o
}

// VolumeVmAlignAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeVmAlignAttributes() VolumeVmAlignAttributesType {
	r := *o.VolumeVmAlignAttributesPtr
	return r
}

// SetVolumeVmAlignAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeVmAlignAttributes(newValue VolumeVmAlignAttributesType) *VolumeAttributesType {
	o.VolumeVmAlignAttributesPtr = &newValue
	return o
}

// VolumeVserverDrProtectionAttributes is a 'getter' method
func (o *VolumeAttributesType) VolumeVserverDrProtectionAttributes() VolumeVserverDrProtectionAttributesType {
	r := *o.VolumeVserverDrProtectionAttributesPtr
	return r
}

// SetVolumeVserverDrProtectionAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeAttributesType) SetVolumeVserverDrProtectionAttributes(newValue VolumeVserverDrProtectionAttributesType) *VolumeAttributesType {
	o.VolumeVserverDrProtectionAttributesPtr = &newValue
	return o
}
