// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// VserverInfoType is a structure to represent a vserver-info ZAPI object
type VserverInfoType struct {
	XMLName     xml.Name                 `xml:"vserver-info"`
	AggrListPtr *VserverInfoTypeAggrList `xml:"aggr-list"`
	// work in progress
	AllowedProtocolsPtr *VserverInfoTypeAllowedProtocols `xml:"allowed-protocols"`
	// work in progress
	AntivirusOnAccessPolicyPtr *AntivirusPolicyType                `xml:"antivirus-on-access-policy"`
	CachingPolicyPtr           *string                             `xml:"caching-policy"`
	CommentPtr                 *string                             `xml:"comment"`
	DisallowedProtocolsPtr     *VserverInfoTypeDisallowedProtocols `xml:"disallowed-protocols"`
	// work in progress
	IpspacePtr                   *string                           `xml:"ipspace"`
	IsConfigLockedForChangesPtr  *bool                             `xml:"is-config-locked-for-changes"`
	IsRepositoryVserverPtr       *bool                             `xml:"is-repository-vserver"`
	IsSpaceEnforcementLogicalPtr *bool                             `xml:"is-space-enforcement-logical"`
	IsSpaceReportingLogicalPtr   *bool                             `xml:"is-space-reporting-logical"`
	IsVserverProtectedPtr        *bool                             `xml:"is-vserver-protected"`
	LanguagePtr                  *LanguageCodeType                 `xml:"language"`
	LdapDomainPtr                *string                           `xml:"ldap-domain"`
	MaxVolumesPtr                *string                           `xml:"max-volumes"`
	NameMappingSwitchPtr         *VserverInfoTypeNameMappingSwitch `xml:"name-mapping-switch"`
	// work in progress
	NameServerSwitchPtr *VserverInfoTypeNameServerSwitch `xml:"name-server-switch"`
	// work in progress
	NisDomainPtr                     *NisDomainType                      `xml:"nis-domain"`
	OperationalStatePtr              *VsoperstateType                    `xml:"operational-state"`
	OperationalStateStoppedReasonPtr *VsopstopreasonType                 `xml:"operational-state-stopped-reason"`
	QosPolicyGroupPtr                *string                             `xml:"qos-policy-group"`
	QuotaPolicyPtr                   *string                             `xml:"quota-policy"`
	RootVolumePtr                    *VolumeNameType                     `xml:"root-volume"`
	RootVolumeAggregatePtr           *AggrNameType                       `xml:"root-volume-aggregate"`
	RootVolumeSecurityStylePtr       *SecurityStyleEnumType              `xml:"root-volume-security-style"`
	SnapshotPolicyPtr                *SnapshotPolicyType                 `xml:"snapshot-policy"`
	StatePtr                         *VsadminstateType                   `xml:"state"`
	UuidPtr                          *UuidType                           `xml:"uuid"`
	VolumeDeleteRetentionHoursPtr    *int                                `xml:"volume-delete-retention-hours"`
	VserverAggrInfoListPtr           *VserverInfoTypeVserverAggrInfoList `xml:"vserver-aggr-info-list"`
	// work in progress
	VserverNamePtr    *string `xml:"vserver-name"`
	VserverSubtypePtr *string `xml:"vserver-subtype"`
	VserverTypePtr    *string `xml:"vserver-type"`
}

// NewVserverInfoType is a factory method for creating new instances of VserverInfoType objects
func NewVserverInfoType() *VserverInfoType {
	return &VserverInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *VserverInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o VserverInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// VserverInfoTypeAggrList is a wrapper
type VserverInfoTypeAggrList struct {
	XMLName     xml.Name       `xml:"aggr-list"`
	AggrNamePtr []AggrNameType `xml:"aggr-name"`
}

// AggrName is a 'getter' method
func (o *VserverInfoTypeAggrList) AggrName() []AggrNameType {
	r := o.AggrNamePtr
	return r
}

// SetAggrName is a fluent style 'setter' method that can be chained
func (o *VserverInfoTypeAggrList) SetAggrName(newValue []AggrNameType) *VserverInfoTypeAggrList {
	newSlice := make([]AggrNameType, len(newValue))
	copy(newSlice, newValue)
	o.AggrNamePtr = newSlice
	return o
}

// AggrList is a 'getter' method
func (o *VserverInfoType) AggrList() VserverInfoTypeAggrList {
	var r VserverInfoTypeAggrList
	if o.AggrListPtr == nil {
		return r
	}
	r = *o.AggrListPtr
	return r
}

// SetAggrList is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetAggrList(newValue VserverInfoTypeAggrList) *VserverInfoType {
	o.AggrListPtr = &newValue
	return o
}

