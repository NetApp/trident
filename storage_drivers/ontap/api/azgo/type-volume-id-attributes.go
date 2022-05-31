// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VolumeIdAttributesType is a structure to represent a volume-id-attributes ZAPI object
type VolumeIdAttributesType struct {
	XMLName     xml.Name                        `xml:"volume-id-attributes"`
	AggrListPtr *VolumeIdAttributesTypeAggrList `xml:"aggr-list"`
	// work in progress
	ApplicationPtr             *string                      `xml:"application"`
	ApplicationUuidPtr         *UuidType                    `xml:"application-uuid"`
	CommentPtr                 *string                      `xml:"comment"`
	ContainingAggregateNamePtr *string                      `xml:"containing-aggregate-name"`
	ContainingAggregateUuidPtr *UuidType                    `xml:"containing-aggregate-uuid"`
	CreationTimePtr            *int                         `xml:"creation-time"`
	DsidPtr                    *int                         `xml:"dsid"`
	ExtentSizePtr              *string                      `xml:"extent-size"`
	FlexcacheEndpointTypePtr   *string                      `xml:"flexcache-endpoint-type"`
	FlexgroupIndexPtr          *int                         `xml:"flexgroup-index"`
	FlexgroupMsidPtr           *int                         `xml:"flexgroup-msid"`
	FlexgroupUuidPtr           *UuidType                    `xml:"flexgroup-uuid"`
	FsidPtr                    *string                      `xml:"fsid"`
	InstanceUuidPtr            *UuidType                    `xml:"instance-uuid"`
	JunctionParentNamePtr      *VolumeNameType              `xml:"junction-parent-name"`
	JunctionPathPtr            *JunctionPathType            `xml:"junction-path"`
	MsidPtr                    *int                         `xml:"msid"`
	NamePtr                    *VolumeNameType              `xml:"name"`
	NameOrdinalPtr             *string                      `xml:"name-ordinal"`
	NodePtr                    *NodeNameType                `xml:"node"`
	NodesPtr                   *VolumeIdAttributesTypeNodes `xml:"nodes"`
	// work in progress
	OwningVserverNamePtr *string       `xml:"owning-vserver-name"`
	OwningVserverUuidPtr *UuidType     `xml:"owning-vserver-uuid"`
	ProvenanceUuidPtr    *UuidType     `xml:"provenance-uuid"`
	StylePtr             *VolstyleType `xml:"style"`
	StyleExtendedPtr     *string       `xml:"style-extended"`
	TypePtr              *string       `xml:"type"`
	UuidPtr              *UuidType     `xml:"uuid"`
}

// NewVolumeIdAttributesType is a factory method for creating new instances of VolumeIdAttributesType objects
func NewVolumeIdAttributesType() *VolumeIdAttributesType {
	return &VolumeIdAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeIdAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeIdAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// VolumeIdAttributesTypeAggrList is a wrapper
type VolumeIdAttributesTypeAggrList struct {
	XMLName     xml.Name       `xml:"aggr-list"`
	AggrNamePtr []AggrNameType `xml:"aggr-name"`
}

// AggrName is a 'getter' method
func (o *VolumeIdAttributesTypeAggrList) AggrName() []AggrNameType {
	r := o.AggrNamePtr
	return r
}

// SetAggrName is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesTypeAggrList) SetAggrName(newValue []AggrNameType) *VolumeIdAttributesTypeAggrList {
	newSlice := make([]AggrNameType, len(newValue))
	copy(newSlice, newValue)
	o.AggrNamePtr = newSlice
	return o
}

// AggrList is a 'getter' method
func (o *VolumeIdAttributesType) AggrList() VolumeIdAttributesTypeAggrList {
	var r VolumeIdAttributesTypeAggrList
	if o.AggrListPtr == nil {
		return r
	}
	r = *o.AggrListPtr
	return r
}

// SetAggrList is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetAggrList(newValue VolumeIdAttributesTypeAggrList) *VolumeIdAttributesType {
	o.AggrListPtr = &newValue
	return o
}

