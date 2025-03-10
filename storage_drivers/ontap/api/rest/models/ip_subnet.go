// Code generated by go-swagger; DO NOT EDIT.

package models

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"strconv"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/go-openapi/validate"
)

// IPSubnet ip subnet
//
// swagger:model ip_subnet
type IPSubnet struct {

	// links
	Links *IPSubnetInlineLinks `json:"_links,omitempty"`

	// available count
	// Read Only: true
	AvailableCount *int64 `json:"available_count,omitempty"`

	// broadcast domain
	BroadcastDomain *IPSubnetInlineBroadcastDomain `json:"broadcast_domain,omitempty"`

	// This action will fail if any existing interface is using an IP address in the ranges provided. Set this to false to associate any manually addressed interfaces with the subnet and allow the action to succeed.
	FailIfLifsConflict *bool `json:"fail_if_lifs_conflict,omitempty"`

	// The IP address of the gateway for this subnet.
	// Example: 10.1.1.1
	Gateway *string `json:"gateway,omitempty"`

	// ip subnet inline available ip ranges
	// Read Only: true
	IPSubnetInlineAvailableIPRanges []*IPAddressRange `json:"available_ip_ranges,omitempty"`

	// ip subnet inline ip ranges
	IPSubnetInlineIPRanges []*IPAddressRange `json:"ip_ranges,omitempty"`

	// ipspace
	Ipspace *IPSubnetInlineIpspace `json:"ipspace,omitempty"`

	// Subnet name
	// Example: subnet1
	Name *string `json:"name,omitempty"`

	// subnet
	Subnet *IPInfo `json:"subnet,omitempty"`

	// total count
	// Read Only: true
	TotalCount *int64 `json:"total_count,omitempty"`

	// used count
	// Read Only: true
	UsedCount *int64 `json:"used_count,omitempty"`

	// The UUID that uniquely identifies the subnet.
	// Example: 1cd8a442-86d1-11e0-ae1c-123478563412
	// Read Only: true
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this ip subnet
func (m *IPSubnet) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateBroadcastDomain(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateIPSubnetInlineAvailableIPRanges(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateIPSubnetInlineIPRanges(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateIpspace(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateSubnet(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnet) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(m.Links) { // not required
		return nil
	}

	if m.Links != nil {
		if err := m.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links")
			}
			return err
		}
	}

	return nil
}

func (m *IPSubnet) validateBroadcastDomain(formats strfmt.Registry) error {
	if swag.IsZero(m.BroadcastDomain) { // not required
		return nil
	}

	if m.BroadcastDomain != nil {
		if err := m.BroadcastDomain.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("broadcast_domain")
			}
			return err
		}
	}

	return nil
}