// VserverInfoTypeAllowedProtocols is a wrapper
type VserverInfoTypeAllowedProtocols struct {
	XMLName     xml.Name       `xml:"allowed-protocols"`
	ProtocolPtr []ProtocolType `xml:"protocol"`
}

// Protocol is a 'getter' method
func (o *VserverInfoTypeAllowedProtocols) Protocol() []ProtocolType {
	r := o.ProtocolPtr
	return r
}

// SetProtocol is a fluent style 'setter' method that can be chained
func (o *VserverInfoTypeAllowedProtocols) SetProtocol(newValue []ProtocolType) *VserverInfoTypeAllowedProtocols {
	newSlice := make([]ProtocolType, len(newValue))
	copy(newSlice, newValue)
	o.ProtocolPtr = newSlice
	return o
}

// AllowedProtocols is a 'getter' method
func (o *VserverInfoType) AllowedProtocols() VserverInfoTypeAllowedProtocols {
	var r VserverInfoTypeAllowedProtocols
	if o.AllowedProtocolsPtr == nil {
		return r
	}
	r = *o.AllowedProtocolsPtr
	return r
}

// SetAllowedProtocols is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetAllowedProtocols(newValue VserverInfoTypeAllowedProtocols) *VserverInfoType {
	o.AllowedProtocolsPtr = &newValue
	return o
}

// AntivirusOnAccessPolicy is a 'getter' method
func (o *VserverInfoType) AntivirusOnAccessPolicy() AntivirusPolicyType {
	var r AntivirusPolicyType
	if o.AntivirusOnAccessPolicyPtr == nil {
		return r
	}
	r = *o.AntivirusOnAccessPolicyPtr
	return r
}

// SetAntivirusOnAccessPolicy is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetAntivirusOnAccessPolicy(newValue AntivirusPolicyType) *VserverInfoType {
	o.AntivirusOnAccessPolicyPtr = &newValue
	return o
}

// CachingPolicy is a 'getter' method
func (o *VserverInfoType) CachingPolicy() string {
	var r string
	if o.CachingPolicyPtr == nil {
		return r
	}
	r = *o.CachingPolicyPtr
	return r
}

// SetCachingPolicy is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetCachingPolicy(newValue string) *VserverInfoType {
	o.CachingPolicyPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *VserverInfoType) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetComment(newValue string) *VserverInfoType {
	o.CommentPtr = &newValue
	return o
}

// VserverInfoTypeDisallowedProtocols is a wrapper
type VserverInfoTypeDisallowedProtocols struct {
	XMLName     xml.Name       `xml:"disallowed-protocols"`
	ProtocolPtr []ProtocolType `xml:"protocol"`
}

// Protocol is a 'getter' method
func (o *VserverInfoTypeDisallowedProtocols) Protocol() []ProtocolType {
	r := o.ProtocolPtr
	return r
}

// SetProtocol is a fluent style 'setter' method that can be chained
func (o *VserverInfoTypeDisallowedProtocols) SetProtocol(newValue []ProtocolType) *VserverInfoTypeDisallowedProtocols {
	newSlice := make([]ProtocolType, len(newValue))
	copy(newSlice, newValue)
	o.ProtocolPtr = newSlice
	return o
}

// DisallowedProtocols is a 'getter' method
func (o *VserverInfoType) DisallowedProtocols() VserverInfoTypeDisallowedProtocols {
	var r VserverInfoTypeDisallowedProtocols
	if o.DisallowedProtocolsPtr == nil {
		return r
	}
	r = *o.DisallowedProtocolsPtr
	return r
}

// SetDisallowedProtocols is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetDisallowedProtocols(newValue VserverInfoTypeDisallowedProtocols) *VserverInfoType {
	o.DisallowedProtocolsPtr = &newValue
	return o
}

// Ipspace is a 'getter' method
func (o *VserverInfoType) Ipspace() string {
	var r string
	if o.IpspacePtr == nil {
		return r
	}
	r = *o.IpspacePtr
	return r
}

// SetIpspace is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetIpspace(newValue string) *VserverInfoType {
	o.IpspacePtr = &newValue
	return o
}

// IsConfigLockedForChanges is a 'getter' method
func (o *VserverInfoType) IsConfigLockedForChanges() bool {
	var r bool
	if o.IsConfigLockedForChangesPtr == nil {
		return r
	}
	r = *o.IsConfigLockedForChangesPtr
	return r
}

// SetIsConfigLockedForChanges is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetIsConfigLockedForChanges(newValue bool) *VserverInfoType {
	o.IsConfigLockedForChangesPtr = &newValue
	return o
}

