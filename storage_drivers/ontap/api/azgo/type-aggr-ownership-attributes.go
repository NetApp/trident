// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrOwnershipAttributesType is a structure to represent a aggr-ownership-attributes ZAPI object
type AggrOwnershipAttributesType struct {
	XMLName       xml.Name `xml:"aggr-ownership-attributes"`
	ClusterPtr    *string  `xml:"cluster"`
	DrHomeIdPtr   *int     `xml:"dr-home-id"`
	DrHomeNamePtr *string  `xml:"dr-home-name"`
	HomeIdPtr     *int     `xml:"home-id"`
	HomeNamePtr   *string  `xml:"home-name"`
	OwnerIdPtr    *int     `xml:"owner-id"`
	OwnerNamePtr  *string  `xml:"owner-name"`
}

// NewAggrOwnershipAttributesType is a factory method for creating new instances of AggrOwnershipAttributesType objects
func NewAggrOwnershipAttributesType() *AggrOwnershipAttributesType {
	return &AggrOwnershipAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrOwnershipAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrOwnershipAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Cluster is a 'getter' method
func (o *AggrOwnershipAttributesType) Cluster() string {
	r := *o.ClusterPtr
	return r
}

// SetCluster is a fluent style 'setter' method that can be chained
func (o *AggrOwnershipAttributesType) SetCluster(newValue string) *AggrOwnershipAttributesType {
	o.ClusterPtr = &newValue
	return o
}

// DrHomeId is a 'getter' method
func (o *AggrOwnershipAttributesType) DrHomeId() int {
	r := *o.DrHomeIdPtr
	return r
}

// SetDrHomeId is a fluent style 'setter' method that can be chained
func (o *AggrOwnershipAttributesType) SetDrHomeId(newValue int) *AggrOwnershipAttributesType {
	o.DrHomeIdPtr = &newValue
	return o
}

// DrHomeName is a 'getter' method
func (o *AggrOwnershipAttributesType) DrHomeName() string {
	r := *o.DrHomeNamePtr
	return r
}

// SetDrHomeName is a fluent style 'setter' method that can be chained
func (o *AggrOwnershipAttributesType) SetDrHomeName(newValue string) *AggrOwnershipAttributesType {
	o.DrHomeNamePtr = &newValue
	return o
}

// HomeId is a 'getter' method
func (o *AggrOwnershipAttributesType) HomeId() int {
	r := *o.HomeIdPtr
	return r
}

// SetHomeId is a fluent style 'setter' method that can be chained
func (o *AggrOwnershipAttributesType) SetHomeId(newValue int) *AggrOwnershipAttributesType {
	o.HomeIdPtr = &newValue
	return o
}

// HomeName is a 'getter' method
func (o *AggrOwnershipAttributesType) HomeName() string {
	r := *o.HomeNamePtr
	return r
}

// SetHomeName is a fluent style 'setter' method that can be chained
func (o *AggrOwnershipAttributesType) SetHomeName(newValue string) *AggrOwnershipAttributesType {
	o.HomeNamePtr = &newValue
	return o
}

// OwnerId is a 'getter' method
func (o *AggrOwnershipAttributesType) OwnerId() int {
	r := *o.OwnerIdPtr
	return r
}

// SetOwnerId is a fluent style 'setter' method that can be chained
func (o *AggrOwnershipAttributesType) SetOwnerId(newValue int) *AggrOwnershipAttributesType {
	o.OwnerIdPtr = &newValue
	return o
}

// OwnerName is a 'getter' method
func (o *AggrOwnershipAttributesType) OwnerName() string {
	r := *o.OwnerNamePtr
	return r
}

// SetOwnerName is a fluent style 'setter' method that can be chained
func (o *AggrOwnershipAttributesType) SetOwnerName(newValue string) *AggrOwnershipAttributesType {
	o.OwnerNamePtr = &newValue
	return o
}
