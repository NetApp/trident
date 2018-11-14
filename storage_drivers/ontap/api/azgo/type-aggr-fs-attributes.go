package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrFsAttributesType is a structure to represent a aggr-fs-attributes ZAPI object
type AggrFsAttributesType struct {
	XMLName      xml.Name `xml:"aggr-fs-attributes"`
	BlockTypePtr *string  `xml:"block-type"`
	FsidPtr      *int     `xml:"fsid"`
	TypePtr      *string  `xml:"type"`
}

// NewAggrFsAttributesType is a factory method for creating new instances of AggrFsAttributesType objects
func NewAggrFsAttributesType() *AggrFsAttributesType {
	return &AggrFsAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrFsAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrFsAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// BlockType is a 'getter' method
func (o *AggrFsAttributesType) BlockType() string {
	r := *o.BlockTypePtr
	return r
}

// SetBlockType is a fluent style 'setter' method that can be chained
func (o *AggrFsAttributesType) SetBlockType(newValue string) *AggrFsAttributesType {
	o.BlockTypePtr = &newValue
	return o
}

// Fsid is a 'getter' method
func (o *AggrFsAttributesType) Fsid() int {
	r := *o.FsidPtr
	return r
}

// SetFsid is a fluent style 'setter' method that can be chained
func (o *AggrFsAttributesType) SetFsid(newValue int) *AggrFsAttributesType {
	o.FsidPtr = &newValue
	return o
}

// Type is a 'getter' method
func (o *AggrFsAttributesType) Type() string {
	r := *o.TypePtr
	return r
}

// SetType is a fluent style 'setter' method that can be chained
func (o *AggrFsAttributesType) SetType(newValue string) *AggrFsAttributesType {
	o.TypePtr = &newValue
	return o
}
