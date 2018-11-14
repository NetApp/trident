package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SystemVersionTupleType is a structure to represent a system-version-tuple ZAPI object
type SystemVersionTupleType struct {
	XMLName       xml.Name `xml:"system-version-tuple"`
	GenerationPtr *int     `xml:"generation"`
	MajorPtr      *int     `xml:"major"`
	MinorPtr      *int     `xml:"minor"`
}

// NewSystemVersionTupleType is a factory method for creating new instances of SystemVersionTupleType objects
func NewSystemVersionTupleType() *SystemVersionTupleType {
	return &SystemVersionTupleType{}
}

// ToXML converts this object into an xml string representation
func (o *SystemVersionTupleType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SystemVersionTupleType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Generation is a 'getter' method
func (o *SystemVersionTupleType) Generation() int {
	r := *o.GenerationPtr
	return r
}

// SetGeneration is a fluent style 'setter' method that can be chained
func (o *SystemVersionTupleType) SetGeneration(newValue int) *SystemVersionTupleType {
	o.GenerationPtr = &newValue
	return o
}

// Major is a 'getter' method
func (o *SystemVersionTupleType) Major() int {
	r := *o.MajorPtr
	return r
}

// SetMajor is a fluent style 'setter' method that can be chained
func (o *SystemVersionTupleType) SetMajor(newValue int) *SystemVersionTupleType {
	o.MajorPtr = &newValue
	return o
}

// Minor is a 'getter' method
func (o *SystemVersionTupleType) Minor() int {
	r := *o.MinorPtr
	return r
}

// SetMinor is a fluent style 'setter' method that can be chained
func (o *SystemVersionTupleType) SetMinor(newValue int) *SystemVersionTupleType {
	o.MinorPtr = &newValue
	return o
}
