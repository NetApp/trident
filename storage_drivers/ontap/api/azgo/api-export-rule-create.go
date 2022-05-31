// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// ExportRuleCreateRequest is a structure to represent a export-rule-create Request ZAPI object
type ExportRuleCreateRequest struct {
	XMLName                      xml.Name                                  `xml:"export-rule-create"`
	AnonymousUserIdPtr           *string                                   `xml:"anonymous-user-id"`
	ClientMatchPtr               *string                                   `xml:"client-match"`
	ExportChownModePtr           *ExportchownmodeType                      `xml:"export-chown-mode"`
	ExportNtfsUnixSecurityOpsPtr *ExportntfsunixsecopsType                 `xml:"export-ntfs-unix-security-ops"`
	IsAllowDevIsEnabledPtr       *bool                                     `xml:"is-allow-dev-is-enabled"`
	IsAllowSetUidEnabledPtr      *bool                                     `xml:"is-allow-set-uid-enabled"`
	PolicyNamePtr                *ExportPolicyNameType                     `xml:"policy-name"`
	ProtocolPtr                  *ExportRuleCreateRequestProtocol          `xml:"protocol"`
	RoRulePtr                    *ExportRuleCreateRequestRoRule            `xml:"ro-rule"`
	RuleIndexPtr                 *int                                      `xml:"rule-index"`
	RwRulePtr                    *ExportRuleCreateRequestRwRule            `xml:"rw-rule"`
	SuperUserSecurityPtr         *ExportRuleCreateRequestSuperUserSecurity `xml:"super-user-security"`
}

// ExportRuleCreateResponse is a structure to represent a export-rule-create Response ZAPI object
type ExportRuleCreateResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          ExportRuleCreateResponseResult `xml:"results"`
}

// NewExportRuleCreateResponse is a factory method for creating new instances of ExportRuleCreateResponse objects
func NewExportRuleCreateResponse() *ExportRuleCreateResponse {
	return &ExportRuleCreateResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleCreateResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ExportRuleCreateResponseResult is a structure to represent a export-rule-create Response Result ZAPI object
type ExportRuleCreateResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewExportRuleCreateRequest is a factory method for creating new instances of ExportRuleCreateRequest objects
func NewExportRuleCreateRequest() *ExportRuleCreateRequest {
	return &ExportRuleCreateRequest{}
}

// NewExportRuleCreateResponseResult is a factory method for creating new instances of ExportRuleCreateResponseResult objects
func NewExportRuleCreateResponseResult() *ExportRuleCreateResponseResult {
	return &ExportRuleCreateResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleCreateResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleCreateRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleCreateResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportRuleCreateRequest) ExecuteUsing(zr *ZapiRunner) (*ExportRuleCreateResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportRuleCreateRequest) executeWithoutIteration(zr *ZapiRunner) (*ExportRuleCreateResponse, error) {
	result, err := zr.ExecuteUsing(o, "ExportRuleCreateRequest", NewExportRuleCreateResponse())
	if result == nil {
		return nil, err
	}
	return result.(*ExportRuleCreateResponse), err
}

// AnonymousUserId is a 'getter' method
func (o *ExportRuleCreateRequest) AnonymousUserId() string {
	var r string
	if o.AnonymousUserIdPtr == nil {
		return r
	}
	r = *o.AnonymousUserIdPtr
	return r
}

// SetAnonymousUserId is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetAnonymousUserId(newValue string) *ExportRuleCreateRequest {
	o.AnonymousUserIdPtr = &newValue
	return o
}

// ClientMatch is a 'getter' method
func (o *ExportRuleCreateRequest) ClientMatch() string {
	var r string
	if o.ClientMatchPtr == nil {
		return r
	}
	r = *o.ClientMatchPtr
	return r
}

// SetClientMatch is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetClientMatch(newValue string) *ExportRuleCreateRequest {
	o.ClientMatchPtr = &newValue
	return o
}

// ExportChownMode is a 'getter' method
func (o *ExportRuleCreateRequest) ExportChownMode() ExportchownmodeType {
	var r ExportchownmodeType
	if o.ExportChownModePtr == nil {
		return r
	}
	r = *o.ExportChownModePtr
	return r
}

