// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrSnapshotAttributesType is a structure to represent a aggr-snapshot-attributes ZAPI object
type AggrSnapshotAttributesType struct {
	XMLName                        xml.Name `xml:"aggr-snapshot-attributes"`
	FilesTotalPtr                  *int     `xml:"files-total"`
	FilesUsedPtr                   *int     `xml:"files-used"`
	InofileVersionPtr              *int     `xml:"inofile-version"`
	IsSnapshotAutoCreateEnabledPtr *bool    `xml:"is-snapshot-auto-create-enabled"`
	IsSnapshotAutoDeleteEnabledPtr *bool    `xml:"is-snapshot-auto-delete-enabled"`
	MaxfilesAvailablePtr           *int     `xml:"maxfiles-available"`
	MaxfilesPossiblePtr            *int     `xml:"maxfiles-possible"`
	MaxfilesUsedPtr                *int     `xml:"maxfiles-used"`
	PercentInodeUsedCapacityPtr    *int     `xml:"percent-inode-used-capacity"`
	PercentUsedCapacityPtr         *int     `xml:"percent-used-capacity"`
	SizeAvailablePtr               *int     `xml:"size-available"`
	SizeTotalPtr                   *int     `xml:"size-total"`
	SizeUsedPtr                    *int     `xml:"size-used"`
	SnapshotReservePercentPtr      *int     `xml:"snapshot-reserve-percent"`
}

// NewAggrSnapshotAttributesType is a factory method for creating new instances of AggrSnapshotAttributesType objects
func NewAggrSnapshotAttributesType() *AggrSnapshotAttributesType {
	return &AggrSnapshotAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrSnapshotAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrSnapshotAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// FilesTotal is a 'getter' method
func (o *AggrSnapshotAttributesType) FilesTotal() int {
	r := *o.FilesTotalPtr
	return r
}

// SetFilesTotal is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetFilesTotal(newValue int) *AggrSnapshotAttributesType {
	o.FilesTotalPtr = &newValue
	return o
}

// FilesUsed is a 'getter' method
func (o *AggrSnapshotAttributesType) FilesUsed() int {
	r := *o.FilesUsedPtr
	return r
}

// SetFilesUsed is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetFilesUsed(newValue int) *AggrSnapshotAttributesType {
	o.FilesUsedPtr = &newValue
	return o
}

// InofileVersion is a 'getter' method
func (o *AggrSnapshotAttributesType) InofileVersion() int {
	r := *o.InofileVersionPtr
	return r
}

// SetInofileVersion is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetInofileVersion(newValue int) *AggrSnapshotAttributesType {
	o.InofileVersionPtr = &newValue
	return o
}

// IsSnapshotAutoCreateEnabled is a 'getter' method
func (o *AggrSnapshotAttributesType) IsSnapshotAutoCreateEnabled() bool {
	r := *o.IsSnapshotAutoCreateEnabledPtr
	return r
}

// SetIsSnapshotAutoCreateEnabled is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetIsSnapshotAutoCreateEnabled(newValue bool) *AggrSnapshotAttributesType {
	o.IsSnapshotAutoCreateEnabledPtr = &newValue
	return o
}

// IsSnapshotAutoDeleteEnabled is a 'getter' method
func (o *AggrSnapshotAttributesType) IsSnapshotAutoDeleteEnabled() bool {
	r := *o.IsSnapshotAutoDeleteEnabledPtr
	return r
}

// SetIsSnapshotAutoDeleteEnabled is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetIsSnapshotAutoDeleteEnabled(newValue bool) *AggrSnapshotAttributesType {
	o.IsSnapshotAutoDeleteEnabledPtr = &newValue
	return o
}

// MaxfilesAvailable is a 'getter' method
func (o *AggrSnapshotAttributesType) MaxfilesAvailable() int {
	r := *o.MaxfilesAvailablePtr
	return r
}

// SetMaxfilesAvailable is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetMaxfilesAvailable(newValue int) *AggrSnapshotAttributesType {
	o.MaxfilesAvailablePtr = &newValue
	return o
}

// MaxfilesPossible is a 'getter' method
func (o *AggrSnapshotAttributesType) MaxfilesPossible() int {
	r := *o.MaxfilesPossiblePtr
	return r
}

// SetMaxfilesPossible is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetMaxfilesPossible(newValue int) *AggrSnapshotAttributesType {
	o.MaxfilesPossiblePtr = &newValue
	return o
}

// MaxfilesUsed is a 'getter' method
func (o *AggrSnapshotAttributesType) MaxfilesUsed() int {
	r := *o.MaxfilesUsedPtr
	return r
}

// SetMaxfilesUsed is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetMaxfilesUsed(newValue int) *AggrSnapshotAttributesType {
	o.MaxfilesUsedPtr = &newValue
	return o
}

// PercentInodeUsedCapacity is a 'getter' method
func (o *AggrSnapshotAttributesType) PercentInodeUsedCapacity() int {
	r := *o.PercentInodeUsedCapacityPtr
	return r
}

// SetPercentInodeUsedCapacity is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetPercentInodeUsedCapacity(newValue int) *AggrSnapshotAttributesType {
	o.PercentInodeUsedCapacityPtr = &newValue
	return o
}

// PercentUsedCapacity is a 'getter' method
func (o *AggrSnapshotAttributesType) PercentUsedCapacity() int {
	r := *o.PercentUsedCapacityPtr
	return r
}

// SetPercentUsedCapacity is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetPercentUsedCapacity(newValue int) *AggrSnapshotAttributesType {
	o.PercentUsedCapacityPtr = &newValue
	return o
}

// SizeAvailable is a 'getter' method
func (o *AggrSnapshotAttributesType) SizeAvailable() int {
	r := *o.SizeAvailablePtr
	return r
}

// SetSizeAvailable is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetSizeAvailable(newValue int) *AggrSnapshotAttributesType {
	o.SizeAvailablePtr = &newValue
	return o
}

// SizeTotal is a 'getter' method
func (o *AggrSnapshotAttributesType) SizeTotal() int {
	r := *o.SizeTotalPtr
	return r
}

// SetSizeTotal is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetSizeTotal(newValue int) *AggrSnapshotAttributesType {
	o.SizeTotalPtr = &newValue
	return o
}

// SizeUsed is a 'getter' method
func (o *AggrSnapshotAttributesType) SizeUsed() int {
	r := *o.SizeUsedPtr
	return r
}

// SetSizeUsed is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetSizeUsed(newValue int) *AggrSnapshotAttributesType {
	o.SizeUsedPtr = &newValue
	return o
}

// SnapshotReservePercent is a 'getter' method
func (o *AggrSnapshotAttributesType) SnapshotReservePercent() int {
	r := *o.SnapshotReservePercentPtr
	return r
}

// SetSnapshotReservePercent is a fluent style 'setter' method that can be chained
func (o *AggrSnapshotAttributesType) SetSnapshotReservePercent(newValue int) *AggrSnapshotAttributesType {
	o.SnapshotReservePercentPtr = &newValue
	return o
}
