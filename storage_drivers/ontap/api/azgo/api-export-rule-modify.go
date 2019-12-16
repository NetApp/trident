package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
)

// ExportRuleModifyRequest is a structure to represent a export-rule-modify Request ZAPI object
type ExportRuleModifyRequest struct {
	XMLName                      xml.Name                                  `xml:"export-rule-modify"`
	AnonymousUserIdPtr           *string                                   `xml:"anonymous-user-id"`
	ClientMatchPtr               *string                                   `xml:"client-match"`
	ExportChownModePtr           *ExportchownmodeType                      `xml:"export-chown-mode"`
	ExportNtfsUnixSecurityOpsPtr *ExportntfsunixsecopsType                 `xml:"export-ntfs-unix-security-ops"`
	IsAllowDevIsEnabledPtr       *bool                                     `xml:"is-allow-dev-is-enabled"`
	IsAllowSetUidEnabledPtr      *bool                                     `xml:"is-allow-set-uid-enabled"`
	PolicyNamePtr                *ExportPolicyNameType                     `xml:"policy-name"`
	ProtocolPtr                  *ExportRuleModifyRequestProtocol          `xml:"protocol"`
	RoRulePtr                    *ExportRuleModifyRequestRoRule            `xml:"ro-rule"`
	RuleIndexPtr                 *int                                      `xml:"rule-index"`
	RwRulePtr                    *ExportRuleModifyRequestRwRule            `xml:"rw-rule"`
	SuperUserSecurityPtr         *ExportRuleModifyRequestSuperUserSecurity `xml:"super-user-security"`
}

// ExportRuleModifyResponse is a structure to represent a export-rule-modify Response ZAPI object
type ExportRuleModifyResponse struct {
	XMLName         xml.Name                       `xml:"netapp"`
	ResponseVersion string                         `xml:"version,attr"`
	ResponseXmlns   string                         `xml:"xmlns,attr"`
	Result          ExportRuleModifyResponseResult `xml:"results"`
}

// NewExportRuleModifyResponse is a factory method for creating new instances of ExportRuleModifyResponse objects
func NewExportRuleModifyResponse() *ExportRuleModifyResponse {
	return &ExportRuleModifyResponse{}
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleModifyResponse) String() string {
	return ToString(reflect.ValueOf(o))
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleModifyResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ExportRuleModifyResponseResult is a structure to represent a export-rule-modify Response Result ZAPI object
type ExportRuleModifyResponseResult struct {
	XMLName          xml.Name `xml:"results"`
	ResultStatusAttr string   `xml:"status,attr"`
	ResultReasonAttr string   `xml:"reason,attr"`
	ResultErrnoAttr  string   `xml:"errno,attr"`
}

// NewExportRuleModifyRequest is a factory method for creating new instances of ExportRuleModifyRequest objects
func NewExportRuleModifyRequest() *ExportRuleModifyRequest {
	return &ExportRuleModifyRequest{}
}

// NewExportRuleModifyResponseResult is a factory method for creating new instances of ExportRuleModifyResponseResult objects
func NewExportRuleModifyResponseResult() *ExportRuleModifyResponseResult {
	return &ExportRuleModifyResponseResult{}
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleModifyRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleModifyResponseResult) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleModifyRequest) String() string {
	return ToString(reflect.ValueOf(o))
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleModifyResponseResult) String() string {
	return ToString(reflect.ValueOf(o))
}

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportRuleModifyRequest) ExecuteUsing(zr *ZapiRunner) (*ExportRuleModifyResponse, error) {
	return o.executeWithoutIteration(zr)
}

// executeWithoutIteration converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer

func (o *ExportRuleModifyRequest) executeWithoutIteration(zr *ZapiRunner) (*ExportRuleModifyResponse, error) {
	result, err := zr.ExecuteUsing(o, "ExportRuleModifyRequest", NewExportRuleModifyResponse())
	if result == nil {
		return nil, err
	}
	return result.(*ExportRuleModifyResponse), err
}

