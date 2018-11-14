package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
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
	r := *o.AggrListPtr
	return r
}

// SetAggrList is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetAggrList(newValue VolumeIdAttributesTypeAggrList) *VolumeIdAttributesType {
	o.AggrListPtr = &newValue
	return o
}

// Application is a 'getter' method
func (o *VolumeIdAttributesType) Application() string {
	r := *o.ApplicationPtr
	return r
}

// SetApplication is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetApplication(newValue string) *VolumeIdAttributesType {
	o.ApplicationPtr = &newValue
	return o
}

// ApplicationUuid is a 'getter' method
func (o *VolumeIdAttributesType) ApplicationUuid() UuidType {
	r := *o.ApplicationUuidPtr
	return r
}

// SetApplicationUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetApplicationUuid(newValue UuidType) *VolumeIdAttributesType {
	o.ApplicationUuidPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *VolumeIdAttributesType) Comment() string {
	r := *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetComment(newValue string) *VolumeIdAttributesType {
	o.CommentPtr = &newValue
	return o
}

// ContainingAggregateName is a 'getter' method
func (o *VolumeIdAttributesType) ContainingAggregateName() string {
	r := *o.ContainingAggregateNamePtr
	return r
}

// SetContainingAggregateName is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetContainingAggregateName(newValue string) *VolumeIdAttributesType {
	o.ContainingAggregateNamePtr = &newValue
	return o
}

// ContainingAggregateUuid is a 'getter' method
func (o *VolumeIdAttributesType) ContainingAggregateUuid() UuidType {
	r := *o.ContainingAggregateUuidPtr
	return r
}

// SetContainingAggregateUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetContainingAggregateUuid(newValue UuidType) *VolumeIdAttributesType {
	o.ContainingAggregateUuidPtr = &newValue
	return o
}

// CreationTime is a 'getter' method
func (o *VolumeIdAttributesType) CreationTime() int {
	r := *o.CreationTimePtr
	return r
}

// SetCreationTime is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetCreationTime(newValue int) *VolumeIdAttributesType {
	o.CreationTimePtr = &newValue
	return o
}

// Dsid is a 'getter' method
func (o *VolumeIdAttributesType) Dsid() int {
	r := *o.DsidPtr
	return r
}

// SetDsid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetDsid(newValue int) *VolumeIdAttributesType {
	o.DsidPtr = &newValue
	return o
}

// ExtentSize is a 'getter' method
func (o *VolumeIdAttributesType) ExtentSize() string {
	r := *o.ExtentSizePtr
	return r
}

// SetExtentSize is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetExtentSize(newValue string) *VolumeIdAttributesType {
	o.ExtentSizePtr = &newValue
	return o
}

// FlexcacheEndpointType is a 'getter' method
func (o *VolumeIdAttributesType) FlexcacheEndpointType() string {
	r := *o.FlexcacheEndpointTypePtr
	return r
}

// SetFlexcacheEndpointType is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetFlexcacheEndpointType(newValue string) *VolumeIdAttributesType {
	o.FlexcacheEndpointTypePtr = &newValue
	return o
}

// FlexgroupIndex is a 'getter' method
func (o *VolumeIdAttributesType) FlexgroupIndex() int {
	r := *o.FlexgroupIndexPtr
	return r
}

// SetFlexgroupIndex is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetFlexgroupIndex(newValue int) *VolumeIdAttributesType {
	o.FlexgroupIndexPtr = &newValue
	return o
}

// FlexgroupMsid is a 'getter' method
func (o *VolumeIdAttributesType) FlexgroupMsid() int {
	r := *o.FlexgroupMsidPtr
	return r
}

// SetFlexgroupMsid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetFlexgroupMsid(newValue int) *VolumeIdAttributesType {
	o.FlexgroupMsidPtr = &newValue
	return o
}

// FlexgroupUuid is a 'getter' method
func (o *VolumeIdAttributesType) FlexgroupUuid() UuidType {
	r := *o.FlexgroupUuidPtr
	return r
}

// SetFlexgroupUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetFlexgroupUuid(newValue UuidType) *VolumeIdAttributesType {
	o.FlexgroupUuidPtr = &newValue
	return o
}

// Fsid is a 'getter' method
func (o *VolumeIdAttributesType) Fsid() string {
	r := *o.FsidPtr
	return r
}

// SetFsid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetFsid(newValue string) *VolumeIdAttributesType {
	o.FsidPtr = &newValue
	return o
}

