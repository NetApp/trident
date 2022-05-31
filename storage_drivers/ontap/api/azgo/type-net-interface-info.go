// Code generated automatically. DO NOT EDIT.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"encoding/xml"
	log "github.com/sirupsen/logrus"
	"reflect"
)

// NetInterfaceInfoType is a structure to represent a net-interface-info ZAPI object
type NetInterfaceInfoType struct {
	XMLName                 xml.Name                           `xml:"net-interface-info"`
	AddressPtr              *IpAddressType                     `xml:"address"`
	AddressFamilyPtr        *string                            `xml:"address-family"`
	AdministrativeStatusPtr *string                            `xml:"administrative-status"`
	CommentPtr              *string                            `xml:"comment"`
	CurrentNodePtr          *string                            `xml:"current-node"`
	CurrentPortPtr          *string                            `xml:"current-port"`
	DataProtocolsPtr        *NetInterfaceInfoTypeDataProtocols `xml:"data-protocols"`
	// work in progress
	DnsDomainNamePtr          *DnsZoneType                      `xml:"dns-domain-name"`
	ExtendedStatusPtr         *string                           `xml:"extended-status"`
	FailoverGroupPtr          *FailoverGroupType                `xml:"failover-group"`
	FailoverPolicyPtr         *string                           `xml:"failover-policy"`
	FirewallPolicyPtr         *string                           `xml:"firewall-policy"`
	ForceSubnetAssociationPtr *bool                             `xml:"force-subnet-association"`
	HomeNodePtr               *string                           `xml:"home-node"`
	HomePortPtr               *string                           `xml:"home-port"`
	InterfaceNamePtr          *string                           `xml:"interface-name"`
	IpspacePtr                *string                           `xml:"ipspace"`
	IsAutoRevertPtr           *bool                             `xml:"is-auto-revert"`
	IsDnsUpdateEnabledPtr     *bool                             `xml:"is-dns-update-enabled"`
	IsHomePtr                 *bool                             `xml:"is-home"`
	IsIpv4LinkLocalPtr        *bool                             `xml:"is-ipv4-link-local"`
	IsVipPtr                  *bool                             `xml:"is-vip"`
	LifUuidPtr                *UuidType                         `xml:"lif-uuid"`
	ListenForDnsQueryPtr      *bool                             `xml:"listen-for-dns-query"`
	NetmaskPtr                *IpAddressType                    `xml:"netmask"`
	NetmaskLengthPtr          *int                              `xml:"netmask-length"`
	OperationalStatusPtr      *string                           `xml:"operational-status"`
	ProbePortPtr              *int                              `xml:"probe-port"`
	RolePtr                   *string                           `xml:"role"`
	RoutingGroupNamePtr       *string                           `xml:"routing-group-name"`
	ServiceNamesPtr           *NetInterfaceInfoTypeServiceNames `xml:"service-names"`
	// work in progress
	ServicePolicyPtr    *string         `xml:"service-policy"`
	SubnetNamePtr       *SubnetNameType `xml:"subnet-name"`
	UseFailoverGroupPtr *string         `xml:"use-failover-group"`
	VserverPtr          *string         `xml:"vserver"`
	WwpnPtr             *string         `xml:"wwpn"`
}

// NewNetInterfaceInfoType is a factory method for creating new instances of NetInterfaceInfoType objects
func NewNetInterfaceInfoType() *NetInterfaceInfoType {
	return &NetInterfaceInfoType{}
}

// ToXML converts this object into an xml string representation
func (o *NetInterfaceInfoType) ToXML() (string, error) {
	output, err := xml.MarshalIndent(o, " ", "    ")
	if err != nil {
		log.Errorf("error: %v", err)
	}
	return string(output), err
}

// String returns a string representation of this object's fields and implements the Stringer interface
func (o NetInterfaceInfoType) String() string {
	return ToString(reflect.ValueOf(o))
}

// Address is a 'getter' method
func (o *NetInterfaceInfoType) Address() IpAddressType {
	var r IpAddressType
	if o.AddressPtr == nil {
		return r
	}
	r = *o.AddressPtr
	return r
}