// Application is a 'getter' method
func (o *VolumeIdAttributesType) Application() string {
	var r string
	if o.ApplicationPtr == nil {
		return r
	}
	r = *o.ApplicationPtr
	return r
}

// SetApplication is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetApplication(newValue string) *VolumeIdAttributesType {
	o.ApplicationPtr = &newValue
	return o
}

// ApplicationUuid is a 'getter' method
func (o *VolumeIdAttributesType) ApplicationUuid() UuidType {
	var r UuidType
	if o.ApplicationUuidPtr == nil {
		return r
	}
	r = *o.ApplicationUuidPtr
	return r
}

// SetApplicationUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetApplicationUuid(newValue UuidType) *VolumeIdAttributesType {
	o.ApplicationUuidPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *VolumeIdAttributesType) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetComment(newValue string) *VolumeIdAttributesType {
	o.CommentPtr = &newValue
	return o
}

// ContainingAggregateName is a 'getter' method
func (o *VolumeIdAttributesType) ContainingAggregateName() string {
	var r string
	if o.ContainingAggregateNamePtr == nil {
		return r
	}
	r = *o.ContainingAggregateNamePtr
	return r
}

// SetContainingAggregateName is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetContainingAggregateName(newValue string) *VolumeIdAttributesType {
	o.ContainingAggregateNamePtr = &newValue
	return o
}

// ContainingAggregateUuid is a 'getter' method
func (o *VolumeIdAttributesType) ContainingAggregateUuid() UuidType {
	var r UuidType
	if o.ContainingAggregateUuidPtr == nil {
		return r
	}
	r = *o.ContainingAggregateUuidPtr
	return r
}

// SetContainingAggregateUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetContainingAggregateUuid(newValue UuidType) *VolumeIdAttributesType {
	o.ContainingAggregateUuidPtr = &newValue
	return o
}

// CreationTime is a 'getter' method
func (o *VolumeIdAttributesType) CreationTime() int {
	var r int
	if o.CreationTimePtr == nil {
		return r
	}
	r = *o.CreationTimePtr
	return r
}

// SetCreationTime is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetCreationTime(newValue int) *VolumeIdAttributesType {
	o.CreationTimePtr = &newValue
	return o
}

// Dsid is a 'getter' method
func (o *VolumeIdAttributesType) Dsid() int {
	var r int
	if o.DsidPtr == nil {
		return r
	}
	r = *o.DsidPtr
	return r
}

// SetDsid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetDsid(newValue int) *VolumeIdAttributesType {
	o.DsidPtr = &newValue
	return o
}

// ExtentSize is a 'getter' method
func (o *VolumeIdAttributesType) ExtentSize() string {
	var r string
	if o.ExtentSizePtr == nil {
		return r
	}
	r = *o.ExtentSizePtr
	return r
}

// SetExtentSize is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetExtentSize(newValue string) *VolumeIdAttributesType {
	o.ExtentSizePtr = &newValue
	return o
}

// FlexcacheEndpointType is a 'getter' method
func (o *VolumeIdAttributesType) FlexcacheEndpointType() string {
	var r string
	if o.FlexcacheEndpointTypePtr == nil {
		return r
	}
	r = *o.FlexcacheEndpointTypePtr
	return r
}

// SetFlexcacheEndpointType is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetFlexcacheEndpointType(newValue string) *VolumeIdAttributesType {
	o.FlexcacheEndpointTypePtr = &newValue
	return o
}

// FlexgroupIndex is a 'getter' method
func (o *VolumeIdAttributesType) FlexgroupIndex() int {
	var r int
	if o.FlexgroupIndexPtr == nil {
		return r
	}
	r = *o.FlexgroupIndexPtr
	return r
}

// SetFlexgroupIndex is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetFlexgroupIndex(newValue int) *VolumeIdAttributesType {
	o.FlexgroupIndexPtr = &newValue
	return o
}

