// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrInodeAttributesType is a structure to represent a aggr-inode-attributes ZAPI object
type AggrInodeAttributesType struct {
	XMLName                     xml.Name `xml:"aggr-inode-attributes"`
	FilesPrivateUsedPtr         *int     `xml:"files-private-used"`
	FilesTotalPtr               *int     `xml:"files-total"`
	FilesUsedPtr                *int     `xml:"files-used"`
	InodefilePrivateCapacityPtr *int     `xml:"inodefile-private-capacity"`
	InodefilePublicCapacityPtr  *int     `xml:"inodefile-public-capacity"`
	InofileVersionPtr           *int     `xml:"inofile-version"`
	MaxfilesAvailablePtr        *int     `xml:"maxfiles-available"`
	MaxfilesPossiblePtr         *int     `xml:"maxfiles-possible"`
	MaxfilesUsedPtr             *int     `xml:"maxfiles-used"`
	PercentInodeUsedCapacityPtr *int     `xml:"percent-inode-used-capacity"`
}

// NewAggrInodeAttributesType is a factory method for creating new instances of AggrInodeAttributesType objects
func NewAggrInodeAttributesType() *AggrInodeAttributesType {
	return &AggrInodeAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrInodeAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrInodeAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// FilesPrivateUsed is a 'getter' method
func (o *AggrInodeAttributesType) FilesPrivateUsed() int {
	r := *o.FilesPrivateUsedPtr
	return r
}

// SetFilesPrivateUsed is a fluent style 'setter' method that can be chained
func (o *AggrInodeAttributesType) SetFilesPrivateUsed(newValue int) *AggrInodeAttributesType {
	o.FilesPrivateUsedPtr = &newValue
	return o
}

// FilesTotal is a 'getter' method
func (o *AggrInodeAttributesType) FilesTotal() int {
	r := *o.FilesTotalPtr
	return r
}

// SetFilesTotal is a fluent style 'setter' method that can be chained
func (o *AggrInodeAttributesType) SetFilesTotal(newValue int) *AggrInodeAttributesType {
	o.FilesTotalPtr = &newValue
	return o
}

// FilesUsed is a 'getter' method
func (o *AggrInodeAttributesType) FilesUsed() int {
	r := *o.FilesUsedPtr
	return r
}

// SetFilesUsed is a fluent style 'setter' method that can be chained
func (o *AggrInodeAttributesType) SetFilesUsed(newValue int) *AggrInodeAttributesType {
	o.FilesUsedPtr = &newValue
	return o
}

// InodefilePrivateCapacity is a 'getter' method
func (o *AggrInodeAttributesType) InodefilePrivateCapacity() int {
	r := *o.InodefilePrivateCapacityPtr
	return r
}

// SetInodefilePrivateCapacity is a fluent style 'setter' method that can be chained
func (o *AggrInodeAttributesType) SetInodefilePrivateCapacity(newValue int) *AggrInodeAttributesType {
	o.InodefilePrivateCapacityPtr = &newValue
	return o
}

// InodefilePublicCapacity is a 'getter' method
func (o *AggrInodeAttributesType) InodefilePublicCapacity() int {
	r := *o.InodefilePublicCapacityPtr
	return r
}

// SetInodefilePublicCapacity is a fluent style 'setter' method that can be chained
func (o *AggrInodeAttributesType) SetInodefilePublicCapacity(newValue int) *AggrInodeAttributesType {
	o.InodefilePublicCapacityPtr = &newValue
	return o
}

// InofileVersion is a 'getter' method
func (o *AggrInodeAttributesType) InofileVersion() int {
	r := *o.InofileVersionPtr
	return r
}

// SetInofileVersion is a fluent style 'setter' method that can be chained
func (o *AggrInodeAttributesType) SetInofileVersion(newValue int) *AggrInodeAttributesType {
	o.InofileVersionPtr = &newValue
	return o
}

// MaxfilesAvailable is a 'getter' method
func (o *AggrInodeAttributesType) MaxfilesAvailable() int {
	r := *o.MaxfilesAvailablePtr
	return r
}

// SetMaxfilesAvailable is a fluent style 'setter' method that can be chained
func (o *AggrInodeAttributesType) SetMaxfilesAvailable(newValue int) *AggrInodeAttributesType {
	o.MaxfilesAvailablePtr = &newValue
	return o
}

// MaxfilesPossible is a 'getter' method
func (o *AggrInodeAttributesType) MaxfilesPossible() int {
	r := *o.MaxfilesPossiblePtr
	return r
}

// SetMaxfilesPossible is a fluent style 'setter' method that can be chained
func (o *AggrInodeAttributesType) SetMaxfilesPossible(newValue int) *AggrInodeAttributesType {
	o.MaxfilesPossiblePtr = &newValue
	return o
}

// MaxfilesUsed is a 'getter' method
func (o *AggrInodeAttributesType) MaxfilesUsed() int {
	r := *o.MaxfilesUsedPtr
	return r
}

// SetMaxfilesUsed is a fluent style 'setter' method that can be chained
func (o *AggrInodeAttributesType) SetMaxfilesUsed(newValue int) *AggrInodeAttributesType {
	o.MaxfilesUsedPtr = &newValue
	return o
}

// PercentInodeUsedCapacity is a 'getter' method
func (o *AggrInodeAttributesType) PercentInodeUsedCapacity() int {
	r := *o.PercentInodeUsedCapacityPtr
	return r
}

// SetPercentInodeUsedCapacity is a fluent style 'setter' method that can be chained
func (o *AggrInodeAttributesType) SetPercentInodeUsedCapacity(newValue int) *AggrInodeAttributesType {
	o.PercentInodeUsedCapacityPtr = &newValue
	return o
}
