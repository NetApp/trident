// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrCheckAttributesType is a structure to represent a aggr-check-attributes ZAPI object
type AggrCheckAttributesType struct {
	XMLName                    xml.Name `xml:"aggr-check-attributes"`
	AddedSpacePtr              *int     `xml:"added-space"`
	CheckLastErrnoPtr          *int     `xml:"check-last-errno"`
	CookiePtr                  *int     `xml:"cookie"`
	IsSpaceEstimateCompletePtr *bool    `xml:"is-space-estimate-complete"`
}

// NewAggrCheckAttributesType is a factory method for creating new instances of AggrCheckAttributesType objects
func NewAggrCheckAttributesType() *AggrCheckAttributesType {
	return &AggrCheckAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrCheckAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrCheckAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AddedSpace is a 'getter' method
func (o *AggrCheckAttributesType) AddedSpace() int {
	r := *o.AddedSpacePtr
	return r
}

// SetAddedSpace is a fluent style 'setter' method that can be chained
func (o *AggrCheckAttributesType) SetAddedSpace(newValue int) *AggrCheckAttributesType {
	o.AddedSpacePtr = &newValue
	return o
}

// CheckLastErrno is a 'getter' method
func (o *AggrCheckAttributesType) CheckLastErrno() int {
	r := *o.CheckLastErrnoPtr
	return r
}

// SetCheckLastErrno is a fluent style 'setter' method that can be chained
func (o *AggrCheckAttributesType) SetCheckLastErrno(newValue int) *AggrCheckAttributesType {
	o.CheckLastErrnoPtr = &newValue
	return o
}

// Cookie is a 'getter' method
func (o *AggrCheckAttributesType) Cookie() int {
	r := *o.CookiePtr
	return r
}

// SetCookie is a fluent style 'setter' method that can be chained
func (o *AggrCheckAttributesType) SetCookie(newValue int) *AggrCheckAttributesType {
	o.CookiePtr = &newValue
	return o
}

// IsSpaceEstimateComplete is a 'getter' method
func (o *AggrCheckAttributesType) IsSpaceEstimateComplete() bool {
	r := *o.IsSpaceEstimateCompletePtr
	return r
}

// SetIsSpaceEstimateComplete is a fluent style 'setter' method that can be chained
func (o *AggrCheckAttributesType) SetIsSpaceEstimateComplete(newValue bool) *AggrCheckAttributesType {
	o.IsSpaceEstimateCompletePtr = &newValue
	return o
}
