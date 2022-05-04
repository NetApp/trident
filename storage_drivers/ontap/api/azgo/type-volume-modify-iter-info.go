// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeModifyIterInfoType is a structure to represent a volume-modify-iter-info ZAPI object
type VolumeModifyIterInfoType struct {
	XMLName         xml.Name                           `xml:"volume-modify-iter-info"`
	ErrorCodePtr    *int                               `xml:"error-code"`
	ErrorMessagePtr *string                            `xml:"error-message"`
	VolumeKeyPtr    *VolumeModifyIterInfoTypeVolumeKey `xml:"volume-key"`
	// work in progress
}

// NewVolumeModifyIterInfoType is a factory method for creating new instances of VolumeModifyIterInfoType objects
func NewVolumeModifyIterInfoType() *VolumeModifyIterInfoType {
	return &VolumeModifyIterInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeModifyIterInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeModifyIterInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// ErrorCode is a 'getter' method
func (o *VolumeModifyIterInfoType) ErrorCode() int {
	r := *o.ErrorCodePtr
	return r
}

// SetErrorCode is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterInfoType) SetErrorCode(newValue int) *VolumeModifyIterInfoType {
	o.ErrorCodePtr = &newValue
	return o
}

// ErrorMessage is a 'getter' method
func (o *VolumeModifyIterInfoType) ErrorMessage() string {
	r := *o.ErrorMessagePtr
	return r
}

// SetErrorMessage is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterInfoType) SetErrorMessage(newValue string) *VolumeModifyIterInfoType {
	o.ErrorMessagePtr = &newValue
	return o
}

// VolumeModifyIterInfoTypeVolumeKey is a wrapper
type VolumeModifyIterInfoTypeVolumeKey struct {
	XMLName             xml.Name              `xml:"volume-key"`
	VolumeAttributesPtr *VolumeAttributesType `xml:"volume-attributes"`
}

// VolumeAttributes is a 'getter' method
func (o *VolumeModifyIterInfoTypeVolumeKey) VolumeAttributes() VolumeAttributesType {
	r := *o.VolumeAttributesPtr
	return r
}

// SetVolumeAttributes is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterInfoTypeVolumeKey) SetVolumeAttributes(newValue VolumeAttributesType) *VolumeModifyIterInfoTypeVolumeKey {
	o.VolumeAttributesPtr = &newValue
	return o
}

// VolumeKey is a 'getter' method
func (o *VolumeModifyIterInfoType) VolumeKey() VolumeModifyIterInfoTypeVolumeKey {
	r := *o.VolumeKeyPtr
	return r
}

// SetVolumeKey is a fluent style 'setter' method that can be chained
func (o *VolumeModifyIterInfoType) SetVolumeKey(newValue VolumeModifyIterInfoTypeVolumeKey) *VolumeModifyIterInfoType {
	o.VolumeKeyPtr = &newValue
	return o
}
