package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeSecurityUnixAttributesType is a structure to represent a volume-security-unix-attributes ZAPI object
type VolumeSecurityUnixAttributesType struct {
	XMLName        xml.Name `xml:"volume-security-unix-attributes"`
	GroupIdPtr     *int     `xml:"group-id"`
	PermissionsPtr *string  `xml:"permissions"`
	UserIdPtr      *int     `xml:"user-id"`
}

// NewVolumeSecurityUnixAttributesType is a factory method for creating new instances of VolumeSecurityUnixAttributesType objects
func NewVolumeSecurityUnixAttributesType() *VolumeSecurityUnixAttributesType {
	return &VolumeSecurityUnixAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeSecurityUnixAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeSecurityUnixAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// GroupId is a 'getter' method
func (o *VolumeSecurityUnixAttributesType) GroupId() int {
	r := *o.GroupIdPtr
	return r
}

// SetGroupId is a fluent style 'setter' method that can be chained
func (o *VolumeSecurityUnixAttributesType) SetGroupId(newValue int) *VolumeSecurityUnixAttributesType {
	o.GroupIdPtr = &newValue
	return o
}

// Permissions is a 'getter' method
func (o *VolumeSecurityUnixAttributesType) Permissions() string {
	r := *o.PermissionsPtr
	return r
}

// SetPermissions is a fluent style 'setter' method that can be chained
func (o *VolumeSecurityUnixAttributesType) SetPermissions(newValue string) *VolumeSecurityUnixAttributesType {
	o.PermissionsPtr = &newValue
	return o
}

// UserId is a 'getter' method
func (o *VolumeSecurityUnixAttributesType) UserId() int {
	r := *o.UserIdPtr
	return r
}

// SetUserId is a fluent style 'setter' method that can be chained
func (o *VolumeSecurityUnixAttributesType) SetUserId(newValue int) *VolumeSecurityUnixAttributesType {
	o.UserIdPtr = &newValue
	return o
}
