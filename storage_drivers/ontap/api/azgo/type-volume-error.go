// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeErrorType is a structure to represent a volume-error ZAPI object
type VolumeErrorType struct {
	XMLName    xml.Name        `xml:"volume-error"`
	ErrnoPtr   *int            `xml:"errno"`
	NamePtr    *VolumeNameType `xml:"name"`
	ReasonPtr  *string         `xml:"reason"`
	VserverPtr *string         `xml:"vserver"`
}

// NewVolumeErrorType is a factory method for creating new instances of VolumeErrorType objects
func NewVolumeErrorType() *VolumeErrorType {
	return &VolumeErrorType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeErrorType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeErrorType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Errno is a 'getter' method
func (o *VolumeErrorType) Errno() int {
	r := *o.ErrnoPtr
	return r
}

// SetErrno is a fluent style 'setter' method that can be chained
func (o *VolumeErrorType) SetErrno(newValue int) *VolumeErrorType {
	o.ErrnoPtr = &newValue
	return o
}

// Name is a 'getter' method
func (o *VolumeErrorType) Name() VolumeNameType {
	r := *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *VolumeErrorType) SetName(newValue VolumeNameType) *VolumeErrorType {
	o.NamePtr = &newValue
	return o
}

// Reason is a 'getter' method
func (o *VolumeErrorType) Reason() string {
	r := *o.ReasonPtr
	return r
}

// SetReason is a fluent style 'setter' method that can be chained
func (o *VolumeErrorType) SetReason(newValue string) *VolumeErrorType {
	o.ReasonPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *VolumeErrorType) Vserver() string {
	r := *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *VolumeErrorType) SetVserver(newValue string) *VolumeErrorType {
	o.VserverPtr = &newValue
	return o
}
