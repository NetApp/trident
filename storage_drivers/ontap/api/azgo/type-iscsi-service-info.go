// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// IscsiServiceInfoType is a structure to represent a iscsi-service-info ZAPI object
type IscsiServiceInfoType struct {
	XMLName                  xml.Name `xml:"iscsi-service-info"`
	AliasNamePtr             *string  `xml:"alias-name"`
	IsAvailablePtr           *bool    `xml:"is-available"`
	LoginTimeoutPtr          *int     `xml:"login-timeout"`
	MaxCmdsPerSessionPtr     *int     `xml:"max-cmds-per-session"`
	MaxConnPerSessionPtr     *int     `xml:"max-conn-per-session"`
	MaxErrorRecoveryLevelPtr *int     `xml:"max-error-recovery-level"`
	NodeNamePtr              *string  `xml:"node-name"`
	RetainTimeoutPtr         *int     `xml:"retain-timeout"`
	TcpWindowSizePtr         *int     `xml:"tcp-window-size"`
	VserverPtr               *string  `xml:"vserver"`
}

// NewIscsiServiceInfoType is a factory method for creating new instances of IscsiServiceInfoType objects
func NewIscsiServiceInfoType() *IscsiServiceInfoType {
	return &IscsiServiceInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *IscsiServiceInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o IscsiServiceInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AliasName is a 'getter' method
func (o *IscsiServiceInfoType) AliasName() string {
	var r string
	if o.AliasNamePtr == nil {
		return r
	}
	r = *o.AliasNamePtr
	return r
}

// SetAliasName is a fluent style 'setter' method that can be chained
func (o *IscsiServiceInfoType) SetAliasName(newValue string) *IscsiServiceInfoType {
	o.AliasNamePtr = &newValue
	return o
}

// IsAvailable is a 'getter' method
func (o *IscsiServiceInfoType) IsAvailable() bool {
	var r bool
	if o.IsAvailablePtr == nil {
		return r
	}
	r = *o.IsAvailablePtr
	return r
}

// SetIsAvailable is a fluent style 'setter' method that can be chained
func (o *IscsiServiceInfoType) SetIsAvailable(newValue bool) *IscsiServiceInfoType {
	o.IsAvailablePtr = &newValue
	return o
}

// LoginTimeout is a 'getter' method
func (o *IscsiServiceInfoType) LoginTimeout() int {
	var r int
	if o.LoginTimeoutPtr == nil {
		return r
	}
	r = *o.LoginTimeoutPtr
	return r
}

// SetLoginTimeout is a fluent style 'setter' method that can be chained
func (o *IscsiServiceInfoType) SetLoginTimeout(newValue int) *IscsiServiceInfoType {
	o.LoginTimeoutPtr = &newValue
	return o
}

// MaxCmdsPerSession is a 'getter' method
func (o *IscsiServiceInfoType) MaxCmdsPerSession() int {
	var r int
	if o.MaxCmdsPerSessionPtr == nil {
		return r
	}
	r = *o.MaxCmdsPerSessionPtr
	return r
}

// SetMaxCmdsPerSession is a fluent style 'setter' method that can be chained
func (o *IscsiServiceInfoType) SetMaxCmdsPerSession(newValue int) *IscsiServiceInfoType {
	o.MaxCmdsPerSessionPtr = &newValue
	return o
}

// MaxConnPerSession is a 'getter' method
func (o *IscsiServiceInfoType) MaxConnPerSession() int {
	var r int
	if o.MaxConnPerSessionPtr == nil {
		return r
	}
	r = *o.MaxConnPerSessionPtr
	return r
}

// SetMaxConnPerSession is a fluent style 'setter' method that can be chained
func (o *IscsiServiceInfoType) SetMaxConnPerSession(newValue int) *IscsiServiceInfoType {
	o.MaxConnPerSessionPtr = &newValue
	return o
}

// MaxErrorRecoveryLevel is a 'getter' method
func (o *IscsiServiceInfoType) MaxErrorRecoveryLevel() int {
	var r int
	if o.MaxErrorRecoveryLevelPtr == nil {
		return r
	}
	r = *o.MaxErrorRecoveryLevelPtr
	return r
}

// SetMaxErrorRecoveryLevel is a fluent style 'setter' method that can be chained
func (o *IscsiServiceInfoType) SetMaxErrorRecoveryLevel(newValue int) *IscsiServiceInfoType {
	o.MaxErrorRecoveryLevelPtr = &newValue
	return o
}

// NodeName is a 'getter' method
func (o *IscsiServiceInfoType) NodeName() string {
	var r string
	if o.NodeNamePtr == nil {
		return r
	}
	r = *o.NodeNamePtr
	return r
}

// SetNodeName is a fluent style 'setter' method that can be chained
func (o *IscsiServiceInfoType) SetNodeName(newValue string) *IscsiServiceInfoType {
	o.NodeNamePtr = &newValue
	return o
}

// RetainTimeout is a 'getter' method
func (o *IscsiServiceInfoType) RetainTimeout() int {
	var r int
	if o.RetainTimeoutPtr == nil {
		return r
	}
	r = *o.RetainTimeoutPtr
	return r
}

// SetRetainTimeout is a fluent style 'setter' method that can be chained
func (o *IscsiServiceInfoType) SetRetainTimeout(newValue int) *IscsiServiceInfoType {
	o.RetainTimeoutPtr = &newValue
	return o
}

// TcpWindowSize is a 'getter' method
func (o *IscsiServiceInfoType) TcpWindowSize() int {
	var r int
	if o.TcpWindowSizePtr == nil {
		return r
	}
	r = *o.TcpWindowSizePtr
	return r
}

// SetTcpWindowSize is a fluent style 'setter' method that can be chained
func (o *IscsiServiceInfoType) SetTcpWindowSize(newValue int) *IscsiServiceInfoType {
	o.TcpWindowSizePtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *IscsiServiceInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *IscsiServiceInfoType) SetVserver(newValue string) *IscsiServiceInfoType {
	o.VserverPtr = &newValue
	return o
}
