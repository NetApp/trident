package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// AggrAttributesType is a structure to represent a aggr-attributes ZAPI object
type AggrAttributesType struct {
	XMLName                                  xml.Name                        `xml:"aggr-attributes"`
	Aggr64bitUpgradeAttributesPtr            *Aggr64bitUpgradeAttributesType `xml:"aggr-64bit-upgrade-attributes"`
	AggrFsAttributesPtr                      *AggrFsAttributesType           `xml:"aggr-fs-attributes"`
	AggrInodeAttributesPtr                   *AggrInodeAttributesType        `xml:"aggr-inode-attributes"`
	AggrOwnershipAttributesPtr               *AggrOwnershipAttributesType    `xml:"aggr-ownership-attributes"`
	AggrPerformanceAttributesPtr             *AggrPerformanceAttributesType  `xml:"aggr-performance-attributes"`
	AggrRaidAttributesPtr                    *AggrRaidAttributesType         `xml:"aggr-raid-attributes"`
	AggrSnaplockAttributesPtr                *AggrSnaplockAttributesType     `xml:"aggr-snaplock-attributes"`
	AggrSnapmirrorAttributesPtr              *AggrSnapmirrorAttributesType   `xml:"aggr-snapmirror-attributes"`
	AggrSnapshotAttributesPtr                *AggrSnapshotAttributesType     `xml:"aggr-snapshot-attributes"`
	AggrSpaceAttributesPtr                   *AggrSpaceAttributesType        `xml:"aggr-space-attributes"`
	AggrStripingAttributesPtr                *AggrStripingAttributesType     `xml:"aggr-striping-attributes"`
	AggrVolumeCountAttributesPtr             *AggrVolumeCountAttributesType  `xml:"aggr-volume-count-attributes"`
	AggrWaflironAttributesPtr                *AggrWaflironAttributesType     `xml:"aggr-wafliron-attributes"`
	AggregateNamePtr                         *string                         `xml:"aggregate-name"`
	AggregateUuidPtr                         *string                         `xml:"aggregate-uuid"`
	AutobalanceAvailableThresholdPercentPtr  *int                            `xml:"autobalance-available-threshold-percent"`
	AutobalanceStatePtr                      *AutobalanceAggregateStateType  `xml:"autobalance-state"`
	AutobalanceStateChangeCounterPtr         *int                            `xml:"autobalance-state-change-counter"`
	AutobalanceUnbalancedThresholdPercentPtr *int                            `xml:"autobalance-unbalanced-threshold-percent"`
	IsAutobalanceEligiblePtr                 *bool                           `xml:"is-autobalance-eligible"`
	IsCftPrecommitPtr                        *bool                           `xml:"is-cft-precommit"`
	IsObjectStoreAttachEligiblePtr           *bool                           `xml:"is-object-store-attach-eligible"`
	IsTransitionOutOfSpacePtr                *bool                           `xml:"is-transition-out-of-space"`
	NodesPtr                                 *AggrAttributesTypeNodes        `xml:"nodes"`
	// work in progress
	StripingTypePtr *string `xml:"striping-type"`
}

// NewAggrAttributesType is a factory method for creating new instances of AggrAttributesType objects
func NewAggrAttributesType() *AggrAttributesType {
	return &AggrAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *AggrAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o AggrAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Aggr64bitUpgradeAttributes is a 'getter' method
func (o *AggrAttributesType) Aggr64bitUpgradeAttributes() Aggr64bitUpgradeAttributesType {
	r := *o.Aggr64bitUpgradeAttributesPtr
	return r
}

// SetAggr64bitUpgradeAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggr64bitUpgradeAttributes(newValue Aggr64bitUpgradeAttributesType) *AggrAttributesType {
	o.Aggr64bitUpgradeAttributesPtr = &newValue
	return o
}

// AggrFsAttributes is a 'getter' method
func (o *AggrAttributesType) AggrFsAttributes() AggrFsAttributesType {
	r := *o.AggrFsAttributesPtr
	return r
}

// SetAggrFsAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrFsAttributes(newValue AggrFsAttributesType) *AggrAttributesType {
	o.AggrFsAttributesPtr = &newValue
	return o
}

// AggrInodeAttributes is a 'getter' method
func (o *AggrAttributesType) AggrInodeAttributes() AggrInodeAttributesType {
	r := *o.AggrInodeAttributesPtr
	return r
}

// SetAggrInodeAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrInodeAttributes(newValue AggrInodeAttributesType) *AggrAttributesType {
	o.AggrInodeAttributesPtr = &newValue
	return o
}

// AggrOwnershipAttributes is a 'getter' method
func (o *AggrAttributesType) AggrOwnershipAttributes() AggrOwnershipAttributesType {
	r := *o.AggrOwnershipAttributesPtr
	return r
}

// SetAggrOwnershipAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrOwnershipAttributes(newValue AggrOwnershipAttributesType) *AggrAttributesType {
	o.AggrOwnershipAttributesPtr = &newValue
	return o
}

// AggrPerformanceAttributes is a 'getter' method
func (o *AggrAttributesType) AggrPerformanceAttributes() AggrPerformanceAttributesType {
	r := *o.AggrPerformanceAttributesPtr
	return r
}

// SetAggrPerformanceAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrPerformanceAttributes(newValue AggrPerformanceAttributesType) *AggrAttributesType {
	o.AggrPerformanceAttributesPtr = &newValue
	return o
}

// AggrRaidAttributes is a 'getter' method
func (o *AggrAttributesType) AggrRaidAttributes() AggrRaidAttributesType {
	r := *o.AggrRaidAttributesPtr
	return r
}

// SetAggrRaidAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrRaidAttributes(newValue AggrRaidAttributesType) *AggrAttributesType {
	o.AggrRaidAttributesPtr = &newValue
	return o
}

// AggrSnaplockAttributes is a 'getter' method
func (o *AggrAttributesType) AggrSnaplockAttributes() AggrSnaplockAttributesType {
	r := *o.AggrSnaplockAttributesPtr
	return r
}

// SetAggrSnaplockAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrSnaplockAttributes(newValue AggrSnaplockAttributesType) *AggrAttributesType {
	o.AggrSnaplockAttributesPtr = &newValue
	return o
}

// AggrSnapmirrorAttributes is a 'getter' method
func (o *AggrAttributesType) AggrSnapmirrorAttributes() AggrSnapmirrorAttributesType {
	r := *o.AggrSnapmirrorAttributesPtr
	return r
}

// SetAggrSnapmirrorAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrSnapmirrorAttributes(newValue AggrSnapmirrorAttributesType) *AggrAttributesType {
	o.AggrSnapmirrorAttributesPtr = &newValue
	return o
}

// AggrSnapshotAttributes is a 'getter' method
func (o *AggrAttributesType) AggrSnapshotAttributes() AggrSnapshotAttributesType {
	r := *o.AggrSnapshotAttributesPtr
	return r
}

// SetAggrSnapshotAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrSnapshotAttributes(newValue AggrSnapshotAttributesType) *AggrAttributesType {
	o.AggrSnapshotAttributesPtr = &newValue
	return o
}

// AggrSpaceAttributes is a 'getter' method
func (o *AggrAttributesType) AggrSpaceAttributes() AggrSpaceAttributesType {
	r := *o.AggrSpaceAttributesPtr
	return r
}

// SetAggrSpaceAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrSpaceAttributes(newValue AggrSpaceAttributesType) *AggrAttributesType {
	o.AggrSpaceAttributesPtr = &newValue
	return o
}

// AggrStripingAttributes is a 'getter' method
func (o *AggrAttributesType) AggrStripingAttributes() AggrStripingAttributesType {
	r := *o.AggrStripingAttributesPtr
	return r
}

// SetAggrStripingAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrStripingAttributes(newValue AggrStripingAttributesType) *AggrAttributesType {
	o.AggrStripingAttributesPtr = &newValue
	return o
}

// AggrVolumeCountAttributes is a 'getter' method
func (o *AggrAttributesType) AggrVolumeCountAttributes() AggrVolumeCountAttributesType {
	r := *o.AggrVolumeCountAttributesPtr
	return r
}

// SetAggrVolumeCountAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrVolumeCountAttributes(newValue AggrVolumeCountAttributesType) *AggrAttributesType {
	o.AggrVolumeCountAttributesPtr = &newValue
	return o
}

// AggrWaflironAttributes is a 'getter' method
func (o *AggrAttributesType) AggrWaflironAttributes() AggrWaflironAttributesType {
	r := *o.AggrWaflironAttributesPtr
	return r
}

// SetAggrWaflironAttributes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggrWaflironAttributes(newValue AggrWaflironAttributesType) *AggrAttributesType {
	o.AggrWaflironAttributesPtr = &newValue
	return o
}

// AggregateName is a 'getter' method
func (o *AggrAttributesType) AggregateName() string {
	r := *o.AggregateNamePtr
	return r
}

// SetAggregateName is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggregateName(newValue string) *AggrAttributesType {
	o.AggregateNamePtr = &newValue
	return o
}

// AggregateUuid is a 'getter' method
func (o *AggrAttributesType) AggregateUuid() string {
	r := *o.AggregateUuidPtr
	return r
}

// SetAggregateUuid is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAggregateUuid(newValue string) *AggrAttributesType {
	o.AggregateUuidPtr = &newValue
	return o
}