// SetExportChownMode is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetExportChownMode(newValue ExportchownmodeType) *ExportRuleCreateRequest {
	o.ExportChownModePtr = &newValue
	return o
}

// ExportNtfsUnixSecurityOps is a 'getter' method
func (o *ExportRuleCreateRequest) ExportNtfsUnixSecurityOps() ExportntfsunixsecopsType {
	var r ExportntfsunixsecopsType
	if o.ExportNtfsUnixSecurityOpsPtr == nil {
		return r
	}
	r = *o.ExportNtfsUnixSecurityOpsPtr
	return r
}

// SetExportNtfsUnixSecurityOps is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetExportNtfsUnixSecurityOps(newValue ExportntfsunixsecopsType) *ExportRuleCreateRequest {
	o.ExportNtfsUnixSecurityOpsPtr = &newValue
	return o
}

// IsAllowDevIsEnabled is a 'getter' method
func (o *ExportRuleCreateRequest) IsAllowDevIsEnabled() bool {
	var r bool
	if o.IsAllowDevIsEnabledPtr == nil {
		return r
	}
	r = *o.IsAllowDevIsEnabledPtr
	return r
}

// SetIsAllowDevIsEnabled is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetIsAllowDevIsEnabled(newValue bool) *ExportRuleCreateRequest {
	o.IsAllowDevIsEnabledPtr = &newValue
	return o
}

// IsAllowSetUidEnabled is a 'getter' method
func (o *ExportRuleCreateRequest) IsAllowSetUidEnabled() bool {
	var r bool
	if o.IsAllowSetUidEnabledPtr == nil {
		return r
	}
	r = *o.IsAllowSetUidEnabledPtr
	return r
}

// SetIsAllowSetUidEnabled is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetIsAllowSetUidEnabled(newValue bool) *ExportRuleCreateRequest {
	o.IsAllowSetUidEnabledPtr = &newValue
	return o
}

// PolicyName is a 'getter' method
func (o *ExportRuleCreateRequest) PolicyName() ExportPolicyNameType {
	var r ExportPolicyNameType
	if o.PolicyNamePtr == nil {
		return r
	}
	r = *o.PolicyNamePtr
	return r
}

// SetPolicyName is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetPolicyName(newValue ExportPolicyNameType) *ExportRuleCreateRequest {
	o.PolicyNamePtr = &newValue
	return o
}

// ExportRuleCreateRequestProtocol is a wrapper
type ExportRuleCreateRequestProtocol struct {
	XMLName           xml.Name             `xml:"protocol"`
	AccessProtocolPtr []AccessProtocolType `xml:"access-protocol"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleCreateRequestProtocol) String() string {
	return ToString(reflect.ValueOf(o))
}

// AccessProtocol is a 'getter' method
func (o *ExportRuleCreateRequestProtocol) AccessProtocol() []AccessProtocolType {
	r := o.AccessProtocolPtr
	return r
}

// SetAccessProtocol is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequestProtocol) SetAccessProtocol(newValue []AccessProtocolType) *ExportRuleCreateRequestProtocol {
	newSlice := make([]AccessProtocolType, len(newValue))
	copy(newSlice, newValue)
	o.AccessProtocolPtr = newSlice
	return o
}

// Protocol is a 'getter' method
func (o *ExportRuleCreateRequest) Protocol() ExportRuleCreateRequestProtocol {
	var r ExportRuleCreateRequestProtocol
	if o.ProtocolPtr == nil {
		return r
	}
	r = *o.ProtocolPtr
	return r
}

// SetProtocol is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetProtocol(newValue ExportRuleCreateRequestProtocol) *ExportRuleCreateRequest {
	o.ProtocolPtr = &newValue
	return o
}

// ExportRuleCreateRequestRoRule is a wrapper
type ExportRuleCreateRequestRoRule struct {
	XMLName           xml.Name             `xml:"ro-rule"`
	SecurityFlavorPtr []SecurityFlavorType `xml:"security-flavor"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleCreateRequestRoRule) String() string {
	return ToString(reflect.ValueOf(o))
}

