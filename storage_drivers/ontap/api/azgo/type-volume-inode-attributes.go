package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeInodeAttributesType is a structure to represent a volume-inode-attributes ZAPI object
type VolumeInodeAttributesType struct {
	XMLName                     xml.Name `xml:"volume-inode-attributes"`
	BlockTypePtr                *string  `xml:"block-type"`
	FilesPrivateUsedPtr         *int     `xml:"files-private-used"`
	FilesTotalPtr               *int     `xml:"files-total"`
	FilesUsedPtr                *int     `xml:"files-used"`
	InodefilePrivateCapacityPtr *int     `xml:"inodefile-private-capacity"`
	InodefilePublicCapacityPtr  *int     `xml:"inodefile-public-capacity"`
	InofileVersionPtr           *int     `xml:"inofile-version"`
}

// NewVolumeInodeAttributesType is a factory method for creating new instances of VolumeInodeAttributesType objects
func NewVolumeInodeAttributesType() *VolumeInodeAttributesType {
	return &VolumeInodeAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeInodeAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeInodeAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// BlockType is a 'getter' method
func (o *VolumeInodeAttributesType) BlockType() string {
	r := *o.BlockTypePtr
	return r
}

// SetBlockType is a fluent style 'setter' method that can be chained
func (o *VolumeInodeAttributesType) SetBlockType(newValue string) *VolumeInodeAttributesType {
	o.BlockTypePtr = &newValue
	return o
}

// FilesPrivateUsed is a 'getter' method
func (o *VolumeInodeAttributesType) FilesPrivateUsed() int {
	r := *o.FilesPrivateUsedPtr
	return r
}

// SetFilesPrivateUsed is a fluent style 'setter' method that can be chained
func (o *VolumeInodeAttributesType) SetFilesPrivateUsed(newValue int) *VolumeInodeAttributesType {
	o.FilesPrivateUsedPtr = &newValue
	return o
}

// FilesTotal is a 'getter' method
func (o *VolumeInodeAttributesType) FilesTotal() int {
	r := *o.FilesTotalPtr
	return r
}

// SetFilesTotal is a fluent style 'setter' method that can be chained
func (o *VolumeInodeAttributesType) SetFilesTotal(newValue int) *VolumeInodeAttributesType {
	o.FilesTotalPtr = &newValue
	return o
}

// FilesUsed is a 'getter' method
func (o *VolumeInodeAttributesType) FilesUsed() int {
	r := *o.FilesUsedPtr
	return r
}

// SetFilesUsed is a fluent style 'setter' method that can be chained
func (o *VolumeInodeAttributesType) SetFilesUsed(newValue int) *VolumeInodeAttributesType {
	o.FilesUsedPtr = &newValue
	return o
}

// InodefilePrivateCapacity is a 'getter' method
func (o *VolumeInodeAttributesType) InodefilePrivateCapacity() int {
	r := *o.InodefilePrivateCapacityPtr
	return r
}

// SetInodefilePrivateCapacity is a fluent style 'setter' method that can be chained
func (o *VolumeInodeAttributesType) SetInodefilePrivateCapacity(newValue int) *VolumeInodeAttributesType {
	o.InodefilePrivateCapacityPtr = &newValue
	return o
}

// InodefilePublicCapacity is a 'getter' method
func (o *VolumeInodeAttributesType) InodefilePublicCapacity() int {
	r := *o.InodefilePublicCapacityPtr
	return r
}

// SetInodefilePublicCapacity is a fluent style 'setter' method that can be chained
func (o *VolumeInodeAttributesType) SetInodefilePublicCapacity(newValue int) *VolumeInodeAttributesType {
	o.InodefilePublicCapacityPtr = &newValue
	return o
}

// InofileVersion is a 'getter' method
func (o *VolumeInodeAttributesType) InofileVersion() int {
	r := *o.InofileVersionPtr
	return r
}

// SetInofileVersion is a fluent style 'setter' method that can be chained
func (o *VolumeInodeAttributesType) SetInofileVersion(newValue int) *VolumeInodeAttributesType {
	o.InofileVersionPtr = &newValue
	return o
}
