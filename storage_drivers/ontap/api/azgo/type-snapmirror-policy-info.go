// Code generated automatically. DO NOT EDIT.
package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// SnapmirrorPolicyInfoType is a structure to represent a snapmirror-policy-info ZAPI object
type SnapmirrorPolicyInfoType struct {
	XMLName                        xml.Name                                `xml:"snapmirror-policy-info"`
	ArchiveAfterDaysPtr            *int                                    `xml:"archive-after-days"`
	AreDataOpsSequentiallySplitPtr *bool                                   `xml:"are-data-ops-sequentially-split"`
	CommentPtr                     *string                                 `xml:"comment"`
	CommonSnapshotSchedulePtr      *string                                 `xml:"common-snapshot-schedule"`
	CreateSnapshotPtr              *bool                                   `xml:"create-snapshot"`
	DiscardConfigsPtr              *SnapmirrorPolicyInfoTypeDiscardConfigs `xml:"discard-configs"`
	// work in progress
	IgnoreAtimePtr                 *bool                                          `xml:"ignore-atime"`
	IsNetworkCompressionEnabledPtr *bool                                          `xml:"is-network-compression-enabled"`
	OwnerPtr                       *PolicyOwnerType                               `xml:"owner"`
	PolicyNamePtr                  *SmPolicyType                                  `xml:"policy-name"`
	RestartPtr                     *SmRestartEnumType                             `xml:"restart"`
	SnapmirrorPolicyRulesPtr       *SnapmirrorPolicyInfoTypeSnapmirrorPolicyRules `xml:"snapmirror-policy-rules"`
	// work in progress
	TotalKeepPtr              *int                        `xml:"total-keep"`
	TotalRulesPtr             *int                        `xml:"total-rules"`
	TransferPriorityPtr       *SmTransferPriorityEnumType `xml:"transfer-priority"`
	TriesPtr                  *string                     `xml:"tries"`
	TypePtr                   *SmPolicyTypeEnumType       `xml:"type"`
	VserverNamePtr            *VserverNameType            `xml:"vserver-name"`
	WindowSizeForTdpMirrorPtr *SizeType                   `xml:"window-size-for-tdp-mirror"`
}

// NewSnapmirrorPolicyInfoType is a factory method for creating new instances of SnapmirrorPolicyInfoType objects
func NewSnapmirrorPolicyInfoType() *SnapmirrorPolicyInfoType {
	return &SnapmirrorPolicyInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorPolicyInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorPolicyInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// ArchiveAfterDays is a 'getter' method
func (o *SnapmirrorPolicyInfoType) ArchiveAfterDays() int {

	var r int
	if o.ArchiveAfterDaysPtr == nil {
		return r
	}
	r = *o.ArchiveAfterDaysPtr

	return r
}

// SetArchiveAfterDays is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetArchiveAfterDays(newValue int) *SnapmirrorPolicyInfoType {
	o.ArchiveAfterDaysPtr = &newValue
	return o
}

// AreDataOpsSequentiallySplit is a 'getter' method
func (o *SnapmirrorPolicyInfoType) AreDataOpsSequentiallySplit() bool {

	var r bool
	if o.AreDataOpsSequentiallySplitPtr == nil {
		return r
	}
	r = *o.AreDataOpsSequentiallySplitPtr

	return r
}

// SetAreDataOpsSequentiallySplit is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetAreDataOpsSequentiallySplit(newValue bool) *SnapmirrorPolicyInfoType {
	o.AreDataOpsSequentiallySplitPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *SnapmirrorPolicyInfoType) Comment() string {

	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr

	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetComment(newValue string) *SnapmirrorPolicyInfoType {
	o.CommentPtr = &newValue
	return o
}

// CommonSnapshotSchedule is a 'getter' method
func (o *SnapmirrorPolicyInfoType) CommonSnapshotSchedule() string {

	var r string
	if o.CommonSnapshotSchedulePtr == nil {
		return r
	}
	r = *o.CommonSnapshotSchedulePtr

	return r
}

// SetCommonSnapshotSchedule is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetCommonSnapshotSchedule(newValue string) *SnapmirrorPolicyInfoType {
	o.CommonSnapshotSchedulePtr = &newValue
	return o
}

// CreateSnapshot is a 'getter' method
func (o *SnapmirrorPolicyInfoType) CreateSnapshot() bool {

	var r bool
	if o.CreateSnapshotPtr == nil {
		return r
	}
	r = *o.CreateSnapshotPtr

	return r
}

