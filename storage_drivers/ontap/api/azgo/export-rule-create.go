// Copyright 2017 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"

	log "github.com/Sirupsen/logrus"
)

// ExportRuleCreateRequest is a structure to represent a export-rule-create ZAPI request object
type ExportRuleCreateRequest struct {
	XMLName xml.Name `xml:"export-rule-create"`

	AnonymousUserIdPtr           *string                   `xml:"anonymous-user-id"`
	ClientMatchPtr               *string                   `xml:"client-match"`
	ExportChownModePtr           *ExportchownmodeType      `xml:"export-chown-mode>exportchownmode"`
	ExportNtfsUnixSecurityOpsPtr *ExportntfsunixsecopsType `xml:"export-ntfs-unix-security-ops>exportntfsunixsecops"`
	IsAllowDevIsEnabledPtr       *bool                     `xml:"is-allow-dev-is-enabled"`
	IsAllowSetUidEnabledPtr      *bool                     `xml:"is-allow-set-uid-enabled"`
	PolicyNamePtr                *ExportPolicyNameType     `xml:"policy-name"`
	ProtocolPtr                  []AccessProtocolType      `xml:"protocol>access-protocol"`
	RoRulePtr                    []SecurityFlavorType      `xml:"ro-rule>security-flavor"`
	RuleIndexPtr                 *int                      `xml:"rule-index"`
	RwRulePtr                    []SecurityFlavorType      `xml:"rw-rule>security-flavor"`
	SuperUserSecurityPtr         []SecurityFlavorType      `xml:"super-user-security>security-flavor"`
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleCreateRequest) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Errorf("error: %v\n", err) }
	return string(output), err
}

// NewExportRuleCreateRequest is a factory method for creating new instances of ExportRuleCreateRequest objects
func NewExportRuleCreateRequest() *ExportRuleCreateRequest { return &ExportRuleCreateRequest{} }

// ExecuteUsing converts this object to a ZAPI XML representation and uses the supplied ZapiRunner to send to a filer
func (o *ExportRuleCreateRequest) ExecuteUsing(zr *ZapiRunner) (ExportRuleCreateResponse, error) {

	if zr.DebugTraceFlags["method"] {
		fields := log.Fields{"Method": "ExecuteUsing", "Type": "ExportRuleCreateRequest"}
		log.WithFields(fields).Debug(">>>> ExecuteUsing")
		defer log.WithFields(fields).Debug("<<<< ExecuteUsing")
	}

	resp, err := zr.SendZapi(o)
	if err != nil {
		log.Errorf("API invocation failed. %v", err.Error())
		return ExportRuleCreateResponse{}, err
	}
	defer resp.Body.Close()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		log.Errorf("Error reading response body. %v", readErr.Error())
		return ExportRuleCreateResponse{}, readErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("response Body:\n%s", string(body))
	}

	var n ExportRuleCreateResponse
	unmarshalErr := xml.Unmarshal(body, &n)
	if unmarshalErr != nil {
		log.WithField("body", string(body)).Warnf("Error unmarshaling response body. %v", unmarshalErr.Error())
		//return ExportRuleCreateResponse{}, unmarshalErr
	}
	if zr.DebugTraceFlags["api"] {
		log.Debugf("export-rule-create result:\n%s", n.Result)
	}

	return n, nil
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleCreateRequest) String() string {
	var buffer bytes.Buffer
	if o.AnonymousUserIdPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "anonymous-user-id", *o.AnonymousUserIdPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("anonymous-user-id: nil\n"))
	}
	if o.ClientMatchPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "client-match", *o.ClientMatchPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("client-match: nil\n"))
	}
	if o.ExportChownModePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "export-chown-mode", *o.ExportChownModePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("export-chown-mode: nil\n"))
	}
	if o.ExportNtfsUnixSecurityOpsPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "export-ntfs-unix-security-ops", *o.ExportNtfsUnixSecurityOpsPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("export-ntfs-unix-security-ops: nil\n"))
	}
	if o.IsAllowDevIsEnabledPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "is-allow-dev-is-enabled", *o.IsAllowDevIsEnabledPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("is-allow-dev-is-enabled: nil\n"))
	}
	if o.IsAllowSetUidEnabledPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "is-allow-set-uid-enabled", *o.IsAllowSetUidEnabledPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("is-allow-set-uid-enabled: nil\n"))
	}
	if o.PolicyNamePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "policy-name", *o.PolicyNamePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("policy-name: nil\n"))
	}
	if o.ProtocolPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "protocol", o.ProtocolPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("protocol: nil\n"))
	}
	if o.RoRulePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "ro-rule", o.RoRulePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("ro-rule: nil\n"))
	}
	if o.RuleIndexPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "rule-index", *o.RuleIndexPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("rule-index: nil\n"))
	}
	if o.RwRulePtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "rw-rule", o.RwRulePtr))
	} else {
		buffer.WriteString(fmt.Sprintf("rw-rule: nil\n"))
	}
	if o.SuperUserSecurityPtr != nil {
		buffer.WriteString(fmt.Sprintf("%s: %v\n", "super-user-security", o.SuperUserSecurityPtr))
	} else {
		buffer.WriteString(fmt.Sprintf("super-user-security: nil\n"))
	}
	return buffer.String()
}

// AnonymousUserId is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) AnonymousUserId() string {
	r := *o.AnonymousUserIdPtr
	return r
}

// SetAnonymousUserId is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetAnonymousUserId(newValue string) *ExportRuleCreateRequest {
	o.AnonymousUserIdPtr = &newValue
	return o
}