// SetAddress is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetAddress(newValue IpAddressType) *NetInterfaceInfoType {
	o.AddressPtr = &newValue
	return o
}

// AddressFamily is a 'getter' method
func (o *NetInterfaceInfoType) AddressFamily() string {
	var r string
	if o.AddressFamilyPtr == nil {
		return r
	}
	r = *o.AddressFamilyPtr
	return r
}

// SetAddressFamily is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetAddressFamily(newValue string) *NetInterfaceInfoType {
	o.AddressFamilyPtr = &newValue
	return o
}

// AdministrativeStatus is a 'getter' method
func (o *NetInterfaceInfoType) AdministrativeStatus() string {
	var r string
	if o.AdministrativeStatusPtr == nil {
		return r
	}
	r = *o.AdministrativeStatusPtr
	return r
}

// SetAdministrativeStatus is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetAdministrativeStatus(newValue string) *NetInterfaceInfoType {
	o.AdministrativeStatusPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *NetInterfaceInfoType) Comment() string {
	var r string
	if o.CommentPtr == nil {
		return r
	}
	r = *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetComment(newValue string) *NetInterfaceInfoType {
	o.CommentPtr = &newValue
	return o
}

// CurrentNode is a 'getter' method
func (o *NetInterfaceInfoType) CurrentNode() string {
	var r string
	if o.CurrentNodePtr == nil {
		return r
	}
	r = *o.CurrentNodePtr
	return r
}

// SetCurrentNode is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetCurrentNode(newValue string) *NetInterfaceInfoType {
	o.CurrentNodePtr = &newValue
	return o
}

// CurrentPort is a 'getter' method
func (o *NetInterfaceInfoType) CurrentPort() string {
	var r string
	if o.CurrentPortPtr == nil {
		return r
	}
	r = *o.CurrentPortPtr
	return r
}

// SetCurrentPort is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetCurrentPort(newValue string) *NetInterfaceInfoType {
	o.CurrentPortPtr = &newValue
	return o
}

// NetInterfaceInfoTypeDataProtocols is a wrapper
type NetInterfaceInfoTypeDataProtocols struct {
	XMLName         xml.Name           `xml:"data-protocols"`
	DataProtocolPtr []DataProtocolType `xml:"data-protocol"`
}

// DataProtocol is a 'getter' method
func (o *NetInterfaceInfoTypeDataProtocols) DataProtocol() []DataProtocolType {
	r := o.DataProtocolPtr
	return r
}

// SetDataProtocol is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoTypeDataProtocols) SetDataProtocol(newValue []DataProtocolType) *NetInterfaceInfoTypeDataProtocols {
	newSlice := make([]DataProtocolType, len(newValue))
	copy(newSlice, newValue)
	o.DataProtocolPtr = newSlice
	return o
}

// DataProtocols is a 'getter' method
func (o *NetInterfaceInfoType) DataProtocols() NetInterfaceInfoTypeDataProtocols {
	var r NetInterfaceInfoTypeDataProtocols
	if o.DataProtocolsPtr == nil {
		return r
	}
	r = *o.DataProtocolsPtr
	return r
}

// SetDataProtocols is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetDataProtocols(newValue NetInterfaceInfoTypeDataProtocols) *NetInterfaceInfoType {
	o.DataProtocolsPtr = &newValue
	return o
}

// DnsDomainName is a 'getter' method
func (o *NetInterfaceInfoType) DnsDomainName() DnsZoneType {
	var r DnsZoneType
	if o.DnsDomainNamePtr == nil {
		return r
	}
	r = *o.DnsDomainNamePtr
	return r
}

// SetDnsDomainName is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetDnsDomainName(newValue DnsZoneType) *NetInterfaceInfoType {
	o.DnsDomainNamePtr = &newValue
	return o
}

// ExtendedStatus is a 'getter' method
func (o *NetInterfaceInfoType) ExtendedStatus() string {
	var r string
	if o.ExtendedStatusPtr == nil {
		return r
	}
	r = *o.ExtendedStatusPtr
	return r
}

