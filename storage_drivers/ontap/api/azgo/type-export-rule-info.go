// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// ExportRuleInfoType is a structure to represent a export-rule-info ZAPI object
type ExportRuleInfoType struct {
	XMLName                      xml.Name                    `xml:"export-rule-info"`
	AnonymousUserIdPtr           *string                     `xml:"anonymous-user-id"`
	ClientMatchPtr               *string                     `xml:"client-match"`
	ExportChownModePtr           *ExportchownmodeType        `xml:"export-chown-mode"`
	ExportNtfsUnixSecurityOpsPtr *ExportntfsunixsecopsType   `xml:"export-ntfs-unix-security-ops"`
	IsAllowDevIsEnabledPtr       *bool                       `xml:"is-allow-dev-is-enabled"`
	IsAllowSetUidEnabledPtr      *bool                       `xml:"is-allow-set-uid-enabled"`
	PolicyNamePtr                *ExportPolicyNameType       `xml:"policy-name"`
	ProtocolPtr                  *ExportRuleInfoTypeProtocol `xml:"protocol"`
	// work in progress
	RoRulePtr *ExportRuleInfoTypeRoRule `xml:"ro-rule"`
	// work in progress
	RuleIndexPtr *int                      `xml:"rule-index"`
	RwRulePtr    *ExportRuleInfoTypeRwRule `xml:"rw-rule"`
	// work in progress
	SuperUserSecurityPtr *ExportRuleInfoTypeSuperUserSecurity `xml:"super-user-security"`
	// work in progress
	VserverNamePtr *string `xml:"vserver-name"`
}

// NewExportRuleInfoType is a factory method for creating new instances of ExportRuleInfoType objects
func NewExportRuleInfoType() *ExportRuleInfoType {
	return &ExportRuleInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// AnonymousUserId is a 'getter' method
func (o *ExportRuleInfoType) AnonymousUserId() string {
	var r string
	if o.AnonymousUserIdPtr == nil {
		return r
	}
	r = *o.AnonymousUserIdPtr
	return r
}

// SetAnonymousUserId is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetAnonymousUserId(newValue string) *ExportRuleInfoType {
	o.AnonymousUserIdPtr = &newValue
	return o
}

// ClientMatch is a 'getter' method
func (o *ExportRuleInfoType) ClientMatch() string {
	var r string
	if o.ClientMatchPtr == nil {
		return r
	}
	r = *o.ClientMatchPtr
	return r
}

// SetClientMatch is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetClientMatch(newValue string) *ExportRuleInfoType {
	o.ClientMatchPtr = &newValue
	return o
}

// ExportChownMode is a 'getter' method
func (o *ExportRuleInfoType) ExportChownMode() ExportchownmodeType {
	var r ExportchownmodeType
	if o.ExportChownModePtr == nil {
		return r
	}
	r = *o.ExportChownModePtr
	return r
}

// SetExportChownMode is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetExportChownMode(newValue ExportchownmodeType) *ExportRuleInfoType {
	o.ExportChownModePtr = &newValue
	return o
}

// ExportNtfsUnixSecurityOps is a 'getter' method
func (o *ExportRuleInfoType) ExportNtfsUnixSecurityOps() ExportntfsunixsecopsType {
	var r ExportntfsunixsecopsType
	if o.ExportNtfsUnixSecurityOpsPtr == nil {
		return r
	}
	r = *o.ExportNtfsUnixSecurityOpsPtr
	return r
}

// SetExportNtfsUnixSecurityOps is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetExportNtfsUnixSecurityOps(newValue ExportntfsunixsecopsType) *ExportRuleInfoType {
	o.ExportNtfsUnixSecurityOpsPtr = &newValue
	return o
}

// IsAllowDevIsEnabled is a 'getter' method
func (o *ExportRuleInfoType) IsAllowDevIsEnabled() bool {
	var r bool
	if o.IsAllowDevIsEnabledPtr == nil {
		return r
	}
	r = *o.IsAllowDevIsEnabledPtr
	return r
}

// SetIsAllowDevIsEnabled is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetIsAllowDevIsEnabled(newValue bool) *ExportRuleInfoType {
	o.IsAllowDevIsEnabledPtr = &newValue
	return o
}

// IsAllowSetUidEnabled is a 'getter' method
func (o *ExportRuleInfoType) IsAllowSetUidEnabled() bool {
	var r bool
	if o.IsAllowSetUidEnabledPtr == nil {
		return r
	}
	r = *o.IsAllowSetUidEnabledPtr
	return r
}

// SetIsAllowSetUidEnabled is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetIsAllowSetUidEnabled(newValue bool) *ExportRuleInfoType {
	o.IsAllowSetUidEnabledPtr = &newValue
	return o
}

// PolicyName is a 'getter' method
func (o *ExportRuleInfoType) PolicyName() ExportPolicyNameType {
	var r ExportPolicyNameType
	if o.PolicyNamePtr == nil {
		return r
	}
	r = *o.PolicyNamePtr
	return r
}

// SetPolicyName is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetPolicyName(newValue ExportPolicyNameType) *ExportRuleInfoType {
	o.PolicyNamePtr = &newValue
	return o
}

// ExportRuleInfoTypeProtocol is a wrapper
type ExportRuleInfoTypeProtocol struct {
	XMLName           xml.Name             `xml:"protocol"`
	AccessProtocolPtr []AccessProtocolType `xml:"access-protocol"`
}