// IsRepositoryVserver is a 'getter' method
func (o *VserverInfoType) IsRepositoryVserver() bool {
	var r bool
	if o.IsRepositoryVserverPtr == nil {
		return r
	}
	r = *o.IsRepositoryVserverPtr
	return r
}

// SetIsRepositoryVserver is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetIsRepositoryVserver(newValue bool) *VserverInfoType {
	o.IsRepositoryVserverPtr = &newValue
	return o
}

// IsSpaceEnforcementLogical is a 'getter' method
func (o *VserverInfoType) IsSpaceEnforcementLogical() bool {
	var r bool
	if o.IsSpaceEnforcementLogicalPtr == nil {
		return r
	}
	r = *o.IsSpaceEnforcementLogicalPtr
	return r
}

// SetIsSpaceEnforcementLogical is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetIsSpaceEnforcementLogical(newValue bool) *VserverInfoType {
	o.IsSpaceEnforcementLogicalPtr = &newValue
	return o
}

// IsSpaceReportingLogical is a 'getter' method
func (o *VserverInfoType) IsSpaceReportingLogical() bool {
	var r bool
	if o.IsSpaceReportingLogicalPtr == nil {
		return r
	}
	r = *o.IsSpaceReportingLogicalPtr
	return r
}

// SetIsSpaceReportingLogical is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetIsSpaceReportingLogical(newValue bool) *VserverInfoType {
	o.IsSpaceReportingLogicalPtr = &newValue
	return o
}

// IsVserverProtected is a 'getter' method
func (o *VserverInfoType) IsVserverProtected() bool {
	var r bool
	if o.IsVserverProtectedPtr == nil {
		return r
	}
	r = *o.IsVserverProtectedPtr
	return r
}

// SetIsVserverProtected is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetIsVserverProtected(newValue bool) *VserverInfoType {
	o.IsVserverProtectedPtr = &newValue
	return o
}

// Language is a 'getter' method
func (o *VserverInfoType) Language() LanguageCodeType {
	var r LanguageCodeType
	if o.LanguagePtr == nil {
		return r
	}
	r = *o.LanguagePtr
	return r
}

// SetLanguage is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetLanguage(newValue LanguageCodeType) *VserverInfoType {
	o.LanguagePtr = &newValue
	return o
}

// LdapDomain is a 'getter' method
func (o *VserverInfoType) LdapDomain() string {
	var r string
	if o.LdapDomainPtr == nil {
		return r
	}
	r = *o.LdapDomainPtr
	return r
}

// SetLdapDomain is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetLdapDomain(newValue string) *VserverInfoType {
	o.LdapDomainPtr = &newValue
	return o
}

// MaxVolumes is a 'getter' method
func (o *VserverInfoType) MaxVolumes() string {
	var r string
	if o.MaxVolumesPtr == nil {
		return r
	}
	r = *o.MaxVolumesPtr
	return r
}

// SetMaxVolumes is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetMaxVolumes(newValue string) *VserverInfoType {
	o.MaxVolumesPtr = &newValue
	return o
}

// VserverInfoTypeNameMappingSwitch is a wrapper
type VserverInfoTypeNameMappingSwitch struct {
	XMLName     xml.Name       `xml:"name-mapping-switch"`
	NmswitchPtr []NmswitchType `xml:"nmswitch"`
}

// Nmswitch is a 'getter' method
func (o *VserverInfoTypeNameMappingSwitch) Nmswitch() []NmswitchType {
	r := o.NmswitchPtr
	return r
}

// SetNmswitch is a fluent style 'setter' method that can be chained
func (o *VserverInfoTypeNameMappingSwitch) SetNmswitch(newValue []NmswitchType) *VserverInfoTypeNameMappingSwitch {
	newSlice := make([]NmswitchType, len(newValue))
	copy(newSlice, newValue)
	o.NmswitchPtr = newSlice
	return o
}

// NameMappingSwitch is a 'getter' method
func (o *VserverInfoType) NameMappingSwitch() VserverInfoTypeNameMappingSwitch {
	var r VserverInfoTypeNameMappingSwitch
	if o.NameMappingSwitchPtr == nil {
		return r
	}
	r = *o.NameMappingSwitchPtr
	return r
}

// SetNameMappingSwitch is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetNameMappingSwitch(newValue VserverInfoTypeNameMappingSwitch) *VserverInfoType {
	o.NameMappingSwitchPtr = &newValue
	return o
}

// VserverInfoTypeNameServerSwitch is a wrapper
type VserverInfoTypeNameServerSwitch struct {
	XMLName     xml.Name       `xml:"name-server-switch"`
	NsswitchPtr []NsswitchType `xml:"nsswitch"`
}

