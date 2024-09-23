// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// VolumeTransitionAttributesType is a structure to represent a volume-transition-attributes ZAPI object
type VolumeTransitionAttributesType struct {
	XMLName                  xml.Name `xml:"volume-transition-attributes"`
	IsCftPrecommitPtr        *bool    `xml:"is-cft-precommit"`
	IsCopiedForTransitionPtr *bool    `xml:"is-copied-for-transition"`
	IsTransitionedPtr        *bool    `xml:"is-transitioned"`
	TransitionBehaviorPtr    *string  `xml:"transition-behavior"`
}

// NewVolumeTransitionAttributesType is a factory method for creating new instances of VolumeTransitionAttributesType objects
func NewVolumeTransitionAttributesType() *VolumeTransitionAttributesType {
	return &VolumeTransitionAttributesType{}
}

// ToXML converts this object into an xml string representation
func (o *VolumeTransitionAttributesType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VolumeTransitionAttributesType) String() string {
	return ToString(reflect.ValueOf(o))
}

// IsCftPrecommit is a 'getter' method
func (o *VolumeTransitionAttributesType) IsCftPrecommit() bool {
	var r bool
	if o.IsCftPrecommitPtr == nil {
		return r
	}
	r = *o.IsCftPrecommitPtr
	return r
}

// SetIsCftPrecommit is a fluent style 'setter' method that can be chained
func (o *VolumeTransitionAttributesType) SetIsCftPrecommit(newValue bool) *VolumeTransitionAttributesType {
	o.IsCftPrecommitPtr = &newValue
	return o
}

// IsCopiedForTransition is a 'getter' method
func (o *VolumeTransitionAttributesType) IsCopiedForTransition() bool {
	var r bool
	if o.IsCopiedForTransitionPtr == nil {
		return r
	}
	r = *o.IsCopiedForTransitionPtr
	return r
}

// SetIsCopiedForTransition is a fluent style 'setter' method that can be chained
func (o *VolumeTransitionAttributesType) SetIsCopiedForTransition(newValue bool) *VolumeTransitionAttributesType {
	o.IsCopiedForTransitionPtr = &newValue
	return o
}

// IsTransitioned is a 'getter' method
func (o *VolumeTransitionAttributesType) IsTransitioned() bool {
	var r bool
	if o.IsTransitionedPtr == nil {
		return r
	}
	r = *o.IsTransitionedPtr
	return r
}

// SetIsTransitioned is a fluent style 'setter' method that can be chained
func (o *VolumeTransitionAttributesType) SetIsTransitioned(newValue bool) *VolumeTransitionAttributesType {
	o.IsTransitionedPtr = &newValue
	return o
}

// TransitionBehavior is a 'getter' method
func (o *VolumeTransitionAttributesType) TransitionBehavior() string {
	var r string
	if o.TransitionBehaviorPtr == nil {
		return r
	}
	r = *o.TransitionBehaviorPtr
	return r
}

// SetTransitionBehavior is a fluent style 'setter' method that can be chained
func (o *VolumeTransitionAttributesType) SetTransitionBehavior(newValue string) *VolumeTransitionAttributesType {
	o.TransitionBehaviorPtr = &newValue
	return o
}
