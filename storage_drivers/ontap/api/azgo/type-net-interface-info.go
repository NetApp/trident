package azgo

import (
	"encoding/xml"
	"reflect"

	log "github.com/sirupsen/logrus"
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
	DnsDomainNamePtr          *DnsZoneType          `xml:"dns-domain-name"`
	ExtendedStatusPtr         *string               `xml:"extended-status"`
	FailoverGroupPtr          *FailoverGroupType    `xml:"failover-group"`
	FailoverPolicyPtr         *string               `xml:"failover-policy"`
	FirewallPolicyPtr         *string               `xml:"firewall-policy"`
	ForceSubnetAssociationPtr *bool                 `xml:"force-subnet-association"`
	HomeNodePtr               *string               `xml:"home-node"`
	HomePortPtr               *string               `xml:"home-port"`
	InterfaceNamePtr          *string               `xml:"interface-name"`
	IpspacePtr                *string               `xml:"ipspace"`
	IsAutoRevertPtr           *bool                 `xml:"is-auto-revert"`
	IsDnsUpdateEnabledPtr     *bool                 `xml:"is-dns-update-enabled"`
	IsHomePtr                 *bool                 `xml:"is-home"`
	IsIpv4LinkLocalPtr        *bool                 `xml:"is-ipv4-link-local"`
	LifUuidPtr                *UuidType             `xml:"lif-uuid"`
	ListenForDnsQueryPtr      *bool                 `xml:"listen-for-dns-query"`
	NetmaskPtr                *IpAddressType        `xml:"netmask"`
	NetmaskLengthPtr          *int                  `xml:"netmask-length"`
	OperationalStatusPtr      *string               `xml:"operational-status"`
	RolePtr                   *string               `xml:"role"`
	RoutingGroupNamePtr       *RoutingGroupTypeType `xml:"routing-group-name"`
	SubnetNamePtr             *SubnetNameType       `xml:"subnet-name"`
	UseFailoverGroupPtr       *string               `xml:"use-failover-group"`
	VserverPtr                *string               `xml:"vserver"`
	WwpnPtr                   *string               `xml:"wwpn"`
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
	if o.AddressPtr != nil {
		return *o.AddressPtr
	}
	return ""
}

// SetAddress is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetAddress(newValue IpAddressType) *NetInterfaceInfoType {
	o.AddressPtr = &newValue
	return o
}

// AddressFamily is a 'getter' method
func (o *NetInterfaceInfoType) AddressFamily() string {
	r := *o.AddressFamilyPtr
	return r
}

// SetAddressFamily is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetAddressFamily(newValue string) *NetInterfaceInfoType {
	o.AddressFamilyPtr = &newValue
	return o
}

// AdministrativeStatus is a 'getter' method
func (o *NetInterfaceInfoType) AdministrativeStatus() string {
	r := *o.AdministrativeStatusPtr
	return r
}

// SetAdministrativeStatus is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetAdministrativeStatus(newValue string) *NetInterfaceInfoType {
	o.AdministrativeStatusPtr = &newValue
	return o
}

// Comment is a 'getter' method
func (o *NetInterfaceInfoType) Comment() string {
	r := *o.CommentPtr
	return r
}

// SetComment is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetComment(newValue string) *NetInterfaceInfoType {
	o.CommentPtr = &newValue
	return o
}

// CurrentNode is a 'getter' method
func (o *NetInterfaceInfoType) CurrentNode() string {
	r := *o.CurrentNodePtr
	return r
}

// SetCurrentNode is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetCurrentNode(newValue string) *NetInterfaceInfoType {
	o.CurrentNodePtr = &newValue
	return o
}

// CurrentPort is a 'getter' method
func (o *NetInterfaceInfoType) CurrentPort() string {
	r := *o.CurrentPortPtr
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
	r := *o.DataProtocolsPtr
	return r
}

// SetDataProtocols is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetDataProtocols(newValue NetInterfaceInfoTypeDataProtocols) *NetInterfaceInfoType {
	o.DataProtocolsPtr = &newValue
	return o
}

// DnsDomainName is a 'getter' method
func (o *NetInterfaceInfoType) DnsDomainName() DnsZoneType {
	r := *o.DnsDomainNamePtr
	return r
}

