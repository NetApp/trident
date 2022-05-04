// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrVolumeCountAttributesType is a structure to represent a aggr-volume-count-attributes ZAPI object
type AggrVolumeCountAttributesType struct {
	XMLName                   xml.Name `xml:"aggr-volume-count-attributes"`
	FlexvolCountPtr           *int     `xml:"flexvol-count"`
	FlexvolCountCollectivePtr *int     `xml:"flexvol-count-collective"`
	FlexvolCountNotOnlinePtr  *int     `xml:"flexvol-count-not-online"`
	FlexvolCountQuiescedPtr   *int     `xml:"flexvol-count-quiesced"`
	FlexvolCountStripedPtr    *int     `xml:"flexvol-count-striped"`
}

// NewAggrVolumeCountAttributesType is a factory method for creating new instances of AggrVolumeCountAttributesType objects
func NewAggrVolumeCountAttributesType() *AggrVolumeCountAttributesType {
	return &AggrVolumeCountAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrVolumeCountAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrVolumeCountAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// FlexvolCount is a 'getter' method
func (o *AggrVolumeCountAttributesType) FlexvolCount() int {
	r := *o.FlexvolCountPtr
	return r
}

// SetFlexvolCount is a fluent style 'setter' method that can be chained
func (o *AggrVolumeCountAttributesType) SetFlexvolCount(newValue int) *AggrVolumeCountAttributesType {
	o.FlexvolCountPtr = &newValue
	return o
}

// FlexvolCountCollective is a 'getter' method
func (o *AggrVolumeCountAttributesType) FlexvolCountCollective() int {
	r := *o.FlexvolCountCollectivePtr
	return r
}

// SetFlexvolCountCollective is a fluent style 'setter' method that can be chained
func (o *AggrVolumeCountAttributesType) SetFlexvolCountCollective(newValue int) *AggrVolumeCountAttributesType {
	o.FlexvolCountCollectivePtr = &newValue
	return o
}

// FlexvolCountNotOnline is a 'getter' method
func (o *AggrVolumeCountAttributesType) FlexvolCountNotOnline() int {
	r := *o.FlexvolCountNotOnlinePtr
	return r
}

// SetFlexvolCountNotOnline is a fluent style 'setter' method that can be chained
func (o *AggrVolumeCountAttributesType) SetFlexvolCountNotOnline(newValue int) *AggrVolumeCountAttributesType {
	o.FlexvolCountNotOnlinePtr = &newValue
	return o
}

// FlexvolCountQuiesced is a 'getter' method
func (o *AggrVolumeCountAttributesType) FlexvolCountQuiesced() int {
	r := *o.FlexvolCountQuiescedPtr
	return r
}

// SetFlexvolCountQuiesced is a fluent style 'setter' method that can be chained
func (o *AggrVolumeCountAttributesType) SetFlexvolCountQuiesced(newValue int) *AggrVolumeCountAttributesType {
	o.FlexvolCountQuiescedPtr = &newValue
	return o
}

// FlexvolCountStriped is a 'getter' method
func (o *AggrVolumeCountAttributesType) FlexvolCountStriped() int {
	r := *o.FlexvolCountStripedPtr
	return r
}

// SetFlexvolCountStriped is a fluent style 'setter' method that can be chained
func (o *AggrVolumeCountAttributesType) SetFlexvolCountStriped(newValue int) *AggrVolumeCountAttributesType {
	o.FlexvolCountStripedPtr = &newValue
	return o
}