// FlexgroupMsid is a 'getter' method
func (o *VolumeIdAttributesType) FlexgroupMsid() int {
	var r int
	if o.FlexgroupMsidPtr == nil {
		return r
	}
	r = *o.FlexgroupMsidPtr
	return r
}

// SetFlexgroupMsid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetFlexgroupMsid(newValue int) *VolumeIdAttributesType {
	o.FlexgroupMsidPtr = &newValue
	return o
}

// FlexgroupUuid is a 'getter' method
func (o *VolumeIdAttributesType) FlexgroupUuid() UuidType {
	var r UuidType
	if o.FlexgroupUuidPtr == nil {
		return r
	}
	r = *o.FlexgroupUuidPtr
	return r
}

// SetFlexgroupUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetFlexgroupUuid(newValue UuidType) *VolumeIdAttributesType {
	o.FlexgroupUuidPtr = &newValue
	return o
}

// Fsid is a 'getter' method
func (o *VolumeIdAttributesType) Fsid() string {
	var r string
	if o.FsidPtr == nil {
		return r
	}
	r = *o.FsidPtr
	return r
}

// SetFsid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetFsid(newValue string) *VolumeIdAttributesType {
	o.FsidPtr = &newValue
	return o
}

// InstanceUuid is a 'getter' method
func (o *VolumeIdAttributesType) InstanceUuid() UuidType {
	var r UuidType
	if o.InstanceUuidPtr == nil {
		return r
	}
	r = *o.InstanceUuidPtr
	return r
}

// SetInstanceUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetInstanceUuid(newValue UuidType) *VolumeIdAttributesType {
	o.InstanceUuidPtr = &newValue
	return o
}

// JunctionParentName is a 'getter' method
func (o *VolumeIdAttributesType) JunctionParentName() VolumeNameType {
	var r VolumeNameType
	if o.JunctionParentNamePtr == nil {
		return r
	}
	r = *o.JunctionParentNamePtr
	return r
}

// SetJunctionParentName is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetJunctionParentName(newValue VolumeNameType) *VolumeIdAttributesType {
	o.JunctionParentNamePtr = &newValue
	return o
}

// JunctionPath is a 'getter' method
func (o *VolumeIdAttributesType) JunctionPath() JunctionPathType {
	var r JunctionPathType
	if o.JunctionPathPtr == nil {
		return r
	}
	r = *o.JunctionPathPtr
	return r
}

// SetJunctionPath is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetJunctionPath(newValue JunctionPathType) *VolumeIdAttributesType {
	o.JunctionPathPtr = &newValue
	return o
}

// Msid is a 'getter' method
func (o *VolumeIdAttributesType) Msid() int {
	var r int
	if o.MsidPtr == nil {
		return r
	}
	r = *o.MsidPtr
	return r
}

// SetMsid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetMsid(newValue int) *VolumeIdAttributesType {
	o.MsidPtr = &newValue
	return o
}

// Name is a 'getter' method
func (o *VolumeIdAttributesType) Name() VolumeNameType {
	var r VolumeNameType
	if o.NamePtr == nil {
		return r
	}
	r = *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetName(newValue VolumeNameType) *VolumeIdAttributesType {
	o.NamePtr = &newValue
	return o
}

// NameOrdinal is a 'getter' method
func (o *VolumeIdAttributesType) NameOrdinal() string {
	var r string
	if o.NameOrdinalPtr == nil {
		return r
	}
	r = *o.NameOrdinalPtr
	return r
}

// SetNameOrdinal is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetNameOrdinal(newValue string) *VolumeIdAttributesType {
	o.NameOrdinalPtr = &newValue
	return o
}

// Node is a 'getter' method
func (o *VolumeIdAttributesType) Node() NodeNameType {
	var r NodeNameType
	if o.NodePtr == nil {
		return r
	}
	r = *o.NodePtr
	return r
}

// SetNode is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetNode(newValue NodeNameType) *VolumeIdAttributesType {
	o.NodePtr = &newValue
	return o
}