// SetExtendedStatus is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetExtendedStatus(newValue string) *NetInterfaceInfoType {
	o.ExtendedStatusPtr = &newValue
	return o
}

// FailoverGroup is a 'getter' method
func (o *NetInterfaceInfoType) FailoverGroup() FailoverGroupType {
	var r FailoverGroupType
	if o.FailoverGroupPtr == nil {
		return r
	}
	r = *o.FailoverGroupPtr
	return r
}

// SetFailoverGroup is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetFailoverGroup(newValue FailoverGroupType) *NetInterfaceInfoType {
	o.FailoverGroupPtr = &newValue
	return o
}

// FailoverPolicy is a 'getter' method
func (o *NetInterfaceInfoType) FailoverPolicy() string {
	var r string
	if o.FailoverPolicyPtr == nil {
		return r
	}
	r = *o.FailoverPolicyPtr
	return r
}

// SetFailoverPolicy is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetFailoverPolicy(newValue string) *NetInterfaceInfoType {
	o.FailoverPolicyPtr = &newValue
	return o
}

// FirewallPolicy is a 'getter' method
func (o *NetInterfaceInfoType) FirewallPolicy() string {
	var r string
	if o.FirewallPolicyPtr == nil {
		return r
	}
	r = *o.FirewallPolicyPtr
	return r
}

// SetFirewallPolicy is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetFirewallPolicy(newValue string) *NetInterfaceInfoType {
	o.FirewallPolicyPtr = &newValue
	return o
}

// ForceSubnetAssociation is a 'getter' method
func (o *NetInterfaceInfoType) ForceSubnetAssociation() bool {
	var r bool
	if o.ForceSubnetAssociationPtr == nil {
		return r
	}
	r = *o.ForceSubnetAssociationPtr
	return r
}

// SetForceSubnetAssociation is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetForceSubnetAssociation(newValue bool) *NetInterfaceInfoType {
	o.ForceSubnetAssociationPtr = &newValue
	return o
}

// HomeNode is a 'getter' method
func (o *NetInterfaceInfoType) HomeNode() string {
	var r string
	if o.HomeNodePtr == nil {
		return r
	}
	r = *o.HomeNodePtr
	return r
}

// SetHomeNode is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetHomeNode(newValue string) *NetInterfaceInfoType {
	o.HomeNodePtr = &newValue
	return o
}

// HomePort is a 'getter' method
func (o *NetInterfaceInfoType) HomePort() string {
	var r string
	if o.HomePortPtr == nil {
		return r
	}
	r = *o.HomePortPtr
	return r
}

// SetHomePort is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetHomePort(newValue string) *NetInterfaceInfoType {
	o.HomePortPtr = &newValue
	return o
}

// InterfaceName is a 'getter' method
func (o *NetInterfaceInfoType) InterfaceName() string {
	var r string
	if o.InterfaceNamePtr == nil {
		return r
	}
	r = *o.InterfaceNamePtr
	return r
}

// SetInterfaceName is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetInterfaceName(newValue string) *NetInterfaceInfoType {
	o.InterfaceNamePtr = &newValue
	return o
}

// Ipspace is a 'getter' method
func (o *NetInterfaceInfoType) Ipspace() string {
	var r string
	if o.IpspacePtr == nil {
		return r
	}
	r = *o.IpspacePtr
	return r
}

// SetIpspace is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetIpspace(newValue string) *NetInterfaceInfoType {
	o.IpspacePtr = &newValue
	return o
}

// IsAutoRevert is a 'getter' method
func (o *NetInterfaceInfoType) IsAutoRevert() bool {
	var r bool
	if o.IsAutoRevertPtr == nil {
		return r
	}
	r = *o.IsAutoRevertPtr
	return r
}

// SetIsAutoRevert is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetIsAutoRevert(newValue bool) *NetInterfaceInfoType {
	o.IsAutoRevertPtr = &newValue
	return o
}