// Nsswitch is a 'getter' method
func (o *VserverInfoTypeNameServerSwitch) Nsswitch() []NsswitchType {
	r := o.NsswitchPtr
	return r
}

// SetNsswitch is a fluent style 'setter' method that can be chained
func (o *VserverInfoTypeNameServerSwitch) SetNsswitch(newValue []NsswitchType) *VserverInfoTypeNameServerSwitch {
	newSlice := make([]NsswitchType, len(newValue))
	copy(newSlice, newValue)
	o.NsswitchPtr = newSlice
	return o
}

// NameServerSwitch is a 'getter' method
func (o *VserverInfoType) NameServerSwitch() VserverInfoTypeNameServerSwitch {
	var r VserverInfoTypeNameServerSwitch
	if o.NameServerSwitchPtr == nil {
		return r
	}
	r = *o.NameServerSwitchPtr
	return r
}

// SetNameServerSwitch is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetNameServerSwitch(newValue VserverInfoTypeNameServerSwitch) *VserverInfoType {
	o.NameServerSwitchPtr = &newValue
	return o
}

// NisDomain is a 'getter' method
func (o *VserverInfoType) NisDomain() NisDomainType {
	var r NisDomainType
	if o.NisDomainPtr == nil {
		return r
	}
	r = *o.NisDomainPtr
	return r
}

// SetNisDomain is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetNisDomain(newValue NisDomainType) *VserverInfoType {
	o.NisDomainPtr = &newValue
	return o
}

// OperationalState is a 'getter' method
func (o *VserverInfoType) OperationalState() VsoperstateType {
	var r VsoperstateType
	if o.OperationalStatePtr == nil {
		return r
	}
	r = *o.OperationalStatePtr
	return r
}

// SetOperationalState is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetOperationalState(newValue VsoperstateType) *VserverInfoType {
	o.OperationalStatePtr = &newValue
	return o
}

// OperationalStateStoppedReason is a 'getter' method
func (o *VserverInfoType) OperationalStateStoppedReason() VsopstopreasonType {
	var r VsopstopreasonType
	if o.OperationalStateStoppedReasonPtr == nil {
		return r
	}
	r = *o.OperationalStateStoppedReasonPtr
	return r
}

// SetOperationalStateStoppedReason is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetOperationalStateStoppedReason(newValue VsopstopreasonType) *VserverInfoType {
	o.OperationalStateStoppedReasonPtr = &newValue
	return o
}

// QosPolicyGroup is a 'getter' method
func (o *VserverInfoType) QosPolicyGroup() string {
	var r string
	if o.QosPolicyGroupPtr == nil {
		return r
	}
	r = *o.QosPolicyGroupPtr
	return r
}

// SetQosPolicyGroup is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetQosPolicyGroup(newValue string) *VserverInfoType {
	o.QosPolicyGroupPtr = &newValue
	return o
}

// QuotaPolicy is a 'getter' method
func (o *VserverInfoType) QuotaPolicy() string {
	var r string
	if o.QuotaPolicyPtr == nil {
		return r
	}
	r = *o.QuotaPolicyPtr
	return r
}

// SetQuotaPolicy is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetQuotaPolicy(newValue string) *VserverInfoType {
	o.QuotaPolicyPtr = &newValue
	return o
}

// RootVolume is a 'getter' method
func (o *VserverInfoType) RootVolume() VolumeNameType {
	var r VolumeNameType
	if o.RootVolumePtr == nil {
		return r
	}
	r = *o.RootVolumePtr
	return r
}

// SetRootVolume is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetRootVolume(newValue VolumeNameType) *VserverInfoType {
	o.RootVolumePtr = &newValue
	return o
}

// RootVolumeAggregate is a 'getter' method
func (o *VserverInfoType) RootVolumeAggregate() AggrNameType {
	var r AggrNameType
	if o.RootVolumeAggregatePtr == nil {
		return r
	}
	r = *o.RootVolumeAggregatePtr
	return r
}

// SetRootVolumeAggregate is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetRootVolumeAggregate(newValue AggrNameType) *VserverInfoType {
	o.RootVolumeAggregatePtr = &newValue
	return o
}

// RootVolumeSecurityStyle is a 'getter' method
func (o *VserverInfoType) RootVolumeSecurityStyle() SecurityStyleEnumType {
	var r SecurityStyleEnumType
	if o.RootVolumeSecurityStylePtr == nil {
		return r
	}
	r = *o.RootVolumeSecurityStylePtr
	return r
}