// InstanceUuid is a 'getter' method
func (o *VolumeIdAttributesType) InstanceUuid() UuidType {
	r := *o.InstanceUuidPtr
	return r
}

// SetInstanceUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetInstanceUuid(newValue UuidType) *VolumeIdAttributesType {
	o.InstanceUuidPtr = &newValue
	return o
}

// JunctionParentName is a 'getter' method
func (o *VolumeIdAttributesType) JunctionParentName() VolumeNameType {
	r := *o.JunctionParentNamePtr
	return r
}

// SetJunctionParentName is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetJunctionParentName(newValue VolumeNameType) *VolumeIdAttributesType {
	o.JunctionParentNamePtr = &newValue
	return o
}

// JunctionPath is a 'getter' method
func (o *VolumeIdAttributesType) JunctionPath() JunctionPathType {
	r := *o.JunctionPathPtr
	return r
}

// SetJunctionPath is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetJunctionPath(newValue JunctionPathType) *VolumeIdAttributesType {
	o.JunctionPathPtr = &newValue
	return o
}

// Msid is a 'getter' method
func (o *VolumeIdAttributesType) Msid() int {
	r := *o.MsidPtr
	return r
}

// SetMsid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetMsid(newValue int) *VolumeIdAttributesType {
	o.MsidPtr = &newValue
	return o
}

// Name is a 'getter' method
func (o *VolumeIdAttributesType) Name() VolumeNameType {
	r := *o.NamePtr
	return r
}

// SetName is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetName(newValue VolumeNameType) *VolumeIdAttributesType {
	o.NamePtr = &newValue
	return o
}

// NameOrdinal is a 'getter' method
func (o *VolumeIdAttributesType) NameOrdinal() string {
	r := *o.NameOrdinalPtr
	return r
}

// SetNameOrdinal is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetNameOrdinal(newValue string) *VolumeIdAttributesType {
	o.NameOrdinalPtr = &newValue
	return o
}

// Node is a 'getter' method
func (o *VolumeIdAttributesType) Node() NodeNameType {
	r := *o.NodePtr
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
	r := *o.NodesPtr
	return r
}

// SetNodes is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetNodes(newValue VolumeIdAttributesTypeNodes) *VolumeIdAttributesType {
	o.NodesPtr = &newValue
	return o
}

// OwningVserverName is a 'getter' method
func (o *VolumeIdAttributesType) OwningVserverName() string {
	r := *o.OwningVserverNamePtr
	return r
}

// SetOwningVserverName is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetOwningVserverName(newValue string) *VolumeIdAttributesType {
	o.OwningVserverNamePtr = &newValue
	return o
}

// OwningVserverUuid is a 'getter' method
func (o *VolumeIdAttributesType) OwningVserverUuid() UuidType {
	r := *o.OwningVserverUuidPtr
	return r
}

// SetOwningVserverUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetOwningVserverUuid(newValue UuidType) *VolumeIdAttributesType {
	o.OwningVserverUuidPtr = &newValue
	return o
}

// ProvenanceUuid is a 'getter' method
func (o *VolumeIdAttributesType) ProvenanceUuid() UuidType {
	r := *o.ProvenanceUuidPtr
	return r
}

// SetProvenanceUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetProvenanceUuid(newValue UuidType) *VolumeIdAttributesType {
	o.ProvenanceUuidPtr = &newValue
	return o
}

// Style is a 'getter' method
func (o *VolumeIdAttributesType) Style() VolstyleType {
	r := *o.StylePtr
	return r
}

// SetStyle is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetStyle(newValue VolstyleType) *VolumeIdAttributesType {
	o.StylePtr = &newValue
	return o
}

// StyleExtended is a 'getter' method
func (o *VolumeIdAttributesType) StyleExtended() string {
	r := *o.StyleExtendedPtr
	return r
}

// SetStyleExtended is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetStyleExtended(newValue string) *VolumeIdAttributesType {
	o.StyleExtendedPtr = &newValue
	return o
}

// Type is a 'getter' method
func (o *VolumeIdAttributesType) Type() string {
	r := *o.TypePtr
	return r
}

// SetType is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetType(newValue string) *VolumeIdAttributesType {
	o.TypePtr = &newValue
	return o
}

// Uuid is a 'getter' method
func (o *VolumeIdAttributesType) Uuid() UuidType {
	r := *o.UuidPtr
	return r
}

// SetUuid is a fluent style 'setter' method that can be chained
func (o *VolumeIdAttributesType) SetUuid(newValue UuidType) *VolumeIdAttributesType {
	o.UuidPtr = &newValue
	return o
}