// ClientMatch is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) ClientMatch() string {
	r := *o.ClientMatchPtr
	return r
}

// SetClientMatch is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetClientMatch(newValue string) *ExportRuleCreateRequest {
	o.ClientMatchPtr = &newValue
	return o
}

// ExportChownMode is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) ExportChownMode() ExportchownmodeType {
	r := *o.ExportChownModePtr
	return r
}

// SetExportChownMode is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetExportChownMode(newValue ExportchownmodeType) *ExportRuleCreateRequest {
	o.ExportChownModePtr = &newValue
	return o
}

// ExportNtfsUnixSecurityOps is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) ExportNtfsUnixSecurityOps() ExportntfsunixsecopsType {
	r := *o.ExportNtfsUnixSecurityOpsPtr
	return r
}

// SetExportNtfsUnixSecurityOps is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetExportNtfsUnixSecurityOps(newValue ExportntfsunixsecopsType) *ExportRuleCreateRequest {
	o.ExportNtfsUnixSecurityOpsPtr = &newValue
	return o
}

// IsAllowDevIsEnabled is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) IsAllowDevIsEnabled() bool {
	r := *o.IsAllowDevIsEnabledPtr
	return r
}

// SetIsAllowDevIsEnabled is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetIsAllowDevIsEnabled(newValue bool) *ExportRuleCreateRequest {
	o.IsAllowDevIsEnabledPtr = &newValue
	return o
}

// IsAllowSetUidEnabled is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) IsAllowSetUidEnabled() bool {
	r := *o.IsAllowSetUidEnabledPtr
	return r
}

// SetIsAllowSetUidEnabled is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetIsAllowSetUidEnabled(newValue bool) *ExportRuleCreateRequest {
	o.IsAllowSetUidEnabledPtr = &newValue
	return o
}

// PolicyName is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) PolicyName() ExportPolicyNameType {
	r := *o.PolicyNamePtr
	return r
}

// SetPolicyName is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetPolicyName(newValue ExportPolicyNameType) *ExportRuleCreateRequest {
	o.PolicyNamePtr = &newValue
	return o
}

// Protocol is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) Protocol() []AccessProtocolType {
	r := o.ProtocolPtr
	return r
}

// SetProtocol is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetProtocol(newValue []AccessProtocolType) *ExportRuleCreateRequest {
	newSlice := make([]AccessProtocolType, len(newValue))
	copy(newSlice, newValue)
	o.ProtocolPtr = newSlice
	return o
}

// RoRule is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) RoRule() []SecurityFlavorType {
	r := o.RoRulePtr
	return r
}

// SetRoRule is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetRoRule(newValue []SecurityFlavorType) *ExportRuleCreateRequest {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.RoRulePtr = newSlice
	return o
}

// RuleIndex is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) RuleIndex() int {
	r := *o.RuleIndexPtr
	return r
}

// SetRuleIndex is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetRuleIndex(newValue int) *ExportRuleCreateRequest {
	o.RuleIndexPtr = &newValue
	return o
}

// RwRule is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) RwRule() []SecurityFlavorType {
	r := o.RwRulePtr
	return r
}

// SetRwRule is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetRwRule(newValue []SecurityFlavorType) *ExportRuleCreateRequest {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.RwRulePtr = newSlice
	return o
}

// SuperUserSecurity is a fluent style 'getter' method that can be chained
func (o *ExportRuleCreateRequest) SuperUserSecurity() []SecurityFlavorType {
	r := o.SuperUserSecurityPtr
	return r
}

// SetSuperUserSecurity is a fluent style 'setter' method that can be chained
func (o *ExportRuleCreateRequest) SetSuperUserSecurity(newValue []SecurityFlavorType) *ExportRuleCreateRequest {
	newSlice := make([]SecurityFlavorType, len(newValue))
	copy(newSlice, newValue)
	o.SuperUserSecurityPtr = newSlice
	return o
}

// ExportRuleCreateResponse is a structure to represent a export-rule-create ZAPI response object
type ExportRuleCreateResponse struct {
	XMLName xml.Name `xml:"netapp"`

	ResponseVersion string `xml:"version,attr"`
	ResponseXmlns   string `xml:"xmlns,attr"`

	Result ExportRuleCreateResponseResult `xml:"results"`
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleCreateResponse) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "version", o.ResponseVersion))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "xmlns", o.ResponseXmlns))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "results", o.Result))
	return buffer.String()
}

// ExportRuleCreateResponseResult is a structure to represent a export-rule-create ZAPI object's result
type ExportRuleCreateResponseResult struct {
	XMLName xml.Name `xml:"results"`

	ResultStatusAttr string `xml:"status,attr"`
	ResultReasonAttr string `xml:"reason,attr"`
	ResultErrnoAttr  string `xml:"errno,attr"`
}

// ToXML converts this object into an xml string representation
func (o *ExportRuleCreateResponse) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	//if err != nil { log.Debugf("error: %v", err) }
	return string(output), err
}

// NewExportRuleCreateResponse is a factory method for creating new instances of ExportRuleCreateResponse objects
func NewExportRuleCreateResponse() *ExportRuleCreateResponse { return &ExportRuleCreateResponse{} }

// String returns a string representation of this object's fields and implements the Stringer interface
func (o ExportRuleCreateResponseResult) String() string {
	var buffer bytes.Buffer
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultStatusAttr", o.ResultStatusAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultReasonAttr", o.ResultReasonAttr))
	buffer.WriteString(fmt.Sprintf("%s: %s\n", "resultErrnoAttr", o.ResultErrnoAttr))
	return buffer.String()
}