// AnonymousUserId is a 'getter' method
func (o *ExportRuleModifyRequest) AnonymousUserId() string {
	r := *o.AnonymousUserIdPtr
	return r
}

// SetAnonymousUserId is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetAnonymousUserId(newValue string) *ExportRuleModifyRequest {
	o.AnonymousUserIdPtr = &newValue
	return o
}

// ClientMatch is a 'getter' method
func (o *ExportRuleModifyRequest) ClientMatch() string {
	r := *o.ClientMatchPtr
	return r
}

// SetClientMatch is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetClientMatch(newValue string) *ExportRuleModifyRequest {
	o.ClientMatchPtr = &newValue
	return o
}

// ExportChownMode is a 'getter' method
func (o *ExportRuleModifyRequest) ExportChownMode() ExportchownmodeType {
	r := *o.ExportChownModePtr
	return r
}

// SetExportChownMode is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetExportChownMode(newValue ExportchownmodeType) *ExportRuleModifyRequest {
	o.ExportChownModePtr = &newValue
	return o
}

// ExportNtfsUnixSecurityOps is a 'getter' method
func (o *ExportRuleModifyRequest) ExportNtfsUnixSecurityOps() ExportntfsunixsecopsType {
	r := *o.ExportNtfsUnixSecurityOpsPtr
	return r
}

// SetExportNtfsUnixSecurityOps is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetExportNtfsUnixSecurityOps(newValue ExportntfsunixsecopsType) *ExportRuleModifyRequest {
	o.ExportNtfsUnixSecurityOpsPtr = &newValue
	return o
}

// IsAllowDevIsEnabled is a 'getter' method
func (o *ExportRuleModifyRequest) IsAllowDevIsEnabled() bool {
	r := *o.IsAllowDevIsEnabledPtr
	return r
}

// SetIsAllowDevIsEnabled is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetIsAllowDevIsEnabled(newValue bool) *ExportRuleModifyRequest {
	o.IsAllowDevIsEnabledPtr = &newValue
	return o
}

// IsAllowSetUidEnabled is a 'getter' method
func (o *ExportRuleModifyRequest) IsAllowSetUidEnabled() bool {
	r := *o.IsAllowSetUidEnabledPtr
	return r
}

// SetIsAllowSetUidEnabled is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetIsAllowSetUidEnabled(newValue bool) *ExportRuleModifyRequest {
	o.IsAllowSetUidEnabledPtr = &newValue
	return o
}

// PolicyName is a 'getter' method
func (o *ExportRuleModifyRequest) PolicyName() ExportPolicyNameType {
	r := *o.PolicyNamePtr
	return r
}

// SetPolicyName is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetPolicyName(newValue ExportPolicyNameType) *ExportRuleModifyRequest {
	o.PolicyNamePtr = &newValue
	return o
}

// ExportRuleModifyRequestProtocol is a wrapper
type ExportRuleModifyRequestProtocol struct {
	XMLName           xml.Name             `xml:"protocol"`
	AccessProtocolPtr []AccessProtocolType `xml:"access-protocol"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleModifyRequestProtocol) String() string {
	return ToString(reflect.ValueOf(o))
}

// AccessProtocol is a 'getter' method
func (o *ExportRuleModifyRequestProtocol) AccessProtocol() []AccessProtocolType {
	r := o.AccessProtocolPtr
	return r
}

// SetAccessProtocol is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequestProtocol) SetAccessProtocol(newValue []AccessProtocolType) *ExportRuleModifyRequestProtocol {
	newSlice := make([]AccessProtocolType, len(newValue))
	copy(newSlice, newValue)
	o.AccessProtocolPtr = newSlice
	return o
}

// Protocol is a 'getter' method
func (o *ExportRuleModifyRequest) Protocol() ExportRuleModifyRequestProtocol {
	r := *o.ProtocolPtr
	return r
}

// SetProtocol is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetProtocol(newValue ExportRuleModifyRequestProtocol) *ExportRuleModifyRequest {
	o.ProtocolPtr = &newValue
	return o
}

// ExportRuleModifyRequestRoRule is a wrapper
type ExportRuleModifyRequestRoRule struct {
	XMLName           xml.Name             `xml:"ro-rule"`
	SecurityFlavorPtr []SecurityFlavorType `xml:"security-flavor"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleModifyRequestRoRule) String() string {
	return ToString(reflect.ValueOf(o))
}