// SecurityFlavor is a 'getter' method
func (o *ExportRuleCreateRequestRoRule) SecurityFlavor() []SecurityFlavorType {
	r := o.SecurityFlavorPtr
	return r
}

// SetSecurityFlavor is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequestRoRule) SetSecurityFlavor(newValue []SecurityFlavorType) *ExportRuleCreateRequestRoRule {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.SecurityFlavorPtr = newSlice
	return o
}

// RoRule is a 'getter' method
func (o *ExportRuleCreateRequest) RoRule() ExportRuleCreateRequestRoRule {
	var r ExportRuleCreateRequestRoRule
	if o.RoRulePtr == nil {
		return r
	}
	r = *o.RoRulePtr
	return r
}

// SetRoRule is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetRoRule(newValue ExportRuleCreateRequestRoRule) *ExportRuleCreateRequest {
	o.RoRulePtr = &newValue
	return o
}

// RuleIndex is a 'getter' method
func (o *ExportRuleCreateRequest) RuleIndex() int {
	var r int
	if o.RuleIndexPtr == nil {
		return r
	}
	r = *o.RuleIndexPtr
	return r
}

// SetRuleIndex is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetRuleIndex(newValue int) *ExportRuleCreateRequest {
	o.RuleIndexPtr = &newValue
	return o
}

// ExportRuleCreateRequestRwRule is a wrapper
type ExportRuleCreateRequestRwRule struct {
	XMLName           xml.Name             `xml:"rw-rule"`
	SecurityFlavorPtr []SecurityFlavorType `xml:"security-flavor"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleCreateRequestRwRule) String() string {
	return ToString(reflect.ValueOf(o))
}

// SecurityFlavor is a 'getter' method
func (o *ExportRuleCreateRequestRwRule) SecurityFlavor() []SecurityFlavorType {
	r := o.SecurityFlavorPtr
	return r
}

// SetSecurityFlavor is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequestRwRule) SetSecurityFlavor(newValue []SecurityFlavorType) *ExportRuleCreateRequestRwRule {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.SecurityFlavorPtr = newSlice
	return o
}

// RwRule is a 'getter' method
func (o *ExportRuleCreateRequest) RwRule() ExportRuleCreateRequestRwRule {
	var r ExportRuleCreateRequestRwRule
	if o.RwRulePtr == nil {
		return r
	}
	r = *o.RwRulePtr
	return r
}

// SetRwRule is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetRwRule(newValue ExportRuleCreateRequestRwRule) *ExportRuleCreateRequest {
	o.RwRulePtr = &newValue
	return o
}

// ExportRuleCreateRequestSuperUserSecurity is a wrapper
type ExportRuleCreateRequestSuperUserSecurity struct {
	XMLName           xml.Name             `xml:"super-user-security"`
	SecurityFlavorPtr []SecurityFlavorType `xml:"security-flavor"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleCreateRequestSuperUserSecurity) String() string {
	return ToString(reflect.ValueOf(o))
}

// SecurityFlavor is a 'getter' method
func (o *ExportRuleCreateRequestSuperUserSecurity) SecurityFlavor() []SecurityFlavorType {
	r := o.SecurityFlavorPtr
	return r
}

// SetSecurityFlavor is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequestSuperUserSecurity) SetSecurityFlavor(newValue []SecurityFlavorType) *ExportRuleCreateRequestSuperUserSecurity {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.SecurityFlavorPtr = newSlice
	return o
}

// SuperUserSecurity is a 'getter' method
func (o *ExportRuleCreateRequest) SuperUserSecurity() ExportRuleCreateRequestSuperUserSecurity {
	var r ExportRuleCreateRequestSuperUserSecurity
	if o.SuperUserSecurityPtr == nil {
		return r
	}
	r = *o.SuperUserSecurityPtr
	return r
}

// SetSuperUserSecurity is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetSuperUserSecurity(newValue ExportRuleCreateRequestSuperUserSecurity) *ExportRuleCreateRequest {
	o.SuperUserSecurityPtr = &newValue
	return o
}