// SetCreateSnapshot is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetCreateSnapshot(newValue bool) *SnapmirrorPolicyInfoType {
	o.CreateSnapshotPtr = &newValue
	return o
}

// SnapmirrorPolicyInfoTypeDiscardConfigs is a wrapper
type SnapmirrorPolicyInfoTypeDiscardConfigs struct {
	XMLName           xml.Name             `xml:"discard-configs"`
	SvmdrConfigObjPtr []SvmdrConfigObjType `xml:"svmdr-config-obj"`
}

// SvmdrConfigObj is a 'getter' method
func (o *SnapmirrorPolicyInfoTypeDiscardConfigs) SvmdrConfigObj() []SvmdrConfigObjType {
	r := o.SvmdrConfigObjPtr
	return r
}

// SetSvmdrConfigObj is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoTypeDiscardConfigs) SetSvmdrConfigObj(newValue []SvmdrConfigObjType) *SnapmirrorPolicyInfoTypeDiscardConfigs {
	newSlice := make([]SvmdrConfigObjType, len(newValue))
	copy(newSlice, newValue)
	o.SvmdrConfigObjPtr = newSlice
	return o
}

// DiscardConfigs is a 'getter' method
func (o *SnapmirrorPolicyInfoType) DiscardConfigs() SnapmirrorPolicyInfoTypeDiscardConfigs {

	var r SnapmirrorPolicyInfoTypeDiscardConfigs
	if o.DiscardConfigsPtr == nil {
		return r
	}
	r = *o.DiscardConfigsPtr

	return r
}

// SetDiscardConfigs is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetDiscardConfigs(newValue SnapmirrorPolicyInfoTypeDiscardConfigs) *SnapmirrorPolicyInfoType {
	o.DiscardConfigsPtr = &newValue
	return o
}

// IgnoreAtime is a 'getter' method
func (o *SnapmirrorPolicyInfoType) IgnoreAtime() bool {

	var r bool
	if o.IgnoreAtimePtr == nil {
		return r
	}
	r = *o.IgnoreAtimePtr

	return r
}

// SetIgnoreAtime is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetIgnoreAtime(newValue bool) *SnapmirrorPolicyInfoType {
	o.IgnoreAtimePtr = &newValue
	return o
}

// IsNetworkCompressionEnabled is a 'getter' method
func (o *SnapmirrorPolicyInfoType) IsNetworkCompressionEnabled() bool {

	var r bool
	if o.IsNetworkCompressionEnabledPtr == nil {
		return r
	}
	r = *o.IsNetworkCompressionEnabledPtr

	return r
}

// SetIsNetworkCompressionEnabled is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetIsNetworkCompressionEnabled(newValue bool) *SnapmirrorPolicyInfoType {
	o.IsNetworkCompressionEnabledPtr = &newValue
	return o
}

// Owner is a 'getter' method
func (o *SnapmirrorPolicyInfoType) Owner() PolicyOwnerType {

	var r PolicyOwnerType
	if o.OwnerPtr == nil {
		return r
	}
	r = *o.OwnerPtr

	return r
}

// SetOwner is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetOwner(newValue PolicyOwnerType) *SnapmirrorPolicyInfoType {
	o.OwnerPtr = &newValue
	return o
}

// PolicyName is a 'getter' method
func (o *SnapmirrorPolicyInfoType) PolicyName() SmPolicyType {

	var r SmPolicyType
	if o.PolicyNamePtr == nil {
		return r
	}
	r = *o.PolicyNamePtr

	return r
}

// SetPolicyName is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetPolicyName(newValue SmPolicyType) *SnapmirrorPolicyInfoType {
	o.PolicyNamePtr = &newValue
	return o
}

// Restart is a 'getter' method
func (o *SnapmirrorPolicyInfoType) Restart() SmRestartEnumType {

	var r SmRestartEnumType
	if o.RestartPtr == nil {
		return r
	}
	r = *o.RestartPtr

	return r
}

// SetRestart is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetRestart(newValue SmRestartEnumType) *SnapmirrorPolicyInfoType {
	o.RestartPtr = &newValue
	return o
}

// SnapmirrorPolicyInfoTypeSnapmirrorPolicyRules is a wrapper
type SnapmirrorPolicyInfoTypeSnapmirrorPolicyRules struct {
	XMLName                     xml.Name                       `xml:"snapmirror-policy-rules"`
	SnapmirrorPolicyRuleInfoPtr []SnapmirrorPolicyRuleInfoType `xml:"snapmirror-policy-rule-info"`
}