// VolumeIdAttributesTypeNodes is a wrapper
type VolumeIdAttributesTypeNodes struct {
	XMLName     xml.Name       `xml:"nodes"`
	NodeNamePtr []NodeNameType `xml:"node-name"`
}

// NodeName is a 'getter' method
func (o *VolumeIdAttributesTypeNodes) NodeName() []NodeNameType {
	r := o.NodeNamePtr
	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesTypeNodes) SetNodeName(newValue []NodeNameType) *VolumeIdAttributesTypeNodes {
	newSlice := make([]NodeNameType, len(newValue))
	copy(newSlice, newValue)
	o.NodeNamePtr = newSlice
	return o
}

// Nodes is a 'getter' method
func (o *VolumeIdAttributesType) Nodes() VolumeIdAttributesTypeNodes {
	var r VolumeIdAttributesTypeNodes
	if o.NodesPtr == nil {
		return r
	}
	r = *o.NodesPtr
	return r
}

// SetNodes is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetNodes(newValue VolumeIdAttributesTypeNodes) *VolumeIdAttributesType {
	o.NodesPtr = &newValue
	return o
}

// OwningVserverName is a 'getter' method
func (o *VolumeIdAttributesType) OwningVserverName() string {
	var r string
	if o.OwningVserverNamePtr == nil {
		return r
	}
	r = *o.OwningVserverNamePtr
	return r
}

// SetOwningVserverName is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetOwningVserverName(newValue string) *VolumeIdAttributesType {
	o.OwningVserverNamePtr = &newValue
	return o
}

// OwningVserverUuid is a 'getter' method
func (o *VolumeIdAttributesType) OwningVserverUuid() UuidType {
	var r UuidType
	if o.OwningVserverUuidPtr == nil {
		return r
	}
	r = *o.OwningVserverUuidPtr
	return r
}

// SetOwningVserverUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetOwningVserverUuid(newValue UuidType) *VolumeIdAttributesType {
	o.OwningVserverUuidPtr = &newValue
	return o
}

// ProvenanceUuid is a 'getter' method
func (o *VolumeIdAttributesType) ProvenanceUuid() UuidType {
	var r UuidType
	if o.ProvenanceUuidPtr == nil {
		return r
	}
	r = *o.ProvenanceUuidPtr
	return r
}

// SetProvenanceUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetProvenanceUuid(newValue UuidType) *VolumeIdAttributesType {
	o.ProvenanceUuidPtr = &newValue
	return o
}

// Style is a 'getter' method
func (o *VolumeIdAttributesType) Style() VolstyleType {
	var r VolstyleType
	if o.StylePtr == nil {
		return r
	}
	r = *o.StylePtr
	return r
}

// SetStyle is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetStyle(newValue VolstyleType) *VolumeIdAttributesType {
	o.StylePtr = &newValue
	return o
}

// StyleExtended is a 'getter' method
func (o *VolumeIdAttributesType) StyleExtended() string {
	var r string
	if o.StyleExtendedPtr == nil {
		return r
	}
	r = *o.StyleExtendedPtr
	return r
}

// SetStyleExtended is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetStyleExtended(newValue string) *VolumeIdAttributesType {
	o.StyleExtendedPtr = &newValue
	return o
}

// Type is a 'getter' method
func (o *VolumeIdAttributesType) Type() string {
	var r string
	if o.TypePtr == nil {
		return r
	}
	r = *o.TypePtr
	return r
}

// SetType is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetType(newValue string) *VolumeIdAttributesType {
	o.TypePtr = &newValue
	return o
}

// Uuid is a 'getter' method
func (o *VolumeIdAttributesType) Uuid() UuidType {
	var r UuidType
	if o.UuidPtr == nil {
		return r
	}
	r = *o.UuidPtr
	return r
}

// SetUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetUuid(newValue UuidType) *VolumeIdAttributesType {
	o.UuidPtr = &newValue
	return o
}
