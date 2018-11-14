package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeDirectoryAttributesType is a structure to represent a volume-directory-attributes ZAPI object
type VolumeDirectoryAttributesType struct {
	XMLName       xml.Name `xml:"volume-directory-attributes"`
	I2pEnabledPtr *bool    `xml:"i2p-enabled"`
	MaxDirSizePtr *int     `xml:"max-dir-size"`
	RootDirGenPtr *string  `xml:"root-dir-gen"`
}

// NewVolumeDirectoryAttributesType is a factory method for creating new instances of VolumeDirectoryAttributesType objects
func NewVolumeDirectoryAttributesType() *VolumeDirectoryAttributesType {
	return &VolumeDirectoryAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeDirectoryAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeDirectoryAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// I2pEnabled is a 'getter' method
func (o *VolumeDirectoryAttributesType) I2pEnabled() bool {
	r := *o.I2pEnabledPtr
	return r
}

// SetI2pEnabled is a fluent style 'setter' method that can be chained
func (o *VolumeDirectoryAttributesType) SetI2pEnabled(newValue bool) *VolumeDirectoryAttributesType {
	o.I2pEnabledPtr = &newValue
	return o
}

// MaxDirSize is a 'getter' method
func (o *VolumeDirectoryAttributesType) MaxDirSize() int {
	r := *o.MaxDirSizePtr
	return r
}

// SetMaxDirSize is a fluent style 'setter' method that can be chained
func (o *VolumeDirectoryAttributesType) SetMaxDirSize(newValue int) *VolumeDirectoryAttributesType {
	o.MaxDirSizePtr = &newValue
	return o
}

// RootDirGen is a 'getter' method
func (o *VolumeDirectoryAttributesType) RootDirGen() string {
	r := *o.RootDirGenPtr
	return r
}

// SetRootDirGen is a fluent style 'setter' method that can be chained
func (o *VolumeDirectoryAttributesType) SetRootDirGen(newValue string) *VolumeDirectoryAttributesType {
	o.RootDirGenPtr = &newValue
	return o
}