// SetRootVolumeSecurityStyle is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetRootVolumeSecurityStyle(newValue SecurityStyleEnumType) *VserverInfoType {
	o.RootVolumeSecurityStylePtr = &newValue
	return o
}

// SnapshotPolicy is a 'getter' method
func (o *VserverInfoType) SnapshotPolicy() SnapshotPolicyType {
	var r SnapshotPolicyType
	if o.SnapshotPolicyPtr == nil {
		return r
	}
	r = *o.SnapshotPolicyPtr
	return r
}

// SetSnapshotPolicy is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetSnapshotPolicy(newValue SnapshotPolicyType) *VserverInfoType {
	o.SnapshotPolicyPtr = &newValue
	return o
}

// State is a 'getter' method
func (o *VserverInfoType) State() VsadminstateType {
	var r VsadminstateType
	if o.StatePtr == nil {
		return r
	}
	r = *o.StatePtr
	return r
}

// SetState is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetState(newValue VsadminstateType) *VserverInfoType {
	o.StatePtr = &newValue
	return o
}

// Uuid is a 'getter' method
func (o *VserverInfoType) Uuid() UuidType {
	var r UuidType
	if o.UuidPtr == nil {
		return r
	}
	r = *o.UuidPtr
	return r
}

// SetUuid is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetUuid(newValue UuidType) *VserverInfoType {
	o.UuidPtr = &newValue
	return o
}

// VolumeDeleteRetentionHours is a 'getter' method
func (o *VserverInfoType) VolumeDeleteRetentionHours() int {
	var r int
	if o.VolumeDeleteRetentionHoursPtr == nil {
		return r
	}
	r = *o.VolumeDeleteRetentionHoursPtr
	return r
}

// SetVolumeDeleteRetentionHours is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetVolumeDeleteRetentionHours(newValue int) *VserverInfoType {
	o.VolumeDeleteRetentionHoursPtr = &newValue
	return o
}

// VserverInfoTypeVserverAggrInfoList is a wrapper
type VserverInfoTypeVserverAggrInfoList struct {
	XMLName            xml.Name              `xml:"vserver-aggr-info-list"`
	VserverAggrInfoPtr []VserverAggrInfoType `xml:"vserver-aggr-info"`
}

// VserverAggrInfo is a 'getter' method
func (o *VserverInfoTypeVserverAggrInfoList) VserverAggrInfo() []VserverAggrInfoType {
	r := o.VserverAggrInfoPtr
	return r
}

// SetVserverAggrInfo is a fluent style 'setter' method that can be chained
func (o *VserverInfoTypeVserverAggrInfoList) SetVserverAggrInfo(newValue []VserverAggrInfoType) *VserverInfoTypeVserverAggrInfoList {
	newSlice := make([]VserverAggrInfoType, len(newValue))
	copy(newSlice, newValue)
	o.VserverAggrInfoPtr = newSlice
	return o
}

// VserverAggrInfoList is a 'getter' method
func (o *VserverInfoType) VserverAggrInfoList() VserverInfoTypeVserverAggrInfoList {
	var r VserverInfoTypeVserverAggrInfoList
	if o.VserverAggrInfoListPtr == nil {
		return r
	}
	r = *o.VserverAggrInfoListPtr
	return r
}

// SetVserverAggrInfoList is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetVserverAggrInfoList(newValue VserverInfoTypeVserverAggrInfoList) *VserverInfoType {
	o.VserverAggrInfoListPtr = &newValue
	return o
}

// VserverName is a 'getter' method
func (o *VserverInfoType) VserverName() string {
	var r string
	if o.VserverNamePtr == nil {
		return r
	}
	r = *o.VserverNamePtr
	return r
}

// SetVserverName is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetVserverName(newValue string) *VserverInfoType {
	o.VserverNamePtr = &newValue
	return o
}

// VserverSubtype is a 'getter' method
func (o *VserverInfoType) VserverSubtype() string {
	var r string
	if o.VserverSubtypePtr == nil {
		return r
	}
	r = *o.VserverSubtypePtr
	return r
}

// SetVserverSubtype is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetVserverSubtype(newValue string) *VserverInfoType {
	o.VserverSubtypePtr = &newValue
	return o
}

// VserverType is a 'getter' method
func (o *VserverInfoType) VserverType() string {
	var r string
	if o.VserverTypePtr == nil {
		return r
	}
	r = *o.VserverTypePtr
	return r
}

// SetVserverType is a fluent style 'setter' method that can be chained
func (o *VserverInfoType) SetVserverType(newValue string) *VserverInfoType {
	o.VserverTypePtr = &newValue
	return o
}