// SecurityFlavor is a 'getter' method
func (o *ExportRuleModifyRequestRoRule) SecurityFlavor() []SecurityFlavorType {
	r := o.SecurityFlavorPtr
	return r
}

// SetSecurityFlavor is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequestRoRule) SetSecurityFlavor(newValue []SecurityFlavorType) *ExportRuleModifyRequestRoRule {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.SecurityFlavorPtr = newSlice
	return o
}

// RoRule is a 'getter' method
func (o *ExportRuleModifyRequest) RoRule() ExportRuleModifyRequestRoRule {
	r := *o.RoRulePtr
	return r
}

// SetRoRule is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetRoRule(newValue ExportRuleModifyRequestRoRule) *ExportRuleModifyRequest {
	o.RoRulePtr = &newValue
	return o
}

// RuleIndex is a 'getter' method
func (o *ExportRuleModifyRequest) RuleIndex() int {
	r := *o.RuleIndexPtr
	return r
}

// SetRuleIndex is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetRuleIndex(newValue int) *ExportRuleModifyRequest {
	o.RuleIndexPtr = &newValue
	return o
}

// ExportRuleModifyRequestRwRule is a wrapper
type ExportRuleModifyRequestRwRule struct {
	XMLName           xml.Name             `xml:"rw-rule"`
	SecurityFlavorPtr []SecurityFlavorType `xml:"security-flavor"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleModifyRequestRwRule) String() string {
	return ToString(reflect.ValueOf(o))
}

// SecurityFlavor is a 'getter' method
func (o *ExportRuleModifyRequestRwRule) SecurityFlavor() []SecurityFlavorType {
	r := o.SecurityFlavorPtr
	return r
}

// SetSecurityFlavor is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequestRwRule) SetSecurityFlavor(newValue []SecurityFlavorType) *ExportRuleModifyRequestRwRule {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.SecurityFlavorPtr = newSlice
	return o
}

// RwRule is a 'getter' method
func (o *ExportRuleModifyRequest) RwRule() ExportRuleModifyRequestRwRule {
	r := *o.RwRulePtr
	return r
}

// SetRwRule is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetRwRule(newValue ExportRuleModifyRequestRwRule) *ExportRuleModifyRequest {
	o.RwRulePtr = &newValue
	return o
}

// ExportRuleModifyRequestSuperUserSecurity is a wrapper
type ExportRuleModifyRequestSuperUserSecurity struct {
	XMLName           xml.Name             `xml:"super-user-security"`
	SecurityFlavorPtr []SecurityFlavorType `xml:"security-flavor"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleModifyRequestSuperUserSecurity) String() string {
	return ToString(reflect.ValueOf(o))
}

// SecurityFlavor is a 'getter' method
func (o *ExportRuleModifyRequestSuperUserSecurity) SecurityFlavor() []SecurityFlavorType {
	r := o.SecurityFlavorPtr
	return r
}

// SetSecurityFlavor is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequestSuperUserSecurity) SetSecurityFlavor(newValue []SecurityFlavorType) *ExportRuleModifyRequestSuperUserSecurity {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.SecurityFlavorPtr = newSlice
	return o
}

// SuperUserSecurity is a 'getter' method
func (o *ExportRuleModifyRequest) SuperUserSecurity() ExportRuleModifyRequestSuperUserSecurity {
	r := *o.SuperUserSecurityPtr
	return r
}

// SetSuperUserSecurity is a fluent style 'setter' method that can be chained
func (o *ExportRuleModifyRequest) SetSuperUserSecurity(newValue ExportRuleModifyRequestSuperUserSecurity) *ExportRuleModifyRequest {
	o.SuperUserSecurityPtr = &newValue
	return o
}