// SnapmirrorPolicyRuleInfo is a 'getter' method
func (o *SnapmirrorPolicyInfoTypeSnapmirrorPolicyRules) SnapmirrorPolicyRuleInfo() []SnapmirrorPolicyRuleInfoType {
	r := o.SnapmirrorPolicyRuleInfoPtr
	return r
}

// SetSnapmirrorPolicyRuleInfo is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoTypeSnapmirrorPolicyRules) SetSnapmirrorPolicyRuleInfo(newValue []SnapmirrorPolicyRuleInfoType) *SnapmirrorPolicyInfoTypeSnapmirrorPolicyRules {
	newSlice := make([]SnapmirrorPolicyRuleInfoType, len(newValue))
	copy(newSlice, newValue)
	o.SnapmirrorPolicyRuleInfoPtr = newSlice
	return o
}

// SnapmirrorPolicyRules is a 'getter' method
func (o *SnapmirrorPolicyInfoType) SnapmirrorPolicyRules() SnapmirrorPolicyInfoTypeSnapmirrorPolicyRules {

	var r SnapmirrorPolicyInfoTypeSnapmirrorPolicyRules
	if o.SnapmirrorPolicyRulesPtr == nil {
		return r
	}
	r = *o.SnapmirrorPolicyRulesPtr

	return r
}

// SetSnapmirrorPolicyRules is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetSnapmirrorPolicyRules(newValue SnapmirrorPolicyInfoTypeSnapmirrorPolicyRules) *SnapmirrorPolicyInfoType {
	o.SnapmirrorPolicyRulesPtr = &newValue
	return o
}

// TotalKeep is a 'getter' method
func (o *SnapmirrorPolicyInfoType) TotalKeep() int {

	var r int
	if o.TotalKeepPtr == nil {
		return r
	}
	r = *o.TotalKeepPtr

	return r
}

// SetTotalKeep is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetTotalKeep(newValue int) *SnapmirrorPolicyInfoType {
	o.TotalKeepPtr = &newValue
	return o
}

// TotalRules is a 'getter' method
func (o *SnapmirrorPolicyInfoType) TotalRules() int {

	var r int
	if o.TotalRulesPtr == nil {
		return r
	}
	r = *o.TotalRulesPtr

	return r
}

// SetTotalRules is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetTotalRules(newValue int) *SnapmirrorPolicyInfoType {
	o.TotalRulesPtr = &newValue
	return o
}

// TransferPriority is a 'getter' method
func (o *SnapmirrorPolicyInfoType) TransferPriority() SmTransferPriorityEnumType {

	var r SmTransferPriorityEnumType
	if o.TransferPriorityPtr == nil {
		return r
	}
	r = *o.TransferPriorityPtr

	return r
}

// SetTransferPriority is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetTransferPriority(newValue SmTransferPriorityEnumType) *SnapmirrorPolicyInfoType {
	o.TransferPriorityPtr = &newValue
	return o
}

// Tries is a 'getter' method
func (o *SnapmirrorPolicyInfoType) Tries() string {

	var r string
	if o.TriesPtr == nil {
		return r
	}
	r = *o.TriesPtr

	return r
}

// SetTries is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetTries(newValue string) *SnapmirrorPolicyInfoType {
	o.TriesPtr = &newValue
	return o
}

// Type is a 'getter' method
func (o *SnapmirrorPolicyInfoType) Type() SmPolicyTypeEnumType {

	var r SmPolicyTypeEnumType
	if o.TypePtr == nil {
		return r
	}
	r = *o.TypePtr

	return r
}

// SetType is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetType(newValue SmPolicyTypeEnumType) *SnapmirrorPolicyInfoType {
	o.TypePtr = &newValue
	return o
}

// VserverName is a 'getter' method
func (o *SnapmirrorPolicyInfoType) VserverName() VserverNameType {

	var r VserverNameType
	if o.VserverNamePtr == nil {
		return r
	}
	r = *o.VserverNamePtr

	return r
}

// SetVserverName is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetVserverName(newValue VserverNameType) *SnapmirrorPolicyInfoType {
	o.VserverNamePtr = &newValue
	return o
}

// WindowSizeForTdpMirror is a 'getter' method
func (o *SnapmirrorPolicyInfoType) WindowSizeForTdpMirror() SizeType {

	var r SizeType
	if o.WindowSizeForTdpMirrorPtr == nil {
		return r
	}
	r = *o.WindowSizeForTdpMirrorPtr

	return r
}

// SetWindowSizeForTdpMirror is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyInfoType) SetWindowSizeForTdpMirror(newValue SizeType) *SnapmirrorPolicyInfoType {
	o.WindowSizeForTdpMirrorPtr = &newValue
	return o
}