// IsDnsUpdateEnabled is a 'getter' method
func (o *NetInterfaceInfoType) IsDnsUpdateEnabled() bool {
	var r bool
	if o.IsDnsUpdateEnabledPtr == nil {
		return r
	}
	r = *o.IsDnsUpdateEnabledPtr
	return r
}

// SetIsDnsUpdateEnabled is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetIsDnsUpdateEnabled(newValue bool) *NetInterfaceInfoType {
	o.IsDnsUpdateEnabledPtr = &newValue
	return o
}

// IsHome is a 'getter' method
func (o *NetInterfaceInfoType) IsHome() bool {
	var r bool
	if o.IsHomePtr == nil {
		return r
	}
	r = *o.IsHomePtr
	return r
}

// SetIsHome is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetIsHome(newValue bool) *NetInterfaceInfoType {
	o.IsHomePtr = &newValue
	return o
}

// IsIpv4LinkLocal is a 'getter' method
func (o *NetInterfaceInfoType) IsIpv4LinkLocal() bool {
	var r bool
	if o.IsIpv4LinkLocalPtr == nil {
		return r
	}
	r = *o.IsIpv4LinkLocalPtr
	return r
}

// SetIsIpv4LinkLocal is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetIsIpv4LinkLocal(newValue bool) *NetInterfaceInfoType {
	o.IsIpv4LinkLocalPtr = &newValue
	return o
}

// IsVip is a 'getter' method
func (o *NetInterfaceInfoType) IsVip() bool {
	var r bool
	if o.IsVipPtr == nil {
		return r
	}
	r = *o.IsVipPtr
	return r
}

// SetIsVip is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetIsVip(newValue bool) *NetInterfaceInfoType {
	o.IsVipPtr = &newValue
	return o
}

// LifUuid is a 'getter' method
func (o *NetInterfaceInfoType) LifUuid() UuidType {
	var r UuidType
	if o.LifUuidPtr == nil {
		return r
	}
	r = *o.LifUuidPtr
	return r
}

// SetLifUuid is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetLifUuid(newValue UuidType) *NetInterfaceInfoType {
	o.LifUuidPtr = &newValue
	return o
}

// ListenForDnsQuery is a 'getter' method
func (o *NetInterfaceInfoType) ListenForDnsQuery() bool {
	var r bool
	if o.ListenForDnsQueryPtr == nil {
		return r
	}
	r = *o.ListenForDnsQueryPtr
	return r
}

// SetListenForDnsQuery is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetListenForDnsQuery(newValue bool) *NetInterfaceInfoType {
	o.ListenForDnsQueryPtr = &newValue
	return o
}

// Netmask is a 'getter' method
func (o *NetInterfaceInfoType) Netmask() IpAddressType {
	var r IpAddressType
	if o.NetmaskPtr == nil {
		return r
	}
	r = *o.NetmaskPtr
	return r
}

// SetNetmask is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetNetmask(newValue IpAddressType) *NetInterfaceInfoType {
	o.NetmaskPtr = &newValue
	return o
}

// NetmaskLength is a 'getter' method
func (o *NetInterfaceInfoType) NetmaskLength() int {
	var r int
	if o.NetmaskLengthPtr == nil {
		return r
	}
	r = *o.NetmaskLengthPtr
	return r
}

// SetNetmaskLength is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetNetmaskLength(newValue int) *NetInterfaceInfoType {
	o.NetmaskLengthPtr = &newValue
	return o
}

// OperationalStatus is a 'getter' method
func (o *NetInterfaceInfoType) OperationalStatus() string {
	var r string
	if o.OperationalStatusPtr == nil {
		return r
	}
	r = *o.OperationalStatusPtr
	return r
}

// SetOperationalStatus is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetOperationalStatus(newValue string) *NetInterfaceInfoType {
	o.OperationalStatusPtr = &newValue
	return o
}

// ProbePort is a 'getter' method
func (o *NetInterfaceInfoType) ProbePort() int {
	var r int
	if o.ProbePortPtr == nil {
		return r
	}
	r = *o.ProbePortPtr
	return r
}