func (m *IPSubnet) validateIPSubnetInlineAvailableIPRanges(formats strfmt.Registry) error {
	if swag.IsZero(m.IPSubnetInlineAvailableIPRanges) { // not required
		return nil
	}

	for i := 0; i < len(m.IPSubnetInlineAvailableIPRanges); i++ {
		if swag.IsZero(m.IPSubnetInlineAvailableIPRanges[i]) { // not required
			continue
		}

		if m.IPSubnetInlineAvailableIPRanges[i] != nil {
			if err := m.IPSubnetInlineAvailableIPRanges[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("available_ip_ranges" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (m *IPSubnet) validateIPSubnetInlineIPRanges(formats strfmt.Registry) error {
	if swag.IsZero(m.IPSubnetInlineIPRanges) { // not required
		return nil
	}

	for i := 0; i < len(m.IPSubnetInlineIPRanges); i++ {
		if swag.IsZero(m.IPSubnetInlineIPRanges[i]) { // not required
			continue
		}

		if m.IPSubnetInlineIPRanges[i] != nil {
			if err := m.IPSubnetInlineIPRanges[i].Validate(formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("ip_ranges" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (m *IPSubnet) validateIpspace(formats strfmt.Registry) error {
	if swag.IsZero(m.Ipspace) { // not required
		return nil
	}

	if m.Ipspace != nil {
		if err := m.Ipspace.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ipspace")
			}
			return err
		}
	}

	return nil
}

func (m *IPSubnet) validateSubnet(formats strfmt.Registry) error {
	if swag.IsZero(m.Subnet) { // not required
		return nil
	}

	if m.Subnet != nil {
		if err := m.Subnet.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("subnet")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this ip subnet based on the context it is used
func (m *IPSubnet) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateAvailableCount(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateBroadcastDomain(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateIPSubnetInlineAvailableIPRanges(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateIPSubnetInlineIPRanges(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateIpspace(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateSubnet(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateTotalCount(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateUsedCount(ctx, formats); err != nil {
		res = append(res, err)
	}

	if err := m.contextValidateUUID(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnet) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if m.Links != nil {
		if err := m.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links")
			}
			return err
		}
	}

	return nil
}

func (m *IPSubnet) contextValidateAvailableCount(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "available_count", "body", m.AvailableCount); err != nil {
		return err
	}

	return nil
}

func (m *IPSubnet) contextValidateBroadcastDomain(ctx context.Context, formats strfmt.Registry) error {

	if m.BroadcastDomain != nil {
		if err := m.BroadcastDomain.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("broadcast_domain")
			}
			return err
		}
	}

	return nil
}

func (m *IPSubnet) contextValidateIPSubnetInlineAvailableIPRanges(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "available_ip_ranges", "body", []*IPAddressRange(m.IPSubnetInlineAvailableIPRanges)); err != nil {
		return err
	}

	for i := 0; i < len(m.IPSubnetInlineAvailableIPRanges); i++ {

		if m.IPSubnetInlineAvailableIPRanges[i] != nil {
			if err := m.IPSubnetInlineAvailableIPRanges[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("available_ip_ranges" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (m *IPSubnet) contextValidateIPSubnetInlineIPRanges(ctx context.Context, formats strfmt.Registry) error {

	for i := 0; i < len(m.IPSubnetInlineIPRanges); i++ {

		if m.IPSubnetInlineIPRanges[i] != nil {
			if err := m.IPSubnetInlineIPRanges[i].ContextValidate(ctx, formats); err != nil {
				if ve, ok := err.(*errors.Validation); ok {
					return ve.ValidateName("ip_ranges" + "." + strconv.Itoa(i))
				}
				return err
			}
		}

	}

	return nil
}

func (m *IPSubnet) contextValidateIpspace(ctx context.Context, formats strfmt.Registry) error {

	if m.Ipspace != nil {
		if err := m.Ipspace.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ipspace")
			}
			return err
		}
	}

	return nil
}

func (m *IPSubnet) contextValidateSubnet(ctx context.Context, formats strfmt.Registry) error {

	if m.Subnet != nil {
		if err := m.Subnet.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("subnet")
			}
			return err
		}
	}

	return nil
}

func (m *IPSubnet) contextValidateTotalCount(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "total_count", "body", m.TotalCount); err != nil {
		return err
	}

	return nil
}

func (m *IPSubnet) contextValidateUsedCount(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "used_count", "body", m.UsedCount); err != nil {
		return err
	}

	return nil
}

func (m *IPSubnet) contextValidateUUID(ctx context.Context, formats strfmt.Registry) error {

	if err := validate.ReadOnly(ctx, "uuid", "body", m.UUID); err != nil {
		return err
	}

	return nil
}

// MarshalBinary interface implementation
func (m *IPSubnet) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *IPSubnet) UnmarshalBinary(b []byte) error {
	var res IPSubnet
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// IPSubnetInlineBroadcastDomain The broadcast domain that the subnet is associated with. Either the UUID or name must be supplied on POST.
//
// swagger:model ip_subnet_inline_broadcast_domain
type IPSubnetInlineBroadcastDomain struct {

	// links
	Links *IPSubnetInlineBroadcastDomainInlineLinks `json:"_links,omitempty"`

	// Name of the broadcast domain, scoped to its IPspace
	// Example: bd1
	Name *string `json:"name,omitempty"`

	// Broadcast domain UUID
	// Example: 1cd8a442-86d1-11e0-ae1c-123478563412
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this ip subnet inline broadcast domain
func (m *IPSubnetInlineBroadcastDomain) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnetInlineBroadcastDomain) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(m.Links) { // not required
		return nil
	}

	if m.Links != nil {
		if err := m.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("broadcast_domain" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this ip subnet inline broadcast domain based on the context it is used
func (m *IPSubnetInlineBroadcastDomain) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnetInlineBroadcastDomain) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if m.Links != nil {
		if err := m.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("broadcast_domain" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *IPSubnetInlineBroadcastDomain) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *IPSubnetInlineBroadcastDomain) UnmarshalBinary(b []byte) error {
	var res IPSubnetInlineBroadcastDomain
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// IPSubnetInlineBroadcastDomainInlineLinks ip subnet inline broadcast domain inline links
//
// swagger:model ip_subnet_inline_broadcast_domain_inline__links
type IPSubnetInlineBroadcastDomainInlineLinks struct {

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this ip subnet inline broadcast domain inline links
func (m *IPSubnetInlineBroadcastDomainInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnetInlineBroadcastDomainInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(m.Self) { // not required
		return nil
	}

	if m.Self != nil {
		if err := m.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("broadcast_domain" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this ip subnet inline broadcast domain inline links based on the context it is used
func (m *IPSubnetInlineBroadcastDomainInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnetInlineBroadcastDomainInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if m.Self != nil {
		if err := m.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("broadcast_domain" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *IPSubnetInlineBroadcastDomainInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *IPSubnetInlineBroadcastDomainInlineLinks) UnmarshalBinary(b []byte) error {
	var res IPSubnetInlineBroadcastDomainInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// IPSubnetInlineIpspace The IPspace that the subnet is associated with. Either the UUID or name must be supplied on POST.
//
// swagger:model ip_subnet_inline_ipspace
type IPSubnetInlineIpspace struct {

	// links
	Links *IPSubnetInlineIpspaceInlineLinks `json:"_links,omitempty"`

	// IPspace name
	// Example: Default
	Name *string `json:"name,omitempty"`

	// IPspace UUID
	// Example: 1cd8a442-86d1-11e0-ae1c-123478563412
	UUID *string `json:"uuid,omitempty"`
}

// Validate validates this ip subnet inline ipspace
func (m *IPSubnetInlineIpspace) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateLinks(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnetInlineIpspace) validateLinks(formats strfmt.Registry) error {
	if swag.IsZero(m.Links) { // not required
		return nil
	}

	if m.Links != nil {
		if err := m.Links.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ipspace" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this ip subnet inline ipspace based on the context it is used
func (m *IPSubnetInlineIpspace) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateLinks(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnetInlineIpspace) contextValidateLinks(ctx context.Context, formats strfmt.Registry) error {

	if m.Links != nil {
		if err := m.Links.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ipspace" + "." + "_links")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *IPSubnetInlineIpspace) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *IPSubnetInlineIpspace) UnmarshalBinary(b []byte) error {
	var res IPSubnetInlineIpspace
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// IPSubnetInlineIpspaceInlineLinks ip subnet inline ipspace inline links
//
// swagger:model ip_subnet_inline_ipspace_inline__links
type IPSubnetInlineIpspaceInlineLinks struct {

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this ip subnet inline ipspace inline links
func (m *IPSubnetInlineIpspaceInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnetInlineIpspaceInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(m.Self) { // not required
		return nil
	}

	if m.Self != nil {
		if err := m.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ipspace" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this ip subnet inline ipspace inline links based on the context it is used
func (m *IPSubnetInlineIpspaceInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnetInlineIpspaceInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if m.Self != nil {
		if err := m.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("ipspace" + "." + "_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *IPSubnetInlineIpspaceInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *IPSubnetInlineIpspaceInlineLinks) UnmarshalBinary(b []byte) error {
	var res IPSubnetInlineIpspaceInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}

// IPSubnetInlineLinks ip subnet inline links
//
// swagger:model ip_subnet_inline__links
type IPSubnetInlineLinks struct {

	// self
	Self *Href `json:"self,omitempty"`
}

// Validate validates this ip subnet inline links
func (m *IPSubnetInlineLinks) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateSelf(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnetInlineLinks) validateSelf(formats strfmt.Registry) error {
	if swag.IsZero(m.Self) { // not required
		return nil
	}

	if m.Self != nil {
		if err := m.Self.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this ip subnet inline links based on the context it is used
func (m *IPSubnetInlineLinks) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateSelf(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *IPSubnetInlineLinks) contextValidateSelf(ctx context.Context, formats strfmt.Registry) error {

	if m.Self != nil {
		if err := m.Self.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("_links" + "." + "self")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *IPSubnetInlineLinks) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *IPSubnetInlineLinks) UnmarshalBinary(b []byte) error {
	var res IPSubnetInlineLinks
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
