package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// SnapmirrorPolicyRuleInfoType is a structure to represent a snapmirror-policy-rule-info ZAPI object
type SnapmirrorPolicyRuleInfoType struct {
	XMLName            xml.Name `xml:"snapmirror-policy-rule-info"`
	KeepPtr            *string  `xml:"keep"`
	PrefixPtr          *string  `xml:"prefix"`
	PreservePtr        *bool    `xml:"preserve"`
	SchedulePtr        *string  `xml:"schedule"`
	SnapmirrorLabelPtr *string  `xml:"snapmirror-label"`
	WarnPtr            *int     `xml:"warn"`
}

// NewSnapmirrorPolicyRuleInfoType is a factory method for creating new instances of SnapmirrorPolicyRuleInfoType objects
func NewSnapmirrorPolicyRuleInfoType() *SnapmirrorPolicyRuleInfoType {
	return &SnapmirrorPolicyRuleInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *SnapmirrorPolicyRuleInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o SnapmirrorPolicyRuleInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Keep is a 'getter' method
func (o *SnapmirrorPolicyRuleInfoType) Keep() string {

	var r string
	if o.KeepPtr == nil {
		return r
	}
	r = *o.KeepPtr

	return r
}

// SetKeep is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyRuleInfoType) SetKeep(newValue string) *SnapmirrorPolicyRuleInfoType {
	o.KeepPtr = &newValue
	return o
}

// Prefix is a 'getter' method
func (o *SnapmirrorPolicyRuleInfoType) Prefix() string {

	var r string
	if o.PrefixPtr == nil {
		return r
	}
	r = *o.PrefixPtr

	return r
}

// SetPrefix is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyRuleInfoType) SetPrefix(newValue string) *SnapmirrorPolicyRuleInfoType {
	o.PrefixPtr = &newValue
	return o
}

// Preserve is a 'getter' method
func (o *SnapmirrorPolicyRuleInfoType) Preserve() bool {

	var r bool
	if o.PreservePtr == nil {
		return r
	}
	r = *o.PreservePtr

	return r
}

// SetPreserve is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyRuleInfoType) SetPreserve(newValue bool) *SnapmirrorPolicyRuleInfoType {
	o.PreservePtr = &newValue
	return o
}

// Schedule is a 'getter' method
func (o *SnapmirrorPolicyRuleInfoType) Schedule() string {

	var r string
	if o.SchedulePtr == nil {
		return r
	}
	r = *o.SchedulePtr

	return r
}

// SetSchedule is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyRuleInfoType) SetSchedule(newValue string) *SnapmirrorPolicyRuleInfoType {
	o.SchedulePtr = &newValue
	return o
}

// SnapmirrorLabel is a 'getter' method
func (o *SnapmirrorPolicyRuleInfoType) SnapmirrorLabel() string {

	var r string
	if o.SnapmirrorLabelPtr == nil {
		return r
	}
	r = *o.SnapmirrorLabelPtr

	return r
}

// SetSnapmirrorLabel is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyRuleInfoType) SetSnapmirrorLabel(newValue string) *SnapmirrorPolicyRuleInfoType {
	o.SnapmirrorLabelPtr = &newValue
	return o
}

// Warn is a 'getter' method
func (o *SnapmirrorPolicyRuleInfoType) Warn() int {

	var r int
	if o.WarnPtr == nil {
		return r
	}
	r = *o.WarnPtr

	return r
}

// SetWarn is a fluent style 'setter' method that can be chained
func (o *SnapmirrorPolicyRuleInfoType) SetWarn(newValue int) *SnapmirrorPolicyRuleInfoType {
	o.WarnPtr = &newValue
	return o
}