// SetProbePort is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetProbePort(newValue int) *NetInterfaceInfoType {
	o.ProbePortPtr = &newValue
	return o
}

// Role is a 'getter' method
func (o *NetInterfaceInfoType) Role() string {
	var r string
	if o.RolePtr == nil {
		return r
	}
	r = *o.RolePtr
	return r
}

// SetRole is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetRole(newValue string) *NetInterfaceInfoType {
	o.RolePtr = &newValue
	return o
}

// RoutingGroupName is a 'getter' method
func (o *NetInterfaceInfoType) RoutingGroupName() string {
	var r string
	if o.RoutingGroupNamePtr == nil {
		return r
	}
	r = *o.RoutingGroupNamePtr
	return r
}

// SetRoutingGroupName is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetRoutingGroupName(newValue string) *NetInterfaceInfoType {
	o.RoutingGroupNamePtr = &newValue
	return o
}

// NetInterfaceInfoTypeServiceNames is a wrapper
type NetInterfaceInfoTypeServiceNames struct {
	XMLName           xml.Name             `xml:"service-names"`
	LifServiceNamePtr []LifServiceNameType `xml:"lif-service-name"`
}

// LifServiceName is a 'getter' method
func (o *NetInterfaceInfoTypeServiceNames) LifServiceName() []LifServiceNameType {
	r := o.LifServiceNamePtr
	return r
}

// SetLifServiceName is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoTypeServiceNames) SetLifServiceName(newValue []LifServiceNameType) *NetInterfaceInfoTypeServiceNames {
	newSlice := make([]LifServiceNameType, len(newValue))
	copy(newSlice, newValue)
	o.LifServiceNamePtr = newSlice
	return o
}

// ServiceNames is a 'getter' method
func (o *NetInterfaceInfoType) ServiceNames() NetInterfaceInfoTypeServiceNames {
	var r NetInterfaceInfoTypeServiceNames
	if o.ServiceNamesPtr == nil {
		return r
	}
	r = *o.ServiceNamesPtr
	return r
}

// SetServiceNames is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetServiceNames(newValue NetInterfaceInfoTypeServiceNames) *NetInterfaceInfoType {
	o.ServiceNamesPtr = &newValue
	return o
}

// ServicePolicy is a 'getter' method
func (o *NetInterfaceInfoType) ServicePolicy() string {
	var r string
	if o.ServicePolicyPtr == nil {
		return r
	}
	r = *o.ServicePolicyPtr
	return r
}

// SetServicePolicy is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetServicePolicy(newValue string) *NetInterfaceInfoType {
	o.ServicePolicyPtr = &newValue
	return o
}

// SubnetName is a 'getter' method
func (o *NetInterfaceInfoType) SubnetName() SubnetNameType {
	var r SubnetNameType
	if o.SubnetNamePtr == nil {
		return r
	}
	r = *o.SubnetNamePtr
	return r
}

// SetSubnetName is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetSubnetName(newValue SubnetNameType) *NetInterfaceInfoType {
	o.SubnetNamePtr = &newValue
	return o
}

// UseFailoverGroup is a 'getter' method
func (o *NetInterfaceInfoType) UseFailoverGroup() string {
	var r string
	if o.UseFailoverGroupPtr == nil {
		return r
	}
	r = *o.UseFailoverGroupPtr
	return r
}

// SetUseFailoverGroup is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetUseFailoverGroup(newValue string) *NetInterfaceInfoType {
	o.UseFailoverGroupPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *NetInterfaceInfoType) Vserver() string {
	var r string
	if o.VserverPtr == nil {
		return r
	}
	r = *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetVserver(newValue string) *NetInterfaceInfoType {
	o.VserverPtr = &newValue
	return o
}

// Wwpn is a 'getter' method
func (o *NetInterfaceInfoType) Wwpn() string {
	var r string
	if o.WwpnPtr == nil {
		return r
	}
	r = *o.WwpnPtr
	return r
}

// SetWwpn is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetWwpn(newValue string) *NetInterfaceInfoType {
	o.WwpnPtr = &newValue
	return o
}
