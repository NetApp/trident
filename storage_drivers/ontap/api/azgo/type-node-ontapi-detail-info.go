package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// NodeOntapiDetailInfoType is a structure to represent a node-ontapi-detail-info ZAPI object
type NodeOntapiDetailInfoType struct {
	XMLName         xml.Name `xml:"node-ontapi-detail-info"`
	MajorVersionPtr *int     `xml:"major-version"`
	MinorVersionPtr *int     `xml:"minor-version"`
	NodeNamePtr     *string  `xml:"node-name"`
	NodeUuidPtr     *string  `xml:"node-uuid"`
}

// NewNodeOntapiDetailInfoType is a factory method for creating new instances of NodeOntapiDetailInfoType objects
func NewNodeOntapiDetailInfoType() *NodeOntapiDetailInfoType {
	return &NodeOntapiDetailInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NodeOntapiDetailInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NodeOntapiDetailInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// MajorVersion is a 'getter' method
func (o *NodeOntapiDetailInfoType) MajorVersion() int {
	r := *o.MajorVersionPtr
	return r
}

// SetMajorVersion is a fluent style 'setter' method that can be chained
func (o *NodeOntapiDetailInfoType) SetMajorVersion(newValue int) *NodeOntapiDetailInfoType {
	o.MajorVersionPtr = &newValue
	return o
}

// MinorVersion is a 'getter' method
func (o *NodeOntapiDetailInfoType) MinorVersion() int {
	r := *o.MinorVersionPtr
	return r
}

// SetMinorVersion is a fluent style 'setter' method that can be chained
func (o *NodeOntapiDetailInfoType) SetMinorVersion(newValue int) *NodeOntapiDetailInfoType {
	o.MinorVersionPtr = &newValue
	return o
}

// NodeName is a 'getter' method
func (o *NodeOntapiDetailInfoType) NodeName() string {
	r := *o.NodeNamePtr
	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *NodeOntapiDetailInfoType) SetNodeName(newValue string) *NodeOntapiDetailInfoType {
	o.NodeNamePtr = &newValue
	return o
}

// NodeUuid is a 'getter' method
func (o *NodeOntapiDetailInfoType) NodeUuid() string {
	r := *o.NodeUuidPtr
	return r
}

// SetNodeUuid is a fluent style 'setter' method that can be chained
func (o *NodeOntapiDetailInfoType) SetNodeUuid(newValue string) *NodeOntapiDetailInfoType {
	o.NodeUuidPtr = &newValue
	return o
}