// SetDnsDomainName is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetDnsDomainName(newValue DnsZoneType) *NetInterfaceInfoType {
	o.DnsDomainNamePtr = &newValue
	return o
}

// ExtendedStatus is a 'getter' method
func (o *NetInterfaceInfoType) ExtendedStatus() string {
	r := *o.ExtendedStatusPtr
	return r
}

// SetExtendedStatus is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetExtendedStatus(newValue string) *NetInterfaceInfoType {
	o.ExtendedStatusPtr = &newValue
	return o
}

// FailoverGroup is a 'getter' method
func (o *NetInterfaceInfoType) FailoverGroup() FailoverGroupType {
	r := *o.FailoverGroupPtr
	return r
}

// SetFailoverGroup is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetFailoverGroup(newValue FailoverGroupType) *NetInterfaceInfoType {
	o.FailoverGroupPtr = &newValue
	return o
}

// FailoverPolicy is a 'getter' method
func (o *NetInterfaceInfoType) FailoverPolicy() string {
	r := *o.FailoverPolicyPtr
	return r
}

// SetFailoverPolicy is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetFailoverPolicy(newValue string) *NetInterfaceInfoType {
	o.FailoverPolicyPtr = &newValue
	return o
}

// FirewallPolicy is a 'getter' method
func (o *NetInterfaceInfoType) FirewallPolicy() string {
	r := *o.FirewallPolicyPtr
	return r
}

// SetFirewallPolicy is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetFirewallPolicy(newValue string) *NetInterfaceInfoType {
	o.FirewallPolicyPtr = &newValue
	return o
}

// ForceSubnetAssociation is a 'getter' method
func (o *NetInterfaceInfoType) ForceSubnetAssociation() bool {
	r := *o.ForceSubnetAssociationPtr
	return r
}

// SetForceSubnetAssociation is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetForceSubnetAssociation(newValue bool) *NetInterfaceInfoType {
	o.ForceSubnetAssociationPtr = &newValue
	return o
}

// HomeNode is a 'getter' method
func (o *NetInterfaceInfoType) HomeNode() string {
	r := *o.HomeNodePtr
	return r
}

// SetHomeNode is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetHomeNode(newValue string) *NetInterfaceInfoType {
	o.HomeNodePtr = &newValue
	return o
}

// HomePort is a 'getter' method
func (o *NetInterfaceInfoType) HomePort() string {
	r := *o.HomePortPtr
	return r
}

// SetHomePort is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetHomePort(newValue string) *NetInterfaceInfoType {
	o.HomePortPtr = &newValue
	return o
}

// InterfaceName is a 'getter' method
func (o *NetInterfaceInfoType) InterfaceName() string {
	r := *o.InterfaceNamePtr
	return r
}

// SetInterfaceName is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetInterfaceName(newValue string) *NetInterfaceInfoType {
	o.InterfaceNamePtr = &newValue
	return o
}

// Ipspace is a 'getter' method
func (o *NetInterfaceInfoType) Ipspace() string {
	r := *o.IpspacePtr
	return r
}

// SetIpspace is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetIpspace(newValue string) *NetInterfaceInfoType {
	o.IpspacePtr = &newValue
	return o
}

// IsAutoRevert is a 'getter' method
func (o *NetInterfaceInfoType) IsAutoRevert() bool {
	r := *o.IsAutoRevertPtr
	return r
}

// SetIsAutoRevert is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetIsAutoRevert(newValue bool) *NetInterfaceInfoType {
	o.IsAutoRevertPtr = &newValue
	return o
}

// IsDnsUpdateEnabled is a 'getter' method
func (o *NetInterfaceInfoType) IsDnsUpdateEnabled() bool {
	r := *o.IsDnsUpdateEnabledPtr
	return r
}

// SetIsDnsUpdateEnabled is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetIsDnsUpdateEnabled(newValue bool) *NetInterfaceInfoType {
	o.IsDnsUpdateEnabledPtr = &newValue
	return o
}

// IsHome is a 'getter' method
func (o *NetInterfaceInfoType) IsHome() bool {
	r := *o.IsHomePtr
	return r
}