// AccessProtocol is a 'getter' method
func (o *ExportRuleInfoTypeProtocol) AccessProtocol() []AccessProtocolType {
	r := o.AccessProtocolPtr
	return r
}

// SetAccessProtocol is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoTypeProtocol) SetAccessProtocol(newValue []AccessProtocolType) *ExportRuleInfoTypeProtocol {
	newSlice := make([]AccessProtocolType, len(newValue))
	copy(newSlice, newValue)
	o.AccessProtocolPtr = newSlice
	return o
}

// Protocol is a 'getter' method
func (o *ExportRuleInfoType) Protocol() ExportRuleInfoTypeProtocol {
	var r ExportRuleInfoTypeProtocol
	if o.ProtocolPtr == nil {
		return r
	}
	r = *o.ProtocolPtr
	return r
}

// SetProtocol is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetProtocol(newValue ExportRuleInfoTypeProtocol) *ExportRuleInfoType {
	o.ProtocolPtr = &newValue
	return o
}

// ExportRuleInfoTypeRoRule is a wrapper
type ExportRuleInfoTypeRoRule struct {
	XMLName           xml.Name             `xml:"ro-rule"`
	SecurityFlavorPtr []SecurityFlavorType `xml:"security-flavor"`
}

// SecurityFlavor is a 'getter' method
func (o *ExportRuleInfoTypeRoRule) SecurityFlavor() []SecurityFlavorType {
	r := o.SecurityFlavorPtr
	return r
}

// SetSecurityFlavor is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoTypeRoRule) SetSecurityFlavor(newValue []SecurityFlavorType) *ExportRuleInfoTypeRoRule {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.SecurityFlavorPtr = newSlice
	return o
}

// RoRule is a 'getter' method
func (o *ExportRuleInfoType) RoRule() ExportRuleInfoTypeRoRule {
	var r ExportRuleInfoTypeRoRule
	if o.RoRulePtr == nil {
		return r
	}
	r = *o.RoRulePtr
	return r
}

// SetRoRule is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetRoRule(newValue ExportRuleInfoTypeRoRule) *ExportRuleInfoType {
	o.RoRulePtr = &newValue
	return o
}

// RuleIndex is a 'getter' method
func (o *ExportRuleInfoType) RuleIndex() int {
	var r int
	if o.RuleIndexPtr == nil {
		return r
	}
	r = *o.RuleIndexPtr
	return r
}

// SetRuleIndex is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetRuleIndex(newValue int) *ExportRuleInfoType {
	o.RuleIndexPtr = &newValue
	return o
}

// ExportRuleInfoTypeRwRule is a wrapper
type ExportRuleInfoTypeRwRule struct {
	XMLName           xml.Name             `xml:"rw-rule"`
	SecurityFlavorPtr []SecurityFlavorType `xml:"security-flavor"`
}

// SecurityFlavor is a 'getter' method
func (o *ExportRuleInfoTypeRwRule) SecurityFlavor() []SecurityFlavorType {
	r := o.SecurityFlavorPtr
	return r
}

// SetSecurityFlavor is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoTypeRwRule) SetSecurityFlavor(newValue []SecurityFlavorType) *ExportRuleInfoTypeRwRule {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.SecurityFlavorPtr = newSlice
	return o
}

// RwRule is a 'getter' method
func (o *ExportRuleInfoType) RwRule() ExportRuleInfoTypeRwRule {
	var r ExportRuleInfoTypeRwRule
	if o.RwRulePtr == nil {
		return r
	}
	r = *o.RwRulePtr
	return r
}

// SetRwRule is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetRwRule(newValue ExportRuleInfoTypeRwRule) *ExportRuleInfoType {
	o.RwRulePtr = &newValue
	return o
}

// ExportRuleInfoTypeSuperUserSecurity is a wrapper
type ExportRuleInfoTypeSuperUserSecurity struct {
	XMLName           xml.Name             `xml:"super-user-security"`
	SecurityFlavorPtr []SecurityFlavorType `xml:"security-flavor"`
}

// SecurityFlavor is a 'getter' method
func (o *ExportRuleInfoTypeSuperUserSecurity) SecurityFlavor() []SecurityFlavorType {
	r := o.SecurityFlavorPtr
	return r
}

// SetSecurityFlavor is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoTypeSuperUserSecurity) SetSecurityFlavor(newValue []SecurityFlavorType) *ExportRuleInfoTypeSuperUserSecurity {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.SecurityFlavorPtr = newSlice
	return o
}

// SuperUserSecurity is a 'getter' method
func (o *ExportRuleInfoType) SuperUserSecurity() ExportRuleInfoTypeSuperUserSecurity {
	var r ExportRuleInfoTypeSuperUserSecurity
	if o.SuperUserSecurityPtr == nil {
		return r
	}
	r = *o.SuperUserSecurityPtr
	return r
}

// SetSuperUserSecurity is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetSuperUserSecurity(newValue ExportRuleInfoTypeSuperUserSecurity) *ExportRuleInfoType {
	o.SuperUserSecurityPtr = &newValue
	return o
}

// VserverName is a 'getter' method
func (o *ExportRuleInfoType) VserverName() string {
	var r string
	if o.VserverNamePtr == nil {
		return r
	}
	r = *o.VserverNamePtr
	return r
}

// SetVserverName is a fluent style 'setter' method that can be chained
func (o *ExportRuleInfoType) SetVserverName(newValue string) *ExportRuleInfoType {
	o.VserverNamePtr = &newValue
	return o
}