// AutobalanceAvailableThresholdPercent is a 'getter' method
func (o *AggrAttributesType) AutobalanceAvailableThresholdPercent() int {
	r := *o.AutobalanceAvailableThresholdPercentPtr
	return r
}

// SetAutobalanceAvailableThresholdPercent is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAutobalanceAvailableThresholdPercent(newValue int) *AggrAttributesType {
	o.AutobalanceAvailableThresholdPercentPtr = &newValue
	return o
}

// AutobalanceState is a 'getter' method
func (o *AggrAttributesType) AutobalanceState() AutobalanceAggregateStateType {
	r := *o.AutobalanceStatePtr
	return r
}

// SetAutobalanceState is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAutobalanceState(newValue AutobalanceAggregateStateType) *AggrAttributesType {
	o.AutobalanceStatePtr = &newValue
	return o
}

// AutobalanceStateChangeCounter is a 'getter' method
func (o *AggrAttributesType) AutobalanceStateChangeCounter() int {
	r := *o.AutobalanceStateChangeCounterPtr
	return r
}

// SetAutobalanceStateChangeCounter is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAutobalanceStateChangeCounter(newValue int) *AggrAttributesType {
	o.AutobalanceStateChangeCounterPtr = &newValue
	return o
}

// AutobalanceUnbalancedThresholdPercent is a 'getter' method
func (o *AggrAttributesType) AutobalanceUnbalancedThresholdPercent() int {
	r := *o.AutobalanceUnbalancedThresholdPercentPtr
	return r
}

// SetAutobalanceUnbalancedThresholdPercent is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetAutobalanceUnbalancedThresholdPercent(newValue int) *AggrAttributesType {
	o.AutobalanceUnbalancedThresholdPercentPtr = &newValue
	return o
}

// IsAutobalanceEligible is a 'getter' method
func (o *AggrAttributesType) IsAutobalanceEligible() bool {
	r := *o.IsAutobalanceEligiblePtr
	return r
}

// SetIsAutobalanceEligible is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetIsAutobalanceEligible(newValue bool) *AggrAttributesType {
	o.IsAutobalanceEligiblePtr = &newValue
	return o
}

// IsCftPrecommit is a 'getter' method
func (o *AggrAttributesType) IsCftPrecommit() bool {
	r := *o.IsCftPrecommitPtr
	return r
}

// SetIsCftPrecommit is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetIsCftPrecommit(newValue bool) *AggrAttributesType {
	o.IsCftPrecommitPtr = &newValue
	return o
}

// IsObjectStoreAttachEligible is a 'getter' method
func (o *AggrAttributesType) IsObjectStoreAttachEligible() bool {
	r := *o.IsObjectStoreAttachEligiblePtr
	return r
}

// SetIsObjectStoreAttachEligible is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetIsObjectStoreAttachEligible(newValue bool) *AggrAttributesType {
	o.IsObjectStoreAttachEligiblePtr = &newValue
	return o
}

// IsTransitionOutOfSpace is a 'getter' method
func (o *AggrAttributesType) IsTransitionOutOfSpace() bool {
	r := *o.IsTransitionOutOfSpacePtr
	return r
}

// SetIsTransitionOutOfSpace is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetIsTransitionOutOfSpace(newValue bool) *AggrAttributesType {
	o.IsTransitionOutOfSpacePtr = &newValue
	return o
}

// AggrAttributesTypeNodes is a wrapper
type AggrAttributesTypeNodes struct {
	XMLName     xml.Name       `xml:"nodes"`
	NodeNamePtr []NodeNameType `xml:"node-name"`
}

// NodeName is a 'getter' method
func (o *AggrAttributesTypeNodes) NodeName() []NodeNameType {
	r := o.NodeNamePtr
	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *AggrAttributesTypeNodes) SetNodeName(newValue []NodeNameType) *AggrAttributesTypeNodes {
	newSlice := make([]NodeNameType, len(newValue))
	copy(newSlice, newValue)
	o.NodeNamePtr = newSlice
	return o
}

// Nodes is a 'getter' method
func (o *AggrAttributesType) Nodes() AggrAttributesTypeNodes {
	r := *o.NodesPtr
	return r
}

// SetNodes is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetNodes(newValue AggrAttributesTypeNodes) *AggrAttributesType {
	o.NodesPtr = &newValue
	return o
}

// StripingType is a 'getter' method
func (o *AggrAttributesType) StripingType() string {
	r := *o.StripingTypePtr
	return r
}

// SetStripingType is a fluent style 'setter' method that can be chained
func (o *AggrAttributesType) SetStripingType(newValue string) *AggrAttributesType {
	o.StripingTypePtr = &newValue
	return o
}