// SetIsHome is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetIsHome(newValue bool) *NetInterfaceInfoType {
	o.IsHomePtr = &newValue
	return o
}

// IsIpv4LinkLocal is a 'getter' method
func (o *NetInterfaceInfoType) IsIpv4LinkLocal() bool {
	r := *o.IsIpv4LinkLocalPtr
	return r
}

// SetIsIpv4LinkLocal is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetIsIpv4LinkLocal(newValue bool) *NetInterfaceInfoType {
	o.IsIpv4LinkLocalPtr = &newValue
	return o
}

// LifUuid is a 'getter' method
func (o *NetInterfaceInfoType) LifUuid() UuidType {
	r := *o.LifUuidPtr
	return r
}

// SetLifUuid is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetLifUuid(newValue UuidType) *NetInterfaceInfoType {
	o.LifUuidPtr = &newValue
	return o
}

// ListenForDnsQuery is a 'getter' method
func (o *NetInterfaceInfoType) ListenForDnsQuery() bool {
	r := *o.ListenForDnsQueryPtr
	return r
}

// SetListenForDnsQuery is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetListenForDnsQuery(newValue bool) *NetInterfaceInfoType {
	o.ListenForDnsQueryPtr = &newValue
	return o
}

// Netmask is a 'getter' method
func (o *NetInterfaceInfoType) Netmask() IpAddressType {
	r := *o.NetmaskPtr
	return r
}

// SetNetmask is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetNetmask(newValue IpAddressType) *NetInterfaceInfoType {
	o.NetmaskPtr = &newValue
	return o
}

// NetmaskLength is a 'getter' method
func (o *NetInterfaceInfoType) NetmaskLength() int {
	r := *o.NetmaskLengthPtr
	return r
}

// SetNetmaskLength is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetNetmaskLength(newValue int) *NetInterfaceInfoType {
	o.NetmaskLengthPtr = &newValue
	return o
}

// OperationalStatus is a 'getter' method
func (o *NetInterfaceInfoType) OperationalStatus() string {
	r := *o.OperationalStatusPtr
	return r
}

// SetOperationalStatus is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetOperationalStatus(newValue string) *NetInterfaceInfoType {
	o.OperationalStatusPtr = &newValue
	return o
}

// Role is a 'getter' method
func (o *NetInterfaceInfoType) Role() string {
	r := *o.RolePtr
	return r
}

// SetRole is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetRole(newValue string) *NetInterfaceInfoType {
	o.RolePtr = &newValue
	return o
}

// RoutingGroupName is a 'getter' method
func (o *NetInterfaceInfoType) RoutingGroupName() RoutingGroupTypeType {
	r := *o.RoutingGroupNamePtr
	return r
}

// SetRoutingGroupName is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetRoutingGroupName(newValue RoutingGroupTypeType) *NetInterfaceInfoType {
	o.RoutingGroupNamePtr = &newValue
	return o
}

// SubnetName is a 'getter' method
func (o *NetInterfaceInfoType) SubnetName() SubnetNameType {
	r := *o.SubnetNamePtr
	return r
}

// SetSubnetName is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetSubnetName(newValue SubnetNameType) *NetInterfaceInfoType {
	o.SubnetNamePtr = &newValue
	return o
}

// UseFailoverGroup is a 'getter' method
func (o *NetInterfaceInfoType) UseFailoverGroup() string {
	r := *o.UseFailoverGroupPtr
	return r
}

// SetUseFailoverGroup is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetUseFailoverGroup(newValue string) *NetInterfaceInfoType {
	o.UseFailoverGroupPtr = &newValue
	return o
}

// Vserver is a 'getter' method
func (o *NetInterfaceInfoType) Vserver() string {
	r := *o.VserverPtr
	return r
}

// SetVserver is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetVserver(newValue string) *NetInterfaceInfoType {
	o.VserverPtr = &newValue
	return o
}

// Wwpn is a 'getter' method
func (o *NetInterfaceInfoType) Wwpn() string {
	r := *o.WwpnPtr
	return r
}

// SetWwpn is a fluent style 'setter' method that can be chained
func (o *NetInterfaceInfoType) SetWwpn(newValue string) *NetInterfaceInfoType {
	o.WwpnPtr = &newValue
	return o
}
