// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// QtreeInfoType is a structure to represent a qtree-info ZAPI object
type QtreeInfoType struct {
	XMLName                    xml.Name `xml:"qtree-info"`
	ExportPolicyPtr            *string  `xml:"export-policy"`
	IdPtr                      *int     `xml:"id"`
	IsExportPolicyInheritedPtr *bool    `xml:"is-export-policy-inherited"`
	ModePtr                    *string  `xml:"mode"`
	OplocksPtr                 *string  `xml:"oplocks"`
	QosPolicyGroupPtr          *string  `xml:"qos-policy-group"`
	QtreePtr                   *string  `xml:"qtree"`
	SecurityStylePtr           *string  `xml:"security-style"`
	StatusPtr                  *string  `xml:"status"`
	VolumePtr                  *string  `xml:"volume"`
	VserverPtr                 *string  `xml:"vserver"`
}

// NewQtreeInfoType is a factory method for creating new instances of QtreeInfoType objects
func NewQtreeInfoType() *QtreeInfoType {
	return &QtreeInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *QtreeInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o QtreeInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExportPolicy is a 'getter' method
func (o *QtreeInfoType) ExportPolicy() string {
	var r string
	if o.ExportPolicyPtr == nil {
		return r
	}
	r = *o.ExportPolicyPtr
	return r
}

// SetExportPolicy is a fluent style 'setter' method that can be chained
func (o *QtreeInfoType) SetExportPolicy(newValue string) *QtreeInfoType {
	o.ExportPolicyPtr = &newValue
	return o
}

// Id is a 'getter' method
func (o *QtreeInfoType) Id() int {
	var r int
	if o.IdPtr == nil {
		return r
	}
	r = *o.IdPtr
	return r
}

// SetId is a fluent style 'setter' method that can be chained
func (o *QtreeInfoType) SetId(newValue int) *QtreeInfoType {
	o.IdPtr = &newValue
	return o
}

// IsExportPolicyInherited is a 'getter' method
func (o *QtreeInfoType) IsExportPolicyInherited() bool {
	var r bool
	if o.IsExportPolicyInheritedPtr == nil {
		return r
	}
	r = *o.IsExportPolicyInheritedPtr
	return r
}

// SetIsExportPolicyInherited is a fluent style 'setter' method that can be chained
func (o *QtreeInfoType) SetIsExportPolicyInherited(newValue bool) *QtreeInfoType {
	o.IsExportPolicyInheritedPtr = &newValue
	return o
}

// Mode is a 'getter' method
func (o *QtreeInfoType) Mode() string {
	var r string
	if o.ModePtr == nil {
		return r
	}
	r = *o.ModePtr
	return r
}

// SetMode is a fluent style 'setter' method that can be chained
func (o *QtreeInfoType) SetMode(newValue string) *QtreeInfoType {
	o.ModePtr = &newValue
	return o
}

// Oplocks is a 'getter' method
func (o *QtreeInfoType) Oplocks() string {
	var r string
	if o.OplocksPtr == nil {
		return r
	}
	r = *o.OplocksPtr
	return r
}

// SetOplocks is a fluent style 'setter' method that can be chained
func (o *QtreeInfoType) SetOplocks(newValue string) *QtreeInfoType {
	o.OplocksPtr = &newValue
	return o
}

// QosPolicyGroup is a 'getter' method
func (o *QtreeInfoType) QosPolicyGroup() string {
	var r string
	if o.QosPolicyGroupPtr == nil {
		return r
	}
	r = *o.QosPolicyGroupPtr
	return r
}

// SetQosPolicyGroup is a fluent style 'setter' method that can be chained
func (o *QtreeInfoType) SetQosPolicyGroup(newValue string) *QtreeInfoType {
	o.QosPolicyGroupPtr = &newValue
	return o
}

// Qtree is a 'getter' method
func (o *QtreeInfoType) Qtree() string {
	var r string
	if o.QtreePtr == nil {
		return r
	}
	r = *o.QtreePtr
	return r
}

// SetQtree is a fluent style 'setter' method that can be chained
func (o *QtreeInfoType) SetQtree(newValue string) *QtreeInfoType {
	o.QtreePtr = &newValue
	return o
}

// SecurityStyle is a 'getter' method
func (o *QtreeInfoType) SecurityStyle() string {
	var r string
	if o.SecurityStylePtr == nil {
		return r
	}
	r = *o.SecurityStylePtr
	return r
}

// SetSecurityStyle is a fluent style 'setter' method that can be chained
func (o *QtreeInfoType) SetSecurityStyle(newValue string) *QtreeInfoType {
	o.SecurityStylePtr = &newValue
	return o
}

// Status is a 'getter' method
func (o *QtreeInfoType) Status() string {
	var r string
	if o.StatusPtr == nil {
		return r
	}
	r = *o.StatusPtr
	return r
}

// SetStatus is a fluent style 'setter' method that can be chained
func (o *QtreeInfoType) SetStatus(newValue string) *QtreeInfoType {
	o.StatusPtr = &newValue
	return o
}

// Volume is a 'getter' method
func (o *QtreeInfoType) Volume() string {
	var r string
	if o.VolumePtr == nil {
		return r
	}
	r = *o.VolumePtr
	return r
}

// SetVolume is a fluent style 'setter' method that can be chained
func (o *QtreeInfoType) SetVolume(newValue string) *QtreeInfoType {
	o.VolumePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *QtreeInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *QtreeInfoType) SetVserver(newValue string) *QtreeInfoType {
	o.VserverPtr = &newValue
	return o
}
